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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"gorm.io/gorm"

	domainentity "github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	domainrepo "github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
	"github.com/coze-dev/coze-studio/backend/infra/storage"
)

var SVC = new(ApplicationService)

var ErrArtifactScanReviewDecisionInvalid = errors.New(
	"artifact scan review decision is invalid",
)

var ErrArtifactSignedURLNotSupported = errors.New(
	"artifact signed url is not supported",
)

type ApplicationService struct {
	ThreadSVC                 domainservice.ThreadService
	ThreadAuthorizer          ThreadAuthorizer
	WorkspaceAuthorizer       WorkspaceAuthorizer
	RuntimeFileSVC            domainservice.RuntimeFileService
	UploadFileSVC             domainservice.UploadFileService
	PlanSVC                   domainservice.PlanService
	ArtifactSVC               domainservice.ArtifactService
	ADKCancelRegistry         *ADKCancelRegistry
	RuntimePolicy             *RuntimePolicy
	ArtifactObjectStorage     ArtifactObjectStorage
	ArtifactAuthorizer        ArtifactAuthorizer
	MemoryAuthorizer          MemoryAuthorizer
	GuardrailAuditRepository  domainrepo.GuardrailAuditRepository
	GuardrailAuditAuthorizer  GuardrailAuditAuthorizer
	MCPRuntimeAuditRepository domainrepo.MCPRuntimeAuditRepository
	MCPRuntimeAuditAuthorizer MCPRuntimeAuditAuthorizer
	GuardrailProviderStatus   GuardrailProviderEnvStatus
	ArtifactScanner           ArtifactContentScanner
	ArtifactScannerStatus     ArtifactScannerEnvStatus
	ArtifactScanReadPolicy    ArtifactScanReadPolicyConfig
	ArtifactReviewClock       func() int64
	ArtifactCleanupNowFunc    func() int64
	MemoryExtractor           MemoryExtractor
}

type ArtifactObjectStorage interface {
	GetObject(ctx context.Context, objectKey string) ([]byte, error)
}

type ArtifactObjectURLSigner interface {
	GetObjectUrl(ctx context.Context, objectKey string, opts ...storage.GetOptFn) (string, error)
}

type ArtifactObjectDeleter interface {
	DeleteObject(ctx context.Context, objectKey string) error
}

type ArtifactContentScanner interface {
	ScanArtifact(ctx context.Context, req ArtifactScanRequest) (*ArtifactScanResult, error)
}

type ArtifactScanRequest struct {
	SpaceID     int64
	ThreadID    int64
	RunID       int64
	UserID      int64
	ArtifactID  int64
	FileID      int64
	Scanner     string
	ContentType string
	SizeBytes   int64
	Content     []byte
}

type ArtifactScanResult struct {
	ScanStatus     string
	ScannerVersion string
	Reason         string
}

type artifactScanStatus string

const (
	artifactScanStatusUnknown     artifactScanStatus = "unknown"
	artifactScanStatusClean       artifactScanStatus = "clean"
	artifactScanStatusPending     artifactScanStatus = "pending"
	artifactScanStatusFailed      artifactScanStatus = "failed"
	artifactScanStatusBlocked     artifactScanStatus = "blocked"
	artifactScanStatusInfected    artifactScanStatus = "infected"
	artifactScanStatusQuarantined artifactScanStatus = "quarantined"
)

const artifactContentAccessedEvent = "artifact.content.accessed"
const artifactContentBlockedEvent = "artifact.content.blocked"
const artifactDeletedEvent = "artifact.deleted"
const artifactCleanedEvent = "artifact.cleaned"
const artifactRestoredEvent = "artifact.restored"
const artifactScanCompletedEvent = "artifact.scan.completed"
const artifactScanReviewedEvent = "artifact.scan.reviewed"
const artifactScanJobRetryRequestedEvent = "artifact.scan_job.retry_requested"
const defaultApplicationArtifactScanner = "default"
const manualArtifactReviewScanner = "manual_review"
const defaultArtifactScanRetryBackoffMillis = int64(60000)

func (s *ApplicationService) CreateThread(ctx context.Context, req *CreateThreadRequest) (*CreateThreadResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("create thread request is required")
	}
	userID, err := s.resolveThreadCreator(ctx, req.SpaceID, req.UserID)
	if err != nil {
		return nil, err
	}

	thread, err := s.ThreadSVC.CreateThread(ctx, &domainservice.CreateThreadRequest{
		SpaceID:      req.SpaceID,
		UserID:       userID,
		AgentID:      req.AgentID,
		Title:        req.Title,
		Source:       domainentity.ThreadSource(req.Source),
		LegacyTaskID: req.LegacyTaskID,
		Metadata:     req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("agent thread service returned empty thread")
	}

	return &CreateThreadResponse{Thread: DomainThreadToSummary(thread)}, nil
}

func (s *ApplicationService) resolveThreadCreator(
	ctx context.Context,
	spaceID int64,
	requestedUserID int64,
) (int64, error) {
	viewerID, public, err := s.publicThreadViewerID(ctx)
	if err != nil {
		return 0, err
	}
	if !public {
		return requestedUserID, nil
	}
	if spaceID <= 0 {
		return 0, ErrThreadAccessDenied
	}
	if err := s.AuthorizeWorkspaceAccess(ctx, WorkspaceAccessRequest{
		ViewerID: viewerID,
		SpaceID:  spaceID,
	}); err != nil {
		return 0, err
	}
	return viewerID, nil
}

func (s *ApplicationService) CreateTaskThread(ctx context.Context, req *CreateTaskThreadRequest) (*CreateTaskThreadResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("create task thread request is required")
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, fmt.Errorf("task thread message is required")
	}
	if err := s.validateRunRuntimeConfig(req.Config); err != nil {
		return nil, err
	}

	title := taskThreadTitle(req.Title, message)
	if req.DeferStart {
		threadResp, err := s.CreateThread(ctx, &CreateThreadRequest{
			SpaceID:  req.SpaceID,
			UserID:   req.UserID,
			Title:    title,
			Source:   ThreadSourceWeb,
			Metadata: `{"source":"workbench_new_task"}`,
		})
		if err != nil {
			return nil, err
		}
		if threadResp == nil || threadResp.Thread == nil {
			return nil, fmt.Errorf("agent thread service returned empty thread")
		}
		return &CreateTaskThreadResponse{
			Thread: threadResp.Thread,
		}, nil
	}

	input, err := taskThreadRunInputFromMessage(message)
	if err != nil {
		return nil, err
	}
	userID, err := s.resolveThreadCreator(ctx, req.SpaceID, req.UserID)
	if err != nil {
		return nil, err
	}
	bundle, err := s.ThreadSVC.CreateThreadRunMessage(ctx, &domainservice.CreateThreadRunMessageRequest{
		Thread: domainservice.CreateThreadRequest{
			SpaceID: req.SpaceID, UserID: userID, Title: title,
			Source: domainentity.ThreadSourceWeb, Metadata: `{"source":"workbench_new_task"}`,
		},
		Run: domainservice.CreateRunRequest{
			AssistantID: req.AssistantID, RunKind: domainentity.RunKindTask,
			Command: req.Command, Input: input, Config: req.Config,
			Context: req.Context, Metadata: req.Metadata, StreamMode: req.StreamMode,
			MultitaskStrategy: req.MultitaskStrategy, OnDisconnect: req.OnDisconnect,
			Durability: req.Durability, IdempotencyKey: req.IdempotencyKey,
		},
		Message: domainservice.CreateMessageSpec{
			Role: domainentity.MessageRoleUser, Content: message,
			Metadata: `{"source":"workbench_new_task"}`,
		},
	})
	if err != nil {
		return nil, err
	}
	if bundle == nil || bundle.Thread == nil || bundle.Run == nil || bundle.Message == nil {
		return nil, fmt.Errorf("agent thread service returned incomplete task thread bundle")
	}

	return &CreateTaskThreadResponse{
		Thread:  DomainThreadToSummary(bundle.Thread),
		Message: DomainMessageToSummary(bundle.Message),
		Run:     DomainRunToSummary(bundle.Run),
	}, nil
}

func taskThreadTitle(title, message string) string {
	const maxTitleRunes = 80
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		trimmed = strings.TrimSpace(message)
	}
	runes := []rune(trimmed)
	if len(runes) <= maxTitleRunes {
		return trimmed
	}

	return string(runes[:maxTitleRunes])
}

func taskThreadRunInputFromMessage(message string) (string, error) {
	payload := struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{
				Role:    string(MessageRoleUser),
				Content: message,
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	return string(raw), nil
}

func (s *ApplicationService) GetThread(ctx context.Context, req *GetThreadRequest) (*GetThreadResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get thread request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}

	thread, err := s.ThreadSVC.GetThread(ctx, req.ThreadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("agent thread service returned empty thread")
	}

	return &GetThreadResponse{Thread: DomainThreadToSummary(thread)}, nil
}

