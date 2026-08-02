# Dev Compose 本地 NSQ 与运行配置优化设计

## 1. 目标

优化 `deploy/dev/docker-compose.yml`，让 2C4G Linux 预发布服务器在继续使用远程
MySQL、Redis、Elasticsearch 和对象存储的同时，在同一 Compose 项目中运行轻量、
持久化的消息队列。后端不再要求 `app.env` 提供消息队列地址，也允许完全省略向量
数据库配置。Web 服务需要通过公网 IP 和可配置端口直接访问，域名与 HTTPS 由宝塔
反向代理独立管理；远程 Redis 可以选择逻辑数据库。

本设计只调整 `dev` 单实例部署拓扑、公开访问配置、Redis 选库和向量存储的未配置
语义，不把完整本地开发栈搬到服务器。

## 2. 当前事实

- `deploy/dev/docker-compose.yml` 目前只运行 `coze-server` 和 `coze-web`。
- `coze-web` 目前只绑定 `127.0.0.1:8888`，公网 IP 不能直接访问该端口。
- 后端消息队列已支持 NSQ，并通过 `ConnectToNSQD` 直接连接 `MQ_NAME_SERVER`，不依赖
  `nsqlookupd`。
- 完整开发 Compose 使用 `nsqio/nsq:v1.2.1`，同时启动 `nsqlookupd`、`nsqd` 和
  `nsqadmin`；该拓扑对单机 2C4G 预发布环境过重。
- `deploy.sh` 只更新和校验两张业务镜像。Compose 启动指定业务服务时会同时启动其
  必需依赖。
- 向量搜索初始化始终保留 Elasticsearch 全文搜索管理器，并根据
  `VECTOR_STORE_TYPE` 创建第二个向量管理器。
- 现有代码已经支持 `none`、`noop`、`disabled` 和调试跳过开关，但
  `VECTOR_STORE_TYPE` 为空时会返回错误并阻止应用启动。
- 现有 noop 向量管理器允许知识库创建、写入和删除流程继续执行，向量检索返回空
  结果；Elasticsearch 全文检索仍可工作。
- `USE_SSL` 只控制后端 Hertz 是否直接加载证书，未设置时已默认关闭。
- `SERVER_HOST` 不控制监听地址，它用于 OAuth 回调、工作流调试和 OpenAPI 等公开
  URL；系统管理页面已经支持持久化修改该值。
- Redis client 当前固定选择逻辑库 `0`，`app.env` 没有可用的数据库编号配置。

## 3. 已确认决策

- 消息队列采用单节点、独立运行的 `nsqd`，不部署 `nsqlookupd` 和 `nsqadmin`。
- 使用固定镜像版本 `nsqio/nsq:v1.3.0`，不使用浮动 `latest`。
- NSQ 消息直接写入 Docker 命名卷，优先降低内存占用并保证正常重启后的队列连续
  性。
- NSQ 端口只在 Docker 网络中可见，不映射到宿主机和公网。
- Compose 固定向后端注入 NSQ 类型和容器内地址，`app.env` 不再配置消息队列。
- `VECTOR_STORE_TYPE` 未设置、为空或仅包含空白时，等价于关闭向量存储；显式启用
  某个 provider 时仍严格初始化并在配置错误时阻止启动。
- Web 默认绑定宿主机所有网络接口的 `8888` 端口，并允许通过 `deploy.env` 修改绑定
  地址和端口，以满足公网 `IP:端口` 访问。
- 应用容器不终止 TLS，`app.env` 不要求 `USE_SSL`；宝塔负责域名证书和 HTTPS。
- `app.env` 不要求 `SERVER_HOST`。公网 IP 或域名确定后，在系统管理页面配置当前
  对外 URL。
- `app.env` 支持可选 `REDIS_DB`，未设置时使用 `0`，非法值不得回退到默认库。
- 不引入本地 Milvus、VikingDB、OceanBase、消息队列管理 UI 或高可用组件。

## 4. 部署架构

Compose 包含三个服务：

```text
public IP:WEB_PORT -----------------------> coze-web -> coze-server
Baota HTTPS domain -> 127.0.0.1:WEB_PORT -----^             |
                                                              +-> nsqd:4150

nsqd -> Docker named volume: nsq-data
coze-server -> remote MySQL / Redis / Elasticsearch / object storage
```

`coze-web` 通过宿主机可配置端口对外发布。`coze-server` 和 `nsqd` 只加入
`coze-dev` bridge 网络，不映射宿主机端口。NSQ 的 HTTP 端口 `4151` 仅用于容器内
健康检查。

## 5. NSQ 服务合同

