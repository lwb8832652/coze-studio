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

package coze

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	appagentthread "github.com/coze-dev/coze-studio/backend/application/agentthread"
)

const (
	canonicalThreadEventPageSize = int32(200)
	canonicalMaxMetadataRunes    = 4096
	canonicalMaxPublicValueRunes = 64 * 1024
	canonicalMaxExactFloatID     = float64(1<<53 - 1)
)

var (
	canonicalMetadataKeyPattern    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
	canonicalIdentifierPattern     = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)
	canonicalIDKeyPattern          = regexp.MustCompile(`(?i)(^id$|_id$)`)
	canonicalIDsKeyPattern         = regexp.MustCompile(`(?i)_ids$`)
	canonicalNumericStringPattern  = regexp.MustCompile(`^[+-]?[0-9]+$`)
	canonicalSensitiveValuePattern = regexp.MustCompile(
		`(?i)(authorization\s*:\s*bearer\s+\S+|bearer\s+[A-Za-z0-9._~+/=-]{8,}|` +
			`(?:api[_-]?key|access[_-]?token|credential|password)\s*[:=]\s*\S+|` +
			`(?:sk|ghp|github_pat|xox[baprs])[-_][A-Za-z0-9_-]{4,})`,
	)
)

type canonicalThread struct {
	ThreadID   string              `json:"thread_id"`
	CreatedAt  string              `json:"created_at"`
	UpdatedAt  string              `json:"updated_at"`
	Metadata   map[string]any      `json:"metadata"`
	Status     string              `json:"status"`
	Values     map[string]any      `json:"values"`
	Interrupts map[string]any      `json:"interrupts"`
	Coze       canonicalThreadCoze `json:"coze"`
}

type canonicalThreadCoze struct {
	ProductStatus     string `json:"product_status"`
	InitialSubmission any    `json:"initial_submission"`
	Source            string `json:"source"`
	Progress          int32  `json:"progress"`
	LastUserMessage   string `json:"last_user_message"`
	LastAgentMessage  string `json:"last_agent_message"`
	CanEdit           bool   `json:"can_edit"`
}

type canonicalRun struct {
	RunID             string           `json:"run_id"`
	ThreadID          string           `json:"thread_id"`
	AssistantID       string           `json:"assistant_id"`
	Status            string           `json:"status"`
	CreatedAt         string           `json:"created_at"`
	UpdatedAt         string           `json:"updated_at"`
	Metadata          map[string]any   `json:"metadata"`
	MultitaskStrategy string           `json:"multitask_strategy"`
	Coze              canonicalRunCoze `json:"coze"`
}

type canonicalRunCoze struct {
	MessageID         *string           `json:"message_id"`
	SubmissionMessage *canonicalMessage `json:"submission_message,omitempty"`
	AttemptKind       string            `json:"attempt_kind"`
	SourceRunID       *string           `json:"source_run_id"`
	ParentRunID       *string           `json:"parent_run_id"`
	RunKind           string            `json:"run_kind"`
	StreamModes       []string          `json:"stream_modes"`
	OnDisconnect      string            `json:"on_disconnect"`
	Durability        string            `json:"durability"`
	TerminalReason    *string           `json:"terminal_reason"`
	StartedAt         *string           `json:"started_at"`
	EndedAt           *string           `json:"ended_at"`
}

type canonicalMessage struct {
	MessageID string         `json:"message_id"`
	ThreadID  string         `json:"thread_id"`
	RunID     string         `json:"run_id"`
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt string         `json:"created_at"`
	Seq       string         `json:"seq,omitempty"`
}

type canonicalRunEvent struct {
	EventID   string         `json:"event_id"`
	ThreadID  string         `json:"thread_id"`
	RunID     string         `json:"run_id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"created_at"`
}

type canonicalCheckpoint struct {
	ThreadID      string         `json:"thread_id"`
	CheckpointNS  string         `json:"checkpoint_ns"`
	CheckpointID  string         `json:"checkpoint_id"`
	CheckpointMap map[string]any `json:"checkpoint_map"`
}

type canonicalThreadState struct {
	Values           map[string]any       `json:"values"`
	Next             []string             `json:"next"`
	Checkpoint       canonicalCheckpoint  `json:"checkpoint"`
	Metadata         map[string]any       `json:"metadata"`
	CreatedAt        string               `json:"created_at"`
	ParentCheckpoint *canonicalCheckpoint `json:"parent_checkpoint"`
	Tasks            []map[string]any     `json:"tasks"`
	Interrupts       []map[string]any     `json:"interrupts"`
}

type canonicalThreadUpdateStateResult struct {
	Checkpoint   canonicalCheckpoint `json:"checkpoint"`
	Configurable canonicalCheckpoint `json:"configurable"`
}

