# dev 数据库事故结构恢复设计

## 决策

本次恢复把共享 dev 数据库结构修到 migration 版本 `20260812000100` 所定义的状态，
随后通过三个新增 `repair` migration 把恢复过程纳入 Atlas 账本。恢复只补结构和系统运行
所需的确定性默认记录，不尝试重建已经删除的业务记录、账号权限、模型配置或用户设置。

代码实现和远程执行分成两次授权：

1. 本次先提交 repair migration、事故清单、门禁、测试和运维文档。
2. 远程 apply 只能在代码合入本地 `dev` 后，针对 exact SHA、三个 repair 文件和当前
   漂移指纹再次确认。普通发布确认不包含远程 repair 权限。

本设计补充
[`2026-08-13-dev-database-schema-safety-design.md`](./2026-08-13-dev-database-schema-safety-design.md)，
不改变普通本地启动零 migration DDL、应用账号与 migration 账号分离、普通发布遇到
漂移即停止的现有边界。

## 当前证据

2026-08-13 的只读检查得到以下结果：

- Atlas 账本状态为 `OK`，当前版本是 `20260812000100`，待执行 migration 数量为零。
- 真实结构与该版本从空库重放得到的结构存在 66 个 Atlas 语义变化。
- 生成的恢复方向 SQL 包含 54 个 `CREATE TABLE` 语句和 11 个 `ALTER TABLE` 语句，
  没有 `DROP TABLE`、`DROP COLUMN`、`TRUNCATE`、`DELETE` 或重命名。
- SQL 顶层语句数量与 Atlas 语义变化数量不一一对应，不能把两组数字相加后解释为
  遗漏。
- 已确认的受影响范围包含插件代码草稿、通知、对象存储、计划任务、计费、Journal、
  Agent 执行以及 Sandbox 等系统表；部分现存表缺少字段或索引。

检查只读取远程结构，没有执行 DDL、修改 Atlas revision 或保留含连接凭据的临时文件。
实现前必须重新读取当前状态。远程版本、漂移数量或漂移指纹发生变化时，现有证据失效。

## 方案比较

### 三个可重入 repair migration（采用）

按“缺失表、现存表结构、必要默认记录”拆成三个版本化 migration。每个文件承担一种
失败模式，便于验证部分执行、定位锁影响和审计数据写入。事故专用门禁只允许匹配已提交
事故清单的数据库进入 apply。

### 单个全量修复 SQL

一个文件能减少版本数量，但会把 54 张表、现存表变更和默认数据混在同一执行单元。
MySQL DDL 会隐式提交，执行中断后难以判断完成边界，也难以证明重跑不会覆盖配置。
本次不采用。

### 直接执行 Atlas 生成的 schema diff

Atlas diff 可以生成本次恢复方向，但直接执行会绕过仓库 migration、命名策略、事故
清单和 exact-SHA 发布门禁。远程状态改变后，重新生成的 SQL 也可能扩大范围。本次
只把 Atlas diff 用作只读证据和修复后验证，不把它作为执行入口。

## Migration 设计

新增文件沿用现有命名合同，版本必须晚于仓库当前最大版本：

```text
YYYYMMDDHHMMSS_repair_schema_restore_missing_tables.sql
YYYYMMDDHHMMSS_repair_schema_restore_columns_indexes.sql
YYYYMMDDHHMMSS_repair_system_restore_required_defaults.sql
```

实现时再固定三个互不重复的 UTC 版本号，并重新生成 `atlas.sum`。已经提交的历史
migration 保持不变。

### 1. 恢复缺失表

- 在一次性 MySQL 中从空库重放到 `20260812000100`，从该数据库提取缺失表的最终
  `CREATE TABLE` 定义，不从过期的 `opencoze_latest_schema.hcl` 复制。
- 按外键依赖顺序创建 54 张缺失表，使用 `CREATE TABLE IF NOT EXISTS`。
- 创建语句必须包含目标版本中的列、默认值、主键、索引、约束、引擎和字符集。
- 文件不得创建、修改或删除资源库运行时拥有的 `table_*`。
- 若远程已经出现同名表，事故门禁会因漂移指纹变化停止，不依靠
  `IF NOT EXISTS` 静默接受未知结构。

### 2. 恢复字段、索引和约束

- 只处理事故清单列出的现存表。一个 `ALTER TABLE` 可以包含多个 Atlas 语义变化，
  语句数不能代替表数量或对象数量。
