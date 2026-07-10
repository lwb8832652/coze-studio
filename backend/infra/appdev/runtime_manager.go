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
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	appdevapp "github.com/coze-dev/coze-studio/backend/application/appdev"
	domainappdev "github.com/coze-dev/coze-studio/backend/domain/appdev"
)

type RuntimeManager struct {
	mu        sync.Mutex
	entries   map[string]*runtimeEntry
	stopCh    chan struct{}
	closeOnce sync.Once
}

type runtimeEntry struct {
	key             string
	status          domainappdev.RuntimeStatus
	previewURL      string
	message         string
	lastKeepAliveAt time.Time
	cmd             *exec.Cmd
	cancel          context.CancelFunc
	done            chan struct{}
	logs            []*domainappdev.RuntimeLog
}

var (
	appDevRuntimeIdleTimeout    = 2 * time.Minute
	appDevRuntimeReaperInterval = 30 * time.Second
	appDevRuntimeTerminateGrace = 3 * time.Second
	appDevRuntimeEntryRetention = 15 * time.Minute
)

func NewRuntimeManager() *RuntimeManager {
	manager := &RuntimeManager{
		entries: map[string]*runtimeEntry{},
		stopCh:  make(chan struct{}),
	}
	go manager.runReaper()
	return manager
}

func (m *RuntimeManager) Start(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	return m.start(ctx, req, domainappdev.RuntimeStatusStarting)
}

func (m *RuntimeManager) start(ctx context.Context, req *appdevapp.RuntimeManagerRequest, initialStatus domainappdev.RuntimeStatus) (*domainappdev.RuntimeInfo, error) {
	key := runtimeKey(req)
	m.reapExpiredRuntimes(time.Now().UTC())

	m.mu.Lock()
	if existing := m.entries[key]; existing != nil &&
		(existing.status == domainappdev.RuntimeStatusRunning ||
			existing.status == domainappdev.RuntimeStatusStarting ||
			existing.status == domainappdev.RuntimeStatusRestarting) {
		info := existing.info()
		m.mu.Unlock()
		return info, nil
	}
	m.mu.Unlock()

	port, err := reservePort()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(filepath.Join(req.ProjectDir, "package.json")); err != nil {
		entry := &runtimeEntry{
			key:             key,
			status:          domainappdev.RuntimeStatusError,
			message:         "项目缺少 package.json，无法启动开发环境",
			lastKeepAliveAt: time.Now().UTC(),
		}
		entry.appendLog("error", "missing package.json")
		m.setEntry(key, entry)
		return entry.info(), nil
	}

	processCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(
		processCtx,
		"sh",
		"-c",
		runtimeStartCommand(port),
	)
	cmd.Dir = req.ProjectDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}

	entry := &runtimeEntry{
		key:             key,
		status:          initialStatus,
		previewURL:      fmt.Sprintf("http://127.0.0.1:%d/", port),
		message:         runtimeStartingMessage(initialStatus),
		cancel:          cancel,
		done:            make(chan struct{}),
		lastKeepAliveAt: time.Now().UTC(),
	}
	entry.appendLog("info", "starting appdev runtime")

	if err := cmd.Start(); err != nil {
		cancel()
		entry.status = domainappdev.RuntimeStatusError
		entry.message = err.Error()
		entry.previewURL = ""
		entry.appendLog("error", err.Error())
		m.setEntry(key, entry)
		return entry.info(), nil
	}

	entry.cmd = cmd
	entry.appendLog("info", "development server process started")
	info := entry.info()
	m.setEntry(key, entry)

	go m.scanLogs(key, stdout, "info")
	go m.scanLogs(key, stderr, "error")
	go m.waitRuntimeReady(processCtx, key, entry.previewURL)
	go m.waitProcess(key, cmd, entry.done)

	return info, nil
}

func (m *RuntimeManager) Status(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.entries[runtimeKey(req)]
	if entry == nil {
		return &domainappdev.RuntimeInfo{
			Status:  domainappdev.RuntimeStatusStopped,
			Message: "开发环境未启动",
		}, nil
	}

	return entry.info(), nil
}

func (m *RuntimeManager) KeepAlive(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	key := runtimeKey(req)

	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.entries[key]
	if entry == nil {
		return &domainappdev.RuntimeInfo{
			Status:  domainappdev.RuntimeStatusStopped,
			Message: "开发环境未启动",
		}, nil
	}

	entry.lastKeepAliveAt = time.Now().UTC()
	entry.appendLog("debug", "keep alive")
	return entry.info(), nil
}

func (m *RuntimeManager) Restart(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	if _, err := m.Stop(ctx, req); err != nil {
		return nil, err
	}
	return m.start(ctx, req, domainappdev.RuntimeStatusRestarting)
}