type canonicalThreadProjectionSnapshot struct {
	LatestRun  *appagentthread.RunSummary
	Values     map[string]any
	Interrupts map[string]any
}

type canonicalStateSource struct {
	ThreadID           int64
	CheckpointID       int64
	ParentCheckpointID int64
	CheckpointNS       string
	ParentCheckpointNS string
	Values             map[string]any
	Next               []string
	Metadata           map[string]any
	CreatedAt          int64
	Tasks              []map[string]any
	Interrupts         []map[string]any
}

// projectCanonicalThread derives SDK lifecycle fields from the repository's
// latest-top-level-Run projection. Idle needs one extra read to distinguish a
// draft/completed lifecycle from a resumable human interruption.
func projectCanonicalThread(
	ctx context.Context,
	summary *appagentthread.ThreadSummary,
) (*canonicalThread, error) {
	if summary == nil {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	snapshot, complete, err := canonicalThreadSnapshotFromProductStatus(summary.Status)
	if err != nil {
		return nil, err
	}
	if !complete {
		snapshot, err = loadCanonicalThreadProjectionSnapshot(ctx, summary.ThreadID)
		if err != nil {
			return nil, err
		}
	}
	projected, err := projectCanonicalThreadSnapshot(summary, snapshot)
	if err != nil {
		return nil, err
	}
	projected.Coze.CanEdit = canonicalThreadCanEdit(ctx, summary.CreatorID)
	return projected, nil
}

func canonicalThreadCanEdit(ctx context.Context, creatorID int64) bool {
	viewerID := workbenchViewerIDFromCtx(ctx)
	return viewerID > 0 && creatorID > 0 && viewerID == creatorID
}

func canonicalThreadSnapshotFromProductStatus(
	status appagentthread.ThreadStatus,
) (canonicalThreadProjectionSnapshot, bool, error) {
	var runStatus appagentthread.RunStatus
	switch status {
	case appagentthread.ThreadStatusIdle:
		return canonicalThreadProjectionSnapshot{}, false, nil
	case appagentthread.ThreadStatusRunning:
		runStatus = appagentthread.RunStatusRunning
	case appagentthread.ThreadStatusCompleted:
		runStatus = appagentthread.RunStatusSucceeded
	case appagentthread.ThreadStatusFailed:
		runStatus = appagentthread.RunStatusFailed
	case appagentthread.ThreadStatusCanceled:
		runStatus = appagentthread.RunStatusCanceled
	default:
		return canonicalThreadProjectionSnapshot{}, false,
			fmt.Errorf("unsupported canonical thread product status %q", status)
	}
	return canonicalThreadProjectionSnapshot{
		LatestRun: &appagentthread.RunSummary{Status: runStatus},
	}, true, nil
}

func projectCanonicalThreadSnapshot(
	summary *appagentthread.ThreadSummary,
	snapshot canonicalThreadProjectionSnapshot,
) (*canonicalThread, error) {
	if summary == nil {
		return nil, nil
	}
	if summary.ThreadID <= 0 {
		return nil, fmt.Errorf("canonical thread projection requires a positive thread id")
	}
	source, err := canonicalThreadSource(summary.Source)
	if err != nil {
		return nil, err
	}

	status, productStatus, err := canonicalThreadStatus(snapshot.LatestRun, snapshot.Interrupts)
	if err != nil {
		return nil, err
	}
	values := canonicalPublicStateValues(snapshot.Values)
	interrupts := map[string]any{}
	if status == "interrupted" {
		interrupts = canonicalSanitizeMap(snapshot.Interrupts)
	}

	return &canonicalThread{
		ThreadID:   strconv.FormatInt(summary.ThreadID, 10),
		CreatedAt:  canonicalTime(summary.CreatedAt),
		UpdatedAt:  canonicalTime(summary.UpdatedAt),
		Metadata:   canonicalEntityMetadataFromJSON(summary.Metadata, summary.Title),
		Status:     status,
		Values:     values,
		Interrupts: interrupts,
		Coze: canonicalThreadCoze{
			ProductStatus: productStatus,
			Source:        source,
			Progress:      summary.Progress,
			LastUserMessage: canonicalCleanString(
				summary.LastUserMessage, canonicalMaxPublicValueRunes,
			),
			LastAgentMessage: canonicalCleanString(
				summary.LastAgentMessage, canonicalMaxPublicValueRunes,
			),
		},
	}, nil
}

func projectCanonicalRun(summary *appagentthread.RunSummary) (*canonicalRun, error) {
	public := appagentthread.ProjectPublicRun(summary)
	if public == nil {
		return nil, nil
	}
	if public.RunID <= 0 || public.ThreadID <= 0 {
		return nil, fmt.Errorf("canonical run projection requires positive run and thread ids")
	}

	status, terminalReason, err := canonicalRunStatus(public.Status)
	if err != nil {
		return nil, err
	}
	runKind, err := canonicalRunKind(public.RunKind)
	if err != nil {
		return nil, err
	}
	rawMetadata := canonicalJSONObject(summary.Metadata)
	metadata := canonicalMetadataFromMap(rawMetadata, "")
	messageID := canonicalNestedMetadataID(rawMetadata, "_message", "message_id")
	sourceRunID := canonicalRunSourceRunID(rawMetadata)
	removeCanonicalInternalMetadata(metadata)
	if sourceRunID == nil && public.RunKind == appagentthread.RunKindSubagent && public.ParentRunID > 0 {
		formatted := strconv.FormatInt(public.ParentRunID, 10)
		sourceRunID = &formatted
	}
	multitaskStrategy := strings.TrimSpace(public.MultitaskStrategy)
	if multitaskStrategy == "" {
		multitaskStrategy = "reject"
	}
	startedAt, err := canonicalOptionalTime(public.StartedAt, "run started_at")
	if err != nil {
		return nil, err
	}
	endedAt, err := canonicalOptionalTime(public.EndedAt, "run ended_at")
	if err != nil {
		return nil, err
	}

	return &canonicalRun{
		RunID:             strconv.FormatInt(public.RunID, 10),
		ThreadID:          strconv.FormatInt(public.ThreadID, 10),
		AssistantID:       canonicalPublicAssistantID,
		Status:            status,
		CreatedAt:         canonicalTime(public.CreatedAt),
		UpdatedAt:         canonicalTime(public.UpdatedAt),
		Metadata:          metadata,
		MultitaskStrategy: multitaskStrategy,
		Coze: canonicalRunCoze{
			MessageID:      messageID,
			AttemptKind:    canonicalRunAttemptKind(public.RunKind, rawMetadata),
			SourceRunID:    sourceRunID,
			ParentRunID:    canonicalOptionalTimeID(public.ParentRunID),
			RunKind:        runKind,
			StreamModes:    canonicalRunStreamModes(public.StreamMode),
			OnDisconnect:   canonicalRunOnDisconnect(public.OnDisconnect),
			Durability:     canonicalRunDurability(public.Durability),
			TerminalReason: terminalReason,
			StartedAt:      startedAt,
			EndedAt:        endedAt,
		},
	}, nil
}

func projectCanonicalMessage(summary *appagentthread.MessageSummary) (*canonicalMessage, error) {
	public := appagentthread.ProjectPublicMessage(summary)
	if public == nil {
		return nil, nil
	}
	if public.MessageID <= 0 || public.ThreadID <= 0 || public.RunID <= 0 {
		return nil, fmt.Errorf("canonical message projection requires positive message, thread, and run ids")
	}

	return &canonicalMessage{
		MessageID: strconv.FormatInt(public.MessageID, 10),
		ThreadID:  strconv.FormatInt(public.ThreadID, 10),
		RunID:     strconv.FormatInt(public.RunID, 10),
		Role:      string(public.Role),
		Content:   public.Content,
		Metadata:  canonicalMetadataFromJSON(public.Metadata, ""),
		CreatedAt: canonicalTime(public.CreatedAt),
	}, nil
}

func projectCanonicalRunEvent(summary *appagentthread.RunEventSummary) (*canonicalRunEvent, error) {
	public := appagentthread.ProjectPublicRunEvent(summary)
	if public == nil {
		return nil, nil
	}
	if public.EventID <= 0 || public.ThreadID <= 0 || public.RunID <= 0 {
		return nil, fmt.Errorf("canonical run event projection requires positive event, thread, and run ids")
	}
	if strings.TrimSpace(public.EventType) == "" {
		return nil, fmt.Errorf("canonical run event projection requires a public event type")
	}

	return &canonicalRunEvent{
		EventID:   strconv.FormatInt(public.EventID, 10),
		ThreadID:  strconv.FormatInt(public.ThreadID, 10),
		RunID:     strconv.FormatInt(public.RunID, 10),
		EventType: public.EventType,
		Payload:   canonicalSanitizeMap(canonicalJSONObject(public.Payload)),
		CreatedAt: canonicalTime(public.CreatedAt),
	}, nil
}

func projectCanonicalThreadState(source canonicalStateSource) (*canonicalThreadState, error) {
	if source.ThreadID <= 0 || source.CheckpointID <= 0 {
		return nil, fmt.Errorf("canonical state projection requires positive thread and checkpoint ids")
	}
	if source.ParentCheckpointID < 0 {
		return nil, fmt.Errorf("canonical state projection parent checkpoint id cannot be negative")
	}
	checkpointNS, err := canonicalCheckpointNamespace(source.CheckpointNS)
	if err != nil {
		return nil, err
	}

	checkpoint := canonicalCheckpoint{
		ThreadID:      strconv.FormatInt(source.ThreadID, 10),
		CheckpointNS:  checkpointNS,
		CheckpointID:  strconv.FormatInt(source.CheckpointID, 10),
		CheckpointMap: map[string]any{},
	}
	var parent *canonicalCheckpoint
	if source.ParentCheckpointID > 0 {
		parentCheckpointNS := source.ParentCheckpointNS
		if strings.TrimSpace(parentCheckpointNS) == "" {
			parentCheckpointNS = checkpoint.CheckpointNS
		}
		parentCheckpointNS, err = canonicalCheckpointNamespace(parentCheckpointNS)
		if err != nil {
			return nil, err
		}
		projected := canonicalCheckpoint{
			ThreadID:      checkpoint.ThreadID,
			CheckpointNS:  parentCheckpointNS,
			CheckpointID:  strconv.FormatInt(source.ParentCheckpointID, 10),
			CheckpointMap: map[string]any{},
		}
		parent = &projected
	}

	return &canonicalThreadState{
		Values:           canonicalPublicStateValues(source.Values),
		Next:             canonicalIdentifiers(source.Next),
		Checkpoint:       checkpoint,
		Metadata:         canonicalEntityMetadataFromMap(source.Metadata, ""),
		CreatedAt:        canonicalTime(source.CreatedAt),
		ParentCheckpoint: parent,
		// Raw task result, error, nested state, and checkpoint data are not public.
		Tasks:      []map[string]any{},
		Interrupts: canonicalStateInterrupts(source.Interrupts),
	}, nil
}

func projectCanonicalThreadUpdateStateResult(
	checkpoint canonicalCheckpoint,
) canonicalThreadUpdateStateResult {
	projected := checkpoint
	return canonicalThreadUpdateStateResult{
		Checkpoint:   projected,
		Configurable: projected,
	}
}

func loadCanonicalThreadProjectionSnapshot(
	ctx context.Context,
	threadID int64,
) (canonicalThreadProjectionSnapshot, error) {
	if threadID <= 0 {
		return canonicalThreadProjectionSnapshot{}, fmt.Errorf("canonical thread projection requires a positive thread id")
	}
	if appagentthread.SVC == nil {
		return canonicalThreadProjectionSnapshot{}, fmt.Errorf("agent thread application service is unavailable")
	}

	ctx = canonicalThreadAccessContext(ctx, threadID, 0)
	runs, err := appagentthread.SVC.ListRuns(ctx, &appagentthread.ListRunsRequest{
		ThreadID: threadID,
		Page:     1,
		PageSize: 1,
	})
	if err != nil {
		return canonicalThreadProjectionSnapshot{}, fmt.Errorf("load latest canonical thread run: %w", err)
	}
	if runs == nil || len(runs.Runs) == 0 {
		return canonicalThreadProjectionSnapshot{}, nil
	}

	snapshot := canonicalThreadProjectionSnapshot{LatestRun: runs.Runs[0]}
	if runs.Runs[0] == nil || runs.Runs[0].Status != appagentthread.RunStatusInterrupted {
		return snapshot, nil
	}

	interrupts, err := loadCanonicalThreadInterrupts(ctx, threadID, runs.Runs[0].RunID)
	if err != nil {
		return canonicalThreadProjectionSnapshot{}, err
	}
	snapshot.Interrupts = interrupts
	return snapshot, nil
}

// loadCanonicalThreadInterrupts scans newest pages first because the resumable
// run.interrupted event is terminal while the existing repository is ID-ascending.
func loadCanonicalThreadInterrupts(
	ctx context.Context,
	threadID,
	runID int64,
) (map[string]any, error) {
	first, err := appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
		ThreadID: threadID,
		RunID:    runID,
		Page:     1,
		PageSize: canonicalThreadEventPageSize,
	})
	if err != nil {
		return nil, fmt.Errorf("load canonical thread interrupt events: %w", err)
	}
	if first == nil || first.Total <= 0 {
		return map[string]any{}, nil
	}
	maxPages := int64(math.MaxInt32)
	lastPage := (first.Total + int64(canonicalThreadEventPageSize) - 1) /
		int64(canonicalThreadEventPageSize)
	if lastPage > maxPages {
		return nil, fmt.Errorf("canonical thread interrupt event history is too large")
	}

	for page := int32(lastPage); page >= 1; page-- {
		response := first
		if page != 1 {
			response, err = appagentthread.SVC.ListRunEvents(ctx, &appagentthread.ListRunEventsRequest{
				ThreadID: threadID,
				RunID:    runID,
				Page:     page,
				PageSize: canonicalThreadEventPageSize,
			})
			if err != nil {
				return nil, fmt.Errorf("load canonical thread interrupt event page: %w", err)
			}
		}
		if response == nil {
			continue
		}
		for index := len(response.Events) - 1; index >= 0; index-- {
			event := response.Events[index]
			if event == nil || strings.TrimSpace(event.EventType) != "run.interrupted" {
				continue
			}
			projected, projectionErr := projectCanonicalRunEvent(event)
			if projectionErr != nil {
				return nil, projectionErr
			}
			if interrupts := canonicalThreadInterrupts(projected); len(interrupts) > 0 {
				return interrupts, nil
			}
		}
	}

	return map[string]any{}, nil
}