### 5.1 容器与持久化

新增 `nsqd` 服务：

- 镜像固定为 `nsqio/nsq:v1.3.0`；
- 启动命令包含 `/nsqd`、`--data-path=/data` 和 `--mem-queue-size=0`；
- 命名卷 `nsq-data` 挂载到 `/data`；
- `restart` 使用 `unless-stopped`；
- `stop_grace_period` 为 30 秒，给磁盘队列正常关闭留出时间；
- `mem_limit` 为 `384m`、`cpus` 为 `0.50`、`pids_limit` 为 `128`，避免队列
  挤占应用与宿主机资源；
- `pull_policy` 为 `missing`，首次自动拉取固定版本，普通业务发布不重复拉取。

`--mem-queue-size=0` 表示消息不保留在 NSQ 的内存队列中，而是进入磁盘队列。这会
增加少量磁盘 I/O，但更适合当前低到中等流量、内存受限且要求正常重启后继续消费
的预发布环境。

该方案不承诺高可用或严格零丢失：单节点 NSQ 没有副本，宿主机磁盘损坏、强制
删除命名卷或未刷盘的异常断电仍可能导致数据丢失。`docker compose down -v` 不得
用于日常部署。

### 5.2 健康与依赖

NSQ 健康检查访问 `http://127.0.0.1:4151/ping` 并要求响应为 `OK`。
`coze-server` 使用长格式 `depends_on` 等待 `nsqd` 健康后启动。

Compose 向 `coze-server` 固定注入：

```text
COZE_MQ_TYPE=nsq
MQ_NAME_SERVER=nsqd:4150
```

这两个变量属于部署拓扑，不属于业务 secret。它们不写入 `app.env`，也不暴露为
`deploy.env` 的可选覆盖项，避免服务器误连外部或宿主机回环地址。

NSQ 运行中短暂重启时由现有 Go NSQ client 负责重连。NSQ 长期不健康时，容器健康
状态和应用日志必须可见，部署不得把该状态记录为成功。

### 5.3 资源与日志

三个服务统一使用 Docker `json-file` 日志轮转，单文件最大 10 MiB，最多保留 3
个文件。只对新增 NSQ 设置硬资源上限；本次不凭经验压缩 `coze-server` 的内存，
避免在尚无生产指标时制造 OOM 回归。

NSQ 数据卷不计入日志轮转。运维需单独监控 Docker 数据目录磁盘占用，磁盘空间不
足时先停止写入并扩容或受控清理，不能直接删除活动队列文件。

## 6. 可选向量数据库合同

`getVectorStore` 对标准化后的 `VECTOR_STORE_TYPE` 使用以下规则：

| 配置值 | 启动行为 |
| --- | --- |
| 未设置、空白、`none`、`noop`、`disabled` | 返回 noop 向量管理器，应用继续启动 |
| `milvus`、`vikingdb`、`oceanbase` | 按现有 provider 严格初始化 |
| 其他值 | 返回明确错误并阻止启动 |

未配置向量库时记录一条不包含配置内容或凭据的 warning，说明语义向量检索已关闭。
该模式下：

- `app.env` 可以省略 `VECTOR_STORE_TYPE` 和所有 provider 专属变量；
- Elasticsearch 全文索引和检索继续可用；
- 向量写入由 noop 实现接收，向量检索不返回结果；
- 后续在 `app.env` 显式配置受支持 provider 后，重启服务即可恢复严格的向量能力；
- 显式启用 provider 但地址或凭据错误时不得静默回退到 noop。

保留 `COZE_DEBUG_SKIP_VECTOR_STORE` 供现有调试流程兼容，但正常部署不依赖该调试
开关。

## 7. 公网访问、TLS 与公开 URL

`coze-web` 使用以下端口合同：

```text
${WEB_BIND_IP:-0.0.0.0}:${WEB_PORT:-8888}:80
```

`deploy.env` 增加非敏感配置 `WEB_BIND_IP=0.0.0.0` 和 `WEB_PORT=8888`。默认情况下，
安全组与主机防火墙放行后可以通过 `http://<公网 IP>:8888` 访问；宝塔仍可反向代理
到 `http://127.0.0.1:8888`。`WEB_BIND_IP` 必须是宿主机可绑定的 IP，`WEB_PORT`
必须是 `1` 到 `65535` 的整数，Compose 合同和部署前检查拒绝无效值。

公网端口提供的是 HTTP，会绕过宝塔证书和 HTTPS 策略。运维应限制来源 IP 或仅在
调试期开放；正式域名流量使用宝塔 HTTPS。`coze-server` 不映射公网端口。

