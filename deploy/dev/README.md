# dev 预发布部署手册

## 适用范围

本目录用于单实例 dev/预发布环境。服务器运行 `nsqd`、`coze-server` 和
`coze-web` 三个容器；MySQL、Elasticsearch、Redis 和对象存储继续使用远程
云服务。`nsqd` 使用 `nsq-data` 命名卷持久化消息，并固定为磁盘队列模式。

`coze-web` 默认发布到 `0.0.0.0:8888`，可通过公网 IP 和端口直接访问；
`coze-server` 与 NSQ 不发布宿主机端口。公网端口是 HTTP，正式域名和 HTTPS
由宝塔 Nginx 反向代理处理。该拓扑允许短时发布中断，不是高可用生产方案。

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
| Variable，可选 | `BAOTA_WEBHOOK_PINNED_PUBKEY` | 宝塔自签名证书的 curl SHA-256 公钥指纹 |

Workflow 的 `GITHUB_TOKEN` 只需要 `contents: read`。不要配置 SSH 私钥、数据库
连接串或 `app.env` 内容。

优先为宝塔 webhook 配置与域名匹配、受公共 CA 信任的证书，此时不要设置
`BAOTA_WEBHOOK_PINNED_PUBKEY`。如果必须使用宝塔自签名证书，生成并核对当前服务端
证书的公钥指纹：

```bash
openssl s_client -connect bt.example.com:8888 -servername bt.example.com \
  -showcerts </dev/null 2>/dev/null \
  | openssl x509 -pubkey -noout \
  | openssl pkey -pubin -outform der \
  | openssl dgst -sha256 -binary \
  | openssl base64
```

在结果前加 `sha256//` 后写入该 Variable。Workflow 只会在配置了合法公钥指纹时
组合使用 curl 的 `--insecure` 和 `--pinnedpubkey`；连接仍必须匹配固定公钥，不会
退化为无校验 HTTPS。宝塔证书或私钥轮换后，需要先同步更新该 Variable。

## 安装服务器目录

服务器需要 Docker Engine、Docker Compose v2、`curl` 和提供 `flock` 的
`util-linux`。先确认命令可用：

```bash
docker version
docker compose version
curl --version
flock --version
```

先在宝塔配置中确认 webhook 实际运行的系统账号，再在管理 shell 中记录该账号和
主组。不要默认使用 `root`；只有确认宝塔 webhook 已明确配置为 `root` 并接受其
权限范围时，才能在提示中输入并确认 `root`：

```bash
read -r -p 'Baota webhook system user: ' DEPLOY_USER
[ -n "$DEPLOY_USER" ] || {
  printf '%s\n' 'deployment user is required' >&2
  exit 1
}
id "$DEPLOY_USER"
DEPLOY_GROUP="$(id -gn "$DEPLOY_USER")"
if [ "$DEPLOY_USER" = root ]; then
  read -r -p 'Confirm the webhook intentionally runs as root [yes/NO]: ' confirm_root
  [ "$confirm_root" = yes ] || {
    printf '%s\n' 'root deployment was not confirmed' >&2
    exit 1
  }
  unset confirm_root
fi
```

服务器 `/opt/coze-dev` 的部署文件清单固定为：

```text
.env.example
docker-compose.yml
deploy.sh
deploy.env
app.env
```

在同一个管理 shell 中安装仓库里的前三个文件。`/opt/coze-dev` 必须由实际部署
账号拥有并可写，因为 `deploy.sh` 会在目录根部创建 `deploy.lock` 和
`deployments/`；脚本、Compose 文件和示例配置继续使用受限模式：

```bash
sudo install -d -o "$DEPLOY_USER" -g "$DEPLOY_GROUP" -m 750 /opt/coze-dev
sudo install -o root -g "$DEPLOY_GROUP" -m 750 \
  deploy/dev/deploy.sh /opt/coze-dev/deploy.sh
sudo install -o root -g "$DEPLOY_GROUP" -m 640 \
  deploy/dev/docker-compose.yml /opt/coze-dev/docker-compose.yml
sudo install -o root -g "$DEPLOY_GROUP" -m 640 \
  deploy/dev/.env.example /opt/coze-dev/.env.example
sudo -u "$DEPLOY_USER" test -w /opt/coze-dev
sudo -u "$DEPLOY_USER" test -x /opt/coze-dev/deploy.sh
sudo -u "$DEPLOY_USER" docker info >/dev/null
```

最后三条命令必须全部成功；实际 webhook 账号不仅需要读取部署文件，还必须拥有
目录写权限并能访问 Docker。若刚调整 Docker 用户组，需要让宝塔执行环境重新登录
或重启后再验证。`deploy.env` 和 `app.env` 按下文在服务器本地创建，归实际部署
账号所有并保持 `600`。`nsq-data` 是由 Docker 创建和管理的命名卷，不作为普通
目录复制到 `/opt/coze-dev`。

