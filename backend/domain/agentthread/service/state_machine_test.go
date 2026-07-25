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
	"strings"
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
	assert.True(t, CanTransitionRun(entity.RunStatusRunning, entity.RunStatusInterrupted))
	assert.True(t, CanTransitionRun(entity.RunStatusInterrupted, entity.RunStatusQueued))
	assert.True(t, CanTransitionRun(entity.RunStatusInterrupted, entity.RunStatusFailed))
	assert.True(t, CanTransitionRun(entity.RunStatusInterrupted, entity.RunStatusCanceled))
}

func TestCanTransitionRunBlocksTerminalToRunning(t *testing.T) {
	assert.False(t, CanTransitionRun(entity.RunStatusSucceeded, entity.RunStatusRunning))
	assert.False(t, CanTransitionRun(entity.RunStatusFailed, entity.RunStatusRunning))
	assert.False(t, CanTransitionRun(entity.RunStatusInterrupted, entity.RunStatusSucceeded))
}

func TestEnsureRunTransitionReturnsClientError(t *testing.T) {
	err := EnsureRunTransition(entity.RunStatusSucceeded, entity.RunStatusRunning)

	require.Error(t, err)
	assert.True(t, IsClientError(err))
	assert.Contains(t, err.Error(), "cannot transition")
}

func TestAwaitingInputInteractionReferenceRequiresExplicitBoundedEventID(t *testing.T) {
	ref := entity.RunAwaitingInputInteractionRef{
		Schema:             entity.RunAwaitingInputInteractionRefSchema,
		InteractionEventID: "interrupt-event-1",
		InteractionID:      "hi_1",
		Kind:               "clarification",
	}

	require.NoError(t, ref.Validate())
	require.Error(t, entity.RunAwaitingInputInteractionRef{
		Schema:        entity.RunAwaitingInputInteractionRefSchema,
		InteractionID: "hi_1",
		Kind:          "clarification",
	}.Validate())
	require.NoError(t, entity.RunAwaitingInputInteractionRef{
		Schema:             entity.RunAwaitingInputInteractionRefSchema,
		InteractionEventID: strings.Repeat("a", 90),
	}.Validate())
	require.Error(t, entity.RunAwaitingInputInteractionRef{
		Schema:             entity.RunAwaitingInputInteractionRefSchema,
		InteractionEventID: strings.Repeat("a", 91),
	}.Validate())
	require.Error(t, entity.RunAwaitingInputInteractionRef{
		Schema:             entity.RunAwaitingInputInteractionRefSchema,
		InteractionEventID: "interrupt event 1",
	}.Validate())
}

func TestAwaitingInputInteractionReferenceParsesOnlyExactPayload(t *testing.T) {
	ref, ok := entity.RunAwaitingInputInteractionRefFromEventPayload(`{
		"status":"interrupted",
		"awaiting_input":{
			"schema":"coze.agentthread.awaiting_input.v1",
			"interaction_event_id":"interrupt-event-1",
			"interaction_id":"hi_1",
			"kind":"clarification"
		}
	}`)

	require.True(t, ok)
	require.Equal(t, "interrupt-event-1", ref.InteractionEventID)
	require.Equal(t, "hi_1", ref.InteractionID)

	_, ok = entity.RunAwaitingInputInteractionRefFromEventPayload(`{"status":"interrupted"}`)
	require.False(t, ok)

	_, ok = entity.RunAwaitingInputInteractionRefFromEventPayload(`{
		"status":"interrupted",
		"awaiting_input":{"interaction_event_id":"interrupt-event-1"}
	}`)
	require.False(t, ok)
}
