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
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

var ErrADKTurnLoopNotActive = errors.New("eino adk turn loop is not active")

type ADKTurnItem struct {
	MessageID int64
	ThreadID  int64
	RunID     int64
	Content   string
	CreatedAt int64
}

type ADKTurnLoopComponents struct {
	AgentFactory   ADKAgentFactory
	EventSink      RunEventSink
	Store          adk.CheckPointStore
	UsageCollector UsageCollector
	Registry       *ADKTurnLoopRegistry
}

type adkTurnRuntimeContextKey struct{}

type adkTurnRuntime struct {
	run         *RunSummary
	usageBridge *ADKUsageBridge
}

type ADKTurnLoopHandle struct {
	run      *RunSummary
	loop     *adk.TurnLoop[ADKTurnItem, *schema.Message]
	registry *ADKTurnLoopRegistry
}

func init() {
	gob.Register(ADKTurnItem{})
}

func NewADKTurnLoop(
	run *RunSummary,
	components ADKTurnLoopComponents,
) (*ADKTurnLoopHandle, error) {
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if run.ThreadID <= 0 {
		return nil, fmt.Errorf("run thread id is required")
	}
	if run.RunID <= 0 {
		return nil, fmt.Errorf("run id is required")
	}
	if components.AgentFactory == nil {
		return nil, fmt.Errorf("eino adk agent factory is required")
	}
	if components.EventSink == nil {
		return nil, fmt.Errorf("eino adk event sink is required")
	}
	if components.Store == nil {
		return nil, fmt.Errorf("eino adk checkpoint store is required")
	}

	baseRun := *run
	handle := &ADKTurnLoopHandle{
		run:      &baseRun,
		registry: components.Registry,
	}
	handle.loop = adk.NewTurnLoop(adk.TurnLoopConfig[ADKTurnItem, *schema.Message]{
		GenInput: func(
			ctx context.Context,
			_ *adk.TurnLoop[ADKTurnItem, *schema.Message],
			items []ADKTurnItem,
		) (*adk.GenInputResult[ADKTurnItem, *schema.Message], error) {
			consumed := uniqueADKTurnItems(items)
			if len(consumed) == 0 {
				return nil, fmt.Errorf("eino adk turn input is empty")
			}
			runtime, runOptions, err := newADKTurnRuntime(
				ctx,
				handle.run,
				consumed,
				components.UsageCollector,
			)
			if err != nil {
				return nil, err
			}

			return &adk.GenInputResult[ADKTurnItem, *schema.Message]{
				RunCtx:  context.WithValue(ctx, adkTurnRuntimeContextKey{}, runtime),
				Input:   adkTurnInput(consumed),
				RunOpts: runOptions,
				Consumed: append(
					[]ADKTurnItem(nil),
					consumed...,
				),
			}, nil
		},
		GenResume: func(
			ctx context.Context,
			_ *adk.TurnLoop[ADKTurnItem, *schema.Message],
			interruptedItems []ADKTurnItem,
			unhandledItems []ADKTurnItem,
			newItems []ADKTurnItem,
		) (*adk.GenResumeResult[ADKTurnItem, *schema.Message], error) {
			consumed := uniqueADKTurnItems(interruptedItems)
			if len(consumed) == 0 {
				return nil, fmt.Errorf("eino adk interrupted turn input is empty")
			}
			remaining := uniqueADKTurnItemsExcluding(
				append(append([]ADKTurnItem(nil), unhandledItems...), newItems...),
				consumed,
			)
			runtime, runOptions, err := newADKTurnRuntime(
				ctx,
				handle.run,
				consumed,
				components.UsageCollector,
			)
			if err != nil {
				return nil, err
			}

			return &adk.GenResumeResult[ADKTurnItem, *schema.Message]{
				RunCtx:    context.WithValue(ctx, adkTurnRuntimeContextKey{}, runtime),
				RunOpts:   runOptions,
				Consumed:  consumed,
				Remaining: remaining,
			}, nil
		},
		PrepareAgent: func(
			ctx context.Context,
			_ *adk.TurnLoop[ADKTurnItem, *schema.Message],
			consumed []ADKTurnItem,
		) (adk.TypedAgent[*schema.Message], error) {
			runtime, err := adkTurnRuntimeFromContext(ctx)
			if err != nil {
				return nil, err
			}
			if len(consumed) == 0 {
				return nil, fmt.Errorf("eino adk consumed turn input is empty")
			}

			agent, err := components.AgentFactory.Build(ctx, runtime.run)
			if err != nil {
				return nil, fmt.Errorf("build eino adk turn agent: %w", err)
			}
			if agent == nil {
				return nil, fmt.Errorf("eino adk turn agent factory returned empty agent")
			}

			return agent, nil
		},
		OnAgentEvents: func(
			ctx context.Context,
			_ *adk.TurnContext[ADKTurnItem, *schema.Message],
			events *adk.AsyncIterator[*adk.TypedAgentEvent[*schema.Message]],
		) error {
			runtime, err := adkTurnRuntimeFromContext(ctx)
			if err != nil {
				return err
			}
			if events == nil {
				return fmt.Errorf("eino adk turn returned empty event iterator")
			}

			for {
				event, ok := events.Next()
				if !ok {
					break
				}
				mapped, err := MapADKEvent(
					ctx,
					runtime.run.ThreadID,
					runtime.run.RunID,
					event,
				)
				if err != nil {
					return err
				}
				if err := components.EventSink.EmitRunEvent(ctx, mapped.RunEvent); err != nil {
					return fmt.Errorf(
						"persist eino adk turn event %s: %w",
						mapped.EventType,
						err,
					)
				}
				if mapped.Usage != nil && runtime.usageBridge != nil {
					if err := runtime.usageBridge.RecordEvent(ctx, *mapped.Usage); err != nil {
						return fmt.Errorf("record eino adk turn token usage: %w", err)
					}
				}

				if event != nil && event.Err != nil {
					var retrying *adk.WillRetryError
					if errors.As(event.Err, &retrying) {
						continue
					}
					var canceled *adk.CancelError
					if errors.As(event.Err, &canceled) {
						continue
					}
					return event.Err
				}
			}
			if runtime.usageBridge != nil {
				if err := runtime.usageBridge.Err(); err != nil {
					return fmt.Errorf("record eino adk turn callback usage: %w", err)
				}
			}

			return nil
		},
		Store:        components.Store,
		CheckpointID: adkTurnLoopCheckpointKey(run.ThreadID, run.RunID),
	})

	return handle, nil
}

