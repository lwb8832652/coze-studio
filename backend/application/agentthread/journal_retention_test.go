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
	"io"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

type journalRetentionRepositoryStub struct {
	steps          []string
	snapshotClaims []JournalSnapshotCleanupClaim
	stagingClaims  []JournalStagingCleanupClaim
	backlog        int64
	released       []string
}

func (r *journalRetentionRepositoryStub) ClaimJournalSnapshotCleanup(
	_ context.Context,
	_ JournalRetentionClaimRequest,
) ([]JournalSnapshotCleanupClaim, error) {
	r.steps = append(r.steps, "claim_snapshots")
	claims := r.snapshotClaims
	r.snapshotClaims = nil
	return claims, nil
}

func (r *journalRetentionRepositoryStub) CompleteJournalSnapshotCleanup(
	_ context.Context,
	claim JournalSnapshotCleanupClaim,
) error {
	r.steps = append(r.steps, "complete_snapshot:"+claim.SnapshotID)
	return nil
}

func (r *journalRetentionRepositoryStub) ReleaseJournalSnapshotCleanup(
	_ context.Context,
	claim JournalSnapshotCleanupClaim,
	errorCode string,
) error {
	r.steps = append(r.steps, "release_snapshot:"+claim.SnapshotID)
	r.released = append(r.released, errorCode)
	return nil
}

func (r *journalRetentionRepositoryStub) ClaimJournalStagingCleanup(
	_ context.Context,
	_ JournalRetentionClaimRequest,
) ([]JournalStagingCleanupClaim, error) {
	r.steps = append(r.steps, "claim_staging")
	claims := r.stagingClaims
	r.stagingClaims = nil
	return claims, nil
}

func (r *journalRetentionRepositoryStub) CompleteJournalStagingCleanup(
	_ context.Context,
	claim JournalStagingCleanupClaim,
) error {
	r.steps = append(r.steps, "complete_staging:"+claim.SnapshotID)
	return nil
}

func (r *journalRetentionRepositoryStub) ReleaseJournalStagingCleanup(
	_ context.Context,
	claim JournalStagingCleanupClaim,
	errorCode string,
) error {
	r.steps = append(r.steps, "release_staging:"+claim.SnapshotID)
	r.released = append(r.released, errorCode)
	return nil
}

func (r *journalRetentionRepositoryStub) DeleteExpiredJournalCheckpoints(
	_ context.Context,
	_ JournalRetentionDeleteRequest,
) (int64, error) {
	r.steps = append(r.steps, "delete_checkpoints")
	return 2, nil
}

func (r *journalRetentionRepositoryStub) DeleteExpiredJournalEventsAndLedgers(
	_ context.Context,
	_ JournalRetentionDeleteRequest,
) (JournalExecutionCleanupResult, error) {
	r.steps = append(r.steps, "delete_events_ledgers")
	return JournalExecutionCleanupResult{EventsDeleted: 3, LedgersDeleted: 4}, nil
}

func (r *journalRetentionRepositoryStub) DeleteUnreferencedJournalAttempts(
	_ context.Context,
	_ JournalRetentionDeleteRequest,
) (int64, error) {
	r.steps = append(r.steps, "delete_attempts")
	return 1, nil
}

func (r *journalRetentionRepositoryStub) CountJournalRetentionBacklog(
	_ context.Context,
	_ JournalRetentionBacklogRequest,
) (int64, error) {
	r.steps = append(r.steps, "count_backlog")
	return r.backlog, nil
}

type journalRetentionObjectStorageStub struct {
	deleted   []string
	objects   map[string][]JournalSnapshotObjectInfo
	deleteErr error
}

func (s *journalRetentionObjectStorageStub) PutJournalSnapshotObject(context.Context, string, []byte) error {
	return nil
}

