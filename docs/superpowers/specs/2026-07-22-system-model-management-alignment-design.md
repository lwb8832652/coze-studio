# 系统模型配置对齐设计

日期：2026-07-22

状态：已确认方案，待实施计划

## 1. 目标

在 Coze Studio 的 `/system/models` 中实现与 Nuwax `/system/model/manage`
等价的公共模型管理闭环。页面结构、核心交互和管理能力以 Nuwax 真实页面、
源码及接口合同为依据，不继续扩展当前“供应商分组 + 简单新增/删除”原型。

本次采用生产级等价对齐方案：模型授权沿用 Coze 已有工作空间和用户主体，
不额外建设 Nuwax 的全局角色、用户组和菜单权限系统。

## 2. 已核实的现状

### 2.1 Nuwax 参考能力

参考页面：`http://localhost/system/model/manage`

参考源码：

- `nuwax/src/pages/GlobalModelManage/index.tsx`
- `nuwax/src/pages/SpaceLibrary/CreateModel/index.tsx`
- `nuwax/src/pages/SystemManagement/Content/components/TargetAuthModal/index.tsx`
- `nuwax/src/services/modelConfig.ts`
- `nuwax/src/services/systemManage.ts`

已核实能力：

- 类型、状态、管控三类筛选，支持重置和查询。
- 模型列表支持拖拽排序、能力标签、启停状态、创建者和更新时间展示。
- 模型支持新增、编辑、连通性测试、授权和删除。
- 新增与编辑包含供应商、名称、模型标识、介绍、能力类型、思考模式、
  Token 限额、函数调用、启停、可用范围、接口协议、调用策略和多端点权重。
- 开启管控后可配置授权主体。

### 2.2 Coze 当前能力

当前页面：`http://localhost:8080/system/models`

当前源码：

- `frontend/apps/coze-studio/src/pages/system/model-config-section.tsx`
- `frontend/apps/coze-studio/src/pages/system/service.ts`
- `idl/admin/config.thrift`
- `backend/api/handler/coze/config_service.go`
- `backend/bizpkg/config/modelmgr/`

当前仅支持：

- 供应商分组列表和前端关键字搜索。
- 新增模型，保存前由后端执行一次连通性调用。
- 删除模型。
- API Key 在列表响应中做部分掩码。

当前缺口：

- 无详情、编辑、独立测试、启停、排序和授权 API。
- 无能力类型、介绍、创建者、更新时间、可用范围和管控状态合同。
- 当前管理接口无法表达多端点、权重和调用策略。
- `UpdateModelReq` 已在 IDL 中定义，但未暴露服务方法和 handler。
- `ModelExtra` 仅保存 `enable_base64_url`。
- `encryptConn` 和 `decryptConn` 仍为空实现，不满足生产密钥保护要求。
- Coze 没有 Nuwax 式全局角色和用户组系统。

## 3. 范围

### 3.1 本次包含

- 公共模型管理列表、筛选、排序和分页。
- 新增、详情、编辑、独立连通性测试、启停和删除保护。
- 能力类型、介绍、模型参数、可用范围和多端点调用配置。
- 受限模式下按工作空间和用户授权。
- 管理 API 的系统管理员强校验。
- 运行时按启停状态、使用场景和授权范围过滤可用模型。
- 密钥加密保存、只写更新、脱敏展示和日志清理。
- loading、empty、error、disabled、readonly、refresh 和冲突状态。
- 内置浏览器页面验收和前后端 targeted tests。

### 3.2 本次不包含

- Nuwax 模型定价页面。
- Nuwax 模型监控页面。
- 全局角色、用户组和菜单权限系统。
- 生态市场、计费、订阅和额度结算。
- 工作空间级自建模型管理页面。

## 4. 页面和交互设计

### 4.1 页面结构

`/system/models` 保持现有系统管理壳和左侧二级菜单，内容区改为 Nuwax 模型
配置工作台：

- 标题栏：`模型配置`，右侧主按钮 `添加模型`。
- 查询栏：类型、状态、管控三个筛选器，`重置` 和 `查询` 按钮。
- 数据表：排序、模型名称、类型、模型标识、模型介绍、状态、创建者、
  更新时间、管控和操作。
- 操作项：编辑、授权、删除。授权仅在受限模式开启时显示。
- 表格使用固定表头和横向滚动，不把表格退化为供应商卡片。

### 4.2 新增和编辑弹窗

新增与编辑复用一个表单组件，字段顺序与 Nuwax 保持一致：

- 供应商。
- 模型名称。
- 模型标识。
- 模型介绍。
- 模型能力类型，多选。
- 思考模式。
- 向量维度，仅向量模型显示。
- 最大输出 Token 数。
- 最大上下文长度。
- 函数调用支持程度。
- 启用状态。
- 可用范围。
- 接口协议。
- 调用策略。
- API 端点列表，每项包含 URL、API Key 和权重。

