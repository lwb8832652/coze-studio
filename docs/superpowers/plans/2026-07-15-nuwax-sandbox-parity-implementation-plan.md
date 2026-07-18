# Nuwax Sandbox Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Coze Studio 中交付与 Nuwax 沙箱管理能力等价、Go-native、可审计且默认 fail-closed 的 Sandbox Control Plane，并让 Agent/Workflow、MCP stdio、AppDev 三类运行时统一通过受控 Provider Router 执行。

**Architecture:** 新增 `domain/sandbox` 领域模型、MySQL 持久化、密钥信封加密、远程/本地调试 Provider Adapter、统一 Provider Router 和系统管理员控制面 API；系统管理新增独立“沙箱管理”二级页面。旧 `BasicConfiguration.sandbox_config` 仅作为一次性兼容导入源，不再作为主数据源。生产环境只允许经过 HTTPS、SSRF 校验、凭据脱敏、健康检查和容量控制的远程 Provider；本机执行仅在 debug 且显式开关开启时可用。

**Tech Stack:** Go 1.x、Hertz、GORM、MySQL、Atlas v0.35.0、React、TypeScript、Semi/Coze Design、Vitest、现有 AppDev/MCP/Eino ADK/CodeRunner 抽象、Codex in-app browser。

---

## 实施边界与完成定义

- [ ] 仅实现沙箱控制面及其三类既有运行时接入，不扩展支付、订阅、IM、完整 RBAC 或新的 Agent 工具体系。
- [ ] 系统管理员权限以服务端认证事实为准；前端隐藏菜单只作为体验优化。
- [ ] UI、API、日志、审计和错误中均不得返回明文凭据、完整内部 Endpoint、原始 Runner 响应、宿主机路径、命令参数、工具结果或 Provider body。
- [ ] `agent`、`mcp_stdio`、`appdev` 每个 scope 都必须显式配置默认 Provider；生产环境缺配置、Provider 禁用、健康失败或密钥不可解密时均 fail closed。
- [ ] 本地调试 Provider 仅在 `APP_ENV=debug` 且 `SANDBOX_LOCAL_DEBUG_ENABLED=true` 时可执行，其他环境即使数据库误配置也必须拒绝。
- [ ] Nuwax 对齐只在控制面 CRUD、启停、默认项、健康测试、审计、运行时生效和页面状态全部通过后宣称完成。

## 固定外部合同

### Scope 和 Provider 枚举

```go
type Scope string

const (
	ScopeAgent    Scope = "agent"
	ScopeMCPStdio Scope = "mcp_stdio"
	ScopeAppDev   Scope = "appdev"
)

type ProviderType string

const (
	ProviderTypeRemoteHTTP ProviderType = "remote_http"
	ProviderTypeLocalDebug ProviderType = "local_debug"
)
```

### 系统管理 API

```text
GET    /api/admin/sandbox/providers
POST   /api/admin/sandbox/providers
GET    /api/admin/sandbox/providers/:provider_id
PUT    /api/admin/sandbox/providers/:provider_id
DELETE /api/admin/sandbox/providers/:provider_id
PATCH  /api/admin/sandbox/providers/:provider_id/status
POST   /api/admin/sandbox/providers/:provider_id/health-check
GET    /api/admin/sandbox/defaults
PUT    /api/admin/sandbox/defaults/:scope
GET    /api/admin/sandbox/audit-events
```

### 环境变量

```text
SANDBOX_CONTROL_PLANE_ENABLED=true|false
SANDBOX_LOCAL_DEBUG_ENABLED=true|false
SANDBOX_CREDENTIAL_KEYS_JSON={"v1":"<base64-encoded-32-byte-key>"}
SANDBOX_CREDENTIAL_ACTIVE_KEY_ID=v1
SANDBOX_RUNNER_ALLOWED_HOSTS=runner.example.com,runner-backup.example.com
SANDBOX_RUNNER_ALLOWED_CIDRS=10.20.0.0/16
SANDBOX_HEALTH_TIMEOUT_SECONDS=5
SANDBOX_CAPACITY_LEASE_SECONDS=120
```

`SANDBOX_CREDENTIAL_KEYS_JSON` 支持读取旧 key id 和使用 active key 写入新密文。密文格式固定为 `v1:<key-id>:<nonce-base64>:<ciphertext-base64>`，算法为 AES-256-GCM，AAD 使用稳定的 `provider_key` 和字段名。

## 执行期代码审计校正

- [ ] `provider_key` 是不可变加密身份：创建时生成并校验，任何 update DTO、repository update 或 API 都不得修改。
- [ ] Provider、Default 和 Audit 的复合写入必须通过领域 `UnitOfWork.WithinTransaction` 进入同一个数据库事务，不能由三个 repository 各自开启事务。
- [ ] `legacy_source_hash` 使用 nullable 唯一列；普通 Provider 写 `NULL`，避免多个空字符串触发唯一键冲突。
- [ ] SQLite 只验证 repository 行为；Atlas 和真实 MySQL 迁移/事务验证是独立交付门槛，并同步 `docker/atlas/opencoze_latest_schema.hcl`。
- [ ] Sandbox key ring 复用从 MCP AES-GCM 抽出的共享 AEAD 原语，但保留 MCP 现有 envelope 完全兼容；Sandbox 另行实现 key id、AAD、rewrap 和引用检查。
- [ ] SSRF 防护从 MCP Runtime 抽到共享 Safe HTTP 包，健康和执行请求使用同一 transport；不得复制简化 URL 校验。
- [ ] 私网 Runner 必须同时命中 host allowlist 和显式 CIDR allowlist；loopback、link-local、metadata、multicast 和特殊转换地址始终拒绝。
- [ ] Redis 容量租约使用 ZSET + Lua、Redis server time 和不可猜测 lease token；不得用单 value 表达多并发。
- [ ] Redis Lua 能力扩展现有共享 `cache.Cmdable`/Redis wrapper，不创建第二个连接池；Redis 失败时生产 fail closed。
- [ ] credential fingerprint 使用带域分离的 HMAC 截断摘要，不能公开低熵凭据的裸 SHA-256。

