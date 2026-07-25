// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package billing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/hertz/pkg/common/hlog"

	domainbilling "github.com/coze-dev/coze-studio/backend/domain/billing"
	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

const (
	defaultMaintenanceInterval        = time.Minute
	defaultSubscriptionExpiringWindow = 7 * 24 * time.Hour
	maintenanceProjectionBatchSize    = 500
)

type MaintenanceWorker struct {
	service        maintenanceRunService
	interval       time.Duration
	expiringWindow time.Duration
	now            func() time.Time
	mu             sync.Mutex
	started        bool
	cancel         context.CancelFunc
	done           chan struct{}
}

type maintenanceRunService interface {
	runMaintenanceAt(
		context.Context,
		time.Time,
		time.Duration,
	) (*MaintenanceResult, error)
}

func StartMaintenanceWorkerFromEnv(ctx context.Context, service *Service) *MaintenanceWorker {
	worker := NewMaintenanceWorkerFromEnv(service)
	if worker != nil {
		worker.Start(ctx)
	}
	return worker
}

func NewMaintenanceWorkerFromEnv(service *Service) *MaintenanceWorker {
	enabled, _ := strconv.ParseBool(os.Getenv("BILLING_MAINTENANCE_WORKER_ENABLED"))
	if !enabled || service == nil {
		return nil
	}
	interval := defaultMaintenanceInterval
	if configured := os.Getenv("BILLING_MAINTENANCE_INTERVAL"); configured != "" {
		if parsed, err := time.ParseDuration(configured); err == nil && parsed >= time.Second {
			interval = parsed
		}
	}
	worker := &MaintenanceWorker{
		service: service, interval: interval,
		expiringWindow: configuredSubscriptionExpiringWindow(),
		now:            time.Now,
	}
	return worker
}

func (w *MaintenanceWorker) Start(ctx context.Context) {
	if w == nil || ctx == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	w.started = true
	w.cancel = cancel
	w.done = done
	go func() {
		defer close(done)
		w.run(runCtx)
	}()
}

func (w *MaintenanceWorker) Shutdown(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("billing maintenance shutdown context is required")
	}
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return nil
	}
	cancel := w.cancel
	done := w.done
	w.mu.Unlock()
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

func (w *MaintenanceWorker) ShutdownName() string {
	return "billing-maintenance-worker"
}

func (w *MaintenanceWorker) run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		result, err := w.runOnce(ctx)
		if result != nil {
			logMaintenanceProjection(ctx, result.NotificationProjection)
		}
		if err != nil && ctx.Err() == nil {
			failureCount := int64(1)
			if result != nil {
				failureCount = result.NotificationProjection.Expiring.Failed +
					result.NotificationProjection.Expired.Failed +
					result.NotificationProjection.Orders.Failed
				if failureCount == 0 {
					failureCount = 1
				}
			}
			hlog.CtxErrorf(ctx, "[billing-maintenance] run failed failure_count=%d", failureCount)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *MaintenanceWorker) runOnce(ctx context.Context) (*MaintenanceResult, error) {
	if w == nil || w.service == nil {
		return nil, fmt.Errorf("billing maintenance service is unavailable")
	}
	now := time.Now
	if w.now != nil {
		now = w.now
	}
	window := w.expiringWindow
	if window <= 0 {
		window = defaultSubscriptionExpiringWindow
	}
	return w.service.runMaintenanceAt(ctx, now().UTC(), window)
}

func configuredSubscriptionExpiringWindow() time.Duration {
	return parseSubscriptionExpiringWindow(os.Getenv("BILLING_SUBSCRIPTION_EXPIRING_WINDOW"))
}

func parseSubscriptionExpiringWindow(value string) time.Duration {
	parsed, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return defaultSubscriptionExpiringWindow
	}
	return parsed
}

