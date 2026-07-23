/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/* eslint-disable @typescript-eslint/naming-convention -- Test doubles preserve external PascalCase component and icon exports. */

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockOpenSettings = vi.hoisted(() => vi.fn());

vi.mock('@coze-foundation/global-adapter/account-settings', () => ({
  useAccountSettings: () => ({
    node: <div data-testid="model-settings-node" />,
    open: mockOpenSettings,
  }),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozArrowDown: () => <span aria-hidden="true" />,
  IconCozCheckMark: () => <span aria-hidden="true">checked</span>,
  IconCozEdit: () => <span aria-hidden="true">edit</span>,
  IconCozMagnifier: () => <span aria-hidden="true" />,
  IconCozPlus: () => <span aria-hidden="true">plus</span>,
  IconCozTrashCan: () => <span aria-hidden="true">delete</span>,
}));

vi.mock('../../../tools/workspace-model-settings-panel', () => ({
  WORKSPACE_MODEL_SETTINGS_TAB_ID: 'workspace-models',
  WorkspaceModelSettingsPanel: () => <div />,
}));

import { WorkbenchModelSelector } from '../workbench-model-selector';

describe('WorkbenchModelSelector Nuwax alignment', () => {
  it('shows model descriptions and opens workspace model creation', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    act(() => {
      root.render(
        <WorkbenchModelSelector
          loading={false}
          models={[
            {
              display_name: 'DeepSeek V4 Pro',
              model_name: 'deepseek-v4-pro',
              model_type: 19,
              model_class_name: 'DeepSeek',
              description: '适合复杂推理与代码任务',
              workspace_can_manage: true,
              workspace_model_id: 'workspace-model-1',
              workspace_model_can_manage: true,
            },
          ]}
          onChange={vi.fn()}
          onModelsChanged={vi.fn()}
          onOpenChange={vi.fn()}
          open
          placement="top"
          spaceId="space-1"
          value={19}
        />,
      );
    });

    expect(container.textContent).toContain('适合复杂推理与代码任务');
    expect(container.querySelector('[data-placement="top"]')).not.toBeNull();
    expect(
      container.querySelector('button[aria-label="编辑 DeepSeek V4 Pro"]'),
    ).not.toBeNull();

    const addButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('模型'),
    );
    await act(async () => {
      addButton?.click();
      await Promise.resolve();
    });
    expect(mockOpenSettings).toHaveBeenCalledWith('workspace-models');

    act(() => root.unmount());
    container.remove();
  });
});