弹窗底部提供 `模型连通性测试`、`取消` 和 `确认`。测试与保存使用同一份
表单校验合同。保存不依赖前端测试结果，服务端仍必须重新校验。

### 4.3 密钥编辑体验

- 列表和详情只返回 `has_api_key`，不返回掩码后的密钥正文。
- 编辑弹窗显示“密钥已配置”，输入框保持空值。
- 留空表示保留旧密钥，输入新值表示替换。
- 删除某个端点需要明确确认；不允许通过空字符串意外清除密钥。
- 前端错误信息不得拼接请求体、响应体或密钥。

### 4.4 授权弹窗

授权弹窗提供两个页签：

- 工作空间：搜索并勾选允许使用该模型的工作空间。
- 用户：搜索并勾选允许使用该模型的用户。

模型管控模式分为：

- 全部可用：所有符合使用场景的工作空间和用户均可使用。
- 受限：当前工作空间命中授权，或当前用户命中授权时可用。

切换为受限模式但授权集合为空时，服务端拒绝保存，防止误操作导致模型全部
不可用。

### 4.5 删除保护

- 删除前展示依赖检查结果。
- 正被系统默认模型、知识库配置或其它已知配置引用时返回 `409`，并展示引用
  摘要。
- 删除使用软删除，不物理清除配置和审计信息。
- 页面删除确认框不得使用浏览器原生 `window.confirm`。

## 5. 数据合同

### 5.1 模型管理投影

管理端模型 DTO 需要包含：

- `id`
- `provider`
- `name`
- `model`
- `description`
- `capability_types`
- `thinking_type`
- `model_type`
- `output_tokens`
- `max_tokens`
- `function_call_mode`
- `enabled`
- `usage_scenarios`
- `protocol`
- `strategy`
- `endpoints`
- `access_mode`
- `sort_order`
- `creator_user_id`
- `creator_name`
- `created_at`
- `updated_at`
- `has_api_key`
- `dependency_summary`

端点 DTO 只返回 URL、权重和 `has_api_key`，不返回 API Key。

### 5.2 持久化

复用 `model_instance` 已有字段保存原生模型信息：

- `display_info` 保存名称、介绍和 Token 限额。
- `capability` 保存模型能力。
- `parameters` 保存运行参数。
- `connection` 保存运行时基础连接信息和加密后的主端点凭据。

扩展 `ModelExtra` 保存管理元数据：

- `enabled`
- `usage_scenarios`
- `protocol`
- `strategy`
- `endpoints`
- `access_mode`
- `sort_order`
- `creator_user_id`
- `function_call_mode`
- `enable_base64_url`

新增 `model_instance_grant` 表保存授权关系：

- `id`
- `model_id`
- `subject_type`，仅允许 `space` 或 `user`
- `subject_id`
- `created_by`
- `created_at`
- `updated_at`

唯一索引为 `(model_id, subject_type, subject_id)`。查询模型时必须先限定模型
状态，再使用当前认证上下文中的用户和空间做授权过滤。

### 5.3 兼容默认值

历史模型迁移时采用：

- `enabled=true`
- `access_mode=all`
- `sort_order` 按创建时间和 ID 稳定生成
- `usage_scenarios` 为全部现有运行场景
- 单个历史连接投影为一个权重为 `1` 的端点

这保证升级后现有模型仍可用，不发生静默停用。

### 5.4 供应商目录和能力边界

- 供应商目录由现有 `ModelMetaConf` 和服务端受控元数据生成，不由前端硬编码。
- 目录返回供应商名称、图标、支持协议、默认 URL、已知模型和模型能力。
- 已知模型选择后自动回填能力、Token 限额、协议和 URL，管理员仍可调整允许
  编辑的字段。
- OpenAI 兼容供应商允许录入目录外模型，但必须显式填写能力和 Token 限额，
  服务端据此生成受控模型元数据，不能继续因 `ModelMetaConf` 未命中而直接失败。
- 只有已经存在运行时适配器的能力可以启用。尚未接入运行时的能力可以展示为
  不可用选项，但不能保存成看似可用的模型配置。

## 6. API 设计

所有接口继续放在 `/api/admin/config/model/*`，并使用服务端系统管理员校验。

- `GET /list`：分页、筛选和排序后的管理投影。
- `GET /providers`：供应商、协议、默认端点和已知模型目录。
- `GET /detail`：返回单个模型详情，不返回密钥正文。
- `POST /create`：创建并执行连通性校验。
- `POST /update`：按 revision 更新，支持保留原密钥。
- `POST /test`：对未保存或已保存配置执行独立连通性测试。
- `POST /status`：启用或停用模型。
- `POST /sort`：批量更新排序，事务提交。
- `GET /grants`：查询工作空间和用户授权。
- `POST /grants/save`：原子替换授权集合。
- `POST /delete`：执行依赖检查后软删除。

