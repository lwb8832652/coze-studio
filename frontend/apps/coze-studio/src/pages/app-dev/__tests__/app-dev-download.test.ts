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

import { afterEach, describe, expect, it, vi } from 'vitest';

import { downloadAppDevBlob } from '../utils/download-blob';

describe('downloadAppDevBlob', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('clicks an opaque blob URL and always revokes it', () => {
    vi.useFakeTimers();
    const create = vi
      .spyOn(URL, 'createObjectURL')
      .mockReturnValue('blob:safe');
    const revoke = vi
      .spyOn(URL, 'revokeObjectURL')
      .mockImplementation(() => undefined);
    const click = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined);

    downloadAppDevBlob(new Blob(['zip']), 'release.zip');
    expect(create).toHaveBeenCalledTimes(1);
    expect(click).toHaveBeenCalledTimes(1);
    expect(revoke).not.toHaveBeenCalled();
    vi.runAllTimers();
    expect(revoke).toHaveBeenCalledWith('blob:safe');
  });

  it('removes the anchor and revokes the URL when click throws', () => {
    const create = vi
      .spyOn(URL, 'createObjectURL')
      .mockReturnValue('blob:safe');
    const revoke = vi
      .spyOn(URL, 'revokeObjectURL')
      .mockImplementation(() => undefined);
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {
      throw new Error('blocked click');
    });

    expect(() => downloadAppDevBlob(new Blob(['zip']), 'release.zip')).toThrow(
      'blocked click',
    );
    expect(create).toHaveBeenCalledTimes(1);
    expect(revoke).toHaveBeenCalledWith('blob:safe');
    expect(document.body.querySelector('a[download="release.zip"]')).toBeNull();
  });
});
