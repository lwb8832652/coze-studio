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
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKToolErrorNormalizationInvokableReturnsStableToolError(t *testing.T) {
	middleware := NewADKToolErrorNormalizationMiddleware()
	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			return "", errors.New(" upstream timeout ")
		},
		&adk.ToolContext{Name: "search_docs", CallID: "call-1"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), `{}`)

	require.NoError(t, err)
	decoded, ok := decodeADKToolErrorResult(result)
	require.True(t, ok)
	require.Equal(t, "search_docs", decoded.ToolName)
	require.Equal(t, "call-1", decoded.ToolCallID)
	require.Equal(t, "upstream timeout", decoded.ErrorMessage)
	require.Contains(t, decoded.Message, "Error: Tool 'search_docs' failed")
	require.Contains(t, decoded.Message, "Continue with available context, or choose an alternative tool.")
}

func TestADKToolErrorNormalizationCapsModelVisibleErrorDetail(t *testing.T) {
	middleware := NewADKToolErrorNormalizationMiddleware()
	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			return "", errors.New(strings.Repeat("x", 600))
		},
		&adk.ToolContext{Name: "search_docs", CallID: "call-1"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), `{}`)

	require.NoError(t, err)
	decoded, ok := decodeADKToolErrorResult(result)
	require.True(t, ok)
	require.Len(t, decoded.ErrorMessage, 500)
	require.Contains(t, decoded.Message, "xxx...")
	require.LessOrEqual(t, len([]rune(decoded.Message)), 620)
}

func TestADKToolErrorNormalizationStreamableReturnsStableToolError(t *testing.T) {
	middleware := NewADKToolErrorNormalizationMiddleware()
	wrapped, err := middleware.WrapStreamableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (*schema.StreamReader[string], error) {
			return nil, errors.New("stream unavailable")
		},
		&adk.ToolContext{Name: "stream_docs", CallID: "call-stream"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), `{}`)
	require.NoError(t, err)
	content, err := result.Recv()

	require.NoError(t, err)
	decoded, ok := decodeADKToolErrorResult(content)
	require.True(t, ok)
	require.Equal(t, "stream_docs", decoded.ToolName)
	require.Equal(t, "call-stream", decoded.ToolCallID)
	require.Equal(t, "stream unavailable", decoded.ErrorMessage)
	_, err = result.Recv()
	require.ErrorIs(t, err, io.EOF)
}

func TestADKToolErrorNormalizationEnhancedInvokableReturnsStableToolError(t *testing.T) {
	middleware := NewADKToolErrorNormalizationMiddleware()
	wrapped, err := middleware.WrapEnhancedInvokableToolCall(
		context.Background(),
		func(
			context.Context,
			*schema.ToolArgument,
			...tool.Option,
		) (*schema.ToolResult, error) {
			return nil, errors.New("image inspect failed")
		},
		&adk.ToolContext{Name: "inspect_image", CallID: "call-image"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), &schema.ToolArgument{Text: `{}`})

	require.NoError(t, err)
	require.Len(t, result.Parts, 1)
	require.Equal(t, schema.ToolPartTypeText, result.Parts[0].Type)
	decoded, ok := decodeADKToolErrorResult(result.Parts[0].Text)
	require.True(t, ok)
	require.Equal(t, "inspect_image", decoded.ToolName)
	require.Equal(t, "call-image", decoded.ToolCallID)
	require.Equal(t, "image inspect failed", decoded.ErrorMessage)
}

func TestADKToolErrorNormalizationEnhancedStreamableReturnsStableToolError(t *testing.T) {
	middleware := NewADKToolErrorNormalizationMiddleware()
	wrapped, err := middleware.WrapEnhancedStreamableToolCall(
		context.Background(),
		func(
			context.Context,
			*schema.ToolArgument,
			...tool.Option,
		) (*schema.StreamReader[*schema.ToolResult], error) {
			return nil, errors.New("video stream failed")
		},
		&adk.ToolContext{Name: "inspect_video", CallID: "call-video"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), &schema.ToolArgument{Text: `{}`})
	require.NoError(t, err)
	chunk, err := result.Recv()

	require.NoError(t, err)
	require.Len(t, chunk.Parts, 1)
	require.Equal(t, schema.ToolPartTypeText, chunk.Parts[0].Type)
	decoded, ok := decodeADKToolErrorResult(chunk.Parts[0].Text)
	require.True(t, ok)
	require.Equal(t, "inspect_video", decoded.ToolName)
	require.Equal(t, "call-video", decoded.ToolCallID)
	require.Equal(t, "video stream failed", decoded.ErrorMessage)
	_, err = result.Recv()
	require.ErrorIs(t, err, io.EOF)
}

func TestADKToolErrorNormalizationPreservesInterruptErrors(t *testing.T) {
	middleware := NewADKToolErrorNormalizationMiddleware()
	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
			return "", tool.Interrupt(ctx, "approval required")
		},
		&adk.ToolContext{Name: "approval", CallID: "call-approval"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), `{}`)

	require.Empty(t, result)
	require.Error(t, err)
	var signal *adk.InterruptSignal
	require.ErrorAs(t, err, &signal)
}

func TestADKToolErrorNormalizationPreservesSkillAgentHubConfigurationErrors(t *testing.T) {
	middleware := NewADKToolErrorNormalizationMiddleware()
	wrapped, err := middleware.WrapInvokableToolCall(
		context.Background(),
		func(context.Context, string, ...tool.Option) (string, error) {
			return "", errors.New(
				"skill 'delegated-research' requires context:fork but AgentHub is not configured",
			)
		},
		&adk.ToolContext{Name: "skill", CallID: "call-skill"},
	)
	require.NoError(t, err)

	result, err := wrapped(context.Background(), `{}`)

	require.Empty(t, result)
	require.ErrorContains(t, err, "requires context:fork but AgentHub is not configured")
}

func TestADKToolErrorNormalizationOrder(t *testing.T) {
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareReduction),
		adkMiddlewareIndex(ADKMiddlewareToolErrorNormalization),
	)
	require.Less(
		t,
		adkMiddlewareIndex(ADKMiddlewareToolErrorNormalization),
		adkMiddlewareIndex(ADKMiddlewareSafetyFinish),
	)
}
