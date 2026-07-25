// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type WorkerRepository interface {
	RecoverExpiredLeases(context.Context, time.Time, time.Duration) (int64, error)
	ClaimOutboxBatch(context.Context, string, time.Time, time.Duration, int) ([]domainnotification.OutboxClaim, error)
	MaterializeAndDeliver(context.Context, domainnotification.OutboxClaim, domainnotification.MessageDraft, []int64, time.Time) error
	FailClaim(context.Context, domainnotification.OutboxClaim, string, time.Time, time.Time, bool) error
}

type RecipientResolver interface {
	Resolve(context.Context, domainnotification.Event) ([]int64, error)
}

type RecipientResolverFunc func(
	context.Context,
	domainnotification.Event,
) ([]int64, error)

type SystemAdminRecipientSource interface {
	ListSystemAdministratorUserIDs(context.Context) ([]int64, error)
}

func (f RecipientResolverFunc) Resolve(
	ctx context.Context,
	event domainnotification.Event,
) ([]int64, error) {
	return f(ctx, event)
}

// PolicyRecipientResolver resolves only server-owned policies. System
// administrator recipients come from persisted configuration and user facts;
// no event payload or client claim can grant administrator routing.
type PolicyRecipientResolver struct {
	SystemAdmins SystemAdminRecipientSource
}

func (r PolicyRecipientResolver) Resolve(
	ctx context.Context,
	event domainnotification.Event,
) ([]int64, error) {
	var ids []int64
	switch event.RecipientPolicy {
	case domainnotification.RecipientActor:
		ids = []int64{event.ActorID}
	case domainnotification.RecipientExplicitInternalUsers:
		ids = event.Payload.ExplicitRecipientIDs
	case domainnotification.RecipientSystemAdmins:
		if r.SystemAdmins == nil {
			return nil, fmt.Errorf(
				"%w: system administrator source is unavailable",
				domainnotification.ErrRecipientPolicyUnavailable,
			)
		}
		var err error
		ids, err = r.SystemAdmins.ListSystemAdministratorUserIDs(ctx)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: system administrator lookup failed: %w",
				domainnotification.ErrRecipientResolution,
				err,
			)
		}
	default:
		return nil, fmt.Errorf(
			"%w: policy %s has no core resolver",
			domainnotification.ErrRecipientPolicyUnavailable,
			event.RecipientPolicy,
		)
	}
	return normalizeSortedRecipientIDs(ids)
}

func normalizeSortedRecipientIDs(ids []int64) ([]int64, error) {
	normalized, err := domainnotification.NormalizeRecipientIDs(
		append([]int64(nil), ids...),
	)
	if err != nil {
		return nil, err
	}
	sort.Slice(normalized, func(left, right int) bool {
		return normalized[left] < normalized[right]
	})
	return normalized, nil
}

type WorkerOptions struct {
	WorkerID              string
	BatchSize             int
	Concurrency           int
	Lease                 time.Duration
	BatchTimeout          time.Duration
	ClaimTimeout          time.Duration
	FailurePersistTimeout time.Duration
	PollInterval          time.Duration
	MaxAttempts           int
	RetryDelays           []time.Duration
	// Now supplies retry durations and the explicit SQLite test clock.
	// Production MySQL repositories rebase all persisted boundaries on DB UTC.
	Now                   func() time.Time
}

type Worker struct {
	repository WorkerRepository
	resolver   RecipientResolver
	registry   *domainnotification.TemplateRegistry
	options    WorkerOptions
	runMu      sync.Mutex
	configurationErr error
	monotonicNow     func() time.Time
}

func DefaultWorkerOptions() WorkerOptions {
	host, _ := os.Hostname()
	return WorkerOptions{
		WorkerID:              fmt.Sprintf("notification-%s-%d", host, os.Getpid()),
		BatchSize:             20,
		Concurrency:           10,
		Lease:                 30 * time.Second,
		BatchTimeout:          10 * time.Second,
		ClaimTimeout:          3 * time.Second,
		FailurePersistTimeout: time.Second,
		PollInterval:          2 * time.Second,
		MaxAttempts:           8,
		RetryDelays: []time.Duration{
			5 * time.Second,
			30 * time.Second,
			2 * time.Minute,
			10 * time.Minute,
			30 * time.Minute,
			time.Hour,
		},
		Now: time.Now,
	}
}

