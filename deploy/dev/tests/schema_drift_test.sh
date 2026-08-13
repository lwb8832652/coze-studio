#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd -P)
DRIFT_SCRIPT=$REPO_ROOT/deploy/dev/check-schema-drift.sh
VERSION=20260812000100
SECRET_MARKER='schema-drift-secret-password'

fail() {
  printf 'schema drift test failure: %s\n' "$*" >&2
  exit 1
}

assert_contains() {
  local file=$1
  local expected=$2
  local message=$3

  grep -F -- "$expected" "$file" >/dev/null || fail "$message"
}

assert_not_contains() {
  local file=$1
  local unexpected=$2
  local message=$3

  if grep -F -- "$unexpected" "$file" >/dev/null; then
    fail "$message"
  fi
}

assert_count() {
  local file=$1
  local pattern=$2
  local expected=$3
  local message=$4
  local actual

  actual=$(grep -c -F -- "$pattern" "$file" || true)
  [ "$actual" -eq "$expected" ] || fail "$message (expected $expected, got $actual)"
}

[ -x "$DRIFT_SCRIPT" ] || fail 'check-schema-drift.sh is missing or not executable'
grep -F "trap 'cleanup_resources >/dev/null 2>&1 || true' EXIT" \
  "$DRIFT_SCRIPT" >/dev/null || \
  fail 'schema drift script must register cleanup on shell exit'
grep -F "trap 'exit 1' HUP INT TERM" "$DRIFT_SCRIPT" >/dev/null || \
  fail 'schema drift script must turn termination signals into a failing exit'

TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/schema-drift-test.XXXXXX")
trap 'rm -rf -- "$TEST_ROOT"' EXIT
FAKE_DOCKER=$TEST_ROOT/docker

cat >"$FAKE_DOCKER" <<'FAKE_DOCKER_SCRIPT'
#!/usr/bin/env bash
set -euo pipefail

{
  printf 'docker'
  printf ' %s' "$@"
  printf '\n'
} >>"$COMMAND_LOG"

case "${1:-}" in
  network)
    case "${2:-}" in
      create)
        [ "$TEST_CASE" != network-create-failure ] || exit 11
        printf '%s\n' fake-network-id
        ;;
      rm)
        [ "$TEST_CASE" != network-cleanup-failure ] || exit 12
        printf '%s\n' "${3:-}"
        ;;
      *) exit 90 ;;
    esac
    ;;
  run)
    if [ "${2:-}" = -d ]; then
      [ "$TEST_CASE" != mysql-start-failure ] || exit 13
      printf '%s\n' fake-mysql-container-id
      exit 0
    fi
    case " $* " in
      *' schema diff '*)
        printf '%s\n' 'Notice: Atlas Community edition diagnostic'
        case "$TEST_CASE" in
          drift)
            printf '%s\n' 'COZE_SCHEMA_DIFF|1'
          ;;
          diff-failure)
            printf 'remote mysql://user:%s@db.invalid/opencoze failed\n' \
              "$SECRET_MARKER" >&2
            exit 14
            ;;
          invalid-diff-output)
            printf '%s\n' 'unexpected formatter output'
            ;;
          *)
            printf '%s\n' 'COZE_SCHEMA_DIFF|0'
            ;;
        esac
        ;;
      *)
        printf 'unexpected docker run command: %s\n' "$*" >&2
        exit 91
        ;;
    esac
    ;;
  inspect)
    case "$TEST_CASE" in
      inspect-failure) exit 15 ;;
      mysql-unhealthy) printf '%s\n' unhealthy ;;
      *) printf '%s\n' healthy ;;
    esac
    ;;
  rm)
    [ "$TEST_CASE" != container-cleanup-failure ] || exit 16
    printf '%s\n' "${3:-}"
    ;;
  *)
    printf 'unexpected fake docker command: %s\n' "$*" >&2
    exit 92
    ;;
esac
FAKE_DOCKER_SCRIPT
chmod 700 "$FAKE_DOCKER"

setup_case() {
  local name=$1

  CASE_DIR=$TEST_ROOT/$name
  MIGRATIONS_DIR=$CASE_DIR/migrations
  ENV_FILE=$CASE_DIR/atlas.env
  COMMAND_LOG=$CASE_DIR/commands.log
  OUTPUT_LOG=$CASE_DIR/output.log
  TEMP_DIR=$CASE_DIR/tmp
  mkdir -p "$MIGRATIONS_DIR" "$TEMP_DIR"
  printf '%s\n' 'h1:fixture' >"$MIGRATIONS_DIR/atlas.sum"
  printf '%s\n' 'CREATE TABLE fixture (id bigint PRIMARY KEY);' \
    >"$MIGRATIONS_DIR/${VERSION}_expand_fixture_create_table.sql"
  printf 'ATLAS_URL=mysql://user:%s@db.invalid:3306/opencoze\n' \
    "$SECRET_MARKER" >"$ENV_FILE"
  chmod 600 "$ENV_FILE"
  : >"$COMMAND_LOG"
  export COMMAND_LOG SECRET_MARKER TEST_CASE=$name
}

