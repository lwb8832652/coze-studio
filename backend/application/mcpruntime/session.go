// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultClientName            = "coze-studio-management"
	defaultClientVersion         = "1.0.0"
	defaultWorkdirCleanupTimeout = 10 * time.Second
)

type ProtocolClient interface {
	Start(ctx context.Context) error
	Initialize(ctx context.Context, request mcpsdk.InitializeRequest) (*mcpsdk.InitializeResult, error)
	ListToolsByPage(ctx context.Context, request mcpsdk.ListToolsRequest) (*mcpsdk.ListToolsResult, error)
	ListResourcesByPage(ctx context.Context, request mcpsdk.ListResourcesRequest) (*mcpsdk.ListResourcesResult, error)
	ListPromptsByPage(ctx context.Context, request mcpsdk.ListPromptsRequest) (*mcpsdk.ListPromptsResult, error)
	CallTool(ctx context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error)
	Close() error
}

type ProtocolClientBuilder interface {
	Build(ctx context.Context, connection ResolvedConnection) (ProtocolClient, error)
}

type ProtocolClientBuilderFunc func(
	ctx context.Context,
	connection ResolvedConnection,
) (ProtocolClient, error)

func (f ProtocolClientBuilderFunc) Build(
	ctx context.Context,
	connection ResolvedConnection,
) (ProtocolClient, error) {
	if f == nil {
		return nil, ErrSessionUnavailable
	}
	return f(ctx, connection)
}

type FactoryOptions struct {
	Policy                 Policy
	ClientName             string
	ClientVersion          string
	ClientBuilder          ProtocolClientBuilder
	ProductionStdioBuilder ProductionStdioClientBuilder
	ExecutionMode          StdioExecutionMode
	WorkdirManager         *SafeWorkdirManager
	WorkdirCleanupTimeout  time.Duration
	CleanupSupervisor      WorkdirCleanupSupervisorOptions
}

type ProductionSessionFactory struct {
	policy                Policy
	clientName            string
	clientVersion         string
	clientBuilder         ProtocolClientBuilder
	workdirManager        *SafeWorkdirManager
	workdirCleanupTimeout time.Duration
	cleanupSupervisor     *workdirCleanupSupervisor
	resourceSupervisor    *sessionResourceRetrySupervisor
	ownsWorkdirManager    bool
	lifecycleMu           sync.Mutex
	closing               bool
	opening               int
	active                int
	nextResourceID        uint64
	resources             map[uint64]*managedSessionResource
	stateChanged          chan struct{}
	shutdownMu            sync.Mutex
	managerClosed         bool
}

