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
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestEncodeAdaptiveAdmissionUsesFixedCanonicalBytesAndDigest(t *testing.T) {
	canonical, digest, err := EncodeAdaptiveAdmission(contractAdmission(entity.AdaptiveAdmissionSourceFresh))
	if err != nil {
		t.Fatalf("EncodeAdaptiveAdmission: %v", err)
	}
	expected := []byte(`{"schema":"workbench-adaptive-admission.v1","feature_gate_enabled":true,"source":"fresh","source_run_id":null,"source_execution_generation":null,"source_config_digest":"","decoder_version":"","capabilities":{"plan_allowed":true,"read_only_tools_allowed":false,"sandbox_writes_allowed":false,"human_interaction_allowed":true,"subagents_allowed":false},"limits":{"max_tool_calls":24,"max_replans":2,"max_verification_repairs":2,"max_consecutive_no_progress":3,"max_active_duration_seconds":1200}}`)
	if !bytes.Equal(canonical, expected) {
		t.Fatalf("canonical = %s", canonical)
	}
	const expectedDigest = "6ee23e53bb69458cda6b1af0342d259f77c782ae90ec43766017068380cfeed0"
	if digest != expectedDigest {
		t.Fatalf("digest = %q", digest)
	}
}

func TestEncodeExecutionDecisionUsesFixedCanonicalBytesAndDigest(t *testing.T) {
	canonical, digest, err := EncodeExecutionDecision(contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty))
	if err != nil {
		t.Fatalf("EncodeExecutionDecision: %v", err)
	}
	expected := []byte(`{"schema":"workbench-adaptive-decision.v1","decision_id":"decision-1","decision_revision":1,"execution_run_id":10,"journal_run_id":11,"attempt_id":"attempt-1","execution_generation":1,"plan_scope_run_id":null,"goal_summary":"deliver goal","deliverables":["deliverable"],"acceptance_checks":[{"check_id":"check-1","kind":"assertion","target_ref":"artifact-1","safe_description":"verify output"}],"decision":"direct","execution_shape":"","clarification_question":null,"safe_summary":"safe summary","created_at":1}`)
	if !bytes.Equal(canonical, expected) {
		t.Fatalf("canonical = %s", canonical)
	}
	const expectedDigest = "4f10d28f41dd2cc3730abb692bc62257d610ef5fb020be51e341c6c7c100359c"
	if digest != expectedDigest {
		t.Fatalf("digest = %q", digest)
	}
}

func TestAdaptiveCodecsRoundTripAndDoNotAlias(t *testing.T) {
	admission := contractAdmission(entity.AdaptiveAdmissionSourceTypedInheritance)
	admissionCanonical, admissionDigest, err := EncodeAdaptiveAdmission(admission)
	if err != nil {
		t.Fatalf("EncodeAdaptiveAdmission: %v", err)
	}
	decodedAdmission, decodedAdmissionCanonical, decodedAdmissionDigest, err := DecodeAdaptiveAdmission(admissionCanonical)
	if err != nil {
		t.Fatalf("DecodeAdaptiveAdmission: %v", err)
	}
	if !reflect.DeepEqual(decodedAdmission, admission) || !bytes.Equal(decodedAdmissionCanonical, admissionCanonical) || decodedAdmissionDigest != admissionDigest {
		t.Fatal("admission did not preserve canonical round trip")
	}
	if decodedAdmission.SourceRunID == admission.SourceRunID || decodedAdmission.SourceExecutionGeneration == admission.SourceExecutionGeneration {
		t.Fatal("decoded admission aliases input pointers")
	}

	decision := contractDecision(entity.ExecutionDecisionClarification, entity.ExecutionShapeEmpty)
	decisionCanonical, decisionDigest, err := EncodeExecutionDecision(decision)
	if err != nil {
		t.Fatalf("EncodeExecutionDecision: %v", err)
	}
	decodedDecision, decodedDecisionCanonical, decodedDecisionDigest, err := DecodeExecutionDecision(decisionCanonical)
	if err != nil {
		t.Fatalf("DecodeExecutionDecision: %v", err)
	}
	if decodedDecision.Schema != decision.Schema || !bytes.Equal(decodedDecisionCanonical, decisionCanonical) || decodedDecisionDigest != decisionDigest {
		t.Fatal("decision did not preserve canonical round trip")
	}
	if &decodedDecision.Deliverables[0] == &decision.Deliverables[0] || &decodedDecision.AcceptanceChecks[0] == &decision.AcceptanceChecks[0] || decodedDecision.ClarificationQuestion == decision.ClarificationQuestion {
		t.Fatal("decoded decision aliases input")
	}
	decodedDecision.Deliverables[0] = "changed"
	if decision.Deliverables[0] != "deliverable" {
		t.Fatal("decoded decision mutation changed input")
	}
}