run_check() {
  TMPDIR="$TEMP_DIR" DOCKER_BIN="$FAKE_DOCKER" \
    "$DRIFT_SCRIPT" \
      --migrations-dir "$MIGRATIONS_DIR" \
      --atlas-env-file "$ENV_FILE" \
      --version "$VERSION" \
      --stage pre-apply >"$OUTPUT_LOG" 2>&1
}

assert_cleanup_attempted() {
  assert_count "$COMMAND_LOG" 'docker rm -f ' 1 \
    'temporary MySQL container cleanup was not attempted exactly once'
  assert_count "$COMMAND_LOG" 'docker network rm ' 1 \
    'temporary Docker network cleanup was not attempted exactly once'
  if find "$TEMP_DIR" -mindepth 1 -maxdepth 1 -name 'coze-schema-drift.*' \
    -print -quit | grep -q .; then
    fail 'schema drift temporary directory was not cleaned up'
  fi
}

assert_safe_output() {
  assert_not_contains "$OUTPUT_LOG" "$SECRET_MARKER" 'credential leaked into output'
  assert_not_contains "$COMMAND_LOG" "$SECRET_MARKER" 'credential leaked into Docker command'
  assert_not_contains "$OUTPUT_LOG" 'ALTER TABLE missing_table' 'raw schema diff leaked into output'
}

setup_case clear
(cd "$CASE_DIR" && TMPDIR="$TEMP_DIR" DOCKER_BIN="$FAKE_DOCKER" \
  "$DRIFT_SCRIPT" \
    --migrations-dir migrations \
    --atlas-env-file "$ENV_FILE" \
    --version "$VERSION" \
    --stage pre-apply >"$OUTPUT_LOG" 2>&1) || \
  fail 'clear schema with a relative migration path unexpectedly failed'
assert_contains "$OUTPUT_LOG" 'schema drift check passed during pre-apply' \
  'clear schema did not report success'
assert_cleanup_attempted
assert_safe_output
assert_contains "$COMMAND_LOG" 'file:///migrations?format=atlas&version=${EXPECTED_VERSION}' \
  'schema diff did not target the requested migration version inside the container'
assert_contains "$COMMAND_LOG" '--from "$ATLAS_URL"' \
  'schema diff did not read the remote URL from the protected env file inside the container'
assert_contains "$COMMAND_LOG" '--dev-url "$ATLAS_DEV_URL"' \
  'schema diff did not use the isolated temporary MySQL as the dev database'
assert_contains "$COMMAND_LOG" '--exclude "atlas_schema_revisions,table_*"' \
  'schema diff must exclude migration metadata and runtime-owned resource tables'
assert_contains "$COMMAND_LOG" '--format "COZE_SCHEMA_DIFF|{{ len .Changes }}"' \
  'schema diff did not request a uniquely parseable change count'
assert_contains "$COMMAND_LOG" \
  '--health-cmd=mysqladmin ping -h 127.0.0.1 -P3306 --protocol=tcp' \
  'temporary MySQL healthcheck must wait for the final TCP server instead of the initialization socket'

setup_case drift
if run_check; then
  fail 'schema drift unexpectedly passed'
fi
assert_contains "$OUTPUT_LOG" 'schema drift detected during pre-apply' \
  'schema drift did not report the expected failure'
assert_cleanup_attempted
assert_safe_output

for failure_case in diff-failure invalid-diff-output inspect-failure mysql-unhealthy \
  mysql-start-failure network-create-failure; do
  setup_case "$failure_case"
  if run_check; then
    fail "$failure_case unexpectedly passed"
  fi
  assert_contains "$OUTPUT_LOG" 'schema drift inspection failed during pre-apply' \
    "$failure_case did not fail closed"
  assert_safe_output
done

setup_case container-cleanup-failure
if run_check; then
  fail 'container cleanup failure unexpectedly passed'
fi
assert_contains "$OUTPUT_LOG" 'schema drift cleanup failed' \
  'container cleanup failure did not fail closed'
assert_count "$COMMAND_LOG" 'docker network rm ' 1 \
  'network cleanup was not attempted after container cleanup failure'
assert_safe_output

setup_case network-cleanup-failure
if run_check; then
  fail 'network cleanup failure unexpectedly passed'
fi
assert_contains "$OUTPUT_LOG" 'schema drift cleanup failed' \
  'network cleanup failure did not fail closed'
assert_safe_output

setup_case invalid-version
if TMPDIR="$TEMP_DIR" DOCKER_BIN="$FAKE_DOCKER" \
  "$DRIFT_SCRIPT" --migrations-dir "$MIGRATIONS_DIR" \
    --atlas-env-file "$ENV_FILE" --version latest --stage pre-apply \
    >"$OUTPUT_LOG" 2>&1; then
  fail 'invalid version unexpectedly passed'
fi
assert_contains "$OUTPUT_LOG" 'version must be a 14-digit migration version' \
  'invalid version did not report the expected error'
assert_count "$COMMAND_LOG" 'docker ' 0 'invalid version reached Docker'
assert_safe_output

printf '%s\n' 'schema drift tests passed'
