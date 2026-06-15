CREATE TABLE IF NOT EXISTS `agent_thread_memories` (
  `id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `run_id` bigint NOT NULL DEFAULT 0,
  `space_id` bigint NOT NULL,
  `scope` varchar(32) NOT NULL,
  `content` longtext NOT NULL,
  `metadata` json DEFAULT NULL,
  `score` double NOT NULL DEFAULT 0,
  `expires_at` bigint NOT NULL DEFAULT 0,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_thread_memories_thread_run_scope` (`thread_id`, `run_id`, `scope`),
  KEY `idx_agent_thread_memories_space_scope` (`space_id`, `scope`),
  KEY `idx_agent_thread_memories_expires` (`expires_at`),
  KEY `idx_agent_thread_memories_updated` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
