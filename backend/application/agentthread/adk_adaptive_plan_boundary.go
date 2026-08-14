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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
)

const adkAdaptivePlanBoundaryIdentitySchema = "workbench-adaptive-plan-boundary.v1"

type adkAdaptivePlanBoundaryContextKey struct{}

func withADKAdaptivePlanBoundaryCoordinator(
	ctx context.Context,
	coordinator *ADKAdaptivePlanBoundaryCoordinator,
) context.Context {
	if coordinator == nil {
		return ctx
	}
	return context.WithValue(ctx, adkAdaptivePlanBoundaryContextKey{}, coordinator)
}

func ADKAdaptivePlanBoundaryCoordinatorFromContext(
	ctx context.Context,
) *ADKAdaptivePlanBoundaryCoordinator {
	if ctx == nil {
		return nil
	}
	coordinator, _ := ctx.Value(adkAdaptivePlanBoundaryContextKey{}).(*ADKAdaptivePlanBoundaryCoordinator)
	return coordinator
}

type ADKAdaptivePlanBoundaryRepository interface {
	ReadAdaptiveExecutionBoundary(
		context.Context,
		domainrepo.ReadAdaptiveExecutionBoundaryRequest,
	) (*domainrepo.CommitAdaptiveExecutionBoundaryResult, error)
	CommitAdaptiveExecutionBoundary(
		context.Context,
		domainrepo.CommitAdaptiveExecutionBoundaryRequest,
	) (*domainrepo.CommitAdaptiveExecutionBoundaryResult, error)
}

type ADKAdaptivePlanBoundaryIDGenerator interface {
	GenID(context.Context) (int64, error)
}

type adkAdaptivePlanPending struct {
	generation      uint64
	toolCallID      string
	addresses       map[string]struct{}
	expected        *ADKPlanSnapshot
	next            *ADKPlanSnapshot
	mutations       map[int64]*domainrepo.AdaptivePlanItemMutation
	eventTaskID     int64
	eventType       string
	highWatermarked bool
	idempotencyKey  string
	mutationDigest  string
	frozen          *domainrepo.CommitAdaptiveExecutionBoundaryRequest
}

// ADKAdaptivePlanBoundaryCoordinator keeps Plan mutations in a run-scoped
// overlay until Eino supplies the real checkpoint at its safe cancellation
// point. The repository then commits event, checkpoint, Plan and items once.
type ADKAdaptivePlanBoundaryCoordinator struct {
	run        *RunSummary
	repository ADKAdaptivePlanBoundaryRepository
	idGen      ADKAdaptivePlanBoundaryIDGenerator
	planStore  ADKPlanStore
	now        func() int64

	mu                  sync.Mutex
	loaded              bool
	overlay             *ADKPlanSnapshot
	pending             *adkAdaptivePlanPending
	nextGeneration      uint64
	committedGeneration uint64
}

func NewADKAdaptivePlanBoundaryCoordinator(
	run *RunSummary,
	repository ADKAdaptivePlanBoundaryRepository,
	idGen ADKAdaptivePlanBoundaryIDGenerator,
	planStore ADKPlanStore,
	now func() int64,
) (*ADKAdaptivePlanBoundaryCoordinator, error) {
	if run == nil || run.RunID <= 0 || run.ThreadID <= 0 || run.SpaceID <= 0 ||
		run.CreatorID <= 0 || run.ExecutionGeneration == 0 ||
		strings.TrimSpace(run.LeaseOwner) == "" || strings.TrimSpace(run.LeaseToken) == "" {
		return nil, fmt.Errorf("adaptive plan boundary run authority is invalid")
	}
	if repository == nil || idGen == nil || planStore == nil || now == nil {
		return nil, fmt.Errorf("adaptive plan boundary dependencies are required")
	}
	return &ADKAdaptivePlanBoundaryCoordinator{
		run: run, repository: repository, idGen: idGen, planStore: planStore, now: now,
	}, nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) PendingGeneration() uint64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		return c.committedGeneration
	}
	return c.pending.generation
}

