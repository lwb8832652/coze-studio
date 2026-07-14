// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSafeWorkdirManagerCreateWritesVersionedAtomicIdentityMarker(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	workdir, err := manager.Create(context.Background(), SafeWorkdirCreateRequest{Prefix: managementSafeWorkdirPrefix})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	markerPath := filepath.Join(
		manager.ControlRoot(),
		safeWorkdirLeaseDirectory,
		safeWorkdirIdentityMarkerFilename(workdir.RelativePath),
	)
	info, err := os.Lstat(markerPath)
	if err != nil {
		t.Fatalf("stat marker: %v", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("marker mode = %v", info.Mode())
	}
	payload, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	var marker safeWorkdirIdentityMarker
	if err := json.Unmarshal(payload, &marker); err != nil {
		t.Fatalf("decode marker: %v", err)
	}
	if marker.Version != safeWorkdirIdentityMarkerVersion || marker.RelativePath != workdir.RelativePath ||
		marker.InvocationID == "" || marker.InstanceID != manager.platform.instanceID ||
		marker.LockGeneration != manager.platform.lockGeneration {
		t.Fatalf("marker identity = %#v", marker)
	}
	entries, err := os.ReadDir(workdir.Path)
	if err != nil || len(entries) != 0 {
		t.Fatalf("control marker leaked into child-visible cwd: entries=%v err=%v", entries, err)
	}
}

func TestSafeWorkdirManagerHoldsCloseOnExecExclusiveRootLock(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new locked manager: %v", err)
	}
	flags, err := unix.FcntlInt(uintptr(manager.platform.lockFD), unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatalf("root lock is not close-on-exec: flags=%d err=%v", flags, err)
	}
	if _, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root}); !errors.Is(err, ErrSafeWorkdirRootLocked) {
		t.Fatalf("second manager acquired active root: %v", err)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("close first manager: %v", err)
	}
	reopened, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("lock was not released on close: %v", err)
	}
	defer reopened.Close()
}

func TestSafeWorkdirManagerRecoveryRejectsDamagedAndUnrelatedDirectories(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	crashedManager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	valid, err := crashedManager.Create(context.Background(), SafeWorkdirCreateRequest{Prefix: managementSafeWorkdirPrefix})
	if err != nil {
		t.Fatalf("create valid crash directory: %v", err)
	}
	nested, err := crashedManager.Create(context.Background(), SafeWorkdirCreateRequest{
		Parent: "nested",
		Prefix: managementSafeWorkdirPrefix,
	})
	if err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	if err := crashedManager.Close(); err != nil {
		t.Fatalf("release crashed instance lock: %v", err)
	}
	damaged := filepath.Join(root, managementSafeWorkdirPrefix+"damaged")
	if err := os.Mkdir(damaged, 0o700); err != nil {
		t.Fatalf("create damaged directory: %v", err)
	}
	unrelated := filepath.Join(root, "operator-owned-unrelated")
	if err := os.Mkdir(unrelated, 0o700); err != nil {
		t.Fatalf("create unrelated directory: %v", err)
	}

	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new recovering manager: %v", err)
	}
	defer manager.Close()
	report, err := manager.RecoverDirectChildren(context.Background(), managementSafeWorkdirPrefix)
	if !errors.Is(err, ErrSafeWorkdirRecoveryRejected) {
		t.Fatalf("recovery error = %v, report=%#v", err, report)
	}
	if report.Recovered != 1 || report.Rejected != 1 {
		t.Fatalf("recovery report = %#v", report)
	}
	if _, err := os.Stat(valid.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("valid crash directory remains: %v", err)
	}
	if _, err := os.Stat(damaged); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("damaged directory was not isolated: %v", err)
	}
	rejected, err := os.ReadDir(filepath.Join(manager.ControlRoot(), safeWorkdirRejectedDirectory))
	if err != nil || len(rejected) == 0 {
		t.Fatalf("damaged directory missing from rejected control area: entries=%v err=%v", rejected, err)
	}
	for _, path := range []string{unrelated, nested.Path} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("recovery deleted protected path %q: %v", path, err)
		}
	}
}

func TestSafeWorkdirManagerNeverRecoversCurrentLockGeneration(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	workdir, err := manager.Create(context.Background(), SafeWorkdirCreateRequest{Prefix: managementSafeWorkdirPrefix})
	if err != nil {
		t.Fatalf("create current workdir: %v", err)
	}
	if _, err := manager.RecoverDirectChildren(context.Background(), managementSafeWorkdirPrefix); err == nil {
		t.Fatal("current lock generation was accepted as a crashed predecessor")
	}
	if _, err := os.Stat(workdir.Path); err != nil {
		t.Fatalf("current workdir was recovered: %v", err)
	}
}

