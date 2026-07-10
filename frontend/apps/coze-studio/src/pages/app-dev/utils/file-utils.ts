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

export const getLanguageFromPath = (path: string) => {
  const extension = path.split('.').pop()?.toLowerCase();

  switch (extension) {
    case 'ts':
    case 'tsx':
      return 'typescript';
    case 'js':
    case 'jsx':
      return 'javascript';
    case 'css':
    case 'less':
      return 'css';
    case 'html':
      return 'html';
    case 'json':
      return 'json';
    case 'md':
      return 'markdown';
    default:
      return 'plaintext';
  }
};

export const formatFileSize = (size?: number) => {
  if (!size) {
    return '0 B';
  }

  if (size < 1024) {
    return `${size} B`;
  }

  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }

  return `${(size / 1024 / 1024).toFixed(1)} MB`;
};

export const getFileName = (path: string) => {
  const segments = path.split('/').filter(Boolean);
  return segments.at(-1) || path;
};

export const isEditableFile = (path: string, size?: number) => {
  if (size && size > 2 * 1024 * 1024) {
    return false;
  }

  return [
    'typescript',
    'javascript',
    'css',
    'html',
    'json',
    'markdown',
    'plaintext',
  ].includes(getLanguageFromPath(path));
};
