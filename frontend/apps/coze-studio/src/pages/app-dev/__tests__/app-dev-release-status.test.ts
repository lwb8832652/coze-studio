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

import { isAppDevReleaseStale } from '../utils/release-status';

describe('isAppDevReleaseStale', () => {
  it('keeps a release current when persisted source predates the build', () => {
    expect(
      isAppDevReleaseStale({
        lastBuildStatus: 'success',
        sourceUpdatedAt: '2026-07-10T12:20:46Z',
        lastBuildAt: '2026-07-10T12:21:13Z',
      }),
    ).toBe(false);
  });

  it('marks a release stale only when persisted source is newer', () => {
    expect(
      isAppDevReleaseStale({
        lastBuildStatus: 'success',
        sourceUpdatedAt: '2026-07-10T12:22:00Z',
        lastBuildAt: '2026-07-10T12:21:13Z',
      }),
    ).toBe(true);
  });
});
