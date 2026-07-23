/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/* eslint-disable @typescript-eslint/naming-convention -- Test doubles preserve external PascalCase component exports. */

import { type ReactNode } from 'react';

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockListModels = vi.hoisted(() => vi.fn());

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => ({ user_id_str: '7' }),
}));

vi.mock('@coze-studio/api-schema/workbench-model', () => ({
  WorkspaceModelScope: { System: 1, Space: 2 },
  ListWorkspaceModels: mockListModels,
  UpsertWorkspaceModel: vi.fn(),
  TestWorkspaceModel: vi.fn(),
  SetWorkspaceModelStatus: vi.fn(),
  DeleteWorkspaceModel: vi.fn(),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozPlus: () => <span aria-hidden="true" />,
  IconCozRefresh: () => <span aria-hidden="true" />,
  IconCozMagnifier: () => <span aria-hidden="true" />,
}));

vi.mock('@coze-arch/coze-design', () => {
  const Button = ({
    children,
    disabled,
    loading,
    onClick,
  }: {
    children?: ReactNode;
    disabled?: boolean;
    loading?: boolean;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      disabled={disabled || loading}
      onClick={() => onClick?.()}
    >
      {children}
    </button>
  );
  const Input = ({
    autoComplete,
    maxLength,
    name,
    onChange,
    placeholder,
    value,
  }: {
    autoComplete?: string;
    maxLength?: number;
    name?: string;
    onChange?: (value: string) => void;
    placeholder?: string;
    value?: string;
  }) => (
    <input
      autoComplete={autoComplete}
      maxLength={maxLength}
      name={name}
      placeholder={placeholder}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  );
  const Modal = ({
    cancelText,
    children,
    okText,
    onCancel,
    onOk,
    title,
    visible,
  }: {
    cancelText?: string;
    children?: ReactNode;
    okText?: string;
    onCancel?: () => void;
    onOk?: () => void;
    title?: ReactNode;
    visible?: boolean;
  }) =>
    visible ? (
      <section role="dialog">
        <h2>{title}</h2>
        {children}
        <button type="button" onClick={onCancel}>
          {cancelText}
        </button>
        <button type="button" onClick={onOk}>
          {okText}
        </button>
      </section>
    ) : null;
  const Switch = ({ checked }: { checked?: boolean }) => (
    <button aria-checked={checked} role="switch" type="button" />
  );
  return {
    Button,
    Input,
    Modal,
    Spin: ({ children }: { children?: ReactNode }) => <>{children}</>,
    Switch,
  };
});

import { WorkspaceModelSettingsPanel } from '../workspace-model-settings-panel';

const listResponse = (canManage: boolean) => ({
  code: 0,
  msg: 'success',
  data: {
    can_manage: canManage,
    providers: [{ key: 'deepseek', name: 'DeepSeek 模型', model_class: 19 }],
    system_models: [],
    workspace_models: [
      {
        id: 'space-1',
        scope: 2,
        provider_key: 'deepseek',
        model_class: 19,
        display_name: '团队 DeepSeek',
        model_identifier: 'deepseek-reasoner',
        protocol: 'openai-compatible',
        enabled: true,
        credential_configured: true,
        can_manage: canManage,
        creator_id: '7',
        description: '用于复杂推理和代码生成',
        capabilities: ['text', 'reasoning'],
        usage_scenarios: ['chat', 'agent', 'workflow', 'appdev'],
        max_context_tokens: 128000,
        max_output_tokens: 4096,
        function_call_mode: 'native',
        endpoints: [
          {
            id: 'endpoint-1',
            base_url: 'https://api.deepseek.com/v1',
            credential_configured: true,
            weight: 1,
            enabled: true,
          },
        ],
      },
      {
        id: 'space-2',
        scope: 2,
        provider_key: 'deepseek',
        model_class: 19,
        display_name: '其他成员模型',
        model_identifier: 'deepseek-chat',
        protocol: 'openai-compatible',
        enabled: true,
        credential_configured: true,
        can_manage: canManage,
        creator_id: '8',
        endpoints: [
          {
            id: 'endpoint-2',
            base_url: 'https://api.deepseek.com/v1',
            credential_configured: true,
            weight: 1,
            enabled: true,
          },
        ],
      },
    ],
  },
});

describe('WorkspaceModelSettingsPanel', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    mockListModels.mockResolvedValue(listResponse(true));
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it('shows the Nuwax component-resource model manager without system scope', async () => {
    await act(async () => {
      root.render(<WorkspaceModelSettingsPanel spaceId="101" />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('模型管理');
    expect(container.textContent).toContain('团队 DeepSeek');
    expect(container.textContent).toContain('所有人');
    expect(container.textContent).toContain('由我创建');
    expect(container.textContent).not.toContain('系统模型');

    const createButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '模型',
    );
    expect(createButton).toBeTruthy();

    act(() => createButton?.click());
    const credentialInput = container.querySelector<HTMLInputElement>(
      'input[name="workspace-model-api-key"]',
    );
    expect(credentialInput).toBeTruthy();
    expect(credentialInput?.autocomplete).toBe('new-password');
    expect(container.textContent).toContain('模型类型');
    expect(container.textContent).toContain('可用范围');
    expect(container.textContent).toContain('模型连通性测试');
    expect(container.textContent).toContain('Endpoint 1');
  });

  it('filters current-space models by creator', async () => {
    await act(async () => {
      root.render(<WorkspaceModelSettingsPanel spaceId="101" />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('其他成员模型');
    const mineButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '由我创建',
    );
    act(() => mineButton?.click());
    expect(container.textContent).toContain('团队 DeepSeek');
    expect(container.textContent).not.toContain('其他成员模型');
  });

  it('keeps workspace models read-only for ordinary members', async () => {
    mockListModels.mockResolvedValue(listResponse(false));
    await act(async () => {
      root.render(<WorkspaceModelSettingsPanel spaceId="101" />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('仅工作空间所有者或管理员可维护');
    expect(container.textContent).not.toContain('新增模型');
  });
});
