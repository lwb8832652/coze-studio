CREATE TABLE `announcements` (
  `id` BIGINT UNSIGNED NOT NULL,
  `title` VARCHAR(128) NOT NULL,
  `body` VARCHAR(512) NOT NULL,
  `severity` VARCHAR(16) NOT NULL,
  `internal_route` VARCHAR(128) NOT NULL DEFAULT '',
  `audience_type` VARCHAR(16) NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `projection_status` VARCHAR(16) NOT NULL DEFAULT 'idle',
  `scheduled_at` BIGINT NOT NULL DEFAULT 0,
  `publish_requested_at` BIGINT NOT NULL DEFAULT 0,
  `snapshot_at` BIGINT NOT NULL DEFAULT 0,
  `published_at` BIGINT NOT NULL DEFAULT 0,
  `cancelled_at` BIGINT NOT NULL DEFAULT 0,
  `created_by` BIGINT UNSIGNED NOT NULL,
  `updated_by` BIGINT UNSIGNED NOT NULL,
  `publish_actor_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `create_idempotency_key` VARCHAR(64) NOT NULL,
  `create_request_hash` CHAR(64) NOT NULL,
  `publish_idempotency_key` VARCHAR(64) NULL,
  `publish_request_hash` CHAR(64) NOT NULL DEFAULT '',
  `snapshot_cursor` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `projection_cursor` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `snapshot_complete` BOOL NOT NULL DEFAULT FALSE,
  `recipient_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `projected_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `next_batch_no` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `last_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `version` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `created_at` BIGINT NOT NULL,
  `updated_at` BIGINT NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_announcements_create_idempotency`
    (`create_idempotency_key`),
  UNIQUE KEY `uk_announcements_publish_idempotency`
    (`publish_idempotency_key`),
  KEY `idx_announcements_replay`
    (`projection_status`, `status`, `scheduled_at`, `id`),
  KEY `idx_announcements_release_projection`
    (`status`, `projection_status`, `id`),
  KEY `idx_announcements_created` (`created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `announcement_audience_targets` (
  `announcement_id` BIGINT UNSIGNED NOT NULL,
  `target_type` VARCHAR(16) NOT NULL,
  `target_id` BIGINT UNSIGNED NOT NULL,
  `created_at` BIGINT NOT NULL,
  PRIMARY KEY (`announcement_id`, `target_type`, `target_id`),
  KEY `idx_announcement_audience_target`
    (`target_type`, `target_id`, `announcement_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `announcement_recipient_snapshots` (
  `announcement_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `batch_no` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `created_at` BIGINT NOT NULL,
  PRIMARY KEY (`announcement_id`, `user_id`),
  KEY `idx_announcement_snapshot_batch`
    (`announcement_id`, `batch_no`, `user_id`),
  KEY `idx_announcement_snapshot_user` (`user_id`, `announcement_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `announcement_delivery_batches` (
  `announcement_id` BIGINT UNSIGNED NOT NULL,
  `batch_no` BIGINT UNSIGNED NOT NULL,
  `event_id` VARCHAR(128) NOT NULL,
  `status` VARCHAR(16) NOT NULL,
  `recipient_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `projected_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `attempt_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `last_error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `created_at` BIGINT NOT NULL,
  `updated_at` BIGINT NOT NULL,
  `projected_at` BIGINT NOT NULL DEFAULT 0,
  PRIMARY KEY (`announcement_id`, `batch_no`),
  UNIQUE KEY `uk_announcement_delivery_event` (`event_id`),
  KEY `idx_announcement_delivery_claim`
    (`status`, `announcement_id`, `batch_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `announcement_audit_events` (
  `id` BIGINT UNSIGNED NOT NULL,
  `announcement_id` BIGINT UNSIGNED NOT NULL,
  `actor_id` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `action` VARCHAR(32) NOT NULL,
  `from_status` VARCHAR(16) NOT NULL DEFAULT '',
  `to_status` VARCHAR(16) NOT NULL DEFAULT '',
  `projection_status` VARCHAR(16) NOT NULL,
  `result` VARCHAR(16) NOT NULL,
  `error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `recipient_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `projected_count` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `created_at` BIGINT NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_announcement_audit_history`
    (`announcement_id`, `created_at`, `id`),
  KEY `idx_announcement_audit_actor`
    (`actor_id`, `created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
