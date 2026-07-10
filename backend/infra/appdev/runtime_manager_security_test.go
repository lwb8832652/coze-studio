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

package appdev

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

func TestRuntimeManagerReapsIdleRuntime(t *testing.T) {
	now := time.Now().UTC()
	entry := &runtimeEntry{
		key:             "space:project",
		status:          domainappdev.RuntimeStatusRunning,
		previewURL:      "http://127.0.0.1:3000/",
		lastKeepAliveAt: now.Add(-3 * time.Minute),
	}
	manager := &RuntimeManager{
		entries: map[string]*runtimeEntry{entry.key: entry},
	}

	manager.reapExpiredRuntimes(now)

	require.Equal(t, domainappdev.RuntimeStatusStopped, entry.status)
	require.Empty(t, entry.previewURL)
	require.Contains(t, entry.message, "长时间未活动")
}

func TestTerminateRuntimeProcessForcesKillAfterGracePeriod(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' TERM; while :; do sleep 1; done")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	previousGrace := appDevRuntimeTerminateGrace
	appDevRuntimeTerminateGrace = 50 * time.Millisecond
	t.Cleanup(func() {
		appDevRuntimeTerminateGrace = previousGrace
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	})

	terminateRuntimeProcess(&runtimeEntry{cmd: cmd, done: done})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime process group was not force-killed")
	}
}

func TestRuntimeEntryRedactsSensitiveLogs(t *testing.T) {
	entry := &runtimeEntry{}
	entry.appendLog("error", "API_TOKEN=secret-value Authorization: Bearer abc.def.ghi")

	require.Len(t, entry.logs, 1)
	require.NotContains(t, entry.logs[0].Message, "secret-value")
	require.NotContains(t, entry.logs[0].Message, "abc.def.ghi")
	require.True(t, strings.Contains(entry.logs[0].Message, "[REDACTED]"))
}
