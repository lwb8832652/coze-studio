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
package impl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strings"

	"github.com/coze-dev/coze-studio/backend/api/model/admin/config"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner/impl/direct"
	"github.com/coze-dev/coze-studio/backend/infra/coderunner/impl/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

type Runner = coderunner.Runner

const (
	maxListConfigEntries   = 128
	maxListConfigItemBytes = 1024
	maxListConfigBytes     = 16 * 1024
	maxScalarConfigBytes   = 4096
)

var (
	ErrInvalidConfiguration = errors.New("code runner configuration invalid")
	ErrRunnerUnavailable    = coderunner.ErrCodeRunnerUnavailable
)

func New(conf *config.BasicConfiguration) Runner {
	if os.Getenv("SANDBOX_CONTROL_PLANE_ENABLED") == "true" {
		if os.Getenv("SANDBOX_RUNTIME_ROUTING_ENABLED") != "false" ||
			conf == nil || conf.CodeRunnerType != config.CodeRunnerType_Sandbox {
			return failedRunner{err: coderunner.ErrCodeRunnerUnavailable}
		}
	}
	allowPlugin := conf != nil && conf.CodeRunnerType == config.CodeRunnerType_Sandbox
	return guardLegacyRunner(newLegacyRunner(conf), allowPlugin)
}

func newLegacyRunner(conf *config.BasicConfiguration) Runner {
	if conf == nil {
		return failedRunner{err: ErrRunnerUnavailable}
	}
	switch conf.CodeRunnerType {
	case config.CodeRunnerType_Sandbox:
		if conf.SandboxConfig == nil || conf.SandboxConfig.TimeoutSeconds < 1 ||
			conf.SandboxConfig.TimeoutSeconds > 3600 || conf.SandboxConfig.MemoryLimitMb < 64 ||
			conf.SandboxConfig.MemoryLimitMb > 32768 {
			return failedRunner{err: ErrInvalidConfiguration}
		}
		allowEnv, err := parseListConfig(conf.SandboxConfig.AllowEnv, os.LookupEnv)
		if err != nil {
			return failedRunner{err: err}
		}
		allowRead, err := parseListConfig(conf.SandboxConfig.AllowRead, os.LookupEnv)
		if err != nil {
			return failedRunner{err: err}
		}
		allowWrite, err := parseListConfig(conf.SandboxConfig.AllowWrite, os.LookupEnv)
		if err != nil {
			return failedRunner{err: err}
		}
		allowNet, err := parseListConfig(conf.SandboxConfig.AllowNet, os.LookupEnv)
		if err != nil {
			return failedRunner{err: err}
		}
		allowRun, err := parseListConfig(conf.SandboxConfig.AllowRun, os.LookupEnv)
		if err != nil {
			return failedRunner{err: err}
		}
		allowFFI, err := parseListConfig(conf.SandboxConfig.AllowFfi, os.LookupEnv)
		if err != nil {
			return failedRunner{err: err}
		}
		nodeModulesDir, err := parseScalarConfig(conf.SandboxConfig.NodeModulesDir, os.LookupEnv)
		if err != nil {
			return failedRunner{err: err}
		}
		runnerConfig := &sandbox.Config{
			AllowEnv: allowEnv, AllowRead: allowRead, AllowWrite: allowWrite,
			AllowNet: allowNet, AllowRun: allowRun, AllowFFI: allowFFI,
			NodeModulesDir: nodeModulesDir,
			TimeoutSeconds: conf.SandboxConfig.TimeoutSeconds,
			MemoryLimitMB:  conf.SandboxConfig.MemoryLimitMb,
		}

		legacyRunner := sandbox.NewRunner(runnerConfig)
		if legacyLocalExecutionDelegateEnabled() {
			return &legacyLocalExecutionRunner{Runner: legacyRunner}
		}
		return legacyRunner
	default:
		return direct.NewRunner()
	}
}

func NewLegacyLocalExecutionDelegate(
	conf *config.BasicConfiguration,
) infrasandbox.LocalExecutionDelegate {
	if !legacyLocalExecutionDelegateEnabled() || conf == nil ||
		conf.CodeRunnerType != config.CodeRunnerType_Sandbox {
		return nil
	}
	runner := newLegacyRunner(conf)
	if _, failed := runner.(failedRunner); failed {
		return nil
	}
	delegate, ok := runner.(infrasandbox.LocalExecutionDelegate)
	if !ok {
		return nil
	}
	return delegate
}

func NewUnavailable() Runner {
	return failedRunner{err: ErrRunnerUnavailable}
}

type legacyPurposeGuardRunner struct {
	Runner
	allowPlugin bool
}

func (r *legacyPurposeGuardRunner) Run(
	ctx context.Context,
	request *coderunner.RunRequest,
) (*coderunner.RunResponse, error) {
	if request != nil {
		switch request.Purpose {
		case "", coderunner.PurposeAgent:
		case coderunner.PurposePlugin:
			if !r.allowPlugin {
				return nil, coderunner.ErrCodeRunnerUnavailable
			}
		default:
			return nil, coderunner.ErrCodeRunnerInvalidRequest
		}
	}
	if r == nil || r.Runner == nil {
		return nil, coderunner.ErrCodeRunnerUnavailable
	}
	return r.Runner.Run(ctx, request)
}

type legacyLocalExecutionPurposeGuard struct {
	*legacyPurposeGuardRunner
	infrasandbox.LocalExecutionDelegate
}

