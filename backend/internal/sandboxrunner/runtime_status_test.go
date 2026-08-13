// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandboxrunner

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuntimeStatusProjectsDisabledCoreWithoutCallingSessionDependencies(t *testing.T) {
	core := &recordingCoreRuntimeStatusSource{snapshot: CoreRuntimeStatusSnapshot{State: coreRuntimeReady, Generation: 9}}
	source := newRuntimeStatusSourceForTest(t, false, core)

	status, err := source.RuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("RuntimeStatus() error = %v", err)
	}
	if status.CoreState != coreRuntimeDisabled || status.AIORuntimeGeneration != 0 {
		t.Fatalf("Core status = %q/%d, want disabled/0", status.CoreState, status.AIORuntimeGeneration)
	}
	if core.calls != 0 {
		t.Fatalf("disabled Core called Session source %d times", core.calls)
	}
}

func TestRuntimeStatusProjectsReadyCoreGenerationWithoutInternalDetails(t *testing.T) {
	core := &recordingCoreRuntimeStatusSource{snapshot: CoreRuntimeStatusSnapshot{State: coreRuntimeReady, Generation: 9}}
	source := newRuntimeStatusSourceForTest(t, true, core)

	status, err := source.RuntimeStatus(context.Background())
	if err != nil {
		t.Fatalf("RuntimeStatus() error = %v", err)
	}
	if status.CoreState != coreRuntimeReady || status.AIORuntimeGeneration != 9 {
		t.Fatalf("Core status = %q/%d, want ready/9", status.CoreState, status.AIORuntimeGeneration)
	}
	if core.calls != 1 {
		t.Fatalf("Core source calls = %d, want 1", core.calls)
	}
}

func TestRuntimeStatusProjectsUnknownCoreWithoutFailingOneShotStatus(t *testing.T) {
	for name, core := range map[string]CoreRuntimeStatusSource{
		"missing source": nil,
		"source error": &recordingCoreRuntimeStatusSource{
			err: errors.New("sentinel-id endpoint and upstream body must stay internal"),
		},
		"ready without generation": &recordingCoreRuntimeStatusSource{
			snapshot: CoreRuntimeStatusSnapshot{State: coreRuntimeReady},
		},
		"unknown with generation": &recordingCoreRuntimeStatusSource{
			snapshot: CoreRuntimeStatusSnapshot{State: coreRuntimeUnknown, Generation: 10},
		},
		"unsupported state": &recordingCoreRuntimeStatusSource{
			snapshot: CoreRuntimeStatusSnapshot{State: "recovering", Generation: 10},
		},
	} {
		t.Run(name, func(t *testing.T) {
			source := newRuntimeStatusSourceForTest(t, true, core)
			status, err := source.RuntimeStatus(context.Background())
			if err != nil {
				t.Fatalf("RuntimeStatus() error = %v; Core uncertainty must not hide one-shot status", err)
			}
			if status.CoreState != coreRuntimeUnknown || status.AIORuntimeGeneration != 0 {
				t.Fatalf("Core status = %q/%d, want unknown/0", status.CoreState, status.AIORuntimeGeneration)
			}
		})
	}
}

func TestCoreRuntimeStatusSnapshotFormattingDoesNotLeakInternalDetails(t *testing.T) {
	snapshot := CoreRuntimeStatusSnapshot{State: coreRuntimeReady, Generation: 7}
	if got := snapshot.String(); got != "sandboxrunner.CoreRuntimeStatusSnapshot{state:ready,generation:7}" {
		t.Fatalf("String() = %q", got)
	}
	if got := snapshot.GoString(); got != snapshot.String() {
		t.Fatalf("GoString() = %q, String() = %q", got, snapshot.String())
	}
}

func newRuntimeStatusSourceForTest(t *testing.T, sessionEnabled bool, core CoreRuntimeStatusSource) runtimeStatusSource {
	t.Helper()
	clock := newSchedulerClock(time.Date(2026, time.August, 14, 9, 0, 0, 0, time.UTC))
	scheduler, err := NewRunnerScheduler(RunnerSchedulerConfig{
		Store:      newSchedulerStore(clock.now),
		Dispatcher: &recordingSchedulerDispatcher{},
		Settings:   schedulerSettingsForTest(),
		Resources:  fixedMemorySampler(4096),
		Now:        clock.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, _ := newLifecycleFixture(t, clock.now())
	return runtimeStatusSource{
		scheduler:             scheduler,
		lifecycle:             lifecycle,
		configuration:         fixedRuntimeStatusConfiguration{version: 3},
		sessionBackendEnabled: sessionEnabled,
		core:                  core,
	}
}

type fixedRuntimeStatusConfiguration struct{ version uint64 }

func (source fixedRuntimeStatusConfiguration) Configuration(context.Context) (ConfigurationProjection, error) {
	return ConfigurationProjection{Schema: "coze.sandbox.runner_configuration.v1", Version: source.version}, nil
}

type recordingCoreRuntimeStatusSource struct {
	snapshot CoreRuntimeStatusSnapshot
	err      error
	calls    int
}

func (source *recordingCoreRuntimeStatusSource) CoreRuntimeStatus(context.Context) (CoreRuntimeStatusSnapshot, error) {
	source.calls++
	return source.snapshot, source.err
}
