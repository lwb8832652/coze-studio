ALTER TABLE `agent_checkpoints`
  ADD COLUMN `runtime_type` varchar(32) NOT NULL DEFAULT 'legacy' AFTER `checkpoint_ns`,
  ADD COLUMN `runtime_key` varchar(255) NOT NULL DEFAULT '' AFTER `runtime_type`,
  ADD COLUMN `envelope_version` int NOT NULL DEFAULT 0 AFTER `runtime_key`,
  ADD COLUMN `runtime_deleted_at` bigint NOT NULL DEFAULT 0 AFTER `envelope_version`,
  ADD KEY `idx_agent_checkpoints_runtime_key`
    (`thread_id`, `run_id`, `runtime_type`, `runtime_key`, `created_at`);
