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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	appnotification "github.com/coze-dev/coze-studio/backend/application/notification"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

func TestClassifyRunMainFlowErrorUsesSafeStableCategories(t *testing.T) {
	require.Equal(t, "none", classifyRunMainFlowError(nil))
	require.Equal(
		t,
		"mcp_auth_unavailable",
		classifyRunMainFlowError(errors.New(
			"resolve eino adk tool set: mcp tool auth decode failed: raw-secret",
		)),
	)
	require.Equal(
		t,
		"toolset_resolution_failed",
		classifyRunMainFlowError(errors.New(
			"resolve eino adk tool set: provider unavailable",
		)),
	)
	require.Equal(
		t,
		"executor_error",
		classifyRunMainFlowError(errors.New("raw-secret provider response")),
	)
	require.Equal(t, "finalized_failure", classifyRunMainFlowOutcome(runProcessFailed, nil))
	require.Equal(t, "none", classifyRunMainFlowOutcome(runProcessSucceeded, nil))
	require.Equal(
		t,
		"mcp_auth_unavailable",
		classifyRunMainFlowOutcome(
			runProcessErrored,
			errors.New("resolve eino adk tool set: mcp tool auth decode failed: raw-secret"),
		),
	)
}

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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
	require.Equal(t, int64(10), eventSink.events[0].ThreadID)
	require.Equal(t, int64(200), eventSink.events[0].RunID)
	require.Contains(t, eventSink.events[0].Payload, `"status":"running"`)
	require.Contains(t, eventSink.events[0].Payload, `"worker_id":"worker-a"`)
	require.Contains(t, domainSVC.finalizeRunSuccessReq.CompletionEventPayload, `"status":"succeeded"`)
}

func TestRunProcessorBuildsTerminalParityCheckpoint(t *testing.T) {
	domainSVC := &recordingThreadService{
		got: &entity.Thread{ID: 10, SpaceID: 7, CreatorID: 9, Title: "Existing title"},
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, SpaceID: 7, CreatorID: 9,
			Status:   entity.RunStatusRunning,
			Input:    `{"messages":[{"role":"user","content":"continue"}]}`,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			LeaseExpiresAt: 60_000, ExecutionGeneration: 3,
		}},
		appended: &entity.Message{
			ID: 300, ThreadID: 10, RunID: 200,
			Role: entity.MessageRoleAssistant, Content: "done",
		},
		completedRun: &entity.Run{
			ID: 200, ThreadID: 10, SpaceID: 7, CreatorID: 9,
			Status: entity.RunStatusSucceeded,
		},
	}
	tracker, err := NewADKParityStateTracker(&RunSummary{
		RunID: 200, ThreadID: 10, SpaceID: 7, CreatorID: 9,
	}, nil)
	require.NoError(t, err)
	require.NoError(t, tracker.SetTitle("Existing title"))
	require.NoError(t, tracker.AppendMessage(ADKParityMessage{
		RunID: 200, Role: "assistant", Content: "done",
	}))
	require.NoError(t, tracker.SetCompletion(ADKParityCompletion{
		RunID: 200, Status: "succeeded", Reason: "completed", CompletedAt: 400,
	}))
	state := tracker.Snapshot()
	processor := NewRunProcessor(
		&ApplicationService{ThreadSVC: domainSVC},
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{
				Message: "done", Metadata: `{"source":"eino_adk"}`,
				ParityState: &state, ParityParentCheckpointID: 77,
			}, nil
		}),
		RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1},
	)

	require.NoError(t, processor.ProcessPendingRuns(context.Background()))
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	checkpoint := domainSVC.finalizeRunSuccessReq.TerminalCheckpoint
	require.NotNil(t, checkpoint)
	require.Equal(t, int64(77), checkpoint.ParentCheckpointID)
	require.Equal(t, "coze-run-200", checkpoint.RuntimeKey)
	require.Equal(t, int32(2), checkpoint.EnvelopeVersion)
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, ADKCheckpointPhaseTerminal, envelope.CheckpointPhase)
	require.Empty(t, envelope.Checkpoint)
	require.Equal(t, "Existing title", envelope.ParityState.Title)
	require.Equal(t, "succeeded", envelope.ParityState.Completion.Status)
}

