# Dev Automated Deployment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在代码进入远程 `dev` 后，为后端和前端构建可追溯镜像并推送阿里云 ACR；无数据库迁移时由 GitHub Actions 晋级 `dev` 标签并触发宝塔 webhook，服务器以双服务事务方式更新，健康检查失败自动回滚。

**Architecture:** GitHub Actions 只负责构建、镜像晋级和调用固定 webhook；服务器上的 `deploy.sh` 持有部署锁，校验两张镜像的 OCI revision 一致后再更新。数据库迁移采用 fail-closed 人工门禁：检测到迁移变化时只产出不可变镜像，人工迁移完成后通过指定完整 SHA 的 `workflow_dispatch` 晋级和部署。

**Tech Stack:** GitHub Actions、Docker Buildx、Docker Compose、Bash、Go/Hertz、Nginx、Alibaba Cloud ACR、Baota webhook、Atlas migrations。

---

## Scope And File Map

本计划新增或修改以下文件：

- 新增 `backend/api/handler/coze/health_service.go`
- 新增 `backend/api/handler/coze/health_service_test.go`
- 新增 `backend/api/router/coze/health_route_test.go`
- 修改 `backend/api/router/coze/custom_routes.go`
- 修改 `backend/Dockerfile`
- 修改 `frontend/Dockerfile`
- 新增 `deploy/dev/nginx/nginx.conf`
- 新增 `deploy/dev/nginx/default.conf`
- 新增 `deploy/dev/docker-compose.yml`
- 新增 `deploy/dev/.env.example`
- 新增 `deploy/dev/deploy.sh`
- 新增 `deploy/dev/tests/image_contract_test.sh`
- 新增 `deploy/dev/tests/compose_contract_test.sh`
- 新增 `deploy/dev/tests/deploy_test.sh`
- 新增 `deploy/dev/tests/workflow_contract_test.sh`
- 新增 `.github/workflows/deploy-dev.yml`
- 新增 `deploy/dev/README.md`
- 修改 `docs/superpowers/context/project-context.md`
- 修改 `docs/superpowers/runbooks/dev-integration-audit.md`

不在本计划范围内：

- 不创建或修改阿里云 ACR 仓库、账号和权限。
- 不在宝塔中创建 webhook 或修改线上 Nginx。
- 不执行 Atlas migration apply。
- 不合并本地 `dev`，不推送远程 `dev`。
- 不把对象存储控制面分支合入当前分支。

## Task 1: Add A Public Revision Health Endpoint

**Files:**

- Create: `backend/api/handler/coze/health_service_test.go`
- Create: `backend/api/router/coze/health_route_test.go`
- Create: `backend/api/handler/coze/health_service.go`
- Modify: `backend/api/router/coze/custom_routes.go`

- [ ] **Step 1: Write the failing handler test**

测试设置 `APP_REVISION` 为固定 40 位 SHA，调用 `GetHealthz`，断言 HTTP 200，并解析 JSON 后严格比较：

    {
      "status": "ok",
      "revision": "0123456789abcdef0123456789abcdef01234567"
    }

同时增加一个未设置 `APP_REVISION` 的用例，断言接口仍返回 200，revision 为空字符串。使用 `t.Setenv`，避免污染其他测试。

- [ ] **Step 2: Write the failing public-route test**

按 `admin_auth_route_test.go` 的 Hertz 测试模式创建服务，注册 `RegisterCustomRoutes`，不附带认证信息请求 `GET /healthz`。断言状态码 200 和响应字段，证明该路由没有落入认证路由组。

- [ ] **Step 3: Run the RED tests**

Run:

    cd backend
    go test ./api/handler/coze ./api/router/coze -run 'TestGetHealthz|TestRegisterCustomRoutesPublishesPublicHealthz' -count=1

Expected: FAIL，原因是 handler 和路由尚不存在。

- [ ] **Step 4: Implement the minimal handler**

`health_service.go` 使用现有 Hertz handler 风格：

    func GetHealthz(ctx context.Context, c *app.RequestContext) {
        revision := strings.TrimSpace(os.Getenv("APP_REVISION"))
        c.JSON(consts.StatusOK, map[string]string{
            "status":   "ok",
            "revision": revision,
        })
    }

保持接口无数据库查询、无外部调用。由于服务只在 `application.Init` 成功后启动监听，该接口可作为启动完成后的存活与发布版本探针。

- [ ] **Step 5: Register the route before authenticated groups**

