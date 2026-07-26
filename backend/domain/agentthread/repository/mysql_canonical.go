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
	"fmt"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func (r *threadRepository) SearchThreads(
	ctx context.Context,
	req SearchThreadsRequest,
) ([]*entity.Thread, int64, error) {
	page, err := normalizeCanonicalRepositoryPage(req.Page, 20)
	if err != nil {
		return nil, 0, err
	}
	sortColumn, direction, err := canonicalThreadSort(req.SortBy, req.SortOrder)
	if err != nil {
		return nil, 0, err
	}

	query := r.db.WithContext(ctx).
		Model(&threadPO{}).
		Where("agent_threads.space_id = ?", req.SpaceID)
	if req.UserID > 0 {
		query = query.Where("agent_threads.creator_id = ?", req.UserID)
	}
	if len(req.IDs) > 0 {
		query = query.Where("agent_threads.id IN ?", req.IDs)
	}
	if req.Status != nil {
		query = query.Where("("+threadLifecycleStatusProjectionSQL+") = ?", string(*req.Status))
	}
	for key, value := range req.Metadata {
		if value == nil {
			query = canonicalMetadataNullQuery(query, key)
			continue
		}
		query = query.Where(datatypes.JSONQuery("metadata").Equals(value, key))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := sortColumn + " " + direction
	if sortColumn != "agent_threads.id" {
		order += ", agent_threads.id " + direction
	}
	pos := make([]*projectedThreadPO, 0, page.Limit)
	if err := query.
		Select("agent_threads.*, (" + threadLifecycleStatusProjectionSQL + ") AS projected_status").
		Order(order).
		Limit(int(page.Limit)).
		Offset(int(page.Offset)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	threads := make([]*entity.Thread, 0, len(pos))
	for _, po := range pos {
		po.Thread.Status = po.ProjectedStatus
		threads = append(threads, po.Thread.toEntity())
	}
	return threads, total, nil
}

func (r *threadRepository) SearchRuns(
	ctx context.Context,
	req SearchRunsRequest,
) ([]*entity.Run, int64, error) {
	page, err := normalizeCanonicalRepositoryPage(req.Page, 20)
	if err != nil {
		return nil, 0, err
	}
	query := r.db.WithContext(ctx).Model(&runPO{}).Where("thread_id = ?", req.ThreadID)
	if req.ParentRunID == nil {
		query = query.Where("parent_run_id = ?", 0)
	} else {
		query = query.Where("parent_run_id = ?", *req.ParentRunID)
	}
	if req.Status != nil {
		query = query.Where("status = ?", string(*req.Status))
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	pos := make([]*runPO, 0, page.Limit)
	if err := query.
		Order("created_at DESC, id DESC").
		Limit(int(page.Limit)).
		Offset(int(page.Offset)).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	runs := make([]*entity.Run, 0, len(pos))
	for _, po := range pos {
		runs = append(runs, po.toEntity())
	}
	return runs, total, nil
}

func (r *threadRepository) ListRunEventsByCursor(
	ctx context.Context,
	req ListRunEventsByCursorRequest,
) ([]*entity.RunEvent, bool, error) {
	limit := normalizeCanonicalCursorLimit(req.Limit, 100)
	query := r.db.WithContext(ctx).
		Model(&runEventPO{}).
		Where("thread_id = ?", req.ThreadID)
	if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
	}
	if req.AfterEventID > 0 {
		query = query.Where("id > ?", req.AfterEventID)
	}
	if len(req.EventTypes) > 0 {
		query = query.Where("event_type IN ?", req.EventTypes)
	}

	pos := make([]*runEventPO, 0, limit+1)
	if err := query.Order("id ASC").Limit(int(limit + 1)).Find(&pos).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(pos) > int(limit)
	if hasMore {
		pos = pos[:limit]
	}
	events := make([]*entity.RunEvent, 0, len(pos))
	for _, po := range pos {
		events = append(events, po.toEntity())
	}
	return events, hasMore, nil
}

func (r *threadRepository) ListCheckpointsBefore(
	ctx context.Context,
	req ListCheckpointsBeforeRequest,
) ([]*entity.Checkpoint, bool, error) {
	limit := normalizeCanonicalCursorLimit(req.Limit, 20)
	query := r.db.WithContext(ctx).
		Model(&checkpointPO{}).
		Where("thread_id = ?", req.ThreadID)
	if req.BeforeCheckpointID > 0 {
		query = query.Where("id < ?", req.BeforeCheckpointID)
	}

	pos := make([]*checkpointPO, 0, limit+1)
	if err := query.
		Order("created_at DESC, id DESC").
		Limit(int(limit + 1)).
		Find(&pos).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(pos) > int(limit)
	if hasMore {
		pos = pos[:limit]
	}
	checkpoints := make([]*entity.Checkpoint, 0, len(pos))
	for _, po := range pos {
		checkpoints = append(checkpoints, po.toEntity())
	}
	return checkpoints, hasMore, nil
}

func (r *threadRepository) PatchThread(
	ctx context.Context,
	req PatchThreadRequest,
) (*entity.Thread, error) {
	if req.ThreadID <= 0 {
		return nil, fmt.Errorf("thread id is required")
	}
	if req.Title == nil && len(req.MetadataPatch) == 0 {
		return nil, fmt.Errorf("thread patch has no fields")
	}
	updatedAt := req.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = time.Now().UnixMilli()
	}

	var snapshot *entity.Thread
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ?", req.ThreadID)
		if tx.Dialector.Name() != "sqlite" {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var current threadPO
		if err := query.First(&current).Error; err != nil {
			return err
		}

		updates := map[string]any{"updated_at": updatedAt}
		if req.Title != nil {
			updates["title"] = *req.Title
		}
		if len(req.MetadataPatch) > 0 {
			metadata := make(map[string]any)
			if raw := strings.TrimSpace(string(current.Metadata)); raw != "" {
				if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
					return fmt.Errorf("parse current thread metadata: %w", err)
				}
				if metadata == nil {
					metadata = make(map[string]any)
				}
			}
			for key, value := range req.MetadataPatch {
				metadata[key] = value
			}
			encoded, err := json.Marshal(metadata)
			if err != nil {
				return fmt.Errorf("marshal merged thread metadata: %w", err)
			}
			updates["metadata"] = datatypes.JSON(encoded)
		}
		if err := tx.Model(&threadPO{}).
			Where("id = ?", req.ThreadID).
			Updates(updates).Error; err != nil {
			return err
		}

		var projected projectedThreadPO
		if err := tx.Model(&threadPO{}).
			Select("agent_threads.*, ("+threadLifecycleStatusProjectionSQL+") AS projected_status").
			Where("agent_threads.id = ?", req.ThreadID).
			First(&projected).Error; err != nil {
			return err
		}
		projected.Thread.Status = projected.ProjectedStatus
		snapshot = projected.Thread.toEntity()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (r *threadRepository) DeleteThreadIfIdle(
	ctx context.Context,
	req DeleteThreadIfIdleRequest,
) (bool, error) {
	if req.ThreadID <= 0 {
		return false, fmt.Errorf("thread id is required")
	}

	var deleted bool
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// This is the same admission lock used by CreateRunBundle, so create and
		// delete have one linear order before either mutates the aggregate.
		threadQuery := tx.Where("id = ?", req.ThreadID)
		if tx.Dialector.Name() != "sqlite" {
			threadQuery = threadQuery.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var thread threadPO
		if err := threadQuery.First(&thread).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}

		activeRuns, err := lockActiveTopLevelRuns(tx, &entity.Run{
			ThreadID: req.ThreadID,
			RunKind:  entity.RunKindTask,
		}, false)
		if err != nil {
			return err
		}
		if len(activeRuns) > 0 {
			return fmt.Errorf("%w: thread %d", ErrActiveRunExists, req.ThreadID)
		}

		deleted, err = deleteThreadCascade(tx, req.ThreadID)
		return err
	})
	return deleted, err
}

