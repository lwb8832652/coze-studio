# dev 本地 Atlas 前置发布设计

## 背景

远程 `dev` 当前在检测到 `docker/atlas/migrations` 变化后进入
`migration-hold`，需要人工执行 migration 并再次触发 workflow。此前尝试把 Atlas
迁移放到 GitHub-hosted runner，但 dev MySQL 不支持 TLS，且标准 runner 没有稳定、
受控的数据库出口，无法满足当前部署前提。

本项目的 dev 环境允许短时中断，维护者本机可以访问 dev MySQL，并希望保持发布
操作简单：本地只执行一个发布命令；远程 `dev` 推送成功后，剩余工作全部由 GitHub
Actions、ACR 和宝塔服务器完成。

## 目标

- 用一个本地命令完成 Atlas 校验、forward apply 和精确 `dev` SHA 推送。
- migration 成功前不推送远程 `dev`。
- 推送成功后本地命令立即结束，不轮询或参与远程部署。
- GitHub Actions 自动构建、验证和晋级两张镜像，然后调用宝塔部署 webhook。
- 移除 `migration-hold` 和 GitHub runner 的数据库访问。
- 把该发布入口和授权边界写入 `AGENTS.md` 与 dev 集成 runbook。
- 数据库 credential 只保存在本机受限文件中，不写入仓库、GitHub 或日志。

## 非目标

- 不自动执行 down migration、schema 修复、Atlas baseline 或数据回填。
- 不为普通终端用户安装全局 Git hook；`AGENTS.md` 约束 Codex，但不能阻止用户手工
  执行 `git push`。
- 不改变生产发布流程，也不把该 dev 流程声明为高可用发布方案。
- 不要求 dev MySQL 开启 TLS；现有公网非 TLS 连接风险由运维边界明确记录。

## 方案选择

### 采用：本地 apply 后推送

本地脚本先完成 Atlas 操作，再推送精确 SHA。流程最短，符合当前 dev 运维方式，
并且推送后的所有步骤都在远端完成。

代价是 migration 成功而 push 被拒绝时，schema 可能短暂领先于远程代码。因此所有
forward migration 必须兼容当前仍在运行的旧应用；push 失败后禁止 force push，必须
重新检查远程基准并按集成门禁恢复。

### 未采用：推送、构建后再由本地 apply

该方式能先证明镜像可构建，但需要本地等待 Actions、执行 migration、再触发
`workflow_dispatch`，操作和故障状态更多，不符合本次简化目标。

### 未采用：宝塔 migration webhook 或 self-hosted runner

这两种方式可以把 apply 放到服务器，但需要额外镜像、Webhook 或 runner 权限管理，
当前单实例 dev 环境不采用。

## 本地发布入口

新增 `deploy/dev/publish-dev.sh`。脚本接收第二次集成审计报告中的两个完整 SHA：

```text
publish-dev.sh <expected-origin-dev-sha> <target-dev-sha>
```

脚本按以下顺序执行：

1. 要求当前分支为 `dev`、工作区干净且 `HEAD` 等于 `target-dev-sha`。
2. fetch `origin/dev`，要求远程仍等于 `expected-origin-dev-sha`，且该 SHA 是目标 SHA
   的祖先。
3. 检查本地 Atlas 环境文件存在、权限为 `600`，且只向 Atlas 容器提供
   `ATLAS_URL`；脚本不得打印 URL。
4. 使用仓库固定的 Atlas Docker digest 对 migration 目录执行 `migrate validate`。
5. 对 dev 数据库执行 `migrate status`；状态不可读取、revision 不一致或需要 baseline
   时失败关闭，不执行 apply。
6. 执行 `migrate apply`。没有待执行 migration 时由 Atlas 正常 no-op。
7. 再次 fetch 并核对远程基准、目标 SHA 和工作区，缩小 apply 与 push 之间的竞态
   窗口。
8. 使用精确 refspec 推送 `target-dev-sha` 到 `refs/heads/dev`，禁止 force push。
9. push 成功后立即返回，不查询 Actions、不触发 dispatch，也不访问宝塔。

默认 credential 文件为：

```text
~/.config/coze-studio/dev-atlas.env
```

