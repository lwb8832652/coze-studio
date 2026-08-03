CREATE TABLE IF NOT EXISTS `agent_journal_snapshots` (
  `snapshot_id` varchar(64) NOT NULL,
  `space_id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `run_id` bigint NOT NULL,
  `journal_run_id` bigint NOT NULL,
  `attempt_id` varchar(64) NOT NULL,
  `event_id` bigint NOT NULL,
  `action_id` varchar(191) NOT NULL,
  `revision` int unsigned NOT NULL DEFAULT 1,
  `content_type` varchar(32) NOT NULL,
  `status` varchar(32) NOT NULL,
  `is_fragmented` tinyint unsigned NOT NULL DEFAULT 0,
  `fragment_count` int unsigned NOT NULL DEFAULT 0,
  `visibility` varchar(16) NOT NULL,
  `error_code` varchar(64) DEFAULT NULL,
  `mime_type` varchar(191) NOT NULL,
  `encoding` varchar(32) NOT NULL,
  `compression` varchar(32) NOT NULL DEFAULT 'identity',
  `content_json` mediumblob,
  `object_key` varchar(1024) DEFAULT NULL,
  `summary_json` mediumblob,
  `summary_hash` char(64) DEFAULT NULL,
  `content_length` bigint NOT NULL DEFAULT 0,
  `content_hash` char(64) NOT NULL,
  `acl_domain` varchar(191) NOT NULL,
  `source_resource_type` varchar(64) DEFAULT NULL,
  `source_resource_id` varchar(191) DEFAULT NULL,
  `source_revision` char(64) DEFAULT NULL,
  `original_object_key` varchar(1024) DEFAULT NULL,
  `expires_at` bigint NOT NULL,
  `cleanup_state` varchar(32) NOT NULL DEFAULT 'active',
  `deleted_at` bigint DEFAULT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`snapshot_id`),
  UNIQUE KEY `uk_agent_journal_snapshots_event_revision` (`event_id`, `revision`),
  UNIQUE KEY `uk_agent_journal_snapshots_action_revision`
    (`journal_run_id`, `attempt_id`, `action_id`, `revision`),
  KEY `idx_agent_journal_snapshots_scope` (`space_id`, `thread_id`, `run_id`),
  KEY `idx_agent_journal_snapshots_attempt` (`journal_run_id`, `attempt_id`, `created_at`),
  KEY `idx_agent_journal_snapshots_hash_scope` (`space_id`, `acl_domain`, `content_hash`),
  KEY `idx_agent_journal_snapshots_cleanup`
    (`cleanup_state`, `expires_at`, `snapshot_id`),
  CONSTRAINT `chk_agent_journal_snapshots_type` CHECK (
    `content_type` IN ('document', 'terminal', 'code', 'skill', 'browser')
  ),
  CONSTRAINT `chk_agent_journal_snapshots_status` CHECK (
    `status` IN ('empty', 'loading', 'streaming', 'ready', 'error', 'no_permission')
  ),
  CONSTRAINT `chk_agent_journal_snapshots_fragmented` CHECK (`is_fragmented` IN (0, 1)),
  CONSTRAINT `chk_agent_journal_snapshots_fragment_count` CHECK (
    (`is_fragmented` = 0 AND `fragment_count` = 0)
    OR (`is_fragmented` = 1 AND `fragment_count` > 0)
  ),
  CONSTRAINT `chk_agent_journal_snapshots_compression` CHECK (
    `compression` IN ('identity')
  ),
  CONSTRAINT `chk_agent_journal_snapshots_cleanup` CHECK (
    `cleanup_state` IN ('active', 'pending', 'deleting', 'failed')
  ),
  CONSTRAINT `chk_agent_journal_snapshots_retention` CHECK (
    `expires_at` = `created_at` + 2592000000
  ),
  CONSTRAINT `chk_agent_journal_snapshots_content_json` CHECK (
    `content_json` IS NULL OR JSON_VALID(`content_json`)
  ),
  CONSTRAINT `chk_agent_journal_snapshots_summary_json` CHECK (
    `summary_json` IS NULL OR JSON_VALID(`summary_json`)
  ),
  CONSTRAINT `chk_agent_journal_snapshots_payload_storage` CHECK (
    (`is_fragmented` = 1 AND `content_json` IS NULL AND `object_key` IS NOT NULL
      AND `summary_json` IS NOT NULL AND `summary_hash` IS NOT NULL)
    OR (`is_fragmented` = 0 AND `content_json` IS NOT NULL AND `object_key` IS NULL
      AND `summary_json` IS NULL AND `summary_hash` IS NULL)
    OR (`is_fragmented` = 0 AND `content_json` IS NULL AND `object_key` IS NOT NULL
      AND `summary_json` IS NULL AND `summary_hash` IS NULL)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `agent_journal_snapshot_reservations` (
  `snapshot_id` varchar(64) NOT NULL,
  `reservation_token` varchar(64) NOT NULL,
  `space_id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `run_id` bigint NOT NULL,
  `journal_run_id` bigint NOT NULL,
  `attempt_id` varchar(64) NOT NULL,
  `action_id` varchar(191) NOT NULL,
  `revision` int unsigned NOT NULL,
  `event_id` bigint NOT NULL,
  `idempotency_key` varchar(191) NOT NULL,
  `content_hash` char(64) NOT NULL,
  `acl_domain` varchar(191) NOT NULL,
  `staging_prefix` varchar(1024) NOT NULL,
  `expires_at` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`snapshot_id`),
  UNIQUE KEY `uk_agent_journal_snapshot_reservations_token` (`reservation_token`),
  UNIQUE KEY `uk_agent_journal_snapshot_reservations_action`
    (`journal_run_id`, `attempt_id`, `action_id`, `revision`),
  KEY `idx_agent_journal_snapshot_reservations_expiry`
    (`space_id`, `expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `agent_journal_snapshot_fragments` (
  `fragment_id` varchar(64) NOT NULL,
  `snapshot_id` varchar(64) NOT NULL,
  `fragment_index` int NOT NULL,
  `kind` varchar(32) NOT NULL,
  `metadata_json` mediumblob,
  `mime_type` varchar(191) DEFAULT NULL,
  `inline_content` mediumblob,
  `object_key` varchar(1024) DEFAULT NULL,
  `byte_start` bigint NOT NULL DEFAULT 0,
  `byte_end` bigint NOT NULL DEFAULT 0,
  `size_bytes` bigint NOT NULL DEFAULT 0,
  `content_hash` char(64) NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`fragment_id`),
  UNIQUE KEY `uk_agent_journal_snapshot_fragment_index` (`snapshot_id`, `fragment_index`),
  KEY `idx_agent_journal_snapshot_fragments_object` (`object_key`(191)),
  CONSTRAINT `chk_agent_journal_snapshot_fragment_index` CHECK (`fragment_index` >= 0),
  CONSTRAINT `chk_agent_journal_snapshot_fragment_kind` CHECK (
    `kind` IN (
      'document_block', 'document_chapters', 'terminal_stdout', 'terminal_stderr',
      'code_lines', 'code_highlights', 'skill_items', 'browser_thumbnail',
      'browser_snapshot', 'browser_analysis'
    )
  ),
  CONSTRAINT `chk_agent_journal_snapshot_fragment_metadata` CHECK (
    `metadata_json` IS NULL OR JSON_VALID(`metadata_json`)
  ),
  CONSTRAINT `chk_agent_journal_snapshot_fragment_storage` CHECK (
    (`inline_content` IS NOT NULL AND `object_key` IS NULL)
    OR (`inline_content` IS NULL AND `object_key` IS NOT NULL)
    OR (`size_bytes` = 0 AND `inline_content` IS NULL AND `object_key` IS NULL)
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;

CREATE TABLE IF NOT EXISTS `agent_journal_snapshot_access_audits` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `space_id` bigint NOT NULL,
  `thread_id` bigint NOT NULL,
  `run_id` bigint NOT NULL,
  `attempt_id` varchar(64) DEFAULT NULL,
  `snapshot_id` varchar(64) NOT NULL,
  `content_type` varchar(32) DEFAULT NULL,
  `action` varchar(64) NOT NULL,
  `actor_id` bigint NOT NULL,
  `permission_result` varchar(32) NOT NULL,
  `idempotency_key` varchar(191) NOT NULL,
  `target_hash` char(64) NOT NULL,
  `trace_id` varchar(128) DEFAULT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_agent_journal_snapshot_audit_idempotency`
    (`space_id`, `snapshot_id`, `action`, `actor_id`, `idempotency_key`),
  KEY `idx_agent_journal_snapshot_audits_scope`
    (`space_id`, `thread_id`, `run_id`, `created_at`),
  CONSTRAINT `chk_agent_journal_snapshot_audit_permission` CHECK (
    `permission_result` IN ('allowed', 'denied', 'expired')
  )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
