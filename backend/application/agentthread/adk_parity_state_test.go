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
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKParityStateTodoReplaceIncludesExplicitEmpty(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)

	require.NoError(t, tracker.ReplaceTodos([]ADKParityTodo{{
		ID: "1", Title: "Collect requirements", Status: "in_progress",
	}}))
	require.Equal(t, []ADKParityTodo{{
		ID: "1", Title: "Collect requirements", Status: "in_progress",
	}}, tracker.Snapshot().Todos)

	require.NoError(t, tracker.ReplaceTodos([]ADKParityTodo{}))
	snapshot := tracker.Snapshot()
	require.NotNil(t, snapshot.Todos)
	require.Empty(t, snapshot.Todos)
}

func TestADKParityStateMergesUploadsAndArtifactsInStableOrder(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)

	require.NoError(t, tracker.MergeUploads([]ADKParityUpload{
		{FileID: 10, FileName: "one.txt", VirtualPath: "/mnt/user-data/uploads/one.txt"},
		{FileID: 20, FileName: "two.txt", VirtualPath: "/mnt/user-data/uploads/two.txt"},
	}))
	require.NoError(t, tracker.MergeUploads([]ADKParityUpload{
		{FileID: 11, FileName: "one.txt", VirtualPath: "/mnt/user-data/uploads/one.txt", SizeBytes: 12},
		{FileID: 30, FileName: "three.txt", VirtualPath: "/mnt/user-data/uploads/three.txt"},
	}))

	require.NoError(t, tracker.MergeArtifacts([]ADKParityArtifact{
		{ArtifactID: 100, FileID: 101, Title: "report.md", VirtualPath: "/mnt/user-data/outputs/report.md"},
		{ArtifactID: 200, FileID: 201, Title: "chart.png", VirtualPath: "/mnt/user-data/outputs/chart.png"},
	}))
	require.NoError(t, tracker.MergeArtifacts([]ADKParityArtifact{
		{ArtifactID: 300, FileID: 101, Title: "report.md", VirtualPath: "/mnt/user-data/outputs/report.md"},
		{ArtifactID: 400, FileID: 401, Title: "notes.md", VirtualPath: "/mnt/user-data/outputs/notes.md"},
	}))

	snapshot := tracker.Snapshot()
	require.Equal(t, []string{
		"/mnt/user-data/uploads/one.txt",
		"/mnt/user-data/uploads/two.txt",
		"/mnt/user-data/uploads/three.txt",
	}, parityUploadPaths(snapshot.Uploads))
	require.Equal(t, int64(11), snapshot.Uploads[0].FileID)
	require.Equal(t, []string{
		"/mnt/user-data/outputs/report.md",
		"/mnt/user-data/outputs/chart.png",
		"/mnt/user-data/outputs/notes.md",
	}, parityArtifactPaths(snapshot.Artifacts))
	require.Equal(t, int64(300), snapshot.Artifacts[0].ArtifactID)
}

func TestADKParityStatePromotedToolsAreScopedByCatalogHash(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)

	require.NoError(t, tracker.MergePromotedTools(&ADKParityPromotedTools{
		CatalogHash: "catalog-a", Names: []string{"weather", "search"},
	}))
	require.NoError(t, tracker.MergePromotedTools(&ADKParityPromotedTools{
		CatalogHash: "catalog-a", Names: []string{"search", "github"},
	}))
	require.Equal(t, &ADKParityPromotedTools{
		CatalogHash: "catalog-a", Names: []string{"weather", "search", "github"},
	}, tracker.Snapshot().PromotedTools)

	require.NoError(t, tracker.MergePromotedTools(&ADKParityPromotedTools{
		CatalogHash: "catalog-b", Names: []string{"postgres", "postgres"},
	}))
	require.Equal(t, &ADKParityPromotedTools{
		CatalogHash: "catalog-b", Names: []string{"postgres"},
	}, tracker.Snapshot().PromotedTools)

	require.NoError(t, tracker.MergePromotedTools(nil))
	require.Equal(t, []string{"postgres"}, tracker.Snapshot().PromotedTools.Names)
}

