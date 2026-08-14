// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	hostShellRuntimeGeneration = uint64(1)
	hostShellMaximumSessions   = 1000
	hostShellGatePollInterval  = 20 * time.Millisecond
)

// HostShellSessionManagerOptions are trusted, process-local debug settings.
// RootDir and SkillsDir are lexical routing roots, not an isolation boundary.
type HostShellSessionManagerOptions struct {
	RootDir                 string
	SkillsDir               string
	ExpectedProviderID      int64
	AllowedEnvironmentNames []string
	Gate                    func() bool
	CancelGrace             time.Duration
	MaxSessions             int
	Now                     func() time.Time
	Random                  func() (string, error)
}

// HostShellSessionManager is deliberately bounded and process-local. It does
// not provide chroot, symlink, network, credential, or hostile-command
// isolation and must only be installed behind the local debug gate.
type HostShellSessionManager struct {
	mu                 sync.Mutex
	rootDir            string
	skillsDir          string
	expectedProviderID int64
	allowedEnvironment []string
	gate               func() bool
	cancelGrace        time.Duration
	maxSessions        int
	now                func() time.Time
	random             func() (string, error)
	sessions           map[string]*hostShellSession
	identities         map[domainsandbox.SessionKey]*hostShellSession
	shutdown           bool
}

type hostShellSession struct {
	manager    *HostShellSessionManager
	ref        domainsandbox.SessionRef
	threadRoot string
	execMu     sync.Mutex
	processMu  sync.Mutex
	process    *hostShellProcess
}

type hostShellProcess struct {
	pid           int
	done          chan struct{}
	terminateOnce sync.Once
	reasonMu      sync.Mutex
	reason        error
}

type hostShellBoundedOutput struct {
	mu       sync.Mutex
	maximum  int64
	total    int64
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	overflow chan struct{}
	once     sync.Once
}

type hostShellOutputWriter struct {
	output *hostShellBoundedOutput
	stderr bool
}

