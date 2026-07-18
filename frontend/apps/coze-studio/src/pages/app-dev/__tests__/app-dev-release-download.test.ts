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

/* eslint-disable @typescript-eslint/require-await -- Response test doubles preserve async browser contracts. */

import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  AppDevSafeError,
  downloadAppDevRelease,
  exportAppDevProject,
} from '../service';

interface ReleaseResponseFixture {
  blob?: Blob | unknown;
  blobError?: unknown;
  headers?: Record<string, string>;
  ok?: boolean;
  status?: number;
  streamChunks?: Uint8Array[];
  streamNeverSettles?: boolean;
  nullBody?: boolean;
}

const releaseResponse = ({
  blob = new Blob(['zip-data'], { type: 'application/zip' }),
  blobError,
  headers = {},
  ok = true,
  status = 200,
  streamChunks,
  streamNeverSettles = false,
  nullBody = false,
}: ReleaseResponseFixture = {}) => {
  const normalizedHeaders = new Map(
    Object.entries(headers).map(([key, value]) => [key.toLowerCase(), value]),
  );
  const cancel = vi.fn().mockResolvedValue(undefined);
  const readBlob = blobError
    ? vi.fn().mockRejectedValue(blobError)
    : vi.fn().mockResolvedValue(blob);
  const json = vi.fn().mockResolvedValue({
    message: 'https://provider.invalid/?token=raw-secret Bearer api-key',
  });
  let chunkIndex = 0;
  const read = streamNeverSettles
    ? vi.fn(() => new Promise<never>(() => undefined))
    : vi
        .fn()
        .mockImplementation(async () =>
          streamChunks && chunkIndex < streamChunks.length
            ? { done: false, value: streamChunks[chunkIndex++] }
            : { done: true, value: undefined },
        );
  const readerCancel = vi.fn().mockResolvedValue(undefined);
  const body = nullBody
    ? null
    : streamChunks || streamNeverSettles
      ? {
          cancel,
          getReader: () => ({
            read,
            cancel: readerCancel,
            releaseLock: vi.fn(),
          }),
        }
      : { cancel };
  return {
    cancel,
    json,
    readBlob,
    read,
    readerCancel,
    response: {
      ok,
      status,
      headers: {
        get: (name: string) =>
          normalizedHeaders.get(name.toLowerCase()) ?? null,
      },
      body,
      blob: readBlob,
      json,
    } as unknown as Response,
  };
};

const download = () =>
  downloadAppDevRelease({ spaceId: 'space-1', projectId: 'project-1' });

