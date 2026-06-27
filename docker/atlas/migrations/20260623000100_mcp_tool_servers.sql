CREATE TABLE IF NOT EXISTS `mcp_tool_servers` (
  `server_id` bigint NOT NULL,
  `space_id` bigint NOT NULL,
  `name` varchar(128) NOT NULL,
  `description` varchar(512) NOT NULL,
  `server_type` varchar(64) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 0,
  `config` json NOT NULL,
  `auth` json NOT NULL,
  `tools` json NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  `deleted_at` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`server_id`),
  KEY `idx_mcp_tool_servers_space_updated` (`space_id`, `updated_at`),
  KEY `idx_mcp_tool_servers_space_enabled` (`space_id`, `enabled`, `deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
