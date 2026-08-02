# Dev 自动 Atlas Migration 设计

## 1. 背景与目标

本设计实施前，`dev` 发布工作流会从已晋级的前后端 `:dev` 镜像 revision 比较到
目标提交。比较区间包含 `docker/atlas/migrations/**` 变化时，工作流构建并推送
不可变镜像，但停在 `migration-hold`，等待人工执行 Atlas 后再手工 dispatch。

2026-08-02 的真实发布验证中，已部署 revision
`3206277af3b705f463e668940e5f986ccb0d4bce` 到目标 revision
`65903e6149d62989987e7bc9599988c0326234bf` 包含
`20260728000200_object_storage_configs.sql`。前后端不可变镜像构建成功，但 `:dev`
标签未晋级，宝塔 webhook 未触发。这符合现有设计，但不满足“合入远程 `dev` 后
完全自动完成预发布”的目标。

本次变更让 GitHub Actions 在确认存在 migration 变化后自动执行 Atlas。只有目标
镜像验证和 Atlas apply 都成功，工作流才晋级两张 `:dev` 镜像并调用宝塔 webhook。
任何无法证明安全前提的状态继续 fail closed。

## 2. 方案比较

### 2.1 GitHub Actions 直接执行 Atlas（采用）

GitHub 使用 Repository Secret `ATLAS_URL` 连接远程 dev MySQL，在不可变镜像验证
成功后运行固定版本 Atlas。改动集中在现有工作流，迁移失败会直接阻止晋级和
webhook。GitHub Runner 必须能访问数据库，仓库也需要保存专用 dev 数据库凭据。

### 2.2 宝塔服务器执行 migration image

构建额外的迁移镜像，由 `/opt/coze-dev/deploy.sh` 使用服务器本地数据库凭据执行
后再启动应用。数据库密钥不会进入 GitHub，但需要新增镜像、修改服务器事务和
远程标签晋级时序，还要解决迁移成功而应用回滚时不能 down migration 的问题。
对于当前单实例 dev 环境，复杂度明显更高。

### 2.3 保留人工 migration hold

安全边界最保守，但每次 migration 都要求人工 apply 和 workflow dispatch，无法
满足已经确认的完全自动化目标。

## 3. 已确认决策

- 只自动迁移 dev/预发布数据库，不覆盖生产环境。
- `ATLAS_URL` 只能使用 GitHub Actions Repository Secret，不能使用 Variable 或
  Environment Secret。当前 `migrate` job 没有声明 GitHub Environment。
- `ATLAS_URL` 采用 Atlas MySQL URL，密码中的保留字符必须 URL 编码；
  `.github/atlas-dev.hcl` 只通过 `getenv("ATLAS_URL")` 读取它。
- 自动迁移只由仓库自身 `dev` push 触发；Pull Request、需求分支和 fork 不执行。
- 先构建并验证同一目标 SHA 的前后端不可变镜像，再执行 migration。
- Atlas 成功后才依次晋级两张 `:dev` 标签并调用宝塔 webhook。
- Atlas 失败、数据库不可达、Secret 缺失、迁移目录校验失败或发布基线无法验证
  时，不晋级镜像、不调用 webhook，旧服务继续运行。
- `workflow_dispatch` 保留为已构建 SHA 的受控重放或回滚入口，不执行 down
  migration，也不执行 Atlas；前向重放仅允许目标区间不含 migration。
- 不自动调用云数据库备份 API；数据库备份策略由腾讯云侧独立管理。

## 4. 工作流状态设计

### 4.1 Preflight 输出拆分

现有 `migration_changed` 同时表达“确有 migration diff”和“无法验证发布基线”。
自动执行数据库变更前必须拆成两个独立输出：

- `migration_changed`：只有在基线、目标 SHA、祖先关系和 Git diff 均可验证，且
  比较区间确实包含 migration 变化时才为 `true`。
- `deployment_blocked`：ACR 拉取失败、仅一张 `:dev` 镜像存在、revision 缺失或
  不一致、基线不属于目标历史、Git 对象或 diff 无法读取时为 `true`。

