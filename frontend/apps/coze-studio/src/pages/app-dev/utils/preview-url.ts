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

const LOCAL_PREVIEW_HOSTS = new Set(['localhost', '127.0.0.1', '[::1]', '::1']);

export const normalizeAppDevPreviewUrl = (
  value?: string,
  applicationOrigin = typeof window === 'undefined'
    ? undefined
    : window.location.origin,
) => {
  if (!value) {
    return undefined;
  }

  try {
    const url = new URL(value);
    if (url.username || url.password) {
      return undefined;
    }
    if (applicationOrigin && url.origin === new URL(applicationOrigin).origin) {
      return undefined;
    }
    if (url.protocol === 'https:') {
      return url.toString();
    }
    if (url.protocol === 'http:' && LOCAL_PREVIEW_HOSTS.has(url.hostname)) {
      return url.toString();
    }
  } catch {
    return undefined;
  }

  return undefined;
};
