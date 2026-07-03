/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * You may not use this file except in compliance with the License.
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

package agentthread

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const (
	agentThreadMCPRuntimeEnabledEnv        = "AGENT_THREAD_MCP_RUNTIME_ENABLED"
	agentThreadMCPRuntimeTimeoutMsEnv      = "AGENT_THREAD_MCP_RUNTIME_TIMEOUT_MS"
	agentThreadMCPRuntimeMaxOutputBytesEnv = "AGENT_THREAD_MCP_RUNTIME_MAX_OUTPUT_BYTES"

	agentThreadMCPStdioDryRunEnabledEnv     = "AGENT_THREAD_MCP_STDIO_DRY_RUN_ENABLED"
	agentThreadMCPStdioEinoEnabledEnv       = "AGENT_THREAD_MCP_STDIO_EINO_ENABLED"
	agentThreadMCPStdioWorkdirRootEnv       = "AGENT_THREAD_MCP_STDIO_WORKDIR_ROOT"
	agentThreadMCPStdioWorkerIDEnv          = "AGENT_THREAD_MCP_STDIO_WORKER_ID"
	agentThreadMCPStdioAllowedCommandsEnv   = "AGENT_THREAD_MCP_STDIO_ALLOWED_COMMANDS"
	agentThreadMCPStdioAllowedEnvKeysEnv    = "AGENT_THREAD_MCP_STDIO_ALLOWED_ENV_KEYS"
	agentThreadMCPStdioMaxArgsEnv           = "AGENT_THREAD_MCP_STDIO_MAX_ARGS"
	agentThreadMCPStdioMaxArgBytesEnv       = "AGENT_THREAD_MCP_STDIO_MAX_ARG_BYTES"
	agentThreadMCPStdioMaxEnvVarsEnv        = "AGENT_THREAD_MCP_STDIO_MAX_ENV_VARS"
	agentThreadMCPStdioMaxEnvValueBytesEnv  = "AGENT_THREAD_MCP_STDIO_MAX_ENV_VALUE_BYTES"
	agentThreadMCPStdioLeaseTTLMsEnv        = "AGENT_THREAD_MCP_STDIO_LEASE_TTL_MS"
	agentThreadMCPStdioMaxConfigBytesEnv    = "AGENT_THREAD_MCP_STDIO_MAX_CONFIG_BYTES"
	agentThreadMCPStdioDryRunOutputBytesEnv = "AGENT_THREAD_MCP_STDIO_DRY_RUN_OUTPUT_BYTES"

	agentThreadMCPRemoteEinoEnabledEnv       = "AGENT_THREAD_MCP_REMOTE_EINO_ENABLED"
	agentThreadMCPRemoteAllowedHostsEnv      = "AGENT_THREAD_MCP_REMOTE_ALLOWED_HOSTS"
	agentThreadMCPRemoteAllowInsecureHTTPEnv = "AGENT_THREAD_MCP_REMOTE_ALLOW_HTTP"
	agentThreadMCPRemoteMaxConfigBytesEnv    = "AGENT_THREAD_MCP_REMOTE_MAX_CONFIG_BYTES"
	agentThreadMCPRemoteMaxHeadersEnv        = "AGENT_THREAD_MCP_REMOTE_MAX_HEADERS"
	agentThreadMCPRemoteMaxHeaderBytesEnv    = "AGENT_THREAD_MCP_REMOTE_MAX_HEADER_BYTES"
)

const (
	defaultADKMCPRuntimeStdioMaxArgs          = 16
	defaultADKMCPRuntimeStdioMaxArgBytes      = 4096
	defaultADKMCPRuntimeStdioMaxEnvVars       = 16
	defaultADKMCPRuntimeStdioMaxEnvValueBytes = 4096
)

