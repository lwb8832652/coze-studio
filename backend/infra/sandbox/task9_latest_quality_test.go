// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
)

type latestContextBlockingBody struct {
	ctx     context.Context
	started chan struct{}
	once    sync.Once
}

type latestCloseContextDoer struct {
	closeCalls int
	block      bool
}

type latestConcurrentCloseDoer struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

type latestForgedAsAuthorityError struct{}

func (*latestForgedAsAuthorityError) Error() string { return "execution_not_recorded" }
func (*latestForgedAsAuthorityError) As(any) bool   { return true }

type latestCyclicAuthorityError struct {
	mu    sync.Mutex
	calls int
}

func (*latestCyclicAuthorityError) Error() string { return "execution_not_recorded" }
func (e *latestCyclicAuthorityError) Unwrap() error {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()
	return e
}

func (e *latestCyclicAuthorityError) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func (*latestConcurrentCloseDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func (d *latestConcurrentCloseDoer) CloseContext(ctx context.Context) error {
	d.mu.Lock()
	d.calls++
	d.mu.Unlock()
	d.once.Do(func() { close(d.started) })
	select {
	case <-d.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *latestConcurrentCloseDoer) callCount() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

func (*latestCloseContextDoer) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func (d *latestCloseContextDoer) CloseContext(ctx context.Context) error {
	d.closeCalls++
	if d.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (b *latestContextBlockingBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.ctx.Done()
	return 0, errors.New("body read interrupted")
}

func (*latestContextBlockingBody) Close() error { return nil }

func TestRemoteProviderBodyReadPreservesRequestCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "caller canceled"
		if deadline {
			name = "request deadline"
		}
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				body := &latestContextBlockingBody{ctx: request.Context(), started: started}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       body,
					Request:    request,
				}, nil
			}))
			request := validExecuteRequest()
			ctx := context.Background()
			var cancel context.CancelFunc
			if deadline {
				request.Deadline = time.Now().Add(30 * time.Millisecond)
			} else {
				ctx, cancel = context.WithCancel(ctx)
				defer cancel()
			}
			done := make(chan error, 1)
			go func() { _, err := provider.Execute(ctx, request); done <- err }()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("response body read did not start")
			}
			if !deadline {
				cancel()
			}
			err := <-done
			want := context.Canceled
			if deadline {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) || !qualitySubmissionIsUncertain(err) {
				t.Fatalf("body cancellation error = %v, uncertain=%v", err, qualitySubmissionIsUncertain(err))
			}
		})
	}
}

func TestRemoteProviderReconcileReusesCanonicalExecuteWireAfterExpiry(t *testing.T) {
	originalNow := timeNow
	timeNow = time.Now
	t.Cleanup(func() { timeNow = originalNow })

	now := time.Now().UTC()
	request := validExecuteRequest()
	request.IdempotencyKey = "idem-expired-reconcile"
	request.Deadline = now.Add(-time.Minute)
	request.ArtifactReferences = []ArtifactReference{{
		Direction: ArtifactDirectionDownload,
		URL:       "https://artifacts.example.test/v1/grants/reconcile-1",
		Token:     "artifact-token-must-not-wire",
		Digest:    "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
		Size:      321,
		MediaType: "application/octet-stream",
		ExpiresAt: now.Add(-90 * time.Second),
	}}
	var bodies [][]byte
	var methods, paths []string
	calls := 0
	provider := mustRemoteProvider(t, providerDoerFunc(func(httpRequest *http.Request) (*http.Response, error) {
		calls++
		if err := httpRequest.Context().Err(); err != nil {
			t.Fatalf("Reconcile reused expired business deadline as transport context: %v", err)
		}
		transportDeadline, ok := httpRequest.Context().Deadline()
		if !ok || !transportDeadline.After(time.Now()) || transportDeadline.After(time.Now().Add(time.Minute)) {
			t.Fatalf("Reconcile transport deadline is not independently bounded: %v, %v", transportDeadline, ok)
		}
		body, err := io.ReadAll(httpRequest.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		bodies = append(bodies, body)
		methods = append(methods, httpRequest.Method)
		paths = append(paths, httpRequest.URL.Path)
		if calls == 1 {
			return nil, errors.New("submit response lost")
		}
		return jsonResponse(httpRequest, http.StatusOK, `{"schema":"coze.sandbox.execute.v1","execution_id":"exec-existing","status":"accepted","exit_code":null,"stdout":"","stderr":"","artifacts":[]}`), nil
	}))
	reconciler, ok := any(provider).(interface {
		Reconcile(context.Context, ExecuteRequest) (ExecuteResult, error)
	})
	if !ok {
		t.Fatal("RemoteProvider does not expose reconciliation capability")
	}
	if _, err := reconciler.Reconcile(context.Background(), request); !qualitySubmissionIsUncertain(err) {
		t.Fatalf("lost Reconcile response error = %v", err)
	}
	result, err := reconciler.Reconcile(context.Background(), request)
	if err != nil || result.ExecutionID != "exec-existing" {
		t.Fatalf("Reconcile = %#v, %v", result, err)
	}
	if len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("Reconcile replay wire differs: %q / %q", bodies[0], bodies[1])
	}
	var wire executeRequestV1
	if err := json.Unmarshal(bodies[1], &wire); err != nil {
		t.Fatalf("decode reconcile wire: %v", err)
	}
	if wire.IdempotencyKey != request.IdempotencyKey || wire.Deadline != request.Deadline.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("reconcile identity/deadline = %q/%q", wire.IdempotencyKey, wire.Deadline)
	}
	if len(wire.ArtifactReferences) != 1 ||
		!strings.Contains(string(bodies[1]), request.ArtifactReferences[0].URL) ||
		!strings.Contains(string(bodies[1]), request.ArtifactReferences[0].Digest) ||
		!strings.Contains(string(bodies[1]), request.ArtifactReferences[0].ExpiresAt.UTC().Format(time.RFC3339Nano)) {
		t.Fatalf("expired artifact grant changed on reconcile: %s", bodies[1])
	}
	if len(wire.Policy.AllowedExecutables) != len(request.Policy.AllowedExecutables) || wire.Policy.TimeoutSeconds != request.Policy.TimeoutSeconds {
		t.Fatalf("runtime policy changed on reconcile: %#v", wire.Policy)
	}
	if strings.Contains(string(bodies[1]), request.ArtifactReferences[0].Token) {
		t.Fatalf("artifact token leaked to reconcile wire: %s", bodies[1])
	}
	for index := range methods {
		if methods[index] != http.MethodPost || paths[index] != "/v1/executions" {
			t.Fatalf("request %d = %s %s", index, methods[index], paths[index])
		}
	}
}