文件只允许包含本地 dev Atlas 所需变量，至少包含 `ATLAS_URL`。密码中的 URL 保留字符
必须编码。脚本通过 Docker `--env-file` 注入环境，不把 DSN 放入命令参数。可通过
`ATLAS_ENV_FILE` 覆盖路径，但仓库内文件和权限宽于 `600` 的文件一律拒绝。

本机已安装的 Atlas `v0.35.0` 不作为发布依赖。validate、status 和 apply 统一使用
项目固定的 `arigaio/atlas:1.2.3-community-alpine` digest，避免不同维护者的本机版本
改变行为。

## GitHub Actions 状态机

远程状态机调整为：

```text
preflight -> build-server/build-web -> verify-images -> promote -> deploy
```

- 删除 `migrate` job 以及 `ATLAS_URL`、`ATLAS_CA_PEM` Secret 引用。
- 不恢复 `migration-hold`。
- push 中检测到 migration 变化时继续构建和发布，因为 apply 已是 push 的本地前提。
- `deployment-blocked` 继续处理镜像基线缺失、不一致、Git 关系不可证明等异常状态。
- `workflow_dispatch` 仍只处理已存在的不可变镜像，不执行 migration 或 down
  migration；允许重试已经进入 `origin/dev` 的 forward SHA，包括其 Git 区间含
  migration 的情况。
- `promote` 只依赖 preflight、两项 build 结果和不可变镜像验证；成功后更新两张
  `:dev` 标签并调用现有宝塔部署 webhook。

GitHub 无法证明某次手工 `git push` 之前是否执行过本地脚本。这是该精简方案明确
接受的边界：仓库流程、`AGENTS.md` 和双重审计共同约束正式 dev 发布，但不伪造一个
远端不存在的数据库证明。

## 授权与文档

`AGENTS.md` 增加短规则：

- 禁止 Codex 直接执行 `git push origin dev`。
- 第二次审计报告必须列出 comparison base、目标 SHA、全部待执行 migration 和副作用。
- 用户对该报告的第二次明确确认，同时授权本地 Atlas forward apply 和随后对同一目标
  SHA 的非 force push。
- Codex 只能调用 `publish-dev.sh`，并传入报告中的远程基准和目标 SHA。

`docs/superpowers/runbooks/dev-integration-audit.md` 保存完整操作和停止条件；
`deploy/dev/README.md` 保存本机 credential 文件、命令用法和故障恢复说明；
`docs/superpowers/context/project-context.md` 只记录长期发布事实。

## 失败处理

- validate、status 或 apply 失败：不 push，不自动重试数据库操作。
- apply 成功但远程基准变化：不 push；报告 schema 可能领先，重新执行第一次审计。
- apply 成功但网络导致 push 失败：不 force push；确认远程状态后重新审计。Atlas 下次
  apply 对已成功版本应为 no-op，但仍需要新的授权。
- push 成功后的 Actions 或宝塔失败：本地不继续操作；按远程 run 证据处理重跑或
  回滚，回滚不执行 down migration。
- schema/revision 需要 baseline、修复或回填：退出发布脚本，另行申请数据库变更授权。

## 测试策略

- Shell 契约测试使用 fake `git`、`docker` 和隔离环境文件，覆盖分支、SHA、工作区、
  远程竞态、权限、validate/status/apply 失败和精确 push refspec。
- 测试必须证明 migration 失败时不会调用 push，push 成功后不会调用 GitHub API、
  `gh`、curl 或宝塔。
- Workflow 契约测试验证 job 集合、依赖、push/dispatch 条件、无 Atlas Secret、无
  `migration-hold`，以及 migration 变化不阻断镜像晋级。
- 继续运行 Atlas migration hash/validate、YAML 解析、Bash 语法、现有 deploy 和
  Compose 契约测试。

## 验收标准

- 维护者只需执行一次 `publish-dev.sh`。
- migration 失败时远程 `dev` 不变化。
- push 成功后脚本立即结束。
- GitHub Actions 无 `migration-hold`，能够完成镜像推送、晋级和宝塔部署。
- GitHub 不保存或读取数据库 credential，服务器不安装 Atlas。
- 所有发布仍受两阶段 dev 审计和精确 SHA 确认约束。
