//go:build darwin || linux

// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	safeWorkdirCreateAttempts     = 8
	safeWorkdirDeleteMaxRounds    = 4
	safeWorkdirDeleteBatchSize    = 64
	safeWorkdirIdentityTokenBytes = 16
)

var errSafeWorkdirParentMissing = errors.New("safe workdir parent missing")

type safeWorkdirPlatform struct {
	rootFD     int
	controlFD  int
	leaseFD    int
	rejectedFD int
	lockFD     int
	lockHeld   bool

	instanceID         string
	lockGeneration     string
	previousInstance   string
	previousGeneration string
	previousValid      bool

	// Tests use this synchronous hook to exercise the rename/recreate race.
	afterQuarantineRemoved func()
}

type safeWorkdirRootLockState struct {
	Version        int    `json:"version"`
	InstanceID     string `json:"instance_id"`
	LockGeneration string `json:"lock_generation"`
	PID            int    `json:"pid"`
}

type safeWorkdirIdentity struct {
	device uint64
	inode  uint64
}

type safeWorkdirDeleteBudget struct {
	limits  SafeWorkdirDeleteLimits
	entries int
	bytes   int64
}

func newSafeWorkdirPlatform(root string) (*safeWorkdirPlatform, error) {
	rootFD, err := openSafeWorkdirAbsoluteDirectory(root)
	if err != nil || !safeWorkdirOwnedDirectoryFD(rootFD) {
		if rootFD >= 0 {
			_ = unix.Close(rootFD)
		}
		return nil, ErrSafeWorkdirInvalid
	}
	platform := &safeWorkdirPlatform{
		rootFD: rootFD, controlFD: -1, leaseFD: -1, rejectedFD: -1, lockFD: -1,
	}
	if err := platform.ensureDirectoryAt(rootFD, safeWorkdirTrashDirectory); err != nil {
		_ = platform.close()
		return nil, ErrSafeWorkdirInvalid
	}
	trashFD, err := safeOpenWorkdirDirectoryAt(rootFD, safeWorkdirTrashDirectory)
	if err != nil || !safeWorkdirOwnedDirectoryFD(trashFD) {
		if trashFD >= 0 {
			_ = unix.Close(trashFD)
		}
		_ = platform.close()
		return nil, ErrSafeWorkdirInvalid
	}
	_ = unix.Close(trashFD)
	if err := platform.initializeControlArea(); err != nil {
		_ = platform.close()
		return nil, err
	}
	return platform, nil
}

func (p *safeWorkdirPlatform) close() error {
	if p == nil {
		return nil
	}
	var returnErr error
	if p.lockFD >= 0 && p.lockHeld {
		if err := unix.Flock(p.lockFD, unix.LOCK_UN); err != nil {
			returnErr = err
		}
		p.lockHeld = false
	}
	for _, fd := range []*int{&p.lockFD, &p.leaseFD, &p.rejectedFD, &p.controlFD, &p.rootFD} {
		if *fd >= 0 {
			if err := unix.Close(*fd); err != nil && returnErr == nil {
				returnErr = err
			}
			*fd = -1
		}
	}
	return returnErr
}

func (p *safeWorkdirPlatform) initializeControlArea() error {
	if p == nil || p.rootFD < 0 {
		return ErrSafeWorkdirInvalid
	}
	if err := p.ensureDirectoryAt(p.rootFD, safeWorkdirControlDirectory); err != nil {
		return ErrSafeWorkdirInvalid
	}
	controlFD, err := safeOpenWorkdirDirectoryAt(p.rootFD, safeWorkdirControlDirectory)
	if err != nil || !safeWorkdirOwnedDirectoryFD(controlFD) {
		if controlFD >= 0 {
			_ = unix.Close(controlFD)
		}
		return ErrSafeWorkdirInvalid
	}
	p.controlFD = controlFD
	lockFD, err := unix.Openat(
		controlFD,
		safeWorkdirRootLockName,
		unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0o600,
	)
	if err != nil || !safeWorkdirOwnedRegularFileFD(lockFD, 0o600) {
		if lockFD >= 0 {
			_ = unix.Close(lockFD)
		}
		return ErrSafeWorkdirInvalid
	}
	p.lockFD = lockFD
	if err := unix.Flock(lockFD, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return ErrSafeWorkdirRootLocked
		}
		return ErrSafeWorkdirInvalid
	}
	p.lockHeld = true
	previous, previousOK := readSafeWorkdirRootLockState(lockFD)
	instanceID, err := newSafeWorkdirIdentityToken()
	if err != nil {
		return ErrSafeWorkdirInvalid
	}
	generation, err := newSafeWorkdirIdentityToken()
	if err != nil {
		return ErrSafeWorkdirInvalid
	}
	p.instanceID = instanceID
	p.lockGeneration = generation
	if previousOK {
		p.previousInstance = previous.InstanceID
		p.previousGeneration = previous.LockGeneration
		p.previousValid = previous.InstanceID != "" && previous.LockGeneration != ""
	}
	if err := writeSafeWorkdirRootLockState(lockFD, safeWorkdirRootLockState{
		Version: safeWorkdirRootLockVersion, InstanceID: instanceID,
		LockGeneration: generation, PID: os.Getpid(),
	}); err != nil {
		return ErrSafeWorkdirInvalid
	}
	for name, target := range map[string]*int{
		safeWorkdirLeaseDirectory:    &p.leaseFD,
		safeWorkdirRejectedDirectory: &p.rejectedFD,
	} {
		if err := p.ensureDirectoryAt(controlFD, name); err != nil {
			return ErrSafeWorkdirInvalid
		}
		fd, err := safeOpenWorkdirDirectoryAt(controlFD, name)
		if err != nil || !safeWorkdirOwnedDirectoryFD(fd) {
			if fd >= 0 {
				_ = unix.Close(fd)
			}
			return ErrSafeWorkdirInvalid
		}
		*target = fd
	}
	return nil
}

