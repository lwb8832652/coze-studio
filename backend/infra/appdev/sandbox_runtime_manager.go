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

package appdev

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	appDevSandboxEntrypoint      = "appdev/runtime"
	appDevSandboxPreviewPort     = 4173
	appDevSandboxCleanupTimeout  = 15 * time.Second
	maxAppDevManifestBytes       = 64 * 1024
	maxAppDevSnapshotURLBytes    = 8 * 1024
	maxAppDevRuntimeMessageBytes = 2 * 1024
	appDevRuntimeManifestVersion = "coze.appdev.runtime.v1"
)

// SandboxDefaultLookup and SandboxProviderLookup are intentionally narrow
// trusted read contracts. The concrete control-plane repository is injected by
// application wiring; no request field can select a Provider.
type SandboxDefaultLookup interface {
	GetProviderDefault(context.Context, domainsandbox.Scope) (*domainsandbox.ProviderDefault, error)
}

type SandboxProviderLookup interface {
	GetProvider(context.Context, int64) (*domainsandbox.Provider, error)
}

type ConfiguredRuntimeManagerOptions struct {
	Router    *appsandbox.ProviderRouter
	Defaults  SandboxDefaultLookup
	Providers SandboxProviderLookup
}

type appDevSandboxSelection interface {
	Policy() domainsandbox.RuntimePolicy
	Execute(context.Context, infrasandbox.ExecuteRequest) (infrasandbox.ExecuteResult, error)
	Cancel(context.Context, string) error
	Renew(context.Context) error
	Release(context.Context) error
}

type appDevSandboxRouter interface {
	ResolveAppDev(context.Context, string, domainsandbox.Scope) (appDevSandboxSelection, error)
}

type appDevDefaultProviderResolver interface {
	ResolveDefaultProviderKey(context.Context, domainsandbox.Scope) (string, error)
}

type providerRouterAdapter struct {
	router *appsandbox.ProviderRouter
}

type routedSandboxSelection struct {
	selected *appsandbox.SelectedProvider
}

