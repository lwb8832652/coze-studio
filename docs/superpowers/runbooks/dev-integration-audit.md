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

### 4. 第二次报告和确认

报告必须列出合并前后 SHA、最终提交范围、重新运行的全部验证、工具影响分析、
远程竞态检查和剩余风险。报告后停止；只有用户第二次明确确认才可推送。

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
