// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	toolmodel "github.com/coze-dev/coze-studio/backend/api/model/workbench/tool"
	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
	"github.com/coze-dev/coze-studio/backend/application/mcptool"
)

const (
	mcpManagementSafeSummary = `{"result":"completed"}`

	defaultMCPManagementRemoteMaxConfigBytes  = 16 << 10
	defaultMCPManagementRemoteMaxHeaders      = 16
	defaultMCPManagementRemoteMaxHeaderBytes  = 4096
	defaultMCPManagementStdioMaxConfigBytes   = 16 << 10
	defaultMCPManagementStdioMaxArgs          = 16
	defaultMCPManagementStdioMaxArgBytes      = 4096
	defaultMCPManagementStdioMaxEnvVars       = 16
	defaultMCPManagementStdioMaxEnvValueBytes = 4096
	defaultMCPManagementTimeout               = 30 * time.Second
)

var ErrMCPManagementRuntimeFailed = errors.New("MCP management runtime request failed")

type mcpManagementServerResolver interface {
	ResolveADKMCPRuntimeServer(
		ctx context.Context,
		serverID int64,
	) (*toolmodel.MCPToolServer, error)
}

type mcpManagementRuntimeBinder interface {
	BindManagementRuntime(
		executor mcptool.RuntimeExecutor,
		discoverer mcptool.CapabilityDiscoverer,
	) error
}

type mcpManagementRuntimeTarget interface {
	mcpManagementServerResolver
	mcpManagementRuntimeBinder
}

type mcpManagementRuntime struct {
	resolver          mcpManagementServerResolver
	sessionFactory    mcpruntime.SessionFactory
	discoveryLimits   mcpruntime.DiscoveryLimits
	managementFactory *mcpruntime.ProductionSessionFactory
}

func newMCPManagementRuntime(
	resolver mcpManagementServerResolver,
	sessionFactory mcpruntime.SessionFactory,
	discoveryLimits mcpruntime.DiscoveryLimits,
) *mcpManagementRuntime {
	return &mcpManagementRuntime{
		resolver:        resolver,
		sessionFactory:  sessionFactory,
		discoveryLimits: discoveryLimits,
	}
}

func (r *mcpManagementRuntime) ExecuteMCPTool(
	ctx context.Context,
	call mcptool.RuntimeToolCall,
) (result *mcptool.RuntimeToolResult, returnErr error) {
	startedAt := time.Now()
	server, arguments, err := r.resolveToolCall(ctx, call)
	if err != nil {
		return nil, ErrMCPManagementRuntimeFailed
	}
	if r == nil || r.sessionFactory == nil {
		return nil, ErrMCPManagementRuntimeFailed
	}
	session, err := r.sessionFactory.Open(ctx, mcpruntime.Connection{
		ServerType: server.ServerType,
		Config:     server.Config,
		Auth:       server.Auth,
	})
	if err != nil || session == nil {
		return nil, ErrMCPManagementRuntimeFailed
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && returnErr == nil {
			result = nil
			returnErr = ErrMCPManagementRuntimeFailed
		}
	}()

	callResult, err := session.CallTool(ctx, strings.TrimSpace(call.ToolName), arguments)
	if err != nil || callResult.IsError {
		return nil, ErrMCPManagementRuntimeFailed
	}
	latency := time.Since(startedAt).Milliseconds()
	if latency < 0 {
		latency = 0
	}
	return &mcptool.RuntimeToolResult{
		Status:    "success",
		Output:    mcpManagementSafeSummary,
		LatencyMs: latency,
	}, nil
}

func (r *mcpManagementRuntime) resolveToolCall(
	ctx context.Context,
	call mcptool.RuntimeToolCall,
) (*toolmodel.MCPToolServer, map[string]any, error) {
	if r == nil || r.resolver == nil || call.SpaceID <= 0 || call.ServerID <= 0 ||
		strings.TrimSpace(call.ToolName) == "" {
		return nil, nil, ErrMCPManagementRuntimeFailed
	}
	server, err := r.resolver.ResolveADKMCPRuntimeServer(ctx, call.ServerID)
	if err != nil || server == nil || server.ServerID != call.ServerID ||
		server.SpaceID != call.SpaceID || !server.Enabled {
		return nil, nil, ErrMCPManagementRuntimeFailed
	}
	toolName := strings.TrimSpace(call.ToolName)
	toolPersisted := false
	for _, tool := range server.Tools {
		if tool != nil && strings.TrimSpace(tool.Name) == toolName {
			toolPersisted = true
			break
		}
	}
	if !toolPersisted {
		return nil, nil, ErrMCPManagementRuntimeFailed
	}
	var arguments map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(call.Arguments)), &arguments); err != nil || arguments == nil {
		return nil, nil, ErrMCPManagementRuntimeFailed
	}
	return server, arguments, nil
}

