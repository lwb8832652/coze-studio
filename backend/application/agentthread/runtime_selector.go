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
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	agentThreadRuntimeDefaultEnv = "AGENT_THREAD_RUNTIME_DEFAULT"
	agentThreadEinoADKEnabledEnv = "AGENT_THREAD_EINO_ADK_ENABLED"
)

type RuntimeMode string

const (
	RuntimeModeLegacy  RuntimeMode = "legacy"
	RuntimeModeEinoADK RuntimeMode = "eino_adk"
)

type RuntimePolicy struct {
	DefaultMode    RuntimeMode
	EinoADKEnabled bool
}

type RuntimeSelector struct {
	legacy RunExecutor
	adk    RunExecutor
	policy RuntimePolicy
	err    error
}

type RuntimeResumeSelector struct {
	legacy ResumeRunExecutor
	adk    ResumeRunExecutor
	policy RuntimePolicy
	err    error
}

func RuntimePolicyFromEnv() (RuntimePolicy, error) {
	defaultMode := RuntimeMode(strings.TrimSpace(os.Getenv(agentThreadRuntimeDefaultEnv)))
	if defaultMode == "" {
		defaultMode = RuntimeModeLegacy
	}

	enabled := false
	if rawEnabled, exists := os.LookupEnv(agentThreadEinoADKEnabledEnv); exists &&
		strings.TrimSpace(rawEnabled) != "" {
		parsed, err := strconv.ParseBool(strings.TrimSpace(rawEnabled))
		if err != nil {
			return RuntimePolicy{}, fmt.Errorf(
				"parse %s: %w",
				agentThreadEinoADKEnabledEnv,
				err,
			)
		}
		enabled = parsed
	}

	policy := RuntimePolicy{
		DefaultMode:    defaultMode,
		EinoADKEnabled: enabled,
	}
	if err := policy.validate(); err != nil {
		return RuntimePolicy{}, err
	}
	return policy, nil
}

func NewRuntimeSelector(
	legacy,
	adkExecutor RunExecutor,
	policies ...RuntimePolicy,
) *RuntimeSelector {
	policy, err := selectRuntimePolicy(policies)
	return &RuntimeSelector{
		legacy: legacy,
		adk:    adkExecutor,
		policy: policy,
		err:    err,
	}
}

func NewRuntimeResumeSelector(
	legacy,
	adkExecutor ResumeRunExecutor,
	policies ...RuntimePolicy,
) *RuntimeResumeSelector {
	policy, err := selectRuntimePolicy(policies)
	return &RuntimeResumeSelector{
		legacy: legacy,
		adk:    adkExecutor,
		policy: policy,
		err:    err,
	}
}

func (s *RuntimeSelector) Execute(ctx context.Context, run *RunSummary) (*RunExecutionResult, error) {
	if s == nil {
		return nil, fmt.Errorf("agent runtime selector is required")
	}
	if s.err != nil {
		return nil, s.err
	}

	mode, err := s.policy.runtimeModeFromRun(run)
	if err != nil {
		return nil, err
	}

	switch mode {
	case RuntimeModeLegacy:
		if s.legacy == nil {
			return nil, fmt.Errorf("legacy runtime is not configured")
		}
		return s.legacy.Execute(ctx, run)
	case RuntimeModeEinoADK:
		if s.adk == nil {
			return nil, fmt.Errorf("eino adk runtime is not configured")
		}
		return s.adk.Execute(ctx, run)
	default:
		return nil, fmt.Errorf("unsupported agent runtime: %s", mode)
	}
}

func (s *RuntimeSelector) ExecuteSubagentRetry(
	ctx context.Context,
	run *RunSummary,
) (*RunExecutionResult, error) {
	if s == nil {
		return nil, fmt.Errorf("agent runtime selector is required")
	}
	if s.err != nil {
		return nil, s.err
	}

	mode, err := s.policy.runtimeModeFromRun(run)
	if err != nil {
		return nil, err
	}

	switch mode {
	case RuntimeModeLegacy:
		if s.legacy == nil {
			return nil, fmt.Errorf("legacy runtime is not configured")
		}
		retryExecutor, ok := s.legacy.(SubagentRetryRunExecutor)
		if !ok {
			return nil, &SubagentRetryUnsupportedError{}
		}
		return retryExecutor.ExecuteSubagentRetry(ctx, run)
	case RuntimeModeEinoADK:
		if s.adk == nil {
			return nil, fmt.Errorf("eino adk runtime is not configured")
		}
		retryExecutor, ok := s.adk.(SubagentRetryRunExecutor)
		if !ok {
			return nil, &SubagentRetryUnsupportedError{}
		}
		return retryExecutor.ExecuteSubagentRetry(ctx, run)
	default:
		return nil, fmt.Errorf("unsupported agent runtime: %s", mode)
	}
}

