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
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestApplicationWriteOutputFileStoresObjectAndRegistersOutputFile(
	t *testing.T,
) {
	runtimeFiles := &recordingRuntimeFileService{}
	objectStorage := &recordingArtifactObjectReader{objects: map[string][]byte{}}
	app := &ApplicationService{
		RuntimeFileSVC:        runtimeFiles,
		ArtifactObjectStorage: objectStorage,
	}
	run := &RunSummary{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}

	resp, err := app.WriteOutputFile(context.Background(), &WriteOutputFileRequest{
		Run:         run,
		FilePath:    "/mnt/user-data/outputs/reports/report.md",
		Content:     "# Report\n",
		ContentType: "text/markdown; charset=utf-8",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.File)
	require.Equal(t, int64(99), resp.File.FileID)
	require.Equal(t, "/mnt/user-data/outputs/reports/report.md", resp.File.VirtualPath)
	require.Equal(
		t,
		"agent-runtime/30/10/runs/20/outputs/reports/report.md",
		objectStorage.key,
	)
	require.Equal(t, "# Report\n", string(objectStorage.objects[objectStorage.key]))
	require.NotNil(t, runtimeFiles.req)
	require.Equal(t, int64(20), runtimeFiles.req.RunID)
	require.Equal(t, "report.md", runtimeFiles.req.FileName)
	require.Equal(t, domainentity.AgentFileKindOutput, runtimeFiles.req.FileKind)
	require.Equal(t, "/mnt/user-data/outputs/reports/report.md", runtimeFiles.req.VirtualPath)
	require.Equal(
		t,
		"agent-runtime/30/10/runs/20/outputs/reports/report.md",
		runtimeFiles.req.ObjectURI,
	)
	require.NotContains(t, resp.Notice, "agent-runtime")
	require.Contains(t, resp.Notice, "/mnt/user-data/outputs/reports/report.md")
}

func TestApplicationWriteOutputFilePublishesTypedDocumentSnapshot(t *testing.T) {
	app, repo, _ := newJournalSnapshotApplicationTestService()
	repo.activeAttempt = &domainentity.RunAttempt{
		ThreadID: 10, JournalRunID: 20, ExecutionRunID: 20,
		AttemptID: "att-document", Status: domainentity.RunAttemptStatusRunning,
		SnapshotsEnabled: true, ProjectionState: domainentity.JournalProjectionStateHealthy,
	}
	app.JournalSnapshotAttemptReader = repo
	app.RuntimeFileSVC = &recordingRuntimeFileService{}
	objectStorage := &recordingArtifactObjectReader{objects: map[string][]byte{}}
	app.ArtifactObjectStorage = objectStorage
	run := &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30, CreatorID: 40}

	resp, err := app.WriteOutputFile(context.Background(), &WriteOutputFileRequest{
		Run: run, ToolCallID: "tool-call-document",
		FilePath: "/mnt/user-data/outputs/reports/report.md",
		Content:  "# Report\n\nSafe findings.\n", ContentType: "text/markdown; charset=utf-8",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, repo.snapshots, 1)
	projection, err := ProjectRunEventToJournal(RunEvent{
		ThreadID: run.ThreadID, RunID: run.RunID, EventType: "tool.completed",
		Payload: `{"tool_name":"write_file","tool_call_id":"tool-call-document"}`,
	})
	require.NoError(t, err)
	require.NotNil(t, projection)
	for _, snapshot := range repo.snapshots {
		require.Equal(t, projection.ActionID, snapshot.ActionID)
		snapshotEvent := repo.events[snapshot.EventID]
		require.NotNil(t, snapshotEvent)
		require.Equal(t, projection.ActionID, snapshotEvent.ActionID)
		require.Equal(t, projection.Operation, snapshotEvent.Operation)
		require.Equal(t, projection.Target, snapshotEvent.Target)
		require.Equal(t, "runtime_file", snapshot.SourceResourceType)
		require.Equal(t, "99", snapshot.SourceResourceID)
		require.Equal(t, resp.File.Digest, snapshot.SourceRevision)
		require.Equal(t, objectStorage.key, snapshot.OriginalObjectKey)
		var content JournalTypedSnapshotContent
		require.NoError(t, json.Unmarshal([]byte(snapshot.ContentJSON), &content))
		require.NotNil(t, content.Document)
		require.Equal(t, "report.md", content.Document.Title)
		require.Equal(t, "# Report\n\nSafe findings.", content.Document.Content)
		require.Equal(t, "synced", content.Document.SyncStatus)
		require.Len(t, content.Document.Chapters, 1)
	}
}

func TestApplicationWriteOutputFileKeepsToolSuccessWhenJournalProjectionFails(t *testing.T) {
	app, repo, _ := newJournalSnapshotApplicationTestService()
	repo.activeAttempt = &domainentity.RunAttempt{
		ThreadID: 10, JournalRunID: 20, ExecutionRunID: 20,
		AttemptID: "att-document", Status: domainentity.RunAttemptStatusRunning,
		SnapshotsEnabled: true, ProjectionState: domainentity.JournalProjectionStateHealthy,
	}
	repo.reserveErr = errors.New("journal unavailable")
	app.JournalSnapshotAttemptReader = repo
	app.RuntimeFileSVC = &recordingRuntimeFileService{}
	app.ArtifactObjectStorage = &recordingArtifactObjectReader{objects: map[string][]byte{}}

	resp, err := app.WriteOutputFile(context.Background(), &WriteOutputFileRequest{
		Run:        &RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30, CreatorID: 40},
		ToolCallID: "tool-call-document",
		FilePath:   "/mnt/user-data/outputs/report.md",
		Content:    "# Report\n", ContentType: "text/markdown",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Empty(t, repo.snapshots)
}

func TestApplicationCreateSkillPackageWritesInstallableSkillArchive(
	t *testing.T,
) {
	runtimeFiles := &recordingRuntimeFileService{}
	objectStorage := &recordingArtifactObjectReader{objects: map[string][]byte{}}
	app := &ApplicationService{
		RuntimeFileSVC:        runtimeFiles,
		ArtifactObjectStorage: objectStorage,
	}
	run := &RunSummary{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}

	resp, err := app.CreateSkillPackage(context.Background(), &CreateSkillPackageRequest{
		Run:       run,
		SkillName: "travel-planner",
		SkillMD: `---
name: travel-planner
description: Build concise travel plans.
---

# Travel Planner

Ask for dates before drafting.
`,
		Resources: []SkillPackageResource{
			{
				Path:    "references/checklist.md",
				Content: "# Checklist\n- dates\n- budget\n",
			},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.File)
	require.Equal(t, "/mnt/user-data/outputs/travel-planner.skill", resp.File.VirtualPath)
	require.Equal(t, "travel-planner.skill", runtimeFiles.req.FileName)
	require.Equal(t, "application/vnd.coze.skill+zip", runtimeFiles.req.ContentType)
	require.Contains(t, resp.Notice, "/mnt/user-data/outputs/travel-planner.skill")
	require.Contains(t, resp.Notice, "present_files")
	archiveBytes := objectStorage.objects[objectStorage.key]
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	require.NoError(t, err)
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	require.Contains(t, names, "travel-planner/SKILL.md")
	require.Contains(t, names, "travel-planner/references/checklist.md")
}

func TestApplicationPresentOutputFilesRegistersArtifactsAndEmitsSafeEvent(
	t *testing.T,
) {
	runtimeFiles := &recordingRuntimeFileService{
		resolved: &domainentity.AgentFile{
			ID:          90,
			SpaceID:     30,
			UserID:      40,
			ThreadID:    10,
			RunID:       20,
			FileName:    "report.md",
			FileKind:    domainentity.AgentFileKindOutput,
			VirtualPath: "/mnt/user-data/outputs/report.md",
			ObjectURI:   "agent-runtime/30/10/runs/20/outputs/report.md",
			ContentType: "text/markdown; charset=utf-8",
			SizeBytes:   128,
			Status:      domainentity.AgentFileStatusActive,
		},
	}
	artifacts := &recordingArtifactService{
		registered: &domainentity.AgentArtifact{
			ID:           100,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			Title:        "report.md",
			ArtifactType: "document",
			VirtualPath:  "/mnt/user-data/outputs/report.md",
			ContentType:  "text/markdown; charset=utf-8",
			SizeBytes:    128,
			PreviewMode:  domainentity.AgentArtifactPreviewModeText,
			Metadata:     `{"source":"present_files"}`,
		},
	}
	threadSVC := &recordingThreadService{
		appendedRunEvent: &domainentity.RunEvent{},
	}
	app := &ApplicationService{
		ThreadSVC:      threadSVC,
		RuntimeFileSVC: runtimeFiles,
		ArtifactSVC:    artifacts,
	}
	run := &RunSummary{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}

	resp, err := app.PresentOutputFiles(context.Background(), &PresentOutputFilesRequest{
		Run:       run,
		FilePaths: []string{"/mnt/user-data/outputs/report.md"},
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Len(t, resp.Artifacts, 1)
	require.Equal(t, int64(100), resp.Artifacts[0].ArtifactID)
	require.Equal(t, "/mnt/user-data/outputs/report.md", resp.Artifacts[0].VirtualPath)
	require.NotNil(t, runtimeFiles.resolveReq)
	require.Equal(t, int64(30), runtimeFiles.resolveReq.SpaceID)
	require.Equal(t, int64(10), runtimeFiles.resolveReq.ThreadID)
	require.Equal(t, int64(20), runtimeFiles.resolveReq.RunID)
	require.Equal(t, "/mnt/user-data/outputs/report.md", runtimeFiles.resolveReq.VirtualPath)
	require.NotNil(t, artifacts.registerReq)
	require.Equal(t, int64(90), artifacts.registerReq.FileID)
	require.Equal(t, "document", artifacts.registerReq.ArtifactType)
	require.JSONEq(t, `{"source":"present_files"}`, artifacts.registerReq.Metadata)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Len(t, threadSVC.appendRunEventReqs, 2)
	require.Equal(t, "artifact.presented", threadSVC.appendRunEventReqs[0].EventType)
	require.NotNil(t, threadSVC.appendRunEventReqs[0].Journal)
	require.Equal(t, "artifact.created", threadSVC.appendRunEventReqs[0].Journal.EventType)
	require.Equal(t, "verification.completed", threadSVC.appendRunEventReqs[1].EventType)
	require.NotNil(t, threadSVC.appendRunEventReqs[1].Journal)
	require.Equal(t, "verification.terminal", threadSVC.appendRunEventReqs[1].Journal.EventType)
	require.Equal(t, "completed", threadSVC.appendRunEventReqs[1].Journal.Status)
	require.NotContains(t, threadSVC.appendRunEventReqs[1].Payload, "/mnt/user-data")
	payload := map[string]any{}
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReqs[0].Payload), &payload))
	require.Equal(t, "coze.artifact_presented.v1", payload["schema"])
	require.NotContains(t, threadSVC.appendRunEventReqs[0].Payload, "agent-runtime")
	require.NotContains(t, resp.Notice, "agent-runtime")
	require.Contains(t, resp.Notice, "/mnt/user-data/outputs/report.md")
}

func TestADKArtifactToolCatalogWritesAndPresentsOutputFiles(t *testing.T) {
	runtimeFiles := &recordingRuntimeFileService{}
	objectStorage := &recordingArtifactObjectReader{objects: map[string][]byte{}}
	artifacts := &recordingArtifactService{
		registered: &domainentity.AgentArtifact{
			ID:           100,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       99,
			Title:        "report.md",
			ArtifactType: "document",
			VirtualPath:  "/mnt/user-data/outputs/report.md",
			ContentType:  "text/markdown; charset=utf-8",
			SizeBytes:    9,
			PreviewMode:  domainentity.AgentArtifactPreviewModeText,
			Metadata:     `{"source":"present_files"}`,
		},
	}
	threadSVC := &recordingThreadService{
		appendedRunEvent: &domainentity.RunEvent{},
	}
	app := &ApplicationService{
		ThreadSVC:              threadSVC,
		RuntimeFileSVC:         runtimeFiles,
		ArtifactSVC:            artifacts,
		ArtifactObjectStorage:  objectStorage,
		ArtifactReviewClock:    func() int64 { return 1000 },
		ArtifactCleanupNowFunc: func() int64 { return 1000 },
	}
	run := &RunSummary{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}
	tracker, err := NewADKParityStateTracker(run, nil)
	require.NoError(t, err)
	parityCtx := withADKParityStateTracker(context.Background(), tracker)
	provider := NewADKRuntimeToolCatalogProvider(NewADKArtifactToolCatalog(app))
	set, err := provider.ResolveToolSet(parityCtx, run)
	require.NoError(t, err)
	writeTool := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"write_file",
	)
	presentTool := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"present_files",
	)

	writeResult, err := writeTool.InvokableRun(
		context.Background(),
		`{"file_path":"/mnt/user-data/outputs/report.md","content":"# Report\n","content_type":"text/markdown; charset=utf-8"}`,
	)

	require.NoError(t, err)
	require.Contains(t, writeResult, "/mnt/user-data/outputs/report.md")
	require.NotContains(t, writeResult, "agent-runtime")
	require.NotNil(t, runtimeFiles.req)
	require.Equal(t, domainentity.AgentFileKindOutput, runtimeFiles.req.FileKind)
	runtimeFiles.resolved = &domainentity.AgentFile{
		ID:          99,
		SpaceID:     30,
		UserID:      40,
		ThreadID:    10,
		RunID:       20,
		FileName:    "report.md",
		FileKind:    domainentity.AgentFileKindOutput,
		VirtualPath: "/mnt/user-data/outputs/report.md",
		ObjectURI:   "agent-runtime/30/10/runs/20/outputs/report.md",
		ContentType: "text/markdown; charset=utf-8",
		SizeBytes:   9,
		Status:      domainentity.AgentFileStatusActive,
	}

	presentResult, err := presentTool.InvokableRun(
		parityCtx,
		`{"filepaths":["/mnt/user-data/outputs/report.md"]}`,
	)

	require.NoError(t, err)
	require.Contains(t, presentResult, "/mnt/user-data/outputs/report.md")
	require.Contains(t, presentResult, `"artifact_count":1`)
	require.NotContains(t, presentResult, "agent-runtime")
	require.NotNil(t, artifacts.registerReq)
	require.Equal(t, int64(99), artifacts.registerReq.FileID)
	require.Len(t, threadSVC.appendRunEventReqs, 2)
	require.Equal(t, "artifact.presented", threadSVC.appendRunEventReqs[0].EventType)
	require.Equal(t, "verification.completed", threadSVC.appendRunEventReqs[1].EventType)
	require.NotNil(t, threadSVC.appendRunEventReqs[1].Journal)
	require.Equal(t, []ADKParityArtifact{{
		ArtifactID: 100, FileID: 99, RunID: 20, Title: "report.md",
		ArtifactType: "document", VirtualPath: "/mnt/user-data/outputs/report.md",
		ContentType: "text/markdown; charset=utf-8", SizeBytes: 9,
		PreviewMode: "text",
	}}, tracker.Snapshot().Artifacts)
}

func TestADKArtifactToolCatalogWritesBase64OutputFile(t *testing.T) {
	runtimeFiles := &recordingRuntimeFileService{}
	objectStorage := &recordingArtifactObjectReader{objects: map[string][]byte{}}
	app := &ApplicationService{
		RuntimeFileSVC:        runtimeFiles,
		ArtifactObjectStorage: objectStorage,
	}
	run := &RunSummary{
		RunID:     20,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}
	provider := NewADKRuntimeToolCatalogProvider(NewADKArtifactToolCatalog(app))
	set, err := provider.ResolveToolSet(context.Background(), run)
	require.NoError(t, err)
	writeTool := requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"write_file",
	)
	pngBytes := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 1, 2, 0xff}
	args, err := json.Marshal(map[string]string{
		"file_path":      "/mnt/user-data/outputs/pixel.png",
		"content_base64": base64.StdEncoding.EncodeToString(pngBytes),
		"content_type":   "image/png",
	})
	require.NoError(t, err)

	writeResult, err := writeTool.InvokableRun(context.Background(), string(args))

	require.NoError(t, err)
	require.Contains(t, writeResult, "/mnt/user-data/outputs/pixel.png")
	require.NotContains(t, writeResult, "agent-runtime")
	require.NotContains(t, writeResult, base64.StdEncoding.EncodeToString(pngBytes))
	require.NotNil(t, runtimeFiles.req)
	require.Equal(t, "image/png", runtimeFiles.req.ContentType)
	require.Equal(t, domainentity.AgentFileKindOutput, runtimeFiles.req.FileKind)
	require.Equal(t, pngBytes, objectStorage.objects[objectStorage.key])

	_, err = writeTool.InvokableRun(
		context.Background(),
		`{"file_path":"/mnt/user-data/outputs/ambiguous.bin","content":"text","content_base64":"dGV4dA=="}`,
	)
	require.ErrorContains(t, err, "either content or content_base64")

	_, err = writeTool.InvokableRun(
		context.Background(),
		`{"file_path":"/mnt/user-data/outputs/missing.bin"}`,
	)
	require.ErrorContains(t, err, "requires either content or content_base64")
}

func TestDefaultADKToolProviderCanWireArtifactTools(t *testing.T) {
	app := &ApplicationService{
		ThreadSVC:             &recordingThreadService{},
		RuntimeFileSVC:        &recordingRuntimeFileService{},
		ArtifactSVC:           &recordingArtifactService{},
		ArtifactObjectStorage: &recordingArtifactObjectReader{},
	}
	provider := NewDefaultADKToolProviderWithSingleAgentSubagents(
		nil,
		WithDefaultADKToolProviderArtifactApp(app),
	)
	setProvider, ok := provider.(ADKToolSetProvider)
	require.True(t, ok)

	set, err := setProvider.ResolveToolSet(
		context.Background(),
		&RunSummary{RunID: 20, ThreadID: 10, SpaceID: 30},
	)

	require.NoError(t, err)
	require.NotNil(t, requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"write_file",
	))
	require.NotNil(t, requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"present_files",
	))
	require.NotNil(t, requireADKInvokableTool(
		t,
		context.Background(),
		set.StaticTools,
		"create_skill_package",
	))
}
