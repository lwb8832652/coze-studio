CREATE TABLE IF NOT EXISTS `agent_run_plans` (
  `run_id` BIGINT NOT NULL,
  `thread_id` BIGINT NOT NULL,
  `space_id` BIGINT NOT NULL,
  `user_id` BIGINT NOT NULL,
  `high_watermark` BIGINT NOT NULL DEFAULT 0,
  `revision` BIGINT NOT NULL DEFAULT 0,
  `created_at` BIGINT NOT NULL,
  `updated_at` BIGINT NOT NULL,
  PRIMARY KEY (`run_id`),
  KEY `idx_agent_run_plans_thread_updated` (`thread_id`, `updated_at`),
  KEY `idx_agent_run_plans_space_updated` (`space_id`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `agent_run_plan_items` (
  `id` BIGINT NOT NULL,
  `run_id` BIGINT NOT NULL,
  `task_id` BIGINT NOT NULL,
  `subject` VARCHAR(512) NOT NULL,
  `description` TEXT NOT NULL,
  `status` VARCHAR(32) NOT NULL,
  `active_form` VARCHAR(512) NOT NULL DEFAULT '',
  `owner` VARCHAR(255) NOT NULL DEFAULT '',
  `blocks` JSON NOT NULL,
  `blocked_by` JSON NOT NULL,
  `metadata` JSON NOT NULL,
  `active` TINYINT(1) NOT NULL DEFAULT 1,
  `version` BIGINT NOT NULL DEFAULT 1,
  `created_at` BIGINT NOT NULL,
  `updated_at` BIGINT NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_agent_run_plan_items_run_task` (`run_id`, `task_id`),
  KEY `idx_agent_run_plan_items_active` (`run_id`, `active`),
  KEY `idx_agent_run_plan_items_updated` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
