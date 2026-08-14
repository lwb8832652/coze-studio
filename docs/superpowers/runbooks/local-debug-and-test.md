# Local Debug And Test Runbook

本手册只记录 Coze Studio 当前项目的本地调试和验收口径。不要写入真实生产
密钥；历史 Nuwax/DeerFlow 环境只在任务明确要求回归追溯时按对应旧文档使用。

## 测试账号与地址

Coze Studio 本地功能测试：

- Frontend: `http://localhost:8080`
- Backend API: `http://localhost:8888`
- Email: `840582614@qq.com`
- Password: `z8832652`

页面功能与样式验收默认使用 Codex in-app browser。验收时记录具体 URL、账号、
空间、关键交互结果和控制台错误，不用 Chrome 或单纯 API 请求替代页面验收。

## Debug 环境

从 `bin` 启动后端时必须设置 `APP_ENV=debug`，否则可能加载 `bin/.env` 而不是
`bin/.env.debug`。

AppDev 本机运行时会执行项目依赖，只允许在显式 Debug 模式启用：

```bash
APP_ENV=debug \
APP_DEV_HOST_RUNTIME_ENABLED=true \
APP_DEV_LOCAL_STORE_ENABLED=true \
./opencoze -start
```

启动日志应包含：

```text
load env file: .env.debug
```

生产和共享测试环境禁止启用 `APP_DEV_HOST_RUNTIME_ENABLED`。生产 AppDev 使用
数据库中的、已启用且支持 `appdev` scope 的默认 remote provider；provider
endpoint 必须为 HTTPS，credential 必须加密保存。部署至少需要：

```bash
SANDBOX_CONTROL_PLANE_ENABLED=true
SANDBOX_CREDENTIAL_KEYS_JSON=<stable-keyring-from-secret-manager>
SANDBOX_CREDENTIAL_ACTIVE_KEY_ID=<primary-key-id>
APP_DEV_PREVIEW_GATEWAY_BASE_URL=https://preview.example.com/apps
APP_DEV_ARTIFACT_GATEWAY_BASE_URL=https://artifacts.example.com/internal
APP_DEV_PROVIDER_AUTH_TOKEN=<provider-gateway-secret>
```

Redis 和对象存储复用服务端生产基础设施配置，缺失时 grant、artifact gateway、
runtime 和 build 均 fail closed，不使用内存 fallback。可信反向代理场景按需配置
`APP_DEV_ARTIFACT_TRUSTED_PROXY_CIDRS`。上述 keyring 必须跨重启稳定，不能使用
临时随机 key；所有 credential/token 只能来自密钥管理，不能写入 tracked 文件。

### 对象存储配置

对象存储默认使用数据库控制面：

```bash
OBJECT_STORAGE_CONFIG_SOURCE=database
OBJECT_STORAGE_CREDENTIAL_KEY=<32-byte-base64-key>
```

首次启动且 `object_storage_configs` 为空时，后端会读取兼容 env 存储配置
（`STORAGE_TYPE`、`STORAGE_BUCKET`、MinIO/TOS/S3/七牛/OSS/COS/OBS 对应 AK/SK
等），加密 AK/SK 后导入为主配置。之后以数据库中的已激活配置为事实源，管理员可在
`/system/object-storage` 新增、编辑、测试、删除和激活七牛、阿里 OSS、腾讯 COS、
华为 OBS、AWS S3、MinIO、TOS 配置；页面不回显 AK/SK，编辑时只能替换密钥。

`OBJECT_STORAGE_CREDENTIAL_KEY` 必须在进程重启、滚动发布和多副本之间保持稳定，
否则数据库中已保存的对象存储密钥无法解密，启动和运行时检查会 fail closed。生产
环境应从密钥管理注入该 key，不要使用 `docker/.env.example` 的本地示例值。

激活新的主配置会先做连接测试并写入数据库；当前进程不会静默热切换已初始化的
storage client。若页面显示需重启，请重启后端实例，让 bootstrap 使用新的主配置。
只有数据库配置损坏且需要抢修时才临时设置 `OBJECT_STORAGE_CONFIG_SOURCE=env`，
该模式会绕过数据库控制面直接使用 env 配置。

`APP_DEV_RUNNER_ENDPOINT` 和 `APP_DEV_RUNNER_TOKEN` 已删除，不再作为兼容配置
或 fallback 来源。debug host runtime 仅在 `APP_ENV=debug` 且
`APP_DEV_HOST_RUNTIME_ENABLED=true` 时启用；此模式的 HTTP gateway 只允许字面
loopback IP，不接受 `localhost` 或非 loopback 地址。

### Host Shell Core Session

Host Shell 是独立的 Core Session debug adapter，不复用
`APP_DEV_HOST_RUNTIME_ENABLED`。它只有在下列三个值精确匹配时才可用：

