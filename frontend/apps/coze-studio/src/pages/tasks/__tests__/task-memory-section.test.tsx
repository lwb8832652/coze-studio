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

import type { ReactNode } from 'react';

import { vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockListTaskThreadMemories = vi.hoisted(() => vi.fn());
const mockUpdateTaskThreadMemory = vi.hoisted(() => vi.fn());
const mockDeleteTaskThreadMemory = vi.hoisted(() => vi.fn());
const mockClearTaskThreadMemories = vi.hoisted(() => vi.fn());
const mockRestoreTaskThreadMemory = vi.hoisted(() => vi.fn());
const mockListTaskThreadMemoryAuditEvents = vi.hoisted(() => vi.fn());
const mockExportTaskThreadMemories = vi.hoisted(() => vi.fn());
const mockImportTaskThreadMemories = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  clearTaskThreadMemories: mockClearTaskThreadMemories,
  deleteTaskThreadMemory: mockDeleteTaskThreadMemory,
  exportTaskThreadMemories: mockExportTaskThreadMemories,
  importTaskThreadMemories: mockImportTaskThreadMemories,
  listTaskThreadMemoryAuditEvents: mockListTaskThreadMemoryAuditEvents,
  listTaskThreadMemories: mockListTaskThreadMemories,
  restoreTaskThreadMemory: mockRestoreTaskThreadMemory,
  updateTaskThreadMemory: mockUpdateTaskThreadMemory,
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    'aria-label': ariaLabel,
    'data-testid': dataTestID,
    children,
    disabled,
    loading,
    onClick,
  }: {
    'aria-label'?: string;
    'data-testid'?: string;
    children?: ReactNode;
    disabled?: boolean;
    loading?: boolean;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      data-testid={dataTestID}
      disabled={disabled}
      data-loading={loading}
      onClick={onClick}
    >
      {children}
    </button>
  ),
  Input: ({
    'aria-label': ariaLabel,
    'data-testid': dataTestID,
    onChange,
    onEnterPress,
    placeholder,
    value,
  }: {
    'aria-label'?: string;
    'data-testid'?: string;
    onChange?: (value: string) => void;
    onEnterPress?: () => void;
    placeholder?: string;
    value?: string;
  }) => (
    <input
      aria-label={ariaLabel}
      data-testid={dataTestID}
      placeholder={placeholder}
      value={value}
      onChange={event => onChange?.(event.target.value)}
      onKeyDown={event => {
        if (event.key === 'Enter') {
          onEnterPress?.();
        }
      }}
    />
  ),
  Popconfirm: ({
    children,
    onConfirm,
    title,
  }: {
    children?: ReactNode;
    onConfirm?: () => void | Promise<void>;
    title?: ReactNode;
  }) => (
    <span>
      {children}
      <button type="button" onClick={() => void onConfirm?.()}>
        确认{title}
      </button>
    </span>
  ),
  SideSheet: ({
    children,
    onCancel,
    title,
    visible,
  }: {
    children?: ReactNode;
    onCancel?: () => void;
    title?: ReactNode;
    visible?: boolean;
  }) =>
    visible ? (
      <section aria-label={String(title)} role="dialog">
        <button type="button" onClick={onCancel}>
          关闭
        </button>
        <div>{title}</div>
        {children}
      </section>
    ) : null,
  Tag: ({
    'data-testid': dataTestID,
    children,
  }: {
    'data-testid'?: string;
    children?: ReactNode;
  }) => <span data-testid={dataTestID}>{children}</span>,
  TextArea: ({
    'aria-label': ariaLabel,
    'data-testid': dataTestID,
    onChange,
    placeholder,
    value,
  }: {
    'aria-label'?: string;
    'data-testid'?: string;
    onChange?: (value: string) => void;
    placeholder?: string;
    value?: string;
  }) => (
    <textarea
      aria-label={ariaLabel}
      data-testid={dataTestID}
      placeholder={placeholder}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  ),
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror icon names. */
vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozEdit: () => <span />,
  IconCozDownload: () => <span />,
  IconCozHistory: () => <span />,
  IconCozImport: () => <span />,
  IconCozMagnifier: () => <span />,
  IconCozRefresh: () => <span />,
  IconCozTrashCan: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import { TaskMemorySection } from '../task-memory-section';

