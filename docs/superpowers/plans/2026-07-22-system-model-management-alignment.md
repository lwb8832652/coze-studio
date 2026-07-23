# System Model Management Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Coze Studio 的 `/system/models` 中交付与 Nuwax 模型配置等价的生产级管理闭环，包括筛选、排序、新增、编辑、连通性测试、启停、授权、删除保护、多端点路由和运行时访问控制，同时保持 Coze 的工作空间与用户权限模型。

**Architecture:** `idl/admin/config.thrift` 是管理 API 的唯一合同；`modelmgr` 负责模型元数据、端点、凭据、授权与运行时查询；系统管理 handler 只做鉴权、参数绑定与错误投影。模型密钥使用独立 keyring 加密，列表和详情只返回 `has_api_key`。Nuwax 的角色/用户组授权映射为 Coze 的工作空间/用户授权，所有运行入口统一经过可用性、场景和授权过滤。

**Tech Stack:** Go 1.x、Hertz、GORM Gen、MySQL、Atlas、Thrift/Hz、React 18、TypeScript、Semi/Coze Design、Vitest、Testing Library、Codex in-app browser。

---

## 0. 已确认边界

- 设计依据：`docs/superpowers/specs/2026-07-22-system-model-management-alignment-design.md`。
- 参考页面：Nuwax `http://localhost/system/model/manage`。
- 目标页面：Coze `http://localhost:8080/system/models`。
- 保留 Nuwax 的模型管理能力，不引入 Nuwax 的全局角色、用户组、计费或模型监控模块。
- 授权对象只有 `workspace` 与 `user`；未设置授权时使用 `all`，兼容历史模型。
- 系统管理员身份必须由服务端会话事实校验，前端隐藏入口不能代替后端鉴权。
- API Key 永不回显；编辑时空值表示保留原密钥，显式 `clear_api_key=true` 才允许清除。
- 历史模型默认启用、全场景、全员可用、稳定排序；历史 connection 映射为权重 `1` 的主端点。
- 删除前检查内置知识模型、Agent 草稿/版本、Workflow、Workbench、AppDev 等真实引用；有引用返回 `409` 和有限依赖摘要。
- 不手工长期维护生成代码；IDL、Go model/router 与 TypeScript schema 必须由仓库生成器产生。

## 1. 完成标准

- [ ] 列表支持关键词、供应商、能力类型、状态、授权范围筛选，支持重置、刷新、分页和拖拽排序。
- [ ] 新增/编辑支持供应商、名称、模型标识、描述、能力、推理模式、Token 限制、Function Call 模式、状态、场景、协议、路由策略和多端点。
- [ ] 每个端点支持 URL、API Key、权重、启用状态和独立连通性测试。
- [ ] 模型支持启停，停用模型不再出现在 Agent、Workflow、Workbench、AppDev 的可选列表和运行构建链路。
- [ ] 模型支持工作空间/用户授权，未授权用户通过直接模型 ID 调用时也被服务端拒绝。
- [ ] Round Robin 与 Weighted Round Robin 具备并发安全、失败切换和可观测错误摘要。
- [ ] 删除被引用模型时返回 `409`；无引用模型删除时同时软删除端点和授权记录。
- [ ] API 响应、日志、错误、审计和浏览器状态中均不出现明文密钥。
- [ ] `/system/models` 在桌面和窄屏下完成 loading、empty、error、disabled、readonly、refresh 和表单校验状态。
- [ ] 使用内置浏览器完成 Nuwax/Coze 同视口、同状态对比，并记录 URL、账号、空间、交互结果和控制台错误。

## Task 1: 扩展模型管理 IDL 与生成客户端

**Files:**

- Modify: `idl/admin/config.thrift`
- Generated: `backend/api/model/admin/config/config.go`
- Generated: `backend/api/handler/coze/config_service.go`
- Generated: `backend/api/router/coze/config_service.go`
- Generated: `frontend/packages/arch/api-schema/src/idl/admin/config.ts`
- Test: `frontend/packages/arch/api-schema/__tests__/admin-model-management-contract.test.ts`

- [ ] 先新增合同测试，断言 IDL 中存在 provider、endpoint、grant、dependency DTO，以及 detail/update/test/status/sort/grants API；断言响应 DTO 不存在 `api_key` 字段，只存在 `has_api_key`。
- [ ] 运行：

```bash
cd frontend/packages/arch/api-schema
npm run test -- admin-model-management-contract.test.ts
```