func TestRunProcessorBuildsTitleConflictFallbackTerminalParityCheckpoint(t *testing.T) {
	userMessage := "请制定青岛三日游路线"
	initialTitle := taskThreadTitle("", userMessage)
	domainSVC := &recordingThreadService{
		got: &entity.Thread{ID: 10, SpaceID: 7, CreatorID: 9, Title: initialTitle},
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, SpaceID: 7, CreatorID: 9,
			Status: entity.RunStatusRunning, Input: `{"messages":[{"role":"user","content":"请制定青岛三日游路线"}]}`,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			LeaseExpiresAt: 60_000, ExecutionGeneration: 3,
		}},
		appended: &entity.Message{
			ID: 300, ThreadID: 10, RunID: 200, Role: entity.MessageRoleAssistant, Content: "done",
		},
		completedRun: &entity.Run{
			ID: 200, ThreadID: 10, SpaceID: 7, CreatorID: 9, Status: entity.RunStatusSucceeded,
		},
	}
	tracker, err := NewADKParityStateTracker(&RunSummary{
		RunID: 200, ThreadID: 10, SpaceID: 7, CreatorID: 9,
	}, nil)
	require.NoError(t, err)
	require.NoError(t, tracker.SetTitle(initialTitle))
	state := tracker.Snapshot()
	processor := NewRunProcessor(
		&ApplicationService{ThreadSVC: domainSVC},
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{
				Message: "done", Title: "青岛亲子三日游",
				ParityState: &state, ParityParentCheckpointID: 77,
			}, nil
		}),
		RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1},
	)

	require.NoError(t, processor.ProcessPendingRuns(context.Background()))
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	primary := domainSVC.finalizeRunSuccessReq.TerminalCheckpoint
	fallback := domainSVC.finalizeRunSuccessReq.TerminalCheckpointOnTitleConflict
	require.NotNil(t, primary)
	require.NotNil(t, fallback)
	primaryEnvelope, err := UnmarshalADKCheckpointEnvelope([]byte(primary.ChannelValues))
	require.NoError(t, err)
	fallbackEnvelope, err := UnmarshalADKCheckpointEnvelope([]byte(fallback.ChannelValues))
	require.NoError(t, err)
	require.Equal(t, "青岛亲子三日游", primaryEnvelope.ParityState.Title)
	require.Empty(t, fallbackEnvelope.ParityState.Title)
	require.Equal(t, primary.ParentCheckpointID, fallback.ParentCheckpointID)
	require.Equal(t, primary.RuntimeKey, fallback.RuntimeKey)
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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
	require.Contains(t, domainSVC.finalizeRunSuccessReq.TitleEventPayload, `"thread_title":"武汉3日游攻略"`)
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
	require.Contains(t, domainSVC.finalizeRunSuccessReq.TitleEventPayload, `"thread_title":"青岛亲子旅行路线"`)
	require.NotContains(t, domainSVC.finalizeRunSuccessReq.TitleEventPayload, "think")
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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
}

func TestRunProcessorGeneratedThreadTitleUsesCleanFallbacksAndRunConfig(t *testing.T) {
	const skillGuide = "我想创建一个技能，请先询问我技能用途、使用场景和期望输出。"

	t.Run("generator error uses active skill fallback", func(t *testing.T) {
		processor := &RunProcessor{
			titleGenerator: &recordingRunTitleGenerator{err: errors.New("title model unavailable")},
		}
		run := &RunSummary{
			Config: `{"enable_skills":["skill-creator"]}`,
		}

		title := processor.generatedThreadTitle(
			context.Background(),
			run,
			skillGuide,
			&RunExecutionResult{Message: "请补充技能信息"},
		)

		require.Equal(t, "创建技能", title)
		require.NotContains(t, title, "@skill-creator")
	})

	t.Run("empty generator result uses clean fallback", func(t *testing.T) {
		processor := &RunProcessor{
			titleGenerator: &recordingRunTitleGenerator{},
		}

		title := processor.generatedThreadTitle(
			context.Background(),
			&RunSummary{Config: `{"enable_skills":["web-search"]}`},
			"请帮我整理季度复盘。 @web-search",
			&RunExecutionResult{Message: "已整理"},
		)

		require.Equal(t, "请帮我整理季度复盘", title)
	})

	t.Run("dirty custom generator title is normalized", func(t *testing.T) {
		processor := &RunProcessor{
			titleGenerator: &recordingRunTitleGenerator{title: "季度复盘 @web-search"},
		}

		title := processor.generatedThreadTitle(
			context.Background(),
			&RunSummary{},
			"请帮我整理季度复盘",
			&RunExecutionResult{Message: "已整理"},
		)

		require.Equal(t, "季度复盘", title)
	})

	t.Run("explicit executor title keeps priority and is normalized", func(t *testing.T) {
		generator := &recordingRunTitleGenerator{title: "生成器标题"}
		processor := &RunProcessor{titleGenerator: generator}

		title := processor.generatedThreadTitle(
			context.Background(),
			&RunSummary{},
			"请帮我整理季度复盘",
			&RunExecutionResult{
				Message: "已整理",
				Title:   "执行器标题 @web-search",
			},
		)

		require.Equal(t, "执行器标题", title)
		require.Zero(t, generator.calls)
	})
}

