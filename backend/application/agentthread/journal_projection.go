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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

const journalProjectionProducerVersion = "runtime-v1"

// JournalEventProjection is the public, storage-ready semantic projection of a
// runtime event. CorrelationKind and CorrelationKey remain internal and allow
// the persistence adapter to bind identities to the authoritative Attempt.
type JournalEventProjection struct {
	EventType       string
	Status          string
	Payload         string
	IdempotencyKey  string
	ActionID        string
	Phase           string
	Operation       string
	Target          string
	Milestone       string
	CorrelationKind string
	CorrelationKey  string
}

type journalProjectionEnvelope struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// ProjectRunEventToJournal converts only events that can produce a trustworthy
// user-visible node. Pending plans, assistant messages, and events without an
// authoritative cross-phase identity intentionally return nil.
func ProjectRunEventToJournal(event RunEvent) (*JournalEventProjection, error) {
	eventType := strings.ToLower(strings.TrimSpace(event.EventType))
	if eventType == "" {
		return nil, fmt.Errorf("runtime event type is required")
	}

	var source map[string]any
	if journalProjectionSupportsEvent(eventType) {
		if err := json.Unmarshal([]byte(event.Payload), &source); err != nil || source == nil {
			return nil, fmt.Errorf("journal source payload for %s must be a JSON object", eventType)
		}
	}
	publicPayload := publicJSONObject(projectPublicRunEventPayload(eventType, event.Payload))
	switch {
	case eventType == "message.completed":
		return projectJournalMessageToolAction(event, publicPayload, source)
	case eventType == "run.interrupted":
		return projectJournalInterruptedConfirmation(event, publicPayload)
	case isJournalRunTerminalEvent(eventType):
		return projectJournalRunTerminal(event, eventType, publicPayload)
	case strings.HasPrefix(eventType, "plan.task."), strings.HasPrefix(eventType, "todo."):
		return projectJournalMilestone(event, eventType, publicPayload)
	case strings.HasPrefix(eventType, "tool."), strings.HasPrefix(eventType, "mcp.tool."):
		return projectJournalToolAction(event, eventType, publicPayload, source)
	case strings.HasPrefix(eventType, "skill."):
		return projectJournalSkillAction(event, eventType, publicPayload)
	case strings.HasPrefix(eventType, "subagent."):
		return projectJournalSubagentAction(event, eventType, publicPayload)
	case strings.HasPrefix(eventType, "artifact."):
		return projectJournalArtifact(event, eventType, publicPayload)
	case strings.HasPrefix(eventType, "verification."):
		return projectJournalVerification(event, eventType, publicPayload)
	case strings.HasPrefix(eventType, "human.interaction."):
		return projectJournalConfirmation(event, eventType, publicPayload)
	default:
		return nil, nil
	}
}

func journalProjectionSupportsEvent(eventType string) bool {
	return eventType == "message.completed" ||
		eventType == "run.interrupted" ||
		isJournalRunTerminalEvent(eventType) ||
		strings.HasPrefix(eventType, "plan.task.") ||
		strings.HasPrefix(eventType, "todo.") ||
		strings.HasPrefix(eventType, "tool.") ||
		strings.HasPrefix(eventType, "mcp.tool.") ||
		strings.HasPrefix(eventType, "skill.") ||
		strings.HasPrefix(eventType, "subagent.") ||
		strings.HasPrefix(eventType, "artifact.") ||
		strings.HasPrefix(eventType, "verification.") ||
		strings.HasPrefix(eventType, "human.interaction.")
}

func projectJournalMessageToolAction(
	event RunEvent,
	payload map[string]any,
	source map[string]any,
) (*JournalEventProjection, error) {
	if strings.ToLower(publicString(payload["role"])) != "assistant" {
		return nil, nil
	}
	toolCalls, ok := payload["tool_calls"].([]any)
	if !ok || len(toolCalls) == 0 {
		return nil, nil
	}
	toolCall, ok := toolCalls[0].(map[string]any)
	if !ok {
		return nil, nil
	}
	return projectJournalToolAction(event, "tool.started", map[string]any{
		"tool_name":      toolCall["name"],
		"tool_call_id":   toolCall["id"],
		"journal_target": toolCall["journal_target"],
		"plan_task_id":   payload["plan_task_id"],
	}, source)
}

