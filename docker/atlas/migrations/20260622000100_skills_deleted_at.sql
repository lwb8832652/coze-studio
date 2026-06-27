ALTER TABLE `skills`
  ADD COLUMN `deleted_at` bigint NOT NULL DEFAULT 0 AFTER `updated_at`,
  ADD KEY `idx_skills_space_deleted` (`space_id`, `deleted_at`);
