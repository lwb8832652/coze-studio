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
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

func TestCanonicalSearchThreadsValidatesAndMapsExactPage(t *testing.T) {
	repo := newCanonicalQueryMemoryRepo()
	repo.searchThreads = []*entity.Thread{{ID: 8, SpaceID: 10, CreatorID: 20}}
	repo.searchThreadsTotal = 15
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)
	status := entity.ThreadStatusIdle

	threads, total, err := svc.SearchThreads(context.Background(), &SearchThreadsRequest{
		SpaceID: 10,
		UserID:  20,
		IDs:     []int64{8, 9},
		Status:  &status,
		Metadata: map[string]any{
			"source.kind": "canonical",
			"enabled":     true,
			"ratio":       1.5,
			"nullable":    nil,
		},
		SortBy:    "created_at",
		SortOrder: "asc",
		Page:      CanonicalPage{Offset: 7, Limit: 3},
	})

	require.NoError(t, err)
	require.Equal(t, int64(15), total)
	require.Equal(t, []*entity.Thread{{ID: 8, SpaceID: 10, CreatorID: 20}}, threads)
	require.Equal(t, repository.SearchThreadsRequest{
		SpaceID: 10,
		UserID:  20,
		IDs:     []int64{8, 9},
		Status:  &status,
		Metadata: map[string]any{
			"source.kind": "canonical",
			"enabled":     true,
			"ratio":       1.5,
			"nullable":    nil,
		},
		SortBy:    "created_at",
		SortOrder: "asc",
		Page:      repository.CanonicalPage{Offset: 7, Limit: 3},
	}, repo.searchThreadsReq)
}

func TestCanonicalSearchThreadsPreservesJSONNumberAndUnsignedMetadata(t *testing.T) {
	repo := newCanonicalQueryMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)
	largeInteger := json.Number("9223372036854775808")
	preciseDecimal := json.Number("0.123456789012345678901234567890")
	unsigned := uint64(1) << 63
	normal := int32(7)

	_, _, err := svc.SearchThreads(context.Background(), &SearchThreadsRequest{
		SpaceID: 10,
		Metadata: map[string]any{
			"large":    largeInteger,
			"precise":  preciseDecimal,
			"unsigned": unsigned,
			"normal":   normal,
		},
	})

	require.NoError(t, err)
	require.Equal(t, largeInteger, repo.searchThreadsReq.Metadata["large"])
	require.Equal(t, preciseDecimal, repo.searchThreadsReq.Metadata["precise"])
	require.Equal(t, unsigned, repo.searchThreadsReq.Metadata["unsigned"])
	require.Equal(t, normal, repo.searchThreadsReq.Metadata["normal"])
}

func TestCanonicalSearchThreadsRejectsUnsafeMetadataAndSort(t *testing.T) {
	tooMany := make(map[string]any, 17)
	for i := 0; i < 17; i++ {
		tooMany[string(rune('a'+i))] = i
	}
	tests := []struct {
		name      string
		metadata  map[string]any
		sortBy    string
		sortOrder string
	}{
		{name: "too many keys", metadata: tooMany},
		{name: "invalid key", metadata: map[string]any{"_owner": "x"}},
		{name: "object value", metadata: map[string]any{"custom": map[string]any{"nested": true}}},
		{name: "array value", metadata: map[string]any{"custom": []any{"nested"}}},
		{name: "nan value", metadata: map[string]any{"score": math.NaN()}},
		{name: "infinite value", metadata: map[string]any{"score": math.Inf(1)}},
		{name: "invalid json number", metadata: map[string]any{"score": json.Number("1.2.3")}},
		{name: "nan json number", metadata: map[string]any{"score": json.Number("NaN")}},
		{name: "infinite json number", metadata: map[string]any{"score": json.Number("Inf")}},
		{name: "unsafe sort", sortBy: "updated_at; DROP TABLE agent_threads"},
		{name: "unsafe order", sortBy: "updated_at", sortOrder: "sideways"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newCanonicalQueryMemoryRepo()
			svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)

			_, _, err := svc.SearchThreads(context.Background(), &SearchThreadsRequest{
				SpaceID: 10, Metadata: tt.metadata, SortBy: tt.sortBy, SortOrder: tt.sortOrder,
			})

			require.Error(t, err)
			require.True(t, IsClientError(err))
			require.Zero(t, repo.searchThreadsCalls)
		})
	}
}

