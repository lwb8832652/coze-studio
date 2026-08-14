// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestHostShellSessionManagerValidatesConfigurationAndLifecycle(t *testing.T) {
	root := t.TempDir()
	skills := t.TempDir()
	gate := func() bool { return true }
	var sequence atomic.Int64
	manager, err := NewHostShellSessionManager(HostShellSessionManagerOptions{
		RootDir: root, SkillsDir: skills, ExpectedProviderID: 41,
		AllowedEnvironmentNames: []string{"LANG"}, Gate: gate,
		CancelGrace: 25 * time.Millisecond, MaxSessions: 1, Now: time.Now,
		Random: func() (string, error) { return fmt.Sprintf("id-%d", sequence.Add(1)), nil },
	})
	if err != nil {
		t.Fatalf("NewHostShellSessionManager() error = %v", err)
	}
	var _ SandboxSessionManager = manager

	key := hostShellTestKey(41, 11, 22, "thread-a")
	first, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: key})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	second, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: key})
	if err != nil || second.Ref() != first.Ref() {
		t.Fatalf("idempotent Acquire() = %#v, %v; want %#v", second, err, first.Ref())
	}
	if first.Ref().Key != key || first.Ref().RuntimeGeneration != 1 {
		t.Fatalf("Acquire() ref = %#v", first.Ref())
	}
	if _, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: hostShellTestKey(41, 11, 22, "thread-b")}); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("Acquire(over capacity) error = %v", err)
	}
	if _, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: hostShellTestKey(42, 11, 22, "thread-c")}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("Acquire(wrong provider) error = %v", err)
	}
	interactive := key
	interactive.ThreadID = "thread-interactive"
	interactive.Profile = domainsandbox.SessionProfileInteractive
	if _, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: interactive}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("Acquire(interactive) error = %v", err)
	}

	if got, err := manager.Get(context.Background(), first.Ref()); err != nil || got.Ref() != first.Ref() {
		t.Fatalf("Get() = %#v, %v", got, err)
	}
	if got, err := manager.Recover(context.Background(), first.Ref()); err != nil || got.Ref() != first.Ref() {
		t.Fatalf("Recover(active) = %#v, %v", got, err)
	}
	for _, mutate := range []func(*domainsandbox.SessionRef){
		func(ref *domainsandbox.SessionRef) { ref.Key.ProviderID++ },
		func(ref *domainsandbox.SessionRef) { ref.RuntimeGeneration++ },
		func(ref *domainsandbox.SessionRef) { ref.SessionID += "-other" },
	} {
		ref := first.Ref()
		mutate(&ref)
		if _, err := manager.Get(context.Background(), ref); !errors.Is(err, domainsandbox.ErrSessionNotFound) && !errors.Is(err, domainsandbox.ErrInvalidInput) {
			t.Fatalf("Get(mismatched ref) error = %v", err)
		}
	}

	if err := manager.Release(context.Background(), first.Ref()); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if _, err := manager.Get(context.Background(), first.Ref()); !errors.Is(err, domainsandbox.ErrSessionNotFound) {
		t.Fatalf("Get(released) error = %v", err)
	}
	reacquired, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: key})
	if err != nil || reacquired.Ref() == first.Ref() {
		t.Fatalf("Acquire(after release) = %#v, %v", reacquired, err)
	}
	if err := manager.Destroy(context.Background(), reacquired.Ref()); err != nil {
		t.Fatalf("Destroy() error = %v", err)
	}
	if _, err := manager.Recover(context.Background(), reacquired.Ref()); !errors.Is(err, domainsandbox.ErrSessionNotFound) {
		t.Fatalf("Recover(destroyed) error = %v", err)
	}

	restarted, err := NewHostShellSessionManager(HostShellSessionManagerOptions{
		RootDir: root, SkillsDir: skills, ExpectedProviderID: 41,
		Gate: gate, CancelGrace: 25 * time.Millisecond, MaxSessions: 1,
	})
	if err != nil {
		t.Fatalf("restart manager error = %v", err)
	}
	if _, err := restarted.Recover(context.Background(), reacquired.Ref()); !errors.Is(err, domainsandbox.ErrSessionNotFound) {
		t.Fatalf("Recover(old ref after restart) error = %v", err)
	}

	invalid := []HostShellSessionManagerOptions{
		{RootDir: "relative", SkillsDir: skills, ExpectedProviderID: 41, Gate: gate, CancelGrace: time.Second, MaxSessions: 1},
		{RootDir: root, SkillsDir: "relative", ExpectedProviderID: 41, Gate: gate, CancelGrace: time.Second, MaxSessions: 1},
		{RootDir: root, SkillsDir: skills, ExpectedProviderID: 0, Gate: gate, CancelGrace: time.Second, MaxSessions: 1},
		{RootDir: root, SkillsDir: skills, ExpectedProviderID: 41, Gate: nil, CancelGrace: time.Second, MaxSessions: 1},
		{RootDir: root, SkillsDir: skills, ExpectedProviderID: 41, Gate: gate, CancelGrace: 0, MaxSessions: 1},
		{RootDir: root, SkillsDir: skills, ExpectedProviderID: 41, Gate: gate, CancelGrace: time.Second, MaxSessions: 0},
		{RootDir: root, SkillsDir: skills, ExpectedProviderID: 41, Gate: gate, CancelGrace: time.Second, MaxSessions: 1, AllowedEnvironmentNames: []string{"LANG", "LANG"}},
	}
	for index, options := range invalid {
		if _, err := NewHostShellSessionManager(options); !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
			t.Errorf("invalid options %d error = %v", index, err)
		}
	}
}

