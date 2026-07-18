// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	codeRunnerEntrypoint            = "agent/code/run"
	defaultCodeRunnerCancelTimeout  = 5 * time.Second
	defaultCodeRunnerCleanupTimeout = 5 * time.Second
)

type ProviderDefaultRepository interface {
	GetProviderDefault(
		ctx context.Context,
		scope domainsandbox.Scope,
	) (*domainsandbox.ProviderDefault, error)
}

type ProviderRepository interface {
	GetProvider(ctx context.Context, providerID int64) (*domainsandbox.Provider, error)
}

type Selection interface {
	CodeRunnerSandboxPolicy() domainsandbox.RuntimePolicy
	Execute(
		ctx context.Context,
		request infrasandbox.ExecuteRequest,
	) (infrasandbox.ExecuteResult, error)
	Status(ctx context.Context) (infrasandbox.ExecuteResult, error)
	Cancel(ctx context.Context, executionID string) error
	Release(ctx context.Context) error
}

type Router interface {
	ResolveCodeRunnerSandbox(
		ctx context.Context,
		providerKey string,
		scope domainsandbox.Scope,
	) (Selection, error)
}

type Binding struct {
	ProviderDefaults ProviderDefaultRepository
	Providers        ProviderRepository
	Router           Router
}

func (b Binding) valid() bool {
	return b.ProviderDefaults != nil && b.Providers != nil && b.Router != nil
}

type BindingSource interface {
	LoadCodeRunnerSandboxBinding() (Binding, bool)
}

type Options struct {
	BindingSource  BindingSource
	MaxInputBytes  int
	MaxOutputBytes int
	CancelTimeout  time.Duration
	CleanupTimeout time.Duration
	OperationID    func(context.Context) (string, error)
	Now            func() time.Time
}

type runner struct {
	bindingSource  BindingSource
	maxInputBytes  int
	maxOutputBytes int
	cancelTimeout  time.Duration
	cleanupTimeout time.Duration
	operationID    func(context.Context) (string, error)
	now            func() time.Time
}

