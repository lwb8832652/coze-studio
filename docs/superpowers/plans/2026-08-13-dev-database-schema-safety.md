# Dev Database Schema Safety Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让普通本地启动永不自动执行 migration/schema DDL，并用可读的迁移命名、发布策略检查和结构漂移门禁阻止共享 dev 数据库再次被旧快照破坏。

**Architecture:** `docker/atlas/migrations` 是唯一结构事实源；本地迁移只能通过固定连接 `mysql:3306` 的显式 Compose profile 执行，远程 dev 迁移只能通过 `deploy/dev/publish-dev.sh` 执行。新增迁移使用“时间版本 + 类型 + 模块 + 动作”命名，发布脚本只检查新增文件并拒绝篡改历史迁移、普通发布中的破坏性迁移以及结构漂移。

**Tech Stack:** POSIX shell、Docker Compose、Atlas Community 1.2.3、MySQL 8.4、Git、Markdown runbook。

---

## 文件边界

- `scripts/database/check-migration-policy.sh`：只负责检查 Git 区间内 migration 的路径、名称、类型和危险 SQL。
- `scripts/database/tests/check-migration-policy-test.sh`：在临时 Git 仓库验证迁移策略，不访问真实数据库。
- `scripts/database/tests/local-database-safety-test.sh`：静态检查三个 Compose 文件和 Makefile 的启动边界。
- `docker/docker-compose-debug.yml`：共享 dev Debug 与显式本地 MySQL 模式；普通 middleware 不依赖迁移。
- `docker/docker-compose-oceanbase_debug.yml`：OceanBase Debug 的 MySQL 元数据边界；普通 middleware 不依赖迁移。
- `docker/docker-compose.yml`：容器化本地运行；MySQL 本身不再执行声明式 schema apply。
- `Makefile`：公开 `db_local_up`、`db_local_migrate` 等显式入口，停用危险的 `sync_db` 快捷路径。
- `deploy/dev/check-schema-drift.sh`：在一次性 MySQL 中重放 migrations，并只读比较目标数据库结构。
- `deploy/dev/tests/schema_drift_test.sh`：用 fake Docker 验证漂移门禁 fail-closed 和清理行为。
- `deploy/dev/publish-dev.sh`、`deploy/dev/tests/publish_dev_test.sh`：接入迁移策略与发布前后漂移检查。
- `docs/superpowers/runbooks/project-operations.md`：统一项目运维入口。
- `AGENTS.md`、现有专题 runbook：链接统一入口并消除冲突。

### Task 1: 新增 migration 命名与内容策略

**Files:**
- Create: `scripts/database/check-migration-policy.sh`
- Create: `scripts/database/tests/check-migration-policy-test.sh`

- [ ] **Step 1: 编写失败测试**

测试在临时 Git 仓库创建以下新增文件并断言结果：

```text
20260813103000_expand_plugin_code_create_tables.sql   -> 允许
20260813104500_data_space_backfill_allow_develop.sql -> 允许
20260814100000_contract_legacy_drop_columns.sql      -> 普通发布拒绝
20260814103000_repair_notification_restore_indexes.sql -> 普通发布拒绝
20260813_add_table.sql                               -> 命名拒绝
20260813110000_expand_space_drop_column.sql           -> 内容与类型不符，拒绝
```

测试还要断言修改或删除已有 migration 会失败，而既有历史文件名不会被重新校验。

- [ ] **Step 2: 运行测试并确认 RED**

Run:

```bash
bash scripts/database/tests/check-migration-policy-test.sh
```

Expected: FAIL，原因是 `scripts/database/check-migration-policy.sh` 尚不存在。

- [ ] **Step 3: 实现最小策略检查器**

脚本接口固定为：

```bash
scripts/database/check-migration-policy.sh <base-sha> <target-sha> [--allow-special]
```

新增 migration 必须匹配：

```regex
^[0-9]{14}_(expand|data|contract|repair)_[a-z0-9]+(_[a-z0-9]+)+\.sql$
```

规则：

- `expand`：允许新增表、列、索引等向后兼容 DDL；出现 `DROP`、`TRUNCATE`、`RENAME` 即失败。
- `data`：用于幂等回填或数据转换；出现 `DROP TABLE`、`DROP COLUMN`、`TRUNCATE` 即失败。
- `contract`：删除或收窄合同，只在 `--allow-special` 下通过。
- `repair`：事故修复，只在 `--allow-special` 下通过。
- Git 区间内修改、重命名或删除已有 migration 一律失败。
- 旧文件不改名，只有 `git diff --diff-filter=A` 识别出的新增文件受新命名规则约束。

