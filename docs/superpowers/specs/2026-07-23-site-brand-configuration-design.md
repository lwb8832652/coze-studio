# Coze 站点品牌配置设计

日期：2026-07-23

## 1. 背景

Nuwax 的系统配置支持站点名称、访问地址、介绍、Logo、favicon 等租户品牌
配置。Coze 当前已经存在管理员基础配置、版本冲突保护和持久化能力，但品牌
信息仍散落在静态 HTML、登录页和工作空间组件中：

- `frontend/apps/coze-studio/index.html` 静态写死 `NewX AI`；
- 登录页写死 `Coze Studio`、欢迎文案和品牌图标；
- 工作空间顶部状态、任务详情 Agent 标题写死 `NewX AI`；
- 工作空间标记使用固定图形；
- `server_host` 已经承担外部访问和回调地址，不应重复新增 `site_url`。

本设计只迁移 Coze 有真实消费点的站点基础配置。项目当前没有智能体广场，
因此不迁移广场 Banner、广场文案、广场链接和站点默认智能体配置。

## 2. 目标

- 在现有“系统管理 > 系统配置”中集中维护站点品牌。
- 配置真实作用于登录前和登录后的用户界面，而不是只保存表单数据。
- 复用现有 `basic_config`、系统管理员鉴权和 revision 乐观锁。
- 图片写入对象存储，配置只保存受控对象 URI，不保存会过期的签名 URL。
- 老环境无新字段时保持当前可用默认值，不要求数据库迁移。

## 3. 非目标

- 不新增生态广场或智能体广场。
- 不增加广场 Banner、登录 Banner、主题模板或默认站点 Agent。
- 不改变工作空间名称、工作空间成员数据和用户头像。
- 不修改 Agent Runtime 内部 prompt、模型身份或任务执行合同。
- 不移除 `Powered by Coze Studio` 等开源归属信息。
- 不允许通过配置注入任意 HTML、CSS、SVG 或外部脚本。

## 4. 配置模型

在现有 `BasicConfiguration` 中增加以下可选字段：

| 字段 | 含义 | 默认行为 |
| --- | --- | --- |
| `site_name` | 用户可见站点名称 | `NewX AI` |
| `site_description` | 站点介绍及 meta description | 使用当前登录页默认介绍 |
| `site_logo_uri` | 对象存储中的站点 Logo URI | 使用当前内置品牌图形 |
| `favicon_uri` | 对象存储中的 favicon URI | 使用应用内置 favicon |

现有 `server_host` 在系统配置页面显示为“站点访问地址（服务地址）”，继续用于
外部访问地址、OAuth 回调和开放 API 地址，不再新增重复字段。

`site_logo_uri` 与 `favicon_uri` 允许保存空字符串。空字符串表示清除自定义
资源并恢复内置默认值。

## 5. 配置消费边界

### 5.1 浏览器级品牌

- 页面标题使用 `site_name`。
- 配置了介绍时，标题为 `${site_name} - ${site_description}`。
- 更新或创建唯一的 `link#site-favicon`，避免重复追加 favicon 节点。
- 更新 `meta[name="description"]`。
- 配置接口失败时继续使用 `index.html` 的静态默认值。

### 5.2 登录与注册页

- 左上品牌图形优先使用 `site_logo_url`，未配置时使用现有 `CozeBrand`。
- 品牌名称、欢迎语使用 `site_name`。
- 介绍区域优先使用 `site_description`。
- 登录、注册、协议和多语言行为保持原样。
- 开源归属 footer 保持原样。

### 5.3 工作空间与任务页面

- 左上工作空间切换器仍显示当前工作空间名称。
- 工作空间切换器前的品牌标记优先使用站点 Logo，未配置时使用当前默认标记。
- 工作空间顶部“专属助理已就绪”中的产品名称使用 `site_name`。
- 任务详情中用户可见的 Agent 品牌标题使用 `site_name`。
- Agent Runtime 内部名称仍保持现有实现，本功能只调整用户界面品牌。

