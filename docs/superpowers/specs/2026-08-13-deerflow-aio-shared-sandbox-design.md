# DeerFlow/AIO 共享 Sandbox 接入设计

日期：2026-08-13
状态：设计已确认，等待实施计划

## 结论

NewX 接入 DeerFlow 已使用的完整 Sandbox 能力，并把 `agent-infra/sandbox`
All-in-One Sandbox（下文简称 AIO）作为可插拔执行后端。NewX 不嵌入 DeerFlow
Python Provider，也不替换现有 Sandbox Control Plane、Native Runner、Eino ADK、
业务线程或审计事实源。

每个 Runner 部署默认维护一个常驻 AIO 容器，所有用户共享该容器。用户和线程不再
各自创建容器，而是在共享容器内通过线程级 Linux UID/GID、工作目录、Shell Session、
Browser Context、Jupyter Kernel、进程组和端口租约隔离。Runner 对执行并发做硬限制：
最低 `2C4G` 配置下，全局权重预算为 `2`，普通任务权重为 `1`，重型任务权重为 `2`，
因此最多同时执行两个普通任务或一个重型任务。

共享容器提供进程、文件权限和会话级隔离，不提供每用户独立内核边界。AIO 原始 API、
CDP、VNC、Jupyter、VSCode 和预览端口不得直接暴露给用户，必须经过 NewX 的身份、
授权、并发和审计边界。

本机开发另外支持显式 `HostShellBackend`。它只允许在本机 debug 模式启用，不具备
容器隔离能力，也不得作为 AIO 或远程 Provider 失败后的自动回退路径。

## 背景与当前事实

当前 NewX 已经具备以下基础：

- Sandbox Provider、默认 Provider、健康检查、凭据加密、配置审计和执行审计；
- `agent`、`plugin`、`mcp_stdio` 和 `appdev` scope；
- 签名执行身份、Redis 公平队列、容量租约、幂等、取消、恢复和 fail-closed；
- Native Runner 的 rootless 容器驱动、固定镜像 digest、资源上限和清理隔离；
- 最低 `2C4G` 下一个重任务或两个轻任务的权重调度；
- Agent 的 Eino ADK、Subagent、上传文件和 Artifact 业务合同。

当前缺口是持久的线程级 Sandbox Session 和 DeerFlow 风格的完整文件、Shell 及交互
能力。现有 Agent 文件工具主要面向上传文件和输出 Artifact，不能提供完整的 `ls`、
`glob`、`grep`、任意文件修改和容器内 Shell。Native Runner 当前执行一次受审核
Adapter 请求，也不是一个长期会话服务。

锁定的 DeerFlow 参考版本为本地基线
`5851f8250eb150ca23134c79b11ebc5073ac2789`。该版本向 Agent 暴露的 Sandbox 能力是：

- `execute_command`；
- `read_file`、`download_file`、`list_dir`、`write_file`；
- `glob`、`grep`、`update_file`/字符串替换；
- `/mnt/user-data/workspace`、`uploads`、`outputs` 和只读 `skills`；
- 延迟获取、线程复用、释放、回收和工作区恢复；
- 主 Agent 与 Subagent 共享同一线程工作区；
- 只从 `outputs` 显式发布 Artifact。

DeerFlow 的 AIO 封装默认使用单个 Shell Session，并通过进程内锁规避并发损坏。AIO
本身提供显式 Shell Session，因此 NewX Go Adapter 必须按 Thread 创建 Session，不能
照搬 DeerFlow 的全局串行封装。

AIO 上游还提供 Browser、Jupyter、VSCode、Terminal、MCP、VNC 和预览端口。这些是
AIO 的扩展能力，不属于 DeerFlow 当前 Agent 合同，但纳入本设计的最终接入范围。

参考上游：

- `https://github.com/agent-infra/sandbox`
- `https://github.com/agent-infra/sandbox-sdk-go`

实施直接使用 `ghcr.io/agent-infra/sandbox:latest`，不固定 AIO tag、OCI digest 或平台
manifest；Go SDK 仍固定为 `v0.0.5` 以稳定客户端字段和路由合同。每次部署必须重新拉取
并运行真实兼容探针，以镜像自报版本记录观测值，而不是把本轮的 `1.11.0` 转化为锁。

## 目标

