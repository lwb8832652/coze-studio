# 对象存储管理控制面设计

**日期：** 2026-07-28
**状态：** 已完成交互设计确认，待书面规格审核
**范围：** 系统级对象存储配置、七家云厂商适配、启动期主配置加载

## 1. 背景

Coze Studio 当前通过环境变量选择对象存储，`backend/infra/storage/impl/storage.go`
只支持 MinIO、TOS 和 S3。`backend/application/base/appinfra/app_infra.go` 在
MySQL 之前创建对象存储客户端，因此运行时无法从数据库读取管理配置。

系统管理页面已经具备基础设置入口，但没有对象存储配置能力。本需求要在不增加
额外常驻服务的前提下，让系统管理员保存多条对象存储配置、检测连通性，并选择
唯一主配置。项目的 MySQL、Elasticsearch、Redis 和对象存储均为外部云服务；
该功能本身不新增容器，对 2C4G 应用主机只增加少量 SDK 和管理请求开销。完整
系统能否在 2C4G 下稳定承载目标并发仍需结合 worker 数、文件流量和实际负载验证，
不能仅凭该功能的资源增量作保证。

## 2. 已确认决策

1. 系统可以保存多条对象存储配置，但数据库中最多只有一条主配置。
2. 支持七牛 Kodo、阿里 OSS、腾讯 COS、华为 OBS、AWS S3、MinIO、火山 TOS。
3. 保留现有 `storage.Storage` 接口，每家厂商使用官方 Go SDK 独立适配。
4. 各适配器通过同一套契约测试，不引入通用存储框架作为运行时依赖。
5. 持久化只使用 `object_storage_configs` 一张表，不建设主配置表和审计表。
6. AK/SK 成对保存，使用 AES-256-GCM 加密后落库，接口不回显。
7. 激活只更新数据库中的期望主配置，不热切换当前进程；重启
   `coze-server` 后生效。
8. 历史对象由运维人员按原对象 Key 完成迁移，系统不做迁移、回退读取或双写。
9. v1 仅支持长期 AK/SK，不支持 STS、IAM Role、实例角色、KMS 和凭证轮换。
10. 连接检测只执行只读能力检查，不创建 Bucket，不上传或删除探测对象。

## 3. 目标与非目标

### 3.1 目标

- 在系统管理中提供对象存储配置的新增、编辑、删除、检测和激活。
- 用数据库配置构造当前进程唯一的对象存储客户端。
- 保持既有对象读写调用方继续依赖 `storage.Storage`，避免业务层感知厂商。
- 兼容现有 Docker 环境变量部署，并提供故障时的显式环境变量救援模式。
- 在 API、日志、数据库备份和前端状态中保护 AK/SK。

### 3.2 非目标

- 不按工作空间或租户配置对象存储；配置是系统全局的。
- 不支持运行时热切换、双写、旧存储回退或自动对象迁移。
- 不管理 Bucket 生命周期、跨域、CDN、权限策略和存储类别。
- 不自动创建 Bucket。
- 不记录配置变更历史和数据库审计事件。
- 不允许把已保存配置的厂商类型修改为另一厂商。
- 不提供长期凭证之外的认证方式。

## 4. 总体架构

~~~mermaid
flowchart LR
    UI["系统管理 / 对象存储"] --> API["Admin Config API"]
    API --> APP["对象存储配置应用服务"]
    APP --> REPO["MySQL 单表仓储"]
    APP --> CODEC["凭证加解密"]
    APP --> REG["适配器注册表"]
    BOOT["coze-server 启动"] --> REPO
    BOOT --> REG
    REG --> CONTRACT["storage.Storage"]
    REG --> SDK["七家官方 SDK"]
~~~

### 4.1 分层职责

- **IDL/API 层**：扩展 `idl/admin/config.thrift`，提供管理员 RPC；不把对象存储
  字段加入 `BasicConfiguration`。
