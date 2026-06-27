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
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	adkfilesystem "github.com/cloudwego/eino/adk/middlewares/filesystem"
	"github.com/cloudwego/eino/adk/middlewares/plantask"
)

const (
	adkPlanBaseDir         = "/plans"
	adkPlanHighWatermark   = ".highwatermark"
	maxADKPlanContentBytes = 32 * 1024
)

type ADKPlanScope struct {
	ActiveRunID int64
	ScopeRunID  int64
	ThreadID    int64
	SpaceID     int64
	UserID      int64
}

type ADKPlanTask struct {
	TaskID      int64          `json:"-"`
	ID          string         `json:"id"`
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Blocks      []string       `json:"blocks"`
	BlockedBy   []string       `json:"blockedBy"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Owner       string         `json:"owner,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Active      bool           `json:"-"`
	Version     int64          `json:"-"`
	UpdatedAt   int64          `json:"-"`
}

type ADKPlanSnapshot struct {
	HighWatermark int64
	Revision      int64
	Tasks         []*ADKPlanTask
}

type ADKPlanMutation struct {
	Snapshot *ADKPlanSnapshot
	Task     *ADKPlanTask
	Previous *ADKPlanTask
	Created  bool
}

type ADKPlanStore interface {
	OpenPlan(
		ctx context.Context,
		scope ADKPlanScope,
	) (*ADKPlanSnapshot, error)
	GetPlanTask(
		ctx context.Context,
		scope ADKPlanScope,
		taskID int64,
	) (*ADKPlanTask, error)
	ReservePlanTaskID(
		ctx context.Context,
		scope ADKPlanScope,
		expected int64,
		next int64,
	) (*ADKPlanSnapshot, error)
	UpsertPlanTask(
		ctx context.Context,
		scope ADKPlanScope,
		task *ADKPlanTask,
	) (*ADKPlanMutation, error)
	ArchivePlanTask(
		ctx context.Context,
		scope ADKPlanScope,
		taskID int64,
	) (*ADKPlanMutation, error)
}

type ADKPlanBackendFactory interface {
	Build(ctx context.Context, run *RunSummary) (plantask.Backend, error)
}

type ADKPlanBackendFactoryFunc func(
	ctx context.Context,
	run *RunSummary,
) (plantask.Backend, error)

func (f ADKPlanBackendFactoryFunc) Build(
	ctx context.Context,
	run *RunSummary,
) (plantask.Backend, error) {
	if f == nil {
		return nil, fmt.Errorf("eino adk plan backend factory is required")
	}
	return f(ctx, run)
}

type ADKPlanBackend struct {
	scope     ADKPlanScope
	store     ADKPlanStore
	eventSink RunEventSink
}

func NewADKPlanBackend(
	run *RunSummary,
	store ADKPlanStore,
	eventSink RunEventSink,
) (*ADKPlanBackend, error) {
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if run.RunID <= 0 || run.ThreadID <= 0 || run.SpaceID <= 0 ||
		run.CreatorID <= 0 {
		return nil, fmt.Errorf("run ownership is required")
	}
	if store == nil {
		return nil, fmt.Errorf("eino adk plan store is required")
	}
	scopeRunID := run.PlanScopeRunID
	if scopeRunID == 0 {
		scopeRunID = run.RunID
	}
	if scopeRunID <= 0 {
		return nil, fmt.Errorf("eino adk plan scope run id is invalid")
	}
	return &ADKPlanBackend{
		scope: ADKPlanScope{
			ActiveRunID: run.RunID,
			ScopeRunID:  scopeRunID,
			ThreadID:    run.ThreadID,
			SpaceID:     run.SpaceID,
			UserID:      run.CreatorID,
		},
		store:     store,
		eventSink: eventSink,
	}, nil
}

