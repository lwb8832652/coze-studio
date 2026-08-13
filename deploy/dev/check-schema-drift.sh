#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -Eeuo pipefail
set +x
umask 077

ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
MYSQL_IMAGE='mysql:8.4.5'
DOCKER_BIN=${DOCKER_BIN:-docker}
LOCAL_MYSQL_PASSWORD='coze-schema-drift-local-only'

migrations_dir=''
atlas_env_file=''
expected_version=''
check_stage=''
temp_root=''
network_name=''
mysql_name=''
network_created=false
mysql_created=false

error() {
  printf '[schema-drift] error: %s\n' "$*" >&2
}

usage() {
  error 'usage: check-schema-drift.sh --migrations-dir <dir> --atlas-env-file <file> --version <14-digit-version> --stage <pre-apply|post-apply>'
  exit 2
}

docker_cmd() {
  "$DOCKER_BIN" "$@"
}

file_mode() {
  local mode

  if mode=$(stat -c '%a' "$1" 2>/dev/null) && [[ "$mode" =~ ^[0-7]+$ ]]; then
    printf '%s\n' "$mode"
    return 0
  fi
  if mode=$(stat -f '%Lp' "$1" 2>/dev/null) && [[ "$mode" =~ ^[0-7]+$ ]]; then
    printf '%s\n' "$mode"
    return 0
  fi
  return 1
}

parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --migrations-dir)
        [ "$#" -ge 2 ] || usage
        migrations_dir=$2
        shift 2
        ;;
      --atlas-env-file)
        [ "$#" -ge 2 ] || usage
        atlas_env_file=$2
        shift 2
        ;;
      --version)
        [ "$#" -ge 2 ] || usage
        expected_version=$2
        shift 2
        ;;
      --stage)
        [ "$#" -ge 2 ] || usage
        check_stage=$2
        shift 2
        ;;
      *)
        usage
        ;;
    esac
  done
}

validate_inputs() {
  local mode
  local atlas_url_count
  local line
  local migrations_parent
  local migrations_name
  local physical_migrations_parent
  local atlas_env_parent
  local atlas_env_name
  local physical_atlas_env_parent

  [[ "$expected_version" =~ ^[0-9]{14}$ ]] || {
    error 'version must be a 14-digit migration version'
    return 1
  }
  case "$check_stage" in
    pre-apply|post-apply) ;;
    *)
      error 'stage must be pre-apply or post-apply'
      return 1
      ;;
  esac
  [ -d "$migrations_dir" ] && [ ! -L "$migrations_dir" ] || {
    error 'migrations directory must be a real directory'
    return 1
  }
  [ -f "$migrations_dir/atlas.sum" ] && [ ! -L "$migrations_dir/atlas.sum" ] || {
    error 'migrations directory must contain a regular atlas.sum'
    return 1
  }
  [ -f "$migrations_dir/${expected_version}"*.sql ] || {
    error 'requested migration version does not exist in the migration directory'
    return 1
  }
  [ -f "$atlas_env_file" ] && [ ! -L "$atlas_env_file" ] || {
    error 'Atlas env file must be a regular file'
    return 1
  }
  mode=$(file_mode "$atlas_env_file") || {
    error 'Atlas env file mode could not be read'
    return 1
  }
  [ "$mode" = 600 ] || {
    error 'Atlas env file mode must be exactly 600'
    return 1
  }

  atlas_url_count=0
  while IFS= read -r line || [ -n "$line" ]; do
    if [[ "$line" =~ ^[[:space:]]*$ ]] || [[ "$line" =~ ^[[:space:]]*# ]]; then
      continue
    fi
    case "$line" in
      ATLAS_URL=mysql://?*) atlas_url_count=$((atlas_url_count + 1)) ;;
      *)
        error 'Atlas env file contains an unsupported or invalid entry'
        return 1
        ;;
    esac
  done <"$atlas_env_file"
  [ "$atlas_url_count" -eq 1 ] || {
    error 'Atlas env file must contain exactly one MySQL ATLAS_URL'
    return 1
  }

  migrations_parent=$(dirname -- "$migrations_dir") || return 1
  migrations_name=$(basename -- "$migrations_dir") || return 1
  physical_migrations_parent=$(CDPATH= cd -- "$migrations_parent" && pwd -P) || return 1
  migrations_dir=$physical_migrations_parent/$migrations_name
  [ -d "$migrations_dir" ] && [ ! -L "$migrations_dir" ] || return 1

  atlas_env_parent=$(dirname -- "$atlas_env_file") || return 1
  atlas_env_name=$(basename -- "$atlas_env_file") || return 1
  physical_atlas_env_parent=$(CDPATH= cd -- "$atlas_env_parent" && pwd -P) || return 1
  atlas_env_file=$physical_atlas_env_parent/$atlas_env_name
  [ -f "$atlas_env_file" ] && [ ! -L "$atlas_env_file" ] || return 1
}