func runMaintenanceNotificationProjection(
	ctx context.Context,
	repository domainbilling.MaintenanceProjectionRepository,
	now time.Time,
	expiringWindow time.Duration,
) (domainbilling.MaintenanceProjectionResult, []error) {
	result := domainbilling.MaintenanceProjectionResult{}
	failures := make([]error, 0)
	if repository == nil || now.IsZero() || expiringWindow <= 0 {
		return result, []error{fmt.Errorf("billing notification maintenance is unavailable")}
	}
	now = now.UTC()

	expiring, err := repository.ScanExpiringSubscriptions(
		ctx, now, expiringWindow, maintenanceProjectionBatchSize,
	)
	if err != nil {
		recordMaintenanceFailure(&result.Expiring, err, false)
		failures = append(failures, fmt.Errorf("scan expiring subscriptions: %w", err))
	} else {
		recordMaintenanceIssues(&result.Expiring, expiring.Issues)
		for _, candidate := range expiring.Candidates {
			result.Expiring.Scanned++
			outcome, projectErr := repository.ProjectExpiringSubscription(ctx, candidate, now, expiringWindow)
			if projectErr != nil {
				recordMaintenanceFailure(
					&result.Expiring,
					projectErr,
					errors.Is(projectErr, domainnotification.ErrRecipientResolution),
				)
				failures = append(failures, fmt.Errorf("project expiring subscription: %w", projectErr))
				continue
			}
			recordMaintenanceOutcome(&result.Expiring, outcome)
			if outcome.NotificationInserted {
				result.ExpiringSubscriptions++
			}
		}
	}

	due, err := repository.ScanDueSubscriptions(ctx, now, maintenanceProjectionBatchSize)
	if err != nil {
		recordMaintenanceFailure(&result.Expired, err, false)
		failures = append(failures, fmt.Errorf("scan due subscriptions: %w", err))
	} else {
		recordMaintenanceIssues(&result.Expired, due.Issues)
		for _, candidate := range due.Candidates {
			result.Expired.Scanned++
			outcome, expireErr := repository.ExpireDueSubscriptionProjection(ctx, candidate, now)
			if expireErr != nil {
				recordMaintenanceFailure(
					&result.Expired,
					expireErr,
					errors.Is(expireErr, domainnotification.ErrRecipientResolution),
				)
				failures = append(failures, fmt.Errorf("expire subscription: %w", expireErr))
				continue
			}
			recordMaintenanceOutcome(&result.Expired, outcome)
			if outcome.Transitioned {
				result.ExpiredSubscriptions++
			}
		}
	}

	orders, err := repository.ScanExpiredOrders(ctx, now, maintenanceProjectionBatchSize)
	if err != nil {
		recordMaintenanceFailure(&result.Orders, err, false)
		failures = append(failures, fmt.Errorf("scan expired orders: %w", err))
	} else {
		recordMaintenanceIssues(&result.Orders, orders.Issues)
		for _, candidate := range orders.Candidates {
			result.Orders.Scanned++
			outcome, closeErr := repository.CloseExpiredOrderProjection(ctx, candidate, now)
			if closeErr != nil {
				recordMaintenanceFailure(
					&result.Orders,
					closeErr,
					errors.Is(closeErr, domainnotification.ErrRecipientResolution),
				)
				failures = append(failures, fmt.Errorf("close expired order: %w", closeErr))
				continue
			}
			recordMaintenanceOutcome(&result.Orders, outcome)
			if outcome.Transitioned {
				result.ClosedOrders++
			}
		}
	}
	return result, failures
}

func recordMaintenanceIssues(
	stage *domainbilling.MaintenanceStageResult,
	issues []domainbilling.MaintenanceIssueSummary,
) {
	if stage == nil {
		return
	}
	for _, issue := range issues {
		stage.InvalidCandidates += int64(issue.Count)
		if len(stage.Issues) < domainbilling.MaxMaintenanceIssueSummaries {
			stage.Issues = append(stage.Issues, issue)
		}
	}
}

