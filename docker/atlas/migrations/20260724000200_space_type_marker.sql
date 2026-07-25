SET @space_type_marker_exists = (
  SELECT COUNT(*)
  FROM `information_schema`.`COLUMNS`
  WHERE `TABLE_SCHEMA` = DATABASE()
    AND `TABLE_NAME` = 'space'
    AND `COLUMN_NAME` = 'space_type'
);

SET @space_type_marker_ddl = IF(
  @space_type_marker_exists = 0,
  'ALTER TABLE `space` ADD COLUMN `space_type` tinyint NOT NULL DEFAULT 0 COMMENT ''Space Type: 0.unknown 1.personal 2.team'' AFTER `creator_id`',
  'SELECT 1'
);

PREPARE space_type_marker_statement FROM @space_type_marker_ddl;
EXECUTE space_type_marker_statement;
DEALLOCATE PREPARE space_type_marker_statement;

-- Historical spaces do not have an immutable type marker. Keep them unknown so
-- workspace membership notifications only emit after an explicit team marker is
-- written by new creation paths or a future audited data repair.