## Task 1: 建立沙箱领域模型、校验和错误合同

**Files:**

- Create: `backend/domain/sandbox/entity.go`
- Create: `backend/domain/sandbox/policy.go`
- Create: `backend/domain/sandbox/repository.go`
- Create: `backend/domain/sandbox/errors.go`
- Create: `backend/domain/sandbox/entity_test.go`
- Create: `backend/domain/sandbox/policy_test.go`

- [ ] 先在 `entity_test.go` 写失败测试，覆盖 Provider 名称、类型、scope、状态、Endpoint hint、版本和健康状态的合法/非法组合。
- [ ] 在 `policy_test.go` 写失败测试，覆盖 timeout `1..3600` 秒、memory `64..32768` MB、CPU `0.1..64`、输出上限 `1 KB..16 MB`、网络默认拒绝、allowlist 去重和规范化。
- [ ] 运行领域测试并确认失败原因是类型和校验尚未实现，而不是测试自身编译错误。

```bash
cd backend
go test ./domain/sandbox -run 'Test(Provider|RuntimePolicy)' -count=1
```

预期：`FAIL`，缺少 `Provider`、`RuntimePolicy` 或其校验函数。

- [ ] 实现 `Provider`、`ProviderDefault`、`ProviderAuditEvent`、`RuntimePolicy`、`HealthSnapshot` 和固定枚举。
- [ ] 实现 `ValidateProviderForCreate`、`ValidateProviderForUpdate`、`ValidateRuntimePolicy` 和 `NormalizeScopes`，不允许未知枚举或空 scope。
- [ ] 实现稳定错误码，不把底层数据库或 URL 内容放入外部错误。

```go
const (
	ErrCodeInvalidInput       = "SANDBOX_INVALID_INPUT"
	ErrCodeProviderNotFound   = "SANDBOX_PROVIDER_NOT_FOUND"
	ErrCodeProviderDisabled   = "SANDBOX_PROVIDER_DISABLED"
	ErrCodeDefaultMissing     = "SANDBOX_DEFAULT_MISSING"
	ErrCodeProviderUnhealthy  = "SANDBOX_PROVIDER_UNHEALTHY"
	ErrCodeProviderInUse      = "SANDBOX_PROVIDER_IN_USE"
	ErrCodeCredentialInvalid  = "SANDBOX_CREDENTIAL_INVALID"
	ErrCodeCapacityExhausted  = "SANDBOX_CAPACITY_EXHAUSTED"
	ErrCodeExecutionForbidden = "SANDBOX_EXECUTION_FORBIDDEN"
)
```

- [ ] 再运行领域测试并确认通过。

```bash
cd backend
go test ./domain/sandbox -count=1
```

预期：`PASS`。

- [ ] 提交本任务。

```bash
git add backend/domain/sandbox
git commit -m "feat: define sandbox control plane domain"
```

## Task 2: 新增 Atlas 迁移和 MySQL 持久化模型

**Files:**

- Create: `docker/atlas/migrations/20260715000100_sandbox_control_plane.sql`
- Modify: `docker/atlas/migrations/atlas.sum`
- Modify: `docker/atlas/opencoze_latest_schema.hcl`
- Create: `backend/infra/sandbox/mysql_models.go`
- Create: `backend/infra/sandbox/mysql_convert.go`
- Create: `backend/infra/sandbox/mysql_repository.go`
- Create: `backend/infra/sandbox/mysql_audit.go`
- Create: `backend/infra/sandbox/mysql_repository_test.go`

- [ ] 在 `mysql_repository_test.go` 先写失败测试，覆盖创建、分页、乐观锁更新、软删除、scope 默认项唯一性、删除默认 Provider 被拒绝和审计事件只增不改。
- [ ] 创建三张表，字符集和时间字段遵循仓库现有 Atlas/MySQL 约定。

```sql
CREATE TABLE `sandbox_providers` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `provider_key` varchar(64) NOT NULL,
  `name` varchar(128) NOT NULL,
  `provider_type` varchar(32) NOT NULL,
  `endpoint_secret` text NULL,
  `endpoint_hint` varchar(255) NOT NULL DEFAULT '',
  `credential_secret` text NULL,
  `credential_fingerprint` varchar(32) NOT NULL DEFAULT '',
  `scopes_json` json NOT NULL,
  `policy_json` json NOT NULL,
  `max_concurrency` int unsigned NOT NULL DEFAULT 1,
  `status` varchar(32) NOT NULL,
  `health_status` varchar(32) NOT NULL,
  `last_health_capabilities_json` json NOT NULL,
  `last_health_code` varchar(64) NOT NULL DEFAULT '',
  `last_health_message` varchar(255) NOT NULL DEFAULT '',
  `last_health_latency_ms` int unsigned NOT NULL DEFAULT 0,
  `last_health_at` datetime(3) NULL,
  `legacy_source_hash` varchar(64) NULL,
  `version` bigint unsigned NOT NULL DEFAULT 1,
  `created_by` bigint unsigned NOT NULL,
  `updated_by` bigint unsigned NOT NULL,
  `created_at` datetime(3) NOT NULL,
  `updated_at` datetime(3) NOT NULL,
  `deleted_at` datetime(3) NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_sandbox_provider_key` (`provider_key`),
  UNIQUE KEY `uk_sandbox_legacy_source_hash` (`legacy_source_hash`),
  KEY `idx_sandbox_provider_status` (`status`, `deleted_at`)
);
```

- [ ] `sandbox_provider_defaults` 使用 `scope` 唯一键，包含 `provider_id`、`version`、`updated_by`、`created_at`、`updated_at`。
- [ ] `sandbox_provider_audit_events` 包含 `provider_id`、`actor_user_id`、`action`、`result`、`request_id`、受限 `metadata_json` 和 `created_at`；不建级联删除，保证删除 Provider 后审计仍保留。
- [ ] 实现 GORM PO 与领域实体的显式转换，禁止将 secret 字段序列化进日志或 API DTO。
- [ ] 通过领域 `UnitOfWork.WithinTransaction` 复用同一个 `*gorm.DB` 完成 Provider/Default mutation 和 append-only audit；禁止每个 repository 自行开启无法组合的事务。
- [ ] 更新和启停用 `WHERE id=? AND version=?` 实现乐观锁并将版本加一；零行更新后在同一事务内区分不存在与版本冲突。
- [ ] 列表默认排除软删除；删除前检查默认项引用；审计只提供 Create/List。
- [ ] Provider 列表服从领域常量 `created_at DESC, id DESC`，Audit 使用 `(created_at, event_id)` 稳定排序；JSON 列始终写入合法的 `[]`/`{}`，健康 capabilities 使用独立 JSON 列。
- [ ] 运行 targeted repository 测试。

```bash
cd backend
go test ./infra/sandbox -run TestMySQLRepository -count=1
```

预期：`PASS`。

- [ ] 更新 Atlas hash 并校验迁移。

```bash
atlas version
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