func projectJournalInterruptedConfirmation(
	event RunEvent,
	payload map[string]any,
) (*JournalEventProjection, error) {
	confirmationID := ""
	var prompt map[string]any
	if container, ok := payload["interrupts"].(map[string]any); ok {
		if items, ok := container["items"].([]any); ok {
			for _, item := range items {
				interrupt, ok := item.(map[string]any)
				if !ok {
					continue
				}
				info, ok := interrupt["info"].(map[string]any)
				if !ok {
					continue
				}
				confirmationID = journalSourceID(interrupt["id"])
				prompt = info
				break
			}
		}
	}
	if prompt == nil {
		prompt, _ = payload["human_interaction"].(map[string]any)
	}
	if prompt == nil {
		return nil, nil
	}
	if confirmationID == "" {
		confirmationID = journalSourceID(prompt["interaction_id"])
	}
	allowedActionKeys := make([]string, 0)
	if choices, ok := prompt["choices"].([]any); ok {
		for _, item := range choices {
			choice, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if id := journalSourceID(choice["id"]); id != "" {
				allowedActionKeys = append(allowedActionKeys, id)
			}
		}
	}
	return projectJournalConfirmation(event, "human.interaction.requested", map[string]any{
		"confirmation_id":     confirmationID,
		"confirmation_type":   prompt["kind"],
		"prompt":              journalFirstLabel(prompt, "question", "title", "summary"),
		"allowed_action_keys": allowedActionKeys,
	})
}

func projectJournalRunTerminal(
	event RunEvent,
	eventType string,
	payload map[string]any,
) (*JournalEventProjection, error) {
	status := journalTerminalStatus(eventType, publicString(payload["error_code"]))
	data := map[string]any{"status": status}
	return newJournalProjection(
		event,
		"run.lifecycle",
		status,
		"terminal",
		data,
		"run",
		strconv.FormatInt(event.RunID, 10),
		"terminal:"+status,
	)
}

func projectJournalMilestone(
	event RunEvent,
	eventType string,
	payload map[string]any,
) (*JournalEventProjection, error) {
	sourceID := journalSourceID(payload["plan_task_id"])
	if sourceID == "" {
		sourceID = journalSourceID(payload["step_id"])
	}
	if sourceID == "" {
		return nil, nil
	}
	status := strings.ToLower(publicString(payload["status"]))
	executionIntro := journalFirstLabel(payload, "execution_intro")
	if strings.HasSuffix(eventType, ".deleted") {
		return nil, nil
	}
	if status == "pending" || status == "planned" {
		if executionIntro == "" || !strings.HasSuffix(eventType, ".created") {
			return nil, nil
		}
		return newJournalProjection(
			event,
			"journal.intro",
			"running",
			"intro",
			map[string]any{"text": executionIntro},
			"run",
			strconv.FormatInt(event.RunID, 10),
			"intro",
		)
	}

	projectionType := "milestone.started"
	projectionStatus := "running"
	phase := "started"
	if status == "completed" || strings.HasSuffix(eventType, ".completed") {
		projectionType = "milestone.terminal"
		projectionStatus = "completed"
		phase = "terminal"
	} else if status != "in_progress" && status != "running" {
		return nil, nil
	}

	title := journalFirstLabel(payload, "subject", "title", "name", "active_form")
	if title == "" {
		return nil, nil
	}
	milestoneID := journalStableProjectionID(event.RunID, "milestone", sourceID)
	data := map[string]any{"milestone_id": milestoneID, "title": title}
	if executionIntro != "" {
		data["execution_intro"] = executionIntro
	}
	return newJournalProjection(
		event,
		projectionType,
		projectionStatus,
		"milestone",
		data,
		"milestone",
		sourceID,
		phase,
	)
}

