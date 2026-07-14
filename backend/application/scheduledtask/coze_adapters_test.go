// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	workflowmodel "github.com/coze-dev/coze-studio/backend/crossdomain/workflow/model"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	workflowentity "github.com/coze-dev/coze-studio/backend/domain/workflow/entity"
	workflowvo "github.com/coze-dev/coze-studio/backend/domain/workflow/entity/vo"
)

func TestCozeAgentRunnerMapsScheduledTaskIntoAgentThread(t *testing.T) {
	t.Parallel()
	client := &agentThreadClientStub{
		createTaskResponse: &appagentthread.CreateTaskThreadResponse{
			Thread: &appagentthread.ThreadSummary{ThreadID: 30},
			Run:    &appagentthread.RunSummary{RunID: 40, ThreadID: 30},
		},
	}
	runner := &CozeAgentRunner{Client: client, PollInterval: time.Nanosecond}

	ref, err := runner.StartNew(context.Background(), AgentRunRequest{
		SpaceID: 1, UserID: 7, AgentID: 200, TaskID: 10, ExecutionID: 20,
		Message: "生成日报", Variables: map[string]any{"city": "武汉"},
	})

	require.NoError(t, err)
	require.Equal(t, AgentRunRef{ThreadID: 30, RunID: 40}, ref)
	require.Equal(t, "200", client.createTaskRequest.AssistantID)
	require.Contains(t, client.createTaskRequest.Context, `"city":"武汉"`)
	require.Contains(t, client.createTaskRequest.Metadata, `"scheduled_task_id":"10"`)
}

func TestCozeAgentRunnerWaitsForTerminalStatus(t *testing.T) {
	t.Parallel()
	client := &agentThreadClientStub{runs: []*appagentthread.RunSummary{
		{RunID: 40, ThreadID: 30, Status: appagentthread.RunStatusRunning},
		{RunID: 40, ThreadID: 30, Status: appagentthread.RunStatusSucceeded},
	}}
	runner := &CozeAgentRunner{Client: client, PollInterval: time.Nanosecond}

	status, err := runner.WaitTerminal(context.Background(), 7, 30, 40)

	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusSucceeded, status.Status)
}

func TestDomainWorkflowRunnerUsesPublishedReleaseExecution(t *testing.T) {
	t.Parallel()
	version := "v1"
	domain := &workflowDomainStub{workflow: &workflowentity.Workflow{ID: 300, Meta: &workflowvo.Meta{SpaceID: 1, Name: "日报流程", LatestPublishedVersion: &version}}, executionID: 50, terminal: &workflowentity.WorkflowExecution{ID: 50, Status: workflowentity.WorkflowSuccess}}
	runner := &DomainWorkflowRunner{Domain: domain, PollInterval: time.Nanosecond}

	executionID, err := runner.Start(context.Background(), 1, 7, 300, map[string]any{"city": "武汉"})

	require.NoError(t, err)
	require.Equal(t, int64(50), executionID)
	require.Equal(t, workflowmodel.ExecuteModeRelease, domain.config.Mode)
	require.Equal(t, workflowmodel.TaskTypeBackground, domain.config.TaskType)
	require.Equal(t, workflowmodel.SyncPatternAsync, domain.config.SyncPattern)

	status, err := runner.WaitTerminal(context.Background(), 300, 50)
	require.NoError(t, err)
	require.Equal(t, entity.ExecutionStatusSucceeded, status.Status)
}

func TestMySQLAgentTargetReaderListsOnlyPublishedWorkspaceAgents(t *testing.T) {
	t.Parallel()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TABLE single_agent_draft (agent_id INTEGER PRIMARY KEY, space_id INTEGER, name TEXT, icon_uri TEXT, updated_at INTEGER DEFAULT 0, deleted_at DATETIME NULL)`).Error)
	require.NoError(t, db.Exec(`CREATE TABLE single_agent_publish (id INTEGER PRIMARY KEY, agent_id INTEGER, status INTEGER)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO single_agent_draft(agent_id, space_id, name, icon_uri) VALUES (1, 10, '日报助手', 'icon'), (2, 10, '未发布助手', ''), (3, 11, '其他空间', '')`).Error)
	require.NoError(t, db.Exec(`INSERT INTO single_agent_publish(id, agent_id, status) VALUES (1, 1, 0), (2, 3, 0)`).Error)
	reader := &MySQLAgentTargetReader{DB: db}

	targets, total, err := reader.List(context.Background(), 10, "日报", 1, 20)

	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, targets, 1)
	require.Equal(t, int64(1), targets[0].ID)
	require.True(t, targets[0].Published)
}

type agentThreadClientStub struct {
	createTaskRequest  *appagentthread.CreateTaskThreadRequest
	createTaskResponse *appagentthread.CreateTaskThreadResponse
	createRunRequest   *appagentthread.CreateRunRequest
	runs               []*appagentthread.RunSummary
}

func (s *agentThreadClientStub) CreateTaskThread(_ context.Context, req *appagentthread.CreateTaskThreadRequest) (*appagentthread.CreateTaskThreadResponse, error) {
	s.createTaskRequest = req
	return s.createTaskResponse, nil
}

func (s *agentThreadClientStub) CreateRun(_ context.Context, req *appagentthread.CreateRunRequest) (*appagentthread.CreateRunResponse, error) {
	s.createRunRequest = req
	return &appagentthread.CreateRunResponse{Run: &appagentthread.RunSummary{RunID: 41, ThreadID: req.ThreadID}}, nil
}

func (s *agentThreadClientStub) GetRun(context.Context, *appagentthread.GetRunRequest) (*appagentthread.GetRunResponse, error) {
	run := s.runs[0]
	s.runs = s.runs[1:]
	return &appagentthread.GetRunResponse{Run: run}, nil
}

type workflowDomainStub struct {
	workflow    *workflowentity.Workflow
	executionID int64
	terminal    *workflowentity.WorkflowExecution
	config      workflowmodel.ExecuteConfig
}

func (s *workflowDomainStub) Get(context.Context, *workflowvo.GetPolicy) (*workflowentity.Workflow, error) {
	return s.workflow, nil
}

func (s *workflowDomainStub) AsyncExecute(_ context.Context, config workflowmodel.ExecuteConfig, _ map[string]any) (int64, error) {
	s.config = config
	return s.executionID, nil
}

func (s *workflowDomainStub) GetExecution(context.Context, *workflowentity.WorkflowExecution, bool) (*workflowentity.WorkflowExecution, error) {
	return s.terminal, nil
}
