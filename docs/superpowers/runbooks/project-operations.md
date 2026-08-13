# 项目运维手册

本手册是 Coze Studio 本地启动、环境配置、数据库更新和 `dev` 发布的统一入口。
开始操作前先选择一种运行模式。不要把不同模式的数据库配置、容器或凭据混用。

专题细节：

- 本地账号、功能开关和页面验收：
  `docs/superpowers/runbooks/local-debug-and-test.md`
- `dev` 分支审计、合并与 exact-SHA 发布：
  `docs/superpowers/runbooks/dev-integration-audit.md`
- dev 服务器、ACR、宝塔和容器运维：`deploy/dev/README.md`
- Sandbox 控制面：
  `docs/superpowers/runbooks/sandbox-control-plane-operations.md`

## 运行模式选择

| 模式 | 数据库 | 需要启动的容器 | DDL 入口 | 适用场景 |
| --- | --- | --- | --- | --- |
| 默认本地开发 | 共享 dev MySQL | 只启动本机缺少的非数据库依赖 | 无 | 日常缺陷修复和页面回归 |
| 隔离本地数据库 | Compose 内 `mysql:3306` | `mysql`，按需启动其他依赖 | `make db_local_migrate` | migration 开发、空库重放、破坏性本地实验 |
| 远程 dev 发布 | 共享 dev MySQL | 本机不启动数据库服务 | `deploy/dev/publish-dev.sh` | 已审计代码合入和标准 dev 发布 |

选择规则：

- 页面和业务缺陷回归使用默认本地开发。
- 修改 `docker/atlas/migrations` 或验证数据库结构时使用隔离本地数据库。
- 共享 dev 的结构变更只走远程 dev 发布。普通本地启动不执行 DDL。
- 任务需要切换模式时，先停止当前模式，核对 `MYSQL_HOST`、`MYSQL_PORT` 和账号，
  再启动下一种模式。

## 默认本地开发（共享 dev 数据）

### 前置条件

1. 从 `docker/.env.debug.example` 创建 ignored 的 `docker/.env.debug`。
2. 配置共享 dev 的 MySQL、Redis、Elasticsearch 和对象存储地址。
3. MySQL 使用受限应用账号。应用账号不得拥有 DDL 权限，包括 `CREATE`、`ALTER`、
   `DROP` 和索引管理权限。
4. `docker/.env.debug` 不得包含 `ATLAS_URL`、迁移账号或 root 凭据。
5. 用 `git check-ignore docker/.env.debug` 确认文件不会被提交，并执行
   `chmod 600 docker/.env.debug`。

真实 endpoint、账号和密钥只写入 ignored 文件或本机密钥管理工具。tracked 示例文件
只能保留占位值。

### 启动

共享 dev 已提供全部基础设施时，不运行 `make middleware`。分别启动后端和前端：

```bash
# 终端 1
make server

# 终端 2
cd frontend/apps/coze-studio
WEB_SERVER_PORT=8888 rushx dev
```

`make server` 会以 `APP_ENV=debug` 构建并运行后端。前端开发服务器把 `/api` 和
`/v1` 转发到本机 `8888` 后端。默认验收地址与账号见
`docs/superpowers/runbooks/local-debug-and-test.md`。

只有某项共享依赖不可用且任务需要本地替代时，才启动对应的 Debug Compose profile。
`make middleware` 会启动一组本地非数据库依赖，内存占用较高；2C4G 环境应按需启动
单个服务，避免同时运行未使用的 Elasticsearch、MinIO、etcd 或 Milvus。

### 健康检查与停止

```bash
curl --fail --silent --show-error http://127.0.0.1:8888/healthz
```

后端和前端在各自终端用 `Ctrl-C` 停止。本地 Compose 依赖使用 `make down` 停止。
不要用 `make clean` 作为日常停止命令；它会删除 `docker/data` 下的本地数据。

启动失败时先检查进程日志、`/healthz` 和依赖连通性。不要通过启动本地 schema 服务、
切换 root 账号或修改 Atlas revision 解决连接问题。

## 隔离本地数据库

该模式只操作 Compose 内的 MySQL。迁移服务把目标固定为 `mysql:3306/opencoze`，不读取
共享 dev 的 `MYSQL_HOST`、`MYSQL_PORT` 或迁移 DSN。

### 启动与迁移

```bash
make db_local_up
make db_local_migrate
```

第一条命令只启动 MySQL，不建表。第二条命令通过固定 digest 的 Atlas Community
镜像按 `docker/atlas/migrations` 重放版本化 migration。再次运行应成为 no-op；若
Atlas 报 checksum、revision 或 SQL 错误，停止并修复 migration，不要 baseline。

启动应用前，把 ignored `docker/.env.debug` 的应用连接改为本地值：

