// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
	"github.com/coze-dev/coze-studio/backend/infra/idgen"
)

const (
	maxInboundPromptBytes = 24 * 1024
	maxOutboundReplyBytes = 64 * 1024
)

type AgentThreadClient interface {
	CreateTaskThread(context.Context, *agentthread.CreateTaskThreadRequest) (*agentthread.CreateTaskThreadResponse, error)
	CreateRun(context.Context, *agentthread.CreateRunRequest) (*agentthread.CreateRunResponse, error)
	GetRun(context.Context, *agentthread.GetRunRequest) (*agentthread.GetRunResponse, error)
	GetThread(context.Context, *agentthread.GetThreadRequest) (*agentthread.GetThreadResponse, error)
	ListMessages(context.Context, *agentthread.ListMessagesRequest) (*agentthread.ListMessagesResponse, error)
}

type AgentRunner struct {
	client       AgentThreadClient
	repository   domain.Repository
	idGen        idgen.IDGenerator
	pollInterval time.Duration
	runTimeout   time.Duration
}

func NewAgentRunner(
	client AgentThreadClient,
	repository domain.Repository,
	idGenerator idgen.IDGenerator,
) *AgentRunner {
	return &AgentRunner{
		client:       client,
		repository:   repository,
		idGen:        idGenerator,
		pollInterval: time.Second,
		runTimeout:   5 * time.Minute,
	}
}

