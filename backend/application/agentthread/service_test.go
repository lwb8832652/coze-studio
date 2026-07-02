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
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

func TestApplicationCreateThreadReturnsTaskSummary(t *testing.T) {
	domainSVC := &recordingThreadService{
		created: &entity.Thread{
			ID:        10,
			SpaceID:   1,
			CreatorID: 2,
			AgentID:   3,
			Title:     "生成周报",
			Status:    entity.ThreadStatusIdle,
			Source:    entity.ThreadSourceIM,
			CreatedAt: 100,
			UpdatedAt: 100,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID:      1,
		UserID:       2,
		AgentID:      3,
		Title:        "生成周报",
		Source:       ThreadSourceIM,
		LegacyTaskID: 4,
		Metadata:     `{"channel":"lark"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), resp.Thread.ThreadID)
	require.Equal(t, "生成周报", domainSVC.createReq.Title)
	require.Equal(t, int64(2), domainSVC.createReq.UserID)
	require.Equal(t, entity.ThreadSourceIM, domainSVC.createReq.Source)
	require.Equal(t, int64(4), domainSVC.createReq.LegacyTaskID)
	require.Equal(t, `{"channel":"lark"}`, domainSVC.createReq.Metadata)
	require.Equal(t, ThreadStatusIdle, resp.Thread.Status)
	require.Equal(t, ThreadSourceIM, resp.Thread.Source)
}

func TestApplicationCreateTaskThreadRejectsRuntimeBeforeThreadPersistence(t *testing.T) {
	domainSVC := &recordingThreadService{
		created: &entity.Thread{
			ID:        10,
			SpaceID:   1,
			CreatorID: 2,
			Title:     "新建任务",
			Status:    entity.ThreadStatusIdle,
			Source:    entity.ThreadSourceWeb,
		},
	}
	policy := RuntimePolicy{
		DefaultMode:    RuntimeModeLegacy,
		EinoADKEnabled: false,
	}
	app := &ApplicationService{
		ThreadSVC:     domainSVC,
		RuntimePolicy: &policy,
	}

	resp, err := app.CreateTaskThread(context.Background(), &CreateTaskThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Message: "请用一句话回复：smoke OK",
		Config:  `{"runtime":"eino_adk"}`,
	})

	require.Nil(t, resp)
	require.ErrorContains(t, err, "eino adk runtime is disabled by server policy")
	require.Nil(t, domainSVC.createReq)
	require.Nil(t, domainSVC.createRunReq)
	require.Nil(t, domainSVC.appendReq)
}

func TestApplicationListThreadsMapsDomainThreads(t *testing.T) {
	domainSVC := &recordingThreadService{
		listed: []*entity.Thread{
			{
				ID:        10,
				SpaceID:   1,
				CreatorID: 2,
				Title:     "新任务",
				Status:    entity.ThreadStatusRunning,
				Source:    entity.ThreadSourceWeb,
				UpdatedAt: 200,
			},
		},
		total: 1,
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	status := ThreadStatusRunning

	resp, err := app.ListThreads(context.Background(), &ListThreadsRequest{
		SpaceID:  1,
		UserID:   2,
		Status:   &status,
		Page:     2,
		PageSize: 5,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Threads, 1)
	require.Equal(t, int64(10), resp.Threads[0].ThreadID)
	require.Equal(t, ThreadStatusRunning, resp.Threads[0].Status)
	require.Equal(t, int64(2), domainSVC.listReq.UserID)
	require.NotNil(t, domainSVC.listReq.Status)
	require.Equal(t, entity.ThreadStatusRunning, *domainSVC.listReq.Status)
	require.Equal(t, int32(2), domainSVC.listReq.Page)
	require.Equal(t, int32(5), domainSVC.listReq.PageSize)
}

func TestApplicationGetThreadMapsDomainThread(t *testing.T) {
	domainSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:        10,
			SpaceID:   1,
			CreatorID: 2,
			Title:     "任务详情",
			Status:    entity.ThreadStatusCompleted,
			Source:    entity.ThreadSourceWeb,
			UpdatedAt: 200,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.GetThread(context.Background(), &GetThreadRequest{ThreadID: 10})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.getID)
	require.Equal(t, int64(10), resp.Thread.ThreadID)
	require.Equal(t, "任务详情", resp.Thread.Title)
	require.Equal(t, ThreadStatusCompleted, resp.Thread.Status)
}

func TestApplicationGetThreadRejectsNilRequestAndEmptyDomainThread(t *testing.T) {
	app := &ApplicationService{ThreadSVC: &recordingThreadService{}}

	_, err := app.GetThread(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get thread request")

	_, err = app.GetThread(context.Background(), &GetThreadRequest{ThreadID: 10})
	require.Error(t, err)
	require.Contains(t, err.Error(), "empty thread")
}

func TestApplicationAppendMessageMapsDomainMessage(t *testing.T) {
	domainSVC := &recordingThreadService{
		appended: &entity.Message{
			ID:        100,
			ThreadID:  10,
			RunID:     20,
			Role:      entity.MessageRoleUser,
			Content:   "请分析客户反馈",
			Metadata:  `{"source":"web"}`,
			CreatedAt: 300,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 10,
		RunID:    20,
		Role:     MessageRoleUser,
		Content:  "请分析客户反馈",
		Metadata: `{"source":"web"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.appendReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.appendReq.RunID)
	require.Equal(t, entity.MessageRoleUser, domainSVC.appendReq.Role)
	require.Equal(t, "请分析客户反馈", domainSVC.appendReq.Content)
	require.Equal(t, int64(100), resp.Message.MessageID)
	require.Equal(t, MessageRoleUser, resp.Message.Role)
	require.Equal(t, "请分析客户反馈", resp.Message.Content)
	require.Equal(t, `{"source":"web"}`, resp.Message.Metadata)
	require.Equal(t, int64(300), resp.Message.CreatedAt)
}

func TestApplicationListMessagesMapsDomainMessages(t *testing.T) {
	domainSVC := &recordingThreadService{
		messages: []*entity.Message{
			{
				ID:       100,
				ThreadID: 10,
				Role:     entity.MessageRoleUser,
				Content:  "第一条",
			},
			{
				ID:       101,
				ThreadID: 10,
				RunID:    20,
				Role:     entity.MessageRoleAssistant,
				Content:  "第二条",
			},
		},
		messageTotal: 2,
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.ListMessages(context.Background(), &ListMessagesRequest{
		ThreadID: 10,
		Page:     2,
		PageSize: 5,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.listMessagesReq.ThreadID)
	require.Equal(t, int32(2), domainSVC.listMessagesReq.Page)
	require.Equal(t, int32(5), domainSVC.listMessagesReq.PageSize)
	require.Equal(t, int64(2), resp.Total)
	require.Len(t, resp.Messages, 2)
	require.Equal(t, int64(100), resp.Messages[0].MessageID)
	require.Equal(t, MessageRoleUser, resp.Messages[0].Role)
	require.Equal(t, int64(101), resp.Messages[1].MessageID)
	require.Equal(t, MessageRoleAssistant, resp.Messages[1].Role)
}

func TestApplicationMemoryMethodsMapDomainMemories(t *testing.T) {
	domainSVC := &recordingThreadService{
		rememberedMemory: &entity.Memory{
			ID:                   300,
			ThreadID:             10,
			RunID:                20,
			SpaceID:              1,
			Scope:                entity.MemoryScopeThread,
			Content:              "用户偏好中文回答",
			Metadata:             `{"source":"profile"}`,
			Score:                0.9,
			Confidence:           0.88,
			SourceType:           "manual",
			SourceID:             "memory-ui",
			CorrectionOfMemoryID: 299,
			CorrectedAt:          399,
			CreatedAt:            400,
			UpdatedAt:            401,
		},
		recalledMemories: []*entity.Memory{
			{
				ID:         301,
				ThreadID:   10,
				RunID:      20,
				Scope:      entity.MemoryScopeRun,
				Content:    "本次任务需要周报",
				Score:      0.8,
				Confidence: 0.7,
				SourceType: "transcript_summary",
				SourceID:   "snapshot-1",
				DeletedAt:  0,
			},
		},
		updatedMemory: &entity.Memory{
			ID:         302,
			ThreadID:   10,
			Scope:      entity.MemoryScopeLongTerm,
			Content:    "用户偏好简短中文回答",
			Metadata:   `{"source":"edited"}`,
			Score:      0.93,
			Confidence: 0.91,
			SourceType: "manual",
			SourceID:   "memory-ui-edited",
			DeletedAt:  0,
			UpdatedAt:  500,
		},
		deleteMemoryOK:   true,
		clearMemoryCount: 2,
		memoryTotal:      1,
		memoryAuditEvents: []*entity.MemoryAuditEvent{
			{
				ID:            701,
				ThreadID:      10,
				RunID:         20,
				SpaceID:       1,
				MemoryID:      302,
				ActorID:       7,
				EventType:     "memory.restored",
				Scope:         entity.MemoryScopeLongTerm,
				SourceType:    "manual",
				SourceID:      "memory-ui-edited",
				AffectedCount: 1,
				CreatedAt:     600,
			},
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	rememberResp, err := app.RememberMemory(context.Background(), &RememberMemoryRequest{
		ThreadID:             10,
		RunID:                20,
		Content:              "用户偏好中文回答",
		Score:                0.9,
		Confidence:           0.88,
		SourceType:           " manual ",
		SourceID:             " memory-ui ",
		CorrectionOfMemoryID: 299,
		CorrectedAt:          399,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.rememberMemoryReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.rememberMemoryReq.RunID)
	require.Equal(t, 0.88, domainSVC.rememberMemoryReq.Confidence)
	require.Equal(t, " manual ", domainSVC.rememberMemoryReq.SourceType)
	require.Equal(t, " memory-ui ", domainSVC.rememberMemoryReq.SourceID)
	require.Equal(t, int64(299), domainSVC.rememberMemoryReq.CorrectionOfMemoryID)
	require.Equal(t, int64(300), rememberResp.Memory.MemoryID)
	require.Equal(t, MemoryScopeThread, rememberResp.Memory.Scope)
	require.Equal(t, "用户偏好中文回答", rememberResp.Memory.Content)
	require.Equal(t, 0.88, rememberResp.Memory.Confidence)
	require.Equal(t, "manual", rememberResp.Memory.SourceType)
	require.Equal(t, "memory-ui", rememberResp.Memory.SourceID)
	require.Equal(t, int64(299), rememberResp.Memory.CorrectionOfMemoryID)
	require.Equal(t, int64(399), rememberResp.Memory.CorrectedAt)

	recallResp, err := app.RecallMemories(context.Background(), &RecallMemoriesRequest{
		ThreadID: 10,
		RunID:    20,
		Limit:    3,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.recallMemoriesReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.recallMemoriesReq.RunID)
	require.Equal(t, int32(3), domainSVC.recallMemoriesReq.Limit)
	require.Equal(t, int64(1), recallResp.Total)
	require.Len(t, recallResp.Memories, 1)
	require.Equal(t, int64(301), recallResp.Memories[0].MemoryID)
	require.Equal(t, MemoryScopeRun, recallResp.Memories[0].Scope)
	require.Equal(t, 0.7, recallResp.Memories[0].Confidence)
	require.Equal(t, "transcript_summary", recallResp.Memories[0].SourceType)
	require.Equal(t, "snapshot-1", recallResp.Memories[0].SourceID)
	require.Zero(t, recallResp.Memories[0].DeletedAt)

	listResp, err := app.ListMemories(context.Background(), &ListMemoriesRequest{
		ThreadID:       10,
		Query:          "中文",
		Scopes:         []MemoryScope{MemoryScopeLongTerm},
		IncludeExpired: true,
		Page:           1,
		PageSize:       20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.listMemoriesReq.ThreadID)
	require.Equal(t, "中文", domainSVC.listMemoriesReq.Query)
	require.Equal(t, []entity.MemoryScope{entity.MemoryScopeLongTerm}, domainSVC.listMemoriesReq.Scopes)
	require.True(t, domainSVC.listMemoriesReq.IncludeExpired)
	require.Equal(t, int64(1), listResp.Total)
	require.Len(t, listResp.Memories, 1)

	updateResp, err := app.UpdateMemory(context.Background(), &UpdateMemoryRequest{
		ThreadID:   10,
		MemoryID:   302,
		Scope:      MemoryScopeLongTerm,
		Content:    "用户偏好简短中文回答",
		Metadata:   `{"source":"edited"}`,
		Score:      0.93,
		Confidence: 0.91,
		SourceType: "manual",
		SourceID:   "memory-ui-edited",
	})
	require.NoError(t, err)
	require.True(t, updateResp.Updated)
	require.Equal(t, int64(302), domainSVC.updateMemoryReq.MemoryID)
	require.Equal(t, entity.MemoryScopeLongTerm, domainSVC.updateMemoryReq.Scope)
	require.Equal(t, "用户偏好简短中文回答", updateResp.Memory.Content)
	require.Equal(t, "memory-ui-edited", updateResp.Memory.SourceID)

	deleteResp, err := app.DeleteMemory(context.Background(), &DeleteMemoryRequest{
		ThreadID: 10,
		MemoryID: 302,
	})
	require.NoError(t, err)
	require.True(t, deleteResp.Deleted)
	require.Equal(t, int64(302), domainSVC.deleteMemoryReq.MemoryID)

	clearResp, err := app.ClearMemories(context.Background(), &ClearMemoriesRequest{
		ThreadID: 10,
		Scopes:   []MemoryScope{MemoryScopeLongTerm},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), clearResp.Deleted)
	require.Equal(t, []entity.MemoryScope{entity.MemoryScopeLongTerm}, domainSVC.clearMemoriesReq.Scopes)

	restoreResp, err := app.RestoreMemory(context.Background(), &RestoreMemoryRequest{
		ThreadID: 10,
		MemoryID: 302,
		ActorID:  7,
	})
	require.NoError(t, err)
	require.True(t, restoreResp.Restored)
	require.Equal(t, int64(302), domainSVC.restoreMemoryReq.MemoryID)
	require.Equal(t, int64(7), domainSVC.restoreMemoryReq.ActorID)
	require.Equal(t, "用户偏好简短中文回答", restoreResp.Memory.Content)

	auditResp, err := app.ListMemoryAuditEvents(context.Background(), &ListMemoryAuditEventsRequest{
		ThreadID: 10,
		MemoryID: 302,
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.listMemoryAuditReq.ThreadID)
	require.Equal(t, int64(302), domainSVC.listMemoryAuditReq.MemoryID)
	require.Equal(t, int64(1), auditResp.Total)
	require.Len(t, auditResp.Events, 1)
	require.Equal(t, int64(701), auditResp.Events[0].EventID)
	require.Equal(t, "memory.restored", auditResp.Events[0].EventType)
	require.Equal(t, MemoryScopeLongTerm, auditResp.Events[0].Scope)
}

func TestApplicationProcessMemoryFlushJobsExtractsMemoriesAndCompletesJob(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedMemoryFlushJobs: []*entity.MemoryFlushJob{
			{
				ID:                   900,
				ThreadID:             10,
				RunID:                20,
				SpaceID:              30,
				UserID:               40,
				AssistantID:          "assistant-a",
				TranscriptSnapshotID: 501,
				Status:               entity.MemoryFlushJobStatusProcessing,
				WorkerID:             "memory-worker-a",
				AttemptCount:         1,
			},
		},
		gotTranscriptSnapshot: &entity.TranscriptSnapshot{
			ID:             501,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			Kind:           entity.TranscriptKindTerminal,
			Digest:         strings.Repeat("a", 64),
			IdempotencyKey: "terminal:" + strings.Repeat("a", 64),
			MessageCount:   2,
			Messages:       `[{"role":"user","content":"请记住我偏好中文回答"},{"role":"assistant","content":"好的"}]`,
			Metadata:       `{"runtime":"eino_adk"}`,
		},
		recalledMemories: []*entity.Memory{
			{
				ID:         401,
				ThreadID:   10,
				RunID:      20,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "正在推进 DeerFlow parity 主线",
				Metadata:   `{"deerflow_section":"user.workContext","path":"/private/secret"}`,
				Confidence: 0.88,
				SourceType: "manual",
				SourceID:   "private-source-id",
			},
			{
				ID:         402,
				ThreadID:   10,
				RunID:      20,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "用户偏好中文回答",
				Metadata:   `{"category":"preference","path":"/private/secret"}`,
				Confidence: 0.92,
				SourceType: "manual",
				SourceID:   "private-source-id",
			},
		},
		rememberedMemories: []*entity.Memory{
			{
				ID:         301,
				ThreadID:   10,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "用户偏好中文回答",
				Confidence: 0.92,
				SourceType: "transcript_summary",
				SourceID:   "snapshot:501:language",
			},
		},
		completedMemoryFlushJob: &entity.MemoryFlushJob{
			ID:     900,
			Status: entity.MemoryFlushJobStatusSucceeded,
		},
		memoryFlushUpdated: true,
	}
	extractor := &recordingMemoryExtractor{
		facts: []MemoryExtractionFact{
			{
				Key:        "language",
				Scope:      MemoryScopeLongTerm,
				Content:    "用户偏好中文回答",
				Metadata:   `{"category":"preference"}`,
				Score:      0.8,
				Confidence: 0.92,
			},
		},
	}
	app := &ApplicationService{
		ThreadSVC:       domainSVC,
		MemoryExtractor: extractor,
	}

	resp, err := app.ProcessMemoryFlushJobs(context.Background(), &ProcessMemoryFlushJobsRequest{
		WorkerID:           "memory-worker-a",
		Limit:              2,
		LeaseTTLMillis:     60000,
		MaxAttempts:        3,
		RetryBackoffMillis: 120000,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int32(1), resp.Claimed)
	require.Equal(t, int32(1), resp.Succeeded)
	require.Equal(t, int32(0), resp.Failed)
	require.Equal(t, "memory-worker-a", domainSVC.claimMemoryFlushReq.WorkerID)
	require.Equal(t, int32(2), domainSVC.claimMemoryFlushReq.Limit)
	require.Equal(t, int64(60000), domainSVC.claimMemoryFlushReq.LeaseTTLMillis)
	require.Equal(t, int64(501), domainSVC.getTranscriptSnapshotReq.SnapshotID)
	require.Equal(t, int64(10), extractor.req.ThreadID)
	require.Equal(t, int64(20), extractor.req.RunID)
	require.Equal(t, int64(501), extractor.req.SnapshotID)
	require.Equal(t, TranscriptKindTerminal, extractor.req.Kind)
	require.Contains(t, extractor.req.Messages, "偏好中文")
	require.Contains(t, extractor.req.CurrentMemory, "正在推进 DeerFlow parity 主线")
	require.Contains(t, extractor.req.CurrentMemory, `"workContext"`)
	require.Contains(t, extractor.req.CurrentMemory, `"preference"`)
	require.NotContains(t, extractor.req.CurrentMemory, "/private/secret")
	require.NotContains(t, extractor.req.CurrentMemory, "private-source-id")
	require.Len(t, domainSVC.rememberMemoryReqs, 1)
	require.Equal(t, int64(10), domainSVC.rememberMemoryReqs[0].ThreadID)
	require.Equal(t, int64(20), domainSVC.rememberMemoryReqs[0].RunID)
	require.Equal(t, entity.MemoryScopeLongTerm, domainSVC.rememberMemoryReqs[0].Scope)
	require.Equal(t, "用户偏好中文回答", domainSVC.rememberMemoryReqs[0].Content)
	require.Equal(t, 0.92, domainSVC.rememberMemoryReqs[0].Confidence)
	require.Equal(t, "transcript_summary", domainSVC.rememberMemoryReqs[0].SourceType)
	require.Equal(t, "snapshot:501:language", domainSVC.rememberMemoryReqs[0].SourceID)
	require.Equal(t, int64(900), domainSVC.completeMemoryFlushReq.JobID)
	require.Equal(t, "memory-worker-a", domainSVC.completeMemoryFlushReq.WorkerID)
	require.Nil(t, domainSVC.retryMemoryFlushReq)
	require.NotNil(t, domainSVC.appendRunEventReq)
	require.Equal(t, "memory.update_completed", domainSVC.appendRunEventReq.EventType)
	require.NotContains(t, domainSVC.appendRunEventReq.Payload, "偏好中文")
	require.NotContains(t, domainSVC.appendRunEventReq.Payload, "用户偏好中文回答")
}

func TestApplicationProcessMemoryFlushJobsRetriesExtractorFailureWithoutTranscriptLeak(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedMemoryFlushJobs: []*entity.MemoryFlushJob{
			{
				ID:                   901,
				ThreadID:             10,
				RunID:                20,
				SpaceID:              30,
				TranscriptSnapshotID: 502,
				Status:               entity.MemoryFlushJobStatusProcessing,
				WorkerID:             "memory-worker-a",
				AttemptCount:         1,
			},
		},
		gotTranscriptSnapshot: &entity.TranscriptSnapshot{
			ID:             502,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			Kind:           entity.TranscriptKindTerminal,
			Digest:         strings.Repeat("b", 64),
			IdempotencyKey: "terminal:" + strings.Repeat("b", 64),
			MessageCount:   1,
			Messages:       `[{"role":"user","content":"secret transcript body"}]`,
			Metadata:       `{"runtime":"eino_adk"}`,
		},
		retriedMemoryFlushJob: &entity.MemoryFlushJob{
			ID:     901,
			Status: entity.MemoryFlushJobStatusPending,
		},
		memoryFlushUpdated: true,
	}
	app := &ApplicationService{
		ThreadSVC: domainSVC,
		MemoryExtractor: &recordingMemoryExtractor{
			err: fmt.Errorf("extractor failed on secret transcript body"),
		},
	}

	resp, err := app.ProcessMemoryFlushJobs(context.Background(), &ProcessMemoryFlushJobsRequest{
		WorkerID:           "memory-worker-a",
		Limit:              1,
		MaxAttempts:        3,
		RetryBackoffMillis: 60000,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int32(1), resp.Claimed)
	require.Equal(t, int32(1), resp.Retried)
	require.Equal(t, int32(0), resp.Failed)
	require.Nil(t, domainSVC.completeMemoryFlushReq)
	require.Nil(t, domainSVC.failMemoryFlushReq)
	require.NotNil(t, domainSVC.retryMemoryFlushReq)
	require.Equal(t, int64(901), domainSVC.retryMemoryFlushReq.JobID)
	require.Equal(t, "memory-worker-a", domainSVC.retryMemoryFlushReq.WorkerID)
	require.Equal(t, "memory extraction failed: extractor_failed", domainSVC.retryMemoryFlushReq.ErrorText)
	require.GreaterOrEqual(t, domainSVC.retryMemoryFlushReq.AvailableAt-domainSVC.retryMemoryFlushReq.Now, int64(60000))
	require.NotContains(t, domainSVC.retryMemoryFlushReq.ErrorText, "secret transcript body")
	require.Empty(t, domainSVC.rememberMemoryReqs)
}

func TestApplicationProcessMemoryFlushJobsAppliesFactsToRemove(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedMemoryFlushJobs: []*entity.MemoryFlushJob{
			{
				ID:                   902,
				ThreadID:             10,
				RunID:                20,
				SpaceID:              30,
				UserID:               40,
				TranscriptSnapshotID: 503,
				Status:               entity.MemoryFlushJobStatusProcessing,
				WorkerID:             "memory-worker-a",
				AttemptCount:         1,
			},
		},
		gotTranscriptSnapshot: &entity.TranscriptSnapshot{
			ID:             503,
			ThreadID:       10,
			RunID:          20,
			SpaceID:        30,
			Kind:           entity.TranscriptKindTerminal,
			Digest:         strings.Repeat("c", 64),
			IdempotencyKey: "terminal:" + strings.Repeat("c", 64),
			MessageCount:   2,
			Messages:       `[{"role":"user","content":"请不要再记住旧地区了"},{"role":"assistant","content":"已更新"}]`,
			Metadata:       `{"runtime":"eino_adk"}`,
		},
		recalledMemories: []*entity.Memory{
			{
				ID:         401,
				ThreadID:   10,
				RunID:      20,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "部署地区是 APAC",
				Metadata:   `{"category":"context"}`,
				Confidence: 0.9,
			},
		},
		completedMemoryFlushJob: &entity.MemoryFlushJob{
			ID:     902,
			Status: entity.MemoryFlushJobStatusSucceeded,
		},
		memoryFlushUpdated: true,
		deleteMemoryOK:     true,
	}
	extractor := &recordingMemoryUpdateExtractor{
		result: &MemoryExtractionResult{
			FactsToRemove: []int64{401},
			Facts: []MemoryExtractionFact{
				{
					Key:        "region",
					Scope:      MemoryScopeLongTerm,
					Content:    "部署地区是 EU",
					Metadata:   `{"category":"correction","sourceError":"部署地区是 APAC"}`,
					Confidence: 0.98,
				},
			},
		},
	}
	app := &ApplicationService{
		ThreadSVC:       domainSVC,
		MemoryExtractor: extractor,
	}

	resp, err := app.ProcessMemoryFlushJobs(context.Background(), &ProcessMemoryFlushJobsRequest{
		WorkerID:           "memory-worker-a",
		Limit:              1,
		MaxAttempts:        3,
		RetryBackoffMillis: 60000,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int32(1), resp.Succeeded)
	require.NotNil(t, domainSVC.deleteMemoryReq)
	require.Equal(t, int64(10), domainSVC.deleteMemoryReq.ThreadID)
	require.Equal(t, int64(401), domainSVC.deleteMemoryReq.MemoryID)
	require.Equal(t, int64(40), domainSVC.deleteMemoryReq.ActorID)
	require.Len(t, domainSVC.rememberMemoryReqs, 1)
	require.Equal(t, "部署地区是 EU", domainSVC.rememberMemoryReqs[0].Content)
	require.NotNil(t, domainSVC.completeMemoryFlushReq)
	require.Equal(t, int64(902), domainSVC.completeMemoryFlushReq.JobID)
	require.NotContains(t, domainSVC.appendRunEventReq.Payload, "部署地区是 APAC")
	require.NotContains(t, domainSVC.appendRunEventReq.Payload, "部署地区是 EU")
}

func TestApplicationTokenUsageMethodsMapDomainUsage(t *testing.T) {
	domainSVC := &recordingThreadService{
		recordedTokenUsage: &entity.TokenUsage{
			ID:           400,
			ThreadID:     10,
			RunID:        20,
			SpaceID:      1,
			Source:       entity.TokenUsageSourceLeadAgent,
			StepID:       "model-1",
			StepIndex:    0,
			StepName:     "generate_answer",
			ModelName:    "gpt-test",
			Provider:     "openai-compatible",
			InputTokens:  12,
			OutputTokens: 8,
			TotalTokens:  20,
			CostMicros:   250,
			Currency:     "USD",
			RawUsage:     `{"prompt_tokens":12}`,
			Metadata:     `{"phase":"app"}`,
			CreatedAt:    500,
		},
		tokenUsageRows: []*entity.TokenUsage{
			{
				ID:          401,
				ThreadID:    10,
				RunID:       20,
				Source:      entity.TokenUsageSourceTool,
				TotalTokens: 10,
				CreatedAt:   501,
			},
		},
		tokenUsageTotal: 1,
		tokenUsageAggregate: &entity.TokenUsageAggregate{
			InputTokens:     12,
			OutputTokens:    8,
			TotalTokens:     20,
			CostMicros:      250,
			CallCount:       1,
			LeadAgentTokens: 20,
		},
		runTokenUsageAggregates: []*entity.RunTokenUsageAggregate{
			{
				RunID: 20,
				Aggregate: &entity.TokenUsageAggregate{
					TotalTokens:     20,
					CallCount:       1,
					LeadAgentTokens: 20,
				},
			},
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	recordResp, err := app.RecordTokenUsage(context.Background(), &RecordTokenUsageRequest{
		RunID:        20,
		Source:       TokenUsageSourceLeadAgent,
		StepID:       "model-1",
		StepName:     "generate_answer",
		ModelName:    "gpt-test",
		Provider:     "openai-compatible",
		InputTokens:  12,
		OutputTokens: 8,
		RawUsage:     `{"prompt_tokens":12}`,
		Metadata:     `{"phase":"app"}`,
	})
	require.NoError(t, err)
	require.Equal(t, int64(20), domainSVC.recordTokenUsageReq.RunID)
	require.Equal(t, entity.TokenUsageSourceLeadAgent, domainSVC.recordTokenUsageReq.Source)
	require.Equal(t, int64(400), recordResp.Usage.UsageID)
	require.Equal(t, TokenUsageSourceLeadAgent, recordResp.Usage.Source)
	require.Equal(t, int64(20), recordResp.Usage.TotalTokens)

	runResp, err := app.GetRunTokenUsage(context.Background(), &GetTokenUsageRequest{
		RunID:            20,
		IncludeChildRuns: true,
		Page:             2,
		PageSize:         5,
	})
	require.NoError(t, err)
	require.Equal(t, int64(20), domainSVC.getRunTokenUsageReq.RunID)
	require.True(t, domainSVC.getRunTokenUsageReq.IncludeChildRuns)
	require.Equal(t, int32(2), domainSVC.getRunTokenUsageReq.Page)
	require.Equal(t, int64(1), runResp.Total)
	require.Len(t, runResp.Usage, 1)
	require.Equal(t, TokenUsageSourceTool, runResp.Usage[0].Source)
	require.Equal(t, int64(20), runResp.Aggregate.TotalTokens)
	require.Equal(t, int64(1), runResp.Aggregate.CallCount)
	require.Len(t, runResp.RunAggregates, 1)
	require.Equal(t, int64(20), runResp.RunAggregates[0].RunID)
	require.Equal(t, int64(20), runResp.RunAggregates[0].Aggregate.TotalTokens)

	threadResp, err := app.GetThreadTokenUsage(context.Background(), &GetTokenUsageRequest{
		ThreadID: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.getThreadTokenUsageReq.ThreadID)
	require.Equal(t, int64(20), threadResp.Aggregate.LeadAgentTokens)
}

func TestApplicationCreateRunMapsDomainRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		createdRun: &entity.Run{
			ID:                200,
			ThreadID:          10,
			ParentRunID:       100,
			SpaceID:           1,
			CreatorID:         2,
			AssistantID:       "default",
			RunKind:           entity.RunKindSubagent,
			Status:            entity.RunStatusPending,
			Command:           `{}`,
			Input:             `{"messages":[]}`,
			Config:            `{"mode":"Auto"}`,
			Context:           `{"source":"web"}`,
			Metadata:          `{"trace":"abc"}`,
			StreamMode:        `["messages","updates"]`,
			MultitaskStrategy: "enqueue",
			OnDisconnect:      "continue",
			Durability:        "async",
			IdempotencyKey:    "idem-1",
			CreatedAt:         300,
			UpdatedAt:         301,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	initialStatus := RunStatusQueued

	resp, err := app.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID:       10,
		ParentRunID:    100,
		AssistantID:    "default",
		RunKind:        RunKindSubagent,
		Input:          `{"messages":[]}`,
		Config:         `{"mode":"Auto"}`,
		Context:        `{"source":"web"}`,
		Metadata:       `{"trace":"abc"}`,
		IdempotencyKey: "idem-1",
		Status:         initialStatus,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.createRunReq.ThreadID)
	require.Equal(t, int64(100), domainSVC.createRunReq.ParentRunID)
	require.Equal(t, "default", domainSVC.createRunReq.AssistantID)
	require.Equal(t, entity.RunKindSubagent, domainSVC.createRunReq.RunKind)
	require.Equal(t, `{"messages":[]}`, domainSVC.createRunReq.Input)
	require.Equal(t, `{"mode":"Auto"}`, domainSVC.createRunReq.Config)
	require.Equal(t, `{"source":"web"}`, domainSVC.createRunReq.Context)
	require.Equal(t, `{"trace":"abc"}`, domainSVC.createRunReq.Metadata)
	require.Equal(t, "idem-1", domainSVC.createRunReq.IdempotencyKey)
	require.Equal(t, entity.RunStatusQueued, domainSVC.createRunReq.Status)
	require.Equal(t, int64(200), resp.Run.RunID)
	require.Equal(t, int64(10), resp.Run.ThreadID)
	require.Equal(t, int64(100), resp.Run.ParentRunID)
	require.Equal(t, RunKindSubagent, resp.Run.RunKind)
	require.Equal(t, RunStatusPending, resp.Run.Status)
	require.Equal(t, `{"messages":[]}`, resp.Run.Input)
	require.Equal(t, `["messages","updates"]`, resp.Run.StreamMode)
}

func TestApplicationCreateRunRejectsRuntimeDisabledByServerPolicy(t *testing.T) {
	domainSVC := &recordingThreadService{}
	policy := RuntimePolicy{
		DefaultMode:    RuntimeModeLegacy,
		EinoADKEnabled: false,
	}
	app := &ApplicationService{
		ThreadSVC:     domainSVC,
		RuntimePolicy: &policy,
	}

	_, err := app.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Config:   `{"runtime":"eino_adk"}`,
	})

	require.ErrorContains(t, err, "eino adk runtime is disabled by server policy")
	require.Nil(t, domainSVC.createRunReq)
}

func TestApplicationCreateRunRejectsUnknownRuntimeBeforePersistence(t *testing.T) {
	domainSVC := &recordingThreadService{}
	policy := RuntimePolicy{
		DefaultMode:    RuntimeModeLegacy,
		EinoADKEnabled: true,
	}
	app := &ApplicationService{
		ThreadSVC:     domainSVC,
		RuntimePolicy: &policy,
	}

	_, err := app.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Config:   `{"runtime":"other"}`,
	})

	require.ErrorContains(t, err, "unsupported agent runtime: other")
	require.Nil(t, domainSVC.createRunReq)
}

func TestApplicationResumeHumanInteractionCreatesQueuedRun(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1},
		Interrupts: map[string]ADKInterruptItem{
			"interrupt-1": {
				ID:          "interrupt-1",
				Address:     "lead/tool/ask_user_clarification",
				IsRootCause: true,
				Info: HumanInteractionPrompt{
					Schema:        humanInteractionSchema,
					InteractionID: "hi_1",
					Kind:          HumanInteractionKindClarification,
					Question:      "请选择时间范围",
					Required:      true,
					AllowFreeText: true,
				},
			},
		},
	}
	domainSVC := &recordingThreadService{
		gotRun: &entity.Run{
			ID:       20,
			ThreadID: 10,
			SpaceID:  1,
			Status:   entity.RunStatusInterrupted,
		},
		checkpoints: []*entity.Checkpoint{
			{
				ID:              503,
				ThreadID:        10,
				RunID:           20,
				CheckpointNS:    "eino.adk",
				RuntimeType:     "eino_adk",
				RuntimeKey:      "checkpoint-1",
				EnvelopeVersion: 1,
				ChannelValues:   mustADKCheckpointEnvelopeJSON(t, envelope),
				ChannelVersions: `{}`,
				PendingSends:    `[]`,
				Metadata:        `{"runtime":"eino_adk"}`,
			},
		},
		createdRun: &entity.Run{
			ID:       21,
			ThreadID: 10,
			SpaceID:  1,
			Status:   entity.RunStatusQueued,
		},
		appended: &entity.Message{ID: 30, ThreadID: 10, RunID: 21, Role: entity.MessageRoleUser},
		appendedRunEvent: &entity.RunEvent{
			ID:        40,
			ThreadID:  10,
			RunID:     21,
			EventType: "human.interaction.resolved",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.ResumeHumanInteraction(context.Background(), &ResumeHumanInteractionRequest{
		ThreadID:    10,
		SourceRunID: 20,
		InterruptID: "interrupt-1",
		Response: HumanInteractionResponse{
			Schema:        humanInteractionResponseSchema,
			InteractionID: "hi_1",
			Kind:          HumanInteractionKindClarification,
			Decision:      HumanInteractionDecisionAnswered,
			Answer:        "最近 7 天",
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(21), resp.Run.RunID)
	require.Equal(t, entity.RunStatusQueued, domainSVC.createRunReq.Status)
	require.Equal(t, int64(10), domainSVC.listCheckpointsReq.ThreadID)
	require.Equal(t, int64(20), domainSVC.listCheckpointsReq.RunID)
	require.JSONEq(t, `{"messages":[]}`, domainSVC.createRunReq.Input)
	require.Contains(t, domainSVC.createRunReq.Command, `"checkpoint_id":503`)
	require.Contains(t, domainSVC.createRunReq.Command, `"interrupt-1"`)
	require.Contains(t, domainSVC.createRunReq.Command, `"answer":"最近 7 天"`)
	require.Contains(t, domainSVC.createRunReq.Metadata, `"source_run_id":20`)
	require.Equal(t, int64(21), domainSVC.appendReq.RunID)
	require.Equal(t, entity.MessageRoleUser, domainSVC.appendReq.Role)
	require.Contains(t, domainSVC.appendReq.Content, "最近 7 天")
	require.Equal(t, int64(21), domainSVC.appendRunEventReq.RunID)
	require.Equal(t, "human.interaction.resolved", domainSVC.appendRunEventReq.EventType)
}

func TestApplicationResumeHumanInteractionRejectsNonInterruptedSourceRun(t *testing.T) {
	app := &ApplicationService{ThreadSVC: &recordingThreadService{
		gotRun: &entity.Run{
			ID:       20,
			ThreadID: 10,
			SpaceID:  1,
			Status:   entity.RunStatusRunning,
		},
	}}

	resp, err := app.ResumeHumanInteraction(context.Background(), &ResumeHumanInteractionRequest{
		ThreadID:    10,
		SourceRunID: 20,
		InterruptID: "interrupt-1",
		Response: HumanInteractionResponse{
			Schema:        humanInteractionResponseSchema,
			InteractionID: "hi_1",
			Kind:          HumanInteractionKindClarification,
			Decision:      HumanInteractionDecisionAnswered,
			Answer:        "最近 7 天",
		},
	})

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "source run must be interrupted")
}

func TestApplicationResumeHumanInteractionRejectsUnknownInterrupt(t *testing.T) {
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: 1,
		Runtime:         string(RuntimeModeEinoADK),
		RuntimeVersion:  "0.9.9",
		RuntimeKey:      "checkpoint-1",
		MessageType:     "schema.Message",
		Checkpoint:      []byte{1},
		Interrupts: map[string]ADKInterruptItem{
			"interrupt-1": {ID: "interrupt-1", Address: "lead/tool/approval"},
		},
	}
	app := &ApplicationService{ThreadSVC: &recordingThreadService{
		gotRun: &entity.Run{
			ID:       20,
			ThreadID: 10,
			SpaceID:  1,
			Status:   entity.RunStatusInterrupted,
		},
		checkpoints: []*entity.Checkpoint{
			{
				ID:              503,
				ThreadID:        10,
				RunID:           20,
				CheckpointNS:    "eino.adk",
				RuntimeType:     "eino_adk",
				RuntimeKey:      "checkpoint-1",
				EnvelopeVersion: 1,
				ChannelValues:   mustADKCheckpointEnvelopeJSON(t, envelope),
				Metadata:        `{"runtime":"eino_adk"}`,
			},
		},
	}}

	resp, err := app.ResumeHumanInteraction(context.Background(), &ResumeHumanInteractionRequest{
		ThreadID:    10,
		SourceRunID: 20,
		InterruptID: "missing",
		Response: HumanInteractionResponse{
			Schema:        humanInteractionResponseSchema,
			InteractionID: "hi_1",
			Kind:          HumanInteractionKindClarification,
			Decision:      HumanInteractionDecisionAnswered,
			Answer:        "最近 7 天",
		},
	})

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "interrupt id is not resumable")
}

func TestApplicationResumeHumanInteractionReturnsExistingIdempotentRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		gotRun: &entity.Run{
			ID:       20,
			ThreadID: 10,
			SpaceID:  1,
			Status:   entity.RunStatusInterrupted,
		},
		idempotentRun: &entity.Run{
			ID:             21,
			ThreadID:       10,
			SpaceID:        1,
			Status:         entity.RunStatusQueued,
			IdempotencyKey: "resume-key",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.ResumeHumanInteraction(context.Background(), &ResumeHumanInteractionRequest{
		ThreadID:       10,
		SourceRunID:    20,
		InterruptID:    "interrupt-1",
		IdempotencyKey: "resume-key",
		Response: HumanInteractionResponse{
			Schema:        humanInteractionResponseSchema,
			InteractionID: "hi_1",
			Kind:          HumanInteractionKindConfirmation,
			Decision:      HumanInteractionDecisionRejected,
			Comment:       "先不要执行",
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(21), resp.Run.RunID)
	require.Nil(t, domainSVC.createRunReq)
}

func TestApplicationRetrySubagentRunCreatesQueuedTopLevelRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		gotRunsByID: map[int64]*entity.Run{
			10: {
				ID:                10,
				ThreadID:          1,
				SpaceID:           2,
				AssistantID:       "lead-agent",
				RunKind:           entity.RunKindTask,
				Status:            entity.RunStatusFailed,
				Input:             `{"messages":[]}`,
				Config:            `{"runtime":"eino_adk"}`,
				Context:           `{"plan_scope_run_id":10}`,
				StreamMode:        `["messages","updates"]`,
				MultitaskStrategy: "enqueue",
				OnDisconnect:      "continue",
				Durability:        "async",
			},
			20: {
				ID:          20,
				ThreadID:    1,
				ParentRunID: 10,
				SpaceID:     2,
				AssistantID: "singleagent:1001",
				RunKind:     entity.RunKindSubagent,
				Status:      entity.RunStatusFailed,
				Metadata:    `{"subagent":{"name":"researcher"}}`,
				ErrorCode:   "subagent_timeout",
			},
		},
		createdRun: &entity.Run{
			ID:          21,
			ThreadID:    1,
			ParentRunID: 0,
			SpaceID:     2,
			AssistantID: "lead-agent",
			RunKind:     entity.RunKindTask,
			Status:      entity.RunStatusQueued,
		},
		appendedRunEvent: &entity.RunEvent{
			ID:        40,
			ThreadID:  1,
			RunID:     21,
			EventType: "subagent.retry.requested",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.RetrySubagentRun(context.Background(), &RetrySubagentRunRequest{
		ThreadID:       1,
		SourceRunID:    20,
		IdempotencyKey: "retry-child-20",
	})

	require.NoError(t, err)
	require.Equal(t, int64(21), resp.Run.RunID)
	require.Equal(t, entity.RunStatusQueued, domainSVC.createRunReq.Status)
	require.Equal(t, entity.RunKindTask, domainSVC.createRunReq.RunKind)
	require.Zero(t, domainSVC.createRunReq.ParentRunID)
	require.Equal(t, "lead-agent", domainSVC.createRunReq.AssistantID)
	require.Equal(t, `{"messages":[]}`, domainSVC.createRunReq.Input)
	require.Equal(t, `{"runtime":"eino_adk"}`, domainSVC.createRunReq.Config)
	require.Equal(t, `{"plan_scope_run_id":10}`, domainSVC.createRunReq.Context)
	require.Equal(t, "retry-child-20", domainSVC.createRunReq.IdempotencyKey)
	require.Contains(t, domainSVC.createRunReq.Command, `"subagent_retry"`)
	require.Contains(t, domainSVC.createRunReq.Command, `"source_run_id":20`)
	require.Contains(t, domainSVC.createRunReq.Command, `"parent_run_id":10`)
	require.Contains(t, domainSVC.createRunReq.Metadata, `"source":"subagent_retry"`)
	require.Contains(t, domainSVC.createRunReq.Metadata, `"source_run_id":20`)
	require.Equal(t, "subagent.retry.requested", domainSVC.appendRunEventReq.EventType)
	require.Equal(t, int64(21), domainSVC.appendRunEventReq.RunID)
	require.Contains(t, domainSVC.appendRunEventReq.Payload, `"source_run_id":20`)
}

func TestApplicationRetrySubagentRunRejectsRunningChildRun(t *testing.T) {
	app := &ApplicationService{ThreadSVC: &recordingThreadService{
		gotRunsByID: map[int64]*entity.Run{
			20: {
				ID:          20,
				ThreadID:    1,
				ParentRunID: 10,
				RunKind:     entity.RunKindSubagent,
				Status:      entity.RunStatusRunning,
			},
		},
	}}

	resp, err := app.RetrySubagentRun(context.Background(), &RetrySubagentRunRequest{
		ThreadID:    1,
		SourceRunID: 20,
	})

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "source subagent run must be failed or canceled")
}

func TestApplicationListRunsMapsDomainRuns(t *testing.T) {
	parentRunID := int64(100)
	domainSVC := &recordingThreadService{
		runs: []*entity.Run{
			{
				ID:          200,
				ThreadID:    10,
				ParentRunID: parentRunID,
				RunKind:     entity.RunKindSubagent,
				Status:      entity.RunStatusPending,
				Input:       `{}`,
			},
			{
				ID:       201,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{}`,
			},
		},
		runTotal: 2,
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	status := RunStatusPending

	resp, err := app.ListRuns(context.Background(), &ListRunsRequest{
		ThreadID:    10,
		ParentRunID: &parentRunID,
		Status:      &status,
		Page:        2,
		PageSize:    5,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.listRunsReq.ThreadID)
	require.NotNil(t, domainSVC.listRunsReq.ParentRunID)
	require.Equal(t, parentRunID, *domainSVC.listRunsReq.ParentRunID)
	require.NotNil(t, domainSVC.listRunsReq.Status)
	require.Equal(t, entity.RunStatusPending, *domainSVC.listRunsReq.Status)
	require.Equal(t, int32(2), domainSVC.listRunsReq.Page)
	require.Equal(t, int32(5), domainSVC.listRunsReq.PageSize)
	require.Equal(t, int64(2), resp.Total)
	require.Len(t, resp.Runs, 2)
	require.Equal(t, int64(200), resp.Runs[0].RunID)
	require.Equal(t, parentRunID, resp.Runs[0].ParentRunID)
	require.Equal(t, RunKindSubagent, resp.Runs[0].RunKind)
	require.Equal(t, RunStatusPending, resp.Runs[0].Status)
	require.Equal(t, int64(201), resp.Runs[1].RunID)
	require.Equal(t, RunStatusRunning, resp.Runs[1].Status)
}

func TestApplicationListArtifactsMapsDomainArtifacts(t *testing.T) {
	runID := int64(20)
	artifactSVC := &recordingArtifactService{
		artifacts: []*entity.AgentArtifact{
			{
				ID:           100,
				SpaceID:      30,
				UserID:       40,
				ThreadID:     10,
				RunID:        20,
				FileID:       90,
				Title:        "Report",
				ArtifactType: "report",
				VirtualPath:  "/mnt/user-data/outputs/report.txt",
				ContentType:  "text/plain; charset=utf-8",
				SizeBytes:    128,
				PreviewMode:  entity.AgentArtifactPreviewModeText,
				Metadata:     `{"source":"present_files"}`,
				CreatedAt:    1000,
				UpdatedAt:    1100,
			},
		},
		total: 1,
	}
	app := &ApplicationService{ArtifactSVC: artifactSVC}

	resp, err := app.ListArtifacts(context.Background(), &ListArtifactsRequest{
		ThreadID:    10,
		RunID:       &runID,
		DeletedOnly: true,
		Page:        2,
		PageSize:    5,
	})

	require.NoError(t, err)
	require.NotNil(t, artifactSVC.listReq)
	require.Equal(t, int64(10), artifactSVC.listReq.ThreadID)
	require.NotNil(t, artifactSVC.listReq.RunID)
	require.Equal(t, runID, *artifactSVC.listReq.RunID)
	require.True(t, artifactSVC.listReq.DeletedOnly)
	require.Equal(t, int32(2), artifactSVC.listReq.Page)
	require.Equal(t, int32(5), artifactSVC.listReq.PageSize)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Artifacts, 1)
	require.Equal(t, int64(100), resp.Artifacts[0].ArtifactID)
	require.Equal(t, int64(90), resp.Artifacts[0].FileID)
	require.Equal(t, ArtifactPreviewModeText, resp.Artifacts[0].PreviewMode)
}

func TestApplicationListArtifactsDeniesUnauthorizedViewerBeforeRepository(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	authorizer := &recordingArtifactAuthorizer{
		err: ErrArtifactAccessDenied,
	}
	app := &ApplicationService{
		ArtifactSVC:        artifactSVC,
		ArtifactAuthorizer: authorizer,
	}

	resp, err := app.ListArtifacts(context.Background(), &ListArtifactsRequest{
		ThreadID: 10,
		ViewerID: 99,
		Page:     1,
		PageSize: 10,
	})

	require.ErrorIs(t, err, ErrArtifactAccessDenied)
	require.Nil(t, resp)
	require.Nil(t, artifactSVC.listReq)
	require.Equal(t, ArtifactAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(99), authorizer.req.ViewerID)
}

func TestApplicationListArtifactsRejectsClientSuppliedSameSpaceViewer(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	threadSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:        10,
			SpaceID:   30,
			CreatorID: 40,
		},
	}
	app := &ApplicationService{
		ArtifactSVC: artifactSVC,
		ThreadSVC:   threadSVC,
	}
	app.ArtifactAuthorizer = NewThreadOwnerArtifactAuthorizer(app.ThreadSVC)

	resp, err := app.ListArtifacts(context.Background(), &ListArtifactsRequest{
		ThreadID: 10,
		SpaceID:  30,
		ViewerID: 99,
		Page:     1,
		PageSize: 10,
	})

	require.ErrorIs(t, err, ErrArtifactAccessDenied)
	require.Nil(t, resp)
	require.Nil(t, artifactSVC.listReq)
	require.Equal(t, int64(10), threadSVC.getID)
}

func TestApplicationListArtifactsDeniesMismatchedSpaceViewerBeforeRepository(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	threadSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:        10,
			SpaceID:   30,
			CreatorID: 40,
		},
	}
	app := &ApplicationService{
		ArtifactSVC: artifactSVC,
		ThreadSVC:   threadSVC,
	}
	app.ArtifactAuthorizer = NewThreadOwnerArtifactAuthorizer(app.ThreadSVC)

	resp, err := app.ListArtifacts(context.Background(), &ListArtifactsRequest{
		ThreadID: 10,
		SpaceID:  31,
		ViewerID: 99,
		Page:     1,
		PageSize: 10,
	})

	require.ErrorIs(t, err, ErrArtifactAccessDenied)
	require.Nil(t, resp)
	require.Nil(t, artifactSVC.listReq)
	require.Equal(t, int64(10), threadSVC.getID)
}

func TestApplicationListArtifactScanJobsMapsDomainJobs(t *testing.T) {
	runID := int64(20)
	artifactID := int64(100)
	artifactSVC := &recordingArtifactService{
		listScanJobs: []*entity.ArtifactScanJob{
			{
				ID:             3001,
				ThreadID:       10,
				RunID:          runID,
				SpaceID:        30,
				UserID:         40,
				ArtifactID:     artifactID,
				FileID:         90,
				Scanner:        "clamav",
				Status:         entity.ArtifactScanJobStatusFailed,
				WorkerID:       "scan-worker-a",
				AttemptCount:   3,
				LastError:      "scanner unavailable",
				AvailableAt:    1000,
				LeaseExpiresAt: 0,
				StartedAt:      1100,
				EndedAt:        1200,
				CreatedAt:      900,
				UpdatedAt:      1200,
			},
		},
		listScanJobsTotal: 1,
	}
	app := &ApplicationService{ArtifactSVC: artifactSVC}

	resp, err := app.ListArtifactScanJobs(
		context.Background(),
		&ListArtifactScanJobsRequest{
			ThreadID:   10,
			RunID:      &runID,
			ArtifactID: &artifactID,
			Status:     "failed",
			Scanner:    "clamav",
			Page:       2,
			PageSize:   5,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, artifactSVC.listScanJobsReq)
	require.Equal(t, int64(10), artifactSVC.listScanJobsReq.ThreadID)
	require.Equal(t, runID, *artifactSVC.listScanJobsReq.RunID)
	require.Equal(t, artifactID, *artifactSVC.listScanJobsReq.ArtifactID)
	require.Equal(t, "failed", artifactSVC.listScanJobsReq.Status)
	require.Equal(t, "clamav", artifactSVC.listScanJobsReq.Scanner)
	require.Equal(t, int32(2), artifactSVC.listScanJobsReq.Page)
	require.Equal(t, int32(5), artifactSVC.listScanJobsReq.PageSize)
	require.Equal(t, int64(1), resp.Total)
	require.Len(t, resp.Jobs, 1)
	require.Equal(t, int64(3001), resp.Jobs[0].JobID)
	require.Equal(t, ArtifactScanJobStatusFailed, resp.Jobs[0].Status)
	require.Equal(t, "scanner unavailable", resp.Jobs[0].LastError)
	require.Equal(t, int32(3), resp.Jobs[0].AttemptCount)
}

func TestApplicationListArtifactScanJobsDeniesUnauthorizedViewerBeforeRepository(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	authorizer := &recordingArtifactAuthorizer{
		err: ErrArtifactAccessDenied,
	}
	app := &ApplicationService{
		ArtifactSVC:        artifactSVC,
		ArtifactAuthorizer: authorizer,
	}

	resp, err := app.ListArtifactScanJobs(
		context.Background(),
		&ListArtifactScanJobsRequest{
			ThreadID: 10,
			ViewerID: 99,
			Page:     1,
			PageSize: 10,
		},
	)

	require.ErrorIs(t, err, ErrArtifactAccessDenied)
	require.Nil(t, resp)
	require.Nil(t, artifactSVC.listScanJobsReq)
	require.Equal(t, ArtifactAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(99), authorizer.req.ViewerID)
}

func TestApplicationRetryArtifactScanJobAuthorizesRequeuesAndAudits(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		requeueFailedScanJob: &entity.ArtifactScanJob{
			ID:           3001,
			ThreadID:     10,
			RunID:        20,
			SpaceID:      30,
			UserID:       40,
			ArtifactID:   100,
			FileID:       90,
			Scanner:      "default",
			Status:       entity.ArtifactScanJobStatusPending,
			AttemptCount: 3,
			LastError:    "manual retry requested",
			AvailableAt:  1234,
			UpdatedAt:    1200,
		},
		requeueFailedScanJobOK: true,
	}
	authorizer := &recordingArtifactAuthorizer{}
	app := &ApplicationService{
		ThreadSVC:          threadSVC,
		ArtifactSVC:        artifactSVC,
		ArtifactAuthorizer: authorizer,
	}

	resp, err := app.RetryArtifactScanJob(
		context.Background(),
		&RetryArtifactScanJobRequest{
			ThreadID: 10,
			JobID:    3001,
			ViewerID: 99,
		},
	)

	require.NoError(t, err)
	require.True(t, resp.Retried)
	require.Equal(t, int64(3001), resp.Job.JobID)
	require.Equal(t, ArtifactScanJobStatusPending, resp.Job.Status)
	require.Equal(t, ArtifactAccessOperationList, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(99), authorizer.req.ViewerID)
	require.Equal(t, int64(3001), artifactSVC.requeueFailedScanJobReq.JobID)
	require.Equal(t, int64(10), artifactSVC.requeueFailedScanJobReq.ThreadID)
	require.Equal(
		t,
		"manual retry requested",
		artifactSVC.requeueFailedScanJobReq.ErrorText,
	)
	require.Greater(t, artifactSVC.requeueFailedScanJobReq.AvailableAt, int64(0))
	require.Equal(
		t,
		artifactSVC.requeueFailedScanJobReq.AvailableAt,
		artifactSVC.requeueFailedScanJobReq.Now,
	)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, int64(20), threadSVC.appendRunEventReq.RunID)
	require.Equal(
		t,
		artifactScanJobRetryRequestedEvent,
		threadSVC.appendRunEventReq.EventType,
	)
	payload := map[string]any{}
	require.NoError(
		t,
		json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload),
	)
	require.Equal(
		t,
		"coze.artifact_scan_job_retry_requested.v1",
		payload["schema"],
	)
	require.Equal(t, float64(3001), payload["job_id"])
	require.Equal(t, float64(10), payload["thread_id"])
	require.Equal(t, float64(20), payload["run_id"])
	require.Equal(t, float64(100), payload["artifact_id"])
	require.Equal(t, "pending", payload["status"])
	require.Equal(t, float64(3), payload["attempt_count"])
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "scanner unavailable")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "/mnt/user-data")
}

func TestApplicationRetryArtifactScanJobReturnsConflictWhenNotRetryable(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	app := &ApplicationService{ArtifactSVC: artifactSVC}

	resp, err := app.RetryArtifactScanJob(
		context.Background(),
		&RetryArtifactScanJobRequest{
			ThreadID: 10,
			JobID:    3001,
		},
	)

	require.ErrorIs(t, err, ErrArtifactScanJobRetryNotAllowed)
	require.Nil(t, resp)
}

func TestApplicationDeleteArtifactEmitsContentFreeAuditEvent(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		deleted: &entity.AgentArtifact{
			ID:           100,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			Title:        "report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    14,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			DeletedAt:    1300,
		},
		deletedOK: true,
	}
	app := &ApplicationService{
		ThreadSVC:   threadSVC,
		ArtifactSVC: artifactSVC,
	}

	resp, err := app.DeleteArtifact(context.Background(), &DeleteArtifactRequest{
		ThreadID:   10,
		ArtifactID: 100,
		DeletedAt:  1300,
	})

	require.NoError(t, err)
	require.True(t, resp.Deleted)
	require.Equal(t, int64(10), artifactSVC.deleteReq.ThreadID)
	require.Equal(t, int64(100), artifactSVC.deleteReq.ArtifactID)
	require.Equal(t, int64(1300), artifactSVC.deleteReq.DeletedAt)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, int64(10), threadSVC.appendRunEventReq.ThreadID)
	require.Equal(t, int64(20), threadSVC.appendRunEventReq.RunID)
	require.Equal(t, "artifact.deleted", threadSVC.appendRunEventReq.EventType)
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "/mnt/user-data")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "report.txt")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload))
	require.Equal(t, "coze.artifact_deleted.v1", payload["schema"])
	require.Equal(t, float64(10), payload["thread_id"])
	require.Equal(t, float64(20), payload["run_id"])
	require.Equal(t, float64(100), payload["artifact_id"])
	require.Equal(t, float64(90), payload["file_id"])
	require.Equal(t, "report", payload["artifact_type"])
	require.Equal(t, "text/plain; charset=utf-8", payload["content_type"])
	require.Equal(t, float64(14), payload["size_bytes"])
	require.Equal(t, float64(1300), payload["deleted_at"])
}

func TestApplicationDeleteArtifactDeniesUnauthorizedViewerBeforeDelete(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	authorizer := &recordingArtifactAuthorizer{
		err: ErrArtifactAccessDenied,
	}
	app := &ApplicationService{
		ArtifactSVC:        artifactSVC,
		ArtifactAuthorizer: authorizer,
	}

	resp, err := app.DeleteArtifact(context.Background(), &DeleteArtifactRequest{
		ThreadID:   10,
		ArtifactID: 100,
		ViewerID:   99,
	})

	require.ErrorIs(t, err, ErrArtifactAccessDenied)
	require.Nil(t, resp)
	require.Nil(t, artifactSVC.deleteReq)
	require.Equal(t, ArtifactAccessOperationDelete, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(100), authorizer.req.ArtifactID)
	require.Equal(t, int64(99), authorizer.req.ViewerID)
}

func TestApplicationRestoreArtifactEmitsContentFreeAuditEvent(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		restored: &entity.AgentArtifact{
			ID:           100,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			Title:        "report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    14,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			UpdatedAt:    1500,
		},
		restoredOK: true,
	}
	app := &ApplicationService{
		ThreadSVC:   threadSVC,
		ArtifactSVC: artifactSVC,
	}

	resp, err := app.RestoreArtifact(context.Background(), &RestoreArtifactRequest{
		ThreadID:   10,
		ArtifactID: 100,
		RestoredAt: 1500,
	})

	require.NoError(t, err)
	require.True(t, resp.Restored)
	require.NotNil(t, resp.Artifact)
	require.Equal(t, int64(10), artifactSVC.restoreReq.ThreadID)
	require.Equal(t, int64(100), artifactSVC.restoreReq.ArtifactID)
	require.Equal(t, int64(1500), artifactSVC.restoreReq.RestoredAt)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, int64(10), threadSVC.appendRunEventReq.ThreadID)
	require.Equal(t, int64(20), threadSVC.appendRunEventReq.RunID)
	require.Equal(t, "artifact.restored", threadSVC.appendRunEventReq.EventType)
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "/mnt/user-data")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "report.txt")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload))
	require.Equal(t, "coze.artifact_restored.v1", payload["schema"])
	require.Equal(t, float64(10), payload["thread_id"])
	require.Equal(t, float64(20), payload["run_id"])
	require.Equal(t, float64(100), payload["artifact_id"])
	require.Equal(t, float64(90), payload["file_id"])
	require.Equal(t, "report", payload["artifact_type"])
	require.Equal(t, "text/plain; charset=utf-8", payload["content_type"])
	require.Equal(t, float64(14), payload["size_bytes"])
	require.Equal(t, float64(1500), payload["restored_at"])
}

func TestApplicationRestoreArtifactDeniesUnauthorizedViewerBeforeRestore(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	authorizer := &recordingArtifactAuthorizer{
		err: ErrArtifactAccessDenied,
	}
	app := &ApplicationService{
		ArtifactSVC:        artifactSVC,
		ArtifactAuthorizer: authorizer,
	}

	resp, err := app.RestoreArtifact(context.Background(), &RestoreArtifactRequest{
		ThreadID:   10,
		ArtifactID: 100,
		ViewerID:   99,
	})

	require.ErrorIs(t, err, ErrArtifactAccessDenied)
	require.Nil(t, resp)
	require.Nil(t, artifactSVC.restoreReq)
	require.Equal(t, ArtifactAccessOperationRestore, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(100), authorizer.req.ArtifactID)
	require.Equal(t, int64(99), authorizer.req.ViewerID)
}

func TestApplicationProcessDeletedArtifactCleanupDeletesObjectBeforeMarkingFile(t *testing.T) {
	artifact := &entity.AgentArtifact{
		ID:           100,
		ThreadID:     10,
		RunID:        20,
		FileID:       90,
		Title:        "secret-report.txt",
		ArtifactType: "report",
		VirtualPath:  "/mnt/user-data/outputs/secret-report.txt",
		ObjectURI:    "agent-runtime/30/10/runs/20/tool-results/trunc/file.txt",
		ContentType:  "text/plain",
		SizeBytes:    11,
		DeletedAt:    1200,
	}
	artifactSVC := &recordingArtifactService{
		cleanupCandidates: []*entity.AgentArtifact{artifact},
		markFileDeletedOK: true,
	}
	storage := &recordingArtifactObjectReader{}
	threadSVC := &recordingThreadService{}
	app := &ApplicationService{
		ThreadSVC:              threadSVC,
		ArtifactSVC:            artifactSVC,
		ArtifactObjectStorage:  storage,
		ArtifactCleanupNowFunc: func() int64 { return 2000 },
	}

	resp, err := app.ProcessDeletedArtifactCleanup(
		context.Background(),
		&ProcessDeletedArtifactCleanupRequest{
			RetentionMillis: 500,
			Limit:           10,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int32(1), resp.Candidates)
	require.Equal(t, int32(1), resp.Deleted)
	require.Equal(t, int32(0), resp.Failed)
	require.Equal(t, int64(1500), artifactSVC.cleanupReq.CutoffDeletedAt)
	require.Equal(t, int32(10), artifactSVC.cleanupReq.Limit)
	require.Equal(
		t,
		[]string{"agent-runtime/30/10/runs/20/tool-results/trunc/file.txt"},
		storage.deletedKeys,
	)
	require.Equal(t, int64(90), artifactSVC.markFileDeletedReq.FileID)
	require.Equal(
		t,
		"agent-runtime/30/10/runs/20/tool-results/trunc/file.txt",
		artifactSVC.markFileDeletedReq.ObjectURI,
	)
	require.Equal(t, int64(2000), artifactSVC.markFileDeletedReq.DeletedAt)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, artifactCleanedEvent, threadSVC.appendRunEventReq.EventType)
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime/")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "/mnt/user-data")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "secret-report.txt")
}

func TestApplicationProcessDeletedArtifactCleanupKeepsFileActiveOnDeleteFailure(t *testing.T) {
	artifact := &entity.AgentArtifact{
		ID:           101,
		ThreadID:     10,
		RunID:        20,
		FileID:       91,
		ArtifactType: "report",
		ObjectURI:    "agent-runtime/30/10/runs/20/tool-results/trunc/fail.txt",
		ContentType:  "text/plain",
		SizeBytes:    11,
		DeletedAt:    1200,
	}
	artifactSVC := &recordingArtifactService{
		cleanupCandidates: []*entity.AgentArtifact{artifact},
		markFileDeletedOK: true,
	}
	storage := &recordingArtifactObjectReader{deleteErr: fmt.Errorf("storage unavailable")}
	app := &ApplicationService{
		ArtifactSVC:            artifactSVC,
		ArtifactObjectStorage:  storage,
		ArtifactCleanupNowFunc: func() int64 { return 2000 },
	}

	resp, err := app.ProcessDeletedArtifactCleanup(
		context.Background(),
		&ProcessDeletedArtifactCleanupRequest{
			RetentionMillis: 500,
			Limit:           10,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int32(1), resp.Candidates)
	require.Equal(t, int32(0), resp.Deleted)
	require.Equal(t, int32(1), resp.Failed)
	require.Nil(t, artifactSVC.markFileDeletedReq)
}

func TestApplicationReadArtifactContentUsesServerSideObjectURI(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:           100,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			Title:        "report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    14,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{"scan_status":"clean"}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/report.txt": []byte("artifact body"),
		},
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 100,
		Mode:       ArtifactContentModePreview,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(10), artifactSVC.getReq.ThreadID)
	require.Equal(t, int64(100), artifactSVC.getReq.ArtifactID)
	require.Equal(t, "agent-runtime/30/10/runs/20/outputs/report.txt", storage.key)
	require.Equal(t, []byte("artifact body"), resp.Content)
	require.Equal(t, "text/plain; charset=utf-8", resp.ContentType)
	require.Equal(t, "report.txt", resp.FileName)
	require.False(t, resp.Attachment)
	require.Equal(t, int64(100), resp.Artifact.ArtifactID)
}

func TestApplicationCreateArtifactSignedURLUsesServerSideObjectURI(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:           100,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			Title:        "report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    14,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{"scan_status":"clean"}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		signedURL: "https://storage.example.test/signed/report.txt?token=abc",
	}
	authorizer := &recordingArtifactAuthorizer{}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
		ArtifactAuthorizer:    authorizer,
	}

	resp, err := app.CreateArtifactSignedURL(context.Background(), &CreateArtifactSignedURLRequest{
		ThreadID:   10,
		ArtifactID: 100,
		Mode:       ArtifactContentModePreview,
		ViewerID:   99,
		TTLSeconds: 99999,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int64(10), artifactSVC.getReq.ThreadID)
	require.Equal(t, int64(100), artifactSVC.getReq.ArtifactID)
	require.Equal(t, "agent-runtime/30/10/runs/20/outputs/report.txt", storage.signKey)
	require.Equal(t, int64(3600), storage.signExpire)
	require.Equal(t, "https://storage.example.test/signed/report.txt?token=abc", resp.URL)
	require.Equal(t, int64(3600), resp.ExpiresInSeconds)
	require.Equal(t, "text/plain; charset=utf-8", resp.ContentType)
	require.Equal(t, ArtifactPreviewModeText, resp.PreviewMode)
	require.Equal(t, int64(100), resp.Artifact.ArtifactID)
	require.Equal(t, ArtifactAccessOperationRead, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(100), authorizer.req.ArtifactID)
	require.Equal(t, int64(99), authorizer.req.ViewerID)
}

func TestApplicationCreateArtifactSignedURLRejectsDownloadOnlyContent(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:          101,
			ThreadID:    10,
			Title:       "page.html",
			ObjectURI:   "agent-runtime/30/10/runs/20/outputs/page.html",
			ContentType: "text/html; charset=utf-8",
			SizeBytes:   20,
			PreviewMode: entity.AgentArtifactPreviewModeDownload,
			Metadata:    `{"scan_status":"clean"}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		signedURL: "https://storage.example.test/signed/page.html?token=abc",
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.CreateArtifactSignedURL(context.Background(), &CreateArtifactSignedURLRequest{
		ThreadID:   10,
		ArtifactID: 101,
		Mode:       ArtifactContentModePreview,
		TTLSeconds: 300,
	})

	require.ErrorIs(t, err, ErrArtifactSignedURLNotSupported)
	require.Nil(t, resp)
	require.Empty(t, storage.signKey)
}

func TestApplicationCreateArtifactSignedURLCreatesAttachmentURLForDownloadContent(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:          102,
			ThreadID:    10,
			Title:       "page.html",
			ObjectURI:   "agent-runtime/30/10/runs/20/outputs/page.html",
			ContentType: "text/html; charset=utf-8",
			SizeBytes:   20,
			PreviewMode: entity.AgentArtifactPreviewModeDownload,
			Metadata:    `{"scan_status":"clean"}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/page.html": []byte("<!doctype html><html></html>"),
		},
		signedURL: "https://storage.example.test/signed/page.html?token=abc",
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.CreateArtifactSignedURL(context.Background(), &CreateArtifactSignedURLRequest{
		ThreadID:   10,
		ArtifactID: 102,
		Mode:       ArtifactContentModeDownload,
		TTLSeconds: 5,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "https://storage.example.test/signed/page.html?token=abc", resp.URL)
	require.Equal(t, int64(60), resp.ExpiresInSeconds)
	require.Equal(t, ArtifactPreviewModeDownload, resp.PreviewMode)
	require.Equal(t, "text/html; charset=utf-8", resp.ContentType)
	require.Equal(t, "agent-runtime/30/10/runs/20/outputs/page.html", storage.signKey)
	require.Equal(t, int64(60), storage.signExpire)
	require.Equal(
		t,
		"attachment; filename*=UTF-8''page.html",
		storage.signContentDisposition,
	)
	require.Equal(t, "text/html; charset=utf-8", storage.signContentType)
}

func TestApplicationReadArtifactContentDeniesUnauthorizedViewerBeforeStorage(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/report.txt": []byte("secret"),
		},
	}
	authorizer := &recordingArtifactAuthorizer{
		err: ErrArtifactAccessDenied,
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
		ArtifactAuthorizer:    authorizer,
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 100,
		Mode:       ArtifactContentModeDownload,
		ViewerID:   99,
	})

	require.ErrorIs(t, err, ErrArtifactAccessDenied)
	require.Nil(t, resp)
	require.Nil(t, artifactSVC.getReq)
	require.Empty(t, storage.key)
	require.Equal(t, ArtifactAccessOperationRead, authorizer.req.Operation)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(100), authorizer.req.ArtifactID)
	require.Equal(t, int64(99), authorizer.req.ViewerID)
}

func TestApplicationReadArtifactContentForcesAttachmentForActiveContent(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:          101,
			ThreadID:    10,
			Title:       "page.html",
			ObjectURI:   "agent-runtime/30/10/runs/20/outputs/page.html",
			ContentType: "text/html; charset=utf-8",
			SizeBytes:   20,
			PreviewMode: entity.AgentArtifactPreviewModeDownload,
			Metadata:    `{"scan_status":"clean"}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/page.html": []byte("<html>unsafe</html>"),
		},
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 101,
		Mode:       ArtifactContentModePreview,
	})

	require.NoError(t, err)
	require.True(t, resp.Attachment)
	require.Equal(t, []byte("<html>unsafe</html>"), resp.Content)
}

func TestApplicationReadArtifactContentSniffsHTMLAndForcesAttachment(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:          102,
			ThreadID:    10,
			Title:       "report.txt",
			ObjectURI:   "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType: "text/plain; charset=utf-8",
			SizeBytes:   43,
			PreviewMode: entity.AgentArtifactPreviewModeText,
			Metadata:    `{"scan_status":"clean"}`,
		},
	}
	content := []byte("<!doctype html><html><body>unsafe</body></html>")
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/report.txt": content,
		},
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 102,
		Mode:       ArtifactContentModePreview,
	})

	require.NoError(t, err)
	require.Equal(t, content, resp.Content)
	require.Contains(t, resp.ContentType, "text/html")
	require.True(t, resp.Attachment)
}

func TestApplicationReadArtifactContentKeepsAttachmentWhenMetadataIsUnsafe(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:          103,
			ThreadID:    10,
			Title:       "icon.svg",
			ObjectURI:   "agent-runtime/30/10/runs/20/outputs/icon.svg",
			ContentType: "image/svg+xml",
			SizeBytes:   11,
			PreviewMode: entity.AgentArtifactPreviewModeDownload,
			Metadata:    `{"scan_status":"clean"}`,
		},
	}
	content := []byte("plain note")
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/icon.svg": content,
		},
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 103,
		Mode:       ArtifactContentModePreview,
	})

	require.NoError(t, err)
	require.Equal(t, content, resp.Content)
	require.Contains(t, resp.ContentType, "text/plain")
	require.True(t, resp.Attachment)
}