1. 完整接入 DeerFlow 当前 Agent-facing Sandbox 合同。
2. 在一个常驻 AIO 容器内支持多个用户、多个 Thread 的受限并发。
3. 保持 NewX 的身份、权限、Provider 路由、调度、审计和业务事实所有权。
4. 支持 Shell、文件、Browser、Jupyter、VSCode、MCP、Terminal 和预览端口。
5. 保持主 Agent 与 Subagent 共享当前 Thread 工作区，禁止访问其他 Thread。
6. 容器重启后保留工作区，并能重建短生命周期 Session。
7. 在 `2C4G` 最低配置下不依赖 OOM 控制容量，不产生无界进程和 Session。
8. 为本机开发提供显式 Host Shell，提高调试效率。
9. 保持当前插件创建、草稿保存、试运行和发布链兼容。
10. 允许 Sandbox remote Provider 使用 HTTP 或 HTTPS，不按环境强制 HTTPS。

## 非目标

1. 不把 DeerFlow Python Runtime、LangGraph 或 Provider 进程嵌入 Go 后端。
2. 不用 AIO 替换 Eino ADK、Workbench Run、Thread、事件、Checkpoint 或 Artifact。
3. 不删除现有 Native Runner 和一次性 Plugin 隔离链。
4. 不承诺共享 AIO 等价于每用户独立容器、VM、gVisor 或 Firecracker 的内核隔离。
5. 不允许前端直接访问 AIO API Key、内部 endpoint 或未经授权的服务端口。
6. 不把 Host Shell 开放到共享 dev、测试或生产。
7. 不在本阶段引入 Kubernetes、多节点自动扩缩容或 AIO 容器池。
8. 不自动迁移、删除或重建现有业务数据和 Sandbox Provider。

## 已选方案与备选方案

### 已选：NewX 控制面 + Native Runner + AIO Backend

保留现有控制面和 Runner，在 Runner 内增加统一 Session 合同、共享 AIO Adapter 和
Gateway。该方案可以复用 NewX 的安全、审计、容量和恢复能力，同时通过标准 Go SDK
接入上游能力。

### 未选：独立 DeerFlow Python Sandbox Service

该方案可以较快复用 DeerFlow Provider，但会形成 Go/Python 两套身份、生命周期、
调度、重试和审计，故障恢复也会出现双事实源，因此不采用。

### 未选：用 AIO/DeerFlow Provisioner 替换 Native Runner

该方案会丢失现有签名身份、持久公平队列、硬容量限制、配置审计和 fail-closed 边界，
并且 DeerFlow 的 soft replica 限制不能保证 `2C4G` 安全，因此不采用。

## 总体架构

```mermaid
flowchart LR
    Agent["Eino Agent / Subagent"] --> Contract["Go Sandbox Session Contract"]
    Plugin["Plugin Trial Run"] --> Contract
    MCP["MCP Runtime"] --> Contract
    AppDev["AppDev"] --> Contract

    Contract --> Router["Sandbox Provider Router"]
    Router --> Runner["Native Runner"]
    Runner --> Scheduler["Weighted Fair Scheduler"]
    Scheduler --> Gateway["AIO Session Gateway"]
    Gateway --> AIO["One Shared AIO Container"]

    Gateway --> Metadata["MySQL Session Metadata"]
    Scheduler --> Lease["Redis Queue / Lease / Cancel"]
    AIO --> Volume["Persistent Thread Workspaces"]
    Gateway --> ObjectStorage["Uploads / Published Artifacts"]

    Local["Local Debug Host Shell"] -. explicit debug only .-> Contract
```

### NewX 控制面

控制面继续负责：

- 从认证上下文确定用户、空间、Thread 和 Run；
- 选择健康且支持目标 Profile 的 Provider；
- 生成签名身份、幂等键和策略快照；
- 保存业务状态、审计和安全错误投影；
- 生成交互服务的短期访问授权；
- 管理 Provider endpoint、凭据、并发上限和能力开关。

### Native Runner

Runner 继续是执行面的安全和容量边界，并新增：

- AIO 容器生命周期和 `runtime_generation`；
- Thread Session 创建、恢复、清理和取消；
- 线程级 UID/GID、目录、进程组、Kernel、Context 和端口租约管理；
- 普通、重型任务的加权公平调度；
- AIO API 的私有调用和输出裁剪；
- Browser、Jupyter、VSCode、Terminal、MCP 和预览的授权代理。

### AIO 容器

每个 Runner 部署默认只运行一个常驻 AIO 容器。容器提供统一文件系统和能力服务，
但不拥有业务身份、权限、队列、最终任务状态或长期审计。容器内存中的 Shell、Browser
和 Kernel 状态均可重建，不能成为唯一事实源。

### HostShellBackend

Host Shell 实现同一 Go Session 合同，但直接在本机线程目录执行。它必须同时满足：

