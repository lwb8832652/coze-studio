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
| Secret（Repository） | `ATLAS_URL` | Atlas MySQL URL，仅用于自动迁移 dev 数据库 |
| Secret（Repository，可选） | `ATLAS_CA_PEM` | Atlas 连接 dev MySQL 时使用的私有 CA PEM |
| Secret | `BAOTA_WEBHOOK_URL` | 宝塔预发布 webhook 地址 |
| Secret，可选 | `BAOTA_WEBHOOK_TOKEN` | webhook 请求头凭据 |
| Variable，可选 | `BAOTA_WEBHOOK_PINNED_PUBKEY` | 宝塔自签名证书的 curl SHA-256 公钥指纹 |

Workflow 的 `GITHUB_TOKEN` 只需要 `contents: read`。`ATLAS_URL` 和可选的
`ATLAS_CA_PEM` 必须配置为 GitHub Actions Repository Secret，不能配置为 Variable
或 Environment Secret。当前 `migrate` job 没有声明 GitHub Environment，因此
Environment Secret 不会生效。URL 使用以下占位格式，不要在仓库中填写真实值：

```text
mysql://USER:URL_ENCODED_PASSWORD@HOST:PORT/DATABASE?tls=true
```

密码中的保留字符必须做 URL 编码。数据库账号只授予目标 dev schema 执行仓库
migration 所需的最小权限，不使用云数据库管理账号，也不授予其他数据库权限。
MySQL 客户端默认不要求 TLS，但本 workflow 要求 URL 使用 `mysql://` scheme，并且
query 中只有一个明确的 `tls=true`；缺失、`tls=false` 或其他 scheme 都会在 Docker
启动前失败。不要把 `ATLAS_URL` 复制到 Variables、`app.env`、命令日志或工单。

服务端证书由公共可信 CA 签发时，不设置 `ATLAS_CA_PEM`，URL 也不声明 `ssl-ca`。
需要云厂商或私有 CA 时，把 PEM 存入 `ATLAS_CA_PEM`，并在 URL 中增加固定参数：

```text
mysql://USER:URL_ENCODED_PASSWORD@HOST:PORT/DATABASE?tls=true&ssl-ca=/atlas-ca.pem
```

Workflow 会把 Secret 写入 `$RUNNER_TEMP/atlas-ca.pem`，设置模式 `600`，只读挂载到
Atlas 容器的 `/atlas-ca.pem`，并在 step 退出时删除。URL 声明 `ssl-ca` 而 Secret
缺失，或 Secret 存在但 URL 没有精确指向 `/atlas-ca.pem`，都会在 Docker 前失败。
不要填写 runner 宿主机上的其他 CA 路径，也不能只填一个 `ssl-ca` 路径后假定文件
已经挂载。

Validate 和 apply 都固定使用
`arigaio/atlas:1.2.3-community-alpine@sha256:f44ca26436e7356832a45d84b8247e16638768b22cd2d97d3e84247ab48d0b1e`。
MySQL `ssl-ca` 是 Atlas v1.1 之后的能力；不得退回 `0.35.0`，也不得把完整引用缩短为
可变 tag。

`.github/atlas-dev.hcl` 通过 `getenv("ATLAS_URL")` 读取 DSN。Workflow 只在
`migrate` step 注入两个 Atlas Secret，并通过 `docker run --env ATLAS_URL` 传递
URL 的环境变量名；宿主机命令参数不包含 DSN 或 PEM。不要配置 SSH 私钥或把
`app.env` 内容放入 GitHub。

每次第二次集成审计都必须对本轮目标 dev MySQL 端点重新执行 TLS 探测，并记录结果，
不能复用历史结论。若端点不支持 SSL，必须先单独授权云侧开启 SSL、接受实例重启、
下载并核验本轮 CA，再更新两个 Repository Secret。常规 `dev` push 不包含这些
外部配置授权；前提完成前不得批准会触发 migration 的 push。

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