### 5.4 不受站点配置影响的内容

- 个人空间、团队空间的名称和类型标签；
- 工作空间 Logo 或未来的空间级品牌；
- 用户头像、昵称和账号信息；
- 具体 Bot、Skill、MCP、网页应用的名称和图标；
- 通知记录中由业务数据明确返回的发送者名称。

## 6. 后端合同

### 6.1 管理员读取与保存

继续使用：

- `GET /api/admin/config/basic/get`
- `POST /api/admin/config/basic/save`

管理员响应和保存 patch 增加四个站点字段。保存继续要求
`expected_revision`，冲突仍返回稳定的
`BASE_CONFIG_VERSION_CONFLICT`。

服务端校验：

- `site_name` 去除首尾空白后必须非空，最长 64 个字符；
- `site_description` 最长 500 个字符；
- `site_logo_uri` 只能为空或属于受控 `site-brand/logo/` 前缀；
- `favicon_uri` 只能为空或属于受控 `site-brand/favicon/` 前缀；
- URI 非空时必须能在对象存储中查询到；
- `server_host` 继续使用现有 URL/主机端口校验。

### 6.2 公共品牌读取

新增无需登录的只读接口：

```text
GET /api/site/config
```

响应只包含：

```json
{
  "site_name": "NewX AI",
  "site_description": "...",
  "site_logo_url": "https://signed-object-url.example/...",
  "favicon_url": "https://signed-object-url.example/...",
  "revision": "..."
}
```

公共接口不返回对象 URI、管理员邮箱、注册策略、插件密钥、Sandbox 配置或其
他后台字段。对象 URI 在服务端解析成短期签名 URL；单个资源解析失败时记录
日志并省略对应 URL，不阻断整个站点加载。

### 6.3 管理员资源上传

新增管理员专用接口：

```text
POST /api/admin/config/site/assets
Content-Type: multipart/form-data
```

请求字段：

- `asset_type`: `logo` 或 `favicon`
- `file`: 单个图片文件

响应字段：

- `uri`: 后续写入基础配置的对象 URI
- `url`: 当前页面即时预览使用的签名 URL
- `mime_type`
- `size`
- `width`
- `height`

上传约束：

- Logo 仅接受 PNG、JPEG、WebP，最大 2 MiB，最大 2048 x 2048；
- favicon 仅接受 PNG，最大 512 KiB，最大 512 x 512；
- 使用文件内容探测和图片解码校验，不信任文件扩展名或客户端 MIME；
- 拒绝 SVG、HTML、脚本和无法解码的图片；
- 对象键使用服务端随机值，固定写入 `site-brand/` 前缀；
- 路由使用现有系统管理员中间件，普通用户返回 403。

## 7. 前端状态与启动流程

扩展现有 `useCommonConfigStore`，增加 `siteConfig` 和明确的默认值。

`useInitCommonConfig` 在应用启动时请求 `/api/site/config`：

1. 请求成功后规范化字段并写入全局 store；
2. 同步标题、description 和 favicon；
3. 请求失败或超时时使用本地默认配置；
4. 无论成功或失败都结束初始化，不能阻断登录页；
5. `GlobalLayout` 在初始化完成前显示现有 Spin，避免品牌闪烁。

系统管理员保存成功后立即更新全局 store 和浏览器元信息，不要求整页刷新。

## 8. 系统配置页面

在现有 `SystemSettingsSection` 顶部增加“站点配置”卡片，不新增一级或二级路
由。页面沿用当前系统管理的颜色、排版、输入框、按钮和反馈组件。

表单包含：

- 站点名称；
- 站点访问地址（复用 `server_host`）；
- 站点介绍；
- 站点 Logo 上传、预览、替换和恢复默认；
- favicon 上传、预览、替换和恢复默认。

交互要求：