func normalizeCanonicalRepositoryPage(page CanonicalPage, defaultLimit int32) (CanonicalPage, error) {
	if page.Offset < 0 {
		return CanonicalPage{}, fmt.Errorf("canonical offset cannot be negative")
	}
	if page.Limit <= 0 {
		page.Limit = defaultLimit
	}
	if page.Limit > 100 {
		page.Limit = 100
	}
	return page, nil
}

func normalizeCanonicalCursorLimit(limit, defaultLimit int32) int32 {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func canonicalThreadSort(sortBy, sortOrder string) (string, string, error) {
	var column string
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "", "updated_at":
		column = "agent_threads.updated_at"
	case "thread_id":
		column = "agent_threads.id"
	case "status":
		column = "(" + threadLifecycleStatusProjectionSQL + ")"
	case "created_at":
		column = "agent_threads.created_at"
	default:
		return "", "", fmt.Errorf("unsupported canonical thread sort %q", sortBy)
	}

	var direction string
	switch strings.ToLower(strings.TrimSpace(sortOrder)) {
	case "", "desc":
		direction = "DESC"
	case "asc":
		direction = "ASC"
	default:
		return "", "", fmt.Errorf("unsupported canonical thread sort order %q", sortOrder)
	}
	return column, direction, nil
}

func canonicalMetadataNullQuery(query *gorm.DB, key string) *gorm.DB {
	path := "$." + key
	if query.Dialector.Name() == "sqlite" {
		return query.Where("json_type(metadata, ?) = ?", path, "null")
	}
	return query.Where("JSON_TYPE(JSON_EXTRACT(metadata, ?)) = ?", path, "NULL")
}
