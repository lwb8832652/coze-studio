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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

var canonicalMetadataKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

var (
	ErrUnsupportedPublicStateChannel = errors.New("unsupported public thread state channel")
	ErrPublicThreadStateConflict     = errors.New("public thread state conflict")
)

const canonicalPublicStateMaxJSONBytes = 64 * 1024

var canonicalProtectedMetadataKeys = map[string]struct{}{
	"space_id": {}, "user_id": {}, "owner_id": {}, "creator_id": {},
	"status": {}, "runtime": {}, "credential": {}, "credentials": {},
	"secret": {}, "token": {}, "api_key": {}, "legacy_task_id": {},
}

type CanonicalPage struct {
	Offset int32
	Limit  int32
}

type SearchThreadsRequest struct {
	SpaceID   int64
	UserID    int64
	IDs       []int64
	Status    *entity.ThreadStatus
	Metadata  map[string]any
	SortBy    string
	SortOrder string
	Page      CanonicalPage
}

type SearchRunsRequest struct {
	ThreadID    int64
	ParentRunID *int64
	Status      *entity.RunStatus
	Page        CanonicalPage
}

type ListRunEventsByCursorRequest struct {
	ThreadID     int64
	RunID        int64
	AfterEventID int64
	EventTypes   []string
	Limit        int32
}

type ListCheckpointsBeforeRequest struct {
	ThreadID           int64
	BeforeCheckpointID int64
	Limit              int32
}

type CanonicalQueryService interface {
	SearchThreads(context.Context, *SearchThreadsRequest) ([]*entity.Thread, int64, error)
	SearchRuns(context.Context, *SearchRunsRequest) ([]*entity.Run, int64, error)
	ListRunEventsByCursor(context.Context, *ListRunEventsByCursorRequest) ([]*entity.RunEvent, bool, error)
	ListCheckpointsBefore(context.Context, *ListCheckpointsBeforeRequest) ([]*entity.Checkpoint, bool, error)
}

type PatchThreadRequest struct {
	ThreadID      int64
	Title         *string
	MetadataPatch map[string]any
	UpdatedAt     int64
}

type CanonicalPatchService interface {
	PatchThread(context.Context, *PatchThreadRequest) (*entity.Thread, error)
}

type DeleteThreadIfIdleRequest struct {
	ThreadID int64
}

type CanonicalDeleteService interface {
	DeleteThreadIfIdle(context.Context, *DeleteThreadIfIdleRequest) (bool, error)
}

type PreparePublicThreadStateRequest struct {
	Values                map[string]any
	PreviousChannelValues string
	AsNode                string
}

type PreparedPublicThreadState struct {
	ChannelValues string
	Metadata      string
}

type CanonicalPublicStateService interface {
	PreparePublicThreadState(context.Context, *PreparePublicThreadStateRequest) (*PreparedPublicThreadState, error)
}

func (s *threadService) SearchThreads(
	ctx context.Context,
	req *SearchThreadsRequest,
) ([]*entity.Thread, int64, error) {
	repo, err := s.requireCanonicalQueryRepo()
	if err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("search threads request is required")
	}
	if req.SpaceID <= 0 {
		return nil, 0, InvalidArgumentErrorf("space id is required")
	}
	for _, id := range req.IDs {
		if id <= 0 {
			return nil, 0, InvalidArgumentErrorf("thread ids must be positive")
		}
	}
	if req.Status != nil && !isCanonicalThreadStatus(*req.Status) {
		return nil, 0, InvalidArgumentErrorf("thread status is invalid")
	}
	metadata, err := validateCanonicalMetadata(req.Metadata)
	if err != nil {
		return nil, 0, err
	}
	sortBy, sortOrder, err := normalizeCanonicalThreadSort(req.SortBy, req.SortOrder)
	if err != nil {
		return nil, 0, err
	}
	page, err := normalizeCanonicalPage(req.Page, 20)
	if err != nil {
		return nil, 0, err
	}

	return repo.SearchThreads(ctx, repository.SearchThreadsRequest{
		SpaceID: req.SpaceID, UserID: req.UserID,
		IDs: append([]int64(nil), req.IDs...), Status: req.Status,
		Metadata: metadata, SortBy: sortBy, SortOrder: sortOrder,
		Page: repository.CanonicalPage{Offset: page.Offset, Limit: page.Limit},
	})
}

