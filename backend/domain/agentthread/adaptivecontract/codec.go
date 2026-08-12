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

package adaptivecontract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const maxAdaptiveContractBytes = 64 * 1024

// The wire types intentionally do not use omitempty. Their declaration order is
// the canonical JSON field order.
type admissionWire struct {
	Schema                    string                         `json:"schema"`
	FeatureGateEnabled        bool                           `json:"feature_gate_enabled"`
	Source                    entity.AdaptiveAdmissionSource `json:"source"`
	SourceRunID               *int64                         `json:"source_run_id"`
	SourceExecutionGeneration *uint64                        `json:"source_execution_generation"`
	SourceConfigDigest        string                         `json:"source_config_digest"`
	DecoderVersion            string                         `json:"decoder_version"`
	Capabilities              admissionCapabilitiesWire      `json:"capabilities"`
	Limits                    admissionLimitsWire            `json:"limits"`
}

type admissionCapabilitiesWire struct {
	PlanAllowed             bool `json:"plan_allowed"`
	ReadOnlyToolsAllowed    bool `json:"read_only_tools_allowed"`
	SandboxWritesAllowed    bool `json:"sandbox_writes_allowed"`
	HumanInteractionAllowed bool `json:"human_interaction_allowed"`
	SubagentsAllowed        bool `json:"subagents_allowed"`
}

type admissionLimitsWire struct {
	MaxToolCalls             int `json:"max_tool_calls"`
	MaxReplans               int `json:"max_replans"`
	MaxVerificationRepairs   int `json:"max_verification_repairs"`
	MaxConsecutiveNoProgress int `json:"max_consecutive_no_progress"`
	MaxActiveDurationSeconds int `json:"max_active_duration_seconds"`
}

type executionDecisionWire struct {
	Schema                string                       `json:"schema"`
	DecisionID            string                       `json:"decision_id"`
	DecisionRevision      uint64                       `json:"decision_revision"`
	ExecutionRunID        int64                        `json:"execution_run_id"`
	JournalRunID          int64                        `json:"journal_run_id"`
	AttemptID             string                       `json:"attempt_id"`
	ExecutionGeneration   uint64                       `json:"execution_generation"`
	PlanScopeRunID        *int64                       `json:"plan_scope_run_id"`
	GoalSummary           string                       `json:"goal_summary"`
	Deliverables          []string                     `json:"deliverables"`
	AcceptanceChecks      []acceptanceCheckWire        `json:"acceptance_checks"`
	Decision              entity.ExecutionDecisionKind `json:"decision"`
	ExecutionShape        entity.ExecutionShape        `json:"execution_shape"`
	ClarificationQuestion *string                      `json:"clarification_question"`
	SafeSummary           string                       `json:"safe_summary"`
	CreatedAt             int64                        `json:"created_at"`
}

type acceptanceCheckWire struct {
	CheckID         string `json:"check_id"`
	Kind            string `json:"kind"`
	TargetRef       string `json:"target_ref"`
	SafeDescription string `json:"safe_description"`
}

var admissionWireFields = []string{
	"schema", "feature_gate_enabled", "source", "source_run_id", "source_execution_generation",
	"source_config_digest", "decoder_version", "capabilities", "limits",
}

var admissionCapabilitiesWireFields = []string{
	"plan_allowed", "read_only_tools_allowed", "sandbox_writes_allowed", "human_interaction_allowed", "subagents_allowed",
}

var admissionLimitsWireFields = []string{
	"max_tool_calls", "max_replans", "max_verification_repairs", "max_consecutive_no_progress", "max_active_duration_seconds",
}

var executionDecisionWireFields = []string{
	"schema", "decision_id", "decision_revision", "execution_run_id", "journal_run_id", "attempt_id",
	"execution_generation", "plan_scope_run_id", "goal_summary", "deliverables", "acceptance_checks", "decision",
	"execution_shape", "clarification_question", "safe_summary", "created_at",
}

var acceptanceCheckWireFields = []string{"check_id", "kind", "target_ref", "safe_description"}

