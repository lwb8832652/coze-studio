# Coze Studio 项目上下文

## 当前阶段

本仓库基于 Coze Studio，已经完成从外部参考项目吸收主要能力的阶段，当前默认
方向是 Coze 原生架构内的功能完善、可靠性、安全、性能、可维护性和用户体验
优化。Nuwax、DeerFlow 等历史资料只用于追溯来源或定位回归，不定义当前产品。

## 技术栈

- 前端：React、TypeScript、Rush.js、Coze Design/Semi UI；
- 后端：Go、Hertz、DDD 分层；
- Agent 执行：Go-native Agent Harness 与 Eino ADK；
- 合同：Thrift IDL 与生成的前端 API schema/client；
- 数据：MySQL、Atlas migrations、Redis、对象存储；
- 本地开发：Make、Rush、Vitest、Go test、Codex in-app browser。

## 稳定架构边界

### 前端

主应用位于 `frontend/apps/coze-studio`，共享能力位于 `frontend/packages`。页面
应使用生成 client、真实权限和现有设计系统，保留完整可访问性与状态反馈。

### 后端

后端入口为 `backend/main.go`，主要分层为：

- `backend/api`：HTTP handler 和路由；
- `backend/application`：用例编排和跨领域协调；
- `backend/domain`：实体、仓储接口和领域服务；
- `backend/infra`：数据库、缓存、对象存储和外部服务实现；
- `backend/crossdomain`：经过明确合同的跨域能力。

身份、空间、角色和系统管理员权限始终由服务端认证上下文及持久化事实决定。

对象存储运行时默认由数据库中的 `object_storage_configs` 主配置驱动。首次启动且
表为空时，后端会从兼容 env 存储配置导入一条主配置，并用
`OBJECT_STORAGE_CREDENTIAL_KEY` 加密 AK/SK；`OBJECT_STORAGE_CONFIG_SOURCE=env`
只作为数据库配置不可用时的 rescue bypass。系统管理页支持七牛、阿里 OSS、腾讯
COS、华为 OBS、AWS S3、MinIO 和 TOS 的多配置维护、连接测试、激活和删除，密钥
不回显。切换主配置持久化后，运行中进程可能展示 `restart_required`，以重启后的
bootstrap 结果作为真正运行时事实。

### Agent Runtime

Eino ADK 是执行内核，Coze 保存公共 task、event、checkpoint、memory、artifact、
token、Skill、MCP 和 guardrail 合同。内部运行时对象必须通过 adapter 投影为
经过审核的公共字段，Workbench 不暴露原始 prompt、tool payload、checkpoint、
credential、provider body 或隐藏配置。

### AppDev 与 Sandbox

Sandbox 控制面和运行流量分别启用。生产与共享环境只通过数据库配置的 HTTPS
remote provider 执行；本机 host runtime 只允许显式 Debug 模式。安全依赖缺失
时 fail closed，不回退到宿主机或内存 stub。

当前产品策略暂时不在工作空间侧栏展示“网页应用开发”入口。该能力的菜单元数据、
路由、页面、后端 API、通知映射和 Sandbox AppDev scope 继续保留，不视为删除或
废弃；后续完成功能开发后通过可见菜单投影恢复入口。

### IM Channels

当前只支持飞书官方 Go SDK。配置按工作空间隔离，secret 加密且不回显；外部
事件先持久化去重，再进入异步 Agent 流程。

### dev 预发布部署

dev 集成采用一次代码与范围审计。需求分支必须先对齐最新 `origin/dev`，报告远程
基准、目标 exact SHA、文件范围、验证结果、migration 清单和风险；用户明确确认后，
本地 `dev` 仅以 fast-forward 合入该已审计 SHA，不重复第二轮代码审计或测试。

合入本地 `dev` 后才执行发布前只读预检：固定实际部署 revision 与目标 SHA，审阅实际
部署区间的 migration，并验证 Atlas credential 文件安全性及 Atlas status。预检报告后
还需用户对同一 exact SHA 单独确认，才可运行 `deploy/dev/publish-dev.sh`；禁止直接
`git push origin dev`、force push、baseline、repair、backfill 或 down migration。