func (c *ADKAdaptivePlanBoundaryCoordinator) CommittedGeneration() uint64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.committedGeneration
}

func (c *ADKAdaptivePlanBoundaryCoordinator) snapshot(
	ctx context.Context,
	scope ADKPlanScope,
) (*ADKPlanSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.validateScope(scope); err != nil {
		return nil, err
	}
	if err := c.loadLocked(ctx, scope); err != nil {
		return nil, err
	}
	return cloneADKPlanSnapshot(c.overlay), nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) task(
	ctx context.Context,
	scope ADKPlanScope,
	taskID int64,
) (*ADKPlanTask, error) {
	snapshot, err := c.snapshot(ctx, scope)
	if err != nil {
		return nil, err
	}
	for _, task := range snapshot.Tasks {
		if task != nil && task.TaskID == taskID && task.Active {
			return cloneAdaptiveADKPlanTask(task), nil
		}
	}
	return nil, fmt.Errorf("eino adk plan task %d is not active", taskID)
}

func (c *ADKAdaptivePlanBoundaryCoordinator) stageHighWatermark(
	ctx context.Context,
	toolCallID string,
	address string,
	expected int64,
	next int64,
) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLocked(ctx, c.scope()); err != nil {
		return err
	}
	pending, err := c.pendingForWriteLocked(toolCallID, address)
	if err != nil {
		return err
	}
	if pending.next.HighWatermark != expected || next != expected+1 {
		return fmt.Errorf("adaptive plan high watermark conflict")
	}
	pending.next.HighWatermark = next
	pending.highWatermarked = true
	c.overlay = cloneADKPlanSnapshot(pending.next)
	return nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) stageTask(
	ctx context.Context,
	toolCallID string,
	address string,
	task *ADKPlanTask,
) error {
	return c.stageTaskWithParity(ctx, toolCallID, address, task, nil)
}