```bash
export MYSQL_DATABASE=opencoze
export MYSQL_USER=coze
export MYSQL_PASSWORD=coze123
export MYSQL_HOST=127.0.0.1
export MYSQL_PORT=3306
export MYSQL_DSN="${MYSQL_USER}:${MYSQL_PASSWORD}@tcp(${MYSQL_HOST}:${MYSQL_PORT})/${MYSQL_DATABASE}?charset=utf8mb4&parseTime=True"
```

这些是本地 Compose 默认值，不得复制到共享 dev 或生产。完成 migration 验证后再运行
`make server` 和前端开发服务器。

### 停止与重建

```bash
make down
```

需要丢弃本地数据库时，先确认目标只在 `docker/data/mysql`，再单独申请删除授权。
不要用递归清理命令处理仓库根目录，也不要把本地重建当作共享 dev 的恢复方案。

## 配置文件与密钥

| 文件或事实源 | 是否 tracked | 内容 | 权限与变更方式 |
| --- | --- | --- | --- |
| `docker/.env.debug.example`、`docker/.env.example` | 是 | 占位值和默认开关 | 不写真实 endpoint 或 secret |
| `docker/.env.debug` | 否 | 本机 Debug 应用配置 | `600`；修改后重启后端 |
| `bin/.env.debug` | 否 | `make server` 生成的运行副本 | 不手工维护，不提交 |
| `~/.config/coze-studio/dev-atlas.env` | 否，且在仓库外 | 一条 dev migration DSN | 普通文件、非 symlink、严格 `600` |
| `/opt/coze-dev/deploy.env` | 服务器本地 | 镜像和部署参数 | 部署账号所有，`600` |
| `/opt/coze-dev/app.env` | 服务器本地 | 应用依赖和运行密钥 | 部署账号所有，`600`；不得含迁移 DSN |
| 数据库配置中心 | 数据库 | 模型、对象存储、站点和运行开关 | 按页面合同更新；需要时重启后端 |

应用账号和 migration 账号必须分离。应用账号不得拥有 DDL 权限；migration 账号只由
发布脚本读取，不写入应用 env、GitHub Secrets、日志或测试夹具。用数据库的
`SHOW GRANTS` 结果核对权限，但报告中不要输出账号、主机或 DSN。

对象存储和 Sandbox 加密 key 必须跨重启稳定。更换 key 属于密钥轮换任务，需要先验证
旧数据可解密和回滚路径。不要用随机临时 key 启动共享环境。

## 数据库 migration 命名

`docker/atlas/migrations` 是数据库结构的唯一事实源。新增文件格式固定为：

```text
YYYYMMDDHHMMSS_{expand|data|contract|repair}_module_action.sql
```

时间使用 14 位 UTC 版本号，文件名使用小写 snake_case。例如：

```text
20260813103000_expand_plugin_code_create_tables.sql
20260813104500_data_space_backfill_allow_develop.sql
20260814100000_contract_legacy_drop_columns.sql
20260814103000_repair_notification_restore_indexes.sql
```

类型含义：

- `expand`：向后兼容的表、列或索引新增，允许普通发布。
- `data`：有边界的数据回填或转换，要求可审计、可恢复，并控制批量和锁影响。
- `contract`：删除表、列，收窄类型或移除旧合同。普通发布拒绝，必须单独审批兼容
  窗口、备份和回滚。
- `repair`：事故后的结构或数据修复。普通发布拒绝，必须基于事故证据单独审批。

所有日常文件都是增量 migration。全量只用于新基线或经批准的 checkpoint，不作为
普通发布类型。仓库中的 `opencoze_latest_schema.hcl` 是遗留快照，不是执行入口。

### 创建和校验

1. 用 `date -u +%Y%m%d%H%M%S` 生成版本号，确保大于仓库当前最大版本。
2. 一个文件只处理一个可说明、可审阅的目的。
3. 保持新旧应用同时可运行；新增字段先允许旧代码继续读写。
4. 运行固定镜像更新 hash：

   ```bash
   make atlas-hash
   ```

5. 验证完整迁移目录：

   ```bash
   ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
   docker run --rm \
     -v "$PWD/docker/atlas/migrations:/migrations:ro" \
     "$ATLAS_IMAGE" migrate validate --dir file:///migrations
   unset ATLAS_IMAGE
   ```

6. 用隔离本地数据库从空库重放，并再次执行确认 no-op。
7. 提交后运行策略检查：

   ```bash
   scripts/database/check-migration-policy.sh origin/dev HEAD
   ```

不要手工编辑 `atlas.sum`，不要修改、重命名或删除已提交 migration。普通发布不接受
`contract`、`repair`，也不接受 `expand` 中的破坏性 SQL。

## 数据库更新流程

本地结构更新只运行 `make db_local_migrate`。共享 dev 的更新只运行
`deploy/dev/publish-dev.sh`，由脚本从目标 exact SHA 创建只读 migration 快照。

