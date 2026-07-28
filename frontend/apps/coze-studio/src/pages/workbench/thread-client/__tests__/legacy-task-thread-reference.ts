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

export interface LegacyTaskThreadEnvelope<T> {
  data?: T;
  code: number;
  msg: string;
}

export type DeepReadonly<T> = T extends (...args: never[]) => unknown
  ? T
  : T extends readonly (infer Item)[]
    ? readonly DeepReadonly<Item>[]
    : T extends object
      ? { readonly [Key in keyof T]: DeepReadonly<T[Key]> }
      : T;

const deepFreeze = (value: unknown): void => {
  if (!value || typeof value !== 'object' || Object.isFrozen(value)) {
    return;
  }

  for (const child of Object.values(value)) {
    deepFreeze(child);
  }

  Object.freeze(value);
};

export const freezeLegacyTaskThreadInput = <T>(value: T): DeepReadonly<T> => {
  deepFreeze(value);

  return value as DeepReadonly<T>;
};

export const unwrapLegacyTaskThreadResponse = <T>(
  response: LegacyTaskThreadEnvelope<T>,
): T | undefined => {
  if (response.code !== 0) {
    throw new Error(response.msg || 'Legacy task thread request failed');
  }

  return response.data;
};

export const legacyTaskThreadReference = freezeLegacyTaskThreadInput({
  envelope: 'code_msg_data',
  ids: 'string',
  timestamps: 'epoch_milliseconds',
  usage_fields_to_drop: ['raw_usage'],
  run_event_fields_to_drop: ['journal_messages'],
});
