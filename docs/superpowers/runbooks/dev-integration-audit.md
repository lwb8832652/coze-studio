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

第二次报告前，使用已审计的只读 ACR 凭据分别拉取当前
`coze-server:dev`、`coze-web:dev`，读取
`org.opencontainers.image.revision`。两个 revision 必须都是合法的 40 位完整 SHA，
并且完全一致。不能使用本地缓存标签代替本次 ACR 读取。

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

区间含 migration 时，还要核对以下 Secret 合同，不读取或记录 Secret 值：

| Repository Secret | 要求 |
| --- | --- |
| `ATLAS_URL` | 必须是 `mysql://` URL，且 query 中有唯一的 `tls=true` |
| `ATLAS_CA_PEM`（可选） | 配置私有 CA 时必须同时让 URL 精确声明 `ssl-ca=/atlas-ca.pem`；无 Secret 时 URL 不得声明 `ssl-ca` |

每次第二次审计都必须对本轮目标 dev MySQL 端点重新执行 TLS 探测，并把结果写入
报告，不能复用历史任务的瞬时结论。若端点不支持 SSL，云侧启用 SSL、实例重启窗口、
CA 下载核验和 Secret 更新必须先取得独立授权；这些外部配置完成并验证前，不得请求
包含 migration 的推送确认。

`migrate` 当前实际配置必须仍是 `runs-on: ubuntu-latest`。第二次报告记录 workflow
runner label、本次连通性验证所用的实际 Actions run/runner，以及 dev 数据库受控
网络策略的验证结果。标准 GitHub-hosted runner 出口范围多且变化，不建议把整段
GitHub 地址加入数据库白名单，`0.0.0.0/0` 不能作为连通性证据。需要稳定白名单时，
只能评估已经配置的 self-hosted runner 或具有静态出口 IP 的 larger runner，并在
修改 `runs-on` 前另做安全审计。无 migration 时明确记录本次 workflow 不连接数据库；
有 migration 但无法从实际 runner 安全连通 dev 数据库时停止审计。

### 5. 第二次报告和确认

报告必须列出合并前后 SHA、最终提交范围、重新运行的全部验证、工具影响分析、
远程竞态检查和剩余风险。

仓库启用 dev 自动发布后，第二次报告还必须明确列出：

- 目标远程分支 `origin/dev` 和待推送的完整目标 SHA；
- 从 ACR 读取的两张当前 `:dev` OCI revision、合法性和一致性证据；首次部署则列出
  两张 manifest 均不存在的证据和已验证 push `before`；
- 实际比较区间、Git 对象和祖先关系，以及该区间内 workflow 会识别的全部 migration；
  没有 migration 时也要明确写出；
- 每个待执行 migration 对远程 dev 数据库的 forward apply 副作用，以及与发布前旧
  应用兼容的证据；
- Atlas Secret 合同、远程 Atlas revision 与 schema 一致性；报告不得包含 Secret、
  DSN 或 CA 内容；
- 当前 `ubuntu-latest` 配置、本次实际 runner 和连通性验证；不得把全网开放作为证据；
- 推送将触发的 ACR 前后端镜像构建、不可变标签、`dev` 标签晋级条件；
- 宝塔 webhook 对 dev/预发布服务器的自动更新副作用；
- workflow 对该 SHA 的 job 顺序，以及 migration 存在时自动 validate/apply、失败时
  不晋级、不部署的行为。

报告后停止。用户第二次明确确认只授权报告中 exact SHA 的 `origin/dev` 推送，
以及该 push 触发的 dev Atlas forward apply、ACR 双镜像晋级和宝塔预发布部署。
授权范围只包含报告逐项列出的 migration 和数据库副作用，不延伸到其他 SHA。
该确认不授权生产发布、down migration、手工数据库操作、备份策略或配置变更、
人工回滚及其他服务器操作。

若 `origin/dev` 在任一审计或等待确认期间变化，当前确认失效。回到需求分支吸收
新基准，并从第一次审计重新执行，不能只补一次远程竞态检查后继续推送。

## 推送与核验

用户第二次确认后执行：

```bash
git push origin dev
git ls-remote --heads origin dev
git rev-parse dev
```

远程 SHA 必须与本地 `dev` 一致。推送被拒绝时不得 force push，也不得自动覆盖
本地 `dev`；保留并报告本地 merge SHA，按用户明确授权恢复基准后从第一次审计
开始。

推送成功后找到该目标 SHA 对应的 `Publish and deploy dev images` Actions run，
记录 run URL 和最终状态，并按顺序核对 `preflight`、两项 build、`verify-images`、
`migrate`、`promote` 和 `deploy`。有 migration 时确认 `migrate` 已 validate/apply；
无 migration 时确认它以 no-op 成功。随后核对两张 `:dev` 标签、宝塔调用、服务
revision，并确认日志未回显 `ATLAS_URL` 的 DSN 值。Preflight 阻断时应看到
`deployment-blocked` 明确失败，数据库、晋级和部署 job 均未继续。

任何失败都先报告，不自动 dispatch、手工改库或修服务器。`migrate` 失败后，先取得
单独授权，在不泄露 DSN 的前提下检查 `atlas migrate status`、Actions 日志和数据库
状态，区分网络瞬断与 checksum、SQL、数据、schema drift 或部分执行。只有证据确认
为可重试的瞬态故障，才可在同一个 Actions run 使用 `Re-run failed jobs`；确定性
失败必须修 migration 后重新审计，数据库修复必须单独授权。

`promote` 部分晋级时，获得恢复授权后仍在同一个 run 重跑失败 job，不得用 dispatch
绕过双 revision 不一致。两张标签已经晋级而 `deploy` 失败时，获得授权后可以
dispatch 同一 SHA 重试部署。初始推送确认不自动授权 status 检查、重试、数据库修复
或服务器操作。
