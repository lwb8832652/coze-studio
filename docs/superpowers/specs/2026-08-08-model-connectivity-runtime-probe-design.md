# 模型连通性真实调用校验与旧管理入口退役设计

## 背景

系统管理的“模型连通性测试”当前仅对 OpenAI Compatible 地址执行
`GET {base_url}/models`。该请求不使用 `model_identifier`，任意 2xx 都会被展示为
“连接成功”。共享 dev 已出现 `/models` 成功、真实 ChatModel 调用却返回
`404 Model not exist` 的假阳性。

## 目标与边界

- 对本次故障使用的 `openai-compatible` 协议，使用待保存表单中的地址、凭据和模型
  标识验证真实模型可调用性。
- 不修改正常模型执行链、数据库表结构、路由策略或现有模型数据。
- 保留 12 秒总超时、禁止重定向、TLS 1.2 下限和响应体上限。
- 不回显 API Key、供应商原始响应体或其他凭据。
- 每次测试允许产生一次极小的模型调用与相应 Token 消耗。
- 以 `/system/models` 作为唯一模型管理入口前，先补齐旧管理页仍独有的运行时配置
  字段；不以删除入口的方式静默取消既有供应商能力。
- 只移除 `backend/conf/admin/index.html` 中模型管理专属的导航、页面、脚本和文案；
  保留该文件的基础配置、知识库配置以及它们仍依赖的后端接口。

## 旧页面兼容性审计

`backend/conf/admin/index.html` 的旧模型管理页和 `/system/models` 使用同一
`model_instance` 数据源，但不是完全等价的界面：

| 能力 | 旧管理页 | 新系统管理页 | 结论 |
| --- | --- | --- | --- |
| 供应商、名称、模型标识、URL、API Key、Base64、思考模式 | 支持 | 支持 | 已覆盖 |
| 列表、新增、删除 | 支持 | 支持 | 已覆盖 |
| 编辑、筛选、启停、排序、使用场景、授权、多 Endpoint | 不支持 | 支持 | 新页增强 |
| Ark Region | 支持 | 无输入和详情投影 | 未覆盖 |
| OpenAI Azure 与 API Version | 支持 | 无输入和详情投影 | 未覆盖 |
| Gemini Backend、Project、Location | 支持 | 无输入和详情投影 | 未覆盖 |

后三类字段会被对应运行时 builder 使用，不能视为纯展示字段。当前新页面更新模型时，
`buildSystemModelPayload` 会重新构造 `connection`；若不先补齐详情回显和保存合同，历史
高级配置还可能在编辑后被默认值覆盖。因此当前状态不能直接删除旧入口。

旧页的 `GET /api/admin/config/model/list` 还被同一 HTML 的知识库内置模型选择器使用。
退役模型页面时保留 `/list`、`/create`、`/delete` 等兼容后端路由；新系统页面也继续使用
`/delete`。本次不删除模型服务、数据表、IDL 旧路由或运行时 builder。

## 方案比较

1. **只解析 `/models` 并精确匹配模型标识**：无 Token 成本，但部分兼容供应商不提供
   完整模型列表，仍不能证明 Chat Completions 可用。
2. **为 OpenAI Compatible 发送最小真实推理（采用）**：与当前故障运行时最接近，
   可准确发现鉴权、模型权限、模型标识和推理路由错误；代价是少量 Token 和略高延迟。
3. **直接复用运行时 ModelBuilder**：一致性最高，但 `modelbuilder` 已依赖
   `modelmgr`，直接复用会形成包循环并扩大本次修复范围。

## 设计

### 1. 连通性测试

`TestSystemModelEndpoint` 在完成现有输入、地址和凭据校验后，对本次缺陷涉及的
`openai-compatible` 协议构造最小非流式推理请求：

- `POST {base_url}/chat/completions`；
- 请求发送表单中的指定 `model`、一条固定短用户消息、`max_tokens: 1` 和
  `stream: false`；
- `anthropic`、`gemini` 和 `ollama` 暂时保持既有健康探测行为，避免在本次局部修复中
  猜测不同 SDK 的实际请求合同或改变原有功能；后续应分别以运行时客户端为基准完善。

探测成功仅表示供应商对指定模型的最小推理返回 2xx，不校验回复文本内容。响应体最多
读取并丢弃 4 KiB。HTTP 状态映射为稳定、脱敏的错误：401 为凭据无效，403 为权限不足，
404 为模型或推理地址不可用，429 为限流，5xx 为供应商暂不可用，其余为供应商拒绝。

### 2. 补齐供应商专属配置

在模型管理合同中新增一个不含密钥的可选 `provider_options`，只承载旧页面已经支持且
会影响现有 builder 的字段：

- Ark：`region`；
- OpenAI：`by_azure`、`api_version`；
- Gemini：`backend`、`project`、`location`。

不直接向新详情接口暴露现有通用 `Connection`，避免把 Bedrock、header 或未来新增的
敏感字段一并扩大到前端合同。详情从历史 `connection` 投影上述白名单字段；新建和更新
时再将白名单字段写回对应 provider connection。编辑历史模型时，未修改的值必须原样
保留。前端按供应商条件显示字段，并对 Vertex 模式要求 Project 和 Location。

### 3. 退役旧模型管理页面

完成上述合同和页面回归后，从 `backend/conf/admin/index.html` 精确移除：

- `model-management` 左侧导航、页面映射、页面加载分支和工作台快捷卡片；
- 中英文模型管理专属 i18n 文案；
- `loadModelManagementPage`、模型渲染、新增弹窗、删除及其辅助函数。

保留知识库配置中的“内置模型选择”和 `initBuiltinModelSelect`，因为它们属于知识库配置，
只是读取模型列表。同步把 README 的旧 `/admin/#model-management` 指引改为
`/system/models`，避免继续引导到已退役入口。

## 测试与验收

- 单元测试先复现旧逻辑假阳性：`/models` 可用但推理端点返回 404 时必须失败。
- 验证请求使用表单中的模型标识、正确路径、鉴权头、非流式与最小 Token。
- 覆盖新 API Key、已保存加密凭据、超时和主要状态码映射；验证非 OpenAI 协议仍走
  原有健康探测路径。
- 运行 `backend/bizpkg/config/modelmgr` 相关 Go 测试；Mockey 测试按仓库要求添加
  `-gcflags="all=-l -N"`。
- 增加合同和前端测试，覆盖 Ark/OpenAI/Gemini 专属字段的新建、详情回显、更新保留，
  并确认 API Key 不出现在详情响应中。
- 增加旧 HTML 静态回归，确认不再包含 `model-management` 和模型管理函数，同时仍包含
  知识库内置模型选择及其 `/api/admin/config/model/list` 读取。
- dev 页面验收以 `https://agent.newxai.cn/system/models` 为目标：原配置应显示真实
  404/模型不可用，而可真实推理的配置才显示连接成功。

## 风险与回滚

测试按钮会产生极少 Token 消耗并增加一次推理延迟。供应商专属字段属于公共 IDL 和既有
`connection` JSON 投影变更，需要同步生成 Go/TypeScript 合同并做历史模型更新回归，
但不新增数据库迁移。旧页面代码只在
新页面完成兼容验证后删除；回滚对应提交可恢复旧入口，不迁移或删除现有模型数据。