在 `RegisterCustomRoutes` 顶部、现有公开站点配置路由附近加入：

    r.GET("/healthz", handler.GetHealthz)

- [ ] **Step 6: Run focused and neighboring tests**

Run:

    cd backend
    go test ./api/handler/coze ./api/router/coze -run 'TestGetHealthz|TestRegisterCustomRoutesPublishesPublicHealthz|TestAdminAuth|TestWorkspace' -count=1

Expected: PASS。

- [ ] **Step 7: Commit**

    git add backend/api/handler/coze/health_service.go backend/api/handler/coze/health_service_test.go backend/api/router/coze/health_route_test.go backend/api/router/coze/custom_routes.go
    git commit -m "feat: add deployment health endpoint"

## Task 2: Make Images Revisioned And Self-Contained

**Files:**

- Create: `deploy/dev/tests/image_contract_test.sh`
- Create: `deploy/dev/nginx/nginx.conf`
- Create: `deploy/dev/nginx/default.conf`
- Modify: `backend/Dockerfile`
- Modify: `frontend/Dockerfile`

- [ ] **Step 1: Write the failing image contract test**

`image_contract_test.sh` 使用 `set -euo pipefail` 和仓库根目录绝对定位，验证：

- 两个 Dockerfile 都声明 `ARG GIT_REVISION` 和 `ARG SOURCE_URL`。
- 最终运行阶段都写入 `org.opencontainers.image.revision` 与 `org.opencontainers.image.source`。
- 后端运行阶段设置 `APP_REVISION`。
- 前端镜像复制本计划的 Nginx 配置并暴露 80。
- Nginx 将 `/healthz` 代理到 `coze-server:8888`。
- 新 Nginx 配置不包含 `minio`、`local_storage` 或本地对象存储替换逻辑。

- [ ] **Step 2: Run the RED test**

Run:

    bash deploy/dev/tests/image_contract_test.sh

Expected: FAIL，因为 Dockerfile 标签和部署 Nginx 配置尚不存在。

- [ ] **Step 3: Add deterministic OCI metadata**

在两个 Dockerfile 的第一个 `FROM` 前添加：

    ARG GIT_REVISION=unknown
    ARG SOURCE_URL=unknown

在各自最终运行阶段重新声明这两个参数，并添加：

    LABEL org.opencontainers.image.revision=$GIT_REVISION
    LABEL org.opencontainers.image.source=$SOURCE_URL

后端最终阶段额外添加：

    ENV APP_REVISION=$GIT_REVISION

构建工作流必须总是传入完整 40 位提交 SHA，默认 `unknown` 只服务本地构建。

- [ ] **Step 4: Bake the deployment Nginx configuration into web image**

`deploy/dev/nginx/nginx.conf` 保留精简的 worker、日志、gzip 和 `conf.d` 加载配置。

`deploy/dev/nginx/default.conf`：

- 监听 80。
- SPA 静态文件使用 `try_files` 回退到 `/index.html`。
- `/healthz`、`/api/`、`/v1/`、`/open_api/` 等现有 API 前缀代理到 `http://coze-server:8888`。
- 保留现有上传大小和代理超时语义。
- 不加入 MinIO、本地存储路径或对象存储 URL 重写。

`frontend/Dockerfile` 将两份配置复制进 Nginx 镜像，并把 `EXPOSE 8888` 更正为 `EXPOSE 80`。

- [ ] **Step 5: Run the GREEN contract test**

Run:

    bash deploy/dev/tests/image_contract_test.sh

Expected: PASS。

- [ ] **Step 6: Commit**

    git add backend/Dockerfile frontend/Dockerfile deploy/dev/nginx deploy/dev/tests/image_contract_test.sh
    git commit -m "build: add revisioned deployment images"

## Task 3: Add A Two-Service Deployment Compose

**Files:**

- Create: `deploy/dev/tests/compose_contract_test.sh`
- Create: `deploy/dev/docker-compose.yml`
- Create: `deploy/dev/.env.example`

- [ ] **Step 1: Write the failing Compose contract test**

测试先调用 `docker compose -f deploy/dev/docker-compose.yml config --services`，严格断言只有：

    coze-server
    coze-web

再检查：

- 镜像分别来自 `ACR_REGISTRY/ACR_NAMESPACE/coze-server` 和 `coze-web`。
- 默认标签为 `dev`，可由部署脚本临时覆盖。
- 后端以只读方式挂载 `./app.env:/app/.env`。
- 后端健康检查访问 `http://127.0.0.1:8888/healthz`。
- Web 仅绑定 `127.0.0.1:8888:80`。
- Web 依赖后端 healthy。
- Compose 中没有 MySQL、Redis、Elasticsearch、MinIO、Milvus、Etcd 或 NSQ 服务。