func canonicalThreadInterrupts(event *canonicalRunEvent) map[string]any {
	result := map[string]any{}
	if event == nil {
		return result
	}
	container, _ := event.Payload["interrupts"].(map[string]any)
	items, _ := container["items"].([]any)
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		interruptID := canonicalString(raw["id"])
		info, _ := raw["info"].(map[string]any)
		if interruptID == "" || len(info) == 0 {
			continue
		}
		value := canonicalSanitizeMap(info)
		value["interrupt_id"] = interruptID
		if kind := canonicalString(info["kind"]); kind != "" {
			value["type"] = kind
		}
		result[interruptID] = []map[string]any{{
			"value":     value,
			"when":      "during",
			"resumable": true,
		}}
	}
	return result
}

func canonicalThreadStatus(
	latestRun *appagentthread.RunSummary,
	interrupts map[string]any,
) (string, string, error) {
	if latestRun == nil {
		return "idle", "idle", nil
	}

	switch latestRun.Status {
	case appagentthread.RunStatusPending, appagentthread.RunStatusQueued, appagentthread.RunStatusRunning:
		return "busy", "running", nil
	case appagentthread.RunStatusInterrupted:
		if len(canonicalSanitizeMap(interrupts)) > 0 {
			return "interrupted", "idle", nil
		}
		return "idle", "idle", nil
	case appagentthread.RunStatusSucceeded:
		return "idle", "completed", nil
	case appagentthread.RunStatusFailed:
		return "error", "failed", nil
	case appagentthread.RunStatusCanceled:
		return "idle", "canceled", nil
	default:
		return "", "", fmt.Errorf("unsupported canonical thread run status %q", latestRun.Status)
	}
}

