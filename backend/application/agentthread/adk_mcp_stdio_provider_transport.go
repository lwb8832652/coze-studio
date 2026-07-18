// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package agentthread

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	defaultADKMCPRuntimeStdioProviderCleanupTimeout = 5 * time.Second
	defaultADKMCPRuntimeStdioProviderCancelTimeout  = 5 * time.Second
	adkMCPRuntimeStdioProviderEntrypoint            = "mcp/stdio/invoke"
)

type ADKMCPRuntimeSandboxProviderRepository interface {
	GetProvider(ctx context.Context, providerID int64) (*domainsandbox.Provider, error)
}

type ADKMCPRuntimeSandboxSelection interface {
	ADKMCPRuntimeSandboxPolicy() domainsandbox.RuntimePolicy
	Execute(ctx context.Context, request infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error)
	Status(ctx context.Context) (infrasandbox.ExecuteResult, error)
	Cancel(ctx context.Context, executionID string) error
	Release(ctx context.Context) error
}

type ADKMCPRuntimeSandboxRouter interface {
	ResolveADKMCPRuntimeSandbox(
		ctx context.Context,
		providerKey string,
		scope domainsandbox.Scope,
	) (ADKMCPRuntimeSandboxSelection, error)
}

type ADKMCPRuntimeSandboxBinding struct {
	ProviderDefaults domainsandbox.ProviderDefaultRepository
	Providers        ADKMCPRuntimeSandboxProviderRepository
	Router           ADKMCPRuntimeSandboxRouter
}

func (b ADKMCPRuntimeSandboxBinding) valid() bool {
	return b.ProviderDefaults != nil && b.Providers != nil && b.Router != nil
}

type ADKMCPRuntimeSandboxBindingSource interface {
	LoadADKMCPRuntimeSandboxBinding() (ADKMCPRuntimeSandboxBinding, bool)
}

type ADKMCPRuntimeStdioProviderTransportOptions struct {
	BindingSource  ADKMCPRuntimeSandboxBindingSource
	Policy         *mcpruntime.LogicalStdioPolicy
	AuditRecorder  ADKMCPRuntimeAuditRecorder
	MaxConfigBytes int
	MaxResultBytes int
	CancelTimeout  time.Duration
	CleanupTimeout time.Duration
	OperationID    func(context.Context) (string, error)
	Now            func() time.Time
}

type ADKMCPRuntimeStdioProviderTransport struct {
	bindingSource  ADKMCPRuntimeSandboxBindingSource
	policy         *mcpruntime.LogicalStdioPolicy
	auditRecorder  ADKMCPRuntimeAuditRecorder
	maxConfigBytes int
	maxResultBytes int
	cancelTimeout  time.Duration
	cleanupTimeout time.Duration
	operationID    func(context.Context) (string, error)
	now            func() time.Time
}

func NewADKMCPRuntimeStdioProviderTransport(
	options ADKMCPRuntimeStdioProviderTransportOptions,
) (*ADKMCPRuntimeStdioProviderTransport, error) {
	if options.BindingSource == nil || options.Policy == nil {
		return nil, errors.New("mcp runtime stdio provider transport is not configured")
	}
	maxConfigBytes := options.MaxConfigBytes
	if maxConfigBytes <= 0 {
		maxConfigBytes = defaultADKMCPRuntimeStdioMaxConfigBytes
	}
	maxResultBytes := options.MaxResultBytes
	if maxResultBytes <= 0 {
		maxResultBytes = defaultADKMCPRuntimeExecutorMaxOutputBytes
	}
	cancelTimeout := options.CancelTimeout
	if cancelTimeout <= 0 {
		cancelTimeout = defaultADKMCPRuntimeStdioProviderCancelTimeout
	}
	cleanupTimeout := options.CleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = defaultADKMCPRuntimeStdioProviderCleanupTimeout
	}
	operationID := options.OperationID
	if operationID == nil {
		operationID = newADKMCPRuntimeStdioProviderOperationID
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &ADKMCPRuntimeStdioProviderTransport{
		bindingSource:  options.BindingSource,
		policy:         options.Policy,
		auditRecorder:  options.AuditRecorder,
		maxConfigBytes: maxConfigBytes,
		maxResultBytes: maxResultBytes,
		cancelTimeout:  cancelTimeout,
		cleanupTimeout: cleanupTimeout,
		operationID:    operationID,
		now:            now,
	}, nil
}