func (c *ADKAdaptivePlanBoundaryCoordinator) stageTaskWithParity(
	ctx context.Context,
	toolCallID string,
	address string,
	task *ADKPlanTask,
	parityTracker *ADKParityStateTracker,
) error {
	if task == nil {
		return fmt.Errorf("adaptive plan task is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.loadLocked(ctx, c.scope()); err != nil {
		return err
	}
	previousPending := cloneADKAdaptivePlanPending(c.pending)
	previousOverlay := cloneADKPlanSnapshot(c.overlay)
	previousNextGeneration := c.nextGeneration
	rollback := func() {
		c.pending = previousPending
		c.overlay = previousOverlay
		c.nextGeneration = previousNextGeneration
	}
	pending, err := c.pendingForWriteLocked(toolCallID, address)
	if err != nil {
		return err
	}
	if task.TaskID > pending.next.HighWatermark {
		return fmt.Errorf("adaptive plan task exceeds high watermark")
	}
	current := planTaskFromSnapshot(pending.expected, task.TaskID)
	if previousMutation := pending.mutations[task.TaskID]; previousMutation != nil {
		current = domainPlanTask(previousMutation.NextItem)
	}
	created := current == nil
	recordID := task.RecordID
	expectedVersion := int64(0)
	createdAt := int64(0)
	updatedAt := int64(0)
	if current != nil {
		recordID = current.RecordID
		if previousMutation := pending.mutations[task.TaskID]; previousMutation != nil {
			expectedVersion = previousMutation.ExpectedVersion
		} else {
			expectedVersion = current.Version
		}
		createdAt = current.CreatedAt
		updatedAt = current.UpdatedAt
	}
	nextTask := cloneAdaptiveADKPlanTask(task)
	nextTask.RecordID = recordID
	nextTask.Active = true
	nextTask.Version = expectedVersion + 1
	nextTask.CreatedAt = createdAt
	nextTask.UpdatedAt = updatedAt
	item, err := adkAdaptiveDomainPlanItem(c.scope(), nextTask)
	if err != nil {
		return err
	}
	pending.mutations[task.TaskID] = &domainrepo.AdaptivePlanItemMutation{
		ExpectedVersion: expectedVersion,
		NextItem:        item,
	}
	replaceADKPlanTask(pending.next, nextTask)
	pending.eventTaskID = task.TaskID
	pending.eventType = adkPlanEventType(nextTask, current, created)
	pending.next.Revision = pending.expected.Revision + 1
	if parityTracker != nil {
		if err := parityTracker.ReplaceTodos(adkParityTodosFromPlanSnapshot(pending.next)); err != nil {
			rollback()
			return fmt.Errorf("record adaptive plan parity todos: %w", err)
		}
	}
	c.overlay = cloneADKPlanSnapshot(pending.next)
	return nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) stageDelete(
	ctx context.Context,
	toolCallID string,
	address string,
	taskID int64,
	parityTracker *ADKParityStateTracker,
) error {
	task, err := c.task(ctx, c.scope(), taskID)
	if err != nil {
		return err
	}
	task.Active = false
	if task.Status != "completed" {
		task.Status = "deleted"
	}
	return c.stageTaskWithParity(ctx, toolCallID, address, task, parityTracker)
}

func (c *ADKAdaptivePlanBoundaryCoordinator) commitCheckpoint(
	ctx context.Context,
	input ADKSideEffectCheckpointInput,
	attempt *domainentity.RunAttempt,
) (*domainrepo.CommitAdaptiveExecutionBoundaryResult, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		return nil, false, nil
	}
	if len(c.pending.mutations) == 0 || c.pending.eventTaskID <= 0 || c.pending.eventType == "" {
		return nil, true, fmt.Errorf("adaptive plan checkpoint boundary has no complete task mutation")
	}
	if attempt == nil || attempt.ThreadID != c.run.ThreadID ||
		attempt.ExecutionRunID != c.run.RunID || attempt.JournalRunID <= 0 ||
		attempt.AttemptID == "" || attempt.AttemptID != c.bootstrapAttemptID(ctx) ||
		attempt.NextSequence == 0 {
		return nil, true, fmt.Errorf("adaptive plan checkpoint attempt authority is invalid")
	}
	if c.pending.frozen != nil {
		if err := sameADKAdaptiveCheckpointInput(c.pending.frozen.Checkpoint, input); err != nil {
			return nil, true, err
		}
	}
	if c.pending.idempotencyKey == "" {
		c.pending.idempotencyKey = c.pending.stableIdempotencyKey(c.run, attempt)
	}
	if c.pending.mutationDigest == "" {
		mutation, err := c.pending.planMutation(c.scope())
		if err != nil {
			return nil, true, err
		}
		c.pending.mutationDigest, err = domainrepo.AdaptiveExecutionPlanMutationDigest(mutation)
		if err != nil {
			return nil, true, fmt.Errorf("digest adaptive plan boundary: %w", err)
		}
	}
	readResult, err := c.repository.ReadAdaptiveExecutionBoundary(
		ctx,
		domainrepo.ReadAdaptiveExecutionBoundaryRequest{
			ThreadID: c.run.ThreadID, ExecutionRunID: c.run.RunID,
			JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
			Generation: c.run.ExecutionGeneration, RuntimeKey: input.RuntimeKey,
			IdempotencyKey:         c.pending.idempotencyKey,
			ExpectedMutationDigest: c.pending.mutationDigest,
		},
	)
	if err == nil {
		if err := c.applyCommittedResultLocked(readResult); err != nil {
			return nil, true, err
		}
		return readResult, true, nil
	}
	if !errors.Is(err, domainrepo.ErrAdaptiveExecutionBoundaryNotFound) {
		return nil, true, fmt.Errorf("read adaptive plan checkpoint boundary: %w", err)
	}
	if c.pending.frozen == nil {
		request, err := c.freezeRequestLocked(ctx, input, attempt)
		if err != nil {
			return nil, true, err
		}
		c.pending.frozen = request
	}
	result, err := c.repository.CommitAdaptiveExecutionBoundary(ctx, *c.pending.frozen)
	if err != nil {
		return nil, true, err
	}
	if result == nil || result.Event == nil || result.Checkpoint == nil || result.Plan == nil {
		return nil, true, fmt.Errorf("adaptive plan boundary returned empty result")
	}
	if err := c.applyCommittedResultLocked(result); err != nil {
		return nil, true, err
	}
	return result, true, nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) applyCommittedResultLocked(
	result *domainrepo.CommitAdaptiveExecutionBoundaryResult,
) error {
	if result == nil || result.Event == nil || result.Checkpoint == nil || result.Plan == nil || c.pending == nil {
		return fmt.Errorf("adaptive plan boundary returned empty result")
	}
	next := cloneADKPlanSnapshot(c.pending.next)
	next.HighWatermark = result.Plan.HighWatermark
	next.Revision = result.Plan.Revision
	for _, item := range result.Items {
		replaceADKPlanTask(next, domainPlanTask(item))
	}
	c.overlay = next
	c.committedGeneration = c.pending.generation
	c.pending = nil
	return nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) freezeRequestLocked(
	ctx context.Context,
	input ADKSideEffectCheckpointInput,
	attempt *domainentity.RunAttempt,
) (*domainrepo.CommitAdaptiveExecutionBoundaryRequest, error) {
	if err := c.allocateCreatedItemIDsLocked(ctx); err != nil {
		return nil, err
	}
	eventID, err := c.idGen.GenID(ctx)
	if err != nil {
		return nil, fmt.Errorf("allocate adaptive plan event id: %w", err)
	}
	checkpointID, err := c.idGen.GenID(ctx)
	if err != nil {
		return nil, fmt.Errorf("allocate adaptive plan checkpoint id: %w", err)
	}
	now := c.now()
	if err := c.applyMutationTimestampsLocked(now); err != nil {
		return nil, err
	}
	task := planTaskFromSnapshot(c.pending.next, c.pending.eventTaskID)
	if task == nil {
		return nil, fmt.Errorf("adaptive plan event task is missing")
	}
	payload := adkPlanMutationPayload(c.scope(), c.pending.next, task)
	parityState := cloneADKParityState(*input.ParityState)
	envelope := ADKCheckpointEnvelope{
		EnvelopeVersion: adkJournalCheckpointEnvelopeVersion,
		SchemaVersion:   adkJournalCheckpointSchemaVersion,
		Runtime:         string(RuntimeModeEinoADK), RuntimeVersion: input.RuntimeVersion,
		RuntimeKey: input.RuntimeKey, MessageType: adkCheckpointMessageType,
		CheckpointPhase: ADKCheckpointPhaseRuntime,
		RuntimeState:    &ADKCheckpointRuntimeState{Checkpoint: append([]byte(nil), input.RuntimeState...)},
		AttemptID:       attempt.AttemptID, LastCommittedSequence: attempt.NextSequence,
		SideEffectLedger: []ADKSideEffectLedgerReference{}, ParityState: &parityState,
		RunRevision: input.RunRevision, CreatedAt: now,
	}
	raw, err := envelope.Marshal()
	if err != nil {
		return nil, err
	}
	planMutation, err := c.pending.planMutation(c.scope())
	if err != nil {
		return nil, err
	}
	return &domainrepo.CommitAdaptiveExecutionBoundaryRequest{
		ThreadID: c.run.ThreadID, ExecutionRunID: c.run.RunID,
		JournalRunID: attempt.JournalRunID, AttemptID: attempt.AttemptID,
		Generation: c.run.ExecutionGeneration,
		LeaseOwner: c.run.LeaseOwner, LeaseToken: c.run.LeaseToken,
		Now: now, IdempotencyKey: c.pending.idempotencyKey,
		Event: &domainentity.RunEvent{
			ID: eventID, ThreadID: c.run.ThreadID, RunID: c.run.RunID,
			EventType: c.pending.eventType, Payload: encodeRunEventPayload(ctx, payload), CreatedAt: now,
		},
		Checkpoint: &domainentity.Checkpoint{
			ID: checkpointID, ThreadID: c.run.ThreadID, RunID: c.run.RunID,
			ParentCheckpointID: input.ParentCheckpointID,
			CheckpointNS:       adkCheckpointNamespace, RuntimeType: string(RuntimeModeEinoADK),
			RuntimeKey: input.RuntimeKey, EnvelopeVersion: adkJournalCheckpointEnvelopeVersion,
			ChannelValues: string(raw), ChannelVersions: `{}`, PendingSends: `[]`,
			Metadata: adkCheckpointMetadataJSONVersion(
				input.RuntimeVersion, input.RuntimeKey, ADKCheckpointPhaseRuntime,
				adkJournalCheckpointEnvelopeVersion,
			),
			CreatedAt: now,
		},
		PlanMutation: planMutation,
	}, nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) applyMutationTimestampsLocked(now int64) error {
	for taskID, mutation := range c.pending.mutations {
		if mutation == nil || mutation.NextItem == nil {
			return fmt.Errorf("adaptive plan mutation item is incomplete")
		}
		if mutation.ExpectedVersion == 0 && mutation.NextItem.CreatedAt == 0 {
			mutation.NextItem.CreatedAt = now
		}
		mutation.NextItem.UpdatedAt = now
		if task := planTaskFromSnapshot(c.pending.next, taskID); task != nil {
			task.CreatedAt = mutation.NextItem.CreatedAt
			task.UpdatedAt = now
			replaceADKPlanTask(c.pending.next, task)
		}
	}
	return nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) allocateCreatedItemIDsLocked(
	ctx context.Context,
) error {
	for taskID, mutation := range c.pending.mutations {
		if mutation == nil || mutation.NextItem == nil {
			return fmt.Errorf("adaptive plan mutation item is incomplete")
		}
		if mutation.ExpectedVersion != 0 || mutation.NextItem.ID > 0 {
			continue
		}
		itemID, err := c.idGen.GenID(ctx)
		if err != nil {
			return fmt.Errorf("allocate adaptive plan item id: %w", err)
		}
		mutation.NextItem.ID = itemID
		if task := planTaskFromSnapshot(c.pending.next, taskID); task != nil {
			task.RecordID = itemID
			replaceADKPlanTask(c.pending.next, task)
		}
	}
	return nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) pendingForWriteLocked(
	toolCallID string,
	address string,
) (*adkAdaptivePlanPending, error) {
	toolCallID = strings.TrimSpace(toolCallID)
	address = strings.TrimSpace(address)
	if toolCallID == "" || address == "" {
		return nil, fmt.Errorf("adaptive plan tool call identity is required")
	}
	if c.pending == nil {
		c.nextGeneration++
		c.pending = &adkAdaptivePlanPending{
			generation: c.nextGeneration, toolCallID: toolCallID,
			addresses: map[string]struct{}{address: {}},
			expected:  cloneADKPlanSnapshot(c.overlay), next: cloneADKPlanSnapshot(c.overlay),
			mutations: make(map[int64]*domainrepo.AdaptivePlanItemMutation),
		}
		return c.pending, nil
	}
	if c.pending.frozen != nil {
		return nil, fmt.Errorf("adaptive plan boundary is already frozen")
	}
	if c.pending.toolCallID != toolCallID {
		return nil, fmt.Errorf("adaptive plan boundary already has a different tool call")
	}
	c.pending.addresses[address] = struct{}{}
	return c.pending, nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) loadLocked(
	ctx context.Context,
	scope ADKPlanScope,
) error {
	if c.loaded {
		return nil
	}
	snapshot, err := c.planStore.OpenPlan(ctx, scope)
	if err != nil {
		return fmt.Errorf("load adaptive plan snapshot: %w", err)
	}
	if snapshot == nil {
		return fmt.Errorf("adaptive plan store returned empty snapshot")
	}
	c.overlay = cloneADKPlanSnapshot(snapshot)
	c.loaded = true
	return nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) validateScope(scope ADKPlanScope) error {
	expected := c.scope()
	if scope != expected {
		return fmt.Errorf("adaptive plan scope does not belong to run")
	}
	return nil
}

