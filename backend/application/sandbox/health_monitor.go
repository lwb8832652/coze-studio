// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	defaultHealthMonitorBatchSize     = 20
	defaultHealthMonitorConcurrency   = 4
	defaultHealthMonitorPollInterval  = 15 * time.Second
	defaultHealthMonitorCheckInterval = 30 * time.Second
	defaultHealthMonitorLease         = 45 * time.Second
	defaultHealthMonitorProbeTimeout  = 10 * time.Second
	healthMonitorDBCompletionBudget   = 5 * time.Second
)

type HealthMonitorOptions struct {
	Disabled         bool
	WorkerID         string
	BatchSize        int
	Concurrency      int
	PollInterval     time.Duration
	CheckInterval    time.Duration
	Lease             time.Duration
	ProbeTimeout      time.Duration
	FailureThreshold int
	Now               func() time.Time
}

type HealthMonitor struct {
	service    *Service
	repository domainsandbox.HealthMonitorRepository
	options    HealthMonitorOptions
	runMu      sync.Mutex
	lifecycleMu sync.Mutex
	started     bool
	cancel      context.CancelFunc
	done        chan struct{}
}

func DefaultHealthMonitorOptions() HealthMonitorOptions {
	return HealthMonitorOptions{
		WorkerID:         defaultHealthMonitorWorkerID(),
		BatchSize:        defaultHealthMonitorBatchSize,
		Concurrency:      defaultHealthMonitorConcurrency,
		PollInterval:     defaultHealthMonitorPollInterval,
		CheckInterval:    defaultHealthMonitorCheckInterval,
		Lease:             defaultHealthMonitorLease,
		ProbeTimeout:      defaultHealthMonitorProbeTimeout,
		FailureThreshold: domainsandbox.DefaultHealthIncidentFailureThreshold,
		Now:               time.Now,
	}
}

func NewHealthMonitor(
	service *Service,
	repository domainsandbox.HealthMonitorRepository,
	options HealthMonitorOptions,
) (*HealthMonitor, error) {
	defaults := DefaultHealthMonitorOptions()
	if strings.TrimSpace(options.WorkerID) == "" {
		options.WorkerID = defaults.WorkerID
	}
	if options.BatchSize == 0 {
		options.BatchSize = defaults.BatchSize
	}
	if options.Concurrency == 0 {
		options.Concurrency = defaults.Concurrency
	}
	if options.PollInterval == 0 {
		options.PollInterval = defaults.PollInterval
	}
	if options.CheckInterval == 0 {
		options.CheckInterval = defaults.CheckInterval
	}
	if options.Lease == 0 {
		options.Lease = defaults.Lease
	}
	if options.ProbeTimeout == 0 {
		options.ProbeTimeout = defaults.ProbeTimeout
	}
	if options.FailureThreshold == 0 {
		options.FailureThreshold = defaults.FailureThreshold
	}
	if options.Now == nil {
		options.Now = defaults.Now
	}
	monitor := &HealthMonitor{
		service: service,
		repository: repository,
		options: options,
	}
	if options.Disabled {
		return monitor, nil
	}
	if service == nil ||
		repository == nil ||
		len(strings.TrimSpace(options.WorkerID)) > 80 ||
		options.BatchSize < 1 ||
		options.BatchSize > domainsandbox.MaxHealthMonitorBatchSize ||
		options.Concurrency < 1 ||
		options.Concurrency > options.BatchSize ||
		options.PollInterval <= 0 ||
		options.CheckInterval <= 0 ||
		options.ProbeTimeout <= 0 ||
		options.Lease <= options.ProbeTimeout+healthMonitorDBCompletionBudget ||
		options.FailureThreshold < 1 ||
		options.FailureThreshold > 100 {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return monitor, nil
}

func (m *HealthMonitor) Start(ctx context.Context) {
	if m == nil || m.options.Disabled || ctx == nil {
		return
	}
	m.lifecycleMu.Lock()
	defer m.lifecycleMu.Unlock()
	if m.started {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	m.started = true
	m.cancel = cancel
	m.done = done
	go func() {
		defer close(done)
		m.Run(runCtx)
	}()
}

func (m *HealthMonitor) Shutdown(ctx context.Context) error {
	if m == nil {
		return nil
	}
	if ctx == nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	m.lifecycleMu.Lock()
	if !m.started {
		m.lifecycleMu.Unlock()
		return nil
	}
	cancel := m.cancel
	done := m.done
	m.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *HealthMonitor) ShutdownName() string {
	return "sandbox-health-monitor"
}

func (m *HealthMonitor) Run(ctx context.Context) {
	if m == nil || m.options.Disabled || ctx == nil {
		return
	}
	if err := m.RunOnce(ctx); err != nil && ctx.Err() == nil {
		logs.CtxWarnf(
			ctx,
			"[sandbox] health monitor initial run failed error_code=%s",
			healthMonitorErrorCode(err),
		)
	}
	ticker := time.NewTicker(m.options.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.RunOnce(ctx); err != nil && ctx.Err() == nil {
				logs.CtxWarnf(
					ctx,
					"[sandbox] health monitor tick failed error_code=%s",
					healthMonitorErrorCode(err),
				)
			}
		}
	}
}

func (m *HealthMonitor) RunOnce(ctx context.Context) error {
	if m == nil || m.options.Disabled {
		return nil
	}
	if m.service == nil || m.repository == nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	if ctx == nil {
		return domainsandbox.ErrConfigurationInvalid
	}
	m.runMu.Lock()
	defer m.runMu.Unlock()

	now := m.options.Now().UTC().Truncate(time.Millisecond)
	if now.IsZero() {
		return domainsandbox.ErrConfigurationInvalid
	}
	var combined error
	if _, err := m.repository.ReconcileDisabledHealthEpisodes(
		ctx,
		now,
		m.options.BatchSize,
	); err != nil {
		combined = errors.Join(combined, err)
	} else {
		remaining := m.options.BatchSize
		for remaining > 0 {
			claimLimit := m.options.Concurrency
			if claimLimit > remaining {
				claimLimit = remaining
			}
			claims, err := m.repository.ClaimHealthChecks(
				ctx,
				domainsandbox.HealthMonitorClaimRequest{
					WorkerID: m.options.WorkerID,
					Now: now,
					Lease: m.options.Lease,
					Limit: claimLimit,
				},
			)
			if err != nil {
				combined = errors.Join(combined, err)
				break
			}
			if len(claims) == 0 {
				break
			}
			combined = errors.Join(combined, m.processClaims(ctx, claims))
			remaining -= len(claims)
		}
	}
	projectionRemaining := m.options.BatchSize
	for projectionRemaining > 0 {
		projectionLimit := m.options.Concurrency
		if projectionLimit > projectionRemaining {
			projectionLimit = projectionRemaining
		}
		projectionResult, projectionErr :=
			m.repository.ProjectPendingHealthNotifications(
				ctx,
				domainsandbox.HealthNotificationProjectionRequest{
					WorkerID: m.options.WorkerID,
					Now: now,
					Lease: m.options.Lease,
					Limit: projectionLimit,
				},
			)
		combined = errors.Join(combined, projectionErr)
		if projectionResult.Claimed <= 0 {
			break
		}
		projectionRemaining -= projectionResult.Claimed
	}
	return combined
}

func (m *HealthMonitor) processClaims(
	ctx context.Context,
	claims []domainsandbox.HealthMonitorClaim,
) error {
	workerCount := m.options.Concurrency
	if workerCount > len(claims) {
		workerCount = len(claims)
	}
	jobs := make(chan domainsandbox.HealthMonitorClaim, len(claims))
	for _, claim := range claims {
		jobs <- claim
	}
	close(jobs)

	var wait sync.WaitGroup
	errorsCh := make(chan error, len(claims))
	for index := 0; index < workerCount; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for claim := range jobs {
				if err := m.processClaim(ctx, claim); err != nil {
					errorsCh <- err
				}
			}
		}()
	}
	wait.Wait()
	close(errorsCh)
	var combined error
	for err := range errorsCh {
		combined = errors.Join(combined, err)
	}
	return combined
}

