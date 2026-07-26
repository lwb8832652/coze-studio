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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestDomainThreadToSummaryKeepsTaskWording(t *testing.T) {
	thread := &entity.Thread{
		ID:        200,
		SpaceID:   1,
		CreatorID: 2,
		Title:     "生成方案",
		Status:    entity.ThreadStatusRunning,
		Source:    entity.ThreadSourceIM,
		CreatedAt: 1717000000000,
		UpdatedAt: 1717000300000,
	}

	summary := DomainThreadToSummary(thread)

	require.NotNil(t, summary)
	require.Equal(t, int64(200), summary.ThreadID)
	require.Equal(t, int64(1), summary.SpaceID)
	require.Equal(t, int64(2), summary.CreatorID)
	require.Equal(t, "生成方案", summary.Title)
	require.Equal(t, ThreadStatusRunning, summary.Status)
	require.Equal(t, ThreadSourceIM, summary.Source)
	require.Equal(t, int64(1717000000000), summary.CreatedAt)
	require.Equal(t, int64(1717000300000), summary.UpdatedAt)
}

func TestDomainThreadToSummaryReturnsNilForNilThread(t *testing.T) {
	require.Nil(t, DomainThreadToSummary(nil))
}