func (r *AgentRunner) Execute(
	ctx context.Context,
	config *domain.Config,
	eventKey string,
	payload domain.InboundPayload,
) (string, error) {
	if r == nil || r.client == nil || r.repository == nil || r.idGen == nil || config == nil {
		return "", domain.ErrRuntimeUnavailable
	}
	prompt := inboundPrompt(payload)
	if prompt == "" {
		return "", domain.ErrInvalidInput
	}
	if len(prompt) > maxInboundPromptBytes {
		prompt = prompt[:maxInboundPromptBytes]
	}

	session, err := r.repository.GetSession(ctx, config.ID, payload.ChatID)
	if err != nil {
		return "", err
	}
	runID, threadID, err := r.startRun(ctx, config, session, eventKey, prompt, payload)
	if err != nil {
		return "", err
	}
	if session == nil {
		sessionID, idErr := r.idGen.GenID(ctx)
		if idErr != nil {
			return "", idErr
		}
		session = &domain.Session{
			ID:             sessionID,
			ConfigID:       config.ID,
			SpaceID:        config.SpaceID,
			ChatID:         payload.ChatID,
			ChatType:       payload.ChatType,
			ExternalUserID: payload.UserID,
			ThreadID:       threadID,
			LastMessageID:  payload.MessageID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
	} else {
		session.ChatType = payload.ChatType
		session.ExternalUserID = payload.UserID
		session.ThreadID = threadID
		session.LastMessageID = payload.MessageID
		session.UpdatedAt = time.Now()
	}
	if err := r.repository.SaveSession(ctx, session); err != nil {
		return "", err
	}

	runCtx, cancel := context.WithTimeout(ctx, r.runTimeout)
	defer cancel()
	if err := r.waitRun(runCtx, runID); err != nil {
		return "", err
	}
	answer, err := r.readAssistantAnswer(runCtx, config.CreatorID, threadID, runID)
	if err != nil {
		return "", err
	}
	if len(answer) > maxOutboundReplyBytes {
		answer = answer[:maxOutboundReplyBytes]
	}
	return answer, nil
}

func (r *AgentRunner) startRun(
	ctx context.Context,
	config *domain.Config,
	session *domain.Session,
	eventKey, prompt string,
	payload domain.InboundPayload,
) (int64, int64, error) {
	metadata := runtimeMetadata(config, payload)
	idempotencyKey := fmt.Sprintf("feishu-im:%d:%s", config.ID, eventKey)
	assistantID := strconv.FormatInt(config.AgentID, 10)
	if session == nil || session.ThreadID <= 0 {
		response, err := r.client.CreateTaskThread(ctx, &agentthread.CreateTaskThreadRequest{
			SpaceID:        config.SpaceID,
			UserID:         config.CreatorID,
			Message:        prompt,
			Title:          taskTitle(prompt),
			AssistantID:    assistantID,
			Metadata:       metadata,
			Context:        metadata,
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			return 0, 0, err
		}
		if response == nil || response.Thread == nil || response.Run == nil {
			return 0, 0, domain.ErrAgentExecutionFailed
		}
		return response.Run.RunID, response.Thread.ThreadID, nil
	}

	response, err := r.client.CreateRun(ctx, &agentthread.CreateRunRequest{
		ThreadID:        session.ThreadID,
		AssistantID:     assistantID,
		RunKind:         agentthread.RunKindTask,
		Input:           "{}",
		Metadata:        metadata,
		Context:         metadata,
		IdempotencyKey:  idempotencyKey,
		MessageContent:  prompt,
		MessageMetadata: metadata,
	})
	if err != nil {
		return 0, 0, err
	}
	if response == nil || response.Run == nil {
		return 0, 0, domain.ErrAgentExecutionFailed
	}
	return response.Run.RunID, session.ThreadID, nil
}

func (r *AgentRunner) waitRun(ctx context.Context, runID int64) error {
	interval := r.pollInterval
	if interval <= 0 {
		interval = time.Second
	}
	for {
		response, err := r.client.GetRun(ctx, &agentthread.GetRunRequest{RunID: runID})
		if err != nil {
			return err
		}
		if response == nil || response.Run == nil {
			return domain.ErrAgentExecutionFailed
		}
		switch response.Run.Status {
		case agentthread.RunStatusSucceeded:
			return nil
		case agentthread.RunStatusFailed, agentthread.RunStatusInterrupted, agentthread.RunStatusCanceled:
			return fmt.Errorf("%w: %s", domain.ErrAgentExecutionFailed, boundedMessage(response.Run.ErrorMessage, 256))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func (r *AgentRunner) readAssistantAnswer(
	ctx context.Context,
	userID, threadID, runID int64,
) (string, error) {
	response, err := r.client.ListMessages(ctx, &agentthread.ListMessagesRequest{
		ThreadID: threadID,
		Page:     1,
		PageSize: 100,
	})
	if err != nil {
		return "", err
	}
	var answer string
	var latestMessageID int64
	if response != nil {
		for _, message := range response.Messages {
			if message == nil || message.Role != agentthread.MessageRoleAssistant || message.RunID != runID {
				continue
			}
			if message.MessageID >= latestMessageID && strings.TrimSpace(message.Content) != "" {
				latestMessageID = message.MessageID
				answer = strings.TrimSpace(message.Content)
			}
		}
	}
	if answer != "" {
		return answer, nil
	}
	thread, err := r.client.GetThread(ctx, &agentthread.GetThreadRequest{ThreadID: threadID})
	if err != nil {
		return "", err
	}
	if thread != nil && thread.Thread != nil && strings.TrimSpace(thread.Thread.LastAgentMessage) != "" {
		return strings.TrimSpace(thread.Thread.LastAgentMessage), nil
	}
	_ = userID
	return "", domain.ErrAgentResponseUnavailable
}

func inboundPrompt(payload domain.InboundPayload) string {
	parts := make([]string, 0, 1+len(payload.Resources))
	if content := strings.TrimSpace(payload.Content); content != "" {
		parts = append(parts, content)
	}
	for _, resource := range payload.Resources {
		name := strings.TrimSpace(resource.FileName)
		if name == "" {
			name = "未命名附件"
		}
		resourceType := strings.TrimSpace(resource.Type)
		if resourceType == "" {
			resourceType = "file"
		}
		parts = append(parts, fmt.Sprintf("[飞书附件：%s，类型：%s]", name, resourceType))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func runtimeMetadata(config *domain.Config, payload domain.InboundPayload) string {
	value, err := json.Marshal(map[string]any{
		"source": "feishu_im",
		"feishu": map[string]string{
			"config_id":  strconv.FormatInt(config.ID, 10),
			"chat_id":    payload.ChatID,
			"chat_type":  payload.ChatType,
			"user_id":    payload.UserID,
			"message_id": payload.MessageID,
		},
	})
	if err != nil {
		return `{"source":"feishu_im"}`
	}
	return string(value)
}

func taskTitle(prompt string) string {
	const prefix = "飞书 · "
	runes := []rune(strings.TrimSpace(prompt))
	if len(runes) > 28 {
		runes = runes[:28]
	}
	if len(runes) == 0 {
		return "飞书会话"
	}
	return prefix + string(runes)
}