func canonicalRunStatus(status appagentthread.RunStatus) (string, *string, error) {
	switch status {
	case appagentthread.RunStatusPending, appagentthread.RunStatusQueued:
		return "pending", nil, nil
	case appagentthread.RunStatusRunning:
		return "running", nil, nil
	case appagentthread.RunStatusInterrupted:
		return "interrupted", nil, nil
	case appagentthread.RunStatusSucceeded:
		return "success", nil, nil
	case appagentthread.RunStatusFailed:
		return "error", nil, nil
	case appagentthread.RunStatusCanceled:
		reason := "canceled"
		return "interrupted", &reason, nil
	default:
		return "", nil, fmt.Errorf("unsupported canonical run status %q", status)
	}
}

func canonicalRunAttemptKind(kind appagentthread.RunKind, metadata map[string]any) string {
	switch {
	case canonicalMetadataObjectSchema(
		metadata,
		"human_interaction",
		"coze.human_interaction_resolved.v1",
	) != nil:
		return "resume"
	case kind == appagentthread.RunKindTask && canonicalProjectedTopLevelRetrySourceRunID(metadata) != nil:
		return "retry"
	case canonicalMetadataObjectSchema(
		metadata,
		"subagent_retry",
		"coze.subagent_retry.metadata.v1",
	) != nil:
		return "retry"
	case kind == appagentthread.RunKindSubagent:
		return "subagent"
	default:
		return "turn"
	}
}

