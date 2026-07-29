# Workbench Canonical Cutover Gate A

## Provenance

- 分支：`codex/workbench-canonical-ui-cutover-retirement`
- Production source baseline SHA：
  `3b060204c039d9cc775ec16298d4296964beee73`。该 revision 只作为生产源码基线，
  不包含后来新增的 10-route exact snapshot，也未被用于声称运行后来新增的测试。
- Initial snapshot lineage：`4b15aefc8588de54bb65162c64f223507902c28d`。
- Browser-evidence lineage：`8690807429a9c4bf9e18d07d21dc26140d8c9957`。
- Exact snapshot/request-resolution test implementation SHA：
  `1d52b7adf719947f1a4898c9fbc3bc4ddcd63f80`。
- 本 evidence 文件的 current revision 不写入自引用 SHA，机械获取命令为：

```bash
git log -1 --format=%H -- \
  docs/superpowers/evidence/2026-07-28-workbench-canonical-cutover-gate-a.md
```

- 固定审计窗口：`2026-06-28T00:00:00+08:00` 至
  `2026-07-29T16:47:51+08:00`
- 前述 snapshot/request-resolution lineage 只冻结测试与证据；后文 Gate A candidate
  regression 还包含通过 TDD 关闭的公开投影与 Run 生命周期缺陷。整个 Gate A 不修改
  route registration、generated files 或数据库状态。

## Immutable Route Inventory

四组 route snapshot 均按 HTTP method 加完整 template 做精确集合比较，不使用
substring 计数或仅验证非 `404`。

| Route family | Prefix | 精确路由数 | 结果 |
| --- | --- | ---: | --- |
| Canonical Workbench Thread | `/api/workbench/threads/**` | 47 | PASS |
| TaskThread V1 | `/api/workbench/task_threads/**` | 36 | PASS |
| LangGraph Thread | `/api/threads/**` | 23 | PASS |
| Stateless Run | `/api/runs/**` | 10 | PASS |

新增的 stateless Run snapshot 为：

```text
POST /api/runs
POST /api/runs/stream
POST /api/runs/wait
GET  /api/runs/:run_id
GET  /api/runs/:run_id/messages
GET  /api/runs/:run_id/feedback
POST /api/runs/:run_id/cancel
GET  /api/runs/:run_id/stream
POST /api/runs/:run_id/join
GET  /api/runs/:run_id/join
```

### Snapshot TDD Lineage

1. RED：先把 `/api/runs/:run_id/feedback` 的预期 method 故意写成 `POST`，运行
   `TestRegisterIncludesLangGraphRunRoutes`。命令退出 `1`，精确 diff 报告 expected
   `POST`、actual `GET`。
2. GREEN：修正为 `GET` 后运行同一测试。命令退出 `0`：
   `ok github.com/coze-dev/coze-studio/backend/api/router/coze 1.295s`。
3. 该初始 snapshot 随 lineage commit `4b15aefc8588de54bb65162c64f223507902c28d`
   提交；production source baseline `3b060204...` 不包含此测试实现。

### Request-resolution TDD

tests-only revision `1d52b7adf719947f1a4898c9fbc3bc4ddcd63f80` 删除 canonical
测试中三个重复 source-family snapshot 调用，保留三个既定顶层测试作为唯一入口；
同时增加 middleware 记录 `RequestContext.FullPath()` 的 concrete request coverage。

1. RED：把 `POST /api/threads/search` 的 expected template 故意写为
   `/api/threads/:thread_id`。测试退出 `1`，报告 actual 为 `/api/threads/search`。
2. GREEN：修正 expected template 后，同一测试退出 `0`：
   `ok github.com/coze-dev/coze-studio/backend/api/router/coze 1.350s`。
3. 提交后在 clean tests-only SHA 上 fresh 复验该测试：退出 `0`，
   `ok github.com/coze-dev/coze-studio/backend/api/router/coze 1.226s`。

### 最终后端复验

以下命令在 clean worktree、HEAD
`1d52b7adf719947f1a4898c9fbc3bc4ddcd63f80` 上执行，验证结果绑定到该
tests-only revision：

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./api/router/coze \
  -run '^(TestWorkbenchCanonicalThreadRoutes|TestRegisterIncludesWorkbenchTaskThreadRoutes|TestRegisterIncludesLangGraphThreadRoutes|TestRegisterIncludesLangGraphRunRoutes)$' \
  -count=1
