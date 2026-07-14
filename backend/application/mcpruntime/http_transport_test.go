// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSafeHTTPPolicyCanonicalizesIDNAAndRequiresExactPort(t *testing.T) {
	t.Parallel()

	policy := mustSafeHTTPPolicy(t, SafeHTTPPolicyOptions{
		AllowedHosts: []string{"MCP.Example.COM.", "BÜCHER.Example.:8443"},
	})
	for _, rawURL := range []string{
		"https://mcp.example.com/path",
		"https://MCP.EXAMPLE.COM.:443/path",
		"https://xn--bcher-kva.example:8443/path",
		"https://BÜCHER.example.:8443/path",
	} {
		if err := policy.ValidateURL(mustParseURL(t, rawURL)); err != nil {
			t.Fatalf("expected URL %q to pass: %v", rawURL, err)
		}
	}
	for _, rawURL := range []string{
		"https://mcp.example.com:8443/path",
		"https://xn--bcher-kva.example/path",
		"https://mcp.example.com.evil/path",
		"https://user:password@mcp.example.com/path",
		"https://mcp.example.com/path#fragment",
	} {
		if err := policy.ValidateURL(mustParseURL(t, rawURL)); !errors.Is(err, ErrInvalidConnection) {
			t.Fatalf("expected URL %q to fail closed, got %v", rawURL, err)
		}
	}
}

func TestSafeHTTPPolicyValidationFailsClosedAtStartup(t *testing.T) {
	t.Parallel()

	policy := testPolicy(t.TempDir())
	policy.RemoteAllowedHosts = []string{"mcp.example.test/path"}
	if err := policy.Validate(); !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("malformed remote allowlist must fail startup, got %v", err)
	}
}

func TestSafeSSEOriginUsesCanonicalIDNAAndExactPort(t *testing.T) {
	t.Parallel()

	unicodeOrigin := mustParseURL(t, "https://BÜCHER.example./sse")
	asciiOrigin := mustParseURL(t, "https://xn--bcher-kva.example:443/messages")
	if !sameSSEOrigin(unicodeOrigin, asciiOrigin) {
		t.Fatal("canonical IDNA origins must match")
	}
	if sameSSEOrigin(unicodeOrigin, mustParseURL(t, "https://xn--bcher-kva.example:8443/messages")) {
		t.Fatal("different endpoint port must not match")
	}
}

func TestSafeHTTPTransportRejectsUnsafeResolvedAddressesAndDNSRebinding(t *testing.T) {
	t.Parallel()

	policy := mustSafeHTTPPolicy(t, SafeHTTPPolicyOptions{AllowedHosts: []string{"mcp.example.test"}})
	for _, address := range []string{
		"0.0.0.0",
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.0.1",
		"100.64.0.1",
		"169.254.169.254",
		"192.0.2.1",
		"192.88.99.1",
		"198.18.0.1",
		"198.51.100.1",
		"203.0.113.1",
		"224.0.0.1",
		"240.0.0.1",
		"::",
		"::1",
		"64:ff9b::a9fe:a9fe",
		"64:ff9b::a00:1",
		"64:ff9b:1:0:a00:1::",
		"100::1",
		"2001::1",
		"2001:2::1",
		"2001:db8::1",
		"2002:a9fe:a9fe::1",
		"3fff::1",
		"5f00::1",
		"fe80::1",
		"fec0::1",
		"fc00::1",
		"ff02::1",
		"::ffff:127.0.0.1",
		"::ffff:169.254.169.254",
		"fd00:ec2::254",
	} {
		address := address
		t.Run(address, func(t *testing.T) {
			transport, err := NewSafeHTTPTransport(SafeHTTPClientOptions{
				Policy:   policy,
				Resolver: &sequenceHTTPResolver{results: [][]netip.Addr{{netip.MustParseAddr(address)}}},
				Dialer:   &recordingHTTPDialer{},
			})
			if err != nil {
				t.Fatalf("new safe HTTP transport: %v", err)
			}
			_, err = transport.dialContext(context.Background(), "tcp", "mcp.example.test:443")
			if !errors.Is(err, ErrUnsafeRemoteAddress) {
				t.Fatalf("address %s must be rejected, got %v", address, err)
			}
		})
	}

	resolver := &sequenceHTTPResolver{results: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")},
		{netip.MustParseAddr("127.0.0.1")},
	}}
	dialer := &recordingHTTPDialer{}
	transport, err := NewSafeHTTPTransport(SafeHTTPClientOptions{
		Policy:   policy,
		Resolver: resolver,
		Dialer:   dialer,
	})
	if err != nil {
		t.Fatalf("new safe HTTP transport: %v", err)
	}
	connection, err := transport.dialContext(context.Background(), "tcp", "mcp.example.test:443")
	if err != nil {
		t.Fatalf("first public DNS result: %v", err)
	}
	_ = connection.Close()
	if _, err := transport.dialContext(context.Background(), "tcp", "mcp.example.test:443"); !errors.Is(err, ErrUnsafeRemoteAddress) {
		t.Fatalf("rebound private result must fail, got %v", err)
	}
	if resolver.callCount() != 2 {
		t.Fatalf("DNS resolver calls = %d, want 2", resolver.callCount())
	}
	if got := dialer.addresses(); len(got) != 1 || got[0] != "93.184.216.34:443" {
		t.Fatalf("dialed addresses = %#v", got)
	}
}

