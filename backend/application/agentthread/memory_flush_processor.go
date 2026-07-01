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
	"strings"
	"time"
	"unicode"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const (
	defaultMemoryFlushRetryBackoffMillis = int64(60000)
	defaultMemoryFlushMaxAttempts        = int32(3)
	memoryFlushSourceTypeTranscript      = "transcript_summary"
	memoryFlushCompletedEvent            = "memory.update_completed"
	memoryFlushFailedEvent               = "memory.update_failed"
)

type MemoryExtractionRequest struct {
	ThreadID       int64
	RunID          int64
	SpaceID        int64
	SnapshotID     int64
	Kind           TranscriptKind
	Digest         string
	IdempotencyKey string
	MessageCount   int32
	Messages       string
	Metadata       string
	CurrentMemory  string
}

type MemoryExtractionFact struct {
	Key                  string
	Scope                MemoryScope
	Content              string
	Metadata             string
	Score                float64
	Confidence           float64
	SourceType           string
	SourceID             string
	CorrectionOfMemoryID int64
	CorrectedAt          int64
	ExpiresAt            int64
}

type MemoryExtractionResult struct {
	Facts         []MemoryExtractionFact
	FactsToRemove []int64
}

type MemoryExtractor interface {
	ExtractMemories(ctx context.Context, req MemoryExtractionRequest) ([]MemoryExtractionFact, error)
}

type MemoryUpdateExtractor interface {
	ExtractMemoryUpdates(ctx context.Context, req MemoryExtractionRequest) (*MemoryExtractionResult, error)
}

type memoryFlushJobProcessOutcome int

const (
	memoryFlushJobProcessSkipped memoryFlushJobProcessOutcome = iota
	memoryFlushJobProcessSucceeded
	memoryFlushJobProcessRetried
	memoryFlushJobProcessFailed
)