- [ ] **Step 4: 运行测试并确认 GREEN**

Run:

```bash
bash scripts/database/tests/check-migration-policy-test.sh
```

Expected: PASS，且输出不包含 SQL 内容或 credential。

- [ ] **Step 5: 提交迁移策略**

```bash
git add scripts/database/check-migration-policy.sh \
  scripts/database/tests/check-migration-policy-test.sh
git commit -m "feat: enforce safe migration policy"
```

### Task 2: 普通启动零 migration/schema DDL 与显式本地迁移

**Files:**
- Create: `scripts/database/tests/local-database-safety-test.sh`
- Modify: `docker/docker-compose-debug.yml`
- Modify: `docker/docker-compose-oceanbase_debug.yml`
- Modify: `docker/docker-compose.yml`
- Modify: `Makefile`
- Modify: `scripts/setup/db_migrate_apply.sh`

- [ ] **Step 1: 编写失败合同测试**

测试必须断言：

```text
middleware/run-server profile 不包含 mysql-setup-schema 或 mysql-setup-init-sql
redis/server 不依赖任何 schema/init 服务
MySQL entrypoint 不下载 Atlas、不执行 schema apply
显式 local-db-migrate 服务只连接 mysql:3306
普通启动路径不包含 schema apply --auto-approve
Makefile 的 sync_db 明确失败并指向 db_local_migrate
```

- [ ] **Step 2: 运行测试并确认 RED**

Run:

```bash
bash scripts/database/tests/local-database-safety-test.sh
```

Expected: FAIL，并命中现有 `mysql-setup-schema`、Redis 依赖和 MySQL 自定义 entrypoint。

- [ ] **Step 3: 最小化 Compose 迁移入口**

三个 Compose 文件统一增加只在 `local-db-migrate` profile 下可见的一次性服务：

```yaml
mysql-migrate-local:
  image: arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e
  profiles: ['local-db-migrate']
  depends_on:
    mysql:
      condition: service_healthy
  volumes:
    - ./atlas/migrations:/migrations:ro
  command:
    - migrate
    - apply
    - --url
    - mysql://coze:coze123@mysql:3306/opencoze
    - --dir
    - file:///migrations
```

实际 URL 使用专门的本地 Compose credential，但 host 和 port 必须是字面量
`mysql:3306`，不得读取 `MYSQL_HOST`、`MYSQL_PORT` 或 `ATLAS_URL`。移除 MySQL
entrypoint 的 Atlas 下载/声明式 apply，并移除 middleware 对 schema/init 服务的依赖。

- [ ] **Step 4: 收敛 Makefile 与旧脚本**

公开命令：

```make
db_local_up:
	@docker compose -f $(COMPOSE_FILE) --env-file $(ENV_FILE) --profile local-mysql up -d mysql --wait

db_local_migrate: db_local_up
	@docker compose -f $(COMPOSE_FILE) --env-file $(ENV_FILE) --profile local-db-migrate run --rm mysql-migrate-local

sync_db:
	@echo "sync_db 已停用；本地请运行 make db_local_migrate，远程 dev 请走发布脚本。"
	@exit 1
```

`scripts/setup/db_migrate_apply.sh` 同样 fail closed，不再接受任意环境 DSN 执行
`schema apply`。

- [ ] **Step 5: 运行测试并确认 GREEN**

Run:

```bash
bash scripts/database/tests/local-database-safety-test.sh
docker compose -f docker/docker-compose-debug.yml --env-file docker/.env.debug config --profiles
docker compose -f docker/docker-compose-oceanbase_debug.yml --env-file docker/.env.debug config --profiles
docker compose -f docker/docker-compose.yml --env-file docker/.env.example config --profiles
```

Expected: 合同测试 PASS，三个 Compose 配置解析成功，profile 列表包含显式本地数据库入口。

- [ ] **Step 6: 提交本地启动保护**

```bash
git add Makefile scripts/setup/db_migrate_apply.sh \
  scripts/database/tests/local-database-safety-test.sh \
  docker/docker-compose-debug.yml docker/docker-compose-oceanbase_debug.yml \
  docker/docker-compose.yml
git commit -m "fix: isolate local database migrations"
```

### Task 3: dev 发布接入策略检查

