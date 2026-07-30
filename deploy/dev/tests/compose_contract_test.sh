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
COMPOSE_FILE=$REPO_ROOT/deploy/dev/docker-compose.yml
ENV_FILE=$REPO_ROOT/deploy/dev/.env.example

fail() {
  printf 'compose contract failure: %s\n' "$1" >&2
  exit 1
}

require_text() {
  text=$1
  pattern=$2
  message=$3
  printf '%s\n' "$text" | grep -Eq -- "$pattern" || fail "$message"
}

require_exact_text() {
  actual=$1
  expected=$2
  message=$3
  [ "$actual" = "$expected" ] || fail "$message (got: $actual)"
}

service_block() {
  printf '%s\n' "$1" | awk -v service="$2" '
    $0 == "  " service ":" {
      in_service=1
    }
    in_service && $0 ~ /^  [^[:space:]]/ && $0 != "  " service ":" {
      exit
    }
    in_service && /^[^[:space:]]/ {
      exit
    }
    in_service {
      print
    }
  '
}

render_config() {
  config_output=$(docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config 2>&1) || {
    fail "docker compose config failed: $config_output"
  }
  printf '%s\n' "$config_output"
}

config=$(render_config)
services=$(printf '%s\n' "$config" | awk '/^services:/{in_services=1; next} in_services && /^[^[:space:]]/{exit} in_services && /^  [^[:space:]]/{sub(/^  /, ""); sub(/:$/, ""); print}')
require_exact_text "$services" $'coze-server\ncoze-web' 'services must be exactly coze-server then coze-web'
server_config=$(service_block "$config" coze-server)
web_config=$(service_block "$config" coze-web)

require_text "$server_config" 'image: registry\.example\.aliyuncs\.com/example/coze-server:dev' 'coze-server image must use the dev tag by default'
require_text "$server_config" '^    expose:$' 'coze-server must declare exposed ports'
require_text "$server_config" '^      - "8888"$' 'coze-server must expose port 8888'
if printf '%s\n' "$server_config" | grep -Eq -- '^    ports:$'; then
  fail 'coze-server must not publish a host port'
fi
require_text "$server_config" 'source: .*/deploy/dev/app\.env' 'coze-server must mount the server-local app.env'
require_text "$server_config" 'target: /app/\.env' 'coze-server must mount app.env at /app/.env'
require_text "$server_config" 'read_only: true' 'the backend app.env mount must be read-only'
require_text "$server_config" 'curl' 'backend healthcheck must use curl'
require_text "$server_config" 'http://127\.0\.0\.1:8888/healthz' 'backend healthcheck must call /healthz'
require_text "$server_config" 'restart: unless-stopped' 'coze-server must restart unless stopped'

require_text "$web_config" 'image: registry\.example\.aliyuncs\.com/example/coze-web:dev' 'coze-web image must use the dev tag by default'
require_text "$web_config" 'condition: service_healthy' 'coze-web must wait for a healthy coze-server'
require_text "$web_config" 'wget' 'web healthcheck must use wget'
require_text "$web_config" 'http://127\.0\.0\.1/healthz' 'web healthcheck must call /healthz'
require_text "$web_config" 'host_ip: 127\.0\.0\.1' 'coze-web must bind only to loopback'
require_text "$web_config" 'target: 80' 'coze-web must target container port 80'
require_text "$web_config" 'published: "8888"' 'coze-web must publish host port 8888'
require_text "$web_config" 'restart: unless-stopped' 'coze-web must restart unless stopped'
require_text "$config" 'driver: bridge' 'the deployment network must be a bridge network'
if printf '%s\n' "$config" | grep -Eq -- 'internal: true'; then
  fail 'the deployment network must allow backend outbound connectivity'
fi
if printf '%s\n' "$config" | grep -Eiq -- 'mysql|redis|elasticsearch|minio|milvus|etcd|nsq'; then
  fail 'compose config contains a forbidden infrastructure service or image reference'
fi

override_config=$(SERVER_IMAGE_TAG=canary WEB_IMAGE_TAG=canary render_config)
require_text "$(service_block "$override_config" coze-server)" 'image: .*/coze-server:canary' 'SERVER_IMAGE_TAG must override the backend tag'
require_text "$(service_block "$override_config" coze-web)" 'image: .*/coze-web:canary' 'WEB_IMAGE_TAG must override the web tag'

printf 'compose contract: ok\n'
