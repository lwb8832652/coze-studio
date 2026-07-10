# AppDev Production Hardening Design

## 1. Goal

把二期 AppDev 从本地单机可演示状态整改为可审计、可隔离、可恢复的生产实现。
整改不改变已验收的页面结构和主流程，优先消除数据损坏、越权、宿主执行、状态
丢失和资源泄漏风险。

## 2. Confirmed risks

- 取消 AI 任务后仍可能写入 `src/App.tsx`。
- 快速切换文件时，旧响应可能覆盖当前文件并导致错误保存。
- AppDev API 只检查空间成员身份，没有执行 `allow_develop` 和角色授权。
- Host Runtime 会执行用户项目依赖，且预览地址使用服务端 `127.0.0.1`。
- 聊天、运行状态和项目文件分别依赖进程内 Map 与本地缓存目录。
- 运行进程没有 keep-alive 过期回收、强制终止和归档联动。
- 原始 npm/Vite 输出会进入 API 和 UI。
- 导入、上传和导出缺少完整的文件数量与流式资源边界。

## 3. Architecture

### 3.1 Access policy

新增 workspace AppDev 授权结果：

- 所有操作必须是空间成员且空间 `allow_develop=true`。
- 查询、聊天、编辑和预览允许 Owner、Admin、Member。
- 项目归档、发布构建和运行环境启停仅允许 Owner、Admin。
- 个人空间 Owner 保持全部权限。
- handler 只消费服务端授权结果，不信任客户端角色或 owner 字段。

### 3.2 Concurrency and cancellation

- AI 生成完成后、写文件前必须再次检查 context 和 request ID。
- Cancel 只在生成协程确认停止后完成最终状态；取消消息写入历史。
- 前端文件、项目、模型和运行状态请求使用 generation token；只有最新 token
  可以更新状态。
- 保存文件时同时校验 `selectedPath` 与返回内容路径。

### 3.3 Runtime isolation

应用层依赖稳定的 `RuntimeManager` 接口，不直接依赖 Host Runtime。

- `HostRuntimeManager` 仅在 `APP_ENV=debug` 且
  `APP_DEV_HOST_RUNTIME_ENABLED=true` 时启用。
- 非 debug 环境没有隔离 Runner 时 fail closed，不执行 `npm install` 或 Vite。
- 生产 Runner 使用远程隔离执行服务，必须提供 CPU、内存、磁盘、进程、网络和
  生命周期限制。
- Runner 返回 Preview Gateway URL，浏览器永远不接收服务端 `127.0.0.1`。
- Preview iframe 使用 sandbox；生产 Preview Gateway 必须使用独立 origin 和 CSP。

### 3.4 Persistence

- 项目元数据、会话和运行记录进入数据库 repository。
- 项目源文件、快照和构建产物使用现有 `infra/storage.Storage`。
- 本地文件存储只作为 debug adapter，并在 API/类型上与生产 adapter 等价。
- 对象 key 固定包含 `space_id/project_id`，不向 UI 暴露原始 object URI。

### 3.5 Resource and output policy

- ZIP 最多 2,000 个 entry、目录深度最多 20、展开后最多 100MB。
- 批量上传最多 100 个文件、总大小最多 100MB、单文件最多 10MB。
- 导出和发布产物设置最大字节数并使用流式接口；超过限制明确失败。
- npm/Vite/Runner 输出通过统一 redactor，只返回 bounded summary。
- Runtime entry 在 keep-alive 超时、归档、停止和服务关闭时清理；SIGTERM 超时后
  使用 SIGKILL。

## 4. Delivery order

1. 修复取消、文件乱序、过期请求和测试缺口。
2. 增加 AppDev workspace 授权器并覆盖角色矩阵。
3. 增加进程回收、日志脱敏和资源限额。
4. 抽象 Runner，debug Host Runner 显式启用，生产 fail closed。
5. 接入 Remote Runner/Preview Gateway 与 iframe 安全策略。
6. 接入 DB/OSS repository，完成重启和多实例恢复。
7. 重新执行自动化、恶意输入和内置浏览器验收。

## 5. Acceptance

- 取消后项目文件摘要不变化。
- 乱序文件响应不能改变当前编辑文件。
- `allow_develop=false`、非成员和角色不足请求均被服务端拒绝。
- 非 debug 环境不会启动 Host Runtime。
- 浏览器预览 URL 不包含服务端 loopback 地址。
- 服务重启后项目、聊天历史和运行记录可恢复。
- 归档或超时后不存在对应运行进程和内存 entry。
- API/UI 不显示凭据、宿主路径、object URI 或原始 provider/runner 输出。
- 恶意 ZIP、超量上传、超大导出和恶意依赖项目均被拒绝。