func TestRunProcessorGeneratedThreadTitleHonorsConfiguredMaxChars(t *testing.T) {
	longTitle := "一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十一二三四五六七八九十"
	require.Len(t, []rune(longTitle), 70)
	processor := &RunProcessor{
		titleGenerator: &recordingRunTitleGenerator{title: longTitle},
	}

	title := processor.generatedThreadTitle(
		context.Background(),
		&RunSummary{Config: `{"title_generation":{"max_chars":80,"max_words":20}}`},
		"请生成长标题",
		&RunExecutionResult{Message: "已完成"},
	)

	require.Equal(t, longTitle, title)

	quotedTitle := (&RunProcessor{}).generatedThreadTitle(
		context.Background(),
		&RunSummary{Config: `{"title_generation":{"max_chars":80,"max_words":20}}`},
		"请生成《"+longTitle+"》",
		&RunExecutionResult{Message: "已完成"},
	)

	require.Equal(t, longTitle, quotedTitle)
}

func TestRunProcessorGeneratedThreadTitleNormalizesDeterministicFallback(t *testing.T) {
	title := (&RunProcessor{}).generatedThreadTitle(
		context.Background(),
		&RunSummary{Config: `{"title_generation":{"max_chars":80,"max_words":2}}`},
		"alpha beta gamma delta",
		&RunExecutionResult{Message: "done"},
	)

	require.Equal(t, "alpha beta", title)
}

func TestRunProcessorSkipsRedundantCleanFallbackTitleUpdate(t *testing.T) {
	userMessage := "我想创建一个技能，请先询问我技能用途、使用场景和期望输出。 @skill-creator"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	initialTitle := taskThreadTitle("", userMessage)
	require.Equal(t, "创建技能", initialTitle)
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
				Config:   `{"enable_skills":["skill-creator"]}`,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "请补充技能信息",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	eventSink := &recordingRunEventSink{}
	processor := NewRunProcessor(
		&ApplicationService{ThreadSVC: domainSVC},
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "请补充技能信息"}, nil
		}),
		RunProcessorOptions{
			WorkerID:       "worker-a",
			BatchSize:      1,
			EventSink:      eventSink,
			TitleGenerator: &recordingRunTitleGenerator{err: errors.New("title model unavailable")},
		},
	)

	err = processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Nil(t, domainSVC.updateThreadTitleReq)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	require.JSONEq(t, `{"thread_title":""}`, domainSVC.finalizeRunSuccessReq.TitleEventPayload)
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
}

func TestRunProcessorRecognizesRunConfiguredProvisionalTitle(t *testing.T) {
	const runConfig = `{"enable_skills":["web-search"]}`
	userMessage := "请帮我整理季度复盘。 @web-search"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	initialTitle := taskThreadTitleWithRunConfig("", userMessage, runConfig)
	require.Equal(t, "请帮我整理季度复盘", initialTitle)
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
				Config:   runConfig,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "季度复盘已整理",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	processor := NewRunProcessor(
		&ApplicationService{ThreadSVC: domainSVC},
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "季度复盘已整理"}, nil
		}),
		RunProcessorOptions{
			WorkerID:       "worker-a",
			BatchSize:      1,
			TitleGenerator: &recordingRunTitleGenerator{title: "季度复盘"},
		},
	)

	err = processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.NotNil(t, domainSVC.updateThreadTitleReq)
	require.Equal(t, "季度复盘", domainSVC.updateThreadTitleReq.Title)
	require.Contains(t, domainSVC.finalizeRunSuccessReq.TitleEventPayload, `"thread_title":"季度复盘"`)
}

func TestRunProcessorKeepsManualTitleWithRunConfiguredResources(t *testing.T) {
	const runConfig = `{"enable_skills":["web-search"]}`
	userMessage := "请帮我整理季度复盘。 @web-search"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	generator := &recordingRunTitleGenerator{title: "季度复盘"}
	domainSVC := &recordingThreadService{
		got: &entity.Thread{
			ID:    10,
			Title: "手动命名的季度任务",
		},
		claimedRuns: []*entity.Run{
			{
				ID:       200,
				ThreadID: 10,
				Status:   entity.RunStatusRunning,
				Input:    input,
				Config:   runConfig,
				WorkerID: "worker-a",
			},
		},
		appended: &entity.Message{
			ID:       300,
			ThreadID: 10,
			RunID:    200,
			Role:     entity.MessageRoleAssistant,
			Content:  "季度复盘已整理",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	processor := NewRunProcessor(
		&ApplicationService{ThreadSVC: domainSVC},
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "季度复盘已整理"}, nil
		}),
		RunProcessorOptions{
			WorkerID:       "worker-a",
			BatchSize:      1,
			TitleGenerator: generator,
		},
	)

	err = processor.ProcessPendingRuns(context.Background())

	require.NoError(t, err)
	require.Zero(t, generator.calls)
	require.Nil(t, domainSVC.updateThreadTitleReq)
	require.JSONEq(t, `{"thread_title":""}`, domainSVC.finalizeRunSuccessReq.TitleEventPayload)
}