create_temp_root() {
  local requested_tmp=${TMPDIR:-/tmp}
  local physical_tmp
  local suffix

  [ -d "$requested_tmp" ] || return 1
  physical_tmp=$(CDPATH= cd -- "$requested_tmp" && pwd -P) || return 1
  temp_root=$(mktemp -d "$physical_tmp/coze-schema-drift.XXXXXX") || return 1
  [ -d "$temp_root" ] && [ ! -L "$temp_root" ] || return 1
  chmod 700 "$temp_root" || return 1
  suffix=${temp_root##*.}
  [[ "$suffix" =~ ^[[:alnum:]]+$ ]] || return 1
  network_name=coze-schema-drift-net-$suffix
  mysql_name=coze-schema-drift-mysql-$suffix
}

cleanup_resources() {
  local cleanup_failed=false

  if [ "$mysql_created" = true ]; then
    if ! docker_cmd rm -f "$mysql_name" >/dev/null 2>&1; then
      cleanup_failed=true
    fi
    mysql_created=false
  fi
  if [ "$network_created" = true ]; then
    if ! docker_cmd network rm "$network_name" >/dev/null 2>&1; then
      cleanup_failed=true
    fi
    network_created=false
  fi
  if [ -n "$temp_root" ] && [ -d "$temp_root" ] && [ ! -L "$temp_root" ]; then
    rm -rf -- "$temp_root" || cleanup_failed=true
  fi
  temp_root=''

  [ "$cleanup_failed" = false ]
}

start_dev_database() {
  local health
  local attempt

  docker_cmd network create "$network_name" >/dev/null || return 1
  network_created=true

  docker_cmd run -d \
    --name "$mysql_name" \
    --network "$network_name" \
    -e "MYSQL_ROOT_PASSWORD=$LOCAL_MYSQL_PASSWORD" \
    -e MYSQL_DATABASE=opencoze \
    --health-cmd="mysqladmin ping -h 127.0.0.1 -P3306 --protocol=tcp -uroot -p$LOCAL_MYSQL_PASSWORD --silent" \
    --health-interval=2s \
    --health-timeout=2s \
    --health-retries=30 \
    "$MYSQL_IMAGE" >/dev/null || return 1
  mysql_created=true

  for attempt in $(seq 1 45); do
    health=$(docker_cmd inspect --format '{{.State.Health.Status}}' "$mysql_name") || return 1
    case "$health" in
      healthy) return 0 ;;
      unhealthy) return 1 ;;
      starting) sleep 2 ;;
      *) return 1 ;;
    esac
  done
  return 1
}

run_schema_diff() {
  local diff_output=$temp_root/schema-diff.out
  local diff_error=$temp_root/schema-diff.err
  local dev_url="mysql://root:$LOCAL_MYSQL_PASSWORD@$mysql_name:3306/opencoze?charset=utf8mb4&parseTime=True"
  local status
  local marker_count=0
  local line
  local change_count=''

  : >"$diff_output"
  : >"$diff_error"
  chmod 600 "$diff_output" "$diff_error"

  if docker_cmd run --rm \
    --network "$network_name" \
    --env-file "$atlas_env_file" \
    -e "ATLAS_DEV_URL=$dev_url" \
    -e "EXPECTED_VERSION=$expected_version" \
    -v "$migrations_dir:/migrations:ro" \
    --entrypoint /bin/sh \
    "$ATLAS_IMAGE" \
    -c 'atlas schema diff --from "$ATLAS_URL" --to "file:///migrations?format=atlas&version=${EXPECTED_VERSION}" --dev-url "$ATLAS_DEV_URL" --exclude atlas_schema_revisions --format "COZE_SCHEMA_DIFF|{{ len .Changes }}"' \
    >"$diff_output" 2>"$diff_error"; then
    status=0
  else
    status=$?
  fi

  if [ "$status" -ne 0 ]; then
    error 'Atlas schema diff command returned a non-zero status'
    return 2
  fi
  while IFS= read -r line || [ -n "$line" ]; do
    case "$line" in
      COZE_SCHEMA_DIFF\|*)
        marker_count=$((marker_count + 1))
        change_count=${line#COZE_SCHEMA_DIFF|}
        ;;
    esac
  done <"$diff_output"
  if [ "$marker_count" -ne 1 ]; then
    error "Atlas schema diff output marker count was $marker_count instead of 1"
    return 2
  fi
  if [[ ! "$change_count" =~ ^[0-9]+$ ]]; then
    error 'Atlas schema diff change count was not numeric'
    return 2
  fi
  if [ "$change_count" -ne 0 ]; then
    error "Atlas schema diff reported $change_count change(s)"
    return 3
  fi
  return 0
}

main() {
  local operation_status=0
  local cleanup_status=0

  parse_args "$@"
  validate_inputs || exit 1
  create_temp_root || {
    error "schema drift inspection failed during $check_stage"
    exit 1
  }

  if ! start_dev_database; then
    error 'temporary MySQL did not become healthy'
    operation_status=2
  elif run_schema_diff; then
    operation_status=0
  else
    operation_status=$?
  fi

  if ! cleanup_resources; then
    cleanup_status=1
  fi
  if [ "$cleanup_status" -ne 0 ]; then
    error 'schema drift cleanup failed'
    exit 1
  fi

  case "$operation_status" in
    0)
      printf '[schema-drift] schema drift check passed during %s\n' "$check_stage"
      ;;
    3)
      error "schema drift detected during $check_stage"
      exit 1
      ;;
    *)
      error "schema drift inspection failed during $check_stage"
      exit 1
      ;;
  esac
}

trap 'cleanup_resources >/dev/null 2>&1 || true' EXIT
trap 'exit 1' HUP INT TERM
main "$@"
trap - EXIT HUP INT TERM
