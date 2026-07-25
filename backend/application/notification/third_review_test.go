// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestWorkerIgnoresCrossClockLeaseExpiryAndUsesMonotonicBudget(
	t *testing.T,
) {
	event := workerTestEvent()
	repository := &fakeWorkerRepository{
		claims: []domainnotification.OutboxClaim{{
			OutboxID:      1,
			LockedBy:      "worker-offset",
			// Simulate a DB wall clock many years behind the host. The worker
			// must not compare this value with its own wall clock.
			LeaseExpiresAt: time.Unix(1, 0),
			Event:         event,
		}},
	}
	worker := NewWorker(repository, PolicyRecipientResolver{}, WorkerOptions{
		WorkerID:              "worker-offset",
		BatchSize:             1,
		Concurrency:           1,
		Lease:                 30 * time.Second,
		BatchTimeout:          time.Second,
		ClaimTimeout:          200 * time.Millisecond,
		FailurePersistTimeout: 100 * time.Millisecond,
		MaxAttempts:           2,
		RetryDelays:           []time.Duration{time.Second},
		Now: func() time.Time {
			return time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
		},
	})
	claimStart := time.Unix(100, 0)
	leaseClockCalls := 0
	worker.monotonicNow = func() time.Time {
		leaseClockCalls++
		if leaseClockCalls == 1 {
			return claimStart
		}
		return claimStart.Add(100 * time.Millisecond)
	}

	err := worker.RunOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{event.EventID}, repository.materializedIDs)
}

func TestWorkerRejectsClaimWhenMonotonicLeaseBudgetIsExhausted(
	t *testing.T,
) {
	event := workerTestEvent()
	repository := &fakeWorkerRepository{
		claims: []domainnotification.OutboxClaim{{
			OutboxID:      1,
			LockedBy:      "worker-exhausted",
			LeaseExpiresAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
			Event:         event,
		}},
	}
	worker := NewWorker(repository, PolicyRecipientResolver{}, WorkerOptions{
		WorkerID:              "worker-exhausted",
		BatchSize:             1,
		Concurrency:           1,
		Lease:                 30 * time.Second,
		BatchTimeout:          10 * time.Second,
		ClaimTimeout:          3 * time.Second,
		FailurePersistTimeout: time.Second,
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
		return claimStart.Add(28 * time.Second)
	}

	err := worker.RunOnce(context.Background())

	require.ErrorIs(t, err, domainnotification.ErrLeaseLost)
	require.Empty(t, repository.materializedIDs)
}
