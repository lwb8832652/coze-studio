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
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestCreateThreadRequiresTitle(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

	_, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "  ",
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestCreateThreadDefaultsToIdleWebTask(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

	thread, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "分析订单异常",
	})

	require.NoError(t, err)
	require.Equal(t, int64(901), thread.ID)
	require.Equal(t, entity.ThreadStatusIdle, thread.Status)
	require.Equal(t, entity.ThreadSourceWeb, thread.Source)
	require.Equal(t, int64(1), thread.SpaceID)
	require.Equal(t, int64(2), thread.CreatorID)
	require.Equal(t, "分析订单异常", thread.Title)
	require.NotZero(t, thread.CreatedAt)
	require.Equal(t, thread.CreatedAt, thread.UpdatedAt)
	require.Equal(t, thread.CreatedAt, thread.LastMessageAt)
}

func TestCreateThreadTrimsTitleAndPreservesSource(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 902}})

	thread, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID:  1,
		UserID:   2,
		AgentID:  3,
		Title:    "  IM 任务  ",
		Source:   entity.ThreadSourceIM,
		Metadata: `{"channel":"slack"}`,
	})

	require.NoError(t, err)
	require.Equal(t, "IM 任务", thread.Title)
	require.Equal(t, entity.ThreadSourceIM, thread.Source)
	require.Equal(t, int64(3), thread.AgentID)
	require.Equal(t, `{"channel":"slack"}`, thread.Metadata)
}

func TestCreateThreadRunMessageCreatesOneAtomicAggregate(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(901)})

	result, err := svc.CreateThreadRunMessage(
		context.Background(),
		&CreateThreadRunMessageRequest{
			Thread: CreateThreadRequest{
				SpaceID: 1, UserID: 2, Title: "atomic task",
			},
			Run: CreateRunRequest{
				Input:  `{"messages":[{"role":"user","content":"start"}]}`,
				Config: `{"runtime":"eino_adk"}`,
			},
			Message: CreateMessageSpec{
				Role: entity.MessageRoleUser, Content: " start ", Metadata: `{"source":"new_task"}`,
			},
		},
	)

	require.NoError(t, err)
	require.Equal(t, int64(901), result.Thread.ID)
	require.Equal(t, int64(902), result.Run.ID)
	require.Equal(t, int64(903), result.Message.ID)
	require.Equal(t, result.Thread.ID, result.Run.ThreadID)
	require.Equal(t, result.Thread.SpaceID, result.Run.SpaceID)
	require.Equal(t, result.Thread.CreatorID, result.Run.CreatorID)
	require.Equal(t, result.Thread.ID, result.Message.ThreadID)
	require.Equal(t, result.Run.ID, result.Message.RunID)
	require.Equal(t, "start", result.Message.Content)
	require.Equal(t, result.Thread.CreatedAt, result.Run.CreatedAt)
	require.Equal(t, result.Run.CreatedAt, result.Message.CreatedAt)
	require.Equal(t, 1, repo.createThreadBundleCalls)
	require.Len(t, repo.threads, 1)
	require.Len(t, repo.runs[result.Thread.ID], 1)
	require.Len(t, repo.messages[result.Thread.ID], 1)
}

func TestCreateThreadRunMessageAllocatesBeforePersistence(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{
		Repo:  repo,
		IDGen: failingIDGen{err: errors.New("id generator unavailable")},
	})

	result, err := svc.CreateThreadRunMessage(
		context.Background(),
		&CreateThreadRunMessageRequest{
			Thread:  CreateThreadRequest{SpaceID: 1, UserID: 2, Title: "atomic task"},
			Run:     CreateRunRequest{Input: `{"messages":[{"role":"user","content":"start"}]}`},
			Message: CreateMessageSpec{Role: entity.MessageRoleUser, Content: "start"},
		},
	)

	require.Nil(t, result)
	require.ErrorContains(t, err, "id generator unavailable")
	require.Zero(t, repo.createThreadBundleCalls)
	require.Empty(t, repo.threads)
}

func TestCreateThreadRunMessageEnrollsJournalInSameAtomicAggregate(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(1001)})

	result, err := svc.CreateThreadRunMessage(
		context.Background(),
		&CreateThreadRunMessageRequest{
			Thread: CreateThreadRequest{SpaceID: 1, UserID: 2, Title: "journal task"},
			Run: CreateRunRequest{
				RunKind: entity.RunKindTask,
				Input:   `{"messages":[{"role":"user","content":"start"}]}`,
				Config:  `{"runtime":"eino_adk","mode":"pro"}`,
			},
			Message:       CreateMessageSpec{Role: entity.MessageRoleUser, Content: "start"},
			EnrollJournal: true,
			JournalEnrollment: &JournalEnrollmentOptions{
				EnrollmentVersion: entity.JournalSchemaVersion,
				SnapshotsEnabled:  true,
			},
		},
	)

	require.NoError(t, err)
	require.NotNil(t, result.Attempt)
	require.Equal(t, int64(1004), result.Attempt.ID)
	require.Equal(t, result.Run.ID, result.Attempt.JournalRunID)
	require.Equal(t, result.Run.ID, result.Attempt.ExecutionRunID)
	require.Equal(t, entity.JournalSchemaVersion, result.Attempt.EnrollmentVersion)
	require.True(t, result.Attempt.SnapshotsEnabled)
	require.Len(t, repo.runAttempts[result.Run.ID], 1)
}

func TestCreateRunBundleBindsMessageAndEventToGeneratedRun(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2001)})

	result, err := svc.CreateRunBundle(context.Background(), &CreateRunBundleRequest{
		Run: CreateRunRequest{
			ThreadID: 10, Status: entity.RunStatusQueued,
			Input: `{"messages":[]}`, IdempotencyKey: "resume-key",
		},
		Message: &CreateMessageSpec{
			Role: entity.MessageRoleUser, Content: "resume",
		},
		Event: &CreateRunEventSpec{
			EventType: "human.interaction.resolved",
			PayloadBuilder: func(runID int64) string {
				return fmt.Sprintf(`{"resume_run_id":%d}`, runID)
			},
		},
	})

	require.NoError(t, err)
	require.True(t, result.Created)
	require.Equal(t, int64(2001), result.Run.ID)
	require.Equal(t, int64(2002), result.Message.ID)
	require.Equal(t, int64(2003), result.Event.ID)
	require.Equal(t, result.Run.ID, result.Message.RunID)
	require.Equal(t, result.Run.ID, result.Event.RunID)
	require.JSONEq(t, `{"resume_run_id":2001}`, result.Event.Payload)
	require.Equal(t, 1, repo.createRunBundleCalls)
}

func TestCreateRunBundleCanEnrollFirstJournalAttemptAtomically(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2101)})

	result, err := svc.CreateRunBundle(context.Background(), &CreateRunBundleRequest{
		Run: CreateRunRequest{
			ThreadID: 10, Status: entity.RunStatusQueued,
			Input: `{"messages":[]}`, IdempotencyKey: "journal-run",
		},
		EnrollJournal: true,
		JournalEnrollment: &JournalEnrollmentOptions{
			EnrollmentVersion: "1.1",
			SnapshotsEnabled:  true,
			TraceID:           "trace-enrollment",
		},
	})

	require.NoError(t, err)
	require.True(t, result.Created)
	require.Equal(t, int64(2101), result.Run.ID)
	require.NotNil(t, result.Attempt)
	require.Equal(t, int64(2102), result.Attempt.ID)
	require.Equal(t, result.Run.ID, result.Attempt.JournalRunID)
	require.Equal(t, result.Run.ID, result.Attempt.ExecutionRunID)
	require.Equal(t, "att_2102", result.Attempt.AttemptID)
	require.Equal(t, uint32(1), result.Attempt.Ordinal)
	require.Equal(t, entity.RunAttemptStatusPending, result.Attempt.Status)
	require.Equal(t, uint64(1), result.Attempt.NextSequence)
	require.Zero(t, result.Attempt.LastCommittedSequence)
	require.Equal(t, "1.1", result.Attempt.EnrollmentVersion)
	require.True(t, result.Attempt.SnapshotsEnabled)
	require.Equal(t, entity.JournalProjectionStateHealthy, result.Attempt.ProjectionState)
	require.NotNil(t, result.Attempt.TraceID)
	require.Equal(t, "trace-enrollment", *result.Attempt.TraceID)
}

func TestCreateRunBundleBuildsAtomicExpiredLeaseRecoveryBoundary(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2201)})

	result, err := svc.CreateRunBundle(context.Background(), &CreateRunBundleRequest{
		Run: CreateRunRequest{
			ThreadID: 10, Status: entity.RunStatusQueued, Input: `{"messages":[]}`,
			IdempotencyKey: "lease-recovery-1", MultitaskStrategy: "reject",
		},
		EnrollJournal: true,
		JournalEnrollment: &JournalEnrollmentOptions{
			Recovery: &JournalRecoveryEnrollmentOptions{
				JournalRunID: 50, SourceCheckpointID: 700,
				SourceAttemptID: "att_100", IdempotencyKey: "lease-recovery-1",
				ExpiredLease: &JournalRecoveryExpiredLeaseOptions{
					RunID: 50, LeaseOwner: "worker-a", LeaseToken: "lease-50",
					ExecutionGeneration: 3, Now: 2000,
					ErrorCode:    "run_recovered",
					ErrorMessage: "execution recovered from a durable checkpoint",
				},
			},
		},
	})

	require.NoError(t, err)
	require.True(t, result.Created)
	require.Equal(t, int64(2201), result.Run.ID)
	require.Equal(t, int64(2203), result.Attempt.ID)
	require.NotNil(t, repo.lastCreateRunBundleReq)
	source := repo.lastCreateRunBundleReq.RecoverySourceLease
	require.NotNil(t, source)
	require.Equal(t, int64(50), source.RunID)
	require.Equal(t, entity.RunStatusFailed, source.ToStatus)
	require.Equal(t, int64(2202), source.Event.ID)
	require.Equal(t, "run.failed", source.Event.EventType)
	require.Contains(t, source.Event.Payload, `"error_code":"run_recovered"`)
	require.NotNil(t, source.JournalEvent)
	require.Equal(t, "run.lifecycle", source.JournalEvent.EventType)
	require.Equal(t, string(entity.RunAttemptStatusFailed), source.JournalEvent.Status)
}

func TestCreateRunBundleRejectsUnsupportedJournalEnrollmentVersion(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2101)})

	_, err := svc.CreateRunBundle(context.Background(), &CreateRunBundleRequest{
		Run: CreateRunRequest{
			ThreadID: 10, Status: entity.RunStatusQueued,
			Input: `{"messages":[]}`, IdempotencyKey: "journal-unsupported-version",
		},
		EnrollJournal: true,
		JournalEnrollment: &JournalEnrollmentOptions{
			EnrollmentVersion: "2.0",
		},
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.Equal(t, 1, repo.createRunBundleCalls)
}

func TestCreateRunBundleReportsEnrollmentVersionDriftAsIdempotencyConflict(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2101)})
	request := &CreateRunBundleRequest{
		Run: CreateRunRequest{
			ThreadID: 10, Status: entity.RunStatusQueued,
			Input: `{"messages":[]}`, IdempotencyKey: "journal-version-replay",
		},
		EnrollJournal: true,
		JournalEnrollment: &JournalEnrollmentOptions{
			EnrollmentVersion: entity.JournalSchemaVersion,
		},
	}
	_, err := svc.CreateRunBundle(context.Background(), request)
	require.NoError(t, err)

	request.JournalEnrollment.EnrollmentVersion = "2.0"
	_, err = svc.CreateRunBundle(context.Background(), request)
	require.ErrorIs(t, err, repository.ErrRunIdempotencyConflict)
	require.Equal(t, 2, repo.createRunBundleCalls)
}

func TestCreateJournalAttemptGeneratesServerIDAndDelegatesRecoveryTaskRun(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, RunKind: entity.RunKindTask, Status: entity.RunStatusRunning},
		{ID: 21, ThreadID: 10, RunKind: entity.RunKindTask, Status: entity.RunStatusRunning, StartedAt: 123},
	}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2201)})
	sourceAttemptID := "att_2102"

	attempt, err := svc.CreateJournalAttempt(context.Background(), &CreateJournalAttemptRequest{
		JournalRunID: 20, ExecutionRunID: 21,
		SourceAttemptID: &sourceAttemptID, RecoveryIdempotencyKey: "recovery-1",
		TraceID: "trace-recovery",
	})

	require.NoError(t, err)
	require.Equal(t, int64(2201), attempt.ID)
	require.Equal(t, int64(10), attempt.ThreadID)
	require.Equal(t, int64(20), attempt.JournalRunID)
	require.Equal(t, int64(21), attempt.ExecutionRunID)
	require.Equal(t, "att_2201", attempt.AttemptID)
	require.Equal(t, entity.RunAttemptStatusRunning, attempt.Status)
	require.Equal(t, "recovery-1", *attempt.RecoveryIdempotencyKey)
	require.Equal(t, sourceAttemptID, *attempt.SourceAttemptID)
	require.Equal(t, int64(123), *attempt.StartedAt)
	require.Equal(t, "trace-recovery", *attempt.TraceID)
	require.Equal(t, attempt, repo.lastJournalAttempt)
}

func TestCreateJournalAttemptRejectsSubagentExecution(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, RunKind: entity.RunKindTask, Status: entity.RunStatusRunning},
		{ID: 21, ThreadID: 10, ParentRunID: 20, RunKind: entity.RunKindSubagent, Status: entity.RunStatusRunning},
	}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2201)})

	_, err := svc.CreateJournalAttempt(context.Background(), &CreateJournalAttemptRequest{
		JournalRunID: 20, ExecutionRunID: 21, RecoveryIdempotencyKey: "subagent-recovery",
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.Nil(t, repo.lastJournalAttempt)
}

func TestCreateJournalAttemptMapsTerminalExecutionForRepositoryReplay(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, RunKind: entity.RunKindTask, Status: entity.RunStatusRunning},
		{ID: 21, ThreadID: 10, RunKind: entity.RunKindTask, Status: entity.RunStatusSucceeded},
	}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2201)})

	attempt, err := svc.CreateJournalAttempt(context.Background(), &CreateJournalAttemptRequest{
		JournalRunID: 20, ExecutionRunID: 21, RecoveryIdempotencyKey: "terminal-replay",
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunAttemptStatusCompleted, attempt.Status)
	require.Equal(t, entity.RunAttemptStatusCompleted, repo.lastJournalAttempt.Status)
}

func TestAppendAndGetJournalEventUseServerOwnedRunThread(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{ID: 20, ThreadID: 10, RunKind: entity.RunKindTask}}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2301)})

	event, err := svc.AppendJournalEvent(context.Background(), &AppendJournalEventRequest{
		RunID: 20, Visibility: entity.JournalVisibilityPublic,
		IdempotencyKey: "service-event", EventType: "message.delta",
		Payload: `{"type":"test","data":{}}`,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2301), event.ID)
	require.Equal(t, int64(10), event.ThreadID)
	require.Equal(t, event, repo.lastJournalEvent)
	got, err := svc.GetJournalEvent(context.Background(), event.ID)
	require.NoError(t, err)
	require.Equal(t, event, got)
}

func TestFinalizeJournalAttemptGeneratesEventIDAndDelegatesStatus(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{ID: 20, ThreadID: 10, RunKind: entity.RunKindTask}}
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(2401)})

	event, won, err := svc.FinalizeJournalAttempt(context.Background(), &FinalizeJournalAttemptRequest{
		Status: entity.RunAttemptStatusTimedOut,
		Event: AppendJournalEventRequest{
			RunID: 20, IdempotencyKey: "timeout", EventType: "run.lifecycle",
			Status:  string(entity.RunAttemptStatusTimedOut),
			Payload: `{"type":"terminal","data":{}}`,
		},
		EndedAt: 1234,
	})

	require.NoError(t, err)
	require.True(t, won)
	require.Equal(t, int64(2401), event.ID)
	require.Equal(t, entity.RunAttemptStatusTimedOut, repo.lastFinalizeJournalReq.Status)
	require.Equal(t, int64(1234), repo.lastFinalizeJournalReq.EndedAt)
}

func TestListThreadsNormalizesPaging(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(901)})
	_, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "A",
	})
	require.NoError(t, err)
	_, err = svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "B",
	})
	require.NoError(t, err)

	threads, total, err := svc.ListThreads(context.Background(), &ListThreadsRequest{
		SpaceID: 1,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, threads, 2)
	require.Equal(t, int32(1), repo.lastListReq.Page)
	require.Equal(t, int32(20), repo.lastListReq.PageSize)
}

func TestListThreadsRequiresRequest(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

	_, _, err := svc.ListThreads(context.Background(), nil)

	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestUpdateThreadTitleTrimsAndPersistsTitle(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})
	thread, err := svc.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1,
		UserID:  2,
		Title:   "请生成一份《武汉3日游攻略》正式文档",
	})
	require.NoError(t, err)

	updated, ok, err := svc.UpdateThreadTitle(context.Background(), &UpdateThreadTitleRequest{
		ThreadID:  thread.ID,
		Title:     "  武汉3日游攻略  ",
		UpdatedAt: 200,
	})

	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "武汉3日游攻略", updated.Title)
	require.Equal(t, int64(200), updated.UpdatedAt)
	require.Equal(t, thread.LastMessageAt, updated.LastMessageAt)
	require.Equal(t, repository.UpdateThreadTitleRequest{
		ThreadID:  thread.ID,
		Title:     "武汉3日游攻略",
		UpdatedAt: 200,
	}, repo.lastUpdateThreadTitleReq)
}

