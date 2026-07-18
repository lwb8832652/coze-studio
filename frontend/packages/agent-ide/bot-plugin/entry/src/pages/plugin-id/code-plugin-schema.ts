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

const MAX_SCHEMA_BYTES = 64 * 1024;
const MAX_SCHEMA_DEPTH = 64;
const JSON_SCHEMA_TYPES = new Set([
  'array',
  'boolean',
  'integer',
  'null',
  'number',
  'object',
  'string',
]);

type SchemaObject = Record<string, unknown>;

const isObject = (value: unknown): value is SchemaObject =>
  Boolean(value) && !Array.isArray(value) && typeof value === 'object';

const assertNonNegativeInteger = (
  schema: SchemaObject,
  keyword: string,
  path: string,
) => {
  const value = schema[keyword];
  if (
    value !== undefined &&
    (!Number.isInteger(value) || (value as number) < 0)
  ) {
    throw new Error(`${path}.${keyword} 必须是非负整数`);
  }
};

const assertFiniteNumber = (
  schema: SchemaObject,
  keyword: string,
  path: string,
) => {
  const value = schema[keyword];
  const positive = keyword === 'multipleOf';
  if (
    value !== undefined &&
    (typeof value !== 'number' ||
      !Number.isFinite(value) ||
      (positive && value <= 0))
  ) {
    throw new Error(
      `${path}.${keyword} 必须是${positive ? '大于 0 的' : '有限'}数字`,
    );
  }
};

const assertString = (schema: SchemaObject, keyword: string, path: string) => {
  if (schema[keyword] !== undefined && typeof schema[keyword] !== 'string') {
    throw new Error(`${path}.${keyword} 必须是字符串`);
  }
};

const assertBoolean = (schema: SchemaObject, keyword: string, path: string) => {
  if (schema[keyword] !== undefined && typeof schema[keyword] !== 'boolean') {
    throw new Error(`${path}.${keyword} 必须是布尔值`);
  }
};

const assertLimitPairs = (schema: SchemaObject, path: string) => {
  for (const [minimum, maximum] of [
    ['minLength', 'maxLength'],
    ['minItems', 'maxItems'],
    ['minContains', 'maxContains'],
    ['minProperties', 'maxProperties'],
  ] as const) {
    const min = schema[minimum];
    const max = schema[maximum];
    if (typeof min === 'number' && typeof max === 'number' && min > max) {
      throw new Error(`${path}.${minimum} 不能大于 ${maximum}`);
    }
  }
};

const assertCommonKeywordTypes = (schema: SchemaObject, path: string) => {
  for (const keyword of [
    'minLength',
    'maxLength',
    'minItems',
    'maxItems',
    'minContains',
    'maxContains',
    'minProperties',
    'maxProperties',
  ]) {
    assertNonNegativeInteger(schema, keyword, path);
  }
  for (const keyword of [
    'minimum',
    'maximum',
    'exclusiveMinimum',
    'exclusiveMaximum',
  ]) {
    assertFiniteNumber(schema, keyword, path);
  }
  assertFiniteNumber(schema, 'multipleOf', path);

  for (const keyword of [
    '$id',
    '$anchor',
    '$dynamicAnchor',
    '$comment',
    'title',
    'description',
    'format',
    'contentEncoding',
    'contentMediaType',
  ]) {
    assertString(schema, keyword, path);
  }
  for (const keyword of [
    'uniqueItems',
    'readOnly',
    'writeOnly',
    'deprecated',
  ]) {
    assertBoolean(schema, keyword, path);
  }
  if (schema.examples !== undefined && !Array.isArray(schema.examples)) {
    throw new Error(`${path}.examples 必须是数组`);
  }
  if (schema.enum !== undefined) {
    if (!Array.isArray(schema.enum) || schema.enum.length === 0) {
      throw new Error(`${path}.enum 必须是非空数组`);
    }
    const values = schema.enum.map(item => JSON.stringify(item));
    if (new Set(values).size !== values.length) {
      throw new Error(`${path}.enum 不能包含重复值`);
    }
  }
  if (schema.pattern !== undefined) {
    assertString(schema, 'pattern', path);
    try {
      new RegExp(schema.pattern as string);
    } catch (error) {
      void error;
      throw new Error(`${path}.pattern 不是有效正则表达式`);
    }
  }
  assertLimitPairs(schema, path);
};

