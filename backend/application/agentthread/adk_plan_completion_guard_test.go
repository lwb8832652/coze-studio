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
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/plantask"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKPlanCompletionGuardContinuesWhenTasksAreIncomplete(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID:     21,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}
	store := newMemoryADKPlanStore()
	backend, err := NewADKPlanBackend(run, store, &recordingRunEventSink{})
	require.NoError(t, err)
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Write final deliverable","description":"","status":"pending","blocks":[],"blockedBy":[]}`,
	}))

	chatModel := &sequenceADKChatModel{responses: []*schema.Message{
		schema.AssistantMessage("premature final", nil),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-complete-plan",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      plantask.TaskUpdateToolName,
				Arguments: `{"taskId":"1","status":"completed"}`,
			},
		}}),
		schema.AssistantMessage("done after todos", nil),
	}}
	assistantContents := runPlanGuardAgent(t, ctx, run, backend, chatModel)

	require.Equal(t, []string{"done after todos"}, assistantContents)
	require.Equal(t, 3, chatModel.callCount())
	require.Equal(t, "completed", store.task(1).Status)
}

func TestADKPlanCompletionGuardAllowsFinalWhenTasksAreComplete(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID:     22,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}
	store := newMemoryADKPlanStore()
	backend, err := NewADKPlanBackend(run, store, &recordingRunEventSink{})
	require.NoError(t, err)
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Write final deliverable","description":"","status":"completed","blocks":[],"blockedBy":[]}`,
	}))

	chatModel := &sequenceADKChatModel{responses: []*schema.Message{
		schema.AssistantMessage("ready final", nil),
	}}
	assistantContents := runPlanGuardAgent(t, ctx, run, backend, chatModel)

	require.Equal(t, []string{"ready final"}, assistantContents)
	require.Equal(t, 1, chatModel.callCount())
}

func TestADKPlanCompletionGuardPreservesStreamWhenTasksAreComplete(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID:     24,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}
	store := newMemoryADKPlanStore()
	backend, err := NewADKPlanBackend(run, store, &recordingRunEventSink{})
	require.NoError(t, err)
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Write final deliverable","description":"","status":"completed","blocks":[],"blockedBy":[]}`,
	}))
	guard, err := newADKPlanCompletionGuardMiddleware(backend)
	require.NoError(t, err)
	base := &chunkedADKChatModel{
		chunks: []*schema.Message{
			{Role: schema.Assistant, Content: "hel"},
			{Role: schema.Assistant, Content: "lo"},
		},
	}
	wrapped, err := guard.WrapModel(ctx, base, nil)
	require.NoError(t, err)

	stream, err := wrapped.Stream(ctx, nil)
	require.NoError(t, err)
	first, err := stream.Recv()
	require.NoError(t, err)
	second, err := stream.Recv()
	require.NoError(t, err)
	_, err = stream.Recv()

	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, "hel", first.Content)
	require.Equal(t, "lo", second.Content)
	require.Equal(t, 1, base.streamCount())
}

func TestADKPlanCompletionGuardAllowsFinalAfterReminderCap(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID:     23,
		ThreadID:  10,
		SpaceID:   30,
		CreatorID: 40,
	}
	store := newMemoryADKPlanStore()
	backend, err := NewADKPlanBackend(run, store, &recordingRunEventSink{})
	require.NoError(t, err)
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Write final deliverable","description":"","status":"in_progress","blocks":[],"blockedBy":[]}`,
	}))

	chatModel := &sequenceADKChatModel{responses: []*schema.Message{
		schema.AssistantMessage("first premature final", nil),
		schema.AssistantMessage("second premature final", nil),
		schema.AssistantMessage("allowed after cap", nil),
	}}
	assistantContents := runPlanGuardAgent(t, ctx, run, backend, chatModel)

	require.Equal(t, []string{"allowed after cap"}, assistantContents)
	require.Equal(t, 3, chatModel.callCount())
	require.Equal(t, "in_progress", store.task(1).Status)
}

