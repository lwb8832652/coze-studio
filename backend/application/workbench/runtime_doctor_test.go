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

package workbench

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"

	diagnosticapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/diagnostic"
	skillapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/skill"
	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/internal/testutil"
	"github.com/coze-dev/coze-studio/backend/types/consts"
)

func TestRuntimeDoctorModelCheckReportsConfiguredDefaultModelWithoutCallingIt(t *testing.T) {
	ctx := context.Background()
	var requestedModelType int64 = -1
	chatModel := &testutil.UTChatModel{}

	check := runtimeDoctorModelCheck(ctx, func(_ context.Context, modelType int64) (model.BaseChatModel, bool, error) {
		requestedModelType = modelType
		return chatModel, true, nil
	})

	require.Equal(t, int64(0), requestedModelType)
	require.Equal(t, "model.default", check.Name)
	require.Equal(t, "model", check.Category)
	require.Equal(t, runtimeDoctorStatusReady, check.Status)
	require.Contains(t, check.Message, "configured")
	require.Zero(t, chatModel.Index)
}

func TestRuntimeDoctorModelCheckReportsProviderErrorsSafely(t *testing.T) {
	ctx := context.Background()

	check := runtimeDoctorModelCheck(ctx, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return nil, false, errors.New("provider failed with api_key=sk-test-secret")
	})

	require.Equal(t, "model.default", check.Name)
	require.Equal(t, "model", check.Category)
	require.Equal(t, runtimeDoctorStatusError, check.Status)
	require.Contains(t, check.Message, "provider failed")
	require.NotContains(t, check.Message, "sk-test-secret")
	require.NotContains(t, check.Message, "api_key")
}

func TestRuntimeDoctorModelCheckConvertsProviderPanicToDiagnosticError(t *testing.T) {
	ctx := context.Background()

	check := runtimeDoctorModelCheck(ctx, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		panic("provider panic password=hunter2")
	})

	require.Equal(t, "model.default", check.Name)
	require.Equal(t, "model", check.Category)
	require.Equal(t, runtimeDoctorStatusError, check.Status)
	require.Contains(t, check.Message, "provider panic")
	require.NotContains(t, check.Message, "hunter2")
	require.NotContains(t, check.Message, "password")
}

func TestRuntimeDoctorModelDiagnosticsReportsCapabilitiesAndOptionalLiveProbe(t *testing.T) {
	t.Setenv("WORKBENCH_RUNTIME_DOCTOR_LIVE_MODEL_PROBE", "true")
	ctx := context.Background()
	chatModel := &fakeRuntimeDoctorCapabilityModel{
		UTChatModel: &testutil.UTChatModel{
			InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
				require.Len(t, in, 1)
				require.NotContains(t, in[0].Content, "user prompt")
				return schema.AssistantMessage("provider replied with hidden diagnostic text", nil), nil
			},
		},
		capabilities: appagentthread.ADKModelCapabilities{
			NativeToolSearch: true,
			Thinking:         true,
			Reasoning:        true,
			Vision:           true,
		},
	}

	data, checks := runtimeDoctorModelDiagnostics(ctx, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return chatModel, true, nil
	})

	require.Equal(t, runtimeDoctorStatusReady, data.Status)
	require.True(t, data.Configured)
	require.Equal(t, runtimeDoctorStatusReady, data.LiveProbe)
	require.NotNil(t, data.Capabilities)
	require.True(t, data.Capabilities.NativeToolSearch)
	require.True(t, data.Capabilities.Thinking)
	require.True(t, data.Capabilities.Reasoning)
	require.True(t, data.Capabilities.Vision)
	require.False(t, data.Capabilities.Audio)
	require.Equal(t, 1, chatModel.Index)

	checkByName := runtimeDoctorChecksByName(checks)
	require.Equal(t, runtimeDoctorStatusReady, checkByName["model.default"].Status)
	require.Equal(t, runtimeDoctorStatusReady, checkByName["model.capabilities"].Status)
	require.Equal(t, runtimeDoctorStatusReady, checkByName["model.live_connectivity"].Status)
	require.NotContains(t, strings.Join(runtimeDoctorCheckMessages(checks), "\n"), "provider replied")
	require.NotContains(t, strings.Join(runtimeDoctorCheckMessages(checks), "\n"), "user prompt")
}