func TestRunProcessorTitleGeneratorTimeoutDoesNotBlockFinalization(t *testing.T) {
	userMessage := "请帮我整理季度复盘"
	input, err := taskThreadRunInputFromMessage(userMessage)
	require.NoError(t, err)
	initialTitle := taskThreadTitle("", userMessage)
	generator := &blockingRunTitleGenerator{}
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
			Content:  "季度复盘已整理",
		},
		completedRun: &entity.Run{
			ID:       200,
			ThreadID: 10,
			Status:   entity.RunStatusSucceeded,
			WorkerID: "worker-a",
		},
	}
	processor := NewRunProcessor(
		&ApplicationService{ThreadSVC: domainSVC},
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "季度复盘已整理"}, nil
		}),
		RunProcessorOptions{
			WorkerID:               "worker-a",
			BatchSize:              1,
			TitleGenerator:         generator,
			TitleGenerationTimeout: 10 * time.Millisecond,
		},
	)
	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = processor.ProcessPendingRuns(parentCtx)

	require.NoError(t, err)
	require.ErrorIs(t, generator.ctxErr, context.DeadlineExceeded)
	require.NoError(t, parentCtx.Err())
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	require.Nil(t, domainSVC.updateThreadTitleReq)
	require.JSONEq(t, `{"thread_title":""}`, domainSVC.finalizeRunSuccessReq.TitleEventPayload)
}

func TestRunProcessorUsesProductionTitleGenerationTimeoutByDefault(t *testing.T) {
	processor := NewRunProcessor(nil, nil, RunProcessorOptions{})

	require.Equal(t, 8*time.Second, processor.titleGenerationTimeout)
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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
	require.Contains(t, domainSVC.failRunReq.EventPayload, `"status":"failed"`)
	require.Contains(t, domainSVC.failRunReq.EventPayload, `"error_code":"executor_error"`)
	require.NotContains(t, domainSVC.failRunReq.EventPayload, "model failed")
}

func TestRunProcessorAttachesTerminalOutboxIntentForSuccessfulRun(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	domainRun := &entity.Run{
		ID: 200, ThreadID: 10, SpaceID: 1, CreatorID: 2,
		Status: entity.RunStatusRunning, Input: `{"messages":[]}`,
		Metadata: `{}`,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
		ExecutionGeneration: 3, UpdatedAt: 1_000,
	}
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{domainRun},
		gotRunsByID: map[int64]*entity.Run{200: domainRun},
		got:         &entity.Thread{ID: 10, Title: "生成周报"},
	}
	processor := NewRunProcessor(&ApplicationService{ThreadSVC: domainSVC}, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.SucceededRuns)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq.OutboxIntent)
	event := domainSVC.finalizeRunSuccessReq.OutboxIntent.Event
	require.Equal(t, domainnotification.EventTaskCompleted, event.EventType)
	require.Equal(t, int64(2), event.ActorID)
	require.Equal(t, int64(1), event.SpaceID)
	require.Equal(t, "thread:10", event.Payload.TargetID)
	require.Equal(t, "生成周报", event.Payload.ResourceDisplayName)
}

func TestRunProcessorDowngradesUnsafeTerminalNotificationTitle(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	domainRun := &entity.Run{
		ID: 200, ThreadID: 10, SpaceID: 1, CreatorID: 2,
		Status: entity.RunStatusRunning, Input: `{"messages":[]}`,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
		ExecutionGeneration: 3, UpdatedAt: 1_000,
	}
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{domainRun},
		gotRunsByID: map[int64]*entity.Run{200: domainRun},
		got:         &entity.Thread{ID: 10, Title: "prompt api_key 排查"},
	}
	processor := NewRunProcessor(&ApplicationService{ThreadSVC: domainSVC}, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "ok"}, nil
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.SucceededRuns)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq.OutboxIntent)
	event := domainSVC.finalizeRunSuccessReq.OutboxIntent.Event
	require.Equal(t, domainnotification.EventTaskCompleted, event.EventType)
	require.Equal(t, "任务", event.Payload.ResourceDisplayName)
	require.NoError(t, domainnotification.DefaultTemplateRegistry().ValidateAppendable(event))
}

func TestRunProcessorAttachesTerminalOutboxIntentForFinalFailure(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	domainRun := &entity.Run{
		ID: 200, ThreadID: 10, SpaceID: 1, CreatorID: 2,
		Status: entity.RunStatusRunning, Input: `{"messages":[]}`,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
		ExecutionGeneration: 3, UpdatedAt: 1_000,
	}
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{domainRun},
		gotRunsByID: map[int64]*entity.Run{200: domainRun},
		got:         &entity.Thread{ID: 10, Title: "生成周报"},
		failedRun:   &entity.Run{ID: 200, ThreadID: 10, Status: entity.RunStatusFailed},
	}
	processor := NewRunProcessor(&ApplicationService{ThreadSVC: domainSVC}, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, fmt.Errorf("model failed")
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.FailedRuns)
	require.NotNil(t, domainSVC.failRunReq)
	require.NotNil(t, domainSVC.failRunReq.OutboxIntent)
	event := domainSVC.failRunReq.OutboxIntent.Event
	require.Equal(t, domainnotification.EventTaskFailed, event.EventType)
	require.Equal(t, int64(2), event.ActorID)
	require.Equal(t, int64(1), event.SpaceID)
	require.Equal(t, "thread:10", event.Payload.TargetID)
	require.Equal(t, "生成周报", event.Payload.ResourceDisplayName)
}

