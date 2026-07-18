// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package safehttp

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPolicyRequiresExactASCIIHTTPSAuthority(t *testing.T) {
	t.Parallel()

	policy, err := NewPolicy(PolicyOptions{AllowedHosts: []string{
		"SANDBOX.Example.TEST.",
		"xn--bcher-kva.example:8443",
	}})
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	for _, rawURL := range []string{
		"https://sandbox.example.test/v1/health",
		"https://SANDBOX.EXAMPLE.TEST.:443/v1/health",
		"https://xn--bcher-kva.example:8443/v1/health",
	} {
		if err := policy.ValidateURL(mustURL(t, rawURL)); err != nil {
			t.Fatalf("URL %q should pass: %v", rawURL, err)
		}
	}
	for _, rawURL := range []string{
		"http://sandbox.example.test/v1/health",
		"https://sandbox.example.test:8443/v1/health",
		"https://sandbox.example.test.evil/v1/health",
		"https://user:pass@sandbox.example.test/v1/health",
		"https://sandbox.example.test/v1/health#fragment",
		"https://BÜCHER.example:8443/v1/health",
		"https://sandbox.example.test:/v1/health",
	} {
		if err := policy.ValidateURL(mustURL(t, rawURL)); !errors.Is(err, ErrURLRejected) {
			t.Fatalf("URL %q must fail closed, got %v", rawURL, err)
		}
	}
	bracketedDNS := &url.URL{Scheme: "https", Host: "[sandbox.example.test]", Path: "/v1/health"}
	if err := policy.ValidateURL(bracketedDNS); !errors.Is(err, ErrURLRejected) {
		t.Fatalf("bracketed DNS name must fail closed, got %v", err)
	}
	for _, authority := range []string{
		"*.example.test",
		"BÜCHER.example",
		"sandbox.example.test/path",
		"sandbox.example.test:0",
		"sandbox.example.test:65536",
		"sandbox.example.test:",
		"[sandbox.example.test]",
	} {
		if _, err := NewPolicy(PolicyOptions{AllowedHosts: []string{authority}}); !errors.Is(err, ErrURLRejected) {
			t.Fatalf("authority %q must fail closed, got %v", authority, err)
		}
	}
}

func TestProductionPolicyHardBlocksLoopbackAndExposesNoDebugOption(t *testing.T) {
	t.Parallel()

	if _, exists := reflect.TypeOf(PolicyOptions{}).FieldByName("AllowLocalDebug"); exists {
		t.Fatal("production PolicyOptions must not expose a local-debug bypass")
	}
	policy := mustPolicy(t, PolicyOptions{
		AllowedHosts:        []string{"127.0.0.1", "localhost"},
		AllowedPrivateCIDRs: []string{"127.0.0.0/8", "0.0.0.0/0"},
	})
	if err := policy.ValidateURL(mustURL(t, "https://127.0.0.1/v1/health")); !errors.Is(err, ErrURLRejected) {
		t.Fatalf("literal loopback must be rejected before dialing, got %v", err)
	}
	transport := mustTestTransport(t, ClientOptions{Policy: policy}, transportTestOptions{
		resolver: &sequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}},
		dialer:   &recordingDialer{},
	})
	if _, err := transport.dialContext(context.Background(), "tcp", "localhost:443"); !errors.Is(err, ErrUnsafeAddress) {
		t.Fatalf("resolved loopback must remain hard-blocked, got %v", err)
	}
}