func (m *RuntimeManager) Stop(ctx context.Context, req *appdevapp.RuntimeManagerRequest) (*domainappdev.RuntimeInfo, error) {
	key := runtimeKey(req)

	m.mu.Lock()
	entry := m.entries[key]
	if entry == nil {
		m.mu.Unlock()
		return &domainappdev.RuntimeInfo{
			Status:  domainappdev.RuntimeStatusStopped,
			Message: "开发环境未启动",
		}, nil
	}
	entry.status = domainappdev.RuntimeStatusStopped
	entry.message = "开发环境已停止"
	entry.previewURL = ""
	entry.lastKeepAliveAt = time.Now().UTC()
	entry.appendLog("info", "development server stopped")
	info := entry.info()
	m.mu.Unlock()
	terminateRuntimeProcess(entry)

	return info, nil
}

func (m *RuntimeManager) Logs(ctx context.Context, req *appdevapp.RuntimeManagerRequest) ([]*domainappdev.RuntimeLog, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry := m.entries[runtimeKey(req)]
	if entry == nil {
		return []*domainappdev.RuntimeLog{}, nil
	}

	logs := make([]*domainappdev.RuntimeLog, len(entry.logs))
	copy(logs, entry.logs)
	return logs, nil
}

func (m *RuntimeManager) setEntry(key string, entry *runtimeEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[key] = entry
}

func (m *RuntimeManager) getEntry(key string) *runtimeEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.entries[key]
}

func (m *RuntimeManager) scanLogs(key string, reader io.Reader, level string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		m.mu.Lock()
		if entry := m.entries[key]; entry != nil {
			entry.appendLog(level, scanner.Text())
		}
		m.mu.Unlock()
	}
}

func (m *RuntimeManager) waitProcess(key string, cmd *exec.Cmd, done chan struct{}) {
	err := cmd.Wait()
	close(done)

	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.entries[key]
	if entry == nil || entry.cmd != cmd || entry.status == domainappdev.RuntimeStatusStopped {
		return
	}
	if entry.status == domainappdev.RuntimeStatusError {
		return
	}

	if err != nil {
		entry.status = domainappdev.RuntimeStatusError
		entry.message = err.Error()
		entry.previewURL = ""
		entry.appendLog("error", err.Error())
		return
	}

	entry.status = domainappdev.RuntimeStatusStopped
	entry.message = "开发环境已退出"
	entry.previewURL = ""
	entry.appendLog("info", "development server exited")
}

func (m *RuntimeManager) waitRuntimeReady(ctx context.Context, key string, previewURL string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	timeout := time.NewTimer(10 * time.Minute)
	defer timeout.Stop()

	client := http.Client{Timeout: 800 * time.Millisecond}
	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout.C:
			m.markRuntimeReadyTimeout(key, previewURL)
			return
		case <-ticker.C:
			request, err := http.NewRequestWithContext(ctx, http.MethodGet, previewURL, nil)
			if err != nil {
				return
			}
			response, err := client.Do(request)
			if err != nil {
				continue
			}
			_ = response.Body.Close()
			if response.StatusCode >= http.StatusInternalServerError {
				continue
			}

			m.mu.Lock()
			if entry := m.entries[key]; entry != nil &&
				entry.previewURL == previewURL &&
				(entry.status == domainappdev.RuntimeStatusStarting || entry.status == domainappdev.RuntimeStatusRestarting) {
				entry.status = domainappdev.RuntimeStatusRunning
				entry.message = "开发环境运行中"
				entry.appendLog("info", "preview server is ready")
			}
			m.mu.Unlock()
			return
		}
	}
}

func (m *RuntimeManager) markRuntimeReadyTimeout(key string, previewURL string) {
	m.mu.Lock()

	entry := m.entries[key]
	if entry == nil ||
		entry.previewURL != previewURL ||
		(entry.status != domainappdev.RuntimeStatusStarting &&
			entry.status != domainappdev.RuntimeStatusRestarting) {
		m.mu.Unlock()
		return
	}

	entry.status = domainappdev.RuntimeStatusError
	entry.previewURL = ""
	entry.message = "开发环境启动超时，请查看日志后重试"
	entry.lastKeepAliveAt = time.Now().UTC()
	entry.appendLog("error", "preview server readiness probe timed out")
	m.mu.Unlock()
	terminateRuntimeProcess(entry)
}

func (e *runtimeEntry) info() *domainappdev.RuntimeInfo {
	return &domainappdev.RuntimeInfo{
		Status:          e.status,
		PreviewURL:      e.previewURL,
		Message:         e.message,
		LastKeepAliveAt: e.lastKeepAliveAt,
	}
}