func TestCanonicalQueryMethodsMapRunEventAndCheckpointContracts(t *testing.T) {
	repo := newCanonicalQueryMemoryRepo()
	repo.searchRuns = []*entity.Run{{ID: 30, ThreadID: 10}}
	repo.searchRunsTotal = 1
	repo.events = []*entity.RunEvent{{ID: 40, ThreadID: 10, RunID: 30}}
	repo.eventsHasMore = true
	repo.checkpoints = []*entity.Checkpoint{{ID: 50, ThreadID: 10, RunID: 30}}
	repo.checkpointsHasMore = true
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)
	parentRunID := int64(0)
	status := entity.RunStatusSucceeded

	runs, total, err := svc.SearchRuns(context.Background(), &SearchRunsRequest{
		ThreadID: 10, ParentRunID: &parentRunID, Status: &status,
		Page: CanonicalPage{Offset: 7, Limit: 3},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, int64(30), runs[0].ID)
	require.Equal(t, repository.SearchRunsRequest{
		ThreadID: 10, ParentRunID: &parentRunID, Status: &status,
		Page: repository.CanonicalPage{Offset: 7, Limit: 3},
	}, repo.searchRunsReq)

	events, hasMore, err := svc.ListRunEventsByCursor(context.Background(), &ListRunEventsByCursorRequest{
		ThreadID: 10, RunID: 30, AfterEventID: 20,
		EventTypes: []string{" message ", "status"}, Limit: 2,
	})
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Equal(t, int64(40), events[0].ID)
	require.Equal(t, []string{"message", "status"}, repo.eventsReq.EventTypes)

	checkpoints, hasMore, err := svc.ListCheckpointsBefore(context.Background(), &ListCheckpointsBeforeRequest{
		ThreadID: 10, BeforeCheckpointID: 60, Limit: 2,
	})
	require.NoError(t, err)
	require.True(t, hasMore)
	require.Equal(t, int64(50), checkpoints[0].ID)
	require.Equal(t, repository.ListCheckpointsBeforeRequest{
		ThreadID: 10, BeforeCheckpointID: 60, Limit: 2,
	}, repo.checkpointsReq)
}

func TestCanonicalQueryMethodsRejectInvalidCursorsAndPages(t *testing.T) {
	tests := []struct {
		name string
		call func(*threadService) error
	}{
		{
			name: "negative search offset",
			call: func(svc *threadService) error {
				_, _, err := svc.SearchRuns(context.Background(), &SearchRunsRequest{
					ThreadID: 10, Page: CanonicalPage{Offset: -1},
				})
				return err
			},
		},
		{
			name: "negative event cursor",
			call: func(svc *threadService) error {
				_, _, err := svc.ListRunEventsByCursor(context.Background(), &ListRunEventsByCursorRequest{
					ThreadID: 10, RunID: 20, AfterEventID: -1,
				})
				return err
			},
		},
		{
			name: "negative checkpoint cursor",
			call: func(svc *threadService) error {
				_, _, err := svc.ListCheckpointsBefore(context.Background(), &ListCheckpointsBeforeRequest{
					ThreadID: 10, BeforeCheckpointID: -1,
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newCanonicalQueryMemoryRepo()
			svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)

			err := tt.call(svc)

			require.Error(t, err)
			require.True(t, IsClientError(err))
		})
	}
}

func TestCanonicalPatchThreadValidatesAndMapsAtomicRequest(t *testing.T) {
	repo := newCanonicalQueryMemoryRepo()
	repo.patchedThread = &entity.Thread{
		ID: 10, Title: "new", Metadata: `{"source":"old","custom":"value"}`,
		UpdatedAt: 100,
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)
	title := "  new  "

	patched, err := svc.PatchThread(context.Background(), &PatchThreadRequest{
		ThreadID: 10,
		Title:    &title,
		MetadataPatch: map[string]any{
			"custom": "value",
		},
		UpdatedAt: 100,
	})

	require.NoError(t, err)
	require.Equal(t, repo.patchedThread, patched)
	require.Equal(t, 1, repo.patchThreadCalls)
	require.NotNil(t, repo.patchThreadReq.Title)
	require.Equal(t, "new", *repo.patchThreadReq.Title)
	require.Equal(t, map[string]any{"custom": "value"}, repo.patchThreadReq.MetadataPatch)
	require.Equal(t, int64(100), repo.patchThreadReq.UpdatedAt)
}

func TestCanonicalPatchThreadRejectsEmptyAndProtectedFieldsWithoutWrite(t *testing.T) {
	protected := []string{
		"space_id", "user_id", "owner_id", "creator_id", "status", "runtime",
		"credential", "credentials", "secret", "token", "api_key", "legacy_task_id",
	}
	tests := []struct {
		name string
		req  *PatchThreadRequest
	}{
		{name: "no fields", req: &PatchThreadRequest{ThreadID: 10}},
		{name: "empty title", req: &PatchThreadRequest{ThreadID: 10, Title: stringPointer("  ")}},
		{name: "invalid key", req: &PatchThreadRequest{ThreadID: 10, MetadataPatch: map[string]any{"_custom": true}}},
		{name: "invalid json", req: &PatchThreadRequest{ThreadID: 10, MetadataPatch: map[string]any{"custom": math.NaN()}}},
	}
	for _, key := range protected {
		tests = append(tests, struct {
			name string
			req  *PatchThreadRequest
		}{
			name: "protected " + key,
			req:  &PatchThreadRequest{ThreadID: 10, MetadataPatch: map[string]any{key: true}},
		})
	}
	tests = append(tests, struct {
		name string
		req  *PatchThreadRequest
	}{
		name: "protected key is case insensitive",
		req:  &PatchThreadRequest{ThreadID: 10, MetadataPatch: map[string]any{"TOKEN": true}},
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newCanonicalQueryMemoryRepo()
			svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)

			_, err := svc.PatchThread(context.Background(), tt.req)

			require.Error(t, err)
			require.True(t, IsClientError(err))
			require.Zero(t, repo.patchThreadCalls)
		})
	}
}

