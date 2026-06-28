# AGENTS.md

本文件是 Codex 在本仓库工作的短入口。它只保留高频规则和不可踩的边界；
长上下文放到专题文档，避免每次开工都被历史细节淹没。

## 先读清单

- P0 主线任务跟踪：
  `docs/superpowers/plans/2026-06-27-deerflow-p0-launch-tracker.md`
- DeerFlow 2.x 完整能力路线：
  `docs/superpowers/plans/2026-06-18-deerflow-2x-parity-master-roadmap.md`
- DeerFlow 主线工作上下文：
  `docs/superpowers/specs/2026-06-28-deerflow-parity-working-context.md`
- Eino Agent Runtime 约束：
  `docs/superpowers/specs/2026-06-28-eino-agent-runtime-guidance.md`
- 本地调试、账号、Atlas、MySQL：
  `docs/superpowers/runbooks/local-debug-and-test.md`

做 DeerFlow 主线任务前，先更新 P0 tracker 中对应子任务为 `进行中`，写清
预期验证方式。完成一个大主线切片后，提交前必须回写状态、验证证据、遗留
问题和 P1/P2 延后项。

## 核心原则

- 主线优先：优先完全复刻 DeerFlow 2.x 可见能力，不要偏移到平台增强。
- 任务命名保持原型：使用 `新建任务`、`全部任务`、`我的任务`、`任务详情`、
  `任务记忆`，不要把主界面改叫 `对话`。
- Go 原生目标：最终使用 Go-native Agent Harness + Eino ADK，不引入 Python
  sidecar。
- Eino 优先：新增 Agent loop、重试、压缩、工具修复、动态工具搜索、Skill、
  MCP、文件系统、子智能体前，先确认 Eino ADK 是否已有原语。
- 质量第一：代码质量、租户隔离、权限、安全边界和可测试性不可妥协。
- 透明记录：关键决策、任务状态、验证命令和已知风险必须能在文档或提交中
  追溯。
- 不做 IM Channels：不新增 Telegram、Slack、Discord、飞书、钉钉、微信等
  channel 适配、设置、菜单、凭据或 worker。

## 工作流程

### 1. 任务分析

- 先确认当前分支、工作区脏文件、P0 tracker 状态和相关源码。
- 拆出依赖关系：哪些必须串行，哪些可以并行读取或验证。
- 明确本次只解决哪个主线和子线，避免“顺手”做 P1/P2 增强。
- 遇到 bug 时先找根因，再改代码；不要靠猜测叠补丁。

### 2. 并行收集

- 独立的读取、搜索、查看日志、查看测试文件可以用
  `multi_tool_use.parallel` 并行。
- 并行任务不能写同一个文件，不能互相依赖输出。
- 搜索优先用 `rg` / `rg --files`。
- 不要用一串 `echo && sed && rg` 拼成嘈杂命令；需要多路输出时用并行工具。

### 3. 实施

- 遵循现有目录、框架、命名、API client 和测试风格。
- 手工编辑文件用 `apply_patch`。
- 不做无关重构，不回滚用户改动，不强行清理无关脏文件。
- 结构化数据优先用结构化 API/解析器，不靠脆弱字符串拼接。
- 涉及前端体验时先看现有 Semi/Coze Design 用法；必要时查 Semi MCP。

### 4. 验证

- 改前端：至少跑相关 Vitest；高风险改动补 `tsc --noEmit` 或 lint。
- 改后端：跑相关 Go package 的 targeted tests；Mockey 相关测试需要按仓库
  现有方式加 `-gcflags="all=-l -N"`。
- 改迁移：用本地 Atlas `v0.35.0` 校验 hash/validate。
- 改 DeerFlow 任务体验：用同一测试用例在 DeerFlow 和 Coze 中对比浏览器可见
  行为。
- 后台单测或长命令要设置合理超时或及时轮询，避免进程长时间卡住。

### 5. 记录和提交

- 一个大的主线任务完成后提交一次代码。
- 提交前更新 tracker 和相关专题文档。
- 不提交 `.codex/config.toml`，除非用户明确要求。
- 推送到测试前，先给用户代码审核；用户确认后再把开发分支合并到 `dev`。

## 项目上下文

Coze Studio 是 React + TypeScript + Go 的 AI Agent 平台，前端由 Rush.js 管理，
后端使用 Hertz 和 DDD 风格分层。

当前集成目标是在 Coze Studio 基础上复刻 DeerFlow 2.x 的可见能力：

- 任务创建、列表、最近任务、任务详情、追问、取消、重试、流式输出；
- Agent、子智能体、Skill、MCP、工具配置和运行；
- 任务记忆、Token 用量、Artifacts、Runtime Doctor、设置面板；
- 前后端都必须是生产级边界，不接受只做 UI 壳或内存 stub。

## 常用命令

```bash
# 安装前端依赖
rush update

# 启动中间件
make middleware

# 启动后端
make server

# 启动前端
cd frontend/apps/coze-studio
npm run dev
```

```bash
# 构建
make fe
make build_server
make web

# 测试
rush test
rush lint
cd backend && go test ./...
```

常用 targeted 校验：

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tasks/__tests__/task-detail.test.tsx
npx tsc --noEmit --project tsconfig.json

cd backend
go test ./application/agentthread ./api/handler/coze ./api/router/coze -run Test -count=1
```

## 本地测试账号

Coze Studio 和本地 DeerFlow 使用同一个功能测试账号：

- Email: `840582614@qq.com`
- Password: `z8832652`

DeerFlow 参考任务详情：

- `http://localhost:2026/workspace/chats/c155a732-f475-4cf9-aa49-13fd26b29888`

