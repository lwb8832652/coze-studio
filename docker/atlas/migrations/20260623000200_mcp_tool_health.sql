ALTER TABLE `mcp_tool_servers`
  ADD COLUMN `health_status` varchar(32) NOT NULL DEFAULT 'unknown' AFTER `tools`,
  ADD COLUMN `health_checked_at` bigint NOT NULL DEFAULT 0 AFTER `health_status`,
  ADD COLUMN `health_latency_ms` bigint NOT NULL DEFAULT 0 AFTER `health_checked_at`,
  ADD COLUMN `health_error` varchar(512) NOT NULL DEFAULT '' AFTER `health_latency_ms`;