func (s *ApplicationService) ProcessMemoryFlushJobs(
	ctx context.Context,
	req *ProcessMemoryFlushJobsRequest,
) (*ProcessMemoryFlushJobsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if s.MemoryExtractor == nil {
		return nil, fmt.Errorf("memory extractor is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("process memory flush jobs request is required")
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, fmt.Errorf("worker id is required")
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMemoryFlushMaxAttempts
	}
	retryBackoffMillis := req.RetryBackoffMillis
	if retryBackoffMillis <= 0 {
		retryBackoffMillis = defaultMemoryFlushRetryBackoffMillis
	}

	jobs, err := s.ThreadSVC.ClaimMemoryFlushJobs(
		ctx,
		&domainservice.ClaimMemoryFlushJobsRequest{
			WorkerID:       workerID,
			Limit:          req.Limit,
			LeaseTTLMillis: req.LeaseTTLMillis,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ProcessMemoryFlushJobsResponse{Claimed: int32(len(jobs))}
	for _, job := range jobs {
		outcome, err := s.processMemoryFlushJob(
			ctx,
			job,
			workerID,
			maxAttempts,
			retryBackoffMillis,
		)
		if err != nil {
			return nil, err
		}
		switch outcome {
		case memoryFlushJobProcessSucceeded:
			resp.Succeeded++
		case memoryFlushJobProcessRetried:
			resp.Retried++
		case memoryFlushJobProcessFailed:
			resp.Failed++
		default:
			resp.Skipped++
		}
	}
	return resp, nil
}

func (s *ApplicationService) processMemoryFlushJob(
	ctx context.Context,
	job *domainentity.MemoryFlushJob,
	workerID string,
	maxAttempts int32,
	retryBackoffMillis int64,
) (memoryFlushJobProcessOutcome, error) {
	if job == nil {
		return memoryFlushJobProcessSkipped, nil
	}
	snapshot, err := s.ThreadSVC.GetTranscriptSnapshot(
		ctx,
		&domainservice.GetTranscriptSnapshotRequest{
			SnapshotID: job.TranscriptSnapshotID,
		},
	)
	if err != nil {
		return s.failClaimedMemoryFlushJob(
			ctx,
			job,
			workerID,
			"memory transcript snapshot unavailable",
			maxAttempts,
			retryBackoffMillis,
		)
	}
	if !memoryFlushSnapshotMatchesJob(snapshot, job) {
		return s.failClaimedMemoryFlushJob(
			ctx,
			job,
			workerID,
			"memory transcript snapshot invalid",
			maxAttempts,
			retryBackoffMillis,
		)
	}

	currentMemory, err := s.memoryFlushCurrentMemoryState(ctx, job)
	if err != nil {
		return s.failClaimedMemoryFlushJob(
			ctx,
			job,
			workerID,
			"memory current state unavailable",
			maxAttempts,
			retryBackoffMillis,
		)
	}

	updates, err := extractMemoryUpdates(ctx, s.MemoryExtractor, MemoryExtractionRequest{
		ThreadID:       snapshot.ThreadID,
		RunID:          snapshot.RunID,
		SpaceID:        snapshot.SpaceID,
		SnapshotID:     snapshot.ID,
		Kind:           TranscriptKind(snapshot.Kind),
		Digest:         snapshot.Digest,
		IdempotencyKey: snapshot.IdempotencyKey,
		MessageCount:   snapshot.MessageCount,
		Messages:       snapshot.Messages,
		Metadata:       snapshot.Metadata,
		CurrentMemory:  currentMemory,
	})
	if err != nil {
		return s.failClaimedMemoryFlushJob(
			ctx,
			job,
			workerID,
			memoryFlushSafeErrorText("memory extraction failed", err),
			maxAttempts,
			retryBackoffMillis,
		)
	}
	if updates == nil {
		updates = &MemoryExtractionResult{}
	}

	removed, removeSkipped, err := s.removeExtractedMemoryFacts(ctx, job, updates.FactsToRemove)
	if err != nil {
		return s.failClaimedMemoryFlushJob(
			ctx,
			job,
			workerID,
			"memory delete failed",
			maxAttempts,
			retryBackoffMillis,
		)
	}

	written, skipped, err := s.rememberExtractedMemoryFacts(ctx, job, snapshot, updates.Facts)
	if err != nil {
		return s.failClaimedMemoryFlushJob(
			ctx,
			job,
			workerID,
			"memory write failed",
			maxAttempts,
			retryBackoffMillis,
		)
	}
	now := time.Now().UnixMilli()
	_, ok, err := s.ThreadSVC.CompleteMemoryFlushJob(
		ctx,
		&domainservice.CompleteMemoryFlushJobRequest{
			JobID:    job.ID,
			WorkerID: workerID,
			Now:      now,
		},
	)
	if err != nil {
		return memoryFlushJobProcessSkipped, err
	}
	if !ok {
		return memoryFlushJobProcessSkipped, nil
	}
	s.emitMemoryFlushEvent(ctx, job, snapshot, memoryFlushCompletedEvent, map[string]any{
		"status":               "succeeded",
		"facts_written":        written,
		"facts_skipped":        skipped,
		"facts_removed":        removed,
		"facts_remove_skipped": removeSkipped,
	})
	return memoryFlushJobProcessSucceeded, nil
}

func extractMemoryUpdates(
	ctx context.Context,
	extractor MemoryExtractor,
	req MemoryExtractionRequest,
) (*MemoryExtractionResult, error) {
	if extractor == nil {
		return nil, fmt.Errorf("memory extractor is not configured")
	}
	if updateExtractor, ok := extractor.(MemoryUpdateExtractor); ok {
		return updateExtractor.ExtractMemoryUpdates(ctx, req)
	}
	facts, err := extractor.ExtractMemories(ctx, req)
	if err != nil {
		return nil, err
	}
	return &MemoryExtractionResult{Facts: facts}, nil
}

func memoryFlushSafeErrorText(prefix string, err error) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "memory flush failed"
	}
	if err == nil {
		return prefix
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "memory extractor is not configured"):
		return prefix + ": extractor_not_configured"
	case strings.Contains(message, "resolve memory extraction model"):
		return prefix + ": resolve_model_failed"
	case strings.Contains(message, "memory extraction model is not configured"):
		return prefix + ": model_not_configured"
	case strings.Contains(message, "run memory extraction model"):
		return prefix + ": model_call_failed"
	case strings.Contains(message, "memory extraction model returned empty response"):
		return prefix + ": empty_model_response"
	case strings.Contains(message, "decode memory extraction model output"):
		return prefix + ": decode_failed"
	case strings.Contains(message, "record memory extraction usage"):
		return prefix + ": usage_record_failed"
	default:
		return prefix + ": extractor_failed"
	}
}