- 图片上传成功只更新表单草稿，不立即影响全站；
- 点击保存后才提交站点字段和资源 URI；
- 离开或刷新前保持现有 dirty-state 规则；
- 保存中禁用重复提交；
- 上传和保存错误分别展示，不互相覆盖；
- revision 冲突时沿用现有刷新提示；
- 删除图片后保存空 URI，恢复内置资源；
- loading、empty、error、retry、disabled 状态完整。

## 9. 持久化与兼容

`basic_config` 当前通过版本化 KV 保存完整 `BasicConfiguration`。新增可选字段
后继续存入同一个记录，不新增数据库表和迁移。

兼容策略：

- 老记录缺少字段时由服务层补默认值；
- 老前端保存旧字段时不会清空站点字段；
- 新前端只提交变更 patch，不覆盖无关配置；
- IDL 新字段使用新的编号，不能复用或改变已有编号；
- 生成的 Go/TypeScript 模型必须通过仓库现有 IDL 流程更新，禁止手改生成文
件。

## 10. 安全与可靠性

- 管理员保存和上传均依赖服务端系统管理员鉴权，不能只靠前端隐藏入口。
- 公共接口使用显式白名单 DTO，不直接序列化完整 `BasicConfiguration`。
- 前端只把站点名称和介绍写入文本节点/DOM 属性，不使用 `innerHTML`。
- 对象存储只保存经过解码校验的图片。
- 日志不记录图片二进制、签名 URL 或完整请求体。
- 上传完成但最终未保存的对象由 `site-brand/` 生命周期策略清理；替换旧资源
时只在新配置成功保存后做最佳努力清理，不能影响主保存结果。

## 11. 预计代码范围

后端及合同：

- `idl/admin/config.thrift`
- `backend/bizpkg/config/base/base.go`
- `backend/api/handler/coze/config_service.go`
- `backend/api/handler/coze/site_config_service.go`
- `backend/api/router/coze/custom_routes.go`
- `backend/application/upload/icon.go`
- 对应生成模型和 targeted tests

前端：

- `frontend/packages/foundation/global-store/src/stores/common-config-store.ts`
- `frontend/packages/foundation/global-adapter/src/hooks/use-app-init/use-init-common-config.ts`
- `frontend/packages/foundation/global-adapter/src/components/global-layout/index.tsx`
- `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/index.tsx`
- `frontend/packages/studio/components/src/coze-brand/index.tsx`
- `frontend/apps/coze-studio/src/components/workspace-mark.tsx`
- `frontend/apps/coze-studio/src/components/workspace-page-top-bar.tsx`
- `frontend/apps/coze-studio/src/pages/workbench/index.tsx`
- `frontend/apps/coze-studio/src/pages/tasks/conversation-turn.tsx`
- `frontend/apps/coze-studio/src/pages/system/service.ts`
- `frontend/apps/coze-studio/src/pages/system/system-settings-section.tsx`
- `frontend/apps/coze-studio/index.html`
- 对应 Vitest

## 12. 验收标准

后端：

- 非管理员不能读取管理员完整配置、保存配置或上传品牌资源；
- 未登录用户可以读取公共品牌白名单；
- 非图片、超限图片、伪造 MIME、非法 URI 和 revision 冲突均被拒绝；
- 旧配置能够无迁移读取并得到默认品牌；
- 保存单个站点字段不会覆盖现有管理员、注册、插件和 Sandbox 配置。

页面：

- `/system/settings` 可以编辑、上传、预览、删除和保存全部五项站点配置；
- `/sign` 在未登录状态下显示配置后的名称、介绍和 Logo；
- 浏览器标题、description 和 favicon 与配置一致；
- 工作空间切换器品牌标记、顶部状态及任务详情用户可见品牌同步更新；
- 工作空间名称、用户头像和业务资源图标不受影响；
- 后端不可用时页面使用默认品牌并可继续登录；
- 桌面和移动宽度下表单、上传卡片、登录页和工作空间头部无溢出；
- in-app browser 控制台无新增错误。