- **应用层**：编排 CRUD、连接检测、激活事务、环境变量导入和运行状态比较。
- **领域层**：定义厂商枚举、强类型公共配置、凭证规则、状态转换和错误。
- **基础设施层**：实现 MySQL 仓储、AES-GCM codec、环境变量解析和七家适配器。
- **运行时层**：启动时只构造一个主存储客户端，并记录实际加载的配置标识。

现有 `storage.Storage`、`storage.StreamingStorage` 和
`storage.ReadinessChecker` 合同保持稳定。新增适配器必须实现
`Storage`；能够直接流式读取的 SDK 同时实现 `StreamingStorage`；所有适配器
实现只读 `ReadinessChecker`。

### 4.2 适配器注册表

注册表按稳定的 `provider_type` 将强类型配置映射到构造器：

- `qiniu`：七牛 Kodo 官方 Go SDK。
- `aliyun_oss`：阿里云 OSS Go SDK v2。
- `tencent_cos`：腾讯云 COS Go SDK v5。
- `huawei_obs`：华为云 OBS Go SDK。
- `aws_s3`：现有 AWS SDK for Go v2。
- `minio`：现有 MinIO Go SDK v7。
- `tos`：现有火山 TOS Go SDK v2。

业务调用方不直接依赖任何厂商 SDK。S3、MinIO、TOS 现有构造函数改为接收强
类型配置；新构造函数不得隐式创建 Bucket。

`ImageXClient` 必须与 `deps.OSS` 使用同一份启动期配置。实现时抽取一个通用
storage-backed ImageX wrapper，避免七个适配器重复读取环境变量或重新选择
厂商。若 `FILE_UPLOAD_COMPONENT_TYPE` 明确选择火山 ImageX，则保持现有
专用 ImageX 路径。

## 5. 单表数据模型

新增 `object_storage_configs`：

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | BIGINT UNSIGNED | 自增主键 |
| `name` | VARCHAR(128) | 管理员可见名称，全局唯一 |
| `provider_type` | VARCHAR(32) | 七种稳定枚举之一，创建后不可修改 |
| `config_json` | JSON | Bucket、Region、Endpoint 等非敏感强类型配置 |
| `credential_secret` | TEXT | 版本化 AES-GCM 密文 envelope |
| `active_slot` | TINYINT UNSIGNED NULL | 主配置为 1，其他配置为 NULL |
| `health_status` | VARCHAR(32) | unknown、healthy、unhealthy |
| `last_health_code` | VARCHAR(64) | 脱敏后的稳定错误码 |
| `last_health_message` | VARCHAR(255) | 脱敏后的简短错误 |
| `last_health_latency_ms` | INT UNSIGNED | 最近检测耗时 |
| `last_health_at` | DATETIME(3) NULL | 最近检测时间 |
| `version` | BIGINT UNSIGNED | 用户变更的乐观锁版本 |
| `runtime_revision` | BIGINT UNSIGNED | 仅运行时有效字段变化时递增 |
| `created_at` | DATETIME(3) | 创建时间 |
| `updated_at` | DATETIME(3) | 最近用户变更时间 |

约束与索引：

- 主键为 `id`。
- `name` 唯一。
- `active_slot` 建唯一索引。MySQL 唯一索引允许多个 NULL，但只允许一个值 1，
  因而数据库层保证最多一个主配置。
- `active_slot` 只允许 NULL 或 1；迁移使用 CHECK 约束，并在领域层重复校验。
- `provider_type` 建普通索引。
- 使用物理删除，不增加 `deleted_at`。
- `credential_secret` 为 NOT NULL；创建事务提交前必须写入非空有效 envelope。

单表约束保证“最多一个”主配置；应用层保证 database 运行模式下“恰好一个”。
零主配置只允许出现在首次引导或救援修复过程中，database 模式启动时发现零主
配置必须失败。

`version` 在编辑和激活等用户变更时递增，用于拒绝过期提交。连接检测只更新
health 字段，不改变 `version`。`runtime_revision` 仅在 Bucket、Region、
Endpoint、下载域名、TLS/Path Style 或凭证发生变化时递增；只修改名称或健康
状态不会触发重启提示。

### 5.1 主配置切换事务

