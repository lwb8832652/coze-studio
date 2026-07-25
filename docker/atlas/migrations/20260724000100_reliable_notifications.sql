CREATE TABLE IF NOT EXISTS `notification_outbox` (
  `id` bigint NOT NULL,
  `event_id` varchar(128) NOT NULL,
  `idempotency_key` varchar(64) NOT NULL,
  `event_type` varchar(64) NOT NULL,
  `aggregate_type` varchar(64) NOT NULL,
  `aggregate_id` varchar(128) NOT NULL,
  `aggregate_version` bigint NOT NULL,
  `occurred_at` bigint NOT NULL,
  `space_id` bigint NOT NULL DEFAULT 0,
  `actor_id` bigint NOT NULL DEFAULT 0,
  `recipient_policy` varchar(64) NOT NULL,
  `payload_schema` int NOT NULL,
  `payload_json` json NOT NULL,
  `status` varchar(16) NOT NULL,
  `attempt_count` int NOT NULL DEFAULT 0,
  `available_at` bigint NOT NULL,
  `locked_at` bigint NOT NULL DEFAULT 0,
  `locked_by` varchar(128) NOT NULL DEFAULT '',
  `last_error_code` varchar(64) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  `delivered_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_outbox_event` (`event_id`),
  UNIQUE KEY `uk_notification_outbox_idempotency` (`idempotency_key`),
  KEY `idx_notification_outbox_ready` (`status`, `available_at`, `id`),
  KEY `idx_notification_outbox_aggregate`
    (`aggregate_type`, `aggregate_id`, `aggregate_version`),
  KEY `idx_notification_outbox_lease` (`locked_at`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `notification_messages` (
  `id` bigint NOT NULL,
  `event_id` varchar(128) NOT NULL,
  `scope` varchar(32) NOT NULL,
  `space_id` bigint NOT NULL DEFAULT 0,
  `sender_id` bigint NOT NULL DEFAULT 0,
  `category` varchar(32) NOT NULL,
  `severity` varchar(16) NOT NULL,
  `event_type` varchar(64) NOT NULL,
  `title` varchar(128) NOT NULL,
  `content` varchar(512) NOT NULL DEFAULT '',
  `target_type` varchar(32) NOT NULL DEFAULT 'none',
  `target_id` varchar(128) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_messages_event` (`event_id`),
  KEY `idx_notification_messages_space_created` (`space_id`, `created_at`),
  KEY `idx_notification_messages_created` (`created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `notification_sequence` (
  `sequence_key` varchar(64) NOT NULL,
  `next_value` bigint unsigned NOT NULL,
  `updated_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`sequence_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT IGNORE INTO `notification_sequence`
  (`sequence_key`, `next_value`, `updated_at`)
VALUES
  ('recipient_materialization', 1, 0);

CREATE TABLE IF NOT EXISTS `notification_recipients` (
  `id` bigint NOT NULL,
  `sequence_no` bigint unsigned NOT NULL,
  `notification_id` bigint NOT NULL,
  `user_id` bigint NOT NULL,
  `read_at` bigint NOT NULL DEFAULT 0,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_notification_recipients_sequence` (`sequence_no`),
  UNIQUE KEY `uk_notification_recipients_message_user`
    (`notification_id`, `user_id`),
  KEY `idx_notification_recipients_user_read_sequence`
    (`user_id`, `read_at`, `sequence_no`),
  KEY `idx_notification_recipients_user_sequence`
    (`user_id`, `sequence_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
