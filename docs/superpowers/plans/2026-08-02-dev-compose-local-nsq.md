# Dev Compose Local NSQ Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the dev deployment run a persistent standalone NSQ beside the two application containers, expose the Web port through a validated host binding, support an optional Redis logical database, and allow startup without a vector database.

**Architecture:** Keep MySQL, Redis, Elasticsearch, object storage, and application credentials external to Compose. Add one internal-only `nsqd` service with a named volume and fixed resource limits; inject its address into `coze-server`, while `deploy.sh` treats NSQ health as part of the release health gate but continues to roll back only the two versioned application images. Parse `REDIS_DB` in the production Redis factory with a bounded readiness check, and map a missing vector-store type to the existing noop manager without weakening explicit-provider failures.

**Tech Stack:** Docker Compose v2, Bash, NSQ 1.3.0, Go 1.24, go-redis v9, miniredis, existing Coze search-store interfaces, GitHub Actions/dev integration runbook.

---

## File Map

- `deploy/dev/docker-compose.yml`: three-service topology, NSQ persistence, dependencies, host port, and log rotation.
- `deploy/dev/.env.example`: non-secret deployment defaults consumed by Compose and `deploy.sh`.
- `deploy/dev/tests/compose_contract_test.sh`: rendered Compose contract.
- `deploy/dev/deploy.sh`: bind validation and composite release health.
- `deploy/dev/tests/deploy_test.sh`: shell behavior without real Docker calls.
- `backend/types/consts/consts.go`: canonical `REDIS_DB` key.
- `backend/infra/cache/impl/redis/redis.go`: production Redis construction and readiness.
- `backend/infra/cache/impl/redis/redis_test.go`: Redis parsing, DB selection, and failure tests.
- `backend/application/base/appinfra/app_infra.go`: startup error propagation.
- `backend/domain/knowledge/service/knowledge_integration_test.go`: adapted Redis factory caller.
- `backend/infra/document/searchstore/impl/impl.go`: missing vector configuration behavior.
- `backend/infra/document/searchstore/impl/impl_test.go`: optional-vector tests.
- `deploy/dev/README.md`: server installation and operations.
- `docs/superpowers/context/project-context.md`: durable project facts.

### Task 1: Lock and Implement the Three-Service Compose Topology

**Files:**
- Modify: `deploy/dev/tests/compose_contract_test.sh`
- Modify: `deploy/dev/docker-compose.yml`
- Modify: `deploy/dev/.env.example`

- [ ] **Step 1: Extend the Compose contract before changing YAML**

Replace `render_config` with:

```bash
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
    docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" config 2>&1) || {
    fail "docker compose config failed: $config_output"
  }
  printf '%s\n' "$config_output"
}
```

After loading `config`, `compose_source`, and `env_source`, add:

```bash
require_text "$env_source" '^WEB_BIND_IP=0\.0\.0\.0$' '.env.example must expose Web on all IPv4 interfaces by default'
require_text "$env_source" '^WEB_PORT=8888$' '.env.example must define the public Web port'

services=$(printf '%s\n' "$config" | awk '/^services:/{in_services=1; next} in_services && /^[^[:space:]]/{exit} in_services && /^  [^[:space:]]/{sub(/^  /, ""); sub(/:$/, ""); print}')
require_exact_text "$services" $'nsqd\ncoze-server\ncoze-web' 'services must be exactly nsqd, coze-server, and coze-web'

nsqd_config=$(service_block "$config" nsqd)
nsqd_source=$(service_block "$compose_source" nsqd)
server_config=$(service_block "$config" coze-server)
web_config=$(service_block "$config" coze-web)

require_text "$nsqd_config" 'image: nsqio/nsq:v1\.3\.0' 'nsqd must use the pinned 1.3.0 image'
require_text "$nsqd_config" 'pull_policy: missing' 'nsqd must use the fixed local image after first pull'
require_text "$nsqd_config" '/nsqd' 'nsqd command must invoke the broker'
require_text "$nsqd_config" '--data-path=/data' 'nsqd must write queue data under /data'
require_text "$nsqd_config" '--mem-queue-size=0' 'nsqd must use disk-backed queues'
require_text "$nsqd_config" 'source: nsq-data' 'nsqd must mount the named data volume'
require_text "$nsqd_config" 'target: /data' 'NSQ data volume must target /data'
require_text "$nsqd_config" 'http://127\.0\.0\.1:4151/ping' 'nsqd healthcheck must use /ping'
require_text "$nsqd_config" 'grep -qx OK' 'nsqd healthcheck must require exact OK'
require_text "$nsqd_source" '^    mem_limit: 384m$' 'nsqd memory must be capped at 384 MiB'
require_text "$nsqd_source" '^    cpus: 0\.50$' 'nsqd CPU must be capped at half a core'
require_text "$nsqd_source" '^    pids_limit: 128$' 'nsqd process count must be capped'
require_text "$nsqd_source" '^    stop_grace_period: 30s$' 'nsqd must receive a graceful shutdown window'
if printf '%s\n' "$nsqd_config" | grep -Eq -- '^    ports:$'; then
  fail 'nsqd must not publish a host port'
fi

require_text "$server_config" 'COZE_MQ_TYPE: nsq' 'coze-server must use NSQ'
require_text "$server_config" 'MQ_NAME_SERVER: nsqd:4150' 'coze-server must use internal nsqd'
require_text "$server_config" 'condition: service_healthy' 'coze-server must wait for healthy nsqd'
require_text "$server_config" 'restart: true' 'coze-server must follow explicit nsqd restarts'

for service_config in "$nsqd_config" "$server_config" "$web_config"; do
  require_text "$service_config" 'driver: json-file' 'every service must use json-file logging'
  require_text "$service_config" 'max-size: 10m' 'every service must rotate logs at 10 MiB'
  require_text "$service_config" 'max-file: "3"' 'every service must retain three log files'
done

require_text "$config" '^volumes:$' 'Compose must declare named volumes'
require_text "$config" '^  nsq-data:$' 'Compose must declare nsq-data'
require_text "$web_config" 'host_ip: 0\.0\.0\.0' 'coze-web must bind all IPv4 interfaces by default'
require_text "$web_config" 'published: "8888"' 'coze-web must publish port 8888 by default'

custom_config=$(render_config dev dev 127.0.0.1 18888)
custom_web_config=$(service_block "$custom_config" coze-web)
require_text "$custom_web_config" 'host_ip: 127\.0\.0\.1' 'WEB_BIND_IP must override host binding'
require_text "$custom_web_config" 'published: "18888"' 'WEB_PORT must override published port'
```