```

结果：退出码 `0`，
`ok github.com/coze-dev/coze-studio/backend/api/router/coze 0.898s`。

## Page-visible V1 Baseline

原始命令按任务原文执行：

```bash
cd frontend/apps/coze-studio
rushx test -- \
  src/pages/workbench/__tests__/workbench.test.tsx \
  src/pages/tasks/__tests__/tasks.test.tsx \
  src/pages/tasks/__tests__/tasks-service.test.ts \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/tasks/__tests__/task-run-actions-hook.test.tsx \
  src/pages/tasks/__tests__/task-memory-section.test.tsx \
  src/pages/tasks/__tests__/task-usage-service.test.ts
```

该命令不是 watch/交互模式，但 `rushx test` 已展开为
`vitest --run --passWithNoTests`，额外的 `--` 又被传给 Vitest，导致七文件筛选失效并
执行全量 94 个 test files。结果为：

- test files：89 passed，5 failed；
- tests：720 passed，13 failed；
- 失败均来自目标七文件之外的 system/announcement 相关 suites：
  `sandbox-system-page.test.tsx`、`system-page.test.tsx`、`content.test.ts`、
  `system-navigation.test.ts`、`system-service.test.ts`；
- 这些全量无关失败记录为 pre-existing/substituted baseline 情况，不归因为本次
  route snapshot 变更。

使用不带额外分隔符、仍由同一 `rushx test` script 驱动的等价七文件命令：

```bash
rushx test \
  src/pages/workbench/__tests__/workbench.test.tsx \
  src/pages/tasks/__tests__/tasks.test.tsx \
  src/pages/tasks/__tests__/tasks-service.test.ts \
  src/pages/tasks/__tests__/task-detail.test.tsx \
  src/pages/tasks/__tests__/task-run-actions-hook.test.tsx \
  src/pages/tasks/__tests__/task-memory-section.test.tsx \
  src/pages/tasks/__tests__/task-usage-service.test.ts
```

结果：退出码 `0`，7/7 files PASS，146/146 tests PASS，0 skipped，耗时
`7.08s`。输出包含既有 React warning 和 notification polling 的 sandbox
`EPERM localhost:3000` 噪声，但不影响上述七文件结果。

## Old-page Browser Control

使用已配置且已登录的 in-app browser。初始控制只创建一条验证 Thread 和一个
Run；后续证据修复均复用该记录，没有新增、删除或批量更新在线记录。持久化证据
只保留脱敏标签和规范化路径，不记录 credential、cookie、header、body 或用户内容。

| 项 | 实际取得的信息 |
| --- | --- |
| URL | 刷新后仍为规范化详情 URL `http://localhost:8080/space/{workspace_id}/tasks/{thread_id}`；动态 ID 已移除 |
| Account / role | 稳定脱敏标签 `gate-a-authenticated-account` 已验证为登录态；个人姓名与账号 ID 已移除。显式角色子项为 **`BLOCKED`**：实际检查详情页左侧账号菜单，未见 role/permission 字段；“系统管理”只是菜单入口，不能作为角色名。browser inventory 能看到 `/api/admin/auth/status`，但不提供响应体；同一已登录 browser 只读打开该 endpoint 时被客户端以 `ERR_BLOCKED_BY_CLIENT` 阻止 |
| Workspace | 脱敏空间标识 `7666...8944`；完整值只在受控 browser session 中核验，不写入持久化 evidence |
| 初始可见状态 | Task list 可见多条现有任务及状态；验证详情页可见 |
| Validation Thread | 稳定脱敏标签 `gate-a-validation-thread`；Thread ID、在线标题和用户内容均已移除 |
| Validation Run | 复用已有 Run；页面显示已完成一个步骤，未记录 Run ID、请求体或用户内容 |
| Refresh | 原控制与后续 evidence 修复均只读 reload 同一 Thread；最新代理观测后的刷新 URL、完成状态仍在，没有创建新 Thread/Run |
| Console | 本次账号菜单检查在 `2026-07-28T13:08:12.139Z` 记录一条 Tooltip/Dropdown React state-update error；`2026-07-28T13:10:22Z` 的网络证据刷新后只有 Zustand devtools 与 React Router future flag warnings，没有新增 error |
| Request paths | **`VERIFIED_BROWSER`**：in-app browser `pageAssets` inventory 记录真实 URL；临时 loopback reverse proxy 仅记录 method 与 URL，补齐真实 HTTP method。列表、详情、Run 数据与 reload 均命中 `/api/workbench/task_threads/**`；规范化 method/path/query 见下节 |

