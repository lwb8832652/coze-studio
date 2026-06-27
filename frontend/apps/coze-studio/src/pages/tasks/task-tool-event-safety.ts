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

const UNSAFE_TOOL_DETAIL_PATTERN =
  /api[_-]?key|access[_-]?token|authorization|bearer|credential|secret|password|token|tool_arguments?|arguments?|tool_result|result|provider_raw|object[_-]?key|checkpoint|https?:\/\/|file:\/\//i;
const SAFE_TOOL_NAME_PATTERN = /^[A-Za-z_][A-Za-z0-9_]{0,63}$/;

export const getSafeTaskToolName = (value?: string) => {
  const trimmed = value?.trim() ?? '';

  if (!trimmed || !SAFE_TOOL_NAME_PATTERN.test(trimmed)) {
    return '工具';
  }

  if (UNSAFE_TOOL_DETAIL_PATTERN.test(trimmed)) {
    return '工具';
  }

  return trimmed;
};

export const getSafeTaskToolDetail = (
  value: string | undefined,
  fallback: string,
) => {
  const trimmed = value?.trim() ?? '';

  if (!trimmed) {
    return fallback;
  }

  if (UNSAFE_TOOL_DETAIL_PATTERN.test(trimmed)) {
    return fallback;
  }

  return trimmed;
};
