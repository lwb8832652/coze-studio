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