func TestUpdateThreadTitleRequiresTitle(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 901}})

	_, _, err := svc.UpdateThreadTitle(context.Background(), &UpdateThreadTitleRequest{
		ThreadID: 1,
		Title:    " ",
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestAppendMessageRequiresContentAndValidRole(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	_, err := svc.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 1,
		Role:     entity.MessageRoleUser,
		Content:  "  ",
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	_, err = svc.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 1,
		Role:     entity.MessageRole("bad"),
		Content:  "hello",
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestAppendMessageCreatesMessageWithGeneratedID(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	message, err := svc.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 10,
		RunID:    20,
		Role:     entity.MessageRoleUser,
		Content:  "  请分析客户反馈  ",
		Metadata: `{"source":"web"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(1001), message.ID)
	require.Equal(t, int64(10), message.ThreadID)
	require.Equal(t, int64(20), message.RunID)
	require.Equal(t, entity.MessageRoleUser, message.Role)
	require.Equal(t, "请分析客户反馈", message.Content)
	require.Equal(t, `{"source":"web"}`, message.Metadata)
	require.NotZero(t, message.CreatedAt)
	require.Len(t, repo.messages[10], 1)
}

func TestAppendMessageRequiresExistingThread(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	_, err := svc.AppendMessage(context.Background(), &AppendMessageRequest{
		ThreadID: 10,
		Role:     entity.MessageRoleUser,
		Content:  "hello",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "thread")
	require.Empty(t, repo.messages[10])
}

func TestListMessagesNormalizesPaging(t *testing.T) {
	repo := newMemoryRepo()
	repo.messages[10] = []*entity.Message{
		{ID: 1, ThreadID: 10, Role: entity.MessageRoleUser, Content: "第一条", CreatedAt: 1},
		{ID: 2, ThreadID: 10, Role: entity.MessageRoleAssistant, Content: "第二条", CreatedAt: 2},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	messages, total, err := svc.ListMessages(context.Background(), &ListMessagesRequest{
		ThreadID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, messages, 2)
	require.Equal(t, int32(1), repo.lastMessageListReq.Page)
	require.Equal(t, int32(50), repo.lastMessageListReq.PageSize)
}

func TestListRecentMessagesByRolesDelegatesOptimizedRepositoryQuery(t *testing.T) {
	repo := newMemoryRepo()
	repo.messages[10] = []*entity.Message{
		{ID: 1, ThreadID: 10, Role: entity.MessageRoleUser, Content: "old", CreatedAt: 100},
		{ID: 2, ThreadID: 10, Role: entity.MessageRoleSystem, Content: "system", CreatedAt: 200},
		{ID: 3, ThreadID: 10, Role: entity.MessageRoleAssistant, Content: "new", CreatedAt: 300},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1001}})

	messages, err := svc.ListRecentMessagesByRoles(context.Background(), &ListRecentMessagesByRolesRequest{
		ThreadID: 10,
		Roles:    []entity.MessageRole{entity.MessageRoleUser, entity.MessageRoleAssistant},
		Limit:    2,
	})

	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, int64(1), messages[0].ID)
	require.Equal(t, int64(3), messages[1].ID)
	require.Equal(t, int64(10), repo.lastRecentMessagesByRolesReq.ThreadID)
	require.Equal(t, []entity.MessageRole{entity.MessageRoleUser, entity.MessageRoleAssistant}, repo.lastRecentMessagesByRolesReq.Roles)
	require.Equal(t, int32(2), repo.lastRecentMessagesByRolesReq.Limit)
}

func TestCreateRunRequiresExistingThreadAndInput(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 0,
		Input:    `{}`,
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	_, err = svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Input:    "  ",
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	_, err = svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Input:    `{}`,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "thread")
}

func TestCreateRunDefaultsStatusAndRuntimeOptions(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Input:    `{"messages":[{"role":"user","content":"hello"}]}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2001), run.ID)
	require.Equal(t, int64(10), run.ThreadID)
	require.Equal(t, int64(1), run.SpaceID)
	require.Equal(t, int64(2), run.CreatorID)
	require.Equal(t, "default", run.AssistantID)
	require.Equal(t, int64(0), run.ParentRunID)
	require.Equal(t, entity.RunKindTask, run.RunKind)
	require.Equal(t, entity.RunStatusPending, run.Status)
	require.Equal(t, `{}`, run.Command)
	require.Equal(t, `{"messages":[{"role":"user","content":"hello"}]}`, run.Input)
	require.Equal(t, `{}`, run.Config)
	require.Equal(t, `{}`, run.Context)
	require.Equal(t, `{}`, run.Metadata)
	require.Equal(t, `["messages","updates"]`, run.StreamMode)
	require.Equal(t, "reject", run.MultitaskStrategy)
	require.Equal(t, "cancel", run.OnDisconnect)
	require.Equal(t, "async", run.Durability)
	require.NotZero(t, run.CreatedAt)
	require.Equal(t, run.CreatedAt, run.UpdatedAt)
	require.Len(t, repo.runs[10], 1)
	require.Equal(t, 1, repo.createRunWithThreadLockCalls)
	require.Zero(t, repo.createRunCalls)
}

func TestCreateRunValidatesDisconnectModeAcrossCreationPaths(t *testing.T) {
	t.Run("explicit continue is preserved", func(t *testing.T) {
		repo := newMemoryRepo()
		repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
		svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

		run, err := svc.CreateRun(context.Background(), &CreateRunRequest{
			ThreadID: 10, Input: `{"messages":[]}`, OnDisconnect: "continue",
		})

		require.NoError(t, err)
		require.Equal(t, "continue", run.OnDisconnect)
	})

	t.Run("single run rejects unknown mode", func(t *testing.T) {
		repo := newMemoryRepo()
		repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
		svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

		run, err := svc.CreateRun(context.Background(), &CreateRunRequest{
			ThreadID: 10, Input: `{"messages":[]}`, OnDisconnect: "detach",
		})

		require.Nil(t, run)
		require.Error(t, err)
		require.True(t, IsClientError(err))
		require.Empty(t, repo.runs[10])
	})

	t.Run("run bundle rejects unknown mode", func(t *testing.T) {
		repo := newMemoryRepo()
		repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
		svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

		result, err := svc.CreateRunBundle(context.Background(), &CreateRunBundleRequest{
			Run: CreateRunRequest{
				ThreadID: 10, Input: `{"messages":[]}`, OnDisconnect: "detach",
			},
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.True(t, IsClientError(err))
		require.Empty(t, repo.runs[10])
	})

	t.Run("new thread aggregate rejects unknown mode", func(t *testing.T) {
		repo := newMemoryRepo()
		svc := NewService(&Components{Repo: repo, IDGen: newSequenceIDGen(901)})

		result, err := svc.CreateThreadRunMessage(context.Background(), &CreateThreadRunMessageRequest{
			Thread: CreateThreadRequest{SpaceID: 1, UserID: 2, Title: "atomic task"},
			Run: CreateRunRequest{
				Input: `{"messages":[]}`, OnDisconnect: "detach",
			},
			Message: CreateMessageSpec{Role: entity.MessageRoleUser, Content: "start"},
		})

		require.Nil(t, result)
		require.Error(t, err)
		require.True(t, IsClientError(err))
		require.Zero(t, repo.createThreadBundleCalls)
	})
}

func TestCreateRunRejectsUnsupportedTopLevelMultitaskStrategy(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID:          10,
		Input:             `{"messages":[{"role":"user","content":"hello"}]}`,
		MultitaskStrategy: "enqueue",
	})

	require.Nil(t, run)
	require.ErrorIs(t, err, ErrUnsupportedMultitaskStrategy)
	require.Empty(t, repo.runs[10])
}

func TestCreateRunBundleAllowsMessageLessPublicRun(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	result, err := svc.CreateRunBundle(context.Background(), &CreateRunBundleRequest{
		Run: CreateRunRequest{
			ThreadID: 10,
			Input:    `{"messages":[{"role":"user","content":"hello"}]}`,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(2001), result.Run.ID)
	require.Nil(t, result.Message)
	require.Nil(t, result.Event)
}

func TestCreateRunCreatesSubagentRunWhenParentRunIsSet(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	repo.runs[10] = []*entity.Run{{
		ID:        100,
		ThreadID:  10,
		SpaceID:   1,
		CreatorID: 2,
		RunKind:   entity.RunKindTask,
		Status:    entity.RunStatusRunning,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID:    10,
		ParentRunID: 100,
		AssistantID: "singleagent:1001",
		Input:       `{"messages":[{"role":"user","content":"delegate"}]}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(100), run.ParentRunID)
	require.Equal(t, entity.RunKindSubagent, run.RunKind)
	require.Equal(t, "singleagent:1001", run.AssistantID)
	require.Len(t, repo.runs[10], 2)
	require.Equal(t, entity.RunKindSubagent, repo.runs[10][1].RunKind)
}

func TestCreateRunAcceptsRunningSubagentInitialStatus(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	repo.runs[10] = []*entity.Run{{
		ID:        100,
		ThreadID:  10,
		SpaceID:   1,
		CreatorID: 2,
		RunKind:   entity.RunKindTask,
		Status:    entity.RunStatusRunning,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID:    10,
		ParentRunID: 100,
		Status:      entity.RunStatusRunning,
		Input:       `{"messages":[]}`,
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusRunning, run.Status)
	require.NotZero(t, run.StartedAt)
}

func TestCreateRunRejectsSubagentRunWithMismatchedParentThread(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	repo.threads[11] = &entity.Thread{ID: 11, SpaceID: 1, CreatorID: 2}
	repo.runs[11] = []*entity.Run{{
		ID:       100,
		ThreadID: 11,
		RunKind:  entity.RunKindTask,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID:    10,
		ParentRunID: 100,
		Input:       `{"messages":[{"role":"user","content":"delegate"}]}`,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "parent run thread")
}

func TestCreateRunAcceptsQueuedInitialStatus(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Input:    `{"messages":[{"role":"user","content":"resume"}]}`,
		Status:   entity.RunStatusQueued,
		Metadata: `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusQueued, run.Status)
	require.Equal(t, entity.RunStatusQueued, repo.runs[10][0].Status)
}

func TestCreateRunRejectsUnsupportedInitialStatus(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: 10,
		Input:    `{"messages":[{"role":"user","content":"resume"}]}`,
		Status:   entity.RunStatusRunning,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "initial run status")
}

func TestThreadServiceGetRunByIdempotencyKey(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID:             100,
		ThreadID:       10,
		SpaceID:        1,
		Status:         entity.RunStatusQueued,
		IdempotencyKey: "idem-1",
	}}
	repo.runs[11] = []*entity.Run{{
		ID:             101,
		ThreadID:       11,
		SpaceID:        2,
		Status:         entity.RunStatusQueued,
		IdempotencyKey: "idem-1",
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.GetRunByIdempotencyKey(context.Background(), 1, "idem-1")

	require.NoError(t, err)
	require.NotNil(t, run)
	require.Equal(t, int64(100), run.ID)

	missing, err := svc.GetRunByIdempotencyKey(context.Background(), 1, "missing")
	require.NoError(t, err)
	require.Nil(t, missing)

	_, err = svc.GetRunByIdempotencyKey(context.Background(), 0, "idem-1")
	require.Error(t, err)
	require.Contains(t, err.Error(), "space id is required")

	_, err = svc.GetRunByIdempotencyKey(context.Background(), 1, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "idempotency key is invalid")
}

func TestListRunsNormalizesPaging(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 1, ThreadID: 10, Status: entity.RunStatusPending, CreatedAt: 1},
		{ID: 2, ThreadID: 10, Status: entity.RunStatusRunning, CreatedAt: 2},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	runs, total, err := svc.ListRuns(context.Background(), &ListRunsRequest{
		ThreadID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, runs, 2)
	require.Equal(t, int32(1), repo.lastRunListReq.Page)
	require.Equal(t, int32(20), repo.lastRunListReq.PageSize)
}

func TestClaimPendingRunsRequiresWorkerID(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.ClaimPendingRuns(context.Background(), &ClaimPendingRunsRequest{
		WorkerID: "  ",
		Limit:    1,
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestClaimPendingRunsNormalizesLimit(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 1, ThreadID: 10, Status: entity.RunStatusPending, CreatedAt: 1},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	claimed, err := svc.ClaimPendingRuns(context.Background(), &ClaimPendingRunsRequest{
		WorkerID:       " worker-a ",
		Now:            1_000,
		LeaseTTLMillis: 5_000,
	})

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, entity.RunStatusRunning, claimed[0].Status)
	require.Equal(t, "worker-a", claimed[0].WorkerID)
	require.Equal(t, int32(10), repo.lastClaimReq.Limit)
	require.Equal(t, int64(1_000), repo.lastClaimReq.Now)
	require.Equal(t, int64(5_000), repo.lastClaimReq.LeaseTTLMillis)
	require.Equal(t, "worker-a", claimed[0].LeaseOwner)
	require.NotEmpty(t, claimed[0].LeaseToken)
	require.Equal(t, int64(6_000), claimed[0].LeaseExpiresAt)
	require.Equal(t, uint64(1), claimed[0].ExecutionGeneration)
}

func TestClaimQueuedResumeRunsRequiresWorkerID(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.ClaimQueuedResumeRuns(context.Background(), &ClaimQueuedResumeRunsRequest{
		WorkerID: " ",
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.Contains(t, err.Error(), "worker id")
}

func TestClaimQueuedResumeRunsNormalizesLimit(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{
			ID:        1,
			ThreadID:  10,
			Status:    entity.RunStatusQueued,
			Metadata:  `{"checkpoint_resume":{"protected_from_worker_claim":true}}`,
			CreatedAt: 1,
		},
		{
			ID:        2,
			ThreadID:  10,
			Status:    entity.RunStatusQueued,
			Metadata:  `{"source":"manual_queue"}`,
			CreatedAt: 2,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	claimed, err := svc.ClaimQueuedResumeRuns(context.Background(), &ClaimQueuedResumeRunsRequest{
		WorkerID:       "resume-worker-a",
		Limit:          0,
		Now:            2_000,
		LeaseTTLMillis: 6_000,
	})

	require.NoError(t, err)
	require.Len(t, claimed, 1)
	require.Equal(t, "resume-worker-a", repo.lastClaimQueuedResumeReq.WorkerID)
	require.Equal(t, int32(10), repo.lastClaimQueuedResumeReq.Limit)
	require.Equal(t, int64(2_000), repo.lastClaimQueuedResumeReq.Now)
	require.Equal(t, int64(6_000), repo.lastClaimQueuedResumeReq.LeaseTTLMillis)
	require.Equal(t, entity.RunStatusRunning, claimed[0].Status)
	require.Equal(t, int64(1), claimed[0].ID)
	require.Equal(t, "resume-worker-a", claimed[0].LeaseOwner)
	require.NotEmpty(t, claimed[0].LeaseToken)
	require.Equal(t, uint64(1), claimed[0].ExecutionGeneration)
}

func TestRenewRunLeaseForwardsFenceCredentials(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID:                  1,
		ThreadID:            10,
		Status:              entity.RunStatusRunning,
		LeaseOwner:          "worker-a",
		LeaseToken:          "lease-1",
		ExecutionGeneration: 2,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	renewed, err := svc.RenewRunLease(context.Background(), &RenewRunLeaseRequest{
		RunID:               1,
		LeaseOwner:          " worker-a ",
		LeaseToken:          " lease-1 ",
		ExecutionGeneration: 2,
		Now:                 3_000,
		LeaseTTLMillis:      4_000,
	})

	require.NoError(t, err)
	require.Equal(t, int64(3_000), renewed.HeartbeatAt)
	require.Equal(t, int64(7_000), renewed.LeaseExpiresAt)
	require.Equal(t, "worker-a", repo.lastRenewRunLeaseReq.LeaseOwner)
	require.Equal(t, "lease-1", repo.lastRenewRunLeaseReq.LeaseToken)
}

func TestReleaseRunLeaseForwardsFenceCredentials(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID:                  1,
		ThreadID:            10,
		Status:              entity.RunStatusRunning,
		LeaseOwner:          "worker-a",
		LeaseToken:          "lease-1",
		ExecutionGeneration: 2,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	released, err := svc.ReleaseRunLease(context.Background(), &ReleaseRunLeaseRequest{
		RunID:               1,
		LeaseOwner:          "worker-a",
		LeaseToken:          "lease-1",
		ExecutionGeneration: 2,
		ToStatus:            entity.RunStatusPending,
		Now:                 3_000,
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusPending, released.Status)
	require.Equal(t, entity.RunStatusPending, repo.lastReleaseRunLeaseReq.ToStatus)
	require.Equal(t, int64(3_000), repo.lastReleaseRunLeaseReq.Now)
}

func TestListExpiredRunLeasesForwardsClockAndLimit(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID:                  1,
		ThreadID:            10,
		Status:              entity.RunStatusRunning,
		LeaseExpiresAt:      2_000,
		ExecutionGeneration: 1,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	expired, err := svc.ListExpiredRunLeases(context.Background(), &ListExpiredRunLeasesRequest{
		Now:   2_001,
		Limit: 5,
	})

	require.NoError(t, err)
	require.Len(t, expired, 1)
	require.Equal(t, int64(2_001), repo.lastListExpiredRunLeasesReq.Now)
	require.Equal(t, int32(5), repo.lastListExpiredRunLeasesReq.Limit)
}

func TestReconcileExpiredRunLeaseValidatesTargetAndForwardsFence(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID: 1, ThreadID: 10, Status: entity.RunStatusRunning,
		LeaseOwner: "worker-a", LeaseToken: "lease-1", LeaseExpiresAt: 2_000,
		ExecutionGeneration: 2,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.ReconcileExpiredRunLease(context.Background(), &ReconcileExpiredRunLeaseRequest{
		RunID:               1,
		LeaseOwner:          "worker-a",
		LeaseToken:          "lease-1",
		ExecutionGeneration: 2,
		ToStatus:            entity.RunStatusSucceeded,
		Now:                 2_001,
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	reconciled, err := svc.ReconcileExpiredRunLease(context.Background(), &ReconcileExpiredRunLeaseRequest{
		RunID:               1,
		LeaseOwner:          " worker-a ",
		LeaseToken:          " lease-1 ",
		ExecutionGeneration: 2,
		ToStatus:            entity.RunStatusFailed,
		Now:                 2_001,
		ErrorCode:           " run_abandoned ",
		ErrorMessage:        " execution lease expired without a recoverable checkpoint ",
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusFailed, reconciled.Status)
	require.Equal(t, "worker-a", repo.lastReconcileExpiredRunLeaseReq.LeaseOwner)
	require.Equal(t, "lease-1", repo.lastReconcileExpiredRunLeaseReq.LeaseToken)
	require.Equal(t, uint64(2), repo.lastReconcileExpiredRunLeaseReq.ExecutionGeneration)
	require.Equal(t, "run_abandoned", repo.lastReconcileExpiredRunLeaseReq.ErrorCode)
	require.Equal(t, "execution lease expired without a recoverable checkpoint", repo.lastReconcileExpiredRunLeaseReq.ErrorMessage)
	require.NotNil(t, repo.lastReconcileExpiredRunLeaseReq.Event)
	require.Equal(t, int64(2001), repo.lastReconcileExpiredRunLeaseReq.Event.ID)
	require.Equal(t, "run.failed", repo.lastReconcileExpiredRunLeaseReq.Event.EventType)
	require.JSONEq(t, `{"status":"failed","error_code":"run_abandoned"}`, repo.lastReconcileExpiredRunLeaseReq.Event.Payload)
	require.NotNil(t, repo.lastReconcileExpiredRunLeaseReq.JournalEvent)
	require.Equal(t, "run.lifecycle", repo.lastReconcileExpiredRunLeaseReq.JournalEvent.EventType)
	require.Equal(t, string(entity.RunAttemptStatusFailed), repo.lastReconcileExpiredRunLeaseReq.JournalEvent.Status)
}

func TestRequestRunCancellationPersistsEventAndReturnsPreviousStatus(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID: 1, ThreadID: 10, Status: entity.RunStatusRunning,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-1",
		LeaseExpiresAt: 5_000, ExecutionGeneration: 2,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 900}})

	result, err := svc.RequestRunCancellation(context.Background(), &RequestRunCancellationRequest{
		RunID: 1,
		Now:   3_000,
	})

	require.NoError(t, err)
	require.True(t, result.Changed)
	require.Equal(t, entity.RunStatusRunning, result.PreviousStatus)
	require.Equal(t, entity.RunStatusCanceled, result.Run.Status)
	require.Equal(t, int64(3_000), result.Run.CancelRequestedAt)
	require.Equal(t, uint64(3), result.Run.ExecutionGeneration)
	require.NotNil(t, repo.lastRequestRunCancellationReq.Event)
	require.Equal(t, int64(900), repo.lastRequestRunCancellationReq.Event.ID)
	require.Equal(t, "run.canceled", repo.lastRequestRunCancellationReq.Event.EventType)
	require.JSONEq(t, `{"status":"canceled"}`, repo.lastRequestRunCancellationReq.Event.Payload)
	require.NotNil(t, repo.lastRequestRunCancellationReq.JournalEvent)
	require.Equal(t, string(entity.RunAttemptStatusCancelled), repo.lastRequestRunCancellationReq.JournalEvent.Status)
}

func TestRequestRunCancellationReturnsCanceledRunWithoutAllocatingEventID(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID: 1, ThreadID: 10, Status: entity.RunStatusCanceled,
		CancelRequestedAt: 3_000, ExecutionGeneration: 3,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: failingIDGen{err: errors.New("id generator unavailable")}})

	result, err := svc.RequestRunCancellation(context.Background(), &RequestRunCancellationRequest{RunID: 1})

	require.NoError(t, err)
	require.False(t, result.Changed)
	require.Equal(t, entity.RunStatusCanceled, result.PreviousStatus)
	require.Equal(t, int64(3_000), result.Run.CancelRequestedAt)
	require.Nil(t, repo.lastRequestRunCancellationReq.Event)
}

func TestFinalizeRunSuccessGeneratesAssistantMessageAndForwardsTitleFence(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, Title: "initial title"}
	repo.runs[10] = []*entity.Run{{
		ID: 1, ThreadID: 10, Status: entity.RunStatusRunning,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-1",
		LeaseExpiresAt: 5_000, ExecutionGeneration: 2,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 300}})

	result, err := svc.FinalizeRunSuccess(context.Background(), &FinalizeRunSuccessRequest{
		RunID: 1, ThreadID: 10,
		LeaseOwner: "worker-a", LeaseToken: "lease-1", ExecutionGeneration: 2,
		Now: 3_000, Message: " final answer ", MessageMetadata: `{"source":"eino_adk"}`,
		ExpectedThreadTitle: "initial title", ThreadTitle: "generated title",
		TerminalCheckpoint: &CreateCheckpointRequest{
			ParentCheckpointID: 50,
			CheckpointNS:       "eino.adk", RuntimeType: "eino_adk",
			RuntimeKey: "coze-run-1", EnvelopeVersion: 2,
			ChannelValues:   `{"envelope_version":2}`,
			ChannelVersions: `{}`, PendingSends: `[]`, Metadata: `{}`,
		},
		TerminalCheckpointOnTitleConflict: &CreateCheckpointRequest{
			ParentCheckpointID: 50,
			CheckpointNS:       "eino.adk", RuntimeType: "eino_adk",
			RuntimeKey: "coze-run-1", EnvelopeVersion: 2,
			ChannelValues:   `{"envelope_version":2,"title":"initial title"}`,
			ChannelVersions: `{}`, PendingSends: `[]`, Metadata: `{}`,
		},
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusSucceeded, result.Run.Status)
	require.Equal(t, int64(300), result.Message.ID)
	require.Equal(t, "final answer", result.Message.Content)
	require.True(t, result.TitleUpdated)
	require.Equal(t, "lease-1", repo.lastFinalizeRunSuccessReq.LeaseToken)
	require.Equal(t, uint64(2), repo.lastFinalizeRunSuccessReq.ExecutionGeneration)
	require.Equal(t, "initial title", repo.lastFinalizeRunSuccessReq.ExpectedThreadTitle)
	require.Equal(t, "generated title", repo.lastFinalizeRunSuccessReq.ThreadTitle)
	require.Equal(t, int64(301), result.TitleEvent.ID)
	require.Equal(t, "context.thread_title_updated", result.TitleEvent.EventType)
	require.Equal(t, int64(302), result.CompletionEvent.ID)
	require.Equal(t, "run.completed", result.CompletionEvent.EventType)
	require.NotNil(t, repo.lastFinalizeRunSuccessReq.JournalEvent)
	require.Equal(t, string(entity.RunAttemptStatusCompleted), repo.lastFinalizeRunSuccessReq.JournalEvent.Status)
	require.NotNil(t, result.TerminalCheckpoint)
	require.Equal(t, int64(303), result.TerminalCheckpoint.ID)
	require.Equal(t, int64(50), result.TerminalCheckpoint.ParentCheckpointID)
	require.Equal(t, int64(303), repo.lastFinalizeRunSuccessReq.TerminalCheckpoint.ID)
	require.Equal(t, int64(303), repo.lastFinalizeRunSuccessReq.TerminalCheckpointOnTitleConflict.ID)
	require.Len(t, repo.runEvents[1], 2)
	require.Equal(t, "context.thread_title_updated", repo.runEvents[1][0].EventType)
	require.Equal(t, "run.completed", repo.runEvents[1][1].EventType)
}

func TestCompleteRunTransitionsRunningToSucceeded(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{
			ID:                  1,
			ThreadID:            10,
			Status:              entity.RunStatusRunning,
			WorkerID:            "worker-a",
			LeaseOwner:          "worker-a",
			LeaseToken:          "lease-1",
			ExecutionGeneration: 1,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.CompleteRun(context.Background(), &UpdateRunStatusRequest{
		RunID:               1,
		From:                entity.RunStatusRunning,
		WorkerID:            "worker-a",
		LeaseOwner:          "worker-a",
		LeaseToken:          "lease-1",
		ExecutionGeneration: 1,
		Now:                 4_000,
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusSucceeded, run.Status)
	require.Equal(t, entity.RunStatusRunning, repo.lastUpdateRunReq.From)
	require.Equal(t, entity.RunStatusSucceeded, repo.lastUpdateRunReq.To)
	require.Equal(t, "worker-a", repo.lastUpdateRunReq.LeaseOwner)
	require.Equal(t, "lease-1", repo.lastUpdateRunReq.LeaseToken)
	require.Equal(t, uint64(1), repo.lastUpdateRunReq.ExecutionGeneration)
	require.Equal(t, int64(4_000), repo.lastUpdateRunReq.Now)
	require.NotNil(t, repo.lastUpdateRunReq.Event)
	require.Equal(t, int64(2001), repo.lastUpdateRunReq.Event.ID)
	require.Equal(t, "run.completed", repo.lastUpdateRunReq.Event.EventType)
	require.JSONEq(t, `{"status":"succeeded","worker_id":"worker-a"}`, repo.lastUpdateRunReq.Event.Payload)
	require.NotNil(t, repo.lastUpdateRunReq.JournalEvent)
	require.Equal(t, string(entity.RunAttemptStatusCompleted), repo.lastUpdateRunReq.JournalEvent.Status)
}

func TestCompleteRunBindsOutboxIntentToDurableTerminalEvent(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID: 1, ThreadID: 10, Status: entity.RunStatusRunning,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-1",
		ExecutionGeneration: 1,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.CompleteRun(context.Background(), &UpdateRunStatusRequest{
		RunID: 1, From: entity.RunStatusRunning, WorkerID: "worker-a",
		LeaseOwner: "worker-a", LeaseToken: "lease-1", ExecutionGeneration: 1,
		Now: 4_000,
		OutboxIntent: &repository.NotificationOutboxIntent{Event: domainnotification.Event{
			EventID:          "run-event:placeholder:completed",
			EventType:        domainnotification.EventTaskCompleted,
			AggregateType:    "agent_run",
			AggregateID:      "run:1",
			AggregateVersion: 1,
		}},
	})

	require.NoError(t, err)
	require.NotNil(t, repo.lastUpdateRunReq.OutboxIntent)
	require.Equal(t, "run-event:2001:succeeded", repo.lastUpdateRunReq.OutboxIntent.Event.EventID)
	require.Equal(t, int64(2001), repo.lastUpdateRunReq.OutboxIntent.Event.AggregateVersion)
	require.Equal(t, domainnotification.EventTaskCompleted, repo.lastUpdateRunReq.OutboxIntent.Event.EventType)
}

func TestInterruptRunUsesPrePersistedEventWithoutAllocatingAnotherID(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID: 1, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "worker-a",
	}}
	svc := NewService(&Components{
		Repo:  repo,
		IDGen: failingIDGen{err: errors.New("id generator must not be called")},
	})

	run, err := svc.InterruptRun(context.Background(), &UpdateRunStatusRequest{
		RunID: 1, From: entity.RunStatusRunning, WorkerID: "worker-a",
		EventAlreadyPersisted: true,
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusInterrupted, run.Status)
	require.True(t, repo.lastUpdateRunReq.EventAlreadyPersisted)
	require.Nil(t, repo.lastUpdateRunReq.Event)
	require.Empty(t, repo.runEvents[1])
}

func TestInterruptRunPassesPrePersistedAwaitingInputPayloadForOutboxAppend(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID: 1, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "worker-a",
	}}
	svc := NewService(&Components{
		Repo:  repo,
		IDGen: failingIDGen{err: errors.New("id generator must not be called")},
	})
	var appended []domainnotification.Event
	payload := `{"status":"interrupted","awaiting_input":{"schema":"coze.agentthread.awaiting_input.v1","interaction_event_id":"interrupt-event-1","interaction_id":"hi_1","kind":"clarification"}}`

	run, err := svc.InterruptRun(context.Background(), &UpdateRunStatusRequest{
		RunID: 1, From: entity.RunStatusRunning, WorkerID: "worker-a",
		EventAlreadyPersisted: true,
		EventPayload:          payload,
		OutboxIntent: &repository.NotificationOutboxIntent{
			Event: domainnotification.Event{
				EventID:          "interaction-event:interrupt-event-1:task.awaiting_input",
				EventType:        domainnotification.EventTaskAwaitingInput,
				AggregateType:    "agent_run_interaction",
				AggregateID:      "interaction:interrupt-event-1",
				AggregateVersion: 1,
			},
			Append: func(_ context.Context, _ *gorm.DB, event domainnotification.Event) error {
				appended = append(appended, event)
				return nil
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusInterrupted, run.Status)
	require.Equal(t, payload, repo.lastUpdateRunReq.EventPayload)
	require.True(t, repo.lastUpdateRunReq.EventAlreadyPersisted)
	require.NotNil(t, repo.lastUpdateRunReq.OutboxIntent)
	require.Len(t, appended, 1)
	require.Equal(t, domainnotification.EventTaskAwaitingInput, appended[0].EventType)
	_, err = svc.InterruptRun(context.Background(), &UpdateRunStatusRequest{
		RunID: 1, From: entity.RunStatusRunning, WorkerID: "worker-a",
		EventAlreadyPersisted: true,
		EventPayload:          payload,
		OutboxIntent:          repo.lastUpdateRunReq.OutboxIntent,
	})
	require.Error(t, err)
	require.Len(t, appended, 1)
}

func TestInterruptRunDoesNotAppendGenericPrePersistedInterruptOutbox(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID: 1, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "worker-a",
	}}
	svc := NewService(&Components{
		Repo:  repo,
		IDGen: failingIDGen{err: errors.New("id generator must not be called")},
	})
	var appendCalls int

	run, err := svc.InterruptRun(context.Background(), &UpdateRunStatusRequest{
		RunID: 1, From: entity.RunStatusRunning, WorkerID: "worker-a",
		EventAlreadyPersisted: true,
		EventPayload:          `{"status":"interrupted","interrupt_count":1}`,
		OutboxIntent: &repository.NotificationOutboxIntent{
			Event: domainnotification.Event{
				EventID:          "interaction-event:interrupt-event-1:task.awaiting_input",
				EventType:        domainnotification.EventTaskAwaitingInput,
				AggregateType:    "agent_run_interaction",
				AggregateID:      "interaction:interrupt-event-1",
				AggregateVersion: 1,
			},
			Append: func(context.Context, *gorm.DB, domainnotification.Event) error {
				appendCalls++
				return nil
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusInterrupted, run.Status)
	require.Zero(t, appendCalls)
	require.Equal(t, `{"status":"interrupted","interrupt_count":1}`, repo.lastUpdateRunReq.EventPayload)
}

func TestFailRunStoresError(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 1, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "worker-a"},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	run, err := svc.FailRun(context.Background(), &UpdateRunStatusRequest{
		RunID:        1,
		From:         entity.RunStatusRunning,
		WorkerID:     "worker-a",
		ErrorCode:    "model_error",
		ErrorMessage: "model failed",
		EventPayload: `{"status":"failed","error_code":"caller_value","error_message":"provider secret","raw_provider":"hidden"}`,
	})

	require.NoError(t, err)
	require.Equal(t, entity.RunStatusFailed, run.Status)
	require.Equal(t, "model_error", run.ErrorCode)
	require.Equal(t, "model failed", run.ErrorMessage)
	require.JSONEq(t, `{"status":"failed","worker_id":"worker-a","error_code":"model_error"}`, repo.lastUpdateRunReq.Event.Payload)
	require.NotContains(t, repo.lastUpdateRunReq.Event.Payload, "provider secret")
	require.NotContains(t, repo.lastUpdateRunReq.Event.Payload, "raw_provider")
	require.NotNil(t, repo.lastUpdateRunReq.JournalEvent)
	require.Equal(t, string(entity.RunAttemptStatusFailed), repo.lastUpdateRunReq.JournalEvent.Status)
}

func TestFailRunProjectsStandardTimeoutCodeAsTimedOut(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 1, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "worker-a"},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 2001}})

	_, err := svc.FailRun(context.Background(), &UpdateRunStatusRequest{
		RunID: 1, From: entity.RunStatusRunning, WorkerID: "worker-a",
		ErrorCode: "task_timeout", ErrorMessage: "execution deadline reached",
	})

	require.NoError(t, err)
	require.NotNil(t, repo.lastUpdateRunReq.JournalEvent)
	require.Equal(t, string(entity.RunAttemptStatusTimedOut), repo.lastUpdateRunReq.JournalEvent.Status)
	require.JSONEq(t,
		`{"type":"terminal","data":{"status":"timed_out"}}`,
		repo.lastUpdateRunReq.JournalEvent.Payload,
	)
}

func TestAppendRunEventCreatesEventFromRun(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, Status: entity.RunStatusRunning},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3001}})

	event, err := svc.AppendRunEvent(context.Background(), &AppendRunEventRequest{
		RunID:     20,
		EventType: "  run.started  ",
		Payload:   `{"status":"running"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(3001), event.ID)
	require.Equal(t, int64(10), event.ThreadID)
	require.Equal(t, int64(20), event.RunID)
	require.Equal(t, "run.started", event.EventType)
	require.Equal(t, `{"status":"running"}`, event.Payload)
	require.NotZero(t, event.CreatedAt)
	require.Len(t, repo.runEvents[20], 1)
	require.Equal(t, "run.started", repo.runEvents[20][0].EventType)
}

func TestAppendRunEventRequiresRunAndEventType(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3001}})

	_, err := svc.AppendRunEvent(context.Background(), &AppendRunEventRequest{
		EventType: "run.started",
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	_, err = svc.AppendRunEvent(context.Background(), &AppendRunEventRequest{
		RunID:     20,
		EventType: "  ",
	})
	require.Error(t, err)
	require.True(t, IsClientError(err))

	_, err = svc.AppendRunEvent(context.Background(), &AppendRunEventRequest{
		RunID:     20,
		EventType: "run.started",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "run")
	require.Empty(t, repo.runEvents[20])
}

func TestAppendRunEventRejectsMismatchedThread(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, Status: entity.RunStatusRunning},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3001}})

	_, err := svc.AppendRunEvent(context.Background(), &AppendRunEventRequest{
		ThreadID:  11,
		RunID:     20,
		EventType: "run.started",
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.Empty(t, repo.runEvents[20])
}

func TestListRunEventsNormalizesPaging(t *testing.T) {
	repo := newMemoryRepo()
	repo.runEvents[20] = []*entity.RunEvent{
		{ID: 1, ThreadID: 10, RunID: 20, EventType: "run.started", Payload: `{}`, CreatedAt: 1},
		{ID: 2, ThreadID: 10, RunID: 20, EventType: "run.completed", Payload: `{}`, CreatedAt: 2},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3001}})

	events, total, err := svc.ListRunEvents(context.Background(), &ListRunEventsRequest{
		RunID:        20,
		AfterEventID: 7,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, events, 2)
	require.Equal(t, int64(7), repo.lastRunEventListReq.AfterEventID)
	require.Equal(t, int32(1), repo.lastRunEventListReq.Page)
	require.Equal(t, int32(100), repo.lastRunEventListReq.PageSize)
}

func TestListRunEventsRejectsNegativeCursor(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3001}})

	_, _, err := svc.ListRunEvents(context.Background(), &ListRunEventsRequest{
		RunID:        20,
		AfterEventID: -1,
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
}

func TestCreateCheckpointDefaultsJSONAndPersistsRunCheckpoint(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, SpaceID: 1, CreatorID: 2, Status: entity.RunStatusRunning},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3501}})

	checkpoint, err := svc.CreateCheckpoint(context.Background(), &CreateCheckpointRequest{
		RunID:              20,
		ParentCheckpointID: 12,
		CheckpointNS:       " planner ",
		RuntimeType:        " eino_adk ",
		RuntimeKey:         " checkpoint-1 ",
		EnvelopeVersion:    1,
		ChannelValues:      `{"messages":["hello"]}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(3501), checkpoint.ID)
	require.Equal(t, int64(10), checkpoint.ThreadID)
	require.Equal(t, int64(20), checkpoint.RunID)
	require.Equal(t, int64(12), checkpoint.ParentCheckpointID)
	require.Equal(t, "planner", checkpoint.CheckpointNS)
	require.Equal(t, "eino_adk", checkpoint.RuntimeType)
	require.Equal(t, "checkpoint-1", checkpoint.RuntimeKey)
	require.Equal(t, int32(1), checkpoint.EnvelopeVersion)
	require.Equal(t, `{"messages":["hello"]}`, checkpoint.ChannelValues)
	require.Equal(t, `{}`, checkpoint.ChannelVersions)
	require.Equal(t, `[]`, checkpoint.PendingSends)
	require.Equal(t, `{}`, checkpoint.Metadata)
	require.NotZero(t, checkpoint.CreatedAt)
	require.Len(t, repo.checkpoints[10], 1)
}

