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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

const (
	agentJournalRetentionWorkerEnabledEnv    = "AGENT_JOURNAL_RETENTION_WORKER_ENABLED"
	agentJournalRetentionIntervalMsEnv       = "AGENT_JOURNAL_RETENTION_INTERVAL_MS"
	agentJournalRetentionBatchSizeEnv        = "AGENT_JOURNAL_RETENTION_BATCH_SIZE"
	agentJournalRetentionLeaseSecondsEnv     = "AGENT_JOURNAL_RETENTION_LEASE_SECONDS"
	agentJournalRetentionStagingGraceMinsEnv = "AGENT_JOURNAL_RETENTION_STAGING_GRACE_MINUTES"

	defaultJournalSnapshotRetention  = 30 * 24 * time.Hour
	defaultJournalExecutionRetention = 90 * 24 * time.Hour
	defaultJournalStagingGrace       = time.Hour
	defaultJournalRetentionInterval  = time.Hour
	defaultJournalRetentionLease     = 5 * time.Minute
	defaultJournalRetentionBatchSize = 100
	journalRetentionObjectPageSize   = 1_000
	journalRetentionMaxObjectPages   = 10_000
)

type JournalRetentionClaimRequest = domainrepo.JournalRetentionClaimRequest
type JournalSnapshotCleanupClaim = domainrepo.JournalSnapshotCleanupClaim
type JournalStagingCleanupClaim = domainrepo.JournalStagingCleanupClaim
type JournalRetentionDeleteRequest = domainrepo.JournalRetentionDeleteRequest
type JournalRetentionBacklogRequest = domainrepo.JournalRetentionBacklogRequest
type JournalExecutionCleanupResult = domainrepo.JournalExecutionCleanupResult
type JournalRetentionRepository = domainrepo.JournalRetentionRepository

type JournalRetentionWorkerOptions struct {
	Interval           time.Duration
	SnapshotRetention  time.Duration
	ExecutionRetention time.Duration
	StagingGrace       time.Duration
	LeaseTTL           time.Duration
	BatchSize          int
	Now                func() time.Time
	Metrics            *JournalPrometheusMetricsCollector
}

type JournalRetentionWorker struct {
	repository         JournalRetentionRepository
	storage            JournalSnapshotObjectStorage
	interval           time.Duration
	snapshotRetention  time.Duration
	executionRetention time.Duration
	stagingGrace       time.Duration
	leaseTTL           time.Duration
	batchSize          int
	now                func() time.Time
	metrics            *JournalPrometheusMetricsCollector
}

type JournalRetentionResult struct {
	SnapshotsDeleted       int64
	StagingPrefixesDeleted int64
	CheckpointsDeleted     int64
	EventsDeleted          int64
	LedgersDeleted         int64
	AttemptsDeleted        int64
	Backlog                int64
}

type JournalRetentionWorkerEnvStatus struct {
	Enabled bool
	Started bool
	Reason  string
}

func NewJournalRetentionWorker(
	repository JournalRetentionRepository,
	storage JournalSnapshotObjectStorage,
	options JournalRetentionWorkerOptions,
) *JournalRetentionWorker {
	interval := options.Interval
	if interval <= 0 {
		interval = defaultJournalRetentionInterval
	}
	snapshotRetention := options.SnapshotRetention
	if snapshotRetention <= 0 {
		snapshotRetention = defaultJournalSnapshotRetention
	}
	executionRetention := options.ExecutionRetention
	if executionRetention <= 0 {
		executionRetention = defaultJournalExecutionRetention
	}
	stagingGrace := options.StagingGrace
	if stagingGrace <= 0 {
		stagingGrace = defaultJournalStagingGrace
	}
	leaseTTL := options.LeaseTTL
	if leaseTTL <= 0 {
		leaseTTL = defaultJournalRetentionLease
	}
	batchSize := options.BatchSize
	if batchSize <= 0 || batchSize > 1_000 {
		batchSize = defaultJournalRetentionBatchSize
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &JournalRetentionWorker{
		repository: repository, storage: storage, interval: interval,
		snapshotRetention: snapshotRetention, executionRetention: executionRetention,
		stagingGrace: stagingGrace, leaseTTL: leaseTTL, batchSize: batchSize,
		now: now, metrics: options.Metrics,
	}
}

func (w *JournalRetentionWorker) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if w == nil || w.repository == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, err := w.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
					logs.CtxErrorf(ctx, "[journal-retention] cleanup tick failed: %v", err)
				}
			}
		}
	}()
	return done
}

