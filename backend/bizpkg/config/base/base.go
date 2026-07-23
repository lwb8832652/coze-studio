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

package base

import (
	"context"
	"errors"
	"os"
	"strings"

	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/pkg/envkey"
	"github.com/coze-dev/coze-studio/backend/pkg/kvstore"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/conv"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ternary"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

const (
	baseConfigKey                 = "basic_config"
	systemAdminBootstrapEmailsEnv = "COZE_SYSTEM_ADMIN_EMAILS"
	defaultSiteName               = "NewX AI"
	defaultSiteDescription        = "NewX AI 是面向个人与团队的智能工作空间，让任务、技能和协作沉淀为可复用的成果。"
)

type BaseConfig struct {
	base basicConfigurationStore
}

type basicConfigurationStore interface {
	GetVersioned(context.Context, string, string) (*config.BasicConfiguration, string, error)
	CompareAndSwap(context.Context, string, string, string, *config.BasicConfiguration) (string, error)
}

type BasicConfigurationPatch struct {
	AdminEmails             *string
	DisableUserRegistration *bool
	AllowRegistrationEmail  *string
	PluginConfiguration     *config.PluginConfiguration
	ServerHost              *string
	SiteName                *string
	SiteDescription         *string
	SiteLogoURI             *string
	FaviconURI              *string
}

func (p BasicConfigurationPatch) IsEmpty() bool {
	return p.AdminEmails == nil && p.DisableUserRegistration == nil &&
		p.AllowRegistrationEmail == nil && p.PluginConfiguration == nil &&
		p.ServerHost == nil && p.SiteName == nil &&
		p.SiteDescription == nil && p.SiteLogoURI == nil &&
		p.FaviconURI == nil
}

func NewBaseConfig(db *gorm.DB) *BaseConfig {
	return &BaseConfig{
		base: kvstore.New[config.BasicConfiguration](db),
	}
}

func (c *BaseConfig) GetBaseConfig(ctx context.Context) (*config.BasicConfiguration, error) {
	conf, _, err := c.GetBaseConfigWithRevision(ctx)
	return conf, err
}

func (c *BaseConfig) GetBaseConfigWithRevision(ctx context.Context) (*config.BasicConfiguration, string, error) {
	if c == nil || c.base == nil || ctx == nil {
		return nil, "", errors.New("basic configuration read input is invalid")
	}
	conf, revision, err := c.base.GetVersioned(ctx, consts.BaseConfigNameSpace, baseConfigKey)
	if err != nil {
		if errors.Is(err, kvstore.ErrKeyNotFound) {
			return getBasicConfigurationFromOldConfig(), kvstore.MissingRevision, nil
		}
		return nil, "", err
	}
	if conf == nil {
		return nil, "", errors.New("basic configuration read returned no value")
	}
	return cloneConfiguration(conf), revision, nil
}

func (c *BaseConfig) SaveBaseConfig(ctx context.Context, patch BasicConfigurationPatch, expectedRevision string) (string, error) {
	if c == nil || c.base == nil || ctx == nil || expectedRevision == "" || patch.IsEmpty() {
		return "", errors.New("basic configuration save input is invalid")
	}
	current, currentRevision, err := c.base.GetVersioned(ctx, consts.BaseConfigNameSpace, baseConfigKey)
	if err != nil {
		if !errors.Is(err, kvstore.ErrKeyNotFound) {
			return "", err
		}
		current = getBasicConfigurationFromOldConfig()
		currentRevision = kvstore.MissingRevision
	}
	if current == nil {
		return "", errors.New("basic configuration read returned no value")
	}
	if currentRevision != expectedRevision {
		return "", kvstore.ErrVersionConflict
	}

	toSave := cloneConfiguration(current)
	if patch.AdminEmails != nil {
		toSave.AdminEmails = *patch.AdminEmails
	}
	if patch.DisableUserRegistration != nil {
		toSave.DisableUserRegistration = *patch.DisableUserRegistration
	}
	if patch.AllowRegistrationEmail != nil {
		toSave.AllowRegistrationEmail = *patch.AllowRegistrationEmail
	}
	if patch.PluginConfiguration != nil {
		plugin := *patch.PluginConfiguration
		toSave.PluginConfiguration = &plugin
	}
	if patch.ServerHost != nil {
		toSave.ServerHost = *patch.ServerHost
	}
	if patch.SiteName != nil {
		toSave.SiteName = cloneStringPointer(patch.SiteName)
	}
	if patch.SiteDescription != nil {
		toSave.SiteDescription = cloneStringPointer(patch.SiteDescription)
	}
	if patch.SiteLogoURI != nil {
		toSave.SiteLogoURI = cloneStringPointer(patch.SiteLogoURI)
	}
	if patch.FaviconURI != nil {
		toSave.FaviconURI = cloneStringPointer(patch.FaviconURI)
	}
	return c.base.CompareAndSwap(ctx, consts.BaseConfigNameSpace, baseConfigKey, expectedRevision, toSave)
}

func cloneConfiguration(value *config.BasicConfiguration) *config.BasicConfiguration {
	if value == nil {
		return nil
	}
	cloned := *value
	if value.PluginConfiguration != nil {
		plugin := *value.PluginConfiguration
		cloned.PluginConfiguration = &plugin
	}
	if value.SandboxConfig != nil {
		sandbox := *value.SandboxConfig
		cloned.SandboxConfig = &sandbox
	}
	cloned.SiteName = cloneStringPointer(value.SiteName)
	cloned.SiteDescription = cloneStringPointer(value.SiteDescription)
	cloned.SiteLogoURI = cloneStringPointer(value.SiteLogoURI)
	cloned.FaviconURI = cloneStringPointer(value.FaviconURI)
	return &cloned
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

// GetLegacySandboxConfig is the sole compatibility read path for the retired
// SandboxConfig data source. Callers receive a copy and cannot mutate config
// state in place.
func (c *BaseConfig) GetLegacySandboxConfig(ctx context.Context) (*config.SandboxConfig, error) {
	conf, err := c.GetBaseConfig(ctx)
	if err != nil {
		return nil, err
	}
	if conf == nil || conf.SandboxConfig == nil {
		return nil, nil
	}
	cloned := *conf.SandboxConfig
	return &cloned, nil
}

func getBasicConfigurationFromOldConfig() *config.BasicConfiguration {
	disableUserRegistration := ternary.IFElse(os.Getenv(consts.DisableUserRegistration) == "true", true, false)
	runnerTypeStr := os.Getenv(consts.CodeRunnerType)
	codeRunnerType := ternary.IFElse(runnerTypeStr == "sandbox" || runnerTypeStr == "", config.CodeRunnerType_Sandbox, config.CodeRunnerType_Local)
	timeoutSecondsStr := os.Getenv(consts.CodeRunnerTimeoutSeconds)
	timeoutSeconds := conv.StrToFloat64D(timeoutSecondsStr, 60)
	memoryLimitMbStr := os.Getenv(consts.CodeRunnerMemoryLimitMB)
	memoryLimitMB := conv.StrToInt64D(memoryLimitMbStr, 100)

	var sandboxConfig *config.SandboxConfig
	if legacySandboxEnvironmentPresent() {
		sandboxConfig = &config.SandboxConfig{
			AllowEnv:       os.Getenv(consts.CodeRunnerAllowEnv),
			AllowRead:      os.Getenv(consts.CodeRunnerAllowRead),
			AllowWrite:     os.Getenv(consts.CodeRunnerAllowWrite),
			AllowNet:       os.Getenv(consts.CodeRunnerAllowNet),
			AllowRun:       os.Getenv(consts.CodeRunnerAllowRun),
			AllowFfi:       os.Getenv(consts.CodeRunnerAllowFFI),
			NodeModulesDir: os.Getenv(consts.CodeRunnerNodeModulesDir),
			TimeoutSeconds: timeoutSeconds,
			MemoryLimitMb:  memoryLimitMB,
		}
	}

	const ServerHost = "SERVER_HOST"
	siteName := defaultSiteName
	siteDescription := defaultSiteDescription
	return &config.BasicConfiguration{
		AdminEmails:             strings.TrimSpace(os.Getenv(systemAdminBootstrapEmailsEnv)),
		DisableUserRegistration: disableUserRegistration,
		AllowRegistrationEmail:  os.Getenv(consts.DisableUserRegistration),
		PluginConfiguration: &config.PluginConfiguration{
			CozeSaasPluginEnabled: envkey.GetBoolD("COZE_SAAS_PLUGIN_ENABLED", false),
			CozeAPIToken:          envkey.GetString("COZE_SAAS_API_KEY"),
			CozeSaasAPIBaseURL:    envkey.GetStringD("COZE_SAAS_API_BASE_URL", "https://api.coze.cn"),
		},
		CodeRunnerType:  codeRunnerType,
		ServerHost:      os.Getenv(ServerHost),
		SandboxConfig:   sandboxConfig,
		SiteName:        &siteName,
		SiteDescription: &siteDescription,
	}
}

func legacySandboxEnvironmentPresent() bool {
	for _, key := range []string{
		consts.CodeRunnerType,
		consts.CodeRunnerAllowEnv,
		consts.CodeRunnerAllowRead,
		consts.CodeRunnerAllowWrite,
		consts.CodeRunnerAllowNet,
		consts.CodeRunnerAllowRun,
		consts.CodeRunnerAllowFFI,
		consts.CodeRunnerNodeModulesDir,
		consts.CodeRunnerTimeoutSeconds,
		consts.CodeRunnerMemoryLimitMB,
	} {
		if _, ok := os.LookupEnv(key); ok {
			return true
		}
	}
	return false
}

func (c *BaseConfig) GetServerHost(ctx context.Context) (string, error) {
	cfg, err := c.GetBaseConfig(ctx)
	if err != nil {
		return "", err
	}

	host := cfg.ServerHost
	if host == "" {
		return "http://127.0.0.1:8888", nil
	}

	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host, nil
	}

	return "https://" + host, nil
}