func TestCreateCheckpointRejectsMismatchedThread(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, SpaceID: 1, CreatorID: 2, Status: entity.RunStatusRunning},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3501}})

	_, err := svc.CreateCheckpoint(context.Background(), &CreateCheckpointRequest{
		ThreadID:      11,
		RunID:         20,
		ChannelValues: `{}`,
	})

	require.Error(t, err)
	require.True(t, IsClientError(err))
	require.Empty(t, repo.checkpoints[10])
}

func TestListCheckpointsNormalizesLimit(t *testing.T) {
	repo := newMemoryRepo()
	repo.checkpoints[10] = []*entity.Checkpoint{
		{ID: 1, ThreadID: 10, RunID: 20, RuntimeType: "eino_adk", CreatedAt: 100},
		{ID: 2, ThreadID: 10, RunID: 20, RuntimeType: "eino_adk", CreatedAt: 200},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3501}})

	checkpoints, total, err := svc.ListCheckpoints(context.Background(), &ListCheckpointsRequest{
		ThreadID:    10,
		RuntimeType: "eino_adk",
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, checkpoints, 2)
	require.Equal(t, int64(2), checkpoints[0].ID)
	require.Equal(t, "eino_adk", repo.lastCheckpointListReq.RuntimeType)
	require.Equal(t, int32(20), repo.lastCheckpointListReq.Limit)
}

