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

package application

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/application/agentthread"
	"github.com/coze-dev/coze-studio/backend/application/mcpruntime"
)

func TestMCPManagementRuntimeSharedWorkdirLifecycleOwnsSingleRootLock(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.Chmod(root, 0o700))
	manager, preparer, err := newMCPRuntimeSharedWorkdir(agentthread.ADKMCPRuntimeBootstrapConfig{
		Enabled: true, StdioDryRunEnabled: true, StdioWorkdirRoot: root,
	})
	require.NoError(t, err)
	require.NotNil(t, manager)
	require.True(t, preparer.Valid())

	_, err = mcpruntime.NewSafeWorkdirManager(mcpruntime.SafeWorkdirOptions{Root: root})
	require.ErrorIs(t, err, mcpruntime.ErrSafeWorkdirRootLocked)
	require.NoError(t, (&mcpManagementRuntimeLifecycle{workdirManager: manager}).Shutdown(context.Background()))

	reopened, err := mcpruntime.NewSafeWorkdirManager(mcpruntime.SafeWorkdirOptions{Root: root})
	require.NoError(t, err)
	require.NoError(t, reopened.Close())
}
