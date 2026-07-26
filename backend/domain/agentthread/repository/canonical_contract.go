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

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

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

type CanonicalQueryRepository interface {
	SearchThreads(context.Context, SearchThreadsRequest) ([]*entity.Thread, int64, error)
	SearchRuns(context.Context, SearchRunsRequest) ([]*entity.Run, int64, error)
	ListRunEventsByCursor(context.Context, ListRunEventsByCursorRequest) ([]*entity.RunEvent, bool, error)
	ListCheckpointsBefore(context.Context, ListCheckpointsBeforeRequest) ([]*entity.Checkpoint, bool, error)
}

type PatchThreadRequest struct {
	ThreadID      int64
	Title         *string
	MetadataPatch map[string]any
	UpdatedAt     int64
}

type CanonicalPatchRepository interface {
	PatchThread(context.Context, PatchThreadRequest) (*entity.Thread, error)
}

type DeleteThreadIfIdleRequest struct {
	ThreadID int64
}

type CanonicalDeleteRepository interface {
	DeleteThreadIfIdle(context.Context, DeleteThreadIfIdleRequest) (bool, error)
}
