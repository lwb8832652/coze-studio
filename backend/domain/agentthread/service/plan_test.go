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

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestPlanServiceOpensMissingPlanFromOwnedRun(t *testing.T) {
	store := &recordingPlanRepository{
		run: &entity.Run{
			ID:        20,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
		},
		getPlanErr: repository.ErrPlanNotFound,
	}
	svc := NewPlanService(&PlanComponents{
		RunReader: store,
		PlanRepo:  store,
		IDGen:     newSequenceIDGen(100),
	})

	snapshot, err := svc.OpenPlan(context.Background(), &OpenPlanRequest{
		ScopeRunID: 20,
		ThreadID:   10,
		SpaceID:    30,
		UserID:     40,
	})

	require.NoError(t, err)
	require.Equal(t, int64(20), snapshot.Plan.RunID)
	require.Equal(t, int64(10), snapshot.Plan.ThreadID)
	require.Equal(t, int64(30), snapshot.Plan.SpaceID)
	require.Equal(t, int64(40), snapshot.Plan.UserID)
	require.Zero(t, snapshot.Plan.HighWatermark)
	require.Empty(t, snapshot.Items)
}

func TestPlanServiceRejectsCrossThreadPlanScope(t *testing.T) {
	store := &recordingPlanRepository{
		run: &entity.Run{
			ID:        20,
			ThreadID:  11,
			SpaceID:   30,
			CreatorID: 40,
		},
	}
	svc := NewPlanService(&PlanComponents{
		RunReader: store,
		PlanRepo:  store,
		IDGen:     newSequenceIDGen(100),
	})

	snapshot, err := svc.OpenPlan(context.Background(), &OpenPlanRequest{
		ScopeRunID: 20,
		ThreadID:   10,
		SpaceID:    30,
		UserID:     40,
	})

	require.ErrorIs(t, err, ErrInvalidArgument)
	require.Nil(t, snapshot)
}

func TestPlanServiceReservesTaskIDWithCompareAndSwap(t *testing.T) {
	store := &recordingPlanRepository{
		run: &entity.Run{
			ID:        20,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
		},
		reserved: &entity.AgentRunPlan{
			RunID:         20,
			ThreadID:      10,
			SpaceID:       30,
			UserID:        40,
			HighWatermark: 2,
			Revision:      3,
		},
	}
	svc := NewPlanService(&PlanComponents{
		RunReader: store,
		PlanRepo:  store,
		IDGen:     newSequenceIDGen(100),
	})

	snapshot, err := svc.ReservePlanTaskID(
		context.Background(),
		&ReservePlanTaskIDRequest{
			OpenPlanRequest: OpenPlanRequest{
				ScopeRunID: 20,
				ThreadID:   10,
				SpaceID:    30,
				UserID:     40,
			},
			Expected: 1,
			Next:     2,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(2), snapshot.Plan.HighWatermark)
	require.Equal(t, int64(1), store.expected)
	require.Equal(t, int64(2), store.next)
	require.Equal(t, int64(20), store.reservation.RunID)
}

func TestPlanServicePreservesCompletedStatusWhenArchiving(t *testing.T) {
	store := &recordingPlanRepository{
		run: &entity.Run{
			ID:        20,
			ThreadID:  10,
			SpaceID:   30,
			CreatorID: 40,
		},
		archived: &entity.AgentRunPlanItem{
			ID:      100,
			RunID:   20,
			TaskID:  1,
			Subject: "Run tests",
			Status:  entity.AgentRunPlanItemStatusCompleted,
			Active:  false,
			Version: 3,
		},
		previous: &entity.AgentRunPlanItem{
			ID:      100,
			RunID:   20,
			TaskID:  1,
			Subject: "Run tests",
			Status:  entity.AgentRunPlanItemStatusCompleted,
			Active:  true,
			Version: 2,
		},
		plan: &entity.AgentRunPlan{
			RunID:         20,
			ThreadID:      10,
			SpaceID:       30,
			UserID:        40,
			HighWatermark: 1,
			Revision:      4,
		},
	}
	svc := NewPlanService(&PlanComponents{
		RunReader: store,
		PlanRepo:  store,
		IDGen:     newSequenceIDGen(100),
	})

	mutation, err := svc.ArchivePlanItem(
		context.Background(),
		&ArchivePlanItemRequest{
			OpenPlanRequest: OpenPlanRequest{
				ScopeRunID: 20,
				ThreadID:   10,
				SpaceID:    30,
				UserID:     40,
			},
			TaskID: 1,
		},
	)

	require.NoError(t, err)
	require.Equal(
		t,
		entity.AgentRunPlanItemStatusCompleted,
		mutation.Item.Status,
	)
	require.False(t, mutation.Item.Active)
}

type recordingPlanRepository struct {
	run         *entity.Run
	getPlanErr  error
	plan        *entity.AgentRunPlan
	items       []*entity.AgentRunPlanItem
	reserved    *entity.AgentRunPlan
	reservation *entity.AgentRunPlan
	expected    int64
	next        int64
	archived    *entity.AgentRunPlanItem
	previous    *entity.AgentRunPlanItem
}

func (r *recordingPlanRepository) GetRun(
	context.Context,
	int64,
) (*entity.Run, error) {
	return r.run, nil
}

func (r *recordingPlanRepository) GetPlan(
	context.Context,
	int64,
) (*entity.AgentRunPlan, error) {
	return r.plan, r.getPlanErr
}

func (r *recordingPlanRepository) ReservePlanTaskID(
	_ context.Context,
	plan *entity.AgentRunPlan,
	expected int64,
	next int64,
) (*entity.AgentRunPlan, error) {
	cloned := *plan
	r.reservation = &cloned
	r.expected = expected
	r.next = next
	if r.reserved == nil {
		return nil, errors.New("missing reserved plan")
	}
	result := *r.reserved
	return &result, nil
}

func (r *recordingPlanRepository) ListPlanItems(
	context.Context,
	int64,
	bool,
) ([]*entity.AgentRunPlanItem, error) {
	return r.items, nil
}

func (r *recordingPlanRepository) GetPlanItem(
	context.Context,
	int64,
	int64,
) (*entity.AgentRunPlanItem, error) {
	return nil, repository.ErrPlanItemNotFound
}

func (r *recordingPlanRepository) UpsertPlanItem(
	context.Context,
	*entity.AgentRunPlanItem,
) (*entity.AgentRunPlanItem, *entity.AgentRunPlanItem, *entity.AgentRunPlan, bool, error) {
	return nil, nil, nil, false, errors.New("not implemented")
}

func (r *recordingPlanRepository) ArchivePlanItem(
	context.Context,
	int64,
	int64,
	int64,
) (*entity.AgentRunPlanItem, *entity.AgentRunPlanItem, *entity.AgentRunPlan, error) {
	return r.archived, r.previous, r.plan, nil
}
