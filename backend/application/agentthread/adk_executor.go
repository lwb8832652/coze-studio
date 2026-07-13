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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type ADKCheckpointStoreFactory func(run *RunSummary) (adk.CheckPointStore, error)

type ADKExecutor struct {
	factory                     ADKAgentFactory
	eventSink                   RunEventSink
	checkpointStoreFactory      ADKCheckpointStoreFactory
	usageCollector              UsageCollector
	cancelRegistry              *ADKCancelRegistry
	subagentRetrySourceResolver ADKSubagentRetrySourceResolver
}

type ADKExecutorOption func(*ADKExecutor)

func WithADKCancelRegistry(registry *ADKCancelRegistry) ADKExecutorOption {
	return func(executor *ADKExecutor) {
		executor.cancelRegistry = registry
	}
}

func WithADKSubagentRetrySourceResolver(
	resolver ADKSubagentRetrySourceResolver,
) ADKExecutorOption {
	return func(executor *ADKExecutor) {
		executor.subagentRetrySourceResolver = resolver
	}
}

func NewADKExecutor(
	factory ADKAgentFactory,
	eventSink RunEventSink,
	checkpointStoreFactory ADKCheckpointStoreFactory,
	usageCollector UsageCollector,
	options ...ADKExecutorOption,
) *ADKExecutor {
	executor := &ADKExecutor{
		factory:                factory,
		eventSink:              eventSink,
		checkpointStoreFactory: checkpointStoreFactory,
		usageCollector:         usageCollector,
	}
	for _, option := range options {
		if option != nil {
			option(executor)
		}
	}

	return executor
}

func (e *ADKExecutor) Execute(
	ctx context.Context,
	run *RunSummary,
) (*RunExecutionResult, error) {
	if err := e.validate(run); err != nil {
		return nil, err
	}

	messages, err := parseModelExecutorMessages(run.Input, "")
	if err != nil {
		return nil, err
	}
	executionCtx, cancelExecution := context.WithCancel(ctx)
	defer cancelExecution()
	agent, store, parityTracker, parityParentID, runtimeCtx, err := e.buildRuntime(
		executionCtx,
		run,
		run,
		messages,
		nil,
		0,
	)
	if err != nil {
		return nil, err
	}
	executionCtx = runtimeCtx

	checkpointKey := adkCheckpointKeyForRun(run.RunID)
	runner := adk.NewRunner(executionCtx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: store,
	})
	runOptions, cleanup, usageBridge := e.runOptions(
		run,
		checkpointKey,
		cancelExecution,
	)
	defer cleanup()
	iter := runner.Run(executionCtx, messages, runOptions...)

	result, err := e.consumeEvents(ctx, run, checkpointKey, store, iter, usageBridge, parityTracker, parityParentID)
	return result, normalizeADKExecutionError(ctx, err)
}

func (e *ADKExecutor) Resume(
	ctx context.Context,
	run *RunSummary,
	input *HarnessResumeInput,
) (*RunExecutionResult, error) {
	if err := e.validate(run); err != nil {
		return nil, err
	}
	if input == nil {
		return nil, fmt.Errorf("eino adk resume input is required")
	}
	if input.Runtime != RuntimeModeEinoADK {
		return nil, fmt.Errorf("resume input is not for the eino adk runtime")
	}
	if input.ADKCheckpoint == nil {
		return nil, fmt.Errorf("eino adk checkpoint envelope is required")
	}

	checkpointKey := strings.TrimSpace(input.RuntimeKey)
	if checkpointKey == "" {
		checkpointKey = strings.TrimSpace(input.ADKCheckpoint.RuntimeKey)
	}
	if checkpointKey == "" {
		return nil, fmt.Errorf("eino adk checkpoint runtime key is required")
	}
	if input.ADKCheckpoint.RuntimeKey != checkpointKey {
		return nil, fmt.Errorf("eino adk resume checkpoint key does not match envelope")
	}

	executionCtx, cancelExecution := context.WithCancel(ctx)
	defer cancelExecution()
	agentRun, err := adkAgentRunForResume(run, input.SourceRunID)
	if err != nil {
		return nil, err
	}
	var paritySeed *ADKParityState
	if input.ADKCheckpoint.ParityState != nil {
		copy := cloneADKParityState(*input.ADKCheckpoint.ParityState)
		paritySeed = &copy
	}
	agent, currentStore, parityTracker, parityParentID, runtimeCtx, err := e.buildRuntime(
		executionCtx,
		agentRun,
		run,
		nil,
		paritySeed,
		input.CheckpointID,
	)
	if err != nil {
		return nil, err
	}
	executionCtx = runtimeCtx
	store := currentStore
	if input.SourceRunID > 0 && input.SourceRunID != run.RunID {
		sourceRun := *run
		sourceRun.RunID = input.SourceRunID
		sourceStore, err := e.checkpointStoreFactory(&sourceRun)
		if err != nil {
			return nil, fmt.Errorf("build source eino adk checkpoint store: %w", err)
		}
		if sourceStore == nil {
			return nil, fmt.Errorf("source eino adk checkpoint store is required")
		}
		store = &adkResumeCheckpointStore{
			current: currentStore,
			source:  sourceStore,
		}
	}
	runner := adk.NewRunner(executionCtx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: store,
	})

	targets := copyADKResumeTargets(input.ADKResumeTargets)
	if len(targets) == 0 {
		targets = adkResumeTargets(input.ADKCheckpoint.Interrupts)
	}

	var iter *adk.AsyncIterator[*adk.AgentEvent]
	runOptions, cleanup, usageBridge := e.runOptions(
		run,
		checkpointKey,
		cancelExecution,
	)
	defer cleanup()
	if len(targets) > 0 {
		iter, err = runner.ResumeWithParams(executionCtx, checkpointKey, &adk.ResumeParams{
			Targets: targets,
		}, runOptions...)
	} else {
		iter, err = runner.Resume(executionCtx, checkpointKey, runOptions...)
	}
	if err != nil {
		return nil, fmt.Errorf("resume eino adk runner: %w", err)
	}

	result, err := e.consumeEvents(ctx, run, checkpointKey, store, iter, usageBridge, parityTracker, parityParentID)
	return result, normalizeADKExecutionError(ctx, err)
}