func TestDebugPolicyHTTPRequiresExplicitLoopbackHostAndResolution(t *testing.T) {
	debugPolicy, err := NewDebugAuthorizedPolicy(DebugPolicyOptions{
		AllowedHosts: []string{
			"localhost:8080",
			"127.0.0.1:8080",
			"[::1]:8080",
			"public.example.test:8080",
			"private.example.test:8080",
			"169.254.169.254:8080",
		},
		AllowedPrivateCIDRs: []string{"10.0.0.0/8"},
	})
	if err != nil {
		t.Fatalf("new debug policy: %v", err)
	}
	for _, rawURL := range []string{
		"http://localhost:8080/health",
		"http://127.0.0.1:8080/health",
		"http://[::1]:8080/health",
	} {
		if err := debugPolicy.ValidateURL(mustURL(t, rawURL)); err != nil {
			t.Fatalf("explicit loopback URL %q: %v", rawURL, err)
		}
	}
	for _, rawURL := range []string{
		"http://public.example.test:8080/health",
		"http://private.example.test:8080/health",
		"http://169.254.169.254:8080/health",
	} {
		if err := debugPolicy.ValidateURL(mustURL(t, rawURL)); !errors.Is(err, ErrURLRejected) {
			t.Fatalf("non-loopback HTTP URL %q must fail, got %v", rawURL, err)
		}
	}

	for _, resolvedAddress := range []string{"93.184.216.34", "10.20.30.40", "169.254.169.254"} {
		dialer := &recordingDialer{}
		transport := mustTestTransport(t, ClientOptions{Policy: debugPolicy, Timeout: time.Second}, transportTestOptions{
			resolver: &sequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr(resolvedAddress)}}},
			dialer:   dialer,
		})
		request, requestErr := http.NewRequest(http.MethodGet, "http://localhost:8080/health", nil)
		if requestErr != nil {
			t.Fatalf("new abnormal-resolution request: %v", requestErr)
		}
		_, _ = transport.RoundTrip(request)
		if got := dialer.addresses(); len(got) != 0 {
			t.Fatalf("localhost resolution %s reached final dial: %#v", resolvedAddress, got)
		}
	}

	loopbackDialer := &recordingDialer{}
	loopbackTransport := mustTestTransport(t, ClientOptions{Policy: debugPolicy, Timeout: time.Second}, transportTestOptions{
		resolver: &sequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}},
		dialer:   loopbackDialer,
	})
	loopbackRequest, err := http.NewRequest(http.MethodGet, "http://localhost:8080/health", nil)
	if err != nil {
		t.Fatalf("new loopback request: %v", err)
	}
	_, _ = loopbackTransport.RoundTrip(loopbackRequest)
	if got := loopbackDialer.addresses(); len(got) != 1 || got[0] != "127.0.0.1:8080" {
		t.Fatalf("validated loopback dial = %#v", got)
	}

	httpsDialer := &recordingDialer{}
	httpsTransport := mustTestTransport(t, ClientOptions{Policy: debugPolicy, Timeout: time.Second}, transportTestOptions{
		resolver: &sequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}}},
		dialer:   httpsDialer,
	})
	httpsRequest, err := http.NewRequest(http.MethodGet, "https://localhost:8080/health", nil)
	if err != nil {
		t.Fatalf("new HTTPS request: %v", err)
	}
	_, _ = httpsTransport.RoundTrip(httpsRequest)
	if got := httpsDialer.addresses(); len(got) != 0 {
		t.Fatalf("HTTPS debug policy bypassed production loopback block: %#v", got)
	}
}

func TestPolicyRejectsNonCanonicalIPv4LookingHosts(t *testing.T) {
	t.Parallel()

	for _, authority := range []string{"127.1", "2130706433", "0177.0.0.1", "0x7f000001"} {
		if _, err := NewPolicy(PolicyOptions{AllowedHosts: []string{authority}}); !errors.Is(err, ErrURLRejected) {
			t.Fatalf("IPv4-looking authority %q must fail closed, got %v", authority, err)
		}
	}
	policy := mustPolicy(t, PolicyOptions{AllowedHosts: []string{"1.example.com"}})
	if err := policy.ValidateURL(mustURL(t, "https://1.example.com/health")); err != nil {
		t.Fatalf("numeric DNS label was rejected: %v", err)
	}
	client, err := NewClient(ClientOptions{Policy: policy})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	redirect := &http.Request{URL: mustURL(t, "https://2130706433/health"), Header: make(http.Header)}
	via := []*http.Request{{URL: mustURL(t, "https://1.example.com/health")}}
	if err := client.CheckRedirect(redirect, via); !errors.Is(err, ErrURLRejected) {
		t.Fatalf("noncanonical redirect host must fail closed, got %v", err)
	}
}

