# MCP Management Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Coze 现有 `/space/:space_id/tools` 页面内完成与 nuwax-ai MCP 管理等价的生产级闭环，不新增一级菜单、子页面或重复的数据源。

**Architecture:** 继续以 `application/mcptool` 和 MySQL catalog 作为 MCP 控制面唯一事实源，补齐来源、创建者和能力元数据；服务端从认证上下文派生空间与权限，并通过现有 Eino ADK MCP runtime 执行真实工具调用。前端只保留一个完整管理页，列表、创建、编辑、能力查看、测试、日志、导出和删除都使用抽屉或弹窗；账号设置中的 `MCP 配置` 复用同一组件的紧凑模式。

**Tech Stack:** Go + Hertz + GORM/MySQL + Eino ADK；React + TypeScript + Semi/Coze Design + Vitest；Atlas migration v0.35.0。

---

## 约束

- 不新增侧边栏一级菜单，不新增 `/mcp`、`/mcp/create`、`/mcp/:id` 等路由。
- 唯一完整页面为 `/space/:space_id/tools`。
- 不接受浏览器提交的 `creator_id`、空间角色、官方来源标识或能力清单作为可信事实。
- 自定义 MCP 可创建、编辑、启停、发现能力、测试、导出和删除；官方 MCP 只允许查看、启停和测试。
- 鉴权信息只允许写入或替换，响应、日志、测试结果和导出中均不得返回明文。
- `TestCall` 必须调用真实 MCP runtime，不保留 stub 成功响应。
- 本计划不包含 Git 提交、推送或合并步骤。

## Task 1: 扩展 MCP 数据合同和持久化投影

**Files:**

