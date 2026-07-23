-- Versioned model prices and idempotent usage settlement.

CREATE TABLE `model_price_versions` (
  `id` BIGINT NOT NULL,
  `provider` VARCHAR(64) NOT NULL,
  `model_id` VARCHAR(128) NOT NULL,
  `version` INT NOT NULL,
  `currency` VARCHAR(8) NOT NULL DEFAULT 'CREDITS',
  `input_per_million_micros` BIGINT NOT NULL DEFAULT 0,
  `output_per_million_micros` BIGINT NOT NULL DEFAULT 0,
  `cache_write_per_million_micros` BIGINT NOT NULL DEFAULT 0,
  `cache_hit_per_million_micros` BIGINT NOT NULL DEFAULT 0,
  `status` VARCHAR(24) NOT NULL DEFAULT 'draft',
  `effective_at` DATETIME(3) NOT NULL,
  `created_by` BIGINT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_model_price_version` (`provider`, `model_id`, `version`),
  KEY `idx_model_price_effective` (`provider`, `model_id`, `status`, `effective_at`, `version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `model_usage_records` (
  `id` BIGINT NOT NULL,
  `account_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `space_id` BIGINT NOT NULL DEFAULT 0,
  `task_id` BIGINT NOT NULL DEFAULT 0,
  `run_id` VARCHAR(128) NOT NULL,
  `usage_sequence` BIGINT NOT NULL,
  `provider` VARCHAR(64) NOT NULL,
  `model_id` VARCHAR(128) NOT NULL,
  `price_version_id` BIGINT NOT NULL,
  `input_tokens` BIGINT NOT NULL DEFAULT 0,
  `output_tokens` BIGINT NOT NULL DEFAULT 0,
  `cache_write_tokens` BIGINT NOT NULL DEFAULT 0,
  `cache_hit_tokens` BIGINT NOT NULL DEFAULT 0,
  `charge_micros` BIGINT NOT NULL,
  `price_snapshot_json` MEDIUMTEXT NOT NULL,
  `status` VARCHAR(24) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_model_usage_run_sequence` (`run_id`, `usage_sequence`),
  KEY `idx_model_usage_account_time` (`account_id`, `created_at`, `id`),
  KEY `idx_model_usage_model_time` (`provider`, `model_id`, `created_at`, `id`),
  KEY `idx_model_usage_task` (`task_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `billing_settlements` (
  `id` BIGINT NOT NULL,
  `usage_record_id` BIGINT NOT NULL,
  `reservation_business_no` VARCHAR(128) NOT NULL,
  `settlement_business_no` VARCHAR(128) NOT NULL,
  `status` VARCHAR(24) NOT NULL,
  `attempt_count` INT NOT NULL DEFAULT 0,
  `next_retry_at` DATETIME(3) NULL,
  `last_error` VARCHAR(512) NOT NULL DEFAULT '',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_billing_settlement_usage` (`usage_record_id`),
  UNIQUE KEY `uk_billing_settlement_business` (`settlement_business_no`),
  KEY `idx_billing_settlement_retry` (`status`, `next_retry_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `billing_reconciliation_runs` (
  `id` BIGINT NOT NULL,
  `scope` VARCHAR(32) NOT NULL,
  `status` VARCHAR(24) NOT NULL,
  `started_at` DATETIME(3) NOT NULL,
  `completed_at` DATETIME(3) NULL,
  `checked_count` BIGINT NOT NULL DEFAULT 0,
  `mismatch_count` BIGINT NOT NULL DEFAULT 0,
  `summary_json` MEDIUMTEXT NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_billing_reconciliation_time` (`started_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