发布脚本负责再次校验分支、干净工作区、目标 SHA、远程竞态和 Atlas 状态，并按需执行
已确认的 forward migration 与 exact-SHA 非 force push。远程推送将触发
`preflight -> build-server/build-web -> verify-images -> promote -> deploy`；GitHub Actions
构建并验证前后端不可变镜像、晋级 `:dev` 标签并调用宝塔 webhook。完整且现行的流程、
凭据限制和异常处理以 `docs/superpowers/runbooks/dev-integration-audit.md` 为准。

该服务器运行两个应用容器和一个持久化的单节点 `nsqd`；MySQL、Elasticsearch、
Redis 和对象存储均为远程服务。NSQ 只在 Compose 网络中可见，业务发布与回滚
保留其命名卷。dev 部署允许省略向量数据库配置，未配置时保留 Elasticsearch
全文检索并关闭语义向量检索。Web 默认通过可配置的公网 HTTP 端口发布，域名与
TLS 由宝塔独立终止。

这是允许短时中断的单实例 dev/预发布流程，不等于生产发布。推送授权、数据库
操作和生产发布保持独立权限边界。

## 主要产品域

- 任务与 Agent Workbench：唯一公共事实模型为 Thread、Message、Run 和
  RunEvent，详细边界见 `docs/superpowers/context/workbench-chat.md`；
- 工作空间与系统管理：成员、角色、系统配置、模型和管理员能力；
- 对象存储控制面：多云配置、加密 credential、主配置切换和 env rescue；
- Skill 与 MCP：配置、版本、授权、健康状态和运行时装配；
- AppDev 与 Sandbox：项目文件、构建、预览、Provider 和安全网关；
- 通知与计划任务：可靠通知、公告、定时执行、幂等和重试；
- 计费与配额：服务端事实、审计和安全边界。

## Workbench Canonical API

`/api/workbench/threads/**` 是 Workbench UI 唯一公共 HTTP 合同，共 52 个
Thread、Run、Message、Upload、Artifact、Memory、Token Usage、Guardrail 和 MCP
Runtime Audit method/path pair；另有 2 个 `/api/workbench/journal/settings`
method/path pair，always-on canonical Workbench 路由合计 54 条。前端页面服务统一委托给进程内唯一
`canonicalThreadClient`；不存在运行时 client selector、canonical 路由开关或旧 HTTP
fallback。session principal 和 path resource 决定身份与资源归属，workspace 请求
使用 `X-Coze-Space-ID` 并由服务端再次授权。

P1M-A 已冻结 canonical 外部执行控制：CreateThread、Create/Wait/Stream Run、Resume
和 Subagent Retry 在 JSON binder 与持久化前，按结构化路径拒绝
`requested_policy`、`mode`、`thinking_enabled`、`reasoning_effort`、
`is_plan_mode`、`subagent_enabled` 和 `max_concurrent_subagents`。第一方前端不再写入
这些字段，并暂时隐藏“模型推理”控件；`runtime=eino_adk`、模型、Skill、MCP、知识库、
数据库和资源配置仍是合法输入。该冻结本身不代表后端 consumer 或恢复继承已经退休；后续
P1M-C3h2d 已另行完成 production ADK 的 `requested_policy`/`mode` consumer 退休。

