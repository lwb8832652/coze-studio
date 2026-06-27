ALTER TABLE `agent_token_usage`
  ADD COLUMN `usage_key` varchar(64)
    GENERATED ALWAYS AS (
      NULLIF(JSON_UNQUOTE(JSON_EXTRACT(`metadata`, '$.idempotency_key')), '')
    ) STORED,
  ADD UNIQUE KEY `uk_agent_token_usage_idempotency` (`run_id`, `usage_key`);
