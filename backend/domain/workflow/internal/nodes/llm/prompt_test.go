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

package llm

import (
	"context"
	"testing"

	"github.com/bytedance/mockey"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/api/model/app/bot_common"
	"github.com/coze-dev/coze-studio/backend/api/model/app/developer_api"
	"github.com/coze-dev/coze-studio/backend/domain/workflow/entity/vo"
	"github.com/coze-dev/coze-studio/backend/pkg/lang/ptr"
	"github.com/coze-dev/coze-studio/backend/pkg/urltobase64url"
)

func TestAdaptDowngradesJSONModelParamsForSingleStringOutput(t *testing.T) {
	cfg := &Config{}
	node := &vo.Node{
		ID: "162256",
		Data: &vo.Data{
			Meta: &vo.NodeMetaFE{Title: "大模型"},
			Inputs: &vo.Inputs{
				InputParameters: []*vo.Param{},
				LLMParam: []*vo.Param{
					literalLLMParam("temperature", "0.85"),
					literalLLMParam("maxTokens", "2000"),
					literalLLMParam("responseFormat", "2"),
					literalLLMParam("modleName", "deepseek-v4-pro"),
					literalLLMParam("modelType", "100002"),
					literalLLMParam("prompt", "你是什么大模型"),
					literalLLMParam("systemPrompt", ""),
					literalLLMParam("generationDiversity", "balance"),
				},
			},
			Outputs: []any{
				&vo.Variable{
					Name: "output",
					Type: vo.VariableTypeString,
				},
			},
		},
	}

	ns, err := cfg.Adapt(context.Background(), node)

	require.NoError(t, err)
	require.NotNil(t, ns.StreamConfigs)
	assert.True(t, ns.StreamConfigs.CanGeneratesStream)
	assert.Equal(t, FormatText, cfg.OutputFormat)
	assert.Equal(t, vo.ResponseFormatText, cfg.LLMParams.ResponseFormat)
	assert.Equal(
		t,
		bot_common.ModelResponseFormat_Text,
		cfg.LLMParams.ToModelBuilderLLMParams().ResponseFormat,
	)
}

func literalLLMParam(name string, content any) *vo.Param {
	return &vo.Param{
		Name: name,
		Input: &vo.BlockInput{
			Type: vo.VariableTypeString,
			Value: &vo.BlockInputValue{
				Type:    vo.BlockInputValueTypeLiteral,
				Content: content,
			},
		},
	}
}