func TestRuntimeDoctorModelDiagnosticsRedactsLiveProbeErrors(t *testing.T) {
	t.Setenv("WORKBENCH_RUNTIME_DOCTOR_LIVE_MODEL_PROBE", "true")
	ctx := context.Background()
	chatModel := &testutil.UTChatModel{
		InvokeResultProvider: func(_ int, _ []*schema.Message) (*schema.Message, error) {
			return nil, errors.New("upstream failed authorization=Bearer sk-live-secret")
		},
	}

	data, checks := runtimeDoctorModelDiagnostics(ctx, func(context.Context, int64) (model.BaseChatModel, bool, error) {
		return chatModel, true, nil
	})

	require.Equal(t, runtimeDoctorStatusError, data.LiveProbe)
	checkByName := runtimeDoctorChecksByName(checks)
	require.Equal(t, runtimeDoctorStatusError, checkByName["model.live_connectivity"].Status)
	message := checkByName["model.live_connectivity"].Message
	require.Contains(t, message, "upstream failed")
	require.NotContains(t, message, "authorization")
	require.NotContains(t, message, "Bearer")
	require.NotContains(t, message, "sk-live-secret")
}

func TestRuntimeDoctorSandboxDataReportsBoundedPolicy(t *testing.T) {
	t.Setenv(consts.CodeRunnerType, "sandbox")
	t.Setenv(consts.CodeRunnerAllowNet, "api.example.test,localhost:3000")
	t.Setenv(consts.CodeRunnerAllowRun, "")
	t.Setenv(consts.CodeRunnerAllowFFI, "")
	t.Setenv(consts.CodeRunnerNodeModulesDir, "/private/node_modules")

	data, check := runtimeDoctorSandboxData()

	require.Equal(t, runtimeDoctorStatusReady, data.Status)
	require.Equal(t, "sandbox", data.RunnerType)
	require.Equal(t, "configured", data.Network)
	require.Equal(t, "restricted", data.Process)
	require.Equal(t, "restricted", data.FFI)
	require.Equal(t, "configured", data.NodeModules)
	require.Equal(t, "sandbox.runner_policy", check.Name)
	require.Equal(t, "sandbox", check.Category)
	require.Equal(t, runtimeDoctorStatusReady, check.Status)
	require.NotContains(t, check.Message, "api.example.test")
	require.NotContains(t, check.Message, "/private/node_modules")
}

func TestRuntimeDoctorSandboxDataWarnsForLocalRunner(t *testing.T) {
	t.Setenv(consts.CodeRunnerType, "local")

	data, check := runtimeDoctorSandboxData()

	require.Equal(t, runtimeDoctorStatusWarning, data.Status)
	require.Equal(t, "local", data.RunnerType)
	require.Equal(t, runtimeDoctorStatusWarning, check.Status)
	require.Contains(t, check.Message, "local code runner")
}

func TestRuntimeDoctorSandboxProviderDataReportsHealthyAgentProviderWithoutSecrets(t *testing.T) {
	now := time.Date(2026, time.July, 18, 10, 0, 0, 0, time.UTC)
	repository := &fakeRuntimeDoctorSandboxRepository{
		defaultProvider: &domainsandbox.ProviderDefault{
			Scope:      domainsandbox.ScopeAgent,
			ProviderID: 42,
		},
		provider: &domainsandbox.Provider{
			ID:               42,
			ProviderKey:      "provider-key-must-not-leak",
			Type:             domainsandbox.ProviderType("remote"),
			Status:           domainsandbox.ProviderStatusEnabled,
			EndpointSecret:   "https://secret-provider.example.test",
			CredentialSecret: "encrypted-secret-must-not-leak",
			Health: domainsandbox.HealthSnapshot{
				Status:        domainsandbox.HealthStatusHealthy,
				CheckedAt:     now.Add(-time.Minute),
				LatencyMillis: 25,
				Capabilities:  []domainsandbox.Scope{domainsandbox.ScopeAgent},
			},
		},
	}

	data, check := runtimeDoctorSandboxProviderData(
		context.Background(),
		repository,
		true,
		true,
		now,
	)

	require.Equal(t, runtimeDoctorStatusReady, data.Status)
	require.Equal(t, "control_plane", data.RunnerType)
	require.Len(t, data.Scopes, 1)
	require.Equal(t, "agent", data.Scopes[0].Scope)
	require.True(t, data.Scopes[0].Configured)
	require.True(t, data.Scopes[0].Available)
	require.True(t, data.Scopes[0].Selected)
	require.Equal(t, "healthy", data.Scopes[0].HealthStatus)
	require.Equal(t, "ready", data.Scopes[0].ReasonCode)
	require.Equal(t, "remote", data.Scopes[0].ProviderType)
	require.Contains(t, data.Scopes[0].ProviderRef, "sha256:")
	require.Equal(t, runtimeDoctorStatusReady, check.Status)

	payload, err := json.Marshal(data)
	require.NoError(t, err)
	require.NotContains(t, string(payload), "provider-key-must-not-leak")
	require.NotContains(t, string(payload), "secret-provider.example.test")
	require.NotContains(t, string(payload), "encrypted-secret-must-not-leak")
}

