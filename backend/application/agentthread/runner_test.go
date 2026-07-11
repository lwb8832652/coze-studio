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
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestRunProcessorCompletesClaimedRunWithAssistantMessage(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:                  200,
				ThreadID:            10,
				Status:              entity.RunStatusRunning,
				Input:               `{"messages":[{"role":"user","content":"分析客户反馈"}]}`,
				WorkerID:            "worker-a",
				LeaseOwner:          "worker-a",
				LeaseToken:          "lease-200",
				LeaseExpiresAt:      60_000,
				ExecutionGeneration: 3,
				StartedAt:           300,
				UpdatedAt:           301,
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "客户反馈已完成分析",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
			EndedAt:  400,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	var executedRun *RunSummary
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		executedRun = run

		return &RunExecutionResult{
			Message:  "客户反馈已完成分析",
			Metadata: `{"source":"agent_harness"}`,
		}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	err := processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Equal(t, "worker-a", domainSVC.claimRunsReq.WorkerID)
	require.Equal(t, int32(1), domainSVC.claimRunsReq.Limit)
	require.NotNil(t, executedRun)
	require.Equal(t, int64(200), executedRun.RunID)
	require.Equal(t, int64(200), domainSVC.appendReq.RunID)
	require.Equal(t, entity.MessageRoleAssistant, domainSVC.appendReq.Role)
	require.Equal(t, "客户反馈已完成分析", domainSVC.appendReq.Content)
	require.Equal(t, `{"source":"agent_harness"}`, domainSVC.appendReq.Metadata)
	require.Equal(t, int64(200), domainSVC.completeRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.completeRunReq.From)
	require.Equal(t, "worker-a", domainSVC.completeRunReq.WorkerID)
	require.Equal(t, "worker-a", domainSVC.completeRunReq.LeaseOwner)
	require.Equal(t, "lease-200", domainSVC.completeRunReq.LeaseToken)
	require.Equal(t, uint64(3), domainSVC.completeRunReq.ExecutionGeneration)
	require.Nil(t, domainSVC.failRunReq)
	require.Equal(t, []string{"run.started", "run.completed"}, eventSink.eventTypes())
	require.Equal(t, int64(10), eventSink.events[0].ThreadID)
	require.Equal(t, int64(200), eventSink.events[0].RunID)
	require.Contains(t, eventSink.events[0].Payload, `"status":"running"`)
	require.Contains(t, eventSink.events[0].Payload, `"worker_id":"worker-a"`)
	require.Contains(t, eventSink.events[1].Payload, `"status":"succeeded"`)
}

func TestRunProcessorTreatsLateSuccessAfterCancellationAsCanceled(t *testing.T) {
	userMessage := "生成文档"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	domainSVC := &recordingThreadService{
		got: &entity.Thread{ID: 10, Title: taskThreadTitle("", userMessage)},
		claimedRuns: []*entity.Run{{
			ID:                  200,
			ThreadID:            10,
			Status:              entity.RunStatusRunning,
			Input:               input,
			WorkerID:            "worker-a",
			LeaseOwner:          "worker-a",
			LeaseToken:          "lease-200",
			ExecutionGeneration: 3,
		}},
		finalizeRunSuccessErr: domainrepo.ErrRunCanceled,
	}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(
		&ApplicationService{ThreadSVC: domainSVC},
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "done", Title: "生成文档结果"}, nil
		}),
		RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1, EventSink: eventSink},
	)

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.CanceledRuns)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	require.Nil(t, domainSVC.appendReq)
	require.Nil(t, domainSVC.updateThreadTitleReq)
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
}

