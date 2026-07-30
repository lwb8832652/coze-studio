# Dev 自动镜像发布与宝塔部署设计

## 1. 目标

远程 `dev` 更新后，由 GitHub Actions 构建前后端镜像并推送到阿里云 ACR。
两个镜像准备完成后，GitHub 调用宝塔 webhook，服务器拉取同一提交对应的镜像并
更新预发布服务。部署失败时恢复上一组前后端镜像。

这套流程服务于 `dev` 和预发布环境，允许单实例更新产生约 10 到 30 秒中断。
正式生产发布不在本设计范围内。

## 2. 当前事实

- `.github/workflows/deploy-dev.yml` 监听远程 `dev` push，并支持使用完整 SHA 的
  `workflow_dispatch`。
- `backend/Dockerfile` 和 `frontend/Dockerfile` 已能分别构建后端与 Web 镜像。
- `docker/docker-compose.yml` 面向本地完整环境，包含 MySQL、Redis、
  Elasticsearch、MinIO、Milvus、Etcd 和 NSQ，不适合 2C4G 预发布服务器。
- 现有 Compose 使用 `cozedev/*:latest`，没有不可变版本、应用健康检查和自动
  回滚。
- 后端先完成 `application.Init()`，随后才启动 Hertz HTTP Server。只要公开健康
  路由能够响应，就说明启动阶段的核心依赖已经初始化。
- 数据库是远程服务，迁移由运维人员手动执行。自动发布不得连接数据库或执行
  Atlas。
- 七牛等多云对象存储能力仍在 `codex/object-storage-control-plane` 分支，未进入
  当前 `dev`。本需求只提供发布通道，不重复实现或绕过该分支的集成审计。

## 3. 已确认决策

- 目标环境是 `dev` 预发布服务器。
- 使用单实例更新，不做蓝绿部署。
- GitHub Actions 在前后端镜像都推送成功后直接调用宝塔 webhook。
- ACR 使用两个仓库：`coze-server` 和 `coze-web`。
- 每次构建产生 `dev-<full-sha>` 不可变标签，并更新兼容 webhook 的 `dev` 标签。
- 宝塔 webhook 暂按不能传递参数设计，部署脚本同时保留可选 SHA 参数。
- 数据库迁移手动执行。提交包含 `docker/atlas/migrations/**` 变化时，自动发布
  构建镜像但暂停部署。
- 健康检查失败后自动恢复部署前的前后端镜像。

## 4. 发布架构

### 4.1 GitHub Actions

新增 `.github/workflows/deploy-dev.yml`，支持两种触发方式：

- `push` 到 `dev`：构建当前提交。没有迁移变化时自动部署。
- `workflow_dispatch`：接收完整提交 SHA，用于迁移完成后继续部署指定版本，或
  重放已构建的版本。

工作流使用 `concurrency: deploy-dev`，`cancel-in-progress` 为 `false`，队列为
`max`。后提交的发布必须保留并等待正在进行的发布结束，不能在服务器更新过程中
取消旧任务或替换待执行的 migration 检查。

工作流包含以下 job：

1. `preflight` 解析目标 SHA，检查事件完整性，并从已晋级 revision 到目标 SHA
   检查迁移目录，输出 `migration_changed`。
2. `build-server` 构建后端镜像，推送 `dev-<sha>`，但暂不覆盖 `dev`。
3. `build-web` 构建 Web 镜像，推送 `dev-<sha>`，但暂不覆盖 `dev`。
4. `verify-images` 在 push 构建完成后和手工任务中确认两张不可变镜像存在且 OCI
   revision 等于目标 SHA；手工任务不重新构建。
5. `promote` 只在两个构建成功、不可变镜像验证成功且没有迁移变化时，把同一 SHA
   的两个镜像提升为 `dev`；手动任务则在同一验证成功后执行提升。
6. `deploy` 只在 `promote` 成功后调用宝塔 webhook。
7. `migration-hold` 在检测到迁移变化时明确结束为待人工迁移状态，不调用
   webhook。

`workflow_dispatch` 不重新构建镜像。它先确认指定 SHA 的两个不可变镜像都存在，
再更新 `dev` 标签并调用 webhook。这样可以在手动迁移后发布原来的提交，也可以
恢复一个已知版本。

### 4.2 镜像合同

两个 Dockerfile 增加构建参数 `GIT_REVISION`，并写入以下 OCI 标签：

- `org.opencontainers.image.revision=<full-sha>`
- `org.opencontainers.image.source=<repository-url>`

