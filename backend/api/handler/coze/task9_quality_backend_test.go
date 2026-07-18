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

package coze

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/stretchr/testify/require"

	appsandbox "github.com/coze-dev/coze-studio/backend/application/sandbox"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestTask9QualityAdminFieldErrorsAreSafeAndBounded(t *testing.T) {
	stub := &adminSandboxServiceStub{err: fmt.Errorf("endpoint=https://secret.example /etc token=raw: %w", domainsandbox.ErrInvalidInput)}
	h := newAdminSandboxTestServer(stub)
	response := performAdminSandboxRequest(h, http.MethodGet, "/api/admin/sandboxes/17", "")
	require.Equal(t, http.StatusBadRequest, response.Code)
	body := string(response.Result().Body())
	require.Contains(t, body, `"field_errors":{"request":"请求参数无效"}`)
	for _, prohibited := range []string{"secret.example", "/etc", "token=raw", "endpoint="} {
		require.NotContains(t, body, prohibited)
	}

	stub.err = fmt.Errorf("raw endpoint and token: %w", domainsandbox.ErrLocalDebugUnavailable)
	response = performAdminSandboxRequest(h, http.MethodGet, "/api/admin/sandboxes/17", "")
	require.Equal(t, http.StatusConflict, response.Code)
	body = string(response.Result().Body())
	require.Contains(t, body, `"error_code":"SANDBOX_LOCAL_DEBUG_UNAVAILABLE"`)
	require.Contains(t, body, `"field_errors":{"type":"当前环境不支持本地调试 Sandbox"}`)
	require.NotContains(t, body, "raw endpoint")
	require.NotContains(t, body, "token")

	stub.err = errors.New("raw provider error endpoint=https://secret.example token=raw")
	response = performAdminSandboxRequest(h, http.MethodGet, "/api/admin/sandboxes/17", "")
	require.Equal(t, http.StatusInternalServerError, response.Code)
	body = string(response.Result().Body())
	require.Contains(t, body, `"error_code":"SANDBOX_INTERNAL"`)
	require.NotContains(t, body, "field_errors")
	require.NotContains(t, body, "secret.example")
	require.NotContains(t, body, "token=raw")
}

func TestAdminSandboxFieldErrorWhitelistMatchesRealSafeBackendErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want map[string]string
	}{
		{name: "request body", err: errAdminSandboxBodyTooLarge, want: map[string]string{"request": "请求体过大"}},
		{name: "invalid request", err: domainsandbox.ErrInvalidInput, want: map[string]string{"request": "请求参数无效"}},
		{name: "invalid configuration", err: domainsandbox.ErrConfigurationInvalid, want: map[string]string{"request": "请求参数无效"}},
		{name: "credential", err: domainsandbox.ErrCredentialInvalid, want: map[string]string{"credential": "凭据配置无效"}},
		{name: "scopes", err: domainsandbox.ErrScopeUnsupported, want: map[string]string{"scopes": "所选作用域不受支持"}},
		{name: "type", err: domainsandbox.ErrLocalDebugUnavailable, want: map[string]string{"type": "当前环境不支持本地调试 Sandbox"}},
		{name: "unknown raw error", err: errors.New("endpoint=https://secret.example token=raw"), want: nil},
	}
	allowed := map[string]struct{}{"request": {}, "credential": {}, "scopes": {}, "type": {}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := adminSandboxFieldErrors(test.err)
			require.Equal(t, test.want, actual)
			for key := range actual {
				_, ok := allowed[key]
				require.Truef(t, ok, "unexpected public field_errors key %q", key)
				require.NotEqual(t, "name", key)
				require.False(t, strings.HasPrefix(key, "policy."))
			}
		})
	}
}

