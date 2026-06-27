ALTER TABLE `agent_artifact_scan_jobs`
  ADD COLUMN `worker_id` VARCHAR(128) NOT NULL DEFAULT '' AFTER `status`,
  ADD COLUMN `lease_expires_at` BIGINT NOT NULL DEFAULT 0 AFTER `available_at`,
  ADD COLUMN `started_at` BIGINT NOT NULL DEFAULT 0 AFTER `lease_expires_at`,
  ADD COLUMN `ended_at` BIGINT NOT NULL DEFAULT 0 AFTER `started_at`;

ALTER TABLE `agent_artifact_scan_jobs`
  DROP INDEX `idx_agent_artifact_scan_jobs_pending`;

ALTER TABLE `agent_artifact_scan_jobs`
  ADD KEY `idx_agent_artifact_scan_jobs_pending` (`status`, `scanner`, `available_at`, `created_at`),
  ADD KEY `idx_agent_artifact_scan_jobs_lease` (`status`, `scanner`, `lease_expires_at`, `created_at`);