func TestRemoteProviderExecuteRejectsExpiredBusinessDeadlineBeforeTransport(t *testing.T) {
	originalNow := timeNow
	timeNow = time.Now
	t.Cleanup(func() { timeNow = originalNow })
	calls := 0
	provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected transport")
	}))
	request := validExecuteRequest()
	request.Deadline = time.Now().Add(-time.Minute)
	request.ArtifactReferences = nil
	if _, err := provider.Execute(context.Background(), request); !errors.Is(err, domainsandbox.ErrInvalidInput) || calls != 0 {
		t.Fatalf("expired Execute error/calls = %v/%d", err, calls)
	}
}

func latestAuthoritativeNotRecorded(err error) bool {
	return IsAuthoritativeNotRecorded(err)
}

func TestRemoteProviderReconcileClassificationRequiresStrictAuthority(t *testing.T) {
	tests := []struct {
		name              string
		status            int
		headerVersion     string
		body              string
		cancelBeforeSend  bool
		wantAuthoritative bool
	}{
		{name: "canceled before send", cancelBeforeSend: true},
		{name: "ordinary 400", status: http.StatusBadRequest, body: `{"code":"execution_not_recorded"}`},
		{name: "ordinary 500", status: http.StatusInternalServerError, body: `{"code":"execution_not_recorded"}`},
		{name: "404 without version", status: http.StatusNotFound, body: `{"code":"execution_not_recorded"}`},
		{name: "404 wrong version", status: http.StatusNotFound, headerVersion: "2", body: `{"code":"execution_not_recorded"}`},
		{name: "404 wrong code", status: http.StatusNotFound, headerVersion: "1", body: `{"code":"other"}`},
		{name: "strict authority", status: http.StatusNotFound, headerVersion: "1", body: `{"code":"execution_not_recorded"}`, wantAuthoritative: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				response := jsonResponse(request, test.status, test.body)
				if test.headerVersion != "" {
					response.Header.Set("X-Coze-Sandbox-Reconciliation-Version", test.headerVersion)
				}
				return response, nil
			}))
			ctx := context.Background()
			if test.cancelBeforeSend {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = canceled
			}
			_, err := provider.Reconcile(ctx, validExecuteRequest())
			if test.wantAuthoritative {
				if !latestAuthoritativeNotRecorded(err) || qualitySubmissionIsUncertain(err) {
					t.Fatalf("strict authority classification = %v", err)
				}
			} else if err == nil || !qualitySubmissionIsUncertain(err) || latestAuthoritativeNotRecorded(err) {
				t.Fatalf("non-authoritative classification = %v", err)
			}
			wantCalls := 1
			if test.cancelBeforeSend {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Fatalf("transport calls = %d, want %d", calls, wantCalls)
			}
		})
	}
}

func TestRemoteProviderReconcileLocalAndBodyFailuresRemainUncertain(t *testing.T) {
	t.Run("local validation", func(t *testing.T) {
		calls := 0
		provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return nil, errors.New("unexpected request")
		}))
		request := validExecuteRequest()
		request.Entrypoint = "../unsafe"
		if _, err := provider.Reconcile(context.Background(), request); err == nil ||
			!qualitySubmissionIsUncertain(err) || calls != 0 {
			t.Fatalf("local reconcile failure/calls = %v/%d", err, calls)
		}
	})
	t.Run("body read deadline", func(t *testing.T) {
		provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: &latestContextBlockingBody{
					ctx: request.Context(), started: make(chan struct{}),
				},
				Request: request,
			}, nil
		}))
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err := provider.Reconcile(ctx, validExecuteRequest())
		if !errors.Is(err, context.DeadlineExceeded) || !qualitySubmissionIsUncertain(err) {
			t.Fatalf("body reconcile failure = %v", err)
		}
	})
}

