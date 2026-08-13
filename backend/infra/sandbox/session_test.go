// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

func TestSessionExecRequestEnforcesCommandEnvironmentAndResourceBoundaries(t *testing.T) {
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	valid := ExecRequest{
		OperationID:    "op_01",
		Argv:           []string{"python3", "script.py"},
		Env:            map[string]string{"LANG": "C.UTF-8"},
		CWD:            "/mnt/user-data/workspace/project",
		Deadline:       now.Add(time.Minute),
		MaxOutputBytes: 64 * 1024,
	}
	got, err := NormalizeExecRequest(valid, now, []string{"LANG"})
	if err != nil {
		t.Fatalf("NormalizeExecRequest() error = %v", err)
	}
	valid.Argv[0] = "mutated"
	valid.Env["LANG"] = "mutated"
	if got.Argv[0] != "python3" || got.Env["LANG"] != "C.UTF-8" || got.Deadline.Location() != time.UTC {
		t.Fatalf("NormalizeExecRequest() did not detach and canonicalize: %#v", got)
	}

	command := got
	command.Argv = nil
	command.Command = "printf ok"
	if _, err := NormalizeExecRequest(command, now, []string{"LANG"}); err != nil {
		t.Fatalf("NormalizeExecRequest(command) error = %v", err)
	}
	if _, err := NormalizeExecRequest(valid, now, nil); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("NormalizeExecRequest(missing trusted allowlist) error = %v, want ErrInvalidInput", err)
	}
	if _, err := NormalizeExecRequest(valid, now, []string{"HOME"}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("NormalizeExecRequest(untrusted env) error = %v, want ErrInvalidInput", err)
	}
	selfAuthorize := valid
	selfAuthorize.Env = map[string]string{"LD_PRELOAD": "/tmp/untrusted.so"}
	if _, err := NormalizeExecRequest(selfAuthorize, now, []string{"LANG"}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("NormalizeExecRequest(self authorized env) error = %v, want ErrInvalidInput", err)
	}

	tests := []struct {
		name   string
		mutate func(*ExecRequest)
	}{
		{name: "missing executable", mutate: func(req *ExecRequest) { req.Argv = nil }},
		{name: "ambiguous executable", mutate: func(req *ExecRequest) { req.Command = "printf duplicate" }},
		{name: "empty argv program", mutate: func(req *ExecRequest) { req.Argv[0] = "" }},
		{name: "blank command", mutate: func(req *ExecRequest) { req.Argv = nil; req.Command = " \t" }},
		{name: "invalid env name", mutate: func(req *ExecRequest) { req.Env = map[string]string{"LD-PRELOAD": "x"} }},
		{name: "unaudited env", mutate: func(req *ExecRequest) { req.Env = map[string]string{"LD_PRELOAD": "/tmp/x.so"} }},
		{name: "env control", mutate: func(req *ExecRequest) { req.Env = map[string]string{"TOKEN": "line\nsecret"} }},
		{name: "relative cwd", mutate: func(req *ExecRequest) { req.CWD = "workspace" }},
		{name: "cwd traversal", mutate: func(req *ExecRequest) { req.CWD = "/mnt/user-data/workspace/../uploads" }},
		{name: "expired deadline", mutate: func(req *ExecRequest) { req.Deadline = now }},
		{name: "deadline too far", mutate: func(req *ExecRequest) { req.Deadline = now.Add(MaxSessionDeadlineAhead + time.Second) }},
		{name: "missing output cap", mutate: func(req *ExecRequest) { req.MaxOutputBytes = 0 }},
		{name: "excess output cap", mutate: func(req *ExecRequest) { req.MaxOutputBytes = MaxSessionOutputBytes + 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ExecRequest{
				OperationID: valid.OperationID, Argv: append([]string(nil), []string{"python3", "script.py"}...),
				Env: map[string]string{"LANG": "C.UTF-8"}, CWD: valid.CWD, Deadline: valid.Deadline,
				MaxOutputBytes: valid.MaxOutputBytes,
			}
			tt.mutate(&req)
			if _, err := NormalizeExecRequest(req, now, []string{"LANG"}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("NormalizeExecRequest() error = %v, want ErrInvalidInput", err)
			}
		})
	}
	if _, err := NormalizeExecRequest(valid, now, []string{"LANG", "LANG"}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("NormalizeExecRequest(duplicate trusted allowlist) error = %v, want ErrInvalidInput", err)
	}
}

