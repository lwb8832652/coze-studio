#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd -P)
PUBLISH_SCRIPT=$REPO_ROOT/deploy/dev/publish-dev.sh

EXPECTED_ORIGIN=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
TARGET_SHA=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
OTHER_SHA=cccccccccccccccccccccccccccccccccccccccc
ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
SECRET_MARKER='publish-dev-test-password'
ATLAS_URL_VALUE="mysql://migration_user:${SECRET_MARKER}@db.invalid:3306/coze_dev"
ALT_MYSQL_SECRET='alternate-mysql-password'
ALT_MYSQL_URI="mysql://alternate_user:${ALT_MYSQL_SECRET}@alternate.invalid:3306/other"
AUTHORITY_SECRET='authority-output-password'
AUTHORITY_USERINFO="authority_user:${AUTHORITY_SECRET}"
AUTHORITY_URI="https://${AUTHORITY_USERINFO}@api.invalid/v1"
PASSWORD_FRAGMENT='plain-output-password'
API_TOKEN_VALUE='mixed-api-token-value'
CLIENT_SECRET_VALUE='mixed-client-secret-value'
ACCESS_KEY_VALUE='mixed-access-key-value'
SECRET_KEY_VALUE='mixed-secret-key-value'
PASSWORD_FIELD_VALUE='matrix-password-value'
PASSWD_FIELD_VALUE='matrix-passwd-value'
PWD_FIELD_VALUE='matrix-pwd-value'
SECRET_FIELD_VALUE='matrix-secret-value'
TOKEN_FIELD_VALUE='matrix-token-value'
API_SPACE_KEY_VALUE='matrix-api-space-key-value'
API_UNDERSCORE_KEY_VALUE='matrix-api-underscore-key-value'
API_HYPHEN_KEY_VALUE='matrix-api-hyphen-key-value'
APIKEY_VALUE='matrix-apikey-value'
ACCESS_SPACE_KEY_VALUE='matrix-access-space-key-value'
ACCESS_UNDERSCORE_KEY_VALUE='matrix-access-underscore-key-value'
ACCESSKEY_VALUE='matrix-accesskey-value'
SECRET_SPACE_KEY_VALUE='matrix-secret-space-key-value'
SECRET_UNDERSCORE_KEY_VALUE='matrix-secret-underscore-key-value'
SECRETKEY_VALUE='matrix-secretkey-value'

fail() {
  printf 'publish dev test failure: %s\n' "$*" >&2
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

[ -f "$PUBLISH_SCRIPT" ] || fail 'publish-dev.sh is missing'
[ -x "$PUBLISH_SCRIPT" ] || fail 'publish-dev.sh is not executable'

TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/publish-dev-test.XXXXXX")
TEST_ROOT=$(CDPATH= cd -- "$TEST_ROOT" && pwd -P)
trap 'rm -rf "$TEST_ROOT"' EXIT
FAKE_BIN=$TEST_ROOT/bin
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/allowed-tool" <<'FAKE_ALLOWED_TOOL'
#!/bin/bash
set -euo pipefail

tool=${0##*/}
[ "$PATH" = "$FAKE_BIN" ] || {
  printf 'unexpected child PATH for %s\n' "$tool" >&2
  exit 94
}
printf '%s\n' "$PATH" >>"$PATH_AUDIT_LOG"

case "$tool" in
  bash) exec /bin/bash "$@" ;;
  basename) exec /usr/bin/basename "$@" ;;
  cat) exec /bin/cat "$@" ;;
  chmod) exec /bin/chmod "$@" ;;
  dirname) exec /usr/bin/dirname "$@" ;;
  grep) exec /usr/bin/grep "$@" ;;
  mktemp) exec /usr/bin/mktemp "$@" ;;
  rm) exec /bin/rm "$@" ;;
  seq) exec /usr/bin/seq "$@" ;;
  tr) exec /usr/bin/tr "$@" ;;
  *)
    printf 'unexpected allowlisted tool: %s\n' "$tool" >&2
    exit 95
    ;;
esac
FAKE_ALLOWED_TOOL

for allowed_tool in bash basename cat chmod dirname grep mktemp rm seq tr; do
  ln -s allowed-tool "$FAKE_BIN/$allowed_tool"
done

if PATH="$FAKE_BIN" /bin/bash -c 'publish_dev_non_allowlisted_command' >/dev/null 2>&1; then
  fail 'an unknown bare command unexpectedly resolved through the closed fake PATH'
else
  unknown_command_status=$?
fi
[ "$unknown_command_status" -eq 127 ] || \
  fail "an unknown bare command exited $unknown_command_status instead of 127"

cat >"$FAKE_BIN/git" <<'FAKE_GIT'
#!/usr/bin/env bash
set -euo pipefail

[ "$PATH" = "$FAKE_BIN" ] || {
  printf '%s\n' 'unexpected child PATH for git' >&2
  exit 94
}
printf '%s\n' "$PATH" >>"$PATH_AUDIT_LOG"

{
  printf 'git'
  printf ' %s' "$@"
  printf '\n'
} >>"$COMMAND_LOG"
printf '%s\n' "$PWD" >>"$GIT_PWD_LOG"

case "${1:-}" in
  symbolic-ref)
    if [ "$TEST_CASE" = wrong-branch ]; then
      printf '%s\n' feature/not-dev
    else
      printf '%s\n' dev
    fi
    ;;
  status)
    if [ "$TEST_CASE" = dirty-worktree ]; then
      printf '%s\n' ' M tracked-file'
    elif [ "$TEST_CASE" = hidden-untracked-migration ] && \
      [ "$*" = 'status --porcelain --untracked-files=all' ]; then
      printf '%s\n' '?? docker/atlas/migrations/20990101000000_untracked.sql'
    fi
    ;;
  fetch)
    count=0
    if [ -f "$FETCH_COUNT_FILE" ]; then
      count=$(cat "$FETCH_COUNT_FILE")
    fi
    count=$((count + 1))
    printf '%s\n' "$count" >"$FETCH_COUNT_FILE"
    ;;
  rev-parse)
    case "${2:-}" in
      HEAD)
        if [ "$TEST_CASE" = wrong-head ]; then
          printf '%s\n' "$OTHER_SHA"
        else
          printf '%s\n' "$TARGET_SHA"
        fi
        ;;
      FETCH_HEAD)
        fetch_count=$(cat "$FETCH_COUNT_FILE")
        if [ "$TEST_CASE" = stale-origin ] || [ "$TEST_CASE" = stale-fetched-head ] || \
          { [ "$TEST_CASE" = second-fetch-race ] && [ "$fetch_count" -gt 1 ]; }; then
          printf '%s\n' "$OTHER_SHA"
        else
          printf '%s\n' "$EXPECTED_ORIGIN"
        fi
        ;;
      refs/remotes/origin/dev)
        printf '%s\n' "$EXPECTED_ORIGIN"
        ;;
      *)
        printf 'unexpected fake git rev-parse arguments: %s\n' "$*" >&2
        exit 91
        ;;
    esac
    ;;
  cat-file)
    if [ "$TEST_CASE" = missing-target ]; then
      exit 1
    fi
    ;;
  merge-base)
    if [ "$TEST_CASE" = unrelated-target ]; then
      exit 1
    fi
    ;;
  diff)
    [ "$*" = "diff --name-status --no-renames $EXPECTED_ORIGIN $TARGET_SHA -- docker/atlas/migrations/*.sql" ] || {
      printf 'unexpected fake git diff arguments: %s\n' "$*" >&2
      exit 91
    }
    case "$TEST_CASE" in
      invalid-migration-name)
        printf 'A\tdocker/atlas/migrations/20260813_add_table.sql\n'
        ;;
      modified-historical-migration)
        printf 'M\tdocker/atlas/migrations/20250101000000_legacy.sql\n'
        ;;
      contract-migration)
        printf 'A\tdocker/atlas/migrations/20260814100000_contract_legacy_drop_columns.sql\n'
        ;;
      repair-migration)
        printf 'A\tdocker/atlas/migrations/20260814103000_repair_legacy_restore_indexes.sql\n'
        ;;
      destructive-expand-migration)
        printf 'A\tdocker/atlas/migrations/20260813110000_expand_legacy_drop_column.sql\n'
        ;;
    esac
    ;;
  show)
    case "$TEST_CASE" in
      contract-migration)
        printf '%s\n' 'ALTER TABLE legacy_table DROP COLUMN obsolete_value;'
        ;;
      repair-migration)
        printf '%s\n' 'CREATE INDEX idx_legacy_id ON legacy_table (id);'
        ;;
      destructive-expand-migration)
        printf '%s\n' 'ALTER TABLE legacy_table DROP COLUMN obsolete_value;'
        ;;
      *)
        printf 'unexpected fake git show arguments: %s\n' "$*" >&2
        exit 91
        ;;
    esac
    ;;
  archive)
    [ "$*" = "archive --format=tar $TARGET_SHA -- docker/atlas/migrations .github/atlas-dev.hcl" ] || {
      printf 'unexpected fake git archive arguments: %s\n' "$*" >&2
      exit 91
    }
    if [ "$TEST_CASE" = archive-failure ] || [ "$TEST_CASE" = repo-tmpdir ] || \
      [ "$TEST_CASE" = repo-symlink-tmpdir ]; then
      printf '%s\n' 'fake git archive failure' >&2
      exit 1
    fi
    cat "$ARCHIVE_FIXTURE"
    ;;
  push)
    if [ "$TEST_CASE" = push-failure ]; then
      printf '%s\n' 'fake git push failure' >&2
      exit 1
    fi
    ;;
  *)
    printf 'unexpected fake git command: %s\n' "$*" >&2
    exit 92
    ;;
