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

	"github.com/coze-dev/coze-studio/backend/domain/task/entity"
)

func TestCanTransitionAllowsHappyPath(t *testing.T) {
	assert.True(t, CanTransition(entity.StatusCreated, entity.StatusQueued))
	assert.True(t, CanTransition(entity.StatusQueued, entity.StatusRunning))
	assert.True(t, CanTransition(entity.StatusRunning, entity.StatusSucceeded))
}

func TestCanTransitionBlocksTerminalToRunning(t *testing.T) {
	assert.False(t, CanTransition(entity.StatusSucceeded, entity.StatusRunning))
	assert.False(t, CanTransition(entity.StatusCanceled, entity.StatusRunning))
}

func TestEnsureTransitionReturnsClearError(t *testing.T) {
	err := EnsureTransition(entity.StatusSucceeded, entity.StatusQueued)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot transition")
}