// EncodeAdaptiveAdmission validates and deterministically encodes an admission
// snapshot. The digest is the lowercase SHA-256 of canonical bytes.
func EncodeAdaptiveAdmission(snapshot entity.AdaptiveAdmissionSnapshot) ([]byte, string, error) {
	if err := ValidateAdaptiveAdmissionSnapshot(snapshot); err != nil {
		return nil, "", ErrAdaptiveAdmissionInvalid
	}
	canonical, err := json.Marshal(admissionToWire(snapshot))
	if err != nil || len(canonical) > maxAdaptiveContractBytes {
		return nil, "", ErrAdaptiveAdmissionInvalid
	}
	return canonical, digestCanonical(canonical), nil
}

// DecodeAdaptiveAdmission strictly decodes exactly one admission wire object and
// returns its independently allocated domain value, canonical bytes, and digest.
func DecodeAdaptiveAdmission(raw []byte) (entity.AdaptiveAdmissionSnapshot, []byte, string, error) {
	if !utf8.Valid(raw) || len(raw) > maxAdaptiveContractBytes || !validJSONStrings(raw) {
		return entity.AdaptiveAdmissionSnapshot{}, nil, "", ErrAdaptiveAdmissionInvalid
	}
	wire, ok := decodeAdmissionWire(raw)
	if !ok {
		return entity.AdaptiveAdmissionSnapshot{}, nil, "", ErrAdaptiveAdmissionInvalid
	}
	snapshot := admissionFromWire(wire)
	canonical, digest, err := EncodeAdaptiveAdmission(snapshot)
	if err != nil {
		return entity.AdaptiveAdmissionSnapshot{}, nil, "", ErrAdaptiveAdmissionInvalid
	}
	return snapshot, canonical, digest, nil
}

// EncodeExecutionDecision validates and deterministically encodes a decision.
// The digest is the lowercase SHA-256 of canonical bytes.
func EncodeExecutionDecision(decision entity.ExecutionDecision) ([]byte, string, error) {
	if err := ValidateExecutionDecision(decision); err != nil {
		return nil, "", ErrExecutionDecisionInvalid
	}
	canonical, err := json.Marshal(decisionToWire(decision))
	if err != nil || len(canonical) > maxAdaptiveContractBytes {
		return nil, "", ErrExecutionDecisionInvalid
	}
	return canonical, digestCanonical(canonical), nil
}

// DecodeExecutionDecision strictly decodes exactly one decision wire object and
// returns its independently allocated domain value, canonical bytes, and digest.
func DecodeExecutionDecision(raw []byte) (entity.ExecutionDecision, []byte, string, error) {
	if !utf8.Valid(raw) || len(raw) > maxAdaptiveContractBytes || !validJSONStrings(raw) {
		return entity.ExecutionDecision{}, nil, "", ErrExecutionDecisionInvalid
	}
	wire, ok := decodeExecutionDecisionWire(raw)
	if !ok {
		return entity.ExecutionDecision{}, nil, "", ErrExecutionDecisionInvalid
	}
	decision := decisionFromWire(wire)
	canonical, digest, err := EncodeExecutionDecision(decision)
	if err != nil {
		return entity.ExecutionDecision{}, nil, "", ErrExecutionDecisionInvalid
	}
	return decision, canonical, digest, nil
}