func (p *safeWorkdirPlatform) hasExclusiveRootLock() bool {
	return p != nil && p.rootFD >= 0 && p.lockFD >= 0 && p.lockHeld
}

func safeWorkdirOwnedRegularFileFD(fd int, mode uint32) bool {
	if fd < 0 {
		return false
	}
	var stat unix.Stat_t
	return unix.Fstat(fd, &stat) == nil && uint32(stat.Mode)&unix.S_IFMT == unix.S_IFREG &&
		uint32(stat.Mode)&0o777 == mode && int(stat.Uid) == os.Geteuid()
}

func readSafeWorkdirRootLockState(fd int) (safeWorkdirRootLockState, bool) {
	if fd < 0 {
		return safeWorkdirRootLockState{}, false
	}
	payload := make([]byte, maximumSafeWorkdirIdentityBytes+1)
	n, err := unix.Pread(fd, payload, 0)
	if err != nil || n <= 0 || n > maximumSafeWorkdirIdentityBytes {
		return safeWorkdirRootLockState{}, false
	}
	payload = payload[:n]
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var state safeWorkdirRootLockState
	if decoder.Decode(&state) != nil || state.Version != safeWorkdirRootLockVersion {
		return safeWorkdirRootLockState{}, false
	}
	var trailing any
	if !errors.Is(decoder.Decode(&trailing), io.EOF) {
		return safeWorkdirRootLockState{}, false
	}
	return state, true
}

func writeSafeWorkdirRootLockState(fd int, state safeWorkdirRootLockState) error {
	payload, err := json.Marshal(state)
	if err != nil || len(payload) == 0 || len(payload) > maximumSafeWorkdirIdentityBytes {
		return ErrSafeWorkdirUnavailable
	}
	if unix.Ftruncate(fd, 0) != nil {
		return ErrSafeWorkdirUnavailable
	}
	if _, err := unix.Seek(fd, 0, io.SeekStart); err != nil {
		return ErrSafeWorkdirUnavailable
	}
	if writeSafeWorkdirBytes(fd, payload) != nil || unix.Fsync(fd) != nil {
		return ErrSafeWorkdirUnavailable
	}
	return nil
}

func newSafeWorkdirIdentityToken() (string, error) {
	payload := make([]byte, safeWorkdirIdentityTokenBytes)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return hex.EncodeToString(payload), nil
}

func (p *safeWorkdirPlatform) matchesRootPath(root string) bool {
	if p == nil || p.rootFD < 0 {
		return false
	}
	currentFD, err := openSafeWorkdirAbsoluteDirectory(root)
	if err != nil {
		return false
	}
	defer unix.Close(currentFD)
	current, ok := safeWorkdirIdentityFD(currentFD)
	pinned, pinnedOK := safeWorkdirIdentityFD(p.rootFD)
	return ok && pinnedOK && current == pinned
}

