// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"

	rootapplication "github.com/coze-dev/coze-studio/backend/application"
	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	userentity "github.com/coze-dev/coze-studio/backend/domain/user/entity"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/ctxcache"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
	typeconsts "github.com/coze-dev/coze-studio/backend/types/consts"
)

const (
	maxAdminSandboxBodyBytes      = 128 * 1024
	maxAdminSandboxRequestIDBytes = 128
)

var (
	errAdminSandboxBodyTooLarge = errors.New("admin sandbox request body too large")
	adminSandboxGetenv          = os.Getenv
)

type adminSandboxService interface {
	List(context.Context, appsandbox.Actor, appsandbox.ListProvidersRequest) (*appsandbox.ListProvidersResult, error)
	ListDefaults(context.Context, appsandbox.Actor) (*appsandbox.ListProviderDefaultsResult, error)
	GetSummary(context.Context, appsandbox.Actor) (*appsandbox.ProviderSummaryDTO, error)
	Create(context.Context, appsandbox.Actor, appsandbox.CreateProviderRequest) (*appsandbox.ProviderDTO, error)
	Get(context.Context, appsandbox.Actor, int64) (*appsandbox.ProviderDTO, error)
	Update(context.Context, appsandbox.Actor, appsandbox.UpdateProviderRequest) (*appsandbox.ProviderDTO, error)
	Delete(context.Context, appsandbox.Actor, appsandbox.DeleteProviderRequest) (*appsandbox.ProviderMutationResult, error)
	SetStatus(context.Context, appsandbox.Actor, appsandbox.SetProviderStatusRequest) (*appsandbox.ProviderDTO, error)
	ReplaceCredentials(context.Context, appsandbox.Actor, int64, uint64, appsandbox.SecretMutation) (*appsandbox.ProviderDTO, error)
	SetDefault(context.Context, appsandbox.Actor, appsandbox.SetProviderDefaultRequest) (*appsandbox.ProviderDefaultDTO, error)
	HealthCheck(context.Context, appsandbox.Actor, appsandbox.HealthCheckRequest) (*appsandbox.ProviderDTO, error)
	ListAuditEvents(context.Context, appsandbox.Actor, appsandbox.ListAuditEventsRequest) (*appsandbox.ListAuditEventsResult, error)
}

// adminSandboxSchedulerService is separate from the provider management
// contract, preserving existing provider-only handler injectables.
type adminSandboxSchedulerService interface {
	GetSchedulerSettings(context.Context, appsandbox.Actor) (*appsandbox.SchedulerSettingsDTO, error)
	UpdateSchedulerSettings(context.Context, appsandbox.Actor, appsandbox.UpdateSchedulerSettingsRequest) (*appsandbox.SchedulerSettingsUpdateResult, error)
	GetRuntimeStatus(context.Context, appsandbox.Actor) (*appsandbox.SchedulerRuntimeStatusDTO, error)
}

type applicationAdminSandboxService struct{}

func (applicationAdminSandboxService) current() (*appsandbox.Service, error) {
	if rootapplication.SandboxSVC == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	return rootapplication.SandboxSVC, nil
}

func (applicationAdminSandboxService) currentScheduler() (*appsandbox.SchedulerService, error) {
	if rootapplication.SandboxSchedulerSVC == nil {
		return nil, domainsandbox.ErrUnavailable
	}
	return rootapplication.SandboxSchedulerSVC, nil
}

func (a applicationAdminSandboxService) List(ctx context.Context, actor appsandbox.Actor, request appsandbox.ListProvidersRequest) (*appsandbox.ListProvidersResult, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.List(ctx, actor, request)
}

func (a applicationAdminSandboxService) ListDefaults(ctx context.Context, actor appsandbox.Actor) (*appsandbox.ListProviderDefaultsResult, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.ListDefaults(ctx, actor)
}

func (a applicationAdminSandboxService) GetSummary(ctx context.Context, actor appsandbox.Actor) (*appsandbox.ProviderSummaryDTO, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.GetSummary(ctx, actor)
}

func (a applicationAdminSandboxService) Create(ctx context.Context, actor appsandbox.Actor, request appsandbox.CreateProviderRequest) (*appsandbox.ProviderDTO, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.Create(ctx, actor, request)
}

func (a applicationAdminSandboxService) Get(ctx context.Context, actor appsandbox.Actor, providerID int64) (*appsandbox.ProviderDTO, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.Get(ctx, actor, providerID)
}

func (a applicationAdminSandboxService) Update(ctx context.Context, actor appsandbox.Actor, request appsandbox.UpdateProviderRequest) (*appsandbox.ProviderDTO, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.Update(ctx, actor, request)
}

func (a applicationAdminSandboxService) Delete(ctx context.Context, actor appsandbox.Actor, request appsandbox.DeleteProviderRequest) (*appsandbox.ProviderMutationResult, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.Delete(ctx, actor, request)
}

func (a applicationAdminSandboxService) SetStatus(ctx context.Context, actor appsandbox.Actor, request appsandbox.SetProviderStatusRequest) (*appsandbox.ProviderDTO, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.SetStatus(ctx, actor, request)
}