func TestADKParityStateReplacesSkillsInterruptsAndViewedImages(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)

	require.NoError(t, tracker.ReplaceActiveSkills([]ADKParitySkill{
		{ID: 1, Name: "research", Version: "v1"},
		{ID: 2, Name: "writer", Version: "v2"},
		{ID: 1, Name: "research", Version: "v3"},
	}))
	require.Equal(t, []ADKParitySkill{
		{ID: 1, Name: "research", Version: "v3"},
		{ID: 2, Name: "writer", Version: "v2"},
	}, tracker.Snapshot().ActiveSkills)

	require.NoError(t, tracker.ReplaceInterrupts([]ADKParityInterrupt{
		{ID: "approval-1", Address: "lead/tool/0", IsRootCause: true},
		{ID: "approval-1", Address: "lead/tool/1", ParentID: "parent"},
	}))
	require.Equal(t, []ADKParityInterrupt{{
		ID: "approval-1", Address: "lead/tool/1", ParentID: "parent",
	}}, tracker.Snapshot().Interrupts)

	require.NoError(t, tracker.MergeViewedImages(map[string]ADKParityViewedImage{
		"/mnt/user-data/uploads/map.png": {
			VirtualPath: "/mnt/user-data/uploads/map.png", ContentType: "image/png",
		},
	}))
	require.Len(t, tracker.Snapshot().ViewedImages, 1)
	require.NoError(t, tracker.MergeViewedImages(map[string]ADKParityViewedImage{}))
	require.Empty(t, tracker.Snapshot().ViewedImages)
}

func TestADKParityStateRejectsConflictingWorkspaceIdentity(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)
	current := tracker.Snapshot().Workspace

	require.NoError(t, tracker.MergeWorkspace(current))
	conflicting := current
	conflicting.Identity = "space:99/thread:42"
	conflicting.SpaceID = 99
	err := tracker.MergeWorkspace(conflicting)
	require.ErrorContains(t, err, "conflicting workspace identity")
	require.Equal(t, current, tracker.Snapshot().Workspace)
}

func TestADKParityStateSnapshotIsDeepCopiedAndConcurrent(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)
	require.NoError(t, tracker.MergePromotedTools(&ADKParityPromotedTools{
		CatalogHash: "catalog-a", Names: []string{"weather"},
	}))

	snapshot := tracker.Snapshot()
	snapshot.PromotedTools.Names[0] = "mutated"
	snapshot.Uploads = append(snapshot.Uploads, ADKParityUpload{
		FileName: "outside.txt", VirtualPath: "/mnt/user-data/uploads/outside.txt",
	})
	require.Equal(t, []string{"weather"}, tracker.Snapshot().PromotedTools.Names)
	require.Empty(t, tracker.Snapshot().Uploads)

	var wait sync.WaitGroup
	for index := 0; index < 16; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			require.NoError(t, tracker.MergePromotedTools(&ADKParityPromotedTools{
				CatalogHash: "catalog-a", Names: []string{"github"},
			}))
			_ = tracker.Snapshot()
		}()
	}
	wait.Wait()
	require.Equal(t, []string{"weather", "github"}, tracker.Snapshot().PromotedTools.Names)
}

func TestADKParityStateRejectsUnsafeOrOversizedValues(t *testing.T) {
	tracker := newTestADKParityStateTracker(t)

	require.Error(t, tracker.ReplaceTodos([]ADKParityTodo{{
		ID: "1", Title: "bad\x00title", Status: "pending",
	}}))
	require.Error(t, tracker.MergeUploads([]ADKParityUpload{{
		FileName: "secret.txt", VirtualPath: "/etc/secret.txt",
	}}))
	require.Error(t, tracker.MergeArtifacts([]ADKParityArtifact{{
		ArtifactID: 1, Title: "escape", VirtualPath: "/mnt/user-data/outputs/../secret.txt",
	}}))
	require.Error(t, tracker.SetCompletion(ADKParityCompletion{
		Status: "unknown", Reason: "not-valid",
	}))
	seed := tracker.Snapshot()
	seed.Revision = -1
	_, err := NewADKParityStateTracker(&RunSummary{
		RunID: 2, ThreadID: 42, SpaceID: 7, CreatorID: 9,
	}, &seed)
	require.ErrorContains(t, err, "revision")
}

