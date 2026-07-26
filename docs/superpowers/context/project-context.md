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

## 主要产品域

- 任务与 Agent Workbench：任务、消息、运行、恢复、工具、产物和长期记忆；
- 工作空间与系统管理：成员、角色、系统配置、模型和管理员能力；
- Skill 与 MCP：配置、版本、授权、健康状态和运行时装配；
- AppDev 与 Sandbox：项目文件、构建、预览、Provider 和安全网关；
- 通知与计划任务：可靠通知、公告、定时执行、幂等和重试；
- 计费与配额：服务端事实、审计和安全边界。

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

- 本地调试：`docs/superpowers/runbooks/local-debug-and-test.md`
- dev 集成审计：`docs/superpowers/runbooks/dev-integration-audit.md`
- Sandbox：`docs/superpowers/runbooks/sandbox-control-plane-operations.md`
- Guardrail：`docs/superpowers/runbooks/guardrail-audit-operations.md`
- Agent Runtime：`docs/superpowers/specs/2026-06-28-eino-agent-runtime-guidance.md`

## 更新规则

出现以下变化时更新本文件或对应 ADR/runbook：

- 跨模块架构、所有权或依赖方向变化；
- 公共 API、IDL、持久化、安全或租户合同变化；
- 生产部署、故障恢复、密钥、迁移或发布流程变化；
- 后续任务必须持续遵守的新决策。

任务进度、临时日志、一次性命令输出和局部实现细节不进入本文件。发现过期事实
时在同一需求分支修正，并在审计报告中列出。