func NewProductionSessionFactory(options FactoryOptions) (*ProductionSessionFactory, error) {
	cleanupTimeout := options.WorkdirCleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = defaultWorkdirCleanupTimeout
	}
	var productionBuilder ProtocolClientBuilder
	var validatedProductionBuilder *validatedProductionStdioClientBuilder
	if options.ProductionStdioBuilder != nil {
		if options.ClientBuilder != nil {
			return nil, ErrStdioHostExecutionDenied
		}
		canonicalRunner, err := validateTrustedProductionExecutable(
			options.ProductionStdioBuilder.TrustedProductionRunnerExecutable(),
		)
		if err != nil {
			return nil, ErrStdioHostExecutionDenied
		}
		identity, err := inspectExecutable(canonicalRunner)
		if err != nil || identity.path != canonicalRunner {
			return nil, ErrStdioHostExecutionDenied
		}
		validatedProductionBuilder = &validatedProductionStdioClientBuilder{
			builder:            options.ProductionStdioBuilder,
			identity:           identity,
			terminationTimeout: cleanupTimeout,
		}
		productionBuilder = validatedProductionBuilder
	}
	if options.Policy.StdioEnabled && !options.ExecutionMode.AllowsHostExecution() && productionBuilder == nil {
		return nil, ErrStdioHostExecutionDenied
	}
	if options.Policy.StdioEnabled && options.Policy.StdioCommandPolicy == nil {
		commandPolicy, err := options.Policy.commandPolicy()
		if err != nil {
			return nil, err
		}
		options.Policy.StdioCommandPolicy = commandPolicy
	}
	if err := options.Policy.Validate(); err != nil {
		return nil, err
	}
	workdirManager := options.WorkdirManager
	ownsWorkdirManager := false
	if options.Policy.StdioEnabled {
		if workdirManager == nil {
			var err error
			workdirManager, err = NewSafeWorkdirManager(SafeWorkdirOptions{
				Root: options.Policy.StdioWorkdirRoot,
			})
			if err != nil {
				return nil, ErrStdioWorkingDirDenied
			}
			ownsWorkdirManager = true
		}
		if filepath.Clean(strings.TrimSpace(options.Policy.StdioWorkdirRoot)) != workdirManager.Root() {
			return nil, ErrStdioWorkingDirDenied
		}
		if !workdirManager.HasExclusiveRootLock() {
			return nil, ErrStdioWorkingDirDenied
		}
		if validatedProductionBuilder != nil {
			validatedProductionBuilder.controlRoot = workdirManager.ControlRoot()
		}
	}
	clientName := strings.TrimSpace(options.ClientName)
	if clientName == "" {
		clientName = defaultClientName
	}
	clientVersion := strings.TrimSpace(options.ClientVersion)
	if clientVersion == "" {
		clientVersion = defaultClientVersion
	}
	builder := productionBuilder
	if builder == nil {
		builder = options.ClientBuilder
	}
	if builder == nil {
		builder = &sdkProtocolClientBuilder{policy: options.Policy, executionMode: options.ExecutionMode}
	}
	factory := &ProductionSessionFactory{
		policy:                options.Policy,
		clientName:            clientName,
		clientVersion:         clientVersion,
		clientBuilder:         builder,
		workdirManager:        workdirManager,
		workdirCleanupTimeout: cleanupTimeout,
		ownsWorkdirManager:    ownsWorkdirManager,
		stateChanged:          make(chan struct{}),
		resources:             make(map[uint64]*managedSessionResource),
	}
	resourceSupervisor, err := newSessionResourceRetrySupervisor(
		options.CleanupSupervisor,
		factory.signalStateChanged,
	)
	if err != nil {
		if ownsWorkdirManager && workdirManager != nil {
			_ = workdirManager.Close()
		}
		return nil, ErrSessionUnavailable
	}
	factory.resourceSupervisor = resourceSupervisor
	if workdirManager != nil {
		supervisor, err := newWorkdirCleanupSupervisor(
			workdirManager,
			cleanupTimeout,
			options.CleanupSupervisor,
		)
		if err != nil {
			if ownsWorkdirManager {
				_ = workdirManager.Close()
			}
			return nil, ErrStdioWorkingDirDenied
		}
		factory.cleanupSupervisor = supervisor
	}
	return factory, nil
}

type validatedProductionStdioClientBuilder struct {
	builder            ProductionStdioClientBuilder
	identity           executableSnapshot
	terminationTimeout time.Duration
	controlRoot        string
}

func (b *validatedProductionStdioClientBuilder) Build(
	ctx context.Context,
	connection ResolvedConnection,
) (ProtocolClient, error) {
	if b == nil || b.builder == nil {
		return nil, ErrStdioHostExecutionDenied
	}
	canonicalRunner, err := validateTrustedProductionExecutable(b.identity.path)
	if err != nil || canonicalRunner != b.identity.path {
		return nil, ErrStdioHostExecutionDenied
	}
	current, err := inspectExecutable(canonicalRunner)
	if err != nil || !sameExecutableSnapshot(b.identity, current) {
		return nil, ErrStdioHostExecutionDenied
	}
	result, buildErr := b.builder.BuildWithTrustedProductionRunner(ctx, canonicalRunner, connection)
	if result == nil || (result.Client == nil && result.ProcessTree == nil) {
		return nil, ErrStdioHostExecutionDenied
	}
	if result.ProcessTree == nil {
		return &unverifiedProductionProtocolClient{ProtocolClient: result.Client},
			ErrStdioHostExecutionDenied
	}
	client := &productionIsolatedProtocolClient{
		ProtocolClient: result.Client,
		processTree:    result.ProcessTree,
		timeout:        b.terminationTimeout,
	}
	if buildErr != nil || result.Client == nil ||
		!validProductionProcessTreeIsolation(result.ProcessTree.Isolation()) ||
		b.controlRoot == "" || !result.ProcessTree.ControlRootInaccessible(b.controlRoot) {
		return client, ErrStdioHostExecutionDenied
	}
	return client, nil
}

type unverifiedProductionProtocolClient struct {
	ProtocolClient
	mu           sync.Mutex
	clientClosed bool
}