func TestADKPlanCompletionGuardLeavesExecutionWithoutPlanUntouched(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID: 25, ThreadID: 10, SpaceID: 30, CreatorID: 40,
	}
	backend, err := NewADKPlanBackend(
		run,
		newMemoryADKPlanStore(),
		&recordingRunEventSink{},
	)
	require.NoError(t, err)
	guard, err := newADKPlanCompletionGuardMiddleware(backend)
	require.NoError(t, err)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-write",
			Function: schema.FunctionCall{
				Name:      adkWriteFileToolName,
				Arguments: `{"file_path":"/mnt/user-data/outputs/report.md","content":"done"}`,
			},
		}}),
	}}

	_, rewritten, err := guard.AfterModelRewriteState(ctx, state, nil)

	require.NoError(t, err)
	require.Len(t, rewritten.Messages, 1)
	require.Len(t, rewritten.Messages[0].ToolCalls, 1)
	require.Equal(
		t,
		adkWriteFileToolName,
		rewritten.Messages[0].ToolCalls[0].Function.Name,
	)
}

func TestADKPlanCompletionGuardLeavesExecutionWithPendingPlanUntouched(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID: 26, ThreadID: 10, SpaceID: 30, CreatorID: 40,
	}
	backend, err := NewADKPlanBackend(
		run,
		newMemoryADKPlanStore(),
		&recordingRunEventSink{},
	)
	require.NoError(t, err)
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Write report","description":"","status":"pending","blocks":[],"blockedBy":[]}`,
	}))
	guard, err := newADKPlanCompletionGuardMiddleware(backend)
	require.NoError(t, err)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-write",
			Function: schema.FunctionCall{
				Name:      adkWriteFileToolName,
				Arguments: `{"file_path":"/mnt/user-data/outputs/report.md","content":"done"}`,
			},
		}}),
	}}

	_, rewritten, err := guard.AfterModelRewriteState(ctx, state, nil)

	require.NoError(t, err)
	require.Equal(
		t,
		adkWriteFileToolName,
		rewritten.Messages[0].ToolCalls[0].Function.Name,
	)
}

func TestADKPlanCompletionGuardLeavesPlanningAndActiveExecutionUntouched(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID: 27, ThreadID: 10, SpaceID: 30, CreatorID: 40,
	}
	backend, err := NewADKPlanBackend(
		run,
		newMemoryADKPlanStore(),
		&recordingRunEventSink{},
	)
	require.NoError(t, err)
	guard, err := newADKPlanCompletionGuardMiddleware(backend)
	require.NoError(t, err)

	planState := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-plan",
			Function: schema.FunctionCall{
				Name:      plantask.TaskCreateToolName,
				Arguments: `{"subject":"Write report","description":"Create the requested report"}`,
			},
		}}),
	}}
	_, rewrittenPlan, err := guard.AfterModelRewriteState(ctx, planState, nil)
	require.NoError(t, err)
	require.Equal(
		t,
		plantask.TaskCreateToolName,
		rewrittenPlan.Messages[0].ToolCalls[0].Function.Name,
	)

	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Write report","description":"","status":"in_progress","blocks":[],"blockedBy":[]}`,
	}))
	executionState := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-write",
			Function: schema.FunctionCall{
				Name:      adkWriteFileToolName,
				Arguments: `{"file_path":"/mnt/user-data/outputs/report.md","content":"done"}`,
			},
		}}),
	}}
	_, rewrittenExecution, err := guard.AfterModelRewriteState(
		ctx,
		executionState,
		nil,
	)
	require.NoError(t, err)
	require.Equal(
		t,
		adkWriteFileToolName,
		rewrittenExecution.Messages[0].ToolCalls[0].Function.Name,
	)
}

func TestADKPlanCompletionGuardLeavesMixedPlanTransitionAndExecutionUntouched(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID: 28, ThreadID: 10, SpaceID: 30, CreatorID: 40,
	}
	backend, err := NewADKPlanBackend(
		run,
		newMemoryADKPlanStore(),
		&recordingRunEventSink{},
	)
	require.NoError(t, err)
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Inspect sources","description":"","status":"in_progress","blocks":[],"blockedBy":[]}`,
	}))
	guard, err := newADKPlanCompletionGuardMiddleware(backend)
	require.NoError(t, err)
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{
			{
				ID: "call-complete",
				Function: schema.FunctionCall{
					Name:      plantask.TaskUpdateToolName,
					Arguments: `{"taskId":"1","status":"completed"}`,
				},
			},
			{
				ID: "call-write",
				Function: schema.FunctionCall{
					Name:      adkWriteFileToolName,
					Arguments: `{"file_path":"/mnt/user-data/outputs/report.md","content":"done"}`,
				},
			},
		}),
	}}

	_, rewritten, err := guard.AfterModelRewriteState(ctx, state, nil)

	require.NoError(t, err)
	require.Len(t, rewritten.Messages[0].ToolCalls, 2)
	require.Equal(
		t,
		plantask.TaskUpdateToolName,
		rewritten.Messages[0].ToolCalls[0].Function.Name,
	)
	require.Equal(
		t,
		adkWriteFileToolName,
		rewritten.Messages[0].ToolCalls[1].Function.Name,
	)
}

