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
	"encoding/json"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	maxPublicIdentifierRunes        = 128
	maxPublicLabelRunes             = 512
	canonicalPublicStateRuntimeType = "canonical_public_state"
)

var (
	publicIdentifierPattern   = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
	publicSensitivePattern    = regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+\S+|bearer\s+[A-Za-z0-9._~+/=-]{8,}|["']?(?:api[_-]?key|access[_-]?token|client[_-]?secret|aws[_-]?secret[_-]?access[_-]?key|secret[_-]?(?:key|token)|private[_-]?key|credential|password|(?:[a-z0-9][a-z0-9_-]*_)?(?:secret|token))["']?\s*[:=]\s*["']?\S+|(?:^|[^A-Za-z0-9])(?:sk|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{4,}|(?:https?|file|s3|oss|cos|minio)://)`)
	publicAbsolutePathPattern = regexp.MustCompile(
		`(?i)(?:^|[^A-Za-z0-9._-])(?:/(?:[^/\s]+)(?:/[^\s]*)?|[a-z]:\\[^\s]+|\\\\[^\s]+)`,
	)
)

type PublicRuntimeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type PublicRun struct {
	RunID             int64               `json:"run_id"`
	ThreadID          int64               `json:"thread_id"`
	ParentRunID       int64               `json:"parent_run_id,omitempty"`
	SpaceID           int64               `json:"space_id"`
	CreatorID         int64               `json:"creator_id"`
	AssistantID       string              `json:"assistant_id,omitempty"`
	RunKind           RunKind             `json:"run_kind"`
	Status            RunStatus           `json:"status"`
	Metadata          string              `json:"metadata"`
	StreamMode        string              `json:"stream_mode,omitempty"`
	MultitaskStrategy string              `json:"multitask_strategy,omitempty"`
	OnDisconnect      string              `json:"on_disconnect,omitempty"`
	Durability        string              `json:"durability,omitempty"`
	Error             *PublicRuntimeError `json:"error,omitempty"`
	StartedAt         int64               `json:"started_at,omitempty"`
	EndedAt           int64               `json:"ended_at,omitempty"`
	CreatedAt         int64               `json:"created_at"`
	UpdatedAt         int64               `json:"updated_at"`
}

type PublicMessageMetadata struct {
	Source      string `json:"source,omitempty"`
	SourceRunID int64  `json:"source_run_id,omitempty"`
	InterruptID string `json:"interrupt_id,omitempty"`
}

type PublicMessage struct {
	MessageID int64       `json:"message_id"`
	ThreadID  int64       `json:"thread_id"`
	RunID     int64       `json:"run_id"`
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	Metadata  string      `json:"metadata"`
	CreatedAt int64       `json:"created_at"`
}

type PublicRunEvent struct {
	EventID   int64  `json:"event_id"`
	ThreadID  int64  `json:"thread_id"`
	RunID     int64  `json:"run_id"`
	EventType string `json:"event_type"`
	Payload   string `json:"payload"`
	CreatedAt int64  `json:"created_at"`
}

type PublicRunJournalToolCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Type      string         `json:"type,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type PublicRunJournalMessage struct {
	ID               string                     `json:"id"`
	ThreadID         int64                      `json:"thread_id"`
	RunID            int64                      `json:"run_id"`
	Type             RunJournalMessageType      `json:"type"`
	Role             MessageRole                `json:"role"`
	Content          string                     `json:"content,omitempty"`
	Name             string                     `json:"name,omitempty"`
	ToolCallID       string                     `json:"tool_call_id,omitempty"`
	ToolCalls        []PublicRunJournalToolCall `json:"tool_calls,omitempty"`
	AdditionalKwargs map[string]any             `json:"additional_kwargs,omitempty"`
	ReasoningPresent bool                       `json:"reasoning_present"`
	Usage            map[string]int64           `json:"usage,omitempty"`
	CreatedAt        int64                      `json:"created_at"`
	SourceEventID    int64                      `json:"source_event_id,omitempty"`
}

type PublicCheckpoint struct {
	CheckpointID       int64          `json:"checkpoint_id"`
	ThreadID           int64          `json:"thread_id"`
	RunID              int64          `json:"run_id"`
	ParentCheckpointID int64          `json:"parent_checkpoint_id,omitempty"`
	CheckpointNS       string         `json:"checkpoint_ns,omitempty"`
	Runtime            string         `json:"runtime,omitempty"`
	InterruptIDs       []string       `json:"interrupt_ids,omitempty"`
	Values             map[string]any `json:"values,omitempty"`
	Metadata           map[string]any `json:"metadata,omitempty"`
	CreatedAt          int64          `json:"created_at"`
}

type PublicTokenUsage struct {
	UsageID      int64            `json:"usage_id"`
	ThreadID     int64            `json:"thread_id"`
	RunID        int64            `json:"run_id"`
	SpaceID      int64            `json:"space_id"`
	Source       TokenUsageSource `json:"source"`
	StepID       string           `json:"step_id,omitempty"`
	StepIndex    int32            `json:"step_index,omitempty"`
	StepName     string           `json:"step_name,omitempty"`
	ModelName    string           `json:"model_name,omitempty"`
	Provider     string           `json:"provider,omitempty"`
	InputTokens  int64            `json:"input_tokens"`
	OutputTokens int64            `json:"output_tokens"`
	TotalTokens  int64            `json:"total_tokens"`
	CostMicros   int64            `json:"cost_micros,omitempty"`
	Currency     string           `json:"currency,omitempty"`
	Estimated    bool             `json:"estimated"`
	CreatedAt    int64            `json:"created_at"`
}

type PublicArtifact struct {
	ArtifactID   int64               `json:"artifact_id"`
	SpaceID      int64               `json:"space_id"`
	ThreadID     int64               `json:"thread_id"`
	RunID        int64               `json:"run_id"`
	FileID       int64               `json:"file_id"`
	Title        string              `json:"title"`
	ArtifactType string              `json:"artifact_type"`
	VirtualPath  string              `json:"virtual_path,omitempty"`
	ContentType  string              `json:"content_type,omitempty"`
	SizeBytes    int64               `json:"size_bytes"`
	PreviewMode  ArtifactPreviewMode `json:"preview_mode"`
	Metadata     string              `json:"metadata"`
	CreatedAt    int64               `json:"created_at"`
	UpdatedAt    int64               `json:"updated_at"`
	DeletedAt    int64               `json:"deleted_at,omitempty"`
}

func ProjectPublicRun(run *RunSummary) *PublicRun {
	if run == nil {
		return nil
	}

	return &PublicRun{
		RunID:             run.RunID,
		ThreadID:          run.ThreadID,
		ParentRunID:       run.ParentRunID,
		SpaceID:           run.SpaceID,
		CreatorID:         run.CreatorID,
		AssistantID:       publicIdentifier(run.AssistantID, maxPublicIdentifierRunes),
		RunKind:           run.RunKind,
		Status:            run.Status,
		Metadata:          projectPublicRunMetadata(run.Metadata),
		StreamMode:        publicRunStreamMode(run.StreamMode),
		MultitaskStrategy: publicIdentifier(run.MultitaskStrategy, maxPublicIdentifierRunes),
		OnDisconnect:      publicIdentifier(run.OnDisconnect, maxPublicIdentifierRunes),
		Durability:        publicIdentifier(run.Durability, maxPublicIdentifierRunes),
		Error:             ProjectPublicRuntimeError(run.ErrorCode, run.ErrorMessage),
		StartedAt:         run.StartedAt,
		EndedAt:           run.EndedAt,
		CreatedAt:         run.CreatedAt,
		UpdatedAt:         run.UpdatedAt,
	}
}

func ProjectPublicRuns(runs []*RunSummary) []*PublicRun {
	result := make([]*PublicRun, 0, len(runs))
	for _, run := range runs {
		if projected := ProjectPublicRun(run); projected != nil {
			result = append(result, projected)
		}
	}
	return result
}

func ProjectPublicMessageMetadata(raw string) PublicMessageMetadata {
	payload := publicJSONObject(raw)
	return PublicMessageMetadata{
		Source:      publicIdentifier(publicString(payload["source"]), maxPublicIdentifierRunes),
		SourceRunID: publicInt64(payload["source_run_id"]),
		InterruptID: publicIdentifier(publicString(payload["interrupt_id"]), maxPublicIdentifierRunes),
	}
}

func ProjectPublicMessageMetadataJSON(raw string) string {
	return publicJSON(ProjectPublicMessageMetadata(raw))
}

func ProjectPublicMessage(message *MessageSummary) *PublicMessage {
	if message == nil || (message.Role != MessageRoleUser && message.Role != MessageRoleAssistant) {
		return nil
	}
	return &PublicMessage{
		MessageID: message.MessageID,
		ThreadID:  message.ThreadID,
		RunID:     message.RunID,
		Role:      message.Role,
		Content:   publicVisibleContent(message.Content),
		Metadata:  ProjectPublicMessageMetadataJSON(message.Metadata),
		CreatedAt: message.CreatedAt,
	}
}

func ProjectPublicMessages(messages []*MessageSummary) []*PublicMessage {
	result := make([]*PublicMessage, 0, len(messages))
	for _, message := range messages {
		if projected := ProjectPublicMessage(message); projected != nil {
			result = append(result, projected)
		}
	}
	return result
}

func ProjectPublicRunEvent(event *RunEventSummary) *PublicRunEvent {
	if event == nil {
		return nil
	}

	return &PublicRunEvent{
		EventID:   event.EventID,
		ThreadID:  event.ThreadID,
		RunID:     event.RunID,
		EventType: publicIdentifier(event.EventType, maxPublicIdentifierRunes),
		Payload:   projectPublicRunEventPayload(event.EventType, event.Payload),
		CreatedAt: event.CreatedAt,
	}
}

func ProjectPublicRunEvents(events []*RunEventSummary) []*PublicRunEvent {
	result := make([]*PublicRunEvent, 0, len(events))
	for _, event := range events {
		if projected := ProjectPublicRunEvent(event); projected != nil {
			result = append(result, projected)
		}
	}
	return result
}

func ProjectPublicRunJournalMessage(message *RunJournalMessage) *PublicRunJournalMessage {
	if message == nil {
		return nil
	}

	content := ""
	if message.Type == RunJournalMessageTypeAI || message.Type == RunJournalMessageTypeHuman {
		content = publicVisibleContent(message.Content)
	}
	result := &PublicRunJournalMessage{
		ID:               publicIdentifier(message.ID, maxPublicIdentifierRunes),
		ThreadID:         message.ThreadID,
		RunID:            message.RunID,
		Type:             message.Type,
		Role:             message.Role,
		Content:          content,
		Name:             publicIdentifier(message.Name, maxPublicIdentifierRunes),
		ToolCallID:       publicIdentifier(message.ToolCallID, maxPublicIdentifierRunes),
		AdditionalKwargs: map[string]any{},
		ReasoningPresent: publicRunJournalReasoningPresent(message.AdditionalKwargs),
		Usage:            projectPublicJournalUsage(message.Usage),
		CreatedAt:        message.CreatedAt,
		SourceEventID:    message.SourceEventID,
	}
	result.ToolCalls = make([]PublicRunJournalToolCall, 0, len(message.ToolCalls))
	for _, toolCall := range message.ToolCalls {
		name := publicIdentifier(toolCall.Name, maxPublicIdentifierRunes)
		if name == "" {
			continue
		}
		result.ToolCalls = append(result.ToolCalls, PublicRunJournalToolCall{
			ID:        publicIdentifier(toolCall.ID, maxPublicIdentifierRunes),
			Name:      name,
			Type:      publicIdentifier(toolCall.Type, maxPublicIdentifierRunes),
			Arguments: map[string]any{},
		})
	}
	return result
}

func publicRunJournalReasoningPresent(values map[string]any) bool {
	for _, key := range []string{"reasoning_content", "reasoning"} {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func ProjectPublicRunJournalMessages(messages []*RunJournalMessage) []*PublicRunJournalMessage {
	result := make([]*PublicRunJournalMessage, 0, len(messages))
	for _, message := range messages {
		if projected := ProjectPublicRunJournalMessage(message); projected != nil {
			result = append(result, projected)
		}
	}
	return result
}

func ProjectPublicRuntimeError(code, rawMessage string) *PublicRuntimeError {
	if strings.TrimSpace(code) == "" && strings.TrimSpace(rawMessage) == "" {
		return nil
	}

	publicCode := publicIdentifier(code, 64)
	if publicCode == "" {
		publicCode = "runtime_failed"
	}
	category := strings.ToLower(strings.TrimSpace(code + " " + rawMessage))
	message := "Agent run failed"
	switch {
	case strings.Contains(category, "cancel"):
		message = "Agent run was canceled"
	case strings.Contains(category, "timeout"), strings.Contains(category, "deadline"):
		message = "Agent run timed out"
	case strings.Contains(category, "guardrail"), strings.Contains(category, "policy"), strings.Contains(category, "safety"):
		message = "Agent run was blocked by policy"
	case strings.Contains(category, "checkpoint"), strings.Contains(category, "resume"):
		message = "Agent run could not be resumed"
	case strings.Contains(category, "tool"), strings.Contains(category, "mcp"):
		message = "Tool execution failed"
	case strings.Contains(category, "model"), strings.Contains(category, "provider"), strings.Contains(category, "llm"):
		message = "Model request failed"
	}

	return &PublicRuntimeError{Code: publicCode, Message: message}
}

func ProjectPublicCheckpoint(checkpoint *CheckpointSummary) *PublicCheckpoint {
	if checkpoint == nil {
		return nil
	}

	result := &PublicCheckpoint{
		CheckpointID:       checkpoint.CheckpointID,
		ThreadID:           checkpoint.ThreadID,
		RunID:              checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID,
		CheckpointNS:       publicIdentifier(checkpoint.CheckpointNS, maxPublicIdentifierRunes),
		Runtime:            publicIdentifier(checkpoint.RuntimeType, maxPublicIdentifierRunes),
		CreatedAt:          checkpoint.CreatedAt,
		Values:             map[string]any{},
		Metadata:           projectPublicCheckpointMetadata(checkpoint.Metadata),
	}

	if strings.TrimSpace(checkpoint.RuntimeType) == string(RuntimeModeEinoADK) {
		result.Runtime = string(RuntimeModeEinoADK)
		result.Values["runtime"] = string(RuntimeModeEinoADK)
		result.Values["interrupts"] = []string{}
		if envelope, err := UnmarshalADKCheckpointEnvelope([]byte(checkpoint.ChannelValues)); err == nil &&
			adkEnvelopeMatchesPublicCheckpoint(checkpoint, &envelope) {
			result.InterruptIDs = make([]string, 0, len(envelope.Interrupts))
			for key, item := range envelope.Interrupts {
				id := publicIdentifier(item.ID, maxPublicIdentifierRunes)
				if id == "" {
					id = publicIdentifier(key, maxPublicIdentifierRunes)
				}
				if id != "" {
					result.InterruptIDs = append(result.InterruptIDs, id)
				}
			}
			sort.Strings(result.InterruptIDs)
			if envelope.EnvelopeVersion == adkCheckpointEnvelopeVersion && envelope.ParityState != nil {
				result.Values = projectPublicADKParityState(envelope.ParityState, result.InterruptIDs)
			}
		}
		result.Values["interrupts"] = append([]string{}, result.InterruptIDs...)
		return result
	}
	if strings.TrimSpace(checkpoint.RuntimeType) == canonicalPublicStateRuntimeType {
		result.Runtime = canonicalPublicStateRuntimeType
		result.Values = projectCanonicalPublicStateValues(checkpoint.ChannelValues)
		return result
	}

	result.Values = projectPublicCheckpointValues(checkpoint.ChannelValues)
	result.InterruptIDs = publicStringSlice(publicJSONValue(checkpoint.PendingSends))
	return result
}

func projectCanonicalPublicStateValues(raw string) map[string]any {
	payload := publicJSONObject(raw)
	custom, ok := payload["custom"].(map[string]any)
	if !ok || custom == nil {
		return map[string]any{}
	}
	projected, ok := projectCanonicalPublicCustomValue(custom, 0)
	if !ok {
		return map[string]any{}
	}
	return map[string]any{"custom": projected}
}

func projectCanonicalPublicCustomValue(value any, depth int) (any, bool) {
	if depth > 32 {
		return nil, false
	}
	switch typed := value.(type) {
	case nil, bool, float64, json.Number:
		return typed, true
	case string:
		projected := publicCleanString(typed, 64*1024)
		if publicSensitivePattern.MatchString(projected) {
			return nil, false
		}
		return projected, true
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			projected, ok := projectCanonicalPublicCustomValue(item, depth+1)
			if ok {
				result = append(result, projected)
			}
		}
		return result, true
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			key = publicCleanString(key, 128)
			if key == "" || canonicalPublicStateUnsafeKey(key) {
				continue
			}
			projected, ok := projectCanonicalPublicCustomValue(item, depth+1)
			if ok {
				result[key] = projected
			}
		}
		return result, true
	default:
		return nil, false
	}
}

func canonicalPublicStateUnsafeKey(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range []string{
		"checkpoint_bytes", "channel_versions", "pending_sends", "provider_body",
		"provider_payload", "raw_provider", "tool_arguments", "tool_args", "tool_result",
		"hidden_config", "worker_id", "lease_owner", "lease_token", "idempotency_key",
		"error_chain", "stack_trace", "traceback", "api_key", "credential", "secret",
		"access_token", "authorization", "password",
	} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func adkEnvelopeMatchesPublicCheckpoint(
	checkpoint *CheckpointSummary,
	envelope *ADKCheckpointEnvelope,
) bool {
	if checkpoint == nil || envelope == nil {
		return false
	}
	if checkpoint.EnvelopeVersion != 0 && int(checkpoint.EnvelopeVersion) != envelope.EnvelopeVersion {
		return false
	}
	if runtimeKey := strings.TrimSpace(checkpoint.RuntimeKey); runtimeKey != "" && runtimeKey != envelope.RuntimeKey {
		return false
	}
	if envelope.EnvelopeVersion != adkCheckpointEnvelopeVersion {
		return true
	}
	return envelope.ParityState != nil &&
		envelope.ParityState.ThreadID == checkpoint.ThreadID &&
		envelope.ParityState.LastRunID == checkpoint.RunID
}

func projectPublicADKParityState(state *ADKParityState, interruptIDs []string) map[string]any {
	result := map[string]any{
		"runtime":        string(RuntimeModeEinoADK),
		"messages":       []map[string]any{},
		"todos":          []map[string]any{},
		"uploaded_files": []map[string]any{},
		"artifacts":      []map[string]any{},
		"viewed_images":  map[string]any{},
		"active_skills":  []map[string]any{},
		"interrupts":     append([]string{}, interruptIDs...),
	}
	if state == nil {
		return result
	}

	if title := publicLabel(state.Title, maxPublicLabelRunes); title != "" {
		result["title"] = title
	}
	messages := make([]map[string]any, 0, len(state.Messages))
	for _, message := range state.Messages {
		role := strings.ToLower(publicIdentifier(message.Role, 32))
		if role != "user" && role != "assistant" {
			continue
		}
		content := publicVisibleContent(stripADKUploadedFilesContext(message.Content))
		if content == "" {
			continue
		}
		projected := map[string]any{"role": role, "content": content}
		if id := publicIdentifier(message.ID, maxPublicIdentifierRunes); id != "" {
			projected["id"] = id
		}
		if message.RunID > 0 {
			projected["run_id"] = strconv.FormatInt(message.RunID, 10)
		}
		if message.CreatedAt > 0 {
			projected["created_at"] = message.CreatedAt
		}
		messages = append(messages, projected)
	}
	result["messages"] = messages

	todos := make([]map[string]any, 0, len(state.Todos))
	for _, todo := range state.Todos {
		projected := map[string]any{}
		if id := publicIdentifier(todo.ID, maxPublicIdentifierRunes); id != "" {
			projected["id"] = id
		}
		if title := publicLabel(todo.Title, maxPublicLabelRunes); title != "" {
			projected["title"] = title
		}
		if description := publicLabel(todo.Description, maxPublicLabelRunes); description != "" {
			projected["description"] = description
		}
		if status := publicIdentifier(todo.Status, 32); status != "" {
			projected["status"] = status
		}
		if activeForm := publicLabel(todo.ActiveForm, maxPublicLabelRunes); activeForm != "" {
			projected["active_form"] = activeForm
		}
		if owner := publicIdentifier(todo.Owner, maxPublicIdentifierRunes); owner != "" {
			projected["owner"] = owner
		}
		if len(projected) > 0 {
			todos = append(todos, projected)
		}
	}
	result["todos"] = todos

	uploads := make([]map[string]any, 0, len(state.Uploads))
	for _, upload := range state.Uploads {
		projected := map[string]any{}
		if upload.FileID > 0 {
			projected["file_id"] = upload.FileID
		}
		if name := publicLabel(upload.FileName, maxPublicLabelRunes); name != "" {
			projected["file_name"] = name
		}
		if virtualPath := publicVirtualPath(upload.VirtualPath, "/mnt/user-data/uploads"); virtualPath != "" {
			projected["virtual_path"] = virtualPath
		}
		if contentType := publicLabel(upload.ContentType, 255); contentType != "" {
			projected["content_type"] = contentType
		}
		if upload.SizeBytes >= 0 {
			projected["size_bytes"] = upload.SizeBytes
		}
		if upload.CreatedAt > 0 {
			projected["created_at"] = upload.CreatedAt
		}
		if len(projected) > 0 {
			uploads = append(uploads, projected)
		}
	}
	result["uploaded_files"] = uploads

	artifacts := make([]map[string]any, 0, len(state.Artifacts))
	for _, artifact := range state.Artifacts {
		projected := map[string]any{}
		if artifact.ArtifactID > 0 {
			projected["artifact_id"] = artifact.ArtifactID
		}
		if artifact.FileID > 0 {
			projected["file_id"] = artifact.FileID
		}
		if artifact.RunID > 0 {
			projected["run_id"] = artifact.RunID
		}
		if title := publicLabel(artifact.Title, maxPublicLabelRunes); title != "" {
			projected["title"] = title
		}
		if artifactType := publicIdentifier(artifact.ArtifactType, maxPublicIdentifierRunes); artifactType != "" {
			projected["artifact_type"] = artifactType
		}
		if virtualPath := publicVirtualPath(artifact.VirtualPath, "/mnt/user-data/outputs"); virtualPath != "" {
			projected["virtual_path"] = virtualPath
		}
		if contentType := publicLabel(artifact.ContentType, 255); contentType != "" {
			projected["content_type"] = contentType
		}
		if artifact.SizeBytes >= 0 {
			projected["size_bytes"] = artifact.SizeBytes
		}
		if previewMode := publicIdentifier(artifact.PreviewMode, maxPublicIdentifierRunes); previewMode != "" {
			projected["preview_mode"] = previewMode
		}
		if scanStatus := publicIdentifier(artifact.ScanStatus, maxPublicIdentifierRunes); scanStatus != "" {
			projected["scan_status"] = scanStatus
		}
		if artifact.CreatedAt > 0 {
			projected["created_at"] = artifact.CreatedAt
		}
		if len(projected) > 0 {
			artifacts = append(artifacts, projected)
		}
	}
	result["artifacts"] = artifacts

	viewedImages := make(map[string]any, len(state.ViewedImages))
	for key, image := range state.ViewedImages {
		virtualPath := publicVirtualPath(image.VirtualPath, "/mnt/user-data")
		if virtualPath == "" || publicVirtualPath(key, "/mnt/user-data") != virtualPath {
			continue
		}
		projected := map[string]any{"virtual_path": virtualPath}
		if contentType := publicLabel(image.ContentType, 255); contentType != "" {
			projected["content_type"] = contentType
		}
		if digest := publicIdentifier(image.Digest, maxPublicIdentifierRunes); digest != "" {
			projected["digest"] = digest
		}
		viewedImages[virtualPath] = projected
	}
	result["viewed_images"] = viewedImages

	if state.PromotedTools != nil {
		names := make([]string, 0, len(state.PromotedTools.Names))
		for _, name := range state.PromotedTools.Names {
			if value := publicIdentifier(name, 255); value != "" {
				names = append(names, value)
			}
		}
		if catalogHash := publicIdentifier(state.PromotedTools.CatalogHash, maxPublicIdentifierRunes); catalogHash != "" {
			result["promoted"] = map[string]any{"catalog_hash": catalogHash, "names": names}
		}
	}

	skills := make([]map[string]any, 0, len(state.ActiveSkills))
	for _, skill := range state.ActiveSkills {
		name := publicLabel(skill.Name, maxPublicLabelRunes)
		version := publicIdentifier(skill.Version, 255)
		if skill.ID <= 0 || name == "" || version == "" {
			continue
		}
		skills = append(skills, map[string]any{"id": skill.ID, "name": name, "version": version})
	}
	result["active_skills"] = skills

	if state.Summary != nil {
		if digest := publicIdentifier(state.Summary.Digest, maxPublicIdentifierRunes); digest != "" {
			result["summary"] = map[string]any{
				"digest": digest, "original_message_count": state.Summary.OriginalMessageCount,
				"active_message_count": state.Summary.ActiveMessageCount, "created_at": state.Summary.CreatedAt,
			}
		}
	}
	if state.Completion != nil {
		completion := map[string]any{}
		if state.Completion.RunID > 0 {
			completion["run_id"] = state.Completion.RunID
		}
		if status := publicIdentifier(state.Completion.Status, 32); status != "" {
			completion["status"] = status
		}
		if reason := publicIdentifier(state.Completion.Reason, 255); reason != "" {
			completion["reason"] = reason
		}
		if state.Completion.CompletedAt > 0 {
			completion["completed_at"] = state.Completion.CompletedAt
		}
		if len(completion) > 0 {
			result["completion"] = completion
		}
	}

	return result
}

func ProjectPublicCheckpoints(checkpoints []*CheckpointSummary) []*PublicCheckpoint {
	result := make([]*PublicCheckpoint, 0, len(checkpoints))
	for _, checkpoint := range checkpoints {
		if projected := ProjectPublicCheckpoint(checkpoint); projected != nil {
			result = append(result, projected)
		}
	}
	return result
}

func ProjectPublicTokenUsage(usage *TokenUsageSummary) *PublicTokenUsage {
	if usage == nil {
		return nil
	}

	return &PublicTokenUsage{
		UsageID:      usage.UsageID,
		ThreadID:     usage.ThreadID,
		RunID:        usage.RunID,
		SpaceID:      usage.SpaceID,
		Source:       usage.Source,
		StepID:       publicIdentifier(usage.StepID, maxPublicIdentifierRunes),
		StepIndex:    usage.StepIndex,
		StepName:     publicLabel(usage.StepName, maxPublicLabelRunes),
		ModelName:    publicLabel(usage.ModelName, maxPublicLabelRunes),
		Provider:     publicIdentifier(usage.Provider, maxPublicIdentifierRunes),
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
		CostMicros:   usage.CostMicros,
		Currency:     publicIdentifier(usage.Currency, 16),
		Estimated:    usage.Estimated,
		CreatedAt:    usage.CreatedAt,
	}
}

func ProjectPublicTokenUsages(usages []*TokenUsageSummary) []*PublicTokenUsage {
	result := make([]*PublicTokenUsage, 0, len(usages))
	for _, usage := range usages {
		if projected := ProjectPublicTokenUsage(usage); projected != nil {
			result = append(result, projected)
		}
	}
	return result
}

func ProjectPublicArtifact(artifact *ArtifactSummary) *PublicArtifact {
	if artifact == nil {
		return nil
	}

	return &PublicArtifact{
		ArtifactID:   artifact.ArtifactID,
		SpaceID:      artifact.SpaceID,
		ThreadID:     artifact.ThreadID,
		RunID:        artifact.RunID,
		FileID:       artifact.FileID,
		Title:        publicLabel(artifact.Title, maxPublicLabelRunes),
		ArtifactType: publicIdentifier(artifact.ArtifactType, maxPublicIdentifierRunes),
		VirtualPath:  publicArtifactVirtualPath(artifact.VirtualPath),
		ContentType:  publicLabel(artifact.ContentType, maxPublicIdentifierRunes),
		SizeBytes:    artifact.SizeBytes,
		PreviewMode:  artifact.PreviewMode,
		Metadata:     projectPublicArtifactMetadata(artifact.Metadata),
		CreatedAt:    artifact.CreatedAt,
		UpdatedAt:    artifact.UpdatedAt,
		DeletedAt:    artifact.DeletedAt,
	}
}

func ProjectPublicArtifacts(artifacts []*ArtifactSummary) []*PublicArtifact {
	result := make([]*PublicArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		if projected := ProjectPublicArtifact(artifact); projected != nil {
			result = append(result, projected)
		}
	}
	return result
}

func projectPublicRunMetadata(raw string) string {
	source := publicJSONObject(raw)
	result := map[string]any{}
	copyPublicIdentifier(source, result, "source")
	copyPublicInt64(source, result, "source_run_id")
	copyPublicInt64(source, result, "requested_at")
	copyPublicInt64(source, result, "parent_run_id")

	if rawSubagent, ok := source["subagent"].(map[string]any); ok {
		name := publicLabel(publicString(rawSubagent["name"]), maxPublicLabelRunes)
		if name != "" {
			result["subagent"] = map[string]any{"name": name}
		}
	}

	return publicJSON(result)
}

func projectPublicRunEventPayload(eventType, raw string) string {
	payload := publicJSONObject(raw)
	result := map[string]any{}
	eventType = strings.ToLower(strings.TrimSpace(eventType))

	switch {
	case eventType == "message.completed":
		result["redacted"] = true
		copyPublicIdentifier(payload, result, "role")
		copyPublicVisibleContent(payload, result, "content")
		copyPublicIdentifier(payload, result, "finish_reason")
		copyPublicIdentifier(payload, result, "status")
		copyPublicMessageToolCalls(payload, result)
	case eventType == "llm.token":
		copyPublicIdentifier(payload, result, "node")
		if chunk, ok := payload["chunk"].(map[string]any); ok {
			projected := map[string]any{}
			copyPublicVisibleContent(chunk, projected, "content")
			copyPublicIdentifier(chunk, projected, "role")
			if len(projected) > 0 {
				result["chunk"] = projected
			}
		}
	case eventType == adkProviderCapabilityDowngradedEventType:
		copyPublicIdentifier(payload, result, "schema")
		copyPublicStringSlice(payload, result, "capabilities")
		copyPublicBool(payload, result, "requested_thinking_enabled")
		copyPublicBool(payload, result, "effective_thinking_enabled")
		copyPublicIdentifier(payload, result, "requested_reasoning_effort")
		copyPublicIdentifier(payload, result, "effective_reasoning_effort")
	case eventType == "agent.output", strings.HasPrefix(eventType, "model."):
		result["redacted"] = true
		copyPublicIdentifier(payload, result, "role")
		copyPublicIdentifier(payload, result, "finish_reason")
		copyPublicIdentifier(payload, result, "status")
		copyPublicMessageToolCalls(payload, result)
	case strings.HasPrefix(eventType, "tool."):
		result["redacted"] = true
		copyPublicIdentifier(payload, result, "role")
		copyPublicIdentifier(payload, result, "tool_name")
		copyPublicIdentifier(payload, result, "tool_call_id")
		copyPublicIdentifier(payload, result, "invocation_id")
		copyPublicIdentifier(payload, result, "step_id")
		copyPublicIdentifier(payload, result, "plan_task_id")
		copyPublicIdentifier(payload, result, "progress_revision")
		copyPublicLabel(payload, result, "journal_target")
		copyPublicIdentifier(payload, result, "status")
		copyPublicIdentifier(payload, result, "error_code")
		copyPublicBool(payload, result, "arguments_present")
		copyPublicBool(payload, result, "result_present")
		if hasAnyPublicValue(payload, "arguments", "args", "input") {
			result["arguments_present"] = true
		}
		if hasAnyPublicValue(payload, "result", "content", "output") {
			result["result_present"] = true
		}
	case eventType == "run.interrupted":
		if interrupts := projectPublicHumanInteractionInterrupts(payload["interrupts"]); len(interrupts) > 0 {
			result["interrupts"] = map[string]any{"items": interrupts}
		}
		if prompt := projectPublicHumanInteractionPrompt(payload["human_interaction"]); len(prompt) > 0 {
			result["human_interaction"] = prompt
		}
		if prompts := projectPublicHumanInteractionPrompts(payload["human_interactions"]); len(prompts) > 0 {
			result["human_interactions"] = prompts
		}
	case strings.HasPrefix(eventType, "run."):
		copyPublicIdentifier(payload, result, "status")
		copyPublicIdentifier(payload, result, "reason")
		copyPublicIdentifier(payload, result, "error_code")
		copyPublicInt64(payload, result, "attempt")
	case strings.HasPrefix(eventType, "step."), strings.HasPrefix(eventType, "plan.task."), strings.HasPrefix(eventType, "todo."):
		copyPublicIdentifier(payload, result, "step_id")
		copyPublicIdentifier(payload, result, "plan_task_id")
		copyPublicIdentifier(payload, result, "step_name")
		copyPublicInt64(payload, result, "step_index")
		copyPublicIdentifier(payload, result, "status")
		copyPublicLabel(payload, result, "title")
		copyPublicLabel(payload, result, "name")
		copyPublicLabel(payload, result, "subject")
		copyPublicLabel(payload, result, "active_form")
		copyPublicLabel(payload, result, "description")
		copyPublicIdentifier(payload, result, "error_code")
	case strings.HasPrefix(eventType, "node."):
		copyPublicIdentifier(payload, result, "node")
		copyPublicIdentifier(payload, result, "step_id")
		copyPublicIdentifier(payload, result, "status")
		if delta, ok := payload["delta"].(map[string]any); ok {
			if projected := projectPublicNodeUpdate(delta); len(projected) > 0 {
				result["delta"] = projected
			}
		}
		if update, ok := payload["update"].(map[string]any); ok {
			if projected := projectPublicNodeUpdate(update); len(projected) > 0 {
				result["update"] = projected
			}
		}
	case strings.HasPrefix(eventType, "subagent."), strings.HasPrefix(eventType, "task."):
		copyPublicInt64(payload, result, "subagent_run_id")
		copyPublicInt64(payload, result, "child_run_id")
		copyPublicInt64(payload, result, "parent_run_id")
		copyPublicIdentifier(payload, result, "status")
		copyPublicLabel(payload, result, "name")
		copyPublicLabel(payload, result, "agent_name")
		copyPublicIdentifier(payload, result, "error_code")
		if rawSubagent, ok := payload["subagent"].(map[string]any); ok {
			projected := map[string]any{}
			copyPublicLabel(rawSubagent, projected, "name")
			copyPublicIdentifier(rawSubagent, projected, "step_id")
			if len(projected) > 0 {
				result["subagent"] = projected
			}
		}
		copyPublicTokenCounts(payload, result)
	case eventType == "skills.loaded", strings.HasPrefix(eventType, "skill."):
		copyPublicInt64(payload, result, "count")
		copyPublicInt64(payload, result, "skill_count")
		copyPublicStringSlice(payload, result, "skill_ids")
		copyPublicStringSlice(payload, result, "skill_names")
		copyPublicIdentifier(payload, result, "status")
		copyPublicSkills(payload, result)
	case strings.HasPrefix(eventType, "human.interaction."):
		copyPublicIdentifier(payload, result, "interrupt_id")
		copyPublicIdentifier(payload, result, "confirmation_id")
		copyPublicIdentifier(payload, result, "kind")
		copyPublicIdentifier(payload, result, "confirmation_type")
		copyPublicIdentifier(payload, result, "status")
		copyPublicLabel(payload, result, "prompt")
		copyPublicStringSlice(payload, result, "allowed_action_keys")
		copyPublicBool(payload, result, "answered")
		copyPublicBool(payload, result, "approved")
		copyPublicIdentifier(payload, result, "decision")
		copyPublicIdentifier(payload, result, "choice_id")
	case strings.HasPrefix(eventType, "artifact."), strings.HasPrefix(eventType, "file."):
		copyPublicInt64(payload, result, "artifact_id")
		copyPublicInt64(payload, result, "file_id")
		copyPublicLabel(payload, result, "title")
		copyPublicIdentifier(payload, result, "artifact_type")
		copyPublicLabel(payload, result, "content_type")
		copyPublicInt64(payload, result, "size_bytes")
		copyPublicIdentifier(payload, result, "preview_mode")
		copyPublicIdentifier(payload, result, "status")
		copyPublicIdentifier(payload, result, "scan_status")
		copyPublicIdentifier(payload, result, "collection_id")
		if artifacts := projectPublicJournalArtifacts(payload["artifacts"]); len(artifacts) > 0 {
			result["artifacts"] = artifacts
		}
	case strings.HasPrefix(eventType, "verification."):
		copyPublicIdentifier(payload, result, "verification_id")
		copyPublicLabel(payload, result, "title")
		copyPublicIdentifier(payload, result, "status")
		copyPublicIdentifier(payload, result, "error_code")
		copyPublicLabel(payload, result, "summary")
		copyPublicLabel(payload, result, "result_summary")
	case strings.HasPrefix(eventType, "memory."):
		copyPublicInt64(payload, result, "count")
		copyPublicIdentifier(payload, result, "scope")
		copyPublicIdentifier(payload, result, "status")
		copyPublicIdentifier(payload, result, "source_type")
	case strings.HasPrefix(eventType, "token."), strings.HasPrefix(eventType, "usage."):
		copyPublicTokenCounts(payload, result)
	case strings.HasPrefix(eventType, "guardrail."):
		copyPublicIdentifier(payload, result, "action")
		copyPublicIdentifier(payload, result, "reason_code")
		copyPublicIdentifier(payload, result, "status")
		copyPublicIdentifier(payload, result, "fail_mode")
		copyPublicStringSlice(payload, result, "rule_ids")
	case strings.HasPrefix(eventType, "mcp."):
		copyPublicInt64(payload, result, "server_id")
		copyPublicIdentifier(payload, result, "tool_name")
		copyPublicIdentifier(payload, result, "invocation_id")
		copyPublicIdentifier(payload, result, "progress_revision")
		copyPublicLabel(payload, result, "journal_target")
		copyPublicIdentifier(payload, result, "status")
		copyPublicIdentifier(payload, result, "error_code")
		copyPublicInt64(payload, result, "elapsed_ms")
		copyPublicInt64(payload, result, "elapsed_millis")
		copyPublicInt64(payload, result, "output_bytes")
	default:
		return `{}`
	}

	return publicJSON(result)
}

func projectPublicCheckpointValues(raw string) map[string]any {
	payload := publicJSONObject(raw)
	result := map[string]any{}
	if title := publicLabel(publicString(payload["title"]), maxPublicLabelRunes); title != "" {
		result["title"] = title
	}
	if todos := projectPublicTodos(payload["todos"]); len(todos) > 0 {
		result["todos"] = todos
	}
	if messages := projectPublicCheckpointMessages(payload["messages"]); len(messages) > 0 {
		result["messages"] = messages
	}
	if artifacts := projectPublicCheckpointArtifacts(payload["artifacts"]); len(artifacts) > 0 {
		result["artifacts"] = artifacts
	}
	return result
}

func projectPublicCheckpointMetadata(raw string) map[string]any {
	payload := publicJSONObject(raw)
	result := map[string]any{}
	copyPublicIdentifier(payload, result, "status")
	copyPublicIdentifier(payload, result, "error_type")
	copyPublicIdentifier(payload, result, "source")
	copyPublicIdentifier(payload, result, "runtime")
	copyPublicIdentifier(payload, result, "checkpoint_phase")
	copyPublicInt64(payload, result, "step")
	return result
}

func projectPublicJournalArtifacts(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		source, ok := item.(map[string]any)
		if !ok {
			continue
		}
		projected := projectPublicCheckpointArtifacts(source)
		if len(projected) > 0 {
			result = append(result, projected)
		}
	}
	return result
}

func projectPublicCheckpointArtifacts(value any) map[string]any {
	source, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	result := map[string]any{}
	for _, key := range []string{
		"artifact_id", "file_id", "report_id", "title", "artifact_type",
		"content_type", "size_bytes", "preview_mode", "scan_status",
	} {
		value, exists := source[key]
		if !exists {
			continue
		}
		switch key {
		case "artifact_id", "file_id", "report_id":
			if id := publicInt64(value); id > 0 {
				result[key] = id
			} else if identifier := publicIdentifier(publicString(value), maxPublicIdentifierRunes); identifier != "" {
				result[key] = identifier
			}
		case "size_bytes":
			if size := publicInt64(value); size >= 0 {
				result[key] = size
			}
		case "title":
			if label := publicLabel(publicString(value), maxPublicLabelRunes); label != "" {
				result[key] = label
			}
		default:
			if identifier := publicIdentifier(publicString(value), maxPublicIdentifierRunes); identifier != "" {
				result[key] = identifier
			}
		}
	}
	return result
}

func projectPublicTodos(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		projected := map[string]any{}
		copyPublicIdentifier(raw, projected, "id")
		copyPublicIdentifier(raw, projected, "status")
		copyPublicLabel(raw, projected, "title")
		copyPublicLabel(raw, projected, "content")
		if len(projected) > 0 {
			result = append(result, projected)
		}
	}
	return result
}

func projectPublicCheckpointMessages(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role := strings.ToLower(publicIdentifier(publicString(raw["role"]), 32))
		if role == "human" {
			role = "user"
		}
		if role == "ai" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" {
			continue
		}
		content := publicVisibleContent(raw["content"])
		if content == "" {
			continue
		}
		projected := map[string]any{"role": role, "content": content}
		if id := publicIdentifier(publicString(raw["id"]), maxPublicIdentifierRunes); id != "" {
			projected["id"] = id
		}
		result = append(result, projected)
	}
	return result
}

func projectPublicNodeUpdate(source map[string]any) map[string]any {
	result := map[string]any{}
	copyPublicIdentifier(source, result, "status")
	copyPublicLabel(source, result, "title")
	copyPublicLabel(source, result, "description")
	copyPublicInt64(source, result, "progress")
	if messages := projectPublicCheckpointMessages(source["messages"]); len(messages) > 0 {
		result["messages"] = messages
	}
	return result
}

func projectPublicArtifactMetadata(raw string) string {
	payload := publicJSONObject(raw)
	status := publicIdentifier(publicString(payload["scan_status"]), 32)
	if status == "" {
		status = publicIdentifier(publicString(payload["scanStatus"]), 32)
	}
	if status == "" {
		return `{}`
	}
	return publicJSON(map[string]any{"scan_status": status})
}

func projectPublicJournalUsage(source map[string]any) map[string]int64 {
	result := map[string]int64{}
	for _, key := range []string{
		"input_tokens",
		"output_tokens",
		"total_tokens",
		"cached_tokens",
		"reasoning_tokens",
	} {
		if value := publicInt64(source[key]); value >= 0 {
			if _, exists := source[key]; exists {
				result[key] = value
			}
		}
	}
	return result
}

func publicArtifactVirtualPath(value string) string {
	return publicVirtualPath(value, "/mnt/user-data/outputs")
}

func publicVirtualPath(value string, root string) string {
	value = strings.TrimSpace(value)
	root = strings.TrimSuffix(strings.TrimSpace(root), "/")
	if value == "" || root == "" || strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	cleaned := path.Clean(value)
	if !strings.HasPrefix(cleaned, root+"/") {
		return ""
	}
	return cleaned
}

func publicRunStreamMode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if modes := publicStringSlice(publicJSONValue(value)); len(modes) > 0 {
		return publicJSON(modes)
	}
	return publicIdentifier(value, maxPublicIdentifierRunes)
}

func copyPublicMessageToolCalls(source, target map[string]any) {
	items, ok := source["tool_calls"].([]any)
	if !ok {
		return
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		function, _ := raw["function"].(map[string]any)
		name := publicIdentifier(publicString(raw["name"]), maxPublicIdentifierRunes)
		if name == "" {
			name = publicIdentifier(publicString(function["name"]), maxPublicIdentifierRunes)
		}
		if name == "" {
			continue
		}
		projected := map[string]any{"name": name, "arguments_present": hasAnyPublicValue(raw, "args", "arguments") || hasAnyPublicValue(function, "arguments")}
		if id := publicIdentifier(publicString(raw["id"]), maxPublicIdentifierRunes); id != "" {
			projected["id"] = id
		}
		result = append(result, projected)
	}
	if len(result) > 0 {
		target["tool_calls"] = result
	}
}

func projectPublicHumanInteractionInterrupts(value any) []map[string]any {
	items, _ := value.([]any)
	if container, ok := value.(map[string]any); ok {
		items, _ = container["items"].([]any)
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id := publicIdentifier(publicString(raw["id"]), maxPublicIdentifierRunes)
		info := projectPublicHumanInteractionPrompt(raw["info"])
		if id == "" || len(info) == 0 {
			continue
		}
		projected := map[string]any{"id": id, "info": info}
		result = append(result, projected)
		if len(result) == 16 {
			break
		}
	}
	return result
}

func projectPublicHumanInteractionPrompts(value any) []map[string]any {
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if projected := projectPublicHumanInteractionPrompt(item); len(projected) > 0 {
			result = append(result, projected)
		}
		if len(result) == 16 {
			break
		}
	}
	return result
}

func projectPublicHumanInteractionPrompt(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	schemaName := publicIdentifier(publicString(raw["schema"]), maxPublicIdentifierRunes)
	interactionID := publicIdentifier(publicString(raw["interaction_id"]), maxPublicIdentifierRunes)
	kind := publicIdentifier(publicString(raw["kind"]), maxPublicIdentifierRunes)
	if schemaName != humanInteractionSchema || interactionID == "" ||
		(kind != string(HumanInteractionKindClarification) && kind != string(HumanInteractionKindConfirmation)) {
		return nil
	}

	result := map[string]any{
		"schema":         schemaName,
		"interaction_id": interactionID,
		"kind":           kind,
	}
	for _, key := range []string{"title", "question", "summary"} {
		copyPublicLabel(raw, result, key)
	}
	copyPublicIdentifier(raw, result, "risk_level")
	copyPublicBool(raw, result, "required")
	copyPublicBool(raw, result, "allow_free_text")
	if choices := projectPublicHumanInteractionChoices(raw["choices"]); len(choices) > 0 {
		result["choices"] = choices
	}
	return result
}

func projectPublicHumanInteractionChoices(value any) []map[string]any {
	items, _ := value.([]any)
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		projected := map[string]any{}
		copyPublicIdentifier(raw, projected, "id")
		copyPublicLabel(raw, projected, "label")
		if len(projected) > 0 {
			result = append(result, projected)
		}
		if len(result) == 32 {
			break
		}
	}
	return result
}

func copyPublicSkills(source, target map[string]any) {
	items, ok := source["skills"].([]any)
	if !ok {
		return
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		projected := map[string]any{}
		copyPublicInt64(raw, projected, "id")
		copyPublicLabel(raw, projected, "name")
		if len(projected) > 0 {
			result = append(result, projected)
		}
	}
	if len(result) > 0 {
		target["skills"] = result
	}
}

func copyPublicTokenCounts(source, target map[string]any) {
	for _, key := range []string{"input_tokens", "output_tokens", "total_tokens", "token_count", "call_count"} {
		copyPublicInt64(source, target, key)
	}
}

func copyPublicIdentifier(source, target map[string]any, key string) {
	if value := publicIdentifier(publicString(source[key]), maxPublicIdentifierRunes); value != "" {
		target[key] = value
	}
}

func copyPublicLabel(source, target map[string]any, key string) {
	if value := publicLabel(publicString(source[key]), maxPublicLabelRunes); value != "" {
		target[key] = value
	}
}

func copyPublicVisibleContent(source, target map[string]any, key string) {
	if value := publicVisibleContent(source[key]); value != "" {
		target[key] = value
	}
}

func copyPublicInt64(source, target map[string]any, key string) {
	if value := publicInt64(source[key]); value != 0 {
		target[key] = value
	}
}

func copyPublicBool(source, target map[string]any, key string) {
	if value, ok := source[key].(bool); ok {
		target[key] = value
	}
}

func copyPublicStringSlice(source, target map[string]any, key string) {
	if values := publicStringSlice(source[key]); len(values) > 0 {
		target[key] = values
	}
}

func hasAnyPublicValue(source map[string]any, keys ...string) bool {
	for _, key := range keys {
		if value, ok := source[key]; ok && value != nil {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return true
				}
			default:
				return true
			}
		}
	}
	return false
}

func publicJSONObject(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil || result == nil {
		return map[string]any{}
	}
	return result
}

func publicJSONValue(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var result any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil
	}
	return result
}

func publicJSON(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return `{}`
	}
	return string(raw)
}

func publicIdentifier(value string, limit int) string {
	value = publicCleanString(value, limit)
	if value == "" || !publicIdentifierPattern.MatchString(value) || publicStringIsSensitive(value) {
		return ""
	}
	return value
}

func publicLabel(value string, limit int) string {
	value = publicCleanString(value, limit)
	if value == "" || publicStringIsSensitive(value) {
		return ""
	}
	return value
}

func publicStringIsSensitive(value string) bool {
	return publicSensitivePattern.MatchString(value) || publicAbsolutePathPattern.MatchString(value)
}

func publicVisibleContent(value any) string {
	content, ok := value.(string)
	if !ok {
		return ""
	}
	return publicCleanString(content, 64*1024)
}

func publicCleanString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 {
		return ""
	}
	value = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		if r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return -1
		}
		switch r {
		case '\u061c', '\u200e', '\u200f',
			'\u202a', '\u202b', '\u202c', '\u202d', '\u202e',
			'\u2066', '\u2067', '\u2068', '\u2069':
			return -1
		}
		return r
	}, value)
	runes := []rune(value)
	if len(runes) > limit {
		value = string(runes[:limit])
	}
	return strings.TrimSpace(value)
}

func publicString(value any) string {
	text, _ := value.(string)
	return text
}

func publicInt64(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case int:
		return int64(typed)
	case int64:
		return typed
	case int32:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
	}
}

func publicStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]string); ok {
			items = make([]any, 0, len(typed))
			for _, item := range typed {
				items = append(items, item)
			}
		} else {
			return nil
		}
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		candidate := publicString(item)
		if raw, ok := item.(map[string]any); ok {
			for _, key := range []string{"node", "target", "name", "id"} {
				if candidate = publicString(raw[key]); strings.TrimSpace(candidate) != "" {
					break
				}
			}
		}
		if value := publicIdentifier(candidate, maxPublicIdentifierRunes); value != "" {
			result = append(result, value)
		}
	}
	return result
}
