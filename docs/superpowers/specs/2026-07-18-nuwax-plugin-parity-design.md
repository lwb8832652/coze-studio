# NuWAX 插件能力对齐设计

日期：2026-07-18

状态：代码实现完成，数据库迁移与内置浏览器验收待完成

## 1. 目标与边界

本次对齐保留 Coze 原有插件入口、详情路由、页面结构、发布入口和资源引用链，
不建立第二套插件系统。代码插件使用现有 `FUNC + IDE` 类型，在原插件页面内
补齐创建、编辑、保存、试运行和生命周期能力。

最终功能边界：

- 代码插件为单源码模型，不支持多文件或源码包。
- 支持 Python 和 JavaScript 两种运行时。
- 支持输入 schema 和输出 schema，schema 与源码共同进入草稿 revision、
  bundle hash 和发布版本快照。
- 草稿保存使用 revision CAS；并发冲突不得静默覆盖。
- 试运行前保存当前草稿，由服务端校验输入 schema、执行代码并校验输出
  schema。
- 发布资格以服务端 `debug_ready` 和当前 revision 为准，前端不能自行推断。
- 发布、复制、删除和权限判断沿用 Coze 插件生命周期，并补齐代码草稿、版本
  快照和调试状态处理。
- 代码执行只允许经过 Sandbox `plugin` scope，不允许回退到宿主机。
- 不引入生态市场、插件分组、文件树、多文件编辑器或新的一级菜单。

## 2. 用户体验

### 2.1 创建

代码插件创建提交固定使用 `plugin_type=FUNC`、
`creation_method=IDE`，并提交用户选择的 Python 或 JavaScript 运行时。普通
HTTP 插件保持原有 `PLUGIN + COZE` 流程。

### 2.2 编辑

代码插件继续使用 Coze 原插件详情页。页面提供：

- 单源码编辑器和规范入口文件。
- Python/JavaScript 运行时选择。
- 输入结构、输出结构和试运行三个区域。
- 保存中、脏数据、冲突、加载失败和只读状态。
- 插件切换时的请求 identity 防护，避免旧请求污染新插件状态。

页面不显示生态市场、分组、文件树或多源码操作。

### 2.3 保存与试运行

- 保存请求携带当前 revision，服务端通过 CAS 创建下一 revision。
- 修改源码、运行时或 schema 后，旧的调试通过状态立即失效。
- 试运行仅接受 JSON object 输入；输入和输出均由服务端按当前 schema 校验。
- 运行结果只展示经过约束的状态、结果、原因、耗时、大小和 revision，不展示
  Provider 原始响应、凭据、内部 endpoint、宿主机路径或未受限日志。
- 试运行成功后重新读取服务端草稿；只有当前 revision 的
  `debug_ready=true` 才允许发布。

## 3. 数据与 API

代码草稿和发布版本持久化以下核心字段：

- `plugin_id`、`space_id`
- `runtime`、`entry_file`、单份源码
- `input_schema_json`、`output_schema_json`
- `revision`、`bundle_sha256`
- 当前 revision 的调试状态
- 创建/更新审计字段

空 schema 规范化为 `{"type":"object","properties":{}}`。单个 schema 最大
64KB；服务端进行 JSON object、结构和运行时输入/输出校验。发布版本保存源码、
运行时、入口文件和两份 schema 的不可变快照。

草稿读取、保存、试运行和版本读取通过生成的 Plugin Develop API 合同访问，
前端不使用平行的手写接口。

## 4. 生命周期与安全

- 创建：服务端校验类型、运行时、空间权限和代码插件无 HTTP/OAuth 配置。
- 保存：服务端校验插件访问权限、schema、源码限制和 revision CAS。
- 试运行：固定路由到 Sandbox `plugin/plugin/plugin/code/run`。
- 发布：服务端锁定并校验当前 revision、bundle hash 和
  `last_debugged_revision`，再创建代码版本快照。
- 复制：保留单源码、runtime 和输入/输出 schema，目标 revision 重置，调试
  状态清零，不复制凭据。
- 删除：沿用插件权限和资源生命周期，并清理代码草稿、版本和调试数据。
- 权限：前端仅消费服务端投影；写操作始终由服务端重新校验用户、空间和插件
  权限。
- Sandbox：代码插件只允许 remote Provider；错误 scope、workload、entrypoint
  或 purpose 均 fail closed。

## 5. 已完成实现

- `FUNC + IDE` 创建及 Python/JavaScript 运行时提交。
- 单源码草稿、输入/输出 schema、bundle hash、revision CAS 和版本快照。
- 原 Coze 插件详情页内的源码编辑、schema 编辑、保存和试运行。
- 服务端权威 `debug_ready`、发布资格和前端防串页处理。
- 发布、复制、删除、权限及 Sandbox plugin scope 的后端闭环。
- 公开错误信息脱敏；调试响应不暴露内部运行细节。
- 明确排除生态市场、分组、文件树和多文件能力。

## 6. 验证证据

已完成：

- 前端代码插件 workspace/schema 定向测试：50 个通过。
- 前端创建提交定向测试：2 个通过。
- `bot-plugin/entry` 与 `plugin-form-adapter` 两个包的 TypeScript 检查通过。
- 后端 `application/plugin`、`domain/plugin/repository`、
  `api/handler/coze` 相关定向测试通过。
- 后端 `domain/sandbox`、`application/sandbox`、`infra/sandbox`、
  `infra/coderunner/impl`、`infra/coderunner/impl/controlplane` 相关定向测试
  通过。
- `atlas migrate validate --dir file://docker/atlas/migrations` 通过。

已知验证口径：

- `api/handler/coze` 全包测试存在与本次插件改动无关的既有
  `TestValidateTree` panic；本次使用相关定向测试验证，不将该既有问题记录为
  本次失败。
- 数据库 migration 尚未 apply。
- 内置浏览器端到端验收仍待本地数据库和服务环境就绪后执行。

## 7. 完成定义

代码实现和静态/定向验证已完成。只有在 migration 完成 apply，并使用 Codex
内置浏览器验证创建、保存、CAS 冲突、schema、试运行、`debug_ready`、发布、
复制、删除、权限拒绝和 Sandbox fail-closed 后，才能将状态更新为“生产验收
完成”。