- `APP_ENV=debug`；
- 显式开启本地 Host Shell 配置；
- 只绑定 loopback；
- 页面和审计明确标记“宿主机执行”；
- 不作为任何远程或 AIO 失败的回退路径。

Host Shell 可以访问开发机上的其他路径和凭据，因此只适用于可信本机开发，不得描述
为 Sandbox 安全隔离。

## 能力分层

### Core Profile

Core Profile 对齐 DeerFlow 当前完整合同：

- 创建、取得、释放、取消和清理 Shell Session；
- 执行前台、后台和流式命令；
- 读取、下载、列目录、写入、追加、替换和删除文件；
- `glob`、`grep` 和文件元数据；
- Thread 工作区、上传区、输出区和只读 Skill；
- 主 Agent/Subagent 工作区共享；
- 从 `outputs` 显式发布 Artifact。

### Interactive Profile

Interactive Profile 增加：

- 独立 Browser Context、截图和 CDP 代理；
- 独立 Jupyter Kernel；
- VSCode 工作区入口；
- Web Terminal；
- AIO MCP 服务；
- 线程级预览端口租约和代理。

选择 Interactive Profile 不代表全部重型服务同时启动。Gateway 只按实际请求创建
Context、Kernel 或代理租约，空闲资源必须按策略回收。

## 统一 Go 合同

业务层依赖 NewX 自有接口，不直接依赖上游 SDK 类型：

```go
type SandboxSessionManager interface {
	Acquire(ctx context.Context, req AcquireSessionRequest) (SandboxSession, error)
	Get(ctx context.Context, ref SessionRef) (SandboxSession, error)
	Release(ctx context.Context, ref SessionRef) error
	Destroy(ctx context.Context, ref SessionRef) error
	Recover(ctx context.Context, ref SessionRef) (SandboxSession, error)
}

type SandboxSession interface {
	Exec(ctx context.Context, req ExecRequest) (ExecutionStream, error)
	Read(ctx context.Context, req ReadRequest) (FileContent, error)
	Write(ctx context.Context, req WriteRequest) error
	List(ctx context.Context, req ListRequest) ([]FileEntry, error)
	Glob(ctx context.Context, req GlobRequest) ([]FileEntry, error)
	Grep(ctx context.Context, req GrepRequest) ([]GrepMatch, error)
	Replace(ctx context.Context, req ReplaceRequest) error
	Download(ctx context.Context, req DownloadRequest) (io.ReadCloser, error)
	Publish(ctx context.Context, req PublishRequest) (ArtifactDescriptor, error)
}
```

Interactive 能力使用独立可选接口，避免 Core Consumer 被迫依赖浏览器或 IDE：

```go
type InteractiveSandboxSession interface {
	SandboxSession
	Browser(ctx context.Context, req BrowserRequest) (BrowserLease, error)
	Jupyter(ctx context.Context, req JupyterRequest) (KernelLease, error)
	Service(ctx context.Context, req ServiceRequest) (ServiceLease, error)
}
```

Adapter 至少包括：

- `AIOBackend`：生产、共享 dev 和本地容器模式；
- `HostShellBackend`：仅本机 debug；
- 现有 Native one-shot Adapter：继续服务未迁移的 Plugin 调用。

## 协议演进与兼容

现有 `coze.sandbox.execute.v1` 保持不变，用于一次性、幂等执行。共享 AIO 新增
`sandbox_session_v1` Provider capability 和会话端点，不把长期 Session 强塞进一次性
执行协议。

签名执行身份新增 v2 合同，至少包含：

- `scope`；
- `space_id`；
- `user_id`；
- `thread_id`；
- `run_id`/`execution_id`；
- `profile`；
- `request_digest`；
- `issued_at`、`expires_at`、`nonce` 和 `key_id`。

Agent 的 Thread ID 必须来自服务端业务记录。当前 Agent scope 不接受 Session ID 的
v1 校验保持兼容；只有声明并通过 `sandbox_session_v1` 握手的 Provider 使用 v2。

当前 Plugin 试运行默认继续走一次性 Native Adapter。第二阶段通过合同测试后才允许
切换到 AIO Core Profile；切换前后草稿 revision、输入输出和错误合同必须一致。

## 共享容器、用户与线程模型

### 隔离层级

```text
Runner Deployment
└── One Shared AIO Container
    ├── User A
    │   ├── Thread 1 (runtime UID/GID 20001): workspace + shell + process group
    │   └── Thread 2 (runtime UID/GID 20002): workspace + browser/kernel leases
    └── User B
        └── Thread 3 (runtime UID/GID 20003): workspace + shell + process group
```

