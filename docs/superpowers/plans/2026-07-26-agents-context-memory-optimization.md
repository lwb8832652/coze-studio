# AGENTS 与长期项目上下文优化实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将仓库默认上下文从历史 Nuwax 对齐阶段切换为 Coze 成熟项目的内部优化入口，并用 codebase-memory、Graphify 和双重 Git 审计支持长期开发。

**Architecture:** 根 `AGENTS.md` 只保留高频规则与事实入口；稳定项目事实进入独立 context 文档，Git 集成细节进入 runbook，历史 parity 文档保留但退出默认加载。codebase-memory 负责代码结构和影响分析，Graphify 只索引精选长期文档；需求分支通过合入前审计和合入后严格审计后才能依次进入本地、远程 `dev`。

**Tech Stack:** Markdown、Git、codebase-memory MCP/CLI、Graphify、现有 Rush/Go/Atlas/浏览器验证工具

---

## 文件职责

- Modify: `AGENTS.md`：仓库默认短入口、高频规则、工具协议和集成门禁。
- Create: `docs/superpowers/context/project-context.md`：跨任务长期有效的产品与架构事实。
- Create: `docs/superpowers/runbooks/dev-integration-audit.md`：双重审计命令、证据和停止条件。
- Modify: `docs/superpowers/runbooks/local-debug-and-test.md`：移除默认 Nuwax 参照和旧分支规则。
- Modify: `.gitignore`：忽略本地 Graphify 图谱缓存。
- Modify: `docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md`：实施时逐项更新状态。

## Task 1：重写仓库默认入口

**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md`

- [x] **Step 1：记录旧规则基线**

Run:

```bash
rg -n -i '当前一期|当前集成目标|nuwax|coze-nuwax-management-mainline' AGENTS.md
```

Expected: 输出 Nuwax、一期目标和旧日常分支相关行，证明旧入口尚未收敛。

- [x] **Step 2：用短入口替换 `AGENTS.md`**

使用 `apply_patch` 将文件整体替换为以下内容：

```markdown
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
- `graphify-out/graph.json` 存在时优先查询，不重复全量构建。
- 长期上下文变化后才做增量更新；普通局部修复不制造图谱噪声。
- 不默认对整个 monorepo 建图，不提交 `graphify-out/` 产物。
- 图谱缺失、过期或不可用时直接读取源文档，并记录缺失的图谱验证。

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
- 迁移使用 Atlas Community `v0.35.0` 校验 hash 和 validate。
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

## dev 集成门禁

每个需求完成后必须遵循
`docs/superpowers/runbooks/dev-integration-audit.md`：

1. 在需求分支完成第一次审计，确保基于最新 `origin/dev`、范围正确且验证通过；
2. 向用户提交审计报告，获得第一次明确确认后才可合入本地 `dev`；
3. 从合并后的本地 `dev` 执行更严格的第二次审计；
4. 向用户提交第二次报告，获得第二次明确确认后才可推送 `origin/dev`；
5. 禁止 force push；远程 `dev` 在审计期间变化时重新从第一次审计开始。

用户确认只对报告中的分支、SHA、文件范围和验证结果有效；提交发生变化后必须
重新审计。不得把“确认合入本地 dev”解释为远程推送授权。

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
```

- [x] **Step 3：验证入口已退出 Nuwax 默认主线**

Run:

```bash
rg -n -i '当前一期|当前集成目标|coze-nuwax-management-mainline|admin@nuwax.com' AGENTS.md
```

Expected: 无输出，退出码 `1`。

Run:

```bash
rg -n 'codebase-memory|Graphify|dev-integration-audit|project-context' AGENTS.md
```

Expected: 四类新入口均有匹配。

- [x] **Step 4：检查格式并提交**

Run:

```bash
git diff --check -- AGENTS.md
```

Expected: 退出码 `0`，无输出。

```bash
git add AGENTS.md docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md
git commit -m "docs: refocus repository agent guidance"
```

## Task 2：建立长期项目上下文

**Files:**
- Create: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md`

- [x] **Step 1：验证长期上下文尚不存在**

Run:

```bash
test -f docs/superpowers/context/project-context.md
```

Expected: 退出码 `1`。

- [x] **Step 2：创建长期上下文文件**

使用 `apply_patch` 创建以下内容：

