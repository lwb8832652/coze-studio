-- Keep deployments that already have the legacy notification_messages table
-- compatible with the reliable notification projection. The original create
-- migration used IF NOT EXISTS, so an older table could survive without the
-- new columns and indexes.

SET @notification_event_id_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_messages'
    AND COLUMN_NAME = 'event_id'
);
SET @notification_event_id_ddl = IF(
  @notification_event_id_exists = 0,
  'ALTER TABLE `notification_messages` ADD COLUMN `event_id` varchar(128) NULL AFTER `id`',
  'SELECT 1'
);
PREPARE notification_event_id_statement
  FROM @notification_event_id_ddl;
EXECUTE notification_event_id_statement;
DEALLOCATE PREPARE notification_event_id_statement;

SET @notification_severity_exists = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_messages'
    AND COLUMN_NAME = 'severity'
);
SET @notification_severity_ddl = IF(
  @notification_severity_exists = 0,
  'ALTER TABLE `notification_messages` ADD COLUMN `severity` varchar(16) NOT NULL DEFAULT ''info'' AFTER `category`',
  'SELECT 1'
);
PREPARE notification_severity_statement
  FROM @notification_severity_ddl;
EXECUTE notification_severity_statement;
DEALLOCATE PREPARE notification_severity_statement;

UPDATE `notification_messages`
SET `event_id` = CONCAT('legacy:', SHA2(CAST(`id` AS CHAR), 256))
WHERE `event_id` IS NULL OR `event_id` = '';

SET @notification_event_id_nullable = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_messages'
    AND COLUMN_NAME = 'event_id'
    AND IS_NULLABLE = 'YES'
);
SET @notification_event_id_not_null_ddl = IF(
  @notification_event_id_nullable > 0,
  'ALTER TABLE `notification_messages` MODIFY COLUMN `event_id` varchar(128) NOT NULL',
  'SELECT 1'
);
PREPARE notification_event_id_not_null_statement
  FROM @notification_event_id_not_null_ddl;
EXECUTE notification_event_id_not_null_statement;
DEALLOCATE PREPARE notification_event_id_not_null_statement;

SET @notification_severity_has_default = (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_messages'
    AND COLUMN_NAME = 'severity'
    AND COLUMN_DEFAULT IS NOT NULL
);
SET @notification_severity_default_ddl = IF(
  @notification_severity_has_default > 0,
  'ALTER TABLE `notification_messages` MODIFY COLUMN `severity` varchar(16) NOT NULL',
  'SELECT 1'
);
PREPARE notification_severity_default_statement
  FROM @notification_severity_default_ddl;
EXECUTE notification_severity_default_statement;
DEALLOCATE PREPARE notification_severity_default_statement;

SET @notification_event_index_exists = (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_messages'
    AND INDEX_NAME = 'uk_notification_messages_event'
);
SET @notification_event_index_ddl = IF(
  @notification_event_index_exists = 0,
  'ALTER TABLE `notification_messages` ADD UNIQUE INDEX `uk_notification_messages_event` (`event_id`)',
  'SELECT 1'
);
PREPARE notification_event_index_statement
  FROM @notification_event_index_ddl;
EXECUTE notification_event_index_statement;
DEALLOCATE PREPARE notification_event_index_statement;

SET @notification_created_index_exists = (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'notification_messages'
    AND INDEX_NAME = 'idx_notification_messages_created'
);
SET @notification_created_index_ddl = IF(
  @notification_created_index_exists = 0,
  'ALTER TABLE `notification_messages` ADD INDEX `idx_notification_messages_created` (`created_at`, `id`)',
  'SELECT 1'
);
PREPARE notification_created_index_statement
  FROM @notification_created_index_ddl;
EXECUTE notification_created_index_statement;
DEALLOCATE PREPARE notification_created_index_statement;
