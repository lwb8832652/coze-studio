# Object Storage Control Plane Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a system-admin object storage control plane that stores multiple encrypted provider configurations, activates one desired primary configuration, and loads that configuration at server startup.

**Architecture:** Keep the existing `storage.Storage` contract stable and add a storage configuration domain plus a database-backed runtime loader above provider adapters. The admin API is added to `idl/admin/config.thrift`, backend handlers delegate to an application service, and the frontend adds a System Admin "对象存储" section using generated API types. Runtime switching is restart-only: admin activation changes the desired primary row, while the current process keeps using the startup descriptor.

**Tech Stack:** Go, Hertz, GORM, Atlas migrations, AES-GCM via `backend/pkg/secureaead`, official Go SDKs for Qiniu Kodo/Ali OSS/Tencent COS/Huawei OBS/AWS S3/MinIO/TOS, Thrift IDL, idl2ts, React, TypeScript, Rush, Vitest, Coze Design.

---

## Scope Check

This plan keeps the database model, provider adapters, admin API, startup loader, and UI in one end-to-end feature because none of those pieces is independently useful for the user request. Each task produces testable software and commits frequently so review can stop at a stable checkpoint.

## File Structure

### New Backend Domain And Application Files

- Create `backend/domain/storageconfig/types.go`: provider enum, health enum, credential value type, config entity, runtime descriptor.
- Create `backend/domain/storageconfig/validation.go`: schema validation, endpoint normalization, credential pair validation, runtime revision comparison.
- Create `backend/domain/storageconfig/errors.go`: stable domain errors and `ErrorCodeOf`.
- Create `backend/domain/storageconfig/types_test.go`: validation matrix for seven providers and credential pair rules.
- Create `backend/application/objectstorage/service.go`: CRUD, draft test, activation, deletion, runtime status projection.
- Create `backend/application/objectstorage/bootstrap.go`: first environment import and rescue mode orchestration.
- Create `backend/application/objectstorage/service_test.go`: fake repository and checker tests for mutations, activation rollback, and redaction.

### New Backend Infra Files

- Create `backend/infra/storage/config/credential_codec.go`: single-key AES-GCM envelope for object storage credentials.
- Create `backend/infra/storage/config/credential_codec_test.go`: round trip, nonce, AAD, tamper, wrong key, plaintext scan.
- Create `backend/infra/storage/config/env.go`: parse `OBJECT_STORAGE_CONFIG_SOURCE`, `OBJECT_STORAGE_CREDENTIAL_KEY`, and legacy provider env vars.
- Create `backend/infra/storage/config/mysql_models.go`: GORM PO for `object_storage_configs`.
- Create `backend/infra/storage/config/mysql_repository.go`: CRUD, health update, activation transaction, deletion guard, list active.
- Create `backend/infra/storage/config/mysql_repository_test.go`: SQLite-backed repository tests plus SQL uniqueness checks.
- Create `backend/infra/storage/impl/registry.go`: provider registry and `NewFromConfig`.
- Create `backend/infra/storage/impl/runtime.go`: startup `RuntimeDescriptor`, current descriptor storage, runtime comparison helpers.
- Create `backend/infra/storage/impl/storage_imagex.go`: generic storage-backed ImageX wrapper.
- Create provider packages:
  - `backend/infra/storage/impl/qiniu/qiniu.go`
  - `backend/infra/storage/impl/aliyunoss/aliyunoss.go`
  - `backend/infra/storage/impl/tencentcos/tencentcos.go`
  - `backend/infra/storage/impl/huaweiobs/huaweiobs.go`
- Modify existing provider packages:
  - `backend/infra/storage/impl/minio/minio.go`
  - `backend/infra/storage/impl/s3/s3.go`
  - `backend/infra/storage/impl/tos/tos.go`
  - `backend/infra/storage/impl/minio/minio_imagex.go`
  - `backend/infra/storage/impl/s3/s3_imagex.go`
  - `backend/infra/storage/impl/tos/tos_imagex.go`
- Create shared provider tests:
  - `backend/infra/storage/impl/internal/contract/storage_contract.go`
  - `backend/infra/storage/impl/internal/contract/readiness_contract.go`
  - `backend/infra/storage/impl/registry_test.go`

### API, Migration, Frontend, Docs Files

- Create migration `docker/atlas/migrations/20260728000100_object_storage_configs.sql`.
- Modify `docker/atlas/migrations/atlas.sum` after Atlas hash.
- Modify `idl/admin/config.thrift`: add object storage structs and six RPCs.
- Regenerate `backend/api/model/admin/config/config.go`, `backend/api/router/coze/api.go`, and related generated router files.
- Create `backend/api/handler/coze/config_service_object_storage.go`.
- Create `backend/api/handler/coze/config_service_object_storage_test.go`.
- Modify `frontend/packages/arch/api-schema/api.config.js`: add `adminConfig` entry.
- Modify `frontend/packages/arch/api-schema/package.json`, `frontend/packages/arch/api-schema/src/index.ts`, generated `frontend/packages/arch/api-schema/src/idl/admin/config.ts`.
- Create `frontend/packages/arch/api-schema/__tests__/admin-object-storage-contract.test.ts`.
- Modify `frontend/apps/coze-studio/src/pages/system/content.ts` and `frontend/apps/coze-studio/src/pages/system/index.tsx`.
- Create `frontend/apps/coze-studio/src/pages/system/object-storage-section.tsx`.
- Create `frontend/apps/coze-studio/src/pages/system/object-storage-form.tsx`.
- Create `frontend/apps/coze-studio/src/pages/system/object-storage-view-model.ts`.
- Create `frontend/apps/coze-studio/src/pages/system/__tests__/object-storage-section.test.tsx`.
- Create `frontend/apps/coze-studio/src/pages/system/__tests__/object-storage-form.test.tsx`.
- Create `frontend/apps/coze-studio/src/pages/system/__tests__/object-storage-view-model.test.ts`.
- Modify `frontend/apps/coze-studio/src/pages/system/service.ts`.
- Modify Docker compose files that define `coze-server` env:
  - `docker/docker-compose.yml`
  - `docker/docker-compose-debug.yml`
  - `docker/docker-compose-oceanbase.yml`
  - `docker/docker-compose-oceanbase_debug.yml`
- Create `docs/superpowers/runbooks/object-storage-control-plane-operations.md`.
- Modify `docs/superpowers/context/project-context.md` with the new persistent/security/runtime fact.

### Official SDK References

Use these module paths unless `go get` reports a direct replacement from the same official repository:

- Qiniu Kodo: `github.com/qiniu/go-sdk/v7`
- Ali OSS v2: `github.com/aliyun/alibabacloud-oss-go-sdk-v2`
- Tencent COS v5: `github.com/tencentyun/cos-go-sdk-v5`
- Huawei OBS: `github.com/huaweicloud/huaweicloud-sdk-go-obs`
- AWS S3 v2: existing `github.com/aws/aws-sdk-go-v2`
- MinIO v7: existing `github.com/minio/minio-go/v7`
- Volcengine TOS v2: existing `github.com/volcengine/ve-tos-golang-sdk/v2`

---

### Task 1: Database Migration And IDL Contract Guard

**Files:**
- Create: `docker/atlas/migrations/20260728000100_object_storage_configs.sql`
- Modify: `idl/admin/config.thrift`
- Create: `frontend/packages/arch/api-schema/__tests__/admin-object-storage-contract.test.ts`

- [ ] **Step 1: Write the contract test before changing IDL**

```ts
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
```

- [ ] **Step 2: Run the contract test and confirm failure**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/frontend/packages/arch/api-schema
node ../../../../common/scripts/install-run-rushx.js test __tests__/admin-object-storage-contract.test.ts
```

Expected: FAIL because `ObjectStorageConfigView` and object storage RPC names do not exist in `idl/admin/config.thrift`.

- [ ] **Step 3: Add the Atlas migration**

```sql
CREATE TABLE `object_storage_configs` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `name` VARCHAR(128) NOT NULL,
  `provider_type` VARCHAR(32) NOT NULL,
  `config_json` JSON NOT NULL,
  `credential_secret` TEXT NOT NULL,
  `active_slot` TINYINT UNSIGNED NULL,
  `health_status` VARCHAR(32) NOT NULL DEFAULT 'unknown',
  `last_health_code` VARCHAR(64) NOT NULL DEFAULT '',
  `last_health_message` VARCHAR(255) NOT NULL DEFAULT '',
  `last_health_latency_ms` INT UNSIGNED NOT NULL DEFAULT 0,
  `last_health_at` DATETIME(3) NULL,
  `version` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `runtime_revision` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `created_at` DATETIME(3) NOT NULL,
  `updated_at` DATETIME(3) NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_object_storage_configs_name` (`name`),
  UNIQUE KEY `uk_object_storage_configs_active_slot` (`active_slot`),
  KEY `idx_object_storage_configs_provider_type` (`provider_type`),
  CONSTRAINT `ck_object_storage_configs_active_slot`
    CHECK (`active_slot` IS NULL OR `active_slot` = 1)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

- [ ] **Step 4: Add IDL structs and RPCs**

Add these enum and struct names to `idl/admin/config.thrift` before `service ConfigService`:

```thrift
enum ObjectStorageProviderType {
    QINIU = 1
    ALIYUN_OSS = 2
    TENCENT_COS = 3
    HUAWEI_OBS = 4
    AWS_S3 = 5
    MINIO = 6
    TOS = 7
}

enum ObjectStorageHealthStatus {
    UNKNOWN = 1
    HEALTHY = 2
    UNHEALTHY = 3
}

enum ObjectStorageRuntimeSource {
    DATABASE = 1
    ENV_RESCUE = 2
}

struct ObjectStoragePublicConfig {
    1: optional string bucket
    2: optional string region
    3: optional string endpoint
    4: optional string endpoint_override
    5: optional bool force_path_style
    6: optional bool use_ssl
    7: optional string download_domain
    8: optional bool use_https
}

struct ObjectStorageCredentialInput {
    1: optional string access_key_id
    2: optional string secret_access_key
}

struct ObjectStorageHealthView {
    1: ObjectStorageHealthStatus status
    2: optional string code
    3: optional string message
    4: optional i64 latency_ms
    5: optional string checked_at
}

struct ObjectStorageConfigView {
    1: i64 id (api.js_conv='true', agw.js_conv='str')
    2: string name
    3: ObjectStorageProviderType provider_type
    4: ObjectStoragePublicConfig config
    5: bool credential_configured
    6: ObjectStorageHealthView health
    7: bool desired_active
    8: bool runtime_active
    9: bool restart_required
    10: i64 version (api.js_conv='true', agw.js_conv='str')
    11: i64 runtime_revision (api.js_conv='true', agw.js_conv='str')
    12: string created_at
    13: string updated_at
}

struct ListObjectStorageConfigsReq {}
struct ListObjectStorageConfigsResp {
    1: list<ObjectStorageConfigView> configs
    2: ObjectStorageRuntimeSource runtime_source
    3: bool restart_required
}

struct CreateObjectStorageConfigReq {
    1: string name
    2: ObjectStorageProviderType provider_type
    3: ObjectStoragePublicConfig config
    4: ObjectStorageCredentialInput credential
}
struct CreateObjectStorageConfigResp { 1: ObjectStorageConfigView config }

struct UpdateObjectStorageConfigReq {
    1: i64 id (api.js_conv='true', agw.js_conv='str')
    2: i64 expected_version (api.js_conv='true', agw.js_conv='str')
    3: string name
    4: ObjectStoragePublicConfig config
    5: optional ObjectStorageCredentialInput credential
}
struct UpdateObjectStorageConfigResp { 1: ObjectStorageConfigView config }

