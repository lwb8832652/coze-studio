// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type mysqlHealthClaimLockContextKey struct{}

type mysqlHealthSelectLockContextKey struct{}

type mysqlHealthSelectLockBarrier struct {
	acquired chan struct{}
	release  chan struct{}
	once     sync.Once
}

func newMySQLHealthSelectLockBarrier() *mysqlHealthSelectLockBarrier {
	return &mysqlHealthSelectLockBarrier{
		acquired: make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (b *mysqlHealthSelectLockBarrier) hold(ctx context.Context) {
	if b == nil {
		return
	}
	close(b.acquired)
	select {
	case <-b.release:
	case <-ctx.Done():
	}
}

func (b *mysqlHealthSelectLockBarrier) unblock() {
	if b == nil {
		return
	}
	b.once.Do(func() {
		close(b.release)
	})
}

type mysqlHealthProjectionTestOutbox struct {
	mu       sync.Mutex
	fail     bool
	eventIDs []string
}

func (o *mysqlHealthProjectionTestOutbox) AppendInTransaction(
	_ context.Context,
	_ *gorm.DB,
	event domainnotification.Event,
) error {
	if o.fail {
		return errors.New("forced health projection append failure")
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.eventIDs = append(o.eventIDs, event.EventID)
	return nil
}

func (o *mysqlHealthProjectionTestOutbox) projectedEventIDs() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.eventIDs...)
}

func TestHealthMonitorMySQLMultiInstanceLeaseFencing(t *testing.T) {
	gate, skipReason, err := loadMySQLIntegrationGate(os.Getenv)
	if skipReason != "" {
		t.Skip(skipReason)
	}
	if err != nil {
		t.Fatalf("unsafe MySQL integration configuration: %v", err)
	}
	openPool := func(label string, maxOpen int) (*gorm.DB, *sql.DB) {
		db, openErr := gorm.Open(
			gormmysql.New(gormmysql.Config{DSNConfig: gate.config}),
			&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)},
		)
		if openErr != nil {
			t.Fatalf("open sandbox disposable MySQL pool %s: %v", label, openErr)
		}
		sqlDB, sqlErr := db.DB()
		if sqlErr != nil {
			t.Fatalf("get MySQL connection pool %s: %v", label, sqlErr)
		}
		sqlDB.SetMaxOpenConns(maxOpen)
		sqlDB.SetMaxIdleConns(maxOpen)
		t.Cleanup(func() {
			if closeErr := sqlDB.Close(); closeErr != nil {
				t.Errorf("close MySQL integration pool %s: %v", label, closeErr)
			}
		})
		return db, sqlDB
	}
	dbA, sqlA := openPool("a", 4)
	dbB, sqlB := openPool("b", 1)
	if dbA == dbB || sqlA == sqlB {
		t.Fatal("health monitor integration instances share a database pool")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for label, sqlDB := range map[string]*sql.DB{"a": sqlA, "b": sqlB} {
		if err := sqlDB.PingContext(ctx); err != nil {
			t.Fatalf("ping sandbox disposable MySQL pool %s: %v", label, err)
		}
	}
	for label, db := range map[string]*gorm.DB{"a": dbA, "b": dbB} {
		var guard string
		if err := db.WithContext(ctx).Raw(
			"SELECT guard_value FROM sandbox_mysql_integration_guard WHERE guard_key = ?",
			mysqlIntegrationGuardKey,
		).Row().Scan(&guard); err != nil || guard != gate.guard {
			t.Fatalf("exclusive MySQL integration guard mismatch on pool %s", label)
		}
	}
	for _, table := range []string{
		"sandbox_provider_health_episodes",
		"sandbox_health_notification_projections",
	} {
		if !dbA.Migrator().HasTable(table) {
			t.Fatalf("required health monitor table %q is missing", table)
		}
	}

	key := "health-mysql-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	providerRepository := NewMySQLRepository(dbA)
	provider := createEnabledHealthMonitorProvider(
		t,
		ctx,
		providerRepository,
		key,
	)
	t.Cleanup(func() {
		_ = dbA.Where("provider_id = ?", provider.ID).
			Delete(&providerHealthNotificationProjectionPO{}).Error
		_ = dbA.Where("provider_id = ?", provider.ID).
			Delete(&providerHealthEpisodePO{}).Error
		_ = dbA.Unscoped().Where("id = ?", provider.ID).
			Delete(&providerPO{}).Error
	})

	repositoryA := NewMySQLHealthMonitorRepository(dbA, nil)
	repositoryB := NewMySQLHealthMonitorRepository(dbB, nil)
	databaseNow := verifyMySQLHealthUTCClock(t, ctx, dbB)
	episode := providerHealthEpisodePO{
		ProviderID: uint64(provider.ID),
		IncidentStatus: string(domainsandbox.HealthIncidentStatusNone),
		Version: domainsandbox.InitialVersion,
		CreatedAt: databaseNow.UnixMilli(),
		UpdatedAt: databaseNow.UnixMilli(),
	}
	if err := dbA.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&episode).Error; err != nil {
		t.Fatalf("seed MySQL health episode: %v", err)
	}

	lockAcquired := make(chan struct{})
	releaseClaim := make(chan struct{})
	var lockOnce sync.Once
	var releaseOnce sync.Once
	releaseHolder := func() {
		releaseOnce.Do(func() {
			close(releaseClaim)
		})
	}
	defer releaseHolder()
	callbackName := "sandbox:mysql_health_claim_lock_" + key
	if err := dbA.Callback().Update().After("gorm:update").
		Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Table != episode.TableName() ||
				tx.Statement.Context.Value(mysqlHealthClaimLockContextKey{}) != true {
				return
			}
			lockOnce.Do(func() {
				close(lockAcquired)
				select {
				case <-releaseClaim:
				case <-ctx.Done():
				}
			})
		}); err != nil {
		t.Fatalf("register MySQL health claim barrier: %v", err)
	}
	defer func() {
		if removeErr := dbA.Callback().Update().Remove(callbackName); removeErr != nil {
			t.Errorf("remove MySQL health claim barrier: %v", removeErr)
		}
	}()

	request := domainsandbox.HealthMonitorClaimRequest{
		WorkerID: "mysql-health-a",
		Now: databaseNow,
		Lease: 10 * time.Second,
		Limit: 1,
	}
	type claimResult struct {
		claims []domainsandbox.HealthMonitorClaim
		err error
	}
	firstClaimDone := make(chan claimResult, 1)
	go func() {
		claims, claimErr := repositoryA.ClaimHealthChecks(
			context.WithValue(ctx, mysqlHealthClaimLockContextKey{}, true),
			request,
		)
		firstClaimDone <- claimResult{claims: claims, err: claimErr}
	}()
	select {
	case <-lockAcquired:
	case early := <-firstClaimDone:
		releaseHolder()
		t.Fatalf(
			"first MySQL claim returned before holding its update lock: %#v, %v",
			early.claims,
			early.err,
		)
	case <-ctx.Done():
		releaseHolder()
		t.Fatalf("wait for first MySQL claim lock: %v", ctx.Err())
	}

	var blockedClaims []domainsandbox.HealthMonitorClaim
	var blockedErr error
	connectionErr := dbB.WithContext(ctx).Connection(func(pinned *gorm.DB) error {
		return withMySQLSessionLockWaitTimeout(t, pinned, 1, func() error {
			candidate := request
			candidate.WorkerID = "mysql-health-b-blocked"
			blockedClaims, blockedErr =
				NewMySQLHealthMonitorRepository(pinned, nil).
					ClaimHealthChecks(ctx, candidate)
			return blockedErr
		})
	})
	if blockedErr == nil {
		blockedErr = connectionErr
	}
	var mysqlError *mysqldriver.MySQLError
	if len(blockedClaims) != 0 ||
		!errors.As(blockedErr, &mysqlError) ||
		mysqlError.Number != 1205 {
		releaseHolder()
		t.Fatalf(
			"second MySQL instance crossed held health lease lock: claims=%#v error=%v",
			blockedClaims,
			blockedErr,
		)
	}
	select {
	case early := <-firstClaimDone:
		releaseHolder()
		t.Fatalf(
			"first MySQL claim left barrier before release: %#v, %v",
			early.claims,
			early.err,
		)
	default:
	}
	releaseHolder()
	firstResult := <-firstClaimDone
	if firstResult.err != nil || len(firstResult.claims) != 1 {
		t.Fatalf(
			"first MySQL claim after release = %#v, %v",
			firstResult.claims,
			firstResult.err,
		)
	}
	first := firstResult.claims[0]
	competingRequest := request
	competingRequest.WorkerID = "mysql-health-b-after-release"
	competingRequest.Now = time.Now().UTC()
	competing, err := repositoryB.ClaimHealthChecks(ctx, competingRequest)
	if err != nil || len(competing) != 0 {
		t.Fatalf("second MySQL claim after committed lease = %#v, %v", competing, err)
	}

	expiredAt, err := repositoryA.currentDatabaseTime(ctx, dbA, time.Time{})
	if err != nil {
		t.Fatalf("read expiry clock: %v", err)
	}
	if err := dbA.Model(&providerHealthEpisodePO{}).
		Where("provider_id = ?", provider.ID).
		Updates(map[string]any{
			"lease_expires_at": expiredAt.UnixMilli(),
			"next_check_at": 0,
		}).Error; err != nil {
		t.Fatalf("expire first MySQL lease: %v", err)
	}
	takeoverClaims, err := repositoryB.ClaimHealthChecks(
		ctx,
		domainsandbox.HealthMonitorClaimRequest{
			WorkerID: "mysql-health-takeover",
			Now: expiredAt,
			Lease: 10 * time.Second,
			Limit: 1,
		},
	)
	if err != nil || len(takeoverClaims) != 1 {
		t.Fatalf("MySQL lease takeover = %#v, %v", takeoverClaims, err)
	}
	_, err = repositoryA.CompleteHealthCheck(
		ctx,
		domainsandbox.CompleteHealthMonitorCheckInput{
			Claim: first,
			Health: healthyHealthSnapshot(expiredAt),
			CheckedAt: expiredAt,
			NextCheckAt: expiredAt.Add(time.Minute),
			FailureThreshold: 3,
		},
	)
	if !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("stale-after-reclaim completion error = %v", err)
	}

	takeover := takeoverClaims[0]
	completion := verifyMySQLHealthCompletionSelectLocks(
		t,
		ctx,
		dbA,
		dbB,
		repositoryA,
		takeover,
		expiredAt.Add(time.Millisecond),
	)
	if !completion.Applied {
		t.Fatalf("takeover MySQL completion = %#v", completion)
	}
	_, err = repositoryA.CompleteHealthCheck(
		ctx,
		domainsandbox.CompleteHealthMonitorCheckInput{
			Claim: takeover,
			Health: healthyHealthSnapshot(expiredAt.Add(2*time.Millisecond)),
			CheckedAt: expiredAt.Add(2*time.Millisecond),
			NextCheckAt: expiredAt.Add(time.Minute),
			FailureThreshold: 3,
		},
	)
	if !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("duplicate MySQL completion error = %v", err)
	}

	boundaryNow, err := repositoryA.currentDatabaseTime(ctx, dbA, time.Time{})
	if err != nil {
		t.Fatalf("read boundary clock: %v", err)
	}
	if err := dbA.Model(&providerHealthEpisodePO{}).
		Where("provider_id = ?", provider.ID).
		Updates(map[string]any{
			"next_check_at": 0,
			"lease_owner": "",
			"lease_token": "",
			"lease_expires_at": 0,
		}).Error; err != nil {
		t.Fatalf("reset boundary lease: %v", err)
	}
	boundaryClaims, err := repositoryA.ClaimHealthChecks(
		ctx,
		domainsandbox.HealthMonitorClaimRequest{
			WorkerID: "mysql-health-boundary",
			Now: boundaryNow,
			Lease: 10 * time.Second,
			Limit: 1,
		},
	)
	if err != nil || len(boundaryClaims) != 1 {
		t.Fatalf("MySQL boundary claim = %#v, %v", boundaryClaims, err)
	}
	boundaryNow, err = repositoryA.currentDatabaseTime(ctx, dbA, time.Time{})
	if err != nil {
		t.Fatalf("refresh boundary clock: %v", err)
	}
	if err := dbA.Model(&providerHealthEpisodePO{}).
		Where("provider_id = ?", provider.ID).
		Update("lease_expires_at", boundaryNow.UnixMilli()).Error; err != nil {
		t.Fatalf("set lease exhaustion boundary: %v", err)
	}
	_, err = repositoryA.CompleteHealthCheck(
		ctx,
		domainsandbox.CompleteHealthMonitorCheckInput{
			Claim: boundaryClaims[0],
			Health: healthyHealthSnapshot(boundaryNow),
			CheckedAt: boundaryNow,
			NextCheckAt: boundaryNow.Add(time.Minute),
			FailureThreshold: 3,
		},
	)
	if !errors.Is(err, domainsandbox.ErrVersionConflict) {
		t.Fatalf("lease exhaustion boundary error = %v", err)
	}

	verifyMySQLHealthProjectionCausalAntiJoin(
		t,
		ctx,
		dbA,
		dbB,
		provider.ID,
	)
}

