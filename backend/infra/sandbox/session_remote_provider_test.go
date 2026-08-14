// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestRemoteSessionProviderRequiresBoundV2Identity(t *testing.T) {
	t.Parallel()

	remote := mustRemoteProvider(t, providerDoerFunc(nil))
	_, err := NewRemoteSessionProvider(remote, RemoteSessionProviderConfig{
		DeploymentID: "runner-dev-a",
		ProviderID:   41,
		Scope:        sandboxidentity.ScopeAgent,
	})
	if err == nil {
		t.Fatal("NewRemoteSessionProvider() without SessionSigner error = nil")
	}
}

func TestRemoteSessionProviderLifecycleBindsServerFactsAndV2Signature(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := remoteSessionTestKeyring(t, now)
	var calls int
	var acquireBody remoteSessionAcquireRequest
	remote := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(body)
		claims, err := keyring.VerifySession(context.Background(), request.Header.Get(sandboxidentity.SessionContextHeader),
			request.Header.Get(sandboxidentity.SessionContextSignatureHeader), sandboxidentity.SessionRequest{
				DeploymentID: "runner-dev-a", ProviderID: 41, Scope: sandboxidentity.ScopeAgent,
				SpaceID: 42, UserID: 43, ThreadID: "thread-a", RunID: "generated-1",
				OperationID: "generated-2", Profile: "core", RequestDigest: digest[:],
			}, request.Method, request.URL.Path, now, acceptingRemoteSessionNonceStore{})
		if err != nil || claims.ProviderID != 41 {
			t.Fatalf("VerifySession() = %#v, %v", claims, err)
		}
		if request.Header.Get("Authorization") != "Bearer synthetic-test-token" {
			t.Fatal("provider credential missing")
		}
		if err := json.Unmarshal(body, &acquireBody); err != nil {
			t.Fatal(err)
		}
		return jsonResponse(request, http.StatusOK, `{"schema":"coze.sandbox.session.v1","session_id":"session-a","state":"active","runtime_generation":7,"profile":"core"}`), nil
	}))
	provider := mustRemoteSessionProvider(t, remote, keyring, now)
	provider.newID = sequenceRemoteSessionIDs("generated-1", "generated-2")
	key := remoteSessionTestKey()
	session, err := provider.Acquire(context.Background(), AcquireSessionRequest{Key: key})
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if calls != 1 || session.Ref() != (domainsandbox.SessionRef{SessionID: "session-a", Key: key, RuntimeGeneration: 7}) {
		t.Fatalf("Acquire() calls/ref = %d/%#v", calls, session.Ref())
	}
	if acquireBody.DeploymentID != key.DeploymentID || acquireBody.ProviderID != key.ProviderID || acquireBody.SpaceID != key.SpaceID ||
		acquireBody.UserID != key.UserID || acquireBody.ThreadID != key.ThreadID || acquireBody.Scope != sandboxidentity.ScopeAgent ||
		acquireBody.RunID != "generated-1" || acquireBody.OperationID != "generated-2" {
		t.Fatalf("acquire body = %#v", acquireBody)
	}

	for name, mutate := range map[string]func(*domainsandbox.SessionKey){
		"deployment": func(value *domainsandbox.SessionKey) { value.DeploymentID = "runner-other" },
		"provider":   func(value *domainsandbox.SessionKey) { value.ProviderID = 99 },
		"space":      func(value *domainsandbox.SessionKey) { value.SpaceID = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := key
			mutate(&bad)
			if _, err := provider.Acquire(context.Background(), AcquireSessionRequest{Key: bad}); !errors.Is(err, domainsandbox.ErrInvalidInput) {
				t.Fatalf("Acquire() error = %v", err)
			}
		})
	}
	if calls != 1 {
		t.Fatalf("invalid identities reached transport: %d", calls)
	}
}