func normalizeADKExecutionError(parent context.Context, err error) error {
	if err == nil || !errors.Is(err, context.Canceled) {
		return err
	}
	if parent != nil && parent.Err() != nil {
		return err
	}

	return &RunCanceledError{EventPersisted: true}
}

func (e *ADKExecutor) runOptions(
	run *RunSummary,
	checkpointKey string,
	executionCancel context.CancelFunc,
) ([]adk.AgentRunOption, func(), *ADKUsageBridge) {
	options := []adk.AgentRunOption{adk.WithCheckPointID(checkpointKey)}
	var usageBridge *ADKUsageBridge
	if e != nil && e.usageCollector != nil {
		usageBridge = NewADKUsageBridge(run, e.usageCollector)
		options = append(options, adk.WithCallbacks(usageBridge.Handler()))
	}
	if e == nil || e.cancelRegistry == nil {
		return options, func() {}, usageBridge
	}

	cancelOption, cancel := adk.WithCancel()
	options = append(options, cancelOption)

	return options, e.cancelRegistry.RegisterWithExecutionCancel(
		run.RunID,
		cancel,
		executionCancel,
	), usageBridge
}

func (e *ADKExecutor) buildRuntime(
	ctx context.Context,
	agentRun *RunSummary,
	storeRun *RunSummary,
	messages []*schema.Message,
	seed *ADKParityState,
	parentCheckpointID int64,
) (adk.ResumableAgent, adk.CheckPointStore, *ADKParityStateTracker, int64, context.Context, error) {
	store, err := e.checkpointStoreFactory(storeRun)
	if err != nil {
		return nil, nil, nil, 0, ctx, fmt.Errorf("build eino adk checkpoint store: %w", err)
	}
	if store == nil {
		return nil, nil, nil, 0, ctx, fmt.Errorf("eino adk checkpoint store is required")
	}

	var parityTracker *ADKParityStateTracker
	parityParentID := parentCheckpointID
	if parityStore, ok := store.(ADKParityCheckpointStore); ok {
		if seed != nil {
			parityTracker, err = NewADKParityStateTracker(storeRun, seed)
			if err == nil {
				err = parityStore.SetParityStateTracker(parityTracker, parentCheckpointID)
			}
		} else {
			parityTracker, parityParentID, err = parityStore.ParityStateTracker(ctx)
		}
		if err != nil {
			return nil, nil, nil, 0, ctx, fmt.Errorf("initialize eino adk parity state: %w", err)
		}
		if err := seedADKParityStateFromRun(parityTracker, agentRun, messages, seed == nil); err != nil {
			return nil, nil, nil, 0, ctx, err
		}
		ctx = withADKParityStateTracker(ctx, parityTracker)
	}

	agent, err := e.factory.Build(ctx, agentRun)
	if err != nil {
		return nil, nil, nil, 0, ctx, err
	}
	if agent == nil {
		return nil, nil, nil, 0, ctx, fmt.Errorf("eino adk agent factory returned empty agent")
	}

	return agent, store, parityTracker, parityParentID, ctx, nil
}