- [ ] **Step 2: Run the RED test**

Run:

    bash deploy/dev/tests/compose_contract_test.sh

Expected: FAIL，因为部署 Compose 尚不存在。

- [ ] **Step 3: Implement the deployment Compose**

`docker-compose.yml` 使用一个私有 bridge 网络。服务约束：

- `coze-server` 使用 `restart: unless-stopped`，只 `expose: 8888`，不映射宿主机端口。
- `coze-server` 的健康检查使用镜像中已有的 `curl`。
- `coze-web` 使用 `restart: unless-stopped`，绑定 `127.0.0.1:8888:80`。
- `coze-web` 健康检查使用 Nginx 镜像中的 `wget` 请求自身 `/healthz`。
- 两个镜像标签读取 `SERVER_IMAGE_TAG` 和 `WEB_IMAGE_TAG`，默认值都是 `dev`。

`.env.example` 只放非敏感部署参数：

    ACR_REGISTRY=registry.example.aliyuncs.com
    ACR_NAMESPACE=example
    SERVER_IMAGE_TAG=dev
    WEB_IMAGE_TAG=dev
    DEPLOY_HEALTH_TIMEOUT_SECONDS=120

业务密钥保存在服务器本地 `app.env`，不得进入模板、Compose environment 或 Git。

- [ ] **Step 4: Run the GREEN test**

Run:

    bash deploy/dev/tests/compose_contract_test.sh

Expected: PASS。

- [ ] **Step 5: Commit**

    git add deploy/dev/docker-compose.yml deploy/dev/.env.example deploy/dev/tests/compose_contract_test.sh
    git commit -m "ops: add dev deployment compose"

## Task 4: Implement Atomic Deploy And Rollback

**Files:**

- Create: `deploy/dev/tests/deploy_test.sh`
- Create: `deploy/dev/deploy.sh`

- [ ] **Step 1: Write a shell test harness**

`deploy_test.sh` 在临时目录中创建假的 `docker`、`curl` 和状态文件，通过 PATH 注入记录命令与控制返回值。测试通过执行真实 `deploy.sh` 验证外部行为，不连接 Docker daemon。

至少覆盖以下场景：

- 两张候选镜像 revision 不一致时，在 `compose up` 前失败。
- 两张候选镜像 revision 相同且健康检查通过时，写入 `deployments/current.env`。
- 新版本健康检查失败时，两张旧 image ID 都被重新打上独立的本地 rollback 标签并共同恢复。
- 回滚成功后部署命令仍返回非零，避免 GitHub 把失败发布标成成功。
- 传入完整 SHA 时，候选 revision 与参数不一致则拒绝部署。
- `deploy.lock` 已被占用时立即失败，不并发执行第二次部署。
- 日志不包含 ACR 密码、业务环境文件内容或 webhook token。

- [ ] **Step 2: Run the RED test**

Run:

    bash deploy/dev/tests/deploy_test.sh

Expected: FAIL，因为 `deploy.sh` 尚不存在。

- [ ] **Step 3: Implement testable command wrappers**

`deploy.sh` 使用 `set -Eeuo pipefail`，并定义可由测试替换的函数：

- `docker_cmd`
- `compose_cmd`
- `wait_for_health`
- `image_revision`
- `container_image_id`
- `record_success`
- `rollback_images`
- `deploy_transaction`

文件尾部只在脚本被直接执行时调用 `main`，被测试 source 时不自动运行。

- [ ] **Step 4: Implement locking and input validation**

`main`：

1. 切换到脚本所在目录。
2. 加载本地 `deploy.env`，校验 `ACR_REGISTRY`、`ACR_NAMESPACE`。
3. 使用 `flock -n` 锁定 `deploy.lock`。
4. 接受零个参数或一个完整 40 位十六进制 SHA；其他输入直接失败。
5. 执行部署事务，不输出环境文件内容。

- [ ] **Step 5: Implement the deployment transaction**

`deploy_transaction` 按此顺序执行：