P1M-C3i1 已交付 canonical Typed Submission V2 的封闭 IDL 与 Go/TypeScript 生成合同，
并让服务端 Create Thread、Create/Wait/Stream Run 与 Resume 接受
`initial_submission_v2`、`deferred_initial_submission_v2`、`submission_v2` 和
`response_v2`。handler 在任何 application mutation 前完成 raw JSON 重复键、字段、presence、
`null`、预算、union 和 V1/V2 混用校验，再把 V2 确定性映射到既有 application command 与
idempotency fingerprint；V1 继续可读。P1M-C3i2 已把五个第一方 writer 切到该合同：Workbench
无附件 atomic create、带附件 deferred create + turn、TaskDetail follow-up、top-level retry 与
Human Resume 分别只写 `initial_submission_v2`/`deferred_initial_submission_v2`、
`submission_v2` 或 `response_v2`，并继续复用唯一 canonical client、upload-before-run 顺序及
既有语义幂等 attempt。服务端 V1 reader 和第三方兼容调用仍保留。C3i2 自身不包含 Human Attempt
rollover；后续 C3h2b 已完成 enrolled Human Resume 的原子 rollover 与 full replay，C3h2c 又补齐
enrolled Resume 的 legacy exact-miss fallback。dev disposable MySQL 已完成 Human rollover、typed
recovery race 与 legacy recovery 门禁；production ADK 的旧 mode/policy consumer 也已退休。
Application rolling Plan writer 后续也已接入 atomic checkpoint boundary；其独立 dev MySQL rolling
gate 仍 `NOT_VERIFIED`。本轮 ordinary MVP 又让 fresh 顶层 Eino Run 不再以 Journal projection gate
作为 Attempt enrollment gate，并闭合 disabled/无 Attempt 的 lease recovery、bare Resume bootstrap 与
bare rolling Plan boundary；ordinary 的真实 dev MySQL 门禁仍 `NOT_VERIFIED`，gate-on producer 仍未
闭合，因此 P1M 仍未 PASS。

P1M-B1 已把同一七字段 admission 下沉到 public `ApplicationService.CreateTaskThread` 与
`CreateRun`，在 runtime normalization、top-level retry 来源读取和任何 mutation 前 fail
closed；因此 canonical、Scheduled Task、飞书和其它 public Application caller 共享同一
防线。package-private ADK server-owned child seam 仍可生成历史控制字段，Human interaction、
Subagent、Journal 与 lease recovery 仍原样继承已持久化 Config/Context。该兼容边界是有意的，
P1M-B1 不等于 mode consumer 退休或完整 P1M PASS。

