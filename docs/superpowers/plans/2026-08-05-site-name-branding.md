# 站点名称动态品牌化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让所有第一方可见站点品牌（尤其是页面标题）从系统站点配置读取，且不改动 Coze 协议和 SDK 兼容标识。

**Architecture:** 在底层 `bot-utils` 标题工具中维护当前已配置站点名称，由 `global-adapter` 在应用站点配置时同步写入。所有现有 `renderHtmlTitle` 调用随之使用运行时名称；首屏、登录页和已识别的直接品牌文案使用同一份 zustand 配置或默认 NewX 名称。

**Tech Stack:** React 18、TypeScript、Zustand、Vitest、Rsbuild、Codex in-app browser。

---

### Task 1: 让通用 HTML 标题使用运行时站点名称

**Files:**
- Modify: `frontend/packages/arch/bot-utils/src/html.ts:22-28`
- Modify: `frontend/packages/arch/bot-utils/src/index.ts:26`
- Test: `frontend/packages/arch/bot-utils/__tests__/html.test.tsx:19-30`

- [ ] **Step 1: 写入失败测试，表达配置名称优先级**

  将测试导入替换为：

  ```tsx
  import {
    renderHtmlTitle,
    setHtmlTitleSiteName,
  } from '../src/html';
  ```

  并加入以下用例及清理逻辑：

  ```tsx
  afterEach(() => setHtmlTitleSiteName(undefined));

  test('uses the configured site name in page titles', () => {
    setHtmlTitleSiteName(' NewX AI ');

    expect(renderHtmlTitle('资源库')).equal('资源库 - NewX AI');
    expect(renderHtmlTitle({} as unknown as ReactNode)).equal('NewX AI');
  });
  ```

- [ ] **Step 2: 运行测试并确认失败**

  Run: `npm run test -- __tests__/html.test.tsx`

  Workdir: `frontend/packages/arch/bot-utils`

  Expected: FAIL，因为 `setHtmlTitleSiteName` 尚未导出。

- [ ] **Step 3: 实现最小标题名称注册器**

  在 `html.ts` 中增加以下契约，并让 `renderHtmlTitle` 使用 `resolveHtmlTitleSiteName()`：

  ```ts
  let configuredSiteName: string | undefined;

  export const setHtmlTitleSiteName = (siteName?: string): void => {
    const normalized = siteName?.trim();
    configuredSiteName = normalized || undefined;
  };

  const resolveHtmlTitleSiteName = (): string =>
    configuredSiteName || I18n.t('platform_name');
  ```

  将现有 `const platformName = I18n.t('platform_name')` 改为：

  ```ts
  const platformName = resolveHtmlTitleSiteName();
  ```

  并从 `src/index.ts` 导出 `setHtmlTitleSiteName`。

- [ ] **Step 4: 运行标题工具测试并确认通过**

  Run: `npm run test -- __tests__/html.test.tsx`

  Workdir: `frontend/packages/arch/bot-utils`

  Expected: PASS，现有 i18n 回退测试和新增动态名称测试均通过。

- [ ] **Step 5: 提交标题工具改动**

  ```bash
  git add frontend/packages/arch/bot-utils/src/html.ts \
    frontend/packages/arch/bot-utils/src/index.ts \
    frontend/packages/arch/bot-utils/__tests__/html.test.tsx
  git commit -m "fix: resolve page titles from site config"
  ```

### Task 2: 将站点配置同步到标题工具与 i18n 兜底

**Files:**
- Modify: `frontend/packages/foundation/global-adapter/src/site-config.ts:5-105`
- Modify: `frontend/packages/foundation/global-adapter/src/site-config.test.ts:7-80`
- Modify: `frontend/packages/arch/resources/studio-i18n-resource/src/locales/zh-CN.json:11402`
- Modify: `frontend/packages/arch/resources/studio-i18n-resource/src/locales/en.json:9858`

- [ ] **Step 1: 写入失败测试，验证配置会同步标题名称**

  在 `site-config.test.ts` 中 mock 标题注册器，并在现有“updates title,
  description and favicon”用例中加入：

  ```ts
  expect(setHtmlTitleSiteName).toHaveBeenCalledWith('Acme AI');
  ```

  mock 形状为：

  ```ts
  const setHtmlTitleSiteName = vi.hoisted(() => vi.fn());
  vi.mock('@coze-arch/bot-utils', () => ({ setHtmlTitleSiteName }));
  ```

