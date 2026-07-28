# Workbench Canonical Cutover Gate A

## 基线标识

- 分支：`codex/workbench-canonical-ui-cutover-retirement`
- Gate A 源码 HEAD：`3b060204c039d9cc775ec16298d4296964beee73`
- 固定审计窗口：`2026-06-28T00:00:00+08:00` 至
  `2026-07-28T21:12:34+08:00`
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

在 Gate A 截止前的最新结果：退出码 `0`，
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
| Account / role | 账号身份已验证为 `刘文波`（`@840582614`）；显式角色子项为 **`BLOCKED`**。实际检查位置是详情页左侧账号菜单：展开后只有姓名、账号 ID、API 授权、模型管理、MCP 配置、IM 机器人、账号设置、积分与订阅、系统管理和退出登录，没有 role/permission 字段。“系统管理”只是菜单入口，不能作为角色名。browser network inventory 能看到 `/api/admin/auth/status`，但不提供响应体；尝试在同一已登录 in-app browser 中只读打开该已观察 endpoint 时被客户端以 `ERR_BLOCKED_BY_CLIENT` 阻止，因此无法取得明确角色值 |
| Workspace | 个人空间 ID `7666420680379858944` |
| 初始可见状态 | Task list 可见多条现有任务及状态；新建后 task detail 可见 |
| Validation Thread | 验证标记 `GATE-A-BASELINE-20260728-2040`；Thread/task ID `7667558479128690688`；完成后页面标题为 `GATE Baseline Validation Confirmed` |
| Validation Run | 页面显示一个 Run 已完成、`已完成 1 个步骤`，token usage 显示 `14.7K`；未在证据中记录请求体或用户内容 |
| Refresh | 原控制完成后刷新一次；本次证据修复又对同一 Thread 做一次只读 reload。修复后的刷新 URL、Thread 标题和完成状态仍在，没有创建新 Thread/Run |
| Console | 本次账号菜单检查在 `2026-07-28T13:08:12.139Z` 记录一条 Tooltip/Dropdown React state-update error；`2026-07-28T13:10:22Z` 的网络证据刷新后只有 Zustand devtools 与 React Router future flag warnings，没有新增 error |
| Request paths | **`VERIFIED_BROWSER`**：使用 in-app browser `pageAssets` network inventory 读取真实 resource URL 和触发类型。列表、详情、Run 数据与 reload 均观察到 `/api/workbench/task_threads/**`；精确路径见下节 |

### Actual Browser Request Paths

以下结论只来自本次 in-app browser 的 network resource inventory，不使用源码推断。
inventory 将普通调用标记为 `xmlhttprequest`，将 event stream 标记为 `other`。

- 刷新已有详情后的 inventory `ec2d4016-43b9-44cd-b0b6-b8d295cd1bc0` 实际记录：

```text
/api/workbench/task_threads?space_id=7666420680379858944&page=1&page_size=20
/api/workbench/task_threads/7667558479128690688
/api/workbench/task_threads/7667558479128690688/messages?page=1&page_size=50
/api/workbench/task_threads/7667558479128690688/runs?parent_run_id=0&page=1&page_size=1
/api/workbench/task_threads/7667558479128690688/artifacts?page=1&page_size=50&space_id=7666420680379858944
/api/workbench/task_threads/7667558479128690688/run_events?page=1&page_size=100
/api/workbench/task_threads/7667558479128690688/runs?page=1&page_size=20
/api/workbench/task_threads/7667558479128690688/runs?parent_run_id=7667558479128707072&page=1&page_size=20
/api/workbench/task_threads/7667558479128690688/token_usage?page=1&page_size=50
```

- 点击“返回全部任务”后，inventory `d251260b-2d2f-4dea-8ef4-d49551ef945f`
  新观察到列表路径
  `/api/workbench/task_threads?space_id=7666420680379858944`。
- 从列表重新打开已有验证 Thread 后，inventory
  `ea1837e1-3010-4335-b6b1-f270afcf50ed` 再次观察到详情、messages、runs、
  artifacts、run events 和 token usage 路径；同时记录到
  `/api/workbench/task_threads/7667558479128690688/suggestions` 与
  `/api/workbench/task_threads/7667558479128690688/run_events/stream`。
- 因此，本次实际 browser control 的列表、详情、Run 读取和刷新都命中
  `/api/workbench/task_threads/**`。该控制仅冻结旧页面行为，不据此判断 canonical UI。

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
