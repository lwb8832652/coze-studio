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

import { dirname, resolve } from 'node:path';
import { existsSync } from 'node:fs';

import type { Plugin } from 'vite';
import { defineConfig } from '@coze-arch/vitest-config';

const packagesRoot = resolve(__dirname, '../../packages');

const packageSourceAliasPlugin = (): Plugin => ({
  name: 'coze-package-source-alias',
  enforce: 'pre',
  resolveId(source, importer) {
    if (!importer || !source.startsWith('@/')) {
      return null;
    }

    let currentDir = dirname(importer.split('?')[0]);
    while (currentDir.startsWith(packagesRoot)) {
      if (existsSync(resolve(currentDir, 'package.json'))) {
        return this.resolve(
          resolve(currentDir, 'src', source.slice(2)),
          importer,
          { skipSelf: true },
        );
      }

      const parentDir = dirname(currentDir);
      if (parentDir === currentDir) {
        break;
      }
      currentDir = parentDir;
    }

    return null;
  },
});

export default defineConfig(
  {
    dirname: __dirname,
    preset: 'web',
    plugins: [packageSourceAliasPlugin()],
    ssr: {
      noExternal: ['@coze-arch/coze-design', '@douyinfe/semi-ui'],
    },
    test: {
      setupFiles: ['./vitest.setup.ts'],
    },
  },
  {
    fixSemi: true,
  },
);