func (c *ADKAdaptivePlanBoundaryCoordinator) scope() ADKPlanScope {
	scopeRunID := c.run.PlanScopeRunID
	if scopeRunID == 0 {
		scopeRunID = c.run.RunID
	}
	return ADKPlanScope{
		ActiveRunID: c.run.RunID, ScopeRunID: scopeRunID, ThreadID: c.run.ThreadID,
		SpaceID: c.run.SpaceID, UserID: c.run.CreatorID,
	}
}

func (c *ADKAdaptivePlanBoundaryCoordinator) bootstrapAttemptID(ctx context.Context) string {
	facts, ok := adaptiveBootstrapFactsFromContext(ctx)
	if !ok || !adkAdaptivePlanFactsApply(c.run, facts) {
		return ""
	}
	return facts.Decision.AttemptID
}

func adkAdaptivePlanFactsApply(run *RunSummary, facts *AdaptiveBootstrapFacts) bool {
	if run == nil || facts == nil || !facts.Admission.Capabilities.PlanAllowed ||
		facts.Decision.Decision != domainentity.ExecutionDecisionExecute ||
		facts.Decision.ExecutionShape != domainentity.ExecutionShapeMultiStep ||
		facts.Decision.ExecutionRunID != run.RunID ||
		facts.Decision.ExecutionGeneration != run.ExecutionGeneration ||
		facts.Decision.JournalRunID <= 0 || strings.TrimSpace(facts.Decision.AttemptID) == "" {
		return false
	}
	scopeRunID := run.PlanScopeRunID
	if scopeRunID == 0 {
		scopeRunID = run.RunID
	}
	return facts.Decision.PlanScopeRunID != nil && *facts.Decision.PlanScopeRunID == scopeRunID
}