激活目标配置时：

1. 校验管理员权限、目标 ID、请求版本和迁移确认字段。
2. 使用目标配置执行一次只读连接检测。
3. 开启事务并锁定当前主配置与目标配置。
4. 将旧主配置 `active_slot` 更新为 NULL。
5. 将目标配置 `active_slot` 更新为 1。
6. 递增受影响行的 `version` 并提交。

任何一步失败都回滚，旧主配置保持不变。允许在待重启期间再次激活另一配置；
最终重启只加载数据库当时的主配置。

目标已经是数据库主配置时，激活接口幂等返回当前状态，不重复更新版本。唯一
索引冲突或锁冲突映射为稳定的并发冲突，不向前端暴露 SQL 错误。

## 6. 厂商配置模型

所有配置共享 `name`、`provider_type`、Bucket 和凭证。API 返回规范化的
非敏感配置对象；未知字段被拒绝，避免拼写错误被静默忽略。

| 厂商 | 必填字段 | 可选/高级字段 |
|---|---|---|
| 七牛 Kodo | bucket、download_domain | region、use_https，默认 true |
| 阿里 OSS | bucket、region | endpoint_override |
| 腾讯 COS | bucket、region | endpoint_override |
| 华为 OBS | bucket、region、endpoint | 无 |
| AWS S3 | bucket、region | endpoint_override、force_path_style |
| MinIO | bucket、endpoint、use_ssl | region |
| 火山 TOS | bucket、region、endpoint | 无 |

补充规则：

- 七牛下载域名用于生成对象访问 URL，只保存不带 scheme 和路径的主机名；
  URL 协议由 use_https 决定。
- 腾讯 Bucket 按 `bucket-appid` 形式校验。
- 阿里 OSS、腾讯 COS 和 AWS S3 在未覆盖 Endpoint 时使用官方规则生成地址。
- MinIO Endpoint 接受 host:port 或 URL，规范化后再交给 SDK。
- 自定义 Endpoint 必须是 HTTPS；仅本机 Debug 环境允许显式 HTTP。
- Bucket 均视为已经存在，检测和运行时不会自动创建。
- 各厂商在 UI 中使用其官方凭证名称，服务端统一为
  `access_key_id` 和 `secret_access_key`。

## 7. 凭证保护

### 7.1 保存格式

明文凭证逻辑结构只包含：

~~~json
{
  "access_key_id": "...",
  "secret_access_key": "..."
}
~~~

服务端将其序列化为有版本的 payload，使用项目
`backend/pkg/secureaead` 提供的 AES-GCM 原语加密。环境变量
`OBJECT_STORAGE_CREDENTIAL_KEY` 必须是标准 Base64 编码的 32 字节随机密钥。
v1 只使用一个固定密钥，不实现 KMS 和在线轮换。

创建时先在事务内插入未提交行以取得自增 ID，再使用
`config ID + provider_type + envelope version` 作为 AAD 加密并更新该行，最后
提交。这样密文不能复制给另一条配置或另一厂商使用。密文 envelope 包含格式
版本、随机 nonce 和 ciphertext。

### 7.2 API 与日志规则

- 创建时 AK/SK 都必填。
- 编辑时两项都留空表示保留原凭证；只填写其中一项返回校验错误。
- 更换凭证时 AK/SK 必须同时提交。
- 查询响应只返回 `credential_configured=true/false`。
- 前端提交后清空凭证输入，不把凭证写入 URL、持久化 store 或浏览器日志。
- 后端不记录请求体、密文、指纹、AK/SK 或包含签名参数的 URL。
- SDK 错误只映射为稳定错误码和脱敏摘要。
- 部署必须通过 HTTPS 暴露管理员 API。

密钥丢失或被替换会导致数据库凭证不可恢复。部署文档必须要求将该密钥放入
Docker Secret 或等价的受控环境变量，并与数据库备份独立备份。

## 8. 管理 API

在现有 Admin `ConfigService` 中增加以下 RPC，继续由 IDL 生成前后端合同：

