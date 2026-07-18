ALTER TABLE `appdev_provider_executions`
  ADD COLUMN `launch_provider_operation_id` varbinary(128) NOT NULL DEFAULT '' AFTER `launch_operation_hash`,
  ADD COLUMN `launch_request_digest` binary(32) NULL AFTER `launch_provider_operation_id`;