预期：测试失败，提示缺少新 DTO/方法。

- [ ] 在 `config.thrift` 中增加以下核心合同，并保留旧 `ListModels/CreateModel/DeleteModel` 路由兼容：

```thrift
enum ModelAccessMode { ALL = 1, RESTRICTED = 2 }
enum ModelGrantSubjectType { WORKSPACE = 1, USER = 2 }
enum ModelRoutingStrategy { ROUND_ROBIN = 1, WEIGHTED_ROUND_ROBIN = 2 }

struct ModelEndpointInput {
  1: optional i64 id (api.js_conv='true')
  2: required string base_url
  3: optional string api_key
  4: optional bool clear_api_key
  5: required i32 weight
  6: required bool enabled
  7: required i32 sort_order
}

struct ModelEndpointView {
  1: required i64 id (api.js_conv='true')
  2: required string base_url
  3: required bool has_api_key
  4: required i32 weight
  5: required bool enabled
  6: required i32 sort_order
}
```

- [ ] 增加 `ListModelProviders`、`GetModelDetail`、`UpdateModel`、`TestModelEndpoint`、`UpdateModelStatus`、`UpdateModelSort`、`GetModelGrants`、`SaveModelGrants` 和增强后的 `DeleteModel`。
- [ ] 所有 ID 使用 `api.js_conv='true'`；列表过滤和分页有明确上限；授权保存使用完整替换语义，避免部分写入。
- [ ] 生成 Go 合同与路由：

```bash
cd backend
hz update -idl ../idl/api.thrift -enable_extends
```

- [ ] 生成 TypeScript schema：

```bash
cd frontend/packages/arch/api-schema
npm run update
```

- [ ] 再运行合同测试，预期通过；检查生成 diff 中没有手写逻辑和无关 IDL 变化。

## Task 2: 建立模型元数据、端点和授权持久化

**Files:**

- Create: `docker/atlas/migrations/20260722000100_system_model_management.sql`
- Modify: `backend/types/ddl/gen_orm_query.go`
- Generated: `backend/bizpkg/config/modelmgr/internal/model/model_instance.gen.go`
- Generated: `backend/bizpkg/config/modelmgr/internal/model/model_instance_endpoint.gen.go`
- Generated: `backend/bizpkg/config/modelmgr/internal/model/model_instance_grant.gen.go`
- Generated: `backend/bizpkg/config/modelmgr/internal/query/model_instance.gen.go`
- Generated: `backend/bizpkg/config/modelmgr/internal/query/model_instance_endpoint.gen.go`
- Generated: `backend/bizpkg/config/modelmgr/internal/query/model_instance_grant.gen.go`
- Test: `backend/bizpkg/config/modelmgr/persistence_test.go`

- [ ] 先写 repository 测试，覆盖状态、排序、场景、端点顺序、软删除、授权唯一约束和事务回滚。
- [ ] 运行：

```bash
cd backend
go test ./bizpkg/config/modelmgr -run 'TestModelManagementPersistence' -count=1
```

预期：因表和 repository 方法不存在而失败。

- [ ] 迁移为 `model_instance` 增加：`description`、`status`、`sort_order`、`creator_id`、`protocol`、`routing_strategy`、`access_mode`、`scenario_json`、`reasoning_mode`、`function_call_mode`、`max_context_tokens`、`max_output_tokens`。
- [ ] 新建 `model_instance_endpoint`：`id`、`model_id`、`base_url`、`api_key_envelope`、`api_key_fingerprint`、`weight`、`enabled`、`sort_order`、时间戳、软删除；建立 `(model_id, sort_order)` 索引。
- [ ] 新建 `model_instance_grant`：`id`、`model_id`、`subject_type`、`subject_id`、时间戳、软删除；建立 `(model_id, subject_type, subject_id, deleted_at)` 唯一约束和 subject 反查索引。
- [ ] SQL 为历史记录设置 `status=1`、`access_mode='all'`、`routing_strategy='round_robin'`、`sort_order=id`、全场景默认值；不在 SQL 中处理或复制明文 API Key。
- [ ] 将两张新表加入 `path2Table2Columns2Model["bizpkg/config/modelmgr/internal/query"]`，生成 GORM model/query：

```bash
cd backend
MYSQL_DSN="$MYSQL_DSN" go run ./types/ddl/gen_orm_query.go
```