func (w *JournalRetentionWorker) RunOnce(ctx context.Context) (JournalRetentionResult, error) {
	result := JournalRetentionResult{}
	if w == nil || w.repository == nil {
		return result, fmt.Errorf("journal retention repository is unavailable")
	}
	now := w.now()
	claimRequest := JournalRetentionClaimRequest{
		CutoffAt: now.Add(-w.snapshotRetention).UnixMilli(),
		Now:      now.UnixMilli(), LeaseUntil: now.Add(w.leaseTTL).UnixMilli(), BatchSize: w.batchSize,
	}
	claims, err := w.repository.ClaimJournalSnapshotCleanup(ctx, claimRequest)
	if err != nil {
		return w.finish(ctx, result, err)
	}
	for _, claim := range claims {
		if err := ctx.Err(); err != nil {
			return w.finish(ctx, result, err)
		}
		if err := w.deleteObjectKeys(ctx, claim.SpaceID, claim.ObjectKeys); err != nil {
			releaseErr := w.repository.ReleaseJournalSnapshotCleanup(ctx, claim, "object_delete_failed")
			return w.finish(ctx, result, errors.Join(err, releaseErr))
		}
		if err := w.repository.CompleteJournalSnapshotCleanup(ctx, claim); err != nil {
			return w.finish(ctx, result, err)
		}
		result.SnapshotsDeleted++
	}

	stagingRequest := claimRequest
	stagingRequest.CutoffAt = now.Add(-w.stagingGrace).UnixMilli()
	stagingClaims, err := w.repository.ClaimJournalStagingCleanup(ctx, stagingRequest)
	if err != nil {
		return w.finish(ctx, result, err)
	}
	for _, claim := range stagingClaims {
		if err := w.deleteStagingPrefix(ctx, claim.SpaceID, claim.Prefix); err != nil {
			releaseErr := w.repository.ReleaseJournalStagingCleanup(ctx, claim, "object_delete_failed")
			return w.finish(ctx, result, errors.Join(err, releaseErr))
		}
		if err := w.repository.CompleteJournalStagingCleanup(ctx, claim); err != nil {
			return w.finish(ctx, result, err)
		}
		result.StagingPrefixesDeleted++
	}

	delete30Day := JournalRetentionDeleteRequest{
		CutoffAt: now.Add(-w.snapshotRetention).UnixMilli(), BatchSize: w.batchSize,
	}
	result.CheckpointsDeleted, err = w.repository.DeleteExpiredJournalCheckpoints(ctx, delete30Day)
	if err != nil {
		return w.finish(ctx, result, err)
	}
	delete90Day := JournalRetentionDeleteRequest{
		CutoffAt: now.Add(-w.executionRetention).UnixMilli(), BatchSize: w.batchSize,
	}
	executionResult, err := w.repository.DeleteExpiredJournalEventsAndLedgers(ctx, delete90Day)
	if err != nil {
		return w.finish(ctx, result, err)
	}
	result.EventsDeleted = executionResult.EventsDeleted
	result.LedgersDeleted = executionResult.LedgersDeleted
	result.AttemptsDeleted, err = w.repository.DeleteUnreferencedJournalAttempts(ctx, delete90Day)
	if err != nil {
		return w.finish(ctx, result, err)
	}
	return w.finish(ctx, result, nil)
}

func (w *JournalRetentionWorker) finish(
	ctx context.Context,
	result JournalRetentionResult,
	cleanupErr error,
) (JournalRetentionResult, error) {
	now := w.now()
	backlog, backlogErr := w.repository.CountJournalRetentionBacklog(
		ctx,
		JournalRetentionBacklogRequest{
			SnapshotCutoffAt:  now.Add(-w.snapshotRetention).UnixMilli(),
			ExecutionCutoffAt: now.Add(-w.executionRetention).UnixMilli(),
			StagingCutoffAt:   now.Add(-w.stagingGrace).UnixMilli(),
		},
	)
	if backlogErr == nil {
		result.Backlog = backlog
	}
	joined := errors.Join(cleanupErr, backlogErr)
	if w.metrics != nil && backlogErr == nil {
		labels := JournalMetricLabels{
			Version: entity.JournalSchemaVersion, RolloutCohort: "treatment",
			TaskType: "unknown", ClientVersion: "unknown", Result: "current", ErrorCode: "none",
		}
		w.metrics.SetRetentionBacklog(ctx, labels, result.Backlog)
	}
	return result, joined
}