1. 记录当前两个容器对应的 image ID；首次部署允许为空。
2. 拉取两个 `dev` 镜像。
3. 读取两个镜像的 OCI revision 标签。
4. 校验 revision 都是完整 40 位十六进制、彼此相同，并在提供参数时与参数相同。
5. 用 Compose 更新两个服务。
6. 轮询后端 `/healthz`，要求 HTTP 200、`status=ok` 且 revision 与候选 SHA 相同。
7. 轮询 Web 首页和 Web 代理的 `/healthz`。
8. 原子写入 `deployments/current.env`，包含成功 SHA、镜像引用和 UTC 时间。

- [ ] **Step 6: Implement two-image rollback**

任一启动或健康检查步骤失败时：

1. 若旧 image ID 不完整，报告首次部署失败并返回非零。
2. 将旧 server image ID 和旧 web image ID 重新标记为各自仓库下的本地 `rollback-<transaction-id>` 标签，不改写候选 `dev` 指针。
3. 以临时 `SERVER_IMAGE_TAG`、`WEB_IMAGE_TAG` 指向两个 rollback 标签，重新执行 Compose 更新两个服务。
4. 轮询旧版本健康状态。
5. 输出不含秘密的回滚结果。
6. 无论回滚成功与否，都以非零结束本次失败发布。

- [ ] **Step 7: Run the GREEN test**

Run:

    bash deploy/dev/tests/deploy_test.sh

Expected: PASS。

- [ ] **Step 8: Run static shell checks**

Run:

    bash -n deploy/dev/deploy.sh deploy/dev/tests/deploy_test.sh

Expected: PASS。

- [ ] **Step 9: Commit**

    git add deploy/dev/deploy.sh deploy/dev/tests/deploy_test.sh
    git commit -m "ops: add atomic dev deployment rollback"

## Task 5: Add The GitHub Actions Pipeline

**Files:**

- Create: `deploy/dev/tests/workflow_contract_test.sh`
- Create: `.github/workflows/deploy-dev.yml`

- [ ] **Step 1: Write the failing workflow contract test**

测试使用 Ruby 或仓库已有 YAML 解析能力加载 workflow，避免只靠脆弱文本匹配。断言：

- 触发器包含推送到 `dev` 和 `workflow_dispatch`。
- 手工触发要求 `target_sha`。
- concurrency group 固定且 `cancel-in-progress` 为 false。
- permissions 仅包含 `contents: read`。
- preflight 输出 `target_sha` 与 `migration_changed`。
- 构建 job 分别产出 `coze-server:dev-<full SHA>` 和 `coze-web:dev-<full SHA>`。
- promote 依赖两个构建结果，或在手工触发时验证两张不可变镜像已存在。
- migration 变化的 push 不运行 promote 和 deploy。
- deploy 只在 promote 成功后调用宝塔 webhook。
- workflow 不含 SSH 私钥、数据库迁移 apply 或生产环境命令。

- [ ] **Step 2: Run the RED test**

Run:

    bash deploy/dev/tests/workflow_contract_test.sh

Expected: FAIL，因为 workflow 尚不存在。

- [ ] **Step 3: Implement preflight**

`preflight` job：

- push 使用当前提交完整 SHA。
- dispatch 校验 `target_sha` 是完整 SHA，并通过 Git 验证它属于 `origin/dev` 历史。
- push 比较事件 before SHA 到目标 SHA 的 `docker/atlas/migrations/**`。
- before 缺失、全零或不可获取时 fail closed，将 `migration_changed=true`。
- 输出标准化 target SHA 和 migration flag，后续所有 job 只消费这两个输出。

- [ ] **Step 4: Build immutable images in parallel**

`build-server` 与 `build-web`：

- 仅在 push 事件运行。
- 登录 ACR。
- 使用 Buildx 和对应 Dockerfile。
- 传入 `GIT_REVISION=<完整 SHA>` 与仓库 URL。
- 推送唯一标签 `dev-<完整 SHA>`。
- 不在构建 job 中更新 `dev` 标签。

仓库分别固定为：

- `ACR_REGISTRY/ACR_NAMESPACE/coze-server`
- `ACR_REGISTRY/ACR_NAMESPACE/coze-web`

- [ ] **Step 5: Implement migration hold and manual resume**

push 检测到 migration 变化时：

- 两个不可变镜像仍正常构建。
- `migration-hold` job 在 Summary 写明目标 SHA 和人工步骤。
- promote 与 deploy 跳过，不能更新 `dev` 标签或调用 webhook。

人工完成远程数据库迁移后，通过 `workflow_dispatch` 输入同一个完整 SHA：

- 不重新构建。
- 验证两张 `dev-<SHA>` 镜像存在。
- 验证镜像 revision 标签都等于输入 SHA。
- 继续 promote。

