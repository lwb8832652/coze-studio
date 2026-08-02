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

package config

import (
	"errors"
	"testing"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
)

func TestLoadEnvConfigDefaultsDatabaseAndParsesLegacyMinIO(t *testing.T) {
	env, err := LoadEnvConfig(mapGetenv(map[string]string{
		"STORAGE_TYPE":   "minio",
		"MINIO_ENDPOINT": "minio:9000",
		"MINIO_AK":       "ak",
		"MINIO_SK":       "sk",
		"STORAGE_BUCKET": "coze",
		"MINIO_USE_SSL":  "false",
	}), domain.ValidationMode{AllowHTTP: true})
	if err != nil {
		t.Fatalf("LoadEnvConfig(minio) error = %v", err)
	}
	if env.Source != domain.RuntimeSourceDatabase || env.ProviderType != domain.ProviderMinIO {
		t.Fatalf("env source/provider = %s/%s", env.Source, env.ProviderType)
	}
	if env.PublicConfig.Endpoint != "minio:9000" || env.PublicConfig.UseSSL {
		t.Fatalf("minio public config = %+v", env.PublicConfig)
	}
	if env.Credential.AccessKeyID != "ak" || env.Credential.SecretAccessKey != "sk" {
		t.Fatal("minio credential was not parsed")
	}
}

func TestLoadEnvConfigParsesAllSupportedProviders(t *testing.T) {
	tests := []struct {
		name     string
		values   map[string]string
		provider domain.ProviderType
		assert   func(*testing.T, domain.PublicConfig)
	}{
		{
			name: "qiniu",
			values: map[string]string{
				"STORAGE_TYPE":               "qiniu",
				"QINIU_ACCESS_KEY":           "ak",
				"QINIU_SECRET_KEY":           "sk",
				"QINIU_BUCKET":               "coze",
				"QINIU_DOWNLOAD_DOMAIN":      "cdn.example.com",
				"QINIU_REGION":               "z0",
				"QINIU_USE_HTTPS":            "",
				ObjectStorageConfigSourceEnv: ObjectStorageSourceDatabase,
			},
			provider: domain.ProviderQiniu,
			assert: func(t *testing.T, cfg domain.PublicConfig) {
				t.Helper()
				if cfg.DownloadDomain != "cdn.example.com" || !cfg.UseHTTPS {
					t.Fatalf("qiniu public config = %+v", cfg)
				}
			},
		},
		{
			name: "aliyun",
			values: map[string]string{
				"STORAGE_TYPE":                 "aliyun_oss",
				"ALIYUN_OSS_ACCESS_KEY":        "ak",
				"ALIYUN_OSS_SECRET_KEY":        "sk",
				"ALIYUN_OSS_BUCKET":            "coze",
				"ALIYUN_OSS_REGION":            "cn-hangzhou",
				"ALIYUN_OSS_ENDPOINT_OVERRIDE": "https://oss-cn-hangzhou.aliyuncs.com",
			},
			provider: domain.ProviderAliyunOSS,
			assert: func(t *testing.T, cfg domain.PublicConfig) {
				t.Helper()
				if cfg.EndpointOverride != "https://oss-cn-hangzhou.aliyuncs.com" {
					t.Fatalf("aliyun public config = %+v", cfg)
				}
			},
		},
		{
			name: "tencent",
			values: map[string]string{
				"STORAGE_TYPE":                  "tencent_cos",
				"TENCENT_COS_ACCESS_KEY":        "ak",
				"TENCENT_COS_SECRET_KEY":        "sk",
				"TENCENT_COS_BUCKET":            "coze-1250000000",
				"TENCENT_COS_REGION":            "ap-shanghai",
				"TENCENT_COS_ENDPOINT_OVERRIDE": "https://coze-1250000000.cos.ap-shanghai.myqcloud.com",
			},
			provider: domain.ProviderTencentCOS,
			assert: func(t *testing.T, cfg domain.PublicConfig) {
				t.Helper()
				if cfg.Region != "ap-shanghai" {
					t.Fatalf("tencent public config = %+v", cfg)
				}
			},
		},
		{
			name: "huawei",
			values: map[string]string{
				"STORAGE_TYPE":          "huawei_obs",
				"HUAWEI_OBS_ACCESS_KEY": "ak",
				"HUAWEI_OBS_SECRET_KEY": "sk",
				"HUAWEI_OBS_BUCKET":     "coze",
				"HUAWEI_OBS_REGION":     "cn-north-4",
				"HUAWEI_OBS_ENDPOINT":   "https://obs.cn-north-4.myhuaweicloud.com",
			},
			provider: domain.ProviderHuaweiOBS,
			assert: func(t *testing.T, cfg domain.PublicConfig) {
				t.Helper()
				if cfg.Endpoint != "https://obs.cn-north-4.myhuaweicloud.com" {
					t.Fatalf("huawei public config = %+v", cfg)
				}
			},
		},
		{
			name: "aws s3",
			values: map[string]string{
				"STORAGE_TYPE":   "s3",
				"S3_ACCESS_KEY":  "ak",
				"S3_SECRET_KEY":  "sk",
				"STORAGE_BUCKET": "coze",
				"S3_REGION":      "us-east-1",
				"S3_ENDPOINT":    "https://s3.us-east-1.amazonaws.com",
			},
			provider: domain.ProviderAWSS3,
			assert: func(t *testing.T, cfg domain.PublicConfig) {
				t.Helper()
				if cfg.EndpointOverride != "https://s3.us-east-1.amazonaws.com" {
					t.Fatalf("s3 public config = %+v", cfg)
				}
			},
		},
		{
			name: "tos",
			values: map[string]string{
				"STORAGE_TYPE":   "tos",
				"TOS_ACCESS_KEY": "ak",
				"TOS_SECRET_KEY": "sk",
				"STORAGE_BUCKET": "coze",
				"TOS_REGION":     "cn-beijing",
				"TOS_ENDPOINT":   "https://tos-cn-beijing.volces.com",
			},
			provider: domain.ProviderTOS,
			assert: func(t *testing.T, cfg domain.PublicConfig) {
				t.Helper()
				if cfg.Endpoint != "https://tos-cn-beijing.volces.com" {
					t.Fatalf("tos public config = %+v", cfg)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			env, err := LoadEnvConfig(mapGetenv(test.values), domain.ValidationMode{})
			if err != nil {
				t.Fatalf("LoadEnvConfig() error = %v", err)
			}
			if env.ProviderType != test.provider {
				t.Fatalf("provider = %s, want %s", env.ProviderType, test.provider)
			}
			test.assert(t, env.PublicConfig)
			if env.Credential.AccessKeyID != "ak" || env.Credential.SecretAccessKey != "sk" {
				t.Fatal("credential was not parsed")
			}
		})
	}
}

func TestLoadEnvConfigRejectsInvalidSourceAndProvider(t *testing.T) {
	if _, err := LoadEnvConfig(mapGetenv(map[string]string{
		ObjectStorageConfigSourceEnv: "automatic",
		"STORAGE_TYPE":               "minio",
	}), domain.ValidationMode{}); !errors.Is(err, domain.ErrConfigInvalid) {
		t.Fatalf("invalid source error = %v", err)
	}

	if _, err := LoadEnvConfig(mapGetenv(map[string]string{
		"STORAGE_TYPE": "ftp",
	}), domain.ValidationMode{}); !errors.Is(err, domain.ErrProviderUnsupported) {
		t.Fatalf("invalid provider error = %v", err)
	}
}

func mapGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
