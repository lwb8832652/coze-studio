# 本地代码沙箱统一运行模式设计

状态：已确认，进入实施
日期：2026-08-14

## 背景

dev 本地环境的工作流 Code 节点和代码插件试运行都通过 `coderunner.Runner`
执行用户代码，但当前行为并不一致：

- 工作流 Code 节点发送 `PurposeAgent`，旧本地 Runner 允许该 purpose；
- 代码插件发送 `PurposePlugin`，旧本地 Runner 明确返回 unavailable；
- Plugin application 比 Workflow 更早初始化，并在 CodeRunner 全局值写入前保存了
  `nil`，即使后续存在可用 Runner，Plugin 仍固定返回 unavailable；
- 新建插件的默认草稿是未持久化的 `revision=0`，前端会在未保存时直接调试，先收到
  400，再把所有异常统一显示为“服务不可用”。

现有 Native Runner/remote Provider 继续提供容器级隔离链。本设计补齐操作者明确选择的
本地代码沙箱模式，不把 Host Shell、宿主 Python 直跑或 Remote Provider 故障回退包装成
本地沙箱。应用代码不根据 `APP_ENV` 限制该模式；运行环境通过显式后端选择决定是否启用，
并始终把它标记为低于 Native Runner 的 `local_wasm_best_effort` 执行可信度。

## 目标

- 选择本地沙箱后，Agent/工作流 Code、Skill Script 与代码插件使用同一个 Runner 实例、
  同一隔离策略、超时、内存和输出上限。
- 本地沙箱能力不依赖 `APP_ENV`；它由独立、显式的运行后端配置启用。
- `PurposeAgent`、`PurposeSkill` 与 `PurposePlugin` 继续作为入口分类，但不再导致本地
  Plugin 被单独拒绝；新的本地 Runner 不接受空 purpose。
- 控制面 Provider 路由与本地沙箱互斥，配置不确定时失败关闭，不静默切换执行后端。
- 新建代码插件第一次点击“试运行”时自动创建草稿，并使用服务端返回的新 revision
  执行。
- 在当前 dev 页面完成工作流 Python Code 和代码插件 Python 的真实执行验收。

## 非目标

- 不把 Plugin 请求改写为 `PurposeAgent`。
- 不允许 Host Shell、legacy direct Runner 或任意宿主 Python 作为失败回退。
- 不让 `local_debug` Provider 获得 `plugin` scope；本地代码沙箱是进程内明确选择的
  CodeRunner 后端，不是假冒 Provider。
- 不改变 Native Runner、签名身份、Provider、容量和容器销毁合同。
- 不在本次为本地沙箱增加 JavaScript；本地 Agent/工作流当前只支持 Python，Plugin
  本地模式保持同一能力并对 JavaScript 返回明确的 unsupported 错误。
- 不新增数据库表或 migration。
- 不把本地试运行成功当作正式发布资格；代码插件发布前仍须通过现有 control-plane
  remote one-shot 信任边界的试运行。

## 运行模式

新增独立精确开关 `SANDBOX_LOCAL_CODE_RUNNER_ENABLED=true`。它不读取或推断
`APP_ENV`。未设置、空值、`false` 均保持现有行为；除精确小写 `true` 外的值启动失败。

运行后端选择规则如下：

1. 本地开关为 `true` 时，原始 `SANDBOX_RUNTIME_ROUTING_ENABLED` 必须精确为
   `false`；空值也拒绝，因为控制面启用时空值兼容语义是 `true`；
2. 本地模式走新的单一构造入口，并在现有 control-plane 早退与 legacy/direct 选择前
   完成；它绝不调用 `newLegacyRunner`、`direct.NewRunner`、Router 或 Host Shell；
3. 本地开关为 `false` 时保持现有选择规则：控制面 routing 可选择 Router-backed
   Runner，控制面关闭时既有 legacy 配置继续兼容；
4. 本地与 control-plane routing 同时启用属于歧义配置，后端启动失败；
5. 任一后端运行中失败都只返回该后端的稳定错误，不尝试另一后端。

`SANDBOX_CONTROL_PLANE_ENABLED=true` 可以继续用于管理 Provider；只要 runtime routing
保持关闭，它不阻止显式本地代码沙箱。这让当前本地管理页面与代码试运行可以同时验收，
又不会把管理面启用误解为远程流量已切换。