P1M-C1 已定义纯 Go、无 product mode 的 `AdaptiveAdmissionSnapshot`、
`ExecutionDecision` 及 fail-closed validators；`BaselineDecisionProducer` 仅在 gate-off
时确定性地产生固定 `execute/multi_step` decision，gate-on 明确返回 producer unavailable。P1M-C2a
已在纯 domain `backend/domain/agentthread/adaptivecontract` 实现 strict canonical
admission/decision codec 与 domain validation，application 保持 C1 validator wrapper 和 sentinel
alias，兼容既有 `errors.Is` 合同。P1M-C2b 已在 private `AdaptiveExecutionRepository` 实现 durable
bootstrap 的 exact-tuple commit/readback，并隔离 `adaptive.admission`、`adaptive.decision` 与
`workbench_control` control checkpoint，通用 writer/reader 不可写入或枚举这些保留事实。这仍是
P1M-B1 之后的内部合同边界。P1M-C3a 已把 gate-off coordinator 接入已 enrolled、fresh、顶层
Eino ADK `Execute`：输入解析成功后先读 durable replay，首次才提交固定 baseline
`execute/multi_step`，随后才允许创建 checkpoint store 与 Agent runtime；明确 `task` 和既有空
`RunKind` 顶层兼容形式在有效 Execute 身份及启动依赖已满足时，未 enrolled 保持 no-op。logical
Journal root 可以与当前 execution Run 不同。P1M-C3b 又把该 durable/replayed admission 与 decision
作为这一条 fresh Execute 链的 Plan capability 权威：Factory 在创建提示词和 middleware 前只覆盖
本地 Plan capability，因此 Todo prompt 与 Plan backend 同步受控；无 facts 的未接入路径保留历史
兼容，无效或不匹配 facts fail closed。P1M-C3c 又以同一 admission 的
`SubagentsAllowed=false` 同步关闭本地 Subagent prompt、limit middleware 与标准 Subagent tool
provider；provider 在解析 definition 或构建 child Agent 前返回 base tools，且不把私有能力信号传给
base provider。P1M-C3d 再把同一 fresh Execute 的旧 `mode`/`thinking_enabled`/`reasoning_effort`
影响中和为本地安全请求（thinking false、reasoning effort 空），使 primary/failover model option 与
provider-capability middleware 不再消费这些旧控制；持久化 Config 不变，也未引入新推理 policy。
P1M-C3e 按交付优先只退休 Journal 的两个旧 mode consumer：enrollment 仅允许明确
`runtime=eino_adk` 的 fresh 顶层 task；后续 ordinary MVP 已把 Attempt enrollment 与 projection
rollout/kill-switch 解耦，projection gate 只决定 healthy/disabled。completed 完整性指标
只以真实 `Enrolled && Completed` 为分母，不再读取 `Mode`。本切片未新增 metrics emitter，也未定义
新的 server inference policy。P1M-C3f 又把 public `CreateTaskThread`、`CreateRun` 与 top-level retry
的规范化结果收口为新写合同：server policy 仍先完成校验与合法 config/context 合并，但持久化前只从
顶层删除七个退休执行控制字段；`runtime=eino_adk`、模型、资源、Token Usage 与其它合法 opaque 配置
继续保留。P1M-C3g 再收口 child 新写：public `CreateRun` 拒绝 caller-owned child shape，只有
package-private trusted child seam 可以持久化 `ParentRunID > 0 && RunKind=subagent` 的 child；该新写
Config 也不再包含七个退休字段。Factory 对这一 exact durable child identity 在本地强制关闭 Plan、
Subagent、thinking 与 reasoning，并在 adaptive facts 投影后再次覆盖，因此旧历史 child Config 也保持
更安全的兼容行为。后续 C3h2d 已让 builtin/single-agent 内存 child writer 停止写入
`requested_policy`/`mode`，并让 production ADK consumer 统一忽略这两个顶层旧键；C3g 本身仍不等于
该退休完成。
继续按交付优先收口的 P1M-C3h1a 只修改三条已有恢复链的目标 Run 新写：Human
interaction resume、ordinary non-Journal lease recovery 和 Journal recovery 在写入新 Config
前，仅删除来源 Config 顶层的七个退休字段；`runtime`、模型、资源、Token Usage、
opaque 配置和 nested 同名业务字段全部保留。来源历史 Config 与 Context 原样保持，
该 C3h1a 切片本身不新增 Attempt enrollment，不把这些新 Run 接入 `ADKExecutor.Resume`，也不实现
legacy decoder、typed inheritance、IDL 或 UI；这些历史范围说明已由后续 C3h2a/C3h2b 的 enrolled
Resume typed inheritance 与原子 Attempt rollover 取代。
P1M-C3h2a 只连接 already-enrolled Journal recovery Resume：其 immediate source 必须具有
有效 fresh/typed durable bootstrap，target 在 ADK `buildRuntime` 前提交或 exact replay gate-off
`typed_inheritance` snapshot。该 C3h2a 切片当时不包含 Human rollover；后续 C3h2b 已补齐 enrolled
Human Resume。后续 C3h2c 又把同一 enrolled Resume bootstrap 收口为 typed-first、legacy-exact-miss
fallback：target exact replay 始终优先；只有 immediate source 的 durable bootstrap 精确返回
`ErrAdaptiveExecutionBootstrapNotFound`，coordinator 才读取 source Run 并用隔离的
`LegacyAdaptiveAdmissionDecoder` 严格解析已知 root legacy control。损坏、冲突或其它 repository
错误不会降级到 Config；decoder 只产生携带 source Run/generation/config digest/decoder version 的
保守 gate-off snapshot，baseline decision 仍由独立 producer 生成，并与 snapshot 在 target
lease/generation fence 下原子提交或 exact replay。后续 hop 继承该 durable typed snapshot，不再次解码，
来源 Config 不回写。后续 ordinary MVP 已补齐无 Attempt/disabled Attempt 的同类 bare/typed 恢复；
gate-on producer 仍 deferred，P1M 未 PASS；
P1L 与 whole-Thread DELETE hard guard 不变。
P1M-C3h2b 已用原子 Journal rollover 替换 C3h1b 临时门。canonical Human Resume 先以不可变
authority 做 full aggregate replay；exact replay 即使 source 生命周期已变化仍返回同一 target，
损坏或漂移 aggregate fail closed。首次写在同一 Thread-first 事务内追加 source resolved 与 physical
`journal.attempt.interrupted` terminal、终结 source Attempt 并释放 active slot，再创建带 source
Attempt/checkpoint lineage 的 pending target Attempt；该 lineage 继续由 C3h2a 在
`ADKExecutor.Resume` 的 `buildRuntime` 前消费为 typed bootstrap。physical helper 不进入公共
RunEvents、total/cursor 或 TaskDetail replay/live。phase-1 compatible-reader build `38ddbaf6f` 是首次
写入 `interrupted` 后的回滚下限，producer activation 为 `212546bc`。后续 ordinary MVP 已补齐
disabled/无 Attempt 的 lease rollover；gate-on producer 仍 deferred。`3c241d012` 已在 dev disposable MySQL 通过
same-key replay、same-key drift conflict 与 different-key single-winner 三项真实双连接验收；
`5408b680` 又通过 typed recovery race 与 legacy decoder recovery 的真实 dev MySQL 门禁，覆盖
原子单写、exact replay、漂移零增量和 durable readback。MySQL JSON 存储归一化在回读时经 typed
codec 重新 canonicalize 后核对既有 digest/fingerprint，非 MySQL 严格 canonical 检查保持不变。
P1M-C3h2d 已完成 production ADK 的旧 mode/policy consumer 退休：Factory、Middleware、标准
Subagent tool provider 与 builtin definition 都经 production-only `parseADKRuntimeConfig` 解析，忽略
顶层 `requested_policy`/`mode`；builtin/single-agent child writer 不再写入这两个键。显式
`subagent_enabled`、Plan、thinking、reasoning 与 model/provider server-owned 配置继续保留，历史
`ParseDeerFlowRuntimeConfig` 也只作为 legacy decoder/兼容 reader，不再驱动 production ADK 分支。
`b8d1c21b0` 同时交付 same-recovery rolling Plan repository foundation：单一 recovery Attempt 可按
B1→B2→B3 连续提交 Plan revision、item version、event sequence 与 checkpoint parent，历史 boundary
exact replay 不改写当前状态，rolling checkpoint 可成为下一 Attempt 的严格恢复来源，漂移均 fail
closed。后续 Application writer 已把 Eino Plan 工具写入 run-scoped overlay；工具调用完成后的
`AfterToolCalls` 内部 cancel 形成真实 Eino v3 runtime checkpoint，`ADKCheckpointStore` 再通过同一个
受 lease/generation/Attempt fence 的 transaction 原子提交 Plan high-watermark、PlanItem、追加 Event、
checkpoint 与 Attempt cursor。首次 Plan 使用 0→1 初始化，后续支持 B1/B2/B3 rolling；当前 Attempt
head 在分配新 ID 前 read-first replay，历史 exact replay 不改写当前状态。Plan 与 side-effect 同一
checkpoint boundary 混用会在任何 durable write 前 fail closed。enrolled typed Resume 继承 durable
source `PlanScopeRunID`，target checkpoint store/coordinator 沿用该 scope，恢复后的 Plan 写仍进入同一
atomic boundary，不回落 legacy Plan writer。repository/application Go 测试已通过；本轮 dev disposable
MySQL rolling gate 因缺少满足安全命名约束的隔离 DSN 保持 `NOT_VERIFIED`。
本轮 ordinary MVP 进一步把 fresh 顶层 Eino Task 的 Attempt enrollment 固定为 always-on：projection
gate 命中时写 healthy Attempt，gate off、依赖 nil 或判定错误时写 disabled Attempt 且强制关闭
snapshot；非 Eino、child 与其它 Run 保持既有边界。expired lease 对 healthy/non-disabled Attempt
继续走既有 Journal recovery，对 disabled Attempt 或无 Attempt source 则经 dedicated
`CreateRunBundle` 分支，在单个 Thread-first transaction 中把 source Run 标记 `interrupted`、仅追加
base terminal event，并创建 disabled target Attempt；已有 disabled source Attempt 同时 CAS
`interrupted`、释放 active slot，bare source 则创建 ordinal 1 target。Application 传递完整 source
checkpoint authority，repository 在事务锁内逐字段并按 JSON 语义重比对，阻断 pre-read 与 write
之间的 TOCTOU 漂移。bare target 的 Resume bootstrap 先 exact replay target，miss 后跳过不存在的
source durable bootstrap，只用 strict legacy decoder 读取 source Run，并继承 source
`PlanScopeRunID`；bare Plan boundary 已支持首次 commit、read-first replay 与 rolling。ordinary 的真实
dev MySQL gate 本轮未运行，明确保持 `NOT_VERIFIED`；rolling MySQL gate 也仍 `NOT_VERIFIED`。
gate-on producer 与真正的 server inference policy 仍未完成，P1M 仍未 PASS。historical runtime compatibility
与 package-private server-owned subagent seam 仍存在。P1L
继续 deferred，whole-Thread DELETE guard 仍 hard-disabled。