func projectJournalToolAction(
	event RunEvent,
	eventType string,
	payload map[string]any,
	source map[string]any,
) (*JournalEventProjection, error) {
	phase, projectionType, status := journalActionPhase(eventType, payload)
	if phase == "" {
		return nil, nil
	}
	correlationKey := journalFirstSourceID(payload, "invocation_id", "tool_call_id", "step_id", "action_id")
	if correlationKey == "" {
		return nil, nil
	}
	correlationKey = journalScopedToolCorrelationKey(source, correlationKey)
	if correlationKey == "" {
		return nil, nil
	}
	toolName := strings.ToLower(publicString(payload["tool_name"]))
	if journalInternalTool(toolName) {
		return nil, nil
	}
	operation := journalToolOperation(toolName)
	target := journalControlledTarget(payload["journal_target"])
	if target == "" {
		target = journalToolTarget(toolName, operation)
	}
	actionID := journalStableProjectionID(event.RunID, "action", correlationKey)
	milestoneID := ""
	if sourceMilestone := journalSourceID(payload["plan_task_id"]); sourceMilestone != "" {
		milestoneID = journalStableProjectionID(event.RunID, "milestone", sourceMilestone)
	}
	runningVerb, completedVerb := journalActionVerbs(operation)
	data := map[string]any{
		"action_id":              actionID,
		"operation":              operation,
		"target":                 target,
		"display_verb_running":   runningVerb,
		"display_verb_completed": completedVerb,
	}
	if milestoneID != "" {
		data["milestone_id"] = milestoneID
	}
	progressPhase := phase
	if phase == "progress" {
		if revision := journalSourceID(payload["progress_revision"]); revision != "" {
			progressPhase += ":" + revision
		}
	}
	projection, err := newJournalProjection(
		event,
		projectionType,
		status,
		"generic",
		data,
		"action",
		correlationKey,
		progressPhase,
	)
	if err != nil {
		return nil, err
	}
	projection.ActionID = actionID
	projection.Phase = progressPhase
	projection.Operation = operation
	projection.Target = target
	projection.Milestone = milestoneID
	return projection, nil
}

func journalScopedToolCorrelationKey(
	source map[string]any,
	toolCallID string,
) string {
	agentName := journalSourceID(source["agent_name"])
	if agentName == "" {
		return toolCallID
	}
	return adkJournalToolBindingKey(agentName, toolCallID)
}

func journalInternalTool(toolName string) bool {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "present_files", "skill", "taskcreate", "taskget", "taskupdate", "tasklist":
		return true
	default:
		return false
	}
}

func projectJournalSkillAction(
	event RunEvent,
	eventType string,
	payload map[string]any,
) (*JournalEventProjection, error) {
	phase, projectionType, status := journalActionPhase(eventType, payload)
	if phase == "" {
		return nil, nil
	}
	skillID := journalSourceID(payload["skill_id"])
	target := journalControlledTarget(payload["skill_name"])
	if skillID == "" || target == "" {
		return nil, nil
	}
	planTaskID := journalSourceID(payload["plan_task_id"])
	correlationKey := journalSourceID(payload["action_id"])
	if correlationKey == "" {
		correlationKey = strings.Join([]string{skillID, planTaskID}, ":")
	}
	actionID := journalSourceID(payload["action_id"])
	if actionID == "" {
		actionID = journalStableProjectionID(event.RunID, "skill", correlationKey)
	}
	milestoneID := ""
	if planTaskID != "" {
		milestoneID = journalStableProjectionID(event.RunID, "milestone", planTaskID)
	}
	runningVerb, completedVerb := journalActionVerbs("use_skill")
	data := map[string]any{
		"action_id":              actionID,
		"operation":              "use_skill",
		"target":                 target,
		"display_verb_running":   runningVerb,
		"display_verb_completed": completedVerb,
		"content_type":           "skill",
	}
	if milestoneID != "" {
		data["milestone_id"] = milestoneID
	}
	projection, err := newJournalProjection(
		event,
		projectionType,
		status,
		"skill",
		data,
		"skill",
		correlationKey,
		phase,
	)
	if err != nil {
		return nil, err
	}
	projection.ActionID = actionID
	projection.Phase = phase
	projection.Operation = "use_skill"
	projection.Target = target
	projection.Milestone = milestoneID
	return projection, nil
}

