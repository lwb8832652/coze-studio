# Workbench 自适应执行 MVP P0A Atomic Mutation Dispatcher

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development`（推荐）或
> `superpowers:executing-plans` 执行。每次只加载本文件、唯一 active 子包及其明确列出的源码。

**Goal:** 依次证明现有表能锁定 Run/Attempt、原子提交 Event/Checkpoint/真实 PlanItem mutation，
并在 disposable MySQL 并发下只有一个 winner。

**Architecture:** P0A 再拆成三个串行小包。当前只展开 P0A1；P0A2/P0A3 必须从前一包的真实
HEAD 生成，不能提前设计或实现。

---

## 1. 子包状态机

| 子包 | 状态 | 只回答的问题 | 退出门 |
| --- | --- | --- | --- |
| P0A1 contract/fence | `ready` | typed request 能否用现有字段稳定拒绝无效 identity、失效 Run lease 与漂移 Attempt？ | contract、Run fence、Attempt fence 的 RED/GREEN 和精确错误全部通过 |
| P0A2 SQLite atomic mutation | `locked` | 同一 SQLite transaction 能否提交 Event、Checkpoint、PlanItem 内容与 Plan revision，失败时全回滚？ | happy/rollback/lineage/Item CAS 全通过 |
| P0A3 MySQL single winner | `locked` | 两连接并发修改同一 PlanItem 时能否线性化为唯一完整 winner？ | disposable MySQL 连续两轮 PASS、零 SKIP/fail |

当前可激活细节包：
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a1-fence.md`

后续包只在上一包 PASS 后创建：

```text
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a2-sqlite-atomic.md
docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0a3-mysql-race.md
```

## 2. 统一规则

- [ ] 同时只有一个子包可为 `active`；其余必须保持 `locked`。
- [ ] 一个 checkbox 只能是写一个测试、运行一次 RED、写一个最小实现、运行一次 GREEN、
      格式检查或提交中的一种动作。
- [ ] 所有失败断言必须使用精确 typed error；禁止只断言 `err != nil`。
- [ ] 每个 GREEN 后读取数据库验证目标行和非目标行；禁止只看 RowsAffected。
- [ ] 每个子包提交后机器验证 base/head 祖先关系、文件 allowlist、clean worktree 和日志 SHA。
- [ ] 子包状态只写入 `/private/tmp` evidence manifest，不修改 dispatcher 或 active plan 文档。
- [ ] 任一子包失败、超时或需要 migration/endpoint/route/第二 finalizer 时停止，不生成下一包。
- [ ] P0A 总时限计入 P0 的 2 engineer-days，不因继续拆包而延长。

## 3. P0A2 冻结退出合同

P0A1 PASS 后生成 P0A2 时，必须逐项拆成独立 RED/实现/GREEN：

- [ ] 定义 repository interface/result 和 `AdaptivePlanMutation`；1～32 个 item，
      `next_revision=expected+1`，每个 `next_item.version=expected+1`。
- [ ] initial Attempt 的 `SourceAttemptID/SourceCheckpointID` 必须同时为 NULL，且
      `PlanScopeRunID=ExecutionRunID`；recovery Attempt 的两字段必须同时非 NULL。
- [ ] recovery 时锁 source Attempt 并按 `SourceCheckpointID` 读取 source checkpoint；只允许
      `sourceAttempt.ExecutionRunID=sourceCheckpoint.RunID`。再锁该 source physical Run，要求
      checkpoint metadata 的 `execution_generation` 与 Run 当前 generation 相等；
      `PlanScopeRunID` 必须等于 checkpoint metadata 继承的 `plan_scope_run_id`，不能强制等于
      source physical Run。Thread、Journal、attempt、Plan revision 全部一致；同 Thread 的无关 Run
      必须拒绝。
- [ ] 增加“同 Thread 但不是 `SourceAttemptID/SourceCheckpointID` canonical source”的独立 RED，
      精确断言 lineage conflict 且所有表不变。
- [ ] 增加 A 持有 Plan、B 从 A 恢复、C 再从 B 恢复但继续使用 A Plan scope 的多跳正例；另加
      source Run generation 已变化的 stale-generation RED，精确断言 lineage conflict。
- [ ] happy path 在一个 transaction 写 authoritative RunEvent、versioned runtime Checkpoint、
      PlanItem 完整内容与 Plan revision。
- [ ] RunEvent 无条件带 `(journal_run_id, attempt_id, idempotency_key)`；Journal 只做可降级投影。
- [ ] event sequence 从已锁 Attempt 的 `NextSequence` 分配；写 Event/Checkpoint/Plan 后，用
      `(attempt id, active_slot, next_sequence, last_committed_sequence)` 条件 CAS
      `LastCommittedSequence`。CAS 失败时整个 transaction 回滚。
- [ ] checkpoint metadata 固定 event ID/sequence、Attempt identity、Plan scope/revision 和 item
      fingerprint，并固定写入 physical execution Run ID 与 execution generation；读取时必须满足
      `checkpoint.sequence <= LastCommittedSequence` 且 source generation 未漂移。
- [ ] checkpoint 唯一键冲突、stale Plan revision、stale item version、跨 Thread 与 scope 漂移
      分别使用精确错误并证明 Event/Checkpoint/Plan/Items 全不变。
- [ ] 锁序固定为 physical Run → Attempt → Plan scope Run → Plan → sorted Items。

## 4. P0A3 冻结退出合同

P0A2 PASS 后生成 P0A3 时：

- [ ] 在写实现前先用现有 MySQL integration test 证明 DSN/DDL gate 可用，零 SKIP。
- [ ] 生产 DDL helper 只按迁移顺序加载
      `20260617000100_agent_checkpoints.sql`、
      `20260619000100_agent_checkpoint_runtime_keys.sql`、
      `20260620000400_agent_run_plans.sql` 及其既有前置迁移；禁止 `AutoMigrate`。
- [ ] 两个独立连接使用 channel barrier 修改同一 Plan/item expected version；禁止 sleep。
- [ ] 断言一个完整 winner、一个 typed conflict，且 loser 没有 Event/Checkpoint/PlanItem 残留。
- [ ] 连续两轮使用 `go test -json`；每轮机器断言
      `TestAdaptiveExecutionBoundaryMySQL/ConcurrentPlanItemMutationHasSingleWinner` 的
      `Action=pass` 精确出现一次、`skip/fail` 为零。零匹配或额外同名 PASS 都失败。
- [ ] 两轮通过后再运行 repository package、`gofmt`、diff check。

## 5. P0A 总退出门

- [ ] P0A1～P0A3 全部 PASS，提交形成线性祖先链。
- [ ] 实际变化包含 PlanItem 内容，不是空 Plan revision bump。
- [ ] SQLite rollback 和 MySQL loser 都没有部分写入。
- [ ] 无 migration、ADK、IDL、frontend 或 finalizer 修改。
- [ ] P0A evidence 含每包 base/head SHA、日志 SHA、文件 allowlist 和剩余风险。

全部满足后才把 P0A 记为 `PASS` 并从真实 HEAD 生成 P0B；否则 P0B～P5 保持 `locked`。