func TestADKParityStateRejectsCumulativeCollectionOverflow(t *testing.T) {
	t.Run("uploads", func(t *testing.T) {
		tracker := newTestADKParityStateTracker(t)
		values := make([]ADKParityUpload, 0, maxADKParityUploads)
		for index := 0; index < maxADKParityUploads; index++ {
			name := fmt.Sprintf("upload-%d.txt", index)
			values = append(values, ADKParityUpload{
				FileName: name, VirtualPath: "/mnt/user-data/uploads/" + name,
			})
		}
		require.NoError(t, tracker.MergeUploads(values))
		require.ErrorContains(t, tracker.MergeUploads([]ADKParityUpload{{
			FileName: "overflow.txt", VirtualPath: "/mnt/user-data/uploads/overflow.txt",
		}}), "exceed")
		require.Len(t, tracker.Snapshot().Uploads, maxADKParityUploads)
	})

	t.Run("artifacts", func(t *testing.T) {
		tracker := newTestADKParityStateTracker(t)
		values := make([]ADKParityArtifact, 0, maxADKParityArtifacts)
		for index := 0; index < maxADKParityArtifacts; index++ {
			name := fmt.Sprintf("artifact-%d.md", index)
			values = append(values, ADKParityArtifact{
				Title: name, VirtualPath: "/mnt/user-data/outputs/" + name,
			})
		}
		require.NoError(t, tracker.MergeArtifacts(values))
		require.ErrorContains(t, tracker.MergeArtifacts([]ADKParityArtifact{{
			Title: "overflow.md", VirtualPath: "/mnt/user-data/outputs/overflow.md",
		}}), "exceed")
		require.Len(t, tracker.Snapshot().Artifacts, maxADKParityArtifacts)
	})

	t.Run("viewed_images", func(t *testing.T) {
		tracker := newTestADKParityStateTracker(t)
		values := make(map[string]ADKParityViewedImage, maxADKParityViewedImages)
		for index := 0; index < maxADKParityViewedImages; index++ {
			virtualPath := fmt.Sprintf("/mnt/user-data/outputs/image-%d.png", index)
			values[virtualPath] = ADKParityViewedImage{VirtualPath: virtualPath, ContentType: "image/png"}
		}
		require.NoError(t, tracker.MergeViewedImages(values))
		overflowPath := "/mnt/user-data/outputs/overflow.png"
		require.ErrorContains(t, tracker.MergeViewedImages(map[string]ADKParityViewedImage{
			overflowPath: {VirtualPath: overflowPath, ContentType: "image/png"},
		}), "exceed")
		require.Len(t, tracker.Snapshot().ViewedImages, maxADKParityViewedImages)
	})

	t.Run("promoted_tools", func(t *testing.T) {
		tracker := newTestADKParityStateTracker(t)
		names := make([]string, 0, maxADKParityPromotedTools)
		for index := 0; index < maxADKParityPromotedTools; index++ {
			names = append(names, fmt.Sprintf("tool_%d", index))
		}
		require.NoError(t, tracker.MergePromotedTools(&ADKParityPromotedTools{
			CatalogHash: "catalog", Names: names,
		}))
		require.ErrorContains(t, tracker.MergePromotedTools(&ADKParityPromotedTools{
			CatalogHash: "catalog", Names: []string{"overflow_tool"},
		}), "exceed")
		require.Len(t, tracker.Snapshot().PromotedTools.Names, maxADKParityPromotedTools)
	})
}

func newTestADKParityStateTracker(t *testing.T) *ADKParityStateTracker {
	t.Helper()
	tracker, err := NewADKParityStateTracker(&RunSummary{
		RunID: 1, ThreadID: 42, SpaceID: 7, CreatorID: 9,
	}, nil)
	require.NoError(t, err)
	return tracker
}

func parityUploadPaths(values []ADKParityUpload) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.VirtualPath)
	}
	return result
}

func parityArtifactPaths(values []ADKParityArtifact) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.VirtualPath)
	}
	return result
}