func TestClientOptionsExposeNoTransportBypass(t *testing.T) {
	t.Parallel()

	optionsType := reflect.TypeOf(ClientOptions{})
	for _, field := range []string{"Resolver", "Dialer", "BaseRoundTripper", "Client", "Transport"} {
		if _, exists := optionsType.FieldByName(field); exists {
			t.Fatalf("production ClientOptions exposes %s bypass", field)
		}
	}
	transport, err := NewTransport(ClientOptions{Policy: mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test"}})})
	if err != nil {
		t.Fatalf("new production transport: %v", err)
	}
	if transport.base == nil || transport.delegate != transport.base || transport.base.Proxy != nil || !transport.base.DisableCompression {
		t.Fatal("production constructor did not build the fixed safe transport")
	}
}

func TestTransportResolvesEveryDialAndEnforcesPrivateCIDRs(t *testing.T) {
	t.Parallel()

	resolver := &sequenceResolver{results: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")},
		{netip.MustParseAddr("10.20.30.40")},
	}}
	dialer := &recordingDialer{}
	policy := mustPolicy(t, PolicyOptions{
		AllowedHosts:        []string{"sandbox.example.test"},
		AllowedPrivateCIDRs: []string{"10.20.0.0/16"},
	})
	transport := mustTestTransport(t, ClientOptions{Policy: policy}, transportTestOptions{resolver: resolver, dialer: dialer})
	for index := 0; index < 2; index++ {
		connection, err := transport.dialContext(context.Background(), "tcp", "sandbox.example.test:443")
		if err != nil {
			t.Fatalf("dial %d: %v", index, err)
		}
		_ = connection.Close()
	}
	if resolver.callCount() != 2 {
		t.Fatalf("resolver calls = %d, want 2", resolver.callCount())
	}
	if got := dialer.addresses(); len(got) != 2 || got[0] != "93.184.216.34:443" || got[1] != "10.20.30.40:443" {
		t.Fatalf("dialed addresses = %#v", got)
	}

	privateWithoutCIDR := mustTestTransport(t,
		ClientOptions{Policy: mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test"}})},
		transportTestOptions{
			resolver: &sequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr("10.20.30.40")}}},
			dialer:   &recordingDialer{},
		},
	)
	if _, err := privateWithoutCIDR.dialContext(context.Background(), "tcp", "sandbox.example.test:443"); !errors.Is(err, ErrUnsafeAddress) {
		t.Fatalf("private address without CIDR must fail, got %v", err)
	}
}

func TestTransportHardBlocksSpecialAndTransitionAddressesEvenWithCIDRs(t *testing.T) {
	t.Parallel()

	policy := mustPolicy(t, PolicyOptions{
		AllowedHosts:        []string{"sandbox.example.test"},
		AllowedPrivateCIDRs: []string{"0.0.0.0/0", "::/0"},
	})
	for _, rawAddress := range []string{
		"0.0.0.0", "127.0.0.1", "169.254.169.254", "100.100.100.200",
		"192.0.2.1", "198.18.0.1", "224.0.0.1", "240.0.0.1",
		"::", "::1", "fe80::1", "ff02::1", "fd00:ec2::254",
		"::ffff:127.0.0.1", "64:ff9b::7f00:1", "64:ff9b:1::7f00:1",
		"2002:7f00:1::1", "2001::ffff:ffff:ffff:fffe",
	} {
		rawAddress := rawAddress
		t.Run(rawAddress, func(t *testing.T) {
			transport := mustTestTransport(t, ClientOptions{Policy: policy}, transportTestOptions{
				resolver: &sequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr(rawAddress)}}},
				dialer:   &recordingDialer{},
			})
			if _, err := transport.dialContext(context.Background(), "tcp", "sandbox.example.test:443"); !errors.Is(err, ErrUnsafeAddress) {
				t.Fatalf("address %s must fail closed, got %v", rawAddress, err)
			}
		})
	}
}

