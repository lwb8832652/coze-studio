ALTER TABLE `mcp_tool_servers`
  ADD UNIQUE KEY `uk_mcp_tool_servers_space_name_deleted` (`space_id`, `name`, `deleted_at`);

ALTER TABLE `mcp_tool_servers`
  ADD COLUMN `creator_id` bigint NOT NULL DEFAULT 0 AFTER `space_id`,
  ADD COLUMN `source_type` varchar(32) NOT NULL DEFAULT 'custom' AFTER `creator_id`,
  ADD COLUMN `resources` json NULL AFTER `tools`,
  ADD COLUMN `prompts` json NULL AFTER `resources`,
  ADD KEY `idx_mcp_tool_servers_creator_updated` (`creator_id`, `updated_at`),
  ADD KEY `idx_mcp_tool_servers_space_source_updated` (`space_id`, `source_type`, `updated_at`);

CREATE TABLE `mcp_management_audit_events` (
  `event_id` bigint NOT NULL AUTO_INCREMENT,
  `space_id` bigint NOT NULL,
  `server_id` bigint NOT NULL,
  `actor_id` bigint NOT NULL,
  `tool_name` varchar(256) NOT NULL,
  `status` varchar(16) NOT NULL,
  `latency_ms` bigint NOT NULL DEFAULT 0,
  `error_code` varchar(64) NOT NULL DEFAULT '',
  `error_summary` varchar(256) NOT NULL DEFAULT '',
  `created_at` bigint NOT NULL,
  `completed_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`event_id`),
  KEY `idx_mcp_management_audit_scope` (`space_id`, `server_id`, `created_at`, `event_id`)
);
