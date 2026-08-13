// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package aio

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSessionAdapterPreparesAndCreatesOnlyBoundWorkspace(t *testing.T) {
	client := newRecordingSessionUpstream()
	ref := sessionAdapterTestRef()
	adapter, err := NewSessionAdapter(ref, "opaque-shell-01", client,
		WithControlShellID("newx-generation-0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	require.Equal(t, ref, adapter.Ref())
	require.NoError(t, adapter.Prepare(context.Background()))
	require.NoError(t, adapter.Create(context.Background()))

	require.Len(t, client.execRequests, 1)
	prepare := client.execRequests[0]
	require.Equal(t, "newx-generation-0123456789abcdef0123456789abcdef", value(prepare.Id))
	require.Equal(t, "/mnt/user-data", value(prepare.ExecDir))
	require.Equal(t, true, value(prepare.Strict))
	require.Equal(t, false, value(prepare.PreserveSymlinks))
	require.NotNil(t, prepare.HardTimeout)
	require.Contains(t, prepare.Command, "mkdir -p --")
	for _, directory := range []string{
		"/mnt/user-data/42/43/thread-abc/workspace",
		"/mnt/user-data/42/43/thread-abc/uploads",
		"/mnt/user-data/42/43/thread-abc/outputs",
	} {
		require.Contains(t, prepare.Command, "'"+directory+"'")
	}
	for _, forbidden := range []string{"opaque-shell-01", "session-abc", "core", "operation"} {
		require.NotContains(t, prepare.Command, forbidden)
	}

	require.Len(t, client.createRequests, 1)
	created := client.createRequests[0]
	require.Equal(t, "opaque-shell-01", value(created.Id))
	require.Equal(t, "/mnt/user-data/42/43/thread-abc/workspace", value(created.ExecDir))
	require.Equal(t, false, value(created.PreserveSymlinks))
}

func TestSessionAdapterSerializesSharedControlShellAcrossAdapters(t *testing.T) {
	client := newRecordingSessionUpstream()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	client.execFunc = func(_ context.Context, request *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
		entered <- struct{}{}
		<-release
		return shellResponse(value(request.Id), "completed", "ok", 0), nil
	}
	controlID := "newx-generation-0123456789abcdef0123456789abcdef"
	first, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-first", client, WithControlShellID(controlID))
	require.NoError(t, err)
	second, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-second", client, WithControlShellID(controlID))
	require.NoError(t, err)
	done := make(chan error, 2)
	go func() { done <- first.Prepare(context.Background()) }()
	<-entered
	go func() { done <- second.Prepare(context.Background()) }()
	select {
	case <-entered:
		t.Fatal("second adapter entered the reserved control shell concurrently")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
}

func TestSessionAdapterExecMapsOnlyCWDAndBoundsResult(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	client := newRecordingSessionUpstream()
	client.execResponse = shellResponse("opaque-shell-01", "completed", strings.Repeat("x", 10), 7)
	adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-01", client,
		WithSessionClock(func() time.Time { return now }), WithAllowedEnvironmentNames("LANG"))
	require.NoError(t, err)

	stream, err := adapter.Exec(context.Background(), infrasandbox.ExecRequest{
		OperationID:    "operation-01",
		Command:        "printf /mnt/user-data/workspace/input.txt",
		CWD:            "/mnt/user-data/workspace/project",
		Deadline:       now.Add(30 * time.Second),
		MaxOutputBytes: 5,
	})
	require.NoError(t, err)
	require.Len(t, client.execRequests, 1)
	request := client.execRequests[0]
	require.Equal(t, "opaque-shell-01", value(request.Id))
	require.Equal(t, "/mnt/user-data/42/43/thread-abc/workspace/project", value(request.ExecDir))
	require.Equal(t, "printf /mnt/user-data/workspace/input.txt", request.Command,
		"logical command text must not be path-rewritten")
	require.Equal(t, true, value(request.Strict))
	require.Equal(t, false, value(request.PreserveSymlinks))
	require.Equal(t, false, value(request.AsyncMode))
	require.Equal(t, 30.0, value(request.HardTimeout))

	stdout, err := stream.Recv(context.Background())
	require.NoError(t, err)
	require.Equal(t, infrasandbox.ExecutionEventStdout, stdout.Kind)
	require.Equal(t, []byte("xxxxx"), stdout.Data)
	terminal, err := stream.Recv(context.Background())
	require.NoError(t, err)
	require.Equal(t, infrasandbox.ExecutionEventTerminal, terminal.Kind)
	require.Equal(t, 7, value(terminal.ExitCode))
	_, err = stream.Recv(context.Background())
	require.ErrorIs(t, err, io.EOF)
}

func TestSessionAdapterExecQuotesArgvWithoutRewritingArguments(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	client := newRecordingSessionUpstream()
	adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-01", client,
		WithSessionClock(func() time.Time { return now }), WithAllowedEnvironmentNames("LANG"))
	require.NoError(t, err)

	_, err = adapter.Exec(context.Background(), infrasandbox.ExecRequest{
		OperationID: "operation-argv",
		Argv:        []string{"python3", "/mnt/user-data/workspace/a b.py", "x'y"},
		Env:         map[string]string{"LANG": "C.UTF-8"},
		CWD:         "/mnt/user-data/workspace",
		Deadline:    now.Add(time.Minute), MaxOutputBytes: 1024,
	})
	require.NoError(t, err)
	require.Equal(t,
		"env 'LANG=C.UTF-8' 'python3' '/mnt/user-data/workspace/a b.py' 'x'\"'\"'y'",
		client.execRequests[0].Command)
	require.NotContains(t, client.execRequests[0].Command, "/mnt/user-data/42/43")
}

func TestSessionAdapterValidatesTrustedEnvironmentAllowlistAtConstruction(t *testing.T) {
	client := newRecordingSessionUpstream()
	for _, names := range [][]string{{"LANG", "LANG"}, {"LD-PRELOAD"}} {
		adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-01", client,
			WithAllowedEnvironmentNames(names...))
		require.Nil(t, adapter)
		require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
	}
}

