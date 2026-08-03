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

### IM Channels

当前只支持飞书官方 Go SDK。配置按工作空间隔离，secret 加密且不回显；外部
事件先持久化去重，再进入异步 Agent 流程。

### dev 预发布部署

dev migration 在远程 push 前由维护者本机执行。第二次集成审计先固定
`origin/dev` 基准、本地 `dev` 目标、ACR 当前两张 `:dev` 镜像的一致 revision 和
实际部署区间，逐项审阅区间内 migration、数据库副作用及旧应用兼容性，并取得
只读 `migrate status` 证据。报告给出两个具体的 40 位 SHA；用户第二次明确确认后，
Codex 只能把这两个 SHA 交给 `deploy/dev/publish-dev.sh`。

`publish-dev.sh` 固定使用
`arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e`。
脚本从 exact target SHA 创建受限快照，依次 validate、status、forward apply；随后
重新检查干净工作区、目标 HEAD，并通过第二次 fetch 的 `FETCH_HEAD` 核对远程基准，
最后只做 exact-SHA 非 force push。Atlas 输出经脱敏后回放。Push 成功后脚本立即
退出，不等待 Actions、不 dispatch，也不调用宝塔。

Atlas migration credential 只存在于仓库外、非 symlink、模式严格为 `600` 的本地
env 文件；GitHub 不持有该 credential，也不连接数据库。预发布服务器仍通过
`app.env` 持有应用运行时 DSN，但不持有 migration credential，也不安装或运行 Atlas。
dev MySQL 不支持 TLS 时允许使用无 TLS 连接，但公共网络会暴露 credential 和 schema
流量；必须限制来源 IP 或网络路径，并使用专用最小权限 migration 账号，禁止 root。
可以提供私网或 TLS 时优先迁移到更安全的连接方式。

远程 push 后，GitHub Actions 只执行
`preflight -> build-server/build-web -> verify-images -> promote -> deploy`：构建并验证
两个 exact-SHA 不可变镜像，晋级两张 `:dev` 标签，再调用宝塔 webhook。Workflow
和服务器继续核对前后端 OCI revision，服务器在记录成功前还会确认两个运行容器的
实际 image ID 一致。`workflow_dispatch` 只重放已有不可变镜像，可处理区间内含
migration 的已推送 target，但不迁移数据库，也不执行 down migration。

第二次确认只授权报告中的 forward apply、exact push，以及该 push 触发的镜像和
宝塔副作用。Baseline、repair、backfill、down migration、数据库重试、生产发布和
配置变更都要单独授权。Apply 成功后若远程竞态或 push 失败，schema 可能领先于
远程代码；禁止 force 或直接 push，必须重新审计并取得新确认。Forward migration
必须兼容暂时继续运行的旧应用。

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

`/api/workbench/threads/**` 是 Workbench UI 唯一公共 HTTP 合同，共 47 条 always-on
Thread、Run、Message、Upload、Artifact、Memory、Token Usage、Guardrail 和 MCP
Runtime Audit 路由。前端页面服务统一委托给进程内唯一
`canonicalThreadClient`；不存在运行时 client selector、canonical 路由开关或旧 HTTP
fallback。session principal 和 path resource 决定身份与资源归属，workspace 请求
使用 `X-Coze-Space-ID` 并由服务端再次授权。

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