func TestSafeHTTPTransportAllowsLoopbackOnlyForExplicitLocalDebug(t *testing.T) {
	t.Parallel()

	resolver := &sequenceHTTPResolver{results: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}}
	allowedPolicy := mustSafeHTTPPolicy(t, SafeHTTPPolicyOptions{
		AllowedHosts:    []string{"localhost:8080"},
		AllowLocalDebug: true,
	})
	if err := allowedPolicy.ValidateURL(mustParseURL(t, "http://LOCALHOST.:8080/sse")); err != nil {
		t.Fatalf("explicit local debug URL: %v", err)
	}
	transport, err := NewSafeHTTPTransport(SafeHTTPClientOptions{
		Policy:   allowedPolicy,
		Resolver: resolver,
		Dialer:   &recordingHTTPDialer{},
	})
	if err != nil {
		t.Fatalf("new safe HTTP transport: %v", err)
	}
	connection, err := transport.dialContext(context.Background(), "tcp", "localhost:8080")
	if err != nil {
		t.Fatalf("explicit localhost loopback: %v", err)
	}
	_ = connection.Close()

	for _, options := range []SafeHTTPPolicyOptions{
		{AllowedHosts: []string{"localhost:8080"}},
		{AllowedHosts: []string{"localhost"}, AllowLocalDebug: true},
		{AllowedHosts: []string{"127.0.0.1:8080"}, AllowLocalDebug: true},
	} {
		policy := mustSafeHTTPPolicy(t, options)
		if err := policy.ValidateURL(mustParseURL(t, "http://localhost:8080/sse")); !errors.Is(err, ErrInvalidConnection) {
			t.Fatalf("local policy %#v must fail closed, got %v", options, err)
		}
	}
}

func TestSafeHTTPTransportDisablesEnvironmentProxyAndDialsValidatedIP(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")

	policy := mustSafeHTTPPolicy(t, SafeHTTPPolicyOptions{AllowedHosts: []string{"mcp.example.test"}})
	dialer := &recordingHTTPDialer{}
	transport, err := NewSafeHTTPTransport(SafeHTTPClientOptions{
		Policy:   policy,
		Resolver: &sequenceHTTPResolver{results: [][]netip.Addr{{netip.MustParseAddr("93.184.216.34")}}},
		Dialer:   dialer,
	})
	if err != nil {
		t.Fatalf("new safe HTTP transport: %v", err)
	}
	if transport.base.Proxy != nil {
		t.Fatal("safe transport must not use ProxyFromEnvironment")
	}
	connection, err := transport.dialContext(context.Background(), "tcp", "mcp.example.test:443")
	if err != nil {
		t.Fatalf("dial validated address: %v", err)
	}
	_ = connection.Close()
	if got := dialer.addresses(); len(got) != 1 || got[0] != "93.184.216.34:443" {
		t.Fatalf("dialed addresses = %#v", got)
	}
}

