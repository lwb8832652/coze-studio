// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package announcement

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	domainannouncement "github.com/coze-dev/coze-studio/backend/domain/announcement"
)

const (
	announcementMySQLTestDSNEnv     = "COZE_ANNOUNCEMENT_TEST_MYSQL_DSN"
	announcementMySQLTestDDLGateEnv = "COZE_ANNOUNCEMENT_TEST_ALLOW_DDL"
	announcementMySQLTestDDLGate    = "I_UNDERSTAND_DISPOSABLE_DB"
)

func TestAnnouncementMySQLDatabaseEpochIgnoresSessionTimezone(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(announcementMySQLTestDSNEnv))
	if dsn == "" ||
		os.Getenv(announcementMySQLTestDDLGateEnv) !=
			announcementMySQLTestDDLGate {
		t.Skip("requires an explicitly gated disposable MySQL database")
	}
	if !strings.Contains(strings.ToLower(dsn), "announcement_disposable") {
		t.Fatalf(
			"%s must select a database whose name contains announcement_disposable",
			announcementMySQLTestDSNEnv,
		)
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	var before int64
	var after int64
	var databaseEpoch int64
	err = db.Connection(func(connection *gorm.DB) (callbackErr error) {
		var originalTimezone string
		if scanErr := connection.Raw(
			"SELECT @@SESSION.time_zone",
		).Scan(&originalTimezone).Error; scanErr != nil {
			return scanErr
		}
		if setErr := connection.Exec(
			"SET SESSION time_zone = '+08:00'",
		).Error; setErr != nil {
			return setErr
		}
		defer func() {
			if restoreErr := connection.Exec(
				"SET SESSION time_zone = ?",
				originalTimezone,
			).Error; restoreErr != nil {
				callbackErr = fmt.Errorf(
					"restore MySQL session time_zone: %w",
					restoreErr,
				)
			}
		}()

		before = time.Now().UTC().UnixMilli()
		databaseEpoch, callbackErr = databaseUTCUnixMilli(
			context.Background(),
			connection,
		)
		after = time.Now().UTC().UnixMilli()
		return callbackErr
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, databaseEpoch, before-1000)
	require.LessOrEqual(t, databaseEpoch, after+1000)
}

func TestAnnouncementMySQLSnapshotExcludesConcurrentUncommittedLowID(
	t *testing.T,
) {
	dsn := strings.TrimSpace(os.Getenv(announcementMySQLTestDSNEnv))
	if dsn == "" ||
		os.Getenv(announcementMySQLTestDDLGateEnv) !=
			announcementMySQLTestDDLGate {
		t.Skip("requires an explicitly gated disposable MySQL database")
	}
	if !strings.Contains(strings.ToLower(dsn), "announcement_disposable") {
		t.Fatalf(
			"%s must select a database whose name contains announcement_disposable",
			announcementMySQLTestDSNEnv,
		)
	}
	dbA, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDBA, err := dbA.DB()
	require.NoError(t, err)
	sqlDBA.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDBA.Close() })
	tables := []any{
		&announcementOutboxRecord{},
		&deliveryBatchPO{},
		&recipientSnapshotPO{},
		&audienceTargetPO{},
		&auditEventPO{},
		&announcementPO{},
		&announcementSpaceUserPO{},
		&announcementSpacePO{},
		&announcementUserPO{},
	}
	require.NoError(t, dbA.Migrator().DropTable(tables...))
	require.NoError(t, dbA.AutoMigrate(tables...))
	t.Cleanup(func() { _ = dbA.Migrator().DropTable(tables...) })

	dbB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDBB, err := dbB.DB()
	require.NoError(t, err)
	sqlDBB.SetMaxOpenConns(2)
	t.Cleanup(func() { _ = sqlDBB.Close() })
	repository := NewMySQLRepository(
		dbA,
		&announcementTestIDGenerator{next: 40_000},
		WithNotificationOutbox(&recordingAnnouncementOutbox{}),
	)
	require.NoError(t, dbA.Create(&announcementUserPO{ID: 100}).Error)
	created := createAnnouncementForTest(
		t,
		repository,
		"create.mysql.snapshot.1",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	uncommitted := dbB.Begin()
	require.NoError(t, uncommitted.Error)
	defer uncommitted.Rollback()
	require.NoError(t, uncommitted.Create(&announcementUserPO{ID: 1}).Error)
	before := time.Now().UTC().UnixMilli()
	publishContext, cancel := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer cancel()

	result, err := repository.RequestPublish(
		publishContext,
		domainannouncement.PublishCommand{
			AnnouncementID: created.ID,
			ActorID:         42,
			ExpectedVersion: created.Version,
			IdempotencyKey:  "publish.mysql.snapshot.1",
			RequestHash:     strings.Repeat("b", 64),
			Now:             time.UnixMilli(1),
		},
	)

	require.NoError(t, err)
	after := time.Now().UTC().UnixMilli()
	require.NoError(t, uncommitted.Commit().Error)
	var recipientIDs []int64
	require.NoError(t, dbA.Model(&recipientSnapshotPO{}).
		Where("announcement_id = ?", created.ID).
		Order("user_id ASC").
		Pluck("user_id", &recipientIDs).Error)
	require.Equal(t, []int64{100}, recipientIDs)
	require.GreaterOrEqual(
		t,
		result.Announcement.SnapshotAt,
		before-1000,
	)
	require.LessOrEqual(t, result.Announcement.SnapshotAt, after+1000)
}