func verifyMySQLHealthUTCClock(
	t *testing.T,
	ctx context.Context,
	db *gorm.DB,
) time.Time {
	t.Helper()
	var databaseNow time.Time
	var originalSessionTimeZone string
	var sessionTimeZone string
	var windowStart time.Time
	var windowEnd time.Time
	err := db.WithContext(ctx).Connection(func(
		pinned *gorm.DB,
	) (connectionErr error) {
		if err := pinned.WithContext(ctx).
			Raw("SELECT @@session.time_zone").
			Row().
			Scan(&originalSessionTimeZone); err != nil {
			return err
		}
		if err := pinned.WithContext(ctx).
			Exec("SET SESSION time_zone = '+08:00'").Error; err != nil {
			return err
		}
		defer func() {
			restoreErr := pinned.WithContext(ctx).
				Exec(
					"SET SESSION time_zone = ?",
					originalSessionTimeZone,
				).Error
			if restoreErr == nil {
				var restored string
				restoreErr = pinned.WithContext(ctx).
					Raw("SELECT @@session.time_zone").
					Row().
					Scan(&restored)
				if restoreErr == nil &&
					restored != originalSessionTimeZone {
					restoreErr = fmt.Errorf(
						"MySQL session timezone restored to %q, want %q",
						restored,
						originalSessionTimeZone,
					)
				}
			}
			connectionErr = errors.Join(connectionErr, restoreErr)
		}()
		if err := pinned.WithContext(ctx).
			Raw("SELECT @@session.time_zone").
			Row().
			Scan(&sessionTimeZone); err != nil {
			return err
		}
		windowStart = time.Now().UTC()
		var clockErr error
		databaseNow, clockErr =
			NewMySQLHealthMonitorRepository(pinned, nil).
				currentDatabaseTime(ctx, pinned, time.Time{})
		windowEnd = time.Now().UTC()
		return clockErr
	})
	if err != nil {
		t.Fatalf("read MySQL UTC health monitor clock: %v", err)
	}
	if sessionTimeZone != "+08:00" {
		t.Fatalf("MySQL health clock session timezone = %q", sessionTimeZone)
	}
	if databaseNow.IsZero() ||
		databaseNow.Location() != time.UTC ||
		databaseNow.Before(windowStart.Add(-2*time.Second)) ||
		databaseNow.After(windowEnd.Add(2*time.Second)) {
		t.Fatalf(
			"MySQL UTC health clock %v is outside real UTC window [%v, %v]",
			databaseNow,
			windowStart,
			windowEnd,
		)
	}
	return databaseNow
}

