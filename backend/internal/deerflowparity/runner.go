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

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const SuiteReportSchemaV1 = "newx.deerflow.agent.acceptance-report.v1"
const RevisionProvenanceVerifiedSourceAttestedRuntime = "verified_source_checkout_operator_attested_runtime"

var ErrInvalidBlocker = errors.New("invalid prerequisite blocker")

type PrerequisiteError struct {
	Code    string
	Product Product
}

func (e *PrerequisiteError) Error() string {
	return "acceptance prerequisite is unavailable: " + e.Code
}

func NewPrerequisiteError(code string) error {
	if safeBlocker(code) == "" {
		return fmt.Errorf("%w: code rejected", ErrInvalidBlocker)
	}
	return &PrerequisiteError{Code: code}
}

type PlatformClient interface {
	Product() Product
	Login(context.Context, Credentials) error
	CreateThread(context.Context, ThreadOptions) (string, error)
	StreamRun(context.Context, string, RunInput) (StreamResult, error)
	StartRun(context.Context, string, RunInput) (RunHandle, error)
	StreamExistingRun(context.Context, string, string, StreamOptions) (StreamResult, error)
	GetRun(context.Context, string, string) (RunHandle, error)
	CancelRun(context.Context, string, string) error
	FollowUpRun(context.Context, string, RunInput, string) (StreamResult, error)
	GetThreadState(context.Context, string) (map[string]any, error)
	GetThreadHistory(context.Context, string, int) ([]map[string]any, error)
	ListRunMessages(context.Context, string, string, PageRequest) (MessagePage, error)
	ListRunEvents(context.Context, string, string, int) ([]map[string]any, error)
	WaitForEvent(context.Context, string, string, string) error
}

type RunnerOptions struct {
	Credentials          Credentials
	NewXSpaceID          string
	DeerFlowRevision     string
	RevisionProvenance   string
	ReferenceEnvironment string
	CandidateEnvironment string
	CaseTimeout          time.Duration
}

type Runner struct {
	reference PlatformClient
	candidate PlatformClient
	options   RunnerOptions
}

type CaseReport struct {
	CaseID     string           `json:"case_id"`
	Reference  Observation      `json:"reference"`
	Candidate  Observation      `json:"candidate"`
	Comparison ComparisonResult `json:"comparison"`
}

type SuiteReport struct {
	Schema               string       `json:"schema"`
	DeerFlowRevision     string       `json:"deerflow_revision"`
	RevisionProvenance   string       `json:"revision_provenance"`
	ReferenceEnvironment string       `json:"reference_environment"`
	CandidateEnvironment string       `json:"candidate_environment"`
	Cases                []CaseReport `json:"cases"`
}

func NewRunner(reference, candidate PlatformClient, options RunnerOptions) (*Runner, error) {
	if reference == nil || reference.Product() != ProductDeerFlow {
		return nil, errors.New("reference client must be DeerFlow")
	}
	if candidate == nil || candidate.Product() != ProductNewX {
		return nil, errors.New("candidate client must be NewX")
	}
	if err := validateCredentials(options.Credentials); err != nil {
		return nil, err
	}
	if err := validateOpaqueID(options.NewXSpaceID); err != nil {
		return nil, errors.New("NewX space id is invalid")
	}
	if options.DeerFlowRevision == "" || len(options.DeerFlowRevision) != 40 || !isLowerHex(options.DeerFlowRevision) {
		return nil, errors.New("DeerFlow revision is invalid")
	}
	if options.RevisionProvenance != RevisionProvenanceVerifiedSourceAttestedRuntime {
		return nil, errors.New("DeerFlow revision provenance is not verified")
	}
	if options.ReferenceEnvironment == "" {
		options.ReferenceEnvironment = "deerflow"
	}
	if options.CandidateEnvironment == "" {
		options.CandidateEnvironment = "newx"
	}
	if safeBlocker(options.ReferenceEnvironment) == "" || safeBlocker(options.CandidateEnvironment) == "" {
		return nil, errors.New("acceptance environment label is invalid")
	}
	if options.CaseTimeout <= 0 {
		options.CaseTimeout = 3 * time.Minute
	}
	return &Runner{reference: reference, candidate: candidate, options: options}, nil
}

