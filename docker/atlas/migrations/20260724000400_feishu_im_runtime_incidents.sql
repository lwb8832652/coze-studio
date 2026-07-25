ALTER TABLE `im_channel_configs`
  ADD COLUMN `runtime_consecutive_failures` INT NOT NULL DEFAULT 0 AFTER `runtime_error`,
  ADD COLUMN `runtime_incident_id` VARCHAR(128) NOT NULL DEFAULT '' AFTER `runtime_consecutive_failures`,
  ADD COLUMN `runtime_incident_notified_at` DATETIME(3) NULL AFTER `runtime_incident_id`,
  ADD COLUMN `runtime_recovery_notified_at` DATETIME(3) NULL AFTER `runtime_incident_notified_at`,
  ADD COLUMN `runtime_last_recovered_at` DATETIME(3) NULL AFTER `runtime_recovery_notified_at`,
  ADD KEY `idx_im_channel_runtime_incident`
    (`space_id`, `runtime_incident_id`, `deleted_at`);