所有用户共用 AIO 容器，但不能共用默认 Shell Session、工作目录、Browser Context、
Jupyter Kernel、进程组或端口租约。

### 业务用户与运行身份

NewX 中的用户身份仍由认证上下文和业务记录确定。Runner 为
`provider_id + space_id + user_id + thread_id` 分配稳定且非 root 的运行 UID/GID。
映射保存在 MySQL，并在单个 Runner Provider 范围内保证唯一。不能仅使用哈希截断生成
UID，以免碰撞后跨 Thread 访问。

运行身份按 Thread 分配，而不是按用户分配。否则同一用户的两个 Thread 会共享 Unix
文件权限，`0700` 无法阻止它们互相读取。Subagent 继承父 Thread 的运行 UID/GID，只有
明确的业务共享操作才能通过对象存储或审核后的共享目录交换文件。

AIO Adapter 在创建 Session 和执行命令时必须把命令降权到对应 Thread UID/GID。公开
AIO API 不支持按 Session 或命令指定 UID/GID，因此必须在 NewX 适配镜像中增加受审核的
执行代理；生产环境不得退回所有用户共用 `gem`、root 或用户级共享 UID。

### 线程身份

Session 的稳定业务键为：

```text
provider_id + space_id + user_id + thread_id + profile
```

上游 Shell Session ID、Browser Context ID 和 Kernel ID 只属于当前
`runtime_generation`，不作为业务主键，也不能由前端提交。

### 工作目录

逻辑虚拟目录保持 DeerFlow 兼容：

```text
/mnt/user-data/workspace
/mnt/user-data/uploads
/mnt/user-data/outputs
/mnt/skills
```

Gateway 将逻辑目录映射到线程专属物理目录。空间、用户和 Thread 段使用服务端生成的
安全标识，不把未经校验的名称拼入路径。Thread 目录归对应运行 UID/GID 所有，权限为
`0700`；`skills` 只读。文件 API 和 Shell 同时依赖路径规范化、软链接检查和操作系统
权限，任一检查失败即拒绝。

普通中间文件保存在持久工作区，`uploads` 和 `outputs` 与对象存储合同集成。
`node_modules`、缓存和构建中间文件不做每次对象存储同步。只有 `outputs` 中被显式
发布的文件生成 Artifact，不能自动暴露整个工作区。

### Subagent

Subagent 继承父 Agent 的 `SessionRef` 和 Thread 工作区，不创建另一个用户或 Thread
Session。递归 Subagent 是否允许由 Agent Runtime 策略决定，与 Sandbox 隔离合同无关。

## 并发和资源模型

### 默认容量

最低 `2C4G` 配置沿用 Native Runner 的权重预算：

| 任务类型 | 示例 | 权重 | 默认并发上限 |
| --- | --- | ---: | ---: |
| Core | Shell、文件、普通插件试运行 | 1 | 2 |
| Heavy | Browser、Jupyter、VSCode、重型构建 | 2 | 1 |

全局权重预算为 `2`。因此两个 Core 可以并发，一个 Heavy 独占预算；Heavy 与 Core 默认
不同时执行。单用户默认最多一个正在执行的任务，防止一个用户占满低配节点。可以保留
最多 20 条空闲 Thread Session 元数据，但默认最多只保留 4 个空闲上游 Shell 进程；
其余 Session 在下次调用时重建。保留 Session 元数据不等于保留正在运行的命令、Kernel
或浏览器页面。

所有数值由系统配置管理并做上下界校验。单个业务请求不能提高容量，Runner 也不能
因为 AIO 仍能接收请求就绕过调度器。

### 公平与排队

复用现有 Redis 持久公平队列、权重槽位、Lease、取消和恢复机制。调度至少保证：

- 按空间和用户轮转；
- 同一用户默认一个 active task；
- Heavy 不被持续 Core 流量永久饿死；
- 超出并发进入队列，不返回通用 500；
- 排队支持取消和截止时间；
- Redis 不保存长期业务事实或明文凭据。

### 进程和资源清理

每个 Thread 使用独立进程组。取消、超时和 Session 清理必须终止整个进程组，不能只
终止父 Shell。后台进程、Jupyter Kernel、Browser Context 和端口租约均有独立 TTL，
不能随 Thread Session 永久累积。

AIO 容器持续运行并接受健康检查。它不是每次命令冷启动，也不是每个用户或 Thread
创建一个容器。

### `2C4G` 宿主预算