func admissionToWire(snapshot entity.AdaptiveAdmissionSnapshot) admissionWire {
	return admissionWire{
		Schema:                    snapshot.Schema,
		FeatureGateEnabled:        snapshot.FeatureGateEnabled,
		Source:                    snapshot.Source,
		SourceRunID:               copyInt64Pointer(snapshot.SourceRunID),
		SourceExecutionGeneration: copyUint64Pointer(snapshot.SourceExecutionGeneration),
		SourceConfigDigest:        snapshot.SourceConfigDigest,
		DecoderVersion:            snapshot.DecoderVersion,
		Capabilities: admissionCapabilitiesWire{
			PlanAllowed:             snapshot.Capabilities.PlanAllowed,
			ReadOnlyToolsAllowed:    snapshot.Capabilities.ReadOnlyToolsAllowed,
			SandboxWritesAllowed:    snapshot.Capabilities.SandboxWritesAllowed,
			HumanInteractionAllowed: snapshot.Capabilities.HumanInteractionAllowed,
			SubagentsAllowed:        snapshot.Capabilities.SubagentsAllowed,
		},
		Limits: admissionLimitsWire{
			MaxToolCalls:             snapshot.Limits.MaxToolCalls,
			MaxReplans:               snapshot.Limits.MaxReplans,
			MaxVerificationRepairs:   snapshot.Limits.MaxVerificationRepairs,
			MaxConsecutiveNoProgress: snapshot.Limits.MaxConsecutiveNoProgress,
			MaxActiveDurationSeconds: snapshot.Limits.MaxActiveDurationSeconds,
		},
	}
}

func admissionFromWire(wire admissionWire) entity.AdaptiveAdmissionSnapshot {
	return entity.AdaptiveAdmissionSnapshot{
		Schema:                    wire.Schema,
		FeatureGateEnabled:        wire.FeatureGateEnabled,
		Source:                    wire.Source,
		SourceRunID:               copyInt64Pointer(wire.SourceRunID),
		SourceExecutionGeneration: copyUint64Pointer(wire.SourceExecutionGeneration),
		SourceConfigDigest:        wire.SourceConfigDigest,
		DecoderVersion:            wire.DecoderVersion,
		Capabilities: entity.AdaptiveCapabilities{
			PlanAllowed:             wire.Capabilities.PlanAllowed,
			ReadOnlyToolsAllowed:    wire.Capabilities.ReadOnlyToolsAllowed,
			SandboxWritesAllowed:    wire.Capabilities.SandboxWritesAllowed,
			HumanInteractionAllowed: wire.Capabilities.HumanInteractionAllowed,
			SubagentsAllowed:        wire.Capabilities.SubagentsAllowed,
		},
		Limits: entity.AdaptiveLimits{
			MaxToolCalls:             wire.Limits.MaxToolCalls,
			MaxReplans:               wire.Limits.MaxReplans,
			MaxVerificationRepairs:   wire.Limits.MaxVerificationRepairs,
			MaxConsecutiveNoProgress: wire.Limits.MaxConsecutiveNoProgress,
			MaxActiveDurationSeconds: wire.Limits.MaxActiveDurationSeconds,
		},
	}
}

func decisionToWire(decision entity.ExecutionDecision) executionDecisionWire {
	deliverables := make([]string, len(decision.Deliverables))
	copy(deliverables, decision.Deliverables)
	checks := make([]acceptanceCheckWire, len(decision.AcceptanceChecks))
	for index, check := range decision.AcceptanceChecks {
		checks[index] = acceptanceCheckWire{
			CheckID:         check.CheckID,
			Kind:            check.Kind,
			TargetRef:       check.TargetRef,
			SafeDescription: check.SafeDescription,
		}
	}
	return executionDecisionWire{
		Schema:                decision.Schema,
		DecisionID:            decision.DecisionID,
		DecisionRevision:      decision.DecisionRevision,
		ExecutionRunID:        decision.ExecutionRunID,
		JournalRunID:          decision.JournalRunID,
		AttemptID:             decision.AttemptID,
		ExecutionGeneration:   decision.ExecutionGeneration,
		PlanScopeRunID:        copyInt64Pointer(decision.PlanScopeRunID),
		GoalSummary:           decision.GoalSummary,
		Deliverables:          deliverables,
		AcceptanceChecks:      checks,
		Decision:              decision.Decision,
		ExecutionShape:        decision.ExecutionShape,
		ClarificationQuestion: copyStringPointer(decision.ClarificationQuestion),
		SafeSummary:           decision.SafeSummary,
		CreatedAt:             decision.CreatedAt,
	}
}

