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
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type TranscriptKind string

const (
	TranscriptKindSummaryInput TranscriptKind = "summary_input"
	TranscriptKindTerminal     TranscriptKind = "terminal"

	einoMessageIDExtraKey             = "_eino_msg_id"
	adkSummarizationContentTypeKey    = "_eino_summarization_content_type"
	adkTranscriptMemoryFlushPurpose   = "memory_flush"
	adkTranscriptUploadedFilesPattern = `(?s)<uploaded_files>.*?</uploaded_files>`
)

var (
	adkTranscriptUploadedFilesRE = regexp.MustCompile(adkTranscriptUploadedFilesPattern)
	adkTranscriptCorrectionREs   = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bthat(?:'s| is) (?:wrong|incorrect)\b`),
		regexp.MustCompile(`(?i)\byou misunderstood\b`),
		regexp.MustCompile(`(?i)\btry again\b`),
		regexp.MustCompile(`(?i)\bredo\b`),
		regexp.MustCompile(`不对`),
		regexp.MustCompile(`你理解错了`),
		regexp.MustCompile(`你理解有误`),
		regexp.MustCompile(`重试`),
		regexp.MustCompile(`重新来`),
		regexp.MustCompile(`换一种`),
		regexp.MustCompile(`改用`),
	}
	adkTranscriptReinforcementREs = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\byes[,.]?\s+(?:exactly|perfect|that(?:'s| is) (?:right|correct|it))\b`),
		regexp.MustCompile(`(?i)\bperfect(?:[.!?]|$)`),
		regexp.MustCompile(`(?i)\bexactly\s+(?:right|correct)\b`),
		regexp.MustCompile(`(?i)\bthat(?:'s| is)\s+(?:exactly\s+)?(?:right|correct|what i (?:wanted|needed|meant))\b`),
		regexp.MustCompile(`(?i)\bkeep\s+(?:doing\s+)?that\b`),
		regexp.MustCompile(`(?i)\bjust\s+(?:like\s+)?(?:that|this)\b`),
		regexp.MustCompile(`(?i)\bthis is (?:great|helpful)\b(?:[.!?]|$)`),
		regexp.MustCompile(`(?i)\bthis is what i wanted\b(?:[.!?]|$)`),
		regexp.MustCompile(`对[，,]?\s*就是这样(?:[。！？!?.]|$)`),
		regexp.MustCompile(`完全正确(?:[。！？!?.]|$)`),
		regexp.MustCompile(`(?:对[，,]?\s*)?就是这个意思(?:[。！？!?.]|$)`),
		regexp.MustCompile(`正是我想要的(?:[。！？!?.]|$)`),
		regexp.MustCompile(`继续保持(?:[。！？!?.]|$)`),
	}
)

type ADKTranscriptPersistRequest struct {
	Kind           TranscriptKind
	Digest         string
	IdempotencyKey string
	MessageCount   int32
	Messages       string
	Metadata       string
}

type ADKTranscriptSnapshot struct {
	SnapshotID     int64
	Kind           TranscriptKind
	Digest         string
	IdempotencyKey string
	MessageCount   int32
	Messages       string
}

type ADKMemoryFlushRequest struct {
	SnapshotID     int64
	Kind           TranscriptKind
	Digest         string
	IdempotencyKey string
	MessageCount   int32
}

type ADKTranscriptStore interface {
	PersistTranscript(
		ctx context.Context,
		run *RunSummary,
		req ADKTranscriptPersistRequest,
	) (ADKTranscriptSnapshot, bool, error)
}

type ADKMemoryFlushQueue interface {
	EnqueueMemoryFlush(
		ctx context.Context,
		run *RunSummary,
		req ADKMemoryFlushRequest,
	) (bool, error)
}

type ApplicationADKContextStore struct {
	app *ApplicationService
}

func NewApplicationADKContextStore(
	app *ApplicationService,
) *ApplicationADKContextStore {
	return &ApplicationADKContextStore{app: app}
}

func (s *ApplicationADKContextStore) PersistTranscript(
	ctx context.Context,
	run *RunSummary,
	req ADKTranscriptPersistRequest,
) (ADKTranscriptSnapshot, bool, error) {
	if s == nil || s.app == nil {
		return ADKTranscriptSnapshot{}, false, fmt.Errorf(
			"agent thread application service is required",
		)
	}
	if run == nil {
		return ADKTranscriptSnapshot{}, false, fmt.Errorf("run is required")
	}
	resp, err := s.app.PersistTranscriptSnapshot(
		ctx,
		&PersistTranscriptSnapshotRequest{
			ThreadID:       run.ThreadID,
			RunID:          run.RunID,
			Kind:           req.Kind,
			Digest:         req.Digest,
			IdempotencyKey: req.IdempotencyKey,
			MessageCount:   req.MessageCount,
			Messages:       req.Messages,
			Metadata:       req.Metadata,
		},
	)
	if err != nil {
		return ADKTranscriptSnapshot{}, false, err
	}
	if resp == nil || resp.Snapshot == nil {
		return ADKTranscriptSnapshot{}, false, fmt.Errorf(
			"persist transcript returned empty snapshot",
		)
	}
	return ADKTranscriptSnapshot{
		SnapshotID:     resp.Snapshot.SnapshotID,
		Kind:           resp.Snapshot.Kind,
		Digest:         resp.Snapshot.Digest,
		IdempotencyKey: resp.Snapshot.IdempotencyKey,
		MessageCount:   resp.Snapshot.MessageCount,
		Messages:       resp.Snapshot.Messages,
	}, resp.Created, nil
}

func (s *ApplicationADKContextStore) EnqueueMemoryFlush(
	ctx context.Context,
	run *RunSummary,
	req ADKMemoryFlushRequest,
) (bool, error) {
	if s == nil || s.app == nil {
		return false, fmt.Errorf("agent thread application service is required")
	}
	if run == nil {
		return false, fmt.Errorf("run is required")
	}
	resp, err := s.app.EnqueueMemoryFlushJob(
		ctx,
		&EnqueueMemoryFlushJobRequest{
			ThreadID:             run.ThreadID,
			RunID:                run.RunID,
			TranscriptSnapshotID: req.SnapshotID,
			IdempotencyKey:       req.IdempotencyKey,
		},
	)
	if err != nil {
		return false, err
	}
	if resp == nil || resp.Job == nil {
		return false, fmt.Errorf("enqueue memory flush returned empty job")
	}
	return resp.Created, nil
}

type ADKTranscriptHooks struct {
	run       *RunSummary
	store     ADKTranscriptStore
	queue     ADKMemoryFlushQueue
	eventSink RunEventSink
}

func NewADKTranscriptHooks(
	run *RunSummary,
	store ADKTranscriptStore,
	queue ADKMemoryFlushQueue,
	eventSink RunEventSink,
) *ADKTranscriptHooks {
	return &ADKTranscriptHooks{
		run:       run,
		store:     store,
		queue:     queue,
		eventSink: eventSink,
	}
}

func (h *ADKTranscriptHooks) PersistSummaryInput(
	ctx context.Context,
	messages []*schema.Message,
) error {
	return h.persist(ctx, TranscriptKindSummaryInput, messages)
}

func (h *ADKTranscriptHooks) PersistTerminal(
	ctx context.Context,
	messages []*schema.Message,
) error {
	return h.persist(ctx, TranscriptKindTerminal, messages)
}

func (h *ADKTranscriptHooks) persist(
	ctx context.Context,
	kind TranscriptKind,
	messages []*schema.Message,
) error {
	if h == nil || h.run == nil {
		return fmt.Errorf("eino adk transcript hooks require a run")
	}
	if h.store == nil {
		return fmt.Errorf("eino adk transcript store is required")
	}
	metadataExtra := map[string]any{}
	var memoryMessages []*schema.Message
	var memorySignal map[string]any
	memoryEligible := false
	if h.queue != nil && kind == TranscriptKindTerminal {
		memoryMessages = buildADKTranscriptMemoryMessages(messages)
		memoryEligible = adkTranscriptMemoryEligible(memoryMessages)
		if memoryEligible {
			memorySignal = buildADKTranscriptMemoryFlushMetadata(memoryMessages)
			metadataExtra["memory_flush"] = memorySignal
		}
	}
	snapshot, digest, _, messageCount, err := h.persistSnapshot(
		ctx,
		kind,
		messages,
		metadataExtra,
		true,
	)
	if err != nil {
		return err
	}

	if h.queue == nil || kind != TranscriptKindTerminal || !memoryEligible {
		return nil
	}
	memoryRaw, memoryDigest, memoryMessageCount, err := encodeADKTranscript(memoryMessages)
	if err != nil {
		return err
	}
	memoryIdempotencyKey := string(kind) + ":" + memoryDigest
	memorySnapshot := snapshot
	if memoryDigest != digest {
		memorySnapshot, err = h.persistMemoryFlushSnapshot(
			ctx,
			kind,
			memoryDigest,
			memoryIdempotencyKey,
			memoryMessageCount,
			memoryRaw,
			digest,
			snapshot.SnapshotID,
			memorySignal,
		)
		if err != nil {
			emitRunEvent(ctx, h.eventSink, RunEvent{
				ThreadID:  h.run.ThreadID,
				RunID:     h.run.RunID,
				EventType: "memory.update_failed",
				Payload: encodeRunEventPayload(ctx, map[string]any{
					"snapshot_id":   snapshot.SnapshotID,
					"kind":          kind,
					"digest":        digest,
					"message_count": messageCount,
					"error":         err.Error(),
				}),
			})
			return nil
		}
	}

	queued, queueErr := h.queue.EnqueueMemoryFlush(
		ctx,
		h.run,
		ADKMemoryFlushRequest{
			SnapshotID:     memorySnapshot.SnapshotID,
			Kind:           kind,
			Digest:         memoryDigest,
			IdempotencyKey: memoryIdempotencyKey,
			MessageCount:   memoryMessageCount,
		},
	)
	if queueErr != nil {
		emitRunEvent(ctx, h.eventSink, RunEvent{
			ThreadID:  h.run.ThreadID,
			RunID:     h.run.RunID,
			EventType: "memory.update_failed",
			Payload: encodeRunEventPayload(ctx, map[string]any{
				"snapshot_id":   memorySnapshot.SnapshotID,
				"kind":          kind,
				"digest":        memoryDigest,
				"message_count": memoryMessageCount,
				"error":         queueErr.Error(),
			}),
		})
		return nil
	}
	if queued {
		emitRunEvent(ctx, h.eventSink, RunEvent{
			ThreadID:  h.run.ThreadID,
			RunID:     h.run.RunID,
			EventType: "memory.update_queued",
			Payload: encodeRunEventPayload(ctx, map[string]any{
				"snapshot_id":   memorySnapshot.SnapshotID,
				"kind":          kind,
				"digest":        memoryDigest,
				"message_count": memoryMessageCount,
			}),
		})
	}
	return nil
}

func (h *ADKTranscriptHooks) persistSnapshot(
	ctx context.Context,
	kind TranscriptKind,
	messages []*schema.Message,
	metadataExtra map[string]any,
	emitEvent bool,
) (ADKTranscriptSnapshot, string, string, int32, error) {
	raw, digest, messageCount, err := encodeADKTranscript(messages)
	if err != nil {
		return ADKTranscriptSnapshot{}, "", "", 0, err
	}
	idempotencyKey := string(kind) + ":" + digest
	metadataMap := map[string]any{
		"runtime": "eino_adk",
		"kind":    kind,
		"digest":  digest,
	}
	for key, value := range metadataExtra {
		metadataMap[key] = value
	}
	metadata, err := json.Marshal(metadataMap)
	if err != nil {
		return ADKTranscriptSnapshot{}, "", "", 0,
			fmt.Errorf("encode eino adk transcript metadata: %w", err)
	}
	snapshot, created, err := h.store.PersistTranscript(
		ctx,
		h.run,
		ADKTranscriptPersistRequest{
			Kind:           kind,
			Digest:         digest,
			IdempotencyKey: idempotencyKey,
			MessageCount:   messageCount,
			Messages:       raw,
			Metadata:       string(metadata),
		},
	)
	if err != nil {
		return ADKTranscriptSnapshot{}, "", "", 0,
			fmt.Errorf("persist eino adk transcript: %w", err)
	}
	if snapshot.SnapshotID <= 0 {
		return ADKTranscriptSnapshot{}, "", "", 0,
			fmt.Errorf("persist eino adk transcript returned empty snapshot")
	}
	if created && emitEvent {
		emitRunEvent(ctx, h.eventSink, RunEvent{
			ThreadID:  h.run.ThreadID,
			RunID:     h.run.RunID,
			EventType: "context.transcript_persisted",
			Payload: encodeRunEventPayload(ctx, map[string]any{
				"snapshot_id":   snapshot.SnapshotID,
				"kind":          kind,
				"digest":        digest,
				"message_count": messageCount,
			}),
		})
	}
	return snapshot, digest, idempotencyKey, messageCount, nil
}

func (h *ADKTranscriptHooks) persistMemoryFlushSnapshot(
	ctx context.Context,
	kind TranscriptKind,
	digest string,
	idempotencyKey string,
	messageCount int32,
	raw string,
	sourceDigest string,
	sourceSnapshotID int64,
	memorySignal map[string]any,
) (ADKTranscriptSnapshot, error) {
	metadata, err := json.Marshal(map[string]any{
		"runtime":            "eino_adk",
		"kind":               kind,
		"digest":             digest,
		"purpose":            adkTranscriptMemoryFlushPurpose,
		"source_digest":      sourceDigest,
		"source_snapshot_id": sourceSnapshotID,
		"memory_flush":       memorySignal,
	})
	if err != nil {
		return ADKTranscriptSnapshot{}, fmt.Errorf(
			"encode eino adk memory transcript metadata: %w",
			err,
		)
	}
	snapshot, _, err := h.store.PersistTranscript(
		ctx,
		h.run,
		ADKTranscriptPersistRequest{
			Kind:           kind,
			Digest:         digest,
			IdempotencyKey: idempotencyKey,
			MessageCount:   messageCount,
			Messages:       raw,
			Metadata:       string(metadata),
		},
	)
	if err != nil {
		return ADKTranscriptSnapshot{}, fmt.Errorf(
			"persist eino adk memory transcript: %w",
			err,
		)
	}
	if snapshot.SnapshotID <= 0 {
		return ADKTranscriptSnapshot{}, fmt.Errorf(
			"persist eino adk memory transcript returned empty snapshot",
		)
	}
	return snapshot, nil
}

type ADKTranscriptMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	hooks *ADKTranscriptHooks
}

