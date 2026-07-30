ALTER TABLE `agent_run_events`
  ADD UNIQUE KEY `uk_agent_run_events_attempt_sequence`
    (`journal_run_id`, `attempt_id`, `sequence`),
  ADD UNIQUE KEY `uk_agent_run_events_attempt_idempotency`
    (`journal_run_id`, `attempt_id`, `idempotency_key`),
  ADD UNIQUE KEY `uk_agent_run_events_action_phase`
    (`journal_run_id`, `attempt_id`, `action_id`, `phase`),
  ADD KEY `idx_agent_run_events_parent` (`parent_event_id`);
