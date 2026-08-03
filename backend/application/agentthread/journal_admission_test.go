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

package agentthread

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJournalAdmissionFailsClosedWhenProductionDependencyIsMissing(t *testing.T) {
	service := &ApplicationService{JournalAdmissionRequired: true}
	_, err := service.AcquireJournalAdmission(context.Background(), JournalAdmissionRequest{
		SpaceID: 1, ViewerID: 2, Kind: JournalAdmissionKindStream,
	})
	require.ErrorIs(t, err, ErrJournalAdmissionUnavailable)
}

func TestJournalAdmissionReturnsControlledRetryAndLease(t *testing.T) {
	limiter := &journalAdmissionLimiterStub{err: &JournalRateLimitError{
		RetryAfter: 3 * time.Second,
	}}
	service := &ApplicationService{JournalAdmissionLimiter: limiter}
	_, err := service.AcquireJournalAdmission(context.Background(), JournalAdmissionRequest{
		SpaceID: 1, ViewerID: 2, Kind: JournalAdmissionKindBootstrap,
	})
	var rateLimited *JournalRateLimitError
	require.ErrorAs(t, err, &rateLimited)
	require.Equal(t, 3*time.Second, rateLimited.RetryAfter)

	lease := &journalAdmissionLeaseStub{}
	limiter.err, limiter.lease = nil, lease
	acquired, err := service.AcquireJournalAdmission(context.Background(), JournalAdmissionRequest{
		SpaceID: 1, ViewerID: 2, Kind: JournalAdmissionKindStream,
	})
	require.NoError(t, err)
	require.Same(t, lease, acquired)
}

type journalAdmissionLimiterStub struct {
	req   JournalAdmissionRequest
	lease JournalAdmissionLease
	err   error
}

func (l *journalAdmissionLimiterStub) Acquire(
	_ context.Context,
	req JournalAdmissionRequest,
) (JournalAdmissionLease, error) {
	l.req = req
	return l.lease, l.err
}

type journalAdmissionLeaseStub struct {
	renewed  int
	released int
}

func (l *journalAdmissionLeaseStub) Renew(context.Context) error {
	l.renewed++
	return nil
}

func (l *journalAdmissionLeaseStub) Release(context.Context) error {
	l.released++
	return nil
}
