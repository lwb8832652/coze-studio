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
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

func TestADKUploadedFilesMiddlewareInjectsLatestUserContext(t *testing.T) {
	input := modelExecutorRunInput{
		Messages: []*schema.Message{
			schema.UserMessage("请总结附件"),
		},
		UploadedFiles: []*TaskThreadUploadedFileSummary{
			{
				FileName:    "report.md",
				VirtualPath: "/mnt/user-data/uploads/report.md",
				ContentType: "text/markdown",
				SizeBytes:   128,
			},
		},
	}
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	middleware := NewADKUploadedFilesMiddleware(&RunSummary{
		RunID: 20,
		Input: string(raw),
	})
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.UserMessage("旧问题"),
			schema.AssistantMessage("旧回答", nil),
			schema.UserMessage("请总结附件"),
		},
	}

	_, got, err := middleware.BeforeModelRewriteState(
		context.Background(),
		state,
		&adk.ModelContext{},
	)

	require.NoError(t, err)
	require.Len(t, got.Messages, 3)
	require.Equal(t, "旧问题", got.Messages[0].Content)
	latest := got.Messages[2]
	require.NotSame(t, state.Messages[2], latest)
	require.Contains(t, latest.Content, "<uploaded_files>")
	require.Contains(t, latest.Content, "report.md")
	require.Contains(t, latest.Content, "/mnt/user-data/uploads/report.md")
	require.Contains(t, latest.Content, "text/markdown")
	require.Contains(t, latest.Content, "128 bytes")
	require.Contains(t, latest.Content, "请总结附件")
	require.NotContains(t, latest.Content, "agent-runtime/")
	require.NotContains(t, state.Messages[2].Content, "<uploaded_files>")
}

func TestADKUploadedFilesMiddlewareSkipsEmptyOrDuplicateContext(t *testing.T) {
	middleware := NewADKUploadedFilesMiddleware(&RunSummary{
		RunID: 20,
		Input: `{"uploaded_files":[{"file_name":"report.md","virtual_path":"/mnt/user-data/uploads/report.md"}]}`,
	})
	state := &adk.ChatModelAgentState{
		Messages: []*schema.Message{
			schema.UserMessage("<uploaded_files>\n- report.md\n</uploaded_files>\n请总结附件"),
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
