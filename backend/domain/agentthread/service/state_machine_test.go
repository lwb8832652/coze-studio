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

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
)

func TestCanTransitionRunAllowsWorkerLifecycle(t *testing.T) {
	assert.True(t, CanTransitionRun(entity.RunStatusPending, entity.RunStatusRunning))
	assert.True(t, CanTransitionRun(entity.RunStatusRunning, entity.RunStatusSucceeded))
	assert.True(t, CanTransitionRun(entity.RunStatusRunning, entity.RunStatusFailed))
	assert.True(t, CanTransitionRun(entity.RunStatusRunning, entity.RunStatusCanceled))
}

func TestCanTransitionRunBlocksTerminalToRunning(t *testing.T) {
	assert.False(t, CanTransitionRun(entity.RunStatusSucceeded, entity.RunStatusRunning))
	assert.False(t, CanTransitionRun(entity.RunStatusFailed, entity.RunStatusRunning))
}

func TestEnsureRunTransitionReturnsClientError(t *testing.T) {
	err := EnsureRunTransition(entity.RunStatusSucceeded, entity.RunStatusRunning)

	require.Error(t, err)
	assert.True(t, IsClientError(err))
	assert.Contains(t, err.Error(), "cannot transition")
}
