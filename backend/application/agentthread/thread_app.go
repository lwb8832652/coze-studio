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

import "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"

func DomainThreadToSummary(thread *entity.Thread) *ThreadSummary {
	if thread == nil {
		return nil
	}

	return &ThreadSummary{
		ThreadID:  thread.ID,
		SpaceID:   thread.SpaceID,
		CreatorID: thread.CreatorID,
		Title:     thread.Title,
		Status:    ThreadStatus(thread.Status),
		Source:    ThreadSource(thread.Source),
		Metadata:  thread.Metadata,
		CreatedAt: thread.CreatedAt,
		UpdatedAt: thread.UpdatedAt,
	}
}

func DomainMessageToSummary(message *entity.Message) *MessageSummary {
	if message == nil {
		return nil
	}

	return &MessageSummary{
		MessageID: message.ID,
		ThreadID:  message.ThreadID,
		RunID:     message.RunID,
		Role:      MessageRole(message.Role),
		Content:   message.Content,
		Metadata:  message.Metadata,
		CreatedAt: message.CreatedAt,
	}
}

func DomainRunToSummary(run *entity.Run) *RunSummary {
	if run == nil {
		return nil
	}

	return &RunSummary{
		RunID:               run.ID,
		ThreadID:            run.ThreadID,
		ParentRunID:         run.ParentRunID,
		SpaceID:             run.SpaceID,
		CreatorID:           run.CreatorID,
		AssistantID:         run.AssistantID,
		RunKind:             RunKind(run.RunKind),
		Status:              RunStatus(run.Status),
		Command:             run.Command,
		Input:               run.Input,
		Config:              run.Config,
		Context:             run.Context,
		Metadata:            run.Metadata,
		StreamMode:          run.StreamMode,
		MultitaskStrategy:   run.MultitaskStrategy,
		OnDisconnect:        run.OnDisconnect,
		Durability:          run.Durability,
		IdempotencyKey:      run.IdempotencyKey,
		WorkerID:            run.WorkerID,
		LeaseOwner:          run.LeaseOwner,
		LeaseToken:          run.LeaseToken,
		LeaseExpiresAt:      run.LeaseExpiresAt,
		HeartbeatAt:         run.HeartbeatAt,
		CancelRequestedAt:   run.CancelRequestedAt,
		ExecutionGeneration: run.ExecutionGeneration,
		ErrorCode:           run.ErrorCode,
		ErrorMessage:        run.ErrorMessage,
		StartedAt:           run.StartedAt,
		EndedAt:             run.EndedAt,
		CreatedAt:           run.CreatedAt,
		UpdatedAt:           run.UpdatedAt,
	}
}

func DomainRunEventToSummary(event *entity.RunEvent) *RunEventSummary {
	if event == nil {
		return nil
	}

	return &RunEventSummary{
		EventID:   event.ID,
		ThreadID:  event.ThreadID,
		RunID:     event.RunID,
		EventType: event.EventType,
		Payload:   event.Payload,
		CreatedAt: event.CreatedAt,
	}
}

func DomainCheckpointToSummary(checkpoint *entity.Checkpoint) *CheckpointSummary {
	if checkpoint == nil {
		return nil
	}

	return &CheckpointSummary{
		CheckpointID:       checkpoint.ID,
		ThreadID:           checkpoint.ThreadID,
		RunID:              checkpoint.RunID,
		ParentCheckpointID: checkpoint.ParentCheckpointID,
		CheckpointNS:       checkpoint.CheckpointNS,
		RuntimeType:        checkpoint.RuntimeType,
		RuntimeKey:         checkpoint.RuntimeKey,
		EnvelopeVersion:    checkpoint.EnvelopeVersion,
		RuntimeDeletedAt:   checkpoint.RuntimeDeletedAt,
		ChannelValues:      checkpoint.ChannelValues,
		ChannelVersions:    checkpoint.ChannelVersions,
		PendingSends:       checkpoint.PendingSends,
		Metadata:           checkpoint.Metadata,
		CreatedAt:          checkpoint.CreatedAt,
	}
}