struct TestObjectStorageConfigReq {
    1: optional i64 id (api.js_conv='true', agw.js_conv='str')
    2: optional i64 expected_version (api.js_conv='true', agw.js_conv='str')
    3: ObjectStorageProviderType provider_type
    4: ObjectStoragePublicConfig config
    5: optional ObjectStorageCredentialInput credential
}
struct TestObjectStorageConfigResp {
    1: bool success
    2: ObjectStorageHealthView health
}

struct ActivateObjectStorageConfigReq {
    1: i64 id (api.js_conv='true', agw.js_conv='str')
    2: i64 expected_version (api.js_conv='true', agw.js_conv='str')
    3: bool migration_confirmed
}
struct ActivateObjectStorageConfigResp { 1: ObjectStorageConfigView config }

struct DeleteObjectStorageConfigReq {
    1: i64 id (api.js_conv='true', agw.js_conv='str')
    2: i64 expected_version (api.js_conv='true', agw.js_conv='str')
}
struct DeleteObjectStorageConfigResp {}
```

Add these service methods:

```thrift
    ListObjectStorageConfigsResp ListObjectStorageConfigs(1:ListObjectStorageConfigsReq req)(api.get='/api/admin/config/object-storage/list', api.category="admin")
    CreateObjectStorageConfigResp CreateObjectStorageConfig(1:CreateObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/create', api.category="admin")
    UpdateObjectStorageConfigResp UpdateObjectStorageConfig(1:UpdateObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/update', api.category="admin")
    TestObjectStorageConfigResp TestObjectStorageConfig(1:TestObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/test', api.category="admin")
    ActivateObjectStorageConfigResp ActivateObjectStorageConfig(1:ActivateObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/activate', api.category="admin")
    DeleteObjectStorageConfigResp DeleteObjectStorageConfig(1:DeleteObjectStorageConfigReq req)(api.post='/api/admin/config/object-storage/delete', api.category="admin")
```

- [ ] **Step 5: Hash the migration**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:0.35.0-community-alpine migrate hash --dir file:///migrations
```

Expected: `atlas.sum` is updated and `git diff -- docker/atlas/migrations/atlas.sum` contains the new migration hash.

- [ ] **Step 6: Run contract test and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/frontend/packages/arch/api-schema
node ../../../../common/scripts/install-run-rushx.js test __tests__/admin-object-storage-contract.test.ts
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add docker/atlas/migrations/20260728000100_object_storage_configs.sql docker/atlas/migrations/atlas.sum idl/admin/config.thrift frontend/packages/arch/api-schema/__tests__/admin-object-storage-contract.test.ts
git commit -m "feat: add object storage config contract"
```

---

### Task 2: Domain Types And Validation

**Files:**
- Create: `backend/domain/storageconfig/types.go`
- Create: `backend/domain/storageconfig/validation.go`
- Create: `backend/domain/storageconfig/errors.go`
- Create: `backend/domain/storageconfig/types_test.go`

- [ ] **Step 1: Write provider validation tests**

```go
package storageconfig

import (
	"errors"
	"testing"
)

func TestValidateConfigAcceptsSupportedProviders(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderType
		config   PublicConfig
	}{
		{name: "qiniu", provider: ProviderQiniu, config: PublicConfig{Bucket: "coze", DownloadDomain: "cdn.example.com", UseHTTPS: true}},
		{name: "aliyun", provider: ProviderAliyunOSS, config: PublicConfig{Bucket: "coze", Region: "cn-hangzhou"}},
		{name: "tencent", provider: ProviderTencentCOS, config: PublicConfig{Bucket: "coze-1250000000", Region: "ap-guangzhou"}},
		{name: "huawei", provider: ProviderHuaweiOBS, config: PublicConfig{Bucket: "coze", Region: "cn-north-4", Endpoint: "https://obs.cn-north-4.myhuaweicloud.com"}},
		{name: "aws", provider: ProviderAWSS3, config: PublicConfig{Bucket: "coze", Region: "us-east-1", ForcePathStyle: true}},
		{name: "minio", provider: ProviderMinIO, config: PublicConfig{Bucket: "coze", Endpoint: "minio:9000", UseSSL: false}},
		{name: "tos", provider: ProviderTOS, config: PublicConfig{Bucket: "coze", Region: "cn-beijing", Endpoint: "https://tos-cn-beijing.volces.com"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			normalized, err := ValidatePublicConfig(test.provider, test.config, ValidationMode{AllowHTTP: true})
			if err != nil {
				t.Fatalf("ValidatePublicConfig() error = %v", err)
			}
			if normalized.Bucket == "" {
				t.Fatal("normalized bucket is empty")
			}
		})
	}
}

func TestValidateConfigRejectsProviderSpecificInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		provider ProviderType
		config   PublicConfig
	}{
		{name: "qiniu domain with scheme", provider: ProviderQiniu, config: PublicConfig{Bucket: "coze", DownloadDomain: "https://cdn.example.com"}},
		{name: "tencent bucket missing appid", provider: ProviderTencentCOS, config: PublicConfig{Bucket: "coze", Region: "ap-guangzhou"}},
		{name: "aws http endpoint outside debug", provider: ProviderAWSS3, config: PublicConfig{Bucket: "coze", Region: "us-east-1", EndpointOverride: "http://s3.example.com"}},
		{name: "minio endpoint empty", provider: ProviderMinIO, config: PublicConfig{Bucket: "coze"}},
		{name: "tos region empty", provider: ProviderTOS, config: PublicConfig{Bucket: "coze", Endpoint: "https://tos-cn-beijing.volces.com"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidatePublicConfig(test.provider, test.config, ValidationMode{})
			if !errors.Is(err, ErrConfigInvalid) {
				t.Fatalf("ValidatePublicConfig() error = %v, want ErrConfigInvalid", err)
			}
		})
	}
}

func TestValidateCredentialsRequiresPair(t *testing.T) {
	if err := ValidateCredentialInput(CredentialInput{AccessKeyID: "ak"}); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("single AK error = %v", err)
	}
	if err := ValidateCredentialInput(CredentialInput{SecretAccessKey: "sk"}); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("single SK error = %v", err)
	}
	if err := ValidateCredentialInput(CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}); err != nil {
		t.Fatalf("pair error = %v", err)
	}
}
```

- [ ] **Step 2: Run validation tests and confirm failure**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./domain/storageconfig
```

Expected: FAIL because package `domain/storageconfig` does not exist.

- [ ] **Step 3: Add domain errors**

```go
package storageconfig

import "errors"

var (
	ErrConfigInvalid          = errors.New("object storage config invalid")
	ErrNotFound               = errors.New("object storage config not found")
	ErrVersionConflict        = errors.New("object storage version conflict")
	ErrConnectionFailed       = errors.New("object storage connection failed")
	ErrActiveDeleteForbidden  = errors.New("object storage active delete forbidden")
	ErrCredentialUnavailable  = errors.New("object storage credential unavailable")
	ErrProviderUnsupported    = errors.New("object storage provider unsupported")
	ErrPrimaryConfigMissing   = errors.New("object storage primary config missing")
	ErrMigrationConfirmation  = errors.New("object storage migration confirmation required")
)

func ErrorCodeOf(err error) string {
	switch {
	case errors.Is(err, ErrConfigInvalid):
		return "OBJECT_STORAGE_CONFIG_INVALID"
	case errors.Is(err, ErrNotFound):
		return "OBJECT_STORAGE_NOT_FOUND"
	case errors.Is(err, ErrVersionConflict):
		return "OBJECT_STORAGE_VERSION_CONFLICT"
	case errors.Is(err, ErrConnectionFailed):
		return "OBJECT_STORAGE_CONNECTION_FAILED"
	case errors.Is(err, ErrActiveDeleteForbidden):
		return "OBJECT_STORAGE_ACTIVE_DELETE_FORBIDDEN"
	case errors.Is(err, ErrCredentialUnavailable):
		return "OBJECT_STORAGE_CREDENTIAL_UNAVAILABLE"
	case errors.Is(err, ErrProviderUnsupported):
		return "OBJECT_STORAGE_PROVIDER_UNSUPPORTED"
	case errors.Is(err, ErrMigrationConfirmation):
		return "OBJECT_STORAGE_MIGRATION_CONFIRMATION_REQUIRED"
	default:
		return "OBJECT_STORAGE_INTERNAL"
	}
}
```

- [ ] **Step 4: Add domain types**

```go
package storageconfig

import "time"

type ProviderType string

const (
	ProviderQiniu      ProviderType = "qiniu"
	ProviderAliyunOSS  ProviderType = "aliyun_oss"
	ProviderTencentCOS ProviderType = "tencent_cos"
	ProviderHuaweiOBS  ProviderType = "huawei_obs"
	ProviderAWSS3      ProviderType = "aws_s3"
	ProviderMinIO      ProviderType = "minio"
	ProviderTOS        ProviderType = "tos"
)

type HealthStatus string

const (
	HealthUnknown   HealthStatus = "unknown"
	HealthHealthy   HealthStatus = "healthy"
	HealthUnhealthy HealthStatus = "unhealthy"
)

type RuntimeSource string

const (
	RuntimeSourceDatabase  RuntimeSource = "database"
	RuntimeSourceEnvRescue RuntimeSource = "env_rescue"
)

type PublicConfig struct {
	Bucket           string `json:"bucket,omitempty"`
	Region           string `json:"region,omitempty"`
	Endpoint         string `json:"endpoint,omitempty"`
	EndpointOverride string `json:"endpoint_override,omitempty"`
	ForcePathStyle   bool   `json:"force_path_style,omitempty"`
	UseSSL           bool   `json:"use_ssl,omitempty"`
	DownloadDomain   string `json:"download_domain,omitempty"`
	UseHTTPS         bool   `json:"use_https,omitempty"`
}

type CredentialInput struct {
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
}

type Health struct {
	Status    HealthStatus
	Code      string
	Message   string
	LatencyMS uint32
	CheckedAt *time.Time
}

type Config struct {
	ID                uint64
	Name              string
	ProviderType      ProviderType
	PublicConfig      PublicConfig
	CredentialSecret  string
	Active            bool
	Health            Health
	Version           uint64
	RuntimeRevision   uint64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RuntimeDescriptor struct {
	Source          RuntimeSource
	ConfigID        uint64
	RuntimeRevision uint64
	ProviderType    ProviderType
}

type ValidationMode struct {
	AllowHTTP bool
}
```

- [ ] **Step 5: Add validation functions**

Implement functions with these signatures in `validation.go`:

```go
func (p ProviderType) Valid() bool
func NormalizeName(name string) (string, error)
func ValidateProvider(provider ProviderType) error
func ValidatePublicConfig(provider ProviderType, input PublicConfig, mode ValidationMode) (PublicConfig, error)
func ValidateCredentialInput(input CredentialInput) error
func HasCredentialPair(input CredentialInput) bool
func RuntimeFieldsEqual(left, right Config) bool
func RestartRequired(runtime RuntimeDescriptor, desired *Config) bool
```

Required behavior:

- trim name, bucket, region, endpoint, endpoint override, download domain, AK, SK.
- reject empty names and names longer than 128 runes.
- reject bucket values containing `/`, `\`, NUL, CR, or LF.
- reject unknown provider strings with `ErrProviderUnsupported`.
- reject custom HTTP endpoints unless `ValidationMode.AllowHTTP` is true.
- normalize MinIO `http://minio:9000` to `Endpoint=minio:9000` and `UseSSL=false`.
- normalize MinIO `https://minio.example.com` to `Endpoint=minio.example.com` and `UseSSL=true`.
- require Qiniu `download_domain` without scheme and without path.
- require Tencent bucket to match `name-appid` where `appid` is all digits.
- require Huawei and TOS `endpoint` with `https://` unless debug mode allows HTTP.
- treat empty `CredentialInput{}` as valid only for edit/test paths that explicitly allow preserved credentials; creation calls must call `HasCredentialPair`.