后端镜像同时设置 `APP_REVISION=<full-sha>`，供 `/healthz` 返回当前运行版本。
不可变标签必须使用完整 SHA。`dev` 只是兼容固定 webhook 的发布指针，不能用于
审计或人工回滚判断。

前端镜像直接复制项目的 Nginx 配置，不依赖服务器源码挂载。配置保留 API 和
SSE 代理，并增加 `/healthz` 到后端的代理。对象存储 URL 由应用配置决定，预发布
Nginx 不再强制代理到本地 `minio:9000`。

### 4.3 服务器部署单元

仓库新增 `deploy/dev/`：

- `docker-compose.yml` 只包含 `coze-server` 和 `coze-web`。
- `deploy.sh` 完成拉取、版本校验、更新、健康检查和回滚。
- `.env.example` 只描述 ACR registry、namespace、前后端标签和健康检查超时，不含
  真实凭据；业务运行配置固定保存在服务器本地 `app.env`。
- `README.md` 说明宝塔安装、ACR 登录、环境文件和 webhook 配置步骤。
- `tests/` 保存部署脚本的命令替身和回归测试。

服务器固定安装目录是 `/opt/coze-dev`：

```text
/opt/coze-dev/
  .env.example
  docker-compose.yml
  deploy.sh
  deploy.env
  app.env
  deploy.lock
  deployments/
```

`app.env` 以只读方式挂载到后端 `/app/.env`，权限必须是 `600`。它保存远程
MySQL、Redis、Elasticsearch、对象存储、向量数据库、消息队列及应用密钥。镜像
和 GitHub Actions 都不保存这些值。

Web 只绑定 `127.0.0.1:8888`，宝塔 Nginx 负责域名、HTTPS 和公网入口。后端端口
只在 Docker 网络内开放。

## 5. 部署状态机

### 5.1 部署前检查

宝塔调用固定的 `/opt/coze-dev/deploy.sh`。脚本先执行以下检查：

- 使用 `flock` 获取 `/opt/coze-dev/deploy.lock`。
- Docker、Docker Compose、curl 和部署文件可用。
- ACR 已登录，目标仓库可以拉取。
- 当前运行容器和镜像状态可以读取。

无法获取锁时返回非零状态，不排队执行第二次部署。前置检查失败时不停止现有
容器。

### 5.2 拉取和版本校验

脚本在更新前保存两个当前容器的镜像 ID、revision。随后拉取两个 `dev` 镜像，
读取 `org.opencontainers.image.revision`，并要求：

- 两个 revision 都存在；
- 两个 revision 完全相同；
- 调用方传入可选 SHA 时，revision 必须与参数相同。

任一拉取失败或 revision 不一致时，脚本直接失败，现有容器继续运行。

### 5.3 更新和健康检查

版本校验通过后，Compose 只更新 `coze-server` 和 `coze-web`。后端增加公开
`GET /healthz`，返回：

```json
{
  "status": "ok",
  "revision": "<full-sha>"
}
```

健康路由不读取数据库，也不返回内部依赖、配置或凭据。Hertz 只会在
`application.Init()` 成功后注册并提供该路由，因此它适合作为本次单实例发布的
启动就绪信号。

部署脚本轮询 Web 入口的 `/healthz` 和 `/`。`/healthz` 必须返回目标 SHA，首页
必须返回成功状态。健康检查后还必须确认两个运行容器的实际 image ID 分别等于
本次拉取的候选 image ID，之后才在 `deployments/` 写入本次 SHA、镜像 ID、时间。

### 5.4 回滚

更新或健康检查失败时，脚本将部署前保存的两个镜像 ID 重新标记为本地回滚标签，
再用 Compose 恢复两个服务。脚本先确认两个容器的实际 image ID 等于保存值，再
重复检查 `/healthz` 和首页。

即使回滚成功，本次部署仍返回非零状态，使 GitHub 和宝塔记录发布失败。回滚也
失败时保留容器日志、旧新镜像 ID 和部署记录，供人工恢复。脚本不得打印应用
环境文件、ACR 密码、Webhook Token 或完整 Docker 登录配置。

## 6. 数据库迁移拦截

自动发布只检查 Git diff，不连接远程数据库。

`push` 事件先校验 `before`、当前 SHA 和两者的祖先关系；字段缺失、全零或 Git
对象不可读时默认暂停部署。实际迁移比较基线来自 ACR 当前
`coze-server:dev`、`coze-web:dev` 两张镜像的一致 OCI revision，即上次成功晋级
的完整 SHA。基线缺失、两个 revision 不一致或基线不属于目标历史时同样暂停。