整 Thread DELETE route 与 IDL 仍保留，但在 dependency、workspace 授权和 path ID 校验后
统一返回 `503 thread_delete_temporarily_disabled`；handler 不读取 Thread 是否存在，也不调用
`DeleteThreadIfIdle`。P1L 完成并在移除 guard 的同一候选 SHA 上重验前，底层 idle cascade
仍不可从 canonical HTTP 到达；Artifact、Upload、Memory 等子资源删除不受影响。

canonical handler 只负责严格 HTTP 合同、公开投影、错误映射和脱敏结构化日志，
继续调用现有 `agentthread.ApplicationService`，不建立第二套状态机、数据库或执行器。
公共合同由 `idl/workbench/thread.thrift` 与 `thread_product.thrift` 定义；
`idl/workbench/task.thrift` 只保留 11 条 Scheduled Task 合同。

旧 `/api/workbench/task_threads/**` 的 36 条路由和本地 LangGraph Thread
`/api/threads/**` 的 23 条路由、stateless LangGraph `/api/runs/**` 的 10 条路由均已
不可达；ChatTask 全栈已退役，这些合同都不得恢复为 fallback。canonical
Run SSE 仅保留经审核的 LangGraph SDK-compatible event shape，不保留旧 HTTP
路由或 LangGraph runtime。内部 `CreateTaskThread` 应用用例仍被 canonical 首次提交、
Scheduled Task 和飞书入口复用，不等同于已退役的旧 HTTP/IDL 合同。