func TestHostShellSessionManagerCreatesMissingTrustedSkillsDirectory(t *testing.T) {
	root := t.TempDir()
	skills := filepath.Join(t.TempDir(), "trusted", "skills")
	manager, err := NewHostShellSessionManager(HostShellSessionManagerOptions{
		RootDir: root, SkillsDir: skills, ExpectedProviderID: 41,
		Gate: func() bool { return true }, CancelGrace: time.Second, MaxSessions: 1,
	})
	if err != nil {
		t.Fatalf("NewHostShellSessionManager(missing SkillsDir) error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	info, err := os.Stat(skills)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("created SkillsDir = %#v, %v", info, err)
	}
}

func TestHostShellSessionCreatesControlledDirectoryHierarchyAndPreservesWorkspace(t *testing.T) {
	manager, root, _ := newHostShellTestManager(t, 2, nil)
	key := hostShellTestKey(41, 101, 202, "thread-dir")
	session, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: key})
	if err != nil {
		t.Fatal(err)
	}
	threadRoot := filepath.Join(root, "101", "202", "thread-dir")
	for _, name := range []string{"workspace", "uploads", "outputs"} {
		info, statErr := os.Stat(filepath.Join(threadRoot, name))
		if statErr != nil || !info.IsDir() {
			t.Fatalf("directory %s = %#v, %v", name, info, statErr)
		}
	}
	physical := filepath.Join(threadRoot, "workspace", "keep.txt")
	if err := os.WriteFile(physical, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Destroy(context.Background(), session.Ref()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(physical)
	if err != nil || string(data) != "keep" {
		t.Fatalf("workspace after destroy = %q, %v", data, err)
	}
}

func TestHostShellSessionExecHonorsCWDArgvCommandEnvironmentAndExit(t *testing.T) {
	manager, root, _ := newHostShellTestManager(t, 1, nil)
	session := acquireHostShellTestSession(t, manager, "thread-exec")
	workspace := filepath.Join(root, "1", "2", "thread-exec", "workspace")
	if err := os.Mkdir(filepath.Join(workspace, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}

	stream, err := session.Exec(context.Background(), hostShellExecRequest([]string{"/bin/pwd"}, "", "/mnt/user-data/workspace/nested", nil, 4096))
	if err != nil {
		t.Fatalf("Exec(pwd) error = %v", err)
	}
	events := receiveHostShellEvents(t, stream)
	if got := string(events[0].Data); got != "/mnt/user-data/workspace/nested\n" {
		t.Fatalf("pwd stdout = %q", got)
	}

	stream, err = session.Exec(context.Background(), hostShellExecRequest([]string{"/usr/bin/printf", "%s", "$LANG"}, "", "/mnt/user-data/workspace", nil, 4096))
	if err != nil {
		t.Fatalf("Exec(argv) error = %v", err)
	}
	if got := string(receiveHostShellEvents(t, stream)[0].Data); got != "$LANG" {
		t.Fatalf("argv was shell rewritten: %q", got)
	}

	stream, err = session.Exec(context.Background(), hostShellExecRequest(nil, `printf 'out:%s' "$LANG"; printf 'err' >&2; exit 7`, "/mnt/user-data/workspace", map[string]string{"LANG": "trusted"}, 4096))
	if err != nil {
		t.Fatalf("Exec(command nonzero) error = %v", err)
	}
	events = receiveHostShellEvents(t, stream)
	if len(events) != 3 || events[0].Kind != ExecutionEventStdout || string(events[0].Data) != "out:trusted" ||
		events[1].Kind != ExecutionEventStderr || string(events[1].Data) != "err" ||
		events[2].Kind != ExecutionEventTerminal || events[2].ExitCode == nil || *events[2].ExitCode != 7 {
		t.Fatalf("nonzero events = %#v", events)
	}

	stream, err = session.Exec(context.Background(), hostShellExecRequest([]string{"/usr/bin/env"}, "", "/mnt/user-data/workspace", map[string]string{"LANG": "trusted"}, 4096))
	if err != nil {
		t.Fatalf("Exec(env) error = %v", err)
	}
	environment := string(receiveHostShellEvents(t, stream)[0].Data)
	if !strings.Contains(environment, "LANG=trusted\n") || strings.Contains(environment, "HOME=") || strings.Contains(environment, "AWS_") || strings.Contains(environment, "TOKEN=") {
		t.Fatalf("environment was not minimal: %q", environment)
	}

	if err := manager.UpdateAllowedEnvironmentNames([]string{"TZ"}); err != nil {
		t.Fatalf("UpdateAllowedEnvironmentNames() error = %v", err)
	}
	if _, err := session.Exec(context.Background(), hostShellExecRequest([]string{"/usr/bin/env"}, "", "/mnt/user-data/workspace", map[string]string{"LANG": "no-longer-allowed"}, 4096)); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("Exec(stale env allowlist) error = %v", err)
	}
	if err := manager.UpdateAllowedEnvironmentNames([]string{"TZ", "TZ"}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("UpdateAllowedEnvironmentNames(duplicate) error = %v", err)
	}
}

func TestHostShellSessionExecCancelsProcessGroupAndEnforcesDeadlineGateAndOutputLimit(t *testing.T) {
	var gate atomic.Bool
	gate.Store(true)
	manager, root, _ := newHostShellTestManager(t, 1, gate.Load)
	session := acquireHostShellTestSession(t, manager, "thread-control")
	workspace := filepath.Join(root, "1", "2", "thread-control", "workspace")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := session.Exec(ctx, hostShellExecRequest(nil, `(sleep 0.35; printf leaked > child-marker) & wait`, "/mnt/user-data/workspace", nil, 4096))
		done <- err
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Exec(cancel) error = %v", err)
	}
	time.Sleep(400 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(workspace, "child-marker")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("child survived process-group cancel: %v", err)
	}

	deadlineRequest := hostShellExecRequest(nil, "sleep 5", "/mnt/user-data/workspace", nil, 4096)
	deadlineRequest.Deadline = time.Now().Add(90 * time.Millisecond)
	started := time.Now()
	if _, err := session.Exec(context.Background(), deadlineRequest); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Exec(deadline) error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("deadline termination took %s", elapsed)
	}

	done = make(chan error, 1)
	go func() {
		_, err := session.Exec(context.Background(), hostShellExecRequest(nil, "sleep 5", "/mnt/user-data/workspace", nil, 4096))
		done <- err
	}()
	time.Sleep(60 * time.Millisecond)
	gate.Store(false)
	select {
	case err := <-done:
		if !errors.Is(err, domainsandbox.ErrUnavailable) {
			t.Fatalf("Exec(hot gate) error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("hot gate did not terminate running Exec")
	}
	if _, err := session.Read(context.Background(), ReadRequest{Path: "/mnt/user-data/workspace/missing", MaxBytes: 1}); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Read(closed gate) error = %v", err)
	}
	gate.Store(true)

	started = time.Now()
	if _, err := session.Exec(context.Background(), hostShellExecRequest(nil, "yes secret", "/mnt/user-data/workspace", nil, 64)); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("Exec(output limit) error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("output-limit termination took %s", elapsed)
	}
}

