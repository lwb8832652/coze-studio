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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/adaptivecontract"
	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const (
	maxAdaptiveDecisionSemanticInputBytes    = 64 << 10
	maxAdaptiveDecisionSemanticInputMessages = 32
	maxAdaptiveDecisionCandidateWireBytes    = 64 << 10
	adaptiveDecisionCandidateWireSchema      = "coze.adaptive_decision_candidate.v1"
)

type adaptiveDecisionRunInputEnvelope struct {
	Messages      json.RawMessage `json:"messages"`
	UploadedFiles json.RawMessage `json:"uploaded_files,omitempty"`
}

type adaptiveDecisionRunInputMessage struct {
	RunID   int64  `json:"_run_id,omitempty"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

type adaptiveDecisionCandidateWire struct {
	Schema                string                                `json:"schema"`
	GoalSummary           string                                `json:"goal_summary"`
	Deliverables          []string                              `json:"deliverables"`
	AcceptanceChecks      []adaptiveDecisionAcceptanceCheckWire `json:"acceptance_checks"`
	Decision              domainentity.ExecutionDecisionKind    `json:"decision"`
	ExecutionShape        domainentity.ExecutionShape           `json:"execution_shape"`
	ClarificationQuestion *string                               `json:"clarification_question,omitempty"`
	SafeSummary           string                                `json:"safe_summary"`
}

type adaptiveDecisionAcceptanceCheckWire struct {
	CheckID         string `json:"check_id"`
	Kind            string `json:"kind"`
	TargetRef       string `json:"target_ref"`
	SafeDescription string `json:"safe_description"`
}

func ProjectAdaptiveDecisionSemanticInput(rawInput string) (AdaptiveDecisionSemanticInput, error) {
	if len(rawInput) == 0 || len(rawInput) > maxAdaptiveDecisionSemanticInputBytes ||
		!utf8.ValidString(rawInput) || strings.TrimSpace(rawInput) == "" {
		return AdaptiveDecisionSemanticInput{}, fmt.Errorf("adaptive decision semantic input is invalid")
	}
	raw := []byte(rawInput)
	if adaptiveDecisionJSONContainsNull(raw) {
		return AdaptiveDecisionSemanticInput{}, fmt.Errorf("adaptive decision semantic input contains null")
	}

	var envelope adaptiveDecisionRunInputEnvelope
	if err := decodeAdaptiveDecisionClosedJSON(raw, &envelope); err != nil {
		return AdaptiveDecisionSemanticInput{}, fmt.Errorf("decode adaptive decision semantic input: %w", err)
	}
	if len(envelope.Messages) == 0 {
		return AdaptiveDecisionSemanticInput{}, fmt.Errorf("adaptive decision semantic messages are required")
	}

	var inputMessages []*adaptiveDecisionRunInputMessage
	if err := decodeAdaptiveDecisionClosedJSON(envelope.Messages, &inputMessages); err != nil {
		return AdaptiveDecisionSemanticInput{}, fmt.Errorf("decode adaptive decision semantic messages: %w", err)
	}
	if len(inputMessages) == 0 || len(inputMessages) > maxAdaptiveDecisionSemanticInputMessages {
		return AdaptiveDecisionSemanticInput{}, fmt.Errorf("adaptive decision semantic message count is invalid")
	}

	messages := make([]AdaptiveDecisionSemanticMessage, 0, len(inputMessages))
	for _, message := range inputMessages {
		if message == nil || (message.Role != "user" && message.Role != "assistant") ||
			strings.TrimSpace(message.Content) == "" {
			return AdaptiveDecisionSemanticInput{}, fmt.Errorf("adaptive decision semantic message is invalid")
		}
		messages = append(messages, AdaptiveDecisionSemanticMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	if messages[len(messages)-1].Role != "user" {
		return AdaptiveDecisionSemanticInput{}, fmt.Errorf("adaptive decision semantic input must end with a user message")
	}

	hasAttachments := false
	if len(envelope.UploadedFiles) > 0 {
		var files []*TaskThreadUploadedFileSummary
		if err := decodeAdaptiveDecisionClosedJSON(envelope.UploadedFiles, &files); err != nil {
			return AdaptiveDecisionSemanticInput{}, fmt.Errorf("decode adaptive decision attachments: %w", err)
		}
		for _, file := range files {
			if file == nil {
				return AdaptiveDecisionSemanticInput{}, fmt.Errorf("adaptive decision attachment is invalid")
			}
		}
		hasAttachments = len(files) > 0
	}
	return AdaptiveDecisionSemanticInput{
		Messages:       messages,
		HasAttachments: hasAttachments,
	}, nil
}

func decodeAdaptiveDecisionClosedJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are forbidden")
		}
		return err
	}
	return nil
}

func adaptiveDecisionJSONContainsNull(raw []byte) bool {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	return adaptiveDecisionValueContainsNull(value)
}

func adaptiveDecisionValueContainsNull(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if adaptiveDecisionValueContainsNull(item) {
				return true
			}
		}
	case map[string]any:
		for _, item := range typed {
			if adaptiveDecisionValueContainsNull(item) {
				return true
			}
		}
	}
	return false
}

func decodeAdaptiveDecisionCandidateWire(
	raw []byte,
	admission domainentity.AdaptiveAdmissionSnapshot,
) (AdaptiveDecisionCandidate, error) {
	if len(raw) == 0 || len(raw) > maxAdaptiveDecisionCandidateWireBytes ||
		!utf8.Valid(raw) || adaptiveDecisionJSONContainsNull(raw) {
		return AdaptiveDecisionCandidate{}, fmt.Errorf("adaptive decision candidate is invalid")
	}
	var wire adaptiveDecisionCandidateWire
	if err := decodeAdaptiveDecisionClosedJSON(raw, &wire); err != nil {
		return AdaptiveDecisionCandidate{}, fmt.Errorf("decode adaptive decision candidate: %w", err)
	}
	if wire.Schema != adaptiveDecisionCandidateWireSchema || wire.Deliverables == nil ||
		wire.AcceptanceChecks == nil {
		return AdaptiveDecisionCandidate{}, fmt.Errorf("adaptive decision candidate contract is invalid")
	}

	checks := make([]domainentity.AdaptiveAcceptanceCheck, 0, len(wire.AcceptanceChecks))
	for _, check := range wire.AcceptanceChecks {
		checks = append(checks, domainentity.AdaptiveAcceptanceCheck{
			CheckID:         check.CheckID,
			Kind:            check.Kind,
			TargetRef:       check.TargetRef,
			SafeDescription: check.SafeDescription,
		})
	}
	candidate := AdaptiveDecisionCandidate{
		GoalSummary:           wire.GoalSummary,
		Deliverables:          append(make([]string, 0, len(wire.Deliverables)), wire.Deliverables...),
		AcceptanceChecks:      checks,
		Decision:              wire.Decision,
		ExecutionShape:        wire.ExecutionShape,
		ClarificationQuestion: cloneAdaptiveDecisionString(wire.ClarificationQuestion),
		SafeSummary:           wire.SafeSummary,
	}
	if err := validateAdaptiveDecisionCandidate(admission, candidate); err != nil {
		return AdaptiveDecisionCandidate{}, err
	}
	return candidate, nil
}

func encodeAdaptiveDecisionCandidateWire(candidate AdaptiveDecisionCandidate) ([]byte, error) {
	checks := make([]adaptiveDecisionAcceptanceCheckWire, 0, len(candidate.AcceptanceChecks))
	for _, check := range candidate.AcceptanceChecks {
		checks = append(checks, adaptiveDecisionAcceptanceCheckWire{
			CheckID:         check.CheckID,
			Kind:            check.Kind,
			TargetRef:       check.TargetRef,
			SafeDescription: check.SafeDescription,
		})
	}
	encoded, err := json.Marshal(adaptiveDecisionCandidateWire{
		Schema:                adaptiveDecisionCandidateWireSchema,
		GoalSummary:           candidate.GoalSummary,
		Deliverables:          append(make([]string, 0, len(candidate.Deliverables)), candidate.Deliverables...),
		AcceptanceChecks:      checks,
		Decision:              candidate.Decision,
		ExecutionShape:        candidate.ExecutionShape,
		ClarificationQuestion: cloneAdaptiveDecisionString(candidate.ClarificationQuestion),
		SafeSummary:           candidate.SafeSummary,
	})
	if err != nil {
		return nil, err
	}
	var canonical any
	if err := json.Unmarshal(encoded, &canonical); err != nil {
		return nil, err
	}
	return json.Marshal(canonical)
}

func validateAdaptiveDecisionCandidate(
	admission domainentity.AdaptiveAdmissionSnapshot,
	candidate AdaptiveDecisionCandidate,
) error {
	if err := adaptivecontract.ValidateAdaptiveAdmissionSnapshot(admission); err != nil {
		return err
	}
	var planScopeRunID *int64
	if candidate.Decision == domainentity.ExecutionDecisionExecute &&
		candidate.ExecutionShape == domainentity.ExecutionShapeMultiStep {
		value := int64(1)
		planScopeRunID = &value
	}
	decision := domainentity.ExecutionDecision{
		Schema:                domainentity.ExecutionDecisionSchemaV1,
		DecisionID:            "candidate-validation",
		DecisionRevision:      1,
		ExecutionRunID:        1,
		JournalRunID:          1,
		AttemptID:             "candidate-validation",
		ExecutionGeneration:   1,
		PlanScopeRunID:        planScopeRunID,
		GoalSummary:           candidate.GoalSummary,
		Deliverables:          candidate.Deliverables,
		AcceptanceChecks:      candidate.AcceptanceChecks,
		Decision:              candidate.Decision,
		ExecutionShape:        candidate.ExecutionShape,
		ClarificationQuestion: candidate.ClarificationQuestion,
		SafeSummary:           candidate.SafeSummary,
		CreatedAt:             1,
	}
	return adaptivecontract.ValidateExecutionDecisionAgainstAdmission(admission, decision)
}

func cloneAdaptiveDecisionString(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