func decisionFromWire(wire executionDecisionWire) entity.ExecutionDecision {
	deliverables := make([]string, len(wire.Deliverables))
	copy(deliverables, wire.Deliverables)
	checks := make([]entity.AdaptiveAcceptanceCheck, len(wire.AcceptanceChecks))
	for index, check := range wire.AcceptanceChecks {
		checks[index] = entity.AdaptiveAcceptanceCheck{
			CheckID:         check.CheckID,
			Kind:            check.Kind,
			TargetRef:       check.TargetRef,
			SafeDescription: check.SafeDescription,
		}
	}
	return entity.ExecutionDecision{
		Schema:                wire.Schema,
		DecisionID:            wire.DecisionID,
		DecisionRevision:      wire.DecisionRevision,
		ExecutionRunID:        wire.ExecutionRunID,
		JournalRunID:          wire.JournalRunID,
		AttemptID:             wire.AttemptID,
		ExecutionGeneration:   wire.ExecutionGeneration,
		PlanScopeRunID:        copyInt64Pointer(wire.PlanScopeRunID),
		GoalSummary:           wire.GoalSummary,
		Deliverables:          deliverables,
		AcceptanceChecks:      checks,
		Decision:              wire.Decision,
		ExecutionShape:        wire.ExecutionShape,
		ClarificationQuestion: copyStringPointer(wire.ClarificationQuestion),
		SafeSummary:           wire.SafeSummary,
		CreatedAt:             wire.CreatedAt,
	}
}

func decodeAdmissionWire(raw []byte) (admissionWire, bool) {
	fields, ok := strictObject(raw, admissionWireFields)
	if !ok {
		return admissionWire{}, false
	}
	capabilities, ok := decodeAdmissionCapabilities(fields["capabilities"])
	if !ok {
		return admissionWire{}, false
	}
	limits, ok := decodeAdmissionLimits(fields["limits"])
	if !ok {
		return admissionWire{}, false
	}
	schema, ok := decodeString(fields["schema"])
	if !ok {
		return admissionWire{}, false
	}
	featureGateEnabled, ok := decodeBool(fields["feature_gate_enabled"])
	if !ok {
		return admissionWire{}, false
	}
	source, ok := decodeString(fields["source"])
	if !ok {
		return admissionWire{}, false
	}
	sourceRunID, ok := decodeNullableInt64(fields["source_run_id"])
	if !ok {
		return admissionWire{}, false
	}
	sourceGeneration, ok := decodeNullableUint64(fields["source_execution_generation"])
	if !ok {
		return admissionWire{}, false
	}
	sourceConfigDigest, ok := decodeString(fields["source_config_digest"])
	if !ok {
		return admissionWire{}, false
	}
	decoderVersion, ok := decodeString(fields["decoder_version"])
	if !ok {
		return admissionWire{}, false
	}
	return admissionWire{
		Schema:                    schema,
		FeatureGateEnabled:        featureGateEnabled,
		Source:                    entity.AdaptiveAdmissionSource(source),
		SourceRunID:               sourceRunID,
		SourceExecutionGeneration: sourceGeneration,
		SourceConfigDigest:        sourceConfigDigest,
		DecoderVersion:            decoderVersion,
		Capabilities:              capabilities,
		Limits:                    limits,
	}, true
}

func decodeAdmissionCapabilities(raw json.RawMessage) (admissionCapabilitiesWire, bool) {
	fields, ok := strictObject(raw, admissionCapabilitiesWireFields)
	if !ok {
		return admissionCapabilitiesWire{}, false
	}
	planAllowed, ok := decodeBool(fields["plan_allowed"])
	if !ok {
		return admissionCapabilitiesWire{}, false
	}
	readOnlyToolsAllowed, ok := decodeBool(fields["read_only_tools_allowed"])
	if !ok {
		return admissionCapabilitiesWire{}, false
	}
	sandboxWritesAllowed, ok := decodeBool(fields["sandbox_writes_allowed"])
	if !ok {
		return admissionCapabilitiesWire{}, false
	}
	humanInteractionAllowed, ok := decodeBool(fields["human_interaction_allowed"])
	if !ok {
		return admissionCapabilitiesWire{}, false
	}
	subagentsAllowed, ok := decodeBool(fields["subagents_allowed"])
	if !ok {
		return admissionCapabilitiesWire{}, false
	}
	return admissionCapabilitiesWire{
		PlanAllowed:             planAllowed,
		ReadOnlyToolsAllowed:    readOnlyToolsAllowed,
		SandboxWritesAllowed:    sandboxWritesAllowed,
		HumanInteractionAllowed: humanInteractionAllowed,
		SubagentsAllowed:        subagentsAllowed,
	}, true
}