func (w *JournalRetentionWorker) deleteObjectKeys(
	ctx context.Context,
	spaceID int64,
	keys []string,
) error {
	if spaceID <= 0 {
		return fmt.Errorf("journal retention object scope is invalid")
	}
	tenantPrefix := fmt.Sprintf("journal-snapshots/%d/", spaceID)
	tenantStagingPrefix := fmt.Sprintf("journal-snapshots/staging/%d/", spaceID)
	seen := make(map[string]struct{}, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if !strings.HasPrefix(key, tenantPrefix) && !strings.HasPrefix(key, tenantStagingPrefix) {
			return fmt.Errorf("journal retention object key is outside the tenant scope")
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if w.storage == nil {
			return fmt.Errorf("journal snapshot object storage is unavailable")
		}
		if err := w.storage.DeleteJournalSnapshotObject(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

func (w *JournalRetentionWorker) deleteStagingPrefix(
	ctx context.Context,
	spaceID int64,
	rawPrefix string,
) error {
	prefix := strings.TrimSpace(rawPrefix)
	tenantPrefix := fmt.Sprintf("journal-snapshots/staging/%d/", spaceID)
	if spaceID <= 0 || !strings.HasPrefix(prefix, tenantPrefix) || w.storage == nil {
		return fmt.Errorf("journal staging cleanup prefix is invalid or storage is unavailable")
	}
	cursor := ""
	for page := 0; page < journalRetentionMaxObjectPages; page++ {
		objects, next, err := w.storage.ListJournalSnapshotObjects(
			ctx, prefix, cursor, journalRetentionObjectPageSize,
		)
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(objects))
		for _, object := range objects {
			if !strings.HasPrefix(strings.TrimSpace(object.Key), prefix) {
				return fmt.Errorf("journal staging listing escaped the claimed prefix")
			}
			keys = append(keys, object.Key)
		}
		if err := w.deleteObjectKeys(ctx, spaceID, keys); err != nil {
			return err
		}
		if next == "" {
			return nil
		}
		if next == cursor {
			return fmt.Errorf("journal staging listing cursor did not advance")
		}
		cursor = next
	}
	return fmt.Errorf("journal staging cleanup exceeded the object page limit")
}

func StartJournalRetentionWorkerFromEnvWithStatus(
	ctx context.Context,
	repository JournalRetentionRepository,
	storage JournalSnapshotObjectStorage,
	metrics *JournalPrometheusMetricsCollector,
) (*JournalRetentionWorker, JournalRetentionWorkerEnvStatus) {
	status := JournalRetentionWorkerEnvStatus{
		Enabled: envkey.GetBoolD(agentJournalRetentionWorkerEnabledEnv, false),
	}
	if !status.Enabled {
		return nil, status
	}
	if repository == nil || storage == nil {
		status.Reason = "journal retention dependencies are unavailable"
		logs.CtxWarnf(ctx, "[journal-retention] enabled but dependencies are unavailable")
		return nil, status
	}
	interval := time.Duration(envkey.GetIntD(
		agentJournalRetentionIntervalMsEnv,
		int(defaultJournalRetentionInterval/time.Millisecond),
	)) * time.Millisecond
	batchSize := envkey.GetIntD(agentJournalRetentionBatchSizeEnv, defaultJournalRetentionBatchSize)
	leaseTTL := time.Duration(envkey.GetIntD(
		agentJournalRetentionLeaseSecondsEnv,
		int(defaultJournalRetentionLease/time.Second),
	)) * time.Second
	stagingGrace := time.Duration(envkey.GetIntD(
		agentJournalRetentionStagingGraceMinsEnv,
		int(defaultJournalStagingGrace/time.Minute),
	)) * time.Minute
	if interval <= 0 || batchSize <= 0 || batchSize > 1_000 || leaseTTL < time.Minute ||
		leaseTTL > time.Hour || stagingGrace < time.Minute || stagingGrace > 24*time.Hour {
		status.Reason = "journal retention configuration is invalid"
		logs.CtxWarnf(ctx, "[journal-retention] enabled but configuration is invalid")
		return nil, status
	}
	worker := NewJournalRetentionWorker(repository, storage, JournalRetentionWorkerOptions{
		Interval: interval, BatchSize: batchSize, LeaseTTL: leaseTTL,
		StagingGrace: stagingGrace, Metrics: metrics,
	})
	worker.Start(ctx)
	status.Started = true
	return worker, status
}