- [ ] 更新 Atlas hash 并验证：

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

预期：migration hash 和 validate 均成功。

## Task 3: 实现模型密钥 keyring、脱敏与历史迁移

**Files:**

- Create: `backend/bizpkg/config/modelmgr/credential.go`
- Create: `backend/bizpkg/config/modelmgr/credential_test.go`
- Modify: `backend/bizpkg/config/modelmgr/modelmgr.go`
- Modify: `backend/bizpkg/config/modelmgr/model_save.go`
- Modify: `backend/bizpkg/config/modelmgr/model_get.go`
- Modify: `backend/bizpkg/config/modelmgr/internal/model/model_instance.gen.go`

- [ ] 先写测试覆盖：缺少 keyring fail closed、加解密、错误 AAD 拒绝、密钥轮换 rewrap、列表/详情不回显、空 key 保留、显式清除、历史 connection 迁移。
- [ ] 定义独立环境变量：

```go
const (
    ModelCredentialKeysJSONEnv = "MODEL_CREDENTIAL_KEYS_JSON"
    ModelCredentialActiveKeyIDEnv = "MODEL_CREDENTIAL_ACTIVE_KEY_ID"
)
```

- [ ] 复用 `backend/infra/sandbox.CredentialCodec`，AAD 使用 `model/<modelID>/endpoint/<endpointID>/api-key`，fingerprint 只用于变更识别，不用于鉴权。
- [ ] `ModelConfig` 注入窄接口：

```go
type CredentialCodec interface {
    Encrypt(providerKey string, field sandbox.CredentialField, plaintext []byte) (string, error)
    Decrypt(providerKey string, field sandbox.CredentialField, envelope string) ([]byte, error)
    Rewrap(providerKey string, field sandbox.CredentialField, envelope string) (sandbox.CredentialRewrapResult, error)
    FingerprintCredential(plaintext []byte) (string, error)
}
```

- [ ] 新建/更新端点在同一事务中加密；读取列表只投影 `has_api_key`；运行时按需解密后立即构造 provider client，不把明文写入结构化日志。
- [ ] 增加幂等 `MigrateLegacyCredentials(ctx)`：读取旧 `connection`，创建主端点并加密 API Key，成功后清空旧 JSON 中的密钥；失败记录 model ID 与错误类别，不记录密钥或 connection body。
- [ ] 运行：

```bash
cd backend
go test ./bizpkg/config/modelmgr -run 'TestModelCredential|TestLegacyModelCredentialMigration' -count=1
```

预期：所有凭据与迁移测试通过。

## Task 4: 建立供应商目录与自定义模型元数据解析

**Files:**

- Create: `backend/bizpkg/config/modelmgr/provider_catalog.go`
- Create: `backend/bizpkg/config/modelmgr/provider_catalog_test.go`
- Modify: `backend/bizpkg/config/modelmgr/extra.go`
- Modify: `backend/bizpkg/config/modelmgr/model_save.go`

- [ ] 先写表驱动测试，覆盖豆包、OpenAI、Claude、DeepSeek、Gemini、Ollama、Qwen 的协议、模型类、Base URL 能力、Function Call 和多模态默认值。
- [ ] 定义后端唯一 provider catalog，前端通过 `ListModelProviders` 获取，不在 UI 复制枚举。
- [ ] 允许管理员录入 catalog 中不存在的模型标识，但 provider/protocol 必须在服务端支持矩阵中；由输入构造 `ModelProvider`、`DisplayInfo`、`ModelAbility` 和参数，而不是强依赖 `ModelMetaConf.GetModelMeta`。
- [ ] 对 unsupported protocol、空 identifier、重复 provider+identifier、非法 token 范围返回稳定业务错误。
- [ ] 运行：

```bash
cd backend
go test ./bizpkg/config/modelmgr -run 'TestProviderCatalog|TestResolveModelMetadata' -count=1
```

预期：catalog 和自定义模型测试通过。

## Task 5: 实现管理查询、筛选、分页、启停与排序

**Files:**

- Create: `backend/bizpkg/config/modelmgr/management.go`
- Create: `backend/bizpkg/config/modelmgr/management_test.go`
- Modify: `backend/bizpkg/config/modelmgr/model_get.go`
- Modify: `backend/bizpkg/config/modelmgr/model_save.go`

- [ ] 先写测试覆盖组合筛选、模糊搜索、分页上限、稳定排序、详情投影、新增/编辑事务、启停和批量排序冲突。
- [ ] 实现窄管理合同：