func (c *unverifiedProductionProtocolClient) Close() error {
	if c == nil || c.ProtocolClient == nil {
		return ErrSessionUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.clientClosed && c.ProtocolClient.Close() == nil {
		c.clientClosed = true
	}
	return ErrSessionUnavailable
}

type productionIsolatedProtocolClient struct {
	ProtocolClient
	processTree    ProductionProcessTreeHandle
	timeout        time.Duration
	mu             sync.Mutex
	clientClosed   bool
	treeTerminated bool
}

func (c *productionIsolatedProtocolClient) Close() error {
	if c == nil || c.processTree == nil {
		return ErrSessionUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.clientClosed {
		if c.ProtocolClient == nil {
			c.clientClosed = true
		} else if err := c.ProtocolClient.Close(); err == nil {
			c.clientClosed = true
		}
	}
	if !c.treeTerminated {
		ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
		err := c.processTree.TerminateAndWait(ctx)
		cancel()
		if err == nil {
			c.treeTerminated = true
		}
	}
	if !c.clientClosed || !c.treeTerminated {
		return ErrSessionUnavailable
	}
	return nil
}

func (f *ProductionSessionFactory) Open(
	ctx context.Context,
	connection Connection,
) (Session, error) {
	if f == nil || f.clientBuilder == nil || !f.beginOpen() {
		return nil, ErrSessionUnavailable
	}
	defer f.finishOpen()
	if f.cleanupSupervisor != nil && !f.cleanupSupervisor.WaitStartup(ctx) {
		return nil, ErrSessionUnavailable
	}
	resolved, err := f.policy.Resolve(connection)
	if err != nil {
		return nil, err
	}
	cleanup := func() error { return nil }
	if resolved.Stdio != nil {
		if f.workdirManager == nil {
			return nil, ErrSessionUnavailable
		}
		workingDir, err := f.workdirManager.Create(ctx, SafeWorkdirCreateRequest{
			Prefix: managementSafeWorkdirPrefix,
		})
		if err != nil {
			return nil, ErrSessionUnavailable
		}
		resolved.Stdio.WorkingDir = workingDir.Path
		cleanup = func() error {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), f.workdirCleanupTimeout)
			defer cancel()
			err := f.workdirManager.Delete(cleanupCtx, workingDir.RelativePath)
			if err != nil && f.cleanupSupervisor != nil {
				_ = f.cleanupSupervisor.enqueueRelative(workingDir.RelativePath)
			}
			return err
		}
	}
	client, err := f.clientBuilder.Build(ctx, resolved)
	if client == nil {
		_ = cleanup()
		return nil, ErrSessionUnavailable
	}
	resource := f.registerResource(client, cleanup)
	closeFailedSession := func() {
		if resource.close() != nil {
			f.enqueueResource(resource)
		}
	}
	if err != nil {
		closeFailedSession()
		return nil, ErrSessionUnavailable
	}
	if err := client.Start(ctx); err != nil {
		closeFailedSession()
		return nil, ErrSessionUnavailable
	}
	request := mcpsdk.InitializeRequest{}
	request.Params.ProtocolVersion = mcpsdk.LATEST_PROTOCOL_VERSION
	request.Params.ClientInfo = mcpsdk.Implementation{Name: f.clientName, Version: f.clientVersion}
	result, err := client.Initialize(ctx, request)
	if err != nil || result == nil {
		closeFailedSession()
		return nil, ErrSessionUnavailable
	}
	session := &protocolSession{
		client: client,
		capabilities: CapabilityFlags{
			Tools:     result.Capabilities.Tools != nil,
			Resources: result.Capabilities.Resources != nil,
			Prompts:   result.Capabilities.Prompts != nil,
		},
		resource: resource,
	}
	return session, nil
}

func (f *ProductionSessionFactory) Shutdown(ctx context.Context) error {
	if f == nil {
		return nil
	}
	if ctx == nil {
		return ErrSessionUnavailable
	}
	f.shutdownMu.Lock()
	defer f.shutdownMu.Unlock()
	f.lifecycleMu.Lock()
	f.closing = true
	f.signalStateChangedLocked()
	f.lifecycleMu.Unlock()
	if !f.waitForSessions(ctx) {
		return ErrSessionUnavailable
	}
	if f.resourceSupervisor != nil && f.resourceSupervisor.Shutdown(ctx) != nil {
		return ErrSessionUnavailable
	}
	if f.cleanupSupervisor != nil && f.cleanupSupervisor.Shutdown(ctx) != nil {
		return ErrSessionUnavailable
	}
	if f.ownsWorkdirManager && f.workdirManager != nil && !f.managerClosed {
		if err := f.workdirManager.Close(); err != nil {
			return ErrSessionUnavailable
		}
		f.managerClosed = true
	}
	return nil
}