func (s *threadService) SearchRuns(
	ctx context.Context,
	req *SearchRunsRequest,
) ([]*entity.Run, int64, error) {
	repo, err := s.requireCanonicalQueryRepo()
	if err != nil {
		return nil, 0, err
	}
	if req == nil {
		return nil, 0, InvalidArgumentErrorf("search runs request is required")
	}
	if req.ThreadID <= 0 {
		return nil, 0, InvalidArgumentErrorf("thread id is required")
	}
	if req.ParentRunID != nil && *req.ParentRunID < 0 {
		return nil, 0, InvalidArgumentErrorf("parent run id cannot be negative")
	}
	if req.Status != nil && !isCanonicalRunStatus(*req.Status) {
		return nil, 0, InvalidArgumentErrorf("run status is invalid")
	}
	page, err := normalizeCanonicalPage(req.Page, 20)
	if err != nil {
		return nil, 0, err
	}

	return repo.SearchRuns(ctx, repository.SearchRunsRequest{
		ThreadID: req.ThreadID, ParentRunID: req.ParentRunID, Status: req.Status,
		Page: repository.CanonicalPage{Offset: page.Offset, Limit: page.Limit},
	})
}

func (s *threadService) ListRunEventsByCursor(
	ctx context.Context,
	req *ListRunEventsByCursorRequest,
) ([]*entity.RunEvent, bool, error) {
	repo, err := s.requireCanonicalQueryRepo()
	if err != nil {
		return nil, false, err
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf("list run events by cursor request is required")
	}
	if req.ThreadID <= 0 || req.RunID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id and run id are required")
	}
	if req.AfterEventID < 0 {
		return nil, false, InvalidArgumentErrorf("after event id cannot be negative")
	}
	eventTypes := make([]string, 0, len(req.EventTypes))
	seen := make(map[string]struct{}, len(req.EventTypes))
	for _, raw := range req.EventTypes {
		eventType := strings.TrimSpace(raw)
		if eventType == "" {
			return nil, false, InvalidArgumentErrorf("event type cannot be empty")
		}
		if _, ok := seen[eventType]; ok {
			continue
		}
		seen[eventType] = struct{}{}
		eventTypes = append(eventTypes, eventType)
	}
	limit, err := normalizeCanonicalLimit(req.Limit, 100)
	if err != nil {
		return nil, false, err
	}
	return repo.ListRunEventsByCursor(ctx, repository.ListRunEventsByCursorRequest{
		ThreadID: req.ThreadID, RunID: req.RunID, AfterEventID: req.AfterEventID,
		EventTypes: eventTypes, Limit: limit,
	})
}

func (s *threadService) ListCheckpointsBefore(
	ctx context.Context,
	req *ListCheckpointsBeforeRequest,
) ([]*entity.Checkpoint, bool, error) {
	repo, err := s.requireCanonicalQueryRepo()
	if err != nil {
		return nil, false, err
	}
	if req == nil {
		return nil, false, InvalidArgumentErrorf("list checkpoints before request is required")
	}
	if req.ThreadID <= 0 {
		return nil, false, InvalidArgumentErrorf("thread id is required")
	}
	if req.BeforeCheckpointID < 0 {
		return nil, false, InvalidArgumentErrorf("before checkpoint id cannot be negative")
	}
	limit, err := normalizeCanonicalLimit(req.Limit, 20)
	if err != nil {
		return nil, false, err
	}
	return repo.ListCheckpointsBefore(ctx, repository.ListCheckpointsBeforeRequest{
		ThreadID: req.ThreadID, BeforeCheckpointID: req.BeforeCheckpointID, Limit: limit,
	})
}