func TestSafeHTTPClientRedirectPolicyDoesNotForwardSecretCrossOrigin(t *testing.T) {
	var crossCalls atomic.Int32
	var crossAuthorization atomic.Value
	crossServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		crossCalls.Add(1)
		crossAuthorization.Store(request.Header.Get("Authorization"))
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer crossServer.Close()
	crossURL := localhostTestURL(t, crossServer.URL)

	var sameAuthorization atomic.Value
	var originServer *httptest.Server
	originServer = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/same":
			http.Redirect(writer, request, "/final", http.StatusFound)
		case "/final":
			sameAuthorization.Store(request.Header.Get("Authorization"))
			writer.WriteHeader(http.StatusNoContent)
		case "/cross":
			http.Redirect(writer, request, crossURL, http.StatusFound)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer originServer.Close()
	originURL := localhostTestURL(t, originServer.URL)

	policy := mustSafeHTTPPolicy(t, SafeHTTPPolicyOptions{
		AllowedHosts:    []string{mustParseURL(t, originURL).Host, mustParseURL(t, crossURL).Host},
		AllowLocalDebug: true,
	})
	client, err := NewSafeHTTPClient(SafeHTTPClientOptions{Policy: policy, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("new safe HTTP client: %v", err)
	}
	defer closeSafeHTTPClient(client)

	request, _ := http.NewRequest(http.MethodGet, originURL+"/same", nil)
	request.Header.Set("Authorization", "Bearer projected-secret")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("same-origin redirect: %v", err)
	}
	_ = response.Body.Close()
	if got, _ := sameAuthorization.Load().(string); got != "Bearer projected-secret" {
		t.Fatalf("same-origin authorization = %q", got)
	}

	request, _ = http.NewRequest(http.MethodGet, originURL+"/cross", nil)
	request.Header.Set("Authorization", "Bearer projected-secret")
	_, err = client.Do(request)
	if !errors.Is(err, ErrInvalidConnection) {
		t.Fatalf("cross-origin redirect must fail closed, got %v", err)
	}
	if crossCalls.Load() != 0 {
		t.Fatalf("cross-origin server received %d requests", crossCalls.Load())
	}
	if got, _ := crossAuthorization.Load().(string); got != "" {
		t.Fatalf("cross-origin authorization leaked: %q", got)
	}

	downgrade := &http.Request{URL: mustParseURL(t, "http://mcp.example.test/path")}
	via := []*http.Request{{URL: mustParseURL(t, "https://mcp.example.test/path")}}
	if err := client.CheckRedirect(downgrade, via); !errors.Is(err, ErrInvalidConnection) {
		t.Fatalf("HTTPS downgrade must fail closed, got %v", err)
	}
	if err := client.CheckRedirect(via[0], []*http.Request{via[0], via[0], via[0], via[0]}); !errors.Is(err, ErrInvalidConnection) {
		t.Fatalf("redirect limit must fail closed, got %v", err)
	}
}

func TestSafeHTTPClientCapsOrdinaryResponseBeforeJSONDecode(t *testing.T) {
	const (
		bodyLimit = 64
		secret    = "oversized-body-secret-must-not-leak"
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/exact" {
			_, _ = io.WriteString(writer, strings.Repeat("x", bodyLimit))
			return
		}
		_, _ = io.WriteString(writer, `{"secret":"`+secret+`","padding":"`+strings.Repeat("x", bodyLimit)+`"}`)
	}))
	defer server.Close()
	targetURL := localhostTestURL(t, server.URL)
	policy := mustSafeHTTPPolicy(t, SafeHTTPPolicyOptions{
		AllowedHosts:    []string{mustParseURL(t, targetURL).Host},
		AllowLocalDebug: true,
	})
	client, err := NewSafeHTTPClient(SafeHTTPClientOptions{
		Policy:               policy,
		Timeout:              3 * time.Second,
		MaxResponseBodyBytes: bodyLimit,
	})
	if err != nil {
		t.Fatalf("new safe HTTP client: %v", err)
	}
	defer closeSafeHTTPClient(client)

	response, err := client.Get(targetURL + "/exact")
	if err != nil {
		t.Fatalf("exact response limit: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || len(body) != bodyLimit {
		t.Fatalf("exact response body len=%d err=%v", len(body), err)
	}

	response, err = client.Get(targetURL + "/oversized")
	if err != nil {
		t.Fatalf("receive capped response: %v", err)
	}
	var payload map[string]any
	err = json.NewDecoder(response.Body).Decode(&payload)
	_ = response.Body.Close()
	if !errors.Is(err, ErrHTTPResponseBodyTooLarge) {
		t.Fatalf("oversized response must fail before decode, got %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("oversized response leaked secret: %v", err)
	}
}

func TestSafeHTTPTransportBoundsSuccessfulSSEBody(t *testing.T) {
	tests := map[string]struct {
		writeBody func(io.Writer)
		wantErr   bool
	}{
		"oversized single line": {
			writeBody: func(writer io.Writer) {
				_, _ = io.WriteString(writer, strings.Repeat("x", maxSSELineBytes+1)+"\n")
			},
			wantErr: true,
		},
		"too many data lines in one event": {
			writeBody: func(writer io.Writer) {
				_, _ = io.WriteString(writer, "event: message\n")
				for index := 0; index <= maxSSEDataLines; index++ {
					_, _ = io.WriteString(writer, "data: x\n")
				}
			},
			wantErr: true,
		},
		"normal multiline event": {
			writeBody: func(writer io.Writer) {
				_, _ = io.WriteString(writer, "event: message\n")
				_, _ = io.WriteString(writer, `data: {"jsonrpc":"2.0",`+"\n")
				_, _ = io.WriteString(writer, `data: "id":1,"result":{}}`+"\n\n")
			},
		},
	}
	for name, test := range tests {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			streamClosed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				defer close(streamClosed)
				writer.Header().Set("Content-Type", "text/event-stream")
				test.writeBody(writer)
				writer.(http.Flusher).Flush()
				if test.wantErr {
					<-request.Context().Done()
				}
			}))
			defer server.Close()
			client := newLocalSafeHTTPClient(t, server.URL, defaultHTTPResponseBodyMaxBytes)
			defer closeSafeHTTPClient(client)
			response, err := client.Get(localhostTestURL(t, server.URL))
			if err != nil {
				t.Fatalf("receive SSE response: %v", err)
			}
			body, readErr := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if test.wantErr {
				if !errors.Is(readErr, ErrSSEEventLimitExceeded) {
					t.Fatalf("expected fixed SSE limit error, got %v", readErr)
				}
				if len(body) != 0 {
					t.Fatalf("unvalidated SSE bytes were exposed: %d", len(body))
				}
				select {
				case <-streamClosed:
				case <-time.After(time.Second):
					t.Fatal("oversized SSE response was not closed")
				}
				return
			}
			if readErr != nil {
				t.Fatalf("read normal multiline event: %v", readErr)
			}
			if !bytes.Contains(body, []byte("data: \"id\":1")) {
				t.Fatalf("normal multiline event changed: %q", body)
			}
		})
	}
}

