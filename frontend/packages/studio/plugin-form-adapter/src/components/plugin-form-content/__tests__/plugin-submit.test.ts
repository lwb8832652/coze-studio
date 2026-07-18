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

import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { CreationMethod, PluginType } from '@coze-arch/bot-api/plugin_develop';

import type * as PluginFormUtils from '../utils';
import type { FormState } from '../hooks';

let pluginFormUtils: typeof PluginFormUtils;

const apiMocks = vi.hoisted(() => ({
  registerPluginMeta: vi.fn(),
}));

vi.mock('@coze-arch/bot-api', () => ({
  PluginDevelopApi: {
    RegisterPluginMeta: apiMocks.registerPluginMeta,
  },
}));

const createFormValue = (runtime: '1' | '2') =>
  ({
    name: 'code-plugin',
    desc: 'code plugin',
    plugin_uri: [{ uid: 'plugin-icon' }],
    auth_type: [0],
    creation_method: `${PluginType.FUNC}-${CreationMethod.IDE}`,
    ide_code_runtime: runtime,
  }) as unknown as FormState;

describe('code plugin form submission', () => {
  beforeAll(async () => {
    vi.stubGlobal('IS_OVERSEA', false);
    vi.stubGlobal('IS_BOE', false);
    pluginFormUtils = await import('../utils');
  });

  beforeEach(() => {
    vi.clearAllMocks();
    apiMocks.registerPluginMeta.mockResolvedValue({ plugin_id: 'plugin-1' });
  });

  it.each([
    ['Python', '1'],
    ['JavaScript', '2'],
  ] as const)(
    'submits FUNC + IDE with the selected %s runtime',
    async (_label, runtime) => {
      const params = pluginFormUtils.convertPluginMetaParams({
        val: createFormValue(runtime),
        spaceId: 'space-1',
        headerList: [],
        projectId: undefined,
        creationMethod: CreationMethod.IDE,
        defaultRuntime: '1',
        pluginType: PluginType.FUNC,
        extItemsJSON: {},
      });

      await pluginFormUtils.registerPluginMeta({ params });

      expect(apiMocks.registerPluginMeta).toHaveBeenCalledWith(
        expect.objectContaining({
          plugin_type: PluginType.FUNC,
          creation_method: CreationMethod.IDE,
          ide_code_runtime: runtime,
          space_id: 'space-1',
        }),
        { __disableErrorToast: true },
      );
    },
  );
});