Delete the former assertion that requires `host_ip: 127.0.0.1`.

- [ ] **Step 2: Run the test and verify the old topology fails**

Run:

```bash
bash deploy/dev/tests/compose_contract_test.sh
```

Expected: non-zero exit with `services must be exactly nsqd, coze-server, and coze-web`.

- [ ] **Step 3: Replace the Compose file with the approved topology**

Use this complete `deploy/dev/docker-compose.yml`:

```yaml
# Copyright 2025 coze-dev Authors
# SPDX-License-Identifier: Apache-2.0

x-logging: &default-logging
  driver: json-file
  options:
    max-size: "10m"
    max-file: "3"

services:
  nsqd:
    image: nsqio/nsq:v1.3.0
    pull_policy: missing
    command:
      - /nsqd
      - --data-path=/data
      - --mem-queue-size=0
    restart: unless-stopped
    stop_grace_period: 30s
    mem_limit: 384m
    cpus: 0.50
    pids_limit: 128
    expose:
      - "4150"
      - "4151"
    volumes:
      - nsq-data:/data
    healthcheck:
      test:
        - CMD-SHELL
        - wget -q -O - http://127.0.0.1:4151/ping | grep -qx OK
      interval: 10s
      timeout: 5s
      retries: 12
      start_period: 10s
    logging: *default-logging
    networks:
      - coze-dev

  coze-server:
    image: ${ACR_REGISTRY:?ACR_REGISTRY is required}/${ACR_NAMESPACE:?ACR_NAMESPACE is required}/coze-server:${SERVER_IMAGE_TAG:-dev}
    restart: unless-stopped
    environment:
      COZE_MQ_TYPE: nsq
      MQ_NAME_SERVER: nsqd:4150
    expose:
      - "8888"
    volumes:
      - type: bind
        source: ./app.env
        target: /app/.env
        read_only: true
        bind:
          create_host_path: false
    depends_on:
      nsqd:
        condition: service_healthy
        restart: true
    healthcheck:
      test:
        - CMD
        - curl
        - --fail
        - --silent
        - --show-error
        - http://127.0.0.1:8888/healthz
      interval: 10s
      timeout: 5s
      retries: 12
      start_period: 20s
    logging: *default-logging
    networks:
      - coze-dev

  coze-web:
    image: ${ACR_REGISTRY:?ACR_REGISTRY is required}/${ACR_NAMESPACE:?ACR_NAMESPACE is required}/coze-web:${WEB_IMAGE_TAG:-dev}
    restart: unless-stopped
    ports:
      - "${WEB_BIND_IP:-0.0.0.0}:${WEB_PORT:-8888}:80"
    depends_on:
      coze-server:
        condition: service_healthy
        restart: true
    healthcheck:
      test:
        - CMD
        - wget
        - --quiet
        - --tries=1
        - --spider
        - http://127.0.0.1/healthz
      interval: 10s
      timeout: 5s
      retries: 12
      start_period: 10s
    logging: *default-logging
    networks:
      - coze-dev

networks:
  coze-dev:
    driver: bridge

volumes:
  nsq-data:
```

- [ ] **Step 4: Add non-secret host defaults**

Add before `DEPLOY_HEALTH_TIMEOUT_SECONDS` in `.env.example`:

```dotenv
WEB_BIND_IP=0.0.0.0
WEB_PORT=8888
```

- [ ] **Step 5: Render and test**

```bash
docker compose --env-file deploy/dev/.env.example -f deploy/dev/docker-compose.yml config --quiet
bash deploy/dev/tests/compose_contract_test.sh
```

Expected: both exit `0`; test ends with `compose contract: ok`.

- [ ] **Step 6: Commit**

```bash
git add deploy/dev/docker-compose.yml deploy/dev/.env.example deploy/dev/tests/compose_contract_test.sh
git commit -m "feat: add persistent nsq to dev compose"
```

### Task 2: Validate Web Binding and Gate Releases on NSQ Health

**Files:**
- Modify: `deploy/dev/tests/deploy_test.sh`
- Modify: `deploy/dev/deploy.sh`

- [ ] **Step 1: Add failing shell tests**

Add valid defaults to `write_direct_case_files` after `ACR_NAMESPACE`:

```bash
    'WEB_BIND_IP=0.0.0.0' \
    'WEB_PORT=8888' \
```

Add before `run_test`:

```bash
test_web_health_base_url_uses_configured_binding() (
  WEB_BIND_IP=0.0.0.0
  WEB_PORT=18888
  [ "$(web_health_base_url)" = 'http://127.0.0.1:18888' ] ||
    fail 'wildcard binding did not use loopback for local health'
  WEB_BIND_IP=192.0.2.10
  [ "$(web_health_base_url)" = 'http://192.0.2.10:18888' ] ||
    fail 'specific binding did not use its configured host'
)

test_health_checks_require_healthy_nsqd() (
  service_is_healthy() { [ "$1" != nsqd ]; }
  compose_cmd() { fail 'backend health ran while nsqd was unhealthy'; }
  curl() { fail 'web health ran while nsqd was unhealthy'; }
  if health_checks_pass "$REV_A"; then
    fail 'health checks passed while nsqd was unhealthy'
  fi
)

test_invalid_web_bind_ip_fails_before_docker() (
  case_dir=$(mktemp -d "$TEST_ROOT/invalid-bind.XXXXXX")
  write_direct_case_files "$case_dir"
  printf '%s\n' 'WEB_BIND_IP=999.0.0.1' >> "$case_dir/deploy.env"
  marker=$case_dir/docker-called
  if DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" >"$case_dir/output.log" 2>&1; then
    fail 'invalid WEB_BIND_IP unexpectedly succeeded'
  fi
  [ ! -e "$marker" ] || fail 'invalid WEB_BIND_IP reached Docker'
  assert_file_contains "$case_dir/output.log" 'WEB_BIND_IP must be a valid IPv4 address' \
    'invalid WEB_BIND_IP did not report its error'
)

test_invalid_web_port_fails_before_docker() (
  case_dir=$(mktemp -d "$TEST_ROOT/invalid-port.XXXXXX")
  write_direct_case_files "$case_dir"
  printf '%s\n' 'WEB_PORT=65536' >> "$case_dir/deploy.env"
  marker=$case_dir/docker-called
  if DOCKER_MARKER="$marker" PATH="$case_dir/bin:$PATH" \
    DEPLOY_ROOT_DIR="$case_dir" DEPLOY_ENV_FILE="$case_dir/deploy.env" \
    DEPLOY_LOCK_FILE="$case_dir/deploy.lock" \
    bash "$DEPLOY_SCRIPT" >"$case_dir/output.log" 2>&1; then
    fail 'invalid WEB_PORT unexpectedly succeeded'
  fi
  [ ! -e "$marker" ] || fail 'invalid WEB_PORT reached Docker'
  assert_file_contains "$case_dir/output.log" 'WEB_PORT must be an integer from 1 to 65535' \
    'invalid WEB_PORT did not report its error'
)
```

Register:

```bash
run_test 'Web health URL follows bind configuration' test_web_health_base_url_uses_configured_binding
run_test 'NSQ health is required for release health' test_health_checks_require_healthy_nsqd
run_test 'invalid Web bind IP fails before Docker' test_invalid_web_bind_ip_fails_before_docker
run_test 'invalid Web port fails before Docker' test_invalid_web_port_fails_before_docker
```

- [ ] **Step 2: Verify the tests fail before implementation**

```bash
bash deploy/dev/tests/deploy_test.sh
```

Expected: non-zero because `web_health_base_url` or `health_checks_pass` is undefined.

- [ ] **Step 3: Add bind and health helpers**

Add after `normalize_revision`:

```bash
is_ipv4() {
  local ip=$1 octet
  local -a octets
  [[ "$ip" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || return 1
  IFS=. read -r -a octets <<< "$ip"
  [ "${#octets[@]}" -eq 4 ] || return 1
  for octet in "${octets[@]}"; do
    ((10#$octet <= 255)) || return 1
  done
}

is_tcp_port() {
  [[ "$1" =~ ^[1-9][0-9]{0,4}$ ]] && ((10#$1 <= 65535))
}

web_health_base_url() {
  local host=${WEB_BIND_IP:-0.0.0.0}
  [ "$host" = 0.0.0.0 ] && host=127.0.0.1
  printf 'http://%s:%s' "$host" "${WEB_PORT:-8888}"
}
```

Add after `image_id`:

```bash
service_health_status() {
  local service=$1 container_id
  container_id=$(compose_cmd ps -q "$service") || return 1
  [ -n "$container_id" ] || return 1
  docker_cmd inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{end}}' "$container_id"
}

service_is_healthy() {
  local status
  status=$(service_health_status "$1" 2>/dev/null) || return 1
  [ "$status" = healthy ]
}
```

- [ ] **Step 4: Make each retry include NSQ and the configured Web port**

Add before `wait_for_health`:

```bash
health_checks_pass() {
  local expected_revision=${1:-}
  local backend_body web_body web_base_url
  service_is_healthy nsqd || return 1
  web_base_url=$(web_health_base_url)
  backend_body=$(compose_cmd exec -T coze-server curl --fail --silent --show-error --max-time 5 http://127.0.0.1:8888/healthz 2>/dev/null) || return 1
  health_body_matches "$backend_body" "$expected_revision" || return 1
  web_body=$(curl --fail --silent --show-error --max-time 5 "$web_base_url/healthz" 2>/dev/null) || return 1
  health_body_matches "$web_body" "$expected_revision" || return 1
  curl --fail --silent --show-error --max-time 5 --output /dev/null "$web_base_url/" 2>/dev/null
}
```

Replace `wait_for_health` with:

```bash
wait_for_health() {
  local expected_revision=${1:-}
  local timeout=${DEPLOY_HEALTH_TIMEOUT_SECONDS:-120}
  local deadline
  [[ "$timeout" =~ ^[1-9][0-9]*$ ]] || {
    error 'DEPLOY_HEALTH_TIMEOUT_SECONDS must be a positive integer'
    return 1
  }
  deadline=$((SECONDS + timeout))
  while ((SECONDS < deadline)); do
    if health_checks_pass "$expected_revision"; then
      return 0
    fi
    sleep 2
  done
  error "health checks did not pass within ${timeout}s"
  return 1
}
```

`deploy_transaction` and `rollback_images` already call this function, so both paths require healthy NSQ while continuing to update only `coze-server` and `coze-web`.

- [ ] **Step 5: Add preflight before Docker checks**

After health-timeout validation in `main`, add:

```bash
  WEB_BIND_IP=${WEB_BIND_IP:-0.0.0.0}
  WEB_PORT=${WEB_PORT:-8888}
  if ! is_ipv4 "$WEB_BIND_IP"; then
    error 'WEB_BIND_IP must be a valid IPv4 address'
    return 1
  fi
  if ! is_tcp_port "$WEB_PORT"; then
    error 'WEB_PORT must be an integer from 1 to 65535'
    return 1
  fi
  export WEB_BIND_IP WEB_PORT
```

- [ ] **Step 6: Run syntax and behavior tests**

```bash
bash -n deploy/dev/deploy.sh deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/compose_contract_test.sh
```

Expected: syntax exits `0`, deploy ends with `deploy tests: 21 passed`, and Compose ends with `compose contract: ok`.

- [ ] **Step 7: Commit**

