# AGENTS.md

本文件是 Codex 在本仓库工作的短入口，只保留高频流程、事实入口和不可破坏的
边界。项目已经进入 Coze 原生能力的内部优化阶段；历史 Nuwax、DeerFlow 和
阶段性迁移文档只用于按需追溯，不再作为默认产品基线。

主应用由 React + TypeScript/Rush.js 前端和 Go + Hertz 后端组成，Agent 执行
使用 Go-native Harness 与 Eino ADK，公共合同以 IDL、迁移和服务端事实为准。

## 开工顺序

1. 阅读 `docs/superpowers/context/project-context.md`，确认当前产品和架构边界。
2. 检查当前分支、工作区、worktree、远程跟踪关系和用户已有改动。
3. 用 codebase-memory 查询相关模块、调用链和预期影响，再核对真实源码。
4. 只读取与任务直接相关的 runbook、spec 或 plan，不批量加载历史资料。
5. 明确任务范围、验收方式和是否会改变长期项目事实。

常用入口：

- WorkbenchChat 当前事实：`docs/superpowers/context/workbench-chat.md`
- Workbench 当前执行链：
  `docs/superpowers/context/workbench-execution-chain.md`
- Workbench 执行图合同：
  `docs/superpowers/context/workbench-execution-graph.json`
- Workbench 图谱运维：
  `docs/superpowers/runbooks/workbench-execution-graph.md`
