ALTER TABLE `agent_thread_memories`
  ADD COLUMN `deleted_at` bigint NOT NULL DEFAULT 0 AFTER `updated_at`,
  ADD KEY `idx_agent_thread_memories_deleted` (`deleted_at`);
