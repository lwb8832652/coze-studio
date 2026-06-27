/*
 * Copyright 2026 coze-dev Authors
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

package agentflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	knowledgeModel "github.com/coze-dev/coze-studio/backend/crossdomain/knowledge/model"
	knowledgeEntity "github.com/coze-dev/coze-studio/backend/domain/knowledge/entity"
)

func TestNewKnowledgeToolPreservesDynamicKnowledgeSchema(t *testing.T) {
	ctx := context.Background()
	tool, err := newKnowledgeTool(ctx, &knowledgeConfig{
		knowledgeInfos: []*knowledgeEntity.Knowledge{
			{
				Knowledge: &knowledgeModel.Knowledge{
					Info: knowledgeModel.Info{
						ID:          101,
						Name:        "Product handbook",
						Description: "Product policies and operating procedures.",
					},
				},
			},
			{
				Knowledge: &knowledgeModel.Knowledge{
					Info: knowledgeModel.Info{
						ID:          202,
						Name:        "Support playbook",
						Description: "Customer support troubleshooting guidance.",
					},
				},
			},
		},
	})
	require.NoError(t, err)

	info, err := tool.Info(ctx)
	require.NoError(t, err)
	params, err := info.ParamsOneOf.ToJSONSchema()
	require.NoError(t, err)

	knowledgeIDs, ok := params.Properties.Get("knowledge_ids")
	require.True(t, ok)
	require.Equal(t, "array", knowledgeIDs.Type)
	require.NotNil(t, knowledgeIDs.Items)
	require.Equal(t, "integer", knowledgeIDs.Items.Type)
	require.Contains(t, knowledgeIDs.Description, "101: Product handbook")
	require.Contains(t, knowledgeIDs.Description, "202: Support playbook")
	require.Equal(t, []any{"101", "202"}, knowledgeIDs.Enum)
}