func NewRunner(options Options) (coderunner.Runner, error) {
	if options.BindingSource == nil {
		return nil, coderunner.ErrCodeRunnerUnavailable
	}
	maxInputBytes := options.MaxInputBytes
	if maxInputBytes <= 0 || maxInputBytes > infrasandbox.MaxCodeRunnerEnvelopeBytes {
		maxInputBytes = infrasandbox.MaxCodeRunnerEnvelopeBytes
	}
	maxOutputBytes := options.MaxOutputBytes
	if maxOutputBytes <= 0 || maxOutputBytes > infrasandbox.MaxCodeRunnerEnvelopeBytes {
		maxOutputBytes = infrasandbox.MaxCodeRunnerEnvelopeBytes
	}
	cancelTimeout := options.CancelTimeout
	if cancelTimeout <= 0 {
		cancelTimeout = defaultCodeRunnerCancelTimeout
	}
	cleanupTimeout := options.CleanupTimeout
	if cleanupTimeout <= 0 {
		cleanupTimeout = defaultCodeRunnerCleanupTimeout
	}
	operationID := options.OperationID
	if operationID == nil {
		operationID = newCodeRunnerOperationID
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &runner{
		bindingSource:  options.BindingSource,
		maxInputBytes:  maxInputBytes,
		maxOutputBytes: maxOutputBytes,
		cancelTimeout:  cancelTimeout,
		cleanupTimeout: cleanupTimeout,
		operationID:    operationID,
		now:            now,
	}, nil
}

func (r *runner) Run(
	ctx context.Context,
	input *coderunner.RunRequest,
) (response *coderunner.RunResponse, returnErr error) {
	if r == nil || r.bindingSource == nil || ctx == nil || input == nil ||
		(input.Language != coderunner.Python && input.Language != coderunner.JavaScript) ||
		input.Code == "" {
		return nil, coderunner.ErrCodeRunnerInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return nil, mapCodeRunnerError(err)
	}
	operationID, err := r.operationID(ctx)
	if err != nil {
		return nil, coderunner.ErrCodeRunnerUnavailable
	}
	params, err := marshalCodeRunnerParams(input.Params)
	if err != nil {
		return nil, coderunner.ErrCodeRunnerInvalidRequest
	}
	stdin, err := infrasandbox.MarshalCodeRunnerInvokeEnvelope(
		infrasandbox.CodeRunnerInvokeEnvelope{
			Schema:        infrasandbox.CodeRunnerInvokeSchemaV1,
			ClientVersion: infrasandbox.CodeRunnerClientVersionV1,
			OperationID:   operationID,
			Language:      string(input.Language),
			Code:          input.Code,
			Params:        params,
		},
	)
	if err != nil || len(stdin) > r.maxInputBytes {
		return nil, coderunner.ErrCodeRunnerInvalidRequest
	}

	binding, ok := r.bindingSource.LoadCodeRunnerSandboxBinding()
	if !ok || !binding.valid() {
		return nil, coderunner.ErrCodeRunnerUnavailable
	}
	defaultProvider, err := binding.ProviderDefaults.GetProviderDefault(
		ctx,
		domainsandbox.ScopeAgent,
	)
	if err != nil || defaultProvider == nil ||
		defaultProvider.Scope != domainsandbox.ScopeAgent ||
		defaultProvider.ProviderID <= 0 {
		return nil, mapCodeRunnerError(err)
	}
	provider, err := binding.Providers.GetProvider(ctx, defaultProvider.ProviderID)
	if err != nil || provider == nil || provider.ID != defaultProvider.ProviderID ||
		strings.TrimSpace(provider.ProviderKey) == "" {
		return nil, mapCodeRunnerError(err)
	}
	selection, err := binding.Router.ResolveCodeRunnerSandbox(
		ctx,
		provider.ProviderKey,
		domainsandbox.ScopeAgent,
	)
	if err != nil || selection == nil {
		return nil, mapCodeRunnerError(err)
	}
	defer func() {
		cleanupCtx, cancel := detachedCodeRunnerContext(ctx, r.cleanupTimeout)
		cleanupErr := selection.Release(cleanupCtx)
		cancel()
		if cleanupErr != nil {
			response = nil
			returnErr = coderunner.ErrCodeRunnerUnavailable
		}
	}()

	policy, err := validateCodeRunnerPolicy(
		selection.CodeRunnerSandboxPolicy(),
		input.Language,
	)
	if err != nil {
		return nil, coderunner.ErrCodeRunnerUnavailable
	}
	now := r.currentTime()
	deadline := now.Add(time.Duration(policy.TimeoutSeconds) * time.Second)
	if callerDeadline, ok := ctx.Deadline(); ok && callerDeadline.Before(deadline) {
		deadline = callerDeadline
	}
	if !deadline.After(now) {
		return nil, coderunner.ErrCodeRunnerTimeout
	}
	request := infrasandbox.ExecuteRequest{
		Scope:          domainsandbox.ScopeAgent,
		WorkloadKind:   infrasandbox.WorkloadAgent,
		IdempotencyKey: "code_runner_" + operationID,
		Deadline:       deadline.UTC(),
		Policy:         policy,
		Entrypoint:     codeRunnerEntrypoint,
		Stdin:          stdin,
	}
	execution, err := selection.Execute(ctx, request)
	if err != nil && shouldReconcileCodeRunnerExecution(err) {
		reconcileCtx := ctx
		cancel := func() {}
		if ctx.Err() != nil || errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) {
			reconcileCtx, cancel = detachedCodeRunnerContext(ctx, r.cancelTimeout)
		}
		execution, err = selection.Execute(reconcileCtx, request)
		cancel()
	}
	if err != nil {
		return nil, mapCodeRunnerError(err)
	}
	switch execution.Status {
	case infrasandbox.ExecutionStatusAccepted, infrasandbox.ExecutionStatusRunning:
		cancelCtx, cancel := detachedCodeRunnerContext(ctx, r.cancelTimeout)
		cancelErr := selection.Cancel(cancelCtx, execution.ExecutionID)
		if cancelErr == nil {
			cancelErr = waitForCodeRunnerTerminal(cancelCtx, selection)
		}
		cancel()
		if cancelErr != nil {
			return nil, mapCodeRunnerError(cancelErr)
		}
		return nil, coderunner.ErrCodeRunnerUnavailable
	case infrasandbox.ExecutionStatusSucceeded:
		if execution.ExitCode == nil || *execution.ExitCode != 0 ||
			execution.Stderr != "" ||
			len(execution.Artifacts) != 0 ||
			len(execution.ArtifactDescriptors) != 0 ||
			execution.PreviewRoute != "" {
			return nil, coderunner.ErrCodeRunnerExecutionFailed
		}
		outputLimit := r.maxOutputBytes
		if policy.MaxOutputBytes > 0 && int64(outputLimit) > policy.MaxOutputBytes {
			outputLimit = int(policy.MaxOutputBytes)
		}
		if len(execution.Stdout) > outputLimit {
			return nil, coderunner.ErrCodeRunnerOutputLimit
		}
		result, err := infrasandbox.ParseCodeRunnerResultEnvelope(
			[]byte(execution.Stdout),
			outputLimit,
		)
		if err != nil {
			return nil, coderunner.ErrCodeRunnerExecutionFailed
		}
		return &coderunner.RunResponse{Result: result}, nil
	case infrasandbox.ExecutionStatusTimedOut:
		return nil, coderunner.ErrCodeRunnerTimeout
	case infrasandbox.ExecutionStatusCanceled:
		return nil, coderunner.ErrCodeRunnerCanceled
	case infrasandbox.ExecutionStatusFailed:
		return nil, coderunner.ErrCodeRunnerExecutionFailed
	default:
		return nil, coderunner.ErrCodeRunnerUnavailable
	}
}

