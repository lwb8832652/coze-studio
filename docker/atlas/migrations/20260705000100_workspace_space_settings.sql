ALTER TABLE `opencoze`.`space`
  ADD COLUMN `allow_develop` tinyint(1) NOT NULL DEFAULT 1 COMMENT "Allow member developer features" AFTER `creator_id`,
  ADD COLUMN `receive_publish` tinyint(1) NOT NULL DEFAULT 0 COMMENT "Receive external space publish" AFTER `allow_develop`;
