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

package deerflowparity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const (
	defaultClientTimeout    = 90 * time.Second
	defaultMaxResponseBytes = int64(4 * 1024 * 1024)
	defaultMaxRequestBytes  = int64(1024 * 1024)
	defaultMaxSSEBytes      = int64(16 * 1024 * 1024)
	defaultMaxSSEFrames     = 8000
	defaultMaxSSELineBytes  = 512 * 1024
)

var (
	errCrossOriginRedirect = errors.New("cross-origin redirect is not allowed")
	errRedirectLimit       = errors.New("redirect limit exceeded")
)

type ClientOptions struct {
	Timeout          time.Duration
	MaxResponseBytes int64
	AllowRemote      bool
}

type Credentials struct {
	Email    string
	Password string
}

type ThreadOptions struct {
	SpaceID string
}

type RunInput struct {
	AssistantID  string         `json:"assistant_id"`
	Input        map[string]any `json:"input"`
	Config       map[string]any `json:"config,omitempty"`
	Context      map[string]any `json:"context,omitempty"`
	StreamMode   []string       `json:"stream_mode,omitempty"`
	OnDisconnect string         `json:"on_disconnect,omitempty"`
}

type StreamResult struct {
	ThreadID    string
	RunID       string
	Terminal    string
	Frames      []SSEFrame
	LastEventID string
}

type RunHandle struct {
	ThreadID string
	RunID    string
	Status   string
}

type StreamOptions struct {
	AfterEventID    string
	StopAfterFrames int
}

type PageRequest struct {
	Limit     int
	BeforeSeq int64
	AfterSeq  int64
}

type MessagePage struct {
	Data    []map[string]any `json:"data"`
	HasMore bool             `json:"has_more"`
}

type safeHTTPClient struct {
	baseURL          *url.URL
	client           *http.Client
	maxResponseBytes int64
}

func newSafeHTTPClient(rawBaseURL string, options ClientOptions) (*safeHTTPClient, error) {
	baseURL, err := url.Parse(strings.TrimSpace(rawBaseURL))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, errors.New("base URL is invalid")
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, errors.New("base URL scheme is unsupported")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" || (baseURL.Path != "" && baseURL.Path != "/") {
		return nil, errors.New("base URL must not contain a path, query or fragment")
	}
	if !options.AllowRemote && !isLoopbackHost(baseURL.Hostname()) {
		return nil, errors.New("base URL must use a loopback host")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create HTTP cookie jar: %w", err)
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultClientTimeout
	}
	maxResponseBytes := options.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	httpClient := &http.Client{
		Timeout: timeout,
		Jar:     jar,
		CheckRedirect: func(request *http.Request, previous []*http.Request) error {
			if len(previous) >= 10 {
				return errRedirectLimit
			}
			if !sameOrigin(baseURL, request.URL) {
				return errCrossOriginRedirect
			}
			return nil
		},
	}
	return &safeHTTPClient{
		baseURL:          baseURL,
		client:           httpClient,
		maxResponseBytes: maxResponseBytes,
	}, nil
}

func sameOrigin(left, right *url.URL) bool {
	return left != nil && right != nil &&
		strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Host, right.Host)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *safeHTTPClient) doJSON(
	ctx context.Context,
	method string,
	endpointName string,
	path string,
	body any,
	headers http.Header,
	out any,
) error {
	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s encode request: %w", endpointName, err)
		}
		if int64(len(encoded)) > defaultMaxRequestBytes {
			return fmt.Errorf("%s request exceeds size limit", endpointName)
		}
		requestBody = bytes.NewReader(encoded)
	}
	requestHeaders := cloneHeaders(headers)
	if body != nil {
		requestHeaders.Set("Content-Type", "application/json")
	}
	return c.do(ctx, method, endpointName, path, requestBody, requestHeaders, out)
}

func (c *safeHTTPClient) doForm(
	ctx context.Context,
	endpointName string,
	path string,
	values url.Values,
	headers http.Header,
	out any,
) error {
	encoded := values.Encode()
	if int64(len(encoded)) > defaultMaxRequestBytes {
		return fmt.Errorf("%s request exceeds size limit", endpointName)
	}
	requestHeaders := cloneHeaders(headers)
	requestHeaders.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(ctx, http.MethodPost, endpointName, path, strings.NewReader(encoded), requestHeaders, out)
}

