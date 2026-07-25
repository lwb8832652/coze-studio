-- Durable low-credit configuration and active hysteresis episodes.

CREATE TABLE `billing_credit_threshold_configs` (
  `subject_type` VARCHAR(24) NOT NULL,
  `subject_id` BIGINT NOT NULL,
  `enabled` TINYINT(1) NOT NULL DEFAULT 0,
  `threshold_micros` BIGINT NOT NULL DEFAULT 0,
  `recovery_margin_micros` BIGINT NOT NULL DEFAULT 0,
  `version` BIGINT NOT NULL DEFAULT 1,
  `updated_by` BIGINT NOT NULL DEFAULT 0,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`subject_type`, `subject_id`),
  CONSTRAINT `chk_credit_threshold_subject_type`
    CHECK (`subject_type` IN ('user', 'workspace')),
  CONSTRAINT `chk_credit_threshold_subject_id`
    CHECK (`subject_id` >= 0),
  CONSTRAINT `chk_credit_threshold_config`
    CHECK (
      (
        `enabled` = 0
        AND `threshold_micros` = 0
        AND `recovery_margin_micros` = 0
      )
      OR
      (
        `enabled` = 1
        AND `threshold_micros` BETWEEN 1 AND 1000000000000000
        AND `recovery_margin_micros` BETWEEN 1 AND 1000000000000000
        AND `threshold_micros`
          <= 1000000000000000 - `recovery_margin_micros`
      )
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- subject_id = 0 is the explicit default for each supported subject type.
-- A positive subject_id overrides its type default, including with disabled.
INSERT INTO `billing_credit_threshold_configs`
  (
    `subject_type`,
    `subject_id`,
    `enabled`,
    `threshold_micros`,
    `recovery_margin_micros`
  )
VALUES
  ('user', 0, 0, 0, 0),
  ('workspace', 0, 0, 0, 0);

CREATE TABLE `billing_credit_threshold_episodes` (
  `account_id` BIGINT NOT NULL,
  `episode_no` BIGINT NOT NULL,
  `active` TINYINT(1) NOT NULL,
  `threshold_micros` BIGINT NOT NULL,
  `recovery_margin_micros` BIGINT NOT NULL,
  `version` BIGINT NOT NULL DEFAULT 1,
  `opened_at` DATETIME(3) NOT NULL,
  `recovered_at` DATETIME(3) NULL,
  `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  PRIMARY KEY (`account_id`),
  KEY `idx_credit_threshold_episode_active`
    (`active`, `updated_at`, `account_id`),
  CONSTRAINT `chk_credit_threshold_episode_no`
    CHECK (`episode_no` > 0),
  CONSTRAINT `chk_credit_threshold_episode_threshold`
    CHECK (
      `threshold_micros` BETWEEN 1 AND 1000000000000000
      AND `recovery_margin_micros` BETWEEN 1 AND 1000000000000000
      AND `threshold_micros`
        <= 1000000000000000 - `recovery_margin_micros`
    ),
  CONSTRAINT `chk_credit_threshold_episode_version`
    CHECK (`version` > 0),
  CONSTRAINT `chk_credit_threshold_episode_active`
    CHECK (`active` IN (0, 1)),
  CONSTRAINT `chk_credit_threshold_episode_recovery`
    CHECK (
      (`active` = 1 AND `recovered_at` IS NULL)
      OR
      (`active` = 0 AND `recovered_at` IS NOT NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
