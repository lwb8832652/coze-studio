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
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKProviderCapabilityMiddlewareRejectsUnsupportedImage(t *testing.T) {
	middleware, err := NewADKProviderCapabilityMiddleware(
		&RunSummary{RunID: 20},
		ADKModelCapabilities{},
	)
	require.NoError(t, err)
	imageURL := "https://example.com/private/image.png"
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			{
				Role: schema.User,
				UserInputMultiContent: []schema.MessageInputPart{
					{
						Type: schema.ChatMessagePartTypeImageURL,
						Image: &schema.MessageInputImage{
							MessagePartCommon: schema.MessagePartCommon{
								URL:      &imageURL,
								MIMEType: "image/png",
							},
						},
					},
				},
			},
		},
	}

	_, got, err := middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	var capabilityErr *ADKProviderCapabilityError
	require.ErrorAs(t, err, &capabilityErr)
	require.Equal(t, ADKProviderCapabilityVision, capabilityErr.Capability)
	require.Equal(t, string(schema.ChatMessagePartTypeImageURL), capabilityErr.PartType)
	require.Equal(t, 1, capabilityErr.Count)
	require.NotContains(t, err.Error(), imageURL)
	require.Same(t, state, got)
}

func TestADKProviderCapabilityMiddlewareAllowsDeclaredImage(t *testing.T) {
	middleware, err := NewADKProviderCapabilityMiddleware(
		&RunSummary{RunID: 20},
		ADKModelCapabilities{Vision: true},
	)
	require.NoError(t, err)
	imageURL := "https://example.com/private/image.png"
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			{
				Role: schema.User,
				MultiContent: []schema.ChatMessagePart{
					{
						Type: schema.ChatMessagePartTypeImageURL,
						ImageURL: &schema.ChatMessageImageURL{
							URL:      imageURL,
							MIMEType: "image/png",
						},
					},
				},
			},
		},
	}

	_, got, err := middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Same(t, state, got)
}

func TestADKProviderCapabilityMiddlewareRejectsUnsupportedPDF(t *testing.T) {
	middleware, err := NewADKProviderCapabilityMiddleware(
		&RunSummary{RunID: 20},
		ADKModelCapabilities{File: true},
	)
	require.NoError(t, err)
	fileURL := "https://example.com/private/spec.pdf"
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			{
				Role: schema.User,
				UserInputMultiContent: []schema.MessageInputPart{
					{
						Type: schema.ChatMessagePartTypeFileURL,
						File: &schema.MessageInputFile{
							MessagePartCommon: schema.MessagePartCommon{
								URL:      &fileURL,
								MIMEType: "application/pdf",
							},
							Name: "confidential.pdf",
						},
					},
				},
			},
		},
	}

	_, _, err = middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	var capabilityErr *ADKProviderCapabilityError
	require.ErrorAs(t, err, &capabilityErr)
	require.Equal(t, ADKProviderCapabilityPDF, capabilityErr.Capability)
	require.Equal(t, string(schema.ChatMessagePartTypeFileURL), capabilityErr.PartType)
	require.NotContains(t, err.Error(), fileURL)
	require.NotContains(t, err.Error(), "confidential.pdf")
}

func TestADKProviderCapabilityMiddlewareRejectsUnsupportedReasoningRequest(t *testing.T) {
	middleware, err := NewADKProviderCapabilityMiddleware(
		&RunSummary{
			RunID: 20,
			Config: `{
				"reasoning_effort":"high",
				"thinking_enabled":true
			}`,
		},
		ADKModelCapabilities{},
	)
	require.NoError(t, err)

	_, _, err = middleware.BeforeModelRewriteState(
		context.Background(),
		&adk.ChatModelAgentState{},
		&adk.ModelContext{},
	)

	var capabilityErr *ADKProviderCapabilityError
	require.ErrorAs(t, err, &capabilityErr)
	require.Equal(t, ADKProviderCapabilityReasoning, capabilityErr.Capability)
	require.Equal(t, "reasoning_effort", capabilityErr.PartType)
}

func TestADKProviderCapabilityConfigParsesRunOverrides(t *testing.T) {
	config, err := adkProviderCapabilityConfigFromRun(
		&RunSummary{
			RunID: 20,
			Config: `{
				"provider_capabilities":{
					"vision":true,
					"pdf":true,
					"audio":true,
					"video":true,
					"file":true
				},
				"reasoningEffort":"medium",
				"thinkingEnabled":true
			}`,
		},
		ADKModelCapabilities{Reasoning: true, Thinking: true},
	)

	require.NoError(t, err)
	require.True(t, config.Capabilities.Vision)
	require.True(t, config.Capabilities.PDF)
	require.True(t, config.Capabilities.Audio)
	require.True(t, config.Capabilities.Video)
	require.True(t, config.Capabilities.File)
	require.True(t, config.Capabilities.Reasoning)
	require.True(t, config.Capabilities.Thinking)
	require.Equal(t, "medium", config.Reasoning.ReasoningEffort)
	require.True(t, config.Reasoning.ThinkingEnabled)
}