func NewADKTranscriptMiddleware(
	hooks *ADKTranscriptHooks,
) *ADKTranscriptMiddleware {
	return &ADKTranscriptMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		hooks:                        hooks,
	}
}

func (m *ADKTranscriptMiddleware) AfterAgent(
	ctx context.Context,
	state *adk.ChatModelAgentState,
) (context.Context, error) {
	if m == nil || m.hooks == nil {
		return ctx, nil
	}
	if state == nil {
		return nil, fmt.Errorf("eino adk terminal state is required")
	}
	if err := m.hooks.PersistTerminal(ctx, state.Messages); err != nil {
		return nil, err
	}
	return ctx, nil
}

func encodeADKTranscript(
	messages []*schema.Message,
) (string, string, int32, error) {
	normalized := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message != nil {
			normalized = append(normalized, message)
		}
	}
	if len(normalized) == 0 {
		return "", "", 0, fmt.Errorf("transcript messages are required")
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", "", 0, fmt.Errorf("encode eino adk transcript: %w", err)
	}
	canonical, err := canonicalADKTranscript(raw)
	if err != nil {
		return "", "", 0, err
	}
	sum := sha256.Sum256(canonical)
	return string(raw), hex.EncodeToString(sum[:]), int32(len(normalized)), nil
}

