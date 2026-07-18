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

export const MAX_APP_DEV_SNAPSHOT_ID_LENGTH = 256;

const APP_DEV_SNAPSHOT_ID_PATTERN = /^[A-Za-z0-9_-]+$/u;

export const normalizeAppDevSnapshotID = (
  value: unknown,
): string | undefined => {
  if (
    typeof value !== 'string' ||
    value.length === 0 ||
    value.length > MAX_APP_DEV_SNAPSHOT_ID_LENGTH ||
    value !== value.trim() ||
    !APP_DEV_SNAPSHOT_ID_PATTERN.test(value)
  ) {
    return undefined;
  }
  return value;
};
