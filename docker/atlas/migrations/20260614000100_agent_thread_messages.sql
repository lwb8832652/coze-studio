CREATE TABLE IF NOT EXISTS `agent_thread_messages` (
  `id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `run_id` bigint NOT NULL DEFAULT 0,
  `role` varchar(32) NOT NULL,
  `content` longtext NOT NULL,
  `metadata` json NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_thread_messages_thread_created` (`thread_id`, `created_at`),
  KEY `idx_agent_thread_messages_run_created` (`run_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
