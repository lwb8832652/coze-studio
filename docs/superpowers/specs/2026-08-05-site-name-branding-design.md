# 站点名称动态品牌化设计

## 背景

站点配置已经提供 `site_name`、Logo 和 favicon，但资源配置页仍将浏览器
标题写成“资源库 - 扣子”。真实复现表明，路由切换时标题先恢复为 `NewX AI`，
随后资源库页的 `Helmet` 使用静态 i18n 键 `platform_name` 重写为“扣子”。

因此，站点品牌不能只在全局布局中设置 `document.title`；所有页面标题的
品牌后缀和首屏兜底文案都必须与运行时站点配置使用同一个来源。

## 目标

1. 页面标题的品牌后缀始终来自当前 `site_name`。例如资源库显示
   `资源库 - NewX AI`，而非 `资源库 - 扣子`。
2. 前端首屏、登录页及已识别的第一方可见品牌文案不再显示“扣子”或
   `Coze Studio`，并能在管理员修改站点名称后使用新名称。
3. 保留 API 头、SDK、包名、类型名、兼容字段、开源仓库链接和历史业务术语中的
   `Coze`，避免破坏运行时协议或外部集成。

## 非目标

- 不重命名 `@coze-*` 包、`X-Coze-Space-ID` 等协议标识。
- 不修改生成的 IDL、历史审计记录或第三方 SDK 的公开名称。
- 不改变站点配置 API、持久化模型或管理员权限。

## 设计

### 运行时标题名称来源

在 `@coze-arch/bot-utils` 的 HTML 标题模块维护一个只保存当前站点名称的
框架无关注册入口。`renderHtmlTitle(prefix)` 优先使用该已注册名称拼接标题；
尚未初始化时才回退到 i18n 的 `platform_name`。

`@coze-foundation/global-adapter` 已依赖 `bot-utils`，因此
`applySiteConfigToDocument(config)` 会在更新 document、i18n 和 favicon 前同步
注册 `config.siteName`。这样所有已经使用 `renderHtmlTitle` 的页面会在下一次
渲染中得到同一份动态名称，无须反向依赖全局 store，也不会形成包依赖环。

页面标题构成如下：

```text
配置 API / 默认配置
        ↓
common-config-store.siteConfig.siteName
        ↓
applySiteConfigToDocument
        ↓
bot-utils 标题名称注册器
        ↓
renderHtmlTitle('资源库') → '资源库 - NewX AI'
        ↓
页面 Helmet
```

当标题没有前缀时，标题函数只返回当前站点名称。站点配置请求失败时，现有的
`DEFAULT_SITE_CONFIG`（`NewX AI`）仍是唯一兜底，不重新暴露旧品牌。

### 首屏和可见文案

- 将 Rsbuild HTML 的静态标题从“扣子 Studio”改为默认站点名称 `NewX AI`。
  运行时 API 返回后仍由上述流程覆盖，因此管理员后续改名可生效。
- 登录页继续优先展示配置 Logo；未配置 Logo 时改为展示 `siteConfig.siteName`
  的可访问文字兜底，不再渲染旧 Coze 品牌图标。欢迎语和页脚继续使用同一名称。
- 对当前主应用中的直接可见旧品牌文案（包括 IM 设置说明）改为读取当前
  `siteConfig.siteName`。每个组件通过现有 zustand selector 订阅配置，保证
  保存站点配置后页面能更新。

### 残留审计边界

实施后对 `frontend/apps/coze-studio/src` 和第一方 foundation/studio UI 包执行
残留扫描。每个命中按以下规则分类：

| 类型 | 处理 |
| --- | --- |
| 用户可见站点品牌、浏览器标题、欢迎语、页脚、说明文案 | 改为动态站点名称或默认 NewX 名称 |
| API 头、SDK 类型、包名、路径、仓库链接、遥测字段 | 保留 |
| 生成 IDL、测试 mock、历史数据说明 | 保留，除非测试断言的是用户可见品牌 |

## 测试与验收

1. `bot-utils` 单测先验证：未注册时保留 i18n 回退；注册 `NewX AI` 后，
   `renderHtmlTitle('资源库')` 返回 `资源库 - NewX AI`；清除注册后回退恢复。
2. `global-adapter` 单测验证应用站点配置会更新标题名称注册器，并保持现有
   description、favicon 和 i18n 更新行为。
3. 登录页和 IM 设置页的相关组件测试验证配置名称被渲染，且没有旧品牌文本。
4. 前端构建验证 Rsbuild 静态标题不含旧品牌。
5. in-app browser 使用普通账号 `840582614`：从工作空间点击资源配置，等待标题
   稳定为 `资源库 - NewX AI`；确认 favicon 是一个受站点配置管理的节点；检查
   工作空间、资源配置和登录入口的可见品牌；控制台无品牌相关错误。

## 风险与处理

- 站点配置异步完成前可能短暂显示默认 `NewX AI`，但绝不显示旧品牌；配置返回后
  将在当前页面和后续路由标题中更新。
- 标题注册器只负责站点名称，不承担 React 状态管理；依赖现有全局配置更新和页面
  渲染，避免把 store 引入底层 `bot-utils` 包。
- 若残留扫描发现跨产品或 SDK 的公开 `Coze` 名称，将记录为兼容项，不在本次替换，
  防止破坏协议。
