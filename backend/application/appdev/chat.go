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

package appdev

import (
	"context"
	"fmt"
	"strings"
	"time"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type ChatManager interface {
	Send(ctx context.Context, req *ChatManagerRequest) (*ChatSession, error)
	Subscribe(ctx context.Context, req *ChatManagerRequest) (<-chan *ChatEvent, func(), error)
	Cancel(ctx context.Context, req *ChatManagerRequest) error
	History(ctx context.Context, req *ChatManagerRequest) ([]*ChatMessage, error)
	Status(ctx context.Context, req *ChatManagerRequest) (*ChatStatus, error)
}

type ChatManagerRequest struct {
	SpaceID           string
	ProjectID         string
	UserID            int64
	Message           string
	ModelID           string
	ProjectDir        string
	DataSources       []ChatDataSource
	Attachments       []ChatAttachment
	SaveGeneratedFile func(ctx context.Context, path string, content string) error
}

type ChatDataSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type ChatAttachment struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	MIMEType string `json:"mimeType,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Type     string `json:"type"`
}

type ChatEvent struct {
	Event string         `json:"event"`
	Data  map[string]any `json:"data"`
}

type ChatSession struct {
	SessionID string `json:"sessionId"`
	RequestID string `json:"requestId"`
	Running   bool   `json:"running"`
}

type ChatMessage struct {
	ID          string           `json:"id"`
	Type        string           `json:"type"`
	Role        string           `json:"role,omitempty"`
	Title       string           `json:"title,omitempty"`
	Content     string           `json:"content"`
	Attachments []ChatAttachment `json:"attachments,omitempty"`
	CreatedAt   time.Time        `json:"createdAt"`
}

type ChatStatus struct {
	Running   bool   `json:"running"`
	SessionID string `json:"sessionId,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

type SendChatRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
	Message       string
	ModelID       string
	DataSources   []ChatDataSource
	Attachments   []ChatAttachment
}

type ChatStreamRequest struct {
	SpaceID       string
	CurrentUserID int64
	ProjectID     string
}

type ChatSessionResponse struct {
	Code    int64        `json:"code"`
	Message string       `json:"message"`
	Data    *ChatSession `json:"data"`
}

type ChatHistoryData struct {
	Items []*ChatMessageDTO `json:"items"`
}

type ChatHistoryResponse struct {
	Code    int64           `json:"code"`
	Message string          `json:"message"`
	Data    ChatHistoryData `json:"data"`
}

type ChatStatusResponse struct {
	Code    int64       `json:"code"`
	Message string      `json:"message"`
	Data    *ChatStatus `json:"data"`
}

type ChatMessageDTO struct {
	ID          string           `json:"id"`
	Type        string           `json:"type"`
	Role        string           `json:"role,omitempty"`
	Title       string           `json:"title,omitempty"`
	Content     string           `json:"content"`
	Attachments []ChatAttachment `json:"attachments,omitempty"`
	CreatedAt   string           `json:"createdAt,omitempty"`
}

