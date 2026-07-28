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
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

func (p ProviderType) Valid() bool {
	switch p {
	case ProviderQiniu, ProviderAliyunOSS, ProviderTencentCOS, ProviderHuaweiOBS, ProviderAWSS3, ProviderMinIO, ProviderTOS:
		return true
	default:
		return false
	}
}

func NormalizeName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", fmt.Errorf("%w: name is required", ErrConfigInvalid)
	}
	if utf8.RuneCountInString(normalized) > 128 {
		return "", fmt.Errorf("%w: name is too long", ErrConfigInvalid)
	}
	return normalized, nil
}

func ValidateProvider(provider ProviderType) error {
	if !provider.Valid() {
		return fmt.Errorf("%w: %s", ErrProviderUnsupported, provider)
	}
	return nil
}

func ValidatePublicConfig(provider ProviderType, input PublicConfig, mode ValidationMode) (PublicConfig, error) {
	if err := ValidateProvider(provider); err != nil {
		return PublicConfig{}, err
	}

	normalized := PublicConfig{
		Bucket:           strings.TrimSpace(input.Bucket),
		Region:           strings.TrimSpace(input.Region),
		Endpoint:         strings.TrimSpace(input.Endpoint),
		EndpointOverride: strings.TrimSpace(input.EndpointOverride),
		ForcePathStyle:   input.ForcePathStyle,
		UseSSL:           input.UseSSL,
		DownloadDomain:   strings.TrimSpace(input.DownloadDomain),
		UseHTTPS:         input.UseHTTPS,
	}
	if err := validateBucket(normalized.Bucket); err != nil {
		return PublicConfig{}, err
	}
	if err := validateEndpointHTTP(normalized.Endpoint, mode); err != nil {
		return PublicConfig{}, err
	}
	if err := validateEndpointHTTP(normalized.EndpointOverride, mode); err != nil {
		return PublicConfig{}, err
	}

	switch provider {
	case ProviderQiniu:
		if err := validateDownloadDomain(normalized.DownloadDomain); err != nil {
			return PublicConfig{}, err
		}
	case ProviderAliyunOSS:
		if normalized.Region == "" {
			return PublicConfig{}, fmt.Errorf("%w: region is required", ErrConfigInvalid)
		}
	case ProviderTencentCOS:
		if normalized.Region == "" {
			return PublicConfig{}, fmt.Errorf("%w: region is required", ErrConfigInvalid)
		}
		if !validTencentBucket(normalized.Bucket) {
			return PublicConfig{}, fmt.Errorf("%w: tencent bucket must include appid", ErrConfigInvalid)
		}
	case ProviderHuaweiOBS:
		if normalized.Region == "" {
			return PublicConfig{}, fmt.Errorf("%w: region is required", ErrConfigInvalid)
		}
		if err := requireSchemedEndpoint(normalized.Endpoint, mode); err != nil {
			return PublicConfig{}, err
		}
	case ProviderAWSS3:
		if normalized.Region == "" {
			return PublicConfig{}, fmt.Errorf("%w: region is required", ErrConfigInvalid)
		}
	case ProviderMinIO:
		var err error
		normalized, err = normalizeMinIOEndpoint(normalized, mode)
		if err != nil {
			return PublicConfig{}, err
		}
	case ProviderTOS:
		if normalized.Region == "" {
			return PublicConfig{}, fmt.Errorf("%w: region is required", ErrConfigInvalid)
		}
		if err := requireSchemedEndpoint(normalized.Endpoint, mode); err != nil {
			return PublicConfig{}, err
		}
	}

	return normalized, nil
}

func ValidateCredentialInput(input CredentialInput) error {
	input.AccessKeyID = strings.TrimSpace(input.AccessKeyID)
	input.SecretAccessKey = strings.TrimSpace(input.SecretAccessKey)
	if input.AccessKeyID == "" && input.SecretAccessKey == "" {
		return nil
	}
	if input.AccessKeyID == "" || input.SecretAccessKey == "" {
		return fmt.Errorf("%w: credential pair is required", ErrConfigInvalid)
	}
	return nil
}

func HasCredentialPair(input CredentialInput) bool {
	return strings.TrimSpace(input.AccessKeyID) != "" && strings.TrimSpace(input.SecretAccessKey) != ""
}

func RuntimeFieldsEqual(left, right Config) bool {
	return left.ProviderType == right.ProviderType &&
		left.PublicConfig == right.PublicConfig &&
		left.CredentialSecret == right.CredentialSecret &&
		left.Active == right.Active
}

func RestartRequired(runtime RuntimeDescriptor, desired *Config) bool {
	if desired == nil {
		return true
	}
	return runtime.ConfigID != desired.ID ||
		runtime.RuntimeRevision != desired.RuntimeRevision ||
		runtime.ProviderType != desired.ProviderType
}

func validateBucket(bucket string) error {
	if bucket == "" {
		return fmt.Errorf("%w: bucket is required", ErrConfigInvalid)
	}
	if strings.ContainsAny(bucket, "/\\\x00\r\n") {
		return fmt.Errorf("%w: bucket contains invalid characters", ErrConfigInvalid)
	}
	return nil
}

func validateEndpointHTTP(endpoint string, mode ValidationMode) error {
	if endpoint == "" || mode.AllowHTTP {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(endpoint), "http://") {
		return fmt.Errorf("%w: http endpoint is not allowed", ErrConfigInvalid)
	}
	return nil
}

func validateDownloadDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("%w: download domain is required", ErrConfigInvalid)
	}
	if strings.Contains(domain, "://") || strings.ContainsAny(domain, "/\\\x00\r\n") {
		return fmt.Errorf("%w: download domain must not include scheme or path", ErrConfigInvalid)
	}
	return nil
}

func validTencentBucket(bucket string) bool {
	index := strings.LastIndex(bucket, "-")
	if index <= 0 || index == len(bucket)-1 {
		return false
	}
	appid := bucket[index+1:]
	for _, r := range appid {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func requireSchemedEndpoint(endpoint string, mode ValidationMode) error {
	if endpoint == "" {
		return fmt.Errorf("%w: endpoint is required", ErrConfigInvalid)
	}
	lower := strings.ToLower(endpoint)
	if strings.HasPrefix(lower, "https://") {
		return nil
	}
	if mode.AllowHTTP && strings.HasPrefix(lower, "http://") {
		return nil
	}
	return fmt.Errorf("%w: endpoint must use https", ErrConfigInvalid)
}

func normalizeMinIOEndpoint(config PublicConfig, mode ValidationMode) (PublicConfig, error) {
	if config.Endpoint == "" {
		return PublicConfig{}, fmt.Errorf("%w: endpoint is required", ErrConfigInvalid)
	}
	lower := strings.ToLower(config.Endpoint)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		parsed, err := url.Parse(config.Endpoint)
		if err != nil || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return PublicConfig{}, fmt.Errorf("%w: endpoint is invalid", ErrConfigInvalid)
		}
		if parsed.Scheme == "http" && !mode.AllowHTTP {
			return PublicConfig{}, fmt.Errorf("%w: http endpoint is not allowed", ErrConfigInvalid)
		}
		config.Endpoint = parsed.Host
		config.UseSSL = parsed.Scheme == "https"
	default:
		if strings.ContainsAny(config.Endpoint, "/\\\x00\r\n") {
			return PublicConfig{}, fmt.Errorf("%w: endpoint is invalid", ErrConfigInvalid)
		}
		if !config.UseSSL && !mode.AllowHTTP {
			return PublicConfig{}, fmt.Errorf("%w: http endpoint is not allowed", ErrConfigInvalid)
		}
	}
	return config, nil
}
