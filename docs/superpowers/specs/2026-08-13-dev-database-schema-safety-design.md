# dev 数据库结构安全设计

## 背景与事故结论

2026-08-13 本地 Debug 启动执行了 `docker-compose-debug.yml` 的
`mysql-setup-schema` 服务。该服务从 `.env.debug` 读取远程 dev 数据库地址，并用
过期的 `docker/atlas/opencoze_latest_schema.hcl` 执行声明式
`atlas schema apply --auto-approve`。Atlas 按旧快照删除了当前数据库中的表、字段和
索引，同时因为命令排除了 `atlas_schema_revisions`，迁移账本仍显示最新版本。
后续增量发布因此无法发现或补回已经被删除的历史结构。

本设计解决两个问题：阻止普通本地启动修改远程数据库结构；阻止结构与迁移账本不一致
的数据库继续发布。现有数据恢复不属于本设计，必须依据备份或 PITR 作为独立运维变更。

本次同时建立统一项目运维手册，消除本地调试、Compose、配置、数据库和 dev 发布文档
之间的口径冲突。后续 Agent 和维护者必须先从统一手册选择运行模式，再执行对应命令。

## 方案比较

### 方案 A：只更新 schema 快照

让 `opencoze_latest_schema.hcl` 始终与 migrations 同步，可以降低旧快照误删概率，但仍
保留普通启动对数据库执行声明式 DDL 的能力。任何生成延迟、分支切换或错误环境变量都
可能再次导致删除，因此不采用。

### 方案 B：只禁止远程地址

在 Compose 脚本中检查 `MYSQL_HOST` 是否为 localhost，可以阻止当前事故路径，但域名、
Docker 网关、端口转发和错误覆盖仍可能绕过字符串判断，也无法发现数据库已经发生的
schema drift，因此只作为辅助保护。

### 方案 C：职责隔离与多层 fail-closed（采用）

普通应用启动不执行 DDL；本地数据库初始化只允许连接 Compose 内的固定 MySQL 服务；
远程 dev 只允许发布脚本执行增量 migrations；发布在推送前验证真实结构与 migrations
重放结果一致。再配合最小权限数据库账号，即使某一层配置错误，其他层仍能阻止破坏。

## 架构边界

### 1. 普通本地启动零 DDL

- 从 `middleware` 和 `run-server` profile 移除 `mysql-setup-schema`。
- 普通 `make debug`、服务端启动和 middleware 启动不得隐式执行 `schema apply`、
  `migrate apply`、baseline、repair 或初始化 SQL。
- `opencoze_latest_schema.hcl` 不再作为普通启动的可执行目标；如果保留，只用于只读检查
  或开发辅助，不能挂载到自动启动服务。

### 2. 本地数据库显式初始化

- 新增独立、一次性的本地数据库迁移 profile/命令，目标主机固定为 Compose 服务名
  `mysql`，目标端口固定为 `3306`。
- 该命令只使用 `docker/atlas/migrations` 和 `atlas migrate apply`，不使用声明式
  `schema apply --auto-approve`。
- 本地初始化命令拒绝外部主机、外部端口和通过 `.env.debug` 覆盖目标地址。
- 是否启动本地 MySQL由显式 profile 决定；使用远程 dev 数据时不启动任何数据库初始化
  容器。

### 3. 远程 dev 只允许受审计的增量迁移

- 远程 dev DDL 入口保持为 `deploy/dev/publish-dev.sh`。
- 发布脚本继续从 exact target SHA 创建 migrations 快照，并执行 Atlas validate、status
  和 forward apply。
- 应用运行凭据与 Atlas 迁移凭据分离。应用账号不得拥有 `CREATE`、`ALTER`、`DROP`、
  `INDEX` 等 DDL 权限；迁移凭据仅保存在仓库外的 `dev-atlas.env` 中。
- Debug 配置不得使用远程 root/管理员账号。缺少受限应用账号时应阻断共享 dev 启动，
  不能自动退回高权限账号。

### 4. 发布前后 schema 一致性证明

- 在一次性临时 MySQL 中从空库重放目标 SHA 的全部 migrations，得到期望结构。
- 对远程 dev 执行只读 schema inspect/diff，不对远程执行声明式 apply。
- 增量迁移前若只读 diff 发现未由待执行 migration 解释的删除性差异，立即阻断。
- 增量迁移后再次比较；只有真实结构与期望结构零差异时才允许 push。
- Atlas revision 已到最新但真实结构存在缺表、缺字段或多余结构时，发布必须失败，并
  明确报告为 schema drift。发布脚本不得自动 baseline、repair 或重建历史结构。

