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

import { describe, expect, it } from 'vitest';

import {
  DEFAULT_CODE_PLUGIN_SCHEMA,
  normalizeCodePluginSchema,
} from '../code-plugin-schema';

describe('normalizeCodePluginSchema', () => {
  it('formats an object schema', () => {
    expect(
      normalizeCodePluginSchema(
        '{"type":"object","properties":{"name":{"type":"string"}}}',
      ),
    ).toContain('"name"');
  });

  it.each(['[]', '{"type":"array"}', '{"type":"object","properties":[]}'])(
    'rejects an unsupported schema: %s',
    value => {
      expect(() => normalizeCodePluginSchema(value)).toThrow();
    },
  );

  it.each([
    '{"type":"object","properties":{"name":1}}',
    '{"type":"object","properties":{"name":{"type":"invalid"}}}',
    '{"type":"object","required":"name"}',
    '{"type":"object","items":1}',
    '{"type":"object","allOf":{}}',
    '{"type":"object","anyOf":[1]}',
    '{"type":"object","not":1}',
    '{"type":"object","enum":{}}',
    '{"type":"object","enum":[]}',
    '{"type":"object","minLength":"1"}',
    '{"type":"object","maxItems":-1}',
    '{"type":"object","minimum":"0"}',
    '{"type":"object","multipleOf":0}',
    '{"type":"object","uniqueItems":"true"}',
    '{"type":"object","format":1}',
    '{"type":"object","pattern":"["}',
    '{"type":"object","patternProperties":{"[":{"type":"string"}}}',
    '{"type":"object","dependentRequired":{"name":"email"}}',
    '{"type":"object","additionalProperties":1}',
    '{"type":"object","$ref":"https://example.com/schema.json"}',
  ])('rejects an invalid nested schema: %s', value => {
    expect(() => normalizeCodePluginSchema(value)).toThrow();
  });

  it('accepts recursively valid properties and combinators', () => {
    expect(() =>
      normalizeCodePluginSchema(
        JSON.stringify({
          type: 'object',
          properties: {
            profile: {
              type: 'object',
              required: ['name'],
              properties: { name: { type: 'string' } },
            },
          },
          allOf: [{ required: ['profile'] }],
          enum: [{ profile: { name: 'Alice' } }],
          minProperties: 1,
          maxProperties: 3,
          additionalProperties: false,
          dependentRequired: { profile: ['profile'] },
          format: 'custom-object',
        }),
      ),
    ).not.toThrow();
  });

  it('keeps a valid default schema', () => {
    expect(normalizeCodePluginSchema(DEFAULT_CODE_PLUGIN_SCHEMA)).toBe(
      DEFAULT_CODE_PLUGIN_SCHEMA,
    );
  });
});
