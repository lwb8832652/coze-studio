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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKTurnLoopInitialInputStartsOneTurn(t *testing.T) {
	model := newTurnLoopModel(false)
	factory := newTurnLoopAgentFactory(t, model)
	sink := newChannelRunEventSink()
	handle := newTestADKTurnLoop(t, factory, sink, newMemoryADKCheckpointStore(), nil)

	ok, _, err := handle.Push(ADKTurnItem{
		MessageID: 1,
		ThreadID:  10,
		RunID:     20,
		Content:   "first task",
		CreatedAt: 100,
	})
	require.NoError(t, err)
	require.True(t, ok)
	handle.Run(context.Background())

	event := sink.waitForContent(t, "answer-1")
	require.Contains(t, event.Payload, `"content":"answer-1"`)
	handle.Stop()
	exit := handle.Wait()

	require.NoError(t, exit.ExitReason)
	require.Equal(t, []string{"first task"}, model.inputs())
}

func TestADKTurnLoopPushBetweenTurnsStartsNextTurn(t *testing.T) {
	model := newTurnLoopModel(false)
	factory := newTurnLoopAgentFactory(t, model)
	sink := newChannelRunEventSink()
	registry := NewADKTurnLoopRegistry()
	handle := newTestADKTurnLoop(t, factory, sink, newMemoryADKCheckpointStore(), registry)
	require.NoError(t, registry.Start(context.Background(), handle))

	ok, _, err := registry.Push(10, 20, ADKTurnItem{
		MessageID: 1,
		ThreadID:  10,
		RunID:     20,
		Content:   "first",
	})
	require.NoError(t, err)
	require.True(t, ok)
	sink.waitForContent(t, "answer-1")

	ok, _, err = registry.Push(10, 20, ADKTurnItem{
		MessageID: 2,
		ThreadID:  10,
		RunID:     20,
		Content:   "second",
	})
	require.NoError(t, err)
	require.True(t, ok)
	sink.waitForContent(t, "answer-2")
	registry.Stop(10, 20)
	exit := handle.Wait()

	require.NoError(t, exit.ExitReason)
	require.Equal(t, []string{"first", "second"}, model.inputs())
}

func TestADKTurnLoopPreemptStartsFollowUpAtSafePoint(t *testing.T) {
	model := newTurnLoopModel(true)
	factory := newTurnLoopAgentFactory(t, model)
	sink := newChannelRunEventSink()
	handle := newTestADKTurnLoop(t, factory, sink, newMemoryADKCheckpointStore(), nil)
	ok, _, err := handle.Push(ADKTurnItem{
		MessageID: 1,
		ThreadID:  10,
		RunID:     20,
		Content:   "slow",
	})
	require.NoError(t, err)
	require.True(t, ok)
	handle.Run(context.Background())
	model.waitStarted(t)

	ok, ack, err := handle.Push(
		ADKTurnItem{
			MessageID: 2,
			ThreadID:  10,
			RunID:     20,
			Content:   "urgent",
		},
		adk.WithPreempt[ADKTurnItem, *schema.Message](adk.AfterChatModel),
	)
	require.NoError(t, err)
	require.True(t, ok)
	model.releaseFirst()
	requireChannelClosed(t, ack)
	sink.waitForContent(t, "answer-2")
	handle.Stop()
	exit := handle.Wait()

	require.NoError(t, exit.ExitReason)
	require.Equal(t, []string{"slow", "urgent"}, model.inputs())
}

func TestADKTurnLoopImmediateStopPersistsAndResumes(t *testing.T) {
	store := newMemoryADKCheckpointStore()
	firstModel := newTurnLoopModel(true)
	firstSink := newChannelRunEventSink()
	first := newTestADKTurnLoop(t, newTurnLoopAgentFactory(t, firstModel), firstSink, store, nil)
	ok, _, err := first.Push(ADKTurnItem{
		MessageID: 1,
		ThreadID:  10,
		RunID:     20,
		Content:   "resume me",
	})
	require.NoError(t, err)
	require.True(t, ok)
	first.Run(context.Background())
	firstModel.waitStarted(t)
	first.Stop(adk.WithImmediate())
	firstExit := first.Wait()

	var canceled *adk.CancelError
	require.ErrorAs(t, firstExit.ExitReason, &canceled)
	require.True(t, firstExit.CheckpointAttempted)
	require.NoError(t, firstExit.CheckpointErr)
	require.Len(t, firstExit.InterruptedItems, 1)

	secondModel := newTurnLoopModel(false)
	secondSink := newChannelRunEventSink()
	second := newTestADKTurnLoop(t, newTurnLoopAgentFactory(t, secondModel), secondSink, store, nil)
	second.Run(context.Background())
	secondSink.waitFor(t, "message.completed")
	second.Stop()
	secondExit := second.Wait()

	require.NoError(t, secondExit.ExitReason)
	require.Equal(t, []string{"resume me"}, secondModel.inputs())
}