func TestGetLatestCheckpointReturnsNewest(t *testing.T) {
	repo := newMemoryRepo()
	repo.checkpoints[10] = []*entity.Checkpoint{
		{ID: 1, ThreadID: 10, RunID: 20, CreatedAt: 100},
		{ID: 2, ThreadID: 10, RunID: 20, CreatedAt: 200},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3501}})

	checkpoint, err := svc.GetLatestCheckpoint(context.Background(), &GetLatestCheckpointRequest{
		ThreadID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), checkpoint.ID)
}

func TestRuntimeCheckpointLifecycleDelegatesToRepository(t *testing.T) {
	repo := newMemoryRepo()
	repo.checkpoints[10] = []*entity.Checkpoint{
		{
			ID:              1,
			ThreadID:        10,
			RunID:           20,
			RuntimeType:     "eino_adk",
			RuntimeKey:      "checkpoint-1",
			EnvelopeVersion: 1,
			CreatedAt:       100,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3501}})

	checkpoint, err := svc.GetLatestRuntimeCheckpoint(
		context.Background(),
		&GetLatestRuntimeCheckpointRequest{
			ThreadID:    10,
			RunID:       20,
			RuntimeType: " eino_adk ",
			RuntimeKey:  " checkpoint-1 ",
		},
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), checkpoint.ID)

	require.NoError(t, svc.DeleteRuntimeCheckpoint(
		context.Background(),
		&DeleteRuntimeCheckpointRequest{
			ThreadID:    10,
			RunID:       20,
			RuntimeType: "eino_adk",
			RuntimeKey:  "checkpoint-1",
			DeletedAt:   500,
		},
	))
	require.Equal(t, int64(500), repo.checkpoints[10][0].RuntimeDeletedAt)

	checkpoint, err = svc.GetLatestRuntimeCheckpoint(
		context.Background(),
		&GetLatestRuntimeCheckpointRequest{
			ThreadID:    10,
			RunID:       20,
			RuntimeType: "eino_adk",
			RuntimeKey:  "checkpoint-1",
		},
	)
	require.NoError(t, err)
	require.Nil(t, checkpoint)
}

func TestGetCheckpointReturnsCheckpointByID(t *testing.T) {
	repo := newMemoryRepo()
	repo.checkpoints[10] = []*entity.Checkpoint{
		{ID: 1, ThreadID: 10, RunID: 20, CreatedAt: 100},
		{ID: 2, ThreadID: 10, RunID: 20, CheckpointNS: "harness.terminal", CreatedAt: 200},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 3501}})

	checkpoint, err := svc.GetCheckpoint(context.Background(), &GetCheckpointRequest{
		CheckpointID: 2,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), repo.lastCheckpointID)
	require.Equal(t, int64(2), checkpoint.ID)
	require.Equal(t, "harness.terminal", checkpoint.CheckpointNS)
}

func TestRememberMemoryCreatesThreadMemory(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4001}})

	memory, err := svc.RememberMemory(context.Background(), &RememberMemoryRequest{
		ThreadID: 10,
		RunID:    20,
		Content:  "  用户偏好中文回答  ",
		Metadata: `{"source":"profile"}`,
		Score:    0.75,
	})

	require.NoError(t, err)
	require.Equal(t, int64(4001), memory.ID)
	require.Equal(t, int64(10), memory.ThreadID)
	require.Zero(t, memory.RunID)
	require.Equal(t, int64(1), memory.SpaceID)
	require.Equal(t, entity.MemoryScopeThread, memory.Scope)
	require.Equal(t, "用户偏好中文回答", memory.Content)
	require.Equal(t, `{"source":"profile"}`, memory.Metadata)
	require.Equal(t, 0.75, memory.Score)
	require.NotZero(t, memory.CreatedAt)
	require.Equal(t, memory.CreatedAt, memory.UpdatedAt)
	require.Len(t, repo.memories[10], 1)
}

func TestRememberLongTermMemoryPersistsFactFields(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4002}})

	memory, err := svc.RememberMemory(context.Background(), &RememberMemoryRequest{
		ThreadID:             10,
		RunID:                20,
		Scope:                entity.MemoryScopeLongTerm,
		Content:              "  用户喜欢简短中文总结  ",
		Score:                0.76,
		Confidence:           0.92,
		SourceType:           " transcript_summary ",
		SourceID:             " snapshot-200 ",
		CorrectionOfMemoryID: 300,
		CorrectedAt:          900,
	})

	require.NoError(t, err)
	require.Equal(t, int64(4002), memory.ID)
	require.Equal(t, entity.MemoryScopeLongTerm, memory.Scope)
	require.Zero(t, memory.RunID)
	require.Equal(t, "用户喜欢简短中文总结", memory.Content)
	require.Equal(t, 0.76, memory.Score)
	require.Equal(t, 0.92, memory.Confidence)
	require.Equal(t, "transcript_summary", memory.SourceType)
	require.Equal(t, "snapshot-200", memory.SourceID)
	require.Equal(t, int64(300), memory.CorrectionOfMemoryID)
	require.Equal(t, int64(900), memory.CorrectedAt)
	require.Len(t, repo.memories[10], 1)
	require.Equal(t, "snapshot-200", repo.memories[10][0].SourceID)
}

func TestRememberMemoryUsesSourceIdempotency(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	repo.sourceMemory = &entity.Memory{
		ID:         3001,
		ThreadID:   10,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "已有事实",
		SourceType: "transcript_summary",
		SourceID:   "snapshot:501:language",
	}
	repo.sourceMemoryCreated = false
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4003}})

	memory, err := svc.RememberMemory(context.Background(), &RememberMemoryRequest{
		ThreadID:   10,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "重复事实",
		SourceType: "transcript_summary",
		SourceID:   "snapshot:501:language",
	})

	require.NoError(t, err)
	require.Equal(t, int64(3001), memory.ID)
	require.Equal(t, "已有事实", memory.Content)
	require.NotNil(t, repo.lastCreateOrGetMemoryBySource)
	require.Equal(t, "snapshot:501:language", repo.lastCreateOrGetMemoryBySource.SourceID)
	require.Empty(t, repo.memories[10])
}

func TestRememberMemoryRejectsInvalidConfidence(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4003}})

	_, err := svc.RememberMemory(context.Background(), &RememberMemoryRequest{
		ThreadID:   10,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "bad confidence",
		Confidence: 1.01,
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "memory confidence")
	require.Empty(t, repo.memories[10])
}

func TestRememberRunMemoryRequiresRunID(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4001}})

	_, err := svc.RememberMemory(context.Background(), &RememberMemoryRequest{
		ThreadID: 10,
		Scope:    entity.MemoryScopeRun,
		Content:  "only for this run",
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "run id is required")
	require.Empty(t, repo.memories[10])

	memory, err := svc.RememberMemory(context.Background(), &RememberMemoryRequest{
		ThreadID: 10,
		RunID:    20,
		Scope:    entity.MemoryScopeRun,
		Content:  "only for this run",
	})

	require.NoError(t, err)
	require.Equal(t, int64(20), memory.RunID)
	require.Equal(t, entity.MemoryScopeRun, memory.Scope)
}

func TestRecallMemoriesNormalizesLimitAndUsesRunContext(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	repo.memories[10] = []*entity.Memory{
		{
			ID:       1,
			ThreadID: 10,
			RunID:    0,
			Scope:    entity.MemoryScopeThread,
			Content:  "thread memory",
			Score:    0.8,
		},
		{
			ID:       2,
			ThreadID: 10,
			RunID:    20,
			Scope:    entity.MemoryScopeRun,
			Content:  "run memory",
			Score:    0.9,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4001}})

	memories, total, err := svc.RecallMemories(context.Background(), &RecallMemoriesRequest{
		ThreadID: 10,
		RunID:    20,
		Query:    "  memory  ",
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, memories, 2)
	require.Equal(t, int64(10), repo.lastMemoryListReq.ThreadID)
	require.Equal(t, int64(20), repo.lastMemoryListReq.RunID)
	require.Equal(t, "memory", repo.lastMemoryListReq.Query)
	require.Equal(t, int32(8), repo.lastMemoryListReq.Limit)
	require.NotZero(t, repo.lastMemoryListReq.Now)
}

func TestManageMemoriesNormalizesAndDelegates(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	repo.memories[10] = []*entity.Memory{
		{
			ID:         1,
			ThreadID:   10,
			Scope:      entity.MemoryScopeLongTerm,
			Content:    "用户偏好中文回答",
			Score:      0.8,
			Confidence: 0.9,
		},
	}
	repo.updatedMemory = &entity.Memory{
		ID:         1,
		ThreadID:   10,
		SpaceID:    1,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "用户偏好简短中文回答",
		Metadata:   `{"source":"edited"}`,
		Score:      0.9,
		Confidence: 0.95,
		UpdatedAt:  900,
	}
	repo.restoredMemory = &entity.Memory{
		ID:        1,
		ThreadID:  10,
		SpaceID:   1,
		Scope:     entity.MemoryScopeLongTerm,
		Content:   "用户偏好简短中文回答",
		UpdatedAt: 1200,
	}
	repo.deleteMemoryOK = true
	repo.clearMemoryCount = 1
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4001}})

	memories, total, err := svc.ListMemories(context.Background(), &ListMemoriesRequest{
		ThreadID:       10,
		Query:          "  中文  ",
		Scopes:         []entity.MemoryScope{entity.MemoryScopeLongTerm},
		IncludeExpired: true,
		Page:           0,
		PageSize:       0,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, memories, 1)
	require.Equal(t, "中文", repo.lastMemoryListReq.Query)
	require.Equal(t, []entity.MemoryScope{entity.MemoryScopeLongTerm}, repo.lastMemoryListReq.Scopes)
	require.True(t, repo.lastMemoryListReq.IncludeExpired)
	require.Equal(t, int32(1), repo.lastMemoryListReq.Page)
	require.Equal(t, int32(20), repo.lastMemoryListReq.PageSize)

	updated, ok, err := svc.UpdateMemory(context.Background(), &UpdateMemoryRequest{
		ThreadID:   10,
		MemoryID:   1,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "  用户偏好简短中文回答  ",
		Metadata:   `{"source":"edited"}`,
		Score:      0.9,
		Confidence: 0.95,
		SourceType: " manual ",
		SourceID:   " memory-1 ",
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "用户偏好简短中文回答", repo.lastUpdateMemoryReq.Content)
	require.Equal(t, "manual", repo.lastUpdateMemoryReq.SourceType)
	require.Equal(t, "memory-1", repo.lastUpdateMemoryReq.SourceID)
	require.NotZero(t, repo.lastUpdateMemoryReq.UpdatedAt)
	require.Equal(t, int64(1), updated.ID)

	deleted, err := svc.DeleteMemory(context.Background(), &DeleteMemoryRequest{
		ThreadID: 10,
		MemoryID: 1,
	})
	require.NoError(t, err)
	require.True(t, deleted)
	require.Equal(t, int64(10), repo.lastDeleteMemoryReq.ThreadID)
	require.NotZero(t, repo.lastDeleteMemoryReq.DeletedAt)

	cleared, err := svc.ClearMemories(context.Background(), &ClearMemoriesRequest{
		ThreadID: 10,
		Scopes:   []entity.MemoryScope{entity.MemoryScopeLongTerm},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), cleared)
	require.Equal(t, []entity.MemoryScope{entity.MemoryScopeLongTerm}, repo.lastClearMemoriesReq.Scopes)
	require.NotZero(t, repo.lastClearMemoriesReq.DeletedAt)
	require.Len(t, repo.memoryAuditEvents, 3)
	require.Equal(t, "memory.updated", repo.memoryAuditEvents[0].EventType)
	require.Equal(t, "memory.deleted", repo.memoryAuditEvents[1].EventType)
	require.Equal(t, "memory.cleared", repo.memoryAuditEvents[2].EventType)

	restored, ok, err := svc.RestoreMemory(context.Background(), &RestoreMemoryRequest{
		ThreadID: 10,
		MemoryID: 1,
		ActorID:  7,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(1), restored.ID)
	require.Equal(t, int64(7), repo.lastRestoreMemoryReq.ActorID)
	require.Len(t, repo.memoryAuditEvents, 4)
	require.Equal(t, "memory.restored", repo.memoryAuditEvents[3].EventType)
	require.Equal(t, int64(7), repo.memoryAuditEvents[3].ActorID)
}

func TestImportMemoriesCreatesRowsSkipsDuplicateSourcesAndAudits(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	repo.sourceMemory = &entity.Memory{
		ID:         9001,
		ThreadID:   10,
		SpaceID:    1,
		Scope:      entity.MemoryScopeThread,
		Content:    "existing imported memory",
		SourceType: "import",
		SourceID:   "duplicate-source",
		CreatedAt:  100,
		UpdatedAt:  100,
	}
	repo.sourceMemoryCreated = false
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 5001}})

	result, err := svc.ImportMemories(context.Background(), &ImportMemoriesRequest{
		ThreadID: 10,
		ActorID:  7,
		Memories: []ImportMemoryItem{
			{
				Scope:      entity.MemoryScopeLongTerm,
				Content:    "  用户偏好中文摘要  ",
				Metadata:   `{"origin":"manual"}`,
				Score:      0.8,
				Confidence: 0.9,
				SourceType: " manual ",
				SourceID:   " memory-ui-1 ",
			},
			{
				RunID:      20,
				Scope:      entity.MemoryScopeRun,
				Content:    "运行中需要保留的约束",
				Confidence: 0.7,
			},
			{
				Scope:      entity.MemoryScopeThread,
				Content:    "duplicate imported memory",
				Confidence: 0.6,
				SourceType: " import ",
				SourceID:   " duplicate-source ",
			},
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), result.Imported)
	require.Equal(t, int64(1), result.Skipped)
	require.Len(t, result.Memories, 3)
	require.Len(t, repo.memories[10], 2)
	require.Equal(t, "用户偏好中文摘要", repo.memories[10][0].Content)
	require.Equal(t, entity.MemoryScopeLongTerm, repo.memories[10][0].Scope)
	require.Equal(t, int64(0), repo.memories[10][0].RunID)
	require.Equal(t, "manual", repo.memories[10][0].SourceType)
	require.Equal(t, "memory-ui-1", repo.memories[10][0].SourceID)
	require.Equal(t, entity.MemoryScopeRun, repo.memories[10][1].Scope)
	require.Equal(t, int64(20), repo.memories[10][1].RunID)
	require.Equal(t, "import", repo.lastCreateOrGetMemoryBySource.SourceType)
	require.Equal(t, "duplicate-source", repo.lastCreateOrGetMemoryBySource.SourceID)
	require.Len(t, repo.memoryAuditEvents, 1)
	require.Equal(t, "memory.imported", repo.memoryAuditEvents[0].EventType)
	require.Equal(t, int64(7), repo.memoryAuditEvents[0].ActorID)
	require.Equal(t, int64(2), repo.memoryAuditEvents[0].AffectedCount)
}

