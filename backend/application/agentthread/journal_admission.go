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
	"errors"
	"fmt"
	"time"
)

var (
	ErrJournalRateLimited          = errors.New("journal request rate limited")
	ErrJournalAdmissionUnavailable = errors.New("journal admission is unavailable")
)

type JournalAdmissionKind string

const (
	JournalAdmissionKindBootstrap JournalAdmissionKind = "bootstrap"
	JournalAdmissionKindSnapshot  JournalAdmissionKind = "snapshot"
	JournalAdmissionKindAction    JournalAdmissionKind = "action"
	JournalAdmissionKindStream    JournalAdmissionKind = "stream"
)

type JournalAdmissionRequest struct {
	SpaceID  int64
	ViewerID int64
	ThreadID int64
	RunID    int64
	Kind     JournalAdmissionKind
}

type JournalAdmissionLease interface {
	Renew(context.Context) error
	Release(context.Context) error
}

type JournalAdmissionLimiter interface {
	Acquire(context.Context, JournalAdmissionRequest) (JournalAdmissionLease, error)
}

type JournalRateLimitError struct {
	RetryAfter time.Duration
}

func (e *JournalRateLimitError) Error() string {
	return ErrJournalRateLimited.Error()
}

func (e *JournalRateLimitError) Unwrap() error {
	return ErrJournalRateLimited
}

func (s *ApplicationService) AcquireJournalAdmission(
	ctx context.Context,
	req JournalAdmissionRequest,
) (JournalAdmissionLease, error) {
	if req.SpaceID <= 0 || req.ViewerID <= 0 || req.Kind == "" {
		return nil, fmt.Errorf("journal admission request is invalid")
	}
	if s == nil || s.JournalAdmissionLimiter == nil {
		if s != nil && !s.JournalAdmissionRequired {
			return nil, nil
		}
		return nil, ErrJournalAdmissionUnavailable
	}
	lease, err := s.JournalAdmissionLimiter.Acquire(ctx, req)
	if err != nil {
		var rateLimited *JournalRateLimitError
		if errors.As(err, &rateLimited) {
			if rateLimited.RetryAfter < time.Second {
				rateLimited.RetryAfter = time.Second
			}
			if rateLimited.RetryAfter > time.Minute {
				rateLimited.RetryAfter = time.Minute
			}
			return nil, rateLimited
		}
		return nil, fmt.Errorf("%w: limiter failed", ErrJournalAdmissionUnavailable)
	}
	return lease, nil
}
