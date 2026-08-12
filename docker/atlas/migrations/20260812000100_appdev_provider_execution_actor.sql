ALTER TABLE `appdev_provider_executions`
  ADD COLUMN `actor_user_id` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `project_id`;
