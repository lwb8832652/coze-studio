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

package mcptool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
)

const (
	ManagementAuditStatusPending = "pending"
	ManagementAuditStatusSuccess = "success"
	ManagementAuditStatusFailed  = "failed"

	ManagementAuditErrorRuntimeFailed      = "runtime_failed"
	ManagementAuditErrorRuntimeUnavailable = "runtime_unavailable"
	ManagementAuditErrorInvalidResult      = "invalid_result"
	ManagementAuditErrorInternal           = "internal_error"

	defaultManagementAuditPageSize = 20
	maxManagementAuditPageSize     = 100
)

var (
	ErrManagementAuditUnavailable = errors.New("mcp management audit unavailable")
	ErrManagementAuditTransition  = errors.New("mcp management audit transition rejected")
)

// ManagementAuditEvent deliberately contains no request arguments, runtime
// result, connection config, credentials, or provider payload.
type ManagementAuditEvent struct {
	EventID      int64  `json:"event_id"`
	SpaceID      int64  `json:"space_id"`
	ServerID     int64  `json:"server_id"`
	ActorID      int64  `json:"actor_id"`
	ToolName     string `json:"tool_name"`
	Status       string `json:"status"`
	LatencyMs    int64  `json:"latency_ms"`
	ErrorCode    string `json:"error_code,omitempty"`
	ErrorSummary string `json:"error_summary,omitempty"`
	CreatedAt    int64  `json:"created_at"`
	CompletedAt  int64  `json:"completed_at,omitempty"`
}

type ManagementAuditCompletion struct {
	Status      string
	LatencyMs   int64
	ErrorCode   string
	CompletedAt int64
}

type ManagementAuditCursor struct {
	CreatedAt int64 `json:"created_at"`
	EventID   int64 `json:"event_id"`
}

type ManagementAuditRepository interface {
	CreatePending(ctx context.Context, event *ManagementAuditEvent) error
	Complete(ctx context.Context, eventID int64, completion ManagementAuditCompletion) error
	List(
		ctx context.Context,
		spaceID int64,
		serverID int64,
		cursor *ManagementAuditCursor,
		limit int,
	) ([]*ManagementAuditEvent, error)
}

type InMemoryManagementAuditRepository struct {
	mu     sync.Mutex
	nextID int64
	events []*ManagementAuditEvent
}

func NewInMemoryManagementAuditRepository() *InMemoryManagementAuditRepository {
	return &InMemoryManagementAuditRepository{}
}

func (r *InMemoryManagementAuditRepository) CreatePending(
	_ context.Context,
	event *ManagementAuditEvent,
) error {
	if r == nil || event == nil || event.SpaceID <= 0 || event.ServerID <= 0 || event.ActorID <= 0 {
		return ErrManagementAuditUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	event.EventID = r.nextID
	event.ToolName = normalizeManagementAuditToolName(event.ToolName)
	event.Status = ManagementAuditStatusPending
	event.LatencyMs = 0
	event.ErrorCode = ""
	event.ErrorSummary = ""
	event.CompletedAt = 0
	r.events = append(r.events, cloneManagementAuditEvent(event))
	return nil
}

func (r *InMemoryManagementAuditRepository) Complete(
	_ context.Context,
	eventID int64,
	completion ManagementAuditCompletion,
) error {
	if r == nil || eventID <= 0 {
		return ErrManagementAuditTransition
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, event := range r.events {
		if event.EventID != eventID || event.Status != ManagementAuditStatusPending {
			continue
		}
		applyManagementAuditCompletion(event, completion)
		return nil
	}
	return ErrManagementAuditTransition
}

func (r *InMemoryManagementAuditRepository) List(
	_ context.Context,
	spaceID int64,
	serverID int64,
	cursor *ManagementAuditCursor,
	limit int,
) ([]*ManagementAuditEvent, error) {
	if r == nil || spaceID <= 0 || serverID <= 0 || limit <= 0 {
		return nil, ErrManagementAuditUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]*ManagementAuditEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.SpaceID != spaceID || event.ServerID != serverID {
			continue
		}
		if cursor != nil && (event.CreatedAt > cursor.CreatedAt ||
			(event.CreatedAt == cursor.CreatedAt && event.EventID >= cursor.EventID)) {
			continue
		}
		events = append(events, cloneManagementAuditEvent(event))
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].CreatedAt == events[j].CreatedAt {
			return events[i].EventID > events[j].EventID
		}
		return events[i].CreatedAt > events[j].CreatedAt
	})
	if len(events) > limit {
		events = events[:limit]
	}
	return events, nil
}

func cloneManagementAuditEvent(event *ManagementAuditEvent) *ManagementAuditEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	return &cloned
}

func normalizeManagementAuditToolName(toolName string) string {
	toolName = strings.TrimSpace(toolName)
	if len(toolName) > 256 {
		return toolName[:256]
	}
	return toolName
}

func normalizeManagementAuditErrorCode(errorCode string) string {
	switch strings.TrimSpace(errorCode) {
	case "":
		return ""
	case ManagementAuditErrorRuntimeFailed,
		ManagementAuditErrorRuntimeUnavailable,
		ManagementAuditErrorInvalidResult:
		return strings.TrimSpace(errorCode)
	default:
		return ManagementAuditErrorInternal
	}
}

func safeManagementAuditErrorSummary(errorCode string) string {
	switch normalizeManagementAuditErrorCode(errorCode) {
	case "":
		return ""
	case ManagementAuditErrorRuntimeUnavailable:
		return "mcp runtime unavailable"
	case ManagementAuditErrorInvalidResult:
		return "runtime returned an invalid result"
	case ManagementAuditErrorRuntimeFailed:
		return "runtime call failed"
	default:
		return "operation failed"
	}
}

func applyManagementAuditCompletion(event *ManagementAuditEvent, completion ManagementAuditCompletion) {
	status := strings.TrimSpace(completion.Status)
	if status != ManagementAuditStatusSuccess && status != ManagementAuditStatusFailed {
		status = ManagementAuditStatusFailed
	}
	latencyMs := completion.LatencyMs
	if latencyMs < 0 {
		latencyMs = 0
	}
	event.Status = status
	event.LatencyMs = latencyMs
	event.ErrorCode = normalizeManagementAuditErrorCode(completion.ErrorCode)
	if status == ManagementAuditStatusSuccess {
		event.ErrorCode = ""
	}
	event.ErrorSummary = safeManagementAuditErrorSummary(event.ErrorCode)
	event.CompletedAt = completion.CompletedAt
	if event.CompletedAt < event.CreatedAt {
		event.CompletedAt = event.CreatedAt
	}
}

func normalizeManagementAuditPageSize(limit int) int {
	if limit <= 0 {
		return defaultManagementAuditPageSize
	}
	if limit > maxManagementAuditPageSize {
		return maxManagementAuditPageSize
	}
	return limit
}

func encodeManagementAuditCursor(cursor ManagementAuditCursor) string {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeManagementAuditCursor(raw string) (*ManagementAuditCursor, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, InvalidArgumentErrorf("audit cursor is invalid")
	}
	var cursor ManagementAuditCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.CreatedAt <= 0 || cursor.EventID <= 0 {
		return nil, InvalidArgumentErrorf("audit cursor is invalid")
	}
	return &cursor, nil
}
