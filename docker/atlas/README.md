# Atlas migration directory

项目启动、配置分层、数据库更新和 `dev` 发布的统一规则见
`docs/superpowers/runbooks/project-operations.md`。本目录只保存 Atlas 配置与版本化
migration，不提供共享数据库的直接变更入口。

## 事实源

- `migrations/*.sql`：数据库结构和受审计数据变更的唯一事实源。
- `migrations/atlas.sum`：Atlas 生成的完整性校验文件，不得手工编辑。
- `atlas.hcl`：隔离本地数据库的 Atlas 配置。
- `opencoze_latest_schema.hcl`：遗留快照，不参与启动、迁移或发布。

新增 migration 使用：

```text
YYYYMMDDHHMMSS_{expand|data|contract|repair}_module_action.sql
```

示例：

```text
20260813103000_expand_plugin_code_create_tables.sql
```

类型、兼容规则、审批边界和完整创建流程以统一运维手册为准。

## Hash 与校验

在仓库根目录运行：

```bash
make atlas-hash

ATLAS_IMAGE='arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e'
docker run --rm \
  -v "$PWD/docker/atlas/migrations:/migrations:ro" \
  "$ATLAS_IMAGE" migrate validate --dir file:///migrations
unset ATLAS_IMAGE
```

## 本地重放

```bash
make db_local_up
make db_local_migrate
```

迁移服务只连接 Compose 内的 `mysql:3306`。普通本地启动不执行 DDL，共享 `dev`
只允许 `deploy/dev/publish-dev.sh` 使用仓库外 migration credential 执行 forward
migration。修改历史 migration、baseline、repair 和破坏性变更都需要按统一手册停止并
单独审批。
