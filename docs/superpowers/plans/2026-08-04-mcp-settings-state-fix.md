# MCP Settings State Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 MCP 配置面板把加载失败错误展示为只读权限的问题，并为线上验收提供稳定错误文案。

**Architecture:** 保留服务端 MCP 默认关闭与 `503` fail-closed 合同。前端用 `loading / success / error` 表达请求生命周期，`can_manage` 只在 `success` 状态生效；HTTP 错误经过本地投影后显示稳定中文提示。

**Tech Stack:** React、TypeScript、Vitest、Coze API Schema client。

---

### Task 1: 分离请求状态与 MCP 权限状态

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/tools/mcp-settings-panel.tsx:40-185`
- Modify: `frontend/apps/coze-studio/src/pages/tools/mcp-settings-panel.tsx:740-835`
- Test: `frontend/apps/coze-studio/src/pages/tools/__tests__/tools.test.tsx:380-535`

- [x] **Step 1: 写失败测试覆盖等待、503 和真实只读**

在 `ToolsPage` 测试组中增加三个行为测试：

```tsx
it('keeps permission undecided while MCP services are loading', async () => {
  mockList.mockReturnValue(new Promise(() => undefined));
  mockListOfficialCatalog.mockReturnValue(new Promise(() => undefined));

  const { container, root } = await renderMcpSettingsPanel();

  expect(container.textContent).toContain('正在加载 MCP 服务');
  expect(container.textContent).not.toContain('当前空间为只读权限');
  cleanup(container, root);
});

it('does not present unavailable MCP services as readonly permissions', async () => {
  mockList.mockRejectedValue(
    Object.assign(new Error('Request failed with status code 503'), {
      response: { status: 503, data: { msg: 'mcp service disabled' } },
    }),
  );

  const { container, root } = await renderMcpSettingsPanel();

  expect(container.textContent).toContain('当前环境未启用或暂时无法提供 MCP 服务');
  expect(container.textContent).not.toContain('当前空间为只读权限');
  cleanup(container, root);
});

it('shows readonly permissions only after successful permission responses', async () => {
  mockList.mockResolvedValue({ data: { servers: [], total: 0, can_manage: false }, code: 0, msg: '' });
  mockListOfficialCatalog.mockResolvedValue({ data: { entries: [], can_manage: false }, code: 0, msg: '' });

  const { container, root } = await renderMcpSettingsPanel();

  expect(container.textContent).toContain('当前空间为只读权限');
  cleanup(container, root);
});
```

- [x] **Step 2: 运行定向测试并确认 RED**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tools/__tests__/tools.test.tsx
```

Expected: 503 测试失败，页面仍包含原始 Axios 文案或错误的只读提示。

- [x] **Step 3: 实现最小状态模型与错误投影**

在面板中增加请求状态和无 Axios 依赖的错误投影：

```tsx
type MCPSettingsLoadStatus = 'loading' | 'success' | 'error';

const mcpSettingsLoadErrorMessage = (cause: unknown) => {
  const response = (cause as {
    response?: { status?: number; data?: { msg?: unknown } };
  })?.response;
  if (response?.status === 401) {
    return '登录状态已失效，请重新登录';
  }
  if (response?.status === 503) {
    return '当前环境未启用或暂时无法提供 MCP 服务';
  }
  if (typeof response?.data?.msg === 'string' && response.data.msg.trim()) {
    return response.data.msg;
  }
  return '加载 MCP 服务失败，请重试';
};
```

将权限状态改为 `boolean | undefined`。每次加载先进入 `loading` 并清除旧权限；两个请求都成功后进入 `success`，异常时进入 `error` 并保持权限为空。只有
`loadStatus === 'success' && canManage === false` 时渲染只读提示，所有管理操作仅在
`canManage === true` 时启用。

- [x] **Step 4: 运行定向测试并确认 GREEN**

Run:

```bash
cd frontend/apps/coze-studio
npm run test -- src/pages/tools/__tests__/tools.test.tsx
```

Expected: 文件内全部测试通过且没有 React `act` 警告。

- [x] **Step 5: 运行前端验证**

Run:

```bash
cd frontend/apps/coze-studio
npx tsc --noEmit --project tsconfig.json
git diff --check
```

Expected: 两条命令均退出 `0`。

- [x] **Step 6: 提交代码**

```bash
git add docs/superpowers/plans/2026-08-04-mcp-settings-state-fix.md \
  frontend/apps/coze-studio/src/pages/tools/mcp-settings-panel.tsx \
  frontend/apps/coze-studio/src/pages/tools/__tests__/tools.test.tsx
git commit -m "fix: separate MCP load and permission states"
```

### Task 2: 发布前页面验收

**Files:**
- No source changes.

- [ ] **Step 1: 在部署环境提供既有 MCP 启用配置**

确认部署密钥存储已提供 `MCP_AES_AUTH_SECRET`，并按现有运行合同设置
`AGENT_THREAD_MCP_RUNTIME_ENABLED=true`。不得输出或提交密钥值。

- [ ] **Step 2: 重建服务并检查接口**

验证以下接口在管理员登录态下返回 `200`：

```text
GET /api/workbench/mcp_tools?space_id=<current-space-id>
GET /api/workbench/mcp_tools/official_catalog?space_id=<current-space-id>
```

- [ ] **Step 3: 浏览器回归**

使用当前空间管理员账号打开“设置 → MCP 配置”，确认加载期间不显示只读提示、加载
成功后新建按钮可用、页面不再出现 MCP 列表接口的 `401` 或 `503`。
