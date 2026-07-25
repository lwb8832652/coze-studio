// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type infrastructureFailureHealthMonitorRepository struct {
	err error
}

func (r infrastructureFailureHealthMonitorRepository) ReconcileDisabledHealthEpisodes(
	context.Context,
	time.Time,
	int,
) (int64, error) {
	return 0, nil
}

func (r infrastructureFailureHealthMonitorRepository) ClaimHealthChecks(
	context.Context,
	domainsandbox.HealthMonitorClaimRequest,
) ([]domainsandbox.HealthMonitorClaim, error) {
	return nil, r.err
}

func (r infrastructureFailureHealthMonitorRepository) CompleteHealthCheck(
	context.Context,
	domainsandbox.CompleteHealthMonitorCheckInput,
) (domainsandbox.HealthMonitorCheckCompletion, error) {
	return domainsandbox.HealthMonitorCheckCompletion{}, nil
}

func (r infrastructureFailureHealthMonitorRepository) ProjectPendingHealthNotifications(
	context.Context,
	domainsandbox.HealthNotificationProjectionRequest,
) (domainsandbox.HealthNotificationProjectionResult, error) {
	return domainsandbox.HealthNotificationProjectionResult{}, nil
}

type recordingHealthMonitorRepository struct {
	claimRequests      []domainsandbox.HealthMonitorClaimRequest
	projectionRequests []domainsandbox.HealthNotificationProjectionRequest
	projectionResults  []domainsandbox.HealthNotificationProjectionResult
	projectionErrors   []error
}

func (r *recordingHealthMonitorRepository) ReconcileDisabledHealthEpisodes(
	context.Context,
	time.Time,
	int,
) (int64, error) {
	return 0, nil
}

func (r *recordingHealthMonitorRepository) ClaimHealthChecks(
	_ context.Context,
	request domainsandbox.HealthMonitorClaimRequest,
) ([]domainsandbox.HealthMonitorClaim, error) {
	r.claimRequests = append(r.claimRequests, request)
	return nil, nil
}

func (r *recordingHealthMonitorRepository) CompleteHealthCheck(
	context.Context,
	domainsandbox.CompleteHealthMonitorCheckInput,
) (domainsandbox.HealthMonitorCheckCompletion, error) {
	return domainsandbox.HealthMonitorCheckCompletion{}, nil
}

func (r *recordingHealthMonitorRepository) ProjectPendingHealthNotifications(
	_ context.Context,
	request domainsandbox.HealthNotificationProjectionRequest,
) (domainsandbox.HealthNotificationProjectionResult, error) {
	r.projectionRequests = append(r.projectionRequests, request)
	index := len(r.projectionRequests) - 1
	var result domainsandbox.HealthNotificationProjectionResult
	if index < len(r.projectionResults) {
		result = r.projectionResults[index]
	}
	var err error
	if index < len(r.projectionErrors) {
		err = r.projectionErrors[index]
	}
	return result, err
}

type blockingHealthMonitorRepository struct {
	started chan struct{}
	once    sync.Once
}

func (r *blockingHealthMonitorRepository) ReconcileDisabledHealthEpisodes(
	context.Context,
	time.Time,
	int,
) (int64, error) {
	return 0, nil
}

func (r *blockingHealthMonitorRepository) ClaimHealthChecks(
	ctx context.Context,
	_ domainsandbox.HealthMonitorClaimRequest,
) ([]domainsandbox.HealthMonitorClaim, error) {
	r.once.Do(func() { close(r.started) })
	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *blockingHealthMonitorRepository) CompleteHealthCheck(
	context.Context,
	domainsandbox.CompleteHealthMonitorCheckInput,
) (domainsandbox.HealthMonitorCheckCompletion, error) {
	return domainsandbox.HealthMonitorCheckCompletion{}, nil
}

func (r *blockingHealthMonitorRepository) ProjectPendingHealthNotifications(
	ctx context.Context,
	_ domainsandbox.HealthNotificationProjectionRequest,
) (domainsandbox.HealthNotificationProjectionResult, error) {
	return domainsandbox.HealthNotificationProjectionResult{}, ctx.Err()
}