- [ ] **Step 6: Run validation tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./domain/storageconfig
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/domain/storageconfig
git commit -m "feat: add object storage config domain"
```

---

### Task 3: Credential Codec

**Files:**
- Create: `backend/infra/storage/config/credential_codec.go`
- Create: `backend/infra/storage/config/credential_codec_test.go`

- [ ] **Step 1: Write codec tests**

```go
package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
)

func TestCredentialCodecRoundTripAndDoesNotStorePlaintext(t *testing.T) {
	codec := testCredentialCodec(t, bytes.NewReader(bytes.Repeat([]byte{9}, 24)))
	plain := domain.CredentialInput{AccessKeyID: "ak-live-marker", SecretAccessKey: "sk-live-marker"}
	envelope, err := codec.Encrypt(42, domain.ProviderMinIO, 1, plain)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	if strings.Contains(envelope, "ak-live-marker") || strings.Contains(envelope, "sk-live-marker") {
		t.Fatal("envelope leaked plaintext credential")
	}
	decoded, err := codec.Decrypt(42, domain.ProviderMinIO, 1, envelope)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if decoded != plain {
		t.Fatalf("decoded credential = %+v", decoded)
	}
}

func TestCredentialCodecUsesFreshNonce(t *testing.T) {
	codec := testCredentialCodec(t, bytes.NewReader(bytes.Repeat([]byte{1}, 24)))
	plain := domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"}
	first, err := codec.Encrypt(7, domain.ProviderAWSS3, 1, plain)
	if err != nil {
		t.Fatalf("first Encrypt() error = %v", err)
	}
	second, err := codec.Encrypt(7, domain.ProviderAWSS3, 1, plain)
	if err != nil {
		t.Fatalf("second Encrypt() error = %v", err)
	}
	if first == second {
		t.Fatal("two envelopes used the same nonce")
	}
}

func TestCredentialCodecBindsAAD(t *testing.T) {
	codec := testCredentialCodec(t, bytes.NewReader(bytes.Repeat([]byte{5}, 24)))
	envelope, err := codec.Encrypt(1, domain.ProviderQiniu, 1, domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"})
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	for _, test := range []struct {
		id       uint64
		provider domain.ProviderType
		version  uint64
	}{
		{id: 2, provider: domain.ProviderQiniu, version: 1},
		{id: 1, provider: domain.ProviderMinIO, version: 1},
		{id: 1, provider: domain.ProviderQiniu, version: 2},
	} {
		if _, err = codec.Decrypt(test.id, test.provider, test.version, envelope); !errors.Is(err, domain.ErrCredentialUnavailable) {
			t.Fatalf("Decrypt(%+v) error = %v", test, err)
		}
	}
}

func TestParseCredentialKeyRequiresBase64ThirtyTwoBytes(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{3}, 32))
	decoded, err := ParseCredentialKey(key)
	if err != nil {
		t.Fatalf("ParseCredentialKey(valid) error = %v", err)
	}
	if len(decoded) != 32 {
		t.Fatalf("decoded length = %d", len(decoded))
	}
	if _, err = ParseCredentialKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31))); !errors.Is(err, domain.ErrCredentialUnavailable) {
		t.Fatalf("short key error = %v", err)
	}
}

func testCredentialCodec(t *testing.T, nonceSource *bytes.Reader) *CredentialCodec {
	t.Helper()
	key := bytes.Repeat([]byte{8}, 32)
	codec, err := NewCredentialCodec(key, nonceSource)
	if err != nil {
		t.Fatalf("NewCredentialCodec() error = %v", err)
	}
	return codec
}
```

- [ ] **Step 2: Run codec tests and confirm failure**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./infra/storage/config -run CredentialCodec
```

Expected: FAIL because `infra/storage/config` does not exist.

- [ ] **Step 3: Add codec implementation**

Required public API:

```go
package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"sync"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/pkg/secureaead"
)

const (
	ObjectStorageCredentialKeyEnv = "OBJECT_STORAGE_CREDENTIAL_KEY"
	credentialEnvelopeVersion     = "v1"
)

type CredentialCodec struct {
	key         []byte
	nonce      io.Reader
	nonceMutex sync.Mutex
}

type credentialEnvelope struct {
	Version    string `json:"version"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

func LoadCredentialCodec(getenv func(string) string) (*CredentialCodec, error)
func ParseCredentialKey(encoded string) ([]byte, error)
func NewCredentialCodec(key []byte, nonceSource io.Reader) (*CredentialCodec, error)
func (c *CredentialCodec) Encrypt(id uint64, provider domain.ProviderType, version uint64, input domain.CredentialInput) (string, error)
func (c *CredentialCodec) Decrypt(id uint64, provider domain.ProviderType, version uint64, envelope string) (domain.CredentialInput, error)
```

Implementation rules:

- `LoadCredentialCodec` reads `OBJECT_STORAGE_CREDENTIAL_KEY` through the supplied `getenv`.
- `ParseCredentialKey` requires canonical standard Base64 and exactly 32 decoded bytes.
- `NewCredentialCodec` copies the key and defaults nil `nonceSource` to `crypto/rand.Reader`.
- `Encrypt` validates a complete credential pair and serializes only `access_key_id` plus `secret_access_key`.
- `Decrypt` rejects malformed JSON, unknown envelope version, invalid Base64, wrong AAD, and wrong key as `domain.ErrCredentialUnavailable`.
- AAD is `object-storage-credential/v1:<id>:<provider>:<version>`.
- Envelope JSON is compact and contains only `version`, `nonce`, and `ciphertext`.
- Error strings must not contain AK, SK, nonce, ciphertext, or raw envelope.

- [ ] **Step 4: Run codec tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./infra/storage/config -run CredentialCodec
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/infra/storage/config/credential_codec.go backend/infra/storage/config/credential_codec_test.go
git commit -m "feat: encrypt object storage credentials"
```

---

### Task 4: MySQL Repository

**Files:**
- Create: `backend/infra/storage/config/mysql_models.go`
- Create: `backend/infra/storage/config/mysql_repository.go`
- Create: `backend/infra/storage/config/mysql_repository_test.go`

- [ ] **Step 1: Write repository tests**

```go
package config

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestMySQLRepositoryCreateListAndActiveUniqueness(t *testing.T) {
	repository, db := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	first := validRepositoryConfig("minio-a")
	created, err := repository.Create(ctx, first)
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	second := validRepositoryConfig("minio-b")
	second.Active = true
	if _, err = repository.Create(ctx, second); err != nil {
		t.Fatalf("Create(second active) error = %v", err)
	}
	list, err := repository.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d", len(list))
	}
	var activeRows int64
	if err = db.Model(&objectStorageConfigPO{}).Where("active_slot = 1").Count(&activeRows).Error; err != nil {
		t.Fatalf("count active error = %v", err)
	}
	if activeRows != 1 || created.ID == 0 {
		t.Fatalf("active rows = %d created id = %d", activeRows, created.ID)
	}
}

func TestMySQLRepositoryOptimisticLockAndRuntimeRevision(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	created, err := repository.Create(ctx, validRepositoryConfig("minio-a"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	updated, err := repository.Update(ctx, UpdateConfigInput{
		ID: created.ID, ExpectedVersion: created.Version, Name: "renamed",
		PublicConfig: created.PublicConfig, CredentialSecret: created.CredentialSecret,
		RuntimeChanged: false,
	})
	if err != nil {
		t.Fatalf("Update(name) error = %v", err)
	}
	if updated.Version != created.Version+1 || updated.RuntimeRevision != created.RuntimeRevision {
		t.Fatalf("version/runtime = %d/%d", updated.Version, updated.RuntimeRevision)
	}
	_, err = repository.Update(ctx, UpdateConfigInput{
		ID: created.ID, ExpectedVersion: created.Version, Name: "stale",
		PublicConfig: created.PublicConfig, CredentialSecret: created.CredentialSecret,
		RuntimeChanged: false,
	})
	if !errors.Is(err, domain.ErrVersionConflict) {
		t.Fatalf("stale update error = %v", err)
	}
}

func TestMySQLRepositoryActivateIsAtomicAndDeleteGuardsActive(t *testing.T) {
	repository, _ := newObjectStorageSQLiteRepository(t)
	ctx := context.Background()
	oldConfig := validRepositoryConfig("old")
	oldConfig.Active = true
	old, err := repository.Create(ctx, oldConfig)
	if err != nil {
		t.Fatalf("Create(old) error = %v", err)
	}
	next, err := repository.Create(ctx, validRepositoryConfig("next"))
	if err != nil {
		t.Fatalf("Create(next) error = %v", err)
	}
	activated, err := repository.Activate(ctx, next.ID, next.Version)
	if err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	if !activated.Active {
		t.Fatal("target is not active")
	}
	if err = repository.Delete(ctx, activated.ID, activated.Version, domain.RuntimeDescriptor{}); !errors.Is(err, domain.ErrActiveDeleteForbidden) {
		t.Fatalf("Delete(active) error = %v", err)
	}
	oldAfter, err := repository.Get(ctx, old.ID)
	if err != nil {
		t.Fatalf("Get(old) error = %v", err)
	}
	if oldAfter.Active {
		t.Fatal("old config is still active")
	}
}

func newObjectStorageSQLiteRepository(t *testing.T) (*MySQLRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite error = %v", err)
	}
	if err = db.AutoMigrate(&objectStorageConfigPO{}); err != nil {
		t.Fatalf("migrate sqlite error = %v", err)
	}
	return NewMySQLRepository(db), db
}

func validRepositoryConfig(name string) domain.Config {
	now := time.Now().UTC()
	return domain.Config{
		Name: name, ProviderType: domain.ProviderMinIO,
		PublicConfig: domain.PublicConfig{Bucket: "coze", Endpoint: "minio:9000", UseSSL: false},
		CredentialSecret: "v1-test-envelope", Health: domain.Health{Status: domain.HealthUnknown},
		Version: 1, RuntimeRevision: 1, CreatedAt: now, UpdatedAt: now,
	}
}
```

- [ ] **Step 2: Run repository tests and confirm failure**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./infra/storage/config -run MySQLRepository
```

Expected: FAIL because repository types do not exist.

- [ ] **Step 3: Add GORM model**

```go
package config

import "time"

type objectStorageConfigPO struct {
	ID                  uint64     `gorm:"column:id;type:bigint unsigned;primaryKey;autoIncrement"`
	Name                string     `gorm:"column:name;size:128;not null;uniqueIndex:uk_object_storage_configs_name"`
	ProviderType        string     `gorm:"column:provider_type;size:32;not null;index:idx_object_storage_configs_provider_type"`
	ConfigJSON          string     `gorm:"column:config_json;type:json;not null"`
	CredentialSecret    string     `gorm:"column:credential_secret;type:text;not null"`
	ActiveSlot          *uint8     `gorm:"column:active_slot;uniqueIndex:uk_object_storage_configs_active_slot"`
	HealthStatus        string     `gorm:"column:health_status;size:32;not null;default:'unknown'"`
	LastHealthCode       string     `gorm:"column:last_health_code;size:64;not null;default:''"`
	LastHealthMessage    string     `gorm:"column:last_health_message;size:255;not null;default:''"`
	LastHealthLatencyMS  uint32     `gorm:"column:last_health_latency_ms;type:int unsigned;not null;default:0"`
	LastHealthAt         *time.Time `gorm:"column:last_health_at"`
	Version             uint64     `gorm:"column:version;type:bigint unsigned;not null;default:1"`
	RuntimeRevision     uint64     `gorm:"column:runtime_revision;type:bigint unsigned;not null;default:1"`
	CreatedAt           time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt           time.Time  `gorm:"column:updated_at;not null"`
}

