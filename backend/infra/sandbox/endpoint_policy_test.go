// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/safehttp"
)

func TestEndpointPolicyRequiresRootHTTPOrHTTPSOriginAndExactHost(t *testing.T) {
	t.Parallel()

	config := validEndpointPolicyConfig()
	policy, err := newEndpointPolicy(config)
	if err != nil {
		t.Fatalf("new endpoint policy: %v", err)
	}
	if got := policy.urlForPath("/v1/health").String(); got != "https://sandbox.example.test/v1/health" {
		t.Fatalf("health URL = %q", got)
	}

	for _, endpoint := range []string{
		"ftp://sandbox.example.test/",
		"https://sandbox.example.test/base",
		"https://sandbox.example.test/?query=1",
		"https://sandbox.example.test/#fragment",
		"https://user:pass@sandbox.example.test/",
		"https://BÜCHER.example/",
		"https://sandbox.example.test:/",
		"https://[sandbox.example.test]/",
		"http://host.docker.internal:8080/",
		" https://sandbox.example.test/",
	} {
		invalid := config
		invalid.Endpoint = endpoint
		if _, err := newEndpointPolicy(invalid); !errors.Is(err, domainsandbox.ErrInvalidInput) {
			t.Fatalf("endpoint %q must fail closed, got %v", endpoint, err)
		}
	}

	hostMismatch := config
	hostMismatch.AllowedHosts = []string{"other.example.test"}
	if _, err := newEndpointPolicy(hostMismatch); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("host mismatch must fail closed, got %v", err)
	}

	badCIDR := config
	badCIDR.AllowedPrivateCIDRs = []string{"10.0.0.1/8"}
	if _, err := newEndpointPolicy(badCIDR); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("non-canonical CIDR must fail closed, got %v", err)
	}
}

func TestEndpointPolicyAcceptsHTTPOrHTTPSExactOrigin(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{
		"http://sandbox.example.test/",
		"http://sandbox.example.test:8080/",
		"https://sandbox.example.test/",
		"https://sandbox.example.test:8443/",
	} {
		config := validEndpointPolicyConfig()
		config.Endpoint = endpoint
		authority := strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://"), "/")
		config.AllowedHosts = []string{authority}
		policy, err := newEndpointPolicy(config)
		if err != nil {
			t.Fatalf("new endpoint policy for %q: %v", endpoint, err)
		}
		if got := policy.urlForPath("/v1/health").String(); got != endpoint+"v1/health" {
			t.Fatalf("health URL = %q, want prefix %q", got, endpoint)
		}
		if got := policy.TransportEncrypted; got != strings.HasPrefix(endpoint, "https://") {
			t.Fatalf("transport encrypted for %q = %t", endpoint, got)
		}
	}
}

