# Workbench Final Contract Retirement

## 结论

Workbench 产品链路已经收敛到唯一公共合同 `/api/workbench/threads/**`。本地旧
TaskThread V1、LangGraph Thread 和 stateless Run HTTP 合同均已物理退役；Scheduled
Task、外部 DeerFlow client 以及共享 application/domain/repository/MySQL/Eino ADK
执行主链保持不变。

本结论只描述本次最终退役分支的源码和本轮验证结果。证据文件 revision 使用以下命令
机械获取，不在文件内写自引用 SHA：

```bash
git log -1 --format=%H -- \
  docs/superpowers/evidence/2026-07-30-workbench-final-contract-retirement.md
```

## 变更边界

| 合同族                                         | 最终状态                       | 精确数量 |
| ---------------------------------------------- | ------------------------------ | -------: |
| `/api/workbench/threads/**`                    | 保留，Workbench 唯一产品合同   |       47 |
| `/api/workbench/scheduled_tasks/**` 及配套查询 | 保留，独立 Scheduled Task 合同 |       11 |
| `/api/workbench/task_threads/**`               | 退役，不可路由                 |       36 |
| 本地 `/api/threads/**`                         | 退役，不可路由                 |       23 |
| 本地 `/api/runs/**`                            | 退役，不可路由                 |       10 |

本次删除 stateless Run 的路由注册、handler、HTTP binding、专属测试和无其他调用方的
手写 LangGraph model。canonical Run SSE 仍保留审核后的 SDK-compatible event name 与
public payload shape，但所有权已迁入
`workbench_canonical_run_stream_protocol.go`，不再依赖旧 HTTP adapter。

没有修改数据库 migration、表结构、存量数据、应用层业务语义、领域状态机或 Runtime
执行策略。外部 DeerFlow 的 `/api/threads/**` client 路径不属于本地路由，继续保留。

## 路由与残留证据

- Hertz 精确路由快照验证 47 条 canonical 和 11 条 Scheduled Task 路由仍注册。
- 同一测试逐 method/path 验证 36 条 TaskThread V1、23 条本地 LangGraph Thread 和
  10 条 stateless Run 路由均不可达。
- 生产源码扫描未发现旧 TaskThread 路径、旧本地 route owner、旧 feature flag、旧
  selector 或 stateless handler/model 符号。
- 生产源码中的本地 `/api/threads/**` 命中为零；保留命中仅位于外部 DeerFlow client
  与 parity 实现。
- codebase-memory 快速重建后，旧 stateless handler/route registration 符号为零；
  canonical `StreamCanonicalRun` 仍连接鉴权、application service 和 canonical SSE
  protocol。

未认证 HTTP 探测会先被全局 session middleware 返回 `401`，不能据此证明具体路由
是否注册。因此旧路由结论以 Hertz 内部精确 route snapshot 和逐路由 handler-boundary
断言为准，不把未认证探测误记为 `404` 证据。

## 自动化验证

| 范围                                                          | 本轮结果                                                          |
| ------------------------------------------------------------- | ----------------------------------------------------------------- |
| Execution graph contract                                      | `53/53` 通过；图谱为 114 nodes、131 edges、27 chains              |
| Backend API codegen verification                              | 通过，58 个生成文件一致                                           |
| Router canonical/retirement tests                             | 通过                                                              |
| Canonical SSE focused handler tests                           | 通过                                                              |
| `application/workbench`、`application/agentthread`            | 通过                                                              |
| `domain/agentthread/service`、`domain/agentthread/repository` | 通过                                                              |
| `internal/deerflowparity`                                     | 通过                                                              |
| Backend focused `go vet`                                      | 通过                                                              |
| Backend build                                                 | `go build` 与 `make build_server` 通过                            |
| Workbench/Tasks frontend tests                                | 42 files、429 tests 全部通过                                      |
| Frontend API schema tests                                     | 全包 4 files、17 tests 全部通过；Workbench 子集 3 files、13 tests |
| Frontend lint/build                                           | 通过                                                              |

完整 `backend/api/handler/coze` package 仍有 23 个既有 Workflow/Mockey 失败；在未修改的
最新 `dev` 基线执行同一测试得到完全相同的 23 个失败。失败集中不含 Workbench 或
canonical handler，本次受影响的 canonical SSE focused tests 单独通过。该基线问题不
作为本次退役新增回归，但仍保留为仓库既有测试债务。

## 运行态页面回归

- Frontend：`http://localhost:8090`。
- Backend：`http://localhost:8890`，使用 ignored debug env 中的远程测试 MySQL；没有
  启动或写入本地 MySQL。
- 页面：`/space/<test-space>/tasks/<existing-thread>`，使用本地调试 runbook 账号。
- 验证：登录、任务列表、已完成任务详情、消息、执行流程、产物、Run、RunEvent、
  Token 用量与 SSE reconnect 均正常展示；本轮没有创建、修改、取消或删除任务。
- Workbench 相关后端访问日志只记录 canonical `thread.search`、`thread.get`、`artifact.list`、
  `run.list`、`thread.messages.list`、`run.events.list`、`run.stream.reconnect`、
  `token_usage.get` 和 `thread.suggestions.generate`，对应路径全部为
  `/api/workbench/threads/**`。
- 浏览器控制台无业务 error；仅有 Redux DevTools 未安装和 React Router future flag
  的开发环境 warning。

Milvus 在当前本机网络不可达，后端按项目已有 debug-only 开关
`COZE_DEBUG_SKIP_VECTOR_STORE=true` 启动。该开关只跳过本地页面回归不涉及的 vector
store 初始化，不改变生产配置或本次 Workbench 合同结论。

## Owner Override 与剩余风险

`/api/runs/**` 五项零使用审计的第 3、4 项仍为 `BLOCKED`：缺少可查询的
gateway/service access logs、external SDK consumer registry 和 named-owner roster。
因此不能声称已经证明外部零流量或零消费者。

项目决策者在知悉该证据缺口后明确要求“不保留尾巴”，并授权
`OWNER_OVERRIDE / RETIRE`。本次据此删除本地 10 条 stateless Run 合同；剩余风险是
仍可能存在当前不可观察的外部调用方。该风险接受不影响本地 Workbench UI 回归结论，
也不改变未来类似公共合同退役默认仍应补齐外部证据的规则。
