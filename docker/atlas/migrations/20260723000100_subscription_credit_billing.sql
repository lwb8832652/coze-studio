-- Immutable credit ledger, expiring credit batches, and runtime reservations.

CREATE TABLE `billing_accounts` (
  `id` BIGINT NOT NULL,
  `subject_type` VARCHAR(24) NOT NULL,
  `subject_id` BIGINT NOT NULL,
  `available_micros` BIGINT NOT NULL DEFAULT 0,
  `reserved_micros` BIGINT NOT NULL DEFAULT 0,
  `version` BIGINT NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_billing_account_subject` (`subject_type`, `subject_id`),
  CONSTRAINT `chk_billing_account_available` CHECK (`available_micros` >= 0),
  CONSTRAINT `chk_billing_account_reserved` CHECK (`reserved_micros` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `credit_batches` (
  `id` BIGINT NOT NULL,
  `account_id` BIGINT NOT NULL,
  `source_type` VARCHAR(32) NOT NULL,
  `source_id` VARCHAR(128) NOT NULL DEFAULT '',
  `grant_business_no` VARCHAR(128) NOT NULL,
  `granted_micros` BIGINT NOT NULL,
  `remaining_micros` BIGINT NOT NULL,
  `expires_at` DATETIME(3) NULL,
  `version` BIGINT NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_credit_batch_grant_business` (`grant_business_no`),
  KEY `idx_credit_batch_spend` (`account_id`, `expires_at`, `remaining_micros`, `id`),
  CONSTRAINT `chk_credit_batch_granted` CHECK (`granted_micros` > 0),
  CONSTRAINT `chk_credit_batch_remaining` CHECK (`remaining_micros` >= 0 AND `remaining_micros` <= `granted_micros`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `credit_ledger_entries` (
  `id` BIGINT NOT NULL,
  `account_id` BIGINT NOT NULL,
  `batch_id` BIGINT NULL,
  `direction` VARCHAR(16) NOT NULL,
  `entry_type` VARCHAR(32) NOT NULL,
  `amount_micros` BIGINT NOT NULL,
  `available_after_micros` BIGINT NOT NULL,
  `reserved_after_micros` BIGINT NOT NULL,
  `business_no` VARCHAR(128) NOT NULL,
  `actor_user_id` BIGINT NOT NULL DEFAULT 0,
  `metadata_json` MEDIUMTEXT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_credit_ledger_business` (`business_no`),
  KEY `idx_credit_ledger_account_time` (`account_id`, `created_at`, `id`),
  KEY `idx_credit_ledger_batch` (`batch_id`),
  CONSTRAINT `chk_credit_ledger_amount` CHECK (`amount_micros` > 0),
  CONSTRAINT `chk_credit_ledger_available` CHECK (`available_after_micros` >= 0),
  CONSTRAINT `chk_credit_ledger_reserved` CHECK (`reserved_after_micros` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `credit_reservations` (
  `id` BIGINT NOT NULL,
  `account_id` BIGINT NOT NULL,
  `reserve_business_no` VARCHAR(128) NOT NULL,
  `settlement_business_no` VARCHAR(128) NULL,
  `release_business_no` VARCHAR(128) NULL,
  `reserved_micros` BIGINT NOT NULL,
  `settled_micros` BIGINT NOT NULL DEFAULT 0,
  `released_micros` BIGINT NOT NULL DEFAULT 0,
  `status` VARCHAR(24) NOT NULL,
  `allocations_json` MEDIUMTEXT NOT NULL,
  `expires_at` DATETIME(3) NOT NULL,
  `version` BIGINT NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_credit_reservation_business` (`reserve_business_no`),
  UNIQUE KEY `uk_credit_settlement_business` (`settlement_business_no`),
  UNIQUE KEY `uk_credit_release_business` (`release_business_no`),
  KEY `idx_credit_reservation_account` (`account_id`, `status`, `expires_at`),
  KEY `idx_credit_reservation_expiry` (`status`, `expires_at`),
  CONSTRAINT `chk_credit_reservation_amount` CHECK (`reserved_micros` > 0),
  CONSTRAINT `chk_credit_reservation_settled` CHECK (`settled_micros` >= 0 AND `settled_micros` <= `reserved_micros`),
  CONSTRAINT `chk_credit_reservation_released` CHECK (`released_micros` >= 0 AND `released_micros` <= `reserved_micros`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `billing_audit_logs` (
  `id` BIGINT NOT NULL,
  `account_id` BIGINT NOT NULL DEFAULT 0,
  `actor_user_id` BIGINT NOT NULL DEFAULT 0,
  `action` VARCHAR(64) NOT NULL,
  `business_no` VARCHAR(128) NOT NULL,
  `summary_json` MEDIUMTEXT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_billing_audit_business_action` (`business_no`, `action`),
  KEY `idx_billing_audit_account_time` (`account_id`, `created_at`, `id`),
  KEY `idx_billing_audit_actor_time` (`actor_user_id`, `created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
