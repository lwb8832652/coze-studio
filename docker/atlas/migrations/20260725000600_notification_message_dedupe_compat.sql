-- Allow reliable notification projections to coexist with deployments that
-- still carry the legacy dedupe_key column. New projections use event_id for
-- idempotency and intentionally omit dedupe_key. Making the legacy column
-- nullable preserves existing values while letting new rows store NULL; the
-- existing unique index safely permits multiple NULL values.
SET @notification_message_dedupe_key_requires_compat = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_messages'
    AND COLUMN_NAME = 'dedupe_key'
    AND IS_NULLABLE = 'NO'
);

SET @notification_message_dedupe_key_compat_sql = IF(
  @notification_message_dedupe_key_requires_compat > 0,
  'ALTER TABLE `notification_messages` MODIFY COLUMN `dedupe_key` varchar(191) NULL DEFAULT NULL',
  'SELECT 1'
);

PREPARE notification_message_dedupe_key_compat_statement
  FROM @notification_message_dedupe_key_compat_sql;
EXECUTE notification_message_dedupe_key_compat_statement;
DEALLOCATE PREPARE notification_message_dedupe_key_compat_statement;