func TestApplicationReadArtifactContentEmitsContentFreeAuditEvent(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:           104,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			Title:        "report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    14,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{"scan_status":"clean"}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/report.txt": []byte("artifact body"),
		},
	}
	app := &ApplicationService{
		ThreadSVC:             threadSVC,
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 104,
		Mode:       ArtifactContentModePreview,
	})

	require.NoError(t, err)
	require.False(t, resp.Attachment)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, int64(10), threadSVC.appendRunEventReq.ThreadID)
	require.Equal(t, int64(20), threadSVC.appendRunEventReq.RunID)
	require.Equal(t, "artifact.content.accessed", threadSVC.appendRunEventReq.EventType)
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "/mnt/user-data")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "report.txt")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "artifact body")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload))
	require.Equal(t, "coze.artifact_access.v1", payload["schema"])
	require.Equal(t, float64(10), payload["thread_id"])
	require.Equal(t, float64(20), payload["run_id"])
	require.Equal(t, float64(104), payload["artifact_id"])
	require.Equal(t, float64(90), payload["file_id"])
	require.Equal(t, "preview", payload["mode"])
	require.Equal(t, "text", payload["preview_mode"])
	require.Equal(t, "report", payload["artifact_type"])
	require.Equal(t, "text/plain; charset=utf-8", payload["content_type"])
	require.Equal(t, float64(14), payload["size_bytes"])
	require.Equal(t, false, payload["attachment"])
	require.Equal(t, "clean", payload["scan_status"])
}