type ADKMCPRuntimeBootstrapConfig struct {
	Enabled bool

	StdioDryRunEnabled     bool
	StdioEinoEnabled       bool
	StdioWorkdirRoot       string
	StdioWorkerID          string
	StdioAllowedCommands   []string
	StdioAllowedEnvKeys    []string
	StdioMaxArgs           int
	StdioMaxArgBytes       int
	StdioMaxEnvVars        int
	StdioMaxEnvValueBytes  int
	StdioLeaseTTLMillis    int64
	StdioMaxConfigBytes    int
	StdioDryRunOutputBytes int

	RemoteEinoEnabled       bool
	RemoteAllowedHosts      []string
	RemoteAllowInsecureHTTP bool
	RemoteMaxConfigBytes    int
	RemoteMaxHeaders        int
	RemoteMaxHeaderBytes    int

	ExecutorTimeout        time.Duration
	ExecutorMaxOutputBytes int

	nowMillis func() int64
}

type ADKMCPRuntimeBootstrapDependencies struct {
	Resolver        ADKMCPRuntimeServerResolver
	LeaseRepository domainrepo.MCPRuntimeWorkdirLeaseRepository
	IDGen           idgen.IDGenerator
	EventSink       RunEventSink
	AuditRecorder   ADKMCPRuntimeAuditRecorder
	HealthReporter  ADKMCPRuntimeHealthReporter
	OutputOffloader ADKMCPRuntimeOutputOffloader
	Config          ADKMCPRuntimeBootstrapConfig
}

func ADKMCPRuntimeBootstrapConfigFromEnv() (ADKMCPRuntimeBootstrapConfig, error) {
	enabled, err := adkMCPRuntimeBoolEnv(agentThreadMCPRuntimeEnabledEnv, false)
	if err != nil {
		return ADKMCPRuntimeBootstrapConfig{}, err
	}
	stdioDryRunEnabled, err := adkMCPRuntimeBoolEnv(
		agentThreadMCPStdioDryRunEnabledEnv,
		false,
	)
	if err != nil {
		return ADKMCPRuntimeBootstrapConfig{}, err
	}
	stdioEinoEnabled, err := adkMCPRuntimeBoolEnv(
		agentThreadMCPStdioEinoEnabledEnv,
		false,
	)
	if err != nil {
		return ADKMCPRuntimeBootstrapConfig{}, err
	}
	remoteEinoEnabled, err := adkMCPRuntimeBoolEnv(
		agentThreadMCPRemoteEinoEnabledEnv,
		false,
	)
	if err != nil {
		return ADKMCPRuntimeBootstrapConfig{}, err
	}
	remoteAllowInsecureHTTP, err := adkMCPRuntimeBoolEnv(
		agentThreadMCPRemoteAllowInsecureHTTPEnv,
		false,
	)
	if err != nil {
		return ADKMCPRuntimeBootstrapConfig{}, err
	}

	config := ADKMCPRuntimeBootstrapConfig{
		Enabled:                 enabled,
		StdioDryRunEnabled:      stdioDryRunEnabled,
		StdioEinoEnabled:        stdioEinoEnabled,
		StdioWorkdirRoot:        strings.TrimSpace(os.Getenv(agentThreadMCPStdioWorkdirRootEnv)),
		StdioWorkerID:           strings.TrimSpace(os.Getenv(agentThreadMCPStdioWorkerIDEnv)),
		StdioAllowedCommands:    adkMCPRuntimeListEnv(agentThreadMCPStdioAllowedCommandsEnv),
		StdioAllowedEnvKeys:     adkMCPRuntimeListEnv(agentThreadMCPStdioAllowedEnvKeysEnv),
		StdioMaxArgs:            defaultADKMCPRuntimeStdioMaxArgs,
		StdioMaxArgBytes:        defaultADKMCPRuntimeStdioMaxArgBytes,
		StdioMaxEnvVars:         defaultADKMCPRuntimeStdioMaxEnvVars,
		StdioMaxEnvValueBytes:   defaultADKMCPRuntimeStdioMaxEnvValueBytes,
		StdioLeaseTTLMillis:     defaultADKMCPRuntimeStdioWorkdirLeaseTTLMillis,
		StdioMaxConfigBytes:     defaultADKMCPRuntimeStdioMaxConfigBytes,
		StdioDryRunOutputBytes:  defaultADKMCPRuntimeStdioDryRunMaxOutputBytes,
		RemoteEinoEnabled:       remoteEinoEnabled,
		RemoteAllowedHosts:      adkMCPRuntimeListEnv(agentThreadMCPRemoteAllowedHostsEnv),
		RemoteAllowInsecureHTTP: remoteAllowInsecureHTTP,
		RemoteMaxConfigBytes:    defaultADKMCPRuntimeRemoteMaxConfigBytes,
		RemoteMaxHeaders:        defaultADKMCPRuntimeRemoteMaxHeaders,
		RemoteMaxHeaderBytes:    defaultADKMCPRuntimeRemoteMaxHeaderBytes,
		ExecutorTimeout:         defaultADKMCPRuntimeExecutorTimeout,
		ExecutorMaxOutputBytes:  defaultADKMCPRuntimeExecutorMaxOutputBytes,
	}

	if err := adkMCPRuntimeParseBootstrapIntegers(&config); err != nil {
		return ADKMCPRuntimeBootstrapConfig{}, err
	}
	if err := config.validate(); err != nil {
		return ADKMCPRuntimeBootstrapConfig{}, err
	}

	return config, nil
}

