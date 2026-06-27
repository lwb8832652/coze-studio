CREATE TABLE IF NOT EXISTS `agent_mcp_runtime_audit_events` (
  `id` BIGINT NOT NULL,
  `space_id` BIGINT NOT NULL,
  `thread_id` BIGINT NOT NULL,
  `run_id` BIGINT NOT NULL,
  `server_id` BIGINT NOT NULL,
  `runtime_tool_name` VARCHAR(128) NOT NULL,
  `event_type` VARCHAR(64) NOT NULL,
  `error_code` VARCHAR(64) NOT NULL DEFAULT '',
  `elapsed_ms` BIGINT NOT NULL DEFAULT 0,
  `output_bytes` BIGINT NOT NULL DEFAULT 0,
  `created_at` BIGINT NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_agent_mcp_runtime_audit_space_created` (`space_id`, `created_at`),
  KEY `idx_agent_mcp_runtime_audit_thread_created` (`thread_id`, `created_at`),
  KEY `idx_agent_mcp_runtime_audit_run_created` (`run_id`, `created_at`),
  KEY `idx_agent_mcp_runtime_audit_server_created` (`server_id`, `created_at`),
  KEY `idx_agent_mcp_runtime_audit_event_created` (`event_type`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
