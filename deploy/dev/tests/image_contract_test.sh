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

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/../../.." && pwd)

fail() {
  printf 'image contract failure: %s\n' "$1" >&2
  exit 1
}

require_line() {
  file=$1
  pattern=$2
  message=$3
  grep -Eiq -- "$pattern" "$file" || fail "$message"
}

backend_dockerfile=$REPO_ROOT/backend/Dockerfile
frontend_dockerfile=$REPO_ROOT/frontend/Dockerfile
nginx_conf=$REPO_ROOT/deploy/dev/nginx/nginx.conf
default_conf=$REPO_ROOT/deploy/dev/nginx/default.conf

[ -f "$backend_dockerfile" ] || fail 'backend Dockerfile is missing'
[ -f "$frontend_dockerfile" ] || fail 'frontend Dockerfile is missing'
[ -f "$nginx_conf" ] || fail 'deployment nginx.conf is missing'
[ -f "$default_conf" ] || fail 'deployment default.conf is missing'

for dockerfile in "$backend_dockerfile" "$frontend_dockerfile"; do
  name=$(basename "$(dirname "$dockerfile")")
  require_line "$dockerfile" '^ARG[[:space:]]+GIT_REVISION(=|[[:space:]]|$)' "$name Dockerfile must declare ARG GIT_REVISION"
  require_line "$dockerfile" '^ARG[[:space:]]+SOURCE_URL(=|[[:space:]]|$)' "$name Dockerfile must declare ARG SOURCE_URL"
  final_stage=$(awk '
    /^FROM[[:space:]]/ { last=NR }
    { lines[NR]=$0 }
    END {
      for (i=last; i<=NR; i++) print lines[i]
    }
  ' "$dockerfile")
  printf '%s\n' "$final_stage" | grep -Eiq 'LABEL[[:space:]]+org\.opencontainers\.image\.revision=' || fail "$name final runtime stage must label org.opencontainers.image.revision"
  printf '%s\n' "$final_stage" | grep -Eiq 'LABEL[[:space:]]+org\.opencontainers\.image\.source=' || fail "$name final runtime stage must label org.opencontainers.image.source"
done

backend_final_stage=$(awk '
  /^FROM[[:space:]]/ { last=NR }
  { lines[NR]=$0 }
  END {
    for (i=last; i<=NR; i++) print lines[i]
  }
' "$backend_dockerfile")
printf '%s\n' "$backend_final_stage" | grep -Eiq '^ENV[[:space:]]+APP_REVISION=\$GIT_REVISION([[:space:]]|$)' || fail 'backend final runtime stage must set APP_REVISION from GIT_REVISION'

require_line "$frontend_dockerfile" 'COPY[[:space:]]+deploy/dev/nginx/nginx\.conf[[:space:]]+/etc/nginx/nginx\.conf' 'frontend Dockerfile must copy deployment nginx.conf'
require_line "$frontend_dockerfile" 'COPY[[:space:]]+deploy/dev/nginx/default\.conf[[:space:]]+/etc/nginx/conf\.d/default\.conf' 'frontend Dockerfile must copy deployment default.conf'
require_line "$frontend_dockerfile" '^EXPOSE[[:space:]]+80([[:space:]]|$)' 'frontend Dockerfile must expose port 80'

require_line "$default_conf" 'location[[:space:]]*=[[:space:]]*/healthz' 'deployment nginx must define an exact /healthz location'
require_line "$default_conf" 'proxy_pass[[:space:]]+http://coze-server:8888' 'deployment nginx must proxy to coze-server:8888'
expected_api_location='location ~ ^/(api|v[1-3]|admin|open_api)(/|$)'
grep -Fq -- "$expected_api_location" "$default_conf" || fail "deployment nginx must use the single suffix-boundary API location: $expected_api_location"
grep -Eiq 'minio|local_storage|sub_filter' "$nginx_conf" "$default_conf" && fail 'deployment nginx must not contain minio, local_storage, or sub_filter'

printf 'image contract: ok\n'
