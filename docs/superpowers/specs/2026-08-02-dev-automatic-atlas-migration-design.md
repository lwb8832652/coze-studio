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

GitHub 使用 Repository Secret `ATLAS_URL` 连接远程 dev MySQL；私有 CA 通过可选
Repository Secret `ATLAS_CA_PEM` 临时挂载。在不可变镜像验证成功后运行固定版本
Atlas。迁移失败会直接阻止晋级和 webhook。GitHub Runner 必须通过受控网络访问
数据库，仓库需要保存专用 dev 数据库凭据。

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
- `ATLAS_URL` 必须是 `mysql://` URL，query 中包含唯一的 `tls=true`。密码中的保留
  字符必须 URL 编码；`.github/atlas-dev.hcl` 只通过 `getenv("ATLAS_URL")` 读取它。
- 私有 CA 使用可选 Repository Secret `ATLAS_CA_PEM`。Secret 存在时 URL 必须精确
  声明 `ssl-ca=/atlas-ca.pem`；Secret 缺失时 URL 不得声明 `ssl-ca`。
- 自动迁移只由仓库自身 `dev` push 触发；Pull Request、需求分支和 fork 不执行。
- 先构建并验证同一目标 SHA 的前后端不可变镜像，再执行 migration。
- Atlas 成功后才依次晋级两张 `:dev` 标签并调用宝塔 webhook。
- Atlas 失败、数据库不可达、Secret 缺失、迁移目录校验失败或发布基线无法验证
  时，不晋级镜像、不调用 webhook，旧服务继续运行。
- `workflow_dispatch` 保留为已构建 SHA 的受控重放或回滚入口，不执行 down
  migration，也不执行 Atlas；前向重放仅允许目标区间不含 migration。
- 不自动调用云数据库备份 API；数据库备份策略由腾讯云侧独立管理。
- `migrate` 保持 `runs-on: ubuntu-latest`。改用 self-hosted 或 larger runner 前必须
  对实际配置另行审计。
- 第二次集成审计使用 ACR 当前双 `:dev` revision 到目标 SHA 的实际部署区间授权
  migration；首次部署使用已验证 push `before`，不以 `origin/dev...dev` 代替。

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

第二次集成审计必须先从 ACR 读取两张当前 `:dev` 镜像的 OCI revision，验证它们是
合法且一致的完整 SHA，再按 `<deployed-revision>..<target-sha>` 列出 workflow 会
识别的全部 migration。首次部署改用
`<verified-push-before>..<target-sha>`。Git 对象、祖先关系或 diff 无法证明时，不得
请求推送确认。每个待执行 migration 都需要数据库副作用和旧应用兼容性证据，授权
只覆盖报告中的文件与 exact SHA。

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

1. 确认 `ATLAS_URL` 非空、使用 `mysql://` scheme，并且 query 中只有一个
   `tls=true`；任何失败都不打印 URL。
2. 校验 `ATLAS_CA_PEM` 与 `ssl-ca` 的成对关系。没有 CA Secret 时允许公共可信 CA，
   URL 不得声明 `ssl-ca`；有 Secret 时 URL 必须精确指向 `/atlas-ca.pem`。
3. 有 CA Secret 时写入 `$RUNNER_TEMP/atlas-ca.pem`，设置模式 `600`，注册退出删除
   trap，并准备只读挂载到容器同名固定路径。
4. `atlas migrate validate --dir file:///migrations` 校验 migration 目录和
   `atlas.sum`。
5. 只用 `docker run --env ATLAS_URL` 传递环境变量名，同时只读挂载 migration
   目录和 `.github/atlas-dev.hcl`。
6. 容器执行 `atlas migrate apply --config file:///atlas.hcl --env dev`；HCL 使用
   `getenv("ATLAS_URL")` 设置数据库 URL 和 migration 目录。有私有 CA 时，apply
   容器额外获得 `$RUNNER_TEMP/atlas-ca.pem:/atlas-ca.pem:ro` 挂载。

Atlas revision 表和 checksum 记录成功状态；失败返回非零并阻止后续 job。失败后
仍须先检查 `atlas migrate status` 和数据库证据，不能仅凭 revision 表存在就假定
可以重跑。两个 Secret 只注入 `Validate and apply Atlas migrations` step，不放在
job 级环境中。工作流不启用 shell xtrace，不把 DSN 或 PEM 插入 shell source、
宿主机命令参数、summary 或 artifact；Docker argv 中只出现环境变量名和 CA 挂载
路径，不出现 Secret 内容。

自动迁移启用前，远程 dev 数据库必须已经由当前 Atlas migration 目录管理，revision
历史与现有 schema 一致。若该前提不成立，先在单独授权下对
`<AUTHORIZED_DEV_DATABASE>` 检查 `atlas migrate status` 和实际 schema，再按证据
选择一次性 baseline。`--baseline <MIGRATION_VERSION_SELECTED_FROM_SCHEMA_EVIDENCE>`
接收 migration 文件名的版本时间戳，不接收 Git SHA；baseline 会写 Atlas revision，
必须取得独立数据库变更授权，不能由常规 push 代替。本文不提供可直接连接远程数据
库的命令或真实参数。

## 6. 权限与网络

| Repository Secret | 要求 |
| --- | --- |
| `ATLAS_URL` | 必填；`mysql://` URL，query 中有唯一 `tls=true` |
| `ATLAS_CA_PEM` | 可选；私有 CA 的 PEM。配置时 URL 必须含 `ssl-ca=/atlas-ca.pem` |