func TestSessionAdapterSerializesSameShellAndKillsOnlyCancelledExec(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	client := newRecordingSessionUpstream()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	var concurrent atomic.Int32
	var maximum atomic.Int32
	client.execFunc = func(ctx context.Context, request *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
		current := concurrent.Add(1)
		defer concurrent.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		entered <- struct{}{}
		select {
		case <-release:
			return shellResponse(value(request.Id), "completed", "ok", 0), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-01", client,
		WithSessionClock(func() time.Time { return now }))
	require.NoError(t, err)
	request := infrasandbox.ExecRequest{OperationID: "op-a", Command: "pwd", CWD: "/mnt/user-data/workspace", Deadline: now.Add(time.Minute), MaxOutputBytes: 100}

	firstDone := make(chan error, 1)
	go func() { _, execErr := adapter.Exec(context.Background(), request); firstDone <- execErr }()
	<-entered
	secondDone := make(chan error, 1)
	request.OperationID = "op-b"
	go func() { _, execErr := adapter.Exec(context.Background(), request); secondDone <- execErr }()
	select {
	case <-entered:
		t.Fatal("second upstream exec entered before first completed")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	require.Equal(t, int32(1), maximum.Load())

	cancelClient := newRecordingSessionUpstream()
	cancelEntered := make(chan struct{})
	cancelClient.execFunc = func(ctx context.Context, _ *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
		close(cancelEntered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	cancelAdapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-cancel", cancelClient,
		WithSessionClock(func() time.Time { return now }))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	request.OperationID = "op-cancel"
	cancelDone := make(chan error, 1)
	go func() { _, execErr := cancelAdapter.Exec(ctx, request); cancelDone <- execErr }()
	<-cancelEntered
	cancel()
	require.Equal(t, ReasonUpstreamCancelled, ReasonCode(<-cancelDone))
	require.Equal(t, []string{"opaque-shell-cancel"}, cancelClient.killIDs)
}

func TestSessionAdapterSerializesSharedShellAcrossAdapters(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	client := newRecordingSessionUpstream()
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	client.execFunc = func(_ context.Context, request *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
		entered <- struct{}{}
		<-release
		return shellResponse(value(request.Id), "completed", "ok", 0), nil
	}
	first, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shared", client,
		WithSessionClock(func() time.Time { return now }))
	require.NoError(t, err)
	second, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shared", client,
		WithSessionClock(func() time.Time { return now }))
	require.NoError(t, err)
	request := infrasandbox.ExecRequest{
		OperationID: "shared-a", Command: "pwd", CWD: "/mnt/user-data/workspace",
		Deadline: now.Add(time.Minute), MaxOutputBytes: 100,
	}
	done := make(chan error, 2)
	go func() { _, execErr := first.Exec(context.Background(), request); done <- execErr }()
	<-entered
	request.OperationID = "shared-b"
	go func() { _, execErr := second.Exec(context.Background(), request); done <- execErr }()
	select {
	case <-entered:
		t.Fatal("second adapter entered the same upstream shell concurrently")
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
}

func TestSessionAdapterKillsCancelledExecEvenWhenUpstreamReturnsAResponse(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	client := newRecordingSessionUpstream()
	client.execFunc = func(ctx context.Context, request *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
		cancel, ok := ctx.Value(cancelAfterExecKey{}).(context.CancelFunc)
		require.True(t, ok)
		cancel()
		return shellResponse(value(request.Id), "completed", "ok", 0), nil
	}
	adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-01", client,
		WithSessionClock(func() time.Time { return now }))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, cancelAfterExecKey{}, context.CancelFunc(cancel))
	_, err = adapter.Exec(ctx, infrasandbox.ExecRequest{
		OperationID: "op-cancel-response-race", Command: "pwd", CWD: "/mnt/user-data/workspace",
		Deadline: now.Add(time.Minute), MaxOutputBytes: 100,
	})
	require.Equal(t, ReasonUpstreamCancelled, ReasonCode(err))
	require.Equal(t, []string{"opaque-shell-01"}, client.killIDs)
}