func projectJournalSubagentAction(
	event RunEvent,
	eventType string,
	payload map[string]any,
) (*JournalEventProjection, error) {
	childRunID := journalFirstSourceID(payload, "child_run_id", "subagent_run_id")
	if childRunID == "" {
		return nil, nil
	}
	phase, projectionType, status := journalActionPhase(eventType, payload)
	if phase == "" {
		return nil, nil
	}
	name := journalFirstLabel(payload, "name", "agent_name")
	if name == "" {
		if nested, ok := payload["subagent"].(map[string]any); ok {
			name = journalFirstLabel(nested, "name")
		}
	}
	if name == "" {
		name = "协作任务"
	}
	actionID := journalStableProjectionID(event.RunID, "subagent", childRunID)
	runningVerb, completedVerb := journalActionVerbs("execute")
	projection, err := newJournalProjection(
		event,
		projectionType,
		status,
		"generic",
		map[string]any{
			"action_id":              actionID,
			"operation":              "execute",
			"target":                 name,
			"display_verb_running":   runningVerb,
			"display_verb_completed": completedVerb,
			"content_type":           "generic",
		},
		"subagent",
		childRunID,
		phase,
	)
	if err != nil {
		return nil, err
	}
	projection.ActionID = actionID
	projection.Phase = phase
	projection.Operation = "execute"
	projection.Target = name
	return projection, nil
}

func projectJournalArtifact(
	event RunEvent,
	eventType string,
	payload map[string]any,
) (*JournalEventProjection, error) {
	if eventType != "artifact.presented" && eventType != "artifact.created" {
		return nil, nil
	}
	artifactID := journalSourceID(payload["artifact_id"])
	title := journalFirstLabel(payload, "title")
	collectionID := journalSourceID(payload["collection_id"])
	if items, ok := payload["artifacts"].([]any); ok && len(items) > 0 {
		collectionArtifactIDs := make([]string, 0, len(items))
		itemCollectionID := ""
		itemCollectionConsistent := true
		for _, item := range items {
			artifact, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if id := journalSourceID(artifact["artifact_id"]); id != "" {
				collectionArtifactIDs = append(collectionArtifactIDs, id)
			}
			if id := journalSourceID(artifact["collection_id"]); id != "" {
				if itemCollectionID != "" && itemCollectionID != id {
					itemCollectionConsistent = false
				}
				itemCollectionID = id
			}
		}
		if first, ok := items[0].(map[string]any); ok {
			artifactID = journalSourceID(first["artifact_id"])
			title = journalFirstLabel(first, "title")
		}
		if collectionID == "" && itemCollectionConsistent {
			collectionID = itemCollectionID
		}
		if collectionID == "" && len(collectionArtifactIDs) > 1 {
			collectionID = journalStableProjectionID(
				event.RunID,
				"artifact-collection",
				strings.Join(collectionArtifactIDs, ","),
			)
		}
	}
	if artifactID == "" {
		return nil, nil
	}
	data := map[string]any{"artifact_id": artifactID}
	if title != "" {
		data["title"] = title
	}
	if collectionID != "" {
		data["collection_id"] = collectionID
	}
	correlationKey := artifactID
	if collectionID != "" {
		correlationKey = collectionID
	}
	projection, err := newJournalProjection(
		event,
		"artifact.created",
		"completed",
		"artifact",
		data,
		"artifact",
		correlationKey,
		"created",
	)
	if projection != nil {
		projection.Target = title
	}
	return projection, err
}

