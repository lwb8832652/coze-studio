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

final_stage() {
  awk '
    /^FROM[[:space:]]/ { last=NR }
    { lines[NR]=$0 }
    END {
      for (i=last; i<=NR; i++) print lines[i]
    }
  ' "$1"
}

location_block() {
  file=$1
  marker=$2
  awk -v marker="$marker" '
    index($0, marker) {
      in_block=1
    }
    in_block {
      print
    }
    in_block && /^[[:space:]]*}[[:space:]]*$/ {
      exit
    }
  ' "$file"
}

require_block_line() {
  block=$1
  pattern=$2
  message=$3
  if ! printf '%s\n' "$block" | grep -Eiq -- "$pattern"; then
    fail "$message"
  fi
}

assert_proxy_block() {
  block=$1
  name=$2
  [ -n "$block" ] || fail "$name proxy location is missing"
  require_block_line "$block" 'proxy_pass[[:space:]]+http://coze-server:8888;' "$name must target coze-server:8888"
  for header in Host X-Real-IP X-Forwarded-For X-Forwarded-Proto; do
    require_block_line "$block" "proxy_set_header[[:space:]]+$header[[:space:]]+" "$name must set $header"
  done
  require_block_line "$block" 'proxy_connect_timeout[[:space:]]+60s;' "$name must set proxy_connect_timeout to 60s"
  require_block_line "$block" 'proxy_send_timeout[[:space:]]+60s;' "$name must set proxy_send_timeout to 60s"
  require_block_line "$block" 'proxy_read_timeout[[:space:]]+600s;' "$name must set proxy_read_timeout to 600s"
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

assert_sqlite_musl_compatibility() {
  go_mod=$1
  sqlite_version=$(awk '$1 == "github.com/mattn/go-sqlite3" { print $2; exit }' "$go_mod")
  [ -n "$sqlite_version" ] || fail 'backend go.mod must declare github.com/mattn/go-sqlite3 directly'

  version=${sqlite_version#v}
  major=${version%%.*}
  remainder=${version#*.}
  minor=${remainder%%.*}
  patch=${remainder#*.}
  case "$major.$minor.$patch" in
    *[!0-9.]*|.*|*..*|*.) fail "backend go.mod has an unsupported go-sqlite3 version: $sqlite_version" ;;
  esac

  if [ "$major" -lt 1 ] || \
     { [ "$major" -eq 1 ] && [ "$minor" -lt 14 ]; } || \
     { [ "$major" -eq 1 ] && [ "$minor" -eq 14 ] && [ "$patch" -lt 19 ]; }; then
    fail "backend go-sqlite3 $sqlite_version is incompatible with Alpine musl; require v1.14.19 or newer"
  fi
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
  health_block=$(location_block "$1" 'location = /healthz {')
  api_block=$(location_block "$1" 'location ~ ^/(api|v[1-3]|admin|open_api)(/|$) {')
  assert_proxy_block "$health_block" '/healthz proxy location'
  assert_proxy_block "$api_block" 'API proxy location'
  require_block_line "$api_block" 'proxy_http_version[[:space:]]+1\.1;' 'API proxy location must use HTTP/1.1 for streaming'
  require_block_line "$api_block" 'proxy_set_header[[:space:]]+Connection[[:space:]]+"";' 'API proxy location must clear the Connection header'
  require_block_line "$api_block" 'proxy_buffering[[:space:]]+off;' 'API proxy location must disable buffering for SSE'
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
backend_go_mod=$CONTRACT_ROOT/backend/go.mod
frontend_dockerfile=$CONTRACT_ROOT/frontend/Dockerfile
sandbox_runner_dockerfile=$CONTRACT_ROOT/backend/Dockerfile.sandbox-runner
nginx_conf=$CONTRACT_ROOT/deploy/dev/nginx/nginx.conf
default_conf=$CONTRACT_ROOT/deploy/dev/nginx/default.conf
compose_file=$CONTRACT_ROOT/deploy/dev/docker-compose.runner-2c4g.yml
deploy_script=$CONTRACT_ROOT/deploy/dev/deploy.sh
workflow_file=$CONTRACT_ROOT/.github/workflows/deploy-dev.yml
aio_dockerfile=$CONTRACT_ROOT/deploy/sandbox-runner/Dockerfile.aio
operations_runbook=$CONTRACT_ROOT/docs/superpowers/runbooks/sandbox-control-plane-operations.md
local_runbook=$CONTRACT_ROOT/docs/superpowers/runbooks/local-debug-and-test.md

[ -f "$backend_dockerfile" ] || fail 'backend Dockerfile is missing'
[ -f "$backend_go_mod" ] || fail 'backend go.mod is missing'
[ -f "$frontend_dockerfile" ] || fail 'frontend Dockerfile is missing'
[ -f "$sandbox_runner_dockerfile" ] || fail 'sandbox runner Dockerfile is missing'
[ -f "$nginx_conf" ] || fail 'deployment nginx.conf is missing'
[ -f "$default_conf" ] || fail 'deployment default.conf is missing'
[ -f "$compose_file" ] || fail 'runner compose file is missing'
[ -f "$deploy_script" ] || fail 'dev deploy script is missing'
[ -f "$workflow_file" ] || fail 'dev workflow is missing'
[ -f "$operations_runbook" ] || fail 'sandbox operations runbook is missing'
[ -f "$local_runbook" ] || fail 'local debug runbook is missing'
[ ! -e "$aio_dockerfile" ] || fail 'official AIO must not have a derived Dockerfile'

for dockerfile in "$backend_dockerfile" "$frontend_dockerfile" "$sandbox_runner_dockerfile"; do
  name=$(basename "$(dirname "$dockerfile")")
  assert_dockerfile "$dockerfile" "$name"
done

sandbox_runner_final_stage=$(final_stage "$sandbox_runner_dockerfile")
printf '%s\n' "$sandbox_runner_final_stage" | grep -Eiq '^USER[[:space:]]+10001:10001([[:space:]]|$)' || fail 'sandbox runner must run as the unprivileged sandbox user'
printf '%s\n' "$sandbox_runner_final_stage" | grep -Eiq '^EXPOSE[[:space:]]+9443([[:space:]]|$)' || fail 'sandbox runner must expose only its private API port'
if printf '%s\n' "$sandbox_runner_final_stage" | grep -Eiq 'opencoze|python3|nodejs|deno'; then
  fail 'sandbox runner final image must not include the application server or execution toolchains'
fi

backend_final_stage=$(final_stage "$backend_dockerfile")
printf '%s\n' "$backend_final_stage" | grep -Eiq '^ENV[[:space:]]+APP_REVISION=\$GIT_REVISION([[:space:]]|$)' || fail 'backend final runtime stage must set APP_REVISION from GIT_REVISION'
printf '%s\n' "$backend_final_stage" | grep -Eq '^COPY[[:space:]]+docker/volumes/minio/default_icon[[:space:]]+/app/resources/storage/default_icon/?$' || fail 'backend image must include bundled default icons'
printf '%s\n' "$backend_final_stage" | grep -Eq '^COPY[[:space:]]+docker/volumes/minio/official_plugin_icon[[:space:]]+/app/resources/storage/official_plugin_icon/?$' || fail 'backend image must include bundled official plugin icons'
printf '%s\n' "$backend_final_stage" | grep -Eq '^ENV[[:space:]]+COZE_BUNDLED_STORAGE_ASSET_DIR=/app/resources/storage([[:space:]]|$)' || fail 'backend image must enable bundled storage asset synchronization'
assert_sqlite_musl_compatibility "$backend_go_mod"

require_line "$frontend_dockerfile" 'COPY[[:space:]]+deploy/dev/nginx/nginx\.conf[[:space:]]+/etc/nginx/nginx\.conf' 'frontend Dockerfile must copy deployment nginx.conf'
require_line "$frontend_dockerfile" 'COPY[[:space:]]+deploy/dev/nginx/default\.conf[[:space:]]+/etc/nginx/conf\.d/default\.conf' 'frontend Dockerfile must copy deployment default.conf'
require_line "$frontend_dockerfile" '^EXPOSE[[:space:]]+80([[:space:]]|$)' 'frontend Dockerfile must expose port 80'

assert_nginx_conf "$nginx_conf"
assert_default_conf "$default_conf"
assert_forbidden_config_content "$nginx_conf" "$default_conf"

aio_image_count=$(grep -Eic -- '^[[:space:]]*image:[[:space:]]+ghcr\.io/agent-infra/sandbox' "$compose_file")
[ "$aio_image_count" -eq 1 ] || fail 'Compose must declare exactly one official AIO image'
require_line "$compose_file" '^[[:space:]]*image:[[:space:]]+ghcr\.io/agent-infra/sandbox:latest[[:space:]]*$' 'Compose must run the official AIO latest image directly'
require_line "$deploy_script" 'ghcr\.io/agent-infra/sandbox:latest' 'deploy must pull the official AIO latest image'
if grep -Eiq -- 'build-sandbox-aio|Dockerfile\.aio|coze-sandbox-aio:[[:space:]]*dev-|AIO_(IMAGE_)?(REVISION|DIGEST)|aio[_-]candidate|promot(e|ion)[^[:space:]]*aio' "$workflow_file" "$deploy_script"; then
  fail 'official AIO must not be built, revision-pinned, promoted, or configured from a candidate image'
fi
if grep -Eiq -- 'ghcr\.io/agent-infra/sandbox@sha256:' "$compose_file" "$deploy_script"; then
  fail 'official AIO image must remain unpinned'
fi
require_line "$operations_runbook" 'ghcr\.io/agent-infra/sandbox:latest' 'operations runbook must document the official latest AIO image'
require_line "$operations_runbook" 'image ID.*(证据|evidence)' 'operations runbook must treat the resolved AIO image ID as evidence only'
require_line "$operations_runbook" '逻辑路由.*不是.*(隔离|chroot)' 'operations runbook must state the logical workspace isolation boundary'
require_line "$local_runbook" 'SANDBOX_HOST_SHELL_SESSION_ENABLED=true' 'local runbook must document the independent Host Shell gate'
require_line "$local_runbook" '不启动本地 MySQL 容器' 'local runbook must forbid a local MySQL container for the runner profile'

printf 'image contract: ok\n'
