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
- Sandbox remote Provider 和 Native Runner 均正式支持 HTTP 与 HTTPS，可通过服务器
  公网 IP 跨服务调度；协议由每条 Provider endpoint 独立选择，不按环境强制。
- 保留用户、空间和单次执行隔离；Plugin 容器执行完成后立即销毁。
- Runner、Redis、密钥、Provider 或路由不可用时继续失败关闭，不回退宿主机执行。
- 相关配置缺失时给出可定位的提示，并在发布前尽早阻断错误部署。

## 非目标

- 不自动向数据库写入 Provider 或明文凭据。
- 不启用本机 host runtime 作为共享 dev 的兜底。
- 不改变现有 Provider 类型、Scheduler、远程执行 JSON 协议或数据库结构。
- 不在本次补齐 MCP stdio 和 AppDev adapter。
- 不在本次接入 `agent-infra/sandbox` AIO Runtime 或其 Go SDK；该能力作为后续独立
  Runtime Profile 设计和验收。

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

共享 dev 使用现有 `runner-2c4g` profile。NewX 后端只调用数据库登记的 HTTP/HTTPS
remote Provider，Runner 只连接专用 rootless Docker/Podman-compatible socket。

Sandbox remote Provider endpoint 接受根 HTTP/HTTPS origin，例如
`http://<runner-ip>:9443` 或 `https://sandbox.example.com`。每条 Provider 保留自己的
scheme；生产与共享环境可以同时登记内部 HTTP Runner 和外部 HTTPS Provider，不新增
环境级协议开关。endpoint 继续禁止 userinfo、query、fragment、非根 path、loopback、
link-local、metadata、multicast 和 unspecified 地址。文档中的 `<runner-ip>` 是占位符，
部署时必须替换为实际可路由且符合地址策略的 IP。

HTTP 放行仅属于 Sandbox remote Provider。共享 `safehttp` 的其他调用方继续保持原协议
规则；Sandbox 出站仍锁定数据库 endpoint 的精确 scheme、host 和 port，禁止跨 origin
重定向，并在 DNS 解析和实际拨号时继续拒绝未授权私网及特殊地址，防止 DNS rebinding。
HTTP 不提供链路保密性，Runner Bearer token 和请求载荷可能被能观察链路的一方读取；
因此使用公网 IP 时，服务器安全组或主机防火墙必须只允许 NewX 后端的固定出口地址访问
Runner 端口。Bearer token、用户/空间执行身份签名、请求摘要、超时和响应大小限制继续
保留，但不把它们描述为 TLS 的替代品。

Runner 增加显式 `SANDBOX_RUNNER_TRANSPORT=http|https`：

- `http` 使用现有 Go HTTP server 监听，不要求或挂载 TLS 证书；
- `https` 继续要求权限合规的证书和私钥，并保持 TLS 1.2 以上；
- 共享与生产部署必须显式配置 transport；非法值、HTTP 携带 TLS 文件、HTTPS 缺少
  TLS 文件均失败关闭；
- Compose 通过 `SANDBOX_RUNNER_BIND_IP` 和 `SANDBOX_RUNNER_PUBLISH_PORT` 发布宿主机
  端口，默认只绑定 `127.0.0.1`；跨服务器调度时显式绑定目标服务器地址，并由防火墙
  限制来源，不直接暴露 rootless runtime socket。

首次启用按以下顺序完成：

1. 服务器准备权限为 `0600` 的 Runner 环境文件和专用 rootless socket；选择 HTTPS 时
   另外准备权限合规的 TLS 文件。
2. `app.env` 设置 `SANDBOX_CONTROL_PLANE_ENABLED=true`，先保持
   `SANDBOX_RUNTIME_ROUTING_ENABLED=false`。
3. 配置 Runner transport、绑定地址、发布端口和安全组来源规则，启动后先验证健康状态。
4. 使用系统管理页创建 Native Runner remote Provider，只授予当前 Runtime 已支持的
   `agent` 和 `plugin` scope。