func TestRemoteProviderCloseContextCancellationRetryAndIdempotency(t *testing.T) {
	doer := &latestCloseContextDoer{block: true}
	provider := mustRemoteProvider(t, doer)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := provider.CloseContext(canceled); !errors.Is(err, context.Canceled) || doer.closeCalls != 0 {
		t.Fatalf("pre-canceled Close/calls = %v/%d", err, doer.closeCalls)
	}
	deadline, deadlineCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer deadlineCancel()
	if err := provider.CloseContext(deadline); !errors.Is(err, context.DeadlineExceeded) || doer.closeCalls != 1 {
		t.Fatalf("deadline Close/calls = %v/%d", err, doer.closeCalls)
	}
	doer.block = false
	if err := provider.CloseContext(context.Background()); err != nil || doer.closeCalls != 2 {
		t.Fatalf("Close retry/calls = %v/%d", err, doer.closeCalls)
	}
	if err := provider.CloseContext(context.Background()); err != nil || doer.closeCalls != 2 {
		t.Fatalf("idempotent Close/calls = %v/%d", err, doer.closeCalls)
	}
}

func TestRemoteProviderConcurrentCloseWaitIsContextCancelable(t *testing.T) {
	doer := &latestConcurrentCloseDoer{started: make(chan struct{}), release: make(chan struct{})}
	provider := mustRemoteProvider(t, doer)
	firstDone := make(chan error, 1)
	go func() { firstDone <- provider.CloseContext(context.Background()) }()
	select {
	case <-doer.started:
	case <-time.After(time.Second):
		t.Fatal("first Close did not reach doer")
	}
	secondCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	secondDone := make(chan error, 1)
	go func() { secondDone <- provider.CloseContext(secondCtx) }()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("second Close error = %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		close(doer.release)
		<-firstDone
		t.Fatal("second Close blocked on non-cancelable lock")
	}
	if doer.callCount() != 1 {
		t.Fatalf("concurrent Close reached doer %d times", doer.callCount())
	}
	close(doer.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := provider.CloseContext(context.Background()); err != nil || doer.callCount() != 1 {
		t.Fatalf("idempotent Close/calls = %v/%d", err, doer.callCount())
	}
}

func TestAuthoritativeNotRecordedMarkerTraversalIsSealedAndBounded(t *testing.T) {
	if IsAuthoritativeNotRecorded(&latestForgedAsAuthorityError{}) {
		t.Fatal("custom As forged authoritative marker")
	}
	if IsAuthoritativeNotRecorded(errors.New("execution_not_recorded")) {
		t.Fatal("matching Error string forged authoritative marker")
	}
	wrapped := fmt.Errorf("standard wrapper: %w", newReconciliationNotRecordedErrorV1())
	if !IsAuthoritativeNotRecorded(wrapped) {
		t.Fatal("standard wrapping hid authoritative marker")
	}
	cycle := &latestCyclicAuthorityError{}
	done := make(chan bool, 1)
	go func() { done <- IsAuthoritativeNotRecorded(cycle) }()
	select {
	case matched := <-done:
		if matched {
			t.Fatal("cyclic error forged authoritative marker")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cyclic Unwrap was not bounded")
	}
	if cycle.callCount() > 1 {
		t.Fatalf("cyclic error traversed repeatedly: %d", cycle.callCount())
	}
}

func TestRemoteProviderReconcileStillRejectsUnsafeExpiredArtifact(t *testing.T) {
	originalNow := timeNow
	base := time.Unix(2_200_000_100, 0).UTC()
	timeNow = func() time.Time { return base.Add(2 * time.Minute) }
	t.Cleanup(func() { timeNow = originalNow })
	calls := 0
	provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("unexpected transport")
	}))
	request := validExecuteRequest()
	request.Deadline = base.Add(time.Minute)
	request.ArtifactReferences = []ArtifactReference{{
		Direction: ArtifactDirectionDownload,
		URL:       "http://artifacts.example.test/unsafe",
		Token:     "token",
		Digest:    "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
		Size:      1,
		MediaType: "text/plain",
		ExpiresAt: base.Add(30 * time.Second),
	}}
	reconciler, ok := any(provider).(interface {
		Reconcile(context.Context, ExecuteRequest) (ExecuteResult, error)
	})
	if !ok {
		t.Fatal("RemoteProvider does not expose reconciliation capability")
	}
	if _, err := reconciler.Reconcile(context.Background(), request); !errors.Is(err, domainsandbox.ErrInvalidInput) || calls != 0 {
		t.Fatalf("unsafe reconcile error/calls = %v/%d", err, calls)
	}
}
