# Workbench Canonical Cutover Gate A

## 基线标识

- 分支：`codex/workbench-canonical-ui-cutover-retirement`
- Gate A 源码 HEAD：`3b060204c039d9cc775ec16298d4296964beee73`
- 固定审计窗口：`2026-06-28T00:00:00+08:00` 至
  `2026-07-28T20:49:05+08:00`
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

### TDD 记录

1. RED：先把 `/api/runs/:run_id/feedback` 的预期 method 故意写成 `POST`，运行
   `TestRegisterIncludesLangGraphRunRoutes`。命令退出 `1`，精确 diff 报告 expected
   `POST`、actual `GET`。
2. GREEN：修正为 `GET` 后运行同一测试。命令退出 `0`：
   `ok github.com/coze-dev/coze-studio/backend/api/router/coze 1.295s`。

### 最终后端复验

```bash
cd backend
GOCACHE=/private/tmp/coze-workbench-cutover-go-cache \
  go test -p 1 -gcflags="all=-l -N" ./api/router/coze \
  -run '^(TestWorkbenchCanonicalThreadRoutes|TestRegisterIncludesWorkbenchTaskThreadRoutes|TestRegisterIncludesLangGraphThreadRoutes|TestRegisterIncludesLangGraphRunRoutes)$' \
  -count=1
```

在 `2026-07-28T20:49:05+08:00` 截止前的最新结果：退出码 `0`，
`ok github.com/coze-dev/coze-studio/backend/api/router/coze 1.403s`。

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

使用已配置且已登录的 in-app browser，只创建一条验证 Thread 和一个 Run；没有
删除或批量更新在线记录。

| 项 | 实际取得的信息 |
| --- | --- |
| URL | 刷新后仍为 `http://localhost:8080/space/7666420680379858944/tasks/7667558479128690688` |
| Account / role | 页面显示账号 `刘文波`（`@840582614`）；未取得明确 role 字段，仅实际观察到账号菜单存在“系统管理”入口，不能据此推断具体角色名 |
| Workspace | 个人空间 ID `7666420680379858944` |
| 初始可见状态 | Task list 可见多条现有任务及状态；新建后 task detail 可见 |
| Validation Thread | 验证标记 `GATE-A-BASELINE-20260728-2040`；Thread/task ID `7667558479128690688`；完成后页面标题为 `GATE Baseline Validation Confirmed` |
| Validation Run | 页面显示一个 Run 已完成、`已完成 1 个步骤`，token usage 显示 `14.7K`；未在证据中记录请求体或用户内容 |
| Refresh | 完成后刷新一次；URL、Thread 标题、完成状态和可见结果仍在 |
| Console | 刷新后未见新增 error；存在 Zustand devtools、React Router future flag warnings。刷新前 `2026-07-28T12:42:18.408Z` 有一条 Tooltip/Dropdown 相关 React state-update error |
| Request paths | **NOT CAPTURED**：in-app browser evaluate 未提供 performance resource 列表，dev logs 对三个 API prefix 的过滤均无记录。不能从本次 browser control 推断实际请求路径 |

本地当前源码另行证明 Workbench/Tasks UI transport 使用
`/api/workbench/task_threads/**`；这不是 browser request log，也不能由本次 control
得出 canonical UI 结论。

## Gate Decision

- 本地四族 route inventory 已冻结，七个目标前端 baseline 通过等价定向命令。
- 使用审计第 3 项 gateway/service access logs 为 `BLOCKED`：缺少可访问日志系统、
  环境索引和只读查询入口，无法区分 authenticated business requests 与
  `404`/security probes。
- 使用审计第 4 项 external SDK consumer registration/named owner 为 `BLOCKED`：
  缺少消费者登记系统、owner roster 和查询入口。
- 因外部证据仍阻塞，**Task 14 禁止执行**；Tasks 2-13 可继续。Gate A 的本地
  baseline 可完成，但不得把本地 `/api/runs/**` caller 数为 0 当作外部零使用证明。