- [ ] **Step 6: Promote both images, then trigger Baota**

`promote` 使用 `docker buildx imagetools create` 将两张不可变镜像分别晋级为 `dev`。只有两个晋级步骤都成功，`deploy` 才使用 `curl --fail-with-body` 调用 `BAOTA_WEBHOOK_URL`。

若配置 `BAOTA_WEBHOOK_TOKEN`，以请求 header 发送；不得拼入日志。webhook body 或参数携带目标 SHA，服务器脚本即使忽略参数也可通过两张 `dev` 镜像的 revision 一致性保护部署。

Workflow 配置：

- vars: `ACR_REGISTRY`、`ACR_NAMESPACE`
- secrets: `ACR_USERNAME`、`ACR_PASSWORD`、`BAOTA_WEBHOOK_URL`
- optional secret: `BAOTA_WEBHOOK_TOKEN`
- concurrency group: `deploy-dev`
- cancel-in-progress: `false`

- [ ] **Step 7: Run the GREEN workflow test**

Run:

    bash deploy/dev/tests/workflow_contract_test.sh

Expected: PASS。

- [ ] **Step 8: Commit**

    git add .github/workflows/deploy-dev.yml deploy/dev/tests/workflow_contract_test.sh
    git commit -m "ci: publish and deploy dev images"

## Task 6: Document Operations And Integration Authorization

**Files:**

- Create: `deploy/dev/README.md`
- Modify: `docs/superpowers/context/project-context.md`
- Modify: `docs/superpowers/runbooks/dev-integration-audit.md`

- [ ] **Step 1: Write the server installation runbook**

`deploy/dev/README.md` 必须包含：

- ACR 创建 `coze-server`、`coze-web` 两个仓库。
- GitHub variables/secrets 清单及最小权限。
- 服务器安装 Docker、Compose、`flock`，登录 ACR 的拉取账号。
- 将目录部署到 `/opt/coze-dev`。
- 从 `.env.example` 创建仅含部署参数的 `deploy.env`。
- 单独创建 `app.env`，权限设为 600，不提交 Git。
- 宝塔 webhook 固定执行 `/opt/coze-dev/deploy.sh`，可选转发 SHA。
- 宝塔 Nginx 将域名 HTTPS 流量代理到 `127.0.0.1:8888`。
- 首次部署、日常发布、migration hold、人工迁移后恢复、失败回滚和人工回滚流程。
- 明确这是 dev/预发布单实例，允许约 10 到 30 秒更新中断。

- [ ] **Step 2: Update long-term project context**

在 `project-context.md` 增加精简事实：

- dev 远程推送会构建并发布两张 ACR 镜像。
- 发布使用不可变完整 SHA 和服务器双镜像一致性校验。
- migration 变化必须人工 apply 后手工恢复。
- 服务器只运行应用两服务，MySQL、ES、Redis、对象存储均为远程服务。
- 该流程不等于生产发布。

- [ ] **Step 3: Update the dev integration gate**

在 `dev-integration-audit.md` 第二次确认说明中补充：

- 推送 `origin/dev` 将触发 ACR 构建和宝塔预发布部署。
- 第二次确认必须在报告中明确披露目标分支、目标 SHA、文件范围、验证结果以及自动部署副作用。
- 该确认不授权生产发布、数据库 migration apply 或其他服务器操作。
- 若远程 `dev` 在审计期间变化，仍按现有规则从第一次审计重来。

- [ ] **Step 4: Scan documentation for secrets and placeholders**

Run:

    rg -n 'aliyuncs\.com|AKID|AccessKey|SecretKey|password=|token=' deploy/dev docs/superpowers/context/project-context.md docs/superpowers/runbooks/dev-integration-audit.md
    rg -n 'TBD|TODO|PLACEHOLDER|implement later' deploy/dev docs/superpowers/context/project-context.md docs/superpowers/runbooks/dev-integration-audit.md
    git diff --check

Expected: 只允许示例域名与变量名，不出现真实凭据；无未完成占位符；diff check 通过。

- [ ] **Step 5: Commit**

    git add deploy/dev/README.md docs/superpowers/context/project-context.md docs/superpowers/runbooks/dev-integration-audit.md
    git commit -m "docs: add dev deployment runbook"

## Task 7: Full Verification And First Dev Audit

**Files:**

- Verify all files changed by Tasks 1 through 6.
- Do not modify cloud resources or remote branches.