本地模式要求 runtime routing 关闭，因此 Host Shell Session、Remote Provider、MCP runtime
binding 和其它依赖 `SandboxRouter` 的执行 API 在该进程中不可用。即使 Host Shell 三重门禁
已打开，也不能绕过该互斥规则。本地代码沙箱与这些能力之间不存在回退关系。

## 本地沙箱合同

本地后端使用新的 Go -> Deno/Pyodide 直接执行器，不复用现有 Python launcher，也不复用
`CodeRunnerType_Local` 的宿主 Python direct Runner。这样可避免 Python 父进程、未受控 pipe、
完整宿主环境继承和 Deno 孙进程残留问题。

运行资产包括仓库内固定的 TypeScript launcher、`deno.lock` 和精确
`jsr:@langchain/pyodide-sandbox@0.0.4` 入口。准备命令只在显式安装阶段联网，将锁定依赖写入
当前用户私有、权限为 `0700`、按 lock digest 隔离的 `DENO_DIR`。应用启动和业务执行始终使用
`--cached-only`、`--frozen`、`--lock=<受管 deno.lock 的规范绝对路径>`、`--no-prompt`、
`--no-config` 和 `--node-modules-dir=none`；缓存缺失、lock 不一致、launcher 不匹配或 Deno major version
不受支持时启动失败，运行时不联网修复。空工作目录中不允许依赖自动发现 lock 或配置文件。

该上游包已经归档，维护方也不建议用于生产。NewX 因此把这个后端明确投影为
`local_wasm_best_effort`，不把它描述为 Native Runner 或对抗性多租户隔离。是否在某个环境显式
选择该后端由部署配置决定；应用不按 `APP_ENV` 硬编码白名单。

启动阶段把 Deno 解析为规范绝对路径，并使用最小 allowlist 环境启动它。用户源码经 stdin
传入，不出现在 argv、进程标题或日志；工作目录是每次执行创建的私有空目录，结束后精确清理。

默认策略固定为：

- Python only；
- 输入与输出 wire 各最多 `1 MiB`，stderr 最多读取 `64 KiB` 且不回传原文；
- 同时最多运行 2 个请求，额外请求立即返回 capacity exhausted；
- 每次最多 60 秒，Deno V8 heap 目标为 `128 MiB`；该值不是 OS/cgroup 硬内存隔离，
  本地模式必须在状态与文档中标为 `local_wasm_best_effort`，不能宣称容器级隔离；
- 不传递宿主环境变量，只传私有 `DENO_DIR`、禁用 prompt/update 所需的固定值；
- Deno 不授予 `read/write/net/env/run/sys/ffi` permission，用户代码不能读写宿主文件、
  联网、读取环境或启动命令；模块解析只允许锁定且已缓存的入口；
- Go 同时流式写 stdin、读取 stdout/stderr；任一上限越界立即终止执行，不先在内存中
  收集无界结果；
- 上下文取消或超时必须终止完整进程组，并等待回收后才返回；
- stdout 只接受一个 JSON object，拒绝 trailing bytes；解析失败、超限或非零退出使用
  稳定分类错误；
- 错误、日志和响应不得暴露源码、argv、宿主路径、环境值、stdout 或 stderr 原文。

本地 Runner 仍接收原始 purpose。允许集合是 `PurposeAgent`、`PurposeSkill` 与
`PurposePlugin`；空值和其它未知 purpose 返回 invalid request。`ScriptExecutor` 必须显式发送
`PurposeSkill`，旧内部 delegate 必须显式发送 `PurposeAgent`，不再靠空值推断身份。
`PurposeSkill` 在 control-plane Runner 中继续映射到现有 `agent` scope 与入口，避免改变远程
Skill 语义；legacy purpose guard 同步把 `PurposeSkill` 作为 Agent-compatible 入口放行，保证
关闭新模式后现有 Skill 行为不被意外切断。Purpose 仅用于入口分类；本设计不声称本地模式
具有 Native Runner 的签名身份或持久审计。三种 purpose 的执行隔离和资源限制完全相同。

## 执行可信度与发布资格

