ALTER TABLE `agent_artifacts`
  ADD CONSTRAINT `chk_agent_artifacts_source` CHECK (
    `source` IN ('agent_generated', 'user_upload', 'tool_output', 'external_reference')
  ),
  ADD CONSTRAINT `chk_agent_artifacts_generation_status` CHECK (
    `generation_status` IN ('processing', 'ready', 'failed', 'expired', 'blocked')
  ),
  ADD CONSTRAINT `chk_agent_artifacts_primary_slot` CHECK (
    `primary_slot` IS NULL OR `primary_slot` = 1
  ),
  ADD CONSTRAINT `chk_agent_artifacts_collection` CHECK (
    (`collection_id` IS NULL AND `collection_order` IS NULL)
    OR (
      `collection_id` IS NOT NULL
      AND `collection_order` IS NOT NULL
      AND `collection_order` < 100
    )
  ),
  ADD CONSTRAINT `chk_agent_artifacts_scanned_metadata` CHECK (
    (`detected_content_type` IS NULL AND `scanned_size_bytes` IS NULL AND `content_hash` IS NULL)
    OR (
      `detected_content_type` IS NOT NULL
      AND `scanned_size_bytes` IS NOT NULL
      AND `scanned_size_bytes` > 0
      AND `content_hash` REGEXP '^[0-9a-f]{64}$'
    )
  ),
  ADD UNIQUE KEY `uk_agent_artifacts_primary` (`journal_run_id`, `primary_slot`),
  ADD UNIQUE KEY `uk_agent_artifacts_collection_order` (`journal_run_id`, `collection_id`, `collection_order`),
  ADD KEY `idx_agent_artifacts_journal_run_created` (`journal_run_id`, `created_at`),
  ADD KEY `idx_agent_artifacts_collection` (`thread_id`, `journal_run_id`, `collection_id`, `collection_order`, `id`);