本地 Coze 常用地址：

- Frontend: `http://localhost:8080`
- Backend: `http://localhost:8888`

## 分支策略

- 日常开发分支：`codex/deerflow-parity-mainline`。
- `dev` 是测试环境验证分支，不要让当前 Codex worktree 长期停在 `dev`。
- 转测试流程：开发分支完成并提交 -> 用户审核 -> 合并到 `dev` -> 推送
  `origin/dev` -> 切回开发分支。
- 如果 `dev` 已被其他 worktree 占用，报告占用路径，不要强制 checkout。

## 前端规则

- 主应用在 `frontend/apps/coze-studio`。
- UI 优先使用 `@coze-arch/coze-design` 和
  `@coze-arch/coze-design/icons`。
- 只有 Coze Design wrapper 不满足且周边已有直接用法时，才直接从
  `@douyinfe/semi-ui` 导入。
- 当前 Semi UI 版本通过 `@coze-arch/coze-design` 使用 `2.72.3`。
- 保留 Semi 自带的键盘行为、焦点、ARIA、loading、disabled、校验、空态和
  错误态。
- 页面功能要像真实产品，不做只有说明文字的占位页面。
- DeerFlow parity 页面对比时，优先关注用户能看到的任务详情、消息渲染、
  Mermaid、Artifacts、Token、思考块和追问体验。

## 后端规则

- 后端使用 Go + Hertz。
- 分层约定：
  - `domain/`：业务实体和领域服务
  - `application/`：用例编排
  - `api/`：HTTP handler 和路由
  - `infra/`：基础设施实现
  - `crossdomain/`：跨域能力
- 身份、空间、权限以服务端认证上下文为准，不能信任客户端提交的
  `user_id`、`space_id` 或 owner 字段。
- 新增持久化能力要考虑租户隔离、幂等、重试、取消、恢复和安全审计。

## Agent Runtime 规则

- Eino ADK 是执行内核，Coze 是控制面和系统记录。
- 公共 task、event、checkpoint、LangGraph、Skill、MCP、memory、token、
  artifact、guardrail 合同都由 Coze adapter 暴露。
- Eino `AgentEvent`、checkpoint bytes、Skill runtime state、provider metadata
  都视为内部合同，不能原样暴露给 Workbench API/UI。
- 生产消息基线使用 `*schema.Message`；`*schema.AgenticMessage` 保持实验态。
- Runtime 默认必须 fail closed：显式请求 `eino_adk` 但服务端策略未启用时，
  不能静默回退。

更细的 subagent、tool policy、memory、token、artifact、guardrail 规则见：

- `docs/superpowers/specs/2026-06-28-eino-agent-runtime-guidance.md`

## IDL 和 API Client

- IDL 源在 `idl/`。
- 前端生成 schema 在 `frontend/packages/arch/api-schema/src/idl/`。
- Workbench task、memory、token、artifact、Skill、MCP、runtime API 优先使用
  生成 client。
- 生成 client 可用时，不新增手写 fetch client，除非记录明确 blocker。

## 本地 Debug

- 从 `bin` 启动后端时使用 `APP_ENV=debug`，否则可能加载 `bin/.env` 而不是
  `bin/.env.debug`。
- debug MySQL 使用忽略文件中的外部测试数据库配置；默认不要拉取或启动本地
  MySQL 镜像。
- 不提交 MySQL 密码。
- 本地 Atlas 使用 Community `v0.35.0`：

```bash
atlas version
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

## 质量标准

- 遵循 SOLID、DRY、关注点分离和 YAGNI。
- 命名清晰，抽象只在能减少真实复杂度时引入。
- 注释只写关键流程、复杂边界和容易误判的原因。
- 删除无用代码；不要保留没有调用方的兼容分支。
- 覆盖边界条件、错误路径、权限路径和空态。
- 前端必须覆盖 loading、empty、error、disabled、readonly、refresh 等状态。
- 后端 API 不只测 happy path，还要测鉴权、非法输入、幂等和安全脱敏。

## 危险操作确认

以下操作前必须获得用户明确确认，除非用户已经在当前对话中明确授权了同一
范围的操作：

- 删除、移动、批量改写大量文件；
- 数据库删除、批量更新、结构变更、迁移 apply；
- 推送、合并到 `dev`、创建/更新远程分支；
- 发送敏感数据到外部服务或生产环境 API；
- 全局安装/卸载工具，或大版本升级核心依赖；
- 任何可能破坏用户未提交工作的操作。

永远不要使用 `git reset --hard` 或 `git checkout --` 回滚文件，除非用户明确
要求。

## 输出风格

- 默认使用简体中文和用户沟通。
- 先给结论，再给关键证据和下一步。
- 少用长表格；多数情况下用短段落和列表更清楚。
- 命令、路径、变量和代码标识用反引号。
- 引用本地文件时使用可点击的绝对路径链接。
- 复杂流程可以用 Mermaid；任务状态必须写清楚主线、子线和验证证据。
- 不把内部工具细节、无关日志、长 diff 全量倾倒给用户。

## 安全边界

- Workbench API/UI 不得暴露 prompt、model completion、tool arguments、
  tool results、checkpoint bytes、credentials、object URIs、raw provider
  bodies、hidden run config 或 raw audit payload。
- Artifact、memory、token、MCP、guardrail、subagent UI 只能展示已审核的
  bounded metadata。
- 发现无关脏文件时忽略；发现相关脏文件时先读懂并协同处理。
- 不要把测试账号以外的真实密钥写入 tracked 文件。
