ALTER TABLE `sandbox_scheduler_settings`
  ADD COLUMN `session_settings_json` JSON NULL AFTER `settings_json`,
  ADD COLUMN `session_settings_version` BIGINT UNSIGNED NOT NULL DEFAULT 1 AFTER `session_settings_json`,
  ADD COLUMN `session_settings_updated_by` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `session_settings_version`,
  ADD COLUMN `session_settings_updated_at` DATETIME(3) NULL AFTER `session_settings_updated_by`,
  ADD COLUMN `aio_runtime_generation` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `session_settings_updated_at`,
  ADD COLUMN `aio_runtime_deployment_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '' AFTER `aio_runtime_generation`,
  ADD COLUMN `aio_runtime_sentinel_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '' AFTER `aio_runtime_deployment_id`;

UPDATE `sandbox_scheduler_settings`
SET `session_settings_json` = '{"core_enabled":false,"interactive_enabled":false,"host_shell_enabled":false,"core_weight":1,"heavy_weight":2,"per_user_active_limit":1,"idle_session_limit":20,"idle_shell_limit":4,"session_idle_ttl_seconds":1200,"shell_idle_ttl_seconds":300,"command_timeout_seconds":600,"cancel_grace_seconds":5,"workspace_quota_mb":2048}',
    `session_settings_updated_at` = UTC_TIMESTAMP(3)
WHERE `id` = 1;

ALTER TABLE `sandbox_scheduler_settings`
  MODIFY COLUMN `session_settings_json` JSON NOT NULL;

CREATE TABLE `sandbox_runtime_sessions` (
  `session_id` CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `deployment_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `provider_id` BIGINT UNSIGNED NOT NULL,
  `space_id` BIGINT UNSIGNED NOT NULL,
  `user_id` BIGINT UNSIGNED NOT NULL,
  `thread_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `profile` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `state` VARCHAR(24) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `runtime_generation` BIGINT UNSIGNED NOT NULL,
  `upstream_shell_id` VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NULL,
  `recovery_reason` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '',
  `version` BIGINT UNSIGNED NOT NULL,
  `last_activity_at` DATETIME(3) NOT NULL,
  `expires_at` DATETIME(3) NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`session_id`),
  UNIQUE KEY `uk_sandbox_runtime_session_business` (`deployment_id`, `provider_id`, `space_id`, `user_id`, `thread_id`, `profile`),
  KEY `idx_sandbox_runtime_session_provider` (`provider_id`),
  KEY `idx_sandbox_runtime_session_recovery` (`deployment_id`, `runtime_generation`, `state`, `session_id`),
  KEY `idx_sandbox_runtime_session_expiry` (`deployment_id`, `state`, `expires_at`, `session_id`),
  CONSTRAINT `fk_sandbox_runtime_session_provider`
    FOREIGN KEY (`provider_id`) REFERENCES `sandbox_providers` (`id`),
  CONSTRAINT `chk_sandbox_runtime_session_profile`
    CHECK (`profile` IN ('core', 'interactive')),
  CONSTRAINT `chk_sandbox_runtime_session_state`
    CHECK (`state` IN ('active', 'recovering', 'released', 'destroyed')),
  CONSTRAINT `chk_sandbox_runtime_session_generation`
    CHECK (`runtime_generation` > 0),
  CONSTRAINT `chk_sandbox_runtime_session_version`
    CHECK (`version` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