func (s *threadService) PatchThread(
	ctx context.Context,
	req *PatchThreadRequest,
) (*entity.Thread, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, InvalidArgumentErrorf("patch thread request is required")
	}
	if req.ThreadID <= 0 {
		return nil, InvalidArgumentErrorf("thread id is required")
	}
	if req.Title == nil && len(req.MetadataPatch) == 0 {
		return nil, InvalidArgumentErrorf("thread patch has no fields")
	}

	var title *string
	if req.Title != nil {
		trimmed := strings.TrimSpace(*req.Title)
		if trimmed == "" {
			return nil, InvalidArgumentErrorf("thread title is required")
		}
		title = &trimmed
	}
	metadataPatch, err := validateCanonicalMetadataPatch(req.MetadataPatch)
	if err != nil {
		return nil, err
	}
	updatedAt := req.UpdatedAt
	if updatedAt <= 0 {
		updatedAt = time.Now().UnixMilli()
	}
	repo, ok := s.repo.(repository.CanonicalPatchRepository)
	if !ok {
		return nil, fmt.Errorf("canonical agent thread patch repository is not configured")
	}
	return repo.PatchThread(ctx, repository.PatchThreadRequest{
		ThreadID: req.ThreadID, Title: title,
		MetadataPatch: metadataPatch, UpdatedAt: updatedAt,
	})
}

func (s *threadService) DeleteThreadIfIdle(
	ctx context.Context,
	req *DeleteThreadIfIdleRequest,
) (bool, error) {
	if err := s.requireRepo(); err != nil {
		return false, err
	}
	if req == nil {
		return false, InvalidArgumentErrorf("delete thread if idle request is required")
	}
	if req.ThreadID <= 0 {
		return false, InvalidArgumentErrorf("thread id is required")
	}
	repo, ok := s.repo.(repository.CanonicalDeleteRepository)
	if !ok {
		return false, fmt.Errorf("canonical agent thread delete repository is not configured")
	}
	return repo.DeleteThreadIfIdle(ctx, repository.DeleteThreadIfIdleRequest{
		ThreadID: req.ThreadID,
	})
}

func (s *threadService) PreparePublicThreadState(
	_ context.Context,
	req *PreparePublicThreadStateRequest,
) (*PreparedPublicThreadState, error) {
	if req == nil {
		return nil, InvalidArgumentErrorf("prepare public thread state request is required")
	}
	if len(req.Values) == 0 {
		return nil, InvalidArgumentErrorf("public thread state custom object is required")
	}
	for channel := range req.Values {
		if channel != "custom" {
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedPublicStateChannel, channel)
		}
	}
	incoming, ok := req.Values["custom"].(map[string]any)
	if !ok || incoming == nil {
		return nil, InvalidArgumentErrorf("public thread state custom value must be a JSON object")
	}

	merged := make(map[string]any)
	if raw := strings.TrimSpace(req.PreviousChannelValues); raw != "" {
		var channels map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &channels); err != nil {
			return nil, fmt.Errorf("parse previous public thread state: %w", err)
		}
		if customRaw, exists := channels["custom"]; exists && string(customRaw) != "null" {
			decoder := json.NewDecoder(bytes.NewReader(customRaw))
			decoder.UseNumber()
			var previous map[string]any
			if err := decoder.Decode(&previous); err != nil || previous == nil {
				if err == nil {
					err = fmt.Errorf("custom value is not an object")
				}
				return nil, fmt.Errorf("parse previous public thread state custom object: %w", err)
			}
			for key, value := range previous {
				merged[key] = value
			}
		}
	}
	for key, value := range incoming {
		merged[key] = value
	}
	customJSON, err := json.Marshal(merged)
	if err != nil {
		return nil, InvalidArgumentErrorf("public thread state custom value must be valid JSON: %v", err)
	}
	if len(customJSON) > canonicalPublicStateMaxJSONBytes {
		return nil, InvalidArgumentErrorf("public thread state custom value exceeds 64 KiB")
	}
	channelValues, err := json.Marshal(map[string]json.RawMessage{
		"custom": customJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal public thread state: %w", err)
	}
	metadata := map[string]any{"source": "canonical_public_state"}
	if asNode := strings.TrimSpace(req.AsNode); asNode != "" {
		metadata["as_node"] = asNode
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal public thread state metadata: %w", err)
	}
	return &PreparedPublicThreadState{
		ChannelValues: string(channelValues),
		Metadata:      string(metadataJSON),
	}, nil
}

