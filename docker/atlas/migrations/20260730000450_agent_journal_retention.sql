ALTER TABLE `agent_journal_snapshots`
  DROP CHECK `chk_agent_journal_snapshots_payload_storage`,
  ADD COLUMN `cleanup_claim_token` varchar(64) DEFAULT NULL AFTER `deleted_at`,
  ADD COLUMN `cleanup_claim_expires_at` bigint DEFAULT NULL AFTER `cleanup_claim_token`,
  ADD COLUMN `cleanup_attempt_count` int unsigned NOT NULL DEFAULT 0 AFTER `cleanup_claim_expires_at`,
  ADD COLUMN `cleanup_last_error_code` varchar(64) DEFAULT NULL AFTER `cleanup_attempt_count`,
  ADD KEY `idx_agent_journal_snapshots_claim`
    (`cleanup_state`, `cleanup_claim_expires_at`, `snapshot_id`),
  ADD CONSTRAINT `chk_agent_journal_snapshots_payload_storage` CHECK (
    (`cleanup_state` <> 'active' AND `content_json` IS NULL AND `summary_json` IS NULL)
    OR (`is_fragmented` = 1 AND `content_json` IS NULL AND `object_key` IS NOT NULL
      AND `summary_json` IS NOT NULL AND `summary_hash` IS NOT NULL)
    OR (`is_fragmented` = 0 AND `content_json` IS NOT NULL AND `object_key` IS NULL
      AND `summary_json` IS NULL AND `summary_hash` IS NULL)
    OR (`is_fragmented` = 0 AND `content_json` IS NULL AND `object_key` IS NOT NULL
      AND `summary_json` IS NULL AND `summary_hash` IS NULL)
  );

ALTER TABLE `agent_journal_snapshot_reservations`
  ADD COLUMN `cleanup_claim_token` varchar(64) DEFAULT NULL AFTER `expires_at`,
  ADD COLUMN `cleanup_claim_expires_at` bigint DEFAULT NULL AFTER `cleanup_claim_token`,
  ADD COLUMN `cleanup_attempt_count` int unsigned NOT NULL DEFAULT 0 AFTER `cleanup_claim_expires_at`,
  ADD COLUMN `cleanup_last_error_code` varchar(64) DEFAULT NULL AFTER `cleanup_attempt_count`,
  ADD KEY `idx_agent_journal_snapshot_reservations_claim`
    (`cleanup_claim_expires_at`, `expires_at`, `snapshot_id`);

ALTER TABLE `agent_run_events`
  ADD KEY `idx_agent_run_events_journal_retention` (`created_at`, `id`);

ALTER TABLE `agent_checkpoints`
  ADD KEY `idx_agent_checkpoints_journal_retention` (`created_at`, `id`);

ALTER TABLE `agent_side_effect_ledger`
  ADD KEY `idx_agent_side_effect_ledger_retention` (`created_at`, `id`);

ALTER TABLE `agent_run_attempts`
  ADD KEY `idx_agent_run_attempts_retention` (`ended_at`, `id`);