func (p *safeWorkdirPlatform) create(
	ctx context.Context,
	parent []string,
	prefix string,
) (string, string, error) {
	if !p.hasExclusiveRootLock() || p.leaseFD < 0 {
		return "", "", ErrSafeWorkdirUnavailable
	}
	parentFD, err := p.openOrCreateParent(ctx, parent)
	if err != nil {
		return "", "", err
	}
	defer unix.Close(parentFD)
	for attempt := 0; attempt < safeWorkdirCreateAttempts; attempt++ {
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		nonce := make([]byte, 16)
		if _, err := rand.Read(nonce); err != nil {
			return "", "", err
		}
		name := prefix + hex.EncodeToString(nonce)
		if err := unix.Mkdirat(parentFD, name, 0o700); err != nil {
			if errors.Is(err, syscall.EEXIST) {
				continue
			}
			return "", "", err
		}
		childFD, openErr := safeOpenWorkdirDirectoryAt(parentFD, name)
		if openErr != nil || !safeWorkdirOwnedDirectoryFD(childFD) {
			if childFD >= 0 {
				_ = unix.Close(childFD)
			}
			_ = unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
			return "", "", ErrSafeWorkdirUnavailable
		}
		components := append(append([]string(nil), parent...), name)
		relative := filepath.Join(components...)
		identity, identityOK := safeWorkdirIdentityFD(childFD)
		invocationID, tokenErr := newSafeWorkdirIdentityToken()
		markerName := safeWorkdirIdentityMarkerFilename(relative)
		if !identityOK || tokenErr != nil || writeSafeWorkdirIdentityMarker(p.leaseFD, markerName, safeWorkdirIdentityMarker{
			Version:        safeWorkdirIdentityMarkerVersion,
			RelativePath:   relative,
			Device:         identity.device,
			Inode:          identity.inode,
			InvocationID:   invocationID,
			InstanceID:     p.instanceID,
			LockGeneration: p.lockGeneration,
		}) != nil {
			_ = unix.Unlinkat(p.leaseFD, markerName, 0)
			_ = unix.Close(childFD)
			_ = unix.Unlinkat(parentFD, name, unix.AT_REMOVEDIR)
			return "", "", ErrSafeWorkdirUnavailable
		}
		_ = unix.Close(childFD)
		return relative, invocationID, nil
	}
	return "", "", ErrSafeWorkdirUnavailable
}

func (p *safeWorkdirPlatform) recoverDirectChildren(
	ctx context.Context,
	root string,
	prefix string,
	limits SafeWorkdirDeleteLimits,
) (SafeWorkdirRecoveryReport, error) {
	report := SafeWorkdirRecoveryReport{}
	if !p.hasExclusiveRootLock() || p.leaseFD < 0 || p.rejectedFD < 0 {
		return report, ErrSafeWorkdirCleanupRetryable
	}
	names, truncated, err := boundedSafeWorkdirDirectoryNames(
		p.rootFD,
		limits.MaxEntries,
		limits.MaxBytes,
	)
	if err != nil {
		return report, err
	}
	for _, name := range names {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		report.Scanned++
		status := p.directChildRecoveryStatus(root, name)
		switch status {
		case safeWorkdirRecoveryCurrent:
			report.Pending++
			continue
		case safeWorkdirRecoveryRejected:
			if err := p.isolateRejectedDirectChild(name); err != nil {
				report.Pending++
				continue
			}
			report.Rejected++
			continue
		}
		if err := p.delete(ctx, name, limits); err != nil {
			report.Pending++
			continue
		}
		report.Recovered++
	}
	if report.Rejected > 0 && report.Pending == 0 && !truncated {
		return report, ErrSafeWorkdirRecoveryRejected
	}
	if truncated || report.Pending > 0 {
		return report, ErrSafeWorkdirCleanupRetryable
	}
	return report, nil
}

type safeWorkdirRecoveryStatus uint8

const (
	safeWorkdirRecoveryRejected safeWorkdirRecoveryStatus = iota
	safeWorkdirRecoveryPrevious
	safeWorkdirRecoveryCurrent
)

func (p *safeWorkdirPlatform) directChildRecoveryStatus(root, name string) safeWorkdirRecoveryStatus {
	components, ok := safeWorkdirRelativeComponents(name, false)
	if !ok || len(components) != 1 {
		return safeWorkdirRecoveryRejected
	}
	path := filepath.Join(root, name)
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || filepath.Clean(canonical) != path {
		return safeWorkdirRecoveryRejected
	}
	var stat unix.Stat_t
	if unix.Fstatat(p.rootFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW) != nil ||
		uint32(stat.Mode)&unix.S_IFMT != unix.S_IFDIR || uint32(stat.Mode)&0o777 != 0o700 ||
		int(stat.Uid) != os.Geteuid() {
		return safeWorkdirRecoveryRejected
	}
	directoryFD, err := safeOpenWorkdirDirectoryAt(p.rootFD, name)
	if err != nil || !safeWorkdirOwnedDirectoryFD(directoryFD) {
		if directoryFD >= 0 {
			_ = unix.Close(directoryFD)
		}
		return safeWorkdirRecoveryRejected
	}
	defer unix.Close(directoryFD)
	identity, identityOK := safeWorkdirIdentityFD(directoryFD)
	before := safeWorkdirIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}
	if !identityOK || identity != before {
		return safeWorkdirRecoveryRejected
	}
	marker, markerOK := readSafeWorkdirIdentityMarker(
		p.leaseFD,
		safeWorkdirIdentityMarkerFilename(name),
	)
	if !markerOK || marker.Version != safeWorkdirIdentityMarkerVersion ||
		marker.RelativePath != name || marker.Device != identity.device || marker.Inode != identity.inode ||
		marker.InvocationID == "" || marker.InstanceID == "" || marker.LockGeneration == "" {
		return safeWorkdirRecoveryRejected
	}
	if marker.InstanceID == p.instanceID && marker.LockGeneration == p.lockGeneration {
		return safeWorkdirRecoveryCurrent
	}
	if p.previousValid && marker.InstanceID == p.previousInstance &&
		marker.LockGeneration == p.previousGeneration {
		return safeWorkdirRecoveryPrevious
	}
	return safeWorkdirRecoveryRejected
}