func TestAdaptiveAdmissionCodecRoundTripsAllSources(t *testing.T) {
	cases := []struct {
		name      string
		admission entity.AdaptiveAdmissionSnapshot
	}{
		{name: "fresh", admission: contractAdmission(entity.AdaptiveAdmissionSourceFresh)},
		{name: "typed_inheritance", admission: contractAdmission(entity.AdaptiveAdmissionSourceTypedInheritance)},
		{name: "legacy_decoder", admission: contractAdmission(entity.AdaptiveAdmissionSourceLegacyDecoder)},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			canonical, digest, err := EncodeAdaptiveAdmission(testCase.admission)
			if err != nil {
				t.Fatalf("EncodeAdaptiveAdmission: %v", err)
			}
			decoded, decodedCanonical, decodedDigest, err := DecodeAdaptiveAdmission(canonical)
			if err != nil {
				t.Fatalf("DecodeAdaptiveAdmission: %v", err)
			}
			reencoded, redigest, err := EncodeAdaptiveAdmission(decoded)
			if err != nil {
				t.Fatalf("re-EncodeAdaptiveAdmission: %v", err)
			}

			if !reflect.DeepEqual(decoded, testCase.admission) {
				t.Fatalf("decoded admission = %#v, want %#v", decoded, testCase.admission)
			}
			if !bytes.Equal(decodedCanonical, canonical) || !bytes.Equal(reencoded, canonical) || decodedDigest != digest || redigest != digest {
				t.Fatalf("admission canonical/digest round trip drifted: canonical=%s decoded=%s reencoded=%s digest=%s decoded_digest=%s redigest=%s", canonical, decodedCanonical, reencoded, digest, decodedDigest, redigest)
			}
			if testCase.admission.Source == entity.AdaptiveAdmissionSourceFresh {
				if decoded.SourceRunID != nil || decoded.SourceExecutionGeneration != nil {
					t.Fatal("fresh source lineage nulls were not preserved")
				}
				return
			}
			if decoded.SourceRunID == nil || decoded.SourceExecutionGeneration == nil ||
				*decoded.SourceRunID != *testCase.admission.SourceRunID ||
				*decoded.SourceExecutionGeneration != *testCase.admission.SourceExecutionGeneration {
				t.Fatal("non-fresh source lineage values were not preserved")
			}
			if decoded.SourceRunID == testCase.admission.SourceRunID || decoded.SourceExecutionGeneration == testCase.admission.SourceExecutionGeneration {
				t.Fatal("decoded admission lineage pointers alias the input")
			}
		})
	}
}

