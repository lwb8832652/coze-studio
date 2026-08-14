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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestJournalMySQLIntegrationGaplessSequenceAcrossConnections(t *testing.T) {
	db, repoA, repoB := journalMySQLIntegrationRepositories(t)
	require.NoError(t, repoA.CreateThread(context.Background(), &entity.Thread{
		ID: 1, SpaceID: 10, CreatorID: 20, Title: "journal mysql",
		Status: entity.ThreadStatusIdle, Source: entity.ThreadSourceWeb,
		CreatedAt: 1, UpdatedAt: 1,
	}))
	run := newRepositoryTestRun(10, 1, entity.RunStatusRunning, 1)
	run.SpaceID = 10
	run.CreatorID = 20
	run.RunKind = entity.RunKindTask
	activeSlot := uint8(1)
	_, err := repoA.CreateRunBundle(context.Background(), CreateRunBundleRequest{
		Run: run,
		Attempt: &entity.RunAttempt{
			ID: 100, ThreadID: 1, JournalRunID: 10, ExecutionRunID: 10,
			AttemptID: "att_100", Ordinal: 1, Status: entity.RunAttemptStatusRunning,
			ActiveSlot: &activeSlot, NextSequence: 1, EnrollmentVersion: entity.JournalSchemaVersion,
			ProjectionState: entity.JournalProjectionStateHealthy,
		},
	})
	require.NoError(t, err)

	const eventCount = 50
	sequences := make([]uint64, eventCount)
	errs := make(chan error, eventCount)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < eventCount; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			repo := repoA
			if i%2 == 1 {
				repo = repoB
			}
			event, err := repo.AppendJournalEvent(context.Background(), &entity.JournalEvent{
				ID: int64(1000 + i), ThreadID: 1, RunID: 10,
				IdempotencyKey: fmt.Sprintf("mysql-event-%d", i),
				EventType:      "message.delta", Payload: journalTestPayload,
			})
			if err == nil {
				sequences[i] = event.Sequence
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })
	for i, sequence := range sequences {
		require.Equal(t, uint64(i+1), sequence)
	}
	var attempt runAttemptPO
	require.NoError(t, db.Where("id = ?", 100).First(&attempt).Error)
	require.Equal(t, uint64(eventCount+1), attempt.NextSequence)
	require.Zero(t, attempt.LastCommittedSequence)
}

func journalMySQLIntegrationRepositories(
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
	sqlDBA.SetMaxOpenConns(8)
	sqlDBB.SetMaxOpenConns(8)

	models := []any{&runAttemptPO{}, &runEventPO{}, &runPO{}, &threadPO{}}
	t.Cleanup(func() {
		_ = dbA.Migrator().DropTable(models...)
		_ = sqlDBB.Close()
		_ = sqlDBA.Close()
	})
	require.NoError(t, dbA.Migrator().DropTable(models...))
	require.NoError(t, createJournalMySQLIntegrationSchema(dbA))
	return dbA, &threadRepository{db: dbA}, &threadRepository{db: dbB}
}

func createJournalMySQLIntegrationSchema(db *gorm.DB) error {
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
		"docker/atlas/migrations/20260614000300_agent_run_events.sql",
		"docker/atlas/migrations/20260730000100_agent_run_attempts.sql",
		"docker/atlas/migrations/20260730000200_agent_run_events_journal_columns.sql",
		"docker/atlas/migrations/20260730000210_agent_run_events_journal_indexes.sql",
		"docker/atlas/migrations/20260730000220_agent_run_events_journal_projection.sql",
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
