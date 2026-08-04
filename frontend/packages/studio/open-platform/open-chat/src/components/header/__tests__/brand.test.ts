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

// @vitest-environment jsdom

import { afterEach, describe, expect, it, vi } from 'vitest';

vi.mock('@coze-arch/i18n', () => ({
  I18n: { t: vi.fn(() => 'Acme AI') },
}));

import { resolveChatHeaderBrand } from '../brand';

describe('resolveChatHeaderBrand', () => {
  afterEach(() => {
    document.head.querySelectorAll('link[rel="icon"]').forEach(node => {
      node.remove();
    });
  });

  it('uses the configured site name and managed favicon when header values are absent', () => {
    const favicon = document.createElement('link');
    favicon.rel = 'icon';
    favicon.dataset.cozeSiteConfig = 'true';
    favicon.href = '/site-brand/favicon.png';
    document.head.appendChild(favicon);

    expect(resolveChatHeaderBrand('', undefined)).toEqual({
      title: 'Acme AI',
      iconUrl: favicon.href,
    });
  });

  it('preserves supplied header values', () => {
    expect(
      resolveChatHeaderBrand(
        'Project assistant',
        'https://assets.example.com/project.png',
      ),
    ).toEqual({
      title: 'Project assistant',
      iconUrl: 'https://assets.example.com/project.png',
    });
  });
});