func canonicalADKTranscript(raw []byte) ([]byte, error) {
	var messages []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &messages); err != nil {
		return nil, fmt.Errorf("decode eino adk transcript for digest: %w", err)
	}
	for _, message := range messages {
		rawExtra, exists := message["extra"]
		if !exists {
			continue
		}
		var extra map[string]json.RawMessage
		if err := json.Unmarshal(rawExtra, &extra); err != nil {
			return nil, fmt.Errorf("decode eino adk transcript extra for digest: %w", err)
		}
		delete(extra, einoMessageIDExtraKey)
		if len(extra) == 0 {
			delete(message, "extra")
			continue
		}
		normalizedExtra, err := json.Marshal(extra)
		if err != nil {
			return nil, fmt.Errorf("encode eino adk transcript extra for digest: %w", err)
		}
		message["extra"] = normalizedExtra
	}
	canonical, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("encode eino adk transcript digest: %w", err)
	}
	return canonical, nil
}

func adkTranscriptMemoryEligible(messages []*schema.Message) bool {
	hasUser := false
	hasAssistantAnswer := false
	for _, message := range messages {
		if message == nil {
			continue
		}
		switch message.Role {
		case schema.User:
			hasUser = hasUser || strings.TrimSpace(message.Content) != "" ||
				len(message.UserInputMultiContent) > 0 ||
				len(message.MultiContent) > 0
		case schema.Assistant:
			hasAssistantAnswer = hasAssistantAnswer ||
				(strings.TrimSpace(message.Content) != "" &&
					len(message.ToolCalls) == 0)
		}
	}
	return hasUser && hasAssistantAnswer
}

