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
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	knowledgemodel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	"github.com/coze-dev/coze-studio/backend/internal/testutil"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
)

func TestRunAutoAnswerUsesBuiltinModelAndMarksArk(t *testing.T) {
	ctx := context.Background()
	app := withTestWorkbenchApp(t, &ServiceComponents{
		ChatModelProvider: func(_ context.Context, modelType int64) (model.BaseChatModel, bool, error) {
			require.Zero(t, modelType)
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
					require.Len(t, in, 1)
					require.Equal(t, schema.User, in[0].Role)
					require.Equal(t, "hello", in[0].Content)
					return schema.AssistantMessage("auto answer", nil), nil
				},
			}, true, nil
		},
	})

	payload, err := app.runAnswer(ctx, answerRequest{mode: ChatModeAuto, message: "hello"})
	require.NoError(t, err)

	assert.Equal(t, resultTypeAnswer, payload.ResultType)
	assert.Equal(t, executionTypeArk, payload.ExecutionType)
	assert.Equal(t, "auto answer", payload.Message)
	assert.Empty(t, payload.RetrievalSources)
}

func TestRunAnswerUsesSelectedModelType(t *testing.T) {
	ctx := context.Background()
	var gotModelType int64
	app := withTestWorkbenchApp(t, &ServiceComponents{
		ChatModelProvider: func(_ context.Context, modelType int64) (model.BaseChatModel, bool, error) {
			gotModelType = modelType
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
					require.Len(t, in, 1)
					require.Equal(t, "hello selected model", in[0].Content)
					return schema.AssistantMessage("selected model answer", nil), nil
				},
			}, true, nil
		},
	})

	payload, err := app.runAnswer(ctx, answerRequest{
		mode:      ChatModeAsk,
		message:   "hello selected model",
		modelType: 100002,
	})
	require.NoError(t, err)

	assert.Equal(t, int64(100002), gotModelType)
	assert.Equal(t, "selected model answer", payload.Message)
}

func TestRunAnswerStreamsAnswerDeltasToTaskEvents(t *testing.T) {
	ctx := context.Background()
	taskApp := &recordingWorkbenchTaskApp{}
	app := &ApplicationService{
		taskApp: taskApp,
		chatModelProvider: func(_ context.Context, _ int64) (model.BaseChatModel, bool, error) {
			return &testutil.UTChatModel{
				StreamResultProvider: func(_ int, in []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
					require.Len(t, in, 1)
					require.Equal(t, "hello", in[0].Content)
					return schema.StreamReaderFromArray([]*schema.Message{
						schema.AssistantMessage("hello ", nil),
						schema.AssistantMessage("world", nil),
					}), nil
				},
			}, true, nil
		},
	}

	payload, err := app.runAnswer(ctx, answerRequest{
		taskID:  10,
		mode:    ChatModeAuto,
		message: "hello",
	})
	require.NoError(t, err)

	assert.Equal(t, resultTypeAnswer, payload.ResultType)
	assert.Equal(t, executionTypeArk, payload.ExecutionType)
	assert.Equal(t, "hello world", payload.Message)
	require.Len(t, taskApp.events, 2)
	assert.Equal(t, "answer.delta", taskApp.events[0].eventType)
	assert.Equal(t, int64(10), taskApp.events[0].taskID)
	assert.Contains(t, taskApp.events[0].payload, `"message":"hello "`)
	assert.Contains(t, taskApp.events[0].payload, `"delta":"hello "`)
	assert.Equal(t, "answer.delta", taskApp.events[1].eventType)
	assert.Contains(t, taskApp.events[1].payload, `"message":"hello world"`)
	assert.Contains(t, taskApp.events[1].payload, `"delta":"world"`)
}