esac
FAKE_GIT

cat >"$FAKE_BIN/docker" <<'FAKE_DOCKER'
#!/usr/bin/env bash
set -euo pipefail

[ "$PATH" = "$FAKE_BIN" ] || {
  printf '%s\n' 'unexpected child PATH for docker' >&2
  exit 94
}
printf '%s\n' "$PATH" >>"$PATH_AUDIT_LOG"

is_drift_command=false
case "${1:-}" in
  network|inspect|rm) is_drift_command=true ;;
  run)
    if [ "${2:-}" = -d ] || [[ " $* " == *' schema diff '* ]]; then
      is_drift_command=true
    fi
    ;;
esac

if [ "$is_drift_command" = true ]; then
  {
    printf 'docker'
    printf ' %s' "$@"
    printf '\n'
  } >>"$DRIFT_COMMAND_LOG"

  case "${1:-}" in
    network)
      case "${2:-}" in
        create) printf '%s\n' fake-drift-network ;;
        rm) printf '%s\n' "${3:-}" ;;
        *) exit 93 ;;
      esac
      ;;
    inspect)
      printf '%s\n' healthy
      ;;
    rm)
      printf '%s\n' "${3:-}"
      ;;
    run)
      if [ "${2:-}" = -d ]; then
        printf '%s\n' fake-drift-mysql
        exit 0
      fi
      count=0
      if [ -f "$DRIFT_COUNT_FILE" ]; then
        count=$(cat "$DRIFT_COUNT_FILE")
      fi
      count=$((count + 1))
      printf '%s\n' "$count" >"$DRIFT_COUNT_FILE"
      if { [ "$TEST_CASE" = pre-drift-failure ] && [ "$count" -eq 1 ]; } || \
        { [ "$TEST_CASE" = post-drift-failure ] && [ "$count" -eq 2 ]; }; then
        printf '%s\n' 'COZE_SCHEMA_DIFF|1'
      elif [ "$TEST_CASE" = drift-inspection-failure ] && [ "$count" -eq 1 ]; then
        printf '%s\n' 'schema inspection failed' >&2
        exit 97
      else
        printf '%s\n' 'COZE_SCHEMA_DIFF|0'
      fi
      ;;
  esac
  exit 0
fi

{
  printf 'docker'
  printf ' %s' "$@"
  printf '\n'
} >>"$COMMAND_LOG"

stage=
case " $* " in
  *' migrate validate --dir file:///migrations '*)
    stage=validate
    ;;
  *' migrate status --config file:///atlas.hcl --env dev '*)
    stage=status
    ;;
  *' migrate apply --config file:///atlas.hcl --env dev '*)
    stage=apply
    ;;
  *)
    printf 'unexpected fake docker command: %s\n' "$*" >&2
    exit 93
    ;;
esac

migrations_source=
config_source=
runtime_env_arg=
mount_count=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    -v)
      shift
      [ "$#" -gt 0 ] || exit 93
      mount_count=$((mount_count + 1))
      case "$1" in
        *:/migrations:ro) migrations_source=${1%:/migrations:ro} ;;
        *:/atlas.hcl:ro) config_source=${1%:/atlas.hcl:ro} ;;
        *)
          printf 'unexpected fake Docker mount: %s\n' "$1" >&2
          exit 93
          ;;
      esac
      ;;
    --env-file)
      shift
      [ "$#" -gt 0 ] || exit 93
      runtime_env_arg=$1
      ;;
  esac
  shift
done

[ -s "$SNAPSHOT_PATH_FILE" ] || {
  printf '%s\n' 'Atlas snapshot path was not recorded before Docker' >&2
  exit 96
}
IFS= read -r snapshot_root <"$SNAPSHOT_PATH_FILE"
[ -n "$snapshot_root" ] || exit 96
[ "$snapshot_root" != "$REPO_ROOT" ] || {
  printf '%s\n' 'Docker mounted the live repository root' >&2
  exit 96
}
[ "$migrations_source" = "$snapshot_root/docker/atlas/migrations" ] || {
  printf '%s\n' 'Docker did not mount migrations from the recorded snapshot' >&2
  exit 96
}
[ -f "$migrations_source/20260812000100_expand_fixture_add_value.sql" ] || {
  printf '%s\n' 'snapshot migration fixture is missing' >&2
  exit 96
}

runtime_env_expected=$snapshot_root/atlas-runtime.env
snapshot_mode=unreadable
runtime_mode=unreadable
if snapshot_mode=$(/usr/bin/stat -c '%a' "$snapshot_root" 2>/dev/null); then
  :
elif snapshot_mode=$(/usr/bin/stat -f '%Lp' "$snapshot_root" 2>/dev/null); then
  :
fi
[ "$snapshot_mode" = 700 ] || {
  printf 'snapshot mode was %s, expected 700\n' "$snapshot_mode" >&2
  exit 96
}
[ -f "$runtime_env_expected" ] || {
  printf '%s\n' 'snapshot runtime env is missing' >&2
  exit 96
}
if runtime_mode=$(/usr/bin/stat -c '%a' "$runtime_env_expected" 2>/dev/null); then
  :
elif runtime_mode=$(/usr/bin/stat -f '%Lp' "$runtime_env_expected" 2>/dev/null); then
  :
fi
[ "$runtime_mode" = 600 ] || {
  printf 'runtime env mode was %s, expected 600\n' "$runtime_mode" >&2
  exit 96
}
{
  runtime_line=
  IFS= read -r runtime_line || {
    printf '%s\n' 'snapshot runtime env could not be read' >&2
    exit 96
  }
  [ "$runtime_line" = "ATLAS_URL=$ATLAS_URL_VALUE" ] || {
    printf '%s\n' 'snapshot runtime env did not retain the validated Atlas URL' >&2
    exit 96
  }
  if IFS= read -r extra_runtime_line; then
    printf '%s\n' 'snapshot runtime env contains extra lines' >&2
    exit 96
  fi
} <"$runtime_env_expected"