5. Provider 健康检查通过后，将它设为 `agent`、`plugin` 默认 Provider。
6. 设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=true` 并重启后端。
7. 执行一次代码插件真实试运行，再开放该能力验收。

部署脚本继续校验 Runner 镜像 revision、TLS/环境文件权限、rootless socket、容器健康和
回滚信息。本次补充 profile 与应用开关的一致性校验：选择 `runner-2c4g` 时控制面必须
启用，运行路由和 Runner transport 必须显式配置；HTTPS 条件校验 TLS 文件，HTTP 不再
要求 TLS 文件；发布地址和端口必须可解析且不能使用宽泛的未确认默认值。选择其他 profile
时不得误启共享环境的 host runtime。

Provider 凭据和默认绑定属于数据库运行配置，只能经系统管理员接口写入并留下审计，
不能写入仓库、Compose 文件或启动时自动导入。

### 错误处理

- 默认草稿保存失败：页面显示“代码草稿保存失败”，不继续试运行。
- 调试接口返回不可用：页面提示“代码执行服务尚未配置或暂不可用，请联系系统管理员”，
  不把 Provider endpoint、凭据或内部错误返回给普通用户。
- 系统管理页允许保存 HTTP endpoint，并显示“未加密传输”状态提示；该提示不阻断健康
  检查、设为默认项或生产使用。
- Runner 健康、路由或默认 Provider 缺失：后端继续返回稳定的 unavailable 类型，系统
  管理员通过 `/system/sandbox` 查看具体状态。
- 超时、容量不足和运行错误继续使用已有状态映射，不统一改成 unavailable。

## 验证

### 自动化测试

- 前端 Vitest：revision 0 点击试运行时，先保存再使用新 revision 调试。
- 前端 Vitest：已有 revision 且无修改时不重复保存。
- 前端 Vitest：自动保存失败时不调用调试接口。
- dev 部署契约测试：`runner-2c4g` 与 Sandbox 开关不一致时预检失败。
- 后端 Provider 测试：HTTP/HTTPS 根 origin 均可创建、健康检查和执行；scheme、host、
  port、重定向或解析地址变化时继续失败关闭。
- 后端安全 HTTP 回归：只有 Sandbox remote Provider 可使用 HTTP；其他调用方、loopback、
  metadata、特殊地址、DNS rebinding 和未授权私网仍被拒绝。
- Runner 配置测试：HTTP/HTTPS、TLS 文件、listener 和 transport 的有效/无效组合覆盖。
- dev Compose/部署契约测试：发布端口、transport、条件 TLS 文件和健康检查 scheme 一致。
- 前端 Vitest：HTTP endpoint 可提交并显示未加密传输提示，HTTPS 行为保持原样。
- 现有 CodeRunner、Sandbox wiring 和部署脚本测试保持通过。

### 浏览器验收

使用本地前端连接 dev 后端，在账号 `840582614@qq.com`、空间
`7671359031507681280` 验证：

1. 打开新建代码插件，默认草稿正常显示。
2. 不手工保存，直接点击“试运行”。
3. 网络请求顺序为保存 200、调试 200，调试 revision 大于 0。
4. 页面显示真实执行结果，刷新后草稿和 debug-ready 状态仍然存在。
5. `/system/sandbox` 显示 Native Runner 健康，`plugin` 默认 Provider 已设置。
6. 分别登记一个 HTTP Runner 和一个 HTTPS 测试 Provider，确认健康检查使用各自 scheme；
   HTTP Provider 可完成真实插件试运行。
7. 页面控制台没有本次改动引入的未处理错误。

## 发布与回滚

代码按正常 dev 集成流程发布。服务器 Secret、rootless socket、防火墙规则、可选 TLS 和
Provider 写操作在发布前单独核对，不随 Git 提交。

需要回滚时先设置 `SANDBOX_RUNTIME_ROUTING_ENABLED=false` 并重启后端，保留控制面、
Provider、默认绑定和审计数据。不得改为宿主机执行。前端草稿修复可随应用版本正常回滚，
不需要数据库处理。

## 影响范围

本次会改变代码插件首次试运行的保存行为、Sandbox remote Provider 的 endpoint 协议
校验、Native Runner 的 HTTP/HTTPS 监听和 dev 部署前置条件。不会改变已有草稿的保存
合同、插件发布条件、其他安全 HTTP 调用方、远程执行 JSON 协议或数据库 schema。