发布脚本按以下顺序执行：

```text
migration policy -> validate -> status -> pre-apply drift
-> forward migrate -> post-apply drift -> exact-SHA push
```

漂移检查在一次性 MySQL 中重放期望版本，再只读比较共享 dev 的真实结构。缺表、缺列、
多余结构、无法解析输出、临时库失败或清理失败都会阻断发布。脚本不会自动 repair、
baseline 或修改 revision。

如果 migration apply 成功而后续检查或 push 失败，数据库可能领先于代码。记录当前
revision、目标 SHA 和安全摘要后停止。重新 apply、repair、回滚或 push 都需要基于
新证据再次确认。

## dev 合并与发布

完整步骤以 `docs/superpowers/runbooks/dev-integration-audit.md` 为准。固定流程：

1. 在独立 `codex/` 分支对齐最新 `origin/dev`，完成测试和一次集成审计。
2. 报告 `origin/dev`、目标 exact SHA、文件范围和 migration 清单。
3. 获得合并确认后，把本地 `dev` fast-forward 到已审计 SHA。
4. 在干净的本地 `dev` 上执行只读预检：

   ```bash
   AUDITED_ORIGIN_DEV_SHA=<reported-origin-dev-sha>
   AUDITED_TARGET_DEV_SHA=<audited-target-dev-sha>
   deploy/dev/publish-dev.sh --status \
     "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
   ```

5. 报告实际部署 revision、migration 区间、credential 文件权限、Atlas status 和
   schema drift 结果，获得发布确认。
6. 只执行一次标准发布：

   ```bash
   deploy/dev/publish-dev.sh "$AUDITED_ORIGIN_DEV_SHA" "$AUDITED_TARGET_DEV_SHA"
   ```

脚本只推送报告中的 exact SHA。不要直接 `git push origin dev`，不要 force push。
SHA、文件范围、migration 清单、实际部署基准或远程基准变化后，旧确认失效。

## dev 服务器运维

服务器只运行 `nsqd`、`coze-server` 和 `coze-web`；MySQL、Redis、Elasticsearch 和
对象存储使用远程服务。服务器不安装 Atlas，也不持有 migration credential。

部署文件、健康检查、ACR、宝塔、NSQ 和应用回滚见 `deploy/dev/README.md`。日常检查：

```bash
cd /opt/coze-dev
docker compose --env-file deploy.env -f docker-compose.yml ps
docker compose --env-file deploy.env -f docker-compose.yml logs --tail=200 \
  nsqd coze-server coze-web
```

不要在服务器执行数据库 baseline、repair 或 down migration。代码回滚前必须确认旧代码
兼容当前数据库结构；镜像回滚不会撤销数据库变更。

## 备份、PITR 与事故恢复

缺表或数据丢失时先停止所有可能写入 schema 的本地任务和发布任务。数据恢复与结构
修复是两类操作：重建表结构不能恢复被删除的行，补写 Atlas revision 也不能恢复数据。

恢复顺序：

1. 固定事故时间窗、当前数据库实例、Atlas revision 和应用 revision。
2. 核对云数据库自动备份与 PITR 可用点，保留审计证据。
3. 在新实例或新库恢复，不直接覆盖共享 dev 现库。
4. 比对 migration revision、表和关键列，再核对关键业务表行数与抽样记录。
5. 处理事故后产生的有效写入，完成管理员页面和核心任务回归。
6. 提交切换方案、回滚点和停机窗口，获得单独确认后切换。

没有可用备份时，只能单独审批结构重建和可重建数据范围。报告必须写明哪些历史数据
无法恢复。PITR、数据合并、repair migration、revision 修改和数据库切换均不属于普通
代码合并或发布授权。

## 禁止事项

- 禁止普通启动、容器 entrypoint 或服务依赖隐式执行 DDL、初始化 SQL 或 migration。
- 禁止让本地 Debug 应用连接共享 dev 的 root 或 migration 账号。
- 禁止从共享数据库反向生成可执行 schema 快照。
- 禁止对共享 dev 使用 Atlas 声明式结构同步、自动批准、baseline 或 revision 手工修改。
- 禁止修改、重命名或删除历史 migration，禁止手工编辑 `atlas.sum`。
- 禁止把真实密码、DSN、API Key、对象存储密钥或加密 key 写入 tracked 文件和日志。
- 禁止在未确认 exact SHA、migration 清单和数据库影响时合并、apply 或推送。
- 禁止用 `make clean`、卷删除或递归清理命令处理共享数据。
- 禁止把结构重建描述为数据恢复，也禁止在没有备份证据时承诺可恢复历史数据。

任何命令目标、权限、revision、输出或清理结果无法确认时，停止操作并保留现场证据。