func (p *adkAdaptivePlanPending) stableIdempotencyKey(
	run *RunSummary,
	attempt *domainentity.RunAttempt,
) string {
	addresses := make([]string, 0, len(p.addresses))
	for address := range p.addresses {
		addresses = append(addresses, address)
	}
	sort.Strings(addresses)
	input := strings.Join([]string{
		adkAdaptivePlanBoundaryIdentitySchema,
		strconv.FormatInt(run.ThreadID, 10), strconv.FormatInt(run.RunID, 10),
		strconv.FormatInt(attempt.JournalRunID, 10), attempt.AttemptID,
		strconv.FormatUint(run.ExecutionGeneration, 10), p.toolCallID,
		strings.Join(addresses, "\x01"),
	}, "\x00")
	digest := sha256.Sum256([]byte(input))
	return "adaptive-plan:" + hex.EncodeToString(digest[:])
}

func (p *adkAdaptivePlanPending) planMutation(
	scope ADKPlanScope,
) (*domainrepo.AdaptivePlanMutation, error) {
	if p == nil || p.expected == nil || p.next == nil || len(p.mutations) == 0 {
		return nil, fmt.Errorf("adaptive plan mutation is incomplete")
	}
	mutations := make([]domainrepo.AdaptivePlanItemMutation, 0, len(p.mutations))
	for _, mutation := range p.mutations {
		if mutation == nil || mutation.NextItem == nil {
			return nil, fmt.Errorf("adaptive plan mutation item is incomplete")
		}
		copy := *mutation.NextItem
		mutations = append(mutations, domainrepo.AdaptivePlanItemMutation{
			ExpectedVersion: mutation.ExpectedVersion, NextItem: &copy,
		})
	}
	sort.Slice(mutations, func(i, j int) bool {
		return mutations[i].NextItem.TaskID < mutations[j].NextItem.TaskID
	})
	return &domainrepo.AdaptivePlanMutation{
		PlanScopeRunID:   scope.ScopeRunID,
		ExpectedRevision: p.expected.Revision, NextRevision: p.next.Revision,
		ExpectedHighWatermark: p.expected.HighWatermark,
		NextHighWatermark:     p.next.HighWatermark, Items: mutations,
	}, nil
}

