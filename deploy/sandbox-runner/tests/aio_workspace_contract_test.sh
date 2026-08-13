#!/usr/bin/env bash
#
# Copyright 2025 coze-dev Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
upstream_contract="$script_dir/aio_upstream_contract_test.sh"
probe_source="$repo_root/backend/cmd/sandbox-aio-compat-probe/main.go"

test -f "$upstream_contract"
grep -Fq 'ghcr.io/agent-infra/sandbox:latest' "$upstream_contract"
grep -Fq -- '--security-opt seccomp=unconfined' "$upstream_contract"
grep -Fq -- '-e OPENSSL_armcap=0' "$upstream_contract"
grep -Fq -- '-e WORKSPACE=/mnt/user-data' "$upstream_contract"
grep -Fq -- '-p 127.0.0.1::8080' "$upstream_contract"
grep -Fq 'probeWorkspaceContract' "$probe_source"
grep -Fq '/mnt/user-data/' "$probe_source"
grep -Fq 'sudoFalse' "$probe_source"

if [[ "${NEWX_AIO_WORKSPACE_REAL_PROBE:-}" == "1" ]]; then
  exec bash "$upstream_contract"
fi

echo 'AIO workspace contract static checks passed; set NEWX_AIO_WORKSPACE_REAL_PROBE=1 for the real official-latest probe.'