func TestRunProcessorGeneratesThreadTitleAfterFirstExchange(t *testing.T) {
	userMessage := "请生成一份《武汉3日游攻略》正式文档，包含行程概览、每日安排、预算表、注意事项"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	initialTitle := taskThreadTitle("", userMessage)
	domainSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:    10,
			Title: initialTitle,
		},
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    input,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "文档已生成",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "文档已生成"}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	err = processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.NotNil(t, domainSVC.updateThreadTitleReq)
	require.Equal(t, int64(10), domainSVC.updateThreadTitleReq.ThreadID)
	require.Equal(t, "武汉3日游攻略", domainSVC.updateThreadTitleReq.Title)
	require.Equal(t, []string{"run.started", "context.thread_title_updated", "run.completed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[1].Payload, `"thread_title":"武汉3日游攻略"`)
}

func TestRunProcessorUsesCleanExplicitGeneratedThreadTitle(t *testing.T) {
	userMessage := "请帮我安排青岛家庭旅行，顺便推荐适合孩子的路线"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	initialTitle := taskThreadTitle("", userMessage)
	domainSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:    10,
			Title: initialTitle,
		},
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    input,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "路线已整理",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{
			Message: "路线已整理",
			Title:   `<think>需要保留短标题</think>"青岛亲子旅行路线"。`,
		}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	err = processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.NotNil(t, domainSVC.updateThreadTitleReq)
	require.Equal(t, "青岛亲子旅行路线", domainSVC.updateThreadTitleReq.Title)
	require.Contains(t, eventSink.events[1].Payload, `"thread_title":"青岛亲子旅行路线"`)
	require.NotContains(t, eventSink.events[1].Payload, "think")
}

func TestRunProcessorGeneratesThreadTitleWithTitleGenerator(t *testing.T) {
	userMessage := "请根据下面这段很长的需求帮我整理一份可执行的项目上线计划，包含排期、风险、负责人和验收标准"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	initialTitle := taskThreadTitle("", userMessage)
	domainSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:    10,
			Title: initialTitle,
		},
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    input,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "上线计划已整理完成",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	titleGenerator := &recordingRunTitleGenerator{title: "项目上线计划"}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "上线计划已整理完成"}, nil
	}), RunProcessorOptions{
		WorkerID:       "worker-a",
		BatchSize:      1,
		EventSink:      eventSink,
		TitleGenerator: titleGenerator,
	})

	err = processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, titleGenerator.calls)
	require.Equal(t, userMessage, titleGenerator.input.UserMessage)
	require.Equal(t, "上线计划已整理完成", titleGenerator.input.AssistantMessage)
	require.Equal(t, int64(200), titleGenerator.input.Run.RunID)
	require.NotNil(t, domainSVC.updateThreadTitleReq)
	require.Equal(t, "项目上线计划", domainSVC.updateThreadTitleReq.Title)
	require.Equal(t, []string{"run.started", "context.thread_title_updated", "run.completed"}, eventSink.eventTypes())
}

func TestRunProcessorDoesNotOverrideExistingThreadTitleOnFollowUp(t *testing.T) {
	input, err := taskThreadRunInputFromMessage("能把预算表导出成 Excel 吗？")
	require.NoError(t, err)
	domainSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:    10,
			Title: "武汉3日游攻略",
		},
		claimedRuns: []*entity.Run{
			{
				ID:       201,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    input,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       301,
			ThreadID: 10,
			RunID:    201,
			Role:     entity.MessageRoleAssistant,
			Content:  "可以，我会整理预算表",
		},
		completedRun: &entity.Run{
			ID:       201,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "可以，我会整理预算表"}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	err = processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Nil(t, domainSVC.updateThreadTitleReq)
	require.Equal(t, []string{"run.started", "run.completed"}, eventSink.eventTypes())
}

func TestRunProcessorReportsProcessResultForCompletedRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "ok",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
	})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, RunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
}

func TestRunProcessorIsolatesBatchFailureAndReleasesUnfinalizedLease(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{ID: 200, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200", ExecutionGeneration: 1, Input: `{"messages":[]}`},
			{ID: 201, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-201", ExecutionGeneration: 1, Input: `{"messages":[]}`},
			{ID: 202, ThreadID: 10, Status: entity.RunStatusRunning, WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-202", ExecutionGeneration: 1, Input: `{"messages":[]}`},
		},
		appended:          &entity.Message{ID: 300, ThreadID: 10, Role: entity.MessageRoleAssistant, Content: "ok"},
		completedRun:      &entity.Run{ID: 999, ThreadID: 10, Status: entity.RunStatusSucceeded},
		completeRunErrors: map[int64]error{200: fmt.Errorf("complete run infrastructure failure")},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	executed := make([]int64, 0, 3)
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		executed = append(executed, run.RunID)
		return &RunExecutionResult{Message: "ok"}, nil
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 3})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.ErrorContains(t, err, "complete run infrastructure failure")
	require.Equal(t, []int64{200, 201, 202}, executed)
	require.Equal(t, RunProcessResult{
		ClaimedRuns:   3,
		ProcessedRuns: 3,
		SucceededRuns: 2,
		ErroredRuns:   1,
	}, result)
	require.Len(t, domainSVC.completeRunReqs, 3)
	require.Len(t, domainSVC.releaseRunLeaseReqs, 1)
	require.Equal(t, int64(200), domainSVC.releaseRunLeaseReqs[0].RunID)
	require.Equal(t, entity.RunStatusPending, domainSVC.releaseRunLeaseReqs[0].ToStatus)
	require.Equal(t, "lease-200", domainSVC.releaseRunLeaseReqs[0].LeaseToken)
}