func TestExecutionDecisionCodecRoundTripsAllForms(t *testing.T) {
	cases := []struct {
		name     string
		decision entity.ExecutionDecision
	}{
		{name: "clarification", decision: contractDecision(entity.ExecutionDecisionClarification, entity.ExecutionShapeEmpty)},
		{name: "direct", decision: contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)},
		{name: "execute_single_step", decision: contractDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeSingleStep)},
		{name: "execute_multi_step", decision: contractDecision(entity.ExecutionDecisionExecute, entity.ExecutionShapeMultiStep)},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			canonical, digest, err := EncodeExecutionDecision(testCase.decision)
			if err != nil {
				t.Fatalf("EncodeExecutionDecision: %v", err)
			}
			decoded, decodedCanonical, decodedDigest, err := DecodeExecutionDecision(canonical)
			if err != nil {
				t.Fatalf("DecodeExecutionDecision: %v", err)
			}
			reencoded, redigest, err := EncodeExecutionDecision(decoded)
			if err != nil {
				t.Fatalf("re-EncodeExecutionDecision: %v", err)
			}

			if !reflect.DeepEqual(decoded, testCase.decision) {
				t.Fatalf("decoded decision = %#v, want %#v", decoded, testCase.decision)
			}
			if !bytes.Equal(decodedCanonical, canonical) || !bytes.Equal(reencoded, canonical) || decodedDigest != digest || redigest != digest {
				t.Fatalf("decision canonical/digest round trip drifted: canonical=%s decoded=%s reencoded=%s digest=%s decoded_digest=%s redigest=%s", canonical, decodedCanonical, reencoded, digest, decodedDigest, redigest)
			}

			switch testCase.decision.Decision {
			case entity.ExecutionDecisionClarification:
				if decoded.PlanScopeRunID != nil || decoded.ClarificationQuestion == nil ||
					decoded.ClarificationQuestion == testCase.decision.ClarificationQuestion {
					t.Fatal("clarification null/value fields were not preserved independently")
				}
			case entity.ExecutionDecisionExecute:
				if testCase.decision.ExecutionShape == entity.ExecutionShapeMultiStep {
					if decoded.PlanScopeRunID == nil || decoded.PlanScopeRunID == testCase.decision.PlanScopeRunID || decoded.ClarificationQuestion != nil {
						t.Fatal("multi-step null/value fields were not preserved independently")
					}
					return
				}
				fallthrough
			case entity.ExecutionDecisionDirect:
				if decoded.PlanScopeRunID != nil || decoded.ClarificationQuestion != nil {
					t.Fatal("direct or single-step null fields were not preserved")
				}
			}
		})
	}
}

func TestAdaptiveDecisionCodecPreservesEmptyArraysAsArrays(t *testing.T) {
	decision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
	decision.Deliverables = []string{}
	decision.AcceptanceChecks = []entity.AdaptiveAcceptanceCheck{}

	canonical, _, err := EncodeExecutionDecision(decision)
	if err != nil {
		t.Fatalf("EncodeExecutionDecision: %v", err)
	}
	if !bytes.Contains(canonical, []byte(`"deliverables":[]`)) || !bytes.Contains(canonical, []byte(`"acceptance_checks":[]`)) {
		t.Fatalf("empty arrays were not encoded as arrays: %s", canonical)
	}
	decoded, _, _, err := DecodeExecutionDecision(canonical)
	if err != nil {
		t.Fatalf("DecodeExecutionDecision: %v", err)
	}
	if decoded.Deliverables == nil || decoded.AcceptanceChecks == nil || len(decoded.Deliverables) != 0 || len(decoded.AcceptanceChecks) != 0 {
		t.Fatal("empty arrays were not preserved")
	}
}