后端不配置 `USE_SSL`，沿用未设置时的默认关闭行为。`SERVER_HOST` 也不作为
`app.env` 必填项，因为它不控制监听地址：

- 仅使用公网 IP 时，在系统管理页面配置 `http://<公网 IP>:<WEB_PORT>`；
- 宝塔域名启用后，将系统配置改为最终的 `https://<域名>`；
- 未配置时应用仍可访问，但 OAuth 回调、工作流调试和 OpenAPI 等生成 URL 会回退
  到 `http://127.0.0.1:8888`，因此启用这些能力前必须完成系统配置。

环境变量 `SERVER_HOST` 保留为旧配置的兼容启动来源，本次不删除后端兼容代码，
但 dev 部署手册不再要求把它写入 `app.env`。

## 8. Redis 逻辑数据库合同

`app.env` 新增可选 `REDIS_DB`：

- 未设置或空白时使用逻辑库 `0`；
- 设置时必须是非负十进制整数；
- 非数字或负数属于启动配置错误，不得静默回退到 `0`；
- client 建立后执行最长 5 秒的 readiness 检查，云实例不支持所选逻辑库时启动
  失败；
- Redis Cluster 或只支持 DB 0 的云实例保持 `REDIS_DB=0`。

继续使用现有 `REDIS_ADDR` 和 `REDIS_PASSWORD`，不把数据库编号编码进地址，也不
新增第二套 Redis client。无数据库参数的 `NewWithAddrAndPassword` 测试辅助入口
保持默认 DB 0；生产环境入口解析并校验 `REDIS_DB`。

## 9. 发布与回滚

`deploy.sh` 继续把 `coze-server` 和 `coze-web` 作为同一业务发布事务：拉取、OCI
revision 校验、更新、健康检查和镜像回滚都只针对这两张业务镜像。

`nsqd` 是固定版本的基础依赖：首次 `docker compose up` 时自动拉取并启动，后续
业务发布不重建、不晋级标签，也不参与业务镜像 revision 比较。部署脚本在写入
成功记录前必须额外确认 `nsqd` 的 Compose health 状态为 `healthy`。应用部署失败
回滚时保留正在运行的 NSQ 和 `nsq-data`，避免回滚动作丢弃排队消息。

升级 NSQ 版本、迁移数据卷或清理队列必须作为独立运维变更执行，不借普通应用发布
隐式完成。

服务器 `/opt/coze-dev` 仍只需安装：

```text
.env.example
docker-compose.yml
deploy.sh
deploy.env
app.env
```

`nsq-data` 由 Docker 自动创建和管理，不复制成普通目录文件。`app.env` 仍保存远程
MySQL、Redis、Elasticsearch、对象存储和应用密钥，但消息队列和向量数据库均不再
是必填项。

## 10. 错误处理

- NSQ 镜像无法拉取或健康检查失败：Compose 更新失败，部署脚本返回失败并按现有
  流程恢复业务镜像；不得写入成功部署记录。
- NSQ 数据目录不可写或磁盘已满：NSQ 保持不健康或退出，由 restart policy 重试，
  运维根据容器日志和磁盘监控处理；不得自动删除数据。
- `app.env` 显式选择无效向量类型：后端启动失败并给出类型错误。
- `app.env` 显式选择有效 provider 但依赖不可用：后端启动失败，不降级。
- `app.env` 未设置向量类型：后端正常启动，warning 明确说明降级范围。
- `WEB_BIND_IP` 或 `WEB_PORT` 无效：部署在停止现有容器前失败。
- `REDIS_DB` 格式错误、为负数或云实例拒绝所选逻辑库：后端启动失败，不回退到
  DB 0。

## 11. 测试与验证

### 11.1 Compose 合同测试

扩展 `deploy/dev/tests/compose_contract_test.sh`，至少验证：

- 服务集合包含 `nsqd`、`coze-server` 和 `coze-web`；
- NSQ 镜像版本固定，命令启用 `/data` 和全磁盘队列；
- `nsq-data` 命名卷存在且正确挂载；
- NSQ 没有宿主机端口映射，健康检查访问 `/ping`；
- NSQ 的 `384m` 内存、`0.50` CPU、`128` 进程和 `missing` 拉取策略固定；
- `coze-server` 等待健康 NSQ，并收到固定的两个 MQ 环境变量；
- 三个服务日志轮转配置一致；
- Web 默认发布到 `0.0.0.0:8888`，自定义绑定 IP 与端口能正确渲染；
- 应用健康检查和只读 `app.env` 挂载合同不回归。

### 11.2 后端单元测试

