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

package repository

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const (
	canonicalMySQLTestDSNEnv     = "COZE_AGENTTHREAD_TEST_MYSQL_DSN"
	canonicalMySQLTestDDLGateEnv = "COZE_AGENTTHREAD_TEST_ALLOW_DDL"
	canonicalMySQLTestDDLGate    = "I_UNDERSTAND_DISPOSABLE_DB"
)

func TestCanonicalMySQLIntegrationMetadataNumberSemantics(t *testing.T) {
	_, repo, _ := canonicalMySQLIntegrationRepositories(t)
	threads := []*entity.Thread{
		{
			ID: 1, SpaceID: 10, CreatorID: 20, Title: "integer",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			Metadata:  `{"n":1,"precise":0.123456789012345,"rounded":0.123456789012345678901234567890,"huge":9223372036854775808,"uint_boundary":18446744073709551615,"beyond":18446744073709551616,"normalized":1.2300,"zero":-0.0}`,
			CreatedAt: 100, UpdatedAt: 100,
		},
		{
			ID: 2, SpaceID: 10, CreatorID: 20, Title: "decimal",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			Metadata:  `{"n":1.0,"precise":0.123456789012346,"rounded":0.123456789012345678901234567891,"huge":9223372036854775809,"uint_boundary":18446744073709551615.0,"beyond":18446744073709551617,"normalized":2.34,"zero":0.0}`,
			CreatedAt: 200, UpdatedAt: 200,
		},
	}
	for _, thread := range threads {
		require.NoError(t, repo.CreateThread(context.Background(), thread))
	}

	exact, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10, Metadata: map[string]any{"n": json.Number("1.0")},
		SortBy: "thread_id", SortOrder: "asc", Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []int64{2}, threadIDs(exact))

	precise, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10,
		Metadata: map[string]any{
			"precise": json.Number("0.123456789012345"),
		},
		Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []int64{1}, threadIDs(precise))
	rounded, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10,
		Metadata: map[string]any{
			"rounded": json.Number("0.123456789012345678901234567890"),
		},
		SortBy: "thread_id", SortOrder: "asc", Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []int64{1, 2}, threadIDs(rounded))

	huge, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10,
		Metadata: map[string]any{
			"huge": json.Number("9223372036854775808"),
		},
		Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []int64{1}, threadIDs(huge))

	beyond, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10,
		Metadata: map[string]any{
			"beyond": json.Number("18446744073709551616"),
		},
		SortBy: "thread_id", SortOrder: "asc", Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []int64{1, 2}, threadIDs(beyond))

	uintBoundary, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10,
		Metadata: map[string]any{
			"uint_boundary": json.Number("18446744073709551615"),
		},
		Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []int64{1}, threadIDs(uintBoundary))

	doubleBoundary, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10,
		Metadata: map[string]any{
			"uint_boundary": json.Number("18446744073709551615.0"),
		},
		Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []int64{2}, threadIDs(doubleBoundary))

	normalized, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10,
		Metadata: map[string]any{
			"normalized": json.Number("1.23"),
		},
		Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, []int64{1}, threadIDs(normalized))

	zero, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
		SpaceID: 10, Metadata: map[string]any{"zero": json.Number("-0.0")},
		SortBy: "thread_id", SortOrder: "asc", Page: CanonicalPage{Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Equal(t, []int64{1, 2}, threadIDs(zero))

	for _, underflow := range []json.Number{"1e-400", "-1e-400"} {
		matches, total, err := repo.SearchThreads(context.Background(), SearchThreadsRequest{
			SpaceID: 10, Metadata: map[string]any{"zero": underflow},
			SortBy: "thread_id", SortOrder: "asc", Page: CanonicalPage{Limit: 10},
		})
		require.NoError(t, err)
		require.Equal(t, int64(2), total)
		require.Equal(t, []int64{1, 2}, threadIDs(matches))
	}
}