func TestRunProcessorSuppressesTerminalOutboxIntentForScheduledTaskOrigin(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	domainRun := &entity.Run{
		ID: 200, ThreadID: 10, SpaceID: 1, CreatorID: 2,
		Status: entity.RunStatusRunning, Input: `{"messages":[]}`,
		Metadata: `{"source":"scheduled_task","scheduled_task_id":"10"}`,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
		ExecutionGeneration: 3, UpdatedAt: 1_000,
	}
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{domainRun},
		gotRunsByID: map[int64]*entity.Run{200: domainRun},
		got:         &entity.Thread{ID: 10, Title: "生成周报"},
		failedRun:   &entity.Run{ID: 200, ThreadID: 10, Status: entity.RunStatusFailed},
	}
	processor := NewRunProcessor(&ApplicationService{ThreadSVC: domainSVC}, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, fmt.Errorf("model failed")
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.FailedRuns)
	require.NotNil(t, domainSVC.failRunReq)
	require.Nil(t, domainSVC.failRunReq.OutboxIntent)
}

func TestRunProcessorAttachesAwaitingInputOutboxForExactHumanInteractionInterrupt(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	domainRun := &entity.Run{
		ID: 200, ThreadID: 10, SpaceID: 1, CreatorID: 2,
		Status: entity.RunStatusRunning, Input: `{"messages":[]}`,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
		ExecutionGeneration: 3, UpdatedAt: 1_000,
	}
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{domainRun},
		gotRunsByID: map[int64]*entity.Run{200: domainRun},
		got:         &entity.Thread{ID: 10, Title: "补充需求"},
		interruptedRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusInterrupted,
		},
	}
	processor := NewRunProcessor(&ApplicationService{ThreadSVC: domainSVC}, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, &RunInterruptedError{
			CheckpointKey: "coze-run-200",
			Interrupts: []ADKInterruptItem{{
				ID:      "interrupt-event-1",
				Address: "agent:lead;tool:request_human_clarification",
				Info: HumanInteractionPrompt{
					Schema:        humanInteractionSchema,
					InteractionID: "hi_1",
					Kind:          HumanInteractionKindClarification,
					Title:         "需要补充信息",
					Question:      "要分析哪个城市？",
					Required:      true,
					AllowFreeText: true,
					RiskLevel:     HumanInteractionRiskNone,
					ToolName:      adkClarificationToolName,
				},
				IsRootCause: true,
			}},
		}
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.InterruptedRuns)
	require.NotNil(t, domainSVC.interruptRunReq)
	require.Contains(t, domainSVC.interruptRunReq.EventPayload, `"awaiting_input"`)
	require.Contains(t, domainSVC.interruptRunReq.EventPayload, `"interaction_event_id":"interrupt-event-1"`)
	require.NotContains(t, domainSVC.interruptRunReq.EventPayload, "要分析哪个城市")
	require.NotNil(t, domainSVC.interruptRunReq.OutboxIntent)
	event := domainSVC.interruptRunReq.OutboxIntent.Event
	require.Equal(t, domainnotification.EventTaskAwaitingInput, event.EventType)
	require.Equal(t, "interaction-event:interrupt-event-1:task.awaiting_input", event.EventID)
	require.Equal(t, "agent_run_interaction", event.AggregateType)
	require.Equal(t, "interaction:interrupt-event-1", event.AggregateID)
	require.Equal(t, int64(1), event.AggregateVersion)
	require.Equal(t, int64(2), event.ActorID)
	require.Equal(t, int64(1), event.SpaceID)
	require.Equal(t, domainnotification.StatusReasonActionRequired, event.Payload.StatusReasonCode)
	require.Equal(t, "thread:10", event.Payload.TargetID)
	require.Equal(t, "补充需求", event.Payload.ResourceDisplayName)
	require.NoError(t, domainnotification.DefaultTemplateRegistry().ValidateAppendable(event))
	replayed := event
	replayed.OccurredAt = time.UnixMilli(9_999)
	require.Equal(t, event.IdempotencyKey(), replayed.IdempotencyKey())
	otherInteraction := event
	otherInteraction.AggregateID = "interaction:interrupt-event-2"
	require.NotEqual(t, event.IdempotencyKey(), otherInteraction.IdempotencyKey())
}

