ALTER TABLE `skill_versions`
  MODIFY COLUMN `declaration` json NULL;

ALTER TABLE `skill_versions`
  ADD COLUMN `skill_md` text NULL AFTER `version`;

ALTER TABLE `skill_versions`
  ADD COLUMN `input_schema` json NULL AFTER `skill_md`;

ALTER TABLE `skill_versions`
  ADD COLUMN `output_schema` json NULL AFTER `input_schema`;

ALTER TABLE `skill_versions`
  ADD COLUMN `executor` json NULL AFTER `output_schema`;

ALTER TABLE `skill_versions`
  ADD COLUMN `permissions` json NULL AFTER `executor`;

CREATE TABLE IF NOT EXISTS `skill_resources` (
  `id` bigint NOT NULL,
  `skill_id` bigint NOT NULL,
  `version_id` bigint NOT NULL,
  `path` varchar(512) NOT NULL,
  `content` longblob NOT NULL,
  `size` bigint NOT NULL,
  `sha256` varchar(64) NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_skill_resources_version_path` (`version_id`, `path`),
  KEY `idx_skill_resources_skill_version` (`skill_id`, `version_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