```go
type ListModelsFilter struct {
    Keyword string
    Provider string
    Capability string
    Status *bool
    AccessMode string
    Offset int
    Limit int
}

type EndpointSecretPatch struct {
    Value *string
    Clear bool
}
```

- [ ] 所有写操作使用数据库事务；端点、授权与主记录任一失败必须整体回滚。
- [ ] 排序 API 接收完整的相邻顺序列表并锁定受影响行，拒绝重复 ID、缺失 ID 和越权 ID。
- [ ] 详情 DTO 返回端点 URL、权重和 `has_api_key`，不返回 envelope、fingerprint 或旧 connection。
- [ ] 运行：

```bash
cd backend
go test ./bizpkg/config/modelmgr -run 'TestModelManagement' -count=1
```

预期：管理 service 测试通过。

## Task 6: 实现工作空间/用户授权与运行时可见性

**Files:**

- Create: `backend/bizpkg/config/modelmgr/grant.go`
- Create: `backend/bizpkg/config/modelmgr/grant_test.go`
- Modify: `backend/application/admin/management.go`
- Modify: `backend/domain/user/service/user.go`
- Modify: `backend/bizpkg/config/modelmgr/model_get.go`
- Modify: `backend/application/appdev/model.go`

- [ ] 先写测试覆盖 `all`、仅 workspace、仅 user、workspace+user 并集、停用、场景不匹配、未知 subject、跨租户直接 ID 调用。
- [ ] 使用现有 `ListAllSpaces`、`ListAllUsers`、`GetUserSpaceList` 和 `GetUserInfo` 校验 subject；保存授权前去重并验证所有对象存在。
- [ ] 新增：

```go
type AvailabilityContext struct {
    UserID int64
    SpaceID int64
    Scenario string
}

func (c *ModelConfig) ListAvailableModels(ctx context.Context, actor AvailabilityContext) ([]*Model, error)
func (c *ModelConfig) GetAvailableModelByID(ctx context.Context, id int64, actor AvailabilityContext) (*Model, error)
```

- [ ] `GetAvailableModelByID` 必须复用与列表相同的状态、场景和 grant predicate，禁止“列表隐藏但直接 ID 可调用”。
- [ ] AppDev `PageApp` 列表改为 `ListAvailableModels`，传入 `CurrentUserID` 与 `SpaceID`。
- [ ] 运行：

```bash
cd backend
go test ./bizpkg/config/modelmgr ./application/appdev -run 'TestModelGrant|TestListModelsAuthorization' -count=1
```

预期：授权和 AppDev 可见性测试通过。

## Task 7: 实现多端点路由、失败切换与连通性测试

**Files:**

- Create: `backend/bizpkg/config/modelmgr/routing.go`
- Create: `backend/bizpkg/config/modelmgr/routing_test.go`
- Modify: `backend/bizpkg/llm/modelbuilder/model_builder.go`
- Modify: `backend/bizpkg/llm/modelbuilder/builtin.go`
- Modify: `backend/infra/appdev/sse_broker.go`
- Modify: `backend/application/agentthread/model_executor.go`
- Modify: `backend/application/workbench/answer_runner.go`
- Modify: `backend/domain/agent/singleagent/internal/agentflow/agent_flow_builder.go`
- Modify: `backend/domain/workflow/internal/nodes/llm/llm.go`
- Modify: `backend/domain/workflow/internal/nodes/intentdetector/intent_detector.go`
- Modify: `backend/domain/workflow/internal/nodes/qa/question_answer.go`

- [ ] 先写 deterministic 测试覆盖 Round Robin、Weighted Round Robin、禁用端点、零权重、并发安全、可重试错误切换、不可重试错误停止、全部失败摘要。
- [ ] 路由器只返回 endpoint ID 和解密后的临时连接；不缓存明文密钥，不在错误中拼接 URL query、header 或 provider raw body。
- [ ] 增加显式 actor 参数的构建入口：

```go
func BuildModelForActor(
    ctx context.Context,
    modelID int64,
    actor modelmgr.AvailabilityContext,
    params *LLMParams,
) (ToolCallingChatModel, *modelmgr.Model, error)
```