### Actual Browser Request Paths

以下结论只来自本次 in-app browser network observation 与 loopback request
interception，不使用源码推断。动态标识已规范化：

```text
GET  /api/workbench/task_threads?space_id={workspace_id}&page=1&page_size=20
GET  /api/workbench/task_threads/{thread_id}
GET  /api/workbench/task_threads/{thread_id}/messages?page=1&page_size=50
GET  /api/workbench/task_threads/{thread_id}/runs?parent_run_id=0&page=1&page_size=1
GET  /api/workbench/task_threads/{thread_id}/artifacts?page=1&page_size=50&space_id={workspace_id}
GET  /api/workbench/task_threads/{thread_id}/run_events?page=1&page_size=100
GET  /api/workbench/task_threads/{thread_id}/runs?page=1&page_size=20
GET  /api/workbench/task_threads/{thread_id}/runs?parent_run_id={run_id}&page=1&page_size=20
GET  /api/workbench/task_threads/{thread_id}/token_usage?page=1&page_size=50
POST /api/workbench/task_threads/{thread_id}/suggestions
GET  /api/workbench/task_threads/{thread_id}/run_events/stream
```

- 详情页 bootstrap 与 reload 都实际记录到 paginated list、detail、messages、runs、
  artifacts、run events、token usage、suggestions 与 stream。
- 对同一详情执行 reload 后，loopback interceptor 再次记录上述方法/路径，证明
  列表、详情、Run 读取和刷新均命中 `/api/workbench/task_threads/**`。
- 该控制仅冻结旧页面行为，不据此判断 canonical UI。

## Canonical Candidate Regression

### Candidate And Environment

- 候选分支：`codex/workbench-canonical-ui-cutover-retirement`。
- 候选 lineage：已提交基线为 `e432cab196ef2250aed27059a1b374a7f62e377a`；
  本节同时覆盖随后提交的 equivalence test、三项 canonical public projection 修复和
  三项前端 Run 生命周期修复。
  evidence 文件不写自引用最终 SHA，使用本文件开头命令机械获取。
- 后端：feature worktree，`127.0.0.1:18888`，显式启用最后一次迁移 gate，使用本机
  ignored debug 环境连接已配置在线数据库；测试环境跳过 vector store 初始化。
- 前端：同一 feature worktree，`127.0.0.1:18080`，只代理上述 feature backend。
- 账号与空间：复用 Old-page Browser Control 的同一登录会话和同一在线空间；空间只
  持久化为 `7666...8944`，不记录 session、cookie 或完整动态 ID。
- 原 `dev` 对照服务和 `*:8888` 后端未被 feature server 覆盖或修改。

### Paired Contract And Automation

`client-equivalence.test.ts` 覆盖 14 组 V1/canonical 资源对、页面可见字段、顺序、
状态、分页、错误、流事件和完整 canonical workflow。测试同时断言：

- 页面请求只使用 `/api/workbench/threads/**`；
- 无文件创建、延迟创建/上传/启动、追问、取消、恢复、重试、Artifact、Memory 和
  SSE 的写次数符合预期；
- 一个 Run 只有一个 SSE source，不存在 fallback、shadow request 或第二条 stream；
- 生产代码不包含旧 route 字符串。

Gate A fresh automation：