func TestProviderHealthEpisodeRequiresStableFailureAndSupportsRecoveryAndNewIncident(
	t *testing.T,
) {
	base := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	state := domainsandbox.ProviderHealthEpisode{ProviderID: 71}
	observe := func(
		offset time.Duration,
		status domainsandbox.HealthStatus,
	) domainsandbox.HealthIncidentTransition {
		transition := domainsandbox.ApplyProviderHealthObservation(
			state,
			domainsandbox.HealthIncidentObservation{
				ProviderID: 71,
				ProviderStatus: domainsandbox.ProviderStatusEnabled,
				HealthStatus: status,
				CheckedAt: base.Add(offset),
				FailureThreshold: 3,
			},
		)
		state = transition.State
		return transition
	}

	if transition := observe(0, domainsandbox.HealthStatusUnhealthy);
		transition.Notification != domainsandbox.HealthIncidentNotificationNone ||
			state.ConsecutiveFailures != 1 {
		t.Fatalf("first transient failure = %#v", transition)
	}
	if transition := observe(30*time.Second, domainsandbox.HealthStatusHealthy);
		transition.Notification != domainsandbox.HealthIncidentNotificationNone ||
			state.ConsecutiveFailures != 0 ||
			state.IncidentStatus != domainsandbox.HealthIncidentStatusNone {
		t.Fatalf("transient recovery = %#v", transition)
	}
	observe(time.Minute, domainsandbox.HealthStatusUnhealthy)
	observe(90*time.Second, domainsandbox.HealthStatusUnhealthy)
	opened := observe(2*time.Minute, domainsandbox.HealthStatusUnhealthy)
	if opened.Notification != domainsandbox.HealthIncidentNotificationUnhealthy ||
		opened.IncidentID != "sandbox-incident-71-1" ||
		state.IncidentSequence != 1 ||
		state.IncidentStatus != domainsandbox.HealthIncidentStatusOpen {
		t.Fatalf("stable incident = %#v", opened)
	}

	restartedState := state
	replay := domainsandbox.ApplyProviderHealthObservation(
		restartedState,
		domainsandbox.HealthIncidentObservation{
			ProviderID: 71,
			ProviderStatus: domainsandbox.ProviderStatusEnabled,
			HealthStatus: domainsandbox.HealthStatusUnhealthy,
			CheckedAt: base.Add(2 * time.Minute),
			FailureThreshold: 3,
		},
	)
	if replay.Applied || replay.Notification != domainsandbox.HealthIncidentNotificationNone {
		t.Fatalf("replayed check changed incident = %#v", replay)
	}
	if repeated := observe(150*time.Second, domainsandbox.HealthStatusUnhealthy);
		repeated.Notification != domainsandbox.HealthIncidentNotificationNone {
		t.Fatalf("repeated unhealthy notification = %#v", repeated)
	}
	recovered := observe(3*time.Minute, domainsandbox.HealthStatusHealthy)
	if recovered.Notification != domainsandbox.HealthIncidentNotificationRecovered ||
		recovered.IncidentID != opened.IncidentID ||
		state.IncidentStatus != domainsandbox.HealthIncidentStatusRecovered ||
		state.LastRecoveredAt.IsZero() {
		t.Fatalf("incident recovery = %#v", recovered)
	}
	observe(210*time.Second, domainsandbox.HealthStatusUnhealthy)
	observe(4*time.Minute, domainsandbox.HealthStatusUnhealthy)
	second := observe(270*time.Second, domainsandbox.HealthStatusUnhealthy)
	if second.Notification != domainsandbox.HealthIncidentNotificationUnhealthy ||
		second.IncidentID != "sandbox-incident-71-2" ||
		state.IncidentSequence != 2 {
		t.Fatalf("new incident after recovery = %#v", second)
	}
}