func (m *HealthMonitor) processClaim(
	ctx context.Context,
	claim domainsandbox.HealthMonitorClaim,
) error {
	if claim.Provider == nil ||
		claim.Provider.ID <= 0 ||
		claim.Provider.Status != domainsandbox.ProviderStatusEnabled {
		return domainsandbox.ErrInvalidInput
	}
	probeCtx, cancel := context.WithTimeout(ctx, m.options.ProbeTimeout)
	probe := m.service.probeProviderHealth(probeCtx, claim.Provider)
	cancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	checkedAt := probe.Snapshot.CheckedAt.UTC().Truncate(time.Millisecond)
	_, err := m.repository.CompleteHealthCheck(ctx, domainsandbox.CompleteHealthMonitorCheckInput{
		Claim: claim,
		Health: probe.Snapshot,
		CheckedAt: checkedAt,
		NextCheckAt: checkedAt.Add(m.options.CheckInterval),
		FailureThreshold: m.options.FailureThreshold,
	})
	if errors.Is(err, domainsandbox.ErrVersionConflict) {
		return nil
	}
	return err
}

func defaultHealthMonitorWorkerID() string {
	host, _ := os.Hostname()
	host = strings.TrimSpace(host)
	host = strings.Map(func(character rune) rune {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '.' ||
			character == '_' ||
			character == '-' {
			return character
		}
		return '-'
	}, host)
	if len(host) > 40 {
		host = host[:40]
	}
	return fmt.Sprintf("sandbox-health-%s-%d", host, os.Getpid())
}

func healthMonitorErrorCode(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	case errors.Is(err, domainsandbox.ErrVersionConflict):
		return "claim"
	case errors.Is(err, domainsandbox.ErrHealthMonitorDBClock):
		return "db_clock"
	case errors.Is(err, domainsandbox.ErrHealthMonitorOutbox):
		return "outbox"
	case errors.Is(err, domainsandbox.ErrConfigurationInvalid),
		errors.Is(err, domainsandbox.ErrInvalidInput):
		return "config"
	default:
		return "storage"
	}
}