func projectJournalVerification(
	event RunEvent,
	eventType string,
	payload map[string]any,
) (*JournalEventProjection, error) {
	if !strings.HasSuffix(eventType, ".completed") &&
		!strings.HasSuffix(eventType, ".succeeded") &&
		!strings.HasSuffix(eventType, ".failed") &&
		!strings.HasSuffix(eventType, ".timed_out") &&
		!strings.HasSuffix(eventType, ".canceled") &&
		!strings.HasSuffix(eventType, ".cancelled") {
		return nil, nil
	}
	verificationID := journalSourceID(payload["verification_id"])
	title := journalFirstLabel(payload, "title", "name")
	if verificationID == "" || title == "" {
		return nil, nil
	}
	status := journalTerminalStatus(eventType, publicString(payload["error_code"]))
	resultSummary := journalFirstLabel(payload, "summary", "result_summary")
	if resultSummary == "" {
		if status == "completed" {
			resultSummary = "校验完成"
		} else {
			resultSummary = "校验未通过"
		}
	}
	projection, err := newJournalProjection(
		event,
		"verification.terminal",
		status,
		"verification",
		map[string]any{
			"verification_id": verificationID,
			"title":           title,
			"result_summary":  resultSummary,
		},
		"verification",
		verificationID,
		"terminal",
	)
	if projection != nil {
		projection.Operation = "verify"
		projection.Target = title
	}
	return projection, err
}

func journalSupplementalRunEvents(event RunEvent) ([]RunEvent, error) {
	if strings.ToLower(strings.TrimSpace(event.EventType)) != "message.completed" {
		return nil, nil
	}
	var source map[string]any
	if err := json.Unmarshal([]byte(event.Payload), &source); err != nil || source == nil {
		return nil, fmt.Errorf("journal supplemental source payload must be a JSON object")
	}
	publicPayload := publicJSONObject(projectPublicRunEventPayload("message.completed", event.Payload))
	if strings.ToLower(publicString(publicPayload["role"])) != "assistant" {
		return nil, nil
	}
	toolCalls, _ := publicPayload["tool_calls"].([]any)
	if len(toolCalls) <= 1 {
		return nil, nil
	}
	result := make([]RunEvent, 0, len(toolCalls)-1)
	planTaskID := journalSourceID(publicPayload["plan_task_id"])
	for _, item := range toolCalls[1:] {
		toolCall, ok := item.(map[string]any)
		if !ok {
			continue
		}
		toolCallID := journalSourceID(toolCall["id"])
		toolName := journalSourceID(toolCall["name"])
		if toolCallID == "" || toolName == "" {
			continue
		}
		payloadData := map[string]any{
			"tool_name": toolName, "tool_call_id": toolCallID,
		}
		if journalTarget := journalControlledTarget(toolCall["journal_target"]); journalTarget != "" {
			payloadData["journal_target"] = journalTarget
		}
		if planTaskID != "" {
			payloadData["plan_task_id"] = planTaskID
		}
		payload, err := json.Marshal(payloadData)
		if err != nil {
			return nil, err
		}
		result = append(result, RunEvent{
			ThreadID: event.ThreadID, RunID: event.RunID,
			EventType: "tool.started", Payload: string(payload),
		})
	}
	return result, nil
}