func (s *ApplicationService) UpdateThreadTitle(
	ctx context.Context,
	req *UpdateThreadTitleRequest,
) (*UpdateThreadTitleResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("update thread title request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}

	thread, updated, err := s.ThreadSVC.UpdateThreadTitle(ctx, &domainservice.UpdateThreadTitleRequest{
		ThreadID: req.ThreadID,
		Title:    req.Title,
	})
	if err != nil {
		return nil, err
	}
	if updated && thread == nil {
		return nil, fmt.Errorf("agent thread service returned empty updated thread")
	}

	return &UpdateThreadTitleResponse{
		Thread:  DomainThreadToSummary(thread),
		Updated: updated,
	}, nil
}

func (s *ApplicationService) UpdateThreadMetadata(
	ctx context.Context,
	req *UpdateThreadMetadataRequest,
) (*UpdateThreadMetadataResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("update thread metadata request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}

	thread, updated, err := s.ThreadSVC.UpdateThreadMetadata(ctx, &domainservice.UpdateThreadMetadataRequest{
		ThreadID: req.ThreadID,
		Metadata: req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if updated && thread == nil {
		return nil, fmt.Errorf("agent thread service returned empty updated thread")
	}

	return &UpdateThreadMetadataResponse{
		Thread:  DomainThreadToSummary(thread),
		Updated: updated,
	}, nil
}

func (s *ApplicationService) DeleteThread(
	ctx context.Context,
	req *DeleteThreadRequest,
) (*DeleteThreadResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("delete thread request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}

	deleted, err := s.ThreadSVC.DeleteThread(ctx, &domainservice.DeleteThreadRequest{
		ThreadID: req.ThreadID,
	})
	if err != nil {
		return nil, err
	}

	return &DeleteThreadResponse{Deleted: deleted}, nil
}

func (s *ApplicationService) ListThreads(ctx context.Context, req *ListThreadsRequest) (*ListThreadsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list threads request is required")
	}
	userID := req.UserID
	if viewerID, public, err := s.publicThreadViewerID(ctx); err != nil {
		return nil, err
	} else if public {
		if req.SpaceID <= 0 {
			return nil, ErrThreadAccessDenied
		}
		if err := s.AuthorizeWorkspaceAccess(ctx, WorkspaceAccessRequest{
			ViewerID: viewerID,
			SpaceID:  req.SpaceID,
		}); err != nil {
			return nil, err
		}
		userID = viewerID
	}

	var status *domainentity.ThreadStatus
	if req.Status != nil {
		mapped := domainentity.ThreadStatus(*req.Status)
		status = &mapped
	}

	threads, total, err := s.ThreadSVC.ListThreads(ctx, &domainservice.ListThreadsRequest{
		SpaceID:  req.SpaceID,
		UserID:   userID,
		Status:   status,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListThreadsResponse{
		Threads: make([]*ThreadSummary, 0, len(threads)),
		Total:   total,
	}
	for _, thread := range threads {
		resp.Threads = append(resp.Threads, DomainThreadToSummary(thread))
	}

	return resp, nil
}

func (s *ApplicationService) AppendMessage(ctx context.Context, req *AppendMessageRequest) (*AppendMessageResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("append message request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}); err != nil {
		return nil, err
	}

	message, err := s.ThreadSVC.AppendMessage(ctx, &domainservice.AppendMessageRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Role:     domainentity.MessageRole(req.Role),
		Content:  req.Content,
		Metadata: req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if message == nil {
		return nil, fmt.Errorf("agent thread service returned empty message")
	}

	return &AppendMessageResponse{Message: DomainMessageToSummary(message)}, nil
}

func (s *ApplicationService) ListMessages(ctx context.Context, req *ListMessagesRequest) (*ListMessagesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list messages request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}

	messages, total, err := s.ThreadSVC.ListMessages(ctx, &domainservice.ListMessagesRequest{
		ThreadID: req.ThreadID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListMessagesResponse{
		Messages: make([]*MessageSummary, 0, len(messages)),
		Total:    total,
	}
	for _, message := range messages {
		resp.Messages = append(resp.Messages, DomainMessageToSummary(message))
	}

	return resp, nil
}

func (s *ApplicationService) CreateRun(ctx context.Context, req *CreateRunRequest) (*CreateRunResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("create run request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}
	if err := s.validateRunRuntimeConfig(req.Config); err != nil {
		return nil, err
	}
	messageContent := strings.TrimSpace(req.MessageContent)
	if messageContent == "" && strings.TrimSpace(req.MessageMetadata) != "" {
		return nil, fmt.Errorf("run message content is required when message metadata is set")
	}
	if messageContent != "" {
		authoritativeInput, err := s.buildAuthoritativeRunInput(
			ctx,
			req.ThreadID,
			messageContent,
			req.Input,
		)
		if err != nil {
			return nil, err
		}
		bundle, err := s.ThreadSVC.CreateRunBundle(ctx, &domainservice.CreateRunBundleRequest{
			Run: domainservice.CreateRunRequest{
				ThreadID: req.ThreadID, ParentRunID: req.ParentRunID,
				AssistantID: req.AssistantID, RunKind: domainentity.RunKind(req.RunKind),
				Status: domainentity.RunStatus(req.Status), Command: req.Command,
				Input: authoritativeInput, Config: req.Config, Context: req.Context,
				Metadata: req.Metadata, StreamMode: req.StreamMode,
				MultitaskStrategy: req.MultitaskStrategy, OnDisconnect: req.OnDisconnect,
				Durability: req.Durability, IdempotencyKey: req.IdempotencyKey,
			},
			Message: &domainservice.CreateMessageSpec{
				Role: domainentity.MessageRoleUser, Content: messageContent,
				Metadata: req.MessageMetadata,
			},
		})
		if err != nil {
			return nil, err
		}
		if bundle == nil || bundle.Run == nil || bundle.Message == nil {
			return nil, fmt.Errorf("agent thread service returned incomplete run bundle")
		}
		return &CreateRunResponse{
			Run: DomainRunToSummary(bundle.Run), Message: DomainMessageToSummary(bundle.Message),
		}, nil
	}

	run, err := s.ThreadSVC.CreateRun(ctx, &domainservice.CreateRunRequest{
		ThreadID:          req.ThreadID,
		ParentRunID:       req.ParentRunID,
		AssistantID:       req.AssistantID,
		RunKind:           domainentity.RunKind(req.RunKind),
		Status:            domainentity.RunStatus(req.Status),
		Command:           req.Command,
		Input:             req.Input,
		Config:            req.Config,
		Context:           req.Context,
		Metadata:          req.Metadata,
		StreamMode:        req.StreamMode,
		MultitaskStrategy: req.MultitaskStrategy,
		OnDisconnect:      req.OnDisconnect,
		Durability:        req.Durability,
		IdempotencyKey:    req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty run")
	}

	return &CreateRunResponse{Run: DomainRunToSummary(run)}, nil
}

const authoritativeRunHistoryPageSize int32 = 200
const authoritativeRunPageSize int32 = 200

type authoritativeRunInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type authoritativeRunInput struct {
	Messages      []authoritativeRunInputMessage   `json:"messages"`
	UploadedFiles []*TaskThreadUploadedFileSummary `json:"uploaded_files,omitempty"`
}

func (s *ApplicationService) buildAuthoritativeRunInput(
	ctx context.Context,
	threadID int64,
	currentMessage string,
	rawInput string,
) (string, error) {
	var submitted struct {
		UploadedFiles []*TaskThreadUploadedFileSummary `json:"uploaded_files"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(rawInput)), &submitted); err != nil {
		return "", fmt.Errorf("parse run input failed: %w", err)
	}

	historyMessages := make([]*domainentity.Message, 0)
	hasLegacyUnboundMessages := false
	page := int32(1)
	for {
		rows, total, err := s.ThreadSVC.ListMessages(ctx, &domainservice.ListMessagesRequest{
			ThreadID: threadID,
			Page:     page,
			PageSize: authoritativeRunHistoryPageSize,
		})
		if err != nil {
			return "", err
		}
		for _, message := range rows {
			if message != nil && message.ID > 0 {
				historyMessages = append(historyMessages, message)
				if message.RunID <= 0 {
					hasLegacyUnboundMessages = true
				}
			}
		}
		if len(rows) == 0 || int64(page)*int64(authoritativeRunHistoryPageSize) >= total {
			break
		}
		page++
	}

	legacyCommittedMessageIDs := map[int64]struct{}{}
	if hasLegacyUnboundMessages {
		var err error
		legacyCommittedMessageIDs, err = s.listLegacyCommittedMessageIDs(ctx, threadID)
		if err != nil {
			return "", err
		}
	}

	messages := make([]authoritativeRunInputMessage, 0, len(historyMessages)+1)
	seen := make(map[int64]struct{}, len(historyMessages))
	for _, message := range historyMessages {
		if _, ok := seen[message.ID]; ok {
			continue
		}
		seen[message.ID] = struct{}{}
		if message.RunID <= 0 {
			if message.Role != domainentity.MessageRoleUser {
				continue
			}
			if _, ok := legacyCommittedMessageIDs[message.ID]; !ok {
				continue
			}
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		switch message.Role {
		case domainentity.MessageRoleUser:
			messages = append(messages, authoritativeRunInputMessage{Role: "user", Content: content})
		case domainentity.MessageRoleAssistant:
			messages = append(messages, authoritativeRunInputMessage{Role: "assistant", Content: content})
		}
	}
	messages = append(messages, authoritativeRunInputMessage{
		Role: "user", Content: strings.TrimSpace(currentMessage),
	})

	encoded, err := json.Marshal(authoritativeRunInput{
		Messages:      messages,
		UploadedFiles: normalizeADKUploadedFiles(submitted.UploadedFiles),
	})
	if err != nil {
		return "", fmt.Errorf("marshal authoritative run input: %w", err)
	}
	return string(encoded), nil
}

func (s *ApplicationService) listLegacyCommittedMessageIDs(
	ctx context.Context,
	threadID int64,
) (map[int64]struct{}, error) {
	messageIDs := make(map[int64]struct{})
	page := int32(1)
	for {
		runs, total, err := s.ThreadSVC.ListRuns(ctx, &domainservice.ListRunsRequest{
			ThreadID: threadID,
			Page:     page,
			PageSize: authoritativeRunPageSize,
		})
		if err != nil {
			return nil, err
		}
		for _, run := range runs {
			if run == nil || run.ThreadID != threadID {
				continue
			}
			if messageID := legacyAppendedMessageID(run.Metadata); messageID > 0 {
				messageIDs[messageID] = struct{}{}
			}
		}
		if len(runs) == 0 || int64(page)*int64(authoritativeRunPageSize) >= total {
			break
		}
		page++
	}
	return messageIDs, nil
}

func legacyAppendedMessageID(metadata string) int64 {
	var value struct {
		AppendedMessageID json.RawMessage `json:"appended_message_id"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(metadata)), &value) != nil || len(value.AppendedMessageID) == 0 {
		return 0
	}

	var stringID string
	if json.Unmarshal(value.AppendedMessageID, &stringID) == nil {
		id, err := strconv.ParseInt(strings.TrimSpace(stringID), 10, 64)
		if err == nil && id > 0 {
			return id
		}
		return 0
	}

	var numericID int64
	if json.Unmarshal(value.AppendedMessageID, &numericID) == nil && numericID > 0 {
		return numericID
	}
	return 0
}

func (s *ApplicationService) validateRunRuntimeConfig(config string) error {
	if s.RuntimePolicy == nil {
		return nil
	}
	_, err := s.RuntimePolicy.runtimeModeFromRun(&RunSummary{
		Config: config,
	})
	return err
}

func (s *ApplicationService) GetRun(ctx context.Context, req *GetRunRequest) (*GetRunResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get run request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		RunID: req.RunID,
	}); err != nil {
		return nil, err
	}

	run, err := s.ThreadSVC.GetRun(ctx, &domainservice.GetRunRequest{RunID: req.RunID})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty run")
	}

	return &GetRunResponse{Run: DomainRunToSummary(run)}, nil
}

func (s *ApplicationService) ListRuns(ctx context.Context, req *ListRunsRequest) (*ListRunsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list runs request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}

	var status *domainentity.RunStatus
	if req.Status != nil {
		mapped := domainentity.RunStatus(*req.Status)
		status = &mapped
	}

	runs, total, err := s.ThreadSVC.ListRuns(ctx, &domainservice.ListRunsRequest{
		ThreadID:         req.ThreadID,
		ParentRunID:      req.ParentRunID,
		IncludeChildRuns: req.IncludeChildRuns,
		Status:           status,
		Page:             req.Page,
		PageSize:         req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListRunsResponse{
		Runs:  make([]*RunSummary, 0, len(runs)),
		Total: total,
	}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, DomainRunToSummary(run))
	}

	return resp, nil
}

func (s *ApplicationService) AppendRunEvent(ctx context.Context, req *AppendRunEventRequest) (*AppendRunEventResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("append run event request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}); err != nil {
		return nil, err
	}

	event, err := s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  req.ThreadID,
		RunID:     req.RunID,
		EventType: req.EventType,
		Payload:   req.Payload,
	})
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, fmt.Errorf("agent thread service returned empty run event")
	}

	return &AppendRunEventResponse{Event: DomainRunEventToSummary(event)}, nil
}

func (s *ApplicationService) ListRunEvents(ctx context.Context, req *ListRunEventsRequest) (*ListRunEventsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list run events request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}); err != nil {
		return nil, err
	}

	events, total, err := s.ThreadSVC.ListRunEvents(ctx, &domainservice.ListRunEventsRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListRunEventsResponse{
		Events: make([]*RunEventSummary, 0, len(events)),
		Total:  total,
	}
	for _, event := range events {
		resp.Events = append(resp.Events, DomainRunEventToSummary(event))
	}

	return resp, nil
}

func (s *ApplicationService) CreateCheckpoint(ctx context.Context, req *CreateCheckpointRequest) (*CreateCheckpointResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("create checkpoint request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}); err != nil {
		return nil, err
	}

	checkpoint, err := s.ThreadSVC.CreateCheckpoint(ctx, &domainservice.CreateCheckpointRequest{
		ThreadID:           req.ThreadID,
		RunID:              req.RunID,
		ParentCheckpointID: req.ParentCheckpointID,
		CheckpointNS:       req.CheckpointNS,
		RuntimeType:        req.RuntimeType,
		RuntimeKey:         req.RuntimeKey,
		EnvelopeVersion:    req.EnvelopeVersion,
		ChannelValues:      req.ChannelValues,
		ChannelVersions:    req.ChannelVersions,
		PendingSends:       req.PendingSends,
		Metadata:           req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if checkpoint == nil {
		return nil, fmt.Errorf("agent thread service returned empty checkpoint")
	}

	return &CreateCheckpointResponse{Checkpoint: DomainCheckpointToSummary(checkpoint)}, nil
}

func (s *ApplicationService) ListCheckpoints(ctx context.Context, req *ListCheckpointsRequest) (*ListCheckpointsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list checkpoints request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}); err != nil {
		return nil, err
	}

	checkpoints, total, err := s.ThreadSVC.ListCheckpoints(ctx, &domainservice.ListCheckpointsRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListCheckpointsResponse{
		Checkpoints: make([]*CheckpointSummary, 0, len(checkpoints)),
		Total:       total,
	}
	for _, checkpoint := range checkpoints {
		resp.Checkpoints = append(resp.Checkpoints, DomainCheckpointToSummary(checkpoint))
	}

	return resp, nil
}

func (s *ApplicationService) GetCheckpoint(ctx context.Context, req *GetCheckpointRequest) (*GetCheckpointResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get checkpoint request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{}); err != nil {
		return nil, err
	}

	checkpoint, err := s.ThreadSVC.GetCheckpoint(ctx, &domainservice.GetCheckpointRequest{
		CheckpointID: req.CheckpointID,
	})
	if err != nil {
		if isPublicThreadAccessContext(ctx) && errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrThreadAccessDenied
		}
		return nil, err
	}
	if checkpoint == nil {
		if isPublicThreadAccessContext(ctx) {
			return nil, ErrThreadAccessDenied
		}
		return nil, fmt.Errorf("agent thread service returned empty checkpoint")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: checkpoint.ThreadID,
		RunID:    checkpoint.RunID,
	}); err != nil {
		return nil, err
	}

	return &GetCheckpointResponse{Checkpoint: DomainCheckpointToSummary(checkpoint)}, nil
}

func (s *ApplicationService) GetLatestCheckpoint(ctx context.Context, req *GetLatestCheckpointRequest) (*GetLatestCheckpointResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get latest checkpoint request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}

	checkpoint, err := s.ThreadSVC.GetLatestCheckpoint(ctx, &domainservice.GetLatestCheckpointRequest{
		ThreadID: req.ThreadID,
	})
	if err != nil {
		return nil, err
	}
	if checkpoint == nil {
		return nil, fmt.Errorf("agent thread service returned empty checkpoint")
	}

	return &GetLatestCheckpointResponse{Checkpoint: DomainCheckpointToSummary(checkpoint)}, nil
}

func (s *ApplicationService) GetLatestRuntimeCheckpoint(
	ctx context.Context,
	req *GetLatestRuntimeCheckpointRequest,
) (*GetLatestRuntimeCheckpointResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get latest runtime checkpoint request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}); err != nil {
		return nil, err
	}

	checkpoint, err := s.ThreadSVC.GetLatestRuntimeCheckpoint(
		ctx,
		&domainservice.GetLatestRuntimeCheckpointRequest{
			ThreadID:    req.ThreadID,
			RunID:       req.RunID,
			RuntimeType: req.RuntimeType,
			RuntimeKey:  req.RuntimeKey,
		},
	)
	if err != nil {
		return nil, err
	}

	return &GetLatestRuntimeCheckpointResponse{
		Checkpoint: DomainCheckpointToSummary(checkpoint),
	}, nil
}

