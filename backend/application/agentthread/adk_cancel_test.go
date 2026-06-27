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
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/stretchr/testify/require"
)

func TestADKCancelRegistryCancelsRegisteredExecution(t *testing.T) {
	registry := NewADKCancelRegistry()
	waiter := &recordingADKCancelWaiter{}
	var request adkCancelRequest
	cleanup := registry.register(20, func(got adkCancelRequest) (adkCancelWaiter, bool) {
		request = got
		return waiter, true
	})
	defer cleanup()

	err := registry.Cancel(
		context.Background(),
		20,
		adk.CancelAfterToolCalls|adk.CancelAfterChatModel,
		true,
	)

	require.NoError(t, err)
	require.Equal(t, adk.CancelAfterToolCalls|adk.CancelAfterChatModel, request.mode)
	require.True(t, request.recursive)
	require.True(t, waiter.waited)
}

func TestADKCancelRegistryStaleCleanupKeepsNewExecution(t *testing.T) {
	registry := NewADKCancelRegistry()
	firstCleanup := registry.register(20, func(adkCancelRequest) (adkCancelWaiter, bool) {
		return &recordingADKCancelWaiter{}, true
	})
	secondCalled := false
	secondCleanup := registry.register(20, func(adkCancelRequest) (adkCancelWaiter, bool) {
		secondCalled = true
		return &recordingADKCancelWaiter{}, true
	})
	defer secondCleanup()

	firstCleanup()
	err := registry.Cancel(context.Background(), 20, adk.CancelImmediate, false)

	require.NoError(t, err)
	require.True(t, secondCalled)
}

func TestADKCancelRegistryReportsInactiveRun(t *testing.T) {
	registry := NewADKCancelRegistry()

	err := registry.Cancel(context.Background(), 20, adk.CancelImmediate, false)

	require.ErrorIs(t, err, ErrADKRunNotActive)
}

type recordingADKCancelWaiter struct {
	waited bool
	err    error
}

func (w *recordingADKCancelWaiter) Wait() error {
	w.waited = true
	return w.err
}