func TestRunProcessorRenewsLeaseWhileExecutionIsActive(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(1_000))
	renewCalls := make(chan *domainservice.RenewRunLeaseRequest, 1)
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 3, Input: `{"messages":[]}`,
		}},
		renewRunLeaseCalls: renewCalls,
		appended:           &entity.Message{ID: 300, ThreadID: 10, RunID: 200, Role: entity.MessageRoleAssistant, Content: "ok"},
		completedRun:       &entity.Run{ID: 200, ThreadID: 10, Status: entity.RunStatusSucceeded},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	started := make(chan struct{})
	release := make(chan struct{})
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		close(started)
		<-release
		return &RunExecutionResult{Message: "ok"}, nil
	}), RunProcessorOptions{
		WorkerID:          "worker-a",
		BatchSize:         1,
		LeaseTTL:          6 * time.Second,
		HeartbeatInterval: 2 * time.Second,
		LeaseClock:        clock,
	})
	done := make(chan error, 1)
	go func() {
		done <- processor.ProcessPendingRuns(context.Background())
	}()

	<-started
	clock.Tick(time.UnixMilli(3_000))
	renew := <-renewCalls
	require.Equal(t, int64(200), renew.RunID)
	require.Equal(t, "worker-a", renew.LeaseOwner)
	require.Equal(t, "lease-200", renew.LeaseToken)
	require.Equal(t, uint64(3), renew.ExecutionGeneration)
	require.Equal(t, int64(3_000), renew.Now)
	require.Equal(t, int64(6_000), renew.LeaseTTLMillis)
	close(release)
	require.NoError(t, <-done)
	require.Equal(t, int64(1_000), domainSVC.claimRunsReq.Now)
	require.Equal(t, int64(6_000), domainSVC.claimRunsReq.LeaseTTLMillis)
	require.Equal(t, 2*time.Second, clock.TickerInterval())
}

func TestRunProcessorStopsExecutionWhenLeaseRenewalFails(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(1_000))
	renewCalls := make(chan *domainservice.RenewRunLeaseRequest, 1)
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 3, Input: `{"messages":[]}`,
		}},
		renewRunLeaseCalls: renewCalls,
		renewRunLeaseErr:   errors.New("lease ownership lost"),
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	started := make(chan struct{})
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}), RunProcessorOptions{
		WorkerID:          "worker-a",
		BatchSize:         1,
		LeaseTTL:          6 * time.Second,
		HeartbeatInterval: 2 * time.Second,
		LeaseClock:        clock,
	})
	done := make(chan error, 1)
	go func() {
		done <- processor.ProcessPendingRuns(context.Background())
	}()

	<-started
	clock.Tick(time.UnixMilli(3_000))
	<-renewCalls
	err := <-done
	require.ErrorContains(t, err, "lease ownership lost")
	require.Nil(t, domainSVC.failRunReq)
	require.Len(t, domainSVC.releaseRunLeaseReqs, 1)
	require.Equal(t, entity.RunStatusPending, domainSVC.releaseRunLeaseReqs[0].ToStatus)
}