```markdown
# Coze Studio 项目上下文

## 当前阶段

本仓库基于 Coze Studio，已经完成从外部参考项目吸收主要能力的阶段，当前默认
方向是 Coze 原生架构内的功能完善、可靠性、安全、性能、可维护性和用户体验
优化。Nuwax、DeerFlow 等历史资料只用于追溯来源或定位回归，不定义当前产品。

## 技术栈

- 前端：React、TypeScript、Rush.js、Coze Design/Semi UI；
- 后端：Go、Hertz、DDD 分层；
- Agent 执行：Go-native Agent Harness 与 Eino ADK；
- 合同：Thrift IDL 与生成的前端 API schema/client；
- 数据：MySQL、Atlas migrations、Redis、对象存储；
- 本地开发：Make、Rush、Vitest、Go test、Codex in-app browser。

## 稳定架构边界

### 前端

主应用位于 `frontend/apps/coze-studio`，共享能力位于 `frontend/packages`。页面
应使用生成 client、真实权限和现有设计系统，保留完整可访问性与状态反馈。

### 后端

后端入口为 `backend/main.go`，主要分层为：

- `backend/api`：HTTP handler 和路由；
- `backend/application`：用例编排和跨领域协调；
- `backend/domain`：实体、仓储接口和领域服务；
- `backend/infra`：数据库、缓存、对象存储和外部服务实现；
- `backend/crossdomain`：经过明确合同的跨域能力。

身份、空间、角色和系统管理员权限始终由服务端认证上下文及持久化事实决定。

### Agent Runtime

Eino ADK 是执行内核，Coze 保存公共 task、event、checkpoint、memory、artifact、
token、Skill、MCP 和 guardrail 合同。内部运行时对象必须通过 adapter 投影为
经过审核的公共字段，Workbench 不暴露原始 prompt、tool payload、checkpoint、
credential、provider body 或隐藏配置。

### AppDev 与 Sandbox

Sandbox 控制面和运行流量分别启用。生产与共享环境只通过数据库配置的 HTTPS
remote provider 执行；本机 host runtime 只允许显式 Debug 模式。安全依赖缺失
时 fail closed，不回退到宿主机或内存 stub。

### IM Channels

当前只支持飞书官方 Go SDK。配置按工作空间隔离，secret 加密且不回显；外部
事件先持久化去重，再进入异步 Agent 流程。

## 主要产品域

- 任务与 Agent Workbench：任务、消息、运行、恢复、工具、产物和长期记忆；
- 工作空间与系统管理：成员、角色、系统配置、模型和管理员能力；
- Skill 与 MCP：配置、版本、授权、健康状态和运行时装配；
- AppDev 与 Sandbox：项目文件、构建、预览、Provider 和安全网关；
- 通知与计划任务：可靠通知、公告、定时执行、幂等和重试；
- 计费与配额：服务端事实、审计和安全边界。

## 事实来源

1. 当前源码、IDL、迁移和运行时行为；
2. 相关测试与生成代码；
3. 本文件及当前有效 runbook/spec；
4. codebase-memory 与 Graphify 派生图谱；
5. 历史 plans/specs 和外部参考项目。

## 长期记忆工具

### codebase-memory

用于代码结构、符号搜索、调用链和变更影响。每次结论都要检查项目、索引新鲜
度和具体源码；架构决策应同步到版本控制文档，不能只保存在本机数据库。

### Graphify

用于本目录内精选长期上下文的概念和文档关系。只有长期事实变化后才更新；
输出保存在 ignored `graphify-out/`，不能代替受版本控制的事实文件。图谱不可用
或与文档冲突时直接读取源文档，并记录降级情况。

## 当前运行手册

- 本地调试：`docs/superpowers/runbooks/local-debug-and-test.md`
- dev 集成审计：`docs/superpowers/runbooks/dev-integration-audit.md`
- Sandbox：`docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Guardrail：`docs/superpowers/runbooks/guardrail-audit-operations.md`
- Agent Runtime：`docs/superpowers/specs/2026-06-28-eino-agent-runtime-guidance.md`

## 更新规则

出现以下变化时更新本文件或对应 ADR/runbook：

- 跨模块架构、所有权或依赖方向变化；
- 公共 API、IDL、持久化、安全或租户合同变化；
- 生产部署、故障恢复、密钥、迁移或发布流程变化；
- 后续任务必须持续遵守的新决策。

