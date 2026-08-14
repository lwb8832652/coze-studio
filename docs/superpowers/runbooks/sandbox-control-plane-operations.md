# Sandbox 控制面生产运维手册

本文覆盖 Sandbox Provider 控制面、运行时路由、凭据密钥、健康检查、审计和
监控的生产操作。所有操作都应先在预发布环境完成同口径验证。

## 安全边界

- Provider endpoint、认证凭据和密钥只能进入受控 Secret Manager 或写入型
  管理表单，不能写入仓库、工单、日志、监控标签或本文档。
- 服务端只信任认证上下文中的用户和空间信息；管理 API 必须继续执行系统管
  理员校验。
- 运行时路由不可用时必须 fail closed，不能回退到宿主机命令、未登记 endpoint
  或历史环境变量。
- 审计、Workbench 和前端只能展示经过裁剪的状态、原因码和关联号，不能展示
  凭据、原始 Provider 响应、执行参数或执行结果。

## 开关与职责

| 环境变量 | 默认/兼容行为 | 职责 |
| --- | --- | --- |
| `SANDBOX_CONTROL_PLANE_ENABLED` | 非 `true` 时关闭 | 启用 Provider 管理、持久化、健康检查和管理 API |
| `SANDBOX_RUNTIME_ROUTING_ENABLED` | 未设置时兼容既有行为；控制面开启后视为开启 | 独立控制 Agent、AppDev、MCP 和 CodeRunner 的 Provider 路由 |
| `SANDBOX_PROMETHEUS_METRICS_ENABLED` | `false` | 注册 Sandbox Prometheus 指标 |
| `SANDBOX_RUNNER_SESSION_ENABLED` | `false` | 启用 Runner 的 Shared AIO Core Session backend；不代表业务 consumer 已切流 |
| `APP_DEV_HOST_RUNTIME_ENABLED` | `false` | 仅用于 `APP_ENV=debug` 的本机 AppDev 调试双重开关 |

生产环境必须显式设置 `SANDBOX_RUNTIME_ROUTING_ENABLED`，不要依赖兼容默认值。
`APP_DEV_HOST_RUNTIME_ENABLED` 不得在生产或共享测试环境开启。

## Native Sandbox Runner（2C4G）

`runner-2c4g` 是额外部署 profile，不会替换既有 `local-data` profile。它启动
`nsqd`、`coze-server`、`coze-web`、`coze-sandbox-runner` 与一个
`coze-sandbox-aio`；MySQL、Redis、
Elasticsearch 和对象存储继续读取服务器本地 `app.env` 的远程配置，禁止为此 profile
额外启动数据服务容器。

启用前必须由运维准备：专用非特权 rootless Docker/Podman-compatible socket、其数值
组 ID、权限为 `600` 且归容器运行 UID `10001` 所有的 TLS 证书和私钥，以及权限为
`600` 且归部署用户所有的 `sandbox-runner.env`。部署脚本会对默认路径或
`SANDBOX_RUNNER_ENV_FILE`、`SANDBOX_RUNNER_TLS_CERT_FILE`、
`SANDBOX_RUNNER_TLS_KEY_FILE` 覆盖路径执行相同校验。环境文件至少包含 Runner 认证 token、身份验签 keyring、队列
加密 keyring 和调度配置验签 keyring；不能放入仓库、CI 变量或 `app.env`。
`SANDBOX_RUNNER_ROOTLESS_SOCKET` 必须是这个专用 socket，绝不能是
`/var/run/docker.sock`。

rootless socket 只服务既有 one-shot Runtime。Shared AIO 的生命周期不经过该 socket：
Runner 不得 create/start/stop/restart/remove/inspect `coze-sandbox-aio`，也不得把
Docker socket 或容器标识作为 Session readiness、generation 或恢复事实。

部署脚本会把执行 Runtime 镜像解析为不可变 digest，写入部署记录，并在回滚时恢复
上一个 digest。不要手工给 `SANDBOX_RUNNER_EXECUTION_IMAGE` 传 mutable tag。

