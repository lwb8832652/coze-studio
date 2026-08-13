// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	redisimpl "github.com/coze-dev/coze-studio/backend/infra/cache/impl/redis"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestSessionOperationStoreEncryptsBoundedMetadataInIndependentNamespace(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	now := store.now()
	digest := sha256.Sum256([]byte("secret command /mnt/user-data/space/user/thread/workspace opaque-shell"))
	input := SessionOperationInput{
		SessionID: "session-a", OperationID: "operation-a", UserID: 42,
		Kind: SessionOperationExec, RequestDigest: digest[:], Weight: 1,
		Deadline: now.Add(time.Minute),
	}
	accepted, replayed, err := store.Accept(context.Background(), input)
	if err != nil || replayed || accepted.State != SessionOperationAccepted {
		t.Fatalf("Accept() = %#v, %t, %v", accepted, replayed, err)
	}
	queued, err := store.MarkQueued(context.Background(), input.SessionID, input.OperationID)
	if err != nil || queued.State != SessionOperationQueued {
		t.Fatalf("MarkQueued() = %#v, %v", queued, err)
	}
	queuedAgain, err := store.MarkQueued(context.Background(), input.SessionID, input.OperationID)
	if err != nil || queuedAgain.State != SessionOperationQueued {
		t.Fatalf("repeated MarkQueued() = %#v, %v", queuedAgain, err)
	}
	queue, err := server.List(store.queueKey())
	if err != nil || len(queue) != 1 {
		t.Fatalf("queue after repeated MarkQueued() = %v, %v", queue, err)
	}
	replay, replayed, err := store.Accept(context.Background(), input)
	if err != nil || !replayed || replay.State != SessionOperationQueued {
		t.Fatalf("replay = %#v, %t, %v", replay, replayed, err)
	}

	for _, key := range server.Keys() {
		if !strings.HasPrefix(key, "coze:sandbox:core:") || strings.Contains(key, "sandbox:runner:") {
			t.Fatalf("Core key escaped independent namespace: %q", key)
		}
		value := ""
		switch server.Type(key) {
		case "string":
			value, _ = server.Get(key)
		case "list":
			values, _ := server.List(key)
			value = strings.Join(values, "\n")
		}
		for _, secret := range []string{"secret command", "/mnt/user-data", "opaque-shell", "session-a", "operation-a", `"user_id":42`, ":42:"} {
			if strings.Contains(key, secret) || strings.Contains(value, secret) {
				t.Fatalf("plaintext %q found in Redis entry %q", secret, key)
			}
		}
		if len(value) > maxSessionOperationEnvelopeBytes {
			t.Fatalf("Redis value is not bounded: key=%q bytes=%d", key, len(value))
		}
	}

	conflict := input
	other := sha256.Sum256([]byte("different request"))
	conflict.RequestDigest = other[:]
	if _, _, err := store.Accept(context.Background(), conflict); !errors.Is(err, ErrSessionOperationConflict) {
		t.Fatalf("digest conflict error = %v", err)
	}
	server.FastForward(3 * time.Minute)
	if _, err := store.Get(context.Background(), input.SessionID, input.OperationID); err == nil {
		t.Fatal("expired operation remained readable")
	}
}

func TestSessionOperationStoreConcurrentAcceptHasOneRecordAndOneActiveEntry(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	input := sessionOperationInput(store.now(), "concurrent", 42)
	const callers = 32
	var group sync.WaitGroup
	results := make(chan struct {
		record   SessionOperationRecord
		replayed bool
		err      error
	}, callers)
	for range callers {
		group.Add(1)
		go func() {
			defer group.Done()
			record, replayed, err := store.Accept(context.Background(), input)
			results <- struct {
				record   SessionOperationRecord
				replayed bool
				err      error
			}{record: record, replayed: replayed, err: err}
		}()
	}
	group.Wait()
	close(results)
	accepted := 0
	for result := range results {
		if result.err != nil || result.record.OperationID != input.OperationID {
			t.Fatalf("Accept() = %#v, %t, %v", result.record, result.replayed, result.err)
		}
		if !result.replayed {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted count = %d, want 1", accepted)
	}
	active, err := server.List(store.activeOperationsKey())
	if err != nil || len(active) != 1 {
		t.Fatalf("active entries = %v, %v", active, err)
	}
	recordKeys := 0
	for _, key := range server.Keys() {
		if strings.Contains(key, ":record:") {
			recordKeys++
		}
	}
	if recordKeys != 1 {
		t.Fatalf("record key count = %d, want 1", recordKeys)
	}
}

func TestSessionOperationStoreRecordCollisionDoesNotPublishLookupOrQueue(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	constant := make([]byte, 24)
	for index := range constant {
		constant[index] = 0x5a
	}
	store.random = &repeatingReader{value: constant}
	first := sessionOperationInput(store.now(), "collision-first", 42)
	second := sessionOperationInput(store.now(), "collision-second", 43)
	if _, _, err := store.Accept(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Accept(context.Background(), second); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("collision Accept() error = %v", err)
	}
	if _, err := server.Get(store.operationLookupKey(second.SessionID, second.OperationID)); err == nil {
		t.Fatal("record collision published a lookup")
	}
	active, err := server.List(store.activeOperationsKey())
	if err != nil || len(active) != 1 {
		t.Fatalf("active entries after collision = %v, %v", active, err)
	}
}

func TestSessionOperationStoreCoreListsAndCountersHaveNonShorteningTTL(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	short := sessionOperationInput(store.now(), "ttl-short", 51)
	short.Deadline = store.now().Add(30 * time.Second)
	long := sessionOperationInput(store.now(), "ttl-long", 52)
	long.Deadline = store.now().Add(90 * time.Second)
	for _, input := range []SessionOperationInput{short, long} {
		if _, _, err := store.Accept(ctx, input); err != nil {
			t.Fatal(err)
		}
		if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
			t.Fatal(err)
		}
		if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, 2, 1); err != nil || !started {
			t.Fatalf("ClaimQueued(%q) = %t, %v", input.OperationID, started, err)
		}
	}
	for _, key := range []string{store.activeOperationsKey(), store.activeWeightKey(), store.activeUserKey(redisHash("51")), store.activeUserKey(redisHash("52"))} {
		if ttl := server.TTL(key); ttl <= 0 {
			t.Fatalf("key %q has no bounded TTL: %v", key, ttl)
		}
	}
	weightTTL := server.TTL(store.activeWeightKey())
	if weightTTL < 80*time.Second {
		t.Fatalf("active weight TTL = %v, want long operation TTL", weightTTL)
	}
	if _, err := store.Complete(ctx, short.SessionID, short.OperationID, successfulSessionOperationCompletion()); err != nil {
		t.Fatal(err)
	}
	if ttl := server.TTL(store.activeWeightKey()); ttl < 80*time.Second {
		t.Fatalf("short completion shortened shared counter TTL to %v", ttl)
	}
}

