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

package workbench

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/coze-dev/coze-studio/backend/bizpkg/llm/modelbuilder"
	crossknowledge "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge"
	knowledgemodel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type chatModelProvider func(ctx context.Context, modelType int64) (model.BaseChatModel, bool, error)

type answerRequest struct {
	mode      ChatMode
	spaceID   int64
	message   string
	modelType int64
	modelName string
	enableKbs []string
}

func defaultChatModelProvider(ctx context.Context, modelType int64) (model.BaseChatModel, bool, error) {
	if modelType > 0 {
		chatModel, _, err := modelbuilder.BuildModelByID(ctx, modelType, nil)
		if err != nil {
			return nil, false, err
		}
		return chatModel, true, nil
	}

	return modelbuilder.GetBuiltinChatModel(ctx, "WKB_")
}

func (s *ApplicationService) runAnswer(ctx context.Context, req answerRequest) (resultPayload, error) {
	var provider chatModelProvider
	if s != nil {
		provider = s.chatModelProvider
	}
	if provider == nil {
		provider = defaultChatModelProvider
	}

	cm, configured, err := provider(ctx, req.modelType)
	if err != nil {
		return resultPayload{}, err
	}
	if !configured || cm == nil {
		return resultPayload{}, fmt.Errorf("workbench chat model is not configured")
	}

	messages := make([]*schema.Message, 0, 2)
	retrievalSources := []string(nil)
	if req.mode == ChatModeAsk {
		if knowledgeContext, sources, ok := s.retrieveAskKnowledge(ctx, req); ok {
			messages = append(messages, schema.SystemMessage(knowledgeContext))
			retrievalSources = sources
		}
	}
	messages = append(messages, schema.UserMessage(req.message))

	resp, err := cm.Generate(ctx, messages)
	if err != nil {
		return resultPayload{}, err
	}
	if resp == nil {
		return resultPayload{}, fmt.Errorf("workbench chat model returned empty response")
	}

	payload := resultPayload{
		Message:          strings.TrimSpace(resp.Content),
		ResultType:       resultTypeAnswer,
		RetrievalSources: retrievalSources,
	}
	if req.mode == ChatModeAuto {
		payload.ExecutionType = executionTypeArk
	}
	return payload, nil
}

func (s *ApplicationService) retrieveAskKnowledge(ctx context.Context, req answerRequest) (string, []string, bool) {
	var knowledgeSVC crossknowledge.Knowledge
	if s != nil {
		knowledgeSVC = s.knowledgeSVC
	}
	if knowledgeSVC == nil {
		knowledgeSVC = crossknowledge.DefaultSVC()
	}
	if knowledgeSVC == nil {
		return "", nil, false
	}
	knowledgeIDs, ok := s.resolveAskKnowledgeIDs(ctx, knowledgeSVC, req)
	if !ok {
		return "", nil, false
	}

	topK := int64(3)
	resp, err := knowledgeSVC.Retrieve(ctx, &knowledgemodel.RetrieveRequest{
		Query:        req.message,
		KnowledgeIDs: knowledgeIDs,
		Strategy: &knowledgemodel.RetrievalStrategy{
			TopK:       &topK,
			SearchType: knowledgemodel.SearchTypeHybrid,
		},
	})
	if err != nil {
		logs.CtxWarnf(ctx, "workbench ask knowledge retrieve failed: %v", err)
		return "", nil, false
	}
	if resp == nil || len(resp.RetrieveSlices) == 0 {
		return "", nil, false
	}

	contexts := make([]string, 0, len(resp.RetrieveSlices))
	for _, item := range resp.RetrieveSlices {
		if item == nil || item.Slice == nil {
			continue
		}
		text := strings.TrimSpace(item.Slice.GetSliceContent())
		if text == "" {
			continue
		}
		documentName := strings.TrimSpace(item.Slice.DocumentName)
		if documentName != "" {
			contexts = append(contexts, fmt.Sprintf("[%s]\n%s", documentName, text))
			continue
		}
		contexts = append(contexts, text)
	}
	if len(contexts) == 0 {
		return "", nil, false
	}

	prompt := "Use the following knowledge context to answer the user.\n\n" + strings.Join(contexts, "\n\n")
	return prompt, []string{"knowledge"}, true
}

func (s *ApplicationService) resolveAskKnowledgeIDs(ctx context.Context, knowledgeSVC crossknowledge.Knowledge, req answerRequest) ([]int64, bool) {
	if len(req.enableKbs) > 0 {
		ids := make([]int64, 0, len(req.enableKbs))
		for _, rawID := range req.enableKbs {
			idText := strings.TrimSpace(rawID)
			if idText == "" {
				continue
			}
			id, err := strconv.ParseInt(idText, 10, 64)
			if err != nil || id <= 0 {
				logs.CtxWarnf(ctx, "workbench ask ignores invalid knowledge id %q: %v", rawID, err)
				continue
			}
			ids = append(ids, id)
		}
		if len(ids) == 0 || req.spaceID == 0 {
			return nil, false
		}
		return s.filterEnabledKnowledgeIDs(ctx, knowledgeSVC, req.spaceID, ids)
	}
	if req.spaceID == 0 {
		return nil, false
	}

	return s.filterEnabledKnowledgeIDs(ctx, knowledgeSVC, req.spaceID, nil)
}

func (s *ApplicationService) filterEnabledKnowledgeIDs(ctx context.Context, knowledgeSVC crossknowledge.Knowledge, spaceID int64, requestedIDs []int64) ([]int64, bool) {
	if knowledgeSVC == nil || spaceID == 0 {
		return nil, false
	}

	pageSize := 100
	listReq := &knowledgemodel.ListKnowledgeRequest{
		SpaceID:  &spaceID,
		Status:   []int32{int32(knowledgemodel.KnowledgeStatusEnable)},
		PageSize: &pageSize,
	}
	if len(requestedIDs) > 0 {
		listReq.IDs = requestedIDs
	}
	resp, err := knowledgeSVC.ListKnowledge(ctx, listReq)
	if err != nil {
		logs.CtxWarnf(ctx, "workbench ask list knowledge failed: %v", err)
		return nil, false
	}
	if resp == nil || len(resp.KnowledgeList) == 0 {
		return nil, false
	}
	ids := make([]int64, 0, len(resp.KnowledgeList))
	for _, item := range resp.KnowledgeList {
		if item == nil || item.ID <= 0 {
			continue
		}
		ids = append(ids, item.ID)
	}
	return ids, len(ids) > 0
}