func TestManageMemoriesRejectsInvalidInput(t *testing.T) {
	repo := newMemoryRepo()
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4001}})

	_, _, err := svc.UpdateMemory(context.Background(), &UpdateMemoryRequest{
		ThreadID:   10,
		MemoryID:   1,
		Scope:      entity.MemoryScopeRun,
		Content:    "run memory missing run id",
		Confidence: 0.5,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "run id is required")

	_, _, err = svc.UpdateMemory(context.Background(), &UpdateMemoryRequest{
		ThreadID:   10,
		MemoryID:   1,
		Scope:      entity.MemoryScopeLongTerm,
		Content:    "bad confidence",
		Confidence: 1.1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "memory confidence")
}

func TestPersistTranscriptSnapshotValidatesRunAndIsIdempotent(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID:        20,
		ThreadID:  10,
		SpaceID:   1,
		CreatorID: 2,
		Status:    entity.RunStatusRunning,
	}}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4501}})
	digest := strings.Repeat("a", 64)
	req := &PersistTranscriptSnapshotRequest{
		ThreadID:       10,
		RunID:          20,
		Kind:           entity.TranscriptKindSummaryInput,
		Digest:         digest,
		IdempotencyKey: "summary_input:" + digest,
		MessageCount:   2,
		Messages:       `[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"}]`,
		Metadata:       `{"runtime":"eino_adk"}`,
	}

	snapshot, created, err := svc.PersistTranscriptSnapshot(
		context.Background(),
		req,
	)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(4501), snapshot.ID)
	require.Equal(t, int64(1), snapshot.SpaceID)
	require.Equal(t, entity.TranscriptKindSummaryInput, snapshot.Kind)
	require.Equal(t, int32(2), snapshot.MessageCount)

	snapshot, created, err = svc.PersistTranscriptSnapshot(
		context.Background(),
		req,
	)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(4501), snapshot.ID)

	bad := *req
	bad.Digest = "short"
	_, _, err = svc.PersistTranscriptSnapshot(context.Background(), &bad)
	require.ErrorContains(t, err, "digest")
}

func TestEnqueueMemoryFlushJobValidatesAndIsIdempotent(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{{
		ID:          20,
		ThreadID:    10,
		SpaceID:     1,
		CreatorID:   2,
		AssistantID: "lead",
		Status:      entity.RunStatusRunning,
	}}
	repo.transcriptSnapshots["20:summary"] = &entity.TranscriptSnapshot{
		ID:             4501,
		ThreadID:       10,
		RunID:          20,
		SpaceID:        1,
		IdempotencyKey: "summary_input:" + strings.Repeat("a", 64),
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 4601}})
	req := &EnqueueMemoryFlushJobRequest{
		ThreadID:             10,
		RunID:                20,
		TranscriptSnapshotID: 4501,
		IdempotencyKey:       "summary_input:" + strings.Repeat("a", 64),
	}

	job, created, err := svc.EnqueueMemoryFlushJob(context.Background(), req)

	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, int64(4601), job.ID)
	require.Equal(t, int64(1), job.SpaceID)
	require.Equal(t, int64(2), job.UserID)
	require.Equal(t, "lead", job.AssistantID)
	require.Equal(t, entity.MemoryFlushJobStatusPending, job.Status)
	require.Equal(t, job.CreatedAt, job.AvailableAt)

	job, created, err = svc.EnqueueMemoryFlushJob(context.Background(), req)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, int64(4601), job.ID)

	bad := *req
	bad.TranscriptSnapshotID = 0
	_, _, err = svc.EnqueueMemoryFlushJob(context.Background(), &bad)
	require.ErrorContains(t, err, "transcript snapshot id")

	bad = *req
	bad.TranscriptSnapshotID = 9999
	_, _, err = svc.EnqueueMemoryFlushJob(context.Background(), &bad)
	require.ErrorContains(t, err, "not found")

	bad = *req
	bad.IdempotencyKey = "terminal:" + strings.Repeat("b", 64)
	_, _, err = svc.EnqueueMemoryFlushJob(context.Background(), &bad)
	require.ErrorContains(t, err, "does not match transcript snapshot")

	repo.transcriptSnapshots["other-run"] = &entity.TranscriptSnapshot{
		ID:       4502,
		ThreadID: 11,
		RunID:    21,
		SpaceID:  1,
	}
	bad = *req
	bad.TranscriptSnapshotID = 4502
	_, _, err = svc.EnqueueMemoryFlushJob(context.Background(), &bad)
	require.ErrorContains(t, err, "does not belong to run")
}

func TestClaimMemoryFlushJobsNormalizesWorkerLeaseAndLimit(t *testing.T) {
	repo := newMemoryRepo()
	repo.claimedMemoryFlushJobs = []*entity.MemoryFlushJob{
		{
			ID:       4601,
			ThreadID: 10,
			RunID:    20,
			Status:   entity.MemoryFlushJobStatusProcessing,
		},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1}})

	jobs, err := svc.ClaimMemoryFlushJobs(context.Background(), &ClaimMemoryFlushJobsRequest{
		WorkerID:       " worker-a ",
		LeaseTTLMillis: 600000,
	})

	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, "worker-a", repo.lastClaimMemoryFlushReq.WorkerID)
	require.Equal(t, int32(10), repo.lastClaimMemoryFlushReq.Limit)
	require.Greater(t, repo.lastClaimMemoryFlushReq.Now, int64(0))
	require.Equal(t, repo.lastClaimMemoryFlushReq.Now+600000, repo.lastClaimMemoryFlushReq.LeaseExpiresAt)

	_, err = svc.ClaimMemoryFlushJobs(context.Background(), &ClaimMemoryFlushJobsRequest{
		WorkerID: " ",
	})
	require.ErrorContains(t, err, "worker id")
}

func TestFinishMemoryFlushJobsValidateWorkerAndDelegate(t *testing.T) {
	repo := newMemoryRepo()
	repo.completedMemoryFlushJob = &entity.MemoryFlushJob{
		ID:     4601,
		Status: entity.MemoryFlushJobStatusSucceeded,
	}
	repo.retriedMemoryFlushJob = &entity.MemoryFlushJob{
		ID:     4602,
		Status: entity.MemoryFlushJobStatusPending,
	}
	repo.failedMemoryFlushJob = &entity.MemoryFlushJob{
		ID:     4603,
		Status: entity.MemoryFlushJobStatusFailed,
	}
	repo.memoryFlushUpdated = true
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 1}})

	completed, ok, err := svc.CompleteMemoryFlushJob(context.Background(), &CompleteMemoryFlushJobRequest{
		JobID:    4601,
		WorkerID: " worker-a ",
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.MemoryFlushJobStatusSucceeded, completed.Status)
	require.Equal(t, "worker-a", repo.lastCompleteMemoryFlushReq.WorkerID)
	require.Greater(t, repo.lastCompleteMemoryFlushReq.Now, int64(0))

	retried, ok, err := svc.RetryMemoryFlushJob(context.Background(), &RetryMemoryFlushJobRequest{
		JobID:       4602,
		WorkerID:    " worker-a ",
		ErrorText:   "temporary extractor error",
		AvailableAt: 900,
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.MemoryFlushJobStatusPending, retried.Status)
	require.Equal(t, "worker-a", repo.lastRetryMemoryFlushReq.WorkerID)
	require.Equal(t, int64(900), repo.lastRetryMemoryFlushReq.AvailableAt)

	failed, ok, err := svc.FailMemoryFlushJob(context.Background(), &FailMemoryFlushJobRequest{
		JobID:     4603,
		WorkerID:  " worker-a ",
		ErrorText: "dead letter",
	})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, entity.MemoryFlushJobStatusFailed, failed.Status)
	require.Equal(t, "worker-a", repo.lastFailMemoryFlushReq.WorkerID)

	_, _, err = svc.CompleteMemoryFlushJob(context.Background(), &CompleteMemoryFlushJobRequest{
		JobID:    4601,
		WorkerID: " ",
	})
	require.ErrorContains(t, err, "worker id")
}

func TestRecordTokenUsageNormalizesAndPersistsRunUsage(t *testing.T) {
	repo := newMemoryRepo()
	repo.threads[10] = &entity.Thread{ID: 10, SpaceID: 1, CreatorID: 2}
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, SpaceID: 1, CreatorID: 2, Status: entity.RunStatusRunning},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 5001}})

	usage, err := svc.RecordTokenUsage(context.Background(), &RecordTokenUsageRequest{
		RunID:        20,
		Source:       entity.TokenUsageSourceLeadAgent,
		StepID:       "model-1",
		StepIndex:    0,
		StepName:     "generate_answer",
		ModelName:    "gpt-test",
		Provider:     "openai-compatible",
		InputTokens:  12,
		OutputTokens: 8,
		RawUsage:     `{"prompt_tokens":12,"completion_tokens":8}`,
		Metadata:     `{"phase":"service"}`,
	})

	require.NoError(t, err)
	require.Equal(t, int64(5001), usage.ID)
	require.Equal(t, int64(10), usage.ThreadID)
	require.Equal(t, int64(20), usage.RunID)
	require.Equal(t, int64(1), usage.SpaceID)
	require.Equal(t, entity.TokenUsageSourceLeadAgent, usage.Source)
	require.Equal(t, int64(20), usage.TotalTokens)
	require.Equal(t, "gpt-test", usage.ModelName)
	require.NotZero(t, usage.CreatedAt)
	require.Len(t, repo.tokenUsages, 1)
	require.Equal(t, `{"prompt_tokens":12,"completion_tokens":8}`, repo.tokenUsages[0].RawUsage)
}

func TestRecordTokenUsageRejectsInvalidSourceAndNegativeTokens(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, SpaceID: 1, CreatorID: 2, Status: entity.RunStatusRunning},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 5001}})

	_, err := svc.RecordTokenUsage(context.Background(), &RecordTokenUsageRequest{
		RunID:       20,
		Source:      entity.TokenUsageSource("unknown"),
		InputTokens: 1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "token usage source is invalid")

	_, err = svc.RecordTokenUsage(context.Background(), &RecordTokenUsageRequest{
		RunID:       20,
		Source:      entity.TokenUsageSourceLeadAgent,
		InputTokens: -1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "tokens cannot be negative")
}

func TestGetRunTokenUsageReturnsRowsAndAggregate(t *testing.T) {
	repo := newMemoryRepo()
	repo.tokenUsages = []*entity.TokenUsage{
		{ID: 1, ThreadID: 10, RunID: 20, Source: entity.TokenUsageSourceLeadAgent, InputTokens: 12, OutputTokens: 8, TotalTokens: 20},
		{ID: 2, ThreadID: 10, RunID: 20, Source: entity.TokenUsageSourceTool, InputTokens: 4, OutputTokens: 6, TotalTokens: 10},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 5001}})

	rows, total, aggregate, runAggregates, err := svc.GetRunTokenUsage(context.Background(), &GetRunTokenUsageRequest{
		RunID:    20,
		Page:     0,
		PageSize: 0,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	require.Equal(t, int64(20), repo.lastTokenUsageListReq.RunID)
	require.Equal(t, int32(1), repo.lastTokenUsageListReq.Page)
	require.Equal(t, int32(100), repo.lastTokenUsageListReq.PageSize)
	require.Equal(t, int64(30), aggregate.TotalTokens)
	require.Equal(t, int64(20), aggregate.LeadAgentTokens)
	require.Equal(t, int64(10), aggregate.ToolTokens)
	require.Empty(t, runAggregates)
}

func TestGetRunTokenUsageCanIncludeDirectChildRuns(t *testing.T) {
	repo := newMemoryRepo()
	repo.runs[10] = []*entity.Run{
		{ID: 20, ThreadID: 10, RunKind: entity.RunKindTask},
		{ID: 21, ThreadID: 10, ParentRunID: 20, RunKind: entity.RunKindSubagent},
		{ID: 22, ThreadID: 10, RunKind: entity.RunKindTask},
	}
	repo.tokenUsages = []*entity.TokenUsage{
		{ID: 1, ThreadID: 10, RunID: 20, Source: entity.TokenUsageSourceLeadAgent, TotalTokens: 20},
		{ID: 2, ThreadID: 10, RunID: 21, Source: entity.TokenUsageSourceSubagent, TotalTokens: 10},
		{ID: 3, ThreadID: 10, RunID: 22, Source: entity.TokenUsageSourceLeadAgent, TotalTokens: 99},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 5001}})

	rows, total, aggregate, runAggregates, err := svc.GetRunTokenUsage(context.Background(), &GetRunTokenUsageRequest{
		RunID:            20,
		IncludeChildRuns: true,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	require.Equal(t, []int64{20, 21}, repo.lastTokenUsageListReq.RunIDs)
	require.Equal(t, []int64{20, 21}, repo.lastTokenUsageAggregateReq.RunIDs)
	require.Equal(t, int64(30), aggregate.TotalTokens)
	require.Equal(t, int64(20), aggregate.LeadAgentTokens)
	require.Equal(t, int64(10), aggregate.SubagentTokens)
	require.Len(t, runAggregates, 2)
	require.Equal(t, int64(20), runAggregates[0].RunID)
	require.Equal(t, int64(20), runAggregates[0].Aggregate.TotalTokens)
	require.Equal(t, int64(21), runAggregates[1].RunID)
	require.Equal(t, int64(10), runAggregates[1].Aggregate.TotalTokens)
}

func TestGetThreadTokenUsageReturnsAggregate(t *testing.T) {
	repo := newMemoryRepo()
	repo.tokenUsages = []*entity.TokenUsage{
		{ID: 1, ThreadID: 10, RunID: 20, Source: entity.TokenUsageSourceLeadAgent, TotalTokens: 20},
		{ID: 2, ThreadID: 10, RunID: 21, Source: entity.TokenUsageSourceMiddleware, TotalTokens: 5},
		{ID: 3, ThreadID: 11, RunID: 22, Source: entity.TokenUsageSourceLeadAgent, TotalTokens: 99},
	}
	svc := NewService(&Components{Repo: repo, IDGen: fixedIDGen{next: 5001}})

	rows, total, aggregate, err := svc.GetThreadTokenUsage(context.Background(), &GetThreadTokenUsageRequest{
		ThreadID: 10,
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, rows, 2)
	require.Equal(t, int64(10), repo.lastTokenUsageListReq.ThreadID)
	require.Equal(t, int64(25), aggregate.TotalTokens)
	require.Equal(t, int64(5), aggregate.MiddlewareTokens)
}

type memoryRepo struct {
	repository.JournalExecutionRepository
	mu                                 sync.Mutex
	threads                            map[int64]*entity.Thread
	messages                           map[int64][]*entity.Message
	runs                               map[int64][]*entity.Run
	runEvents                          map[int64][]*entity.RunEvent
	runAttempts                        map[int64][]*entity.RunAttempt
	journalEvents                      map[int64]*entity.JournalEvent
	lastJournalAttempt                 *entity.RunAttempt
	lastJournalEvent                   *entity.JournalEvent
	lastCreateRunBundleReq             *repository.CreateRunBundleRequest
	lastFinalizeJournalReq             repository.FinalizeJournalAttemptRequest
	checkpoints                        map[int64][]*entity.Checkpoint
	memories                           map[int64][]*entity.Memory
	sourceMemory                       *entity.Memory
	sourceMemoryCreated                bool
	updatedMemory                      *entity.Memory
	restoredMemory                     *entity.Memory
	deleteMemoryOK                     bool
	clearMemoryCount                   int64
	memoryAuditEvents                  []*entity.MemoryAuditEvent
	transcriptSnapshots                map[string]*entity.TranscriptSnapshot
	memoryFlushJobs                    map[string]*entity.MemoryFlushJob
	claimedMemoryFlushJobs             []*entity.MemoryFlushJob
	memoryFlushBacklogAggregates       []*entity.MemoryFlushBacklogAggregate
	completedMemoryFlushJob            *entity.MemoryFlushJob
	retriedMemoryFlushJob              *entity.MemoryFlushJob
	failedMemoryFlushJob               *entity.MemoryFlushJob
	memoryFlushUpdated                 bool
	tokenUsages                        []*entity.TokenUsage
	lastListReq                        repository.ListThreadsRequest
	lastUpdateThreadTitleReq           repository.UpdateThreadTitleRequest
	lastUpdateThreadMetadataReq        repository.UpdateThreadMetadataRequest
	lastDeleteThreadReq                repository.DeleteThreadRequest
	lastMessageListReq                 repository.ListMessagesRequest
	lastRecentMessagesByRolesReq       repository.ListRecentMessagesByRolesRequest
	lastRunListReq                     repository.ListRunsRequest
	lastRunEventListReq                repository.ListRunEventsRequest
	lastCheckpointListReq              repository.ListCheckpointsRequest
	lastCheckpointID                   int64
	lastMemoryListReq                  repository.ListMemoriesRequest
	lastUpdateMemoryReq                repository.UpdateMemoryRequest
	lastDeleteMemoryReq                repository.DeleteMemoryRequest
	lastRestoreMemoryReq               repository.RestoreMemoryRequest
	lastClearMemoriesReq               repository.ClearMemoriesRequest
	lastMemoryAuditListReq             repository.ListMemoryAuditEventsRequest
	lastCreateOrGetMemoryBySource      *entity.Memory
	lastClaimMemoryFlushReq            repository.ClaimMemoryFlushJobsRequest
	lastAggregateMemoryFlushBacklogReq repository.AggregateMemoryFlushBacklogRequest
	lastCompleteMemoryFlushReq         repository.CompleteMemoryFlushJobRequest
	lastRetryMemoryFlushReq            repository.RetryMemoryFlushJobRequest
	lastFailMemoryFlushReq             repository.FailMemoryFlushJobRequest
	lastTokenUsageListReq              repository.ListTokenUsageRequest
	lastTokenUsageAggregateReq         repository.AggregateTokenUsageRequest
	lastClaimReq                       repository.ClaimPendingRunsRequest
	lastAggregateRunBacklogReq         repository.AggregateRunBacklogRequest
	lastClaimQueuedResumeReq           repository.ClaimQueuedResumeRunsRequest
	lastRenewRunLeaseReq               repository.RenewRunLeaseRequest
	lastReleaseRunLeaseReq             repository.ReleaseRunLeaseRequest
	lastListExpiredRunLeasesReq        repository.ListExpiredRunLeasesRequest
	lastReconcileExpiredRunLeaseReq    repository.ReconcileExpiredRunLeaseRequest
	lastRequestRunCancellationReq      repository.RequestRunCancellationRequest
	lastFinalizeRunSuccessReq          repository.FinalizeRunSuccessRequest
	lastUpdateRunReq                   repository.UpdateRunStatusRequest
	createThreadBundleCalls            int
	createRunCalls                     int
	createRunWithThreadLockCalls       int
	createRunBundleCalls               int
}

var _ repository.Repository = (*memoryRepo)(nil)

var errMemoryJournalSnapshotUnsupported = errors.New("memory journal snapshot repository is not configured")

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		threads:             make(map[int64]*entity.Thread),
		messages:            make(map[int64][]*entity.Message),
		runs:                make(map[int64][]*entity.Run),
		runEvents:           make(map[int64][]*entity.RunEvent),
		runAttempts:         make(map[int64][]*entity.RunAttempt),
		journalEvents:       make(map[int64]*entity.JournalEvent),
		checkpoints:         make(map[int64][]*entity.Checkpoint),
		memories:            make(map[int64][]*entity.Memory),
		transcriptSnapshots: make(map[string]*entity.TranscriptSnapshot),
		memoryFlushJobs:     make(map[string]*entity.MemoryFlushJob),
	}
}