| RPC | 路由 | 行为 |
|---|---|---|
| `ListObjectStorageConfigs` | GET `/api/admin/config/object-storage/list` | 返回全部非敏感配置和运行状态 |
| `CreateObjectStorageConfig` | POST `/api/admin/config/object-storage/create` | 创建非主配置 |
| `UpdateObjectStorageConfig` | POST `/api/admin/config/object-storage/update` | 按 ID/version 更新 |
| `TestObjectStorageConfig` | POST `/api/admin/config/object-storage/test` | 检测草稿或已保存配置 |
| `ActivateObjectStorageConfig` | POST `/api/admin/config/object-storage/activate` | 检测后原子切换主配置 |
| `DeleteObjectStorageConfig` | POST `/api/admin/config/object-storage/delete` | 物理删除可删除配置 |

### 8.1 列表响应

每条配置返回：

- ID、name、provider_type、公开 config、version、runtime_revision。
- credential_configured。
- health 状态、脱敏错误、耗时和检测时间。
- desired_active：是否为数据库主配置。
- runtime_active：当前进程是否实际使用该 ID 和 runtime_revision。
- restart_required：数据库主配置与当前运行描述不一致。

顶层响应还返回 `runtime_source=database|env_rescue`。救援模式下页面显示明确
状态，不把环境变量凭证导入或回显。

### 8.2 草稿检测

新增抽屉可以在保存前检测。检测请求携带完整公开配置和 AK/SK，但服务端不持久化。
编辑抽屉可以省略凭证并提供 config ID/version，此时服务端解密已保存凭证与草稿
公开配置组合检测。任何检测请求都不得进入通用请求体日志。

草稿公开配置或草稿凭证与数据库内容不完全一致时，检测结果只随响应返回，不更新
已保存记录的 health 字段。只有对未修改的已保存配置执行检测，以及激活时对持久
化配置执行检测，才更新该记录的 health 字段。

### 8.3 删除约束

以下配置禁止删除：

- 数据库当前主配置。
- 当前进程仍在使用的运行配置，即使数据库已激活另一条待重启配置。

因此切换后至少要完成一次重启，旧配置才能删除。删除请求也使用 `version`
防止误删已被其他管理员修改的记录。

## 9. 连接检测

`ReadinessChecker.CheckReadiness` 是统一检测入口，必须满足：

- 设置短超时并遵循请求 context 取消。
- 仅使用 HeadBucket、GetBucketLocation 或最多一条 List 等只读 API。
- 不调用 CreateBucket、PutObject、DeleteObject。
- 不把不存在的固定对象当作探针。
- 关闭临时客户端产生的响应体和连接资源。
- 返回统一分类：认证失败、Bucket 不存在/无权访问、Endpoint/Region 错误、
  网络超时、厂商限流、未知错误。

激活操作总是重新检测，不依赖“最近一次成功”的时间窗口。检测失败只更新目标
配置的 health 字段，不改变主配置。

## 10. 启动与重启语义

### 10.1 初始化顺序

`AppInfra.Init` 调整为：

1. 初始化 MySQL。
2. 加载对象存储凭证 codec；database 模式要求可用，救援模式允许不可用。
3. 执行首次环境变量导入检查。
4. 读取数据库主配置并构造 `Storage` 与 storage-backed `ImageX`。
5. 初始化 Redis、ID generator、基础 config 和其他依赖。

这是必要的有向依赖调整，不把数据库仓储逻辑放进具体厂商 adapter。

### 10.2 首次环境变量导入

默认 `OBJECT_STORAGE_CONFIG_SOURCE=database`。当表中没有任何配置时：

1. 解析现有 `STORAGE_TYPE` 及对应厂商环境变量。
2. 对字段做与管理 API 相同的强校验。
3. 使用 `OBJECT_STORAGE_CREDENTIAL_KEY` 加密凭证。
4. 在一个事务内创建名为 `Environment import` 的配置并设置
   `active_slot=1`。
5. 从刚导入的数据库记录构造客户端。

环境变量解析扩展到七家厂商；已有 MinIO、TOS、S3 变量保持兼容，`s3` 映射为
规范化的 `aws_s3`。

