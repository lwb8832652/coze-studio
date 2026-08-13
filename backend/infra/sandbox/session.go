// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

const (
	MaxSessionDeadlineAhead = time.Hour
	MaxSessionOutputBytes   = 4 * 1024 * 1024
	MaxSessionPathBytes     = 1024
	MaxSessionBodyBytes     = 1024 * 1024
	MaxSessionFileBytes     = 32 * 1024 * 1024
	MaxSessionEntries       = 1000
	MaxSessionPatternBytes  = 4096
)

type SandboxSessionManager interface {
	Acquire(context.Context, AcquireSessionRequest) (SandboxSession, error)
	Get(context.Context, domainsandbox.SessionRef) (SandboxSession, error)
	Release(context.Context, domainsandbox.SessionRef) error
	Destroy(context.Context, domainsandbox.SessionRef) error
	Recover(context.Context, domainsandbox.SessionRef) (SandboxSession, error)
}

type SandboxSession interface {
	Ref() domainsandbox.SessionRef
	Exec(context.Context, ExecRequest) (ExecutionStream, error)
	Read(context.Context, ReadRequest) (FileContent, error)
	Write(context.Context, WriteRequest) error
	List(context.Context, ListRequest) ([]FileEntry, error)
	Glob(context.Context, GlobRequest) ([]FileEntry, error)
	Grep(context.Context, GrepRequest) ([]GrepMatch, error)
	Replace(context.Context, ReplaceRequest) error
	Download(context.Context, DownloadRequest) (io.ReadCloser, error)
}

type AcquireSessionRequest struct {
	Key domainsandbox.SessionKey
}

func NormalizeAcquireSessionRequest(input AcquireSessionRequest) (AcquireSessionRequest, error) {
	key, err := domainsandbox.NormalizeSessionKey(input.Key)
	if err != nil || domainsandbox.ValidatePhase1SessionProfile(key.Profile) != nil {
		return AcquireSessionRequest{}, domainsandbox.ErrInvalidInput
	}
	return AcquireSessionRequest{Key: key}, nil
}

func (AcquireSessionRequest) String() string {
	return "sandbox.AcquireSessionRequest{identity:<redacted>}"
}
func (AcquireSessionRequest) GoString() string {
	return "sandbox.AcquireSessionRequest{identity:<redacted>}"
}
func (AcquireSessionRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.AcquireSessionRequest{identity:<redacted>}")
}

type ExecRequest struct {
	OperationID    string
	Argv           []string
	Command        string
	Env            map[string]string
	CWD            string
	Deadline       time.Time
	MaxOutputBytes int64
}

func NormalizeExecRequest(input ExecRequest, now time.Time, allowedEnvNames []string) (ExecRequest, error) {
	if now.IsZero() || !validSessionIdentifier(input.OperationID) ||
		(len(input.Argv) == 0) == (input.Command == "") || len(input.Argv) > MaxArgs ||
		len(input.Env) > MaxEnvVars || len(allowedEnvNames) > MaxEnvVars ||
		!validSessionPath(input.CWD, true) || input.Deadline.IsZero() ||
		!input.Deadline.After(now) || input.Deadline.After(now.Add(MaxSessionDeadlineAhead)) ||
		input.MaxOutputBytes <= 0 || input.MaxOutputBytes > MaxSessionOutputBytes {
		return ExecRequest{}, domainsandbox.ErrInvalidInput
	}
	argv := make([]string, len(input.Argv))
	for index, argument := range input.Argv {
		if argument == "" || !validSessionText(argument, MaxArgumentBytes, false) {
			return ExecRequest{}, domainsandbox.ErrInvalidInput
		}
		argv[index] = argument
	}
	if input.Command != "" && (strings.TrimSpace(input.Command) == "" ||
		!validSessionText(input.Command, MaxSessionBodyBytes, true)) {
		return ExecRequest{}, domainsandbox.ErrInvalidInput
	}
	allowedEnvironment := make(map[string]struct{}, len(allowedEnvNames))
	for _, name := range allowedEnvNames {
		if !validSessionEnvironmentKey(name) {
			return ExecRequest{}, domainsandbox.ErrInvalidInput
		}
		if _, duplicate := allowedEnvironment[name]; duplicate {
			return ExecRequest{}, domainsandbox.ErrInvalidInput
		}
		allowedEnvironment[name] = struct{}{}
	}
	environment := make(map[string]string, len(input.Env))
	for key, value := range input.Env {
		if !validSessionEnvironmentKey(key) || !validSessionText(value, MaxEnvValueBytes, false) {
			return ExecRequest{}, domainsandbox.ErrInvalidInput
		}
		if _, allowed := allowedEnvironment[key]; !allowed {
			return ExecRequest{}, domainsandbox.ErrInvalidInput
		}
		environment[key] = value
	}
	return ExecRequest{
		OperationID: input.OperationID, Argv: argv, Command: input.Command, Env: environment,
		CWD: input.CWD, Deadline: input.Deadline.UTC(), MaxOutputBytes: input.MaxOutputBytes,
	}, nil
}

