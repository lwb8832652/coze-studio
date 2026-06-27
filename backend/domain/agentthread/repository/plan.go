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
	"errors"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

var (
	ErrPlanNotFound            = errors.New("agent run plan not found")
	ErrPlanItemNotFound        = errors.New("agent run plan item not found")
	ErrPlanReservationConflict = errors.New("agent run plan reservation conflict")
)

type PlanRepository interface {
	GetPlan(ctx context.Context, runID int64) (*entity.AgentRunPlan, error)
	ReservePlanTaskID(
		ctx context.Context,
		plan *entity.AgentRunPlan,
		expected int64,
		next int64,
	) (*entity.AgentRunPlan, error)
	ListPlanItems(
		ctx context.Context,
		runID int64,
		activeOnly bool,
	) ([]*entity.AgentRunPlanItem, error)
	GetPlanItem(
		ctx context.Context,
		runID int64,
		taskID int64,
	) (*entity.AgentRunPlanItem, error)
	UpsertPlanItem(
		ctx context.Context,
		item *entity.AgentRunPlanItem,
	) (
		stored *entity.AgentRunPlanItem,
		previous *entity.AgentRunPlanItem,
		plan *entity.AgentRunPlan,
		created bool,
		err error,
	)
	ArchivePlanItem(
		ctx context.Context,
		runID int64,
		taskID int64,
		updatedAt int64,
	) (
		stored *entity.AgentRunPlanItem,
		previous *entity.AgentRunPlanItem,
		plan *entity.AgentRunPlan,
		err error,
	)
}

func NewPlanRepository(db *gorm.DB) PlanRepository {
	return &threadRepository{db: db}
}