func (h *ADKTurnLoopHandle) ThreadID() int64 {
	if h == nil || h.run == nil {
		return 0
	}
	return h.run.ThreadID
}

func (h *ADKTurnLoopHandle) RunID() int64 {
	if h == nil || h.run == nil {
		return 0
	}
	return h.run.RunID
}

func (h *ADKTurnLoopHandle) Run(ctx context.Context) {
	if h == nil || h.loop == nil {
		return
	}
	h.loop.Run(ctx)
}

func (h *ADKTurnLoopHandle) Push(
	item ADKTurnItem,
	options ...adk.PushOption[ADKTurnItem, *schema.Message],
) (bool, <-chan struct{}, error) {
	if h == nil || h.loop == nil {
		return false, nil, ErrADKTurnLoopNotActive
	}
	if err := h.validateItem(item); err != nil {
		return false, nil, err
	}

	ok, ack := h.loop.Push(item, options...)
	return ok, ack, nil
}

func (h *ADKTurnLoopHandle) Stop(options ...adk.StopOption) {
	if h == nil || h.loop == nil {
		return
	}
	h.loop.Stop(options...)
}

func (h *ADKTurnLoopHandle) Wait() *adk.TurnLoopExitState[ADKTurnItem, *schema.Message] {
	if h == nil || h.loop == nil {
		return &adk.TurnLoopExitState[ADKTurnItem, *schema.Message]{
			ExitReason: ErrADKTurnLoopNotActive,
		}
	}
	result := h.loop.Wait()
	if h.registry != nil {
		h.registry.unregister(h)
	}
	return result
}

func (h *ADKTurnLoopHandle) validateItem(item ADKTurnItem) error {
	if item.ThreadID != h.ThreadID() {
		return fmt.Errorf("turn item thread id does not match active loop")
	}
	if item.RunID != h.RunID() {
		return fmt.Errorf("turn item run id does not match active loop")
	}
	if strings.TrimSpace(item.Content) == "" {
		return fmt.Errorf("turn item content is required")
	}
	return nil
}

type adkTurnLoopKey struct {
	threadID int64
	runID    int64
}

type ADKTurnLoopRegistry struct {
	mu      sync.RWMutex
	handles map[adkTurnLoopKey]*ADKTurnLoopHandle
}

func NewADKTurnLoopRegistry() *ADKTurnLoopRegistry {
	return &ADKTurnLoopRegistry{
		handles: make(map[adkTurnLoopKey]*ADKTurnLoopHandle),
	}
}

func (r *ADKTurnLoopRegistry) Start(
	ctx context.Context,
	handle *ADKTurnLoopHandle,
) error {
	if r == nil {
		return fmt.Errorf("eino adk turn loop registry is required")
	}
	if handle == nil || handle.loop == nil {
		return fmt.Errorf("eino adk turn loop handle is required")
	}

	key := adkTurnLoopKey{threadID: handle.ThreadID(), runID: handle.RunID()}
	r.mu.Lock()
	if r.handles == nil {
		r.handles = make(map[adkTurnLoopKey]*ADKTurnLoopHandle)
	}
	if _, exists := r.handles[key]; exists {
		r.mu.Unlock()
		return fmt.Errorf("eino adk turn loop is already active")
	}
	r.handles[key] = handle
	r.mu.Unlock()

	handle.registry = r
	handle.Run(ctx)
	go func() {
		handle.Wait()
	}()

	return nil
}

