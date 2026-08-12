# Workbench 自适应执行 MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use
> `superpowers:subagent-driven-development`（推荐）或
> `superpowers:executing-plans` 逐任务执行。执行前必须先使用
> `superpowers:using-git-worktrees` 建立隔离工作树。所有可执行步骤使用
> checkbox（`- [ ]`）跟踪。

**Goal:** 在现有 Workbench composer → canonical Thread/Run → Eino ADK →
TaskDetail 主链内彻底退休 `auto/pro/ultra/requested_policy` 产品执行模式，以 typed admission 和
`ExecutionDecision` 建立唯一任务形态权威，再增加可恢复的 progress、verification 最小闭环，
并用同一候选 SHA 的 gate-off/on 评测证明复杂任务收益且不损伤简单任务。

**Architecture:** 不新增页面、HTTP route、worker、executor 或第二套模型循环。normal 与
resume 分支在同一个 `ADKExecutor` 内复用 admission/decision coordinator；gate-on/off 都持久化
typed admission 与 `ExecutionDecision`，只有 gate-on 启用 progress/replan/verification。三个
类型化事实复用现有 RunEvent、Checkpoint、Plan、Journal 和公共 SSE 投影；旧模式只允许在隔离
legacy decoder 中被读取一次并原子转换为 typed snapshot。实施采用门禁驱动的渐进展开：本文件
固定完整路线和每阶段的小步骤索引，只有当前阶段的执行包展开到 2～5 分钟原子步骤。

**Tech Stack:** Go、GORM/MySQL、Hertz/Thrift、Eino ADK、React、TypeScript、Rush.js、
Vitest、Codex in-app browser、Workbench execution graph tooling。

---

## 0. 计划加载规则

**权威规格：**
`docs/superpowers/specs/2026-08-10-workbench-adaptive-execution-mvp-design.md`

**批准的 docs-only 设计提交链：**

- 初始设计：`6500c8b9e379d05693bada1ca627a5937f8841c3`
- 执行模式退休：`038f7e1a43a22e44143b7970e3f9d94f5ff1b517`
- 初始实施计划与 P0 包：`1060716f1b327cbd46104f12657dde1483a60830`

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

### 交付优先规则（P1M～P5 强制）

本节是后续所有阶段和执行包的全局裁剪规则。若后文任务清单包含超出当前阶段退出门的扩展项，
以本节为准；不得因为历史清单写得更满，就把非必要能力重新塞入主线。

- [ ] 每个执行包只实现满足当前阶段退出门的最小端到端闭环，并尽快形成可运行、可回归、可提交的
      候选 SHA。不得顺带建设通用框架、新页面、新公共接口、新 migration、第二套执行链或未被当前
      退出门消费的能力。
- [ ] 只有以下问题属于当前包硬阻断，必须在交付前关闭：受影响代码不能编译或构建；核心 happy
      path/必要失败路径回归失败；鉴权、安全、租户隔离、数据一致性、幂等或恢复合同错误；公共
      API/持久化合同不兼容；长期权威事实与实际实现不一致；当前阶段退出门无法满足。
- [ ] 下列工作默认记为非阻断后续项，不得延长当前交付：额外泛化或重构、未被主线调用的兼容层、
      超出代表性核心用例的测试排列组合、额外竞态/性能矩阵、非必要可观测性、UI 润色、备用实现、
      未来扩展点和不影响当前退出门的 P2/P3 质量建议。
- [ ] 发现新问题时先按“硬阻断/非阻断后续项”二分。只有能指向上述硬阻断条件或当前退出门的事项
      才能进入 active 包；其余只记录 `description`、`reason_deferred`、`reentry_gate`，不得立即实现。
- [ ] 外部环境缺失时，先交付可编译且严格门控的测试/能力，并明确记录 `NOT_VERIFIED`；不得把
      `SKIP` 伪装为 PASS，也不得为了等待非当前退出门所需的环境阻塞其它已验证交付。该验证只在其
      re-entry gate 成为后续阶段硬门时恢复。
- [ ] 每个执行包提交前必须输出一份紧凑范围核对：`delivered`、`deferred_non_blocking`、
      `unverified_external_gate`、`blocking_remaining`。只有 `blocking_remaining` 为空才可提交；
      `deferred_non_blocking` 不阻止主线进入下一最小包。
