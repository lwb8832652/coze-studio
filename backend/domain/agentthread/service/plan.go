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
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const (
	maxPlanSubjectBytes     = 512
	maxPlanDescriptionBytes = 16 * 1024
	maxPlanActiveFormBytes  = 512
	maxPlanOwnerBytes       = 255
	maxPlanJSONBytes        = 16 * 1024
)

type PlanRunReader interface {
	GetRun(ctx context.Context, id int64) (*entity.Run, error)
}

type OpenPlanRequest struct {
	ScopeRunID int64
	ThreadID   int64
	SpaceID    int64
	UserID     int64
}

type ReservePlanTaskIDRequest struct {
	OpenPlanRequest
	Expected int64
	Next     int64
}

type UpsertPlanItemRequest struct {
	OpenPlanRequest
	TaskID      int64
	Subject     string
	Description string
	Status      entity.AgentRunPlanItemStatus
	ActiveForm  string
	Owner       string
	Blocks      string
	BlockedBy   string
	Metadata    string
}

type GetPlanItemRequest struct {
	OpenPlanRequest
	TaskID int64
}

type ArchivePlanItemRequest struct {
	OpenPlanRequest
	TaskID int64
}

type PlanSnapshot struct {
	Plan  *entity.AgentRunPlan
	Items []*entity.AgentRunPlanItem
}

type PlanMutation struct {
	Plan     *entity.AgentRunPlan
	Item     *entity.AgentRunPlanItem
	Previous *entity.AgentRunPlanItem
	Created  bool
}

type PlanService interface {
	OpenPlan(ctx context.Context, req *OpenPlanRequest) (*PlanSnapshot, error)
	GetPlanItem(
		ctx context.Context,
		req *GetPlanItemRequest,
	) (*entity.AgentRunPlanItem, error)
	ReservePlanTaskID(
		ctx context.Context,
		req *ReservePlanTaskIDRequest,
	) (*PlanSnapshot, error)
	UpsertPlanItem(
		ctx context.Context,
		req *UpsertPlanItemRequest,
	) (*PlanMutation, error)
	ArchivePlanItem(
		ctx context.Context,
		req *ArchivePlanItemRequest,
	) (*PlanMutation, error)
}

type PlanComponents struct {
	RunReader PlanRunReader
	PlanRepo  repository.PlanRepository
	IDGen     idgen.IDGenerator
}

type planService struct {
	runReader PlanRunReader
	planRepo  repository.PlanRepository
	idGen     idgen.IDGenerator
}

func NewPlanService(c *PlanComponents) PlanService {
	if c == nil {
		return &planService{}
	}
	return &planService{
		runReader: c.RunReader,
		planRepo:  c.PlanRepo,
		idGen:     c.IDGen,
	}
}

func (s *planService) OpenPlan(
	ctx context.Context,
	req *OpenPlanRequest,
) (*PlanSnapshot, error) {
	run, err := s.validateScope(ctx, req)
	if err != nil {
		return nil, err
	}
	plan, err := s.planRepo.GetPlan(ctx, req.ScopeRunID)
	if errors.Is(err, repository.ErrPlanNotFound) {
		now := time.Now().UnixMilli()
		plan = planFromRun(run, now)
	} else if err != nil {
		return nil, err
	}
	items, err := s.planRepo.ListPlanItems(ctx, req.ScopeRunID, true)
	if err != nil {
		return nil, err
	}
	return &PlanSnapshot{Plan: plan, Items: items}, nil
}

func (s *planService) GetPlanItem(
	ctx context.Context,
	req *GetPlanItemRequest,
) (*entity.AgentRunPlanItem, error) {
	if req == nil || req.TaskID <= 0 {
		return nil, InvalidArgumentErrorf("plan task id is required")
	}
	if _, err := s.validateScope(ctx, &req.OpenPlanRequest); err != nil {
		return nil, err
	}
	return s.planRepo.GetPlanItem(ctx, req.ScopeRunID, req.TaskID)
}

func (s *planService) ReservePlanTaskID(
	ctx context.Context,
	req *ReservePlanTaskIDRequest,
) (*PlanSnapshot, error) {
	if req == nil {
		return nil, InvalidArgumentErrorf("reserve plan task id request is required")
	}
	if req.Expected < 0 || req.Next <= 0 || req.Next != req.Expected+1 {
		return nil, InvalidArgumentErrorf(
			"plan task id reservation must advance by one",
		)
	}
	run, err := s.validateScope(ctx, &req.OpenPlanRequest)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	plan, err := s.planRepo.ReservePlanTaskID(
		ctx,
		planFromRun(run, now),
		req.Expected,
		req.Next,
	)
	if err != nil {
		return nil, err
	}
	items, err := s.planRepo.ListPlanItems(ctx, req.ScopeRunID, true)
	if err != nil {
		return nil, err
	}
	return &PlanSnapshot{Plan: plan, Items: items}, nil
}