func (f *ProductionSessionFactory) Close() error {
	return f.Shutdown(context.Background())
}

func (f *ProductionSessionFactory) beginOpen() bool {
	f.lifecycleMu.Lock()
	defer f.lifecycleMu.Unlock()
	if f.closing {
		return false
	}
	f.opening++
	f.signalStateChangedLocked()
	return true
}

func (f *ProductionSessionFactory) finishOpen() {
	f.lifecycleMu.Lock()
	defer f.lifecycleMu.Unlock()
	if f.opening > 0 {
		f.opening--
	}
	f.signalStateChangedLocked()
}

func (f *ProductionSessionFactory) registerResource(
	client ProtocolClient,
	cleanup func() error,
) *managedSessionResource {
	f.lifecycleMu.Lock()
	f.nextResourceID++
	id := f.nextResourceID
	resource := &managedSessionResource{client: client, cleanup: cleanup, onRetryable: f.enqueueResource}
	resource.onCompleted = func() { f.completeResource(id, resource) }
	f.resources[id] = resource
	f.active++
	f.signalStateChangedLocked()
	f.lifecycleMu.Unlock()
	return resource
}

func (f *ProductionSessionFactory) completeResource(
	id uint64,
	resource *managedSessionResource,
) {
	f.lifecycleMu.Lock()
	if current, exists := f.resources[id]; exists && current == resource {
		delete(f.resources, id)
		if f.active > 0 {
			f.active--
		}
	}
	f.signalStateChangedLocked()
	f.lifecycleMu.Unlock()
}

func (f *ProductionSessionFactory) enqueueResource(resource *managedSessionResource) {
	if f == nil || resource == nil || f.resourceSupervisor == nil {
		return
	}
	resource.markRetryable()
	_ = f.resourceSupervisor.enqueue(resource)
}

func (f *ProductionSessionFactory) enqueueAllResources() {
	f.lifecycleMu.Lock()
	resources := make([]*managedSessionResource, 0, len(f.resources))
	for _, resource := range f.resources {
		if resource.shouldRetry() {
			resources = append(resources, resource)
		}
	}
	f.lifecycleMu.Unlock()
	for _, resource := range resources {
		f.enqueueResource(resource)
	}
}

func (f *ProductionSessionFactory) waitForSessions(ctx context.Context) bool {
	for {
		f.enqueueAllResources()
		f.lifecycleMu.Lock()
		if f.opening == 0 && f.active == 0 {
			f.lifecycleMu.Unlock()
			return true
		}
		changed := f.stateChanged
		f.lifecycleMu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return false
		}
	}
}

func (f *ProductionSessionFactory) signalStateChanged() {
	if f == nil {
		return
	}
	f.lifecycleMu.Lock()
	f.signalStateChangedLocked()
	f.lifecycleMu.Unlock()
}

func (f *ProductionSessionFactory) signalStateChangedLocked() {
	close(f.stateChanged)
	f.stateChanged = make(chan struct{})
}

type protocolSession struct {
	client       ProtocolClient
	capabilities CapabilityFlags
	resource     *managedSessionResource
}

func (s *protocolSession) Capabilities() CapabilityFlags {
	if s == nil {
		return CapabilityFlags{}
	}
	return s.capabilities
}

func (s *protocolSession) ListToolsByPage(ctx context.Context, cursor string) (ToolPage, error) {
	if s == nil || s.client == nil {
		return ToolPage{}, ErrSessionUnavailable
	}
	pager, err := NewMCPToolPager(s.client)
	if err != nil {
		return ToolPage{}, err
	}
	return pager.ListToolsByPage(ctx, cursor)
}

func (s *protocolSession) ListResourcesByPage(ctx context.Context, cursor string) (ResourcePage, error) {
	if s == nil || s.client == nil {
		return ResourcePage{}, ErrSessionUnavailable
	}
	request := mcpsdk.ListResourcesRequest{}
	request.Params.Cursor = mcpsdk.Cursor(cursor)
	result, err := s.client.ListResourcesByPage(ctx, request)
	if err != nil || result == nil {
		return ResourcePage{}, ErrSessionUnavailable
	}
	items := make([]Resource, 0, len(result.Resources))
	for _, item := range result.Resources {
		items = append(items, Resource{
			URI:         item.URI,
			Name:        item.Name,
			Description: item.Description,
			MIMEType:    item.MIMEType,
		})
	}
	return ResourcePage{Items: items, NextCursor: string(result.NextCursor)}, nil
}

