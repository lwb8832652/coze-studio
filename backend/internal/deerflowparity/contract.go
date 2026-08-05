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

package deerflowparity

const (
	ContractSchemaV1  = "newx.deerflow.agent.acceptance.v1"
	ScopeSemanticCore = "semantic_core"
)

type Mode string

const (
	ModePro   Mode = "pro"
	ModeUltra Mode = "ultra"
)

type ActionType string

const (
	ActionRun         ActionType = "run"
	ActionReloadState ActionType = "reload_state"
	ActionFollowUp    ActionType = "follow_up"
	ActionCancel      ActionType = "cancel"
	ActionReconnect   ActionType = "reconnect"
)

type Suite struct {
	Schema           string `json:"schema"`
	Scope            string `json:"scope"`
	DeerFlowRevision string `json:"deerflow_revision"`
	Cases            []Case `json:"cases"`
}

func (s Suite) CaseIDs() []string {
	ids := make([]string, 0, len(s.Cases))
	for _, testCase := range s.Cases {
		ids = append(ids, testCase.ID)
	}
	return ids
}

type Case struct {
	ID          string      `json:"id"`
	Mode        Mode        `json:"mode"`
	InputPrompt string      `json:"input_prompt"`
	Actions     []Action    `json:"actions"`
	Expect      Expectation `json:"expect"`
}

type Action struct {
	Type        ActionType `json:"type"`
	Answer      string     `json:"answer,omitempty"`
	AfterEvent  string     `json:"after_event,omitempty"`
	AfterEvents int        `json:"after_events,omitempty"`
}

type Expectation struct {
	Capabilities             CapabilityExpectation    `json:"capabilities"`
	AssistantMessageRequired bool                     `json:"assistant_message_required,omitempty"`
	TokenUsageRequired       bool                     `json:"token_usage_required,omitempty"`
	Todo                     TodoExpectation          `json:"todo"`
	Children                 ChildrenExpectation      `json:"children"`
	Clarification            ClarificationExpectation `json:"clarification"`
	NoSuccessAfterCancel     bool                     `json:"no_success_after_cancel,omitempty"`
	ReconnectDeduplicated    bool                     `json:"reconnect_deduplicated,omitempty"`
	RequiredTerminal         []string                 `json:"required_terminal"`
	RequiredEvents           []string                 `json:"required_events"`
	EventOrder               [][]string               `json:"event_order"`
}

type CapabilityExpectation struct {
	Thinking *bool `json:"thinking,omitempty"`
	Plan     *bool `json:"plan,omitempty"`
	Subagent *bool `json:"subagent,omitempty"`
}

type TodoExpectation struct {
	Required     bool `json:"required,omitempty"`
	AllCompleted bool `json:"all_completed,omitempty"`
}

type ChildrenExpectation struct {
	Minimum int `json:"minimum,omitempty"`
}

type ClarificationExpectation struct {
	Required         bool `json:"required,omitempty"`
	FollowUpRequired bool `json:"follow_up_required,omitempty"`
}

const ObservationSchemaV1 = "newx.deerflow.agent.observation.v1"

type Product string

const (
	ProductDeerFlow Product = "deerflow"
	ProductNewX     Product = "newx"
)

type CapabilityState struct {
	Thinking bool `json:"thinking"`
	Plan     bool `json:"plan"`
	Subagent bool `json:"subagent"`
}

type RawCapture struct {
	Product                  Product
	CaseID                   string
	Mode                     Mode
	Events                   []RawEvent
	Messages                 []RawMessage
	State                    map[string]any
	Tokens                   []RawTokenUsage
	Terminal                 string
	Reconnected              bool
	StreamTerminalObserved   bool
	ReconnectDuplicateEvents int
	RunCount                 int
	CancelRequested          bool
	StateReloaded            bool
	HistoryEntries           int
}

type RawEvent struct {
	ID      string
	Type    string
	Payload map[string]any
}

type RawMessage struct {
	Role             string
	Content          string
	ReasoningPresent bool
}

type RawTokenUsage struct {
	Input  int64
	Output int64
	Total  int64
}

type Observation struct {
	Schema                  string           `json:"schema"`
	Product                 Product          `json:"product"`
	CaseID                  string           `json:"case_id"`
	Mode                    Mode             `json:"mode"`
	Capabilities            CapabilityState  `json:"capabilities"`
	EventFamilies           []string         `json:"event_families"`
	AssistantMessagePresent bool             `json:"assistant_message_present"`
	AssistantMessageBytes   int              `json:"assistant_message_bytes"`
	Todo                    TodoObservation  `json:"todo"`
	ChildRuns               int              `json:"child_runs"`
	CompletedChildRuns      int              `json:"completed_child_runs"`
	InterruptPresent        bool             `json:"interrupt_present"`
	ResumeObserved          bool             `json:"resume_observed"`
	ClarificationPresent    bool             `json:"clarification_present"`
	FollowUpObserved        bool             `json:"follow_up_observed"`
	Token                   TokenObservation `json:"token"`
	Terminal                string           `json:"terminal"`
	Reconnected             bool             `json:"reconnected"`
	StreamTerminalObserved  bool             `json:"stream_terminal_observed"`
	DuplicateEvents         int              `json:"duplicate_events"`
	RunCount                int              `json:"run_count"`
	TerminalEvents          int              `json:"terminal_events"`
	StateReloaded           bool             `json:"state_reloaded"`
	HistoryEntries          int              `json:"history_entries"`
	SuccessAfterCancel      bool             `json:"success_after_cancel"`
	Blocker                 string           `json:"blocker,omitempty"`
}

type TodoObservation struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Completed  int `json:"completed"`
	Other      int `json:"other"`
}

type TokenObservation struct {
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	Total  int64 `json:"total"`
}

type ComparisonStatus string

const (
	StatusAligned   ComparisonStatus = "aligned"
	StatusStronger  ComparisonStatus = "stronger"
	StatusDifferent ComparisonStatus = "different"
	StatusBlocked   ComparisonStatus = "blocked"
)

type ComparisonResult struct {
	Schema  string            `json:"schema"`
	CaseID  string            `json:"case_id"`
	Status  ComparisonStatus  `json:"status"`
	Blocker string            `json:"blocker,omitempty"`
	Checks  []ComparisonCheck `json:"checks"`
}

type ComparisonCheck struct {
	Name     string `json:"name"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
}

func SemanticCoreCaseIDs() []string {
	return []string{
		"core.pro.direct",
		"core.ultra.direct",
		"core.pro.todo",
		"core.ultra.subagents",
		"core.clarify.followup",
		"core.cancel",
		"core.stream.reconnect",
	}
}