func (objectStorageConfigPO) TableName() string {
	return "object_storage_configs"
}
```

- [ ] **Step 4: Add repository implementation**

Required public API:

```go
type MySQLRepository struct { db *gorm.DB }

type UpdateConfigInput struct {
	ID               uint64
	ExpectedVersion  uint64
	Name             string
	PublicConfig     domain.PublicConfig
	CredentialSecret string
	RuntimeChanged   bool
}

func NewMySQLRepository(db *gorm.DB) *MySQLRepository
func (r *MySQLRepository) Count(ctx context.Context) (int64, error)
func (r *MySQLRepository) List(ctx context.Context) ([]domain.Config, error)
func (r *MySQLRepository) Get(ctx context.Context, id uint64) (*domain.Config, error)
func (r *MySQLRepository) GetActive(ctx context.Context) (*domain.Config, error)
func (r *MySQLRepository) Create(ctx context.Context, config domain.Config) (*domain.Config, error)
func (r *MySQLRepository) Update(ctx context.Context, input UpdateConfigInput) (*domain.Config, error)
func (r *MySQLRepository) UpdateHealth(ctx context.Context, id uint64, health domain.Health) error
func (r *MySQLRepository) Activate(ctx context.Context, id uint64, expectedVersion uint64) (*domain.Config, error)
func (r *MySQLRepository) Delete(ctx context.Context, id uint64, expectedVersion uint64, runtime domain.RuntimeDescriptor) error
```

Implementation rules:

- Marshal `domain.PublicConfig` with `encoding/json` and reject marshal/unmarshal failures as `domain.ErrConfigInvalid`.
- Map GORM `ErrRecordNotFound` to `domain.ErrNotFound`.
- Map duplicate name and active-slot unique conflicts to stable domain errors.
- `Create` sets `ActiveSlot=&1` only when `domain.Config.Active` is true.
- `Update` requires `id` plus `expected_version`, increments `version`, and increments `runtime_revision` only when `RuntimeChanged` is true.
- `UpdateHealth` updates only health columns and does not increment `version`.
- `Activate` runs in one transaction with `FOR UPDATE`, clears any current `active_slot=1`, sets target to 1, and increments versions of changed rows.
- `Activate` returns unchanged target when it is already active and version matches.
- `Delete` rejects the database active row and any row matching `runtime.ConfigID`.

- [ ] **Step 5: Run repository tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./infra/storage/config -run MySQLRepository
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/infra/storage/config/mysql_models.go backend/infra/storage/config/mysql_repository.go backend/infra/storage/config/mysql_repository_test.go
git commit -m "feat: persist object storage configs"
```

---

### Task 5: Provider Registry And Existing Adapter Refactor

**Files:**
- Create: `backend/infra/storage/impl/registry.go`
- Create: `backend/infra/storage/impl/runtime.go`
- Create: `backend/infra/storage/impl/registry_test.go`
- Modify: `backend/infra/storage/impl/storage.go`
- Modify: `backend/infra/storage/impl/minio/minio.go`
- Modify: `backend/infra/storage/impl/s3/s3.go`
- Modify: `backend/infra/storage/impl/tos/tos.go`

- [ ] **Step 1: Write registry and no-write readiness tests**

```go
package impl

import (
	"context"
	"errors"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type fakeProviderStorage struct{ readiness error }

func (f fakeProviderStorage) PutObject(context.Context, string, []byte, ...storage.PutOptFn) error { return nil }
func (f fakeProviderStorage) PutObjectWithReader(context.Context, string, io.Reader, ...storage.PutOptFn) error { return nil }
func (f fakeProviderStorage) GetObject(context.Context, string) ([]byte, error) { return nil, nil }
func (f fakeProviderStorage) DeleteObject(context.Context, string) error { return nil }
func (f fakeProviderStorage) GetObjectUrl(context.Context, string, ...storage.GetOptFn) (string, error) { return "", nil }
func (f fakeProviderStorage) HeadObject(context.Context, string, ...storage.GetOptFn) (*storage.FileInfo, error) { return nil, nil }
func (f fakeProviderStorage) ListAllObjects(context.Context, string, ...storage.GetOptFn) ([]*storage.FileInfo, error) { return nil, nil }
func (f fakeProviderStorage) ListObjectsPaginated(context.Context, *storage.ListObjectsPaginatedInput, ...storage.GetOptFn) (*storage.ListObjectsPaginatedOutput, error) {
	return nil, nil
}
func (f fakeProviderStorage) CheckReadiness(context.Context) error { return f.readiness }

func TestRegistryBuildsSupportedProviderAndRejectsUnknown(t *testing.T) {
	registry := NewRegistry()
	registry.Register(domain.ProviderMinIO, func(context.Context, BuildInput) (storage.Storage, error) {
		return fakeProviderStorage{}, nil
	})
	_, err := registry.New(context.Background(), BuildInput{
		ProviderType: domain.ProviderMinIO,
		PublicConfig: domain.PublicConfig{Bucket: "coze", Endpoint: "minio:9000"},
		Credential: domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
	})
	if err != nil {
		t.Fatalf("registry.New(minio) error = %v", err)
	}
	_, err = registry.New(context.Background(), BuildInput{ProviderType: domain.ProviderType("unknown")})
	if !errors.Is(err, domain.ErrProviderUnsupported) {
		t.Fatalf("registry.New(unknown) error = %v", err)
	}
}

func TestSetAndGetRuntimeDescriptor(t *testing.T) {
	SetRuntimeDescriptor(domain.RuntimeDescriptor{Source: domain.RuntimeSourceDatabase, ConfigID: 7, RuntimeRevision: 3, ProviderType: domain.ProviderMinIO})
	got := CurrentRuntimeDescriptor()
	if got.ConfigID != 7 || got.RuntimeRevision != 3 || got.Source != domain.RuntimeSourceDatabase {
		t.Fatalf("runtime descriptor = %+v", got)
	}
}
```

- [ ] **Step 2: Run registry test and confirm failure**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./infra/storage/impl -run 'Registry|RuntimeDescriptor'
```

Expected: FAIL because registry types do not exist.

- [ ] **Step 3: Add registry implementation**

```go
package impl

import (
	"context"
	"sync"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type BuildInput struct {
	ProviderType  domain.ProviderType
	PublicConfig  domain.PublicConfig
	Credential    domain.CredentialInput
	ConfigID      uint64
	RuntimeRevision uint64
}

type Builder func(context.Context, BuildInput) (storage.Storage, error)

type Registry struct {
	builders map[domain.ProviderType]Builder
}

func NewRegistry() *Registry
func DefaultRegistry() *Registry
func (r *Registry) Register(provider domain.ProviderType, builder Builder)
func (r *Registry) New(ctx context.Context, input BuildInput) (storage.Storage, error)
```

Default registry entries:

- `ProviderMinIO` calls `minio.NewFromConfig`.
- `ProviderAWSS3` calls `s3.NewFromConfig`.
- `ProviderTOS` calls `tos.NewFromConfig`.
- Qiniu/Ali/Tencent/Huawei builders return `domain.ErrProviderUnsupported` until Task 7 adds implementations.

- [ ] **Step 4: Add runtime descriptor helpers**

```go
package impl

import (
	"sync/atomic"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
)

var runtimeDescriptor atomic.Value

func SetRuntimeDescriptor(descriptor domain.RuntimeDescriptor) {
	runtimeDescriptor.Store(descriptor)
}

func CurrentRuntimeDescriptor() domain.RuntimeDescriptor {
	value := runtimeDescriptor.Load()
	if value == nil {
		return domain.RuntimeDescriptor{}
	}
	descriptor, _ := value.(domain.RuntimeDescriptor)
	return descriptor
}
```

- [ ] **Step 5: Refactor existing adapters to strong config constructors**

Add these constructor signatures:

```go
// backend/infra/storage/impl/minio/minio.go
func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error)

// backend/infra/storage/impl/s3/s3.go
func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error)

// backend/infra/storage/impl/tos/tos.go
func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error)
```

Required refactor behavior:

- Keep old `New(...)` signatures as compatibility wrappers used by legacy env parsing.
- Remove MinIO `createBucketIfNeed` call from constructor path.
- Remove unused `test()` method calls and prevent signed URLs from being logged.
- Ensure `CheckReadiness` only calls read-only SDK APIs: MinIO `BucketExists`, S3 `HeadBucket`, TOS read-only bucket metadata/list call.
- Return `storage.ErrReadinessUnavailable` for SDK readiness errors after checking `ctx.Err()`.

- [ ] **Step 6: Run storage tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./infra/storage/...
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/infra/storage/impl backend/infra/storage/impl/minio/minio.go backend/infra/storage/impl/s3/s3.go backend/infra/storage/impl/tos/tos.go
git commit -m "feat: add object storage provider registry"
```

---

### Task 6: Shared Storage Contract Tests

**Files:**
- Create: `backend/infra/storage/impl/internal/contract/storage_contract.go`
- Create: `backend/infra/storage/impl/internal/contract/readiness_contract.go`
- Modify or create provider tests under `backend/infra/storage/impl/*/*_test.go`

- [ ] **Step 1: Add shared contract helpers**

```go
package contract

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type StorageFactory func(t *testing.T) storage.Storage

func RunStorageLifecycle(t *testing.T, factory StorageFactory) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := factory(t)
	key := "contract/object-storage-lifecycle.txt"
	body := []byte("object storage contract body")
	if err := client.PutObject(ctx, key, body, storage.WithContentType("text/plain")); err != nil {
		t.Fatalf("PutObject() error = %v", err)
	}
	got, err := client.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject() error = %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("GetObject() = %q", string(got))
	}
	info, err := client.HeadObject(ctx, key, storage.WithURL(true))
	if err != nil {
		t.Fatalf("HeadObject() error = %v", err)
	}
	if info == nil || info.Key == "" {
		t.Fatalf("HeadObject() info = %+v", info)
	}
	list, err := client.ListObjectsPaginated(ctx, &storage.ListObjectsPaginatedInput{Prefix: "contract/", PageSize: 20})
	if err != nil {
		t.Fatalf("ListObjectsPaginated() error = %v", err)
	}
	if list == nil {
		t.Fatal("ListObjectsPaginated() returned nil")
	}
	signed, err := client.GetObjectUrl(ctx, key, storage.WithExpire(time.Minute))
	if err != nil {
		t.Fatalf("GetObjectUrl() error = %v", err)
	}
	if !strings.Contains(signed, key) {
		t.Fatalf("signed URL does not reference key: %s", signed)
	}
	if err = client.DeleteObject(ctx, key); err != nil {
		t.Fatalf("DeleteObject() error = %v", err)
	}
}

type ReadinessRecorder struct {
	HeadBucketCalls int
	ListCalls       int
	CreateCalls     int
	PutCalls        int
	DeleteCalls     int
}

func AssertReadinessIsReadOnly(t *testing.T, recorder ReadinessRecorder) {
	t.Helper()
	if recorder.CreateCalls != 0 || recorder.PutCalls != 0 || recorder.DeleteCalls != 0 {
		t.Fatalf("readiness performed writes: %+v", recorder)
	}
	if recorder.HeadBucketCalls+recorder.ListCalls == 0 {
		t.Fatalf("readiness did not perform a read-only bucket check: %+v", recorder)
	}
}
```

- [ ] **Step 2: Wire existing provider tests to the contract**

