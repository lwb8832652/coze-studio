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

const readContract = () => readFileSync(configThriftPath, 'utf8');

describe('admin object storage api contract source', () => {
  it('declares all object storage management methods', () => {
    const source = readContract();
    for (const method of [
      'ListObjectStorageConfigs',
      'CreateObjectStorageConfig',
      'UpdateObjectStorageConfig',
      'TestObjectStorageConfig',
      'ActivateObjectStorageConfig',
      'DeleteObjectStorageConfig',
    ]) {
      expect(source).toContain(`${method}(`);
    }
  });

  it('declares exact object storage management routes', () => {
    const source = readContract();
    for (const route of [
      "api.get='/api/admin/config/object-storage/list'",
      "api.post='/api/admin/config/object-storage/create'",
      "api.post='/api/admin/config/object-storage/update'",
      "api.post='/api/admin/config/object-storage/test'",
      "api.post='/api/admin/config/object-storage/activate'",
      "api.post='/api/admin/config/object-storage/delete'",
    ]) {
      expect(source).toContain(route);
    }
  });

  it('uses the standard ConfigService response envelope', () => {
    const source = readContract();
    for (const response of [
      'ListObjectStorageConfigsResp',
      'CreateObjectStorageConfigResp',
      'UpdateObjectStorageConfigResp',
      'TestObjectStorageConfigResp',
      'ActivateObjectStorageConfigResp',
      'DeleteObjectStorageConfigResp',
    ]) {
      const body = source.match(
        new RegExp(`struct\\s+${response}\\s*\\{(?<body>[\\s\\S]*?)\\n\\}`),
      )?.groups?.body;
      expect(body).toBeDefined();
      expect(body).toContain('253: required i64 code');
      expect(body).toContain('254: required string msg');
      expect(body).toContain(
        '255: required base.BaseResp BaseResp (api.none="true")',
      );
    }
  });

  it('models credentials as write-only values', () => {
    const source = readContract();
    const view = source.match(
      /struct\s+ObjectStorageConfigView\s*\{(?<body>[\s\S]*?)\n\}/,
    )?.groups?.body;
    expect(view).toBeDefined();
    expect(view).toContain('credential_configured');
    expect(view).not.toMatch(/\baccess_key_id\b/);
    expect(view).not.toMatch(/\bsecret_access_key\b/);
    expect(source).toContain('struct ObjectStorageCredentialInput');
  });

  it('declares the seven supported providers and restart state fields', () => {
    const source = readContract();
    for (const provider of [
      'QINIU',
      'ALIYUN_OSS',
      'TENCENT_COS',
      'HUAWEI_OBS',
      'AWS_S3',
      'MINIO',
      'TOS',
    ]) {
      expect(source).toContain(provider);
    }
    expect(source).toContain('desired_active');
    expect(source).toContain('runtime_active');
    expect(source).toContain('restart_required');
    expect(source).toContain('migration_confirmed');
  });
});
