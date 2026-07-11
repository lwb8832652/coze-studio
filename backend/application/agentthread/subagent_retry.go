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
	"fmt"
	"strings"
	"time"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const subagentRetryRequestedEventType = "subagent.retry.requested"

func (s *ApplicationService) RetrySubagentRun(
	ctx context.Context,
	req *RetrySubagentRunRequest,
) (*RetrySubagentRunResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("retry subagent run request is required")
	}
	if req.ThreadID <= 0 {
		return nil, fmt.Errorf("thread id is required")
	}
	if req.SourceRunID <= 0 {
		return nil, fmt.Errorf("source run id is required")
	}

	sourceRun, err := s.ThreadSVC.GetRun(ctx, &domainservice.GetRunRequest{RunID: req.SourceRunID})
	if err != nil {
		return nil, err
	}
	if sourceRun == nil {
		return nil, fmt.Errorf("source subagent run is missing")
	}
	if sourceRun.ThreadID != req.ThreadID {
		return nil, fmt.Errorf("source subagent run does not belong to thread")
	}
	if sourceRun.RunKind != domainentity.RunKindSubagent || sourceRun.ParentRunID <= 0 {
		return nil, fmt.Errorf("source run must be a subagent run")
	}
	if sourceRun.Status != domainentity.RunStatusFailed &&
		sourceRun.Status != domainentity.RunStatusCanceled {
		return nil, fmt.Errorf("source subagent run must be failed or canceled")
	}

	parentRun, err := s.ThreadSVC.GetRun(ctx, &domainservice.GetRunRequest{RunID: sourceRun.ParentRunID})
	if err != nil {
		return nil, err
	}
	if parentRun == nil {
		return nil, fmt.Errorf("parent run is missing")
	}
	if parentRun.ThreadID != req.ThreadID {
		return nil, fmt.Errorf("parent run does not belong to thread")
	}

	idempotencyKey, err := subagentRetryIdempotencyKey(req)
	if err != nil {
		return nil, err
	}
	existing, err := s.ThreadSVC.GetRunByIdempotencyKey(ctx, sourceRun.SpaceID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &RetrySubagentRunResponse{Run: DomainRunToSummary(existing)}, nil
	}

	requestedAt := time.Now().UnixMilli()
	command, err := subagentRetryCommand(parentRun, sourceRun, requestedAt)
	if err != nil {
		return nil, err
	}
	metadata, err := subagentRetryMetadata(parentRun, sourceRun, requestedAt)
	if err != nil {
		return nil, err
	}

	bundle, err := s.ThreadSVC.CreateRunBundle(ctx, &domainservice.CreateRunBundleRequest{
		Run: domainservice.CreateRunRequest{
			ThreadID: req.ThreadID, AssistantID: parentRun.AssistantID,
			RunKind: domainentity.RunKindTask, Status: domainentity.RunStatusQueued,
			Command: command, Input: `{"messages":[]}`, Config: parentRun.Config,
			Context: parentRun.Context, Metadata: metadata, StreamMode: parentRun.StreamMode,
			MultitaskStrategy: parentRun.MultitaskStrategy, OnDisconnect: parentRun.OnDisconnect,
			Durability: parentRun.Durability, IdempotencyKey: idempotencyKey,
		},
		Event: &domainservice.CreateRunEventSpec{
			EventType: subagentRetryRequestedEventType,
			PayloadBuilder: func(runID int64) string {
				return encodeRunEventPayload(ctx, subagentRetryRequestedPayload(
					parentRun, sourceRun, runID, requestedAt,
				))
			},
		},
	})
	if err != nil {
		return nil, err
	}
	if bundle == nil || bundle.Run == nil || bundle.Event == nil {
		return nil, fmt.Errorf("agent thread service returned incomplete subagent retry bundle")
	}

	return &RetrySubagentRunResponse{Run: DomainRunToSummary(bundle.Run)}, nil
}

func subagentRetryIdempotencyKey(req *RetrySubagentRunRequest) (string, error) {
	key := strings.TrimSpace(req.IdempotencyKey)
	if key != "" {
		if len(key) > 128 {
			return "", fmt.Errorf("idempotency key is invalid")
		}
		return key, nil
	}

	return fmt.Sprintf("subagent-retry:%d:%d", req.ThreadID, req.SourceRunID), nil
}

func subagentRetryCommand(parentRun, sourceRun *domainentity.Run, requestedAt int64) (string, error) {
	return marshalHumanInteractionJSON(map[string]any{
		"subagent_retry": map[string]any{
			"schema":             "coze.subagent_retry.v1",
			"source_run_id":      sourceRun.ID,
			"parent_run_id":      parentRun.ID,
			"source_status":      string(sourceRun.Status),
			"source_error_code":  strings.TrimSpace(sourceRun.ErrorCode),
			"parent_assistant":   strings.TrimSpace(parentRun.AssistantID),
			"requested_at":       requestedAt,
			"worker_contract":    "top_level_retry",
			"child_run_replayed": false,
		},
	}, "subagent retry command")
}

func subagentRetryMetadata(parentRun, sourceRun *domainentity.Run, requestedAt int64) (string, error) {
	return marshalHumanInteractionJSON(map[string]any{
		"source":        "subagent_retry",
		"source_run_id": sourceRun.ID,
		"parent_run_id": sourceRun.ParentRunID,
		"thread_id":     sourceRun.ThreadID,
		"requested_at":  requestedAt,
		"subagent_retry": map[string]any{
			"schema":        "coze.subagent_retry.metadata.v1",
			"source_status": string(sourceRun.Status),
			"parent_run_id": parentRun.ID,
		},
	}, "subagent retry metadata")
}

func subagentRetryRequestedPayload(
	parentRun *domainentity.Run,
	sourceRun *domainentity.Run,
	retryRunID int64,
	requestedAt int64,
) map[string]any {
	return map[string]any{
		"schema":        "coze.subagent_retry_requested.v1",
		"thread_id":     sourceRun.ThreadID,
		"source_run_id": sourceRun.ID,
		"parent_run_id": parentRun.ID,
		"retry_run_id":  retryRunID,
		"source_status": string(sourceRun.Status),
		"requested_at":  requestedAt,
	}
}