func TestRemoteSessionProviderLifecycleRoutesAreExactAndDoNotTrustProjectionIdentity(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := remoteSessionTestKeyring(t, now)
	ref := domainsandbox.SessionRef{SessionID: "session-a", Key: remoteSessionTestKey(), RuntimeGeneration: 7}
	tests := []struct {
		name       string
		method     string
		path       string
		status     int
		body       string
		invoke     func(*RemoteSessionProvider) (SandboxSession, error)
		wantResult bool
	}{
		{name: "get", method: http.MethodGet, path: "/v1/sessions/session-a", status: http.StatusOK,
			body: `{"schema":"coze.sandbox.session.v1","session_id":"session-a","state":"active","runtime_generation":7,"profile":"core"}`,
			invoke: func(provider *RemoteSessionProvider) (SandboxSession, error) {
				return provider.Get(context.Background(), ref)
			}, wantResult: true},
		{name: "release", method: http.MethodPost, path: "/v1/sessions/session-a:release", status: http.StatusNoContent,
			invoke: func(provider *RemoteSessionProvider) (SandboxSession, error) {
				return nil, provider.Release(context.Background(), ref)
			}},
		{name: "destroy", method: http.MethodPost, path: "/v1/sessions/session-a:destroy", status: http.StatusNoContent,
			invoke: func(provider *RemoteSessionProvider) (SandboxSession, error) {
				return nil, provider.Destroy(context.Background(), ref)
			}},
		{name: "recover", method: http.MethodPost, path: "/v1/sessions/session-a:recover", status: http.StatusOK,
			body: `{"schema":"coze.sandbox.session.v1","session_id":"session-a","state":"active","runtime_generation":8,"profile":"core"}`,
			invoke: func(provider *RemoteSessionProvider) (SandboxSession, error) {
				return provider.Recover(context.Background(), ref)
			}, wantResult: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remote := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != tt.method || request.URL.Path != tt.path || request.URL.RawQuery != "" {
					t.Fatalf("request = %s %s", request.Method, request.URL.String())
				}
				if tt.method == http.MethodPost {
					body, _ := io.ReadAll(request.Body)
					if string(body) != "{}" {
						t.Fatalf("mutation body = %q", body)
					}
				}
				if tt.status == http.StatusNoContent {
					return &http.Response{StatusCode: tt.status, Header: make(http.Header), Body: http.NoBody, ContentLength: 0, Request: request}, nil
				}
				return jsonResponse(request, tt.status, tt.body), nil
			}))
			provider := mustRemoteSessionProvider(t, remote, keyring, now)
			provider.newID = sequenceRemoteSessionIDs("run-a", "route-a")
			got, err := tt.invoke(provider)
			if err != nil {
				t.Fatalf("invoke error = %v", err)
			}
			wantRef := ref
			if tt.name == "recover" {
				wantRef.RuntimeGeneration = 8
			}
			if tt.wantResult && (got == nil || got.Ref() != wantRef) {
				t.Fatalf("result = %#v", got)
			}
		})
	}

	remote := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(request, http.StatusOK, `{"schema":"coze.sandbox.session.v1","session_id":"session-a","state":"active","runtime_generation":8,"profile":"core"}`), nil
	}))
	provider := mustRemoteSessionProvider(t, remote, keyring, now)
	provider.newID = sequenceRemoteSessionIDs("run-a", "route-a")
	if _, err := provider.Get(context.Background(), ref); !errors.Is(err, domainsandbox.ErrUnavailable) {
		t.Fatalf("Get(mismatched generation) error = %v", err)
	}
}