func TestRuntimeDoctorSandboxProviderDataReturnsActionableMissingDefault(t *testing.T) {
	data, check := runtimeDoctorSandboxProviderData(
		context.Background(),
		&fakeRuntimeDoctorSandboxRepository{defaultErr: domainsandbox.ErrDefaultMissing},
		true,
		true,
		time.Now().UTC(),
	)

	require.Equal(t, runtimeDoctorStatusWarning, data.Status)
	require.Len(t, data.Scopes, 1)
	require.False(t, data.Scopes[0].Configured)
	require.False(t, data.Scopes[0].Available)
	require.Equal(t, "default_missing", data.Scopes[0].ReasonCode)
	require.Contains(t, check.Message, "contact the system administrator")
}

func TestRuntimeDoctorSandboxProviderDataReportsDisabledControlPlaneWithoutRepositoryReads(t *testing.T) {
	repository := &fakeRuntimeDoctorSandboxRepository{}

	data, check := runtimeDoctorSandboxProviderData(
		context.Background(),
		repository,
		false,
		false,
		time.Now().UTC(),
	)

	require.Equal(t, runtimeDoctorStatusDisabled, data.Status)
	require.Equal(t, "disabled", data.RunnerType)
	require.Equal(t, "control_plane_disabled", data.Scopes[0].ReasonCode)
	require.Zero(t, repository.defaultCalls)
	require.Zero(t, repository.providerCalls)
	require.Equal(t, runtimeDoctorStatusDisabled, check.Status)
}

func TestRuntimeDoctorSandboxProviderDataShowsConfiguredProviderWhileRoutingDisabled(t *testing.T) {
	now := time.Now().UTC()
	repository := &fakeRuntimeDoctorSandboxRepository{
		defaultProvider: &domainsandbox.ProviderDefault{
			Scope: domainsandbox.ScopeAgent, ProviderID: 42,
		},
		provider: &domainsandbox.Provider{
			ID: 42, ProviderKey: "provider-key", Type: domainsandbox.ProviderTypeRemoteHTTP,
			Status: domainsandbox.ProviderStatusEnabled,
			Health: domainsandbox.HealthSnapshot{
				Status: domainsandbox.HealthStatusHealthy, CheckedAt: now,
				LatencyMillis: 1, Capabilities: []domainsandbox.Scope{domainsandbox.ScopeAgent},
			},
		},
	}

	data, check := runtimeDoctorSandboxProviderData(
		context.Background(), repository, true, false, now,
	)

	require.Equal(t, runtimeDoctorStatusDisabled, data.Status)
	require.True(t, data.Scopes[0].Configured)
	require.False(t, data.Scopes[0].Available)
	require.Equal(t, "runtime_routing_disabled", data.Scopes[0].ReasonCode)
	require.Equal(t, runtimeDoctorStatusDisabled, check.Status)
}

