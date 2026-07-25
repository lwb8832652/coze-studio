/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package notification

import (
	"context"
	"testing"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	infranotification "github.com/coze-dev/coze-studio/backend/infra/notification"
	"github.com/stretchr/testify/require"
)

type infrastructureClockWorkerRepository struct {
	recoverErr error
	failDead   bool
	failCalled bool
}

func (r *infrastructureClockWorkerRepository) RecoverExpiredLeases(
	context.Context,
	time.Time,
	time.Duration,
) (int64, error) {
	return 0, r.recoverErr
}

func (*infrastructureClockWorkerRepository) ClaimOutboxBatch(
	context.Context,
	string,
	time.Time,
	time.Duration,
	int,
) ([]domainnotification.OutboxClaim, error) {
	return nil, nil
}

func (*infrastructureClockWorkerRepository) MaterializeAndDeliver(
	context.Context,
	domainnotification.OutboxClaim,
	domainnotification.MessageDraft,
	[]int64,
	time.Time,
) error {
	return nil
}

func (r *infrastructureClockWorkerRepository) FailClaim(
	_ context.Context,
	_ domainnotification.OutboxClaim,
	_ string,
	_ time.Time,
	_ time.Time,
	dead bool,
) error {
	r.failCalled = true
	r.failDead = dead
	return nil
}

type retryUntilRecipientExistsError struct{}

func (retryUntilRecipientExistsError) Error() string {
	return "system administrator recipients are not yet available"
}

func (retryUntilRecipientExistsError) Unwrap() error {
	return domainnotification.ErrRecipientResolution
}

func (retryUntilRecipientExistsError) RetryWithoutDeadLetter() bool {
	return true
}

type failingRecipientResolver struct {
	err error
}

func (r failingRecipientResolver) Resolve(
	context.Context,
	domainnotification.Event,
) ([]int64, error) {
	return nil, r.err
}

func TestNotificationWorkerPropagatesDatabaseClockFailure(t *testing.T) {
	repository := &infrastructureClockWorkerRepository{
		recoverErr: infranotification.ErrNotificationDatabaseClock,
	}
	worker := NewWorker(repository, nil, WorkerOptions{})

	err := worker.RunOnce(context.Background())
	require.ErrorIs(t, err, infranotification.ErrNotificationDatabaseClock)
}

func TestNotificationWorkerKeepsMissingBootstrapUserRetryable(t *testing.T) {
	repository := &infrastructureClockWorkerRepository{}
	worker := NewWorker(
		repository,
		failingRecipientResolver{err: retryUntilRecipientExistsError{}},
		WorkerOptions{MaxAttempts: 1},
	)
	claim := domainnotification.OutboxClaim{
		OutboxID:       1,
		LockedBy:       "worker-a",
		LeaseExpiresAt: time.Now().Add(time.Minute),
		AttemptCount:   7,
		Event: domainnotification.Event{
			EventID:          "bootstrap-recipient-later",
			EventType:        domainnotification.EventSystemProviderUnavailable,
			AggregateType:    "sandbox_provider_health_incident",
			AggregateID:      "sandbox-incident-1",
			AggregateVersion: 1,
			OccurredAt:       time.Now(),
			RecipientPolicy:  domainnotification.RecipientSystemAdmins,
			PayloadSchema:    domainnotification.CurrentPayloadSchema,
			Payload: domainnotification.EventPayload{
				ResourceDisplayName: "Sandbox provider 1",
				StatusReasonCode: domainnotification.
					StatusReasonProviderUnavailable,
			},
		},
	}

	err := worker.processClaim(context.Background(), claim)
	require.NoError(t, err)
	require.True(t, repository.failCalled)
	require.False(t, repository.failDead)
}
