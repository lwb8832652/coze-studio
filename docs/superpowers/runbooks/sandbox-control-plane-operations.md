# Sandbox 控制面生产运维手册

本文覆盖 Sandbox Provider 控制面、运行时路由、凭据密钥、健康检查、审计和
监控的生产操作。所有操作都应先在预发布环境完成同口径验证。

## 安全边界

- Provider endpoint、认证凭据和密钥只能进入受控 Secret Manager 或写入型
  管理表单，不能写入仓库、工单、日志、监控标签或本文档。
- 服务端只信任认证上下文中的用户和空间信息；管理 API 必须继续执行系统管
  理员校验。
- 运行时路由不可用时必须 fail closed，不能回退到宿主机命令、未登记 endpoint
  或历史环境变量。
- 审计、Workbench 和前端只能展示经过裁剪的状态、原因码和关联号，不能展示
  凭据、原始 Provider 响应、执行参数或执行结果。

## 开关与职责

| 环境变量 | 默认/兼容行为 | 职责 |
| --- | --- | --- |
| `SANDBOX_CONTROL_PLANE_ENABLED` | 非 `true` 时关闭 | 启用 Provider 管理、持久化、健康检查和管理 API |
| `SANDBOX_RUNTIME_ROUTING_ENABLED` | 未设置时兼容既有行为；控制面开启后视为开启 | 独立控制 Agent、AppDev、MCP 和 CodeRunner 的 Provider 路由 |
| `SANDBOX_PROMETHEUS_METRICS_ENABLED` | `false` | 注册 Sandbox Prometheus 指标 |
| `APP_DEV_HOST_RUNTIME_ENABLED` | `false` | 仅用于 `APP_ENV=debug` 的本机 AppDev 调试双重开关 |

生产环境必须显式设置 `SANDBOX_RUNTIME_ROUTING_ENABLED`，不要依赖兼容默认值。
`APP_DEV_HOST_RUNTIME_ENABLED` 不得在生产或共享测试环境开启。

## 首次上线顺序

1. 应用并校验 Atlas 迁移，但暂不开放 Sandbox 前端入口。
2. 在 Secret Manager 中配置凭据 keyring、active key id 和 endpoint allowlist。
3. 设置 `SANDBOX_CONTROL_PLANE_ENABLED=true`、
   `SANDBOX_RUNTIME_ROUTING_ENABLED=false`，启动后端。
4. 由系统管理员在 `/system/sandbox` 创建 Provider，并按环境实际需要覆盖
   `agent`、`appdev`、`mcp` 三类 scope。
5. 分别执行健康检查；只有健康状态正常的 Provider 才能设为对应 scope 默认
   Provider。
6. 核对默认 Provider、审计记录、凭据重包状态和监控采集，不执行真实流量。
7. 显式设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=true`，滚动重启后端。
8. 依次执行 Agent、AppDev、MCP/CodeRunner 的最小真实任务，再开放前端入口。

如果任一 scope 没有可用默认 Provider，该 scope 应保持不可用，而不是使用其
他 scope、其他租户或宿主机作为兜底。

## Keyring 初始化与轮换

keyring 的每个值必须是标准 Base64 编码的 32 字节随机密钥，key id 必须稳定
且可审计。随机值应直接在 Secret Manager 内生成；不要在终端历史、CI 输出或
聊天中生成和传递。

轮换步骤：

1. 在 Secret Manager 中新增 key id 和 32 字节随机密钥，保留所有仍被密文引
   用的旧 key。
2. 将新 key id 设为 active key id，滚动重启后端。
3. 通过 `/system/sandbox` 的写入型表单逐个重新提交 Provider 凭据；读取接口
   不会也不应返回原凭据。
4. 确认所有 Provider 的 `needs_rewrap` 均为 `false`，健康检查正常，并核对
   更新审计记录。
5. 观察至少一个完整业务周期，确认
   `coze_sandbox_credential_decrypt_failures_total` 无新增。
6. 仅在数据库中已无旧 key id 引用后，从 keyring 删除旧 key，再滚动重启。

不要先删除旧 key。当前轮换采用管理员重新提交凭据完成重包，不存在可绕过写
入权限或返回明文凭据的批量重包接口。

## Provider 日常操作

### 新增或替换

1. 先新增 Provider，不覆盖当前默认项。
2. 完成 endpoint allowlist、凭据、scope 和容量配置。
3. 执行健康检查，确认状态和原因码符合预期。
4. 设为目标 scope 默认项。
5. 执行该 scope 的最小真实任务并观察指标。
6. 稳定后再禁用旧 Provider。

### 禁用或删除

1. 如果 Provider 是默认项，先为每个受影响 scope 切换到已通过健康检查的新
   默认项。
2. 禁用 Provider，确认没有新执行被路由到该项。
3. 等待已有执行自然完成，或通过业务取消流程终止；不要直接删除运行中依赖。
4. 核对运行审计和指标后再删除。

紧急情况下先设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=false` 并滚动重启。控制
面、Provider 配置和审计数据应继续保留，便于诊断和恢复。