func safeWorkdirIdentityMarkerFilename(relative string) string {
	digest := sha256.Sum256([]byte(relative))
	return hex.EncodeToString(digest[:]) + ".json"
}

func writeSafeWorkdirIdentityMarker(
	directoryFD int,
	markerName string,
	marker safeWorkdirIdentityMarker,
) error {
	payload, err := json.Marshal(marker)
	if err != nil || len(payload) == 0 || len(payload) > maximumSafeWorkdirIdentityBytes ||
		filepath.Base(markerName) != markerName {
		return ErrSafeWorkdirUnavailable
	}
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return ErrSafeWorkdirUnavailable
	}
	temporary := markerName + ".tmp-" + hex.EncodeToString(nonce)
	fd, err := unix.Openat(
		directoryFD,
		temporary,
		unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0o600,
	)
	if err != nil {
		return ErrSafeWorkdirUnavailable
	}
	cleanupTemporary := true
	defer func() {
		_ = unix.Close(fd)
		if cleanupTemporary {
			_ = unix.Unlinkat(directoryFD, temporary, 0)
		}
	}()
	if unix.Fchmod(fd, 0o600) != nil || writeSafeWorkdirBytes(fd, payload) != nil || unix.Fsync(fd) != nil {
		return ErrSafeWorkdirUnavailable
	}
	if err := unix.Close(fd); err != nil {
		fd = -1
		return ErrSafeWorkdirUnavailable
	}
	fd = -1
	if err := unix.Renameat(directoryFD, temporary, directoryFD, markerName); err != nil {
		return ErrSafeWorkdirUnavailable
	}
	cleanupTemporary = false
	if err := unix.Fsync(directoryFD); err != nil {
		_ = unix.Unlinkat(directoryFD, markerName, 0)
		return ErrSafeWorkdirUnavailable
	}
	return nil
}

func writeSafeWorkdirBytes(fd int, payload []byte) error {
	for len(payload) > 0 {
		written, err := unix.Write(fd, payload)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil || written <= 0 {
			return ErrSafeWorkdirUnavailable
		}
		payload = payload[written:]
	}
	return nil
}