if [ "$stage" = validate ]; then
  [ "$mount_count" -eq 1 ] && [ -z "$config_source" ] && [ -z "$runtime_env_arg" ] || {
    printf '%s\n' 'validate received unexpected mounts or env file' >&2
    exit 96
  }
else
  [ "$mount_count" -eq 2 ] || {
    printf '%s\n' 'status/apply did not receive exactly two mounts' >&2
    exit 96
  }
  [ "$config_source" = "$snapshot_root/.github/atlas-dev.hcl" ] || {
    printf '%s\n' 'Docker did not mount Atlas config from the recorded snapshot' >&2
    exit 96
  }
  [ -f "$config_source" ] || {
    printf '%s\n' 'snapshot Atlas config fixture is missing' >&2
    exit 96
  }
  [ "$runtime_env_arg" = "$runtime_env_expected" ] || {
    printf '%s\n' 'status/apply did not use the snapshot runtime env' >&2
    exit 96
  }
  [ "$runtime_env_arg" != "$ENV_FILE" ] || {
    printf '%s\n' 'Docker received the original Atlas env file' >&2
    exit 96
  }
fi

printf 'snapshot stage=%s root=%s root-mode=%s runtime=%s runtime-mode=%s value=original\n' \
  "$stage" "$snapshot_root" "$snapshot_mode" "$runtime_env_expected" "$runtime_mode" \
  >>"$SNAPSHOT_AUDIT_LOG"

if [ "$stage" = validate ]; then
  printf '%s\n' 'ATLAS_URL=mysql://mutated:ORIGINAL_ENV_CHANGED@changed.invalid/db' >"$ENV_FILE"
fi

capture_count=0
capture_mode=missing
for capture_file in "$TMPDIR"/coze-publish-dev-atlas.*; do
  [ -f "$capture_file" ] || continue
  capture_count=$((capture_count + 1))
  if capture_mode=$(/usr/bin/stat -c '%a' "$capture_file" 2>/dev/null); then
    :
  elif capture_mode=$(/usr/bin/stat -f '%Lp' "$capture_file" 2>/dev/null); then
    :
  else
    capture_mode=unreadable
  fi
done
printf 'atlas-temp stage=%s count=%s mode=%s\n' \
  "$stage" "$capture_count" "$capture_mode" >>"$TEMP_AUDIT_LOG"

printf 'atlas %s stdout diagnostic\n' "$stage"
printf 'atlas %s stderr diagnostic\n' "$stage" >&2
if [ "$stage" = status ]; then
  if [ "$TEST_CASE" = pending-migration-success ]; then
    printf '%s\n' 'COZE_ATLAS_STATUS|PENDING|20260811000200|1'
  else
    printf '%s\n' 'COZE_ATLAS_STATUS|OK|20260812000100|0'
  fi
fi
printf 'full Atlas URL: %s\n' "$ATLAS_URL_VALUE"
printf 'alternate MySQL URI: %s\n' "$ALT_MYSQL_URI"
printf 'URL authority: %s\n' "$AUTHORITY_URI"
printf 'standalone userinfo: %s@authority.invalid\n' "$AUTHORITY_USERINFO"
printf 'password=%s\n' "$PASSWORD_FRAGMENT"
printf 'secret fragment: %s\n' "$SECRET_MARKER"
printf 'field stdout before API_TOKEN=%s field stdout after\n' "$API_TOKEN_VALUE"
printf 'field stderr before client_secret: %s field stderr after\n' "$CLIENT_SECRET_VALUE" >&2
printf 'field stdout before Access-Key=%s field stdout after\n' "$ACCESS_KEY_VALUE"
printf 'field stderr before Secret-Key: %s field stderr after\n' "$SECRET_KEY_VALUE" >&2
printf 'matrix password before password=%s matrix password after\n' "$PASSWORD_FIELD_VALUE"
printf 'matrix passwd before PaSsWd: %s matrix passwd after\n' "$PASSWD_FIELD_VALUE" >&2
printf 'matrix pwd before PWD=%s matrix pwd after\n' "$PWD_FIELD_VALUE"
printf 'matrix secret before secret: %s matrix secret after\n' "$SECRET_FIELD_VALUE" >&2
printf 'matrix token before ToKeN=%s matrix token after\n' "$TOKEN_FIELD_VALUE"
printf 'matrix api-space before Api Key: %s matrix api-space after\n' "$API_SPACE_KEY_VALUE" >&2
printf 'matrix api-underscore before api_key=%s matrix api-underscore after\n' "$API_UNDERSCORE_KEY_VALUE"
printf 'matrix api-hyphen before API-KEY: %s matrix api-hyphen after\n' "$API_HYPHEN_KEY_VALUE" >&2
printf 'matrix apikey before apikey=%s matrix apikey after\n' "$APIKEY_VALUE"
printf 'matrix access-space before Access Key: %s matrix access-space after\n' "$ACCESS_SPACE_KEY_VALUE" >&2
printf 'matrix access-underscore before access_key=%s matrix access-underscore after\n' "$ACCESS_UNDERSCORE_KEY_VALUE"
printf 'matrix accesskey before ACCESSKEY: %s matrix accesskey after\n' "$ACCESSKEY_VALUE" >&2
printf 'matrix secret-space before Secret Key=%s matrix secret-space after\n' "$SECRET_SPACE_KEY_VALUE"
printf 'matrix secret-underscore before secret_key: %s matrix secret-underscore after\n' "$SECRET_UNDERSCORE_KEY_VALUE" >&2
printf 'matrix secretkey before secretkey=%s matrix secretkey after\n' "$SECRETKEY_VALUE"

if [ "$TEST_CASE" = "$stage-failure" ]; then
  printf 'fake docker %s failure diagnostic\n' "$stage" >&2
  exit 1
fi
FAKE_DOCKER

cat >"$FAKE_BIN/forbidden-tool" <<'FAKE_FORBIDDEN_TOOL'
#!/usr/bin/env bash
set -euo pipefail

tool=${0##*/}
[ "$PATH" = "$FAKE_BIN" ] || {
  printf 'unexpected child PATH for %s\n' "$tool" >&2
  exit 94
}
printf '%s\n' "$PATH" >>"$PATH_AUDIT_LOG"
{
  printf '%s' "$tool"
  printf ' %s' "$@"
  printf '\n'
} >>"$COMMAND_LOG"
printf 'forbidden fake %s was invoked\n' "$tool" >&2
exit 99
FAKE_FORBIDDEN_TOOL

ln -s forbidden-tool "$FAKE_BIN/gh"
ln -s forbidden-tool "$FAKE_BIN/curl"

cat >"$FAKE_BIN/stat" <<'FAKE_STAT'
#!/usr/bin/env bash
set -euo pipefail

[ "$PATH" = "$FAKE_BIN" ] || {
  printf '%s\n' 'unexpected child PATH for stat' >&2
  exit 94
}
printf '%s\n' "$PATH" >>"$PATH_AUDIT_LOG"

if [ "${TEST_CASE:-}" = linux-stat-success ]; then
  target=
  for target in "$@"; do :; done
  case "${1:-}" in
    -c)
      if [ -d "$target" ]; then
        printf '%s\n' 700
      else
        printf '%s\n' 600
      fi
      exit 0
      ;;
    -f)
      printf '%s\n' 'simulated GNU stat filesystem output'
      exit 0
      ;;
  esac
fi

exec /usr/bin/stat "$@"
FAKE_STAT

cat >"$FAKE_BIN/tar" <<'FAKE_TAR'
#!/usr/bin/env bash
set -euo pipefail