func (r *ADKTurnLoopRegistry) Push(
	threadID int64,
	runID int64,
	item ADKTurnItem,
	options ...adk.PushOption[ADKTurnItem, *schema.Message],
) (bool, <-chan struct{}, error) {
	handle := r.active(threadID, runID)
	if handle == nil {
		return false, nil, ErrADKTurnLoopNotActive
	}
	return handle.Push(item, options...)
}

func (r *ADKTurnLoopRegistry) Stop(
	threadID int64,
	runID int64,
	options ...adk.StopOption,
) error {
	handle := r.active(threadID, runID)
	if handle == nil {
		return ErrADKTurnLoopNotActive
	}
	handle.Stop(options...)
	return nil
}

func (r *ADKTurnLoopRegistry) active(
	threadID int64,
	runID int64,
) *ADKTurnLoopHandle {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.handles[adkTurnLoopKey{threadID: threadID, runID: runID}]
}

func (r *ADKTurnLoopRegistry) unregister(handle *ADKTurnLoopHandle) {
	if r == nil || handle == nil {
		return
	}
	key := adkTurnLoopKey{threadID: handle.ThreadID(), runID: handle.RunID()}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.handles[key] == handle {
		delete(r.handles, key)
	}
}

func newADKTurnRuntime(
	ctx context.Context,
	baseRun *RunSummary,
	items []ADKTurnItem,
	collector UsageCollector,
) (*adkTurnRuntime, []adk.AgentRunOption, error) {
	run, err := adkTurnRun(baseRun, items)
	if err != nil {
		return nil, nil, err
	}
	runtime := &adkTurnRuntime{run: run}
	var runOptions []adk.AgentRunOption
	if collector != nil {
		runtime.usageBridge = NewADKUsageBridge(run, collector)
		runOptions = append(runOptions, adk.WithCallbacks(runtime.usageBridge.Handler()))
	}

	return runtime, runOptions, nil
}

func adkTurnRuntimeFromContext(ctx context.Context) (*adkTurnRuntime, error) {
	runtime, _ := ctx.Value(adkTurnRuntimeContextKey{}).(*adkTurnRuntime)
	if runtime == nil || runtime.run == nil {
		return nil, fmt.Errorf("eino adk turn runtime context is missing")
	}
	return runtime, nil
}

func adkTurnRun(baseRun *RunSummary, items []ADKTurnItem) (*RunSummary, error) {
	if baseRun == nil {
		return nil, fmt.Errorf("run is required")
	}
	input := modelExecutorRunInput{
		Messages: make([]*schema.Message, 0, len(items)),
	}
	for _, item := range items {
		input.Messages = append(
			input.Messages,
			schema.UserMessage(item.Content),
		)
	}
	rawInput, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal eino adk turn input: %w", err)
	}

	run := *baseRun
	run.Input = string(rawInput)
	return &run, nil
}

func adkTurnInput(items []ADKTurnItem) *adk.TypedAgentInput[*schema.Message] {
	messages := make([]*schema.Message, 0, len(items))
	for _, item := range items {
		messages = append(messages, schema.UserMessage(item.Content))
	}
	return &adk.TypedAgentInput[*schema.Message]{
		Messages:        messages,
		EnableStreaming: true,
	}
}

func uniqueADKTurnItems(items []ADKTurnItem) []ADKTurnItem {
	return uniqueADKTurnItemsExcluding(items, nil)
}

func uniqueADKTurnItemsExcluding(
	items []ADKTurnItem,
	excluded []ADKTurnItem,
) []ADKTurnItem {
	seen := make(map[string]struct{}, len(items)+len(excluded))
	for _, item := range excluded {
		seen[adkTurnItemKey(item)] = struct{}{}
	}

	result := make([]ADKTurnItem, 0, len(items))
	for _, item := range items {
		key := adkTurnItemKey(item)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func adkTurnItemKey(item ADKTurnItem) string {
	if item.MessageID > 0 {
		return fmt.Sprintf("message:%d", item.MessageID)
	}
	return fmt.Sprintf(
		"item:%d:%d:%d:%s",
		item.ThreadID,
		item.RunID,
		item.CreatedAt,
		item.Content,
	)
}

func adkTurnLoopCheckpointKey(threadID, runID int64) string {
	return fmt.Sprintf("coze-turn-loop-%d-%d", threadID, runID)
}
