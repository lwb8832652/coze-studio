# Nuwax 通知中心对齐设计

## 目标

在 Coze Studio 当前工作台顶部铃铛位置实现生产级通知中心，对齐 Nuwax 的
未读数量、通知弹层、消息列表和已读闭环，并保持 Coze 现有页面信息架构。
通知中心不是独立一级菜单，也不复用任务列表伪装消息数据。

参考基线：

- Nuwax 前端实现：
  `/Users/liuwenbo/code/BuildingAI/nuwax-ai/nuwax/src/layouts/Message`
- Nuwax 前端接口：
  `/Users/liuwenbo/code/BuildingAI/nuwax-ai/nuwax/src/services/message.ts`
- Nuwax 后端领域：
  `/Users/liuwenbo/code/BuildingAI/nuwax-ai/nuwax-backend`
- Coze 当前入口：
  `frontend/apps/coze-studio/src/pages/workbench/index.tsx`

Nuwax 当前在打开弹层时立即清空全部未读。Coze 保留相同的信息架构和视觉
体验，但采用单条已读和显式 `全部已读`，避免用户仅查看弹层就丢失未读状态。

## 用户体验

顶部铃铛替换为可复用的 `NotificationBell` 组件，工作台首页、任务详情等存在
同类顶部操作区的页面使用同一状态和交互。

- 未读数量显示在铃铛 Badge 中，超过 99 显示 `99+`；
- 点击铃铛打开右侧对齐的紧凑弹层，不新增通知一级菜单或独立页面；
- 弹层顶部展示 `通知` 和 `全部已读`，没有未读时禁用批量操作；
- 列表项展示系统图标或发送人头像、标题、内容摘要、时间和未读标识；
- 点击未读项先标记已读，再按受控业务目标跳转；
- 没有业务目标的消息仅更新已读状态，不执行跳转；
- 列表使用游标分页，滚动到底部加载更多；
- 支持 loading skeleton、empty、error、retry、loading-more 和 disabled 状态；
- 弹层打开不会自动清空未读，关闭和重新打开保留准确状态；
- 页面重新获得焦点时刷新未读数，页面可见期间按固定间隔轻量轮询。

通知内容长度受限，正文过长时在弹层中截断。时间使用当前用户时区；当天消息
显示相对时间，较早消息显示明确日期。

## 领域模型

### Notification

- `id`
- `scope`: `personal`、`workspace` 或 `system`
- `space_id`: 工作空间消息必填，系统消息为空
- `sender_id`: 系统消息可为空
- `category`: `task`、`scheduled_task`、`workspace` 或 `system`
- `event_type`: 稳定的业务事件类型
- `title`
- `content`: 已脱敏且有长度限制的摘要
- `target_type`: 受控跳转目标类型
- `target_id`: 对应业务对象 ID
- `dedupe_key`: 业务事件幂等键
- `created_at`

### NotificationRecipient

- `id`
- `notification_id`
- `user_id`
- `read_at`: 为空表示未读
- `created_at`

`notification.dedupe_key` 唯一，防止任务重试或事件重复投递产生重复消息。
`notification_recipient` 对 `(notification_id, user_id)` 建唯一索引，并为
`(user_id, read_at, created_at)` 建列表和未读计数索引。

个人消息只创建目标用户接收记录。工作空间消息在服务端根据真实成员关系生成
接收记录。系统消息由系统管理员权限保护的应用服务创建，接收范围由服务端
策略计算，客户端不能提交任意接收人冒充系统广播。

## 事件来源

首批闭环接入当前已有且用户可感知的事件：

- Agent 任务完成、失败和取消；
- 定时任务执行完成和失败；
- 工作空间邀请、成员加入、角色变化和移除；
- 由系统管理应用服务发布的系统公告。

生产者只调用内部通知应用服务，不通过内部 HTTP 请求自身 API。每类生产者
使用 `event_type + source_id + recipient_id` 形成稳定幂等键。任务中心和
AgentThread 对同一执行结果只允许一个通知生产者，避免双重提醒。

通知写入失败不能覆盖原业务结果。生产者记录结构化错误并允许幂等重试；需要
跨事务投递的事件使用现有可靠事件机制或事务后重试，不启动不可恢复的临时
goroutine。

