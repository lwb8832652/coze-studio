// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"errors"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestSchedulerDispatchesSpacesRoundRobinWhilePreservingSpaceFIFO(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	first := schedulerCommand(clock.now(), "space-a-first", 101, 201, domainsandbox.ScopePlugin)
	second := schedulerCommand(clock.now(), "space-a-second", 101, 202, domainsandbox.ScopePlugin)
	third := schedulerCommand(clock.now(), "space-b-first", 102, 203, domainsandbox.ScopePlugin)
	for _, command := range []ExecuteCommand{first, second, third} {
		if _, err := scheduler.Accept(context.Background(), command); err != nil {
			t.Fatalf("Accept(%q): %v", command.IdempotencyKey, err)
		}
	}
	for range 3 {
		dispatched, err := scheduler.DispatchNext(context.Background())
		if err != nil || !dispatched {
			t.Fatalf("DispatchNext() = %t, %v", dispatched, err)
		}
		if err := scheduler.Finish(context.Background(), dispatcher.lastExecutionID(), infrasandbox.ExecutionStatusSucceeded); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := dispatcher.operationIDs(), []string{"space-a-first", "space-b-first", "space-a-second"}; !sameStrings(got, want) {
		t.Fatalf("dispatch order = %v, want %v", got, want)
	}
}

