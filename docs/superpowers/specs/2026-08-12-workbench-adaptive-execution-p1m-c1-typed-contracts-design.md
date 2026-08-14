# P1M-C1 Mode-free Typed Contracts 设计

## 1. 结论

P1M-C1 只建立后续执行链需要的纯 Go 合同：typed admission snapshot、typed execution
decision、确定性校验和 gate-off baseline producer。该切片不接入数据库、RunEvent、
`ADKExecutor`、IDL、前端或旧 mode consumer，也不改变生产运行行为。

这样可以先冻结“允许什么”和“决定怎么执行”之间的边界，同时避免在 Run/Attempt fence、
持久化 codec 和恢复协议尚未接入时制造半套生产权威。

## 2. 当前事实

- P1M-A 已冻结 canonical raw ingress，并停止第一方前端写入七个旧执行控制字段。
- P1M-B1 已在 public `ApplicationService.CreateTaskThread/CreateRun` 增加同义 admission，
  但现有 runtime normalization、ADK consumer 和恢复链仍保留历史 mode 兼容行为。
- P0 repository 已提供 Run、Journal root、Attempt、Plan、Checkpoint 和 verified-success 的原子
  事务基础，但还没有 admission/decision 的生产持久化入口。
- `AdaptiveVerifiedSuccessGate` 已引用 `decision_id/decision_revision`，但它不是
  `ExecutionDecision` 的定义或 producer。

## 3. 目标

本切片完成后，代码库具备以下可独立测试的能力：

1. 表达无产品 mode 的 `AdaptiveAdmissionSnapshot`；
2. 表达 `clarification/direct/execute` 与 `single_step/multi_step` XOR 的
   `ExecutionDecision`；
3. 对 admission、decision、能力匹配和有界字段执行确定性、fail-closed 校验；
4. gate-off 使用唯一 `BaselineDecisionProducer` 产生固定 `execute/multi_step` decision；
5. capability 不匹配时返回稳定阻断错误，绝不把 execution shape 静默降级。

## 4. 明确非目标

P1M-C1 不做以下工作：

- 不新增或修改 migration、repository interface、MySQL transaction 或 P0 lock order；
- 不把 snapshot/decision 写入 Run config、RunEvent、Checkpoint 或 Journal；
- 不接入 `ADKExecutor.Execute/Resume`，不调用 producer；
- 不定义 JSON codec、公共 DTO、IDL 或 TypeScript 类型；
- 不删除 `DeerFlowMode`、`DeerFlowRequestedPolicy` 或历史 decoder；
- 不改变 Plan、Subagent、reasoning、Journal enrollment 或 metrics consumer；
- 不实现 adaptive model producer、progress、verification 或公共投影；
- 不声称 P1M、P1D 或 AdaptiveGate production wiring 已完成。

## 5. 文件边界

本切片只允许以下实现文件：

- Create: `backend/domain/agentthread/entity/adaptive_execution.go`
- Create: `backend/domain/agentthread/entity/adaptive_execution_test.go`
- Create: `backend/application/agentthread/adaptive_admission.go`
- Create: `backend/application/agentthread/adaptive_admission_test.go`
- Create: `backend/application/agentthread/adaptive_baseline_decision.go`
- Create: `backend/application/agentthread/adaptive_baseline_decision_test.go`

若实现需要修改 repository、executor、runtime config、IDL 或前端，本切片立即停止并回到设计，
不得扩 allowlist。

## 6. Domain 类型

### 6.1 Admission snapshot

`AdaptiveAdmissionSnapshot` 是数据结构，不包含持久化或 JSON 标签。字段语义为：

- `Schema`：精确 `workbench-adaptive-admission.v1`；
- `FeatureGateEnabled`：首次 admission 冻结的实验 gate；
- `Source`：`fresh`、`typed_inheritance` 或 `legacy_decoder`；
- `SourceRunID`、`SourceExecutionGeneration`：非 fresh 来源的 lineage；
- `SourceConfigDigest`、`DecoderVersion`：只允许 legacy decoder 来源使用；
- `Capabilities`：服务端冻结的功能许可；
- `Limits`：服务端冻结的执行上限。