```bash
git add deploy/dev/deploy.sh deploy/dev/tests/deploy_test.sh
git commit -m "feat: validate dev endpoint and nsq health"
```

### Task 3: Add Strict `REDIS_DB` Selection and Startup Readiness

**Files:**
- Modify: `backend/types/consts/consts.go`
- Modify: `backend/infra/cache/impl/redis/redis.go`
- Modify: `backend/infra/cache/impl/redis/redis_test.go`
- Modify: `backend/application/base/appinfra/app_infra.go`
- Modify: `backend/domain/knowledge/service/knowledge_integration_test.go`

- [ ] **Step 1: Add parsing, selected-DB, and readiness failure tests**

Add `strings` to the imports in `redis_test.go`, then append:

```go
func TestParseRedisDB(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{name: "missing", raw: "", want: 0},
		{name: "blank", raw: "   ", want: 0},
		{name: "zero", raw: "0", want: 0},
		{name: "selected", raw: " 2 ", want: 2},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "not decimal", raw: "1.5", wantErr: true},
		{name: "signed", raw: "+1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRedisDB(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseRedisDB(%q) unexpectedly succeeded", tt.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseRedisDB(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("parseRedisDB(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNewUsesConfiguredRedisDB(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	t.Setenv("REDIS_ADDR", server.Addr())
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "2")

	client, err := New(context.Background())
	if err != nil {
		t.Fatalf("initialize redis: %v", err)
	}
	if err := client.Set(context.Background(), "selected-db", "ok", 0).Err(); err != nil {
		t.Fatalf("write selected database: %v", err)
	}
	if _, err := server.DB(0).Get("selected-db"); !errors.Is(err, miniredis.ErrKeyNotFound) {
		t.Fatalf("DB 0 unexpectedly contains selected-db: %v", err)
	}
	got, err := server.DB(2).Get("selected-db")
	if err != nil || got != "ok" {
		t.Fatalf("DB 2 selected-db = %q, %v", got, err)
	}
}

func TestNewPropagatesReadinessFailure(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(server.Close)
	server.SetError("ERR selected database unavailable")
	t.Setenv("REDIS_ADDR", server.Addr())
	t.Setenv("REDIS_PASSWORD", "")
	t.Setenv("REDIS_DB", "3")

	_, err = New(context.Background())
	if err == nil || !strings.Contains(err.Error(), "redis readiness check failed") {
		t.Fatalf("New() readiness error = %v", err)
	}
}
```

- [ ] **Step 2: Run the new tests and verify the API is absent**

```bash
cd backend
go test ./infra/cache/impl/redis -run 'Test(ParseRedisDB|New)' -count=1
```

Expected: compile failure because `parseRedisDB` does not exist and `New` does not accept a context or return an error.

- [ ] **Step 3: Define the environment key**

Add next to `RedisAddr` in `consts.go`:

```go
RedisDB = "REDIS_DB"
```

- [ ] **Step 4: Implement strict parsing and production readiness**

Add `fmt`, `strconv`, `strings`, and `backend/types/consts` to `redis.go`. Replace the constructors with:

```go
const redisReadinessTimeout = 5 * time.Second

func New(ctx context.Context) (cache.Cmdable, error) {
	if ctx == nil {
		return nil, fmt.Errorf("redis initialization context is nil")
	}
	db, err := parseRedisDB(os.Getenv(consts.RedisDB))
	if err != nil {
		return nil, err
	}
	client := newWithAddrPasswordAndDB(
		os.Getenv(consts.RedisAddr),
		os.Getenv("REDIS_PASSWORD"),
		db,
	)
	readinessCtx, cancel := context.WithTimeout(ctx, redisReadinessTimeout)
	defer cancel()
	if err := client.CheckReadiness(readinessCtx); err != nil {
		_ = client.client.Close()
		return nil, fmt.Errorf("redis readiness check failed: %w", err)
	}
	return client, nil
}

func parseRedisDB(raw string) (int, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, nil
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("%s must be a non-negative decimal integer", consts.RedisDB)
		}
	}
	db, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a non-negative decimal integer: %w", consts.RedisDB, err)
	}
	return db, nil
}

func NewWithAddrAndPassword(addr, password string) cache.Cmdable {
	return newWithAddrPasswordAndDB(addr, password, 0)
}

func newWithAddrPasswordAndDB(addr, password string, db int) *redisImpl {
	cache.SetDefaultNilError(redis.Nil)
	rdb := redis.NewClient(&redis.Options{
		Addr:            addr,
		DB:              db,
		Password:        password,
		PoolSize:        100,
		MinIdleConns:    10,
		MaxIdleConns:    30,
		ConnMaxIdleTime: 5 * time.Minute,
		DialTimeout:     5 * time.Second,
		ReadTimeout:     3 * time.Second,
		WriteTimeout:    3 * time.Second,
	})
	return &redisImpl{client: rdb}
}
```

The public test/helper constructor stays fixed to DB 0 and does not add a readiness round trip.

- [ ] **Step 5: Propagate the constructor error**

Replace the cache assignment in `app_infra.go` with:

```go
	deps.CacheCli, err = redis.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("init redis failed, err=%w", err)
	}
```

Replace the fixture assignment in `knowledge_integration_test.go` with:

```go
	cacheCli, err := redis.New(ctx)
	if err != nil {
		panic(err)
	}
```

- [ ] **Step 6: Format, test, and compile changed callers**

```bash
gofmt -w backend/types/consts/consts.go backend/infra/cache/impl/redis/redis.go backend/infra/cache/impl/redis/redis_test.go backend/application/base/appinfra/app_infra.go backend/domain/knowledge/service/knowledge_integration_test.go
cd backend
go test ./infra/cache/impl/redis -count=1
go test ./application/base/appinfra -count=1
go test ./domain/knowledge/service -run '^$' -count=1
```

Expected: all exit `0`; Redis reports `ok`, and both caller packages compile without running the knowledge integration suite.

- [ ] **Step 7: Commit**