- [ ] **Step 1: Run backend verification**

Run:

    cd backend
    go test ./api/handler/coze ./api/router/coze -count=1

Expected: PASS。

- [ ] **Step 2: Run deployment contract tests**

Run from repository root:

    bash deploy/dev/tests/image_contract_test.sh
    bash deploy/dev/tests/compose_contract_test.sh
    bash deploy/dev/tests/deploy_test.sh
    bash deploy/dev/tests/workflow_contract_test.sh
    bash -n deploy/dev/deploy.sh deploy/dev/tests/*.sh

Expected: 全部 PASS。

- [ ] **Step 3: Validate Compose rendering**

Run:

    cp deploy/dev/.env.example deploy/dev/deploy.env
    touch deploy/dev/app.env
    docker compose --env-file deploy/dev/deploy.env -f deploy/dev/docker-compose.yml config

Expected: 只有 `coze-server` 和 `coze-web`，环境替换完整，无 YAML 或 Compose 错误。验证后删除本地临时 `deploy.env` 与 `app.env`，不得提交。

- [ ] **Step 4: Build both images locally**

使用固定测试 revision：

    docker build -f backend/Dockerfile --build-arg GIT_REVISION=0123456789abcdef0123456789abcdef01234567 --build-arg SOURCE_URL=https://github.com/example/coze-studio -t coze-server:deploy-test .
    docker build -f frontend/Dockerfile --build-arg GIT_REVISION=0123456789abcdef0123456789abcdef01234567 --build-arg SOURCE_URL=https://github.com/example/coze-studio -t coze-web:deploy-test .

Expected: 两张镜像构建成功。

- [ ] **Step 5: Inspect immutable metadata**

Run:

    docker image inspect coze-server:deploy-test --format '{{ index .Config.Labels "org.opencontainers.image.revision" }} {{ index .Config.Labels "org.opencontainers.image.source" }} {{ index .Config.Env }}'
    docker image inspect coze-web:deploy-test --format '{{ index .Config.Labels "org.opencontainers.image.revision" }} {{ index .Config.Labels "org.opencontainers.image.source" }}'

Expected: revision 都是固定完整 SHA，source 正确，后端环境包含同一 `APP_REVISION`。

- [ ] **Step 6: Recheck structural impact with codebase-memory**

刷新或检测图谱变更后：

- 搜索 `GetHealthz`，确认只由公开路由引用。
- 追踪 `RegisterCustomRoutes`，确认没有改变既有认证组所有权。
- 回到真实源码和 diff 核对图谱结论。

- [ ] **Step 7: Perform the first dev integration audit**

Run:

    git fetch origin dev
    git rev-list --left-right --count origin/dev...HEAD
    git diff --check origin/dev...HEAD
    git diff --stat origin/dev...HEAD
    git diff --name-status origin/dev...HEAD
    git log --oneline --decorate origin/dev..HEAD
    git status --short --branch

审计报告必须列出：

- 分支名与完整 HEAD SHA。
- 与最新 `origin/dev` 的 ahead/behind。
- 全部变更文件和提交。
- 本轮新鲜测试结果。
- 未执行的真实 ACR、GitHub Actions、宝塔 webhook 和远程服务器验证。
- migration 人工门禁和自动回滚剩余风险。

- [ ] **Step 8: Stop for the first explicit confirmation**

向用户提交第一次审计报告后停止。未经第一次明确确认，不合入本地 `dev`；未经合并后的第二次审计及第二次明确确认，不推送 `origin/dev`。任何真实云配置、migration apply、宝塔 webhook 创建仍需单独操作和授权。

## Acceptance Checklist

- [ ] 未认证的 `GET /healthz` 返回状态和完整构建 revision。
- [ ] 后端与前端镜像都包含相同 OCI revision 元数据。
- [ ] 部署 Compose 只运行 server 和 web，外部基础设施不容器化。
- [ ] Web 只监听宿主机 loopback，由宝塔提供公网 HTTPS。
- [ ] 两张候选镜像 revision 不一致时不会重启服务。
- [ ] 健康检查失败会同时恢复两张旧镜像，并把发布标记为失败。
- [ ] migration 变化的 push 不更新 `dev` 标签、不调用 webhook。
- [ ] 人工迁移后可以用完整 SHA 手工恢复同一批不可变镜像。
- [ ] GitHub 和服务器权限最小化，仓库无真实凭据。
- [ ] 完成第一次 dev 集成审计并等待用户确认。
