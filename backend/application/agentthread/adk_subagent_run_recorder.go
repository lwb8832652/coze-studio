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
)

type ADKSubagentRunStartRequest struct {
	Parent          *RunSummary
	Definition      ADKSubagentDefinition
	ArgumentsInJSON string
}

type ADKSubagentRunFinishRequest struct {
	Parent       *RunSummary
	Child        *RunSummary
	Definition   ADKSubagentDefinition
	Status       RunStatus
	ErrorCode    string
	ErrorMessage string
}

type ADKSubagentRunRecorder interface {
	StartADKSubagentRun(
		ctx context.Context,
		req ADKSubagentRunStartRequest,
	) (*RunSummary, error)
	FinishADKSubagentRun(
		ctx context.Context,
		req ADKSubagentRunFinishRequest,
	) (*RunSummary, error)
}

type ApplicationADKSubagentRunRecorder struct {
	app *ApplicationService
}

func NewApplicationADKSubagentRunRecorder(
	app *ApplicationService,
) *ApplicationADKSubagentRunRecorder {
	return &ApplicationADKSubagentRunRecorder{app: app}
}

func (r *ApplicationADKSubagentRunRecorder) StartADKSubagentRun(
	ctx context.Context,
	req ADKSubagentRunStartRequest,
) (*RunSummary, error) {
	if r == nil || r.app == nil {
		return nil, fmt.Errorf("subagent run recorder application service is required")
	}
	if req.Parent == nil || req.Parent.RunID <= 0 || req.Parent.ThreadID <= 0 {
		return nil, fmt.Errorf("parent run is required")
	}
	definition := normalizeADKSubagentDefinition(req.Definition)
	if err := validateADKSubagentDefinition(definition); err != nil {
		return nil, err
	}

	input, err := adkSubagentRunInput(definition, req.ArgumentsInJSON)
	if err != nil {
		return nil, err
	}
	config, metadata, err := adkSubagentRunPayloads(req.Parent, definition)
	if err != nil {
		return nil, err
	}
	resp, err := r.app.CreateRun(ctx, &CreateRunRequest{
		ThreadID:    req.Parent.ThreadID,
		ParentRunID: req.Parent.RunID,
		AssistantID: adkSubagentAssistantID(definition),
		RunKind:     RunKindSubagent,
		Status:      RunStatusRunning,
		Input:       input,
		Config:      config,
		Metadata:    metadata,
		StreamMode:  req.Parent.StreamMode,
		Durability:  req.Parent.Durability,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Run == nil {
		return nil, fmt.Errorf("subagent run recorder returned empty run")
	}
	return resp.Run, nil
}

func (r *ApplicationADKSubagentRunRecorder) FinishADKSubagentRun(
	ctx context.Context,
	req ADKSubagentRunFinishRequest,
) (*RunSummary, error) {
	if r == nil || r.app == nil {
		return nil, fmt.Errorf("subagent run recorder application service is required")
	}
	if req.Child == nil || req.Child.RunID <= 0 {
		return nil, fmt.Errorf("child run is required")
	}

	switch req.Status {
	case RunStatusSucceeded:
		resp, err := r.app.CompleteRun(ctx, &UpdateRunStatusRequest{
			RunID: req.Child.RunID,
			From:  RunStatusRunning,
		})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("subagent complete returned empty response")
		}
		return resp.Run, nil
	case RunStatusFailed:
		errorCode := strings.TrimSpace(req.ErrorCode)
		if errorCode == "" {
			errorCode = "subagent_failed"
		}
		resp, err := r.app.FailRun(ctx, &UpdateRunStatusRequest{
			RunID:        req.Child.RunID,
			From:         RunStatusRunning,
			ErrorCode:    errorCode,
			ErrorMessage: sanitizeADKSubagentLifecycleErrorMessage(req.ErrorMessage),
		})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("subagent fail returned empty response")
		}
		return resp.Run, nil
	case RunStatusCanceled:
		errorCode := strings.TrimSpace(req.ErrorCode)
		if errorCode == "" {
			errorCode = "subagent_canceled"
		}
		resp, err := r.app.CancelRun(ctx, &UpdateRunStatusRequest{
			RunID:        req.Child.RunID,
			From:         RunStatusRunning,
			ErrorCode:    errorCode,
			ErrorMessage: sanitizeADKSubagentLifecycleErrorMessage(req.ErrorMessage),
		})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("subagent cancel returned empty response")
		}
		return resp.Run, nil
	default:
		return nil, fmt.Errorf("unsupported subagent terminal status: %s", req.Status)
	}
}

func adkSubagentRunPayloads(
	parent *RunSummary,
	definition ADKSubagentDefinition,
) (string, string, error) {
	configPayload := map[string]any{
		"runtime":           string(RuntimeModeEinoADK),
		"agent_name":        definition.Name,
		"agent_description": definition.Description,
		"full_chat_history": definition.FullChatHistoryAsInput,
		"single_agent": map[string]any{
			"agent_id": definition.AgentID,
			"version":  definition.Version,
			"is_draft": definition.IsDraft,
		},
		"tool_policy": map[string]any{
			"allowed_tools": normalizeConfigStringSlice(
				definition.AllowedTools,
			),
			"allowed_dynamic_tools": normalizeConfigStringSlice(
				definition.AllowedDynamicTools,
			),
		},
	}
	metadataPayload := map[string]any{
		"source":        "eino_adk_subagent",
		"parent_run_id": runSummaryRunID(parent),
		"subagent":      adkChildSubagentIdentity(parent, definition.Name),
		"agent_id":      definition.AgentID,
		"is_draft":      definition.IsDraft,
	}
	if definition.Version != "" {
		metadataPayload["version"] = definition.Version
	}

	config, err := json.Marshal(configPayload)
	if err != nil {
		return "", "", fmt.Errorf("marshal subagent run config: %w", err)
	}
	metadata, err := json.Marshal(metadataPayload)
	if err != nil {
		return "", "", fmt.Errorf("marshal subagent run metadata: %w", err)
	}
	return string(config), string(metadata), nil
}

func adkSubagentRunInput(
	definition ADKSubagentDefinition,
	argumentsInJSON string,
) (string, error) {
	arguments := strings.TrimSpace(argumentsInJSON)
	if arguments == "" {
		arguments = "{}"
	}
	if !json.Valid([]byte(arguments)) {
		return "", fmt.Errorf("subagent tool arguments must be valid JSON")
	}

	payload := struct {
		Schema    string          `json:"schema"`
		ToolName  string          `json:"tool_name"`
		Arguments json.RawMessage `json:"arguments"`
	}{
		Schema:    "coze.subagent_tool_call.v1",
		ToolName:  definition.Name,
		Arguments: json.RawMessage(arguments),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal subagent run input: %w", err)
	}

	return string(encoded), nil
}

func adkSubagentAssistantID(definition ADKSubagentDefinition) string {
	if definition.AgentID > 0 {
		return fmt.Sprintf("singleagent:%d", definition.AgentID)
	}
	return fmt.Sprintf("subagent:%s", definition.Name)
}