```bash
git add backend/types/consts/consts.go backend/infra/cache/impl/redis/redis.go backend/infra/cache/impl/redis/redis_test.go backend/application/base/appinfra/app_infra.go backend/domain/knowledge/service/knowledge_integration_test.go
git commit -m "feat: support redis logical database selection"
```

### Task 4: Make an Unconfigured Vector Store Explicitly Noop

**Files:**
- Create: `backend/infra/document/searchstore/impl/impl_test.go`
- Modify: `backend/infra/document/searchstore/impl/impl.go`

- [ ] **Step 1: Add provider-free behavior tests**

Create `impl_test.go`:

```go
// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package impl

import (
	"context"
	"strings"
	"testing"
)

func TestGetVectorStoreUsesNoopWhenDisabled(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "missing", value: ""},
		{name: "blank", value: "   "},
		{name: "none", value: "none"},
		{name: "noop", value: "noop"},
		{name: "disabled", value: "disabled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("COZE_DEBUG_SKIP_VECTOR_STORE", "false")
			t.Setenv("VECTOR_STORE_TYPE", tt.value)
			manager, err := getVectorStore(context.Background(), nil)
			if err != nil {
				t.Fatalf("getVectorStore(%q): %v", tt.value, err)
			}
			if _, ok := manager.(noopVectorManager); !ok {
				t.Fatalf("getVectorStore(%q) returned %T, want noopVectorManager", tt.value, manager)
			}
		})
	}
}

func TestGetVectorStoreRejectsUnknownType(t *testing.T) {
	t.Setenv("COZE_DEBUG_SKIP_VECTOR_STORE", "false")
	t.Setenv("VECTOR_STORE_TYPE", "unknown-provider")
	_, err := getVectorStore(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected vector store type") {
		t.Fatalf("unknown vector store error = %v", err)
	}
}
```

- [ ] **Step 2: Verify blank configuration still fails**

```bash
cd backend
go test ./infra/document/searchstore/impl -run 'TestGetVectorStore' -count=1
```

Expected: `missing` or `blank` fails with `unexpected vector store type`.

- [ ] **Step 3: Implement missing-as-noop with a safe warning**

Import:

```go
"github.com/coze-dev/coze-studio/backend/pkg/logs"
```

Change the beginning of the switch to:

```go
	switch vsType {
	case "":
		logs.Warnf("VECTOR_STORE_TYPE is not configured; semantic vector retrieval is disabled")
		return newNoopVectorManager(), nil
	case "none", "noop", "disabled":
		return newNoopVectorManager(), nil
```

Leave all explicit provider cases and `default` unchanged.

- [ ] **Step 4: Format and test**

```bash
gofmt -w backend/infra/document/searchstore/impl/impl.go backend/infra/document/searchstore/impl/impl_test.go
cd backend
go test ./infra/document/searchstore/impl -run 'TestGetVectorStore' -count=1
```

Expected: package reports `ok`; no real vector database is contacted.

- [ ] **Step 5: Commit**

```bash
git add backend/infra/document/searchstore/impl/impl.go backend/infra/document/searchstore/impl/impl_test.go
git commit -m "feat: allow startup without vector storage"
```

### Task 5: Update the Operator Runbook and Long-Lived Context

**Files:**
- Modify: `deploy/dev/README.md`
- Modify: `docs/superpowers/context/project-context.md`

- [ ] **Step 1: Replace stale topology and access facts**

Replace the opening deployment description in `deploy/dev/README.md` with:

```markdown
本目录用于单实例 dev/预发布环境。服务器运行 `nsqd`、`coze-server` 和
`coze-web` 三个容器；MySQL、Elasticsearch、Redis 和对象存储继续使用远程
云服务。`nsqd` 使用 `nsq-data` 命名卷持久化消息，并固定为磁盘队列模式。

`coze-web` 默认发布到 `0.0.0.0:8888`，可通过公网 IP 和端口直接访问；
`coze-server` 与 NSQ 不发布宿主机端口。公网端口是 HTTP，正式域名和 HTTPS
由宝塔 Nginx 反向代理处理。该拓扑允许短时发布中断，不是高可用生产方案。
```

- [ ] **Step 2: Document deployment and application configuration**

Add these paragraphs to the `deploy.env` and `app.env` sections:

```markdown
`WEB_BIND_IP` 必须是合法 IPv4 地址，默认 `0.0.0.0`；`WEB_PORT` 必须是
`1` 到 `65535` 的整数，默认 `8888`。调试公网访问时需要同步放行安全组和主机
防火墙；正式域名仍代理到本机相同端口。

Compose 固定向后端注入 `COZE_MQ_TYPE=nsq` 和 `MQ_NAME_SERVER=nsqd:4150`，
不要在 `app.env` 重复配置消息队列。`REDIS_DB` 可省略，默认使用逻辑库 `0`；
显式值必须是非负十进制整数。Redis Cluster 或只支持 DB 0 的云实例必须保持
`REDIS_DB=0`。

`VECTOR_STORE_TYPE` 及 provider 专属变量可以全部省略。此时 Elasticsearch
全文检索继续工作，语义向量检索关闭；显式配置 `milvus`、`vikingdb` 或
`oceanbase` 后仍会严格校验并在依赖不可用时阻止启动。

`app.env` 不要求 `USE_SSL` 或 `SERVER_HOST`。容器内后端保持 HTTP；公网 URL
先在系统管理页面配置为公网 IP 与端口，宝塔域名启用后再改为最终 HTTPS 域名。
```

Update the server file list to exactly `.env.example`, `docker-compose.yml`, `deploy.sh`, `deploy.env`, and `app.env`. State that `nsq-data` is Docker-managed and is not copied as a normal directory.

- [ ] **Step 3: Add NSQ operations and data warnings**

Add:

````markdown
## NSQ 数据与排障

```bash
cd /opt/coze-dev
docker compose --env-file deploy.env -f docker-compose.yml ps nsqd
docker compose --env-file deploy.env -f docker-compose.yml logs --tail=200 nsqd
docker compose --env-file deploy.env -f docker-compose.yml exec -T nsqd \
  wget -q -O - http://127.0.0.1:4151/ping
docker volume inspect coze-dev_nsq-data
docker system df -v
```

