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
	"fmt"
	"strconv"
	"strings"

	domain "github.com/coze-dev/coze-studio/backend/domain/storageconfig"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

const (
	ObjectStorageConfigSourceEnv = "OBJECT_STORAGE_CONFIG_SOURCE"
	ObjectStorageSourceDatabase  = "database"
	ObjectStorageSourceEnv       = "env"

	qiniuAccessKeyEnv      = "QINIU_ACCESS_KEY"
	qiniuSecretKeyEnv      = "QINIU_SECRET_KEY"
	qiniuBucketEnv         = "QINIU_BUCKET"
	qiniuDownloadDomainEnv = "QINIU_DOWNLOAD_DOMAIN"
	qiniuRegionEnv         = "QINIU_REGION"
	qiniuUseHTTPSEnv       = "QINIU_USE_HTTPS"

	aliyunOSSAccessKeyEnv        = "ALIYUN_OSS_ACCESS_KEY"
	aliyunOSSSecretKeyEnv        = "ALIYUN_OSS_SECRET_KEY"
	aliyunOSSBucketEnv           = "ALIYUN_OSS_BUCKET"
	aliyunOSSRegionEnv           = "ALIYUN_OSS_REGION"
	aliyunOSSEndpointOverrideEnv = "ALIYUN_OSS_ENDPOINT_OVERRIDE"

	tencentCOSAccessKeyEnv        = "TENCENT_COS_ACCESS_KEY"
	tencentCOSSecretKeyEnv        = "TENCENT_COS_SECRET_KEY"
	tencentCOSBucketEnv           = "TENCENT_COS_BUCKET"
	tencentCOSRegionEnv           = "TENCENT_COS_REGION"
	tencentCOSEndpointOverrideEnv = "TENCENT_COS_ENDPOINT_OVERRIDE"

	huaweiOBSAccessKeyEnv = "HUAWEI_OBS_ACCESS_KEY"
	huaweiOBSSecretKeyEnv = "HUAWEI_OBS_SECRET_KEY"
	huaweiOBSBucketEnv    = "HUAWEI_OBS_BUCKET"
	huaweiOBSRegionEnv    = "HUAWEI_OBS_REGION"
	huaweiOBSEndpointEnv  = "HUAWEI_OBS_ENDPOINT"

	s3ForcePathStyleEnv = "S3_FORCE_PATH_STYLE"
)

type EnvConfig struct {
	Source       domain.RuntimeSource
	ProviderType domain.ProviderType
	PublicConfig domain.PublicConfig
	Credential   domain.CredentialInput
}

func LoadEnvConfig(getenv func(string) string, mode domain.ValidationMode) (EnvConfig, error) {
	if getenv == nil {
		return EnvConfig{}, domain.ErrConfigInvalid
	}
	source, err := LoadConfigSource(getenv)
	if err != nil {
		return EnvConfig{}, err
	}
	provider, publicConfig, credential, err := loadProviderEnvConfig(getenv)
	if err != nil {
		return EnvConfig{}, err
	}
	normalizedConfig, err := domain.ValidatePublicConfig(provider, publicConfig, mode)
	if err != nil {
		return EnvConfig{}, err
	}
	credential = domain.NormalizeCredentialInput(credential)
	if err = domain.ValidateCredentialInput(credential); err != nil {
		return EnvConfig{}, err
	}
	if !domain.HasCredentialPair(credential) {
		return EnvConfig{}, fmt.Errorf("%w: credential pair is required", domain.ErrConfigInvalid)
	}
	return EnvConfig{
		Source:       source,
		ProviderType: provider,
		PublicConfig: normalizedConfig,
		Credential:   credential,
	}, nil
}

func LoadConfigSource(getenv func(string) string) (domain.RuntimeSource, error) {
	if getenv == nil {
		return "", domain.ErrConfigInvalid
	}
	source := strings.ToLower(strings.TrimSpace(getenv(ObjectStorageConfigSourceEnv)))
	if source == "" || source == ObjectStorageSourceDatabase {
		return domain.RuntimeSourceDatabase, nil
	}
	if source == ObjectStorageSourceEnv {
		return domain.RuntimeSourceEnvRescue, nil
	}
	return "", fmt.Errorf("%w: object storage config source is invalid", domain.ErrConfigInvalid)
}