func TestCanonicalDeleteThreadIfIdleValidatesAndPreservesActiveRunError(t *testing.T) {
	t.Run("maps valid request", func(t *testing.T) {
		repo := newCanonicalQueryMemoryRepo()
		repo.deleteThreadIfIdleOK = true
		svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)

		deleted, err := svc.DeleteThreadIfIdle(context.Background(), &DeleteThreadIfIdleRequest{
			ThreadID: 10,
		})

		require.NoError(t, err)
		require.True(t, deleted)
		require.Equal(t, 1, repo.deleteThreadIfIdleCalls)
		require.Equal(t, repository.DeleteThreadIfIdleRequest{ThreadID: 10}, repo.deleteThreadIfIdleReq)
	})

	t.Run("preserves active run error", func(t *testing.T) {
		repo := newCanonicalQueryMemoryRepo()
		repo.deleteThreadIfIdleErr = repository.ErrActiveRunExists
		svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)

		deleted, err := svc.DeleteThreadIfIdle(context.Background(), &DeleteThreadIfIdleRequest{
			ThreadID: 10,
		})

		require.False(t, deleted)
		require.ErrorIs(t, err, ErrActiveRunExists)
		require.Equal(t, 1, repo.deleteThreadIfIdleCalls)
	})

	t.Run("rejects invalid request without delete", func(t *testing.T) {
		repo := newCanonicalQueryMemoryRepo()
		svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 100}}).(*threadService)

		deleted, err := svc.DeleteThreadIfIdle(context.Background(), &DeleteThreadIfIdleRequest{})

		require.False(t, deleted)
		require.Error(t, err)
		require.True(t, IsClientError(err))
		require.Zero(t, repo.deleteThreadIfIdleCalls)
	})
}

func TestCanonicalPreparePublicThreadStateMergesOnlyReviewedCustomValues(t *testing.T) {
	svc := NewService(&Components{
		Repo: newCanonicalQueryMemoryRepo(), IDGen: fixedIDGen{next: 100},
	}).(*threadService)

	prepared, err := svc.PreparePublicThreadState(context.Background(), &PreparePublicThreadStateRequest{
		Values: map[string]any{
			"custom": map[string]any{"keep": false, "new": "value"},
		},
		PreviousChannelValues: `{
			"custom":{"keep":true,"old":1},
			"runtime":{"checkpoint_bytes":"must-not-propagate"}
		}`,
		AsNode: "  editor  ",
	})

	require.NoError(t, err)
	require.NotNil(t, prepared)
	require.JSONEq(t, `{"custom":{"keep":false,"old":1,"new":"value"}}`, prepared.ChannelValues)
	require.JSONEq(t, `{"source":"canonical_public_state","as_node":"editor"}`, prepared.Metadata)
	require.NotContains(t, prepared.ChannelValues, "runtime")
	require.NotContains(t, prepared.ChannelValues, "checkpoint_bytes")
}