func TestTransformMessagePart(t *testing.T) {
	mockey.PatchConvey("TestTransformMessagePart", t, func() {
		tests := []struct {
			name                 string
			part                 schema.ChatMessagePart
			supportedModals      *developer_api.ModelAbility
			enableTransferBase64 bool
			expectedPart         schema.ChatMessagePart
			mockB64              bool
		}{
			{
				name: "Image modal not supported",
				part: schema.ChatMessagePart{
					Type:     schema.ChatMessagePartTypeImageURL,
					ImageURL: &schema.ChatMessageImageURL{URL: "http://example.com/image.png"},
				},
				supportedModals: &developer_api.ModelAbility{},
				expectedPart: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeText,
					Text: "http://example.com/image.png",
				},
			},
			{
				name: "Image modal supported, no base64 transfer",
				part: schema.ChatMessagePart{
					Type:     schema.ChatMessagePartTypeImageURL,
					ImageURL: &schema.ChatMessageImageURL{URL: "http://example.com/image.png"},
				},
				supportedModals: &developer_api.ModelAbility{ImageUnderstanding: ptr.Of(true), SupportMultiModal: ptr.Of(true)},
				expectedPart: schema.ChatMessagePart{
					Type:     schema.ChatMessagePartTypeImageURL,
					ImageURL: &schema.ChatMessageImageURL{URL: "http://example.com/image.png"},
				},
			},
			{
				name: "Audio modal not supported",
				part: schema.ChatMessagePart{
					Type:     schema.ChatMessagePartTypeAudioURL,
					AudioURL: &schema.ChatMessageAudioURL{URL: "http://example.com/audio.mp3"},
				},
				supportedModals: &developer_api.ModelAbility{},
				expectedPart: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeText,
					Text: "http://example.com/audio.mp3",
				},
			},
			{
				name: "Video modal not supported",
				part: schema.ChatMessagePart{
					Type:     schema.ChatMessagePartTypeVideoURL,
					VideoURL: &schema.ChatMessageVideoURL{URL: "http://example.com/video.mp4"},
				},
				supportedModals: &developer_api.ModelAbility{},
				expectedPart: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeText,
					Text: "http://example.com/video.mp4",
				},
			},
			{
				name: "File modal not supported",
				part: schema.ChatMessagePart{
					Type:    schema.ChatMessagePartTypeFileURL,
					FileURL: &schema.ChatMessageFileURL{URL: "http://example.com/file.txt"},
				},
				supportedModals: &developer_api.ModelAbility{},
				expectedPart: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeText,
					Text: "http://example.com/file.txt",
				},
			},
			{
				name: "Text part is unchanged",
				part: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeText,
					Text: "hello world",
				},
				supportedModals: &developer_api.ModelAbility{},
				expectedPart: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeText,
					Text: "hello world",
				},
			},
			{
				name: "Image modal supported, with base64 transfer",
				part: schema.ChatMessagePart{
					Type:     schema.ChatMessagePartTypeImageURL,
					ImageURL: &schema.ChatMessageImageURL{URL: "http://example.com/image.png"},
				},
				supportedModals:      &developer_api.ModelAbility{ImageUnderstanding: ptr.Of(true), SupportMultiModal: ptr.Of(true)},
				enableTransferBase64: true,
				expectedPart: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeImageURL,
					ImageURL: &schema.ChatMessageImageURL{
						URL:      "data:image/png;base64,base64encodedstring",
						MIMEType: "image/png",
					},
				},
				mockB64: true,
			},
			{
				name: "Audio modal supported, with base64 transfer",
				part: schema.ChatMessagePart{
					Type:     schema.ChatMessagePartTypeAudioURL,
					AudioURL: &schema.ChatMessageAudioURL{URL: "http://example.com/audio.mp3"},
				},
				supportedModals:      &developer_api.ModelAbility{AudioUnderstanding: ptr.Of(true), SupportMultiModal: ptr.Of(true)},
				enableTransferBase64: true,
				expectedPart: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeAudioURL,
					AudioURL: &schema.ChatMessageAudioURL{
						URL:      "data:audio/mpeg;base64,base64encodedstring",
						MIMEType: "audio/mpeg",
					},
				},
				mockB64: true,
			},
			{
				name: "Video modal supported, with base64 transfer",
				part: schema.ChatMessagePart{
					Type:     schema.ChatMessagePartTypeVideoURL,
					VideoURL: &schema.ChatMessageVideoURL{URL: "http://example.com/video.mp4"},
				},
				supportedModals:      &developer_api.ModelAbility{VideoUnderstanding: ptr.Of(true), SupportMultiModal: ptr.Of(true)},
				enableTransferBase64: true,
				expectedPart: schema.ChatMessagePart{
					Type: schema.ChatMessagePartTypeVideoURL,
					VideoURL: &schema.ChatMessageVideoURL{
						URL:      "data:video/mp4;base64,base64encodedstring",
						MIMEType: "video/mp4",
					},
				},
				mockB64: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if tt.mockB64 {
					t.Cleanup(mockey.UnPatchAll)
					var u, m string
					switch tt.part.Type {
					case schema.ChatMessagePartTypeImageURL:
						u, m = tt.expectedPart.ImageURL.URL, tt.expectedPart.ImageURL.MIMEType
					case schema.ChatMessagePartTypeAudioURL:
						u, m = tt.expectedPart.AudioURL.URL, tt.expectedPart.AudioURL.MIMEType
					case schema.ChatMessagePartTypeVideoURL:
						u, m = tt.expectedPart.VideoURL.URL, tt.expectedPart.VideoURL.MIMEType
					case schema.ChatMessagePartTypeFileURL:
						u, m = tt.expectedPart.FileURL.URL, tt.expectedPart.FileURL.MIMEType
					}
					mockey.Mock(urltobase64url.URLToBase64).Return(&urltobase64url.FileData{
						Base64Url: u,
						MimeType:  m,
					}, nil).Build()
				}
				result := transformMessagePart(tt.part, tt.supportedModals, tt.enableTransferBase64)
				assert.Equal(t, tt.expectedPart, result)
			})
		}
	})
}