func (s *Service) SendChatMessage(ctx context.Context, req *SendChatRequest) (*ChatSessionResponse, error) {
	managerReq, err := s.chatRequest(ctx, &ChatStreamRequest{
		SpaceID:       req.SpaceID,
		CurrentUserID: req.CurrentUserID,
		ProjectID:     req.ProjectID,
	})
	if err != nil {
		return nil, err
	}
	managerReq.SaveGeneratedFile = func(saveCtx context.Context, path string, content string) error {
		_, saveErr := s.store.SaveFileContent(
			saveCtx,
			strings.TrimSpace(req.SpaceID),
			strings.TrimSpace(req.ProjectID),
			path,
			content,
		)
		return saveErr
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, fmt.Errorf("message cannot be empty")
	}
	if len(message) > 12000 {
		return nil, fmt.Errorf("message cannot exceed 12000 characters")
	}
	modelID := strings.TrimSpace(req.ModelID)
	if modelID == "" {
		return nil, fmt.Errorf("model_id is required")
	}
	if err := s.validateChatModel(ctx, req.SpaceID, req.CurrentUserID, modelID); err != nil {
		return nil, err
	}

	managerReq.Message = message
	managerReq.ModelID = modelID
	managerReq.DataSources = sanitizeChatDataSources(req.DataSources)
	managerReq.Attachments = sanitizeChatAttachments(req.Attachments)

	session, err := s.chat.Send(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	return &ChatSessionResponse{
		Code:    0,
		Message: "success",
		Data:    session,
	}, nil
}

func sanitizeChatDataSources(dataSources []ChatDataSource) []ChatDataSource {
	const maxDataSources = 8

	capacity := len(dataSources)
	if capacity > maxDataSources {
		capacity = maxDataSources
	}

	sanitized := make([]ChatDataSource, 0, capacity)
	for _, dataSource := range dataSources {
		name := strings.TrimSpace(dataSource.Name)
		if name == "" {
			continue
		}
		if len([]rune(name)) > 80 {
			name = string([]rune(name)[:80])
		}

		id := strings.TrimSpace(dataSource.ID)
		if len([]rune(id)) > 64 {
			id = string([]rune(id)[:64])
		}
		sourceType := strings.TrimSpace(dataSource.Type)
		if len([]rune(sourceType)) > 32 {
			sourceType = string([]rune(sourceType)[:32])
		}

		sanitized = append(sanitized, ChatDataSource{
			ID:   id,
			Name: name,
			Type: sourceType,
		})
		if len(sanitized) >= maxDataSources {
			break
		}
	}

	return sanitized
}

func sanitizeChatAttachments(attachments []ChatAttachment) []ChatAttachment {
	const maxAttachments = 8

	capacity := len(attachments)
	if capacity > maxAttachments {
		capacity = maxAttachments
	}

	sanitized := make([]ChatAttachment, 0, capacity)
	for _, attachment := range attachments {
		filePath, err := domainappdev.NormalizeRelativePath(attachment.Path)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(filePath, "src/assets/uploads/") {
			continue
		}
		name := strings.TrimSpace(attachment.Name)
		if name == "" {
			parts := strings.Split(filePath, "/")
			name = parts[len(parts)-1]
		}
		if len([]rune(name)) > 120 {
			name = string([]rune(name)[:120])
		}
		mimeType := strings.TrimSpace(attachment.MIMEType)
		if len([]rune(mimeType)) > 120 {
			mimeType = string([]rune(mimeType)[:120])
		}
		attachmentType := strings.TrimSpace(attachment.Type)
		if attachmentType == "prototype_image" && strings.HasPrefix(mimeType, "image/") {
			attachmentType = "prototype_image"
		} else if attachmentType != "image" {
			attachmentType = "file"
		}

		sanitized = append(sanitized, ChatAttachment{
			ID:       strings.TrimSpace(attachment.ID),
			Name:     name,
			Path:     filePath,
			MIMEType: mimeType,
			Size:     attachment.Size,
			Type:     attachmentType,
		})
		if len(sanitized) >= maxAttachments {
			break
		}
	}

	return sanitized
}

func (s *Service) validateChatModel(ctx context.Context, spaceID string, currentUserID int64, modelID string) error {
	models, err := s.ListModels(ctx, &ListModelsRequest{
		SpaceID:       spaceID,
		CurrentUserID: currentUserID,
		Scenario:      "PageApp",
	})
	if err != nil {
		return err
	}

	for _, model := range models.Data.Items {
		if model.ID == modelID {
			return nil
		}
	}

	return fmt.Errorf("model_id is not available")
}

func (s *Service) SubscribeChatEvents(ctx context.Context, req *ChatStreamRequest) (<-chan *ChatEvent, func(), error) {
	managerReq, err := s.chatRequest(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	return s.chat.Subscribe(ctx, managerReq)
}

func (s *Service) CancelChat(ctx context.Context, req *ChatStreamRequest) (*ChatStatusResponse, error) {
	managerReq, err := s.chatRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := s.chat.Cancel(ctx, managerReq); err != nil {
		return nil, err
	}

	return &ChatStatusResponse{
		Code:    0,
		Message: "success",
		Data:    &ChatStatus{Running: false},
	}, nil
}

func (s *Service) ListChatHistory(ctx context.Context, req *ChatStreamRequest) (*ChatHistoryResponse, error) {
	managerReq, err := s.chatRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	messages, err := s.chat.History(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	items := make([]*ChatMessageDTO, 0, len(messages))
	for _, message := range messages {
		items = append(items, &ChatMessageDTO{
			ID:          message.ID,
			Type:        message.Type,
			Role:        message.Role,
			Title:       message.Title,
			Content:     message.Content,
			Attachments: message.Attachments,
			CreatedAt:   formatTime(message.CreatedAt),
		})
	}

	return &ChatHistoryResponse{
		Code:    0,
		Message: "success",
		Data:    ChatHistoryData{Items: items},
	}, nil
}

func (s *Service) GetChatStatus(ctx context.Context, req *ChatStreamRequest) (*ChatStatusResponse, error) {
	managerReq, err := s.chatRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	status, err := s.chat.Status(ctx, managerReq)
	if err != nil {
		return nil, err
	}

	return &ChatStatusResponse{
		Code:    0,
		Message: "success",
		Data:    status,
	}, nil
}

func (s *Service) chatRequest(ctx context.Context, req *ChatStreamRequest) (*ChatManagerRequest, error) {
	if s.chat == nil {
		return nil, fmt.Errorf("appdev chat manager is not configured")
	}
	if err := validateProjectFileRequest(req.SpaceID, req.CurrentUserID, req.ProjectID); err != nil {
		return nil, err
	}
	if _, err := s.store.GetProject(ctx, strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID)); err != nil {
		return nil, err
	}
	projectDir, err := s.store.ProjectFilesDir(strings.TrimSpace(req.SpaceID), strings.TrimSpace(req.ProjectID))
	if err != nil {
		return nil, err
	}

	return &ChatManagerRequest{
		SpaceID:    strings.TrimSpace(req.SpaceID),
		ProjectID:  strings.TrimSpace(req.ProjectID),
		UserID:     req.CurrentUserID,
		ProjectDir: projectDir,
	}, nil
}