- [ ] **Step 2: 运行站点配置测试并确认失败**

  Run: `npm run test -- src/site-config.test.ts`

  Workdir: `frontend/packages/foundation/global-adapter`

  Expected: FAIL，因为配置应用流程尚未调用标题名称注册器。

- [ ] **Step 3: 在配置应用边界同步名称**

  在 `site-config.ts` 导入并调用：

  ```ts
  import { setHtmlTitleSiteName } from '@coze-arch/bot-utils';

  // applySiteConfigToDocument 内，在 document.title 之前执行
  setHtmlTitleSiteName(config.siteName);
  ```

  将两份 locale 文件中的 `platform_name` 默认值分别改为 `NewX AI`，使配置尚未
  拉取时也不出现旧站点名称。

- [ ] **Step 4: 运行站点配置测试并确认通过**

  Run: `npm run test -- src/site-config.test.ts`

  Workdir: `frontend/packages/foundation/global-adapter`

  Expected: PASS，保留 favicon、description、请求超时和默认配置覆盖率。

- [ ] **Step 5: 提交配置同步改动**

  ```bash
  git add frontend/packages/foundation/global-adapter/src/site-config.ts \
    frontend/packages/foundation/global-adapter/src/site-config.test.ts \
    frontend/packages/arch/resources/studio-i18n-resource/src/locales/zh-CN.json \
    frontend/packages/arch/resources/studio-i18n-resource/src/locales/en.json
  git commit -m "fix: synchronize site name across titles"
  ```

### Task 3: 替换首屏、登录页和 IM 设置中的可见旧品牌

**Files:**
- Modify: `frontend/apps/coze-studio/rsbuild.config.ts:44-48`
- Modify: `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/index.tsx:17-207`
- Modify: `frontend/packages/foundation/account-ui-adapter/src/pages/login-page/__tests__/index.test.tsx:20-160`
- Modify: `frontend/apps/coze-studio/src/pages/tools/feishu-im-settings-panel.tsx:5-370`
- Modify: `frontend/apps/coze-studio/src/pages/tools/__tests__/feishu-im-settings-panel.test.tsx:1-180`

- [ ] **Step 1: 写入失败的登录页和 IM 文案测试**

  登录页测试中移除 `CozeBrand` mock，新增未配置 Logo 时的断言：

  ```tsx
  it('uses the configured site name when no logo is configured', () => {
    useCommonConfigStore.getState().updateSiteConfig({
      ...DEFAULT_SITE_CONFIG,
      siteName: 'Acme AI',
    });

    render(<LoginPage />);

    expect(screen.getByTestId('login.brand-fallback')).toHaveTextContent('Acme AI');
    expect(screen.queryByText(/Coze|扣子/)).not.toBeInTheDocument();
  });
  ```

  IM 设置测试中将全局站点配置设为 `Acme AI`，令 mock 返回一条可删除配置，点击
  删除后断言 `Modal.confirm` 的 `content` 包含 `Acme AI` 而不包含 `Coze`。

  同时记录首屏静态标题的红灯证据：

  ```bash
  rg -n '扣子 Studio|Coze Studio' rsbuild.config.ts
  ```

  Workdir: `frontend/apps/coze-studio`

  Expected: exits `0` 并命中当前旧标题。

- [ ] **Step 2: 运行两个组件测试并确认失败**

  Run: `npm run test -- src/pages/login-page/__tests__/index.test.tsx`

  Workdir: `frontend/packages/foundation/account-ui-adapter`

  Run: `npm run test -- feishu-im-settings-panel.test.tsx`

  Workdir: `frontend/apps/coze-studio`

  Expected: FAIL，登录页仍渲染 `CozeBrand`，IM 删除文案仍包含 `Coze`。