当前 Runtime 镜像只包含并公开 `agent` 与 `plugin` 的受审核 Code adapter。Runner
健康检查也只声明这两个 scope，因此不能将它设为 MCP 或 AppDev 默认 Provider；这两个
scope 应继续使用其已验证的兼容 Provider。MCP stdio 需要受控依赖包和协议客户端，
AppDev 需要受签名源码快照、长驻预览端口和网关路由；在各自 adapter 经过独立实现和
验收前，禁止以 shell 或通用 Code adapter 代替。

出现 Runner 健康失败、Redis 不可用、验签失败或 rootless runtime socket 不可达时，
Runner 必须保持不可用并拒绝新执行。已运行执行可按业务取消路径终止；取消会先结束
容器再释放调度容量。复用容器前清理临时文件、短期 secret 和同 UID 残留进程。

Runner 的 `/v1/runtime-status` 与 `/v1/metrics` 均要求 Runner Bearer token；它们只
返回固定 scope 的队列/容量、容器聚合和内存水位状态，不含用户、空间、业务执行 ID、
容器 ID、endpoint 或凭据。Prometheus 应经受控采集端访问 `/v1/metrics`，不得把该
端点映射为公网匿名接口。

## Shared AIO Core Session（默认关闭）

deploy/Compose 直接拉取并运行 `ghcr.io/agent-infra/sandbox:latest`。仓库不构建派生
AIO 镜像，不固定 tag、OCI digest 或版本，也不要求 JWT、`DISABLE_*` 或容器级
CPU/内存/PID 启动参数。每次部署应记录当次解析到的 image ID 作为审计证据；image ID
不是配置锁、readiness 条件或 `runtime_generation` 来源。

启动只保留 Task 0/1 已由 official latest 真实探针确认的兼容项：
`seccomp=unconfined` 与 ARM64 上的 `OPENSSL_armcap=0`。前者会放宽容器 syscall
过滤，是已接受并必须显式记录的上游风险；它不等于 `privileged`，也不能被描述为
Session 隔离能力。`WORKSPACE=/mnt/user-data` 只固定持久工作目录，不是业务配置入口。

未锁定 `latest` 的代价是无法仅凭代码 SHA 精确重现或回退 AIO。若节点仍保留上一
image ID，运维可以按已审核记录显式选择；否则必须保持 Core disabled，不能把当前
`latest` 冒充上一版本或由 Runner 猜测性重启。AIO 首次启动或健康检查失败时，现有
server/web 与 one-shot Runner 继续启动，只有 Core Session 投影为 unavailable。

生命周期唯一归 deploy 层：顺序为 pull/up AIO、确认 raw `8080` health，再启动或更新
Runner。AIO 只在 Compose 私网 `expose: 8080`，不得配置宿主机 `ports`、host network、
privileged 或 Docker socket。Runner 只通过
`http://coze-sandbox-aio:8080` 监督 raw health、reserved sentinel 和 MySQL CAS
generation；它不能管理容器生命周期。

`/mnt/user-data` 使用保留型 named volume，`/mnt/skills` 只读挂载。服务端从可信事实
派生目录：

```text
/mnt/user-data/<space_id>/<user_id>/<thread_id>/workspace
/mnt/user-data/<space_id>/<user_id>/<thread_id>/uploads
/mnt/user-data/<space_id>/<user_id>/<thread_id>/outputs
/mnt/skills
```

业务 API 只接受逻辑 `/mnt/user-data/{workspace,uploads,outputs}` 路径；File mutation
固定 `sudo=false`。该层级是控制面的逻辑路由，不是 chroot、Unix 用户隔离或恶意命令
边界。Shared AIO 是共享 failure domain；绝不能把它描述为对抗性多租户隔离。