func TestTransportDisablesProxyCompressionAndPreservesTLSSNI(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")

	sni := make(chan string, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	server.TLS.GetConfigForClient = func(info *tls.ClientHelloInfo) (*tls.Config, error) {
		select {
		case sni <- info.ServerName:
		default:
		}
		return nil, nil
	}
	listenerAddress := server.Listener.Addr().String()

	transport := mustTestTransport(t,
		ClientOptions{Policy: mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test"}})},
		transportTestOptions{
			resolver: &sequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr("93.184.216.34")}}},
			dialer: dialerFunc(func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, listenerAddress)
			}),
		},
	)
	if transport.base.Proxy != nil {
		t.Fatal("safe transport must not use an environment proxy")
	}
	if !transport.base.DisableCompression {
		t.Fatal("safe transport must disable automatic compression")
	}
	transport.base.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // test server certificate
	client := &http.Client{Transport: transport, Timeout: time.Second}
	response, err := client.Get("https://sandbox.example.test/v1/health")
	if err != nil {
		t.Fatalf("TLS request: %v", err)
	}
	_ = response.Body.Close()
	select {
	case got := <-sni:
		if got != "sandbox.example.test" {
			t.Fatalf("TLS SNI = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("TLS server did not observe SNI")
	}
}

func TestClientRejectsCrossOriginRedirectBeforeForwardingHeaders(t *testing.T) {
	t.Parallel()

	policy := mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test", "other.example.test"}})
	client, err := NewClient(ClientOptions{Policy: policy})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	request := &http.Request{
		URL: mustURL(t, "https://other.example.test/v1/health"),
		Header: http.Header{
			"Authorization": []string{"Bearer synthetic-token"},
			"Cookie":        []string{"session=synthetic"},
		},
	}
	via := []*http.Request{{URL: mustURL(t, "https://sandbox.example.test/v1/health")}}
	if err := client.CheckRedirect(request, via); !errors.Is(err, ErrURLRejected) {
		t.Fatalf("cross-origin redirect must fail, got %v", err)
	}
	if len(request.Header) != 0 {
		t.Fatalf("rejected redirect retained headers: %#v", request.Header)
	}

	sameOrigin := &http.Request{URL: mustURL(t, "https://sandbox.example.test/v1/executions"), Header: http.Header{"Authorization": []string{"Bearer synthetic-token"}}}
	if err := client.CheckRedirect(sameOrigin, via); err != nil {
		t.Fatalf("same-origin redirect: %v", err)
	}
	tooMany := []*http.Request{via[0], via[0], via[0], via[0]}
	if err := client.CheckRedirect(sameOrigin, tooMany); !errors.Is(err, ErrURLRejected) {
		t.Fatalf("redirect limit must fail, got %v", err)
	}
}

func TestTransportBoundsResponseFromInjectedRoundTripper(t *testing.T) {
	t.Parallel()

	client, err := newClientForTest(ClientOptions{
		Policy:               mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test"}}),
		MaxResponseBodyBytes: 4,
	}, transportTestOptions{
		baseRoundTripper: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("12345")),
				Request:    request,
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	response, err := client.Get("https://sandbox.example.test/v1/health")
	if err != nil {
		t.Fatalf("get capped response: %v", err)
	}
	_, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !errors.Is(readErr, ErrTransportUnavailable) {
		t.Fatalf("untrusted custom response body error = %v", readErr)
	}
}

type contextBoundStaticBody struct {
	reader *strings.Reader
	closed bool
}

func (*contextBoundStaticBody) safeHTTPContextBoundBody() {}

func (body *contextBoundStaticBody) Read(buffer []byte) (int, error) {
	return body.reader.Read(buffer)
}

func (body *contextBoundStaticBody) Close() error {
	body.closed = true
	return nil
}

func TestTransportBoundsTrustedContextBoundInjectedRoundTripper(t *testing.T) {
	body := &contextBoundStaticBody{reader: strings.NewReader("12345")}
	client, err := newClientForTest(ClientOptions{
		Policy:               mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test"}}),
		MaxResponseBodyBytes: 4,
	}, transportTestOptions{
		baseRoundTripper: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       body,
				Request:    request,
			}, nil
		}),
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	response, err := client.Get("https://sandbox.example.test/v1/health")
	if err != nil {
		t.Fatalf("get capped response: %v", err)
	}
	_, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !errors.Is(readErr, ErrResponseBodyTooLarge) {
		t.Fatalf("trusted oversized body error = %v", readErr)
	}
	if !body.closed {
		t.Fatal("trusted oversized response body was not closed")
	}
}

func TestTransportDirectUseEnforcesTotalBodyTimeout(t *testing.T) {
	t.Parallel()

	closed := make(chan struct{})
	transport := mustTestTransport(t, ClientOptions{
		Policy:  mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test"}}),
		Timeout: time.Nanosecond,
	}, transportTestOptions{
		baseRoundTripper: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       &contextBlockingBody{ctx: request.Context(), closed: closed},
				Request:    request,
			}, nil
		}),
	})
	request, err := http.NewRequest(http.MethodGet, "https://sandbox.example.test/v1/health", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	_, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !errors.Is(readErr, ErrTransportUnavailable) {
		t.Fatalf("timed-out body error = %v", readErr)
	}
	select {
	case <-closed:
	default:
		t.Fatal("timed-out response body was not closed")
	}
}

