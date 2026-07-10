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

import { useCallback, useRef, useState } from 'react';

export interface TextInputDialogOptions {
  title: string;
  description?: string;
  label: string;
  placeholder?: string;
  initialValue?: string;
  confirmText?: string;
  allowEmpty?: boolean;
  maxLength?: number;
}

export interface ConfirmDialogOptions {
  title: string;
  description: string;
  confirmText?: string;
  cancelText?: string;
}

interface TextInputDialogState extends TextInputDialogOptions {
  value: string;
  error: string;
}

export const useTextInputDialog = () => {
  const resolverRef = useRef<((value: string | null) => void) | null>(null);
  const [dialog, setDialog] = useState<TextInputDialogState | null>(null);

  const closeDialog = useCallback((value: string | null) => {
    resolverRef.current?.(value);
    resolverRef.current = null;
    setDialog(null);
  }, []);

  const openTextInputDialog = useCallback(
    (options: TextInputDialogOptions) =>
      new Promise<string | null>(resolve => {
        resolverRef.current?.(null);
        resolverRef.current = resolve;
        setDialog({
          ...options,
          value: options.initialValue || '',
          error: '',
        });
      }),
    [],
  );

  const submitDialog = useCallback(() => {
    if (!dialog) {
      return;
    }

    const value = dialog.allowEmpty ? dialog.value : dialog.value.trim();
    if (!dialog.allowEmpty && !value) {
      setDialog(current =>
        current ? { ...current, error: `${current.label}不能为空` } : current,
      );
      return;
    }

    closeDialog(value);
  }, [closeDialog, dialog]);

  const textInputDialog = dialog ? (
    <div className="app-dev-modal-mask app-dev-modal-mask--nested">
      <section className="app-dev-modal app-dev-modal--compact">
        <header className="app-dev-modal__header">
          <div>
            <h2>{dialog.title}</h2>
            {dialog.description ? <p>{dialog.description}</p> : null}
          </div>
          <button type="button" onClick={() => closeDialog(null)}>
            关闭
          </button>
        </header>
        <label className="app-dev-form-field">
          <span>{dialog.label}</span>
          <input
            value={dialog.value}
            maxLength={dialog.maxLength}
            placeholder={dialog.placeholder}
            onChange={event =>
              setDialog(current =>
                current
                  ? {
                      ...current,
                      value: event.target.value,
                      error: '',
                    }
                  : current,
              )
            }
          />
        </label>
        {dialog.error ? (
          <div className="app-dev-form-error">{dialog.error}</div>
        ) : null}
        <footer className="app-dev-modal__footer">
          <button type="button" onClick={() => closeDialog(null)}>
            取消
          </button>
          <button type="button" onClick={submitDialog}>
            {dialog.confirmText || '确定'}
          </button>
        </footer>
      </section>
    </div>
  ) : null;

  return {
    openTextInputDialog,
    textInputDialog,
  };
};

export const useConfirmDialog = () => {
  const resolverRef = useRef<((confirmed: boolean) => void) | null>(null);
  const [dialog, setDialog] = useState<ConfirmDialogOptions | null>(null);

  const closeDialog = useCallback((confirmed: boolean) => {
    resolverRef.current?.(confirmed);
    resolverRef.current = null;
    setDialog(null);
  }, []);

  const openConfirmDialog = useCallback(
    (options: ConfirmDialogOptions) =>
      new Promise<boolean>(resolve => {
        resolverRef.current?.(false);
        resolverRef.current = resolve;
        setDialog(options);
      }),
    [],
  );

  const confirmDialog = dialog ? (
    <div className="app-dev-modal-mask app-dev-modal-mask--nested">
      <section className="app-dev-modal app-dev-modal--compact">
        <header className="app-dev-modal__header">
          <div>
            <h2>{dialog.title}</h2>
            <p>{dialog.description}</p>
          </div>
          <button type="button" onClick={() => closeDialog(false)}>
            关闭
          </button>
        </header>
        <footer className="app-dev-modal__footer">
          <button type="button" onClick={() => closeDialog(false)}>
            {dialog.cancelText || '取消'}
          </button>
          <button type="button" onClick={() => closeDialog(true)}>
            {dialog.confirmText || '确认'}
          </button>
        </footer>
      </section>
    </div>
  ) : null;

  return {
    openConfirmDialog,
    confirmDialog,
  };
};