func TestAdaptiveCodecsRejectLossyUnicodeAndEncoding(t *testing.T) {
	admission, _, err := EncodeAdaptiveAdmission(contractAdmission(entity.AdaptiveAdmissionSourceFresh))
	if err != nil {
		t.Fatal(err)
	}
	decision, _, err := EncodeExecutionDecision(contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty))
	if err != nil {
		t.Fatal(err)
	}
	for name, replace := range map[string]string{
		"unpaired_high_surrogate": `\uD800`,
		"unpaired_low_surrogate":  `\uDC00`,
	} {
		t.Run(name, func(t *testing.T) {
			admissionRaw := []byte(strings.Replace(string(admission), `"source_config_digest":""`, `"source_config_digest":"`+replace+`"`, 1))
			if _, _, _, err := DecodeAdaptiveAdmission(admissionRaw); !errors.Is(err, ErrAdaptiveAdmissionInvalid) {
				t.Fatalf("admission accepted %s: %v", name, err)
			}
			decisionRaw := []byte(strings.Replace(string(decision), `"goal_summary":"deliver goal"`, `"goal_summary":"`+replace+`"`, 1))
			if _, _, _, err := DecodeExecutionDecision(decisionRaw); !errors.Is(err, ErrExecutionDecisionInvalid) {
				t.Fatalf("decision accepted %s: %v", name, err)
			}
		})
	}

	validPair := []byte(strings.Replace(string(decision), `"goal_summary":"deliver goal"`, `"goal_summary":"\\uD83D\\uDE00"`, 1))
	if _, _, _, err := DecodeExecutionDecision(validPair); err != nil {
		t.Fatalf("decision rejected valid surrogate pair: %v", err)
	}
	literalReplacement := []byte(strings.Replace(string(decision), `"goal_summary":"deliver goal"`, `"goal_summary":"�"`, 1))
	if _, _, _, err := DecodeExecutionDecision(literalReplacement); err != nil {
		t.Fatalf("decision rejected literal replacement rune: %v", err)
	}

	invalidUTF8Admission := bytes.Replace(admission, []byte(`"source":"fresh"`), []byte{'"', 's', 'o', 'u', 'r', 'c', 'e', '"', ':', '"', 'f', 'r', 0xff, 's', 'h', '"'}, 1)
	if _, _, _, err := DecodeAdaptiveAdmission(invalidUTF8Admission); !errors.Is(err, ErrAdaptiveAdmissionInvalid) {
		t.Fatalf("admission accepted invalid UTF-8: %v", err)
	}
	invalidUTF8Decision := bytes.Replace(decision, []byte(`"goal_summary":"deliver goal"`), []byte{'"', 'g', 'o', 'a', 'l', '_', 's', 'u', 'm', 'm', 'a', 'r', 'y', '"', ':', '"', 'd', 'e', 'l', 'i', 'v', 'e', 'r', ' ', 0xff, 'g', 'o', 'a', 'l', '"'}, 1)
	if _, _, _, err := DecodeExecutionDecision(invalidUTF8Decision); !errors.Is(err, ErrExecutionDecisionInvalid) {
		t.Fatalf("decision accepted invalid UTF-8: %v", err)
	}

	invalidAdmission := contractAdmission(entity.AdaptiveAdmissionSourceFresh)
	invalidAdmission.SourceConfigDigest = string([]byte{0xff})
	if _, _, err := EncodeAdaptiveAdmission(invalidAdmission); !errors.Is(err, ErrAdaptiveAdmissionInvalid) {
		t.Fatalf("admission encode accepted invalid UTF-8: %v", err)
	}
	invalidDecision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
	invalidDecision.GoalSummary = string([]byte{0xff})
	if _, _, err := EncodeExecutionDecision(invalidDecision); !errors.Is(err, ErrExecutionDecisionInvalid) {
		t.Fatalf("decision encode accepted invalid UTF-8: %v", err)
	}
}

func TestAdaptiveCodecsEnforceValidRawAndCanonicalSizeBoundaries(t *testing.T) {
	admission := contractAdmission(entity.AdaptiveAdmissionSourceFresh)
	canonicalAdmission, _, err := EncodeAdaptiveAdmission(admission)
	if err != nil {
		t.Fatal(err)
	}
	decision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
	canonicalDecision, _, err := EncodeExecutionDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	for name, testCase := range map[string]struct {
		canonical []byte
		decode    func([]byte) error
	}{
		"admission": {canonical: canonicalAdmission, decode: func(raw []byte) error { _, _, _, err := DecodeAdaptiveAdmission(raw); return err }},
		"decision":  {canonical: canonicalDecision, decode: func(raw []byte) error { _, _, _, err := DecodeExecutionDecision(raw); return err }},
	} {
		t.Run(name, func(t *testing.T) {
			atLimit := append(append([]byte(nil), testCase.canonical...), bytes.Repeat([]byte(" "), maxAdaptiveContractBytes-len(testCase.canonical))...)
			if len(atLimit) != maxAdaptiveContractBytes {
				t.Fatalf("at-limit size = %d", len(atLimit))
			}
			if err := testCase.decode(atLimit); err != nil {
				t.Fatalf("valid %d-byte JSON rejected: %v", len(atLimit), err)
			}
			overLimit := append(append([]byte(nil), atLimit...), ' ')
			if err := testCase.decode(overLimit); err == nil {
				t.Fatalf("valid %d-byte JSON accepted", len(overLimit))
			}
		})
	}

	largeDecision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
	largeDecision.GoalSummary = strings.Repeat(`"`, 1024)
	largeDecision.SafeSummary = strings.Repeat(`"`, 1024)
	largeDecision.Deliverables = make([]string, 16)
	for index := range largeDecision.Deliverables {
		largeDecision.Deliverables[index] = strings.Repeat(`"`, 512)
	}
	largeDecision.AcceptanceChecks = make([]entity.AdaptiveAcceptanceCheck, 32)
	for index := range largeDecision.AcceptanceChecks {
		largeDecision.AcceptanceChecks[index] = entity.AdaptiveAcceptanceCheck{
			CheckID: strings.Repeat("a", 191), Kind: strings.Repeat("b", 64), TargetRef: strings.Repeat("c", 191), SafeDescription: strings.Repeat(`"`, 512),
		}
	}
	if err := ValidateExecutionDecision(largeDecision); err != nil {
		t.Fatalf("large decision must be otherwise valid: %v", err)
	}
	if _, _, err := EncodeExecutionDecision(largeDecision); err == nil {
		t.Fatal("canonical decision above 64 KiB was accepted")
	}
}

