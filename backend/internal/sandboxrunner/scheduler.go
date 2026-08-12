// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"strconv"
	"sync"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

// ScheduledExecution is the trusted, in-memory projection needed to dispatch
// an encrypted execution record. The raw provider body remains in the store.
type ScheduledExecution struct {
	ExecutionID          string
	Command              ExecuteCommand
	Weight               int
	ConfigurationVersion uint64
}

// ExecutionDispatcher is intentionally narrow: Task 10 will implement it via
// the rootless runtime driver. This task schedules without any host execution.
type ExecutionDispatcher interface {
	Dispatch(context.Context, ScheduledExecution) error
}

type RunnerSchedulerConfig struct {
	Store      ExecutionStore
	Dispatcher ExecutionDispatcher
	Settings   domainsandbox.SchedulerSettings
	Resources  ResourceSampler
	Now        func() time.Time
}

type RunnerScheduler struct {
	store      ExecutionStore
	dispatcher ExecutionDispatcher
	settings   domainsandbox.SchedulerSettings
	watermark  ResourceWatermark
	now        func() time.Time

	mu              sync.Mutex
	queues          map[int64][]scheduledItem
	spaceOrder      []int64
	nextSpace       int
	running         map[string]scheduledItem
	usedWeight      int
	runningHeavy    int
	runningLight    int
	heavyUsers      map[int64]struct{}
	appDevProjects  map[string]struct{}
	durationAverage map[domainsandbox.Scope]time.Duration
}

type scheduledItem struct {
	execution ScheduledExecution
	spaceID   int64
	userID    int64
	projectID string
	enqueued  time.Time
	queueEnds time.Time
	startedAt time.Time
}

func NewRunnerScheduler(config RunnerSchedulerConfig) (*RunnerScheduler, error) {
	settings, err := domainsandbox.NormalizeSchedulerSettings(config.Settings)
	if err != nil || config.Store == nil || config.Dispatcher == nil || config.Resources == nil || config.Now == nil {
		return nil, ErrConfiguration
	}
	return &RunnerScheduler{
		store: config.Store, dispatcher: config.Dispatcher, settings: settings,
		watermark: ResourceWatermark{Sampler: config.Resources, ReserveMB: settings.HostMemoryReserveMB}, now: config.Now,
		queues: map[int64][]scheduledItem{}, running: map[string]scheduledItem{}, heavyUsers: map[int64]struct{}{}, appDevProjects: map[string]struct{}{}, durationAverage: map[domainsandbox.Scope]time.Duration{},
	}, nil
}