func decodeAdmissionLimits(raw json.RawMessage) (admissionLimitsWire, bool) {
	fields, ok := strictObject(raw, admissionLimitsWireFields)
	if !ok {
		return admissionLimitsWire{}, false
	}
	maxToolCalls, ok := decodeInt(fields["max_tool_calls"])
	if !ok {
		return admissionLimitsWire{}, false
	}
	maxReplans, ok := decodeInt(fields["max_replans"])
	if !ok {
		return admissionLimitsWire{}, false
	}
	maxVerificationRepairs, ok := decodeInt(fields["max_verification_repairs"])
	if !ok {
		return admissionLimitsWire{}, false
	}
	maxConsecutiveNoProgress, ok := decodeInt(fields["max_consecutive_no_progress"])
	if !ok {
		return admissionLimitsWire{}, false
	}
	maxActiveDurationSeconds, ok := decodeInt(fields["max_active_duration_seconds"])
	if !ok {
		return admissionLimitsWire{}, false
	}
	return admissionLimitsWire{
		MaxToolCalls:             maxToolCalls,
		MaxReplans:               maxReplans,
		MaxVerificationRepairs:   maxVerificationRepairs,
		MaxConsecutiveNoProgress: maxConsecutiveNoProgress,
		MaxActiveDurationSeconds: maxActiveDurationSeconds,
	}, true
}

func decodeExecutionDecisionWire(raw []byte) (executionDecisionWire, bool) {
	fields, ok := strictObject(raw, executionDecisionWireFields)
	if !ok {
		return executionDecisionWire{}, false
	}
	deliverables, ok := decodeDeliverables(fields["deliverables"])
	if !ok {
		return executionDecisionWire{}, false
	}
	checks, ok := decodeAcceptanceChecks(fields["acceptance_checks"])
	if !ok {
		return executionDecisionWire{}, false
	}
	schema, ok := decodeString(fields["schema"])
	if !ok {
		return executionDecisionWire{}, false
	}
	decisionID, ok := decodeString(fields["decision_id"])
	if !ok {
		return executionDecisionWire{}, false
	}
	decisionRevision, ok := decodeUint64(fields["decision_revision"])
	if !ok {
		return executionDecisionWire{}, false
	}
	executionRunID, ok := decodeInt64(fields["execution_run_id"])
	if !ok {
		return executionDecisionWire{}, false
	}
	journalRunID, ok := decodeInt64(fields["journal_run_id"])
	if !ok {
		return executionDecisionWire{}, false
	}
	attemptID, ok := decodeString(fields["attempt_id"])
	if !ok {
		return executionDecisionWire{}, false
	}
	executionGeneration, ok := decodeUint64(fields["execution_generation"])
	if !ok {
		return executionDecisionWire{}, false
	}
	planScopeRunID, ok := decodeNullableInt64(fields["plan_scope_run_id"])
	if !ok {
		return executionDecisionWire{}, false
	}
	goalSummary, ok := decodeString(fields["goal_summary"])
	if !ok {
		return executionDecisionWire{}, false
	}
	decisionKind, ok := decodeString(fields["decision"])
	if !ok {
		return executionDecisionWire{}, false
	}
	executionShape, ok := decodeString(fields["execution_shape"])
	if !ok {
		return executionDecisionWire{}, false
	}
	clarificationQuestion, ok := decodeNullableString(fields["clarification_question"])
	if !ok {
		return executionDecisionWire{}, false
	}
	safeSummary, ok := decodeString(fields["safe_summary"])
	if !ok {
		return executionDecisionWire{}, false
	}
	createdAt, ok := decodeInt64(fields["created_at"])
	if !ok {
		return executionDecisionWire{}, false
	}
	return executionDecisionWire{
		Schema:                schema,
		DecisionID:            decisionID,
		DecisionRevision:      decisionRevision,
		ExecutionRunID:        executionRunID,
		JournalRunID:          journalRunID,
		AttemptID:             attemptID,
		ExecutionGeneration:   executionGeneration,
		PlanScopeRunID:        planScopeRunID,
		GoalSummary:           goalSummary,
		Deliverables:          deliverables,
		AcceptanceChecks:      checks,
		Decision:              entity.ExecutionDecisionKind(decisionKind),
		ExecutionShape:        entity.ExecutionShape(executionShape),
		ClarificationQuestion: clarificationQuestion,
		SafeSummary:           safeSummary,
		CreatedAt:             createdAt,
	}, true
}