共机最低配置仍是 `2C4G`，Runner 常驻预算保持约 `128 MiB`；调度器在宿主机可用内存
低于安全水位时停止出队。按用户确认的官网启动合同，AIO 容器启动不要求设置 memory、
CPU、PID 或 shm cgroup 参数；兼容探针和 Compose 不虚构这些限制。容量安全由全局权重、
单用户上限、进程组、`rlimit`、TTL、输出/磁盘限制和实测水位共同约束，并在系统管理页
明确标记为共享容器配额，不能宣称是独立 cgroup。

若 Core Profile 在 `2C4G` 宿主实测出现 OOM 或无界增长，第一阶段不得上线。若
Interactive Profile 无法通过真实浏览器和 Jupyter 压测，则只关闭 Interactive Profile。

### 动态配置

并发和资源策略由 Sandbox 系统配置管理，不硬编码在业务调用方。第一阶段至少提供：

- 全局权重预算，默认 `2`；
- Core 和 Heavy 权重，默认分别为 `1`、`2`；
- 单用户 active task 上限，默认 `1`；
- 空闲 Session 元数据上限，默认 `20`；
- 空闲上游 Shell 进程上限，默认 `4`；
- 命令、后台进程、Kernel、Context 和服务租约 TTL；
- AIO 容器观测水位以及磁盘、输出、进程和运行时间上限；
- Host Shell、Core Profile 和 Interactive Profile 独立开关。

安全范围内的并发、TTL 和容量水位可以动态更新；已执行任务继续使用入队时的配置快照。
降低上限后不强杀已运行任务，但停止新的出队，直到用量回到新上限。AIO endpoint、
`sessiond` service token/grant key、传输模式和 Host Shell 环境门禁属于启动或 Provider 安全配置，修改后
必须重新健康检查，不能当作普通热更新参数。

## 持久化与运行状态

### MySQL

MySQL 保存：

- Provider 范围内的稳定 Thread UID/GID 映射；
- Thread Session 的业务键、Profile、状态和最后活动时间；
- `runtime_generation` 和上游短期资源映射；
- 配置版本、审计和安全恢复原因。

上游 Session、Context 和 Kernel 标识只用于恢复判断，容器重启后必须失效并重建。

实现使用三张新增表，名称在实施计划中保持固定：

| 表 | 用途 |
| --- | --- |
| `sandbox_runtime_identities` | Provider 范围内的 Thread UID/GID 映射和回收状态 |
| `sandbox_runtime_sessions` | 稳定业务键、Profile、generation、上游资源映射和最后活动时间 |
| `sandbox_runtime_service_leases` | Browser、Jupyter、VSCode、Terminal、MCP 和预览的短期租约 |

三张表只通过新的 Atlas 增量迁移创建。不得 drop、rename、truncate 或重建现有 Sandbox
表，也不得在应用启动时用自动建表代替迁移。唯一键必须覆盖 Provider 和完整线程业务键；
并发创建通过数据库唯一约束和事务处理，不能依赖进程内锁。

### Redis

Redis 保存短期队列、并发令牌、Lease、分布式锁、取消信号和有 TTL 的运行投影。
Redis 丢失时通过 MySQL、Runner generation 和原幂等键 reconcile，不能创建第二个逻辑
Run 或盲目重放命令。

### 持久卷和对象存储

持久卷保存线程工作区。对象存储保存用户上传和已发布 Artifact。AIO 容器可销毁的
内存状态不得作为文件唯一副本。

## 执行数据流

1. Agent、Plugin、MCP 或 AppDev 发起 Sandbox 能力调用。
2. NewX 从认证和业务记录生成用户、空间、Thread、Run 和 Profile 身份。
3. Provider Router 选择健康且声明对应 capability 的 Provider。
4. Runner 验证签名、摘要、时间窗、nonce、权限和策略版本。
5. 调度器按任务权重和公平策略取得执行额度。
6. Gateway 获取或创建稳定 Thread UID/GID、Thread 目录和 Session。
7. AIO Adapter 使用显式 Session ID、UID/GID、工作目录、环境和超时执行。
8. 输出以流式、限长的 stdout/stderr、退出码和状态返回。
9. 取消或超时终止当前 Thread 的进程组，并释放对应资源租约。
10. 显式发布的 `outputs` 文件进入 Artifact 安全链；其他文件不投影到前端。
11. Run 完成后释放调度权重，Thread Session 可以保留，AIO 容器继续常驻。

## Interactive 服务访问

Browser、Jupyter、VSCode、VNC、Terminal、MCP 和预览端口不得把 AIO 内部 URL 直接
返回浏览器。访问流程为：