- Modify: `backend/api/model/workbench/tool/tool.go`
- Modify: `backend/application/mcptool/catalog.go`
- Modify: `backend/application/mcptool/catalog_mysql.go`
- Modify: `backend/application/mcptool/catalog_mysql_test.go`
- Modify: `backend/application/mcptool/service_test.go`
- Create: `docker/atlas/migrations/20260714000100_mcp_management_parity.sql`
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/tool.ts`

- [x] 1. 在 catalog/service 测试中先加入以下失败用例：`creator_id` 和 `source_type` 持久化；tools/resources/prompts 独立 JSON 能力投影；旧行默认 `custom`；敏感 auth 仍被掩码。
- [x] 2. 运行 `cd backend && go test ./application/mcptool -run 'TestMySQLCatalog|TestApplicationService' -count=1`，确认新断言失败且原因是合同缺失。
- [x] 3. 新增模型字段：`creator_id`、`source_type`、`resources`、`prompts`；来源枚举只允许 `custom`/`official`。
- [x] 4. 新增 Atlas migration，为现有数据回填 `source_type='custom'`，新增 creator/source 索引和能力 JSON 列；不得修改已有 migration。
- [x] 5. 更新 MySQL record/domain 转换和前端 schema，确保列表、详情与注册表返回一致的有界元数据。
- [x] 6. 重跑 targeted tests，确认通过。

## Task 2: 服务端空间授权和官方资源约束

**Files:**

- Create: `backend/application/mcptool/authorization.go`
- Create: `backend/application/mcptool/authorization_test.go`
- Modify: `backend/application/mcptool/service.go`
- Modify: `backend/application/mcptool/service_test.go`
- Modify: `backend/application/application.go`

- [x] 1. 先写失败测试：未登录拒绝；非空间成员拒绝；空间成员可读；只有空间 Owner/Admin 可写；普通成员不可创建、编辑、删除；官方 MCP 不可编辑和删除；启停仍按管理权限校验。
- [x] 2. 运行 `cd backend && go test ./application/mcptool -run 'TestAuthorization|TestApplicationService' -count=1`，确认失败。
- [x] 3. 新增最小授权接口，从 `ctxutil.GetUIDFromCtx` 获取当前用户，并通过后端空间成员/角色投影判断权限；禁止信任请求中的 user/role 字段。
- [x] 4. 将授权依赖注入 `mcptool.Components`；读取、写入、删除、测试、导出和日志查询均 fail closed。
- [x] 5. 创建时由服务端写入 `creator_id` 和 `source_type=custom`；更新时不可篡改这两个字段。
- [x] 6. 重跑 targeted tests，确认通过。

## Task 3: 用真实 MCP runtime 替换测试调用 stub

**Files:**

- Create: `backend/application/mcptool/runtime.go`
- Create: `backend/application/mcptool/runtime_test.go`
- Modify: `backend/application/mcptool/service.go`
- Modify: `backend/application/mcptool/service_test.go`
- Modify: `backend/application/application.go`

- [x] 1. 先把现有 stub 断言改为失败测试：测试调用必须把 `space_id/server_id/tool_name/arguments` 委托给 runtime；runtime 错误向上返回；成功结果有大小限制；调用后健康状态按真实结果更新。
- [x] 2. 运行 `cd backend && go test ./application/mcptool -run 'TestApplicationServiceTestCall|TestRuntime' -count=1`，确认现有 `mcp_test_call_stub` 逻辑导致失败。
- [x] 3. 定义 `RuntimeExecutor` 控制面接口和有界返回类型，适配现有 `ADKMCPRuntimeExecutor.InvokeADKMCPRuntimeTool`。
- [x] 4. 在应用装配阶段注入 executor，删除 stub JSON；未配置 runtime 时 fail closed，不伪造成功。
- [x] 5. 对结果执行长度、字段和错误脱敏，禁止透出 provider 原始响应、认证信息和隐藏运行参数。
- [x] 6. 重跑 targeted tests，确认通过。

## Task 4: 服务端能力发现、安全导出和审计查询

**Files:**

- Create: `backend/application/mcptool/discovery.go`
- Create: `backend/application/mcptool/discovery_test.go`
- Modify: `backend/application/mcptool/service.go`
- Modify: `backend/application/mcptool/service_test.go`
- Modify: `backend/api/model/workbench/tool/tool.go`
- Modify: `backend/api/handler/coze/workbench_mcp_tool_service.go`
- Modify: `backend/api/handler/coze/workbench_mcp_tool_service_test.go`
- Modify: `backend/api/router/coze/api.go`
- Modify: `frontend/packages/arch/api-schema/src/idl/workbench/tool.ts`

- [ ] 1. 先写失败测试：发现操作从 server config 建立真实 MCP client 并返回 tools/resources/prompts；能力写回 catalog；导出不含 auth；审计查询只返回 bounded metadata；跨空间访问失败。
- [ ] 2. 运行 `cd backend && go test ./application/mcptool ./api/handler/coze -run 'Test.*MCP' -count=1 -gcflags='all=-N -l'`，确认失败。
- [ ] 3. 复用 Eino MCP client 工厂实现 stdio(npx/uvx)、SSE、Streamable HTTP 的真实发现；不为 `component` 类型新增伪实现。
- [ ] 4. 新增单资源操作：`POST /api/workbench/mcp_tools/:server_id/discover`、`GET /api/workbench/mcp_tools/:server_id/export`、`GET /api/workbench/mcp_tools/:server_id/audit_events`。
- [ ] 5. 导出只包含可重新导入的安全配置与能力摘要，不包含 auth、内部 URI、provider body；审计只返回时间、状态、耗时、工具名和安全错误摘要。
- [ ] 6. 重跑 targeted tests，确认通过。

## Task 5: 前端 service 和单页交互测试

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/tools/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tools/__tests__/tools-service.test.ts`
- Modify: `frontend/apps/coze-studio/src/pages/tools/__tests__/tools.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tools/__tests__/mcp-server-form.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tools/__tests__/mcp-capabilities.test.tsx`

- [ ] 1. 先重写旧的“无创建/测试/删除”断言，覆盖：自定义/官方筛选、搜索、状态筛选、创建抽屉、编辑抽屉、启停、发现、能力 tabs、schema 测试、日志、导出、删除确认、权限只读、loading/empty/error/refresh。
- [ ] 2. 为 service 增加 discover/export/audit 请求断言以及空间 ID、server ID 参数校验。
- [ ] 3. 运行 `cd frontend/apps/coze-studio && npm run test -- src/pages/tools/__tests__/tools.test.tsx src/pages/tools/__tests__/tools-service.test.ts src/pages/tools/__tests__/mcp-server-form.test.tsx src/pages/tools/__tests__/mcp-capabilities.test.tsx`，确认测试因 UI/接口尚未实现而失败。