func DomainMemoryToSummary(memory *entity.Memory) *MemorySummary {
	if memory == nil {
		return nil
	}

	return &MemorySummary{
		MemoryID:             memory.ID,
		ThreadID:             memory.ThreadID,
		RunID:                memory.RunID,
		SpaceID:              memory.SpaceID,
		Scope:                MemoryScope(memory.Scope),
		Content:              memory.Content,
		Metadata:             memory.Metadata,
		Score:                memory.Score,
		Confidence:           memory.Confidence,
		SourceType:           memory.SourceType,
		SourceID:             memory.SourceID,
		CorrectionOfMemoryID: memory.CorrectionOfMemoryID,
		CorrectedAt:          memory.CorrectedAt,
		ExpiresAt:            memory.ExpiresAt,
		CreatedAt:            memory.CreatedAt,
		UpdatedAt:            memory.UpdatedAt,
		DeletedAt:            memory.DeletedAt,
	}
}

func DomainMemoryAuditEventToSummary(event *entity.MemoryAuditEvent) *MemoryAuditEventSummary {
	if event == nil {
		return nil
	}

	return &MemoryAuditEventSummary{
		EventID:       event.ID,
		ThreadID:      event.ThreadID,
		RunID:         event.RunID,
		SpaceID:       event.SpaceID,
		MemoryID:      event.MemoryID,
		ActorID:       event.ActorID,
		EventType:     event.EventType,
		Scope:         MemoryScope(event.Scope),
		SourceType:    event.SourceType,
		SourceID:      event.SourceID,
		AffectedCount: event.AffectedCount,
		CreatedAt:     event.CreatedAt,
	}
}

func DomainGuardrailAuditEventToSummary(
	event *entity.GuardrailAuditEvent,
) *GuardrailAuditEventSummary {
	if event == nil {
		return nil
	}

	return &GuardrailAuditEventSummary{
		EventID:    event.ID,
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
	}
}

func DomainMCPRuntimeAuditEventToSummary(
	event *entity.MCPRuntimeAuditEvent,
) *MCPRuntimeAuditEventSummary {
	if event == nil {
		return nil
	}

	return &MCPRuntimeAuditEventSummary{
		EventID:         event.ID,
		SpaceID:         event.SpaceID,
		ThreadID:        event.ThreadID,
		RunID:           event.RunID,
		ServerID:        event.ServerID,
		RuntimeToolName: event.RuntimeToolName,
		EventType:       event.EventType,
		ErrorCode:       event.ErrorCode,
		ElapsedMillis:   event.ElapsedMillis,
		OutputBytes:     event.OutputBytes,
		CreatedAt:       event.CreatedAt,
	}
}

func DomainTranscriptSnapshotToSummary(
	snapshot *entity.TranscriptSnapshot,
) *TranscriptSnapshotSummary {
	if snapshot == nil {
		return nil
	}
	return &TranscriptSnapshotSummary{
		SnapshotID:     snapshot.ID,
		ThreadID:       snapshot.ThreadID,
		RunID:          snapshot.RunID,
		SpaceID:        snapshot.SpaceID,
		Kind:           TranscriptKind(snapshot.Kind),
		Digest:         snapshot.Digest,
		IdempotencyKey: snapshot.IdempotencyKey,
		MessageCount:   snapshot.MessageCount,
		Messages:       snapshot.Messages,
		Metadata:       snapshot.Metadata,
		CreatedAt:      snapshot.CreatedAt,
	}
}

func DomainMemoryFlushJobToSummary(
	job *entity.MemoryFlushJob,
) *MemoryFlushJobSummary {
	if job == nil {
		return nil
	}
	return &MemoryFlushJobSummary{
		JobID:                job.ID,
		ThreadID:             job.ThreadID,
		RunID:                job.RunID,
		SpaceID:              job.SpaceID,
		UserID:               job.UserID,
		AssistantID:          job.AssistantID,
		TranscriptSnapshotID: job.TranscriptSnapshotID,
		IdempotencyKey:       job.IdempotencyKey,
		Status:               string(job.Status),
		AttemptCount:         job.AttemptCount,
		WorkerID:             job.WorkerID,
		LastError:            job.LastError,
		AvailableAt:          job.AvailableAt,
		LeaseExpiresAt:       job.LeaseExpiresAt,
		StartedAt:            job.StartedAt,
		EndedAt:              job.EndedAt,
		CreatedAt:            job.CreatedAt,
		UpdatedAt:            job.UpdatedAt,
	}
}

