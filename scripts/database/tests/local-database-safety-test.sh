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

require_file_text() {
  local file=$1
  local pattern=$2
  local message=$3

  grep -Eq -- "$pattern" "$file" || fail "$message"
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
  "$REPO_ROOT/docker/docker-compose-oceanbase.yml"
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
  require_text "$mysql_block" "'127.0.0.1'" \
    "${compose_file##*/} MySQL healthcheck must target the final TCP listener"
  require_text "$mysql_block" "'--protocol=tcp'" \
    "${compose_file##*/} MySQL healthcheck must not use the initialization socket"
  require_text "$mysql_block" "'3306'" \
    "${compose_file##*/} MySQL healthcheck must target TCP port 3306"

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
atlas_hash_block=$(printf '%s\n' "$makefile" | awk '
  /^atlas-hash:/ {in_target=1}
  in_target && /^[[:alnum:]_.-]+:/ && $0 !~ /^atlas-hash:/ {exit}
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
require_text "$atlas_hash_block" "$ATLAS_IMAGE" \
  'atlas-hash must use the approved pinned Atlas image'
require_text "$atlas_hash_block" 'migrate hash --dir file:///migrations' \
  'atlas-hash must hash only the versioned migration directory'
forbid_text "$atlas_hash_block" '^([[:space:]]*@)?\(?cd .*&&[[:space:]]*atlas|[[:space:]]atlas[[:space:]]+migrate' \
  'atlas-hash must not depend on an unpinned host Atlas binary'

legacy_apply=$(<"$REPO_ROOT/scripts/setup/db_migrate_apply.sh")
forbid_text "$legacy_apply" 'schema[[:space:]]+apply|--auto-approve|ATLAS_URL' \
  'legacy db_migrate_apply.sh must not accept an arbitrary database target'
require_text "$legacy_apply" 'db_local_migrate' \
  'legacy db_migrate_apply.sh must point users to the safe local command'
require_text "$legacy_apply" 'publish-dev\.sh' \
  'legacy db_migrate_apply.sh must point remote dev changes to the publish workflow'

legacy_dump=$(<"$REPO_ROOT/scripts/setup/db_migrate_dump.sh")
[ -x "$REPO_ROOT/scripts/setup/db_migrate_dump.sh" ] || \
  fail 'legacy db_migrate_dump.sh must remain executable so it can fail with safe guidance'
forbid_text "$legacy_dump" 'ATLAS_URL|schema[[:space:]]+inspect|migrate[[:space:]]+diff|opencoze_latest_schema\.hcl' \
  'legacy db_migrate_dump.sh must not inspect arbitrary databases or generate executable snapshots'
require_text "$legacy_dump" '已停用' \
  'legacy db_migrate_dump.sh must fail closed with explicit guidance'
require_text "$legacy_dump" 'docker/atlas/migrations' \
  'legacy db_migrate_dump.sh must point users to versioned migrations'

for env_example in \
  "$REPO_ROOT/docker/.env.debug.example" \
  "$REPO_ROOT/docker/.env.example"; do
  env_source=$(<"$env_example")
  forbid_text "$env_source" '^export[[:space:]]+ATLAS_URL=' \
    "${env_example##*/} must not give the application a migration credential"
done
debug_env=$(<"$REPO_ROOT/docker/.env.debug.example")
forbid_text "$debug_env" '^export[[:space:]]+MYSQL_USER=root([[:space:]]|$)' \
  'debug example must not use a remote DDL-capable root account'
forbid_text "$debug_env" 'DML-only' \
  'debug example must preserve DDL required by runtime-owned resource tables'
require_text "$debug_env" 'dedicated non-root application account' \
  'debug example must require a dedicated non-root application account'
require_text "$debug_env" 'table_<id>' \
  'debug example must explain why the application account still needs scoped DDL'
forbid_text "$debug_env" 'sql\.tencentcdb\.com|gz-cynosdbmysql' \
  'debug example must not embed a specific shared database endpoint'

physical_table_file=$REPO_ROOT/backend/domain/memory/database/internal/physicaltable/physical.go
database_service_file=$REPO_ROOT/backend/domain/memory/database/service/database_impl.go
require_file_text "$physical_table_file" 'db\.CreateTable' \
  'resource database creation must remain covered by the credential contract'
require_file_text "$physical_table_file" 'db\.AlterTable' \
  'resource database editing must remain covered by the credential contract'
require_file_text "$physical_table_file" 'table_%d' \
  'runtime-owned resource table naming must remain explicit'
require_file_text "$database_service_file" '\.DropTable' \
  'resource database deletion must remain covered by the credential contract'

printf '%s\n' 'local database safety tests passed'