- [ ] Agent、Workflow、Workbench 和 AppDev 能拿到 user/space/scenario 的调用链使用 `BuildModelForActor`；纯系统内置任务使用 `system` 场景但仍要求模型启用。
- [ ] `TestModelEndpoint` 构建临时 provider client，发送最小短请求，设置超时与取消，返回延迟、成功状态和安全错误码，不返回 completion 文本。
- [ ] 运行：

```bash
cd backend
go test ./bizpkg/config/modelmgr ./bizpkg/llm/modelbuilder ./application/agentthread ./application/workbench ./application/appdev -run 'TestModelRouting|TestBuildModelForActor|TestModelEndpoint' -count=1
```

预期：路由、访问控制和连通性测试通过。

## Task 8: 实现删除依赖检查与安全删除

**Files:**

- Create: `backend/bizpkg/config/modelmgr/dependency.go`
- Create: `backend/bizpkg/config/modelmgr/dependency_test.go`
- Modify: `backend/bizpkg/config/knowledge/knowledge.go`
- Modify: `backend/domain/agent/singleagent/internal/dal/single_agent_draft.go`
- Modify: `backend/domain/agent/singleagent/internal/dal/single_agent_version.go`
- Modify: `backend/bizpkg/config/modelmgr/model_save.go`

- [ ] 先写测试覆盖内置知识模型、Agent 草稿、Agent 版本、Workflow 节点、Workbench/AppDev 配置依赖，以及无依赖删除。
- [ ] 定义 bounded 摘要：

```go
type ModelDependencySummary struct {
    Type string
    Count int64
    Samples []ModelDependencySample
}
```

- [ ] 每类最多返回 5 个已脱敏 sample，总响应有固定字节上限；不暴露 prompt、workflow config、tool args 或 provider body。
- [ ] 有依赖返回 domain conflict，handler 投影为 HTTP `409`；无依赖时同一事务软删除 model、endpoint、grant。
- [ ] 运行：

```bash
cd backend
go test ./bizpkg/config/modelmgr -run 'TestModelDependency|TestDeleteModel' -count=1
```

预期：依赖保护测试通过。

## Task 9: 接入系统管理员 handler、错误码与审计边界

**Files:**

- Modify: `backend/api/handler/coze/config_service.go`
- Modify: `backend/api/handler/coze/admin_auth_service.go`
- Create: `backend/api/handler/coze/config_service_model_management_test.go`
- Modify: `backend/application/user/admin_auth.go`

- [ ] 先写 handler 测试覆盖未登录 `401`、普通用户 `403`、参数错误 `400`、引用冲突 `409`、密钥缺失 `503`、成功响应脱敏。
- [ ] 修复 `GetAdminAuthStatus` 当前硬编码 `true`，统一从会话用户、数据库配置和 `COZE_SYSTEM_ADMIN_EMAILS` 兼容引导事实判断。
- [ ] 所有模型管理 handler 在业务调用前执行相同 system-admin guard，不信任客户端的 `is_admin`、`user_id` 或 email。
- [ ] 将 modelmgr domain error 映射为稳定 HTTP 与前端可翻译 code；日志只记录 request ID、actor ID、model ID、endpoint ID 和错误分类。
- [ ] 运行：

```bash
cd backend
go test ./api/handler/coze ./application/user -run 'TestAdminModel|TestAdminAuthStatus' -count=1
```

预期：鉴权、错误映射和脱敏测试通过。

## Task 10: 建立前端生成 client 封装与纯 view model

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/system/service.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/model-management-view-model.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/model-management-service.test.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/model-management-view-model.test.ts`

- [ ] 先写测试覆盖 query 序列化、ID 字符串保持、错误映射、secret patch、provider label、能力 label、场景 label和表单 DTO 转换。
- [ ] `service.ts` 只调用生成 client；删除模型管理范围内的手写 fetch 和重复 DTO。
- [ ] view model 只包含纯函数：

```ts
export const toModelEditorValues = (detail: ModelDetail): ModelEditorValues => ({
  ...detail,
  endpoints: detail.endpoints.map(endpoint => ({
    ...endpoint,
    apiKey: '',
    hasApiKey: endpoint.has_api_key,
    clearApiKey: false,
  })),
});
```

- [ ] 明确提交规则：`apiKey === '' && hasApiKey` 不发送 key；`clearApiKey` 显式发送；浏览器内不保存已提交 secret。
- [ ] 运行：

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/model-management-service.test.ts src/pages/system/__tests__/model-management-view-model.test.ts
```

预期：service/view-model 测试通过。

## Task 11: 构建模型管理列表、筛选、分页与排序工作台