func buildADKTranscriptMemoryMessages(messages []*schema.Message) []*schema.Message {
	filtered := make([]*schema.Message, 0, len(messages))
	skipNextAssistant := false
	for _, message := range messages {
		if message == nil || isADKHiddenTranscriptMessage(message) {
			continue
		}
		switch message.Role {
		case schema.User:
			content, uploadOnly := sanitizeADKTranscriptUserMemoryContent(message)
			if content == "" {
				if uploadOnly {
					skipNextAssistant = true
				}
				continue
			}
			skipNextAssistant = false
			filtered = append(filtered, schema.UserMessage(content))
		case schema.Assistant:
			if skipNextAssistant {
				skipNextAssistant = false
				continue
			}
			content := strings.TrimSpace(message.Content)
			if content == "" || len(message.ToolCalls) > 0 {
				continue
			}
			filtered = append(filtered, schema.AssistantMessage(content, nil))
		}
	}
	return filtered
}

func buildADKTranscriptMemoryFlushMetadata(messages []*schema.Message) map[string]any {
	correction := detectADKTranscriptMemorySignal(messages, adkTranscriptCorrectionREs)
	reinforcement := false
	if !correction {
		reinforcement = detectADKTranscriptMemorySignal(
			messages,
			adkTranscriptReinforcementREs,
		)
	}
	return map[string]any{
		"purpose":                adkTranscriptMemoryFlushPurpose,
		"correction_detected":    correction,
		"reinforcement_detected": reinforcement,
		"schema":                 "coze.adk_memory_flush.v1",
	}
}