Phase 1 的 Core、Interactive 与 remote Host Shell 均默认关闭。Plugin 继续使用既有
one-shot Runtime；Agent、Subagent、MCP、AppDev 和 Interactive 均未迁移。Core 只能在
MySQL/Redis、raw AIO、sentinel/generation 和 workspace 全部确定后显式启用，任一状态
unknown 时 fail closed，且不回退本机 Host Shell。

Runner 直接使用 dev 环境已配置的 MySQL 与 Redis；Compose 不启动数据库容器。
`deploy.sh` 在启动 AIO 或更新服务前，用已经拉取并校验 revision 的候选 Runner 执行一次
`migration-status`。该命令从权限为 `600` 的 Runner env 读取既有 `MYSQL_DSN`，在数据库
只读事务中核验 `atlas_schema_revisions` 的 `20260814000100_sandbox_shared_aio_core` 已完整
applied，并再次核对 Shared AIO 所需表和列；它不接收或输出 DSN，不执行 apply，也不启动
数据库容器。缺表、缺列、缺 revision、description 不符、部分执行、Atlas error 或连接状态
未知都必须在任何 service `up` 前 fail closed。

只有 `publish-dev.sh` 在用户确认 exact code SHA、实际 migration 区间、credential 文件
权限与 Atlas status 后，才可执行一次 forward apply；禁止 AutoMigrate、drop、truncate
或 schema reset。

### Core E2E 与 2C4G 资源闸门

真实验证只允许在没有实时流量的隔离 deployment 运行。受控环境文件必须是当前用户
拥有的绝对路径普通文件，权限为 `0600`；它复用既有 dev MySQL、Redis、Runner 和签名
配置，不启动本地数据库容器。真实门禁必须从专用 worktree 运行。脚本会核对整个
worktree 的 tracked 和 untracked 状态为空、`HEAD` 等于配置的 exact code SHA，并确认
当前脚本已提交在该 SHA；任一条件不满足都不能开始真实验证。运行前还必须同时给出
以下精确确认：

```text
SANDBOX_AIO_CORE_E2E_ISOLATED_DEPLOYMENT=ISOLATED_TEST_DEPLOYMENT_WITH_NO_LIVE_TRAFFIC
SANDBOX_RUNTIME_SESSION_DEV_MYSQL_ROLLBACK_ONLY=ROLLBACK_ONLY_ON_EXISTING_DEV_DATABASE
SANDBOX_AIO_CORE_E2E_EXACT_CLEANUP=EXACT_PREFIX_ONLY_ON_EXISTING_DEV_DEPENDENCIES
SANDBOX_AIO_CORE_E2E_DEV_DB_FIXTURES=EXACT_COMMIT_AND_CLEANUP_ON_ISOLATED_DEV_DATABASE
```

E2E deployment ID 必须是唯一的 `aio-e2e-*` 前缀并与 Runner 已启动时的 deployment
完全一致。由于 `scheduler_settings` 中的 AIO generation 字段在一个 schema 内具有永久
singleton owner，本轮还必须使用已迁移、专供该隔离 deployment 的 dev schema，并只读
确认 `aio_runtime_deployment_id` 精确匹配；不得清空、改写或复用其他 deployment 的
generation owner。Core settings 必须由受权流程预先
启用并已被 Runner applied，测试不得临时改写全局 Session settings。

脚本只接受 fixed Compose 选中的 Runner。`COMPOSE_PROJECT_NAME`、Compose project label、
Runner 内的 deployment 和上述 E2E deployment 必须相等；
`SANDBOX_AIO_CORE_E2E_RUNNER_URL` 必须精确指向该 Runner 容器的私网 IPv4 与 `9443`
端口。Runner image 的完整 SHA256 image ID 必须有效，OCI
`org.opencontainers.image.revision` 必须精确等于 exact code SHA；外部证据同时记录完整
image ID 和 revision。选中的 Runner 与 AIO 必须只接入同一个 Compose 私网，AIO 必须有
`coze-sandbox-aio` alias，Runner upstream 必须是
`http://coze-sandbox-aio:8080`。脚本还要把 `coze-server` 绑定到同一个 Compose project，
并在资源闸门前后确认 AIO、Runner 和 `coze-server` 的固定容器、健康状态、OOM 状态和
restart count 没有漂移。