**Files:**
- Modify: `deploy/dev/publish-dev.sh`
- Modify: `deploy/dev/tests/publish_dev_test.sh`

- [ ] **Step 1: 增加失败测试**

在现有 fake Git/Docker 测试中增加：新增非法名称、修改历史 migration、普通发布新增
`contract`、`expand` 内含 `DROP COLUMN` 四种场景；每个场景必须在 Atlas apply 和
`git push` 前退出。

- [ ] **Step 2: 运行测试并确认 RED**

Run:

```bash
bash deploy/dev/tests/publish_dev_test.sh
```

Expected: 新增场景 FAIL，因为发布脚本尚未调用 migration policy checker。

- [ ] **Step 3: 接入检查器**

在凭据读取和 Atlas 运行前调用：

```bash
"$repo_root/scripts/database/check-migration-policy.sh" \
  "$expected_origin_dev_sha" "$target_dev_sha"
```

普通发布不传 `--allow-special`。检查失败时保留明确但不包含 SQL 全文的错误摘要。

- [ ] **Step 4: 运行测试并确认 GREEN**

Run:

```bash
bash deploy/dev/tests/publish_dev_test.sh
```

Expected: 全部场景 PASS，危险 migration 场景的 fake Docker 日志中没有 apply/push。

- [ ] **Step 5: 提交发布策略接入**

```bash
git add deploy/dev/publish-dev.sh deploy/dev/tests/publish_dev_test.sh
git commit -m "fix: gate dev migration changes"
```

### Task 4: 发布前后 schema drift 门禁

**Files:**
- Create: `deploy/dev/check-schema-drift.sh`
- Create: `deploy/dev/tests/schema_drift_test.sh`
- Modify: `deploy/dev/publish-dev.sh`
- Modify: `deploy/dev/tests/publish_dev_test.sh`

- [ ] **Step 1: 编写失败测试**

fake Docker 覆盖：临时 MySQL 重放成功且 diff 为空、diff 非空、inspect 失败、重放失败、
清理失败。任何不确定结果必须返回非零；所有退出路径都要清理仅由本脚本创建的临时
容器和网络。

- [ ] **Step 2: 运行测试并确认 RED**

Run:

```bash
bash deploy/dev/tests/schema_drift_test.sh
```

Expected: FAIL，原因是漂移脚本不存在。

- [ ] **Step 3: 实现只读漂移检查**

脚本接受 exact migration 快照、受保护的 `ATLAS_URL` env 文件和期望版本。它创建唯一
命名的临时网络与 MySQL 容器，用固定 Atlas 镜像在临时库重放 migrations，然后执行：

```text
远程真实 schema --只读 schema diff--> 临时重放得到的期望 schema
```

排除 `atlas_schema_revisions` 和资源库运行时动态表 `table_*`，不执行远程
`schema apply`、baseline 或 repair。diff
非空、命令失败、输出无法识别都必须失败；输出必须复用发布脚本的 DSN 脱敏逻辑。

- [ ] **Step 4: 在 publish 前后调用**

发布顺序固定为：

```text
validate -> status -> pre-apply drift -> migrate apply -> post-apply drift -> exact push
```

`--status` 只执行 validate、status 和 pre-apply drift，不执行 apply/push。无待执行
migration 时，pre-apply drift 直接与目标 SHA 全量结构比较；存在待执行 migration 时，
先与当前 revision 对应结构比较，apply 后再与目标结构比较。

- [ ] **Step 5: 运行测试并确认 GREEN**

Run:

```bash
bash deploy/dev/tests/schema_drift_test.sh
bash deploy/dev/tests/publish_dev_test.sh
```

Expected: PASS；漂移和检查失败场景均在 push 前停止。

- [ ] **Step 6: 提交漂移门禁**

```bash
git add deploy/dev/check-schema-drift.sh deploy/dev/tests/schema_drift_test.sh \
  deploy/dev/publish-dev.sh deploy/dev/tests/publish_dev_test.sh
git commit -m "fix: block dev schema drift"
```

### Task 5: 统一运维手册和 Agent 入口

**Files:**
- Create: `docs/superpowers/runbooks/project-operations.md`
- Create: `scripts/database/tests/project-operations-doc-test.sh`
- Modify: `AGENTS.md`
- Modify: `docs/superpowers/runbooks/local-debug-and-test.md`
- Modify: `docs/superpowers/runbooks/dev-integration-audit.md`
- Modify: `deploy/dev/README.md`

