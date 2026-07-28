-- Canonical suggestions read the newest public thread messages by role.
-- Keep the role predicate inside the index so threads with dense tool/system
-- messages do not require scanning a large recent-message tail.
SET @agent_thread_message_public_role_index_exists = (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'agent_thread_messages'
    AND INDEX_NAME = 'idx_agent_thread_messages_thread_role_created'
);
SET @agent_thread_message_public_role_index_ddl = IF(
  @agent_thread_message_public_role_index_exists = 0,
  'ALTER TABLE `agent_thread_messages` ADD INDEX `idx_agent_thread_messages_thread_role_created` (`thread_id`, `role`, `created_at`, `id`)',
  'SELECT 1'
);
PREPARE agent_thread_message_public_role_index_statement
  FROM @agent_thread_message_public_role_index_ddl;
EXECUTE agent_thread_message_public_role_index_statement;
DEALLOCATE PREPARE agent_thread_message_public_role_index_statement;