`Source` 使用 `AdaptiveAdmissionSource` 字符串枚举。`SourceRunID` 使用 `*int64`，
`SourceExecutionGeneration` 使用 `*uint64`；fresh 来源以 nil 表示没有 lineage，禁止用 0 冒充
null。digest 与 decoder version 使用 string，非 legacy 来源必须为空。

`AdaptiveCapabilities` 只表达本期确定需要的能力：

- `PlanAllowed`；
- `ReadOnlyToolsAllowed`；
- `SandboxWritesAllowed`；
- `HumanInteractionAllowed`；
- `SubagentsAllowed`，P1M/P1D MVP 中必须为 `false`。

`AdaptiveLimits` 固定包含：

- `MaxToolCalls`，上限 24；
- `MaxReplans`，上限 2；
- `MaxVerificationRepairs`，上限 2；
- `MaxConsecutiveNoProgress`，上限 3；
- `MaxActiveDurationSeconds`，上限 1200。

所有 limits 必须为正数且不能超过硬上限。gate-off/on 使用同一结构；gate 不得改写能力或上限。

来源合同：

- `fresh`：所有 source identity、digest、decoder version 必须为空；
- `typed_inheritance`：source Run 与 generation 必须有效，digest 和 decoder version 必须为空；
- `legacy_decoder`：source Run/generation 必须有效，digest 必须是 64 位小写十六进制，
  decoder version 必须精确为 `workbench-adaptive-legacy-decoder.v1`，且
  `FeatureGateEnabled=false`；
- 未知来源或混合字段一律拒绝。

C1 只声明这一项 legacy decoder version；新增版本必须在后续切片显式扩展校验和测试，不能接受
任意非空字符串。

### 6.2 Execution decision

`ExecutionDecision` 同样只做内部 Go 数据结构，不在本切片冻结 wire codec。它包含既有设计已经
批准的字段：

- schema、decision ID/revision；
- execution Run、Journal Run、Attempt、execution generation；
- 可空 Plan scope Run；
- goal summary、deliverables、acceptance checks；
- decision、execution shape、可空 clarification question；
- safe summary、created time。

`PlanScopeRunID` 使用 `*int64`，`ClarificationQuestion` 使用 `*string`，空值必须为 nil；
`ExecutionShape` 使用字符串枚举的零值表示逻辑 null。`Deliverables` 固定为 `[]string`；
`AcceptanceChecks` 固定为 `[]AdaptiveAcceptanceCheck`，每项字段为 `CheckID`、`Kind`、
`TargetRef`、`SafeDescription`。`CreatedAt` 是正数 Unix 毫秒。

枚举固定为：

- decision：`clarification`、`direct`、`execute`；
- shape：空、`single_step`、`multi_step`。

XOR 合同：

- `clarification` 必须有问题，shape 与 Plan scope 为空；
- `direct` 的问题、shape 与 Plan scope 均为空；
- `execute/single_step` 不带问题和 Plan scope；
- `execute/multi_step` 不带问题，且必须有有效 Plan scope Run；
- 其它组合全部拒绝。

字段预算沿用总设计：opaque ID 最多 191 bytes；summary/question 最多 1024 bytes；
deliverables 最多 16 项且每项最多 512 bytes；acceptance checks 最多 32 项，check ID 与
target ref 最多 191 bytes，kind 最多 64 bytes，safe description 最多 512 bytes。C1 只做结构与
长度校验；敏感信息 sanitizer 属于后续
producer/codec 接入门，C1 的固定 baseline 文本不含请求内容。

## 7. Application 校验

`adaptive_admission.go` 提供纯函数：