**Files:**

- Create: `frontend/apps/coze-studio/src/pages/system/model-management-section.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/model-management-table.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/model-management-section.module.less`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/model-management-section.test.tsx`

- [ ] 先写 UI 测试覆盖 loading、empty、error、refresh、组合筛选、重置、分页、打开新增/编辑/授权、启停确认、删除确认和排序失败回滚。
- [ ] 使用 `@coze-arch/coze-design` 的 Button、Input、Select、Table、Tag、Switch、Tooltip、Empty、Spin 和 Toast；图标只来自 `@coze-arch/coze-design/icons`。
- [ ] 页面结构与 Nuwax 能力顺序一致，同时继承当前系统管理视觉语言：页头、统计摘要、筛选条、主表、右侧主操作；桌面主表不使用卡片堆叠替代。
- [ ] 拖拽排序提供键盘可达替代：上移/下移操作和 aria label；失败时恢复原顺序并提示。
- [ ] 所有输入使用全局统一 focus ring，不额外叠加多层 outline。
- [ ] 运行：

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/model-management-section.test.tsx
```

预期：列表核心交互测试通过。

## Task 12: 构建新增/编辑模型与多端点测试弹窗

**Files:**

- Create: `frontend/apps/coze-studio/src/pages/system/model-editor-modal.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/model-endpoint-list.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/model-editor-modal.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/model-endpoint-list.test.tsx`

- [ ] 先写测试覆盖 provider 驱动字段、模型标识校验、能力多选、场景、协议、策略、Token 上下限、至少一个启用端点、权重、添加/删除/排序端点、保留/替换/清除密钥和连通性测试状态。
- [ ] 弹窗使用分区表单：基础信息、能力与限制、使用场景、路由策略、端点配置；footer 固定但不遮挡最后一行。
- [ ] endpoint 行显示 URL、masked key 状态、权重、启用、测试按钮与结果；测试中禁止重复提交，失败展示安全错误文案。
- [ ] 新增必须成功测试至少一个启用端点后才允许保存；编辑允许保存非连接字段，但修改 endpoint URL/key 后要求重新测试该 endpoint。
- [ ] 关闭有未保存修改的弹窗时二次确认；保存中禁止关闭和重复提交。
- [ ] 运行：

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/model-editor-modal.test.tsx src/pages/system/__tests__/model-endpoint-list.test.tsx
```

预期：表单和端点交互测试通过。

## Task 13: 构建工作空间/用户授权和删除依赖弹窗

**Files:**

- Create: `frontend/apps/coze-studio/src/pages/system/model-grant-modal.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/model-delete-dialog.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/model-grant-modal.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/model-delete-dialog.test.tsx`

- [ ] 先写测试覆盖全员/限制模式切换、工作空间 tab、用户 tab、远程搜索、已选回显、去重、取消不保存、保存失败保留状态。
- [ ] 授权弹窗沿用 Nuwax 双 tab 交互，但 tab 固定为 `工作空间` 与 `用户`；选项展示名称、类型/邮箱和 ID，禁止用前端伪造的 owner/role 判断授权。
- [ ] 删除确认先调用依赖预检；无依赖显示二次确认，有依赖显示分类、数量、有限样例和处理建议，主删除按钮禁用。
- [ ] 运行：

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/model-grant-modal.test.tsx src/pages/system/__tests__/model-delete-dialog.test.tsx
```

预期：授权与删除保护测试通过。

## Task 14: 接入系统页面状态编排并移除旧模型壳层

**Files:**

- Modify: `frontend/apps/coze-studio/src/pages/system/index.tsx`
- Delete: `frontend/apps/coze-studio/src/pages/system/model-config-section.tsx`
- Modify: `frontend/apps/coze-studio/src/pages/system/newx-system-ui.less`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/system-page.test.tsx`

- [ ] 先扩展 system page 测试，断言 `/system/models` 首次加载 providers+models，筛选只刷新列表，保存后刷新当前页，403 跳转/禁用，错误可重试。
- [ ] `index.tsx` 只负责页面级 query/mutation 状态，编辑器、授权、删除的局部状态留在对应组件；避免继续扩大单文件 orchestrator。
- [ ] 移除旧 `ModelConfigSection` 及其本地 DTO/表单逻辑，确保只有一个模型配置实现。
- [ ] 扩展系统页面 token 和通用样式，覆盖表格、筛选、弹窗、表单、focus、空态和窄屏；不要引入新的孤立色值体系。
- [ ] 运行：

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__/system-page.test.tsx src/pages/system/__tests__/model-management-section.test.tsx
npx tsc --noEmit --project tsconfig.json
```

