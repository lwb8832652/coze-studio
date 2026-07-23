-- Singleton billing control-plane configuration.

CREATE TABLE `billing_system_config` (
  `id` BIGINT NOT NULL,
  `credit_name` VARCHAR(32) NOT NULL DEFAULT '积分',
  `display_scale` INT NOT NULL DEFAULT 2,
  `allow_negative` TINYINT(1) NOT NULL DEFAULT 0,
  `settlement_enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `payment_enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `default_currency` VARCHAR(8) NOT NULL DEFAULT 'CNY',
  `version` BIGINT NOT NULL DEFAULT 1,
  `updated_by` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`id`),
  CONSTRAINT `chk_billing_config_singleton` CHECK (`id` = 1),
  CONSTRAINT `chk_billing_config_scale` CHECK (`display_scale` >= 0 AND `display_scale` <= 6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO `billing_system_config` (`id`) VALUES (1);