func guardLegacyRunner(runner Runner, allowPlugin bool) Runner {
	guard := &legacyPurposeGuardRunner{Runner: runner, allowPlugin: allowPlugin}
	delegate, ok := runner.(infrasandbox.LocalExecutionDelegate)
	if !ok {
		return guard
	}
	return &legacyLocalExecutionPurposeGuard{
		legacyPurposeGuardRunner: guard,
		LocalExecutionDelegate:   delegate,
	}
}

type legacyLocalExecutionRunner struct {
	Runner
}

func (r *legacyLocalExecutionRunner) Health(context.Context) (infrasandbox.HealthResult, error) {
	if r == nil || r.Runner == nil {
		return infrasandbox.HealthResult{}, domainsandbox.ErrUnavailable
	}
	return infrasandbox.HealthResult{
		ProtocolVersion: infrasandbox.HealthProtocolV1,
		Status:          domainsandbox.HealthStatusHealthy,
		Capabilities:    []domainsandbox.Scope{domainsandbox.ScopeAgent},
	}, nil
}

func (r *legacyLocalExecutionRunner) Execute(
	ctx context.Context,
	request infrasandbox.ExecuteRequest,
) (infrasandbox.ExecuteResult, error) {
	if r == nil || r.Runner == nil ||
		request.Scope != domainsandbox.ScopeAgent ||
		request.WorkloadKind != infrasandbox.WorkloadAgent ||
		request.Entrypoint != "agent/code/run" {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	envelope, err := infrasandbox.ParseCodeRunnerInvokeEnvelope(
		request.Stdin,
		infrasandbox.MaxCodeRunnerEnvelopeBytes,
	)
	if err != nil {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	if !legacyCodeRunnerLanguageAllowed(
		request.Policy.AllowedExecutables,
		coderunner.Language(envelope.Language),
	) {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrExecutionForbidden
	}
	params := map[string]any{}
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrInvalidInput
	}
	response, err := r.Runner.Run(ctx, &coderunner.RunRequest{
		Code:     envelope.Code,
		Params:   params,
		Language: coderunner.Language(envelope.Language),
	})
	if err != nil || response == nil {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrUnavailable
	}
	stdout, err := infrasandbox.MarshalCodeRunnerResultEnvelope(response.Result)
	if err != nil || int64(len(stdout)) > request.Policy.MaxOutputBytes {
		return infrasandbox.ExecuteResult{}, domainsandbox.ErrUnavailable
	}
	exitCode := 0
	digest := sha256.Sum256([]byte(request.IdempotencyKey))
	return infrasandbox.ExecuteResult{
		ExecutionID: "local_" + hex.EncodeToString(digest[:16]),
		Status:      infrasandbox.ExecutionStatusSucceeded,
		ExitCode:    &exitCode,
		Stdout:      string(stdout),
	}, nil
}

func legacyCodeRunnerLanguageAllowed(
	executables []string,
	language coderunner.Language,
) bool {
	for _, executable := range executables {
		if language == coderunner.Python &&
			(executable == "python" || executable == "python3") {
			return true
		}
		if language == coderunner.JavaScript &&
			(executable == "node" || executable == "nodejs") {
			return true
		}
	}
	return false
}

func (r *legacyLocalExecutionRunner) Cancel(context.Context, string) error {
	return domainsandbox.ErrUnavailable
}

func legacyLocalExecutionDelegateEnabled() bool {
	return os.Getenv("APP_ENV") == "debug" && os.Getenv("APP_DEV_HOST_RUNTIME_ENABLED") == "true"
}

type failedRunner struct {
	err error
}

func (r failedRunner) Run(context.Context, *coderunner.RunRequest) (*coderunner.RunResponse, error) {
	if r.err == nil {
		return nil, ErrRunnerUnavailable
	}
	return nil, r.err
}

func parseListConfig(value string, lookupEnv func(string) (string, bool)) ([]string, error) {
	resolved, _, err := resolveConfigValue(value, lookupEnv)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		return nil, nil
	}
	if len(resolved) > maxListConfigBytes {
		return nil, ErrInvalidConfiguration
	}
	unique := make(map[string]struct{})
	for _, item := range strings.Split(resolved, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if len(item) > maxListConfigItemBytes || !validConfigText(item) {
			return nil, ErrInvalidConfiguration
		}
		unique[item] = struct{}{}
		if len(unique) > maxListConfigEntries {
			return nil, ErrInvalidConfiguration
		}
	}
	items := make([]string, 0, len(unique))
	for item := range unique {
		items = append(items, item)
	}
	sort.Strings(items)
	return items, nil
}

func parseScalarConfig(value string, lookupEnv func(string) (string, bool)) (string, error) {
	resolved, _, err := resolveConfigValue(value, lookupEnv)
	if err != nil {
		return "", err
	}
	if len(resolved) > maxScalarConfigBytes || !validConfigText(resolved) {
		return "", ErrInvalidConfiguration
	}
	return resolved, nil
}

func resolveConfigValue(value string, lookupEnv func(string) (string, bool)) (string, bool, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "env:") {
		return value, false, nil
	}
	name := strings.TrimPrefix(value, "env:")
	if !validCanonicalEnvReference(name) || lookupEnv == nil {
		return "", true, ErrInvalidConfiguration
	}
	resolved, ok := lookupEnv(name)
	resolved = strings.TrimSpace(resolved)
	if !ok || resolved == "" {
		return "", true, ErrInvalidConfiguration
	}
	return resolved, true, nil
}

func validCanonicalEnvReference(value string) bool {
	if len(value) == 0 || len(value) > 128 || value[0] != '_' && (value[0] < 'A' || value[0] > 'Z') {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if character != '_' && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validConfigText(value string) bool {
	for _, character := range value {
		if character == 0 || character == 0x7f || character < 0x20 {
			return false
		}
	}
	return true
}