1. NewX 校验当前用户对空间和 Thread 的访问权。
2. NewX 签发短有效期、单服务、单 Thread 的 capability token。
3. Gateway 校验 token、用户会话、Provider、服务类型和 runtime generation。
4. Runner 代理到对应 Browser Context、Kernel、VSCode workspace 或端口租约。
5. token 到期、Thread 关闭、generation 变化或权限撤销后立即失效。

Browser 必须使用独立 Context；Jupyter Kernel 和 VSCode/Terminal 进程必须以当前
Thread UID/GID 运行；VSCode 打开当前 Thread 工作目录。预览端口由 Runner 分配并绑定
Thread，用户不能声明任意宿主机端口。NewX 只代理 Context 或服务租约，不把 Browser
级 CDP endpoint 直接交给用户。

## 网络与传输

### AIO 内部网络

AIO `8080` 只绑定 Runner 私有 loopback 或私有容器网络。官方 raw AIO 默认模式不要求
JWT/API Key，因此私网与 loopback 是必要边界；SDK Adapter 支持可选 Bearer，但不能把
它描述成默认认证。NewX `sessiond` 仍使用独立 service token 和 HMAC grant。上游文档
明确说明容器内监听 `0.0.0.0`，本设计不得把该端口直接发布给用户。

官网启动要求显式 `seccomp=unconfined`；本地 ARM64 的 `cryptography 49.0.0`
`_rust.abi3.so` 默认会 SIGILL/132，实测 `OPENSSL_armcap=0` 后 health、`/v1/ping` 和
`/v1/sandbox` 正常。部署保留这两个启动项，并把 seccomp 放宽记录为已接受风险；不得
再叠加 privileged 或宿主 Docker socket。

### Remote Provider

NewX 到 Runner 的 remote Provider 支持 HTTP 和 HTTPS，协议由每条 Provider endpoint
选择，不按本地、dev 或生产环境强制 HTTPS。两种协议都必须保留：

- Provider credential；
- 签名执行身份和请求摘要；
- 时间戳、nonce 和防重放；
- 精确 scheme/host/port 锁定；
- 禁止跨 origin 重定向；
- SSRF、DNS rebinding、metadata 和特殊地址防护；
- 请求、响应、超时和输出大小限制。

HTTP 不提供链路保密性。使用内网或公网 HTTP 时，系统管理页必须显示未加密传输风险，
服务器安全组或防火墙必须限制 Runner 端口来源。签名和 API Key 不能被描述为 TLS 的
替代品。

### Sandbox 出站

容器出站默认拒绝。按 Profile 和策略通过 Runner 管理的代理开放域名及端口白名单，
禁止访问云 metadata、宿主机网关、容器控制接口、NewX 管理面和 Secret 服务。网络
策略在 AIO 用户进程之外执行，不能依赖 Agent 自律。

## 本机 Host Shell

本机 dev 允许显式选择 Host Shell：

- 使用线程专属工作目录作为默认 `cwd`；
- 清理继承环境，只注入审核后的变量；
- 设置命令超时、输出上限、并发限制、低优先级和进程组回收；
- 只接受服务端生成的 Thread 身份；
- 记录用户、空间、Thread、Run、耗时、退出码和脱敏审计；
- 页面持续显示宿主机风险标识。

Host Shell 无法阻止命令读取开发用户可访问的其他文件，也无法提供硬 CPU/内存和网络
隔离。该限制属于产品可见事实，不能用目录校验掩盖。

## 故障恢复

### AIO 重启

Runner 每次发现 AIO 实例变化时递增 `runtime_generation`。旧 Shell Session、Browser
Context、Jupyter Kernel 和端口租约全部失效；线程工作区继续保留。下一次调用自动创建
新资源并更新映射。

正在执行的命令标记为基础设施中断，默认不自动重放。命令可能已经产生文件、数据库或
外部 API 副作用，只有业务层明确证明幂等时才允许重试。

### Session 异常

- 上游返回 Session 404：只重建当前 Thread Session；
- Shell Session 损坏：清理当前进程组并重建，不盲目重复原命令；
- Browser Context 或 Kernel 丢失：按当前 Thread 重建；
- 工作目录权限或 UID 映射异常：fail closed，不降级到共享用户；
- Redis、MySQL、签名 keyring、API Key 或持久卷不可用：停止接收新任务。

### 共享故障域

单个 AIO 崩溃会中断所有正在执行的 Thread，这是共享容器方案的已知故障域。健康检查、
自动重启、generation fencing 和持久目录恢复用于缩短故障时间，但不能把共享容器描述
为无单点故障。未来可以在不改变业务合同的前提下扩展 AIO 容器池。