func TestCanonicalPreparePublicThreadStateRejectsUnsupportedChannels(t *testing.T) {
	unsupported := []string{
		"messages", "status", "run_status", "artifacts", "usage", "audit",
		"interrupts", "runtime", "uploaded_files", "checkpoint_bytes", "unknown",
	}
	for _, channel := range unsupported {
		t.Run(channel, func(t *testing.T) {
			svc := NewService(&Components{
				Repo: newCanonicalQueryMemoryRepo(), IDGen: fixedIDGen{next: 100},
			}).(*threadService)

			prepared, err := svc.PreparePublicThreadState(
				context.Background(),
				&PreparePublicThreadStateRequest{Values: map[string]any{channel: map[string]any{}}},
			)

			require.Nil(t, prepared)
			require.ErrorIs(t, err, ErrUnsupportedPublicStateChannel)
		})
	}
}

func TestCanonicalPreparePublicThreadStateRejectsInvalidOrOversizedCustomObject(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]any
	}{
		{name: "missing custom", values: map[string]any{}},
		{name: "custom null", values: map[string]any{"custom": nil}},
		{name: "custom string", values: map[string]any{"custom": "not-an-object"}},
		{name: "invalid json", values: map[string]any{"custom": map[string]any{"bad": math.NaN()}}},
		{name: "over 64 KiB", values: map[string]any{
			"custom": map[string]any{"value": strings.Repeat("x", 64*1024)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(&Components{
				Repo: newCanonicalQueryMemoryRepo(), IDGen: fixedIDGen{next: 100},
			}).(*threadService)

			prepared, err := svc.PreparePublicThreadState(
				context.Background(),
				&PreparePublicThreadStateRequest{Values: tt.values},
			)

			require.Nil(t, prepared)
			require.Error(t, err)
			require.True(t, IsClientError(err))
		})
	}
}

type canonicalQueryMemoryRepo struct {
	*memoryRepo
	searchThreads           []*entity.Thread
	searchThreadsTotal      int64
	searchThreadsReq        repository.SearchThreadsRequest
	searchThreadsCalls      int
	searchRuns              []*entity.Run
	searchRunsTotal         int64
	searchRunsReq           repository.SearchRunsRequest
	events                  []*entity.RunEvent
	eventsHasMore           bool
	eventsReq               repository.ListRunEventsByCursorRequest
	checkpoints             []*entity.Checkpoint
	checkpointsHasMore      bool
	checkpointsReq          repository.ListCheckpointsBeforeRequest
	patchedThread           *entity.Thread
	patchThreadReq          repository.PatchThreadRequest
	patchThreadCalls        int
	deleteThreadIfIdleOK    bool
	deleteThreadIfIdleErr   error
	deleteThreadIfIdleReq   repository.DeleteThreadIfIdleRequest
	deleteThreadIfIdleCalls int
}

func newCanonicalQueryMemoryRepo() *canonicalQueryMemoryRepo {
	return &canonicalQueryMemoryRepo{memoryRepo: newMemoryRepo()}
}

func (r *canonicalQueryMemoryRepo) SearchThreads(
	_ context.Context,
	req repository.SearchThreadsRequest,
) ([]*entity.Thread, int64, error) {
	r.searchThreadsCalls++
	r.searchThreadsReq = req
	return r.searchThreads, r.searchThreadsTotal, nil
}

func (r *canonicalQueryMemoryRepo) SearchRuns(
	_ context.Context,
	req repository.SearchRunsRequest,
) ([]*entity.Run, int64, error) {
	r.searchRunsReq = req
	return r.searchRuns, r.searchRunsTotal, nil
}

func (r *canonicalQueryMemoryRepo) ListRunEventsByCursor(
	_ context.Context,
	req repository.ListRunEventsByCursorRequest,
) ([]*entity.RunEvent, bool, error) {
	r.eventsReq = req
	return r.events, r.eventsHasMore, nil
}

func (r *canonicalQueryMemoryRepo) ListCheckpointsBefore(
	_ context.Context,
	req repository.ListCheckpointsBeforeRequest,
) ([]*entity.Checkpoint, bool, error) {
	r.checkpointsReq = req
	return r.checkpoints, r.checkpointsHasMore, nil
}

func (r *canonicalQueryMemoryRepo) PatchThread(
	_ context.Context,
	req repository.PatchThreadRequest,
) (*entity.Thread, error) {
	r.patchThreadCalls++
	r.patchThreadReq = req
	return r.patchedThread, nil
}

func (r *canonicalQueryMemoryRepo) DeleteThreadIfIdle(
	_ context.Context,
	req repository.DeleteThreadIfIdleRequest,
) (bool, error) {
	r.deleteThreadIfIdleCalls++
	r.deleteThreadIfIdleReq = req
	return r.deleteThreadIfIdleOK, r.deleteThreadIfIdleErr
}

func stringPointer(value string) *string {
	return &value
}
