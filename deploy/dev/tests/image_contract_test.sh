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
CONTRACT_ROOT=${CONTRACT_ROOT:-$REPO_ROOT}

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

require_exact_line() {
  file=$1
  line=$2
  message=$3
  grep -Fqx -- "$line" "$file" || fail "$message"
}

require_min_count() {
  file=$1
  pattern=$2
  expected=$3
  message=$4
  count=$(grep -Eic -- "$pattern" "$file" || true)
  [ "$count" -ge "$expected" ] || fail "$message"
}

final_stage() {
  awk '
    /^FROM[[:space:]]/ { last=NR }
    { lines[NR]=$0 }
    END {
      for (i=last; i<=NR; i++) print lines[i]
    }
  ' "$1"
}

assert_dockerfile() {
  dockerfile=$1
  name=$2
  require_exact_line "$dockerfile" 'ARG GIT_REVISION=unknown' "$name Dockerfile must set the global GIT_REVISION default"
  require_exact_line "$dockerfile" 'ARG SOURCE_URL=unknown' "$name Dockerfile must set the global SOURCE_URL default"

  stage=$(final_stage "$dockerfile")
  printf '%s\n' "$stage" | grep -Eiq '^ARG[[:space:]]+GIT_REVISION([[:space:]]|$)' || fail "$name final runtime stage must redeclare ARG GIT_REVISION"
  printf '%s\n' "$stage" | grep -Eiq '^ARG[[:space:]]+SOURCE_URL([[:space:]]|$)' || fail "$name final runtime stage must redeclare ARG SOURCE_URL"
  printf '%s\n' "$stage" | grep -Eiq 'LABEL[[:space:]]+org\.opencontainers\.image\.revision=' || fail "$name final runtime stage must label org.opencontainers.image.revision"
  printf '%s\n' "$stage" | grep -Eiq 'LABEL[[:space:]]+org\.opencontainers\.image\.source=' || fail "$name final runtime stage must label org.opencontainers.image.source"
}

assert_nginx_conf() {
  require_line "$1" '^[[:space:]]*worker_processes[[:space:]]+[^;]+;' 'nginx.conf must configure worker_processes'
  require_line "$1" '^[[:space:]]*events[[:space:]]*\{' 'nginx.conf must define events'
  require_line "$1" '^[[:space:]]*http[[:space:]]*\{' 'nginx.conf must define http'
  require_line "$1" '^[[:space:]]*include[[:space:]]+/etc/nginx/mime\.types;' 'nginx.conf must include mime.types'
  require_line "$1" '^[[:space:]]*sendfile[[:space:]]+on;' 'nginx.conf must enable sendfile'
  require_line "$1" '^[[:space:]]*keepalive_timeout[[:space:]]+[^;]+;' 'nginx.conf must configure keepalive_timeout'
  require_line "$1" '^[[:space:]]*access_log[[:space:]]+[^;]+;' 'nginx.conf must configure access_log'
  require_line "$1" '^[[:space:]]*error_log[[:space:]]+[^;]+;' 'nginx.conf must configure error_log'
  require_line "$1" '^[[:space:]]*gzip[[:space:]]+on;' 'nginx.conf must enable gzip'
  require_line "$1" '^[[:space:]]*client_max_body_size[[:space:]]+0;' 'nginx.conf must disable the request body limit'
  require_line "$1" '^[[:space:]]*include[[:space:]]+/etc/nginx/conf\.d/\*\.conf;' 'nginx.conf must include conf.d files'
}

assert_default_conf() {
  require_exact_line "$1" '    location = /healthz {' 'default.conf must define exact /healthz location'
  require_exact_line "$1" '    location ~ ^/(api|v[1-3]|admin|open_api)(/|$) {' 'default.conf must use the suffix-boundary API location'
  require_exact_line "$1" '        try_files $uri $uri/ /index.html;' 'default.conf must use the SPA fallback'
  require_min_count "$1" '^        proxy_pass[[:space:]]+http://coze-server:8888;' 2 'both proxy locations must target coze-server:8888'
  for header in Host X-Real-IP X-Forwarded-For X-Forwarded-Proto; do
    require_min_count "$1" "proxy_set_header[[:space:]]+$header[[:space:]]+" 2 "both proxy locations must set $header"
  done
  require_min_count "$1" '^        proxy_connect_timeout[[:space:]]+60s;' 2 'both proxy locations must set proxy_connect_timeout to 60s'
  require_min_count "$1" '^        proxy_send_timeout[[:space:]]+60s;' 2 'both proxy locations must set proxy_send_timeout to 60s'
  require_min_count "$1" '^        proxy_read_timeout[[:space:]]+600s;' 2 'both proxy locations must set proxy_read_timeout to 600s'
}

assert_forbidden_config_content() {
  if grep -Eiq -- 'minio|local_storage|sub_filter|ssl_certificate|listen[[:space:]]+443|proxy_ssl' "$1" "$2"; then
    fail 'deployment nginx configs contain a forbidden storage or TLS directive'
  fi
  if grep -Eiq -- 'proxy_pass[[:space:]]+[^;]*(minio|s3|oss|object[-_]storage)|rewrite[[:space:]]+[^;]*(minio|s3|oss|object[-_]storage)' "$1" "$2"; then
    fail 'deployment nginx configs contain an object-storage proxy or rewrite marker'
  fi
}

backend_dockerfile=$CONTRACT_ROOT/backend/Dockerfile
frontend_dockerfile=$CONTRACT_ROOT/frontend/Dockerfile
nginx_conf=$CONTRACT_ROOT/deploy/dev/nginx/nginx.conf
default_conf=$CONTRACT_ROOT/deploy/dev/nginx/default.conf

[ -f "$backend_dockerfile" ] || fail 'backend Dockerfile is missing'
[ -f "$frontend_dockerfile" ] || fail 'frontend Dockerfile is missing'
[ -f "$nginx_conf" ] || fail 'deployment nginx.conf is missing'
[ -f "$default_conf" ] || fail 'deployment default.conf is missing'

for dockerfile in "$backend_dockerfile" "$frontend_dockerfile"; do
  name=$(basename "$(dirname "$dockerfile")")
  assert_dockerfile "$dockerfile" "$name"
done

backend_final_stage=$(final_stage "$backend_dockerfile")
printf '%s\n' "$backend_final_stage" | grep -Eiq '^ENV[[:space:]]+APP_REVISION=\$GIT_REVISION([[:space:]]|$)' || fail 'backend final runtime stage must set APP_REVISION from GIT_REVISION'

require_line "$frontend_dockerfile" 'COPY[[:space:]]+deploy/dev/nginx/nginx\.conf[[:space:]]+/etc/nginx/nginx\.conf' 'frontend Dockerfile must copy deployment nginx.conf'
require_line "$frontend_dockerfile" 'COPY[[:space:]]+deploy/dev/nginx/default\.conf[[:space:]]+/etc/nginx/conf\.d/default\.conf' 'frontend Dockerfile must copy deployment default.conf'
require_line "$frontend_dockerfile" '^EXPOSE[[:space:]]+80([[:space:]]|$)' 'frontend Dockerfile must expose port 80'

assert_nginx_conf "$nginx_conf"
assert_default_conf "$default_conf"
assert_forbidden_config_content "$nginx_conf" "$default_conf"

printf 'image contract: ok\n'