正常无迁移时两个值都为 `false`；正常有迁移时仅 `migration_changed=true`；无法
证明基线时仅 `deployment_blocked=true`。不允许用网络错误或未知状态触发 Atlas。
输出缺失、取值未知或 `deployment_blocked` 不是明确的 `false` 时，
`deployment-blocked` job 会结束为失败，避免 workflow 以 skipped 状态掩盖阻断。

首次部署仍只在两张 `:dev` manifest 都明确不存在时使用 push `before` 作为基线。
此时 diff 可验证且含 migration，可以自动执行 Atlas；只有一张 manifest 缺失仍然
设置 `deployment_blocked=true`。

### 4.2 Job 顺序

工作流按以下顺序执行：

1. `preflight` 解析目标 SHA，输出迁移和阻断状态。
2. `build-server`、`build-web` 并行构建并推送 `dev-<full-sha>`。
3. `verify-images` 拉取两张不可变镜像并验证 OCI revision 等于目标 SHA。
4. `migrate` 在普通 push 且 `migration_changed=true` 时校验并 apply Atlas；没有
   migration 时以成功的 no-op 状态通过。
5. `promote` 只在 `deployment_blocked=false`、镜像验证成功且 `migrate` 成功后
   晋级两张 `:dev` 标签。
6. `deploy` 只在 `promote` 成功后调用宝塔 webhook。

`migration-hold` 被自动迁移 job 取代。若 preflight 无法验证发布基线，构建任务
仍可保留不可变镜像作为诊断产物，`deployment-blocked` 明确失败，verify、migrate、
promote 和 deploy 都不得继续。

## 5. Atlas 执行合同

`migrate` checkout `preflight.target_sha`，使用仓库约定的
`arigaio/atlas:0.35.0-community-alpine`，按以下顺序运行：

1. 确认 `ATLAS_URL` 非空，但不打印其值。
2. `atlas migrate validate --dir file:///migrations` 校验 migration 目录和
   `atlas.sum`。
3. 只用 `docker run --env ATLAS_URL` 传递环境变量名，同时只读挂载 migration
   目录和 `.github/atlas-dev.hcl`。
4. 容器执行 `atlas migrate apply --config file:///atlas.hcl --env dev`；HCL 使用
   `getenv("ATLAS_URL")` 设置数据库 URL 和 migration 目录。

Atlas revision 表和 checksum 提供重复执行语义。同一目标 SHA 因 Runner 中断而
重跑时，已经成功记录的 migration 不重复执行；失败返回非零并阻止后续 job。
Secret 只注入 `Validate and apply Atlas migrations` step，不放在 job 级环境中。
工作流不启用 shell xtrace，不把 DSN 插入 shell source、宿主机命令参数、summary
或 artifact；Docker argv 中只出现环境变量名 `ATLAS_URL`。

自动迁移启用前，远程 dev 数据库必须已经由当前 Atlas migration 目录管理，revision
历史与现有 schema 一致。若该前提不成立，只允许一次性人工校准 revision；不得让
首次自动任务尝试重建已有 schema。

## 6. 权限与网络

- `ATLAS_URL` 必须配置为 GitHub Actions Repository Secret，不能配置为 Variable
  或 Environment Secret。当前 `migrate` job 没有 `environment`，Environment
  Secret 不会进入任务。
- 数据库账号只授予 dev schema 执行仓库 migration 所需权限，不使用云数据库管理
  账号，也不授予其他数据库权限。
- 腾讯云数据库访问控制必须允许 GitHub-hosted Runner 连接；无法建立连接时任务
  失败关闭，不临时扩大权限或跳过 migration。
- 工作流继续使用 `contents: read`；数据库 Secret 只注入
  `Validate and apply Atlas migrations` step，再由 HCL 从容器环境读取。
- GitHub 日志、Docker 参数诊断和错误摘要不得回显 DSN、用户名或密码。

## 7. 失败、重试与回滚

### 7.1 `workflow_dispatch` 关系矩阵

手工任务要求目标是 `origin/dev` 历史中的完整 SHA，且两张目标不可变镜像存在。
Preflight 还要先读到当前两张 `:dev` 镜像的一致 revision：

| 目标 SHA 与当前 revision 的关系 | 处理 |
| --- | --- |
| 相同 | 允许重试已经晋级版本的部署 |
| 目标是当前 revision 的祖先 | 允许应用镜像回滚，不执行 down migration |
| 目标是当前 revision 的后代，区间无 migration | 允许前向重放 |
| 前向区间含 migration、关系无法证明，或双 revision 异常 | `deployment_blocked=true`，任务明确失败 |

