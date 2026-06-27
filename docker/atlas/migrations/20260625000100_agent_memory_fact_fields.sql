ALTER TABLE `agent_thread_memories`
  ADD COLUMN `confidence` double NOT NULL DEFAULT 0 AFTER `score`,
  ADD COLUMN `source_type` varchar(64) NOT NULL DEFAULT '' AFTER `confidence`,
  ADD COLUMN `source_id` varchar(128) NOT NULL DEFAULT '' AFTER `source_type`,
  ADD COLUMN `correction_of_memory_id` bigint NOT NULL DEFAULT 0 AFTER `source_id`,
  ADD COLUMN `corrected_at` bigint NOT NULL DEFAULT 0 AFTER `correction_of_memory_id`,
  ADD KEY `idx_agent_thread_memories_source` (`source_type`, `source_id`),
  ADD KEY `idx_agent_thread_memories_correction` (`correction_of_memory_id`);
