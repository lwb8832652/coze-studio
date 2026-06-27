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
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ADKToolErrorNormalizationMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
}

func NewADKToolErrorNormalizationMiddleware() *ADKToolErrorNormalizationMiddleware {
	return &ADKToolErrorNormalizationMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
	}
}

func (m *ADKToolErrorNormalizationMiddleware) WrapInvokableToolCall(
	_ context.Context,
	endpoint adk.InvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.InvokableToolCallEndpoint, error) {
	return func(
		ctx context.Context,
		argumentsInJSON string,
		opts ...tool.Option,
	) (string, error) {
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		if err != nil {
			if shouldPassThroughADKToolError(err, tCtx) {
				return "", err
			}
			return encodeADKToolErrorResult(
				adkToolContextName(tCtx),
				adkToolContextCallID(tCtx),
				err,
			), nil
		}

		return result, nil
	}, nil
}

func (m *ADKToolErrorNormalizationMiddleware) WrapStreamableToolCall(
	_ context.Context,
	endpoint adk.StreamableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.StreamableToolCallEndpoint, error) {
	return func(
		ctx context.Context,
		argumentsInJSON string,
		opts ...tool.Option,
	) (*schema.StreamReader[string], error) {
		result, err := endpoint(ctx, argumentsInJSON, opts...)
		if err != nil {
			if shouldPassThroughADKToolError(err, tCtx) {
				return nil, err
			}
			return schema.StreamReaderFromArray([]string{
				encodeADKToolErrorResult(
					adkToolContextName(tCtx),
					adkToolContextCallID(tCtx),
					err,
				),
			}), nil
		}

		return result, nil
	}, nil
}

func (m *ADKToolErrorNormalizationMiddleware) WrapEnhancedInvokableToolCall(
	_ context.Context,
	endpoint adk.EnhancedInvokableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.EnhancedInvokableToolCallEndpoint, error) {
	return func(
		ctx context.Context,
		toolArgument *schema.ToolArgument,
		opts ...tool.Option,
	) (*schema.ToolResult, error) {
		result, err := endpoint(ctx, toolArgument, opts...)
		if err != nil {
			if shouldPassThroughADKToolError(err, tCtx) {
				return nil, err
			}
			return newADKToolErrorToolResult(
				adkToolContextName(tCtx),
				adkToolContextCallID(tCtx),
				err,
			), nil
		}

		return result, nil
	}, nil
}

func (m *ADKToolErrorNormalizationMiddleware) WrapEnhancedStreamableToolCall(
	_ context.Context,
	endpoint adk.EnhancedStreamableToolCallEndpoint,
	tCtx *adk.ToolContext,
) (adk.EnhancedStreamableToolCallEndpoint, error) {
	return func(
		ctx context.Context,
		toolArgument *schema.ToolArgument,
		opts ...tool.Option,
	) (*schema.StreamReader[*schema.ToolResult], error) {
		result, err := endpoint(ctx, toolArgument, opts...)
		if err != nil {
			if shouldPassThroughADKToolError(err, tCtx) {
				return nil, err
			}
			return schema.StreamReaderFromArray([]*schema.ToolResult{
				newADKToolErrorToolResult(
					adkToolContextName(tCtx),
					adkToolContextCallID(tCtx),
					err,
				),
			}), nil
		}

		return result, nil
	}, nil
}

func newADKToolErrorToolResult(
	toolName string,
	toolCallID string,
	err error,
) *schema.ToolResult {
	return &schema.ToolResult{
		Parts: []schema.ToolOutputPart{{
			Type: schema.ToolPartTypeText,
			Text: encodeADKToolErrorResult(toolName, toolCallID, err),
		}},
	}
}

func adkToolContextName(tCtx *adk.ToolContext) string {
	if tCtx == nil {
		return ""
	}

	return tCtx.Name
}

func adkToolContextCallID(tCtx *adk.ToolContext) string {
	if tCtx == nil {
		return ""
	}

	return tCtx.CallID
}

func shouldPassThroughADKToolError(err error, tCtx *adk.ToolContext) bool {
	if err == nil {
		return false
	}
	if isADKToolControlFlowError(err) {
		return true
	}
	if isADKInternalToolConfigurationError(err, tCtx) {
		return true
	}

	return false
}

func isADKToolControlFlowError(err error) bool {
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if _, ok := compose.ExtractInterruptInfo(err); ok {
		return true
	}
	var interruptSignal *adk.InterruptSignal
	if errors.As(err, &interruptSignal) {
		return true
	}
	var cancelErr *adk.CancelError
	if errors.As(err, &cancelErr) {
		return true
	}
	var interruptErr *adk.InterruptError
	if errors.As(err, &interruptErr) {
		return true
	}

	return false
}

func isADKInternalToolConfigurationError(
	err error,
	tCtx *adk.ToolContext,
) bool {
	if adkToolContextName(tCtx) != "skill" {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "AgentHub is not configured") ||
		strings.Contains(message, "ModelHub is not configured")
}
