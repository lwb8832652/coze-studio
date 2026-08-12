// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package appdev

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	applicationappdev "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const providerExecutionMySQLDDLConfirmation = "YES_I_OWN_THIS_DISPOSABLE_SCHEMA"

var providerExecutionMySQLDatabasePattern = regexp.MustCompile(`^coze_appdev_provider_exec_it_[a-z0-9_]+$`)

type providerExecutionMySQLGate struct{ config *mysqldriver.Config }

func loadProviderExecutionMySQLGate(getenv func(string) string) (providerExecutionMySQLGate, string, error) {
	dsn := strings.TrimSpace(getenv("APPDEV_PROVIDER_EXECUTION_MYSQL_DISPOSABLE_DSN"))
	if dsn == "" {
		return providerExecutionMySQLGate{}, "APPDEV_PROVIDER_EXECUTION_MYSQL_DISPOSABLE_DSN is not set", nil
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return providerExecutionMySQLGate{}, "", fmt.Errorf("invalid disposable MySQL DSN")
	}
	if !providerExecutionMySQLDatabasePattern.MatchString(config.DBName) {
		return providerExecutionMySQLGate{}, "", fmt.Errorf("database name is not an AppDev disposable integration schema")
	}
	if getenv("APPDEV_PROVIDER_EXECUTION_MYSQL_ALLOW_DDL") != providerExecutionMySQLDDLConfirmation {
		return providerExecutionMySQLGate{}, "", fmt.Errorf("explicit disposable-schema DDL confirmation is missing")
	}
	return providerExecutionMySQLGate{config: config}, "", nil
}