func waitForCodeRunnerTerminal(ctx context.Context, selection Selection) error {
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

func validateCodeRunnerPolicy(
	input domainsandbox.RuntimePolicy,
	language coderunner.Language,
) (domainsandbox.RuntimePolicy, error) {
	policy, err := domainsandbox.NormalizeRuntimePolicy(input)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, err
	}
	allowed := false
	for _, executable := range policy.AllowedExecutables {
		if language == coderunner.Python &&
			(executable == "python" || executable == "python3") {
			allowed = true
		}
		if language == coderunner.JavaScript &&
			(executable == "node" || executable == "nodejs") {
			allowed = true
		}
	}
	if !allowed {
		return domainsandbox.RuntimePolicy{}, domainsandbox.ErrExecutionForbidden
	}
	return policy, nil
}

func marshalCodeRunnerParams(params map[string]any) ([]byte, error) {
	if params == nil {
		params = map[string]any{}
	}
	return json.Marshal(params)
}

func mapCodeRunnerError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return coderunner.ErrCodeRunnerCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return coderunner.ErrCodeRunnerTimeout
	case errors.Is(err, domainsandbox.ErrCapacityExhausted):
		return coderunner.ErrCodeRunnerCapacityExhausted
	case errors.Is(err, domainsandbox.ErrExecutionForbidden):
		return coderunner.ErrCodeRunnerExecutionFailed
	default:
		return coderunner.ErrCodeRunnerUnavailable
	}
}

func shouldReconcileCodeRunnerExecution(err error) bool {
	return infrasandbox.IsExecutionSubmissionUncertain(err) ||
		errors.Is(err, domainsandbox.ErrUnavailable) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded)
}

func newCodeRunnerOperationID(context.Context) (string, error) {
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", coderunner.ErrCodeRunnerUnavailable
	}
	return "operation_" + hex.EncodeToString(entropy[:]), nil
}

func detachedCodeRunnerContext(
	parent context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

func (r *runner) currentTime() time.Time {
	if r == nil || r.now == nil {
		return time.Now().UTC()
	}
	return r.now().UTC()
}