func (a applicationAdminSandboxService) ReplaceCredentials(
	ctx context.Context,
	actor appsandbox.Actor,
	providerID int64,
	expectedVersion uint64,
	mutation appsandbox.SecretMutation,
) (*appsandbox.ProviderDTO, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	provider, err := service.Get(ctx, actor, providerID)
	if err != nil {
		return nil, err
	}
	return service.Update(ctx, actor, appsandbox.UpdateProviderRequest{
		ProviderID:      providerID,
		ExpectedVersion: expectedVersion,
		Name:            provider.Name,
		Scopes:          provider.Scopes,
		Policy:          provider.Policy,
		Credential:      mutation,
	})
}

func (a applicationAdminSandboxService) SetDefault(ctx context.Context, actor appsandbox.Actor, request appsandbox.SetProviderDefaultRequest) (*appsandbox.ProviderDefaultDTO, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.SetDefault(ctx, actor, request)
}

func (a applicationAdminSandboxService) HealthCheck(ctx context.Context, actor appsandbox.Actor, request appsandbox.HealthCheckRequest) (*appsandbox.ProviderDTO, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.HealthCheck(ctx, actor, request)
}

func (a applicationAdminSandboxService) ListAuditEvents(ctx context.Context, actor appsandbox.Actor, request appsandbox.ListAuditEventsRequest) (*appsandbox.ListAuditEventsResult, error) {
	service, err := a.current()
	if err != nil {
		return nil, err
	}
	return service.ListAuditEvents(ctx, actor, request)
}

func (a applicationAdminSandboxService) GetSchedulerSettings(ctx context.Context, actor appsandbox.Actor) (*appsandbox.SchedulerSettingsDTO, error) {
	service, err := a.currentScheduler()
	if err != nil {
		return nil, err
	}
	return service.Get(ctx, actor)
}

func (a applicationAdminSandboxService) UpdateSchedulerSettings(ctx context.Context, actor appsandbox.Actor, request appsandbox.UpdateSchedulerSettingsRequest) (*appsandbox.SchedulerSettingsUpdateResult, error) {
	service, err := a.currentScheduler()
	if err != nil {
		return nil, err
	}
	return service.Update(ctx, actor, request)
}

func (a applicationAdminSandboxService) GetRuntimeStatus(ctx context.Context, actor appsandbox.Actor) (*appsandbox.SchedulerRuntimeStatusDTO, error) {
	service, err := a.currentScheduler()
	if err != nil {
		return &appsandbox.SchedulerRuntimeStatusDTO{ReasonCode: appsandbox.SchedulerReasonProviderUnavailable}, nil
	}
	return service.RuntimeStatus(ctx, actor)
}

type adminSandboxHandler struct {
	service adminSandboxService
}

func newAdminSandboxHandler(service adminSandboxService) *adminSandboxHandler {
	return &adminSandboxHandler{service: service}
}

func defaultAdminSandboxHandler() *adminSandboxHandler {
	return newAdminSandboxHandler(applicationAdminSandboxService{})
}

// AdminSandboxRouteHandlers exposes only HTTP handlers to the router package.
// The application service is resolved per request so control-plane wiring can
// fail closed without leaking the internal service contract.
type AdminSandboxRouteHandlers struct {
	List                    app.HandlerFunc
	ListDefaults            app.HandlerFunc
	GetSummary              app.HandlerFunc
	Capabilities            app.HandlerFunc
	Create                  app.HandlerFunc
	Get                     app.HandlerFunc
	Update                  app.HandlerFunc
	Delete                  app.HandlerFunc
	Enable                  app.HandlerFunc
	Disable                 app.HandlerFunc
	Credentials             app.HandlerFunc
	SetDefault              app.HandlerFunc
	Health                  app.HandlerFunc
	AuditEvents             app.HandlerFunc
	SchedulerSettings       app.HandlerFunc
	UpdateSchedulerSettings app.HandlerFunc
	RuntimeStatus           app.HandlerFunc
}

func DefaultAdminSandboxRouteHandlers() *AdminSandboxRouteHandlers {
	handler := defaultAdminSandboxHandler()
	return &AdminSandboxRouteHandlers{
		List:                    handler.list,
		ListDefaults:            handler.defaults,
		GetSummary:              handler.summary,
		Capabilities:            handler.capabilities,
		Create:                  handler.create,
		Get:                     handler.get,
		Update:                  handler.update,
		Delete:                  handler.delete,
		Enable:                  handler.enable,
		Disable:                 handler.disable,
		Credentials:             handler.credentials,
		SetDefault:              handler.setDefault,
		Health:                  handler.health,
		AuditEvents:             handler.auditEvents,
		SchedulerSettings:       handler.schedulerSettings,
		UpdateSchedulerSettings: handler.updateSchedulerSettings,
		RuntimeStatus:           handler.runtimeStatus,
	}
}

