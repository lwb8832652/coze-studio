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
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestADKToolResultProtocolEncodesAndDecodesToolError(t *testing.T) {
	encoded := encodeADKToolErrorResult(
		"search_docs",
		"call-1",
		errors.New(" upstream timeout "),
	)

	decoded, ok := decodeADKToolErrorResult(encoded)

	require.True(t, ok)
	require.Equal(t, adkToolErrorSchema, decoded.Schema)
	require.Equal(t, "failed", decoded.Status)
	require.Equal(t, "search_docs", decoded.ToolName)
	require.Equal(t, "call-1", decoded.ToolCallID)
	require.Equal(t, "upstream timeout", decoded.ErrorMessage)
	require.True(t, decoded.Recoverable)
	require.True(t, decoded.Normalized)
	require.JSONEq(t, `{
		"schema":"coze.tool_error.v1",
		"status":"failed",
		"tool_name":"search_docs",
		"tool_call_id":"call-1",
		"error_message":"upstream timeout",
		"recoverable":true,
		"normalized":true
	}`, encoded)
}

func TestADKToolResultProtocolDefaultsBlankToolError(t *testing.T) {
	encoded := encodeADKToolErrorResult(
		"search_docs",
		"call-1",
		errors.New("   "),
	)

	decoded, ok := decodeADKToolErrorResult(encoded)

	require.True(t, ok)
	require.Equal(t, "tool call failed", decoded.ErrorMessage)
}

func TestADKToolResultProtocolTruncatesToolError(t *testing.T) {
	encoded := encodeADKToolErrorResult(
		"search_docs",
		"call-1",
		errors.New(strings.Repeat("x", adkToolErrorMessageMaxLength+32)),
	)

	decoded, ok := decodeADKToolErrorResult(encoded)

	require.True(t, ok)
	require.Len(t, decoded.ErrorMessage, adkToolErrorMessageMaxLength)
}

func TestADKToolResultProtocolIgnoresNonToolErrorPayload(t *testing.T) {
	decoded, ok := decodeADKToolErrorResult(`{"schema":"coze.tool_repair.v1"}`)

	require.False(t, ok)
	require.Nil(t, decoded)
}

func TestADKToolResultProtocolEncodesToolRepair(t *testing.T) {
	encoded := encodeADKToolRepairResult("search_docs", "call-1")

	require.JSONEq(t, `{
		"schema":"coze.tool_repair.v1",
		"status":"patched",
		"tool_name":"search_docs",
		"tool_call_id":"call-1",
		"reason":"missing_tool_result",
		"message":"Tool result was missing and has been patched by Coze runtime. Continue with available context or retry the tool if needed."
	}`, encoded)
}
