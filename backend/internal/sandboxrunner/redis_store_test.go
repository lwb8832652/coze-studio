// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestRedisStoreAcceptIsAtomicAndIdempotent(t *testing.T) {
	store, server := newRedisStoreFixture(t, "key-1")
	command := validStoredCommand(t, "operation-1", "first request")
	first, replayed, err := store.Accept(context.Background(), command)
	if err != nil || replayed || first.State != ExecutionStateAccepted {
		t.Fatalf("first/replayed/error = %#v/%t/%v", first, replayed, err)
	}
	replay, replayed, err := store.Accept(context.Background(), command)
	if err != nil || !replayed || replay.ExecutionID != first.ExecutionID {
		t.Fatalf("replay/replayed/error = %#v/%t/%v", replay, replayed, err)
	}
	conflict := validStoredCommand(t, "operation-1", "different request")
	if _, _, err := store.Accept(context.Background(), conflict); err == nil {
		t.Fatal("Accept() unexpectedly accepted same operation with a different digest")
	}
	for _, key := range server.Keys() {
		if server.Type(key) != "string" {
			continue
		}
		value, err := server.Get(key)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(key, "space-11") || strings.Contains(value, "first request") || strings.Contains(value, "project-22") {
			t.Fatalf("plaintext found in redis key/value: %q=%q", key, value)
		}
	}
}

func TestRedisStoreIdempotencyReplayRejectsDifferentAuthenticatedIdentity(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	firstCommand := validStoredCommand(t, "operation-shared", "same request")
	secondCommand := validStoredCommand(t, "operation-shared", "same request")
	secondCommand.Identity.SpaceID = 99
	secondCommand.Identity.UserID = 100
	secondCommand.Identity.ProjectID = "project-other"

	first, replayed, err := store.Accept(context.Background(), firstCommand)
	if err != nil || replayed {
		t.Fatalf("first accept/replayed/error = %#v/%t/%v", first, replayed, err)
	}
	second, replayed, err := store.Accept(context.Background(), secondCommand)
	if !errors.Is(err, ErrExecutionConflict) || replayed || second.ExecutionID != "" {
		t.Fatalf("cross-identity accept/replayed/error = %#v/%t/%v", second, replayed, err)
	}
}