## API 合同

在 IDL 中新增通知合同并生成前端客户端：

- `ListNotifications`
- `GetNotificationUnreadCount`
- `MarkNotificationsRead`
- `MarkAllNotificationsRead`

列表请求包含 `cursor`、`limit` 和可选 `unread_only`，响应包含受限消息字段、
`next_cursor` 和 `has_more`。批量已读限制单次 ID 数量；全部已读使用服务端
时间边界更新，避免并发到达的新消息被误标已读。

所有接口从认证上下文取得用户 ID。服务端查询必须以
`notification_recipient.user_id` 过滤，不能信任客户端提交的 user ID、
sender、scope 或 read 状态。工作空间 ID 只用于受控跳转和权限复核，不能
扩大消息查询范围。

前端通过生成客户端访问 API，不新增手写 fetch client。查询缓存以当前账号
为边界；账号退出或切换时清空通知列表、未读数量和轮询状态。

## 前端状态与交互

通知状态封装为单一 hook/store，负责：

- 未读数量请求、轮询和页面焦点刷新；
- 弹层打开状态、首屏列表和游标分页；
- 单条已读的乐观更新和失败回滚；
- 全部已读的确认更新和失败恢复；
- 重复请求合并、组件卸载取消和账号切换清理。

跳转不接受后端返回的任意 URL。前端仅根据白名单 `target_type` 组装现有
Coze 路由，例如任务详情、定时任务中心和工作空间成员页；目标不存在或权限
失效时保留消息并给出温和提示。

轮询仅在页面可见且用户已登录时运行，默认间隔 30 秒。弹层打开、消息已读和
业务事件在当前页面完成时立即刷新，不等待下一轮询。首期不新增 WebSocket
或 SSE，避免为通知单点能力引入另一套长连接基础设施。

## 安全与隔离

- 通知列表、计数和已读操作均以服务端认证用户为唯一接收人依据；
- 工作空间事件在投递和跳转时都复核真实成员与角色；
- 系统公告发布使用现有系统管理员服务端强校验；
- 通知正文不保存 prompt、模型输出全文、工具参数、工具结果、凭据、
  checkpoint、对象存储 URI 或 provider 原始错误；
- 错误消息只保存脱敏摘要和稳定错误码；
- 列表 limit、正文长度、批量 ID 数量和轮询频率都有服务端或客户端上限；
- 日志不打印通知正文和接收人敏感资料；
- 已删除业务目标不会绕过权限，消息仍可读取和标记已读。

## 数据迁移

使用 Atlas 新增通知和接收人表、唯一键及查询索引，并更新迁移 hash。迁移
只创建新表，不回填任务历史，也不修改现有任务、用户或空间数据。

迁移文件可随代码提交，但本地或共享数据库的 migrate apply 必须单独获得
明确授权。Atlas hash/validate 通过后才能进入部署流程。

## 测试与验收

后端覆盖：

- 未登录、跨用户读取和非法批量 ID；
- 个人、工作空间和系统消息的接收人隔离；
- 幂等投递、重复接收人、并发已读和全部已读时间边界；
- 游标分页、未读计数、空列表和软失效目标；
- 任务、定时任务和工作空间事件的成功及失败路径；
- 系统公告的管理员与普通用户权限。

前端覆盖：

- Badge 的 0、正常数字和 `99+`；
- 弹层打开、首屏加载、空态、错误重试和分页；
- 单条已读、全部已读、失败回滚和重复点击；
- 白名单跳转、失效目标、账号切换和轮询清理；
- 不因打开弹层自动清空未读。

页面验收使用 Codex in-app browser，在本地 Coze 账号和个人/团队空间中验证：

- 新消息到达后未读数量出现；
- 弹层结构、列表密度和状态反馈与 Nuwax 基线一致；
- 打开弹层不清空未读，单条和全部已读结果正确；
- 任务完成/失败、定时任务和工作空间事件产生真实通知；
- 点击通知只能跳转到有权限的目标；
- 刷新页面、切换空间和重新登录后状态与数据库一致；
- 网络失败时显示可恢复错误，控制台无未处理异常。

相关 Go tests、Vitest、TypeScript 检查和 Atlas hash/validate 全部通过后，才可
声明通知中心对齐完成。