- [ ] **Step 1: 编写失败文档合同测试**

断言根 `AGENTS.md` 要求相关任务先读统一手册，且统一手册包含以下稳定标题：

```text
运行模式选择
默认本地开发（共享 dev 数据）
隔离本地数据库
配置文件与密钥
数据库 migration 命名
dev 合并与发布
备份、PITR 与事故恢复
禁止事项
```

- [ ] **Step 2: 运行测试并确认 RED**

Run:

```bash
bash scripts/database/tests/project-operations-doc-test.sh
```

Expected: FAIL，因为统一手册尚不存在。

- [ ] **Step 3: 编写统一手册**

手册明确：

- 共享 dev 模式不启动 MySQL/schema/init 容器，只使用受限业务账号；
- 隔离模式先 `make db_local_up`，再 `make db_local_migrate`，最后启动应用；
- 迁移文件示例采用 `20260813103000_expand_plugin_code_create_tables.sql`；
- `expand`、`data`、`contract`、`repair` 的语义和发布边界；
- 日常不使用“全量 migration”，全量只用于全新基线或经批准的 checkpoint；
- dev 发布只使用 exact-SHA 发布脚本；
- 数据丢失只通过备份/PITR 恢复，禁止用 schema 重建冒充数据恢复；
- 禁止远程 `schema apply --auto-approve`、root Debug、修改历史 migration、手工 revision。

- [ ] **Step 4: 接入 AGENTS 和专题文档**

根 `AGENTS.md` 的开工顺序加入统一手册，并规定项目启动、配置、Compose、数据库、迁移、
dev 合并和发布任务必须先完整阅读。专题 runbook 和 `deploy/dev/README.md` 在顶部链接
统一入口，不复制相互冲突的流程。

- [ ] **Step 5: 运行测试并确认 GREEN**

Run:

```bash
bash scripts/database/tests/project-operations-doc-test.sh
git diff --check
```

Expected: PASS，文档无空白错误。

- [ ] **Step 6: 提交运维文档**

```bash
git add AGENTS.md docs/superpowers/runbooks/project-operations.md \
  docs/superpowers/runbooks/local-debug-and-test.md \
  docs/superpowers/runbooks/dev-integration-audit.md deploy/dev/README.md \
  scripts/database/tests/project-operations-doc-test.sh
git commit -m "docs: add unified project operations runbook"
```

### Task 6: 集成验证与影响检查

**Files:**
- Modify only if verification exposes a defect in files already listed above.

- [ ] **Step 1: 运行全部专项测试**

```bash
bash scripts/database/tests/check-migration-policy-test.sh
bash scripts/database/tests/local-database-safety-test.sh
bash scripts/database/tests/project-operations-doc-test.sh
bash deploy/dev/tests/schema_drift_test.sh
bash deploy/dev/tests/publish_dev_test.sh
bash deploy/dev/tests/compose_contract_test.sh
bash deploy/dev/tests/workflow_contract_test.sh
```

Expected: 全部 PASS。

- [ ] **Step 2: 验证 Atlas migration 完整性**

```bash
docker run --rm \
  -v "$PWD/docker/atlas/migrations:/migrations:ro" \
  arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e \
  migrate validate --dir file:///migrations
```

Expected: migration directory valid。

- [ ] **Step 3: 验证 Compose 展开与 profile 隔离**

```bash
docker compose -f docker/docker-compose-debug.yml --env-file docker/.env.debug config --services
docker compose -f docker/docker-compose-debug.yml --env-file docker/.env.debug --profile middleware config --services
docker compose -f docker/docker-compose-debug.yml --env-file docker/.env.debug --profile local-db-migrate config --services
```

Expected: `middleware` 结果不含 `mysql-migrate-local`；显式迁移 profile 才包含该服务。

- [ ] **Step 4: 检查 diff 与影响面**

```bash
git diff --check origin/dev...HEAD
git status --short
git log --oneline origin/dev..HEAD
```

使用 codebase-memory `detect_changes`（可用时）核对启动脚本、Compose 和发布脚本影响，
再回真实 diff 确认没有业务代码或远程 credential 变更。

- [ ] **Step 5: 报告并等待集成授权**

报告 exact SHA、提交、文件范围、验证结果、无远程数据库写入事实和剩余风险。合并、
远程推送、migration apply、PITR/恢复和数据库权限变更仍需用户针对当前范围单独确认。