func TestEncodeExecutionDecisionEnforcesExactCanonicalSizeBoundary(t *testing.T) {
	atLimit := decisionWithCanonicalSize(t, maxAdaptiveContractBytes)
	canonical, _, err := EncodeExecutionDecision(atLimit)
	if err != nil {
		t.Fatalf("EncodeExecutionDecision(%d bytes): %v", maxAdaptiveContractBytes, err)
	}
	if len(canonical) != maxAdaptiveContractBytes {
		t.Fatalf("canonical size = %d, want %d", len(canonical), maxAdaptiveContractBytes)
	}

	oneByteOver := atLimit
	oneByteOver.GoalSummary += "x"
	if err := ValidateExecutionDecision(oneByteOver); err != nil {
		t.Fatalf("one-byte-over decision must otherwise be valid: %v", err)
	}
	uncappedCanonical, err := json.Marshal(decisionToWire(oneByteOver))
	if err != nil {
		t.Fatalf("marshal one-byte-over decision: %v", err)
	}
	if len(uncappedCanonical) != maxAdaptiveContractBytes+1 {
		t.Fatalf("one-byte-over canonical size = %d, want %d", len(uncappedCanonical), maxAdaptiveContractBytes+1)
	}
	if _, _, err := EncodeExecutionDecision(oneByteOver); !errors.Is(err, ErrExecutionDecisionInvalid) {
		t.Fatalf("expected ErrExecutionDecisionInvalid for %d-byte canonical JSON, got %v", len(uncappedCanonical), err)
	}
}

func decisionWithCanonicalSize(t *testing.T, targetSize int) entity.ExecutionDecision {
	t.Helper()

	// Vary the fixed serialized overhead instead of assuming a padding length.
	// Each candidate has only valid, non-sensitive values; the goal summary is
	// then padded from the actual encoded overhead.
	for checkCount := 0; checkCount <= 32; checkCount++ {
		decision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
		decision.DecisionID = strings.Repeat("d", 191)
		decision.AttemptID = strings.Repeat("a", 191)
		decision.SafeSummary = strings.Repeat(`"`, 1024)
		decision.Deliverables = make([]string, 16)
		for index := range decision.Deliverables {
			decision.Deliverables[index] = strings.Repeat(`"`, 512)
		}
		decision.AcceptanceChecks = make([]entity.AdaptiveAcceptanceCheck, checkCount)
		for index := range decision.AcceptanceChecks {
			decision.AcceptanceChecks[index] = entity.AdaptiveAcceptanceCheck{
				CheckID:         strings.Repeat("c", 191),
				Kind:            strings.Repeat("k", 64),
				TargetRef:       strings.Repeat("t", 191),
				SafeDescription: strings.Repeat(`"`, 512),
			}
		}

		decision.GoalSummary = "x"
		canonical, err := json.Marshal(decisionToWire(decision))
		if err != nil {
			t.Fatalf("marshal fixed-size decision: %v", err)
		}
		encodedGoalSummary, err := json.Marshal(decision.GoalSummary)
		if err != nil {
			t.Fatalf("marshal goal summary: %v", err)
		}
		goalSummary, ok := safeGoalSummaryWithEncodedSize(targetSize - (len(canonical) - len(encodedGoalSummary)))
		if !ok {
			continue
		}
		decision.GoalSummary = goalSummary
		if err := ValidateExecutionDecision(decision); err != nil {
			t.Fatalf("exact-size decision must otherwise be valid: %v", err)
		}
		return decision
	}
	t.Fatalf("could not construct a valid decision with canonical size %d", targetSize)
	return entity.ExecutionDecision{}
}

