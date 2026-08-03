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
	"io"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const (
	canonicalPublicStateMaxJSONBytes = 64 * 1024
	canonicalPublicStateRuntimeType  = "canonical_public_state"
	canonicalPublicStateNamespace    = "canonical.public"
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
		query, err = canonicalMetadataEqualsQuery(query, key, value)
		if err != nil {
			return nil, 0, err
		}
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
) ([]*entity.RunEvent, int64, bool, error) {
	limit := normalizeCanonicalCursorLimit(req.Limit, 100)
	query := r.db.WithContext(ctx).
		Model(&runEventPO{}).
		Where("thread_id = ?", req.ThreadID)
	if req.RunID > 0 {
		query = query.Where("run_id = ?", req.RunID)
	}
	if len(req.EventTypes) > 0 {
		query = query.Where("event_type IN ?", req.EventTypes)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, false, err
	}

	if req.AfterEventID > 0 {
		query = query.Where("id > ?", req.AfterEventID)
	}
	pos := make([]*runEventPO, 0, limit+1)
	if err := query.Order("id ASC").Limit(int(limit + 1)).Find(&pos).Error; err != nil {
		return nil, 0, false, err
	}
	hasMore := len(pos) > int(limit)
	if hasMore {
		pos = pos[:limit]
	}
	events := make([]*entity.RunEvent, 0, len(pos))
	for _, po := range pos {
		events = append(events, po.toEntity())
	}
	return events, total, hasMore, nil
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
		Order("id DESC").
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
		current, err := lockThreadForUpdate(tx, req.ThreadID)
		if err != nil {
			return err
		}

		updates := map[string]any{"updated_at": updatedAt}
		if req.Title != nil {
			updates["title"] = *req.Title
		}
		if len(req.MetadataPatch) > 0 {
			var metadata map[string]any
			if raw := strings.TrimSpace(string(current.Metadata)); raw != "" {
				metadata, err = decodeCanonicalMetadataObject(raw)
				if err != nil {
					return fmt.Errorf("parse current thread metadata: %w", err)
				}
			} else {
				metadata = make(map[string]any)
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
		if _, err := lockThreadForUpdate(tx, req.ThreadID); err != nil {
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
		activeAttempts, err := lockActiveJournalAttemptsForThread(tx, req.ThreadID)
		if err != nil {
			return err
		}
		if len(activeAttempts) > 0 {
			return fmt.Errorf("%w: thread %d has an active Journal attempt", ErrActiveRunExists, req.ThreadID)
		}

		deleted, err = deleteThreadCascade(tx, req.ThreadID)
		return err
	})
	return deleted, err
}

func (r *threadRepository) UpdatePublicThreadState(
	ctx context.Context,
	req UpdatePublicThreadStateRequest,
) (*entity.Checkpoint, error) {
	if req.CheckpointID <= 0 {
		return nil, fmt.Errorf("checkpoint id is required")
	}
	if req.ThreadID <= 0 {
		return nil, fmt.Errorf("thread id is required")
	}
	if req.BaseCheckpointID < 0 {
		return nil, fmt.Errorf("base checkpoint id cannot be negative")
	}
	createdAt := req.CreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().UnixMilli()
	}

	var snapshot *entity.Checkpoint
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := lockThreadForUpdate(tx, req.ThreadID); err != nil {
			return err
		}

		var latestRun runPO
		if err := tx.Where("thread_id = ?", req.ThreadID).
			Where("parent_run_id = ?", 0).
			Order("created_at DESC, id DESC").
			First(&latestRun).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: thread %d has no top-level run", ErrPublicThreadStateConflict, req.ThreadID)
			}
			return err
		}

		parentCheckpointID, err := selectCanonicalPublicStateParent(
			tx,
			req.ThreadID,
			req.BaseCheckpointID,
		)
		if err != nil {
			return err
		}

		previousChannelValues, previousCreatedAt, err := latestCanonicalPublicStateChannelValues(
			tx,
			req.ThreadID,
		)
		if err != nil {
			return err
		}
		if createdAt <= previousCreatedAt {
			if previousCreatedAt == 1<<63-1 {
				return fmt.Errorf("public thread state timestamp overflow")
			}
			createdAt = previousCreatedAt + 1
		}
		channelValues, err := mergeCanonicalPublicStateChannelValues(
			previousChannelValues,
			req.ChannelValues,
		)
		if err != nil {
			return err
		}
		metadata := strings.TrimSpace(req.Metadata)
		if metadata == "" {
			metadata = `{}`
		}
		if _, err := decodeCanonicalMetadataObject(metadata); err != nil {
			return fmt.Errorf("parse public thread state metadata: %w", err)
		}

		checkpoint := &entity.Checkpoint{
			ID:                 req.CheckpointID,
			ThreadID:           req.ThreadID,
			RunID:              latestRun.ID,
			ParentCheckpointID: parentCheckpointID,
			CheckpointNS:       canonicalPublicStateNamespace,
			RuntimeType:        canonicalPublicStateRuntimeType,
			RuntimeKey:         fmt.Sprintf("thread:%d", req.ThreadID),
			EnvelopeVersion:    0,
			ChannelValues:      channelValues,
			ChannelVersions:    `{}`,
			PendingSends:       `[]`,
			Metadata:           metadata,
			CreatedAt:          createdAt,
		}
		po, err := checkpointToPO(checkpoint)
		if err != nil {
			return err
		}
		if err := tx.Create(po).Error; err != nil {
			return err
		}
		snapshot = checkpoint
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func selectCanonicalPublicStateParent(
	tx *gorm.DB,
	threadID, baseCheckpointID int64,
) (int64, error) {
	query := tx.Where("thread_id = ?", threadID)
	if baseCheckpointID > 0 {
		var base checkpointPO
		if err := query.Where("id = ?", baseCheckpointID).First(&base).Error; err != nil {
			return 0, err
		}
		return base.ID, nil
	}

	var latest checkpointPO
	if err := query.Order("created_at DESC, id DESC").First(&latest).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return latest.ID, nil
}