type sequenceResolver struct {
	mu      sync.Mutex
	results [][]netip.Addr
	calls   int
}

func (r *sequenceResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.calls
	if index >= len(r.results) {
		index = len(r.results) - 1
	}
	r.calls++
	return append([]netip.Addr(nil), r.results[index]...), nil
}

func (r *sequenceResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type recordingDialer struct {
	mu     sync.Mutex
	dialed []string
}

type dialerFunc func(context.Context, string, string) (net.Conn, error)

func (f dialerFunc) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return f(ctx, network, address)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type idleClosingRoundTripper struct {
	closeCalls int
}

func (*idleClosingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected request")
}

func (r *idleClosingRoundTripper) CloseIdleConnections() {
	r.closeCalls++
}

func TestTransportCloseIdleConnectionsDelegatesForWrappedTransports(t *testing.T) {
	delegate := &idleClosingRoundTripper{}
	transport := mustTestTransport(t, ClientOptions{
		Policy: mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test"}}),
	}, transportTestOptions{baseRoundTripper: delegate})
	transport.CloseIdleConnections()
	if delegate.closeCalls != 1 {
		t.Fatalf("close idle calls = %d", delegate.closeCalls)
	}
}

type contextBlockingBody struct {
	ctx    context.Context
	closed chan struct{}
	once   sync.Once
}

func (*contextBlockingBody) safeHTTPContextBoundBody() {}

func (b *contextBlockingBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *contextBlockingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}

type uninterruptibleBody struct {
	readEntered  chan struct{}
	closeEntered chan struct{}
	release      chan struct{}
	readOnce     sync.Once
	closeOnce    sync.Once
}

func (body *uninterruptibleBody) Read([]byte) (int, error) {
	body.readOnce.Do(func() { close(body.readEntered) })
	<-body.release
	return 0, io.EOF
}

func (body *uninterruptibleBody) Close() error {
	body.closeOnce.Do(func() { close(body.closeEntered) })
	<-body.release
	return nil
}

func TestTransportRejectsUninterruptibleInjectedBodyWithoutReadOrCloseGoroutine(t *testing.T) {
	body := &uninterruptibleBody{
		readEntered: make(chan struct{}), closeEntered: make(chan struct{}), release: make(chan struct{}),
	}
	transport := mustTestTransport(t, ClientOptions{
		Policy:  mustPolicy(t, PolicyOptions{AllowedHosts: []string{"sandbox.example.test"}}),
		Timeout: 10 * time.Millisecond,
	}, transportTestOptions{
		baseRoundTripper: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: request,
			}, nil
		}),
	})
	request, err := http.NewRequest(http.MethodGet, "https://sandbox.example.test/v1/health", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if response == nil {
		t.Fatal("round trip returned no response")
	}
	_, readErr := io.ReadAll(response.Body)
	if !errors.Is(readErr, ErrTransportUnavailable) {
		t.Fatalf("body error = %v, want unavailable", readErr)
	}
	select {
	case <-body.readEntered:
		t.Fatal("uninterruptible custom body was read")
	default:
	}
	select {
	case <-body.closeEntered:
		t.Fatal("uninterruptible custom body was closed on a goroutine")
	default:
	}
	close(body.release)
}

func (d *recordingDialer) DialContext(_ context.Context, _ string, stringAddress string) (net.Conn, error) {
	d.mu.Lock()
	d.dialed = append(d.dialed, stringAddress)
	d.mu.Unlock()
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func (d *recordingDialer) addresses() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.dialed...)
}

func mustPolicy(t *testing.T, options PolicyOptions) *Policy {
	t.Helper()
	policy, err := NewPolicy(options)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	return policy
}

func mustTransport(t *testing.T, options ClientOptions) *Transport {
	t.Helper()
	transport, err := NewTransport(options)
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	return transport
}

func mustTestTransport(t *testing.T, options ClientOptions, testOptions transportTestOptions) *Transport {
	t.Helper()
	transport, err := newTransportForTest(options, testOptions)
	if err != nil {
		t.Fatalf("new test transport: %v", err)
	}
	return transport
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return parsed
}