func readSafeWorkdirIdentityMarker(
	directoryFD int,
	markerName string,
) (safeWorkdirIdentityMarker, bool) {
	fd, err := unix.Openat(
		directoryFD,
		markerName,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return safeWorkdirIdentityMarker{}, false
	}
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil || uint32(stat.Mode)&unix.S_IFMT != unix.S_IFREG ||
		uint32(stat.Mode)&0o777 != 0o600 || int(stat.Uid) != os.Geteuid() ||
		stat.Size <= 0 || stat.Size > maximumSafeWorkdirIdentityBytes {
		_ = unix.Close(fd)
		return safeWorkdirIdentityMarker{}, false
	}
	file := os.NewFile(uintptr(fd), "safe-workdir-identity")
	if file == nil {
		_ = unix.Close(fd)
		return safeWorkdirIdentityMarker{}, false
	}
	payload, err := io.ReadAll(io.LimitReader(file, maximumSafeWorkdirIdentityBytes+1))
	_ = file.Close()
	if err != nil || len(payload) == 0 || len(payload) > maximumSafeWorkdirIdentityBytes {
		return safeWorkdirIdentityMarker{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var marker safeWorkdirIdentityMarker
	if decoder.Decode(&marker) != nil {
		return safeWorkdirIdentityMarker{}, false
	}
	var trailing any
	if !errors.Is(decoder.Decode(&trailing), io.EOF) {
		return safeWorkdirIdentityMarker{}, false
	}
	return marker, true
}

func (p *safeWorkdirPlatform) isolateRejectedDirectChild(name string) error {
	if p == nil || p.rejectedFD < 0 || p.leaseFD < 0 {
		return ErrSafeWorkdirCleanupRetryable
	}
	for attempt := 0; attempt < safeWorkdirCreateAttempts; attempt++ {
		token, err := newSafeWorkdirIdentityToken()
		if err != nil {
			return err
		}
		bucketName := "rejected-" + token
		if err := unix.Mkdirat(p.rejectedFD, bucketName, 0o700); err != nil {
			if errors.Is(err, syscall.EEXIST) {
				continue
			}
			return err
		}
		bucketFD, err := safeOpenWorkdirDirectoryAt(p.rejectedFD, bucketName)
		if err != nil {
			_ = unix.Unlinkat(p.rejectedFD, bucketName, unix.AT_REMOVEDIR)
			return err
		}
		if err := unix.Renameat(p.rootFD, name, bucketFD, "source"); err != nil {
			_ = unix.Close(bucketFD)
			_ = unix.Unlinkat(p.rejectedFD, bucketName, unix.AT_REMOVEDIR)
			return err
		}
		markerName := safeWorkdirIdentityMarkerFilename(name)
		if err := unix.Renameat(p.leaseFD, markerName, bucketFD, "marker.json"); err != nil &&
			!errors.Is(err, syscall.ENOENT) {
			_ = unix.Close(bucketFD)
			return ErrSafeWorkdirCleanupRetryable
		}
		_ = unix.Fsync(bucketFD)
		_ = unix.Close(bucketFD)
		_ = unix.Fsync(p.rootFD)
		_ = unix.Fsync(p.rejectedFD)
		return nil
	}
	return ErrSafeWorkdirCleanupRetryable
}

func boundedSafeWorkdirDirectoryNames(
	directoryFD int,
	maxEntries int,
	maxBytes int64,
) ([]string, bool, error) {
	readFD, err := safeOpenWorkdirDirectoryAt(directoryFD, ".")
	if err != nil {
		return nil, false, err
	}
	file := os.NewFile(uintptr(readFD), "safe-workdir-recovery-scan")
	if file == nil {
		_ = unix.Close(readFD)
		return nil, false, ErrSafeWorkdirCleanupRetryable
	}
	defer file.Close()
	names := make([]string, 0, min(maxEntries, safeWorkdirDeleteBatchSize))
	var bytesRead int64
	for len(names) < maxEntries {
		batch, readErr := file.Readdirnames(1)
		if len(batch) == 1 {
			nameBytes := int64(len(batch[0]) + 1)
			if nameBytes > maxBytes-bytesRead {
				return names, true, nil
			}
			names = append(names, batch[0])
			bytesRead += nameBytes
		}
		if errors.Is(readErr, io.EOF) {
			return names, false, nil
		}
		if readErr != nil {
			return nil, false, readErr
		}
	}
	more, err := file.Readdirnames(1)
	if errors.Is(err, io.EOF) && len(more) == 0 {
		return names, false, nil
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	return names, len(more) > 0, nil
}

func (p *safeWorkdirPlatform) delete(
	ctx context.Context,
	relative string,
	limits SafeWorkdirDeleteLimits,
) (returnErr error) {
	if p == nil || !p.hasExclusiveRootLock() || p.leaseFD < 0 {
		return ErrSafeWorkdirCleanupRetryable
	}
	defer func() {
		if returnErr != nil {
			return
		}
		err := unix.Unlinkat(p.leaseFD, safeWorkdirIdentityMarkerFilename(relative), 0)
		if err != nil && !errors.Is(err, syscall.ENOENT) {
			returnErr = ErrSafeWorkdirCleanupRetryable
		}
	}()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	components, ok := safeWorkdirRelativeComponents(relative, false)
	if !ok || len(components) == 0 {
		return ErrSafeWorkdirInvalid
	}
	trashFD, err := safeOpenWorkdirDirectoryAt(p.rootFD, safeWorkdirTrashDirectory)
	if err != nil {
		return err
	}
	defer unix.Close(trashFD)

	bucketName := safeWorkdirTrashBucketName(relative)
	legacyName := safeWorkdirTrashName(relative)
	budget := &safeWorkdirDeleteBudget{limits: limits}
	for round := 0; round < safeWorkdirDeleteMaxRounds; round++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		sourcePresent, err := p.sourcePresent(components)
		if err != nil {
			return err
		}
		bucketPresent, err := safeWorkdirEntryPresentAt(trashFD, bucketName)
		if err != nil {
			return err
		}
		legacyPresent, err := safeWorkdirEntryPresentAt(trashFD, legacyName)
		if err != nil {
			return err
		}
		if !sourcePresent && !bucketPresent && !legacyPresent {
			return nil
		}

		bucketFD, err := p.openOrCreateDeleteBucket(trashFD, bucketName)
		if err != nil {
			return err
		}
		if legacyPresent {
			if err := moveSafeWorkdirEntryIntoBucket(trashFD, legacyName, bucketFD, "legacy-"); err != nil &&
				!errors.Is(err, syscall.ENOENT) {
				_ = unix.Close(bucketFD)
				return err
			}
		}
		if sourcePresent {
			if err := p.quarantineSource(components, bucketFD); err != nil &&
				!errors.Is(err, syscall.ENOENT) && !errors.Is(err, errSafeWorkdirParentMissing) {
				_ = unix.Close(bucketFD)
				return err
			}
		}
		if err := removeSafeWorkdirDirectoryContents(ctx, bucketFD, bucketFD, -1, budget); err != nil {
			_ = unix.Close(bucketFD)
			return err
		}
		_ = unix.Close(bucketFD)
		if err := unix.Unlinkat(trashFD, bucketName, unix.AT_REMOVEDIR); err != nil {
			if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
				return ErrSafeWorkdirCleanupRetryable
			}
			if !errors.Is(err, syscall.ENOENT) {
				return err
			}
		}
		if p.afterQuarantineRemoved != nil {
			p.afterQuarantineRemoved()
		}

		sourcePresent, err = p.sourcePresent(components)
		if err != nil {
			return err
		}
		if !sourcePresent {
			bucketPresent, err = safeWorkdirEntryPresentAt(trashFD, bucketName)
			if err != nil {
				return err
			}
			legacyPresent, err = safeWorkdirEntryPresentAt(trashFD, legacyName)
			if err != nil {
				return err
			}
			if !bucketPresent && !legacyPresent {
				return nil
			}
			continue
		}

		bucketFD, err = p.openOrCreateDeleteBucket(trashFD, bucketName)
		if err != nil {
			return err
		}
		err = p.quarantineSource(components, bucketFD)
		_ = unix.Close(bucketFD)
		if err != nil && !errors.Is(err, syscall.ENOENT) && !errors.Is(err, errSafeWorkdirParentMissing) {
			return err
		}
	}
	return ErrSafeWorkdirCleanupRetryable
}

func (p *safeWorkdirPlatform) cleanupQuarantine(
	ctx context.Context,
	limits SafeWorkdirDeleteLimits,
) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	trashFD, err := safeOpenWorkdirDirectoryAt(p.rootFD, safeWorkdirTrashDirectory)
	if err != nil {
		return err
	}
	defer unix.Close(trashFD)
	budget := &safeWorkdirDeleteBudget{limits: limits}
	if err := removeSafeWorkdirQuarantineContents(ctx, trashFD, budget); err != nil {
		return err
	}
	return nil
}