两个值都不能配置为 Variable 或 Environment Secret。当前 `migrate` job 没有
`environment`，Environment Secret 不会进入任务。

- 数据库账号只授予 dev schema 执行仓库 migration 所需权限，不使用云数据库管理
  账号，也不授予其他数据库权限。
- `migrate` 保持 `runs-on: ubuntu-latest`。标准 GitHub-hosted runner 出口范围多且
  变化，不把整段 GitHub 地址加入数据库白名单，也不把 `0.0.0.0/0` 视为连通性验证。
  只有 dev 数据库通过受控网络策略安全可达，并从本次实际 runner 验证成功时才启用。
- 需要稳定白名单时，使用已经配置的 self-hosted runner 或支持静态出口 IP 的 larger
  runner；修改 `runs-on` 前另行审计 runner 加固、容量、凭据和网络边界。
- 当前腾讯云 TDSQL-C dev 地址已验证返回 `MySQL server does not support SSL`。
  云侧启用 SSL 会重启实例，客户端需要实例下载的 CA。启用、重启、CA 下载核验和
  Secret 更新必须在含 migration 的 push 前独立授权；本 workflow 不执行外部变更。
- 工作流继续使用 `contents: read`；数据库 Secret 只注入
  `Validate and apply Atlas migrations` step，再由 HCL 从容器环境读取。
- GitHub 日志、Docker 参数诊断和错误摘要不得回显 DSN、用户名、密码或 CA 内容。

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
  先在单独授权且不泄密的检查中运行或核对 `atlas migrate status`，区分网络瞬断与
  checksum、SQL、数据、schema drift 或部分执行。只有确认可重试的瞬态故障，才在
  同一个 Actions run 使用 `Re-run failed jobs`。
- 确定性 migration 失败必须修 migration 后重新审计；补数据、修 schema 或 revision
  等数据库处理必须单独授权。不要新建 dispatch 绕过失败的 migration push。
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
- 使用公共可信 CA 的 dummy `mysql://...?...tls=true` URL 可以执行 validate/apply；
  无 `tls`、`tls=false` 或非 MySQL scheme 均在 Docker 前失败。
- 私有 CA dummy Secret 会以 `600` 文件写入 runner temp、只读挂载到固定路径并在退出
  时删除；CA Secret 与 `ssl-ca=/atlas-ca.pem` 任一缺失或不匹配都在 Docker 前失败。
- `ATLAS_URL` 缺失或 Atlas 返回非零：不晋级、不调用 webhook。
- ACR 超时、认证失败、revision 不一致或 Git 基线不可用：
  `deployment_blocked=true`，`deployment-blocked` 明确失败，不得执行 Atlas。
- 两张 manifest 都不存在的首次 push：以 `before` 比较；含 migration 时自动 Atlas，
  不含时直接继续。
- 只有一张 manifest 缺失：保持阻断。
- `workflow_dispatch` 覆盖同 SHA 重试、祖先回滚、无 migration 前向重放，以及有
  migration 或关系不可证明时的阻断；任何分支都不执行 Atlas 或 down migration。
- Atlas apply 通过 HCL 的 `getenv("ATLAS_URL")` 取值，Docker argv 不包含 DSN 或
  PEM；`migrate` 继续使用 `ubuntu-latest`。
- `deploy` 使用 10 秒连接超时和 840 秒总请求窗口，job 上限为 15 分钟。
- Workflow、测试和文档不包含真实 DSN、密码、CA、AK/SK 或 webhook key。

真实端到端验收使用一次包含本设计和待执行 migration 的 `dev` push：

1. 第二次审计已从 ACR 读取双 `:dev` revision，按实际部署区间列出全部 migration
   和兼容性证据，并取得精确推送授权。
2. 腾讯云 SSL、重启窗口、CA 和受控网络已在独立授权下配置；实际
   `ubuntu-latest` runner 连通性验证已记录，且没有使用全网开放。
3. 两张 `dev-<sha>` 镜像构建并通过 OCI revision 验证。
4. Atlas job 显示 validate 和 apply 成功，日志不包含 DSN 或 CA 内容。
5. 两张 `:dev` 标签 revision 一致且等于目标 SHA。
6. 宝塔 webhook 成功，服务器 `/healthz` 返回目标 SHA。
7. 对象存储配置表存在，系统管理对象存储接口可正常使用。

## 9. 文档与门禁更新

实现时同步更新：

- `docs/superpowers/context/project-context.md`：dev push 会在确有 migration 时自动
  apply；TLS/CA、runner 网络、实际部署区间和重试授权都 fail closed。
- `docs/superpowers/runbooks/dev-integration-audit.md`：第二次审计报告必须明确列出
  ACR 双 revision、实际部署区间、全部 migration、副作用、旧应用兼容性、Secret
  合同和 runner 连通性；用户确认只授权该 exact SHA 和所列 dev migration。
- `deploy/dev/README.md`：记录 TLS/CA Secret、数据库网络、一次性 Atlas baseline、
  实际部署区间和分类重试，删除人工 migration hold 作为常规流程的说明。
- `docs/superpowers/specs/2026-07-30-dev-automated-deployment-design.md`：标明数据库
  迁移章节已由本设计取代，避免长期事实冲突。

## 10. 不在范围内

- 生产数据库 migration、生产部署和自动 down migration。
- 腾讯云 SSL 启用与实例重启、CA 下载、自动备份、白名单或数据库账号的创建与变更。
- 通用数据库发布平台、蓝绿数据库或跨环境 migration 编排。
- 绕过两阶段 `dev` 集成审计，或在未报告 migration 副作用时自动推送。
