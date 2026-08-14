#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

printf '%s\n' 'db_migrate_apply.sh 已停用，避免任意环境 DSN 执行声明式数据库变更。' >&2
printf '%s\n' '本地隔离数据库请运行: make db_local_migrate' >&2
printf '%s\n' '远程 dev 数据库请运行: deploy/dev/publish-dev.sh <origin-dev-sha> <target-dev-sha>' >&2
exit 1
