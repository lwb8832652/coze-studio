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

const copyTextWithTextareaFallback = (text: string) => {
  if (typeof document.execCommand !== 'function') {
    return false;
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.left = '-9999px';
  textarea.style.top = '0';
  document.body.appendChild(textarea);
  textarea.select();
  textarea.setSelectionRange(0, text.length);

  try {
    return document.execCommand('copy');
  } finally {
    textarea.remove();
  }
};

export const copyTextToClipboard = async (text: string) => {
  if (!text) {
    return false;
  }

  try {
    await navigator.clipboard?.writeText(text);
    if (navigator.clipboard?.writeText) {
      return true;
    }
  } catch (err) {
    // Permission denials are expected in embedded browsers; fallback handles it.
    void err;
  }

  return copyTextWithTextareaFallback(text);
};