func latestCanonicalPublicStateChannelValues(
	tx *gorm.DB,
	threadID int64,
) (string, int64, error) {
	var latest checkpointPO
	if err := tx.Where("thread_id = ?", threadID).
		Where("runtime_type = ?", canonicalPublicStateRuntimeType).
		Order("created_at DESC, id DESC").
		First(&latest).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", 0, nil
		}
		return "", 0, err
	}
	return string(latest.ChannelValues), latest.CreatedAt, nil
}

func mergeCanonicalPublicStateChannelValues(previousRaw, incomingRaw string) (string, error) {
	previous, err := decodeCanonicalPublicStateCustom(previousRaw, false)
	if err != nil {
		return "", fmt.Errorf("parse previous public thread state: %w", err)
	}
	incoming, err := decodeCanonicalPublicStateCustom(incomingRaw, true)
	if err != nil {
		return "", fmt.Errorf("parse incoming public thread state: %w", err)
	}
	for key, value := range incoming {
		previous[key] = value
	}
	customJSON, err := json.Marshal(previous)
	if err != nil {
		return "", fmt.Errorf("marshal merged public thread state: %w", err)
	}
	if len(customJSON) > canonicalPublicStateMaxJSONBytes {
		return "", ErrPublicThreadStateTooLarge
	}
	channelValues, err := json.Marshal(map[string]json.RawMessage{"custom": customJSON})
	if err != nil {
		return "", fmt.Errorf("marshal public thread state channels: %w", err)
	}
	return string(channelValues), nil
}

func decodeCanonicalPublicStateCustom(raw string, required bool) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if required {
			return nil, fmt.Errorf("custom channel is required")
		}
		return make(map[string]any), nil
	}
	var channels map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &channels); err != nil {
		return nil, err
	}
	if len(channels) != 1 {
		return nil, fmt.Errorf("only the custom channel is supported")
	}
	customRaw, ok := channels["custom"]
	if !ok {
		return nil, fmt.Errorf("custom channel is required")
	}
	return decodeCanonicalMetadataObject(string(customRaw))
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
	path := canonicalTopLevelJSONPath(key)
	if query.Dialector.Name() == "sqlite" {
		return query.Where("json_type(metadata, ?) = ?", path, "null")
	}
	return query.Where("JSON_TYPE(JSON_EXTRACT(metadata, ?)) = ?", path, "NULL")
}