func TestCanonicalMySQLIntegrationConcurrentPatchPreservesBothKeys(t *testing.T) {
	_, repoA, repoB := canonicalMySQLIntegrationRepositories(t)
	require.NoError(t, repoA.CreateThread(context.Background(), &entity.Thread{
		ID: 10, SpaceID: 20, CreatorID: 30, Title: "patch",
		Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
		Metadata: `{"base":true}`, CreatedAt: 100, UpdatedAt: 100,
	}))

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for index, item := range []struct {
		repo *threadRepository
		key  string
	}{{repo: repoA, key: "first"}, {repo: repoB, key: "second"}} {
		wg.Add(1)
		go func(index int, item struct {
			repo *threadRepository
			key  string
		}) {
			defer wg.Done()
			<-start
			_, err := item.repo.PatchThread(context.Background(), PatchThreadRequest{
				ThreadID: 10, MetadataPatch: map[string]any{item.key: true},
				UpdatedAt: int64(200 + index),
			})
			errs <- err
		}(index, item)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	patched, err := repoA.GetThread(context.Background(), 10)
	require.NoError(t, err)
	require.JSONEq(t, `{"base":true,"first":true,"second":true}`, patched.Metadata)
}

func TestCanonicalMySQLIntegrationConcurrentPublicStatePreservesBothKeys(t *testing.T) {
	_, repoA, repoB := canonicalMySQLIntegrationRepositories(t)
	require.NoError(t, repoA.CreateThread(context.Background(), &entity.Thread{
		ID: 10, SpaceID: 20, CreatorID: 30, Title: "public state",
		Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
		CreatedAt: 100, UpdatedAt: 100,
	}))
	require.NoError(t, repoA.CreateRun(context.Background(),
		newCanonicalRepositoryRun(20, 10, 0, 100)))

	start := make(chan struct{})
	errs := make(chan error, 2)
	requests := []struct {
		repo      *threadRepository
		id        int64
		createdAt int64
		values    string
	}{
		{repo: repoA, id: 101, createdAt: 700, values: `{"custom":{"first":true}}`},
		{repo: repoB, id: 102, createdAt: 600, values: `{"custom":{"second":true}}`},
	}
	var wg sync.WaitGroup
	for _, req := range requests {
		wg.Add(1)
		go func(req struct {
			repo      *threadRepository
			id        int64
			createdAt int64
			values    string
		}) {
			defer wg.Done()
			<-start
			_, err := req.repo.UpdatePublicThreadState(
				context.Background(),
				UpdatePublicThreadStateRequest{
					CheckpointID: req.id, ThreadID: 10,
					ChannelValues: req.values,
					Metadata:      `{"source":"canonical_public_state"}`,
					CreatedAt:     req.createdAt,
				},
			)
			errs <- err
		}(req)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	latest, total, err := repoA.ListCheckpoints(context.Background(), ListCheckpointsRequest{
		ThreadID: 10, RuntimeType: "canonical_public_state", Limit: 1,
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, latest, 1)
	require.JSONEq(t, `{"custom":{"first":true,"second":true}}`, latest[0].ChannelValues)
}

func TestCanonicalMySQLIntegrationGuardedRunAndDeleteAreLinearizable(t *testing.T) {
	db, repoA, repoB := canonicalMySQLIntegrationRepositories(t)
	for iteration := 0; iteration < 12; iteration++ {
		threadID := int64(1_000 + iteration)
		runID := int64(2_000 + iteration)
		require.NoError(t, repoA.CreateThread(context.Background(), &entity.Thread{
			ID: threadID, SpaceID: 20, CreatorID: 30, Title: "run-delete race",
			Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
			CreatedAt: 100, UpdatedAt: 100,
		}))
		run := newCanonicalRepositoryRun(runID, threadID, 0, 100)
		run.Status = entity.RunStatusPending

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		start := make(chan struct{})
		createResult := make(chan error, 1)
		type deleteOutcome struct {
			deleted bool
			err     error
		}
		deleteResult := make(chan deleteOutcome, 1)
		go func() {
			<-start
			createResult <- repoA.CreateRunWithThreadLock(ctx, run)
		}()
		go func() {
			<-start
			deleted, err := repoB.DeleteThreadIfIdle(ctx, DeleteThreadIfIdleRequest{
				ThreadID: threadID,
			})
			deleteResult <- deleteOutcome{deleted: deleted, err: err}
		}()
		close(start)
		createErr := <-createResult
		deleted := <-deleteResult
		cancel()

		var threadCount int64
		var runCount int64
		require.NoError(t, db.Model(&threadPO{}).Where("id = ?", threadID).Count(&threadCount).Error)
		require.NoError(t, db.Model(&runPO{}).Where("thread_id = ?", threadID).Count(&runCount).Error)
		switch {
		case createErr == nil:
			require.False(t, deleted.deleted)
			require.ErrorIs(t, deleted.err, ErrActiveRunExists)
			require.Equal(t, int64(1), threadCount)
			require.Equal(t, int64(1), runCount)
			require.NoError(t, db.Model(&runPO{}).Where("id = ?", runID).
				Update("status", string(entity.RunStatusSucceeded)).Error)
			cleaned, err := repoA.DeleteThreadIfIdle(context.Background(), DeleteThreadIfIdleRequest{
				ThreadID: threadID,
			})
			require.NoError(t, err)
			require.True(t, cleaned)
		case deleted.err == nil && deleted.deleted:
			require.ErrorIs(t, createErr, gorm.ErrRecordNotFound)
			require.Zero(t, threadCount)
			require.Zero(t, runCount)
		default:
			t.Fatalf(
				"iteration %d produced non-linearizable outcome: create=%v delete=(%t,%v)",
				iteration,
				createErr,
				deleted.deleted,
				deleted.err,
			)
		}
	}
}

func canonicalMySQLIntegrationRepositories(
	t *testing.T,
) (*gorm.DB, *threadRepository, *threadRepository) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(canonicalMySQLTestDSNEnv))
	if dsn == "" || os.Getenv(canonicalMySQLTestDDLGateEnv) != canonicalMySQLTestDDLGate {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("CI")), "true") {
			t.Fatalf(
				"CI requires %s and %s=%s",
				canonicalMySQLTestDSNEnv,
				canonicalMySQLTestDDLGateEnv,
				canonicalMySQLTestDDLGate,
			)
		}
		t.Skip("requires an explicitly gated disposable MySQL database")
	}
	config, err := mysqldriver.ParseDSN(dsn)
	require.NoError(t, err)
	if !strings.Contains(strings.ToLower(config.DBName), "agentthread_disposable") {
		t.Fatalf("%s must select a database whose name contains agentthread_disposable", canonicalMySQLTestDSNEnv)
	}

	dbA, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	dbB, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDBA, err := dbA.DB()
	require.NoError(t, err)
	sqlDBB, err := dbB.DB()
	require.NoError(t, err)
	sqlDBA.SetMaxOpenConns(4)
	sqlDBB.SetMaxOpenConns(4)

	models := canonicalMySQLIntegrationModels()
	t.Cleanup(func() {
		_ = dbA.Migrator().DropTable(models...)
		_ = sqlDBB.Close()
		_ = sqlDBA.Close()
	})
	require.NoError(t, dbA.Migrator().DropTable(models...))
	require.NoError(t, createCanonicalMySQLIntegrationSchema(dbA))
	return dbA, &threadRepository{db: dbA}, &threadRepository{db: dbB}
}

