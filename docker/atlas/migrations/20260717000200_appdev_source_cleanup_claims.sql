-- Fence source-object cleanup against snapshot restore commits. Cleanup uses
-- database-clock leases and retains a bounded deletion tombstone so stale
-- restore attempts cannot reintroduce an object after external deletion.
ALTER TABLE `appdev_source_object_cleanups`
  ADD COLUMN `cleanup_state` varchar(16) NOT NULL DEFAULT 'pending' AFTER `cleanup_after`,
  ADD COLUMN `claim_token_hash` binary(32) DEFAULT NULL AFTER `cleanup_state`,
  ADD COLUMN `claim_expires_at` datetime(6) DEFAULT NULL AFTER `claim_token_hash`,
  ADD COLUMN `attempt_count` int unsigned NOT NULL DEFAULT 0 AFTER `claim_expires_at`,
  ADD COLUMN `last_attempt_at` datetime(6) DEFAULT NULL AFTER `attempt_count`,
  ADD COLUMN `deleted_at` datetime(6) DEFAULT NULL AFTER `last_attempt_at`,
  ADD COLUMN `tombstone_expires_at` datetime(6) DEFAULT NULL AFTER `deleted_at`,
  ADD COLUMN `updated_at` datetime(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) AFTER `created_at`,
  ADD KEY `idx_appdev_source_cleanup_claim` (`cleanup_state`, `cleanup_after`, `claim_expires_at`, `space_id`, `project_id`),
  ADD KEY `idx_appdev_source_cleanup_tombstone` (`cleanup_state`, `tombstone_expires_at`);
