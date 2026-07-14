// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package entity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskStateTransitions(t *testing.T) {
	t.Parallel()

	task := &Task{Status: StatusEnabled}
	require.NoError(t, task.Disable())
	require.Equal(t, StatusDisabled, task.Status)
	require.NoError(t, task.Enable())
	require.Equal(t, StatusEnabled, task.Status)
	require.NoError(t, task.Complete())
	require.Equal(t, StatusCompleted, task.Status)
	require.Error(t, task.Enable())
}