func recordMaintenanceFailure(
	stage *domainbilling.MaintenanceStageResult,
	err error,
	invalid bool,
) {
	if stage == nil {
		return
	}
	if invalid {
		stage.InvalidCandidates++
	} else {
		stage.Failed++
	}
	code := string(domainnotification.StableErrorCode(err))
	for index := range stage.Errors {
		if stage.Errors[index].Code == code {
			stage.Errors[index].Count++
			return
		}
	}
	if len(stage.Errors) < domainbilling.MaxMaintenanceIssueSummaries {
		stage.Errors = append(stage.Errors, domainbilling.MaintenanceErrorSummary{
			Code: code, Count: 1,
		})
	}
}

func recordMaintenanceOutcome(
	stage *domainbilling.MaintenanceStageResult,
	outcome domainbilling.MaintenanceProjectionOutcome,
) {
	if stage == nil {
		return
	}
	stage.Succeeded++
	if outcome.Stale {
		stage.StaleCandidates++
		return
	}
	if outcome.NotificationInserted {
		stage.NewProjections++
	} else {
		stage.DuplicateProjections++
	}
}

func logMaintenanceProjection(ctx context.Context, result domainbilling.MaintenanceProjectionResult) {
	hlog.CtxInfof(
		ctx,
		"[billing-maintenance] projection expiring_scanned=%d expiring_succeeded=%d expiring_new=%d expiring_duplicate=%d expiring_invalid=%d expiring_stale=%d expiring_failed=%d expiring_errors=%s expired_scanned=%d expired_succeeded=%d expired_new=%d expired_duplicate=%d expired_invalid=%d expired_stale=%d expired_failed=%d expired_errors=%s orders_scanned=%d orders_succeeded=%d orders_new=%d orders_duplicate=%d orders_invalid=%d orders_stale=%d orders_failed=%d orders_errors=%s",
		result.Expiring.Scanned,
		result.Expiring.Succeeded,
		result.Expiring.NewProjections,
		result.Expiring.DuplicateProjections,
		result.Expiring.InvalidCandidates,
		result.Expiring.StaleCandidates,
		result.Expiring.Failed,
		formatMaintenanceErrorSummaries(result.Expiring.Errors),
		result.Expired.Scanned,
		result.Expired.Succeeded,
		result.Expired.NewProjections,
		result.Expired.DuplicateProjections,
		result.Expired.InvalidCandidates,
		result.Expired.StaleCandidates,
		result.Expired.Failed,
		formatMaintenanceErrorSummaries(result.Expired.Errors),
		result.Orders.Scanned,
		result.Orders.Succeeded,
		result.Orders.NewProjections,
		result.Orders.DuplicateProjections,
		result.Orders.InvalidCandidates,
		result.Orders.StaleCandidates,
		result.Orders.Failed,
		formatMaintenanceErrorSummaries(result.Orders.Errors),
	)
}

func formatMaintenanceErrorSummaries(summaries []domainbilling.MaintenanceErrorSummary) string {
	if len(summaries) == 0 {
		return "none"
	}
	var builder strings.Builder
	for index, summary := range summaries {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(summary.Code)
		builder.WriteByte(':')
		builder.WriteString(strconv.FormatInt(summary.Count, 10))
	}
	return builder.String()
}

func boundedMaintenanceFailureSummaries(failures []error) []string {
	type errorCount struct {
		code  string
		count int
	}
	counts := make([]errorCount, 0, domainbilling.MaxMaintenanceIssueSummaries)
	for _, failure := range failures {
		code := string(domainnotification.StableErrorCode(failure))
		found := false
		for index := range counts {
			if counts[index].code == code {
				counts[index].count++
				found = true
				break
			}
		}
		if !found && len(counts) < domainbilling.MaxMaintenanceIssueSummaries {
			counts = append(counts, errorCount{code: code, count: 1})
		}
	}
	result := make([]string, 0, len(counts))
	for _, count := range counts {
		result = append(result, fmt.Sprintf("%s:%d", count.code, count.count))
	}
	return result
}
