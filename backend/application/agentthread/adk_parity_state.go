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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

const (
	adkParityStateSchemaVersion = 1
	maxADKParityMessages        = 4096
	maxADKParityTodos           = 512
	maxADKParityUploads         = 512
	maxADKParityArtifacts       = 512
	maxADKParityViewedImages    = 128
	maxADKParityPromotedTools   = 512
	maxADKParitySkills          = 256
	maxADKParityInterrupts      = 64
	maxADKParityJournalBindings = 4096
	maxADKParityRunPathDepth    = 64
	maxADKParityLabelRunes      = 1024
	maxADKParityContentBytes    = 256 * 1024
	maxADKParityStateBytes      = 8 << 20
)

type ADKParityState struct {
	SchemaVersion int                             `json:"schema_version"`
	Revision      int64                           `json:"revision"`
	SpaceID       int64                           `json:"space_id"`
	ThreadID      int64                           `json:"thread_id"`
	LastRunID     int64                           `json:"last_run_id"`
	Messages      []ADKParityMessage              `json:"messages"`
	Summary       *ADKParitySummaryBoundary       `json:"summary,omitempty"`
	Title         string                          `json:"title,omitempty"`
	Todos         []ADKParityTodo                 `json:"todos"`
	Workspace     ADKParityWorkspace              `json:"workspace"`
	Uploads       []ADKParityUpload               `json:"uploaded_files"`
	Artifacts     []ADKParityArtifact             `json:"artifacts"`
	ViewedImages  map[string]ADKParityViewedImage `json:"viewed_images"`
	PromotedTools *ADKParityPromotedTools         `json:"promoted,omitempty"`
	ActiveSkills  []ADKParitySkill                `json:"active_skills"`
	Interrupts    []ADKParityInterrupt            `json:"interrupts"`
	Completion    *ADKParityCompletion            `json:"completion,omitempty"`
}

func validateADKParityStateSnapshot(state *ADKParityState) error {
	if state == nil {
		return fmt.Errorf("eino adk parity state is required")
	}
	runID := state.LastRunID
	if runID <= 0 {
		return fmt.Errorf("eino adk parity state last run id is required")
	}
	if _, err := NewADKParityStateTracker(&RunSummary{
		RunID: runID, ThreadID: state.ThreadID, SpaceID: state.SpaceID, CreatorID: 1,
	}, state); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal eino adk parity state: %w", err)
	}
	if len(raw) > maxADKParityStateBytes {
		return fmt.Errorf("eino adk parity state exceeds %d bytes", maxADKParityStateBytes)
	}
	return nil
}

type ADKParityMessage struct {
	ID        string `json:"id,omitempty"`
	RunID     int64  `json:"run_id,omitempty"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"created_at,omitempty"`
}

type ADKParitySummaryBoundary struct {
	Digest               string `json:"digest"`
	OriginalMessageCount int    `json:"original_message_count"`
	ActiveMessageCount   int    `json:"active_message_count"`
	CreatedAt            int64  `json:"created_at,omitempty"`
}

type ADKParityTodo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	ActiveForm  string `json:"active_form,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

type ADKParityWorkspace struct {
	SpaceID       int64  `json:"space_id"`
	ThreadID      int64  `json:"thread_id"`
	Identity      string `json:"identity"`
	SandboxID     string `json:"sandbox_id,omitempty"`
	WorkspacePath string `json:"workspace_path"`
	UploadsPath   string `json:"uploads_path"`
	OutputsPath   string `json:"outputs_path"`
}

type ADKParityUpload struct {
	FileID      int64  `json:"file_id,omitempty"`
	FileName    string `json:"file_name"`
	VirtualPath string `json:"virtual_path"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
}

type ADKParityArtifact struct {
	ArtifactID   int64  `json:"artifact_id,omitempty"`
	FileID       int64  `json:"file_id,omitempty"`
	RunID        int64  `json:"run_id,omitempty"`
	Title        string `json:"title"`
	ArtifactType string `json:"artifact_type,omitempty"`
	VirtualPath  string `json:"virtual_path"`
	ContentType  string `json:"content_type,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	PreviewMode  string `json:"preview_mode,omitempty"`
	ScanStatus   string `json:"scan_status,omitempty"`
	CreatedAt    int64  `json:"created_at,omitempty"`
}

type ADKParityViewedImage struct {
	VirtualPath string `json:"virtual_path"`
	ContentType string `json:"content_type"`
	Digest      string `json:"digest,omitempty"`
}

type ADKParityPromotedTools struct {
	CatalogHash string   `json:"catalog_hash"`
	Names       []string `json:"names"`
}

type ADKParitySkill struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ADKParityInterrupt struct {
	ID          string `json:"id"`
	Address     string `json:"address,omitempty"`
	IsRootCause bool   `json:"is_root_cause,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`
}

