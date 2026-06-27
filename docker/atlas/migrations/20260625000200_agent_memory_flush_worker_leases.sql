ALTER TABLE `agent_memory_flush_jobs`
  ADD COLUMN `worker_id` varchar(128) NOT NULL DEFAULT '' AFTER `attempt_count`,
  ADD COLUMN `lease_expires_at` bigint NOT NULL DEFAULT 0 AFTER `available_at`,
  ADD COLUMN `started_at` bigint NOT NULL DEFAULT 0 AFTER `lease_expires_at`,
  ADD COLUMN `ended_at` bigint NOT NULL DEFAULT 0 AFTER `started_at`,
  ADD KEY `idx_agent_memory_flush_worker` (`worker_id`);