const createMemory = (content = '用户偏好中文摘要') => ({
  memory_id: 'memory-1',
  thread_id: 'thread-memory-1',
  run_id: 'run-1',
  space_id: 'space-1',
  scope: 'thread',
  content,
  metadata: '{"internal_path":"/mnt/user-data/raw.txt"}',
  score: 0.7,
  confidence: 0.82,
  source_type: 'extractor',
  source_id: 'source-1',
  correction_of_memory_id: '',
  corrected_at: 0,
  expires_at: 0,
  created_at: 1717000100000,
  updated_at: 1717000200000,
  deleted_at: 0,
});

const createDeletedMemory = () => ({
  ...createMemory('已删除的任务记忆'),
  deleted_at: 1717000400000,
});

const renderSection = async (
  threadId = 'thread-memory-1',
  readOnly = false,
) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  let root: Root | undefined;

  await act(async () => {
    root = createRoot(container);
    root.render(
      <TaskMemorySection
        spaceId="space-1"
        threadId={threadId}
        readOnly={readOnly}
      />,
    );
    await Promise.resolve();
    await Promise.resolve();
  });

  return {
    container,
    cleanup: () => {
      act(() => {
        root?.unmount();
      });
      container.remove();
    },
  };
};

const collectDataAttributeValues = (container: HTMLElement) =>
  Array.from(container.querySelectorAll('[data-testid]'))
    .flatMap(element =>
      element
        .getAttributeNames()
        .filter(name => name.startsWith('data-'))
        .map(name => element.getAttribute(name) ?? ''),
    )
    .join(' ');

