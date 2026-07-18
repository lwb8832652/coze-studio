-- Preserve pre-interlock AppDev launches as either recoverable identities or
-- explicit operator-visible quarantine. This migration never guesses a
-- canonical request digest and never classifies a possibly submitted launch as
-- not submitted.
ALTER TABLE `appdev_provider_executions`
  MODIFY COLUMN `launch_state` varchar(24) NOT NULL DEFAULT 'none';

-- A durable handle plus checkpoint is already addressable. The legacy row did
-- not persist the canonical request digest, so the digest remains NULL while
-- the immutable operation identities are reconstructed from the original
-- idempotency key.
UPDATE `appdev_provider_executions`
SET
  `launch_state` = 'complete',
  `launch_operation_hash` = UNHEX(SHA2(CONCAT(
    'appdev_provider_execution_launch:v1', CHAR(0),
    CAST(`space_id` AS CHAR), CHAR(0),
    `project_id`, CHAR(0),
    CAST(`generation` AS CHAR), CHAR(0),
    `provider_key`, CHAR(0),
    `provider_scope`, CHAR(0),
    CONVERT(`idempotency_key` USING utf8mb4)
  ), 256)),
  `launch_provider_operation_id` = CONCAT('appdev_start_', LOWER(SHA2(CONCAT(
    CAST(`space_id` AS CHAR), CHAR(0),
    `project_id`, CHAR(0),
    CONVERT(`idempotency_key` USING utf8mb4), CHAR(0)
  ), 256))),
  `launch_request_digest` = NULL,
  `launch_expires_at` = NULL,
  `observed_state` = 'running',
  `submission_started_at` = COALESCE(`submission_started_at`, UTC_TIMESTAMP(6)),
  `owner_identity_hash` = NULL,
  `owner_expires_at` = NULL,
  `version` = `version` + 1,
  `updated_at` = UTC_TIMESTAMP(6)
WHERE `launch_state` = 'none'
  AND `observed_state` IN ('pending', 'submitting')
  AND `provider_execution_id` <> ''
  AND `checkpoint_envelope` <> ''
  AND OCTET_LENGTH(`idempotency_key`) BETWEEN 1 AND 128
  AND CONVERT(`idempotency_key` USING utf8mb4) REGEXP '^[!-~]+$';

-- Rows with a stable original operation but no addressable handle use a
-- versioned tenant-scoped legacy lookup. No request digest is fabricated.
UPDATE `appdev_provider_executions`
SET
  `launch_state` = 'legacy_submitted',
  `launch_operation_hash` = UNHEX(SHA2(CONCAT(
    'appdev_provider_execution_launch:v1', CHAR(0),
    CAST(`space_id` AS CHAR), CHAR(0),
    `project_id`, CHAR(0),
    CAST(`generation` AS CHAR), CHAR(0),
    `provider_key`, CHAR(0),
    `provider_scope`, CHAR(0),
    CONVERT(`idempotency_key` USING utf8mb4)
  ), 256)),
  `launch_provider_operation_id` = CONCAT('appdev_start_', LOWER(SHA2(CONCAT(
    CAST(`space_id` AS CHAR), CHAR(0),
    `project_id`, CHAR(0),
    CONVERT(`idempotency_key` USING utf8mb4), CHAR(0)
  ), 256))),
  `launch_request_digest` = NULL,
  `launch_expires_at` = NULL,
  `observed_state` = 'submitting',
  `submission_started_at` = COALESCE(`submission_started_at`, UTC_TIMESTAMP(6)),
  `owner_identity_hash` = NULL,
  `owner_expires_at` = NULL,
  `version` = `version` + 1,
  `updated_at` = UTC_TIMESTAMP(6)
WHERE `launch_state` = 'none'
  AND `observed_state` IN ('pending', 'submitting')
  AND `provider_execution_id` = ''
  AND OCTET_LENGTH(`idempotency_key`) BETWEEN 1 AND 128
  AND CONVERT(`idempotency_key` USING utf8mb4) REGEXP '^[!-~]+$';

-- Anything lacking a stable operation identity remains visible but cannot be
-- automatically started, stopped, archived, or reconciled. An operator can
-- inspect and resolve the quarantined record without repository hydration
-- failing closed as a generic database outage.
UPDATE `appdev_provider_executions`
SET
  `idempotency_key` = CONCAT('legacy-quarantine-', `id`),
  `launch_state` = 'quarantined',
  `launch_operation_hash` = NULL,
  `launch_provider_operation_id` = '',
  `launch_request_digest` = NULL,
  `launch_expires_at` = NULL,
  `observed_state` = 'submitting',
  `submission_started_at` = COALESCE(`submission_started_at`, UTC_TIMESTAMP(6)),
  `safe_error_code` = 'legacy_launch_quarantined',
  `safe_error_message` = 'provider launch requires operator recovery',
  `owner_identity_hash` = NULL,
  `owner_expires_at` = NULL,
  `version` = `version` + 1,
  `updated_at` = UTC_TIMESTAMP(6)
WHERE `launch_state` = 'none'
  AND `observed_state` IN ('pending', 'submitting');