func (e *runtimeEntry) appendLog(level string, message string) {
	trimmed := strings.TrimSpace(appdevapp.SanitizeAppDevOutput(message))
	if trimmed == "" {
		return
	}

	e.logs = append(e.logs, &domainappdev.RuntimeLog{
		ID:        fmt.Sprintf("%d", time.Now().UnixNano()),
		Level:     level,
		Message:   trimmed,
		Timestamp: time.Now().UTC(),
	})

	if len(e.logs) > 1000 {
		e.logs = e.logs[len(e.logs)-1000:]
	}
}

func (m *RuntimeManager) runReaper() {
	ticker := time.NewTicker(appDevRuntimeReaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.reapExpiredRuntimes(time.Now().UTC())
		case <-m.stopCh:
			return
		}
	}
}

func (m *RuntimeManager) reapExpiredRuntimes(now time.Time) {
	toTerminate := make([]*runtimeEntry, 0)

	m.mu.Lock()
	for key, entry := range m.entries {
		if entry == nil {
			delete(m.entries, key)
			continue
		}
		active := entry.status == domainappdev.RuntimeStatusStarting ||
			entry.status == domainappdev.RuntimeStatusRestarting ||
			entry.status == domainappdev.RuntimeStatusRunning
		if active && !entry.lastKeepAliveAt.IsZero() && now.Sub(entry.lastKeepAliveAt) > appDevRuntimeIdleTimeout {
			entry.status = domainappdev.RuntimeStatusStopped
			entry.previewURL = ""
			entry.message = "开发环境长时间未活动，已自动停止"
			entry.lastKeepAliveAt = now
			entry.appendLog("info", "runtime stopped after inactivity timeout")
			toTerminate = append(toTerminate, entry)
			continue
		}
		if !active && !entry.lastKeepAliveAt.IsZero() && now.Sub(entry.lastKeepAliveAt) > appDevRuntimeEntryRetention {
			delete(m.entries, key)
		}
	}
	m.mu.Unlock()

	for _, entry := range toTerminate {
		terminateRuntimeProcess(entry)
	}
}

func (m *RuntimeManager) Close() {
	m.closeOnce.Do(func() {
		if m.stopCh != nil {
			close(m.stopCh)
		}

		m.mu.Lock()
		entries := make([]*runtimeEntry, 0, len(m.entries))
		for _, entry := range m.entries {
			if entry != nil {
				entry.status = domainappdev.RuntimeStatusStopped
				entry.previewURL = ""
				entries = append(entries, entry)
			}
		}
		m.mu.Unlock()

		for _, entry := range entries {
			terminateRuntimeProcess(entry)
		}
	})
}

func terminateRuntimeProcess(entry *runtimeEntry) {
	if entry == nil {
		return
	}
	if entry.cmd == nil || entry.cmd.Process == nil {
		if entry.cancel != nil {
			entry.cancel()
		}
		return
	}

	pid := entry.cmd.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	timer := time.NewTimer(appDevRuntimeTerminateGrace)
	defer timer.Stop()

	if entry.done != nil {
		select {
		case <-entry.done:
			if entry.cancel != nil {
				entry.cancel()
			}
			return
		case <-timer.C:
		}
	} else {
		<-timer.C
	}

	_ = syscall.Kill(-pid, syscall.SIGKILL)
	if entry.cancel != nil {
		entry.cancel()
	}
}

func runtimeKey(req *appdevapp.RuntimeManagerRequest) string {
	return req.SpaceID + ":" + req.ProjectID
}

func reservePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	return listener.Addr().(*net.TCPAddr).Port, nil
}

func runtimeStartingMessage(status domainappdev.RuntimeStatus) string {
	if status == domainappdev.RuntimeStatusRestarting {
		return "开发环境重启中，正在重新准备预览服务"
	}

	return "开发环境启动中，首次启动会自动安装依赖"
}

func runtimeStartCommand(port int) string {
	return fmt.Sprintf(`set -e
if [ ! -d node_modules ]; then
  echo "installing project dependencies"
  npm install --ignore-scripts --package-lock=false --no-audit --no-fund --prefer-offline --loglevel=warn
fi
if [ ! -x ./node_modules/.bin/vite ]; then
  echo "installing appdev preview dependencies"
  npm install --ignore-scripts --package-lock=false --no-audit --no-fund --prefer-offline --loglevel=warn vite@^5.4.14 react@^18.2.0 react-dom@^18.2.0 typescript@^5.8.2
fi
mkdir -p .coze-appdev
cat > .coze-appdev/vite.config.mjs <<'EOF'
import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    host: '127.0.0.1',
  },
});
EOF
echo "starting vite preview server"
./node_modules/.bin/vite --config .coze-appdev/vite.config.mjs --host 127.0.0.1 --port %d`, port)
}