func TestSessionOperationStoreImmediateQueuedCancelRetainsTerminalLookupTTL(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "cancel-terminal-ttl", 53)
	input.Deadline = store.now().Add(10 * time.Second)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	server.FastForward(9 * time.Second)
	if canceled, changed, err := store.CancelQueued(ctx, input.SessionID, input.OperationID); err != nil || !changed || canceled.State != SessionOperationCanceled {
		t.Fatalf("CancelQueued() = %#v, %t, %v", canceled, changed, err)
	}
	payload, _, err := store.loadOperation(ctx, input.SessionID, input.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	recordTTL := server.TTL(store.recordKey(payload.RecordID))
	lookupTTL := server.TTL(store.operationLookupKey(input.SessionID, input.OperationID))
	if recordTTL != store.recordTTL || lookupTTL != recordTTL {
		t.Fatalf("cancel terminal TTLs record=%v lookup=%v want=%v", recordTTL, lookupTTL, store.recordTTL)
	}
}

func TestSessionOperationStoreRecoveryFencesEveryPayloadlessActiveOperationUnknown(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	acceptedInput := sessionOperationInput(store.now(), "accepted", 10)
	queuedInput := sessionOperationInput(store.now(), "queued", 11)
	runningInput := sessionOperationInput(store.now(), "running", 12)
	for _, input := range []SessionOperationInput{acceptedInput, queuedInput, runningInput} {
		if _, _, err := store.Accept(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.MarkQueued(ctx, queuedInput.SessionID, queuedInput.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, runningInput.SessionID, runningInput.OperationID); err != nil {
		t.Fatal(err)
	}
	claimed, started, err := store.ClaimQueued(ctx, queuedInput.SessionID, queuedInput.OperationID, 2, 1)
	if err != nil || !started || claimed.State != SessionOperationRunning {
		t.Fatalf("ClaimQueued(queue head) = %#v, %t, %v", claimed, started, err)
	}
	if _, err := store.Complete(ctx, queuedInput.SessionID, queuedInput.OperationID, successfulSessionOperationCompletion()); err != nil {
		t.Fatal(err)
	}
	claimed, started, err = store.ClaimQueued(ctx, runningInput.SessionID, runningInput.OperationID, 2, 1)
	if err != nil || !started || claimed.State != SessionOperationRunning {
		t.Fatalf("ClaimQueued() = %#v, %t, %v", claimed, started, err)
	}

	recovered, err := store.RecoverOperations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 0 {
		t.Fatalf("RecoverOperations() = %#v", recovered)
	}
	for _, input := range []SessionOperationInput{acceptedInput, runningInput} {
		record, getErr := store.Get(ctx, input.SessionID, input.OperationID)
		if getErr != nil || record.State != SessionOperationUnknown {
			t.Fatalf("%s after recovery = %#v, %v", input.OperationID, record, getErr)
		}
	}
	if _, started, err := store.ClaimQueued(ctx, runningInput.SessionID, runningInput.OperationID, 2, 1); !errors.Is(err, ErrSessionOperationConflict) || started {
		t.Fatalf("running replay claim = %t, %v", started, err)
	}
	completion := successfulSessionOperationCompletion()
	if _, err := store.Complete(ctx, runningInput.SessionID, runningInput.OperationID, completion); !errors.Is(err, ErrSessionOperationConflict) {
		t.Fatalf("late completion error = %v", err)
	}
}

func TestSessionOperationStoreRecoveryFailsClosedForMissingOrCorruptActiveRecord(t *testing.T) {
	for _, test := range []struct {
		name    string
		corrupt func(*miniredis.Miniredis, string)
	}{
		{name: "missing", corrupt: func(server *miniredis.Miniredis, key string) { server.Del(key) }},
		{name: "corrupt", corrupt: func(server *miniredis.Miniredis, key string) { server.Set(key, `{"schema":"tampered"}`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, server := newSessionRedisStoreFixture(t)
			input := sessionOperationInput(store.now(), "recover-"+test.name, 13)
			if _, _, err := store.Accept(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			active, err := server.List(store.activeOperationsKey())
			if err != nil || len(active) != 1 {
				t.Fatalf("active records = %v, %v", active, err)
			}
			test.corrupt(server, store.recordKey(active[0]))
			if recovered, err := store.RecoverOperations(context.Background()); !errors.Is(err, ErrUnavailable) || len(recovered) != 0 {
				t.Fatalf("RecoverOperations() = %#v, %v", recovered, err)
			}
		})
	}
}

func TestSessionOperationStoreRecoveryFailsClosedWhenActiveIndexExceedsScanBound(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	for index := 0; index <= store.maxQueueDepth; index++ {
		if _, err := server.RPush(store.activeOperationsKey(), strings.Repeat("X", 31)+string(rune('A'+index%26))); err != nil {
			t.Fatal(err)
		}
	}
	if recovered, err := store.RecoverOperations(context.Background()); !errors.Is(err, ErrUnavailable) || len(recovered) != 0 {
		t.Fatalf("RecoverOperations(oversized index) = %#v, %v", recovered, err)
	}
}

func TestSessionOperationStoreDropsMissingQueueHeadAtomicallyButFailsClosedForCorruption(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		store, server := newSessionRedisStoreFixture(t)
		first := sessionOperationInput(store.now(), "stale-head", 31)
		second := sessionOperationInput(store.now(), "live-after-stale", 32)
		for _, input := range []SessionOperationInput{first, second} {
			if _, _, err := store.Accept(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			if _, err := store.MarkQueued(context.Background(), input.SessionID, input.OperationID); err != nil {
				t.Fatal(err)
			}
		}
		queue, err := server.List(store.queueKey())
		if err != nil || len(queue) != 2 {
			t.Fatalf("queue = %v, %v", queue, err)
		}
		server.Del(store.recordKey(queue[0]))
		if record, started, err := store.ClaimQueued(context.Background(), second.SessionID, second.OperationID, 2, 2); err != nil || !started || record.State != SessionOperationRunning {
			t.Fatalf("ClaimQueued(after missing head) = %#v, %t, %v", record, started, err)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		store, server := newSessionRedisStoreFixture(t)
		first := sessionOperationInput(store.now(), "corrupt-head", 33)
		second := sessionOperationInput(store.now(), "live-after-corrupt", 34)
		for _, input := range []SessionOperationInput{first, second} {
			if _, _, err := store.Accept(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			if _, err := store.MarkQueued(context.Background(), input.SessionID, input.OperationID); err != nil {
				t.Fatal(err)
			}
		}
		queue, err := server.List(store.queueKey())
		if err != nil || len(queue) != 2 {
			t.Fatalf("queue = %v, %v", queue, err)
		}
		server.Set(store.recordKey(queue[0]), `{"schema":"tampered"}`)
		if _, started, err := store.ClaimQueued(context.Background(), second.SessionID, second.OperationID, 2, 2); !errors.Is(err, ErrUnavailable) || started {
			t.Fatalf("ClaimQueued(after corrupt head) = %t, %v", started, err)
		}
		remaining, err := server.List(store.queueKey())
		if err != nil || len(remaining) != 2 || remaining[0] != queue[0] {
			t.Fatalf("corrupt head was not retained fail closed: %v, %v", remaining, err)
		}
	})
}

func TestSessionOperationStoreDropsExpiredQueueHeadWithoutPoisoningLiveWaiter(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	live := sessionOperationInput(store.now(), "live-after-expired", 35)
	if _, _, err := store.Accept(ctx, live); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, live.SessionID, live.OperationID); err != nil {
		t.Fatal(err)
	}
	expiredRecordID := strings.Repeat("Z", 32)
	if _, err := server.Lpush(store.queueKey(), expiredRecordID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.RPush(store.activeOperationsKey(), expiredRecordID); err != nil {
		t.Fatal(err)
	}
	if record, started, err := store.ClaimQueued(ctx, live.SessionID, live.OperationID, 2, 2); err != nil || !started || record.State != SessionOperationRunning {
		t.Fatalf("ClaimQueued(after expired head) = %#v, %t, %v", record, started, err)
	}
	active, err := server.List(store.activeOperationsKey())
	if err != nil {
		t.Fatal(err)
	}
	for _, recordID := range active {
		if recordID == expiredRecordID {
			t.Fatalf("expired record remained active: %v", active)
		}
	}
}

func TestSessionOperationStorePersistsEncryptedTerminalResultForGetAndReplay(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "persist-result", 41)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, 2, 2); err != nil || !started {
		t.Fatalf("ClaimQueued() = %t, %v", started, err)
	}
	result := []byte(`{"stdout":"terminal-result-secret","exit_code":0}`)
	digest := sha256.Sum256(result)
	completed, err := store.Complete(ctx, input.SessionID, input.OperationID, SessionOperationCompletion{
		State: SessionOperationSucceeded, Result: result, ResultDigest: digest[:],
	})
	if err != nil || string(completed.Result) != string(result) {
		t.Fatalf("Complete() = %#v, %v", completed, err)
	}
	observed, err := store.Get(ctx, input.SessionID, input.OperationID)
	if err != nil || string(observed.Result) != string(result) {
		t.Fatalf("Get() = %#v, %v", observed, err)
	}
	replayed, replay, err := store.Accept(ctx, input)
	if err != nil || !replay || string(replayed.Result) != string(result) {
		t.Fatalf("Accept(replay) = %#v, %t, %v", replayed, replay, err)
	}
	for _, key := range server.Keys() {
		if server.Type(key) != "string" {
			continue
		}
		value, _ := server.Get(key)
		if strings.Contains(value, "terminal-result-secret") {
			t.Fatalf("terminal result leaked in plaintext at %q", key)
		}
	}
	if strings.Contains(completed.String(), "terminal-result-secret") || strings.Contains(completed.GoString(), "terminal-result-secret") {
		t.Fatal("terminal result leaked through record formatting")
	}
	payload, _, loadErr := store.loadOperation(ctx, input.SessionID, input.OperationID)
	if loadErr != nil || server.TTL(store.resultKey(payload.RecordID)) <= 0 {
		t.Fatalf("result TTL missing: %v", loadErr)
	}
}

func TestSessionOperationStoreTerminalWriteResetsRecordResultAndLookupToSameBoundedTTL(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "terminal-ttl", 44)
	input.Deadline = store.now().Add(10 * time.Second)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, 2, 2); err != nil || !started {
		t.Fatal(err)
	}
	server.FastForward(9 * time.Second)
	result := []byte(`{"terminal":true}`)
	digest := sha256.Sum256(result)
	if _, err := store.Complete(ctx, input.SessionID, input.OperationID, SessionOperationCompletion{
		State: SessionOperationSucceeded, Result: result, ResultDigest: digest[:],
	}); err != nil {
		t.Fatal(err)
	}
	payload, _, err := store.loadOperation(ctx, input.SessionID, input.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	recordTTL := server.TTL(store.recordKey(payload.RecordID))
	resultTTL := server.TTL(store.resultKey(payload.RecordID))
	lookupTTL := server.TTL(store.operationLookupKey(input.SessionID, input.OperationID))
	if recordTTL != store.recordTTL || resultTTL != recordTTL || lookupTTL != recordTTL {
		t.Fatalf("terminal TTLs record=%v result=%v lookup=%v want=%v", recordTTL, resultTTL, lookupTTL, store.recordTTL)
	}
	server.FastForward(2 * time.Second)
	if observed, err := store.Get(ctx, input.SessionID, input.OperationID); err != nil || string(observed.Result) != string(result) {
		t.Fatalf("Get(after original deadline TTL) = %#v, %v", observed, err)
	}
}

func TestSessionOperationStoreTerminalWriteFailsClosedWhenLookupIsMissing(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "terminal-missing-lookup", 45)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, 2, 2); err != nil || !started {
		t.Fatal(err)
	}
	server.Del(store.operationLookupKey(input.SessionID, input.OperationID))
	result := []byte(`{"terminal":true}`)
	digest := sha256.Sum256(result)
	if _, err := store.Complete(ctx, input.SessionID, input.OperationID, SessionOperationCompletion{
		State: SessionOperationSucceeded, Result: result, ResultDigest: digest[:],
	}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Complete(missing lookup) = %v", err)
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, ":result:") {
			t.Fatalf("result was published without lookup: %q", key)
		}
	}
}