Add provider-specific real tests behind environment gates:

```go
func TestMinIOContractWithExternalEndpoint(t *testing.T) {
	if os.Getenv("COZE_STORAGE_CONTRACT_MINIO") != "1" {
		t.Skip("set COZE_STORAGE_CONTRACT_MINIO=1 to run")
	}
	contract.RunStorageLifecycle(t, func(t *testing.T) storage.Storage {
		client, err := NewFromConfig(context.Background(), domain.PublicConfig{
			Bucket: os.Getenv("MINIO_CONTRACT_BUCKET"),
			Endpoint: os.Getenv("MINIO_CONTRACT_ENDPOINT"),
			UseSSL: os.Getenv("MINIO_CONTRACT_USE_SSL") == "true",
		}, domain.CredentialInput{
			AccessKeyID: os.Getenv("MINIO_CONTRACT_AK"),
			SecretAccessKey: os.Getenv("MINIO_CONTRACT_SK"),
		})
		if err != nil {
			t.Fatalf("NewFromConfig() error = %v", err)
		}
		return client
	})
}
```

Use provider-specific env names for S3/TOS and new providers:

- `COZE_STORAGE_CONTRACT_AWS_S3=1`
- `COZE_STORAGE_CONTRACT_TOS=1`
- `COZE_STORAGE_CONTRACT_QINIU=1`
- `COZE_STORAGE_CONTRACT_ALIYUN_OSS=1`
- `COZE_STORAGE_CONTRACT_TENCENT_COS=1`
- `COZE_STORAGE_CONTRACT_HUAWEI_OBS=1`

- [ ] **Step 3: Add mock readiness tests**

For each provider package, add a mock test that constructs the adapter with a test SDK facade and calls `CheckReadiness`. The recorder must pass `contract.AssertReadinessIsReadOnly`.

Required test names:

- `TestMinIOReadinessIsReadOnly`
- `TestS3ReadinessIsReadOnly`
- `TestTOSReadinessIsReadOnly`
- `TestQiniuReadinessIsReadOnly`
- `TestAliyunOSSReadinessIsReadOnly`
- `TestTencentCOSReadinessIsReadOnly`
- `TestHuaweiOBSReadinessIsReadOnly`

- [ ] **Step 4: Run contract tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./infra/storage/impl/...
```

Expected: PASS with real cloud contract tests skipped unless their explicit env gate is `1`.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/infra/storage/impl
git commit -m "test: add object storage adapter contracts"
```

---

### Task 7: Seven Provider Adapters

**Files:**
- Create: `backend/infra/storage/impl/qiniu/qiniu.go`
- Create: `backend/infra/storage/impl/aliyunoss/aliyunoss.go`
- Create: `backend/infra/storage/impl/tencentcos/tencentcos.go`
- Create: `backend/infra/storage/impl/huaweiobs/huaweiobs.go`
- Modify: `backend/infra/storage/impl/registry.go`
- Modify: `backend/go.mod`
- Modify: `backend/go.sum`

- [ ] **Step 1: Add official SDK dependencies**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
go get github.com/qiniu/go-sdk/v7
go get github.com/aliyun/alibabacloud-oss-go-sdk-v2
go get github.com/tencentyun/cos-go-sdk-v5
go get github.com/huaweicloud/huaweicloud-sdk-go-obs
```

Expected: `backend/go.mod` and `backend/go.sum` include only official SDK dependencies and transitive modules.

- [ ] **Step 2: Implement Qiniu adapter**

Required behavior:

- `NewFromConfig(ctx, cfg, credential)` validates `download_domain`, bucket, and credential pair.
- `PutObjectWithReader` uploads through Qiniu Kodo official SDK.
- `GetObjectUrl` builds URL from `download_domain`, `use_https`, object key escaping, and optional expiration.
- `GetObject` uses the generated object URL and bounded HTTP client path already used in storage helpers.
- `HeadObject`, `ListObjectsPaginated`, `DeleteObject`, and `CheckReadiness` call Kodo read/delete APIs.
- `CheckReadiness` lists at most one object and does not write.

Minimum compile-time checks:

```go
var (
	_ storage.Storage          = (*qiniuClient)(nil)
	_ storage.ReadinessChecker = (*qiniuClient)(nil)
)

func NewFromConfig(ctx context.Context, cfg domain.PublicConfig, credential domain.CredentialInput) (storage.Storage, error)
```

- [ ] **Step 3: Implement Ali OSS adapter**

Required behavior:

- Use `github.com/aliyun/alibabacloud-oss-go-sdk-v2/oss`.
- Use region-derived endpoint when `endpoint_override` is empty.
- Use HTTPS for override endpoints outside debug mode.
- `CheckReadiness` calls bucket metadata/location read API.
- `GetObjectUrl` signs GET URLs with response header override support from `storage.GetOption`.

Minimum compile-time checks:

```go
var (
	_ storage.Storage          = (*aliyunOSSClient)(nil)
	_ storage.StreamingStorage = (*aliyunOSSClient)(nil)
	_ storage.ReadinessChecker = (*aliyunOSSClient)(nil)
)
```

- [ ] **Step 4: Implement Tencent COS adapter**

Required behavior:

- Use `github.com/tencentyun/cos-go-sdk-v5`.
- Validate `bucket-appid` before client creation.
- Build endpoint from region when override is empty.
- `OpenObjectStream` returns SDK response body directly.
- `CheckReadiness` calls bucket read metadata/head API.

Minimum compile-time checks:

```go
var (
	_ storage.Storage          = (*tencentCOSClient)(nil)
	_ storage.StreamingStorage = (*tencentCOSClient)(nil)
	_ storage.ReadinessChecker = (*tencentCOSClient)(nil)
)
```

- [ ] **Step 5: Implement Huawei OBS adapter**

Required behavior:

- Use `github.com/huaweicloud/huaweicloud-sdk-go-obs/obs`.
- Require explicit HTTPS `endpoint`.
- `OpenObjectStream` returns SDK object body directly.
- `GetObjectUrl` uses OBS temporary signed URL API.
- `CheckReadiness` calls bucket metadata or location API.

Minimum compile-time checks:

```go
var (
	_ storage.Storage          = (*huaweiOBSClient)(nil)
	_ storage.StreamingStorage = (*huaweiOBSClient)(nil)
	_ storage.ReadinessChecker = (*huaweiOBSClient)(nil)
)
```

- [ ] **Step 6: Register all providers**

`DefaultRegistry()` must register:

```go
registry.Register(domain.ProviderQiniu, qiniu.NewFromConfig)
registry.Register(domain.ProviderAliyunOSS, aliyunoss.NewFromConfig)
registry.Register(domain.ProviderTencentCOS, tencentcos.NewFromConfig)
registry.Register(domain.ProviderHuaweiOBS, huaweiobs.NewFromConfig)
registry.Register(domain.ProviderAWSS3, s3.NewFromConfig)
registry.Register(domain.ProviderMinIO, minio.NewFromConfig)
registry.Register(domain.ProviderTOS, tos.NewFromConfig)
```

- [ ] **Step 7: Run provider tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./infra/storage/impl/...
```

Expected: PASS with real cloud tests skipped by default.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/go.mod backend/go.sum backend/infra/storage/impl
git commit -m "feat: support main object storage providers"
```

---

### Task 8: Runtime Loader, Env Import, Rescue Mode, ImageX

**Files:**
- Create: `backend/infra/storage/config/env.go`
- Create: `backend/application/objectstorage/bootstrap.go`
- Modify: `backend/infra/storage/impl/storage.go`
- Create: `backend/infra/storage/impl/storage_imagex.go`
- Modify: `backend/application/base/appinfra/app_infra.go`
- Modify: `backend/types/consts/consts.go`
- Create: `backend/application/objectstorage/bootstrap_test.go`

- [ ] **Step 1: Write bootstrap tests**

```go
package objectstorage

import (
	"context"
	"encoding/base64"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
)

func TestBootstrapImportsLegacyEnvWhenTableEmpty(t *testing.T) {
	h := newBootstrapHarness(t)
	h.getenv = mapGetenv(map[string]string{
		"OBJECT_STORAGE_CONFIG_SOURCE": "database",
		"OBJECT_STORAGE_CREDENTIAL_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"STORAGE_TYPE": "minio",
		"MINIO_ENDPOINT": "minio:9000",
		"MINIO_AK": "ak",
		"MINIO_SK": "sk",
		"STORAGE_BUCKET": "coze",
		"MINIO_USE_SSL": "false",
	})
	result, err := h.bootstrap.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}
	if result.RuntimeDescriptor.Source != domain.RuntimeSourceDatabase || result.RuntimeDescriptor.ConfigID == 0 {
		t.Fatalf("runtime descriptor = %+v", result.RuntimeDescriptor)
	}
	if h.repository.createCalls != 1 || !h.repository.created[0].Active {
		t.Fatalf("created configs = %+v", h.repository.created)
	}
}

func TestBootstrapFailsClosedWhenDatabaseModeHasNoActiveConfig(t *testing.T) {
	h := newBootstrapHarness(t)
	h.repository.count = 1
	h.repository.activeErr = domain.ErrPrimaryConfigMissing
	_, err := h.bootstrap.Bootstrap(context.Background())
	if err == nil {
		t.Fatal("Bootstrap() succeeded without active config")
	}
}

func TestBootstrapEnvRescueBypassesDatabaseActiveConfig(t *testing.T) {
	h := newBootstrapHarness(t)
	h.getenv = mapGetenv(map[string]string{
		"OBJECT_STORAGE_CONFIG_SOURCE": "env",
		"STORAGE_TYPE": "minio",
		"MINIO_ENDPOINT": "minio:9000",
		"MINIO_AK": "ak",
		"MINIO_SK": "sk",
		"STORAGE_BUCKET": "coze",
	})
	result, err := h.bootstrap.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("Bootstrap(env) error = %v", err)
	}
	if result.RuntimeDescriptor.Source != domain.RuntimeSourceEnvRescue {
		t.Fatalf("runtime source = %s", result.RuntimeDescriptor.Source)
	}
	if h.repository.getActiveCalls != 0 {
		t.Fatalf("rescue mode queried active config %d times", h.repository.getActiveCalls)
	}
}
```

- [ ] **Step 2: Run bootstrap tests and confirm failure**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./application/objectstorage -run Bootstrap
```

Expected: FAIL because application package does not exist.

- [ ] **Step 3: Add env parser**

Public API:

```go
const (
	ObjectStorageConfigSourceEnv = "OBJECT_STORAGE_CONFIG_SOURCE"
	ObjectStorageSourceDatabase  = "database"
	ObjectStorageSourceEnv       = "env"
)

type EnvConfig struct {
	Source       domain.RuntimeSource
	ProviderType domain.ProviderType
	PublicConfig domain.PublicConfig
	Credential   domain.CredentialInput
}

func LoadEnvConfig(getenv func(string) string) (EnvConfig, error)
```

Legacy mappings:

- `STORAGE_TYPE=minio` uses `MINIO_ENDPOINT`, `MINIO_AK`, `MINIO_SK`, `STORAGE_BUCKET`, `MINIO_USE_SSL`.
- `STORAGE_TYPE=tos` uses `TOS_ACCESS_KEY`, `TOS_SECRET_KEY`, `STORAGE_BUCKET`, `TOS_ENDPOINT`, `TOS_REGION`.
- `STORAGE_TYPE=s3` maps to `aws_s3` and uses `S3_ACCESS_KEY`, `S3_SECRET_KEY`, `STORAGE_BUCKET`, `S3_ENDPOINT`, `S3_REGION`.
- New provider env names:
  - `QINIU_ACCESS_KEY`, `QINIU_SECRET_KEY`, `QINIU_BUCKET`, `QINIU_DOWNLOAD_DOMAIN`, `QINIU_REGION`, `QINIU_USE_HTTPS`
  - `ALIYUN_OSS_ACCESS_KEY`, `ALIYUN_OSS_SECRET_KEY`, `ALIYUN_OSS_BUCKET`, `ALIYUN_OSS_REGION`, `ALIYUN_OSS_ENDPOINT_OVERRIDE`
  - `TENCENT_COS_ACCESS_KEY`, `TENCENT_COS_SECRET_KEY`, `TENCENT_COS_BUCKET`, `TENCENT_COS_REGION`, `TENCENT_COS_ENDPOINT_OVERRIDE`
  - `HUAWEI_OBS_ACCESS_KEY`, `HUAWEI_OBS_SECRET_KEY`, `HUAWEI_OBS_BUCKET`, `HUAWEI_OBS_REGION`, `HUAWEI_OBS_ENDPOINT`

- [ ] **Step 4: Add bootstrap service**

Public API:

```go
type Bootstrapper struct {
	Repository Repository
	Codec      CredentialCodec
	Registry   ProviderRegistry
	Getenv     func(string) string
	AllowHTTP  bool
}

type BootstrapResult struct {
	Storage           storage.Storage
	ImageX            imagex.ImageX
	RuntimeDescriptor domain.RuntimeDescriptor
}

func (b *Bootstrapper) Bootstrap(ctx context.Context) (*BootstrapResult, error)
```

Rules:

- database mode with empty table imports env config as `Environment import` and active.
- database mode with non-empty table loads exactly one active config or fails.
- env rescue mode builds directly from env and never writes database config.
- database mode requires codec for import and active DB config decryption.
- env rescue mode can run when codec is unavailable.
- after successful build, call `impl.SetRuntimeDescriptor`.

- [ ] **Step 5: Update `AppInfra.Init` order**

Change startup order in `backend/application/base/appinfra/app_infra.go`:

```go
deps.DB, err = mysql.New()
if err != nil {
	return nil, fmt.Errorf("init db failed, err=%w", err)
}

storageBootstrap := objectstorage.NewBootstrapper(deps.DB)
storageRuntime, err := storageBootstrap.Bootstrap(ctx)
if err != nil {
	return nil, fmt.Errorf("init object storage failed, err=%w", err)
}
deps.OSS = storageRuntime.Storage
deps.ImageXClient = storageRuntime.ImageX
```

Remove the earlier `deps.OSS, err = storage.New(ctx)` call. Keep `veimagex.NewDefault()` only when `FILE_UPLOAD_COMPONENT_TYPE=imagex`.

- [ ] **Step 6: Add generic storage-backed ImageX**

`storage_imagex.go` must implement `imagex.ImageX` by delegating uploads and URLs to the selected `storage.Storage`. It must not read `STORAGE_TYPE` or provider credentials.

- [ ] **Step 7: Run bootstrap and appinfra tests, then commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./application/objectstorage ./application/base/appinfra ./infra/storage/...
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/application/objectstorage backend/application/base/appinfra/app_infra.go backend/infra/storage/config/env.go backend/infra/storage/impl backend/types/consts/consts.go
git commit -m "feat: load object storage config at startup"
```

---

### Task 9: Application CRUD, Test, Activate, Delete

**Files:**
- Create: `backend/application/objectstorage/service.go`
- Create: `backend/application/objectstorage/service_test.go`

- [ ] **Step 1: Write service tests**

```go
package objectstorage

import (
	"context"
	"errors"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
)