func safeGoalSummaryWithEncodedSize(encodedSize int) (string, bool) {
	const jsonQuoteBytes = 2
	contentBytes := encodedSize - jsonQuoteBytes
	if contentBytes <= 0 {
		return "", false
	}
	escapedQuoteCount := contentBytes / 2
	plainByteCount := contentBytes % 2
	if escapedQuoteCount+plainByteCount > 1024 {
		return "", false
	}
	return strings.Repeat(`"`, escapedQuoteCount) + strings.Repeat("x", plainByteCount), true
}

func TestDecodeAdaptiveAdmissionRejectsStrictWireViolations(t *testing.T) {
	valid, _, err := EncodeAdaptiveAdmission(contractAdmission(entity.AdaptiveAdmissionSourceFresh))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"unknown":      []byte(strings.Replace(string(valid), `"schema":`, `"unknown":true,"schema":`, 1)),
		"duplicate":    []byte(strings.Replace(string(valid), `"schema":`, `"schema":"x","schema":`, 1)),
		"missing":      []byte(strings.Replace(string(valid), `,"decoder_version":""`, ``, 1)),
		"wrong_type":   []byte(strings.Replace(string(valid), `"feature_gate_enabled":true`, `"feature_gate_enabled":"true"`, 1)),
		"non_integral": []byte(strings.Replace(string(valid), `"max_tool_calls":24`, `"max_tool_calls":24.5`, 1)),
		"overflow":     []byte(strings.Replace(string(valid), `"max_tool_calls":24`, `"max_tool_calls":999999999999999999999999`, 1)),
		"trailing":     append(append([]byte(nil), valid...), []byte(` {}`)...),
		"root_null":    []byte(`null`),
		"nested_null":  []byte(strings.Replace(string(valid), `"capabilities":{"plan_allowed":true,"read_only_tools_allowed":false,"sandbox_writes_allowed":false,"human_interaction_allowed":true,"subagents_allowed":false}`, `"capabilities":null`, 1)),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := DecodeAdaptiveAdmission(raw)
			if !errors.Is(err, ErrAdaptiveAdmissionInvalid) {
				t.Fatalf("expected ErrAdaptiveAdmissionInvalid, got %v", err)
			}
		})
	}
}

func TestDecodeAdaptiveAdmissionRejectsNestedObjectUnknownDuplicateAndMissingFields(t *testing.T) {
	valid, _, err := EncodeAdaptiveAdmission(contractAdmission(entity.AdaptiveAdmissionSourceFresh))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		raw  []byte
	}{
		{
			name: "capabilities_unknown",
			raw:  []byte(strings.Replace(string(valid), `"capabilities":{"plan_allowed":true`, `"capabilities":{"unknown":true,"plan_allowed":true`, 1)),
		},
		{
			name: "capabilities_duplicate",
			raw:  []byte(strings.Replace(string(valid), `"plan_allowed":true`, `"plan_allowed":false,"plan_allowed":true`, 1)),
		},
		{
			name: "capabilities_missing",
			raw:  []byte(strings.Replace(string(valid), `,"subagents_allowed":false`, ``, 1)),
		},
		{
			name: "limits_unknown",
			raw:  []byte(strings.Replace(string(valid), `"limits":{"max_tool_calls":24`, `"limits":{"unknown":true,"max_tool_calls":24`, 1)),
		},
		{
			name: "limits_duplicate",
			raw:  []byte(strings.Replace(string(valid), `"max_tool_calls":24`, `"max_tool_calls":1,"max_tool_calls":24`, 1)),
		},
		{
			name: "limits_missing",
			raw:  []byte(strings.Replace(string(valid), `,"max_active_duration_seconds":1200`, ``, 1)),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, _, err := DecodeAdaptiveAdmission(testCase.raw); !errors.Is(err, ErrAdaptiveAdmissionInvalid) {
				t.Fatalf("expected ErrAdaptiveAdmissionInvalid, got %v", err)
			}
		})
	}
}

