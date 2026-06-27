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
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
)

type adkGuardrailConfirmationPromptOptions struct {
	Title          string
	TargetLabel    string
	ResourcePrefix string
	Consequence    string
}

func adkGuardrailConfirmationInterrupt(
	ctx context.Context,
	request GuardrailRequest,
	decision GuardrailDecision,
	options adkGuardrailConfirmationPromptOptions,
) error {
	prompt, err := adkGuardrailConfirmationPrompt(
		request,
		decision,
		options,
	)
	if err != nil {
		return err
	}

	return tool.StatefulInterrupt(
		ctx,
		prompt,
		humanInteractionToolState{Prompt: prompt},
	)
}

func adkGuardrailConfirmationPrompt(
	request GuardrailRequest,
	decision GuardrailDecision,
	options adkGuardrailConfirmationPromptOptions,
) (HumanInteractionPrompt, error) {
	decision.Action = GuardrailActionConfirm
	decision = normalizeGuardrailDecision(decision)
	targetID := sanitizeGuardrailAuditTargetID(request.TargetID)
	if targetID == "" {
		targetID = "guarded_target"
	}
	targetLabel := strings.TrimSpace(options.TargetLabel)
	if targetLabel == "" {
		targetLabel = "guarded target"
	}
	resourcePrefix := sanitizeGuardrailIdentifier(options.ResourcePrefix, 64)
	if resourcePrefix == "" {
		resourcePrefix = "guarded_target"
	}
	title := strings.TrimSpace(options.Title)
	if title == "" {
		title = "Review guarded action"
	}
	consequence := strings.TrimSpace(options.Consequence)
	if consequence == "" {
		consequence = "The guarded action will not run unless this request is approved."
	}

	policyRef := strings.Trim(decision.Provider+":"+decision.ReasonCode, ":")
	descriptionParts := []string{
		"provider=" + decision.Provider,
		"reason=" + decision.ReasonCode,
	}
	if len(decision.RuleIDs) > 0 {
		descriptionParts = append(
			descriptionParts,
			"rules="+strings.Join(decision.RuleIDs, ","),
		)
	}
	affectedResources := []string{resourcePrefix + ":" + targetID}
	for _, ruleID := range decision.RuleIDs {
		affectedResources = append(affectedResources, "rule:"+ruleID)
	}

	prompt := HumanInteractionPrompt{
		Schema:        humanInteractionSchema,
		InteractionID: newHumanInteractionID(HumanInteractionKindConfirmation, policyRef+":"+targetID),
		Kind:          HumanInteractionKindConfirmation,
		Title:         title,
		Summary: "Guardrail requires approval before invoking " +
			targetLabel + " " + targetID + ".",
		Description:       strings.Join(descriptionParts, "; "),
		Required:          true,
		AllowFreeText:     true,
		RiskLevel:         HumanInteractionRiskHigh,
		ToolName:          targetID,
		PolicyRef:         policyRef,
		Action:            "invoke",
		Consequences:      []string{consequence},
		AffectedResources: affectedResources,
		DefaultDecision:   string(HumanInteractionDecisionRejected),
		RejectionGuidance: "Reject to ask the agent to choose a safer alternative.",
		CreatedAt:         time.Now().UnixMilli(),
	}
	if err := validateHumanInteractionPrompt(prompt); err != nil {
		return HumanInteractionPrompt{}, err
	}

	return prompt, nil
}

func adkGuardrailHandleConfirmationResume(ctx context.Context) (bool, error) {
	wasInterrupted, hasState, state := tool.GetInterruptState[humanInteractionToolState](ctx)
	if !wasInterrupted {
		return false, nil
	}
	if !hasState {
		return true, fmt.Errorf("guardrail confirmation state is missing")
	}
	if state.Prompt.Kind != HumanInteractionKindConfirmation {
		return true, fmt.Errorf("guardrail confirmation state kind is invalid")
	}

	isResumeTarget, hasData, data := tool.GetResumeContext[any](ctx)
	if !isResumeTarget || !hasData {
		return true, tool.StatefulInterrupt(ctx, state.Prompt, state)
	}
	response, err := normalizeHumanInteractionResponse(data)
	if err != nil {
		return true, err
	}
	if strings.TrimSpace(response.InteractionID) == "" {
		response.InteractionID = state.Prompt.InteractionID
	}
	if response.Kind == "" {
		response.Kind = state.Prompt.Kind
	}
	if response.Schema == "" {
		response.Schema = humanInteractionResponseSchema
	}
	if response.InteractionID != state.Prompt.InteractionID ||
		response.Kind != state.Prompt.Kind {
		return true, fmt.Errorf("guardrail confirmation response does not match interrupt")
	}
	if err := validateHumanInteractionResponse(response); err != nil {
		return true, err
	}
	if response.Decision == HumanInteractionDecisionRejected {
		return true, &GuardrailConfirmationRejectedError{
			ReasonCode: state.Prompt.PolicyRef,
		}
	}

	return true, nil
}
