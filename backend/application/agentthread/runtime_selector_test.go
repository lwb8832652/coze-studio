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

package agentthread

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuntimeModeFromRunDefaultsToLegacy(t *testing.T) {
	mode, err := runtimeModeFromRun(&RunSummary{})

	require.NoError(t, err)
	require.Equal(t, RuntimeModeLegacy, mode)
}

func TestRuntimeModeFromRunSelectsEinoADK(t *testing.T) {
	mode, err := runtimeModeFromRun(&RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	})

	require.NoError(t, err)
	require.Equal(t, RuntimeModeEinoADK, mode)
}

func TestRuntimeModeFromRunRejectsUnknownRuntime(t *testing.T) {
	_, err := runtimeModeFromRun(&RunSummary{
		Config: `{"runtime":"unknown"}`,
	})

	require.ErrorContains(t, err, "unsupported agent runtime: unknown")
}

func TestRuntimePolicyFromEnvDefaultsToLegacyWithADKDisabled(t *testing.T) {
	t.Setenv(agentThreadRuntimeDefaultEnv, "")
	t.Setenv(agentThreadEinoADKEnabledEnv, "")

	policy, err := RuntimePolicyFromEnv()

	require.NoError(t, err)
	require.Equal(t, RuntimeModeLegacy, policy.DefaultMode)
	require.False(t, policy.EinoADKEnabled)
}

func TestRuntimePolicyFromEnvRejectsUnknownDefault(t *testing.T) {
	t.Setenv(agentThreadRuntimeDefaultEnv, "unknown")

	_, err := RuntimePolicyFromEnv()

	require.ErrorContains(t, err, "unsupported default agent runtime: unknown")
}

func TestRuntimePolicyFromEnvRejectsDisabledADKDefault(t *testing.T) {
	t.Setenv(agentThreadRuntimeDefaultEnv, string(RuntimeModeEinoADK))
	t.Setenv(agentThreadEinoADKEnabledEnv, "false")

	_, err := RuntimePolicyFromEnv()

	require.ErrorContains(t, err, "default eino adk runtime is disabled")
}

func TestRuntimePolicyFromEnvRejectsInvalidADKFlag(t *testing.T) {
	t.Setenv(agentThreadRuntimeDefaultEnv, string(RuntimeModeLegacy))
	t.Setenv(agentThreadEinoADKEnabledEnv, "sometimes")

	_, err := RuntimePolicyFromEnv()

	require.ErrorContains(t, err, "parse AGENT_THREAD_EINO_ADK_ENABLED")
}

func TestRuntimeSelectorRoutesToConfiguredExecutor(t *testing.T) {
	legacyCalls := 0
	adkCalls := 0
	selector := NewRuntimeSelector(
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			legacyCalls++
			return &RunExecutionResult{Message: "legacy"}, nil
		}),
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			adkCalls++
			return &RunExecutionResult{Message: "adk"}, nil
		}),
	)

	result, err := selector.Execute(context.Background(), &RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	})

	require.NoError(t, err)
	require.Equal(t, "adk", result.Message)
	require.Zero(t, legacyCalls)
	require.Equal(t, 1, adkCalls)
}

func TestRuntimeSelectorRoutesSubagentRetryToConfiguredADKExecutor(t *testing.T) {
	legacy := &recordingSubagentRetryRunExecutor{
		retryResult: &RunExecutionResult{Message: "legacy-retry"},
	}
	adkExecutor := &recordingSubagentRetryRunExecutor{
		retryResult: &RunExecutionResult{Message: "adk-retry"},
	}
	selector := NewRuntimeSelector(
		legacy,
		adkExecutor,
		RuntimePolicy{
			DefaultMode:    RuntimeModeLegacy,
			EinoADKEnabled: true,
		},
	)

	result, err := selector.ExecuteSubagentRetry(context.Background(), &RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	})

	require.NoError(t, err)
	require.Equal(t, "adk-retry", result.Message)
	require.False(t, legacy.retryExecuteCalled)
	require.True(t, adkExecutor.retryExecuteCalled)
}

func TestRuntimeSelectorUsesPolicyDefault(t *testing.T) {
	selector := NewRuntimeSelector(
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "legacy"}, nil
		}),
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "adk"}, nil
		}),
		RuntimePolicy{
			DefaultMode:    RuntimeModeEinoADK,
			EinoADKEnabled: true,
		},
	)

	result, err := selector.Execute(context.Background(), &RunSummary{})

	require.NoError(t, err)
	require.Equal(t, "adk", result.Message)
}