func (r providerRouterAdapter) ResolveAppDev(
	ctx context.Context,
	providerKey string,
	scope domainsandbox.Scope,
) (appDevSandboxSelection, error) {
	if r.router == nil || scope != domainsandbox.ScopeAppDev {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	request, err := appsandbox.NewResolveProviderRequest(providerKey, scope)
	if err != nil {
		return nil, normalizeAppDevSandboxError(ctx, err)
	}
	selected, err := r.router.Resolve(ctx, request)
	if err != nil {
		return nil, normalizeAppDevSandboxError(ctx, err)
	}
	if selected == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	return &routedSandboxSelection{selected: selected}, nil
}

func (s *routedSandboxSelection) Policy() domainsandbox.RuntimePolicy {
	if s == nil || s.selected == nil {
		return domainsandbox.RuntimePolicy{}
	}
	return s.selected.Policy
}

func (s *routedSandboxSelection) Execute(
	ctx context.Context,
	request infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	return s.selected.Execute(ctx, request)
}

func (s *routedSandboxSelection) Cancel(ctx context.Context, executionID string) error {
	return s.selected.Cancel(ctx, executionID)
}

func (s *routedSandboxSelection) Renew(ctx context.Context) error {
	return s.selected.Renew(ctx)
}

func (s *routedSandboxSelection) Release(ctx context.Context) error {
	return s.selected.Release(ctx)
}

type repositoryDefaultProviderResolver struct {
	defaults  SandboxDefaultLookup
	providers SandboxProviderLookup
}

func (r repositoryDefaultProviderResolver) ResolveDefaultProviderKey(
	ctx context.Context,
	scope domainsandbox.Scope,
) (string, error) {
	if r.defaults == nil || r.providers == nil || scope != domainsandbox.ScopeAppDev {
		return "", domainsandbox.ErrConfigurationInvalid
	}
	defaultProvider, err := r.defaults.GetProviderDefault(ctx, scope)
	if err != nil {
		return "", normalizeAppDevSandboxError(ctx, err)
	}
	if defaultProvider == nil || defaultProvider.Scope != scope || defaultProvider.ProviderID <= 0 {
		return "", domainsandbox.ErrDefaultMissing
	}
	provider, err := r.providers.GetProvider(ctx, defaultProvider.ProviderID)
	if err != nil {
		return "", normalizeAppDevSandboxError(ctx, err)
	}
	if provider == nil || provider.ID != defaultProvider.ProviderID || provider.DeletedAt != nil ||
		domainsandbox.ValidateProviderKey(provider.ProviderKey) != nil {
		return "", domainsandbox.ErrUnavailable
	}
	return provider.ProviderKey, nil
}

type sandboxRuntimeManagerOptions struct {
	router         appDevSandboxRouter
	defaults       appDevDefaultProviderResolver
	previewBaseURL string
	now            func() time.Time
	newRuntimeID   func() (string, error)
}

type SandboxRuntimeManager struct {
	mu             sync.Mutex
	entries        map[string]*sandboxRuntimeEntry
	router         appDevSandboxRouter
	defaults       appDevDefaultProviderResolver
	previewBaseURL string
	now            func() time.Time
	newRuntimeID   func() (string, error)
}

type sandboxRuntimeEntry struct {
	runtimeID      string
	operation      string
	snapshotDigest string
	status         domainappdev.RuntimeStatus
	previewURL     string
	message        string
	lastKeepAlive  time.Time
	selection      appDevSandboxSelection
	cancelExecute  context.CancelFunc
	done           chan struct{}
	stopRequested  bool
	logs           []*domainappdev.RuntimeLog
}

type appDevRuntimeManifest struct {
	Schema         string  `json:"schema"`
	Operation      string  `json:"operation"`
	RuntimeID      string  `json:"runtime_id"`
	SnapshotRef    string  `json:"snapshot_ref"`
	PreviewPort    int     `json:"preview_port"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	MemoryLimitMB  int     `json:"memory_limit_mb"`
	CPULimit       float64 `json:"cpu_limit"`
	MaxOutputBytes int64   `json:"max_output_bytes"`
}

func newSandboxRuntimeManager(options sandboxRuntimeManagerOptions) (*SandboxRuntimeManager, error) {
	if options.router == nil || options.defaults == nil || options.now == nil || options.newRuntimeID == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	previewBaseURL, err := normalizePreviewGatewayBaseURL(options.previewBaseURL)
	if err != nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return &SandboxRuntimeManager{
		entries:        make(map[string]*sandboxRuntimeEntry),
		router:         options.router,
		defaults:       options.defaults,
		previewBaseURL: previewBaseURL,
		now:            options.now,
		newRuntimeID:   options.newRuntimeID,
	}, nil
}

func newConfiguredSandboxRuntimeManager(options ConfiguredRuntimeManagerOptions, previewBaseURL string) (*SandboxRuntimeManager, error) {
	if options.Router == nil || options.Defaults == nil || options.Providers == nil {
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	return newSandboxRuntimeManager(sandboxRuntimeManagerOptions{
		router: providerRouterAdapter{router: options.Router},
		defaults: repositoryDefaultProviderResolver{
			defaults: options.Defaults, providers: options.Providers,
		},
		previewBaseURL: previewBaseURL,
		now:            time.Now,
		newRuntimeID:   newAppDevRuntimeID,
	})
}

func (*SandboxRuntimeManager) RequiresSourceSnapshot() bool { return true }

func (m *SandboxRuntimeManager) Start(
	ctx context.Context,
	req *appdevapp.RuntimeManagerRequest,
) (*domainappdev.RuntimeInfo, error) {
	return m.start(ctx, req, "start")
}

func (m *SandboxRuntimeManager) Restart(
	ctx context.Context,
	req *appdevapp.RuntimeManagerRequest,
) (*domainappdev.RuntimeInfo, error) {
	if _, err := m.Stop(ctx, req); err != nil {
		return nil, err
	}
	return m.start(ctx, req, "restart")
}

func (m *SandboxRuntimeManager) start(
	ctx context.Context,
	req *appdevapp.RuntimeManagerRequest,
	operation string,
) (*domainappdev.RuntimeInfo, error) {
	if m == nil || ctx == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	if err := validateSandboxRuntimeRequest(req, true); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := sandboxRuntimeKey(req)

	m.mu.Lock()
	if existing := m.entries[key]; existing != nil && isSandboxRuntimeActive(existing.status) {
		if existing.snapshotDigest != req.Snapshot.Digest {
			m.mu.Unlock()
			return nil, domainsandbox.ErrExecutionForbidden
		}
		info := sandboxRuntimeInfo(existing)
		m.mu.Unlock()
		return info, nil
	}
	runtimeID := ""
	if existing := m.entries[key]; existing != nil && existing.status == domainappdev.RuntimeStatusError &&
		existing.snapshotDigest == req.Snapshot.Digest && existing.operation == operation && existing.selection == nil {
		runtimeID = existing.runtimeID
	}
	if runtimeID == "" {
		var err error
		runtimeID, err = m.newRuntimeID()
		if err != nil || !validAppDevRuntimeIdentifier(runtimeID) {
			m.mu.Unlock()
			return nil, domainsandbox.ErrUnavailable
		}
	}
	previewURL, err := buildTrustedPreviewURL(m.previewBaseURL, runtimeID)
	if err != nil {
		m.mu.Unlock()
		return nil, domainsandbox.ErrConfigurationInvalid
	}
	entry := &sandboxRuntimeEntry{
		runtimeID: runtimeID, operation: operation, snapshotDigest: req.Snapshot.Digest,
		status: domainappdev.RuntimeStatusStarting, previewURL: previewURL,
		message: "Sandbox 运行时启动中", lastKeepAlive: m.now().UTC(),
	}
	m.entries[key] = entry
	m.mu.Unlock()

	providerKey, err := m.defaults.ResolveDefaultProviderKey(ctx, domainsandbox.ScopeAppDev)
	if err != nil {
		return nil, m.failStart(key, entry, normalizeAppDevSandboxError(ctx, err))
	}
	selection, err := m.router.ResolveAppDev(ctx, providerKey, domainsandbox.ScopeAppDev)
	if err != nil {
		return nil, m.failStart(key, entry, normalizeAppDevSandboxError(ctx, err))
	}
	if selection == nil {
		return nil, m.failStart(key, entry, domainsandbox.ErrUnavailable)
	}
	executeRequest, err := buildAppDevExecuteRequest(
		m.now().UTC(), runtimeID, operation, req.SpaceID, req.ProjectID, req.ActorUserID, req.Snapshot, selection.Policy(),
	)
	if err != nil {
		_ = releaseAppDevSelection(selection)
		return nil, m.failStart(key, entry, err)
	}

	executeCtx, cancelExecute := context.WithCancel(context.Background())
	started := make(chan struct{})
	entry.selection = selection
	entry.cancelExecute = cancelExecute
	entry.done = make(chan struct{})
	m.mu.Lock()
	if m.entries[key] != entry {
		m.mu.Unlock()
		cancelExecute()
		_ = releaseAppDevSelection(selection)
		return nil, domainsandbox.ErrExecutionForbidden
	}
	m.mu.Unlock()
	go m.runExecution(key, entry, executeCtx, executeRequest, started)

	select {
	case <-started:
		m.mu.Lock()
		if m.entries[key] == entry && entry.status == domainappdev.RuntimeStatusStarting {
			entry.status = domainappdev.RuntimeStatusRunning
			entry.message = "Sandbox 运行时运行中"
		}
		info := sandboxRuntimeInfo(entry)
		m.mu.Unlock()
		return info, nil
	case <-ctx.Done():
		_, _ = m.Stop(context.Background(), req)
		return nil, ctx.Err()
	}
}

func (m *SandboxRuntimeManager) failStart(
	key string,
	entry *sandboxRuntimeEntry,
	err error,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[key] == entry {
		entry.status = domainappdev.RuntimeStatusError
		entry.previewURL = ""
		entry.message = safeAppDevRuntimeMessage(err)
	}
	return err
}

func (m *SandboxRuntimeManager) runExecution(
	key string,
	entry *sandboxRuntimeEntry,
	ctx context.Context,
	request infrasandbox.ExecuteRequest,
	started chan<- struct{},
) {
	close(started)
	result, executeErr := entry.selection.Execute(ctx, request)
	releaseErr := releaseAppDevSelection(entry.selection)

	m.mu.Lock()
	defer m.mu.Unlock()
	defer close(entry.done)
	if m.entries[key] != entry {
		return
	}
	entry.selection = nil
	entry.cancelExecute = nil
	entry.previewURL = ""
	appendSandboxRuntimeOutput(entry, "info", result.Stdout)
	appendSandboxRuntimeOutput(entry, "error", result.Stderr)

	if entry.stopRequested && (executeErr == nil || errors.Is(executeErr, context.Canceled)) && releaseErr == nil {
		entry.status = domainappdev.RuntimeStatusStopped
		entry.message = "Sandbox 运行时已停止"
		return
	}
	if executeErr != nil || releaseErr != nil {
		entry.status = domainappdev.RuntimeStatusError
		entry.message = safeAppDevRuntimeMessage(normalizeAppDevSandboxError(ctx, errors.Join(executeErr, releaseErr)))
		return
	}
	if result.ExecutionID != entry.runtimeID {
		entry.status = domainappdev.RuntimeStatusError
		entry.message = safeAppDevRuntimeMessage(domainsandbox.ErrUnavailable)
		return
	}
	switch result.Status {
	case infrasandbox.ExecutionStatusCanceled, infrasandbox.ExecutionStatusSucceeded:
		entry.status = domainappdev.RuntimeStatusStopped
		entry.message = "Sandbox 运行时已停止"
	case infrasandbox.ExecutionStatusFailed, infrasandbox.ExecutionStatusTimedOut:
		entry.status = domainappdev.RuntimeStatusError
		entry.message = "Sandbox 运行时执行失败"
	default:
		// AppDev executions are long-lived. Returning accepted/running would
		// make the Router selection impossible to cancel safely later.
		entry.status = domainappdev.RuntimeStatusError
		entry.message = safeAppDevRuntimeMessage(domainsandbox.ErrUnavailable)
	}
}

func (m *SandboxRuntimeManager) Status(
	ctx context.Context,
	req *appdevapp.RuntimeManagerRequest,
) (*domainappdev.RuntimeInfo, error) {
	if m == nil || ctx == nil || validateSandboxRuntimeRequest(req, false) != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[sandboxRuntimeKey(req)]
	if entry == nil {
		return stoppedSandboxRuntimeInfo(), nil
	}
	return sandboxRuntimeInfo(entry), nil
}

func (m *SandboxRuntimeManager) KeepAlive(
	ctx context.Context,
	req *appdevapp.RuntimeManagerRequest,
) (*domainappdev.RuntimeInfo, error) {
	if m == nil || ctx == nil || validateSandboxRuntimeRequest(req, false) != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	key := sandboxRuntimeKey(req)
	m.mu.Lock()
	entry := m.entries[key]
	if entry == nil || entry.selection == nil || !isSandboxRuntimeActive(entry.status) {
		m.mu.Unlock()
		return stoppedSandboxRuntimeInfo(), nil
	}
	selection := entry.selection
	m.mu.Unlock()
	if err := selection.Renew(ctx); err != nil {
		return nil, normalizeAppDevSandboxError(ctx, err)
	}
	m.mu.Lock()
	if m.entries[key] == entry {
		entry.lastKeepAlive = m.now().UTC()
	}
	info := sandboxRuntimeInfo(entry)
	m.mu.Unlock()
	return info, nil
}

func (m *SandboxRuntimeManager) Stop(
	ctx context.Context,
	req *appdevapp.RuntimeManagerRequest,
) (*domainappdev.RuntimeInfo, error) {
	if m == nil || ctx == nil || validateSandboxRuntimeRequest(req, false) != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	key := sandboxRuntimeKey(req)
	m.mu.Lock()
	entry := m.entries[key]
	if entry == nil {
		m.mu.Unlock()
		return stoppedSandboxRuntimeInfo(), nil
	}
	if entry.selection == nil || !isSandboxRuntimeActive(entry.status) {
		entry.status = domainappdev.RuntimeStatusStopped
		entry.previewURL = ""
		entry.message = "Sandbox 运行时已停止"
		info := sandboxRuntimeInfo(entry)
		m.mu.Unlock()
		return info, nil
	}
	entry.stopRequested = true
	selection := entry.selection
	runtimeID := entry.runtimeID
	cancelExecute := entry.cancelExecute
	done := entry.done
	m.mu.Unlock()

	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), appDevSandboxCleanupTimeout)
	defer cancel()
	cancelErr := selection.Cancel(cleanupCtx, runtimeID)
	if cancelExecute != nil {
		cancelExecute()
	}
	select {
	case <-done:
	case <-cleanupCtx.Done():
		return nil, domainsandbox.ErrUnavailable
	}
	if cancelErr != nil {
		return nil, normalizeAppDevSandboxError(cleanupCtx, cancelErr)
	}
	m.mu.Lock()
	info := sandboxRuntimeInfo(entry)
	m.mu.Unlock()
	if info.Status != domainappdev.RuntimeStatusStopped {
		return nil, domainsandbox.ErrUnavailable
	}
	return info, nil
}

func (m *SandboxRuntimeManager) Logs(
	ctx context.Context,
	req *appdevapp.RuntimeManagerRequest,
) ([]*domainappdev.RuntimeLog, error) {
	if m == nil || ctx == nil || validateSandboxRuntimeRequest(req, false) != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[sandboxRuntimeKey(req)]
	if entry == nil {
		return []*domainappdev.RuntimeLog{}, nil
	}
	logs := make([]*domainappdev.RuntimeLog, 0, len(entry.logs))
	for _, item := range entry.logs {
		if item == nil {
			continue
		}
		copyItem := *item
		logs = append(logs, &copyItem)
	}
	return logs, nil
}

func buildAppDevExecuteRequest(
	now time.Time,
	runtimeID string,
	operation string,
	spaceID string,
	projectID string,
	actorUserID int64,
	snapshot *appdevapp.RuntimeSnapshotReference,
	policy domainsandbox.RuntimePolicy,
) (infrasandbox.ExecuteRequest, error) {
	if now.IsZero() || !validAppDevRuntimeIdentifier(runtimeID) ||
		(operation != "start" && operation != "restart") || validateRuntimeSnapshot(snapshot) != nil {
		return infrasandbox.ExecuteRequest{}, domainsandbox.ErrInvalidInput
	}
	normalizedPolicy, err := domainsandbox.NormalizeRuntimePolicy(policy)
	if err != nil {
		return infrasandbox.ExecuteRequest{}, domainsandbox.ErrConfigurationInvalid
	}
	manifest, err := json.Marshal(appDevRuntimeManifest{
		Schema: appDevRuntimeManifestVersion, Operation: operation, RuntimeID: runtimeID,
		SnapshotRef: snapshot.ID, PreviewPort: appDevSandboxPreviewPort,
		TimeoutSeconds: normalizedPolicy.TimeoutSeconds, MemoryLimitMB: normalizedPolicy.MemoryLimitMB,
		CPULimit: normalizedPolicy.CPULimit, MaxOutputBytes: normalizedPolicy.MaxOutputBytes,
	})
	if err != nil || len(manifest) == 0 || len(manifest) > maxAppDevManifestBytes {
		return infrasandbox.ExecuteRequest{}, domainsandbox.ErrInvalidInput
	}
	deadline := now.Add(time.Duration(normalizedPolicy.TimeoutSeconds) * time.Second)
	if deadline.After(now.Add(infrasandbox.MaxExecutionDeadlineAhead)) {
		return infrasandbox.ExecuteRequest{}, domainsandbox.ErrConfigurationInvalid
	}
	request := infrasandbox.ExecuteRequest{
		Scope: domainsandbox.ScopeAppDev, WorkloadKind: infrasandbox.WorkloadAppDev,
		IdempotencyKey: runtimeID, Deadline: deadline, Policy: normalizedPolicy,
		Entrypoint: appDevSandboxEntrypoint,
		Args: []string{
			operation, "--runtime-id", runtimeID, "--snapshot-ref", snapshot.ID,
			"--port", fmt.Sprintf("%d", appDevSandboxPreviewPort),
		},
		Env: map[string]string{}, Stdin: manifest,
		Files: []infrasandbox.FileReference{{
			ID: snapshot.ID, Path: snapshot.Path, Digest: snapshot.Digest, Size: snapshot.Size,
		}},
	}
	if actorUserID > 0 {
		numericSpaceID, parseErr := strconv.ParseInt(strings.TrimSpace(spaceID), 10, 64)
		if parseErr != nil || numericSpaceID <= 0 || strings.TrimSpace(projectID) == "" {
			return infrasandbox.ExecuteRequest{}, domainsandbox.ErrInvalidInput
		}
		request.Identity = infrasandbox.ExecutionIdentity{
			SpaceID: numericSpaceID, UserID: actorUserID, ProjectID: strings.TrimSpace(projectID), ExecutionID: runtimeID,
		}
	}
	return request, nil
}

func validateSandboxRuntimeRequest(req *appdevapp.RuntimeManagerRequest, requireSnapshot bool) error {
	if req == nil || !validAppDevRuntimeIdentifier(strings.TrimSpace(req.SpaceID)) ||
		!validAppDevRuntimeIdentifier(strings.TrimSpace(req.ProjectID)) {
		return domainsandbox.ErrInvalidInput
	}
	if requireSnapshot {
		return validateRuntimeSnapshot(req.Snapshot)
	}
	return nil
}

func validateRuntimeSnapshot(snapshot *appdevapp.RuntimeSnapshotReference) error {
	if snapshot == nil || !validAppDevRuntimeIdentifier(snapshot.ID) || !validAppDevLogicalPath(snapshot.Path) ||
		!validAppDevDigest(snapshot.Digest) || snapshot.Size <= 0 || snapshot.Size > infrasandbox.MaxLogicalFileSizeBytes {
		return domainsandbox.ErrInvalidInput
	}
	if _, err := normalizeSnapshotDownloadURL(snapshot.DownloadURL); err != nil {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}

func normalizePreviewGatewayBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", domainsandbox.ErrConfigurationInvalid
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func normalizeSnapshotDownloadURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) == 0 || len(value) > maxAppDevSnapshotURLBytes || !utf8.ValidString(value) {
		return "", domainsandbox.ErrInvalidInput
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return "", domainsandbox.ErrInvalidInput
	}
	return parsed.String(), nil
}

func buildTrustedPreviewURL(baseURL string, runtimeID string) (string, error) {
	if !validAppDevRuntimeIdentifier(runtimeID) {
		return "", domainsandbox.ErrInvalidInput
	}
	candidate, err := url.JoinPath(baseURL, runtimeID)
	if err != nil {
		return "", domainsandbox.ErrConfigurationInvalid
	}
	candidate = strings.TrimRight(candidate, "/") + "/"
	if err := validateRemotePreviewURL(baseURL, candidate); err != nil {
		return "", domainsandbox.ErrConfigurationInvalid
	}
	return candidate, nil
}

func releaseAppDevSelection(selection appDevSandboxSelection) error {
	if selection == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), appDevSandboxCleanupTimeout)
	defer cancel()
	return normalizeAppDevSandboxError(ctx, selection.Release(ctx))
}

func normalizeAppDevSandboxError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	for _, safe := range []error{
		domainsandbox.ErrInvalidInput,
		domainsandbox.ErrProviderNotFound,
		domainsandbox.ErrProviderDisabled,
		domainsandbox.ErrDefaultMissing,
		domainsandbox.ErrProviderUnhealthy,
		domainsandbox.ErrCredentialInvalid,
		domainsandbox.ErrCapacityExhausted,
		domainsandbox.ErrExecutionForbidden,
		domainsandbox.ErrScopeUnsupported,
		domainsandbox.ErrConfigurationInvalid,
		domainsandbox.ErrLocalDebugUnavailable,
		domainsandbox.ErrUnavailable,
	} {
		if errors.Is(err, safe) {
			return safe
		}
	}
	return domainsandbox.ErrUnavailable
}

func newAppDevRuntimeID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", domainsandbox.ErrUnavailable
	}
	return "appdev-" + hex.EncodeToString(raw), nil
}

func sandboxRuntimeKey(req *appdevapp.RuntimeManagerRequest) string {
	return strings.TrimSpace(req.SpaceID) + ":" + strings.TrimSpace(req.ProjectID)
}

func sandboxRuntimeInfo(entry *sandboxRuntimeEntry) *domainappdev.RuntimeInfo {
	if entry == nil {
		return stoppedSandboxRuntimeInfo()
	}
	return &domainappdev.RuntimeInfo{
		Status: entry.status, PreviewURL: entry.previewURL,
		Message: entry.message, LastKeepAliveAt: entry.lastKeepAlive,
	}
}

func stoppedSandboxRuntimeInfo() *domainappdev.RuntimeInfo {
	return &domainappdev.RuntimeInfo{Status: domainappdev.RuntimeStatusStopped, Message: "开发环境未启动"}
}

func isSandboxRuntimeActive(status domainappdev.RuntimeStatus) bool {
	return status == domainappdev.RuntimeStatusStarting || status == domainappdev.RuntimeStatusRunning ||
		status == domainappdev.RuntimeStatusRestarting
}

func safeAppDevRuntimeMessage(err error) string {
	message := appdevapp.SanitizeAppDevOutput(normalizeAppDevSandboxError(context.Background(), err).Error())
	if len(message) > maxAppDevRuntimeMessageBytes {
		message = message[:maxAppDevRuntimeMessageBytes]
	}
	return message
}

func appendSandboxRuntimeOutput(entry *sandboxRuntimeEntry, level string, value string) {
	if entry == nil {
		return
	}
	message := strings.TrimSpace(appdevapp.SanitizeAppDevOutput(value))
	if message == "" {
		return
	}
	entry.logs = append(entry.logs, &domainappdev.RuntimeLog{
		ID: fmt.Sprintf("%s-%d", entry.runtimeID, len(entry.logs)+1), Level: level,
		Message: message, Timestamp: time.Now().UTC(),
	})
	if len(entry.logs) > 1000 {
		entry.logs = entry.logs[len(entry.logs)-1000:]
	}
}

func validAppDevRuntimeIdentifier(value string) bool {
	if len(value) == 0 || len(value) > infrasandbox.MaxIdentifierBytes {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || index > 0 && strings.ContainsRune("._-:", rune(character)) {
			continue
		}
		return false
	}
	return true
}

func validAppDevLogicalPath(value string) bool {
	return value != "" && len(value) <= infrasandbox.MaxLogicalPathBytes && utf8.ValidString(value) &&
		!path.IsAbs(value) && path.Clean(value) == value && value != "." &&
		!strings.ContainsAny(value, "\\:\x00") && !strings.Contains(value, "../")
}

func validAppDevDigest(value string) bool {
	if len(value) != infrasandbox.MaxDigestBytes || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	for _, character := range value[len("sha256:"):] {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				return false
			}
		}
	}
	return true
}