func TestAwaitingInputOutboxAcceptsMaximumLegalInteractionEventID(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	interactionEventID := strings.Repeat("a", 90)
	domainRun := &entity.Run{
		ID: 200, ThreadID: 10, SpaceID: 1, CreatorID: 2,
		Status: entity.RunStatusRunning,
	}
	domainSVC := &recordingThreadService{
		gotRunsByID: map[int64]*entity.Run{200: domainRun},
		got:         &entity.Thread{ID: 10, Title: "最长交互 ID"},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	payload := encodeRunEventPayload(context.Background(), map[string]any{
		"status": string(RunStatusInterrupted),
		entity.RunAwaitingInputInteractionRefPayloadKey: entity.RunAwaitingInputInteractionRef{
			Schema:             entity.RunAwaitingInputInteractionRefSchema,
			InteractionEventID: interactionEventID,
			InteractionID:      "hi_max",
			Kind:               "clarification",
		},
	})

	intent := app.agentRunAwaitingInputOutboxIntent(context.Background(), 200, payload, 2_000)

	require.NotNil(t, intent)
	require.Len(t, []rune(intent.Event.EventID), 128)
	require.NoError(t, domainnotification.DefaultTemplateRegistry().ValidateAppendable(intent.Event))
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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
	require.Contains(t, domainSVC.failRunReq.EventPayload, `"status":"failed"`)
	require.Contains(t, domainSVC.failRunReq.EventPayload, `"error_code":"subagent_retry_not_supported"`)
	require.NotContains(t, domainSVC.failRunReq.EventPayload, "subagent retry executor is not implemented")
	require.NotContains(t, domainSVC.failRunReq.EventPayload, "source_run_id")
	require.NotContains(t, domainSVC.failRunReq.EventPayload, "parent_run_id")
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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
}

func TestSubagentRetryPublicCommandRunsThroughProductionWorker(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadTableForTest(db))
	app := InitService(&ServiceComponents{
		DB: db, IDGen: &sequentialTranscriptIDGen{next: 1},
	})

	threadResp, err := app.CreateThread(context.Background(), &CreateThreadRequest{
		SpaceID: 1, UserID: 2, Title: "子智能体重试验收",
	})
	require.NoError(t, err)
	parentResp, err := app.CreateRun(context.Background(), &CreateRunRequest{
		ThreadID: threadResp.Thread.ThreadID,
		Input:    `{"messages":[{"role":"user","content":"分析资料"}]}`,
	})
	require.NoError(t, err)
	childResp, err := app.createRun(context.Background(), &CreateRunRequest{
		ThreadID:    threadResp.Thread.ThreadID,
		ParentRunID: parentResp.Run.RunID,
		RunKind:     RunKindSubagent,
		Input:       `{"messages":[{"role":"user","content":"检索资料"}]}`,
	}, createRunServerOwnedSubagent)
	require.NoError(t, err)
	require.NoError(t, db.Table("agent_runs").Where("id = ?", parentResp.Run.RunID).
		Updates(map[string]any{"status": string(RunStatusRunning)}).Error)
	require.NoError(t, db.Table("agent_runs").Where("id = ?", childResp.Run.RunID).
		Updates(map[string]any{
			"status": string(RunStatusFailed), "error_code": "tool_failed",
		}).Error)

	retryResp, err := app.RetrySubagentRun(context.Background(), &RetrySubagentRunRequest{
		ThreadID: threadResp.Thread.ThreadID, SourceRunID: childResp.Run.RunID,
	})
	require.NoError(t, err)
	require.Equal(t, RunStatusQueued, retryResp.Run.Status)

	executor := &recordingSubagentRetryRunExecutor{retryResult: &RunExecutionResult{
		Message: "子智能体重试已完成", Metadata: `{"source":"subagent_retry_replay"}`,
	}}
	processor := NewRunProcessor(app, executor, RunProcessorOptions{
		WorkerID: "worker-a", BatchSize: 1,
	})
	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.True(t, executor.retryExecuteCalled)
	require.Equal(t, retryResp.Run.RunID, executor.retryRun.RunID)
	require.Equal(t, 1, result.SucceededRuns)
	persisted, err := app.GetRun(context.Background(), &GetRunRequest{RunID: retryResp.Run.RunID})
	require.NoError(t, err)
	require.Equal(t, RunStatusSucceeded, persisted.Run.Status)

	replayed, err := app.RetrySubagentRun(context.Background(), &RetrySubagentRunRequest{
		ThreadID: threadResp.Thread.ThreadID, SourceRunID: childResp.Run.RunID,
	})
	require.NoError(t, err)
	require.Equal(t, retryResp.Run.RunID, replayed.Run.RunID)
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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
	require.Contains(t, domainSVC.failRunReq.EventPayload, `"error_code":"subagent_retry_not_supported"`)
	require.NotContains(t, domainSVC.failRunReq.EventPayload, "executor_error")
}