type ADKParityCompletion struct {
	RunID       int64  `json:"run_id,omitempty"`
	Status      string `json:"status"`
	Reason      string `json:"reason,omitempty"`
	CompletedAt int64  `json:"completed_at,omitempty"`
}

type ADKParityStateTracker struct {
	mu    sync.RWMutex
	state ADKParityState
	// Projection-only correlation state. It must never enter ADK checkpoints.
	journalToolPlanTasks map[string]string
	journalToolCallIDs   map[string]string
}

type adkParityStateContextKey struct{}

func withADKParityStateTracker(
	ctx context.Context,
	tracker *ADKParityStateTracker,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if tracker == nil {
		return ctx
	}
	return context.WithValue(ctx, adkParityStateContextKey{}, tracker)
}

func adkParityStateTrackerFromContext(ctx context.Context) *ADKParityStateTracker {
	if ctx == nil {
		return nil
	}
	tracker, _ := ctx.Value(adkParityStateContextKey{}).(*ADKParityStateTracker)
	return tracker
}

func activeADKPlanTaskIDFromContext(ctx context.Context) string {
	tracker := adkParityStateTrackerFromContext(ctx)
	if tracker == nil {
		return ""
	}
	activeID := ""
	for _, todo := range tracker.Snapshot().Todos {
		if strings.ToLower(strings.TrimSpace(todo.Status)) != "in_progress" {
			continue
		}
		if activeID != "" {
			return ""
		}
		activeID = strings.TrimSpace(todo.ID)
	}
	return activeID
}

func bindADKJournalToolPlanTasks(
	ctx context.Context,
	toolCallIDs []string,
	planTaskID string,
) bool {
	return bindADKJournalScopedToolPlanTasks(
		ctx,
		"",
		nil,
		toolCallIDs,
		planTaskID,
	)
}