func TestADKTurnLoopReturnsLateItemsAfterStop(t *testing.T) {
	model := newTurnLoopModel(false)
	handle := newTestADKTurnLoop(
		t,
		newTurnLoopAgentFactory(t, model),
		newChannelRunEventSink(),
		newMemoryADKCheckpointStore(),
		nil,
	)
	handle.Stop()
	handle.Run(context.Background())
	exit := handle.Wait()

	late := ADKTurnItem{
		MessageID: 9,
		ThreadID:  10,
		RunID:     20,
		Content:   "too late",
	}
	ok, _, err := handle.Push(late)

	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, []ADKTurnItem{late}, exit.TakeLateItems())
}

func TestADKTurnLoopRegistryCanRestartKeyAfterWait(t *testing.T) {
	registry := NewADKTurnLoopRegistry()
	first := newTestADKTurnLoop(
		t,
		newTurnLoopAgentFactory(t, newTurnLoopModel(false)),
		newChannelRunEventSink(),
		newMemoryADKCheckpointStore(),
		registry,
	)
	require.NoError(t, registry.Start(context.Background(), first))
	first.Stop()
	require.NoError(t, first.Wait().ExitReason)

	second := newTestADKTurnLoop(
		t,
		newTurnLoopAgentFactory(t, newTurnLoopModel(false)),
		newChannelRunEventSink(),
		newMemoryADKCheckpointStore(),
		registry,
	)
	require.NoError(t, registry.Start(context.Background(), second))
	second.Stop()
	require.NoError(t, second.Wait().ExitReason)
}

func newTestADKTurnLoop(
	t *testing.T,
	factory ADKAgentFactory,
	sink RunEventSink,
	store adk.CheckPointStore,
	registry *ADKTurnLoopRegistry,
) *ADKTurnLoopHandle {
	t.Helper()

	handle, err := NewADKTurnLoop(&RunSummary{
		ThreadID: 10,
		RunID:    20,
		Config:   `{"runtime":"eino_adk","agent_name":"lead"}`,
	}, ADKTurnLoopComponents{
		AgentFactory: factory,
		EventSink:    sink,
		Store:        store,
		Registry:     registry,
	})
	require.NoError(t, err)
	return handle
}

func newTurnLoopAgentFactory(t *testing.T, chatModel model.BaseChatModel) ADKAgentFactory {
	t.Helper()
	return ADKAgentFactoryFunc(func(context.Context, *RunSummary) (adk.ResumableAgent, error) {
		return adk.NewChatModelAgent(context.Background(), &adk.ChatModelAgentConfig{
			Name:        "lead",
			Description: "turn loop test agent",
			Model:       chatModel,
		})
	})
}

type turnLoopModel struct {
	mu          sync.Mutex
	received    []string
	calls       int
	blockFirst  bool
	started     chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
}

func newTurnLoopModel(blockFirst bool) *turnLoopModel {
	return &turnLoopModel{
		blockFirst: blockFirst,
		started:    make(chan struct{}),
		release:    make(chan struct{}),
	}
}

func (m *turnLoopModel) Generate(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.Message, error) {
	return m.respond(ctx, input)
}

func (m *turnLoopModel) Stream(
	ctx context.Context,
	input []*schema.Message,
	options ...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	message, err := m.respond(ctx, input)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func (m *turnLoopModel) respond(
	ctx context.Context,
	input []*schema.Message,
) (*schema.Message, error) {
	content := ""
	for _, message := range input {
		if message != nil && message.Role == schema.User {
			content = message.Content
		}
	}
	m.mu.Lock()
	m.calls++
	call := m.calls
	m.received = append(m.received, content)
	m.mu.Unlock()

	if call == 1 && m.blockFirst {
		m.startOnce.Do(func() { close(m.started) })
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.release:
		}
	}

	return schema.AssistantMessage(fmt.Sprintf("answer-%d", call), nil), nil
}

func (m *turnLoopModel) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-m.started:
	case <-time.After(3 * time.Second):
		t.Fatal("model did not start")
	}
}

func (m *turnLoopModel) releaseFirst() {
	m.releaseOnce.Do(func() { close(m.release) })
}

func (m *turnLoopModel) inputs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.received...)
}

type channelRunEventSink struct {
	events chan RunEvent
}

func newChannelRunEventSink() *channelRunEventSink {
	return &channelRunEventSink{events: make(chan RunEvent, 32)}
}

func (s *channelRunEventSink) EmitRunEvent(_ context.Context, event RunEvent) error {
	s.events <- event
	return nil
}

func (s *channelRunEventSink) waitFor(t *testing.T, eventType string) RunEvent {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-s.events:
			if event.EventType == eventType {
				return event
			}
		case <-timer.C:
			t.Fatalf("event %s was not emitted", eventType)
		}
	}
}

func (s *channelRunEventSink) waitForContent(t *testing.T, content string) RunEvent {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-s.events:
			if event.EventType == "message.completed" &&
				strings.Contains(event.Payload, content) {
				return event
			}
		case <-timer.C:
			t.Fatalf("message content %q was not emitted", content)
		}
	}
}

func requireChannelClosed(t *testing.T, channel <-chan struct{}) {
	t.Helper()
	require.NotNil(t, channel)
	select {
	case <-channel:
	case <-time.After(3 * time.Second):
		t.Fatal("ack channel was not closed")
	}
}
