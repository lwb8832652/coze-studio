CREATE TABLE IF NOT EXISTS `agent_run_attempts` (
  `id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `journal_run_id` bigint NOT NULL,
  `execution_run_id` bigint NOT NULL,
  `attempt_id` varchar(64) NOT NULL,
  `ordinal` int unsigned NOT NULL,
  `status` varchar(32) NOT NULL,
  `active_slot` tinyint unsigned DEFAULT NULL,
  `next_sequence` bigint unsigned NOT NULL DEFAULT 1,
  `last_committed_sequence` bigint unsigned NOT NULL DEFAULT 0,
  `source_checkpoint_id` bigint DEFAULT NULL,
  `source_attempt_id` varchar(64) DEFAULT NULL,
  `recovery_idempotency_key` varchar(191) DEFAULT NULL,
  `enrollment_version` varchar(32) NOT NULL,
  `snapshots_enabled` tinyint unsigned NOT NULL DEFAULT 0,
  `projection_state` varchar(16) NOT NULL DEFAULT 'healthy',
  `projection_degraded_at` bigint DEFAULT NULL,
  `trace_id` varchar(128) DEFAULT NULL,
  `terminal_event_id` bigint DEFAULT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  `started_at` bigint DEFAULT NULL,
  `ended_at` bigint DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_agent_run_attempts_execution` (`execution_run_id`),
  UNIQUE KEY `uk_agent_run_attempts_identity` (`journal_run_id`, `attempt_id`),
  UNIQUE KEY `uk_agent_run_attempts_ordinal` (`journal_run_id`, `ordinal`),
  UNIQUE KEY `uk_agent_run_attempts_active` (`journal_run_id`, `active_slot`),
  UNIQUE KEY `uk_agent_run_attempts_recovery_key`
    (`journal_run_id`, `recovery_idempotency_key`),
  KEY `idx_agent_run_attempts_thread_created` (`thread_id`, `created_at`),
  CONSTRAINT `chk_agent_run_attempts_ordinal` CHECK (`ordinal` > 0),
  CONSTRAINT `chk_agent_run_attempts_status` CHECK (
    `status` IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'timed_out')
  ),
  CONSTRAINT `chk_agent_run_attempts_active_slot` CHECK (
      (`status` IN ('pending', 'running') AND `active_slot` IS NOT NULL AND `active_slot` = 1)
    OR (`status` IN ('completed', 'failed', 'cancelled', 'timed_out') AND `active_slot` IS NULL)
  ),
  CONSTRAINT `chk_agent_run_attempts_sequence` CHECK (
    `next_sequence` > 0
    AND `last_committed_sequence` < `next_sequence`
  ),
  CONSTRAINT `chk_agent_run_attempts_snapshots` CHECK (`snapshots_enabled` IN (0, 1)),
  CONSTRAINT `chk_agent_run_attempts_projection` CHECK (
    (`projection_state` = 'degraded' AND `projection_degraded_at` IS NOT NULL)
    OR (`projection_state` IN ('healthy', 'disabled') AND `projection_degraded_at` IS NULL)
  ),
  CONSTRAINT `chk_agent_run_attempts_lifecycle` CHECK (
    (`status` = 'pending' AND `started_at` IS NULL AND `ended_at` IS NULL AND `terminal_event_id` IS NULL)
    OR (`status` = 'running' AND `started_at` IS NOT NULL AND `ended_at` IS NULL AND `terminal_event_id` IS NULL)
    OR (`status` IN ('completed', 'failed', 'cancelled', 'timed_out')
      AND `started_at` IS NOT NULL AND `ended_at` IS NOT NULL AND `terminal_event_id` IS NOT NULL)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