func (s *ApplicationService) memoryFlushCurrentMemoryState(
	ctx context.Context,
	job *domainentity.MemoryFlushJob,
) (string, error) {
	if s == nil || s.ThreadSVC == nil || job == nil {
		return "{}", nil
	}
	memories, _, err := s.ThreadSVC.RecallMemories(
		ctx,
		&domainservice.RecallMemoriesRequest{
			ThreadID: job.ThreadID,
			RunID:    job.RunID,
			Scopes: []domainentity.MemoryScope{
				domainentity.MemoryScopeLongTerm,
				domainentity.MemoryScopeThread,
				domainentity.MemoryScopeRun,
			},
			Limit: 100,
		},
	)
	if err != nil {
		return "", err
	}
	return buildMemoryFlushCurrentMemoryDocument(memories), nil
}

func buildMemoryFlushCurrentMemoryDocument(memories []*domainentity.Memory) string {
	newSection := func() map[string]string {
		return map[string]string{"summary": ""}
	}
	user := map[string]map[string]string{
		"workContext":     newSection(),
		"personalContext": newSection(),
		"topOfMind":       newSection(),
	}
	history := map[string]map[string]string{
		"recentMonths":       newSection(),
		"earlierContext":     newSection(),
		"longTermBackground": newSection(),
	}
	sectionConfidence := make(map[string]float64)
	facts := make([]map[string]any, 0, len(memories))
	for _, memory := range memories {
		if memory == nil || memory.DeletedAt > 0 {
			continue
		}
		content := strings.TrimSpace(memory.Content)
		if content == "" {
			continue
		}
		metadata := adkMemoryMetadata(memory.Metadata)
		if section, ok := adkStructuredMemorySection(metadata); ok && section.path != "" {
			confidence := memoryFlushMemoryConfidence(memory)
			if confidence >= sectionConfidence[section.path] {
				if section.group == "user" {
					user[strings.TrimPrefix(section.path, "user.")]["summary"] = content
				} else if section.group == "history" {
					history[strings.TrimPrefix(section.path, "history.")]["summary"] = content
				}
				sectionConfidence[section.path] = confidence
			}
			continue
		}
		category := firstADKMemoryMetadataString(metadata, "category")
		if category == "" {
			category = string(memory.Scope)
		}
		if category == "" {
			category = "context"
		}
		fact := map[string]any{
			"id":         fmt.Sprintf("memory_%d", memory.ID),
			"content":    content,
			"category":   category,
			"confidence": memoryFlushMemoryConfidence(memory),
		}
		if sourceError := firstADKMemoryMetadataString(metadata, "sourceError", "source_error"); sourceError != "" {
			fact["sourceError"] = sourceError
		}
		facts = append(facts, fact)
	}
	raw, err := json.Marshal(map[string]any{
		"version": "1.0",
		"user":    user,
		"history": history,
		"facts":   facts,
	})
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func memoryFlushMemoryConfidence(memory *domainentity.Memory) float64 {
	if memory == nil {
		return 0
	}
	if memory.Confidence > 0 {
		return memory.Confidence
	}
	if memory.Score > 0 {
		return memory.Score
	}
	return 0
}

func (s *ApplicationService) rememberExtractedMemoryFacts(
	ctx context.Context,
	job *domainentity.MemoryFlushJob,
	snapshot *domainentity.TranscriptSnapshot,
	facts []MemoryExtractionFact,
) (int32, int32, error) {
	var written int32
	var skipped int32
	for index, fact := range facts {
		content := strings.TrimSpace(fact.Content)
		if content == "" {
			skipped++
			continue
		}
		sourceType := strings.TrimSpace(fact.SourceType)
		if sourceType == "" {
			sourceType = memoryFlushSourceTypeTranscript
		}
		sourceID := memoryFlushFactSourceID(snapshot, fact, index)
		exists, err := s.memoryFlushFactExists(ctx, job.ThreadID, job.RunID, sourceType, sourceID)
		if err != nil {
			return written, skipped, err
		}
		if exists {
			skipped++
			continue
		}
		_, err = s.ThreadSVC.RememberMemory(
			ctx,
			&domainservice.RememberMemoryRequest{
				ThreadID:             job.ThreadID,
				RunID:                job.RunID,
				Scope:                domainentity.MemoryScope(memoryFlushFactScope(fact.Scope)),
				Content:              content,
				Metadata:             memoryFlushFactMetadata(fact.Metadata),
				Score:                fact.Score,
				Confidence:           fact.Confidence,
				SourceType:           sourceType,
				SourceID:             sourceID,
				CorrectionOfMemoryID: fact.CorrectionOfMemoryID,
				CorrectedAt:          fact.CorrectedAt,
				ExpiresAt:            fact.ExpiresAt,
			},
		)
		if err != nil {
			return written, skipped, err
		}
		written++
	}
	return written, skipped, nil
}

func (s *ApplicationService) removeExtractedMemoryFacts(
	ctx context.Context,
	job *domainentity.MemoryFlushJob,
	memoryIDs []int64,
) (int32, int32, error) {
	if s == nil || s.ThreadSVC == nil || job == nil || len(memoryIDs) == 0 {
		return 0, 0, nil
	}
	seen := make(map[int64]struct{}, len(memoryIDs))
	var removed int32
	var skipped int32
	for _, memoryID := range memoryIDs {
		if memoryID <= 0 {
			skipped++
			continue
		}
		if _, exists := seen[memoryID]; exists {
			skipped++
			continue
		}
		seen[memoryID] = struct{}{}
		deleted, err := s.ThreadSVC.DeleteMemory(ctx, &domainservice.DeleteMemoryRequest{
			ThreadID: job.ThreadID,
			MemoryID: memoryID,
			ActorID:  job.UserID,
		})
		if err != nil {
			return removed, skipped, err
		}
		if deleted {
			removed++
			continue
		}
		skipped++
	}
	return removed, skipped, nil
}

func (s *ApplicationService) memoryFlushFactExists(
	ctx context.Context,
	threadID int64,
	runID int64,
	sourceType string,
	sourceID string,
) (bool, error) {
	if strings.TrimSpace(sourceType) == "" || strings.TrimSpace(sourceID) == "" {
		return false, nil
	}
	memories, _, err := s.ThreadSVC.RecallMemories(
		ctx,
		&domainservice.RecallMemoriesRequest{
			ThreadID: threadID,
			RunID:    runID,
			Scopes: []domainentity.MemoryScope{
				domainentity.MemoryScopeLongTerm,
				domainentity.MemoryScopeThread,
				domainentity.MemoryScopeRun,
			},
			Limit: 100,
		},
	)
	if err != nil {
		return false, err
	}
	for _, memory := range memories {
		if memory == nil {
			continue
		}
		if strings.TrimSpace(memory.SourceType) == sourceType &&
			strings.TrimSpace(memory.SourceID) == sourceID {
			return true, nil
		}
	}
	return false, nil
}

func (s *ApplicationService) failClaimedMemoryFlushJob(
	ctx context.Context,
	job *domainentity.MemoryFlushJob,
	workerID string,
	errorText string,
	maxAttempts int32,
	retryBackoffMillis int64,
) (memoryFlushJobProcessOutcome, error) {
	if job == nil {
		return memoryFlushJobProcessSkipped, nil
	}
	now := time.Now().UnixMilli()
	if shouldRetryMemoryFlushJob(job, maxAttempts) {
		_, ok, err := s.ThreadSVC.RetryMemoryFlushJob(
			ctx,
			&domainservice.RetryMemoryFlushJobRequest{
				JobID:       job.ID,
				WorkerID:    workerID,
				ErrorText:   errorText,
				AvailableAt: now + retryBackoffMillis,
				Now:         now,
			},
		)
		if err != nil {
			return memoryFlushJobProcessSkipped, err
		}
		if !ok {
			return memoryFlushJobProcessSkipped, nil
		}
		return memoryFlushJobProcessRetried, nil
	}
	_, ok, err := s.ThreadSVC.FailMemoryFlushJob(
		ctx,
		&domainservice.FailMemoryFlushJobRequest{
			JobID:     job.ID,
			WorkerID:  workerID,
			ErrorText: errorText,
			Now:       now,
		},
	)
	if err != nil {
		return memoryFlushJobProcessSkipped, err
	}
	if !ok {
		return memoryFlushJobProcessSkipped, nil
	}
	s.emitMemoryFlushEvent(ctx, job, nil, memoryFlushFailedEvent, map[string]any{
		"status": "failed",
		"error":  errorText,
	})
	return memoryFlushJobProcessFailed, nil
}

func (s *ApplicationService) emitMemoryFlushEvent(
	ctx context.Context,
	job *domainentity.MemoryFlushJob,
	snapshot *domainentity.TranscriptSnapshot,
	eventType string,
	fields map[string]any,
) {
	if s == nil || s.ThreadSVC == nil || job == nil {
		return
	}
	payload := map[string]any{
		"schema":      "coze.memory_flush.v1",
		"job_id":      job.ID,
		"thread_id":   job.ThreadID,
		"run_id":      job.RunID,
		"snapshot_id": job.TranscriptSnapshotID,
	}
	if snapshot != nil {
		payload["kind"] = snapshot.Kind
		payload["digest"] = snapshot.Digest
		payload["message_count"] = snapshot.MessageCount
	}
	for key, value := range fields {
		payload[key] = value
	}
	_, _ = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  job.ThreadID,
		RunID:     job.RunID,
		EventType: eventType,
		Payload:   encodeRunEventPayload(ctx, payload),
	})
}

