ALTER TABLE `agent_artifacts`
  ADD COLUMN `deleted_at` BIGINT NOT NULL DEFAULT 0 AFTER `updated_at`,
  ADD KEY `idx_agent_artifacts_thread_active_created` (`thread_id`, `deleted_at`, `created_at`),
  ADD KEY `idx_agent_artifacts_run_active_created` (`run_id`, `deleted_at`, `created_at`);