func TestADKPlanCompletionGuardPreservesReasoningForExecutionToolCalls(t *testing.T) {
	ctx := context.Background()
	run := &RunSummary{
		RunID: 29, ThreadID: 10, SpaceID: 30, CreatorID: 40,
	}
	backend, err := NewADKPlanBackend(
		run,
		newMemoryADKPlanStore(),
		&recordingRunEventSink{},
	)
	require.NoError(t, err)
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/.highwatermark",
		Content:  "1",
	}))
	require.NoError(t, backend.Write(ctx, &plantask.WriteRequest{
		FilePath: "/plans/1.json",
		Content:  `{"id":"1","subject":"Inspect sources","description":"","status":"in_progress","blocks":[],"blockedBy":[]}`,
	}))
	guard, err := newADKPlanCompletionGuardMiddleware(backend)
	require.NoError(t, err)
	message := schema.AssistantMessage("", []schema.ToolCall{
		{
			ID: "call-complete",
			Function: schema.FunctionCall{
				Name:      plantask.TaskUpdateToolName,
				Arguments: `{"taskId":"1","status":"completed"}`,
			},
		},
		{
			ID: "call-write",
			Function: schema.FunctionCall{
				Name:      adkWriteFileToolName,
				Arguments: `{"file_path":"/mnt/user-data/outputs/report.md","content":"done"}`,
			},
		},
	})
	message.ReasoningContent = "I should complete the current step before writing."
	state := &adk.ChatModelAgentState{Messages: []*schema.Message{message}}

	_, rewritten, err := guard.AfterModelRewriteState(ctx, state, nil)

	require.NoError(t, err)
	require.Equal(
		t,
		message.ReasoningContent,
		rewritten.Messages[0].ReasoningContent,
	)
	require.Len(t, rewritten.Messages[0].ToolCalls, 2)
	require.Equal(
		t,
		plantask.TaskUpdateToolName,
		rewritten.Messages[0].ToolCalls[0].Function.Name,
	)
	require.Equal(
		t,
		adkWriteFileToolName,
		rewritten.Messages[0].ToolCalls[1].Function.Name,
	)
}

func runPlanGuardAgent(
	t *testing.T,
	ctx context.Context,
	run *RunSummary,
	backend plantask.Backend,
	chatModel *sequenceADKChatModel,
) []string {
	t.Helper()
	assembler := NewADKMiddlewareAssembler(ADKMiddlewareAssemblerOptions{
		PlanBackendFactory: ADKPlanBackendFactoryFunc(func(
			context.Context,
			*RunSummary,
		) (plantask.Backend, error) {
			return backend, nil
		}),
	})
	bundle, err := assembler.Build(ctx, ADKMiddlewareBuildInput{
		Run:   run,
		Model: chatModel,
	})
	require.NoError(t, err)
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "lead",
		Description: "plan completion guard test",
		Model:       chatModel,
		Handlers:    bundle.Handlers,
	})
	require.NoError(t, err)

	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
	})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("finish the plan")})

	var assistantContents []string
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		require.NoError(t, event.Err)
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		msg, err := event.Output.MessageOutput.GetMessage()
		require.NoError(t, err)
		if msg != nil && msg.Role == schema.Assistant &&
			msg.Content != "" {
			assistantContents = append(assistantContents, msg.Content)
		}
	}

	return assistantContents
}

type sequenceADKChatModel struct {
	mu        sync.Mutex
	responses []*schema.Message
	calls     int
}

func (m *sequenceADKChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return m.next()
}

func (m *sequenceADKChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.next()
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *sequenceADKChatModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func (m *sequenceADKChatModel) next() (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.calls >= len(m.responses) {
		return nil, fmt.Errorf("unexpected model call %d", m.calls+1)
	}
	msg := m.responses[m.calls]
	m.calls++
	return msg, nil
}

type chunkedADKChatModel struct {
	mu      sync.Mutex
	chunks  []*schema.Message
	streams int
}

func (m *chunkedADKChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return nil, fmt.Errorf("unexpected generate call")
}

func (m *chunkedADKChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	m.mu.Lock()
	m.streams++
	m.mu.Unlock()
	return schema.StreamReaderFromArray(m.chunks), nil
}

func (m *chunkedADKChatModel) streamCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.streams
}