func canonicalRunSourceRunID(metadata map[string]any) *string {
	if human := canonicalMetadataObjectSchema(
		metadata,
		"human_interaction",
		"coze.human_interaction_resolved.v1",
	); human != nil {
		if sourceRunID := canonicalMetadataID(human["source_run_id"]); sourceRunID != nil {
			return sourceRunID
		}
		return canonicalNestedMetadataID(metadata, "checkpoint_resume", "source_run_id")
	}
	if sourceRunID := canonicalProjectedTopLevelRetrySourceRunID(metadata); sourceRunID != nil {
		return sourceRunID
	}
	if canonicalMetadataObjectSchema(
		metadata,
		"subagent_retry",
		"coze.subagent_retry.metadata.v1",
	) != nil {
		return canonicalMetadataID(metadata["source_run_id"])
	}
	return nil
}

func canonicalProjectedTopLevelRetrySourceRunID(metadata map[string]any) *string {
	attemptKind, ok := metadata["attempt_kind"].(string)
	if !ok || strings.TrimSpace(attemptKind) != "retry" {
		return nil
	}
	return canonicalMetadataID(metadata["source_run_id"])
}

func canonicalNestedMetadataID(metadata map[string]any, objectKey, idKey string) *string {
	object, ok := metadata[objectKey].(map[string]any)
	if !ok {
		return nil
	}
	return canonicalMetadataID(object[idKey])
}

