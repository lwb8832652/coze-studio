// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type fakeWorkerRepository struct {
	mu                 sync.Mutex
	claims             []domainnotification.OutboxClaim
	materializeErr     error
	materializeStarted chan string
	materializeRelease <-chan struct{}
	recovered          bool
	failed             bool
	failedAsDead       bool
	failureCode        string
	failureCtxErr      error
	failedEventIDs     []string
	materializedFor    []int64
	materializedIDs    []string
}

func (f *fakeWorkerRepository) RecoverExpiredLeases(
	context.Context,
	time.Time,
	time.Duration,
) (int64, error) {
	f.recovered = true
	return 1, nil
}

func (f *fakeWorkerRepository) ClaimOutboxBatch(
	_ context.Context,
	_ string,
	now time.Time,
	lease time.Duration,
	_ int,
) ([]domainnotification.OutboxClaim, error) {
	claims := append([]domainnotification.OutboxClaim(nil), f.claims...)
	for index := range claims {
		if claims[index].LeaseExpiresAt.IsZero() {
			claims[index].LeaseExpiresAt = time.Now().Add(lease)
			if now.After(time.Now()) {
				claims[index].LeaseExpiresAt = now.Add(lease)
			}
		}
	}
	return claims, nil
}

func (f *fakeWorkerRepository) MaterializeAndDeliver(
	ctx context.Context,
	claim domainnotification.OutboxClaim,
	_ domainnotification.MessageDraft,
	recipients []int64,
	_ time.Time,
) error {
	f.mu.Lock()
	f.materializedFor = append([]int64(nil), recipients...)
	f.materializedIDs = append(f.materializedIDs, claim.Event.EventID)
	f.mu.Unlock()
	if f.materializeStarted != nil {
		f.materializeStarted <- claim.Event.EventID
	}
	if f.materializeRelease != nil {
		select {
		case <-f.materializeRelease:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return f.materializeErr
}

func (f *fakeWorkerRepository) FailClaim(
	ctx context.Context,
	claim domainnotification.OutboxClaim,
	code string,
	_ time.Time,
	_ time.Time,
	dead bool,
) error {
	f.failed = true
	f.failedAsDead = dead
	f.failureCode = code
	f.failureCtxErr = ctx.Err()
	f.failedEventIDs = append(f.failedEventIDs, claim.Event.EventID)
	return nil
}

func TestWorkerRecoversLeasesAndMaterializesRecipients(t *testing.T) {
	event := workerTestEvent()
	repository := &fakeWorkerRepository{
		claims: []domainnotification.OutboxClaim{{
			OutboxID:     1,
			LockedBy:     "worker-test",
			AttemptCount: 0,
			Event:        event,
		}},
	}
	worker := NewWorker(repository, RecipientResolverFunc(
		func(context.Context, domainnotification.Event) ([]int64, error) {
			return []int64{202, 101, 202}, nil
		},
	), WorkerOptions{
		WorkerID:    "worker-test",
		BatchSize:   10,
		Lease:       30 * time.Second,
		MaxAttempts: 3,
		RetryDelays: []time.Duration{time.Second},
		Now: func() time.Time {
			return time.UnixMilli(1_721_000_000_000)
		},
	})

	require.NoError(t, worker.RunOnce(context.Background()))
	require.True(t, repository.recovered)
	require.Equal(t, []int64{101, 202}, repository.materializedFor)
	require.Equal(t, int64(101), repository.materializedFor[0])
	require.Equal(t, int64(202), repository.materializedFor[1])
	require.False(t, repository.failed)
}

type fakeSystemAdminRecipientSource struct {
	ids []int64
	err error
}

func (f fakeSystemAdminRecipientSource) ListSystemAdministratorUserIDs(
	context.Context,
) ([]int64, error) {
	return append([]int64(nil), f.ids...), f.err
}

func TestPolicyRecipientResolverUsesOnlyServerSystemAdministratorSource(t *testing.T) {
	event := workerTestEvent()
	event.RecipientPolicy = domainnotification.RecipientSystemAdmins
	event.ActorID = 0
	event.SpaceID = 0
	event.Payload.ExplicitRecipientIDs = nil
	resolver := PolicyRecipientResolver{
		SystemAdmins: fakeSystemAdminRecipientSource{ids: []int64{202, 101, 202}},
	}
	recipients, err := resolver.Resolve(context.Background(), event)
	require.NoError(t, err)
	require.Equal(t, []int64{101, 202}, recipients)

	_, err = (PolicyRecipientResolver{}).Resolve(context.Background(), event)
	require.ErrorIs(t, err, domainnotification.ErrRecipientPolicyUnavailable)
}

func TestWorkerRetriesThenDeadLettersWithoutReturningBusinessFailure(t *testing.T) {
	event := workerTestEvent()
	repository := &fakeWorkerRepository{
		claims: []domainnotification.OutboxClaim{{
			OutboxID:     1,
			LockedBy:     "worker-test",
			AttemptCount: 1,
			Event:        event,
		}},
		materializeErr: errors.New("database unavailable with raw details"),
	}
	worker := NewWorker(repository, RecipientResolverFunc(
		func(context.Context, domainnotification.Event) ([]int64, error) {
			return []int64{101}, nil
		},
	), WorkerOptions{
		WorkerID:    "worker-test",
		BatchSize:   10,
		Lease:       30 * time.Second,
		MaxAttempts: 2,
		RetryDelays: []time.Duration{time.Second},
		Now:         time.Now,
	})

	require.NoError(t, worker.RunOnce(context.Background()))
	require.True(t, repository.failed)
	require.True(t, repository.failedAsDead)
	require.Equal(t, domainnotification.ErrorCodeStorage, repository.failureCode)
	require.NotContains(t, repository.failureCode, "raw details")
}

func TestWorkerMalformedClaimDeadLettersAndDoesNotBlockNextEvent(t *testing.T) {
	valid := workerTestEvent()
	repository := &fakeWorkerRepository{
		claims: []domainnotification.OutboxClaim{
			{
				OutboxID:       1,
				LockedBy:       "worker-test",
				AttemptCount:   0,
				Event:          domainnotification.Event{EventID: "evt-malformed"},
				ClaimErrorCode: domainnotification.ErrorCodeInvalidEvent,
			},
			{
				OutboxID:     2,
				LockedBy:     "worker-test",
				AttemptCount: 0,
				Event:        valid,
			},
		},
	}
	worker := NewWorker(repository, PolicyRecipientResolver{}, WorkerOptions{
		WorkerID:    "worker-test",
		BatchSize:   10,
		Lease:       30 * time.Second,
		MaxAttempts: 1,
		RetryDelays: []time.Duration{time.Second},
		Now:         time.Now,
	})

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Equal(t, []string{"evt-malformed"}, repository.failedEventIDs)
	require.Equal(t, domainnotification.ErrorCodeInvalidEvent, repository.failureCode)
	require.True(t, repository.failedAsDead)
	require.Equal(t, []string{valid.EventID}, repository.materializedIDs)
}

func TestWorkerPersistsFailureWithFreshContextAfterClaimTimeout(t *testing.T) {
	event := workerTestEvent()
	repository := &fakeWorkerRepository{
		claims: []domainnotification.OutboxClaim{{
			OutboxID:     1,
			LockedBy:     "worker-test",
			AttemptCount: 0,
			Event:        event,
		}},
	}
	worker := NewWorker(repository, RecipientResolverFunc(
		func(ctx context.Context, _ domainnotification.Event) ([]int64, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	), WorkerOptions{
		WorkerID:              "worker-test",
		BatchSize:             10,
		Lease:                 30 * time.Second,
		BatchTimeout:          2 * time.Second,
		ClaimTimeout:          time.Millisecond,
		FailurePersistTimeout: time.Second,
		MaxAttempts:           2,
		RetryDelays:           []time.Duration{time.Second},
		Now:                   time.Now,
	})

	require.NoError(t, worker.RunOnce(context.Background()))
	require.True(t, repository.failed)
	require.NoError(t, repository.failureCtxErr)
}

func TestWorkerRejectsLeaseUnsafeBatchConfiguration(t *testing.T) {
	repository := &fakeWorkerRepository{}
	worker := NewWorker(repository, PolicyRecipientResolver{}, WorkerOptions{
		WorkerID:              "worker-unsafe",
		BatchSize:             10,
		Concurrency:           1,
		Lease:                 10 * time.Second,
		BatchTimeout:          5 * time.Second,
		ClaimTimeout:          time.Second,
		FailurePersistTimeout: time.Second,
		MaxAttempts:           2,
		RetryDelays:           []time.Duration{time.Second},
		Now:                   time.Now,
	})

	err := worker.RunOnce(context.Background())

	require.ErrorIs(t, err, domainnotification.ErrInvalidEvent)
	require.False(t, repository.recovered)
}

func TestDefaultWorkerTimingSeparatesBatchAndClaimBudgets(t *testing.T) {
	options := DefaultWorkerOptions()

	require.Equal(t, 10*time.Second, options.BatchTimeout)
	require.Equal(t, 3*time.Second, options.ClaimTimeout)
	require.Less(t, options.ClaimTimeout, options.BatchTimeout)
	require.NoError(t, validateWorkerTiming(options))
}

func TestClaimBatchBudgetChargesClaimRoundTripAndSafetyMargin(t *testing.T) {
	budget, err := claimBatchBudget(
		10*time.Second,
		30*time.Second,
		0,
	)
	require.NoError(t, err)
	require.Equal(t, 10*time.Second, budget)

	budget, err = claimBatchBudget(
		10*time.Second,
		30*time.Second,
		23*time.Second,
	)
	require.NoError(t, err)
	require.Equal(t, 4*time.Second, budget)

	_, err = claimBatchBudget(
		10*time.Second,
		30*time.Second,
		27*time.Second,
	)
	require.ErrorIs(t, err, domainnotification.ErrLeaseLost)
}

func TestWorkerLimitsConcurrencyWithinLeaseSafeBudget(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	claims := make([]domainnotification.OutboxClaim, 0, 4)
	for index := 0; index < 4; index++ {
		event := workerTestEvent()
		event.EventID = event.EventID + string(rune('a'+index))
		claims = append(claims, domainnotification.OutboxClaim{
			OutboxID:     int64(index + 1),
			LockedBy:     "worker-concurrent",
			AttemptCount: 0,
			Event:        event,
		})
	}
	repository := &fakeWorkerRepository{
		claims:             claims,
		materializeStarted: started,
		materializeRelease: release,
	}
	worker := NewWorker(repository, PolicyRecipientResolver{}, WorkerOptions{
		WorkerID:              "worker-concurrent",
		BatchSize:             4,
		Concurrency:           2,
		Lease:                 2 * time.Second,
		BatchTimeout:          time.Second,
		ClaimTimeout:          200 * time.Millisecond,
		FailurePersistTimeout: 20 * time.Millisecond,
		MaxAttempts:           2,
		RetryDelays:           []time.Duration{time.Second},
		Now:                   time.Now,
	})
	done := make(chan error, 1)
	go func() {
		done <- worker.RunOnce(context.Background())
	}()

	<-started
	<-started
	select {
	case third := <-started:
		t.Fatalf("worker pool exceeded concurrency limit before release: %s", third)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)

	require.NoError(t, <-done)
	require.Len(t, repository.materializedIDs, 4)
}

func TestWorkerStopsDequeuingClaimsWhenShortLeaseBudgetExpires(
	t *testing.T,
) {
	started := make(chan string, 5)
	release := make(chan struct{})
	claims := make([]domainnotification.OutboxClaim, 0, 5)
	for index := 0; index < 5; index++ {
		event := workerTestEvent()
		event.EventID = event.EventID + string(rune('a'+index))
		claims = append(claims, domainnotification.OutboxClaim{
			OutboxID:     int64(index + 1),
			LockedBy:     "worker-short-budget",
			AttemptCount: 0,
			Event:        event,
		})
	}
	repository := &fakeWorkerRepository{
		claims:             claims,
		materializeStarted: started,
		materializeRelease: release,
	}
	worker := NewWorker(repository, PolicyRecipientResolver{}, WorkerOptions{
		WorkerID:              "worker-short-budget",
		BatchSize:             5,
		Concurrency:           1,
		Lease:                 2 * time.Second,
		BatchTimeout:          time.Second,
		ClaimTimeout:          100 * time.Millisecond,
		FailurePersistTimeout: time.Millisecond,
		MaxAttempts:           2,
		RetryDelays:           []time.Duration{time.Second},
		Now:                   time.Now,
	})
	claimStart := time.Unix(100, 0)
	leaseClockCalls := 0
	worker.monotonicNow = func() time.Time {
		leaseClockCalls++
		if leaseClockCalls == 1 {
			return claimStart
		}
		// A 2s lease has a 500ms safety margin, leaving only 50ms.
		return claimStart.Add(1450 * time.Millisecond)
	}

	require.NoError(t, worker.RunOnce(context.Background()))
	require.Equal(t, []string{claims[0].Event.EventID}, repository.materializedIDs)
	require.Equal(t, []string{claims[0].Event.EventID}, repository.failedEventIDs)
	require.Len(t, started, 1)
}

func TestPolicyResolverSupportsOnlyActorAndExplicitRecipients(t *testing.T) {
	resolver := PolicyRecipientResolver{}
	actor := workerTestEvent()
	ids, err := resolver.Resolve(context.Background(), actor)
	require.NoError(t, err)
	require.Equal(t, []int64{actor.ActorID}, ids)

	explicit := actor
	explicit.RecipientPolicy = domainnotification.RecipientExplicitInternalUsers
	explicit.Payload.ExplicitRecipientIDs = []int64{202, 101, 202}
	ids, err = resolver.Resolve(context.Background(), explicit)
	require.NoError(t, err)
	require.Equal(t, []int64{101, 202}, ids)
	require.Len(t, ids, 2)
	require.Less(t, ids[0], ids[1])

	unsupported := actor
	unsupported.RecipientPolicy = domainnotification.RecipientResourceOwner
	_, err = resolver.Resolve(context.Background(), unsupported)
	require.ErrorIs(t, err, domainnotification.ErrRecipientPolicyUnavailable)
}

func workerTestEvent() domainnotification.Event {
	return domainnotification.Event{
		EventID:          "evt-worker",
		EventType:        domainnotification.EventAgentRunSucceeded,
		AggregateType:    "agent_run",
		AggregateID:      "run-worker",
		AggregateVersion: 1,
		OccurredAt:       time.Now(),
		ActorID:          101,
		SpaceID:          202,
		RecipientPolicy:  domainnotification.RecipientActor,
		PayloadSchema:    domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			TargetID: "thread-worker",
		},
	}
}