更新、状态、排序和授权写操作携带 `revision` 或等价版本字段，冲突返回
`409`，前端提示刷新后重试。

## 7. 多端点和运行时

- 管理配置允许一个或多个端点。
- `RoundRobin` 和 `WeightedRoundRobin` 为首期必须实现的策略。
- 调度游标使用共享存储或无状态加权算法，不能依赖单进程内存状态。
- 运行时解析模型前，从已启用端点中选择一个，并投影为现有
  `BaseConnectionInfo`，保持现有模型 builder 的调用边界。
- 端点失败只在明确可重试错误下切换，鉴权失败不得无限重试其它端点。
- 连通性测试逐端点返回成功或已审核的错误摘要，不返回原始供应商响应。
- 模型选择器、Agent Runtime、AppDev 和知识库模型查询都必须共享同一可用性
  过滤服务，避免各入口规则不一致。

## 8. 安全设计

- 新增 `MODEL_CREDENTIAL_KEYS_JSON` 和
  `MODEL_CREDENTIAL_ACTIVE_KEY_ID` 作为模型凭据 keyring。
- API Key 写入前使用带 key id 的 envelope encryption，加密失败时 fail closed。
- 禁止在 debug 日志中打印创建、更新、测试请求和连接对象。
- 管理接口只接受服务端认证用户，不信任客户端提交的管理员、用户或空间身份。
- 授权保存前校验目标工作空间和用户真实存在。
- Workbench 和运行时投影不得暴露管理元数据、凭据、授权列表或 Provider 原始
  错误体。

## 9. 错误处理

- `400`：字段校验失败、协议与供应商不兼容、端点配置非法。
- `401`：未登录。
- `403`：非系统管理员访问管理接口。
- `404`：模型、工作空间或用户不存在。
- `409`：revision 冲突、删除存在引用、受限模式授权为空。
- `422`：模型连通性测试失败。
- `500`：未分类服务端错误，前端只展示安全错误摘要。

列表加载失败保留页面壳并提供重试；写操作失败保留用户表单内容；排序失败回滚
本地顺序；授权保存失败保留勾选状态。

## 10. 前端组件边界

- `ModelManagementSection`：页面状态和查询编排。
- `ModelManagementTable`：表格、筛选、排序和操作入口。
- `ModelEditorModal`：新增、编辑和连通性测试。
- `ModelEndpointList`：端点、权重和密钥只写交互。
- `ModelGrantModal`：工作空间和用户授权。
- `model-management-view-model.ts`：管理 DTO 到展示模型的纯函数。
- `service.ts`：只封装生成或明确记录的管理 API，不保存 UI 状态。

避免继续扩张当前 400 行的 `model-config-section.tsx`。

## 11. 验证方案

### 11.1 后端

- 非管理员访问所有管理接口均为 `403`。
- 列表、详情和错误日志不包含 API Key。
- 新增、更新和测试覆盖成功、鉴权失败、超时和协议错误。
- 留空密钥更新能够保留旧密钥。
- 启停和授权立即影响运行时模型可见性。
- 排序事务失败不产生部分更新。
- 删除引用中的模型返回 `409`，无引用模型软删除成功。
- 历史模型默认值迁移后仍可用。

### 11.2 前端

- 筛选、重置、查询、分页和空态。
- 新增与编辑字段联动及校验。
- 连通性测试 loading、成功和失败状态。
- 密钥已配置、保留和替换状态。
- 拖拽排序成功及回滚。
- 授权搜索、全选、保存和冲突提示。
- 删除保护对话框和引用摘要。
- 键盘焦点、弹窗焦点锁定、ARIA、disabled 和错误提示。

### 11.3 页面验收

默认使用 Codex 内置浏览器：

- Nuwax：`http://localhost/system/model/manage`
- Coze：`http://localhost:8080/system/models`

使用相同视口逐项对比列表、筛选、新增、编辑、测试、授权和删除保护。验收记录
必须包含 URL、账号、关键可见状态、核心交互结果和控制台错误。

## 12. 完成标准

- Coze 页面信息架构和核心交互与 Nuwax 模型配置页一致。
- 本文范围内所有按钮均连接真实后端，不存在 UI stub。
- 页面刷新、服务重启后模型配置、排序和授权仍然存在。
- 运行时真实遵守启停、使用场景和授权规则。
- API Key 不回显、不明文落库、不进入日志。
- targeted tests、IDL 校验、Atlas 校验和内置浏览器验收全部通过。