Workflow 比较已晋级 SHA 到当前目标 SHA 的完整迁移目录变化。迁移提交 A 被暂停
后，后续普通提交 B 仍从旧的已晋级 SHA 比较，因此不能绕过 hold。人工恢复成功
后，两张新 `dev` 镜像的 revision 成为下一次 push 的比较基线。

有迁移变化时，两个不可变镜像仍然构建并推送，但 `dev` 标签不更新，webhook
不调用。运维人员完成远程 Atlas 迁移后，从 Actions 手动运行同一个 workflow，
填写待发布完整 SHA。手动任务确认两个镜像存在后再提升和部署。

## 7. 权限和密钥

GitHub 配置：

- Variables：`ACR_REGISTRY`、`ACR_NAMESPACE`。
- Secrets：`ACR_USERNAME`、`ACR_PASSWORD`、`BAOTA_WEBHOOK_URL`。
- 可选 Secret：`BAOTA_WEBHOOK_TOKEN`。
- Workflow 权限：`contents: read`。

GitHub ACR 账号只允许向目标命名空间推送镜像。服务器 ACR 账号只允许拉取。
Webhook URL 本身按密钥管理；宝塔支持请求头时，再发送 Bearer Token。请求体可以
包含 revision 供日志使用，但服务器部署不能依赖该字段。

只允许 `push` 到仓库自身的 `dev` 触发部署。Pull Request、fork 和普通需求分支
不能访问部署 Secrets，也不能调用 webhook。

## 8. 测试

### 8.1 后端测试

路由测试覆盖：

- `/healthz` 不需要登录。
- 响应状态是 200。
- `status` 固定为 `ok`。
- `revision` 来自 `APP_REVISION`，未配置时返回空字符串但仍可用于本地开发探活。

### 8.2 部署脚本测试

Shell 测试通过临时 PATH 注入 Docker、Compose、curl 和 flock 替身，不访问真实
ACR 或服务器。至少覆盖：

- 拉取失败时不更新容器。
- 两个镜像 revision 不一致时拒绝部署。
- 成功部署后写入目标 SHA。
- 健康检查失败时恢复两个旧镜像。
- 回滚成功后部署仍返回失败。
- 已持有部署锁时第二次调用失败。

### 8.3 配置和工作流测试

- `docker compose config` 校验预发布 Compose。
- `bash -n` 与 Shell 测试校验 `deploy.sh`。
- 静态检查 workflow 的 `dev` 触发、并发策略、迁移拦截、双镜像依赖和 Secrets
  引用。
- 构建前后端镜像，检查 OCI revision 标签。
- 启动最小可用环境后验证后端与 Web 的 `/healthz`。

真实 ACR 推送和宝塔 webhook 只在 GitHub 配置完成后验收。第一次发布使用
`workflow_dispatch` 指定已构建 SHA，记录 Actions URL、目标 SHA、容器 revision、
健康检查结果和服务器回滚记录目录。

## 9. 运维和集成门禁

这项能力改变了远程 `dev` 的运行语义：推送成功会自动触发预发布部署。因此需要
同步更新 `project-context.md` 和 `dev-integration-audit.md`。

第二次审计报告必须明确写出：确认推送 `origin/dev` 将触发 ACR 镜像发布和宝塔
预发布部署。用户的第二次确认同时授权该次推送及其自动部署结果，不授权生产
部署、迁移执行或其他服务器操作。

远程 `dev` 在审计期间变化时，继续遵循现有规则，回到第一次审计。自动发布不
改变禁止 force push、双重确认和远程竞态检查。

## 10. 不在本次范围内

- 正式生产环境发布。
- 蓝绿、金丝雀或多实例滚动更新。
- 自动执行远程数据库迁移或备份。
- 创建或修改阿里云 ACR、RAM、宝塔、DNS 和 TLS 资源。
- 合并对象存储控制面分支。
- 把 Milvus、消息队列或其他中间件迁移到云服务。

## 11. 验收标准

- `dev` 普通提交能构建并推送两个同 SHA 镜像，随后触发一次 webhook。
- 迁移提交不会更新 `dev` 标签，也不会调用 webhook。
- 手动任务可以部署已存在的完整 SHA。
- 服务器在拉取失败或版本不一致时保持旧服务运行。
- 新版本健康时，两个容器 revision 与目标 SHA 一致。
- 新版本不健康时，两个服务恢复到部署前镜像，发布状态是失败。
- 应用密钥、ACR 密码和 webhook 凭据不进入镜像、Git、响应或部署日志。
- 2C4G 服务器只运行前后端和必要的轻量代理，不启动默认 Compose 的完整中间件。