- [ ] 本规则不允许降低安全、数据一致性、公共合同或阶段退出门，也不允许把真实 blocker 改名为
      “后续优化”。它只裁掉对当前可交付结果没有必要贡献的扩展工作。

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
| P1M 无模式基础 | `locked` | P0 PASS | 新请求/恢复不写旧 mode；生产 gate-off 持久化 admission+baseline decision，共享边界双 producer 合同测试、legacy snapshot、422 拒绝、等价 smoke 通过 |
| P1D adaptive decision 纵切 | `locked` | P1M PASS | adaptive direct/multi-step、normal/resume 恢复、最小 TaskDetail 投影通过 |
| P2 progress/verification 闭环 | `locked` | P1D PASS | gate-on repair/replan/clarify/stop、成功门禁、取消/恢复/安全矩阵通过；gate-off 仍走基础终态 |
| P3 公共投影与当前页面完成态 | `locked` | P2 PASS | list/SSE 等价、未知版本 fail closed、当前页面 Vitest/browser 通过 |
| P4 评测与候选冻结 | `locked` | P3 PASS、30 开发集净增至少 3/30 | 同 SHA 80 holdout、安全门和全部工程门通过 |
| P5 本地 `dev` 集成 | `locked` | 用户确认候选 exact SHA 与范围 | 仅本地 `ff-only`，不 push、不发布 |

首个 active 包保存在：
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p0-spike.md`。

### 工作量护栏

- P0 是唯一立即展开的高风险验证，硬时限为 2 engineer-days；它失败就停止，后续工作量归零。
- P1M 的执行模式退休为 4～6 engineer-days，包含后端 consumer/recovery、前端 V2 client 和测试；
  admission/decision 合同冻结后，前端与后端可由 2 人并行，预计 2～3 个工作日完成，不另立项目。
- P0 通过后按 Week 1 技术纵切、Week 2 完整闭环、Week 3 冻结验收推进。基准配置为 4 名专职
  FTE；只有 3 名时按 3～4 周排期，不以删测试换取两周口径。
- 后续每个执行包只展开到当周可提交边界，原则上控制在约 200～350 行；超出时按独立 gate 再拆包，
  不能把多个共享状态写任务并发化。
- 非 MVP：新页面/route、第二套 worker/executor、Subagent、多模型 classifier/planner/verifier 循环、
  默认 migration、深层 options 管理、发布自动化改造。发现这些需求只记录，不进入当前包。
- 最快效果验证点不是 UI 完成，而是 P0 的真实 MySQL 原子性、P1M 的无模式 gate-off 基础和
  P1D 的 direct/multi-step 小型纵切；基准 4 FTE 时预计第 5～7 个工作日可在现有页面验证，
  任一门禁不成立都不继续堆页面和评测工程。

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
  git cherry-pick 1060716f1b327cbd46104f12657dde1483a60830
  git cherry-pick 038f7e1a43a22e44143b7970e3f9d94f5ff1b517
  PLAN_ALIGNMENT_COMMIT_SHA="$(
    git rev-list --all --max-count=1 --fixed-strings \
      --grep='docs: align adaptive MVP plan with mode retirement'
  )"
  test -n "$PLAN_ALIGNMENT_COMMIT_SHA"
  test "$(git log -1 --format='%s' "$PLAN_ALIGNMENT_COMMIT_SHA")" = \
    "docs: align adaptive MVP plan with mode retirement"
  test "$(git diff-tree --no-commit-id --name-only -r "$PLAN_ALIGNMENT_COMMIT_SHA")" = \
    "docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp.md"
  git cherry-pick "$PLAN_ALIGNMENT_COMMIT_SHA"
  ```

  Expected: 四个提交都只含 `docs/superpowers/specs/**` 或
  `docs/superpowers/plans/**`；最后一个提交的 subject 精确为
  `docs: align adaptive MVP plan with mode retirement`。若检索为空或文件范围不符则停止。

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
- [ ] 把 P1M～P5 保持 `locked`；
- [ ] 返回“一张紧凑表”的修订规格评审；
- [ ] 不增加 004/005 表、不调用后续实现包。

## 2. P1：无模式基础与 adaptive decision 纵切（P0 PASS 后加载）

P1 分为两个串行包。P1M 先建立无模式、可恢复的 gate-off 基础；P1D 才接 adaptive producer。
两包共用同一 `ExecutionDecision` schema，不能并行修改 admission、Run config 或恢复链。