func (r *memoryRepo) CreateThread(ctx context.Context, thread *entity.Thread) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.threads[thread.ID]; ok {
		return fmt.Errorf("thread %d already exists", thread.ID)
	}
	r.threads[thread.ID] = cloneThread(thread)
	return nil
}

func (r *memoryRepo) CreateThreadBundle(
	ctx context.Context,
	req repository.CreateThreadBundleRequest,
) (*repository.CreateThreadBundleResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createThreadBundleCalls++
	if req.Thread == nil || req.Run == nil || req.Message == nil {
		return nil, fmt.Errorf("incomplete thread bundle")
	}
	if req.Run.IdempotencyKey != "" {
		for threadID, runs := range r.runs {
			for _, run := range runs {
				if run.SpaceID != req.Run.SpaceID || run.IdempotencyKey != req.Run.IdempotencyKey {
					continue
				}
				if len(r.messages[threadID]) == 0 {
					return nil, fmt.Errorf("idempotent thread bundle is missing message")
				}
				return &repository.CreateThreadBundleResult{
					Thread: cloneThread(r.threads[threadID]), Run: cloneRun(run),
					Message: cloneMessage(r.messages[threadID][0]),
					Attempt: cloneRunAttemptForServiceTest(firstRunAttempt(r.runAttempts[run.ID])),
				}, nil
			}
		}
	}
	if _, ok := r.threads[req.Thread.ID]; ok {
		return nil, fmt.Errorf("thread %d already exists", req.Thread.ID)
	}
	r.threads[req.Thread.ID] = cloneThread(req.Thread)
	r.runs[req.Thread.ID] = append(r.runs[req.Thread.ID], cloneRun(req.Run))
	r.messages[req.Thread.ID] = append(r.messages[req.Thread.ID], cloneMessage(req.Message))
	if req.Attempt != nil {
		r.runAttempts[req.Run.ID] = append(
			r.runAttempts[req.Run.ID],
			cloneRunAttemptForServiceTest(req.Attempt),
		)
	}
	return &repository.CreateThreadBundleResult{
		Thread: cloneThread(req.Thread), Run: cloneRun(req.Run),
		Message: cloneMessage(req.Message), Attempt: cloneRunAttemptForServiceTest(req.Attempt), Created: true,
	}, nil
}

func firstRunAttempt(attempts []*entity.RunAttempt) *entity.RunAttempt {
	if len(attempts) == 0 {
		return nil
	}
	return attempts[0]
}

func (r *memoryRepo) GetThread(ctx context.Context, id int64) (*entity.Thread, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	thread, ok := r.threads[id]
	if !ok {
		return nil, fmt.Errorf("thread %d not found", id)
	}
	return cloneThread(thread), nil
}

func (r *memoryRepo) UpdateThreadTitle(
	ctx context.Context,
	req repository.UpdateThreadTitleRequest,
) (*entity.Thread, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastUpdateThreadTitleReq = req
	thread, ok := r.threads[req.ThreadID]
	if !ok {
		return nil, false, nil
	}
	cloned := cloneThread(thread)
	cloned.Title = strings.TrimSpace(req.Title)
	cloned.UpdatedAt = req.UpdatedAt
	r.threads[req.ThreadID] = cloned
	return cloneThread(cloned), true, nil
}

func (r *memoryRepo) UpdateThreadMetadata(
	ctx context.Context,
	req repository.UpdateThreadMetadataRequest,
) (*entity.Thread, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastUpdateThreadMetadataReq = req
	thread, ok := r.threads[req.ThreadID]
	if !ok {
		return nil, false, nil
	}
	cloned := cloneThread(thread)
	cloned.Metadata = strings.TrimSpace(req.Metadata)
	cloned.UpdatedAt = req.UpdatedAt
	r.threads[req.ThreadID] = cloned
	return cloneThread(cloned), true, nil
}

func (r *memoryRepo) DeleteThread(
	ctx context.Context,
	req repository.DeleteThreadRequest,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastDeleteThreadReq = req
	if _, ok := r.threads[req.ThreadID]; !ok {
		return false, nil
	}
	delete(r.threads, req.ThreadID)
	delete(r.messages, req.ThreadID)
	delete(r.runs, req.ThreadID)
	delete(r.runEvents, req.ThreadID)
	delete(r.checkpoints, req.ThreadID)
	delete(r.memories, req.ThreadID)
	return true, nil
}

func (r *memoryRepo) ListThreads(ctx context.Context, req repository.ListThreadsRequest) ([]*entity.Thread, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastListReq = req

	threads := make([]*entity.Thread, 0, len(r.threads))
	for _, thread := range r.threads {
		if thread.SpaceID != req.SpaceID {
			continue
		}
		if req.UserID > 0 && thread.CreatorID != req.UserID {
			continue
		}
		if req.Status != nil && thread.Status != *req.Status {
			continue
		}
		threads = append(threads, cloneThread(thread))
	}

	sort.Slice(threads, func(i, j int) bool {
		if threads[i].UpdatedAt == threads[j].UpdatedAt {
			return threads[i].ID > threads[j].ID
		}
		return threads[i].UpdatedAt > threads[j].UpdatedAt
	})

	total := int64(len(threads))
	start := int((req.Page - 1) * req.PageSize)
	if start >= len(threads) {
		return []*entity.Thread{}, total, nil
	}
	end := start + int(req.PageSize)
	if end > len(threads) {
		end = len(threads)
	}
	return threads[start:end], total, nil
}

func (r *memoryRepo) CreateMessage(ctx context.Context, message *entity.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages[message.ThreadID] = append(r.messages[message.ThreadID], cloneMessage(message))
	return nil
}

func (r *memoryRepo) ListMessages(ctx context.Context, req repository.ListMessagesRequest) ([]*entity.Message, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastMessageListReq = req

	messages := make([]*entity.Message, 0, len(r.messages[req.ThreadID]))
	for _, message := range r.messages[req.ThreadID] {
		messages = append(messages, cloneMessage(message))
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt == messages[j].CreatedAt {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].CreatedAt < messages[j].CreatedAt
	})

	total := int64(len(messages))
	start := int((req.Page - 1) * req.PageSize)
	if start >= len(messages) {
		return []*entity.Message{}, total, nil
	}
	end := start + int(req.PageSize)
	if end > len(messages) {
		end = len(messages)
	}
	return messages[start:end], total, nil
}

func (r *memoryRepo) ListRecentMessagesByRoles(
	ctx context.Context,
	req repository.ListRecentMessagesByRolesRequest,
) ([]*entity.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRecentMessagesByRolesReq = req

	roleSet := make(map[entity.MessageRole]bool, len(req.Roles))
	for _, role := range req.Roles {
		roleSet[role] = true
	}
	messages := make([]*entity.Message, 0, len(r.messages[req.ThreadID]))
	for _, message := range r.messages[req.ThreadID] {
		if !roleSet[message.Role] {
			continue
		}
		messages = append(messages, cloneMessage(message))
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].CreatedAt == messages[j].CreatedAt {
			return messages[i].ID > messages[j].ID
		}
		return messages[i].CreatedAt > messages[j].CreatedAt
	})
	if req.Limit > 0 && len(messages) > int(req.Limit) {
		messages = messages[:int(req.Limit)]
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, nil
}

func (r *memoryRepo) CreateRun(ctx context.Context, run *entity.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createRunCalls++
	r.runs[run.ThreadID] = append(r.runs[run.ThreadID], cloneRun(run))
	return nil
}

func (r *memoryRepo) CreateRunWithThreadLock(ctx context.Context, run *entity.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createRunWithThreadLockCalls++
	r.runs[run.ThreadID] = append(r.runs[run.ThreadID], cloneRun(run))
	return nil
}

func (r *memoryRepo) CreateRunBundle(
	ctx context.Context,
	req repository.CreateRunBundleRequest,
) (*repository.CreateRunBundleResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createRunBundleCalls++
	requestCopy := req
	r.lastCreateRunBundleReq = &requestCopy
	if req.Run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if req.Run.IdempotencyKey != "" {
		for _, runs := range r.runs {
			for _, run := range runs {
				if run.SpaceID != req.Run.SpaceID || run.IdempotencyKey != req.Run.IdempotencyKey {
					continue
				}
				result := &repository.CreateRunBundleResult{Run: cloneRun(run)}
				existingAttempts := r.runAttempts[run.ID]
				if req.Attempt == nil && len(existingAttempts) > 0 {
					return nil, fmt.Errorf("%w: journal enrollment changed", repository.ErrRunIdempotencyConflict)
				}
				if req.Attempt != nil {
					if len(existingAttempts) == 0 {
						return nil, fmt.Errorf("%w: journal attempt is missing", repository.ErrRunIdempotencyConflict)
					}
					existingAttempt := existingAttempts[0]
					if existingAttempt.EnrollmentVersion != req.Attempt.EnrollmentVersion ||
						existingAttempt.SnapshotsEnabled != req.Attempt.SnapshotsEnabled {
						return nil, fmt.Errorf("%w: journal enrollment semantics changed", repository.ErrRunIdempotencyConflict)
					}
					result.Attempt = cloneRunAttemptForServiceTest(existingAttempt)
				}
				if req.Message != nil {
					for _, message := range r.messages[run.ThreadID] {
						if message.RunID == run.ID && message.Role == req.Message.Role {
							result.Message = cloneMessage(message)
							break
						}
					}
					if result.Message == nil {
						return nil, fmt.Errorf("idempotent run bundle is missing message")
					}
				}
				if req.Event != nil {
					for _, event := range r.runEvents[run.ID] {
						if event.EventType == req.Event.EventType {
							result.Event = cloneRunEvent(event)
							break
						}
					}
					if result.Event == nil {
						return nil, fmt.Errorf("idempotent run bundle is missing event")
					}
				}
				return result, nil
			}
		}
	}
	if req.Attempt != nil && req.Attempt.RecoveryIdempotencyKey == nil &&
		req.Attempt.EnrollmentVersion != entity.JournalSchemaVersion {
		return nil, repository.ErrUnsupportedJournalEnrollmentVersion
	}

	r.runs[req.Run.ThreadID] = append(r.runs[req.Run.ThreadID], cloneRun(req.Run))
	if req.Message != nil {
		r.messages[req.Run.ThreadID] = append(r.messages[req.Run.ThreadID], cloneMessage(req.Message))
	}
	if req.Event != nil {
		r.runEvents[req.Run.ID] = append(r.runEvents[req.Run.ID], cloneRunEvent(req.Event))
	}
	if req.Attempt != nil {
		r.runAttempts[req.Run.ID] = append(r.runAttempts[req.Run.ID], cloneRunAttemptForServiceTest(req.Attempt))
	}
	return &repository.CreateRunBundleResult{
		Run: cloneRun(req.Run), Message: cloneMessage(req.Message), Event: cloneRunEvent(req.Event),
		Attempt: cloneRunAttemptForServiceTest(req.Attempt), Created: true,
	}, nil
}

func cloneRunAttemptForServiceTest(attempt *entity.RunAttempt) *entity.RunAttempt {
	if attempt == nil {
		return nil
	}
	cloned := *attempt
	return &cloned
}

func (r *memoryRepo) CreateJournalAttempt(
	ctx context.Context,
	attempt *entity.RunAttempt,
) (*entity.RunAttempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastJournalAttempt = cloneRunAttemptForServiceTest(attempt)
	r.runAttempts[attempt.JournalRunID] = append(r.runAttempts[attempt.JournalRunID], cloneRunAttemptForServiceTest(attempt))
	return cloneRunAttemptForServiceTest(attempt), nil
}

func (r *memoryRepo) GetActiveJournalAttempt(
	ctx context.Context,
	runID int64,
) (*entity.RunAttempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempts := r.runAttempts[runID]
	if len(attempts) == 0 {
		return nil, repository.ErrJournalNotEnrolled
	}
	return cloneRunAttemptForServiceTest(attempts[len(attempts)-1]), nil
}

func (r *memoryRepo) ListJournalAttempts(
	ctx context.Context,
	runID int64,
) ([]*entity.RunAttempt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	attempts := r.runAttempts[runID]
	if len(attempts) == 0 {
		return nil, repository.ErrJournalNotEnrolled
	}
	result := make([]*entity.RunAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		result = append(result, cloneRunAttemptForServiceTest(attempt))
	}
	return result, nil
}

func (r *memoryRepo) AppendJournalEvent(
	ctx context.Context,
	event *entity.JournalEvent,
) (*entity.JournalEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cloned := cloneJournalEventForServiceTest(event)
	r.lastJournalEvent = cloned
	r.journalEvents[event.ID] = cloned
	return cloneJournalEventForServiceTest(cloned), nil
}

func (r *memoryRepo) FinalizeJournalAttempt(
	ctx context.Context,
	req repository.FinalizeJournalAttemptRequest,
) (*entity.JournalEvent, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastFinalizeJournalReq = req
	cloned := cloneJournalEventForServiceTest(req.Event)
	r.journalEvents[cloned.ID] = cloned
	return cloneJournalEventForServiceTest(cloned), true, nil
}

func (r *memoryRepo) GetJournalEvent(
	ctx context.Context,
	eventID int64,
) (*entity.JournalEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	event, ok := r.journalEvents[eventID]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	return cloneJournalEventForServiceTest(event), nil
}

func (r *memoryRepo) ListJournalEvents(
	ctx context.Context,
	req repository.ListJournalEventsRequest,
) (*repository.ListJournalEventsResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]*entity.JournalEvent, 0)
	for _, event := range r.journalEvents {
		if event.JournalRunID != req.RunID || event.AttemptID != req.AttemptID ||
			event.Visibility == entity.JournalVisibilityInternal || event.Sequence <= req.AfterSequence {
			continue
		}
		events = append(events, cloneJournalEventForServiceTest(event))
	}
	sort.Slice(events, func(i, j int) bool { return events[i].Sequence < events[j].Sequence })
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}
	return &repository.ListJournalEventsResult{Events: events, HasMore: hasMore}, nil
}

func (r *memoryRepo) GetJournalBootstrap(
	_ context.Context,
	_ repository.GetJournalBootstrapRequest,
) (*repository.GetJournalBootstrapResult, error) {
	return nil, errMemoryJournalSnapshotUnsupported
}

func (r *memoryRepo) ReserveJournalSnapshot(
	_ context.Context,
	_ repository.ReserveJournalSnapshotRequest,
) (*repository.ReserveJournalSnapshotResult, error) {
	return nil, errMemoryJournalSnapshotUnsupported
}

func (r *memoryRepo) DeleteExpiredJournalSnapshotReservations(
	_ context.Context,
	_ int64,
	_ int64,
	_ int,
) (int64, error) {
	return 0, errMemoryJournalSnapshotUnsupported
}

func (r *memoryRepo) CreateJournalSnapshot(
	_ context.Context,
	_ repository.CreateJournalSnapshotRequest,
) (*entity.JournalContentSnapshot, *entity.JournalEvent, bool, error) {
	return nil, nil, false, errMemoryJournalSnapshotUnsupported
}

func (r *memoryRepo) GetJournalSnapshot(
	_ context.Context,
	_ repository.GetJournalSnapshotRequest,
) (*entity.JournalContentSnapshot, error) {
	return nil, errMemoryJournalSnapshotUnsupported
}

func (r *memoryRepo) ListJournalSnapshotFragments(
	_ context.Context,
	_ repository.ListJournalSnapshotFragmentsRequest,
) (*repository.ListJournalSnapshotFragmentsResult, error) {
	return nil, errMemoryJournalSnapshotUnsupported
}

func (r *memoryRepo) RecordJournalSnapshotAccess(
	_ context.Context,
	_ *entity.JournalSnapshotAccessAudit,
) (*entity.JournalSnapshotAccessAudit, bool, error) {
	return nil, false, errMemoryJournalSnapshotUnsupported
}