func TestSessionOperationStoreRecoveryResetsUnknownRecordAndLookupTTL(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "recover-terminal-ttl", 46)
	input.Deadline = store.now().Add(10 * time.Second)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	server.FastForward(9 * time.Second)
	if recovered, err := store.RecoverOperations(ctx); err != nil || len(recovered) != 0 {
		t.Fatalf("RecoverOperations() = %#v, %v", recovered, err)
	}
	payload, _, err := store.loadOperation(ctx, input.SessionID, input.OperationID)
	if err != nil || payload.State != SessionOperationUnknown {
		t.Fatalf("loadOperation() = %#v, %v", payload, err)
	}
	recordTTL := server.TTL(store.recordKey(payload.RecordID))
	lookupTTL := server.TTL(store.operationLookupKey(input.SessionID, input.OperationID))
	if recordTTL != store.recordTTL || lookupTTL != recordTTL {
		t.Fatalf("recovery TTLs record=%v lookup=%v want=%v", recordTTL, lookupTTL, store.recordTTL)
	}
}

func TestSessionOperationStoreRejectsResultDigestMismatchBeforeTerminalWrite(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "result-digest-mismatch", 43)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, 2, 2); err != nil || !started {
		t.Fatal(err)
	}
	wrong := sha256.Sum256([]byte("wrong"))
	if _, err := store.Complete(ctx, input.SessionID, input.OperationID, SessionOperationCompletion{
		State: SessionOperationSucceeded, Result: []byte(`{"actual":true}`), ResultDigest: wrong[:],
	}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("Complete(digest mismatch) = %v", err)
	}
	if observed, err := store.Get(ctx, input.SessionID, input.OperationID); err != nil || observed.State != SessionOperationRunning {
		t.Fatalf("Get(after rejected completion) = %#v, %v", observed, err)
	}
}

