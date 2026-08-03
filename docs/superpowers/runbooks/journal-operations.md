# Journal Operations Runbook

本手册定义 Coze Journal 的生产开关、监控、回滚、保留清理和恢复处置。产品和
交互基线为已验收的 v26 原型；本期不包含客服诊断、工单授权或第三方代操作。

## 当前上线状态

- `journal_projection`、`journal_ui`、`journal_snapshots`、
  `checkpoint_recovery` 默认全部关闭。
- 只有 Pro、Ultra 的根 Task Run 参与稳定空间灰度；Flash、Thinking、子 Run 和
  subagent 不创建 Journal。
- 直接回答或只有生命周期事件的简单任务不展示空 Journal，也不伪造步骤。
- Document、Code、Skill 已有受信生产边界；Terminal 和 Browser 尚无满足合同的
  完整生产数据源。因此 `journal_snapshots` 必须保持关闭，FR-2.3 和外部 5% 灰度
  当前均不得宣称通过。
- `checkpoint_recovery` 只有在副作用分类、故障注入和重复副作用为零的证据完成后
  才可单独开启。

## 值班责任

以下信息必须在外部灰度审批单中实名填写，并由当班人员完成一次回滚演练。任一项
为空都阻断外部灰度；本仓库不保存个人手机号或临时群链接。

| 职责 | 审批单必填内容 |
| --- | --- |
| Journal 服务 Owner | 姓名、值班系统身份、主备关系 |
| 告警通知 | 正式通知群和 paging 服务 |
| 升级链 | P1、P0 的服务、数据库、对象存储和安全联系人 |
| 安全审批 | 安全审批人和事件编号 |
| 回滚执行 | 当班执行人与复核人 |

## 控制面与开关

Journal 开关保存在 Basic Configuration，使用服务端 CAS 更新，禁止直接修改数据
库。先由系统管理员读取 `/api/admin/config/basic/get` 的 `revision`，再向
`/api/admin/config/basic/save` 提交完整 `journal_runtime_configuration` 和相同的
`expected_revision`。冲突返回 `BASE_CONFIG_VERSION_CONFLICT`，必须重新读取后
再判断，不能盲目重试覆盖。

四个开关及稳定空间桶字段如下：

| 能力 | 主开关 | 灰度字段 |
| --- | --- | --- |
| 事件投影 | `journal_projection` | `journal_projection_rollout_basis_points` |
| Journal 页面 | `journal_ui` | `journal_ui_rollout_basis_points` |
| 五视图快照 | `journal_snapshots` | `journal_snapshots_rollout_basis_points` |
| Checkpoint 恢复 | `checkpoint_recovery` | `checkpoint_recovery_rollout_basis_points` |

灰度值范围是 0 到 10000 basis points。主开关为 false 或灰度值为 0 都表示关闭。
配置缓存最长 30 秒；生产环境开启任一能力前会校验 Redis readiness，失败时拒绝
保存。默认容量参数是租户 SSE 32、集群 SSE 4096、发送队列高水位 128、最大
256、短请求 QPS 20、burst 40、lease 90 秒、快照分片阈值 4 MiB。这些只是关闭
状态下的保护值，必须由 2 倍峰值压测冻结后才能用于外部灰度。

## 放量顺序

1. 在生产等量脱敏副本完成 nullable migration 演练，记录 DDL algorithm、
   metadata lock wait、写入 P99 和 replica lag。
2. 部署兼容服务端，保持四个开关关闭；再部署兼容前端。
3. 仅为内部空间开启 projection 和 UI。只有五类 producer、权限、安全和容量证据
   完整后，才可为同一批空间开启 snapshots。
4. 内部验证通过后进入 5%，至少观察 3 天；recovery 保持关闭。
5. 5% 无 P0/P1、事件完整率至少 99.99%、任务主链路不受影响后进入 25%。
6. 25% 至少观察 7 天，快照成功率至少 99.9%、重复副作用为零后，recovery 才能
   只对白名单任务开启。
7. 安全复审、容量复核和回滚演练完成后才能评估全量。

百分比变更必须小步进行，四个能力分别审批。禁止只提高 UI 灰度而让 projection
覆盖不足，也禁止让 snapshots 或 recovery 的灰度超过其依赖能力。

## Kill Switch 与回滚

出现越权、敏感泄漏、重复副作用、任务主流程受影响，或达到严重告警阈值时，按
以下顺序把主开关和 rollout 同时设为关闭：

1. `checkpoint_recovery=false`，rollout 设为 0。
2. `journal_snapshots=false`，rollout 设为 0。
3. `journal_ui=false`，rollout 设为 0。
4. `journal_projection=false`，rollout 设为 0。
5. 确认新请求在 30 秒缓存窗口后不再进入对应能力，再回滚前端和服务端。

回滚不删除或回填已经写入的 RunEvent、Attempt、Snapshot、Checkpoint 或 Ledger。
设计目标为 RTO 不超过 15 分钟、已提交事件和快照 RPO 为 0；只有回滚演练报告
通过后才可对外宣称达到目标。

## 指标与告警

