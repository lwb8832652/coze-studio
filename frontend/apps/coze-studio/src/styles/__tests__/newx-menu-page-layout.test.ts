// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

const readSource = (relativePath: string) =>
  readFileSync(new URL(relativePath, import.meta.url), 'utf8');

const pageSources = [
  [
    '资源配置',
    readSource('../../pages/library.tsx'),
    'newx-menu-page--library',
  ],
  [
    '网页应用开发',
    readSource('../../pages/app-dev/index.tsx'),
    'app-dev-page newx-menu-page',
  ],
  [
    '技能配置',
    readSource('../../pages/skill/management-page.tsx'),
    'skill-management-page newx-menu-page',
  ],
  [
    '开发配置',
    readSource('../../pages/develop.tsx'),
    'newx-menu-page--develop',
  ],
  [
    '工作空间',
    readSource('../../pages/workspace/index.tsx'),
    'coze-prototype-team-setting-page newx-menu-page',
  ],
  [
    '任务中心',
    readSource('../../pages/task-center/index.tsx'),
    'task-center-page newx-menu-page',
  ],
  [
    '全部任务',
    readSource('../../pages/tasks/index.tsx'),
    'newx-tasks-page-shell',
  ],
] as const;

describe('NewX C 端菜单页面布局合同', () => {
  it.each(pageSources)('%s 使用统一菜单页语义类', (_, source, className) => {
    expect(source).toContain(className);
  });

  it('顶部状态栏和跨包列表暴露稳定布局类', () => {
    expect(readSource('../../components/workspace-page-top-bar.tsx')).toContain(
      'newx-workspace-topbar',
    );
    expect(
      readSource(
        '../../../../../packages/studio/workspace/entry-base/src/components/layout/list.tsx',
      ),
    ).toContain('newx-adapter-list-layout');
  });

  it('资源库包含面向 C 端用户的页面说明', () => {
    expect(
      readSource(
        '../../../../../packages/studio/workspace/entry-base/src/pages/library/components/library-header.tsx',
      ),
    ).toContain('workspace-library-description');
  });

  it('集中主题定义统一页面尺寸令牌', () => {
    const theme = readSource('../newx-components.less');

    expect(theme).toContain('--newx-page-max-width');
    expect(theme).toContain('--newx-page-padding-inline');
    expect(theme).toContain('.newx-menu-page');
  });
});