func (t *ADKMCPRuntimeStdioProviderTransport) InvokeADKMCPRuntimeTransport(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
) (result string, returnErr error) {
	startedAt := t.currentTime()
	fail := func(err error) (string, error) {
		t.recordProviderAudit(ctx, call, "mcp.provider.selection", normalizeADKMCPProviderErrorCode(err), startedAt)
		return "", errors.New("mcp runtime stdio provider transport failed")
	}
	if t == nil || t.bindingSource == nil || t.policy == nil || ctx == nil {
		return fail(domainsandbox.ErrUnavailable)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	config, err := parseADKMCPRuntimeStdioConfig(call, t.maxConfigBytes)
	if err != nil {
		return fail(domainsandbox.ErrInvalidInput)
	}
	config.WorkingDir, err = virtualADKMCPRuntimeStdioProviderWorkdir(call)
	if err != nil {
		return fail(domainsandbox.ErrInvalidInput)
	}
	if err := t.policy.Validate(mcpruntime.StdioConfig{
		Command:    config.Command,
		Args:       append([]string(nil), config.Args...),
		Env:        cloneADKMCPRuntimeStringMap(config.Env),
		WorkingDir: config.WorkingDir,
	}); err != nil {
		return fail(domainsandbox.ErrExecutionForbidden)
	}
	binding, ok := t.bindingSource.LoadADKMCPRuntimeSandboxBinding()
	if !ok || !binding.valid() {
		return fail(domainsandbox.ErrUnavailable)
	}
	defaultProvider, err := binding.ProviderDefaults.GetProviderDefault(ctx, domainsandbox.ScopeMCPStdio)
	if err != nil || defaultProvider == nil || defaultProvider.Scope != domainsandbox.ScopeMCPStdio ||
		defaultProvider.ProviderID <= 0 {
		if err == nil {
			err = domainsandbox.ErrDefaultMissing
		}
		return fail(err)
	}
	provider, err := binding.Providers.GetProvider(ctx, defaultProvider.ProviderID)
	if err != nil || provider == nil || provider.ID != defaultProvider.ProviderID ||
		strings.TrimSpace(provider.ProviderKey) == "" {
		if err == nil {
			err = domainsandbox.ErrProviderNotFound
		}
		return fail(err)
	}
	selection, err := binding.Router.ResolveADKMCPRuntimeSandbox(
		ctx,
		provider.ProviderKey,
		domainsandbox.ScopeMCPStdio,
	)
	if err != nil || selection == nil {
		if err == nil {
			err = domainsandbox.ErrUnavailable
		}
		return fail(err)
	}
	t.recordProviderAudit(ctx, call, "mcp.provider.selection", "selected", startedAt)
	defer func() {
		cleanupStarted := t.currentTime()
		cleanupCtx, cancel := detachedADKMCPRuntimeContext(ctx, t.cleanupTimeout)
		cleanupErr := selection.Release(cleanupCtx)
		code := "cleanup_released"
		if cleanupErr != nil {
			code = normalizeADKMCPProviderCleanupCode(cleanupCtx, cleanupErr)
			result = ""
			if returnErr == nil {
				returnErr = errors.New("mcp runtime stdio provider transport failed")
			}
		}
		cancel()
		t.recordProviderAudit(context.WithoutCancel(ctx), call, "mcp.provider.cleanup", code, cleanupStarted)
	}()

	providerPolicy, err := validateADKMCPRuntimeStdioProviderPolicy(
		selection.ADKMCPRuntimeSandboxPolicy(),
		config,
	)
	if err != nil {
		return "", errors.New("mcp runtime stdio provider transport failed")
	}
	operationID, err := t.operationID(ctx)
	if err != nil {
		return "", errors.New("mcp runtime stdio provider transport failed")
	}
	request, err := t.buildExecuteRequest(ctx, call, config, providerPolicy, operationID)
	if err != nil {
		return "", errors.New("mcp runtime stdio provider transport failed")
	}
	execution, err := selection.Execute(ctx, request)
	if err != nil && shouldReconcileADKMCPRuntimeProviderExecution(err) {
		reconcileCtx := ctx
		cancel := func() {}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			reconcileCtx, cancel = detachedADKMCPRuntimeContext(ctx, t.cancelTimeout)
		}
		execution, err = selection.Execute(reconcileCtx, request)
		cancel()
	}
	if err != nil {
		t.recordProviderAudit(
			context.WithoutCancel(ctx),
			call,
			"mcp.provider.execution",
			normalizeADKMCPProviderErrorCode(err),
			startedAt,
		)
		return "", errors.New("mcp runtime stdio provider transport failed")
	}
	switch execution.Status {
	case infrasandbox.ExecutionStatusAccepted, infrasandbox.ExecutionStatusRunning:
		cancelStarted := t.currentTime()
		cancelCtx, cancel := detachedADKMCPRuntimeContext(ctx, t.cancelTimeout)
		cancelErr := selection.Cancel(cancelCtx, execution.ExecutionID)
		if cancelErr == nil {
			cancelErr = waitForADKMCPRuntimeProviderTerminal(cancelCtx, selection)
		}
		cancelCode := "cancel_completed"
		if cancelErr != nil {
			cancelCode = normalizeADKMCPProviderCancelCode(cancelCtx, cancelErr)
		}
		cancel()
		t.recordProviderAudit(context.WithoutCancel(ctx), call, "mcp.provider.cancel", cancelCode, cancelStarted)
		return "", errors.New("mcp runtime stdio provider transport failed")
	case infrasandbox.ExecutionStatusSucceeded:
		if execution.ExitCode == nil || *execution.ExitCode != 0 || execution.Stderr != "" ||
			len(execution.Artifacts) != 0 || len(execution.ArtifactDescriptors) != 0 ||
			execution.PreviewRoute != "" {
			return "", errors.New("mcp runtime stdio provider transport failed")
		}
		reviewed, parseErr := infrasandbox.ParseMCPStdioResultEnvelope(
			[]byte(execution.Stdout),
			t.maxResultBytes,
		)
		if parseErr != nil {
			return "", errors.New("mcp runtime stdio provider transport failed")
		}
		projected, marshalErr := json.Marshal(reviewed)
		if marshalErr != nil || len(projected) > t.maxResultBytes {
			return "", errors.New("mcp runtime stdio provider transport failed")
		}
		t.recordProviderAudit(context.WithoutCancel(ctx), call, "mcp.provider.execution", "completed", startedAt)
		return string(projected), nil
	case infrasandbox.ExecutionStatusFailed, infrasandbox.ExecutionStatusCanceled:
		return "", errors.New("mcp runtime stdio provider transport failed")
	case infrasandbox.ExecutionStatusTimedOut:
		t.recordProviderAudit(context.WithoutCancel(ctx), call, "mcp.provider.execution", "provider_timeout", startedAt)
		return "", errors.New("mcp runtime stdio provider transport failed")
	default:
		return "", errors.New("mcp runtime stdio provider transport failed")
	}
}