func projectJournalConfirmation(
	event RunEvent,
	eventType string,
	payload map[string]any,
) (*JournalEventProjection, error) {
	confirmationID := journalFirstSourceID(payload, "interrupt_id", "confirmation_id")
	prompt := journalFirstLabel(payload, "prompt")
	resolved := strings.HasSuffix(eventType, ".resolved")
	if confirmationID == "" || (!resolved && prompt == "") {
		return nil, nil
	}
	projectionType := "confirmation.requested"
	status := "pending"
	phase := "requested"
	if resolved {
		projectionType = "confirmation.resolved"
		status = "completed"
		phase = "resolved"
	}
	data := map[string]any{
		"confirmation_id":     confirmationID,
		"confirmation_type":   journalFirstIdentifier(payload, "kind", "confirmation_type"),
		"allowed_action_keys": journalStringSlice(payload["allowed_action_keys"]),
	}
	if prompt != "" {
		data["prompt"] = prompt
	}
	projection, err := newJournalProjection(
		event,
		projectionType,
		status,
		"confirmation",
		data,
		"confirmation",
		confirmationID,
		phase,
	)
	if projection != nil {
		projection.Target = prompt
	}
	return projection, err
}

func newJournalProjection(
	event RunEvent,
	eventType string,
	status string,
	payloadType string,
	data map[string]any,
	correlationKind string,
	correlationKey string,
	phase string,
) (*JournalEventProjection, error) {
	payload, err := json.Marshal(journalProjectionEnvelope{Type: payloadType, Data: data})
	if err != nil {
		return nil, fmt.Errorf("marshal journal projection payload: %w", err)
	}
	return &JournalEventProjection{
		EventType:       eventType,
		Status:          status,
		Payload:         string(payload),
		IdempotencyKey:  journalProjectionIdempotencyKey(event.RunID, correlationKind, correlationKey, phase),
		CorrelationKind: correlationKind,
		CorrelationKey:  correlationKey,
	}, nil
}

func journalActionPhase(eventType string, payload map[string]any) (string, string, string) {
	switch {
	case strings.HasSuffix(eventType, ".started"):
		return "started", "action.started", "running"
	case strings.HasSuffix(eventType, ".progress"):
		return "progress", "action.progress", "running"
	case strings.HasSuffix(eventType, ".completed"), strings.HasSuffix(eventType, ".succeeded"):
		return "terminal", "action.terminal", "completed"
	case strings.HasSuffix(eventType, ".canceled"), strings.HasSuffix(eventType, ".cancelled"):
		return "terminal", "action.terminal", "cancelled"
	case strings.HasSuffix(eventType, ".failed"):
		return "terminal", "action.terminal", journalTerminalStatus(eventType, publicString(payload["error_code"]))
	default:
		return "", "", ""
	}
}

func journalTerminalStatus(eventType, errorCode string) string {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	errorCode = strings.ToLower(strings.TrimSpace(errorCode))
	if strings.HasSuffix(eventType, ".timed_out") || domainentity.IsJournalTimeoutErrorCode(errorCode) {
		return "timed_out"
	}
	if strings.HasSuffix(eventType, ".failed") {
		return "failed"
	}
	if strings.HasSuffix(eventType, ".canceled") || strings.HasSuffix(eventType, ".cancelled") {
		return "cancelled"
	}
	return "completed"
}

func isJournalRunTerminalEvent(eventType string) bool {
	switch eventType {
	case "run.completed", "run.succeeded", "run.failed", "run.canceled", "run.cancelled", "run.timed_out":
		return true
	default:
		return false
	}
}

func journalToolOperation(toolName string) string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch {
	case strings.Contains(name, "upload"):
		return "upload"
	case strings.Contains(name, "download"):
		return "download"
	case strings.Contains(name, "search"):
		return "search"
	case strings.Contains(name, "browser"), strings.Contains(name, "browse"),
		strings.Contains(name, "navigate"), strings.Contains(name, "visit"),
		strings.Contains(name, "open_url"):
		return "browse"
	case strings.Contains(name, "skill"):
		return "use_skill"
	case strings.Contains(name, "verify"), strings.Contains(name, "validate"),
		strings.Contains(name, "check"), strings.Contains(name, "test"):
		return "verify"
	case strings.Contains(name, "terminal"), strings.Contains(name, "shell"),
		strings.Contains(name, "bash"), strings.Contains(name, "command"),
		strings.Contains(name, "execute"), strings.Contains(name, "exec"):
		return "execute"
	case strings.Contains(name, "edit"), strings.Contains(name, "patch"),
		strings.Contains(name, "replace"):
		return "edit"
	case strings.Contains(name, "write"), strings.Contains(name, "save"):
		return "write"
	case strings.Contains(name, "create"):
		return "create"
	case strings.Contains(name, "generate"), strings.Contains(name, "render"):
		return "generate"
	case strings.Contains(name, "read"):
		return "read"
	case strings.Contains(name, "inspect"), strings.Contains(name, "view"),
		strings.Contains(name, "list"), strings.Contains(name, "get"):
		return "inspect"
	default:
		return "process"
	}
}