不要让无关账号获得 `app.env`、`deploy.env` 或 Docker socket 权限。

## 配置文件

以下所有权命令继续使用上文已确认的 `DEPLOY_USER` 和 `DEPLOY_GROUP`；若已打开新的
管理 shell，先重新执行账号确认与主组查询，不能猜测账号。

### deploy.env

`deploy.env` 只保存部署参数：

```bash
cd /opt/coze-dev
if [ ! -e deploy.env ]; then
  sudo install -o "$DEPLOY_USER" -g "$DEPLOY_GROUP" -m 600 \
    .env.example deploy.env
fi
sudo chown "$DEPLOY_USER:$DEPLOY_GROUP" deploy.env
sudo chmod 600 deploy.env
```

按实际 ACR 修改 `ACR_REGISTRY` 和 `ACR_NAMESPACE`。日常发布保持
`SERVER_IMAGE_TAG=dev`、`WEB_IMAGE_TAG=dev`。`DEPLOY_HEALTH_TIMEOUT_SECONDS`
必须是正整数。`deploy.env` 是服务器本地文件，不要提交或附到工单中。

`WEB_BIND_IP` 必须是合法 IPv4 地址，默认 `0.0.0.0`；`WEB_PORT` 必须是
`1` 到 `65535` 的整数，默认 `8888`。调试公网访问时需要同步放行安全组和主机
防火墙；自定义公网端口时三处使用同一个 `WEB_PORT`：`deploy.env`、放行规则和
后续宝塔反向代理上游。

使用服务器只读账号登录 ACR。登录动作必须由实际执行 webhook 的同一系统账号
完成；在管理 shell 中切换到该账号：

```bash
sudo -H -u "$DEPLOY_USER" bash
```

随后在该部署账号的 shell 中执行，并在登录完成后退出该 shell：

```bash
cd /opt/coze-dev
source deploy.env
read -r -p 'ACR pull username: ' ACR_PULL_USERNAME
read -r -s -p 'ACR pull password: ' ACR_PULL_PASSWORD
printf '\n'
printf '%s' "$ACR_PULL_PASSWORD" | docker login "$ACR_REGISTRY" \
  --username "$ACR_PULL_USERNAME" --password-stdin
unset ACR_PULL_PASSWORD
exit
```

不要在脚本、命令历史或宝塔日志中写入凭据字面值。

### app.env

单独创建 `/opt/coze-dev/app.env`，写入后端运行所需的远程服务地址和应用密钥：

```bash
cd /opt/coze-dev
if [ ! -e app.env ]; then
  sudo install -o "$DEPLOY_USER" -g "$DEPLOY_GROUP" -m 600 \
    /dev/null app.env
fi
sudo chown "$DEPLOY_USER:$DEPLOY_GROUP" app.env
sudo chmod 600 app.env
```

`app.env` 至少需要按目标版本核对 MySQL DSN、Redis、Elasticsearch 和对象存储
配置。远程地址不能继续使用 `mysql`、`redis`、`elasticsearch` 等本地 Compose
服务名，也不能误写服务器容器内的 `127.0.0.1`。对象存储 credential 只写入此
文件；后台配置能力合入后，按该能力的加密存储合同执行，不把 secret 回显到页面。

Compose 固定向后端注入 `COZE_MQ_TYPE=nsq` 和 `MQ_NAME_SERVER=nsqd:4150`，
不要在 `app.env` 重复配置消息队列。`REDIS_DB` 可省略，默认使用逻辑库 `0`；
显式值必须是非负十进制整数。Redis Cluster 或只支持 DB 0 的云实例必须保持
`REDIS_DB=0`。

`VECTOR_STORE_TYPE` 及 provider 专属变量可以全部省略。此时 Elasticsearch
全文检索继续工作，语义向量检索关闭；显式配置 `milvus`、`vikingdb` 或
`oceanbase` 后仍会严格校验并在依赖不可用时阻止启动。

`app.env` 不要求 `USE_SSL` 或 `SERVER_HOST`。容器内后端保持 HTTP；公网 URL
先在系统管理页面配置为公网 IP 与端口，宝塔域名启用后再改为最终 HTTPS 域名。

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

在宝塔站点启用 HTTPS，将流量代理到 Web 容器发布的同一宿主机端口。以下为默认
`WEB_BIND_IP=0.0.0.0`、`WEB_PORT=8888` 的上游示例：

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

根据业务上传上限配置站点的 `client_max_body_size`。自定义 `WEB_PORT` 时同步修改
`proxy_pass`；若 `WEB_BIND_IP` 改为宿主机的特定地址，上游也必须使用该可达地址，
不能继续假定回环地址已监听。