func (s *planService) UpsertPlanItem(
	ctx context.Context,
	req *UpsertPlanItemRequest,
) (*PlanMutation, error) {
	if req == nil {
		return nil, InvalidArgumentErrorf("upsert plan item request is required")
	}
	if req.TaskID <= 0 {
		return nil, InvalidArgumentErrorf("plan task id is required")
	}
	if err := validatePlanItemRequest(req); err != nil {
		return nil, err
	}
	if _, err := s.validateScope(ctx, &req.OpenPlanRequest); err != nil {
		return nil, err
	}
	plan, err := s.planRepo.GetPlan(ctx, req.ScopeRunID)
	if err != nil {
		return nil, err
	}
	if req.TaskID > plan.HighWatermark {
		return nil, InvalidArgumentErrorf(
			"plan task id %d exceeds high watermark %d",
			req.TaskID,
			plan.HighWatermark,
		)
	}
	id, err := s.idGen.GenID(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	stored, previous, updatedPlan, created, err := s.planRepo.UpsertPlanItem(
		ctx,
		&entity.AgentRunPlanItem{
			ID:          id,
			RunID:       req.ScopeRunID,
			TaskID:      req.TaskID,
			Subject:     strings.TrimSpace(req.Subject),
			Description: strings.TrimSpace(req.Description),
			Status:      req.Status,
			ActiveForm:  strings.TrimSpace(req.ActiveForm),
			Owner:       strings.TrimSpace(req.Owner),
			Blocks:      normalizePlanJSON(req.Blocks, "[]"),
			BlockedBy:   normalizePlanJSON(req.BlockedBy, "[]"),
			Metadata:    normalizePlanJSON(req.Metadata, "{}"),
			Active:      true,
			Version:     1,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	)
	if err != nil {
		return nil, err
	}
	return &PlanMutation{
		Plan:     updatedPlan,
		Item:     stored,
		Previous: previous,
		Created:  created,
	}, nil
}

func (s *planService) ArchivePlanItem(
	ctx context.Context,
	req *ArchivePlanItemRequest,
) (*PlanMutation, error) {
	if req == nil || req.TaskID <= 0 {
		return nil, InvalidArgumentErrorf("plan task id is required")
	}
	if _, err := s.validateScope(ctx, &req.OpenPlanRequest); err != nil {
		return nil, err
	}
	stored, previous, plan, err := s.planRepo.ArchivePlanItem(
		ctx,
		req.ScopeRunID,
		req.TaskID,
		time.Now().UnixMilli(),
	)
	if err != nil {
		return nil, err
	}
	return &PlanMutation{
		Plan:     plan,
		Item:     stored,
		Previous: previous,
	}, nil
}

func (s *planService) validateScope(
	ctx context.Context,
	req *OpenPlanRequest,
) (*entity.Run, error) {
	if s == nil || s.runReader == nil || s.planRepo == nil || s.idGen == nil {
		return nil, InvalidArgumentErrorf("plan service is not configured")
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("open plan request is required")
	}
	if req.ScopeRunID <= 0 || req.ThreadID <= 0 || req.SpaceID <= 0 ||
		req.UserID <= 0 {
		return nil, InvalidArgumentErrorf("plan scope ownership is required")
	}
	run, err := s.runReader.GetRun(ctx, req.ScopeRunID)
	if err != nil {
		return nil, err
	}
	if run == nil || run.ID != req.ScopeRunID ||
		run.ThreadID != req.ThreadID ||
		run.SpaceID != req.SpaceID ||
		run.CreatorID != req.UserID {
		return nil, InvalidArgumentErrorf("plan scope does not belong to active run")
	}
	return run, nil
}

func planFromRun(run *entity.Run, now int64) *entity.AgentRunPlan {
	return &entity.AgentRunPlan{
		RunID:     run.ID,
		ThreadID:  run.ThreadID,
		SpaceID:   run.SpaceID,
		UserID:    run.CreatorID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func validatePlanItemRequest(req *UpsertPlanItemRequest) error {
	if strings.TrimSpace(req.Subject) == "" ||
		len(req.Subject) > maxPlanSubjectBytes {
		return InvalidArgumentErrorf("plan task subject is invalid")
	}
	if len(req.Description) > maxPlanDescriptionBytes ||
		len(req.ActiveForm) > maxPlanActiveFormBytes ||
		len(req.Owner) > maxPlanOwnerBytes {
		return InvalidArgumentErrorf("plan task text exceeds limit")
	}
	switch req.Status {
	case entity.AgentRunPlanItemStatusPending,
		entity.AgentRunPlanItemStatusInProgress,
		entity.AgentRunPlanItemStatusCompleted,
		entity.AgentRunPlanItemStatusDeleted:
	default:
		return InvalidArgumentErrorf("plan task status is invalid")
	}
	if err := validatePlanTaskIDs(req.Blocks); err != nil {
		return InvalidArgumentErrorf("plan task blocks is invalid")
	}
	if err := validatePlanTaskIDs(req.BlockedBy); err != nil {
		return InvalidArgumentErrorf("plan task blocked by is invalid")
	}
	metadata := normalizePlanJSON(req.Metadata, "{}")
	if len(metadata) > maxPlanJSONBytes || !json.Valid([]byte(metadata)) {
		return InvalidArgumentErrorf("plan task metadata is invalid")
	}
	var metadataObject map[string]any
	if err := json.Unmarshal([]byte(metadata), &metadataObject); err != nil {
		return InvalidArgumentErrorf("plan task metadata must be an object")
	}
	if metadataObject == nil {
		return InvalidArgumentErrorf("plan task metadata must be an object")
	}
	return nil
}

func validatePlanTaskIDs(value string) error {
	normalized := normalizePlanJSON(value, "[]")
	if len(normalized) > maxPlanJSONBytes || !json.Valid([]byte(normalized)) {
		return errors.New("invalid json")
	}
	var ids []string
	if err := json.Unmarshal([]byte(normalized), &ids); err != nil {
		return err
	}
	for _, id := range ids {
		parsed, err := strconv.ParseInt(id, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != id {
			return errors.New("invalid task id")
		}
	}
	return nil
}

func normalizePlanJSON(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