func TestProviderHealthEpisodeResetsUnstableSequenceAndClosesOnDisable(t *testing.T) {
	base := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	state := domainsandbox.ProviderHealthEpisode{ProviderID: 72}
	apply := func(
		offset time.Duration,
		providerStatus domainsandbox.ProviderStatus,
		healthStatus domainsandbox.HealthStatus,
	) domainsandbox.HealthIncidentTransition {
		transition := domainsandbox.ApplyProviderHealthObservation(
			state,
			domainsandbox.HealthIncidentObservation{
				ProviderID: 72,
				ProviderStatus: providerStatus,
				HealthStatus: healthStatus,
				CheckedAt: base.Add(offset),
				FailureThreshold: 3,
			},
		)
		state = transition.State
		return transition
	}
	apply(0, domainsandbox.ProviderStatusEnabled, domainsandbox.HealthStatusUnhealthy)
	apply(time.Minute, domainsandbox.ProviderStatusEnabled, domainsandbox.HealthStatusDegraded)
	if state.ConsecutiveFailures != 0 ||
		state.IncidentStatus != domainsandbox.HealthIncidentStatusNone {
		t.Fatalf("degraded check did not break unhealthy sequence: %#v", state)
	}
	apply(2*time.Minute, domainsandbox.ProviderStatusEnabled, domainsandbox.HealthStatusUnhealthy)
	apply(3*time.Minute, domainsandbox.ProviderStatusEnabled, domainsandbox.HealthStatusUnhealthy)
	apply(4*time.Minute, domainsandbox.ProviderStatusEnabled, domainsandbox.HealthStatusUnhealthy)
	disabled := apply(
		5*time.Minute,
		domainsandbox.ProviderStatusDisabled,
		domainsandbox.HealthStatusUnknown,
	)
	if disabled.Notification != domainsandbox.HealthIncidentNotificationNone ||
		state.IncidentStatus != domainsandbox.HealthIncidentStatusDisabled ||
		state.ConsecutiveFailures != 0 {
		t.Fatalf("disabled provider generated a health incident: %#v", disabled)
	}
	reenabled := apply(
		6*time.Minute,
		domainsandbox.ProviderStatusEnabled,
		domainsandbox.HealthStatusHealthy,
	)
	if reenabled.Notification != domainsandbox.HealthIncidentNotificationNone ||
		state.IncidentStatus != domainsandbox.HealthIncidentStatusNone {
		t.Fatalf("re-enabled provider generated a recovery notification: %#v", reenabled)
	}
	apply(7*time.Minute, domainsandbox.ProviderStatusEnabled, domainsandbox.HealthStatusUnhealthy)
	apply(8*time.Minute, domainsandbox.ProviderStatusEnabled, domainsandbox.HealthStatusUnhealthy)
	second := apply(
		9*time.Minute,
		domainsandbox.ProviderStatusEnabled,
		domainsandbox.HealthStatusUnhealthy,
	)
	if second.Notification != domainsandbox.HealthIncidentNotificationUnhealthy ||
		second.IncidentID != "sandbox-incident-72-2" ||
		state.IncidentSequence != 2 {
		t.Fatalf("new incident after re-enable = %#v", second)
	}
}

func TestDisabledHealthMonitorDoesNotTouchPersistence(t *testing.T) {
	monitor, err := NewHealthMonitor(nil, nil, HealthMonitorOptions{Disabled: true})
	if err != nil {
		t.Fatalf("NewHealthMonitor(disabled) error = %v", err)
	}
	if err := monitor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce(disabled) error = %v", err)
	}
}

func TestHealthMonitorPropagatesInfrastructureFailure(t *testing.T) {
	infrastructureError := errors.New(
		"sandbox health monitor infrastructure unavailable",
	)
	monitor, err := NewHealthMonitor(
		&Service{},
		infrastructureFailureHealthMonitorRepository{err: infrastructureError},
		DefaultHealthMonitorOptions(),
	)
	if err != nil {
		t.Fatalf("NewHealthMonitor() error = %v", err)
	}

	err = monitor.RunOnce(context.Background())
	if !errors.Is(err, infrastructureError) {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("infrastructure failure was swallowed as a claim conflict: %v", err)
	}
}