func NewHostShellSessionManager(options HostShellSessionManagerOptions) (*HostShellSessionManager, error) {
	if !validHostShellRoot(options.RootDir) || !validHostShellRoot(options.SkillsDir) ||
		options.ExpectedProviderID <= 0 || options.Gate == nil || options.CancelGrace <= 0 ||
		options.CancelGrace > 5*time.Minute || options.MaxSessions <= 0 || options.MaxSessions > hostShellMaximumSessions {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	allowed, err := normalizeHostShellEnvironmentNames(options.AllowedEnvironmentNames)
	if err != nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	if err := os.MkdirAll(options.RootDir, 0o700); err != nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	if err := os.MkdirAll(options.SkillsDir, 0o700); err != nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	rootInfo, rootErr := os.Stat(options.RootDir)
	skillsInfo, skillsErr := os.Stat(options.SkillsDir)
	if rootErr != nil || skillsErr != nil || !rootInfo.IsDir() || !skillsInfo.IsDir() {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	canonicalRoot, rootErr := filepath.EvalSymlinks(options.RootDir)
	canonicalSkills, skillsErr := filepath.EvalSymlinks(options.SkillsDir)
	if rootErr != nil || skillsErr != nil || !validHostShellRoot(canonicalRoot) || !validHostShellRoot(canonicalSkills) {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	randomID := options.Random
	if randomID == nil {
		randomID = newHostShellRandomID
	}
	return &HostShellSessionManager{
		rootDir: canonicalRoot, skillsDir: canonicalSkills, expectedProviderID: options.ExpectedProviderID,
		allowedEnvironment: allowed, gate: options.Gate, cancelGrace: options.CancelGrace,
		maxSessions: options.MaxSessions, now: now, random: randomID,
		sessions: make(map[string]*hostShellSession), identities: make(map[domainsandbox.SessionKey]*hostShellSession),
	}, nil
}

func validHostShellRoot(value string) bool {
	return value != "" && len(value) <= 4096 && filepath.IsAbs(value) && filepath.Clean(value) == value &&
		!strings.ContainsRune(value, 0)
}

func newHostShellRandomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func normalizeHostShellEnvironmentNames(names []string) ([]string, error) {
	if len(names) > MaxEnvVars {
		return nil, domainsandbox.ErrInvalidInput
	}
	result := append([]string(nil), names...)
	sort.Strings(result)
	for index, name := range result {
		if !validSessionEnvironmentKey(name) || index > 0 && result[index-1] == name {
			return nil, domainsandbox.ErrInvalidInput
		}
	}
	return result, nil
}

// UpdateAllowedEnvironmentNames atomically replaces the trusted allowlist.
// Existing requests are revalidated when they start; values are never stored.
func (manager *HostShellSessionManager) UpdateAllowedEnvironmentNames(names []string) error {
	if manager == nil {
		return domainsandbox.ErrInvalidInput
	}
	allowed, err := normalizeHostShellEnvironmentNames(names)
	if err != nil {
		return err
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.shutdown {
		return domainsandbox.ErrUnavailable
	}
	manager.allowedEnvironment = allowed
	return nil
}

func (manager *HostShellSessionManager) Acquire(ctx context.Context, input AcquireSessionRequest) (SandboxSession, error) {
	if manager == nil || ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	request, err := NormalizeAcquireSessionRequest(input)
	if err != nil || request.Key.ProviderID != manager.expectedProviderID {
		return nil, domainsandbox.ErrInvalidInput
	}
	if !manager.gate() {
		return nil, domainsandbox.ErrUnavailable
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.shutdown || !manager.gate() {
		return nil, domainsandbox.ErrUnavailable
	}
	if existing := manager.identities[request.Key]; existing != nil {
		return existing, nil
	}
	if len(manager.sessions) >= manager.maxSessions {
		return nil, domainsandbox.ErrCapacityExhausted
	}
	sessionID, err := manager.nextSessionIDLocked()
	if err != nil {
		return nil, err
	}
	threadRoot := filepath.Join(manager.rootDir, fmt.Sprint(request.Key.SpaceID), fmt.Sprint(request.Key.UserID), request.Key.ThreadID)
	for _, name := range []string{"workspace", "uploads", "outputs"} {
		if err := os.MkdirAll(filepath.Join(threadRoot, name), 0o700); err != nil {
			return nil, domainsandbox.ErrUnavailable
		}
	}
	ref := domainsandbox.SessionRef{SessionID: sessionID, Key: request.Key, RuntimeGeneration: hostShellRuntimeGeneration}
	session := &hostShellSession{manager: manager, ref: ref, threadRoot: threadRoot}
	manager.sessions[sessionID] = session
	manager.identities[request.Key] = session
	return session, nil
}

func (manager *HostShellSessionManager) nextSessionIDLocked() (string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		randomValue, err := manager.random()
		candidate := "host-" + randomValue
		if err != nil || !validSessionIdentifier(candidate) {
			return "", domainsandbox.ErrUnavailable
		}
		if manager.sessions[candidate] == nil {
			return candidate, nil
		}
	}
	return "", domainsandbox.ErrCapacityExhausted
}

func (manager *HostShellSessionManager) Get(ctx context.Context, ref domainsandbox.SessionRef) (SandboxSession, error) {
	return manager.lookup(ctx, ref)
}

func (manager *HostShellSessionManager) Recover(ctx context.Context, ref domainsandbox.SessionRef) (SandboxSession, error) {
	return manager.lookup(ctx, ref)
}

func (manager *HostShellSessionManager) lookup(ctx context.Context, ref domainsandbox.SessionRef) (SandboxSession, error) {
	if manager == nil || ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalized != ref || ref.Key.ProviderID != manager.expectedProviderID {
		return nil, domainsandbox.ErrInvalidInput
	}
	if !manager.gate() {
		return nil, domainsandbox.ErrUnavailable
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.shutdown || !manager.gate() {
		return nil, domainsandbox.ErrUnavailable
	}
	session := manager.sessions[ref.SessionID]
	if session == nil || session.ref != ref {
		return nil, domainsandbox.ErrSessionNotFound
	}
	return session, nil
}

func (manager *HostShellSessionManager) Release(ctx context.Context, ref domainsandbox.SessionRef) error {
	return manager.remove(ctx, ref)
}

func (manager *HostShellSessionManager) Destroy(ctx context.Context, ref domainsandbox.SessionRef) error {
	return manager.remove(ctx, ref)
}

func (manager *HostShellSessionManager) remove(ctx context.Context, ref domainsandbox.SessionRef) error {
	if manager == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	normalized, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalized != ref || ref.Key.ProviderID != manager.expectedProviderID {
		return domainsandbox.ErrInvalidInput
	}
	manager.mu.Lock()
	session := manager.sessions[ref.SessionID]
	if session == nil || session.ref != ref {
		manager.mu.Unlock()
		return domainsandbox.ErrSessionNotFound
	}
	delete(manager.sessions, ref.SessionID)
	delete(manager.identities, ref.Key)
	manager.mu.Unlock()
	return session.stop(ctx, domainsandbox.ErrUnavailable)
}

// Shutdown explicitly terminates all active process groups. The manager does
// not implement CloseContext because provider selection may discard it while
// retained sessions are still in use.
func (manager *HostShellSessionManager) Shutdown(ctx context.Context) error {
	if manager == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	manager.mu.Lock()
	if manager.shutdown {
		manager.mu.Unlock()
		return nil
	}
	manager.shutdown = true
	sessions := make([]*hostShellSession, 0, len(manager.sessions))
	for _, session := range manager.sessions {
		sessions = append(sessions, session)
	}
	manager.sessions = make(map[string]*hostShellSession)
	manager.identities = make(map[domainsandbox.SessionKey]*hostShellSession)
	manager.mu.Unlock()
	results := make(chan error, len(sessions))
	for _, session := range sessions {
		go func(session *hostShellSession) {
			results <- session.stop(ctx, domainsandbox.ErrUnavailable)
		}(session)
	}
	errorsFound := make([]error, 0, len(sessions))
	for range sessions {
		if err := <-results; err != nil {
			errorsFound = append(errorsFound, err)
		}
	}
	return errors.Join(errorsFound...)
}

func (manager *HostShellSessionManager) operationState(session *hostShellSession) ([]string, error) {
	if manager == nil || session == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	if !manager.gate() {
		return nil, domainsandbox.ErrUnavailable
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.shutdown || !manager.gate() {
		return nil, domainsandbox.ErrUnavailable
	}
	if manager.sessions[session.ref.SessionID] != session {
		return nil, domainsandbox.ErrSessionNotFound
	}
	return append([]string(nil), manager.allowedEnvironment...), nil
}

func (session *hostShellSession) Ref() domainsandbox.SessionRef {
	if session == nil {
		return domainsandbox.SessionRef{}
	}
	return session.ref
}

func (session *hostShellSession) Exec(ctx context.Context, input ExecRequest) (ExecutionStream, error) {
	if session == nil || session.manager == nil || ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	allowed, err := session.manager.operationState(session)
	if err != nil {
		return nil, err
	}
	request, err := NormalizeExecRequest(input, session.manager.now(), allowed)
	if err != nil {
		return nil, err
	}
	physicalCWD, err := session.resolveLogicalPath(request.CWD, false)
	if err != nil {
		return nil, err
	}

	session.execMu.Lock()
	defer session.execMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	allowed, err = session.manager.operationState(session)
	if err != nil {
		return nil, err
	}
	request, err = NormalizeExecRequest(input, session.manager.now(), allowed)
	if err != nil {
		return nil, err
	}

	var command *exec.Cmd
	if len(request.Argv) != 0 {
		command = exec.Command(request.Argv[0], request.Argv[1:]...)
	} else {
		command = exec.Command("/bin/sh", "-c", request.Command)
	}
	command.Dir = physicalCWD
	command.Env = hostShellEnvironment(request.Env)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	output := &hostShellBoundedOutput{maximum: request.MaxOutputBytes, overflow: make(chan struct{})}
	command.Stdout = hostShellOutputWriter{output: output}
	command.Stderr = hostShellOutputWriter{output: output, stderr: true}
	if err := command.Start(); err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	process := &hostShellProcess{pid: command.Process.Pid, done: make(chan struct{})}
	session.processMu.Lock()
	session.process = process
	session.processMu.Unlock()
	waitResult := make(chan error, 1)
	go func() {
		waitResult <- command.Wait()
		close(process.done)
	}()
	defer func() {
		session.processMu.Lock()
		if session.process == process {
			session.process = nil
		}
		session.processMu.Unlock()
	}()

	duration := request.Deadline.Sub(session.manager.now())
	if duration <= 0 {
		_ = session.terminateProcess(context.Background(), process, context.DeadlineExceeded)
		return nil, context.DeadlineExceeded
	}
	deadline := time.NewTimer(duration)
	defer deadline.Stop()
	poll := time.NewTicker(hostShellGatePollInterval)
	defer poll.Stop()
	for {
		select {
		case waitErr := <-waitResult:
			if reason := process.terminationReason(); reason != nil {
				return nil, reason
			}
			exitCode, ok := hostShellExitCode(waitErr)
			if !ok {
				return nil, domainsandbox.ErrUnavailable
			}
			stdout, stderr := output.snapshot()
			stdout = session.redactPhysicalPaths(stdout)
			stderr = session.redactPhysicalPaths(stderr)
			return newRemoteExecutionStream(remoteExecutionResult{ExitCode: exitCode, Stdout: stdout, Stderr: stderr}), nil
		case <-ctx.Done():
			_ = session.terminateProcess(context.WithoutCancel(ctx), process, ctx.Err())
			return nil, ctx.Err()
		case <-deadline.C:
			_ = session.terminateProcess(context.Background(), process, context.DeadlineExceeded)
			return nil, context.DeadlineExceeded
		case <-output.overflow:
			_ = session.terminateProcess(context.Background(), process, domainsandbox.ErrCapacityExhausted)
			return nil, domainsandbox.ErrCapacityExhausted
		case <-poll.C:
			if _, stateErr := session.manager.operationState(session); stateErr != nil {
				_ = session.terminateProcess(context.Background(), process, domainsandbox.ErrUnavailable)
				return nil, domainsandbox.ErrUnavailable
			}
		}
	}
}

func hostShellEnvironment(environment map[string]string) []string {
	result := []string{"PATH=/usr/bin:/bin"}
	names := make([]string, 0, len(environment))
	for name := range environment {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, name+"="+environment[name])
	}
	return result
}

func hostShellExitCode(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() >= 0 {
		return exitError.ExitCode(), true
	}
	return 0, false
}

func (writer hostShellOutputWriter) Write(value []byte) (int, error) {
	if writer.output == nil {
		return len(value), nil
	}
	writer.output.mu.Lock()
	defer writer.output.mu.Unlock()
	remaining := writer.output.maximum - writer.output.total
	if remaining > 0 {
		count := int64(len(value))
		if count > remaining {
			count = remaining
		}
		if writer.stderr {
			_, _ = writer.output.stderr.Write(value[:count])
		} else {
			_, _ = writer.output.stdout.Write(value[:count])
		}
		writer.output.total += count
	}
	if int64(len(value)) > remaining {
		writer.output.once.Do(func() { close(writer.output.overflow) })
	}
	return len(value), nil
}

func (output *hostShellBoundedOutput) snapshot() ([]byte, []byte) {
	output.mu.Lock()
	defer output.mu.Unlock()
	return append([]byte(nil), output.stdout.Bytes()...), append([]byte(nil), output.stderr.Bytes()...)
}

func (process *hostShellProcess) terminationReason() error {
	process.reasonMu.Lock()
	defer process.reasonMu.Unlock()
	return process.reason
}

func (process *hostShellProcess) setTerminationReason(reason error) {
	process.reasonMu.Lock()
	if process.reason == nil {
		process.reason = reason
	}
	process.reasonMu.Unlock()
}

func (session *hostShellSession) stop(ctx context.Context, reason error) error {
	session.processMu.Lock()
	process := session.process
	session.processMu.Unlock()
	if process == nil {
		return nil
	}
	return session.terminateProcess(ctx, process, reason)
}

func (session *hostShellSession) terminateProcess(ctx context.Context, process *hostShellProcess, reason error) error {
	if process == nil {
		return nil
	}
	process.terminateOnce.Do(func() {
		process.setTerminationReason(reason)
		_ = signalHostShellProcessGroup(process.pid, syscall.SIGTERM)
	})
	timer := time.NewTimer(session.manager.cancelGrace)
	defer timer.Stop()
	select {
	case <-process.done:
		return nil
	case <-ctx.Done():
		_ = signalHostShellProcessGroup(process.pid, syscall.SIGKILL)
		return ctx.Err()
	case <-timer.C:
		_ = signalHostShellProcessGroup(process.pid, syscall.SIGKILL)
	}
	select {
	case <-process.done:
		return nil
	case <-ctx.Done():
		_ = signalHostShellProcessGroup(process.pid, syscall.SIGKILL)
		return ctx.Err()
	}
}

func signalHostShellProcessGroup(pid int, signal syscall.Signal) error {
	if pid <= 0 {
		return domainsandbox.ErrUnavailable
	}
	err := syscall.Kill(-pid, signal)
	if err == nil || errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return domainsandbox.ErrUnavailable
}

func (session *hostShellSession) Read(ctx context.Context, input ReadRequest) (FileContent, error) {
	request, err := NormalizeReadRequest(input)
	if err != nil {
		return FileContent{}, err
	}
	if err := session.checkContext(ctx); err != nil {
		return FileContent{}, err
	}
	physical, err := session.resolveLogicalPath(request.Path, false)
	if err != nil {
		return FileContent{}, err
	}
	data, err := readHostShellFile(physical, request.MaxBytes)
	if err != nil {
		return FileContent{}, err
	}
	if err := session.checkContext(ctx); err != nil {
		return FileContent{}, err
	}
	return FileContent{Data: data}, nil
}

func (session *hostShellSession) Write(ctx context.Context, input WriteRequest) error {
	request, err := NormalizeWriteRequest(input)
	if err != nil {
		return err
	}
	if err := session.checkContext(ctx); err != nil {
		return err
	}
	physical, err := session.resolveLogicalPath(request.Path, true)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(physical), 0o700); err != nil {
		return mapHostShellFilesystemError(err)
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if request.Append {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	file, err := os.OpenFile(physical, flags, 0o600)
	if err != nil {
		return mapHostShellFilesystemError(err)
	}
	_, writeErr := file.Write(request.Content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return domainsandbox.ErrUnavailable
	}
	return session.checkContext(ctx)
}

func (session *hostShellSession) List(ctx context.Context, input ListRequest) ([]FileEntry, error) {
	request, err := NormalizeListRequest(input)
	if err != nil {
		return nil, err
	}
	if err := session.checkContext(ctx); err != nil {
		return nil, err
	}
	physical, err := session.resolveLogicalPath(request.Path, false)
	if err != nil {
		return nil, err
	}
	children, err := os.ReadDir(physical)
	if err != nil {
		return nil, mapHostShellFilesystemError(err)
	}
	if len(children) > request.Limit {
		children = children[:request.Limit]
	}
	entries := make([]FileEntry, 0, len(children))
	for _, child := range children {
		if err := session.checkContext(ctx); err != nil {
			return nil, err
		}
		info, err := child.Info()
		if err != nil {
			return nil, mapHostShellFilesystemError(err)
		}
		entries = append(entries, FileEntry{
			Path: path.Join(request.Path, child.Name()), Directory: child.IsDir(),
			Size: info.Size(), Modified: info.ModTime().UTC(),
		})
	}
	return entries, nil
}

func (session *hostShellSession) Glob(ctx context.Context, input GlobRequest) ([]FileEntry, error) {
	request, err := NormalizeGlobRequest(input)
	if err != nil {
		return nil, err
	}
	if err := session.checkContext(ctx); err != nil {
		return nil, err
	}
	matcher, err := compileHostShellGlob(request.Pattern)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	physical, err := session.resolveLogicalPath(request.Path, false)
	if err != nil {
		return nil, err
	}
	entries := make([]FileEntry, 0, request.Limit)
	err = filepath.WalkDir(physical, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return mapHostShellFilesystemError(walkErr)
		}
		if err := session.checkContext(ctx); err != nil {
			return err
		}
		if current == physical {
			return nil
		}
		relative, relErr := filepath.Rel(physical, current)
		if relErr != nil {
			return domainsandbox.ErrUnavailable
		}
		if !matcher.MatchString(filepath.ToSlash(relative)) {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return mapHostShellFilesystemError(infoErr)
		}
		entries = append(entries, FileEntry{
			Path: path.Join(request.Path, filepath.ToSlash(relative)), Directory: entry.IsDir(),
			Size: info.Size(), Modified: info.ModTime().UTC(),
		})
		if len(entries) >= request.Limit {
			return errHostShellLimitReached
		}
		return nil
	})
	if err != nil && !errors.Is(err, errHostShellLimitReached) {
		return nil, err
	}
	return entries, nil
}

func compileHostShellGlob(patternValue string) (*regexp.Regexp, error) {
	var expression strings.Builder
	expression.WriteByte('^')
	for index := 0; index < len(patternValue); {
		switch patternValue[index] {
		case '*':
			if index+1 < len(patternValue) && patternValue[index+1] == '*' {
				index += 2
				if index < len(patternValue) && patternValue[index] == '/' {
					expression.WriteString("(?:.*/)?")
					index++
				} else {
					expression.WriteString(".*")
				}
			} else {
				expression.WriteString("[^/]*")
				index++
			}
		case '?':
			expression.WriteString("[^/]")
			index++
		default:
			expression.WriteString(regexp.QuoteMeta(string(patternValue[index])))
			index++
		}
	}
	expression.WriteByte('$')
	return regexp.Compile(expression.String())
}

func (session *hostShellSession) Grep(ctx context.Context, input GrepRequest) ([]GrepMatch, error) {
	request, err := NormalizeGrepRequest(input)
	if err != nil {
		return nil, err
	}
	if err := session.checkContext(ctx); err != nil {
		return nil, err
	}
	matcher, err := regexp.Compile(request.Pattern)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	physical, err := session.resolveLogicalPath(request.Path, false)
	if err != nil {
		return nil, err
	}
	matches := make([]GrepMatch, 0, request.Limit)
	err = filepath.WalkDir(physical, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return mapHostShellFilesystemError(walkErr)
		}
		if entry.IsDir() {
			return session.checkContext(ctx)
		}
		file, openErr := os.Open(current)
		if openErr != nil {
			return mapHostShellFilesystemError(openErr)
		}
		defer file.Close()
		relative, relErr := filepath.Rel(physical, current)
		if relErr != nil {
			return domainsandbox.ErrUnavailable
		}
		logical := path.Join(request.Path, filepath.ToSlash(relative))
		scanner := bufio.NewScanner(io.LimitReader(file, MaxSessionFileBytes+1))
		scanner.Buffer(make([]byte, 64*1024), MaxSessionBodyBytes)
		line, offset := 0, int64(0)
		for scanner.Scan() {
			if err := session.checkContext(ctx); err != nil {
				return err
			}
			line++
			textValue := scanner.Text()
			lineBytes := int64(len(scanner.Bytes()))
			if utf8.ValidString(textValue) && matcher.MatchString(textValue) {
				matches = append(matches, GrepMatch{Path: logical, Line: line, ByteOffset: offset, Text: textValue})
				if len(matches) >= request.Limit {
					return errHostShellLimitReached
				}
			}
			offset += lineBytes + 1
		}
		if scanner.Err() != nil {
			return domainsandbox.ErrCapacityExhausted
		}
		return nil
	})
	if err != nil && !errors.Is(err, errHostShellLimitReached) {
		return nil, err
	}
	return matches, nil
}

func (session *hostShellSession) Replace(ctx context.Context, input ReplaceRequest) error {
	request, err := NormalizeReplaceRequest(input)
	if err != nil {
		return err
	}
	if err := session.checkContext(ctx); err != nil {
		return err
	}
	physical, err := session.resolveLogicalPath(request.Path, true)
	if err != nil {
		return err
	}
	data, err := readHostShellFile(physical, MaxSessionFileBytes)
	if err != nil {
		return err
	}
	if !bytes.Contains(data, request.Old) {
		return domainsandbox.ErrInvalidInput
	}
	replaced := bytes.ReplaceAll(data, request.Old, request.New)
	if len(replaced) > MaxSessionFileBytes {
		return domainsandbox.ErrCapacityExhausted
	}
	if err := os.WriteFile(physical, replaced, 0o600); err != nil {
		return mapHostShellFilesystemError(err)
	}
	return session.checkContext(ctx)
}

func (session *hostShellSession) Download(ctx context.Context, input DownloadRequest) (io.ReadCloser, error) {
	request, err := NormalizeDownloadRequest(input)
	if err != nil {
		return nil, err
	}
	if err := session.checkContext(ctx); err != nil {
		return nil, err
	}
	physical, err := session.resolveLogicalPath(request.Path, false)
	if err != nil {
		return nil, err
	}
	data, err := readHostShellFile(physical, request.MaxBytes)
	if err != nil {
		return nil, err
	}
	if err := session.checkContext(ctx); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func readHostShellFile(physical string, maximum int64) ([]byte, error) {
	file, err := os.Open(physical)
	if err != nil {
		return nil, mapHostShellFilesystemError(err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, domainsandbox.ErrUnavailable
	}
	if int64(len(data)) > maximum {
		return nil, domainsandbox.ErrCapacityExhausted
	}
	return data, nil
}

func (session *hostShellSession) checkContext(ctx context.Context) error {
	if session == nil || session.manager == nil || ctx == nil {
		return domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := session.manager.operationState(session)
	return err
}

func (session *hostShellSession) resolveLogicalPath(logical string, write bool) (string, error) {
	var logicalRoot, physicalRoot string
	switch {
	case logical == "/mnt/user-data/workspace" || strings.HasPrefix(logical, "/mnt/user-data/workspace/"):
		logicalRoot, physicalRoot = "/mnt/user-data/workspace", filepath.Join(session.threadRoot, "workspace")
	case logical == "/mnt/user-data/uploads" || strings.HasPrefix(logical, "/mnt/user-data/uploads/"):
		logicalRoot, physicalRoot = "/mnt/user-data/uploads", filepath.Join(session.threadRoot, "uploads")
	case logical == "/mnt/user-data/outputs" || strings.HasPrefix(logical, "/mnt/user-data/outputs/"):
		logicalRoot, physicalRoot = "/mnt/user-data/outputs", filepath.Join(session.threadRoot, "outputs")
	case !write && (logical == "/mnt/skills" || strings.HasPrefix(logical, "/mnt/skills/")):
		logicalRoot, physicalRoot = "/mnt/skills", session.manager.skillsDir
	default:
		return "", domainsandbox.ErrInvalidInput
	}
	relative := strings.TrimPrefix(logical, logicalRoot)
	relative = strings.TrimPrefix(relative, "/")
	physical := physicalRoot
	if relative != "" {
		physical = filepath.Join(physicalRoot, filepath.FromSlash(relative))
	}
	resolved, err := filepath.Rel(physicalRoot, physical)
	if err != nil || resolved == ".." || strings.HasPrefix(resolved, ".."+string(filepath.Separator)) {
		return "", domainsandbox.ErrInvalidInput
	}
	return physical, nil
}

func (session *hostShellSession) redactPhysicalPaths(value []byte) []byte {
	result := append([]byte(nil), value...)
	replacements := [][2]string{
		{filepath.Join(session.threadRoot, "workspace"), "/mnt/user-data/workspace"},
		{filepath.Join(session.threadRoot, "uploads"), "/mnt/user-data/uploads"},
		{filepath.Join(session.threadRoot, "outputs"), "/mnt/user-data/outputs"},
		{session.manager.skillsDir, "/mnt/skills"},
		{session.threadRoot, "/mnt/user-data"},
		{session.manager.rootDir, "/mnt/user-data"},
	}
	for _, replacement := range replacements {
		result = bytes.ReplaceAll(result, []byte(replacement[0]), []byte(replacement[1]))
	}
	return result
}

func mapHostShellFilesystemError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrPermission) {
		return domainsandbox.ErrExecutionForbidden
	}
	return domainsandbox.ErrUnavailable
}

var errHostShellLimitReached = errors.New("host shell result limit reached")

var _ SandboxSessionManager = (*HostShellSessionManager)(nil)
var _ SandboxSession = (*hostShellSession)(nil)

func (*HostShellSessionManager) String() string {
	return "HostShellSessionManager{host_debug_unisolated:<redacted>}"
}
func (*HostShellSessionManager) GoString() string {
	return "HostShellSessionManager{host_debug_unisolated:<redacted>}"
}
func (manager *HostShellSessionManager) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, manager.String())
}
func (*hostShellSession) String() string {
	return "HostShellSession{host_debug_unisolated:<redacted>}"
}
func (*hostShellSession) GoString() string {
	return "HostShellSession{host_debug_unisolated:<redacted>}"
}
func (session *hostShellSession) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, session.String())
}
