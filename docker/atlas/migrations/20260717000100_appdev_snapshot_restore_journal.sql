-- Add recoverable snapshot restore journaling without rewriting the historical
-- AppDev persistence migration.
ALTER TABLE `appdev_projects`
  ADD COLUMN `restore_operation_hash` binary(32) DEFAULT NULL,
  ADD COLUMN `restore_parent_operation_hash` binary(32) DEFAULT NULL,
  ADD COLUMN `restore_snapshot_id` varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN `restore_phase` varchar(24) NOT NULL DEFAULT 'none',
  ADD COLUMN `restore_runtime_generation` bigint unsigned NOT NULL DEFAULT 0,
  ADD COLUMN `restore_restart_required` tinyint(1) NOT NULL DEFAULT 0,
  ADD COLUMN `restore_source_version` bigint unsigned NOT NULL DEFAULT 0,
  ADD COLUMN `restore_result_source_version` bigint unsigned NOT NULL DEFAULT 0,
  ADD COLUMN `restore_started_generation` bigint unsigned NOT NULL DEFAULT 0,
  ADD COLUMN `restore_safe_error_code` varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN `restore_safe_error_message` varchar(255) NOT NULL DEFAULT '',
  ADD COLUMN `restore_updated_at` datetime(6) DEFAULT NULL;

CREATE TABLE `appdev_source_object_cleanups` (
  `object_key` varchar(512) NOT NULL,
  `space_id` bigint unsigned NOT NULL,
  `project_id` varchar(64) NOT NULL,
  `restore_operation_hash` binary(32) NOT NULL,
  `source_version` bigint unsigned NOT NULL,
  `cleanup_after` datetime(6) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  PRIMARY KEY (`object_key`),
  KEY `idx_appdev_source_cleanup_due` (`cleanup_after`, `space_id`, `project_id`),
  KEY `idx_appdev_source_cleanup_project` (`space_id`, `project_id`, `source_version`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