func canonicalMetadataObjectSchema(
	metadata map[string]any,
	key string,
	wantSchema string,
) map[string]any {
	object, ok := metadata[key].(map[string]any)
	if !ok || canonicalString(object["schema"]) != wantSchema {
		return nil
	}
	return object
}

func canonicalRunStreamModes(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"messages-tuple", "updates"}
	}
	allowed := map[string]bool{
		"values":         true,
		"updates":        true,
		"messages":       true,
		"messages-tuple": true,
		"custom":         true,
		"events":         true,
	}
	values := make([]string, 0)
	var decoded []string
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		values = decoded
	} else {
		values = []string{raw}
	}
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !allowed[value] || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func canonicalRunOnDisconnect(value string) string {
	switch strings.TrimSpace(value) {
	case "cancel":
		return "cancel"
	case "continue", "":
		return "continue"
	default:
		return "continue"
	}
}

func canonicalRunDurability(string) string {
	return "async"
}

func canonicalThreadSource(source appagentthread.ThreadSource) (string, error) {
	switch source {
	case "":
		return "", nil
	case appagentthread.ThreadSourceWeb, appagentthread.ThreadSourceIM, appagentthread.ThreadSourceAPI:
		return string(source), nil
	default:
		return "", fmt.Errorf("unsupported canonical thread source %q", source)
	}
}

func canonicalRunKind(kind appagentthread.RunKind) (string, error) {
	switch kind {
	case "", appagentthread.RunKindTask:
		return "task", nil
	case appagentthread.RunKindSubagent:
		return "subagent", nil
	default:
		return "", fmt.Errorf("unsupported canonical run kind %q", kind)
	}
}

func canonicalOptionalTimeID(value int64) *string {
	if value <= 0 {
		return nil
	}
	formatted := strconv.FormatInt(value, 10)
	return &formatted
}

func canonicalOptionalTime(value int64, resource string) (*string, error) {
	return canonicalCheckedTime(value, resource)
}

func canonicalRequiredTime(value int64, resource string) (string, error) {
	projected, err := canonicalCheckedTime(value, resource)
	if err != nil {
		return "", err
	}
	if projected == nil {
		return "", fmt.Errorf("canonical %s projection requires a valid time", resource)
	}
	return *projected, nil
}

func canonicalCheckedTime(value int64, resource string) (*string, error) {
	if value == 0 {
		return nil, nil
	}
	if value < 0 {
		return nil, fmt.Errorf("canonical %s projection requires a valid time", resource)
	}
	projected := canonicalTime(value)
	if projected == "" {
		return nil, fmt.Errorf("canonical %s projection requires a valid time", resource)
	}
	if _, err := time.Parse(time.RFC3339Nano, projected); err != nil {
		return nil, fmt.Errorf("canonical %s projection requires a valid time", resource)
	}
	return &projected, nil
}

func canonicalMetadataFromJSON(raw, title string) map[string]any {
	return canonicalMetadataFromMap(canonicalJSONObject(raw), title)
}

func canonicalEntityMetadataFromJSON(raw, title string) map[string]any {
	result := canonicalMetadataFromJSON(raw, title)
	removeCanonicalInternalMetadata(result)
	return result
}

func canonicalEntityMetadataFromMap(source map[string]any, title string) map[string]any {
	result := canonicalMetadataFromMap(source, title)
	removeCanonicalInternalMetadata(result)
	return result
}

func removeCanonicalInternalMetadata(metadata map[string]any) {
	for _, key := range []string{
		"status", "appended_message_id", "attempt_kind", "source_run_id", "parent_run_id", "requested_at",
	} {
		delete(metadata, key)
	}
}

