ALTER TABLE `agent_run_events`
  ADD COLUMN `journal_event_type` varchar(128) DEFAULT NULL AFTER `event_type`,
  ADD COLUMN `journal_payload` json DEFAULT NULL AFTER `payload`;