在向量存储实现包新增测试，覆盖：

- 未设置和空白 `VECTOR_STORE_TYPE` 返回 noop 管理器且不报错；
- `none` 继续返回 noop 管理器；
- 未知类型继续报错；
- 测试路径不连接任何真实向量数据库。

Redis 配置测试覆盖：

- 未设置和空白 `REDIS_DB` 使用 DB 0；
- 合法非负整数传入 go-redis client；
- 非数字和负数返回配置错误；
- readiness 失败会传播到应用启动，不回退到 DB 0。

### 11.3 静态与运行验证

- `docker compose config` 渲染通过；
- `bash deploy/dev/tests/compose_contract_test.sh` 通过；
- 相关 Go package 测试通过；
- 部署脚本测试覆盖 NSQ 非 `healthy` 时不写成功记录；
- Docker 可用时启动独立 NSQ，确认 `/ping`、数据卷、服务依赖和重启后消息连续性；
- 更新部署手册，核对首次部署、排障、磁盘监控和禁止 `down -v` 的说明。

### 11.4 本地完整启动验收

实现完成后，使用用户提供的远程 MySQL、Redis 和 Elasticsearch 配置，结合其余
必需的本地应用配置执行一次完整启动验收：

- credential 只写入 `/private/tmp` 下权限为 `600` 的临时环境文件，不写入仓库、
  shell 命令参数、测试快照或日志；
- 不在输出中回显 DSN、密码或完整环境文件；
- 先验证远程端口与认证，再启动 NSQ、后端和 Web；
- 验证 `http://127.0.0.1:<WEB_PORT>/healthz`、首页以及公网绑定的实际监听状态；
- 验证后端 `/healthz` 经过全局认证中间件时，无 Session 请求仍返回 200；
- 验证后端使用指定 `REDIS_DB`，并用唯一临时键完成一次不涉及业务数据的读写删除
  探测；
- 验收结束后删除临时凭据文件和测试键，不删除 NSQ 持久化卷。

远程依赖只参与这次受控手工验收，不进入可重复单元测试和 CI。宝塔服务器不参与
本地测试。

## 12. 文档与长期事实

实现时同步更新：

- `deploy/dev/README.md`：运行拓扑、`app.env` 合同、公网端口、宝塔 TLS、Redis DB、
  安装、首次启动、排障和数据卷；
- `docs/superpowers/context/project-context.md`：dev 服务器现在包含持久化单节点 NSQ，
  向量数据库可选；
- 原自动部署设计中的“两容器”和“消息队列、向量库均写入 app.env”事实由本文档
  覆盖，不回写历史设计。

## 13. 不在本次范围内

- NSQ 高可用、跨主机复制、`nsqlookupd` 或 `nsqadmin`；
- 消息队列公网访问、TLS 或账号认证；
- 自动备份、恢复或迁移 NSQ 数据；
- 本地部署 MySQL、Redis、Elasticsearch、对象存储或向量数据库；
- 修改知识库页面以隐藏、禁用或说明向量能力；
- 在应用容器中配置域名证书或 HTTPS；
- 为公网调试端口增加独立认证层；
- 修改正式生产环境拓扑；
- 自动执行远程数据库迁移。

## 14. 验收标准

- 仅提供远程数据库、Redis、Elasticsearch、对象存储和应用密钥的 `app.env` 时，
  三个 Compose 服务可以按依赖顺序启动。
- 后端实际使用 `nsq` 和 `nsqd:4150`，无需在 `app.env` 重复配置。
- NSQ 只在内部网络监听，消息进入 `nsq-data`，正常容器重建后数据卷保留。
- NSQ 不健康时后端不会被 Compose 视为就绪，部署不会记录成功。
- 省略全部向量数据库变量时后端正常启动，Elasticsearch 全文检索保留，向量检索
  返回空结果并记录关闭 warning。
- 显式配置向量 provider 失败时启动仍然 fail closed。
- 默认可以通过 `http://<公网 IP>:8888` 访问 Web，修改 `WEB_BIND_IP` 或
  `WEB_PORT` 后 Compose 正确发布目标地址。
- `app.env` 不配置 `USE_SSL` 和 `SERVER_HOST` 时服务可以启动；公开 URL 通过系统
  管理页面独立配置。
- `REDIS_DB` 未设置时使用 0，合法编号生效，非法编号或云实例拒绝时启动失败。
- 日常业务部署和回滚不会删除、重建或改变 NSQ 数据卷。
- Compose 合同测试、相关 Go 测试和文档检查全部通过，并使用受控临时配置完成一次
  本地完整启动验收。