func waitForADKMCPRuntimeProviderTerminal(
	ctx context.Context,
	selection ADKMCPRuntimeSandboxSelection,
) error {
	if ctx == nil || selection == nil {
		return domainsandbox.ErrInvalidInput
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := selection.Status(ctx)
		if err != nil {
			return err
		}
		switch status.Status {
		case infrasandbox.ExecutionStatusSucceeded,
			infrasandbox.ExecutionStatusFailed,
			infrasandbox.ExecutionStatusCanceled,
			infrasandbox.ExecutionStatusTimedOut:
			return nil
		case infrasandbox.ExecutionStatusAccepted,
			infrasandbox.ExecutionStatusRunning:
		default:
			return domainsandbox.ErrUnavailable
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (t *ADKMCPRuntimeStdioProviderTransport) buildExecuteRequest(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
	config ADKMCPRuntimeStdioConfig,
	policy domainsandbox.RuntimePolicy,
	operationID string,
) (infrasandbox.ExecuteRequest, error) {
	if call.Run == nil || call.Server == nil {
		return infrasandbox.ExecuteRequest{}, domainsandbox.ErrInvalidInput
	}
	envNames := make([]string, 0, len(config.Env))
	for name := range config.Env {
		envNames = append(envNames, name)
	}
	sort.Strings(envNames)
	stdin, err := infrasandbox.MarshalMCPStdioInvokeEnvelope(infrasandbox.MCPStdioInvokeEnvelope{
		Schema:        infrasandbox.MCPStdioInvokeSchemaV1,
		OperationID:   operationID,
		Command:       config.Command,
		Args:          append([]string(nil), config.Args...),
		ToolName:      strings.TrimSpace(call.ToolName),
		Arguments:     json.RawMessage(strings.TrimSpace(call.Arguments)),
		WorkingDir:    config.WorkingDir,
		EnvNames:      envNames,
		ClientVersion: infrasandbox.MCPStdioClientVersionV1,
	})
	if err != nil {
		return infrasandbox.ExecuteRequest{}, domainsandbox.ErrInvalidInput
	}
	now := t.currentTime()
	deadline := now.Add(time.Duration(policy.TimeoutSeconds) * time.Second)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	if !deadline.After(now) {
		return infrasandbox.ExecuteRequest{}, context.DeadlineExceeded
	}
	return infrasandbox.ExecuteRequest{
		Scope:          domainsandbox.ScopeMCPStdio,
		WorkloadKind:   infrasandbox.WorkloadMCPStdio,
		IdempotencyKey: "mcp_stdio:" + operationID,
		Deadline:       deadline.UTC(),
		Policy:         policy,
		Entrypoint:     adkMCPRuntimeStdioProviderEntrypoint,
		Env:            cloneADKMCPRuntimeStringMap(config.Env),
		Stdin:          stdin,
	}, nil
}

func validateADKMCPRuntimeStdioProviderPolicy(
	policy domainsandbox.RuntimePolicy,
	config ADKMCPRuntimeStdioConfig,
) (domainsandbox.RuntimePolicy, error) {
	normalized, err := domainsandbox.NormalizeRuntimePolicy(policy)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, domainsandbox.ErrConfigurationInvalid
	}
	if !adkMCPRuntimeContainsString(normalized.AllowedExecutables, config.Command) ||
		!adkMCPRuntimeVirtualPathAllowed(config.WorkingDir, normalized.VirtualWritePrefixes) {
		return domainsandbox.RuntimePolicy{}, domainsandbox.ErrExecutionForbidden
	}
	for key := range config.Env {
		if !adkMCPRuntimeContainsString(normalized.AllowedEnvNames, key) {
			return domainsandbox.RuntimePolicy{}, domainsandbox.ErrExecutionForbidden
		}
	}
	return normalized, nil
}

func virtualADKMCPRuntimeStdioProviderWorkdir(
	call ADKMCPRuntimeTransportCall,
) (string, error) {
	if call.Run == nil || call.Server == nil || call.Run.SpaceID <= 0 ||
		call.Run.ThreadID <= 0 || call.Run.RunID <= 0 || call.Server.ServerID <= 0 {
		return "", domainsandbox.ErrInvalidInput
	}
	return fmt.Sprintf(
		"workspace/mcp_stdio/space-%d/thread-%d/run-%d/server-%d",
		call.Run.SpaceID,
		call.Run.ThreadID,
		call.Run.RunID,
		call.Server.ServerID,
	), nil
}

func newADKMCPRuntimeStdioProviderOperationID(context.Context) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", errors.New("mcp runtime stdio provider operation identity unavailable")
	}
	return "op_" + hex.EncodeToString(entropy[:]), nil
}

