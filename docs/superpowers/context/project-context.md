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
数据库和资源配置仍是合法输入。该冻结不代表后端 mode、ADK consumer 或恢复继承已经退休。

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
兼容，无效或不匹配 facts fail closed。真实 MySQL 双连接验收仍待显式
disposable DSN/DDL gate；`Resume`、legacy runtime、gate-on producer、runtime selector/handler、IDL
和 frontend/UI 未接；Subagent、reasoning/model inference、Journal enrollment/metrics 仍使用历史
consumer。历史 runtime controls 与 package-private server-owned subagent compatibility
seam 仍存在；P1M 尚未 PASS。P1L 继续 deferred，whole-Thread DELETE guard 仍 hard-disabled。

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
