// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	mcpsdk "github.com/mark3labs/mcp-go/mcp"
)

const (
	defaultSSEEndpointTimeout = 30 * time.Second
	maxSSELineBytes           = 256 * 1024
	maxSSEEventBytes          = 8 * 1024 * 1024
	maxSSEDataLines           = 4096
)

type SSETransportOptions struct {
	URL             string
	Headers         map[string]string
	HTTPClient      *http.Client
	HTTPPolicy      *SafeHTTPPolicy
	EndpointTimeout time.Duration
}

type safeSSETransport struct {
	baseURL         *url.URL
	httpClient      *http.Client
	httpPolicy      *SafeHTTPPolicy
	headers         map[string]string
	endpointTimeout time.Duration

	endpointMu    sync.RWMutex
	endpoint      *url.URL
	endpointReady chan struct{}
	endpointOnce  sync.Once

	responsesMu sync.Mutex
	responses   map[string]chan *mcptransport.JSONRPCResponse

	notificationMu      sync.RWMutex
	notificationHandler func(mcpsdk.JSONRPCNotification)

	protocolVersion atomic.Value
	started         atomic.Bool
	closed          atomic.Bool
	cancel          context.CancelFunc
	streamBody      io.ReadCloser
	streamDone      chan struct{}
	streamDoneOnce  sync.Once
	streamStarted   atomic.Bool
	closeOnce       sync.Once
}

func NewSafeSSETransport(options SSETransportOptions) (mcptransport.Interface, error) {
	baseURL, err := url.Parse(strings.TrimSpace(options.URL))
	if err != nil || baseURL == nil || baseURL.Host == "" ||
		(baseURL.Scheme != "http" && baseURL.Scheme != "https") ||
		baseURL.User != nil || baseURL.Fragment != "" {
		return nil, ErrInvalidConnection
	}
	if options.HTTPPolicy != nil && options.HTTPPolicy.ValidateURL(baseURL) != nil {
		return nil, ErrInvalidConnection
	}
	endpointTimeout := options.EndpointTimeout
	if endpointTimeout <= 0 {
		endpointTimeout = defaultSSEEndpointTimeout
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: endpointTimeout}
	} else {
		clone := *client
		client = &clone
	}
	existingRedirectPolicy := client.CheckRedirect
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request == nil || request.URL == nil || !sameSSEOrigin(baseURL, request.URL) {
			return ErrInvalidConnection
		}
		if options.HTTPPolicy != nil && options.HTTPPolicy.ValidateURL(request.URL) != nil {
			return ErrInvalidConnection
		}
		if existingRedirectPolicy != nil {
			return existingRedirectPolicy(request, via)
		}
		if len(via) >= 10 {
			return ErrInvalidConnection
		}
		return nil
	}
	return &safeSSETransport{
		baseURL:         baseURL,
		httpClient:      client,
		httpPolicy:      options.HTTPPolicy,
		headers:         cloneStringMap(options.Headers),
		endpointTimeout: endpointTimeout,
		endpointReady:   make(chan struct{}),
		responses:       make(map[string]chan *mcptransport.JSONRPCResponse),
		streamDone:      make(chan struct{}),
	}, nil
}

