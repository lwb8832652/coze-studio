// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package impl

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

func TestCodeRunnerListConfigUsesLiteralValuesUnlessEnvPrefixIsExplicit(t *testing.T) {
	environment := map[string]string{
		"ALLOW_ENV":             "SECRET_SHOULD_NOT_REPLACE_LITERAL",
		"CODE_RUNNER_ALLOW_ENV": " PATH,HOME,PATH,, ",
	}
	lookup := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}

	literal, err := parseListConfig(" ALLOW_ENV,HOME,ALLOW_ENV, ", lookup)
	if err != nil {
		t.Fatalf("parseListConfig(literal) error = %v", err)
	}
	if got := strings.Join(literal, ","); got != "ALLOW_ENV,HOME" {
		t.Fatalf("literal list = %q", got)
	}

	explicit, err := parseListConfig(" env:CODE_RUNNER_ALLOW_ENV ", lookup)
	if err != nil {
		t.Fatalf("parseListConfig(env) error = %v", err)
	}
	if got := strings.Join(explicit, ","); got != "HOME,PATH" {
		t.Fatalf("explicit env list = %q", got)
	}

	implicit, err := parseListConfig("${ALLOW_ENV},$ALLOW_ENV", lookup)
	if err != nil {
		t.Fatalf("parseListConfig(implicit literal) error = %v", err)
	}
	if got := strings.Join(implicit, ","); got != "$ALLOW_ENV,${ALLOW_ENV}" {
		t.Fatalf("implicit syntax was not literal: %q", got)
	}
}

func TestCodeRunnerConfigRejectsInvalidMissingOrEmptyExplicitEnvReferences(t *testing.T) {
	secret := strings.Repeat("secret-value-must-not-leak", 500)
	environment := map[string]string{"EMPTY": "", "OVERSIZED": secret}
	lookup := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}
	tests := []string{
		"env:MISSING", "env:EMPTY", "env:lowercase", "env:HAS-DASH", "env: BAD", "env:", "env:OVERSIZED",
	}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseListConfig(input, lookup)
			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("parseListConfig(%q) error = %v", input, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-value-must-not-leak") {
				t.Fatalf("safe error leaked environment value: %v", err)
			}
		})
	}
}

func TestCodeRunnerScalarConfigHasTheSameExplicitEnvContract(t *testing.T) {
	environment := map[string]string{"NODE_MODULES_DIR": " /opt/node_modules "}
	lookup := func(key string) (string, bool) {
		value, ok := environment[key]
		return value, ok
	}
	literal, err := parseScalarConfig("NODE_MODULES_DIR", lookup)
	if err != nil || literal != "NODE_MODULES_DIR" {
		t.Fatalf("literal scalar = %q, %v", literal, err)
	}
	explicit, err := parseScalarConfig("env:NODE_MODULES_DIR", lookup)
	if err != nil || explicit != "/opt/node_modules" {
		t.Fatalf("explicit scalar = %q, %v", explicit, err)
	}
	if _, err := parseScalarConfig("env:MISSING", lookup); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("missing scalar env error = %v", err)
	}
}

func TestLegacyRunnerRemainsAvailableAndImplementsLocalExecutionDelegate(t *testing.T) {
	t.Setenv("APP_ENV", "debug")
	t.Setenv("APP_DEV_HOST_RUNTIME_ENABLED", "true")
	runner := New(&config.BasicConfiguration{
		CodeRunnerType: config.CodeRunnerType_Sandbox,
		SandboxConfig: &config.SandboxConfig{
			MemoryLimitMb:  64,
			TimeoutSeconds: 60,
			AllowNet:       "api.example.com",
		},
	})
	if _, unavailable := runner.(*failedRunner); unavailable {
		t.Fatal("valid legacy sandbox runner must not be unavailable")
	}
	delegate, ok := any(runner).(infrasandbox.LocalExecutionDelegate)
	if !ok {
		t.Fatalf("legacy runner type %T does not implement LocalExecutionDelegate", runner)
	}
	result, err := delegate.Health(context.Background())
	if err != nil {
		t.Fatalf("delegate health: %v", err)
	}
	if result.ProtocolVersion != infrasandbox.HealthProtocolV1 {
		t.Fatalf("protocol version = %q, want %q", result.ProtocolVersion, infrasandbox.HealthProtocolV1)
	}
}

func TestControlPlaneEnabledCodeRunnerNeverFallsBackToDirect(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "true")
	t.Setenv("APP_ENV", "production")

	runner := New(&config.BasicConfiguration{})
	response, err := runner.Run(context.Background(), &coderunner.RunRequest{
		Language: coderunner.Python,
		Code:     "async def main(args): return {'unsafe': True}",
		Params:   map[string]any{},
	})

	if !errors.Is(err, coderunner.ErrCodeRunnerUnavailable) {
		t.Fatalf("Run() error = %v", err)
	}
	if response != nil {
		t.Fatalf("response = %#v", response)
	}
}

func TestControlPlaneDisabledPluginFailsClosedBeforeLegacyRunner(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "false")
	t.Setenv("APP_ENV", "production")

	runner := New(invalidLegacySandboxConfiguration())
	response, err := runner.Run(context.Background(), &coderunner.RunRequest{
		Purpose:  coderunner.PurposePlugin,
		Language: coderunner.Python,
		Code:     "async def main(args): return {'unsafe': True}",
		Params:   map[string]any{},
	})

	if !errors.Is(err, coderunner.ErrCodeRunnerUnavailable) {
		t.Fatalf("Run() error = %v, want %v", err, coderunner.ErrCodeRunnerUnavailable)
	}
	if response != nil {
		t.Fatalf("response = %#v", response)
	}
}

func TestControlPlaneDisabledUnknownPurposeIsInvalidRequest(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "false")
	t.Setenv("APP_ENV", "production")

	runner := New(invalidLegacySandboxConfiguration())
	response, err := runner.Run(context.Background(), &coderunner.RunRequest{
		Purpose:  coderunner.Purpose("future_non_agent"),
		Language: coderunner.Python,
		Code:     "async def main(args): return {'unsafe': True}",
		Params:   map[string]any{},
	})

	if !errors.Is(err, coderunner.ErrCodeRunnerInvalidRequest) {
		t.Fatalf("Run() error = %v, want %v", err, coderunner.ErrCodeRunnerInvalidRequest)
	}
	if response != nil {
		t.Fatalf("response = %#v", response)
	}
}

func TestLegacyRunnerPurposeGuardKeepsEmptyAndAgentCompatibility(t *testing.T) {
	t.Setenv("SANDBOX_CONTROL_PLANE_ENABLED", "false")
	t.Setenv("APP_ENV", "production")

	for _, purpose := range []coderunner.Purpose{"", coderunner.PurposeAgent} {
		name := string(purpose)
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			runner := New(invalidLegacySandboxConfiguration())
			response, err := runner.Run(context.Background(), &coderunner.RunRequest{
				Purpose:  purpose,
				Language: coderunner.Python,
				Code:     "async def main(args): return {}",
				Params:   map[string]any{},
			})

			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("Run() error = %v, want %v", err, ErrInvalidConfiguration)
			}
			if response != nil {
				t.Fatalf("response = %#v", response)
			}
		})
	}
}

func invalidLegacySandboxConfiguration() *config.BasicConfiguration {
	return &config.BasicConfiguration{
		CodeRunnerType: config.CodeRunnerType_Sandbox,
		SandboxConfig: &config.SandboxConfig{
			MemoryLimitMb:  63,
			TimeoutSeconds: 60,
		},
	}
}
