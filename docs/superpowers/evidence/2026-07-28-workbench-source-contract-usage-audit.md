# Workbench Source Contract Usage Audit

## 审计边界

- 固定窗口：`2026-06-28T00:00:00+08:00` 至
  `2026-07-28T21:41:32+08:00`。
- Production source baseline SHA：
  `3b060204c039d9cc775ec16298d4296964beee73`。production-hit 查询直接针对该
  Git object 执行，不依赖当前工作树内容。
- 本 evidence 文件的 current revision 机械获取命令：

```bash
git log -1 --format=%H -- \
  docs/superpowers/evidence/2026-07-28-workbench-source-contract-usage-audit.md
```

- 本地退役范围：canonical `/api/workbench/threads/**`、TaskThread V1
  `/api/workbench/task_threads/**`、LangGraph Thread `/api/threads/**` 和 stateless
  Run `/api/runs/**`。
- `backend/application/skill/builtin_deerflow/**` 是外部 DeerFlow 调用方，按任务约定
  排除在本地退役范围外。
- 本文不包含 credential、cookie、请求体、用户消息或其他用户内容。

## 五项零使用审计

| 项 | 状态 | 证据系统/来源 | 查询 | 环境 | 结果数 | Reviewer | 结论 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1. 生产源码与调用图 | `VERIFIED_LOCAL` | codebase-memory 用于调用图导航；Git object 中的 Go/TS 源码用于最终核对 | 调用图优先检查 `CreateLangGraphStatelessRun`、`registerLangGraphCustomRoutes`、`CreateTaskThread` 和 NewX client 路径，再以本文下方 baseline-bound `git grep` 命令核对字面量 | source SHA `3b060204...` | 本地生产 `/api/runs/**` 调用方：`0` | Codex | `/api/runs/**` 只有 route/handler/测试实现，未发现本地生产调用方。该结论不覆盖外部调用方。 |
| 2. 前端、生成 client、IM、scheduled task、internal tool、operations script | `VERIFIED_LOCAL` | `frontend`、生成 schema、`backend/internal`、IM/scheduled task 调用链、`scripts`、`idl` | 同一 baseline-bound `git grep` 查询及调用链核对；分类见下表 | source SHA `3b060204...`，排除 builtin DeerFlow 与测试载体 | production literal-hit files：`10`，已分类 `10/10`；本地生产 `/api/runs/**` client：`0`；operations-script 调用：`0` | Codex | Workbench/Tasks UI 使用 TaskThread V1；NewX 使用本地 Thread 路径及两个 TaskThread 产品路径；`DeerFlowClient` 使用外部 DeerFlow base URL；IM/scheduled task 直接调用应用服务。 |
| 3. Gateway/service access logs | `BLOCKED` | **缺失来源：未提供可访问的 gateway/service access-log 系统、环境索引及只读查询入口** | 固定窗口内筛选 `^/api/runs(?:/|$)`，并按 authenticated business request 与 `404`/security probe 分组 | 目标 gateway/service 环境未知且不可查询 | **不可得，不是 `0`** | Codex | 无法证明固定窗口内没有外部业务请求，也无法把业务请求与探测流量分开。 |
| 4. 外部 SDK 消费者、公开文档承诺、named owner | `BLOCKED` | **缺失来源：未提供 external SDK consumer registry/登记台账及 named-owner roster/查询入口**；仓库内 README/公开文档字面量核对可用 | 消费者登记中查找 `/api/runs/**`；仓库公开文档查找 stateless Run 承诺；核对具名 owner | 外部登记系统不可查询；本地仓库可查询 | 外部消费者：**不可得，不是 `0`**；仓库公开 README 承诺：`0`；具名 owner：不可得 | Codex | 本地未发现公开 README 承诺，旧内部 spec 的接口描述不能替代消费者登记或 owner 确认。 |
| 5. Release/acceptance matrices | `VERIFIED_LOCAL` | 仓库 release/acceptance 文档、相关 spec、route/handler tests、execution graph contract | 搜索 `stateless` 与 `/api/runs`，人工区分发布/验收矩阵和实现/测试合同 | production source baseline `3b060204...` | 要求 stateless Run 的本地 release/acceptance matrix：`0` | Codex | 本地仅有当前 route/handler tests、execution graph contract 和旧内部设计描述；未发现发布/验收矩阵。此结果不证明外部验收系统中不存在依赖。 |