func TestRunProcessorMarksADKInterruptWithoutFailingRun(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
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
	require.Equal(t, []string{"run.started"}, eventSink.eventTypes())
	require.Contains(t, domainSVC.interruptRunReq.EventPayload, `"status":"interrupted"`)
	require.Contains(t, domainSVC.interruptRunReq.EventPayload, `"checkpoint_key":"coze-run-200"`)
	require.NotContains(t, domainSVC.interruptRunReq.EventPayload, `"awaiting_input"`)
	require.Nil(t, domainSVC.interruptRunReq.OutboxIntent)
	require.False(t, domainSVC.interruptRunReq.EventAlreadyPersisted)
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

func TestRunProcessorAttachesTerminalOutboxIntentForExplicitCancellation(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	domainRun := &entity.Run{
		ID: 200, ThreadID: 10, SpaceID: 1, CreatorID: 2,
		Status: entity.RunStatusRunning, Input: `{"messages":[]}`,
		WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
		ExecutionGeneration: 3, UpdatedAt: 1_000,
	}
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{domainRun},
		gotRunsByID: map[int64]*entity.Run{200: domainRun},
		got:         &entity.Thread{ID: 10, Title: "生成周报"},
		canceledRun: &entity.Run{ID: 200, ThreadID: 10, Status: entity.RunStatusCanceled},
	}
	processor := NewRunProcessor(&ApplicationService{ThreadSVC: domainSVC}, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, &RunCanceledError{}
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.CanceledRuns)
	require.NotNil(t, domainSVC.requestRunCancellationReq)
	require.NotNil(t, domainSVC.requestRunCancellationReq.OutboxIntent)
	event := domainSVC.requestRunCancellationReq.OutboxIntent.Event
	require.Equal(t, domainnotification.EventTaskCancelled, event.EventType)
	require.NotEqual(t, domainnotification.EventTaskAwaitingInput, event.EventType)
	require.Equal(t, int64(2), event.ActorID)
	require.Equal(t, int64(1), event.SpaceID)
	require.Equal(t, "thread:10", event.Payload.TargetID)
	require.Equal(t, "生成周报", event.Payload.ResourceDisplayName)
}

func TestRunProcessorPreservesDurableMultitaskInterruptionAfterExecutorCancel(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 3, Config: `{"runtime":"eino_adk"}`,
		}},
		gotRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusInterrupted,
			ExecutionGeneration: 4, ErrorCode: "multitask_interrupt",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(app, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, &RunCanceledError{EventPersisted: true}
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.InterruptedRuns)
	require.Zero(t, result.CanceledRuns)
	require.Nil(t, domainSVC.requestRunCancellationReq)
}

func TestRunProcessorCleansEinoCheckpointForDurableMultitaskRollback(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 3, Config: `{"runtime":"eino_adk"}`,
		}},
		gotRun: &entity.Run{
			ID: 200, ThreadID: 10, SpaceID: 1, CreatorID: 2, Status: entity.RunStatusInterrupted,
			ExecutionGeneration: 4, ErrorCode: "multitask_rollback",
		},
		got: &entity.Thread{ID: 10, Title: "被新任务回滚"},
		failedRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusFailed,
			ExecutionGeneration: 4, ErrorCode: "multitask_rollback",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(app, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, &RunCanceledError{EventPersisted: true}
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.InterruptedRuns)
	require.NotNil(t, domainSVC.deleteRuntimeReq)
	require.Equal(t, int64(10), domainSVC.deleteRuntimeReq.ThreadID)
	require.Equal(t, int64(200), domainSVC.deleteRuntimeReq.RunID)
	require.Equal(t, string(RuntimeModeEinoADK), domainSVC.deleteRuntimeReq.RuntimeType)
	require.Equal(t, adkCheckpointKeyForRun(200), domainSVC.deleteRuntimeReq.RuntimeKey)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, entity.RunStatusInterrupted, domainSVC.failRunReq.From)
	require.Equal(t, "multitask_rollback", domainSVC.failRunReq.ErrorCode)
	require.Equal(t, "run rolled back by a newer thread run", domainSVC.failRunReq.ErrorMessage)
	require.NotNil(t, domainSVC.failRunReq.OutboxIntent)
	require.NotEqual(t, domainnotification.EventTaskAwaitingInput, domainSVC.failRunReq.OutboxIntent.Event.EventType)
}

func TestApplicationServiceDoesNotPublishAwaitingInputForLeaseRecoveryInterruptedStatus(t *testing.T) {
	appnotification.SetDefaultService(appnotification.NewService(&recordingNotificationRepository{}))
	defer appnotification.SetDefaultService(nil)
	domainSVC := &recordingThreadService{
		reconciledRunLease: &entity.Run{
			ID: 201, ThreadID: 10, Status: entity.RunStatusInterrupted,
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	resp, err := app.ReconcileExpiredRunLease(context.Background(), &ReconcileExpiredRunLeaseRequest{
		RunID:               201,
		LeaseOwner:          "worker-a",
		LeaseToken:          "lease-201",
		ExecutionGeneration: 2,
		ToStatus:            RunStatusInterrupted,
		Now:                 2_000,
		ErrorCode:           "lease_expired",
		ErrorMessage:        "run lease expired",
		EventPayload:        `{"status":"interrupted","error_code":"lease_expired"}`,
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, domainSVC.reconcileExpiredRunLeaseReq)
	require.Nil(t, domainSVC.reconcileExpiredRunLeaseReq.OutboxIntent)
}

func TestRunProcessorFinalizesLegacyMultitaskRollbackWithoutEinoCheckpoint(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 3, Config: `{}`,
		}},
		gotRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusInterrupted,
			ExecutionGeneration: 4, ErrorCode: "multitask_rollback",
		},
		failedRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusFailed,
			ExecutionGeneration: 4, ErrorCode: "multitask_rollback",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(app, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return nil, &RunCanceledError{EventPersisted: true}
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.InterruptedRuns)
	require.Nil(t, domainSVC.deleteRuntimeReq)
	require.NotNil(t, domainSVC.failRunReq)
	require.Equal(t, entity.RunStatusInterrupted, domainSVC.failRunReq.From)
}

