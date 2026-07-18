-- Correct legacy launch rows that cannot be reconciled by operation identity.
-- A checkpoint without a provider handle is ambiguous rather than
-- reconcilable, so it remains fail-closed for audited operator disposition.
UPDATE `appdev_provider_executions`
SET
  `launch_state` = 'quarantined',
  `launch_operation_hash` = NULL,
  `launch_provider_operation_id` = '',
  `launch_request_digest` = NULL,
  `launch_expires_at` = NULL,
  `safe_error_code` = 'legacy_launch_quarantined',
  `safe_error_message` = 'provider launch requires operator recovery',
  `updated_at` = UTC_TIMESTAMP(6)
WHERE
  `launch_state` = 'legacy_submitted'
  AND `provider_execution_id` = ''
  AND `checkpoint_envelope` <> '';

CREATE TABLE `appdev_provider_execution_quarantine_audits` (
  `id` varchar(64) NOT NULL,
  `execution_id` varchar(64) NOT NULL,
  `space_id` bigint unsigned NOT NULL,
  `project_id` varchar(64) NOT NULL,
  `generation` bigint unsigned NOT NULL,
  `expected_version` bigint unsigned NOT NULL,
  `owner_identity_hash` binary(32) NOT NULL,
  `owner_epoch` bigint unsigned NOT NULL,
  `actor_id` bigint unsigned NOT NULL,
  `operation_hash` binary(32) NOT NULL,
  `acknowledgement` varchar(32) NOT NULL,
  `reason` varchar(64) NOT NULL,
  `evidence_hash` binary(32) NOT NULL,
  `created_at` datetime(6) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_appdev_provider_quarantine_audit_operation`
    (`space_id`, `project_id`, `generation`, `operation_hash`),
  KEY `idx_appdev_provider_quarantine_audit_execution`
    (`space_id`, `project_id`, `generation`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