describe('TaskMemorySection', () => {
  beforeEach(() => {
    mockListTaskThreadMemories.mockReset();
    mockUpdateTaskThreadMemory.mockReset();
    mockDeleteTaskThreadMemory.mockReset();
    mockClearTaskThreadMemories.mockReset();
    mockRestoreTaskThreadMemory.mockReset();
    mockListTaskThreadMemoryAuditEvents.mockReset();
    mockExportTaskThreadMemories.mockReset();
    mockImportTaskThreadMemories.mockReset();
    mockListTaskThreadMemories.mockResolvedValue({
      data: {
        memories: [createMemory()],
        total: 1,
      },
      code: 0,
      msg: '',
    });
    mockUpdateTaskThreadMemory.mockResolvedValue({
      data: {
        memory: createMemory('用户偏好英文摘要'),
      },
      code: 0,
      msg: '',
    });
    mockDeleteTaskThreadMemory.mockResolvedValue({
      data: { deleted: true },
      code: 0,
      msg: '',
    });
    mockClearTaskThreadMemories.mockResolvedValue({
      data: { cleared_count: 1 },
      code: 0,
      msg: '',
    });
    mockRestoreTaskThreadMemory.mockResolvedValue({
      data: { memory: createMemory('恢复后的任务记忆'), restored: true },
      code: 0,
      msg: '',
    });
    mockListTaskThreadMemoryAuditEvents.mockResolvedValue({
      data: {
        events: [
          {
            actor_id: 'user-1',
            affected_count: 1,
            created_at: 1717000500000,
            event_id: 'audit-1',
            event_type: 'memory.restored',
            memory_id: 'memory-1',
            run_id: 'run-1',
            scope: 'thread',
            source_id: 'source-1',
            source_type: 'extractor',
            space_id: 'space-1',
            thread_id: 'thread-memory-1',
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });
    mockExportTaskThreadMemories.mockResolvedValue({
      data: {
        exported_at: 1717000600000,
        memories: [createMemory()],
        schema: 'coze.task_thread_memories.export.v1',
        thread_id: 'thread-memory-1',
        total: 1,
      },
      code: 0,
      msg: '',
    });
    mockImportTaskThreadMemories.mockResolvedValue({
      data: {
        imported: 1,
        memories: [createMemory('导入后的任务记忆')],
        skipped: 0,
      },
      code: 0,
      msg: '',
    });
  });

  it('loads and searches task memories with safe list metadata', async () => {
    const { cleanup, container } = await renderSection();

    expect(mockListTaskThreadMemories).toHaveBeenCalledWith({
      page: 1,
      page_size: 20,
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });
    expect(container.textContent).toContain('任务记忆');
    expect(container.textContent).toContain('1 条');
    expect(container.textContent).toContain('用户偏好中文摘要');
    expect(container.textContent).toContain('thread');
    expect(container.textContent).toContain('extractor/source-1');
    expect(container.textContent).not.toContain('/mnt/user-data');

    const input = container.querySelector(
      'input[aria-label="搜索记忆"]',
    ) as HTMLInputElement;
    const threadScopeButton = Array.from(
      container.querySelectorAll('button'),
    ).find(
      button => button.textContent?.trim() === '线程',
    ) as HTMLButtonElement;
    const searchButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '搜索',
    ) as HTMLButtonElement;

    expect(input).toBeTruthy();
    expect(threadScopeButton).toBeTruthy();
    expect(searchButton).toBeTruthy();

    await act(async () => {
      input.value = '偏好';
      Simulate.change(input);
      Simulate.click(threadScopeButton);
      Simulate.click(searchButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockListTaskThreadMemories).toHaveBeenLastCalledWith({
      page: 1,
      page_size: 20,
      q: '偏好',
      scope: 'thread',
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });

    cleanup();
  });

  it('exposes content-free selectors for future browser e2e coverage', async () => {
    const { cleanup, container } = await renderSection();

    const selectorPayload = collectDataAttributeValues(container);
    const panel = container.querySelector('[data-testid="task-memory-panel"]');
    const row = container.querySelector(
      '[data-testid="task-memory-row"]',
    ) as HTMLElement;

    expect(panel).toBeTruthy();
    expect(row).toBeTruthy();
    expect(row.getAttribute('data-memory-id')).toBe('memory-1');
    expect(
      container.querySelector('[data-testid="task-memory-search"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-testid="task-memory-refresh"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-testid="task-memory-export"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-testid="task-memory-import"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-testid="task-memory-toggle-deleted"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-testid="task-memory-clear"]'),
    ).toBeTruthy();

    expect(selectorPayload).toContain('task-memory-panel');
    expect(selectorPayload).toContain('task-memory-row');
    expect(selectorPayload).not.toContain('用户偏好中文摘要');
    expect(selectorPayload).not.toContain('/mnt/user-data');
    expect(selectorPayload).not.toContain('source-1');
    expect(selectorPayload).not.toContain('run-1');

    const importButton = container.querySelector(
      '[data-testid="task-memory-import"]',
    ) as HTMLButtonElement;
    await act(async () => {
      Simulate.click(importButton);
      await Promise.resolve();
    });

    expect(
      container.querySelector('[data-testid="task-memory-import-sheet"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-testid="task-memory-import-input"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-testid="task-memory-import-submit"]'),
    ).toBeTruthy();

    cleanup();
  });

  it('edits a memory with a complete safe snapshot and reloads the list', async () => {
    const { cleanup, container } = await renderSection();

    const editButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '编辑',
    ) as HTMLButtonElement;
    expect(editButton).toBeTruthy();

    await act(async () => {
      Simulate.click(editButton);
      await Promise.resolve();
    });

    const contentInput = container.querySelector(
      'textarea[aria-label="记忆内容"]',
    ) as HTMLTextAreaElement;
    expect(contentInput).toBeTruthy();

    const saveButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '保存',
    ) as HTMLButtonElement;
    expect(saveButton).toBeTruthy();

    await act(async () => {
      Object.getOwnPropertyDescriptor(
        window.HTMLTextAreaElement.prototype,
        'value',
      )?.set?.call(contentInput, '用户偏好英文摘要');
      Simulate.change(contentInput);
      await Promise.resolve();
    });

    await act(async () => {
      Simulate.click(saveButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockUpdateTaskThreadMemory).toHaveBeenCalledWith({
      confidence: 0.82,
      content: '用户偏好英文摘要',
      correction_of_memory_id: '',
      corrected_at: 0,
      expires_at: 0,
      memory_id: 'memory-1',
      metadata: '{"internal_path":"/mnt/user-data/raw.txt"}',
      run_id: 'run-1',
      scope: 'thread',
      score: 0.7,
      source_id: 'source-1',
      source_type: 'extractor',
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });
    expect(mockListTaskThreadMemories).toHaveBeenCalledTimes(2);

    cleanup();
  });

  it('deletes and clears memories through confirmation actions', async () => {
    const { cleanup, container } = await renderSection();

    const deleteConfirm = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '确认删除记忆',
    ) as HTMLButtonElement;
    expect(deleteConfirm).toBeTruthy();

    await act(async () => {
      Simulate.click(deleteConfirm);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockDeleteTaskThreadMemory).toHaveBeenCalledWith({
      memory_id: 'memory-1',
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });

    const clearConfirm = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '确认清空任务记忆',
    ) as HTMLButtonElement;
    expect(clearConfirm).toBeTruthy();

    await act(async () => {
      Simulate.click(clearConfirm);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockClearTaskThreadMemories).toHaveBeenCalledWith({
      scopes: ['thread', 'run', 'long_term'],
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });
    expect(mockListTaskThreadMemories).toHaveBeenCalledTimes(3);

    cleanup();
  });

  it('lists deleted memories, restores one, and opens metadata-only audit history', async () => {
    mockListTaskThreadMemories
      .mockResolvedValueOnce({
        data: {
          memories: [createMemory()],
          total: 1,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          memories: [createDeletedMemory()],
          total: 1,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          memories: [createDeletedMemory()],
          total: 1,
        },
        code: 0,
        msg: '',
      });
    const { cleanup, container } = await renderSection();

    const deletedToggle = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '已删除',
    ) as HTMLButtonElement;
    expect(deletedToggle).toBeTruthy();

    await act(async () => {
      Simulate.click(deletedToggle);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockListTaskThreadMemories).toHaveBeenLastCalledWith({
      include_deleted: true,
      page: 1,
      page_size: 20,
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });
    expect(container.textContent).toContain('已删除的任务记忆');
    expect(container.textContent).toContain('已删除');

    const restoreConfirm = Array.from(
      container.querySelectorAll('button'),
    ).find(
      button => button.textContent?.trim() === '确认恢复记忆',
    ) as HTMLButtonElement;
    expect(restoreConfirm).toBeTruthy();

    await act(async () => {
      Simulate.click(restoreConfirm);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockRestoreTaskThreadMemory).toHaveBeenCalledWith({
      memory_id: 'memory-1',
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });

    const auditButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '审计',
    ) as HTMLButtonElement;
    expect(auditButton).toBeTruthy();

    await act(async () => {
      Simulate.click(auditButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockListTaskThreadMemoryAuditEvents).toHaveBeenCalledWith({
      memory_id: 'memory-1',
      page: 1,
      page_size: 20,
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });
    expect(container.textContent).toContain('记忆审计');
    expect(container.textContent).toContain('memory.restored');
    expect(container.textContent).toContain('extractor/source-1');
    expect(container.textContent).not.toContain('/mnt/user-data');
    expect(
      container.querySelector('[data-testid="task-memory-audit-sheet"]'),
    ).toBeTruthy();
    expect(
      container.querySelector('[data-testid="task-memory-audit-row"]'),
    ).toBeTruthy();

    cleanup();
  });

  it('keeps write actions disabled when task memory is read-only', async () => {
    const { cleanup, container } = await renderSection('thread-memory-1', true);

    expect(container.textContent).toContain('只读');
    expect(
      container.querySelector('[data-testid="task-memory-readonly"]'),
    ).toBeTruthy();
    const importButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '导入记忆',
    ) as HTMLButtonElement;
    const clearButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '清空',
    ) as HTMLButtonElement;
    const editButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '编辑',
    ) as HTMLButtonElement;
    const deleteButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '删除',
    ) as HTMLButtonElement;
    const exportButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '导出记忆',
    ) as HTMLButtonElement;
    const auditButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '审计',
    ) as HTMLButtonElement;

    expect(importButton.disabled).toBe(true);
    expect(clearButton.disabled).toBe(true);
    expect(editButton.disabled).toBe(true);
    expect(deleteButton.disabled).toBe(true);
    expect(exportButton.disabled).toBe(false);
    expect(auditButton.disabled).toBe(false);
    expect(container.textContent).not.toContain('确认清空任务记忆');
    expect(container.textContent).not.toContain('确认删除记忆');

    cleanup();
  });

  it('exports task memories as the safe schema payload', async () => {
    const previousCreateObjectURL = URL.createObjectURL;
    const previousRevokeObjectURL = URL.revokeObjectURL;
    const createObjectURL = vi.fn().mockReturnValue('blob:memory-export');
    const revokeObjectURL = vi.fn();
    URL.createObjectURL = createObjectURL;
    URL.revokeObjectURL = revokeObjectURL;
    const clickedDownloads: string[] = [];
    const previousCreateElement = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation(tagName => {
      const element = previousCreateElement(tagName);
      if (tagName === 'a') {
        Object.defineProperty(element, 'click', {
          configurable: true,
          value: () =>
            clickedDownloads.push((element as HTMLAnchorElement).download),
        });
      }

      return element;
    });

    try {
      const { cleanup, container } = await renderSection();

      const exportButton = Array.from(
        container.querySelectorAll('button'),
      ).find(
        button => button.textContent?.trim() === '导出记忆',
      ) as HTMLButtonElement;
      expect(exportButton).toBeTruthy();

      await act(async () => {
        Simulate.click(exportButton);
        await Promise.resolve();
        await Promise.resolve();
      });

      expect(mockExportTaskThreadMemories).toHaveBeenCalledWith({
        limit: 100,
        space_id: 'space-1',
        thread_id: 'thread-memory-1',
      });
      expect(createObjectURL).toHaveBeenCalledTimes(1);
      const blob = createObjectURL.mock.calls[0][0] as Blob;
      expect(blob.type).toBe('application/json');
      expect(clickedDownloads).toEqual([
        'task-thread-thread-memory-1-memories.json',
      ]);
      expect(revokeObjectURL).toHaveBeenCalledWith('blob:memory-export');
      expect(container.textContent).toContain('已导出 1 条任务记忆');

      cleanup();
    } finally {
      vi.restoreAllMocks();
      URL.createObjectURL = previousCreateObjectURL;
      URL.revokeObjectURL = previousRevokeObjectURL;
    }
  });

  it('imports task memories from a safe export payload and reloads the list', async () => {
    const { cleanup, container } = await renderSection();

    const importButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '导入记忆',
    ) as HTMLButtonElement;
    expect(importButton).toBeTruthy();

    await act(async () => {
      Simulate.click(importButton);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('导入任务记忆');
    const payloadInput = container.querySelector(
      'textarea[aria-label="导入任务记忆 JSON"]',
    ) as HTMLTextAreaElement;
    expect(payloadInput).toBeTruthy();

    const importPayload = JSON.stringify({
      schema: 'coze.task_thread_memories.export.v1',
      memories: [
        {
          content: '导入后的任务记忆',
          metadata: '{"safe":true}',
          run_id: 'run-import-1',
          scope: 'run',
          score: 0.64,
          confidence: 0.91,
          source_type: 'manual_import',
          source_id: 'import-1',
          correction_of_memory_id: '',
          corrected_at: 0,
          expires_at: 0,
          hidden_config: 'must not be forwarded',
        },
      ],
    });

    await act(async () => {
      Object.getOwnPropertyDescriptor(
        window.HTMLTextAreaElement.prototype,
        'value',
      )?.set?.call(payloadInput, importPayload);
      Simulate.change(payloadInput);
      await Promise.resolve();
    });

    const submitButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.trim() === '开始导入',
    ) as HTMLButtonElement;
    expect(submitButton).toBeTruthy();

    await act(async () => {
      Simulate.click(submitButton);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockImportTaskThreadMemories).toHaveBeenCalledWith({
      memories: [
        {
          confidence: 0.91,
          content: '导入后的任务记忆',
          corrected_at: 0,
          expires_at: 0,
          metadata: '{"safe":true}',
          run_id: 'run-import-1',
          scope: 'run',
          score: 0.64,
          source_id: 'import-1',
          source_type: 'manual_import',
        },
      ],
      space_id: 'space-1',
      thread_id: 'thread-memory-1',
    });
    expect(mockListTaskThreadMemories).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('已导入 1 条，跳过 0 条');
    expect(container.textContent).not.toContain('hidden_config');

    cleanup();
  });
});
