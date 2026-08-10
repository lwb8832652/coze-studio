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

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func lockAdaptiveExecutionRun(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
) (*runPO, error) {
	query := tx.Where("id = ?", req.ExecutionRunID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var run runPO
	err := query.First(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("%w: run %d not found", ErrRunLeaseLost, req.ExecutionRunID)
	}
	if err != nil {
		return nil, err
	}
	if run.ThreadID != req.ThreadID || entity.RunStatus(run.Status) != entity.RunStatusRunning ||
		run.ExecutionGeneration != req.Generation ||
		run.LeaseOwner == nil || *run.LeaseOwner != req.LeaseOwner ||
		run.LeaseToken == nil || *run.LeaseToken != req.LeaseToken ||
		run.LeaseExpiresAt == nil || *run.LeaseExpiresAt <= req.Now {
		return nil, fmt.Errorf("%w: run %d identity drift", ErrRunLeaseLost, req.ExecutionRunID)
	}
	if run.CancelRequestedAt != nil {
		return nil, fmt.Errorf("%w: run %d has cancellation requested", ErrRunCanceled, req.ExecutionRunID)
	}
	return &run, nil
}

func lockAdaptiveExecutionAttempt(
	tx *gorm.DB,
	req CommitAdaptiveExecutionBoundaryRequest,
) (*runAttemptPO, error) {
	query := tx.Where(
		"journal_run_id = ? AND attempt_id = ?", req.JournalRunID, req.AttemptID,
	)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var attempt runAttemptPO
	err := query.First(&attempt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf(
			"%w: attempt %s not found", ErrAdaptiveExecutionAttemptConflict, req.AttemptID,
		)
	}
	if err != nil {
		return nil, err
	}
	if attempt.ThreadID != req.ThreadID || attempt.ExecutionRunID != req.ExecutionRunID ||
		!entity.RunAttemptStatus(attempt.Status).IsActive() || attempt.ActiveSlot == nil {
		return nil, fmt.Errorf(
			"%w: attempt %s identity drift", ErrAdaptiveExecutionAttemptConflict, req.AttemptID,
		)
	}
	return &attempt, nil
}