func decodeDeliverables(raw json.RawMessage) ([]string, bool) {
	values, ok := strictArray(raw)
	if !ok {
		return nil, false
	}
	deliverables := make([]string, len(values))
	for index, value := range values {
		deliverable, ok := decodeString(value)
		if !ok {
			return nil, false
		}
		deliverables[index] = deliverable
	}
	return deliverables, true
}

func decodeAcceptanceChecks(raw json.RawMessage) ([]acceptanceCheckWire, bool) {
	values, ok := strictArray(raw)
	if !ok {
		return nil, false
	}
	checks := make([]acceptanceCheckWire, len(values))
	for index, value := range values {
		fields, ok := strictObject(value, acceptanceCheckWireFields)
		if !ok {
			return nil, false
		}
		checkID, ok := decodeString(fields["check_id"])
		if !ok {
			return nil, false
		}
		kind, ok := decodeString(fields["kind"])
		if !ok {
			return nil, false
		}
		targetRef, ok := decodeString(fields["target_ref"])
		if !ok {
			return nil, false
		}
		safeDescription, ok := decodeString(fields["safe_description"])
		if !ok {
			return nil, false
		}
		checks[index] = acceptanceCheckWire{
			CheckID:         checkID,
			Kind:            kind,
			TargetRef:       targetRef,
			SafeDescription: safeDescription,
		}
	}
	return checks, true
}

func strictObject(raw []byte, expected []string) (map[string]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, false
	}
	expectedSet := make(map[string]struct{}, len(expected))
	for _, key := range expected {
		expectedSet[key] = struct{}{}
	}
	fields := make(map[string]json.RawMessage, len(expected))
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, ok := keyToken.(string)
		if err != nil || !ok {
			return nil, false
		}
		if _, known := expectedSet[key]; !known {
			return nil, false
		}
		if _, duplicate := fields[key]; duplicate {
			return nil, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
		fields[key] = append(json.RawMessage(nil), value...)
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim('}') || !strictEOF(decoder) || len(fields) != len(expected) {
		return nil, false
	}
	for _, key := range expected {
		if _, present := fields[key]; !present {
			return nil, false
		}
	}
	return fields, true
}

func strictArray(raw []byte) ([]json.RawMessage, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil || token != json.Delim('[') {
		return nil, false
	}
	values := make([]json.RawMessage, 0)
	for decoder.More() {
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
		values = append(values, append(json.RawMessage(nil), value...))
	}
	end, err := decoder.Token()
	if err != nil || end != json.Delim(']') || !strictEOF(decoder) {
		return nil, false
	}
	return values, true
}

func strictValue(raw []byte) (any, bool) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil || !strictEOF(decoder) {
		return nil, false
	}
	return value, true
}

func strictEOF(decoder *json.Decoder) bool {
	var extra json.RawMessage
	return decoder.Decode(&extra) == io.EOF
}

