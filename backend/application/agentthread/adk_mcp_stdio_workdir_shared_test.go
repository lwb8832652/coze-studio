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

	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

func TestADKMCPRuntimeStdioFilesystemWorkdirPreparerSharesLockedManager(t *testing.T) {
	root := canonicalADKMCPWorkdirTestRoot(t)
	manager, err := mcpruntime.NewSafeWorkdirManager(mcpruntime.SafeWorkdirOptions{Root: root})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })

	first := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root, Manager: manager},
	)
	second := NewADKMCPRuntimeStdioFilesystemWorkdirPreparer(
		ADKMCPRuntimeStdioFilesystemWorkdirPreparerOptions{Root: root, Manager: manager},
	)

	require.True(t, first.Valid())
	require.True(t, second.Valid())
	_, err = mcpruntime.NewSafeWorkdirManager(mcpruntime.SafeWorkdirOptions{Root: root})
	require.ErrorIs(t, err, mcpruntime.ErrSafeWorkdirRootLocked)
}