func (s *threadService) requireCanonicalQueryRepo() (repository.CanonicalQueryRepository, error) {
	if err := s.requireRepo(); err != nil {
		return nil, err
	}
	repo, ok := s.repo.(repository.CanonicalQueryRepository)
	if !ok {
		return nil, fmt.Errorf("canonical agent thread query repository is not configured")
	}
	return repo, nil
}

func validateCanonicalMetadata(metadata map[string]any) (map[string]any, error) {
	if len(metadata) > 16 {
		return nil, InvalidArgumentErrorf("metadata supports at most 16 keys")
	}
	validated := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if !canonicalMetadataKeyPattern.MatchString(key) {
			return nil, InvalidArgumentErrorf("metadata key %q is invalid", key)
		}
		if !isCanonicalMetadataScalar(value) {
			return nil, InvalidArgumentErrorf("metadata value for %q must be a scalar", key)
		}
		validated[key] = value
	}
	return validated, nil
}

func validateCanonicalMetadataPatch(metadata map[string]any) (map[string]any, error) {
	if len(metadata) > 16 {
		return nil, InvalidArgumentErrorf("metadata patch supports at most 16 keys")
	}
	validated := make(map[string]any, len(metadata))
	for key, value := range metadata {
		if !canonicalMetadataKeyPattern.MatchString(key) {
			return nil, InvalidArgumentErrorf("metadata key %q is invalid", key)
		}
		if _, protected := canonicalProtectedMetadataKeys[strings.ToLower(key)]; protected {
			return nil, InvalidArgumentErrorf("metadata key %q is protected", key)
		}
		validated[key] = value
	}
	if _, err := json.Marshal(validated); err != nil {
		return nil, InvalidArgumentErrorf("metadata patch must be valid JSON: %v", err)
	}
	return validated, nil
}

func isCanonicalMetadataScalar(value any) bool {
	switch value := value.(type) {
	case nil, string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return true
	case float32:
		return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0)
	case json.Number:
		_, err := json.Marshal(value)
		return err == nil
	default:
		return false
	}
}

func normalizeCanonicalThreadSort(sortBy, sortOrder string) (string, string, error) {
	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	if sortBy == "" {
		sortBy = "updated_at"
	}
	switch sortBy {
	case "thread_id", "status", "created_at", "updated_at":
	default:
		return "", "", InvalidArgumentErrorf("thread sort field is invalid")
	}
	sortOrder = strings.ToLower(strings.TrimSpace(sortOrder))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return "", "", InvalidArgumentErrorf("thread sort order is invalid")
	}
	return sortBy, sortOrder, nil
}

func normalizeCanonicalPage(page CanonicalPage, defaultLimit int32) (CanonicalPage, error) {
	if page.Offset < 0 {
		return CanonicalPage{}, InvalidArgumentErrorf("offset cannot be negative")
	}
	limit, err := normalizeCanonicalLimit(page.Limit, defaultLimit)
	if err != nil {
		return CanonicalPage{}, err
	}
	page.Limit = limit
	return page, nil
}

func normalizeCanonicalLimit(limit, defaultLimit int32) (int32, error) {
	if limit < 0 {
		return 0, InvalidArgumentErrorf("limit cannot be negative")
	}
	if limit == 0 {
		return defaultLimit, nil
	}
	if limit > 100 {
		return 100, nil
	}
	return limit, nil
}

func isCanonicalThreadStatus(status entity.ThreadStatus) bool {
	switch status {
	case entity.ThreadStatusIdle, entity.ThreadStatusRunning, entity.ThreadStatusFailed,
		entity.ThreadStatusCompleted, entity.ThreadStatusCanceled:
		return true
	default:
		return false
	}
}

func isCanonicalRunStatus(status entity.RunStatus) bool {
	switch status {
	case entity.RunStatusPending, entity.RunStatusQueued, entity.RunStatusRunning,
		entity.RunStatusInterrupted, entity.RunStatusSucceeded, entity.RunStatusFailed,
		entity.RunStatusCanceled:
		return true
	default:
		return false
	}
}