func removeSafeWorkdirQuarantineContents(
	ctx context.Context,
	trashFD int,
	budget *safeWorkdirDeleteBudget,
) error {
	names, err := safeWorkdirDirectorySnapshot(trashFD, budget)
	if err != nil {
		return err
	}
	for _, name := range names {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(trashFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if errors.Is(err, syscall.ENOENT) {
				continue
			}
			return err
		}
		if uint32(stat.Mode)&unix.S_IFMT == unix.S_IFDIR {
			entryFD, err := safeOpenWorkdirDirectoryAt(trashFD, name)
			if err != nil {
				return err
			}
			before := safeWorkdirIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}
			after, ok := safeWorkdirIdentityFD(entryFD)
			if !ok || before != after {
				_ = unix.Close(entryFD)
				return ErrSafeWorkdirCleanupRetryable
			}
			err = removeSafeWorkdirDirectoryContents(ctx, entryFD, entryFD, -1, budget)
			_ = unix.Close(entryFD)
			if err != nil {
				return err
			}
			if !budget.consume(name) {
				return ErrSafeWorkdirCleanupRetryable
			}
			if err := unix.Unlinkat(trashFD, name, unix.AT_REMOVEDIR); err != nil &&
				!errors.Is(err, syscall.ENOENT) {
				return err
			}
			continue
		}
		if !budget.consume(name) {
			return ErrSafeWorkdirCleanupRetryable
		}
		if err := unix.Unlinkat(trashFD, name, 0); err != nil && !errors.Is(err, syscall.ENOENT) {
			return err
		}
	}
	empty, err := safeWorkdirDirectoryEmpty(trashFD)
	if err != nil {
		return err
	}
	if !empty {
		return ErrSafeWorkdirCleanupRetryable
	}
	return nil
}

func (p *safeWorkdirPlatform) openOrCreateDeleteBucket(trashFD int, name string) (int, error) {
	if err := p.ensureDirectoryAt(trashFD, name); err != nil {
		return -1, err
	}
	return safeOpenWorkdirDirectoryAt(trashFD, name)
}

func (p *safeWorkdirPlatform) quarantineSource(components []string, bucketFD int) error {
	parentFD, err := p.openExistingParent(components[:len(components)-1])
	if err != nil {
		return err
	}
	defer unix.Close(parentFD)
	leaf := components[len(components)-1]
	leafFD, err := safeOpenWorkdirDirectoryAt(parentFD, leaf)
	if err != nil {
		return err
	}
	identity, ok := safeWorkdirIdentityFD(leafFD)
	_ = unix.Close(leafFD)
	if !ok {
		return ErrSafeWorkdirCleanupRetryable
	}
	targetName := "source-" + safeWorkdirIdentityToken(identity)
	if err := unix.Renameat(parentFD, leaf, bucketFD, targetName); err != nil {
		return err
	}
	quarantinedFD, err := safeOpenWorkdirDirectoryAt(bucketFD, targetName)
	if err != nil {
		return ErrSafeWorkdirCleanupRetryable
	}
	current, currentOK := safeWorkdirIdentityFD(quarantinedFD)
	_ = unix.Close(quarantinedFD)
	if !currentOK || current != identity {
		return ErrSafeWorkdirCleanupRetryable
	}
	return nil
}

func (p *safeWorkdirPlatform) sourcePresent(components []string) (bool, error) {
	parentFD, err := p.openExistingParent(components[:len(components)-1])
	if errors.Is(err, errSafeWorkdirParentMissing) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer unix.Close(parentFD)
	return safeWorkdirEntryPresentAt(parentFD, components[len(components)-1])
}

