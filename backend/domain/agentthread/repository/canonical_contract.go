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
	"math"
	"strconv"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

var (
	ErrPublicThreadStateConflict = errors.New("public thread state conflict")
	ErrPublicThreadStateTooLarge = errors.New("public thread state exceeds size limit")
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

// ThreadGuardedRunRepository serializes a standalone Run insert with Thread
// aggregate mutations without enlarging the legacy repository contract.
type ThreadGuardedRunRepository interface {
	CreateRunWithThreadLock(context.Context, *entity.Run) error
}

type UpdatePublicThreadStateRequest struct {
	CheckpointID     int64
	ThreadID         int64
	BaseCheckpointID int64
	ChannelValues    string
	Metadata         string
	CreatedAt        int64
}

type CanonicalPublicStateRepository interface {
	UpdatePublicThreadState(context.Context, UpdatePublicThreadStateRequest) (*entity.Checkpoint, error)
}

// ValidateCanonicalMetadataNumber rejects malformed or non-finite values that
// cannot participate in the MySQL JSON metadata filter.
func ValidateCanonicalMetadataNumber(value any) error {
	switch number := value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return nil
	case float32:
		if math.IsNaN(float64(number)) || math.IsInf(float64(number), 0) {
			return fmt.Errorf("metadata number must be finite")
		}
		return nil
	case float64:
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("metadata number must be finite")
		}
		return nil
	case json.Number:
		return validateCanonicalJSONNumber(number.String())
	default:
		return fmt.Errorf("metadata value is not a supported number")
	}
}

func validateCanonicalJSONNumber(raw string) error {
	if _, err := json.Marshal(json.Number(raw)); err != nil {
		return fmt.Errorf("metadata number is invalid")
	}
	parsed, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return fmt.Errorf("metadata number is outside the finite MySQL JSON range")
	}
	return nil
}