`ES_ADDR` 应使用与访问域名匹配、由系统信任 CA 签发的 HTTPS 证书。服务镜像
直接使用系统 CA bundle，不在 Compose 中挂载自签名 CA，也不能通过 `curl -k`
或其他方式跳过 TLS 校验。

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

`origin/dev` push 的正常 job 顺序固定为：

```text
preflight -> build-server/build-web -> verify-images -> migrate -> promote -> deploy
```

`build-server` 与 `build-web` 并行执行。`preflight` 分别输出
`migration_changed` 和 `deployment_blocked`；未知、缺失或无法验证的状态不会被
当成 migration。阻断状态会由 `deployment-blocked` job 明确失败，后续
`verify-images`、`migrate`、`promote` 和 `deploy` 不执行。

### 自动迁移前提

启用自动迁移前，远程 dev 数据库必须已有可信的 Atlas revision 基线。日常发布时，
现有 schema、已执行 migration 和 revision 记录必须与当前两张已晋级 `:dev` 镜像的
revision 一致；首次没有 `:dev` 镜像时，则必须与经过校验的 push `before` 一致。
第二次审计必须先从 ACR 读取两张当前 `:dev` 镜像的合法且一致 OCI revision，再按
`<deployed-revision>..<target-sha>` 检查全部 migration；首次部署改用已验证的
`<push-before>..<target-sha>`。这份完整区间中的文件才是该 push 的数据库授权范围，
不能只查看 `origin/dev...dev` 或目标提交新增的文件。Git 对象、祖先关系或 diff
任何一项无法证明时，不得请求推送确认。

#### 一次性 baseline

数据库已有 schema，但 Atlas revision 缺失或不一致时，先做一次受控 baseline，
不能让自动任务用 migration 目录重建已有对象。操作前须在单独授权的只读检查中，
对 `<AUTHORIZED_DEV_DATABASE>` 运行 `atlas migrate status`，并逐项核对实际 schema、
现有对象和 migration 内容。最终 baseline 版本只能根据这些 schema 证据选择。

`--baseline <MIGRATION_VERSION_SELECTED_FROM_SCHEMA_EVIDENCE>` 的值是 migration
文件名开头的版本时间戳，不是 Git SHA。Baseline 会写 Atlas revision 记录，属于
数据库变更，必须另行取得精确的数据库操作授权；常规 push 和推送确认不能替代这项
授权。这里的尖括号值都是审计占位符，不是可直接执行的远程数据库命令。

#### Runner 网络

`migrate` 当前保持 `runs-on: ubuntu-latest`。标准 GitHub-hosted runner 的出口地址
范围多且会变化，不建议把 GitHub 公布的整段地址加入数据库白名单，更不能用
`0.0.0.0/0` 作为“已验证连通”。只有 dev 数据库已经通过受控网络策略安全可达，并且
在实际 `ubuntu-latest` runner 上完成本次连通性验证后，才能启用自动迁移。

需要稳定白名单时，应使用已经配置和加固的 self-hosted runner，或支持静态出口 IP
的 GitHub larger runner。修改 `runs-on` 前必须另做安全、容量、凭据和网络审计；本
流程不会擅自切换到一个尚未配置的 runner。`ATLAS_URL` 缺失、TLS/CA 合同不成立、
网络不可达、目录 validate 失败或 apply 失败都会终止 `migrate`；两张 `:dev` 标签
不晋级，宝塔 webhook 不调用。Workflow 不调用数据库备份 API，腾讯云备份策略独立
配置和核验。

### 首次部署

完成上述 Atlas revision baseline 后，首次 push 也按完整 job 顺序执行：

1. 推送目标提交到 `origin/dev`。首次 push 可以包含新的 forward migration。
2. Actions 确认两张 `:dev` manifest 都不存在后，以 push 前 SHA 检查本次迁移
   变化，并构建、推送 `coze-server:dev-<full-sha>` 和
   `coze-web:dev-<full-sha>`。
