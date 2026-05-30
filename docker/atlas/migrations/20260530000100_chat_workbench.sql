CREATE TABLE IF NOT EXISTS `skills` (
  `id` bigint NOT NULL,
  `space_id` bigint NOT NULL,
  `name` varchar(255) NOT NULL,
  `description` text NOT NULL,
  `type` varchar(32) NOT NULL,
  `version` varchar(64) NOT NULL,
  `enabled` tinyint(1) NOT NULL DEFAULT 1,
  `input_schema` json NOT NULL,
  `output_schema` json NOT NULL,
  `executor` json NOT NULL,
  `permissions` json NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_skills_space_type` (`space_id`, `type`),
  KEY `idx_skills_space_enabled` (`space_id`, `enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `skill_versions` (
  `id` bigint NOT NULL,
  `skill_id` bigint NOT NULL,
  `version` varchar(64) NOT NULL,
  `declaration` json NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_skill_versions_skill_version` (`skill_id`, `version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `chat_tasks` (
  `id` bigint NOT NULL,
  `space_id` bigint NOT NULL,
  `creator_id` bigint NOT NULL,
  `conversation_id` bigint NOT NULL DEFAULT 0,
  `message_id` bigint NOT NULL DEFAULT 0,
  `skill_id` bigint NOT NULL DEFAULT 0,
  `title` varchar(255) NOT NULL,
  `status` varchar(32) NOT NULL,
  `progress` int NOT NULL DEFAULT 0,
  `input` json NULL,
  `result` json NULL,
  `error` text NOT NULL,
  `created_at` bigint NOT NULL,
  `updated_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_chat_tasks_space_status` (`space_id`, `status`),
  KEY `idx_chat_tasks_creator_updated` (`creator_id`, `updated_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `chat_task_attempts` (
  `id` bigint NOT NULL,
  `task_id` bigint NOT NULL,
  `attempt_no` int NOT NULL,
  `status` varchar(32) NOT NULL,
  `started_at` bigint NOT NULL DEFAULT 0,
  `ended_at` bigint NOT NULL DEFAULT 0,
  `runtime` varchar(64) NOT NULL DEFAULT '',
  `error` text NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_chat_task_attempts_task_attempt` (`task_id`, `attempt_no`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `chat_task_events` (
  `id` bigint NOT NULL,
  `task_id` bigint NOT NULL,
  `event_type` varchar(64) NOT NULL,
  `payload` json NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_chat_task_events_task_created` (`task_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
