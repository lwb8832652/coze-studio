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
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
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

func TestApplicationCreateRunMapsDomainRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		createdRun: &entity.Run{
			ID:                200,
			ThreadID:          10,
			SpaceID:           1,
			CreatorID:         2,
			AssistantID:       "default",
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

	resp, err := app.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID:       10,
		AssistantID:    "default",
		Input:          `{"messages":[]}`,
		Config:         `{"mode":"Auto"}`,
		Context:        `{"source":"web"}`,
		Metadata:       `{"trace":"abc"}`,
		IdempotencyKey: "idem-1",
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.createRunReq.ThreadID)
	require.Equal(t, "default", domainSVC.createRunReq.AssistantID)
	require.Equal(t, `{"messages":[]}`, domainSVC.createRunReq.Input)
	require.Equal(t, `{"mode":"Auto"}`, domainSVC.createRunReq.Config)
	require.Equal(t, `{"source":"web"}`, domainSVC.createRunReq.Context)
	require.Equal(t, `{"trace":"abc"}`, domainSVC.createRunReq.Metadata)
	require.Equal(t, "idem-1", domainSVC.createRunReq.IdempotencyKey)
	require.Equal(t, int64(200), resp.Run.RunID)
	require.Equal(t, int64(10), resp.Run.ThreadID)
	require.Equal(t, RunStatusPending, resp.Run.Status)
	require.Equal(t, `{"messages":[]}`, resp.Run.Input)
	require.Equal(t, `["messages","updates"]`, resp.Run.StreamMode)
}

func TestApplicationListRunsMapsDomainRuns(t *testing.T) {
	domainSVC := &recordingThreadService{
		runs: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusPending,
				Input:    `{}`,
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
		ThreadID: 10,
		Status:   &status,
		Page:     2,
		PageSize: 5,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), domainSVC.listRunsReq.ThreadID)
	require.NotNil(t, domainSVC.listRunsReq.Status)
	require.Equal(t, entity.RunStatusPending, *domainSVC.listRunsReq.Status)
	require.Equal(t, int32(2), domainSVC.listRunsReq.Page)
	require.Equal(t, int32(5), domainSVC.listRunsReq.PageSize)
	require.Equal(t, int64(2), resp.Total)
	require.Len(t, resp.Runs, 2)
	require.Equal(t, int64(200), resp.Runs[0].RunID)
	require.Equal(t, RunStatusPending, resp.Runs[0].Status)
	require.Equal(t, int64(201), resp.Runs[1].RunID)
	require.Equal(t, RunStatusRunning, resp.Runs[1].Status)
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

type recordingThreadService struct {
	created         *entity.Thread
	createdRun      *entity.Run
	claimedRuns     []*entity.Run
	completedRun    *entity.Run
	failedRun       *entity.Run
	canceledRun     *entity.Run
	listed          []*entity.Thread
	got             *entity.Thread
	appended        *entity.Message
	messages        []*entity.Message
	runs            []*entity.Run
	total           int64
	messageTotal    int64
	runTotal        int64
	createReq       *domainservice.CreateThreadRequest
	createRunReq    *domainservice.CreateRunRequest
	claimRunsReq    *domainservice.ClaimPendingRunsRequest
	completeRunReq  *domainservice.UpdateRunStatusRequest
	failRunReq      *domainservice.UpdateRunStatusRequest
	cancelRunReq    *domainservice.UpdateRunStatusRequest
	listReq         *domainservice.ListThreadsRequest
	listRunsReq     *domainservice.ListRunsRequest
	appendReq       *domainservice.AppendMessageRequest
	listMessagesReq *domainservice.ListMessagesRequest
	getID           int64
	getRunID        int64
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
			space_id integer,
			creator_id integer,
			assistant_id text,
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
	}
	return nil, nil
}

func (s *recordingThreadService) ListRuns(ctx context.Context, req *domainservice.ListRunsRequest) ([]*entity.Run, int64, error) {
	s.listRunsReq = req
	return s.runs, s.runTotal, nil
}

func (s *recordingThreadService) ClaimPendingRuns(ctx context.Context, req *domainservice.ClaimPendingRunsRequest) ([]*entity.Run, error) {
	s.claimRunsReq = req
	return s.claimedRuns, nil
}

func (s *recordingThreadService) CompleteRun(ctx context.Context, req *domainservice.UpdateRunStatusRequest) (*entity.Run, error) {
	s.completeRunReq = req
	return s.completedRun, nil
}

func (s *recordingThreadService) FailRun(ctx context.Context, req *domainservice.UpdateRunStatusRequest) (*entity.Run, error) {
	s.failRunReq = req
	return s.failedRun, nil
}

func (s *recordingThreadService) CancelRun(ctx context.Context, req *domainservice.UpdateRunStatusRequest) (*entity.Run, error) {
	s.cancelRunReq = req
	return s.canceledRun, nil
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