func TestSchedulerRespectsHeavyLightAndSameUserHeavyLimits(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	settings := schedulerSettingsForTest()
	settings.TotalWeight = 4
	settings.Workloads[domainsandbox.ScopeAgent] = domainsandbox.SchedulerWorkload{Weight: 2, CPULimit: 1250, MemoryLimitMB: 1536, PIDLimit: 128, QueueTimeoutSeconds: 600, IdleTTLSeconds: 300}
	settings.Workloads[domainsandbox.ScopeAppDev] = domainsandbox.SchedulerWorkload{Weight: 2, CPULimit: 1250, MemoryLimitMB: 1536, PIDLimit: 192, QueueTimeoutSeconds: 1200, IdleTTLSeconds: 600}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: settings, Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	firstHeavy := schedulerCommand(clock.now(), "heavy-user-one", 101, 201, domainsandbox.ScopeAgent)
	secondHeavySameUser := schedulerCommand(clock.now(), "heavy-user-one-second", 102, 201, domainsandbox.ScopeAppDev)
	thirdHeavyOtherUser := schedulerCommand(clock.now(), "heavy-user-two", 103, 202, domainsandbox.ScopeAgent)
	light := schedulerCommand(clock.now(), "light", 104, 203, domainsandbox.ScopePlugin)
	for _, command := range []ExecuteCommand{firstHeavy, secondHeavySameUser, thirdHeavyOtherUser, light} {
		if _, err := scheduler.Accept(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("first DispatchNext() = %t, %v", dispatched, err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("second DispatchNext() = %t, %v; another user's heavy work should use the remaining heavy slot", dispatched, err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || dispatched {
		t.Fatalf("third DispatchNext() = %t, %v; heavy work must not mix with light work", dispatched, err)
	}
	if err := scheduler.Finish(context.Background(), dispatcher.lastExecutionID(), infrasandbox.ExecutionStatusSucceeded); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || dispatched {
		t.Fatalf("post-one-finish DispatchNext() = %t, %v; same-user heavy remains blocked", dispatched, err)
	}
	if err := scheduler.Finish(context.Background(), firstHeavy.IdempotencyKeyToExecutionID(t, store), infrasandbox.ExecutionStatusSucceeded); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("post-heavy-finish DispatchNext() = %t, %v", dispatched, err)
	}
	if err := scheduler.Finish(context.Background(), dispatcher.lastExecutionID(), infrasandbox.ExecutionStatusSucceeded); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("post-light-finish DispatchNext() = %t, %v", dispatched, err)
	}
	if got, want := dispatcher.operationIDs(), []string{"heavy-user-one", "heavy-user-two", "light", "heavy-user-one-second"}; !sameStrings(got, want) {
		t.Fatalf("dispatch order = %v, want %v", got, want)
	}
}

func TestSchedulerPausesAtMemoryWatermarkWithoutDroppingQueue(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	sampler := &mutableMemorySampler{available: 1535}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: sampler, Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := scheduler.Accept(context.Background(), schedulerCommand(clock.now(), "watermark", 101, 201, domainsandbox.ScopePlugin))
	if err != nil {
		t.Fatal(err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || dispatched {
		t.Fatalf("DispatchNext() = %t, %v; expected safe pause", dispatched, err)
	}
	if status, err := store.Get(context.Background(), accepted.ExecutionID); err != nil || status.State != ExecutionStateAccepted {
		t.Fatalf("stored state/error = %#v/%v", status, err)
	}
	sampler.available = 1536
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("DispatchNext() after recovery = %t, %v", dispatched, err)
	}
}

func TestSchedulerExpiresQueuedWorkAndCancelsWithoutDispatch(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	expired := schedulerCommand(clock.now(), "expired", 101, 201, domainsandbox.ScopePlugin)
	expired.Deadline = clock.now().Add(time.Second)
	accepted, err := scheduler.Accept(context.Background(), expired)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(2 * time.Second)
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || dispatched {
		t.Fatalf("expired DispatchNext() = %t, %v", dispatched, err)
	}
	if stored, err := store.Get(context.Background(), accepted.ExecutionID); err != nil || stored.State != infrasandbox.ExecutionStatusTimedOut {
		t.Fatalf("expired state/error = %#v/%v", stored, err)
	}
	queued, err := scheduler.Accept(context.Background(), schedulerCommand(clock.now(), "canceled", 102, 202, domainsandbox.ScopePlugin))
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Cancel(context.Background(), queued.ExecutionID); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || dispatched {
		t.Fatalf("canceled DispatchNext() = %t, %v", dispatched, err)
	}
	if got := dispatcher.operationIDs(); len(got) != 0 {
		t.Fatalf("dispatcher received canceled or expired work: %v", got)
	}
}

func TestSchedulerUsesConfiguredQueueTimeoutBeforeExecutionDeadline(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	settings := schedulerSettingsForTest()
	plugin := settings.Workloads[domainsandbox.ScopePlugin]
	plugin.QueueTimeoutSeconds = 1
	settings.Workloads[domainsandbox.ScopePlugin] = plugin
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: settings, Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	command := schedulerCommand(clock.now(), "queue-timeout", 101, 201, domainsandbox.ScopePlugin)
	command.Deadline = clock.now().Add(time.Hour)
	accepted, err := scheduler.Accept(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(2 * time.Second)
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || dispatched {
		t.Fatalf("DispatchNext() = %t, %v", dispatched, err)
	}
	if stored, err := store.Get(context.Background(), accepted.ExecutionID); err != nil || stored.State != infrasandbox.ExecutionStatusTimedOut {
		t.Fatalf("state/error = %#v/%v", stored, err)
	}
}

func TestSchedulerSerializesAppDevBySpaceAndProject(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	settings := schedulerSettingsForTest()
	settings.TotalWeight = 4
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: settings, Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	first := schedulerCommand(clock.now(), "project-first", 101, 201, domainsandbox.ScopeAppDev)
	first.Identity.ProjectID = "same-project"
	sameProject := schedulerCommand(clock.now(), "project-same-space", 101, 202, domainsandbox.ScopeAppDev)
	sameProject.Identity.ProjectID = "same-project"
	otherSpace := schedulerCommand(clock.now(), "project-other-space", 102, 203, domainsandbox.ScopeAppDev)
	otherSpace.Identity.ProjectID = "same-project"
	for _, command := range []ExecuteCommand{first, sameProject, otherSpace} {
		if _, err := scheduler.Accept(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
			t.Fatalf("DispatchNext() = %t, %v", dispatched, err)
		}
	}
	if got, want := dispatcher.operationIDs(), []string{"project-first", "project-other-space"}; !sameStrings(got, want) {
		t.Fatalf("dispatch order = %v, want %v", got, want)
	}
}

func TestSchedulerFreezesConfigVersionAndProjectsQueueWithinItsOwnSpace(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	settings := schedulerSettingsForTest()
	settings.Version = 37
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: settings, Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	first := schedulerCommand(clock.now(), "projection-first", 101, 201, domainsandbox.ScopeAgent)
	secondSameSpace := schedulerCommand(clock.now(), "projection-second", 101, 202, domainsandbox.ScopeAgent)
	otherSpace := schedulerCommand(clock.now(), "projection-other", 102, 203, domainsandbox.ScopeAgent)
	acceptedFirst, err := scheduler.Accept(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	acceptedSecond, err := scheduler.Accept(context.Background(), secondSameSpace)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scheduler.Accept(context.Background(), otherSpace); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("DispatchNext() = %t, %v", dispatched, err)
	}
	if got := dispatcher.records[0].ConfigurationVersion; got != 37 {
		t.Fatalf("configuration version = %d, want 37", got)
	}
	clock.advance(8 * time.Second)
	if err := scheduler.Finish(context.Background(), acceptedFirst.ExecutionID, infrasandbox.ExecutionStatusSucceeded); err != nil {
		t.Fatal(err)
	}
	projection, ok := scheduler.QueueProjection(acceptedSecond.ExecutionID)
	expectedQueueDeadline := secondSameSpace.Deadline
	if deadline := clock.now().Add(592 * time.Second); deadline.Before(expectedQueueDeadline) {
		expectedQueueDeadline = deadline
	}
	if !ok || !projection.Waiting || !projection.Cancelable || projection.ApproximatePosition != 1 || projection.EstimatedWaitSeconds != 8 || projection.DeadlineUnixMilli != expectedQueueDeadline.UnixMilli() || projection.ReasonCode != "" {
		t.Fatalf("queue projection = %#v, found=%t", projection, ok)
	}
}

func TestSchedulerConcurrentDispatchNeverExceedsConfiguredWeight(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 12; index++ {
		command := schedulerCommand(clock.now(), "concurrent-"+strconv.Itoa(index), int64(100+index), int64(200+index), domainsandbox.ScopePlugin)
		if _, err := scheduler.Accept(context.Background(), command); err != nil {
			t.Fatal(err)
		}
	}
	var group sync.WaitGroup
	for range 24 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := scheduler.DispatchNext(context.Background()); err != nil {
				t.Errorf("DispatchNext(): %v", err)
			}
		}()
	}
	group.Wait()
	snapshot := scheduler.Snapshot()
	if snapshot.UsedWeight > snapshot.TotalWeight || snapshot.Running != 2 || len(dispatcher.operationIDs()) != 2 {
		t.Fatalf("snapshot=%#v dispatched=%v", snapshot, dispatcher.operationIDs())
	}
}

func TestSchedulerEstimatesWaitFromRunningDurationNotQueueDelay(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	first := schedulerCommand(clock.now(), "duration-first", 101, 201, domainsandbox.ScopeAgent)
	second := schedulerCommand(clock.now(), "duration-second", 102, 202, domainsandbox.ScopeAgent)
	third := schedulerCommand(clock.now(), "duration-third", 103, 203, domainsandbox.ScopeAgent)
	acceptedFirst, err := scheduler.Accept(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	acceptedSecond, err := scheduler.Accept(context.Background(), second)
	if err != nil {
		t.Fatal(err)
	}
	acceptedThird, err := scheduler.Accept(context.Background(), third)
	if err != nil {
		t.Fatal(err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("first DispatchNext() = %t, %v", dispatched, err)
	}
	clock.advance(3 * time.Second)
	if err := scheduler.Finish(context.Background(), acceptedFirst.ExecutionID, infrasandbox.ExecutionStatusSucceeded); err != nil {
		t.Fatal(err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("second DispatchNext() = %t, %v", dispatched, err)
	}
	clock.advance(5 * time.Second)
	if err := scheduler.Finish(context.Background(), acceptedSecond.ExecutionID, infrasandbox.ExecutionStatusSucceeded); err != nil {
		t.Fatal(err)
	}
	projection, ok := scheduler.QueueProjection(acceptedThird.ExecutionID)
	if !ok || projection.EstimatedWaitSeconds != 4 {
		t.Fatalf("queue projection = %#v, found=%t", projection, ok)
	}
}

func TestSchedulerDoesNotRequeueCanceledIdempotentReplay(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	command := schedulerCommand(clock.now(), "replay-after-cancel", 101, 201, domainsandbox.ScopePlugin)
	accepted, err := scheduler.Accept(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Cancel(context.Background(), accepted.ExecutionID); err != nil {
		t.Fatal(err)
	}
	if replay, err := scheduler.Accept(context.Background(), command); err != nil || replay.ExecutionID != accepted.ExecutionID || replay.Status != infrasandbox.ExecutionStatusCanceled {
		t.Fatalf("replay/error = %#v/%v", replay, err)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || dispatched {
		t.Fatalf("DispatchNext() = %t, %v", dispatched, err)
	}
}

func TestSchedulerRejectsTerminalCallbackForQueuedExecution(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: &recordingSchedulerDispatcher{}, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := scheduler.Accept(context.Background(), schedulerCommand(clock.now(), "queued-terminal", 101, 201, domainsandbox.ScopePlugin))
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Finish(context.Background(), accepted.ExecutionID, infrasandbox.ExecutionStatusSucceeded); !errors.Is(err, ErrExecutionConflict) {
		t.Fatalf("Finish() error = %v, want execution conflict", err)
	}
	if stored, err := store.Get(context.Background(), accepted.ExecutionID); err != nil || stored.State != ExecutionStateAccepted {
		t.Fatalf("stored state/error = %#v/%v", stored, err)
	}
}

func TestSchedulerKeepsQueuedWorkWhenCancellationStateWriteFails(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	store.transitionErr = ErrUnavailable
	dispatcher := &recordingSchedulerDispatcher{}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := scheduler.Accept(context.Background(), schedulerCommand(clock.now(), "cancel-state-write", 101, 201, domainsandbox.ScopePlugin))
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Cancel(context.Background(), accepted.ExecutionID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Cancel() error = %v", err)
	}
	store.transitionErr = nil
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || !dispatched {
		t.Fatalf("DispatchNext() = %t, %v; failed cancellation must retain accepted work", dispatched, err)
	}
}

func TestSchedulerRecoversAcceptedQueueAndReservesRunningCapacity(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	acceptedCommand := schedulerCommand(clock.now(), "recover-accepted", 101, 201, domainsandbox.ScopePlugin)
	runningCommand := schedulerCommand(clock.now(), "recover-running", 102, 202, domainsandbox.ScopeAgent)
	store.recovered = []RecoveredExecution{
		{Stored: StoredExecution{ExecutionID: "exec-recover-accepted", Scope: string(acceptedCommand.Scope), WorkloadKind: string(acceptedCommand.WorkloadKind), Deadline: acceptedCommand.Deadline, State: ExecutionStateAccepted, AcceptedAt: clock.now()}, Command: acceptedCommand},
		{Stored: StoredExecution{ExecutionID: "exec-recover-running", Scope: string(runningCommand.Scope), WorkloadKind: string(runningCommand.WorkloadKind), Deadline: runningCommand.Deadline, State: ExecutionStateRunning, AcceptedAt: clock.now()}, Command: runningCommand},
	}
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: &recordingSchedulerDispatcher{}, Settings: schedulerSettingsForTest(), Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if snapshot := scheduler.Snapshot(); snapshot.Queued != 1 || snapshot.Running != 1 || snapshot.UsedWeight != 2 {
		t.Fatalf("recovered snapshot = %#v", snapshot)
	}
	if dispatched, err := scheduler.DispatchNext(context.Background()); err != nil || dispatched {
		t.Fatalf("DispatchNext() = %t, %v; recovered heavy task must retain capacity", dispatched, err)
	}
}

func TestSchedulerMaintainsCapacityInvariantsAcrossRandomOperations(t *testing.T) {
	clock := newSchedulerClock(time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC))
	store := newSchedulerStore(clock.now)
	dispatcher := &recordingSchedulerDispatcher{}
	settings := schedulerSettingsForTest()
	settings.TotalWeight = 4
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{Store: store, Dispatcher: dispatcher, Settings: settings, Resources: fixedMemorySampler(4096), Now: clock.now})
	if err != nil {
		t.Fatal(err)
	}
	random := rand.New(rand.NewSource(17))
	executions := make([]string, 0, 1000)
	scopes := []domainsandbox.Scope{domainsandbox.ScopeAgent, domainsandbox.ScopeAppDev, domainsandbox.ScopeMCPStdio, domainsandbox.ScopePlugin}
	for index := 0; index < 1000; index++ {
		switch random.Intn(4) {
		case 0:
			scope := scopes[random.Intn(len(scopes))]
			command := schedulerCommand(clock.now(), "random-"+strconv.Itoa(index), int64(100+random.Intn(5)), int64(200+random.Intn(5)), scope)
			command.Identity.ProjectID = "project-" + strconv.Itoa(random.Intn(3))
			accepted, err := scheduler.Accept(context.Background(), command)
			if err != nil {
				t.Fatalf("Accept(%d): %v", index, err)
			}
			executions = append(executions, accepted.ExecutionID)
		case 1:
			if _, err := scheduler.DispatchNext(context.Background()); err != nil {
				t.Fatalf("DispatchNext(%d): %v", index, err)
			}
		case 2:
			if len(executions) > 0 {
				_ = scheduler.Cancel(context.Background(), executions[random.Intn(len(executions))])
			}
		case 3:
			if len(executions) > 0 {
				_ = scheduler.Finish(context.Background(), executions[random.Intn(len(executions))], infrasandbox.ExecutionStatusSucceeded)
			}
		}
		scheduler.mu.Lock()
		usedWeight, totalWeight := scheduler.usedWeight, scheduler.settings.TotalWeight
		heavy, light := scheduler.runningHeavy, scheduler.runningLight
		for userID := range scheduler.heavyUsers {
			count := 0
			for _, running := range scheduler.running {
				if running.userID == userID && isHeavyWorkload(running.execution.Command.Scope) {
					count++
				}
			}
			if count != 1 {
				scheduler.mu.Unlock()
				t.Fatalf("user %d heavy count = %d", userID, count)
			}
		}
		scheduler.mu.Unlock()
		if usedWeight < 0 || usedWeight > totalWeight || (heavy > 0 && light > 0) {
			t.Fatalf("invariant violation used=%d total=%d heavy=%d light=%d", usedWeight, totalWeight, heavy, light)
		}
	}
}

type schedulerClock struct {
	mu    sync.Mutex
	value time.Time
}

func newSchedulerClock(value time.Time) *schedulerClock { return &schedulerClock{value: value} }
func (clock *schedulerClock) now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.value
}
func (clock *schedulerClock) advance(duration time.Duration) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.value = clock.value.Add(duration)
}

type schedulerStore struct {
	mu            sync.Mutex
	now           func() time.Time
	next          int
	byKey         map[string]StoredExecution
	byID          map[string]StoredExecution
	recovered     []RecoveredExecution
	transitionErr error
}

func newSchedulerStore(now func() time.Time) *schedulerStore {
	return &schedulerStore{now: now, byKey: map[string]StoredExecution{}, byID: map[string]StoredExecution{}}
}
func (store *schedulerStore) Accept(_ context.Context, command ExecuteCommand) (StoredExecution, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if record, ok := store.byKey[command.IdempotencyKey]; ok {
		return record, true, nil
	}
	store.next++
	record := StoredExecution{ExecutionID: "exec-scheduler-" + string(rune('0'+store.next)), RequestDigest: "digest", Scope: string(command.Scope), WorkloadKind: string(command.WorkloadKind), Deadline: command.Deadline, State: ExecutionStateAccepted, AcceptedAt: store.now(), UpdatedAt: store.now()}
	store.byKey[command.IdempotencyKey], store.byID[record.ExecutionID] = record, record
	return record, false, nil
}
func (store *schedulerStore) Get(_ context.Context, executionID string) (StoredExecution, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	record, ok := store.byID[executionID]
	if !ok {
		return StoredExecution{}, errors.New("missing")
	}
	return record, nil
}
func (store *schedulerStore) Transition(_ context.Context, executionID string, state infrasandbox.ExecutionStatus) (StoredExecution, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.transitionErr != nil {
		return StoredExecution{}, store.transitionErr
	}
	record, ok := store.byID[executionID]
	if !ok {
		return StoredExecution{}, errors.New("missing")
	}
	record.State, record.UpdatedAt = state, store.now()
	store.byID[executionID] = record
	return record, nil
}
func (store *schedulerStore) Recover(context.Context) ([]StoredExecution, error) { return nil, nil }
func (store *schedulerStore) RecoverExecutions(context.Context) ([]RecoveredExecution, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]RecoveredExecution(nil), store.recovered...), nil
}

type recordingSchedulerDispatcher struct {
	mu      sync.Mutex
	records []ScheduledExecution
}

func (dispatcher *recordingSchedulerDispatcher) Dispatch(_ context.Context, execution ScheduledExecution) error {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	dispatcher.records = append(dispatcher.records, execution)
	return nil
}
func (dispatcher *recordingSchedulerDispatcher) lastExecutionID() string {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	return dispatcher.records[len(dispatcher.records)-1].ExecutionID
}
func (dispatcher *recordingSchedulerDispatcher) operationIDs() []string {
	dispatcher.mu.Lock()
	defer dispatcher.mu.Unlock()
	values := make([]string, 0, len(dispatcher.records))
	for _, record := range dispatcher.records {
		values = append(values, record.Command.IdempotencyKey)
	}
	return values
}

type mutableMemorySampler struct{ available int }

func fixedMemorySampler(available int) *mutableMemorySampler {
	return &mutableMemorySampler{available: available}
}
func (sampler *mutableMemorySampler) AvailableMemoryMB(context.Context) (int, error) {
	return sampler.available, nil
}

func schedulerSettingsForTest() domainsandbox.SchedulerSettings {
	return domainsandbox.DefaultSchedulerSettings()
}

func schedulerCommand(now time.Time, operationID string, spaceID, userID int64, scope domainsandbox.Scope) ExecuteCommand {
	workload := infrasandbox.WorkloadKind(scope)
	return ExecuteCommand{Scope: scope, WorkloadKind: workload, IdempotencyKey: operationID, Deadline: now.Add(time.Hour), Identity: sandboxidentity.Request{SpaceID: spaceID, UserID: userID, ProjectID: "project-1", SessionID: "session-1", ExecutionID: "execution-1"}}
}

func (command ExecuteCommand) IdempotencyKeyToExecutionID(t *testing.T, store *schedulerStore) string {
	t.Helper()
	store.mu.Lock()
	defer store.mu.Unlock()
	execution, ok := store.byKey[command.IdempotencyKey]
	if !ok {
		t.Fatalf("missing execution for %q", command.IdempotencyKey)
	}
	return execution.ExecutionID
}

func sameStrings(left, right []string) bool {
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