func NewADKMCPRuntimeToolExecutorFromConfig(
	deps ADKMCPRuntimeBootstrapDependencies,
) ADKMCPRuntimeToolExecutor {
	config := deps.Config.withDefaults()
	if !config.Enabled || !config.hasEnabledMCPRuntimeTransport() {
		return nil
	}
	if deps.Resolver == nil {
		return nil
	}
	if config.stdioEnabled() && (deps.LeaseRepository == nil || deps.IDGen == nil) {
		return nil
	}
	if err := config.validate(); err != nil {
		return nil
	}

	routerOptions := ADKMCPRuntimeTransportRouterOptions{}
	if config.stdioEnabled() {
		routerOptions.Stdio = newADKMCPRuntimeStdioTransportFromBootstrap(config, deps)
	}
	if config.RemoteEinoEnabled {
		remote := newADKMCPRuntimeRemoteTransportFromBootstrap(config)
		routerOptions.SSE = remote
		routerOptions.StreamableHTTP = remote
	}
	router := NewADKMCPRuntimeTransportRouter(routerOptions)

	return NewADKMCPRuntimeExecutor(
		deps.Resolver,
		router,
		WithADKMCPRuntimeExecutorTimeout(config.ExecutorTimeout),
		WithADKMCPRuntimeExecutorMaxOutputBytes(config.ExecutorMaxOutputBytes),
		WithADKMCPRuntimeExecutorEventSink(deps.EventSink),
		WithADKMCPRuntimeExecutorAuditRecorder(deps.AuditRecorder),
		WithADKMCPRuntimeExecutorHealthReporter(
			newADKMCPRuntimeHealthReporterFromEnv(deps.HealthReporter),
		),
		WithADKMCPRuntimeExecutorOutputOffloader(deps.OutputOffloader),
	)
}

func newADKMCPRuntimeHealthReporterFromEnv(
	base ADKMCPRuntimeHealthReporter,
) ADKMCPRuntimeHealthReporter {
	collector := NewRuntimePrometheusMetricsCollectorFromEnv()
	if collector == nil {
		return base
	}
	return newADKMCPRuntimeHealthReporterWithMetrics(base, collector)
}

