// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	workflowmodel "github.com/coze-dev/coze-studio/backend/crossdomain/workflow/model"
	"github.com/coze-dev/coze-studio/backend/domain/scheduledtask/entity"
	domainworkflow "github.com/coze-dev/coze-studio/backend/domain/workflow"
	workflowentity "github.com/coze-dev/coze-studio/backend/domain/workflow/entity"
	workflowvo "github.com/coze-dev/coze-studio/backend/domain/workflow/entity/vo"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

type AgentThreadClient interface {
	CreateTaskThread(context.Context, *appagentthread.CreateTaskThreadRequest) (*appagentthread.CreateTaskThreadResponse, error)
	CreateRun(context.Context, *appagentthread.CreateRunRequest) (*appagentthread.CreateRunResponse, error)
	GetRun(context.Context, *appagentthread.GetRunRequest) (*appagentthread.GetRunResponse, error)
}

type CozeAgentRunner struct {
	Client       AgentThreadClient
	PollInterval time.Duration
}

func (r *CozeAgentRunner) StartNew(ctx context.Context, req AgentRunRequest) (AgentRunRef, error) {
	if r == nil || r.Client == nil { return AgentRunRef{}, fmt.Errorf("agent thread client is unavailable") }
	runContext, metadata, err := scheduledRunMetadata(req)
	if err != nil { return AgentRunRef{}, err }
	resp, err := r.Client.CreateTaskThread(ctx, &appagentthread.CreateTaskThreadRequest{
		SpaceID: req.SpaceID, UserID: req.UserID, Message: req.Message,
		Title: "定时任务 · " + req.Message, AssistantID: strconv.FormatInt(req.AgentID, 10),
		Context: runContext, Metadata: metadata,
		IdempotencyKey: fmt.Sprintf("scheduled-task:%d:%d", req.TaskID, req.ExecutionID),
	})
	if err != nil { return AgentRunRef{}, err }
	if resp == nil || resp.Thread == nil || resp.Run == nil { return AgentRunRef{}, fmt.Errorf("agent thread response is incomplete") }
	return AgentRunRef{ThreadID: resp.Thread.ThreadID, RunID: resp.Run.RunID}, nil
}

func (r *CozeAgentRunner) StartInThread(ctx context.Context, threadID int64, req AgentRunRequest) (AgentRunRef, error) {
	if r == nil || r.Client == nil { return AgentRunRef{}, fmt.Errorf("agent thread client is unavailable") }
	runContext, metadata, err := scheduledRunMetadata(req)
	if err != nil { return AgentRunRef{}, err }
	ctx = appagentthread.WithThreadAccessRequest(ctx, appagentthread.ThreadAccessRequest{ViewerID: req.UserID, ThreadID: threadID})
	resp, err := r.Client.CreateRun(ctx, &appagentthread.CreateRunRequest{
		ThreadID: threadID, AssistantID: strconv.FormatInt(req.AgentID, 10),
		RunKind: appagentthread.RunKindTask, Context: runContext, Metadata: metadata,
		IdempotencyKey: fmt.Sprintf("scheduled-task:%d:%d", req.TaskID, req.ExecutionID),
		MessageContent: req.Message, MessageMetadata: metadata,
	})
	if err != nil { return AgentRunRef{}, err }
	if resp == nil || resp.Run == nil { return AgentRunRef{}, fmt.Errorf("agent run response is incomplete") }
	return AgentRunRef{ThreadID: threadID, RunID: resp.Run.RunID}, nil
}

