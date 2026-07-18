ALTER TABLE `appdev_provider_executions`
  ADD COLUMN `launch_state` varchar(16) NOT NULL DEFAULT 'none' AFTER `submission_started_at`,
  ADD COLUMN `launch_operation_hash` binary(32) NULL AFTER `launch_state`,
  ADD COLUMN `launch_expires_at` datetime(6) NULL AFTER `launch_operation_hash`,
  ADD KEY `idx_appdev_provider_exec_launch` (`space_id`, `project_id`, `launch_state`, `launch_expires_at`);