func verifyMySQLHealthCompletionSelectLocks(
	t *testing.T,
	parent context.Context,
	dbA *gorm.DB,
	dbB *gorm.DB,
	repository *MySQLHealthMonitorRepository,
	claim domainsandbox.HealthMonitorClaim,
	checkedAt time.Time,
) domainsandbox.HealthMonitorCheckCompletion {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	episodeBarrier := newMySQLHealthSelectLockBarrier()
	providerBarrier := newMySQLHealthSelectLockBarrier()
	defer episodeBarrier.unblock()
	defer providerBarrier.unblock()
	callbackName := "sandbox:mysql_health_completion_select_lock:" +
		strconv.FormatInt(claim.Provider.ID, 10)
	if err := dbA.Callback().Query().After("gorm:query").
		Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Context.Value(mysqlHealthSelectLockContextKey{}) !=
				"completion" ||
				!strings.Contains(
					strings.ToUpper(tx.Statement.SQL.String()),
					"FOR UPDATE",
				) {
				return
			}
			switch tx.Statement.Table {
			case (providerHealthEpisodePO{}).TableName():
				episodeBarrier.hold(ctx)
			case (providerPO{}).TableName():
				providerBarrier.hold(ctx)
			}
		}); err != nil {
		t.Fatalf("register MySQL completion SELECT barrier: %v", err)
	}
	defer func() {
		if removeErr := dbA.Callback().Query().
			Remove(callbackName); removeErr != nil {
			t.Errorf("remove MySQL completion SELECT barrier: %v", removeErr)
		}
	}()

	type completionResult struct {
		completion domainsandbox.HealthMonitorCheckCompletion
		err        error
	}
	done := make(chan completionResult, 1)
	go func() {
		completion, err := repository.CompleteHealthCheck(
			context.WithValue(
				ctx,
				mysqlHealthSelectLockContextKey{},
				"completion",
			),
			domainsandbox.CompleteHealthMonitorCheckInput{
				Claim: claim,
				Health: healthyHealthSnapshot(checkedAt),
				CheckedAt: checkedAt,
				NextCheckAt: checkedAt.Add(time.Minute),
				FailureThreshold: 3,
			},
		)
		done <- completionResult{completion: completion, err: err}
	}()
	select {
	case <-episodeBarrier.acquired:
	case early := <-done:
		t.Fatalf(
			"completion passed episode SELECT without FOR UPDATE: %#v, %v",
			early.completion,
			early.err,
		)
	case <-ctx.Done():
		t.Fatalf("wait for completion episode SELECT lock: %v", ctx.Err())
	}
	assertMySQLHealthRowLockWaitTimeout(
		t,
		ctx,
		dbB,
		func(tx *gorm.DB) error {
			var row providerHealthEpisodePO
			return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("provider_id = ?", claim.Provider.ID).
				Take(&row).Error
		},
	)
	episodeBarrier.unblock()

	select {
	case <-providerBarrier.acquired:
	case early := <-done:
		t.Fatalf(
			"completion passed provider SELECT without FOR UPDATE: %#v, %v",
			early.completion,
			early.err,
		)
	case <-ctx.Done():
		t.Fatalf("wait for completion provider SELECT lock: %v", ctx.Err())
	}
	assertMySQLHealthRowLockWaitTimeout(
		t,
		ctx,
		dbB,
		func(tx *gorm.DB) error {
			var row providerPO
			return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ?", claim.Provider.ID).
				Take(&row).Error
		},
	)
	providerBarrier.unblock()

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("completion after SELECT lock release: %v", result.err)
		}
		return result.completion
	case <-ctx.Done():
		t.Fatalf("wait for completion after SELECT lock release: %v", ctx.Err())
		return domainsandbox.HealthMonitorCheckCompletion{}
	}
}

