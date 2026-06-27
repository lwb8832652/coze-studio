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

import type { workbenchSkill } from '@coze-studio/api-schema';

type SkillResource = workbenchSkill.SkillResource;

const TEXT_RESOURCE_PATTERN =
  /\.(?:c|cc|conf|cpp|css|csv|go|h|html|ini|java|js|json|jsx|md|mjs|py|rb|rs|sh|sql|toml|ts|tsx|txt|xml|yaml|yml)$/i;

export const resourceIsText = (resource?: SkillResource) =>
  resource ? TEXT_RESOURCE_PATTERN.test(resource.path) : false;

export const decodeBase64Text = (content: string) => {
  const binary = window.atob(content);
  const bytes = Uint8Array.from(binary, character => character.charCodeAt(0));

  return new TextDecoder().decode(bytes);
};

export const encodeBase64Text = (content: string) => {
  const bytes = new TextEncoder().encode(content);
  let binary = '';

  bytes.forEach(byte => {
    binary += String.fromCharCode(byte);
  });

  return window.btoa(binary);
};

const base64ToBlob = (content: string, contentType: string) => {
  const binary = window.atob(content);
  const bytes = Uint8Array.from(binary, character => character.charCodeAt(0));

  return new Blob([bytes.buffer], { type: contentType });
};

export const downloadVersionArchive = (
  content: string,
  contentType: string,
  fileName: string,
) => {
  const url = URL.createObjectURL(base64ToBlob(content, contentType));
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  link.click();
  URL.revokeObjectURL(url);
};

export const formatVersionTime = (timestamp: number) =>
  timestamp ? new Date(timestamp).toLocaleString() : '-';