func DomainArtifactToSummary(artifact *entity.AgentArtifact) *ArtifactSummary {
	if artifact == nil {
		return nil
	}
	contentType := legacyArtifactDownloadContentType
	sizeBytes := int64(0)
	previewMode := entity.AgentArtifactPreviewModeDownload
	if trustedContentType, trustedSizeBytes, err := validatedArtifactScanMetadata(artifact); err == nil {
		contentType = trustedContentType
		sizeBytes = trustedSizeBytes
		previewMode = artifact.PreviewMode
	}
	return &ArtifactSummary{
		ArtifactID:       artifact.ID,
		SpaceID:          artifact.SpaceID,
		ThreadID:         artifact.ThreadID,
		RunID:            artifact.RunID,
		JournalRunID:     artifact.JournalRunID,
		FileID:           artifact.FileID,
		Title:            artifact.Title,
		ArtifactType:     artifact.ArtifactType,
		VirtualPath:      artifact.VirtualPath,
		ContentType:      contentType,
		SizeBytes:        sizeBytes,
		PreviewMode:      ArtifactPreviewMode(previewMode),
		Source:           string(artifact.Source),
		GenerationStatus: string(artifact.GenerationStatus),
		Capabilities:     artifactPublicCapabilities(artifact),
		IsPrimary:        artifact.IsPrimary,
		CollectionID:     artifact.CollectionID,
		CollectionOrder:  artifact.CollectionOrder,
		Metadata:         artifact.Metadata,
		CreatedAt:        artifact.CreatedAt,
		UpdatedAt:        artifact.UpdatedAt,
		DeletedAt:        artifact.DeletedAt,
	}
}

func DomainArtifactScanJobToSummary(
	job *entity.ArtifactScanJob,
) *ArtifactScanJobSummary {
	if job == nil {
		return nil
	}
	return &ArtifactScanJobSummary{
		JobID:          job.ID,
		ThreadID:       job.ThreadID,
		RunID:          job.RunID,
		SpaceID:        job.SpaceID,
		UserID:         job.UserID,
		ArtifactID:     job.ArtifactID,
		FileID:         job.FileID,
		Scanner:        job.Scanner,
		Status:         ArtifactScanJobStatus(job.Status),
		WorkerID:       job.WorkerID,
		AttemptCount:   job.AttemptCount,
		LastError:      job.LastError,
		AvailableAt:    job.AvailableAt,
		LeaseExpiresAt: job.LeaseExpiresAt,
		StartedAt:      job.StartedAt,
		EndedAt:        job.EndedAt,
		CreatedAt:      job.CreatedAt,
		UpdatedAt:      job.UpdatedAt,
	}
}

func DomainTokenUsageToSummary(usage *entity.TokenUsage) *TokenUsageSummary {
	if usage == nil {
		return nil
	}

	return &TokenUsageSummary{
		UsageID:      usage.ID,
		ThreadID:     usage.ThreadID,
		RunID:        usage.RunID,
		SpaceID:      usage.SpaceID,
		Source:       TokenUsageSource(usage.Source),
		StepID:       usage.StepID,
		StepIndex:    usage.StepIndex,
		StepName:     usage.StepName,
		ModelName:    usage.ModelName,
		Provider:     usage.Provider,
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
		CostMicros:   usage.CostMicros,
		Currency:     usage.Currency,
		Estimated:    usage.Estimated,
		RawUsage:     usage.RawUsage,
		Metadata:     usage.Metadata,
		CreatedAt:    usage.CreatedAt,
	}
}

func DomainTokenUsageAggregateToSummary(aggregate *entity.TokenUsageAggregate) *TokenUsageAggregateSummary {
	if aggregate == nil {
		return &TokenUsageAggregateSummary{}
	}

	return &TokenUsageAggregateSummary{
		InputTokens:      aggregate.InputTokens,
		OutputTokens:     aggregate.OutputTokens,
		TotalTokens:      aggregate.TotalTokens,
		CostMicros:       aggregate.CostMicros,
		CallCount:        aggregate.CallCount,
		LeadAgentTokens:  aggregate.LeadAgentTokens,
		SubagentTokens:   aggregate.SubagentTokens,
		MiddlewareTokens: aggregate.MiddlewareTokens,
		ToolTokens:       aggregate.ToolTokens,
	}
}

func DomainRunTokenUsageAggregateToSummary(aggregate *entity.RunTokenUsageAggregate) *RunTokenUsageAggregateSummary {
	if aggregate == nil {
		return nil
	}

	return &RunTokenUsageAggregateSummary{
		RunID:     aggregate.RunID,
		Aggregate: DomainTokenUsageAggregateToSummary(aggregate.Aggregate),
	}
}