func adkAdaptiveDomainPlanItem(
	scope ADKPlanScope,
	task *ADKPlanTask,
) (*domainentity.AgentRunPlanItem, error) {
	blocks, err := json.Marshal(task.Blocks)
	if err != nil {
		return nil, err
	}
	blockedBy, err := json.Marshal(task.BlockedBy)
	if err != nil {
		return nil, err
	}
	metadata := []byte(`{}`)
	if task.Metadata != nil {
		metadata, err = json.Marshal(task.Metadata)
		if err != nil {
			return nil, err
		}
	}
	return &domainentity.AgentRunPlanItem{
		ID: task.RecordID, RunID: scope.ScopeRunID, TaskID: task.TaskID,
		Subject: task.Subject, Description: task.Description,
		Status:     domainentity.AgentRunPlanItemStatus(task.Status),
		ActiveForm: task.ActiveForm, Owner: task.Owner,
		Blocks: string(blocks), BlockedBy: string(blockedBy), Metadata: string(metadata),
		Active: task.Active, Version: task.Version,
		CreatedAt: task.CreatedAt, UpdatedAt: task.UpdatedAt,
	}, nil
}

func adkPlanEventType(task, previous *ADKPlanTask, created bool) string {
	if !task.Active && task.Status == "deleted" {
		return "plan.task.deleted"
	}
	if created {
		return "plan.task.created"
	}
	if task.Status == "completed" && (previous == nil || previous.Status != "completed") {
		return "plan.task.completed"
	}
	return "plan.task.updated"
}

