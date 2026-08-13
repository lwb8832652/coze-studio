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

package aio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	sandboxapi "github.com/agent-infra/sandbox-sdk-go"
	sandboxclient "github.com/agent-infra/sandbox-sdk-go/client"
	"github.com/agent-infra/sandbox-sdk-go/core"
	"github.com/agent-infra/sandbox-sdk-go/option"
)

const (
	maxUpstreamHTTPTimeout   = 30 * time.Second
	maxUpstreamResponseBytes = int64(64 * 1024 * 1024)
	maxHealthResponseBytes   = int64(16)

	ReasonUpstreamCancelled         = "AIO_UPSTREAM_CANCELLED"
	ReasonUpstreamTimeout           = "AIO_UPSTREAM_TIMEOUT"
	ReasonUpstreamUnauthorized      = "AIO_UPSTREAM_UNAUTHORIZED"
	ReasonUpstreamNotFound          = "AIO_UPSTREAM_NOT_FOUND"
	ReasonUpstreamRejected          = "AIO_UPSTREAM_REJECTED"
	ReasonUpstreamServerError       = "AIO_UPSTREAM_SERVER_ERROR"
	ReasonUpstreamUnavailable       = "AIO_UPSTREAM_UNAVAILABLE"
	ReasonUpstreamMalformedResponse = "AIO_UPSTREAM_MALFORMED_RESPONSE"
	ReasonUpstreamResponseTooLarge  = "AIO_UPSTREAM_RESPONSE_TOO_LARGE"
)

var ErrInvalidUpstreamConfig = errors.New("invalid AIO upstream configuration")

var errUpstreamResponseTooLarge = errors.New("AIO upstream response exceeded limit")

type UpstreamClientConfig struct {
	BaseURL    string
	BearerJWT  string
	HTTPClient *http.Client
}

type UpstreamClient struct {
	sdk          *sandboxclient.Client
	baseURL      string
	httpClient   *http.Client
	headers      http.Header
	lockIdentity string
}

func NewUpstreamClient(config UpstreamClientConfig) (*UpstreamClient, error) {
	baseURL := strings.TrimSuffix(config.BaseURL, "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" {
		return nil, ErrInvalidUpstreamConfig
	}
	if config.HTTPClient == nil || config.HTTPClient.Timeout <= 0 || config.HTTPClient.Timeout > maxUpstreamHTTPTimeout {
		return nil, ErrInvalidUpstreamConfig
	}
	httpClient := isolatedHTTPClient(config.HTTPClient)

	headers := make(http.Header)
	if config.BearerJWT != "" {
		headers.Set("Authorization", "Bearer "+config.BearerJWT)
	}
	sdk := sandboxclient.NewClient(
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(httpClient),
		option.WithHTTPHeader(headers),
		option.WithMaxAttempts(1),
	)
	return &UpstreamClient{
		sdk: sdk, baseURL: baseURL, httpClient: httpClient, headers: headers.Clone(), lockIdentity: baseURL,
	}, nil
}

func (client *UpstreamClient) ShellLockIdentity() string { return client.lockIdentity }

func (client *UpstreamClient) Health(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/v1/ping", nil)
	if err != nil {
		return sanitizeUpstreamError(ctx, err)
	}
	request.Header = client.headers.Clone()
	response, err := client.httpClient.Do(request)
	if err != nil {
		return sanitizeUpstreamError(ctx, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode > http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
			return &UpstreamError{reasonCode: ReasonUpstreamMalformedResponse}
		}
		return sanitizeUpstreamError(ctx, core.NewAPIError(response.StatusCode, response.Header, nil))
	}
	mediaType, parameters, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != "text/plain" || len(parameters) != 1 || !strings.EqualFold(parameters["charset"], "utf-8") {
		return &UpstreamError{reasonCode: ReasonUpstreamMalformedResponse}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxHealthResponseBytes+1))
	if err != nil {
		return sanitizeUpstreamError(ctx, err)
	}
	if int64(len(body)) > maxHealthResponseBytes {
		return &UpstreamError{reasonCode: ReasonUpstreamResponseTooLarge}
	}
	if string(body) != "pong" {
		return &UpstreamError{reasonCode: ReasonUpstreamMalformedResponse}
	}
	return nil
}

func (client *UpstreamClient) ListSessions(ctx context.Context) (*sandboxapi.ResponseActiveShellSessionsResult, error) {
	response, err := client.sdk.Shell.ListSessions(ctx)
	if err != nil {
		return nil, sanitizeUpstreamError(ctx, err)
	}
	if response == nil || response.Success == nil || !*response.Success || response.Data == nil || response.Data.Sessions == nil {
		return nil, &UpstreamError{reasonCode: ReasonUpstreamMalformedResponse}
	}
	for _, session := range response.Data.Sessions {
		if session == nil {
			return nil, &UpstreamError{reasonCode: ReasonUpstreamMalformedResponse}
		}
	}
	return response, nil
}

