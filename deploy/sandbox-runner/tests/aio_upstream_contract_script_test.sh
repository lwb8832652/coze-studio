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
#


set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/../../.." && pwd)"
probe_script="$script_dir/aio_upstream_contract_test.sh"

require_literal() {
  if ! grep -F -- "$1" "$probe_script" >/dev/null; then
    echo "missing official AIO probe contract: $1" >&2
    exit 1
  fi
}

reject_pattern() {
  if grep -E -- "$1" "$probe_script" >/dev/null; then
    echo "obsolete AIO probe contract remains: $1" >&2
    exit 1
  fi
}

if [[ -e "$repo_root/deploy/sandbox-runner/aio.lock.json" ]]; then
  echo "obsolete immutable AIO lock still exists" >&2
  exit 1
fi

require_literal 'image_ref="ghcr.io/agent-infra/sandbox:latest"'
require_literal '--security-opt seccomp=unconfined'
require_literal '-e OPENSSL_armcap=0'
require_literal '-p 127.0.0.1::8080'
require_literal 'assert not host["Privileged"]'
require_literal 'assert host.get("SecurityOpt") == ["seccomp=unconfined"]'
require_literal 'assert not container.get("Mounts")'

reject_pattern '@sha256:|imagetools|manifest mismatch|aio\.lock'
reject_pattern '--memory|--cpus|--pids-limit|--shm-size'
reject_pattern 'DISABLE_(BROWSER|JUPYTER|CODE_SERVER|MCP_BROWSER|VNC|NODEJS_REPL)'
reject_pattern 'JWT_PUBLIC_KEY|NEWX_AIO_JWT'
reject_pattern '--privileged|docker\.sock'
