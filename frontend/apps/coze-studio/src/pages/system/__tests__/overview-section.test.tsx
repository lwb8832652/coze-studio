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

import { OverviewSection } from '../overview-section';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('OverviewSection', () => {
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

  it('renders system overview totals and recent snapshots', () => {
    act(() => {
      root.render(
        <OverviewSection
          userTotal={12}
          workspaceTotal={3}
          basicConfig={{
            admin_emails: 'admin@example.test',
          }}
          users={[
            {
              user_id: '9',
              name: 'Owner',
              email: 'owner@example.test',
              user_unique_name: 'owner',
            },
          ]}
          workspaces={[
            {
              id: '101',
              name: '畅享 AI',
              owner_name: 'Owner',
              total_member_num: 2,
            },
          ]}
        />,
      );
    });

    expect(container.textContent).toContain('用户总数');
    expect(container.textContent).toContain('12');
    expect(container.textContent).toContain('工作空间总数');
    expect(container.textContent).toContain('3');
    expect(container.textContent).toContain('管理员邮箱');
    expect(container.textContent).toContain('admin@example.test');
    expect(container.textContent).toContain('最近用户');
    expect(container.textContent).toContain('Owner');
    expect(container.textContent).toContain('owner@example.test');
    expect(container.textContent).toContain('最近工作空间');
    expect(container.textContent).toContain('畅享 AI');
  });

  it('renders empty snapshot hints when overview lists are empty', () => {
    act(() => {
      root.render(
        <OverviewSection
          userTotal={0}
          workspaceTotal={0}
          basicConfig={{}}
          users={[]}
          workspaces={[]}
        />,
      );
    });

    expect(container.textContent).toContain('暂无用户快照');
    expect(container.textContent).toContain('暂无工作空间快照');
  });
});