所有 dispatch 都设置 `migration_changed=false`，不执行 Atlas。它不能替代含
migration 的 push，也不能用于绕过未知基线。

### 7.2 失败恢复

- 镜像构建或 revision 验证失败时不连接数据库。
- Atlas validate 或 apply 失败时保留不可变镜像，不更新 `:dev`，不调用 webhook。
  在同一个 Actions run 使用 `Re-run failed jobs`；不要新建 dispatch。
- `promote` 可能在第一张 `:dev` 标签更新后失败。此时双 revision 不一致会阻断
  dispatch，必须在同一个 run 使用 `Re-run failed jobs` 完成双标签晋级。
- 两张标签已经晋级而 `deploy` 失败时，可以 dispatch 同一 SHA 重试 webhook 和
  服务器部署，不重复执行 Atlas。
- `deploy` job 超时为 15 分钟。curl 连接超时为 10 秒，总请求窗口为 840 秒；超时、
  TLS 校验失败或非成功响应都使 job 失败。
- 已完成的 forward migration 不执行 down。Migration 必须兼容发布前应用，因为
  Atlas 成功后，晋级或部署仍可能失败，旧代码会暂时运行在新 schema 上。

## 8. 测试与验收

Workflow 合同测试至少覆盖：

- 可验证基线且无 migration：不执行 Atlas，正常晋级和部署。
- 可验证基线且有 migration：镜像验证后执行 validate/apply，再晋级和部署。
- `ATLAS_URL` 缺失或 Atlas 返回非零：不晋级、不调用 webhook。
- ACR 超时、认证失败、revision 不一致或 Git 基线不可用：
  `deployment_blocked=true`，`deployment-blocked` 明确失败，不得执行 Atlas。
- 两张 manifest 都不存在的首次 push：以 `before` 比较；含 migration 时自动 Atlas，
  不含时直接继续。
- 只有一张 manifest 缺失：保持阻断。
- `workflow_dispatch` 覆盖同 SHA 重试、祖先回滚、无 migration 前向重放，以及有
  migration 或关系不可证明时的阻断；任何分支都不执行 Atlas 或 down migration。
- Atlas apply 通过 HCL 的 `getenv("ATLAS_URL")` 取值，Docker argv 不包含 DSN。
- `deploy` 使用 10 秒连接超时和 840 秒总请求窗口，job 上限为 15 分钟。
- Workflow 和文档不包含真实 DSN 或凭据。

真实端到端验收使用一次包含本设计和待执行 migration 的 `dev` push：

1. 两张 `dev-<sha>` 镜像构建并通过 OCI revision 验证。
2. Atlas job 显示 validate 和 apply 成功，日志不包含 `ATLAS_URL` 的 DSN 值。
3. 两张 `:dev` 标签 revision 一致且等于目标 SHA。
4. 宝塔 webhook 成功，服务器 `/healthz` 返回目标 SHA。
5. 对象存储配置表存在，系统管理对象存储接口可正常使用。

## 9. 文档与门禁更新

实现时同步更新：

- `docs/superpowers/context/project-context.md`：dev push 会在确有 migration 时自动
  apply，失败仍 fail closed。
- `docs/superpowers/runbooks/dev-integration-audit.md`：第二次审计报告必须明确列出
  migration 文件、自动数据库副作用和目标 SHA；用户确认该次 push 时同时授权该
  exact SHA 的 dev migration。
- `deploy/dev/README.md`：增加 `ATLAS_URL`、数据库网络和 Atlas revision 基线配置，
  删除人工 migration hold 作为常规流程的说明。
- `docs/superpowers/specs/2026-07-30-dev-automated-deployment-design.md`：标明数据库
  迁移章节已由本设计取代，避免长期事实冲突。

## 10. 不在范围内

- 生产数据库 migration、生产部署和自动 down migration。
- 腾讯云自动备份、白名单或数据库账号的创建与变更。
- 通用数据库发布平台、蓝绿数据库或跨环境 migration 编排。
- 绕过两阶段 `dev` 集成审计，或在未报告 migration 副作用时自动推送。
