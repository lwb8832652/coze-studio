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
import { useEffect, useRef } from 'react';

interface AccessibleDialogProps {
  title: string;
  children: ReactNode;
  onClose: () => void;
  className?: string;
  maskClassName?: string;
  closeOnBackdrop?: boolean;
}

const focusableSelector = [
  'button:not([disabled])',
  'input:not([disabled])',
  'textarea:not([disabled])',
  'select:not([disabled])',
  'a[href]',
  '[tabindex]:not([tabindex="-1"])',
].join(',');

const dialogStack: symbol[] = [];
let bodyLockCount = 0;
let previousBodyOverflow = '';

const lockBackground = () => {
  if (bodyLockCount === 0) {
    previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
  }
  bodyLockCount += 1;
};

const unlockBackground = () => {
  bodyLockCount = Math.max(0, bodyLockCount - 1);
  if (bodyLockCount === 0) {
    document.body.style.overflow = previousBodyOverflow;
  }
};

export const AccessibleDialog = ({
  title,
  children,
  onClose,
  className = 'app-dev-modal app-dev-modal--compact',
  maskClassName = 'app-dev-modal-mask',
  closeOnBackdrop = false,
}: AccessibleDialogProps) => {
  const dialogRef = useRef<HTMLElement>(null);
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  useEffect(() => {
    const token = Symbol(title);
    const previouslyFocused =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : undefined;
    dialogStack.push(token);
    lockBackground();

    queueMicrotask(() => {
      const dialog = dialogRef.current;
      if (!dialog || dialogStack.at(-1) !== token) {
        return;
      }
      const initial =
        dialog.querySelector<HTMLElement>('[data-dialog-autofocus]') ??
        dialog.querySelector<HTMLElement>(focusableSelector) ??
        dialog;
      initial.focus();
    });

    const onKeyDown = (event: KeyboardEvent) => {
      if (dialogStack.at(-1) !== token) {
        return;
      }
      if (event.key === 'Escape') {
        event.preventDefault();
        onCloseRef.current();
        return;
      }
      if (event.key !== 'Tab') {
        return;
      }
      const dialog = dialogRef.current;
      if (!dialog) {
        return;
      }
      const focusable = Array.from(
        dialog.querySelectorAll<HTMLElement>(focusableSelector),
      ).filter(element => !element.hidden);
      if (!focusable.length) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable.at(-1);
      if (!last) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (
        !event.shiftKey &&
        (document.activeElement === last ||
          !dialog.contains(document.activeElement))
      ) {
        event.preventDefault();
        first.focus();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => {
      window.removeEventListener('keydown', onKeyDown);
      const index = dialogStack.lastIndexOf(token);
      if (index >= 0) {
        dialogStack.splice(index, 1);
      }
      unlockBackground();
      if (previouslyFocused?.isConnected) {
        previouslyFocused.focus();
      }
    };
  }, [title]);

  return (
    <div
      className={maskClassName}
      role="presentation"
      onClick={event => {
        if (closeOnBackdrop && event.target === event.currentTarget) {
          onCloseRef.current();
        }
      }}
    >
      <section
        ref={dialogRef}
        className={className}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
      >
        {children}
      </section>
    </div>
  );
};
