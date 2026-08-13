#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd -P)
POLICY_SCRIPT=$REPO_ROOT/scripts/database/check-migration-policy.sh

fail() {
  printf 'migration policy test failure: %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  local file=$1
  local expected=$2
  local message=$3

  grep -F -- "$expected" "$file" >/dev/null || fail "$message"
}

new_repo() {
  local name=$1

  CASE_DIR=$TEST_ROOT/$name
  mkdir -p "$CASE_DIR/docker/atlas/migrations"
  git -C "$CASE_DIR" init -q
  git -C "$CASE_DIR" config user.email migration-policy@example.test
  git -C "$CASE_DIR" config user.name 'Migration Policy Test'
  printf '%s\n' 'CREATE TABLE legacy_table (id bigint PRIMARY KEY);' \
    >"$CASE_DIR/docker/atlas/migrations/20250101000000_legacy.sql"
  printf '%s\n' 'legacy checksum' \
    >"$CASE_DIR/docker/atlas/migrations/atlas.sum"
  git -C "$CASE_DIR" add docker/atlas/migrations
  git -C "$CASE_DIR" commit -q -m baseline
  BASE_SHA=$(git -C "$CASE_DIR" rev-parse HEAD)
}

commit_case() {
  git -C "$CASE_DIR" add -A docker/atlas/migrations
  git -C "$CASE_DIR" commit -q -m case
  TARGET_SHA=$(git -C "$CASE_DIR" rev-parse HEAD)
}

expect_success() {
  local name=$1
  shift
  local output=$TEST_ROOT/$name.out

  if ! (cd "$CASE_DIR" && "$POLICY_SCRIPT" "$BASE_SHA" "$TARGET_SHA" "$@") \
    >"$output" 2>&1; then
    cat "$output" >&2
    fail "$name should pass"
  fi
}

expect_failure() {
  local name=$1
  local expected=$2
  shift 2
  local output=$TEST_ROOT/$name.out

  if (cd "$CASE_DIR" && "$POLICY_SCRIPT" "$BASE_SHA" "$TARGET_SHA" "$@") \
    >"$output" 2>&1; then
    fail "$name should fail"
  fi
  assert_contains "$output" "$expected" "$name should explain the policy failure"
}

[ -x "$POLICY_SCRIPT" ] || fail 'check-migration-policy.sh is missing or not executable'

TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/migration-policy-test.XXXXXX")
trap 'rm -rf -- "$TEST_ROOT"' EXIT

new_repo valid-forward-migrations
printf '%s\n' \
  'CREATE TABLE plugin_code_drafts (id bigint PRIMARY KEY);' \
  >"$CASE_DIR/docker/atlas/migrations/20260813103000_expand_plugin_code_create_tables.sql"
printf '%s\n' \
  'UPDATE space SET allow_develop = 1 WHERE allow_develop IS NULL;' \
  >"$CASE_DIR/docker/atlas/migrations/20260813104500_data_space_backfill_allow_develop.sql"
commit_case
expect_success valid-forward-migrations

new_repo invalid-name
printf '%s\n' 'CREATE TABLE bad_name (id bigint PRIMARY KEY);' \
  >"$CASE_DIR/docker/atlas/migrations/20260813_add_table.sql"
commit_case
expect_failure invalid-name 'invalid migration filename'

new_repo duplicate-version
printf '%s\n' 'CREATE TABLE first_table (id bigint PRIMARY KEY);' \
  >"$CASE_DIR/docker/atlas/migrations/20260813105000_expand_first_create_table.sql"
printf '%s\n' 'CREATE TABLE second_table (id bigint PRIMARY KEY);' \
  >"$CASE_DIR/docker/atlas/migrations/20260813105000_expand_second_create_table.sql"
commit_case
expect_failure duplicate-version 'migration version must be unique'

new_repo out-of-order-version
printf '%s\n' 'CREATE TABLE future_table (id bigint PRIMARY KEY);' \
  >"$CASE_DIR/docker/atlas/migrations/20260813120000_expand_future_create_table.sql"
git -C "$CASE_DIR" add docker/atlas/migrations
git -C "$CASE_DIR" commit -q -m future-baseline
BASE_SHA=$(git -C "$CASE_DIR" rev-parse HEAD)
printf '%s\n' 'CREATE TABLE older_table (id bigint PRIMARY KEY);' \
  >"$CASE_DIR/docker/atlas/migrations/20260813110000_expand_older_create_table.sql"
commit_case
expect_failure out-of-order-version 'new migration version must be greater than the existing maximum'

new_repo contract-default-blocked
printf '%s\n' 'ALTER TABLE legacy_table DROP COLUMN obsolete_value;' \
  >"$CASE_DIR/docker/atlas/migrations/20260814100000_contract_legacy_drop_columns.sql"
commit_case
expect_failure contract-default-blocked 'requires a separate special migration approval'
expect_success contract-explicitly-allowed --allow-special

new_repo repair-default-blocked
printf '%s\n' 'CREATE INDEX idx_legacy_id ON legacy_table (id);' \
  >"$CASE_DIR/docker/atlas/migrations/20260814103000_repair_legacy_restore_indexes.sql"
commit_case
expect_failure repair-default-blocked 'requires a separate special migration approval'
expect_success repair-explicitly-allowed --allow-special

new_repo expand-cannot-drop
printf '%s\n' 'ALTER TABLE legacy_table DROP COLUMN obsolete_value;' \
  >"$CASE_DIR/docker/atlas/migrations/20260813110000_expand_legacy_drop_column.sql"
commit_case
expect_failure expand-cannot-drop 'expand migration contains destructive SQL'

new_repo data-cannot-drop
printf '%s\n' 'TRUNCATE TABLE legacy_table;' \
  >"$CASE_DIR/docker/atlas/migrations/20260813111000_data_legacy_clear_rows.sql"
commit_case
expect_failure data-cannot-drop 'data migration contains schema-destructive SQL'

new_repo historical-migration-modified
printf '%s\n' 'CREATE TABLE legacy_table (id bigint PRIMARY KEY, changed int);' \
  >"$CASE_DIR/docker/atlas/migrations/20250101000000_legacy.sql"
commit_case
expect_failure historical-migration-modified 'existing migration files are immutable'

new_repo historical-migration-deleted
rm "$CASE_DIR/docker/atlas/migrations/20250101000000_legacy.sql"
commit_case
expect_failure historical-migration-deleted 'existing migration files are immutable'

printf '%s\n' 'migration policy tests passed'