func TestEndpointHTTPClientPinsOriginAndDisablesProxyAndRedirect(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")

	for _, endpoint := range []string{"http://sandbox.example.test:8080/", "https://sandbox.example.test:8443/"} {
		config := validEndpointPolicyConfig()
		config.Endpoint = endpoint
		config.AllowedHosts = []string{strings.TrimSuffix(strings.SplitN(endpoint, "://", 2)[1], "/")}
		policy, err := newEndpointPolicy(config)
		if err != nil {
			t.Fatalf("new policy for %q: %v", endpoint, err)
		}
		client, err := policy.newHTTPClientForTest(time.Second, 1024, domainsandbox.ErrProviderUnhealthy, endpointHTTPClientTestOptions{
			resolver: &endpointSequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr("93.184.216.34")}}},
			dialer:   &endpointRecordingDialer{},
		})
		if err != nil {
			t.Fatalf("new client for %q: %v", endpoint, err)
		}
		transport, ok := client.Transport.(*endpointHTTPTransport)
		if !ok {
			t.Fatalf("transport for %q = %T", endpoint, client.Transport)
		}
		if transport.base.Proxy != nil {
			t.Fatalf("transport for %q uses an environment proxy", endpoint)
		}

		redirect, requestErr := http.NewRequest(http.MethodGet, endpoint+"redirected", nil)
		if requestErr != nil {
			t.Fatalf("new redirect: %v", requestErr)
		}
		redirect.Header.Set("Authorization", "Bearer synthetic")
		via := []*http.Request{{URL: policy.urlForPath("/v1/health")}}
		if redirectErr := client.CheckRedirect(redirect, via); !errors.Is(redirectErr, domainsandbox.ErrInvalidInput) {
			t.Fatalf("same-origin redirect for %q must fail closed, got %v", endpoint, redirectErr)
		}
		if len(redirect.Header) != 0 {
			t.Fatalf("rejected redirect for %q retained headers: %#v", endpoint, redirect.Header)
		}

		mismatchPort := "9443"
		mismatchScheme := "http"
		if strings.HasPrefix(endpoint, "http://") {
			mismatchPort = "8081"
			mismatchScheme = "https"
		}
		for _, rawURL := range []string{
			mismatchScheme + "://sandbox.example.test:" + mismatchPort + "/v1/health",
			strings.SplitN(endpoint, "://", 2)[0] + "://other.example.test:" + mismatchPort + "/v1/health",
			strings.SplitN(endpoint, "://", 2)[0] + "://sandbox.example.test:" + mismatchPort + "/v1/health",
		} {
			request, requestErr := http.NewRequest(http.MethodGet, rawURL, nil)
			if requestErr != nil {
				t.Fatalf("new mismatch request: %v", requestErr)
			}
			if validateErr := policy.validateRequest(request); !errors.Is(validateErr, domainsandbox.ErrInvalidInput) {
				t.Fatalf("mismatched origin %q for %q must fail closed, got %v", rawURL, endpoint, validateErr)
			}
		}
	}
}

func TestEndpointHTTPTransportResolvesEveryDialAndRequiresAllAddressesSafe(t *testing.T) {
	t.Parallel()

	config := validEndpointPolicyConfig()
	config.Endpoint = "http://sandbox.example.test:8080/"
	config.AllowedHosts = []string{"sandbox.example.test:8080"}
	config.AllowedPrivateCIDRs = []string{"10.20.0.0/16"}
	policy, err := newEndpointPolicy(config)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	resolver := &endpointSequenceResolver{results: [][]netip.Addr{
		{netip.MustParseAddr("93.184.216.34")},
		{netip.MustParseAddr("10.20.30.40")},
		{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("127.0.0.1")},
	}}
	dialer := &endpointRecordingDialer{}
	client, err := policy.newHTTPClientForTest(time.Second, 1024, domainsandbox.ErrProviderUnhealthy, endpointHTTPClientTestOptions{
		resolver: resolver,
		dialer:   dialer,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	transport := client.Transport.(*endpointHTTPTransport)
	for index := 0; index < 2; index++ {
		connection, dialErr := transport.dialContext(context.Background(), "tcp", "sandbox.example.test:8080")
		if dialErr != nil {
			t.Fatalf("dial %d: %v", index, dialErr)
		}
		_ = connection.Close()
	}
	if _, dialErr := transport.dialContext(context.Background(), "tcp", "sandbox.example.test:8080"); !errors.Is(dialErr, safehttp.ErrUnsafeAddress) {
		t.Fatalf("mixed safe/unsafe resolution must fail closed, got %v", dialErr)
	}
	if got := resolver.callCount(); got != 3 {
		t.Fatalf("resolver calls = %d, want 3", got)
	}
	if got := dialer.addresses(); len(got) != 2 || got[0] != "93.184.216.34:8080" || got[1] != "10.20.30.40:8080" {
		t.Fatalf("dialed addresses = %#v", got)
	}
	if _, dialErr := transport.dialContext(context.Background(), "unix", "/var/run/docker.sock"); !errors.Is(dialErr, safehttp.ErrUnsafeAddress) {
		t.Fatalf("Unix socket must fail closed, got %v", dialErr)
	}
	if _, dialErr := transport.dialContext(context.Background(), "tcp", "other.example.test:8080"); !errors.Is(dialErr, safehttp.ErrUnsafeAddress) {
		t.Fatalf("non-allowlist host must fail closed, got %v", dialErr)
	}
	if _, dialErr := transport.dialContext(context.Background(), "tcp", "sandbox.example.test:8081"); !errors.Is(dialErr, safehttp.ErrUnsafeAddress) {
		t.Fatalf("non-allowlist port must fail closed, got %v", dialErr)
	}
}

func TestEndpointHTTPTransportRejectsSpecialMetadataGatewayAndNonAllowlistAddresses(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"127.0.0.1",
		"169.254.1.1",
		"224.0.0.1",
		"169.254.169.254",
		"100.100.100.200",
		"172.17.0.1",
		"10.21.0.1",
		"::1",
		"fe80::1",
		"ff02::1",
	} {
		address := address
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			config := validEndpointPolicyConfig()
			config.Endpoint = "https://sandbox.example.test:8443/"
			config.AllowedHosts = []string{"sandbox.example.test:8443"}
			config.AllowedPrivateCIDRs = []string{"10.20.0.0/16", "172.17.0.0/16"}
			policy, err := newEndpointPolicy(config)
			if err != nil {
				t.Fatalf("new policy: %v", err)
			}
			dialer := &endpointRecordingDialer{}
			client, err := policy.newHTTPClientForTest(time.Second, 1024, domainsandbox.ErrProviderUnhealthy, endpointHTTPClientTestOptions{
				resolver: &endpointSequenceResolver{results: [][]netip.Addr{{netip.MustParseAddr(address)}}},
				dialer:   dialer,
			})
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			transport := client.Transport.(*endpointHTTPTransport)
			if _, dialErr := transport.dialContext(context.Background(), "tcp", "sandbox.example.test:8443"); !errors.Is(dialErr, safehttp.ErrUnsafeAddress) {
				t.Fatalf("address %s must fail closed, got %v", address, dialErr)
			}
			if got := dialer.addresses(); len(got) != 0 {
				t.Fatalf("unsafe address %s reached final dial: %#v", address, got)
			}
		})
	}
}