func (s *ApplicationService) DeleteRuntimeCheckpoint(
	ctx context.Context,
	req *DeleteRuntimeCheckpointRequest,
) error {
	if err := s.requireThreadSVC(); err != nil {
		return err
	}
	if req == nil {
		return fmt.Errorf("delete runtime checkpoint request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}); err != nil {
		return err
	}

	return s.ThreadSVC.DeleteRuntimeCheckpoint(ctx, &domainservice.DeleteRuntimeCheckpointRequest{
		ThreadID:    req.ThreadID,
		RunID:       req.RunID,
		RuntimeType: req.RuntimeType,
		RuntimeKey:  req.RuntimeKey,
		DeletedAt:   req.DeletedAt,
	})
}

func (s *ApplicationService) RememberMemory(ctx context.Context, req *RememberMemoryRequest) (*RememberMemoryResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("remember memory request is required")
	}

	memory, err := s.ThreadSVC.RememberMemory(ctx, &domainservice.RememberMemoryRequest{
		ThreadID:             req.ThreadID,
		RunID:                req.RunID,
		Scope:                domainentity.MemoryScope(req.Scope),
		Content:              req.Content,
		Metadata:             req.Metadata,
		Score:                req.Score,
		Confidence:           req.Confidence,
		SourceType:           req.SourceType,
		SourceID:             req.SourceID,
		CorrectionOfMemoryID: req.CorrectionOfMemoryID,
		CorrectedAt:          req.CorrectedAt,
		ExpiresAt:            req.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	if memory == nil {
		return nil, fmt.Errorf("agent thread service returned empty memory")
	}

	return &RememberMemoryResponse{Memory: DomainMemoryToSummary(memory)}, nil
}

func (s *ApplicationService) RecallMemories(ctx context.Context, req *RecallMemoriesRequest) (*RecallMemoriesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("recall memories request is required")
	}

	scopes := make([]domainentity.MemoryScope, 0, len(req.Scopes))
	for _, scope := range req.Scopes {
		if scope == "" {
			continue
		}
		scopes = append(scopes, domainentity.MemoryScope(scope))
	}

	memories, total, err := s.ThreadSVC.RecallMemories(ctx, &domainservice.RecallMemoriesRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Scopes:   scopes,
		Query:    req.Query,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, err
	}

	resp := &RecallMemoriesResponse{
		Memories: make([]*MemorySummary, 0, len(memories)),
		Total:    total,
	}
	for _, memory := range memories {
		resp.Memories = append(resp.Memories, DomainMemoryToSummary(memory))
	}

	return resp, nil
}

func (s *ApplicationService) ListMemories(ctx context.Context, req *ListMemoriesRequest) (*ListMemoriesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list memories request is required")
	}
	if err := s.authorizeMemoryAccess(ctx, MemoryAccessRequest{
		ThreadID:  req.ThreadID,
		ViewerID:  req.ViewerID,
		Operation: MemoryAccessOperationList,
	}); err != nil {
		return nil, err
	}

	scopes := make([]domainentity.MemoryScope, 0, len(req.Scopes))
	for _, scope := range req.Scopes {
		if scope == "" {
			continue
		}
		scopes = append(scopes, domainentity.MemoryScope(scope))
	}
	memories, total, err := s.ThreadSVC.ListMemories(ctx, &domainservice.ListMemoriesRequest{
		ThreadID:       req.ThreadID,
		RunID:          req.RunID,
		Scopes:         scopes,
		Query:          req.Query,
		IncludeExpired: req.IncludeExpired,
		IncludeDeleted: req.IncludeDeleted,
		Page:           req.Page,
		PageSize:       req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	resp := &ListMemoriesResponse{
		Memories: make([]*MemorySummary, 0, len(memories)),
		Total:    total,
	}
	for _, memory := range memories {
		resp.Memories = append(resp.Memories, DomainMemoryToSummary(memory))
	}
	return resp, nil
}

func (s *ApplicationService) ExportMemories(ctx context.Context, req *ExportMemoriesRequest) (*ExportMemoriesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("export memories request is required")
	}
	if err := s.authorizeMemoryAccess(ctx, MemoryAccessRequest{
		ThreadID:  req.ThreadID,
		ViewerID:  req.ViewerID,
		Operation: MemoryAccessOperationExport,
	}); err != nil {
		return nil, err
	}

	scopes := make([]domainentity.MemoryScope, 0, len(req.Scopes))
	for _, scope := range req.Scopes {
		if scope == "" {
			continue
		}
		scopes = append(scopes, domainentity.MemoryScope(scope))
	}
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	memories, total, err := s.ThreadSVC.ListMemories(ctx, &domainservice.ListMemoriesRequest{
		ThreadID:       req.ThreadID,
		RunID:          req.RunID,
		Scopes:         scopes,
		Query:          req.Query,
		IncludeExpired: req.IncludeExpired,
		IncludeDeleted: req.IncludeDeleted,
		Page:           1,
		PageSize:       limit,
	})
	if err != nil {
		return nil, err
	}

	resp := &ExportMemoriesResponse{
		Schema:     MemoryExportSchema,
		ThreadID:   req.ThreadID,
		ExportedAt: time.Now().UnixMilli(),
		Total:      total,
		Memories:   make([]*MemorySummary, 0, len(memories)),
	}
	for _, memory := range memories {
		resp.Memories = append(resp.Memories, DomainMemoryToSummary(memory))
	}
	return resp, nil
}

func (s *ApplicationService) ImportMemories(ctx context.Context, req *ImportMemoriesRequest) (*ImportMemoriesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("import memories request is required")
	}
	if err := s.authorizeMemoryAccess(ctx, MemoryAccessRequest{
		ThreadID:  req.ThreadID,
		ViewerID:  req.ViewerID,
		Operation: MemoryAccessOperationImport,
	}); err != nil {
		return nil, err
	}

	items := make([]domainservice.ImportMemoryItem, 0, len(req.Memories))
	for _, item := range req.Memories {
		items = append(items, domainservice.ImportMemoryItem{
			RunID:                item.RunID,
			Scope:                domainentity.MemoryScope(item.Scope),
			Content:              item.Content,
			Metadata:             item.Metadata,
			Score:                item.Score,
			Confidence:           item.Confidence,
			SourceType:           item.SourceType,
			SourceID:             item.SourceID,
			CorrectionOfMemoryID: item.CorrectionOfMemoryID,
			CorrectedAt:          item.CorrectedAt,
			ExpiresAt:            item.ExpiresAt,
		})
	}
	result, err := s.ThreadSVC.ImportMemories(ctx, &domainservice.ImportMemoriesRequest{
		ThreadID: req.ThreadID,
		ActorID:  req.ActorID,
		Memories: items,
	})
	if err != nil {
		return nil, err
	}

	resp := &ImportMemoriesResponse{
		Imported: result.Imported,
		Skipped:  result.Skipped,
		Memories: make([]*MemorySummary, 0, len(result.Memories)),
	}
	for _, memory := range result.Memories {
		resp.Memories = append(resp.Memories, DomainMemoryToSummary(memory))
	}
	return resp, nil
}

func (s *ApplicationService) UpdateMemory(ctx context.Context, req *UpdateMemoryRequest) (*UpdateMemoryResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("update memory request is required")
	}
	if err := s.authorizeMemoryAccess(ctx, MemoryAccessRequest{
		ThreadID:  req.ThreadID,
		MemoryID:  req.MemoryID,
		ViewerID:  req.ViewerID,
		Operation: MemoryAccessOperationUpdate,
	}); err != nil {
		return nil, err
	}

	memory, updated, err := s.ThreadSVC.UpdateMemory(ctx, &domainservice.UpdateMemoryRequest{
		ThreadID:             req.ThreadID,
		MemoryID:             req.MemoryID,
		ActorID:              req.ActorID,
		RunID:                req.RunID,
		Scope:                domainentity.MemoryScope(req.Scope),
		Content:              req.Content,
		Metadata:             req.Metadata,
		Score:                req.Score,
		Confidence:           req.Confidence,
		SourceType:           req.SourceType,
		SourceID:             req.SourceID,
		CorrectionOfMemoryID: req.CorrectionOfMemoryID,
		CorrectedAt:          req.CorrectedAt,
		ExpiresAt:            req.ExpiresAt,
	})
	if err != nil {
		return nil, err
	}
	return &UpdateMemoryResponse{
		Memory:  DomainMemoryToSummary(memory),
		Updated: updated,
	}, nil
}

func (s *ApplicationService) DeleteMemory(ctx context.Context, req *DeleteMemoryRequest) (*DeleteMemoryResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("delete memory request is required")
	}
	if err := s.authorizeMemoryAccess(ctx, MemoryAccessRequest{
		ThreadID:  req.ThreadID,
		MemoryID:  req.MemoryID,
		ViewerID:  req.ViewerID,
		Operation: MemoryAccessOperationDelete,
	}); err != nil {
		return nil, err
	}
	deleted, err := s.ThreadSVC.DeleteMemory(ctx, &domainservice.DeleteMemoryRequest{
		ThreadID: req.ThreadID,
		MemoryID: req.MemoryID,
		ActorID:  req.ActorID,
	})
	if err != nil {
		return nil, err
	}
	return &DeleteMemoryResponse{Deleted: deleted}, nil
}

func (s *ApplicationService) ClearMemories(ctx context.Context, req *ClearMemoriesRequest) (*ClearMemoriesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("clear memories request is required")
	}
	if err := s.authorizeMemoryAccess(ctx, MemoryAccessRequest{
		ThreadID:  req.ThreadID,
		ViewerID:  req.ViewerID,
		Operation: MemoryAccessOperationClear,
	}); err != nil {
		return nil, err
	}
	scopes := make([]domainentity.MemoryScope, 0, len(req.Scopes))
	for _, scope := range req.Scopes {
		if scope == "" {
			continue
		}
		scopes = append(scopes, domainentity.MemoryScope(scope))
	}
	deleted, err := s.ThreadSVC.ClearMemories(ctx, &domainservice.ClearMemoriesRequest{
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
		Scopes:   scopes,
		ActorID:  req.ActorID,
	})
	if err != nil {
		return nil, err
	}
	return &ClearMemoriesResponse{Deleted: deleted}, nil
}

func (s *ApplicationService) RestoreMemory(ctx context.Context, req *RestoreMemoryRequest) (*RestoreMemoryResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("restore memory request is required")
	}
	if err := s.authorizeMemoryAccess(ctx, MemoryAccessRequest{
		ThreadID:  req.ThreadID,
		MemoryID:  req.MemoryID,
		ViewerID:  req.ViewerID,
		Operation: MemoryAccessOperationRestore,
	}); err != nil {
		return nil, err
	}
	memory, restored, err := s.ThreadSVC.RestoreMemory(ctx, &domainservice.RestoreMemoryRequest{
		ThreadID: req.ThreadID,
		MemoryID: req.MemoryID,
		ActorID:  req.ActorID,
	})
	if err != nil {
		return nil, err
	}
	return &RestoreMemoryResponse{
		Memory:   DomainMemoryToSummary(memory),
		Restored: restored,
	}, nil
}

func (s *ApplicationService) ListMemoryAuditEvents(
	ctx context.Context,
	req *ListMemoryAuditEventsRequest,
) (*ListMemoryAuditEventsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list memory audit events request is required")
	}
	if err := s.authorizeMemoryAccess(ctx, MemoryAccessRequest{
		ThreadID:  req.ThreadID,
		MemoryID:  req.MemoryID,
		ViewerID:  req.ViewerID,
		Operation: MemoryAccessOperationAudit,
	}); err != nil {
		return nil, err
	}
	events, total, err := s.ThreadSVC.ListMemoryAuditEvents(ctx, &domainservice.ListMemoryAuditEventsRequest{
		ThreadID: req.ThreadID,
		MemoryID: req.MemoryID,
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}
	resp := &ListMemoryAuditEventsResponse{
		Events: make([]*MemoryAuditEventSummary, 0, len(events)),
		Total:  total,
	}
	for _, event := range events {
		resp.Events = append(resp.Events, DomainMemoryAuditEventToSummary(event))
	}
	return resp, nil
}