describe('AppDev release download boundary', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('checks the stale capability before status or response metadata', async () => {
    const fixture = releaseResponse({
      ok: false,
      status: 503,
      headers: {
        'x-appdev-release-stale': 'true',
        'content-type': 'text/html',
      },
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    const error = await download().catch(caught => caught);

    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({ code: 'release_stale' });
    expect(fixture.cancel).toHaveBeenCalledTimes(1);
    expect(fixture.readBlob).not.toHaveBeenCalled();
    expect(fixture.json).not.toHaveBeenCalled();
  });

  it('does not parse or expose a non-success response body', async () => {
    const fixture = releaseResponse({ ok: false, status: 503 });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    const error = await download().catch(caught => caught);

    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({ code: 'unavailable' });
    expect(String(error)).not.toMatch(/provider\.invalid|raw-secret|Bearer/u);
    expect(fixture.cancel).toHaveBeenCalledTimes(1);
    expect(fixture.readBlob).not.toHaveBeenCalled();
    expect(fixture.json).not.toHaveBeenCalled();
  });

  it('normalizes release transport failures without exposing raw details', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(
      new Error('https://provider.invalid/?token=raw-secret'),
    );

    const error = await download().catch(caught => caught);

    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({ code: 'unavailable' });
    expect(String(error)).not.toMatch(/provider\.invalid|raw-secret/u);
  });

  it('accepts the backend zip contract and matching optional content length', async () => {
    const bytes = new TextEncoder().encode('zip-data');
    const fixture = releaseResponse({
      streamChunks: [bytes],
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
        'content-length': String(bytes.byteLength),
      },
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    await expect(download()).resolves.toMatchObject({ size: bytes.byteLength });
    expect(fixture.cancel).not.toHaveBeenCalled();
    expect(fixture.readBlob).not.toHaveBeenCalled();
  });

  it('accepts a valid release when Content-Length is omitted', async () => {
    const bytes = new TextEncoder().encode('zip-data');
    const fixture = releaseResponse({
      streamChunks: [bytes],
      headers: {
        'content-type': 'application/zip; charset=binary',
        'content-disposition': 'attachment; filename=appdev-release.zip',
      },
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    await expect(download()).resolves.toMatchObject({ size: bytes.byteLength });
  });

  it.each([
    ['release', () => download()],
    [
      'export',
      () => exportAppDevProject({ spaceId: 'space-1', projectId: 'project-1' }),
    ],
  ])('cancels a chunked %s at the 128 MiB boundary', async (_, request) => {
    const oneMiB = new Uint8Array(1024 * 1024);
    const fixture = releaseResponse({
      streamChunks: Array.from({ length: 129 }, () => oneMiB),
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
      },
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    await expect(request()).rejects.toMatchObject({ code: 'invalid_response' });
    expect(fixture.readerCancel).toHaveBeenCalledTimes(1);
    expect(fixture.read).toHaveBeenCalledTimes(129);
    expect(fixture.readBlob).not.toHaveBeenCalled();
  });

  it('cancels a pending stream when the external signal aborts', async () => {
    const fixture = releaseResponse({
      streamNeverSettles: true,
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
      },
    });
    const controller = new AbortController();
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    const promise = downloadAppDevRelease({
      spaceId: 'space-1',
      projectId: 'project-1',
      signal: controller.signal,
    });
    await Promise.resolve();
    controller.abort();

    await expect(promise).rejects.toBeInstanceOf(AppDevSafeError);
    expect(fixture.readerCancel).toHaveBeenCalledTimes(1);
  });

  it('rejects a non-streaming body without a trusted length', async () => {
    const fixture = releaseResponse({
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
      },
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    await expect(download()).rejects.toMatchObject({
      code: 'invalid_response',
    });
    expect(fixture.cancel).toHaveBeenCalledTimes(1);
    expect(fixture.readBlob).not.toHaveBeenCalled();
  });

  it('rejects a null body even with a trusted length without calling blob', async () => {
    const fixture = releaseResponse({
      nullBody: true,
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
        'content-length': '8',
      },
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    await expect(download()).rejects.toMatchObject({
      code: 'invalid_response',
    });
    expect(fixture.readBlob).not.toHaveBeenCalled();
  });

  it.each([
    {
      name: 'HTML content',
      headers: {
        'content-type': 'text/html',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
      },
    },
    {
      name: 'JSON content',
      headers: {
        'content-type': 'application/json',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
      },
    },
    {
      name: 'inline disposition',
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'inline; filename="appdev-release.zip"',
      },
    },
    {
      name: 'missing disposition',
      headers: { 'content-type': 'application/zip' },
    },
    {
      name: 'missing content type',
      headers: {
        'content-disposition': 'attachment; filename="appdev-release.zip"',
      },
    },
    {
      name: 'CRLF filename',
      headers: {
        'content-type': 'application/zip',
        'content-disposition':
          'attachment; filename="release.zip\r\nX-Secret: token"',
      },
    },
    {
      name: 'traversal filename',
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="../release.zip"',
      },
    },
    {
      name: 'negative length',
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
        'content-length': '-1',
      },
    },
    {
      name: 'fractional length',
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
        'content-length': '8.5',
      },
    },
    {
      name: 'oversized length',
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
        'content-length': String(512 * 1024 * 1024 + 1),
      },
    },
  ])(
    'rejects $name before creating a browser object URL',
    async ({ headers }) => {
      const fixture = releaseResponse({ headers });
      const createObjectURL = vi.spyOn(URL, 'createObjectURL');
      vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

      const error = await download().catch(caught => caught);

      expect(error).toBeInstanceOf(AppDevSafeError);
      expect(error).toMatchObject({ code: 'invalid_response' });
      expect(fixture.cancel).toHaveBeenCalledTimes(1);
      expect(fixture.readBlob).not.toHaveBeenCalled();
      expect(createObjectURL).not.toHaveBeenCalled();
    },
  );

  it.each([
    ['mismatched size', new Blob(['short']), '100'],
    ['non-Blob body', 'not-a-blob', undefined],
  ])(
    'rejects a %s response shape after cancelling it',
    async (_, blob, length) => {
      const fixture = releaseResponse({
        blob,
        headers: {
          'content-type': 'application/zip',
          'content-disposition': 'attachment; filename="appdev-release.zip"',
          ...(length ? { 'content-length': length } : {}),
        },
      });
      const createObjectURL = vi.spyOn(URL, 'createObjectURL');
      vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

      const error = await download().catch(caught => caught);

      expect(error).toBeInstanceOf(AppDevSafeError);
      expect(error).toMatchObject({ code: 'invalid_response' });
      expect(fixture.cancel).toHaveBeenCalledTimes(1);
      expect(createObjectURL).not.toHaveBeenCalled();
    },
  );

  it('maps blob decoding failures to a fixed invalid response', async () => {
    const fixture = releaseResponse({
      blobError: new Error('s3://private/object?token=raw-secret'),
      headers: {
        'content-type': 'application/zip',
        'content-disposition': 'attachment; filename="appdev-release.zip"',
      },
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    const error = await download().catch(caught => caught);

    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({ code: 'invalid_response' });
    expect(String(error)).not.toContain('raw-secret');
    expect(fixture.cancel).toHaveBeenCalledTimes(1);
  });
});
