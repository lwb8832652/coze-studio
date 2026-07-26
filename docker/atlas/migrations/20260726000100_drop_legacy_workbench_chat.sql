-- Legacy WorkbenchChat rows and metadata must be backed up and purged first.
-- Every DDL step is existence-guarded so an interrupted apply can be retried.
DROP PROCEDURE IF EXISTS `migrate_20260726000100_legacy_workbench_chat`;

DELIMITER //
CREATE PROCEDURE `migrate_20260726000100_legacy_workbench_chat`()
BEGIN
  DECLARE object_exists BIGINT DEFAULT 0;

  SELECT COUNT(*) INTO object_exists
  FROM `information_schema`.`tables`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'agent_threads';
  IF object_exists = 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'agent_threads must exist before legacy WorkbenchChat cleanup';
  END IF;

  SELECT COUNT(*) INTO object_exists
  FROM `information_schema`.`tables`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'chat_tasks';
  IF object_exists > 0 THEN
    SET @legacy_workbench_sql =
      'SELECT EXISTS(SELECT 1 FROM `chat_tasks` LIMIT 1) INTO @legacy_workbench_rows';
    PREPARE legacy_workbench_stmt FROM @legacy_workbench_sql;
    EXECUTE legacy_workbench_stmt;
    DEALLOCATE PREPARE legacy_workbench_stmt;
    IF COALESCE(@legacy_workbench_rows, 0) <> 0 THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'chat_tasks must be backed up and purged before migration';
    END IF;
  END IF;

  SELECT COUNT(*) INTO object_exists
  FROM `information_schema`.`tables`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'chat_task_attempts';
  IF object_exists > 0 THEN
    SET @legacy_workbench_sql =
      'SELECT EXISTS(SELECT 1 FROM `chat_task_attempts` LIMIT 1) INTO @legacy_workbench_rows';
    PREPARE legacy_workbench_stmt FROM @legacy_workbench_sql;
    EXECUTE legacy_workbench_stmt;
    DEALLOCATE PREPARE legacy_workbench_stmt;
    IF COALESCE(@legacy_workbench_rows, 0) <> 0 THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'chat_task_attempts must be backed up and purged before migration';
    END IF;
  END IF;

  SELECT COUNT(*) INTO object_exists
  FROM `information_schema`.`tables`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'chat_task_events';
  IF object_exists > 0 THEN
    SET @legacy_workbench_sql =
      'SELECT EXISTS(SELECT 1 FROM `chat_task_events` LIMIT 1) INTO @legacy_workbench_rows';
    PREPARE legacy_workbench_stmt FROM @legacy_workbench_sql;
    EXECUTE legacy_workbench_stmt;
    DEALLOCATE PREPARE legacy_workbench_stmt;
    IF COALESCE(@legacy_workbench_rows, 0) <> 0 THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'chat_task_events must be backed up and purged before migration';
    END IF;
  END IF;

  SELECT COUNT(*) INTO object_exists
  FROM `information_schema`.`columns`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'agent_threads'
    AND `column_name` = 'legacy_task_id';
  IF object_exists > 0 THEN
    SET @legacy_workbench_sql =
      'SELECT EXISTS(SELECT 1 FROM `agent_threads` WHERE `legacy_task_id` <> 0 LIMIT 1) INTO @legacy_workbench_rows';
    PREPARE legacy_workbench_stmt FROM @legacy_workbench_sql;
    EXECUTE legacy_workbench_stmt;
    DEALLOCATE PREPARE legacy_workbench_stmt;
    IF COALESCE(@legacy_workbench_rows, 0) <> 0 THEN
      SIGNAL SQLSTATE '45000'
        SET MESSAGE_TEXT = 'agent_threads.legacy_task_id must be cleared before migration';
    END IF;
  END IF;

  SET @legacy_workbench_sql =
    'SELECT EXISTS(SELECT 1 FROM `agent_threads` WHERE CASE WHEN JSON_VALID(`metadata`) THEN JSON_CONTAINS_PATH(`metadata`, ''one'', ''$.legacy_task_id'') ELSE 0 END = 1 LIMIT 1) INTO @legacy_workbench_rows';
  PREPARE legacy_workbench_stmt FROM @legacy_workbench_sql;
  EXECUTE legacy_workbench_stmt;
  DEALLOCATE PREPARE legacy_workbench_stmt;
  IF COALESCE(@legacy_workbench_rows, 0) <> 0 THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'agent_threads metadata legacy_task_id must be cleared before migration';
  END IF;

  DROP TABLE IF EXISTS `chat_task_events`;
  DROP TABLE IF EXISTS `chat_task_attempts`;
  DROP TABLE IF EXISTS `chat_tasks`;

  SELECT COUNT(*) INTO object_exists
  FROM `information_schema`.`statistics`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'agent_threads'
    AND `index_name` = 'idx_agent_threads_legacy_task';
  IF object_exists > 0 THEN
    ALTER TABLE `agent_threads` DROP INDEX `idx_agent_threads_legacy_task`;
  END IF;

  SELECT COUNT(*) INTO object_exists
  FROM `information_schema`.`columns`
  WHERE `table_schema` = DATABASE()
    AND `table_name` = 'agent_threads'
    AND `column_name` = 'legacy_task_id';
  IF object_exists > 0 THEN
    ALTER TABLE `agent_threads` DROP COLUMN `legacy_task_id`;
  END IF;
END//
DELIMITER ;

CALL `migrate_20260726000100_legacy_workbench_chat`();
DROP PROCEDURE `migrate_20260726000100_legacy_workbench_chat`;