如果表为空、环境变量无效或加密密钥缺失，服务启动失败并给出不含秘密的明确
错误。导入只在表完全为空时执行，不覆盖已有配置。

### 10.3 救援模式

设置 `OBJECT_STORAGE_CONFIG_SOURCE=env` 时：

- 启动完全绕过数据库中的主配置，直接使用环境变量构造客户端。
- 不自动修改、删除或重新加密数据库记录。
- 管理页可以查看和修复数据库配置，但运行状态显示 `env_rescue`。
- 修复后移除救援变量并重启，才恢复数据库主配置。

救援模式是显式运维开关，不是自动回退。数据库解密或构造失败时，默认行为仍为
fail closed。救援运行时不依赖数据库密文，因此旧加密密钥丢失时仍可使用环境
变量启动；此时列表等非凭证操作可用，依赖解密或加密的管理操作返回
`OBJECT_STORAGE_CREDENTIAL_UNAVAILABLE`。运维人员可配置一把新的有效密钥、
在救援模式重启后为各配置重新成对录入 AK/SK，再恢复 database 模式。

### 10.4 待重启判断

进程启动后只在内存保存
`runtime_source + config_id + runtime_revision + provider_type`，不保存第二份
数据库状态。管理接口把它与当前数据库主配置比较：

- ID 或 runtime_revision 不同：restart_required=true。
- 仅 name、health 或 version 不同：restart_required=false。
- 再次激活回当前运行 ID 且 runtime_revision 相同：取消待重启状态。
- 救援模式：显示救援状态，数据库切换不会热生效。

当前进程中的 `Storage` 指针不会被管理请求替换。

## 11. 历史对象与切换确认

系统假设对象 Key 是跨厂商迁移后的稳定标识。激活弹窗必须要求管理员确认：

“旧 Bucket 中的对象已迁移到目标 Bucket，并保持原对象 Key。”

切换到与当前运行配置不同的目标时，请求必须包含
`migration_confirmed=true`，缺失则服务端拒绝激活。目标已经是数据库主配置的
幂等请求不要求重复确认。该确认不代表系统验证了全量对象，也不产生迁移记录。
激活后、重启前仍由旧运行配置读写；重启后只访问新主配置，不回退读取旧配置。

运维人员应在低写入窗口完成最终增量同步，再激活并尽快重启，以缩短切换窗口。

## 12. 管理页面

在现有系统管理导航增加“对象存储”二级页面，使用
`@coze-arch/coze-design` 和现有页面结构。

### 12.1 列表

列表列为：配置名称、厂商、Bucket、连接状态、运行状态、更新时间、操作。

运行状态使用互斥标签表达：

- 当前主配置：数据库期望与当前运行一致。
- 待重启：数据库期望主配置尚未运行。
- 运行中：待重启期间仍由当前进程使用的旧配置。
- 救援模式：当前进程来自环境变量。

每行操作为检测、编辑、激活、删除。当前主配置不显示激活操作；禁止删除的操作
为 disabled，并提供原因 tooltip。

### 12.2 新增与编辑抽屉

- 新增和编辑共用右侧抽屉。
- 厂商使用选择器；编辑时只读。
- 选择厂商后动态渲染对应强类型字段。
- 低频 Endpoint override、Path Style 等放入高级配置折叠区。
- AK/SK 使用密码输入；编辑时不填充掩码值。
- 操作为测试连接、取消、保存，具备 loading、disabled 和错误状态。
- 提交成功后立即清空凭证字段。

删除使用二次确认。激活使用高风险确认弹窗，包含对象迁移确认复选框；确认成功
后刷新列表并显示待重启状态。

## 13. 错误处理

应用层输出稳定业务错误，避免前端解析 SDK 文本：