func TestRunProcessorTreatsLeaseLossFromDurableCancellationAsCanceled(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(1_000))
	renewCalls := make(chan *domainservice.RenewRunLeaseRequest, 1)
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 3, Input: `{"messages":[]}`,
		}},
		gotRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusCanceled,
			CancelRequestedAt: 3_000, ExecutionGeneration: 4,
		},
		renewRunLeaseCalls: renewCalls,
		renewRunLeaseErr:   domainrepo.ErrRunLeaseLost,
	}
	started := make(chan struct{})
	causeCh := make(chan error, 1)
	processor := NewRunProcessor(
		&ApplicationService{ThreadSVC: domainSVC},
		RunExecutorFunc(func(ctx context.Context, _ *RunSummary) (*RunExecutionResult, error) {
			close(started)
			<-ctx.Done()
			causeCh <- context.Cause(ctx)
			return nil, ctx.Err()
		}),
		RunProcessorOptions{
			WorkerID:          "worker-a",
			BatchSize:         1,
			LeaseTTL:          6 * time.Second,
			HeartbeatInterval: 2 * time.Second,
			LeaseClock:        clock,
		},
	)
	done := make(chan struct {
		result RunProcessResult
		err    error
	}, 1)
	go func() {
		result, err := processor.ProcessPendingRunsWithResult(context.Background())
		done <- struct {
			result RunProcessResult
			err    error
		}{result: result, err: err}
	}()

	<-started
	clock.Tick(time.UnixMilli(3_000))
	<-renewCalls
	require.ErrorIs(t, <-causeCh, domainrepo.ErrRunLeaseLost)
	got := <-done
	require.NoError(t, got.err)
	require.Equal(t, 1, got.result.CanceledRuns)
	require.Empty(t, domainSVC.releaseRunLeaseReqs)
	require.Nil(t, domainSVC.failRunReq)
}

func TestRunProcessorShutdownReleasesActiveLeaseWithoutFailingRun(t *testing.T) {
	clock := newManualRunLeaseClock(time.UnixMilli(1_000))
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 3, Input: `{"messages":[]}`,
		}},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	started := make(chan struct{})
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}), RunProcessorOptions{
		WorkerID:          "worker-a",
		BatchSize:         1,
		LeaseTTL:          6 * time.Second,
		HeartbeatInterval: 2 * time.Second,
		LeaseClock:        clock,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- processor.ProcessPendingRuns(ctx)
	}()

	<-started
	cancel()
	err := <-done
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, domainSVC.failRunReq)
	require.Len(t, domainSVC.releaseRunLeaseReqs, 1)
	require.Equal(t, int64(200), domainSVC.releaseRunLeaseReqs[0].RunID)
	require.Equal(t, entity.RunStatusPending, domainSVC.releaseRunLeaseReqs[0].ToStatus)
}

func TestRunProcessorReportsProcessResultWhenCompleteRunFails(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "ok",
		},
		completeRunErr: fmt.Errorf("complete run failed"),
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
	})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.ErrorContains(t, err, "complete run failed")
	require.NotNil(t, domainSVC.appendReq)
	require.NotNil(t, domainSVC.completeRunReq)
	require.Nil(t, domainSVC.failRunReq)
	require.Equal(t, RunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		ErroredRuns:   1,
	}, result)
}

type manualRunLeaseClock struct {
	mu       sync.Mutex
	now      time.Time
	ticker   *manualRunLeaseTicker
	interval time.Duration
}

func newManualRunLeaseClock(now time.Time) *manualRunLeaseClock {
	return &manualRunLeaseClock{now: now}
}

func (c *manualRunLeaseClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualRunLeaseClock) NewTicker(interval time.Duration) RunLeaseTicker {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interval = interval
	c.ticker = &manualRunLeaseTicker{ch: make(chan time.Time, 4)}
	return c.ticker
}

func (c *manualRunLeaseClock) Tick(now time.Time) {
	c.mu.Lock()
	c.now = now
	ticker := c.ticker
	c.mu.Unlock()
	if ticker == nil {
		panic("run lease ticker is not initialized")
	}
	ticker.Tick(now)
}

func (c *manualRunLeaseClock) TickerInterval() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interval
}

type manualRunLeaseTicker struct {
	mu      sync.Mutex
	ch      chan time.Time
	stopped bool
}

func (t *manualRunLeaseTicker) C() <-chan time.Time {
	return t.ch
}

func (t *manualRunLeaseTicker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.stopped = true
}

func (t *manualRunLeaseTicker) Tick(now time.Time) {
	t.mu.Lock()
	stopped := t.stopped
	t.mu.Unlock()
	if !stopped {
		t.ch <- now
	}
}