- 本地调试与账号：`docs/superpowers/runbooks/local-debug-and-test.md`
- `dev` 集成审计：`docs/superpowers/runbooks/dev-integration-audit.md`
- Sandbox 运维：`docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Agent Runtime：`docs/superpowers/specs/2026-06-28-eino-agent-runtime-guidance.md`

## 事实优先级

发生冲突时按以下顺序判断：

1. 当前源码、IDL、数据库迁移和运行时行为；
2. 相关测试和生成代码；
3. `project-context.md` 与当前有效 runbook/spec；
4. codebase-memory、Graphify 等派生图谱；
5. 历史 plans/specs 和外部参考项目。

图谱只用于导航，不能替代源码和运行时证据。历史 Nuwax/DeerFlow 文档只有任务
明确要求迁移追溯、回归定位或行为来源时才读取。

## 代码库分析工具

### codebase-memory

- 开工先检查索引和项目是否匹配当前仓库。
- 用 `get_architecture`、`search_graph`、`trace_path` 分析结构和调用链。
- 修改前检查调用方、被调用方、跨层依赖和共享合同。
- 修改后用 `detect_changes` 检查影响面，再回到真实 diff 和源码核实。
- 索引过期、缺失或与源码冲突时刷新索引；无法刷新则改用 `rg` 和直接读取。

### Graphify

- 只用于精选长期上下文、架构决策、关键 runbook 和活跃设计的关系查询。
- Workbench 派生目录存在时优先查询，不重复全量构建：业务意图检索使用
  `graphify-out/query-graph.json`，执行路径和边方向审计使用
  `graphify-out/graph.json`。该稳定目录是指向已完整校验版本的原子指针，不要
  绕过指针直接读取 `workbench-execution-graphify-versions/`；上一有效版本由
  `workbench-execution-graphify-previous` 指向，仅用于本地故障回滚。
- 非 Workbench 的普通局部修复不制造图谱噪声；命中 Workbench 合同
  `monitored_paths` 的新增、修改、重命名或删除必须同步两份权威文件并重建。
- 不默认对整个 monorepo 建图，不提交 `graphify-out/` 产物。
- 图谱缺失、过期或不可用时直接读取源文档，并记录缺失的图谱验证。
- 图谱发布只能使用 `docs/superpowers/context` 下的受管目录；不得把
  `--derived-root` 指向业务目录、仓库外目录或没有受管标记的现有目录。
- Workbench 完整问题必须在 `query-graph.json` 命中 `query_overlay` 所需节点；
  所需有向业务边和执行路径必须在无 overlay 的 `graph.json` 独立核验，并能经
  `anchored_in` 到达真实 AST 文件节点。每个 current 合同节点都必须具备该桥；
  同名文本、`retrieves` 检索边或无方向连通路径不能作为执行证据。

涉及 WorkbenchChat、任务列表、任务详情或 Agent 运行链时，先读
`docs/superpowers/context/workbench-execution-chain.md`，再按
`docs/superpowers/context/workbench-execution-graph.json` 查询当前链和源码证据；
同时以 `workbench-chat.md` 与当前源码核实现状。任何节点、关系、顺序、框架或
边界变化都要同步更新两份执行链权威文件，并运行：

```bash
node scripts/workbench-execution-graph.mjs verify --changed-from origin/dev
node scripts/workbench-execution-graph.mjs build
node scripts/workbench-execution-graph.mjs verify-derived
```

详细维护和单次 dev 图谱集成检查见
`docs/superpowers/runbooks/workbench-execution-graph.md`。K2 和历史 ChatTask
plans/specs 只能按需追溯，不能用于推断当前合同。

## 工作流程

### 分析

- Bug 先定位根因，再修改；不靠猜测叠补丁。
- 明确依赖关系、风险、权限边界和验证矩阵。
- 独立读取可并行；相互依赖或写同一文件的任务必须串行。
- 搜索优先使用图谱结构查询和 `rg` / `rg --files`。

### 实施

- 所有需求在独立 `codex/` 分支实施，不直接在 `dev` 上开发。
- 遵循现有目录、框架、命名、API client 和测试风格。
- 手工编辑使用 `apply_patch`；结构化数据使用结构化解析器。
- 不做无关重构，不回滚用户改动，不清理无关脏文件。
- 生成 client 可用时不新增手写 HTTP client。

### 验证

- 前端至少运行相关 Vitest；高风险改动补 typecheck、lint 和构建。
- 后端运行相关 Go package 测试；Mockey 测试按仓库方式加
  `-gcflags="all=-l -N"`。
- 迁移使用 Atlas Community `v1.2.3` 校验 hash 和 validate；Docker 命令固定为
  `arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e`。
- 页面验收默认使用 Codex in-app browser，记录 URL、账号/空间、可见状态、
  核心交互和控制台错误。
- 结论必须基于本轮新鲜命令输出；未运行的验证必须明确说明。

### 长期上下文更新

只有以下内容变化时更新 `project-context.md` 或对应 ADR/runbook：

- 跨模块架构和所有权边界；
- 公共 API、IDL、持久化或安全合同；
- 生产运行方式、故障恢复或发布流程；
- 后续任务必须持续遵守的工程决策。

一次性任务状态、临时日志和局部实现细节保留在任务 plan、提交或验证证据中。

## dev 自动发布

默认发布流程保持简单：需求分支完成必要测试和一次集成检查、获得用户对合并范围的
明确确认并 fast-forward 合入本地 `dev` 后，不再重复第二轮审计或测试；发布确认后
只执行一次本地发布脚本。不要求手动
登录 ACR、拉取镜像、读取镜像 revision、操作
GitHub 页面或调用宝塔接口。脚本自身会校验当前分支、工作区、exact SHA、远程竞态
和 Atlas 状态；这些是脚本内部的 fail-closed 检查，不需要用户额外操作。

在本地 `dev` 工作区执行：

```bash
AUDITED_ORIGIN_DEV_SHA=$(git rev-parse origin/dev)
AUDITED_TARGET_DEV_SHA=$(git rev-parse dev)
deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
```

脚本成功后停止本地流程。GitHub Actions 负责 ACR 登录、构建并推送
`coze-server`/`coze-web` 镜像、校验镜像、晋级 `dev` 标签并调用宝塔 WebHook；服务器
负责拉取镜像、启动 OceanBase/NSQ 和应用健康检查。不要直接执行 `git push origin dev`
或手动重复这些远程步骤，也不要把 ACR 密码写入仓库或脚本。

合并、迁移 apply、推送和远程分支操作仍需用户对当前 exact SHA 与范围明确授权；
可以复用同一份集成检查证据，不要求重复审计。SHA、文件范围、迁移清单或远程基准
变化后必须重新检查和确认。禁止 force push。
详细脚本行为见 `docs/superpowers/runbooks/dev-integration-audit.md`，其中的本地 ACR
预校验仅作为异常排查/严格审计参考，不是默认发布前置步骤。

## 项目结构

- 前端主应用：`frontend/apps/coze-studio`
- 前端共享包：`frontend/packages`
- 后端入口：`backend/main.go`
- 后端分层：`backend/api`、`backend/application`、`backend/domain`、
  `backend/infra`、`backend/crossdomain`
- IDL 源：`idl`
- 前端生成 schema：`frontend/packages/arch/api-schema/src/idl`
- 数据库迁移：`docker/atlas/migrations`
- 长期文档：`docs/superpowers/context`、`docs/superpowers/runbooks`、
  `docs/superpowers/specs`、`docs/superpowers/plans`

## 前端边界

- UI 优先使用 `@coze-arch/coze-design` 和其 icons；只有周边已有直接用法且
  wrapper 不满足时才使用 Semi UI。
- 保留键盘、焦点、ARIA、loading、empty、error、disabled、readonly 和 refresh
  状态。
- 页面必须使用真实后端合同，不做只有说明文字的壳或前端伪权限。
- 页面验收使用 in-app browser；Chrome 仅在用户明确同意的兜底场景使用。

## 后端边界

- 后端使用 Go + Hertz，遵循现有 DDD 分层。
- 身份、空间和权限以服务端认证上下文为准，不信任客户端提交的 owner、
  `user_id` 或 `space_id`。
- 新增持久化能力必须考虑租户隔离、幂等、重试、取消、恢复和安全审计。
- credential、token、secret 不得明文落库、回显、写日志或发送到前端。

## Agent Runtime 边界

- Eino ADK 是执行内核，Coze 是控制面和系统记录。
- 新任务规范化为 `runtime=eino_adk`；`legacy` 只读历史记录和显式迁移测试。
- Workbench API/UI 只暴露审核后的公共合同，不暴露内部事件、checkpoint bytes、
  provider 原始载荷、tool 参数/结果或隐藏配置。
- Runtime 和安全依赖默认 fail closed，不静默回退到未授权执行路径。

只有任务涉及 Agent loop、Skill、MCP、memory、subagent、token、artifact 或
guardrail 时，才加载 Agent Runtime 专题文档。

## IM 与 Sandbox 边界

- IM Channels 当前只支持飞书官方 Go SDK，按工作空间隔离；外部事件先持久化
  去重，再异步映射到 Agent，不在回调中执行长任务。
- Sandbox 管理面与运行时路由分离；生产和共享环境只使用数据库中的 HTTPS
  remote provider，缺少密钥、Redis、对象存储或安全依赖时 fail closed。
- 本机 AppDev host runtime 只允许显式 Debug 模式和 loopback gateway。

## Git 与安全

- 不提交 `.codex/config.toml`、本地图谱缓存、数据库密码或生产密钥。
- 不使用 `git reset --hard` 或 `git checkout --` 回滚用户文件。
- 删除、迁移 apply、批量数据更新、合并、推送和远程分支操作必须获得用户
  对当前范围的明确确认。
- 发现相关用户改动时先理解并协同处理；无关改动保持原样。

## 输出约定

- 默认使用简体中文，先给结论，再给证据和下一步。
- 本地文件使用可点击绝对路径，命令、路径和标识符使用反引号。
- 不倾倒无关日志或长 diff；必须写清已验证、未验证和剩余风险。