func TestDecodeExecutionDecisionRejectsStrictWireViolations(t *testing.T) {
	valid, _, err := EncodeExecutionDecision(contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"unknown": []byte(strings.Replace(string(valid), `"schema":`, `"unknown":true,"schema":`, 1)),
		"duplicate_check": []byte(strings.Replace(
			string(valid), `"check_id":"check-1"`, `"check_id":"other","check_id":"check-1"`, 1,
		)),
		"missing":      []byte(strings.Replace(string(valid), `,"safe_summary":"safe summary"`, ``, 1)),
		"wrong_type":   []byte(strings.Replace(string(valid), `"deliverables":["deliverable"]`, `"deliverables":{}`, 1)),
		"non_integral": []byte(strings.Replace(string(valid), `"created_at":1`, `"created_at":1.5`, 1)),
		"overflow":     []byte(strings.Replace(string(valid), `"execution_run_id":10`, `"execution_run_id":999999999999999999999999`, 1)),
		"trailing":     append(append([]byte(nil), valid...), []byte(` null`)...),
		"root_array":   []byte(`[]`),
		"arrays_null":  []byte(strings.Replace(string(valid), `"deliverables":["deliverable"]`, `"deliverables":null`, 1)),
		"checks_null":  []byte(strings.Replace(string(valid), `"acceptance_checks":[{"check_id":"check-1","kind":"assertion","target_ref":"artifact-1","safe_description":"verify output"}]`, `"acceptance_checks":null`, 1)),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, _, err := DecodeExecutionDecision(raw)
			if !errors.Is(err, ErrExecutionDecisionInvalid) {
				t.Fatalf("expected ErrExecutionDecisionInvalid, got %v", err)
			}
		})
	}
}

func TestDecodeExecutionDecisionRejectsAcceptanceCheckUnknownDuplicateAndMissingFields(t *testing.T) {
	valid, _, err := EncodeExecutionDecision(contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		raw  []byte
	}{
		{
			name: "acceptance_check_unknown",
			raw:  []byte(strings.Replace(string(valid), `"acceptance_checks":[{"check_id":"check-1"`, `"acceptance_checks":[{"unknown":true,"check_id":"check-1"`, 1)),
		},
		{
			name: "acceptance_check_duplicate",
			raw:  []byte(strings.Replace(string(valid), `"check_id":"check-1"`, `"check_id":"other","check_id":"check-1"`, 1)),
		},
		{
			name: "acceptance_check_missing",
			raw:  []byte(strings.Replace(string(valid), `,"safe_description":"verify output"`, ``, 1)),
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, _, _, err := DecodeExecutionDecision(testCase.raw); !errors.Is(err, ErrExecutionDecisionInvalid) {
				t.Fatalf("expected ErrExecutionDecisionInvalid, got %v", err)
			}
		})
	}
}

func TestDecodeAdaptiveDecisionPreservesNullAndEmptyDistinctions(t *testing.T) {
	decision := contractDecision(entity.ExecutionDecisionDirect, entity.ExecutionShapeEmpty)
	canonical, _, err := EncodeExecutionDecision(decision)
	if err != nil {
		t.Fatal(err)
	}
	decoded, decodedCanonical, _, err := DecodeExecutionDecision(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.PlanScopeRunID != nil || decoded.ClarificationQuestion != nil || !bytes.Equal(decodedCanonical, canonical) {
		t.Fatal("nullable fields were not preserved as null")
	}
	badNull := []byte(strings.Replace(string(canonical), `"safe_summary":"safe summary"`, `"safe_summary":null`, 1))
	if _, _, _, err := DecodeExecutionDecision(badNull); !errors.Is(err, ErrExecutionDecisionInvalid) {
		t.Fatalf("non-nullable field accepted null: %v", err)
	}
}