func (r *Runner) Authenticate(ctx context.Context) error {
	if err := r.reference.Login(ctx, r.options.Credentials); err != nil {
		if isEndpointUnavailable(err) {
			return &PrerequisiteError{Code: "deerflow_unavailable", Product: ProductDeerFlow}
		}
		return fmt.Errorf("deerflow authentication failed: %w", err)
	}
	if err := r.candidate.Login(ctx, r.options.Credentials); err != nil {
		if isEndpointUnavailable(err) {
			return &PrerequisiteError{Code: "newx_unavailable", Product: ProductNewX}
		}
		return fmt.Errorf("newx authentication failed: %w", err)
	}
	return nil
}

func (r *Runner) RunSuite(ctx context.Context, suite *Suite) (SuiteReport, error) {
	if err := validateSuite(suite); err != nil {
		return SuiteReport{}, err
	}
	report := SuiteReport{
		Schema:               SuiteReportSchemaV1,
		DeerFlowRevision:     suite.DeerFlowRevision,
		RevisionProvenance:   r.options.RevisionProvenance,
		ReferenceEnvironment: r.options.ReferenceEnvironment,
		CandidateEnvironment: r.options.CandidateEnvironment,
		Cases:                make([]CaseReport, 0, len(suite.Cases)),
	}
	if r.options.DeerFlowRevision != suite.DeerFlowRevision {
		return blockedSuiteReport(report, suite.Cases, ProductDeerFlow, "deerflow_revision_mismatch"), nil
	}
	if err := r.Authenticate(ctx); err != nil {
		var prerequisite *PrerequisiteError
		if errors.As(err, &prerequisite) {
			return blockedSuiteReport(report, suite.Cases, prerequisite.Product, prerequisite.Code), nil
		}
		return SuiteReport{}, err
	}
	for _, testCase := range suite.Cases {
		report.Cases = append(report.Cases, r.RunCase(ctx, testCase))
	}
	return report, nil
}

func (r *Runner) RunCase(ctx context.Context, testCase Case) CaseReport {
	reference := r.runProductCase(ctx, r.reference, testCase)
	candidate := r.runProductCase(ctx, r.candidate, testCase)
	return CaseReport{
		CaseID:     testCase.ID,
		Reference:  reference,
		Candidate:  candidate,
		Comparison: CompareCase(testCase, reference, candidate),
	}
}

func (r *Runner) runProductCase(ctx context.Context, client PlatformClient, testCase Case) Observation {
	caseCtx, cancel := context.WithTimeout(ctx, r.options.CaseTimeout)
	defer cancel()
	observation, err := r.executeProductCase(caseCtx, client, testCase)
	if err == nil {
		return observation
	}
	var prerequisite *PrerequisiteError
	if errors.As(err, &prerequisite) {
		return Observation{
			Schema: ObservationSchemaV1, Product: client.Product(), CaseID: testCase.ID,
			Mode: testCase.Mode, Blocker: prerequisite.Code,
		}
	}
	if isEndpointUnavailable(err) {
		return Observation{
			Schema: ObservationSchemaV1, Product: client.Product(), CaseID: testCase.ID,
			Mode: testCase.Mode, Blocker: string(client.Product()) + "_unavailable",
		}
	}
	return Observation{
		Schema: ObservationSchemaV1, Product: client.Product(), CaseID: testCase.ID,
		Mode: testCase.Mode, Terminal: "failed", EventFamilies: []string{"acceptance.execution_error"},
	}
}