### 5. 破坏性变化策略

- 普通发布默认禁止 `DROP TABLE`、`DROP COLUMN` 和可能丢数据的类型收窄。
- 确需破坏性迁移时，必须作为独立数据库变更，列明目标对象、备份、回滚和兼容窗口，
  获得单独确认后执行；普通发布确认不能覆盖它。
- 所有 schema 工具默认 fail closed。无法连接、无法 inspect、diff 无法解析或临时重放
  失败时均停止发布。

### 6. 统一运维手册与 Agent 入口

- 新增 `docs/superpowers/runbooks/project-operations.md`，作为项目启动、停止、配置调整、
  数据库更新和 dev 发布的统一权威入口。
- 手册明确区分三种互斥运行模式：
  1. 默认本地开发：本地运行前后端，连接共享 dev 基础设施，不启动任何数据库初始化
     或 schema 服务；
  2. 隔离本地数据库：显式启动本地 MySQL，并只对 Compose 内 `mysql:3306` 重放增量
     migrations；
  3. 远程 dev 发布：在独立需求分支完成验证，经审计和确认后由
     `deploy/dev/publish-dev.sh` 执行增量迁移与 exact-SHA 推送。
- 手册为每种模式列出：适用场景、前置检查、配置文件、启动顺序、健康检查、停止方式、
  数据库权限、禁止命令和异常处理。
- 配置章节区分 tracked 示例、ignored 本机配置、仓库外迁移凭据和服务器本地配置；说明
  每类配置的事实源、权限模式、修改后的重启要求，以及禁止保存的真实密钥。
- 数据库章节覆盖 migration 创建、hash、validate、空库重放、只读 status/drift 检查、
  dev apply、备份/PITR 恢复和紧急停止条件。手册不得提供绕过授权的 repair、baseline、
  回填或直接远程 `schema apply` 快捷命令。
- `local-debug-and-test.md`、`dev-integration-audit.md`、`deploy/dev/README.md` 继续承载各自
  专题细节，但必须由统一手册路由，且与统一手册不存在冲突。
- 根 `AGENTS.md` 增加统一手册入口，并规定任何涉及项目启动、停止、环境配置、Compose、
  数据库、迁移、dev 合并或发布的任务都必须先完整阅读该手册。

## 测试与验收

### 自动化合同测试

- Compose 测试断言 `middleware`、`run-server` 不包含任何 schema/init 服务。
- Compose 测试断言本地迁移目标固定为 `mysql:3306`，且不读取远程 `MYSQL_HOST`。
- 静态测试禁止普通启动路径出现 `schema apply --auto-approve`。
- 发布脚本测试覆盖：零 drift 通过、缺表阻断、缺字段阻断、revision 最新但结构不一致
  阻断、检查失败阻断、未授权破坏性差异阻断。
- 文档合同测试检查根 `AGENTS.md` 指向统一运维手册，并检查统一手册包含三种运行模式、
  配置边界、数据库更新、dev 发布、恢复和禁止事项。

### 本地验收

- 使用远程 dev 业务配置启动 middleware，确认没有 Atlas/MySQL schema 容器被创建。
- 使用本地 MySQL profile 从空库执行 migrations，确认结构完整且第二次执行为 no-op。
- 使用只读测试库制造缺表，确认发布预检在任何 push 之前失败。

### 远程 dev 验收

- 恢复完成后核对 Atlas revision、表清单、关键字段和关键业务数据数量。
- 使用管理员账号验证工作空间菜单、代码插件草稿、模型管理、公告、计划任务和对象存储
  等受影响功能。
- 验收日志不得包含 DSN、密码、API Key 或对象存储密钥。

## 恢复边界

本次事故日志已经证明存在真实 `DROP TABLE` 和 `DROP COLUMN`。恢复顺序必须是：

1. 停止所有可能写入远程 dev schema 的本地和服务器任务；
2. 确认云数据库备份/PITR 可用时间点；
3. 优先在新实例或新库恢复并比对，不直接覆盖现库；
4. 合并事故后新增的有效数据；
5. 切换前再次校验 schema、迁移账本和业务数据；
6. 没有备份时，单独确认结构重建范围，并明确历史数据不可恢复。

任何恢复、迁移 repair、revision 修改、结构重建和数据回填都需要单独授权，不由本设计
的代码修改授权自动覆盖。
