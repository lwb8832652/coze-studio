#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd -P)
ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'

fail() {
  printf 'local database safety test failure: %s\n' "$*" >&2
  exit 1
}

require_text() {
  local text=$1
  local pattern=$2
  local message=$3

  printf '%s\n' "$text" | grep -Eq -- "$pattern" || fail "$message"
}

forbid_text() {
  local text=$1
  local pattern=$2
  local message=$3

  if printf '%s\n' "$text" | grep -Eiq -- "$pattern"; then
    fail "$message"
  fi
}

service_block() {
  local file=$1
  local service=$2

  awk -v service="$service" '
    $0 == "  " service ":" {
      in_service=1
    }
    in_service && $0 ~ /^  [^[:space:]]/ && $0 != "  " service ":" {
      exit
    }
    in_service && /^[^[:space:]]/ {
      exit
    }
    in_service {
      print
    }
  ' "$file"
}

compose_files=(
  "$REPO_ROOT/docker/docker-compose-debug.yml"
  "$REPO_ROOT/docker/docker-compose-oceanbase_debug.yml"
  "$REPO_ROOT/docker/docker-compose.yml"
)

for compose_file in "${compose_files[@]}"; do
  source=$(<"$compose_file")
  mysql_block=$(service_block "$compose_file" mysql)
  migrate_block=$(service_block "$compose_file" mysql-migrate-local)
  redis_block=$(service_block "$compose_file" redis)

  forbid_text "$source" 'mysql-setup-schema|mysql-setup-init-sql' \
    "${compose_file##*/} must not contain legacy schema/init services"
  forbid_text "$source" 'schema[[:space:]]+apply|--auto-approve|opencoze_latest_schema\.hcl' \
    "${compose_file##*/} must not contain declarative schema apply"
  forbid_text "$mysql_block" 'docker-entrypoint-initdb\.d|curl[[:space:]].*atlasgo|entrypoint:' \
    "${compose_file##*/} mysql startup must not execute schema/bootstrap scripts"

  require_text "$migrate_block" '^  mysql-migrate-local:$' \
    "${compose_file##*/} must define mysql-migrate-local"
  require_text "$migrate_block" "image: ${ATLAS_IMAGE//\//\\/}" \
    "${compose_file##*/} must pin the approved Atlas image"
  require_text "$migrate_block" "profiles: \['local-db-migrate'\]" \
    "${compose_file##*/} migration service must use only local-db-migrate profile"
  require_text "$migrate_block" 'mysql:3306/opencoze' \
    "${compose_file##*/} migration target must be the Compose-local mysql:3306 database"
  require_text "$migrate_block" './atlas/migrations:/migrations:ro' \
    "${compose_file##*/} migration service must mount versioned migrations read-only"
  require_text "$migrate_block" 'condition: service_healthy' \
    "${compose_file##*/} migration service must wait for local MySQL health"
  forbid_text "$migrate_block" 'MYSQL_HOST|MYSQL_PORT|ATLAS_URL|env_file:' \
    "${compose_file##*/} migration service must not inherit an external database target"
  forbid_text "$redis_block" 'mysql-migrate-local|mysql-setup|schema|init-sql' \
    "${compose_file##*/} redis must not trigger database migration or initialization"
done

makefile=$(<"$REPO_ROOT/Makefile")
sync_db_block=$(printf '%s\n' "$makefile" | awk '
  /^sync_db:/ {in_target=1}
  in_target && /^[[:alnum:]_.-]+:/ && $0 !~ /^sync_db:/ {exit}
  in_target {print}
')
local_migrate_block=$(printf '%s\n' "$makefile" | awk '
  /^db_local_migrate:/ {in_target=1}
  in_target && /^[[:alnum:]_.-]+:/ && $0 !~ /^db_local_migrate:/ {exit}
  in_target {print}
')

require_text "$makefile" '^db_local_up:' 'Makefile must expose db_local_up'
require_text "$makefile" '^db_local_migrate:' 'Makefile must expose db_local_migrate'
require_text "$sync_db_block" '已停用' 'legacy sync_db target must fail closed with guidance'
require_text "$sync_db_block" 'exit 1' 'legacy sync_db target must return a failure status'
forbid_text "$sync_db_block" 'docker compose|db_migrate_apply' \
  'legacy sync_db target must not execute any database tool'
require_text "$local_migrate_block" "--profile local-db-migrate run --rm mysql-migrate-local" \
  'db_local_migrate must invoke only the explicit local migration service'

legacy_apply=$(<"$REPO_ROOT/scripts/setup/db_migrate_apply.sh")
forbid_text "$legacy_apply" 'schema[[:space:]]+apply|--auto-approve|ATLAS_URL' \
  'legacy db_migrate_apply.sh must not accept an arbitrary database target'
require_text "$legacy_apply" 'db_local_migrate' \
  'legacy db_migrate_apply.sh must point users to the safe local command'
require_text "$legacy_apply" 'publish-dev\.sh' \
  'legacy db_migrate_apply.sh must point remote dev changes to the publish workflow'

printf '%s\n' 'local database safety tests passed'
