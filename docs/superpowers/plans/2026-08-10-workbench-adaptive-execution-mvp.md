# Workbench 自适应执行 MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development`（推荐）或
> `superpowers:executing-plans` 逐任务执行。执行前必须先使用
> `superpowers:using-git-worktrees` 建立隔离工作树。所有可执行步骤使用
> checkbox（`- [ ]`）跟踪。

**Goal:** 在现有 Workbench composer → canonical Thread/Run → Eino ADK →
TaskDetail 主链内增加可恢复的 decision、progress、verification 最小闭环，并用同一候选
SHA 的 gate-off/on 评测证明复杂任务收益且不损伤简单任务。

**Architecture:** 不新增页面、HTTP route、worker、executor 或第二套模型循环。normal 与
resume 分支在同一个 `ADKExecutor` 内复用 adaptive coordinator；三个类型化事实复用现有
RunEvent、Checkpoint、Plan、Journal 和公共 SSE 投影。实施采用门禁驱动的渐进展开：本文件
固定完整路线和每阶段的小步骤索引，只有当前阶段的执行包展开到 2～5 分钟原子步骤。

**Tech Stack:** Go、GORM/MySQL、Hertz/Thrift、Eino ADK、React、TypeScript、Rush.js、
Vitest、Codex in-app browser、Workbench execution graph tooling。

---

## 0. 计划加载规则

**权威规格：**
`docs/superpowers/specs/2026-08-10-workbench-adaptive-execution-mvp-design.md`

**当前 docs-only 设计提交：** `6500c8b9e379d05693bada1ca627a5937f8841c3`

**为什么拆包：** 旧计划一次性展开 27 个大任务，把未验证的迁移、运行时、UI 和发布假设都
提前写死，导致计划本身接近实现规模。本计划先验证最危险的零迁移事务假设，再按已经通过的
代码状态加载下一包。

执行纪律：

- [ ] 同一时间只能有一个 `active` 执行包；后续包保持 `locked`。
- [ ] 每个 active 包的每一步只能包含一个 2～5 分钟动作：写失败测试、运行 RED、写最小实现、
      运行 GREEN、提交。
- [ ] active 包必须列出精确文件、符号、代码、命令和预期结果；禁止 `TBD`、`TODO`、
      “类似上一任务”或泛化的“补错误处理”。
- [ ] 当前包退出门通过并提交后，记录 `PACKET_BASE_SHA`、测试输出摘要和风险，再从该 SHA
      生成下一包；不能从本实验工作树的未提交文件推断未来实现。
- [ ] `ready/active/PASS/FAIL/BLOCKED` 运行状态只写入 `/private/tmp` evidence manifest；不得为切换
      状态修改已批准 plan 文档或制造工作树脏文件。
- [ ] 任一包未在 timebox 内满足退出门时，状态只能记为 `FAIL` 或 `BLOCKED`；后续包继续
      `locked`，停止并只返回失败证据。不得自动扩 scope、跳 gate 或把未完成项挪到验收阶段。
- [ ] 一个包内发现需新增 migration、endpoint、route、worker、executor 或模型循环时立即停止，
      回到规格评审，不自动扩大下一包。
- [ ] 每个提交只暂存该小任务列出的精确文件；发现用户或并行改动时停止，不回滚、不夹带。
- [ ] 004/005 实验测试和迁移不属于 MVP 输入，不能 cherry-pick 或复制到实施分支。

按需生成下一执行包时，统一使用 docs-only handoff：

- [ ] 从上一代码包的已提交 HEAD 创建唯一下一包 plan 文件，运行 Bash fence 语法与 whitespace gate。
- [ ] 单独暂存这一个 plan 文件，提交 subject 精确为
      `docs: add adaptive execution PACKET_ID packet`；`PACKET_ID` 必须是状态表中的精确 ID
      （如 `P0A2`），该提交不得含产品代码或其它文档。
- [ ] 机器验证 docs commit 的 parent 等于上一代码包 HEAD、diff 精确一个预期文件、工作树 clean。
- [ ] 下一代码包的 `PACKET_BASE_SHA` 记录为 docs commit HEAD。最终产品代码 allowlist 忽略这些已独立
      审计的 packet-doc commits，但必须另行验证每个 docs commit 的单文件范围和线性祖先关系。