func (s *ApplicationService) ListGuardrailAuditEvents(
	ctx context.Context,
	req *ListGuardrailAuditEventsRequest,
) (*ListGuardrailAuditEventsResponse, error) {
	if err := s.requireGuardrailAuditRepository(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list guardrail audit events request is required")
	}
	if req.ThreadID <= 0 {
		return nil, fmt.Errorf("thread id is required")
	}
	if req.RunID < 0 {
		return nil, fmt.Errorf("run id is invalid")
	}
	if err := s.authorizeGuardrailAuditAccess(ctx, GuardrailAuditAccessRequest{
		ThreadID:  req.ThreadID,
		RunID:     req.RunID,
		ViewerID:  req.ViewerID,
		Operation: GuardrailAuditAccessOperationList,
	}); err != nil {
		return nil, err
	}

	limit, offset := normalizeGuardrailAuditPage(req.Page, req.PageSize)
	events, total, err := s.GuardrailAuditRepository.ListGuardrailAuditEvents(
		ctx,
		domainrepo.ListGuardrailAuditEventsRequest{
			ThreadID: req.ThreadID,
			RunID:    req.RunID,
			Limit:    limit,
			Offset:   offset,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ListGuardrailAuditEventsResponse{
		Events: make([]*GuardrailAuditEventSummary, 0, len(events)),
		Total:  total,
	}
	for _, event := range events {
		resp.Events = append(resp.Events, DomainGuardrailAuditEventToSummary(event))
	}
	return resp, nil
}

func (s *ApplicationService) ListMCPRuntimeAuditEvents(
	ctx context.Context,
	req *ListMCPRuntimeAuditEventsRequest,
) (*ListMCPRuntimeAuditEventsResponse, error) {
	if err := s.requireMCPRuntimeAuditRepository(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list mcp runtime audit events request is required")
	}
	if req.ThreadID <= 0 {
		return nil, fmt.Errorf("thread id is required")
	}
	if req.RunID < 0 {
		return nil, fmt.Errorf("run id is invalid")
	}
	if err := s.authorizeMCPRuntimeAuditAccess(ctx, MCPRuntimeAuditAccessRequest{
		ThreadID:  req.ThreadID,
		RunID:     req.RunID,
		ViewerID:  req.ViewerID,
		Operation: MCPRuntimeAuditAccessOperationList,
	}); err != nil {
		return nil, err
	}

	limit, offset := normalizeGuardrailAuditPage(req.Page, req.PageSize)
	events, total, err := s.MCPRuntimeAuditRepository.ListMCPRuntimeAuditEvents(
		ctx,
		domainrepo.ListMCPRuntimeAuditEventsRequest{
			ThreadID: req.ThreadID,
			RunID:    req.RunID,
			Limit:    limit,
			Offset:   offset,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ListMCPRuntimeAuditEventsResponse{
		Events: make([]*MCPRuntimeAuditEventSummary, 0, len(events)),
		Total:  total,
	}
	for _, event := range events {
		resp.Events = append(resp.Events, DomainMCPRuntimeAuditEventToSummary(event))
	}
	return resp, nil
}

func (s *ApplicationService) ExportGuardrailAuditEvents(
	ctx context.Context,
	req *ExportGuardrailAuditEventsRequest,
) (*ExportGuardrailAuditEventsResponse, error) {
	if err := s.requireGuardrailAuditRepository(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("export guardrail audit events request is required")
	}
	if req.ThreadID <= 0 {
		return nil, fmt.Errorf("thread id is required")
	}
	if req.RunID < 0 {
		return nil, fmt.Errorf("run id is invalid")
	}
	if err := s.authorizeGuardrailAuditAccess(ctx, GuardrailAuditAccessRequest{
		ThreadID:  req.ThreadID,
		RunID:     req.RunID,
		ViewerID:  req.ViewerID,
		Operation: GuardrailAuditAccessOperationExport,
	}); err != nil {
		return nil, err
	}

	page, pageSize, offset := normalizeGuardrailAuditExportPage(
		req.Page,
		req.PageSize,
	)
	events, total, err := s.GuardrailAuditRepository.ListGuardrailAuditEvents(
		ctx,
		domainrepo.ListGuardrailAuditEventsRequest{
			ThreadID: req.ThreadID,
			RunID:    req.RunID,
			Limit:    pageSize,
			Offset:   offset,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ExportGuardrailAuditEventsResponse{
		Schema:     GuardrailAuditExportSchema,
		ThreadID:   req.ThreadID,
		ExportedAt: time.Now().UnixMilli(),
		Page:       page,
		PageSize:   pageSize,
		Total:      total,
		Events:     make([]*GuardrailAuditEventSummary, 0, len(events)),
	}
	for _, event := range events {
		resp.Events = append(resp.Events, DomainGuardrailAuditEventToSummary(event))
	}
	return resp, nil
}

func (s *ApplicationService) PersistTranscriptSnapshot(
	ctx context.Context,
	req *PersistTranscriptSnapshotRequest,
) (*PersistTranscriptSnapshotResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("persist transcript snapshot request is required")
	}
	snapshot, created, err := s.ThreadSVC.PersistTranscriptSnapshot(
		ctx,
		&domainservice.PersistTranscriptSnapshotRequest{
			ThreadID:       req.ThreadID,
			RunID:          req.RunID,
			Kind:           domainentity.TranscriptKind(req.Kind),
			Digest:         req.Digest,
			IdempotencyKey: req.IdempotencyKey,
			MessageCount:   req.MessageCount,
			Messages:       req.Messages,
			Metadata:       req.Metadata,
		},
	)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, fmt.Errorf(
			"agent thread service returned empty transcript snapshot",
		)
	}
	return &PersistTranscriptSnapshotResponse{
		Snapshot: DomainTranscriptSnapshotToSummary(snapshot),
		Created:  created,
	}, nil
}

func (s *ApplicationService) EnqueueMemoryFlushJob(
	ctx context.Context,
	req *EnqueueMemoryFlushJobRequest,
) (*EnqueueMemoryFlushJobResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("enqueue memory flush job request is required")
	}
	job, created, err := s.ThreadSVC.EnqueueMemoryFlushJob(
		ctx,
		&domainservice.EnqueueMemoryFlushJobRequest{
			ThreadID:             req.ThreadID,
			RunID:                req.RunID,
			TranscriptSnapshotID: req.TranscriptSnapshotID,
			IdempotencyKey:       req.IdempotencyKey,
			AvailableAt:          req.AvailableAt,
		},
	)
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, fmt.Errorf(
			"agent thread service returned empty memory flush job",
		)
	}
	return &EnqueueMemoryFlushJobResponse{
		Job:     DomainMemoryFlushJobToSummary(job),
		Created: created,
	}, nil
}

func (s *ApplicationService) RecordTokenUsage(ctx context.Context, req *RecordTokenUsageRequest) (*RecordTokenUsageResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("record token usage request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		RunID: req.RunID,
	}); err != nil {
		return nil, err
	}

	usage, err := s.ThreadSVC.RecordTokenUsage(ctx, &domainservice.RecordTokenUsageRequest{
		RunID:        req.RunID,
		Source:       domainentity.TokenUsageSource(req.Source),
		StepID:       req.StepID,
		StepIndex:    req.StepIndex,
		StepName:     req.StepName,
		ModelName:    req.ModelName,
		Provider:     req.Provider,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		TotalTokens:  req.TotalTokens,
		CostMicros:   req.CostMicros,
		Currency:     req.Currency,
		Estimated:    req.Estimated,
		RawUsage:     req.RawUsage,
		Metadata:     req.Metadata,
	})
	if err != nil {
		return nil, err
	}
	if usage == nil {
		return nil, fmt.Errorf("agent thread service returned empty token usage")
	}

	return &RecordTokenUsageResponse{Usage: DomainTokenUsageToSummary(usage)}, nil
}

func (s *ApplicationService) GetRunTokenUsage(ctx context.Context, req *GetTokenUsageRequest) (*GetTokenUsageResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get run token usage request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		RunID: req.RunID,
	}); err != nil {
		return nil, err
	}

	rows, total, aggregate, runAggregates, err := s.ThreadSVC.GetRunTokenUsage(ctx, &domainservice.GetRunTokenUsageRequest{
		RunID:            req.RunID,
		IncludeChildRuns: req.IncludeChildRuns,
		Source:           domainentity.TokenUsageSource(req.Source),
		Page:             req.Page,
		PageSize:         req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	return tokenUsageResponse(rows, total, aggregate, runAggregates), nil
}

func (s *ApplicationService) GetThreadTokenUsage(ctx context.Context, req *GetTokenUsageRequest) (*GetTokenUsageResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("get thread token usage request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		ThreadID: req.ThreadID,
	}); err != nil {
		return nil, err
	}

	rows, total, aggregate, err := s.ThreadSVC.GetThreadTokenUsage(ctx, &domainservice.GetThreadTokenUsageRequest{
		ThreadID: req.ThreadID,
		Source:   domainentity.TokenUsageSource(req.Source),
		Page:     req.Page,
		PageSize: req.PageSize,
	})
	if err != nil {
		return nil, err
	}

	return tokenUsageResponse(rows, total, aggregate, nil), nil
}

func (s *ApplicationService) ListArtifacts(
	ctx context.Context,
	req *ListArtifactsRequest,
) (*ListArtifactsResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list artifacts request is required")
	}
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID:  req.ThreadID,
		SpaceID:   req.SpaceID,
		ViewerID:  req.ViewerID,
		Operation: ArtifactAccessOperationList,
	}); err != nil {
		return nil, err
	}
	artifacts, total, err := s.ArtifactSVC.ListArtifacts(
		ctx,
		&domainservice.ListArtifactsRequest{
			ThreadID:    req.ThreadID,
			RunID:       req.RunID,
			DeletedOnly: req.DeletedOnly,
			Page:        req.Page,
			PageSize:    req.PageSize,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ListArtifactsResponse{
		Artifacts: make([]*ArtifactSummary, 0, len(artifacts)),
		Total:     total,
	}
	for _, artifact := range artifacts {
		resp.Artifacts = append(resp.Artifacts, DomainArtifactToSummary(artifact))
	}
	return resp, nil
}

func (s *ApplicationService) ListArtifactScanJobs(
	ctx context.Context,
	req *ListArtifactScanJobsRequest,
) (*ListArtifactScanJobsResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list artifact scan jobs request is required")
	}
	var artifactID int64
	if req.ArtifactID != nil {
		artifactID = *req.ArtifactID
	}
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID:   req.ThreadID,
		ArtifactID: artifactID,
		SpaceID:    req.SpaceID,
		ViewerID:   req.ViewerID,
		Operation:  ArtifactAccessOperationList,
	}); err != nil {
		return nil, err
	}

	jobs, total, err := s.ArtifactSVC.ListArtifactScanJobs(
		ctx,
		&domainservice.ListArtifactScanJobsRequest{
			ThreadID:   req.ThreadID,
			RunID:      req.RunID,
			ArtifactID: req.ArtifactID,
			Status:     req.Status,
			Scanner:    req.Scanner,
			Page:       req.Page,
			PageSize:   req.PageSize,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ListArtifactScanJobsResponse{
		Jobs:  make([]*ArtifactScanJobSummary, 0, len(jobs)),
		Total: total,
	}
	for _, job := range jobs {
		resp.Jobs = append(resp.Jobs, DomainArtifactScanJobToSummary(job))
	}
	return resp, nil
}

func (s *ApplicationService) RetryArtifactScanJob(
	ctx context.Context,
	req *RetryArtifactScanJobRequest,
) (*RetryArtifactScanJobResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("retry artifact scan job request is required")
	}
	if req.ThreadID <= 0 || req.JobID <= 0 {
		return nil, fmt.Errorf("artifact scan job scope is required")
	}
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID:  req.ThreadID,
		SpaceID:   req.SpaceID,
		ViewerID:  req.ViewerID,
		Operation: ArtifactAccessOperationList,
	}); err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	job, ok, err := s.ArtifactSVC.RequeueFailedArtifactScanJob(
		ctx,
		&domainservice.RequeueFailedArtifactScanJobRequest{
			JobID:       req.JobID,
			ThreadID:    req.ThreadID,
			ErrorText:   "manual retry requested",
			AvailableAt: now,
			Now:         now,
		},
	)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrArtifactScanJobRetryNotAllowed
	}
	if err := s.auditArtifactScanJobRetryRequested(ctx, job); err != nil {
		return nil, err
	}
	return &RetryArtifactScanJobResponse{
		Job:     DomainArtifactScanJobToSummary(job),
		Retried: true,
	}, nil
}

func (s *ApplicationService) DeleteArtifact(
	ctx context.Context,
	req *DeleteArtifactRequest,
) (*DeleteArtifactResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("delete artifact request is required")
	}
	if req.ThreadID <= 0 || req.ArtifactID <= 0 {
		return nil, fmt.Errorf("artifact scope is required")
	}
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID:   req.ThreadID,
		ArtifactID: req.ArtifactID,
		SpaceID:    req.SpaceID,
		ViewerID:   req.ViewerID,
		Operation:  ArtifactAccessOperationDelete,
	}); err != nil {
		return nil, err
	}
	deletedAt := req.DeletedAt
	if deletedAt <= 0 {
		deletedAt = time.Now().UnixMilli()
	}

	artifact, deleted, err := s.ArtifactSVC.DeleteArtifact(
		ctx,
		&domainservice.DeleteArtifactRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			DeletedAt:  deletedAt,
		},
	)
	if err != nil {
		return nil, err
	}
	if deleted {
		if err := s.auditArtifactDeleted(ctx, artifact); err != nil {
			return nil, err
		}
	}
	return &DeleteArtifactResponse{Deleted: deleted}, nil
}