func (ExecRequest) String() string   { return "sandbox.ExecRequest{operation:<redacted>}" }
func (ExecRequest) GoString() string { return "sandbox.ExecRequest{operation:<redacted>}" }
func (ExecRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.ExecRequest{operation:<redacted>}")
}

type ExecutionEventKind string

const (
	ExecutionEventStdout   ExecutionEventKind = "stdout"
	ExecutionEventStderr   ExecutionEventKind = "stderr"
	ExecutionEventTerminal ExecutionEventKind = "terminal"
)

type ExecutionEvent struct {
	Kind     ExecutionEventKind
	Data     []byte
	ExitCode *int
}

func (ExecutionEvent) String() string   { return "sandbox.ExecutionEvent{data:<redacted>}" }
func (ExecutionEvent) GoString() string { return "sandbox.ExecutionEvent{data:<redacted>}" }
func (ExecutionEvent) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.ExecutionEvent{data:<redacted>}")
}

type ExecutionStream interface {
	Recv(context.Context) (ExecutionEvent, error)
	Close() error
}

type ReadRequest struct {
	Path     string
	MaxBytes int64
}

func NormalizeReadRequest(input ReadRequest) (ReadRequest, error) {
	if !validSessionPath(input.Path, true) || input.MaxBytes <= 0 || input.MaxBytes > MaxSessionFileBytes {
		return ReadRequest{}, domainsandbox.ErrInvalidInput
	}
	return input, nil
}

func (ReadRequest) String() string   { return "sandbox.ReadRequest{path:<redacted>}" }
func (ReadRequest) GoString() string { return "sandbox.ReadRequest{path:<redacted>}" }
func (ReadRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.ReadRequest{path:<redacted>}")
}

type FileContent struct {
	Data []byte
}

func (FileContent) String() string   { return "sandbox.FileContent{body:<redacted>}" }
func (FileContent) GoString() string { return "sandbox.FileContent{body:<redacted>}" }
func (FileContent) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.FileContent{body:<redacted>}")
}

type WriteRequest struct {
	Path    string
	Content []byte
	Append  bool
}

func NormalizeWriteRequest(input WriteRequest) (WriteRequest, error) {
	if !validSessionPath(input.Path, false) || len(input.Content) > MaxSessionBodyBytes {
		return WriteRequest{}, domainsandbox.ErrInvalidInput
	}
	input.Content = append([]byte(nil), input.Content...)
	return input, nil
}

func (WriteRequest) String() string   { return "sandbox.WriteRequest{path:<redacted> body:<redacted>}" }
func (WriteRequest) GoString() string { return "sandbox.WriteRequest{path:<redacted> body:<redacted>}" }
func (WriteRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.WriteRequest{path:<redacted> body:<redacted>}")
}

type ListRequest struct {
	Path  string
	Limit int
}

func NormalizeListRequest(input ListRequest) (ListRequest, error) {
	if !validSessionPath(input.Path, true) || input.Limit <= 0 || input.Limit > MaxSessionEntries {
		return ListRequest{}, domainsandbox.ErrInvalidInput
	}
	return input, nil
}

func (ListRequest) String() string   { return "sandbox.ListRequest{path:<redacted>}" }
func (ListRequest) GoString() string { return "sandbox.ListRequest{path:<redacted>}" }
func (ListRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.ListRequest{path:<redacted>}")
}

type FileEntry struct {
	Path      string
	Directory bool
	Size      int64
	Modified  time.Time
}

func (FileEntry) String() string   { return "sandbox.FileEntry{path:<redacted>}" }
func (FileEntry) GoString() string { return "sandbox.FileEntry{path:<redacted>}" }
func (FileEntry) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.FileEntry{path:<redacted>}")
}

type GlobRequest struct {
	Path    string
	Pattern string
	Limit   int
}

func NormalizeGlobRequest(input GlobRequest) (GlobRequest, error) {
	if !validSessionPath(input.Path, true) || !validSessionPattern(input.Pattern) ||
		input.Limit <= 0 || input.Limit > MaxSessionEntries {
		return GlobRequest{}, domainsandbox.ErrInvalidInput
	}
	return input, nil
}

func (GlobRequest) String() string { return "sandbox.GlobRequest{path:<redacted> pattern:<redacted>}" }
func (GlobRequest) GoString() string {
	return "sandbox.GlobRequest{path:<redacted> pattern:<redacted>}"
}
func (GlobRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.GlobRequest{path:<redacted> pattern:<redacted>}")
}

type GrepRequest struct {
	Path    string
	Pattern string
	Limit   int
}

func NormalizeGrepRequest(input GrepRequest) (GrepRequest, error) {
	if !validSessionPath(input.Path, true) || !validSessionText(input.Pattern, MaxSessionPatternBytes, true) ||
		input.Pattern == "" || input.Limit <= 0 || input.Limit > MaxSessionEntries {
		return GrepRequest{}, domainsandbox.ErrInvalidInput
	}
	return input, nil
}