- [ ] **Step 3: 实现可见品牌替换**

  - 将 Rsbuild `html.title` 改为 `NewX AI`。
  - 登录页移除 `CozeBrand` import 及未使用的 `AUTH_COPY` 旧品牌字段，在 Logo
    缺失时渲染：

    ```tsx
    <span data-testid="login.brand-fallback" aria-label={siteConfig.siteName}>
      {siteConfig.siteName}
    </span>
    ```

  - IM 设置面板导入 `useCommonConfigStore`，读取：

    ```ts
    const siteName = useCommonConfigStore(state => state.siteConfig.siteName);
    ```

    并用 `siteName` 替换删除确认和说明中的 `Coze`。

- [ ] **Step 4: 运行组件测试并确认通过**

  重复 Step 2 的两个命令。

  Expected: PASS，配置名称显示在登录兜底与 IM 文案中，现有凭据安全测试仍通过。

- [ ] **Step 5: 构建首屏并检查静态旧标题已移除**

  Run: `npm run build`

  Workdir: `frontend/apps/coze-studio`

  Then run: `rg -n '扣子 Studio|Coze Studio' dist/index.html`

  Expected: build exits `0`，扫描 exits `1`（无匹配）。

- [ ] **Step 6: 提交可见品牌替换**

  ```bash
  git add frontend/apps/coze-studio/rsbuild.config.ts \
    frontend/apps/coze-studio/src/pages/tools/feishu-im-settings-panel.tsx \
    frontend/apps/coze-studio/src/pages/tools/__tests__/feishu-im-settings-panel.test.tsx \
    frontend/packages/foundation/account-ui-adapter/src/pages/login-page/index.tsx \
    frontend/packages/foundation/account-ui-adapter/src/pages/login-page/__tests__/index.test.tsx
  git commit -m "fix: remove visible legacy site branding"
  ```

### Task 4: 全量验证、残留分类和浏览器自动验收

**Files:**
- Verify: `frontend/apps/coze-studio`
- Verify: `frontend/packages/arch/bot-utils`
- Verify: `frontend/packages/foundation/global-adapter`
- Verify: `frontend/packages/foundation/account-ui-adapter`

- [ ] **Step 1: 运行相关 Vitest**

  Run:

  ```bash
  npm run test -- __tests__/html.test.tsx
  npm run test -- src/site-config.test.ts
  npm run test -- src/pages/login-page/__tests__/index.test.tsx
  npm run test -- feishu-im-settings-panel.test.tsx
  ```

  在对应 package 工作目录分别执行。Expected: 全部 PASS。

- [ ] **Step 2: 运行应用 lint、类型检查和生产构建**

  Run:

  ```bash
  npm run lint
  npm run build
  ```

  Workdir: `frontend/apps/coze-studio`

  Expected: exit `0`；已有非阻塞告警单独记录，不将其伪报为本次引入的问题。

- [ ] **Step 3: 残留扫描并分类**

  Run:

  ```bash
  rg -n --glob '*.ts' --glob '*.tsx' --glob '!**/__tests__/**' \
    --glob '!**/auto-generated/**' --glob '!**/node_modules/**' \
    '扣子|Coze' frontend/apps/coze-studio/src frontend/packages/foundation \
    frontend/packages/studio
  ```

  Expected: 每个命中都被归类为动态可见品牌（应修复）或兼容技术标识（保留），
  不保留未解释的第一方可见旧品牌。

- [ ] **Step 4: 在当前分支镜像上完成浏览器验收**

  使用现有单一本地 Compose 栈，替换为当前分支构建后，在 in-app browser 使用普通
  账号 `840582614` 执行：

  1. 打开工作空间 `7666420680379858944` 并点击资源配置；
  2. 以条件轮询等待标题稳定为 `资源库 - NewX AI`；
  3. 确认 favicon 只有一个 `data-coze-site-config=true` 的站点配置节点；
  4. 检查工作空间、资源配置和登录页都不展示旧品牌；
  5. 记录控制台 error 总数及品牌相关 error 总数，避免输出带签名的资源 URL。

- [ ] **Step 5: 提交验收后必要修正并准备单次集成检查**

  Run:

  ```bash
  git diff --check
  git status --short
  git log --oneline origin/dev..HEAD
  ```

  Expected: 工作区干净、没有 migration、所有提交仅覆盖本任务范围。
