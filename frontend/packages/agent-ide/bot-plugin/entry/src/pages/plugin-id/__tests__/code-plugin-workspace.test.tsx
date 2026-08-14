/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/* eslint-disable @typescript-eslint/naming-convention -- Test mocks mirror the public component export names. */

import { type ReactNode } from 'react';

import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react';
import type {
  CodePluginDebugData,
  CodePluginDraftData,
  DebugCodePluginRequest,
  SaveCodePluginDraftRequest,
} from '@coze-studio/api-schema/plugin-develop';
import { CreationMethod, PluginType } from '@coze-arch/bot-api/plugin_develop';

import { CodePluginWorkspace, isCodePlugin } from '../code-plugin-workspace';
import { DEFAULT_CODE_PLUGIN_SCHEMA } from '../code-plugin-schema';

const apiMocks = vi.hoisted(() => ({
  getDraft: vi.fn(),
  saveDraft: vi.fn(),
  debug: vi.fn(),
}));

vi.mock('@coze-studio/api-schema/plugin-develop', () => ({
  plugin_develop_common: {
    CodePluginRuntime: { Python: 1, JavaScript: 2 },
    CodePluginDebugStatus: {
      Success: 1,
      RuntimeError: 2,
      Timeout: 3,
      Capacity: 4,
      OutputLimit: 5,
      Canceled: 6,
      Unavailable: 7,
    },
  },
  GetCodePluginDraft: apiMocks.getDraft,
  SaveCodePluginDraft: apiMocks.saveDraft,
  DebugCodePlugin: apiMocks.debug,
}));

vi.mock('@coze-arch/bot-monaco-editor', () => ({
  Editor: ({
    value,
    onChange,
    options,
  }: {
    value: string;
    onChange: (value: string) => void;
    options?: { readOnly?: boolean };
  }) => (
    <textarea
      aria-label="mock-code-editor"
      value={value}
      readOnly={options?.readOnly}
      onChange={event => onChange(event.target.value)}
    />
  ),
}));

vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    children,
    onClick,
    disabled,
  }: {
    children: ReactNode;
    onClick?: () => void;
    disabled?: boolean;
  }) => (
    <button type="button" disabled={disabled} onClick={onClick}>
      {children}
    </button>
  ),
  Select: ({
    value,
    onChange,
    optionList,
    disabled,
  }: {
    value: number;
    onChange: (value: number) => void;
    optionList: Array<{ label: string; value: number }>;
    disabled?: boolean;
  }) => (
    <select
      aria-label="代码运行时"
      value={value}
      disabled={disabled}
      onChange={event => onChange(Number(event.target.value))}
    >
      {optionList.map(option => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  ),
  Tag: ({ children }: { children: ReactNode }) => <span>{children}</span>,
  TextArea: ({
    id,
    value,
    onChange,
    disabled,
  }: {
    id?: string;
    value: string;
    onChange: (value: string) => void;
    disabled?: boolean;
  }) => (
    <textarea
      id={id}
      value={value}
      disabled={disabled}
      onChange={event => onChange(event.target.value)}
    />
  ),
  Toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
  Typography: {
    Title: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
    Text: ({ children }: { children: ReactNode }) => <span>{children}</span>,
  },
}));

const draft = {
  plugin_id: '100',
  space_id: '200',
  runtime: 1,
  entry_file: 'main.py',
  files: [{ path: 'main.py', content: 'async def main(args):\n  return {}' }],
  revision: 1,
  last_debugged_revision: 1,
  debug_ready: true,
  input_schema_json:
    '{"type":"object","properties":{"message":{"type":"string"}}}',
  output_schema_json:
    '{"type":"object","properties":{"result":{"type":"string"}}}',
} satisfies CodePluginDraftData;

const successfulDebug = {
  success: true,
  status: 1,
  result: '{"result":2}',
  reason: '',
  duration_ms: 12,
  output_bytes: 12,
  revision: 2,
} satisfies CodePluginDebugData;

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => {
    resolve = done;
  });
  return { promise, resolve };
};

