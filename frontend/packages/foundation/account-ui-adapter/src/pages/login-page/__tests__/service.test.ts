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

import { describe, expect, it } from 'vitest';

import { resolveLoginRedirect } from '../redirect';

describe('resolveLoginRedirect', () => {
  it('keeps internal application redirects', () => {
    expect(resolveLoginRedirect('/space/1001/app-dev?tab=published')).toBe(
      '/space/1001/app-dev?tab=published',
    );
  });

  it.each([null, '', 'https://evil.example', '//evil.example', '/\\evil'])(
    'rejects unsafe redirect %s',
    redirect => {
      expect(resolveLoginRedirect(redirect)).toBe('/');
    },
  );
});
