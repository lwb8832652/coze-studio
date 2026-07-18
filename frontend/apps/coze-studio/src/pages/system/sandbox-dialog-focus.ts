/* Copyright 2025 coze-dev Authors */

import {
  useCallback,
  useEffect,
  useRef,
  type KeyboardEvent as ReactKeyboardEvent,
  type RefObject,
} from 'react';

const FOCUSABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
  '[contenteditable="true"]',
].join(',');

const getFocusableElements = (container: HTMLElement) =>
  Array.from(
    container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR),
  ).filter(
    element =>
      element.getAttribute('aria-hidden') !== 'true' &&
      !element.hasAttribute('hidden'),
  );

interface SandboxDialogFocusOptions {
  canClose?: boolean;
  initialFocusRef: RefObject<HTMLElement>;
  onClose: () => void;
  open: boolean;
}

export const useSandboxDialogFocus = ({
  canClose = true,
  initialFocusRef,
  onClose,
  open,
}: SandboxDialogFocusOptions) => {
  const dialogRef = useRef<HTMLElement>(null);
  const closeRef = useRef(onClose);
  const canCloseRef = useRef(canClose);
  closeRef.current = onClose;
  canCloseRef.current = canClose;

  useEffect(() => {
    if (!open) {
      return;
    }
    const previousFocus =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    initialFocusRef.current?.focus();
    return () => {
      if (previousFocus?.isConnected) {
        previousFocus.focus();
      }
    };
  }, [initialFocusRef, open]);

  const onDialogKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLElement>) => {
      if (event.key === 'Escape') {
        if (!canCloseRef.current) {
          return;
        }
        event.preventDefault();
        closeRef.current();
        return;
      }
      if (event.key !== 'Tab') {
        return;
      }

      const dialog = dialogRef.current;
      if (!dialog) {
        return;
      }
      const focusable = getFocusableElements(dialog);
      if (focusable.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      const active = document.activeElement;
      if (event.shiftKey && (active === first || !dialog.contains(active))) {
        event.preventDefault();
        last.focus();
      } else if (
        !event.shiftKey &&
        (active === last || !dialog.contains(active))
      ) {
        event.preventDefault();
        first.focus();
      }
    },
    [],
  );

  return { dialogRef, onDialogKeyDown };
};
