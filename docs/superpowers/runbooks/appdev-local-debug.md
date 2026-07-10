# AppDev Local Debug Runbook

## 本地地址

- Coze Studio Frontend: `http://localhost:8080`
- Coze Studio Backend: `http://localhost:8888`
- AppDev 项目入口: `http://localhost:8080/space/<space_id>/app-dev`
- AppDev IDE: `http://localhost:8080/space/<space_id>/app-dev/<project_id>`

## nuwax-ai 参考环境

- Reference URL: `http://localhost/`
- Login: `admin@nuwax.com`
- Password: `123456`

## AppDev 运行目录

默认工作目录来自环境变量 `APP_DEV_WORKSPACE_ROOT`。

未配置时会落到用户缓存目录：

```bash
~/Library/Caches/coze-studio/appdev
```

目录结构：

```text
spaces/<space_id>/<project_id>/project.json
spaces/<space_id>/<project_id>/files/
```

## 关键 API

- `GET /api/app-dev/spaces/:space_id/projects`
- `POST /api/app-dev/spaces/:space_id/projects`
- `POST /api/app-dev/spaces/:space_id/projects/import`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/duplicate`
- `DELETE /api/app-dev/spaces/:space_id/projects/:project_id`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/export`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/build`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/release`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/files`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/files/content?path=<path>`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/files/content`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/files/upload`
- `DELETE /api/app-dev/spaces/:space_id/projects/:project_id/files`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/files/rename`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/snapshots`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/snapshots`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/snapshots/:snapshot_id/restore`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/runtime/start`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/runtime/status`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/runtime/keep-alive`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/runtime/restart`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/runtime/stop`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/runtime/logs`
- `GET /api/app-dev/spaces/:space_id/models?scenario=PageApp`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/chat`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/chat/events`
- `POST /api/app-dev/spaces/:space_id/projects/:project_id/chat/cancel`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/chat/history`
- `GET /api/app-dev/spaces/:space_id/projects/:project_id/chat/status`

## 常见问题

- 项目管理：入口页支持创建、导入、重命名、复制、归档和进入开发；IDE 顶部支持修改当前项目名称和描述。
- 文件编辑：保存会携带版本号；如果服务端文件已变化，会拒绝陈旧版本覆盖并提示重新加载；在线编辑读写单文件限制为 2MB；二进制文件会保留在项目中，但不能直接在代码编辑器打开。
- 文件管理：新建、上传和重命名会避免覆盖已有路径；删除和重命名会同步刷新项目更新时间；文件树、导入和导出会过滤 `.coze-appdev`、`.git`、`node_modules`。
- 看不到模型：先到系统管理的模型配置中添加 LLM 模型，AppDev 只读取安全元数据。
- 首次启动预览慢：运行环境会自动执行 `npm install --ignore-scripts --silent`，日志面板可查看进度；如果导入项目缺少 Vite，会用安全安装方式补齐 `vite/react/react-dom/typescript` 最小运行依赖；启动时不会执行用户项目里的 `npm run dev`，而是写入 `.coze-appdev/vite.config.mjs` 并直接调用本地 Vite；后端只有在预览 URL 探测可访问后才会把状态置为 `running`；如果 5 分钟仍不可访问，会标记为异常并停止对应进程组；停止运行环境也会清理对应进程组，避免残留 `npm/vite` 进程。
- 启动失败：如果项目缺少 `package.json`，运行状态会显示异常并在日志里记录 `missing package.json`。
- AI 生成结果：发送网页应用需求后，后端会优先调用所选模型生成 `src/App.tsx`；模型不可用或输出不安全时使用安全模板兜底，完成后 IDE 会刷新文件树。
- AI 迭代上下文：生成时会读取当前 `src/App.tsx` 的安全截断内容，便于在已有页面基础上继续修改。
- 数据源绑定：聊天面板会读取当前空间知识库列表，选中的数据源名称会作为安全上下文传给模型；当前阶段不会直接检索或展示知识库内容。
- SSE 事件流：服务端会发送 `heartbeat` 保持长连接；前端只投影安全字段，并会对 token、API key、对象存储 URI 等敏感片段做兜底脱敏。
- SSE 完成事件丢失：聊天运行期间前端每 3 秒查询一次 `chat/status`；服务端返回
  `running: false` 后会重新加载历史并刷新文件树和预览，然后停止轮询。不要通过
  增加更高频轮询修复连接问题。
- 导入失败：只支持 `.zip`，压缩包内路径不能包含 `..`、绝对路径或超大文件；缺少 `package.json`、`index.html`、`src/main.tsx` 或 `src/App.tsx` 时会自动补默认运行文件。
- 导出文件名异常：中文项目名会在后端下载头中转义，前端下载时会使用当前项目名兜底。
- nuwax 页面空白且 `/api/tenant/config` 失败：先确认 `docker-backend-1` 能解析
  `redis`、`mysql`、`elasticsearch`。如果 nuwax backend 未加入 Coze middleware
  网络，可执行：

```bash
docker network connect coze-studio-debug_coze-network docker-backend-1
```

重复执行前先检查容器网络，避免已连接时产生无意义错误。

## 建议验收命令

```bash
cd frontend/apps/coze-studio
npm run test -- src/components/workspace-sub-menu/__tests__/workspace-sub-menu.test.tsx src/pages/app-dev
npx tsc --noEmit --project tsconfig.json
```

```bash
cd backend
go test ./application/appdev ./infra/appdev -count=1
go test -gcflags='all=-l -N' ./api/handler/coze -run AppDev -count=1
```

2026-07-10 验收结果：前端 6 个文件共 39 项测试通过，TypeScript 检查通过，
AppDev 应用层、基础设施和 handler 定向测试通过。内置浏览器完成 AI 长任务、
SSE 状态对账、发布、产物下载、源码导出和一期核心页面回归，应用控制台错误数为 0。