func TestApplicationReadArtifactContentBlocksUnsafeScanStatusBeforeStorage(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:           105,
			ThreadID:     10,
			RunID:        20,
			FileID:       91,
			Title:        "blocked.txt",
			ArtifactType: "report",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/blocked.txt",
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    11,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{"scan_status":"blocked"}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/blocked.txt": []byte("blocked body"),
		},
	}
	app := &ApplicationService{
		ThreadSVC:             &recordingThreadService{},
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 105,
		Mode:       ArtifactContentModeDownload,
	})

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "artifact scan status blocks content read")
	require.Empty(t, storage.key)
}

func TestArtifactScanReadPolicyAllowsOnlyClean(t *testing.T) {
	tests := []struct {
		status artifactScanStatus
		allow  bool
		reason string
	}{
		{status: artifactScanStatusClean, allow: true},
		{status: artifactScanStatusUnknown, reason: "scan_unknown"},
		{status: artifactScanStatusPending, reason: "scan_pending"},
		{status: artifactScanStatusFailed, reason: "scan_failed"},
		{status: artifactScanStatusBlocked, reason: "scan_blocked"},
		{status: artifactScanStatusInfected, reason: "scan_infected"},
		{status: artifactScanStatusQuarantined, reason: "scan_quarantined"},
	}

	for _, testCase := range tests {
		t.Run(string(testCase.status), func(t *testing.T) {
			policy := artifactScanReadPolicy(
				testCase.status,
				&entity.AgentArtifact{PreviewMode: entity.AgentArtifactPreviewModeText, ContentType: "text/plain"},
				ArtifactScanReadPolicyConfig{},
			)

			require.Equal(t, testCase.allow, policy.Allowed)
			require.Equal(t, testCase.reason, policy.Reason)
		})
	}
}