func NewWorker(
	repository WorkerRepository,
	resolver RecipientResolver,
	options WorkerOptions,
) *Worker {
	defaults := DefaultWorkerOptions()
	if options.WorkerID == "" {
		options.WorkerID = defaults.WorkerID
	}
	if options.BatchSize <= 0 {
		options.BatchSize = defaults.BatchSize
	}
	if options.Concurrency <= 0 {
		options.Concurrency = defaults.Concurrency
	}
	if options.Lease <= 0 {
		options.Lease = defaults.Lease
	}
	if options.BatchTimeout <= 0 {
		options.BatchTimeout = defaults.BatchTimeout
	}
	if options.ClaimTimeout <= 0 {
		options.ClaimTimeout = defaults.ClaimTimeout
	}
	if options.FailurePersistTimeout <= 0 {
		options.FailurePersistTimeout = defaults.FailurePersistTimeout
	}
	if options.PollInterval <= 0 {
		options.PollInterval = defaults.PollInterval
	}
	if options.MaxAttempts <= 0 {
		options.MaxAttempts = defaults.MaxAttempts
	}
	if len(options.RetryDelays) == 0 {
		options.RetryDelays = defaults.RetryDelays
	}
	if options.Now == nil {
		options.Now = defaults.Now
	}
	if resolver == nil {
		resolver = PolicyRecipientResolver{}
	}
	return &Worker{
		repository:       repository,
		resolver:         resolver,
		registry:         domainnotification.DefaultTemplateRegistry(),
		options:          options,
		configurationErr: validateWorkerTiming(options),
		monotonicNow:     time.Now,
	}
}

func (w *Worker) Run(claimLoopCtx context.Context, inFlightCtx context.Context) {
	if w == nil {
		return
	}
	if claimLoopCtx == nil {
		claimLoopCtx = context.Background()
	}
	if inFlightCtx == nil {
		inFlightCtx = context.Background()
	}
	if claimLoopCtx.Err() == nil {
		if err := w.RunOnce(inFlightCtx); err != nil {
			logs.CtxWarnf(
				inFlightCtx,
				"[notification] worker_initial_run_failed error_code=%s",
				domainnotification.StableErrorCode(err),
			)
		}
	}
	ticker := time.NewTicker(w.options.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-claimLoopCtx.Done():
			return
		case <-ticker.C:
			if claimLoopCtx.Err() != nil {
				return
			}
			if err := w.RunOnce(inFlightCtx); err != nil {
				logs.CtxWarnf(
					inFlightCtx,
					"[notification] worker_tick_failed error_code=%s",
					domainnotification.StableErrorCode(err),
				)
			}
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) error {
	if w == nil || w.repository == nil || w.resolver == nil {
		return domainnotification.ErrStorage
	}
	if w.configurationErr != nil {
		return w.configurationErr
	}
	w.runMu.Lock()
	defer w.runMu.Unlock()

	now := w.options.Now()
	if _, err := w.repository.RecoverExpiredLeases(ctx, now, w.options.Lease); err != nil {
		return err
	}
	// The DB may have a different wall clock. Capture before the claim call so
	// the full round trip is conservatively charged to the local monotonic
	// lease budget. The repository remains authoritative when fencing writes.
	claimStartedAt := w.monotonicNow()
	claims, err := w.repository.ClaimOutboxBatch(
		ctx,
		w.options.WorkerID,
		now,
		w.options.Lease,
		w.options.BatchSize,
	)
	if err != nil {
		return err
	}
	return w.processClaimBatch(ctx, claims, claimStartedAt)
}

func (w *Worker) processClaimBatch(
	ctx context.Context,
	claims []domainnotification.OutboxClaim,
	claimStartedAt time.Time,
) error {
	if len(claims) == 0 {
		return nil
	}
	claimElapsed := w.monotonicNow().Sub(claimStartedAt)
	batchBudget, err := claimBatchBudget(
		w.options.BatchTimeout,
		w.options.Lease,
		claimElapsed,
	)
	if err != nil {
		return err
	}
	batchCtx, cancelBatch := context.WithTimeout(ctx, batchBudget)
	defer cancelBatch()

	jobs := make(chan domainnotification.OutboxClaim)
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		defer close(jobs)
		for _, claim := range claims {
			if batchCtx.Err() != nil {
				return
			}
			select {
			case <-batchCtx.Done():
				return
			case jobs <- claim:
			}
		}
	}()

	workerCount := w.options.Concurrency
	if workerCount > len(claims) {
		workerCount = len(claims)
	}
	persistenceErrorCh := make(chan error, len(claims))
	var wait sync.WaitGroup
	for index := 0; index < workerCount; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				if batchCtx.Err() != nil {
					return
				}
				var claim domainnotification.OutboxClaim
				var ok bool
				select {
				case <-batchCtx.Done():
					return
				case claim, ok = <-jobs:
					if !ok {
						return
					}
				}
				// Cancellation can race with a ready job. Recheck after receive
				// before render, recipient resolution, logging, or persistence.
				if batchCtx.Err() != nil {
					return
				}
				claimCtx, cancelClaim := context.WithTimeout(
					batchCtx,
					w.options.ClaimTimeout,
				)
				err := w.processClaim(claimCtx, claim)
				cancelClaim()
				if err != nil {
					persistenceErrorCh <- err
				}
			}
		}()
	}
	wait.Wait()
	cancelBatch()
	<-producerDone
	close(persistenceErrorCh)

	var persistenceErrors []error
	for err := range persistenceErrorCh {
		persistenceErrors = append(persistenceErrors, err)
	}
	if len(persistenceErrors) > 0 {
		return errors.Join(persistenceErrors...)
	}
	return nil
}

