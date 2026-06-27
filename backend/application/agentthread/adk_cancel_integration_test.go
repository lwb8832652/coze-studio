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
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKExecutorCancellationReachesActiveTool(t *testing.T) {
	blockingTool := newCancelAwareTool("slow_tool")
	t.Cleanup(blockingTool.release)
	chatModel := &scriptedCancelChatModel{
		messages: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "slow_tool",
					Arguments: `{"input":"test"}`,
				},
			}}),
		},
	}
	registry := NewADKCancelRegistry()
	executor := newCancelIntegrationExecutor(t, registry, func(ctx context.Context) (adk.ResumableAgent, error) {
		return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
			Name:        "lead",
			Description: "tool cancellation test",
			Model:       chatModel,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig: compose.ToolsNodeConfig{
					Tools: []tool.BaseTool{blockingTool},
				},
			},
		})
	})

	done := executeADKForCancelTest(executor)
	requireSignal(t, blockingTool.started, "tool did not start")
	require.NoError(t, cancelActiveADKForTest(registry))

	requireSignal(t, blockingTool.canceled, "tool context was not canceled")
	requireRunCanceled(t, done)
}

func TestADKExecutorSafePointCancellationWaitsForActiveTool(t *testing.T) {
	blockingTool := newCancelAwareTool("slow_tool")
	t.Cleanup(blockingTool.release)
	chatModel := &scriptedCancelChatModel{
		messages: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "slow_tool",
					Arguments: `{"input":"test"}`,
				},
			}}),
		},
	}
	registry := NewADKCancelRegistry()
	executor := newCancelIntegrationExecutor(t, registry, func(ctx context.Context) (adk.ResumableAgent, error) {
		return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
			Name:        "lead",
			Description: "safe-point tool cancellation test",
			Model:       chatModel,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig: compose.ToolsNodeConfig{
					Tools: []tool.BaseTool{blockingTool},
				},
			},
		})
	})

	done := executeADKForCancelTest(executor)
	requireSignal(t, blockingTool.started, "tool did not start")
	cancelDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cancelDone <- registry.Cancel(
			ctx,
			20,
			adk.CancelAfterToolCalls,
			true,
		)
	}()

	select {
	case <-blockingTool.canceled:
		t.Fatal("safe-point cancellation canceled the active tool context")
	case <-time.After(100 * time.Millisecond):
	}
	blockingTool.release()
	require.NoError(t, <-cancelDone)
	requireRunCanceled(t, done)
}

func TestADKExecutorCancellationReachesNestedAgentTool(t *testing.T) {
	leafModel := newCancelAwareStreamModel("leaf result")
	t.Cleanup(leafModel.release)
	leafAgent, err := adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
		Name:        "leaf",
		Description: "nested cancellation leaf",
		Model:       leafModel,
	})
	require.NoError(t, err)

	rootModel := &scriptedCancelChatModel{
		messages: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{{
				ID:   "call-leaf",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "leaf",
					Arguments: `{"request":"work"}`,
				},
			}}),
		},
	}
	registry := NewADKCancelRegistry()
	executor := newCancelIntegrationExecutor(t, registry, func(ctx context.Context) (adk.ResumableAgent, error) {
		return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
			Name:        "lead",
			Description: "nested cancellation root",
			Model:       rootModel,
			ToolsConfig: adk.ToolsConfig{
				ToolsNodeConfig: compose.ToolsNodeConfig{
					Tools: []tool.BaseTool{adk.NewAgentTool(ctx, leafAgent)},
				},
			},
		})
	})

	done := executeADKForCancelTest(executor)
	requireSignal(t, leafModel.started, "nested agent model did not start")
	cancelErr := cancelActiveADKForTest(registry)
	if cancelErr != nil {
		require.ErrorIs(t, cancelErr, adk.ErrExecutionEnded)
	}

	requireRunCanceled(t, done)
}

func newCancelIntegrationExecutor(
	t *testing.T,
	registry *ADKCancelRegistry,
	build func(context.Context) (adk.ResumableAgent, error),
) *ADKExecutor {
	t.Helper()

	return NewADKExecutor(
		ADKAgentFactoryFunc(func(ctx context.Context, _ *RunSummary) (adk.ResumableAgent, error) {
			return build(ctx)
		}),
		&recordingRunEventSink{},
		func(*RunSummary) (adk.CheckPointStore, error) {
			return newMemoryADKCheckpointStore(), nil
		},
		nil,
		WithADKCancelRegistry(registry),
	)
}

func executeADKForCancelTest(executor *ADKExecutor) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := executor.Execute(context.Background(), contractRun(20))
		done <- err
	}()
	return done
}

func cancelActiveADKForTest(registry *ADKCancelRegistry) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return registry.Cancel(ctx, 20, adk.CancelImmediate, true)
}

func requireRunCanceled(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		var canceled *RunCanceledError
		require.ErrorAs(t, err, &canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("ADK execution did not finish after cancellation")
	}
}

func requireSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal(message)
	}
}

type cancelAwareTool struct {
	name        string
	started     chan struct{}
	canceled    chan struct{}
	released    chan struct{}
	start       sync.Once
	cancel      sync.Once
	releaseOnce sync.Once
}

func newCancelAwareTool(name string) *cancelAwareTool {
	return &cancelAwareTool{
		name:     name,
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		released: make(chan struct{}),
	}
}

func (t *cancelAwareTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: "blocks until canceled",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"input": {Type: "string"},
		}),
	}, nil
}

func (t *cancelAwareTool) InvokableRun(
	ctx context.Context,
	_ string,
	_ ...tool.Option,
) (string, error) {
	t.start.Do(func() { close(t.started) })
	select {
	case <-ctx.Done():
		t.cancel.Do(func() { close(t.canceled) })
		return "", ctx.Err()
	case <-t.released:
		return "tool result", nil
	}
}

func (t *cancelAwareTool) release() {
	t.releaseOnce.Do(func() { close(t.released) })
}

type scriptedCancelChatModel struct {
	mu       sync.Mutex
	messages []*schema.Message
	call     int
}

func (m *scriptedCancelChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	return m.next(), nil
}

func (m *scriptedCancelChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{m.next()}), nil
}

func (m *scriptedCancelChatModel) next() *schema.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.call >= len(m.messages) {
		return schema.AssistantMessage("done", nil)
	}
	message := m.messages[m.call]
	m.call++
	return message
}

type cancelAwareStreamModel struct {
	message     string
	started     chan struct{}
	releaseChan chan struct{}
	start       sync.Once
	releaseOnce sync.Once
}

func newCancelAwareStreamModel(message string) *cancelAwareStreamModel {
	return &cancelAwareStreamModel{
		message:     message,
		started:     make(chan struct{}),
		releaseChan: make(chan struct{}),
	}
}

func (m *cancelAwareStreamModel) Generate(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.Message, error) {
	m.start.Do(func() { close(m.started) })
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.releaseChan:
		return schema.AssistantMessage(m.message, nil), nil
	}
}

func (m *cancelAwareStreamModel) Stream(
	ctx context.Context,
	_ []*schema.Message,
	_ ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	m.start.Do(func() { close(m.started) })
	go func() {
		defer writer.Close()
		select {
		case <-ctx.Done():
			writer.Send(nil, ctx.Err())
		case <-m.releaseChan:
			writer.Send(schema.AssistantMessage(m.message, nil), nil)
		}
	}()
	return reader, nil
}

func (m *cancelAwareStreamModel) release() {
	m.releaseOnce.Do(func() { close(m.releaseChan) })
}
