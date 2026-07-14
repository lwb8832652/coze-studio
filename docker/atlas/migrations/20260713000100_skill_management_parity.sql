ALTER TABLE `skills`
  ADD COLUMN `icon_uri` varchar(1024) NOT NULL DEFAULT '' AFTER `permissions`,
  ADD COLUMN `usage_scenarios` text NOT NULL DEFAULT ('') AFTER `icon_uri`,
  ADD COLUMN `development_thread_id` bigint NOT NULL DEFAULT 0 AFTER `usage_scenarios`,
  ADD KEY `idx_skills_development_thread` (`development_thread_id`);