健康响应必须是 `OK`。日常部署和应用镜像回滚只更新 `coze-server` 与
`coze-web`，保留 `nsqd` 和 `nsq-data`。禁止执行 `docker compose down -v`；
升级 NSQ、迁移卷或清理队列必须作为独立运维变更执行。单节点 NSQ 没有副本，
宿主机磁盘损坏或强制删除卷仍会丢失消息。
````

Include `nsqd` in the existing logs command. For a custom port, use the value from `deploy.env` rather than hard-coding `8888` in the health command.

- [ ] **Step 4: Update the durable project fact**

Replace the sentence in `project-context.md` that says the server runs two containers with:

```markdown
该服务器运行两个应用容器和一个持久化的单节点 `nsqd`；MySQL、Elasticsearch、
Redis 和对象存储均为远程服务。NSQ 只在 Compose 网络中可见，业务发布与回滚
保留其命名卷。dev 部署允许省略向量数据库配置，未配置时保留 Elasticsearch
全文检索并关闭语义向量检索。Web 默认通过可配置的公网 HTTP 端口发布，域名与
TLS 由宝塔独立终止。
```

- [ ] **Step 5: Scan stale claims and whitespace**

```bash
rg -n '只运行.*两个|只绑定.*127\.0\.0\.1|VECTOR_STORE_TYPE.*必须|消息队列.*app\.env|down -v' deploy/dev/README.md docs/superpowers/context/project-context.md
git diff --check
```

Expected: only the intentional warning prohibiting `down -v` remains; there is no stale two-container, loopback-only, mandatory-vector, or app-env MQ claim. `git diff --check` exits `0`.

- [ ] **Step 6: Commit**

```bash
git add deploy/dev/README.md docs/superpowers/context/project-context.md
git commit -m "docs: update dev nsq operations"
```

### Task 6: Run Static, Unit, Compose, and Local Full-Startup Verification

**Files:**
- No tracked files change.
- Create temporarily outside Git: `/private/tmp/coze-dev-smoke/`

- [ ] **Step 1: Run the deterministic verification matrix**

From the worktree root:

```bash
bash -n deploy/dev/deploy.sh deploy/dev/tests/deploy_test.sh deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
bash deploy/dev/tests/compose_contract_test.sh
docker compose --env-file deploy/dev/.env.example -f deploy/dev/docker-compose.yml config --quiet
cd backend
go test ./infra/cache/impl/redis -count=1
go test ./infra/document/searchstore/impl -run 'TestGetVectorStore' -count=1
go test ./application/base/appinfra -count=1
go test ./domain/knowledge/service -run '^$' -count=1
go build ./...
cd ..
git diff --check
git status --short
```

Expected: all exit `0`; the worktree is clean after the five implementation commits.

- [ ] **Step 2: Ensure Docker is available**

```bash
docker info >/dev/null
```

Expected: exit `0`. The daemon was stopped during design. If still unavailable, request GUI approval, run `open -a Docker`, and wait for `docker info` before continuing.

- [ ] **Step 3: Build the current backend and frontend images**

```bash
revision=$(git rev-parse HEAD)
docker build --build-arg GIT_REVISION="$revision" --build-arg SOURCE_URL=local-smoke \
  -f backend/Dockerfile -t local/coze-dev/coze-server:smoke .
docker build --build-arg GIT_REVISION="$revision" --build-arg SOURCE_URL=local-smoke \
  -f frontend/Dockerfile -t local/coze-dev/coze-web:smoke .
```

Expected: both exit `0`; each image label `org.opencontainers.image.revision` is the current full SHA.

- [ ] **Step 4: Create a private server-shaped smoke directory**

Run Steps 4 through 9 in the same interactive shell so the no-echo values, cleanup trap, and helper functions stay in scope.

```bash
SMOKE_DIR=/private/tmp/coze-dev-smoke
install -d -m 700 "$SMOKE_DIR"
install -m 640 deploy/dev/docker-compose.yml "$SMOKE_DIR/docker-compose.yml"
printf '%s\n' \
  'ACR_REGISTRY=local' \
  'ACR_NAMESPACE=coze-dev' \
  'SERVER_IMAGE_TAG=smoke' \
  'WEB_IMAGE_TAG=smoke' \
  'WEB_BIND_IP=0.0.0.0' \
  'WEB_PORT=18888' \
  'DEPLOY_HEALTH_TIMEOUT_SECONDS=180' > "$SMOKE_DIR/deploy.env"
chmod 600 "$SMOKE_DIR/deploy.env"
cleanup_smoke_secrets() {
  rm -f "$SMOKE_DIR/app.env" "$SMOKE_DIR/app.env.next" "$SMOKE_DIR/mysql.cnf" \
    "$SMOKE_DIR/es-curl.conf" "$SMOKE_DIR/service.log"
  unset MYSQL_PASSWORD MYSQL_DSN REDIS_PASSWORD REDISCLI_AUTH ES_PASSWORD
}
trap cleanup_smoke_secrets EXIT
```

Use an existing ignored private application env as the base for object-storage and application keys. Collect remote values through no-echo prompts, never command arguments:

```bash
set +x
read -r -p 'Private base app.env path: ' BASE_APP_ENV
test -f "$BASE_APP_ENV"
install -m 600 "$BASE_APP_ENV" "$SMOKE_DIR/app.env"
read -r -p 'MySQL host: ' MYSQL_HOST
read -r -p 'MySQL port: ' MYSQL_PORT
read -r -p 'MySQL database: ' MYSQL_DATABASE
read -r -p 'MySQL user: ' MYSQL_USER
read -r -s -p 'MySQL password: ' MYSQL_PASSWORD; printf '\n'
read -r -s -p 'MySQL DSN: ' MYSQL_DSN; printf '\n'
read -r -p 'Redis address: ' REDIS_ADDR
read -r -s -p 'Redis password: ' REDIS_PASSWORD; printf '\n'
read -r -p 'Redis database [0]: ' REDIS_DB
REDIS_DB=${REDIS_DB:-0}
read -r -p 'Elasticsearch address: ' ES_ADDR
read -r -p 'Elasticsearch version: ' ES_VERSION
read -r -p 'Elasticsearch username: ' ES_USERNAME
read -r -s -p 'Elasticsearch password: ' ES_PASSWORD; printf '\n'
awk '!/^(export[[:space:]]+)?(LISTEN_ADDR|LOG_LEVEL|MYSQL_HOST|MYSQL_PORT|MYSQL_DATABASE|MYSQL_USER|MYSQL_PASSWORD|MYSQL_DSN|REDIS_ADDR|REDIS_PASSWORD|REDIS_DB|ES_ADDR|ES_VERSION|ES_USERNAME|ES_PASSWORD|USE_SSL|SERVER_HOST|VECTOR_STORE_TYPE|MILVUS_|VIKING_DB_|OCEANBASE_)/' \
  "$SMOKE_DIR/app.env" > "$SMOKE_DIR/app.env.next"
printf '%s\n' \
  'LISTEN_ADDR=:8888' \
  'LOG_LEVEL=info' \
  "MYSQL_HOST=$MYSQL_HOST" \
  "MYSQL_PORT=$MYSQL_PORT" \
  "MYSQL_DATABASE=$MYSQL_DATABASE" \
  "MYSQL_USER=$MYSQL_USER" \
  "MYSQL_PASSWORD=$MYSQL_PASSWORD" \
  "MYSQL_DSN=$MYSQL_DSN" \
  "REDIS_ADDR=$REDIS_ADDR" \
  "REDIS_PASSWORD=$REDIS_PASSWORD" \
  "REDIS_DB=$REDIS_DB" \
  "ES_ADDR=$ES_ADDR" \
  "ES_VERSION=$ES_VERSION" \
  "ES_USERNAME=$ES_USERNAME" \
  "ES_PASSWORD=$ES_PASSWORD" >> "$SMOKE_DIR/app.env.next"
chmod 600 "$SMOKE_DIR/app.env.next"
mv "$SMOKE_DIR/app.env.next" "$SMOKE_DIR/app.env"
if grep -Eq '^(export[[:space:]]+)?(USE_SSL|SERVER_HOST|VECTOR_STORE_TYPE|MILVUS_|VIKING_DB_|OCEANBASE_)' "$SMOKE_DIR/app.env"; then
  exit 1
fi
```

Expected: mode `600`; it contains the supplied remote dependencies plus existing private object-storage/application values, and omits `USE_SSL`, `SERVER_HOST`, and all vector variables. If no complete private base exists, stop and request only the missing object-storage/application values; do not substitute fake credentials or weaken startup.

- [ ] **Step 5: Verify remote authentication without business-data changes**

Create client config files from in-memory variables:

```bash
printf '%s\n' \
  '[client]' \
  "host=$MYSQL_HOST" \
  "port=$MYSQL_PORT" \
  "user=$MYSQL_USER" \
  "password=$MYSQL_PASSWORD" > "$SMOKE_DIR/mysql.cnf"
chmod 600 "$SMOKE_DIR/mysql.cnf"
docker run --rm -v "$SMOKE_DIR/mysql.cnf:/tmp/mysql.cnf:ro" mysql:8.4 \
  mysqladmin --defaults-extra-file=/tmp/mysql.cnf ping --silent

printf 'user = "%s:%s"\nsilent\nshow-error\nfail\n' "$ES_USERNAME" "$ES_PASSWORD" \
  > "$SMOKE_DIR/es-curl.conf"
chmod 600 "$SMOKE_DIR/es-curl.conf"
curl --config "$SMOKE_DIR/es-curl.conf" --output /dev/null "$ES_ADDR/_cluster/health"

REDIS_HOST=${REDIS_ADDR%:*}
REDIS_PORT=${REDIS_ADDR##*:}
export REDISCLI_AUTH=$REDIS_PASSWORD
redis_probe="codex:dev-compose:$(date +%s):$$"
redis_cli() {
  docker run --rm -e REDISCLI_AUTH redis:7.4-alpine redis-cli --no-auth-warning \
    -h "$REDIS_HOST" -p "$REDIS_PORT" -n "$REDIS_DB" "$@"
}
[ "$(redis_cli SET "$redis_probe" ok EX 60)" = OK ]
[ "$(redis_cli GET "$redis_probe")" = ok ]
[ "$(redis_cli DEL "$redis_probe")" = 1 ]
```

Expected: MySQL reports alive, ES returns 2xx, and the unique Redis key is written, read, and deleted only in the configured DB.

- [ ] **Step 6: Start and inspect all services**

```bash
cd "$SMOKE_DIR"
compose_smoke() {
  docker compose --project-name coze-dev-smoke --env-file deploy.env -f docker-compose.yml "$@"
}
compose_smoke up -d
timeout=240
until [ "$timeout" -le 0 ]; do
  if curl --fail --silent --show-error --output /dev/null http://127.0.0.1:18888/healthz &&
    curl --fail --silent --show-error --output /dev/null http://127.0.0.1:18888/; then
    break
  fi
  sleep 5
  timeout=$((timeout - 5))
done
[ "$timeout" -gt 0 ]
compose_smoke ps
[ "$(compose_smoke exec -T nsqd wget -q -O - http://127.0.0.1:4151/ping)" = OK ]
compose_smoke port coze-web 80 | grep -Eq '0\.0\.0\.0:18888$'
nsqd_id=$(compose_smoke ps -q nsqd)
docker inspect --format '{{.HostConfig.Memory}} {{.HostConfig.NanoCpus}} {{.HostConfig.PidsLimit}}' "$nsqd_id" |
  grep -qx '402653184 500000000 128'
```

Expected: all three services are `running (healthy)`; `/healthz` and `/` return 2xx; Web binds `0.0.0.0:18888`; backend and NSQ have no host port.

- [ ] **Step 7: Verify the rendered page in the in-app browser**

