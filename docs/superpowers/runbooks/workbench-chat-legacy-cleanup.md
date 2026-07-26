# WorkbenchChat 旧数据清理

本手册只用于执行已合入的 ChatTask 退役迁移。每个环境独立操作，不把 Debug
结论外推到测试或生产。命令输出和证据不得包含 DSN、消息、payload 或凭据。

## 1. 只读审计

确认以下计数并记录聚合结果：

- `chat_tasks`、`chat_task_attempts`、`chat_task_events` 总数；
- `chat_tasks` 中 queued、running、canceling 数量；
- `agent_threads.legacy_task_id <> 0` 数量；
- `agent_threads.metadata` 中有效 JSON 含 `$.legacy_task_id` 的数量；
- 映射重复、孤立 Thread 和仍有关联 Message/Run/RunEvent 的数量。

存在活跃旧任务、重复映射或无法解释的关联数据时停止，不进入写操作。

## 2. 备份门禁

先向用户提交环境、聚合计数、备份范围、恢复校验方式和保留期限。取得针对该环境
的明确确认后，备份三个旧表、`agent_threads` 的 legacy 映射及 metadata 中对应
键值，并在隔离位置验证备份可读。备份失败或不可恢复时停止。

## 3. 清空门禁

提交备份证据和待执行 SQL 摘要，再取得一次明确确认。事务内按以下顺序处理：

1. 删除 `chat_task_events`；
2. 删除 `chat_task_attempts`；
3. 删除 `chat_tasks`；
4. 将非零 `agent_threads.legacy_task_id` 更新为零；
5. 对有效 JSON metadata 使用 `JSON_REMOVE(metadata, '$.legacy_task_id')` 删除旧键；
6. 提交后重新执行第 1 节计数，所有目标必须为零。

不得把这次确认解释为允许执行 Atlas migration。

## 4. 迁移门禁

零计数复核后，提交 Atlas 版本、migration 文件、hash/validate 结果和目标环境。
同时只读确认迁移账号具备 `SELECT`、`CREATE ROUTINE`、`ALTER ROUTINE`、
`EXECUTE`、`DROP`，以及 MySQL 执行 `ALTER TABLE` 所需的 `ALTER`、`CREATE`、
`INSERT` 权限；不得在本流程中临时提权。以上证据齐全并再次取得明确确认后，才可
应用 `20260726000100_drop_legacy_workbench_chat.sql`。迁移会在旧表非空、legacy
映射非零或 metadata 仍含旧键时失败；所有 DDL 均按对象存在性执行，允许在中途
失败原因修复后安全重试。

应用后验证三个旧表、legacy index/column 均不存在，metadata 旧键计数仍为零，
TaskThread 创建、详情、追问、取消和重试仍通过。任何失败都停止后续环境，不手改
Atlas revision 或跳过门禁。
