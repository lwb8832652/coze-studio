-- Subscription catalog, credit packages, orders, payments, and durable fulfillment.

CREATE TABLE `subscription_plans` (
  `id` BIGINT NOT NULL,
  `plan_key` VARCHAR(64) NOT NULL,
  `name` VARCHAR(80) NOT NULL,
  `description` VARCHAR(512) NOT NULL DEFAULT '',
  `status` VARCHAR(24) NOT NULL DEFAULT 'draft',
  `sort_order` INT NOT NULL DEFAULT 0,
  `current_version` INT NOT NULL DEFAULT 0,
  `created_by` BIGINT NOT NULL,
  `updated_by` BIGINT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_subscription_plan_key` (`plan_key`),
  KEY `idx_subscription_plan_status_sort` (`status`, `sort_order`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `subscription_plan_versions` (
  `id` BIGINT NOT NULL,
  `plan_id` BIGINT NOT NULL,
  `version` INT NOT NULL,
  `billing_cycle` VARCHAR(24) NOT NULL,
  `price_micros` BIGINT NOT NULL,
  `currency` VARCHAR(8) NOT NULL DEFAULT 'CNY',
  `credit_grant_micros` BIGINT NOT NULL,
  `features_json` MEDIUMTEXT NOT NULL,
  `effective_at` DATETIME(3) NOT NULL,
  `created_by` BIGINT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_subscription_plan_version` (`plan_id`, `version`),
  KEY `idx_subscription_plan_effective` (`plan_id`, `effective_at`, `id`),
  CONSTRAINT `chk_subscription_plan_price` CHECK (`price_micros` >= 0),
  CONSTRAINT `chk_subscription_plan_credit` CHECK (`credit_grant_micros` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `credit_packages` (
  `id` BIGINT NOT NULL,
  `package_key` VARCHAR(64) NOT NULL,
  `name` VARCHAR(80) NOT NULL,
  `description` VARCHAR(512) NOT NULL DEFAULT '',
  `status` VARCHAR(24) NOT NULL DEFAULT 'draft',
  `price_micros` BIGINT NOT NULL,
  `currency` VARCHAR(8) NOT NULL DEFAULT 'CNY',
  `credit_micros` BIGINT NOT NULL,
  `validity_days` INT NOT NULL DEFAULT 0,
  `purchase_limit` INT NOT NULL DEFAULT 0,
  `sort_order` INT NOT NULL DEFAULT 0,
  `version` BIGINT NOT NULL DEFAULT 1,
  `created_by` BIGINT NOT NULL,
  `updated_by` BIGINT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `deleted_at` DATETIME(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_credit_package_key` (`package_key`),
  KEY `idx_credit_package_status_sort` (`status`, `sort_order`, `id`),
  CONSTRAINT `chk_credit_package_price` CHECK (`price_micros` >= 0),
  CONSTRAINT `chk_credit_package_credit` CHECK (`credit_micros` > 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `user_subscriptions` (
  `id` BIGINT NOT NULL,
  `account_id` BIGINT NOT NULL,
  `plan_id` BIGINT NOT NULL,
  `plan_version_id` BIGINT NOT NULL,
  `source_order_id` BIGINT NOT NULL,
  `status` VARCHAR(24) NOT NULL,
  `current_period_start` DATETIME(3) NOT NULL,
  `current_period_end` DATETIME(3) NOT NULL,
  `auto_renew` TINYINT(1) NOT NULL DEFAULT 0,
  `provider_subscription_id` VARCHAR(128) NOT NULL DEFAULT '',
  `version` BIGINT NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_subscription_order` (`source_order_id`),
  KEY `idx_user_subscription_account` (`account_id`, `status`, `current_period_end`),
  KEY `idx_user_subscription_renewal` (`status`, `auto_renew`, `current_period_end`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `subscription_cycle_grants` (
  `id` BIGINT NOT NULL,
  `subscription_id` BIGINT NOT NULL,
  `period_start` DATETIME(3) NOT NULL,
  `period_end` DATETIME(3) NOT NULL,
  `credit_micros` BIGINT NOT NULL,
  `grant_business_no` VARCHAR(128) NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_subscription_cycle_period` (`subscription_id`, `period_start`),
  UNIQUE KEY `uk_subscription_cycle_business` (`grant_business_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `billing_orders` (
  `id` BIGINT NOT NULL,
  `order_no` VARCHAR(64) NOT NULL,
  `user_id` BIGINT NOT NULL,
  `account_id` BIGINT NOT NULL,
  `order_type` VARCHAR(24) NOT NULL,
  `status` VARCHAR(24) NOT NULL,
  `payment_status` VARCHAR(24) NOT NULL,
  `fulfillment_status` VARCHAR(24) NOT NULL,
  `total_micros` BIGINT NOT NULL,
  `currency` VARCHAR(8) NOT NULL,
  `snapshot_json` MEDIUMTEXT NOT NULL,
  `version` BIGINT NOT NULL DEFAULT 1,
  `expires_at` DATETIME(3) NOT NULL,
  `paid_at` DATETIME(3) NULL,
  `fulfilled_at` DATETIME(3) NULL,
  `closed_at` DATETIME(3) NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_billing_order_no` (`order_no`),
  KEY `idx_billing_order_user_time` (`user_id`, `created_at`, `id`),
  KEY `idx_billing_order_status_expiry` (`status`, `expires_at`),
  CONSTRAINT `chk_billing_order_total` CHECK (`total_micros` >= 0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `billing_order_items` (
  `id` BIGINT NOT NULL,
  `order_id` BIGINT NOT NULL,
  `item_type` VARCHAR(24) NOT NULL,
  `target_id` BIGINT NOT NULL,
  `target_version_id` BIGINT NOT NULL DEFAULT 0,
  `quantity` INT NOT NULL DEFAULT 1,
  `unit_price_micros` BIGINT NOT NULL,
  `snapshot_json` MEDIUMTEXT NOT NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  KEY `idx_billing_order_item_order` (`order_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `payment_transactions` (
  `id` BIGINT NOT NULL,
  `order_id` BIGINT NOT NULL,
  `gateway` VARCHAR(32) NOT NULL,
  `provider_transaction_id` VARCHAR(128) NOT NULL,
  `status` VARCHAR(24) NOT NULL,
  `amount_micros` BIGINT NOT NULL,
  `currency` VARCHAR(8) NOT NULL,
  `event_digest` VARCHAR(64) NOT NULL,
  `failure_code` VARCHAR(64) NOT NULL DEFAULT '',
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_payment_gateway_transaction` (`gateway`, `provider_transaction_id`),
  UNIQUE KEY `uk_payment_event_digest` (`event_digest`),
  KEY `idx_payment_order_time` (`order_id`, `created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