预期：Atlas 为 `v0.35.0`，hash 更新成功，validate 无错误。

- [ ] 提供由独占 `SANDBOX_MYSQL_INTEGRATION_DSN` 门控的真实 MySQL 集成测试，验证 JSON、唯一键、乐观锁、锁顺序和 nullable legacy hash；当前 Runbook 未提供安全独占 schema 时不得执行或声称通过，该证据缺口保留到 Task 16。

- [ ] 提交本任务。

```bash
git add docker/atlas/migrations backend/infra/sandbox/mysql_models.go backend/infra/sandbox/mysql_repository.go backend/infra/sandbox/mysql_repository_test.go
git commit -m "feat: persist sandbox providers and audit events"
```

## Task 3: 实现版本化凭据加密和写入后不可回显

**Files:**

- Create: `backend/infra/sandbox/credential_codec.go`
- Create: `backend/infra/sandbox/credential_codec_test.go`
- Create: `backend/pkg/secureaead/aead.go`
- Create: `backend/pkg/secureaead/aead_test.go`
- Modify: `backend/application/mcptool/catalog_mysql.go`
- Modify: `backend/application/mcptool/catalog_mysql_config_credentials_test.go`
- Create: `backend/application/sandbox/secret_projection.go`
- Create: `backend/application/sandbox/secret_projection_test.go`

- [ ] 先抽取共享 `secureaead` Seal/Open 原语，并用 MCP 回归测试证明现有 MCP envelope、固定 AAD 和历史密文兼容性不变；不得让 `infra/sandbox` 依赖 `application/mcptool`。
- [ ] 写失败测试，覆盖 AES-GCM round trip、错误 key、未知 key id、密文篡改、AAD 不匹配、旧 key 解密、新 active key 加密和日志/JSON 不含明文。
- [ ] 凭据、Endpoint 分别使用 AAD `sandbox-provider:<provider_key>:credential` 和 `sandbox-provider:<provider_key>:endpoint`。
- [ ] 实现严格环境解析：生产开启控制面但 key ring 缺失、active key 不存在、key 非 32 字节时启动失败；debug 未配置远程 Provider 时允许控制面只管理 local-debug。
- [ ] API 投影只返回 `credential_configured`、`credential_fingerprint` 和 `endpoint_hint`；创建后任何 GET 均不返回明文或密文。
- [ ] 更新 Provider 时只有 `replace_credential=true` 才读取新的 `credential`；空字符串和未传字段语义分离。
- [ ] Endpoint hint 只保留 scheme 和经脱敏的 host，不保留 path、query、userinfo。
- [ ] 提供 CAS rewrap：用旧 key 解密后以 active key 重加密，版本冲突安全重试；删除旧 key 前扫描引用并 fail closed。
- [ ] credential fingerprint 使用 active key 派生的域分离 HMAC，输出固定长度摘要，不使用明文 SHA。
- [ ] 运行 targeted tests。

```bash
cd backend
go test ./infra/sandbox ./application/sandbox -run 'Test(CredentialCodec|SecretProjection)' -count=1
```

预期：`PASS`，测试断言中固定检查示例 secret 不出现在 JSON、错误和日志 buffer。

- [ ] 提交本任务。

```bash
git add backend/infra/sandbox/credential_codec.go backend/infra/sandbox/credential_codec_test.go backend/application/sandbox/secret_projection.go backend/application/sandbox/secret_projection_test.go
git commit -m "feat: protect sandbox provider credentials"
```

## Task 4: 实现远程 Provider、SSRF 防护和本地调试 Provider

**Files:**

- Create: `backend/infra/sandbox/provider.go`
- Create: `backend/infra/sandbox/remote_provider.go`
- Create: `backend/infra/sandbox/remote_provider_test.go`
- Create: `backend/infra/sandbox/endpoint_policy.go`
- Create: `backend/infra/sandbox/endpoint_policy_test.go`
- Create: `backend/infra/sandbox/local_debug_provider.go`
- Create: `backend/infra/sandbox/local_debug_provider_test.go`
- Create: `backend/pkg/safehttp/policy.go`
- Create: `backend/pkg/safehttp/transport.go`
- Create: `backend/pkg/safehttp/special_address.go`
- Create: `backend/pkg/safehttp/transport_test.go`
- Modify: `backend/application/mcpruntime/http_transport.go`
- Modify: `backend/application/mcpruntime/special_address.go`
- Modify: `backend/application/mcpruntime/http_transport_test.go`

- [ ] 定义内部 Provider Adapter 合同，输入使用受限 metadata，输出只包含状态、退出码、截断后的 stdout/stderr 和 artifact 引用摘要。

```go
type RuntimeProvider interface {
	Health(ctx context.Context) (HealthResult, error)
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
	Cancel(ctx context.Context, executionID string) error
}
```