func adkPlanMutationPayload(
	scope ADKPlanScope,
	snapshot *ADKPlanSnapshot,
	task *ADKPlanTask,
) map[string]any {
	activeCount := 0
	completedCount := 0
	for _, item := range snapshot.Tasks {
		if item == nil || !item.Active {
			continue
		}
		activeCount++
		if item.Status == "completed" {
			completedCount++
		}
	}
	payload := map[string]any{
		"plan_scope_run_id": scope.ScopeRunID, "plan_task_id": task.ID,
		"subject": task.Subject, "status": task.Status,
		"active_form": task.ActiveForm, "owner": task.Owner,
		"blocks": task.Blocks, "blocked_by": task.BlockedBy,
		"revision": snapshot.Revision, "active_count": activeCount,
		"completed_count": completedCount, "total_count": activeCount,
	}
	if intro, explicit := adkPlanExecutionIntro(snapshot); intro != "" {
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if explicit || (status != "pending" && status != "planned") {
			payload[adkPlanExecutionIntroMetadataKey] = intro
		}
	}
	return payload
}

func cloneADKPlanSnapshot(snapshot *ADKPlanSnapshot) *ADKPlanSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := &ADKPlanSnapshot{HighWatermark: snapshot.HighWatermark, Revision: snapshot.Revision}
	clone.Tasks = make([]*ADKPlanTask, 0, len(snapshot.Tasks))
	for _, task := range snapshot.Tasks {
		clone.Tasks = append(clone.Tasks, cloneAdaptiveADKPlanTask(task))
	}
	return clone
}