func TestRunAskRetrievesKnowledgeContext(t *testing.T) {
	ctx := context.Background()
	knowledge := &fakeKnowledgeService{
		listResponse: &knowledgemodel.ListKnowledgeResponse{
			KnowledgeList: []*knowledgemodel.Knowledge{
				{Info: knowledgemodel.Info{ID: 10}},
			},
		},
		response: &knowledgemodel.RetrieveResponse{
			RetrieveSlices: []*knowledgemodel.RetrieveSlice{
				{
					Slice: &knowledgemodel.Slice{
						DocumentName: "guide",
						RawContent: []*knowledgemodel.SliceContent{
							{Type: knowledgemodel.SliceContentTypeText, Text: ptr.Of("knowledge text")},
						},
					},
				},
			},
		},
	}
	app := withTestWorkbenchApp(t, &ServiceComponents{
		KnowledgeSVC: knowledge,
		ChatModelProvider: func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
					require.Len(t, in, 2)
					require.Equal(t, schema.System, in[0].Role)
					require.Contains(t, in[0].Content, "knowledge text")
					require.Equal(t, schema.User, in[1].Role)
					require.Equal(t, "what is in docs", in[1].Content)
					return schema.AssistantMessage("ask answer", nil), nil
				},
			}, true, nil
		},
	})

	payload, err := app.runAnswer(ctx, answerRequest{
		mode:      ChatModeAsk,
		spaceID:   1,
		message:   "what is in docs",
		enableKbs: []string{"10", "99"},
	})
	require.NoError(t, err)

	assert.Equal(t, int64(1), knowledge.lastListSpaceID)
	assert.Equal(t, []int64{10, 99}, knowledge.lastListIDs)
	assert.Equal(t, "what is in docs", knowledge.lastQuery)
	assert.Equal(t, []int64{10}, knowledge.lastKnowledgeIDs)
	assert.Equal(t, resultTypeAnswer, payload.ResultType)
	assert.Empty(t, payload.ExecutionType)
	assert.Equal(t, "ask answer", payload.Message)
	assert.Equal(t, []string{"knowledge"}, payload.RetrievalSources)
}

func TestRunAskUsesDefaultEnabledKnowledgeWhenNoSelection(t *testing.T) {
	ctx := context.Background()
	knowledge := &fakeKnowledgeService{
		listResponse: &knowledgemodel.ListKnowledgeResponse{
			KnowledgeList: []*knowledgemodel.Knowledge{
				{Info: knowledgemodel.Info{ID: 11}},
				{Info: knowledgemodel.Info{ID: 12}},
			},
		},
		response: &knowledgemodel.RetrieveResponse{
			RetrieveSlices: []*knowledgemodel.RetrieveSlice{
				{
					Slice: &knowledgemodel.Slice{
						DocumentName: "default",
						RawContent: []*knowledgemodel.SliceContent{
							{Type: knowledgemodel.SliceContentTypeText, Text: ptr.Of("default knowledge")},
						},
					},
				},
			},
		},
	}
	app := withTestWorkbenchApp(t, &ServiceComponents{
		KnowledgeSVC: knowledge,
		ChatModelProvider: func(context.Context, int64) (model.BaseChatModel, bool, error) {
			return &testutil.UTChatModel{
				InvokeResultProvider: func(_ int, in []*schema.Message) (*schema.Message, error) {
					require.Len(t, in, 2)
					require.Contains(t, in[0].Content, "default knowledge")
					return schema.AssistantMessage("default ask answer", nil), nil
				},
			}, true, nil
		},
	})

	payload, err := app.runAnswer(ctx, answerRequest{
		mode:    ChatModeAsk,
		spaceID: 99,
		message: "use defaults",
	})
	require.NoError(t, err)

	assert.Equal(t, int64(99), knowledge.lastListSpaceID)
	assert.Equal(t, []int64{11, 12}, knowledge.lastKnowledgeIDs)
	assert.Equal(t, "default ask answer", payload.Message)
	assert.Equal(t, []string{"knowledge"}, payload.RetrievalSources)
}

