# WorkbenchChat 旧数据 Debug 审计证据

本文件是 2026-07-26 Debug 环境的一次性只读证据，不是当前架构或其它环境事实，
不得用于跳过后续环境的独立审计、备份和确认。

- `chat_tasks`：22；其中 created 2、failed 9、succeeded 11，活跃状态 0；
- `chat_task_attempts`：0；
- `chat_task_events`：5,878；
- `agent_threads.legacy_task_id <> 0`：2；
- 映射到 Thread 的旧任务：2；未映射旧任务：20；
- 未发现重复映射、孤立映射；已映射 Thread 没有 Message、Run 或 RunEvent；
- metadata `$.legacy_task_id`：0（同日补充只读审计）；

证据只包含聚合计数，不包含 DSN、消息、payload、凭据或连接信息。数据库尚未执行
备份、清空或 migration apply。
