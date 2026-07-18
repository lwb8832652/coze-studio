CREATE TABLE `plugin_code_drafts` (
  `plugin_id` bigint unsigned NOT NULL,
  `space_id` bigint unsigned NOT NULL,
  `runtime` varchar(16) NOT NULL,
  `entry_file` varchar(512) NOT NULL,
  `source_bundle_ref` char(64) NOT NULL,
  `input_schema_json` json NOT NULL DEFAULT (JSON_OBJECT('type', 'object', 'properties', JSON_OBJECT())),
  `output_schema_json` json NOT NULL DEFAULT (JSON_OBJECT('type', 'object', 'properties', JSON_OBJECT())),
  `revision` bigint unsigned NOT NULL,
  `last_debugged_revision` bigint unsigned NOT NULL DEFAULT 0,
  `created_at` datetime(6) NOT NULL,
  `updated_at` datetime(6) NOT NULL,
  PRIMARY KEY (`plugin_id`),
  KEY `idx_plugin_code_drafts_space` (`space_id`, `updated_at`),
  CONSTRAINT `fk_plugin_code_drafts_plugin_draft` FOREIGN KEY (`plugin_id`) REFERENCES `plugin_draft` (`id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `plugin_code_draft_files` (
  `plugin_id` bigint unsigned NOT NULL,
  `path` varchar(512) NOT NULL,
  `content` longblob NOT NULL,
  `size` bigint unsigned NOT NULL,
  `sha256` char(64) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  `updated_at` datetime(6) NOT NULL,
  PRIMARY KEY (`plugin_id`, `path`),
  CONSTRAINT `fk_plugin_code_draft_files_draft` FOREIGN KEY (`plugin_id`) REFERENCES `plugin_code_drafts` (`plugin_id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `plugin_code_versions` (
  `plugin_id` bigint unsigned NOT NULL,
  `version` varchar(64) NOT NULL,
  `space_id` bigint unsigned NOT NULL,
  `runtime` varchar(16) NOT NULL,
  `entry_file` varchar(512) NOT NULL,
  `source_bundle_ref` char(64) NOT NULL,
  `input_schema_json` json NOT NULL DEFAULT (JSON_OBJECT('type', 'object', 'properties', JSON_OBJECT())),
  `output_schema_json` json NOT NULL DEFAULT (JSON_OBJECT('type', 'object', 'properties', JSON_OBJECT())),
  `source_revision` bigint unsigned NOT NULL,
  `created_by` bigint unsigned NOT NULL,
  `created_at` datetime(6) NOT NULL,
  PRIMARY KEY (`plugin_id`, `version`),
  KEY `idx_plugin_code_versions_space` (`space_id`, `created_at`),
  CONSTRAINT `fk_plugin_code_versions_draft` FOREIGN KEY (`plugin_id`) REFERENCES `plugin_code_drafts` (`plugin_id`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `plugin_code_version_files` (
  `plugin_id` bigint unsigned NOT NULL,
  `version` varchar(64) NOT NULL,
  `path` varchar(512) NOT NULL,
  `content` longblob NOT NULL,
  `size` bigint unsigned NOT NULL,
  `sha256` char(64) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  PRIMARY KEY (`plugin_id`, `version`, `path`),
  CONSTRAINT `fk_plugin_code_version_files_version` FOREIGN KEY (`plugin_id`, `version`) REFERENCES `plugin_code_versions` (`plugin_id`, `version`) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
