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

import { configureMonacoWorkers } from '../monaco-workers';

interface MonacoTestEnvironment {
  getWorker?: (workerId: string, label: string) => Worker;
}

type MonacoTestGlobal = typeof globalThis & {
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Mirrors Monaco's browser-standard global name.
  MonacoEnvironment?: MonacoTestEnvironment;
};

class WorkerMock {
  readonly options?: WorkerOptions;
  readonly url: string | URL;

  constructor(url: string | URL, options?: WorkerOptions) {
    this.url = url;
    this.options = options;
  }
}

const runtimeGlobal = globalThis as MonacoTestGlobal;

describe('configureMonacoWorkers', () => {
  afterEach(() => {
    delete runtimeGlobal.MonacoEnvironment;
    vi.unstubAllGlobals();
  });

  it.each([
    ['typescript', 'typescript/ts.worker'],
    ['javascript', 'typescript/ts.worker'],
    ['json', 'json/json.worker'],
    ['css', 'css/css.worker'],
    ['less', 'css/css.worker'],
    ['scss', 'css/css.worker'],
    ['html', 'html/html.worker'],
    ['plaintext', 'editor/editor.worker'],
  ])('routes %s models to the expected worker', (label, expectedPath) => {
    vi.stubGlobal('Worker', WorkerMock);
    configureMonacoWorkers();

    const worker = runtimeGlobal.MonacoEnvironment?.getWorker?.(
      'worker-id',
      label,
    ) as unknown as WorkerMock;

    expect(worker.url.toString()).toContain(expectedPath);
    expect(worker.options).toEqual({ type: 'module' });
  });

  it('keeps an existing host worker configuration', () => {
    const getWorker = vi.fn();
    runtimeGlobal.MonacoEnvironment = { getWorker };

    configureMonacoWorkers();

    expect(runtimeGlobal.MonacoEnvironment.getWorker).toBe(getWorker);
  });
});
