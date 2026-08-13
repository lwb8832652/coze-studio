#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

printf '%s\n' 'db_migrate_dump.sh 已停用，禁止从任意数据库反向生成可执行 schema 快照。' >&2
printf '%s\n' '数据库结构的唯一事实源是 docker/atlas/migrations 下的版本化 migration。' >&2
printf '%s\n' '创建、校验和发布 migration 请按 docs/superpowers/runbooks/project-operations.md 操作。' >&2
exit 1
