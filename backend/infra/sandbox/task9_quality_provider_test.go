// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/logs"
)

type qualitySubmissionClassification interface {
	SubmissionOutcomeUncertain() bool
}

func qualitySubmissionIsUncertain(err error) bool {
	var classified qualitySubmissionClassification
	return errors.As(err, &classified) && classified.SubmissionOutcomeUncertain()
}

func TestRemoteProviderPreservesRequestContextErrors(t *testing.T) {
	t.Run("canceled before call", func(t *testing.T) {
		calls := 0
		provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			return jsonResponse(request, http.StatusOK, `{"schema":"coze.sandbox.execute.v1","execution_id":"exec-canceled","status":"accepted","exit_code":null,"stdout":"","stderr":"","artifacts":[]}`), nil
		}))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := provider.Execute(ctx, validExecuteRequest()); !errors.Is(err, context.Canceled) || calls != 0 {
			t.Fatalf("pre-canceled Execute error/calls = %v/%d", err, calls)
		}
	})

	t.Run("canceled during transport", func(t *testing.T) {
		started := make(chan struct{})
		provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
			close(started)
			<-request.Context().Done()
			return nil, errors.New("transport detail must-not-leak")
		}))
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { _, err := provider.Execute(ctx, validExecuteRequest()); done <- err }()
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("transport did not start")
		}
		cancel()
		err := <-done
		if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "must-not-leak") || !qualitySubmissionIsUncertain(err) {
			t.Fatalf("transport cancellation error = %v, uncertain=%v", err, qualitySubmissionIsUncertain(err))
		}
	})

	t.Run("request deadline precedes parent", func(t *testing.T) {
		provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, errors.New("deadline transport detail")
		}))
		request := validExecuteRequest()
		request.Deadline = time.Now().Add(30 * time.Millisecond)
		_, err := provider.Execute(context.Background(), request)
		if !errors.Is(err, context.DeadlineExceeded) || !qualitySubmissionIsUncertain(err) {
			t.Fatalf("request deadline error = %v, uncertain=%v", err, qualitySubmissionIsUncertain(err))
		}
	})
}

func TestRemoteProviderExecuteClassifiesSubmissionCertainty(t *testing.T) {
	for _, test := range []struct {
		name      string
		status    int
		want      error
		uncertain bool
	}{
		{name: "bad request", status: http.StatusBadRequest, want: domainsandbox.ErrInvalidInput},
		{name: "unauthorized", status: http.StatusUnauthorized, want: domainsandbox.ErrCredentialInvalid},
		{name: "not found", status: http.StatusNotFound, want: domainsandbox.ErrExecutionForbidden},
		{name: "capacity", status: http.StatusTooManyRequests, want: domainsandbox.ErrCapacityExhausted},
		{name: "server error", status: http.StatusInternalServerError, want: domainsandbox.ErrProviderUnhealthy, uncertain: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := mustRemoteProvider(t, providerDoerFunc(func(request *http.Request) (*http.Response, error) {
				return jsonResponse(request, test.status, `{}`), nil
			}))
			_, err := provider.Execute(context.Background(), validExecuteRequest())
			if !errors.Is(err, test.want) || qualitySubmissionIsUncertain(err) != test.uncertain {
				t.Fatalf("Execute error = %v, uncertain=%v", err, qualitySubmissionIsUncertain(err))
			}
		})
	}

	provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial detail must-not-leak")
	}))
	_, err := provider.Execute(context.Background(), validExecuteRequest())
	if !errors.Is(err, domainsandbox.ErrProviderUnhealthy) || !qualitySubmissionIsUncertain(err) || strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("transport error = %v, uncertain=%v", err, qualitySubmissionIsUncertain(err))
	}
}

func TestSandboxProviderSecretsAreRedactedForFormatting(t *testing.T) {
	artifact := ArtifactReference{
		Direction: ArtifactDirectionDownload,
		URL:       "https://artifacts.example.test/v1/grants/complete-secret-url",
		Token:     "artifact-bearer-token",
		Digest:    "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Size:      123,
		MediaType: "application/octet-stream",
		ExpiresAt: time.Unix(2_100_000_000, 0).UTC(),
	}
	provider := mustRemoteProvider(t, providerDoerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("unexpected transport")
	}))
	for _, value := range []any{artifact, provider} {
		formatted := fmt.Sprintf("%v %+v %#v", value, value, value)
		goStringer, ok := value.(fmt.GoStringer)
		if !ok {
			t.Fatalf("%T does not implement fmt.GoStringer", value)
		}
		formatted += " " + goStringer.GoString()
		for _, secret := range []string{artifact.URL, artifact.Token, validRemoteProviderConfig().Credential} {
			if strings.Contains(formatted, secret) {
				t.Fatalf("%T formatting leaked %q: %s", value, secret, formatted)
			}
		}
	}
	artifactText := fmt.Sprintf("%+v", artifact)
	if !strings.Contains(artifactText, artifact.Digest) || !strings.Contains(artifactText, "123") {
		t.Fatalf("redacted artifact omitted safe metadata: %s", artifactText)
	}
	var logOutput bytes.Buffer
	logger := logs.DefaultLogger()
	logger.SetOutput(&logOutput)
	t.Cleanup(func() { logger.SetOutput(os.Stderr) })
	logs.CtxInfof(context.Background(), "artifact=%+v provider=%#v", artifact, provider)
	for _, secret := range []string{artifact.URL, artifact.Token, validRemoteProviderConfig().Credential} {
		if strings.Contains(logOutput.String(), secret) {
			t.Fatalf("repository logger leaked %q: %s", secret, logOutput.String())
		}
	}
	encoded, err := json.Marshal(artifact)
	if err != nil || strings.Contains(string(encoded), artifact.URL) || strings.Contains(string(encoded), artifact.Token) {
		t.Fatalf("artifact JSON = %s, %v", encoded, err)
	}
}

func TestNormalizeExecuteResultRejectsStatusExitCodeMismatch(t *testing.T) {
	zero, one := 0, 1
	for _, result := range []ExecuteResult{
		{ExecutionID: "exec-failed-zero", Status: ExecutionStatusFailed, ExitCode: &zero},
		{ExecutionID: "exec-canceled-exit", Status: ExecutionStatusCanceled, ExitCode: &one},
		{ExecutionID: "exec-timeout-exit", Status: ExecutionStatusTimedOut, ExitCode: &one},
	} {
		if _, err := normalizeExecuteResult(result, 1024); !errors.Is(err, domainsandbox.ErrInvalidInput) {
			t.Fatalf("invalid status/exit result accepted: %#v, err=%v", result, err)
		}
	}
}