func TestSafeWorkdirManagerDoesNotFollowReplacedAncestor(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()

	workdir, err := manager.Create(context.Background(), SafeWorkdirCreateRequest{
		Parent: filepath.Join("runs", "one"),
		Prefix: "invocation-",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	outside := safeWorkdirTestRoot(t)
	moved := filepath.Join(outside, "moved-parent")
	if err := os.Rename(filepath.Join(root, "runs"), moved); err != nil {
		t.Fatalf("move checked ancestor: %v", err)
	}
	marker := filepath.Join(outside, "must-survive")
	if err := os.WriteFile(marker, []byte("safe"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "runs")); err != nil {
		t.Fatalf("replace ancestor: %v", err)
	}

	err = manager.Delete(context.Background(), workdir.RelativePath)
	if !errors.Is(err, ErrSafeWorkdirCleanupRetryable) {
		t.Fatalf("expected retryable no-follow failure, got %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("outside marker changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(moved, "one", filepath.Base(workdir.Path))); err != nil {
		t.Fatalf("moved invocation was unexpectedly removed: %v", err)
	}
}

func TestSafeWorkdirManagerCleanupBudgetAndTimeoutAreRetryable(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{
		Root: root,
		DeleteLimits: SafeWorkdirDeleteLimits{
			MaxEntries: 3,
			MaxBytes:   512,
			MaxDepth:   8,
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	workdir, err := manager.Create(context.Background(), SafeWorkdirCreateRequest{Prefix: "invocation-"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for index := 0; index < 12; index++ {
		name := filepath.Join(workdir.Path, "entry-"+string(rune('a'+index)))
		if err := os.WriteFile(name, []byte("bounded"), 0o600); err != nil {
			t.Fatalf("write tree: %v", err)
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.Delete(canceled, workdir.RelativePath); !errors.Is(err, ErrSafeWorkdirCleanupRetryable) {
		t.Fatalf("canceled cleanup = %v", err)
	}

	retries := 0
	for attempt := 0; attempt < 16; attempt++ {
		err = manager.Delete(context.Background(), workdir.RelativePath)
		if err == nil {
			break
		}
		if !errors.Is(err, ErrSafeWorkdirCleanupRetryable) {
			t.Fatalf("cleanup attempt %d: %v", attempt, err)
		}
		retries++
	}
	if err != nil || retries == 0 {
		t.Fatalf("bounded cleanup did not converge, retries=%d err=%v", retries, err)
	}
	if _, err := os.Stat(workdir.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workdir remains: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, safeWorkdirTrashDirectory))
	if err != nil || len(entries) != 0 {
		t.Fatalf("cleanup quarantine remains: entries=%d err=%v", len(entries), err)
	}
}

func TestSafeWorkdirManagerDeepTreesConvergeThroughAssociatedSpills(t *testing.T) {
	for _, depth := range []int{65, 131} {
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			root := safeWorkdirTestRoot(t)
			manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{
				Root: root,
				DeleteLimits: SafeWorkdirDeleteLimits{
					MaxEntries: 4096,
					MaxBytes:   1 << 20,
					MaxDepth:   64,
				},
			})
			if err != nil {
				t.Fatalf("new manager: %v", err)
			}
			defer manager.Close()
			workdir, err := manager.Create(context.Background(), SafeWorkdirCreateRequest{Prefix: "invocation-"})
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			current := workdir.Path
			for index := 0; index < depth; index++ {
				current = filepath.Join(current, "d")
				if err := os.Mkdir(current, 0o700); err != nil {
					t.Fatalf("mkdir depth %d: %v", index, err)
				}
			}
			if err := os.WriteFile(filepath.Join(current, "leaf"), []byte("bounded"), 0o600); err != nil {
				t.Fatalf("write leaf: %v", err)
			}

			retries := 0
			for attempt := 0; attempt < 8; attempt++ {
				err = manager.Delete(context.Background(), workdir.RelativePath)
				if err == nil {
					break
				}
				if !errors.Is(err, ErrSafeWorkdirCleanupRetryable) {
					t.Fatalf("cleanup attempt %d: %v", attempt, err)
				}
				retries++
			}
			if err != nil || retries == 0 {
				t.Fatalf("deep cleanup did not converge: retries=%d err=%v", retries, err)
			}
			assertSafeWorkdirCleanupComplete(t, root, workdir.Path)
		})
	}
}

func TestSafeWorkdirManagerQuarantineScanKeepsSpillsAssociated(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{
		Root: root,
		DeleteLimits: SafeWorkdirDeleteLimits{
			MaxEntries: 128,
			MaxBytes:   1 << 16,
			MaxDepth:   2,
		},
	})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	bucketName := safeWorkdirTrashBucketName(filepath.Join("runs", "invocation-associated"))
	current := filepath.Join(root, safeWorkdirTrashDirectory, bucketName)
	if err := os.Mkdir(current, 0o700); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	for index := 0; index < 8; index++ {
		current = filepath.Join(current, "d")
		if err := os.Mkdir(current, 0o700); err != nil {
			t.Fatalf("create depth %d: %v", index, err)
		}
	}
	if err := os.WriteFile(filepath.Join(current, "leaf"), []byte("bounded"), 0o600); err != nil {
		t.Fatalf("write leaf: %v", err)
	}

	err = manager.CleanupQuarantine(context.Background())
	if !errors.Is(err, ErrSafeWorkdirCleanupRetryable) {
		t.Fatalf("first scan = %v, want retryable spill", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, safeWorkdirTrashDirectory))
	if err != nil || len(entries) == 0 {
		t.Fatalf("read associated spill: entries=%d err=%v", len(entries), err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), bucketName) {
			t.Fatalf("orphaned spill %q is not associated with %q", entry.Name(), bucketName)
		}
	}
	for attempt := 0; attempt < 8; attempt++ {
		err = manager.CleanupQuarantine(context.Background())
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("associated startup cleanup did not converge: %v", err)
	}
	assertSafeWorkdirCleanupComplete(t, root, filepath.Join(root, "absent-source"))
}

func TestSafeWorkdirManagerRequarantinesSourceRecreatedAfterRemoval(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	workdir, err := manager.Create(context.Background(), SafeWorkdirCreateRequest{Prefix: "invocation-"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rebuild := true
	var rebuildErr error
	manager.platform.afterQuarantineRemoved = func() {
		if !rebuild {
			return
		}
		rebuild = false
		rebuildErr = os.Mkdir(workdir.Path, 0o700)
		if rebuildErr == nil {
			rebuildErr = os.WriteFile(filepath.Join(workdir.Path, "rebuilt"), []byte("again"), 0o600)
		}
	}
	err = manager.Delete(context.Background(), workdir.RelativePath)
	if rebuildErr != nil {
		t.Fatalf("rebuild source: %v", rebuildErr)
	}
	if err != nil && !errors.Is(err, ErrSafeWorkdirCleanupRetryable) {
		t.Fatalf("cleanup after one rebuild: %v", err)
	}
	for attempt := 0; err != nil && attempt < 8; attempt++ {
		err = manager.Delete(context.Background(), workdir.RelativePath)
	}
	if err != nil {
		t.Fatalf("recreated source cleanup did not converge: %v", err)
	}
	assertSafeWorkdirCleanupComplete(t, root, workdir.Path)
}

func TestSafeWorkdirManagerContinuousSourceRebuildNeverReportsSuccess(t *testing.T) {
	root := safeWorkdirTestRoot(t)
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer manager.Close()
	workdir, err := manager.Create(context.Background(), SafeWorkdirCreateRequest{Prefix: "invocation-"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var rebuildErr error
	manager.platform.afterQuarantineRemoved = func() {
		if rebuildErr != nil {
			return
		}
		rebuildErr = os.Mkdir(workdir.Path, 0o700)
	}
	err = manager.Delete(context.Background(), workdir.RelativePath)
	if rebuildErr != nil {
		t.Fatalf("continuously rebuild source: %v", rebuildErr)
	}
	if !errors.Is(err, ErrSafeWorkdirCleanupRetryable) {
		t.Fatalf("continuous rebuild cleanup = %v, want retryable", err)
	}
	if _, statErr := os.Stat(workdir.Path); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rebuilt source was not requarantined: %v", statErr)
	}
	manager.platform.afterQuarantineRemoved = nil
	for attempt := 0; attempt < 8; attempt++ {
		err = manager.Delete(context.Background(), workdir.RelativePath)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("cleanup after rebuild stopped: %v", err)
	}
	assertSafeWorkdirCleanupComplete(t, root, workdir.Path)
}

func TestSafeWorkdirManagerRejectsSymlinkRoot(t *testing.T) {
	target := safeWorkdirTestRoot(t)
	container := safeWorkdirTestRoot(t)
	root := filepath.Join(container, "root-link")
	if err := os.Symlink(target, root); err != nil {
		t.Fatalf("symlink root: %v", err)
	}
	manager, err := NewSafeWorkdirManager(SafeWorkdirOptions{Root: root})
	if err == nil || manager != nil {
		t.Fatalf("symlink root must fail closed: manager=%v err=%v", manager, err)
	}
}

func safeWorkdirTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("canonical root: %v", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatalf("secure root: %v", err)
	}
	return filepath.Clean(root)
}

func assertSafeWorkdirCleanupComplete(t *testing.T, root, source string) {
	t.Helper()
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source remains: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, safeWorkdirTrashDirectory))
	if err != nil || len(entries) != 0 {
		t.Fatalf("quarantine remains: entries=%d err=%v", len(entries), err)
	}
}
