ALTER TABLE `skill_versions`
  DROP INDEX `uk_skill_versions_skill_version`;

ALTER TABLE `skill_versions`
  ADD INDEX `idx_skill_versions_skill_version` (`skill_id`, `version`);