func TestAdminSandboxCreateRequestIDContractUsesRealServiceTransactionAndAudit(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("APP_ENV", "debug")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")

	repository := &task9QualityMutationRepository{}
	audits := &task9QualityAuditRepository{}
	uow := &task9QualityUnitOfWork{repositories: domainsandbox.TransactionRepositories{
		Providers: repository, Defaults: &task9QualityDefaultRepository{}, Audits: audits,
	}}
	service, err := appsandbox.NewService(appsandbox.ServiceOptions{
		Providers: repository, UnitOfWork: uow, Codec: &task9QualityCodec{},
		Leases: &task9QualityLeaseGuard{}, Factory: &task9QualityHealthFactory{},
		ProviderKey: func(appsandbox.Actor) (string, error) {
			return "018f0d2e-7b73-7e21-9a89-1a2b3c4d5e70", nil
		},
		Now: func() time.Time { return time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC) },
	})
	require.NoError(t, err)
	h := newAdminSandboxTestServer(task9QualityRealService{Service: service})
	body := fmt.Sprintf(`{"name":"Local","type":"local_debug","scopes":["agent"],"policy":%s}`, adminSandboxTestPolicyJSON(t))

	missing := performAdminSandboxRequest(h, http.MethodPost, "/api/admin/sandboxes", body)
	require.Equal(t, http.StatusBadRequest, missing.Code, string(missing.Result().Body()))
	require.Contains(t, string(missing.Result().Body()), `"error_code":"SANDBOX_CONFIGURATION_INVALID"`)
	require.Zero(t, repository.createCalls)
	require.Zero(t, uow.transactionCalls)
	require.Zero(t, uow.providerCreateCalls)
	require.Zero(t, audits.appendCalls)

	requestBody := &ut.Body{Body: bytes.NewBufferString(body), Len: len(body)}
	withID := ut.PerformRequest(
		h.Engine, http.MethodPost, "/api/admin/sandboxes", requestBody,
		ut.Header{Key: "content-type", Value: "application/json"},
		ut.Header{Key: "X-Request-ID", Value: "task9-request-1"},
	)
	require.Equal(t, http.StatusOK, withID.Code, string(withID.Result().Body()))
	require.Equal(t, 1, repository.createCalls)
	require.Zero(t, uow.transactionCalls)
	require.Equal(t, 1, uow.providerCreateCalls)
	require.Equal(t, 1, audits.appendCalls)
	require.Equal(t, "task9-request-1", audits.last.RequestID)
}

type task9QualityMutationRepository struct {
	domainsandbox.ProviderRepository
	createCalls int
}

func (r *task9QualityMutationRepository) CreateProvider(_ context.Context, input domainsandbox.CreateProviderInput) (*domainsandbox.Provider, error) {
	r.createCalls++
	provider, err := domainsandbox.NewProvider(input)
	if err != nil {
		return nil, err
	}
	provider.ID = 17
	provider.Version = domainsandbox.InitialVersion
	provider.CreatedAt = time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	provider.UpdatedAt = provider.CreatedAt
	return provider, nil
}

type task9QualityDefaultRepository struct {
	domainsandbox.ProviderDefaultRepository
}

type task9QualityAuditRepository struct {
	domainsandbox.ProviderAuditRepository
	last        domainsandbox.AppendProviderAuditEventInput
	appendCalls int
}

func (r *task9QualityAuditRepository) AppendProviderAuditEvent(_ context.Context, input domainsandbox.AppendProviderAuditEventInput) (*domainsandbox.ProviderAuditEvent, error) {
	r.appendCalls++
	r.last = input
	return &domainsandbox.ProviderAuditEvent{
		ID: 1, ProviderID: input.ProviderID, ActorUserID: input.ActorUserID,
		Action: input.Action, Result: input.Result, RequestID: input.RequestID,
		Metadata: input.Metadata, CreatedAt: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
	}, nil
}

type task9QualityUnitOfWork struct {
	repositories        domainsandbox.TransactionRepositories
	transactionCalls    int
	providerCreateCalls int
}

func (u *task9QualityUnitOfWork) WithinTransaction(ctx context.Context, callback func(context.Context, domainsandbox.TransactionRepositories) error) error {
	u.transactionCalls++
	return callback(ctx, u.repositories)
}

func (u *task9QualityUnitOfWork) WithinProviderCreateTransaction(ctx context.Context, callback func(context.Context, domainsandbox.TransactionRepositories) error) error {
	u.providerCreateCalls++
	return callback(ctx, u.repositories)
}

type task9QualityCodec struct{}

func (*task9QualityCodec) Encrypt(string, infrasandbox.CredentialField, []byte) (string, error) {
	return "encrypted", nil
}

func (*task9QualityCodec) FingerprintCredential([]byte) (string, error) {
	return strings.Repeat("a", 32), nil
}

func (*task9QualityCodec) Inspect(string) (infrasandbox.CredentialEnvelopeMetadata, error) {
	return infrasandbox.CredentialEnvelopeMetadata{Active: true}, nil
}

type task9QualityLeaseGuard struct {
	appsandbox.ProviderLifecycleGuard
}

type task9QualityHealthFactory struct {
	appsandbox.HealthProviderFactory
}

type task9QualityRealService struct {
	*appsandbox.Service
}

func (s task9QualityRealService) ReplaceCredentials(
	ctx context.Context,
	actor appsandbox.Actor,
	providerID int64,
	expectedVersion uint64,
	mutation appsandbox.SecretMutation,
) (*appsandbox.ProviderDTO, error) {
	provider, err := s.Get(ctx, actor, providerID)
	if err != nil {
		return nil, err
	}
	return s.Update(ctx, actor, appsandbox.UpdateProviderRequest{
		ProviderID: providerID, ExpectedVersion: expectedVersion,
		Name: provider.Name, Scopes: provider.Scopes, Policy: provider.Policy,
		Credential: mutation,
	})
}