func detectADKTranscriptMemorySignal(
	messages []*schema.Message,
	patterns []*regexp.Regexp,
) bool {
	seenUsers := 0
	for index := len(messages) - 1; index >= 0 && seenUsers < 6; index-- {
		message := messages[index]
		if message == nil || message.Role != schema.User {
			continue
		}
		seenUsers++
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		for _, pattern := range patterns {
			if pattern.MatchString(content) {
				return true
			}
		}
	}
	return false
}

func isADKHiddenTranscriptMessage(message *schema.Message) bool {
	if message == nil {
		return true
	}
	if isADKDynamicContextReminder(message) {
		return true
	}
	if message.Extra != nil {
		if hidden, _ := message.Extra[adkHideFromUIExtraKey].(bool); hidden {
			return true
		}
		if _, summarized := message.Extra[adkSummarizationContentTypeKey]; summarized {
			return true
		}
	}
	return false
}

func sanitizeADKTranscriptUserMemoryContent(message *schema.Message) (string, bool) {
	if message == nil {
		return "", false
	}
	parts := make([]string, 0, 1+len(message.UserInputMultiContent)+len(message.MultiContent))
	if strings.TrimSpace(message.Content) != "" {
		parts = append(parts, message.Content)
	}
	for _, part := range message.UserInputMultiContent {
		if part.Type == schema.ChatMessagePartTypeText && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	for _, part := range message.MultiContent {
		if part.Type == schema.ChatMessagePartTypeText && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	raw := strings.Join(parts, "\n")
	hadUploadBlock := adkTranscriptUploadedFilesRE.MatchString(raw)
	sanitized := strings.TrimSpace(adkTranscriptUploadedFilesRE.ReplaceAllString(raw, ""))
	return sanitized, hadUploadBlock && sanitized == ""
}
