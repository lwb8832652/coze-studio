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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

const (
	defaultGuardrailAuditArchiveObjectPrefix = "guardrail/audit/archive"
	guardrailAuditArchiveContentType         = "application/json; charset=utf-8"
)

var errGuardrailAuditArchiveWriteFailed = errors.New("guardrail audit archive write failed")

type GuardrailAuditArchiveObjectStorage interface {
	PutObject(
		ctx context.Context,
		objectKey string,
		content []byte,
		opts ...storage.PutOptFn,
	) error
}

type GuardrailAuditObjectStorageArchiveWriterOptions struct {
	Storage GuardrailAuditArchiveObjectStorage
	Prefix  string
}

type GuardrailAuditObjectStorageArchiveWriter struct {
	storage GuardrailAuditArchiveObjectStorage
	prefix  string
}

type guardrailAuditArchiveJSONPayload struct {
	Schema          string                           `json:"schema"`
	ExportedAt      int64                            `json:"exported_at"`
	CutoffCreatedAt int64                            `json:"cutoff_created_at"`
	Total           int64                            `json:"total"`
	Events          []guardrailAuditArchiveJSONEvent `json:"events"`
}

type guardrailAuditArchiveJSONEvent struct {
	EventID    int64  `json:"event_id"`
	ThreadID   int64  `json:"thread_id"`
	RunID      int64  `json:"run_id"`
	SpaceID    int64  `json:"space_id"`
	ActorID    int64  `json:"actor_id"`
	EventType  string `json:"event_type"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Operation  string `json:"operation"`
	Source     string `json:"source"`
	Action     string `json:"action"`
	FailMode   string `json:"fail_mode"`
	Provider   string `json:"provider"`
	ReasonCode string `json:"reason_code"`
	RuleIDs    string `json:"rule_ids"`
	CreatedAt  int64  `json:"created_at"`
}

func NewGuardrailAuditObjectStorageArchiveWriter(
	options GuardrailAuditObjectStorageArchiveWriterOptions,
) *GuardrailAuditObjectStorageArchiveWriter {
	return &GuardrailAuditObjectStorageArchiveWriter{
		storage: options.Storage,
		prefix:  normalizeGuardrailAuditArchivePrefix(options.Prefix),
	}
}

func (w *GuardrailAuditObjectStorageArchiveWriter) WriteGuardrailAuditArchive(
	ctx context.Context,
	payload GuardrailAuditArchivePayload,
) (GuardrailAuditArchiveWriteResult, error) {
	if w == nil || w.storage == nil || !validGuardrailAuditArchivePayload(payload) {
		return GuardrailAuditArchiveWriteResult{}, errGuardrailAuditArchiveWriteFailed
	}

	content, err := json.Marshal(guardrailAuditArchiveJSONFromPayload(payload))
	if err != nil {
		return GuardrailAuditArchiveWriteResult{}, errGuardrailAuditArchiveWriteFailed
	}
	archiveID := guardrailAuditArchiveID(payload, content)
	objectKey := path.Join(
		w.prefix,
		fmt.Sprintf("%d", payload.CutoffCreatedAt),
		archiveID+".json",
	)
	if err := w.storage.PutObject(
		ctx,
		objectKey,
		content,
		storage.WithContentType(guardrailAuditArchiveContentType),
		storage.WithObjectSize(int64(len(content))),
	); err != nil {
		return GuardrailAuditArchiveWriteResult{}, errGuardrailAuditArchiveWriteFailed
	}

	return GuardrailAuditArchiveWriteResult{ArchiveID: archiveID}, nil
}

func normalizeGuardrailAuditArchivePrefix(prefix string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" ||
		strings.Contains(prefix, "://") ||
		strings.Contains(prefix, "\\") {
		return defaultGuardrailAuditArchiveObjectPrefix
	}
	cleaned := path.Clean(prefix)
	if cleaned == "." ||
		cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") ||
		strings.HasPrefix(cleaned, "/") {
		return defaultGuardrailAuditArchiveObjectPrefix
	}
	return strings.Trim(cleaned, "/")
}

func validGuardrailAuditArchivePayload(payload GuardrailAuditArchivePayload) bool {
	if payload.Schema != GuardrailAuditArchiveSchema ||
		payload.ExportedAt <= 0 ||
		payload.CutoffCreatedAt <= 0 ||
		len(payload.Events) == 0 {
		return false
	}
	for _, event := range payload.Events {
		if event == nil {
			return false
		}
	}
	return true
}

func guardrailAuditArchiveJSONFromPayload(
	payload GuardrailAuditArchivePayload,
) guardrailAuditArchiveJSONPayload {
	events := make([]guardrailAuditArchiveJSONEvent, 0, len(payload.Events))
	for _, event := range payload.Events {
		events = append(events, guardrailAuditArchiveJSONEvent{
			EventID:    event.EventID,
			ThreadID:   event.ThreadID,
			RunID:      event.RunID,
			SpaceID:    event.SpaceID,
			ActorID:    event.ActorID,
			EventType:  event.EventType,
			TargetType: event.TargetType,
			TargetID:   event.TargetID,
			Operation:  event.Operation,
			Source:     event.Source,
			Action:     event.Action,
			FailMode:   event.FailMode,
			Provider:   event.Provider,
			ReasonCode: event.ReasonCode,
			RuleIDs:    event.RuleIDs,
			CreatedAt:  event.CreatedAt,
		})
	}

	return guardrailAuditArchiveJSONPayload{
		Schema:          payload.Schema,
		ExportedAt:      payload.ExportedAt,
		CutoffCreatedAt: payload.CutoffCreatedAt,
		Total:           payload.Total,
		Events:          events,
	}
}

func guardrailAuditArchiveID(
	payload GuardrailAuditArchivePayload,
	content []byte,
) string {
	digest := sha256.Sum256(content)
	return fmt.Sprintf(
		"guardrail_archive_%d_%d_%s",
		payload.ExportedAt,
		payload.CutoffCreatedAt,
		hex.EncodeToString(digest[:])[:16],
	)
}
