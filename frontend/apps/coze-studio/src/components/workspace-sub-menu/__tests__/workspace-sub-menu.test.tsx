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

import {
  ASSISTANT_BADGE,
  ASSISTANT_LABEL,
  WORKSPACE_MENU_META,
} from '../menu';

describe('Coze Studio WorkspaceSubMenu', () => {
  it('defines the Figma workspace navigation structure', () => {
    const labels = WORKSPACE_MENU_META.map(item => item.label);

    expect(ASSISTANT_LABEL).toBe('专属助理');
    expect(ASSISTANT_BADGE).toBe('Beta');
    expect(labels).toEqual([
      '新建任务',
      '资源配置',
      '技能配置',
      '开发配置',
      '任务触发器',
      '全部任务',
    ]);
    expect(WORKSPACE_MENU_META[0]).toMatchObject({
      label: '新建任务',
      path: 'workbench',
      variant: 'primary',
    });
  });
});
