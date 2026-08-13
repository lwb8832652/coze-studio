// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package aio

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"hash/fnv"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	infrasandbox "github.com/coze-dev/coze-studio/backend/infra/sandbox"
)

const (
	ReasonAdapterContractInvalid = "AIO_ADAPTER_CONTRACT_INVALID"
	ReasonAdapterResultTooLarge  = "AIO_ADAPTER_RESULT_TOO_LARGE"
)

type SessionUpstreamClient interface {
	Create(context.Context, *sandboxapi.ShellCreateSessionRequest) (*sandboxapi.ResponseShellCreateSessionResponse, error)
	Exec(context.Context, *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error)
	View(context.Context, *sandboxapi.ShellViewRequest) (*sandboxapi.ResponseShellViewResult, error)
	Wait(context.Context, *sandboxapi.ShellWaitRequest) (*sandboxapi.ResponseShellWaitResult, error)
	Kill(context.Context, *sandboxapi.ShellKillProcessRequest) (*sandboxapi.ResponseShellKillResult, error)
	Cleanup(context.Context, string) error
	Read(context.Context, *sandboxapi.FileReadRequest) (*sandboxapi.ResponseFileReadResult, error)
	Write(context.Context, *sandboxapi.FileWriteRequest) (*sandboxapi.ResponseFileWriteResult, error)
	List(context.Context, *sandboxapi.FileListRequest) (*sandboxapi.ResponseFileListResult, error)
	Glob(context.Context, *sandboxapi.FileGlobRequest) (*sandboxapi.ResponseFileGlobResult, error)
	Grep(context.Context, *sandboxapi.FileGrepRequest) (*sandboxapi.ResponseFileGrepResult, error)
	Replace(context.Context, *sandboxapi.FileReplaceRequest) (*sandboxapi.ResponseFileReplaceResult, error)
	Download(context.Context, string) (io.Reader, error)
}

type SessionAdapterOption func(*sessionAdapterOptions) error

type ShellLockIdentityProvider interface {
	ShellLockIdentity() string
}

type sessionAdapterOptions struct {
	controlShellID  string
	clock           func() time.Time
	allowedEnvNames []string
}

func WithControlShellID(shellID string) SessionAdapterOption {
	return func(options *sessionAdapterOptions) error {
		options.controlShellID = shellID
		return nil
	}
}

func WithSessionClock(clock func() time.Time) SessionAdapterOption {
	return func(options *sessionAdapterOptions) error {
		options.clock = clock
		return nil
	}
}

func WithAllowedEnvironmentNames(names ...string) SessionAdapterOption {
	return func(options *sessionAdapterOptions) error {
		options.allowedEnvNames = append([]string(nil), names...)
		return nil
	}
}

type SessionAdapter struct {
	ref             domainsandbox.SessionRef
	upstreamShellID string
	upstream        SessionUpstreamClient
	mapper          *WorkspaceMapper
	options         sessionAdapterOptions
	businessShellMu *sync.Mutex
	controlShellMu  *sync.Mutex
}

var processShellLocks [64]sync.Mutex

func NewSessionAdapter(ref domainsandbox.SessionRef, upstreamShellID string, upstream SessionUpstreamClient, options ...SessionAdapterOption) (*SessionAdapter, error) {
	normalizedRef, err := domainsandbox.NormalizeSessionRef(ref)
	if err != nil || normalizedRef != ref || !validBusinessShellID(upstreamShellID) || upstream == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	mapper, err := NewWorkspaceMapper(ref)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	configured := sessionAdapterOptions{clock: time.Now}
	for _, option := range options {
		if option == nil || option(&configured) != nil {
			return nil, domainsandbox.ErrInvalidInput
		}
	}
	if configured.clock == nil || configured.controlShellID != "" && !validControlShellID(configured.controlShellID) {
		return nil, domainsandbox.ErrInvalidInput
	}
	if !validAllowedEnvironmentNames(configured.allowedEnvNames) {
		return nil, domainsandbox.ErrInvalidInput
	}
	lockProvider, ok := upstream.(ShellLockIdentityProvider)
	if !ok || !validShellLockIdentity(lockProvider.ShellLockIdentity()) {
		return nil, domainsandbox.ErrInvalidInput
	}
	adapter := &SessionAdapter{
		ref: ref, upstreamShellID: upstreamShellID, upstream: upstream, mapper: mapper, options: configured,
		businessShellMu: sharedShellMutex(lockProvider.ShellLockIdentity(), upstreamShellID),
	}
	if configured.controlShellID != "" {
		adapter.controlShellMu = sharedShellMutex(lockProvider.ShellLockIdentity(), configured.controlShellID)
	}
	return adapter, nil
}

