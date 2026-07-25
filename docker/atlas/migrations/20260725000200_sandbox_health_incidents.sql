CREATE TABLE `sandbox_provider_health_episodes` (
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `consecutive_failures` INT NOT NULL DEFAULT 0,
  `failure_started_at` BIGINT NOT NULL DEFAULT 0,
  `incident_sequence` BIGINT UNSIGNED NOT NULL DEFAULT 0,
  `incident_id` VARCHAR(128) NOT NULL DEFAULT '',
  `incident_status` VARCHAR(16) NOT NULL DEFAULT 'none',
  `incident_opened_at` BIGINT NOT NULL DEFAULT 0,
  `incident_notified_at` BIGINT NOT NULL DEFAULT 0,
  `incident_closed_at` BIGINT NOT NULL DEFAULT 0,
  `recovery_notified_at` BIGINT NOT NULL DEFAULT 0,
  `last_recovered_at` BIGINT NOT NULL DEFAULT 0,
  `last_checked_at` BIGINT NOT NULL DEFAULT 0,
  `next_check_at` BIGINT NOT NULL DEFAULT 0,
  `lease_owner` VARCHAR(80) NOT NULL DEFAULT '',
  `lease_token` VARCHAR(128) NOT NULL DEFAULT '',
  `lease_expires_at` BIGINT NOT NULL DEFAULT 0,
  `version` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `created_at` BIGINT NOT NULL,
  `updated_at` BIGINT NOT NULL,
  PRIMARY KEY (`provider_id`),
  KEY `idx_sandbox_health_episode_due`
    (`next_check_at`, `lease_expires_at`, `provider_id`),
  KEY `idx_sandbox_health_episode_incident`
    (`incident_status`, `incident_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `sandbox_health_notification_projections` (
  `event_id` VARCHAR(128) NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `incident_id` VARCHAR(128) NOT NULL,
  `incident_sequence` BIGINT UNSIGNED NOT NULL,
  `notification_type` VARCHAR(16) NOT NULL,
  `occurred_at` BIGINT NOT NULL,
  `status` VARCHAR(16) NOT NULL DEFAULT 'pending',
  `attempt_count` INT NOT NULL DEFAULT 0,
  `next_attempt_at` BIGINT NOT NULL DEFAULT 0,
  `lease_owner` VARCHAR(80) NOT NULL DEFAULT '',
  `lease_token` VARCHAR(128) NOT NULL DEFAULT '',
  `lease_expires_at` BIGINT NOT NULL DEFAULT 0,
  `projected_at` BIGINT NOT NULL DEFAULT 0,
  `version` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `created_at` BIGINT NOT NULL,
  `updated_at` BIGINT NOT NULL,
  PRIMARY KEY (`event_id`),
  UNIQUE KEY `uk_sandbox_health_projection_incident`
    (`incident_id`, `notification_type`),
  KEY `idx_sandbox_health_projection_pending`
    (`status`, `next_attempt_at`, `lease_expires_at`, `event_id`),
  KEY `idx_sandbox_health_projection_provider`
    (`provider_id`, `incident_sequence`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