func (s *journalRetentionObjectStorageStub) OpenJournalSnapshotObject(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (s *journalRetentionObjectStorageStub) DeleteJournalSnapshotObject(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return s.deleteErr
}

func (s *journalRetentionObjectStorageStub) ListJournalSnapshotObjects(
	_ context.Context,
	prefix string,
	_ string,
	_ int,
) ([]JournalSnapshotObjectInfo, string, error) {
	return s.objects[prefix], "", nil
}

func TestJournalRetentionWorkerUsesFrozenCleanupOrderAndRecordsBacklog(t *testing.T) {
	now := time.UnixMilli(10_000_000_000)
	repo := &journalRetentionRepositoryStub{
		snapshotClaims: []JournalSnapshotCleanupClaim{{
			SpaceID: 10, SnapshotID: "snap-1", ClaimToken: "claim-1",
			ObjectKeys: []string{"journal-snapshots/staging/10/snap-1/content", "journal-snapshots/staging/10/snap-1/fragment-1"},
		}},
		stagingClaims: []JournalStagingCleanupClaim{{
			SpaceID: 10, SnapshotID: "staging-1", ClaimToken: "claim-staging-1", Prefix: "journal-snapshots/staging/10/staging-1/",
		}},
		backlog: 7,
	}
	storage := &journalRetentionObjectStorageStub{objects: map[string][]JournalSnapshotObjectInfo{
		"journal-snapshots/staging/10/staging-1/": {{Key: "journal-snapshots/staging/10/staging-1/orphan"}},
	}}
	registry := prometheus.NewRegistry()
	metrics, err := NewJournalPrometheusMetricsCollector(registry)
	require.NoError(t, err)
	worker := NewJournalRetentionWorker(repo, storage, JournalRetentionWorkerOptions{
		Now: func() time.Time { return now }, BatchSize: 100, Metrics: metrics,
	})

	result, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	require.Equal(t, JournalRetentionResult{
		SnapshotsDeleted: 1, StagingPrefixesDeleted: 1, CheckpointsDeleted: 2,
		EventsDeleted: 3, LedgersDeleted: 4, AttemptsDeleted: 1, Backlog: 7,
	}, result)
	require.Equal(t, []string{
		"claim_snapshots", "complete_snapshot:snap-1", "claim_staging",
		"complete_staging:staging-1", "delete_checkpoints",
		"delete_events_ledgers", "delete_attempts", "count_backlog",
	}, repo.steps)
	require.ElementsMatch(t, []string{
		"journal-snapshots/staging/10/snap-1/content", "journal-snapshots/staging/10/snap-1/fragment-1", "journal-snapshots/staging/10/staging-1/orphan",
	}, storage.deleted)
	require.Equal(t, float64(7), testutil.ToFloat64(metrics.retentionBacklog.With(
		prometheus.Labels((JournalMetricLabels{
			Version: "1.1", RolloutCohort: "treatment", TaskType: "unknown",
			ClientVersion: "unknown", Result: "current", ErrorCode: "none",
		}).prometheusLabels()),
	)))
}

func TestJournalRetentionWorkerKeepsMetadataWhenObjectDeletionFails(t *testing.T) {
	repo := &journalRetentionRepositoryStub{snapshotClaims: []JournalSnapshotCleanupClaim{{
		SpaceID: 10, SnapshotID: "snap-retry", ClaimToken: "claim-retry", ObjectKeys: []string{"journal-snapshots/staging/10/retry"},
	}}}
	storage := &journalRetentionObjectStorageStub{deleteErr: errors.New("storage unavailable")}
	worker := NewJournalRetentionWorker(repo, storage, JournalRetentionWorkerOptions{
		Now: func() time.Time { return time.UnixMilli(10_000_000_000) },
	})

	_, err := worker.RunOnce(context.Background())
	require.Error(t, err)
	require.Equal(t, []string{"object_delete_failed"}, repo.released)
	require.Equal(t, []string{"claim_snapshots", "release_snapshot:snap-retry", "count_backlog"}, repo.steps)
}

func TestJournalRetentionWorkerRejectsCrossTenantObjectKeys(t *testing.T) {
	repo := &journalRetentionRepositoryStub{snapshotClaims: []JournalSnapshotCleanupClaim{{
		SpaceID: 10, SnapshotID: "snap-cross-tenant", ClaimToken: "claim-cross-tenant",
		ObjectKeys: []string{"journal-snapshots/staging/11/foreign"},
	}}}
	storage := &journalRetentionObjectStorageStub{}
	worker := NewJournalRetentionWorker(repo, storage, JournalRetentionWorkerOptions{
		Now: func() time.Time { return time.UnixMilli(10_000_000_000) },
	})

	_, err := worker.RunOnce(context.Background())
	require.Error(t, err)
	require.Empty(t, storage.deleted)
	require.Equal(t, []string{"object_delete_failed"}, repo.released)
}

func TestJournalRetentionWorkerStopsAfterContextCancellation(t *testing.T) {
	repo := &journalRetentionRepositoryStub{}
	worker := NewJournalRetentionWorker(repo, &journalRetentionObjectStorageStub{}, JournalRetentionWorkerOptions{
		Interval: time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := worker.Start(ctx)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retention worker did not stop after context cancellation")
	}
}

func TestJournalRetentionWorkerFromEnvDisabledByDefault(t *testing.T) {
	t.Setenv(agentJournalRetentionWorkerEnabledEnv, "false")
	worker, status := StartJournalRetentionWorkerFromEnvWithStatus(
		context.Background(),
		&journalRetentionRepositoryStub{},
		&journalRetentionObjectStorageStub{},
		nil,
	)
	require.Nil(t, worker)
	require.False(t, status.Enabled)
	require.False(t, status.Started)
}