func canonicalMetadataEqualsQuery(query *gorm.DB, key string, value any) (*gorm.DB, error) {
	if isCanonicalMetadataNumber(value) {
		if err := ValidateCanonicalMetadataNumber(value); err != nil {
			return nil, fmt.Errorf("metadata value for %q: %w", key, err)
		}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata value for %q: %w", key, err)
	}
	path := canonicalTopLevelJSONPath(key)
	if isCanonicalMetadataNumber(value) {
		if query.Dialector.Name() == "sqlite" {
			encodedNumber := string(encoded)
			if !strings.ContainsAny(encodedNumber, ".eE") {
				if _, err := strconv.ParseInt(encodedNumber, 10, 64); err != nil {
					if _, unsignedErr := strconv.ParseUint(encodedNumber, 10, 64); unsignedErr == nil {
						// SQLite coerces uint64-scale JSON integers to REAL, so retain
						// the exact token while MySQL keeps an UNSIGNED INTEGER.
						return query.Where("metadata -> ? = ?", path, encodedNumber), nil
					}
					return canonicalSQLiteDoubleNumberEqualsQuery(query, path, encodedNumber), nil
				}
				return query.Where(
					"json_type(metadata, ?) = ? AND JSON_EXTRACT(metadata, ?) = JSON_EXTRACT(?, '$')",
					path, "integer", path, encodedNumber,
				), nil
			}
			return canonicalSQLiteDoubleNumberEqualsQuery(query, path, encodedNumber), nil
		}
		// Match the MySQL storage type first, then its normalized numeric value so
		// integer/decimal representations remain distinct without rejecting finite
		// values that MySQL stores as DOUBLE.
		return query.Where(
			"JSON_TYPE(JSON_EXTRACT(metadata, ?)) = JSON_TYPE(JSON_EXTRACT(?, '$')) AND "+
				"JSON_EXTRACT(metadata, ?) = JSON_EXTRACT(?, '$')",
			path, string(encoded), path, string(encoded),
		), nil
	}
	return query.Where(
		"JSON_EXTRACT(metadata, ?) = JSON_EXTRACT(?, '$')",
		path, string(encoded),
	), nil
}

func canonicalSQLiteDoubleNumberEqualsQuery(
	query *gorm.DB,
	path, encodedNumber string,
) *gorm.DB {
	// MySQL stores decimal/exponent values and integers outside its signed and
	// unsigned 64-bit ranges as DOUBLE. SQLite exposes every oversized integer
	// token as REAL, so exclude the uint64-scale subset that MySQL keeps as an
	// UNSIGNED INTEGER before applying numeric equality.
	const maxMySQLUnsignedInteger = "18446744073709551615"
	return query.Where(
		"typeof(JSON_EXTRACT(metadata, ?)) = 'real' AND ("+
			"json_type(metadata, ?) = 'real' OR ("+
			"json_type(metadata, ?) = 'integer' AND ("+
			"substr(CAST(metadata -> ? AS TEXT), 1, 1) = '-' OR "+
			"length(CAST(metadata -> ? AS TEXT)) > 20 OR "+
			"(length(CAST(metadata -> ? AS TEXT)) = 20 AND "+
			"CAST(metadata -> ? AS TEXT) > CAST(? AS TEXT))"+
			"))) AND JSON_EXTRACT(metadata, ?) = JSON_EXTRACT(?, '$')",
		path,
		path,
		path,
		path,
		path,
		path,
		path,
		maxMySQLUnsignedInteger,
		path,
		encodedNumber,
	)
}

func isCanonicalMetadataNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return true
	default:
		return false
	}
}

func canonicalTopLevelJSONPath(key string) string {
	encoded, _ := json.Marshal(key)
	return "$." + string(encoded)
}

func decodeCanonicalMetadataObject(raw string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var metadata map[string]any
	if err := decoder.Decode(&metadata); err != nil {
		return nil, err
	}
	if metadata == nil {
		return nil, fmt.Errorf("metadata must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("metadata must contain exactly one JSON object")
		}
		return nil, err
	}
	return metadata, nil
}
