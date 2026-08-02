CREATE TABLE `object_storage_configs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(128) NOT NULL,
  `provider_type` VARCHAR(32) NOT NULL,
  `config_json` JSON NOT NULL,
  `credential_secret` TEXT NOT NULL,
  `active_slot` TINYINT UNSIGNED NULL,
  `health_status` VARCHAR(32) NOT NULL DEFAULT 'unknown',
  `last_health_code` VARCHAR(64) NOT NULL DEFAULT '',
  `last_health_message` VARCHAR(255) NOT NULL DEFAULT '',
  `last_health_latency_ms` INT UNSIGNED NOT NULL DEFAULT 0,
  `last_health_at` DATETIME(3) NULL,
  `version` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `runtime_revision` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_object_storage_configs_name` (`name`),
  UNIQUE KEY `uk_object_storage_configs_active_slot` (`active_slot`),
  KEY `idx_object_storage_configs_provider_type` (`provider_type`),
  CONSTRAINT `ck_object_storage_configs_active_slot`
    CHECK (`active_slot` IS NULL OR `active_slot` = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