func withTestWorkbenchApp(t *testing.T, components *ServiceComponents) *ApplicationService {
	t.Helper()
	prevSkillSVC := SVC.skillSVC
	prevTaskSVC := SVC.taskSVC
	prevKnowledgeSVC := SVC.knowledgeSVC
	prevAgentRunSVC := SVC.agentRunSVC
	prevChatModelProvider := SVC.chatModelProvider
	app := InitService(components)
	app.skillSVC = components.SkillSVC
	app.taskSVC = components.TaskSVC
	app.knowledgeSVC = components.KnowledgeSVC
	app.agentRunSVC = components.AgentRunSVC
	app.chatModelProvider = components.ChatModelProvider
	t.Cleanup(func() {
		SVC.skillSVC = prevSkillSVC
		SVC.taskSVC = prevTaskSVC
		SVC.knowledgeSVC = prevKnowledgeSVC
		SVC.agentRunSVC = prevAgentRunSVC
		SVC.chatModelProvider = prevChatModelProvider
	})
	return app
}

type fakeKnowledgeService struct {
	lastQuery        string
	lastKnowledgeIDs []int64
	lastListSpaceID  int64
	lastListIDs      []int64
	listResponse     *knowledgemodel.ListKnowledgeResponse
	response         *knowledgemodel.RetrieveResponse
	err              error
}

func (f *fakeKnowledgeService) ListKnowledge(_ context.Context, req *knowledgemodel.ListKnowledgeRequest) (*knowledgemodel.ListKnowledgeResponse, error) {
	if req != nil && req.SpaceID != nil {
		f.lastListSpaceID = *req.SpaceID
	}
	if req != nil {
		f.lastListIDs = append([]int64(nil), req.IDs...)
	}
	return f.listResponse, nil
}

func (f *fakeKnowledgeService) GetKnowledgeByID(context.Context, *knowledgemodel.GetKnowledgeByIDRequest) (*knowledgemodel.GetKnowledgeByIDResponse, error) {
	return nil, nil
}

func (f *fakeKnowledgeService) Retrieve(_ context.Context, req *knowledgemodel.RetrieveRequest) (*knowledgemodel.RetrieveResponse, error) {
	if req != nil {
		f.lastQuery = req.Query
		f.lastKnowledgeIDs = append([]int64(nil), req.KnowledgeIDs...)
	}
	if len(f.lastKnowledgeIDs) == 0 {
		return &knowledgemodel.RetrieveResponse{}, f.err
	}
	return f.response, f.err
}

func (f *fakeKnowledgeService) DeleteKnowledge(context.Context, *knowledgemodel.DeleteKnowledgeRequest) error {
	return nil
}

func (f *fakeKnowledgeService) MGetKnowledgeByID(context.Context, *knowledgemodel.MGetKnowledgeByIDRequest) (*knowledgemodel.MGetKnowledgeByIDResponse, error) {
	return nil, nil
}

func (f *fakeKnowledgeService) Store(context.Context, *knowledgemodel.CreateDocumentRequest) (*knowledgemodel.CreateDocumentResponse, error) {
	return nil, nil
}

func (f *fakeKnowledgeService) Delete(context.Context, *knowledgemodel.DeleteDocumentRequest) (*knowledgemodel.DeleteDocumentResponse, error) {
	return nil, nil
}

func (f *fakeKnowledgeService) ListKnowledgeDetail(context.Context, *knowledgemodel.ListKnowledgeDetailRequest) (*knowledgemodel.ListKnowledgeDetailResponse, error) {
	return nil, nil
}

func (f *fakeKnowledgeService) MGetSlice(context.Context, *knowledgemodel.MGetSliceRequest) (*knowledgemodel.MGetSliceResponse, error) {
	return nil, nil
}

func (f *fakeKnowledgeService) MGetDocument(context.Context, *knowledgemodel.MGetDocumentRequest) (*knowledgemodel.MGetDocumentResponse, error) {
	return nil, nil
}