任务进度、临时日志、一次性命令输出和局部实现细节不进入本文件。发现过期事实
时在同一需求分支修正，并在审计报告中列出。
```

- [x] **Step 3：验证长期上下文结构和链接**

Run:

```bash
rg -n '^## (当前阶段|技术栈|稳定架构边界|主要产品域|事实来源|长期记忆工具|当前运行手册|更新规则)$' docs/superpowers/context/project-context.md
```

Expected: 八个标题均有匹配。

Run:

```bash
test -f docs/superpowers/runbooks/local-debug-and-test.md && test -f docs/superpowers/runbooks/sandbox-control-plane-operations.md && test -f docs/superpowers/runbooks/guardrail-audit-operations.md && test -f docs/superpowers/specs/2026-06-28-eino-agent-runtime-guidance.md
```

Expected: 退出码 `0`。

- [x] **Step 4：提交长期上下文**

```bash
git add docs/superpowers/context/project-context.md docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md
git commit -m "docs: add durable project context"
```

## Task 3：固化双重 dev 审计

**Files:**
- Create: `docs/superpowers/runbooks/dev-integration-audit.md`
- Modify: `docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md`

- [ ] **Step 1：验证审计手册尚不存在**

Run:

```bash
test -f docs/superpowers/runbooks/dev-integration-audit.md
```

Expected: 退出码 `1`。

- [ ] **Step 2：创建审计手册**

使用 `apply_patch` 创建以下内容：

```markdown
# dev 集成双重审计手册

## 目的

每个需求必须先在独立 `codex/` 分支完成。需求分支通过第一次审计并获得用户
确认后，才能合入本地 `dev`；合并后的本地 `dev` 必须接受更严格的第二次审计，
再次获得用户确认后才能推送 `origin/dev`。

两次确认只对报告中列出的分支、SHA、文件范围和验证结果有效。提交、远程基准
或范围变化后，原确认失效。

## 禁止事项

- 禁止直接在 `dev` 开发需求；
- 禁止把第一次确认解释为远程推送授权；
- 禁止 force push、自动合并或自动解决冲突；
- 禁止在本地 `dev` 上试合并、修冲突或补功能；
- 禁止合并未提交、未审计或来源不明的 worktree 改动；
- 禁止复用需求分支测试结果冒充合并后验证。

## 审计前提

执行前确认：

```bash
git status --short --branch
git worktree list --porcelain
git branch -vv
git remote -v
```

若需求相关 worktree 不干净、`dev` 被未知 worktree 占用、远程配置异常或存在
无法归属的相关改动，停止并向用户报告。

## 第一次审计：需求分支

### 1. 固定远程基准

```bash
git fetch origin dev
git rev-parse origin/dev
git rev-parse dev
git rev-parse HEAD
git rev-list --left-right --count dev...origin/dev
```

记录三个 SHA 和领先/落后计数。本地 `dev` 必须能够 fast-forward 到
`origin/dev`；若本地 `dev` 含远程没有的提交或双方分叉，停止审计。

### 2. 在需求分支吸收最新基准

```bash
git merge-base --is-ancestor origin/dev HEAD
```

若退出码不是 `0`，在需求分支执行：

```bash
git merge --no-edit origin/dev
```

冲突只能在需求分支解决。解决后重新运行完整需求验证；不得在第一次审计期间
切到或修改本地 `dev`。

### 3. 审核提交和文件范围

```bash
git log --oneline --decorate origin/dev..HEAD
git diff --stat origin/dev...HEAD
git diff --name-status origin/dev...HEAD
git diff --check origin/dev...HEAD
git diff --check
```

检查无关文件、生成代码、锁文件、迁移、credential、调试日志和本地缓存。

### 4. 验证需求

按变更类型执行：

- 前端：相关 Vitest，必要时 typecheck、lint、build 和 in-app browser；
- 后端：相关 Go package 测试，必要时跨包测试和 build；
- 迁移：Atlas hash 与 validate；
- 文档：链接、残留规则、`git diff --check` 和工具可用性；
- 所有需求：codebase-memory `detect_changes` 或等价影响分析。

### 5. 第一次报告和确认

报告必须列出：

- `origin/dev` 基准 SHA；
- 本地 `dev` SHA 和领先/落后关系；
- 需求分支与审计 SHA；
- 提交列表和文件范围；
- 验证命令、退出状态和关键结果；
- codebase-memory 影响面；
- Graphify 验证状态（适用时）；
- 已知风险、未验证项和停止条件。