func (t *safeSSETransport) Start(ctx context.Context) error {
	if t.closed.Load() {
		return ErrSessionUnavailable
	}
	if t.started.Load() {
		return nil
	}
	streamCtx, cancel := context.WithCancel(ctx)
	t.cancel = cancel
	request, err := http.NewRequestWithContext(streamCtx, http.MethodGet, t.baseURL.String(), nil)
	if err != nil {
		cancel()
		return ErrSessionUnavailable
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Cache-Control", "no-cache")
	t.applyHeaders(request)
	response, err := t.httpClient.Do(request)
	if err != nil {
		cancel()
		return ErrSessionUnavailable
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		cancel()
		return ErrSessionUnavailable
	}
	t.streamBody = response.Body
	t.streamStarted.Store(true)
	go t.readStream(response.Body)

	timer := time.NewTimer(t.endpointTimeout)
	defer timer.Stop()
	select {
	case <-t.endpointReady:
		t.started.Store(true)
		return nil
	case <-t.streamDone:
		return ErrSessionUnavailable
	case <-streamCtx.Done():
		return ErrSessionUnavailable
	case <-timer.C:
		cancel()
		return ErrSessionUnavailable
	}
}

func (t *safeSSETransport) readStream(reader io.ReadCloser) {
	defer func() {
		_ = reader.Close()
		t.streamDoneOnce.Do(func() { close(t.streamDone) })
	}()
	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, maxSSELineBytes+2)
	event := ""
	var data bytes.Buffer
	eventBytes := 0
	dataLines := 0
	reset := func() {
		event = ""
		data.Reset()
		eventBytes = 0
		dataLines = 0
	}
	dispatch := func() {
		if dataLines == 0 {
			return
		}
		if event == "" {
			event = "message"
		}
		t.handleEvent(event, data.String())
	}
	for scanner.Scan() {
		rawLine := scanner.Bytes()
		if len(rawLine) > maxSSELineBytes {
			return
		}
		line := rawLine
		if len(line) > 0 && line[len(line)-1] == '\r' {
			line = line[:len(line)-1]
		}
		if len(line) == 0 {
			dispatch()
			reset()
			continue
		}
		if len(rawLine) > maxSSEEventBytes-eventBytes {
			return
		}
		eventBytes += len(rawLine)
		if bytes.HasPrefix(line, []byte("event:")) {
			event = string(bytes.TrimSpace(line[len("event:"):]))
			continue
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			if dataLines >= maxSSEDataLines {
				return
			}
			payload := bytes.TrimSpace(line[len("data:"):])
			additionalBytes := len(payload)
			if dataLines > 0 {
				additionalBytes++
			}
			if additionalBytes > maxSSEEventBytes-data.Len() {
				return
			}
			if dataLines > 0 {
				_ = data.WriteByte('\n')
			}
			_, _ = data.Write(payload)
			dataLines++
		}
	}
	if scanner.Err() != nil {
		return
	}
	dispatch()
}

func (t *safeSSETransport) handleEvent(event, data string) {
	switch event {
	case "endpoint":
		endpoint, err := t.baseURL.Parse(data)
		if err != nil || endpoint == nil || endpoint.User != nil || endpoint.Fragment != "" ||
			!sameSSEOrigin(t.baseURL, endpoint) {
			return
		}
		if t.httpPolicy != nil && t.httpPolicy.ValidateURL(endpoint) != nil {
			return
		}
		t.endpointOnce.Do(func() {
			t.endpointMu.Lock()
			t.endpoint = endpoint
			t.endpointMu.Unlock()
			close(t.endpointReady)
		})
	case "message":
		var response mcptransport.JSONRPCResponse
		if json.Unmarshal([]byte(data), &response) != nil {
			return
		}
		if response.ID.IsNil() {
			var notification mcpsdk.JSONRPCNotification
			if json.Unmarshal([]byte(data), &notification) != nil {
				return
			}
			t.notificationMu.RLock()
			handler := t.notificationHandler
			t.notificationMu.RUnlock()
			if handler != nil {
				handler(notification)
			}
			return
		}
		key := response.ID.String()
		t.responsesMu.Lock()
		responseChannel := t.responses[key]
		delete(t.responses, key)
		t.responsesMu.Unlock()
		if responseChannel != nil {
			select {
			case responseChannel <- &response:
			case <-t.streamDone:
			}
		}
	}
}