func (s *ApplicationService) RestoreArtifact(
	ctx context.Context,
	req *RestoreArtifactRequest,
) (*RestoreArtifactResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("restore artifact request is required")
	}
	if req.ThreadID <= 0 || req.ArtifactID <= 0 {
		return nil, fmt.Errorf("artifact scope is required")
	}
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID:   req.ThreadID,
		ArtifactID: req.ArtifactID,
		SpaceID:    req.SpaceID,
		ViewerID:   req.ViewerID,
		Operation:  ArtifactAccessOperationRestore,
	}); err != nil {
		return nil, err
	}
	restoredAt := req.RestoredAt
	if restoredAt <= 0 {
		restoredAt = time.Now().UnixMilli()
	}

	artifact, restored, err := s.ArtifactSVC.RestoreArtifact(
		ctx,
		&domainservice.RestoreArtifactRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			RestoredAt: restoredAt,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &RestoreArtifactResponse{Restored: restored}
	if artifact != nil {
		resp.Artifact = DomainArtifactToSummary(artifact)
	}
	if restored {
		if err := s.auditArtifactRestored(ctx, artifact, restoredAt); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (s *ApplicationService) ProcessDeletedArtifactCleanup(
	ctx context.Context,
	req *ProcessDeletedArtifactCleanupRequest,
) (*ProcessDeletedArtifactCleanupResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	deleter, ok := s.ArtifactObjectStorage.(ArtifactObjectDeleter)
	if s.ArtifactObjectStorage == nil || !ok {
		return nil, fmt.Errorf("artifact object storage deletion is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("process deleted artifact cleanup request is required")
	}
	now := s.artifactCleanupNow(req.NowMillis)
	retentionMillis := req.RetentionMillis
	if retentionMillis <= 0 {
		retentionMillis = int64(7 * 24 * time.Hour / time.Millisecond)
	}
	cutoff := now - retentionMillis
	if cutoff <= 0 {
		return &ProcessDeletedArtifactCleanupResponse{}, nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	candidates, err := s.ArtifactSVC.ListDeletedArtifactCleanupCandidates(
		ctx,
		&domainservice.ListDeletedArtifactCleanupCandidatesRequest{
			CutoffDeletedAt: cutoff,
			Limit:           limit,
		},
	)
	if err != nil {
		return nil, err
	}

	resp := &ProcessDeletedArtifactCleanupResponse{
		Candidates: int32(len(candidates)),
	}
	for _, artifact := range candidates {
		if artifact == nil {
			resp.Skipped++
			continue
		}
		objectURI := strings.TrimSpace(artifact.ObjectURI)
		if objectURI == "" {
			resp.Failed++
			continue
		}
		deleteErr := deleter.DeleteObject(ctx, objectURI)
		notFound := false
		if deleteErr != nil {
			if !errors.Is(deleteErr, storage.ErrObjectNotFound) {
				resp.Failed++
				continue
			}
			notFound = true
			resp.NotFound++
		}
		marked, err := s.ArtifactSVC.MarkArtifactFileDeleted(
			ctx,
			&domainservice.MarkArtifactFileDeletedRequest{
				FileID:    artifact.FileID,
				ObjectURI: objectURI,
				DeletedAt: now,
			},
		)
		if err != nil {
			resp.Failed++
			continue
		}
		if !marked {
			resp.Skipped++
			continue
		}
		resp.Deleted++
		if err := s.auditArtifactCleaned(ctx, artifact, now, notFound); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (s *ApplicationService) RecordArtifactScanResult(
	ctx context.Context,
	req *RecordArtifactScanResultRequest,
) (*RecordArtifactScanResultResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("record artifact scan result request is required")
	}

	artifact, updated, err := s.ArtifactSVC.UpdateArtifactScanResult(
		ctx,
		&domainservice.UpdateArtifactScanResultRequest{
			ThreadID:       req.ThreadID,
			ArtifactID:     req.ArtifactID,
			ScanStatus:     req.ScanStatus,
			Scanner:        req.Scanner,
			ScannerVersion: req.ScannerVersion,
			Reason:         req.Reason,
			ScannedAt:      req.ScannedAt,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &RecordArtifactScanResultResponse{Updated: updated}
	if artifact != nil {
		resp.Artifact = DomainArtifactToSummary(artifact)
	}
	if updated {
		if err := s.auditArtifactScanCompleted(ctx, artifact); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (s *ApplicationService) ReviewArtifactScan(
	ctx context.Context,
	req *ReviewArtifactScanRequest,
) (*ReviewArtifactScanResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("review artifact scan request is required")
	}
	decision, scanStatus, defaultReason, err := normalizeArtifactScanReviewDecision(
		req.Decision,
	)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID:   req.ThreadID,
		ArtifactID: req.ArtifactID,
		SpaceID:    req.SpaceID,
		ViewerID:   req.ViewerID,
		Operation:  ArtifactAccessOperationReview,
	}); err != nil {
		return nil, err
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = defaultReason
	}
	reviewedAt := s.artifactReviewNow()
	artifact, updated, err := s.ArtifactSVC.UpdateArtifactScanResult(
		ctx,
		&domainservice.UpdateArtifactScanResultRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			ScanStatus: scanStatus,
			Scanner:    manualArtifactReviewScanner,
			Reason:     reason,
			ScannedAt:  reviewedAt,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ReviewArtifactScanResponse{
		ArtifactID: req.ArtifactID,
		Decision:   decision,
		ScanStatus: scanStatus,
		Reviewed:   updated,
	}
	if artifact != nil {
		resp.ArtifactID = artifact.ID
		if gotStatus := normalizeArtifactScanStatus(artifact.Metadata); gotStatus != artifactScanStatusUnknown {
			resp.ScanStatus = string(gotStatus)
		}
	}
	if updated {
		if err := s.auditArtifactScanReviewed(ctx, artifact, decision, reviewedAt); err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (s *ApplicationService) ProcessArtifactScanJobs(
	ctx context.Context,
	req *ProcessArtifactScanJobsRequest,
) (*ProcessArtifactScanJobsResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if s.ArtifactObjectStorage == nil {
		return nil, fmt.Errorf("artifact object storage is not configured")
	}
	if s.ArtifactScanner == nil {
		return nil, fmt.Errorf("artifact content scanner is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("process artifact scan jobs request is required")
	}
	workerID := strings.TrimSpace(req.WorkerID)
	if workerID == "" {
		return nil, fmt.Errorf("worker id is required")
	}
	scannerName := strings.TrimSpace(req.Scanner)
	if scannerName == "" {
		scannerName = defaultApplicationArtifactScanner
	}
	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	retryBackoffMillis := req.RetryBackoffMillis
	if retryBackoffMillis <= 0 {
		retryBackoffMillis = defaultArtifactScanRetryBackoffMillis
	}

	jobs, err := s.ArtifactSVC.ClaimArtifactScanJobs(
		ctx,
		&domainservice.ClaimArtifactScanJobsRequest{
			Scanner:        scannerName,
			WorkerID:       workerID,
			Limit:          req.Limit,
			LeaseTTLMillis: req.LeaseTTLMillis,
		},
	)
	if err != nil {
		return nil, err
	}
	resp := &ProcessArtifactScanJobsResponse{Claimed: int32(len(jobs))}
	for _, job := range jobs {
		outcome, updatedJob, contentFamily, err := s.processArtifactScanJob(
			ctx,
			job,
			workerID,
			scannerName,
			maxAttempts,
			retryBackoffMillis,
		)
		if err != nil {
			return nil, err
		}
		if metric := artifactScanJobMetricSummary(job, updatedJob, contentFamily, scannerName, outcome); metric != nil {
			resp.JobMetrics = append(resp.JobMetrics, metric)
		}
		switch outcome {
		case artifactScanJobProcessSucceeded:
			resp.Succeeded++
		case artifactScanJobProcessRetried:
			resp.Retried++
		case artifactScanJobProcessFailed:
			resp.Failed++
		default:
			resp.Skipped++
		}
	}
	return resp, nil
}

func (s *ApplicationService) ReadArtifactContent(
	ctx context.Context,
	req *ReadArtifactContentRequest,
) (*ReadArtifactContentResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	if s.ArtifactObjectStorage == nil {
		return nil, fmt.Errorf("artifact object storage is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("read artifact content request is required")
	}
	if req.ThreadID <= 0 || req.ArtifactID <= 0 {
		return nil, fmt.Errorf("artifact scope is required")
	}
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID:   req.ThreadID,
		ArtifactID: req.ArtifactID,
		SpaceID:    req.SpaceID,
		ViewerID:   req.ViewerID,
		Operation:  ArtifactAccessOperationRead,
	}); err != nil {
		return nil, err
	}

	artifact, err := s.ArtifactSVC.GetArtifact(ctx, &domainservice.GetArtifactRequest{
		ThreadID:   req.ThreadID,
		ArtifactID: req.ArtifactID,
	})
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, fmt.Errorf("artifact not found")
	}
	objectURI := strings.TrimSpace(artifact.ObjectURI)
	if objectURI == "" {
		return nil, fmt.Errorf("artifact object is not registered")
	}
	scanStatus := normalizeArtifactScanStatus(artifact.Metadata)
	readPolicy := artifactScanReadPolicy(scanStatus, artifact, s.ArtifactScanReadPolicy)
	if !readPolicy.Allowed {
		if err := s.auditArtifactContentBlocked(ctx, req, artifact, readPolicy); err != nil {
			return nil, err
		}
		return nil, &ArtifactContentBlockedByScanError{
			ScanStatus: string(scanStatus),
			Reason:     readPolicy.Reason,
		}
	}
	content, err := s.ArtifactObjectStorage.GetObject(ctx, objectURI)
	if err != nil {
		return nil, err
	}

	contentType := effectiveArtifactContentType(artifact.ContentType, content)
	resp := &ReadArtifactContentResponse{
		Artifact:    DomainArtifactToSummary(artifact),
		Content:     content,
		ContentType: contentType,
		FileName:    artifactContentFileName(artifact),
		Attachment:  shouldAttachArtifactContent(req.Mode, artifact, contentType),
	}
	if err := s.auditArtifactContentAccess(ctx, req, artifact, resp, scanStatus, readPolicy); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *ApplicationService) CreateArtifactSignedURL(
	ctx context.Context,
	req *CreateArtifactSignedURLRequest,
) (*CreateArtifactSignedURLResponse, error) {
	if err := s.requireArtifactSVC(); err != nil {
		return nil, err
	}
	signer, ok := s.ArtifactObjectStorage.(ArtifactObjectURLSigner)
	if s.ArtifactObjectStorage == nil || !ok {
		return nil, fmt.Errorf("artifact object storage signing is not configured")
	}
	if req == nil {
		return nil, fmt.Errorf("create artifact signed url request is required")
	}
	if req.ThreadID <= 0 || req.ArtifactID <= 0 {
		return nil, fmt.Errorf("artifact scope is required")
	}
	mode := normalizeArtifactContentMode(req.Mode)
	if err := s.authorizeArtifactAccess(ctx, ArtifactAccessRequest{
		ThreadID:   req.ThreadID,
		ArtifactID: req.ArtifactID,
		SpaceID:    req.SpaceID,
		ViewerID:   req.ViewerID,
		Operation:  ArtifactAccessOperationRead,
	}); err != nil {
		return nil, err
	}

	artifact, err := s.ArtifactSVC.GetArtifact(ctx, &domainservice.GetArtifactRequest{
		ThreadID:   req.ThreadID,
		ArtifactID: req.ArtifactID,
	})
	if err != nil {
		return nil, err
	}
	if artifact == nil {
		return nil, fmt.Errorf("artifact not found")
	}
	objectURI := strings.TrimSpace(artifact.ObjectURI)
	if objectURI == "" {
		return nil, fmt.Errorf("artifact object is not registered")
	}
	scanStatus := normalizeArtifactScanStatus(artifact.Metadata)
	readPolicy := artifactScanReadPolicy(scanStatus, artifact, s.ArtifactScanReadPolicy)
	if !readPolicy.Allowed {
		if err := s.auditArtifactContentBlocked(ctx, &ReadArtifactContentRequest{
			ThreadID:   req.ThreadID,
			ArtifactID: req.ArtifactID,
			Mode:       mode,
			ViewerID:   req.ViewerID,
		}, artifact, readPolicy); err != nil {
			return nil, err
		}
		return nil, &ArtifactContentBlockedByScanError{
			ScanStatus: string(scanStatus),
			Reason:     readPolicy.Reason,
		}
	}

	content, err := s.ArtifactObjectStorage.GetObject(ctx, objectURI)
	if err != nil {
		return nil, err
	}
	contentType := effectiveArtifactContentType(artifact.ContentType, content)
	if mode == ArtifactContentModePreview &&
		!canCreateArtifactSignedPreviewURL(artifact, contentType) {
		return nil, ErrArtifactSignedURLNotSupported
	}

	expiresIn := normalizeArtifactSignedURLTTL(req.TTLSeconds)
	signOpts := []storage.GetOptFn{storage.WithExpire(expiresIn)}
	if mode == ArtifactContentModeDownload {
		signOpts = append(
			signOpts,
			storage.WithResponseContentDisposition(
				artifactContentDisposition(artifactContentFileName(artifact), true),
			),
			storage.WithResponseContentType(contentType),
		)
	}
	signedURL, err := signer.GetObjectUrl(ctx, objectURI, signOpts...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(signedURL) == "" {
		return nil, fmt.Errorf("artifact signed url is empty")
	}

	return &CreateArtifactSignedURLResponse{
		Artifact:         DomainArtifactToSummary(artifact),
		URL:              signedURL,
		ExpiresInSeconds: expiresIn,
		ContentType:      contentType,
		PreviewMode:      ArtifactPreviewMode(artifact.PreviewMode),
	}, nil
}

type artifactScanJobProcessOutcome int

const (
	artifactScanJobProcessSkipped artifactScanJobProcessOutcome = iota
	artifactScanJobProcessSucceeded
	artifactScanJobProcessRetried
	artifactScanJobProcessFailed
)

func (s *ApplicationService) processArtifactScanJob(
	ctx context.Context,
	job *domainentity.ArtifactScanJob,
	workerID string,
	defaultScanner string,
	maxAttempts int32,
	retryBackoffMillis int64,
) (artifactScanJobProcessOutcome, *domainentity.ArtifactScanJob, string, error) {
	if job == nil {
		return artifactScanJobProcessSkipped, nil, "", nil
	}
	artifact, err := s.ArtifactSVC.GetArtifact(
		ctx,
		&domainservice.GetArtifactRequest{
			ThreadID:   job.ThreadID,
			ArtifactID: job.ArtifactID,
		},
	)
	if err != nil {
		return artifactScanJobProcessSkipped, nil, "", err
	}
	contentFamily := artifactScanContentFamily(artifact)
	if artifact == nil {
		return s.failClaimedArtifactScanJob(
			ctx,
			job,
			workerID,
			"artifact scan artifact missing",
			maxAttempts,
			retryBackoffMillis,
			contentFamily,
		)
	}
	objectURI := strings.TrimSpace(artifact.ObjectURI)
	if objectURI == "" {
		return s.failClaimedArtifactScanJob(
			ctx,
			job,
			workerID,
			"artifact scan object missing",
			maxAttempts,
			retryBackoffMillis,
			contentFamily,
		)
	}
	content, err := s.ArtifactObjectStorage.GetObject(ctx, objectURI)
	if err != nil {
		return s.failClaimedArtifactScanJob(
			ctx,
			job,
			workerID,
			"artifact scan storage read failed",
			maxAttempts,
			retryBackoffMillis,
			contentFamily,
		)
	}
	scannerName := strings.TrimSpace(job.Scanner)
	if scannerName == "" {
		scannerName = defaultScanner
	}
	result, err := s.ArtifactScanner.ScanArtifact(
		ctx,
		ArtifactScanRequest{
			SpaceID:     artifact.SpaceID,
			ThreadID:    artifact.ThreadID,
			RunID:       artifact.RunID,
			UserID:      artifact.UserID,
			ArtifactID:  artifact.ID,
			FileID:      artifact.FileID,
			Scanner:     scannerName,
			ContentType: artifact.ContentType,
			SizeBytes:   artifact.SizeBytes,
			Content:     content,
		},
	)
	if err != nil {
		return s.failClaimedArtifactScanJob(
			ctx,
			job,
			workerID,
			"artifact scan failed",
			maxAttempts,
			retryBackoffMillis,
			contentFamily,
		)
	}
	if result == nil || strings.TrimSpace(result.ScanStatus) == "" {
		return s.failClaimedArtifactScanJob(
			ctx,
			job,
			workerID,
			"artifact scan result invalid",
			maxAttempts,
			retryBackoffMillis,
			contentFamily,
		)
	}
	scannedAt := time.Now().UnixMilli()

	completed, ok, err := s.ArtifactSVC.CompleteArtifactScanJob(
		ctx,
		&domainservice.CompleteArtifactScanJobRequest{
			JobID:          job.ID,
			WorkerID:       workerID,
			ScanStatus:     result.ScanStatus,
			ScannerVersion: result.ScannerVersion,
			Reason:         result.Reason,
			ScannedAt:      scannedAt,
		},
	)
	if err != nil {
		return artifactScanJobProcessSkipped, nil, contentFamily, err
	}
	if !ok {
		return artifactScanJobProcessSkipped, nil, contentFamily, nil
	}
	if err := s.auditArtifactScanCompleted(
		ctx,
		artifactScanAuditArtifact(artifact, result, scannerName, scannedAt),
	); err != nil {
		return artifactScanJobProcessSkipped, nil, contentFamily, err
	}
	return artifactScanJobProcessSucceeded, completed, contentFamily, nil
}

func (s *ApplicationService) failClaimedArtifactScanJob(
	ctx context.Context,
	job *domainentity.ArtifactScanJob,
	workerID string,
	errorText string,
	maxAttempts int32,
	retryBackoffMillis int64,
	contentFamily string,
) (artifactScanJobProcessOutcome, *domainentity.ArtifactScanJob, string, error) {
	if job == nil {
		return artifactScanJobProcessSkipped, nil, "", nil
	}
	if shouldRetryArtifactScanJob(job, maxAttempts) {
		now := time.Now().UnixMilli()
		retried, ok, err := s.ArtifactSVC.RetryArtifactScanJob(
			ctx,
			&domainservice.RetryArtifactScanJobRequest{
				JobID:       job.ID,
				WorkerID:    workerID,
				ErrorText:   errorText,
				AvailableAt: now + retryBackoffMillis,
				Now:         now,
			},
		)
		if err != nil {
			return artifactScanJobProcessSkipped, nil, contentFamily, err
		}
		if !ok {
			return artifactScanJobProcessSkipped, nil, contentFamily, nil
		}
		return artifactScanJobProcessRetried, retried, contentFamily, nil
	}
	failed, ok, err := s.ArtifactSVC.FailArtifactScanJob(
		ctx,
		&domainservice.FailArtifactScanJobRequest{
			JobID:     job.ID,
			WorkerID:  workerID,
			ErrorText: errorText,
			EndedAt:   time.Now().UnixMilli(),
		},
	)
	if err != nil {
		return artifactScanJobProcessSkipped, nil, contentFamily, err
	}
	if !ok {
		return artifactScanJobProcessSkipped, nil, contentFamily, nil
	}
	return artifactScanJobProcessFailed, failed, contentFamily, nil
}

func artifactScanJobMetricSummary(
	claimed *domainentity.ArtifactScanJob,
	updated *domainentity.ArtifactScanJob,
	contentFamily string,
	defaultScanner string,
	outcome artifactScanJobProcessOutcome,
) *ArtifactScanJobMetricsSummary {
	if claimed == nil {
		return nil
	}
	scanner := strings.TrimSpace(claimed.Scanner)
	if scanner == "" {
		scanner = strings.TrimSpace(defaultScanner)
	}
	result := "skipped"
	errorCode := runtimeMetricErrorNone
	observeLatency := false
	switch outcome {
	case artifactScanJobProcessSucceeded:
		result = runtimeMetricResultSuccess
		observeLatency = true
	case artifactScanJobProcessRetried:
		result = "retried"
		errorCode = runtimeMetricErrorProcessFailed
	case artifactScanJobProcessFailed:
		result = runtimeMetricResultFailed
		errorCode = runtimeMetricErrorProcessFailed
		observeLatency = true
	default:
		result = "skipped"
	}

	startedAt := claimed.StartedAt
	endedAt := int64(0)
	if updated != nil {
		if updated.Scanner != "" {
			scanner = updated.Scanner
		}
		if updated.StartedAt > 0 {
			startedAt = updated.StartedAt
		}
		endedAt = updated.EndedAt
	}
	if observeLatency && endedAt <= 0 {
		observeLatency = false
	}
	observeQueueDelay := claimed.CreatedAt > 0 && startedAt > 0

	return &ArtifactScanJobMetricsSummary{
		Scanner:           scanner,
		ContentFamily:     contentFamily,
		Result:            result,
		ErrorCode:         errorCode,
		CreatedAt:         claimed.CreatedAt,
		StartedAt:         startedAt,
		EndedAt:           endedAt,
		ObserveLatency:    observeLatency,
		ObserveQueueDelay: observeQueueDelay,
	}
}

func artifactScanContentFamily(artifact *domainentity.AgentArtifact) string {
	if artifact == nil {
		return "unknown"
	}
	contentType := strings.ToLower(strings.TrimSpace(artifact.ContentType))
	if slash := strings.Index(contentType, "/"); slash > 0 {
		contentType = contentType[:slash]
	}
	switch contentType {
	case "text", "image", "audio", "video":
		return contentType
	case "application":
		normalized := strings.ToLower(strings.TrimSpace(artifact.ContentType))
		switch {
		case strings.Contains(normalized, "json"):
			return "json"
		case strings.Contains(normalized, "pdf"):
			return "pdf"
		default:
			return "binary"
		}
	case "":
		return "unknown"
	default:
		return "binary"
	}
}

func shouldRetryArtifactScanJob(
	job *domainentity.ArtifactScanJob,
	maxAttempts int32,
) bool {
	return job != nil && maxAttempts > 1 && job.AttemptCount < maxAttempts
}

func artifactScanAuditArtifact(
	artifact *domainentity.AgentArtifact,
	result *ArtifactScanResult,
	scanner string,
	scannedAt int64,
) *domainentity.AgentArtifact {
	if artifact == nil {
		return nil
	}
	cloned := *artifact
	metadata := map[string]any{
		"scan_status":     strings.TrimSpace(result.ScanStatus),
		"scan_scanner":    strings.TrimSpace(scanner),
		"scan_scanned_at": scannedAt,
	}
	if scannerVersion := strings.TrimSpace(result.ScannerVersion); scannerVersion != "" {
		metadata["scan_scanner_version"] = scannerVersion
	}
	raw, err := json.Marshal(metadata)
	if err == nil {
		cloned.Metadata = string(raw)
	}
	return &cloned
}

func (s *ApplicationService) auditArtifactScanCompleted(
	ctx context.Context,
	artifact *domainentity.AgentArtifact,
) error {
	if s == nil || s.ThreadSVC == nil || artifact == nil || artifact.RunID <= 0 {
		return nil
	}
	scanStatus, scanner, scannerVersion, scannedAt := artifactScanAuditFields(
		artifact.Metadata,
	)
	payload, err := json.Marshal(struct {
		Schema         string `json:"schema"`
		ThreadID       int64  `json:"thread_id"`
		RunID          int64  `json:"run_id"`
		ArtifactID     int64  `json:"artifact_id"`
		FileID         int64  `json:"file_id"`
		ArtifactType   string `json:"artifact_type"`
		ContentType    string `json:"content_type"`
		SizeBytes      int64  `json:"size_bytes"`
		ScanStatus     string `json:"scan_status"`
		Scanner        string `json:"scanner,omitempty"`
		ScannerVersion string `json:"scanner_version,omitempty"`
		ScannedAt      int64  `json:"scanned_at"`
	}{
		Schema:         "coze.artifact_scan.v1",
		ThreadID:       artifact.ThreadID,
		RunID:          artifact.RunID,
		ArtifactID:     artifact.ID,
		FileID:         artifact.FileID,
		ArtifactType:   artifact.ArtifactType,
		ContentType:    artifact.ContentType,
		SizeBytes:      artifact.SizeBytes,
		ScanStatus:     scanStatus,
		Scanner:        scanner,
		ScannerVersion: scannerVersion,
		ScannedAt:      scannedAt,
	})
	if err != nil {
		return err
	}
	_, err = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  artifact.ThreadID,
		RunID:     artifact.RunID,
		EventType: artifactScanCompletedEvent,
		Payload:   string(payload),
	})
	if err != nil {
		return fmt.Errorf("audit artifact scan result: %w", err)
	}
	return nil
}

func (s *ApplicationService) auditArtifactScanReviewed(
	ctx context.Context,
	artifact *domainentity.AgentArtifact,
	decision string,
	reviewedAt int64,
) error {
	if s == nil || s.ThreadSVC == nil || artifact == nil || artifact.RunID <= 0 {
		return nil
	}
	scanStatus, scanner, scannerVersion, scannedAt := artifactScanAuditFields(
		artifact.Metadata,
	)
	if reviewedAt <= 0 {
		reviewedAt = scannedAt
	}
	payload, err := json.Marshal(struct {
		Schema         string `json:"schema"`
		ThreadID       int64  `json:"thread_id"`
		RunID          int64  `json:"run_id"`
		ArtifactID     int64  `json:"artifact_id"`
		FileID         int64  `json:"file_id"`
		ArtifactType   string `json:"artifact_type"`
		ContentType    string `json:"content_type"`
		SizeBytes      int64  `json:"size_bytes"`
		Decision       string `json:"decision"`
		ScanStatus     string `json:"scan_status"`
		Scanner        string `json:"scanner,omitempty"`
		ScannerVersion string `json:"scanner_version,omitempty"`
		ScannedAt      int64  `json:"scanned_at"`
		ReviewedAt     int64  `json:"reviewed_at"`
	}{
		Schema:         "coze.artifact_scan_review.v1",
		ThreadID:       artifact.ThreadID,
		RunID:          artifact.RunID,
		ArtifactID:     artifact.ID,
		FileID:         artifact.FileID,
		ArtifactType:   artifact.ArtifactType,
		ContentType:    artifact.ContentType,
		SizeBytes:      artifact.SizeBytes,
		Decision:       decision,
		ScanStatus:     scanStatus,
		Scanner:        scanner,
		ScannerVersion: scannerVersion,
		ScannedAt:      scannedAt,
		ReviewedAt:     reviewedAt,
	})
	if err != nil {
		return err
	}
	_, err = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  artifact.ThreadID,
		RunID:     artifact.RunID,
		EventType: artifactScanReviewedEvent,
		Payload:   string(payload),
	})
	if err != nil {
		return fmt.Errorf("audit artifact scan review: %w", err)
	}
	return nil
}

func (s *ApplicationService) auditArtifactScanJobRetryRequested(
	ctx context.Context,
	job *domainentity.ArtifactScanJob,
) error {
	if s == nil || s.ThreadSVC == nil || job == nil || job.RunID <= 0 {
		return nil
	}
	payload, err := json.Marshal(struct {
		Schema       string `json:"schema"`
		JobID        int64  `json:"job_id"`
		ThreadID     int64  `json:"thread_id"`
		RunID        int64  `json:"run_id"`
		ArtifactID   int64  `json:"artifact_id"`
		FileID       int64  `json:"file_id"`
		Scanner      string `json:"scanner,omitempty"`
		Status       string `json:"status"`
		AttemptCount int32  `json:"attempt_count"`
		AvailableAt  int64  `json:"available_at"`
	}{
		Schema:       "coze.artifact_scan_job_retry_requested.v1",
		JobID:        job.ID,
		ThreadID:     job.ThreadID,
		RunID:        job.RunID,
		ArtifactID:   job.ArtifactID,
		FileID:       job.FileID,
		Scanner:      job.Scanner,
		Status:       string(job.Status),
		AttemptCount: job.AttemptCount,
		AvailableAt:  job.AvailableAt,
	})
	if err != nil {
		return err
	}
	_, err = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  job.ThreadID,
		RunID:     job.RunID,
		EventType: artifactScanJobRetryRequestedEvent,
		Payload:   string(payload),
	})
	if err != nil {
		return fmt.Errorf("audit artifact scan job retry requested: %w", err)
	}
	return nil
}

func (s *ApplicationService) auditArtifactDeleted(
	ctx context.Context,
	artifact *domainentity.AgentArtifact,
) error {
	if s == nil || s.ThreadSVC == nil || artifact == nil || artifact.RunID <= 0 {
		return nil
	}
	payload, err := json.Marshal(struct {
		Schema       string `json:"schema"`
		ThreadID     int64  `json:"thread_id"`
		RunID        int64  `json:"run_id"`
		ArtifactID   int64  `json:"artifact_id"`
		FileID       int64  `json:"file_id"`
		ArtifactType string `json:"artifact_type"`
		ContentType  string `json:"content_type"`
		SizeBytes    int64  `json:"size_bytes"`
		DeletedAt    int64  `json:"deleted_at"`
	}{
		Schema:       "coze.artifact_deleted.v1",
		ThreadID:     artifact.ThreadID,
		RunID:        artifact.RunID,
		ArtifactID:   artifact.ID,
		FileID:       artifact.FileID,
		ArtifactType: artifact.ArtifactType,
		ContentType:  artifact.ContentType,
		SizeBytes:    artifact.SizeBytes,
		DeletedAt:    artifact.DeletedAt,
	})
	if err != nil {
		return err
	}
	_, err = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  artifact.ThreadID,
		RunID:     artifact.RunID,
		EventType: artifactDeletedEvent,
		Payload:   string(payload),
	})
	if err != nil {
		return fmt.Errorf("audit artifact delete: %w", err)
	}
	return nil
}

func (s *ApplicationService) auditArtifactCleaned(
	ctx context.Context,
	artifact *domainentity.AgentArtifact,
	cleanedAt int64,
	notFound bool,
) error {
	if s == nil || s.ThreadSVC == nil || artifact == nil || artifact.RunID <= 0 {
		return nil
	}
	payload, err := json.Marshal(struct {
		Schema       string `json:"schema"`
		ThreadID     int64  `json:"thread_id"`
		RunID        int64  `json:"run_id"`
		ArtifactID   int64  `json:"artifact_id"`
		FileID       int64  `json:"file_id"`
		ArtifactType string `json:"artifact_type"`
		ContentType  string `json:"content_type"`
		SizeBytes    int64  `json:"size_bytes"`
		DeletedAt    int64  `json:"deleted_at"`
		CleanedAt    int64  `json:"cleaned_at"`
		NotFound     bool   `json:"not_found,omitempty"`
	}{
		Schema:       "coze.artifact_cleaned.v1",
		ThreadID:     artifact.ThreadID,
		RunID:        artifact.RunID,
		ArtifactID:   artifact.ID,
		FileID:       artifact.FileID,
		ArtifactType: artifact.ArtifactType,
		ContentType:  artifact.ContentType,
		SizeBytes:    artifact.SizeBytes,
		DeletedAt:    artifact.DeletedAt,
		CleanedAt:    cleanedAt,
		NotFound:     notFound,
	})
	if err != nil {
		return err
	}
	_, err = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  artifact.ThreadID,
		RunID:     artifact.RunID,
		EventType: artifactCleanedEvent,
		Payload:   string(payload),
	})
	if err != nil {
		return fmt.Errorf("audit artifact cleanup: %w", err)
	}
	return nil
}