- 每项变更先查询 `information_schema`，仅在目标字段、索引或约束缺失时执行对应
  `ALTER TABLE`。
- 已存在但定义不同的对象视为事故清单不匹配，预检必须停止；repair SQL 不覆盖、不
  重命名，也不缩窄现有定义。
- 使用仓库已有的存储过程和条件 DDL 风格，过程名称包含 migration 版本，执行结束后
  删除临时过程。
- 实现审计逐项记录 MySQL DDL 算法、预期锁和失败后的续跑方式。

### 3. 恢复必要默认记录

结构恢复不会自动重放旧 migration 中的 `INSERT`。实现必须从历史 migration、当前
服务启动路径和测试中列出系统运行必需的确定性默认记录，并将清单写入事故 manifest。

- 有稳定唯一键的记录使用 `INSERT IGNORE`。
- 单例记录使用 `INSERT ... SELECT ... WHERE NOT EXISTS`。
- 已存在记录保持原值，repair 不执行覆盖式 upsert。
- 不创建管理员身份，不恢复用户角色、模型 API Key、对象存储密钥、站点配置或用户
  业务数据。
- 无法证明为系统启动必需的记录不进入 repair，由管理员在功能页面重新配置。

## 事故 Manifest 与精确门禁

仓库新增受版本控制的事故 manifest，例如：

```text
deploy/dev/schema-repairs/20260813-dev-schema-loss.json
```

manifest 只保存非敏感结构证据：事故标识、基线 migration 版本、三个 repair 文件、
预期 Atlas 变化数、规范化 diff 的 SHA-256、允许的操作计数、缺失对象名称和允许插入的
默认记录键。它不保存 SQL 原文、DSN、账号、密码或远程地址。

发布脚本只能从 target SHA 的只读 Git archive 加载 manifest、migration 和 Atlas 配置，
不能读取工作区同名文件。manifest 必须记录自身格式版本；解析器拒绝未知字段、重复
对象、绝对路径、父目录跳转、符号链接和不在 target SHA 中的 repair 文件。

实现为发布脚本增加两种显式模式：

```text
deploy/dev/publish-dev.sh --repair-status <incident-id> <origin-dev-sha> <target-dev-sha>
deploy/dev/publish-dev.sh --repair <incident-id> <origin-dev-sha> <target-dev-sha>
```

`--repair-status` 只读检查，不 apply、不 push。`--repair` 执行相同预检，通过后运行
三个 repair migration、执行零漂移验证，再推送同一个 exact SHA。两个模式都要求：

1. 当前分支为本地 `dev`，工作区干净，`HEAD` 等于 target SHA。
2. fetch 得到的 `origin/dev` 等于明确传入的 origin SHA，且它是 target 的祖先。
3. migration policy 以 `--allow-special` 检查，pending migration 与 manifest 中三个
   文件完全一致，不允许夹带 `expand`、`data` 或 `contract`。
4. 远程 Atlas current 等于 manifest 基线，pending 数量等于三个 repair migration。
5. 使用固定 Atlas 镜像，对“远程当前结构到基线期望结构”的 diff 做规范化；变化数、
   操作类型、对象清单和 SHA-256 必须全部匹配 manifest。
6. diff 中出现删除、截断、重命名、未知对象或解析失败时立即停止。
7. 执行人已经停止其他 schema 任务并获得本轮维护窗口；备份/PITR 可用性已记录。没有
   可用恢复点时，用户必须在远程 repair 授权中接受无数据级回滚，脚本不能把结构可重入
   描述为数据可恢复。

规范化逻辑只接受固定 Atlas 版本产生的 SQL，去除注释和无语义空白后按完整语句计算
摘要，同时独立解析操作类型和对象名称。只比较 hash 不足以授权执行。

普通 `release` 和 `--status` 模式继续调用普通 migration policy，不传
`--allow-special`，因此仍会拒绝 `repair` migration。普通模式也继续要求 apply 前零
漂移，不能借用事故 manifest 绕过门禁。

## 执行顺序与失败处理

事故模式按以下顺序执行：

```text
exact SHA 与凭据检查
-> Atlas validate/status
-> 基线漂移与事故 manifest 精确匹配
-> 执行前再次读取并匹配同一漂移指纹
-> 三个 forward repair migration
-> target 版本零漂移
-> 再次固定本地与 origin SHA
-> exact-SHA push
```