func (c *safeHTTPClient) do(
	ctx context.Context,
	method string,
	endpointName string,
	path string,
	body io.Reader,
	headers http.Header,
	out any,
) error {
	response, err := c.send(ctx, method, endpointName, path, body, headers)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = readBounded(response.Body, c.maxResponseBytes)
		return fmt.Errorf("%s returned HTTP %d", endpointName, response.StatusCode)
	}
	payload, err := readBounded(response.Body, c.maxResponseBytes)
	if err != nil {
		return fmt.Errorf("%s %w", endpointName, err)
	}
	if out == nil || len(bytes.TrimSpace(payload)) == 0 {
		return nil
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "application/json") && !strings.Contains(contentType, "+json") {
		return fmt.Errorf("%s returned unsupported content type", endpointName)
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("%s returned invalid JSON", endpointName)
	}
	return nil
}

func (c *safeHTTPClient) openStream(
	ctx context.Context,
	method string,
	endpointName string,
	path string,
	body any,
	headers http.Header,
) (*http.Response, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%s encode request: %w", endpointName, err)
	}
	if int64(len(encoded)) > defaultMaxRequestBytes {
		return nil, fmt.Errorf("%s request exceeds size limit", endpointName)
	}
	requestHeaders := cloneHeaders(headers)
	requestHeaders.Set("Content-Type", "application/json")
	return c.openSSE(ctx, method, endpointName, path, bytes.NewReader(encoded), requestHeaders)
}

func (c *safeHTTPClient) openSSE(
	ctx context.Context,
	method string,
	endpointName string,
	path string,
	body io.Reader,
	headers http.Header,
) (*http.Response, error) {
	requestHeaders := cloneHeaders(headers)
	requestHeaders.Set("Accept", "text/event-stream")
	response, err := c.send(ctx, method, endpointName, path, body, requestHeaders)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer response.Body.Close()
		_, _ = readBounded(response.Body, c.maxResponseBytes)
		return nil, fmt.Errorf("%s returned HTTP %d", endpointName, response.StatusCode)
	}
	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		response.Body.Close()
		return nil, fmt.Errorf("%s returned unsupported content type", endpointName)
	}
	return response, nil
}

func (c *safeHTTPClient) send(
	ctx context.Context,
	method string,
	endpointName string,
	path string,
	body io.Reader,
	headers http.Header,
) (*http.Response, error) {
	target, err := c.resolve(path)
	if err != nil {
		return nil, fmt.Errorf("%s endpoint is invalid", endpointName)
	}
	request, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, fmt.Errorf("%s create request: %w", endpointName, err)
	}
	request.Header = cloneHeaders(headers)
	response, err := c.client.Do(request)
	if err != nil {
		if errors.Is(err, errCrossOriginRedirect) {
			return nil, fmt.Errorf("%s rejected cross-origin redirect", endpointName)
		}
		if errors.Is(err, errRedirectLimit) {
			return nil, fmt.Errorf("%s exceeded redirect limit", endpointName)
		}
		return nil, fmt.Errorf("%s request failed: %w", endpointName, err)
	}
	return response, nil
}

func (c *safeHTTPClient) resolve(path string) (*url.URL, error) {
	relative, err := url.Parse(path)
	if err != nil || relative.IsAbs() || relative.Host != "" || !strings.HasPrefix(relative.Path, "/") {
		return nil, errors.New("invalid relative endpoint")
	}
	for _, segment := range strings.Split(relative.EscapedPath(), "/") {
		if segment == ".." || strings.EqualFold(segment, "%2e%2e") {
			return nil, errors.New("endpoint path traversal")
		}
	}
	return c.baseURL.ResolveReference(relative), nil
}

func (c *safeHTTPClient) cookie(name string) (*http.Cookie, bool) {
	for _, cookie := range c.client.Jar.Cookies(c.baseURL) {
		if cookie.Name == name {
			return cookie, true
		}
	}
	return nil, false
}

func readBounded(reader io.Reader, maximum int64) ([]byte, error) {
	payload, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, errors.New("response read failed")
	}
	if int64(len(payload)) > maximum {
		return nil, errors.New("response exceeds size limit")
	}
	return payload, nil
}

func cloneHeaders(headers http.Header) http.Header {
	if headers == nil {
		return make(http.Header)
	}
	return headers.Clone()
}
