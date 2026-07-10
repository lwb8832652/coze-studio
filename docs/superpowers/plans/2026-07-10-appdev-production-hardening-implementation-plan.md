# AppDev Production Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 消除 AppDev 二期验收发现的生产阻断和高风险缺陷，并以自动化和浏览器证据重新验收。

**Architecture:** 先修复确定性数据损坏与授权问题，再把宿主运行时隔离在 debug adapter 后，接入生产 Runner/Preview Gateway 和 DB/OSS repository。所有行为先写失败测试，再做最小实现。

**Tech Stack:** Go、Hertz、React、TypeScript、Vitest、Go test、现有 workspace service、infra/storage。

---

## Task 1: Cancel safety

**Files:**
- Modify: `backend/infra/appdev/sse_broker.go`
- Modify: `backend/infra/appdev/sse_broker_test.go`

- [ ] 写失败测试：生成器在取消后返回时，`src/App.tsx` 内容保持不变。
- [ ] 运行 `go test ./infra/appdev -run Cancel -count=1`，确认因取消后仍写文件而失败。
- [ ] 在写入和广播完成事件前检查 context、request ID，并持久化 cancelled history。
- [ ] 重跑测试并确认通过。

## Task 2: Frontend request ordering

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-files.ts`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-runtime.ts`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-projects.ts`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-project-info.ts`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/hooks/use-app-dev-model-selector.ts`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-ide.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-request-ordering.test.tsx`

- [ ] 写失败测试：A/B 文件请求逆序完成时仍显示 B，保存只能写 B 内容。
- [ ] 写失败测试：空间、项目和运行状态旧响应不能覆盖新请求。
- [ ] 为各 Hook 增加 generation ref，并只接受当前 generation 的结果和 finally。
- [ ] 运行 AppDev Vitest 与 TypeScript 检查。

## Task 3: Workspace authorization

**Files:**
- Modify: `backend/application/workspace/workspace.go`
- Modify: `backend/application/workspace/workspace_test.go`
- Modify: `backend/api/handler/coze/app_dev_service.go`
- Modify: `backend/api/handler/coze/app_dev_service_test.go`

- [ ] 写 Owner/Admin/Member、`allow_develop=false` 和非成员授权矩阵失败测试。
- [ ] 增加 AppDev read/write/manage 授权请求和结果。
- [ ] handler 按路由操作级别调用授权器。
- [ ] 运行 workspace 与 AppDev handler 测试。

## Task 4: Runtime lifecycle, redaction, and limits

**Files:**
- Modify: `backend/infra/appdev/runtime_manager.go`
- Create: `backend/infra/appdev/runtime_manager_test.go`
- Modify: `backend/application/appdev/build.go`
- Modify: `backend/infra/appdev/local_store.go`
- Modify: `backend/api/handler/coze/app_dev_service.go`
- Modify: `backend/application/appdev/project.go`

- [ ] 写失败测试：keep-alive 过期和归档会停止并删除 runtime entry。
- [ ] 写失败测试：SIGTERM 超时后执行强制终止。
- [ ] 写失败测试：日志和构建错误隐藏 token、宿主路径和 object URI。
- [ ] 写失败测试：ZIP/上传文件数量和导出大小超过限制时拒绝。
- [ ] 实现 bounded redactor、资源常量、runtime reaper 和归档清理编排。
- [ ] 运行 Go race tests。

## Task 5: Runner boundary and preview safety

**Files:**
- Modify: `backend/application/appdev/init.go`
- Modify: `backend/infra/appdev/runtime_manager.go`
- Create: `backend/infra/appdev/remote_runtime_manager.go`
- Create: `backend/infra/appdev/remote_runtime_manager_test.go`
- Modify: `frontend/apps/coze-studio/src/pages/app-dev/components/preview-panel.tsx`
- Create: `frontend/apps/coze-studio/src/pages/app-dev/__tests__/app-dev-preview-security.test.tsx`

- [ ] 写失败测试：非 debug 环境不能构造 Host Runtime。
- [ ] 写 Remote Runner 合同测试：请求 scope、超时、状态和 Preview Gateway URL。
- [ ] 实现显式 debug Host Runner 与生产 Remote Runner 选择器。
- [ ] 拒绝 loopback Preview URL；iframe 增加最小 sandbox 权限。
- [ ] 运行后端测试、Vitest 和类型检查。

## Task 6: DB and OSS persistence

**Files:**
- Create: `backend/domain/appdev/repository_persistent.go`
- Create: `backend/infra/appdev/persistent_store.go`
- Create: `backend/infra/appdev/persistent_store_test.go`
- Modify: `backend/application/appdev/init.go`
- Modify: `backend/application/application.go`
- Add: `docker/atlas/migrations/*_appdev.sql`

- [ ] 写失败测试：新 store 实例可恢复项目、文件、快照和聊天历史。
- [ ] 定义项目/会话/运行记录数据库模型和 OSS object key。
- [ ] 实现 DB metadata repository 与 OSS file repository。
- [ ] 初始化时 production 使用 persistent store，debug 可显式使用 local store。
- [ ] 校验 Atlas hash/validate，运行 repository integration tests。

## Task 7: Final acceptance

- [ ] 运行前端 AppDev 与菜单测试、TypeScript 检查。
- [ ] 运行 AppDev application、infra、workspace、handler tests 和 race tests。
- [ ] 使用内置浏览器验收创建、编辑、取消、预览、发布、导出和页面回归。
- [ ] 使用恶意 ZIP、恶意 package、权限矩阵和服务重启场景验收。
- [ ] 更新二期计划、设计和 runbook，只在全部证据通过后恢复生产完成结论。
