# Workbench 自适应执行 MVP P0 Spike Dispatcher

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development`（推荐）或
> `superpowers:executing-plans` 执行。每次只加载本文件、唯一 active 子包及其明确列出的源码；
> 不预读 locked 子包涉及的 ADK、UI 或评测实现。

**Goal:** 在 2 engineer-days 硬时限内，依次证明零迁移原子写、幂等恢复、唯一成功终态和真实
MySQL race；任一子包失败就停止整个 MVP。

**Architecture:** P0 不是一个大实现任务，而是四个串行证据包。只有 P0A 现在展开；P0B～P0D
由前一包的真实 HEAD 和证据生成。P0 全部通过前，P1～P5 保持 locked。

---

## 1. 子包状态机

| 子包 | 状态 | 只回答的问题 | 退出门 |
| --- | --- | --- | --- |
| P0A atomic mutation | `ready` | 现有表能否在一个 MySQL transaction 中 fence Run/Attempt 并原子写 typed event、checkpoint、真实 PlanItem mutation？ | SQLite happy/rollback + MySQL concurrent mutation 单 winner |
| P0B replay/recovery | `locked` | Journal projection 降级后能否仍按权威 tuple 幂等重放，并用现有 repository 恢复？ | lost response、duplicate、degraded projection、repository reload 全通过 |
| P0C verified success | `locked` | 现有唯一 `FinalizeRunSuccess` 能否把 passed verification 与 succeeded 同事务提交？ | success/rollback/cancel race 全通过，无第二 finalizer |
| P0D final race/evidence | `locked` | lease takeover、cancel、crash retry 和证据绑定能否在真实 MySQL 稳定成立？ | 两轮 4 子测试、零 fail/skip、clean allowlist diff |

当前 active 细节包：
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a-atomic.md`

后续包只在上一包 PASS 后创建：

```text
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0b-recovery.md
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0c-terminal.md
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0d-evidence.md
```

## 2. 统一加载与失败规则

- [ ] 同时只有一个子包可以为 `active`；其余保持 `locked`。
- [ ] 子包每一步是一个 2～5 分钟动作；每个行为改动先有失败测试，再做最小实现。
- [ ] 子包通过后先提交，记录 base/head SHA、测试日志 hash、剩余风险，再从该 HEAD 写下一包。
- [ ] 子包状态只记录在 `/private/tmp/workbench-adaptive-mvp-*` evidence manifest；不得回写本计划。
- [ ] 下一子包 plan 必须按 master 的 docs-only handoff 单独提交；下一代码包从该 docs commit 的 clean
      HEAD 开始，不能把未提交计划文件夹进实现提交。
- [ ] 任一子包出现 migration、新 endpoint/route、第二 worker/executor/finalizer 或 ad-hoc JSON 双写，
      立即记为 `FAIL`，不自动扩 scope。
- [ ] 任一真实 MySQL 测试缺 DSN、DDL gate 不符、数据库名不含 `agentthread_disposable` 或 SKIP，
      记为 `BLOCKED`，不能用 SQLite 代替。
- [ ] P0 总时限超过 2 engineer-days 时停止；不以砍掉 terminal/recovery/race 证明换取 PASS。
- [ ] 004/005 实验迁移和测试永远不是 P0 输入，不复制、不 cherry-pick。

## 3. 后续子包的冻结退出合同

### P0B replay/recovery

生成时必须逐步覆盖：

- [ ] 无条件将 `(journal_run_id, attempt_id, idempotency_key)` 写入权威 RunEvent 行；projection
      degraded 不能删除该 tuple。
- [ ] 为 adaptive authority 增加窄的 repository replay/readback 方法，直接按上述 tuple 读取
      `runEventPO` 的 event ID/sequence/idempotency 和 checkpoint identity；公共 `ListRunEvents`
      仍是安全投影，不能作为幂等或恢复 authority。
- [ ] lost-response retry 返回原 event/checkpoint/Plan/Items；payload 或 mutation fingerprint 漂移
      fail closed。
- [ ] duplicate decision 冲突、duplicate verification replay 各有独立测试。
- [ ] checkpoint 只能通过 boundary 返回的精确 ID 或 recovery Attempt 的 `SourceCheckpointID`
      读取；必须校验 checkpoint sequence 不高于 Attempt `LastCommittedSequence`，不得靠
      `ListCheckpoints` 猜 latest。
- [ ] `GetCheckpoint`、`GetPlan`、`ListPlanItems` 继续用于实体兼容性读回；恢复结果必须同时包含
      execution/journal/attempt/generation、source lineage、Plan scope/revision 和 item fingerprint。

### P0C verified success

生成时只允许修改现有 `FinalizeRunSuccess` 路径：

- [ ] `FinalizeRunSuccessRequest` 增加可选 typed adaptive gate；nil 保持 gate-off 行为。
- [ ] 同一 transaction 重校 active attempt、decision、current Plan/Items、passed verification、
      evidence high-watermark 和 terminal checkpoint ref。
- [ ] verification event 必须排在 completion event 前；任一漂移使 Run/Message/events/checkpoint
      全回滚。
- [ ] 禁止新增第二 finalizer；cancel 与 verified success 只能线性化出一个终态。

### P0D final evidence

生成时必须使用 `go test -json` 和机器断言：

- [ ] MySQL 子测试精确为 concurrent PlanItem mutation、lease takeover、cancel vs verified success、
      crash-after-commit retry；连续两轮各 4 项 PASS、零 SKIP/fail。
- [ ] SQLite/P0 tests 全部 PASS，工作树干净。
- [ ] `P0_BASE_SHA..P0_HEAD_SHA` 只允许四个新 adaptive repository 文件、`repository.go`、
      `mysql.go` 和确有必要的 `journal.go` 聚合行；packet plan 文件只允许出现在经过单文件
      diff-tree 校验的 docs-only commits，并在产品代码 allowlist 计算中显式过滤。
- [ ] 每个 packet-doc commit 的 parent/child SHA 形成线性链，subject 与唯一新增 plan 路径精确匹配；
      任一混合 code+docs commit 都使 P0 失败。
- [ ] 只有测试数量、结果、allowlist 和日志 SHA 全部机器校验后才能写 `P0_RESULT=PASS`。

## 4. P0 总退出门

- [ ] P0A～P0D 全部 PASS，且各自提交 SHA 形成线性祖先链。
- [ ] 原子边界提交真实 PlanItem 内容，不是空 revision bump。
- [ ] recovery target 可继续使用同 Thread 的合法 `plan_scope_run_id`，不强制 Plan 属于当前物理 Run。
- [ ] Journal 只是可降级安全投影，不能决定权威幂等或恢复。
- [ ] passed verification 与 succeeded 只由现有 finalizer 同事务提交。
- [ ] migration 清单为空；未加载 ADK coordinator、公共投影或 UI 实现。

只有以上全部满足，才把总控 P0 记为 `PASS` 并按真实 P0 HEAD 生成 P1 decision 包。否则停止，
返回单张紧凑失败表：子包、失败断言、证据 SHA、最小设计选择；不进入 P1。