| 命令 | 结果 |
| --- | --- |
| `rushx test src/pages/tasks/__tests__/task-run-actions-hook.test.tsx src/pages/tasks/__tests__/task-run-event-stream.test.tsx src/pages/tasks/__tests__/task-detail-loader.test.ts` | PASS，3/3 files，26/26 tests；修复前新增断言稳定产生 5 个失败 |
| `rushx test src/pages/workbench/thread-client src/pages/workbench src/pages/tasks` | PASS，42/42 files，429/429 tests；equivalence 5/5 |
| `rushx lint` | PASS，退出码 0 |
| `rushx build` | PASS，退出码 0；仅有既有 Browserslist/package-type warning |
| `go test -p 1 -gcflags="all=-l -N" ./api/handler/coze ./api/router/coze ./application/agentthread ./application/workbench -count=1` | PASS；使用允许 loopback httptest 的本机执行环境 |

### Browser Matrix

以下均为本轮 in-app browser 实际观测；动态 ID 已规范化。测试创建两条带 Gate A
标签的 Thread，没有删除、批量更新或改写既有在线记录。

| 场景 | 结果 | 证据摘要 |
| --- | --- | --- |
| 列表、详情、状态与历史数据 | PASS | canonical 搜索和详情可见；旧 Thread 可正常解码，标题、消息、执行流程与输入区一致 |
| 无附件首次提交 | PASS | 创建、stream、终态与自动标题成功；Agent 回复只显示一次 |
| 同 Thread 追问 | PASS | 原子 Run 提交、单 stream、终态与刷新后持久化成功 |
| 单附件首次提交 | PASS | 选择安全文本 fixture 后完成延迟 Thread、上传和 Run；页面显示两步执行流程与终态 |
| 取消 | PARTIAL / RACE | 页面两次实际发出停止操作，但在线模型均在取消持久化前自然完成；不声称得到 canceled 终态。取消 route、写次数和错误投影由 deterministic tests 覆盖 |
| 恢复与重试 | AUTOMATED SUBSTITUTE | 本轮没有可安全构造的 interrupted/failed Run；canonical resume/retry 合同和页面 action 由定向测试覆盖 |
| Artifact | PASS EMPTY | 详情显示 `产物 0`，产物抽屉、当前/已移除范围、扫描队列和空态正常加载 |
| Token usage | PASS | 弹层显示对话总量、输入、输出和当前回复聚合；刷新后总量仍可见 |
| Runtime doctor | PASS | Eino ADK、模型、Web Fetch/Search、Skill 状态可见；Sandbox 未启用和 MCP unknown 是环境诊断，不是 transport 错误 |
| Memory 与安全审计 | PASS EMPTY | Memory、Guardrail、MCP audit 使用 canonical 子资源并显示 0 条空态；刷新、筛选和导入/导出入口存在 |
| 刷新持久化 | PASS | 详情 reload 后标题、两条验证回复、输入区和 usage 均恢复；无重复 Agent 回复 |

### Canonical Request And Log Ownership

浏览器行为对应的服务端 structured log 只出现 canonical contract。实际观测并规范化
的 Thread workflow 路径包括：

```text
POST /api/workbench/threads/search
POST /api/workbench/threads
GET  /api/workbench/threads/{thread_id}
GET  /api/workbench/threads/{thread_id}/messages
GET  /api/workbench/threads/{thread_id}/runs
POST /api/workbench/threads/{thread_id}/runs
GET  /api/workbench/threads/{thread_id}/runs/{run_id}/events
GET  /api/workbench/threads/{thread_id}/runs/{run_id}/stream
GET  /api/workbench/threads/{thread_id}/artifacts
GET  /api/workbench/threads/{thread_id}/token_usage
GET  /api/workbench/threads/{thread_id}/memories
GET  /api/workbench/threads/{thread_id}/guardrail_audit_events
GET  /api/workbench/threads/{thread_id}/mcp_runtime_audit_events
POST /api/workbench/threads/{thread_id}/suggestions
```

附件场景还实际完成 canonical upload workflow。日志使用 operation、route template、
status、duration、脱敏 principal hash、Thread/Run ID 和公开资源类型；未观察到消息正文、
附件名、signed URL、credential、tool 参数/结果或 checkpoint bytes。候选页未出现
`/api/workbench/task_threads/**`、本地 `/api/threads/**` 或 `/api/runs/**` 请求。

同一 Run 的服务端记录只有一条 `run.stream.reconnect`，配合页面级 write-count 测试，
未发现 duplicate write 或第二条 SSE。