func (s *protocolSession) ListPromptsByPage(ctx context.Context, cursor string) (PromptPage, error) {
	if s == nil || s.client == nil {
		return PromptPage{}, ErrSessionUnavailable
	}
	request := mcpsdk.ListPromptsRequest{}
	request.Params.Cursor = mcpsdk.Cursor(cursor)
	result, err := s.client.ListPromptsByPage(ctx, request)
	if err != nil || result == nil {
		return PromptPage{}, ErrSessionUnavailable
	}
	items := make([]Prompt, 0, len(result.Prompts))
	for _, item := range result.Prompts {
		arguments := make([]PromptArgument, 0, len(item.Arguments))
		for _, argument := range item.Arguments {
			arguments = append(arguments, PromptArgument{
				Name:        argument.Name,
				Description: argument.Description,
				Required:    argument.Required,
			})
		}
		items = append(items, Prompt{
			Name:        item.Name,
			Description: item.Description,
			Arguments:   arguments,
		})
	}
	return PromptPage{Items: items, NextCursor: string(result.NextCursor)}, nil
}

func (s *protocolSession) CallTool(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (ToolCallResult, error) {
	if s == nil || s.client == nil {
		return ToolCallResult{}, ErrSessionUnavailable
	}
	request := mcpsdk.CallToolRequest{}
	request.Params.Name = strings.TrimSpace(name)
	request.Params.Arguments = arguments
	result, err := s.client.CallTool(ctx, request)
	if err != nil || result == nil {
		return ToolCallResult{}, ErrSessionUnavailable
	}
	return ToolCallResult{IsError: result.IsError}, nil
}

func (s *protocolSession) Close() error {
	if s == nil {
		return nil
	}
	if s.resource == nil || s.resource.close() != nil {
		if s.resource != nil {
			s.resource.requestRetry()
		}
		return ErrSessionUnavailable
	}
	return nil
}

type sdkProtocolClientBuilder struct {
	policy        Policy
	executionMode StdioExecutionMode
}

func (b *sdkProtocolClientBuilder) Build(
	_ context.Context,
	connection ResolvedConnection,
) (ProtocolClient, error) {
	if connection.Stdio != nil {
		config := *connection.Stdio
		env := sortedEnv(config.Env)
		transport, err := NewSafeStdioTransport(StdioTransportOptions{
			Command:       config.Command,
			Args:          append([]string(nil), config.Args...),
			Env:           env,
			WorkingDir:    config.WorkingDir,
			CommandPolicy: b.policy.StdioCommandPolicy,
			ExecutionMode: b.executionMode,
		})
		if err != nil {
			return nil, ErrSessionUnavailable
		}
		return mcpclient.NewClient(transport), nil
	}
	if connection.Remote == nil {
		return nil, ErrSessionUnavailable
	}
	remote := connection.Remote
	httpPolicy, err := NewSafeHTTPPolicy(SafeHTTPPolicyOptions{
		AllowedHosts:    b.policy.RemoteAllowedHosts,
		AllowLocalDebug: b.policy.AllowInsecureHTTP,
	})
	if err != nil {
		return nil, ErrSessionUnavailable
	}
	httpClient, err := NewSafeHTTPClient(SafeHTTPClientOptions{
		Policy:  httpPolicy,
		Timeout: remote.HTTPTimeout,
	})
	if err != nil {
		return nil, ErrSessionUnavailable
	}
	headers := cloneStringMap(remote.Headers)
	switch remote.ServerType {
	case ServerTypeSSE:
		transport, err := NewSafeSSETransport(SSETransportOptions{
			URL:             remote.URL,
			Headers:         headers,
			HTTPClient:      httpClient,
			HTTPPolicy:      httpPolicy,
			EndpointTimeout: remote.HTTPTimeout,
		})
		if err != nil {
			return nil, ErrSessionUnavailable
		}
		return mcpclient.NewClient(transport), nil
	case ServerTypeStreamableHTTP:
		transport, err := mcptransport.NewStreamableHTTP(
			remote.URL,
			mcptransport.WithHTTPHeaders(headers),
			mcptransport.WithHTTPBasicClient(httpClient),
			mcptransport.WithHTTPTimeout(remote.HTTPTimeout),
		)
		if err != nil {
			return nil, ErrSessionUnavailable
		}
		return mcpclient.NewClient(transport), nil
	default:
		return nil, ErrSessionUnavailable
	}
}
