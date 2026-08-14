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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestHumanResumeRolloverMySQLSameKeyReplay(t *testing.T) {
	db, repoA, repoB := humanResumeMySQLIntegrationRepositories(t)
	seedHumanResumeRolloverSource(t, db)

	type outcome struct {
		result *CreateRunBundleResult
		err    error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	create := func(repo *threadRepository, request CreateRunBundleRequest) {
		<-start
		result, err := repo.CreateRunBundle(ctx, request)
		outcomes <- outcome{result: result, err: err}
	}
	go create(repoA, humanResumeRolloverRequest("human-resume-concurrent", 60, 601, 602, 603, 604))
	go create(repoB, humanResumeRolloverRequest("human-resume-concurrent", 70, 701, 702, 703, 704))
	close(start)
	first, second := <-outcomes, <-outcomes

	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.NotNil(t, first.result)
	require.NotNil(t, second.result)
	require.Equal(t, first.result.Run.ID, second.result.Run.ID)
	require.Equal(t, first.result.Message.ID, second.result.Message.ID)
	require.Equal(t, first.result.Event.ID, second.result.Event.ID)
	require.Equal(t, first.result.Attempt.ID, second.result.Attempt.ID)
	require.NotEqual(t, first.result.Created, second.result.Created)
	assertHumanResumeMySQLSingleAggregate(t, db, "human-resume-concurrent")
}

func TestHumanResumeRolloverMySQLSameKeyDriftConflict(t *testing.T) {
	db, repoA, repoB := humanResumeMySQLIntegrationRepositories(t)
	seedHumanResumeRolloverSource(t, db)
	created, err := repoA.CreateRunBundle(context.Background(),
		humanResumeRolloverRequest("human-resume-drift", 60, 601, 602, 603, 604))
	require.NoError(t, err)
	require.True(t, created.Created)

	drifted := humanResumeRolloverRequest("human-resume-drift", 80, 801, 802, 803, 804)
	drifted.Run.Command = strings.Replace(drifted.Run.Command, `"decision":"approved"`, `"decision":"rejected"`, 1)
	drifted.Run.Metadata = strings.Replace(drifted.Run.Metadata, `"decision":"approved"`, `"decision":"rejected"`, 1)
	drifted.Message.Content = "已拒绝执行"
	drifted.Message.Metadata = strings.Replace(drifted.Message.Metadata, `"decision":"approved"`, `"decision":"rejected"`, 1)
	drifted.Event.Payload = strings.Replace(drifted.Event.Payload, `"decision":"approved"`, `"decision":"rejected"`, 1)
	_, err = repoB.CreateRunBundle(context.Background(), drifted)
	require.ErrorIs(t, err, ErrRunIdempotencyConflict)
	assertHumanResumeTargetAbsent(t, db, 80, 801, 802, 803, 804)
	assertHumanResumeMySQLSingleAggregate(t, db, "human-resume-drift")
}

func TestHumanResumeRolloverMySQLDifferentKeySingleWinner(t *testing.T) {
	db, repoA, repoB := humanResumeMySQLIntegrationRepositories(t)
	seedHumanResumeRolloverSource(t, db)

	type outcome struct {
		request CreateRunBundleRequest
		result  *CreateRunBundleResult
		err     error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	outcomes := make(chan outcome, 2)
	create := func(repo *threadRepository, request CreateRunBundleRequest) {
		<-start
		result, err := repo.CreateRunBundle(ctx, request)
		outcomes <- outcome{request: request, result: result, err: err}
	}
	go create(repoA, humanResumeRolloverRequest("human-resume-first", 60, 601, 602, 603, 604))
	go create(repoB, humanResumeRolloverRequest("human-resume-second", 70, 701, 702, 703, 704))
	close(start)
	results := []outcome{<-outcomes, <-outcomes}

	winners := 0
	losers := 0
	for _, result := range results {
		if result.err == nil {
			winners++
			require.NotNil(t, result.result)
			require.True(t, result.result.Created)
			continue
		}
		losers++
		require.ErrorIs(t, result.err, ErrHumanResumeRolloverConflict)
		assertHumanResumeTargetAbsent(t, db,
			result.request.Run.ID, result.request.Message.ID, result.request.Event.ID,
			result.request.HumanResumeRollover.TerminalBase.ID, result.request.Attempt.ID)
	}
	require.Equal(t, 1, winners)
	require.Equal(t, 1, losers)

	var source runAttemptPO
	require.NoError(t, db.Where("id = ?", 500).First(&source).Error)
	require.Equal(t, string(entity.RunAttemptStatusInterrupted), source.Status)
	require.Nil(t, source.ActiveSlot)
	require.NotNil(t, source.TerminalEventID)
	var targetAttempts int64
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("journal_run_id = ? AND ordinal = ?", 40, 2).Count(&targetAttempts).Error)
	require.Equal(t, int64(1), targetAttempts)
	var targetRuns int64
	require.NoError(t, db.Model(&runPO{}).
		Where("id IN ?", []int64{60, 70}).Count(&targetRuns).Error)
	require.Equal(t, int64(1), targetRuns)
	var targetMessages int64
	require.NoError(t, db.Model(&messagePO{}).
		Where("id IN ?", []int64{601, 701}).Count(&targetMessages).Error)
	require.Equal(t, int64(1), targetMessages)
	var targetEvents int64
	require.NoError(t, db.Model(&runEventPO{}).
		Where("id IN ?", []int64{602, 603, 702, 703}).Count(&targetEvents).Error)
	require.Equal(t, int64(2), targetEvents)
}

func assertHumanResumeMySQLSingleAggregate(t *testing.T, db *gorm.DB, key string) {
	t.Helper()
	var runs int64
	require.NoError(t, db.Model(&runPO{}).
		Where("space_id = ? AND idempotency_key = ?", 10, key).Count(&runs).Error)
	require.Equal(t, int64(1), runs)
	var messages int64
	require.NoError(t, db.Model(&messagePO{}).
		Where("run_id IN ?", []int64{60, 70}).Count(&messages).Error)
	require.Equal(t, int64(1), messages)
	var attempts int64
	require.NoError(t, db.Model(&runAttemptPO{}).
		Where("execution_run_id IN ?", []int64{60, 70}).Count(&attempts).Error)
	require.Equal(t, int64(1), attempts)
	var events int64
	require.NoError(t, db.Model(&runEventPO{}).
		Where("id IN ?", []int64{602, 603, 702, 703}).Count(&events).Error)
	require.Equal(t, int64(2), events)
}

func humanResumeMySQLIntegrationRepositories(
	t *testing.T,
) (*gorm.DB, *threadRepository, *threadRepository) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(canonicalMySQLTestDSNEnv))
	if dsn == "" || os.Getenv(canonicalMySQLTestDDLGateEnv) != canonicalMySQLTestDDLGate {
		if strings.EqualFold(strings.TrimSpace(os.Getenv("CI")), "true") {
			t.Fatalf("CI requires %s and %s=%s", canonicalMySQLTestDSNEnv,
				canonicalMySQLTestDDLGateEnv, canonicalMySQLTestDDLGate)
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

	models := []any{&runAttemptPO{}, &runEventPO{}, &checkpointPO{}, &messagePO{}, &runPO{}, &threadPO{}}
	t.Cleanup(func() {
		_ = dbA.Migrator().DropTable(models...)
		_ = sqlDBB.Close()
		_ = sqlDBA.Close()
	})
	require.NoError(t, dbA.Migrator().DropTable(models...))
	require.NoError(t, createHumanResumeMySQLIntegrationSchema(dbA))
	return dbA, &threadRepository{db: dbA}, &threadRepository{db: dbB}
}

func createHumanResumeMySQLIntegrationSchema(db *gorm.DB) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return gorm.ErrInvalidData
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../../.."))
	migrations := []string{
		"docker/atlas/migrations/20260613000100_agent_threads.sql",
		"docker/atlas/migrations/20260614000100_agent_thread_messages.sql",
		"docker/atlas/migrations/20260614000200_agent_runs.sql",
		"docker/atlas/migrations/20260621000100_agent_run_children.sql",
		"docker/atlas/migrations/20260711000100_agent_run_leases.sql",
		"docker/atlas/migrations/20260614000300_agent_run_events.sql",
		"docker/atlas/migrations/20260617000100_agent_checkpoints.sql",
		"docker/atlas/migrations/20260619000100_agent_checkpoint_runtime_keys.sql",
		"docker/atlas/migrations/20260730000100_agent_run_attempts.sql",
		"docker/atlas/migrations/20260730000200_agent_run_events_journal_columns.sql",
		"docker/atlas/migrations/20260730000210_agent_run_events_journal_indexes.sql",
		"docker/atlas/migrations/20260730000220_agent_run_events_journal_projection.sql",
		"docker/atlas/migrations/20260813000100_agent_run_attempts_interrupted.sql",
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
	return nil
}