```bash
APP_ENV=debug
SANDBOX_HOST_SHELL_SESSION_ENABLED=true
SANDBOX_HOST_SHELL_GATEWAY_ADDR=127.0.0.1:8099
```

IPv6 只接受 `[::1]:8099`；不接受 `localhost`、`0.0.0.0`、公网地址、额外空白或其他
端口。三门禁满足后，仍需在 `/system/sandbox` 为 `local_debug` Provider 配置两个
Session feature，并显式开启 desired Host Shell。remote/Core 不可用时不会 fallback
到 Host Shell，热关闭任一门禁会使后续操作 fail closed。

Host Shell 逻辑路径仍是 `/mnt/user-data/{workspace,uploads,outputs}` 与只读
`/mnt/skills`，本机物理目录由服务端按 space/user/thread 派生。该目录只是逻辑路由；
进程直接以本机开发用户执行，没有容器、网络、credential、symlink 或恶意命令隔离，
只允许可信本机 Debug。应用重启不会恢复或重放 running command，workspace 文件可保留。

只有任务明确涉及 Agent Runtime 时，才额外启用 Eino ADK 相关变量；AppDev 页面
调试不依赖这些变量。

### Sandbox 分阶段启用

- 首次启动且 `basic_config` 从未持久化时，可显式设置
  `COZE_SYSTEM_ADMIN_EMAILS=<local-test-email>` 引导系统管理员。配置一旦落库，
  数据库立即成为唯一事实源，环境变量不能覆盖已保存或已撤销的管理员权限。
- `SANDBOX_CONTROL_PLANE_ENABLED=true` 只表示 Provider 管理面可用。
- `SANDBOX_RUNTIME_ROUTING_ENABLED=false` 可在 Provider 配置、健康检查和默
  认项准备期间阻止真实任务进入 Provider；本地验收完成后再显式改为 `true`。
- 生产和共享测试环境必须显式设置运行时路由开关，不依赖兼容默认值。
- 本机 AppDev 宿主运行时仍只允许
  `APP_ENV=debug` 与 `APP_DEV_HOST_RUNTIME_ENABLED=true` 同时满足；任一条件
  不满足都必须 fail closed，不能成为远程 Provider 的回退路径。
- 需要验证监控时显式设置 `SANDBOX_PROMETHEUS_METRICS_ENABLED=true`；指标
  不得包含 endpoint、凭据、用户、空间或执行 ID。
- Provider 运维、密钥轮换、上线和回滚流程见
  `docs/superpowers/runbooks/sandbox-control-plane-operations.md`。

`runner-2c4g` profile 直接拉取 `ghcr.io/agent-infra/sandbox:latest`，只在 Compose 私网
暴露 raw `8080`。它不构建派生 AIO 镜像、不锁版本，也不启动本地 MySQL 容器；Runner
继续使用 ignored env 中的 dev MySQL/Redis 配置。缺少这些配置时应明确 blocked，不能
临时启动、清空或重建数据库。AIO image ID 只记录为本次测试证据，不参与 generation。

Core Session 默认关闭，Phase 1 不切 Agent、Subagent、Plugin、MCP、AppDev 或
Interactive 流量；Plugin 继续 one-shot。调试 AIO 失败时不得自动改用 Host Shell。

### MCP management/runtime

MCP management 与 Agent Runtime 共用显式启用开关，未配置时默认关闭：

```bash
AGENT_THREAD_MCP_RUNTIME_ENABLED=true
MCP_AES_AUTH_SECRET=<16-or-24-or-32-byte-secret-from-local-secret-store>
```

启用 MCP 时，application startup 会立即校验 `MCP_AES_AUTH_SECRET` 并构造
AES-GCM codec；缺失或不是 16、24、32 bytes 会直接启动失败。Debug 模式同样
不会生成临时或默认密钥。密钥只能放在 ignored `bin/.env.debug`、进程环境或部署
密钥管理中，不得把真实值写入 tracked 文件、日志或测试夹具。

显式设为 `AGENT_THREAD_MCP_RUNTIME_ENABLED=false`（或不设置）时，MCP 管理 API、
默认 server seed、runtime registry 与 runtime binding 全部关闭，不会形成只可读或
只可写的半可用状态。

### Journal 调试

Journal 的四个运行开关保存在 Basic Configuration，不使用环境变量替代。默认
全部关闭。本地以系统管理员身份先读取
`GET /api/admin/config/basic/get`，取得 `revision` 和完整
`journal_runtime_configuration`，再向 `POST /api/admin/config/basic/save` 提交
CAS patch。`expected_revision` 与内层 `config_revision` 必须使用同一次读取值；
遇到 `BASE_CONFIG_VERSION_CONFLICT` 时重新读取，不覆盖别人的更新。

