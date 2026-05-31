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

	chatapi "github.com/coze-dev/coze-studio/backend/api/model/workbench/chat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleMessageRejectsBlankMessageAsClientError(t *testing.T) {
	_, err := SVC.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		Message: " \t\n ",
		Mode:    chatapi.ChatMode_Auto,
	})

	require.Error(t, err)
	assert.True(t, IsClientError(err))
}

func TestHandleMessageRejectsInvalidModeAsClientError(t *testing.T) {
	_, err := SVC.HandleMessage(context.Background(), &chatapi.WorkbenchChatRequest{
		SpaceID: 1,
		Message: "hello",
		Mode:    chatapi.ChatMode(99),
	})

	require.Error(t, err)
	assert.True(t, IsClientError(err))
}
