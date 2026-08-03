CREATE TABLE IF NOT EXISTS `agent_side_effect_ledger` (
  `id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `journal_run_id` bigint NOT NULL,
  `attempt_id` varchar(64) NOT NULL,
  `idempotency_key` varchar(191) NOT NULL,
  `action_kind` varchar(128) NOT NULL,
  `replay_policy` varchar(32) NOT NULL,
  `status` varchar(32) NOT NULL,
  `request_hash` char(64) NOT NULL,
  `request_summary` mediumblob,
  `external_reference_digest` char(64) DEFAULT NULL,
  `result_snapshot_id` varchar(64) DEFAULT NULL,
  `result_event_id` bigint DEFAULT NULL,
  `checkpoint_id` bigint DEFAULT NULL,
  `compensation_kind` varchar(128) DEFAULT NULL,
  `resolution_action` varchar(32) DEFAULT NULL,
  `resolution_idempotency_key` varchar(191) DEFAULT NULL,
  `resolved_at` bigint DEFAULT NULL,
  `version` bigint unsigned NOT NULL DEFAULT 1,
  `prepared_at` bigint NOT NULL,
  `executing_at` bigint DEFAULT NULL,
  `succeeded_at` bigint DEFAULT NULL,
  `failed_at` bigint DEFAULT NULL,
  `unknown_at` bigint DEFAULT NULL,
  `compensated_at` bigint DEFAULT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_agent_side_effect_ledger_identity`
    (`journal_run_id`, `attempt_id`, `idempotency_key`),
  UNIQUE KEY `uk_agent_side_effect_ledger_resolution`
    (`journal_run_id`, `resolution_idempotency_key`),
  KEY `idx_agent_side_effect_ledger_attempt`
    (`journal_run_id`, `attempt_id`, `created_at`),
  KEY `idx_agent_side_effect_ledger_status`
    (`status`, `updated_at`, `id`),
  CONSTRAINT `chk_agent_side_effect_ledger_policy` CHECK (
    `replay_policy` IN ('read_only', 'idempotent_write', 'non_replayable')
  ),
  CONSTRAINT `chk_agent_side_effect_ledger_status` CHECK (
    `status` IN ('prepared', 'executing', 'succeeded', 'failed', 'unknown', 'compensated')
  ),
  CONSTRAINT `chk_agent_side_effect_ledger_version` CHECK (`version` > 0),
  CONSTRAINT `chk_agent_side_effect_ledger_request_summary` CHECK (
    `request_summary` IS NULL OR JSON_VALID(`request_summary`)
  ),
  CONSTRAINT `chk_agent_side_effect_ledger_resolution` CHECK (
    (`resolution_action` IS NULL
      AND `resolution_idempotency_key` IS NULL AND `resolved_at` IS NULL)
    OR (`status` = 'unknown'
      AND `resolution_action` IN ('mark_succeeded', 'skip', 'retry')
      AND `resolution_idempotency_key` IS NOT NULL AND `resolved_at` IS NOT NULL)
  ),
  CONSTRAINT `chk_agent_side_effect_ledger_lifecycle` CHECK (
    (`status` = 'prepared'
      AND `executing_at` IS NULL AND `succeeded_at` IS NULL
      AND `failed_at` IS NULL AND `unknown_at` IS NULL AND `compensated_at` IS NULL)
    OR (`status` = 'executing'
      AND `executing_at` IS NOT NULL AND `succeeded_at` IS NULL
      AND `failed_at` IS NULL AND `unknown_at` IS NULL AND `compensated_at` IS NULL)
    OR (`status` = 'succeeded'
      AND `executing_at` IS NOT NULL AND `succeeded_at` IS NOT NULL
      AND `failed_at` IS NULL AND `unknown_at` IS NULL AND `compensated_at` IS NULL)
    OR (`status` = 'failed'
      AND `executing_at` IS NOT NULL AND `succeeded_at` IS NULL
      AND `failed_at` IS NOT NULL AND `unknown_at` IS NULL AND `compensated_at` IS NULL)
    OR (`status` = 'unknown'
      AND `executing_at` IS NOT NULL AND `succeeded_at` IS NULL
      AND `failed_at` IS NULL AND `unknown_at` IS NOT NULL AND `compensated_at` IS NULL)
    OR (`status` = 'compensated'
      AND `executing_at` IS NOT NULL AND `compensated_at` IS NOT NULL
      AND `compensation_kind` IS NOT NULL
      AND (`succeeded_at` IS NOT NULL OR `unknown_at` IS NOT NULL))
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
