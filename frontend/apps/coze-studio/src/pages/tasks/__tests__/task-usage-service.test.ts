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

import { getTaskThreadTokenUsage } from '../task-usage-service';

const originalXMLHttpRequestDescriptor = Object.getOwnPropertyDescriptor(
  globalThis,
  'XMLHttpRequest',
);

class TaskUsageTestXMLHttpRequest {
  static instances: TaskUsageTestXMLHttpRequest[] = [];

  aborted = false;
  onabort: ((event: Event) => void) | null = null;
  onerror: ((event: Event) => void) | null = null;
  onloadend: ((event: Event) => void) | null = null;
  onreadystatechange: ((event: Event) => void) | null = null;
  ontimeout: ((event: Event) => void) | null = null;
  readyState = 0;
  responseText = '';
  responseType: XMLHttpRequestResponseType = '';
  status = 0;
  statusText = '';
  timeout = 0;
  upload = {
    addEventListener: () => undefined,
  };
  withCredentials = false;

  constructor() {
    TaskUsageTestXMLHttpRequest.instances.push(this);
  }

  abort() {
    this.aborted = true;
    this.onabort?.(new Event('abort'));
  }

  addEventListener() {
    return undefined;
  }

  getAllResponseHeaders() {
    return '';
  }

  open() {
    this.readyState = 1;
  }

  removeEventListener() {
    return undefined;
  }

  send() {
    return undefined;
  }

  setRequestHeader() {
    return undefined;
  }
}

beforeEach(() => {
  TaskUsageTestXMLHttpRequest.instances = [];
  Object.defineProperty(globalThis, 'XMLHttpRequest', {
    configurable: true,
    value: TaskUsageTestXMLHttpRequest,
    writable: true,
  });
});

afterEach(() => {
  if (originalXMLHttpRequestDescriptor) {
    Object.defineProperty(
      globalThis,
      'XMLHttpRequest',
      originalXMLHttpRequestDescriptor,
    );
  } else {
    Reflect.deleteProperty(globalThis, 'XMLHttpRequest');
  }
});

describe('task usage generated client cancellation boundary', () => {
  it('propagates an external AbortSignal through the real generated API request', async () => {
    const controller = new AbortController();
    const request = getTaskThreadTokenUsage(
      {
        thread_id: 'thread-abort',
        page: 1,
        page_size: 50,
      },
      { signal: controller.signal },
    );
    const rejection = expect(request).rejects.toMatchObject({
      name: 'AbortError',
    });

    await Promise.resolve();
    await Promise.resolve();

    expect(TaskUsageTestXMLHttpRequest.instances).toHaveLength(1);
    const xhr = TaskUsageTestXMLHttpRequest.instances[0];
    expect(xhr.aborted).toBe(false);

    controller.abort();

    expect(xhr.aborted).toBe(true);
    await rejection;
  });
});
