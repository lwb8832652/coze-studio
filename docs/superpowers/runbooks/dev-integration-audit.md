# dev 集成双重审计手册

## 目的

每个需求必须先在独立 `codex/` 分支完成。需求分支通过第一次审计并获得用户
确认后，才能合入本地 `dev`；合并后的本地 `dev` 必须接受更严格的第二次审计，
再次获得用户确认后才能推送 `origin/dev`。

两次确认只对报告中列出的分支、SHA、文件范围和验证结果有效。提交、远程基准
或范围变化后，原确认失效。

## 禁止事项

- 禁止直接在 `dev` 开发需求；
- 禁止把第一次确认解释为远程推送授权；
- 禁止 Codex 直接运行 `git push origin dev` 或等价 refspec 绕过发布脚本；
- 禁止 force push、自动合并或自动解决冲突；
- 禁止在本地 `dev` 上试合并、修冲突或补功能；
- 禁止合并未提交、未审计或来源不明的 worktree 改动；
- 禁止复用需求分支测试结果冒充合并后验证。

## 审计前提

执行前确认：

```bash
git status --short --branch
git worktree list --porcelain
git branch -vv
git remote -v
```

若需求相关 worktree 不干净、`dev` 被未知 worktree 占用、远程配置异常或存在
无法归属的相关改动，停止并向用户报告。

## 第一次审计：需求分支

### 1. 固定远程基准

```bash
git fetch origin dev
git rev-parse origin/dev
git rev-parse dev
git rev-parse HEAD
git rev-list --left-right --count dev...origin/dev
```

记录三个 SHA 和领先/落后计数。本地 `dev` 必须能够 fast-forward 到
`origin/dev`；若本地 `dev` 含远程没有的提交或双方分叉，停止审计。

### 2. 在需求分支吸收最新基准

```bash
git merge-base --is-ancestor origin/dev HEAD
```

若退出码不是 `0`，在需求分支执行：

```bash
git merge --no-edit origin/dev
```

冲突只能在需求分支解决。解决后重新运行完整需求验证；不得在第一次审计期间
切到或修改本地 `dev`。

### 3. 审核提交和文件范围

```bash
git log --oneline --decorate origin/dev..HEAD
git diff --stat origin/dev...HEAD
git diff --name-status origin/dev...HEAD
git diff --check origin/dev...HEAD
git diff --check
```

检查无关文件、生成代码、锁文件、迁移、credential、调试日志和本地缓存。

### 4. 验证需求

按变更类型执行：

- 前端：相关 Vitest，必要时 typecheck、lint、build 和 in-app browser；
- 后端：相关 Go package 测试，必要时跨包测试和 build；
- 迁移：Atlas hash 与 validate；
- 文档：链接、残留规则、`git diff --check` 和工具可用性；
- 所有需求：codebase-memory `detect_changes` 或等价影响分析。

### 5. 第一次报告和确认

报告必须列出：

- `origin/dev` 基准 SHA；
- 本地 `dev` SHA 和领先/落后关系；
- 需求分支与审计 SHA；
- 提交列表和文件范围；
- 验证命令、退出状态和关键结果；
- codebase-memory 影响面；
- Graphify 验证状态（适用时）；
- 已知风险、未验证项和停止条件。

报告后停止。只有用户明确确认合入本地 `dev` 才能进入下一阶段。

## 合入本地 dev

用户第一次确认后，先重新 fetch 并确认远程 SHA 未变化：

```bash
git fetch origin dev
git rev-parse origin/dev
```

若 SHA 与第一次报告不同，确认失效，回到第一次审计。

在 `dev` 所在 worktree 确认工作区干净后执行：

```bash
git switch dev
git merge --ff-only origin/dev
git merge --no-ff --no-edit <audited-feature-sha>
```

合并对象必须是第一次报告中的审计 SHA，并再次确认需求分支仍指向该 SHA。
发生冲突时立即执行 `git merge --abort`，回到需求分支吸收最新 `origin/dev`；
不得直接在 `dev` 解决冲突。

## 第二次审计：本地 dev

### 1. 固定合并结果

```bash
git rev-parse origin/dev
git rev-parse dev
git log --oneline --decorate origin/dev..dev
git diff --stat origin/dev...dev
git diff --name-status origin/dev...dev
git diff --check origin/dev...dev
```

确认只包含已批准需求提交和预期 merge commit。

### 2. 严格重新验证

所有相关测试必须从本地 `dev` 重新执行。第二次审计至少包含第一次全部命令，
并按风险增加：