func assertMySQLHealthRowLockWaitTimeout(
	t *testing.T,
	ctx context.Context,
	db *gorm.DB,
	lock func(*gorm.DB) error,
) {
	t.Helper()
	err := db.WithContext(ctx).Connection(func(pinned *gorm.DB) error {
		return withMySQLSessionLockWaitTimeout(t, pinned, 1, func() error {
			return pinned.WithContext(ctx).Transaction(lock)
		})
	})
	var mysqlError *mysqldriver.MySQLError
	if !errors.As(err, &mysqlError) || mysqlError.Number != 1205 {
		t.Fatalf("competing MySQL SELECT lock error = %v, want typed 1205", err)
	}
}

func verifyMySQLHealthProjectionCausalAntiJoin(
	t *testing.T,
	ctx context.Context,
	dbA *gorm.DB,
	dbB *gorm.DB,
	providerID int64,
) {
	t.Helper()
	clockRepository := NewMySQLHealthMonitorRepository(dbA, nil)
	now, err := clockRepository.currentDatabaseTime(ctx, dbA, time.Time{})
	if err != nil {
		t.Fatalf("read MySQL projection clock: %v", err)
	}
	const incidentSequence = uint64(9001)
	incidentID := "sandbox-incident-" +
		strconv.FormatInt(providerID, 10) +
		"-" +
		strconv.FormatUint(incidentSequence, 10)
	unhealthyEventID := "sandbox-health:" +
		strconv.FormatInt(providerID, 10) +
		":" +
		strconv.FormatUint(incidentSequence, 10) +
		":unhealthy"
	recoveredEventID := "sandbox-health:" +
		strconv.FormatInt(providerID, 10) +
		":" +
		strconv.FormatUint(incidentSequence, 10) +
		":recovered"
	rows := []providerHealthNotificationProjectionPO{
		{
			EventID: unhealthyEventID,
			ProviderID: uint64(providerID),
			IncidentID: incidentID,
			IncidentSequence: incidentSequence,
			NotificationType: string(
				domainsandbox.HealthIncidentNotificationUnhealthy,
			),
			OccurredAt: now.UnixMilli(),
			Status: healthProjectionStatusPending,
			NextAttemptAt: now.UnixMilli(),
			Version: domainsandbox.InitialVersion,
			CreatedAt: now.UnixMilli(),
			UpdatedAt: now.UnixMilli(),
		},
		{
			EventID: recoveredEventID,
			ProviderID: uint64(providerID),
			IncidentID: incidentID,
			IncidentSequence: incidentSequence,
			NotificationType: string(
				domainsandbox.HealthIncidentNotificationRecovered,
			),
			OccurredAt: now.Add(time.Millisecond).UnixMilli(),
			Status: healthProjectionStatusPending,
			NextAttemptAt: now.UnixMilli(),
			Version: domainsandbox.InitialVersion,
			CreatedAt: now.UnixMilli(),
			UpdatedAt: now.UnixMilli(),
		},
	}
	if err := dbA.WithContext(ctx).Create(&rows).Error; err != nil {
		t.Fatalf("seed MySQL causal health projections: %v", err)
	}
	readProjection := func(eventID string) providerHealthNotificationProjectionPO {
		var row providerHealthNotificationProjectionPO
		if err := dbA.WithContext(ctx).
			Where("event_id = ?", eventID).
			Take(&row).Error; err != nil {
			t.Fatalf("read MySQL health projection %q: %v", eventID, err)
		}
		return row
	}
	request := domainsandbox.HealthNotificationProjectionRequest{
		WorkerID: "mysql-health-projector",
		Now: time.Now().UTC(),
		Lease: 10 * time.Second,
		Limit: 2,
	}
	failingOutbox := &mysqlHealthProjectionTestOutbox{fail: true}
	failingRepository := NewMySQLHealthMonitorRepository(dbA, failingOutbox)
	result, err := failingRepository.ProjectPendingHealthNotifications(ctx, request)
	if !errors.Is(err, domainsandbox.ErrHealthMonitorOutbox) ||
		result.Claimed != 1 ||
		result.Projected != 0 {
		t.Fatalf("MySQL pending unhealthy projection = %#v, %v", result, err)
	}
	unhealthy := readProjection(unhealthyEventID)
	recovered := readProjection(recoveredEventID)
	if unhealthy.Status != healthProjectionStatusPending ||
		unhealthy.AttemptCount != 1 ||
		unhealthy.LeaseToken != "" ||
		recovered.Status != healthProjectionStatusPending ||
		recovered.AttemptCount != 0 ||
		recovered.LeaseToken != "" {
		t.Fatalf(
			"MySQL failed unhealthy projection state unhealthy=%#v recovered=%#v",
			unhealthy,
			recovered,
		)
	}

	now, err = clockRepository.currentDatabaseTime(ctx, dbA, time.Time{})
	if err != nil {
		t.Fatalf("refresh MySQL projection clock: %v", err)
	}
	if err := dbA.WithContext(ctx).
		Model(&providerHealthNotificationProjectionPO{}).
		Where("event_id = ?", unhealthyEventID).
		Updates(map[string]any{
			"next_attempt_at": now.Add(time.Minute).UnixMilli(),
			"lease_owner": "",
			"lease_token": "",
			"lease_expires_at": 0,
		}).Error; err != nil {
		t.Fatalf("set MySQL unhealthy projection backoff: %v", err)
	}
	goodOutbox := &mysqlHealthProjectionTestOutbox{}
	goodRepository := NewMySQLHealthMonitorRepository(dbA, goodOutbox)
	request.Now = time.Now().UTC()
	result, err = goodRepository.ProjectPendingHealthNotifications(ctx, request)
	if err != nil || result.Claimed != 0 || result.Projected != 0 {
		t.Fatalf("MySQL backoff anti-join projection = %#v, %v", result, err)
	}

	now, err = clockRepository.currentDatabaseTime(ctx, dbA, time.Time{})
	if err != nil {
		t.Fatalf("refresh MySQL leased projection clock: %v", err)
	}
	if err := dbA.WithContext(ctx).
		Model(&providerHealthNotificationProjectionPO{}).
		Where("event_id = ?", unhealthyEventID).
		Updates(map[string]any{
			"next_attempt_at": 0,
			"lease_owner": "held-projector",
			"lease_token": "held-projection-token",
			"lease_expires_at": now.Add(time.Minute).UnixMilli(),
		}).Error; err != nil {
		t.Fatalf("lease MySQL unhealthy projection: %v", err)
	}
	request.Now = time.Now().UTC()
	result, err = goodRepository.ProjectPendingHealthNotifications(ctx, request)
	if err != nil || result.Claimed != 0 || result.Projected != 0 {
		t.Fatalf("MySQL leased anti-join projection = %#v, %v", result, err)
	}

	if err := dbA.WithContext(ctx).
		Model(&providerHealthNotificationProjectionPO{}).
		Where("event_id = ?", unhealthyEventID).
		Update("lease_expires_at", 0).Error; err != nil {
		t.Fatalf("expire MySQL unhealthy projection lease: %v", err)
	}
	request.Now = time.Now().UTC()
	result, err = projectMySQLHealthNotificationWithSelectLockBarrier(
		t,
		ctx,
		dbA,
		dbB,
		goodRepository,
		request,
		unhealthyEventID,
	)
	if err != nil || result.Claimed != 1 || result.Projected != 1 {
		t.Fatalf("MySQL unhealthy projection after lease = %#v, %v", result, err)
	}
	unhealthy = readProjection(unhealthyEventID)
	recovered = readProjection(recoveredEventID)
	if unhealthy.Status != healthProjectionStatusProjected ||
		recovered.Status != healthProjectionStatusPending ||
		recovered.LeaseToken != "" {
		t.Fatalf(
			"MySQL projected unhealthy gate state unhealthy=%#v recovered=%#v",
			unhealthy,
			recovered,
		)
	}

	request.Now = time.Now().UTC()
	result, err = goodRepository.ProjectPendingHealthNotifications(ctx, request)
	if err != nil || result.Claimed != 1 || result.Projected != 1 {
		t.Fatalf("MySQL recovered projection after unhealthy = %#v, %v", result, err)
	}
	recovered = readProjection(recoveredEventID)
	if recovered.Status != healthProjectionStatusProjected {
		t.Fatalf("MySQL recovered projection state = %#v", recovered)
	}
	eventIDs := goodOutbox.projectedEventIDs()
	if len(eventIDs) != 2 ||
		eventIDs[0] != unhealthyEventID ||
		eventIDs[1] != recoveredEventID {
		t.Fatalf("MySQL causal projection order = %#v", eventIDs)
	}
}

