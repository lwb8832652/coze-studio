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

import { afterEach, describe, expect, it, vi } from 'vitest';

import { copyTextToClipboard } from '../task-clipboard';

const restoreClipboard = () => {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: undefined,
  });
};

const mockExecCommand = (result = true) => {
  const execCommand = vi.fn().mockReturnValue(result);
  Object.defineProperty(document, 'execCommand', {
    configurable: true,
    value: execCommand,
  });
  return execCommand;
};

afterEach(() => {
  vi.restoreAllMocks();
  restoreClipboard();
  document.body.innerHTML = '';
});

describe('copyTextToClipboard', () => {
  it('falls back to a temporary textarea when Clipboard API is unavailable', async () => {
    restoreClipboard();
    const execCommand = mockExecCommand();

    await expect(copyTextToClipboard('artifact preview')).resolves.toBe(true);

    expect(execCommand).toHaveBeenCalledWith('copy');
    expect(document.querySelector('textarea')).toBeNull();
  });

  it('falls back when Clipboard API rejects', async () => {
    const writeText = vi.fn().mockRejectedValue(new Error('denied'));
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });
    const execCommand = mockExecCommand();

    await expect(copyTextToClipboard('retry preview copy')).resolves.toBe(true);

    expect(writeText).toHaveBeenCalledWith('retry preview copy');
    expect(execCommand).toHaveBeenCalledWith('copy');
  });

  it('returns false instead of throwing when no copy mechanism is available', async () => {
    restoreClipboard();
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: undefined,
    });

    await expect(copyTextToClipboard('blocked copy')).resolves.toBe(false);
  });
});