## 监控与告警

启用 `SANDBOX_PROMETHEUS_METRICS_ENABLED=true` 后采集：

- `coze_sandbox_provider_selections_total`
- `coze_sandbox_provider_health_checks_total`
- `coze_sandbox_provider_health_check_duration_seconds`
- `coze_sandbox_executions_total`
- `coze_sandbox_execution_duration_seconds`
- `coze_sandbox_capacity_rejections_total`
- `coze_sandbox_credential_decrypt_failures_total`

标签仅包含 Provider 类型、scope、结果、原因码和状态，不包含 Provider 名称、
用户、空间、执行 ID、endpoint 或凭据。

建议至少配置：

- 凭据解密失败在任意五分钟窗口内新增即告警。
- 默认 Provider 健康检查连续失败即告警。
- 容量拒绝持续新增、执行失败率突增或延迟分位数越过既定 SLO 时告警。
- Provider 选择失败和无可用默认项按 scope 分组告警。

阈值应依据生产基线和 SLO 调整，不把本文建议直接当作固定容量结论。

## 审计查询

- 系统管理员在 `/system/sandbox` 查询配置变更和运行审计。
- 运行事件 action 为 `runtime.execute`，结果为 `success` 或 `failure`。
- 前端只显示关联号的首八位和末四位；持久化值为域隔离摘要，不保存调用方
  原始执行 ID。
- 根据时间、scope、Provider 和结果组合定位问题，不在日志中补打原始参数或
  Provider 响应。
- 缺少认证 actor 的后台执行不会伪造系统用户审计；此类缺口应通过调用链身份
  传播修复。

## 回滚与恢复

1. 设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=false` 并滚动重启。
2. 保持 `SANDBOX_CONTROL_PLANE_ENABLED=true`，保留 Provider、默认项、健康
   状态和审计记录。
3. 已在 Provider 上运行的任务按 Provider 合同完成或取消；不要迁移到宿主机
   或另一 Provider 继续执行。
4. 修复并验证健康检查、密钥、allowlist、容量和默认项。
5. 在预发布执行最小真实任务后，再重新开启运行时路由。

如果必须同时关闭控制面，应先导出合规的配置元数据和审计证据；不得导出明文
凭据。

## 发布验收

- Atlas hash 和 validate 通过。
- 三类 scope 的 Provider 健康检查和默认项符合部署清单。
- 管理员可以访问 `/system/sandbox`，普通用户收到服务端 `403`。
- Workbench Runtime Doctor 只展示裁剪后的 Provider 状态和原因码。
- Agent、AppDev、MCP/CodeRunner 最小真实任务分别通过。
- 超时、取消、容量拒绝和 Provider 不可用路径均 fail closed。
- Prometheus 指标不含高基数或敏感标签。
- 审计可按关联号追踪，页面和控制台无未处理错误。
- 页面验收使用 Codex 内置 in-app browser，并记录 URL、账号/空间、交互结果
  和控制台状态。
