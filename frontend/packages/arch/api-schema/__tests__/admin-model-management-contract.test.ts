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

import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

const configThriftPath = new URL(
  '../../../../../idl/admin/config.thrift',
  import.meta.url,
);

const readContract = () =>
  // eslint-disable-next-line security/detect-non-literal-fs-filename -- Static fixture path.
  readFileSync(configThriftPath, 'utf8');
describe('admin model management api contract source', () => {
  it('declares the complete production model management methods', () => {
    const source = readContract();
    const requiredMethods = [
      'ListModelProviders',
      'ListModels',
      'GetModelDetail',
      'CreateModel',
      'UpdateModel',
      'TestModelEndpoint',
      'UpdateModelStatus',
      'UpdateModelSort',
      'GetModelGrants',
      'SaveModelGrants',
      'DeleteModel',
    ];

    for (const method of requiredMethods) {
      expect(source).toContain(`${method}(`);
    }
  });

  it('models endpoint secrets as write-only values', () => {
    const source = readContract();
    const endpointView = source.match(
      /struct\s+ModelEndpointView\s*\{(?<body>[\s\S]*?)\n\}/,
    )?.groups?.body;

    expect(endpointView).toBeDefined();
    expect(endpointView).toContain('has_api_key');
    expect(endpointView).not.toMatch(/\bapi_key\b/);
    expect(endpointView).not.toContain('api_key_envelope');
    expect(endpointView).not.toContain('fingerprint');
  });

  it('uses Coze workspace and user grants instead of global role groups', () => {
    const source = readContract();
    const subjectType = source.match(
      /enum\s+ModelGrantSubjectType\s*\{(?<body>[\s\S]*?)\n\}/,
    )?.groups?.body;

    expect(subjectType).toBeDefined();
    expect(subjectType).toContain('WORKSPACE');
    expect(subjectType).toContain('USER');
    expect(subjectType).not.toContain('ROLE');
    expect(subjectType).not.toContain('USER_GROUP');
  });

  it('declares bounded dependency summaries for protected deletion', () => {
    const source = readContract();

    expect(source).toContain('struct ModelDependencySummary');
    expect(source).toContain('struct ModelDependencySample');
    expect(source).toContain('list<ModelDependencySummary> dependencies');
  });

  it('exposes only safe legacy runtime provider options', () => {
    const source = readContract();
    const options = source.match(
      /struct\s+ModelProviderOptions\s*\{(?<body>[\s\S]*?)\n\s*\}/,
    )?.groups?.body;

    expect(options).toBeDefined();
    expect(options).toContain('ark_region');
    expect(options).toContain('openai_by_azure');
    expect(options).toContain('openai_api_version');
    expect(options).toContain('gemini_backend');
    expect(options).toContain('gemini_project');
    expect(options).toContain('gemini_location');
    expect(options).not.toMatch(/api_key|secret|access_key|header/i);
    expect(source).toMatch(
      /struct\s+ModelManagementInput[\s\S]*optional\s+ModelProviderOptions\s+provider_options/,
    );
    expect(source).toMatch(
      /struct\s+ModelDetail[\s\S]*optional\s+ModelProviderOptions\s+provider_options/,
    );
  });
});
