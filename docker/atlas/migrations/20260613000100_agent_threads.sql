CREATE TABLE IF NOT EXISTS `agent_threads` (
  `id` bigint NOT NULL,
  `space_id` bigint NOT NULL,
  `creator_id` bigint NOT NULL,
  `agent_id` bigint NOT NULL DEFAULT 0,
  `title` varchar(255) NOT NULL,
  `status` varchar(32) NOT NULL,
  `source` varchar(32) NOT NULL,
  `legacy_task_id` bigint NOT NULL DEFAULT 0,
  `metadata` json NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  `last_message_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_threads_space_updated` (`space_id`, `updated_at`),
  KEY `idx_agent_threads_space_status` (`space_id`, `status`),
  KEY `idx_agent_threads_creator_updated` (`creator_id`, `updated_at`),
  KEY `idx_agent_threads_legacy_task` (`legacy_task_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
