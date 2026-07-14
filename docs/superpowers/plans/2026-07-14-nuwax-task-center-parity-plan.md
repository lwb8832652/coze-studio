# Nuwax 任务中心完整迁移实施计划

> **执行要求：** 按 TDD 的 RED-GREEN-REFACTOR 顺序逐项落地；生成代码使用
> 仓库既有 IDL 工具，不手工维护生成文件。

**目标：** 新增独立一级菜单 `任务中心`，完整迁移 Nuwax Agent/Workflow 定时
任务能力，并达到多租户、可恢复、多实例安全的生产级交付标准。

**架构：** 新增 `scheduledtask` 领域和 MySQL repository；应用层负责任务用例、
调度 worker 与 Agent/Workflow adapter；Workbench Task IDL 暴露合同；React 页面
使用生成 client 和 Coze Design 完成对齐交互。

---

## 任务 1：锁定 API 与调度行为合同

**文件：**

- 修改：`idl/workbench/task.thrift`
- 修改：`frontend/packages/arch/api-schema/__tests__/workbench-task-contract.test.ts`
- 新增：`backend/domain/scheduledtask/entity/task_test.go`
- 新增：`backend/domain/scheduledtask/service/schedule_test.go`

1. 先增加失败的 IDL 合同测试，覆盖十个 Task Center API。
2. 增加 schedule validation/next-time 的表驱动失败测试。
3. 运行 targeted tests，确认失败原因是合同和实现缺失。
4. 定义 IDL struct、enum、请求响应和 service methods。
5. 实现领域实体、状态转换、时区和下一次执行计算。
6. 生成前后端代码并让合同测试转绿。

## 任务 2：持久化与多实例领取

**文件：**

- 新增：`docker/atlas/migrations/20260714000200_scheduled_tasks.sql`
- 新增：`backend/domain/scheduledtask/repository/repository.go`
- 新增：`backend/domain/scheduledtask/repository/mysql.go`
- 新增：`backend/domain/scheduledtask/repository/mysql_test.go`

1. 先写 repository 失败测试，覆盖 CRUD、空间过滤、软删除、分页。
2. 写领取竞争和 idempotency 唯一键失败测试。
3. 创建 `scheduled_tasks` 与 `scheduled_task_executions` 表、索引和约束。
4. 实现事务领取、lease、执行记录和完成状态更新。
5. 运行 repository targeted tests 并验证 Atlas。

## 任务 3：应用用例与权限边界

**文件：**

- 新增：`backend/application/scheduledtask/service.go`
- 新增：`backend/application/scheduledtask/service_test.go`
- 修改：`backend/application/application.go`

1. 先写创建、编辑、启停、手动执行、删除和列表失败测试。
2. 覆盖非空间成员、跨空间 target、未发布 Workflow、非法 payload 和数量限制。
3. 实现应用服务，认证身份只从 context 获取。
4. 在应用启动和关闭生命周期中注册服务与 worker。
5. 确认所有权限和错误路径测试转绿。

## 任务 4：Agent 与 Workflow 执行适配

**文件：**

- 新增：`backend/application/scheduledtask/agent_executor.go`
- 新增：`backend/application/scheduledtask/workflow_executor.go`
- 新增：`backend/application/scheduledtask/executor_test.go`
- 新增：`backend/application/scheduledtask/worker.go`
- 新增：`backend/application/scheduledtask/worker_test.go`

1. 先写 Agent 新会话、保持会话、幂等 run 和失败记录测试。
2. 先写 Workflow 发布态异步执行、输入校验和失败关闭测试。
3. 先写到期领取、手动执行、不重复触发、重启恢复和 graceful shutdown 测试。
4. 实现两个 executor 和统一 dispatcher。
5. 实现 worker、有限并发和结构化日志/metrics。
6. 运行 targeted tests，让执行闭环转绿。

## 任务 5：HTTP handler 与生成客户端

**文件：**

- 新增：`backend/api/handler/coze/scheduled_task_service.go`
- 新增：`backend/api/handler/coze/scheduled_task_service_test.go`
- 修改：`backend/api/router/coze/api.go`
- 修改：`frontend/packages/arch/api-schema/src/idl/workbench/task.ts`（生成）
- 修改：`backend/api/model/workbench/task/task.go`（生成）

1. 先写 handler 失败测试，覆盖所有 API、参数映射、鉴权和错误码。
2. 运行 IDL 生成工具，不手工改生成文件。
3. 实现 handler 映射和路由注册。
4. 确认 handler 与客户端合同测试转绿。

## 任务 6：一级菜单、路由和页面服务

**文件：**

- 修改：`frontend/apps/coze-studio/src/components/workspace-sub-menu/menu.ts`
- 修改：`frontend/apps/coze-studio/src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx`
- 修改：`frontend/apps/coze-studio/src/routes/async-components.tsx`
- 修改：`frontend/apps/coze-studio/src/routes/index.tsx`
- 新增：`frontend/apps/coze-studio/src/pages/task-center/service.ts`
- 新增：`frontend/apps/coze-studio/src/pages/task-center/__tests__/service.test.ts`

1. 先写菜单、路由和 service 失败测试。
2. 新增 `任务中心` 一级菜单，位置在 `全部任务` 上方。
3. 注册 `/space/:space_id/task-center` 懒加载路由。
4. 使用生成 client 实现 Task Center service。
5. 确认菜单、路由和 service 测试转绿。

## 任务 7：Nuwax 对齐页面与交互

**文件：**

- 新增：`frontend/apps/coze-studio/src/pages/task-center/index.tsx`
- 新增：`frontend/apps/coze-studio/src/pages/task-center/task-form-modal.tsx`
- 新增：`frontend/apps/coze-studio/src/pages/task-center/schedule-editor.tsx`
- 新增：`frontend/apps/coze-studio/src/pages/task-center/execution-drawer.tsx`
- 新增：`frontend/apps/coze-studio/src/pages/task-center/index.module.less`
- 新增：`frontend/apps/coze-studio/src/pages/task-center/__tests__/task-center.test.tsx`

1. 先写列表 loading/empty/error、筛选、分页和所有操作的失败测试。
2. 写 Agent/Workflow 动态表单、保持会话和参数校验失败测试。
3. 实现页面骨架、表格、状态标签、操作与确认反馈。
4. 实现创建/编辑弹窗、周期编辑器和目标参数配置。
5. 实现执行记录抽屉及关联详情跳转。
6. 补齐响应式、键盘焦点、ARIA、disabled 和错误恢复。
7. 运行页面 targeted tests 并转绿。

## 任务 8：生产验收与追踪记录

**文件：**

- 更新：本设计文档的验收记录部分
- 更新：相关 P0 tracker（若仓库存在）

1. 运行相关 Go targeted tests、Vitest、TypeScript 和 lint。
2. 运行 Atlas hash/validate。
3. 启动前后端并使用 Codex in-app browser 登录测试账号。
4. 在个人空间和团队空间分别验证 Agent 与 Workflow 完整流程。
5. 验证跨空间权限、停用状态、重复手动点击、会话过期和服务重启恢复。
6. 记录 URL、账号、空间、可见状态、核心交互和控制台错误。
7. 仅在全部证据通过后声明完成并提交代码审核。