func journalToolTarget(toolName, operation string) string {
	switch operation {
	case "search":
		return "相关资料"
	case "browse":
		return "网页"
	case "use_skill":
		return "技能"
	case "execute":
		return "命令"
	case "verify":
		return "执行结果"
	case "upload", "download":
		return "文件"
	}
	name := strings.ToLower(toolName)
	if strings.Contains(name, "document") || strings.Contains(name, "doc") {
		return "文档"
	}
	if strings.Contains(name, "file") || strings.Contains(name, "code") {
		return "文件"
	}
	return "内容"
}

func journalActionVerbs(operation string) (string, string) {
	switch operation {
	case "read":
		return "正在读取", "已读取"
	case "inspect":
		return "正在查看", "已查看"
	case "use_skill":
		return "正在使用", "已使用"
	case "search":
		return "正在搜索", "已搜索"
	case "browse":
		return "正在访问", "已访问"
	case "execute":
		return "正在执行", "已执行"
	case "create":
		return "正在创建", "已创建"
	case "write":
		return "正在写入", "已写入"
	case "edit":
		return "正在编辑", "已编辑"
	case "generate":
		return "正在生成", "已生成"
	case "verify":
		return "正在校验", "已校验"
	case "upload":
		return "正在上传", "已上传"
	case "download":
		return "正在下载", "已下载"
	default:
		return "正在处理", "已处理"
	}
}

func journalStableProjectionID(runID int64, kind, sourceKey string) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(runID, 10) + "\x00" + kind + "\x00" + sourceKey))
	return kind + "_" + hex.EncodeToString(sum[:16])
}

func journalProjectionIdempotencyKey(runID int64, kind, sourceKey, phase string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		journalProjectionProducerVersion,
		strconv.FormatInt(runID, 10),
		kind,
		sourceKey,
		phase,
	}, "\x00")))
	return "journal:" + journalProjectionProducerVersion + ":" + hex.EncodeToString(sum[:])
}

func journalFirstSourceID(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := journalSourceID(payload[key]); value != "" {
			return value
		}
	}
	return ""
}

func journalSourceID(value any) string {
	switch typed := value.(type) {
	case string:
		return publicIdentifier(typed, maxPublicIdentifierRunes)
	case float64:
		if typed > 0 && typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
	case int64:
		if typed > 0 {
			return strconv.FormatInt(typed, 10)
		}
	case int:
		if typed > 0 {
			return strconv.Itoa(typed)
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return strconv.FormatInt(parsed, 10)
		}
	}
	return ""
}

func journalFirstLabel(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := publicLabel(publicString(payload[key]), maxPublicLabelRunes); value != "" {
			return value
		}
	}
	return ""
}

func journalFirstIdentifier(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := publicIdentifier(publicString(payload[key]), maxPublicIdentifierRunes); value != "" {
			return value
		}
	}
	return ""
}

func journalControlledTarget(value any) string {
	target := publicLabel(publicString(value), maxPublicLabelRunes)
	if target == "" || strings.ContainsAny(target, "/\\") {
		return ""
	}
	return target
}

func journalStringSlice(value any) []string {
	values := publicStringSlice(value)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if cleaned := publicLabel(value, maxPublicLabelRunes); cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return result
}
