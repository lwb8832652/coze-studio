ALTER TABLE `sandbox_providers`
  ADD COLUMN `last_health_features_json` JSON NULL AFTER `last_health_capabilities_json`;

UPDATE `sandbox_providers`
SET `last_health_features_json` = JSON_ARRAY()
WHERE `last_health_features_json` IS NULL;

ALTER TABLE `sandbox_providers`
  MODIFY COLUMN `last_health_features_json` JSON NOT NULL;

CREATE TABLE `sandbox_scheduler_settings` (
  `id` TINYINT UNSIGNED NOT NULL,
  `settings_json` JSON NOT NULL,
  `version` BIGINT UNSIGNED NOT NULL,
  `updated_by` BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  CONSTRAINT `chk_sandbox_scheduler_settings_singleton` CHECK (`id` = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `sandbox_scheduler_settings` (
  `id`, `settings_json`, `version`, `updated_by`, `created_at`, `updated_at`
) VALUES (
  1,
  '{"total_weight":2,"max_outstanding":32,"global_queue_depth":32,"per_space_queue_depth":8,"per_user_queue_depth":4,"host_memory_reserve_mb":1536,"cancel_grace_seconds":5,"health_failure_threshold":3,"health_recovery_threshold":2,"workloads":{"agent":{"weight":2,"cpu_limit":1.25,"memory_limit_mb":1536,"pid_limit":128,"queue_timeout_seconds":600,"idle_ttl_seconds":300},"appdev":{"weight":2,"cpu_limit":1.25,"memory_limit_mb":1536,"pid_limit":192,"queue_timeout_seconds":1200,"idle_ttl_seconds":600},"mcp_stdio":{"weight":1,"cpu_limit":0.4,"memory_limit_mb":384,"pid_limit":64,"queue_timeout_seconds":300,"idle_ttl_seconds":180},"plugin":{"weight":1,"cpu_limit":0.4,"memory_limit_mb":384,"pid_limit":64,"queue_timeout_seconds":300,"idle_ttl_seconds":0}}}',
  1,
  0,
  UTC_TIMESTAMP(3),
  UTC_TIMESTAMP(3)
);