func moveSafeWorkdirEntryIntoBucket(
	sourceParentFD int,
	sourceName string,
	bucketFD int,
	targetPrefix string,
) error {
	identity, ok, err := safeWorkdirIdentityAt(sourceParentFD, sourceName)
	if err != nil {
		return err
	}
	if !ok {
		return syscall.ENOENT
	}
	targetName := targetPrefix + safeWorkdirIdentityToken(identity)
	if err := unix.Renameat(sourceParentFD, sourceName, bucketFD, targetName); err != nil {
		return err
	}
	current, currentOK, err := safeWorkdirIdentityAt(bucketFD, targetName)
	if err != nil || !currentOK || current != identity {
		return ErrSafeWorkdirCleanupRetryable
	}
	return nil
}

func removeSafeWorkdirDirectoryContents(
	ctx context.Context,
	directoryFD int,
	spillFD int,
	currentDepth int,
	budget *safeWorkdirDeleteBudget,
) error {
	names, err := safeWorkdirDirectorySnapshot(directoryFD, budget)
	if err != nil {
		return err
	}
	for _, name := range names {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(directoryFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			if errors.Is(err, syscall.ENOENT) {
				continue
			}
			return err
		}
		identity := safeWorkdirIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}
		entryDepth := currentDepth + 1
		if uint32(stat.Mode)&unix.S_IFMT == unix.S_IFDIR && entryDepth > budget.limits.MaxDepth {
			spillName := "spill-" + safeWorkdirIdentityToken(identity)
			if !budget.consume(name, spillName) {
				return ErrSafeWorkdirCleanupRetryable
			}
			if err := unix.Renameat(directoryFD, name, spillFD, spillName); err != nil {
				return err
			}
			current, ok, err := safeWorkdirIdentityAt(spillFD, spillName)
			if err != nil || !ok || current != identity {
				return ErrSafeWorkdirCleanupRetryable
			}
			continue
		}
		if uint32(stat.Mode)&unix.S_IFMT == unix.S_IFDIR {
			childFD, err := safeOpenWorkdirDirectoryAt(directoryFD, name)
			if err != nil {
				return err
			}
			current, ok := safeWorkdirIdentityFD(childFD)
			if !ok || current != identity {
				_ = unix.Close(childFD)
				return ErrSafeWorkdirCleanupRetryable
			}
			err = removeSafeWorkdirDirectoryContents(ctx, childFD, spillFD, entryDepth, budget)
			_ = unix.Close(childFD)
			if err != nil {
				return err
			}
			if !budget.consume(name) {
				return ErrSafeWorkdirCleanupRetryable
			}
			if err := unix.Unlinkat(directoryFD, name, unix.AT_REMOVEDIR); err != nil &&
				!errors.Is(err, syscall.ENOENT) {
				return err
			}
			continue
		}
		if !budget.consume(name) {
			return ErrSafeWorkdirCleanupRetryable
		}
		if err := unix.Unlinkat(directoryFD, name, 0); err != nil && !errors.Is(err, syscall.ENOENT) {
			return err
		}
	}
	empty, err := safeWorkdirDirectoryEmpty(directoryFD)
	if err != nil {
		return err
	}
	if !empty {
		return ErrSafeWorkdirCleanupRetryable
	}
	return nil
}

func safeWorkdirDirectorySnapshot(
	directoryFD int,
	budget *safeWorkdirDeleteBudget,
) ([]string, error) {
	if budget == nil || budget.entries >= budget.limits.MaxEntries ||
		budget.bytes >= budget.limits.MaxBytes {
		return nil, ErrSafeWorkdirCleanupRetryable
	}
	readFD, err := safeOpenWorkdirDirectoryAt(directoryFD, ".")
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(readFD), "safe-workdir-snapshot")
	if file == nil {
		_ = unix.Close(readFD)
		return nil, ErrSafeWorkdirCleanupRetryable
	}
	defer file.Close()
	maxNames := safeWorkdirDeleteBatchSize
	if remaining := budget.limits.MaxEntries - budget.entries; remaining < maxNames {
		maxNames = remaining
	}
	remainingBytes := budget.limits.MaxBytes - budget.bytes
	names := make([]string, 0, maxNames)
	var listedBytes int64
	for len(names) < maxNames {
		batch, readErr := file.Readdirnames(1)
		if len(batch) == 1 {
			nameBytes := int64(len(batch[0]) + 1)
			if nameBytes > remainingBytes-listedBytes {
				break
			}
			names = append(names, batch[0])
			listedBytes += nameBytes
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return names, nil
}

func safeWorkdirDirectoryEmpty(directoryFD int) (bool, error) {
	readFD, err := safeOpenWorkdirDirectoryAt(directoryFD, ".")
	if err != nil {
		return false, err
	}
	file := os.NewFile(uintptr(readFD), "safe-workdir-empty")
	if file == nil {
		_ = unix.Close(readFD)
		return false, ErrSafeWorkdirCleanupRetryable
	}
	defer file.Close()
	names, err := file.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return len(names) == 0, nil
	}
	if err != nil {
		return false, err
	}
	return len(names) == 0, nil
}

