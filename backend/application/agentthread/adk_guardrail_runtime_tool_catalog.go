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
	"errors"
	"fmt"
	"strings"
)

type ADKGuardrailEnforcer interface {
	Evaluate(
		ctx context.Context,
		request GuardrailRequest,
	) (GuardrailEnforcementResult, error)
}

type ADKGuardrailRuntimeToolCatalog struct {
	base     ADKRuntimeToolCatalog
	enforcer ADKGuardrailEnforcer
}

func NewADKGuardrailRuntimeToolCatalog(
	base ADKRuntimeToolCatalog,
	enforcer ADKGuardrailEnforcer,
) *ADKGuardrailRuntimeToolCatalog {
	return &ADKGuardrailRuntimeToolCatalog{
		base:     base,
		enforcer: enforcer,
	}
}

func (c *ADKGuardrailRuntimeToolCatalog) LoadADKRuntimeTools(
	ctx context.Context,
	run *RunSummary,
) ([]ADKRuntimeToolDefinition, error) {
	if c == nil || c.base == nil {
		return nil, fmt.Errorf("guardrail runtime tool catalog base is required")
	}
	definitions, err := c.base.LoadADKRuntimeTools(ctx, run)
	if err != nil {
		return nil, err
	}
	if c.enforcer == nil {
		return definitions, nil
	}

	wrapped := make([]ADKRuntimeToolDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.Invoker != nil {
			definition.Invoker = adkGuardrailRuntimeToolInvoker{
				name:     definition.Name,
				invoker:  definition.Invoker,
				enforcer: c.enforcer,
			}
		}
		wrapped = append(wrapped, definition)
	}

	return wrapped, nil
}

type adkGuardrailRuntimeToolInvoker struct {
	name     string
	invoker  ADKRuntimeToolInvoker
	enforcer ADKGuardrailEnforcer
}

func (i adkGuardrailRuntimeToolInvoker) InvokeADKRuntimeTool(
	ctx context.Context,
	call ADKRuntimeToolCall,
) (string, error) {
	if handled, err := adkGuardrailHandleConfirmationResume(ctx); handled {
		if err != nil {
			return "", err
		}
		if i.invoker == nil {
			return "", fmt.Errorf("runtime tool invoker is required")
		}
		return i.invoker.InvokeADKRuntimeTool(ctx, call)
	}
	if i.enforcer != nil {
		request := adkGuardrailRuntimeToolRequest(i.name, call)
		result, err := i.enforcer.Evaluate(ctx, request)
		if err != nil {
			var confirmation *GuardrailConfirmationRequiredError
			if errors.As(err, &confirmation) || result.RequiresConfirmation {
				decision := result.Decision
				if confirmation != nil &&
					strings.TrimSpace(decision.ReasonCode) == "" {
					decision.ReasonCode = confirmation.ReasonCode
				}
				return "", adkGuardrailRuntimeToolConfirmationInterrupt(
					ctx,
					request,
					decision,
				)
			}
			return "", err
		}
		if result.RequiresConfirmation {
			return "", adkGuardrailRuntimeToolConfirmationInterrupt(
				ctx,
				request,
				result.Decision,
			)
		}
		if !result.Allowed {
			return "", &GuardrailDeniedError{
				ReasonCode: result.Decision.ReasonCode,
			}
		}
	}
	if i.invoker == nil {
		return "", fmt.Errorf("runtime tool invoker is required")
	}

	return i.invoker.InvokeADKRuntimeTool(ctx, call)
}

func adkGuardrailRuntimeToolConfirmationInterrupt(
	ctx context.Context,
	request GuardrailRequest,
	decision GuardrailDecision,
) error {
	return adkGuardrailConfirmationInterrupt(
		ctx,
		request,
		decision,
		adkGuardrailConfirmationPromptOptions{
			Title:          "Review runtime tool invocation",
			TargetLabel:    "runtime tool",
			ResourcePrefix: "runtime_tool",
			Consequence:    "The runtime tool will not run unless this request is approved.",
		},
	)
}

func adkGuardrailRuntimeToolRequest(
	definitionName string,
	call ADKRuntimeToolCall,
) GuardrailRequest {
	run := call.Run
	name := strings.TrimSpace(call.Name)
	if name == "" {
		name = strings.TrimSpace(definitionName)
	}
	request := GuardrailRequest{
		TargetType: GuardrailTargetToolCall,
		TargetID:   name,
		Operation:  "invoke",
		Source:     "adk_runtime_tool",
		FailMode:   GuardrailFailClosed,
	}
	if run != nil {
		request.SpaceID = run.SpaceID
		request.ThreadID = run.ThreadID
		request.RunID = run.RunID
		request.UserID = run.CreatorID
	}

	return request
}