- [ ] 先把现有 MCP Safe HTTP policy/transport/special-address 检测无行为变化地抽到共享 `backend/pkg/safehttp`，并让 MCP 原测试全部通过。
- [ ] 写 endpoint policy 失败测试：拒绝 HTTP、userinfo、fragment、非 allowlist host、DNS 解析到 loopback/link-local/multicast/unspecified/metadata/特殊转换地址、重定向到禁区和 DNS rebinding。
- [ ] 生产只允许 HTTPS；公网地址必须命中 host allowlist，私网地址还必须同时命中 `SANDBOX_RUNNER_ALLOWED_CIDRS`。debug local provider 不经过远程 URL。
- [ ] HTTP client 使用固定 connect/header/body timeout、禁用自动重定向、限制响应体大小、每次请求重新校验目标地址；不使用系统代理访问 Provider。
- [ ] 健康检查只接受固定 schema，返回 `status`、`capabilities`、`protocol_version` 和 bounded code，不保留 raw body。
- [ ] 执行请求携带 idempotency key、request deadline、scope、policy、workload manifest；Authorization header 只在最终发送时解密生成。
- [ ] 本地调试 Provider 在构造和执行两个阶段都校验 `APP_ENV=debug` 与 `SANDBOX_LOCAL_DEBUG_ENABLED=true`。
- [ ] 本地调试执行复用已有受限 CodeRunner/进程治理原语，不新增裸 `sh -c`；输出、时间、内存、目录和网络按 RuntimePolicy 收紧。
- [ ] 运行 adapter tests。

```bash
cd backend
go test ./infra/sandbox -run 'Test(RemoteProvider|EndpointPolicy|LocalDebugProvider)' -count=1
```

预期：`PASS`；恶意 URL、重定向、超大 body 和非 debug 本地执行全部被拒绝。

- [ ] 提交本任务。

```bash
git add backend/infra/sandbox/provider.go backend/infra/sandbox/remote_provider.go backend/infra/sandbox/remote_provider_test.go backend/infra/sandbox/endpoint_policy.go backend/infra/sandbox/endpoint_policy_test.go backend/infra/sandbox/local_debug_provider.go backend/infra/sandbox/local_debug_provider_test.go
git commit -m "feat: add hardened sandbox provider adapters"
```

## Task 5: 实现统一 Provider Router 和分布式容量租约

**Files:**

- Create: `backend/application/sandbox/router.go`
- Create: `backend/application/sandbox/router_test.go`
- Create: `backend/application/sandbox/capacity.go`
- Create: `backend/infra/sandbox/redis_capacity_limiter.go`
- Create: `backend/infra/sandbox/redis_capacity_limiter_test.go`
- Modify: `backend/infra/cache/cache.go`
- Modify: `backend/infra/cache/impl/redis/redis.go`
- Modify: `backend/infra/cache/impl/redis/redis_test.go`

- [ ] 先写 Router 失败测试，覆盖内部调用缺少显式 `provider_key`、Provider 不存在/禁用/已删除/不支持 scope、健康过期、local-debug 环境不允许、容量满和凭据不可解密。
- [ ] Provider 选择规则固定为“接受内部调用显式 `provider_key` -> 校验 Provider -> 无副作用构建 Adapter 配置 -> 获取容量租约”；禁止默认 Provider 选择和自动 fallback，避免静默降级。
- [ ] 健康状态 `unknown`、`unhealthy` 或超过 5 分钟未成功检查均拒绝生产执行；管理员手动健康测试成功后才能恢复。
- [ ] 为现有共享 `cache.Cmdable` 增加窄化脚本执行能力，并由现有 Redis wrapper 实现；不得创建新的 Redis client/pool。
- [ ] `CapacityLimiter` 使用 Redis ZSET + Lua；key 为 `sandbox:capacity:<provider_key>`，member 为不可猜测 lease token，score 为 Redis server time 计算的到期时间。脚本先清理过期成员，再原子检查计数并获取租约。
- [ ] 获取、续租、释放均校验 lease token 且幂等；过期持有者不能释放后来者租约。Runner 超时、取消、panic 和请求断开必须通过 `defer` 释放；reaper 清理过期租约。
- [ ] Redis 不可用时生产 fail closed；debug local provider 可使用同进程 limiter，但必须在运行时状态中标记 `debug_only`。
- [ ] Router 返回稳定的 `SelectedProvider`，其中不含 Endpoint、凭据和密文。
- [ ] 运行 Router 和 capacity tests。

```bash
cd backend
go test ./application/sandbox ./infra/sandbox -run 'Test(ProviderRouter|RedisCapacityLimiter)' -count=1
```

预期：`PASS`；并发测试证明不超过 `max_concurrency`，异常路径无租约泄漏。

- [ ] 提交本任务。

```bash
git add backend/application/sandbox/router.go backend/application/sandbox/router_test.go backend/application/sandbox/capacity.go backend/infra/sandbox/redis_capacity_limiter.go backend/infra/sandbox/redis_capacity_limiter_test.go
git commit -m "feat: route sandbox workloads with capacity control"
```

## Task 6: 实现控制面 Application Service、健康测试和审计

**Files:**

- Create: `backend/application/sandbox/types.go`
- Create: `backend/application/sandbox/service.go`
- Create: `backend/application/sandbox/service_test.go`
- Create: `backend/application/sandbox/health.go`
- Create: `backend/application/sandbox/health_test.go`
- Create: `backend/application/sandbox/audit.go`
- Create: `backend/application/sandbox/audit_test.go`

- [ ] 先写失败测试覆盖 List/Create/Get/Update/Delete/SetStatus/SetDefault/HealthCheck/ListAuditEvents 全流程。
- [ ] Create 流程顺序固定为：校验 -> 生成 UUID provider key -> 加密 Endpoint/credential -> 事务写 Provider -> 写 sanitized audit。
- [ ] Update 使用 expected version；名称、scope、policy、容量、Endpoint 和 credential 分别记录 changed field 名称，但不记录新旧值。
- [ ] 禁用前检查活跃容量租约；仍有 workload 时返回冲突，不强杀运行中任务。
- [ ] 删除只允许 disabled、非默认且无活跃租约 Provider；使用软删除并保留审计。
- [ ] SetDefault 只允许 enabled、支持目标 scope、健康通过的 Provider；同一事务更新唯一默认项和审计。
- [ ] HealthCheck 使用真实 Adapter，持久化 bounded health snapshot；错误消息映射为稳定 code，不保存底层 body、URL 或 secret。
- [ ] 审计 metadata 采用 allowlist 字段：`scope`、`changed_fields`、`previous_status`、`new_status`、`health_code`、`version`。
- [ ] 所有 service 方法接收可信 `Actor`，但仍由 API 层的系统管理员授权器创建；不得信任 request body 中的 user id。
- [ ] 运行 application tests。