func memoryFlushSnapshotMatchesJob(
	snapshot *domainentity.TranscriptSnapshot,
	job *domainentity.MemoryFlushJob,
) bool {
	if snapshot == nil || job == nil {
		return false
	}
	return snapshot.ID == job.TranscriptSnapshotID &&
		snapshot.ThreadID == job.ThreadID &&
		snapshot.RunID == job.RunID &&
		snapshot.SpaceID == job.SpaceID
}

func shouldRetryMemoryFlushJob(job *domainentity.MemoryFlushJob, maxAttempts int32) bool {
	return job != nil && maxAttempts > 1 && job.AttemptCount < maxAttempts
}

func memoryFlushFactScope(scope MemoryScope) MemoryScope {
	switch scope {
	case MemoryScopeThread, MemoryScopeRun, MemoryScopeLongTerm:
		return scope
	default:
		return MemoryScopeLongTerm
	}
}

func memoryFlushFactMetadata(metadata string) string {
	metadata = strings.TrimSpace(metadata)
	if metadata == "" || !json.Valid([]byte(metadata)) {
		return "{}"
	}
	return metadata
}

func memoryFlushFactSourceID(
	snapshot *domainentity.TranscriptSnapshot,
	fact MemoryExtractionFact,
	index int,
) string {
	prefix := "snapshot:0"
	if snapshot != nil {
		prefix = fmt.Sprintf("snapshot:%d", snapshot.ID)
	}
	key := strings.TrimSpace(fact.SourceID)
	if key == "" {
		key = strings.TrimSpace(fact.Key)
	}
	if key == "" {
		sum := sha256.Sum256([]byte(strings.TrimSpace(fact.Content)))
		key = "fact:" + hex.EncodeToString(sum[:8])
	}
	key = memoryFlushSourceIDPart(key)
	if key == "" {
		key = fmt.Sprintf("fact:%d", index)
	}
	return prefix + ":" + key
}

func memoryFlushSourceIDPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for _, current := range value {
		if unicode.IsLetter(current) || unicode.IsNumber(current) ||
			current == '_' || current == '-' {
			builder.WriteRune(current)
			continue
		}
		if builder.Len() > 0 {
			builder.WriteByte('_')
		}
	}
	part := strings.Trim(builder.String(), "_")
	if len(part) > 64 {
		part = part[:64]
	}
	return part
}