[ "$PATH" = "$FAKE_BIN" ] || {
  printf '%s\n' 'unexpected child PATH for tar' >&2
  exit 94
}
printf '%s\n' "$PATH" >>"$PATH_AUDIT_LOG"

previous=
for argument in "$@"; do
  if [ "$previous" = -C ]; then
    printf '%s\n' "$argument" >"$SNAPSHOT_PATH_FILE"
    break
  fi
  previous=$argument
done

[ -s "$SNAPSHOT_PATH_FILE" ] || {
  printf '%s\n' 'fake tar did not receive a snapshot extraction directory' >&2
  exit 97
}
if [ "$TEST_CASE" = extract-failure ]; then
  printf '%s\n' 'fake tar extraction failure' >&2
  exit 1
fi

exec /usr/bin/tar "$@"
FAKE_TAR

chmod 700 "$FAKE_BIN/allowed-tool" "$FAKE_BIN/git" "$FAKE_BIN/docker" \
  "$FAKE_BIN/forbidden-tool" "$FAKE_BIN/stat" "$FAKE_BIN/tar"

write_valid_env() {
  local path=$1

  {
    printf '%s\n' '# local test credential'
    printf '\n'
    printf 'ATLAS_URL=%s\n' "$ATLAS_URL_VALUE"
  } >"$path"
  chmod 600 "$path"
}

setup_case() {
  local name=$1

  CASE_DIR=$TEST_ROOT/$name
  COMMAND_LOG=$CASE_DIR/commands.log
  GIT_PWD_LOG=$CASE_DIR/git-pwd.log
  PATH_AUDIT_LOG=$CASE_DIR/path-audit.log
  OUTPUT_LOG=$CASE_DIR/output.log
  FETCH_COUNT_FILE=$CASE_DIR/fetch-count
  HOME_DIR=$CASE_DIR/home
  ENV_FILE=$CASE_DIR/dev-atlas.env
  TEMP_DIR=$CASE_DIR/tmp
  TEMP_AUDIT_LOG=$CASE_DIR/temp-audit.log
  DRIFT_COMMAND_LOG=$CASE_DIR/drift-commands.log
  DRIFT_COUNT_FILE=$CASE_DIR/drift-count
  SNAPSHOT_AUDIT_LOG=$CASE_DIR/snapshot-audit.log
  SNAPSHOT_PATH_FILE=$CASE_DIR/snapshot-path
  ARCHIVE_TREE=$CASE_DIR/archive-tree
  ARCHIVE_FIXTURE=$CASE_DIR/target.tar
  UNRELATED_DIR=$CASE_DIR/unrelated
  mkdir -p "$CASE_DIR" "$HOME_DIR" "$TEMP_DIR" "$UNRELATED_DIR" \
    "$ARCHIVE_TREE/docker/atlas/migrations" "$ARCHIVE_TREE/.github"
  : >"$COMMAND_LOG"
  : >"$GIT_PWD_LOG"
  : >"$PATH_AUDIT_LOG"
  : >"$TEMP_AUDIT_LOG"
  : >"$DRIFT_COMMAND_LOG"
  : >"$SNAPSHOT_AUDIT_LOG"
  : >"$SNAPSHOT_PATH_FILE"
  printf '%s\n' 'h1:fixture' >"$ARCHIVE_TREE/docker/atlas/migrations/atlas.sum"
  printf '%s\n' 'CREATE TABLE fixture (id bigint PRIMARY KEY);' \
    >"$ARCHIVE_TREE/docker/atlas/migrations/20260811000200_expand_fixture_create_table.sql"
  printf '%s\n' 'ALTER TABLE fixture ADD COLUMN value bigint;' \
    >"$ARCHIVE_TREE/docker/atlas/migrations/20260812000100_expand_fixture_add_value.sql"
  printf '%s\n' 'env "dev" {}' >"$ARCHIVE_TREE/.github/atlas-dev.hcl"
  /usr/bin/tar -cf "$ARCHIVE_FIXTURE" -C "$ARCHIVE_TREE" \
    docker/atlas/migrations .github/atlas-dev.hcl
  write_valid_env "$ENV_FILE"

  export COMMAND_LOG GIT_PWD_LOG FETCH_COUNT_FILE EXPECTED_ORIGIN TARGET_SHA OTHER_SHA
  export PATH_AUDIT_LOG TEMP_AUDIT_LOG SNAPSHOT_AUDIT_LOG SNAPSHOT_PATH_FILE
  export DRIFT_COMMAND_LOG DRIFT_COUNT_FILE
  export ARCHIVE_FIXTURE ENV_FILE FAKE_BIN REPO_ROOT
  export ATLAS_URL_VALUE SECRET_MARKER ALT_MYSQL_URI AUTHORITY_URI AUTHORITY_USERINFO
  export PASSWORD_FRAGMENT
  export API_TOKEN_VALUE CLIENT_SECRET_VALUE ACCESS_KEY_VALUE SECRET_KEY_VALUE
  export PASSWORD_FIELD_VALUE PASSWD_FIELD_VALUE PWD_FIELD_VALUE SECRET_FIELD_VALUE
  export TOKEN_FIELD_VALUE API_SPACE_KEY_VALUE API_UNDERSCORE_KEY_VALUE
  export API_HYPHEN_KEY_VALUE APIKEY_VALUE ACCESS_SPACE_KEY_VALUE
  export ACCESS_UNDERSCORE_KEY_VALUE ACCESSKEY_VALUE SECRET_SPACE_KEY_VALUE
  export SECRET_UNDERSCORE_KEY_VALUE SECRETKEY_VALUE
  export TEST_CASE=$name
}

run_publish() {
  PATH="$FAKE_BIN" \
    HOME="$HOME_DIR" \
    TMPDIR="$TEMP_DIR" \
    ATLAS_ENV_FILE="$ENV_FILE" \
    GIT_BIN=git \
    DOCKER_BIN=docker \
    "$PUBLISH_SCRIPT" "$@" >"$OUTPUT_LOG" 2>&1
}

run_publish_with_nonempty_stdin() {
  printf '%s\n' 'injected-stdin-line' |
    PATH="$FAKE_BIN" \
      HOME="$HOME_DIR" \
      TMPDIR="$TEMP_DIR" \
      ATLAS_ENV_FILE="$ENV_FILE" \
      GIT_BIN=git \
      DOCKER_BIN=docker \
      "$PUBLISH_SCRIPT" "$@" >"$OUTPUT_LOG" 2>&1
}

run_publish_with_xtrace() {
  PATH="$FAKE_BIN" \
    HOME="$HOME_DIR" \
    TMPDIR="$TEMP_DIR" \
    ATLAS_ENV_FILE="$ENV_FILE" \
    GIT_BIN=git \
    DOCKER_BIN=docker \
    bash -x "$PUBLISH_SCRIPT" "$@" >"$OUTPUT_LOG" 2>&1
}

run_publish_with_default_env() (
  unset ATLAS_ENV_FILE
  PATH="$FAKE_BIN" \
    HOME="$HOME_DIR" \
    TMPDIR="$TEMP_DIR" \
    GIT_BIN=git \
    DOCKER_BIN=docker \
    "$PUBLISH_SCRIPT" "$@" >"$OUTPUT_LOG" 2>&1
)

run_publish_from_unrelated_cwd() (
  CDPATH= cd -- "$UNRELATED_DIR"
  PATH="$FAKE_BIN" \
    HOME="$HOME_DIR" \
    TMPDIR="$TEMP_DIR" \
    ATLAS_ENV_FILE="$ENV_FILE" \
    GIT_BIN=git \
    DOCKER_BIN=docker \
    "$PUBLISH_SCRIPT" "$@" >"$OUTPUT_LOG" 2>&1
)

