// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	domainnotification "github.com/coze-dev/coze-studio/backend/domain/notification"
)

type fakeSystemAdministratorEmailProjection struct {
	emails []string
	err    error
}

func (f fakeSystemAdministratorEmailProjection) ListCanonicalEmails(
	context.Context,
) ([]string, error) {
	return append([]string(nil), f.emails...), f.err
}

type systemAdministratorUserTestPO struct {
	ID        int64      `gorm:"column:id;primaryKey"`
	Email     string     `gorm:"column:email"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
}

func (systemAdministratorUserTestPO) TableName() string {
	return "user"
}

type systemAdministratorNotificationIDGenerator struct {
	next int64
}

func (g *systemAdministratorNotificationIDGenerator) GenID(
	context.Context,
) (int64, error) {
	g.next++
	return g.next, nil
}

func (g *systemAdministratorNotificationIDGenerator) GenMultiIDs(
	ctx context.Context,
	count int,
) ([]int64, error) {
	ids := make([]int64, count)
	for index := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[index] = id
	}
	return ids, nil
}

func TestMySQLSystemAdminRecipientSourceUsesPersistedAdminEmailsAndLiveUsers(
	t *testing.T,
) {
	db, err := gorm.Open(sqlite.Open("file:system-admin-route?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&systemAdministratorUserTestPO{},
		&outboxPO{},
		&messagePO{},
		&recipientPO{},
		&notificationSequencePO{},
	); err != nil {
		t.Fatalf("migrate notification route fixtures: %v", err)
	}
	deletedAt := time.Date(2026, 7, 25, 0, 0, 0, 0, time.UTC)
	if err := db.Create([]systemAdministratorUserTestPO{
		{ID: 11, Email: "admin-a@example.test"},
		{ID: 12, Email: "ADMIN-B@example.test"},
		{ID: 13, Email: "member@example.test"},
		{ID: 14, Email: "deleted@example.test", DeletedAt: &deletedAt},
	}).Error; err != nil {
		t.Fatalf("seed users: %v", err)
	}
	source := &MySQLSystemAdminRecipientSource{
		db: db,
		emails: fakeSystemAdministratorEmailProjection{
			emails: []string{
				"admin-a@example.test",
				"admin-b@example.test",
				"deleted@example.test",
			},
		},
	}
	ids, err := source.ListSystemAdministratorUserIDs(context.Background())
	if err != nil {
		t.Fatalf("ListSystemAdministratorUserIDs() error = %v", err)
	}
	if len(ids) != 2 || ids[0] != 11 || ids[1] != 12 {
		t.Fatalf("system administrator recipients = %#v", ids)
	}

	now := time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)
	if err := db.Create(&notificationSequencePO{
		SequenceKey: "recipient_materialization",
		NextValue: 1,
		UpdatedAt: now.UnixMilli(),
	}).Error; err != nil {
		t.Fatalf("seed notification sequence: %v", err)
	}
	event, err := domainnotification.CanonicalizeEvent(domainnotification.Event{
		EventID: "sandbox-health:88:1:unhealthy",
		EventType: domainnotification.EventSystemProviderUnavailable,
		AggregateType: "sandbox_provider_health_incident",
		AggregateID: "sandbox-incident-88-1",
		AggregateVersion: 1,
		OccurredAt: now,
		ActorID: 0,
		SpaceID: 0,
		RecipientPolicy: domainnotification.RecipientSystemAdmins,
		PayloadSchema: domainnotification.CurrentPayloadSchema,
		Payload: domainnotification.EventPayload{
			ResourceDisplayName: "Sandbox provider 88",
			StatusReasonCode: domainnotification.StatusReasonProviderUnavailable,
		},
	})
	if err != nil {
		t.Fatalf("CanonicalizeEvent() error = %v", err)
	}
	repository := NewMySQLRepository(
		db,
		&systemAdministratorNotificationIDGenerator{next: 1000},
	)
	if err := repository.AppendInTransaction(context.Background(), db, event); err != nil {
		t.Fatalf("AppendInTransaction() error = %v", err)
	}
	claims, err := repository.ClaimOutboxBatch(
		context.Background(),
		"system-admin-route-worker",
		now.Add(time.Second),
		30*time.Second,
		1,
	)
	if err != nil {
		t.Fatalf("ClaimOutboxBatch() error = %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("ClaimOutboxBatch() claims = %#v", claims)
	}
	draft, err := domainnotification.DefaultTemplateRegistry().Render(event)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if err := repository.MaterializeAndDeliver(
		context.Background(),
		claims[0],
		draft,
		ids,
		now.Add(2*time.Second),
	); err != nil {
		t.Fatalf("MaterializeAndDeliver() error = %v", err)
	}
	var persisted []recipientPO
	if err := db.Order("user_id ASC").Find(&persisted).Error; err != nil {
		t.Fatalf("read persisted system administrator recipients: %v", err)
	}
	if len(persisted) != 2 ||
		persisted[0].UserID != 11 ||
		persisted[1].UserID != 12 {
		t.Fatalf("persisted system administrator recipients = %#v", persisted)
	}
}

func TestMySQLSystemAdminRecipientSourceRedactsProjectionFailure(
	t *testing.T,
) {
	db, err := gorm.Open(
		sqlite.Open("file:system-admin-invalid-route?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&systemAdministratorUserTestPO{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	if err := db.Create(&systemAdministratorUserTestPO{
		ID: 21,
		Email: "admin@example.test",
	}).Error; err != nil {
		t.Fatalf("seed administrator: %v", err)
	}
	const secret = "mysql://administrator:credential@private"
	source := &MySQLSystemAdminRecipientSource{
		db: db,
		emails: fakeSystemAdministratorEmailProjection{
			err: errors.New(secret),
		},
	}
	ids, err := source.ListSystemAdministratorUserIDs(context.Background())
	if !errors.Is(err, domainnotification.ErrRecipientResolution) ||
		len(ids) != 0 {
		t.Fatalf("projection failure resolved ids=%#v err=%v", ids, err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("recipient resolution error leaked projection failure")
	}
}

func TestMySQLSystemAdminRecipientSourceRetriesUntilUserAppears(
	t *testing.T,
) {
	db, err := gorm.Open(
		sqlite.Open("file:system-admin-route-later?mode=memory&cache=shared"),
		&gorm.Config{},
	)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&systemAdministratorUserTestPO{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	source := &MySQLSystemAdminRecipientSource{
		db: db,
		emails: fakeSystemAdministratorEmailProjection{
			emails: []string{"later@example.test"},
		},
	}

	ids, err := source.ListSystemAdministratorUserIDs(context.Background())
	if !errors.Is(err, domainnotification.ErrRecipientResolution) ||
		len(ids) != 0 {
		t.Fatalf("missing user resolved ids=%#v err=%v", ids, err)
	}
	var retryUntilResolved interface {
		RetryWithoutDeadLetter() bool
	}
	if !errors.As(err, &retryUntilResolved) ||
		!retryUntilResolved.RetryWithoutDeadLetter() {
		t.Fatal("missing bootstrap user must remain retryable without dead-letter")
	}

	if err := db.Create(&systemAdministratorUserTestPO{
		ID:    33,
		Email: "later@example.test",
	}).Error; err != nil {
		t.Fatalf("create later administrator: %v", err)
	}
	ids, err = source.ListSystemAdministratorUserIDs(context.Background())
	if err != nil {
		t.Fatalf("resolve later administrator: %v", err)
	}
	if len(ids) != 1 || ids[0] != 33 {
		t.Fatalf("later administrator recipients = %#v", ids)
	}
}