func TestSessionOperationStoreRejectsSucceededCompletionWithoutRecoverableResult(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "result-required", 47)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, 2, 2); err != nil || !started {
		t.Fatal(err)
	}
	if _, err := store.Complete(ctx, input.SessionID, input.OperationID, SessionOperationCompletion{State: SessionOperationSucceeded}); !errors.Is(err, ErrProtocol) {
		t.Fatalf("Complete(success without result) = %v", err)
	}
}

func TestSessionOperationStoreFailsClosedWhenEncryptedResultIsMissingOrTampered(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*miniredis.Miniredis, string)
	}{
		{name: "missing", mutate: func(server *miniredis.Miniredis, key string) { server.Del(key) }},
		{name: "tampered", mutate: func(server *miniredis.Miniredis, key string) { server.Set(key, `{"schema":"tampered"}`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, server := newSessionRedisStoreFixture(t)
			ctx := context.Background()
			input := sessionOperationInput(store.now(), "result-"+test.name, 42)
			if _, _, err := store.Accept(ctx, input); err != nil {
				t.Fatal(err)
			}
			if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
				t.Fatal(err)
			}
			if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, 2, 2); err != nil || !started {
				t.Fatal(err)
			}
			result := []byte(`{"value":"persisted"}`)
			digest := sha256.Sum256(result)
			if _, err := store.Complete(ctx, input.SessionID, input.OperationID, SessionOperationCompletion{State: SessionOperationSucceeded, Result: result, ResultDigest: digest[:]}); err != nil {
				t.Fatal(err)
			}
			payload, _, err := store.loadOperation(ctx, input.SessionID, input.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(server, store.resultKey(payload.RecordID))
			if record, err := store.Get(ctx, input.SessionID, input.OperationID); !errors.Is(err, ErrUnavailable) || len(record.Result) != 0 {
				t.Fatalf("Get() = %#v, %v", record, err)
			}
		})
	}
}

