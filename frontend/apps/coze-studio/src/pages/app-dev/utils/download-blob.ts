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

export const downloadAppDevBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  let cleanupScheduled = false;
  let cleaned = false;
  const cleanup = () => {
    if (cleaned) {
      return;
    }
    cleaned = true;
    link.remove();
    URL.revokeObjectURL(url);
  };
  link.href = url;
  link.download = filename;
  link.rel = 'noreferrer';
  link.style.display = 'none';
  try {
    document.body.appendChild(link);
    link.click();
    window.setTimeout(cleanup, 0);
    cleanupScheduled = true;
  } finally {
    if (!cleanupScheduled) {
      cleanup();
    }
  }
};