func TestHealthMonitorDoesNotPreclaimBeyondImmediateConcurrency(t *testing.T) {
	repository := &recordingHealthMonitorRepository{}
	monitor, err := NewHealthMonitor(
		&Service{},
		repository,
		DefaultHealthMonitorOptions(),
	)
	if err != nil {
		t.Fatalf("NewHealthMonitor() error = %v", err)
	}
	if err := monitor.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repository.claimRequests) != 1 ||
		repository.claimRequests[0].Limit != defaultHealthMonitorConcurrency {
		t.Fatalf("health claim requests = %#v", repository.claimRequests)
	}
	if len(repository.projectionRequests) != 1 ||
		repository.projectionRequests[0].Limit != defaultHealthMonitorConcurrency {
		t.Fatalf("projection requests = %#v", repository.projectionRequests)
	}
}

func TestHealthMonitorRequiresLeaseCompletionBudget(t *testing.T) {
	options := DefaultHealthMonitorOptions()
	options.Lease = options.ProbeTimeout + healthMonitorDBCompletionBudget
	_, err := NewHealthMonitor(&Service{}, &recordingHealthMonitorRepository{}, options)
	if !errors.Is(err, domainsandbox.ErrConfigurationInvalid) {
		t.Fatalf("unsafe lease budget error = %v", err)
	}
}

func TestHealthMonitorShutdownCancelsAndWaitsForWorker(t *testing.T) {
	repository := &blockingHealthMonitorRepository{started: make(chan struct{})}
	options := DefaultHealthMonitorOptions()
	options.PollInterval = time.Hour
	monitor, err := NewHealthMonitor(&Service{}, repository, options)
	if err != nil {
		t.Fatalf("NewHealthMonitor() error = %v", err)
	}
	monitor.Start(context.Background())
	<-repository.started

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := monitor.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestHealthMonitorErrorCodesAreStableAndRedacted(t *testing.T) {
	for _, test := range []struct {
		err  error
		code string
	}{
		{err: domainsandbox.ErrVersionConflict, code: "claim"},
		{err: domainsandbox.ErrHealthMonitorDBClock, code: "db_clock"},
		{err: domainsandbox.ErrHealthMonitorOutbox, code: "outbox"},
		{err: domainsandbox.ErrConfigurationInvalid, code: "config"},
		{err: errors.New("raw endpoint credential provider body"), code: "storage"},
	} {
		if got := healthMonitorErrorCode(test.err); got != test.code {
			t.Fatalf("healthMonitorErrorCode(%v) = %q, want %q", test.err, got, test.code)
		}
	}
}

func TestHealthMonitorContinuesCausalProjectionRoundsAfterOneIncidentBacksOff(
	t *testing.T,
) {
	repository := &recordingHealthMonitorRepository{
		projectionResults: []domainsandbox.HealthNotificationProjectionResult{
			{Claimed: 2, Projected: 1},
			{Claimed: 1, Projected: 1},
			{},
		},
		projectionErrors: []error{
			domainsandbox.ErrHealthMonitorOutbox,
			nil,
			nil,
		},
	}
	monitor, err := NewHealthMonitor(
		&Service{},
		repository,
		DefaultHealthMonitorOptions(),
	)
	if err != nil {
		t.Fatalf("NewHealthMonitor() error = %v", err)
	}
	err = monitor.RunOnce(context.Background())
	if !errors.Is(err, domainsandbox.ErrHealthMonitorOutbox) {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if len(repository.projectionRequests) != 3 {
		t.Fatalf(
			"projection requests = %#v",
			repository.projectionRequests,
		)
	}
	for _, request := range repository.projectionRequests {
		if request.Limit > defaultHealthMonitorConcurrency {
			t.Fatalf("projection request overclaimed: %#v", request)
		}
	}
}
