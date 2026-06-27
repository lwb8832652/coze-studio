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
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
)

const (
	adkSubagentRetryCommandSchema   = "coze.subagent_retry.v1"
	adkSubagentToolCallInputSchema  = "coze.subagent_tool_call.v1"
	adkSubagentRetryExecutionSource = "eino_adk_subagent_retry"
)

type ADKSubagentRetrySourceRequest struct {
	RetryRun    *RunSummary
	SourceRunID int64
	ParentRunID int64
}

type ADKSubagentRetrySourceResolver interface {
	ResolveADKSubagentRetrySource(
		ctx context.Context,
		req ADKSubagentRetrySourceRequest,
	) (*RunSummary, error)
}

type ADKSubagentRetrySourceResolverFunc func(
	ctx context.Context,
	req ADKSubagentRetrySourceRequest,
) (*RunSummary, error)

func (f ADKSubagentRetrySourceResolverFunc) ResolveADKSubagentRetrySource(
	ctx context.Context,
	req ADKSubagentRetrySourceRequest,
) (*RunSummary, error) {
	if f == nil {
		return nil, fmt.Errorf("subagent retry source resolver is required")
	}
	return f(ctx, req)
}

type ApplicationADKSubagentRetrySourceResolver struct {
	app *ApplicationService
}

func NewApplicationADKSubagentRetrySourceResolver(
	app *ApplicationService,
) *ApplicationADKSubagentRetrySourceResolver {
	return &ApplicationADKSubagentRetrySourceResolver{app: app}
}

func (r *ApplicationADKSubagentRetrySourceResolver) ResolveADKSubagentRetrySource(
	ctx context.Context,
	req ADKSubagentRetrySourceRequest,
) (*RunSummary, error) {
	if r == nil || r.app == nil {
		return nil, fmt.Errorf("subagent retry source resolver application service is required")
	}
	if req.SourceRunID <= 0 {
		return nil, fmt.Errorf("source subagent run id is required")
	}

	resp, err := r.app.GetRun(ctx, &GetRunRequest{RunID: req.SourceRunID})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Run == nil {
		return nil, fmt.Errorf("source subagent run is required")
	}

	return resp.Run, nil
}

