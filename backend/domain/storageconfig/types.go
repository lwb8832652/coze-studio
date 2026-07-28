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
	ID               uint64
	Name             string
	ProviderType     ProviderType
	PublicConfig     PublicConfig
	CredentialSecret string
	Active           bool
	Health           Health
	Version          uint64
	RuntimeRevision  uint64
	CreatedAt        time.Time
	UpdatedAt        time.Time
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
