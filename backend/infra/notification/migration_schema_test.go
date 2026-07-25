// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package notification

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNotificationMessageLegacyCompatibilityMigration(t *testing.T) {
	migrationPath := filepath.Join(
		"..",
		"..",
		"..",
		"docker",
		"atlas",
		"migrations",
		"20260725000400_notification_message_legacy_compat.sql",
	)
	contents, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	schema := string(contents)

	for _, fragment := range []string{
		"information_schema.COLUMNS",
		"TABLE_SCHEMA = DATABASE()",
		"COLUMN_NAME = 'event_id'",
		"ADD COLUMN `event_id` varchar(128) NULL",
		"COLUMN_NAME = 'severity'",
		"ADD COLUMN `severity` varchar(16) NOT NULL DEFAULT ''info''",
		"CONCAT('legacy:', SHA2(CAST(`id` AS CHAR), 256))",
		"MODIFY COLUMN `event_id` varchar(128) NOT NULL",
		"information_schema.STATISTICS",
		"INDEX_NAME = 'uk_notification_messages_event'",
		"ADD UNIQUE INDEX `uk_notification_messages_event` (`event_id`)",
		"INDEX_NAME = 'idx_notification_messages_created'",
		"ADD INDEX `idx_notification_messages_created` (`created_at`, `id`)",
	} {
		require.Contains(t, schema, fragment)
	}

	for _, destructiveFragment := range []string{
		"DROP TABLE",
		"DROP COLUMN",
		"DELETE FROM `notification_messages`",
	} {
		require.NotContains(t, schema, destructiveFragment)
	}
}

func TestNotificationRecipientLegacyCompatibilityMigration(t *testing.T) {
	migrationPath := filepath.Join(
		"..",
		"..",
		"..",
		"docker",
		"atlas",
		"migrations",
		"20260725000500_notification_recipient_legacy_compat.sql",
	)
	contents, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	schema := string(contents)

	for _, fragment := range []string{
		"information_schema.COLUMNS",
		"TABLE_SCHEMA = DATABASE()",
		"COLUMN_NAME = 'sequence_no'",
		"ADD COLUMN `sequence_no` bigint unsigned NULL AFTER `id`",
		"ROW_NUMBER() OVER (ORDER BY `created_at`, `id`)",
		"INSERT INTO `notification_sequence`",
		"'recipient_materialization'",
		"GREATEST(`next_value`, VALUES(`next_value`))",
		"MODIFY COLUMN `sequence_no` bigint unsigned NOT NULL",
		"information_schema.STATISTICS",
		"INDEX_NAME = 'uk_notification_recipients_sequence'",
		"ADD UNIQUE INDEX `uk_notification_recipients_sequence` (`sequence_no`)",
		"INDEX_NAME = 'idx_notification_recipients_user_read_sequence'",
		"ADD INDEX `idx_notification_recipients_user_read_sequence` (`user_id`, `read_at`, `sequence_no`)",
		"INDEX_NAME = 'idx_notification_recipients_user_sequence'",
		"ADD INDEX `idx_notification_recipients_user_sequence` (`user_id`, `sequence_no`)",
	} {
		require.Contains(t, schema, fragment)
	}

	for _, destructiveFragment := range []string{
		"DROP TABLE",
		"DROP COLUMN",
		"DELETE FROM `notification_recipients`",
	} {
		require.NotContains(t, schema, destructiveFragment)
	}
}

func TestNotificationMessageLegacyDedupeKeyCompatibilityMigration(t *testing.T) {
	migrationPath := filepath.Join(
		"..",
		"..",
		"..",
		"docker",
		"atlas",
		"migrations",
		"20260725000600_notification_message_dedupe_compat.sql",
	)
	contents, err := os.ReadFile(migrationPath)
	require.NoError(t, err)
	schema := string(contents)

	for _, fragment := range []string{
		"information_schema.COLUMNS",
		"TABLE_SCHEMA = DATABASE()",
		"TABLE_NAME = 'notification_messages'",
		"COLUMN_NAME = 'dedupe_key'",
		"IS_NULLABLE = 'NO'",
		"MODIFY COLUMN `dedupe_key` varchar(191) NULL DEFAULT NULL",
	} {
		require.Contains(t, schema, fragment)
	}

	for _, destructiveFragment := range []string{
		"DROP TABLE",
		"DROP COLUMN",
		"DELETE FROM `notification_messages`",
	} {
		require.NotContains(t, schema, destructiveFragment)
	}
}