func TestRunProcessorMarksRunFailedWhenExecutorErrors(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				WorkerID: "worker-a",
			},
		},
		failedRun: &entity.Run{
			ID:           200,
			ThreadID:     10,
			Status:       entity.RunStatusFailed,
			WorkerID:     "worker-a",
			ErrorCode:    "executor_error",
			ErrorMessage: "model failed",
			EndedAt:      400,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		return nil, fmt.Errorf("model failed")
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	err := processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Nil(t, domainSVC.appendReq)
	require.Nil(t, domainSVC.completeRunReq)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, int64(200), domainSVC.failRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.failRunReq.From)
	require.Equal(t, "worker-a", domainSVC.failRunReq.WorkerID)
	require.Equal(t, "executor_error", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "model failed", domainSVC.failRunReq.ErrorMessage)
	require.Equal(t, []string{"run.started", "run.failed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[1].Payload, `"status":"failed"`)
	require.Contains(t, eventSink.events[1].Payload, `"error_code":"executor_error"`)
	require.Contains(t, eventSink.events[1].Payload, `"error_message":"model failed"`)
}

func TestRunProcessorFailsSubagentRetryCommandBeforeExecutorSupport(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				Command:  `{"subagent_retry":{"schema":"coze.subagent_retry.v1","source_run_id":20,"parent_run_id":10}}`,
				WorkerID: "worker-a",
			},
		},
		failedRun: &entity.Run{
			ID:           200,
			ThreadID:     10,
			Status:       entity.RunStatusFailed,
			WorkerID:     "worker-a",
			ErrorCode:    "subagent_retry_not_supported",
			ErrorMessage: "subagent retry executor is not implemented",
			EndedAt:      400,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	executed := false
	processor := NewRunProcessor(app, RunExecutorFunc(func(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
		executed = true

		return &RunExecutionResult{Message: "should not execute"}, nil
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.False(t, executed)
	require.Nil(t, domainSVC.appendReq)
	require.Nil(t, domainSVC.completeRunReq)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, int64(200), domainSVC.failRunReq.RunID)
	require.Equal(t, entity.RunStatusRunning, domainSVC.failRunReq.From)
	require.Equal(t, "worker-a", domainSVC.failRunReq.WorkerID)
	require.Equal(t, "subagent_retry_not_supported", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "subagent retry executor is not implemented", domainSVC.failRunReq.ErrorMessage)
	require.Equal(t, RunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		FailedRuns:    1,
	}, result)
	require.Equal(t, []string{"run.started", "run.failed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[1].Payload, `"status":"failed"`)
	require.Contains(t, eventSink.events[1].Payload, `"error_code":"subagent_retry_not_supported"`)
	require.Contains(t, eventSink.events[1].Payload, `"error_message":"subagent retry executor is not implemented"`)
	require.NotContains(t, eventSink.events[1].Payload, "source_run_id")
	require.NotContains(t, eventSink.events[1].Payload, "parent_run_id")
}

func TestRunProcessorDispatchesSubagentRetryCommandToCapableExecutor(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				Command:  `{"subagent_retry":{"schema":"coze.subagent_retry.v1","source_run_id":20,"parent_run_id":10}}`,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "子智能体重试已完成",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	executor := &recordingSubagentRetryRunExecutor{
		retryResult: &RunExecutionResult{
			Message:  "子智能体重试已完成",
			Metadata: `{"source":"subagent_retry_replay"}`,
		},
	}
	processor := NewRunProcessor(app, executor, RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.False(t, executor.executeCalled)
	require.True(t, executor.retryExecuteCalled)
	require.Equal(t, int64(200), executor.retryRun.RunID)
	require.Nil(t, domainSVC.failRunReq)
	require.Equal(t, int64(200), domainSVC.appendReq.RunID)
	require.Equal(t, "子智能体重试已完成", domainSVC.appendReq.Content)
	require.Equal(t, `{"source":"subagent_retry_replay"}`, domainSVC.appendReq.Metadata)
	require.NotNil(t, domainSVC.completeRunReq)
	require.Equal(t, RunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		SucceededRuns: 1,
	}, result)
	require.Equal(t, []string{"run.started", "run.completed"}, eventSink.eventTypes())
}

func TestRunProcessorMapsUnsupportedSubagentRetryExecutorToFixedFailure(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    `{"messages":[]}`,
				Command:  `{"subagent_retry":{"schema":"coze.subagent_retry.v1","source_run_id":20,"parent_run_id":10}}`,
				WorkerID: "worker-a",
			},
		},
		failedRun: &entity.Run{
			ID:           200,
			ThreadID:     10,
			Status:       entity.RunStatusFailed,
			WorkerID:     "worker-a",
			ErrorCode:    "subagent_retry_not_supported",
			ErrorMessage: "subagent retry executor is not implemented",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(app, unsupportedSubagentRetryRunExecutor{}, RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, "subagent_retry_not_supported", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "subagent retry executor is not implemented", domainSVC.failRunReq.ErrorMessage)
	require.Equal(t, RunProcessResult{
		ClaimedRuns:   1,
		ProcessedRuns: 1,
		FailedRuns:    1,
	}, result)
	require.Equal(t, []string{"run.started", "run.failed"}, eventSink.eventTypes())
	require.Contains(t, eventSink.events[1].Payload, `"error_code":"subagent_retry_not_supported"`)
	require.NotContains(t, eventSink.events[1].Payload, "executor_error")
}

func TestRunProcessorMarksADKInterruptWithoutFailingRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusRunning,
			WorkerID: "worker-a",
		}},
		interruptedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusInterrupted,
			WorkerID: "worker-a",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(app, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, &RunInterruptedError{
			CheckpointKey: "coze-run-200",
			Interrupts: []ADKInterruptItem{{
				ID:          "approval",
				Address:     "agent:lead;tool:approval",
				IsRootCause: true,
			}},
		}
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.InterruptedRuns)
	require.NotNil(t, domainSVC.interruptRunReq)
	require.Nil(t, domainSVC.failRunReq)
	require.Nil(t, domainSVC.appendReq)
	require.Equal(t, []string{"run.started", "run.interrupted"}, eventSink.eventTypes())
}