报告后停止。只有用户明确确认合入本地 `dev` 才能进入下一阶段。

## 合入本地 dev

用户第一次确认后，先重新 fetch 并确认远程 SHA 未变化：

```bash
git fetch origin dev
git rev-parse origin/dev
```

若 SHA 与第一次报告不同，确认失效，回到第一次审计。

在 `dev` 所在 worktree 确认工作区干净后执行：

```bash
git switch dev
git merge --ff-only origin/dev
git merge --no-ff <audited-feature-sha>
```

合并对象必须是第一次报告中的审计 SHA，并再次确认需求分支仍指向该 SHA。
发生冲突时中止合并，回到需求分支吸收最新 `origin/dev`；不得直接在 `dev`
解决冲突。

## 第二次审计：本地 dev

### 1. 固定合并结果

```bash
git rev-parse origin/dev
git rev-parse dev
git log --oneline --decorate origin/dev..dev
git diff --stat origin/dev...dev
git diff --name-status origin/dev...dev
git diff --check origin/dev...dev
```

确认只包含已批准需求提交和预期 merge commit。

### 2. 严格重新验证

所有相关测试必须从本地 `dev` 重新执行。第二次审计至少包含第一次全部命令，
并按风险增加：

- 共享前端包：消费方测试、typecheck 或 build；
- 公共后端合同：跨 application/api/router package 测试；
- 权限与租户：未授权、跨空间和脱敏路径；
- 页面：真实 URL、账号/空间、核心交互和控制台；
- 迁移：hash、validate、顺序和兼容性；
- 文档与上下文：链接、残留扫描、Graphify 增量结果；
- 全部变更：codebase-memory 最终影响和反向依赖。

同时扫描：

```bash
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!graphify-out/**' --glob '!.git/**'
git status --short
```

### 3. 远程竞态检查

```bash
git fetch origin dev
git rev-parse origin/dev
git rev-list --left-right --count origin/dev...dev
```

远程 SHA 必须仍等于第一次审计基准，且本地 `dev` 只能领先预期提交。否则停止，
不得自动 pull、rebase、push 或覆盖本地 `dev`。记录当前 merge SHA 并向用户报告；
只有获得明确恢复授权后，才能将本地 `dev` 重新对齐新的 `origin/dev`，随后回到
需求分支重新执行第一次审计。

### 4. 第二次报告和确认

报告必须列出合并前后 SHA、最终提交范围、重新运行的全部验证、工具影响分析、
远程竞态检查和剩余风险。报告后停止；只有用户第二次明确确认才可推送。

## 推送与核验

用户第二次确认后执行：

```bash
git push origin dev
git ls-remote --heads origin dev
git rev-parse dev
```

远程 SHA 必须与本地 `dev` 一致。推送被拒绝时不得 force push，也不得自动覆盖
本地 `dev`；保留并报告本地 merge SHA，按用户明确授权恢复基准后从第一次审计
开始。
```

- [ ] **Step 3：验证双重门禁和停止条件**

Run:

```bash
rg -n '^## (第一次审计：需求分支|合入本地 dev|第二次审计：本地 dev|推送与核验)$' docs/superpowers/runbooks/dev-integration-audit.md
```

Expected: 四个关键阶段均有匹配。

Run:

```bash
rg -n '第一次确认|第二次确认|禁止 force push|回到第一次审计' docs/superpowers/runbooks/dev-integration-audit.md
```

Expected: 两次确认、禁止强推和失效重审规则均有匹配。

- [ ] **Step 4：提交审计手册**

```bash
git add docs/superpowers/runbooks/dev-integration-audit.md docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md
git commit -m "docs: add strict dev integration audit"
```

## Task 4：清理旧调试入口并忽略 Graphify 缓存

**Files:**
- Modify: `docs/superpowers/runbooks/local-debug-and-test.md:3-19,140-145`
- Modify: `.gitignore:64-68`
- Modify: `docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md`

- [ ] **Step 1：验证旧默认项和缓存未忽略**

Run:

```bash
rg -n -i 'nuwax|coze-nuwax-management-mainline' docs/superpowers/runbooks/local-debug-and-test.md
git check-ignore graphify-out/.graphify_detect.json
```

Expected: 第一条命令有匹配；第二条命令退出码 `1`。

- [ ] **Step 2：收敛调试手册**

使用 `apply_patch`：

```diff
-本手册只记录 Coze Studio 当前主线的本地调试、nuwax-ai 页面参照和验收口径。
-不要写入真实生产密钥，也不要恢复已经移除的 DeerFlow 对齐流程。
+本手册只记录 Coze Studio 当前项目的本地调试和验收口径。不要写入真实生产
+密钥；历史 Nuwax/DeerFlow 环境只在任务明确要求回归追溯时按对应旧文档使用。
@@
-nuwax-ai 本地参照环境：
-
-- URL: `http://localhost/`
-- Email: `admin@nuwax.com`
-- Password: `123456`
-
@@
 ## 分支与转测试