```bash
cd backend
go test ./application/sandbox -count=1
```

预期：`PASS`；测试固定检查 secret、Endpoint path、Runner body 不出现在响应和审计。

- [ ] 提交本任务。

```bash
git add backend/application/sandbox/types.go backend/application/sandbox/service.go backend/application/sandbox/service_test.go backend/application/sandbox/health.go backend/application/sandbox/health_test.go backend/application/sandbox/audit.go backend/application/sandbox/audit_test.go
git commit -m "feat: add sandbox control plane application service"
```

## Task 7: 暴露系统管理员 API 并执行服务端强鉴权

**Files:**

- Create: `backend/api/handler/coze/admin_sandbox.go`
- Create: `backend/api/handler/coze/admin_sandbox_test.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `backend/application/application.go`

- [ ] 先写 handler 失败测试，覆盖未登录 `401`、普通用户 `403`、管理员成功、非法 JSON `400`、版本冲突 `409`、不存在 `404` 和内部错误 `500` 的脱敏响应。
- [ ] 复用现有系统管理员后端授权事实，不新增客户端 role 判断作为 API 信任来源。
- [ ] 请求 DTO 对未知字段使用严格拒绝；Provider id、分页、expected version、scope 和状态均做边界校验。
- [ ] 响应 DTO 只投影公开字段；Endpoint 仅返回 `endpoint_hint`，凭据仅返回 configured/fingerprint。
- [ ] 在 `/api/admin/sandbox` 路由组注册固定合同中的全部 endpoint；保持现有 `/api/admin/*` middleware 和错误响应格式。
- [ ] 在 application wiring 注入 repository、codec、adapter factory、capacity limiter、router 和 service；开启控制面但关键依赖缺失时启动失败，不退回旧 sandbox config。
- [ ] 运行 handler/router tests。

```bash
cd backend
go test ./api/handler/coze ./api/router/coze -run 'TestAdminSandbox' -count=1
```

预期：`PASS`；普通用户无法通过直接请求访问 API。

- [ ] 提交本任务。

```bash
git add backend/api/handler/coze/admin_sandbox.go backend/api/handler/coze/admin_sandbox_test.go backend/api/router/coze/api.go backend/application/application.go
git commit -m "feat: expose secured sandbox admin api"
```

## Task 8: 一次性导入旧 SandboxConfig，并将旧配置降为只读兼容源

**Files:**

- Create: `backend/application/sandbox/legacy_importer.go`
- Create: `backend/application/sandbox/legacy_importer_test.go`
- Modify: `backend/bizpkg/config/base/base.go`
- Modify: `backend/application/base/appinfra/app_infra.go`
- Modify: `backend/infra/coderunner/impl/impl.go`
- Modify: `frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx`

- [ ] 先写 importer 失败测试：无旧配置不导入、相同 hash 幂等、不同旧配置创建新 import 记录、生产不把 local-debug 设为默认、debug 可显式设默认。
- [ ] 将旧 `SandboxConfig` 映射到 `RuntimePolicy`：allow env/read/write/run/net/ffi、node modules、timeout、memory；生成不含 secret 的 SHA-256 source hash。
- [ ] 仅当新表为空时执行自动导入；导入 Provider 名称固定为“Legacy local sandbox”，类型 `local_debug`，状态由 debug 双开关决定。
- [ ] 生产环境导入记录保持 disabled 且不设默认项，确保 fail closed。
- [ ] `base.go` 保留读取旧字段供 importer 使用；停止让新的运行时 wiring 直接从 `BasicConfiguration.sandbox_config` 构造主 Runner。
- [ ] 修正 `impl.go` 中将配置值误当环境变量名读取的行为：配置合同改为直接解析逗号分隔值；若确需环境变量引用，只允许显式 `env:VARIABLE_NAME` 语法并限制变量名格式。
- [ ] 系统设置旧“代码运行器/SandboxConfig”表单改为只读迁移提示，并链接到 `/system/sandbox`；不再允许形成两个可写主数据源。
- [ ] 运行兼容和 coderunner tests。

```bash
cd backend
go test ./application/sandbox ./infra/coderunner/impl ./application/base/appinfra -run 'Test(Legacy|CodeRunner|AppInfra)' -count=1
```

预期：`PASS`；重复启动不会重复导入，生产不会启用 host/local 执行。

- [ ] 提交本任务。

```bash
git add backend/application/sandbox/legacy_importer.go backend/application/sandbox/legacy_importer_test.go backend/bizpkg/config/base/base.go backend/application/base/appinfra/app_infra.go backend/infra/coderunner/impl/impl.go frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx
git commit -m "refactor: migrate legacy sandbox configuration"
```

## Task 9: 将 AppDev 运行时接入 Provider Router

**Files:**

- Create: `backend/infra/appdev/sandbox_runtime_manager.go`
- Create: `backend/infra/appdev/sandbox_runtime_manager_test.go`
- Modify: `backend/infra/appdev/runtime_factory.go`
- Modify: `backend/infra/appdev/runtime_factory_test.go`
- Modify: `backend/infra/appdev/runtime_manager_security_test.go`
- Modify: `backend/application/appdev/service.go`

- [ ] 先写失败测试，证明 AppDev create/start/stop/preview 均请求 `ScopeAppDev`，没有默认 Provider 时返回稳定 unavailable，而不是 host fallback。
- [ ] `sandbox_runtime_manager.go` 将 AppDev manifest、文件快照引用、端口、超时和资源限制转换成 bounded `ExecuteRequest`；不传数据库凭据或宿主机绝对路径。
- [ ] 将 Provider 返回的 execution id 映射为 AppDev runtime id，将 preview capability 映射到已有 HTTPS preview gateway；拒绝 Runner 直接返回的任意 preview URL。
- [ ] `runtime_factory.go` 在控制面开启时只构造 Router-backed manager；原 `APP_DEV_RUNNER_ENDPOINT` 仅作为 legacy importer 的远程 Provider 创建来源，不再绕过控制面。
- [ ] 保留原 host manager 但只供 `ProviderTypeLocalDebug` Adapter 内部使用，继续执行 debug 双开关。
- [ ] 请求取消、SSE 断开和 stop 操作释放容量租约并调用 Provider Cancel；重试复用 idempotency key。
- [ ] 运行 AppDev tests。

```bash
cd backend
go test ./infra/appdev ./application/appdev -run 'Test(SandboxRuntimeManager|ConfiguredRuntimeManager|RuntimeManagerSecurity|AppDev)' -count=1
```

预期：`PASS`；生产配置缺失和恶意 preview URL 均 fail closed。

- [ ] 提交本任务。

```bash
git add backend/infra/appdev/sandbox_runtime_manager.go backend/infra/appdev/sandbox_runtime_manager_test.go backend/infra/appdev/runtime_factory.go backend/infra/appdev/runtime_factory_test.go backend/infra/appdev/runtime_manager_security_test.go backend/application/appdev/service.go
git commit -m "feat: run appdev through sandbox providers"
```

## Task 10: 将 MCP stdio 执行接入 Provider Router

**Files:**

- Modify: `backend/application/agentthread/adk_mcp_stdio_sandbox.go`
- Modify: `backend/application/agentthread/adk_mcp_stdio_sandbox_test.go`
- Modify: `backend/application/agentthread/adk_mcp_stdio_transport.go`
- Modify: `backend/application/agentthread/adk_mcp_runtime_bootstrap.go`
- Modify: `backend/application/agentthread/adk_mcp_runtime_bootstrap_test.go`
- Modify: `backend/application/mcpruntime/stdio_transport.go`
- Modify: `backend/application/mcpruntime/production_runner_test.go`

- [ ] 先写失败测试，证明 ADK MCP 和管理态 MCP 两条 stdio 路径都必须请求 `ScopeMCPStdio`，且 remote HTTP/SSE MCP 不误走沙箱。
- [ ] 将 command、args、允许的环境变量名、workdir projection、输入 frame 和 timeout 转换为受限 MCP workload；继续使用现有 command policy、safe workdir 和 lease 校验。
- [ ] Provider 请求中不发送宿主机 workdir；只发送逻辑 workspace id 和已审计 artifact/file manifest。
- [ ] 将 Provider stdout 按现有 MCP frame parser 解析；超限、非 JSON-RPC、协议污染和 stderr secret 均沿现有 bounded error 路径处理。
- [ ] dry-run 仍不执行 Provider；production runner 不再启动本机 stdio process。
- [ ] Provider 选择、健康码和 execution id 的截断摘要写入现有 MCP runtime audit，禁止写 Endpoint、credential、完整 command args 和 tool results。
- [ ] 运行 MCP targeted tests，Mockey 用仓库要求的编译参数。

```bash
cd backend
go test -gcflags="all=-l -N" ./application/agentthread ./application/mcpruntime -run 'Test(ADKMCPRuntimeStdio|ADKMCPRuntimeBootstrap|ProductionRunner)' -count=1
```

预期：`PASS`；production 测试证明不会创建宿主机 stdio 子进程。

- [ ] 提交本任务。

```bash
git add backend/application/agentthread/adk_mcp_stdio_sandbox.go backend/application/agentthread/adk_mcp_stdio_sandbox_test.go backend/application/agentthread/adk_mcp_stdio_transport.go backend/application/agentthread/adk_mcp_runtime_bootstrap.go backend/application/agentthread/adk_mcp_runtime_bootstrap_test.go backend/application/mcpruntime/stdio_transport.go backend/application/mcpruntime/production_runner_test.go
git commit -m "feat: isolate mcp stdio with sandbox providers"
```

## Task 11: 将 Agent/Workflow CodeRunner 接入 Provider Router

**Files:**

- Create: `backend/infra/coderunner/impl/controlplane/runner.go`
- Create: `backend/infra/coderunner/impl/controlplane/runner_test.go`
- Modify: `backend/infra/coderunner/code.go`
- Modify: `backend/infra/coderunner/impl/impl.go`
- Modify: `backend/application/base/appinfra/app_infra.go`
- Modify: `backend/domain/workflow/internal/nodes/code/code.go`
- Modify: `backend/domain/workflow/internal/nodes/code/code_test.go`

- [ ] 先写失败测试，证明 Workflow code node 和 Agent 可复用 CodeRunner 均请求 `ScopeAgent`，控制面开启后不能落到 direct runner。
- [ ] 新 control-plane Runner 将 language、code artifact、声明式输入、超时和 RuntimePolicy 转为 bounded request；不得发送用户 prompt、完整 run config 或无关上下文。
- [ ] 输出按现有 `coderunner.Runner` 合同截断和映射；Provider body、内部 execution metadata、artifact object URI 不进入 Workbench API。
- [ ] `impl.New` 在 `SANDBOX_CONTROL_PLANE_ENABLED=true` 时要求注入 Router-backed runner；旧 direct/sandbox runner 只作为 local-debug adapter 内部实现。
- [ ] 保持 Workflow 节点业务合同不变，补充 Provider unavailable、timeout、cancel、output limit 和 capacity exhausted 的映射测试。
- [ ] 运行 CodeRunner 和 Workflow targeted tests。

```bash
cd backend
go test ./infra/coderunner/... ./domain/workflow/internal/nodes/code -run 'Test(ControlPlaneRunner|CodeNode)' -count=1
```

预期：`PASS`；控制面开启的生产测试无法观察到 direct runner 调用。

- [ ] 提交本任务。

```bash
git add backend/infra/coderunner/impl/controlplane backend/infra/coderunner/code.go backend/infra/coderunner/impl/impl.go backend/application/base/appinfra/app_infra.go backend/domain/workflow/internal/nodes/code/code.go backend/domain/workflow/internal/nodes/code/code_test.go
git commit -m "feat: route agent code execution through sandbox"
```

## Task 12: 新增前端 Sandbox Service、类型和页面状态模型

**Files:**

- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-service.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-view-model.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-service.test.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-view-model.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/content.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/view-model.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/index.tsx`

- [ ] 先写失败测试，覆盖 DTO 解析、分页、错误映射、scope 标签、健康状态、可操作权限和 secret 不可回显。
- [ ] 定义显式 TypeScript DTO；禁止 `unknown`、`any` 和直接复用旧 `sandbox_config?: unknown`。
- [ ] `sandbox-service.ts` 只调用固定 `/api/admin/sandbox/*` 合同，统一携带现有认证和 CSRF 机制，支持 AbortSignal，错误只暴露稳定 code/message。
- [ ] `sandbox-view-model.ts` 提供 loading/empty/error/refresh、分页、筛选、健康检查中、提交中、版本冲突和只读状态；所有 mutation 完成后精确刷新受影响数据。
- [ ] 在系统管理导航加入 `/system/sandbox` 二级项“沙箱管理”，放在“系统配置”相邻位置；非系统管理员仍由已有系统页守卫阻止访问。
- [ ] `index.tsx` 通过 route content map 渲染新 section，不引入新的一级菜单。
- [ ] 运行 frontend model tests。

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/sandbox-service.test.ts src/pages/system/__tests__/sandbox-view-model.test.ts
```

预期：`PASS`。

- [ ] 提交本任务。

```bash
git add frontend/apps/coze-studio/src/pages/system/sandbox-service.ts frontend/apps/coze-studio/src/pages/system/sandbox-view-model.ts frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-service.test.ts frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-view-model.test.ts frontend/apps/coze-studio/src/pages/system/content.ts frontend/apps/coze-studio/src/pages/system/view-model.ts frontend/apps/coze-studio/src/pages/system/index.tsx
git commit -m "feat: add sandbox admin frontend model"
```

## Task 13: 实现与 Nuwax 能力等价的沙箱管理页面

**Files:**

- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-management-section.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-provider-form.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-provider-detail.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-audit-drawer.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/sandbox-management-section.module.less`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-management-section.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-provider-form.test.tsx`

- [ ] 先写失败组件测试，覆盖列表、空态、错误态、创建、编辑、credential replacement、启停、删除、设默认、健康测试、审计抽屉和版本冲突刷新。
- [ ] 页面头部展示 Provider 总数、已启用数、异常数和三个 scope 默认项；数据来自真实 API，不使用静态演示值。
- [ ] 列表列固定为名称、类型、适用范围、Endpoint hint、默认范围、健康状态、容量、更新时间和操作。
- [ ] 创建/编辑抽屉使用 Coze Design/Semi Form，包含名称、类型、Endpoint、凭据、scope、资源策略、网络 allowlist、容量；切换 local-debug 时显示双开关限制说明。
- [ ] 编辑页不回填 secret；显示 fingerprint，并通过“替换凭据”显式开启输入框。取消替换不提交 credential 字段。
- [ ] 启用前要求健康检查通过；设默认前再次显示 scope 和 Provider；禁用/删除使用风险确认且展示被默认项或活跃 workload 阻塞的原因。
- [ ] 健康状态包含 unknown/checking/healthy/degraded/unhealthy，展示上次时间、latency 和 bounded message；不展示 raw response。
- [ ] 审计抽屉支持 provider/action/result 筛选和分页，只展示 sanitized metadata。
- [ ] 完整覆盖键盘焦点、ARIA、loading、disabled、readonly、表单校验、窄屏抽屉和 1280px 桌面布局；遵循现有系统管理视觉语言，不复制 Nuwax 框架代码。
- [ ] 运行组件测试和 TypeScript 校验。

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/sandbox-management-section.test.tsx src/pages/system/__tests__/sandbox-provider-form.test.tsx
npx tsc --noEmit --project tsconfig.json
```

预期：测试和类型检查均通过。

- [ ] 提交本任务。

```bash
git add frontend/apps/coze-studio/src/pages/system/sandbox-management-section.tsx frontend/apps/coze-studio/src/pages/system/sandbox-provider-form.tsx frontend/apps/coze-studio/src/pages/system/sandbox-provider-detail.tsx frontend/apps/coze-studio/src/pages/system/sandbox-audit-drawer.tsx frontend/apps/coze-studio/src/pages/system/sandbox-management-section.module.less frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-management-section.test.tsx frontend/apps/coze-studio/src/pages/system/__tests__/sandbox-provider-form.test.tsx
git commit -m "feat: build sandbox management experience"
```

## Task 14: 扩展 Runtime Doctor、指标和安全审计

**Files:**

- Modify: `backend/application/agentthread/runtime_doctor.go`
- Modify: `backend/application/agentthread/runtime_doctor_test.go`
- Create: `backend/application/sandbox/metrics.go`
- Create: `backend/application/sandbox/metrics_test.go`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-runtime-doctor-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/__tests__/task-runtime-doctor-section.test.tsx`

- [x] 先写失败测试，覆盖 Doctor 只返回 Provider key 摘要、type、scope、health code、selected/default 状态和 checked_at，不返回 URL、credential、policy path、command 或 raw body。
- [x] Runtime Doctor 为每个 scope 输出 `configured`、`available`、`health_status`、`reason_code`；普通用户只看到当前任务实际使用 scope 的 bounded 信息。
- [x] 新增 Prometheus 指标：provider selection、health check、execution count/duration/result、capacity rejection、credential decrypt failure；label 只允许 provider type/scope/result code，不使用 provider name、user id 或 execution id。
- [x] 控制面审计与运行时审计通过 request/execution correlation id 关联，但 UI 只展示截断 id。
- [x] 前端 Doctor 添加“沙箱运行环境”卡片和恢复建议；管理员链接到 `/system/sandbox`，普通用户只看到联系管理员提示。
- [x] 运行 targeted tests。

```bash
cd backend
go test ./application/agentthread ./application/sandbox -run 'Test(RuntimeDoctor|SandboxMetrics)' -count=1
cd ../frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-runtime-doctor-section.test.tsx
```

预期：`PASS`；敏感字符串 fixture 不出现在 Doctor JSON、指标 label 或审计投影。

- [ ] 提交本任务。

```bash
git add backend/application/agentthread/runtime_doctor.go backend/application/agentthread/runtime_doctor_test.go backend/application/sandbox/metrics.go backend/application/sandbox/metrics_test.go frontend/apps/coze-studio/src/pages/tasks/task-runtime-doctor-section.tsx frontend/apps/coze-studio/src/pages/tasks/__tests__/task-runtime-doctor-section.test.tsx
git commit -m "feat: diagnose and observe sandbox providers"
```

## Task 15: 编写运维 Runbook 和升级/回滚规则

**Files:**

- Create: `docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Modify: `docs/superpowers/runbooks/local-debug-and-test.md`
- Modify: `docs/superpowers/specs/2026-07-15-nuwax-sandbox-parity-design.md`
- Modify: `AGENTS.md`

- [x] Runbook 写明 key ring 生成、active key 轮换、Provider 添加顺序、健康检查、默认 scope 切换、禁用/删除、容量告警和审计查询。
- [x] 写明升级顺序：先迁移 -> 配 key ring/allowlist -> 开启管理面但保持运行路由关闭 -> 创建并验证 Provider -> 设置三类默认项 -> 开启运行路由 -> 部署前端。
- [x] 写明回滚边界：可以关闭新流量入口，但生产不得自动回落 host execution；运行中 workload 等待结束或显式取消，保留 Provider/audit 数据。
- [x] 写明 local-debug 仅供本机开发，包含两个必需开关和验证命令；不记录真实 key、token 或内部 Endpoint。
- [x] 在设计文档追加实现状态矩阵，逐项链接 test/browser evidence；只有验收后才将状态改为 complete。
- [x] 精简更新 `AGENTS.md` 高频规则：系统沙箱管理入口、三 scope、生产 fail-closed、内置浏览器验收；长配置仍留在 Runbook。
- [ ] 提交本任务。

```bash
git add docs/superpowers/runbooks/sandbox-control-plane-operations.md docs/superpowers/runbooks/local-debug-and-test.md docs/superpowers/specs/2026-07-15-nuwax-sandbox-parity-design.md AGENTS.md
git commit -m "docs: document sandbox control plane operations"
```

## Task 16: 执行生产级总验收

**Files:**

- Modify: `docs/superpowers/specs/2026-07-15-nuwax-sandbox-parity-design.md`
- Modify: `docs/superpowers/plans/2026-07-15-nuwax-sandbox-parity-implementation-plan.md`

- [ ] 运行完整 Sandbox、AppDev、MCP stdio、CodeRunner、管理员 API targeted tests。

```bash
cd backend
go test ./domain/sandbox ./infra/sandbox ./application/sandbox ./infra/appdev ./application/appdev ./infra/coderunner/... ./domain/workflow/internal/nodes/code ./api/handler/coze ./api/router/coze -count=1
go test -gcflags="all=-l -N" ./application/agentthread ./application/mcpruntime -count=1
```

预期：全部 `PASS`。

- [ ] 运行前端系统管理和 Runtime Doctor tests、类型检查。

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__ src/pages/tasks/__tests__/task-runtime-doctor-section.test.tsx
npx tsc --noEmit --project tsconfig.json
```

预期：全部通过，无 unresolved module/type error。

- [ ] 重新校验 Atlas。

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

预期：hash 无意外变化，validate 通过。

- [ ] 用 Codex in-app browser 登录 Nuwax `http://localhost/`，记录沙箱配置列表、创建/编辑、启停、健康检查和反馈状态，作为功能参照而非源码复制依据。
- [ ] 用 Codex in-app browser 登录 Coze `http://localhost:8080/system/sandbox`，以系统管理员验收列表、创建远程 Provider、credential 不回显、健康检查、三 scope 默认项、启停、删除阻塞、审计抽屉和刷新恢复。
- [ ] 以普通用户直接访问 `/system/sandbox` 并调用 `/api/admin/sandbox/providers`，确认页面不可见且 API 返回 `403`。
- [ ] 在 debug 双开关开启时验收 local-debug；关闭任一开关后确认执行 fail closed。
- [ ] 对 `agent`、`mcp_stdio`、`appdev` 各执行一个最小真实 workload，确认选中正确 Provider、产生受限审计、容量释放且 Runtime Doctor 状态一致。
- [ ] 禁用默认 Provider、模拟 unhealthy、移除 Redis、使用错误 key 和触发超时，确认所有生产路径均 fail closed 且错误不泄密。
- [ ] 浏览器验收记录必须包含 URL、账号类型、空间、操作、可见结果、网络响应码和控制台错误；控制台需无新增 error。
- [ ] 对照 Nuwax 与 Coze 的验收矩阵，只把“能力等价且符合 Coze 安全边界”的条目标记为通过；视觉差异仅允许来自 Coze 既有设计系统。
- [ ] 更新设计和计划文档中的证据、已知限制和最终状态；不得用“代码已写”代替运行时验收。
- [ ] 审核最终 diff，确认未提交 `.codex/config.toml`、真实 secret、内部 Endpoint、`.codegraph/` 或 `coze-studio.wiki/`。
- [ ] 提交验收记录，但不自动推送或合并。

```bash
git add docs/superpowers/specs/2026-07-15-nuwax-sandbox-parity-design.md docs/superpowers/plans/2026-07-15-nuwax-sandbox-parity-implementation-plan.md
git commit -m "test: record sandbox parity acceptance"
```

## 最终交付门槛

- [ ] 三个 scope 都通过真实 Provider workload 验收。
- [ ] 管理员页面和 API 的 CRUD、启停、默认项、健康、审计形成完整闭环。
- [ ] 普通用户无法访问系统管理 API，服务端权限测试通过。
- [ ] 生产环境没有任何 host/local silent fallback。
- [ ] secret、Endpoint path、Provider body、命令参数和内部路径未出现在 API、UI、日志、指标或审计。
- [ ] 迁移可验证、legacy importer 幂等、key rotation 可操作。
- [ ] targeted tests、TypeScript、Atlas 和 in-app browser 验收全部有可追溯证据。
- [ ] 完成代码审核并获得用户对推送/合并范围的明确授权后，才执行远程操作。