- 共享前端包：消费方测试、typecheck 或 build；
- 公共后端合同：跨 application/api/router package 测试；
- 权限与租户：未授权、跨空间和脱敏路径；
- 页面：真实 URL、账号/空间、核心交互和控制台；
- 迁移：hash、validate、顺序和兼容性；
- 文档与上下文：链接、残留扫描、Graphify 增量结果；
- 全部变更：codebase-memory 最终影响和反向依赖。

同时扫描：

```bash
rg -n '^(<<<<<<<|=======|>>>>>>>)' . --glob '!graphify-out/**' --glob '!.git/**'
git status --short
```

### 3. 远程竞态检查

```bash
git fetch origin dev
git rev-parse origin/dev
git rev-list --left-right --count origin/dev...dev
```

远程 SHA 必须仍等于第一次审计基准，且本地 `dev` 只能领先预期提交。否则停止，
不得自动 pull、rebase、push 或覆盖本地 `dev`。记录当前 merge SHA 并向用户报告；
只有获得明确恢复授权后，才能将本地 `dev` 重新对齐新的 `origin/dev`，随后回到
需求分支重新执行第一次审计。

### 4. 固定实际部署区间和 migration 授权

第二次报告前，把远程竞态检查得到的 `origin/dev` 完整 SHA 记为
`AUDITED_ORIGIN_DEV_SHA`，把本地 `dev` 完整 SHA 记为
`AUDITED_TARGET_DEV_SHA`。使用已审计的只读 ACR 凭据分别拉取当前
`coze-server:dev`、`coze-web:dev`，读取 `org.opencontainers.image.revision`。
两个 revision 必须都是合法的 40 位完整 SHA，并且完全一致。不能使用本地缓存标签
代替本次 ACR 读取。

若两张 `:dev` manifest 都明确不存在，只有在首次部署条件已经验证时，才能把远程
竞态检查固定的 push `before` 作为比较基线。只有一张 manifest 缺失、ACR 认证或
网络错误、revision 缺失或不一致时立即停止，不得请求推送确认。

把一致的 `<deployed-revision>`，或首次部署的 `<verified-push-before>`，记为
`comparison_base`；把本地 `dev` 完整 SHA 记为 `target_sha`。然后验证实际部署区间：

```bash
git cat-file -e "${comparison_base}^{commit}"
git cat-file -e "${target_sha}^{commit}"
git merge-base --is-ancestor "$comparison_base" "$target_sha"
git diff --name-status "$comparison_base" "$target_sha" -- docker/atlas/migrations
```

记录每条命令的退出状态。Git 对象、祖先关系或 diff 任一项无法取得可信结果时，
禁止进入确认阶段。Migration 清单必须来自
`<deployed-revision>..<target-sha>`；首次部署来自
`<verified-push-before>..<target-sha>`。不得只列 `origin/dev...dev` 中新增的文件，
也不得漏掉区间内修改、删除或较早提交引入的 migration。

对区间内每个待执行 migration 逐项审阅 SQL，报告数据库对象、数据、锁和可逆性等
forward apply 副作用，并给出旧应用在镜像晋级前继续访问迁移后 schema 的兼容性
证据。授权只覆盖清单中的文件和副作用。数据库 schema、已执行 migration 与 Atlas
revision 必须有一致性证据；需要 baseline 时停止常规审计，baseline 写 revision 的
操作另行申请数据库变更授权，`--baseline` 使用 migration 文件名的版本时间戳而非
Git SHA。

检查本机 Atlas credential 文件。默认路径是
`~/.config/coze-studio/dev-atlas.env`，可由 `ATLAS_ENV_FILE` 指定其他路径。它必须
存在、是普通文件而不是 symlink、物理路径位于仓库外，且模式严格为 `600`。文件只
允许注释、空行和一条 `ATLAS_URL=mysql://...`。审计报告只记录物理路径和各项检查
结果，禁止记录、打印或转述文件内容。

第二次确认前只允许读取 Atlas 状态。先设置报告中的两个具体 SHA，再运行安全的只读
模式：

```bash
: "${AUDITED_ORIGIN_DEV_SHA:?set from the second audit evidence}"
: "${AUDITED_TARGET_DEV_SHA:?set from the second audit evidence}"
deploy/dev/publish-dev.sh --status \
  "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
```