Provider、Session、operation、Redis namespace 和 workspace fixture 都要在创建时记录。
清理先尝试清空所有 Session 的经过认证的逻辑 workspace；任一 workspace 清理失败时，
清理流程不得 Destroy 任何 Session，也不得删除 Session 或 Provider 的数据库 provenance
记录。失败记录必须原样保留，供仓库内固定的 `TestAIOCoreE2ECleanupOnly` 按相同配置重试。
只有已持久化为 destroyed 且主键、deployment、provider 与本轮记录完全一致的 Session
记录才允许直接删除。Phase 1 的 Destroy 合同会保留 workspace 根，且没有受认证的物理
目录删除 API，因此测试只能清空本轮逻辑 workspace 内容；空的派生目录可能保留，不能
为追求目录消失而增加任意物理删除入口。禁止 `LIKE` 范围删除、无主键 `DELETE`、
`flushdb`、drop/truncate、schema reset、`down -v` 或 volume rm。

资源脚本取得最后一份已签名且完全 drain 的 aggregate snapshot 后，必须关闭后续签名
状态读取，再执行固定 cleanup-only。cleanup-only 按记录的数据库主键和 Redis namespace
使用 cursor scan 到 `0`，并重新确认零残留；只有这一步完成后才能写入
`CLEAN_EXACT_PREFIX_CURSOR_RESCAN_ZERO`。中断、清理失败或 cursor 未归零时必须保留
`BLOCKED_EXACT_PREFIX_CLEANUP_REQUIRED`，不能把 deployment 标为 clean。

先运行无外部依赖的合同自测，再运行真实门禁：

```bash
bash deploy/sandbox-runner/tests/aio_core_e2e_test.sh --self-test
bash deploy/sandbox-runner/tests/aio_core_e2e_test.sh
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --self-test
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --duration 2m
bash deploy/sandbox-runner/tests/aio_2c4g_soak_test.sh --duration 30m
```

Core E2E 只通过 deploy helper stop/start/recreate official AIO，并始终保留 named volume；
Runner 内部不执行 Docker lifecycle。2C4G 脚本必须在 Docker runtime 真实提供 2 CPU、
4 GiB 内存时运行，不能通过给 official AIO 临时添加 Compose 资源参数伪造同构环境。
缺受控配置、隔离确认、准确环境规格或健康依赖时，脚本返回 `BLOCKED` 且不执行压力或
共享数据变更；`BLOCKED` 不是通过证据，Core 必须继续 disabled。

真实 Core E2E 必须从运行时观察并通过以下边界，不能用单元测试代替：占满两份 Core
weight 后，同 Shell 串行操作与第三个 Session 操作进入 queued；queued cancel 达到
`canceled` 且 marker 从未执行；运行中的 File download 在 HTTP context cancel 后达到
`canceled`，随后同一 Session 仍可执行；32 MiB 文件分块写入后的 read/download 与原始
SHA256 相同；`max_bytes=32 MiB+1` 和超过 1 MiB 的原始请求均被拒绝。正常容量排队只证明
当前总 weight 边界，不证明 Redis 的 4096 queue hard limit。

`/v1/session-runtime-status` 的 Core memory state 只能是 `available`、
`below_watermark` 或 `unknown`。Core scheduler 只在 `available` 时 dequeue；低于水位或
采样失败时保留 queued，不启动新操作。2C4G 门禁使用 1536 MiB（1.5 GiB）host available
reserve。当前合同没有安全方式在共享 dev 依赖上制造真实低水位，也不允许用 4096 个
真实排队操作冲击 Redis；没有专用故障注入环境时，这两项验收必须分别记录 `BLOCKED`，
不能用正常 2C4G workload 或单元测试伪报通过。

