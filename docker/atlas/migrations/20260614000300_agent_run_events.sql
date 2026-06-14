CREATE TABLE IF NOT EXISTS `agent_run_events` (
  `id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `run_id` bigint NOT NULL,
  `event_type` varchar(128) NOT NULL,
  `payload` json NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_run_events_run_created` (`run_id`, `created_at`),
  KEY `idx_agent_run_events_thread_created` (`thread_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
