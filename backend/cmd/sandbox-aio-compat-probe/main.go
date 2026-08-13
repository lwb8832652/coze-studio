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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	"github.com/coze-dev/coze-studio/backend/internal/sandboxrunner/aio"
)

const (
	probeTimeout = 90 * time.Second
	probeDir     = "/tmp/newx-aio-contract-probe"
)

type probeReport struct {
	Schema          string   `json:"schema"`
	AIOVersion      string   `json:"aio_version"`
	GoSDK           string   `json:"go_sdk"`
	NativeUIDGID    bool     `json:"native_uid_gid"`
	Capabilities    []string `json:"capabilities"`
	DurationMillis  int64    `json:"duration_millis"`
	ContainerMemory string   `json:"container_memory"`
	ContainerCPU    string   `json:"container_cpu"`
	ContainerPIDs   string   `json:"container_pids"`
}

type cancellationClient interface {
	Exec(context.Context, *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error)
	View(context.Context, *sandboxapi.ShellViewRequest) (*sandboxapi.ResponseShellViewResult, error)
	Wait(context.Context, *sandboxapi.ShellWaitRequest) (*sandboxapi.ResponseShellWaitResult, error)
	Kill(context.Context, *sandboxapi.ShellKillProcessRequest) (*sandboxapi.ResponseShellKillResult, error)
}

type cleanupClient interface {
	Cleanup(context.Context, string) error
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, safeProbeFailure(err))
		os.Exit(1)
	}
}

type probeFailure struct {
	stage  string
	reason string
}

func (failure *probeFailure) Error() string {
	return failure.reason
}

func newProbeFailure(stage string, err error) error {
	reason := aio.ReasonCode(err)
	if reason == "" {
		reason = "AIO_PROBE_CONTRACT_FAILED"
	}
	return &probeFailure{stage: stage, reason: reason}
}

func safeProbeFailure(err error) string {
	failure := &probeFailure{stage: "bootstrap", reason: "AIO_PROBE_CONTRACT_FAILED"}
	if errors.As(err, &failure) {
		return fmt.Sprintf("AIO compatibility probe failed: stage=%s reason=%s", failure.stage, failure.reason)
	}
	return "AIO compatibility probe failed: stage=bootstrap reason=AIO_PROBE_CONTRACT_FAILED"
}