func TestSessionFileRequestsEnforceLogicalRootsAndReadOnlySkills(t *testing.T) {
	readPaths := []string{
		"/mnt/user-data/workspace/project/file.txt",
		"/mnt/user-data/uploads/input.txt",
		"/mnt/user-data/outputs/result.txt",
		"/mnt/skills/tool/SKILL.md",
	}
	for _, logicalPath := range readPaths {
		if _, err := NormalizeReadRequest(ReadRequest{Path: logicalPath, MaxBytes: 1024}); err != nil {
			t.Fatalf("NormalizeReadRequest(%q) error = %v", logicalPath, err)
		}
	}
	writePaths := readPaths[:3]
	for _, logicalPath := range writePaths {
		if _, err := NormalizeWriteRequest(WriteRequest{Path: logicalPath, Content: []byte("safe")}); err != nil {
			t.Fatalf("NormalizeWriteRequest(%q) error = %v", logicalPath, err)
		}
	}
	if _, err := NormalizeWriteRequest(WriteRequest{Path: "/mnt/skills/tool/SKILL.md", Content: []byte("overwrite")}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("NormalizeWriteRequest(skills) error = %v, want ErrInvalidInput", err)
	}

	invalid := []string{
		"", "relative/path", "/etc/passwd", "/mnt/user-data", "/mnt/user-data/workspace/../uploads/x",
		"/mnt/user-data/workspace//x", "/mnt/user-data/workspace/x\x00y",
		strings.Repeat("a", MaxSessionPathBytes+1),
	}
	for index, logicalPath := range invalid {
		t.Run(fmt.Sprintf("invalid_%d", index), func(t *testing.T) {
			if _, err := NormalizeReadRequest(ReadRequest{Path: logicalPath, MaxBytes: 1024}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("NormalizeReadRequest(%q) error = %v, want ErrInvalidInput", logicalPath, err)
			}
		})
	}

	if _, err := NormalizeListRequest(ListRequest{Path: "/mnt/user-data/workspace", Limit: 100}); err != nil {
		t.Fatalf("NormalizeListRequest() error = %v", err)
	}
	if _, err := NormalizeGlobRequest(GlobRequest{Path: "/mnt/user-data/workspace", Pattern: "**/*.go", Limit: 100}); err != nil {
		t.Fatalf("NormalizeGlobRequest() error = %v", err)
	}
	if _, err := NormalizeGrepRequest(GrepRequest{Path: "/mnt/skills", Pattern: "sandbox", Limit: 100}); err != nil {
		t.Fatalf("NormalizeGrepRequest() error = %v", err)
	}
	if _, err := NormalizeReplaceRequest(ReplaceRequest{Path: "/mnt/user-data/workspace/a", Old: []byte("old"), New: []byte("new")}); err != nil {
		t.Fatalf("NormalizeReplaceRequest() error = %v", err)
	}
	if _, err := NormalizeDownloadRequest(DownloadRequest{Path: "/mnt/user-data/outputs/a", MaxBytes: 1024}); err != nil {
		t.Fatalf("NormalizeDownloadRequest() error = %v", err)
	}

	tests := []struct {
		name string
		err  error
	}{
		{name: "write body too large", err: func() error {
			_, err := NormalizeWriteRequest(WriteRequest{Path: "/mnt/user-data/workspace/a", Content: make([]byte, MaxSessionBodyBytes+1)})
			return err
		}()},
		{name: "list limit missing", err: func() error {
			_, err := NormalizeListRequest(ListRequest{Path: "/mnt/user-data/workspace"})
			return err
		}()},
		{name: "glob traversal pattern", err: func() error {
			_, err := NormalizeGlobRequest(GlobRequest{Path: "/mnt/user-data/workspace", Pattern: "../*.go", Limit: 1})
			return err
		}()},
		{name: "grep empty pattern", err: func() error {
			_, err := NormalizeGrepRequest(GrepRequest{Path: "/mnt/user-data/workspace", Limit: 1})
			return err
		}()},
		{name: "replace empty old", err: func() error {
			_, err := NormalizeReplaceRequest(ReplaceRequest{Path: "/mnt/user-data/workspace/a", New: []byte("new")})
			return err
		}()},
		{name: "download cap missing", err: func() error {
			_, err := NormalizeDownloadRequest(DownloadRequest{Path: "/mnt/user-data/outputs/a"})
			return err
		}()},
	}
	for _, tt := range tests {
		if !errors.Is(tt.err, domainsandbox.ErrInvalidInput) {
			t.Errorf("%s error = %v, want ErrInvalidInput", tt.name, tt.err)
		}
	}
}

