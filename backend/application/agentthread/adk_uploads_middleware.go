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
	"fmt"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

const (
	adkUploadedFilesStart = "<uploaded_files>"
	adkUploadedFilesEnd   = "</uploaded_files>"
)

type ADKUploadedFilesMiddleware struct {
	*adk.BaseChatModelAgentMiddleware
	run *RunSummary
}

func NewADKUploadedFilesMiddleware(
	run *RunSummary,
) *ADKUploadedFilesMiddleware {
	return &ADKUploadedFilesMiddleware{
		BaseChatModelAgentMiddleware: &adk.BaseChatModelAgentMiddleware{},
		run:                          run,
	}
}

func (m *ADKUploadedFilesMiddleware) BeforeModelRewriteState(
	ctx context.Context,
	state *adk.ChatModelAgentState,
	_ *adk.ModelContext,
) (context.Context, *adk.ChatModelAgentState, error) {
	if state == nil {
		state = &adk.ChatModelAgentState{}
	}
	if m == nil || m.run == nil {
		return ctx, state, nil
	}

	files := uploadedFilesFromRunInput(m.run.Input)
	if len(files) == 0 {
		return ctx, state, nil
	}
	if tracker := adkParityStateTrackerFromContext(ctx); tracker != nil {
		if err := tracker.MergeUploads(adkParityUploadsFromSummaries(files)); err != nil {
			return ctx, state, fmt.Errorf("record eino adk parity uploads: %w", err)
		}
	}
	if len(state.Messages) == 0 {
		return ctx, state, nil
	}
	targetIndex := lastADKUserInjectionTargetIndex(state.Messages)
	if targetIndex < 0 {
		return ctx, state, nil
	}
	target := state.Messages[targetIndex]
	if target == nil || strings.Contains(target.Content, adkUploadedFilesStart) {
		return ctx, state, nil
	}

	block := buildADKUploadedFilesContext(files)
	if block == "" {
		return ctx, state, nil
	}

	nextMessage := cloneADKMessage(target)
	content := strings.TrimSpace(nextMessage.Content)
	if content == "" {
		nextMessage.Content = block
	} else {
		nextMessage.Content = block + "\n\n" + content
	}

	nextState := *state
	nextState.Messages = append([]*schema.Message(nil), state.Messages...)
	nextState.Messages[targetIndex] = nextMessage
	return ctx, &nextState, nil
}

func adkParityUploadsFromSummaries(
	files []*TaskThreadUploadedFileSummary,
) []ADKParityUpload {
	result := make([]ADKParityUpload, 0, len(files))
	for _, file := range normalizeADKUploadedFiles(files) {
		result = append(result, ADKParityUpload{
			FileID: file.FileID, FileName: file.FileName,
			VirtualPath: file.VirtualPath, ContentType: file.ContentType,
			SizeBytes: file.SizeBytes, CreatedAt: file.CreatedAt,
		})
	}
	return result
}

func uploadedFilesFromRunInput(rawInput string) []*TaskThreadUploadedFileSummary {
	rawInput = strings.TrimSpace(rawInput)
	if rawInput == "" {
		return nil
	}

	var input modelExecutorRunInput
	if err := json.Unmarshal([]byte(rawInput), &input); err == nil &&
		len(input.UploadedFiles) > 0 {
		return normalizeADKUploadedFiles(input.UploadedFiles)
	}

	var deerflowInput struct {
		Messages []struct {
			AdditionalKwargs struct {
				Files []*TaskThreadUploadedFileSummary `json:"files"`
			} `json:"additional_kwargs"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(rawInput), &deerflowInput); err != nil {
		return nil
	}
	files := make([]*TaskThreadUploadedFileSummary, 0)
	for _, message := range deerflowInput.Messages {
		files = append(files, message.AdditionalKwargs.Files...)
	}
	return normalizeADKUploadedFiles(files)
}

func normalizeADKUploadedFiles(
	files []*TaskThreadUploadedFileSummary,
) []*TaskThreadUploadedFileSummary {
	if len(files) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(files))
	result := make([]*TaskThreadUploadedFileSummary, 0, len(files))
	for _, file := range files {
		if file == nil {
			continue
		}
		fileName, err := domainservice.NormalizeUploadFileName(file.FileName)
		if err != nil {
			continue
		}
		virtualPath := strings.TrimSpace(file.VirtualPath)
		if virtualPath == "" {
			virtualPath = domainservice.UploadVirtualPath(fileName)
		}
		if virtualPath != domainservice.UploadVirtualPath(fileName) {
			continue
		}
		if _, exists := seen[virtualPath]; exists {
			continue
		}
		seen[virtualPath] = struct{}{}

		result = append(result, &TaskThreadUploadedFileSummary{
			FileID:      file.FileID,
			FileName:    fileName,
			VirtualPath: virtualPath,
			ContentType: strings.TrimSpace(file.ContentType),
			SizeBytes:   file.SizeBytes,
			CreatedAt:   file.CreatedAt,
		})
	}
	return result
}

func buildADKUploadedFilesContext(
	files []*TaskThreadUploadedFileSummary,
) string {
	files = normalizeADKUploadedFiles(files)
	if len(files) == 0 {
		return ""
	}

	lines := []string{
		adkUploadedFilesStart,
		"The user uploaded the following files. Use the read_file tool with the path when file content is needed.",
	}
	for _, file := range files {
		lines = append(
			lines,
			fmt.Sprintf("- name: %s", file.FileName),
			fmt.Sprintf("  path: %s", file.VirtualPath),
		)
		if file.ContentType != "" {
			lines = append(lines, fmt.Sprintf("  type: %s", file.ContentType))
		}
		if file.SizeBytes > 0 {
			lines = append(lines, fmt.Sprintf("  size: %d bytes", file.SizeBytes))
		}
	}
	lines = append(lines, adkUploadedFilesEnd)
	return strings.Join(lines, "\n")
}

func cloneADKMessage(message *schema.Message) *schema.Message {
	if message == nil {
		return nil
	}
	next := *message
	if message.Extra != nil {
		next.Extra = make(map[string]any, len(message.Extra))
		for key, value := range message.Extra {
			next.Extra[key] = value
		}
	}
	return &next
}