func createCanonicalMySQLIntegrationSchema(db *gorm.DB) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return gorm.ErrInvalidData
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../../.."))
	migrations := []string{
		"docker/atlas/migrations/20260613000100_agent_threads.sql",
		"docker/atlas/migrations/20260614000200_agent_runs.sql",
		"docker/atlas/migrations/20260621000100_agent_run_children.sql",
		"docker/atlas/migrations/20260711000100_agent_run_leases.sql",
		"docker/atlas/migrations/20260617000100_agent_checkpoints.sql",
		"docker/atlas/migrations/20260619000100_agent_checkpoint_runtime_keys.sql",
	}
	for _, migration := range migrations {
		contents, err := os.ReadFile(filepath.Join(repositoryRoot, migration))
		if err != nil {
			return err
		}
		if err := db.Exec(string(contents)).Error; err != nil {
			return err
		}
	}
	for _, statement := range canonicalMySQLCascadeTableDDL() {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func canonicalMySQLCascadeTableDDL() []string {
	return []string{
		"CREATE TABLE `agent_thread_messages` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_run_events` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_thread_memories` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_memory_audit_events` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_transcript_snapshots` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_memory_flush_jobs` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_token_usage` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_files` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_artifacts` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_artifact_scan_jobs` (`id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_run_plans` (`run_id` BIGINT NOT NULL PRIMARY KEY, `thread_id` BIGINT NOT NULL) ENGINE=InnoDB",
		"CREATE TABLE `agent_run_plan_items` (`id` BIGINT NOT NULL PRIMARY KEY, `run_id` BIGINT NOT NULL) ENGINE=InnoDB",
	}
}

func canonicalMySQLIntegrationModels() []any {
	return []any{
		&threadPO{},
		&messagePO{},
		&runPO{},
		&runEventPO{},
		&checkpointPO{},
		&memoryPO{},
		&memoryAuditEventPO{},
		&transcriptSnapshotPO{},
		&memoryFlushJobPO{},
		&tokenUsagePO{},
		&agentFilePO{},
		&agentArtifactPO{},
		&agentArtifactScanJobPO{},
		&agentRunPlanPO{},
		&agentRunPlanItemPO{},
	}
}