func loadProviderEnvConfig(getenv func(string) string) (domain.ProviderType, domain.PublicConfig, domain.CredentialInput, error) {
	storageType := normalizeStorageType(getenv(consts.StorageType))
	switch storageType {
	case "minio":
		useSSL, err := envBool(getenv, "MINIO_USE_SSL", false)
		if err != nil {
			return "", domain.PublicConfig{}, domain.CredentialInput{}, err
		}
		return domain.ProviderMinIO,
			domain.PublicConfig{
				Bucket:   getenv(consts.StorageBucket),
				Endpoint: getenv(consts.MinIOEndpoint),
				UseSSL:   useSSL,
			},
			domain.CredentialInput{
				AccessKeyID:     getenv(consts.MinIOAK),
				SecretAccessKey: getenv(consts.MinIOSK),
			},
			nil
	case "tos":
		return domain.ProviderTOS,
			domain.PublicConfig{
				Bucket:   getenv(consts.StorageBucket),
				Region:   getenv(consts.TOSRegion),
				Endpoint: getenv(consts.TOSEndpoint),
			},
			domain.CredentialInput{
				AccessKeyID:     getenv(consts.TOSAccessKey),
				SecretAccessKey: getenv(consts.TOSSecretKey),
			},
			nil
	case "s3", "aws_s3":
		forcePathStyle, err := envBool(getenv, s3ForcePathStyleEnv, false)
		if err != nil {
			return "", domain.PublicConfig{}, domain.CredentialInput{}, err
		}
		return domain.ProviderAWSS3,
			domain.PublicConfig{
				Bucket:           getenv(consts.StorageBucket),
				Region:           getenv(consts.S3Region),
				EndpointOverride: getenv(consts.S3Endpoint),
				ForcePathStyle:   forcePathStyle,
			},
			domain.CredentialInput{
				AccessKeyID:     getenv(consts.S3AccessKey),
				SecretAccessKey: getenv(consts.S3SecretKey),
			},
			nil
	case "qiniu":
		useHTTPS, err := envBool(getenv, qiniuUseHTTPSEnv, true)
		if err != nil {
			return "", domain.PublicConfig{}, domain.CredentialInput{}, err
		}
		return domain.ProviderQiniu,
			domain.PublicConfig{
				Bucket:         getenv(qiniuBucketEnv),
				Region:         getenv(qiniuRegionEnv),
				DownloadDomain: getenv(qiniuDownloadDomainEnv),
				UseHTTPS:       useHTTPS,
			},
			domain.CredentialInput{
				AccessKeyID:     getenv(qiniuAccessKeyEnv),
				SecretAccessKey: getenv(qiniuSecretKeyEnv),
			},
			nil
	case "aliyun_oss", "aliyunoss", "oss":
		return domain.ProviderAliyunOSS,
			domain.PublicConfig{
				Bucket:           getenv(aliyunOSSBucketEnv),
				Region:           getenv(aliyunOSSRegionEnv),
				EndpointOverride: getenv(aliyunOSSEndpointOverrideEnv),
			},
			domain.CredentialInput{
				AccessKeyID:     getenv(aliyunOSSAccessKeyEnv),
				SecretAccessKey: getenv(aliyunOSSSecretKeyEnv),
			},
			nil
	case "tencent_cos", "tencentcos", "cos":
		return domain.ProviderTencentCOS,
			domain.PublicConfig{
				Bucket:           getenv(tencentCOSBucketEnv),
				Region:           getenv(tencentCOSRegionEnv),
				EndpointOverride: getenv(tencentCOSEndpointOverrideEnv),
			},
			domain.CredentialInput{
				AccessKeyID:     getenv(tencentCOSAccessKeyEnv),
				SecretAccessKey: getenv(tencentCOSSecretKeyEnv),
			},
			nil
	case "huawei_obs", "huaweiobs", "obs":
		return domain.ProviderHuaweiOBS,
			domain.PublicConfig{
				Bucket:   getenv(huaweiOBSBucketEnv),
				Region:   getenv(huaweiOBSRegionEnv),
				Endpoint: getenv(huaweiOBSEndpointEnv),
			},
			domain.CredentialInput{
				AccessKeyID:     getenv(huaweiOBSAccessKeyEnv),
				SecretAccessKey: getenv(huaweiOBSSecretKeyEnv),
			},
			nil
	default:
		return "", domain.PublicConfig{}, domain.CredentialInput{}, fmt.Errorf("%w: %s", domain.ErrProviderUnsupported, storageType)
	}
}

func normalizeStorageType(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
}

func envBool(getenv func(string) string, key string, defaultValue bool) (bool, error) {
	value := strings.TrimSpace(getenv(key))
	if value == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%w: %s must be a boolean", domain.ErrConfigInvalid, key)
	}
	return parsed, nil
}