func adkMCPRuntimeParseBootstrapIntegers(
	config *ADKMCPRuntimeBootstrapConfig,
) error {
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPStdioMaxArgsEnv,
		config.StdioMaxArgs,
	); err != nil {
		return err
	} else {
		config.StdioMaxArgs = value
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPStdioMaxArgBytesEnv,
		config.StdioMaxArgBytes,
	); err != nil {
		return err
	} else {
		config.StdioMaxArgBytes = value
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPStdioMaxEnvVarsEnv,
		config.StdioMaxEnvVars,
	); err != nil {
		return err
	} else {
		config.StdioMaxEnvVars = value
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPStdioMaxEnvValueBytesEnv,
		config.StdioMaxEnvValueBytes,
	); err != nil {
		return err
	} else {
		config.StdioMaxEnvValueBytes = value
	}
	if value, err := adkMCPRuntimePositiveInt64Env(
		agentThreadMCPStdioLeaseTTLMsEnv,
		config.StdioLeaseTTLMillis,
	); err != nil {
		return err
	} else {
		config.StdioLeaseTTLMillis = value
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPStdioMaxConfigBytesEnv,
		config.StdioMaxConfigBytes,
	); err != nil {
		return err
	} else {
		config.StdioMaxConfigBytes = value
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPStdioDryRunOutputBytesEnv,
		config.StdioDryRunOutputBytes,
	); err != nil {
		return err
	} else {
		config.StdioDryRunOutputBytes = value
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPRemoteMaxConfigBytesEnv,
		config.RemoteMaxConfigBytes,
	); err != nil {
		return err
	} else {
		config.RemoteMaxConfigBytes = value
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPRemoteMaxHeadersEnv,
		config.RemoteMaxHeaders,
	); err != nil {
		return err
	} else {
		config.RemoteMaxHeaders = value
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPRemoteMaxHeaderBytesEnv,
		config.RemoteMaxHeaderBytes,
	); err != nil {
		return err
	} else {
		config.RemoteMaxHeaderBytes = value
	}
	if value, err := adkMCPRuntimePositiveInt64Env(
		agentThreadMCPRuntimeTimeoutMsEnv,
		int64(config.ExecutorTimeout/time.Millisecond),
	); err != nil {
		return err
	} else {
		config.ExecutorTimeout = time.Duration(value) * time.Millisecond
	}
	if value, err := adkMCPRuntimePositiveIntEnv(
		agentThreadMCPRuntimeMaxOutputBytesEnv,
		config.ExecutorMaxOutputBytes,
	); err != nil {
		return err
	} else {
		config.ExecutorMaxOutputBytes = value
	}

	return nil
}

func (c ADKMCPRuntimeBootstrapConfig) validate() error {
	if !c.Enabled {
		return nil
	}
	if c.StdioDryRunEnabled && c.StdioEinoEnabled {
		return fmt.Errorf(
			"%s and %s cannot both be true",
			agentThreadMCPStdioDryRunEnabledEnv,
			agentThreadMCPStdioEinoEnabledEnv,
		)
	}
	if !c.hasEnabledMCPRuntimeTransport() {
		return nil
	}
	if c.ExecutorTimeout <= 0 {
		return fmt.Errorf("%s must be positive", agentThreadMCPRuntimeTimeoutMsEnv)
	}
	if c.ExecutorMaxOutputBytes <= 0 {
		return fmt.Errorf("%s must be positive", agentThreadMCPRuntimeMaxOutputBytesEnv)
	}
	if c.RemoteEinoEnabled {
		if len(adkMCPRuntimeStringSet(c.RemoteAllowedHosts)) == 0 {
			return fmt.Errorf("%s is required", agentThreadMCPRemoteAllowedHostsEnv)
		}
		if c.RemoteMaxConfigBytes <= 0 {
			return fmt.Errorf("%s must be positive", agentThreadMCPRemoteMaxConfigBytesEnv)
		}
		if c.RemoteMaxHeaders <= 0 {
			return fmt.Errorf("%s must be positive", agentThreadMCPRemoteMaxHeadersEnv)
		}
		if c.RemoteMaxHeaderBytes <= 0 {
			return fmt.Errorf("%s must be positive", agentThreadMCPRemoteMaxHeaderBytesEnv)
		}
	}
	if !c.stdioEnabled() {
		return nil
	}
	if strings.TrimSpace(c.StdioWorkdirRoot) == "" ||
		!filepath.IsAbs(strings.TrimSpace(c.StdioWorkdirRoot)) {
		return fmt.Errorf("%s must be an absolute path", agentThreadMCPStdioWorkdirRootEnv)
	}
	if strings.TrimSpace(c.StdioWorkerID) == "" {
		return fmt.Errorf("%s is required", agentThreadMCPStdioWorkerIDEnv)
	}
	if len(adkMCPRuntimeStringSet(c.StdioAllowedCommands)) == 0 {
		return fmt.Errorf("%s is required", agentThreadMCPStdioAllowedCommandsEnv)
	}
	if c.StdioMaxArgs < 0 {
		return fmt.Errorf("%s must be non-negative", agentThreadMCPStdioMaxArgsEnv)
	}
	if c.StdioMaxArgBytes <= 0 {
		return fmt.Errorf("%s must be positive", agentThreadMCPStdioMaxArgBytesEnv)
	}
	if c.StdioMaxEnvVars < 0 {
		return fmt.Errorf("%s must be non-negative", agentThreadMCPStdioMaxEnvVarsEnv)
	}
	if c.StdioMaxEnvValueBytes <= 0 {
		return fmt.Errorf("%s must be positive", agentThreadMCPStdioMaxEnvValueBytesEnv)
	}
	if c.StdioLeaseTTLMillis <= 0 {
		return fmt.Errorf("%s must be positive", agentThreadMCPStdioLeaseTTLMsEnv)
	}
	if c.StdioMaxConfigBytes <= 0 {
		return fmt.Errorf("%s must be positive", agentThreadMCPStdioMaxConfigBytesEnv)
	}
	if c.StdioDryRunOutputBytes <= 0 {
		return fmt.Errorf("%s must be positive", agentThreadMCPStdioDryRunOutputBytesEnv)
	}

	return nil
}