P1M-A 已作为独立安全切片承担 canonical raw ingress freeze、第一方前端旧字段清理与
whole-Thread DELETE 编译期 guard；P1M-B1 又完成 public Application admission、canonical typed
error 映射和历史恢复兼容边界。两者都不等于 P1M PASS。后续 P1M-B/C 仍负责 typed
admission/decision，并退休 server-owned ADK child legacy seam 与 legacy consumer。P1L 已延期，guard 必须
保持；P1D 仍以完整 P1M 与 rolling-authority 闭环为前置。

### P1M：退休产品执行模式

**加载时必须创建：**
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p1m-mode-retirement.md`

**主要文件边界：**

- Create: `backend/domain/agentthread/entity/adaptive_execution.go`
- Create: `backend/application/agentthread/adaptive_admission.go`
- Create: `backend/application/agentthread/adaptive_admission_test.go`
- Create: `backend/application/agentthread/adaptive_baseline_decision.go`
- Create: `backend/application/agentthread/adaptive_baseline_decision_test.go`
- Create: `backend/application/agentthread/adaptive_legacy_admission.go`
- Create: `backend/application/agentthread/retired_execution_mode_contract_test.go`
- Modify: P0 已创建的 `backend/domain/agentthread/repository/adaptive_execution.go`
- Modify: P0 已创建的 `backend/domain/agentthread/repository/mysql_adaptive_execution.go`
- Modify: P0 已创建的 `backend/domain/agentthread/repository/mysql_adaptive_execution_test.go`
- Modify: `backend/application/agentthread/runtime_config.go`
- Modify: `backend/application/agentthread/service.go`
- Modify: `backend/application/agentthread/adk_executor.go`
- Modify: `backend/application/agentthread/adk_agent_factory.go`
- Modify: `backend/application/agentthread/adk_lead_prompt.go`
- Modify: `backend/application/agentthread/adk_middleware.go`
- Modify: `backend/application/agentthread/adk_provider_capability.go`
- Modify: `backend/application/agentthread/adk_subagent_tool_provider.go`
- Modify: `backend/application/agentthread/adk_builtin_subagent.go`
- Modify: `backend/application/agentthread/adk_singleagent_subagent_agent_factory.go`
- Modify: `backend/application/agentthread/adk_subagent_run_recorder.go`
- Modify: `backend/application/agentthread/journal_feature_gate.go`
- Modify: `backend/application/agentthread/journal_metrics.go`
- Modify: `backend/application/agentthread/run_lease_recovery.go`
- Modify: `backend/application/agentthread/human_interaction_resume.go`
- Modify: `backend/application/agentthread/subagent_retry.go`
- Modify: `backend/application/application.go`
- Modify: `backend/application/agentthread/init.go`
- Create: `backend/api/handler/coze/workbench_execution_control_validator.go`
- Create: `backend/api/handler/coze/workbench_execution_control_validator_test.go`
- Modify: `backend/api/handler/coze/workbench_canonical_thread_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_run_service.go`
- Modify: `backend/api/handler/coze/workbench_canonical_contract.go`
- Modify: `idl/workbench/thread.thrift`
- Regenerate: `backend/api/model/workbench/thread_contract/thread.go`
- Regenerate: `frontend/packages/arch/api-schema/src/idl/workbench/thread.ts`
- Test: `frontend/packages/arch/api-schema/src/__tests__/workbench-thread-contract.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/components/types.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/workbench/thread-client/canonical-thread-client.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-follow-up.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tasks/task-run-actions-hook.ts`
- Verify/Modify: `frontend/apps/coze-studio/src/pages/tasks/task-detail-hooks.ts`
- Test: 与上述文件同目录的现有 `*_test.go`、`__tests__/*.test.ts(x)`；不得新建第二套 client。

P1M 小步骤索引；P1M 文件必须把每项继续拆成 RED/最小实现/GREEN/commit：

- [x] P1M-B1 在 P1M-A canonical raw ingress 与第一方 writer 清理之后，为 public
      `CreateTaskThread/CreateRun` 增加同义七字段 admission；top-level retry 在来源读取前拒绝，
      package-private ADK child provenance 只允许非零 parent 的精确 Subagent Run，未知 provenance
      fail closed。canonical typed error 映射为 422，Human/Subagent/Journal/lease recovery 用旧
      Config/Context fixture 证明历史继承不被新 admission 改写。
- [x] P1M-C1 定义纯 Go、mode-free `AdaptiveAdmissionSnapshot`/`ExecutionDecision` 和
      fail-closed validators；`BaselineDecisionProducer` 仅 gate-off、deterministic 地生成固定
      `execute/multi_step` decision，gate-on fail closed。该内部 future contract 保持
      `implemented_unwired`；历史 runtime controls 和
      package-private server-owned subagent compatibility seam 保持，故 P1M 未 PASS。P1L 继续 deferred，
      whole-Thread DELETE hard-disabled。
- [x] P1M-C2a 在纯 domain `backend/domain/agentthread/adaptivecontract` 实现 strict canonical
      admission/decision codec 与 fail-closed domain validation；application 保持 C1 validator wrapper
      和 sentinel alias，兼容既有 `errors.Is` 合同。
- [x] P1M-C2b 在 private `AdaptiveExecutionRepository` 实现 durable bootstrap 的 exact-tuple
      commit/readback，并以 internal/unsequenced `adaptive.admission`、`adaptive.decision` 和
      `workbench_control` checkpoint 存储；generic writer/reader 隔离这些保留事实。真实 MySQL
      双连接验收仍待显式 disposable DSN/DDL gate。该 repository 切片本身不接 runtime，不代表 P1M
      或 P1L PASS，whole-Thread DELETE guard 继续 hard-disabled。
- [x] P1M-C3a 只接已 enrolled、fresh、顶层 Eino ADK `Execute`：输入解析后、`buildRuntime` 前由
      gate-off coordinator 先读 durable replay，首次才提交固定 baseline `execute/multi_step`；未
      enrolled no-op 仅适用于已通过 Execute Thread/Run 身份校验且启动依赖已装配的明确 `task` 或既有
      空 `RunKind` 顶层兼容形式；logical Journal root 可与 execution Run 不同，失败不得创建 checkpoint
      store 或 Agent runtime。`Resume`、legacy runtime、gate-on、IDL 与 frontend/UI 暂不接；真实 MySQL
      双连接验收继续记为 `NOT_VERIFIED`，P1M 未 PASS，P1L deferred 与 whole-Thread DELETE hard guard
      不变。
- [x] P1M-C3b 只消费 C3a 的 durable/replayed facts：`ADKExecutor.Execute` 用私有 context 交给
      `ApplicationADKAgentFactory`，Factory 重新校验 identity/policy 后只覆盖本地 Plan capability，
      使 Todo prompt 与 Plan backend 同时服从 `PlanAllowed + execute/multi_step`；direct 不建 Plan，
      invalid/blocked facts fail closed，无 facts 的未接入路径保留历史兼容。Subagent、reasoning/model
      inference、Journal enrollment/metrics、Resume、legacy、gate-on、IDL/UI 都不在本切片；真实 MySQL
      双连接仍 `NOT_VERIFIED`，P1M 未 PASS，P1L/DELETE guard 不变。
- [x] P1M-C3c 继续只消费同一 fresh Execute 的 durable admission：Factory 将
      `SubagentsAllowed=false` 投影到本地 runtime config，使 lead prompt 与 Subagent limit middleware
      同时禁用；标准 `ADKSubagentToolProvider` 在解析 definition 或构建 child Agent 前消费一次性私有
      disable 信号，仅保留 base tools，并在调用 base provider 前清掉信号，避免能力状态下传到 child。
      无 facts 的未接入路径继续保留历史兼容。Resume、reasoning/model inference、Journal
      enrollment/metrics、legacy、gate-on、IDL/UI 均不在本切片；真实 MySQL 双连接仍
      `NOT_VERIFIED`，P1M 未 PASS，P1L/DELETE guard 不变。
- [x] P1M-C3d 继续限定同一 fresh Execute：Factory 在 durable facts 已重验后，将旧
      `mode`、`thinking_enabled`、`reasoning_effort` 对当前本地 runtime config 的影响中和为
      `ThinkingEnabled=false`、空 `ReasoningEffort`，使 primary/failover model option projection 与
      provider-capability middleware 使用同一安全中性请求；不改持久化 Config，也不引入新的推理
      policy。无 facts 的未接入路径继续保留历史兼容。Resume、Journal enrollment/metrics、legacy、
      gate-on、IDL/UI 均不在本切片；真实 MySQL 双连接仍 `NOT_VERIFIED`，P1M 未 PASS，P1L/DELETE
      guard 不变。
- [ ] 引入 typed adaptive envelope/decision 时继续复用 P1M-A/B1 已完成的 raw ingress、
      Application admission 与 422 合同，补齐 typed payload 的 root、附件、follow-up、retry、resume
      组合回归；不得重新开放七字段、扫描正文字符串、误伤其它领域的同名 `mode`，也不得把未知
      execution-control 字段静默丢掉。
- [ ] 定义 `AdaptiveAdmissionSnapshot`、`AdaptiveAdmissionSource`、`AdaptiveCapabilities`、
      `AdaptiveLimits` 与 `ExecutionDecision`；snapshot 只含 gate/schema/capability/limits 及可空
      source Run/generation/config digest/decoder version，不得定义 product mode 字段。
- [ ] 写 validation RED：capability 只能接受或拒绝 decision；不匹配必须在 Plan/工具前
      `blocked_policy` 或产生新 decision revision，禁止静默降级 execution shape。
- [ ] 扩展 P0 transaction：Run 被 claim 且已有 active Attempt/lease 后，在
      `ADKExecutor.Execute/Resume` 的 `buildRuntime` 之前一次受 fence 提交 admission snapshot 与
      typed decision；CreateThread/CreateRunBundle 阶段只保证新 Config 无旧字段，不能提前绕过
      P0 的 Run/Attempt fence。lost-response retry 只能返回原事实。
- [ ] 实现 `BaselineDecisionProducer`：固定输出 `execute/multi_step`、服务端固定 safe summary、空
      deliverables/checks；P1M 生产只安装该 producer，若持久化快照的 gate=true 则明确返回
      `ErrAdaptiveProducerUnavailable`，直到 P1D 安装 adaptive producer，不能增加第二个隐藏分类器。
- [ ] fresh 顶层 Execute 的 Plan capability、Subagent 禁用与旧 reasoning 控制中和已由
      P1M-C3b/C3c/C3d 切到 durable admission/decision；继续把 Resume/历史兼容 Plan 路径、真正的
      server inference policy、Journal enrollment/metrics
      切换到 admission、purpose binding 与 server inference config，再删除 mode consumer；
      `RuntimeModeEinoADK/legacy` 执行内核路由保持不变。
- [ ] 把 root、附件、follow-up、retry、resume 与 child/retry config 写入全部切到审核后的 canonical
      typed V2 envelope；先证明 IDL/生成 client 能无损承载现有模型、Skill、MCP、知识库和数据库选择，
      再删除前端 `WORKBENCH_REQUESTED_POLICY` 与旧 runtime config/mode metadata。缺少 typed 替代字段
      时 P1M 立即停止，禁止丢弃选择、塞进 `coze:any`/metadata 或另造手写 client。
- [ ] 先为 `ResumeCanonicalRunRequest` 补齐生成的 `Idempotency-Key` header，并为 mode-free 写入所需的
      非模式 composer 选择冻结 typed IDL；重新生成 Go/TypeScript 合同，运行
      `workbench-thread-contract.test.ts`，再改五个前端入口。root 带附件和 follow-up 必须保持
      upload-before-run，retry 只带 source lineage，resume body 只带 interrupt response。
- [ ] 为切换前来源实现隔离 `LegacyAdaptiveAdmissionDecoder`：只在来源缺 typed admission 时读取
      已知旧字符串；recovery/follow-up/retry 创建目标 Run 时先用 server-only sanitizer 去掉旧键并
      保存 source lineage，目标被 claim 后由 coordinator 优先继承 source typed snapshot，否则读取
      source Config 并与首次 target baseline decision 一次受 fence 提交 decoder version、source
      Run/generation/config digest、`feature_gate=false` 和保守能力；decoder 自身不得生成 decision。
      crash/retry、多跳只继承同一 snapshot，不再次解码。
- [ ] 写 unknown/conflict、source-generation drift、A→B→C 多跳与并发 recovery RED；decoder
      不得创建 decision、回写旧字符串或把 legacy value 公开投影。
- [ ] 删除生产主路径的 `DeerFlowMode`、`DeerFlowRequestedPolicy`、`ModeExplicit`、归一化与 mode
      metrics；结构扫描只允许专用 legacy decoder 和固定历史 fixture 出现旧 literal。
- [ ] 运行后端 RED/GREEN：

      ```bash
      cd backend
      GOCACHE=/private/tmp/coze-adaptive-p1m-go-cache \
        go test -p 1 -gcflags="all=-l -N" \
        ./application/agentthread ./api/handler/coze \
        -run 'RetiredExecution|AdaptiveAdmission|BaselineDecision|LegacyAdaptiveAdmission|CanonicalAdaptiveEnvelope' \
        -count=1
      ```

      Expected: RED 时精确缺少新类型/producer/validator；实现后全部 PASS，不能 SKIP。
- [ ] 运行前端 RED/GREEN：

      ```bash
      cd frontend/packages/arch/api-schema
      rushx test \
        src/__tests__/workbench-thread-contract.test.ts \
        -t 'canonical resume idempotency|typed mode-free canonical write'

      cd frontend/apps/coze-studio
      rushx test \
        src/pages/workbench/__tests__/workbench.test.tsx \
        src/pages/workbench/thread-client/__tests__/canonical-thread-core-client.test.ts \
        src/pages/workbench/thread-client/__tests__/workbench-thread-client-contract.test.ts \
        src/pages/tasks/__tests__/task-follow-up.test.ts \
        src/pages/tasks/__tests__/task-run-actions-hook.test.tsx \
        src/pages/tasks/__tests__/task-detail-follow-up-actions.test.tsx \
        src/pages/tasks/__tests__/task-detail.test.tsx \
        src/pages/tasks/__tests__/canonical-frontend-contract.test.ts
      rushx lint
      rushx build
      ```

      Expected: 目标 Vitest、lint、生产构建全部 PASS；旧 payload snapshot 断言已经由 V2 typed
      envelope 断言替代，不是直接删除测试。
- [ ] 运行 `gofmt`、`git diff --check` 和顶层、无 Subagent fixture 的 gate-off 对 `origin/dev`
      行为等价 smoke；比较 Plan 早于首个 Tool、基础终态、`runtime=eino_adk` 以及无 progress/
      verification，不要求字节复现旧 auto→Ultra 的 Subagent 行为。smoke 失败即 P1M FAIL，不得用
      隐藏 mode 分支修补。
- [ ] 分四个小提交收口：`feat: add mode-free adaptive admission`、
      `refactor: retire legacy execution modes`、`feat: reject retired execution controls`、
      `feat: move workbench writes to generated canonical v2`；每个提交都先完成对应 RED、最小实现和 GREEN，
      记录 `P1M_BASE_SHA/P1M_RESULT/P1M_HEAD_SHA` 后才解锁 P1D。

### P1D：adaptive decision 最小纵切

**加载时必须创建：**
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p1d-decision.md`

P1D 小步骤索引；每一项在 P1D 文件中继续拆成 RED/最小实现/GREEN/commit：

- [ ] 由独立评测负责人封存 80 holdout/evaluator，记录 seal hash；实现侧只获得 schema、计数和
      30 开发集，P4 只能复核同一 hash，不能重新冻结。
- [ ] 冻结 feature eligibility、Journal enrollment、gate-off reason 与 admission metrics，证明
      两组都持久化 admission/decision，gate 只选择 producer 和后续闭环。
- [ ] 完成 `ExecutionDecision` 有界 codec/公共 DTO，并定义 `ProgressEvaluation`、
      `VerificationResult` codec；后两者在本包只做零迁移持久化/恢复 fixture，不接运行循环。
- [ ] 为三种 decision 和 `single_step/multi_step` XOR 写 Go validation RED。
- [ ] 实现服务端 acceptance-check registry，强制模型提出项为 required。
- [ ] 实现 adaptive decision producer；模型只提交候选，服务端校验后通过 P0 transaction 写 typed
      RunEvent + checkpoint refs，不能读取或重新生成 product mode。
- [ ] 在 `ADKExecutor.Execute` 接入同一 admission/decision coordinator，但保留现有
      `RuntimeSelector`；在 `ADKExecutor.Resume` 恢复同一状态，不新建 resume 实现。
- [ ] 证明 gate-on `direct` 不创建 Plan，也不产生非验证 ToolStarted；证明 gate-on
      `multi_step` 的 Plan persisted sequence 小于首个 ToolStarted sequence。
- [ ] 证明 gate-off 仍由 baseline producer 写 fixed multi-step decision，不产生 progress/
      verification，沿既有终态规则完成且不写伪 verification passed。
- [ ] 把 `payload_version` 从持久化 decision 传到 public list/SSE 和生成 TS 类型；在现有
      TaskDetail 只展示 direct/planning/executing/final 最小状态，不新建 route/page。
- [ ] 运行 Week-1 技术纵切门：三个 typed fact fixture 可读回/恢复、两组 admission/decision
      可审计、Plan 顺序正确、安全硬门为零；在现有页面完成第一轮 5～7 日效果验证。
- [ ] 提交 P1D，记录 `P1D_BASE_SHA/P1D_RESULT/P1D_HEAD_SHA` 后才解锁 P2。

## 3. P2：progress、verification 与安全闭环（P1D PASS 后加载）

**加载时必须创建：**
`docs/superpowers/plans/2026-08-10-workbench-adaptive-execution-mvp-p2-loop.md`

P2 小步骤索引：

- [ ] 为 `ProgressEvaluation` 的五种 recommendation 与字段 XOR 写 RED。
- [ ] 仅在 gate-on 的 terminal tool result、里程碑、状态转换和 verification 前接入 evaluation；
      gate-off 不调用 progress/verifier，也不写空壳事件。
- [ ] 对 evidence lineage、decision/Plan revision、generation/lease 做服务端重校。
- [ ] 实现最多 2 次 replan、2 次 verification repair、连续 3 次 no-progress。
- [ ] 把 24 tool calls 和 20 active minutes 按 canonical lineage 累积。
- [ ] 复用现有 human interaction 实现 clarify/resume，并继承 gate/schema/limits。
- [ ] 为 `VerificationResult` registry coverage、证据高水位和 stale invalidation 写 RED。
- [ ] 实现 gate-on `passed` 的同事务 terminal gate；gate-on 无 passed verification 不得 succeeded，
      gate-off 继续走既有终态事务并禁止伪造 verification passed。
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
- [ ] 证明 `adaptive.decision` 对 gate-on/off 都存在，而 progress/verification 只能由 gate-on 产生。
- [ ] 证明 internal discriminator 决定 `payload_version`，不按当前代码版本猜测。
- [ ] 证明 canonical list 与 SSE 对同一事件返回相同版本和安全 payload。
- [ ] 在 frontend adapter 保留 `payload_version`，未知版本不推进执行 reducer。
- [ ] 在现有 transcript 显示 clarify/final，在 Todo dock 显示 Plan revision。
- [ ] 在现有 Journal panel 映射 milestone/progress/verification terminal。
- [ ] 增加 planning/executing/repairing/replanning/verifying 状态与紧凑验证摘要。
- [ ] 保留 loading/empty/error/readonly/cancel/reconnect/keyboard/ARIA 行为。
- [ ] 用 Vitest 覆盖 direct 无空壳、多步更新、验证失败和 SSE reconnect。
- [ ] 用 in-app browser 验收现有 Workbench URL；记录账号/空间、可见状态和控制台错误。
- [ ] 延续 P1M-A 已同步的 ingress/UI 当前事实；完成 P1M/P1D 后继续更新
      `workbench-execution-chain.md`、`workbench-execution-graph.json` 和必要的
      `workbench-chat.md`，但保留 `RuntimeModeEinoADK/legacy` 执行内核路由事实。
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
- [ ] 运行模式退休结构门：新 root/follow-up/retry/resume/child payload 无旧字段；生产主路径无
      product mode enum/branch/metric；旧 literal 仅位于批准的 decoder/fixture allowlist。
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

- [ ] P0、P1M、P1D、P2～P4 全部 `PASS`，没有跳过或事后补写的执行包。
- [ ] `auto/pro/ultra/requested_policy` 与同语义 product `mode` 已从请求、新持久化、生产分支、
      consumer 和指标中退休；只读 legacy decoder 不回写、不重解、不公开投影。
- [ ] gate-on/off 都原子持久化 typed admission 与 `ExecutionDecision`；baseline producer 只有固定
      `execute/multi_step`，capability 不改写或降级 shape。
- [ ] 现有 normal/resume 分支复用同一 admission/decision coordinator。
- [ ] 没有新增页面、公共 endpoint、worker、executor、模型循环或默认 migration。
- [ ] gate-on `direct` 维持快速路径、multi-step Plan 前置、成功全部有当前 passed verification；
      gate-off 无 progress/verification 并沿既有终态规则完成。
- [ ] 未授权写入、重复副作用、敏感公共投影均为 0。
- [ ] 80 holdout、12 场景矩阵、工程测试、执行图和当前页面验收绑定同一候选 SHA。
- [ ] 用户确认前不合并 `dev`；确认后也只执行本地 `ff-only`。
