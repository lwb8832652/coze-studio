ALTER TABLE `agent_runs`
  ADD COLUMN `parent_run_id` bigint NOT NULL DEFAULT 0 AFTER `thread_id`,
  ADD COLUMN `run_kind` varchar(32) NOT NULL DEFAULT 'task' AFTER `assistant_id`,
  ADD KEY `idx_agent_runs_parent_created` (`parent_run_id`, `created_at`),
  ADD KEY `idx_agent_runs_thread_kind` (`thread_id`, `run_kind`, `created_at`);