## 事实来源

1. 当前源码、IDL、迁移和运行时行为；
2. 相关测试与生成代码；
3. 本文件及当前有效 runbook/spec；
4. codebase-memory 与 Graphify 派生图谱；
5. 历史 plans/specs 和外部参考项目。

## 长期记忆工具

### codebase-memory

用于代码结构、符号搜索、调用链和变更影响。每次结论都要检查项目、索引新鲜
度和具体源码；架构决策应同步到版本控制文档，不能只保存在本机数据库。

### Graphify

用于本目录内精选长期上下文的概念和文档关系。只有长期事实变化后才更新；
输出保存在 ignored `graphify-out/`，不能代替受版本控制的事实文件。图谱不可用
或与文档冲突时直接读取源文档，并记录降级情况。

## 当前运行手册

- WorkbenchChat 当前事实：`docs/superpowers/context/workbench-chat.md`
- 本地调试：`docs/superpowers/runbooks/local-debug-and-test.md`
- dev 集成审计：`docs/superpowers/runbooks/dev-integration-audit.md`
- dev 预发布运维：`deploy/dev/README.md`
- Sandbox：`docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Guardrail：`docs/superpowers/runbooks/guardrail-audit-operations.md`
- Workbench canonical product client：
  `docs/superpowers/runbooks/workbench-canonical-product-client-validation.md`
- Agent Runtime：`docs/superpowers/specs/2026-06-28-eino-agent-runtime-guidance.md`

## 更新规则

出现以下变化时更新本文件或对应 ADR/runbook：

- 跨模块架构、所有权或依赖方向变化；
- 公共 API、IDL、持久化、安全或租户合同变化；
- 生产部署、故障恢复、密钥、迁移或发布流程变化；
- 后续任务必须持续遵守的新决策。

任务进度、临时日志、一次性命令输出和局部实现细节不进入本文件。发现过期事实
时在同一需求分支修正，并在审计报告中列出。