func TestRemoteSessionOperationsMapFullMatrixAndNeverRetry(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := remoteSessionTestKeyring(t, now)
	ref := domainsandbox.SessionRef{SessionID: "session-a", Key: remoteSessionTestKey(), RuntimeGeneration: 7}
	tests := []struct {
		name       string
		wantKind   remoteSessionOperationKind
		wantResult any
		result     any
		invoke     func(*RemoteSession) (any, error)
	}{
		{name: "exec", wantKind: remoteSessionOperationExec, result: remoteExecutionResult{ExitCode: 0, Stdout: []byte("ok")},
			invoke: func(session *RemoteSession) (any, error) {
				stream, err := session.Exec(context.Background(), ExecRequest{OperationID: "exec-a", Argv: []string{"pwd"}, Env: map[string]string{"LANG": "C"}, CWD: "/mnt/user-data/workspace", Deadline: now.Add(time.Minute), MaxOutputBytes: 1024})
				if err != nil {
					return nil, err
				}
				stdout, err := stream.Recv(context.Background())
				if err != nil {
					return nil, err
				}
				terminal, err := stream.Recv(context.Background())
				if err != nil {
					return nil, err
				}
				return []ExecutionEvent{stdout, terminal}, nil
			}},
		{name: "read", wantKind: remoteSessionOperationRead, result: FileContent{Data: []byte("read")}, wantResult: FileContent{Data: []byte("read")}, invoke: func(session *RemoteSession) (any, error) {
			return session.Read(context.Background(), ReadRequest{Path: "/mnt/user-data/workspace/a", MaxBytes: 100})
		}},
		{name: "write", wantKind: remoteSessionOperationWrite, result: map[string]bool{"written": true}, invoke: func(session *RemoteSession) (any, error) {
			return nil, session.Write(context.Background(), WriteRequest{Path: "/mnt/user-data/workspace/a", Content: []byte("x")})
		}},
		{name: "append", wantKind: remoteSessionOperationAppend, result: map[string]bool{"written": true}, invoke: func(session *RemoteSession) (any, error) {
			return nil, session.Write(context.Background(), WriteRequest{Path: "/mnt/user-data/workspace/a", Content: []byte("x"), Append: true})
		}},
		{name: "list", wantKind: remoteSessionOperationList, result: []FileEntry{}, wantResult: []FileEntry{}, invoke: func(session *RemoteSession) (any, error) {
			return session.List(context.Background(), ListRequest{Path: "/mnt/user-data/workspace", Limit: 10})
		}},
		{name: "glob", wantKind: remoteSessionOperationGlob, result: []FileEntry{}, wantResult: []FileEntry{}, invoke: func(session *RemoteSession) (any, error) {
			return session.Glob(context.Background(), GlobRequest{Path: "/mnt/user-data/workspace", Pattern: "*.go", Limit: 10})
		}},
		{name: "grep", wantKind: remoteSessionOperationGrep, result: []GrepMatch{}, wantResult: []GrepMatch{}, invoke: func(session *RemoteSession) (any, error) {
			return session.Grep(context.Background(), GrepRequest{Path: "/mnt/user-data/workspace", Pattern: "x", Limit: 10})
		}},
		{name: "replace", wantKind: remoteSessionOperationReplace, result: map[string]bool{"replaced": true}, invoke: func(session *RemoteSession) (any, error) {
			return nil, session.Replace(context.Background(), ReplaceRequest{Path: "/mnt/user-data/workspace/a", Old: []byte("x"), New: []byte("y")})
		}},
		{name: "download", wantKind: remoteSessionOperationDownload, result: []byte("download"), wantResult: []byte("download"), invoke: func(session *RemoteSession) (any, error) {
			reader, err := session.Download(context.Background(), DownloadRequest{Path: "/mnt/user-data/outputs/a", MaxBytes: 100})
			if err != nil {
				return nil, err
			}
			defer reader.Close()
			return io.ReadAll(reader)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			remote := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				var wire remoteSessionOperationRequest
				if err := json.NewDecoder(request.Body).Decode(&wire); err != nil {
					t.Fatal(err)
				}
				if request.Method != http.MethodPost || request.URL.Path != "/v1/sessions/session-a/operations" || wire.Kind != tt.wantKind || wire.Schema != remoteSessionOperationSchema {
					t.Fatalf("request/wire = %s %s %#v", request.Method, request.URL.Path, wire)
				}
				result, _ := json.Marshal(tt.result)
				digest := sha256.Sum256(result)
				body, _ := json.Marshal(remoteSessionOperationProjection{Schema: remoteSessionOperationSchema, SessionID: ref.SessionID, OperationID: wire.OperationID, Kind: wire.Kind, State: SessionOperationSucceeded, ResultDigest: base64.RawURLEncoding.EncodeToString(digest[:]), Result: result})
				return jsonResponse(request, http.StatusAccepted, string(body)), nil
			}))
			provider := mustRemoteSessionProvider(t, remote, keyring, now)
			provider.newID = sequenceRemoteSessionIDs("file-op-a")
			session := &RemoteSession{provider: provider, ref: ref, runID: "run-a"}
			got, err := tt.invoke(session)
			if err != nil {
				t.Fatalf("invoke error = %v", err)
			}
			if tt.wantResult != nil && !reflect.DeepEqual(got, tt.wantResult) {
				t.Fatalf("result = %#v, want %#v", got, tt.wantResult)
			}
			if calls != 1 {
				t.Fatalf("calls = %d", calls)
			}
		})
	}

	calls := 0
	remote := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) { calls++; return nil, errors.New("uncertain transport") }))
	provider := mustRemoteSessionProvider(t, remote, keyring, now)
	session := &RemoteSession{provider: provider, ref: ref, runID: "run-a"}
	_, err := session.Exec(context.Background(), ExecRequest{OperationID: "exec-a", Argv: []string{"pwd"}, CWD: "/mnt/user-data/workspace", Deadline: now.Add(time.Minute), MaxOutputBytes: 1024})
	if !errors.Is(err, domainsandbox.ErrProviderUnhealthy) || calls != 1 {
		t.Fatalf("uncertain Exec error/calls = %v/%d", err, calls)
	}
}

