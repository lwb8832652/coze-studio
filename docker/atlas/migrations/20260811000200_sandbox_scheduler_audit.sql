CREATE TABLE `sandbox_scheduler_audit_events` (
  `event_id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `actor_user_id` BIGINT UNSIGNED NOT NULL,
  `request_id` VARCHAR(128) NOT NULL,
  `action` VARCHAR(64) NOT NULL,
  `metadata_json` JSON NOT NULL,
  `created_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`event_id`),
  CONSTRAINT `chk_sandbox_scheduler_audit_action`
    CHECK (`action` IN ('scheduler_settings.update', 'scheduler_settings.update_failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
