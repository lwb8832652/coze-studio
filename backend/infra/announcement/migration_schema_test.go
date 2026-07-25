// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package announcement

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdminAnnouncementMigrationCreatesFinalReleaseBarrierSchema(
	t *testing.T,
) {
	migrationDirectory := filepath.Join(
		"..",
		"..",
		"..",
		"docker",
		"atlas",
		"migrations",
	)
	migrationPath := filepath.Join(
		migrationDirectory,
		"20260725000300_admin_announcements.sql",
	)
	contents, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	schema := string(contents)

	for _, fragment := range []string{
		"`snapshot_at` BIGINT NOT NULL DEFAULT 0",
		"`snapshot_complete` BOOL NOT NULL DEFAULT FALSE",
		"`idx_announcements_release_projection`",
		"`batch_no` BIGINT UNSIGNED NOT NULL DEFAULT 0",
		"`idx_announcement_snapshot_batch`",
		"CREATE TABLE `announcement_delivery_batches`",
		"`uk_announcement_delivery_event`",
		"`idx_announcement_delivery_claim`",
	} {
		require.Contains(t, schema, fragment)
	}
	require.NotContains(t, schema, "user_high_water_id")
	require.NotContains(t, schema, "membership_high_water_id")
	require.NotContains(t, schema, "ALTER TABLE")

	_, err = os.Stat(filepath.Join(
		migrationDirectory,
		"20260725000400_announcement_release_barrier.sql",
	))
	require.ErrorIs(t, err, os.ErrNotExist)

	require.Equal(t, "announcements", (announcementPO{}).TableName())
	require.Equal(
		t,
		"announcement_recipient_snapshots",
		(recipientSnapshotPO{}).TableName(),
	)
	require.Equal(
		t,
		"announcement_delivery_batches",
		(deliveryBatchPO{}).TableName(),
	)
	requireModelColumn(
		t,
		announcementPO{},
		"SnapshotAt",
		"snapshot_at",
	)
	_, found := reflect.TypeOf(announcementPO{}).FieldByName("UserHighWaterID")
	require.False(t, found)
	_, found = reflect.TypeOf(announcementPO{}).
		FieldByName("MembershipHighWaterID")
	require.False(t, found)
	requireModelColumn(t, recipientSnapshotPO{}, "BatchNo", "batch_no")
	requireModelColumn(t, deliveryBatchPO{}, "EventID", "event_id")
	requireModelColumn(t, deliveryBatchPO{}, "Status", "status")
	requireModelIndex(
		t,
		announcementPO{},
		"Status",
		"idx_announcements_release_projection",
	)
	requireModelIndex(
		t,
		announcementPO{},
		"ProjectionStatus",
		"idx_announcements_release_projection",
	)
	requireModelIndex(
		t,
		recipientSnapshotPO{},
		"AnnouncementID",
		"idx_announcement_snapshot_batch",
	)
	requireModelIndex(
		t,
		recipientSnapshotPO{},
		"UserID",
		"idx_announcement_snapshot_batch",
	)
	requireModelIndex(
		t,
		deliveryBatchPO{},
		"BatchNo",
		"idx_announcement_delivery_claim",
	)
}

func requireModelIndex(
	t *testing.T,
	model any,
	fieldName string,
	indexName string,
) {
	t.Helper()
	field, found := reflect.TypeOf(model).FieldByName(fieldName)
	require.True(t, found, "model field %s must exist", fieldName)
	require.Contains(
		t,
		field.Tag.Get("gorm"),
		"index:"+indexName,
		"model field %s must participate in %s",
		fieldName,
		indexName,
	)
}

func requireModelColumn(
	t *testing.T,
	model any,
	fieldName string,
	columnName string,
) {
	t.Helper()
	field, found := reflect.TypeOf(model).FieldByName(fieldName)
	require.True(t, found, "model field %s must exist", fieldName)
	require.Contains(
		t,
		field.Tag.Get("gorm"),
		"column:"+columnName,
		"model field %s must map to %s",
		fieldName,
		columnName,
	)
}
