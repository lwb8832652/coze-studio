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

import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { SystemSettingsSection } from '../system-settings-section';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('SystemSettingsSection', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it('renders configured system settings', () => {
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{
            admin_emails: 'admin@example.test',
            disable_user_registration: true,
            server_host: 'http://localhost:8888',
          }}
          knowledgeConfig={{
            builtin_model_id: 42,
            embedding_config: {
              type: 1,
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain('服务地址');
    expect(container.textContent).toContain('http://localhost:8888');
    expect(container.textContent).toContain('注册开关');
    expect(container.textContent).toContain('已关闭');
    expect(container.textContent).toContain('管理员邮箱');
    expect(container.textContent).toContain('admin@example.test');
    expect(container.textContent).toContain('知识库配置');
    expect(container.textContent).toContain('内置模型 ID：42');
    expect(container.textContent).toContain('Embedding：已配置');
  });

  it('renders empty setting hints', () => {
    act(() => {
      root.render(
        <SystemSettingsSection
          basicConfig={{}}
          knowledgeConfig={{}}
        />,
      );
    });

    expect(container.textContent).toContain('服务地址未配置');
    expect(container.textContent).toContain('管理员邮箱未配置');
    expect(container.textContent).toContain('内置模型 ID：未配置');
    expect(container.textContent).toContain('Embedding：未配置');
  });
});