- `ValidateAdaptiveAdmissionSnapshot(snapshot)`；
- `ValidateExecutionDecision(decision)`；
- `ValidateExecutionDecisionAgainstAdmission(snapshot, decision)`。

稳定错误为：

- `ErrAdaptiveAdmissionInvalid`；
- `ErrExecutionDecisionInvalid`；
- `ErrAdaptiveDecisionBlockedPolicy`。

第三个函数只接受或拒绝 decision：

- `multi_step` 需要 `PlanAllowed`；
- clarification 需要 `HumanInteractionAllowed`；
- MVP snapshot 若允许 Subagent，整个 snapshot 无效；
- capability 不满足时返回 `ErrAdaptiveDecisionBlockedPolicy`；
- 禁止把 multi-step 改成 single-step/direct，禁止生成替代 decision。

Run/Journal/Attempt/generation 的数据库真实性不由纯校验函数判断，后续 transaction coordinator
必须在当前 lease/generation fence 下完成 current-lock revalidation。

## 8. Baseline producer

`BaselineDecisionProducer` 是无模型、无 I/O 的确定性 producer。输入由调用方提供既有身份、
decision ID/revision、Plan scope 和创建时间；producer 不分配 ID、不读取 Run config、不访问数据库。
输入类型固定为 `BaselineDecisionRequest`，字段为 `Admission`、`DecisionID`、
`DecisionRevision`、`ExecutionRunID`、`JournalRunID`、`AttemptID`、`ExecutionGeneration`、
`PlanScopeRunID` 和 `CreatedAt`；Plan scope 使用正数 `int64`，producer 写入 decision 时转成非 nil
指针。

固定输出：

- schema：`workbench-adaptive-decision.v1`；
- decision：`execute`；
- execution shape：`multi_step`；
- goal summary：`Execute the submitted task.`；
- safe summary：`Use the baseline multi-step execution path.`；
- deliverables 与 acceptance checks：非 nil 的空集合；
- 其余身份字段逐字复制输入。

行为约束：

- snapshot `FeatureGateEnabled=true` 时返回 `ErrAdaptiveProducerUnavailable`；
- admission 或 producer 输入无效时 fail closed；
- `PlanAllowed=false` 时返回 `ErrAdaptiveDecisionBlockedPolicy`；
- 不读取任务正文，不分类，不推导 mode，不产生 progress/verification；
- 同一输入必须产生值相等的 decision。

## 9. 测试

Domain 测试覆盖：

- 三种 decision 与 shape/Plan/question XOR；
- ID、revision、generation、时间和所有预算边界；
- 空集合与 nil 集合的规范要求；
- domain 类型和生产文件不声明七个退休字段或 `auto/pro/ultra` 产品枚举。

Application 测试覆盖：

- fresh、typed inheritance、legacy decoder 的合法/非法 tuple；
- legacy gate 必须关闭；
- limits 硬上限与 Subagent fail closed；
- Plan/Human capability 接受与 `blocked_policy`；
- capability 不匹配时原 decision 不被修改。

Baseline producer 测试覆盖：

- gate-off 固定输出 `execute/multi_step`；
- 输出不含请求正文、旧模式字段或伪 verification；
- gate-on 返回 `ErrAdaptiveProducerUnavailable`；
- Plan capability 缺失返回 `ErrAdaptiveDecisionBlockedPolicy`；
- 同输入值相等且输入 snapshot 不被修改。

## 10. 验收与后续

P1M-C1 退出门：

- 新单元测试 RED→GREEN；
- `go test` 覆盖 entity 与 application/agentthread；
- package compile、`gofmt`、`git diff --check` 通过；
- 结构扫描证明新合同没有产品 mode 字段；
- 独立规格和质量审查 P0/P1=0。

后续 P1M-C2 才设计 codec、RunEvent/Checkpoint 引用和 Run/Attempt fence 下的一次原子提交；
P1M-C3 再把同一 coordinator 接入 Execute/Resume。任何生产接线都不能引用 C1 的进程内对象替代
durable fact。
