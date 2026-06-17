CREATE TABLE IF NOT EXISTS `agent_checkpoints` (
  `id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `run_id` bigint NOT NULL,
  `parent_checkpoint_id` bigint NOT NULL DEFAULT 0,
  `checkpoint_ns` varchar(128) NOT NULL DEFAULT '',
  `channel_values` json NOT NULL,
  `channel_versions` json NOT NULL,
  `pending_sends` json NOT NULL,
  `metadata` json NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_checkpoints_thread_created` (`thread_id`, `created_at`),
  KEY `idx_agent_checkpoints_run_created` (`run_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
