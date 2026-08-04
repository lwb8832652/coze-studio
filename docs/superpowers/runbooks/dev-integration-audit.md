# dev 单次集成检查手册

## 默认路径

每个需求仍在独立 `codex/` 分支完成，但只做一次代码与范围审计：

1. 需求分支对齐最新 `origin/dev`，完成范围审核和必要验证；
2. 报告远程基准、目标 exact SHA、文件范围、测试结果、migration 清单和风险；
3. 用户确认后，将本地 `dev` fast-forward 到已审计的需求 SHA；
4. 本地 `dev` 与已验证分支是同一个 exact SHA，因此不重复第二轮测试或审计；
5. 展示合并结果和标准发布命令，获得发布确认后运行一次
   `deploy/dev/publish-dev.sh`。

合并确认与发布确认复用同一份审计证据，不再生成第二份审计报告。SHA、文件范围、
migration 清单或远程基准变化时，证据失效并重新执行单次检查。

普通发布不要求手动完成 ACR 登录、镜像拉取、revision 检查、GitHub 页面操作或
宝塔接口调用。发布脚本负责分支、干净工作区、exact SHA、远程竞态和 Atlas 状态
检查；GitHub Actions 负责镜像构建、校验、标签晋级和宝塔 WebHook。

## 禁止事项

- 禁止直接在 `dev` 开发需求；
- 禁止未经当前范围确认就合并、apply migration 或推送；
- 禁止直接运行 `git push origin dev` 绕过发布脚本；
- 禁止 force push、自动解决冲突或在本地 `dev` 补功能；
- 禁止把未提交、来源不明或未验证的改动合入 `dev`；
- 禁止在 SHA、文件范围、migration 清单或远程基准变化后复用旧确认；
- 禁止把 credential、DSN、token 或密码写入仓库、脚本、日志或报告。

## 单次集成检查

### 1. 固定工作区与远程基准

```bash
git status --short --branch
git worktree list --porcelain
git branch -vv
git remote -v
git fetch origin dev
git rev-parse origin/dev
git rev-parse dev
git rev-parse HEAD
git rev-list --left-right --count dev...origin/dev
```

记录 `origin/dev`、本地 `dev` 和需求分支的完整 SHA。本地 `dev` 必须干净且能够
fast-forward 到 `origin/dev`；若本地 `dev` 有未解释提交、工作区改动或双方分叉，
停止并报告。

### 2. 对齐需求分支

```bash
git merge-base --is-ancestor origin/dev HEAD
```

退出码不是 `0` 时，只能在需求分支吸收最新 `origin/dev` 并解决冲突：

```bash
git merge --no-edit origin/dev
```

冲突解决或基准变化后重新运行受影响验证。不得在本地 `dev` 解决冲突。

### 3. 审核提交与文件范围

```bash
git log --oneline --decorate origin/dev..HEAD
git diff --stat origin/dev...HEAD
git diff --name-status origin/dev...HEAD
git diff --check origin/dev...HEAD
git diff --check
```

检查无关文件、生成代码、锁文件、credential、调试日志、本地缓存和 migration。
涉及 migration 时，逐个列出 SQL 文件、数据库对象、forward apply 副作用、锁风险、
兼容性和不可逆操作；没有 migration 也要明确记录。

### 4. 验证需求

按变更风险执行：

- 前端：相关 Vitest，必要时 typecheck、lint、build 和 in-app browser；
- 后端：相关 Go package 测试，必要时跨包、race 和 build；
- 公共合同或权限：补未授权、跨空间、脱敏和兼容性验证；
- migration：Atlas hash、validate/status 和 SQL 审阅；
- 文档：链接、残留规则和 `git diff --check`；
- 结构影响：codebase-memory 或等价调用链分析。

### 5. 审计报告与合并确认

报告必须列出：

- `origin/dev` 基准 SHA、本地 `dev` SHA 和领先/落后关系；
- 需求分支名、目标 exact SHA 和提交列表；
- 文件范围、验证命令、退出状态和关键结果；
- migration 清单及副作用，或明确“无 migration”；
- 业务影响边界、已知风险和未验证项；
- 推送会触发的镜像构建、标签晋级和预发布部署。

报告后停止。用户明确确认后，只授权报告中 exact SHA 的本地 fast-forward 合并；
不自动授权 migration apply、远程推送、部署、baseline、repair、backfill、down
migration、生产发布、配置变更、人工回滚或服务器操作。

## 合入本地 dev

确认后先再次固定远程状态：

```bash
git fetch origin dev
git rev-parse origin/dev
git rev-parse <audited-feature-sha>
```

若 `origin/dev` 或需求 SHA 与报告不同，确认失效，回到单次集成检查。两者未变化时，
在干净的本地 `dev` worktree 执行：

```bash
git switch dev
git merge --ff-only origin/dev
git merge --ff-only <audited-feature-sha>
```

第二次 `--ff-only` 保证本地 `dev` 与已经验证的需求 SHA 完全一致，不产生新的 merge
commit。合并后只核对：

```bash
git rev-parse dev
git status --short
git diff --check origin/dev...dev
```

不再重跑同一 exact SHA 的测试、构建、浏览器验收或第二轮审计。任一 fast-forward
失败时立即停止，回需求分支处理；不能在 `dev` 上 rebase、补提交或解决冲突。

## 发布确认与执行

展示以下完整 SHA 和命令，等待用户对本次发布明确确认：

```bash
AUDITED_ORIGIN_DEV_SHA=<reported-origin-dev-sha>
AUDITED_TARGET_DEV_SHA=<audited-feature-sha>
deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
```

该确认只授权报告中已审阅的 migration forward apply、从基准到目标 SHA 的非 force
push，以及该 push 触发的标准镜像发布和预发布部署。确认不延伸到其他 SHA 或其他
数据库、生产、配置、回滚和服务器操作。

发布脚本会再次校验当前分支、干净工作区、目标 SHA、祖先关系、远程竞态和 Atlas
状态，再按顺序执行 validate、status、必要的 forward apply 和 exact push。任一步
失败都停止；不得改用直接 push、force 参数或未经确认的数据库修复命令。

若 migration apply 已成功但 push 失败，schema 可能领先于远程代码。保留两个 SHA
和脚本输出并报告，不自动重试 apply、push、baseline、repair 或回滚。

## 发布后核验

push 成功后不再执行第二次本地审计。只观察目标 SHA 对应的
`Publish and deploy dev images`，核对 `preflight`、前后端 build、`verify-images`、
`promote`、`deploy` 和服务健康。任何失败先报告；Actions 重跑、标签修复、人工回滚
或服务器操作需要根据当次证据另行确认。

## 严格排查模式

只有用户明确要求严格发布审计、migration 风险排查或发布异常定位时，才增加 ACR
revision、实际部署区间、Atlas credential 文件权限、数据库网络和部署链路等专项
检查。专项结果补充到同一份审计证据，不恢复合并后的第二轮重复测试。