func TestAnnouncementMySQLMultipleRepositoriesProjectStableBatchOnce(
	t *testing.T,
) {
	dsn := strings.TrimSpace(os.Getenv(announcementMySQLTestDSNEnv))
	if dsn == "" ||
		os.Getenv(announcementMySQLTestDDLGateEnv) !=
			announcementMySQLTestDDLGate {
		t.Skip("requires an explicitly gated disposable MySQL database")
	}
	if !strings.Contains(strings.ToLower(dsn), "announcement_disposable") {
		t.Fatalf(
			"%s must select a database whose name contains announcement_disposable",
			announcementMySQLTestDSNEnv,
		)
	}

	dbA, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDBA, err := dbA.DB()
	require.NoError(t, err)
	sqlDBA.SetMaxOpenConns(4)
	t.Cleanup(func() {
		_ = sqlDBA.Close()
	})

	tables := []any{
		&announcementOutboxRecord{},
		&deliveryBatchPO{},
		&recipientSnapshotPO{},
		&audienceTargetPO{},
		&auditEventPO{},
		&announcementPO{},
		&announcementSpaceUserPO{},
		&announcementSpacePO{},
		&announcementUserPO{},
	}
	require.NoError(t, dbA.Migrator().DropTable(tables...))
	require.NoError(t, dbA.AutoMigrate(tables...))
	t.Cleanup(func() {
		_ = dbA.Migrator().DropTable(tables...)
	})

	dbB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDBB, err := dbB.DB()
	require.NoError(t, err)
	sqlDBB.SetMaxOpenConns(4)
	t.Cleanup(func() {
		_ = sqlDBB.Close()
	})

	idGenerator := &announcementTestIDGenerator{next: 20_000}
	outbox := &recordingAnnouncementOutbox{}
	repositoryA := NewMySQLRepository(
		dbA,
		idGenerator,
		WithNotificationOutbox(outbox),
	)
	repositoryB := NewMySQLRepository(
		dbB,
		idGenerator,
		WithNotificationOutbox(outbox),
	)
	require.NoError(t, dbA.Create(&announcementUserPO{ID: 1}).Error)

	created := createAnnouncementForTest(
		t,
		repositoryA,
		"create.mysql.multi.1",
		domainannouncement.Audience{Type: domainannouncement.AudienceAll},
	)
	publishAnnouncementForTest(t, repositoryA, created)
	_, err = repositoryA.AdvancePublication(
		context.Background(),
		created.ID,
		42,
		time.UnixMilli(1_800_000_000_100),
		500,
	)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, repository := range []*MySQLRepository{
		repositoryA,
		repositoryB,
	} {
		go func(current *MySQLRepository) {
			<-start
			_, advanceErr := current.AdvancePublication(
				context.Background(),
				created.ID,
				42,
				time.UnixMilli(1_800_000_000_200),
				500,
			)
			results <- advanceErr
		}(repository)
	}
	close(start)
	require.NoError(t, <-results)
	require.NoError(t, <-results)

	published, err := repositoryA.Get(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, domainannouncement.StatusPublished, published.Status)
	require.Equal(
		t,
		domainannouncement.ProjectionCompleted,
		published.ProjectionStatus,
	)
	require.Equal(t, int64(1), published.ProjectedCount)

	var outboxCount int64
	require.NoError(
		t,
		dbA.Model(&announcementOutboxRecord{}).Count(&outboxCount).Error,
	)
	require.Equal(t, int64(1), outboxCount)
}
