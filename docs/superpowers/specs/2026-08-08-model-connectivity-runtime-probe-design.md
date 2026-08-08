# 模型连通性真实调用校验设计

## 背景

系统管理的“模型连通性测试”当前仅对 OpenAI Compatible 地址执行
`GET {base_url}/models`。该请求不使用 `model_identifier`，任意 2xx 都会被展示为
“连接成功”。共享 dev 已出现 `/models` 成功、真实 ChatModel 调用却返回
`404 Model not exist` 的假阳性。

## 目标与边界

- 对本次故障使用的 `openai-compatible` 协议，使用待保存表单中的地址、凭据和模型
  标识验证真实模型可调用性。
- 不修改正常模型执行链、模型持久化结构、路由策略或现有模型配置。
- 保留 12 秒总超时、禁止重定向、TLS 1.2 下限和响应体上限。
- 不回显 API Key、供应商原始响应体或其他凭据。
- 每次测试允许产生一次极小的模型调用与相应 Token 消耗。

## 方案比较

1. **只解析 `/models` 并精确匹配模型标识**：无 Token 成本，但部分兼容供应商不提供
   完整模型列表，仍不能证明 Chat Completions 可用。
2. **为 OpenAI Compatible 发送最小真实推理（采用）**：与当前故障运行时最接近，
   可准确发现鉴权、模型权限、模型标识和推理路由错误；代价是少量 Token 和略高延迟。
3. **直接复用运行时 ModelBuilder**：一致性最高，但 `modelbuilder` 已依赖
   `modelmgr`，直接复用会形成包循环并扩大本次修复范围。

## 设计

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

## 测试与验收

- 单元测试先复现旧逻辑假阳性：`/models` 可用但推理端点返回 404 时必须失败。
- 验证请求使用表单中的模型标识、正确路径、鉴权头、非流式与最小 Token。
- 覆盖新 API Key、已保存加密凭据、超时和主要状态码映射；验证非 OpenAI 协议仍走
  原有健康探测路径。
- 运行 `backend/bizpkg/config/modelmgr` 相关 Go 测试；Mockey 测试按仓库要求添加
  `-gcflags="all=-l -N"`。
- dev 页面验收以 `https://agent.newxai.cn/system/models` 为目标：原配置应显示真实
  404/模型不可用，而可真实推理的配置才显示连接成功。

## 风险与回滚

测试按钮会产生极少 Token 消耗并增加一次推理延迟。变更仅影响测试 API，可通过回滚
对应提交恢复，不影响已保存模型的正常执行。