func TestHostShellSessionSerializesExecPerSession(t *testing.T) {
	manager, _, _ := newHostShellTestManager(t, 1, nil)
	session := acquireHostShellTestSession(t, manager, "thread-serial")
	firstStarted := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		close(firstStarted)
		_, err := session.Exec(context.Background(), hostShellExecRequest(nil, "sleep 0.2", "/mnt/user-data/workspace", nil, 1024))
		firstDone <- err
	}()
	<-firstStarted
	time.Sleep(30 * time.Millisecond)
	secondStarted := time.Now()
	_, secondErr := session.Exec(context.Background(), hostShellExecRequest(nil, "true", "/mnt/user-data/workspace", nil, 1024))
	if firstErr := <-firstDone; firstErr != nil || secondErr != nil {
		t.Fatalf("serialized Exec errors = %v, %v", firstErr, secondErr)
	}
	if elapsed := time.Since(secondStarted); elapsed < 130*time.Millisecond {
		t.Fatalf("second Exec was not serialized: %s", elapsed)
	}
}

func TestHostShellSessionFileOperationsMapLogicalPathsAndBoundResults(t *testing.T) {
	manager, root, skills := newHostShellTestManager(t, 1, nil)
	session := acquireHostShellTestSession(t, manager, "thread-files")
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(skills, "SKILL.md"), []byte("trusted skill"), 0o600); err != nil {
		t.Fatal(err)
	}

	file := "/mnt/user-data/workspace/dir/data.txt"
	if err := session.Write(ctx, WriteRequest{Path: file, Content: []byte("alpha\nbeta\nalpha\n")}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := session.Write(ctx, WriteRequest{Path: file, Content: []byte("tail\n"), Append: true}); err != nil {
		t.Fatalf("Write(append) error = %v", err)
	}
	content, err := session.Read(ctx, ReadRequest{Path: file, MaxBytes: 128})
	if err != nil || string(content.Data) != "alpha\nbeta\nalpha\ntail\n" {
		t.Fatalf("Read() = %q, %v", content.Data, err)
	}
	if _, err := session.Read(ctx, ReadRequest{Path: file, MaxBytes: 2}); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("Read(over limit) error = %v", err)
	}

	entries, err := session.List(ctx, ListRequest{Path: "/mnt/user-data/workspace/dir", Limit: 10})
	if err != nil || len(entries) != 1 || entries[0].Path != file || entries[0].Directory || entries[0].Size == 0 || entries[0].Modified.IsZero() {
		t.Fatalf("List() = %#v, %v", entries, err)
	}
	if strings.Contains(fmt.Sprintf("%#v", entries), root) {
		t.Fatalf("List leaked physical root: %#v", entries)
	}
	if _, err := session.List(ctx, ListRequest{Path: "/mnt/user-data/workspace/dir", Limit: 0}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("List(invalid limit) error = %v", err)
	}

	globbed, err := session.Glob(ctx, GlobRequest{Path: "/mnt/user-data/workspace", Pattern: "**/*.txt", Limit: 10})
	if err != nil || len(globbed) != 1 || globbed[0].Path != file {
		t.Fatalf("Glob() = %#v, %v", globbed, err)
	}
	matches, err := session.Grep(ctx, GrepRequest{Path: "/mnt/user-data/workspace", Pattern: "alpha", Limit: 10})
	if err != nil || len(matches) != 2 || matches[0].Path != file || matches[0].Line != 1 || matches[1].Line != 3 || matches[0].ByteOffset != 0 {
		t.Fatalf("Grep() = %#v, %v", matches, err)
	}
	if err := session.Replace(ctx, ReplaceRequest{Path: file, Old: []byte("alpha"), New: []byte("omega")}); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	content, _ = session.Read(ctx, ReadRequest{Path: file, MaxBytes: 128})
	if got := string(content.Data); got != "omega\nbeta\nomega\ntail\n" {
		t.Fatalf("Replace result = %q", got)
	}
	download, err := session.Download(ctx, DownloadRequest{Path: file, MaxBytes: 128})
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	downloaded, readErr := io.ReadAll(download)
	closeErr := download.Close()
	if readErr != nil || closeErr != nil || string(downloaded) != string(content.Data) {
		t.Fatalf("Download body = %q, %v, %v", downloaded, readErr, closeErr)
	}
	if _, err := session.Download(ctx, DownloadRequest{Path: file, MaxBytes: 2}); !errors.Is(err, domainsandbox.ErrCapacityExhausted) {
		t.Fatalf("Download(over limit) error = %v", err)
	}

	skill, err := session.Read(ctx, ReadRequest{Path: "/mnt/skills/SKILL.md", MaxBytes: 128})
	if err != nil || string(skill.Data) != "trusted skill" {
		t.Fatalf("Read(skill) = %q, %v", skill.Data, err)
	}
	if err := session.Write(ctx, WriteRequest{Path: "/mnt/skills/SKILL.md", Content: []byte("overwrite")}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("Write(skill) error = %v", err)
	}
	if err := session.Replace(ctx, ReplaceRequest{Path: "/mnt/skills/SKILL.md", Old: []byte("trusted"), New: []byte("bad")}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("Replace(skill) error = %v", err)
	}

	physical := filepath.Join(root, "1", "2", "thread-files", "workspace", "dir", "data.txt")
	for _, bad := range []ReadRequest{
		{Path: physical, MaxBytes: 1},
		{Path: "/mnt/user-data/workspace/../uploads/escape", MaxBytes: 1},
	} {
		if _, err := session.Read(ctx, bad); !errors.Is(err, domainsandbox.ErrInvalidInput) {
			t.Fatalf("Read(escape) error = %v", err)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := session.Read(canceled, ReadRequest{Path: file, MaxBytes: 128}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Read(canceled) error = %v", err)
	}
}

func TestHostShellSessionErrorsAndFormattingDoNotLeakHostDetails(t *testing.T) {
	manager, root, _ := newHostShellTestManager(t, 1, nil)
	session := acquireHostShellTestSession(t, manager, "thread-redact")
	secretCommand := "definitely-not-an-executable-secret"
	secretEnv := "secret-env-value"
	_, err := session.Exec(context.Background(), hostShellExecRequest([]string{secretCommand}, "", "/mnt/user-data/workspace", map[string]string{"LANG": secretEnv}, 1024))
	if err == nil {
		t.Fatal("Exec(missing executable) unexpectedly succeeded")
	}
	for _, leaked := range []string{root, secretCommand, secretEnv, "pid"} {
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(leaked)) {
			t.Fatalf("error leaked %q: %v", leaked, err)
		}
	}
	for _, value := range []any{manager, session} {
		rendered := fmt.Sprintf("%+v %#v %v", value, value, value)
		if strings.Contains(rendered, root) || strings.Contains(rendered, secretEnv) {
			t.Fatalf("formatting leaked host details: %q", rendered)
		}
	}
}

