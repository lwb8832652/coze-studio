# Dev 数据库重建设计

## 目标

重建当前 Debug 配置指向的共享远程 dev MySQL 数据库，清除 Atlas revision 与实际
schema 不一致造成的 Workbench 新任务 500。后续本地调试统一连接该共享 dev 数据库，
不启动或使用本地 MySQL。

## 范围与边界

- 目标仅限 `docker/.env.debug` 中 `MYSQL_HOST`、`MYSQL_PORT`、`MYSQL_DATABASE`
  明确指定的远程 dev 数据库。
- 执行前必须验证目标 host 不是 loopback，数据库名为预期的 `opencoze`，避免扩大
  删除范围。
- 允许删除目标数据库中的全部业务数据和 Atlas revision；不修改同一实例上的其他
  数据库。
- 不提交数据库密码、DSN、运行日志或 `.env.debug`。
- 不启动本地 MySQL；Redis、Elasticsearch、对象存储继续使用当前 Debug 配置。

## 实施方案

1. 先停止当前后端进程，避免重建期间继续写库；前端可保持运行。
2. 使用仓库固定 Atlas 镜像校验 migration hash 与 migration 语法。
3. 从 ignored Debug 环境解析并校验远程目标，删除并重新创建目标数据库，字符集使用
   `utf8mb4`，排序规则使用 `utf8mb4_unicode_ci`。
4. 对空数据库执行仓库全部 forward migration，不做 baseline、repair、down migration
   或手工 revision 写入。
5. 只读核对 Atlas status、核心表、Journal 表、系统配置表及模型管理表均存在。
6. 使用同一份远程 dev 数据库配置重启后端，确认启动阶段无缺表错误。
7. 通过本地页面完成账号初始化后，回归新任务创建和 Pro/Ultra Journal 创建路径。

## 失败与恢复

- 目标校验、Atlas validate 或连接检查任一失败时，在删除数据库前停止。
- 删除完成但 migration apply 失败时，不回写旧 revision；修复 migration 问题后继续对
  空目标库 forward apply。
- 本方案不保留旧数据，因此不提供数据级回滚；恢复方式是再次从空库执行完整迁移。
- 若账号或模型配置因重建丢失，按系统管理页面重新初始化，不从旧库手工拷贝记录。

## 验收

- Atlas status 无 pending migration，且 revision 与仓库 migration 目录一致。
- `agent_run_attempts`、Journal snapshot/ledger、`scheduled_tasks`、通知、公告、IM、
  模型管理等当前代码依赖表存在。
- 后端启动日志不再出现这些表不存在的错误。
- `http://localhost:8080` 使用共享 dev 数据库完成登录/初始化后，新建简单任务不再返回
  `Internal server error`；Pro/Ultra 根任务能创建 Run 与 Journal Attempt。
