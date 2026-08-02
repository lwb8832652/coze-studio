# Dev First Deployment Automation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让远程 `dev` 的普通 push 在 ACR 尚无两张 `:dev` manifest 时也能自动完成镜像构建、校验、晋级和宝塔部署，同时继续阻断数据库迁移提交。

**Architecture:** 保留当前不可变 `dev-<full-sha>` 镜像和已晋级 revision 的日常迁移基线。只在 server、web 两张 `:dev` 镜像都明确返回 manifest 不存在时，使用已验证的 GitHub push `before` 作为一次性启动基线；其他拉取错误、单边缺失和 revision 异常仍然 fail closed。

**Tech Stack:** GitHub Actions YAML、Bash、Ruby YAML contract tests、Docker/Alibaba Cloud ACR、Baota webhook。

---

## File Map

- Modify: `deploy/dev/tests/workflow_contract_test.sh` - 覆盖首次双 manifest 缺失、迁移提交和 registry 故障三种语义。
- Modify: `.github/workflows/deploy-dev.yml` - 识别双 manifest 缺失并使用 push range 检查迁移。
- Modify: `deploy/dev/README.md` - 将首次发布步骤改为 push 自动部署，并记录启动前提。
- Modify: `docs/superpowers/specs/2026-07-30-dev-automated-deployment-design.md` - 固化新的长期发布合同。

### Task 1: Add Bootstrap Regression Contracts

**Files:**

- Modify: `deploy/dev/tests/workflow_contract_test.sh`

- [x] **Step 1: Add a fake Docker mode for missing manifests**

让测试命令替身在 `DOCKER_MODE=missing` 时，对两张 `docker pull` 都输出 ACR 实际使用的 `manifest unknown` 错误并返回非零；`DOCKER_MODE=registry-error` 返回不含 manifest 缺失标记的连接错误；默认模式继续返回已部署 revision。

- [x] **Step 2: Add the expected bootstrap assertions**

使用现有测试仓库中的 `BEFORE_REVISION..TARGET_REVISION` 普通变更运行 extracted preflight，断言：

```text
target_sha=<TARGET_REVISION>
migration_changed=false
```

再以 `DEPLOYED_REVISION..BEFORE_REVISION` 的迁移提交运行，断言 `migration_changed=true`；以 registry 故障运行普通变更，同样断言 `migration_changed=true`。

- [x] **Step 3: Run the RED test**

Run: `bash deploy/dev/tests/workflow_contract_test.sh`

Expected: FAIL with `manifest-missing bootstrap push did not continue deployment`, because the current workflow always holds when `docker pull` fails.

### Task 2: Implement First-Push Bootstrap

**Files:**

- Modify: `.github/workflows/deploy-dev.yml`
- Modify: `deploy/dev/README.md`

- [x] **Step 1: Capture both mutable-image pull outcomes**

在 preflight 中为两张 `:dev` 镜像分别保存 pull 状态和 stderr。只有状态都为零时才读取 OCI revision；临时文件通过 `trap` 清理。

- [x] **Step 2: Distinguish bootstrap from registry failure**

加入仅匹配 `manifest unknown` 或 manifest `not found` 的 Bash helper。两次 pull 都符合该错误时进入启动分支；只有一张缺失、认证失败、超时或其他 registry 错误继续保持 `migration_changed=true`。

- [x] **Step 3: Check the bootstrap push range**

启动分支执行：

```bash
git diff --quiet "$before" "$target_sha" -- docker/atlas/migrations
```

退出 `0` 时设置 `migration_changed=false`；退出 `1` 表示存在迁移并保持 hold；其他退出值记录比较失败并保持 hold。

- [x] **Step 4: Update the operator runbook**

把首次部署改为普通 push 自动构建、校验、晋级和 webhook。明确远程数据库必须已匹配 push 前的 `dev`，迁移提交仍需手工 apply 后使用 `workflow_dispatch`。

- [x] **Step 5: Run the GREEN test**

Run: `bash deploy/dev/tests/workflow_contract_test.sh`

Expected: `workflow contract: passed` and `workflow semantic contract: passed`.

- [x] **Step 6: Commit the focused fix**

```bash
git add .github/workflows/deploy-dev.yml deploy/dev/tests/workflow_contract_test.sh deploy/dev/README.md docs/superpowers/specs/2026-07-30-dev-automated-deployment-design.md docs/superpowers/plans/2026-08-01-dev-first-deployment-automation.md
git commit -m "fix: automate initial dev image promotion"
```

### Task 3: Verify And Audit The Deployment Contract

**Files:**

- Verify only; no additional files expected.

- [x] **Step 1: Run all deployment contracts**

```bash
bash deploy/dev/tests/workflow_contract_test.sh
bash deploy/dev/tests/image_contract_test.sh
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/deploy_test.sh
```

Expected: all commands exit `0`; deploy tests report `17 passed`.

- [x] **Step 2: Run static validation**

```bash
bash -n deploy/dev/deploy.sh
git diff --check origin/dev...HEAD
git diff --check
```

Expected: all commands exit `0` with no whitespace errors.

- [ ] **Step 3: Perform the first dev integration audit**

Fix `origin/dev`, local `dev`, feature SHA, commit list and changed-file scope; confirm the feature branch contains no migrations, credentials, database access, production deployment, or unrelated changes. Stop for the required first user confirmation before merging local `dev`.
