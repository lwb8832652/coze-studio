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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

var (
	ErrAdaptiveExecutionBoundaryInvalid         = errors.New("adaptive execution boundary is invalid")
	ErrAdaptiveExecutionAttemptConflict         = errors.New("adaptive execution attempt conflict")
	ErrAdaptiveExecutionLineageConflict         = errors.New("adaptive execution lineage conflict")
	ErrAdaptiveExecutionPlanScopeConflict       = errors.New("adaptive execution plan scope conflict")
	ErrAdaptiveExecutionCheckpointConflict      = errors.New("adaptive execution checkpoint conflict")
	ErrAdaptiveExecutionPlanRevisionConflict    = errors.New("adaptive execution plan revision conflict")
	ErrAdaptiveExecutionPlanItemVersionConflict = errors.New("adaptive execution plan item version conflict")
	ErrAdaptiveExecutionSequenceConflict        = errors.New("adaptive execution sequence conflict")
)

type AdaptivePlanItemMutation struct {
	ExpectedVersion int64
	NextItem        *entity.AgentRunPlanItem
}

type AdaptivePlanMutation struct {
	PlanScopeRunID   int64
	ExpectedRevision int64
	NextRevision     int64
	Items            []AdaptivePlanItemMutation
}

type CommitAdaptiveExecutionBoundaryResult struct {
	Event                 *entity.RunEvent
	Checkpoint            *entity.Checkpoint
	Plan                  *entity.AgentRunPlan
	Items                 []*entity.AgentRunPlanItem
	LastCommittedSequence uint64
}

type AdaptiveExecutionRepository interface {
	CommitAdaptiveExecutionBoundary(
		ctx context.Context,
		req CommitAdaptiveExecutionBoundaryRequest,
	) (*CommitAdaptiveExecutionBoundaryResult, error)
}

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
	PlanMutation   *AdaptivePlanMutation
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

func validateAdaptiveExecutionMutationRequest(req CommitAdaptiveExecutionBoundaryRequest) error {
	if err := validateAdaptiveExecutionBoundaryRequest(req); err != nil {
		return err
	}
	mutation := req.PlanMutation
	if mutation == nil || mutation.PlanScopeRunID <= 0 || mutation.ExpectedRevision <= 0 ||
		mutation.ExpectedRevision == math.MaxInt64 ||
		mutation.NextRevision != mutation.ExpectedRevision+1 ||
		len(mutation.Items) < 1 || len(mutation.Items) > 32 {
		return fmt.Errorf("%w: plan mutation is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if req.Event.ID <= 0 || strings.TrimSpace(req.Event.EventType) == "" || req.Event.CreatedAt != req.Now {
		return fmt.Errorf("%w: event is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if _, err := runEventToPO(req.Event); err != nil {
		return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
	}
	if req.Checkpoint.ID <= 0 || strings.TrimSpace(req.Checkpoint.CheckpointNS) == "" ||
		req.Checkpoint.RuntimeType != "eino_adk" || strings.TrimSpace(req.Checkpoint.RuntimeKey) == "" ||
		req.Checkpoint.EnvelopeVersion <= 0 || req.Checkpoint.RuntimeDeletedAt != 0 ||
		req.Checkpoint.CreatedAt != req.Now {
		return fmt.Errorf("%w: checkpoint is invalid", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if _, err := checkpointToPO(req.Checkpoint); err != nil {
		return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
	}
	metadata := make(map[string]json.RawMessage)
	if err := json.Unmarshal([]byte(req.Checkpoint.Metadata), &metadata); err != nil || metadata == nil {
		return fmt.Errorf("%w: checkpoint metadata must be a JSON object", ErrAdaptiveExecutionBoundaryInvalid)
	}
	if _, exists := metadata["adaptive_execution"]; exists {
		return fmt.Errorf("%w: checkpoint metadata contains reserved namespace", ErrAdaptiveExecutionBoundaryInvalid)
	}

	itemIDs := make(map[int64]struct{}, len(mutation.Items))
	taskIDs := make(map[int64]struct{}, len(mutation.Items))
	for _, itemMutation := range mutation.Items {
		item := itemMutation.NextItem
		if itemMutation.ExpectedVersion < 0 || itemMutation.ExpectedVersion == math.MaxInt64 || item == nil ||
			item.ID <= 0 || item.RunID <= 0 || item.TaskID <= 0 ||
			item.RunID != mutation.PlanScopeRunID || item.Version != itemMutation.ExpectedVersion+1 {
			return fmt.Errorf("%w: plan item mutation is invalid", ErrAdaptiveExecutionBoundaryInvalid)
		}
		if _, exists := itemIDs[item.ID]; exists {
			return fmt.Errorf("%w: duplicate plan item id", ErrAdaptiveExecutionBoundaryInvalid)
		}
		if _, exists := taskIDs[item.TaskID]; exists {
			return fmt.Errorf("%w: duplicate plan task id", ErrAdaptiveExecutionBoundaryInvalid)
		}
		itemIDs[item.ID] = struct{}{}
		taskIDs[item.TaskID] = struct{}{}
		if _, err := agentRunPlanItemToPO(item); err != nil {
			return fmt.Errorf("%w: %v", ErrAdaptiveExecutionBoundaryInvalid, err)
		}
	}
	return nil
}