- `OBJECT_STORAGE_CONFIG_INVALID`：厂商字段或 AK/SK 规则不合法。
- `OBJECT_STORAGE_NOT_FOUND`：配置不存在。
- `OBJECT_STORAGE_VERSION_CONFLICT`：乐观锁版本过期。
- `OBJECT_STORAGE_CONNECTION_FAILED`：只读连接检测失败。
- `OBJECT_STORAGE_ACTIVE_DELETE_FORBIDDEN`：目标仍是期望或运行配置。
- `OBJECT_STORAGE_CREDENTIAL_UNAVAILABLE`：密钥缺失、密文损坏或解密失败。
- `OBJECT_STORAGE_PROVIDER_UNSUPPORTED`：未知厂商类型。

创建、编辑和激活失败不产生部分可见状态。连接检测 health 更新失败不应覆盖
原始检测错误；接口返回检测结果，持久化失败作为独立内部错误记录。

## 14. 测试与验收

### 14.1 后端

- 领域测试：七种 schema、Endpoint 规范化、COS Bucket、AK/SK 成对规则。
- codec 测试：round trip、随机 nonce、AAD 绑定、篡改、错误密钥、明文扫描。
- 仓储测试：CRUD、唯一名称、唯一 active_slot、乐观锁、物理删除。
- 应用测试：凭证保留/替换、激活事务、检测失败回滚、删除限制。
- API 测试：管理员鉴权、响应不含凭证、稳定错误码、请求不进入日志。
- 启动测试：首次导入、DB 加载、无主配置、错误密钥、救援模式。
- 适配器共享契约：Put/Get/Head/List/Delete/签名 URL/流式读取/错误映射。
- readiness 契约：使用 mock SDK 断言不发生创建、上传和删除调用。

MinIO 使用本地容器做真实集成测试。其他六家云厂商的真实测试通过显式环境变量
启用，不进入默认 CI；默认 CI 使用各官方 SDK 的窄接口 mock。

### 14.2 前端

- 动态厂商字段和校验。
- 新增、编辑、AK/SK 留空保留与成对替换。
- 检测、激活、删除交互及 loading/error/disabled 状态。
- 当前、运行中、待重启和救援状态。
- active/runtime 配置删除限制。
- 生成 API 类型的编译检查。

### 14.3 工程验证

- 相关 Go package 测试；需要 Mockey 的测试使用仓库规定 gcflags。
- 相关 Vitest、TypeScript typecheck 和 lint。
- Atlas Community v0.35.0 migration hash 与 validate。
- 应用内浏览器验证列表、抽屉、检测、激活确认、删除限制和控制台错误。
- 检查最终构建产物和 API 响应中不存在测试 AK/SK。

## 15. 发布与恢复

发布顺序：

1. 生成并安全保存 `OBJECT_STORAGE_CREDENTIAL_KEY`。
2. 保留当前对象存储环境变量，发布迁移和新版本。
3. 首次启动自动导入环境配置并设置为主配置。
4. 在管理页确认导入配置与当前运行状态一致。
5. 新增目标配置并通过连接检测。
6. 运维迁移对象并保持 Key，完成激活确认。
7. 重启 `coze-server`，验证新配置成为当前主配置。

故障恢复：

- 未重启前可以重新激活原运行配置以取消待切换状态。
- 重启后发生问题，可以重新激活旧配置并再次重启。
- 数据库配置或密钥故障时，显式设置
  `OBJECT_STORAGE_CONFIG_SOURCE=env` 并重启进入救援模式。
- 禁止自动回退，避免恢复过程中把新对象分散写入两个 Bucket。

## 16. 完成标准

以下条件全部满足才视为功能完成：

1. 管理员可以新增、编辑、检测、激活和删除七家厂商配置。
2. 数据库只有一张业务表，且并发激活后最多一条 `active_slot=1`。
3. AK/SK 不明文落库、不回显、不写日志。
4. 当前主配置及待重启状态与真实运行配置一致。
5. 连接检测不产生任何写操作。
6. 环境变量首次导入和救援模式可验证。
7. 重启后所有对象存储及 storage-backed ImageX 调用使用同一主配置。
8. 系统不执行历史对象迁移、回退读取或双写。
9. 后端、前端、迁移和浏览器验收均通过，未验证的真实云厂商测试有明确记录。
