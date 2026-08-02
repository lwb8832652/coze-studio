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

env_file="${1:-}"
if [[ -z "$env_file" || ! -f "$env_file" ]]; then
  echo "usage: $0 <env-file>" >&2
  exit 1
fi

key_line="$(grep -E '^[[:space:]]*(export[[:space:]]+)?OBJECT_STORAGE_CREDENTIAL_KEY=' "$env_file" | tail -n 1 || true)"
key_value="${key_line#*=}"
key_value="$(printf '%s' "$key_value" | tr -d '[:space:]')"
key_value="${key_value#\"}"
key_value="${key_value%\"}"
key_value="${key_value#\'}"
key_value="${key_value%\'}"
if [[ -n "$key_value" ]]; then
  exit 0
fi

if ! command -v openssl >/dev/null 2>&1; then
  echo "openssl is required to generate OBJECT_STORAGE_CREDENTIAL_KEY" >&2
  exit 1
fi

credential_key="$(openssl rand -base64 32)"
tmp_file="$(mktemp "${env_file}.tmp.XXXXXX")"
trap 'rm -f "$tmp_file"' EXIT

awk -v replacement="export OBJECT_STORAGE_CREDENTIAL_KEY=\"${credential_key}\"" '
  BEGIN { replaced = 0 }
  /^[[:space:]]*(export[[:space:]]+)?OBJECT_STORAGE_CREDENTIAL_KEY=/ {
    if (!replaced) {
      print replacement
      replaced = 1
    }
    next
  }
  { print }
  END {
    if (!replaced) {
      print replacement
    }
  }
' "$env_file" > "$tmp_file"

chmod 600 "$tmp_file"
mv "$tmp_file" "$env_file"
trap - EXIT
echo "Generated OBJECT_STORAGE_CREDENTIAL_KEY in $env_file"