func (client *UpstreamClient) Create(ctx context.Context, request *sandboxapi.ShellCreateSessionRequest) (*sandboxapi.ResponseShellCreateSessionResponse, error) {
	response, err := client.sdk.Shell.CreateSession(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Exec(ctx context.Context, request *sandboxapi.ShellExecRequest) (*sandboxapi.ResponseShellCommandResult, error) {
	response, err := client.sdk.Shell.ExecCommand(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) View(ctx context.Context, request *sandboxapi.ShellViewRequest) (*sandboxapi.ResponseShellViewResult, error) {
	response, err := client.sdk.Shell.View(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Wait(ctx context.Context, request *sandboxapi.ShellWaitRequest) (*sandboxapi.ResponseShellWaitResult, error) {
	response, err := client.sdk.Shell.WaitForProcess(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Kill(ctx context.Context, request *sandboxapi.ShellKillProcessRequest) (*sandboxapi.ResponseShellKillResult, error) {
	response, err := client.sdk.Shell.KillProcess(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Cleanup(ctx context.Context, sessionID string) error {
	_, err := client.sdk.Shell.CleanupSession(ctx, sessionID)
	return sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Read(ctx context.Context, request *sandboxapi.FileReadRequest) (*sandboxapi.ResponseFileReadResult, error) {
	response, err := client.sdk.File.ReadFile(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Write(ctx context.Context, request *sandboxapi.FileWriteRequest) (*sandboxapi.ResponseFileWriteResult, error) {
	response, err := client.sdk.File.WriteFile(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) List(ctx context.Context, request *sandboxapi.FileListRequest) (*sandboxapi.ResponseFileListResult, error) {
	response, err := client.sdk.File.ListPath(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Glob(ctx context.Context, request *sandboxapi.FileGlobRequest) (*sandboxapi.ResponseFileGlobResult, error) {
	response, err := client.sdk.File.GlobFiles(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Grep(ctx context.Context, request *sandboxapi.FileGrepRequest) (*sandboxapi.ResponseFileGrepResult, error) {
	response, err := client.sdk.File.GrepFiles(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Replace(ctx context.Context, request *sandboxapi.FileReplaceRequest) (*sandboxapi.ResponseFileReplaceResult, error) {
	response, err := client.sdk.File.ReplaceInFile(ctx, request)
	return response, sanitizeUpstreamError(ctx, err)
}

func (client *UpstreamClient) Download(ctx context.Context, path string) (io.Reader, error) {
	response, err := client.sdk.File.DownloadFile(ctx, &sandboxapi.FileDownloadFileRequest{Path: path})
	return response, sanitizeUpstreamError(ctx, err)
}

type UpstreamError struct {
	reasonCode string
}

func (err *UpstreamError) Error() string {
	return "AIO upstream request failed: " + err.reasonCode
}

func ReasonCode(err error) string {
	var upstreamError *UpstreamError
	if errors.As(err, &upstreamError) {
		return upstreamError.reasonCode
	}
	return ""
}

func sanitizeUpstreamError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return &UpstreamError{reasonCode: ReasonUpstreamCancelled}
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return &UpstreamError{reasonCode: ReasonUpstreamTimeout}
	}
	if errors.Is(err, errUpstreamResponseTooLarge) {
		return &UpstreamError{reasonCode: ReasonUpstreamResponseTooLarge}
	}
	var apiError *core.APIError
	if errors.As(err, &apiError) {
		switch {
		case apiError.StatusCode == http.StatusUnauthorized || apiError.StatusCode == http.StatusForbidden:
			return &UpstreamError{reasonCode: ReasonUpstreamUnauthorized}
		case apiError.StatusCode == http.StatusNotFound:
			return &UpstreamError{reasonCode: ReasonUpstreamNotFound}
		case apiError.StatusCode >= http.StatusMultipleChoices && apiError.StatusCode < http.StatusInternalServerError:
			return &UpstreamError{reasonCode: ReasonUpstreamRejected}
		case apiError.StatusCode >= http.StatusInternalServerError:
			return &UpstreamError{reasonCode: ReasonUpstreamServerError}
		}
	}
	return &UpstreamError{reasonCode: ReasonUpstreamUnavailable}
}

func (err *UpstreamError) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprint(state, err.Error())
}

func isolatedHTTPClient(source *http.Client) *http.Client {
	client := *source
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	client.Transport = isolatedTransport(source.Transport)
	return &client
}

func isolatedTransport(source http.RoundTripper) http.RoundTripper {
	if source == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = nil
		return &boundedResponseTransport{delegate: transport}
	}
	if transport, ok := source.(*http.Transport); ok {
		clone := transport.Clone()
		clone.Proxy = nil
		source = clone
	}
	return &boundedResponseTransport{delegate: source}
}

type boundedResponseTransport struct {
	delegate http.RoundTripper
}

func (transport *boundedResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.delegate.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.ContentLength > maxUpstreamResponseBytes {
		_ = response.Body.Close()
		return nil, errUpstreamResponseTooLarge
	}
	response.Body = &boundedResponseBody{delegate: response.Body, remaining: maxUpstreamResponseBytes}
	return response, nil
}

type boundedResponseBody struct {
	delegate  io.ReadCloser
	remaining int64
}

func (body *boundedResponseBody) Read(buffer []byte) (int, error) {
	if body.remaining < 0 {
		return 0, errUpstreamResponseTooLarge
	}
	limit := int64(len(buffer))
	if limit > body.remaining+1 {
		limit = body.remaining + 1
	}
	read, err := body.delegate.Read(buffer[:limit])
	body.remaining -= int64(read)
	if body.remaining < 0 {
		allowed := read + int(body.remaining)
		if allowed < 0 {
			allowed = 0
		}
		return allowed, errUpstreamResponseTooLarge
	}
	return read, err
}

func (body *boundedResponseBody) Close() error { return body.delegate.Close() }