func claimBatchBudget(
	batchTimeout time.Duration,
	lease time.Duration,
	claimElapsed time.Duration,
) (time.Duration, error) {
	if batchTimeout <= 0 || lease <= 0 || claimElapsed < 0 {
		return 0, fmt.Errorf(
			"%w: invalid claimed batch timing",
			domainnotification.ErrLeaseLost,
		)
	}
	safeLeaseBudget := lease - claimElapsed - leaseSafetyMargin(lease)
	if safeLeaseBudget <= 0 {
		return 0, fmt.Errorf(
			"%w: claimed batch has no safe processing window",
			domainnotification.ErrLeaseLost,
		)
	}
	if batchTimeout < safeLeaseBudget {
		return batchTimeout, nil
	}
	return safeLeaseBudget, nil
}

func validateWorkerTiming(options WorkerOptions) error {
	if options.BatchSize <= 0 ||
		options.BatchSize > 200 ||
		options.Concurrency <= 0 ||
		options.Lease <= 0 ||
		options.BatchTimeout <= 0 ||
		options.ClaimTimeout <= 0 ||
		options.FailurePersistTimeout <= 0 ||
		options.BatchTimeout >= options.Lease ||
		options.ClaimTimeout >= options.BatchTimeout ||
		options.ClaimTimeout >= options.Lease {
		return fmt.Errorf("%w: invalid notification worker timing", domainnotification.ErrInvalidEvent)
	}
	concurrency := options.Concurrency
	if concurrency > options.BatchSize {
		concurrency = options.BatchSize
	}
	waves := (options.BatchSize + concurrency - 1) / concurrency
	worstCase := time.Duration(waves) *
		(options.ClaimTimeout + options.FailurePersistTimeout)
	safeWindow := options.Lease - leaseSafetyMargin(options.Lease)
	if options.BatchTimeout < safeWindow {
		safeWindow = options.BatchTimeout
	}
	if safeWindow <= 0 || worstCase >= safeWindow {
		return fmt.Errorf(
			"%w: notification batch can outlive its lease",
			domainnotification.ErrInvalidEvent,
		)
	}
	return nil
}