func (e *ADKExecutor) ExecuteSubagentRetry(
	ctx context.Context,
	run *RunSummary,
) (*RunExecutionResult, error) {
	if err := e.validate(run); err != nil {
		return nil, err
	}
	command, err := parseADKSubagentRetryCommand(run.Command)
	if err != nil {
		return nil, err
	}
	if e.subagentRetrySourceResolver == nil {
		return nil, fmt.Errorf("subagent retry source resolver is required")
	}

	sourceRun, err := e.subagentRetrySourceResolver.ResolveADKSubagentRetrySource(
		ctx,
		ADKSubagentRetrySourceRequest{
			RetryRun:    run,
			SourceRunID: command.SourceRunID,
			ParentRunID: command.ParentRunID,
		},
	)
	if err != nil {
		return nil, err
	}
	if err := validateADKSubagentRetrySource(run, sourceRun, command); err != nil {
		return nil, err
	}

	toolCall, err := parseADKSubagentReplayInput(sourceRun.Input)
	if err != nil {
		return nil, err
	}
	replayConfig, err := parseADKSubagentReplayConfig(sourceRun.Config)
	if err != nil {
		return nil, err
	}
	if replayConfig.AgentName != "" && toolCall.ToolName != replayConfig.AgentName {
		return nil, fmt.Errorf("subagent retry tool name does not match source config")
	}

	childRun := *sourceRun
	agent, err := e.factory.Build(ctx, &childRun)
	if err != nil {
		return nil, fmt.Errorf("build subagent retry agent: %w", err)
	}
	if agent == nil {
		return nil, fmt.Errorf("subagent retry agent factory returned empty agent")
	}

	options := []adk.AgentToolOption{}
	if replayConfig.FullChatHistory {
		options = append(options, adk.WithFullChatHistoryAsInput())
	}
	agentTool := adk.NewAgentTool(ctx, agent, options...)
	invokable, ok := agentTool.(tool.InvokableTool)
	if !ok {
		return nil, fmt.Errorf("subagent retry agent tool is not invokable")
	}

	message, err := invokable.InvokableRun(ctx, string(toolCall.Arguments))
	if err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(map[string]any{
		"source":        adkSubagentRetryExecutionSource,
		"source_run_id": command.SourceRunID,
		"parent_run_id": command.ParentRunID,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal subagent retry metadata: %w", err)
	}

	return &RunExecutionResult{
		Message:  message,
		Metadata: string(metadata),
	}, nil
}

type adkSubagentRetryCommand struct {
	SourceRunID int64
	ParentRunID int64
}

func parseADKSubagentRetryCommand(command string) (adkSubagentRetryCommand, error) {
	if strings.TrimSpace(command) == "" {
		return adkSubagentRetryCommand{}, fmt.Errorf("subagent retry command is required")
	}
	var payload struct {
		SubagentRetry struct {
			Schema      string `json:"schema"`
			SourceRunID int64  `json:"source_run_id"`
			ParentRunID int64  `json:"parent_run_id"`
		} `json:"subagent_retry"`
	}
	if err := json.Unmarshal([]byte(command), &payload); err != nil {
		return adkSubagentRetryCommand{}, fmt.Errorf("parse subagent retry command: %w", err)
	}
	if payload.SubagentRetry.Schema != adkSubagentRetryCommandSchema {
		return adkSubagentRetryCommand{}, fmt.Errorf("unsupported subagent retry command schema")
	}
	if payload.SubagentRetry.SourceRunID <= 0 {
		return adkSubagentRetryCommand{}, fmt.Errorf("source subagent run id is required")
	}
	if payload.SubagentRetry.ParentRunID <= 0 {
		return adkSubagentRetryCommand{}, fmt.Errorf("parent run id is required")
	}

	return adkSubagentRetryCommand{
		SourceRunID: payload.SubagentRetry.SourceRunID,
		ParentRunID: payload.SubagentRetry.ParentRunID,
	}, nil
}

type adkSubagentReplayInput struct {
	Schema    string          `json:"schema"`
	ToolName  string          `json:"tool_name"`
	Arguments json.RawMessage `json:"arguments"`
}

func parseADKSubagentReplayInput(input string) (adkSubagentReplayInput, error) {
	if strings.TrimSpace(input) == "" {
		return adkSubagentReplayInput{}, fmt.Errorf("source subagent input is required")
	}
	var payload adkSubagentReplayInput
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return adkSubagentReplayInput{}, fmt.Errorf("parse source subagent input: %w", err)
	}
	if payload.Schema != adkSubagentToolCallInputSchema {
		return adkSubagentReplayInput{}, fmt.Errorf("unsupported source subagent input schema")
	}
	payload.ToolName = strings.TrimSpace(payload.ToolName)
	if payload.ToolName == "" {
		return adkSubagentReplayInput{}, fmt.Errorf("source subagent tool name is required")
	}
	if len(payload.Arguments) == 0 || strings.TrimSpace(string(payload.Arguments)) == "null" {
		payload.Arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(payload.Arguments) {
		return adkSubagentReplayInput{}, fmt.Errorf("source subagent arguments must be valid JSON")
	}

	return payload, nil
}

type adkSubagentReplayConfig struct {
	AgentName       string `json:"agent_name"`
	FullChatHistory bool   `json:"full_chat_history"`
}

func parseADKSubagentReplayConfig(config string) (adkSubagentReplayConfig, error) {
	if strings.TrimSpace(config) == "" {
		return adkSubagentReplayConfig{}, fmt.Errorf("source subagent config is required")
	}
	var payload adkSubagentReplayConfig
	if err := json.Unmarshal([]byte(config), &payload); err != nil {
		return adkSubagentReplayConfig{}, fmt.Errorf("parse source subagent config: %w", err)
	}
	payload.AgentName = strings.TrimSpace(payload.AgentName)

	return payload, nil
}

func validateADKSubagentRetrySource(
	retryRun *RunSummary,
	sourceRun *RunSummary,
	command adkSubagentRetryCommand,
) error {
	if sourceRun == nil {
		return fmt.Errorf("source subagent run is required")
	}
	if sourceRun.RunID != command.SourceRunID {
		return fmt.Errorf("source subagent run id does not match retry command")
	}
	if retryRun != nil && retryRun.ThreadID > 0 && sourceRun.ThreadID != retryRun.ThreadID {
		return fmt.Errorf("source subagent run does not belong to retry thread")
	}
	if sourceRun.RunKind != RunKindSubagent {
		return fmt.Errorf("source run must be a subagent run")
	}
	if sourceRun.ParentRunID != command.ParentRunID {
		return fmt.Errorf("source subagent parent run does not match retry command")
	}

	return nil
}