func TestArtifactScanReadPolicyOpenNonExecutableAllowsOnlyOutageForTextAndImage(t *testing.T) {
	config := ArtifactScanReadPolicyConfig{OutageFailMode: ArtifactScanOutageFailModeOpenNonExecutable}
	tests := []struct {
		name     string
		status   artifactScanStatus
		artifact *entity.AgentArtifact
		allow    bool
		reason   string
		override bool
	}{
		{
			name:   "failed text allowed by explicit outage override",
			status: artifactScanStatusFailed,
			artifact: &entity.AgentArtifact{
				PreviewMode: entity.AgentArtifactPreviewModeText,
				ContentType: "text/plain; charset=utf-8",
			},
			allow:    true,
			reason:   "scan_failed",
			override: true,
		},
		{
			name:   "unknown image allowed by explicit outage override",
			status: artifactScanStatusUnknown,
			artifact: &entity.AgentArtifact{
				PreviewMode: entity.AgentArtifactPreviewModeImage,
				ContentType: "image/png",
			},
			allow:    true,
			reason:   "scan_unknown",
			override: true,
		},
		{
			name:   "infected text still blocked",
			status: artifactScanStatusInfected,
			artifact: &entity.AgentArtifact{
				PreviewMode: entity.AgentArtifactPreviewModeText,
				ContentType: "text/plain",
			},
			reason: "scan_infected",
		},
		{
			name:   "failed pdf still blocked",
			status: artifactScanStatusFailed,
			artifact: &entity.AgentArtifact{
				PreviewMode: entity.AgentArtifactPreviewModePDF,
				ContentType: "application/pdf",
			},
			reason: "scan_failed",
		},
		{
			name:   "pending download still blocked",
			status: artifactScanStatusPending,
			artifact: &entity.AgentArtifact{
				PreviewMode: entity.AgentArtifactPreviewModeDownload,
				ContentType: "text/html",
			},
			reason: "scan_pending",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			policy := artifactScanReadPolicy(testCase.status, testCase.artifact, config)

			require.Equal(t, testCase.allow, policy.Allowed)
			require.Equal(t, testCase.reason, policy.Reason)
			require.Equal(t, testCase.override, policy.Override)
			if testCase.override {
				require.Equal(t, string(ArtifactScanOutageFailModeOpenNonExecutable), policy.FailMode)
			}
		})
	}
}