assert_no_secret_output() {
  local secret

  for secret in \
    "$ATLAS_URL_VALUE" \
    "$SECRET_MARKER" \
    "$ALT_MYSQL_URI" \
    "$ALT_MYSQL_SECRET" \
    "$AUTHORITY_URI" \
    "$AUTHORITY_USERINFO" \
    "$AUTHORITY_SECRET" \
    "$PASSWORD_FRAGMENT" \
    "$API_TOKEN_VALUE" \
    "$CLIENT_SECRET_VALUE" \
    "$ACCESS_KEY_VALUE" \
    "$SECRET_KEY_VALUE" \
    "$PASSWORD_FIELD_VALUE" \
    "$PASSWD_FIELD_VALUE" \
    "$PWD_FIELD_VALUE" \
    "$SECRET_FIELD_VALUE" \
    "$TOKEN_FIELD_VALUE" \
    "$API_SPACE_KEY_VALUE" \
    "$API_UNDERSCORE_KEY_VALUE" \
    "$API_HYPHEN_KEY_VALUE" \
    "$APIKEY_VALUE" \
    "$ACCESS_SPACE_KEY_VALUE" \
    "$ACCESS_UNDERSCORE_KEY_VALUE" \
    "$ACCESSKEY_VALUE" \
    "$SECRET_SPACE_KEY_VALUE" \
    "$SECRET_UNDERSCORE_KEY_VALUE" \
    "$SECRETKEY_VALUE"; do
    assert_not_contains "$OUTPUT_LOG" "$secret" 'Atlas credential leaked into command output'
    assert_not_contains "$COMMAND_LOG" "$secret" 'Atlas credential leaked into command log'
  done
}

assert_sensitive_field_diagnostics() {
  assert_contains "$OUTPUT_LOG" \
    'field stdout before API_TOKEN [REDACTED_SECRET_FIELD] field stdout after' \
    'API_TOKEN field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'field stderr before client_secret [REDACTED_SECRET_FIELD] field stderr after' \
    'client_secret field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'field stdout before Access-Key [REDACTED_SECRET_FIELD] field stdout after' \
    'Access-Key field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'field stderr before Secret-Key [REDACTED_SECRET_FIELD] field stderr after' \
    'Secret-Key field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix password before password [REDACTED_SECRET_FIELD] matrix password after' \
    'password field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix passwd before PaSsWd [REDACTED_SECRET_FIELD] matrix passwd after' \
    'passwd field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix pwd before PWD [REDACTED_SECRET_FIELD] matrix pwd after' \
    'pwd field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix secret before secret [REDACTED_SECRET_FIELD] matrix secret after' \
    'secret field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix token before ToKeN [REDACTED_SECRET_FIELD] matrix token after' \
    'token field context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix api-space before Api Key [REDACTED_SECRET_FIELD] matrix api-space after' \
    'space-separated API key context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix api-underscore before api_key [REDACTED_SECRET_FIELD] matrix api-underscore after' \
    'api_key context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix api-hyphen before API-KEY [REDACTED_SECRET_FIELD] matrix api-hyphen after' \
    'api-key context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix apikey before apikey [REDACTED_SECRET_FIELD] matrix apikey after' \
    'apikey context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix access-space before Access Key [REDACTED_SECRET_FIELD] matrix access-space after' \
    'space-separated access key context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix access-underscore before access_key [REDACTED_SECRET_FIELD] matrix access-underscore after' \
    'access_key context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix accesskey before ACCESSKEY [REDACTED_SECRET_FIELD] matrix accesskey after' \
    'accesskey context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix secret-space before Secret Key [REDACTED_SECRET_FIELD] matrix secret-space after' \
    'space-separated secret key context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix secret-underscore before secret_key [REDACTED_SECRET_FIELD] matrix secret-underscore after' \
    'secret_key context was not preserved safely'
  assert_contains "$OUTPUT_LOG" \
    'matrix secretkey before secretkey [REDACTED_SECRET_FIELD] matrix secretkey after' \
    'secretkey context was not preserved safely'
}

assert_git_repo_root() {
  local observed_pwd

  [ -s "$GIT_PWD_LOG" ] || fail 'fake Git did not record any working directory'
  while IFS= read -r observed_pwd; do
    [ "$observed_pwd" = "$REPO_ROOT" ] || \
      fail "fake Git ran outside REPO_ROOT: $observed_pwd"
  done <"$GIT_PWD_LOG"
}

assert_closed_child_path() {
  local observed_path

  [ -s "$PATH_AUDIT_LOG" ] || fail 'the tested child PATH was not observed'
  while IFS= read -r observed_path; do
    [ "$observed_path" = "$FAKE_BIN" ] || \
      fail "tested child inherited an unexpected PATH: $observed_path"
  done <"$PATH_AUDIT_LOG"
}

assert_stage_diagnostics() {
  local stage=$1

  assert_contains "$OUTPUT_LOG" "atlas $stage stdout diagnostic" \
    "$stage stdout diagnostic was not preserved"
  assert_contains "$OUTPUT_LOG" "atlas $stage stderr diagnostic" \
    "$stage stderr diagnostic was not preserved"
}

assert_capture_security() {
  local expected_count=$1
  local capture_file

  assert_count "$TEMP_AUDIT_LOG" 'atlas-temp ' "$expected_count" \
    'temporary capture was not audited once per Docker call'
  assert_count "$TEMP_AUDIT_LOG" 'count=1 mode=600' "$expected_count" \
    'Atlas output was not captured in exactly one mode-600 temporary file'
  for capture_file in "$TEMP_DIR"/coze-publish-dev-atlas.*; do
    [ ! -e "$capture_file" ] || fail 'Atlas output temporary file was not cleaned up'
  done
}

assert_snapshot_security() {
  local expected_docker_count=$1
  local snapshot_root
  local runtime_env
  local env_line

  assert_count "$SNAPSHOT_AUDIT_LOG" 'snapshot stage=' "$expected_docker_count" \
    'snapshot was not audited once per Docker call'
  if [ "$expected_docker_count" -eq 0 ]; then
    return 0
  fi

  [ -s "$SNAPSHOT_PATH_FILE" ] || fail 'snapshot extraction path was not recorded'
  IFS= read -r snapshot_root <"$SNAPSHOT_PATH_FILE"
  runtime_env=$snapshot_root/atlas-runtime.env
  case "$snapshot_root" in
    "$TEMP_DIR"/coze-publish-dev-snapshot.*) ;;
    *) fail "snapshot was not created under the test temp directory: $snapshot_root" ;;
  esac
  [ "$snapshot_root" != "$REPO_ROOT" ] || fail 'snapshot reused the live repository root'
  assert_count "$SNAPSHOT_AUDIT_LOG" "root=$snapshot_root " "$expected_docker_count" \
    'Atlas stages did not share one snapshot root'
  assert_count "$SNAPSHOT_AUDIT_LOG" 'root-mode=700' "$expected_docker_count" \
    'Atlas snapshot was not mode 700'
  assert_count "$SNAPSHOT_AUDIT_LOG" "runtime=$runtime_env " "$expected_docker_count" \
    'Atlas stages did not share one runtime env file'
  assert_count "$SNAPSHOT_AUDIT_LOG" 'runtime-mode=600 value=original' \
    "$expected_docker_count" \
    'runtime env was not mode 600 or changed after the original env was mutated'
  assert_not_contains "$COMMAND_LOG" "$REPO_ROOT/docker/atlas/migrations:/migrations:ro" \
    'Docker mounted migrations from the live worktree'
  assert_not_contains "$COMMAND_LOG" "$REPO_ROOT/.github/atlas-dev.hcl:/atlas.hcl:ro" \
    'Docker mounted Atlas config from the live worktree'
  assert_not_contains "$COMMAND_LOG" "--env-file $ENV_FILE" \
    'Docker received the original Atlas env file'

  env_line=
  IFS= read -r env_line <"$ENV_FILE" || fail 'mutated original env could not be read'
  [ "$env_line" = 'ATLAS_URL=mysql://mutated:ORIGINAL_ENV_CHANGED@changed.invalid/db' ] || \
    fail 'fake Docker did not mutate the original env after validate'
}