func blockedSuiteReport(report SuiteReport, cases []Case, product Product, code string) SuiteReport {
	for _, testCase := range cases {
		reference := Observation{
			Schema: ObservationSchemaV1, Product: ProductDeerFlow, CaseID: testCase.ID, Mode: testCase.Mode,
		}
		candidate := Observation{
			Schema: ObservationSchemaV1, Product: ProductNewX, CaseID: testCase.ID, Mode: testCase.Mode,
		}
		if product == ProductDeerFlow {
			reference.Blocker = code
		} else {
			candidate.Blocker = code
		}
		report.Cases = append(report.Cases, CaseReport{
			CaseID: testCase.ID, Reference: reference, Candidate: candidate,
			Comparison: CompareCase(testCase, reference, candidate),
		})
	}
	return report
}

func isEndpointUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) {
		return true
	}
	var dnsError *net.DNSError
	if errors.As(err, &dnsError) {
		return true
	}
	var operationError *net.OpError
	return errors.As(err, &operationError) &&
		strings.EqualFold(strings.TrimSpace(operationError.Op), "dial")
}

func isLowerHex(value string) bool {
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func (r *Runner) executeProductCase(ctx context.Context, client PlatformClient, testCase Case) (Observation, error) {
	threadID, err := client.CreateThread(ctx, ThreadOptions{SpaceID: r.options.NewXSpaceID})
	if err != nil {
		return Observation{}, err
	}
	input := BuildRunInput(testCase)
	input.OnDisconnect = "cancel"
	if caseHasAction(testCase, ActionReconnect) {
		input.OnDisconnect = "continue"
	}
	runIDs := make([]string, 0, 2)
	combinedStream := StreamResult{ThreadID: threadID}

	switch {
	case caseHasAction(testCase, ActionCancel):
		handle, startErr := client.StartRun(ctx, threadID, input)
		if startErr != nil {
			return Observation{}, startErr
		}
		runIDs = append(runIDs, handle.RunID)
		cancelAction := caseAction(testCase, ActionCancel)
		if waitErr := client.WaitForEvent(ctx, threadID, handle.RunID, cancelAction.AfterEvent); waitErr != nil {
			return Observation{}, waitErr
		}
		if cancelErr := client.CancelRun(ctx, threadID, handle.RunID); cancelErr != nil {
			return Observation{}, cancelErr
		}
		combinedStream, err = client.StreamExistingRun(ctx, threadID, handle.RunID, StreamOptions{})
	case caseHasAction(testCase, ActionReconnect):
		handle, startErr := client.StartRun(ctx, threadID, input)
		if startErr != nil {
			return Observation{}, startErr
		}
		runIDs = append(runIDs, handle.RunID)
		reconnect := caseAction(testCase, ActionReconnect)
		first, streamErr := client.StreamExistingRun(ctx, threadID, handle.RunID, StreamOptions{StopAfterFrames: reconnect.AfterEvents})
		if streamErr != nil {
			return Observation{}, streamErr
		}
		second, streamErr := client.StreamExistingRun(ctx, threadID, handle.RunID, StreamOptions{AfterEventID: first.LastEventID})
		if streamErr != nil {
			return Observation{}, streamErr
		}
		combinedStream = second
		combinedStream.Frames = append(append([]SSEFrame(nil), first.Frames...), second.Frames...)
		combinedStream.LastEventID = second.LastEventID
	default:
		combinedStream, err = client.StreamRun(ctx, threadID, input)
		if err == nil {
			runIDs = append(runIDs, combinedStream.RunID)
		}
		if err == nil && caseHasAction(testCase, ActionFollowUp) {
			followUp := caseAction(testCase, ActionFollowUp)
			var followedUp StreamResult
			followedUp, err = client.FollowUpRun(ctx, threadID, input, followUp.Answer)
			if err == nil {
				runIDs = append(runIDs, followedUp.RunID)
				combinedStream.Terminal = followedUp.Terminal
				combinedStream.LastEventID = followedUp.LastEventID
				combinedStream.Frames = append(combinedStream.Frames, followedUp.Frames...)
			}
		}
	}
	if err != nil {
		return Observation{}, err
	}
	if len(runIDs) == 0 && combinedStream.RunID != "" {
		runIDs = append(runIDs, combinedStream.RunID)
	}
	if caseHasAction(testCase, ActionCancel) && len(runIDs) == 1 {
		handle, getErr := client.GetRun(ctx, threadID, runIDs[0])
		if getErr != nil {
			return Observation{}, getErr
		}
		switch handle.Status {
		case "cancelled", "interrupted":
			combinedStream.Terminal = "cancelled"
		default:
			combinedStream.Terminal = handle.Status
		}
	}

	state, err := client.GetThreadState(ctx, threadID)
	if err != nil {
		return Observation{}, err
	}
	history, err := client.GetThreadHistory(ctx, threadID, 100)
	if err != nil {
		return Observation{}, err
	}
	historyEntries, err := validateHistoryEntries(history)
	if err != nil {
		return Observation{}, err
	}
	stateValuesBeforeReload := stateValues(state)
	stateReloaded := false
	if caseHasAction(testCase, ActionReloadState) {
		reloaded, reloadErr := client.GetThreadState(ctx, threadID)
		if reloadErr != nil {
			return Observation{}, reloadErr
		}
		stateValuesAfterReload := stateValues(reloaded)
		stateReloaded = durableStateMatches(testCase, stateValuesBeforeReload, stateValuesAfterReload)
		state = reloaded
	}
	messages := make([]RawMessage, 0)
	tokens := make([]RawTokenUsage, 0)
	events := make([]RawEvent, 0)
	for runIndex, runID := range runIDs {
		if caseHasAction(testCase, ActionFollowUp) && runIndex == 1 {
			events = append(events, RawEvent{
				ID: "acceptance.followup.started", Type: "followup.started", Payload: map[string]any{},
			})
		}
		runMessages, usage, readErr := readAllRunMessages(ctx, client, threadID, runID)
		if readErr != nil {
			return Observation{}, readErr
		}
		messages = append(messages, runMessages...)
		tokens = append(tokens, usage...)
		runEvents, readErr := client.ListRunEvents(ctx, threadID, runID, 1000)
		if readErr != nil {
			return Observation{}, readErr
		}
		events = append(events, rawEventsFromMaps(runEvents)...)
		if caseHasAction(testCase, ActionFollowUp) && runIndex == 1 && combinedStream.Terminal == "success" {
			events = append(events, RawEvent{
				ID: "acceptance.followup.completed", Type: "followup.completed", Payload: map[string]any{},
			})
		}
	}
	if caseHasAction(testCase, ActionCancel) && combinedStream.Terminal == "cancelled" && !hasCanonicalEvent(events, "run.cancelled") {
		events = append(events, RawEvent{
			ID: "acceptance.cancel.terminal", Type: "run.cancelled", Payload: map[string]any{},
		})
	}

	return NormalizeCapture(RawCapture{
		Product:                  client.Product(),
		CaseID:                   testCase.ID,
		Mode:                     testCase.Mode,
		Events:                   events,
		Messages:                 messages,
		State:                    stateValues(state),
		Tokens:                   tokens,
		Terminal:                 combinedStream.Terminal,
		Reconnected:              caseHasAction(testCase, ActionReconnect),
		ReconnectDuplicateEvents: countDuplicateSSEEventIDs(combinedStream.Frames),
		RunCount:                 len(runIDs),
		CancelRequested:          caseHasAction(testCase, ActionCancel),
		StateReloaded:            stateReloaded,
		HistoryEntries:           historyEntries,
	})
}

func validateHistoryEntries(history []map[string]any) (int, error) {
	if len(history) == 0 || len(history) > 1000 {
		return 0, errors.New("checkpoint history is empty or outside bounds")
	}
	seen := make(map[string]struct{}, len(history))
	for _, entry := range history {
		checkpointID := strings.TrimSpace(stringValue(entry["checkpoint_id"]))
		if validateOpaqueID(checkpointID) != nil {
			return 0, errors.New("checkpoint history contains an invalid id")
		}
		if _, exists := seen[checkpointID]; exists {
			return 0, errors.New("checkpoint history contains duplicate ids")
		}
		seen[checkpointID] = struct{}{}
	}
	return len(history), nil
}

func durableStateMatches(testCase Case, before, after map[string]any) bool {
	if testCase.Expect.Todo.Required {
		beforeTodo := normalizeTodo(before["todos"])
		afterTodo := normalizeTodo(after["todos"])
		return beforeTodo.Total > 0 && beforeTodo == afterTodo
	}
	return false
}

func hasCanonicalEvent(events []RawEvent, expected string) bool {
	for _, event := range events {
		family, err := canonicalEventFamily(event)
		if err == nil && family == expected {
			return true
		}
	}
	return false
}

func BuildRunInput(testCase Case) RunInput {
	contextValues := map[string]any{
		"mode":             string(testCase.Mode),
		"thinking_enabled": testCase.Mode != ModeFlash,
		"is_plan_mode":     testCase.Mode == ModePro || testCase.Mode == ModeUltra,
		"subagent_enabled": testCase.Mode == ModeUltra,
	}
	configValues := map[string]any{
		"runtime":          "eino_adk",
		"recursion_limit":  1000,
		"mode":             string(testCase.Mode),
		"thinking_enabled": contextValues["thinking_enabled"],
		"is_plan_mode":     contextValues["is_plan_mode"],
		"subagent_enabled": contextValues["subagent_enabled"],
	}
	switch testCase.Mode {
	case ModeThinking:
		contextValues["reasoning_effort"] = "low"
	case ModePro:
		contextValues["reasoning_effort"] = "medium"
	case ModeUltra:
		contextValues["reasoning_effort"] = "high"
	}
	if effort, ok := contextValues["reasoning_effort"]; ok {
		configValues["reasoning_effort"] = effort
	}
	return RunInput{
		AssistantID: "lead_agent",
		Input: map[string]any{
			"messages": []any{map[string]any{"role": "user", "content": testCase.InputPrompt}},
		},
		Config:     configValues,
		Context:    contextValues,
		StreamMode: []string{"events"},
	}
}

func caseHasAction(testCase Case, actionType ActionType) bool {
	return slices.ContainsFunc(testCase.Actions, func(action Action) bool { return action.Type == actionType })
}

func caseAction(testCase Case, actionType ActionType) Action {
	for _, action := range testCase.Actions {
		if action.Type == actionType {
			return action
		}
	}
	return Action{}
}

func readAllRunMessages(
	ctx context.Context,
	client PlatformClient,
	threadID string,
	runID string,
) ([]RawMessage, []RawTokenUsage, error) {
	messages := make([]RawMessage, 0)
	tokens := make([]RawTokenUsage, 0)
	afterSeq := int64(0)
	for pageIndex := 0; pageIndex < 32; pageIndex++ {
		page, err := client.ListRunMessages(ctx, threadID, runID, PageRequest{Limit: 200, AfterSeq: afterSeq})
		if err != nil {
			return nil, nil, err
		}
		for _, message := range page.Data {
			messages = append(messages, rawMessageFromMap(message))
			if usage, ok := rawTokenUsageFromMap(message); ok {
				tokens = append(tokens, usage)
			}
			if sequence := int64Value(message["seq"]); sequence > afterSeq {
				afterSeq = sequence
			}
		}
		if !page.HasMore {
			return messages, tokens, nil
		}
		if len(page.Data) == 0 || afterSeq == 0 {
			return nil, nil, errors.New("message pagination did not advance")
		}
	}
	return nil, nil, errors.New("message pagination exceeded page limit")
}

func rawMessageFromMap(message map[string]any) RawMessage {
	source := message
	if content, ok := message["content"].(map[string]any); ok {
		source = content
	}
	role, _ := source["role"].(string)
	if role == "" {
		role, _ = message["role"].(string)
	}
	if role == "" {
		typeName, _ := source["type"].(string)
		if typeName == "" {
			typeName, _ = message["type"].(string)
		}
		switch strings.ToLower(typeName) {
		case "ai", "assistant":
			role = "assistant"
		case "human", "user":
			role = "user"
		default:
			role = typeName
		}
	}
	if role == "" {
		switch strings.ToLower(strings.TrimSpace(stringValue(message["event_type"]))) {
		case "llm.ai.response":
			role = "assistant"
		case "llm.human.input":
			role = "user"
		case "llm.tool.result":
			role = "tool"
		}
	}
	content, _ := source["content"].(string)
	if content == "" && source["content"] != nil {
		content = "structured-content-present"
	}
	reasoningPresent, _ := source["reasoning_present"].(bool)
	if !reasoningPresent {
		reasoningPresent, _ = message["reasoning_present"].(bool)
	}
	return RawMessage{
		Role: role, Content: content, ReasoningPresent: reasoningPresent || hasReasoningSignal(source),
	}
}

func rawTokenUsageFromMap(message map[string]any) (RawTokenUsage, bool) {
	usage, _ := message["usage_metadata"].(map[string]any)
	if usage == nil {
		usage, _ = message["usage"].(map[string]any)
	}
	if usage == nil {
		metadata, _ := message["metadata"].(map[string]any)
		usage, _ = metadata["usage"].(map[string]any)
	}
	if usage == nil {
		content, _ := message["content"].(map[string]any)
		usage, _ = content["usage_metadata"].(map[string]any)
		if usage == nil {
			usage, _ = content["usage"].(map[string]any)
		}
	}
	if usage == nil {
		return RawTokenUsage{}, false
	}
	result := RawTokenUsage{
		Input:  firstInt64(usage, "input_tokens", "prompt_tokens"),
		Output: firstInt64(usage, "output_tokens", "completion_tokens"),
		Total:  firstInt64(usage, "total_tokens"),
	}
	return result, result.Input > 0 || result.Output > 0 || result.Total > 0
}

func hasReasoningSignal(message map[string]any) bool {
	if message == nil {
		return false
	}
	for _, key := range []string{"reasoning_content", "reasoning"} {
		if value, ok := message[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	for _, key := range []string{"additional_kwargs", "response_metadata"} {
		if nested, ok := message[key].(map[string]any); ok && hasReasoningSignal(nested) {
			return true
		}
	}
	if blocks, ok := message["content"].([]any); ok {
		for _, block := range blocks {
			item, _ := block.(map[string]any)
			blockType := strings.ToLower(strings.TrimSpace(stringValue(item["type"])))
			if blockType == "reasoning" || blockType == "thinking" {
				return true
			}
		}
	}
	return false
}

func firstInt64(values map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value := int64Value(values[key]); value != 0 {
			return value
		}
	}
	return 0
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	default:
		return 0
	}
}

func rawEventsFromMaps(values []map[string]any) []RawEvent {
	result := make([]RawEvent, 0, len(values)+4)
	seenSubagents := map[string]struct{}{}
	for _, value := range values {
		eventType, _ := value["event_type"].(string)
		payload := boundedEventPayload(value)
		caller := stringValue(payload["caller"])
		if strings.HasPrefix(strings.ToLower(caller), "subagent:") {
			if _, exists := seenSubagents[caller]; !exists {
				seenSubagents[caller] = struct{}{}
				result = append(result, RawEvent{
					ID: stringValue(value["seq"]) + ".subagent", Type: "subagent.started", Payload: map[string]any{},
				})
			}
			payload["kind"] = "subagent"
		}
		eventID := stringValue(value["event_id"])
		if eventID == "" {
			eventID = stringValue(value["seq"])
		}
		result = append(result, RawEvent{
			ID:      eventID,
			Type:    eventType,
			Payload: payload,
		})
		if strings.EqualFold(strings.TrimSpace(eventType), "run.interrupted") &&
			strings.EqualFold(strings.TrimSpace(stringValue(payload["interaction_kind"])), "clarification") {
			result = append(result, RawEvent{
				ID: eventID + ".clarification", Type: "clarification.requested", Payload: map[string]any{},
			})
		}
		if usage, ok := rawTokenUsageFromMap(value); ok && usage.Total+usage.Input+usage.Output > 0 {
			result = append(result, RawEvent{ID: eventID + ".usage", Type: "token.usage", Payload: map[string]any{}})
		}
	}
	return result
}

func boundedEventPayload(value map[string]any) map[string]any {
	result := map[string]any{}
	payload := mapValue(value["payload"])
	metadata := mapValue(value["metadata"])
	content := mapValue(value["content"])
	for _, key := range []string{"status", "kind", "caller", "tool_name"} {
		for _, source := range []map[string]any{payload, metadata, content} {
			if candidate := strings.TrimSpace(stringValue(source[key])); candidate != "" && len(candidate) <= 128 && !strings.ContainsAny(candidate, "\r\n\x00") {
				result[key] = candidate
				break
			}
		}
	}
	if _, exists := result["tool_name"]; !exists {
		if name := strings.TrimSpace(stringValue(content["name"])); name != "" && len(name) <= 128 && !strings.ContainsAny(name, "\r\n\x00") {
			result["tool_name"] = name
		}
	}
	if interaction := mapValue(payload["human_interaction"]); interaction != nil {
		if kind := strings.TrimSpace(stringValue(interaction["kind"])); kind != "" && len(kind) <= 80 && !strings.ContainsAny(kind, "\r\n\x00") {
			result["interaction_kind"] = kind
		}
	}
	if hasVisibleMessageContent(content) || hasVisibleMessageContent(payload) {
		result["assistant_content_present"] = true
	}
	if hasReasoningSignal(content) {
		result["reasoning_present"] = true
	}
	return result
}

func hasVisibleMessageContent(value map[string]any) bool {
	if value == nil {
		return false
	}
	if content, ok := value["content"].(string); ok {
		return strings.TrimSpace(content) != ""
	}
	if blocks, ok := value["content"].([]any); ok {
		return len(blocks) > 0
	}
	return false
}

func countDuplicateSSEEventIDs(frames []SSEFrame) int {
	seen := make(map[string]struct{}, len(frames))
	duplicates := 0
	for _, frame := range frames {
		id := strings.TrimSpace(frame.ID)
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "/\\\r\n\t") {
			continue
		}
		if _, exists := seen[id]; exists {
			duplicates++
			continue
		}
		seen[id] = struct{}{}
	}
	return duplicates
}

