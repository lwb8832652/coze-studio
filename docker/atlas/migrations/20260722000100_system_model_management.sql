-- Production model-management metadata, encrypted endpoints, and access grants.

ALTER TABLE `model_instance`
  ADD COLUMN `provider_key` VARCHAR(64) NOT NULL DEFAULT '' AFTER `id`,
  ADD COLUMN `model_identifier` VARCHAR(256) NOT NULL DEFAULT '' AFTER `provider_key`,
  ADD COLUMN `description` VARCHAR(1024) NOT NULL DEFAULT '' AFTER `model_identifier`,
  ADD COLUMN `status` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '1 enabled, 0 disabled' AFTER `description`,
  ADD COLUMN `sort_order` BIGINT NOT NULL DEFAULT 0 AFTER `status`,
  ADD COLUMN `creator_id` BIGINT NOT NULL DEFAULT 0 AFTER `sort_order`,
  ADD COLUMN `protocol` VARCHAR(32) NOT NULL DEFAULT '' AFTER `creator_id`,
  ADD COLUMN `routing_strategy` VARCHAR(32) NOT NULL DEFAULT 'round_robin' AFTER `protocol`,
  ADD COLUMN `access_mode` VARCHAR(16) NOT NULL DEFAULT 'all' AFTER `routing_strategy`,
  ADD COLUMN `scenario_json` JSON NULL AFTER `access_mode`,
  ADD COLUMN `reasoning_mode` VARCHAR(32) NOT NULL DEFAULT 'default' AFTER `scenario_json`,
  ADD COLUMN `function_call_mode` VARCHAR(32) NOT NULL DEFAULT 'auto' AFTER `reasoning_mode`,
  ADD COLUMN `max_context_tokens` BIGINT NOT NULL DEFAULT 0 AFTER `function_call_mode`,
  ADD COLUMN `max_output_tokens` BIGINT NOT NULL DEFAULT 0 AFTER `max_context_tokens`,
  ADD KEY `idx_model_instance_management` (`status`, `sort_order`, `deleted_at`),
  ADD KEY `idx_model_instance_provider` (`provider_key`, `status`, `deleted_at`),
  ADD KEY `idx_model_instance_creator` (`creator_id`, `deleted_at`);

UPDATE `model_instance` SET `sort_order` = `id` WHERE `sort_order` = 0;

CREATE TABLE `model_instance_endpoint` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `model_id` BIGINT NOT NULL,
  `base_url` VARCHAR(2048) NOT NULL DEFAULT '',
  `api_key_envelope` MEDIUMTEXT NOT NULL,
  `api_key_fingerprint` VARCHAR(64) NOT NULL DEFAULT '',
  `weight` INT NOT NULL DEFAULT 1,
  `enabled` TINYINT(1) NOT NULL DEFAULT 1,
  `sort_order` INT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  KEY `idx_model_endpoint_order` (`model_id`, `enabled`, `sort_order`, `deleted_at`),
  KEY `idx_model_endpoint_updated` (`model_id`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `model_instance_grant` (
  `id` BIGINT NOT NULL AUTO_INCREMENT,
  `model_id` BIGINT NOT NULL,
  `subject_type` VARCHAR(16) NOT NULL,
  `subject_id` BIGINT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL,
  `active_key` TINYINT GENERATED ALWAYS AS (
    CASE WHEN `deleted_at` IS NULL THEN 1 ELSE NULL END
  ) STORED,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uniq_model_subject` (`model_id`, `subject_type`, `subject_id`, `active_key`),
  KEY `idx_model_grant_subject` (`subject_type`, `subject_id`, `deleted_at`),
  KEY `idx_model_grant_model` (`model_id`, `deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
