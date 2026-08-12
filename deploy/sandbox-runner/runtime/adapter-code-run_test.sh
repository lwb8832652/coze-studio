#!/bin/sh
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

set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
INPUT=/tmp/newx-input.json
cleanup() { rm -f "$INPUT"; }
trap cleanup EXIT

cat >"$INPUT" <<'JSON'
{"schema":"coze.sandbox.execute.v1","scope":"agent","workload_kind":"agent","idempotency_key":"code-test","deadline":"2030-01-01T00:00:00Z","policy":{"max_output_bytes":4096,"allowed_executables":["python3"]},"entrypoint":"agent/code/run","stdin":"eyJzY2hlbWEiOiJjb3plLnNhbmRib3guY29kZS5pbnZva2UudjEiLCJjbGllbnRfdmVyc2lvbiI6IjEiLCJvcGVyYXRpb25faWQiOiJjb2RlLXRlc3QiLCJsYW5ndWFnZSI6IlB5dGhvbiIsImNvZGUiOiJhc3luYyBkZWYgbWFpbihhcmdzKTpcbiAgICByZXR1cm4ge1wib2tcIjogYXJncy5wYXJhbXNbXCJ2YWx1ZVwiXX0iLCJwYXJhbXMiOnsidmFsdWUiOjF9fQ=="}
JSON
sh "$ROOT/adapter-agent-code" | grep -F '"schema":"coze.sandbox.code.result.v1"' >/dev/null

cat >"$INPUT" <<'JSON'
{"schema":"coze.sandbox.execute.v1","scope":"agent","workload_kind":"agent","idempotency_key":"code-test","deadline":"2030-01-01T00:00:00Z","policy":{"max_output_bytes":4096,"allowed_executables":["python3"]},"entrypoint":"sh","stdin":""}
JSON
if sh "$ROOT/adapter-agent-code" >/dev/null 2>&1; then
  echo "adapter accepted an arbitrary entrypoint" >&2
  exit 1
fi

cat >"$INPUT" <<'JSON'
{"schema":"coze.sandbox.execute.v1","scope":"agent","workload_kind":"agent","idempotency_key":"code-test","deadline":"2030-01-01T00:00:00Z","policy":{"max_output_bytes":1048576,"allowed_executables":["python3"]},"entrypoint":"agent/code/run","stdin":"eyJzY2hlbWEiOiJjb3plLnNhbmRib3guY29kZS5pbnZva2UudjEiLCJjbGllbnRfdmVyc2lvbiI6IjEiLCJvcGVyYXRpb25faWQiOiJjb2RlLXRlc3QiLCJsYW5ndWFnZSI6IlB5dGhvbiIsImNvZGUiOiJhc3luYyBkZWYgbWFpbihhcmdzKTpcbiAgICBwcmludChcInhcIiogNzAwMDApXG4gICAgcmV0dXJuIHt9IiwicGFyYW1zIjp7fX0="}
JSON
if sh "$ROOT/adapter-agent-code" >/dev/null 2>&1; then
  echo "adapter accepted excessive user output" >&2
  exit 1
fi

cat >"$INPUT" <<'JSON'
{"schema":"coze.sandbox.execute.v1","scope":"plugin","workload_kind":"plugin","idempotency_key":"code-test","deadline":"2030-01-01T00:00:00Z","policy":{"max_output_bytes":4096,"allowed_executables":["node"]},"entrypoint":"plugin/code/run","stdin":"eyJzY2hlbWEiOiJjb3plLnNhbmRib3guY29kZS5pbnZva2UudjEiLCJjbGllbnRfdmVyc2lvbiI6IjEiLCJvcGVyYXRpb25faWQiOiJjb2RlLXRlc3QiLCJsYW5ndWFnZSI6IkphdmFTY3JpcHQiLCJjb2RlIjoiYXN5bmMgZnVuY3Rpb24gbWFpbih7cGFyYW1zfSl7IHJldHVybiB7b2s6IHBhcmFtcy52YWx1ZX07IH0iLCJwYXJhbXMiOnsidmFsdWUiOjJ9fQ=="}
JSON
sh "$ROOT/adapter-plugin-code" | grep -F '"schema":"coze.sandbox.code.result.v1"' >/dev/null
