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

package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

var (
	ErrAdaptiveExecutionBoundaryInvalid = errors.New("adaptive execution boundary is invalid")
	ErrAdaptiveExecutionAttemptConflict = errors.New("adaptive execution attempt conflict")
)

type CommitAdaptiveExecutionBoundaryRequest struct {
	ThreadID       int64
	ExecutionRunID int64
	JournalRunID   int64
	AttemptID      string
	Generation     uint64
	LeaseOwner     string
	LeaseToken     string
	Now            int64
	IdempotencyKey string
	Event          *entity.RunEvent
	Checkpoint     *entity.Checkpoint
}

func validateAdaptiveExecutionBoundaryRequest(req CommitAdaptiveExecutionBoundaryRequest) error {
	if req.ThreadID <= 0 || req.ExecutionRunID <= 0 || req.JournalRunID <= 0 ||
		strings.TrimSpace(req.AttemptID) == "" || req.Generation == 0 ||
		strings.TrimSpace(req.LeaseOwner) == "" || strings.TrimSpace(req.LeaseToken) == "" ||
		req.Now <= 0 || strings.TrimSpace(req.IdempotencyKey) == "" ||
		req.Event == nil || req.Checkpoint == nil {
		return fmt.Errorf("%w: required identity is missing", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if req.Event.ThreadID != req.ThreadID || req.Event.RunID != req.ExecutionRunID {
		return fmt.Errorf("%w: event identity drift", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if req.Checkpoint.ThreadID != req.ThreadID || req.Checkpoint.RunID != req.ExecutionRunID {
		return fmt.Errorf("%w: checkpoint identity drift", ErrAdaptiveExecutionBoundaryInvalid)
	}
	return nil
}