func TestSafeHTTPTransportCapsNon2xxFakeEventStream(t *testing.T) {
	const (
		bodyLimit = 64
		secret    = "fake-event-stream-error-secret"
	)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(writer, "data: "+secret+strings.Repeat("x", bodyLimit)+"\n\n")
	}))
	defer server.Close()
	client := newLocalSafeHTTPClient(t, server.URL, bodyLimit)
	defer closeSafeHTTPClient(client)
	response, err := client.Get(localhostTestURL(t, server.URL))
	if err != nil {
		t.Fatalf("receive fake event-stream error: %v", err)
	}
	_, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !errors.Is(readErr, ErrHTTPResponseBodyTooLarge) {
		t.Fatalf("non-2xx event-stream must use ordinary body cap, got %v", readErr)
	}
	if strings.Contains(readErr.Error(), secret) {
		t.Fatalf("non-2xx body leaked secret: %v", readErr)
	}
}

type sequenceHTTPResolver struct {
	mu      sync.Mutex
	results [][]netip.Addr
	calls   int
}

func (r *sequenceHTTPResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.calls
	if index >= len(r.results) {
		index = len(r.results) - 1
	}
	r.calls++
	return append([]netip.Addr(nil), r.results[index]...), nil
}

func (r *sequenceHTTPResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type recordingHTTPDialer struct {
	mu      sync.Mutex
	dialed  []string
	dialErr error
}

func (d *recordingHTTPDialer) DialContext(_ context.Context, _, address string) (net.Conn, error) {
	d.mu.Lock()
	d.dialed = append(d.dialed, address)
	d.mu.Unlock()
	if d.dialErr != nil {
		return nil, d.dialErr
	}
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func (d *recordingHTTPDialer) addresses() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.dialed...)
}

func mustSafeHTTPPolicy(t *testing.T, options SafeHTTPPolicyOptions) *SafeHTTPPolicy {
	t.Helper()
	policy, err := NewSafeHTTPPolicy(options)
	if err != nil {
		t.Fatalf("new safe HTTP policy: %v", err)
	}
	return policy
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse URL %q: %v", rawURL, err)
	}
	return parsed
}

func localhostTestURL(t *testing.T, rawURL string) string {
	t.Helper()
	parsed := mustParseURL(t, rawURL)
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port <= 0 {
		t.Fatalf("test server port %q", parsed.Port())
	}
	parsed.Host = net.JoinHostPort("localhost", strconv.Itoa(port))
	return parsed.String()
}

func newLocalSafeHTTPClient(t *testing.T, serverURL string, bodyLimit int64) *http.Client {
	t.Helper()
	targetURL := localhostTestURL(t, serverURL)
	policy := mustSafeHTTPPolicy(t, SafeHTTPPolicyOptions{
		AllowedHosts:    []string{mustParseURL(t, targetURL).Host},
		AllowLocalDebug: true,
	})
	client, err := NewSafeHTTPClient(SafeHTTPClientOptions{
		Policy:               policy,
		Timeout:              3 * time.Second,
		MaxResponseBodyBytes: bodyLimit,
	})
	if err != nil {
		t.Fatalf("new local safe HTTP client: %v", err)
	}
	return client
}

func closeSafeHTTPClient(client *http.Client) {
	if client == nil {
		return
	}
	if transport, ok := client.Transport.(*SafeHTTPTransport); ok {
		transport.CloseIdleConnections()
	}
}