func (s *RuntimeResumeSelector) Resume(
	ctx context.Context,
	run *RunSummary,
	input *HarnessResumeInput,
) (*RunExecutionResult, error) {
	if s == nil {
		return nil, fmt.Errorf("agent resume runtime selector is required")
	}
	if s.err != nil {
		return nil, s.err
	}

	mode := RuntimeMode("")
	if input != nil {
		mode = input.Runtime
	}
	if mode == "" {
		var err error
		mode, err = s.policy.runtimeModeFromRun(run)
		if err != nil {
			return nil, err
		}
	} else if err := s.policy.authorize(mode); err != nil {
		return nil, err
	}

	switch mode {
	case RuntimeModeLegacy:
		if s.legacy == nil {
			return nil, fmt.Errorf("legacy resume runtime is not configured")
		}
		return s.legacy.Resume(ctx, run, input)
	case RuntimeModeEinoADK:
		if s.adk == nil {
			return nil, fmt.Errorf("eino adk resume runtime is not configured")
		}
		return s.adk.Resume(ctx, run, input)
	default:
		return nil, fmt.Errorf("unsupported agent runtime: %s", mode)
	}
}

func runtimeModeFromRun(run *RunSummary) (RuntimeMode, error) {
	return RuntimePolicy{
		DefaultMode:    RuntimeModeLegacy,
		EinoADKEnabled: true,
	}.runtimeModeFromRun(run)
}

func selectRuntimePolicy(policies []RuntimePolicy) (RuntimePolicy, error) {
	policy := RuntimePolicy{
		DefaultMode:    RuntimeModeLegacy,
		EinoADKEnabled: true,
	}
	if len(policies) > 0 {
		policy = policies[0]
	}
	if policy.DefaultMode == "" {
		policy.DefaultMode = RuntimeModeLegacy
	}

	return policy, policy.validate()
}

func (p RuntimePolicy) validate() error {
	switch p.DefaultMode {
	case RuntimeModeLegacy:
		return nil
	case RuntimeModeEinoADK:
		if !p.EinoADKEnabled {
			return fmt.Errorf("default eino adk runtime is disabled by server policy")
		}
		return nil
	default:
		return fmt.Errorf("unsupported default agent runtime: %s", p.DefaultMode)
	}
}

func (p RuntimePolicy) runtimeModeFromRun(run *RunSummary) (RuntimeMode, error) {
	if run == nil {
		return "", fmt.Errorf("run is required")
	}

	rawConfig := strings.TrimSpace(run.Config)
	if rawConfig == "" {
		return p.authorizedMode(p.DefaultMode)
	}

	var config struct {
		Runtime string `json:"runtime"`
	}
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		return "", fmt.Errorf("parse run config failed: %w", err)
	}

	mode := RuntimeMode(strings.TrimSpace(config.Runtime))
	if mode == "" {
		return p.authorizedMode(p.DefaultMode)
	}
	return p.authorizedMode(mode)
}

func (p RuntimePolicy) authorizedMode(mode RuntimeMode) (RuntimeMode, error) {
	if err := p.authorize(mode); err != nil {
		return "", err
	}
	return mode, nil
}

func (p RuntimePolicy) authorize(mode RuntimeMode) error {
	switch mode {
	case RuntimeModeLegacy:
		return nil
	case RuntimeModeEinoADK:
		if !p.EinoADKEnabled {
			return fmt.Errorf("eino adk runtime is disabled by server policy")
		}
		return nil
	default:
		return fmt.Errorf("unsupported agent runtime: %s", mode)
	}
}