assert_snapshot_cleaned() {
  local snapshot_root

  if [ -s "$SNAPSHOT_PATH_FILE" ]; then
    IFS= read -r snapshot_root <"$SNAPSHOT_PATH_FILE"
    [ ! -e "$snapshot_root" ] || fail 'Atlas snapshot directory was not cleaned up'
  fi
}

assert_no_forbidden_tool_calls() {
  if grep -E '^(gh|curl)( |$)' "$COMMAND_LOG" >/dev/null; then
    fail 'publish script invoked a forbidden gh or curl command'
  fi
}

assert_success_command_log() {
  local expected_log=$CASE_DIR/expected-commands.log
  local snapshot_root
  local runtime_env

  [ -s "$SNAPSHOT_PATH_FILE" ] || fail 'success case did not record a snapshot path'
  IFS= read -r snapshot_root <"$SNAPSHOT_PATH_FILE"
  runtime_env=$snapshot_root/atlas-runtime.env

  {
    printf '%s\n' 'git symbolic-ref --quiet --short HEAD'
    printf '%s\n' 'git status --porcelain --untracked-files=all'
    printf '%s\n' 'git rev-parse HEAD'
    printf '%s\n' 'git fetch --no-tags origin dev'
    printf '%s\n' 'git rev-parse FETCH_HEAD'
    printf 'git cat-file -e %s^{commit}\n' "$TARGET_SHA"
    printf 'git merge-base --is-ancestor %s %s\n' "$EXPECTED_ORIGIN" "$TARGET_SHA"
    printf 'git cat-file -e %s^{commit}\n' "$EXPECTED_ORIGIN"
    printf 'git cat-file -e %s^{commit}\n' "$TARGET_SHA"
    printf 'git diff --name-status --no-renames %s %s -- docker/atlas/migrations/*.sql\n' \
      "$EXPECTED_ORIGIN" "$TARGET_SHA"
    printf 'git archive --format=tar %s -- docker/atlas/migrations .github/atlas-dev.hcl\n' \
      "$TARGET_SHA"
    printf 'docker run --rm -v %s/docker/atlas/migrations:/migrations:ro %s migrate validate --dir file:///migrations\n' \
      "$snapshot_root" "$ATLAS_IMAGE"
    printf 'docker run --rm --env-file %s -v %s/docker/atlas/migrations:/migrations:ro -v %s/.github/atlas-dev.hcl:/atlas.hcl:ro %s migrate status --config file:///atlas.hcl --env dev --format COZE_ATLAS_STATUS|{{ .Status }}|{{ .Current }}|{{ .Count }}\n' \
      "$runtime_env" "$snapshot_root" "$snapshot_root" "$ATLAS_IMAGE"
    printf 'docker run --rm --env-file %s -v %s/docker/atlas/migrations:/migrations:ro -v %s/.github/atlas-dev.hcl:/atlas.hcl:ro %s migrate apply --config file:///atlas.hcl --env dev\n' \
      "$runtime_env" "$snapshot_root" "$snapshot_root" "$ATLAS_IMAGE"
    printf '%s\n' 'git status --porcelain --untracked-files=all'
    printf '%s\n' 'git rev-parse HEAD'
    printf '%s\n' 'git fetch --no-tags origin dev'
    printf '%s\n' 'git rev-parse FETCH_HEAD'
    printf 'git cat-file -e %s^{commit}\n' "$TARGET_SHA"
    printf 'git merge-base --is-ancestor %s %s\n' "$EXPECTED_ORIGIN" "$TARGET_SHA"
    printf 'git push origin %s:refs/heads/dev\n' "$TARGET_SHA"
  } >"$expected_log"

  if ! cmp -s "$expected_log" "$COMMAND_LOG"; then
    diff -u "$expected_log" "$COMMAND_LOG" >&2 || true
    fail 'success command log did not exactly match the required transaction'
  fi
}

assert_production_forbidden_tokens_absent() {
  local forbidden_pattern

  forbidden_pattern='(^|[^[:alnum:]_])(gh|curl)([^[:alnum:]_]|$)|api\.github\.com|github\.com/api|workflow_dispatch|(^|[^[:alnum:]_])hook([^[:alnum:]_]|$)|--url([^[:alnum:]-]|$)|--force-with-lease([^[:alnum:]-]|$)|--force([^[:alnum:]-]|$)'
  if grep -En -- "$forbidden_pattern" "$PUBLISH_SCRIPT" >/dev/null; then
    fail 'production script contains a forbidden release token'
  fi
}

assert_no_docker() {
  assert_count "$COMMAND_LOG" 'docker ' 0 'Docker ran for a rejected case'
  assert_capture_security 0
  assert_snapshot_security 0
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_git_repo_root
}

assert_no_push() {
  assert_count "$COMMAND_LOG" 'git push ' 0 'push ran for a rejected case'
}

assert_drift_transaction_count() {
  local expected=$1

  assert_count "$DRIFT_COMMAND_LOG" ' schema diff ' "$expected" \
    'schema drift inspection ran the wrong number of times'
  assert_count "$DRIFT_COMMAND_LOG" 'docker network create ' "$expected" \
    'schema drift network creation ran the wrong number of times'
  assert_count "$DRIFT_COMMAND_LOG" 'docker run -d ' "$expected" \
    'schema drift MySQL startup ran the wrong number of times'
  assert_count "$DRIFT_COMMAND_LOG" 'docker rm -f ' "$expected" \
    'schema drift MySQL cleanup ran the wrong number of times'
  assert_count "$DRIFT_COMMAND_LOG" 'docker network rm ' "$expected" \
    'schema drift network cleanup ran the wrong number of times'
}

run_rejected_case() {
  local name=$1
  local expected_error=$2
  local expected_docker_count=$3
  local expected_push_count=$4
  shift 4

  setup_case "$name"
  if run_publish "$@"; then
    fail "$name unexpectedly succeeded"
  fi
  assert_contains "$OUTPUT_LOG" "$expected_error" "$name did not report the expected failure"
  assert_count "$COMMAND_LOG" 'docker ' "$expected_docker_count" \
    "$name ran the wrong number of Docker commands"
  assert_count "$COMMAND_LOG" 'git push ' "$expected_push_count" \
    "$name ran the wrong number of push commands"
  assert_capture_security "$expected_docker_count"
  assert_snapshot_security "$expected_docker_count"
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_no_forbidden_tool_calls
  if grep -E '^git( |$)' "$COMMAND_LOG" >/dev/null; then
    assert_git_repo_root
  fi
  assert_no_secret_output
}

test_success() {
  setup_case success
  run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA" || fail 'valid publish flow failed'

  assert_success_command_log
  assert_stage_diagnostics validate
  assert_stage_diagnostics status
  assert_stage_diagnostics apply
  assert_contains "$OUTPUT_LOG" '[REDACTED' 'redacted Atlas diagnostics contain no marker'
  assert_contains "$OUTPUT_LOG" \
    "pushed audited dev revision $TARGET_SHA; GitHub and Baota own all remaining steps" \
    'success message did not transfer all remaining work to GitHub and Baota'
  assert_capture_security 3
  assert_snapshot_security 3
  assert_drift_transaction_count 2
  assert_contains "$DRIFT_COMMAND_LOG" '-e EXPECTED_VERSION=20260812000100' \
    'success flow did not compare the expected target migration version'
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_no_forbidden_tool_calls
  assert_git_repo_root
  assert_sensitive_field_diagnostics
  assert_no_secret_output
}