// eslint-disable-next-line @coze-arch/max-line-per-function -- One recursive traversal keeps nested JSON Schema keyword validation consistent.
const assertSchemaNode = (
  value: unknown,
  path: string,
  depth: number,
): void => {
  if (typeof value === 'boolean') {
    return;
  }
  if (!isObject(value)) {
    throw new Error(`${path} 必须是 JSON Schema 对象或布尔值`);
  }
  if (depth > MAX_SCHEMA_DEPTH) {
    throw new Error('参数结构嵌套不能超过 64 层');
  }

  if (value.type !== undefined) {
    const types = Array.isArray(value.type) ? value.type : [value.type];
    if (
      types.length === 0 ||
      types.some(
        type => typeof type !== 'string' || !JSON_SCHEMA_TYPES.has(type),
      )
    ) {
      throw new Error(`${path}.type 包含不支持的类型`);
    }
    if (new Set(types).size !== types.length) {
      throw new Error(`${path}.type 不能包含重复类型`);
    }
  }
  assertCommonKeywordTypes(value, path);

  for (const keyword of [
    'properties',
    'patternProperties',
    '$defs',
    'definitions',
    'dependentSchemas',
  ]) {
    const children = value[keyword];
    if (children === undefined) {
      continue;
    }
    if (!isObject(children)) {
      throw new Error(`${path}.${keyword} 必须是对象`);
    }
    Object.entries(children).forEach(([key, child]) =>
      assertSchemaNode(child, `${path}.${keyword}.${key}`, depth + 1),
    );
    if (keyword === 'patternProperties') {
      Object.keys(children).forEach(pattern => {
        try {
          new RegExp(pattern);
        } catch (error) {
          void error;
          throw new Error(`${path}.patternProperties.${pattern} 不是有效正则`);
        }
      });
    }
  }

  if (value.required !== undefined) {
    if (
      !Array.isArray(value.required) ||
      value.required.some(item => typeof item !== 'string') ||
      new Set(value.required).size !== value.required.length
    ) {
      throw new Error(`${path}.required 必须是不重复的字符串数组`);
    }
  }

  for (const keyword of ['items', 'prefixItems']) {
    const item = value[keyword];
    if (item === undefined) {
      continue;
    }
    if (Array.isArray(item)) {
      item.forEach((child, index) =>
        assertSchemaNode(child, `${path}.${keyword}[${index}]`, depth + 1),
      );
    } else {
      assertSchemaNode(item, `${path}.${keyword}`, depth + 1);
    }
  }

  for (const keyword of ['allOf', 'anyOf', 'oneOf']) {
    const branches = value[keyword];
    if (branches === undefined) {
      continue;
    }
    if (!Array.isArray(branches) || branches.length === 0) {
      throw new Error(`${path}.${keyword} 必须是非空 Schema 数组`);
    }
    branches.forEach((branch, index) =>
      assertSchemaNode(branch, `${path}.${keyword}[${index}]`, depth + 1),
    );
  }

  for (const keyword of [
    'not',
    'if',
    'then',
    'else',
    'additionalProperties',
    'additionalItems',
    'contains',
    'contentSchema',
    'propertyNames',
    'unevaluatedProperties',
    'unevaluatedItems',
  ]) {
    if (value[keyword] !== undefined) {
      assertSchemaNode(value[keyword], `${path}.${keyword}`, depth + 1);
    }
  }

  if (value.dependencies !== undefined) {
    if (!isObject(value.dependencies)) {
      throw new Error(`${path}.dependencies 必须是对象`);
    }
    Object.entries(value.dependencies).forEach(([key, dependency]) => {
      if (
        Array.isArray(dependency) &&
        dependency.every(item => typeof item === 'string') &&
        new Set(dependency).size === dependency.length
      ) {
        return;
      }
      assertSchemaNode(dependency, `${path}.dependencies.${key}`, depth + 1);
    });
  }

  if (value.dependentRequired !== undefined) {
    if (!isObject(value.dependentRequired)) {
      throw new Error(`${path}.dependentRequired 必须是对象`);
    }
    Object.entries(value.dependentRequired).forEach(([key, dependencies]) => {
      if (
        !Array.isArray(dependencies) ||
        dependencies.some(item => typeof item !== 'string') ||
        new Set(dependencies).size !== dependencies.length
      ) {
        throw new Error(
          `${path}.dependentRequired.${key} 必须是不重复的字符串数组`,
        );
      }
    });
  }

  for (const keyword of ['$ref', '$dynamicRef']) {
    const reference = value[keyword];
    if (reference === undefined) {
      continue;
    }
    if (typeof reference !== 'string') {
      throw new Error(`${path}.${keyword} 必须是字符串`);
    }
    if (!reference.startsWith('#')) {
      throw new Error(`${path}.${keyword} 仅支持当前结构内的本地引用`);
    }
  }
};

export const DEFAULT_CODE_PLUGIN_SCHEMA = JSON.stringify(
  { type: 'object', properties: {} },
  null,
  2,
);

export const normalizeCodePluginSchema = (value: string) => {
  if (new TextEncoder().encode(value).byteLength > MAX_SCHEMA_BYTES) {
    throw new Error('参数结构不能超过 64KB');
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch (error) {
    void error;
    throw new Error('参数结构必须是有效 JSON');
  }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('参数结构必须是 JSON 对象');
  }

  const schema = parsed as Record<string, unknown>;
  if (schema.type !== 'object') {
    throw new Error('参数结构的 type 必须是 object');
  }
  if (
    schema.properties !== undefined &&
    (!schema.properties ||
      Array.isArray(schema.properties) ||
      typeof schema.properties !== 'object')
  ) {
    throw new Error('参数结构的 properties 必须是对象');
  }
  assertSchemaNode(schema, '$', 0);

  return JSON.stringify(schema, null, 2);
};