Open `http://127.0.0.1:18888` with the Codex in-app browser. Verify the application shell renders, the page is not blank, authentication or workspace entry is visible, and the browser console has no startup-blocking error. Record the URL, visible state, and console result in the verification evidence.

Expected: the real Web UI renders through `coze-web`; this is not replaced by the `/healthz` API check.

- [ ] **Step 8: Verify NSQ data survives container recreation**

```bash
topic="codex_smoke_$(date +%s)_$$"
channel=persist
export NSQ_TOPIC=$topic NSQ_CHANNEL=$channel
compose_smoke exec -T nsqd wget -q -O - --post-data='' \
  "http://127.0.0.1:4151/channel/create?topic=$topic&channel=$channel" >/dev/null
compose_smoke exec -T nsqd wget -q -O - --post-data='persistent-message' \
  "http://127.0.0.1:4151/pub?topic=$topic" >/dev/null
compose_smoke exec -T nsqd wget -q -O - 'http://127.0.0.1:4151/stats?format=json' |
  python3 -c 'import json, os, sys; d=json.load(sys.stdin); t=next(x for x in d["topics"] if x["topic_name"]==os.environ["NSQ_TOPIC"]); c=next(x for x in t["channels"] if x["channel_name"]==os.environ["NSQ_CHANNEL"]); assert c["depth"] >= 1'
compose_smoke up -d --force-recreate nsqd
nsq_timeout=120
until [ "$nsq_timeout" -le 0 ] || [ "$(compose_smoke exec -T nsqd wget -q -O - http://127.0.0.1:4151/ping 2>/dev/null || true)" = OK ]; do
  sleep 2
  nsq_timeout=$((nsq_timeout - 2))
done
[ "$nsq_timeout" -gt 0 ]
compose_smoke exec -T nsqd wget -q -O - 'http://127.0.0.1:4151/stats?format=json' |
  python3 -c 'import json, os, sys; d=json.load(sys.stdin); t=next(x for x in d["topics"] if x["topic_name"]==os.environ["NSQ_TOPIC"]); c=next(x for x in t["channels"] if x["channel_name"]==os.environ["NSQ_CHANNEL"]); assert c["depth"] >= 1'
docker volume inspect coze-dev-smoke_nsq-data >/dev/null
```

Expected: channel depth is at least one before and after recreation, and `coze-dev-smoke_nsq-data` exists.

- [ ] **Step 9: Check logs and remove temporary credentials**

```bash
compose_smoke logs --no-color --tail=200 > "$SMOKE_DIR/service.log"
for secret in "$MYSQL_PASSWORD" "$REDIS_PASSWORD" "$ES_PASSWORD"; do
  [ -z "$secret" ] || ! grep -Fq -- "$secret" "$SMOKE_DIR/service.log"
done
compose_smoke down
docker volume inspect coze-dev-smoke_nsq-data >/dev/null
rm -f "$SMOKE_DIR/app.env" "$SMOKE_DIR/mysql.cnf" "$SMOKE_DIR/es-curl.conf" "$SMOKE_DIR/service.log"
unset MYSQL_PASSWORD MYSQL_DSN REDIS_PASSWORD REDISCLI_AUTH ES_PASSWORD NSQ_TOPIC NSQ_CHANNEL
trap - EXIT
```

Expected: no credential appears in logs; containers/network stop; the NSQ volume remains; credential files and Redis probe key are gone. Do not use `down -v`, Atlas, or any migration command.

### Task 7: Perform the First dev Integration Audit and Stop for Approval

**Files:**
- No tracked files change.

- [ ] **Step 1: Fix the remote baseline and worktree state**

```bash
git status --short --branch
git worktree list --porcelain
git branch -vv
git remote -v
git fetch origin dev
git rev-parse origin/dev
git rev-parse dev
git rev-parse HEAD
git rev-list --left-right --count dev...origin/dev
git merge-base --is-ancestor origin/dev HEAD
```

Expected: feature worktree is clean; local `dev` can fast-forward to `origin/dev`; feature branch contains the latest remote baseline. If the final command is non-zero, merge `origin/dev` into the feature branch, rerun Tasks 1 through 6, and restart the audit with the new SHA.

- [ ] **Step 2: Audit commits, files, secrets, and migration scope**

```bash
git log --oneline --decorate origin/dev..HEAD
git diff --stat origin/dev...HEAD
git diff --name-status origin/dev...HEAD
git diff --check origin/dev...HEAD
git diff --check
if git diff --name-only origin/dev...HEAD | grep -Eq '^docker/atlas/migrations/'; then exit 1; fi
rg -n 'MYSQL_PASSWORD=|REDIS_PASSWORD=|ES_PASSWORD=|MYSQL_DSN=' \
  deploy/dev backend docs/superpowers --glob '!*.example' --glob '!*.md'
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!graphify-out/**' --glob '!.git/**'
```

Expected: only planned source, test, docs, spec, and plan files appear; no migration, real credential assignment, or conflict marker is found.

- [ ] **Step 3: Re-run tests and inspect code impact**

Repeat Task 6 Step 1 at the exact clean feature SHA. Use codebase-memory `search_graph` and `trace_path` to inspect inbound callers of `redis.New`, the `appinfra.Init` startup path, and `getVectorStore`; compare graph results to the real diff because the index may predate the branch.

Expected: tests/build pass; every Redis caller is adapted; explicit vector providers are unchanged; there is no unrelated persistence or API change.

- [ ] **Step 4: Report and wait for the first explicit approval**

Report exact `origin/dev`, local `dev`, feature branch, and audited feature SHAs; commits and files; every verification result; graph impact; three-container smoke evidence; retained NSQ volume; no-migration result; and residual single-node/public-HTTP risks. Stop and request explicit approval to merge that exact SHA into local `dev`.

Do not merge local `dev`, push `origin/dev`, trigger GitHub Actions, call the Baota webhook, or perform any remote database operation in this task. After first approval, follow `docs/superpowers/runbooks/dev-integration-audit.md` for the local merge, second audit, second explicit approval, push, and Actions/ACR/webhook verification.