### Defects Found And Closed Before Gate Decision

1. 旧数据 journal fallback 使用内部字符串 ID，违反 canonical `Message.message_id` 必须
   为正十进制字符串的公开合同。先新增失败测试，再将 fallback 投影为 source Run/Event
   数字 ID，同时拒绝非法值和归一化后重复 ID。
2. 隐藏 tool/reasoning 细节后，event-derived assistant 与持久化 assistant 可能投影为
   两条相同公开消息。先新增失败测试，再只在 canonical message projection 中优先保留
   durable assistant；events endpoint 保持不变。
3. 完整 handler package 回归进一步证明：同一毫秒内的 user fallback 与 durable
   assistant 会被 handler 的第二次 source-kind 排序反转。应用层 journal 已提供稳定的
   会话顺序，因此删除冗余二次排序，保留 user-before-assistant 的既有语义。
4. 子智能体重试返回的内部 worker 虽然是 top-level `run_kind=task`，但不代表新的主
   Task Run。前端不再把 `source=subagent_retry` 的返回值提交给主 SSE，正常顶层任务
   retry 仍按既有语义切换主 Run。
5. 详情刷新原先会把最新的子智能体重试 worker 当成主 Run。主 Run 查询现在按审核后的
   `source=subagent_retry` metadata 排除内部 worker；若第一页全是 retry worker，则按
   canonical `total` 继续读取后续有界分页，直到找到主 Run 或数据耗尽。测试同时固定
   normal retry 仍可被选中，并覆盖 20 个 retry worker 占满第一页的场景。
6. SSE 收到 `run.completed`、`run.failed`、`run.canceled/cancelled` 或
   `run.interrupted` 后原先等待额外 `onEnd` 才关闭。现在先提交终态事件，再立即 abort
   并关闭唯一 subscription；重复 terminal/end 回调均不再产生第二次关闭或写入。

六项修复后，既有在线 Thread 可正常打开且 Agent 回复只显示一次；对应前后端定向测试、
42-file 前端回归和四 package 后端 Gate A 回归均通过。

### Console Residual

- 修复上述 decoder/duplicate 问题后，没有 canonical HTTP、SSE、React render crash、
  unhandled rejection 或敏感日志错误。
- 打开 Token 用量弹层时记录一条 Semi Tooltip 的 React development warning，浏览器
  以 `error` level 收集。stack 指向未被本分支修改的
  `task-usage-popover.tsx`；`origin/dev...HEAD` 和 working-tree diff 对该文件均为空。
  Old-page control 已记录同类 Tooltip/Dropdown state-update error，因此本项作为既有
  UI library residual，不归因为 canonical transport；Gate B 仍会复核。
- 其余为 Zustand devtools extension 和 React Router future flag warning。

## Final Gate A Decision

- Steps 1-4：PASS；取消/恢复/重试中不可稳定构造的终态已由具名 deterministic test
  替代，没有虚构页面结果。
- 页面可见 parity、canonical-only request ownership、单写和单 SSE：PASS。
- 四组 source routes 在 Gate A 时仍存在，满足可回退的比较边界。
- 五项 `/api/runs/**` audit 均有显式状态，但第 3、4 项仍为 `BLOCKED`。
- **Gate A：PASS。Tasks 10-13 可继续；Task 14 仍禁止执行。**

## Baseline Gate Decision

- 本地四族 route inventory 已冻结，七个目标前端 baseline 通过等价定向命令。
- Browser request-path 子项已由真实 network inventory 补齐；显式 account-role
  字段仍为 `BLOCKED`，原因记录在上表，未用菜单权限信号推断角色名。
- 使用审计第 3 项 gateway/service access logs 为 `BLOCKED`：缺少可访问日志系统、
  环境索引和只读查询入口，无法区分 authenticated business requests 与
  `404`/security probes。
- 使用审计第 4 项 external SDK consumer registration/named owner 为 `BLOCKED`：
  缺少消费者登记系统、owner roster 和查询入口。
- 因外部证据仍阻塞，**Task 14 禁止执行**；Tasks 2-13 可继续。Gate A 的本地
  baseline 可完成，但不得把本地 `/api/runs/**` caller 数为 0 当作外部零使用证明。