func TestEndpointPolicyRejectsLiteralMetadataAddressAtConstruction(t *testing.T) {
	t.Parallel()

	config := validEndpointPolicyConfig()
	config.Endpoint = "https://100.100.100.200:8443/"
	config.AllowedHosts = []string{"100.100.100.200:8443"}
	config.AllowedPrivateCIDRs = []string{"0.0.0.0/0"}
	if _, err := newEndpointPolicy(config); !errors.Is(err, domainsandbox.ErrInvalidInput) {
		t.Fatalf("literal metadata endpoint must fail closed, got %v", err)
	}
}

func TestEndpointHTTPTransportBoundsResponseBody(t *testing.T) {
	t.Parallel()

	transport := &endpointHTTPTransport{maxResponseBodyBytes: 4, unavailableError: domainsandbox.ErrProviderUnhealthy}
	response := &http.Response{
		Body:          io.NopCloser(strings.NewReader("12345")),
		ContentLength: -1,
	}
	transport.bufferResponse(response)
	_, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if !errors.Is(readErr, safehttp.ErrResponseBodyTooLarge) {
		t.Fatalf("oversized response body error = %v", readErr)
	}
}

type endpointSequenceResolver struct {
	mu      sync.Mutex
	results [][]netip.Addr
	calls   int
}

func (r *endpointSequenceResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.calls
	if index >= len(r.results) {
		index = len(r.results) - 1
	}
	r.calls++
	return append([]netip.Addr(nil), r.results[index]...), nil
}

func (r *endpointSequenceResolver) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type endpointRecordingDialer struct {
	mu     sync.Mutex
	dialed []string
}

func validEndpointPolicyConfig() RemoteProviderConfig {
	return RemoteProviderConfig{
		Endpoint:            "https://sandbox.example.test/",
		Credential:          "synthetic-test-token",
		AllowedHosts:        []string{"sandbox.example.test"},
		AllowedPrivateCIDRs: []string{"10.0.0.0/8"},
		Timeout:             2 * time.Second,
	}
}

func (d *endpointRecordingDialer) DialContext(_ context.Context, _ string, address string) (net.Conn, error) {
	d.mu.Lock()
	d.dialed = append(d.dialed, address)
	d.mu.Unlock()
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func (d *endpointRecordingDialer) addresses() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.dialed...)
}
