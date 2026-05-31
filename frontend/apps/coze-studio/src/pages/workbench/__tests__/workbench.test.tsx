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

import type { ReactNode } from 'react';

import { vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    children,
    disabled,
    icon,
  }: {
    children: ReactNode;
    disabled?: boolean;
    icon?: ReactNode;
  }) => (
    <button type="button" disabled={disabled}>
      {icon}
      {children}
    </button>
  ),
  TextArea: ({
    'aria-label': ariaLabel,
    placeholder,
    value,
  }: {
    'aria-label'?: string;
    placeholder?: string;
    value?: string;
  }) => (
    <textarea
      aria-label={ariaLabel}
      placeholder={placeholder}
      value={value}
      readOnly
    />
  ),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozSendFill: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import WorkbenchPage from '../index';

describe('WorkbenchPage', () => {
  it('renders the static chat workbench first screen', () => {
    const markup = renderToStaticMarkup(<WorkbenchPage />);

    expect(markup).toContain('欢迎来到 刘文波 的工作空间');
    expect(markup).toContain('让我们一起高效完成工作吧');
    expect(markup).toContain(
      'Hi，我会根据你的任务特性，自动匹配最佳处理方式。',
    );
    expect(markup).toContain('aria-label="任务描述"');
    expect(markup).toContain('Auto');
    expect(markup).toContain('Ask');
    expect(markup).toContain('Agent');
    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain('aria-pressed="false"');
    expect(markup).toContain('工作总结');
    expect(markup).toContain('数据分析');
    expect(markup).toContain('代码分析');
    expect(markup).toContain('异常排查');
    expect(markup).toContain('飞书文档撰写');
  });
});
