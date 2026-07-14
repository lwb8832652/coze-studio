# Local Debug And Test Runbook

本手册只记录 Coze Studio 当前主线的本地调试、nuwax-ai 页面参照和验收口径。
不要写入真实生产密钥，也不要恢复已经移除的 DeerFlow 对齐流程。

## 测试账号与地址

Coze Studio 本地功能测试：

- Frontend: `http://localhost:8080`
- Backend API: `http://localhost:8888`
- Email: `840582614@qq.com`
- Password: `z8832652`

nuwax-ai 本地参照环境：

- URL: `http://localhost/`
- Email: `admin@nuwax.com`
- Password: `123456`

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

生产和共享测试环境禁止启用 `APP_DEV_HOST_RUNTIME_ENABLED`。必须配置隔离 Runner
和独立 HTTPS Preview Gateway：

```bash
APP_DEV_RUNNER_ENDPOINT=https://runner.internal.example.com
APP_DEV_RUNNER_TOKEN=replace-with-secret-manager-value
APP_DEV_PREVIEW_GATEWAY_BASE_URL=https://preview.example.com/apps
```

未配置隔离 Runner 时，AppDev 运行和构建按 fail-closed 处理，不回退到宿主机
执行。隔离 Runner 需实现 `/v1/appdev/runtimes/*` 与 `/v1/appdev/builds` 合同，并
将构建产物写入请求限定的 OSS 前缀。`APP_DEV_RUNNER_TOKEN` 只能来自部署环境的
密钥管理，不写入 tracked 文件。

只有任务明确涉及 Agent Runtime 时，才额外启用 Eino ADK 相关变量；AppDev 页面
调试不依赖这些变量。

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

- 日常开发分支为 `codex/coze-nuwax-management-mainline`。
- 不默认合并、推送或切换到 `dev`。
- 转测试前先完成代码审核，再按用户本次明确授权的目标分支和步骤执行。
- 目标分支被其他 worktree 占用时，报告占用路径，不强制 checkout。