func TestRemoteSessionStatusEventsCancelAreStrictBounded(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := remoteSessionTestKeyring(t, now)
	ref := domainsandbox.SessionRef{SessionID: "session-a", Key: remoteSessionTestKey(), RuntimeGeneration: 7}
	requests := make([]string, 0, 3)
	remote := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.Method+" "+request.URL.Path)
		switch {
		case strings.HasSuffix(request.URL.Path, "/events"):
			if request.Header.Get("Last-Event-ID") != "op-a_accepted" {
				t.Fatalf("Last-Event-ID = %q", request.Header.Get("Last-Event-ID"))
			}
			body := `{"schema":"coze.sandbox.session_operation_event.v1","event_id":"op-a_accepted","operation_id":"op-a","state":"accepted"}` + "\n"
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/x-ndjson"}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
		default:
			return jsonResponse(request, http.StatusOK, `{"schema":"coze.sandbox.session_operation.v1","session_id":"session-a","operation_id":"op-a","kind":"exec","state":"running"}`), nil
		}
	}))
	provider := mustRemoteSessionProvider(t, remote, keyring, now)
	session := &RemoteSession{provider: provider, ref: ref, runID: "run-a"}
	if status, err := session.OperationStatus(context.Background(), "op-a"); err != nil || status.State != SessionOperationRunning {
		t.Fatalf("status = %#v, %v", status, err)
	}
	if events, err := session.OperationEvents(context.Background(), "op-a", "op-a_accepted"); err != nil || len(events) != 1 || events[0].State != SessionOperationAccepted {
		t.Fatalf("events = %#v, %v", events, err)
	}
	if status, err := session.CancelOperation(context.Background(), "op-a"); err != nil || status.OperationID != "op-a" {
		t.Fatalf("cancel = %#v, %v", status, err)
	}
	want := []string{"GET /v1/sessions/session-a/operations/op-a", "GET /v1/sessions/session-a/operations/op-a/events", "POST /v1/sessions/session-a/operations/op-a:cancel"}
	if !reflect.DeepEqual(requests, want) {
		t.Fatalf("requests = %#v", requests)
	}
	if _, err := session.CancelOperation(context.Background(), "upstream/shell-id"); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("shell id error = %v", err)
	}
}

func TestRemoteSessionOperationStatusRecoversVerifiedResultWithoutLeakingFormatting(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	result := json.RawMessage(`{"written":true,"secret":"must-not-format"}`)
	digest := sha256.Sum256(result)
	body, err := json.Marshal(remoteSessionOperationProjection{
		Schema: remoteSessionOperationSchema, SessionID: "session-a", OperationID: "op-a",
		Kind: remoteSessionOperationWrite, State: SessionOperationSucceeded,
		ResultDigest: base64.RawURLEncoding.EncodeToString(digest[:]), Result: result,
	})
	if err != nil {
		t.Fatal(err)
	}
	remote := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(request, http.StatusOK, string(body)), nil
	}))
	provider := mustRemoteSessionProvider(t, remote, remoteSessionTestKeyring(t, now), now)
	session := &RemoteSession{provider: provider, ref: domainsandbox.SessionRef{SessionID: "session-a", Key: remoteSessionTestKey(), RuntimeGeneration: 7}, runID: "run-a"}
	status, err := session.OperationStatus(context.Background(), "op-a")
	if err != nil || !bytes.Equal(status.Result, result) {
		t.Fatalf("OperationStatus() = %#v, %v", status, err)
	}
	status.Result[0] = '['
	if bytes.Equal(status.Result, result) {
		t.Fatal("status result was not detached")
	}
	for _, rendered := range []string{status.String(), status.GoString(), strings.TrimSpace(strings.ReplaceAll(strings.TrimSpace(status.String()), " ", ""))} {
		if strings.Contains(rendered, "must-not-format") {
			t.Fatalf("formatting leaked result: %q", rendered)
		}
	}

	badDigest := append([]byte(nil), body...)
	badDigest = bytes.Replace(badDigest, []byte(base64.RawURLEncoding.EncodeToString(digest[:])), []byte(base64.RawURLEncoding.EncodeToString(make([]byte, sha256.Size))), 1)
	remote = mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(request, http.StatusOK, string(badDigest)), nil
	}))
	provider = mustRemoteSessionProvider(t, remote, remoteSessionTestKeyring(t, now), now)
	session = &RemoteSession{provider: provider, ref: session.ref, runID: "run-a"}
	if _, err := session.OperationStatus(context.Background(), "op-a"); !errors.Is(err, domainsandbox.ErrProviderUnhealthy) {
		t.Fatalf("bad digest error = %v", err)
	}
}