func TestRunProcessorDoesNotFailCanceledADKRun(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusRunning,
			WorkerID: "worker-a",
		}},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(app, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, &RunCanceledError{EventPersisted: true}
	}), RunProcessorOptions{
		WorkerID:  "worker-a",
		BatchSize: 1,
		EventSink: eventSink,
	})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.CanceledRuns)
	require.Nil(t, domainSVC.failRunReq)
	require.Nil(t, domainSVC.appendReq)
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
}

type recordingRunEventSink struct {
	mu     sync.Mutex
	events []RunEvent
}

func (s *recordingRunEventSink) EmitRunEvent(ctx context.Context, event RunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)

	return nil
}

func (s *recordingRunEventSink) eventTypes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	eventTypes := make([]string, 0, len(s.events))
	for _, event := range s.events {
		eventTypes = append(eventTypes, event.EventType)
	}

	return eventTypes
}

type recordingSubagentRetryRunExecutor struct {
	executeCalled      bool
	retryExecuteCalled bool
	executeRun         *RunSummary
	retryRun           *RunSummary
	executeResult      *RunExecutionResult
	retryResult        *RunExecutionResult
	executeErr         error
	retryErr           error
}

func (e *recordingSubagentRetryRunExecutor) Execute(
	_ context.Context,
	run *RunSummary,
) (*RunExecutionResult, error) {
	e.executeCalled = true
	e.executeRun = run

	return e.executeResult, e.executeErr
}

func (e *recordingSubagentRetryRunExecutor) ExecuteSubagentRetry(
	_ context.Context,
	run *RunSummary,
) (*RunExecutionResult, error) {
	e.retryExecuteCalled = true
	e.retryRun = run

	return e.retryResult, e.retryErr
}

type unsupportedSubagentRetryRunExecutor struct{}

func (unsupportedSubagentRetryRunExecutor) Execute(
	context.Context,
	*RunSummary,
) (*RunExecutionResult, error) {
	return &RunExecutionResult{Message: "ordinary execution"}, nil
}

func (unsupportedSubagentRetryRunExecutor) ExecuteSubagentRetry(
	context.Context,
	*RunSummary,
) (*RunExecutionResult, error) {
	return nil, &SubagentRetryUnsupportedError{}
}

type recordingRunTitleGenerator struct {
	title string
	err   error
	calls int
	input RunTitleGenerationInput
}

func (g *recordingRunTitleGenerator) GenerateTitle(
	_ context.Context,
	input RunTitleGenerationInput,
) (string, error) {
	g.calls++
	g.input = input

	return g.title, g.err
}
