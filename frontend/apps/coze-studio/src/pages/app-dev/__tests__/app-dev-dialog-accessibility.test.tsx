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

import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { AccessibleDialog } from '../components/accessible-dialog';
import {
  useConfirmDialog,
  useTextInputDialog,
} from '../components/text-input-dialog';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const flush = async () => {
  await Promise.resolve();
  await Promise.resolve();
};

describe('AppDev accessible dialogs', () => {
  let root: Root | undefined;
  let container: HTMLDivElement;

  afterEach(async () => {
    if (root) {
      await act(async () => {
        root?.unmount();
        await flush();
      });
    }
    root = undefined;
    container?.remove();
    document.body.style.overflow = '';
    vi.restoreAllMocks();
  });

  it('traps focus, closes on Escape, and restores the trigger', async () => {
    container = document.createElement('div');
    document.body.append(container);
    const Harness = () => {
      const [open, setOpen] = useState(false);
      return (
        <>
          <button type="button" onClick={() => setOpen(true)}>
            打开项目设置
          </button>
          {open ? (
            <AccessibleDialog
              title="项目设置"
              onClose={() => setOpen(false)}
              className="app-dev-modal app-dev-modal--compact"
            >
              <input data-dialog-autofocus aria-label="项目名称" />
              <button type="button">保存</button>
            </AccessibleDialog>
          ) : null}
        </>
      );
    };
    root = createRoot(container);
    await act(async () => {
      root?.render(<Harness />);
      await flush();
    });
    const trigger = container.querySelector<HTMLButtonElement>('button')!;
    trigger.focus();
    await act(async () => {
      trigger.click();
      await flush();
    });

    const dialog = container.querySelector<HTMLElement>('[role="dialog"]')!;
    const input = dialog.querySelector<HTMLInputElement>('input')!;
    const save = Array.from(dialog.querySelectorAll('button')).find(
      button => button.textContent === '保存',
    )!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(document.activeElement).toBe(input);
    expect(document.body.style.overflow).toBe('hidden');

    save.focus();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }));
    expect(document.activeElement).toBe(input);
    input.focus();
    window.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true }),
    );
    expect(document.activeElement).toBe(save);

    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
      await flush();
    });
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(trigger);
    expect(document.body.style.overflow).toBe('');
  });

  it('settles replaced and unmounted text input promises exactly once', async () => {
    container = document.createElement('div');
    document.body.append(container);
    let open!: ReturnType<typeof useTextInputDialog>['openTextInputDialog'];
    const Harness = () => {
      const state = useTextInputDialog();
      open = state.openTextInputDialog;
      return state.textInputDialog;
    };
    root = createRoot(container);
    await act(async () => {
      root?.render(<Harness />);
      await flush();
    });

    let first!: Promise<string | null>;
    await act(async () => {
      first = open({ title: '重命名', label: '名称' });
      await flush();
    });
    const firstSettled = vi.fn();
    void first.then(firstSettled);
    let second!: Promise<string | null>;
    await act(async () => {
      second = open({ title: '新建文件', label: '名称' });
      await flush();
    });
    const secondSettled = vi.fn();
    void second.then(secondSettled);
    await act(flush);
    await expect(first).resolves.toBeNull();
    expect(firstSettled).toHaveBeenCalledTimes(1);
    const textDialog = container.querySelector<HTMLElement>('[role="dialog"]')!;
    expect(textDialog.getAttribute('aria-modal')).toBe('true');
    expect(document.activeElement).toBe(textDialog.querySelector('input'));

    await act(async () => {
      root?.unmount();
      root = undefined;
      await flush();
    });
    await expect(second).resolves.toBeNull();
    expect(secondSettled).toHaveBeenCalledTimes(1);
  });

  it('settles replaced and unmounted confirm promises exactly once', async () => {
    container = document.createElement('div');
    document.body.append(container);
    let open!: ReturnType<typeof useConfirmDialog>['openConfirmDialog'];
    const Harness = () => {
      const state = useConfirmDialog();
      open = state.openConfirmDialog;
      return state.confirmDialog;
    };
    root = createRoot(container);
    await act(async () => {
      root?.render(<Harness />);
      await flush();
    });

    let first!: Promise<boolean>;
    let second!: Promise<boolean>;
    await act(async () => {
      first = open({ title: '删除文件', description: '确认删除？' });
      await flush();
    });
    await act(async () => {
      second = open({ title: '恢复版本', description: '确认恢复？' });
      await flush();
    });
    await expect(first).resolves.toBe(false);
    const confirmDialog =
      container.querySelector<HTMLElement>('[role="dialog"]')!;
    expect(confirmDialog.getAttribute('aria-modal')).toBe('true');
    expect(confirmDialog.contains(document.activeElement)).toBe(true);

    await act(async () => {
      root?.unmount();
      root = undefined;
      await flush();
    });
    await expect(second).resolves.toBe(false);
  });
});
