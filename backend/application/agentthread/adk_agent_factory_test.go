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
	"errors"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKAgentFactoryBuildsRunnableSchemaMessageAgent(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("done", nil),
	}
	var gotModelID int64
	factory := NewApplicationADKAgentFactory(
		func(ctx context.Context, modelID int64) (model.BaseChatModel, bool, error) {
			gotModelID = modelID
			return chatModel, true, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_id":100002,
			"model_name":"doubao-pro",
			"temperature":0.2,
			"max_tokens":512,
			"top_p":0.8,
			"system_prompt":"You are the task lead.",
			"agent_name":"lead",
			"agent_description":"Coordinates the task.",
			"max_iterations":3
		}`,
	})
	require.NoError(t, err)
	require.Equal(t, int64(100002), gotModelID)
	require.Equal(t, "lead", agent.Name(context.Background()))
	require.Equal(t, "Coordinates the task.", agent.Description(context.Background()))

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})
	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Len(t, chatModel.messages, 2)
	require.Equal(t, schema.System, chatModel.messages[0].Role)
	require.Equal(t, "You are the task lead.", chatModel.messages[0].Content)
	require.Equal(t, schema.User, chatModel.messages[1].Role)
	require.Equal(t, "hello", chatModel.messages[1].Content)
	require.NotNil(t, chatModel.options.Model)
	require.Equal(t, "doubao-pro", *chatModel.options.Model)
	require.NotNil(t, chatModel.options.Temperature)
	require.InDelta(t, float32(0.2), *chatModel.options.Temperature, 0.0001)
	require.NotNil(t, chatModel.options.MaxTokens)
	require.Equal(t, 512, *chatModel.options.MaxTokens)
	require.NotNil(t, chatModel.options.TopP)
	require.InDelta(t, float32(0.8), *chatModel.options.TopP, 0.0001)
}

func TestADKAgentFactoryPreservesMaxIterations(t *testing.T) {
	loopTool, err := toolutils.InferTool(
		"loop",
		"Continue the loop.",
		func(context.Context, struct{}) (string, error) {
			return "continue", nil
		},
	)
	require.NoError(t, err)

	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-loop",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "loop",
				Arguments: `{}`,
			},
		}}),
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		ADKToolProviderFunc(func(context.Context, *RunSummary) ([]tool.BaseTool, error) {
			return []tool.BaseTool{loopTool}, nil
		}),
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{"max_iterations":2}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("loop")},
	})

	var terminalErr error
	for _, event := range events {
		if event.Err != nil {
			terminalErr = event.Err
		}
	}
	require.ErrorIs(t, terminalErr, adk.ErrExceedMaxIterations)
	require.Equal(t, 2, chatModel.calls)
}

func TestADKAgentFactoryConfiguresModelRetry(t *testing.T) {
	chatModel := &flakyADKChatModel{
		failuresBeforeSuccess: 1,
		success:               schema.AssistantMessage("done after retry", nil),
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_retry":{
				"max_retries":1,
				"backoff_ms":0
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("retry once")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(t, 2, chatModel.calls)
}

func TestADKAgentFactoryModelRetryExhaustionUsesEinoError(t *testing.T) {
	chatModel := &flakyADKChatModel{
		failuresBeforeSuccess: 3,
		success:               schema.AssistantMessage("unused", nil),
	}
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		nil,
	)
	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"model_retry":{
				"max_retries":1,
				"backoff_ms":0
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("retry exhaust")},
	})

	require.NotEmpty(t, events)
	var exhausted *adk.RetryExhaustedError
	require.ErrorAs(t, events[len(events)-1].Err, &exhausted)
	require.Equal(t, 1, exhausted.TotalRetries)
	require.Equal(t, 2, chatModel.calls)
}

func TestADKAgentFactoryPassesProviderCapabilitiesToMiddleware(t *testing.T) {
	chatModel := &providerCapabilityChatModel{
		recordingChatModel: recordingChatModel{
			resp: schema.AssistantMessage("done", nil),
		},
		capabilities: ADKModelCapabilities{
			Thinking:  true,
			Reasoning: true,
			Vision:    true,
			PDF:       true,
			File:      true,
			Audio:     true,
			Video:     true,
		},
	}
	var got ADKMiddlewareBuildInput
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		nil,
		ADKMiddlewareFactoryFunc(func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			got = input
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(context.Background(), &RunSummary{})

	require.NoError(t, err)
	require.NotNil(t, agent)
	require.True(t, got.ModelCapabilities.Thinking)
	require.True(t, got.ModelCapabilities.Reasoning)
	require.True(t, got.ModelCapabilities.Vision)
	require.True(t, got.ModelCapabilities.PDF)
	require.True(t, got.ModelCapabilities.File)
	require.True(t, got.ModelCapabilities.Audio)
	require.True(t, got.ModelCapabilities.Video)
}

func TestADKAgentFactoryUsesPolicyFilteredToolSetForModelAndMiddleware(t *testing.T) {
	chatModel := &recordingChatModel{
		resp: schema.AssistantMessage("done", nil),
	}
	var got ADKMiddlewareBuildInput
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return chatModel, true, nil
		},
		NewADKToolPolicyProvider(&recordingADKToolSetProvider{
			set: ADKToolSet{
				StaticTools: []tool.BaseTool{
					&namedTestTool{name: "safe_static"},
					&namedTestTool{name: "blocked_static"},
				},
				DynamicTools: []tool.BaseTool{
					&namedTestTool{name: "safe_dynamic"},
					&namedTestTool{name: "blocked_dynamic"},
				},
			},
		}),
		ADKMiddlewareFactoryFunc(func(
			_ context.Context,
			input ADKMiddlewareBuildInput,
		) (ADKMiddlewareBundle, error) {
			got = input
			return ADKMiddlewareBundle{}, nil
		}),
	)

	agent, err := factory.Build(context.Background(), &RunSummary{
		Config: `{
			"tool_policy":{
				"allowed_tools":["safe_static"],
				"allowed_dynamic_tools":["safe_dynamic"]
			}
		}`,
	})
	require.NoError(t, err)

	events := collectADKAgentEvents(t, agent, &adk.AgentInput{
		Messages: []*schema.Message{schema.UserMessage("hello")},
	})

	require.NotEmpty(t, events)
	require.NoError(t, events[len(events)-1].Err)
	require.Equal(
		t,
		[]string{"safe_static"},
		adkToolInfoNames(chatModel.options.Tools),
	)
	require.Equal(
		t,
		[]string{"safe_static"},
		adkToolNames(t, context.Background(), got.StaticTools),
	)
	require.Equal(
		t,
		[]string{"safe_dynamic"},
		adkToolNames(t, context.Background(), got.DynamicTools),
	)
}

func TestDefaultADKToolProviderIncludesHumanInteractionTools(t *testing.T) {
	provider := NewDefaultADKToolProvider()

	tools, err := provider.ResolveTools(context.Background(), &RunSummary{RunID: 1})

	require.NoError(t, err)
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		info, infoErr := item.Info(context.Background())
		require.NoError(t, infoErr)
		names = append(names, info.Name)
	}
	require.ElementsMatch(t, []string{
		adkClarificationToolName,
		adkConfirmationToolName,
	}, names)
}

func adkToolInfoNames(tools []*schema.ToolInfo) []string {
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		if item != nil {
			names = append(names, item.Name)
		}
	}
	return names
}

type providerCapabilityChatModel struct {
	recordingChatModel
	capabilities ADKModelCapabilities
}

func (m *providerCapabilityChatModel) ADKProviderCapabilities() ADKModelCapabilities {
	if m == nil {
		return ADKModelCapabilities{}
	}
	return m.capabilities
}

func TestADKAgentFactoryRejectsUnconfiguredModel(t *testing.T) {
	factory := NewApplicationADKAgentFactory(
		func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return nil, false, nil
		},
		nil,
		nil,
	)

	agent, err := factory.Build(context.Background(), &RunSummary{})

	require.ErrorContains(t, err, "agent thread chat model is not configured")
	require.Nil(t, agent)
}

func collectADKAgentEvents(
	t *testing.T,
	agent adk.ResumableAgent,
	input *adk.AgentInput,
) []*adk.AgentEvent {
	t.Helper()

	iter := agent.Run(context.Background(), input)
	events := make([]*adk.AgentEvent, 0)
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		events = append(events, event)
	}

	return events
}

type flakyADKChatModel struct {
	failuresBeforeSuccess int
	success               *schema.Message
	calls                 int
}

func (m *flakyADKChatModel) Generate(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.Message, error) {
	m.calls++
	if m.calls <= m.failuresBeforeSuccess {
		return nil, fmt.Errorf("temporary provider error %d", m.calls)
	}

	return m.success, nil
}

func (m *flakyADKChatModel) Stream(
	context.Context,
	[]*schema.Message,
	...model.Option,
) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream is not implemented")
}