// validJSONStrings rejects JSON strings whose escaped UTF-16 code units cannot
// round-trip to the decoded Go string. encoding/json replaces unpaired
// surrogates with U+FFFD, so this check must run before decoding.
func validJSONStrings(raw []byte) bool {
	for index := 0; index < len(raw); {
		if raw[index] != '"' {
			index++
			continue
		}
		index++
		closed := false
		for index < len(raw) {
			value := raw[index]
			index++
			switch value {
			case '"':
				closed = true
			case '\\':
				if index >= len(raw) {
					return false
				}
				escape := raw[index]
				index++
				switch escape {
				case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				case 'u':
					codeUnit, ok := decodeJSONHexCodeUnit(raw, index)
					if !ok {
						return false
					}
					index += 4
					if codeUnit >= 0xd800 && codeUnit <= 0xdbff {
						if index+2 > len(raw) || raw[index] != '\\' || raw[index+1] != 'u' {
							return false
						}
						lowSurrogate, ok := decodeJSONHexCodeUnit(raw, index+2)
						if !ok || lowSurrogate < 0xdc00 || lowSurrogate > 0xdfff {
							return false
						}
						index += 6
					} else if codeUnit >= 0xdc00 && codeUnit <= 0xdfff {
						return false
					}
				default:
					return false
				}
			default:
				if value < 0x20 {
					return false
				}
			}
			if closed {
				break
			}
		}
		if !closed {
			return false
		}
	}
	return true
}

func decodeJSONHexCodeUnit(raw []byte, start int) (uint16, bool) {
	if start+4 > len(raw) {
		return 0, false
	}
	var value uint16
	for _, digit := range raw[start : start+4] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value += uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value += uint16(digit-'a') + 10
		case digit >= 'A' && digit <= 'F':
			value += uint16(digit-'A') + 10
		default:
			return 0, false
		}
	}
	return value, true
}

func decodeString(raw []byte) (string, bool) {
	value, ok := strictValue(raw)
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func decodeNullableString(raw []byte) (*string, bool) {
	value, ok := strictValue(raw)
	if !ok {
		return nil, false
	}
	if value == nil {
		return nil, true
	}
	text, ok := value.(string)
	if !ok {
		return nil, false
	}
	return &text, true
}

func decodeBool(raw []byte) (bool, bool) {
	value, ok := strictValue(raw)
	if !ok {
		return false, false
	}
	boolean, ok := value.(bool)
	return boolean, ok
}

func decodeInt(raw []byte) (int, bool) {
	number, ok := decodeNumber(raw)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseInt(number.String(), 10, strconv.IntSize)
	return int(value), err == nil
}

func decodeInt64(raw []byte) (int64, bool) {
	number, ok := decodeNumber(raw)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseInt(number.String(), 10, 64)
	return value, err == nil
}

func decodeNullableInt64(raw []byte) (*int64, bool) {
	value, ok := strictValue(raw)
	if !ok {
		return nil, false
	}
	if value == nil {
		return nil, true
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, false
	}
	parsed, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil {
		return nil, false
	}
	return &parsed, true
}

func decodeUint64(raw []byte) (uint64, bool) {
	number, ok := decodeNumber(raw)
	if !ok {
		return 0, false
	}
	value, err := strconv.ParseUint(number.String(), 10, 64)
	return value, err == nil
}

func decodeNullableUint64(raw []byte) (*uint64, bool) {
	value, ok := strictValue(raw)
	if !ok {
		return nil, false
	}
	if value == nil {
		return nil, true
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, false
	}
	parsed, err := strconv.ParseUint(number.String(), 10, 64)
	if err != nil {
		return nil, false
	}
	return &parsed, true
}

func decodeNumber(raw []byte) (json.Number, bool) {
	value, ok := strictValue(raw)
	if !ok {
		return "", false
	}
	number, ok := value.(json.Number)
	return number, ok
}

func digestCanonical(canonical []byte) string {
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:])
}

func copyInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyUint64Pointer(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func copyStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
