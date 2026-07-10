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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { getAppDevChatStatus, listAppDevChatHistory } from '../service';
import { useAppDevChat } from '../hooks/use-app-dev-chat';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('../service', () => ({
  buildAppDevEventsUrl: vi.fn(() => '/events'),
  cancelAppDevChat: vi.fn(),
  getAppDevChatStatus: vi.fn(),
  listAppDevChatHistory: vi.fn(),
  normalizeAppDevError: vi.fn(error => String(error)),
  sendAppDevChatMessage: vi.fn(),
  uploadAppDevFiles: vi.fn(),
}));

class MockEventSource {
  onerror?: () => void;

  addEventListener() {}

  close() {}
}

describe('useAppDevChat', () => {
  let root: Root | undefined;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('EventSource', MockEventSource);
    vi.mocked(listAppDevChatHistory)
      .mockResolvedValueOnce({ items: [] })
      .mockResolvedValueOnce({
        items: [
          {
            id: 'assistant-recovered',
            type: 'assistant',
            role: 'assistant',
            content: '任务已完成',
          },
        ],
      });
    vi.mocked(getAppDevChatStatus)
      .mockResolvedValueOnce({ running: true })
      .mockResolvedValueOnce({ running: false });
  });

  afterEach(() => {
    if (root) {
      act(() => root?.unmount());
      root = undefined;
    }
    vi.unstubAllGlobals();
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('reconciles completed backend state when the SSE end event is missed', async () => {
    const onTaskSettled = vi.fn();
    const onFilesChanged = vi.fn();
    let chat: ReturnType<typeof useAppDevChat> | undefined;
    const Harness = () => {
      chat = useAppDevChat(
        'space-1',
        'project-1',
        'model-1',
        [],
        () => onTaskSettled(),
        () => onFilesChanged(),
      );
      return null;
    };

    root = createRoot(document.createElement('div'));

    await act(async () => {
      root?.render(<Harness />);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(chat?.running).toBe(true);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3000);
    });

    expect(chat?.running).toBe(false);
    expect(chat?.messages).toEqual([
      expect.objectContaining({
        id: 'assistant-recovered',
        content: '任务已完成',
      }),
    ]);
    expect(onTaskSettled).toHaveBeenCalledTimes(1);
    expect(onFilesChanged).toHaveBeenCalledTimes(1);
    expect(getAppDevChatStatus).toHaveBeenCalledTimes(2);
  });
});