func TestFinalizeMultitaskRollbackPersistsFailedStatusWhenCheckpointCleanupFails(t *testing.T) {
	domainSVC := &recordingThreadService{
		deleteRuntimeErr: errors.New("checkpoint storage unavailable"),
		failedRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusFailed,
			ExecutionGeneration: 4, ErrorCode: "multitask_rollback",
		},
	}
	app := &ApplicationService{ThreadSVC: domainSVC}

	terminal, err := finalizeMultitaskRollback(
		context.Background(),
		app,
		&RunSummary{RunID: 200, ThreadID: 10, Config: `{"runtime":"eino_adk"}`},
		&RunSummary{
			RunID: 200, ThreadID: 10, Status: RunStatusInterrupted,
			ExecutionGeneration: 4, ErrorCode: "multitask_rollback",
		},
	)

	require.ErrorContains(t, err, "checkpoint storage unavailable")
	require.NotNil(t, terminal)
	require.Equal(t, RunStatusFailed, terminal.Status)
	require.NotNil(t, domainSVC.deleteRuntimeReq)
	require.NotNil(t, domainSVC.failRunReq)
}

func TestRunProcessorDiscardsLateSuccessAfterDurableMultitaskInterruption(t *testing.T) {
	domainSVC := &recordingThreadService{
		claimedRuns: []*entity.Run{{
			ID: 200, ThreadID: 10, Status: entity.RunStatusRunning,
			WorkerID: "worker-a", LeaseOwner: "worker-a", LeaseToken: "lease-200",
			ExecutionGeneration: 3, Config: `{"runtime":"eino_adk"}`,
		}},
		gotRun: &entity.Run{
			ID: 200, ThreadID: 10, Status: entity.RunStatusInterrupted,
			ExecutionGeneration: 4, ErrorCode: "multitask_interrupt",
		},
		finalizeRunSuccessErr: domainrepo.ErrRunLeaseLost,
	}
	app := &ApplicationService{ThreadSVC: domainSVC}
	processor := NewRunProcessor(app, RunExecutorFunc(func(
		context.Context,
		*RunSummary,
	) (*RunExecutionResult, error) {
		return &RunExecutionResult{Message: "stale assistant result"}, nil
	}), RunProcessorOptions{WorkerID: "worker-a", BatchSize: 1})

	result, err := processor.ProcessPendingRunsWithResult(context.Background())

	require.NoError(t, err)
	require.Equal(t, 1, result.InterruptedRuns)
	require.Zero(t, result.SucceededRuns)
	require.NotNil(t, domainSVC.finalizeRunSuccessReq)
	require.Nil(t, domainSVC.appendReq)
}

type recordingRunEventSink struct {
	mu      sync.Mutex
	events  []RunEvent
	onEmit  func(RunEvent)
	emitErr func(RunEvent) error
}

func (s *recordingRunEventSink) EmitRunEvent(ctx context.Context, event RunEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	if s.onEmit != nil {
		s.onEmit(event)
	}
	if s.emitErr != nil {
		return s.emitErr(event)
	}

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

type recordingNotificationRepository struct{}

func (recordingNotificationRepository) Append(context.Context, domainnotification.Event) error {
	return nil
}

func (recordingNotificationRepository) AppendInTransaction(context.Context, *gorm.DB, domainnotification.Event) error {
	return nil
}

func (recordingNotificationRepository) ListForUser(context.Context, domainnotification.ListFilter) (domainnotification.ListPage, error) {
	return domainnotification.ListPage{}, nil
}

func (recordingNotificationRepository) CountUnread(context.Context, int64) (int64, error) {
	return 0, nil
}

func (recordingNotificationRepository) MarkRead(context.Context, int64, []int64, int64) (int64, error) {
	return 0, nil
}

func (recordingNotificationRepository) MarkAllRead(context.Context, int64, int64, int64) (int64, error) {
	return 0, nil
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

type blockingRunTitleGenerator struct {
	ctxErr error
}

func (g *blockingRunTitleGenerator) GenerateTitle(
	ctx context.Context,
	_ RunTitleGenerationInput,
) (string, error) {
	<-ctx.Done()
	g.ctxErr = ctx.Err()

	return "", g.ctxErr
}