func (GrepRequest) String() string { return "sandbox.GrepRequest{path:<redacted> pattern:<redacted>}" }
func (GrepRequest) GoString() string {
	return "sandbox.GrepRequest{path:<redacted> pattern:<redacted>}"
}
func (GrepRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.GrepRequest{path:<redacted> pattern:<redacted>}")
}

type GrepMatch struct {
	Path       string
	Line       int
	ByteOffset int64
	Text       string
}

func (GrepMatch) String() string   { return "sandbox.GrepMatch{value:<redacted>}" }
func (GrepMatch) GoString() string { return "sandbox.GrepMatch{value:<redacted>}" }
func (GrepMatch) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.GrepMatch{value:<redacted>}")
}

type ReplaceRequest struct {
	Path string
	Old  []byte
	New  []byte
}

func NormalizeReplaceRequest(input ReplaceRequest) (ReplaceRequest, error) {
	if !validSessionPath(input.Path, false) || len(input.Old) == 0 || len(input.Old) > MaxSessionBodyBytes ||
		len(input.New) > MaxSessionBodyBytes || len(input.Old)+len(input.New) > MaxSessionBodyBytes {
		return ReplaceRequest{}, domainsandbox.ErrInvalidInput
	}
	input.Old = append([]byte(nil), input.Old...)
	input.New = append([]byte(nil), input.New...)
	return input, nil
}

func (ReplaceRequest) String() string {
	return "sandbox.ReplaceRequest{path:<redacted> body:<redacted>}"
}
func (ReplaceRequest) GoString() string {
	return "sandbox.ReplaceRequest{path:<redacted> body:<redacted>}"
}
func (ReplaceRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.ReplaceRequest{path:<redacted> body:<redacted>}")
}

type DownloadRequest struct {
	Path     string
	MaxBytes int64
}

func NormalizeDownloadRequest(input DownloadRequest) (DownloadRequest, error) {
	if !validSessionPath(input.Path, true) || input.MaxBytes <= 0 || input.MaxBytes > MaxSessionFileBytes {
		return DownloadRequest{}, domainsandbox.ErrInvalidInput
	}
	return input, nil
}

func (DownloadRequest) String() string   { return "sandbox.DownloadRequest{path:<redacted>}" }
func (DownloadRequest) GoString() string { return "sandbox.DownloadRequest{path:<redacted>}" }
func (DownloadRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "sandbox.DownloadRequest{path:<redacted>}")
}

var sessionPathRoots = []string{
	"/mnt/user-data/workspace",
	"/mnt/user-data/uploads",
	"/mnt/user-data/outputs",
	"/mnt/skills",
}

func validSessionPath(value string, allowSkills bool) bool {
	if value == "" || len(value) > MaxSessionPathBytes || !utf8.ValidString(value) ||
		!path.IsAbs(value) || path.Clean(value) != value || strings.ContainsAny(value, "\\\x00") {
		return false
	}
	for _, root := range sessionPathRoots {
		if !allowSkills && root == "/mnt/skills" {
			continue
		}
		if value == root || strings.HasPrefix(value, root+"/") {
			return true
		}
	}
	return false
}

func validSessionPattern(value string) bool {
	if value == "" || !validSessionText(value, MaxSessionPatternBytes, false) ||
		path.IsAbs(value) || strings.Contains(value, "\\") {
		return false
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func validSessionIdentifier(value string) bool {
	if value == "" || len(value) > MaxIdentifierBytes || !utf8.ValidString(value) ||
		strings.TrimSpace(value) != value {
		return false
	}
	for index := range value {
		character := value[index]
		if !(character >= 'a' && character <= 'z') && !(character >= 'A' && character <= 'Z') &&
			!(character >= '0' && character <= '9') && character != '_' && character != '-' &&
			character != '.' && character != ':' {
			return false
		}
	}
	return true
}

func validSessionEnvironmentKey(value string) bool {
	if value == "" || len(value) > MaxEnvKeyBytes || !isASCIIAlpha(value[0]) && value[0] != '_' {
		return false
	}
	for index := 1; index < len(value); index++ {
		if !isASCIIAlpha(value[index]) && (value[index] < '0' || value[index] > '9') && value[index] != '_' {
			return false
		}
	}
	return true
}

func validSessionText(value string, maximum int, allowLineBreaks bool) bool {
	if len(value) > maximum || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if character == 0 || character == 0x7f || character < 0x20 &&
			!(allowLineBreaks && (character == '\n' || character == '\r' || character == '\t')) {
			return false
		}
	}
	return true
}

// CanonicalEnvironmentNames exposes only stable ordering, never values.
func CanonicalEnvironmentNames(environment map[string]string) ([]string, error) {
	names := make([]string, 0, len(environment))
	for name := range environment {
		if !validSessionEnvironmentKey(name) {
			return nil, domainsandbox.ErrInvalidInput
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