func leaseSafetyMargin(lease time.Duration) time.Duration {
	margin := lease / 10
	if margin < 500*time.Millisecond {
		margin = 500 * time.Millisecond
	}
	return margin
}

func (w *Worker) processClaim(
	ctx context.Context,
	claim domainnotification.OutboxClaim,
) error {
	if ctx == nil || ctx.Err() != nil {
		return nil
	}
	logs.CtxInfof(
		ctx,
		"[notification] event_claimed event_id=%s event_type=%s aggregate_type=%s aggregate_id=%s attempt=%d",
		claim.Event.EventID,
		claim.Event.EventType,
		claim.Event.AggregateType,
		claim.Event.AggregateID,
		claim.AttemptCount+1,
	)
	errorCode := claim.ClaimErrorCode
	var err error
	if errorCode == "" {
		var draft domainnotification.MessageDraft
		draft, err = w.registry.Render(claim.Event)
		if err == nil {
			var recipients []int64
			recipients, err = w.resolver.Resolve(ctx, claim.Event)
			if err == nil {
				recipients, err = normalizeSortedRecipientIDs(recipients)
			}
			if err == nil {
				err = w.repository.MaterializeAndDeliver(
					ctx,
					claim,
					draft,
					recipients,
					w.options.Now(),
				)
				if err == nil {
					logs.CtxInfof(
						ctx,
						"[notification] projection_succeeded event_id=%s event_type=%s aggregate_type=%s aggregate_id=%s recipient_count=%d",
						claim.Event.EventID,
						claim.Event.EventType,
						claim.Event.AggregateType,
						claim.Event.AggregateID,
						len(recipients),
					)
					return nil
				}
			}
		}
	}

	if errorCode == "" {
		errorCode = domainnotification.StableErrorCode(err)
	}
	nextAttempt := claim.AttemptCount + 1
	retryable := domainnotification.IsRetryableErrorCode(errorCode) ||
		claim.ClaimErrorCode != ""
	retryUntilResolved := shouldRetryWithoutDeadLetter(err)
	dead := !retryable ||
		(!retryUntilResolved && nextAttempt >= w.options.MaxAttempts)
	now := w.options.Now()
	nextAvailable := now.Add(w.retryDelay(nextAttempt))
	failureCtx, cancelFailure := context.WithTimeout(
		context.WithoutCancel(ctx),
		w.options.FailurePersistTimeout,
	)
	defer cancelFailure()
	if failErr := w.repository.FailClaim(
		failureCtx,
		claim,
		errorCode,
		now,
		nextAvailable,
		dead,
	); failErr != nil {
		return failErr
	}
	if dead {
		logs.CtxWarnf(
			failureCtx,
			"[notification] event_dead_lettered event_id=%s event_type=%s aggregate_type=%s aggregate_id=%s attempt=%d error_code=%s",
			claim.Event.EventID,
			claim.Event.EventType,
			claim.Event.AggregateType,
			claim.Event.AggregateID,
			nextAttempt,
			errorCode,
		)
		return nil
	}
	logs.CtxWarnf(
		failureCtx,
		"[notification] projection_failed event_id=%s event_type=%s aggregate_type=%s aggregate_id=%s attempt=%d error_code=%s",
		claim.Event.EventID,
		claim.Event.EventType,
		claim.Event.AggregateType,
		claim.Event.AggregateID,
		nextAttempt,
		errorCode,
	)
	return nil
}

type retryWithoutDeadLetter interface {
	RetryWithoutDeadLetter() bool
}

func shouldRetryWithoutDeadLetter(err error) bool {
	var marker retryWithoutDeadLetter
	return errors.As(err, &marker) && marker.RetryWithoutDeadLetter()
}

func (w *Worker) retryDelay(attempt int) time.Duration {
	index := attempt - 1
	if index < 0 {
		index = 0
	}
	if index >= len(w.options.RetryDelays) {
		index = len(w.options.RetryDelays) - 1
	}
	delay := w.options.RetryDelays[index]
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