执行包清单：

| 包 | 状态 | 入口门 | 退出门 |
| --- | --- | --- | --- |
| P0 分支与原子事务 spike | `ready` | 用户批准规格和本计划 | disposable MySQL 证明 fence、attempt、Plan/Item、event、checkpoint、恢复、幂等及 verified success 单事务成立 |
| P1 decision 纵切 | `locked` | P0 PASS | direct/multi-step 正向链、normal/resume 恢复、最小 TaskDetail 投影通过 |
| P2 progress/verification 闭环 | `locked` | P1 PASS | repair/replan/clarify/stop、成功门禁、取消/恢复/安全矩阵通过 |
| P3 公共投影与当前页面完成态 | `locked` | P2 PASS | list/SSE 等价、未知版本 fail closed、当前页面 Vitest/browser 通过 |
| P4 评测与候选冻结 | `locked` | P3 PASS、30 开发集净增至少 3/30 | 同 SHA 80 holdout、安全门和全部工程门通过 |
| P5 本地 `dev` 集成 | `locked` | 用户确认候选 exact SHA 与范围 | 仅本地 `ff-only`，不 push、不发布 |

首个 active 包保存在：
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0-spike.md`。

### 工作量护栏

- P0 是唯一立即展开的高风险验证，硬时限为 2 engineer-days；它失败就停止，后续工作量归零。
- P0 通过后按 Week 1 技术纵切、Week 2 完整闭环、Week 3 冻结验收推进。基准配置为 4 名专职
  FTE；只有 3 名时按 3～4 周排期，不以删测试换取两周口径。
- 后续每个执行包只展开到当周可提交边界，原则上控制在约 200～350 行；超出时按独立 gate 再拆包，
  不能把多个共享状态写任务并发化。
- 非 MVP：新页面/route、第二套 worker/executor、Subagent、多模型 classifier/planner/verifier 循环、
  默认 migration、深层 options 管理、发布自动化改造。发现这些需求只记录，不进入当前包。
- 最快效果验证点不是 UI 完成，而是 P0 的真实 MySQL 原子性和 P1 的 direct/multi-step 小型纵切；
  任何一个不成立都不继续堆页面和评测工程。

## 1. P0：隔离分支与 Day-2 原子事务 spike

### Task 1：冻结输入并创建干净实施工作树

**Files:**

- Read: `docs/superpowers/specs/2026-08-10-workbench-adaptive-execution-mvp-design.md`
- Read: `docs/superpowers/runbooks/dev-integration-audit.md`
- Read: `docs/superpowers/context/workbench-execution-chain.md`
- Create in clean branch by cherry-pick: this plan and the approved design only

- [ ] **Step 1: 记录实验工作树，不修改它。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-execution-implementation
  git branch --show-current
  git rev-parse HEAD
  git status --short
  ```

  Expected: branch 为 `codex/workbench-adaptive-execution-implementation`；输出保留现有
  `mysql_journal.go` 修改及 00250/00300、schema/integration 测试草稿，且不执行清理命令。

- [ ] **Step 2: 只读取得最新基准。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio
  git fetch origin dev
  AUDITED_ORIGIN_DEV_SHA="$(git rev-parse origin/dev)"
  test -n "$AUDITED_ORIGIN_DEV_SHA"
  printf '%s\n' "$AUDITED_ORIGIN_DEV_SHA" > \
    /private/tmp/workbench-adaptive-mvp-origin-dev.sha
  ```

  Expected: 得到非空 `AUDITED_ORIGIN_DEV_SHA`；不改变本地 `dev`。

- [ ] **Step 3: 创建隔离 worktree。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio
  AUDITED_ORIGIN_DEV_SHA="$(tr -d '\n' < /private/tmp/workbench-adaptive-mvp-origin-dev.sha)"
  test -n "$AUDITED_ORIGIN_DEV_SHA"
  test ! -e .worktrees/workbench-adaptive-mvp
  git worktree add .worktrees/workbench-adaptive-mvp \
    -b codex/workbench-adaptive-mvp "$AUDITED_ORIGIN_DEV_SHA"
  test "$(git -C .worktrees/workbench-adaptive-mvp rev-parse HEAD)" = \
    "$AUDITED_ORIGIN_DEV_SHA"
  ```

  Expected: 新工作树分支为 `codex/workbench-adaptive-mvp`，起点精确等于刚记录的
  `origin/dev`。