type cancelAfterExecKey struct{}

func TestSessionAdapterMapsFileRequestsAndReverseMapsResponses(t *testing.T) {
	client := newRecordingSessionUpstream()
	client.listResultCount = 1
	adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-01", client)
	require.NoError(t, err)
	ctx := context.Background()

	content, err := adapter.Read(ctx, infrasandbox.ReadRequest{Path: "/mnt/user-data/workspace/a.txt", MaxBytes: 20})
	require.NoError(t, err)
	require.Equal(t, []byte("read-body"), content.Data)
	require.Equal(t, "/mnt/user-data/42/43/thread-abc/workspace/a.txt", client.readRequests[0].File)
	require.Equal(t, false, value(client.readRequests[0].Sudo))

	require.NoError(t, adapter.Write(ctx, infrasandbox.WriteRequest{
		Path: "/mnt/user-data/uploads/raw.bin", Content: []byte{0, 1, 2, 0xff}, Append: true,
	}))
	write := client.writeRequests[0]
	require.Equal(t, "/mnt/user-data/42/43/thread-abc/uploads/raw.bin", write.File)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte{0, 1, 2, 0xff}), write.Content)
	require.Equal(t, sandboxapi.FileContentEncodingBase64, value(write.Encoding))
	require.Equal(t, true, value(write.Append))
	require.Equal(t, false, value(write.Sudo))

	entries, err := adapter.List(ctx, infrasandbox.ListRequest{Path: "/mnt/user-data/workspace", Limit: 1})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "/mnt/user-data/workspace/a.txt", entries[0].Path)
	require.Len(t, client.listRequests, 1)
	require.Equal(t, "/mnt/user-data/42/43/thread-abc/workspace", client.listRequests[0].Path)

	entries, err = adapter.Glob(ctx, infrasandbox.GlobRequest{Path: "/mnt/user-data/workspace", Pattern: "*.txt", Limit: 1})
	require.NoError(t, err)
	require.Equal(t, "/mnt/user-data/workspace/a.txt", entries[0].Path)
	require.Equal(t, 1, value(client.globRequests[0].MaxResults))

	matches, err := adapter.Grep(ctx, infrasandbox.GrepRequest{Path: "/mnt/skills", Pattern: "needle", Limit: 1})
	require.NoError(t, err)
	require.Len(t, matches, 1)
	require.Equal(t, "/mnt/skills/tool/SKILL.md", matches[0].Path)
	require.Equal(t, 9, matches[0].Line)
	require.Equal(t, "needle", matches[0].Text)
	require.Equal(t, "/mnt/skills", client.grepRequests[0].Path)

	require.NoError(t, adapter.Replace(ctx, infrasandbox.ReplaceRequest{
		Path: "/mnt/user-data/outputs/result.txt", Old: []byte("old"), New: []byte("new"),
	}))
	replace := client.replaceRequests[0]
	require.Equal(t, "/mnt/user-data/42/43/thread-abc/outputs/result.txt", replace.File)
	require.Equal(t, false, value(replace.Sudo))

	download, err := adapter.Download(ctx, infrasandbox.DownloadRequest{Path: "/mnt/skills/tool/SKILL.md", MaxBytes: 20})
	require.NoError(t, err)
	t.Cleanup(func() { _ = download.Close() })
	downloaded, err := io.ReadAll(download)
	require.NoError(t, err)
	require.Equal(t, []byte("download-body"), downloaded)
	require.Equal(t, "/mnt/skills/tool/SKILL.md", client.downloadPaths[0])
}