func (r *mcpManagementRuntime) Discover(
	ctx context.Context,
	connection mcptool.MCPServerConnection,
) (result *mcptool.DiscoveredCapabilities, returnErr error) {
	if r == nil || r.sessionFactory == nil {
		return nil, ErrMCPManagementRuntimeFailed
	}
	session, err := r.sessionFactory.Open(ctx, mcpruntime.Connection{
		ServerType: connection.ServerType,
		Config:     connection.Config,
		Auth:       connection.Auth,
	})
	if err != nil || session == nil {
		return nil, ErrMCPManagementRuntimeFailed
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && returnErr == nil {
			result = nil
			returnErr = ErrMCPManagementRuntimeFailed
		}
	}()

	discovered, err := mcpruntime.Discover(ctx, session, r.discoveryLimits)
	if err != nil || discovered == nil {
		return nil, ErrMCPManagementRuntimeFailed
	}
	return mapMCPManagementCapabilities(discovered), nil
}

func mapMCPManagementCapabilities(
	discovered *mcpruntime.DiscoveredCapabilities,
) *mcptool.DiscoveredCapabilities {
	result := &mcptool.DiscoveredCapabilities{
		Tools:     make([]*toolmodel.MCPToolDefinition, 0, len(discovered.Tools)),
		Resources: make([]*toolmodel.MCPResource, 0, len(discovered.Resources)),
		Prompts:   make([]*toolmodel.MCPPrompt, 0, len(discovered.Prompts)),
	}
	for _, tool := range discovered.Tools {
		result.Tools = append(result.Tools, &toolmodel.MCPToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: string(tool.InputSchema),
		})
	}
	for _, resource := range discovered.Resources {
		result.Resources = append(result.Resources, &toolmodel.MCPResource{
			URI:         resource.URI,
			Name:        resource.Name,
			Description: resource.Description,
			MIMEType:    resource.MIMEType,
		})
	}
	for _, prompt := range discovered.Prompts {
		arguments := make([]*toolmodel.MCPPromptArgument, 0, len(prompt.Arguments))
		for _, argument := range prompt.Arguments {
			arguments = append(arguments, &toolmodel.MCPPromptArgument{
				Name:        argument.Name,
				Description: argument.Description,
				Required:    argument.Required,
			})
		}
		result.Prompts = append(result.Prompts, &toolmodel.MCPPrompt{
			Name:        prompt.Name,
			Description: prompt.Description,
			Arguments:   arguments,
		})
	}
	return result
}

func bindMCPManagementRuntime(
	target mcpManagementRuntimeTarget,
	config agentthread.ADKMCPRuntimeBootstrapConfig,
	workdirManagers ...*mcpruntime.SafeWorkdirManager,
) (*mcpManagementRuntime, error) {
	if target == nil {
		return nil, ErrMCPManagementRuntimeFailed
	}
	adapter, err := newProductionMCPManagementRuntime(target, config, workdirManagers...)
	if err != nil {
		return nil, err
	}
	if adapter == nil {
		return nil, nil
	}
	if err := target.BindManagementRuntime(adapter, adapter); err != nil {
		_ = adapter.Shutdown(context.Background())
		return nil, fmt.Errorf("bind MCP management runtime: %w", err)
	}
	return adapter, nil
}

func newProductionMCPManagementRuntime(
	resolver mcpManagementServerResolver,
	config agentthread.ADKMCPRuntimeBootstrapConfig,
	workdirManagers ...*mcpruntime.SafeWorkdirManager,
) (*mcpManagementRuntime, error) {
	if !config.Enabled || (!config.StdioEinoEnabled && !config.RemoteEinoEnabled) {
		return nil, nil
	}
	policy := managementPolicyFromADKConfig(config)
	var workdirManager *mcpruntime.SafeWorkdirManager
	if len(workdirManagers) > 1 {
		return nil, ErrMCPManagementRuntimeFailed
	}
	if len(workdirManagers) == 1 {
		workdirManager = workdirManagers[0]
	}
	factory, err := mcpruntime.NewProductionSessionFactory(mcpruntime.FactoryOptions{
		Policy:         policy,
		WorkdirManager: workdirManager,
		ExecutionMode: mcpruntime.NewStdioExecutionMode(
			config.AppEnv,
			config.StdioDebugHostExecutionEnabled,
		),
	})
	if err != nil {
		return nil, fmt.Errorf("configure MCP management runtime: %w", err)
	}
	adapter := newMCPManagementRuntime(resolver, factory, mcpruntime.DefaultDiscoveryLimits())
	adapter.managementFactory = factory
	return adapter, nil
}