func (r *memoryRepo) IsJournalSnapshotObjectProtected(
	_ context.Context,
	_ int64,
	_ string,
	_ int64,
) (bool, error) {
	return false, errMemoryJournalSnapshotUnsupported
}

func cloneJournalEventForServiceTest(event *entity.JournalEvent) *entity.JournalEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	return &cloned
}

func (r *memoryRepo) GetRun(ctx context.Context, id int64) (*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.ID == id {
				return cloneRun(run), nil
			}
		}
	}

	return nil, fmt.Errorf("run %d not found", id)
}

func (r *memoryRepo) GetRunByIdempotencyKey(
	ctx context.Context,
	spaceID int64,
	idempotencyKey string,
) (*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.SpaceID == spaceID && run.IdempotencyKey == idempotencyKey {
				return cloneRun(run), nil
			}
		}
	}

	return nil, nil
}

func (r *memoryRepo) ListRuns(ctx context.Context, req repository.ListRunsRequest) ([]*entity.Run, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRunListReq = req

	runs := make([]*entity.Run, 0, len(r.runs[req.ThreadID]))
	for _, run := range r.runs[req.ThreadID] {
		if req.ParentRunID != nil {
			if run.ParentRunID != *req.ParentRunID {
				continue
			}
		} else if !req.IncludeChildRuns && run.ParentRunID != 0 {
			continue
		}
		if req.Status != nil && run.Status != *req.Status {
			continue
		}
		runs = append(runs, cloneRun(run))
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CreatedAt == runs[j].CreatedAt {
			return runs[i].ID > runs[j].ID
		}
		return runs[i].CreatedAt > runs[j].CreatedAt
	})

	total := int64(len(runs))
	start := int((req.Page - 1) * req.PageSize)
	if start >= len(runs) {
		return []*entity.Run{}, total, nil
	}
	end := start + int(req.PageSize)
	if end > len(runs) {
		end = len(runs)
	}
	return runs[start:end], total, nil
}

func (r *memoryRepo) AggregateRunBacklog(
	ctx context.Context,
	req repository.AggregateRunBacklogRequest,
) ([]*entity.RunBacklogAggregate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastAggregateRunBacklogReq = req

	statuses := req.Statuses
	if len(statuses) == 0 {
		statuses = defaultRunBacklogStatuses()
	}
	allowed := make(map[entity.RunStatus]struct{}, len(statuses))
	for _, status := range statuses {
		if status != "" {
			allowed[status] = struct{}{}
		}
	}
	type aggregateKey struct {
		status entity.RunStatus
		config string
	}
	counts := map[aggregateKey]int64{}
	for _, runs := range r.runs {
		for _, run := range runs {
			if run == nil {
				continue
			}
			if _, ok := allowed[run.Status]; !ok {
				continue
			}
			counts[aggregateKey{status: run.Status, config: run.Config}]++
		}
	}
	aggregates := make([]*entity.RunBacklogAggregate, 0, len(counts))
	for key, count := range counts {
		aggregates = append(aggregates, &entity.RunBacklogAggregate{
			Status: key.status,
			Config: key.config,
			Count:  count,
		})
	}
	sort.Slice(aggregates, func(i, j int) bool {
		if aggregates[i].Status == aggregates[j].Status {
			return aggregates[i].Config < aggregates[j].Config
		}
		return aggregates[i].Status < aggregates[j].Status
	})

	return aggregates, nil
}

func (r *memoryRepo) CreateRunEvent(ctx context.Context, event *entity.RunEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runEvents[event.RunID] = append(r.runEvents[event.RunID], cloneRunEvent(event))
	return nil
}

func (r *memoryRepo) ListRunEvents(ctx context.Context, req repository.ListRunEventsRequest) ([]*entity.RunEvent, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRunEventListReq = req

	events := make([]*entity.RunEvent, 0)
	if req.RunID > 0 {
		for _, event := range r.runEvents[req.RunID] {
			events = append(events, cloneRunEvent(event))
		}
	} else {
		for _, runEvents := range r.runEvents {
			for _, event := range runEvents {
				if event.ThreadID == req.ThreadID {
					events = append(events, cloneRunEvent(event))
				}
			}
		}
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].CreatedAt == events[j].CreatedAt {
			return events[i].ID < events[j].ID
		}
		return events[i].CreatedAt < events[j].CreatedAt
	})

	total := int64(len(events))
	start := int((req.Page - 1) * req.PageSize)
	if start >= len(events) {
		return []*entity.RunEvent{}, total, nil
	}
	end := start + int(req.PageSize)
	if end > len(events) {
		end = len(events)
	}
	return events[start:end], total, nil
}

func (r *memoryRepo) CreateCheckpoint(ctx context.Context, checkpoint *entity.Checkpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checkpoints[checkpoint.ThreadID] = append(r.checkpoints[checkpoint.ThreadID], cloneCheckpoint(checkpoint))
	return nil
}

func (r *memoryRepo) GetCheckpoint(ctx context.Context, checkpointID int64) (*entity.Checkpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastCheckpointID = checkpointID

	for _, threadCheckpoints := range r.checkpoints {
		for _, checkpoint := range threadCheckpoints {
			if checkpoint.ID == checkpointID {
				return cloneCheckpoint(checkpoint), nil
			}
		}
	}

	return nil, fmt.Errorf("checkpoint %d not found", checkpointID)
}

func (r *memoryRepo) ListCheckpoints(ctx context.Context, req repository.ListCheckpointsRequest) ([]*entity.Checkpoint, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastCheckpointListReq = req

	checkpoints := make([]*entity.Checkpoint, 0, len(r.checkpoints[req.ThreadID]))
	for _, checkpoint := range r.checkpoints[req.ThreadID] {
		if req.RunID > 0 && checkpoint.RunID != req.RunID {
			continue
		}
		if req.RuntimeType != "" && checkpoint.RuntimeType != req.RuntimeType {
			continue
		}
		checkpoints = append(checkpoints, cloneCheckpoint(checkpoint))
	}
	sort.Slice(checkpoints, func(i, j int) bool {
		if checkpoints[i].CreatedAt == checkpoints[j].CreatedAt {
			return checkpoints[i].ID > checkpoints[j].ID
		}
		return checkpoints[i].CreatedAt > checkpoints[j].CreatedAt
	})

	total := int64(len(checkpoints))
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if len(checkpoints) > int(limit) {
		checkpoints = checkpoints[:limit]
	}

	return checkpoints, total, nil
}

func (r *memoryRepo) GetLatestCheckpoint(ctx context.Context, threadID int64) (*entity.Checkpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var latest *entity.Checkpoint
	for _, checkpoint := range r.checkpoints[threadID] {
		if latest == nil ||
			checkpoint.CreatedAt > latest.CreatedAt ||
			(checkpoint.CreatedAt == latest.CreatedAt && checkpoint.ID > latest.ID) {
			latest = checkpoint
		}
	}
	if latest == nil {
		return nil, fmt.Errorf("checkpoint for thread %d not found", threadID)
	}

	return cloneCheckpoint(latest), nil
}

func (r *memoryRepo) GetLatestRuntimeCheckpoint(
	ctx context.Context,
	threadID, runID int64,
	runtimeType, runtimeKey string,
) (*entity.Checkpoint, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var latest *entity.Checkpoint
	for _, checkpoint := range r.checkpoints[threadID] {
		if checkpoint.RunID != runID ||
			checkpoint.RuntimeType != runtimeType ||
			checkpoint.RuntimeKey != runtimeKey ||
			checkpoint.RuntimeDeletedAt != 0 {
			continue
		}
		if latest == nil ||
			checkpoint.CreatedAt > latest.CreatedAt ||
			(checkpoint.CreatedAt == latest.CreatedAt && checkpoint.ID > latest.ID) {
			latest = checkpoint
		}
	}
	if latest == nil {
		return nil, nil
	}

	return cloneCheckpoint(latest), nil
}

func (r *memoryRepo) DeleteRuntimeCheckpoint(
	ctx context.Context,
	threadID, runID int64,
	runtimeType, runtimeKey string,
	deletedAt int64,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, checkpoint := range r.checkpoints[threadID] {
		if checkpoint.RunID == runID &&
			checkpoint.RuntimeType == runtimeType &&
			checkpoint.RuntimeKey == runtimeKey &&
			checkpoint.RuntimeDeletedAt == 0 {
			checkpoint.RuntimeDeletedAt = deletedAt
		}
	}

	return nil
}

func (r *memoryRepo) CreateMemory(ctx context.Context, memory *entity.Memory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.memories[memory.ThreadID] = append(r.memories[memory.ThreadID], cloneMemory(memory))
	return nil
}

func (r *memoryRepo) CreateOrGetMemoryBySource(
	ctx context.Context,
	memory *entity.Memory,
) (*entity.Memory, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastCreateOrGetMemoryBySource = cloneMemory(memory)
	if r.sourceMemory != nil &&
		r.sourceMemory.SourceType == memory.SourceType &&
		r.sourceMemory.SourceID == memory.SourceID {
		return cloneMemory(r.sourceMemory), r.sourceMemoryCreated, nil
	}
	r.memories[memory.ThreadID] = append(r.memories[memory.ThreadID], cloneMemory(memory))
	return cloneMemory(memory), true, nil
}

func (r *memoryRepo) ListMemories(ctx context.Context, req repository.ListMemoriesRequest) ([]*entity.Memory, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastMemoryListReq = req

	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	memories := make([]*entity.Memory, 0, len(r.memories[req.ThreadID]))
	for _, memory := range r.memories[req.ThreadID] {
		if !req.IncludeDeleted && memory.DeletedAt > 0 {
			continue
		}
		if req.RunID > 0 {
			if memory.RunID != 0 && memory.RunID != req.RunID {
				continue
			}
		} else if memory.RunID != 0 {
			continue
		}
		if !req.IncludeExpired && memory.ExpiresAt > 0 && memory.ExpiresAt <= now {
			continue
		}
		if len(req.Scopes) > 0 {
			matched := false
			for _, scope := range req.Scopes {
				if scope == memory.Scope {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		if req.Query != "" && !strings.Contains(memory.Content, req.Query) {
			continue
		}
		memories = append(memories, cloneMemory(memory))
	}
	sort.Slice(memories, func(i, j int) bool {
		if memories[i].Score == memories[j].Score {
			if memories[i].UpdatedAt == memories[j].UpdatedAt {
				return memories[i].ID > memories[j].ID
			}
			return memories[i].UpdatedAt > memories[j].UpdatedAt
		}
		return memories[i].Score > memories[j].Score
	})

	total := int64(len(memories))
	limit := req.Limit
	if limit <= 0 {
		limit = 8
	}
	if len(memories) > int(limit) {
		memories = memories[:limit]
	}
	if req.Page > 0 && req.PageSize > 0 {
		start := int((req.Page - 1) * req.PageSize)
		if start >= len(memories) {
			return []*entity.Memory{}, total, nil
		}
		end := start + int(req.PageSize)
		if end > len(memories) {
			end = len(memories)
		}
		memories = memories[start:end]
	}

	return memories, total, nil
}

func (r *memoryRepo) UpdateMemory(
	ctx context.Context,
	req repository.UpdateMemoryRequest,
) (*entity.Memory, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastUpdateMemoryReq = req
	if r.updatedMemory != nil {
		return cloneMemory(r.updatedMemory), true, nil
	}
	return nil, false, nil
}

func (r *memoryRepo) DeleteMemory(
	ctx context.Context,
	req repository.DeleteMemoryRequest,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastDeleteMemoryReq = req
	return r.deleteMemoryOK, nil
}

func (r *memoryRepo) RestoreMemory(
	ctx context.Context,
	req repository.RestoreMemoryRequest,
) (*entity.Memory, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRestoreMemoryReq = req
	if r.restoredMemory != nil {
		return cloneMemory(r.restoredMemory), true, nil
	}
	return nil, false, nil
}

func (r *memoryRepo) ClearMemories(
	ctx context.Context,
	req repository.ClearMemoriesRequest,
) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastClearMemoriesReq = req
	return r.clearMemoryCount, nil
}

func (r *memoryRepo) CreateMemoryAuditEvent(ctx context.Context, event *entity.MemoryAuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.memoryAuditEvents = append(r.memoryAuditEvents, cloneMemoryAuditEvent(event))
	return nil
}

func (r *memoryRepo) ListMemoryAuditEvents(
	ctx context.Context,
	req repository.ListMemoryAuditEventsRequest,
) ([]*entity.MemoryAuditEvent, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastMemoryAuditListReq = req
	events := make([]*entity.MemoryAuditEvent, 0, len(r.memoryAuditEvents))
	for _, event := range r.memoryAuditEvents {
		events = append(events, cloneMemoryAuditEvent(event))
	}
	return events, int64(len(events)), nil
}

func (r *memoryRepo) CreateOrGetTranscriptSnapshot(
	ctx context.Context,
	snapshot *entity.TranscriptSnapshot,
) (*entity.TranscriptSnapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%d:%s", snapshot.RunID, snapshot.IdempotencyKey)
	if existing := r.transcriptSnapshots[key]; existing != nil {
		cloned := *existing
		return &cloned, false, nil
	}
	cloned := *snapshot
	r.transcriptSnapshots[key] = &cloned
	return snapshot, true, nil
}

func (r *memoryRepo) GetTranscriptSnapshot(
	ctx context.Context,
	snapshotID int64,
) (*entity.TranscriptSnapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, snapshot := range r.transcriptSnapshots {
		if snapshot.ID == snapshotID {
			cloned := *snapshot
			return &cloned, nil
		}
	}
	return nil, fmt.Errorf("transcript snapshot %d not found", snapshotID)
}

func (r *memoryRepo) CreateOrGetMemoryFlushJob(
	ctx context.Context,
	job *entity.MemoryFlushJob,
) (*entity.MemoryFlushJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := fmt.Sprintf("%d:%s", job.RunID, job.IdempotencyKey)
	if existing := r.memoryFlushJobs[key]; existing != nil {
		cloned := *existing
		return &cloned, false, nil
	}
	cloned := *job
	r.memoryFlushJobs[key] = &cloned
	return job, true, nil
}

func (r *memoryRepo) ClaimMemoryFlushJobs(
	ctx context.Context,
	req repository.ClaimMemoryFlushJobsRequest,
) ([]*entity.MemoryFlushJob, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastClaimMemoryFlushReq = req
	jobs := make([]*entity.MemoryFlushJob, 0, len(r.claimedMemoryFlushJobs))
	for _, job := range r.claimedMemoryFlushJobs {
		jobs = append(jobs, cloneMemoryFlushJob(job))
	}
	return jobs, nil
}

func (r *memoryRepo) AggregateMemoryFlushBacklog(
	ctx context.Context,
	req repository.AggregateMemoryFlushBacklogRequest,
) ([]*entity.MemoryFlushBacklogAggregate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastAggregateMemoryFlushBacklogReq = req
	aggregates := make([]*entity.MemoryFlushBacklogAggregate, 0, len(r.memoryFlushBacklogAggregates))
	for _, aggregate := range r.memoryFlushBacklogAggregates {
		if aggregate == nil {
			continue
		}
		cloned := *aggregate
		aggregates = append(aggregates, &cloned)
	}
	return aggregates, nil
}

func (r *memoryRepo) CompleteMemoryFlushJob(
	ctx context.Context,
	req repository.CompleteMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastCompleteMemoryFlushReq = req
	return cloneMemoryFlushJob(r.completedMemoryFlushJob), r.memoryFlushUpdated, nil
}

func (r *memoryRepo) RetryMemoryFlushJob(
	ctx context.Context,
	req repository.RetryMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRetryMemoryFlushReq = req
	return cloneMemoryFlushJob(r.retriedMemoryFlushJob), r.memoryFlushUpdated, nil
}

func (r *memoryRepo) FailMemoryFlushJob(
	ctx context.Context,
	req repository.FailMemoryFlushJobRequest,
) (*entity.MemoryFlushJob, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastFailMemoryFlushReq = req
	return cloneMemoryFlushJob(r.failedMemoryFlushJob), r.memoryFlushUpdated, nil
}

func (r *memoryRepo) CreateTokenUsage(ctx context.Context, usage *entity.TokenUsage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokenUsages = append(r.tokenUsages, cloneTokenUsage(usage))
	return nil
}

func (r *memoryRepo) ListTokenUsage(ctx context.Context, req repository.ListTokenUsageRequest) ([]*entity.TokenUsage, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastTokenUsageListReq = req

	rows := make([]*entity.TokenUsage, 0, len(r.tokenUsages))
	for _, usage := range r.tokenUsages {
		if req.ThreadID > 0 && usage.ThreadID != req.ThreadID {
			continue
		}
		if len(req.RunIDs) > 0 && !containsInt64(req.RunIDs, usage.RunID) {
			continue
		}
		if len(req.RunIDs) == 0 && req.RunID > 0 && usage.RunID != req.RunID {
			continue
		}
		if req.Source != "" && usage.Source != req.Source {
			continue
		}
		rows = append(rows, cloneTokenUsage(usage))
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].CreatedAt == rows[j].CreatedAt {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].CreatedAt < rows[j].CreatedAt
	})

	total := int64(len(rows))
	page := req.Page
	if page <= 0 {
		page = 1
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	start := int((page - 1) * pageSize)
	if start >= len(rows) {
		return []*entity.TokenUsage{}, total, nil
	}
	end := start + int(pageSize)
	if end > len(rows) {
		end = len(rows)
	}

	return rows[start:end], total, nil
}

func (r *memoryRepo) AggregateTokenUsage(ctx context.Context, req repository.AggregateTokenUsageRequest) (*entity.TokenUsageAggregate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastTokenUsageAggregateReq = req

	aggregate := &entity.TokenUsageAggregate{}
	for _, usage := range r.tokenUsages {
		if req.ThreadID > 0 && usage.ThreadID != req.ThreadID {
			continue
		}
		if len(req.RunIDs) > 0 && !containsInt64(req.RunIDs, usage.RunID) {
			continue
		}
		if len(req.RunIDs) == 0 && req.RunID > 0 && usage.RunID != req.RunID {
			continue
		}
		if req.Source != "" && usage.Source != req.Source {
			continue
		}

		aggregate.InputTokens += usage.InputTokens
		aggregate.OutputTokens += usage.OutputTokens
		aggregate.TotalTokens += usage.TotalTokens
		aggregate.CostMicros += usage.CostMicros
		aggregate.CallCount++
		switch usage.Source {
		case entity.TokenUsageSourceLeadAgent:
			aggregate.LeadAgentTokens += usage.TotalTokens
		case entity.TokenUsageSourceSubagent:
			aggregate.SubagentTokens += usage.TotalTokens
		case entity.TokenUsageSourceMiddleware:
			aggregate.MiddlewareTokens += usage.TotalTokens
		case entity.TokenUsageSourceTool:
			aggregate.ToolTokens += usage.TotalTokens
		}
	}

	return aggregate, nil
}

func (r *memoryRepo) AggregateTokenUsageByRun(ctx context.Context, req repository.AggregateTokenUsageRequest) ([]*entity.RunTokenUsageAggregate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	aggregateByRunID := make(map[int64]*entity.TokenUsageAggregate)
	for _, usage := range r.tokenUsages {
		if req.ThreadID > 0 && usage.ThreadID != req.ThreadID {
			continue
		}
		if len(req.RunIDs) > 0 && !containsInt64(req.RunIDs, usage.RunID) {
			continue
		}
		if len(req.RunIDs) == 0 && req.RunID > 0 && usage.RunID != req.RunID {
			continue
		}
		if req.Source != "" && usage.Source != req.Source {
			continue
		}

		aggregate := aggregateByRunID[usage.RunID]
		if aggregate == nil {
			aggregate = &entity.TokenUsageAggregate{}
			aggregateByRunID[usage.RunID] = aggregate
		}
		aggregate.InputTokens += usage.InputTokens
		aggregate.OutputTokens += usage.OutputTokens
		aggregate.TotalTokens += usage.TotalTokens
		aggregate.CostMicros += usage.CostMicros
		aggregate.CallCount++
		switch usage.Source {
		case entity.TokenUsageSourceLeadAgent:
			aggregate.LeadAgentTokens += usage.TotalTokens
		case entity.TokenUsageSourceSubagent:
			aggregate.SubagentTokens += usage.TotalTokens
		case entity.TokenUsageSourceMiddleware:
			aggregate.MiddlewareTokens += usage.TotalTokens
		case entity.TokenUsageSourceTool:
			aggregate.ToolTokens += usage.TotalTokens
		}
	}

	runIDs := make([]int64, 0, len(aggregateByRunID))
	for runID := range aggregateByRunID {
		runIDs = append(runIDs, runID)
	}
	sort.Slice(runIDs, func(i, j int) bool {
		return runIDs[i] < runIDs[j]
	})

	result := make([]*entity.RunTokenUsageAggregate, 0, len(runIDs))
	for _, runID := range runIDs {
		result = append(result, &entity.RunTokenUsageAggregate{
			RunID:     runID,
			Aggregate: aggregateByRunID[runID],
		})
	}

	return result, nil
}

func (r *memoryRepo) GetTokenUsageSnapshot(
	ctx context.Context,
	req repository.ListTokenUsageRequest,
	includeRunAggregates bool,
) (*repository.TokenUsageSnapshot, error) {
	rows, total, err := r.ListTokenUsage(ctx, req)
	if err != nil {
		return nil, err
	}
	aggregateReq := repository.AggregateTokenUsageRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		RunIDs:   req.RunIDs,
		Source:   req.Source,
	}
	aggregate, err := r.AggregateTokenUsage(ctx, aggregateReq)
	if err != nil {
		return nil, err
	}
	var runAggregates []*entity.RunTokenUsageAggregate
	if includeRunAggregates {
		runAggregates, err = r.AggregateTokenUsageByRun(ctx, aggregateReq)
		if err != nil {
			return nil, err
		}
	}
	return &repository.TokenUsageSnapshot{
		Rows:          rows,
		Total:         total,
		Aggregate:     aggregate,
		RunAggregates: runAggregates,
	}, nil
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (r *memoryRepo) ClaimPendingRuns(ctx context.Context, req repository.ClaimPendingRunsRequest) ([]*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastClaimReq = req

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	pending := make([]*entity.Run, 0)
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.Status == entity.RunStatusPending {
				pending = append(pending, run)
			}
		}
	}
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].CreatedAt == pending[j].CreatedAt {
			return pending[i].ID < pending[j].ID
		}
		return pending[i].CreatedAt < pending[j].CreatedAt
	})

	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	ttlMillis := req.LeaseTTLMillis
	if ttlMillis <= 0 {
		ttlMillis = 60_000
	}
	claimed := make([]*entity.Run, 0, limit)
	for _, run := range pending {
		if len(claimed) >= int(limit) {
			break
		}
		run.Status = entity.RunStatusRunning
		run.WorkerID = req.WorkerID
		run.LeaseOwner = req.WorkerID
		run.LeaseToken = fmt.Sprintf("lease-%d-%d", run.ID, run.ExecutionGeneration+1)
		run.LeaseExpiresAt = now + ttlMillis
		run.HeartbeatAt = now
		run.ExecutionGeneration++
		run.StartedAt = now
		run.UpdatedAt = now
		claimed = append(claimed, cloneRun(run))
	}

	return claimed, nil
}