func TestRuntimeSelectorRejectsADKWhenServerPolicyDisablesIt(t *testing.T) {
	adkCalls := 0
	selector := NewRuntimeSelector(
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "legacy"}, nil
		}),
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			adkCalls++
			return &RunExecutionResult{Message: "adk"}, nil
		}),
		RuntimePolicy{
			DefaultMode:    RuntimeModeLegacy,
			EinoADKEnabled: false,
		},
	)

	_, err := selector.Execute(context.Background(), &RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	})

	require.ErrorContains(t, err, "eino adk runtime is disabled by server policy")
	require.Zero(t, adkCalls)
}

func TestRuntimeSelectorRequiresSelectedExecutor(t *testing.T) {
	selector := NewRuntimeSelector(
		RunExecutorFunc(func(context.Context, *RunSummary) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "legacy"}, nil
		}),
		nil,
	)

	_, err := selector.Execute(context.Background(), &RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	})

	require.ErrorContains(t, err, "eino adk runtime is not configured")
}

func TestRuntimeResumeSelectorRoutesToConfiguredExecutor(t *testing.T) {
	legacyCalls := 0
	adkCalls := 0
	selector := NewRuntimeResumeSelector(
		ResumeRunExecutorFunc(func(context.Context, *RunSummary, *HarnessResumeInput) (*RunExecutionResult, error) {
			legacyCalls++
			return &RunExecutionResult{Message: "legacy"}, nil
		}),
		ResumeRunExecutorFunc(func(context.Context, *RunSummary, *HarnessResumeInput) (*RunExecutionResult, error) {
			adkCalls++
			return &RunExecutionResult{Message: "adk"}, nil
		}),
	)

	result, err := selector.Resume(context.Background(), &RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	}, &HarnessResumeInput{})

	require.NoError(t, err)
	require.Equal(t, "adk", result.Message)
	require.Zero(t, legacyCalls)
	require.Equal(t, 1, adkCalls)
}

func TestRuntimeResumeSelectorRejectsADKCheckpointWhenDisabled(t *testing.T) {
	adkCalls := 0
	selector := NewRuntimeResumeSelector(
		ResumeRunExecutorFunc(func(context.Context, *RunSummary, *HarnessResumeInput) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "legacy"}, nil
		}),
		ResumeRunExecutorFunc(func(context.Context, *RunSummary, *HarnessResumeInput) (*RunExecutionResult, error) {
			adkCalls++
			return &RunExecutionResult{Message: "adk"}, nil
		}),
		RuntimePolicy{
			DefaultMode:    RuntimeModeLegacy,
			EinoADKEnabled: false,
		},
	)

	_, err := selector.Resume(
		context.Background(),
		&RunSummary{},
		&HarnessResumeInput{Runtime: RuntimeModeEinoADK},
	)

	require.ErrorContains(t, err, "eino adk runtime is disabled by server policy")
	require.Zero(t, adkCalls)
}

func TestRuntimeResumeSelectorUsesCheckpointRuntimeBeforeRunConfig(t *testing.T) {
	legacyCalls := 0
	adkCalls := 0
	selector := NewRuntimeResumeSelector(
		ResumeRunExecutorFunc(func(context.Context, *RunSummary, *HarnessResumeInput) (*RunExecutionResult, error) {
			legacyCalls++
			return &RunExecutionResult{Message: "legacy"}, nil
		}),
		ResumeRunExecutorFunc(func(context.Context, *RunSummary, *HarnessResumeInput) (*RunExecutionResult, error) {
			adkCalls++
			return &RunExecutionResult{Message: "adk"}, nil
		}),
	)

	result, err := selector.Resume(
		context.Background(),
		&RunSummary{Config: `{"runtime":"legacy"}`},
		&HarnessResumeInput{Runtime: RuntimeModeEinoADK},
	)

	require.NoError(t, err)
	require.Equal(t, "adk", result.Message)
	require.Zero(t, legacyCalls)
	require.Equal(t, 1, adkCalls)
}

func TestRuntimeResumeSelectorRequiresSelectedExecutor(t *testing.T) {
	selector := NewRuntimeResumeSelector(
		ResumeRunExecutorFunc(func(context.Context, *RunSummary, *HarnessResumeInput) (*RunExecutionResult, error) {
			return &RunExecutionResult{Message: "legacy"}, nil
		}),
		nil,
	)

	_, err := selector.Resume(context.Background(), &RunSummary{
		Config: `{"runtime":"eino_adk"}`,
	}, &HarnessResumeInput{})

	require.ErrorContains(t, err, "eino adk resume runtime is not configured")
}