30 分钟证据只写仓库外权限受控目录，并绑定 exact code SHA 与当次 official latest
image ID。只记录聚合 CPU、RSS、PID/fd、host available、queue、weight、Session/Shell 数
和 drain 状态，不得记录身份、命令、路径、DSN、token 或响应正文。资源判定使用三段
证据：运行前 drained baseline 的最大值、整个 workload 的显式绝对上限和 cleanup 后
drain window 相对 baseline 的固定增量上限；不使用末四个点的趋势代替这些边界。验收还
要求 1.5 GiB reserve 始终满足、Runner RSS 不超过 192 MiB、无 OOM 或 restart、
queue/weight/Session/Shell 全部 drain。结果仍只证明两个正常 Core workload 的共享运行
水位，不证明恶意命令或租户硬隔离。Shared AIO 继续是共享 failure domain。

截至 2026-08-14，本轮只完成了合同自测和本地相关回归。当前环境没有上述专用、已迁移
且 owner 匹配的 dev schema 与受控配置，Docker runtime 为 4 CPU/8 GiB；真实 Core E2E、
2 分钟和 30 分钟 2C4G 门禁均未运行，状态为 `BLOCKED`，没有通过证据。

## 首次上线顺序

1. 应用并校验 Atlas 迁移，但暂不开放 Sandbox 前端入口。
2. 在 Secret Manager 中配置凭据 keyring、active key id 和 endpoint allowlist。
3. 设置 `SANDBOX_CONTROL_PLANE_ENABLED=true`、
   `SANDBOX_RUNTIME_ROUTING_ENABLED=false`，启动后端。
4. 由系统管理员在 `/system/sandbox` 创建 Provider，并按环境实际需要覆盖
   `agent`、`appdev`、`mcp` 三类 scope。
5. 分别执行健康检查；只有健康状态正常的 Provider 才能设为对应 scope 默认
   Provider。
6. 核对默认 Provider、审计记录、凭据重包状态和监控采集，不执行真实流量。
7. 显式设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=true`，滚动重启后端。
8. 依次执行 Agent、AppDev、MCP/CodeRunner 的既有链路回归；这些回归不代表已迁移到
   Shared AIO。Phase 1 保持 Core disabled，不开放业务入口。

如果任一 scope 没有可用默认 Provider，该 scope 应保持不可用，而不是使用其
他 scope、其他租户或宿主机作为兜底。

## Keyring 初始化与轮换

keyring 的每个值必须是标准 Base64 编码的 32 字节随机密钥，key id 必须稳定
且可审计。随机值应直接在 Secret Manager 内生成；不要在终端历史、CI 输出或
聊天中生成和传递。

轮换步骤：

1. 在 Secret Manager 中新增 key id 和 32 字节随机密钥，保留所有仍被密文引
   用的旧 key。
2. 将新 key id 设为 active key id，滚动重启后端。
3. 通过 `/system/sandbox` 的写入型表单逐个重新提交 Provider 凭据；读取接口
   不会也不应返回原凭据。
4. 确认所有 Provider 的 `needs_rewrap` 均为 `false`，健康检查正常，并核对
   更新审计记录。
5. 观察至少一个完整业务周期，确认
   `coze_sandbox_credential_decrypt_failures_total` 无新增。
6. 仅在数据库中已无旧 key id 引用后，从 keyring 删除旧 key，再滚动重启。

不要先删除旧 key。当前轮换采用管理员重新提交凭据完成重包，不存在可绕过写
入权限或返回明文凭据的批量重包接口。

## Provider 日常操作

### 新增或替换

1. 先新增 Provider，不覆盖当前默认项。
2. 完成 endpoint allowlist、凭据、scope 和容量配置。
3. 执行健康检查，确认状态和原因码符合预期。
4. 设为目标 scope 默认项。
5. 执行该 scope 的最小真实任务并观察指标。
6. 稳定后再禁用旧 Provider。

### 禁用或删除

1. 如果 Provider 是默认项，先为每个受影响 scope 切换到已通过健康检查的新
   默认项。
2. 禁用 Provider，确认没有新执行被路由到该项。
3. 等待已有执行自然完成，或通过业务取消流程终止；不要直接删除运行中依赖。
4. 核对运行审计和指标后再删除。

紧急情况下先设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=false` 并滚动重启。控制
面、Provider 配置和审计数据应继续保留，便于诊断和恢复。