func shouldReconcileADKMCPRuntimeProviderExecution(err error) bool {
	return errors.Is(err, domainsandbox.ErrUnavailable) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func detachedADKMCPRuntimeContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

func (t *ADKMCPRuntimeStdioProviderTransport) currentTime() time.Time {
	if t == nil || t.now == nil {
		return time.Now().UTC()
	}
	return t.now().UTC()
}

func (t *ADKMCPRuntimeStdioProviderTransport) recordProviderAudit(
	ctx context.Context,
	call ADKMCPRuntimeTransportCall,
	eventType string,
	errorCode string,
	startedAt time.Time,
) {
	if t == nil || t.auditRecorder == nil || call.Run == nil || call.Server == nil {
		return
	}
	elapsed := t.currentTime().Sub(startedAt).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	_ = t.auditRecorder.RecordADKMCPRuntimeAudit(ctx, ADKMCPRuntimeAuditRecord{
		SpaceID:         call.Run.SpaceID,
		ThreadID:        call.Run.ThreadID,
		RunID:           call.Run.RunID,
		ServerID:        call.Server.ServerID,
		RuntimeToolName: adkMCPRuntimeSafeName(call.Name),
		Transport:       adkMCPRuntimeTransportStdio,
		EventType:       eventType,
		ErrorCode:       errorCode,
		ElapsedMillis:   elapsed,
	})
}

func normalizeADKMCPProviderErrorCode(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "provider_timeout"
	}
	if code := domainsandbox.ErrorCodeOf(err); code != "" {
		return code
	}
	return domainsandbox.ErrCodeUnavailable
}

func normalizeADKMCPProviderCancelCode(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) ||
		ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "cancel_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancel_canceled"
	}
	return "cancel_failed"
}

func normalizeADKMCPProviderCleanupCode(ctx context.Context, err error) string {
	if errors.Is(err, context.DeadlineExceeded) ||
		ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "cleanup_timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cleanup_canceled"
	}
	return "cleanup_failed"
}

func adkMCPRuntimeContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func adkMCPRuntimeVirtualPathAllowed(target string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if target == prefix || strings.HasPrefix(target, prefix+"/") {
			return true
		}
	}
	return false
}