func TestProviderExecutionMySQLIntegrationCAS(t *testing.T) {
	gate, skipReason, err := loadProviderExecutionMySQLGate(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	if err != nil {
		t.Fatalf("unsafe MySQL integration configuration: %v", err)
	}
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{DSNConfig: gate.config}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open disposable MySQL schema failed")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal("access disposable MySQL pool failed")
	}
	sqlDB.SetMaxOpenConns(16)
	sqlDB.SetMaxIdleConns(16)
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatal("ping disposable MySQL schema failed")
	}
	var databaseName string
	if err := db.WithContext(ctx).Raw("SELECT DATABASE()").Row().Scan(&databaseName); err != nil || databaseName != gate.config.DBName || !providerExecutionMySQLDatabasePattern.MatchString(databaseName) {
		t.Fatal("connected MySQL schema did not satisfy the disposable gate")
	}
	cleanup := func() {
		_ = db.Session(&gorm.Session{Logger: logger.Discard}).Exec("DROP TABLE IF EXISTS appdev_provider_executions").Error
		_ = db.Session(&gorm.Session{Logger: logger.Discard}).Exec("DROP TABLE IF EXISTS appdev_projects").Error
	}
	cleanup()
	defer cleanup()
	if err := db.WithContext(ctx).Exec(`CREATE TABLE appdev_projects (
        id varchar(64) NOT NULL,
        space_id bigint unsigned NOT NULL,
        PRIMARY KEY (id),
        KEY idx_appdev_projects_space (space_id, id)
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`).Error; err != nil {
		t.Fatal("create disposable AppDev project fixture table failed")
	}
	for _, migrationPath := range []string{
		filepath.Join("..", "..", "..", "docker", "atlas", "migrations", "20260716000100_appdev_provider_executions.sql"),
		filepath.Join("..", "..", "..", "docker", "atlas", "migrations", "20260812000100_appdev_provider_execution_actor.sql"),
	} {
		migration, err := os.ReadFile(migrationPath)
		if err != nil {
			t.Fatal("read provider execution migration failed")
		}
		if err := db.WithContext(ctx).Exec(string(migration)).Error; err != nil {
			t.Fatal("apply provider execution migration to disposable schema failed")
		}
	}
	if err := db.WithContext(ctx).Exec("INSERT INTO appdev_projects (id, space_id) VALUES (?, ?)", "project-a", 1001).Error; err != nil {
		t.Fatal("seed disposable AppDev project failed")
	}

	repository := NewProviderExecutionRepository(db)
	now := time.Date(2026, 7, 16, 18, 0, 0, 123456000, time.UTC)
	const workers = 12
	sameResults := make(chan *domainappdev.ProviderExecution, workers)
	sameErrors := make(chan error, workers)
	runProviderExecutionBarrier(workers, func(index int) {
		input := providerExecutionStartInput(fmt.Sprintf("mysql-same-%02d", index), "mysql-same-idem", now)
		record, callErr := repository.EnsureStart(ctx, input)
		sameResults <- record
		sameErrors <- callErr
	})
	var first *domainappdev.ProviderExecution
	for range workers {
		if err := <-sameErrors; err != nil {
			t.Fatalf("concurrent MySQL idempotency: %v", err)
		}
		record := <-sameResults
		if first == nil {
			first = record
		}
		if record.ID != first.ID || record.Generation != first.Generation {
			t.Fatalf("MySQL idempotent records differ: %#v / %#v", first, record)
		}
	}
	var sameCount int64
	if err := db.WithContext(ctx).Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND idempotency_key = ?", 1001, "project-a", "mysql-same-idem").
		Count(&sameCount).Error; err != nil || sameCount != 1 {
		t.Fatalf("MySQL idempotency row count = %d, %v", sameCount, err)
	}
	for _, malformed := range []string{"1001suffix", " 1001", "1001 ", "1e3", "9223372036854775808"} {
		if _, err := repository.LoadCurrent(ctx, domainappdev.LoadCurrentProviderExecutionInput{
			SpaceID: malformed, ProjectID: "project-a",
		}); !errors.Is(err, domainappdev.ErrProviderExecutionInvalid) {
			t.Fatalf("MySQL LoadCurrent malformed space %q error = %v", malformed, err)
		}
	}

	generationResults := make(chan *domainappdev.ProviderExecution, workers)
	generationErrors := make(chan error, workers)
	runProviderExecutionBarrier(workers, func(index int) {
		input := providerExecutionStartInput(fmt.Sprintf("mysql-generation-%02d", index), fmt.Sprintf("mysql-generation-idem-%02d", index), now)
		record, callErr := repository.EnsureStart(ctx, input)
		generationResults <- record
		generationErrors <- callErr
	})
	generations := make([]int, 0, workers)
	for range workers {
		if err := <-generationErrors; err != nil {
			t.Fatalf("concurrent MySQL generation: %v", err)
		}
		generations = append(generations, int((<-generationResults).Generation))
	}
	sort.Ints(generations)
	for index, generation := range generations {
		if generation != index+2 {
			t.Fatalf("MySQL generations = %v", generations)
		}
	}

	type claimWinner struct {
		record *domainappdev.ProviderExecution
		hash   domainappdev.ProviderExecutionOwnerHash
	}
	var winnersMu sync.Mutex
	winners := make([]claimWinner, 0, 1)
	claimErrors := make(chan error, workers)
	runProviderExecutionBarrier(workers, func(index int) {
		hash := testOwnerHash(fmt.Sprintf("mysql-owner-%02d", index))
		claimed, claimErr := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
			SpaceID: first.SpaceID, ProjectID: first.ProjectID, Generation: first.Generation,
			ExpectedVersion: first.Version, OwnerHash: hash, LeaseDuration: time.Minute,
		})
		if claimErr == nil {
			winnersMu.Lock()
			winners = append(winners, claimWinner{record: claimed, hash: hash})
			winnersMu.Unlock()
		}
		claimErrors <- claimErr
	})
	conflicts := 0
	for range workers {
		if err := <-claimErrors; err != nil {
			if !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
				t.Fatalf("MySQL claim error = %v", err)
			}
			conflicts++
		}
	}
	if len(winners) != 1 || conflicts != workers-1 {
		t.Fatalf("MySQL claim winners/conflicts = %d/%d", len(winners), conflicts)
	}
	winner := winners[0]
	if err := db.WithContext(ctx).Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, "project-a", winner.record.Generation).
		UpdateColumn("owner_expires_at", gorm.Expr("TIMESTAMPADD(MICROSECOND, -1, UTC_TIMESTAMP(6))")).Error; err != nil {
		t.Fatal("expire owner with authoritative MySQL clock failed")
	}
	takeoverHash := testOwnerHash("mysql-takeover")
	taken, err := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: winner.record.SpaceID, ProjectID: winner.record.ProjectID, Generation: winner.record.Generation,
		ExpectedVersion: winner.record.Version, OwnerHash: takeoverHash, LeaseDuration: time.Minute,
	})
	if err != nil || taken.OwnerEpoch != winner.record.OwnerEpoch+1 {
		t.Fatalf("MySQL expired takeover = %#v, %v", taken, err)
	}
	staleEpoch := providerExecutionCAS(taken, takeoverHash, now)
	staleEpoch.OwnerEpoch--
	if _, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: staleEpoch}); !errors.Is(err, domainappdev.ErrProviderExecutionOwnerConflict) {
		t.Fatalf("MySQL stale epoch error = %v", err)
	}
	staleVersion := providerExecutionCAS(taken, takeoverHash, now)
	staleVersion.ExpectedVersion--
	if _, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{OwnerCAS: staleVersion}); !errors.Is(err, domainappdev.ErrProviderExecutionVersionConflict) {
		t.Fatalf("MySQL stale version error = %v", err)
	}

	operationHash := testOperationHash(t, "mysql-checkpoint-operation")
	reserveInput := domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: providerExecutionCAS(taken, takeoverHash, now), OperationHash: operationHash, ReservationDuration: time.Minute,
	}
	reservations := make(chan *domainappdev.ProviderExecutionCheckpointReservation, workers)
	reserveErrors := make(chan error, workers)
	runProviderExecutionBarrier(workers, func(_ int) {
		reservation, callErr := repository.ReserveCheckpointWrite(ctx, reserveInput)
		reservations <- reservation
		reserveErrors <- callErr
	})
	var reservation *domainappdev.ProviderExecutionCheckpointReservation
	for range workers {
		if err := <-reserveErrors; err != nil {
			t.Fatalf("concurrent MySQL checkpoint reservation: %v", err)
		}
		candidate := <-reservations
		if reservation == nil {
			reservation = candidate
		}
		if candidate.Revision != reservation.Revision || candidate.ReservedVersion != reservation.ReservedVersion {
			t.Fatalf("MySQL reservation mismatch: %#v / %#v", reservation, candidate)
		}
	}
	saveCAS := reserveInput.OwnerCAS
	saveCAS.ExpectedVersion = reservation.ReservedVersion
	saves := make(chan *domainappdev.ProviderExecution, workers)
	saveErrors := make(chan error, workers)
	runProviderExecutionBarrier(workers, func(_ int) {
		saved, callErr := repository.SaveCheckpoint(ctx, domainappdev.SaveProviderCheckpointInput{
			OwnerCAS: saveCAS, OperationHash: operationHash, CheckpointWriteRevision: reservation.Revision,
			CheckpointEnvelope: "ecp1:mysql-idempotent-envelope",
		})
		saves <- saved
		saveErrors <- callErr
	})
	var savedVersion uint64
	for range workers {
		if err := <-saveErrors; err != nil {
			t.Fatalf("concurrent MySQL checkpoint save: %v", err)
		}
		saved := <-saves
		if savedVersion == 0 {
			savedVersion = saved.Version
		}
		if saved.Version != savedVersion {
			t.Fatalf("MySQL checkpoint versions differ: %d/%d", savedVersion, saved.Version)
		}
	}

	service, err := applicationappdev.NewProviderExecutionService(
		repository, &providerExecutionLostResponseCodec{},
		applicationappdev.WithProviderExecutionClock(func() time.Time { return now.Add(10 * 365 * 24 * time.Hour) }),
		applicationappdev.WithProviderExecutionRandom(bytes.NewReader(bytes.Repeat([]byte{0x73}, 128))),
	)
	if err != nil {
		t.Fatal(err)
	}
	leaseStart, err := service.EnsureStart(ctx, applicationappdev.EnsureProviderExecutionStartRequest{
		SpaceID: "1001", ProjectID: "project-a", IdempotencyKey: "mysql-provider-lease-clock",
		ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaseClaim, err := service.ClaimRecovery(ctx, applicationappdev.ClaimProviderExecutionRecoveryRequest{
		SpaceID: leaseStart.SpaceID, ProjectID: leaseStart.ProjectID, Generation: leaseStart.Generation, ExpectedVersion: leaseStart.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaseSubmitting, err := service.StartProviderSubmission(
		ctx,
		providerExecutionServiceStartSubmissionRequest(
			leaseClaim.Owner(), leaseClaim.Metadata.Version, "mysql-provider-lease-launch",
		),
	)
	if err != nil {
		t.Fatal(err)
	}
	leaseSubmitting, err = service.MarkProviderLaunchSubmitted(ctx, applicationappdev.MarkProviderExecutionLaunchSubmittedRequest{
		Owner: leaseClaim.Owner(), ExpectedVersion: leaseSubmitting.Version, OperationID: "mysql-provider-lease-launch",
		DispatchLeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaseRunning, err := service.SaveProviderSubmission(ctx, applicationappdev.SaveProviderExecutionSubmissionRequest{
		Owner: leaseClaim.Owner(), ExpectedVersion: leaseSubmitting.Version,
		LaunchOperationID:   "mysql-provider-lease-launch",
		ProviderExecutionID: "mysql-provider-lease-execution", ObservedState: domainappdev.ProviderExecutionObservedRunning,
		ProviderLeaseDuration: 2 * time.Minute,
	})
	if err != nil || leaseRunning.ProviderLeaseExpiresAt == nil || leaseRunning.ProviderLeaseExpiresAt.Sub(leaseRunning.UpdatedAt) != 2*time.Minute {
		t.Fatalf("MySQL provider lease DB clock = %#v, %v", leaseRunning, err)
	}
	leaseComplete, err := service.SaveCheckpoint(ctx, applicationappdev.SaveProviderExecutionCheckpointRequest{
		Owner: leaseClaim.Owner(), ExpectedVersion: leaseRunning.Version,
		OperationID: "mysql-provider-lease-checkpoint", LaunchOperationID: "mysql-provider-lease-launch",
		ProviderKey: "provider-a", ProviderScope: domainsandbox.ScopeAppDev,
		Checkpoint: providerExecutionRouterCheckpoint(t),
	})
	if err != nil || leaseComplete.LaunchState != domainappdev.ProviderExecutionLaunchComplete {
		t.Fatalf("MySQL launch completion before cleanup = %#v, %v", leaseComplete, err)
	}
	leaseStopping, err := service.SetDesiredStop(ctx, applicationappdev.SetProviderExecutionDesiredStopRequest{
		Owner: leaseClaim.Owner(), ExpectedVersion: leaseComplete.Version,
	})
	if err != nil {
		t.Fatal(err)
	}
	leaseCleanup, err := service.BeginCleanup(ctx, applicationappdev.BeginProviderExecutionCleanupRequest{
		Owner: leaseClaim.Owner(), ExpectedVersion: leaseStopping.Version,
	})
	if err != nil || leaseCleanup.ObservedState != domainappdev.ProviderExecutionObservedCleanupPending {
		t.Fatalf("MySQL BeginCleanup = %#v, %v", leaseCleanup, err)
	}
	cleanupRequest := applicationappdev.CompleteProviderExecutionCleanupRequest{
		Owner: leaseClaim.Owner(), ExpectedVersion: leaseCleanup.Version, OperationID: "mysql-cleanup-operation",
	}
	firstCleanup, err := service.CompleteCleanup(ctx, cleanupRequest)
	if err != nil {
		t.Fatal(err)
	}
	retriedCleanup, err := service.CompleteCleanup(ctx, cleanupRequest)
	if err != nil || retriedCleanup.Version != firstCleanup.Version {
		t.Fatalf("MySQL cleanup lost-response retry = %#v, %v", retriedCleanup, err)
	}
	cleanupRequest.OperationID = "mysql-cleanup-other-operation"
	if _, err := service.CompleteCleanup(ctx, cleanupRequest); !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("MySQL cleanup different operation = %v", err)
	}

	terminal, err := repository.EnsureStart(ctx, providerExecutionStartInput("mysql-terminal-id", "mysql-terminal-idem", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	terminalHash := testOwnerHash("mysql-terminal-owner")
	terminal, err = repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: terminal.SpaceID, ProjectID: terminal.ProjectID, Generation: terminal.Generation,
		ExpectedVersion: terminal.Version, OwnerHash: terminalHash, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	terminalCAS := providerExecutionCAS(terminal, terminalHash, time.Time{})
	terminal, err = repository.StartSubmission(
		ctx,
		providerExecutionStartSubmissionInput(t, terminalCAS, "mysql-terminal-launch"),
	)
	if err != nil {
		t.Fatal(err)
	}
	terminal = markProviderExecutionSubmittedForTest(
		t, repository, terminal, terminalHash, "mysql-terminal-launch",
	)
	terminal, err = repository.SaveSubmission(ctx, providerExecutionSaveSubmissionInput(
		t, providerExecutionCAS(terminal, terminalHash, time.Time{}),
		"mysql-terminal-launch", "mysql-terminal-execution", time.Minute, "",
	))
	if err != nil {
		t.Fatal(err)
	}
	terminalInput := domainappdev.AdvanceProviderExecutionTerminalInput{
		OwnerCAS:      providerExecutionCAS(terminal, terminalHash, time.Time{}),
		OperationHash: testOperationHash(t, "mysql-terminal-operation"), ObservedState: domainappdev.ProviderExecutionObservedFailed,
	}
	terminal, err = repository.AdvanceTerminal(ctx, terminalInput)
	if err != nil {
		t.Fatal(err)
	}
	terminalRetry, err := repository.AdvanceTerminal(ctx, terminalInput)
	if err != nil || terminalRetry.Version != terminal.Version {
		t.Fatalf("MySQL terminal lost-response retry = %#v, %v", terminalRetry, err)
	}
	terminalWrongOperation := terminalInput
	terminalWrongOperation.OperationHash = testOperationHash(t, "mysql-terminal-other-operation")
	if _, err := repository.AdvanceTerminal(ctx, terminalWrongOperation); !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("MySQL terminal different operation = %v", err)
	}
	if err := db.WithContext(ctx).Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, "project-a", terminal.Generation).
		UpdateColumn("owner_expires_at", gorm.Expr("TIMESTAMPADD(MICROSECOND, -1, UTC_TIMESTAMP(6))")).Error; err != nil {
		t.Fatal(err)
	}
	recoverable, err := repository.ListRecoverable(ctx, domainappdev.ListRecoverableProviderExecutionsInput{SpaceID: "1001", ProjectID: "project-a", Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	foundTerminal := false
	for _, candidate := range recoverable {
		foundTerminal = foundTerminal || candidate.Generation == terminal.Generation
	}
	if !foundTerminal {
		t.Fatal("MySQL terminal execution was absent from recoverable list")
	}
	terminalTakeoverHash := testOwnerHash("mysql-terminal-takeover")
	terminalRecovered, err := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: terminal.SpaceID, ProjectID: terminal.ProjectID, Generation: terminal.Generation,
		ExpectedVersion: terminal.Version, OwnerHash: terminalTakeoverHash, LeaseDuration: time.Minute,
	})
	if err != nil || terminalRecovered.OwnerEpoch != terminal.OwnerEpoch+1 {
		t.Fatalf("MySQL terminal recovery claim = %#v, %v", terminalRecovered, err)
	}
	if _, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(terminal, terminalHash, time.Time{}),
	}); !errors.Is(err, domainappdev.ErrProviderExecutionConflict) {
		t.Fatalf("MySQL old terminal owner write = %v", err)
	}
	terminalStopping, err := repository.SetDesiredStop(ctx, domainappdev.SetProviderExecutionDesiredStopInput{
		OwnerCAS: providerExecutionCAS(terminalRecovered, terminalTakeoverHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	terminalCleanup, err := repository.BeginCleanup(ctx, domainappdev.BeginProviderExecutionCleanupInput{
		OwnerCAS: providerExecutionCAS(terminalStopping, terminalTakeoverHash, time.Time{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	terminalComplete, err := repository.CompleteCleanup(ctx, domainappdev.CompleteProviderExecutionCleanupInput{
		OwnerCAS:    providerExecutionCAS(terminalCleanup, terminalTakeoverHash, time.Time{}),
		OperationID: "mysql-terminal-cleanup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: terminalComplete.SpaceID, ProjectID: terminalComplete.ProjectID, Generation: terminalComplete.Generation,
		ExpectedVersion: terminalComplete.Version, OwnerHash: testOwnerHash("mysql-cleanup-complete-owner"), LeaseDuration: time.Minute,
	}); !errors.Is(err, domainappdev.ErrProviderExecutionStateConflict) {
		t.Fatalf("MySQL cleanup_complete claim = %v", err)
	}

	fenced, err := repository.EnsureStart(ctx, providerExecutionStartInput("mysql-fenced-id", "mysql-fenced-idem", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	fencedOwner := testOwnerHash("mysql-fenced-owner")
	fenced, err = repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: fenced.SpaceID, ProjectID: fenced.ProjectID, Generation: fenced.Generation,
		ExpectedVersion: fenced.Version, OwnerHash: fencedOwner, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	fencedCAS := providerExecutionCAS(fenced, fencedOwner, time.Time{})
	fencedOperation := testOperationHash(t, "mysql-fenced-checkpoint")
	fencedReservation, err := repository.ReserveCheckpointWrite(ctx, domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: fencedCAS, OperationHash: fencedOperation, ReservationDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&providerExecutionRecord{}).
		Where("space_id = ? AND project_id = ? AND generation = ?", 1001, "project-a", fenced.Generation).
		UpdateColumn("checkpoint_write_expires_at", gorm.Expr("TIMESTAMPADD(MICROSECOND, -1, UTC_TIMESTAMP(6))")).Error; err != nil {
		t.Fatal(err)
	}
	fencedSaveCAS := fencedCAS
	fencedSaveCAS.ExpectedVersion = fencedReservation.ReservedVersion
	if _, err := repository.SaveCheckpoint(ctx, domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: fencedSaveCAS, OperationHash: fencedOperation, CheckpointWriteRevision: fencedReservation.Revision,
		CheckpointEnvelope: "ecp1:mysql-expired-reservation",
	}); !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("MySQL expired reservation save = %v", err)
	}
	fencedRenewal, err := repository.ReserveCheckpointWrite(ctx, domainappdev.ReserveProviderCheckpointWriteInput{
		OwnerCAS: fencedCAS, OperationHash: fencedOperation, ReservationDuration: time.Minute,
	})
	if err != nil || fencedRenewal.Revision != fencedReservation.Revision+1 || fencedRenewal.ReservedVersion != fencedReservation.ReservedVersion+1 {
		t.Fatalf("MySQL reservation renewal = %#v, %v", fencedRenewal, err)
	}
	fencedSaveCAS.ExpectedVersion = fencedRenewal.ReservedVersion
	if _, err := repository.SaveCheckpoint(ctx, domainappdev.SaveProviderCheckpointInput{
		OwnerCAS: fencedSaveCAS, OperationHash: fencedOperation, CheckpointWriteRevision: fencedRenewal.Revision,
		CheckpointEnvelope: "ecp1:mysql-renewed-reservation",
	}); err != nil {
		t.Fatal(err)
	}

	releaseRecord, err := repository.EnsureStart(ctx, providerExecutionStartInput("mysql-release-id", "mysql-release-idem", time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	releaseOwner := testOwnerHash("mysql-release-owner")
	releaseRecord, err = repository.Claim(ctx, domainappdev.ClaimProviderExecutionInput{
		SpaceID: releaseRecord.SpaceID, ProjectID: releaseRecord.ProjectID, Generation: releaseRecord.Generation,
		ExpectedVersion: releaseRecord.Version, OwnerHash: releaseOwner, LeaseDuration: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	releaseHash, err := domainappdev.HashProviderExecutionReleaseOperationID("mysql-release-operation", releaseOwner)
	if err != nil {
		t.Fatal(err)
	}
	releaseInput := domainappdev.ReleaseProviderExecutionOwnerInput{
		OwnerCAS: providerExecutionCAS(releaseRecord, releaseOwner, time.Time{}), OperationHash: releaseHash,
	}
	released, err := repository.ReleaseOwner(ctx, releaseInput)
	if err != nil {
		t.Fatal(err)
	}
	releaseRetry, err := repository.ReleaseOwner(ctx, releaseInput)
	if err != nil || releaseRetry.Version != released.Version {
		t.Fatalf("MySQL release lost-response retry = %#v, %v", releaseRetry, err)
	}
	releaseInput.OperationHash, _ = domainappdev.HashProviderExecutionReleaseOperationID("mysql-release-other-operation", releaseOwner)
	if _, err := repository.ReleaseOwner(ctx, releaseInput); !errors.Is(err, domainappdev.ErrProviderExecutionOperationConflict) {
		t.Fatalf("MySQL release different operation = %v", err)
	}
}

func TestProviderExecutionMySQLIntegrationGateRejectsOrdinaryDatabase(t *testing.T) {
	environment := map[string]string{
		"APPDEV_PROVIDER_EXECUTION_MYSQL_DISPOSABLE_DSN": "user:secret@tcp(localhost:3306)/opencoze",
		"APPDEV_PROVIDER_EXECUTION_MYSQL_ALLOW_DDL":      providerExecutionMySQLDDLConfirmation,
	}
	_, _, err := loadProviderExecutionMySQLGate(func(key string) string { return environment[key] })
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("ordinary database gate error = %v", err)
	}
}