test_status_only() {
  setup_case status-only-success
  run_publish --status "$EXPECTED_ORIGIN" "$TARGET_SHA" || \
    fail 'valid status-only flow failed'

  assert_count "$COMMAND_LOG" 'docker ' 2 \
    'status-only flow did not run exactly validate and status'
  assert_contains "$COMMAND_LOG" \
    'migrate validate --dir file:///migrations' \
    'status-only flow did not validate the migration snapshot'
  assert_contains "$COMMAND_LOG" \
    'migrate status --config file:///atlas.hcl --env dev' \
    'status-only flow did not inspect Atlas status'
  assert_not_contains "$COMMAND_LOG" 'migrate apply' \
    'status-only flow applied a migration'
  assert_count "$COMMAND_LOG" 'git fetch --no-tags origin dev' 2 \
    'status-only flow did not recheck the remote baseline'
  assert_count "$COMMAND_LOG" 'git push ' 0 \
    'status-only flow attempted a push'
  assert_drift_transaction_count 1
  assert_contains "$OUTPUT_LOG" \
    "inspected Atlas status for audited dev revision $TARGET_SHA; no migration was applied and no push was attempted" \
    'status-only success message did not state the read-only boundary'
  assert_stage_diagnostics validate
  assert_stage_diagnostics status
  assert_capture_security 2
  assert_snapshot_security 2
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_no_forbidden_tool_calls
  assert_git_repo_root
  assert_sensitive_field_diagnostics
  assert_no_secret_output
}

test_pending_migration_uses_current_then_target_version() {
  setup_case pending-migration-success
  run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA" || \
    fail 'pending migration publish flow failed'

  assert_drift_transaction_count 2
  assert_contains "$DRIFT_COMMAND_LOG" '-e EXPECTED_VERSION=20260811000200' \
    'pre-apply drift did not compare the current migration version'
  assert_contains "$DRIFT_COMMAND_LOG" '-e EXPECTED_VERSION=20260812000100' \
    'post-apply drift did not compare the target migration version'
  assert_contains "$OUTPUT_LOG" 'current=20260811000200 pending=1' \
    'pending migration status was not parsed'
  assert_count "$COMMAND_LOG" "git push origin $TARGET_SHA:refs/heads/dev" 1 \
    'pending migration flow did not complete the exact push'
  assert_snapshot_cleaned
  assert_no_secret_output
}

test_nonempty_stdin_is_ignored() {
  setup_case nonempty-stdin-success
  run_publish_with_nonempty_stdin "$EXPECTED_ORIGIN" "$TARGET_SHA" || \
    fail 'publish flow consumed injected stdin'
  assert_count "$COMMAND_LOG" 'docker ' 3 \
    'nonempty stdin flow did not complete all Atlas stages'
  assert_count "$COMMAND_LOG" "git push origin $TARGET_SHA:refs/heads/dev" 1 \
    'nonempty stdin flow did not complete the exact push'
  assert_capture_security 3
  assert_snapshot_security 3
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_git_repo_root
  assert_no_secret_output
}

test_xtrace_does_not_leak_credentials() {
  setup_case xtrace-success
  run_publish_with_xtrace "$EXPECTED_ORIGIN" "$TARGET_SHA" || \
    fail 'xtrace publish flow failed'
  assert_stage_diagnostics status
  assert_sensitive_field_diagnostics
  assert_capture_security 3
  assert_snapshot_security 3
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_git_repo_root
  assert_no_secret_output
}

test_linux_stat_mode_detection() {
  setup_case linux-stat-success
  run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA" || \
    fail 'GNU stat-compatible publish flow failed'
  assert_count "$COMMAND_LOG" 'docker ' 3 'GNU stat-compatible flow did not run Atlas'
  assert_count "$COMMAND_LOG" "git push origin $TARGET_SHA:refs/heads/dev" 1 \
    'GNU stat-compatible flow did not push the exact refspec'
  assert_capture_security 3
  assert_snapshot_security 3
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_git_repo_root
  assert_no_secret_output
}

test_default_env_path() {
  setup_case default-env-success
  mkdir -p "$HOME_DIR/.config/coze-studio"
  mv "$ENV_FILE" "$HOME_DIR/.config/coze-studio/dev-atlas.env"
  ENV_FILE=$HOME_DIR/.config/coze-studio/dev-atlas.env

  run_publish_with_default_env "$EXPECTED_ORIGIN" "$TARGET_SHA" || \
    fail 'default Atlas env path publish flow failed'
  assert_count "$COMMAND_LOG" "--env-file $ENV_FILE" 0 \
    'default Atlas env path was passed directly to Docker'
  assert_capture_security 3
  assert_snapshot_security 3
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_git_repo_root
  assert_no_secret_output
}

test_unrelated_cwd_uses_repo_root() {
  setup_case unrelated-cwd
  run_publish_from_unrelated_cwd "$EXPECTED_ORIGIN" "$TARGET_SHA" || \
    fail 'publish from an unrelated cwd failed'
  assert_git_repo_root
  assert_capture_security 3
  assert_snapshot_security 3
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_no_secret_output
}

test_snapshot_creation_failures() {
  run_rejected_case archive-failure 'failed to create Atlas snapshot' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  assert_contains "$COMMAND_LOG" \
    "git archive --format=tar $TARGET_SHA -- docker/atlas/migrations .github/atlas-dev.hcl" \
    'archive failure did not target the exact audited revision and Atlas inputs'

  run_rejected_case extract-failure 'failed to create Atlas snapshot' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  assert_contains "$COMMAND_LOG" \
    "git archive --format=tar $TARGET_SHA -- docker/atlas/migrations .github/atlas-dev.hcl" \
    'extract failure did not target the exact audited revision and Atlas inputs'
}

assert_no_repo_temp_artifacts() {
  local artifact

  for artifact in \
    "$REPO_TMP_DIR"/coze-publish-dev-snapshot.* \
    "$REPO_TMP_DIR"/coze-publish-dev-atlas.*; do
    [ ! -e "$artifact" ] && [ ! -L "$artifact" ] || \
      fail "publish temp artifact was left inside the repository: $artifact"
  done
}

run_rejected_repo_tmp_case() {
  local name=$1

  assert_no_repo_temp_artifacts
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail "$name unexpectedly succeeded"
  fi
  assert_no_repo_temp_artifacts
  assert_contains "$OUTPUT_LOG" 'publish temp root must be outside the repository' \
    "$name did not reject the physical repository temp root"
  assert_count "$COMMAND_LOG" 'git archive ' 0 \
    "$name reached archive before rejecting the temp root"
  assert_count "$COMMAND_LOG" 'docker ' 0 "$name reached Docker"
  assert_count "$COMMAND_LOG" 'git push ' 0 "$name reached push"
  assert_capture_security 0
  assert_snapshot_security 0
  assert_snapshot_cleaned
  assert_closed_child_path
  assert_git_repo_root
  assert_no_forbidden_tool_calls
  assert_no_secret_output
}

test_repository_tmp_roots_are_rejected() {
  REPO_TMP_DIR=$REPO_ROOT/deploy/dev/tests

  setup_case repo-tmpdir
  TEMP_DIR=$REPO_TMP_DIR
  run_rejected_repo_tmp_case repo-tmpdir

  setup_case repo-symlink-tmpdir
  rmdir "$TEMP_DIR"
  ln -s "$REPO_TMP_DIR" "$TEMP_DIR"
  run_rejected_repo_tmp_case repo-symlink-tmpdir
}