type adminSandboxEnvelope struct {
	Data        any               `json:"data,omitempty"`
	Code        int               `json:"code"`
	ErrorCode   string            `json:"error_code,omitempty"`
	Msg         string            `json:"msg"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
}

type createAdminSandboxRequest struct {
	Name       string                           `json:"name"`
	Type       domainsandbox.ProviderType       `json:"type"`
	Endpoint   string                           `json:"endpoint"`
	Credential string                           `json:"credential"`
	Scopes     []domainsandbox.Scope            `json:"scopes"`
	Policy     adminSandboxRuntimePolicyRequest `json:"policy"`
}

type adminSandboxSecretMutationRequest struct {
	Mode  string `json:"mode"`
	Value string `json:"value"`
}

type updateAdminSandboxRequest struct {
	ExpectedVersion uint64                             `json:"expected_version"`
	Name            string                             `json:"name"`
	Scopes          []domainsandbox.Scope              `json:"scopes"`
	Policy          adminSandboxRuntimePolicyRequest   `json:"policy"`
	Endpoint        *adminSandboxSecretMutationRequest `json:"endpoint,omitempty"`
	Credential      *adminSandboxSecretMutationRequest `json:"credential,omitempty"`
}

type adminSandboxRuntimePolicyRequest struct {
	TimeoutSeconds          *int                           `json:"timeout_seconds,omitempty"`
	LegacyTimeoutSeconds    *int                           `json:"TimeoutSeconds,omitempty"`
	MemoryLimitMB           *int                           `json:"memory_limit_mb,omitempty"`
	LegacyMemoryLimitMB     *int                           `json:"MemoryLimitMB,omitempty"`
	CPULimit                *float64                       `json:"cpu_limit,omitempty"`
	LegacyCPULimit          *float64                       `json:"CPULimit,omitempty"`
	MaxOutputBytes          *int64                         `json:"max_output_bytes,omitempty"`
	LegacyMaxOutputBytes    *int64                         `json:"MaxOutputBytes,omitempty"`
	MaxConcurrency          *int                           `json:"max_concurrency,omitempty"`
	LegacyMaxConcurrency    *int                           `json:"MaxConcurrency,omitempty"`
	AllowNetwork            *bool                          `json:"allow_network,omitempty"`
	LegacyAllowNetwork      *bool                          `json:"AllowNetwork,omitempty"`
	NetworkAllowlist        *[]string                      `json:"network_allowlist,omitempty"`
	LegacyNetworkAllowlist  *[]string                      `json:"NetworkAllowlist,omitempty"`
	AllowedEnvNames         *[]string                      `json:"allowed_env_names,omitempty"`
	VirtualReadPrefixes     *[]string                      `json:"virtual_read_prefixes,omitempty"`
	VirtualWritePrefixes    *[]string                      `json:"virtual_write_prefixes,omitempty"`
	AllowedExecutables      *[]string                      `json:"allowed_executables,omitempty"`
	FFIEnabled              *bool                          `json:"ffi_enabled,omitempty"`
	NodeModulesMode         *domainsandbox.NodeModulesMode `json:"node_modules_mode,omitempty"`
	NodeModulesDirectoryRef *string                        `json:"node_modules_directory_ref,omitempty"`
}

func (r adminSandboxRuntimePolicyRequest) domainPolicy() (domainsandbox.RuntimePolicy, error) {
	timeoutSeconds, err := adminSandboxPolicyField(r.TimeoutSeconds, r.LegacyTimeoutSeconds)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, err
	}
	memoryLimitMB, err := adminSandboxPolicyField(r.MemoryLimitMB, r.LegacyMemoryLimitMB)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, err
	}
	cpuLimit, err := adminSandboxPolicyField(r.CPULimit, r.LegacyCPULimit)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, err
	}
	maxOutputBytes, err := adminSandboxPolicyField(r.MaxOutputBytes, r.LegacyMaxOutputBytes)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, err
	}
	maxConcurrency, err := adminSandboxPolicyField(r.MaxConcurrency, r.LegacyMaxConcurrency)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, err
	}
	allowNetwork, err := adminSandboxPolicyField(r.AllowNetwork, r.LegacyAllowNetwork)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, err
	}
	networkAllowlist, err := adminSandboxPolicyField(r.NetworkAllowlist, r.LegacyNetworkAllowlist)
	if err != nil {
		return domainsandbox.RuntimePolicy{}, err
	}
	return domainsandbox.RuntimePolicy{
		TimeoutSeconds: timeoutSeconds, MemoryLimitMB: memoryLimitMB, CPULimit: cpuLimit,
		MaxOutputBytes: maxOutputBytes, MaxConcurrency: maxConcurrency, AllowNetwork: allowNetwork,
		NetworkAllowlist:        networkAllowlist,
		AllowedEnvNames:         adminSandboxOptionalPolicyField(r.AllowedEnvNames),
		VirtualReadPrefixes:     adminSandboxOptionalPolicyField(r.VirtualReadPrefixes),
		VirtualWritePrefixes:    adminSandboxOptionalPolicyField(r.VirtualWritePrefixes),
		AllowedExecutables:      adminSandboxOptionalPolicyField(r.AllowedExecutables),
		FFIEnabled:              adminSandboxOptionalPolicyField(r.FFIEnabled),
		NodeModulesMode:         adminSandboxOptionalPolicyField(r.NodeModulesMode),
		NodeModulesDirectoryRef: adminSandboxOptionalPolicyField(r.NodeModulesDirectoryRef),
	}, nil
}

func adminSandboxOptionalPolicyField[T any](value *T) T {
	var zero T
	if value == nil {
		return zero
	}
	return *value
}

func adminSandboxPolicyField[T any](canonical, legacy *T) (T, error) {
	var zero T
	if canonical != nil && legacy != nil {
		return zero, domainsandbox.ErrInvalidInput
	}
	if canonical != nil {
		return *canonical, nil
	}
	if legacy != nil {
		return *legacy, nil
	}
	return zero, nil
}

type versionedAdminSandboxRequest struct {
	ExpectedVersion uint64 `json:"expected_version"`
}

type credentialsAdminSandboxRequest struct {
	ExpectedVersion uint64 `json:"expected_version"`
	Mode            string `json:"mode"`
	Credential      string `json:"credential,omitempty"`
}

type defaultAdminSandboxRequest struct {
	ExpectedVersion        uint64              `json:"expected_version"`
	DefaultExpectedVersion uint64              `json:"default_expected_version"`
	Scope                  domainsandbox.Scope `json:"scope"`
}

type updateAdminSandboxSchedulerSettingsRequest struct {
	ExpectedVersion uint64                          `json:"expected_version"`
	Settings        domainsandbox.SchedulerSettings `json:"settings"`
}

func (h *adminSandboxHandler) list(ctx context.Context, c *app.RequestContext) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return
	}
	query, err := decodeAdminSandboxQuery(c, "keyword", "type", "status", "health", "scope", "offset", "limit")
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	request := appsandbox.ListProvidersRequest{
		Keyword: query["keyword"],
		Type:    domainsandbox.ProviderType(query["type"]),
		Status:  domainsandbox.ProviderStatus(query["status"]),
		Health:  domainsandbox.HealthStatus(query["health"]),
		Scope:   domainsandbox.Scope(query["scope"]),
	}
	if request.Offset, err = parseAdminSandboxPageValue(query["offset"], true); err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	if request.Limit, err = parseAdminSandboxPageValue(query["limit"], false); err != nil ||
		!validAdminSandboxProviderType(request.Type, true) ||
		!validAdminSandboxProviderStatus(request.Status, true) ||
		!validAdminSandboxHealthStatus(request.Health, true) ||
		!validAdminSandboxScope(request.Scope, true) {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	result, err := h.service.List(ctx, actor, request)
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) defaults(ctx context.Context, c *app.RequestContext) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return
	}
	result, err := h.service.ListDefaults(ctx, actor)
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) summary(ctx context.Context, c *app.RequestContext) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return
	}
	result, err := h.service.GetSummary(ctx, actor)
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) capabilities(ctx context.Context, c *app.RequestContext) {
	if _, ok := adminSandboxActor(ctx, c); !ok {
		return
	}
	adminSandboxResult(ctx, c, appsandbox.ProjectCapabilities(adminSandboxGetenv), nil)
}

func (h *adminSandboxHandler) create(ctx context.Context, c *app.RequestContext) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return
	}
	var request createAdminSandboxRequest
	if err := decodeAdminSandboxJSON(c, &request); err != nil {
		adminSandboxError(ctx, c, err)
		return
	}
	if !validAdminSandboxProviderType(request.Type, false) ||
		len(request.Endpoint) > appsandbox.MaxEndpointURLBytes ||
		len(request.Credential) > infrasandbox.MaxProviderCredentialBytes {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	policy, err := request.Policy.domainPolicy()
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	result, err := h.service.Create(ctx, actor, appsandbox.CreateProviderRequest{
		Name:       request.Name,
		Type:       request.Type,
		Endpoint:   []byte(request.Endpoint),
		Credential: []byte(request.Credential),
		Scopes:     request.Scopes,
		Policy:     policy,
	})
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) get(ctx context.Context, c *app.RequestContext) {
	actor, providerID, ok := adminSandboxActorAndProviderID(ctx, c)
	if !ok {
		return
	}
	result, err := h.service.Get(ctx, actor, providerID)
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) update(ctx context.Context, c *app.RequestContext) {
	actor, providerID, ok := adminSandboxActorAndProviderID(ctx, c)
	if !ok {
		return
	}
	var request updateAdminSandboxRequest
	if err := decodeAdminSandboxJSON(c, &request); err != nil {
		adminSandboxError(ctx, c, err)
		return
	}
	if request.ExpectedVersion == 0 {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	endpoint, err := adminSandboxSecretMutation(request.Endpoint, appsandbox.MaxEndpointURLBytes, false)
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	credential, err := adminSandboxSecretMutation(request.Credential, infrasandbox.MaxProviderCredentialBytes, true)
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	policy, err := request.Policy.domainPolicy()
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	result, err := h.service.Update(ctx, actor, appsandbox.UpdateProviderRequest{
		ProviderID: providerID, ExpectedVersion: request.ExpectedVersion, Name: request.Name,
		Scopes: request.Scopes, Policy: policy, Endpoint: endpoint, Credential: credential,
	})
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) delete(ctx context.Context, c *app.RequestContext) {
	actor, providerID, request, ok := adminSandboxVersionedRequest(ctx, c)
	if !ok {
		return
	}
	result, err := h.service.Delete(ctx, actor, appsandbox.DeleteProviderRequest{
		ProviderID: providerID, ExpectedVersion: request.ExpectedVersion,
	})
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) enable(ctx context.Context, c *app.RequestContext) {
	h.setStatus(ctx, c, domainsandbox.ProviderStatusEnabled)
}

func (h *adminSandboxHandler) disable(ctx context.Context, c *app.RequestContext) {
	h.setStatus(ctx, c, domainsandbox.ProviderStatusDisabled)
}

func (h *adminSandboxHandler) setStatus(ctx context.Context, c *app.RequestContext, status domainsandbox.ProviderStatus) {
	actor, providerID, request, ok := adminSandboxVersionedRequest(ctx, c)
	if !ok {
		return
	}
	result, err := h.service.SetStatus(ctx, actor, appsandbox.SetProviderStatusRequest{
		ProviderID: providerID, ExpectedVersion: request.ExpectedVersion, Status: status,
	})
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) credentials(ctx context.Context, c *app.RequestContext) {
	actor, providerID, ok := adminSandboxActorAndProviderID(ctx, c)
	if !ok {
		return
	}
	var request credentialsAdminSandboxRequest
	if err := decodeAdminSandboxJSON(c, &request); err != nil {
		adminSandboxError(ctx, c, err)
		return
	}
	if request.ExpectedVersion == 0 {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	mutation, err := adminSandboxSecretMutation(&adminSandboxSecretMutationRequest{
		Mode: request.Mode, Value: request.Credential,
	}, infrasandbox.MaxProviderCredentialBytes, true)
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	result, err := h.service.ReplaceCredentials(ctx, actor, providerID, request.ExpectedVersion, mutation)
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) setDefault(ctx context.Context, c *app.RequestContext) {
	actor, providerID, ok := adminSandboxActorAndProviderID(ctx, c)
	if !ok {
		return
	}
	var request defaultAdminSandboxRequest
	if err := decodeAdminSandboxJSON(c, &request); err != nil {
		adminSandboxError(ctx, c, err)
		return
	}
	if request.ExpectedVersion == 0 || !validAdminSandboxScope(request.Scope, false) {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	result, err := h.service.SetDefault(ctx, actor, appsandbox.SetProviderDefaultRequest{
		ProviderID: providerID, ProviderExpectedVersion: request.ExpectedVersion,
		ExpectedVersion: request.DefaultExpectedVersion, Scope: request.Scope,
	})
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) health(ctx context.Context, c *app.RequestContext) {
	actor, providerID, request, ok := adminSandboxVersionedRequest(ctx, c)
	if !ok {
		return
	}
	result, err := h.service.HealthCheck(ctx, actor, appsandbox.HealthCheckRequest{
		ProviderID: providerID, ExpectedVersion: request.ExpectedVersion,
	})
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) auditEvents(ctx context.Context, c *app.RequestContext) {
	actor, providerID, ok := adminSandboxActorAndProviderID(ctx, c)
	if !ok {
		return
	}
	query, err := decodeAdminSandboxQuery(c, "action", "result", "offset", "limit")
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	request := appsandbox.ListAuditEventsRequest{
		ProviderID: providerID, Action: query["action"], Result: query["result"],
	}
	if request.Offset, err = parseAdminSandboxPageValue(query["offset"], true); err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	if request.Limit, err = parseAdminSandboxPageValue(query["limit"], false); err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	result, err := h.service.ListAuditEvents(ctx, actor, request)
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) schedulerSettings(ctx context.Context, c *app.RequestContext) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return
	}
	service, ok := h.service.(adminSandboxSchedulerService)
	if !ok {
		adminSandboxError(ctx, c, domainsandbox.ErrUnavailable)
		return
	}
	result, err := service.GetSchedulerSettings(ctx, actor)
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) updateSchedulerSettings(ctx context.Context, c *app.RequestContext) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return
	}
	var request updateAdminSandboxSchedulerSettingsRequest
	if err := decodeAdminSandboxJSON(c, &request); err != nil {
		adminSandboxError(ctx, c, err)
		return
	}
	if request.ExpectedVersion == 0 {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	settingsJSON, err := json.Marshal(request.Settings)
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	settings, err := domainsandbox.DecodeSchedulerSettingsJSON(settingsJSON)
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return
	}
	service, ok := h.service.(adminSandboxSchedulerService)
	if !ok {
		adminSandboxError(ctx, c, domainsandbox.ErrUnavailable)
		return
	}
	result, err := service.UpdateSchedulerSettings(ctx, actor, appsandbox.UpdateSchedulerSettingsRequest{ExpectedVersion: request.ExpectedVersion, Settings: settings})
	adminSandboxResult(ctx, c, result, err)
}

func (h *adminSandboxHandler) runtimeStatus(ctx context.Context, c *app.RequestContext) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return
	}
	service, ok := h.service.(adminSandboxSchedulerService)
	if !ok {
		adminSandboxResult(ctx, c, &appsandbox.SchedulerRuntimeStatusDTO{ReasonCode: appsandbox.SchedulerReasonProviderUnavailable}, nil)
		return
	}
	result, err := service.GetRuntimeStatus(ctx, actor)
	adminSandboxResult(ctx, c, result, err)
}

func adminSandboxActor(ctx context.Context, c *app.RequestContext) (appsandbox.Actor, bool) {
	session, authenticated := ctxcache.Get[*userentity.Session](ctx, typeconsts.SessionDataKeyInCtx)
	if !authenticated || session == nil || session.UserID <= 0 {
		c.AbortWithStatusJSON(http.StatusUnauthorized, adminSandboxEnvelope{
			Code: http.StatusUnauthorized, ErrorCode: "UNAUTHENTICATED", Msg: "authentication required",
		})
		return appsandbox.Actor{}, false
	}
	requestID := string(c.Request.Header.Peek("X-Request-ID"))
	if len(requestID) > maxAdminSandboxRequestIDBytes {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return appsandbox.Actor{}, false
	}
	return appsandbox.Actor{
		UserID: session.UserID, SystemAdmin: true, RequestID: requestID,
		AllowedScopes: []domainsandbox.Scope{
			domainsandbox.ScopeAgent, domainsandbox.ScopeMCPStdio, domainsandbox.ScopeAppDev,
		},
	}, true
}

func adminSandboxActorAndProviderID(ctx context.Context, c *app.RequestContext) (appsandbox.Actor, int64, bool) {
	actor, ok := adminSandboxActor(ctx, c)
	if !ok {
		return appsandbox.Actor{}, 0, false
	}
	providerID, err := parseAdminSandboxProviderID(c.Param("id"))
	if err != nil {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return appsandbox.Actor{}, 0, false
	}
	return actor, providerID, true
}

func adminSandboxVersionedRequest(
	ctx context.Context,
	c *app.RequestContext,
) (appsandbox.Actor, int64, versionedAdminSandboxRequest, bool) {
	actor, providerID, ok := adminSandboxActorAndProviderID(ctx, c)
	if !ok {
		return appsandbox.Actor{}, 0, versionedAdminSandboxRequest{}, false
	}
	var request versionedAdminSandboxRequest
	if err := decodeAdminSandboxJSON(c, &request); err != nil {
		adminSandboxError(ctx, c, err)
		return appsandbox.Actor{}, 0, versionedAdminSandboxRequest{}, false
	}
	if request.ExpectedVersion == 0 {
		adminSandboxError(ctx, c, domainsandbox.ErrInvalidInput)
		return appsandbox.Actor{}, 0, versionedAdminSandboxRequest{}, false
	}
	return actor, providerID, request, true
}

func parseAdminSandboxProviderID(raw string) (int64, error) {
	if raw == "" || raw[0] == '0' || strings.TrimSpace(raw) != raw {
		return 0, domainsandbox.ErrInvalidInput
	}
	providerID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || providerID <= 0 || strconv.FormatInt(providerID, 10) != raw {
		return 0, domainsandbox.ErrInvalidInput
	}
	return providerID, nil
}

func adminSandboxSecretMutation(
	request *adminSandboxSecretMutationRequest,
	maxBytes int,
	allowClear bool,
) (appsandbox.SecretMutation, error) {
	if request == nil {
		return appsandbox.SecretMutation{}, nil
	}
	switch request.Mode {
	case string(appsandbox.SecretMutationReplace):
		if request.Value == "" || len(request.Value) > maxBytes {
			return appsandbox.SecretMutation{}, domainsandbox.ErrInvalidInput
		}
		return appsandbox.SecretMutation{Mode: appsandbox.SecretMutationReplace, Value: []byte(request.Value)}, nil
	case string(appsandbox.SecretMutationClear):
		if !allowClear || request.Value != "" {
			return appsandbox.SecretMutation{}, domainsandbox.ErrInvalidInput
		}
		return appsandbox.SecretMutation{Mode: appsandbox.SecretMutationClear}, nil
	default:
		return appsandbox.SecretMutation{}, domainsandbox.ErrInvalidInput
	}
}

func decodeAdminSandboxJSON(c *app.RequestContext, target any) error {
	contentType, _, err := mime.ParseMediaType(string(c.Request.Header.ContentType()))
	if err != nil || contentType != "application/json" {
		return domainsandbox.ErrInvalidInput
	}
	if contentLength := c.Request.Header.ContentLength(); contentLength > maxAdminSandboxBodyBytes {
		return errAdminSandboxBodyTooLarge
	}
	if !c.Request.IsBodyStream() {
		return domainsandbox.ErrInvalidInput
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.BodyStream(), maxAdminSandboxBodyBytes+1))
	_ = c.Request.CloseBodyStream()
	if err != nil || len(body) == 0 {
		return domainsandbox.ErrInvalidInput
	}
	if len(body) > maxAdminSandboxBodyBytes {
		return errAdminSandboxBodyTooLarge
	}
	if !utf8.Valid(body) {
		return domainsandbox.ErrInvalidInput
	}
	if err := validateAdminSandboxJSON(body, reflect.TypeOf(target)); err != nil {
		return domainsandbox.ErrInvalidInput
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domainsandbox.ErrInvalidInput
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}

func validateAdminSandboxJSON(body []byte, targetType reflect.Type) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domainsandbox.ErrInvalidInput
	}
	return validateAdminSandboxJSONValue(raw, targetType)
}

func validateAdminSandboxJSONValue(raw json.RawMessage, targetType reflect.Type) error {
	for targetType.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		targetType = targetType.Elem()
	}
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		if targetType.Kind() == reflect.Slice || targetType.Kind() == reflect.Map || targetType.Kind() == reflect.Interface {
			return nil
		}
		return domainsandbox.ErrInvalidInput
	}
	switch targetType.Kind() {
	case reflect.Struct:
		return validateAdminSandboxJSONObject(trimmed, targetType)
	case reflect.Slice, reflect.Array:
		var values []json.RawMessage
		if err := json.Unmarshal(trimmed, &values); err != nil {
			return err
		}
		for _, value := range values {
			if err := validateAdminSandboxJSONValue(value, targetType.Elem()); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if targetType.Key().Kind() != reflect.String {
			return domainsandbox.ErrInvalidInput
		}
		return validateAdminSandboxJSONMap(trimmed, targetType.Elem())
	default:
		value := reflect.New(targetType).Interface()
		return json.Unmarshal(trimmed, value)
	}
}

func validateAdminSandboxJSONObject(raw []byte, targetType reflect.Type) error {
	fields := make(map[string]reflect.Type)
	for index := 0; index < targetType.NumField(); index++ {
		field := targetType.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name := field.Name
		if tag := strings.Split(field.Tag.Get("json"), ",")[0]; tag != "" {
			if tag == "-" {
				continue
			}
			name = tag
		}
		fields[name] = field.Type
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return domainsandbox.ErrInvalidInput
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		fieldType, known := fields[name]
		if !ok || !known {
			return domainsandbox.ErrInvalidInput
		}
		if _, duplicate := seen[name]; duplicate {
			return domainsandbox.ErrInvalidInput
		}
		seen[name] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if err := validateAdminSandboxJSONValue(value, fieldType); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}

func validateAdminSandboxJSONMap(raw []byte, valueType reflect.Type) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return domainsandbox.ErrInvalidInput
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name, ok := token.(string)
		if !ok {
			return domainsandbox.ErrInvalidInput
		}
		if _, duplicate := seen[name]; duplicate {
			return domainsandbox.ErrInvalidInput
		}
		seen[name] = struct{}{}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if err := validateAdminSandboxJSONValue(value, valueType); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func decodeAdminSandboxQuery(c *app.RequestContext, allowed ...string) (map[string]string, error) {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	values := make(map[string]string, len(allowed))
	var queryErr error
	c.Request.URI().QueryArgs().VisitAll(func(key, value []byte) {
		if queryErr != nil {
			return
		}
		name := string(key)
		if _, ok := allowedSet[name]; !ok {
			queryErr = domainsandbox.ErrInvalidInput
			return
		}
		if _, duplicate := values[name]; duplicate {
			queryErr = domainsandbox.ErrInvalidInput
			return
		}
		values[name] = string(value)
	})
	return values, queryErr
}

func parseAdminSandboxPageValue(raw string, offset bool) (int, error) {
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 || strconv.Itoa(value) != raw {
		return 0, domainsandbox.ErrInvalidInput
	}
	if offset {
		if value > domainsandbox.MaxPageOffset {
			return 0, domainsandbox.ErrInvalidInput
		}
	} else if value == 0 || value > domainsandbox.MaxPageLimit {
		return 0, domainsandbox.ErrInvalidInput
	}
	return value, nil
}

func validAdminSandboxProviderType(value domainsandbox.ProviderType, optional bool) bool {
	return optional && value == "" || value == domainsandbox.ProviderTypeRemoteHTTP || value == domainsandbox.ProviderTypeLocalDebug
}

func validAdminSandboxProviderStatus(value domainsandbox.ProviderStatus, optional bool) bool {
	return optional && value == "" || value == domainsandbox.ProviderStatusEnabled || value == domainsandbox.ProviderStatusDisabled
}

func validAdminSandboxHealthStatus(value domainsandbox.HealthStatus, optional bool) bool {
	switch value {
	case domainsandbox.HealthStatusUnknown, domainsandbox.HealthStatusHealthy,
		domainsandbox.HealthStatusDegraded, domainsandbox.HealthStatusUnhealthy:
		return true
	default:
		return optional && value == ""
	}
}

func validAdminSandboxScope(value domainsandbox.Scope, optional bool) bool {
	return optional && value == "" || value == domainsandbox.ScopeAgent || value == domainsandbox.ScopeMCPStdio || value == domainsandbox.ScopeAppDev
}

type adminSandboxResponsePayload interface {
	*appsandbox.ListProvidersResult |
		*appsandbox.ListProviderDefaultsResult |
		*appsandbox.ProviderSummaryDTO |
		*appsandbox.SandboxCapabilitiesDTO |
		*appsandbox.ProviderDTO |
		*appsandbox.ProviderMutationResult |
		*appsandbox.ProviderDefaultDTO |
		*appsandbox.ListAuditEventsResult |
		*appsandbox.SchedulerSettingsDTO |
		*appsandbox.SchedulerSettingsUpdateResult |
		*appsandbox.SchedulerRuntimeStatusDTO
}

func adminSandboxResult[T adminSandboxResponsePayload](ctx context.Context, c *app.RequestContext, result T, err error) {
	if err != nil {
		adminSandboxError(ctx, c, err)
		return
	}
	c.JSON(http.StatusOK, adminSandboxEnvelope{Data: result, Code: 0, Msg: ""})
}

func adminSandboxError(ctx context.Context, c *app.RequestContext, err error) {
	status, code, message := adminSandboxErrorContract(err)
	if status == http.StatusInternalServerError {
		logs.CtxErrorf(ctx, "[AdminSandbox] request failed: %v", err)
	}
	c.AbortWithStatusJSON(status, adminSandboxEnvelope{
		Code: status, ErrorCode: code, Msg: message, FieldErrors: adminSandboxFieldErrors(err),
	})
}

func adminSandboxFieldErrors(err error) map[string]string {
	switch {
	case errors.Is(err, errAdminSandboxBodyTooLarge):
		return map[string]string{"request": "请求体过大"}
	case errors.Is(err, domainsandbox.ErrLocalDebugUnavailable):
		return map[string]string{"type": "当前环境不支持本地调试 Sandbox"}
	case errors.Is(err, domainsandbox.ErrScopeUnsupported):
		return map[string]string{"scopes": "所选作用域不受支持"}
	case errors.Is(err, domainsandbox.ErrCredentialInvalid):
		return map[string]string{"credential": "凭据配置无效"}
	case errors.Is(err, domainsandbox.ErrInvalidInput), errors.Is(err, domainsandbox.ErrConfigurationInvalid):
		return map[string]string{"request": "请求参数无效"}
	default:
		return nil
	}
}

func adminSandboxErrorContract(err error) (int, string, string) {
	switch {
	case errors.Is(err, errAdminSandboxBodyTooLarge):
		return http.StatusRequestEntityTooLarge, "SANDBOX_REQUEST_TOO_LARGE", "sandbox request body is too large"
	case errors.Is(err, appsandbox.ErrPermissionDenied):
		return http.StatusForbidden, "SANDBOX_POLICY_DENIED", "sandbox operation is not permitted"
	case errors.Is(err, domainsandbox.ErrProviderNotFound):
		return http.StatusNotFound, "SANDBOX_PROVIDER_NOT_FOUND", "sandbox provider was not found"
	case errors.Is(err, domainsandbox.ErrDefaultMissing):
		return http.StatusNotFound, "SANDBOX_DEFAULT_NOT_FOUND", "sandbox default provider was not found"
	case errors.Is(err, domainsandbox.ErrVersionConflict):
		return http.StatusConflict, "SANDBOX_VERSION_CONFLICT", "sandbox provider version is stale"
	case errors.Is(err, domainsandbox.ErrProviderInUse):
		return http.StatusConflict, "SANDBOX_PROVIDER_IN_USE", "sandbox provider is in use"
	case errors.Is(err, domainsandbox.ErrProviderAlreadyExists):
		return http.StatusConflict, "SANDBOX_PROVIDER_ALREADY_EXISTS", "sandbox provider already exists"
	case errors.Is(err, domainsandbox.ErrProviderDisabled):
		return http.StatusConflict, "SANDBOX_PROVIDER_DISABLED", "sandbox provider is disabled"
	case errors.Is(err, domainsandbox.ErrProviderUnhealthy):
		return http.StatusConflict, "SANDBOX_PROVIDER_UNHEALTHY", "sandbox provider is unhealthy"
	case errors.Is(err, domainsandbox.ErrScopeUnsupported):
		return http.StatusBadRequest, "SANDBOX_SCOPE_UNSUPPORTED", "sandbox scope is unsupported"
	case errors.Is(err, domainsandbox.ErrLocalDebugUnavailable):
		return http.StatusConflict, domainsandbox.ErrCodeLocalDebugUnavailable, "local debug sandbox is unavailable"
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "SANDBOX_TIMEOUT", "sandbox operation timed out"
	case errors.Is(err, domainsandbox.ErrInvalidInput),
		errors.Is(err, domainsandbox.ErrConfigurationInvalid),
		errors.Is(err, domainsandbox.ErrCredentialInvalid):
		return http.StatusBadRequest, "SANDBOX_CONFIGURATION_INVALID", "sandbox request is invalid"
	case errors.Is(err, domainsandbox.ErrUnavailable), errors.Is(err, context.Canceled):
		return http.StatusServiceUnavailable, "SANDBOX_UNAVAILABLE", "sandbox control plane is unavailable"
	default:
		return http.StatusInternalServerError, "SANDBOX_INTERNAL", "internal server error"
	}
}

func ListAdminSandboxes(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().list(ctx, c)
}

func ListAdminSandboxDefaults(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().defaults(ctx, c)
}

func GetAdminSandboxSummary(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().summary(ctx, c)
}

func GetAdminSandboxCapabilities(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().capabilities(ctx, c)
}

func CreateAdminSandbox(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().create(ctx, c)
}

func GetAdminSandbox(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().get(ctx, c)
}

func UpdateAdminSandbox(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().update(ctx, c)
}

func DeleteAdminSandbox(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().delete(ctx, c)
}

func EnableAdminSandbox(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().enable(ctx, c)
}

func DisableAdminSandbox(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().disable(ctx, c)
}

func UpdateAdminSandboxCredentials(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().credentials(ctx, c)
}

func SetAdminSandboxDefault(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().setDefault(ctx, c)
}

func HealthCheckAdminSandbox(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().health(ctx, c)
}

func ListAdminSandboxAuditEvents(ctx context.Context, c *app.RequestContext) {
	defaultAdminSandboxHandler().auditEvents(ctx, c)
}

func RejectAdminSandboxTrailingSlash(_ context.Context, c *app.RequestContext) {
	c.AbortWithStatusJSON(http.StatusNotFound, adminSandboxEnvelope{
		Code: http.StatusNotFound, ErrorCode: "NOT_FOUND", Msg: "not found",
	})
}
