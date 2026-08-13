#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd -P)
RUNBOOK=$REPO_ROOT/docs/superpowers/runbooks/project-operations.md
AGENTS=$REPO_ROOT/AGENTS.md

fail() {
  printf 'project operations documentation test failure: %s\n' "$*" >&2
  exit 1
}

require_literal() {
  local file=$1
  local expected=$2
  local message=$3

  grep -F -- "$expected" "$file" >/dev/null || fail "$message"
}

forbid_pattern() {
  local file=$1
  local pattern=$2
  local message=$3

  if grep -Eiq -- "$pattern" "$file"; then
    fail "$message"
  fi
}

[ -f "$RUNBOOK" ] || fail 'project-operations.md is missing'

for heading in \
  '# 项目运维手册' \
  '## 运行模式选择' \
  '## 默认本地开发（共享 dev 数据）' \
  '## 隔离本地数据库' \
  '## 配置文件与密钥' \
  '## 数据库 migration 命名' \
  '## dev 合并与发布' \
  '## 备份、PITR 与事故恢复' \
  '## 禁止事项'; do
  require_literal "$RUNBOOK" "$heading" "runbook is missing heading: $heading"
done

require_literal "$RUNBOOK" \
  'YYYYMMDDHHMMSS_{expand|data|contract|repair}_module_action.sql' \
  'runbook must define the stable migration filename format'
require_literal "$RUNBOOK" \
  '20260813103000_expand_plugin_code_create_tables.sql' \
  'runbook must include a concrete migration filename example'
require_literal "$RUNBOOK" 'make db_local_up' \
  'runbook must document the isolated local MySQL startup command'
require_literal "$RUNBOOK" 'make db_local_migrate' \
  'runbook must document the explicit local migration command'
require_literal "$RUNBOOK" 'deploy/dev/publish-dev.sh --status' \
  'runbook must document the read-only dev preflight command'
require_literal "$RUNBOOK" 'deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"' \
  'runbook must document the exact-SHA dev release command'
require_literal "$RUNBOOK" '全量只用于新基线或经批准的 checkpoint' \
  'runbook must constrain full schema snapshots'
require_literal "$RUNBOOK" '应用账号与 migration 账号必须分离' \
  'runbook must separate application and migration credentials'
require_literal "$RUNBOOK" '资源库动态表 `table_<id>`' \
  'runbook must document the runtime-owned resource table exception'
require_literal "$RUNBOOK" '漂移检查排除 `atlas_schema_revisions` 和 `table_*`' \
  'runbook must exclude migration metadata and runtime-owned tables from drift checks'
require_literal "$RUNBOOK" '数据恢复' \
  'runbook must distinguish schema repair from data recovery'

require_literal "$AGENTS" 'docs/superpowers/runbooks/project-operations.md' \
  'AGENTS.md must link the unified project operations runbook'
require_literal "$AGENTS" \
  '启动、停止、环境配置、Compose、数据库、迁移、dev 合并或发布' \
  'AGENTS.md must define when the unified runbook is mandatory'
require_literal "$AGENTS" '必须先完整阅读' \
  'AGENTS.md must require a complete runbook read'

for linked_doc in \
  "$REPO_ROOT/docs/superpowers/runbooks/local-debug-and-test.md" \
  "$REPO_ROOT/docs/superpowers/runbooks/dev-integration-audit.md" \
  "$REPO_ROOT/deploy/dev/README.md" \
  "$REPO_ROOT/docker/atlas/README.md" \
  "$REPO_ROOT/CLAUDE.md"; do
  require_literal "$linked_doc" 'docs/superpowers/runbooks/project-operations.md' \
    "${linked_doc#$REPO_ROOT/} must route database operations through the unified runbook"
done

for active_doc in \
  "$RUNBOOK" \
  "$REPO_ROOT/docs/superpowers/runbooks/local-debug-and-test.md" \
  "$REPO_ROOT/docs/superpowers/runbooks/dev-integration-audit.md" \
  "$REPO_ROOT/deploy/dev/README.md" \
  "$REPO_ROOT/docker/atlas/README.md" \
  "$REPO_ROOT/CLAUDE.md"; do
  forbid_pattern "$active_doc" 'atlas[[:space:]]+schema[[:space:]]+apply|schema[[:space:]]+apply[[:space:]].*--auto-approve' \
    "${active_doc#$REPO_ROOT/} must not instruct operators to run declarative schema apply"
done

for credential_doc in \
  "$RUNBOOK" \
  "$REPO_ROOT/docs/superpowers/context/project-context.md" \
  "$REPO_ROOT/docs/superpowers/runbooks/local-debug-and-test.md" \
  "$REPO_ROOT/docs/superpowers/specs/2026-08-13-dev-database-schema-safety-design.md" \
  "$REPO_ROOT/docs/superpowers/plans/2026-08-13-dev-database-schema-safety.md" \
  "$REPO_ROOT/docker/.env.debug.example" \
  "$REPO_ROOT/CLAUDE.md"; do
  forbid_pattern "$credential_doc" 'DML-only|应用账号.*不得拥有.*DDL|应用账号.*不拥有.*DDL' \
    "${credential_doc#$REPO_ROOT/} must not remove DDL required by runtime-owned resource tables"
done

printf '%s\n' 'project operations documentation tests passed'
