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

interface MonacoEnvironmentConfig {
  getWorker?: (workerId: string, label: string) => Worker;
  [key: string]: unknown;
}

type MonacoRuntimeGlobal = typeof globalThis & {
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Monaco reads this browser-standard global name.
  MonacoEnvironment?: MonacoEnvironmentConfig;
};

const createMonacoWorker = (_workerId: string, label: string): Worker => {
  switch (label) {
    case 'json':
      return new Worker(
        new URL(
          'monaco-editor/esm/vs/language/json/json.worker',
          import.meta.url,
        ),
        { type: 'module' },
      );
    case 'css':
    case 'less':
    case 'scss':
      return new Worker(
        new URL(
          'monaco-editor/esm/vs/language/css/css.worker',
          import.meta.url,
        ),
        { type: 'module' },
      );
    case 'html':
    case 'handlebars':
    case 'razor':
      return new Worker(
        new URL(
          'monaco-editor/esm/vs/language/html/html.worker',
          import.meta.url,
        ),
        { type: 'module' },
      );
    case 'typescript':
    case 'javascript':
      return new Worker(
        new URL(
          'monaco-editor/esm/vs/language/typescript/ts.worker',
          import.meta.url,
        ),
        { type: 'module' },
      );
    default:
      return new Worker(
        new URL('monaco-editor/esm/vs/editor/editor.worker', import.meta.url),
        { type: 'module' },
      );
  }
};

export const configureMonacoWorkers = (): void => {
  const runtimeGlobal = globalThis as MonacoRuntimeGlobal;
  if (typeof runtimeGlobal.MonacoEnvironment?.getWorker === 'function') {
    return;
  }

  runtimeGlobal.MonacoEnvironment = {
    ...runtimeGlobal.MonacoEnvironment,
    getWorker: createMonacoWorker,
  };
};
