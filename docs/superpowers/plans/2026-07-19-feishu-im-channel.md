# 飞书 IM 机器人实施计划

**目标：** 在现有设置弹窗内交付工作空间级飞书机器人配置与真实 Agent
闭环，仅使用飞书官方 Go SDK。

## 工作项

- [x] 明确 Nuwax 参考能力、Coze 权限边界和官方 SDK 接入方式。
- [x] 新增配置、会话、事件三张持久化表。
- [x] 新增工作空间权限控制、凭据加密、CRUD、启停和连接检测 API。
- [x] 新增官方 SDK WebSocket 长连接、数据库租约、事件去重与失败重试。
- [x] 将飞书 chat 映射到 Coze `agentthread` 并回传 Agent 结果。
- [x] 在现有设置弹窗加入 `IM 机器人` 管理页面。
- [x] 更新 `AGENTS.md`，把 IM 边界调整为“仅允许飞书官方 SDK”。
- [x] 更新 Go module 与 Atlas hash。
- [x] 定向 Go/前端测试、迁移校验和内置浏览器验收。

## 定向验证命令

```bash
cd backend
go test ./application/imchannel ./infra/imchannel ./api/handler/coze

cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu
npx tsc --noEmit --project tsconfig.json

cd ../../..
(cd docker/atlas && atlas migrate hash)
atlas migrate validate --dir file://docker/atlas/migrations
```

页面验收使用 Codex 内置浏览器，访问任一工作空间，打开左下角账号设置中的
`IM 机器人`，覆盖空态、创建、检测、启停、编辑、删除、只读权限和错误态。

## 后续事项（暂缓）

当前已通过真实飞书账号完成配置、连接检测、后端重启恢复、首次私聊、连续
私聊、Agent 执行、会话持久化和消息回发闭环。以下生产边界留待后续继续：

- [ ] 验收群聊未 `@` 不响应、明确 `@机器人` 正常响应。
- [ ] 验收长文本流式回复的顺序、完整性、终止和异常恢复。
- [ ] 验收图片、文件等附件消息的受控元数据投影与安全边界。
- [ ] 验收飞书事件重投、同一会话并发消息和跨用户会话隔离。
- [ ] 验收凭据轮换、错误凭据、禁用后重启及重新启用。
- [ ] 验收 Owner/Admin 与普通成员的管理权限和服务端拒绝路径。
- [ ] 补齐上述场景的自动化集成测试和发布前回归清单。
