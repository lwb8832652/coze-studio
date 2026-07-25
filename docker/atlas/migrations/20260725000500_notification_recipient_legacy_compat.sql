-- Upgrade the legacy notification_recipients table to the reliable cursor
-- contract without deleting or reordering historical recipient records.

SET @notification_sequence_no_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_recipients'
    AND COLUMN_NAME = 'sequence_no'
);
SET @notification_sequence_no_ddl = IF(
  @notification_sequence_no_exists = 0,
  'ALTER TABLE `notification_recipients` ADD COLUMN `sequence_no` bigint unsigned NULL AFTER `id`',
  'SELECT 1'
);
PREPARE notification_sequence_no_statement
  FROM @notification_sequence_no_ddl;
EXECUTE notification_sequence_no_statement;
DEALLOCATE PREPARE notification_sequence_no_statement;

UPDATE `notification_recipients` AS recipients
JOIN (
  SELECT ranked.`id`, ranked.`sequence_no`
  FROM (
    SELECT
      `id`,
      ROW_NUMBER() OVER (ORDER BY `created_at`, `id`) AS `sequence_no`
    FROM `notification_recipients`
  ) AS ranked
) AS legacy_recipients
  ON legacy_recipients.`id` = recipients.`id`
SET recipients.`sequence_no` = legacy_recipients.`sequence_no`
WHERE recipients.`sequence_no` IS NULL
   OR recipients.`sequence_no` = 0;

INSERT INTO `notification_sequence`
  (`sequence_key`, `next_value`, `updated_at`)
SELECT
  'recipient_materialization',
  COALESCE(MAX(`sequence_no`), 0) + 1,
  UNIX_TIMESTAMP()
FROM `notification_recipients`
ON DUPLICATE KEY UPDATE
  `next_value` = GREATEST(`next_value`, VALUES(`next_value`)),
  `updated_at` = GREATEST(`updated_at`, VALUES(`updated_at`));

SET @notification_sequence_no_nullable = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_recipients'
    AND COLUMN_NAME = 'sequence_no'
    AND IS_NULLABLE = 'YES'
);
SET @notification_sequence_no_not_null_ddl = IF(
  @notification_sequence_no_nullable > 0,
  'ALTER TABLE `notification_recipients` MODIFY COLUMN `sequence_no` bigint unsigned NOT NULL',
  'SELECT 1'
);
PREPARE notification_sequence_no_not_null_statement
  FROM @notification_sequence_no_not_null_ddl;
EXECUTE notification_sequence_no_not_null_statement;
DEALLOCATE PREPARE notification_sequence_no_not_null_statement;

SET @notification_sequence_index_exists = (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_recipients'
    AND INDEX_NAME = 'uk_notification_recipients_sequence'
);
SET @notification_sequence_index_ddl = IF(
  @notification_sequence_index_exists = 0,
  'ALTER TABLE `notification_recipients` ADD UNIQUE INDEX `uk_notification_recipients_sequence` (`sequence_no`)',
  'SELECT 1'
);
PREPARE notification_sequence_index_statement
  FROM @notification_sequence_index_ddl;
EXECUTE notification_sequence_index_statement;
DEALLOCATE PREPARE notification_sequence_index_statement;

SET @notification_user_read_sequence_index_exists = (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_recipients'
    AND INDEX_NAME = 'idx_notification_recipients_user_read_sequence'
);
SET @notification_user_read_sequence_index_ddl = IF(
  @notification_user_read_sequence_index_exists = 0,
  'ALTER TABLE `notification_recipients` ADD INDEX `idx_notification_recipients_user_read_sequence` (`user_id`, `read_at`, `sequence_no`)',
  'SELECT 1'
);
PREPARE notification_user_read_sequence_index_statement
  FROM @notification_user_read_sequence_index_ddl;
EXECUTE notification_user_read_sequence_index_statement;
DEALLOCATE PREPARE notification_user_read_sequence_index_statement;

SET @notification_user_sequence_index_exists = (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_recipients'
    AND INDEX_NAME = 'idx_notification_recipients_user_sequence'
);
SET @notification_user_sequence_index_ddl = IF(
  @notification_user_sequence_index_exists = 0,
  'ALTER TABLE `notification_recipients` ADD INDEX `idx_notification_recipients_user_sequence` (`user_id`, `sequence_no`)',
  'SELECT 1'
);
PREPARE notification_user_sequence_index_statement
  FROM @notification_user_sequence_index_ddl;
EXECUTE notification_user_sequence_index_statement;
DEALLOCATE PREPARE notification_user_sequence_index_statement;