MySQL DDL 可能隐式提交，三个 migration 也可能只完成一部分。任一步失败后脚本停止，
不得自动修改 revision 或继续 push。维护者先读取 Atlas status 和只读结构证据；因网络
中断需要续跑时，仍要针对新的 current、pending 和漂移结果单独确认。条件 DDL 保证
已完成部分不会因重跑而被覆盖，但可重入不等于自动重试授权。

repair 只增加缺失对象和默认记录，不提供 down migration。出现错误时优先从事故前
备份或 PITR 恢复到新库；若继续在现库修复，必须提交新的 forward repair。

## 测试矩阵

### 静态合同

- 三个文件名符合 migration 命名规则，版本唯一且晚于当前最大版本。
- repair 文件不包含 `DROP`、`TRUNCATE`、`DELETE`、`RENAME`、覆盖式 upsert 或
  `table_*` 目标。
- 普通 migration policy 和普通发布拒绝 repair；事故模式才传 `--allow-special`。
- incident ID 只能解析到仓库内固定目录的普通 JSON 文件，拒绝路径穿越和符号链接。
- 输出和临时文件不包含 Atlas URL、用户信息、密码或其他密钥。

### 一次性 MySQL

1. **健康库重放**：从空库重放全部 migration，包括三个 repair，最终 schema 与目标
   版本零漂移。
2. **健康库重入**：在同一一次性数据库再次执行三个 repair SQL，结构和默认记录不变。
3. **事故库恢复**：先重放到基线，再仅在一次性数据库按 manifest 制造缺失对象；Atlas
   apply 三个 repair 后必须零漂移。
4. **部分执行续跑**：分别制造第一、第二个 repair 已完成的状态，继续执行不得报重复
   对象或覆盖记录。
5. **默认记录保护**：预置不同的合法系统配置值，repair 后值保持不变；缺失的确定性
   默认记录只新增一次。
6. **额外漂移阻断**：在事故 fixture 上再制造一个 manifest 外差异，
   `--repair-status` 必须失败且不执行 apply。

制造事故 fixture 的破坏性 SQL 只能连接测试脚本刚创建的一次性容器。测试必须校验
容器名称、网络和固定本地测试凭据，拒绝外部 host 与外部端口。

### 远程验收

远程阶段必须另行确认后才执行：

1. 停止其他 schema 任务，记录维护窗口和备份/PITR 状态；没有恢复点时明确记录用户
   接受历史数据无法恢复。
2. 先运行 `--repair-status`，记录 exact SHA、Atlas current/pending、manifest ID 和
   非敏感漂移摘要。
3. 运行 `--repair`，确认三个 migration 成功且 post-apply drift 为零。
4. 回归登录、菜单、插件草稿、模型配置、对象存储、通知、计划任务、Workbench 和
   Sandbox 等受影响功能。
5. 验收报告明确写明历史记录没有恢复，管理员权限和模型密钥需要按页面重新配置。

## 实现范围

预计修改以下范围：

- `docker/atlas/migrations/`：三个 repair migration 与 `atlas.sum`。
- `deploy/dev/schema-repairs/`：事故 manifest。
- `deploy/dev/publish-dev.sh`：事故只读模式、事故执行模式和普通模式隔离。
- `deploy/dev/check-schema-repair.sh`：manifest 校验、规范化 diff 与敏感信息清理。
- `scripts/database/check-migration-policy.sh` 及相关测试：特殊 migration 调用合同。
- `deploy/dev/tests/`、`scripts/database/tests/`：门禁、fixture 和安全回归。
- `docs/superpowers/runbooks/project-operations.md` 与
  `docs/superpowers/runbooks/dev-integration-audit.md`：事故恢复操作和单独授权边界。

不修改应用业务代码，不恢复历史数据，不调整远程账号权限，不执行远程 DDL，也不删除
`opencoze_latest_schema.hcl`。遗留快照继续保持非执行入口。

## 完成标准

- 事故 fixture、健康库、部分执行和额外漂移测试全部通过。
- 三个 repair migration 可从空库完整重放，重复执行不改变已有结构或配置。
- 普通发布仍在任何 schema drift 或 repair migration 前 fail closed。
- 事故模式只有在 exact SHA、pending 文件和完整 manifest 全部匹配时才允许 apply。
- 代码提交不包含凭据、远程 SQL 输出、临时数据库文件或用户业务数据。
- 远程执行保持未授权状态，直到用户确认新的 exact SHA、repair 清单和只读预检结果。