本地只调试核心事件流时，可把 projection/UI 对同一批空间设为开启，snapshots 和
recovery 继续关闭：

```json
{
  "expected_revision": "<revision-from-get>",
  "configuration": {
    "journal_runtime_configuration": {
      "journal_projection": true,
      "journal_ui": true,
      "journal_snapshots": false,
      "checkpoint_recovery": false,
      "journal_projection_rollout_basis_points": 10000,
      "journal_ui_rollout_basis_points": 10000,
      "journal_snapshots_rollout_basis_points": 0,
      "checkpoint_recovery_rollout_basis_points": 0,
      "sse_tenant_connection_cap": 32,
      "sse_cluster_connection_cap": 4096,
      "sse_send_queue_high_watermark": 128,
      "sse_send_queue_max": 256,
      "short_request_qps": 20,
      "short_request_burst": 40,
      "lease_ttl_seconds": 90,
      "snapshot_fragment_threshold_bytes": 4194304,
      "config_revision": "<revision-from-get>"
    }
  }
}
```

用 Pro 或 Ultra 创建根 Task Run 验证 Journal。Flash 只显示结果，Thinking 只显示
公共思考摘要，二者不入组；子 Run/subagent 也不单独创建 Journal。简单直接任务
没有真实公开步骤时不展示空 Journal。当前 Terminal 和 Browser 尚缺满足生产合同
的数据源，所以不得为了本地界面效果从日志文本猜快照，也不得开启 snapshots 后
宣称五视图已具备生产条件。

需要检查 Prometheus 指标时显式设置：

```bash
AGENT_JOURNAL_PROMETHEUS_METRICS_ENABLED=true
```

需要调试 30/90 天保留清理时，再显式开启 retention worker；它依赖数据库和对象
存储，依赖缺失会阻止服务启动。变量、默认值、清理顺序和生产处置见
`docs/superpowers/runbooks/journal-operations.md`。

相关验证命令：

```bash
(cd backend && GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" \
  ./domain/agentthread/... ./application/agentthread/... \
  ./api/handler/coze/... ./infra/agentthread/...)
(cd frontend/apps/coze-studio && rushx test)
(cd frontend/apps/coze-studio && rushx lint)
(cd frontend/apps/coze-studio && rushx build)
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" \
  arigaio/atlas:0.35.0-community-alpine migrate validate --dir file:///migrations
```

页面验收覆盖直接任务、多步骤任务、失败/重试、关闭/恢复、五个固定标签、媒体和
历史 Timeline，并在桌面及移动 viewport 检查控制台。当前生产门禁和回滚顺序以
`journal-operations.md` 为准。

## 本地 Web Search Proxy

Go `net/http` 不会自动读取 macOS 系统代理。若本地 `web_search` 请求失败，可在
忽略的 `bin/.env.debug` 中配置：

```bash
export HTTP_PROXY="http://127.0.0.1:7893"
export HTTPS_PROXY="http://127.0.0.1:7893"
export NO_PROXY="localhost,127.0.0.1,::1"
export http_proxy="http://127.0.0.1:7893"
export https_proxy="http://127.0.0.1:7893"
export no_proxy="localhost,127.0.0.1,::1"
```

该配置只用于本机，生产和共享测试环境应使用各自获批的出口代理或搜索服务。

## Debug MySQL

- 正常 Debug 路径不要拉取或启动本地 MySQL 镜像。
- 使用 `docker/.env.debug`、`bin/.env.debug` 中配置的外部测试数据库。
- 数据库密码只放在 ignored env 文件，不在日志、文档或对话中输出解析后的完整
  Compose 配置。

## Atlas CLI

本仓库使用 Atlas Community `v1.2.3`，并固定不可变镜像 digest。该版本包含 MySQL
`ssl-ca` 能力；本地 hash/validate 与 `deploy/dev/publish-dev.sh` 使用同一镜像。
GitHub Actions 不运行 Atlas，也不持有 migration credential：

```bash
ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
docker run --rm "$ATLAS_IMAGE" version
docker run --rm \
  -v "$PWD/docker/atlas/migrations:/migrations" \
  "$ATLAS_IMAGE" migrate hash --dir file:///migrations
docker run --rm \
  -v "$PWD/docker/atlas/migrations:/migrations:ro" \
  "$ATLAS_IMAGE" migrate validate --dir file:///migrations
unset ATLAS_IMAGE
```

不要手工编辑 `docker/atlas/migrations/atlas.sum`。

## 分支与集成

- `dev` 是本地与远程集成分支，需求在独立 `codex/` 分支实施。
- 合入本地 `dev` 前后分别执行一次审计，并在两个阶段各获得用户明确确认。
- 完整命令、证据和停止条件见
  `docs/superpowers/runbooks/dev-integration-audit.md`。
- 目标分支被其他 worktree 占用时，报告占用路径，不强制 checkout。