func TestSessionOperationStoreCancelQueuedIsAtomicAndLateCompletionCannotWin(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "cancel", 20)
	if _, _, err := store.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
		t.Fatal(err)
	}
	canceled, changed, err := store.CancelQueued(ctx, input.SessionID, input.OperationID)
	if err != nil || !changed || canceled.State != SessionOperationCanceled {
		t.Fatalf("CancelQueued() = %#v, %t, %v", canceled, changed, err)
	}
	if _, started, err := store.ClaimQueued(ctx, input.SessionID, input.OperationID, 2, 1); !errors.Is(err, ErrSessionOperationConflict) || started {
		t.Fatalf("ClaimQueued(canceled) = %t, %v", started, err)
	}
	if _, err := store.Complete(ctx, input.SessionID, input.OperationID, successfulSessionOperationCompletion()); !errors.Is(err, ErrSessionOperationConflict) {
		t.Fatalf("late Complete() = %v", err)
	}
	if recovered, err := store.RecoverOperations(ctx); err != nil || len(recovered) != 0 {
		t.Fatalf("RecoverOperations() = %#v, %v", recovered, err)
	}
}

func TestSessionOperationStoreSerializesRunningOperationsPerSession(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	secondStore, _ := newSessionRedisStoreFixtureWithServer(t, server)
	ctx := context.Background()
	first := sessionOperationInput(store.now(), "serial-first", 201)
	second := sessionOperationInput(store.now(), "serial-second", 202)
	second.SessionID = first.SessionID
	parallel := sessionOperationInput(store.now(), "parallel", 203)
	for _, input := range []SessionOperationInput{first, second, parallel} {
		if _, _, err := store.Accept(ctx, input); err != nil {
			t.Fatal(err)
		}
		if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
			t.Fatal(err)
		}
	}
	type claimResult struct {
		input   SessionOperationInput
		record  SessionOperationRecord
		started bool
		err     error
	}
	results := make(chan claimResult, 2)
	for index, input := range []SessionOperationInput{first, second} {
		candidateStore := store
		if index == 1 {
			candidateStore = secondStore
		}
		go func() {
			record, started, err := candidateStore.ClaimQueued(ctx, input.SessionID, input.OperationID, 4, 4)
			results <- claimResult{input: input, record: record, started: started, err: err}
		}()
	}
	var running, blocked SessionOperationInput
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent ClaimQueued(%q) error = %v", result.input.OperationID, result.err)
		}
		if result.started {
			if running.OperationID != "" || result.record.State != SessionOperationRunning {
				t.Fatalf("unexpected running claim = %#v", result)
			}
			running = result.input
		} else {
			if blocked.OperationID != "" || result.record.State != SessionOperationQueued {
				t.Fatalf("unexpected blocked claim = %#v", result)
			}
			blocked = result.input
		}
	}
	if running.OperationID == "" || blocked.OperationID == "" {
		t.Fatalf("same-session claims did not serialize: running=%#v blocked=%#v", running, blocked)
	}
	if _, started, err := store.ClaimQueued(ctx, parallel.SessionID, parallel.OperationID, 4, 4); err != nil || !started {
		t.Fatalf("different-session ClaimQueued() = %t, %v", started, err)
	}
	if ttl := server.TTL(store.activeSessionKey(first.SessionID)); ttl <= 0 {
		t.Fatalf("per-session active key has no bounded TTL: %v", ttl)
	}
	if _, err := store.Complete(ctx, running.SessionID, running.OperationID, successfulSessionOperationCompletion()); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, blocked.SessionID, blocked.OperationID, 4, 4); err != nil || !started {
		t.Fatalf("same-session ClaimQueued() after finish = %t, %v", started, err)
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, first.SessionID) || strings.Contains(key, parallel.SessionID) {
			t.Fatalf("session identifier leaked in Redis key %q", key)
		}
	}
}

