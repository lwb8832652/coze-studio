ALTER TABLE `agent_runs`
  ADD COLUMN `lease_owner` varchar(128) DEFAULT NULL AFTER `worker_id`,
  ADD COLUMN `lease_token` varchar(128) DEFAULT NULL AFTER `lease_owner`,
  ADD COLUMN `lease_expires_at` bigint DEFAULT NULL AFTER `lease_token`,
  ADD COLUMN `heartbeat_at` bigint DEFAULT NULL AFTER `lease_expires_at`,
  ADD COLUMN `cancel_requested_at` bigint DEFAULT NULL AFTER `heartbeat_at`,
  ADD COLUMN `execution_generation` bigint unsigned NOT NULL DEFAULT 0 AFTER `cancel_requested_at`,
  ADD KEY `idx_agent_runs_status_lease_expiry` (`status`, `lease_expires_at`, `created_at`),
  ADD KEY `idx_agent_runs_lease_owner_heartbeat` (`lease_owner`, `heartbeat_at`);
