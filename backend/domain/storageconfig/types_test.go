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

package storageconfig

import (
	"errors"
	"strings"
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

func TestErrorCodeOfMapsAllDomainErrors(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: ErrConfigInvalid, want: "OBJECT_STORAGE_CONFIG_INVALID"},
		{err: ErrNotFound, want: "OBJECT_STORAGE_NOT_FOUND"},
		{err: ErrVersionConflict, want: "OBJECT_STORAGE_VERSION_CONFLICT"},
		{err: ErrConnectionFailed, want: "OBJECT_STORAGE_CONNECTION_FAILED"},
		{err: ErrActiveDeleteForbidden, want: "OBJECT_STORAGE_ACTIVE_DELETE_FORBIDDEN"},
		{err: ErrCredentialUnavailable, want: "OBJECT_STORAGE_CREDENTIAL_UNAVAILABLE"},
		{err: ErrProviderUnsupported, want: "OBJECT_STORAGE_PROVIDER_UNSUPPORTED"},
		{err: ErrPrimaryConfigMissing, want: "OBJECT_STORAGE_PRIMARY_CONFIG_MISSING"},
		{err: ErrMigrationConfirmation, want: "OBJECT_STORAGE_MIGRATION_CONFIRMATION_REQUIRED"},
	}
	for _, test := range tests {
		if got := ErrorCodeOf(test.err); got != test.want {
			t.Fatalf("ErrorCodeOf(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestNormalizeNameTrimsAndRejectsInvalidNames(t *testing.T) {
	normalized, err := NormalizeName("  primary storage  ")
	if err != nil {
		t.Fatalf("NormalizeName() error = %v", err)
	}
	if normalized != "primary storage" {
		t.Fatalf("NormalizeName() = %q, want %q", normalized, "primary storage")
	}

	if _, err := NormalizeName(" \t\n "); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("empty name error = %v, want ErrConfigInvalid", err)
	}
	if _, err := NormalizeName(strings.Repeat("界", 129)); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("too long name error = %v, want ErrConfigInvalid", err)
	}
}

func TestValidateCredentialInputTreatsWhitespaceOnlyAsEmpty(t *testing.T) {
	normalized := NormalizeCredentialInput(CredentialInput{AccessKeyID: " ak ", SecretAccessKey: " sk "})
	if normalized.AccessKeyID != "ak" || normalized.SecretAccessKey != "sk" {
		t.Fatalf("NormalizeCredentialInput() = %+v, want trimmed AK/SK", normalized)
	}
	if err := ValidateCredentialInput(CredentialInput{AccessKeyID: " ak ", SecretAccessKey: " sk "}); err != nil {
		t.Fatalf("trimmed pair error = %v", err)
	}
	if err := ValidateCredentialInput(CredentialInput{AccessKeyID: "", SecretAccessKey: " \t\n "}); err != nil {
		t.Fatalf("whitespace-only single side error = %v, want nil", err)
	}
	if HasCredentialPair(CredentialInput{AccessKeyID: " ak ", SecretAccessKey: " \t\n "}) {
		t.Fatal("HasCredentialPair() = true, want false for whitespace-only secret")
	}
}

func TestValidatePublicConfigNormalizesMinIOEndpointSchemes(t *testing.T) {
	httpConfig, err := ValidatePublicConfig(ProviderMinIO, PublicConfig{Bucket: "coze", Endpoint: "http://minio:9000"}, ValidationMode{AllowHTTP: true})
	if err != nil {
		t.Fatalf("http MinIO config error = %v", err)
	}
	if httpConfig.Endpoint != "minio:9000" || httpConfig.UseSSL {
		t.Fatalf("http MinIO config = %+v, want Endpoint minio:9000 and UseSSL false", httpConfig)
	}

	httpsConfig, err := ValidatePublicConfig(ProviderMinIO, PublicConfig{Bucket: "coze", Endpoint: "https://minio.example.com"}, ValidationMode{})
	if err != nil {
		t.Fatalf("https MinIO config error = %v", err)
	}
	if httpsConfig.Endpoint != "minio.example.com" || !httpsConfig.UseSSL {
		t.Fatalf("https MinIO config = %+v, want Endpoint minio.example.com and UseSSL true", httpsConfig)
	}
}

func TestRuntimeFieldsEqualIgnoresControlPlaneState(t *testing.T) {
	left := Config{
		ID:               1,
		Name:             "left",
		ProviderType:     ProviderAWSS3,
		PublicConfig:     PublicConfig{Bucket: "coze", Region: "us-east-1"},
		CredentialSecret: "secret-a",
		Active:           true,
		Version:          1,
		RuntimeRevision:  11,
	}
	right := left
	right.ID = 2
	right.Name = "right"
	right.Active = false
	right.Version = 2
	right.RuntimeRevision = 12
	if !RuntimeFieldsEqual(left, right) {
		t.Fatal("RuntimeFieldsEqual() = false, want true when only control-plane state changes")
	}

	right = left
	right.PublicConfig.Region = "us-west-2"
	if RuntimeFieldsEqual(left, right) {
		t.Fatal("RuntimeFieldsEqual() = true, want false when public config changes")
	}

	right = left
	right.CredentialSecret = "secret-b"
	if RuntimeFieldsEqual(left, right) {
		t.Fatal("RuntimeFieldsEqual() = true, want false when credential secret changes")
	}
}

func TestRestartRequiredComparesDatabaseRuntimeDescriptor(t *testing.T) {
	desired := &Config{ID: 7, ProviderType: ProviderAWSS3, RuntimeRevision: 42}
	if RestartRequired(RuntimeDescriptor{Source: RuntimeSourceDatabase, ConfigID: 7, ProviderType: ProviderAWSS3, RuntimeRevision: 42}, desired) {
		t.Fatal("RestartRequired() = true, want false for matching database runtime")
	}
	if !RestartRequired(RuntimeDescriptor{Source: RuntimeSourceDatabase, ConfigID: 7, ProviderType: ProviderAWSS3, RuntimeRevision: 43}, desired) {
		t.Fatal("RestartRequired() = false, want true for revision mismatch")
	}
	if !RestartRequired(RuntimeDescriptor{Source: RuntimeSourceDatabase, ConfigID: 7, ProviderType: ProviderAWSS3, RuntimeRevision: 42}, nil) {
		t.Fatal("RestartRequired() = false, want true for missing desired config")
	}
	if !RestartRequired(RuntimeDescriptor{Source: RuntimeSourceEnvRescue, ConfigID: 7, ProviderType: ProviderAWSS3, RuntimeRevision: 42}, desired) {
		t.Fatal("RestartRequired() = false, want true for env rescue runtime with database desired config")
	}
}