func TestRemoteSessionStrictJSONRejectsUnknownDuplicateOversizeAndBadNDJSON(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	keyring := remoteSessionTestKeyring(t, now)
	ref := domainsandbox.SessionRef{SessionID: "session-a", Key: remoteSessionTestKey(), RuntimeGeneration: 7}
	for name, response := range map[string]string{
		"unknown":   `{"schema":"coze.sandbox.session.v1","session_id":"session-a","state":"active","runtime_generation":7,"profile":"core","physical_path":"/host/secret"}`,
		"duplicate": `{"schema":"coze.sandbox.session.v1","session_id":"session-a","session_id":"session-b","state":"active","runtime_generation":7,"profile":"core"}`,
	} {
		t.Run(name, func(t *testing.T) {
			remote := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				return jsonResponse(request, http.StatusOK, response), nil
			}))
			provider := mustRemoteSessionProvider(t, remote, keyring, now)
			provider.newID = sequenceRemoteSessionIDs("run-a", "route-a")
			if _, err := provider.Get(context.Background(), ref); !errors.Is(err, domainsandbox.ErrProviderUnhealthy) {
				t.Fatalf("Get() error = %v", err)
			}
		})
	}

	remote := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		body := `{"schema":"coze.sandbox.session_operation_event.v1","event_id":"event-a","operation_id":"op-a","state":"accepted","unknown":true}` + "\n"
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/x-ndjson"}}, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
	}))
	provider := mustRemoteSessionProvider(t, remote, keyring, now)
	session := &RemoteSession{provider: provider, ref: ref, runID: "run-a"}
	if _, err := session.OperationEvents(context.Background(), "op-a", ""); !errors.Is(err, domainsandbox.ErrProviderUnhealthy) {
		t.Fatalf("events error = %v", err)
	}

	remote = mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(nil)), ContentLength: maxRemoteSessionResponseBytes + 1, Request: request}, nil
	}))
	provider = mustRemoteSessionProvider(t, remote, keyring, now)
	session = &RemoteSession{provider: provider, ref: ref, runID: "run-a"}
	if _, err := session.OperationStatus(context.Background(), "op-a"); !errors.Is(err, domainsandbox.ErrProviderUnhealthy) {
		t.Fatalf("oversize error = %v", err)
	}
}

func remoteSessionTestKey() domainsandbox.SessionKey {
	return domainsandbox.SessionKey{DeploymentID: "runner-dev-a", ProviderID: 41, SpaceID: 42, UserID: 43, ThreadID: "thread-a", Profile: domainsandbox.SessionProfileCore}
}

func remoteSessionTestKeyring(t *testing.T, now time.Time) sandboxidentity.Keyring {
	t.Helper()
	keyring, err := sandboxidentity.NewKeyring("current", map[string][]byte{"current": []byte("test-signing-key-material")}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	keyring.Now = func() time.Time { return now }
	var nonce int
	keyring.Nonce = func() (string, error) { nonce++; return "nonce-" + string(rune('a'+nonce)), nil }
	return keyring
}

func mustRemoteSessionProvider(t *testing.T, remote *RemoteProvider, signer sandboxidentity.SessionSigner, now time.Time) *RemoteSessionProvider {
	t.Helper()
	provider, err := NewRemoteSessionProvider(remote, RemoteSessionProviderConfig{DeploymentID: "runner-dev-a", ProviderID: 41, Scope: sandboxidentity.ScopeAgent, SessionSigner: signer, AllowedEnvironmentNames: []string{"LANG"}})
	if err != nil {
		t.Fatal(err)
	}
	provider.now = func() time.Time { return now }
	return provider
}

func sequenceRemoteSessionIDs(ids ...string) func() (string, error) {
	index := 0
	return func() (string, error) {
		if index >= len(ids) {
			return "", errors.New("no more ids")
		}
		value := ids[index]
		index++
		return value, nil
	}
}

type acceptingRemoteSessionNonceStore struct{}

func (acceptingRemoteSessionNonceStore) Consume(context.Context, string, string, time.Time) (bool, error) {
	return true, nil
}