func stateValues(state map[string]any) map[string]any {
	if values, ok := state["values"].(map[string]any); ok {
		return values
	}
	return state
}

func mapValue(value any) map[string]any {
	if mapped, ok := value.(map[string]any); ok {
		return mapped
	}
	if encoded, ok := value.(string); ok && len(encoded) <= 1024*1024 {
		var mapped map[string]any
		if json.Unmarshal([]byte(encoded), &mapped) == nil {
			return mapped
		}
	}
	return map[string]any{}
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

func WriteJSONReport(writer io.Writer, report SuiteReport) error {
	if err := validateSuiteReport(report); err != nil {
		return err
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func WriteMarkdownReport(writer io.Writer, report SuiteReport) error {
	if err := validateSuiteReport(report); err != nil {
		return err
	}
	var buffer bytes.Buffer
	fmt.Fprintln(&buffer, "# DeerFlow Agent Semantic Core Acceptance")
	fmt.Fprintln(&buffer)
	fmt.Fprintf(&buffer, "- DeerFlow revision: `%s`\n", report.DeerFlowRevision)
	fmt.Fprintf(&buffer, "- Revision provenance: `%s`\n", report.RevisionProvenance)
	fmt.Fprintf(&buffer, "- Reference environment: `%s`\n", report.ReferenceEnvironment)
	fmt.Fprintf(&buffer, "- Candidate environment: `%s`\n", report.CandidateEnvironment)
	fmt.Fprintf(&buffer, "- Cases: `%d`\n\n", len(report.Cases))
	fmt.Fprintln(&buffer, "| Case | Status | Blocker |")
	fmt.Fprintln(&buffer, "| --- | --- | --- |")
	for _, testCase := range report.Cases {
		blocker := testCase.Comparison.Blocker
		if blocker == "" {
			blocker = "-"
		}
		fmt.Fprintf(&buffer, "| `%s` | `%s` | `%s` |\n", testCase.CaseID, testCase.Comparison.Status, blocker)
	}
	for _, testCase := range report.Cases {
		if len(testCase.Comparison.Checks) == 0 {
			continue
		}
		fmt.Fprintf(&buffer, "\n## `%s` invariant results\n\n", testCase.CaseID)
		fmt.Fprintln(&buffer, "| Invariant | Expected | Actual | Result |")
		fmt.Fprintln(&buffer, "| --- | --- | --- | --- |")
		for _, check := range testCase.Comparison.Checks {
			result := "pass"
			if !check.Passed {
				result = "fail"
			}
			fmt.Fprintf(
				&buffer,
				"| `%s` | `%s` | `%s` | `%s` |\n",
				markdownReportCell(check.Name),
				markdownReportCell(check.Expected),
				markdownReportCell(check.Actual),
				result,
			)
		}
	}
	_, err := io.Copy(writer, &buffer)
	return err
}

func validateSuiteReport(report SuiteReport) error {
	if report.Schema != SuiteReportSchemaV1 {
		return errors.New("acceptance report schema is invalid")
	}
	if report.DeerFlowRevision != lockedDeerFlowRevision {
		return errors.New("acceptance report revision is invalid")
	}
	if report.RevisionProvenance != RevisionProvenanceVerifiedSourceAttestedRuntime {
		return errors.New("acceptance report revision provenance is invalid")
	}
	if len(report.Cases) > 64 {
		return errors.New("acceptance report exceeds case limit")
	}
	if safeBlocker(report.ReferenceEnvironment) == "" || safeBlocker(report.CandidateEnvironment) == "" {
		return errors.New("acceptance report environment label is invalid")
	}
	for _, testCase := range report.Cases {
		if strings.TrimSpace(testCase.CaseID) == "" || len(testCase.CaseID) > 80 {
			return errors.New("acceptance report case id is invalid")
		}
		if blocker := testCase.Comparison.Blocker; blocker != "" && safeBlocker(blocker) == "" {
			return errors.New("acceptance report blocker is invalid")
		}
		if blocker := testCase.Reference.Blocker; blocker != "" && safeBlocker(blocker) == "" {
			return errors.New("acceptance report reference blocker is invalid")
		}
		if blocker := testCase.Candidate.Blocker; blocker != "" && safeBlocker(blocker) == "" {
			return errors.New("acceptance report candidate blocker is invalid")
		}
		for _, check := range testCase.Comparison.Checks {
			if !safeReportSymbol(check.Name) || !safeReportSymbol(check.Expected) || !safeReportSymbol(check.Actual) {
				return errors.New("acceptance report invariant is invalid")
			}
		}
	}
	return nil
}

func safeReportSymbol(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") {
		return false
	}
	return true
}

func markdownReportCell(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "`", "'")
	return value
}
