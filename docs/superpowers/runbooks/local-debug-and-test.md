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

`APP_DEV_RUNNER_ENDPOINT` 和 `APP_DEV_RUNNER_TOKEN` 已删除，不再作为兼容配置
或 fallback 来源。debug host runtime 仅在 `APP_ENV=debug` 且
`APP_DEV_HOST_RUNTIME_ENABLED=true` 时启用；此模式的 HTTP gateway 只允许字面
loopback IP，不接受 `localhost` 或非 loopback 地址。

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

本仓库使用 Atlas Community `v0.35.0`：

```bash
atlas version
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

不要手工编辑 `docker/atlas/migrations/atlas.sum`。

## 分支与转测试

- `dev` 是本地与远程集成分支，需求在独立 `codex/` 分支实施。
- 合入本地 `dev` 前后分别执行一次审计，并在两个阶段各获得用户明确确认。
- 完整命令、证据和停止条件见
  `docs/superpowers/runbooks/dev-integration-audit.md`。
- 目标分支被其他 worktree 占用时，报告占用路径，不强制 checkout。