func TestSessionOperationStoreRecoveryFencesQueuedSiblingAndReleasesRunningCounter(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	running := sessionOperationInput(store.now(), "recover-session-running", 211)
	queued := sessionOperationInput(store.now(), "recover-session-queued", 212)
	queued.SessionID = running.SessionID
	for _, input := range []SessionOperationInput{running, queued} {
		if _, _, err := store.Accept(ctx, input); err != nil {
			t.Fatal(err)
		}
		if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
			t.Fatal(err)
		}
	}
	if _, started, err := store.ClaimQueued(ctx, running.SessionID, running.OperationID, 2, 2); err != nil || !started {
		t.Fatalf("ClaimQueued() = %t, %v", started, err)
	}
	if _, err := store.RecoverOperations(ctx); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, queued.SessionID, queued.OperationID, 2, 2); !errors.Is(err, ErrSessionOperationConflict) || started {
		t.Fatalf("ClaimQueued(payloadless queued after recovery) = %t, %v", started, err)
	}
	fresh := sessionOperationInput(store.now(), "recover-session-fresh", 213)
	fresh.SessionID = running.SessionID
	if _, _, err := store.Accept(ctx, fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkQueued(ctx, fresh.SessionID, fresh.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := store.ClaimQueued(ctx, fresh.SessionID, fresh.OperationID, 2, 2); err != nil || !started {
		t.Fatalf("ClaimQueued(fresh after recovery) = %t, %v", started, err)
	}
}

func TestSessionOperationStoreRequestCancelIsCrossReplicaAndFailClosed(t *testing.T) {
	firstStore, server := newSessionRedisStoreFixture(t)
	secondStore, _ := newSessionRedisStoreFixtureWithServer(t, server)
	ctx := context.Background()
	accepted := sessionOperationInput(firstStore.now(), "cancel-accepted-cross-replica", 220)
	if _, _, err := firstStore.Accept(ctx, accepted); err != nil {
		t.Fatal(err)
	}
	acceptedCanceled, immediate, err := secondStore.RequestCancel(ctx, accepted.SessionID, accepted.OperationID)
	if err != nil || !immediate || acceptedCanceled.State != SessionOperationCanceled {
		t.Fatalf("accepted RequestCancel() = %#v, %t, %v", acceptedCanceled, immediate, err)
	}

	queued := sessionOperationInput(firstStore.now(), "cancel-queued-cross-replica", 221)
	if _, _, err := firstStore.Accept(ctx, queued); err != nil {
		t.Fatal(err)
	}
	if _, err := firstStore.MarkQueued(ctx, queued.SessionID, queued.OperationID); err != nil {
		t.Fatal(err)
	}
	canceled, immediate, err := secondStore.RequestCancel(ctx, queued.SessionID, queued.OperationID)
	if err != nil || !immediate || canceled.State != SessionOperationCanceled || canceled.CancelRequested {
		t.Fatalf("queued RequestCancel() = %#v, %t, %v", canceled, immediate, err)
	}
	repeated, immediate, err := firstStore.RequestCancel(ctx, queued.SessionID, queued.OperationID)
	if err != nil || immediate || repeated.State != SessionOperationCanceled {
		t.Fatalf("repeated terminal RequestCancel() = %#v, %t, %v", repeated, immediate, err)
	}
	if queue, listErr := server.List(firstStore.queueKey()); listErr == nil && len(queue) != 0 {
		t.Fatalf("canceled operation remained queued: %v", queue)
	}
	if active, listErr := server.List(firstStore.activeOperationsKey()); listErr == nil && len(active) != 0 {
		t.Fatalf("immediately canceled operation remained active: %v", active)
	}

	running := sessionOperationInput(firstStore.now(), "cancel-running-cross-replica", 222)
	if _, _, err := firstStore.Accept(ctx, running); err != nil {
		t.Fatal(err)
	}
	if _, err := firstStore.MarkQueued(ctx, running.SessionID, running.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, started, err := firstStore.ClaimQueued(ctx, running.SessionID, running.OperationID, 2, 2); err != nil || !started {
		t.Fatalf("ClaimQueued() = %t, %v", started, err)
	}
	requested, immediate, err := secondStore.RequestCancel(ctx, running.SessionID, running.OperationID)
	if err != nil || immediate || requested.State != SessionOperationRunning || !requested.CancelRequested {
		t.Fatalf("running RequestCancel() = %#v, %t, %v", requested, immediate, err)
	}
	observed, err := firstStore.Get(ctx, running.SessionID, running.OperationID)
	if err != nil || !observed.CancelRequested || observed.State != SessionOperationRunning {
		t.Fatalf("Get() after cross-replica cancel = %#v, %v", observed, err)
	}
	if ttl := server.TTL(firstStore.activeSessionKey(running.SessionID)); ttl <= 0 {
		t.Fatalf("cancel request released running session capacity early: %v", ttl)
	}
	finished, err := firstStore.Complete(ctx, running.SessionID, running.OperationID, successfulSessionOperationCompletion())
	if err != nil || finished.State != SessionOperationCanceled || !finished.CancelRequested || len(finished.ResultDigest) != 0 {
		t.Fatalf("late success after cancel request = %#v, %v", finished, err)
	}
	if _, err := firstStore.Complete(ctx, running.SessionID, running.OperationID, successfulSessionOperationCompletion()); !errors.Is(err, ErrSessionOperationConflict) {
		t.Fatalf("late terminal success error = %v", err)
	}
}

func TestCoreSessionSchedulerExposesOperationLookupAndCancellation(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.CoreEnabled = true
	scheduler, err := NewCoreSessionScheduler(CoreSessionSchedulerConfig{Store: store, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	input := sessionOperationInput(store.now(), "scheduler-cancel", 231)
	if _, err := scheduler.Accept(ctx, input); err != nil {
		t.Fatal(err)
	}
	if observed, err := scheduler.Get(ctx, input.SessionID, input.OperationID); err != nil || observed.State != SessionOperationQueued {
		t.Fatalf("Get() = %#v, %v", observed, err)
	}
	if canceled, immediate, err := scheduler.RequestCancel(ctx, input.SessionID, input.OperationID); err != nil || !immediate || canceled.State != SessionOperationCanceled {
		t.Fatalf("RequestCancel() = %#v, %t, %v", canceled, immediate, err)
	}
}

func TestCoreSessionSchedulerEnforcesGlobalWeightAndPerUserActiveLimit(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	settings.CoreEnabled = true
	scheduler, err := NewCoreSessionScheduler(CoreSessionSchedulerConfig{Store: store, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first := sessionOperationInput(store.now(), "first", 101)
	second := sessionOperationInput(store.now(), "second", 102)
	third := sessionOperationInput(store.now(), "third", 103)
	for _, input := range []SessionOperationInput{first, second, third} {
		if record, err := scheduler.Accept(ctx, input); err != nil || record.State != SessionOperationQueued {
			t.Fatalf("Accept(%q) = %#v, %v", input.OperationID, record, err)
		}
	}
	for _, input := range []SessionOperationInput{first, second} {
		if _, started, err := scheduler.TryStart(ctx, input.SessionID, input.OperationID); err != nil || !started {
			t.Fatalf("TryStart(%q) = %t, %v", input.OperationID, started, err)
		}
	}
	if queued, started, err := scheduler.TryStart(ctx, third.SessionID, third.OperationID); err != nil || started || queued.State != SessionOperationQueued {
		t.Fatalf("third TryStart() = %#v, %t, %v", queued, started, err)
	}
	if _, err := scheduler.Finish(ctx, first.SessionID, first.OperationID, successfulSessionOperationCompletion()); err != nil {
		t.Fatal(err)
	}
	if _, started, err := scheduler.TryStart(ctx, third.SessionID, third.OperationID); err != nil || !started {
		t.Fatalf("third TryStart() after release = %t, %v", started, err)
	}

	sameUser := sessionOperationInput(store.now(), "same-user", second.UserID)
	if _, err := scheduler.Accept(ctx, sameUser); err != nil {
		t.Fatal(err)
	}
	if queued, started, err := scheduler.TryStart(ctx, sameUser.SessionID, sameUser.OperationID); err != nil || started || queued.State != SessionOperationQueued {
		t.Fatalf("same-user TryStart() = %#v, %t, %v", queued, started, err)
	}
}

func TestSessionOperationStoreOnlyClaimsQueueHead(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	first := sessionOperationInput(store.now(), "fifo-first", 301)
	second := sessionOperationInput(store.now(), "fifo-second", 302)
	for _, input := range []SessionOperationInput{first, second} {
		if _, _, err := store.Accept(ctx, input); err != nil {
			t.Fatal(err)
		}
		if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
			t.Fatal(err)
		}
	}
	if queued, started, err := store.ClaimQueued(ctx, second.SessionID, second.OperationID, 4, 4); err != nil || started || queued.State != SessionOperationQueued {
		t.Fatalf("second ClaimQueued before queue head = %#v, %t, %v", queued, started, err)
	}
	if _, started, err := store.ClaimQueued(ctx, first.SessionID, first.OperationID, 4, 4); err != nil || !started {
		t.Fatalf("first ClaimQueued = %t, %v", started, err)
	}
}

func TestSessionOperationStoreRotatesUserBlockedHeadWithoutBreakingFIFO(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	ctx := context.Background()
	running := sessionOperationInput(store.now(), "running-user", 401)
	blocked := sessionOperationInput(store.now(), "blocked-same-user", running.UserID)
	eligible := sessionOperationInput(store.now(), "eligible-other-user", 403)
	for _, input := range []SessionOperationInput{running, blocked, eligible} {
		if _, _, err := store.Accept(ctx, input); err != nil {
			t.Fatal(err)
		}
		if _, err := store.MarkQueued(ctx, input.SessionID, input.OperationID); err != nil {
			t.Fatal(err)
		}
	}
	if _, started, err := store.ClaimQueued(ctx, running.SessionID, running.OperationID, 4, 1); err != nil || !started {
		t.Fatalf("running ClaimQueued() = %t, %v", started, err)
	}
	if queued, started, err := store.ClaimQueued(ctx, eligible.SessionID, eligible.OperationID, 4, 1); err != nil || !started || queued.State != SessionOperationRunning {
		t.Fatalf("eligible ClaimQueued() = %#v, %t, %v", queued, started, err)
	}
}

func TestCoreSessionSchedulerMayStartDisabledAndFailsClosedUntilApplied(t *testing.T) {
	store, _ := newSessionRedisStoreFixture(t)
	settings := domainsandbox.DefaultSessionRuntimeSettings()
	scheduler, err := NewCoreSessionScheduler(CoreSessionSchedulerConfig{Store: store, Settings: settings})
	if err != nil {
		t.Fatalf("NewCoreSessionScheduler(disabled) = %v", err)
	}
	if scheduler.CoreEnabled() {
		t.Fatal("CoreEnabled() = true before the persisted setting is enabled")
	}
	now := store.now()
	input := SessionOperationInput{SessionID: "session_disabled", OperationID: "operation_disabled", UserID: 1,
		Kind: SessionOperationExec, RequestDigest: func() []byte { sum := sha256.Sum256([]byte("disabled")); return sum[:] }(), Deadline: now.Add(time.Minute)}
	if _, err := scheduler.Accept(context.Background(), input); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Accept(disabled) = %v, want ErrUnavailable", err)
	}
	settings.CoreEnabled = true
	if err := scheduler.ApplySessionSettings(settings); err != nil {
		t.Fatal(err)
	}
	if !scheduler.CoreEnabled() {
		t.Fatal("CoreEnabled() = false after applying the enabled setting")
	}
	if _, err := scheduler.Accept(context.Background(), input); err != nil {
		t.Fatalf("Accept(enabled) = %v", err)
	}
}

func TestSessionLeaseIsCrossReplicaOwnerFencedAndLossCancelsHandle(t *testing.T) {
	firstStore, server := newSessionRedisStoreFixture(t)
	secondStore, _ := newSessionRedisStoreFixtureWithServer(t, server)
	ctx := context.Background()
	first, err := firstStore.Acquire(ctx, "deployment-a", "session-resource")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secondStore.Acquire(ctx, "deployment-a", "session-resource"); !errors.Is(err, ErrSessionLeaseHeld) {
		t.Fatalf("second Acquire() error = %v", err)
	}
	if err := first.Owned(ctx); err != nil {
		t.Fatalf("Owned() = %v", err)
	}
	if err := first.Renew(ctx); err != nil {
		t.Fatalf("Renew() = %v", err)
	}
	server.FastForward(3 * time.Second)
	if err := first.Owned(ctx); !errors.Is(err, ErrSessionLeaseLost) {
		t.Fatalf("expired Owned() = %v", err)
	}
	select {
	case <-first.Context().Done():
	default:
		t.Fatal("lease loss did not cancel handle context")
	}
	second, err := secondStore.Acquire(ctx, "deployment-a", "session-resource")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Release(ctx); !errors.Is(err, ErrSessionLeaseLost) {
		t.Fatalf("stale Release() = %v", err)
	}
	if err := second.Owned(ctx); err != nil {
		t.Fatalf("stale release deleted new owner: %v", err)
	}
	if err := second.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestSessionLeaseAutomaticallyRenewsUntilRelease(t *testing.T) {
	server := miniredis.RunT(t)
	store, err := NewRedisSessionStore(redisimpl.NewWithAddrAndPassword(server.Addr(), ""), RedisSessionStoreConfig{
		DeploymentID: "deployment-a", ActiveKeyID: "queue-key",
		Keys:      map[string]string{"queue-key": "0123456789abcdef0123456789abcdef"},
		RecordTTL: time.Minute, LeaseTTL: 120 * time.Millisecond, MaxQueueDepth: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := store.Acquire(context.Background(), "deployment-a", "long-operation")
	if err != nil {
		t.Fatal(err)
	}
	concrete := handle.(*redisSessionLeaseHandle)
	server.FastForward(100 * time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	server.FastForward(40 * time.Millisecond)
	if err := handle.Owned(context.Background()); err != nil {
		t.Fatalf("long operation lost an automatically renewed lease: %v", err)
	}
	if err := handle.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-concrete.renewDone:
	case <-time.After(time.Second):
		t.Fatal("Release() did not stop the renewal worker")
	}
}

func TestSessionNonceStoreHashesReplayKeyAndFailsClosed(t *testing.T) {
	store, server := newSessionRedisStoreFixture(t)
	var _ sandboxidentity.SessionNonceStore = store
	ctx := context.Background()
	expiresAt := store.now().Add(time.Minute)
	consumed, err := store.Consume(ctx, "key-id-sensitive", "nonce-sensitive", expiresAt)
	if err != nil || !consumed {
		t.Fatalf("first Consume() = %t, %v", consumed, err)
	}
	consumed, err = store.Consume(ctx, "key-id-sensitive", "nonce-sensitive", expiresAt)
	if err != nil || consumed {
		t.Fatalf("replay Consume() = %t, %v", consumed, err)
	}
	for _, key := range server.Keys() {
		if strings.Contains(key, "key-id-sensitive") || strings.Contains(key, "nonce-sensitive") {
			t.Fatalf("nonce plaintext found in key %q", key)
		}
		if server.Type(key) == "string" {
			value, _ := server.Get(key)
			if strings.Contains(value, "key-id-sensitive") || strings.Contains(value, "nonce-sensitive") {
				t.Fatalf("nonce plaintext found in value %q", value)
			}
		}
	}
	server.Close()
	if consumed, err := store.Consume(ctx, "key-two", "nonce-two", expiresAt); err == nil || consumed {
		t.Fatalf("Redis-down Consume() = %t, %v", consumed, err)
	}
}

func newSessionRedisStoreFixture(t *testing.T) (*RedisSessionStore, *miniredis.Miniredis) {
	t.Helper()
	server := miniredis.RunT(t)
	return newSessionRedisStoreFixtureWithServer(t, server)
}

func newSessionRedisStoreFixtureWithServer(t *testing.T, server *miniredis.Miniredis) (*RedisSessionStore, *miniredis.Miniredis) {
	t.Helper()
	store, err := NewRedisSessionStore(redisimpl.NewWithAddrAndPassword(server.Addr(), ""), RedisSessionStoreConfig{
		DeploymentID: "deployment-a", ActiveKeyID: "queue-key",
		Keys:      map[string]string{"queue-key": "0123456789abcdef0123456789abcdef"},
		RecordTTL: 2 * time.Minute, LeaseTTL: 2 * time.Second, MaxQueueDepth: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixedNow := time.Date(2026, time.August, 14, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return fixedNow }
	return store, server
}

func sessionOperationInput(now time.Time, operationID string, userID int64) SessionOperationInput {
	digest := sha256.Sum256([]byte("request-" + operationID))
	return SessionOperationInput{
		SessionID: "session-" + operationID, OperationID: "operation-" + operationID,
		UserID: userID, Kind: SessionOperationExec, RequestDigest: digest[:],
		Weight: 1, Deadline: now.Add(time.Minute),
	}
}

func successfulSessionOperationCompletion() SessionOperationCompletion {
	result := []byte(`{"ok":true}`)
	digest := sha256.Sum256(result)
	return SessionOperationCompletion{State: SessionOperationSucceeded, Result: result, ResultDigest: digest[:]}
}

type repeatingReader struct{ value []byte }

func (reader *repeatingReader) Read(output []byte) (int, error) {
	if len(reader.value) == 0 {
		return 0, io.EOF
	}
	for index := range output {
		output[index] = reader.value[index%len(reader.value)]
	}
	return len(output), nil
}