3. `verify-images` 确认两张不可变镜像的 OCI revision 都等于目标 SHA。
4. 确有 migration 时，`migrate` 先 validate 目录，再自动 apply；没有 migration
   时记录成功的 no-op。
5. `migrate` 成功后，`promote` 晋级两张 `:dev` 标签，`deploy` 调用宝塔 webhook。
6. 检查 Actions、`/healthz` 和 `/opt/coze-dev/deployments/current.env`。

只有一张 `:dev` manifest 缺失、registry 认证失败、镜像 inspect 失败或 push
`before` 无法验证时，不进入首次启动路径。不可变镜像可能仍会构建，但
`deployment-blocked` 会明确失败，数据库、`:dev` 标签和运行服务不变。

### 日常发布

日常 push 使用相同的完整 job 顺序。`preflight` 要求当前两张 `:dev` 镜像具有相同
且可验证的 revision，并证明该 revision 是目标 SHA 的祖先；随后比较完整区间内的
`docker/atlas/migrations`。没有 migration 时 `migrate` 成功 no-op；确有 migration
时自动 validate 和 apply。只有不可变镜像、Atlas 和双标签晋级都成功后才调用
webhook。服务器会再次校验双 revision，共同更新两个服务，并在记录成功前核对
两个容器的实际 image ID。

推送前的第二次审计使用与 workflow 相同的部署区间，逐个列出全部待执行 migration、
数据库副作用，以及旧应用继续运行在迁移后 schema 上的兼容性证据。用户授权只覆盖
报告中的目标 SHA、这些 migration 文件和逐项副作用；部署基线变化后必须重新审计。

进入 `dev` 的 forward migration 必须兼容发布前应用。Atlas 成功后，镜像晋级或
宝塔部署仍可能失败，旧代码会在恢复完成前继续连接已经迁移的 schema。

### `workflow_dispatch` 边界

`workflow_dispatch` 只重放已经构建、仍位于 `origin/dev` 历史中的完整 SHA；它不
重新构建镜像，也不执行 Atlas 或 down migration。Preflight 先要求当前两张
`:dev` 镜像的 revision 存在且一致，再按以下四类关系处理：

| 目标 SHA 与当前 `dev` revision 的关系 | 结果 |
| --- | --- |
| 两者相同 | 允许重试已经晋级版本的部署 |
| 目标 SHA 是当前 revision 的祖先 | 允许应用镜像回滚，不执行 down migration |
| 目标 SHA 是当前 revision 的后代，且区间没有 migration | 允许前向重放 |
| 前向区间含 migration、关系无法证明，或当前双 revision 异常 | 阻断并由 `deployment-blocked` 明确失败 |

允许的 dispatch 仍会验证两张 `dev-<full-sha>` 不可变镜像及其 OCI revision，之后
才晋级双标签并调用 webhook。

### 失败与重试

- `migrate` 失败后不得直接选择 `Re-run failed jobs`。先在不泄露 DSN 的前提下，
  通过单独授权的 `atlas migrate status` 检查和 Actions/数据库证据，区分网络瞬断
  与 checksum、SQL、数据、schema drift 或部分执行。只有确认是可重试的瞬态故障，
  才能在同一个 run 重跑失败 job。
- 确定性 migration 失败必须修正 migration 后重新走完整审计；需要补数据、修 schema
  或 revision 的数据库处理必须单独授权。不得用新建 `workflow_dispatch` 绕过原
  push，因为 dispatch 不执行 Atlas。
- `promote` 只晋级一张标签时，仍在同一个 Actions run 使用 `Re-run failed jobs`
  完成双标签晋级；部分晋级会因双 revision 不一致而阻断 dispatch。
- 两张 `:dev` 标签都已晋级，但 `deploy` 的 webhook 或服务器部署失败时，可以对
  同一 SHA 运行 `workflow_dispatch`。它只重新验证、晋级和部署，不执行 Atlas。
- `deploy` job 的超时是 15 分钟。curl 连接超时为 10 秒，总请求窗口为 840 秒；
  超时或非成功响应都会让 job 失败。

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