test_argument_validation() {
  run_rejected_case missing-argument 'expected exactly two arguments' 0 0 "$EXPECTED_ORIGIN"
  run_rejected_case extra-argument 'expected exactly two arguments' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA" "$OTHER_SHA"
  run_rejected_case invalid-expected-sha 'expected origin/dev SHA must be exactly 40 hexadecimal characters' \
    0 0 not-a-sha "$TARGET_SHA"
  run_rejected_case invalid-target-sha 'target dev SHA must be exactly 40 hexadecimal characters' \
    0 0 "$EXPECTED_ORIGIN" 1234
}

test_git_preconditions() {
  run_rejected_case wrong-branch 'current branch must be dev' 0 0 "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case dirty-worktree 'worktree must be clean' 0 0 "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case wrong-head 'HEAD does not match target dev SHA' 0 0 "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case stale-origin 'fetched origin/dev does not match expected SHA' \
    0 0 "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case stale-fetched-head 'fetched origin/dev does not match expected SHA' \
    0 0 "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case missing-target 'target commit does not exist' 0 0 "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case unrelated-target 'expected origin/dev is not an ancestor of target' \
    0 0 "$EXPECTED_ORIGIN" "$TARGET_SHA"
}

test_hidden_untracked_migration_is_dirty() {
  run_rejected_case hidden-untracked-migration 'worktree must be clean' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
}

test_migration_policy_gate() {
  run_rejected_case invalid-migration-name 'invalid migration filename' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case modified-historical-migration 'existing migration files are immutable' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case contract-migration 'requires a separate special migration approval' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case repair-migration 'requires a separate special migration approval' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case destructive-expand-migration 'expand migration contains destructive SQL' 0 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
}

test_env_validation() {
  setup_case missing-env
  rm "$ENV_FILE"
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail 'missing-env unexpectedly succeeded'
  fi
  assert_contains "$OUTPUT_LOG" 'Atlas env file must exist as a regular file' \
    'missing env file was not rejected'
  assert_no_docker
  assert_no_push
  assert_no_secret_output

  setup_case symlink-env
  mv "$ENV_FILE" "$ENV_FILE.target"
  ln -s "$ENV_FILE.target" "$ENV_FILE"
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail 'symlink-env unexpectedly succeeded'
  fi
  assert_contains "$OUTPUT_LOG" 'Atlas env file must not be a symbolic link' \
    'symlink env file was not rejected'
  assert_no_docker
  assert_no_push
  assert_no_secret_output

  setup_case repo-env
  ENV_FILE=$REPO_ROOT/.github/atlas-dev.hcl
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail 'repo-env unexpectedly succeeded'
  fi
  assert_contains "$OUTPUT_LOG" 'Atlas env file must be outside the repository' \
    'repository env file was not rejected'
  assert_no_docker
  assert_no_push
  assert_no_secret_output

  setup_case insecure-env-mode
  chmod 640 "$ENV_FILE"
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail 'insecure-env-mode unexpectedly succeeded'
  fi
  assert_contains "$OUTPUT_LOG" 'Atlas env file mode must be exactly 600' \
    'insecure env mode was not rejected'
  assert_no_docker
  assert_no_push
  assert_no_secret_output

  setup_case invalid-env-key
  printf '%s\n' 'UNSUPPORTED=value' >>"$ENV_FILE"
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail 'invalid-env-key unexpectedly succeeded'
  fi
  assert_contains "$OUTPUT_LOG" 'Atlas env file contains an unsupported entry' \
    'unsupported env key was not rejected'
  assert_no_docker
  assert_no_push
  assert_no_secret_output

  setup_case duplicate-atlas-url
  printf 'ATLAS_URL=%s\n' "$ATLAS_URL_VALUE" >>"$ENV_FILE"
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail 'duplicate-atlas-url unexpectedly succeeded'
  fi
  assert_contains "$OUTPUT_LOG" 'Atlas env file must contain exactly one ATLAS_URL entry' \
    'duplicate Atlas URL was not rejected'
  assert_no_docker
  assert_no_push
  assert_no_secret_output

  setup_case empty-atlas-url
  printf '%s\n' 'ATLAS_URL=' >"$ENV_FILE"
  chmod 600 "$ENV_FILE"
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail 'empty-atlas-url unexpectedly succeeded'
  fi
  assert_contains "$OUTPUT_LOG" 'ATLAS_URL must not be empty' 'empty Atlas URL was not rejected'
  assert_no_docker
  assert_no_push
  assert_no_secret_output

  setup_case invalid-atlas-scheme
  printf '%s\n' 'ATLAS_URL=postgres://user:password@db.invalid/coze_dev' >"$ENV_FILE"
  chmod 600 "$ENV_FILE"
  if run_publish "$EXPECTED_ORIGIN" "$TARGET_SHA"; then
    fail 'invalid-atlas-scheme unexpectedly succeeded'
  fi
  assert_contains "$OUTPUT_LOG" 'ATLAS_URL must use the mysql scheme' \
    'non-MySQL Atlas URL was not rejected'
  assert_no_docker
  assert_no_push
  assert_no_secret_output
}

test_atlas_and_race_failures() {
  run_rejected_case validate-failure 'Atlas migration validation failed' 1 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  assert_stage_diagnostics validate
  assert_contains "$OUTPUT_LOG" 'fake docker validate failure diagnostic' \
    'validate failure evidence was not preserved'

  run_rejected_case status-failure 'Atlas migration status failed' 2 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  assert_stage_diagnostics validate
  assert_stage_diagnostics status
  assert_contains "$OUTPUT_LOG" 'fake docker status failure diagnostic' \
    'status failure evidence was not preserved'

  run_rejected_case apply-failure 'Atlas migration apply failed' 3 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  assert_stage_diagnostics validate
  assert_stage_diagnostics status
  assert_stage_diagnostics apply
  assert_contains "$OUTPUT_LOG" 'fake docker apply failure diagnostic' \
    'apply failure evidence was not preserved'

  run_rejected_case second-fetch-race 'fetched origin/dev does not match expected SHA' 3 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  run_rejected_case push-failure 'dev push failed' 3 1 "$EXPECTED_ORIGIN" "$TARGET_SHA"
}

test_schema_drift_gate() {
  run_rejected_case pre-drift-failure 'schema drift detected during pre-apply' 2 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  assert_drift_transaction_count 1
  assert_count "$COMMAND_LOG" 'migrate apply' 0 \
    'pre-apply drift failure reached migration apply'

  run_rejected_case drift-inspection-failure \
    'schema drift inspection failed during pre-apply' 2 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  assert_drift_transaction_count 1
  assert_count "$COMMAND_LOG" 'migrate apply' 0 \
    'drift inspection failure reached migration apply'

  run_rejected_case post-drift-failure 'schema drift detected during post-apply' 3 0 \
    "$EXPECTED_ORIGIN" "$TARGET_SHA"
  assert_drift_transaction_count 2
  assert_count "$COMMAND_LOG" 'migrate apply' 1 \
    'post-apply drift failure did not occur after exactly one migration apply'
}

assert_production_forbidden_tokens_absent
test_hidden_untracked_migration_is_dirty
test_migration_policy_gate
test_unrelated_cwd_uses_repo_root
test_repository_tmp_roots_are_rejected
test_snapshot_creation_failures
test_success
test_status_only
test_pending_migration_uses_current_then_target_version
test_nonempty_stdin_is_ignored
test_xtrace_does_not_leak_credentials
test_linux_stat_mode_detection
test_default_env_path
test_argument_validation
test_git_preconditions
test_env_validation
test_atlas_and_race_failures
test_schema_drift_gate

printf '%s\n' 'publish dev contract: passed'