- [ ] **Step 4: 只带入 docs-only 提交。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  git cherry-pick 6500c8b9e379d05693bada1ca627a5937f8841c3
  PLAN_DOCS_COMMIT_SHA="$(
    git rev-list --all --max-count=1 --fixed-strings \
      --grep='docs: add adaptive execution MVP implementation plan'
  )"
  test -n "$PLAN_DOCS_COMMIT_SHA"
  test "$(git log -1 --format='%s' "$PLAN_DOCS_COMMIT_SHA")" = \
    "docs: add adaptive execution MVP implementation plan"
  git diff-tree --no-commit-id --name-only -r "$PLAN_DOCS_COMMIT_SHA" | \
    awk 'BEGIN { count = 0 } { count++; if ($0 !~ /^docs\/superpowers\/plans\//) bad = 1 } END { exit count == 0 || bad }'
  git cherry-pick "$PLAN_DOCS_COMMIT_SHA"
  ```

  Expected: 两个提交都只含 `docs/superpowers/specs/**` 或
  `docs/superpowers/plans/**`；第二个提交的 subject 精确为
  `docs: add adaptive execution MVP implementation plan`。若检索为空或文件范围不符则停止。

- [ ] **Step 5: 证明没有带入实验实现。**

  ```bash
  set -euo pipefail
  cd /Users/liuwenbo/code/BuildingAI/coze-studio/.worktrees/workbench-adaptive-mvp
  AUDITED_ORIGIN_DEV_SHA="$(tr -d '\n' < /private/tmp/workbench-adaptive-mvp-origin-dev.sha)"
  test -z "$(git status --porcelain)"
  test ! -e backend/domain/agentthread/repository/mysql_adaptive_execution_schema_test.go
  test ! -e backend/domain/agentthread/repository/mysql_adaptive_execution_migration_integration_test.go
  test ! -e docker/atlas/migrations/20260808000250_agent_side_effect_request_summary_check.sql
  test ! -e docker/atlas/migrations/20260808000300_agent_run_attempts_multi_attempt.sql
  test ! -e docker/atlas/migrations/20260808000400_agent_run_execution_contract.sql
  git diff --name-only "$AUDITED_ORIGIN_DEV_SHA...HEAD" | \
    awk 'BEGIN { count = 0 } { count++; if ($0 !~ /^docs\/superpowers\/(specs|plans)\//) bad = 1 } END { exit count != 5 || bad }'
  ```

  Expected: diff 精确为 1 份批准的 design 和 4 份 plan 文档；任何产品代码、00250/00300、
  004/005 实验或额外文件都会使命令失败。

### Task 2：执行 P0 spike 包

**Files:**

- Follow: `docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0-spike.md`

- [ ] **Step 1: 在外部 evidence manifest 记录 P0=`active` 和 `PACKET_BASE_SHA`，不修改 plan 文档。**
- [ ] **Step 2: 严格按 P0 包先写失败测试并保存 RED 输出。**
- [ ] **Step 3: 只实现足以让 spike 通过的 repository transaction，不接 ADK/UI。**
- [ ] **Step 4: 运行 SQLite 语义/恢复测试和 disposable MySQL 双连接并发测试。**
- [ ] **Step 5: 对 crash/retry/replan/lease/cancel/duplicate/verified-success 矩阵逐项记录单一结果。**
- [ ] **Step 6: 运行 package 回归、`gofmt`、`git diff --check`。**
- [ ] **Step 7: 提交 P0；记录 `P0_RESULT=PASS|FAIL` 和提交 SHA。**

P0 为 `FAIL` 时，只允许：

- [ ] 保存失败测试和证据；
- [ ] 把 P1～P5 保持 `locked`；
- [ ] 返回“一张紧凑表”的修订规格评审；
- [ ] 不增加 004/005 表、不调用后续实现包。

## 2. P1：decision 最小纵切（P0 PASS 后加载）

**加载时必须创建：**
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p1-decision.md`

P1 小步骤索引；每一项在 P1 文件中继续拆成 RED/最小实现/GREEN/commit：

- [ ] 由独立评测负责人封存 80 holdout/evaluator，记录 seal hash；实现侧只获得 schema、计数和
      30 开发集，P4 只能复核同一 hash，不能重新冻结。
- [ ] 冻结 feature eligibility、Journal enrollment、gate-off reason 与 admission metrics 的
      服务端合同，并证明 gate-off 行为不变。
- [ ] 定义 `ExecutionDecision`、`ProgressEvaluation`、`VerificationResult` 三个内部类型、codec、
      有界 schema 与公共版本 DTO；本包只把 decision 接入运行时。
- [ ] 用 P0 transaction 各持久化并经现有 repository 读回/恢复一个三合同 fixture，先证明零迁移
      编解码与 checkpoint refs 可行，再接 decision coordinator。
- [ ] 为三种 decision 和 `single_step/multi_step` XOR 写 Go validation RED。
- [ ] 实现服务端 acceptance-check registry，强制模型提出项为 required。
- [ ] 在首次模型/工具调用前持久化 gate/schema/limits admission fact。
- [ ] 把 decision 通过 P0 transaction 写成 typed RunEvent + checkpoint refs。
- [ ] 在 `ADKExecutor.Execute` 接入 coordinator，但保留现有 RuntimeSelector。
- [ ] 在 `ADKExecutor.Resume` 恢复同一 coordinator 状态，不新建 resume 实现。
- [ ] 证明 `direct` 不创建 Plan，也不产生非验证 ToolStarted。
- [ ] 证明 `multi_step` 的 Plan persisted sequence 小于首个 ToolStarted sequence。
- [ ] 把 `payload_version` 从持久化事件传到 public list/SSE 和生成 TS 类型。
- [ ] 在现有 TaskDetail 只展示 planning/executing/final 最小状态，不新建 route/page。
- [ ] 运行 Week-1 技术纵切门：三个 typed fact 可读回/恢复、Plan 顺序正确、安全硬门为零。
- [ ] 提交 P1，记录 `P1_BASE_SHA/P1_RESULT/P1_HEAD_SHA` 后才解锁 P2。

## 3. P2：progress、verification 与安全闭环（P1 PASS 后加载）

**加载时必须创建：**
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p2-loop.md`

P2 小步骤索引：

- [ ] 为 `ProgressEvaluation` 的五种 recommendation 与字段 XOR 写 RED。
- [ ] 在 terminal tool result、里程碑、状态转换和 verification 前接入 evaluation。
- [ ] 对 evidence lineage、decision/Plan revision、generation/lease 做服务端重校。
- [ ] 实现最多 2 次 replan、2 次 verification repair、连续 3 次 no-progress。
- [ ] 把 24 tool calls 和 20 active minutes 按 canonical lineage 累积。
- [ ] 复用现有 human interaction 实现 clarify/resume，并继承 gate/schema/limits。
- [ ] 为 `VerificationResult` registry coverage、证据高水位和 stale invalidation 写 RED。
- [ ] 实现 `passed` 的同事务 terminal gate；无 passed verification 不得 succeeded。
- [ ] 接入只读验证工具 allowlist；任何写工具仍算非验证动作。
- [ ] 冻结全部工具的 deterministic read/write/verification 分类；未知工具 fail closed。
- [ ] 对 Sandbox target 做 canonical workspace/tenant 校验，并复用现有 SideEffect
      prepared/executing/terminal/replay 边界，禁止 adaptive 自建副作用账本。
- [ ] 实现稳定 block/error → 现有 Run status 映射。
- [ ] 覆盖取消、lease recovery、lost response、重复 side effect 和敏感投影。
- [ ] 运行 30 开发集 gate-off/on；净增不足 3/30 时最多一个 2 engineer-day 修复 cycle。
- [ ] 唯一修复 cycle 后仍低于净增 3/30 时把 P2 记为 `FAIL`，停止且不生成 P3。
- [ ] 提交 P2，记录 `P2_BASE_SHA/P2_RESULT/P2_HEAD_SHA` 后才解锁 P3。

## 4. P3：公共投影与当前 TaskDetail 完成态（P2 PASS 后加载）

**加载时必须创建：**
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p3-ui.md`

P3 小步骤索引：

- [ ] 为 `adaptive.decision/progress/verification` 的公共 allowlist 写后端 RED。
- [ ] 证明 internal discriminator 决定 `payload_version`，不按当前代码版本猜测。
- [ ] 证明 canonical list 与 SSE 对同一事件返回相同版本和安全 payload。
- [ ] 在 frontend adapter 保留 `payload_version`，未知版本不推进执行 reducer。
- [ ] 在现有 transcript 显示 clarify/final，在 Todo dock 显示 Plan revision。
- [ ] 在现有 Journal panel 映射 milestone/progress/verification terminal。
- [ ] 增加 planning/executing/repairing/replanning/verifying 状态与紧凑验证摘要。
- [ ] 保留 loading/empty/error/readonly/cancel/reconnect/keyboard/ARIA 行为。
- [ ] 用 Vitest 覆盖 direct 无空壳、多步更新、验证失败和 SSE reconnect。
- [ ] 用 in-app browser 验收现有 Workbench URL；记录账号/空间、可见状态和控制台错误。
- [ ] 更新 `workbench-execution-chain.md`、`workbench-execution-graph.json` 和必要的
      `workbench-chat.md`。
- [ ] 运行 execution graph verify/build/verify-derived 并提交 P3。

## 5. P4：评测、候选冻结与一次性验收（P3 PASS 后加载）

**加载时必须创建：**
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p4-acceptance.md`

P4 小步骤索引：

- [ ] 独立评测负责人校验 Week-1 封存的 holdout/evaluator hash，禁止重新冻结。
- [ ] 只用 30 开发集完成最后修复；实现侧不得读取 80 holdout 内容。
- [ ] fetch 最新 `origin/dev`，对齐后运行代码审查并冻结 `CANDIDATE_SHA`。
- [ ] 冻结 model/prompt/tool/catalog/sandbox/source snapshot 指纹。
- [ ] 对最终 SHA 再跑 gate-off 与最新 `origin/dev` 行为等价 smoke。
- [ ] 从相同干净 snapshot 成对交错运行 gate-off/on，禁止共享 cache/memory/artifact。
- [ ] 运行 60 个复杂任务，检查 on≥42 且 on-off≥6、每类 on≥12 且不低于 off-2。
- [ ] 对 20 个简单任务每 arm 运行 3 次，按 3/3 direct 计算 19/20。
- [ ] 计算 Run committed→公共终态可见的 nearest-rank P95 与平均 billable Token，比例≤1.10。
- [ ] 检查所有 expected multi-step run 均正确分类且 Plan 先于 ToolStarted，要求 100%。
- [ ] 运行 12 场景安全/取消/恢复/SSE/verification matrix，要求 100%。
- [ ] 运行 Go、Vitest、typecheck、lint、build、执行图和 browser 全部门禁。
- [ ] 生成含逐任务原始结果、paired bootstrap 区间、排除记录和全部 hash 的报告。
- [ ] 任一代码/依赖/基准/evaluator 变化即废弃证据并从候选冻结步骤重来。

## 6. P5：用户确认后的本地 `dev` fast-forward

**加载时必须创建：**
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p5-integrate.md`

- [ ] 报告 `AUDITED_ORIGIN_DEV_SHA`、`CANDIDATE_SHA`、文件范围、测试、migration 清单和风险。
- [ ] 等待用户对同一范围和 exact SHA 的明确确认。
- [ ] 再次 `git fetch origin dev`，证明远程基准和候选 SHA 未变化。
- [ ] 证明 migration 清单为空；若不为空，停止并单独申请 migration 授权。
- [ ] 按 `dev-integration-audit.md` 只把本地 `dev` fast-forward 到候选 SHA。
- [ ] 停止；不得把合并确认解释成 push、migration apply 或发布授权。

## 7. 总完成定义

- [ ] P0～P4 全部 `PASS`，没有跳过或事后补写的执行包。
- [ ] 现有 normal/resume 分支复用同一 adaptive coordinator。
- [ ] 没有新增页面、公共 endpoint、worker、executor、模型循环或默认 migration。
- [ ] `direct` 维持快速路径；multi-step Plan 前置；成功全部有当前 passed verification。
- [ ] 未授权写入、重复副作用、敏感公共投影均为 0。
- [ ] 80 holdout、12 场景矩阵、工程测试、执行图和当前页面验收绑定同一候选 SHA。
- [ ] 用户确认前不合并 `dev`；确认后也只执行本地 `ff-only`。