type mcpManagementRuntimeLifecycle struct {
	runtime        *mcpManagementRuntime
	workdirManager *mcpruntime.SafeWorkdirManager
}

func (l *mcpManagementRuntimeLifecycle) Shutdown(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if l.runtime != nil {
		if err := l.runtime.Shutdown(ctx); err != nil {
			return err
		}
	}
	if l.workdirManager != nil {
		return l.workdirManager.Close()
	}
	return nil
}

func newMCPRuntimeSharedWorkdir(
	config agentthread.ADKMCPRuntimeBootstrapConfig,
) (*mcpruntime.SafeWorkdirManager, *agentthread.ADKMCPRuntimeStdioFilesystemWorkdirPreparer, error) {
	if !config.Enabled || (!config.StdioEinoEnabled && !config.StdioDryRunEnabled) {
		return nil, nil, nil
	}
	manager, err := mcpruntime.NewSafeWorkdirManager(mcpruntime.SafeWorkdirOptions{
		Root: strings.TrimSpace(config.StdioWorkdirRoot),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("configure MCP workdir manager: %w", err)
	}
	preparer := agentthread.NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		agentthread.ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{
			Root:    strings.TrimSpace(config.StdioWorkdirRoot),
			Manager: manager,
		},
	)
	if !preparer.Valid() {
		_ = manager.Close()
		return nil, nil, ErrMCPManagementRuntimeFailed
	}
	return manager, preparer, nil
}

func (r *mcpManagementRuntime) Shutdown(ctx context.Context) error {
	if r == nil || r.managementFactory == nil {
		return nil
	}
	return r.managementFactory.Shutdown(ctx)
}

func managementPolicyFromADKConfig(
	config agentthread.ADKMCPRuntimeBootstrapConfig,
) mcpruntime.Policy {
	timeout := config.ExecutorTimeout
	if timeout <= 0 {
		timeout = defaultMCPManagementTimeout
	}
	return mcpruntime.Policy{
		RemoteEnabled:               config.RemoteEinoEnabled,
		RemoteAllowedHosts:          append([]string(nil), config.RemoteAllowedHosts...),
		AllowInsecureHTTP:           config.RemoteAllowInsecureHTTP,
		RemoteMaxConfigBytes:        positiveOr(config.RemoteMaxConfigBytes, defaultMCPManagementRemoteMaxConfigBytes),
		RemoteMaxHeaders:            positiveOr(config.RemoteMaxHeaders, defaultMCPManagementRemoteMaxHeaders),
		RemoteMaxHeaderBytes:        positiveOr(config.RemoteMaxHeaderBytes, defaultMCPManagementRemoteMaxHeaderBytes),
		HTTPTimeout:                 timeout,
		StdioEnabled:                config.StdioEinoEnabled,
		StdioWorkdirRoot:            strings.TrimSpace(config.StdioWorkdirRoot),
		StdioAllowedCommands:        append([]string(nil), config.StdioAllowedCommands...),
		StdioAllowedNpxPackages:     append([]string(nil), config.StdioAllowedNpxPackages...),
		StdioAllowedUVXPackages:     append([]string(nil), config.StdioAllowedUVXPackages...),
		StdioAllowedNodeScriptRoots: append([]string(nil), config.StdioAllowedNodeScriptRoots...),
		StdioCommandRules:           append([]mcpruntime.StdioCommandRule(nil), config.StdioCommandRules...),
		StdioAllowedEnvKeys:         append([]string(nil), config.StdioAllowedEnvKeys...),
		StdioMaxConfigBytes:         positiveOr(config.StdioMaxConfigBytes, defaultMCPManagementStdioMaxConfigBytes),
		StdioMaxArgs:                nonNegativeOr(config.StdioMaxArgs, defaultMCPManagementStdioMaxArgs),
		StdioMaxArgBytes:            positiveOr(config.StdioMaxArgBytes, defaultMCPManagementStdioMaxArgBytes),
		StdioMaxEnvVars:             nonNegativeOr(config.StdioMaxEnvVars, defaultMCPManagementStdioMaxEnvVars),
		StdioMaxEnvValueBytes:       positiveOr(config.StdioMaxEnvValueBytes, defaultMCPManagementStdioMaxEnvValueBytes),
		RejectUserWorkingDir:        true,
	}
}

func positiveOr(value, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func nonNegativeOr(value, fallback int) int {
	if value < 0 {
		return fallback
	}
	if value == 0 {
		return fallback
	}
	return value
}

var (
	_ mcptool.RuntimeExecutor      = (*mcpManagementRuntime)(nil)
	_ mcptool.CapabilityDiscoverer = (*mcpManagementRuntime)(nil)
)