func (r *CozeAgentRunner) WaitTerminal(ctx context.Context, userID, threadID, runID int64) (RunTerminalStatus, error) {
	if r == nil || r.Client == nil { return RunTerminalStatus{}, fmt.Errorf("agent thread client is unavailable") }
	interval := r.PollInterval
	if interval <= 0 { interval = time.Second }
	for {
		accessCtx := appagentthread.WithThreadAccessRequest(ctx, appagentthread.ThreadAccessRequest{ViewerID: userID, ThreadID: threadID, RunID: runID})
		resp, err := r.Client.GetRun(accessCtx, &appagentthread.GetRunRequest{RunID: runID})
		if err != nil { return RunTerminalStatus{}, err }
		if resp == nil || resp.Run == nil { return RunTerminalStatus{}, fmt.Errorf("agent run is unavailable") }
		switch resp.Run.Status {
		case appagentthread.RunStatusSucceeded:
			return RunTerminalStatus{Status: entity.ExecutionStatusSucceeded}, nil
		case appagentthread.RunStatusFailed, appagentthread.RunStatusInterrupted:
			return RunTerminalStatus{Status: entity.ExecutionStatusFailed, ErrorCode: resp.Run.ErrorCode, ErrorMessage: resp.Run.ErrorMessage}, nil
		case appagentthread.RunStatusCanceled:
			return RunTerminalStatus{Status: entity.ExecutionStatusCanceled, ErrorCode: resp.Run.ErrorCode}, nil
		}
		select {
		case <-ctx.Done(): return RunTerminalStatus{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func scheduledRunMetadata(req AgentRunRequest) (string, string, error) {
	contextPayload, err := json.Marshal(map[string]any{"scheduled_task": map[string]any{"task_id": strconv.FormatInt(req.TaskID, 10), "execution_id": strconv.FormatInt(req.ExecutionID, 10), "variables": req.Variables}})
	if err != nil { return "", "", err }
	metadata, err := json.Marshal(map[string]string{"source": "scheduled_task", "scheduled_task_id": strconv.FormatInt(req.TaskID, 10), "scheduled_task_execution_id": strconv.FormatInt(req.ExecutionID, 10)})
	if err != nil { return "", "", err }
	return string(contextPayload), string(metadata), nil
}

type WorkflowDomain interface {
	Get(context.Context, *workflowvo.GetPolicy) (*workflowentity.Workflow, error)
	AsyncExecute(context.Context, workflowmodel.ExecuteConfig, map[string]any) (int64, error)
	GetExecution(context.Context, *workflowentity.WorkflowExecution, bool) (*workflowentity.WorkflowExecution, error)
}

type DomainWorkflowRunner struct {
	Domain       WorkflowDomain
	PollInterval time.Duration
}

func (r *DomainWorkflowRunner) Start(ctx context.Context, spaceID, userID, workflowID int64, inputs map[string]any) (int64, error) {
	if r == nil || r.Domain == nil { return 0, fmt.Errorf("workflow domain is unavailable") }
	workflow, err := r.Domain.Get(ctx, &workflowvo.GetPolicy{ID: workflowID, MetaOnly: true})
	if err != nil { return 0, err }
	if workflow == nil || workflow.Meta == nil || workflow.SpaceID != spaceID { return 0, ErrTargetUnavailable }
	if workflow.LatestPublishedVersion == nil { return 0, fmt.Errorf("%w: workflow is not published", ErrTargetUnavailable) }
	config := workflowmodel.ExecuteConfig{ID: workflowID, From: workflowmodel.FromSpecificVersion, Version: *workflow.LatestPublishedVersion, Operator: userID, Mode: workflowmodel.ExecuteModeRelease, ConnectorID: consts.CozeConnectorID, ConnectorUID: strconv.FormatInt(userID, 10), TaskType: workflowmodel.TaskTypeBackground, SyncPattern: workflowmodel.SyncPatternAsync, InputFailFast: true, BizType: workflowmodel.BizTypeWorkflow, Cancellable: true}
	return r.Domain.AsyncExecute(ctx, config, inputs)
}

func (r *DomainWorkflowRunner) WaitTerminal(ctx context.Context, workflowID, executionID int64) (RunTerminalStatus, error) {
	if r == nil || r.Domain == nil { return RunTerminalStatus{}, fmt.Errorf("workflow domain is unavailable") }
	interval := r.PollInterval
	if interval <= 0 { interval = time.Second }
	for {
		execution, err := r.Domain.GetExecution(ctx, &workflowentity.WorkflowExecution{ID: executionID, WorkflowID: workflowID}, false)
		if err != nil { return RunTerminalStatus{}, err }
		if execution == nil { return RunTerminalStatus{}, fmt.Errorf("workflow execution is unavailable") }
		switch execution.Status {
		case workflowentity.WorkflowSuccess:
			return RunTerminalStatus{Status: entity.ExecutionStatusSucceeded}, nil
		case workflowentity.WorkflowFailed, workflowentity.WorkflowInterrupted:
			return RunTerminalStatus{Status: entity.ExecutionStatusFailed, ErrorCode: stringValue(execution.ErrorCode), ErrorMessage: stringValue(execution.FailReason)}, nil
		case workflowentity.WorkflowCancel:
			return RunTerminalStatus{Status: entity.ExecutionStatusCanceled}, nil
		}
		select {
		case <-ctx.Done(): return RunTerminalStatus{}, ctx.Err()
		case <-time.After(interval):
		}
	}
}

func stringValue(value *string) string { if value == nil { return "" }; return *value }

type TargetReader interface {
	Resolve(context.Context, int64, int64) (*Target, error)
	List(context.Context, int64, string, int32, int32) ([]*Target, int64, error)
}

type CozeTargetCatalog struct { AgentTargets, WorkflowTargets TargetReader }

func (c *CozeTargetCatalog) Resolve(ctx context.Context, spaceID int64, targetType entity.TargetType, targetID int64) (*Target, error) {
	reader := c.reader(targetType)
	if reader == nil { return nil, ErrTargetUnavailable }
	return reader.Resolve(ctx, spaceID, targetID)
}

func (c *CozeTargetCatalog) List(ctx context.Context, req ListTargetsRequest) ([]*Target, int64, error) {
	reader := c.reader(req.Type)
	if reader == nil { return nil, 0, ErrTargetUnavailable }
	return reader.List(ctx, req.SpaceID, req.Keyword, req.Page, req.PageSize)
}

func (c *CozeTargetCatalog) reader(targetType entity.TargetType) TargetReader {
	if c == nil { return nil }
	switch targetType { case entity.TargetTypeAgent: return c.AgentTargets; case entity.TargetTypeWorkflow: return c.WorkflowTargets; default: return nil }
}

type MySQLAgentTargetReader struct { DB *gorm.DB }

type agentTargetRow struct { ID int64; Name string; IconURI string; Published bool }

func (r *MySQLAgentTargetReader) Resolve(ctx context.Context, spaceID, targetID int64) (*Target, error) {
	if r == nil || r.DB == nil { return nil, ErrTargetUnavailable }
	var row agentTargetRow
	err := r.agentQuery(ctx, spaceID, "").Where("d.agent_id = ?", targetID).Take(&row).Error
	if err != nil { return nil, err }
	return agentRowTarget(row), nil
}

func (r *MySQLAgentTargetReader) List(ctx context.Context, spaceID int64, keyword string, page, pageSize int32) ([]*Target, int64, error) {
	if r == nil || r.DB == nil { return nil, 0, ErrTargetUnavailable }
	page, pageSize = normalizedTargetPage(page, pageSize)
	query := r.agentQuery(ctx, spaceID, keyword)
	var total int64
	if err := query.Count(&total).Error; err != nil { return nil, 0, err }
	var rows []agentTargetRow
	if err := query.Order("d.updated_at DESC, d.agent_id DESC").Limit(int(pageSize)).Offset(int((page-1)*pageSize)).Scan(&rows).Error; err != nil { return nil, 0, err }
	targets := make([]*Target, 0, len(rows))
	for _, row := range rows { targets = append(targets, agentRowTarget(row)) }
	return targets, total, nil
}

func (r *MySQLAgentTargetReader) agentQuery(ctx context.Context, spaceID int64, keyword string) *gorm.DB {
	query := r.DB.WithContext(ctx).Table("single_agent_draft AS d").
		Select("d.agent_id AS id, d.name, d.icon_uri, TRUE AS published").
		Where("d.space_id = ? AND d.deleted_at IS NULL AND EXISTS (SELECT 1 FROM single_agent_publish p WHERE p.agent_id = d.agent_id AND p.status = 0)", spaceID)
	if value := strings.TrimSpace(keyword); value != "" { query = query.Where("d.name LIKE ?", "%"+value+"%") }
	return query
}

func agentRowTarget(row agentTargetRow) *Target { return &Target{ID: row.ID, Type: entity.TargetTypeAgent, Name: row.Name, IconURI: row.IconURI, Published: row.Published} }

type WorkflowMetadataDomain interface {
	Get(context.Context, *workflowvo.GetPolicy) (*workflowentity.Workflow, error)
	MGet(context.Context, *workflowvo.MGetPolicy) ([]*workflowentity.Workflow, int64, error)
}

type DomainWorkflowTargetReader struct { Domain WorkflowMetadataDomain }

func (r *DomainWorkflowTargetReader) Resolve(ctx context.Context, spaceID, targetID int64) (*Target, error) {
	if r == nil || r.Domain == nil { return nil, ErrTargetUnavailable }
	workflow, err := r.Domain.Get(ctx, &workflowvo.GetPolicy{ID: targetID, MetaOnly: true})
	if err != nil { return nil, err }
	if workflow == nil || workflow.Meta == nil || workflow.SpaceID != spaceID || workflow.LatestPublishedVersion == nil { return nil, ErrTargetUnavailable }
	return workflowTarget(workflow), nil
}

func (r *DomainWorkflowTargetReader) List(ctx context.Context, spaceID int64, keyword string, page, pageSize int32) ([]*Target, int64, error) {
	if r == nil || r.Domain == nil { return nil, 0, ErrTargetUnavailable }
	page, pageSize = normalizedTargetPage(page, pageSize)
	publishStatus := workflowvo.HasPublished
	var name *string
	if value := strings.TrimSpace(keyword); value != "" { name = &value }
	workflows, total, err := r.Domain.MGet(ctx, &workflowvo.MGetPolicy{MetaQuery: workflowvo.MetaQuery{SpaceID: &spaceID, Page: &workflowvo.Page{Page: page, Size: pageSize}, Name: name, PublishStatus: &publishStatus, NeedTotalNumber: true, DescByUpdate: true}, MetaOnly: true})
	if err != nil { return nil, 0, err }
	targets := make([]*Target, 0, len(workflows))
	for _, workflow := range workflows { if workflow != nil && workflow.Meta != nil && workflow.LatestPublishedVersion != nil { targets = append(targets, workflowTarget(workflow)) } }
	return targets, total, nil
}

func workflowTarget(workflow *workflowentity.Workflow) *Target { return &Target{ID: workflow.ID, Type: entity.TargetTypeWorkflow, Name: workflow.Name, IconURI: workflow.IconURI, Published: workflow.LatestPublishedVersion != nil} }

func normalizedTargetPage(page, pageSize int32) (int32, int32) { if page <= 0 { page = 1 }; if pageSize <= 0 { pageSize = 20 }; if pageSize > 100 { pageSize = 100 }; return page, pageSize }

var _ WorkflowDomain = domainworkflow.Service(nil)