func TestSessionContractsAreSDKIndependentAndDoNotExposePublish(t *testing.T) {
	managerType := reflect.TypeOf((*SandboxSessionManager)(nil)).Elem()
	for _, method := range []string{"Acquire", "Get", "Release", "Destroy", "Recover"} {
		if _, ok := managerType.MethodByName(method); !ok {
			t.Fatalf("SandboxSessionManager is missing %s", method)
		}
	}
	sessionType := reflect.TypeOf((*SandboxSession)(nil)).Elem()
	for _, method := range []string{"Ref", "Exec", "Read", "Write", "List", "Glob", "Grep", "Replace", "Download"} {
		if _, ok := sessionType.MethodByName(method); !ok {
			t.Fatalf("SandboxSession is missing %s", method)
		}
	}
	if _, ok := sessionType.MethodByName("Publish"); ok {
		t.Fatal("SandboxSession must not expose Phase 2 Publish")
	}

	var _ SandboxSessionManager = (*contractManager)(nil)
	var _ SandboxSession = (*contractSession)(nil)
}

func TestSessionRequestFormattingRedactsCommandsPathsAndBodies(t *testing.T) {
	values := []any{
		ExecRequest{Command: "secret-command", CWD: "/mnt/user-data/workspace/private"},
		ReadRequest{Path: "/mnt/user-data/workspace/private"},
		WriteRequest{Path: "/mnt/user-data/workspace/private", Content: []byte("secret-body")},
		ReplaceRequest{Path: "/mnt/user-data/workspace/private", Old: []byte("secret-old"), New: []byte("secret-new")},
		ExecutionEvent{Kind: ExecutionEventStdout, Data: []byte("secret-body")},
		FileContent{Data: []byte("secret-body")},
		FileEntry{Path: "/mnt/user-data/workspace/private"},
		GrepMatch{Path: "/mnt/user-data/workspace/private", Text: "secret-body"},
	}
	for _, value := range values {
		for _, rendered := range []string{fmt.Sprint(value), fmt.Sprintf("%#v", value), fmt.Sprintf("%+v", value)} {
			for _, secret := range []string{"secret-command", "/mnt/user-data", "secret-body", "secret-old", "secret-new"} {
				if strings.Contains(rendered, secret) {
					t.Fatalf("formatting leaked %q in %q", secret, rendered)
				}
			}
		}
	}
}

type contractManager struct{}

func (*contractManager) Acquire(context.Context, AcquireSessionRequest) (SandboxSession, error) {
	return nil, nil
}
func (*contractManager) Get(context.Context, domainsandbox.SessionRef) (SandboxSession, error) {
	return nil, nil
}
func (*contractManager) Release(context.Context, domainsandbox.SessionRef) error { return nil }
func (*contractManager) Destroy(context.Context, domainsandbox.SessionRef) error { return nil }
func (*contractManager) Recover(context.Context, domainsandbox.SessionRef) (SandboxSession, error) {
	return nil, nil
}

type contractSession struct{}

func (*contractSession) Ref() domainsandbox.SessionRef                              { return domainsandbox.SessionRef{} }
func (*contractSession) Exec(context.Context, ExecRequest) (ExecutionStream, error) { return nil, nil }
func (*contractSession) Read(context.Context, ReadRequest) (FileContent, error) {
	return FileContent{}, nil
}
func (*contractSession) Write(context.Context, WriteRequest) error              { return nil }
func (*contractSession) List(context.Context, ListRequest) ([]FileEntry, error) { return nil, nil }
func (*contractSession) Glob(context.Context, GlobRequest) ([]FileEntry, error) { return nil, nil }
func (*contractSession) Grep(context.Context, GrepRequest) ([]GrepMatch, error) { return nil, nil }
func (*contractSession) Replace(context.Context, ReplaceRequest) error          { return nil }
func (*contractSession) Download(context.Context, DownloadRequest) (io.ReadCloser, error) {
	return nil, nil
}