func canonicalMetadataFromMap(source map[string]any, title string) map[string]any {
	result := map[string]any{}
	for key, value := range source {
		if !canonicalMetadataKeyPattern.MatchString(key) || canonicalProtectedMetadataKey(key) {
			continue
		}
		if canonicalIDKeyPattern.MatchString(key) {
			if projected, ok := canonicalIDScalar(value); ok {
				result[key] = projected
			}
			continue
		}
		if canonicalIDsKeyPattern.MatchString(key) {
			if projected, ok := canonicalIDSlice(value); ok {
				result[key] = projected
			}
			continue
		}
		if projected, ok := canonicalMetadataValue(value); ok {
			result[key] = projected
		}
	}
	if title = canonicalMetadataLabel(title); title != "" {
		result["title"] = title
	}
	return result
}

func canonicalMetadataLabel(value string) string {
	value = canonicalCleanString(value, canonicalMaxMetadataRunes)
	if value == "" || canonicalSensitiveValuePattern.MatchString(value) {
		return ""
	}
	return value
}

func canonicalMetadataValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, true
	case bool:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return typed, err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		parsed := float64(typed)
		return typed, !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return typed, true
	case string:
		projected := canonicalCleanString(typed, canonicalMaxMetadataRunes)
		if projected == "" || canonicalMetadataStringIsSensitive(projected) {
			return nil, false
		}
		return projected, true
	default:
		return nil, false
	}
}

func canonicalMetadataStringIsSensitive(value string) bool {
	// This reviewed business label contains the substring "sk_", which the
	// generic credential detector intentionally treats as sensitive elsewhere.
	if value == "task_retry" {
		return false
	}
	return canonicalSensitiveValuePattern.MatchString(value)
}

func canonicalProtectedMetadataKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if canonicalUnsafePublicField(normalized) {
		return true
	}
	for _, fragment := range []string{
		"owner", "creator", "user_id", "space", "can_edit", "runtime", "credential", "secret", "token",
		"api_key", "apikey", "authorization", "password", "legacy_task_id", "worker_id",
		"lease_owner", "lease_token", "idempotency_key", "checkpoint_bytes", "provider_body",
		"hidden_config",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func canonicalPublicStateValues(source map[string]any) map[string]any {
	// The source must already be an application public projection. This adapter
	// narrows it again to the canonical state channels promised to SDK clients.
	result := map[string]any{"messages": []map[string]any{}}
	allowed := map[string]bool{
		"title":          true,
		"runtime":        true,
		"messages":       true,
		"todos":          true,
		"uploaded_files": true,
		"artifacts":      true,
		"viewed_images":  true,
		"active_skills":  true,
		"promoted":       true,
		"summary":        true,
		"completion":     true,
		"custom":         true,
	}
	for key, value := range source {
		if !allowed[key] || canonicalUnsafePublicField(key) {
			continue
		}
		if projected, ok := canonicalSanitizeValue(value); ok {
			result[key] = projected
		}
	}
	return result
}

func canonicalStateInterrupts(source []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(source))
	for _, item := range source {
		projected := map[string]any{}
		for _, key := range []string{"id", "value", "when", "resumable", "ns"} {
			value, exists := item[key]
			if !exists {
				continue
			}
			if sanitized, ok := canonicalSanitizeValue(value); ok {
				projected[key] = sanitized
			}
		}
		if len(projected) > 0 {
			result = append(result, projected)
		}
	}
	return result
}

func canonicalSanitizeMap(source map[string]any) map[string]any {
	// Key-based defense in depth prevents reviewed DTO containers from regaining
	// raw runtime fields when future application projections add data.
	result := map[string]any{}
	for key, value := range source {
		if canonicalUnsafePublicField(key) {
			continue
		}
		if canonicalIDKeyPattern.MatchString(key) {
			if projected, ok := canonicalIDScalar(value); ok {
				result[key] = projected
			}
			continue
		}
		if canonicalIDsKeyPattern.MatchString(key) {
			if projected, ok := canonicalIDSlice(value); ok {
				result[key] = projected
			}
			continue
		}
		if projected, ok := canonicalSanitizeValue(value); ok {
			result[key] = projected
		}
	}
	return result
}

func canonicalSanitizeValue(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, true
	case map[string]any:
		return canonicalSanitizeMap(typed), true
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			if projected, ok := canonicalSanitizeValue(item); ok {
				result = append(result, projected)
			}
		}
		return result, true
	case []map[string]any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, canonicalSanitizeMap(item))
		}
		return result, true
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if projected := canonicalCleanString(item, canonicalMaxPublicValueRunes); projected != "" {
				result = append(result, projected)
			}
		}
		return result, true
	case string:
		return canonicalCleanString(typed, canonicalMaxPublicValueRunes), true
	case bool, json.Number, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return typed, true
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	case float32:
		parsed := float64(typed)
		return typed, !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
	default:
		return nil, false
	}
}