func TestArtifactScanReadPolicyConfigFromEnv(t *testing.T) {
	t.Setenv(agentArtifactScanOutageFailModeEnv, "open_non_executable")

	config := NewArtifactScanReadPolicyConfigFromEnv()

	require.Equal(t, ArtifactScanOutageFailModeOpenNonExecutable, config.OutageFailMode)
}

func TestArtifactScanReadPolicyConfigFromEnvDefaultsClosedForUnknownValue(t *testing.T) {
	t.Setenv(agentArtifactScanOutageFailModeEnv, "open_all")

	config := NewArtifactScanReadPolicyConfigFromEnv()

	require.Equal(t, ArtifactScanOutageFailModeClosed, config.OutageFailMode)
}

func TestApplicationReadArtifactContentAllowsUnscannedTextWithExplicitOutageFailOpen(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:           108,
			ThreadID:     10,
			RunID:        20,
			FileID:       93,
			Title:        "report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    11,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{"scan_status":"failed"}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/report.txt": []byte("report body"),
		},
	}
	app := &ApplicationService{
		ThreadSVC:             threadSVC,
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
		ArtifactScanReadPolicy: ArtifactScanReadPolicyConfig{
			OutageFailMode: ArtifactScanOutageFailModeOpenNonExecutable,
		},
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 108,
		Mode:       ArtifactContentModePreview,
	})

	require.NoError(t, err)
	require.Equal(t, []byte("report body"), resp.Content)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, "artifact.content.accessed", threadSVC.appendRunEventReq.EventType)
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "report.txt")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "report body")
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload))
	require.Equal(t, "failed", payload["scan_status"])
	require.Equal(t, "scan_failed", payload["scan_policy_reason"])
	require.Equal(t, true, payload["scan_policy_override"])
	require.Equal(t, "open_non_executable", payload["scan_policy_mode"])
}

func TestApplicationReadArtifactContentBlocksUnknownScanStatusAndAudits(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		got: &entity.AgentArtifact{
			ID:           106,
			ThreadID:     10,
			RunID:        20,
			FileID:       92,
			ArtifactType: "note",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/note.txt",
			ContentType:  "text/plain",
			SizeBytes:    9,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{}`,
		},
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/note.txt": []byte("note body"),
		},
	}
	app := &ApplicationService{
		ThreadSVC:             threadSVC,
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
	}

	resp, err := app.ReadArtifactContent(context.Background(), &ReadArtifactContentRequest{
		ThreadID:   10,
		ArtifactID: 106,
		Mode:       ArtifactContentModePreview,
	})

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "artifact scan status blocks content read")
	require.Empty(t, storage.key)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, "artifact.content.blocked", threadSVC.appendRunEventReq.EventType)
	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload))
	require.Equal(t, "coze.artifact_access_blocked.v1", payload["schema"])
	require.Equal(t, "unknown", payload["scan_status"])
	require.Equal(t, "scan_unknown", payload["reason"])
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "note.txt")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "note body")
}

func TestApplicationRecordArtifactScanResultEmitsContentFreeAuditEvent(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		scanUpdated: &entity.AgentArtifact{
			ID:           107,
			ThreadID:     10,
			RunID:        20,
			FileID:       93,
			Title:        "unsafe-report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/unsafe-report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/unsafe-report.txt",
			ContentType:  "text/plain",
			SizeBytes:    128,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{"scan_status":"infected","scan_scanner":"clamav","scan_scanner_version":"1.2.3","scan_reason":"signature","scan_scanned_at":3000}`,
		},
		scanUpdatedOK: true,
	}
	app := &ApplicationService{
		ThreadSVC:   threadSVC,
		ArtifactSVC: artifactSVC,
	}

	resp, err := app.RecordArtifactScanResult(
		context.Background(),
		&RecordArtifactScanResultRequest{
			ThreadID:       10,
			ArtifactID:     107,
			ScanStatus:     "infected",
			Scanner:        "clamav",
			ScannerVersion: "1.2.3",
			Reason:         "signature",
			ScannedAt:      3000,
		},
	)

	require.NoError(t, err)
	require.True(t, resp.Updated)
	require.NotNil(t, artifactSVC.scanReq)
	require.Equal(t, int64(10), artifactSVC.scanReq.ThreadID)
	require.Equal(t, int64(107), artifactSVC.scanReq.ArtifactID)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, int64(10), threadSVC.appendRunEventReq.ThreadID)
	require.Equal(t, int64(20), threadSVC.appendRunEventReq.RunID)
	require.Equal(t, "artifact.scan.completed", threadSVC.appendRunEventReq.EventType)
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "/mnt/user-data")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "unsafe-report.txt")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "signature")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload))
	require.Equal(t, "coze.artifact_scan.v1", payload["schema"])
	require.Equal(t, float64(10), payload["thread_id"])
	require.Equal(t, float64(20), payload["run_id"])
	require.Equal(t, float64(107), payload["artifact_id"])
	require.Equal(t, float64(93), payload["file_id"])
	require.Equal(t, "report", payload["artifact_type"])
	require.Equal(t, "text/plain", payload["content_type"])
	require.Equal(t, float64(128), payload["size_bytes"])
	require.Equal(t, "infected", payload["scan_status"])
	require.Equal(t, "clamav", payload["scanner"])
	require.Equal(t, "1.2.3", payload["scanner_version"])
	require.Equal(t, float64(3000), payload["scanned_at"])
}

func TestApplicationReviewArtifactScanReleaseAuthorizesUpdatesAndAudits(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		scanUpdated: &entity.AgentArtifact{
			ID:           108,
			ThreadID:     10,
			RunID:        20,
			FileID:       94,
			Title:        "manual-report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/manual-report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/manual-report.txt",
			ContentType:  "text/plain",
			SizeBytes:    256,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{"scan_status":"clean","scan_scanner":"manual_review","scan_reason":"manual release requested","scan_scanned_at":4000}`,
		},
		scanUpdatedOK: true,
	}
	authorizer := &recordingArtifactAuthorizer{}
	app := &ApplicationService{
		ThreadSVC:           threadSVC,
		ArtifactSVC:         artifactSVC,
		ArtifactAuthorizer:  authorizer,
		ArtifactReviewClock: func() int64 { return 4000 },
	}

	resp, err := app.ReviewArtifactScan(
		context.Background(),
		&ReviewArtifactScanRequest{
			ThreadID:   10,
			ArtifactID: 108,
			ViewerID:   99,
			Decision:   "release",
			Reason:     "manual release requested",
		},
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.True(t, resp.Reviewed)
	require.Equal(t, int64(108), resp.ArtifactID)
	require.Equal(t, "release", resp.Decision)
	require.Equal(t, "clean", resp.ScanStatus)
	require.NotNil(t, authorizer.req)
	require.Equal(t, int64(10), authorizer.req.ThreadID)
	require.Equal(t, int64(108), authorizer.req.ArtifactID)
	require.Equal(t, int64(99), authorizer.req.ViewerID)
	require.Equal(t, ArtifactAccessOperationReview, authorizer.req.Operation)
	require.NotNil(t, artifactSVC.scanReq)
	require.Equal(t, int64(10), artifactSVC.scanReq.ThreadID)
	require.Equal(t, int64(108), artifactSVC.scanReq.ArtifactID)
	require.Equal(t, "clean", artifactSVC.scanReq.ScanStatus)
	require.Equal(t, "manual_review", artifactSVC.scanReq.Scanner)
	require.Equal(t, "manual release requested", artifactSVC.scanReq.Reason)
	require.Equal(t, int64(4000), artifactSVC.scanReq.ScannedAt)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, int64(10), threadSVC.appendRunEventReq.ThreadID)
	require.Equal(t, int64(20), threadSVC.appendRunEventReq.RunID)
	require.Equal(t, "artifact.scan.reviewed", threadSVC.appendRunEventReq.EventType)
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "/mnt/user-data")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "manual-report.txt")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "manual release requested")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload))
	require.Equal(t, "coze.artifact_scan_review.v1", payload["schema"])
	require.Equal(t, float64(10), payload["thread_id"])
	require.Equal(t, float64(20), payload["run_id"])
	require.Equal(t, float64(108), payload["artifact_id"])
	require.Equal(t, "release", payload["decision"])
	require.Equal(t, "clean", payload["scan_status"])
	require.Equal(t, "manual_review", payload["scanner"])
	require.Equal(t, float64(4000), payload["reviewed_at"])
}

func TestApplicationReviewArtifactScanRejectsInvalidDecision(t *testing.T) {
	artifactSVC := &recordingArtifactService{}
	app := &ApplicationService{ArtifactSVC: artifactSVC}

	resp, err := app.ReviewArtifactScan(
		context.Background(),
		&ReviewArtifactScanRequest{
			ThreadID:   10,
			ArtifactID: 108,
			Decision:   "delete",
		},
	)

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "artifact scan review decision is invalid")
	require.Nil(t, artifactSVC.scanReq)
}

func TestApplicationProcessArtifactScanJobsCompletesCleanScan(t *testing.T) {
	threadSVC := &recordingThreadService{}
	artifactSVC := &recordingArtifactService{
		claimedScanJobs: []*entity.ArtifactScanJob{
			{
				ID:         700,
				ThreadID:   10,
				RunID:      20,
				SpaceID:    30,
				UserID:     40,
				ArtifactID: 100,
				FileID:     90,
				Scanner:    "clamav",
				Status:     entity.ArtifactScanJobStatusProcessing,
				WorkerID:   "scan-worker-a",
			},
		},
		got: &entity.AgentArtifact{
			ID:           100,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       90,
			Title:        "report.txt",
			ArtifactType: "report",
			VirtualPath:  "/mnt/user-data/outputs/report.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/report.txt",
			ContentType:  "text/plain; charset=utf-8",
			SizeBytes:    13,
			PreviewMode:  entity.AgentArtifactPreviewModeText,
			Metadata:     `{"scan_status":"pending"}`,
		},
		completeScanJob:   &entity.ArtifactScanJob{ID: 700, Status: entity.ArtifactScanJobStatusSucceeded},
		completeScanJobOK: true,
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/report.txt": []byte("artifact body"),
		},
	}
	scanner := &recordingArtifactContentScanner{
		result: &ArtifactScanResult{
			ScanStatus:     "clean",
			ScannerVersion: "1.4.0",
			Reason:         "no threats found",
		},
	}
	app := &ApplicationService{
		ThreadSVC:             threadSVC,
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
		ArtifactScanner:       scanner,
	}

	resp, err := app.ProcessArtifactScanJobs(
		context.Background(),
		&ProcessArtifactScanJobsRequest{
			Scanner:        "clamav",
			WorkerID:       "scan-worker-a",
			Limit:          3,
			LeaseTTLMillis: 60000,
		},
	)

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, int32(1), resp.Claimed)
	require.Equal(t, int32(1), resp.Succeeded)
	require.Equal(t, int32(0), resp.Failed)
	require.Equal(t, "clamav", artifactSVC.claimScanJobsReq.Scanner)
	require.Equal(t, "scan-worker-a", artifactSVC.claimScanJobsReq.WorkerID)
	require.Equal(t, int32(3), artifactSVC.claimScanJobsReq.Limit)
	require.Equal(t, int64(60000), artifactSVC.claimScanJobsReq.LeaseTTLMillis)
	require.Equal(t, int64(10), artifactSVC.getReq.ThreadID)
	require.Equal(t, int64(100), artifactSVC.getReq.ArtifactID)
	require.Equal(t, "agent-runtime/30/10/runs/20/outputs/report.txt", storage.key)
	require.Equal(t, int64(10), scanner.req.ThreadID)
	require.Equal(t, int64(20), scanner.req.RunID)
	require.Equal(t, int64(100), scanner.req.ArtifactID)
	require.Equal(t, int64(90), scanner.req.FileID)
	require.Equal(t, "clamav", scanner.req.Scanner)
	require.Equal(t, "text/plain; charset=utf-8", scanner.req.ContentType)
	require.Equal(t, int64(13), scanner.req.SizeBytes)
	require.Equal(t, []byte("artifact body"), scanner.req.Content)
	require.Equal(t, int64(700), artifactSVC.completeScanJobReq.JobID)
	require.Equal(t, "scan-worker-a", artifactSVC.completeScanJobReq.WorkerID)
	require.Equal(t, "clean", artifactSVC.completeScanJobReq.ScanStatus)
	require.Equal(t, "1.4.0", artifactSVC.completeScanJobReq.ScannerVersion)
	require.Equal(t, "no threats found", artifactSVC.completeScanJobReq.Reason)
	require.NotZero(t, artifactSVC.completeScanJobReq.ScannedAt)
	require.Nil(t, artifactSVC.failScanJobReq)
	require.NotNil(t, threadSVC.appendRunEventReq)
	require.Equal(t, int64(10), threadSVC.appendRunEventReq.ThreadID)
	require.Equal(t, int64(20), threadSVC.appendRunEventReq.RunID)
	require.Equal(t, "artifact.scan.completed", threadSVC.appendRunEventReq.EventType)
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "agent-runtime")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "/mnt/user-data")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "report.txt")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "artifact body")
	require.NotContains(t, threadSVC.appendRunEventReq.Payload, "no threats found")

	var payload map[string]any
	require.NoError(t, json.Unmarshal([]byte(threadSVC.appendRunEventReq.Payload), &payload))
	require.Equal(t, "coze.artifact_scan.v1", payload["schema"])
	require.Equal(t, float64(10), payload["thread_id"])
	require.Equal(t, float64(20), payload["run_id"])
	require.Equal(t, float64(100), payload["artifact_id"])
	require.Equal(t, float64(90), payload["file_id"])
	require.Equal(t, "report", payload["artifact_type"])
	require.Equal(t, "text/plain; charset=utf-8", payload["content_type"])
	require.Equal(t, float64(13), payload["size_bytes"])
	require.Equal(t, "clean", payload["scan_status"])
	require.Equal(t, "clamav", payload["scanner"])
	require.Equal(t, "1.4.0", payload["scanner_version"])
	require.NotZero(t, payload["scanned_at"])
}

func TestApplicationProcessArtifactScanJobsFailsJobOnScannerErrorWithoutContentLeak(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		claimedScanJobs: []*entity.ArtifactScanJob{
			{
				ID:         701,
				ThreadID:   10,
				RunID:      20,
				SpaceID:    30,
				UserID:     40,
				ArtifactID: 101,
				FileID:     91,
				Scanner:    "clamav",
				Status:     entity.ArtifactScanJobStatusProcessing,
				WorkerID:   "scan-worker-a",
			},
		},
		got: &entity.AgentArtifact{
			ID:           101,
			SpaceID:      30,
			ThreadID:     10,
			RunID:        20,
			FileID:       91,
			Title:        "secret.txt",
			ArtifactType: "note",
			VirtualPath:  "/mnt/user-data/outputs/secret.txt",
			ObjectURI:    "agent-runtime/30/10/runs/20/outputs/secret.txt",
			ContentType:  "text/plain",
			SizeBytes:    11,
			Metadata:     `{"scan_status":"pending"}`,
		},
		failScanJob:   &entity.ArtifactScanJob{ID: 701, Status: entity.ArtifactScanJobStatusFailed},
		failScanJobOK: true,
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/secret.txt": []byte("secret body"),
		},
	}
	scanner := &recordingArtifactContentScanner{
		err: fmt.Errorf("scanner crashed on agent-runtime/30/10/runs/20/outputs/secret.txt with secret body"),
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
		ArtifactScanner:       scanner,
	}

	resp, err := app.ProcessArtifactScanJobs(
		context.Background(),
		&ProcessArtifactScanJobsRequest{
			Scanner:  "clamav",
			WorkerID: "scan-worker-a",
			Limit:    1,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int32(1), resp.Claimed)
	require.Equal(t, int32(0), resp.Succeeded)
	require.Equal(t, int32(1), resp.Failed)
	require.Nil(t, artifactSVC.completeScanJobReq)
	require.NotNil(t, artifactSVC.failScanJobReq)
	require.Equal(t, int64(701), artifactSVC.failScanJobReq.JobID)
	require.Equal(t, "scan-worker-a", artifactSVC.failScanJobReq.WorkerID)
	require.Equal(t, "artifact scan failed", artifactSVC.failScanJobReq.ErrorText)
	require.NotContains(t, artifactSVC.failScanJobReq.ErrorText, "agent-runtime")
	require.NotContains(t, artifactSVC.failScanJobReq.ErrorText, "/mnt/user-data")
	require.NotContains(t, artifactSVC.failScanJobReq.ErrorText, "secret.txt")
	require.NotContains(t, artifactSVC.failScanJobReq.ErrorText, "secret body")
}

func TestApplicationProcessArtifactScanJobsRetriesScannerErrorBeforeMaxAttempts(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		claimedScanJobs: []*entity.ArtifactScanJob{
			{
				ID:           702,
				ThreadID:     10,
				RunID:        20,
				SpaceID:      30,
				UserID:       40,
				ArtifactID:   102,
				FileID:       92,
				Scanner:      "clamav",
				Status:       entity.ArtifactScanJobStatusProcessing,
				WorkerID:     "scan-worker-a",
				AttemptCount: 1,
			},
		},
		got: &entity.AgentArtifact{
			ID:          102,
			SpaceID:     30,
			ThreadID:    10,
			RunID:       20,
			FileID:      92,
			ObjectURI:   "agent-runtime/30/10/runs/20/outputs/retry.txt",
			ContentType: "text/plain",
			SizeBytes:   10,
			Metadata:    `{"scan_status":"pending"}`,
		},
		retryScanJob:   &entity.ArtifactScanJob{ID: 702, Status: entity.ArtifactScanJobStatusPending},
		retryScanJobOK: true,
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/retry.txt": []byte("retry body"),
		},
	}
	scanner := &recordingArtifactContentScanner{
		err: fmt.Errorf("scanner unavailable with retry body"),
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
		ArtifactScanner:       scanner,
	}

	resp, err := app.ProcessArtifactScanJobs(
		context.Background(),
		&ProcessArtifactScanJobsRequest{
			Scanner:            "clamav",
			WorkerID:           "scan-worker-a",
			Limit:              1,
			MaxAttempts:        3,
			RetryBackoffMillis: 60000,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int32(1), resp.Claimed)
	require.Equal(t, int32(1), resp.Retried)
	require.Equal(t, int32(0), resp.Failed)
	require.Nil(t, artifactSVC.failScanJobReq)
	require.NotNil(t, artifactSVC.retryScanJobReq)
	require.Equal(t, int64(702), artifactSVC.retryScanJobReq.JobID)
	require.Equal(t, "scan-worker-a", artifactSVC.retryScanJobReq.WorkerID)
	require.Equal(t, "artifact scan failed", artifactSVC.retryScanJobReq.ErrorText)
	require.NotZero(t, artifactSVC.retryScanJobReq.AvailableAt)
	require.GreaterOrEqual(t, artifactSVC.retryScanJobReq.AvailableAt-artifactSVC.retryScanJobReq.Now, int64(60000))
	require.NotContains(t, artifactSVC.retryScanJobReq.ErrorText, "agent-runtime")
	require.NotContains(t, artifactSVC.retryScanJobReq.ErrorText, "retry body")
}

func TestApplicationProcessArtifactScanJobsFinalFailsAfterMaxAttempts(t *testing.T) {
	artifactSVC := &recordingArtifactService{
		claimedScanJobs: []*entity.ArtifactScanJob{
			{
				ID:           703,
				ThreadID:     10,
				RunID:        20,
				SpaceID:      30,
				UserID:       40,
				ArtifactID:   103,
				FileID:       93,
				Scanner:      "clamav",
				Status:       entity.ArtifactScanJobStatusProcessing,
				WorkerID:     "scan-worker-a",
				AttemptCount: 3,
			},
		},
		got: &entity.AgentArtifact{
			ID:          103,
			SpaceID:     30,
			ThreadID:    10,
			RunID:       20,
			FileID:      93,
			ObjectURI:   "agent-runtime/30/10/runs/20/outputs/final.txt",
			ContentType: "text/plain",
			SizeBytes:   10,
			Metadata:    `{"scan_status":"pending"}`,
		},
		failScanJob:   &entity.ArtifactScanJob{ID: 703, Status: entity.ArtifactScanJobStatusFailed},
		failScanJobOK: true,
	}
	storage := &recordingArtifactObjectReader{
		objects: map[string][]byte{
			"agent-runtime/30/10/runs/20/outputs/final.txt": []byte("final body"),
		},
	}
	scanner := &recordingArtifactContentScanner{
		err: fmt.Errorf("scanner unavailable with final body"),
	}
	app := &ApplicationService{
		ArtifactSVC:           artifactSVC,
		ArtifactObjectStorage: storage,
		ArtifactScanner:       scanner,
	}

	resp, err := app.ProcessArtifactScanJobs(
		context.Background(),
		&ProcessArtifactScanJobsRequest{
			Scanner:            "clamav",
			WorkerID:           "scan-worker-a",
			Limit:              1,
			MaxAttempts:        3,
			RetryBackoffMillis: 60000,
		},
	)

	require.NoError(t, err)
	require.Equal(t, int32(1), resp.Claimed)
	require.Equal(t, int32(0), resp.Retried)
	require.Equal(t, int32(1), resp.Failed)
	require.Nil(t, artifactSVC.retryScanJobReq)
	require.NotNil(t, artifactSVC.failScanJobReq)
	require.Equal(t, int64(703), artifactSVC.failScanJobReq.JobID)
	require.Equal(t, "artifact scan failed", artifactSVC.failScanJobReq.ErrorText)
}

func TestApplicationProcessArtifactScanJobsRequiresScanner(t *testing.T) {
	app := &ApplicationService{
		ArtifactSVC:           &recordingArtifactService{},
		ArtifactObjectStorage: &recordingArtifactObjectReader{},
	}

	resp, err := app.ProcessArtifactScanJobs(
		context.Background(),
		&ProcessArtifactScanJobsRequest{WorkerID: "scan-worker-a"},
	)

	require.Error(t, err)
	require.Nil(t, resp)
	require.Contains(t, err.Error(), "artifact content scanner is not configured")
}

func TestApplicationClaimPendingRunsMapsDomainRuns(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:        200,
				ThreadID:  10,
				Status:    entity.RunStatusRunning,
				Input:     `{"messages":[]}`,
				WorkerID:  "worker-a",
				StartedAt: 300,
				UpdatedAt: 301,
			},
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.ClaimPendingRuns(context.Background(), &ClaimPendingRunsRequest{
		WorkerID: "worker-a",
		Limit:    2,
	})

	require.NoError(t, err)
	require.Equal(t, "worker-a", domainSVC.claimRunsReq.WorkerID)
	require.Equal(t, int32(2), domainSVC.claimRunsReq.Limit)
	require.Len(t, resp.Runs, 1)
	require.Equal(t, int64(200), resp.Runs[0].RunID)
	require.Equal(t, RunStatusRunning, resp.Runs[0].Status)
	require.Equal(t, "worker-a", resp.Runs[0].WorkerID)
}

func TestApplicationClaimQueuedResumeRunsMapsDomainRuns(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedQueuedResumeRuns: []*entity.Run{
			{
				ID:       201,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				WorkerID: "resume-worker-a",
				Metadata: `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
			},
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.ClaimQueuedResumeRuns(context.Background(), &ClaimQueuedResumeRunsRequest{
		WorkerID: "resume-worker-a",
		Limit:    2,
	})

	require.NoError(t, err)
	require.Equal(t, "resume-worker-a", domainSVC.claimQueuedResumeRunsReq.WorkerID)
	require.Equal(t, int32(2), domainSVC.claimQueuedResumeRunsReq.Limit)
	require.Len(t, resp.Runs, 1)
	require.Equal(t, int64(201), resp.Runs[0].RunID)
	require.Equal(t, RunStatusRunning, resp.Runs[0].Status)
	require.Equal(t, "resume-worker-a", resp.Runs[0].WorkerID)
}

func TestApplicationCompleteRunMapsDomainRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
			EndedAt:  400,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.CompleteRun(context.Background(), &UpdateRunStatusRequest{
		RunID:    200,
		From:     RunStatusRunning,
		WorkerID: "worker-a",
	})

	require.NoError(t, err)
	require.Equal(t, int64(200), domainSVC.completeRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.completeRunReq.From)
	require.Equal(t, "worker-a", domainSVC.completeRunReq.WorkerID)
	require.Equal(t, int64(200), resp.Run.RunID)
	require.Equal(t, RunStatusSucceeded, resp.Run.Status)
	require.Equal(t, int64(400), resp.Run.EndedAt)
}

func TestApplicationFailRunMapsErrorFields(t *testing.T) {
	domainSVC := &recordingThreadService{
		failedRun: &entity.Run{
			ID:           200,
			ThreadID:     10,
			Status:       entity.RunStatusFailed,
			WorkerID:     "worker-a",
			ErrorCode:    "model_error",
			ErrorMessage: "model failed",
			EndedAt:      400,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.FailRun(context.Background(), &UpdateRunStatusRequest{
		RunID:        200,
		From:         RunStatusRunning,
		WorkerID:     "worker-a",
		ErrorCode:    "model_error",
		ErrorMessage: "model failed",
	})

	require.NoError(t, err)
	require.Equal(t, int64(200), domainSVC.failRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.failRunReq.From)
	require.Equal(t, "model_error", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "model failed", domainSVC.failRunReq.ErrorMessage)
	require.Equal(t, RunStatusFailed, resp.Run.Status)
	require.Equal(t, "model_error", resp.Run.ErrorCode)
	require.Equal(t, "model failed", resp.Run.ErrorMessage)
}

func TestApplicationCancelRunSignalsActiveADKExecution(t *testing.T) {
	domainSVC := &recordingThreadService{
		canceledRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusCanceled,
			Config:   `{"runtime":"eino_adk"}`,
		},
	}
	registry := NewADKCancelRegistry()
	var cancelRequest adkCancelRequest
	cleanup := registry.register(200, func(request adkCancelRequest) (adkCancelWaiter, bool) {
		cancelRequest = request
		return &recordingADKCancelWaiter{}, true
	})
	defer cleanup()
	app := &ApplicationService{
		ThreadSVC:         domainSVC,
		ADKCancelRegistry: registry,
	}

	resp, err := app.CancelRun(context.Background(), &UpdateRunStatusRequest{
		RunID:    200,
		From:     RunStatusRunning,
		WorkerID: "worker-a",
	})

	require.NoError(t, err)
	require.Equal(t, RunStatusCanceled, resp.Run.Status)
	require.Equal(t, adk.CancelAfterToolCalls|adk.CancelAfterChatModel, cancelRequest.mode)
	require.True(t, cancelRequest.recursive)
}

func TestApplicationAppendRunEventMapsDomainEvent(t *testing.T) {
	domainSVC := &recordingThreadService{
		appendedRunEvent: &entity.RunEvent{
			ID:        300,
			ThreadID:  10,
			RunID:     200,
			EventType: "run.started",
			Payload:   `{"status":"running"}`,
			CreatedAt: 400,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.AppendRunEvent(context.Background(), &AppendRunEventRequest{
		ThreadID:  10,
		RunID:     200,
		EventType: "run.started",
		Payload:   `{"status":"running"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.appendRunEventReq.ThreadID)
	require.Equal(t, int64(200), domainSVC.appendRunEventReq.RunID)
	require.Equal(t, "run.started", domainSVC.appendRunEventReq.EventType)
	require.Equal(t, `{"status":"running"}`, domainSVC.appendRunEventReq.Payload)
	require.Equal(t, int64(300), resp.Event.EventID)
	require.Equal(t, int64(10), resp.Event.ThreadID)
	require.Equal(t, int64(200), resp.Event.RunID)
	require.Equal(t, "run.started", resp.Event.EventType)
	require.Equal(t, `{"status":"running"}`, resp.Event.Payload)
	require.Equal(t, int64(400), resp.Event.CreatedAt)
}

func TestApplicationListRunEventsMapsDomainEvents(t *testing.T) {
	domainSVC := &recordingThreadService{
		runEvents: []*entity.RunEvent{
			{ID: 300, ThreadID: 10, RunID: 200, EventType: "run.started", Payload: `{}`, CreatedAt: 400},
			{ID: 301, ThreadID: 10, RunID: 200, EventType: "run.completed", Payload: `{"ok":true}`, CreatedAt: 500},
		},
		runEventTotal: 2,
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.ListRunEvents(context.Background(), &ListRunEventsRequest{
		ThreadID: 10,
		RunID:    200,
		Page:     2,
		PageSize: 5,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.listRunEventsReq.ThreadID)
	require.Equal(t, int64(200), domainSVC.listRunEventsReq.RunID)
	require.Equal(t, int32(2), domainSVC.listRunEventsReq.Page)
	require.Equal(t, int32(5), domainSVC.listRunEventsReq.PageSize)
	require.Equal(t, int64(2), resp.Total)
	require.Len(t, resp.Events, 2)
	require.Equal(t, int64(300), resp.Events[0].EventID)
	require.Equal(t, "run.started", resp.Events[0].EventType)
	require.Equal(t, int64(301), resp.Events[1].EventID)
	require.Equal(t, `{"ok":true}`, resp.Events[1].Payload)
}

func TestApplicationCheckpointMethodsMapDomainCheckpoints(t *testing.T) {
	domainSVC := &recordingThreadService{
		createdCheckpoint: &entity.Checkpoint{
			ID:                 500,
			ThreadID:           10,
			RunID:              200,
			ParentCheckpointID: 499,
			CheckpointNS:       "planner",
			RuntimeType:        "eino_adk",
			RuntimeKey:         "checkpoint-1",
			EnvelopeVersion:    1,
			ChannelValues:      `{"messages":["ok"]}`,
			ChannelVersions:    `{"messages":1}`,
			PendingSends:       `[]`,
			Metadata:           `{"source":"runtime"}`,
			CreatedAt:          600,
		},
		checkpoints: []*entity.Checkpoint{
			{ID: 501, ThreadID: 10, RunID: 200, ChannelValues: `{}`, ChannelVersions: `{}`, PendingSends: `[]`, Metadata: `{}`, CreatedAt: 601},
		},
		latestCheckpoint: &entity.Checkpoint{
			ID:              502,
			ThreadID:        10,
			RunID:           201,
			ChannelValues:   `{"messages":["latest"]}`,
			ChannelVersions: `{}`,
			PendingSends:    `[]`,
			Metadata:        `{}`,
			CreatedAt:       602,
		},
		checkpoint: &entity.Checkpoint{
			ID:              503,
			ThreadID:        10,
			RunID:           202,
			CheckpointNS:    "harness.terminal",
			ChannelValues:   `{"messages":["by-id"]}`,
			ChannelVersions: `{}`,
			PendingSends:    `[{"node":"resume"}]`,
			Metadata:        `{"source":"agent_harness"}`,
			CreatedAt:       603,
		},
		checkpointTotal: 1,
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	createResp, err := app.CreateCheckpoint(context.Background(), &CreateCheckpointRequest{
		ThreadID:           10,
		RunID:              200,
		ParentCheckpointID: 499,
		CheckpointNS:       "planner",
		RuntimeType:        "eino_adk",
		RuntimeKey:         "checkpoint-1",
		EnvelopeVersion:    1,
		ChannelValues:      `{"messages":["ok"]}`,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.createCheckpointReq.ThreadID)
	require.Equal(t, int64(200), domainSVC.createCheckpointReq.RunID)
	require.Equal(t, "eino_adk", domainSVC.createCheckpointReq.RuntimeType)
	require.Equal(t, "checkpoint-1", domainSVC.createCheckpointReq.RuntimeKey)
	require.Equal(t, int64(500), createResp.Checkpoint.CheckpointID)
	require.Equal(t, int64(499), createResp.Checkpoint.ParentCheckpointID)
	require.Equal(t, "planner", createResp.Checkpoint.CheckpointNS)
	require.Equal(t, "eino_adk", createResp.Checkpoint.RuntimeType)
	require.Equal(t, "checkpoint-1", createResp.Checkpoint.RuntimeKey)
	require.Equal(t, int32(1), createResp.Checkpoint.EnvelopeVersion)
	require.Equal(t, `{"messages":["ok"]}`, createResp.Checkpoint.ChannelValues)

	listResp, err := app.ListCheckpoints(context.Background(), &ListCheckpointsRequest{
		ThreadID: 10,
		RunID:    200,
		Limit:    5,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.listCheckpointsReq.ThreadID)
	require.Equal(t, int64(200), domainSVC.listCheckpointsReq.RunID)
	require.Equal(t, int32(5), domainSVC.listCheckpointsReq.Limit)
	require.Equal(t, int64(1), listResp.Total)
	require.Len(t, listResp.Checkpoints, 1)
	require.Equal(t, int64(501), listResp.Checkpoints[0].CheckpointID)

	latestResp, err := app.GetLatestCheckpoint(context.Background(), &GetLatestCheckpointRequest{
		ThreadID: 10,
	})
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.getLatestCheckpointReq.ThreadID)
	require.Equal(t, int64(502), latestResp.Checkpoint.CheckpointID)
	require.Equal(t, `{"messages":["latest"]}`, latestResp.Checkpoint.ChannelValues)

	getResp, err := app.GetCheckpoint(context.Background(), &GetCheckpointRequest{
		CheckpointID: 503,
	})
	require.NoError(t, err)
	require.Equal(t, int64(503), domainSVC.getCheckpointReq.CheckpointID)
	require.Equal(t, int64(503), getResp.Checkpoint.CheckpointID)
	require.Equal(t, "harness.terminal", getResp.Checkpoint.CheckpointNS)
	require.Equal(t, `[{"node":"resume"}]`, getResp.Checkpoint.PendingSends)

	runtimeResp, err := app.GetLatestRuntimeCheckpoint(
		context.Background(),
		&GetLatestRuntimeCheckpointRequest{
			ThreadID:    10,
			RunID:       200,
			RuntimeType: "eino_adk",
			RuntimeKey:  "checkpoint-1",
		},
	)
	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.getLatestRuntimeReq.ThreadID)
	require.Equal(t, "checkpoint-1", domainSVC.getLatestRuntimeReq.RuntimeKey)
	require.Equal(t, int64(502), runtimeResp.Checkpoint.CheckpointID)

	require.NoError(t, app.DeleteRuntimeCheckpoint(
		context.Background(),
		&DeleteRuntimeCheckpointRequest{
			ThreadID:    10,
			RunID:       200,
			RuntimeType: "eino_adk",
			RuntimeKey:  "checkpoint-1",
			DeletedAt:   700,
		},
	))
	require.Equal(t, int64(700), domainSVC.deleteRuntimeReq.DeletedAt)
}

func TestApplicationServiceRequiresThreadService(t *testing.T) {
	_, err := (*ApplicationService)(nil).CreateThread(context.Background(), &CreateThreadRequest{Title: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent thread service")

	_, err = (&ApplicationService{}).ListThreads(context.Background(), &ListThreadsRequest{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent thread service")

	_, err = (&ApplicationService{}).GetThread(context.Background(), &GetThreadRequest{ThreadID: 1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent thread service")

	_, err = (&ApplicationService{}).CompleteRun(context.Background(), &UpdateRunStatusRequest{RunID: 1})
	require.Error(t, err)
	require.Contains(t, err.Error(), "agent thread service")
}

func TestApplicationServiceRejectsNilRequests(t *testing.T) {
	app := &ApplicationService{ThreadSVC: &recordingThreadService{}}

	_, err := app.CreateThread(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "create thread request")

	_, err = app.ListThreads(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "list threads request")
}

func TestApplicationCreateThreadRejectsEmptyDomainThread(t *testing.T) {
	app := &ApplicationService{ThreadSVC: &recordingThreadService{}}

	_, err := app.CreateThread(context.Background(), &CreateThreadRequest{Title: "空结果"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "empty thread")
}

func TestInitServiceBuildsUsableThreadService(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadTableForTest(db))

	app := InitService(&ServiceComponents{
		DB:    db,
		IDGen: fixedIDGen{},
	})
	resp, err := app.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "初始化任务",
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), resp.Thread.ThreadID)
	require.Equal(t, "初始化任务", resp.Thread.Title)
}

func TestInitServiceRecordsArtifactScannerStatus(t *testing.T) {
	t.Setenv(agentArtifactScannerTypeEnv, "http")
	t.Setenv(agentArtifactScannerHTTPURLEnv, "")
	t.Setenv(agentArtifactScanOutageFailModeEnv, "open_non_executable")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadTableForTest(db))

	app := InitService(&ServiceComponents{
		DB:    db,
		IDGen: fixedIDGen{},
	})

	require.Nil(t, app.ArtifactScanner)
	require.True(t, app.ArtifactScannerStatus.Enabled)
	require.Equal(t, "http", app.ArtifactScannerStatus.Type)
	require.False(t, app.ArtifactScannerStatus.Configured)
	require.Contains(t, app.ArtifactScannerStatus.Error, "artifact scanner endpoint is required")
	require.Equal(t, ArtifactScanOutageFailModeOpenNonExecutable, app.ArtifactScanReadPolicy.OutageFailMode)
}

func TestInitServiceConfiguresMemoryExtractorFromEnv(t *testing.T) {
	t.Setenv(agentMemoryExtractorEnabledEnv, "true")
	t.Setenv(agentMemoryExtractorModelNameEnv, "memory-model")
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadTableForTest(db))

	app := InitService(&ServiceComponents{
		DB:    db,
		IDGen: fixedIDGen{},
	})

	extractor, ok := app.MemoryExtractor.(*ModelMemoryExtractor)
	require.True(t, ok)
	require.Equal(t, "memory-model", extractor.options.ModelName)
	require.NotNil(t, extractor.options.UsageCollector)
}

func TestApplicationExportsAndImportsMemories(t *testing.T) {
	domainSVC := &recordingThreadService{
		recalledMemories: []*entity.Memory{
			{
				ID:         301,
				ThreadID:   10,
				RunID:      20,
				SpaceID:    1,
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "用户偏好中文摘要",
				Metadata:   `{"origin":"manual"}`,
				Score:      0.8,
				Confidence: 0.9,
				SourceType: "manual",
				SourceID:   "memory-ui-1",
				CreatedAt:  500,
				UpdatedAt:  600,
			},
		},
		memoryTotal: 1,
		importMemoriesResult: &domainservice.ImportMemoriesResult{
			Imported: 2,
			Skipped:  1,
			Memories: []*entity.Memory{
				{
					ID:         401,
					ThreadID:   10,
					SpaceID:    1,
					Scope:      entity.MemoryScopeThread,
					Content:    "导入后的记忆",
					Confidence: 0.85,
					SourceType: "import",
					SourceID:   "file-1",
					CreatedAt:  700,
					UpdatedAt:  700,
				},
			},
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	exportResp, err := app.ExportMemories(context.Background(), &ExportMemoriesRequest{
		ThreadID:       10,
		RunID:          20,
		Query:          "中文",
		Scopes:         []MemoryScope{MemoryScopeLongTerm},
		IncludeDeleted: true,
		Limit:          50,
	})
	require.NoError(t, err)
	require.Equal(t, MemoryExportSchema, exportResp.Schema)
	require.Equal(t, int64(10), exportResp.ThreadID)
	require.NotZero(t, exportResp.ExportedAt)
	require.Equal(t, int64(1), exportResp.Total)
	require.Len(t, exportResp.Memories, 1)
	require.Equal(t, int64(20), domainSVC.listMemoriesReq.RunID)
	require.Equal(t, "中文", domainSVC.listMemoriesReq.Query)
	require.True(t, domainSVC.listMemoriesReq.IncludeDeleted)
	require.Equal(t, int32(1), domainSVC.listMemoriesReq.Page)
	require.Equal(t, int32(50), domainSVC.listMemoriesReq.PageSize)

	importResp, err := app.ImportMemories(context.Background(), &ImportMemoriesRequest{
		ThreadID: 10,
		ActorID:  7,
		Memories: []ImportMemoryItem{
			{
				Scope:      MemoryScopeThread,
				Content:    "导入后的记忆",
				Metadata:   `{"origin":"file"}`,
				Score:      0.75,
				Confidence: 0.85,
				SourceType: "import",
				SourceID:   "file-1",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), importResp.Imported)
	require.Equal(t, int64(1), importResp.Skipped)
	require.Len(t, importResp.Memories, 1)
	require.Equal(t, int64(10), domainSVC.importMemoriesReq.ThreadID)
	require.Equal(t, int64(7), domainSVC.importMemoriesReq.ActorID)
	require.Len(t, domainSVC.importMemoriesReq.Memories, 1)
	require.Equal(t, entity.MemoryScopeThread, domainSVC.importMemoriesReq.Memories[0].Scope)
	require.Equal(t, "导入后的记忆", domainSVC.importMemoriesReq.Memories[0].Content)
	require.Equal(t, "import", domainSVC.importMemoriesReq.Memories[0].SourceType)
	require.Equal(t, "file-1", domainSVC.importMemoriesReq.Memories[0].SourceID)
}

func TestApplicationMemoryManagementDeniesUnauthorizedViewerBeforeRepository(t *testing.T) {
	cases := []struct {
		name      string
		call      func(*ApplicationService) error
		operation MemoryAccessOperation
		memoryID  int64
		assert    func(*testing.T, *recordingThreadService)
	}{
		{
			name: "list",
			call: func(app *ApplicationService) error {
				_, err := app.ListMemories(context.Background(), &ListMemoriesRequest{
					ThreadID: 10,
					ViewerID: 99,
				})
				return err
			},
			operation: MemoryAccessOperationList,
			assert: func(t *testing.T, svc *recordingThreadService) {
				require.Nil(t, svc.listMemoriesReq)
			},
		},
		{
			name: "export",
			call: func(app *ApplicationService) error {
				_, err := app.ExportMemories(context.Background(), &ExportMemoriesRequest{
					ThreadID: 10,
					ViewerID: 99,
				})
				return err
			},
			operation: MemoryAccessOperationExport,
			assert: func(t *testing.T, svc *recordingThreadService) {
				require.Nil(t, svc.listMemoriesReq)
			},
		},
		{
			name: "import",
			call: func(app *ApplicationService) error {
				_, err := app.ImportMemories(context.Background(), &ImportMemoriesRequest{
					ThreadID: 10,
					ActorID:  99,
					ViewerID: 99,
					Memories: []ImportMemoryItem{{Content: "记忆"}},
				})
				return err
			},
			operation: MemoryAccessOperationImport,
			assert: func(t *testing.T, svc *recordingThreadService) {
				require.Nil(t, svc.importMemoriesReq)
			},
		},
		{
			name: "update",
			call: func(app *ApplicationService) error {
				_, err := app.UpdateMemory(context.Background(), &UpdateMemoryRequest{
					ThreadID: 10,
					MemoryID: 302,
					ViewerID: 99,
					Scope:    MemoryScopeThread,
					Content:  "记忆",
				})
				return err
			},
			operation: MemoryAccessOperationUpdate,
			memoryID:  302,
			assert: func(t *testing.T, svc *recordingThreadService) {
				require.Nil(t, svc.updateMemoryReq)
			},
		},
		{
			name: "delete",
			call: func(app *ApplicationService) error {
				_, err := app.DeleteMemory(context.Background(), &DeleteMemoryRequest{
					ThreadID: 10,
					MemoryID: 302,
					ViewerID: 99,
				})
				return err
			},
			operation: MemoryAccessOperationDelete,
			memoryID:  302,
			assert: func(t *testing.T, svc *recordingThreadService) {
				require.Nil(t, svc.deleteMemoryReq)
			},
		},
		{
			name: "clear",
			call: func(app *ApplicationService) error {
				_, err := app.ClearMemories(context.Background(), &ClearMemoriesRequest{
					ThreadID: 10,
					ViewerID: 99,
				})
				return err
			},
			operation: MemoryAccessOperationClear,
			assert: func(t *testing.T, svc *recordingThreadService) {
				require.Nil(t, svc.clearMemoriesReq)
			},
		},
		{
			name: "restore",
			call: func(app *ApplicationService) error {
				_, err := app.RestoreMemory(context.Background(), &RestoreMemoryRequest{
					ThreadID: 10,
					MemoryID: 302,
					ViewerID: 99,
				})
				return err
			},
			operation: MemoryAccessOperationRestore,
			memoryID:  302,
			assert: func(t *testing.T, svc *recordingThreadService) {
				require.Nil(t, svc.restoreMemoryReq)
			},
		},
		{
			name: "audit",
			call: func(app *ApplicationService) error {
				_, err := app.ListMemoryAuditEvents(context.Background(), &ListMemoryAuditEventsRequest{
					ThreadID: 10,
					MemoryID: 302,
					ViewerID: 99,
				})
				return err
			},
			operation: MemoryAccessOperationAudit,
			memoryID:  302,
			assert: func(t *testing.T, svc *recordingThreadService) {
				require.Nil(t, svc.listMemoryAuditReq)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			threadSVC := &recordingThreadService{}
			authorizer := &recordingMemoryAuthorizer{
				err: ErrMemoryAccessDenied,
			}
			app := &ApplicationService{
				ThreadSVC:        threadSVC,
				MemoryAuthorizer: authorizer,
			}

			err := tc.call(app)

			require.ErrorIs(t, err, ErrMemoryAccessDenied)
			require.Equal(t, tc.operation, authorizer.req.Operation)
			require.Equal(t, int64(10), authorizer.req.ThreadID)
			require.Equal(t, tc.memoryID, authorizer.req.MemoryID)
			require.Equal(t, int64(99), authorizer.req.ViewerID)
			tc.assert(t, threadSVC)
		})
	}
}

func TestThreadOwnerMemoryAuthorizerAllowsOnlyThreadCreator(t *testing.T) {
	threadSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:        10,
			CreatorID: 99,
		},
	}
	authorizer := NewThreadOwnerMemoryAuthorizer(threadSVC)

	err := authorizer.AuthorizeMemoryAccess(context.Background(), MemoryAccessRequest{
		ThreadID:  10,
		ViewerID:  99,
		Operation: MemoryAccessOperationList,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), threadSVC.getID)

	err = authorizer.AuthorizeMemoryAccess(context.Background(), MemoryAccessRequest{
		ThreadID:  10,
		ViewerID:  100,
		Operation: MemoryAccessOperationUpdate,
	})

	require.ErrorIs(t, err, ErrMemoryAccessDenied)
}

type recordingThreadService struct {
	created                  *entity.Thread
	createdRun               *entity.Run
	gotRun                   *entity.Run
	idempotentRun            *entity.Run
	claimedRuns              []*entity.Run
	completedRun             *entity.Run
	failedRun                *entity.Run
	canceledRun              *entity.Run
	listed                   []*entity.Thread
	got                      *entity.Thread
	appended                 *entity.Message
	appendedRunEvent         *entity.RunEvent
	createdCheckpoint        *entity.Checkpoint
	latestCheckpoint         *entity.Checkpoint
	checkpoint               *entity.Checkpoint
	rememberedMemory         *entity.Memory
	rememberedMemories       []*entity.Memory
	updatedMemory            *entity.Memory
	memoryAuditEvents        []*entity.MemoryAuditEvent
	deleteMemoryOK           bool
	clearMemoryCount         int64
	transcriptSnapshot       *entity.TranscriptSnapshot
	gotTranscriptSnapshot    *entity.TranscriptSnapshot
	memoryFlushJob           *entity.MemoryFlushJob
	claimedMemoryFlushJobs   []*entity.MemoryFlushJob
	completedMemoryFlushJob  *entity.MemoryFlushJob
	retriedMemoryFlushJob    *entity.MemoryFlushJob
	failedMemoryFlushJob     *entity.MemoryFlushJob
	transcriptCreated        bool
	memoryFlushCreated       bool
	memoryFlushUpdated       bool
	recordedTokenUsage       *entity.TokenUsage
	messages                 []*entity.Message
	runs                     []*entity.Run
	gotRunsByID              map[int64]*entity.Run
	claimedQueuedResumeRuns  []*entity.Run
	interruptedRun           *entity.Run
	runEvents                []*entity.RunEvent
	checkpoints              []*entity.Checkpoint
	recalledMemories         []*entity.Memory
	tokenUsageRows           []*entity.TokenUsage
	total                    int64
	messageTotal             int64
	runTotal                 int64
	runEventTotal            int64
	checkpointTotal          int64
	memoryTotal              int64
	tokenUsageTotal          int64
	tokenUsageAggregate      *entity.TokenUsageAggregate
	runTokenUsageAggregates  []*entity.RunTokenUsageAggregate
	createReq                *domainservice.CreateThreadRequest
	updateThreadTitleReq     *domainservice.UpdateThreadTitleRequest
	updateThreadMetadataReq  *domainservice.UpdateThreadMetadataRequest
	deleteThreadReq          *domainservice.DeleteThreadRequest
	deleteThreadOK           bool
	createRunReq             *domainservice.CreateRunRequest
	claimRunsReq             *domainservice.ClaimPendingRunsRequest
	claimQueuedResumeRunsReq *domainservice.ClaimQueuedResumeRunsRequest
	completeRunReq           *domainservice.UpdateRunStatusRequest
	interruptRunReq          *domainservice.UpdateRunStatusRequest
	failRunReq               *domainservice.UpdateRunStatusRequest
	cancelRunReq             *domainservice.UpdateRunStatusRequest
	appendRunEventReq        *domainservice.AppendRunEventRequest
	createCheckpointReq      *domainservice.CreateCheckpointRequest
	listCheckpointsReq       *domainservice.ListCheckpointsRequest
	getCheckpointReq         *domainservice.GetCheckpointRequest
	getLatestCheckpointReq   *domainservice.GetLatestCheckpointRequest
	getLatestRuntimeReq      *domainservice.GetLatestRuntimeCheckpointRequest
	deleteRuntimeReq         *domainservice.DeleteRuntimeCheckpointRequest
	rememberMemoryReq        *domainservice.RememberMemoryRequest
	rememberMemoryReqs       []*domainservice.RememberMemoryRequest
	importMemoriesReq        *domainservice.ImportMemoriesRequest
	importMemoriesResult     *domainservice.ImportMemoriesResult
	recallMemoriesReq        *domainservice.RecallMemoriesRequest
	listMemoriesReq          *domainservice.ListMemoriesRequest
	updateMemoryReq          *domainservice.UpdateMemoryRequest
	deleteMemoryReq          *domainservice.DeleteMemoryRequest
	restoreMemoryReq         *domainservice.RestoreMemoryRequest
	clearMemoriesReq         *domainservice.ClearMemoriesRequest
	listMemoryAuditReq       *domainservice.ListMemoryAuditEventsRequest
	persistTranscriptReq     *domainservice.PersistTranscriptSnapshotRequest
	getTranscriptSnapshotReq *domainservice.GetTranscriptSnapshotRequest
	enqueueMemoryFlushReq    *domainservice.EnqueueMemoryFlushJobRequest
	claimMemoryFlushReq      *domainservice.ClaimMemoryFlushJobsRequest
	completeMemoryFlushReq   *domainservice.CompleteMemoryFlushJobRequest
	retryMemoryFlushReq      *domainservice.RetryMemoryFlushJobRequest
	failMemoryFlushReq       *domainservice.FailMemoryFlushJobRequest
	recordTokenUsageReq      *domainservice.RecordTokenUsageRequest
	getRunTokenUsageReq      *domainservice.GetRunTokenUsageRequest
	getThreadTokenUsageReq   *domainservice.GetThreadTokenUsageRequest
	getRunByIdemSpaceID      int64
	getRunByIdemKey          string
	listReq                  *domainservice.ListThreadsRequest
	listRunsReq              *domainservice.ListRunsRequest
	listRunEventsReq         *domainservice.ListRunEventsRequest
	appendReq                *domainservice.AppendMessageRequest
	listMessagesReq          *domainservice.ListMessagesRequest
	getID                    int64
	getRunID                 int64
	completeRunErr           error
}

type recordingArtifactService struct {
	artifacts               []*entity.AgentArtifact
	registered              *entity.AgentArtifact
	got                     *entity.AgentArtifact
	deleted                 *entity.AgentArtifact
	deletedOK               bool
	restored                *entity.AgentArtifact
	restoredOK              bool
	scanUpdated             *entity.AgentArtifact
	scanUpdatedOK           bool
	claimedScanJobs         []*entity.ArtifactScanJob
	completeScanJob         *entity.ArtifactScanJob
	completeScanJobOK       bool
	retryScanJob            *entity.ArtifactScanJob
	retryScanJobOK          bool
	requeueFailedScanJob    *entity.ArtifactScanJob
	requeueFailedScanJobOK  bool
	failScanJob             *entity.ArtifactScanJob
	failScanJobOK           bool
	listScanJobs            []*entity.ArtifactScanJob
	listScanJobsTotal       int64
	cleanupCandidates       []*entity.AgentArtifact
	markFileDeletedOK       bool
	total                   int64
	registerReq             *domainservice.RegisterArtifactRequest
	listReq                 *domainservice.ListArtifactsRequest
	listScanJobsReq         *domainservice.ListArtifactScanJobsRequest
	cleanupReq              *domainservice.ListDeletedArtifactCleanupCandidatesRequest
	markFileDeletedReq      *domainservice.MarkArtifactFileDeletedRequest
	getReq                  *domainservice.GetArtifactRequest
	deleteReq               *domainservice.DeleteArtifactRequest
	restoreReq              *domainservice.RestoreArtifactRequest
	scanReq                 *domainservice.UpdateArtifactScanResultRequest
	claimScanJobsReq        *domainservice.ClaimArtifactScanJobsRequest
	completeScanJobReq      *domainservice.CompleteArtifactScanJobRequest
	retryScanJobReq         *domainservice.RetryArtifactScanJobRequest
	requeueFailedScanJobReq *domainservice.RequeueFailedArtifactScanJobRequest
	failScanJobReq          *domainservice.FailArtifactScanJobRequest
}

func (s *recordingArtifactService) RegisterArtifact(
	_ context.Context,
	req *domainservice.RegisterArtifactRequest,
) (*entity.AgentArtifact, bool, error) {
	s.registerReq = req
	if s.registered == nil {
		return nil, false, nil
	}
	cloned := *s.registered
	return &cloned, true, nil
}

func (s *recordingArtifactService) ListArtifacts(
	_ context.Context,
	req *domainservice.ListArtifactsRequest,
) ([]*entity.AgentArtifact, int64, error) {
	s.listReq = req
	return s.artifacts, s.total, nil
}

func (s *recordingArtifactService) GetArtifact(
	_ context.Context,
	req *domainservice.GetArtifactRequest,
) (*entity.AgentArtifact, error) {
	s.getReq = req
	if s.got == nil {
		return nil, nil
	}
	cloned := *s.got
	return &cloned, nil
}

func (s *recordingArtifactService) DeleteArtifact(
	_ context.Context,
	req *domainservice.DeleteArtifactRequest,
) (*entity.AgentArtifact, bool, error) {
	s.deleteReq = req
	if s.deleted == nil {
		return nil, s.deletedOK, nil
	}
	cloned := *s.deleted
	return &cloned, s.deletedOK, nil
}

func (s *recordingArtifactService) RestoreArtifact(
	_ context.Context,
	req *domainservice.RestoreArtifactRequest,
) (*entity.AgentArtifact, bool, error) {
	s.restoreReq = req
	if s.restored == nil {
		return nil, s.restoredOK, nil
	}
	cloned := *s.restored
	return &cloned, s.restoredOK, nil
}

func (s *recordingArtifactService) UpdateArtifactScanResult(
	_ context.Context,
	req *domainservice.UpdateArtifactScanResultRequest,
) (*entity.AgentArtifact, bool, error) {
	s.scanReq = req
	if s.scanUpdated == nil {
		return nil, s.scanUpdatedOK, nil
	}
	cloned := *s.scanUpdated
	return &cloned, s.scanUpdatedOK, nil
}

func (s *recordingArtifactService) ClaimArtifactScanJobs(
	_ context.Context,
	req *domainservice.ClaimArtifactScanJobsRequest,
) ([]*entity.ArtifactScanJob, error) {
	s.claimScanJobsReq = req
	return s.claimedScanJobs, nil
}

func (s *recordingArtifactService) CompleteArtifactScanJob(
	_ context.Context,
	req *domainservice.CompleteArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	s.completeScanJobReq = req
	if s.completeScanJob == nil {
		return nil, s.completeScanJobOK, nil
	}
	cloned := *s.completeScanJob
	return &cloned, s.completeScanJobOK, nil
}

func (s *recordingArtifactService) RetryArtifactScanJob(
	_ context.Context,
	req *domainservice.RetryArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	s.retryScanJobReq = req
	if s.retryScanJob == nil {
		return nil, s.retryScanJobOK, nil
	}
	cloned := *s.retryScanJob
	return &cloned, s.retryScanJobOK, nil
}

func (s *recordingArtifactService) RequeueFailedArtifactScanJob(
	_ context.Context,
	req *domainservice.RequeueFailedArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	s.requeueFailedScanJobReq = req
	if s.requeueFailedScanJob == nil {
		return nil, s.requeueFailedScanJobOK, nil
	}
	cloned := *s.requeueFailedScanJob
	return &cloned, s.requeueFailedScanJobOK, nil
}

func (s *recordingArtifactService) FailArtifactScanJob(
	_ context.Context,
	req *domainservice.FailArtifactScanJobRequest,
) (*entity.ArtifactScanJob, bool, error) {
	s.failScanJobReq = req
	if s.failScanJob == nil {
		return nil, s.failScanJobOK, nil
	}
	cloned := *s.failScanJob
	return &cloned, s.failScanJobOK, nil
}

func (s *recordingArtifactService) ListArtifactScanJobs(
	_ context.Context,
	req *domainservice.ListArtifactScanJobsRequest,
) ([]*entity.ArtifactScanJob, int64, error) {
	s.listScanJobsReq = req
	return s.listScanJobs, s.listScanJobsTotal, nil
}

func (s *recordingArtifactService) ListDeletedArtifactCleanupCandidates(
	_ context.Context,
	req *domainservice.ListDeletedArtifactCleanupCandidatesRequest,
) ([]*entity.AgentArtifact, error) {
	s.cleanupReq = req
	return s.cleanupCandidates, nil
}

func (s *recordingArtifactService) MarkArtifactFileDeleted(
	_ context.Context,
	req *domainservice.MarkArtifactFileDeletedRequest,
) (bool, error) {
	s.markFileDeletedReq = req
	return s.markFileDeletedOK, nil
}

type recordingArtifactObjectReader struct {
	objects                map[string][]byte
	key                    string
	signedURL              string
	signKey                string
	signExpire             int64
	signContentDisposition string
	signContentType        string
	deletedKeys            []string
	deleteErr              error
}

func (r *recordingArtifactObjectReader) PutObject(
	_ context.Context,
	objectKey string,
	content []byte,
	_ ...storage.PutOptFn,
) error {
	r.key = objectKey
	if r.objects == nil {
		r.objects = map[string][]byte{}
	}
	r.objects[objectKey] = append([]byte(nil), content...)
	return nil
}

func (r *recordingArtifactObjectReader) GetObject(
	_ context.Context,
	objectKey string,
) ([]byte, error) {
	r.key = objectKey
	content := r.objects[objectKey]
	cloned := append([]byte(nil), content...)
	return cloned, nil
}

func (r *recordingArtifactObjectReader) GetObjectUrl(
	_ context.Context,
	objectKey string,
	opts ...storage.GetOptFn,
) (string, error) {
	r.signKey = objectKey
	option := storage.GetOption{}
	for _, opt := range opts {
		opt(&option)
	}
	r.signExpire = option.Expire
	r.signContentDisposition = option.ResponseContentDisposition
	r.signContentType = option.ResponseContentType
	return r.signedURL, nil
}

func (r *recordingArtifactObjectReader) DeleteObject(
	_ context.Context,
	objectKey string,
) error {
	r.deletedKeys = append(r.deletedKeys, objectKey)
	return r.deleteErr
}

type recordingArtifactContentScanner struct {
	req    ArtifactScanRequest
	result *ArtifactScanResult
	err    error
}

func (s *recordingArtifactContentScanner) ScanArtifact(
	_ context.Context,
	req ArtifactScanRequest,
) (*ArtifactScanResult, error) {
	s.req = req
	return s.result, s.err
}

type recordingArtifactAuthorizer struct {
	req ArtifactAccessRequest
	err error
}

func (a *recordingArtifactAuthorizer) AuthorizeArtifactAccess(
	_ context.Context,
	req ArtifactAccessRequest,
) error {
	a.req = req
	return a.err
}

type recordingMemoryAuthorizer struct {
	req MemoryAccessRequest
	err error
}

func (a *recordingMemoryAuthorizer) AuthorizeMemoryAccess(
	_ context.Context,
	req MemoryAccessRequest,
) error {
	a.req = req
	return a.err
}

func migrateAgentThreadTableForTest(db *gorm.DB) error {
	return db.Exec(`
		CREATE TABLE agent_threads (
			id integer PRIMARY KEY,
			space_id integer,
			creator_id integer,
			agent_id integer,
			title text,
			status text,
			source text,
			legacy_task_id integer,
			metadata json,
			created_at integer,
			updated_at integer,
			last_message_at integer
		);
		CREATE TABLE agent_runs (
			id integer PRIMARY KEY,
			thread_id integer,
			parent_run_id integer DEFAULT 0,
			space_id integer,
			creator_id integer,
			assistant_id text,
			run_kind text DEFAULT 'task',
			status text,
			command json,
			input json,
			config json,
			context json,
			metadata json,
			stream_mode json,
			multitask_strategy text,
			on_disconnect text,
			durability text,
			idempotency_key text,
			worker_id text,
			error_code text,
			error_message text,
			started_at integer,
			ended_at integer,
			created_at integer,
			updated_at integer
		)
	`).Error
}

func (s *recordingThreadService) CreateThread(ctx context.Context, req *domainservice.CreateThreadRequest) (*entity.Thread, error) {
	s.createReq = req
	return s.created, nil
}

func (s *recordingThreadService) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
	s.getID = id
	return s.got, nil
}

func (s *recordingThreadService) UpdateThreadTitle(
	ctx context.Context,
	req *domainservice.UpdateThreadTitleRequest,
) (*entity.Thread, bool, error) {
	s.updateThreadTitleReq = req
	if s.got == nil {
		return nil, false, nil
	}
	updated := *s.got
	updated.Title = strings.TrimSpace(req.Title)
	return &updated, true, nil
}

func (s *recordingThreadService) UpdateThreadMetadata(
	ctx context.Context,
	req *domainservice.UpdateThreadMetadataRequest,
) (*entity.Thread, bool, error) {
	s.updateThreadMetadataReq = req
	if s.got == nil {
		return nil, false, nil
	}
	updated := *s.got
	updated.Metadata = strings.TrimSpace(req.Metadata)
	return &updated, true, nil
}

func (s *recordingThreadService) DeleteThread(
	ctx context.Context,
	req *domainservice.DeleteThreadRequest,
) (bool, error) {
	s.deleteThreadReq = req
	return s.deleteThreadOK, nil
}

func (s *recordingThreadService) ListThreads(ctx context.Context, req *domainservice.ListThreadsRequest) ([]*entity.Thread, int64, error) {
	s.listReq = req
	return s.listed, s.total, nil
}

func (s *recordingThreadService) AppendMessage(ctx context.Context, req *domainservice.AppendMessageRequest) (*entity.Message, error) {
	s.appendReq = req
	return s.appended, nil
}

func (s *recordingThreadService) ListMessages(ctx context.Context, req *domainservice.ListMessagesRequest) ([]*entity.Message, int64, error) {
	s.listMessagesReq = req
	return s.messages, s.messageTotal, nil
}

func (s *recordingThreadService) CreateRun(ctx context.Context, req *domainservice.CreateRunRequest) (*entity.Run, error) {
	s.createRunReq = req
	return s.createdRun, nil
}

func (s *recordingThreadService) GetRun(ctx context.Context, req *domainservice.GetRunRequest) (*entity.Run, error) {
	if req != nil {
		s.getRunID = req.RunID
		if s.gotRunsByID != nil {
			return s.gotRunsByID[req.RunID], nil
		}
	}
	return s.gotRun, nil
}

func (s *recordingThreadService) GetRunByIdempotencyKey(
	ctx context.Context,
	spaceID int64,
	idempotencyKey string,
) (*entity.Run, error) {
	s.getRunByIdemSpaceID = spaceID
	s.getRunByIdemKey = idempotencyKey
	return s.idempotentRun, nil
}

func (s *recordingThreadService) ListRuns(ctx context.Context, req *domainservice.ListRunsRequest) ([]*entity.Run, int64, error) {
	s.listRunsReq = req
	return s.runs, s.runTotal, nil
}

func (s *recordingThreadService) ClaimPendingRuns(ctx context.Context, req *domainservice.ClaimPendingRunsRequest) ([]*entity.Run, error) {
	s.claimRunsReq = req
	return s.claimedRuns, nil
}

func (s *recordingThreadService) ClaimQueuedResumeRuns(ctx context.Context, req *domainservice.ClaimQueuedResumeRunsRequest) ([]*entity.Run, error) {
	s.claimQueuedResumeRunsReq = req
	return s.claimedQueuedResumeRuns, nil
}

func (s *recordingThreadService) CompleteRun(ctx context.Context, req *domainservice.UpdateRunStatusRequest) (*entity.Run, error) {
	s.completeRunReq = req
	if s.completeRunErr != nil {
		return nil, s.completeRunErr
	}

	return s.completedRun, nil
}

func (s *recordingThreadService) InterruptRun(ctx context.Context, req *domainservice.UpdateRunStatusRequest) (*entity.Run, error) {
	s.interruptRunReq = req
	return s.interruptedRun, nil
}

func (s *recordingThreadService) FailRun(ctx context.Context, req *domainservice.UpdateRunStatusRequest) (*entity.Run, error) {
	s.failRunReq = req
	return s.failedRun, nil
}

func (s *recordingThreadService) CancelRun(ctx context.Context, req *domainservice.UpdateRunStatusRequest) (*entity.Run, error) {
	s.cancelRunReq = req
	return s.canceledRun, nil
}

func (s *recordingThreadService) AppendRunEvent(ctx context.Context, req *domainservice.AppendRunEventRequest) (*entity.RunEvent, error) {
	s.appendRunEventReq = req
	return s.appendedRunEvent, nil
}

func (s *recordingThreadService) ListRunEvents(ctx context.Context, req *domainservice.ListRunEventsRequest) ([]*entity.RunEvent, int64, error) {
	s.listRunEventsReq = req
	return s.runEvents, s.runEventTotal, nil
}

func (s *recordingThreadService) CreateCheckpoint(ctx context.Context, req *domainservice.CreateCheckpointRequest) (*entity.Checkpoint, error) {
	s.createCheckpointReq = req
	return s.createdCheckpoint, nil
}

func (s *recordingThreadService) ListCheckpoints(ctx context.Context, req *domainservice.ListCheckpointsRequest) ([]*entity.Checkpoint, int64, error) {
	s.listCheckpointsReq = req
	return s.checkpoints, s.checkpointTotal, nil
}

func (s *recordingThreadService) GetCheckpoint(ctx context.Context, req *domainservice.GetCheckpointRequest) (*entity.Checkpoint, error) {
	s.getCheckpointReq = req
	return s.checkpoint, nil
}

func (s *recordingThreadService) GetLatestCheckpoint(ctx context.Context, req *domainservice.GetLatestCheckpointRequest) (*entity.Checkpoint, error) {
	s.getLatestCheckpointReq = req
	return s.latestCheckpoint, nil
}

func (s *recordingThreadService) GetLatestRuntimeCheckpoint(
	ctx context.Context,
	req *domainservice.GetLatestRuntimeCheckpointRequest,
) (*entity.Checkpoint, error) {
	s.getLatestRuntimeReq = req
	return s.latestCheckpoint, nil
}

func (s *recordingThreadService) DeleteRuntimeCheckpoint(
	ctx context.Context,
	req *domainservice.DeleteRuntimeCheckpointRequest,
) error {
	s.deleteRuntimeReq = req
	return nil
}

func (s *recordingThreadService) RememberMemory(ctx context.Context, req *domainservice.RememberMemoryRequest) (*entity.Memory, error) {
	s.rememberMemoryReq = req
	s.rememberMemoryReqs = append(s.rememberMemoryReqs, req)
	index := len(s.rememberMemoryReqs) - 1
	if index >= 0 && index < len(s.rememberedMemories) {
		return s.rememberedMemories[index], nil
	}
	return s.rememberedMemory, nil
}

func (s *recordingThreadService) ImportMemories(
	ctx context.Context,
	req *domainservice.ImportMemoriesRequest,
) (*domainservice.ImportMemoriesResult, error) {
	s.importMemoriesReq = req
	if s.importMemoriesResult != nil {
		return s.importMemoriesResult, nil
	}
	return &domainservice.ImportMemoriesResult{}, nil
}

func (s *recordingThreadService) RecallMemories(ctx context.Context, req *domainservice.RecallMemoriesRequest) ([]*entity.Memory, int64, error) {
	s.recallMemoriesReq = req
	return s.recalledMemories, s.memoryTotal, nil
}

func (s *recordingThreadService) ListMemories(ctx context.Context, req *domainservice.ListMemoriesRequest) ([]*entity.Memory, int64, error) {
	s.listMemoriesReq = req
	return s.recalledMemories, s.memoryTotal, nil
}

func (s *recordingThreadService) UpdateMemory(ctx context.Context, req *domainservice.UpdateMemoryRequest) (*entity.Memory, bool, error) {
	s.updateMemoryReq = req
	return s.updatedMemory, s.updatedMemory != nil, nil
}

func (s *recordingThreadService) DeleteMemory(ctx context.Context, req *domainservice.DeleteMemoryRequest) (bool, error) {
	s.deleteMemoryReq = req
	return s.deleteMemoryOK, nil
}

func (s *recordingThreadService) RestoreMemory(
	ctx context.Context,
	req *domainservice.RestoreMemoryRequest,
) (*entity.Memory, bool, error) {
	s.restoreMemoryReq = req
	return s.updatedMemory, s.updatedMemory != nil, nil
}

func (s *recordingThreadService) ClearMemories(ctx context.Context, req *domainservice.ClearMemoriesRequest) (int64, error) {
	s.clearMemoriesReq = req
	return s.clearMemoryCount, nil
}

func (s *recordingThreadService) ListMemoryAuditEvents(
	ctx context.Context,
	req *domainservice.ListMemoryAuditEventsRequest,
) ([]*entity.MemoryAuditEvent, int64, error) {
	s.listMemoryAuditReq = req
	return s.memoryAuditEvents, int64(len(s.memoryAuditEvents)), nil
}

func (s *recordingThreadService) PersistTranscriptSnapshot(
	ctx context.Context,
	req *domainservice.PersistTranscriptSnapshotRequest,
) (*entity.TranscriptSnapshot, bool, error) {
	s.persistTranscriptReq = req
	return s.transcriptSnapshot, s.transcriptCreated, nil
}

func (s *recordingThreadService) GetTranscriptSnapshot(
	ctx context.Context,
	req *domainservice.GetTranscriptSnapshotRequest,
) (*entity.TranscriptSnapshot, error) {
	s.getTranscriptSnapshotReq = req
	return s.gotTranscriptSnapshot, nil
}

func (s *recordingThreadService) EnqueueMemoryFlushJob(
	ctx context.Context,
	req *domainservice.EnqueueMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	s.enqueueMemoryFlushReq = req
	return s.memoryFlushJob, s.memoryFlushCreated, nil
}

func (s *recordingThreadService) ClaimMemoryFlushJobs(
	_ context.Context,
	req *domainservice.ClaimMemoryFlushJobsRequest,
) ([]*entity.MemoryFlushJob, error) {
	s.claimMemoryFlushReq = req
	return s.claimedMemoryFlushJobs, nil
}

func (s *recordingThreadService) CompleteMemoryFlushJob(
	_ context.Context,
	req *domainservice.CompleteMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	s.completeMemoryFlushReq = req
	return s.completedMemoryFlushJob, s.memoryFlushUpdated, nil
}

func (s *recordingThreadService) RetryMemoryFlushJob(
	_ context.Context,
	req *domainservice.RetryMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	s.retryMemoryFlushReq = req
	return s.retriedMemoryFlushJob, s.memoryFlushUpdated, nil
}

func (s *recordingThreadService) FailMemoryFlushJob(
	_ context.Context,
	req *domainservice.FailMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	s.failMemoryFlushReq = req
	return s.failedMemoryFlushJob, s.memoryFlushUpdated, nil
}

func (s *recordingThreadService) RecordTokenUsage(ctx context.Context, req *domainservice.RecordTokenUsageRequest) (*entity.TokenUsage, error) {
	s.recordTokenUsageReq = req
	return s.recordedTokenUsage, nil
}

func (s *recordingThreadService) GetRunTokenUsage(ctx context.Context, req *domainservice.GetRunTokenUsageRequest) ([]*entity.TokenUsage, int64, *entity.TokenUsageAggregate, []*entity.RunTokenUsageAggregate, error) {
	s.getRunTokenUsageReq = req
	return s.tokenUsageRows, s.tokenUsageTotal, s.tokenUsageAggregate, s.runTokenUsageAggregates, nil
}

func (s *recordingThreadService) GetThreadTokenUsage(ctx context.Context, req *domainservice.GetThreadTokenUsageRequest) ([]*entity.TokenUsage, int64, *entity.TokenUsageAggregate, error) {
	s.getThreadTokenUsageReq = req
	return s.tokenUsageRows, s.tokenUsageTotal, s.tokenUsageAggregate, nil
}

type recordingMemoryExtractor struct {
	req   MemoryExtractionRequest
	facts []MemoryExtractionFact
	err   error
}

func (e *recordingMemoryExtractor) ExtractMemories(
	_ context.Context,
	req MemoryExtractionRequest,
) ([]MemoryExtractionFact, error) {
	e.req = req
	if e.err != nil {
		return nil, e.err
	}
	return e.facts, nil
}

type recordingMemoryUpdateExtractor struct {
	req    MemoryExtractionRequest
	result *MemoryExtractionResult
	err    error
}

func (e *recordingMemoryUpdateExtractor) ExtractMemories(
	ctx context.Context,
	req MemoryExtractionRequest,
) ([]MemoryExtractionFact, error) {
	result, err := e.ExtractMemoryUpdates(ctx, req)
	if err != nil || result == nil {
		return nil, err
	}
	return result.Facts, nil
}

func (e *recordingMemoryUpdateExtractor) ExtractMemoryUpdates(
	_ context.Context,
	req MemoryExtractionRequest,
) (*MemoryExtractionResult, error) {
	e.req = req
	if e.err != nil {
		return nil, e.err
	}
	return e.result, nil
}

type fixedIDGen struct{}

func (fixedIDGen) GenID(ctx context.Context) (int64, error) {
	return 1, nil
}

func (fixedIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		ids[i] = int64(i + 1)
	}

	return ids, nil
}