第 3、4 项保持 `BLOCKED`。因此不得把“本地生产调用方为 0”扩写为“无外部
使用”，并且 **Task 14 禁止执行**；Tasks 2-13 可继续。

## 本地调用方分类

从仓库根目录运行以下完整命令。它把查询绑定到 production source baseline SHA，
明确排除 `*_test.go`、`*.test.*`、`__tests__`、`testdata` 与约定的 builtin DeerFlow
目录，并直接输出按 path 排序去重后的文件及计数：

```bash
SOURCE_SHA=3b060204c039d9cc775ec16298d4296964beee73
git grep -l -E \
  '/api/runs(/|\")|/api/threads(/|\")|/api/workbench/task_threads' \
  "$SOURCE_SHA" -- backend frontend scripts idl \
  ':(exclude)backend/application/skill/builtin_deerflow/**' \
  ':(exclude)**/*_test.go' \
  ':(exclude)**/*.test.*' \
  ':(exclude)**/__tests__/**' \
  ':(exclude)**/testdata/**' \
  | sed "s#^$SOURCE_SHA:##" \
  | LC_ALL=C sort -u \
  | awk '{ print } END { print "count=" NR }'
```

实际输出：

```text
backend/api/handler/coze/langgraph_run_service.go
backend/api/handler/coze/langgraph_thread_service.go
backend/api/handler/coze/workbench_thread_service.go
backend/internal/deerflowparity/deerflow_client.go
backend/internal/deerflowparity/newx_client.go
frontend/apps/coze-studio/src/pages/tasks/service.ts
frontend/apps/coze-studio/src/pages/tasks/task-memory-service.ts
frontend/apps/coze-studio/src/pages/workbench/service.ts
frontend/packages/arch/api-schema/src/idl/workbench/task.ts
idl/workbench/task.thrift
count=10
```

下表覆盖上述全部 10 个 production-hit 文件。

| 分类 | 实际命中与调用方式 | 合同族 |
| --- | --- | --- |
| Workbench/Tasks UI | `frontend/apps/coze-studio/src/pages/workbench/service.ts`、`src/pages/tasks/service.ts`、`src/pages/tasks/task-memory-service.ts` | `/api/workbench/task_threads/**` |
| IDL 与生成 client | `idl/workbench/task.thrift`、`frontend/packages/arch/api-schema/src/idl/workbench/task.ts` | `/api/workbench/task_threads/**` |
| NewX parity/internal tool | `backend/internal/deerflowparity/newx_client.go` 使用本地 `/api/threads/**`；另有两个 TaskThread 产品路径，分别用于 resume 和 run events | `/api/threads/**` 与 `/api/workbench/task_threads/**` |
| External DeerFlow parity client | `backend/internal/deerflowparity/deerflow_client.go` 的 `NewDeerFlowClient(baseURL, ...)` 用调用方提供的外部 DeerFlow base URL 构造 HTTP client；其 `/api/threads/**` 是指向外部 DeerFlow 服务的生产客户端路径 | 这是 production hit，但不是本项目本地 `/api/threads/**` retirement caller；该客户端路径保留，不会随本地旧合同删除 |
| IM 与 scheduled task | 调用图和源码显示其直接进入 `ApplicationService.CreateTaskThread`，不经上述本地 HTTP source family | 应用服务调用 |
| Route/handler | `langgraph_run_service.go`、`langgraph_thread_service.go`、`workbench_thread_service.go` 及 router 注册实现三组 source family | 服务端实现，不是生产 client |
| Operations scripts | `scripts` 中未发现匹配的生产调用方 | 无命中 |
| Stateless Run client | 排除约定的 DeerFlow 目录后，`/api/runs/**` 仅见 route/handler/tests；未见本地生产 client | 本地生产调用方 `0` |

## 限制与复核规则

- 浏览器控制不能替代 gateway/service access logs，也不能用于计算第 3 项结果数。
- 仓库搜索不能替代 external SDK consumer registry 或具名 owner 确认。
- 任何后续准备执行 Task 14 的变更，必须先补齐第 3、4 项固定窗口证据，并由
  对应系统 reviewer 复核；不得沿用本文的本地零调用结论作为外部零使用证明。
