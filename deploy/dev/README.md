# dev 预发布部署手册

## 适用范围

本目录用于单实例 dev/预发布环境。服务器只运行 `coze-server` 和 `coze-web`
两个容器，MySQL、Elasticsearch、Redis 和对象存储使用远程云服务。2C4G Linux
服务器可以承载这两个应用容器，但需要为镜像拉取、日志和构建外的运行峰值保留
磁盘与内存余量。

`coze-web` 只绑定 `127.0.0.1:8888`，`coze-server` 只在 Docker 网络内暴露
`8888`。宝塔 Nginx 负责公网 HTTPS。该拓扑没有蓝绿或多实例滚动能力，发布时
允许约 10 到 30 秒中断，不能直接作为生产发布方案。

对象存储配置必须以目标 SHA 已合入的 provider 为准。当前部署分支不包含七牛
适配时，不能只修改 `app.env` 来启用七牛；应先完成对应功能分支的审计和合入。

## ACR 和 GitHub

在同一 ACR 命名空间创建两个私有仓库：

- `coze-server`
- `coze-web`

为 GitHub Actions 创建只允许向这两个仓库拉取、推送和更新标签的账号。为服务器
创建独立的只读拉取账号，不要复用 Actions 推送账号。

在 GitHub 仓库中配置：

| 类型 | 名称 | 用途 |
| --- | --- | --- |
| Variable | `ACR_REGISTRY` | ACR registry 主机名 |
| Variable | `ACR_NAMESPACE` | 两个镜像仓库所在命名空间 |
| Secret | `ACR_USERNAME` | Actions 推送账号 |
| Secret | `ACR_PASSWORD` | Actions 推送凭据 |
| Secret | `BAOTA_WEBHOOK_URL` | 宝塔预发布 webhook 地址 |
| Secret，可选 | `BAOTA_WEBHOOK_TOKEN` | webhook 请求头凭据 |

Workflow 的 `GITHUB_TOKEN` 只需要 `contents: read`。不要配置 SSH 私钥、数据库
连接串或 `app.env` 内容。

## 安装服务器目录

服务器需要 Docker Engine、Docker Compose v2、`curl` 和提供 `flock` 的
`util-linux`。先确认命令可用：

```bash
docker version
docker compose version
curl --version
flock --version
```

将部署文件安装到固定目录：

```bash
sudo install -d -m 750 /opt/coze-dev
sudo install -m 750 deploy/dev/deploy.sh /opt/coze-dev/deploy.sh
sudo install -m 640 deploy/dev/docker-compose.yml /opt/coze-dev/docker-compose.yml
sudo install -m 640 deploy/dev/.env.example /opt/coze-dev/.env.example
```

宝塔 webhook 使用的系统账号必须能够读取该目录并访问 Docker。不要让无关账号
获得 `app.env` 或 Docker socket 权限。

## 配置文件

### deploy.env

`deploy.env` 只保存部署参数：

```bash
cd /opt/coze-dev
test -e deploy.env || cp .env.example deploy.env
chmod 600 deploy.env
```

按实际 ACR 修改 `ACR_REGISTRY` 和 `ACR_NAMESPACE`。日常发布保持
`SERVER_IMAGE_TAG=dev`、`WEB_IMAGE_TAG=dev`。`DEPLOY_HEALTH_TIMEOUT_SECONDS`
必须是正整数。`deploy.env` 是服务器本地文件，不要提交或附到工单中。

使用服务器只读账号登录 ACR。登录动作必须由实际执行 webhook 的同一系统账号
完成，并在 `deploy.env` 配置后执行：

```bash
cd /opt/coze-dev
source deploy.env
read -r -p 'ACR pull username: ' ACR_PULL_USERNAME
read -r -s -p 'ACR pull password: ' ACR_PULL_PASSWORD
printf '\n'
printf '%s' "$ACR_PULL_PASSWORD" | docker login "$ACR_REGISTRY" \
  --username "$ACR_PULL_USERNAME" --password-stdin
unset ACR_PULL_PASSWORD
```

不要在脚本、命令历史或宝塔日志中写入凭据字面值。

### app.env

单独创建 `/opt/coze-dev/app.env`，写入后端运行所需的远程服务地址和应用密钥：

```bash
cd /opt/coze-dev
touch app.env
chmod 600 app.env
```

`app.env` 至少需要按目标版本核对 MySQL DSN、Redis、Elasticsearch 和对象存储
配置。远程地址不能继续使用 `mysql`、`redis`、`elasticsearch` 等本地 Compose
服务名，也不能误写服务器容器内的 `127.0.0.1`。对象存储 credential 只写入此
文件；后台配置能力合入后，按该能力的加密存储合同执行，不把 secret 回显到页面。

该文件以只读方式挂载到 `/app/.env`。不要提交、打印或通过 webhook 传输它。

## 宝塔配置

### Webhook

宝塔 webhook 固定执行：

```bash
/opt/coze-dev/deploy.sh
```

GitHub 会在 JSON body 中发送目标 SHA。即使宝塔忽略 body，`deploy.sh` 仍会比较
两张 `:dev` 镜像的 OCI revision，revision 不一致时不会重启服务。若宝塔脚本
转发 SHA，只能把已校验的 40 位十六进制值作为唯一参数传给 `deploy.sh`。

为 webhook 配置随机 URL 或 token，并限制来源网络。不要把 token 拼到固定命令、
查询日志或部署输出中。

### Nginx

在宝塔站点启用 HTTPS，将流量代理到本机 Web 容器：

