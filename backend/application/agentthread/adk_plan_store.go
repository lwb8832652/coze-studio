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
	"encoding/json"
	"fmt"
	"strconv"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

type ApplicationADKPlanStore struct {
	app *ApplicationService
}

func NewApplicationADKPlanStore(
	app *ApplicationService,
) *ApplicationADKPlanStore {
	return &ApplicationADKPlanStore{app: app}
}

func (s *ApplicationADKPlanStore) OpenPlan(
	ctx context.Context,
	scope ADKPlanScope,
) (*ADKPlanSnapshot, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	snapshot, err := s.app.PlanSVC.OpenPlan(
		ctx,
		domainOpenPlanRequest(scope),
	)
	if err != nil {
		return nil, err
	}
	return domainPlanSnapshot(snapshot), nil
}

func (s *ApplicationADKPlanStore) GetPlanTask(
	ctx context.Context,
	scope ADKPlanScope,
	taskID int64,
) (*ADKPlanTask, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	item, err := s.app.PlanSVC.GetPlanItem(
		ctx,
		&domainservice.GetPlanItemRequest{
			OpenPlanRequest: *domainOpenPlanRequest(scope),
			TaskID:          taskID,
		},
	)
	if err != nil {
		return nil, err
	}
	return domainPlanTask(item), nil
}

func (s *ApplicationADKPlanStore) ReservePlanTaskID(
	ctx context.Context,
	scope ADKPlanScope,
	expected int64,
	next int64,
) (*ADKPlanSnapshot, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	snapshot, err := s.app.PlanSVC.ReservePlanTaskID(
		ctx,
		&domainservice.ReservePlanTaskIDRequest{
			OpenPlanRequest: *domainOpenPlanRequest(scope),
			Expected:        expected,
			Next:            next,
		},
	)
	if err != nil {
		return nil, err
	}
	return domainPlanSnapshot(snapshot), nil
}

func (s *ApplicationADKPlanStore) UpsertPlanTask(
	ctx context.Context,
	scope ADKPlanScope,
	task *ADKPlanTask,
) (*ADKPlanMutation, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if task == nil {
		return nil, fmt.Errorf("eino adk plan task is required")
	}
	blocks, err := json.Marshal(task.Blocks)
	if err != nil {
		return nil, err
	}
	blockedBy, err := json.Marshal(task.BlockedBy)
	if err != nil {
		return nil, err
	}
	metadata := []byte("{}")
	if task.Metadata != nil {
		metadata, err = json.Marshal(task.Metadata)
		if err != nil {
			return nil, err
		}
	}
	mutation, err := s.app.PlanSVC.UpsertPlanItem(
		ctx,
		&domainservice.UpsertPlanItemRequest{
			OpenPlanRequest: *domainOpenPlanRequest(scope),
			TaskID:          task.TaskID,
			Subject:         task.Subject,
			Description:     task.Description,
			Status:          domainentity.AgentRunPlanItemStatus(task.Status),
			ActiveForm:      task.ActiveForm,
			Owner:           task.Owner,
			Blocks:          string(blocks),
			BlockedBy:       string(blockedBy),
			Metadata:        string(metadata),
		},
	)
	if err != nil {
		return nil, err
	}
	return s.completeMutation(ctx, scope, mutation)
}

func (s *ApplicationADKPlanStore) ArchivePlanTask(
	ctx context.Context,
	scope ADKPlanScope,
	taskID int64,
) (*ADKPlanMutation, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	mutation, err := s.app.PlanSVC.ArchivePlanItem(
		ctx,
		&domainservice.ArchivePlanItemRequest{
			OpenPlanRequest: *domainOpenPlanRequest(scope),
			TaskID:          taskID,
		},
	)
	if err != nil {
		return nil, err
	}
	return s.completeMutation(ctx, scope, mutation)
}

func (s *ApplicationADKPlanStore) completeMutation(
	ctx context.Context,
	scope ADKPlanScope,
	mutation *domainservice.PlanMutation,
) (*ADKPlanMutation, error) {
	if mutation == nil || mutation.Item == nil {
		return nil, fmt.Errorf("plan service returned empty mutation")
	}
	snapshot, err := s.app.PlanSVC.OpenPlan(
		ctx,
		domainOpenPlanRequest(scope),
	)
	if err != nil {
		return nil, err
	}
	return &ADKPlanMutation{
		Snapshot: domainPlanSnapshot(snapshot),
		Task:     domainPlanTask(mutation.Item),
		Previous: domainPlanTask(mutation.Previous),
		Created:  mutation.Created,
	}, nil
}

func (s *ApplicationADKPlanStore) validate() error {
	if s == nil || s.app == nil || s.app.PlanSVC == nil {
		return fmt.Errorf("agent run plan service is not configured")
	}
	return nil
}

func domainOpenPlanRequest(
	scope ADKPlanScope,
) *domainservice.OpenPlanRequest {
	return &domainservice.OpenPlanRequest{
		ScopeRunID: scope.ScopeRunID,
		ThreadID:   scope.ThreadID,
		SpaceID:    scope.SpaceID,
		UserID:     scope.UserID,
	}
}

func domainPlanSnapshot(
	snapshot *domainservice.PlanSnapshot,
) *ADKPlanSnapshot {
	if snapshot == nil || snapshot.Plan == nil {
		return nil
	}
	result := &ADKPlanSnapshot{
		HighWatermark: snapshot.Plan.HighWatermark,
		Revision:      snapshot.Plan.Revision,
		Tasks:         make([]*ADKPlanTask, 0, len(snapshot.Items)),
	}
	for _, item := range snapshot.Items {
		result.Tasks = append(result.Tasks, domainPlanTask(item))
	}
	return result
}

func domainPlanTask(
	item *domainentity.AgentRunPlanItem,
) *ADKPlanTask {
	if item == nil {
		return nil
	}
	var blocks []string
	_ = json.Unmarshal([]byte(item.Blocks), &blocks)
	var blockedBy []string
	_ = json.Unmarshal([]byte(item.BlockedBy), &blockedBy)
	var metadata map[string]any
	_ = json.Unmarshal([]byte(item.Metadata), &metadata)
	return &ADKPlanTask{
		TaskID:      item.TaskID,
		ID:          strconv.FormatInt(item.TaskID, 10),
		Subject:     item.Subject,
		Description: item.Description,
		Status:      string(item.Status),
		Blocks:      blocks,
		BlockedBy:   blockedBy,
		ActiveForm:  item.ActiveForm,
		Owner:       item.Owner,
		Metadata:    metadata,
		Active:      item.Active,
		Version:     item.Version,
		UpdatedAt:   item.UpdatedAt,
	}
}