func (s *ApplicationService) auditArtifactRestored(
	ctx context.Context,
	artifact *domainentity.AgentArtifact,
	restoredAt int64,
) error {
	if s == nil || s.ThreadSVC == nil || artifact == nil || artifact.RunID <= 0 {
		return nil
	}
	payload, err := json.Marshal(struct {
		Schema       string `json:"schema"`
		ThreadID     int64  `json:"thread_id"`
		RunID        int64  `json:"run_id"`
		ArtifactID   int64  `json:"artifact_id"`
		FileID       int64  `json:"file_id"`
		ArtifactType string `json:"artifact_type"`
		ContentType  string `json:"content_type"`
		SizeBytes    int64  `json:"size_bytes"`
		RestoredAt   int64  `json:"restored_at"`
	}{
		Schema:       "coze.artifact_restored.v1",
		ThreadID:     artifact.ThreadID,
		RunID:        artifact.RunID,
		ArtifactID:   artifact.ID,
		FileID:       artifact.FileID,
		ArtifactType: artifact.ArtifactType,
		ContentType:  artifact.ContentType,
		SizeBytes:    artifact.SizeBytes,
		RestoredAt:   restoredAt,
	})
	if err != nil {
		return err
	}
	_, err = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  artifact.ThreadID,
		RunID:     artifact.RunID,
		EventType: artifactRestoredEvent,
		Payload:   string(payload),
	})
	if err != nil {
		return fmt.Errorf("audit artifact restore: %w", err)
	}
	return nil
}