`coderunner.RunResponse` 新增必填的内部 `Assurance`，取值至少包括：

- `control_plane_remote`：经现有 control-plane Router 选中的 remote one-shot Provider 执行；
- `local_wasm_best_effort`：经本设计的本地 Deno/Pyodide 执行。

`control_plane_remote` 是当前代码插件既有的发布信任边界，不宣称能从响应证明某个具体
Provider 产品身份。只有 control-plane Runner 在 Router 选择、策略校验和 remote 执行全部成功
后填写该值；本地 Runner 只填写 `local_wasm_best_effort`。空值或未知值不具备发布资格，不能
为了兼容测试桩默认提升为 remote。

代码插件调试响应新增可选 `execution_assurance` 公共字段并更新 IDL 生成代码。两类执行都可以
返回真实结果并显示“试运行成功”，但只有 `control_plane_remote` 成功才调用
`MarkDebuggedCAS`、推进 `last_debugged_revision` 并使草稿具备发布资格。本地成功仍返回
`execution_assurance=local_wasm_best_effort`，但不写发布资格，页面显示“本地沙箱试运行通过；
发布前需使用远程沙箱验证”，发布按钮继续保持不可用。
这样本地验收证明代码可执行，但不会用低可信执行绕过既有发布门槛。

## 依赖注入与调用链

application infra 新增返回 `(coderunner.Runner, error)` 的模式解析与构造入口，在 Plugin
初始化前完成本地依赖预检、原始开关冲突校验和最终 Runner 选择。Plugin
`ServiceComponents` 新增显式 `CodeRunner coderunner.Runner`；application composition 把最终
选择出的 `basicServices.infra.CodeRunner` 直接传给 Plugin，不再由 Plugin 在初始化期间调用
全局 `coderunner.GetCodeRunner()`。

Workflow 暂时保留现有全局兼容入口，但 `workflow.InitService` 写入的必须是同一个
`basicServices.infra.CodeRunner`。因此 Plugin 与工作流持有的是同一实例，而不是两个配置
相似但生命周期不同的 Runner。后续移除 Workflow 全局值属于独立重构，不在本次扩大。

Skill `ScriptExecutor` 同样接收该实例，并把所有脚本调用标记为 `PurposeSkill`。这不是新增
执行入口，而是把现有隐式消费者纳入相同的选择、隔离、容量和失败关闭合同。

调用关系保持：

```text
Workflow Code -> PurposeAgent  ----\
Skill Script  -> PurposeSkill  -----+--> Shared Local Sandbox Runner
Code Plugin   -> PurposePlugin ----/
```

当控制面 runtime routing 被选择时，这三个 purpose 继续由 control-plane Runner 映射到
既有入口：Agent/Skill 使用 `agent` scope，Plugin 使用 `plugin` scope；本设计不改变远程路由身份。

## 插件草稿与错误反馈

代码插件页面在以下任一条件成立时先保存：

- 当前内容相对保存快照已变更；
- 当前 revision 为 0。

保存成功后必须使用 `SaveCodePluginDraft` 返回的新 revision 调用 `DebugCodePlugin`。保存
失败、页面身份变化、组件卸载或旧请求失效时不发送调试请求。

前端错误展示保留稳定、可操作的分类：草稿保存失败、沙箱未配置、超时、容量、输出超限
和执行失败。JavaScript 本地执行映射到现有 `RuntimeError` 并显示固定“当前本地沙箱仅支持
Python”，本次不扩展 IDL debug status。普通用户不看到宿主路径、stderr 原文或内部配置。
本次只为成功响应增加 execution assurance，不新增错误状态枚举。

## 验证

### 后端自动化

- Plugin 初始化获得显式注入 Runner；初始化顺序不再产生 `nil`。
- Workflow、Skill 与 Plugin 观察到同一个 Runner 实例，并分别发送正确 purpose；新的本地
  Runner 拒绝空 purpose。
- 本地开关开启时 Agent、Skill 与 Plugin 均可执行 Python；关闭时恢复当前基线选择规则，
  Plugin 保持 unavailable，Workflow/Skill 是否可用由原有 legacy 配置决定。
- legacy purpose guard 把 `PurposeSkill` 作为 Agent-compatible 入口放行，验证关闭新模式不会
  破坏既有 Skill Script；未知 purpose 仍失败关闭。