`--status` 会复用正式发布的 env 校验、exact target 快照、固定 Atlas 镜像、安全临时
输出和 credential 脱敏，只执行 validate/status，再次核对本地与远程 SHA 后退出；它
不会 apply 或 push。若状态无法读取、checksum/revision/schema 基线与审阅结果不一致，
或需要 baseline、repair、backfill，立即停止；第二次确认前禁止运行 `migrate apply`、
`migrate down`、baseline 或修复命令。

当前 dev MySQL 可以使用无 TLS 连接，但公网链路会暴露数据库 credential 和 schema
流量。审计必须确认安全组或数据库白名单只允许受控来源，并使用专用最小权限
migration 账号，禁止 root。无法证明网络限制或账号权限时，不得请求包含数据库变更
的第二次确认。

### 5. 第二次报告和确认

报告必须列出合并前后 SHA、最终提交范围、重新运行的全部验证、工具影响分析、
远程竞态检查和剩余风险。

仓库启用 dev 自动发布后，第二次报告还必须明确列出：

- 具体的 `AUDITED_ORIGIN_DEV_SHA` 与 `AUDITED_TARGET_DEV_SHA`，均为 40 位完整 SHA；
- 从 ACR 读取的两张当前 `:dev` OCI revision、合法性和一致性证据；首次部署则列出
  两张 manifest 均不存在的证据和已验证 push `before`；
- `comparison_base`、实际部署区间、Git 对象和祖先关系，以及该区间内的全部 migration；
  没有 migration 时也要明确写出；
- 每个待执行 migration 对远程 dev 数据库的 forward apply 副作用，以及与发布前旧
  应用兼容的证据；
- 本地 env 文件的物理路径、普通文件/非 symlink/仓库外/模式 `600` 检查结果，以及
  pinned Atlas `migrate status` 摘要；报告不得包含 credential 或 DSN；
- dev MySQL 的 TLS 现状、受控来源网络证据和最小权限 migration 账号检查结果；
- 推送将触发的 ACR 前后端镜像构建、不可变标签、`dev` 标签晋级条件；
- 宝塔 webhook 对 dev/预发布服务器的自动更新副作用；
- workflow 对该 SHA 的成功路径：`preflight -> build-server/build-web ->
  verify-images -> promote -> deploy`。GitHub 不连接数据库；预发布服务器保留应用
  运行时 DSN，但不持有 migration credential，也不执行 Atlas。

报告后停止。用户第二次明确确认只授权报告逐项列出的本地 Atlas forward apply、
从 `AUDITED_ORIGIN_DEV_SHA` 到 `AUDITED_TARGET_DEV_SHA` 的 exact-SHA 非 force push，
以及该 push 触发的 ACR 双镜像晋级和宝塔预发布部署。授权不延伸到其他 SHA，也不
包含 baseline、repair、backfill、down migration、数据库重试、生产发布、配置变更、
人工回滚或其他服务器操作。

若 `origin/dev` 在任一审计或等待确认期间变化，当前确认失效。回到需求分支吸收
新基准，并从第一次审计重新执行，不能只补一次远程竞态检查后继续推送。

## 推送与核验

用户第二次确认后，只执行：

```bash
: "${AUDITED_ORIGIN_DEV_SHA:?set from the second audit report}"
: "${AUDITED_TARGET_DEV_SHA:?set from the second audit report}"
deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
```

脚本会再次核对分支、干净工作区、两个 SHA、祖先关系和远程竞态，再从 exact target
快照依次执行 validate、status、forward apply 和 exact push。任一步失败都立即停止。
不得改用直接 push，也不得追加 force 参数。

若 apply 已成功，但第二次远程检查或 push 失败，dev schema 可能领先于远程代码。
保留并报告两个审计 SHA 与脚本结果，不再 push、不自动重试数据库操作；远程恢复后
从第一次审计重新开始并取得新的两次确认。

push 成功后脚本立即结束。本地不再调用 `gh`、GitHub API、curl、dispatch 或服务器
命令。只观察目标 SHA 对应的 `Publish and deploy dev images` run，依次核对
`preflight`、两项 build、`verify-images`、`promote`、`deploy`，再核对两张 `:dev`
revision、宝塔结果和服务健康。`deployment-blocked` 出现时，后续晋级和部署不得继续。

任何 post-push 失败都先报告。初始确认不授权 Actions 重跑、dispatch、标签修复、
人工回滚或服务器操作；这些动作必须根据当次证据另行确认。GitHub 侧没有 Atlas job，
不得用远程重跑代替新的数据库审计或授权。