预期：页面测试与 TypeScript 检查通过。

## Task 15: 运行后端回归、安全扫描与迁移验证

**Files:**

- Modify as failures require: only files listed in Tasks 1-14
- Test: all model management tests from Tasks 1-14

- [ ] 运行 targeted backend suite：

```bash
cd backend
go test ./bizpkg/config/modelmgr ./bizpkg/llm/modelbuilder ./application/admin ./application/appdev ./application/agentthread ./application/workbench ./api/handler/coze -count=1
```

- [ ] 运行涉及真实 builder 的 Mockey 测试时按仓库约定：

```bash
cd backend
go test -gcflags="all=-l -N" ./bizpkg/llm/modelbuilder ./application/agentthread -count=1
```

- [ ] 运行前端 suite：

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/system/__tests__
npx tsc --noEmit --project tsconfig.json
```

- [ ] 再运行 Atlas：

```bash
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

- [ ] 搜索敏感字段泄露：

```bash
rg -n 'api_key|apiKey|credential|envelope|fingerprint' backend/api/handler/coze frontend/apps/coze-studio/src/pages/system
```

预期：只有输入字段、`has_api_key`、安全 codec 和测试断言；响应 DTO、Toast、日志模板中无明文或 envelope。

## Task 16: 使用内置浏览器完成 Nuwax/Coze 对比验收

**Files:**

- Create: `docs/superpowers/qa/2026-07-22-system-model-management-alignment-acceptance.md`
- Modify: `docs/superpowers/plans/2026-07-22-system-model-management-alignment.md`

- [ ] 启动中间件、后端和前端，确认 `http://localhost:8888` 与 `http://localhost:8080` 可用。
- [ ] 使用 Codex in-app browser 登录 Coze 测试账号，进入 `/system/models`；另开 Nuwax `/system/model/manage`，使用相同桌面视口截图。
- [ ] 对比并验收：列表列项、筛选、重置、分页、排序、新增、编辑、端点测试、启停、授权、删除保护。
- [ ] 验收运行时：授权 workspace 可见并可调用；未授权 workspace 列表不可见且直接 ID 被拒绝；停用模型不可见且不可调用；AppDev `PageApp` 场景过滤正确。
- [ ] 验收安全：编辑详情无密钥；Network 响应无密钥；错误 Toast 无 provider raw body；普通用户访问 admin API 为 `403`。
- [ ] 验收窄屏：筛选换行、表格横向滚动、弹窗可滚动、footer 不遮挡、所有主操作可达。
- [ ] 在验收文档记录每个 URL、账号角色、空间、操作、结果、截图路径、控制台错误和未通过项；任何失败项修复后必须在相同状态重新验收。
- [ ] 只有全部完成标准通过后，将本计划所有 checkbox 勾选并声明生产级对齐完成。

## 实施顺序与并行边界

- Tasks 1-3 必须串行：合同、迁移与密钥安全是后续基础。
- Tasks 4-5 可在 Task 3 后由同一后端 worker 连续完成，避免同时修改 `model_save.go`。
- Task 6 与 Task 8 可并行，但不能同时修改 `model_get.go` 或 `model_save.go`；由主 worker 统一整合。
- Task 7 必须在 Task 6 的可用性合同稳定后执行。
- Tasks 10-13 可在 Task 1 生成 TypeScript client 后并行，组件文件互不重叠。
- Task 14 由主 worker 在所有前端组件完成后统一接线。
- Tasks 15-16 只能在实现全部完成后执行，不得用单元测试替代页面验收。

## 不允许的捷径

- 不把模型列表、provider、授权对象或测试结果写成前端 mock。
- 不用 localStorage 保存模型配置或 API Key。
- 不在旧 `connection` JSON 中继续明文保存新密钥。
- 不只在列表过滤授权而放过直接模型 ID 构建。
- 不用前端 `is_admin` 代替服务端 system-admin guard。
- 不在 handler 中堆积查询、事务、加解密或路由算法。
- 不为视觉对齐复制 Nuwax 的全局角色、用户组、计费或生态市场。
- 不在未完成 Atlas、targeted tests、TypeScript 和内置浏览器验收时声明完成。