-- 日常开发分支为 `codex/coze-nuwax-management-mainline`。
-- 不默认合并、推送或切换到 `dev`。
-- 转测试前先完成代码审核，再按用户本次明确授权的目标分支和步骤执行。
-- 目标分支被其他 worktree 占用时，报告占用路径，不强制 checkout。
+- `dev` 是本地与远程集成分支，需求在独立 `codex/` 分支实施。
+- 合入本地 `dev` 前后分别执行一次审计，并在两个阶段各获得用户明确确认。
+- 完整命令、证据和停止条件见
+  `docs/superpowers/runbooks/dev-integration-audit.md`。
+- 目标分支被其他 worktree 占用时，报告占用路径，不强制 checkout。
```

- [ ] **Step 3：增加本地图谱忽略规则**

在 `.gitignore` 末尾添加：

```gitignore

# Local Graphify cache
/graphify-out/
```

- [ ] **Step 4：验证旧入口消失且缓存被忽略**

Run:

```bash
rg -n -i 'admin@nuwax.com|coze-nuwax-management-mainline' AGENTS.md docs/superpowers/runbooks/local-debug-and-test.md
```

Expected: 无输出，退出码 `1`。

Run:

```bash
git check-ignore -v graphify-out/.graphify_detect.json
git diff --check -- .gitignore docs/superpowers/runbooks/local-debug-and-test.md
```

Expected: Graphify 缓存路径命中 `.gitignore`；diff check 退出码 `0`。

- [ ] **Step 5：提交调试与忽略规则**

```bash
git add .gitignore docs/superpowers/runbooks/local-debug-and-test.md docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md
git commit -m "docs: retire legacy parity defaults"
```

## Task 5：验证长期记忆工具和文档合同

**Files:**
- Modify: `docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md`
- Local ignored output: `graphify-out/`

- [ ] **Step 1：验证 codebase-memory 项目索引**

Run:

```bash
codebase-memory-mcp cli list_projects
```

Expected: 输出项目根路径 `/Users/liuwenbo/code/BuildingAI/coze-studio`，节点和边数
均大于 `0`。

Run:

```bash
codebase-memory-mcp cli get_architecture --project Users-liuwenbo-code-BuildingAI-coze-studio --aspects overview
```

Expected: 退出码 `0`，包含 TypeScript、Go 和主要 backend 分层信息。

- [ ] **Step 2：执行 codebase-memory 变更影响基线**

Run:

```bash
codebase-memory-mcp cli detect_changes --project Users-liuwenbo-code-BuildingAI-coze-studio --since origin/dev --depth 2
```

Expected: 退出码 `0`；输出列出本需求文档变更。文档变更可以没有受影响代码
符号；用户已有无关未跟踪文件必须单独标记，不能归入本需求范围。

- [ ] **Step 3：对精选上下文运行 Graphify**

调用 Graphify skill，对 `docs/superpowers/context` 执行完整流程，技能调用参数为：

```text
/graphify docs/superpowers/context --no-viz
```

这不是原始 shell CLI 命令；必须按 Graphify `SKILL.md` 完成检测、提取、聚类和
报告生成。

Expected: `graphify-out/graph.json` 与 `graphify-out/GRAPH_REPORT.md` 存在，检测语料
只来自 `docs/superpowers/context`，图非空。

- [ ] **Step 4：验证精选语料和图谱输出**

Run:

```bash
test -s graphify-out/graph.json
test -s graphify-out/GRAPH_REPORT.md
rg -n 'docs/superpowers/context' graphify-out/.graphify_root graphify-out/.graphify_detect.json
```

Expected: 两个产物非空，root 与检测清单都指向精选 context 目录。

Run:

```bash
git check-ignore -v graphify-out/graph.json graphify-out/GRAPH_REPORT.md
git status --short
```

Expected: 两个文件均命中 `.gitignore`；`git status` 不显示 `graphify-out/`。

- [ ] **Step 5：验证所有默认入口链接存在**

Run:

```bash
for path in \
  docs/superpowers/context/project-context.md \
  docs/superpowers/runbooks/local-debug-and-test.md \
  docs/superpowers/runbooks/dev-integration-audit.md \
  docs/superpowers/runbooks/sandbox-control-plane-operations.md \
  docs/superpowers/specs/2026-06-28-eino-agent-runtime-guidance.md; do
  test -f "$path" || exit 1
