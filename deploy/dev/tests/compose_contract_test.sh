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
RUNNER_COMPOSE_FILE=$REPO_ROOT/deploy/dev/docker-compose.runner-2c4g.yml
ENV_FILE=$REPO_ROOT/deploy/dev/.env.example
SOAK_TEST=$REPO_ROOT/deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh
CORE_E2E_TEST=$REPO_ROOT/deploy/sandbox-runner/tests/aio_core_e2e_test.sh

fail() {
  printf 'compose contract failure: %s\n' "$1" >&2
  exit 1
}

require_text() {
  text=$1
  pattern=$2
  message=$3
  grep -Eq -- "$pattern" <<<"$text" || fail "$message"
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

dependency_block() {
  printf '%s\n' "$1" | awk -v dependency="$2" '
    $0 == "    depends_on:" {
      in_depends_on=1
      next
    }
    in_depends_on && $0 ~ /^    [^[:space:]]/ {
      exit
    }
    in_depends_on && $0 == "      " dependency ":" {
      in_dependency=1
    }
    in_dependency && $0 ~ /^      [^[:space:]]/ && $0 != "      " dependency ":" {
      exit
    }
    in_dependency {
      print
    }
  '
}

render_config() {
  server_tag=$1
  web_tag=$2
  bind_ip=${3:-0.0.0.0}
  web_port=${4:-8888}
  config_output=$(ACR_REGISTRY=registry.example.aliyuncs.com \
    ACR_NAMESPACE=example \
    SERVER_IMAGE_TAG="$server_tag" \
    WEB_IMAGE_TAG="$web_tag" \
    WEB_BIND_IP="$bind_ip" \
    WEB_PORT="$web_port" \
    OCEANBASE_PASSWORD=sentinel-oceanbase-password \
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config 2>&1) || {
    fail "docker compose config failed: $config_output"
  }
  printf '%s\n' "$config_output"
}

render_runner_config() {
  runner_env=$(mktemp "${TMPDIR:-/tmp}/coze-runner-compose-env.XXXXXX")
  runner_cert=$(mktemp "${TMPDIR:-/tmp}/coze-runner-compose-cert.XXXXXX")
  runner_key=$(mktemp "${TMPDIR:-/tmp}/coze-runner-compose-key.XXXXXX")
  trap 'rm -f -- "$runner_env" "$runner_cert" "$runner_key"' RETURN
  printf 'SANDBOX_RUNNER_AUTH_TOKEN=runner-auth-token-0123456789\n' > "$runner_env"
  printf 'placeholder\n' > "$runner_cert"
  printf 'placeholder\n' > "$runner_key"
  config_output=$(ACR_REGISTRY=registry.example.aliyuncs.com \
    ACR_NAMESPACE=example \
    SERVER_IMAGE_TAG=dev \
    WEB_IMAGE_TAG=dev \
    SANDBOX_RUNNER_IMAGE_TAG=dev \
    SANDBOX_RUNNER_EXECUTION_IMAGE=registry.example.aliyuncs.com/example/coze-sandbox-runtime@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef \
    WEB_BIND_IP=0.0.0.0 \
    WEB_PORT=8888 \
    SANDBOX_RUNNER_ROOTLESS_SOCKET="$runner_env" \
    SANDBOX_RUNNER_ROOTLESS_SOCKET_GID=10001 \
    SANDBOX_RUNNER_ENV_FILE="$runner_env" \
    SANDBOX_RUNNER_TLS_CERT_FILE="$runner_cert" \
    SANDBOX_RUNNER_TLS_KEY_FILE="$runner_key" \
    docker compose --env-file "$ENV_FILE" \
      -f "$RUNNER_COMPOSE_FILE" config 2>&1) || {
    fail "runner compose config failed: $config_output"
  }
  printf '%s\n' "$config_output"
}

config=$(render_config dev dev)
compose_source=$(<"$COMPOSE_FILE")
env_source=$(<"$ENV_FILE")
require_text "$env_source" '^ACR_REGISTRY=registry\.example\.aliyuncs\.com$' '.env.example must define the registry placeholder'
require_text "$env_source" '^ACR_NAMESPACE=example$' '.env.example must define the namespace placeholder'
require_text "$env_source" '^SERVER_IMAGE_TAG=dev$' '.env.example must default the server tag to dev'
require_text "$env_source" '^WEB_IMAGE_TAG=dev$' '.env.example must default the web tag to dev'
require_text "$env_source" '^WEB_BIND_IP=0\.0\.0\.0$' '.env.example must default the web bind IP to all interfaces'
require_text "$env_source" '^WEB_PORT=8888$' '.env.example must default the web port to 8888'
require_text "$env_source" '^OCEANBASE_PASSWORD=$' '.env.example must leave the OceanBase password blank'
require_text "$env_source" '^OCEANBASE_DATAFILE_SIZE=1G$' '.env.example must define the OceanBase datafile size'
require_text "$env_source" '^OCEANBASE_MEM_LIMIT=2g$' '.env.example must define the OceanBase memory limit'
require_text "$env_source" '^OCEANBASE_CPUS=1\.00$' '.env.example must define the OceanBase CPU limit'
require_text "$env_source" '^DEPLOY_HEALTH_TIMEOUT_SECONDS=120$' '.env.example must define the deployment health timeout'
env_web_settings_order=$(printf '%s\n' "$env_source" | awk '
  /^WEB_BIND_IP=/ {print "WEB_BIND_IP"}
  /^WEB_PORT=/ {print "WEB_PORT"}
  /^DEPLOY_HEALTH_TIMEOUT_SECONDS=/ {print "DEPLOY_HEALTH_TIMEOUT_SECONDS"}
')
require_exact_text "$env_web_settings_order" $'WEB_BIND_IP\nWEB_PORT\nDEPLOY_HEALTH_TIMEOUT_SECONDS' 'web bind defaults must precede the deployment health timeout'
services=$(printf '%s\n' "$compose_source" | awk '/^services:/{in_services=1; next} in_services && /^[^[:space:]]/{exit} in_services && /^  [^[:space:]]/{sub(/^  /, ""); sub(/:$/, ""); print}')
require_exact_text "$services" $'oceanbase\nnsqd\ncoze-server\ncoze-web' 'services must be exactly oceanbase, nsqd, coze-server, then coze-web'
oceanbase_config=$(service_block "$config" oceanbase)
nsqd_config=$(service_block "$config" nsqd)
server_config=$(service_block "$config" coze-server)
web_config=$(service_block "$config" coze-web)
oceanbase_source=$(service_block "$compose_source" oceanbase)
nsqd_source=$(service_block "$compose_source" nsqd)
web_source=$(service_block "$compose_source" coze-web)

require_text "$oceanbase_config" '^    image: oceanbase/oceanbase-ce:latest$' 'OceanBase image must use the CE image'
require_text "$oceanbase_config" '^      MODE: SLIM$' 'OceanBase must use slim mode'
require_text "$oceanbase_config" '^      OB_DATAFILE_SIZE: 1G$' 'OceanBase datafile size must default to 1G'
require_text "$oceanbase_config" '^      OB_SYS_PASSWORD: sentinel-oceanbase-password$' 'OceanBase system password must come from deploy.env'
require_text "$oceanbase_config" '^      OB_TENANT_PASSWORD: sentinel-oceanbase-password$' 'OceanBase tenant password must come from deploy.env'
require_text "$oceanbase_config" '^    mem_limit: "2147483648"$' 'OceanBase memory must default to 2g'
require_text "$oceanbase_config" '^    cpus: 1$' 'OceanBase CPU must default to 1 core'
require_text "$oceanbase_config" '^        source: oceanbase-ob$' 'OceanBase data must use a named volume'
require_text "$oceanbase_config" '^        source: oceanbase-cluster$' 'OceanBase cluster metadata must use a named volume'
require_text "$oceanbase_config" '^        - CMD-SHELL$' 'OceanBase healthcheck must execute through a shell'
require_text "$oceanbase_source" 'obclient -h127\.0\.0\.1 -P2881 -uroot@test -p\$\$\{OB_TENANT_PASSWORD\}' 'OceanBase healthcheck must verify tenant connectivity'
require_text "$oceanbase_config" '^    restart: unless-stopped$' 'OceanBase must restart unless stopped'
require_text "$oceanbase_config" '^    pull_policy: missing$' 'OceanBase must pull only when its image is missing'

require_text "$nsqd_config" '^    image: nsqio/nsq:v1\.3\.0$' 'nsqd image must be pinned to v1.3.0'
nsqd_command=$(printf '%s\n' "$nsqd_config" | awk '
  $0 == "    command:" {in_command=1; next}
  in_command && /^    [^[:space:]]/ {exit}
  in_command && /^      - / {sub(/^      - /, ""); gsub(/^"|"$/, ""); print}
')
require_exact_text "$nsqd_command" $'/nsqd\n--data-path=/data\n--mem-queue-size=0' 'nsqd command must enable persistent disk-backed queues'
nsqd_expose=$(printf '%s\n' "$nsqd_config" | awk '
  $0 == "    expose:" {in_expose=1; next}
  in_expose && /^    [^[:space:]]/ {exit}
  in_expose && /^      - / {sub(/^      - /, ""); gsub(/^"|"$/, ""); print}
')
require_exact_text "$nsqd_expose" $'4150\n4151' 'nsqd must expose only its TCP and HTTP ports internally'
if grep -Eq -- '^    ports:$' <<<"$nsqd_config"; then
  fail 'nsqd must not publish a host port'
fi
require_text "$nsqd_config" '^      - type: volume$' 'nsqd data must use a named volume'
require_text "$nsqd_config" '^        source: nsq-data$' 'nsqd data volume must use nsq-data'
require_text "$nsqd_config" '^        target: /data$' 'nsqd data volume must mount at /data'
require_text "$nsqd_config" '^        - CMD-SHELL$' 'nsqd healthcheck must execute through a shell'
require_text "$nsqd_source" '^        - wget -q -O - http://127\.0\.0\.1:4151/ping \| grep -qx OK$' 'nsqd healthcheck command must require an exact OK response'
require_text "$nsqd_config" '^      interval: 10s$' 'nsqd healthcheck interval must be 10 seconds'
require_text "$nsqd_config" '^      timeout: 5s$' 'nsqd healthcheck timeout must be 5 seconds'
require_text "$nsqd_config" '^      retries: 12$' 'nsqd healthcheck must retry 12 times'
require_text "$nsqd_config" '^      start_period: 10s$' 'nsqd healthcheck start period must be 10 seconds'
require_text "$nsqd_config" '^    restart: unless-stopped$' 'nsqd must restart unless stopped'
require_text "$nsqd_config" '^    pull_policy: missing$' 'nsqd must pull only when its image is missing'
require_text "$nsqd_config" '^    stop_grace_period: 30s$' 'nsqd must allow 30 seconds for a graceful stop'
require_text "$nsqd_source" '^    mem_limit: 384m$' 'nsqd memory must be limited to 384m'
require_text "$nsqd_source" '^    cpus: 0\.50$' 'nsqd CPU must be limited to 0.50'
require_text "$nsqd_config" '^    pids_limit: 128$' 'nsqd process count must be limited to 128'
require_text "$compose_source" '^x-logging: &default-logging$' 'compose must define the shared logging anchor'
logging_alias_count=$(printf '%s\n' "$compose_source" | awk '$0 == "    logging: *default-logging" {count++} END {print count + 0}')
require_exact_text "$logging_alias_count" '4' 'all four services must use the shared logging anchor'

require_text "$server_config" 'image: registry\.example\.aliyuncs\.com/example/coze-server:dev' 'coze-server image must use the dev tag by default'
require_text "$server_config" '^      COZE_MQ_TYPE: nsq$' 'coze-server must use the NSQ message queue'
require_text "$server_config" '^      MQ_NAME_SERVER: nsqd:4150$' 'coze-server must connect to the local nsqd service'
require_text "$server_config" '^      VECTOR_STORE_TYPE: oceanbase$' 'coze-server must use OceanBase for vector storage'
require_text "$server_config" '^      OCEANBASE_HOST: oceanbase$' 'coze-server must connect to the local OceanBase service'
require_text "$server_config" '^      OCEANBASE_PORT: "2881"$' 'coze-server must use the OceanBase SQL port'
require_text "$server_config" '^      OCEANBASE_USER: root@test$' 'coze-server must use the OceanBase tenant user by default'
require_text "$server_config" '^      OCEANBASE_DATABASE: test$' 'coze-server must use the OceanBase test database by default'
require_text "$server_config" '^      OCEANBASE_PASSWORD: sentinel-oceanbase-password$' 'coze-server must receive the OceanBase password from deploy.env'
require_text "$server_config" '^      AGENT_THREAD_RUNTIME_DEFAULT: eino_adk$' 'coze-server must use the Eino ADK runtime'
require_text "$server_config" '^      AGENT_THREAD_EINO_ADK_ENABLED: "true"$' 'coze-server must enable the Eino ADK runtime'
require_text "$server_config" '^      AGENT_THREAD_WORKER_ENABLED: "true"$' 'coze-server must enable the run worker'
require_text "$server_config" '^      AGENT_THREAD_RESUME_WORKER_ENABLED: "true"$' 'coze-server must enable the resume worker'
require_text "$server_config" '^      AGENT_THREAD_LEASE_RECOVERY_WORKER_ENABLED: "true"$' 'coze-server must enable lease recovery'
require_text "$server_config" '^      oceanbase:$' 'coze-server must depend on OceanBase'
require_text "$server_config" '^      nsqd:$' 'coze-server must depend on nsqd'
require_text "$server_config" '^        condition: service_healthy$' 'coze-server must wait for healthy nsqd'
require_text "$server_config" '^        restart: true$' 'coze-server must restart when nsqd is explicitly restarted'
require_text "$server_config" '^    expose:$' 'coze-server must declare exposed ports'
require_text "$server_config" '^      - "8888"$' 'coze-server must expose port 8888'
if grep -Eq -- '^    ports:$' <<<"$server_config"; then
  fail 'coze-server must not publish a host port'
fi
require_text "$server_config" 'source: .*/deploy/dev/app\.env' 'coze-server must mount the server-local app.env'
require_text "$server_config" 'target: /app/\.env' 'coze-server must mount app.env at /app/.env'
require_text "$server_config" 'read_only: true' 'the backend app.env mount must be read-only'
require_text "$compose_source" 'create_host_path:[[:space:]]+false' 'a missing app.env must fail closed instead of creating a directory'
require_text "$server_config" 'curl' 'backend healthcheck must use curl'
require_text "$server_config" 'http://127\.0\.0\.1:8888/healthz' 'backend healthcheck must call /healthz'
require_text "$server_config" 'restart: unless-stopped' 'coze-server must restart unless stopped'

require_text "$web_config" 'image: registry\.example\.aliyuncs\.com/example/coze-web:dev' 'coze-web image must use the dev tag by default'
require_text "$web_config" 'condition: service_healthy' 'coze-web must wait for a healthy coze-server'
web_server_dependency=$(dependency_block "$web_config" coze-server)
require_text "$web_server_dependency" '^      coze-server:$' 'coze-web must depend specifically on coze-server'
require_text "$web_server_dependency" '^        condition: service_healthy$' 'coze-web must wait for a healthy coze-server dependency'
require_text "$web_server_dependency" '^        restart: true$' 'coze-web must restart when coze-server is explicitly restarted'
require_text "$web_config" 'wget' 'web healthcheck must use wget'
require_text "$web_config" 'http://127\.0\.0\.1/healthz' 'web healthcheck must call /healthz'
require_text "$web_source" '^      - "\$\{WEB_BIND_IP:-0\.0\.0\.0\}:\$\{WEB_PORT:-8888\}:80"$' 'coze-web must retain the defaulted bind and port expression'
require_text "$web_config" 'host_ip: 0\.0\.0\.0' 'coze-web must bind to all interfaces by default'
require_text "$web_config" 'target: 80' 'coze-web must target container port 80'
require_text "$web_config" 'published: "8888"' 'coze-web must publish host port 8888'
require_text "$web_config" 'restart: unless-stopped' 'coze-web must restart unless stopped'

for service_config in "$oceanbase_config" "$nsqd_config" "$server_config" "$web_config"; do
  require_text "$service_config" '^    logging:$' 'every service must configure logging'
  require_text "$service_config" '^      driver: json-file$' 'every service must use the json-file log driver'
  require_text "$service_config" '^        max-file: "3"$' 'every service must retain three log files'
  require_text "$service_config" '^        max-size: 10m$' 'every service log file must be limited to 10m'
  require_text "$service_config" '^      coze-dev: null$' 'every service must attach to the coze-dev network'
done

networks=$(printf '%s\n' "$config" | awk '
  /^networks:$/ {in_networks=1; next}
  in_networks && /^[^[:space:]]/ {exit}
  in_networks && /^  [^[:space:]]/ {sub(/^  /, ""); sub(/:$/, ""); print}
')
require_exact_text "$networks" 'coze-dev' 'coze-dev must be the only top-level network'
require_text "$config" 'driver: bridge' 'the deployment network must be a bridge network'
if grep -Eq -- 'internal: true' <<<"$config"; then
  fail 'the deployment network must allow backend outbound connectivity'
fi
volumes=$(printf '%s\n' "$config" | awk '
  /^volumes:$/ {in_volumes=1; next}
  in_volumes && /^[^[:space:]]/ {exit}
  in_volumes && /^  [^[:space:]]/ {sub(/^  /, ""); sub(/:$/, ""); print}
')
require_exact_text "$volumes" $'nsq-data\noceanbase-cluster\noceanbase-ob' 'OceanBase and nsq-data must be the only top-level volumes'

override_config=$(render_config canary canary)
require_text "$(service_block "$override_config" coze-server)" 'image: .*/coze-server:canary' 'SERVER_IMAGE_TAG must override the backend tag'
require_text "$(service_block "$override_config" coze-web)" 'image: .*/coze-web:canary' 'WEB_IMAGE_TAG must override the web tag'

custom_bind_config=$(render_config dev dev 127.0.0.1 18888)
custom_web_config=$(service_block "$custom_bind_config" coze-web)
require_text "$custom_web_config" 'host_ip: 127\.0\.0\.1' 'WEB_BIND_IP must override the web bind address'
require_text "$custom_web_config" 'published: "18888"' 'WEB_PORT must override the published web port'

[ -f "$RUNNER_COMPOSE_FILE" ] || fail 'runner-2c4g compose file is missing'
runner_source=$(<"$RUNNER_COMPOSE_FILE")
runner_config=$(render_runner_config)
runner_services=$(printf '%s\n' "$runner_source" | awk '/^services:/{in_services=1; next} in_services && /^[^[:space:]]/{exit} in_services && /^  [^[:space:]]/{sub(/^  /, ""); sub(/:$/, ""); print}')
require_exact_text "$runner_services" $'nsqd\ncoze-server\ncoze-sandbox-aio\ncoze-sandbox-runner\ncoze-web' 'runner-2c4g must run exactly one official AIO alongside nsqd, server, sandbox runner, and web'
if grep -Eiq '^  (oceanbase|mysql|redis|elasticsearch|minio|object-storage):' <<<"$runner_source"; then
  fail 'runner-2c4g must not start local data service containers'
fi
runner_aio_source=$(service_block "$runner_source" coze-sandbox-aio)
runner_aio_config=$(service_block "$runner_config" coze-sandbox-aio)
runner_config_block=$(service_block "$runner_config" coze-sandbox-runner)

require_text "$runner_aio_config" '^    image: ghcr\.io/agent-infra/sandbox:latest$' 'AIO must run the official latest image directly'
require_text "$runner_aio_config" '^      OPENSSL_armcap: "0"$' 'AIO must retain the verified ARM64 OpenSSL compatibility setting'
require_text "$runner_aio_config" '^      WORKSPACE: /mnt/user-data$' 'AIO must use the persistent user-data root as its workspace'
runner_aio_expose=$(printf '%s\n' "$runner_aio_config" | awk '
  $0 == "    expose:" {in_expose=1; next}
  in_expose && /^    [^[:space:]]/ {exit}
  in_expose && /^      - / {sub(/^      - /, ""); gsub(/^"|"$/, ""); print}
')
require_exact_text "$runner_aio_expose" '8080' 'AIO must expose only raw port 8080 inside the Compose network'
if grep -Eq -- '^    ports:$' <<<"$runner_aio_config"; then
  fail 'AIO must not publish a host port'
fi
if grep -Eq -- '^    (command|entrypoint|privileged|network_mode|mem_limit|cpus|pids_limit):' <<<"$runner_aio_source"; then
  fail 'AIO must use the official default startup without host networking, privilege, or resource overrides'
fi
if grep -Eiq -- '(JWT|DISABLE_|docker\.sock)' <<<"$runner_aio_source"; then
  fail 'AIO must not require JWT, DISABLE flags, or a Docker socket'
fi
if grep -Eq -- '8090' <<<"$runner_source"; then
  fail 'runner-2c4g must not retain the retired AIO port 8090'
fi
require_text "$runner_aio_config" '^      - seccomp=unconfined$' 'AIO must retain the verified seccomp compatibility setting'
require_text "$runner_aio_config" '^      - type: volume$' 'AIO persistent paths must use named volumes'
require_text "$runner_aio_config" '^        source: sandbox-aio-user-data$' 'AIO user data must use the sandbox-aio-user-data volume'
require_text "$runner_aio_config" '^        target: /mnt/user-data$' 'AIO user data must mount at /mnt/user-data'
require_text "$runner_aio_config" '^        source: sandbox-aio-skills$' 'AIO skills must use the sandbox-aio-skills volume'
require_text "$runner_aio_config" '^        target: /mnt/skills$' 'AIO skills must mount at /mnt/skills'
skills_mount=$(printf '%s\n' "$runner_aio_config" | awk '
  $0 == "        source: sandbox-aio-skills" {in_skills=1}
  in_skills {print}
  in_skills && $0 == "        read_only: true" {exit}
')
require_text "$skills_mount" '^        source: sandbox-aio-skills$' 'AIO skills mount must be present'
require_text "$skills_mount" '^        target: /mnt/skills$' 'AIO skills volume must target /mnt/skills'
require_text "$skills_mount" '^        read_only: true$' 'AIO skills mount must be read-only'

require_text "$runner_config_block" 'image: registry\.example\.aliyuncs\.com/example/coze-sandbox-runner:dev' 'runner service must use the sandbox runner image'
require_text "$runner_config_block" '^    mem_limit: "201326592"$' 'runner memory must be limited to 192 MiB'
require_text "$runner_config_block" '^    cpus: 0\.2$' 'runner CPU must be limited to 0.20'
require_text "$runner_config_block" '^    pids_limit: 64$' 'runner process count must be limited to 64'
require_text "$runner_config_block" 'no-new-privileges:true' 'runner must enable no-new-privileges'
require_text "$runner_config_block" 'read_only: true' 'runner root filesystem must be read-only'
require_text "$runner_config_block" 'target: /app/runtime/docker\.sock' 'runner must mount only its dedicated rootless socket path'
if grep -Fq '/var/run/docker.sock' <<<"$runner_source"; then
  fail 'runner-2c4g must never mount the host Docker socket'
fi
require_text "$runner_config_block" 'SANDBOX_RUNNER_ROOTLESS_ENDPOINT: unix:///app/runtime/docker\.sock' 'runner must address its dedicated rootless socket'
require_text "$runner_config_block" 'SANDBOX_RUNNER_EXECUTION_IMAGE: registry\.example\.aliyuncs\.com/example/coze-sandbox-runtime@sha256:' 'runner must receive an immutable execution runtime image digest'
require_text "$runner_source" '^      SANDBOX_RUNNER_SESSION_ENABLED: "\$\{SANDBOX_RUNNER_SESSION_ENABLED:-false\}"$' 'runner Session backend must default closed while allowing deploy to opt in explicitly'
require_text "$runner_config_block" '^      SANDBOX_RUNNER_SESSION_ENABLED: "false"$' 'runner Core sessions must default to disabled'
require_text "$runner_config_block" '^      SANDBOX_RUNNER_AIO_UPSTREAM_URL: http://coze-sandbox-aio:8080$' 'runner must supervise AIO through its private service URL'
if grep -Eq -- 'SANDBOX_HOST_SHELL_SESSION_ENABLED:[[:space:]]*"?true"?' <<<"$runner_source"; then
  fail 'remote deployment must not enable debug Host Shell sessions'
fi
require_text "$runner_config_block" 'https://127\.0\.0\.1:9443/v1/health' 'runner healthcheck must verify the private TLS endpoint'

runner_volumes=$(printf '%s\n' "$runner_config" | awk '
  /^volumes:$/ {in_volumes=1; next}
  in_volumes && /^[^[:space:]]/ {exit}
  in_volumes && /^  [^[:space:]]/ {sub(/^  /, ""); sub(/:$/, ""); print}
')
require_exact_text "$runner_volumes" $'nsq-data\nsandbox-aio-skills\nsandbox-aio-user-data' 'runner-2c4g must persist only NSQ, AIO skills, and AIO user data as named volumes'

[ -f "$CORE_E2E_TEST" ] || fail 'AIO Core E2E harness is missing'
[ -x "$CORE_E2E_TEST" ] || fail 'AIO Core E2E harness must be executable'
bash -n "$CORE_E2E_TEST" || fail 'AIO Core E2E harness must have valid Bash syntax'
core_e2e_source=$(<"$CORE_E2E_TEST")
require_text "$core_e2e_source" '--self-test' 'AIO Core E2E harness must provide a dependency-free self-test'
require_text "$core_e2e_source" 'SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT.*ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC' 'AIO Core E2E harness must require the exact isolated no-live-traffic declaration'
require_text "$core_e2e_source" 'SANDBOX_AIO_CORE_E2E_CODE_SHA' 'AIO Core E2E harness must bind the configured code revision'
require_text "$core_e2e_source" 'rev-parse HEAD' 'AIO Core E2E harness must compare its code revision with HEAD'
require_text "$core_e2e_source" '--porcelain=v1 --untracked-files=all' 'AIO Core E2E harness must reject tracked and untracked worktree changes'
require_text "$core_e2e_source" 'org.opencontainers.image.revision' 'AIO Core E2E harness must bind the running Runner OCI revision'
require_text "$core_e2e_source" 'RUNNER_IMAGE_ID=' 'AIO Core E2E evidence must record the full Runner image ID'
require_text "$core_e2e_source" 'RUNNER_IMAGE_REVISION=' 'AIO Core E2E evidence must record the Runner OCI revision'
require_text "$core_e2e_source" 'COMPOSE_PROJECT_NAME.*SANDBOX_RUNNER_DEPLOYMENT_ID' 'AIO Core E2E harness must bind its Compose project to the isolated deployment'
require_text "$core_e2e_source" 'SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP.*EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES' 'AIO Core E2E harness must require exact prefix-only cleanup'
require_text "$core_e2e_source" 'SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES.*EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE' 'AIO Core E2E harness must require exact isolated dev fixture commit and cleanup'
require_text "$core_e2e_source" 'SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY.*ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE' 'AIO Core E2E harness must restrict database cleanup to rollback-only fixture handling'
require_text "$core_e2e_source" 'duplicate key' 'AIO Core E2E controlled environment parsing must reject duplicate keys'
require_text "$core_e2e_source" 'GIT_\*' 'AIO Core E2E controlled environment parsing must reject Git redirection variables'
require_text "$core_e2e_source" 'GOWORK=off' 'AIO Core E2E fixed Go test must ignore workspace redirection'
for control_verb in stop-aio start-aio recreate-aio-preserve-volumes restart-runner; do
  require_text "$core_e2e_source" "$control_verb" "AIO Core E2E harness must retain fixed helper verb: $control_verb"
done
"$CORE_E2E_TEST" --self-test >/dev/null || fail 'AIO Core E2E harness self-test failed'
if grep -Eiq -- '(down[[:space:]]+-v|volume[[:space:]]+rm)' <<<"$core_e2e_source"; then
  fail 'AIO Core E2E harness must preserve deployment volumes'
fi

[ -f "$SOAK_TEST" ] || fail 'AIO 2C4G soak resource gate is missing'
[ -x "$SOAK_TEST" ] || fail 'AIO 2C4G soak resource gate must be executable'
bash -n "$SOAK_TEST" || fail 'AIO 2C4G soak resource gate must have valid Bash syntax'
soak_source=$(<"$SOAK_TEST")
require_text "$soak_source" '--self-test' 'soak resource gate must provide a dependency-free self-test'
require_text "$soak_source" '2m.*30m|30m.*2m' 'soak resource gate must accept only the 2m and 30m durations'
require_text "$soak_source" 'BLOCKED' 'missing exact environment prerequisites must report BLOCKED'
require_text "$soak_source" 'SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT.*ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC' 'soak resource gate must require the exact isolated no-live-traffic declaration'
require_text "$soak_source" 'SANDBOX_AIO_CORE_E2E_CODE_SHA' 'soak resource gate must bind its configured code revision'
require_text "$soak_source" 'rev-parse HEAD' 'soak resource gate must compare its code revision with HEAD'
require_text "$soak_source" '--porcelain=v1 --untracked-files=all' 'soak resource gate must reject tracked and untracked worktree changes'
require_text "$soak_source" 'org.opencontainers.image.revision' 'soak resource gate must bind the running Runner OCI revision'
require_text "$soak_source" 'runner_image_id' 'soak evidence must include the full Runner image ID'
require_text "$soak_source" 'runner_image_revision' 'soak evidence must include the Runner OCI revision'
require_text "$soak_source" 'COMPOSE_PROJECT_NAME.*SANDBOX_RUNNER_DEPLOYMENT_ID' 'soak resource gate must bind its Compose project to the isolated deployment'
require_text "$soak_source" 'SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP.*EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES' 'soak resource gate must require exact prefix-only cleanup'
require_text "$soak_source" 'SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES.*EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE' 'soak resource gate must require exact isolated dev fixture commit and cleanup'
require_text "$soak_source" 'SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY.*ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE' 'soak resource gate must restrict database cleanup to rollback-only fixture handling'
require_text "$soak_source" '\^TestAIOCoreE2ESoakWorkload\$' 'soak resource gate must call only the fixed workload Go test'
require_text "$soak_source" '\^TestAIOCoreE2ESoakSnapshot\$' 'soak resource gate must call only the fixed snapshot Go test'
require_text "$soak_source" '\^TestAIOCoreE2ECleanupOnly\$' 'soak resource gate must finish with the fixed cleanup-only Go test'
require_text "$soak_source" 'AIO_CORE_E2E_SNAPSHOT queue=' 'soak resource gate must parse the fixed aggregate snapshot record'
require_text "$soak_source" 'SANDBOX_AIO_CORE_E2E_SOAK_DURATION="\$duration"' 'soak resource gate must pass only its validated duration to the fixed workload Go test'
require_text "$soak_source" 'duplicate key' 'soak controlled environment parsing must reject duplicate keys'
require_text "$soak_source" 'GIT_\*' 'soak controlled environment parsing must reject Git redirection variables'
require_text "$soak_source" 'GOWORK=off' 'soak fixed Go tests must ignore workspace redirection'
require_text "$soak_source" 'go test .* -run "\$workload_test_regex"' 'soak resource gate must execute the fixed workload Go test directly'
require_text "$soak_source" 'go test .* -run "\$snapshot_test_regex"' 'soak resource gate must execute the fixed snapshot Go test directly'
require_text "$soak_source" 'evidence directory must be outside the repository' 'soak evidence must be written outside the repository'
require_text "$soak_source" 'kill -INT "\$workload_pid"' 'soak interruption must first request bounded graceful workload termination'
require_text "$soak_source" 'BLOCKED_EXACT_PREFIX_CLEANUP_REQUIRED' 'interrupted soak evidence must block on exact-prefix residual cleanup'
require_text "$soak_source" 'signed_requests_closed' 'soak resource gate must close signed status reads before cleanup-only verification'
require_text "$soak_source" 'coze-server' 'soak resource gate must include the selected server in its health and resource gates'
require_text "$soak_source" 'host_memory_available' 'soak evidence must record aggregate host reserve samples'
require_text "$soak_source" 'baseline' 'soak resource gate must collect an idle resource baseline'
require_text "$soak_source" 'drain' 'soak resource gate must collect a post-drain steady-state window'
require_text "$soak_source" 'max_' 'soak resource gate must enforce explicit maximum resource thresholds'
if grep -Fq 'SANDBOX_SOAK_COMPOSE_PROJECT' <<<"$soak_source"; then
  fail 'soak resource gate must not select containers through an independent Compose project variable'
fi
if grep -Fq 'SANDBOX_SOAK_WORKLOAD_HELPER' <<<"$soak_source"; then
  fail 'soak resource gate must not trust an external workload helper'
fi
if grep -Eiq -- "printf[[:space:]]+['\"](identity|command|path|dsn|token)[[:space:]]*\\\\t" <<<"$soak_source"; then
  fail 'soak evidence must not record identity, command, path, DSN, or token fields'
fi
"$SOAK_TEST" --self-test >/dev/null || fail 'AIO 2C4G soak resource gate self-test failed'
if grep -Eiq -- '(down[[:space:]]+-v|volume[[:space:]]+rm|flush(all|db)|drop[[:space:]]+(database|schema|table)|truncate|atlas[^[:cntrl:]]*apply|migrate[^[:cntrl:]]*apply)' <<<"$soak_source"; then
  fail 'soak resource gate must not mutate shared databases or delete deployment volumes'
fi
soak_host_port_source=$(printf '%s\n' "$soak_source" | sed 's#http://coze-sandbox-aio:8080##g')
if grep -Eq -- '(^|[^0-9])8080:8080([^0-9]|$)|(^|[^0-9]):8080([^0-9]|$)' <<<"$soak_host_port_source"; then
  fail 'soak resource gate must not publish the AIO port on the host'
fi

printf 'compose contract: ok\n'