func TestSessionAdapterFailsClosedOnInvalidResponsesAndReadOnlySkills(t *testing.T) {
	client := newRecordingSessionUpstream()
	adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-01", client)
	require.NoError(t, err)

	err = adapter.Write(context.Background(), infrasandbox.WriteRequest{Path: "/mnt/skills/tool/SKILL.md", Content: []byte("bad")})
	require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
	require.Empty(t, client.writeRequests)

	client.readResponse = &sandboxapi.ResponseFileReadResult{Success: ptr(true), Data: &sandboxapi.FileReadResult{
		File: "/mnt/user-data/42/43/sibling/workspace/a.txt", Content: "secret",
	}}
	_, err = adapter.Read(context.Background(), infrasandbox.ReadRequest{Path: "/mnt/user-data/workspace/a.txt", MaxBytes: 20})
	require.Equal(t, ReasonAdapterContractInvalid, ReasonCode(err))
	for _, rendered := range []string{fmt.Sprint(err), fmt.Sprintf("%+v", err)} {
		require.NotContains(t, rendered, "/mnt/user-data/42/43")
		require.NotContains(t, rendered, "secret")
	}

	client.readResponse = &sandboxapi.ResponseFileReadResult{Success: ptr(true), Data: &sandboxapi.FileReadResult{
		File: "/mnt/user-data/42/43/thread-abc/workspace/a.txt", Content: "too-large",
	}}
	_, err = adapter.Read(context.Background(), infrasandbox.ReadRequest{Path: "/mnt/user-data/workspace/a.txt", MaxBytes: 3})
	require.Equal(t, ReasonAdapterResultTooLarge, ReasonCode(err))

	client.downloadBody = strings.NewReader("too-large")
	_, err = adapter.Download(context.Background(), infrasandbox.DownloadRequest{Path: "/mnt/skills/tool/SKILL.md", MaxBytes: 3})
	require.Equal(t, ReasonAdapterResultTooLarge, ReasonCode(err))
}

func TestSessionAdapterRejectsOversizedStructuredResultsInsteadOfTruncating(t *testing.T) {
	client := newRecordingSessionUpstream()
	adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell-01", client)
	require.NoError(t, err)

	_, err = adapter.List(context.Background(), infrasandbox.ListRequest{Path: "/mnt/user-data/workspace", Limit: 1})
	require.Equal(t, ReasonAdapterContractInvalid, ReasonCode(err))
}

func TestSessionAdapterRejectsInvalidConstructionAndMalformedCreateResponse(t *testing.T) {
	client := newRecordingSessionUpstream()
	for _, test := range []struct {
		name   string
		ref    domainsandbox.SessionRef
		shell  string
		client SessionUpstreamClient
	}{
		{name: "invalid ref", ref: domainsandbox.SessionRef{}, shell: "opaque-shell", client: client},
		{name: "missing shell", ref: sessionAdapterTestRef(), client: client},
		{name: "reserved business shell", ref: sessionAdapterTestRef(), shell: "newx-generation-0123456789abcdef0123456789abcdef", client: client},
		{name: "missing client", ref: sessionAdapterTestRef(), shell: "opaque-shell"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewSessionAdapter(test.ref, test.shell, test.client)
			require.ErrorIs(t, err, domainsandbox.ErrInvalidInput)
		})
	}

	client.createResponse = &sandboxapi.ResponseShellCreateSessionResponse{Success: ptr(true), Data: &sandboxapi.ShellCreateSessionResponse{
		SessionId: "wrong-shell", WorkingDir: "/mnt/user-data/42/43/thread-abc/workspace",
	}}
	adapter, err := NewSessionAdapter(sessionAdapterTestRef(), "opaque-shell", client)
	require.NoError(t, err)
	err = adapter.Create(context.Background())
	require.Equal(t, ReasonAdapterContractInvalid, ReasonCode(err))
}