func TestHostShellSessionShutdownTerminatesProcessesAndRejectsOperations(t *testing.T) {
	manager, _, _ := newHostShellTestManager(t, 1, nil)
	session := acquireHostShellTestSession(t, manager, "thread-shutdown")
	done := make(chan error, 1)
	go func() {
		_, err := session.Exec(context.Background(), hostShellExecRequest(nil, "sleep 5", "/mnt/user-data/workspace", nil, 1024))
		done <- err
	}()
	time.Sleep(60 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, domainsandbox.ErrUnavailable) {
			t.Fatalf("running Exec after Shutdown error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not stop running Exec")
	}
	if _, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: hostShellTestKey(41, 1, 2, "thread-new")}); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Acquire(after Shutdown) error = %v", err)
	}
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
}

func TestHostShellSessionShutdownSignalsAllSessionsBeforeReturningDeadline(t *testing.T) {
	root := t.TempDir()
	skills := t.TempDir()
	var sequence atomic.Int64
	manager, err := NewHostShellSessionManager(HostShellSessionManagerOptions{
		RootDir: root, SkillsDir: skills, ExpectedProviderID: 41,
		Gate: func() bool { return true }, CancelGrace: 2 * time.Second, MaxSessions: 3,
		Random: func() (string, error) { return fmt.Sprintf("shutdown-%d", sequence.Add(1)), nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	results := make([]chan error, 0, 3)
	readyPaths := make([]string, 0, 3)
	survivedPaths := make([]string, 0, 3)
	for index := 0; index < 3; index++ {
		threadID := fmt.Sprintf("thread-shutdown-%d", index)
		session, acquireErr := manager.Acquire(context.Background(), AcquireSessionRequest{
			Key: hostShellTestKey(41, 1, int64(index+1), threadID),
		})
		if acquireErr != nil {
			t.Fatal(acquireErr)
		}
		result := make(chan error, 1)
		results = append(results, result)
		readyPaths = append(readyPaths, filepath.Join(root, "1", fmt.Sprint(index+1), threadID, "workspace", "shutdown-ready"))
		survivedPaths = append(survivedPaths, filepath.Join(root, "1", fmt.Sprint(index+1), threadID, "workspace", "shutdown-survived"))
		go func(session SandboxSession, result chan<- error) {
			request := hostShellExecRequest(nil, "trap '' TERM; : > shutdown-ready; sleep 0.4; : > shutdown-survived; sleep 0.6", "/mnt/user-data/workspace", nil, 1024)
			_, execErr := session.Exec(context.Background(), request)
			result <- execErr
		}(session, result)
	}
	readyDeadline := time.Now().Add(time.Second)
	for _, readyPath := range readyPaths {
		for {
			if _, statErr := os.Stat(readyPath); statErr == nil {
				break
			}
			if time.Now().After(readyDeadline) {
				t.Fatalf("process did not become ready: %s", readyPath)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	started := time.Now()
	shutdownErr := manager.Shutdown(shutdownCtx)
	if !errors.Is(shutdownErr, context.DeadlineExceeded) {
		t.Fatalf("Shutdown(short deadline) error = %v", shutdownErr)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown ignored total deadline: %s", elapsed)
	}
	for index, result := range results {
		select {
		case execErr := <-result:
			if !errors.Is(execErr, domainsandbox.ErrUnavailable) {
				t.Errorf("Exec session %d error = %v, want ErrUnavailable", index, execErr)
			}
		case <-time.After(500 * time.Millisecond):
			t.Errorf("Exec session %d was not terminated", index)
		}
		if _, statErr := os.Stat(survivedPaths[index]); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("Session %d survived shutdown deadline: %v", index, statErr)
		}
	}
}

func newHostShellTestManager(t *testing.T, maximum int, gate func() bool) (*HostShellSessionManager, string, string) {
	t.Helper()
	root := t.TempDir()
	skills := t.TempDir()
	if gate == nil {
		gate = func() bool { return true }
	}
	var mu sync.Mutex
	sequence := 0
	manager, err := NewHostShellSessionManager(HostShellSessionManagerOptions{
		RootDir: root, SkillsDir: skills, ExpectedProviderID: 41,
		AllowedEnvironmentNames: []string{"LANG"}, Gate: gate,
		CancelGrace: 20 * time.Millisecond, MaxSessions: maximum, Now: time.Now,
		Random: func() (string, error) {
			mu.Lock()
			defer mu.Unlock()
			sequence++
			return fmt.Sprintf("session-%d", sequence), nil
		},
	})
	if err != nil {
		t.Fatalf("NewHostShellSessionManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	return manager, root, skills
}

func hostShellTestKey(providerID, spaceID, userID int64, threadID string) domainsandbox.SessionKey {
	return domainsandbox.SessionKey{
		DeploymentID: "runner-dev-a", ProviderID: providerID, SpaceID: spaceID,
		UserID: userID, ThreadID: threadID, Profile: domainsandbox.SessionProfileCore,
	}
}

func acquireHostShellTestSession(t *testing.T, manager *HostShellSessionManager, threadID string) SandboxSession {
	t.Helper()
	session, err := manager.Acquire(context.Background(), AcquireSessionRequest{Key: hostShellTestKey(41, 1, 2, threadID)})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	return session
}

func hostShellExecRequest(argv []string, command, cwd string, environment map[string]string, maximum int64) ExecRequest {
	return ExecRequest{
		OperationID: fmt.Sprintf("operation-%d", time.Now().UnixNano()), Argv: argv, Command: command,
		Env: environment, CWD: cwd, Deadline: time.Now().Add(5 * time.Second), MaxOutputBytes: maximum,
	}
}

func receiveHostShellEvents(t *testing.T, stream ExecutionStream) []ExecutionEvent {
	t.Helper()
	defer stream.Close()
	var events []ExecutionEvent
	for {
		event, err := stream.Recv(context.Background())
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}
		events = append(events, event)
	}
	return events
}
