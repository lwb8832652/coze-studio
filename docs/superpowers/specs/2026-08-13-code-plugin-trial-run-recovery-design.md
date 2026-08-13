# 代码插件试运行恢复设计

状态：已确认  
日期：2026-08-13

## 背景

代码插件编辑页当前会连续遇到两个问题：

1. 后端在草稿不存在时返回一份 `revision=0` 的默认草稿，但不写数据库。前端把这份
   内容记录为已保存状态，直接点击“试运行”时不会先保存，调试接口因此返回 400。
2. 草稿保存后，调试请求已经进入 CodeRunner。dev 环境仍使用 `local-data` 部署
   profile，Sandbox 控制面关闭且没有 `plugin` 默认 Provider，因此执行失败并返回
   503。

`dev` 已包含 Native Sandbox Runner、Plugin adapter、用户与空间身份签名、容量控制和
`runner-2c4g` 部署 profile。本次只补齐草稿生命周期和 dev 接线，不重做执行架构。

## 目标

- 新建代码插件无需手工修改或先点保存，首次试运行也能正确创建草稿。
- dev 的代码插件通过 Native Sandbox Runner 真实执行。
- 保留用户、空间和单次执行隔离；Plugin 容器执行完成后立即销毁。
- Runner、Redis、密钥、Provider 或路由不可用时继续失败关闭，不回退宿主机执行。
- 相关配置缺失时给出可定位的提示，并在发布前尽早阻断错误部署。

## 非目标

- 不自动向数据库写入 Provider 或明文凭据。
- 不启用本机 host runtime 作为共享 dev 的兜底。
- 不改变现有 Provider、Scheduler、远程执行协议或数据库结构。
- 不在本次补齐 MCP stdio 和 AppDev adapter。

## 方案

### 默认草稿生命周期

`GetCodePluginDraft` 保持只读语义。`revision=0` 表示后端合成、尚未持久化的默认草稿。

前端发起试运行时，将以下任一情况视为需要保存：

- 编辑内容与已保存快照不同；
- 当前 revision 为 0。

前端先调用 `SaveCodePluginDraft`，使用 revision 0 的现有 CAS 合同创建草稿；保存成功后
使用服务端返回的新 revision 调用 `DebugCodePlugin`。保存失败、页面身份已切换或并发
请求失效时，不发送调试请求。普通保存、后续试运行和发布资格判断保持原样。

### dev Sandbox 接线

共享 dev 使用现有 `runner-2c4g` profile。NewX 后端只调用数据库登记的 HTTPS remote
Provider，Runner 只连接专用 rootless Docker/Podman-compatible socket。

首次启用按以下顺序完成：

1. 服务器准备权限为 `0600` 的 Runner 环境文件、TLS 文件和专用 rootless socket。
2. `app.env` 设置 `SANDBOX_CONTROL_PLANE_ENABLED=true`，先保持
   `SANDBOX_RUNTIME_ROUTING_ENABLED=false`。
3. 使用系统管理页创建 Native Runner remote Provider，只授予当前 Runtime 已支持的
   `agent` 和 `plugin` scope。
4. Provider 健康检查通过后，将它设为 `agent`、`plugin` 默认 Provider。
5. 设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=true` 并重启后端。
6. 执行一次代码插件真实试运行，再开放该能力验收。

部署脚本继续校验 Runner 镜像 revision、TLS/环境文件权限、rootless socket、容器健康和
回滚信息。本次补充 profile 与应用开关的一致性校验：选择 `runner-2c4g` 时控制面必须
启用，运行路由必须显式配置；选择其他 profile 时不得误启共享环境的 host runtime。

Provider 凭据和默认绑定属于数据库运行配置，只能经系统管理员接口写入并留下审计，
不能写入仓库、Compose 文件或启动时自动导入。

### 错误处理

- 默认草稿保存失败：页面显示“代码草稿保存失败”，不继续试运行。
- 调试接口返回不可用：页面提示“代码执行服务尚未配置或暂不可用，请联系系统管理员”，
  不把 Provider endpoint、凭据或内部错误返回给普通用户。
- Runner 健康、路由或默认 Provider 缺失：后端继续返回稳定的 unavailable 类型，系统
  管理员通过 `/system/sandbox` 查看具体状态。
- 超时、容量不足和运行错误继续使用已有状态映射，不统一改成 unavailable。

## 验证

### 自动化测试

- 前端 Vitest：revision 0 点击试运行时，先保存再使用新 revision 调试。
- 前端 Vitest：已有 revision 且无修改时不重复保存。
- 前端 Vitest：自动保存失败时不调用调试接口。
- dev 部署契约测试：`runner-2c4g` 与 Sandbox 开关不一致时预检失败。
- 现有 CodeRunner、Sandbox wiring 和部署脚本测试保持通过。

### 浏览器验收

使用本地前端连接 dev 后端，在账号 `840582614@qq.com`、空间
`7671359031507681280` 验证：

1. 打开新建代码插件，默认草稿正常显示。
2. 不手工保存，直接点击“试运行”。
3. 网络请求顺序为保存 200、调试 200，调试 revision 大于 0。
4. 页面显示真实执行结果，刷新后草稿和 debug-ready 状态仍然存在。
5. `/system/sandbox` 显示 Native Runner 健康，`plugin` 默认 Provider 已设置。
6. 页面控制台没有本次改动引入的未处理错误。

## 发布与回滚

代码按正常 dev 集成流程发布。服务器 Secret、TLS、rootless socket 和 Provider 写操作在
发布前单独核对，不随 Git 提交。

需要回滚时先设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=false` 并重启后端，保留控制面、
Provider、默认绑定和审计数据。不得改为宿主机执行。前端草稿修复可随应用版本正常回滚，
不需要数据库处理。

## 影响范围

本次会改变代码插件首次试运行的保存行为和 dev 部署前置条件。不会改变已有草稿的保存
合同、插件发布条件、其他 Provider 路由或数据库 schema。