调试期可在安全组和主机防火墙中放行 `WEB_PORT`，最好限制来源 IP。该端口提供
明文 HTTP，会绕过宝塔的域名和 TLS 策略；正式域名启用后可关闭公网入站或继续
限制来源，公网业务流量只经宝塔 Nginx 的 HTTPS 端口进入。

## 发布流程

### 首次部署

首次自动启动前，确认远程数据库 schema 已经与 push 前的 `dev` 代码一致。随后：

1. 推送不含 `docker/atlas/migrations/**` 变化的目标提交到 `origin/dev`。
2. Actions 确认两张 `:dev` manifest 都不存在后，以 push 前 SHA 检查本次迁移
   变化，并构建、推送 `coze-server:dev-<full-sha>` 和
   `coze-web:dev-<full-sha>`。
3. Workflow 验证两张不可变镜像和 OCI revision，自动晋级两个 `:dev` 标签，再
   调用宝塔 webhook，无需手工运行 workflow。
4. 检查 Actions、`/healthz` 和 `/opt/coze-dev/deployments/current.env`。

如果首次 push 包含迁移，workflow 仍会进入 migration hold。先备份并手工执行
Atlas，再使用同一完整 SHA 运行 `workflow_dispatch`。只有一张 `:dev` 缺失、
registry 认证失败、超时或其他拉取错误也会保持 hold，不会被当作首次自动启动。

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

## NSQ 数据与排障

```bash
cd /opt/coze-dev
compose=(docker compose --env-file deploy.env -f docker-compose.yml)
"${compose[@]}" ps nsqd
"${compose[@]}" logs --tail=200 nsqd
"${compose[@]}" exec -T nsqd \
  wget -q -O - http://127.0.0.1:4151/ping
nsqd_container_id="$("${compose[@]}" ps -q nsqd)"
[ -n "$nsqd_container_id" ] || {
  printf '%s\n' 'nsqd container was not found' >&2
  exit 1
}
nsq_volume_name="$(
  docker inspect --format \
    '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' \
    "$nsqd_container_id"
)"
[ -n "$nsq_volume_name" ] || {
  printf '%s\n' 'nsqd /data named volume was not found' >&2
  exit 1
}
docker volume inspect "$nsq_volume_name"
docker system df -v
unset compose nsqd_container_id nsq_volume_name
```

健康响应必须是 `OK`。日常部署和应用镜像回滚只更新 `coze-server` 与
`coze-web`，保留 `nsqd` 和 `nsq-data`。禁止执行 `docker compose down -v`；
升级 NSQ、迁移卷或清理队列必须作为独立运维变更执行。单节点 NSQ 没有副本，
宿主机磁盘损坏或强制删除卷仍会丢失消息。

## 排障

以下 Bash 片段会 `source` 本机的 `deploy.sh` 和 `deploy.env`。只在确认它们是按
上述所有权和模式维护的可信本地文件后运行，不要 `source` 下载件或工单附件。该
过程加载镜像地址和端口配置、复用部署脚本的校验 helper，但不会打印环境文件内容。

```bash
cd /opt/coze-dev
source ./deploy.sh
set -a
source ./deploy.env
set +a

WEB_BIND_IP=${WEB_BIND_IP:-0.0.0.0}
WEB_PORT=${WEB_PORT:-8888}
if ! is_ipv4 "$WEB_BIND_IP"; then
  error 'WEB_BIND_IP must be a valid IPv4 address'
  exit 1
fi
if ! is_tcp_port "$WEB_PORT"; then
  error 'WEB_PORT must be an integer from 1 to 65535'
  exit 1
fi
if [ -z "${ACR_REGISTRY:-}" ] || [ -z "${ACR_NAMESPACE:-}" ]; then
  error 'ACR_REGISTRY and ACR_NAMESPACE are required'
  exit 1
fi
if ! validate_component "$ACR_REGISTRY" ||
  ! validate_component "$ACR_NAMESPACE"; then
  error 'ACR registry or namespace contains unsupported characters'
  exit 1
fi
export WEB_BIND_IP WEB_PORT

docker compose --env-file deploy.env -f docker-compose.yml ps
docker compose --env-file deploy.env -f docker-compose.yml logs --tail=200 nsqd coze-server coze-web
web_health_url="$(web_health_base_url)"
curl --fail --silent --show-error --output /dev/null \
  "${web_health_url}/healthz"
unset web_health_url
docker image inspect "$ACR_REGISTRY/$ACR_NAMESPACE/coze-server:dev" \
  --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}'
docker image inspect "$ACR_REGISTRY/$ACR_NAMESPACE/coze-web:dev" \
  --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}'
```

两个 revision 必须相同。查看 `deployments/failed-*.env` 中的失败原因和回滚结果，
不要把 `app.env`、Docker 登录配置或 GitHub Secrets 附到工单中。
