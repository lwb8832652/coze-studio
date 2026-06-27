ALTER TABLE `agent_thread_memories`
  ADD COLUMN `source_key` varchar(256)
    GENERATED ALWAYS AS (
      CASE
        WHEN `source_type` <> '' AND `source_id` <> ''
        THEN CONCAT(`source_type`, ':', `source_id`)
        ELSE NULL
      END
    ) STORED,
  ADD UNIQUE KEY `uk_agent_thread_memories_source_key` (`thread_id`, `source_key`);
