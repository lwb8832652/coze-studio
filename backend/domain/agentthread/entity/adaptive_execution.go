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

package entity

const (
	AdaptiveAdmissionSchemaV1      = "workbench-adaptive-admission.v1"
	ExecutionDecisionSchemaV1      = "workbench-adaptive-decision.v1"
	AdaptiveLegacyDecoderVersionV1 = "workbench-adaptive-legacy-decoder.v1"
)

type AdaptiveAdmissionSource string

const (
	AdaptiveAdmissionSourceFresh            AdaptiveAdmissionSource = "fresh"
	AdaptiveAdmissionSourceTypedInheritance AdaptiveAdmissionSource = "typed_inheritance"
	AdaptiveAdmissionSourceLegacyDecoder    AdaptiveAdmissionSource = "legacy_decoder"
)

type AdaptiveCapabilities struct {
	PlanAllowed             bool
	ReadOnlyToolsAllowed    bool
	SandboxWritesAllowed    bool
	HumanInteractionAllowed bool
	SubagentsAllowed        bool
}

type AdaptiveLimits struct {
	MaxToolCalls             int
	MaxReplans               int
	MaxVerificationRepairs   int
	MaxConsecutiveNoProgress int
	MaxActiveDurationSeconds int
}

type AdaptiveAdmissionSnapshot struct {
	Schema                    string
	FeatureGateEnabled        bool
	Source                    AdaptiveAdmissionSource
	SourceRunID               *int64
	SourceExecutionGeneration *uint64
	SourceConfigDigest        string
	DecoderVersion            string
	Capabilities              AdaptiveCapabilities
	Limits                    AdaptiveLimits
}

type ExecutionDecisionKind string

const (
	ExecutionDecisionClarification ExecutionDecisionKind = "clarification"
	ExecutionDecisionDirect        ExecutionDecisionKind = "direct"
	ExecutionDecisionExecute       ExecutionDecisionKind = "execute"
)

type ExecutionShape string

const (
	ExecutionShapeEmpty      ExecutionShape = ExecutionShape("")
	ExecutionShapeSingleStep ExecutionShape = "single_step"
	ExecutionShapeMultiStep  ExecutionShape = "multi_step"
)

type AdaptiveAcceptanceCheck struct {
	CheckID         string
	Kind            string
	TargetRef       string
	SafeDescription string
}

type ExecutionDecision struct {
	Schema                string
	DecisionID            string
	DecisionRevision      uint64
	ExecutionRunID        int64
	JournalRunID          int64
	AttemptID             string
	ExecutionGeneration   uint64
	PlanScopeRunID        *int64
	GoalSummary           string
	Deliverables          []string
	AcceptanceChecks      []AdaptiveAcceptanceCheck
	Decision              ExecutionDecisionKind
	ExecutionShape        ExecutionShape
	ClarificationQuestion *string
	SafeSummary           string
	CreatedAt             int64
}