func run() error {
	config, err := upstreamConfigFromEnvironment()
	if err != nil {
		return newProbeFailure("config", err)
	}
	client, err := aio.NewUpstreamClient(config)
	if err != nil {
		return newProbeFailure("client_create", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	started := time.Now()
	for _, sessionID := range []string{"newx-probe-a", "newx-probe-b"} {
		defer func(id string) { _ = client.Cleanup(context.Background(), id) }(sessionID)
		if _, err = client.Create(ctx, &sandboxapi.ShellCreateSessionRequest{Id: &sessionID, ExecDir: stringPointer("/tmp")}); err != nil {
			return newProbeFailure("shell_session_create", err)
		}
	}

	if err = execCompleted(ctx, client, "newx-probe-a", "rm -rf "+probeDir+" && mkdir -p "+probeDir+"/a "+probeDir+"/b"); err != nil {
		return newProbeFailure("shell_prepare", err)
	}
	var wait sync.WaitGroup
	errCh := make(chan error, 2)
	for _, session := range []struct{ id, marker string }{{"newx-probe-a", "alpha"}, {"newx-probe-b", "bravo"}} {
		wait.Add(1)
		go func(sessionID, marker string) {
			defer wait.Done()
			command := fmt.Sprintf("cd %s/%s && export NEWX_PROBE_MARKER=%s && printf 'marker=%s\\n' > marker.txt && test \"$NEWX_PROBE_MARKER\" = %s && test \"$(pwd)\" = %s/%s", probeDir, string(marker[0]), marker, marker, marker, probeDir, string(marker[0]))
			errCh <- execCompleted(ctx, client, sessionID, command)
		}(session.id, session.marker)
	}
	wait.Wait()
	close(errCh)
	for execErr := range errCh {
		if execErr != nil {
			return newProbeFailure("shell_concurrent_isolation", execErr)
		}
	}
	if err = execCompleted(ctx, client, "newx-probe-a", "grep -qx 'marker=alpha' "+probeDir+"/a/marker.txt && ! grep -qx 'marker=bravo' "+probeDir+"/a/marker.txt"); err != nil {
		return newProbeFailure("shell_cross_read", err)
	}

	if err = probeCancellation(ctx, client); err != nil {
		return err
	}
	if err = execCompleted(ctx, client, "newx-probe-b", "! pgrep -f '^sleep 30$'"); err != nil {
		return newProbeFailure("shell_cancel_cleanup", err)
	}

	filePath := probeDir + "/a/file-api.txt"
	utf8 := sandboxapi.FileContentEncodingUtf8
	if _, err = client.Write(ctx, &sandboxapi.FileWriteRequest{File: filePath, Content: "needle-old", Encoding: &utf8}); err != nil {
		return newProbeFailure("file_write", err)
	}
	read, err := client.Read(ctx, &sandboxapi.FileReadRequest{File: filePath})
	if err != nil || read == nil || read.Data == nil || read.Data.Content != "needle-old" {
		return newProbeFailure("file_read", err)
	}
	if _, err = client.List(ctx, &sandboxapi.FileListRequest{Path: probeDir + "/a"}); err != nil {
		return newProbeFailure("file_list", err)
	}
	if _, err = client.Glob(ctx, &sandboxapi.FileGlobRequest{Path: probeDir + "/a", Pattern: "*.txt"}); err != nil {
		return newProbeFailure("file_glob", err)
	}
	if _, err = client.Grep(ctx, &sandboxapi.FileGrepRequest{Path: probeDir + "/a", Pattern: "needle-old"}); err != nil {
		return newProbeFailure("file_grep", err)
	}
	if _, err = client.Replace(ctx, &sandboxapi.FileReplaceRequest{File: filePath, OldStr: "needle-old", NewStr: "needle-new"}); err != nil {
		return newProbeFailure("file_replace", err)
	}
	read, err = client.Read(ctx, &sandboxapi.FileReadRequest{File: filePath})
	if err != nil || read == nil || read.Data == nil || read.Data.Content != "needle-new" {
		return newProbeFailure("file_replace_verify", err)
	}
	if err = execCompleted(ctx, client, "newx-probe-b", "rm -rf "+probeDir); err != nil {
		return newProbeFailure("shell_probe_cleanup", err)
	}
	if err = probeCleanup(ctx, client); err != nil {
		return err
	}

	report := probeReport{
		Schema:          "newx.aio.compat-probe.v1",
		AIOVersion:      observedAIOVersion(),
		GoSDK:           "github.com/agent-infra/sandbox-sdk-go@v0.0.5",
		NativeUIDGID:    false,
		Capabilities:    []string{"shell_create", "shell_exec", "shell_view", "shell_wait", "shell_kill", "shell_cleanup", "file_read", "file_write", "file_list", "file_glob", "file_grep", "file_replace"},
		DurationMillis:  time.Since(started).Milliseconds(),
		ContainerMemory: strings.TrimSpace(os.Getenv("NEWX_AIO_CONTAINER_MEMORY")),
		ContainerCPU:    strings.TrimSpace(os.Getenv("NEWX_AIO_CONTAINER_CPU")),
		ContainerPIDs:   strings.TrimSpace(os.Getenv("NEWX_AIO_CONTAINER_PIDS")),
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(true)
	return encoder.Encode(report)
}

func probeCancellation(ctx context.Context, client cancellationClient) error {
	async := true
	execResult, err := client.Exec(ctx, &sandboxapi.ShellExecRequest{Id: stringPointer("newx-probe-a"), Command: "sleep 30", AsyncMode: &async})
	if err != nil || execResult == nil || execResult.Data == nil || execResult.Data.Status != sandboxapi.BashCommandStatusRunning {
		return newProbeFailure("shell_async_exec", err)
	}
	viewResult, err := client.View(ctx, &sandboxapi.ShellViewRequest{Id: "newx-probe-a"})
	if err != nil || viewResult == nil || viewResult.Data == nil || viewResult.Data.Status != sandboxapi.BashCommandStatusRunning {
		return newProbeFailure("shell_view", err)
	}
	waitSeconds := 1
	waitResult, err := client.Wait(ctx, &sandboxapi.ShellWaitRequest{Id: "newx-probe-a", Seconds: &waitSeconds, MaxWaitSeconds: &waitSeconds})
	if err != nil || waitResult == nil || waitResult.Data == nil || waitResult.Data.Status != sandboxapi.BashCommandStatusRunning {
		return newProbeFailure("shell_wait", err)
	}
	killResult, err := client.Kill(ctx, &sandboxapi.ShellKillProcessRequest{Id: "newx-probe-a"})
	if err != nil || killResult == nil || killResult.Data == nil || killResult.Data.Status != sandboxapi.BashCommandStatusTerminated {
		return newProbeFailure("shell_kill", err)
	}
	return nil
}

func probeCleanup(ctx context.Context, client cleanupClient) error {
	if err := client.Cleanup(ctx, "newx-probe-a"); err != nil && aio.ReasonCode(err) != aio.ReasonUpstreamNotFound {
		return newProbeFailure("shell_killed_session_cleanup", err)
	}
	if err := client.Cleanup(ctx, "newx-probe-b"); err != nil {
		return newProbeFailure("shell_session_cleanup", err)
	}
	return nil
}

func upstreamConfigFromEnvironment() (aio.UpstreamClientConfig, error) {
	baseURL := strings.TrimSpace(os.Getenv("NEWX_AIO_BASE_URL"))
	if baseURL == "" {
		return aio.UpstreamClientConfig{}, errors.New("NEWX_AIO_BASE_URL is required")
	}
	return aio.UpstreamClientConfig{
		BaseURL:    baseURL,
		BearerJWT:  strings.TrimSpace(os.Getenv("NEWX_AIO_JWT")),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func execCompleted(ctx context.Context, client *aio.UpstreamClient, sessionID, command string) error {
	async := false
	result, err := client.Exec(ctx, &sandboxapi.ShellExecRequest{Id: &sessionID, Command: command, AsyncMode: &async})
	if err != nil {
		return err
	}
	if result == nil || result.Data == nil || result.Data.Status != sandboxapi.BashCommandStatusCompleted || result.Data.ExitCode == nil || *result.Data.ExitCode != 0 {
		return errors.New("shell command contract failed")
	}
	return nil
}

func stringPointer(value string) *string { return &value }

func observedAIOVersion() string {
	return strings.TrimSpace(os.Getenv("NEWX_AIO_VERSION"))
}