## Task 6: 实现 Coze 原有工具页中的完整 MCP 管理

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/tools/index.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/mcp-settings-panel.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/tools/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/tools/components/mcp-server-card.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tools/components/mcp-server-form-drawer.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tools/components/mcp-capability-drawer.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tools/components/mcp-tool-test-modal.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tools/components/mcp-audit-drawer.tsx`
- Create: `frontend/apps/coze-studio/src/pages/tools/tools.module.less`

- [ ] 1. 将 `MCPToolSettingsPanel` 改为 `full`/`compact` 两种展示模式；完整页使用 `full`，账号设置继续使用 `compact`，不新增菜单和路由。
- [ ] 2. 完整页顶部实现标题、说明、刷新、搜索、创建按钮、自定义/官方分段和状态筛选；卡片展示来源、创建者、连接类型、健康状态、能力数量与更新时间。
- [ ] 3. 创建/编辑抽屉支持名称、描述、连接类型、JSON 配置和 auth；保留服务端掩码语义；提供保存、保存并启用、取消和未保存离开保护。
- [ ] 4. 能力抽屉提供工具、资源、提示词、运行日志四个 tab；工具参数根据 JSON Schema 生成测试表单，结果展示 loading/success/error/copy 状态。
- [ ] 5. 实现真实启停、能力发现、安全导出和删除确认；官方项隐藏编辑/删除；无写权限时统一 readonly，并明确原因。
- [ ] 6. 使用 Coze Design/Semi 既有 token 完成桌面与窄屏布局，覆盖键盘焦点、ARIA、disabled、loading、empty、error 和 refresh 状态。
- [ ] 7. 重跑 Task 5 的全部 Vitest，确认通过。

## Task 7: 迁移、类型与后端回归验证

**Files:**

- Modify only if failures reveal defects in files changed above.

- [ ] 1. 运行 `cd docker/atlas && atlas migrate hash`，更新 migration hash。
- [ ] 2. 运行 `atlas migrate validate --dir file://docker/atlas/migrations`，确认 migration 有效。
- [ ] 3. 运行 `cd backend && go test ./application/mcptool ./api/handler/coze ./api/router/coze -count=1 -gcflags='all=-N -l'`。
- [ ] 4. 运行 `cd frontend/apps/coze-studio && npx tsc --noEmit --project tsconfig.json`。
- [ ] 5. 重跑 Task 5 的全部 Vitest，记录实际通过数量；不得用跳过测试代替修复。

## Task 8: 内置浏览器双环境页面验收

**Files:**

- Modify only if browser evidence reveals defects in files changed above.

- [ ] 1. 使用 Codex in-app browser 登录 nuwax-ai `http://localhost/`，记录 MCP 单页的筛选、创建、编辑、部署、能力测试、导出、日志和删除行为。
- [ ] 2. 使用 Coze 测试账号进入 `http://localhost:8080/space/<space_id>/tools`，验证同一关键流程；确认没有新增一级菜单或子路由。
- [ ] 3. 验证自定义 MCP：创建 stdio 或 HTTP 配置、保存并启用、发现能力、执行真实工具测试、查看日志、安全导出、停用和删除。
- [ ] 4. 验证官方 MCP：只读编辑/删除边界、启停与测试；验证普通成员看得到但不能写，非成员和跨空间请求失败。
- [ ] 5. 验证账号设置中的 `MCP 配置` 紧凑模式仍可加载和启停，不出现重复导航。
- [ ] 6. 记录 URL、账号/空间、可见状态、交互结果和控制台错误；最后成功调用浏览器 `tabs.finalize`。

## 完成定义

- [ ] `/space/:space_id/tools` 是唯一 MCP 管理页，所有操作均在抽屉/弹窗内闭环。
- [ ] 前端没有新增一级菜单、重复 MCP 入口或子路由。
- [ ] 数据来自真实 catalog，测试来自真实 MCP runtime，权限来自服务端空间事实。
- [ ] auth、运行原始数据和内部配置不会通过列表、日志、测试或导出泄露。
- [ ] targeted Go tests、Vitest、TypeScript、Atlas validate 和 in-app browser 验收全部有通过证据。