func artifactScanAuditFields(metadata string) (string, string, string, int64) {
	status := string(normalizeArtifactScanStatus(metadata))
	var raw map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(metadata)), &raw); err != nil {
		return status, "", "", 0
	}
	scanner := trimmedStringValue(raw["scan_scanner"])
	scannerVersion := trimmedStringValue(raw["scan_scanner_version"])
	return status, scanner, scannerVersion, int64NumberValue(raw["scan_scanned_at"])
}

func (s *ApplicationService) artifactReviewNow() int64 {
	if s != nil && s.ArtifactReviewClock != nil {
		if now := s.ArtifactReviewClock(); now > 0 {
			return now
		}
	}
	return time.Now().UnixMilli()
}

func (s *ApplicationService) artifactCleanupNow(reqNow int64) int64 {
	if reqNow > 0 {
		return reqNow
	}
	if s != nil && s.ArtifactCleanupNowFunc != nil {
		if now := s.ArtifactCleanupNowFunc(); now > 0 {
			return now
		}
	}
	return time.Now().UnixMilli()
}

func normalizeArtifactScanReviewDecision(
	decision string,
) (string, string, string, error) {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "release":
		return "release", string(artifactScanStatusClean), "manual release requested", nil
	case "quarantine":
		return "quarantine", string(artifactScanStatusQuarantined), "manual quarantine requested", nil
	case "block":
		return "block", string(artifactScanStatusBlocked), "manual block requested", nil
	default:
		return "", "", "", ErrArtifactScanReviewDecisionInvalid
	}
}

func trimmedStringValue(value any) string {
	raw, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(raw)
}

func int64NumberValue(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		got, _ := typed.Int64()
		return got
	default:
		return 0
	}
}

func normalizeArtifactScanStatus(metadata string) artifactScanStatus {
	trimmed := strings.TrimSpace(metadata)
	if trimmed == "" {
		return artifactScanStatusUnknown
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return artifactScanStatusUnknown
	}
	for _, key := range []string{"scan_status", "scanStatus"} {
		value, ok := raw[key].(string)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case string(artifactScanStatusClean):
			return artifactScanStatusClean
		case string(artifactScanStatusPending):
			return artifactScanStatusPending
		case string(artifactScanStatusFailed):
			return artifactScanStatusFailed
		case string(artifactScanStatusBlocked):
			return artifactScanStatusBlocked
		case string(artifactScanStatusInfected):
			return artifactScanStatusInfected
		case string(artifactScanStatusQuarantined):
			return artifactScanStatusQuarantined
		default:
			return artifactScanStatusUnknown
		}
	}
	return artifactScanStatusUnknown
}

type artifactScanReadPolicyDecision struct {
	Allowed  bool
	Reason   string
	Override bool
	FailMode string
}

func (s *ApplicationService) auditArtifactContentAccess(
	ctx context.Context,
	req *ReadArtifactContentRequest,
	artifact *domainentity.AgentArtifact,
	resp *ReadArtifactContentResponse,
	scanStatus artifactScanStatus,
	policy artifactScanReadPolicyDecision,
) error {
	if s == nil || s.ThreadSVC == nil || req == nil || artifact == nil || resp == nil || artifact.RunID <= 0 {
		return nil
	}
	payload, err := json.Marshal(struct {
		Schema             string `json:"schema"`
		ThreadID           int64  `json:"thread_id"`
		RunID              int64  `json:"run_id"`
		ArtifactID         int64  `json:"artifact_id"`
		FileID             int64  `json:"file_id"`
		Mode               string `json:"mode"`
		PreviewMode        string `json:"preview_mode"`
		ArtifactType       string `json:"artifact_type"`
		ContentType        string `json:"content_type"`
		SizeBytes          int64  `json:"size_bytes"`
		Attachment         bool   `json:"attachment"`
		ScanStatus         string `json:"scan_status"`
		ScanPolicyMode     string `json:"scan_policy_mode,omitempty"`
		ScanPolicyReason   string `json:"scan_policy_reason,omitempty"`
		ScanPolicyOverride bool   `json:"scan_policy_override,omitempty"`
	}{
		Schema:             "coze.artifact_access.v1",
		ThreadID:           req.ThreadID,
		RunID:              artifact.RunID,
		ArtifactID:         artifact.ID,
		FileID:             artifact.FileID,
		Mode:               string(normalizeArtifactContentMode(req.Mode)),
		PreviewMode:        string(artifact.PreviewMode),
		ArtifactType:       artifact.ArtifactType,
		ContentType:        resp.ContentType,
		SizeBytes:          artifact.SizeBytes,
		Attachment:         resp.Attachment,
		ScanStatus:         string(scanStatus),
		ScanPolicyMode:     policy.FailMode,
		ScanPolicyReason:   policy.Reason,
		ScanPolicyOverride: policy.Override,
	})
	if err != nil {
		return err
	}
	_, err = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  req.ThreadID,
		RunID:     artifact.RunID,
		EventType: artifactContentAccessedEvent,
		Payload:   string(payload),
	})
	if err != nil {
		return fmt.Errorf("audit artifact content access: %w", err)
	}
	return nil
}

func (s *ApplicationService) auditArtifactContentBlocked(
	ctx context.Context,
	req *ReadArtifactContentRequest,
	artifact *domainentity.AgentArtifact,
	policy artifactScanReadPolicyDecision,
) error {
	if s == nil || s.ThreadSVC == nil || req == nil || artifact == nil || artifact.RunID <= 0 {
		return nil
	}
	payload, err := json.Marshal(struct {
		Schema       string `json:"schema"`
		ThreadID     int64  `json:"thread_id"`
		RunID        int64  `json:"run_id"`
		ArtifactID   int64  `json:"artifact_id"`
		FileID       int64  `json:"file_id"`
		Mode         string `json:"mode"`
		PreviewMode  string `json:"preview_mode"`
		ArtifactType string `json:"artifact_type"`
		ScanStatus   string `json:"scan_status"`
		Reason       string `json:"reason"`
	}{
		Schema:       "coze.artifact_access_blocked.v1",
		ThreadID:     req.ThreadID,
		RunID:        artifact.RunID,
		ArtifactID:   artifact.ID,
		FileID:       artifact.FileID,
		Mode:         string(normalizeArtifactContentMode(req.Mode)),
		PreviewMode:  string(artifact.PreviewMode),
		ArtifactType: artifact.ArtifactType,
		ScanStatus:   string(normalizeArtifactScanStatus(artifact.Metadata)),
		Reason:       policy.Reason,
	})
	if err != nil {
		return err
	}
	_, err = s.ThreadSVC.AppendRunEvent(ctx, &domainservice.AppendRunEventRequest{
		ThreadID:  req.ThreadID,
		RunID:     artifact.RunID,
		EventType: artifactContentBlockedEvent,
		Payload:   string(payload),
	})
	if err != nil {
		return fmt.Errorf("audit artifact content blocked: %w", err)
	}
	return nil
}

func normalizeArtifactContentMode(mode ArtifactContentMode) ArtifactContentMode {
	if mode == ArtifactContentModeDownload {
		return ArtifactContentModeDownload
	}
	return ArtifactContentModePreview
}

func shouldAttachArtifactContent(
	mode ArtifactContentMode,
	artifact *domainentity.AgentArtifact,
	effectiveContentType string,
) bool {
	if mode == ArtifactContentModeDownload || artifact == nil {
		return true
	}
	if artifact.PreviewMode == domainentity.AgentArtifactPreviewModeDownload ||
		artifact.PreviewMode == domainentity.AgentArtifactPreviewModeUnsupported {
		return true
	}
	if domainservice.DetermineArtifactPreviewMode(
		artifact.ContentType,
	) == domainentity.AgentArtifactPreviewModeDownload {
		return true
	}
	return domainservice.DetermineArtifactPreviewMode(
		effectiveContentType,
	) == domainentity.AgentArtifactPreviewModeDownload
}

