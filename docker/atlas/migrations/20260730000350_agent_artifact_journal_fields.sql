ALTER TABLE `agent_artifacts`
  ADD COLUMN `journal_run_id` BIGINT NULL AFTER `run_id`,
  ADD COLUMN `source` VARCHAR(32) NULL AFTER `preview_mode`,
  ADD COLUMN `generation_status` VARCHAR(32) NULL AFTER `source`,
  ADD COLUMN `primary_slot` TINYINT UNSIGNED NULL AFTER `generation_status`,
  ADD COLUMN `collection_id` VARCHAR(128) NULL AFTER `primary_slot`,
  ADD COLUMN `collection_order` INT UNSIGNED NULL AFTER `collection_id`,
  ADD COLUMN `detected_content_type` VARCHAR(255) NULL AFTER `collection_order`,
  ADD COLUMN `scanned_size_bytes` BIGINT NULL AFTER `detected_content_type`,
  ADD COLUMN `content_hash` CHAR(64) NULL AFTER `scanned_size_bytes`;