func (scheduler *RunnerScheduler) Accept(ctx context.Context, command ExecuteCommand) (ExecutionProjection, error) {
	if scheduler == nil || ctx == nil || command.Identity.SpaceID <= 0 || command.Identity.UserID <= 0 || command.Deadline.IsZero() || !command.Deadline.After(scheduler.now()) {
		return ExecutionProjection{}, ErrProtocol
	}
	workload, ok := scheduler.settings.Workloads[command.Scope]
	if !ok || workload.Weight < 1 {
		return ExecutionProjection{}, ErrProtocol
	}
	stored, _, err := scheduler.store.Accept(ctx, command)
	if err != nil {
		return ExecutionProjection{}, err
	}
	stored, err = scheduler.store.Get(ctx, stored.ExecutionID)
	if err != nil {
		return ExecutionProjection{}, err
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if stored.State != ExecutionStateAccepted || scheduler.queuedLocked(stored.ExecutionID) {
		return ExecutionProjection{ExecutionID: stored.ExecutionID, Status: stored.State}, nil
	}
	enqueued := scheduler.now()
	queueEnds := enqueued.Add(time.Duration(workload.QueueTimeoutSeconds) * time.Second)
	if command.Deadline.Before(queueEnds) {
		queueEnds = command.Deadline
	}
	item := scheduledItem{execution: ScheduledExecution{ExecutionID: stored.ExecutionID, Command: command, Weight: workload.Weight, ConfigurationVersion: scheduler.settings.Version}, spaceID: command.Identity.SpaceID, userID: command.Identity.UserID, projectID: command.Identity.ProjectID, enqueued: enqueued, queueEnds: queueEnds}
	if _, exists := scheduler.queues[item.spaceID]; !exists {
		scheduler.spaceOrder = append(scheduler.spaceOrder, item.spaceID)
	}
	scheduler.queues[item.spaceID] = append(scheduler.queues[item.spaceID], item)
	return ExecutionProjection{ExecutionID: stored.ExecutionID, Status: stored.State}, nil
}

// DispatchNext performs a single non-blocking fair dispatch attempt. A false
// result is normal when no queued item is currently safe to run.
func (scheduler *RunnerScheduler) DispatchNext(ctx context.Context) (bool, error) {
	if scheduler == nil || ctx == nil {
		return false, ErrProtocol
	}
	if !scheduler.watermark.AllowsDispatch(ctx) {
		return false, nil
	}
	scheduler.mu.Lock()
	item, expired := scheduler.nextLocked(scheduler.now())
	scheduler.mu.Unlock()
	for _, candidate := range expired {
		if _, err := scheduler.store.Transition(ctx, candidate.execution.ExecutionID, infrasandbox.ExecutionStatusTimedOut); err != nil {
			return false, err
		}
	}
	if item == nil {
		return false, nil
	}
	if _, err := scheduler.store.Transition(ctx, item.execution.ExecutionID, ExecutionStateRunning); err != nil {
		scheduler.mu.Lock()
		scheduler.releaseLocked(*item)
		scheduler.mu.Unlock()
		return false, err
	}
	if err := scheduler.dispatcher.Dispatch(ctx, item.execution); err != nil {
		_, transitionErr := scheduler.store.Transition(ctx, item.execution.ExecutionID, infrasandbox.ExecutionStatusFailed)
		scheduler.mu.Lock()
		scheduler.releaseLocked(*item)
		scheduler.mu.Unlock()
		if transitionErr != nil {
			return false, transitionErr
		}
		return false, ErrUnavailable
	}
	return true, nil
}

func (scheduler *RunnerScheduler) Finish(ctx context.Context, executionID string, state infrasandbox.ExecutionStatus) error {
	if scheduler == nil || ctx == nil || !isTerminalExecutionState(state) || !validExecutionID(executionID) {
		return ErrProtocol
	}
	scheduler.mu.Lock()
	_, running := scheduler.running[executionID]
	scheduler.mu.Unlock()
	if !running {
		return ErrExecutionConflict
	}
	if _, err := scheduler.store.Transition(ctx, executionID, state); err != nil {
		return err
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if current, ok := scheduler.running[executionID]; ok {
		if state == infrasandbox.ExecutionStatusSucceeded || state == infrasandbox.ExecutionStatusFailed || state == infrasandbox.ExecutionStatusTimedOut {
			scheduler.recordDurationLocked(current, scheduler.now().Sub(current.startedAt))
		}
		scheduler.releaseLocked(current)
	}
	return nil
}

func (scheduler *RunnerScheduler) Cancel(ctx context.Context, executionID string) error {
	if scheduler == nil || ctx == nil || !validExecutionID(executionID) {
		return ErrProtocol
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if !scheduler.queuedLocked(executionID) {
		return ErrExecutionConflict
	}
	if _, err := scheduler.store.Transition(ctx, executionID, infrasandbox.ExecutionStatusCanceled); err != nil {
		return err
	}
	if !scheduler.removeQueuedLocked(executionID) {
		return ErrExecutionConflict
	}
	return nil
}

// Recover rebuilds only accepted queue items and capacity reservations for
// already-running work. It never re-dispatches a running execution.
func (scheduler *RunnerScheduler) Recover(ctx context.Context) error {
	if scheduler == nil || ctx == nil {
		return ErrProtocol
	}
	store, ok := scheduler.store.(RecoverableExecutionStore)
	if !ok {
		return ErrUnavailable
	}
	recovered, err := store.RecoverExecutions(ctx)
	if err != nil {
		return err
	}
	for _, recoveredExecution := range recovered {
		if err := scheduler.recoverOne(ctx, recoveredExecution); err != nil {
			return err
		}
	}
	return nil
}

func (scheduler *RunnerScheduler) recoverOne(ctx context.Context, recovered RecoveredExecution) error {
	command, stored := recovered.Command, recovered.Stored
	if stored.ExecutionID == "" || command.Identity.SpaceID <= 0 || command.Identity.UserID <= 0 || command.Scope != domainsandbox.Scope(stored.Scope) || command.WorkloadKind != infrasandbox.WorkloadKind(stored.WorkloadKind) {
		return ErrUnavailable
	}
	workload, ok := scheduler.settings.Workloads[command.Scope]
	if !ok || stored.Deadline.IsZero() {
		return ErrUnavailable
	}
	enqueued := stored.AcceptedAt
	queueEnds := enqueued.Add(time.Duration(workload.QueueTimeoutSeconds) * time.Second)
	if stored.Deadline.Before(queueEnds) {
		queueEnds = stored.Deadline
	}
	if stored.State == ExecutionStateAccepted && !queueEnds.After(scheduler.now()) {
		_, err := scheduler.store.Transition(ctx, stored.ExecutionID, infrasandbox.ExecutionStatusTimedOut)
		return err
	}
	item := scheduledItem{execution: ScheduledExecution{ExecutionID: stored.ExecutionID, Command: command, Weight: workload.Weight, ConfigurationVersion: scheduler.settings.Version}, spaceID: command.Identity.SpaceID, userID: command.Identity.UserID, projectID: command.Identity.ProjectID, enqueued: enqueued, queueEnds: queueEnds, startedAt: stored.UpdatedAt}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if scheduler.queuedLocked(stored.ExecutionID) {
		return nil
	}
	switch stored.State {
	case ExecutionStateAccepted:
		if _, exists := scheduler.queues[item.spaceID]; !exists {
			scheduler.spaceOrder = append(scheduler.spaceOrder, item.spaceID)
		}
		scheduler.queues[item.spaceID] = append(scheduler.queues[item.spaceID], item)
	case ExecutionStateRunning:
		if !scheduler.canRunLocked(item) {
			return ErrUnavailable
		}
		scheduler.reserveLocked(item)
	default:
		return ErrUnavailable
	}
	return nil
}

func (scheduler *RunnerScheduler) nextLocked(now time.Time) (*scheduledItem, []scheduledItem) {
	if len(scheduler.spaceOrder) == 0 {
		return nil, nil
	}
	expired := make([]scheduledItem, 0)
	for checked := 0; checked < len(scheduler.spaceOrder); checked++ {
		index := (scheduler.nextSpace + checked) % len(scheduler.spaceOrder)
		spaceID := scheduler.spaceOrder[index]
		queue := scheduler.queues[spaceID]
		for len(queue) > 0 && !queue[0].queueEnds.After(now) {
			expired = append(expired, queue[0])
			queue = queue[1:]
		}
		if len(queue) == 0 {
			scheduler.queues[spaceID] = nil
			continue
		}
		candidate := queue[0]
		if !scheduler.canRunLocked(candidate) {
			scheduler.queues[spaceID] = queue
			continue
		}
		scheduler.queues[spaceID] = queue[1:]
		scheduler.nextSpace = (index + 1) % len(scheduler.spaceOrder)
		candidate.startedAt = now
		scheduler.reserveLocked(candidate)
		return &candidate, expired
	}
	return nil, expired
}

func (scheduler *RunnerScheduler) canRunLocked(item scheduledItem) bool {
	if scheduler.usedWeight+item.execution.Weight > scheduler.settings.TotalWeight {
		return false
	}
	heavy := isHeavyWorkload(item.execution.Command.Scope)
	if heavy && scheduler.runningLight > 0 {
		return false
	}
	if !heavy && scheduler.runningHeavy > 0 {
		return false
	}
	if heavy {
		if _, exists := scheduler.heavyUsers[item.userID]; exists {
			return false
		}
	}
	if projectKey := item.appDevProjectKey(); projectKey != "" {
		if _, exists := scheduler.appDevProjects[projectKey]; exists {
			return false
		}
	}
	return true
}

func (scheduler *RunnerScheduler) reserveLocked(item scheduledItem) {
	scheduler.running[item.execution.ExecutionID] = item
	scheduler.usedWeight += item.execution.Weight
	if isHeavyWorkload(item.execution.Command.Scope) {
		scheduler.runningHeavy++
		scheduler.heavyUsers[item.userID] = struct{}{}
	} else {
		scheduler.runningLight++
	}
	if projectKey := item.appDevProjectKey(); projectKey != "" {
		scheduler.appDevProjects[projectKey] = struct{}{}
	}
}

func (scheduler *RunnerScheduler) releaseLocked(item scheduledItem) {
	if _, exists := scheduler.running[item.execution.ExecutionID]; !exists {
		return
	}
	delete(scheduler.running, item.execution.ExecutionID)
	scheduler.usedWeight -= item.execution.Weight
	if isHeavyWorkload(item.execution.Command.Scope) {
		scheduler.runningHeavy--
		delete(scheduler.heavyUsers, item.userID)
	} else {
		scheduler.runningLight--
	}
	if projectKey := item.appDevProjectKey(); projectKey != "" {
		delete(scheduler.appDevProjects, projectKey)
	}
}

func (item scheduledItem) appDevProjectKey() string {
	if item.execution.Command.Scope != domainsandbox.ScopeAppDev || item.projectID == "" {
		return ""
	}
	return strconv.FormatInt(item.spaceID, 10) + ":" + item.projectID
}

func isHeavyWorkload(scope domainsandbox.Scope) bool {
	return scope == domainsandbox.ScopeAgent || scope == domainsandbox.ScopeAppDev
}

func (scheduler *RunnerScheduler) queuedLocked(executionID string) bool {
	for _, queue := range scheduler.queues {
		for _, item := range queue {
			if item.execution.ExecutionID == executionID {
				return true
			}
		}
	}
	return false
}

func (scheduler *RunnerScheduler) removeQueuedLocked(executionID string) bool {
	for spaceID, queue := range scheduler.queues {
		for index, item := range queue {
			if item.execution.ExecutionID != executionID {
				continue
			}
			scheduler.queues[spaceID] = append(queue[:index:index], queue[index+1:]...)
			return true
		}
	}
	return false
}

// Snapshot returns safe scheduler aggregate state for future health/metrics
// wiring. It deliberately omits tenant and execution identifiers.
func (scheduler *RunnerScheduler) Snapshot() SchedulerSnapshot {
	if scheduler == nil {
		return SchedulerSnapshot{}
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	queued := 0
	for _, queue := range scheduler.queues {
		queued += len(queue)
	}
	return SchedulerSnapshot{Queued: queued, Running: len(scheduler.running), UsedWeight: scheduler.usedWeight, TotalWeight: scheduler.settings.TotalWeight}
}

type SchedulerSnapshot struct {
	Queued      int
	Running     int
	UsedWeight  int
	TotalWeight int
}

// QueueProjection intentionally contains only a same-space approximate
// position and aggregate timing. It never reveals another tenant's queue.
type QueueProjection struct {
	Waiting              bool
	ApproximatePosition  int
	EstimatedWaitSeconds int
	DeadlineUnixMilli    int64
	Cancelable           bool
	ReasonCode           string
}

func (scheduler *RunnerScheduler) QueueProjection(executionID string) (QueueProjection, bool) {
	if scheduler == nil || !validExecutionID(executionID) {
		return QueueProjection{}, false
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	for _, queue := range scheduler.queues {
		for index, item := range queue {
			if item.execution.ExecutionID != executionID {
				continue
			}
			estimate := scheduler.durationAverage[item.execution.Command.Scope]
			return QueueProjection{Waiting: true, ApproximatePosition: index + 1, EstimatedWaitSeconds: int(estimate.Round(time.Second) / time.Second), DeadlineUnixMilli: item.queueEnds.UnixMilli(), Cancelable: true}, true
		}
	}
	return QueueProjection{}, false
}

func (scheduler *RunnerScheduler) recordDurationLocked(item scheduledItem, duration time.Duration) {
	if duration <= 0 {
		return
	}
	previous := scheduler.durationAverage[item.execution.Command.Scope]
	if previous == 0 {
		scheduler.durationAverage[item.execution.Command.Scope] = duration
		return
	}
	scheduler.durationAverage[item.execution.Command.Scope] = (previous + duration) / 2
}

var _ Scheduler = (*RunnerScheduler)(nil)