func (c ADKMCPRuntimeBootstrapConfig) withDefaults() ADKMCPRuntimeBootstrapConfig {
	if c.StdioMaxArgs == 0 {
		c.StdioMaxArgs = defaultADKMCPRuntimeStdioMaxArgs
	}
	if c.StdioMaxArgBytes == 0 {
		c.StdioMaxArgBytes = defaultADKMCPRuntimeStdioMaxArgBytes
	}
	if c.StdioMaxEnvVars == 0 {
		c.StdioMaxEnvVars = defaultADKMCPRuntimeStdioMaxEnvVars
	}
	if c.StdioMaxEnvValueBytes == 0 {
		c.StdioMaxEnvValueBytes = defaultADKMCPRuntimeStdioMaxEnvValueBytes
	}
	if c.StdioLeaseTTLMillis == 0 {
		c.StdioLeaseTTLMillis = defaultADKMCPRuntimeStdioWorkdirLeaseTTLMillis
	}
	if c.StdioMaxConfigBytes == 0 {
		c.StdioMaxConfigBytes = defaultADKMCPRuntimeStdioMaxConfigBytes
	}
	if c.StdioDryRunOutputBytes == 0 {
		c.StdioDryRunOutputBytes = defaultADKMCPRuntimeStdioDryRunMaxOutputBytes
	}
	if c.RemoteMaxConfigBytes == 0 {
		c.RemoteMaxConfigBytes = defaultADKMCPRuntimeRemoteMaxConfigBytes
	}
	if c.RemoteMaxHeaders == 0 {
		c.RemoteMaxHeaders = defaultADKMCPRuntimeRemoteMaxHeaders
	}
	if c.RemoteMaxHeaderBytes == 0 {
		c.RemoteMaxHeaderBytes = defaultADKMCPRuntimeRemoteMaxHeaderBytes
	}
	if c.ExecutorTimeout == 0 {
		c.ExecutorTimeout = defaultADKMCPRuntimeExecutorTimeout
	}
	if c.ExecutorMaxOutputBytes == 0 {
		c.ExecutorMaxOutputBytes = defaultADKMCPRuntimeExecutorMaxOutputBytes
	}

	return c
}

func (c ADKMCPRuntimeBootstrapConfig) hasEnabledMCPRuntimeTransport() bool {
	return c.stdioEnabled() || c.RemoteEinoEnabled
}

func (c ADKMCPRuntimeBootstrapConfig) stdioEnabled() bool {
	return c.StdioDryRunEnabled || c.StdioEinoEnabled
}