func TestRedisStoreReadsOldKeyAndWritesWithActiveKey(t *testing.T) {
	store, server := newRedisStoreFixture(t, "old")
	command := validStoredCommand(t, "operation-rotate", "rotation request")
	stored, _, err := store.Accept(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	store.keys.ActiveKeyID = "new"
	if _, err := store.Get(context.Background(), stored.ExecutionID); err != nil {
		t.Fatalf("Get() after rotation: %v", err)
	}
	next, _, err := store.Accept(context.Background(), validStoredCommand(t, "operation-new", "new key request"))
	if err != nil || next.ExecutionID == "" {
		t.Fatalf("next/error = %#v/%v", next, err)
	}
	ciphertext, err := server.Get(store.executionKey(next.ExecutionID))
	if err != nil || !strings.Contains(ciphertext, `"key_id":"new"`) {
		t.Fatalf("new key ciphertext/error = %q/%v", ciphertext, err)
	}
}

func TestRedisStoreAcceptsOneConcurrentOperationAndEnforcesQueueDepth(t *testing.T) {
	store, server := newRedisStoreFixture(t, "key-1")
	command := validStoredCommand(t, "operation-race", "concurrent request")
	var group sync.WaitGroup
	results := make(chan struct {
		execution StoredExecution
		replayed  bool
		err       error
	}, 16)
	for range 16 {
		group.Add(1)
		go func() {
			defer group.Done()
			execution, replayed, err := store.Accept(context.Background(), command)
			results <- struct {
				execution StoredExecution
				replayed  bool
				err       error
			}{execution, replayed, err}
		}()
	}
	group.Wait()
	close(results)
	var executionID string
	accepted := 0
	for result := range results {
		if result.err != nil {
			t.Fatalf("Accept() error = %v", result.err)
		}
		if !result.replayed {
			accepted++
		}
		if executionID == "" {
			executionID = result.execution.ExecutionID
		} else if result.execution.ExecutionID != executionID {
			t.Fatalf("execution IDs differ: %q / %q", executionID, result.execution.ExecutionID)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted count = %d", accepted)
	}
	if _, _, err := store.Accept(context.Background(), validStoredCommand(t, "operation-two", "second request")); err != nil {
		t.Fatalf("second accept: %v", err)
	}
	if _, _, err := store.Accept(context.Background(), validStoredCommand(t, "operation-three", "third request")); err == nil {
		t.Fatal("Accept() unexpectedly exceeded queue depth")
	}
	if value, err := server.Get(store.queueKey()); err != nil || value != "2" {
		t.Fatalf("queue depth/error = %q/%v", value, err)
	}
}

func TestRedisStoreRecordExpiresAndRejectsTamperedCiphertext(t *testing.T) {
	store, server := newRedisStoreFixture(t, "key-1")
	store.recordTTL = time.Second
	stored, _, err := store.Accept(context.Background(), validStoredCommand(t, "operation-expiry", "expiring request"))
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Set(store.executionKey(stored.ExecutionID), `{"schema":"coze.sandbox.runner_encrypted_execution.v1","key_id":"key-1","deployment_id":"bad","nonce":"bad","ciphertext":"bad"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), stored.ExecutionID); err == nil {
		t.Fatal("Get() unexpectedly accepted tampered ciphertext")
	}
	stored, _, err = store.Accept(context.Background(), validStoredCommand(t, "operation-ttl", "ttl request"))
	if err != nil {
		t.Fatal(err)
	}
	server.FastForward(2 * time.Second)
	if _, err := store.Get(context.Background(), stored.ExecutionID); err == nil {
		t.Fatal("Get() unexpectedly found expired execution")
	}
}

func TestRedisStoreStateTransitionsAreMonotonicAndTerminalStatesImmutable(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	stored, _, err := store.Accept(context.Background(), validStoredCommand(t, "operation-state", "state request"))
	if err != nil {
		t.Fatal(err)
	}
	running, err := store.Transition(context.Background(), stored.ExecutionID, ExecutionStateRunning)
	if err != nil || running.State != ExecutionStateRunning {
		t.Fatalf("running/error = %#v/%v", running, err)
	}
	terminal, err := store.Transition(context.Background(), stored.ExecutionID, infrasandbox.ExecutionStatusCanceled)
	if err != nil || terminal.State != infrasandbox.ExecutionStatusCanceled {
		t.Fatalf("terminal/error = %#v/%v", terminal, err)
	}
	if _, err := store.Transition(context.Background(), stored.ExecutionID, ExecutionStateRunning); err == nil {
		t.Fatal("Transition() unexpectedly revived terminal execution")
	}
	if _, err := store.Transition(context.Background(), stored.ExecutionID, infrasandbox.ExecutionStatusSucceeded); err == nil {
		t.Fatal("Transition() unexpectedly changed terminal execution")
	}
}

func TestRedisStoreImplementsSafeStatusLookupQueueAndCancel(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	command := validStoredCommand(t, "operation-control", "control request")
	stored, _, err := store.Accept(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status(context.Background(), stored.ExecutionID)
	if err != nil || status.ExecutionID != stored.ExecutionID || status.Status != ExecutionStateAccepted {
		t.Fatalf("status/error = %#v/%v", status, err)
	}
	queue, err := store.QueueStatus(context.Background(), stored.ExecutionID)
	if err != nil || !queue.Waiting || !queue.Cancelable {
		t.Fatalf("queue/error = %#v/%v", queue, err)
	}
	lookup, err := store.Lookup(context.Background(), infrasandbox.ExecutionLookupRequest{Scope: command.Scope, WorkloadKind: command.WorkloadKind, OperationID: command.IdempotencyKey, RequestDigest: mustExecutionDigest(t, command.RawBody)})
	if err != nil || lookup.Status != infrasandbox.ExecutionLookupFound || lookup.Execution.ExecutionID != stored.ExecutionID {
		t.Fatalf("lookup/error = %#v/%v", lookup, err)
	}
	if err := store.Cancel(context.Background(), stored.ExecutionID); err != nil {
		t.Fatalf("Cancel(): %v", err)
	}
	queue, err = store.QueueStatus(context.Background(), stored.ExecutionID)
	if err != nil || queue.Waiting || queue.Cancelable {
		t.Fatalf("queue after cancel/error = %#v/%v", queue, err)
	}
	for _, key := range []string{store.queueKey(), store.spaceQueueKey(command.Identity.SpaceID), store.userQueueKey(command.Identity.SpaceID, command.Identity.UserID)} {
		value, err := store.client.Get(context.Background(), key).Result()
		if err != nil || value != "0" {
			t.Fatalf("queue counter %q/error = %q/%v", key, value, err)
		}
	}
}

func TestRedisStoreCancelIsIdempotentAcrossRaces(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	stored, _, err := store.Accept(context.Background(), validStoredCommand(t, "operation-cancel-race", "cancel race request"))
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			errors <- store.Cancel(context.Background(), stored.ExecutionID)
		}()
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("Cancel() error = %v", err)
		}
	}
	result, err := store.Get(context.Background(), stored.ExecutionID)
	if err != nil || result.State != infrasandbox.ExecutionStatusCanceled {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
}

func TestRedisStoreEnforcesSpaceAndUserQueueLimits(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	store.maxQueueDepth, store.perSpaceQueueDepth, store.perUserQueueDepth = 4, 2, 1
	first := validStoredCommand(t, "operation-space-1", "space one")
	if _, _, err := store.Accept(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Accept(context.Background(), validStoredCommand(t, "operation-user-2", "same user")); err == nil {
		t.Fatal("Accept() unexpectedly exceeded per-user depth")
	}
	second := validStoredCommand(t, "operation-space-2", "second user")
	second.Identity.UserID = 13
	if _, _, err := store.Accept(context.Background(), second); err != nil {
		t.Fatalf("second user: %v", err)
	}
	third := validStoredCommand(t, "operation-space-3", "third user")
	third.Identity.UserID = 14
	if _, _, err := store.Accept(context.Background(), third); err == nil {
		t.Fatal("Accept() unexpectedly exceeded per-space depth")
	}
}

func TestRedisStoreCompletesExecutionWithBoundedResultProjection(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	command := recoverableStoredCommand(t, "operation-complete-result", 11, 12, "project-result")
	stored, _, err := store.Accept(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), stored.ExecutionID, ExecutionStateRunning); err != nil {
		t.Fatal(err)
	}
	exitCode := 0
	result, err := store.Complete(context.Background(), infrasandbox.ExecuteResult{ExecutionID: stored.ExecutionID, Status: infrasandbox.ExecutionStatusSucceeded, ExitCode: &exitCode, Stdout: "approved output"})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if result.State != infrasandbox.ExecutionStatusSucceeded || result.Result.ExitCode == nil || *result.Result.ExitCode != 0 || result.Result.Stdout != "approved output" {
		t.Fatalf("stored result = %#v", result)
	}
	status, err := store.Status(context.Background(), stored.ExecutionID)
	if err != nil || status.Status != infrasandbox.ExecutionStatusSucceeded || status.ExitCode == nil || *status.ExitCode != 0 || status.Stdout != "approved output" {
		t.Fatalf("status/error = %#v/%v", status, err)
	}
	lookup, err := store.Lookup(context.Background(), infrasandbox.ExecutionLookupRequest{Scope: command.Scope, WorkloadKind: command.WorkloadKind, OperationID: command.IdempotencyKey, RequestDigest: mustExecutionDigest(t, command.RawBody)})
	if err != nil || lookup.Status != infrasandbox.ExecutionLookupFound || lookup.Execution.Stdout != "approved output" {
		t.Fatalf("lookup/error = %#v/%v", lookup, err)
	}
}

func TestRedisStoreRejectsOversizedCompletionOutput(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	command := recoverableStoredCommand(t, "operation-complete-oversized", 11, 12, "project-result")
	stored, _, err := store.Accept(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), stored.ExecutionID, ExecutionStateRunning); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Complete(context.Background(), infrasandbox.ExecuteResult{ExecutionID: stored.ExecutionID, Status: infrasandbox.ExecutionStatusFailed, Stdout: strings.Repeat("x", maxExecutionResultBytes+1)}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("Complete() error = %v, want protocol error", err)
	}
}

func TestRedisStoreAppliesNewSchedulerQueueLimitsForFutureAdmissions(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 2
	settings.GlobalQueueDepth = 1
	settings.PerSpaceQueueDepth = 1
	settings.PerUserQueueDepth = 1
	if err := store.ApplySchedulerSettings(context.Background(), settings); err != nil {
		t.Fatalf("ApplySchedulerSettings() error = %v", err)
	}
	if _, _, err := store.Accept(context.Background(), recoverableStoredCommand(t, "queue-limit-one", 11, 12, "project-result")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Accept(context.Background(), recoverableStoredCommand(t, "queue-limit-two", 13, 14, "project-result")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Accept() error = %v, want capacity rejection", err)
	}
}

func TestRedisStoreAppliesMaxOutstandingAsConservativeAdmissionCap(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	settings := domainsandbox.DefaultSchedulerSettings()
	settings.Version = 2
	settings.MaxOutstanding = 1
	if err := store.ApplySchedulerSettings(context.Background(), settings); err != nil {
		t.Fatalf("ApplySchedulerSettings() error = %v", err)
	}
	if _, _, err := store.Accept(context.Background(), recoverableStoredCommand(t, "outstanding-one", 11, 12, "project-result")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Accept(context.Background(), recoverableStoredCommand(t, "outstanding-two", 13, 14, "project-result")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Accept() error = %v, want max-outstanding rejection", err)
	}
}

func TestRedisStoreRecoveryReturnsOnlyNonTerminalExecutions(t *testing.T) {
	store, _ := newRedisStoreFixture(t, "key-1")
	store.maxQueueDepth, store.maxOutstanding, store.perSpaceQueueDepth, store.perUserQueueDepth = 3, 3, 3, 3
	accepted, _, err := store.Accept(context.Background(), recoverableStoredCommand(t, "operation-recover-accepted", 11, 12, "project-accepted"))
	if err != nil {
		t.Fatal(err)
	}
	running, _, err := store.Accept(context.Background(), recoverableStoredCommand(t, "operation-recover-running", 12, 13, "project-running"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), running.ExecutionID, ExecutionStateRunning); err != nil {
		t.Fatal(err)
	}
	terminal, _, err := store.Accept(context.Background(), recoverableStoredCommand(t, "operation-recover-terminal", 13, 14, "project-terminal"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), terminal.ExecutionID, infrasandbox.ExecutionStatusCanceled); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.Recover(context.Background())
	if err != nil || len(recovered) != 2 {
		t.Fatalf("recovered/error = %#v/%v", recovered, err)
	}
	if recovered[0].ExecutionID != accepted.ExecutionID || recovered[1].ExecutionID != running.ExecutionID {
		t.Fatalf("recovery order = %#v", recovered)
	}
}

func TestRedisStoreRecoveryRehydratesOnlyAuthenticatedExecutionCommands(t *testing.T) {
	store, server := newRedisStoreFixture(t, "key-1")
	acceptedCommand := recoverableStoredCommand(t, "operation-recover-command-accepted", 11, 12, "project-accepted")
	accepted, _, err := store.Accept(context.Background(), acceptedCommand)
	if err != nil {
		t.Fatal(err)
	}
	runningCommand := recoverableStoredCommand(t, "operation-recover-command-running", 12, 13, "project-running")
	running, _, err := store.Accept(context.Background(), runningCommand)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), running.ExecutionID, ExecutionStateRunning); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.RecoverExecutions(context.Background())
	if err != nil || len(recovered) != 2 {
		t.Fatalf("recovered/error = %#v/%v", recovered, err)
	}
	if recovered[0].Stored.ExecutionID != accepted.ExecutionID || recovered[0].Command.IdempotencyKey != acceptedCommand.IdempotencyKey || recovered[0].Command.Identity.SpaceID != acceptedCommand.Identity.SpaceID ||
		recovered[1].Stored.ExecutionID != running.ExecutionID || recovered[1].Command.IdempotencyKey != runningCommand.IdempotencyKey {
		t.Fatalf("recovered commands = %#v", recovered)
	}
	if recovered[0].Command.Identity.ExecutionID != acceptedCommand.Identity.ExecutionID ||
		recovered[1].Command.Identity.ExecutionID != runningCommand.Identity.ExecutionID ||
		recovered[0].Command.Identity.ExecutionID == recovered[0].Stored.ExecutionID ||
		recovered[1].Command.Identity.ExecutionID == recovered[1].Stored.ExecutionID {
		t.Fatalf("recovery did not preserve the signed business execution ID separately: %#v", recovered)
	}
	if err := server.Set(store.executionKey(accepted.ExecutionID), `{"schema":"coze.sandbox.runner_encrypted_execution.v1","key_id":"key-1","deployment_id":"invalid","nonce":"bad","ciphertext":"bad"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RecoverExecutions(context.Background()); err != nil {
		t.Fatalf("stale tampered record should be skipped, got %v", err)
	}
}

func TestRedisStoreUsesOnlyDeploymentAndHashedTenantKeyMaterial(t *testing.T) {
	store, server := newRedisStoreFixture(t, "key-1")
	command := validStoredCommand(t, "operation-key-privacy", "private body")
	if _, _, err := store.Accept(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, "runner-dev-1") || strings.Contains(key, "space-11") || strings.Contains(key, "user-12") || strings.Contains(key, "project-22") {
			t.Fatalf("redis key leaks deployment or tenant material: %q", key)
		}
	}
}

func TestRedisStoreRecordTTLNeverOutlivesExecutionDeadline(t *testing.T) {
	store, server := newRedisStoreFixture(t, "key-1")
	command := validStoredCommand(t, "operation-deadline-ttl", "deadline ttl request")
	command.Deadline = time.Now().Add(2 * time.Second).UTC()
	stored, _, err := store.Accept(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	server.FastForward(3 * time.Second)
	if _, err := store.Get(context.Background(), stored.ExecutionID); err == nil {
		t.Fatal("Get() unexpectedly retained a record after its execution deadline")
	}
}

func TestRedisStoreQueueCountersDoNotExpireBeforeLongestOutstandingExecution(t *testing.T) {
	store, server := newRedisStoreFixture(t, "key-1")
	store.maxQueueDepth, store.perSpaceQueueDepth, store.perUserQueueDepth = 3, 3, 3
	long := validStoredCommand(t, "operation-long", "long request")
	long.Deadline = time.Now().Add(time.Minute).UTC()
	if _, _, err := store.Accept(context.Background(), long); err != nil {
		t.Fatal(err)
	}
	short := validStoredCommand(t, "operation-short", "short request")
	short.Identity.UserID = 13
	short.Deadline = time.Now().Add(time.Second).UTC()
	if _, _, err := store.Accept(context.Background(), short); err != nil {
		t.Fatal(err)
	}
	if ttl := server.TTL(store.queueKey()); ttl <= 0 {
		t.Fatalf("queue depth TTL = %v, want a finite TTL", ttl)
	}
	server.FastForward(2 * time.Second)
	if value, err := server.Get(store.queueKey()); err != nil || value != "2" {
		t.Fatalf("queue depth/error = %q/%v", value, err)
	}
}

func mustExecutionDigest(t *testing.T, body []byte) infrasandbox.ExecutionRequestDigest {
	t.Helper()
	digest := sha256.Sum256(body)
	value, err := infrasandbox.ExecutionRequestDigestFromBytes(digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func newRedisStoreFixture(t *testing.T, activeKeyID string) (*RedisStore, *miniredis.Miniredis) {
	t.Helper()
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	store, err := NewRedisStore(redisimpl.NewWithAddrAndPassword(server.Addr(), ""), RedisStoreConfig{
		DeploymentID: "runner-dev-1", ActiveKeyID: activeKeyID,
		Keys:          map[string]string{"key-1": "11111111111111111111111111111111", "old": "0123456789abcdef0123456789abcdef", "new": "abcdef0123456789abcdef0123456789"},
		MaxQueueDepth: 2, PerSpaceQueueDepth: 2, PerUserQueueDepth: 2, RecordTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return store, server
}

func validStoredCommand(t *testing.T, operationID, body string) ExecuteCommand {
	t.Helper()
	digest := sha256.Sum256([]byte(body))
	return ExecuteCommand{IdempotencyKey: operationID, Scope: "appdev", WorkloadKind: "appdev", Entrypoint: "appdev/runtime", RawBody: []byte(body), Deadline: time.Now().Add(time.Minute), Identity: sandboxidentity.Request{SpaceID: 11, UserID: 12, ProjectID: "project-22", ExecutionID: "exec-context", RequestDigest: digest[:]}}
}

func recoverableStoredCommand(t *testing.T, operationID string, spaceID, userID int64, projectID string) ExecuteCommand {
	t.Helper()
	raw := []byte(strings.Replace(validExecuteWireBody(t), "runner-test-operation", operationID, 1))
	command, err := parseExecute(raw)
	if err != nil {
		t.Fatalf("parse recoverable command: %v", err)
	}
	digest := sha256.Sum256(raw)
	command.Identity = sandboxidentity.Request{SpaceID: spaceID, UserID: userID, ProjectID: projectID, ExecutionID: "exec-context", RequestDigest: digest[:]}
	return command
}