done
```

Expected: 退出码 `0`。

- [ ] **Step 6：提交验证状态**

更新本计划 Task 5 checkbox 后提交，不包含 ignored 图谱：

```bash
git add docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md
git commit -m "docs: verify durable context tooling"
```

## Task 6：完成需求分支验证

**Files:**
- Modify: `docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md`

- [ ] **Step 1：运行旧规则残留扫描**

Run:

```bash
rg -n -i '当前一期|当前集成目标|admin@nuwax.com|coze-nuwax-management-mainline' AGENTS.md docs/superpowers/context docs/superpowers/runbooks/local-debug-and-test.md docs/superpowers/runbooks/dev-integration-audit.md
```

Expected: 无输出，退出码 `1`。

- [ ] **Step 2：运行结构与格式检查**

Run:

```bash
test "$(wc -l < AGENTS.md | tr -d ' ')" -le 220
git diff --check origin/dev...HEAD
rg -n '^(<<<<<<<|=======|>>>>>>>)' AGENTS.md docs/superpowers/context docs/superpowers/runbooks
```

Expected: 行数检查和 diff check 退出码 `0`；冲突标记扫描无输出、退出码 `1`。

- [ ] **Step 3：复核提交范围**

Run:

```bash
git log --oneline --decorate origin/dev..HEAD
git diff --stat origin/dev...HEAD
git diff --name-status origin/dev...HEAD
git status --short
```

Expected: 仅包含本需求设计、计划、四份目标文档和 `.gitignore`；用户已有无关
未跟踪文件保持原样且未进入提交。

- [ ] **Step 4：提交完成状态**

```bash
git add docs/superpowers/plans/2026-07-26-agents-context-memory-optimization.md
git commit -m "docs: complete context guidance rollout"
```

## Task 7：执行第一次 dev 集成审计并停止

**Files:**
- Read only: Git refs、worktree、提交和验证输出

- [ ] **Step 1：固定远程和本地基准**

```bash
git fetch origin dev
git rev-parse origin/dev
git rev-parse dev
git rev-parse HEAD
git rev-list --left-right --count dev...origin/dev
git worktree list --porcelain
git status --short --branch
```

Expected: 能明确记录三个 SHA；本地 `dev` 不得含未同步远程提交或分叉。

- [ ] **Step 2：确保需求分支包含最新远程 dev**

```bash
git merge-base --is-ancestor origin/dev HEAD
```

Expected: 退出码 `0`。若不是，执行 `git merge --no-edit origin/dev` 后重新运行
Task 5 和 Task 6 的全部验证。

- [ ] **Step 3：输出第一次审计报告**

报告包括基准/需求 SHA、提交列表、文件范围、所有验证、codebase-memory 影响、
Graphify 状态、无关工作区文件和剩余风险。

- [ ] **Step 4：停止并等待第一次用户确认**

不得切换到 `dev`、合并本地 `dev` 或推送远程。只有用户明确回复允许合入本地
`dev` 后，才能按 `dev-integration-audit.md` 执行本地合并和第二次审计。

## 用户第一次确认后的流程

以下步骤不在第一次审计前自动执行：

1. 重新 fetch 并确认 `origin/dev` SHA 未变化；
2. fast-forward 本地 `dev` 到已审计远程基准；
3. 用 `--no-ff` 合入审计提交；
4. 从本地 `dev` 重新运行全部验证和更广的第二次审计；
5. 输出第二次报告并再次停止；
6. 只有用户第二次确认后才执行 `git push origin dev`；
7. 推送后比较远程和本地 SHA，禁止 force push。