func (b *safeWorkdirDeleteBudget) consume(names ...string) bool {
	if b == nil || len(names) == 0 || b.entries >= b.limits.MaxEntries {
		return false
	}
	var bytes int64
	for _, name := range names {
		bytes += int64(len(name) + 1)
	}
	if bytes > b.limits.MaxBytes-b.bytes {
		return false
	}
	b.entries++
	b.bytes += bytes
	return true
}

func safeWorkdirEntryPresentAt(parentFD int, name string) (bool, error) {
	_, ok, err := safeWorkdirIdentityAt(parentFD, name)
	return ok, err
}

func safeWorkdirIdentityAt(parentFD int, name string) (safeWorkdirIdentity, bool, error) {
	var stat unix.Stat_t
	if err := unix.Fstatat(parentFD, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		if errors.Is(err, syscall.ENOENT) {
			return safeWorkdirIdentity{}, false, nil
		}
		return safeWorkdirIdentity{}, false, err
	}
	return safeWorkdirIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}, true, nil
}

func safeWorkdirIdentityToken(identity safeWorkdirIdentity) string {
	return strconv.FormatUint(identity.device, 16) + "-" + strconv.FormatUint(identity.inode, 16)
}

func (p *safeWorkdirPlatform) openOrCreateParent(
	ctx context.Context,
	components []string,
) (int, error) {
	currentFD, err := safeOpenWorkdirDirectoryAt(p.rootFD, ".")
	if err != nil {
		return -1, err
	}
	for _, component := range components {
		if ctx.Err() != nil {
			_ = unix.Close(currentFD)
			return -1, ctx.Err()
		}
		if err := p.ensureDirectoryAt(currentFD, component); err != nil {
			_ = unix.Close(currentFD)
			return -1, err
		}
		nextFD, err := safeOpenWorkdirDirectoryAt(currentFD, component)
		_ = unix.Close(currentFD)
		if err != nil || !safeWorkdirOwnedDirectoryFD(nextFD) {
			if nextFD >= 0 {
				_ = unix.Close(nextFD)
			}
			return -1, ErrSafeWorkdirUnavailable
		}
		currentFD = nextFD
	}
	return currentFD, nil
}

func (p *safeWorkdirPlatform) openExistingParent(components []string) (int, error) {
	currentFD, err := safeOpenWorkdirDirectoryAt(p.rootFD, ".")
	if err != nil {
		return -1, err
	}
	for _, component := range components {
		nextFD, err := safeOpenWorkdirDirectoryAt(currentFD, component)
		_ = unix.Close(currentFD)
		if errors.Is(err, syscall.ENOENT) {
			return -1, errSafeWorkdirParentMissing
		}
		if err != nil || !safeWorkdirOwnedDirectoryFD(nextFD) {
			if nextFD >= 0 {
				_ = unix.Close(nextFD)
			}
			return -1, ErrSafeWorkdirCleanupRetryable
		}
		currentFD = nextFD
	}
	return currentFD, nil
}

func (p *safeWorkdirPlatform) ensureDirectoryAt(parentFD int, name string) error {
	if err := unix.Mkdirat(parentFD, name, 0o700); err != nil && !errors.Is(err, syscall.EEXIST) {
		return err
	}
	fd, err := safeOpenWorkdirDirectoryAt(parentFD, name)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	if !safeWorkdirOwnedDirectoryFD(fd) {
		return ErrSafeWorkdirInvalid
	}
	return nil
}

func openSafeWorkdirAbsoluteDirectory(path string) (int, error) {
	currentFD, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	components := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	for _, component := range components {
		if component == "" {
			continue
		}
		nextFD, err := safeOpenWorkdirDirectoryAt(currentFD, component)
		_ = unix.Close(currentFD)
		if err != nil {
			return -1, err
		}
		currentFD = nextFD
	}
	return currentFD, nil
}

func safeWorkdirOwnedDirectoryFD(fd int) bool {
	if fd < 0 {
		return false
	}
	var stat unix.Stat_t
	if unix.Fstat(fd, &stat) != nil {
		return false
	}
	return uint32(stat.Mode)&unix.S_IFMT == unix.S_IFDIR &&
		uint32(stat.Mode)&0o777 == 0o700 && int(stat.Uid) == os.Geteuid()
}

func safeWorkdirIdentityFD(fd int) (safeWorkdirIdentity, bool) {
	var stat unix.Stat_t
	if fd < 0 || unix.Fstat(fd, &stat) != nil {
		return safeWorkdirIdentity{}, false
	}
	return safeWorkdirIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}, true
}

func safeWorkdirTrashName(relative string) string {
	digest := sha256.Sum256([]byte(relative))
	return "delete-" + hex.EncodeToString(digest[:16])
}

func safeWorkdirTrashBucketName(relative string) string {
	return safeWorkdirTrashName(relative) + "-bucket"
}