func TestServiceCreateEncryptsCredentialAndDoesNotReturnSecret(t *testing.T) {
	h := newServiceHarness(t)
	created, err := h.service.Create(context.Background(), CreateRequest{
		Name: "qiniu-prod", ProviderType: domain.ProviderQiniu,
		PublicConfig: domain.PublicConfig{Bucket: "coze", DownloadDomain: "cdn.example.com", UseHTTPS: true},
		Credential: domain.CredentialInput{AccessKeyID: "ak", SecretAccessKey: "sk"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !created.CredentialConfigured {
		t.Fatalf("created view did not report configured credential: %+v", created)
	}
	if h.codec.lastPlain.AccessKeyID != "ak" || h.repository.created[0].CredentialSecret == "" {
		t.Fatalf("credential was not encrypted: %+v", h.repository.created)
	}
}

func TestServiceUpdatePreservesCredentialWhenBothFieldsEmpty(t *testing.T) {
	h := newServiceHarness(t)
	existing := h.repository.seed(validDomainConfig(1, "minio", true))
	updated, err := h.service.Update(context.Background(), UpdateRequest{
		ID: existing.ID, ExpectedVersion: existing.Version, Name: "renamed",
		PublicConfig: existing.PublicConfig,
		Credential: domain.CredentialInput{},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Version != existing.Version+1 || h.codec.encryptCalls != 0 {
		t.Fatalf("update result = %+v encrypt calls = %d", updated, h.codec.encryptCalls)
	}
}

func TestServiceTestDraftDoesNotPersistHealthWhenDraftDiffers(t *testing.T) {
	h := newServiceHarness(t)
	existing := h.repository.seed(validDomainConfig(1, "minio", true))
	result, err := h.service.Test(context.Background(), TestRequest{
		ID: existing.ID, ExpectedVersion: existing.Version, ProviderType: existing.ProviderType,
		PublicConfig: domain.PublicConfig{Bucket: "other", Endpoint: "minio:9000", UseSSL: false},
		Credential: domain.CredentialInput{},
	})
	if err != nil || !result.Success {
		t.Fatalf("Test() result = %+v error = %v", result, err)
	}
	if h.repository.healthUpdates != 0 {
		t.Fatalf("draft test persisted health %d times", h.repository.healthUpdates)
	}
}

func TestServiceActivateRequiresMigrationConfirmationAndRollsBackOnReadinessFailure(t *testing.T) {
	h := newServiceHarness(t)
	target := h.repository.seed(validDomainConfig(2, "target", false))
	_, err := h.service.Activate(context.Background(), ActivateRequest{ID: target.ID, ExpectedVersion: target.Version})
	if !errors.Is(err, domain.ErrMigrationConfirmation) {
		t.Fatalf("missing confirmation error = %v", err)
	}
	h.checker.err = domain.ErrConnectionFailed
	_, err = h.service.Activate(context.Background(), ActivateRequest{ID: target.ID, ExpectedVersion: target.Version, MigrationConfirmed: true})
	if !errors.Is(err, domain.ErrConnectionFailed) || h.repository.activateCalls != 0 {
		t.Fatalf("readiness failure error = %v activate calls = %d", err, h.repository.activateCalls)
	}
}
```

- [ ] **Step 2: Run service tests and confirm failure**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./application/objectstorage -run Service
```

Expected: FAIL because service API is missing.

- [ ] **Step 3: Define application interfaces and DTOs**

Required public API:

```go
import (
	"context"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	configrepo "github.com/coze-dev/coze-studio/backend/infra/storage/config"
	storageimpl "github.com/coze-dev/coze-studio/backend/infra/storage/impl"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

type Repository interface {
	Count(context.Context) (int64, error)
	List(context.Context) ([]domain.Config, error)
	Get(context.Context, uint64) (*domain.Config, error)
	GetActive(context.Context) (*domain.Config, error)
	Create(context.Context, domain.Config) (*domain.Config, error)
	Update(context.Context, configrepo.UpdateConfigInput) (*domain.Config, error)
	UpdateHealth(context.Context, uint64, domain.Health) error
	Activate(context.Context, uint64, uint64) (*domain.Config, error)
	Delete(context.Context, uint64, uint64, domain.RuntimeDescriptor) error
}

type CredentialCodec interface {
	Encrypt(uint64, domain.ProviderType, uint64, domain.CredentialInput) (string, error)
	Decrypt(uint64, domain.ProviderType, uint64, string) (domain.CredentialInput, error)
}

type ProviderRegistry interface {
	New(context.Context, storageimpl.BuildInput) (storage.Storage, error)
}

type Service struct {
	repository Repository
	codec      CredentialCodec
	registry   ProviderRegistry
	runtime    func() domain.RuntimeDescriptor
	allowHTTP  bool
}
```

Request/view types:

```go
type CreateRequest struct { Name string; ProviderType domain.ProviderType; PublicConfig domain.PublicConfig; Credential domain.CredentialInput }
type UpdateRequest struct { ID uint64; ExpectedVersion uint64; Name string; PublicConfig domain.PublicConfig; Credential domain.CredentialInput }
type TestRequest struct { ID uint64; ExpectedVersion uint64; ProviderType domain.ProviderType; PublicConfig domain.PublicConfig; Credential domain.CredentialInput }
type ActivateRequest struct { ID uint64; ExpectedVersion uint64; MigrationConfirmed bool }
type DeleteRequest struct { ID uint64; ExpectedVersion uint64 }

type ConfigView struct {
	ID uint64
	Name string
	ProviderType domain.ProviderType
	PublicConfig domain.PublicConfig
	CredentialConfigured bool
	Health domain.Health
	DesiredActive bool
	RuntimeActive bool
	RestartRequired bool
	Version uint64
	RuntimeRevision uint64
	CreatedAt string
	UpdatedAt string
}
```

- [ ] **Step 4: Implement service behavior**

Rules:

- `List` returns all configs plus top-level runtime source/restart state.
- `Create` requires AK/SK pair, validates config, inserts non-active config, encrypts credentials with the created ID, and updates the row with the envelope in one repository transaction path.
- `Update` rejects provider changes by reading the saved row and using saved `ProviderType`.
- `Update` preserves credentials only when both credential fields are empty.
- `Update` rejects a single credential field with `domain.ErrConfigInvalid`.
- `Test` uses draft values and persists health only when request public config and credential source match the saved row exactly.
- `Activate` runs readiness against target credentials before calling repository `Activate`.
- `Activate` requires `MigrationConfirmed` when target is not already desired-active.
- `Activate` is idempotent for an already desired-active target.
- `Delete` passes `impl.CurrentRuntimeDescriptor()` to repository deletion guard.
- SDK and repository errors are mapped to domain errors with no raw credentials in messages.

- [ ] **Step 5: Run service tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./application/objectstorage
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/application/objectstorage
git commit -m "feat: add object storage admin service"
```

---

### Task 10: Backend API Generation And Handlers

**Files:**
- Regenerate: `backend/api/model/admin/config/config.go`
- Regenerate: `backend/api/router/coze/api.go`
- Create: `backend/api/handler/coze/config_service_object_storage.go`
- Create: `backend/api/handler/coze/config_service_object_storage_test.go`

- [ ] **Step 1: Generate backend API model and router**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
PATH="$HOME/go/bin:$PATH" hz update -idl ../idl/api.thrift -enable_extends --exclude_file api/handler/coze/config_service.go
```

Then run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
./scripts/verify_api_codegen.sh
```

Expected: generated model/router include object storage RPCs, and verifier reports deterministic generation success.

- [ ] **Step 2: Write handler tests**

```go
package coze

import (
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"
)

func TestObjectStorageHandlersRedactCredentialsAndMapErrors(t *testing.T) {
	stub := &objectStorageServiceStub{}
	h := newObjectStorageTestServer(stub)
	body := `{"name":"minio","provider_type":6,"config":{"bucket":"coze","endpoint":"minio:9000","use_ssl":false},"credential":{"access_key_id":"ak-secret","secret_access_key":"sk-secret"}}`
	resp := ut.PerformRequest(h.Engine, http.MethodPost, "/api/admin/config/object-storage/create", strings.NewReader(body))
	require.Equal(t, http.StatusOK, resp.Code)
	require.NotContains(t, string(resp.Body.Bytes()), "ak-secret")
	require.NotContains(t, string(resp.Body.Bytes()), "sk-secret")
}

func TestObjectStorageHandlersRejectIncompleteCredentials(t *testing.T) {
	h := newObjectStorageTestServer(&objectStorageServiceStub{})
	body := `{"name":"minio","provider_type":6,"config":{"bucket":"coze","endpoint":"minio:9000"},"credential":{"access_key_id":"ak-only"}}`
	resp := ut.PerformRequest(h.Engine, http.MethodPost, "/api/admin/config/object-storage/create", strings.NewReader(body))
	require.Equal(t, http.StatusBadRequest, resp.Code)
	require.Contains(t, string(resp.Body.Bytes()), "OBJECT_STORAGE_CONFIG_INVALID")
}

func newObjectStorageTestServer(service objectStorageAdminService) *server.Hertz {
	h := server.Default()
	handler := newObjectStorageAdminHandler(service)
	h.GET("/api/admin/config/object-storage/list", handler.list)
	h.POST("/api/admin/config/object-storage/create", handler.create)
	h.POST("/api/admin/config/object-storage/update", handler.update)
	h.POST("/api/admin/config/object-storage/test", handler.test)
	h.POST("/api/admin/config/object-storage/activate", handler.activate)
	h.POST("/api/admin/config/object-storage/delete", handler.delete)
	return h
}
```

- [ ] **Step 3: Run handler tests and confirm failure**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./api/handler/coze -run ObjectStorage
```

Expected: FAIL because object storage handler glue does not exist.

- [ ] **Step 4: Add handler implementation**

`config_service_object_storage.go` must expose these generated function names:

```go
func ListObjectStorageConfigs(ctx context.Context, c *app.RequestContext)
func CreateObjectStorageConfig(ctx context.Context, c *app.RequestContext)
func UpdateObjectStorageConfig(ctx context.Context, c *app.RequestContext)
func TestObjectStorageConfig(ctx context.Context, c *app.RequestContext)
func ActivateObjectStorageConfig(ctx context.Context, c *app.RequestContext)
func DeleteObjectStorageConfig(ctx context.Context, c *app.RequestContext)
```

Handler rules:

- Bind generated request DTOs from `backend/api/model/admin/config`.
- Convert IDL enum values to domain strings through explicit switch statements.
- Reject unknown enum values with `OBJECT_STORAGE_PROVIDER_UNSUPPORTED`.
- Return `credential_configured` and never return `access_key_id` or `secret_access_key`.
- Map domain errors:
  - config invalid/provider unsupported: HTTP 400
  - not found: HTTP 404
  - version conflict: HTTP 409
  - connection failed: HTTP 400
  - active delete forbidden: HTTP 409
  - credential unavailable: HTTP 503
  - internal: HTTP 500 with redacted log line
- Limit body size to `128 * 1024` bytes and use strict JSON key validation copied from `admin_sandbox.go` under object-storage-specific function names.

- [ ] **Step 5: Run API tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./api/handler/coze ./api/router/coze -run 'ObjectStorage|APICodegen'
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add backend/api/model/admin/config/config.go backend/api/router backend/api/handler/coze/config_service_object_storage.go backend/api/handler/coze/config_service_object_storage_test.go
git commit -m "feat: expose object storage admin api"
```

---

### Task 11: Frontend API Schema And Service Layer

**Files:**
- Modify: `frontend/packages/arch/api-schema/api.config.js`
- Modify: `frontend/packages/arch/api-schema/package.json`
- Modify: `frontend/packages/arch/api-schema/src/index.ts`
- Generate: `frontend/packages/arch/api-schema/src/idl/admin/config.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/service.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/__tests__/system-service.test.ts`

- [ ] **Step 1: Add api-schema admin config entry**

In `api.config.js`, add:

```js
adminConfig: './idl/admin/config.thrift',
```

In `package.json` exports and `typesVersions`, add:

```json
"./admin-config": "./src/idl/admin/config.ts"
```

In `src/index.ts`, add:

```ts
export * as adminConfig from './idl/admin/config';
```

- [ ] **Step 2: Generate frontend API schema**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/frontend/packages/arch/api-schema
node ../../../../common/scripts/install-run-rushx.js update
```

Expected: `src/idl/admin/config.ts` exists and exports `ListObjectStorageConfigs`, `CreateObjectStorageConfig`, `UpdateObjectStorageConfig`, `TestObjectStorageConfig`, `ActivateObjectStorageConfig`, and `DeleteObjectStorageConfig`.

- [ ] **Step 3: Write service tests**

Add to `system-service.test.ts`:

```ts
it('uses generated object storage APIs without returning credentials', async () => {
  fetchMock.mockResolvedValueOnce(
    new Response(
      JSON.stringify({
        configs: [
          {
            id: '1',
            name: 'minio',
            provider_type: 6,
            config: { bucket: 'coze', endpoint: 'minio:9000', use_ssl: false },
            credential_configured: true,
            health: { status: 1 },
            desired_active: true,
            runtime_active: true,
            restart_required: false,
            version: '1',
            runtime_revision: '1',
            created_at: '2026-07-28T00:00:00Z',
            updated_at: '2026-07-28T00:00:00Z',
          },
        ],
        runtime_source: 1,
        restart_required: false,
      }),
      { status: 200 },
    ),
  );

  const result = await listObjectStorageConfigs();
  expect(result.configs[0]?.credential_configured).toBe(true);
  expect(JSON.stringify(result)).not.toContain('secret_access_key');
});

it('sends activation migration confirmation', async () => {
  fetchMock.mockResolvedValueOnce(new Response(JSON.stringify({ config: {} }), { status: 200 }));
  await activateObjectStorageConfig({ id: '7', expected_version: '3', migration_confirmed: true });
  expect(fetchMock).toHaveBeenCalledWith(
    '/api/admin/config/object-storage/activate',
    expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({ id: '7', expected_version: '3', migration_confirmed: true }),
    }),
  );
});
```

- [ ] **Step 4: Add service functions**

Add typed exports in `service.ts`:

```ts
export type ObjectStorageProviderType =
  | 'qiniu'
  | 'aliyun_oss'
  | 'tencent_cos'
  | 'huawei_obs'
  | 'aws_s3'
  | 'minio'
  | 'tos';

export interface ObjectStoragePublicConfig {
  bucket?: string;
  region?: string;
  endpoint?: string;
  endpoint_override?: string;
  force_path_style?: boolean;
  use_ssl?: boolean;
  download_domain?: string;
  use_https?: boolean;
}

export interface ObjectStorageCredentialInput {
  access_key_id?: string;
  secret_access_key?: string;
}

export const listObjectStorageConfigs = () =>
  getJSON<ListObjectStorageConfigsResp>('/api/admin/config/object-storage/list');

export const createObjectStorageConfig = (payload: CreateObjectStorageConfigReq) =>
  postJSON<CreateObjectStorageConfigResp>('/api/admin/config/object-storage/create', payload);

export const updateObjectStorageConfig = (payload: UpdateObjectStorageConfigReq) =>
  postJSON<UpdateObjectStorageConfigResp>('/api/admin/config/object-storage/update', payload);

export const testObjectStorageConfig = (payload: TestObjectStorageConfigReq) =>
  postJSON<TestObjectStorageConfigResp>('/api/admin/config/object-storage/test', payload);

export const activateObjectStorageConfig = (payload: ActivateObjectStorageConfigReq) =>
  postJSON<ActivateObjectStorageConfigResp>('/api/admin/config/object-storage/activate', payload);

export const deleteObjectStorageConfig = (payload: DeleteObjectStorageConfigReq) =>
  postJSON<Record<string, never>>('/api/admin/config/object-storage/delete', payload);
```

Use generated IDL types from `@coze-studio/api-schema/admin-config` where the generator exports them. Keep the small string union helpers only for UI labels and form state.

- [ ] **Step 5: Run service tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/frontend/apps/coze-studio
node ../../../common/scripts/install-run-rushx.js test src/pages/system/__tests__/system-service.test.ts
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add frontend/packages/arch/api-schema frontend/apps/coze-studio/src/pages/system/service.ts frontend/apps/coze-studio/src/pages/system/__tests__/system-service.test.ts
git commit -m "feat: add object storage frontend api client"
```

---

### Task 12: Frontend Object Storage UI

**Files:**
- Modify: `frontend/apps/coze-studio/src/pages/system/content.ts`
- Modify: `frontend/apps/coze-studio/src/pages/system/index.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/object-storage-view-model.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/object-storage-section.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/object-storage-form.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/object-storage-view-model.test.ts`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/object-storage-section.test.tsx`
- Create: `frontend/apps/coze-studio/src/pages/system/__tests__/object-storage-form.test.tsx`

- [ ] **Step 1: Write view-model tests**

```ts
import { describe, expect, it } from 'vitest';

import {
  getObjectStorageProviderLabel,
  getObjectStorageRuntimeLabel,
  validateObjectStorageDraft,
} from '../object-storage-view-model';

describe('object storage view model', () => {
  it('labels providers and runtime state', () => {
    expect(getObjectStorageProviderLabel('qiniu')).toBe('七牛 Kodo');
    expect(
      getObjectStorageRuntimeLabel({
        desired_active: true,
        runtime_active: false,
        restart_required: true,
      }),
    ).toBe('待重启');
    expect(
      getObjectStorageRuntimeLabel({
        desired_active: false,
        runtime_active: true,
        restart_required: true,
      }),
    ).toBe('运行中');
  });

  it('validates provider-specific draft fields and credential pairs', () => {
    expect(
      validateObjectStorageDraft({
        name: 'qiniu',
        provider_type: 'qiniu',
        config: { bucket: 'coze', download_domain: 'cdn.example.com', use_https: true },
        credential: { access_key_id: 'ak', secret_access_key: 'sk' },
        editing: false,
      }),
    ).toEqual([]);
    expect(
      validateObjectStorageDraft({
        name: 'bad',
        provider_type: 'tencent_cos',
        config: { bucket: 'coze', region: 'ap-guangzhou' },
        credential: { access_key_id: 'ak' },
        editing: false,
      }),
    ).toContain('腾讯 COS Bucket 需要使用 bucket-appid 格式');
  });
});
```

- [ ] **Step 2: Write component tests**

```tsx
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';

import { ObjectStorageSection } from '../object-storage-section';

describe('ObjectStorageSection', () => {
  it('renders runtime labels and disables protected delete', async () => {
    render(
      <ObjectStorageSection
        configs={[
          {
            id: '1',
            name: 'minio',
            provider_type: 'minio',
            config: { bucket: 'coze', endpoint: 'minio:9000' },
            credential_configured: true,
            health: { status: 'healthy' },
            desired_active: true,
            runtime_active: true,
            restart_required: false,
            version: '1',
            runtime_revision: '1',
            created_at: '2026-07-28T00:00:00Z',
            updated_at: '2026-07-28T00:00:00Z',
          },
        ]}
        loading={false}
        runtimeSource="database"
        onCreate={vi.fn()}
        onUpdate={vi.fn()}
        onTest={vi.fn()}
        onActivate={vi.fn()}
        onDelete={vi.fn()}
        onRefresh={vi.fn()}
      />,
    );
    expect(screen.getByText('当前主配置')).toBeInTheDocument();
    const row = screen.getByRole('row', { name: /minio/i });
    expect(within(row).getByRole('button', { name: '删除配置' })).toBeDisabled();
  });

  it('requires migration confirmation before activation', async () => {
    const onActivate = vi.fn();
    render(<ObjectStorageSection configs={[inactiveConfigFixture]} loading={false} runtimeSource="database" onActivate={onActivate} />);
    await userEvent.click(screen.getByRole('button', { name: '激活配置' }));
    expect(screen.getByRole('button', { name: '确认激活' })).toBeDisabled();
    await userEvent.click(screen.getByLabelText('旧 Bucket 中的对象已迁移到目标 Bucket，并保持原对象 Key'));
    await userEvent.click(screen.getByRole('button', { name: '确认激活' }));
    expect(onActivate).toHaveBeenCalledWith(expect.objectContaining({ migration_confirmed: true }));
  });
});
```

- [ ] **Step 3: Implement view model**

`object-storage-view-model.ts` exports:

```ts
export const OBJECT_STORAGE_PROVIDERS = [
  { value: 'qiniu', label: '七牛 Kodo' },
  { value: 'aliyun_oss', label: '阿里 OSS' },
  { value: 'tencent_cos', label: '腾讯 COS' },
  { value: 'huawei_obs', label: '华为 OBS' },
  { value: 'aws_s3', label: 'AWS S3' },
  { value: 'minio', label: 'MinIO' },
  { value: 'tos', label: '火山 TOS' },
] as const;

export function getObjectStorageProviderLabel(provider: ObjectStorageProviderType): string
export function getObjectStorageRuntimeLabel(config: Pick<ObjectStorageConfigView, 'desired_active' | 'runtime_active' | 'restart_required'>): '当前主配置' | '待重启' | '运行中' | '未激活'
export function validateObjectStorageDraft(draft: ObjectStorageDraft): string[]
export function toObjectStorageCreatePayload(draft: ObjectStorageDraft): CreateObjectStorageConfigReq
export function toObjectStorageUpdatePayload(draft: ObjectStorageDraft, existing: ObjectStorageConfigView): UpdateObjectStorageConfigReq
```

Validation messages:

- `配置名称不能为空`
- `Bucket 不能为空`
- `AK/SK 需要同时填写`
- `新增配置需要填写 AK/SK`
- `七牛下载域名不能包含协议或路径`
- `腾讯 COS Bucket 需要使用 bucket-appid 格式`
- `Endpoint 不能为空`
- `Region 不能为空`

- [ ] **Step 4: Implement section and form**

UI requirements:

- Add system section key `object-storage` with title `对象存储`.
- Put it in `SYSTEM_NAV_GROUPS` system group before `settings`.
- List columns: name, provider, bucket, health, runtime status, updated time, actions.
- Use icon buttons from existing design/icon package where available, with `aria-label` values:
  - `刷新对象存储配置`
  - `新增对象存储配置`
  - `测试连接`
  - `编辑配置`
  - `激活配置`
  - `删除配置`
- Use a right drawer for create/edit.
- Provider selector is disabled when editing.
- Credential password inputs are empty on edit.
- Save button is disabled when validation messages exist.
- Test button calls draft test and does not close drawer.
- Delete confirmation displays protected reasons for desired-active and runtime-active configs.
- Activation confirmation includes the migration checkbox and shows pending restart state after success.
- Rescue mode top banner displays `当前进程正在使用环境变量救援配置`.

- [ ] **Step 5: Wire page state in `index.tsx`**

Add state and loaders:

```ts
const [objectStorageConfigs, setObjectStorageConfigs] = useState<ObjectStorageConfigView[]>([]);
const [objectStorageRuntimeSource, setObjectStorageRuntimeSource] = useState<'database' | 'env_rescue'>('database');
const [objectStorageLoading, setObjectStorageLoading] = useState(false);
const [objectStorageMessage, setObjectStorageMessage] = useState('');

const loadObjectStorageConfigs = useCallback(async () => {
  setObjectStorageLoading(true);
  setObjectStorageMessage('');
  try {
    const response = await listObjectStorageConfigs();
    setObjectStorageConfigs(response.configs ?? []);
    setObjectStorageRuntimeSource(response.runtime_source === 2 ? 'env_rescue' : 'database');
  } catch (error) {
    setObjectStorageMessage('加载对象存储配置失败');
  } finally {
    setObjectStorageLoading(false);
  }
}, []);
```

Call `loadObjectStorageConfigs()` with other admin initial data and after create/update/test/activate/delete mutations.

- [ ] **Step 6: Run frontend tests and commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/frontend/apps/coze-studio
node ../../../common/scripts/install-run-rushx.js test src/pages/system/__tests__/object-storage-view-model.test.ts src/pages/system/__tests__/object-storage-form.test.tsx src/pages/system/__tests__/object-storage-section.test.tsx src/pages/system/__tests__/content.test.ts src/pages/system/__tests__/system-page.test.tsx
```

Expected: PASS.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add frontend/apps/coze-studio/src/pages/system
git commit -m "feat: add object storage admin page"
```

---

### Task 13: Docker Env, Runbook, Long-Term Context

**Files:**
- Modify: `docker/docker-compose.yml`
- Modify: `docker/docker-compose-debug.yml`
- Modify: `docker/docker-compose-oceanbase.yml`
- Modify: `docker/docker-compose-oceanbase_debug.yml`
- Create: `docs/superpowers/runbooks/object-storage-control-plane-operations.md`
- Modify: `docs/superpowers/context/project-context.md`

- [ ] **Step 1: Add Docker env variables**

Add to `coze-server` environment blocks:

```yaml
OBJECT_STORAGE_CONFIG_SOURCE: ${OBJECT_STORAGE_CONFIG_SOURCE:-database}
OBJECT_STORAGE_CREDENTIAL_KEY: ${OBJECT_STORAGE_CREDENTIAL_KEY:-}
```

Keep existing `STORAGE_TYPE`, `STORAGE_BUCKET`, MinIO, TOS, and S3 variables so first import can read them. Do not add sample AK/SK values for real cloud providers.

- [ ] **Step 2: Write runbook**

Create `docs/superpowers/runbooks/object-storage-control-plane-operations.md` with these sections:

~~~markdown
# Object Storage Control Plane Operations

## Required Secret

Generate `OBJECT_STORAGE_CREDENTIAL_KEY` as standard Base64 for 32 random bytes:

```bash
openssl rand -base64 32
```

Store it outside the database backup, for example Docker Secret or a managed secret store. Losing this key makes existing database credentials undecryptable.

## First Startup

Keep the existing `STORAGE_TYPE` provider variables for the first new-version startup. When `object_storage_configs` is empty, the server imports the env config as `Environment import` and sets it active.

## Configure A New Provider

Create the provider in System Admin > 对象存储, test it, migrate historical objects preserving object keys, activate it, and restart `coze-server`.

## Rescue Mode

Set `OBJECT_STORAGE_CONFIG_SOURCE=env` and provide provider env variables. Rescue mode bypasses database active config and does not modify saved rows. Remove the variable and restart to return to database mode.

## Deletion Guard

The desired active config and the currently running config cannot be deleted. After switching providers, restart once before deleting the old running row.
~~~

- [ ] **Step 3: Update project context**

Add one bullet under backend or system management facts:

```markdown
- 对象存储配置由系统级 `object_storage_configs` 单表管理，AK/SK 使用
  `OBJECT_STORAGE_CREDENTIAL_KEY` AES-GCM 加密；启动时加载唯一主配置，管理页
  激活只改变期望主配置，重启后生效。显式
  `OBJECT_STORAGE_CONFIG_SOURCE=env` 是数据库配置故障时的救援模式。
```

- [ ] **Step 4: Run docs and compose grep checks, then commit**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
rg "OBJECT_STORAGE_CREDENTIAL_KEY" docker docs/superpowers
rg "OBJECT_STORAGE_CONFIG_SOURCE" docker docs/superpowers
```

Expected: both variables appear in compose files and the runbook.

Commit:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add docker docs/superpowers/runbooks/object-storage-control-plane-operations.md docs/superpowers/context/project-context.md
git commit -m "docs: document object storage operations"
```

---

### Task 14: Full Verification And Local QA

**Files:**
- No planned source changes unless verification exposes a failing test or UI defect.

- [ ] **Step 1: Run Atlas validation**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
docker run --rm -v "$PWD/docker/atlas/migrations:/migrations" arigaio/atlas:0.35.0-community-alpine migrate validate --dir file:///migrations
```

Expected: validation succeeds for the migration directory.

- [ ] **Step 2: Run backend focused tests**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-object-storage-go-cache go test ./domain/storageconfig ./infra/storage/config ./infra/storage/... ./application/objectstorage ./application/base/appinfra ./api/handler/coze ./api/router/coze
```

Expected: PASS.

- [ ] **Step 3: Run backend broad test if focused tests pass**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/backend
GOCACHE=/private/tmp/coze-go-build go test -p 1 -gcflags="all=-l -N" ./...
```

Expected: PASS or a documented unrelated existing failure with exact package and error.

- [ ] **Step 4: Run frontend focused tests**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/frontend/apps/coze-studio
node ../../../common/scripts/install-run-rushx.js test src/pages/system/__tests__/object-storage-view-model.test.ts src/pages/system/__tests__/object-storage-form.test.tsx src/pages/system/__tests__/object-storage-section.test.tsx src/pages/system/__tests__/system-service.test.ts src/pages/system/__tests__/system-page.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Run frontend schema tests**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane/frontend/packages/arch/api-schema
node ../../../../common/scripts/install-run-rushx.js test __tests__/admin-object-storage-contract.test.ts
```

Expected: PASS.

- [ ] **Step 6: Scan for credential leaks**

Run:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
rg "ak-live-marker|sk-live-marker|secret_access_key.*json|access_key_id.*response" backend frontend docker docs
```

Expected: no production response or UI path returns AK/SK; test-only markers appear only in tests.

- [ ] **Step 7: Browser QA**

Start the app stack using the repository's local runbook. In Codex in-app browser, verify:

- `/space/newx-system/object-storage` or the route produced by system navigation loads for an admin user.
- Empty state shows create action.
- Create drawer switches fields for Qiniu, Ali OSS, Tencent COS, Huawei OBS, AWS S3, MinIO, and TOS.
- Edit drawer keeps provider read-only and credential fields empty.
- Test connection shows loading and result state.
- Activate dialog keeps confirm disabled until migration checkbox is selected.
- Desired active and runtime active rows cannot be deleted.
- Rescue mode banner appears when backend reports `runtime_source=env_rescue`.
- Browser console has no React errors.

- [ ] **Step 8: Final commit for verification fixes**

If verification required source fixes, commit them:

```bash
cd /private/tmp/coze-studio-object-storage-control-plane
git add <fixed-files>
git commit -m "fix: stabilize object storage control plane"
```

If no fixes were required, do not create an empty commit.

---

## Self-Review Checklist

- Spec coverage:
  - Multiple configs and exactly one desired active: Tasks 1, 4, 9.
  - One table only: Task 1.
  - Encrypted AK/SK and no API echo: Tasks 3, 9, 10, 11, 14.
  - Seven providers with official SDKs: Tasks 5, 6, 7.
  - Read-only connection test: Tasks 6, 9.
  - Restart-only activation and runtime descriptor: Tasks 5, 8, 9, 12.
  - First env import and rescue mode: Tasks 8, 13.
  - System Admin CRUD/test/activate/delete UI: Tasks 11, 12.
  - Object migration confirmation: Tasks 9, 10, 12.
  - Docker/runbook operations: Task 13.
  - Verification and browser QA: Task 14.
- Placeholder scan:
  - The plan contains no deferred implementation markers and no unspecified provider task.
- Type consistency:
  - Domain provider strings are stable lower-case values.
  - IDL provider enum values are mapped through handlers instead of passing numeric values into the domain.
  - `version` and `runtime_revision` are unsigned in Go, string-converted in API/TS where IDL uses JS conversion.