func cloneADKAdaptivePlanPending(
	pending *adkAdaptivePlanPending,
) *adkAdaptivePlanPending {
	if pending == nil {
		return nil
	}
	clone := *pending
	clone.addresses = make(map[string]struct{}, len(pending.addresses))
	for address := range pending.addresses {
		clone.addresses[address] = struct{}{}
	}
	clone.expected = cloneADKPlanSnapshot(pending.expected)
	clone.next = cloneADKPlanSnapshot(pending.next)
	clone.mutations = make(map[int64]*domainrepo.AdaptivePlanItemMutation, len(pending.mutations))
	for taskID, mutation := range pending.mutations {
		if mutation == nil {
			clone.mutations[taskID] = nil
			continue
		}
		mutationClone := *mutation
		if mutation.NextItem != nil {
			itemClone := *mutation.NextItem
			mutationClone.NextItem = &itemClone
		}
		clone.mutations[taskID] = &mutationClone
	}
	if pending.frozen != nil {
		frozen := *pending.frozen
		clone.frozen = &frozen
	}
	return &clone
}

func cloneAdaptiveADKPlanTask(task *ADKPlanTask) *ADKPlanTask {
	if task == nil {
		return nil
	}
	clone := *task
	clone.Blocks = append([]string(nil), task.Blocks...)
	clone.BlockedBy = append([]string(nil), task.BlockedBy...)
	clone.Metadata = make(map[string]any, len(task.Metadata))
	for key, value := range task.Metadata {
		clone.Metadata[key] = value
	}
	return &clone
}

func planTaskFromSnapshot(snapshot *ADKPlanSnapshot, taskID int64) *ADKPlanTask {
	if snapshot == nil {
		return nil
	}
	for _, task := range snapshot.Tasks {
		if task != nil && task.TaskID == taskID {
			return cloneAdaptiveADKPlanTask(task)
		}
	}
	return nil
}

func replaceADKPlanTask(snapshot *ADKPlanSnapshot, task *ADKPlanTask) {
	for index, current := range snapshot.Tasks {
		if current != nil && current.TaskID == task.TaskID {
			snapshot.Tasks[index] = cloneAdaptiveADKPlanTask(task)
			return
		}
	}
	snapshot.Tasks = append(snapshot.Tasks, cloneAdaptiveADKPlanTask(task))
}

func adkPlanSnapshotFromBoundaryResult(
	result *domainrepo.CommitAdaptiveExecutionBoundaryResult,
) *ADKPlanSnapshot {
	snapshot := &ADKPlanSnapshot{
		HighWatermark: result.Plan.HighWatermark, Revision: result.Plan.Revision,
		Tasks: make([]*ADKPlanTask, 0, len(result.Items)),
	}
	for _, item := range result.Items {
		snapshot.Tasks = append(snapshot.Tasks, domainPlanTask(item))
	}
	return snapshot
}

func sameADKAdaptiveCheckpointInput(
	checkpoint *domainentity.Checkpoint,
	input ADKSideEffectCheckpointInput,
) error {
	if checkpoint == nil || checkpoint.RuntimeKey != input.RuntimeKey ||
		checkpoint.ParentCheckpointID != input.ParentCheckpointID {
		return fmt.Errorf("adaptive plan checkpoint retry identity drift")
	}
	envelope, err := UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues))
	if err != nil {
		return err
	}
	if envelope.RuntimeState == nil ||
		!equalBytes(envelope.RuntimeState.Checkpoint, input.RuntimeState) {
		return fmt.Errorf("adaptive plan checkpoint retry bytes drift")
	}
	if envelope.RuntimeVersion != input.RuntimeVersion {
		return fmt.Errorf("adaptive plan checkpoint retry runtime version drift")
	}
	if envelope.RunRevision != input.RunRevision {
		return fmt.Errorf("adaptive plan checkpoint retry run revision drift")
	}
	frozenParity, err := json.Marshal(envelope.ParityState)
	if err != nil {
		return fmt.Errorf("marshal frozen adaptive plan parity state: %w", err)
	}
	inputParity, err := json.Marshal(input.ParityState)
	if err != nil {
		return fmt.Errorf("marshal retry adaptive plan parity state: %w", err)
	}
	if !equalBytes(frozenParity, inputParity) {
		return fmt.Errorf("adaptive plan checkpoint retry parity state drift")
	}
	return nil
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