生产设置 `AGENT_JOURNAL_PROMETHEUS_METRICS_ENABLED=true` 后暴露以下低基数
指标。允许标签只有 `version`、`rollout_cohort`、`task_type`、
`client_version`、`result`、`error_code`，不得加入正文、命令、代码、URL、用户、
空间、Run、Attempt、Trace 或 Event ID。

| 指标 | 用途 |
| --- | --- |
| `coze_journal_event_visible_total` | 已唯一渲染事件数 |
| `coze_journal_event_visible_latency_ms` | 事件提交到可见延迟 |
| `coze_journal_completed_tasks_total` | 入组完成任务的 complete/incomplete 分母 |
| `coze_journal_sse_active` | 活跃 Journal SSE lease |
| `coze_journal_sse_signals_total` | slow_consumer/backfill 信号 |
| `coze_journal_snapshot_requests_total` | 快照请求结果 |
| `coze_journal_snapshot_latency_ms` | 快照响应延迟 |
| `coze_journal_recovery_results_total` | 恢复请求结果 |
| `coze_journal_retention_backlog` | 待清理记录数 |

| 信号 | 预警 | 严重/P0 | 处置 |
| --- | --- | --- | --- |
| 事件完整率，5 分钟 | <99.995% | <99.99% | 停止扩大；严重时关闭 UI 新流量，保留任务主链路 |
| 快照成功率，5 分钟 | <99.95% | <99.9% | 关闭 snapshots，回退任务摘要 |
| SSE 相对压测容量 | >70% | >85% | 限流、断开慢消费者，客户端转 polling |
| 重复副作用 | 不适用 | 任意 1 次为 P0 | 立即关闭 recovery，冻结对应 Attempt |
| 越权或敏感泄漏 | 不适用 | 任意 1 次为 P0 | 立即关闭 projection 及读取能力，启动安全事件流程 |

完整率和快照成功率的自动判定至少需要 100 个样本；安全事件不设最小样本。告警
恢复后也不能自动重新开闸，必须经过人工复核和 CAS 变更。

## 事件处置

### 越权或敏感内容

1. 关闭 projection、snapshots、UI 和 recovery，保留任务执行主链路。
2. 撤销相关 Artifact/Snapshot capability；停止签发新 URL，不在工单中粘贴正文。
3. 用访问审计中的资源标识、动作、结果和时间定位范围；禁止导出快照正文到日志。
4. 按安全事件流程评估对象存储、缓存和客户端暴露，完成复核后再决定数据清理。

### 重复副作用或恢复异常

1. 立即关闭 recovery，不重试同一恢复请求。
2. 按 task、root run、attempt 和 ledger 检查幂等状态，保留现场记录。
3. 确认外部副作用后由领域 Owner 处置；不得通过删除 Ledger 重新放行。
4. 只有故障注入和并发恢复测试重新通过后才可恢复白名单。

### 事件缺口或 SSE 压力

1. 检查 incomplete 任务、gap、backfill 和 slow_consumer 指标。
2. SSE 到 70% 时停止扩大灰度；到 85% 时关闭新 UI 流量并确认 polling 降级。
3. 核对 RunEvent 写入、Attempt sequence 和客户端 cursor，不做全表回填猜测。

### 快照错误

1. 关闭 snapshots，任务摘要和事件流继续可用。
2. 区分 failed、no_permission、expired/`SNAPSHOT_UNAVAILABLE`；权限拒绝不进入
   快照成功率分母。
3. 检查对象提交、数据库 commit marker、fragment 和访问审计；禁止返回临时对象
   地址绕过 capability。

## 保留与清理

Journal retention worker 默认关闭。生产依赖和对象存储准备完成后设置：

```bash
AGENT_JOURNAL_RETENTION_WORKER_ENABLED=true
AGENT_JOURNAL_RETENTION_INTERVAL_MS=3600000
AGENT_JOURNAL_RETENTION_BATCH_SIZE=100
AGENT_JOURNAL_RETENTION_LEASE_SECONDS=300
AGENT_JOURNAL_RETENTION_STAGING_GRACE_MINUTES=60
```

固定保留期为 Snapshot/Checkpoint 30 天，RunEvent/Attempt/Ledger 90 天。每轮先
claim 并清理快照对象和过期 staging prefix，再删 checkpoint、event/ledger，最后
删除未被引用的 attempt。活跃 Run/Attempt 不清理；对象 key 必须位于当前 tenant
前缀。worker 开启但数据库或对象存储不可用、配置越界时服务启动失败，不能静默
跳过。

`coze_journal_retention_backlog` 持续增长时先暂停扩大灰度，检查对象删除错误和
claim lease；不要手工删除数据库行或对象。重跑 worker 利用 claim token 和过期
lease 收敛。任务/线程删除后的 capability 撤销优先于异步物理清理。

## 发布证据

每次灰度审批至少附带：

- v26 原型 hash 与需求追踪矩阵；客服范围移出记录。
- 新服务端+旧前端、旧服务端+新前端、新服务端+新前端兼容结果。
- 五类真实 producer E2E；缺一项时 snapshots 保持关闭。
- 权限、脱敏、恶意内容、撤权和访问审计矩阵。
- 2 倍峰值容量报告和最终冻结参数。
- migration rehearsal、回滚演练、值班责任和本轮 CAS revision。
- 后端测试、前端 test/lint/build、Atlas validate 和 in-app browser 验收结果。