func (t *safeSSETransport) SendRequest(
	ctx context.Context,
	request mcptransport.JSONRPCRequest,
) (*mcptransport.JSONRPCResponse, error) {
	endpoint := t.currentEndpoint()
	if !t.started.Load() || t.closed.Load() || endpoint == nil {
		return nil, ErrSessionUnavailable
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, ErrSessionUnavailable
	}
	httpRequest, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint.String(),
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, ErrSessionUnavailable
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	t.applyHeaders(httpRequest)
	for key, values := range request.Header {
		if _, exists := httpRequest.Header[key]; !exists {
			httpRequest.Header[key] = append([]string(nil), values...)
		}
	}

	key := request.ID.String()
	responseChannel := make(chan *mcptransport.JSONRPCResponse, 1)
	t.responsesMu.Lock()
	if _, duplicate := t.responses[key]; duplicate {
		t.responsesMu.Unlock()
		return nil, ErrSessionUnavailable
	}
	t.responses[key] = responseChannel
	t.responsesMu.Unlock()
	removeResponse := func() {
		t.responsesMu.Lock()
		if t.responses[key] == responseChannel {
			delete(t.responses, key)
		}
		t.responsesMu.Unlock()
	}

	response, err := t.httpClient.Do(httpRequest)
	if err != nil {
		removeResponse()
		return nil, ErrSessionUnavailable
	}
	_, drainErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if drainErr != nil || closeErr != nil ||
		(response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted) {
		removeResponse()
		return nil, ErrSessionUnavailable
	}

	select {
	case result := <-responseChannel:
		return result, nil
	case <-ctx.Done():
		removeResponse()
		return nil, ctx.Err()
	case <-t.streamDone:
		removeResponse()
		return nil, ErrSessionUnavailable
	}
}

func (t *safeSSETransport) SendNotification(
	ctx context.Context,
	notification mcpsdk.JSONRPCNotification,
) error {
	endpoint := t.currentEndpoint()
	if !t.started.Load() || t.closed.Load() || endpoint == nil {
		return ErrSessionUnavailable
	}
	payload, err := json.Marshal(notification)
	if err != nil {
		return ErrSessionUnavailable
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		endpoint.String(),
		bytes.NewReader(payload),
	)
	if err != nil {
		return ErrSessionUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	t.applyHeaders(request)
	response, err := t.httpClient.Do(request)
	if err != nil {
		return ErrSessionUnavailable
	}
	_, drainErr := io.Copy(io.Discard, response.Body)
	closeErr := response.Body.Close()
	if drainErr != nil || closeErr != nil ||
		(response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted &&
			response.StatusCode != http.StatusNoContent) {
		return ErrSessionUnavailable
	}
	return nil
}

func (t *safeSSETransport) SetNotificationHandler(
	handler func(notification mcpsdk.JSONRPCNotification),
) {
	t.notificationMu.Lock()
	t.notificationHandler = handler
	t.notificationMu.Unlock()
}

func (t *safeSSETransport) SetProtocolVersion(version string) {
	t.protocolVersion.Store(strings.TrimSpace(version))
}

func (t *safeSSETransport) GetSessionId() string {
	return ""
}

func (t *safeSSETransport) Close() error {
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		if t.cancel != nil {
			t.cancel()
		}
		if t.streamBody != nil {
			_ = t.streamBody.Close()
		}
		if t.streamStarted.Load() {
			<-t.streamDone
		}
		t.responsesMu.Lock()
		t.responses = make(map[string]chan *mcptransport.JSONRPCResponse)
		t.responsesMu.Unlock()
	})
	return nil
}

func (t *safeSSETransport) currentEndpoint() *url.URL {
	t.endpointMu.RLock()
	defer t.endpointMu.RUnlock()
	if t.endpoint == nil {
		return nil
	}
	copy := *t.endpoint
	return &copy
}

func (t *safeSSETransport) applyHeaders(request *http.Request) {
	for key, value := range t.headers {
		request.Header.Set(key, value)
	}
	if version, ok := t.protocolVersion.Load().(string); ok && version != "" {
		request.Header.Set(mcptransport.HeaderKeyProtocolVersion, version)
	}
}

func sameSSEOrigin(left, right *url.URL) bool {
	same, err := sameHTTPOrigin(left, right)
	return err == nil && same
}
