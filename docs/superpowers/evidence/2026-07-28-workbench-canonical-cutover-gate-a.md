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
  `2026-07-28T21:41:32+08:00`
- 本次只冻结测试与证据，不修改业务代码、route registration、前端 transport、
  generated files 或数据库状态。

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

## Gate Decision

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
