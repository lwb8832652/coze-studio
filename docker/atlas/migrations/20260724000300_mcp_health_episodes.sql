ALTER TABLE `mcp_tool_servers`
  ADD COLUMN `health_consecutive_failures` int NOT NULL DEFAULT 0 AFTER `health_error`,
  ADD COLUMN `health_incident_id` varchar(128) NOT NULL DEFAULT '' AFTER `health_consecutive_failures`,
  ADD COLUMN `health_incident_opened_at` bigint NOT NULL DEFAULT 0 AFTER `health_incident_id`,
  ADD COLUMN `health_last_recovered_at` bigint NOT NULL DEFAULT 0 AFTER `health_incident_opened_at`,
  ADD KEY `idx_mcp_tool_servers_health_incident`
    (`space_id`, `health_incident_id`, `deleted_at`);