func canonicalUnsafePublicField(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, fragment := range []string{
		"checkpoint_bytes", "channel_versions", "pending_sends", "provider_body", "provider_payload",
		"raw_provider", "tool_arguments", "tool_args", "tool_result", "hidden_config", "legacy_task_id",
		"worker_id", "lease_owner", "lease_token", "idempotency_key", "error_chain", "stack_trace",
		"traceback", "api_key", "credential", "secret", "access_token", "authorization", "password",
		"raw_usage", "signed_url", "download_url", "object_url",
	} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

func canonicalIDScalar(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, true
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil || parsed <= 0 {
			return nil, false
		}
		return strconv.FormatInt(parsed, 10), true
	case int:
		return canonicalPositiveInt64ID(int64(typed))
	case int8:
		return canonicalPositiveInt64ID(int64(typed))
	case int16:
		return canonicalPositiveInt64ID(int64(typed))
	case int32:
		return canonicalPositiveInt64ID(int64(typed))
	case int64:
		return canonicalPositiveInt64ID(typed)
	case uint:
		return canonicalPositiveUint64ID(uint64(typed))
	case uint8:
		return canonicalPositiveUint64ID(uint64(typed))
	case uint16:
		return canonicalPositiveUint64ID(uint64(typed))
	case uint32:
		return canonicalPositiveUint64ID(uint64(typed))
	case uint64:
		return canonicalPositiveUint64ID(typed)
	case float32:
		return canonicalFloatID(float64(typed))
	case float64:
		return canonicalFloatID(typed)
	case string:
		projected := canonicalCleanString(typed, 128)
		if projected == "" || canonicalSensitiveValuePattern.MatchString(projected) {
			return nil, false
		}
		if canonicalNumericStringPattern.MatchString(projected) {
			parsed, err := strconv.ParseInt(projected, 10, 64)
			if err != nil || parsed <= 0 {
				return nil, false
			}
			return strconv.FormatInt(parsed, 10), true
		}
		return projected, true
	default:
		return nil, false
	}
}

func canonicalIDSlice(value any) (any, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, true
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			projected, ok := canonicalIDScalar(item)
			if !ok {
				continue
			}
			result = append(result, projected)
		}
		return result, true
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			projected, ok := canonicalIDScalar(item)
			if !ok {
				continue
			}
			result = append(result, projected.(string))
		}
		return result, true
	case []int64:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			projected, ok := canonicalPositiveInt64ID(item)
			if ok {
				result = append(result, projected.(string))
			}
		}
		return result, true
	default:
		return nil, false
	}
}

func canonicalPositiveInt64ID(value int64) (any, bool) {
	if value <= 0 {
		return nil, false
	}
	return strconv.FormatInt(value, 10), true
}

func canonicalPositiveUint64ID(value uint64) (any, bool) {
	if value == 0 {
		return nil, false
	}
	return strconv.FormatUint(value, 10), true
}

func canonicalFloatID(value float64) (any, bool) {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) ||
		value > canonicalMaxExactFloatID {
		return nil, false
	}
	return strconv.FormatInt(int64(value), 10), true
}

func canonicalJSONObject(raw string) map[string]any {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.UseNumber()
	result := map[string]any{}
	if err := decoder.Decode(&result); err != nil {
		return map[string]any{}
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return map[string]any{}
	}
	if result == nil {
		return map[string]any{}
	}
	return result
}

func canonicalMetadataID(value any) *string {
	var parsed int64
	switch typed := value.(type) {
	case json.Number:
		parsed, _ = typed.Int64()
	case string:
		parsed, _ = strconv.ParseInt(typed, 10, 64)
	case int64:
		parsed = typed
	case int:
		parsed = int64(typed)
	case float64:
		if typed == math.Trunc(typed) && typed <= math.MaxInt64 {
			parsed = int64(typed)
		}
	}
	if parsed <= 0 {
		return nil
	}
	formatted := strconv.FormatInt(parsed, 10)
	return &formatted
}

func canonicalCheckpointNamespace(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if len([]rune(value)) > 128 || !canonicalIdentifierPattern.MatchString(value) {
		return "", fmt.Errorf("canonical checkpoint namespace is invalid")
	}
	return value, nil
}

func canonicalIdentifiers(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) > 128 || !canonicalIdentifierPattern.MatchString(value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func canonicalString(value any) string {
	text, _ := value.(string)
	return canonicalCleanString(text, 128)
}

func canonicalCleanString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 {
		return ""
	}
	value = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		if r == 0x7f {
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

func canonicalTime(milliseconds int64) string {
	if milliseconds <= 0 {
		return ""
	}
	return time.UnixMilli(milliseconds).UTC().Format(time.RFC3339Nano)
}
