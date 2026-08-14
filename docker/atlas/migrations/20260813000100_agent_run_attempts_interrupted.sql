ALTER TABLE `agent_run_attempts`
  DROP CHECK `chk_agent_run_attempts_status`,
  DROP CHECK `chk_agent_run_attempts_active_slot`,
  DROP CHECK `chk_agent_run_attempts_lifecycle`,
  ADD CONSTRAINT `chk_agent_run_attempts_status` CHECK (
    `status` IN ('pending', 'running', 'completed', 'failed', 'cancelled', 'timed_out', 'interrupted')
  ),
  ADD CONSTRAINT `chk_agent_run_attempts_active_slot` CHECK (
      (`status` IN ('pending', 'running') AND `active_slot` IS NOT NULL AND `active_slot` = 1)
    OR (`status` IN ('completed', 'failed', 'cancelled', 'timed_out', 'interrupted') AND `active_slot` IS NULL)
  ),
  ADD CONSTRAINT `chk_agent_run_attempts_lifecycle` CHECK (
    (`status` = 'pending' AND `started_at` IS NULL AND `ended_at` IS NULL AND `terminal_event_id` IS NULL)
    OR (`status` = 'running' AND `started_at` IS NOT NULL AND `ended_at` IS NULL AND `terminal_event_id` IS NULL)
    OR (`status` IN ('completed', 'failed', 'cancelled', 'timed_out', 'interrupted')
      AND `started_at` IS NOT NULL AND `ended_at` IS NOT NULL AND `terminal_event_id` IS NOT NULL)
  );