```nginx
location / {
    proxy_pass http://127.0.0.1:8888;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header Connection "";
    proxy_buffering off;
    proxy_read_timeout 3600s;
    proxy_send_timeout 3600s;
}
```

根据业务上传上限配置站点的 `client_max_body_size`。防火墙不要向公网开放
`8888`；公网只暴露宝塔 Nginx 的 HTTPS 端口和受限管理入口。

## 发布流程

### 首次部署

1. 推送目标提交到 `origin/dev`。Actions 构建并推送
   `coze-server:dev-<full-sha>` 和 `coze-web:dev-<full-sha>`。
2. 首次没有两张一致的 `:dev` 基线，workflow 会进入 migration hold，不会晋级
   `:dev`，也不会调用 webhook。
3. 运维人员核对远程数据库 schema。存在待执行迁移时，先备份，再从目标 SHA 的
   受控仓库 checkout 手工执行 Atlas。
4. 在 Actions 手工运行 `Publish and deploy dev images`，输入同一完整 SHA。
5. Workflow 验证两张不可变镜像和 OCI revision，晋级两个 `:dev` 标签，再调用
   宝塔 webhook。
6. 检查 Actions、`/healthz` 和 `/opt/coze-dev/deployments/current.env`。

### 日常发布

没有 migration 变化时，push workflow 从当前两张 `:dev` 的一致 revision 比较到
目标 SHA。两个不可变镜像构建成功后，workflow 先拉取并确认两张镜像的 OCI
revision 都等于目标 SHA，再依次晋级两个 `:dev` 标签并调用 webhook。服务器再次
校验双 revision，共同更新两个服务，并在记录成功前核对两个容器实际运行的
image ID 都是本次候选值。

### Migration hold

只要当前已晋级 SHA 到目标 SHA 之间包含 `docker/atlas/migrations` 变化，workflow
就只构建不可变镜像，不更新 `:dev`，也不调用 webhook。A 被 hold 后，即使又推送
不含 migration 的 B，比较区间仍从旧的已晋级 SHA 到 B，因此 B 继续 hold。

在目标 SHA 的受控 checkout 中先校验 migration：

```bash
docker run --rm \
  -v "$PWD/docker/atlas/migrations:/migrations:ro" \
  arigaio/atlas:0.35.0-community-alpine \
  migrate validate --dir file:///migrations
```

备份远程数据库并确认维护窗口后，由授权运维人员手工 apply：

```bash
docker run --rm \
  -v "$PWD/docker/atlas/migrations:/migrations:ro" \
  arigaio/atlas:0.35.0-community-alpine \
  migrate apply --dir file:///migrations --url "$ATLAS_URL"
```

`ATLAS_URL` 只存在于受控运维环境。apply 成功后，用同一目标 SHA 执行
`workflow_dispatch`。Workflow 和服务器脚本都不会自动执行数据库 migration。

## 回滚

`deploy.sh` 在更新前保存两个旧 image ID。Compose 更新、健康检查或候选容器
image ID 核验失败时，它会给两个旧镜像创建同一 transaction 的本地 rollback
标签，共同恢复两项服务，并核对两个容器的实际 image ID 都等于保存值。回滚
成功后本次发布仍返回非零，Actions/宝塔必须显示失败。检查：

```bash
cd /opt/coze-dev
ls -lt deployments
cat deployments/current.env
cat deployments/failed-*.env
```

人工回滚首选 Actions `workflow_dispatch`：输入已构建且仍在 `origin/dev` 历史中的
旧完整 SHA。Workflow 会验证旧的两张不可变镜像，重新晋级并部署。执行前确认该
代码版本与当前数据库 schema 向后兼容；该流程不执行 down migration。

只有 GitHub/ACR 流程不可用且自动回滚未完成时，才根据同一份 `failed-*.env` 中的
两个旧 image ID 做服务器本地恢复。两个服务必须使用同一 transaction 标签：

```bash
cd /opt/coze-dev
source deploy.env
transaction="manual-$(date -u +%Y%m%dT%H%M%SZ)"
docker tag "$OLD_SERVER_IMAGE_ID" \
  "$ACR_REGISTRY/$ACR_NAMESPACE/coze-server:rollback-$transaction"
docker tag "$OLD_WEB_IMAGE_ID" \
  "$ACR_REGISTRY/$ACR_NAMESPACE/coze-web:rollback-$transaction"
SERVER_IMAGE_TAG="rollback-$transaction" WEB_IMAGE_TAG="rollback-$transaction" \
  docker compose --env-file deploy.env -f docker-compose.yml \
  up -d --no-build --remove-orphans coze-server coze-web
```

先从失败记录读取并人工核对 `OLD_SERVER_IMAGE_ID`、`OLD_WEB_IMAGE_ID`，再导出这
两个变量。禁止只回滚一个服务。恢复后执行完整健康检查，并保留操作记录。

## 排障

```bash
cd /opt/coze-dev
docker compose --env-file deploy.env -f docker-compose.yml ps
docker compose --env-file deploy.env -f docker-compose.yml logs --tail=200 coze-server coze-web
curl --fail --silent --show-error http://127.0.0.1:8888/healthz
docker image inspect "$ACR_REGISTRY/$ACR_NAMESPACE/coze-server:dev" \
  --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}'
docker image inspect "$ACR_REGISTRY/$ACR_NAMESPACE/coze-web:dev" \
  --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}'
```

两个 revision 必须相同。查看 `deployments/failed-*.env` 中的失败原因和回滚结果，
不要把 `app.env`、Docker 登录配置或 GitHub Secrets 附到工单中。
