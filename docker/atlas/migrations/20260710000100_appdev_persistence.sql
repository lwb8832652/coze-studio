-- Persist AppDev project metadata while keeping source and snapshot payloads in object storage.
CREATE TABLE `appdev_projects` (
  `id` varchar(64) NOT NULL,
  `space_id` bigint unsigned NOT NULL,
  `name` varchar(128) NOT NULL,
  `description` varchar(1024) NOT NULL DEFAULT '',
  `prompt` text,
  `status` varchar(32) NOT NULL,
  `runtime_status` varchar(32) NOT NULL,
  `preview_url` varchar(2048) NOT NULL DEFAULT '',
  `last_build_status` varchar(32) NOT NULL DEFAULT '',
  `last_build_type` varchar(32) NOT NULL DEFAULT '',
  `last_build_artifact` varchar(512) NOT NULL DEFAULT '',
  `last_build_message` varchar(2048) NOT NULL DEFAULT '',
  `last_build_at` datetime(3) DEFAULT NULL,
  `source_object_key` varchar(512) NOT NULL,
  `source_version` bigint unsigned NOT NULL,
  `source_updated_at` datetime(3) NOT NULL,
  `creator_id` bigint unsigned NOT NULL,
  `creator_name` varchar(128) NOT NULL DEFAULT '',
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_appdev_projects_space_updated` (`space_id`, `updated_at`),
  KEY `idx_appdev_projects_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `appdev_project_snapshots` (
  `id` varchar(64) NOT NULL,
  `space_id` bigint unsigned NOT NULL,
  `project_id` varchar(64) NOT NULL,
  `label` varchar(128) NOT NULL,
  `source_object_key` varchar(512) NOT NULL,
  `source_version` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_appdev_snapshots_project` (`space_id`, `project_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `appdev_runtime_sessions` (
  `space_id` bigint unsigned NOT NULL,
  `project_id` varchar(64) NOT NULL,
  `status` varchar(32) NOT NULL,
  `preview_url` varchar(2048) NOT NULL DEFAULT '',
  `last_keep_alive_at` datetime(3) DEFAULT NULL,
  `runtime_message` varchar(1024) NOT NULL DEFAULT '',
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`space_id`, `project_id`),
  KEY `idx_appdev_runtime_updated` (`updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `appdev_chat_sessions` (
  `space_id` bigint unsigned NOT NULL,
  `project_id` varchar(64) NOT NULL,
  `session_id` varchar(64) NOT NULL,
  `request_id` varchar(64) NOT NULL,
  `running` tinyint(1) NOT NULL DEFAULT 0,
  `updated_at` datetime(3) NOT NULL,
  PRIMARY KEY (`space_id`, `project_id`),
  KEY `idx_appdev_chat_sessions_running` (`running`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE `appdev_chat_messages` (
  `id` varchar(128) NOT NULL,
  `space_id` bigint unsigned NOT NULL,
  `project_id` varchar(64) NOT NULL,
  `session_id` varchar(64) NOT NULL,
  `request_id` varchar(64) NOT NULL,
  `message_type` varchar(32) NOT NULL,
  `role` varchar(32) NOT NULL DEFAULT '',
  `title` varchar(256) NOT NULL DEFAULT '',
  `content` mediumtext NOT NULL,
  `attachments_json` json DEFAULT NULL,
  `created_at` datetime(3) NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_appdev_chat_messages_project` (`space_id`, `project_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