func newADKMCPRuntimeStdioTransportFromBootstrap(
	config ADKMCPRuntimeBootstrapConfig,
	deps ADKMCPRuntimeBootstrapDependencies,
) *ADKMCPRuntimeStdioTransport {
	options := ADKMCPRuntimeStdioRuntimeTransportOptions{
		WorkdirRoot:      config.StdioWorkdirRoot,
		LeaseRepository:  deps.LeaseRepository,
		IDGen:            deps.IDGen,
		WorkerID:         config.StdioWorkerID,
		LeaseTTLMillis:   config.StdioLeaseTTLMillis,
		AllowedCommands:  config.StdioAllowedCommands,
		AllowedEnvKeys:   config.StdioAllowedEnvKeys,
		MaxArgs:          config.StdioMaxArgs,
		MaxArgBytes:      config.StdioMaxArgBytes,
		MaxEnvVars:       config.StdioMaxEnvVars,
		MaxEnvValueBytes: config.StdioMaxEnvValueBytes,
		MaxConfigBytes:   config.StdioMaxConfigBytes,
		DirMode:          0,
		NowMillis:        config.nowMillis,
		Runner:           nil,
	}
	if config.StdioEinoEnabled {
		options.Runner = NewADKMCPRuntimeStdioEinoRunner(
			ADKMCPRuntimeStdioEinoRunnerOptions{
				ClientFactory: NewADKMCPRuntimeStdioEinoMCPClientFactory(
					ADKMCPRuntimeStdioEinoMCPClientFactoryOptions{},
				),
				ToolProvider:   &ADKMCPRuntimeStdioEinoMCPToolProvider{},
				MaxOutputBytes: config.ExecutorMaxOutputBytes,
			},
		)

		return NewADKMCPRuntimeStdioRuntimeTransport(options)
	}

	options.Runner = NewADKMCPRuntimeStdioDryRunRunner(
		ADKMCPRuntimeStdioDryRunRunnerOptions{
			MaxOutputBytes: config.StdioDryRunOutputBytes,
		},
	)

	return NewADKMCPRuntimeStdioRuntimeTransport(options)
}

func newADKMCPRuntimeRemoteTransportFromBootstrap(
	config ADKMCPRuntimeBootstrapConfig,
) *ADKMCPRuntimeRemoteTransport {
	return NewADKMCPRuntimeRemoteTransport(
		ADKMCPRuntimeRemoteTransportOptions{
			Runner: NewADKMCPRuntimeRemoteEinoRunner(
				ADKMCPRuntimeRemoteEinoRunnerOptions{
					ClientFactory: NewADKMCPRuntimeRemoteEinoMCPClientFactory(
						ADKMCPRuntimeRemoteEinoMCPClientFactoryOptions{
							Timeout: config.ExecutorTimeout,
						},
					),
					ToolProvider:   &ADKMCPRuntimeRemoteEinoMCPToolProvider{},
					MaxOutputBytes: config.ExecutorMaxOutputBytes,
					Timeout:        config.ExecutorTimeout,
				},
			),
			AllowedHosts:      config.RemoteAllowedHosts,
			AllowInsecureHTTP: config.RemoteAllowInsecureHTTP,
			MaxConfigBytes:    config.RemoteMaxConfigBytes,
			MaxHeaders:        config.RemoteMaxHeaders,
			MaxHeaderBytes:    config.RemoteMaxHeaderBytes,
		},
	)
}

func adkMCPRuntimeBoolEnv(key string, defaultValue bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}

	return parsed, nil
}

func adkMCPRuntimePositiveIntEnv(key string, defaultValue int) (int, error) {
	value, err := adkMCPRuntimePositiveInt64Env(key, int64(defaultValue))
	if err != nil {
		return 0, err
	}
	if value > int64(^uint(0)>>1) {
		return 0, fmt.Errorf("%s is out of range", key)
	}

	return int(value), nil
}

func adkMCPRuntimePositiveInt64Env(key string, defaultValue int64) (int64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}

	return parsed, nil
}

func adkMCPRuntimeListEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}

	return values
}
