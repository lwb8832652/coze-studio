/* Copyright 2025 coze-dev Authors */

/* eslint-disable @typescript-eslint/require-await, @typescript-eslint/no-invalid-void-type -- Deferred test doubles preserve React and async contracts. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { SandboxAPIError } from '../sandbox-service';
import { SandboxProviderForm } from '../sandbox-provider-form';
import { fullPolicy, providerFixture } from './sandbox-test-fixtures';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, reject, resolve };
};

const capabilities = (localDebug: boolean) => ({
  control_plane: {
    available: true,
    reason_code: 'AVAILABLE',
    message: 'control ready',
  },
  local_debug: {
    available: localDebug,
    reason_code: localDebug ? 'AVAILABLE' : 'LOCAL_DEBUG_UNAVAILABLE',
    message: localDebug
      ? 'local debug ready'
      : 'local debug disabled by server',
  },
});

describe('SandboxProviderForm', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  const setInput = (label: string, value: string) => {
    const input = container.querySelector<
      HTMLInputElement | HTMLTextAreaElement
    >(`[aria-label="${label}"]`)!;
    input.value = value;
    Simulate.change(input);
  };

  it('enables local-debug only from the service capability and shows its safe reason', () => {
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(false)}
          mode="create"
          open
          onCancel={vi.fn()}
          onSubmit={vi.fn()}
        />,
      ),
    );
    expect(
      container.querySelector<HTMLOptionElement>('option[value="local_debug"]')
        ?.disabled,
    ).toBe(true);
    expect(container.textContent).toContain('local debug disabled by server');

    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open
          onCancel={vi.fn()}
          onSubmit={vi.fn()}
        />,
      ),
    );
    expect(
      container.querySelector<HTMLOptionElement>('option[value="local_debug"]')
        ?.disabled,
    ).toBe(false);
  });

  it('submits the complete policy payload without exposing edit secrets', async () => {
    const submit = vi.fn().mockResolvedValue(undefined);
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="edit"
          open
          provider={providerFixture}
          onCancel={vi.fn()}
          onSubmit={submit}
        />,
      ),
    );

    expect(container.innerHTML).not.toContain('must-never-survive');
    expect(container.textContent).toContain('sha256:bounded');
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存 Provider"]',
        )!,
      );
      await Promise.resolve();
    });

    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        credential: { mode: 'keep', value: '' },
        policy: fullPolicy,
      }),
    );
  });

  it('builds controlled policy lists and rejects unsafe client input before submission', async () => {
    const submit = vi.fn().mockResolvedValue(undefined);
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open
          onCancel={vi.fn()}
          onSubmit={submit}
        />,
      ),
    );

    await act(async () => {
      setInput('Provider 名称', 'Local');
      Simulate.change(
        container.querySelector<HTMLSelectElement>(
          'select[aria-label="Provider 类型"]',
        )!,
        {
          target: { value: 'local_debug' },
        },
      );
      setInput('允许的环境变量名', 'PATH\nTOKEN=value');
      setInput('虚拟只读前缀', 'inputs\n/etc');
      setInput('允许的可执行文件', 'node --eval');
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="创建 Provider"]',
        )!,
      );
      await Promise.resolve();
    });

    expect(submit).not.toHaveBeenCalled();
    expect(container.textContent).toContain('环境变量名只能包含大写字母');
    expect(container.textContent).toContain(
      '虚拟路径必须从 workspace、inputs、outputs 或 artifacts 开始',
    );
    expect(container.textContent).toContain('可执行文件只允许填写名称');
  });

  it('maps safe server field errors and blocks duplicate submission', async () => {
    let resolveSubmit: (() => void) | undefined;
    const submit = vi.fn(
      () =>
        new Promise<void>(resolve => {
          resolveSubmit = resolve;
        }),
    );
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open
          onCancel={vi.fn()}
          onSubmit={submit}
        />,
      ),
    );
    await act(async () => {
      setInput('Provider 名称', 'Primary');
      setInput('Provider Endpoint', 'https://runner.example.test');
      setInput('Provider 凭据', 'create-secret');
    });
    const save = container.querySelector<HTMLButtonElement>(
      'button[aria-label="创建 Provider"]',
    )!;
    await act(async () => {
      Simulate.click(save);
      Simulate.click(save);
      await Promise.resolve();
    });
    expect(submit).toHaveBeenCalledTimes(1);
    expect(save.disabled).toBe(true);
    await act(async () => resolveSubmit?.());

    const rejected = vi.fn().mockRejectedValue(
      new SandboxAPIError(400, 'SANDBOX_CONFIGURATION_INVALID', {
        request: '请求参数无效',
        credential: '凭据配置无效',
        scopes: '所选作用域不受支持',
        type: '当前环境不支持本地调试 Sandbox',
        name: 'must-drop-name',
        'policy.allowed_env_names': 'must-drop-policy',
      }),
    );
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open
          onCancel={vi.fn()}
          onSubmit={rejected}
        />,
      ),
    );
    await act(async () => {
      setInput('Provider 名称', 'Primary');
      setInput('Provider Endpoint', 'https://runner.example.test');
      setInput('Provider 凭据', 'create-secret');
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="创建 Provider"]',
        )!,
      );
      await Promise.resolve();
    });
    expect(container.textContent).toContain('请求参数无效');
    expect(container.textContent).toContain('凭据配置无效');
    expect(container.textContent).toContain('所选作用域不受支持');
    expect(container.textContent).toContain('当前环境不支持本地调试 Sandbox');
    expect(container.textContent).not.toContain('must-drop-name');
    expect(container.textContent).not.toContain('must-drop-policy');
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="创建 Provider"]',
      )?.disabled,
    ).toBe(false);
  });

  it('locks every payload control while submitting and clears secrets after success', async () => {
    const pending = deferred<void>();
    const submit = vi.fn(() => pending.promise);
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open
          onCancel={vi.fn()}
          onSubmit={submit}
        />,
      ),
    );
    await act(async () => {
      setInput('Provider 名称', 'Primary');
      setInput('Provider Endpoint', 'https://runner.example.test');
      setInput('Provider 凭据', 'create-secret');
      Simulate.change(
        container.querySelector<HTMLInputElement>(
          'input[aria-label="允许 Provider 网络访问"]',
        )!,
        {
          target: { checked: true },
        },
      );
      Simulate.change(
        container.querySelector<HTMLInputElement>(
          'input[aria-label="允许 FFI"]',
        )!,
        {
          target: { checked: true },
        },
      );
      Simulate.change(
        container.querySelector<HTMLSelectElement>(
          'select[aria-label="Node modules 模式"]',
        )!,
        {
          target: { value: 'approved_directory' },
        },
      );
      await Promise.resolve();
    });
    await act(async () => {
      setInput('网络访问白名单', 'api.example.test');
      setInput('允许的环境变量名', 'PATH');
      setInput('Node modules 目录引用', 'approved-node-modules');
      await Promise.resolve();
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="创建 Provider"]',
        )!,
      );
      await Promise.resolve();
    });

    const labels = [
      'Provider 名称',
      'Provider 类型',
      'Provider Endpoint',
      'Provider 凭据',
      '运行超时秒数',
      '允许的环境变量名',
      '允许 Provider 网络访问',
      '网络访问白名单',
      '允许 FFI',
      'Node modules 模式',
      'Node modules 目录引用',
    ];
    for (const label of labels) {
      expect(
        container.querySelector<
          HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement
        >(`[aria-label="${label}"]`)?.disabled,
      ).toBe(true);
    }
    expect(
      Array.from(
        container.querySelectorAll<HTMLInputElement>('input[type="checkbox"]'),
      ).every(control => control.disabled),
    ).toBe(true);

    await act(async () => {
      pending.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="Provider 名称"]',
      )?.disabled,
    ).toBe(false);
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="Provider Endpoint"]',
      )?.value,
    ).toBe('');
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="Provider 凭据"]',
      )?.value,
    ).toBe('');
  });

  it.each(['resolve', 'reject'] as const)(
    'ignores form promise settlement after unmount when it %s',
    async outcome => {
      const pending = deferred<void>();
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => undefined);
      act(() =>
        root.render(
          <SandboxProviderForm
            capabilities={capabilities(true)}
            mode="create"
            open
            onCancel={vi.fn()}
            onSubmit={() => pending.promise}
          />,
        ),
      );
      await act(async () => {
        setInput('Provider 名称', 'Primary');
        setInput('Provider Endpoint', 'https://runner.example.test');
        setInput('Provider 凭据', 'create-secret');
        await Promise.resolve();
      });
      await act(async () => {
        Simulate.click(
          container.querySelector<HTMLButtonElement>(
            'button[aria-label="创建 Provider"]',
          )!,
        );
        await Promise.resolve();
      });
      act(() => root.unmount());
      await act(async () => {
        if (outcome === 'resolve') {
          pending.resolve();
        } else {
          pending.reject(new Error('late form failure'));
        }
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(consoleError.mock.calls.flat().join(' ')).not.toMatch(
        /state update|unmounted component/i,
      );
      root = createRoot(container);
    },
  );

  it.each(['resolve', 'reject'] as const)(
    'does not let a closed submission settle into a reopened form when it %s',
    async outcome => {
      const pending = deferred<void>();
      const cancel = vi.fn();
      act(() =>
        root.render(
          <SandboxProviderForm
            capabilities={capabilities(true)}
            mode="create"
            open
            onCancel={cancel}
            onSubmit={() => pending.promise}
          />,
        ),
      );
      await act(async () => {
        setInput('Provider 名称', 'Old request');
        setInput('Provider Endpoint', 'https://old.example.test');
        setInput('Provider 凭据', 'old-secret');
        await Promise.resolve();
      });
      await act(async () => {
        Simulate.click(
          container.querySelector<HTMLButtonElement>(
            'button[aria-label="创建 Provider"]',
          )!,
        );
        await Promise.resolve();
      });

      act(() =>
        root.render(
          <SandboxProviderForm
            capabilities={capabilities(true)}
            mode="create"
            open={false}
            onCancel={cancel}
            onSubmit={vi.fn()}
          />,
        ),
      );
      act(() =>
        root.render(
          <SandboxProviderForm
            capabilities={capabilities(true)}
            mode="create"
            open
            onCancel={cancel}
            onSubmit={vi.fn()}
          />,
        ),
      );
      await act(async () => {
        setInput('Provider 名称', 'Fresh request');
        setInput('Provider Endpoint', 'https://fresh.example.test');
        setInput('Provider 凭据', 'fresh-secret');
        await Promise.resolve();
      });

      await act(async () => {
        if (outcome === 'resolve') {
          pending.resolve();
        } else {
          pending.reject(new Error('late form failure'));
        }
        await Promise.resolve();
        await Promise.resolve();
      });

      expect(
        container.querySelector<HTMLInputElement>(
          'input[aria-label="Provider 名称"]',
        )?.value,
      ).toBe('Fresh request');
      expect(
        container.querySelector<HTMLInputElement>(
          'input[aria-label="Provider Endpoint"]',
        )?.value,
      ).toBe('https://fresh.example.test');
      expect(
        container.querySelector<HTMLInputElement>(
          'input[aria-label="Provider 凭据"]',
        )?.value,
      ).toBe('fresh-secret');
      expect(container.textContent).not.toContain('late form failure');
    },
  );

  it('keeps Escape dismissal and confirms explicit credential replacement', async () => {
    const cancel = vi.fn();
    const submit = vi.fn().mockResolvedValue(undefined);
    vi.spyOn(window, 'confirm').mockReturnValue(true);
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="edit"
          open
          provider={providerFixture}
          onCancel={cancel}
          onSubmit={submit}
        />,
      ),
    );
    await act(async () => {
      Simulate.change(
        container.querySelector<HTMLInputElement>(
          'input[aria-label="替换 Provider 凭据"]',
        )!,
        { target: { checked: true } },
      );
    });
    await act(async () => {
      setInput('新的 Provider 凭据', 'replacement-secret');
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存 Provider"]',
        )!,
      );
      await Promise.resolve();
    });
    expect(submit).toHaveBeenCalledWith(
      expect.objectContaining({
        credential: { mode: 'replace', value: 'replacement-secret' },
      }),
    );
    act(() =>
      Simulate.keyDown(
        container.querySelector<HTMLElement>('[role="dialog"]')!,
        { key: 'Escape' },
      ),
    );
    expect(cancel).toHaveBeenCalledTimes(1);
  });

  it('clears connection secrets on cancel, close and successful submit', async () => {
    const cancel = vi.fn();
    const submit = vi.fn().mockResolvedValue(undefined);
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open
          onCancel={cancel}
          onSubmit={submit}
        />,
      ),
    );
    await act(async () => {
      setInput('Provider 名称', 'Primary');
      setInput('Provider Endpoint', 'https://runner.example.test');
      setInput('Provider 凭据', 'cancel-secret');
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="取消 Provider 表单"]',
        )!,
      );
      await Promise.resolve();
    });
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="Provider Endpoint"]',
      )?.value,
    ).toBe('');
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="Provider 凭据"]',
      )?.value,
    ).toBe('');

    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open={false}
          onCancel={cancel}
          onSubmit={submit}
        />,
      ),
    );
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open
          onCancel={cancel}
          onSubmit={submit}
        />,
      ),
    );
    await act(async () => {
      setInput('Provider 名称', 'Primary');
      setInput('Provider Endpoint', 'https://runner.example.test');
      setInput('Provider 凭据', 'submit-secret');
      await Promise.resolve();
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="创建 Provider"]',
        )!,
      );
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(submit).toHaveBeenLastCalledWith(
      expect.objectContaining({
        credential: { mode: 'replace', value: 'submit-secret' },
      }),
    );
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="Provider Endpoint"]',
      )?.value,
    ).toBe('');
    expect(
      container.querySelector<HTMLInputElement>(
        'input[aria-label="Provider 凭据"]',
      )?.value,
    ).toBe('');
    expect(container.innerHTML).not.toContain('submit-secret');
  });

  it('traps Tab in the dialog, closes with Escape and restores opener focus', async () => {
    const opener = document.createElement('button');
    document.body.appendChild(opener);
    opener.focus();
    const cancel = vi.fn();
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open
          onCancel={cancel}
          onSubmit={vi.fn()}
        />,
      ),
    );
    const dialog = container.querySelector<HTMLElement>('[role="dialog"]')!;
    const close = container.querySelector<HTMLButtonElement>(
      'button[aria-label="关闭 Provider 表单"]',
    )!;
    const save = container.querySelector<HTMLButtonElement>(
      'button[aria-label="创建 Provider"]',
    )!;
    expect(document.activeElement?.getAttribute('aria-label')).toBe(
      'Provider 名称',
    );

    save.focus();
    act(() => Simulate.keyDown(dialog, { key: 'Tab' }));
    expect(document.activeElement).toBe(close);
    close.focus();
    act(() => Simulate.keyDown(dialog, { key: 'Tab', shiftKey: true }));
    expect(document.activeElement).toBe(save);
    act(() => Simulate.keyDown(dialog, { key: 'Escape' }));
    expect(cancel).toHaveBeenCalledTimes(1);
    act(() =>
      root.render(
        <SandboxProviderForm
          capabilities={capabilities(true)}
          mode="create"
          open={false}
          onCancel={cancel}
          onSubmit={vi.fn()}
        />,
      ),
    );
    expect(document.activeElement).toBe(opener);
    opener.remove();
  });
});
