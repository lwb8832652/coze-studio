/* Copyright 2025 coze-dev Authors */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { SandboxAuditDrawer } from '../sandbox-audit-drawer';
import { SandboxProviderDetail } from '../sandbox-provider-detail';
import { providerFixture } from './sandbox-test-fixtures';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const audit = vi.hoisted(() => vi.fn());
vi.mock('../sandbox-service', async importOriginal => ({
  ...(await importOriginal()),
  listSandboxAuditEvents: audit,
}));

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(onResolve => {
    resolve = onResolve;
  });
  return { promise, resolve };
};

describe('Sandbox detail and audit drawers', () => {
  let container: HTMLDivElement;
  let root: Root;
  let opener: HTMLButtonElement;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    opener = document.createElement('button');
    opener.textContent = 'open';
    document.body.appendChild(opener);
    root = createRoot(container);
    audit.mockResolvedValue({ items: [], total: 0 });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    opener.remove();
    vi.clearAllMocks();
  });

  it('focuses, closes detail with Escape and restores opener focus', async () => {
    opener.focus();
    const close = vi.fn();
    await act(async () => {
      root.render(
        <SandboxProviderDetail provider={providerFixture} onClose={close} />,
      );
      await Promise.resolve();
    });
    expect(document.activeElement?.getAttribute('aria-label')).toBe(
      '关闭 Provider 详情',
    );
    act(() =>
      Simulate.keyDown(
        container.querySelector<HTMLElement>('[role="dialog"]')!,
        { key: 'Tab' },
      ),
    );
    expect(document.activeElement?.getAttribute('aria-label')).toBe(
      '关闭 Provider 详情',
    );
    act(() =>
      Simulate.keyDown(
        container.querySelector<HTMLElement>('[role="dialog"]')!,
        { key: 'Escape' },
      ),
    );
    expect(close).toHaveBeenCalledTimes(1);
    act(() =>
      root.render(<SandboxProviderDetail provider={null} onClose={close} />),
    );
    expect(document.activeElement).toBe(opener);
  });

  it('supports audit loading/error/retry/pagination, Escape and focus restoration', async () => {
    audit
      .mockRejectedValueOnce(new Error('audit offline'))
      .mockResolvedValueOnce({
        items: [
          {
            id: 1,
            provider_id: 17,
            actor_user_id: 42,
            action: 'update',
            result: 'success',
            request_id: 'correlation-0123456789abcdef',
            metadata: { changed_fields: 'policy' },
            created_at: '2026-07-16T08:00:00Z',
          },
        ],
        total: 21,
      })
      .mockResolvedValueOnce({ items: [], total: 21 });
    opener.focus();
    const close = vi.fn();
    await act(async () => {
      root.render(
        <SandboxAuditDrawer provider={providerFixture} onClose={close} />,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(document.activeElement?.getAttribute('aria-label')).toBe(
      '关闭 Sandbox 审计',
    );
    expect(container.textContent).toContain('audit offline');

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="重试加载 Sandbox 审计"]',
        )!,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain('changed_fields');
    expect(container.textContent).toContain('correlat...cdef');
    expect(container.textContent).not.toContain('correlation-0123456789abcdef');
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="Sandbox 审计下一页"]',
        )!,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(audit).toHaveBeenLastCalledWith(
      17,
      expect.objectContaining({ offset: 20, limit: 20 }),
      expect.anything(),
    );

    act(() =>
      Simulate.keyDown(
        container.querySelector<HTMLElement>('[role="dialog"]')!,
        { key: 'Escape' },
      ),
    );
    expect(close).toHaveBeenCalledTimes(1);
    act(() =>
      root.render(<SandboxAuditDrawer provider={null} onClose={close} />),
    );
    expect(document.activeElement).toBe(opener);
  });

  it('traps audit focus and prevents Provider A from overwriting Provider B', async () => {
    const providerB = { ...providerFixture, id: 18, name: 'Secondary' };
    const responseA = deferred<{ items: never[]; total: number }>();
    const responseB = deferred<{
      items: Array<{
        id: number;
        provider_id: number;
        actor_user_id: number;
        action: string;
        result: string;
        request_id: string;
        metadata: Record<string, string>;
        created_at: string;
      }>;
      total: number;
    }>();
    const signals: AbortSignal[] = [];
    audit.mockImplementation((providerID, _filters, signal) => {
      signals.push(signal);
      return providerID === 17 ? responseA.promise : responseB.promise;
    });

    await act(async () => {
      root.render(
        <SandboxAuditDrawer provider={providerFixture} onClose={vi.fn()} />,
      );
      await Promise.resolve();
    });
    await act(async () => {
      root.render(
        <SandboxAuditDrawer provider={providerB} onClose={vi.fn()} />,
      );
      await Promise.resolve();
    });
    await act(async () => {
      responseB.resolve({
        items: [
          {
            id: 2,
            provider_id: 18,
            actor_user_id: 42,
            action: 'provider-b-action',
            result: 'success',
            request_id: 'request-b',
            metadata: {},
            created_at: '2026-07-16T08:00:00Z',
          },
        ],
        total: 1,
      });
      await Promise.resolve();
      await Promise.resolve();
    });
    await act(async () => {
      responseA.resolve({ items: [], total: 0 });
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(signals[0].aborted).toBe(true);
    expect(container.textContent).toContain('Secondary · 审计记录');
    expect(container.textContent).toContain('provider-b-action');
    const dialog = container.querySelector<HTMLElement>('[role="dialog"]')!;
    const close = container.querySelector<HTMLButtonElement>(
      'button[aria-label="关闭 Sandbox 审计"]',
    )!;
    const refresh = container.querySelector<HTMLButtonElement>(
      'button[aria-label="刷新 Sandbox 审计"]',
    )!;
    refresh.focus();
    act(() => Simulate.keyDown(dialog, { key: 'Tab' }));
    expect(document.activeElement).toBe(close);
    close.focus();
    act(() => Simulate.keyDown(dialog, { key: 'Tab', shiftKey: true }));
    expect(document.activeElement).toBe(refresh);
  });
});