func sharedShellMutex(upstreamIdentity, shellID string) *sync.Mutex {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(upstreamIdentity))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(shellID))
	return &processShellLocks[hasher.Sum64()%uint64(len(processShellLocks))]
}

func validShellLockIdentity(value string) bool {
	return value != "" && len(value) <= 2048 && !strings.ContainsAny(value, "\x00\r\n")
}

func validAllowedEnvironmentNames(names []string) bool {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" || len(name) > infrasandbox.MaxEnvKeyBytes || !isASCIIEnvironmentName(name) {
			return false
		}
		if _, duplicate := seen[name]; duplicate {
			return false
		}
		seen[name] = struct{}{}
	}
	return true
}

func isASCIIEnvironmentName(value string) bool {
	for index := range value {
		character := value[index]
		if index == 0 && !asciiLetter(character) && character != '_' {
			return false
		}
		if index > 0 && !asciiLetter(character) && (character < '0' || character > '9') && character != '_' {
			return false
		}
	}
	return true
}

func asciiLetter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func (adapter *SessionAdapter) Ref() domainsandbox.SessionRef { return adapter.ref }

func (adapter *SessionAdapter) Prepare(ctx context.Context) error {
	if adapter == nil || adapter.options.controlShellID == "" {
		return domainsandbox.ErrInvalidInput
	}
	adapter.controlShellMu.Lock()
	defer adapter.controlShellMu.Unlock()
	strict, preserveSymlinks := true, false
	hardTimeout := 30.0
	command := "mkdir -p -- " + shellQuote(adapter.mapper.physicalWorkspaceRoot()) + " " +
		shellQuote(adapter.mapper.physicalThreadRoot()+"/uploads") + " " +
		shellQuote(adapter.mapper.physicalThreadRoot()+"/outputs")
	response, err := adapter.upstream.Exec(ctx, &sandboxapi.ShellExecRequest{
		Id: &adapter.options.controlShellID, ExecDir: stringPointerAdapter(logicalUserDataRoot), Command: command,
		AsyncMode: boolPointerAdapter(false), Strict: &strict, HardTimeout: &hardTimeout,
		PreserveSymlinks: &preserveSymlinks,
	})
	if err != nil {
		return err
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		response.Data.SessionId != adapter.options.controlShellID ||
		response.Data.Status != sandboxapi.BashCommandStatusCompleted || response.Data.ExitCode == nil || *response.Data.ExitCode != 0 {
		return &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	return nil
}

func (adapter *SessionAdapter) Create(ctx context.Context) error {
	if adapter == nil {
		return domainsandbox.ErrInvalidInput
	}
	adapter.businessShellMu.Lock()
	defer adapter.businessShellMu.Unlock()
	preserveSymlinks := false
	workspace := adapter.mapper.physicalWorkspaceRoot()
	response, err := adapter.upstream.Create(ctx, &sandboxapi.ShellCreateSessionRequest{
		Id: &adapter.upstreamShellID, ExecDir: &workspace, PreserveSymlinks: &preserveSymlinks,
	})
	if err != nil {
		return err
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		response.Data.SessionId != adapter.upstreamShellID || response.Data.WorkingDir != workspace {
		return &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	return nil
}

func validBusinessShellID(value string) bool {
	return validAdapterIdentifier(value) && !strings.HasPrefix(value, "newx-generation-")
}

func validControlShellID(value string) bool {
	const prefix = "newx-generation-"
	if len(value) != len(prefix)+32 || !strings.HasPrefix(value, prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func validAdapterIdentifier(value string) bool {
	if value == "" || len(value) > domainsandbox.MaxSessionIdentifierBytes || value == "." || value == ".." || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '_' && character != '-' && character != '.' {
			return false
		}
	}
	return true
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func stringPointerAdapter(value string) *string { return &value }
func boolPointerAdapter(value bool) *bool       { return &value }

func (adapter *SessionAdapter) Exec(ctx context.Context, input infrasandbox.ExecRequest) (infrasandbox.ExecutionStream, error) {
	if adapter == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	request, err := infrasandbox.NormalizeExecRequest(input, adapter.options.clock(), adapter.options.allowedEnvNames)
	if err != nil {
		return nil, err
	}
	execDir, err := adapter.mapper.ResolveExecCWD(request.CWD)
	if err != nil {
		return nil, err
	}
	command := request.Command
	if len(request.Argv) != 0 {
		parts := make([]string, 0, len(request.Env)+len(request.Argv)+1)
		if len(request.Env) != 0 {
			parts = append(parts, "env")
			names := make([]string, 0, len(request.Env))
			for name := range request.Env {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				parts = append(parts, shellQuote(name+"="+request.Env[name]))
			}
		}
		for _, argument := range request.Argv {
			parts = append(parts, shellQuote(argument))
		}
		command = strings.Join(parts, " ")
	} else if len(request.Env) != 0 {
		parts := []string{"env"}
		names := make([]string, 0, len(request.Env))
		for name := range request.Env {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			parts = append(parts, shellQuote(name+"="+request.Env[name]))
		}
		parts = append(parts, "sh", "-c", shellQuote(command))
		command = strings.Join(parts, " ")
	}
	hardTimeout := request.Deadline.Sub(adapter.options.clock()).Seconds()
	if hardTimeout <= 0 || hardTimeout > infrasandbox.MaxSessionDeadlineAhead.Seconds() {
		return nil, domainsandbox.ErrInvalidInput
	}
	strict, preserveSymlinks, asyncMode := true, false, false
	adapter.businessShellMu.Lock()
	response, execErr := adapter.upstream.Exec(ctx, &sandboxapi.ShellExecRequest{
		Id: &adapter.upstreamShellID, ExecDir: &execDir, Command: command, AsyncMode: &asyncMode,
		Strict: &strict, HardTimeout: &hardTimeout, PreserveSymlinks: &preserveSymlinks,
	})
	if errors.Is(ctx.Err(), context.Canceled) {
		_, _ = adapter.upstream.Kill(context.WithoutCancel(ctx), &sandboxapi.ShellKillProcessRequest{Id: adapter.upstreamShellID})
		execErr = context.Canceled
	}
	adapter.businessShellMu.Unlock()
	if execErr != nil {
		return nil, sanitizeUpstreamError(ctx, execErr)
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		response.Data.SessionId != adapter.upstreamShellID || response.Data.Status != sandboxapi.BashCommandStatusCompleted || response.Data.ExitCode == nil {
		return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	output := ""
	if response.Data.Output != nil {
		output = *response.Data.Output
	}
	if int64(len(output)) > request.MaxOutputBytes {
		output = output[:request.MaxOutputBytes]
	}
	return &adapterExecutionStream{events: []infrasandbox.ExecutionEvent{
		{Kind: infrasandbox.ExecutionEventStdout, Data: []byte(output)},
		{Kind: infrasandbox.ExecutionEventTerminal, ExitCode: response.Data.ExitCode},
	}}, nil
}

type adapterExecutionStream struct {
	mu     sync.Mutex
	events []infrasandbox.ExecutionEvent
	index  int
	closed bool
}

func (stream *adapterExecutionStream) Recv(ctx context.Context) (infrasandbox.ExecutionEvent, error) {
	if err := ctx.Err(); err != nil {
		return infrasandbox.ExecutionEvent{}, err
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.closed || stream.index >= len(stream.events) {
		return infrasandbox.ExecutionEvent{}, io.EOF
	}
	event := stream.events[stream.index]
	stream.index++
	event.Data = append([]byte(nil), event.Data...)
	if event.ExitCode != nil {
		exitCode := *event.ExitCode
		event.ExitCode = &exitCode
	}
	return event, nil
}

func (stream *adapterExecutionStream) Close() error {
	stream.mu.Lock()
	stream.closed = true
	stream.mu.Unlock()
	return nil
}

func (adapter *SessionAdapter) Read(ctx context.Context, input infrasandbox.ReadRequest) (infrasandbox.FileContent, error) {
	request, err := infrasandbox.NormalizeReadRequest(input)
	if err != nil {
		return infrasandbox.FileContent{}, err
	}
	physical, err := adapter.mapper.ResolveFilePath(WorkspaceOperationRead, request.Path)
	if err != nil {
		return infrasandbox.FileContent{}, err
	}
	sudo := false
	adapter.businessShellMu.Lock()
	response, err := adapter.upstream.Read(ctx, &sandboxapi.FileReadRequest{File: physical, Sudo: &sudo})
	adapter.businessShellMu.Unlock()
	if err != nil {
		return infrasandbox.FileContent{}, err
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		!adapter.matchesLogicalPath(response.Data.File, request.Path) {
		return infrasandbox.FileContent{}, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	if int64(len(response.Data.Content)) > request.MaxBytes {
		return infrasandbox.FileContent{}, &UpstreamError{reasonCode: ReasonAdapterResultTooLarge}
	}
	return infrasandbox.FileContent{Data: []byte(response.Data.Content)}, nil
}

func (adapter *SessionAdapter) Write(ctx context.Context, input infrasandbox.WriteRequest) error {
	request, err := infrasandbox.NormalizeWriteRequest(input)
	if err != nil {
		return err
	}
	physical, err := adapter.mapper.ResolveFilePath(WorkspaceOperationWrite, request.Path)
	if err != nil {
		return err
	}
	sudo, encoding := false, sandboxapi.FileContentEncodingBase64
	adapter.businessShellMu.Lock()
	response, err := adapter.upstream.Write(ctx, &sandboxapi.FileWriteRequest{
		File: physical, Content: base64.StdEncoding.EncodeToString(request.Content), Encoding: &encoding,
		Append: &request.Append, Sudo: &sudo,
	})
	adapter.businessShellMu.Unlock()
	if err != nil {
		return err
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		!adapter.matchesLogicalPath(response.Data.File, request.Path) {
		return &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	return nil
}

func (adapter *SessionAdapter) List(ctx context.Context, input infrasandbox.ListRequest) ([]infrasandbox.FileEntry, error) {
	request, err := infrasandbox.NormalizeListRequest(input)
	if err != nil {
		return nil, err
	}
	physical, err := adapter.mapper.ResolveFilePath(WorkspaceOperationList, request.Path)
	if err != nil {
		return nil, err
	}
	adapter.businessShellMu.Lock()
	response, err := adapter.upstream.List(ctx, &sandboxapi.FileListRequest{Path: physical})
	adapter.businessShellMu.Unlock()
	if err != nil {
		return nil, err
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		!adapter.matchesLogicalPath(response.Data.Path, request.Path) {
		return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	if len(response.Data.Files) > request.Limit {
		return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	entries := make([]infrasandbox.FileEntry, 0, len(response.Data.Files))
	for _, file := range response.Data.Files {
		if file == nil {
			return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
		}
		entry, mapErr := adapter.mapFileEntry(file.Path, file.IsDirectory, file.Size, file.ModifiedTime)
		if mapErr != nil {
			return nil, mapErr
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (adapter *SessionAdapter) Glob(ctx context.Context, input infrasandbox.GlobRequest) ([]infrasandbox.FileEntry, error) {
	request, err := infrasandbox.NormalizeGlobRequest(input)
	if err != nil {
		return nil, err
	}
	physical, err := adapter.mapper.ResolveFilePath(WorkspaceOperationGlob, request.Path)
	if err != nil {
		return nil, err
	}
	adapter.businessShellMu.Lock()
	response, err := adapter.upstream.Glob(ctx, &sandboxapi.FileGlobRequest{Path: physical, Pattern: request.Pattern, MaxResults: &request.Limit})
	adapter.businessShellMu.Unlock()
	if err != nil {
		return nil, err
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		!adapter.matchesLogicalPath(response.Data.Path, request.Path) || response.Data.Pattern != request.Pattern ||
		len(response.Data.Files) > request.Limit {
		return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	entries := make([]infrasandbox.FileEntry, 0, len(response.Data.Files))
	for _, file := range response.Data.Files {
		if file == nil {
			return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
		}
		directory := false
		if file.IsDirectory != nil {
			directory = *file.IsDirectory
		}
		entry, mapErr := adapter.mapFileEntry(file.Path, directory, file.Size, file.ModifiedTime)
		if mapErr != nil {
			return nil, mapErr
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func (adapter *SessionAdapter) Grep(ctx context.Context, input infrasandbox.GrepRequest) ([]infrasandbox.GrepMatch, error) {
	request, err := infrasandbox.NormalizeGrepRequest(input)
	if err != nil {
		return nil, err
	}
	physical, err := adapter.mapper.ResolveFilePath(WorkspaceOperationGrep, request.Path)
	if err != nil {
		return nil, err
	}
	adapter.businessShellMu.Lock()
	response, err := adapter.upstream.Grep(ctx, &sandboxapi.FileGrepRequest{Path: physical, Pattern: request.Pattern, MaxResults: &request.Limit})
	adapter.businessShellMu.Unlock()
	if err != nil {
		return nil, err
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		!adapter.matchesLogicalPath(response.Data.Path, request.Path) || response.Data.Pattern != request.Pattern ||
		len(response.Data.Matches) > request.Limit {
		return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	matches := make([]infrasandbox.GrepMatch, 0, len(response.Data.Matches))
	for _, match := range response.Data.Matches {
		if match == nil || match.LineNumber < 1 {
			return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
		}
		logical, mapErr := adapter.mapper.ReverseFilePath(match.File)
		if mapErr != nil {
			return nil, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
		}
		matches = append(matches, infrasandbox.GrepMatch{Path: logical, Line: match.LineNumber, Text: match.LineContent})
	}
	return matches, nil
}

func (adapter *SessionAdapter) Replace(ctx context.Context, input infrasandbox.ReplaceRequest) error {
	request, err := infrasandbox.NormalizeReplaceRequest(input)
	if err != nil {
		return err
	}
	physical, err := adapter.mapper.ResolveFilePath(WorkspaceOperationReplace, request.Path)
	if err != nil {
		return err
	}
	sudo := false
	adapter.businessShellMu.Lock()
	response, err := adapter.upstream.Replace(ctx, &sandboxapi.FileReplaceRequest{
		File: physical, OldStr: string(request.Old), NewStr: string(request.New), Sudo: &sudo,
	})
	adapter.businessShellMu.Unlock()
	if err != nil {
		return err
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil ||
		!adapter.matchesLogicalPath(response.Data.File, request.Path) {
		return &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	return nil
}

func (adapter *SessionAdapter) Download(ctx context.Context, input infrasandbox.DownloadRequest) (io.ReadCloser, error) {
	request, err := infrasandbox.NormalizeDownloadRequest(input)
	if err != nil {
		return nil, err
	}
	physical, err := adapter.mapper.ResolveFilePath(WorkspaceOperationDownload, request.Path)
	if err != nil {
		return nil, err
	}
	adapter.businessShellMu.Lock()
	reader, err := adapter.upstream.Download(ctx, physical)
	if err != nil {
		adapter.businessShellMu.Unlock()
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(reader, request.MaxBytes+1))
	adapter.businessShellMu.Unlock()
	if readErr != nil {
		return nil, &UpstreamError{reasonCode: ReasonUpstreamUnavailable}
	}
	if int64(len(data)) > request.MaxBytes {
		return nil, &UpstreamError{reasonCode: ReasonAdapterResultTooLarge}
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (adapter *SessionAdapter) matchesLogicalPath(physical, expectedLogical string) bool {
	logical, err := adapter.mapper.ReverseFilePath(physical)
	return err == nil && logical == expectedLogical
}

func (adapter *SessionAdapter) mapFileEntry(physical string, directory bool, size *int, modified *string) (infrasandbox.FileEntry, error) {
	logical, err := adapter.mapper.ReverseFilePath(physical)
	if err != nil {
		return infrasandbox.FileEntry{}, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
	}
	entry := infrasandbox.FileEntry{Path: logical, Directory: directory}
	if size != nil {
		if *size < 0 {
			return infrasandbox.FileEntry{}, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
		}
		entry.Size = int64(*size)
	}
	if modified != nil && *modified != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, *modified)
		if parseErr != nil {
			return infrasandbox.FileEntry{}, &UpstreamError{reasonCode: ReasonAdapterContractInvalid}
		}
		entry.Modified = parsed.UTC()
	}
	return entry, nil
}

var _ infrasandbox.SandboxSession = (*SessionAdapter)(nil)