func (b *ADKPlanBackend) LsInfo(
	ctx context.Context,
	req *plantask.LsInfoRequest,
) ([]plantask.FileInfo, error) {
	if b == nil || b.store == nil {
		return nil, fmt.Errorf("eino adk plan backend is not configured")
	}
	if req == nil || req.Path != adkPlanBaseDir {
		return nil, fmt.Errorf("eino adk plan list path is invalid")
	}
	snapshot, err := b.store.OpenPlan(ctx, b.scope)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, fmt.Errorf("eino adk plan store returned empty snapshot")
	}
	sort.Slice(snapshot.Tasks, func(i, j int) bool {
		return snapshot.Tasks[i].TaskID < snapshot.Tasks[j].TaskID
	})
	files := []plantask.FileInfo{{
		Path: path.Join(adkPlanBaseDir, adkPlanHighWatermark),
		Size: int64(len(strconv.FormatInt(snapshot.HighWatermark, 10))),
	}}
	for _, task := range snapshot.Tasks {
		if task == nil || !task.Active {
			continue
		}
		content, err := json.Marshal(task)
		if err != nil {
			return nil, fmt.Errorf("marshal eino adk plan task: %w", err)
		}
		modifiedAt := ""
		if task.UpdatedAt > 0 {
			modifiedAt = time.UnixMilli(task.UpdatedAt).UTC().Format(time.RFC3339)
		}
		files = append(files, plantask.FileInfo{
			Path:       planTaskPath(task.TaskID),
			Size:       int64(len(content)),
			ModifiedAt: modifiedAt,
		})
	}
	return files, nil
}

func (b *ADKPlanBackend) Read(
	ctx context.Context,
	req *plantask.ReadRequest,
) (*adkfilesystem.FileContent, error) {
	if b == nil || b.store == nil {
		return nil, fmt.Errorf("eino adk plan backend is not configured")
	}
	if req == nil || req.Offset != 0 || req.Limit != 0 {
		return nil, fmt.Errorf("eino adk plan reads do not support ranges")
	}
	kind, taskID, err := parseADKPlanPath(req.FilePath)
	if err != nil {
		return nil, err
	}
	if kind == adkPlanHighWatermark {
		snapshot, err := b.store.OpenPlan(ctx, b.scope)
		if err != nil {
			return nil, err
		}
		return &adkfilesystem.FileContent{
			Content: strconv.FormatInt(snapshot.HighWatermark, 10),
		}, nil
	}
	task, err := b.store.GetPlanTask(ctx, b.scope, taskID)
	if err != nil {
		return nil, err
	}
	if task == nil || !task.Active {
		return nil, fmt.Errorf("eino adk plan task %d is not active", taskID)
	}
	content, err := json.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("marshal eino adk plan task: %w", err)
	}
	return &adkfilesystem.FileContent{Content: string(content)}, nil
}

func (b *ADKPlanBackend) Write(
	ctx context.Context,
	req *plantask.WriteRequest,
) error {
	if b == nil || b.store == nil {
		return fmt.Errorf("eino adk plan backend is not configured")
	}
	if req == nil || len(req.Content) == 0 ||
		len(req.Content) > maxADKPlanContentBytes {
		return fmt.Errorf("eino adk plan write content is invalid")
	}
	kind, taskID, err := parseADKPlanPath(req.FilePath)
	if err != nil {
		return err
	}
	if kind == adkPlanHighWatermark {
		next, err := parseCanonicalPlanID(req.Content)
		if err != nil {
			return fmt.Errorf("eino adk plan high watermark is invalid: %w", err)
		}
		if _, err := b.store.ReservePlanTaskID(
			ctx,
			b.scope,
			next-1,
			next,
		); err != nil {
			return fmt.Errorf("reserve eino adk plan task id: %w", err)
		}
		return nil
	}

	var task ADKPlanTask
	if err := json.Unmarshal([]byte(req.Content), &task); err != nil {
		return fmt.Errorf("parse eino adk plan task: %w", err)
	}
	contentTaskID, err := parseCanonicalPlanID(task.ID)
	if err != nil || contentTaskID != taskID {
		return fmt.Errorf("eino adk plan task id does not match path")
	}
	task.TaskID = taskID
	task.Active = true
	if task.Blocks == nil {
		task.Blocks = []string{}
	}
	if task.BlockedBy == nil {
		task.BlockedBy = []string{}
	}
	if err := validateADKPlanTask(&task); err != nil {
		return err
	}
	mutation, err := b.store.UpsertPlanTask(ctx, b.scope, &task)
	if err != nil {
		return err
	}
	if mutation == nil || mutation.Task == nil || mutation.Snapshot == nil {
		return fmt.Errorf("eino adk plan store returned empty mutation")
	}
	eventType := "plan.task.updated"
	if mutation.Created {
		eventType = "plan.task.created"
	} else if mutation.Task.Status == "completed" &&
		(mutation.Previous == nil || mutation.Previous.Status != "completed") {
		eventType = "plan.task.completed"
	}
	b.emitMutation(ctx, eventType, mutation)
	return nil
}