describe('CodePluginWorkspace', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiMocks.getDraft.mockResolvedValue({ data: draft, code: 0, msg: '' });
  });

  it('restores a debug-ready draft and enables publishing', async () => {
    const onPublishReadyChange = vi.fn();

    render(
      <CodePluginWorkspace
        pluginID="100"
        spaceID="200"
        canEdit
        onPublishReadyChange={onPublishReadyChange}
      />,
    );

    const editor = await screen.findByLabelText('mock-code-editor');
    expect((editor as HTMLTextAreaElement).value).toBe(draft.files[0].content);
    await waitFor(() =>
      expect(onPublishReadyChange).toHaveBeenLastCalledWith(true),
    );
    expect(screen.getByText('可发布')).toBeTruthy();

    fireEvent.click(screen.getByRole('tab', { name: '输入结构' }));
    expect(
      (screen.getByLabelText('输入结构 JSON') as HTMLTextAreaElement).value,
    ).toContain('"message"');

    fireEvent.click(screen.getByRole('tab', { name: '输出结构' }));
    expect(
      (screen.getByLabelText('输出结构 JSON') as HTMLTextAreaElement).value,
    ).toContain('"result"');
  });

  it('uses the default object schema when the draft omits schemas', async () => {
    apiMocks.getDraft.mockResolvedValue({
      data: {
        ...draft,
        input_schema_json: undefined,
        output_schema_json: undefined,
      },
      code: 0,
      msg: '',
    });

    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);

    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('tab', { name: '输入结构' }));
    expect(
      (screen.getByLabelText('输入结构 JSON') as HTMLTextAreaElement).value,
    ).toBe(DEFAULT_CODE_PLUGIN_SCHEMA);
  });

  it('saves normalized schemas and marks schema edits as unpublished', async () => {
    const onPublishReadyChange = vi.fn();
    apiMocks.saveDraft.mockResolvedValue({
      data: {
        ...draft,
        revision: 2,
        last_debugged_revision: 1,
        input_schema_json:
          '{"type":"object","properties":{"query":{"type":"string"}}}',
      },
      code: 0,
      msg: '',
    });

    render(
      <CodePluginWorkspace
        pluginID="100"
        spaceID="200"
        canEdit
        onPublishReadyChange={onPublishReadyChange}
      />,
    );

    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('tab', { name: '输入结构' }));
    fireEvent.change(screen.getByLabelText('输入结构 JSON'), {
      target: {
        value: '{"type":"object","properties":{"query":{"type":"string"}}}',
      },
    });

    await waitFor(() =>
      expect(onPublishReadyChange).toHaveBeenLastCalledWith(false),
    );
    expect(screen.getByText('有未保存修改')).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    await waitFor(() => expect(apiMocks.saveDraft).toHaveBeenCalledTimes(1));
    expect(apiMocks.saveDraft).toHaveBeenCalledWith(
      expect.objectContaining({
        revision: 1,
        input_schema_json:
          '{\n  "type": "object",\n  "properties": {\n    "query": {\n      "type": "string"\n    }\n  }\n}',
        output_schema_json:
          '{\n  "type": "object",\n  "properties": {\n    "result": {\n      "type": "string"\n    }\n  }\n}',
      }),
    );
  });

  it('blocks save and shows an inline error for an invalid schema', async () => {
    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);

    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('tab', { name: '输出结构' }));
    fireEvent.change(screen.getByLabelText('输出结构 JSON'), {
      target: { value: '{"type":"array"}' },
    });
    fireEvent.click(screen.getByRole('button', { name: '保存' }));

    expect(
      await screen.findByText('参数结构的 type 必须是 object'),
    ).toBeTruthy();
    expect(apiMocks.saveDraft).not.toHaveBeenCalled();
  });

  it('saves dirty source and schemas before debugging the returned revision', async () => {
    apiMocks.getDraft
      .mockResolvedValueOnce({
        data: { ...draft, last_debugged_revision: 0, debug_ready: false },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          ...draft,
          revision: 2,
          last_debugged_revision: 2,
          debug_ready: true,
        },
        code: 0,
        msg: '',
      });
    apiMocks.saveDraft.mockResolvedValue({
      data: {
        ...draft,
        revision: 2,
        last_debugged_revision: 0,
        files: [
          { path: 'main.py', content: 'async def main(args):\n  return 2' },
        ],
        input_schema_json: draft.input_schema_json,
        output_schema_json: draft.output_schema_json,
      },
      code: 0,
      msg: '',
    });
    apiMocks.debug.mockResolvedValue({
      data: successfulDebug,
      code: 0,
      msg: '',
    });

    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);

    const editor = await screen.findByLabelText('mock-code-editor');
    fireEvent.change(editor, {
      target: { value: 'async def main(args):\n  return 2' },
    });
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));

    await waitFor(() => expect(apiMocks.saveDraft).toHaveBeenCalledTimes(1));
    expect(apiMocks.saveDraft).toHaveBeenCalledWith(
      expect.objectContaining({
        plugin_id: '100',
        space_id: '200',
        revision: 1,
        input_schema_json: expect.stringContaining('"message"'),
        output_schema_json: expect.stringContaining('"result"'),
      }),
    );
    await waitFor(() =>
      expect(apiMocks.debug).toHaveBeenCalledWith(
        expect.objectContaining({ revision: 2 }),
      ),
    );
    expect(await screen.findByText('{"result":2}')).toBeTruthy();
    const saveRequest = apiMocks.saveDraft.mock
      .calls[0][0] as SaveCodePluginDraftRequest;
    const debugRequest = apiMocks.debug.mock
      .calls[0][0] as DebugCodePluginRequest;
    expect(saveRequest.revision).toBe(1);
    expect(debugRequest.revision).toBe(2);
  });

  it('saves a clean revision-zero draft before debugging the returned revision', async () => {
    const revisionZeroDraft = {
      ...draft,
      revision: 0,
      last_debugged_revision: 0,
      debug_ready: false,
    } satisfies CodePluginDraftData;
    const savedDraft = {
      ...revisionZeroDraft,
      revision: 1,
    } satisfies CodePluginDraftData;
    apiMocks.getDraft
      .mockResolvedValueOnce({ data: revisionZeroDraft, code: 0, msg: '' })
      .mockResolvedValueOnce({
        data: {
          ...savedDraft,
          last_debugged_revision: 1,
          debug_ready: true,
        },
        code: 0,
        msg: '',
      });
    apiMocks.saveDraft.mockResolvedValue({
      data: savedDraft,
      code: 0,
      msg: '',
    });
    apiMocks.debug.mockResolvedValue({
      data: { ...successfulDebug, revision: 1 },
      code: 0,
      msg: '',
    });

    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);

    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));

    await waitFor(() => expect(apiMocks.saveDraft).toHaveBeenCalledTimes(1));
    expect(apiMocks.saveDraft).toHaveBeenCalledWith(
      expect.objectContaining({ revision: 0 }),
    );
    await waitFor(() =>
      expect(apiMocks.debug).toHaveBeenCalledWith(
        expect.objectContaining({ revision: 1 }),
      ),
    );
  });

  it('debugs a clean persisted draft without saving it again', async () => {
    apiMocks.getDraft
      .mockResolvedValueOnce({ data: draft, code: 0, msg: '' })
      .mockResolvedValueOnce({ data: draft, code: 0, msg: '' });
    apiMocks.debug.mockResolvedValue({
      data: { ...successfulDebug, revision: 1 },
      code: 0,
      msg: '',
    });

    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);

    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));

    await waitFor(() => expect(apiMocks.debug).toHaveBeenCalledTimes(1));
    expect(apiMocks.saveDraft).not.toHaveBeenCalled();
    expect(apiMocks.debug).toHaveBeenCalledWith(
      expect.objectContaining({ revision: 1 }),
    );
  });

  it('does not debug a revision-zero draft when its automatic save fails', async () => {
    apiMocks.getDraft.mockResolvedValueOnce({
      data: {
        ...draft,
        revision: 0,
        last_debugged_revision: 0,
        debug_ready: false,
      },
      code: 0,
      msg: '',
    });
    apiMocks.saveDraft.mockRejectedValue(new Error('session expired'));

    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);

    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));

    await waitFor(() => expect(apiMocks.saveDraft).toHaveBeenCalledTimes(1));
    expect(await screen.findByText('试运行失败，请稍后重试')).toBeTruthy();
    expect(apiMocks.debug).not.toHaveBeenCalled();
  });

  it('keeps publishing disabled when the server does not confirm debug_ready', async () => {
    const onPublishReadyChange = vi.fn();
    apiMocks.getDraft
      .mockResolvedValueOnce({
        data: { ...draft, last_debugged_revision: 0, debug_ready: false },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: { ...draft, last_debugged_revision: 1, debug_ready: false },
        code: 0,
        msg: '',
      });
    apiMocks.debug.mockResolvedValue({
      data: { ...successfulDebug, revision: 1 },
      code: 0,
      msg: '',
    });

    render(
      <CodePluginWorkspace
        pluginID="100"
        spaceID="200"
        canEdit
        onPublishReadyChange={onPublishReadyChange}
      />,
    );
    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));

    await waitFor(() => expect(apiMocks.getDraft).toHaveBeenCalledTimes(2));
    expect(onPublishReadyChange).toHaveBeenLastCalledWith(false);
    expect(
      await screen.findByText('服务端尚未确认发布资格，请重新试运行'),
    ).toBeTruthy();
  });

  it('ignores a stale draft response after switching plugins', async () => {
    const oldRequest = deferred<{
      data: CodePluginDraftData;
      code: number;
      msg: string;
    }>();
    apiMocks.getDraft
      .mockReturnValueOnce(oldRequest.promise)
      .mockResolvedValueOnce({
        data: {
          ...draft,
          plugin_id: '101',
          files: [{ path: 'main.py', content: 'new plugin source' }],
        },
        code: 0,
        msg: '',
      });

    const view = render(
      <CodePluginWorkspace pluginID="100" spaceID="200" canEdit />,
    );
    view.rerender(<CodePluginWorkspace pluginID="101" spaceID="200" canEdit />);
    expect(
      (
        (await screen.findByLabelText(
          'mock-code-editor',
        )) as HTMLTextAreaElement
      ).value,
    ).toBe('new plugin source');

    oldRequest.resolve({ data: draft, code: 0, msg: '' });
    await waitFor(() =>
      expect(
        (screen.getByLabelText('mock-code-editor') as HTMLTextAreaElement)
          .value,
      ).toBe('new plugin source'),
    );
  });

  it('ignores a stale save response after switching plugins', async () => {
    const pendingSave = deferred<{
      data: CodePluginDraftData;
      code: number;
      msg: string;
    }>();
    const nextDraft: CodePluginDraftData = {
      ...draft,
      plugin_id: '101',
      revision: 7,
      last_debugged_revision: 0,
      debug_ready: false,
      files: [{ path: 'main.py', content: 'new plugin source' }],
    };
    apiMocks.getDraft
      .mockResolvedValueOnce({ data: draft, code: 0, msg: '' })
      .mockResolvedValueOnce({ data: nextDraft, code: 0, msg: '' });
    apiMocks.saveDraft.mockReturnValueOnce(pendingSave.promise);

    const view = render(
      <CodePluginWorkspace pluginID="100" spaceID="200" canEdit />,
    );
    const editor = await screen.findByLabelText('mock-code-editor');
    fireEvent.change(editor, {
      target: { value: 'old plugin changed source' },
    });
    fireEvent.click(screen.getByRole('button', { name: '保存' }));
    await waitFor(() => expect(apiMocks.saveDraft).toHaveBeenCalledTimes(1));

    view.rerender(<CodePluginWorkspace pluginID="101" spaceID="200" canEdit />);
    await waitFor(() =>
      expect(
        (screen.getByLabelText('mock-code-editor') as HTMLTextAreaElement)
          .value,
      ).toBe('new plugin source'),
    );

    await act(async () => {
      pendingSave.resolve({
        data: {
          ...draft,
          revision: 2,
          files: [{ path: 'main.py', content: 'stale saved source' }],
        },
        code: 0,
        msg: '',
      });
      await pendingSave.promise;
    });

    expect(
      (screen.getByLabelText('mock-code-editor') as HTMLTextAreaElement).value,
    ).toBe('new plugin source');
    expect(screen.getByText('Revision 7')).toBeTruthy();
    expect(screen.queryByText('可发布')).toBeNull();
  });

  it('ignores a stale debug response after switching plugins', async () => {
    const pendingDebug = deferred<{
      data: CodePluginDebugData;
      code: number;
      msg: string;
    }>();
    const nextDraft: CodePluginDraftData = {
      ...draft,
      plugin_id: '101',
      revision: 3,
      last_debugged_revision: 0,
      debug_ready: false,
      files: [{ path: 'main.py', content: 'new plugin source' }],
    };
    apiMocks.getDraft
      .mockResolvedValueOnce({ data: draft, code: 0, msg: '' })
      .mockResolvedValueOnce({ data: nextDraft, code: 0, msg: '' });
    apiMocks.debug.mockReturnValueOnce(pendingDebug.promise);

    const view = render(
      <CodePluginWorkspace pluginID="100" spaceID="200" canEdit />,
    );
    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));
    await waitFor(() => expect(apiMocks.debug).toHaveBeenCalledTimes(1));

    view.rerender(<CodePluginWorkspace pluginID="101" spaceID="200" canEdit />);
    await waitFor(() =>
      expect(
        (screen.getByLabelText('mock-code-editor') as HTMLTextAreaElement)
          .value,
      ).toBe('new plugin source'),
    );

    await act(async () => {
      pendingDebug.resolve({
        data: { ...successfulDebug, revision: 1 },
        code: 0,
        msg: '',
      });
      await pendingDebug.promise;
    });

    expect(screen.queryByText('{"result":2}')).toBeNull();
    expect(screen.queryByText('可发布')).toBeNull();
  });

  it('revokes stale publish readiness immediately while the next plugin loads', async () => {
    const onPublishReadyChange = vi.fn();
    const nextLoad = deferred<{
      data: CodePluginDraftData;
      code: number;
      msg: string;
    }>();
    apiMocks.getDraft
      .mockResolvedValueOnce({ data: draft, code: 0, msg: '' })
      .mockReturnValueOnce(nextLoad.promise);

    const view = render(
      <CodePluginWorkspace
        pluginID="100"
        spaceID="200"
        canEdit
        onPublishReadyChange={onPublishReadyChange}
      />,
    );
    await screen.findByLabelText('mock-code-editor');
    await waitFor(() =>
      expect(onPublishReadyChange).toHaveBeenLastCalledWith(true),
    );

    view.rerender(
      <CodePluginWorkspace
        pluginID="101"
        spaceID="200"
        canEdit
        onPublishReadyChange={onPublishReadyChange}
      />,
    );
    expect(onPublishReadyChange).toHaveBeenLastCalledWith(false);
    expect(screen.queryByText('可发布')).toBeNull();

    await act(async () => {
      nextLoad.resolve({
        data: {
          ...draft,
          plugin_id: '101',
          debug_ready: false,
          last_debugged_revision: 0,
        },
        code: 0,
        msg: '',
      });
      await nextLoad.promise;
    });
  });

  it('prevents duplicate save and debug requests synchronously', async () => {
    const pendingSave = deferred<{
      data: CodePluginDraftData;
      code: number;
      msg: string;
    }>();
    apiMocks.saveDraft.mockReturnValue(pendingSave.promise);

    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);
    const editor = await screen.findByLabelText('mock-code-editor');
    fireEvent.change(editor, { target: { value: 'changed source' } });
    const saveButton = screen.getByRole('button', { name: '保存' });
    fireEvent.click(saveButton);
    fireEvent.click(saveButton);
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));

    expect(apiMocks.saveDraft).toHaveBeenCalledTimes(1);
    expect(apiMocks.debug).not.toHaveBeenCalled();
    await act(async () => {
      pendingSave.resolve({
        data: { ...draft, revision: 2, debug_ready: false },
        code: 0,
        msg: '',
      });
      await pendingSave.promise;
    });
  });

  it('locks editing while a debug request is running', async () => {
    const pendingDebug = deferred<{
      data: CodePluginDebugData;
      code: number;
      msg: string;
    }>();
    apiMocks.debug.mockReturnValue(pendingDebug.promise);

    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);
    const editor = await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));

    expect((editor as HTMLTextAreaElement).readOnly).toBe(true);
    expect(
      (screen.getByRole('button', { name: '保存' }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);

    await act(async () => {
      pendingDebug.resolve({
        data: { ...successfulDebug, revision: 1 },
        code: 0,
        msg: '',
      });
      await pendingDebug.promise;
    });
  });

  it('switches to JavaScript with the canonical entry file', async () => {
    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);
    await screen.findByLabelText('mock-code-editor');

    fireEvent.change(screen.getByLabelText('代码运行时'), {
      target: { value: '2' },
    });

    expect(screen.getAllByText('index.js').length).toBeGreaterThan(0);
    expect(screen.getByText('有未保存修改')).toBeTruthy();
  });

  it('reports output limits using the generated debug contract', async () => {
    apiMocks.debug.mockResolvedValue({
      data: {
        ...successfulDebug,
        success: false,
        status: 5,
        result: '',
        reason: '输出超过允许大小',
        revision: 1,
      } satisfies CodePluginDebugData,
      code: 0,
      msg: '',
    });
    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);
    await screen.findByLabelText('mock-code-editor');
    fireEvent.click(screen.getByRole('button', { name: '试运行' }));

    expect(await screen.findByText('输出超限')).toBeTruthy();
    expect(screen.getByText('输出超过允许大小')).toBeTruthy();
  });

  it('supports roving tab focus and arrow-key navigation', async () => {
    render(<CodePluginWorkspace pluginID="100" spaceID="200" canEdit />);
    await screen.findByLabelText('mock-code-editor');
    const debugTab = screen.getByRole('tab', { name: '试运行' });
    const inputTab = screen.getByRole('tab', { name: '输入结构' });
    expect(debugTab.getAttribute('tabindex')).toBe('0');
    expect(inputTab.getAttribute('tabindex')).toBe('-1');

    fireEvent.keyDown(debugTab, { key: 'ArrowRight' });
    expect(inputTab.getAttribute('tabindex')).toBe('0');
    expect(document.activeElement).toBe(inputTab);
    expect(inputTab.getAttribute('aria-controls')).toBeTruthy();
    expect(screen.getByRole('tabpanel').getAttribute('aria-labelledby')).toBe(
      inputTab.id,
    );
  });
});

describe('isCodePlugin', () => {
  it('only accepts FUNC + IDE', () => {
    expect(isCodePlugin(PluginType.FUNC, CreationMethod.IDE)).toBe(true);
    expect(isCodePlugin(PluginType.PLUGIN, CreationMethod.IDE)).toBe(false);
    expect(isCodePlugin(PluginType.FUNC, CreationMethod.COZE)).toBe(false);
  });
});