func normalizeArtifactSignedURLTTL(ttlSeconds int64) int64 {
	const (
		defaultTTLSeconds = int64(300)
		minTTLSeconds     = int64(60)
		maxTTLSeconds     = int64(3600)
	)
	if ttlSeconds <= 0 {
		return defaultTTLSeconds
	}
	if ttlSeconds < minTTLSeconds {
		return minTTLSeconds
	}
	if ttlSeconds > maxTTLSeconds {
		return maxTTLSeconds
	}
	return ttlSeconds
}

func canCreateArtifactSignedPreviewURL(
	artifact *domainentity.AgentArtifact,
	effectiveContentType string,
) bool {
	if artifact == nil {
		return false
	}
	storedMode := domainservice.DetermineArtifactPreviewMode(artifact.ContentType)
	effectiveMode := domainservice.DetermineArtifactPreviewMode(effectiveContentType)
	if storedMode != effectiveMode || storedMode == domainentity.AgentArtifactPreviewModeDownload {
		return false
	}
	return artifact.PreviewMode == storedMode
}

func artifactContentDisposition(fileName string, attachment bool) string {
	disposition := "inline"
	if attachment {
		disposition = "attachment"
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "artifact"
	}
	return disposition + "; filename*=UTF-8''" + url.PathEscape(fileName)
}

func effectiveArtifactContentType(storedContentType string, content []byte) string {
	if len(content) > 0 {
		return http.DetectContentType(content)
	}
	contentType := strings.TrimSpace(storedContentType)
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func artifactContentFileName(artifact *domainentity.AgentArtifact) string {
	if artifact == nil {
		return "artifact"
	}
	if title := strings.TrimSpace(artifact.Title); title != "" {
		return title
	}
	if base := strings.TrimSpace(path.Base(artifact.VirtualPath)); base != "" &&
		base != "." && base != "/" {
		return base
	}
	return "artifact"
}

func tokenUsageResponse(
	rows []*domainentity.TokenUsage,
	total int64,
	aggregate *domainentity.TokenUsageAggregate,
	runAggregates []*domainentity.RunTokenUsageAggregate,
) *GetTokenUsageResponse {
	resp := &GetTokenUsageResponse{
		Usage:         make([]*TokenUsageSummary, 0, len(rows)),
		Total:         total,
		Aggregate:     DomainTokenUsageAggregateToSummary(aggregate),
		RunAggregates: make([]*RunTokenUsageAggregateSummary, 0, len(runAggregates)),
	}
	for _, usage := range rows {
		resp.Usage = append(resp.Usage, DomainTokenUsageToSummary(usage))
	}
	for _, runAggregate := range runAggregates {
		resp.RunAggregates = append(resp.RunAggregates, DomainRunTokenUsageAggregateToSummary(runAggregate))
	}

	return resp
}

func (s *ApplicationService) ClaimPendingRuns(ctx context.Context, req *ClaimPendingRunsRequest) (*ClaimPendingRunsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("claim pending runs request is required")
	}

	runs, err := s.ThreadSVC.ClaimPendingRuns(ctx, &domainservice.ClaimPendingRunsRequest{
		WorkerID:       req.WorkerID,
		Limit:          req.Limit,
		Now:            req.Now,
		LeaseTTLMillis: req.LeaseTTLMillis,
	})
	if err != nil {
		return nil, err
	}

	resp := &ClaimPendingRunsResponse{
		Runs: make([]*RunSummary, 0, len(runs)),
	}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, DomainRunToSummary(run))
	}

	return resp, nil
}

func (s *ApplicationService) ClaimQueuedResumeRuns(ctx context.Context, req *ClaimQueuedResumeRunsRequest) (*ClaimQueuedResumeRunsResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("claim queued resume runs request is required")
	}

	runs, err := s.ThreadSVC.ClaimQueuedResumeRuns(ctx, &domainservice.ClaimQueuedResumeRunsRequest{
		WorkerID:       req.WorkerID,
		Limit:          req.Limit,
		Now:            req.Now,
		LeaseTTLMillis: req.LeaseTTLMillis,
	})
	if err != nil {
		return nil, err
	}

	resp := &ClaimQueuedResumeRunsResponse{
		Runs: make([]*RunSummary, 0, len(runs)),
	}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, DomainRunToSummary(run))
	}

	return resp, nil
}

func (s *ApplicationService) RenewRunLease(ctx context.Context, req *RenewRunLeaseRequest) (*RenewRunLeaseResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("renew run lease request is required")
	}

	run, err := s.ThreadSVC.RenewRunLease(ctx, &domainservice.RenewRunLeaseRequest{
		RunID:               req.RunID,
		LeaseOwner:          req.LeaseOwner,
		LeaseToken:          req.LeaseToken,
		ExecutionGeneration: req.ExecutionGeneration,
		Now:                 req.Now,
		LeaseTTLMillis:      req.LeaseTTLMillis,
	})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty renewed run")
	}
	return &RenewRunLeaseResponse{Run: DomainRunToSummary(run)}, nil
}

func (s *ApplicationService) ReleaseRunLease(ctx context.Context, req *ReleaseRunLeaseRequest) (*ReleaseRunLeaseResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("release run lease request is required")
	}

	run, err := s.ThreadSVC.ReleaseRunLease(ctx, &domainservice.ReleaseRunLeaseRequest{
		RunID:               req.RunID,
		LeaseOwner:          req.LeaseOwner,
		LeaseToken:          req.LeaseToken,
		ExecutionGeneration: req.ExecutionGeneration,
		ToStatus:            domainentity.RunStatus(req.ToStatus),
		Now:                 req.Now,
	})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty released run")
	}
	return &ReleaseRunLeaseResponse{Run: DomainRunToSummary(run)}, nil
}

func (s *ApplicationService) ListExpiredRunLeases(
	ctx context.Context,
	req *ListExpiredRunLeasesRequest,
) (*ListExpiredRunLeasesResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("list expired run leases request is required")
	}

	runs, err := s.ThreadSVC.ListExpiredRunLeases(ctx, &domainservice.ListExpiredRunLeasesRequest{
		Now:   req.Now,
		Limit: req.Limit,
	})
	if err != nil {
		return nil, err
	}
	resp := &ListExpiredRunLeasesResponse{Runs: make([]*RunSummary, 0, len(runs))}
	for _, run := range runs {
		resp.Runs = append(resp.Runs, DomainRunToSummary(run))
	}
	return resp, nil
}

func (s *ApplicationService) ReconcileExpiredRunLease(
	ctx context.Context,
	req *ReconcileExpiredRunLeaseRequest,
) (*ReconcileExpiredRunLeaseResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("reconcile expired run lease request is required")
	}

	run, err := s.ThreadSVC.ReconcileExpiredRunLease(ctx, &domainservice.ReconcileExpiredRunLeaseRequest{
		RunID:               req.RunID,
		LeaseOwner:          req.LeaseOwner,
		LeaseToken:          req.LeaseToken,
		ExecutionGeneration: req.ExecutionGeneration,
		ToStatus:            domainentity.RunStatus(req.ToStatus),
		Now:                 req.Now,
		ErrorCode:           req.ErrorCode,
		ErrorMessage:        req.ErrorMessage,
	})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty reconciled run")
	}

	return &ReconcileExpiredRunLeaseResponse{Run: DomainRunToSummary(run)}, nil
}

func (s *ApplicationService) FinalizeRunSuccess(
	ctx context.Context,
	req *FinalizeRunSuccessRequest,
) (*FinalizeRunSuccessResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("finalize run success request is required")
	}

	result, err := s.ThreadSVC.FinalizeRunSuccess(ctx, &domainservice.FinalizeRunSuccessRequest{
		RunID:               req.RunID,
		ThreadID:            req.ThreadID,
		LeaseOwner:          req.LeaseOwner,
		LeaseToken:          req.LeaseToken,
		ExecutionGeneration: req.ExecutionGeneration,
		Now:                 req.Now,
		Message:             req.Message,
		MessageMetadata:     req.MessageMetadata,
		ExpectedThreadTitle: req.ExpectedThreadTitle,
		ThreadTitle:         req.ThreadTitle,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Run == nil || result.Message == nil {
		return nil, fmt.Errorf("agent thread service returned empty finalized run")
	}

	return &FinalizeRunSuccessResponse{
		Run:          DomainRunToSummary(result.Run),
		Message:      DomainMessageToSummary(result.Message),
		TitleUpdated: result.TitleUpdated,
	}, nil
}

func (s *ApplicationService) CompleteRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}

	return s.updateRunStatus(ctx, req, s.ThreadSVC.CompleteRun)
}

func (s *ApplicationService) InterruptRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}

	return s.updateRunStatus(ctx, req, s.ThreadSVC.InterruptRun)
}

func (s *ApplicationService) FailRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}

	return s.updateRunStatus(ctx, req, s.ThreadSVC.FailRun)
}

func (s *ApplicationService) CancelRun(ctx context.Context, req *UpdateRunStatusRequest) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("update run status request is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		RunID: req.RunID,
	}); err != nil {
		return nil, err
	}

	result, err := s.requestRunCancellation(ctx, &domainservice.RequestRunCancellationRequest{
		RunID:        req.RunID,
		Now:          req.Now,
		ErrorCode:    req.ErrorCode,
		ErrorMessage: req.ErrorMessage,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.Run == nil {
		return nil, fmt.Errorf("agent thread service returned empty canceled run")
	}
	resp := &UpdateRunStatusResponse{Run: DomainRunToSummary(result.Run)}
	s.cancelActiveADKRun(ctx, result)

	return resp, nil
}

func (s *ApplicationService) requestRunCancellation(
	ctx context.Context,
	req *domainservice.RequestRunCancellationRequest,
) (*domainservice.RequestRunCancellationResult, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("request run cancellation request is required")
	}

	return s.ThreadSVC.RequestRunCancellation(ctx, req)
}

func (s *ApplicationService) cancelActiveADKRun(
	ctx context.Context,
	result *domainservice.RequestRunCancellationResult,
) {
	if s == nil || s.ADKCancelRegistry == nil || result == nil || result.Run == nil ||
		!result.Changed || result.PreviousStatus != domainentity.RunStatusRunning {
		return
	}
	run := DomainRunToSummary(result.Run)
	mode, err := runtimeModeFromRun(run)
	if err != nil || mode != RuntimeModeEinoADK {
		return
	}

	_ = s.ADKCancelRegistry.Request(
		ctx,
		run.RunID,
		adk.CancelAfterToolCalls|adk.CancelAfterChatModel,
		true,
	)
}

func (s *ApplicationService) updateRunStatus(
	ctx context.Context,
	req *UpdateRunStatusRequest,
	update func(context.Context, *domainservice.UpdateRunStatusRequest) (*domainentity.Run, error),
) (*UpdateRunStatusResponse, error) {
	if err := s.requireThreadSVC(); err != nil {
		return nil, err
	}
	if req == nil {
		return nil, fmt.Errorf("update run status request is required")
	}
	if update == nil {
		return nil, fmt.Errorf("update run status handler is required")
	}
	if err := s.authorizeThreadAccessFromContext(ctx, ThreadAccessRequest{
		RunID: req.RunID,
	}); err != nil {
		return nil, err
	}

	run, err := update(ctx, &domainservice.UpdateRunStatusRequest{
		RunID:               req.RunID,
		From:                domainentity.RunStatus(req.From),
		To:                  domainentity.RunStatus(req.To),
		WorkerID:            req.WorkerID,
		LeaseOwner:          req.LeaseOwner,
		LeaseToken:          req.LeaseToken,
		ExecutionGeneration: req.ExecutionGeneration,
		Now:                 req.Now,
		ErrorCode:           req.ErrorCode,
		ErrorMessage:        req.ErrorMessage,
	})
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("agent thread service returned empty run")
	}

	return &UpdateRunStatusResponse{Run: DomainRunToSummary(run)}, nil
}

func (s *ApplicationService) requireThreadSVC() error {
	if s == nil || s.ThreadSVC == nil {
		return fmt.Errorf("agent thread service is not initialized")
	}

	return nil
}

func (s *ApplicationService) requireArtifactSVC() error {
	if s == nil || s.ArtifactSVC == nil {
		return fmt.Errorf("agent artifact service is not initialized")
	}

	return nil
}

func (s *ApplicationService) requireGuardrailAuditRepository() error {
	if s == nil || s.GuardrailAuditRepository == nil {
		return fmt.Errorf("guardrail audit repository is not initialized")
	}

	return nil
}

func (s *ApplicationService) requireMCPRuntimeAuditRepository() error {
	if s == nil || s.MCPRuntimeAuditRepository == nil {
		return fmt.Errorf("mcp runtime audit repository is not initialized")
	}

	return nil
}

func normalizeGuardrailAuditPage(page, pageSize int32) (int32, int32) {
	_, normalizedPageSize, offset := normalizeGuardrailAuditPageWithMax(
		page,
		pageSize,
		20,
		100,
	)

	return normalizedPageSize, offset
}

func normalizeGuardrailAuditExportPage(page, pageSize int32) (int32, int32, int32) {
	return normalizeGuardrailAuditPageWithMax(page, pageSize, 100, 1000)
}

func normalizeGuardrailAuditPageWithMax(
	page,
	pageSize,
	defaultPageSize,
	maxPageSize int32,
) (int32, int32, int32) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	return page, pageSize, (page - 1) * pageSize
}