func TestRuntimeDoctorSkillCheckSummarizesCountsAndSafeNames(t *testing.T) {
	ctx := context.Background()
	service := &fakeRuntimeDoctorSkillService{
		resp: &skillapi.ListSkillsResponse{
			Data: &skillapi.ListSkillsData{
				Skills: []*skillapi.Skill{
					{
						Name:        "research",
						Enabled:     true,
						Permissions: `{"token":"secret"}`,
					},
					{
						Name:         "writer\n<script>",
						Enabled:      true,
						InputSchema:  `{"prompt":"hidden"}`,
						OutputSchema: `{"result":"hidden"}`,
					},
					{
						Name:    "api_key=skill-name-secret",
						Enabled: true,
					},
					{
						Name:    "disabled",
						Enabled: false,
					},
				},
			},
		},
	}

	check := runtimeDoctorSkillCheck(ctx, service, 100)

	require.Equal(t, int64(100), service.req.SpaceID)
	require.Equal(t, "skills.runtime_catalog", check.Name)
	require.Equal(t, "skills", check.Category)
	require.Equal(t, runtimeDoctorStatusReady, check.Status)
	require.Contains(t, check.Message, "3 enabled / 4 total skills")
	require.Contains(t, check.Message, "research")
	require.Contains(t, check.Message, "writer script")
	require.NotContains(t, check.Message, "api_key")
	require.NotContains(t, check.Message, "secret")
	require.NotContains(t, check.Message, "hidden")
}

func TestRuntimeDoctorSkillCheckHandlesListFailureAsDiagnosticCheck(t *testing.T) {
	ctx := context.Background()
	service := &fakeRuntimeDoctorSkillService{
		err: errors.New("skill store failed password=hunter2"),
	}

	check := runtimeDoctorSkillCheck(ctx, service, 100)

	require.Equal(t, "skills.runtime_catalog", check.Name)
	require.Equal(t, "skills", check.Category)
	require.Equal(t, runtimeDoctorStatusError, check.Status)
	require.Contains(t, check.Message, "skill store failed")
	require.NotContains(t, check.Message, "hunter2")
	require.NotContains(t, check.Message, "password")
}

func TestRuntimeDoctorSkillCheckTreatsUninitializedServiceAsDisabled(t *testing.T) {
	ctx := context.Background()
	service := &fakeRuntimeDoctorSkillService{
		err: errors.New("skill service is not initialized"),
	}

	check := runtimeDoctorSkillCheck(ctx, service, 100)

	require.Equal(t, "skills.runtime_catalog", check.Name)
	require.Equal(t, "skills", check.Category)
	require.Equal(t, runtimeDoctorStatusDisabled, check.Status)
	require.Contains(t, check.Message, "not configured")
}

type fakeRuntimeDoctorSkillService struct {
	resp *skillapi.ListSkillsResponse
	err  error
	req  *skillapi.ListSkillsRequest
}

func (f *fakeRuntimeDoctorSkillService) ListSkills(
	_ context.Context,
	req *skillapi.ListSkillsRequest,
) (*skillapi.ListSkillsResponse, error) {
	f.req = req
	return f.resp, f.err
}

type fakeRuntimeDoctorSandboxRepository struct {
	defaultProvider *domainsandbox.ProviderDefault
	defaultErr      error
	provider        *domainsandbox.Provider
	providerErr     error
	defaultCalls    int
	providerCalls   int
}

func (f *fakeRuntimeDoctorSandboxRepository) GetProviderDefault(
	_ context.Context,
	_ domainsandbox.Scope,
) (*domainsandbox.ProviderDefault, error) {
	f.defaultCalls++
	return f.defaultProvider, f.defaultErr
}

func (f *fakeRuntimeDoctorSandboxRepository) GetProvider(
	_ context.Context,
	_ int64,
) (*domainsandbox.Provider, error) {
	f.providerCalls++
	return f.provider, f.providerErr
}

type fakeRuntimeDoctorCapabilityModel struct {
	*testutil.UTChatModel
	capabilities appagentthread.ADKModelCapabilities
}

func (f *fakeRuntimeDoctorCapabilityModel) ADKProviderCapabilities() appagentthread.ADKModelCapabilities {
	return f.capabilities
}

func runtimeDoctorChecksByName(
	checks []*diagnosticapi.RuntimeDoctorCheck,
) map[string]*diagnosticapi.RuntimeDoctorCheck {
	result := make(map[string]*diagnosticapi.RuntimeDoctorCheck, len(checks))
	for _, check := range checks {
		if check != nil {
			result[check.Name] = check
		}
	}
	return result
}

func runtimeDoctorCheckMessages(checks []*diagnosticapi.RuntimeDoctorCheck) []string {
	result := make([]string, 0, len(checks))
	for _, check := range checks {
		if check != nil {
			result = append(result, check.Message)
		}
	}
	return result
}