func (e *ADKExecutor) consumeEvents(
	ctx context.Context,
	run *RunSummary,
	checkpointKey string,
	store adk.CheckPointStore,
	iter *adk.AsyncIterator[*adk.AgentEvent],
	usageBridge *ADKUsageBridge,
	parityTracker *ADKParityStateTracker,
	parityParentCheckpointID int64,
) (*RunExecutionResult, error) {
	if iter == nil {
		return nil, fmt.Errorf("eino adk runner returned empty event iterator")
	}

	finalText := ""
	var interrupted *RunInterruptedError
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		mapped, err := MapADKEvent(ctx, run.ThreadID, run.RunID, event)
		if err != nil {
			return nil, err
		}
		if err := e.eventSink.EmitRunEvent(ctx, mapped.RunEvent); err != nil {
			return nil, fmt.Errorf("persist eino adk event %s: %w", mapped.EventType, err)
		}
		if mapped.Usage != nil && usageBridge != nil {
			if err := usageBridge.RecordEvent(ctx, *mapped.Usage); err != nil {
				return nil, fmt.Errorf("record eino adk token usage: %w", err)
			}
		} else if mapped.Usage != nil && e.usageCollector != nil {
			if err := e.usageCollector.Record(ctx, run, *mapped.Usage); err != nil {
				return nil, fmt.Errorf("record eino adk token usage: %w", err)
			}
		}
		if text := strings.TrimSpace(mapped.FinalText); text != "" {
			finalText = text
			if parityTracker != nil {
				if err := parityTracker.AppendMessage(ADKParityMessage{
					RunID: run.RunID, Role: "assistant", Content: text,
				}); err != nil {
					return nil, fmt.Errorf("record eino adk parity assistant message: %w", err)
				}
			}
		}
		if mapped.Interrupt != nil && len(mapped.Interrupt.Items) > 0 {
			interrupted = &RunInterruptedError{
				CheckpointKey:  checkpointKey,
				Interrupts:     append([]ADKInterruptItem(nil), mapped.Interrupt.Items...),
				EventPersisted: true,
			}
			if recorder, ok := store.(ADKCheckpointInterruptRecorder); ok {
				if err := recorder.RecordInterrupts(
					ctx,
					checkpointKey,
					mapped.Interrupt.Items,
				); err != nil {
					return nil, fmt.Errorf("persist eino adk interrupt targets: %w", err)
				}
			}
		}

		if event != nil && event.Err != nil {
			var retrying *adk.WillRetryError
			if errors.As(event.Err, &retrying) {
				continue
			}
			if isADKCancellationError(event.Err) {
				return nil, &RunCanceledError{EventPersisted: true}
			}

			return nil, event.Err
		}
	}

	if usageBridge != nil {
		if err := usageBridge.Err(); err != nil {
			return nil, fmt.Errorf("record eino adk callback usage: %w", err)
		}
	}
	if interrupted != nil {
		return nil, interrupted
	}
	if finalText == "" {
		return nil, fmt.Errorf("eino adk runner returned empty assistant message")
	}
	metadataPayload := map[string]string{
		"source":         "eino_adk",
		"checkpoint_key": checkpointKey,
	}
	if usageBridge != nil {
		if traceID := usageBridge.TraceID(); traceID != "" {
			metadataPayload["trace_id"] = traceID
		}
	}
	metadata, err := json.Marshal(metadataPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal eino adk execution metadata: %w", err)
	}

	result := &RunExecutionResult{
		Message:  finalText,
		Metadata: string(metadata),
	}
	if parityTracker != nil {
		if err := parityTracker.ReplaceInterrupts([]ADKParityInterrupt{}); err != nil {
			return nil, fmt.Errorf("clear eino adk parity interrupts: %w", err)
		}
		if err := parityTracker.SetCompletion(ADKParityCompletion{
			RunID: run.RunID, Status: "succeeded", Reason: "completed",
			CompletedAt: time.Now().UnixMilli(),
		}); err != nil {
			return nil, fmt.Errorf("record eino adk parity completion: %w", err)
		}
		snapshot := parityTracker.Snapshot()
		result.ParityState = &snapshot
		result.ParityParentCheckpointID = parityParentCheckpointID
	}
	return result, nil
}

