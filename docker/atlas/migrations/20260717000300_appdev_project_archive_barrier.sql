ALTER TABLE `appdev_projects`
  ADD COLUMN `archive_state` varchar(16) NOT NULL DEFAULT 'none' AFTER `source_updated_at`,
  ADD COLUMN `archive_intent_version` bigint unsigned NOT NULL DEFAULT 0 AFTER `archive_state`,
  ADD COLUMN `archive_operation_hash` binary(32) DEFAULT NULL AFTER `archive_intent_version`,
  ADD COLUMN `archive_source_version` bigint unsigned NOT NULL DEFAULT 0 AFTER `archive_operation_hash`,
  ADD COLUMN `archive_runtime_generation` bigint unsigned NOT NULL DEFAULT 0 AFTER `archive_source_version`,
  ADD COLUMN `archive_started_at` datetime(6) DEFAULT NULL AFTER `archive_runtime_generation`,
  ADD COLUMN `archive_completed_at` datetime(6) DEFAULT NULL AFTER `archive_started_at`,
  ADD KEY `idx_appdev_projects_archive` (`archive_state`, `archive_started_at`);