func (b *ADKPlanBackend) Delete(
	ctx context.Context,
	req *plantask.DeleteRequest,
) error {
	if b == nil || b.store == nil {
		return fmt.Errorf("eino adk plan backend is not configured")
	}
	if req == nil {
		return fmt.Errorf("eino adk plan delete request is required")
	}
	kind, taskID, err := parseADKPlanPath(req.FilePath)
	if err != nil {
		return err
	}
	if kind == adkPlanHighWatermark {
		return fmt.Errorf("eino adk plan high watermark cannot be deleted")
	}
	mutation, err := b.store.ArchivePlanTask(ctx, b.scope, taskID)
	if err != nil {
		return err
	}
	if mutation == nil || mutation.Task == nil || mutation.Snapshot == nil {
		return fmt.Errorf("eino adk plan store returned empty archive mutation")
	}
	if mutation.Task.Status == "deleted" {
		b.emitMutation(ctx, "plan.task.deleted", mutation)
	}
	return nil
}

func (b *ADKPlanBackend) emitMutation(
	ctx context.Context,
	eventType string,
	mutation *ADKPlanMutation,
) {
	activeCount := 0
	completedCount := 0
	for _, task := range mutation.Snapshot.Tasks {
		if task == nil || !task.Active {
			continue
		}
		activeCount++
		if task.Status == "completed" {
			completedCount++
		}
	}
	emitRunEvent(ctx, b.eventSink, RunEvent{
		ThreadID:  b.scope.ThreadID,
		RunID:     b.scope.ActiveRunID,
		EventType: eventType,
		Payload: encodeRunEventPayload(ctx, map[string]any{
			"plan_scope_run_id": b.scope.ScopeRunID,
			"plan_task_id":      mutation.Task.ID,
			"subject":           mutation.Task.Subject,
			"status":            mutation.Task.Status,
			"active_form":       mutation.Task.ActiveForm,
			"owner":             mutation.Task.Owner,
			"blocks":            mutation.Task.Blocks,
			"blocked_by":        mutation.Task.BlockedBy,
			"revision":          mutation.Snapshot.Revision,
			"active_count":      activeCount,
			"completed_count":   completedCount,
			"total_count":       activeCount,
		}),
	})
}

func parseADKPlanPath(filePath string) (string, int64, error) {
	if filePath == "" || strings.Contains(filePath, "\\") ||
		path.Clean(filePath) != filePath {
		return "", 0, fmt.Errorf("eino adk plan path is invalid")
	}
	for _, char := range filePath {
		if unicode.IsControl(char) {
			return "", 0, fmt.Errorf("eino adk plan path is invalid")
		}
	}
	if filePath == path.Join(adkPlanBaseDir, adkPlanHighWatermark) {
		return adkPlanHighWatermark, 0, nil
	}
	if path.Dir(filePath) != adkPlanBaseDir ||
		path.Ext(filePath) != ".json" {
		return "", 0, fmt.Errorf("eino adk plan task path is invalid")
	}
	idText := strings.TrimSuffix(path.Base(filePath), ".json")
	taskID, err := parseCanonicalPlanID(idText)
	if err != nil {
		return "", 0, fmt.Errorf("eino adk plan task path is invalid")
	}
	return "task", taskID, nil
}

func parseCanonicalPlanID(value string) (int64, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return 0, fmt.Errorf("plan task id is empty")
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != value {
		return 0, fmt.Errorf("plan task id is not canonical")
	}
	return id, nil
}

func planTaskPath(taskID int64) string {
	return path.Join(
		adkPlanBaseDir,
		strconv.FormatInt(taskID, 10)+".json",
	)
}

func validateADKPlanTask(task *ADKPlanTask) error {
	if task == nil || strings.TrimSpace(task.Subject) == "" ||
		len(task.Subject) > 512 ||
		len(task.Description) > 16*1024 ||
		len(task.ActiveForm) > 512 ||
		len(task.Owner) > 255 {
		return fmt.Errorf("eino adk plan task text is invalid")
	}
	switch task.Status {
	case "pending", "in_progress", "completed", "deleted":
	default:
		return fmt.Errorf("eino adk plan task status is invalid")
	}
	for _, ids := range [][]string{task.Blocks, task.BlockedBy} {
		for _, id := range ids {
			if _, err := parseCanonicalPlanID(id); err != nil {
				return fmt.Errorf("eino adk plan task dependency is invalid")
			}
		}
	}
	return nil
}