func bindADKJournalScopedToolPlanTasks(
	ctx context.Context,
	agentName string,
	runPath []string,
	toolCallIDs []string,
	planTaskID string,
) bool {
	if len(toolCallIDs) == 0 {
		return true
	}
	tracker := adkParityStateTrackerFromContext(ctx)
	planTaskID = strings.TrimSpace(planTaskID)
	if tracker == nil ||
		!isADKParityLabel(planTaskID, maxADKParityLabelRunes) {
		return false
	}
	next := make(map[string]string, len(toolCallIDs))
	nextToolCallIDs := make(map[string]string, len(toolCallIDs))
	for _, toolCallID := range toolCallIDs {
		toolCallID = strings.TrimSpace(toolCallID)
		bindingKey := adkJournalToolBindingKey(agentName, runPath, toolCallID)
		if bindingKey == "" {
			return false
		}
		next[bindingKey] = planTaskID
		nextToolCallIDs[bindingKey] = toolCallID
	}
	if len(next) > maxADKParityJournalBindings {
		return false
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	additional := 0
	for toolCallID, nextPlanTaskID := range next {
		currentPlanTaskID, exists := tracker.journalToolPlanTasks[toolCallID]
		if exists && currentPlanTaskID != nextPlanTaskID {
			return false
		}
		if currentToolCallID, exists := tracker.journalToolCallIDs[toolCallID]; exists && currentToolCallID != nextToolCallIDs[toolCallID] {
			return false
		}
		if !exists {
			additional++
		}
	}
	if len(tracker.journalToolPlanTasks)+additional > maxADKParityJournalBindings {
		return false
	}
	if tracker.journalToolPlanTasks == nil {
		tracker.journalToolPlanTasks = make(map[string]string, len(next))
	}
	if tracker.journalToolCallIDs == nil {
		tracker.journalToolCallIDs = make(map[string]string, len(next))
	}
	for bindingKey, nextPlanTaskID := range next {
		tracker.journalToolPlanTasks[bindingKey] = nextPlanTaskID
		tracker.journalToolCallIDs[bindingKey] = nextToolCallIDs[bindingKey]
	}
	return true
}

func releaseADKJournalToolPlanTask(ctx context.Context, toolCallID string) {
	tracker := adkParityStateTrackerFromContext(ctx)
	toolCallID = strings.TrimSpace(toolCallID)
	if tracker == nil || toolCallID == "" ||
		len(toolCallID) > maxADKParityLabelRunes*2+32 {
		return
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if _, exists := tracker.journalToolPlanTasks[toolCallID]; !exists {
		return
	}
	delete(tracker.journalToolPlanTasks, toolCallID)
	delete(tracker.journalToolCallIDs, toolCallID)
}

func boundADKJournalToolPlanTaskIDFromContext(
	ctx context.Context,
	toolCallID string,
) string {
	_, planTaskID, found, _ := resolveADKJournalToolBindingFromContext(
		ctx,
		toolCallID,
	)
	if !found {
		return ""
	}
	return planTaskID
}

func boundADKJournalScopedToolPlanTaskIDFromContext(
	ctx context.Context,
	agentName string,
	runPath []string,
	toolCallID string,
) string {
	tracker := adkParityStateTrackerFromContext(ctx)
	bindingKey := adkJournalToolBindingKey(agentName, runPath, toolCallID)
	if tracker == nil || bindingKey == "" {
		return ""
	}
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()
	return tracker.journalToolPlanTasks[bindingKey]
}

func resolveADKJournalToolBindingFromContext(
	ctx context.Context,
	toolCallID string,
) (bindingKey, planTaskID string, found, ambiguous bool) {
	tracker := adkParityStateTrackerFromContext(ctx)
	toolCallID = strings.TrimSpace(toolCallID)
	if tracker == nil ||
		!isADKParityLabel(toolCallID, maxADKParityLabelRunes) {
		return "", "", false, false
	}
	tracker.mu.RLock()
	defer tracker.mu.RUnlock()
	for candidateKey, candidateToolCallID := range tracker.journalToolCallIDs {
		if candidateToolCallID != toolCallID {
			continue
		}
		if found {
			return "", "", false, true
		}
		bindingKey = candidateKey
		planTaskID = tracker.journalToolPlanTasks[candidateKey]
		found = true
	}
	return bindingKey, planTaskID, found, false
}

func adkJournalToolBindingKey(
	agentName string,
	runPath []string,
	toolCallID string,
) string {
	agentName = strings.TrimSpace(agentName)
	toolCallID = strings.TrimSpace(toolCallID)
	if !isADKParityLabel(toolCallID, maxADKParityLabelRunes) {
		return ""
	}
	if agentName == "" && len(runPath) == 0 {
		return toolCallID
	}
	if agentName != "" &&
		!isADKParityLabel(agentName, maxADKParityLabelRunes) {
		return ""
	}
	if len(runPath) > maxADKParityRunPathDepth {
		return ""
	}
	normalizedRunPath := make([]string, 0, len(runPath))
	for _, step := range runPath {
		step = strings.TrimSpace(step)
		if !isADKParityLabel(step, maxADKParityLabelRunes) {
			return ""
		}
		normalizedRunPath = append(normalizedRunPath, step)
	}
	encoded, err := json.Marshal(struct {
		AgentName  string   `json:"agent_name"`
		RunPath    []string `json:"run_path"`
		ToolCallID string   `json:"tool_call_id"`
	}{
		AgentName: agentName, RunPath: normalizedRunPath, ToolCallID: toolCallID,
	})
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("scope:%x", digest)
}

func NewADKParityStateTracker(
	run *RunSummary,
	seed *ADKParityState,
) (*ADKParityStateTracker, error) {
	if run == nil || run.RunID <= 0 || run.ThreadID <= 0 || run.SpaceID <= 0 || run.CreatorID <= 0 {
		return nil, fmt.Errorf("eino adk parity state requires run ownership")
	}
	workspace := newADKParityWorkspace(run.SpaceID, run.ThreadID)
	tracker := &ADKParityStateTracker{
		state: ADKParityState{
			SchemaVersion: adkParityStateSchemaVersion,
			SpaceID:       run.SpaceID,
			ThreadID:      run.ThreadID,
			LastRunID:     run.RunID,
			Messages:      []ADKParityMessage{},
			Todos:         []ADKParityTodo{},
			Workspace:     workspace,
			Uploads:       []ADKParityUpload{},
			Artifacts:     []ADKParityArtifact{},
			ViewedImages:  map[string]ADKParityViewedImage{},
			ActiveSkills:  []ADKParitySkill{},
			Interrupts:    []ADKParityInterrupt{},
		},
		journalToolPlanTasks: map[string]string{},
		journalToolCallIDs:   map[string]string{},
	}
	if seed == nil {
		return tracker, nil
	}
	if err := tracker.applySeed(*seed); err != nil {
		return nil, err
	}
	tracker.state.LastRunID = run.RunID
	return tracker, nil
}

func (t *ADKParityStateTracker) Snapshot() ADKParityState {
	if t == nil {
		return ADKParityState{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return cloneADKParityState(t.state)
}

func (t *ADKParityStateTracker) ReplaceMessages(
	messages []ADKParityMessage,
	summary *ADKParitySummaryBoundary,
) error {
	if len(messages) > maxADKParityMessages {
		return fmt.Errorf("eino adk parity messages exceed %d items", maxADKParityMessages)
	}
	next := make([]ADKParityMessage, 0, len(messages))
	for _, message := range messages {
		message.Role = strings.ToLower(strings.TrimSpace(message.Role))
		if message.Role == "human" {
			message.Role = "user"
		}
		if message.Role == "ai" {
			message.Role = "assistant"
		}
		if message.Role != "user" && message.Role != "assistant" {
			return fmt.Errorf("eino adk parity message role is invalid")
		}
		message.ID = strings.TrimSpace(message.ID)
		message.Content = strings.TrimSpace(message.Content)
		if message.Content == "" || len(message.Content) > maxADKParityContentBytes || hasUnsafeParityControl(message.Content, true) {
			return fmt.Errorf("eino adk parity message content is invalid")
		}
		next = append(next, message)
	}
	var normalizedSummary *ADKParitySummaryBoundary
	if summary != nil {
		copy := *summary
		copy.Digest = strings.TrimSpace(copy.Digest)
		if !isADKParityIdentifier(copy.Digest, 128) || copy.OriginalMessageCount < 0 || copy.ActiveMessageCount < 0 {
			return fmt.Errorf("eino adk parity summary boundary is invalid")
		}
		normalizedSummary = &copy
	}
	return t.mutate(func(state *ADKParityState) {
		state.Messages = next
		state.Summary = normalizedSummary
	})
}

func (t *ADKParityStateTracker) AppendMessage(message ADKParityMessage) error {
	normalized, err := normalizeADKParityMessage(message)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.state.Messages) >= maxADKParityMessages {
		return fmt.Errorf("eino adk parity messages exceed %d items", maxADKParityMessages)
	}
	if len(t.state.Messages) > 0 {
		last := t.state.Messages[len(t.state.Messages)-1]
		if last.Role == normalized.Role && last.Content == normalized.Content &&
			last.RunID == normalized.RunID {
			return nil
		}
	}
	t.state.Messages = append(t.state.Messages, normalized)
	t.state.Revision++
	return nil
}

func (t *ADKParityStateTracker) SetTitle(title string) error {
	title = strings.TrimSpace(title)
	if title != "" && !isADKParityLabel(title, maxADKParityLabelRunes) {
		return fmt.Errorf("eino adk parity title is invalid")
	}
	return t.mutate(func(state *ADKParityState) { state.Title = title })
}

func (t *ADKParityStateTracker) ReplaceTodos(todos []ADKParityTodo) error {
	if len(todos) > maxADKParityTodos {
		return fmt.Errorf("eino adk parity todos exceed %d items", maxADKParityTodos)
	}
	next := make([]ADKParityTodo, 0, len(todos))
	indexes := make(map[string]int, len(todos))
	for _, todo := range todos {
		todo.ID = strings.TrimSpace(todo.ID)
		todo.Title = strings.TrimSpace(todo.Title)
		todo.Description = strings.TrimSpace(todo.Description)
		todo.Status = strings.TrimSpace(todo.Status)
		todo.ActiveForm = strings.TrimSpace(todo.ActiveForm)
		todo.Owner = strings.TrimSpace(todo.Owner)
		if !isADKParityIdentifier(todo.ID, 128) || !isADKParityLabel(todo.Title, maxADKParityLabelRunes) ||
			len(todo.Description) > maxADKParityContentBytes || hasUnsafeParityControl(todo.Description, true) ||
			!isADKParityTodoStatus(todo.Status) || !isOptionalADKParityLabel(todo.ActiveForm) ||
			!isOptionalADKParityLabel(todo.Owner) {
			return fmt.Errorf("eino adk parity todo is invalid")
		}
		if index, ok := indexes[todo.ID]; ok {
			next[index] = todo
			continue
		}
		indexes[todo.ID] = len(next)
		next = append(next, todo)
	}
	if next == nil {
		next = []ADKParityTodo{}
	}
	return t.mutate(func(state *ADKParityState) { state.Todos = next })
}

func (t *ADKParityStateTracker) MergeWorkspace(workspace ADKParityWorkspace) error {
	workspace = normalizeADKParityWorkspace(workspace)
	if err := validateADKParityWorkspace(workspace); err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	current := t.state.Workspace
	if current.Identity != workspace.Identity || current.SpaceID != workspace.SpaceID ||
		current.ThreadID != workspace.ThreadID || current.WorkspacePath != workspace.WorkspacePath ||
		current.UploadsPath != workspace.UploadsPath || current.OutputsPath != workspace.OutputsPath {
		return fmt.Errorf("conflicting workspace identity: %s != %s", current.Identity, workspace.Identity)
	}
	if current.SandboxID != "" && workspace.SandboxID != "" && current.SandboxID != workspace.SandboxID {
		return fmt.Errorf("conflicting workspace identity: sandbox %s != %s", current.SandboxID, workspace.SandboxID)
	}
	if current.SandboxID == "" && workspace.SandboxID != "" {
		t.state.Workspace.SandboxID = workspace.SandboxID
		t.state.Revision++
	}
	return nil
}

func (t *ADKParityStateTracker) MergeUploads(uploads []ADKParityUpload) error {
	if len(uploads) > maxADKParityUploads {
		return fmt.Errorf("eino adk parity uploads exceed %d items", maxADKParityUploads)
	}
	normalized := make([]ADKParityUpload, 0, len(uploads))
	for _, upload := range uploads {
		upload.FileName = strings.TrimSpace(upload.FileName)
		upload.VirtualPath = strings.TrimSpace(upload.VirtualPath)
		upload.ContentType = strings.TrimSpace(upload.ContentType)
		if upload.FileID < 0 || upload.SizeBytes < 0 || !isADKParityLabel(upload.FileName, maxADKParityLabelRunes) ||
			!isADKParityVirtualFile(upload.VirtualPath, "/mnt/user-data/uploads") ||
			!isOptionalADKParityLabel(upload.ContentType) {
			return fmt.Errorf("eino adk parity upload is invalid")
		}
		normalized = append(normalized, upload)
	}
	if t == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	next := mergeADKParityUploads(t.state.Uploads, normalized)
	if len(next) > maxADKParityUploads {
		return fmt.Errorf("eino adk parity uploads exceed %d items", maxADKParityUploads)
	}
	t.state.Uploads = next
	t.state.Revision++
	return nil
}

func (t *ADKParityStateTracker) MergeArtifacts(artifacts []ADKParityArtifact) error {
	if len(artifacts) > maxADKParityArtifacts {
		return fmt.Errorf("eino adk parity artifacts exceed %d items", maxADKParityArtifacts)
	}
	normalized := make([]ADKParityArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		artifact.Title = strings.TrimSpace(artifact.Title)
		artifact.VirtualPath = strings.TrimSpace(artifact.VirtualPath)
		artifact.ArtifactType = strings.TrimSpace(artifact.ArtifactType)
		artifact.ContentType = strings.TrimSpace(artifact.ContentType)
		artifact.PreviewMode = strings.TrimSpace(artifact.PreviewMode)
		artifact.ScanStatus = strings.TrimSpace(artifact.ScanStatus)
		if artifact.ArtifactID < 0 || artifact.FileID < 0 || artifact.RunID < 0 || artifact.SizeBytes < 0 ||
			!isADKParityLabel(artifact.Title, maxADKParityLabelRunes) ||
			!isADKParityVirtualFile(artifact.VirtualPath, "/mnt/user-data/outputs") ||
			!isOptionalADKParityLabel(artifact.ArtifactType) || !isOptionalADKParityLabel(artifact.ContentType) ||
			!isOptionalADKParityLabel(artifact.PreviewMode) || !isOptionalADKParityLabel(artifact.ScanStatus) {
			return fmt.Errorf("eino adk parity artifact is invalid")
		}
		normalized = append(normalized, artifact)
	}
	if t == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	next := mergeADKParityArtifacts(t.state.Artifacts, normalized)
	if len(next) > maxADKParityArtifacts {
		return fmt.Errorf("eino adk parity artifacts exceed %d items", maxADKParityArtifacts)
	}
	t.state.Artifacts = next
	t.state.Revision++
	return nil
}

func (t *ADKParityStateTracker) MergeViewedImages(images map[string]ADKParityViewedImage) error {
	if images == nil {
		return nil
	}
	if len(images) > maxADKParityViewedImages {
		return fmt.Errorf("eino adk parity viewed images exceed %d items", maxADKParityViewedImages)
	}
	normalized := make(map[string]ADKParityViewedImage, len(images))
	for key, image := range images {
		key = strings.TrimSpace(key)
		image.VirtualPath = strings.TrimSpace(image.VirtualPath)
		image.ContentType = strings.TrimSpace(image.ContentType)
		image.Digest = strings.TrimSpace(image.Digest)
		if image.VirtualPath == "" {
			image.VirtualPath = key
		}
		if key != image.VirtualPath || !isADKParityVirtualFile(image.VirtualPath, "/mnt/user-data") ||
			!isADKParityLabel(image.ContentType, 255) ||
			(image.Digest != "" && !isADKParityIdentifier(image.Digest, 128)) {
			return fmt.Errorf("eino adk parity viewed image is invalid")
		}
		normalized[key] = image
	}
	if t == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(normalized) == 0 {
		t.state.ViewedImages = map[string]ADKParityViewedImage{}
		t.state.Revision++
		return nil
	}
	next := make(map[string]ADKParityViewedImage, len(t.state.ViewedImages)+len(normalized))
	for key, image := range t.state.ViewedImages {
		next[key] = image
	}
	for key, image := range normalized {
		next[key] = image
	}
	if len(next) > maxADKParityViewedImages {
		return fmt.Errorf("eino adk parity viewed images exceed %d items", maxADKParityViewedImages)
	}
	t.state.ViewedImages = next
	t.state.Revision++
	return nil
}

func (t *ADKParityStateTracker) MergePromotedTools(promoted *ADKParityPromotedTools) error {
	if promoted == nil {
		return nil
	}
	next := &ADKParityPromotedTools{CatalogHash: strings.TrimSpace(promoted.CatalogHash)}
	if !isADKParityIdentifier(next.CatalogHash, 128) || len(promoted.Names) > maxADKParityPromotedTools {
		return fmt.Errorf("eino adk parity promoted tools are invalid")
	}
	seen := make(map[string]struct{}, len(promoted.Names))
	for _, name := range promoted.Names {
		name = strings.TrimSpace(name)
		if !isADKParityIdentifier(name, 255) {
			return fmt.Errorf("eino adk parity promoted tool name is invalid")
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		next.Names = append(next.Names, name)
	}
	if t == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	merged := cloneADKParityPromotedTools(next)
	if t.state.PromotedTools != nil && t.state.PromotedTools.CatalogHash == next.CatalogHash {
		merged.Names = mergeADKParityStrings(t.state.PromotedTools.Names, next.Names)
	}
	if len(merged.Names) > maxADKParityPromotedTools {
		return fmt.Errorf("eino adk parity promoted tools exceed %d items", maxADKParityPromotedTools)
	}
	t.state.PromotedTools = merged
	t.state.Revision++
	return nil
}

func (t *ADKParityStateTracker) ReplaceActiveSkills(skills []ADKParitySkill) error {
	if len(skills) > maxADKParitySkills {
		return fmt.Errorf("eino adk parity skills exceed %d items", maxADKParitySkills)
	}
	next := make([]ADKParitySkill, 0, len(skills))
	indexes := make(map[int64]int, len(skills))
	for _, skill := range skills {
		skill.Name = strings.TrimSpace(skill.Name)
		skill.Version = strings.TrimSpace(skill.Version)
		if skill.ID <= 0 || !isADKParityLabel(skill.Name, maxADKParityLabelRunes) ||
			!isADKParityIdentifier(skill.Version, 255) {
			return fmt.Errorf("eino adk parity skill is invalid")
		}
		if index, ok := indexes[skill.ID]; ok {
			next[index] = skill
			continue
		}
		indexes[skill.ID] = len(next)
		next = append(next, skill)
	}
	if next == nil {
		next = []ADKParitySkill{}
	}
	return t.mutate(func(state *ADKParityState) { state.ActiveSkills = next })
}

func (t *ADKParityStateTracker) ReplaceInterrupts(interrupts []ADKParityInterrupt) error {
	if len(interrupts) > maxADKParityInterrupts {
		return fmt.Errorf("eino adk parity interrupts exceed %d items", maxADKParityInterrupts)
	}
	next := make([]ADKParityInterrupt, 0, len(interrupts))
	indexes := make(map[string]int, len(interrupts))
	for _, interrupt := range interrupts {
		interrupt.ID = strings.TrimSpace(interrupt.ID)
		interrupt.Address = strings.TrimSpace(interrupt.Address)
		interrupt.ParentID = strings.TrimSpace(interrupt.ParentID)
		if !isADKParityIdentifier(interrupt.ID, 255) ||
			(interrupt.Address != "" && !isADKParityLabel(interrupt.Address, maxADKParityLabelRunes)) ||
			(interrupt.ParentID != "" && !isADKParityIdentifier(interrupt.ParentID, 255)) {
			return fmt.Errorf("eino adk parity interrupt is invalid")
		}
		if index, ok := indexes[interrupt.ID]; ok {
			next[index] = interrupt
			continue
		}
		indexes[interrupt.ID] = len(next)
		next = append(next, interrupt)
	}
	if next == nil {
		next = []ADKParityInterrupt{}
	}
	return t.mutate(func(state *ADKParityState) { state.Interrupts = next })
}

func (t *ADKParityStateTracker) SetCompletion(completion ADKParityCompletion) error {
	completion.Status = strings.TrimSpace(completion.Status)
	completion.Reason = strings.TrimSpace(completion.Reason)
	switch completion.Status {
	case "succeeded", "failed", "canceled", "interrupted":
	default:
		return fmt.Errorf("eino adk parity completion status is invalid")
	}
	if completion.RunID < 0 || (completion.Reason != "" && !isADKParityIdentifier(completion.Reason, 255)) {
		return fmt.Errorf("eino adk parity completion is invalid")
	}
	return t.mutate(func(state *ADKParityState) {
		copy := completion
		if copy.RunID == 0 {
			copy.RunID = state.LastRunID
		}
		state.Completion = &copy
	})
}

func (t *ADKParityStateTracker) ClearCompletion() error {
	if t == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state.Completion == nil {
		return nil
	}
	t.state.Completion = nil
	t.state.Revision++
	return nil
}

func (t *ADKParityStateTracker) mutate(update func(*ADKParityState)) error {
	if t == nil {
		return fmt.Errorf("eino adk parity state tracker is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	update(&t.state)
	t.state.Revision++
	return nil
}

func (t *ADKParityStateTracker) applySeed(seed ADKParityState) error {
	if seed.SchemaVersion != adkParityStateSchemaVersion {
		return fmt.Errorf("unsupported eino adk parity state version: %d", seed.SchemaVersion)
	}
	if seed.Revision < 0 || seed.LastRunID <= 0 {
		return fmt.Errorf("eino adk parity state revision or last run id is invalid")
	}
	if seed.SpaceID != t.state.SpaceID || seed.ThreadID != t.state.ThreadID {
		return fmt.Errorf("eino adk parity state does not belong to the active thread")
	}
	if err := t.MergeWorkspace(seed.Workspace); err != nil {
		return err
	}
	if err := t.ReplaceMessages(seed.Messages, seed.Summary); err != nil {
		return err
	}
	if err := t.SetTitle(seed.Title); err != nil {
		return err
	}
	if err := t.ReplaceTodos(seed.Todos); err != nil {
		return err
	}
	if err := t.MergeUploads(seed.Uploads); err != nil {
		return err
	}
	if err := t.MergeArtifacts(seed.Artifacts); err != nil {
		return err
	}
	if err := t.MergeViewedImages(seed.ViewedImages); err != nil {
		return err
	}
	if err := t.MergePromotedTools(seed.PromotedTools); err != nil {
		return err
	}
	if err := t.ReplaceActiveSkills(seed.ActiveSkills); err != nil {
		return err
	}
	if err := t.ReplaceInterrupts(seed.Interrupts); err != nil {
		return err
	}
	if seed.Completion != nil {
		if err := t.SetCompletion(*seed.Completion); err != nil {
			return err
		}
	}
	t.mu.Lock()
	t.state.Revision = seed.Revision
	t.mu.Unlock()
	return nil
}

func newADKParityWorkspace(spaceID, threadID int64) ADKParityWorkspace {
	return ADKParityWorkspace{
		SpaceID:       spaceID,
		ThreadID:      threadID,
		Identity:      fmt.Sprintf("space:%d/thread:%d", spaceID, threadID),
		WorkspacePath: "/mnt/user-data/workspace",
		UploadsPath:   "/mnt/user-data/uploads",
		OutputsPath:   "/mnt/user-data/outputs",
	}
}

func normalizeADKParityWorkspace(workspace ADKParityWorkspace) ADKParityWorkspace {
	workspace.Identity = strings.TrimSpace(workspace.Identity)
	workspace.SandboxID = strings.TrimSpace(workspace.SandboxID)
	workspace.WorkspacePath = strings.TrimSpace(workspace.WorkspacePath)
	workspace.UploadsPath = strings.TrimSpace(workspace.UploadsPath)
	workspace.OutputsPath = strings.TrimSpace(workspace.OutputsPath)
	return workspace
}

func validateADKParityWorkspace(workspace ADKParityWorkspace) error {
	if workspace.SpaceID <= 0 || workspace.ThreadID <= 0 ||
		!isADKParityIdentifier(workspace.Identity, 255) ||
		(workspace.SandboxID != "" && !isADKParityIdentifier(workspace.SandboxID, 255)) ||
		workspace.WorkspacePath != "/mnt/user-data/workspace" ||
		workspace.UploadsPath != "/mnt/user-data/uploads" ||
		workspace.OutputsPath != "/mnt/user-data/outputs" {
		return fmt.Errorf("eino adk parity workspace is invalid")
	}
	return nil
}

func mergeADKParityUploads(current, updates []ADKParityUpload) []ADKParityUpload {
	result := append([]ADKParityUpload(nil), current...)
	indexes := make(map[string]int, len(result)+len(updates))
	for index, value := range result {
		indexes[value.VirtualPath] = index
	}
	for _, value := range updates {
		if index, ok := indexes[value.VirtualPath]; ok {
			result[index] = value
			continue
		}
		indexes[value.VirtualPath] = len(result)
		result = append(result, value)
	}
	return result
}

func mergeADKParityArtifacts(current, updates []ADKParityArtifact) []ADKParityArtifact {
	result := append([]ADKParityArtifact(nil), current...)
	indexes := make(map[string]int, len(result)+len(updates))
	for index, value := range result {
		indexes[value.VirtualPath] = index
	}
	for _, value := range updates {
		if index, ok := indexes[value.VirtualPath]; ok {
			result[index] = value
			continue
		}
		indexes[value.VirtualPath] = len(result)
		result = append(result, value)
	}
	return result
}

func mergeADKParityStrings(current, updates []string) []string {
	result := append([]string(nil), current...)
	seen := make(map[string]struct{}, len(result)+len(updates))
	for _, value := range result {
		seen[value] = struct{}{}
	}
	for _, value := range updates {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cloneADKParityState(state ADKParityState) ADKParityState {
	copy := state
	copy.Messages = cloneADKParitySlice(state.Messages)
	copy.Todos = cloneADKParitySlice(state.Todos)
	copy.Uploads = cloneADKParitySlice(state.Uploads)
	copy.Artifacts = cloneADKParitySlice(state.Artifacts)
	copy.ActiveSkills = cloneADKParitySlice(state.ActiveSkills)
	copy.Interrupts = cloneADKParitySlice(state.Interrupts)
	copy.ViewedImages = make(map[string]ADKParityViewedImage, len(state.ViewedImages))
	for key, value := range state.ViewedImages {
		copy.ViewedImages[key] = value
	}
	if state.Summary != nil {
		value := *state.Summary
		copy.Summary = &value
	}
	copy.PromotedTools = cloneADKParityPromotedTools(state.PromotedTools)
	if state.Completion != nil {
		value := *state.Completion
		copy.Completion = &value
	}
	return copy
}

func cloneADKParitySlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	result := make([]T, len(values))
	copy(result, values)
	return result
}

func cloneADKParityPromotedTools(value *ADKParityPromotedTools) *ADKParityPromotedTools {
	if value == nil {
		return nil
	}
	return &ADKParityPromotedTools{
		CatalogHash: value.CatalogHash,
		Names:       append([]string(nil), value.Names...),
	}
}

func isADKParityTodoStatus(status string) bool {
	switch status {
	case "pending", "in_progress", "completed", "deleted":
		return true
	default:
		return false
	}
}

func normalizeADKParityMessage(message ADKParityMessage) (ADKParityMessage, error) {
	message.Role = strings.ToLower(strings.TrimSpace(message.Role))
	if message.Role == "human" {
		message.Role = "user"
	}
	if message.Role == "ai" {
		message.Role = "assistant"
	}
	if message.Role != "user" && message.Role != "assistant" {
		return ADKParityMessage{}, fmt.Errorf("eino adk parity message role is invalid")
	}
	message.ID = strings.TrimSpace(message.ID)
	message.Content = strings.TrimSpace(message.Content)
	if message.Content == "" || len(message.Content) > maxADKParityContentBytes ||
		hasUnsafeParityControl(message.Content, true) {
		return ADKParityMessage{}, fmt.Errorf("eino adk parity message content is invalid")
	}
	return message, nil
}

func isADKParityVirtualFile(value, root string) bool {
	if value == "" || path.Clean(value) != value || value == root {
		return false
	}
	return strings.HasPrefix(value, root+"/") && !hasUnsafeParityControl(value, false)
}

func isOptionalADKParityLabel(value string) bool {
	return value == "" || isADKParityLabel(value, maxADKParityLabelRunes)
}

func isADKParityLabel(value string, maxRunes int) bool {
	value = strings.TrimSpace(value)
	return value != "" && utf8.RuneCountInString(value) <= maxRunes && !hasUnsafeParityControl(value, false)
}

func isADKParityIdentifier(value string, maxRunes int) bool {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > maxRunes || hasUnsafeParityControl(value, false) {
		return false
	}
	for _, char := range value {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || strings.ContainsRune("._:/-@", char) {
			continue
		}
		return false
	}
	return true
}

func hasUnsafeParityControl(value string, allowWhitespace bool) bool {
	for _, char := range value {
		if !unicode.IsControl(char) {
			continue
		}
		if allowWhitespace && (char == '\n' || char == '\r' || char == '\t') {
			continue
		}
		return true
	}
	return false
}
