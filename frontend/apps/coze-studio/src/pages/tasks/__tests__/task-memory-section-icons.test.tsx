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
import { act } from 'react-dom/test-utils';
import { createRoot } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockListTaskThreadMemories = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  clearTaskThreadMemories: vi.fn(),
  deleteTaskThreadMemory: vi.fn(),
  exportTaskThreadMemories: vi.fn(),
  importTaskThreadMemories: vi.fn(),
  listTaskThreadMemoryAuditEvents: vi.fn(),
  listTaskThreadMemories: mockListTaskThreadMemories,
  restoreTaskThreadMemory: vi.fn(),
  updateTaskThreadMemory: vi.fn(),
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => ({
  Button: ({ children, icon }: { children?: ReactNode; icon?: ReactNode }) => (
    <button type="button">
      {icon}
      {children}
    </button>
  ),
  Input: () => <input />,
  Popconfirm: ({ children }: { children?: ReactNode }) => <>{children}</>,
  SideSheet: () => null,
  Tag: ({ children }: { children?: ReactNode }) => <span>{children}</span>,
  TextArea: () => <textarea />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import { TaskMemorySection } from '../task-memory-section';

describe('TaskMemorySection icon integration', () => {
  beforeEach(() => {
    mockListTaskThreadMemories.mockReset();
    mockListTaskThreadMemories.mockResolvedValue({
      data: {
        memories: [],
        total: 0,
      },
      code: 0,
      msg: '',
    });
  });

  it('renders the memory toolbar with real Coze Design icon exports', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    await act(async () => {
      root.render(<TaskMemorySection threadId="thread-memory-icons" />);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('任务记忆');
    expect(container.textContent).toContain('搜索');
    expect(container.textContent).toContain('导出记忆');

    act(() => {
      root.unmount();
    });
    container.remove();
  });
});
