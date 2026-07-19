// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package imchannel

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	domain "github.com/coze-dev/coze-studio/backend/domain/imchannel"
)

type recordingAgentThreadClient struct {
	createRunRequest *agentthread.CreateRunRequest
}

func (c *recordingAgentThreadClient) CreateTaskThread(
	context.Context,
	*agentthread.CreateTaskThreadRequest,
) (*agentthread.CreateTaskThreadResponse, error) {
	panic("unexpected CreateTaskThread call")
}

func (c *recordingAgentThreadClient) CreateRun(
	_ context.Context,
	req *agentthread.CreateRunRequest,
) (*agentthread.CreateRunResponse, error) {
	c.createRunRequest = req
	return &agentthread.CreateRunResponse{
		Run: &agentthread.RunSummary{RunID: 101},
	}, nil
}

func (c *recordingAgentThreadClient) GetRun(
	context.Context,
	*agentthread.GetRunRequest,
) (*agentthread.GetRunResponse, error) {
	panic("unexpected GetRun call")
}

func (c *recordingAgentThreadClient) GetThread(
	context.Context,
	*agentthread.GetThreadRequest,
) (*agentthread.GetThreadResponse, error) {
	panic("unexpected GetThread call")
}

func (c *recordingAgentThreadClient) ListMessages(
	context.Context,
	*agentthread.ListMessagesRequest,
) (*agentthread.ListMessagesResponse, error) {
	panic("unexpected ListMessages call")
}

func TestAgentRunnerFollowUpProvidesValidRunInput(t *testing.T) {
	client := &recordingAgentThreadClient{}
	runner := &AgentRunner{client: client}

	runID, threadID, err := runner.startRun(
		context.Background(),
		&domain.Config{ID: 1, SpaceID: 2, AgentID: 3},
		&domain.Session{ThreadID: 4},
		"event-1",
		"在吗2",
		domain.InboundPayload{
			ChatID:    "chat-1",
			ChatType:  "p2p",
			UserID:    "user-1",
			MessageID: "message-1",
		},
	)
	if err != nil {
		t.Fatalf("start follow-up run: %v", err)
	}
	if runID != 101 || threadID != 4 {
		t.Fatalf("unexpected run identifiers: run=%d thread=%d", runID, threadID)
	}
	if client.createRunRequest == nil {
		t.Fatal("follow-up run request was not recorded")
	}
	if !json.Valid([]byte(client.createRunRequest.Input)) {
		t.Fatalf("follow-up run input is not valid JSON: %q", client.createRunRequest.Input)
	}
}
