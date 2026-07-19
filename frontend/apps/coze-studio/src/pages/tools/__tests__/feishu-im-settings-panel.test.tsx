/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

import { type ReactNode } from 'react';

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockListConfigs = vi.hoisted(() => vi.fn());
const mockListAgents = vi.hoisted(() => vi.fn());

vi.mock('../feishu-im-service', () => ({
  createFeishuIMConfig: vi.fn(),
  deleteFeishuIMConfig: vi.fn(),
  listFeishuIMConfigs: mockListConfigs,
  listPublishedAgentTargets: mockListAgents,
  setFeishuIMConfigEnabled: vi.fn(),
  testFeishuIMConfig: vi.fn(),
  updateFeishuIMConfig: vi.fn(),
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
    mode,
    name,
    onChange,
    placeholder,
    value,
  }: {
    autoComplete?: string;
    maxLength?: number;
    mode?: string;
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
      type={mode === 'password' ? 'password' : 'text'}
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
  Modal.confirm = vi.fn();
  const Select = () => <select aria-label="select" />;
  const Switch = () => <button type="button" role="switch" />;
  return {
    Button,
    Input,
    Modal,
    Select,
    // eslint-disable-next-line @typescript-eslint/naming-convention
    Spin: ({ children }: { children?: ReactNode }) => <>{children}</>,
    Switch,
  };
});

import { FeishuIMSettingsPanel } from '../feishu-im-settings-panel';

describe('FeishuIMSettingsPanel', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(async () => {
    mockListConfigs.mockResolvedValue({
      configs: [],
      can_manage: true,
      credential_ready: true,
    });
    mockListAgents.mockResolvedValue([
      {
        id: '1',
        name: '飞书验收助手',
        published: true,
      },
    ]);
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    await act(async () => {
      root.render(<FeishuIMSettingsPanel spaceId="space-1" />);
      await Promise.resolve();
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it('does not expose Feishu credential fields to login autofill', () => {
    const addButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '+ 添加飞书机器人',
    );
    expect(addButton).toBeTruthy();

    act(() => {
      addButton?.click();
    });

    const appID = container.querySelector<HTMLInputElement>(
      'input[placeholder="cli_xxxxxxxxxxxxxxxx"]',
    );
    const appSecret = container.querySelector<HTMLInputElement>(
      'input[placeholder="请输入 App Secret"]',
    );

    expect(appID?.name).toBe('feishu-app-id');
    expect(appID?.autocomplete).toBe('off');
    expect(appSecret?.name).toBe('feishu-app-secret');
    expect(appSecret?.autocomplete).toBe('new-password');
  });
});