## 错误合同

后端和前端必须区分以下稳定状态：

| 原因码 | 用户可见含义 |
| --- | --- |
| `SANDBOX_QUEUED` | 正在等待 Sandbox 资源 |
| `SANDBOX_QUEUE_TIMEOUT` | 等待资源超时 |
| `SANDBOX_CAPACITY_EXCEEDED` | 当前资源不足或达到并发上限 |
| `SANDBOX_SESSION_NOT_FOUND` | Session 已失效，允许按合同恢复 |
| `SANDBOX_RUNTIME_RESTARTED` | Sandbox 服务重启导致本次执行中断 |
| `SANDBOX_PATH_DENIED` | 文件路径或权限被拒绝 |
| `SANDBOX_EXECUTION_FAILED` | 命令完成但退出失败，保留安全退出码 |
| `SANDBOX_UNAVAILABLE` | Provider、AIO 或必要依赖不可用 |
| `SANDBOX_CANCELLED` | 用户或系统取消任务 |

普通用户不接收 Provider endpoint、API Key、宿主机路径、UID/GID、内部栈或上游响应体。
系统管理员可以在审计和健康页查看经过脱敏的 provider、generation、延迟、并发和原因码。

## 安全边界与已接受风险

共享 AIO 的安全边界是线程级 Linux UID/GID、文件权限、进程组、路径校验和 NewX Gateway，
不是独立内核。获得任意 Shell 的恶意用户若利用容器内核或 AIO 服务漏洞，可能影响同一
AIO 容器中的其他用户。该风险已作为“所有用户共用一个 AIO 容器”的设计约束接受。

因此生产启用必须满足：

- AIO `latest` 每次部署重新拉取、扫描并通过真实兼容探针；Go SDK 固定为 `v0.0.5`；
- AIO 使用官网要求的 `seccomp=unconfined`，这是明确接受的上游风险；不得使用 privileged；
- 不挂载宿主机 Docker Socket、Secret 目录或任意宿主机路径；
- 持久卷只包含受管理的线程工作区；
- AIO API 和交互服务保持私有；
- 每条命令和文件请求都重新绑定服务端身份；
- UID/GID 降权能力缺失时生产启动失败；
- 容量、PID、文件描述符、输出、磁盘和运行时间均有限制。

如果未来用户可信度或合规要求提高，应切换为 AIO 容器池或每租户容器；本设计的 Session
合同和 Gateway token 不依赖单容器实现，可以平滑演进。

## 分阶段实施

### 第一阶段：Session 基础和 Core Backend

- 完成 AIO `latest` 及 Go SDK 的真实兼容性探针；
- 增加统一 Go Session 合同和 `sandbox_session_v1` capability；
- 增加签名身份 v2、Thread UID/GID 映射和线程工作区；
- 增加共享 AIO 容器、显式 Shell Session、文件能力和 generation 恢复；
- 接入现有权重调度，默认 Core 并发 2、Heavy 并发 1；
- 增加本机 debug Host Shell；
- 保持现有 Plugin one-shot 链不变。

第一阶段的 AIO 兼容性探针必须实际验证多 Session 并发、UID/GID 能力边界、文件 API、
取消和 Session 清理。公开 API 不提供 UID/GID，因此生产降权必须由 NewX `sessiond`
补齐，不能只在客户端假设支持。真实上游顺序为 Exec → View → Wait → Kill；被 Kill 的
Session 后续 Cleanup 允许成功或 404，另一个活 Session 必须 Cleanup 成功。

### 第二阶段：Agent、Subagent、Plugin 和 Artifact

- Eino Agent 接入完整 DeerFlow Core 工具；
- 主 Agent 与 Subagent 共享 Thread Session；
- `uploads`、`outputs`、Skill 和 Artifact 合同接入；
- Plugin 经兼容测试后切换到 Core Profile；
- 保留 Provider 开关，可回滚到现有一次性 Adapter。

### 第三阶段：Interactive 能力

- Browser Context 和 CDP/VNC 代理；
- Jupyter Kernel；
- VSCode、Terminal、MCP 和预览端口；
- 短期 capability token 和 Gateway；
- Heavy 权重、空闲回收和 `2C4G` 压测。

## 测试与验收

### 合同与单元测试