func sessionAdapterTestRef() domainsandbox.SessionRef {
	return domainsandbox.SessionRef{
		SessionID: "session-abc",
		Key: domainsandbox.SessionKey{
			DeploymentID: "deployment-a", ProviderID: 12, SpaceID: 42, UserID: 43,
			ThreadID: "thread-abc", Profile: domainsandbox.SessionProfileCore,
		},
		RuntimeGeneration: 9,
	}
}

func shellResponse(sessionID, status, output string, exitCode int) *sandboxapi.ResponseShellCommandResult {
	return &sandboxapi.ResponseShellCommandResult{Success: ptr(true), Data: &sandboxapi.ShellCommandResult{
		SessionId: sessionID, Status: sandboxapi.BashCommandStatus(status), Output: ptr(output), ExitCode: ptr(exitCode),
	}}
}

type recordingSessionUpstream struct {
	mu sync.Mutex

	createRequests  []*sandboxapi.ShellCreateSessionRequest
	execRequests    []*sandboxapi.ShellExecRequest
	readRequests    []*sandboxapi.FileReadRequest
	writeRequests   []*sandboxapi.FileWriteRequest
	listRequests    []*sandboxapi.FileListRequest
	globRequests    []*sandboxapi.FileGlobRequest
	grepRequests    []*sandboxapi.FileGrepRequest
	replaceRequests []*sandboxapi.FileReplaceRequest
	downloadPaths   []string
	killIDs         []string

	createResponse  *sandboxapi.ResponseShellCreateSessionResponse
	execResponse    *sandboxapi.ResponseShellCommandResult
	readResponse    *sandboxapi.ResponseFileReadResult
	downloadBody    io.Reader
	listResultCount int
	execFunc        func(context.Context, *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error)
}

func (*recordingSessionUpstream) ShellLockIdentity() string { return "recording-session-upstream" }

func newRecordingSessionUpstream() *recordingSessionUpstream {
	root := "/mnt/user-data/42/43/thread-abc"
	return &recordingSessionUpstream{
		createResponse: &sandboxapi.ResponseShellCreateSessionResponse{Success: ptr(true), Data: &sandboxapi.ShellCreateSessionResponse{
			SessionId: "opaque-shell-01", WorkingDir: root + "/workspace",
		}},
		execResponse: shellResponse("opaque-shell-01", "completed", "ok", 0),
		readResponse: &sandboxapi.ResponseFileReadResult{Success: ptr(true), Data: &sandboxapi.FileReadResult{
			File: root + "/workspace/a.txt", Content: "read-body",
		}},
		downloadBody:    strings.NewReader("download-body"),
		listResultCount: 2,
	}
}

func (client *recordingSessionUpstream) Create(_ context.Context, request *sandboxapi.ShellCreateSessionRequest) (*sandboxapi.ResponseShellCreateSessionResponse, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	copyRequest := *request
	client.createRequests = append(client.createRequests, &copyRequest)
	response := client.createResponse
	if response != nil && response.Data != nil && response.Data.SessionId == "opaque-shell-01" && value(request.Id) != "opaque-shell-01" {
		clone := *response
		data := *response.Data
		data.SessionId = value(request.Id)
		clone.Data = &data
		response = &clone
	}
	return response, nil
}

func (client *recordingSessionUpstream) Exec(ctx context.Context, request *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
	client.mu.Lock()
	copyRequest := *request
	client.execRequests = append(client.execRequests, &copyRequest)
	fn := client.execFunc
	response := client.execResponse
	client.mu.Unlock()
	if fn != nil {
		return fn(ctx, request)
	}
	if response != nil && response.Data != nil && response.Data.SessionId == "opaque-shell-01" && value(request.Id) != "opaque-shell-01" {
		clone := *response
		data := *response.Data
		data.SessionId = value(request.Id)
		clone.Data = &data
		response = &clone
	}
	return response, nil
}