func projectMySQLHealthNotificationWithSelectLockBarrier(
	t *testing.T,
	parent context.Context,
	dbA *gorm.DB,
	dbB *gorm.DB,
	repository *MySQLHealthMonitorRepository,
	request domainsandbox.HealthNotificationProjectionRequest,
	eventID string,
) (domainsandbox.HealthNotificationProjectionResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	barrier := newMySQLHealthSelectLockBarrier()
	defer barrier.unblock()
	callbackName := "sandbox:mysql_health_projection_select_lock:" + eventID
	if err := dbA.Callback().Query().After("gorm:query").
		Register(callbackName, func(tx *gorm.DB) {
			if tx.Statement.Context.Value(mysqlHealthSelectLockContextKey{}) !=
				"projection" ||
				tx.Statement.Table !=
					(providerHealthNotificationProjectionPO{}).TableName() ||
				!strings.Contains(
					strings.ToUpper(tx.Statement.SQL.String()),
					"FOR UPDATE",
				) {
				return
			}
			barrier.hold(ctx)
		}); err != nil {
		t.Fatalf("register MySQL projection SELECT barrier: %v", err)
	}
	defer func() {
		if removeErr := dbA.Callback().Query().
			Remove(callbackName); removeErr != nil {
			t.Errorf("remove MySQL projection SELECT barrier: %v", removeErr)
		}
	}()

	type projectionResult struct {
		result domainsandbox.HealthNotificationProjectionResult
		err    error
	}
	done := make(chan projectionResult, 1)
	go func() {
		result, err := repository.ProjectPendingHealthNotifications(
			context.WithValue(
				ctx,
				mysqlHealthSelectLockContextKey{},
				"projection",
			),
			request,
		)
		done <- projectionResult{result: result, err: err}
	}()
	select {
	case <-barrier.acquired:
	case early := <-done:
		t.Fatalf(
			"projection passed row SELECT without FOR UPDATE: %#v, %v",
			early.result,
			early.err,
		)
	case <-ctx.Done():
		t.Fatalf("wait for projection SELECT lock: %v", ctx.Err())
	}
	assertMySQLHealthRowLockWaitTimeout(
		t,
		ctx,
		dbB,
		func(tx *gorm.DB) error {
			var row providerHealthNotificationProjectionPO
			return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("event_id = ?", eventID).
				Take(&row).Error
		},
	)
	barrier.unblock()
	select {
	case result := <-done:
		return result.result, result.err
	case <-ctx.Done():
		t.Fatalf("wait for projection after SELECT lock release: %v", ctx.Err())
		return domainsandbox.HealthNotificationProjectionResult{}, ctx.Err()
	}
}