- AIO SDK Adapter 的请求映射、超时、取消、错误裁剪和可选 Bearer 注入；
- v1 one-shot 与 v2 Session 身份兼容；
- Thread UID/GID 分配唯一性、并发创建和回收；
- Session、Context、Kernel、端口和 generation 状态机；
- 路径规范化、软链接、权限和 Artifact 输出边界；
- Host Shell debug 门禁和非 debug fail-closed；
- HTTP/HTTPS Provider、签名、防重放、SSRF 和重定向策略。

### AIO 真实合同测试

- 两个显式 Shell Session 并发执行，不出现 DeerFlow 默认 Session 的并发损坏；
- 同一用户两个 Thread 的 `cwd`、环境和输出互不串扰；
- 任意两个 Thread UID 无法读取、列出或写入对方 `0700` 目录；
- 恶意 `..`、绝对路径和软链接不能越过 Thread 边界；
- 取消只终止当前 Thread 进程组；
- AIO 重启后 generation 更新，工作区保留且 Session 可重建；
- 浏览器 Cookie/Storage 在 Context 间隔离；
- Jupyter 变量在 Kernel 间隔离；
- VSCode、MCP 和预览授权只能访问绑定 Thread。

### 调度与资源测试

- 两个 Core 并发执行，第三个进入公平队列；
- 一个 Heavy 独占权重预算，Core 不越过容量硬限制；
- 单用户并发上限、跨空间轮转、取消和排队截止时间有效；
- Session 和后台进程在 TTL 后释放，无进程、Kernel、Context 或端口泄漏；
- `2C4G` 共机压测期间 NewX 主服务不 OOM，Runner 不依赖宿主机 OOM Killer；
- AIO 异常时不回退 Host Shell，也不重复执行有副作用命令。

### 业务回归

- Agent Shell、文件、上传、Subagent 和 Artifact；
- 代码插件创建、草稿加载、保存、试运行、失败提示和发布；
- MCP stdio 和 AppDev 现有路径在未切换前无行为变化；
- 前端正确展示排队、容量不足、执行失败、服务重启、不可用和取消；
- 系统管理页显示 Provider capability、generation、并发和 HTTP 风险状态；
- 页面与日志不泄露内部路径、凭据、UID/GID 或上游错误正文。

## 发布与回滚

每个阶段使用独立 capability 和 Provider 开关灰度。启用顺序为：

1. 部署 AIO 与 Runner Adapter，但保持 Session 路由关闭；
2. 完成健康、兼容性、UID/GID、并发和持久卷检查；
3. 只对测试空间启用 Core Profile；
4. 验收 Agent 和 Plugin 后扩大范围；
5. Interactive Profile 独立启用。

回滚时关闭 `sandbox_session_v1` 路由，保留 Session 元数据和持久工作区，不删除业务
文件或 Provider。Plugin 回到现有 one-shot Adapter；Agent 不允许回退宿主机。数据库
迁移只能增量执行，回滚应用版本不得 drop 表或清空数据。

## 与现有设计的关系

- `2026-08-11-sandbox-runner-2c4g-design.md` 继续定义 Native Runner、权重调度、队列、
  rootless 容器和一次性/混合隔离基线。
- 本设计新增“共享 AIO Session Profile”，仅在该 Profile 内采用一个常驻共享容器，
  不把现有 Native one-shot Plugin 容器改成共享容器。
- `2026-08-13-code-plugin-trial-run-recovery-design.md` 的草稿修复和 HTTP/HTTPS remote
  Provider 调整继续独立实施；其中 AIO 非目标只表示不属于该缺陷修复，不表示永久不接入。
- 实施完成并准备启用生产流量前，必须同步更新项目长期上下文、Sandbox 运维手册和
  HTTPS-only 的旧描述；设计文档本身不提前改变尚未上线的运行事实。

## 最终验收标准

本设计完成的判定不是“AIO 健康检查成功”，而是同时满足：

1. DeerFlow Core 能力全部通过真实 AIO 合同测试；
2. 一个常驻 AIO 容器能够在 `2C4G` 宿主准入边界下可靠执行两个并发 Core Session；
3. 不同用户和 Thread 的文件、进程和交互状态按本设计隔离；
4. AIO 重启不丢工作区，且不会盲目重放命令；
5. Agent、Subagent、Plugin 和 Artifact 业务回归通过；
6. Interactive 能力全部经过 NewX Gateway 授权；
7. `2C4G` 压测无 OOM、无无界队列、无资源泄漏；
8. 本机 Host Shell 仅 debug 显式启用，所有远程环境 fail closed；
9. HTTP/HTTPS Provider 均按配置工作，HTTP 风险明确可见；
10. 关闭 Session Profile 后可以恢复现有执行链，且不删除任何用户数据。