func (client *recordingSessionUpstream) View(context.Context, *sandboxapi.ShellViewRequest) (*sandboxapi.ResponseShellViewResult, error) {
	return nil, errors.New("unexpected View")
}
func (client *recordingSessionUpstream) Wait(context.Context, *sandboxapi.ShellWaitRequest) (*sandboxapi.ResponseShellWaitResult, error) {
	return nil, errors.New("unexpected Wait")
}
func (client *recordingSessionUpstream) Kill(_ context.Context, request *sandboxapi.ShellKillProcessRequest) (*sandboxapi.ResponseShellKillResult, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.killIDs = append(client.killIDs, request.Id)
	return &sandboxapi.ResponseShellKillResult{}, nil
}
func (client *recordingSessionUpstream) Cleanup(context.Context, string) error { return nil }

func (client *recordingSessionUpstream) Read(_ context.Context, request *sandboxapi.FileReadRequest) (*sandboxapi.ResponseFileReadResult, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	copyRequest := *request
	client.readRequests = append(client.readRequests, &copyRequest)
	return client.readResponse, nil
}

func (client *recordingSessionUpstream) Write(_ context.Context, request *sandboxapi.FileWriteRequest) (*sandboxapi.ResponseFileWriteResult, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	copyRequest := *request
	client.writeRequests = append(client.writeRequests, &copyRequest)
	written := len(request.Content)
	return &sandboxapi.ResponseFileWriteResult{Success: ptr(true), Data: &sandboxapi.FileWriteResult{File: request.File, BytesWritten: &written}}, nil
}

func (client *recordingSessionUpstream) List(_ context.Context, request *sandboxapi.FileListRequest) (*sandboxapi.ResponseFileListResult, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	copyRequest := *request
	client.listRequests = append(client.listRequests, &copyRequest)
	size := 9
	modified := "2026-08-13T12:00:00Z"
	files := []*sandboxapi.FileInfo{
		{Path: request.Path + "/a.txt", Size: &size, ModifiedTime: &modified},
		{Path: request.Path + "/b.txt", Size: &size, ModifiedTime: &modified},
	}
	files = files[:client.listResultCount]
	return &sandboxapi.ResponseFileListResult{Success: ptr(true), Data: &sandboxapi.FileListResult{Path: request.Path, Files: files}}, nil
}

func (client *recordingSessionUpstream) Glob(_ context.Context, request *sandboxapi.FileGlobRequest) (*sandboxapi.ResponseFileGlobResult, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	copyRequest := *request
	client.globRequests = append(client.globRequests, &copyRequest)
	size := 9
	modified := "2026-08-13T12:00:00Z"
	directory := false
	return &sandboxapi.ResponseFileGlobResult{Success: ptr(true), Data: &sandboxapi.FileGlobResult{
		Path: request.Path, Pattern: request.Pattern, Files: []*sandboxapi.GlobFileInfo{
			{Path: request.Path + "/a.txt", Size: &size, ModifiedTime: &modified, IsDirectory: &directory},
		},
	}}, nil
}

func (client *recordingSessionUpstream) Grep(_ context.Context, request *sandboxapi.FileGrepRequest) (*sandboxapi.ResponseFileGrepResult, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	copyRequest := *request
	client.grepRequests = append(client.grepRequests, &copyRequest)
	return &sandboxapi.ResponseFileGrepResult{Success: ptr(true), Data: &sandboxapi.FileGrepResult{
		Path: request.Path, Pattern: request.Pattern, Matches: []*sandboxapi.GrepMatch{
			{File: request.Path + "/tool/SKILL.md", LineNumber: 9, LineContent: "needle"},
		},
	}}, nil
}

func (client *recordingSessionUpstream) Replace(_ context.Context, request *sandboxapi.FileReplaceRequest) (*sandboxapi.ResponseFileReplaceResult, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	copyRequest := *request
	client.replaceRequests = append(client.replaceRequests, &copyRequest)
	count := 1
	return &sandboxapi.ResponseFileReplaceResult{Success: ptr(true), Data: &sandboxapi.FileReplaceResult{File: request.File, ReplacedCount: &count}}, nil
}

func (client *recordingSessionUpstream) Download(_ context.Context, path string) (io.Reader, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.downloadPaths = append(client.downloadPaths, path)
	return client.downloadBody, nil
}

func value[T any](pointer *T) T {
	if pointer == nil {
		var zero T
		return zero
	}
	return *pointer
}

var _ infrasandbox.SandboxSession = (*SessionAdapter)(nil)
var _ SessionUpstreamClient = (*recordingSessionUpstream)(nil)