## 监控与告警

启用 `SANDBOX_PROMETHEUS_METRICS_ENABLED=true` 后采集：

- `coze_sandbox_provider_selections_total`
- `coze_sandbox_provider_health_checks_total`
- `coze_sandbox_provider_health_check_duration_seconds`
- `coze_sandbox_executions_total`
- `coze_sandbox_execution_duration_seconds`
- `coze_sandbox_capacity_rejections_total`
- `coze_sandbox_credential_decrypt_failures_total`

标签仅包含 Provider 类型、scope、结果、原因码和状态，不包含 Provider 名称、
用户、空间、执行 ID、endpoint 或凭据。

建议至少配置：

- 凭据解密失败在任意五分钟窗口内新增即告警。
- 默认 Provider 健康检查连续失败即告警。
- 容量拒绝持续新增、执行失败率突增或延迟分位数越过既定 SLO 时告警。
- Provider 选择失败和无可用默认项按 scope 分组告警。

阈值应依据生产基线和 SLO 调整，不把本文建议直接当作固定容量结论。

## 审计查询

- 系统管理员在 `/system/sandbox` 查询配置变更和运行审计。
- 运行事件 action 为 `runtime.execute`，结果为 `success` 或 `failure`。
- 前端只显示关联号的首八位和末四位；持久化值为域隔离摘要，不保存调用方
  原始执行 ID。
- 根据时间、scope、Provider 和结果组合定位问题，不在日志中补打原始参数或
  Provider 响应。
- 缺少认证 actor 的后台执行不会伪造系统用户审计；此类缺口应通过调用链身份
  传播修复。

## 回滚与恢复

1. 先把 Session desired Core 设置为 disabled；若控制面不可用，再设置
   `SANDBOX_RUNNER_SESSION_ENABLED=false`。随后按需要设置
   `SANDBOX_RUNTIME_ROUTING_ENABLED=false` 并滚动重启。
2. 保持 `SANDBOX_CONTROL_PLANE_ENABLED=true`，保留 Provider、默认项、健康
   状态和审计记录。
3. 保留 `sandbox_runtime_sessions`、scheduler additive columns、AIO named volume 和
   Thread workspace；禁止 `down -v`、volume rm、drop/truncate 或批量删除目录。
4. 已在 Provider 上运行的任务按 Provider 合同完成或取消；不要迁移到宿主机
   或另一 Provider 继续执行。
5. AIO `latest` 无法精确回退时保持 Core disabled，并报告本次/上次 image ID 与限制；
   Runner 不得自行 restart AIO。
6. 修复并验证健康检查、密钥、allowlist、容量和默认项。
7. 在预发布执行最小真实任务后，再重新开启运行时路由。

如果必须同时关闭控制面，应先导出合规的配置元数据和审计证据；不得导出明文
凭据。

## 发布验收

- Atlas hash 和 validate 通过。
- 三类 scope 的 Provider 健康检查和默认项符合部署清单。
- 管理员可以访问 `/system/sandbox`，普通用户收到服务端 `403`。
- Workbench Runtime Doctor 只展示裁剪后的 Provider 状态和原因码。
- Agent、AppDev、MCP/CodeRunner 最小真实任务分别通过。
- 超时、取消、容量拒绝和 Provider 不可用路径均 fail closed。
- Prometheus 指标不含高基数或敏感标签。
- 审计可按关联号追踪，页面和控制台无未处理错误。
- 页面验收使用 Codex 内置 in-app browser，并记录 URL、账号/空间、交互结果
  和控制台状态。