func (r *memoryRepo) ClaimQueuedResumeRuns(ctx context.Context, req repository.ClaimQueuedResumeRunsRequest) ([]*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastClaimQueuedResumeReq = req

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	queued := make([]*entity.Run, 0)
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.Status == entity.RunStatusQueued && strings.Contains(run.Metadata, `"checkpoint_resume"`) {
				queued = append(queued, run)
			}
		}
	}
	sort.Slice(queued, func(i, j int) bool {
		if queued[i].CreatedAt == queued[j].CreatedAt {
			return queued[i].ID < queued[j].ID
		}
		return queued[i].CreatedAt < queued[j].CreatedAt
	})

	now := req.Now
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	ttlMillis := req.LeaseTTLMillis
	if ttlMillis <= 0 {
		ttlMillis = 60_000
	}
	claimed := make([]*entity.Run, 0, limit)
	for _, run := range queued {
		if len(claimed) >= int(limit) {
			break
		}
		run.Status = entity.RunStatusRunning
		run.WorkerID = req.WorkerID
		run.LeaseOwner = req.WorkerID
		run.LeaseToken = fmt.Sprintf("lease-%d-%d", run.ID, run.ExecutionGeneration+1)
		run.LeaseExpiresAt = now + ttlMillis
		run.HeartbeatAt = now
		run.ExecutionGeneration++
		run.StartedAt = now
		run.UpdatedAt = now
		claimed = append(claimed, cloneRun(run))
	}

	return claimed, nil
}

func (r *memoryRepo) RenewRunLease(
	ctx context.Context,
	req repository.RenewRunLeaseRequest,
) (*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRenewRunLeaseReq = req
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.ID != req.RunID {
				continue
			}
			if run.LeaseOwner != req.LeaseOwner || run.LeaseToken != req.LeaseToken ||
				run.ExecutionGeneration != req.ExecutionGeneration {
				return nil, repository.ErrRunLeaseLost
			}
			run.HeartbeatAt = req.Now
			run.LeaseExpiresAt = req.Now + req.LeaseTTLMillis
			return cloneRun(run), nil
		}
	}
	return nil, fmt.Errorf("run %d not found", req.RunID)
}

func (r *memoryRepo) ReleaseRunLease(
	ctx context.Context,
	req repository.ReleaseRunLeaseRequest,
) (*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastReleaseRunLeaseReq = req
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.ID != req.RunID {
				continue
			}
			if run.LeaseOwner != req.LeaseOwner || run.LeaseToken != req.LeaseToken ||
				run.ExecutionGeneration != req.ExecutionGeneration {
				return nil, repository.ErrRunLeaseLost
			}
			run.Status = req.ToStatus
			run.WorkerID = ""
			run.LeaseOwner = ""
			run.LeaseToken = ""
			run.LeaseExpiresAt = 0
			run.HeartbeatAt = 0
			run.CancelRequestedAt = 0
			return cloneRun(run), nil
		}
	}
	return nil, fmt.Errorf("run %d not found", req.RunID)
}

func (r *memoryRepo) ListExpiredRunLeases(
	ctx context.Context,
	req repository.ListExpiredRunLeasesRequest,
) ([]*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastListExpiredRunLeasesReq = req
	result := make([]*entity.Run, 0)
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.Status == entity.RunStatusRunning && run.ExecutionGeneration > 0 &&
				run.LeaseExpiresAt > 0 && run.LeaseExpiresAt <= req.Now {
				result = append(result, cloneRun(run))
			}
		}
	}
	return result, nil
}

func (r *memoryRepo) ReconcileExpiredRunLease(
	ctx context.Context,
	req repository.ReconcileExpiredRunLeaseRequest,
) (*entity.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastReconcileExpiredRunLeaseReq = req
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.ID != req.RunID {
				continue
			}
			if run.Status != entity.RunStatusRunning || run.LeaseOwner != req.LeaseOwner ||
				run.LeaseToken != req.LeaseToken || run.ExecutionGeneration != req.ExecutionGeneration ||
				run.LeaseExpiresAt <= 0 || run.LeaseExpiresAt > req.Now {
				return nil, repository.ErrRunLeaseLost
			}
			run.Status = req.ToStatus
			run.ErrorCode = req.ErrorCode
			run.ErrorMessage = req.ErrorMessage
			run.EndedAt = req.Now
			run.UpdatedAt = req.Now
			run.WorkerID = ""
			run.LeaseOwner = ""
			run.LeaseToken = ""
			run.LeaseExpiresAt = 0
			run.HeartbeatAt = 0
			run.CancelRequestedAt = 0
			if req.Event != nil {
				r.runEvents[run.ID] = append(r.runEvents[run.ID], cloneRunEvent(req.Event))
			}
			return cloneRun(run), nil
		}
	}
	return nil, fmt.Errorf("run %d not found", req.RunID)
}

func (r *memoryRepo) RequestRunCancellation(
	ctx context.Context,
	req repository.RequestRunCancellationRequest,
) (*repository.RequestRunCancellationResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastRequestRunCancellationReq = req
	for _, runs := range r.runs {
		for _, run := range runs {
			if run.ID != req.RunID {
				continue
			}
			previous := run.Status
			if previous == entity.RunStatusCanceled {
				return &repository.RequestRunCancellationResult{
					Run: cloneRun(run), PreviousStatus: previous,
				}, nil
			}
			run.Status = entity.RunStatusCanceled
			run.ExecutionGeneration++
			run.CancelRequestedAt = req.Now
			run.EndedAt = req.Now
			run.WorkerID = ""
			run.LeaseOwner = ""
			run.LeaseToken = ""
			run.LeaseExpiresAt = 0
			run.HeartbeatAt = 0
			if req.Event != nil {
				r.runEvents[run.ID] = append(r.runEvents[run.ID], cloneRunEvent(req.Event))
			}
			return &repository.RequestRunCancellationResult{
				Run: cloneRun(run), PreviousStatus: previous, Changed: true,
			}, nil
		}
	}
	return nil, fmt.Errorf("run %d not found", req.RunID)
}

func (r *memoryRepo) FinalizeRunSuccess(
	ctx context.Context,
	req repository.FinalizeRunSuccessRequest,
) (*repository.FinalizeRunSuccessResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastFinalizeRunSuccessReq = req
	for threadID, runs := range r.runs {
		for _, run := range runs {
			if run.ID != req.RunID {
				continue
			}
			if run.Status == entity.RunStatusCanceled || run.CancelRequestedAt > 0 {
				return nil, repository.ErrRunCanceled
			}
			if run.Status != entity.RunStatusRunning || run.LeaseOwner != req.LeaseOwner ||
				run.LeaseToken != req.LeaseToken || run.ExecutionGeneration != req.ExecutionGeneration {
				return nil, repository.ErrRunLeaseLost
			}
			run.Status = entity.RunStatusSucceeded
			run.EndedAt = req.Now
			run.WorkerID = ""
			run.LeaseOwner = ""
			run.LeaseToken = ""
			run.LeaseExpiresAt = 0
			run.HeartbeatAt = 0
			r.messages[threadID] = append(r.messages[threadID], cloneMessage(req.Message))
			titleUpdated := false
			if thread := r.threads[threadID]; thread != nil &&
				thread.Title == req.ExpectedThreadTitle && req.ThreadTitle != "" &&
				req.ThreadTitle != req.ExpectedThreadTitle {
				thread.Title = req.ThreadTitle
				titleUpdated = true
				if req.TitleEvent != nil {
					r.runEvents[run.ID] = append(r.runEvents[run.ID], cloneRunEvent(req.TitleEvent))
				}
			}
			if req.CompletionEvent != nil {
				r.runEvents[run.ID] = append(r.runEvents[run.ID], cloneRunEvent(req.CompletionEvent))
			}
			terminalCheckpoint := req.TerminalCheckpoint
			if req.ThreadTitle != "" && req.ThreadTitle != req.ExpectedThreadTitle &&
				!titleUpdated && req.TerminalCheckpointOnTitleConflict != nil {
				terminalCheckpoint = req.TerminalCheckpointOnTitleConflict
			}
			if terminalCheckpoint != nil {
				r.checkpoints[threadID] = append(
					r.checkpoints[threadID],
					cloneCheckpoint(terminalCheckpoint),
				)
			}
			return &repository.FinalizeRunSuccessResult{
				Run: cloneRun(run), Message: cloneMessage(req.Message),
				TitleEvent: cloneRunEvent(req.TitleEvent), CompletionEvent: cloneRunEvent(req.CompletionEvent),
				TerminalCheckpoint: cloneCheckpoint(terminalCheckpoint),
				TitleUpdated:       titleUpdated,
			}, nil
		}
	}
	return nil, fmt.Errorf("run %d not found", req.RunID)
}

func (r *memoryRepo) UpdateRunStatus(ctx context.Context, req repository.UpdateRunStatusRequest) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastUpdateRunReq = req

	for _, runs := range r.runs {
		for _, run := range runs {
			if run.ID != req.RunID {
				continue
			}
			if run.Status != req.From {
				return fmt.Errorf("update run status failed: run %d is not in status %s", req.RunID, req.From)
			}
			if req.WorkerID != "" && run.WorkerID != req.WorkerID {
				return fmt.Errorf("update run status failed: run %d is not owned by worker %s", req.RunID, req.WorkerID)
			}

			run.Status = req.To
			run.ErrorCode = req.ErrorCode
			run.ErrorMessage = req.ErrorMessage
			run.UpdatedAt = time.Now().UnixMilli()
			if isMemoryTerminalRunStatus(req.To) {
				run.EndedAt = run.UpdatedAt
			}
			if req.Event != nil && !req.EventAlreadyPersisted {
				r.runEvents[run.ID] = append(r.runEvents[run.ID], cloneRunEvent(req.Event))
			}
			if req.EventAlreadyPersisted &&
				req.To == entity.RunStatusInterrupted &&
				req.OutboxIntent != nil &&
				req.OutboxIntent.Event.EventType == domainnotification.EventTaskAwaitingInput {
				if _, ok := entity.RunAwaitingInputInteractionRefFromEventPayload(req.EventPayload); ok {
					if err := req.OutboxIntent.Append(ctx, nil, req.OutboxIntent.Event); err != nil {
						return err
					}
				}
			}

			return nil
		}
	}

	return fmt.Errorf("run %d not found", req.RunID)
}

func cloneThread(thread *entity.Thread) *entity.Thread {
	if thread == nil {
		return nil
	}
	cloned := *thread
	return &cloned
}

func cloneMessage(message *entity.Message) *entity.Message {
	if message == nil {
		return nil
	}
	cloned := *message
	return &cloned
}

func cloneRun(run *entity.Run) *entity.Run {
	if run == nil {
		return nil
	}
	cloned := *run
	return &cloned
}

func cloneRunEvent(event *entity.RunEvent) *entity.RunEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	return &cloned
}

func cloneCheckpoint(checkpoint *entity.Checkpoint) *entity.Checkpoint {
	if checkpoint == nil {
		return nil
	}
	cloned := *checkpoint
	return &cloned
}

func cloneMemory(memory *entity.Memory) *entity.Memory {
	if memory == nil {
		return nil
	}
	cloned := *memory
	return &cloned
}

func cloneMemoryAuditEvent(event *entity.MemoryAuditEvent) *entity.MemoryAuditEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	return &cloned
}

func cloneMemoryFlushJob(job *entity.MemoryFlushJob) *entity.MemoryFlushJob {
	if job == nil {
		return nil
	}
	cloned := *job
	return &cloned
}

func cloneTokenUsage(usage *entity.TokenUsage) *entity.TokenUsage {
	if usage == nil {
		return nil
	}
	cloned := *usage
	return &cloned
}

type fixedIDGen struct {
	next int64
}

type failingIDGen struct {
	err error
}

func (g failingIDGen) GenID(context.Context) (int64, error) {
	return 0, g.err
}

func (g failingIDGen) GenMultiIDs(context.Context, int) ([]int64, error) {
	return nil, g.err
}

func (g fixedIDGen) GenID(ctx context.Context) (int64, error) {
	return g.next, nil
}

func (g fixedIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		ids[i] = g.next + int64(i)
	}
	return ids, nil
}

func isMemoryTerminalRunStatus(status entity.RunStatus) bool {
	switch status {
	case entity.RunStatusSucceeded, entity.RunStatusFailed, entity.RunStatusCanceled:
		return true
	default:
		return false
	}
}

type sequenceIDGen struct {
	mu   sync.Mutex
	next int64
}

func newSequenceIDGen(next int64) *sequenceIDGen {
	return &sequenceIDGen{next: next}
}

func (g *sequenceIDGen) GenID(ctx context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := g.next
	g.next++
	return id, nil
}

func (g *sequenceIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}