func seedADKParityStateFromRun(
	tracker *ADKParityStateTracker,
	run *RunSummary,
	messages []*schema.Message,
	replaceMessages bool,
) error {
	if tracker == nil || run == nil {
		return nil
	}
	if err := tracker.ClearCompletion(); err != nil {
		return fmt.Errorf("clear eino adk parity completion: %w", err)
	}
	if err := tracker.ReplaceActiveSkills([]ADKParitySkill{}); err != nil {
		return fmt.Errorf("clear eino adk parity skills: %w", err)
	}
	parityMessages := make([]ADKParityMessage, 0, len(messages))
	for _, message := range messages {
		if message == nil || (message.Role != schema.User && message.Role != schema.Assistant) ||
			adkParityHiddenMessage(message) {
			continue
		}
		content := stripADKUploadedFilesContext(message.Content)
		if strings.TrimSpace(content) == "" {
			continue
		}
		parityMessages = append(parityMessages, ADKParityMessage{
			RunID: run.RunID, Role: string(message.Role), Content: content,
		})
	}
	if replaceMessages {
		if err := tracker.ReplaceMessages(parityMessages, nil); err != nil {
			return fmt.Errorf("seed eino adk parity messages: %w", err)
		}
	} else {
		for _, message := range parityMessages {
			if err := tracker.AppendMessage(message); err != nil {
				return fmt.Errorf("append eino adk parity resume message: %w", err)
			}
		}
	}
	if err := tracker.MergeUploads(adkParityUploadsFromSummaries(
		uploadedFilesFromRunInput(run.Input),
	)); err != nil {
		return fmt.Errorf("seed eino adk parity uploads: %w", err)
	}
	return nil
}

func (e *ADKExecutor) validate(run *RunSummary) error {
	if e == nil {
		return fmt.Errorf("eino adk executor is required")
	}
	if e.factory == nil {
		return fmt.Errorf("eino adk agent factory is required")
	}
	if e.eventSink == nil {
		return fmt.Errorf("eino adk event sink is required")
	}
	if e.checkpointStoreFactory == nil {
		return fmt.Errorf("eino adk checkpoint store factory is required")
	}
	if run == nil {
		return fmt.Errorf("run is required")
	}
	if run.ThreadID <= 0 {
		return fmt.Errorf("run thread id is required")
	}
	if run.RunID <= 0 {
		return fmt.Errorf("run id is required")
	}

	return nil
}

func adkAgentRunForResume(
	run *RunSummary,
	sourceRunID int64,
) (*RunSummary, error) {
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if sourceRunID < 0 {
		return nil, fmt.Errorf("eino adk source run id is invalid")
	}
	agentRun := *run
	if sourceRunID > 0 {
		agentRun.PlanScopeRunID = sourceRunID
	}
	return &agentRun, nil
}

func adkCheckpointKeyForRun(runID int64) string {
	return "coze-run-" + strconv.FormatInt(runID, 10)
}

func copyADKResumeTargets(targets map[string]any) map[string]any {
	if len(targets) == 0 {
		return nil
	}

	copied := make(map[string]any, len(targets))
	for key, value := range targets {
		if key = strings.TrimSpace(key); key != "" {
			copied[key] = value
		}
	}

	return copied
}

type adkResumeCheckpointStore struct {
	current adk.CheckPointStore
	source  adk.CheckPointStore
}

func (s *adkResumeCheckpointStore) Get(
	ctx context.Context,
	checkpointID string,
) ([]byte, bool, error) {
	if s == nil || s.current == nil || s.source == nil {
		return nil, false, fmt.Errorf("resume checkpoint stores are required")
	}

	value, exists, err := s.current.Get(ctx, checkpointID)
	if err != nil || exists {
		return value, exists, err
	}

	return s.source.Get(ctx, checkpointID)
}

func (s *adkResumeCheckpointStore) Set(
	ctx context.Context,
	checkpointID string,
	checkpoint []byte,
) error {
	if s == nil || s.current == nil {
		return fmt.Errorf("current resume checkpoint store is required")
	}

	return s.current.Set(ctx, checkpointID, checkpoint)
}

func (s *adkResumeCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	if s == nil {
		return fmt.Errorf("resume checkpoint store is required")
	}

	var deleteErrors []error
	if deleter, ok := s.current.(adk.CheckPointDeleter); ok {
		if err := deleter.Delete(ctx, checkpointID); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("delete current checkpoint: %w", err))
		}
	}
	if deleter, ok := s.source.(adk.CheckPointDeleter); ok {
		if err := deleter.Delete(ctx, checkpointID); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("delete source checkpoint: %w", err))
		}
	}

	return errors.Join(deleteErrors...)
}

func (s *adkResumeCheckpointStore) RecordInterrupts(
	ctx context.Context,
	checkpointID string,
	interrupts []ADKInterruptItem,
) error {
	if s == nil || s.current == nil {
		return fmt.Errorf("current resume checkpoint store is required")
	}
	recorder, ok := s.current.(ADKCheckpointInterruptRecorder)
	if !ok {
		return nil
	}

	return recorder.RecordInterrupts(ctx, checkpointID, interrupts)
}

var _ adk.CheckPointStore = (*adkResumeCheckpointStore)(nil)
var _ adk.CheckPointDeleter = (*adkResumeCheckpointStore)(nil)
var _ ADKCheckpointInterruptRecorder = (*adkResumeCheckpointStore)(nil)