- 完整模式真值表覆盖控制面、本地开关以及 routing 的空、true、false、非法值；尤其验证
  `control-plane=true + routing空值 + local=true` 启动失败。
- 本地与 control-plane routing 同时开启时启动失败。
- 无 Deno、缺 launcher、冷缓存、lock/版本不符、非法开关、未知 purpose、JavaScript、
  超时、取消、非 JSON、trailing bytes、输入/输出超限均按稳定错误失败关闭。
- 文件、环境、网络、FFI、宿主命令与动态 import 探针全部拒绝；离线运行期间不发网络请求。
- timeout/cancel/超限后没有残留 Deno 进程，临时目录和 FD 被回收。
- 并发 Agent/Plugin 不串状态；第 3 个同时请求稳定返回 capacity exhausted。
- Plugin 本地执行不会调用 Router、RemoteProvider、Host Shell 或 direct Runner。
- Plugin 本地成功返回真实结果和 `local_wasm_best_effort`，但不调用 `MarkDebuggedCAS`、
  不推进 `last_debugged_revision`、不解锁发布；control-plane remote 成功保持既有发布资格行为。
- Host Shell 门禁即使全部开启，本地模式下也保持不可用且不回退。
- 现有生产默认、`local_debug` 禁止 plugin scope、Remote Provider Plugin 路由测试保持
  通过。

### 前端自动化

- revision 0 试运行先保存，再用新 revision 调试。
- revision 大于 0 且未修改时不重复保存。
- 自动保存失败时不发送调试请求。
- unavailable、timeout、capacity、output limit 与 execution failed 不再全部显示成同一状态；
  JavaScript 使用现有 RuntimeError 和固定提示。
- 本地成功显示真实结果与低可信提示，发布仍禁用；control-plane remote 成功继续推进
  debug-ready。

### 本地真实验收

使用当前账号和空间，先运行显式依赖准备命令，再重启 dev 后端并确认本地沙箱依赖 ready：

1. 在工作流中运行一个 Python Code 节点，验证输入映射和 JSON 输出；
2. 打开代码插件 `7672703696211279872`，保留 Python 代码与
   `{"message":"hello"}` 输入，直接点击“试运行”；
3. 首次请求顺序必须是保存成功后再调试，revision 大于 0；
4. 页面显示实际 JSON 结果，不再出现“服务不可用”；
5. 页面显示 `local_wasm_best_effort` 提示且发布仍禁用；当前本地页面验收不伪造 remote
   资格，control-plane remote 的同 revision debug-ready 行为由既有自动化与专用 fixture 验证；
6. 关闭本地沙箱开关并重启后恢复当前基线：Plugin unavailable，工作流按现有 legacy 配置
   行为，不允许静默回退 Host Shell 或 Remote Provider；
7. 开关恢复后再次执行成功，证明不是前端缓存或测试 stub；
8. 浏览器控制台和后端日志无本次功能引入的未处理错误或敏感信息。

## 回滚

关闭 `SANDBOX_LOCAL_CODE_RUNNER_ENABLED` 并重启即可恢复现有后端选择基线，不改
数据库。代码回滚不需要清理 Provider、凭据或 workspace。Deno 属于本地开发依赖，删除
它不会触发远程回退，只会让显式本地模式在启动时失败。

## 对既有设计的影响

本设计覆盖
`docs/superpowers/specs/2026-08-13-code-plugin-trial-run-recovery-design.md`
中“dev 只能通过 Native Runner 执行代码插件”的本地开发限制，但不改变其默认草稿恢复、
Native Runner、安全 HTTP、Provider 和正式发布资格合同。部署是否选择本地后端由显式配置
决定，应用本身不使用 `APP_ENV` 白名单。

这项选择改变了当前长期文档中“共享/生产只允许 remote Provider”的绝对表述。实现时必须
同步更新 `docs/superpowers/context/project-context.md` 与 Sandbox 运维 runbook：任何环境都可
显式选择本地后端，但状态必须公开 `local_wasm_best_effort`，不得称为 Native/container 隔离；
共享、多租户或对抗性负载仍推荐并默认使用 HTTPS Native Runner。未设置本地开关时现有默认
行为完全不变。
