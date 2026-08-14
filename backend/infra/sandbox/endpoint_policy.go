// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/safehttp"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

type RemoteProviderConfig struct {
	Endpoint            string
	Credential          string
	AllowedHosts        []string
	AllowedPrivateCIDRs []string
	Timeout             time.Duration
	IdentitySigner      sandboxidentity.Signer
}

type endpointPolicy struct {
	endpoint            *url.URL
	httpPolicy          *safehttp.Policy
	allowedPrivateCIDRs []netip.Prefix
	TransportEncrypted  bool
}

func newEndpointPolicy(config RemoteProviderConfig) (*endpointPolicy, error) {
	if config.Endpoint == "" || strings.TrimSpace(config.Endpoint) != config.Endpoint {
		return nil, domainsandbox.ErrInvalidInput
	}
	parsed, err := url.Parse(config.Endpoint)
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawFragment != "" || parsed.RawPath != "" ||
		(parsed.Path != "" && parsed.Path != "/") || strings.HasSuffix(parsed.Host, ":") ||
		isHostGatewayName(parsed.Hostname()) {
		return nil, domainsandbox.ErrInvalidInput
	}
	policy, err := safehttp.NewPolicy(safehttp.PolicyOptions{
		AllowedHosts:        append([]string(nil), config.AllowedHosts...),
		AllowedPrivateCIDRs: append([]string(nil), config.AllowedPrivateCIDRs...),
	})
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	authority, err := safehttp.NormalizeAuthority(parsed)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	privateCIDRs := make([]netip.Prefix, 0, len(config.AllowedPrivateCIDRs))
	for _, value := range config.AllowedPrivateCIDRs {
		prefix, parseErr := netip.ParsePrefix(value)
		if parseErr != nil {
			return nil, domainsandbox.ErrInvalidInput
		}
		privateCIDRs = append(privateCIDRs, prefix)
	}
	if !endpointAuthorityAllowed(parsed, config.AllowedHosts) {
		return nil, domainsandbox.ErrInvalidInput
	}
	if literal, parseErr := netip.ParseAddr(parsed.Hostname()); parseErr == nil &&
		(!safehttp.IsAddressAllowed(literal, privateCIDRs) || isKnownHostGatewayAddress(literal)) {
		return nil, domainsandbox.ErrInvalidInput
	}
	return &endpointPolicy{
		endpoint:            &url.URL{Scheme: parsed.Scheme, Host: authority, Path: "/"},
		httpPolicy:          policy,
		allowedPrivateCIDRs: privateCIDRs,
		TransportEncrypted:  parsed.Scheme == "https",
	}, nil
}

func (p *endpointPolicy) urlForPath(value string) *url.URL {
	if p == nil || p.endpoint == nil {
		return nil
	}
	result := *p.endpoint
	result.Path = value
	return &result
}

func (p *endpointPolicy) validateRequest(request *http.Request) error {
	if p == nil || p.endpoint == nil || p.httpPolicy == nil || request == nil || request.URL == nil ||
		request.URL.Opaque != "" || request.URL.User != nil || request.URL.Fragment != "" || request.URL.RawFragment != "" {
		return domainsandbox.ErrInvalidInput
	}
	sameOrigin, err := safehttp.SameOrigin(p.endpoint, request.URL)
	if err != nil || !sameOrigin {
		return domainsandbox.ErrInvalidInput
	}
	requestAuthority, err := endpointEffectiveAuthority(request.URL)
	if err != nil {
		return domainsandbox.ErrInvalidInput
	}
	expectedAuthority, err := endpointEffectiveAuthority(p.endpoint)
	if err != nil || requestAuthority != expectedAuthority {
		return domainsandbox.ErrInvalidInput
	}
	if request.Host != "" {
		override := &url.URL{Scheme: p.endpoint.Scheme, Host: request.Host}
		overrideAuthority, normalizeErr := endpointEffectiveAuthority(override)
		if normalizeErr != nil || overrideAuthority != expectedAuthority {
			return domainsandbox.ErrInvalidInput
		}
	}
	return nil
}

type endpointResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type endpointDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type endpointHTTPClientTestOptions struct {
	resolver endpointResolver
	dialer   endpointDialer
}

func (p *endpointPolicy) newHTTPClient(timeout time.Duration, maxResponseBodyBytes int64, unavailableError error) (*http.Client, error) {
	return p.newHTTPClientWithOptions(timeout, maxResponseBodyBytes, unavailableError, endpointHTTPClientTestOptions{})
}

func (p *endpointPolicy) newHTTPClientForTest(timeout time.Duration, maxResponseBodyBytes int64, unavailableError error, options endpointHTTPClientTestOptions) (*http.Client, error) {
	return p.newHTTPClientWithOptions(timeout, maxResponseBodyBytes, unavailableError, options)
}

func (p *endpointPolicy) newHTTPClientWithOptions(timeout time.Duration, maxResponseBodyBytes int64, unavailableError error, options endpointHTTPClientTestOptions) (*http.Client, error) {
	if p == nil || p.endpoint == nil || p.httpPolicy == nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return newExactOriginHTTPClient(p, timeout, maxResponseBodyBytes, unavailableError, options)
}

func rejectEndpointRedirect(request *http.Request, _ []*http.Request) error {
	safehttp.ClearRedirectHeaders(request)
	return domainsandbox.ErrInvalidInput
}

type endpointHTTPTransport struct {
	base                 *http.Transport
	policy               *endpointPolicy
	resolver             endpointResolver
	dialer               endpointDialer
	timeout              time.Duration
	maxResponseBodyBytes int64
	unavailableError     error
}

func newExactOriginHTTPClient(policy *endpointPolicy, timeout time.Duration, maxResponseBodyBytes int64, unavailableError error, options endpointHTTPClientTestOptions) (*http.Client, error) {
	if policy == nil || policy.endpoint == nil || (policy.endpoint.Scheme != "http" && policy.endpoint.Scheme != "https") {
		return nil, domainsandbox.ErrInvalidInput
	}
	if timeout <= 0 {
		timeout = safehttp.DefaultTimeout
	}
	if timeout > safehttp.MaximumTimeout {
		return nil, domainsandbox.ErrInvalidInput
	}
	if maxResponseBodyBytes <= 0 {
		maxResponseBodyBytes = safehttp.DefaultResponseBodyMaxBytes
	}
	if maxResponseBodyBytes > safehttp.MaximumResponseBodyMaxBytes {
		return nil, domainsandbox.ErrInvalidInput
	}
	if unavailableError == nil {
		unavailableError = safehttp.ErrTransportUnavailable
	}
	resolver := options.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := options.dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	}
	transport := &endpointHTTPTransport{
		policy: policy, resolver: resolver, dialer: dialer, timeout: timeout,
		maxResponseBodyBytes: maxResponseBodyBytes, unavailableError: unavailableError,
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.DisableCompression = true
	base.DialContext = transport.dialContext
	base.DialTLSContext = nil
	base.ResponseHeaderTimeout = timeout
	base.TLSHandshakeTimeout = minEndpointDuration(timeout, 10*time.Second)
	base.ExpectContinueTimeout = time.Second
	base.MaxResponseHeaderBytes = 1 << 20
	transport.base = base
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: rejectEndpointRedirect}, nil
}

func (t *endpointHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil || t.base == nil || t.policy == nil || request == nil || request.URL == nil ||
		t.policy.validateRequest(request) != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	requestContext, cancel := context.WithTimeout(request.Context(), t.timeout)
	response, err := t.base.RoundTrip(request.Clone(requestContext))
	if err != nil {
		cancel()
		if requestContext.Err() != nil {
			return nil, requestContext.Err()
		}
		return nil, t.unavailableError
	}
	if response == nil || response.Body == nil {
		cancel()
		return nil, t.unavailableError
	}
	t.bufferResponse(response)
	cancel()
	return response, nil
}

func (t *endpointHTTPTransport) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if t == nil || t.policy == nil || t.resolver == nil || t.dialer == nil ||
		(network != "tcp" && network != "tcp4" && network != "tcp6") {
		return nil, safehttp.ErrUnsafeAddress
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, safehttp.ErrUnsafeAddress
	}
	host, err = safehttp.CanonicalHostname(host)
	if err != nil || isHostGatewayName(host) {
		return nil, safehttp.ErrUnsafeAddress
	}
	port, err = safehttp.CanonicalPort(port)
	if err != nil {
		return nil, safehttp.ErrUnsafeAddress
	}
	dialAuthority := net.JoinHostPort(host, port)
	expectedAuthority, expectedErr := endpointEffectiveAuthority(t.policy.endpoint)
	if expectedErr != nil || dialAuthority != expectedAuthority {
		return nil, safehttp.ErrUnsafeAddress
	}
	addresses := make([]netip.Addr, 0, 2)
	if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = append(addresses, literal)
	} else {
		resolved, resolveErr := t.resolver.LookupNetIP(ctx, "ip", host)
		if resolveErr != nil || len(resolved) == 0 {
			return nil, safehttp.ErrUnsafeAddress
		}
		addresses = append(addresses, resolved...)
	}
	for _, resolved := range addresses {
		if !safehttp.IsAddressAllowed(resolved, t.policy.allowedPrivateCIDRs) || isKnownHostGatewayAddress(resolved) {
			return nil, safehttp.ErrUnsafeAddress
		}
	}
	for _, resolved := range addresses {
		if network == "tcp4" && !resolved.Is4() || network == "tcp6" && !resolved.Is6() {
			continue
		}
		connection, dialErr := t.dialer.DialContext(ctx, network, net.JoinHostPort(resolved.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, t.unavailableError
}

func (t *endpointHTTPTransport) bufferResponse(response *http.Response) {
	original := response.Body
	if response.ContentLength > t.maxResponseBodyBytes {
		_ = original.Close()
		response.Body = &endpointFixedErrorBody{err: safehttp.ErrResponseBodyTooLarge}
		response.ContentLength = -1
		return
	}
	data, readErr := io.ReadAll(io.LimitReader(original, t.maxResponseBodyBytes+1))
	closeErr := original.Close()
	if int64(len(data)) > t.maxResponseBodyBytes {
		response.Body = &endpointFixedErrorBody{err: safehttp.ErrResponseBodyTooLarge}
		response.ContentLength = -1
		return
	}
	if readErr != nil || closeErr != nil {
		response.Body = &endpointFixedErrorBody{err: t.unavailableError}
		response.ContentLength = -1
		return
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
}

func (t *endpointHTTPTransport) CloseIdleConnections() {
	if t != nil && t.base != nil {
		t.base.CloseIdleConnections()
	}
}

type endpointFixedErrorBody struct{ err error }

func (b *endpointFixedErrorBody) Read([]byte) (int, error) { return 0, b.err }
func (*endpointFixedErrorBody) Close() error               { return nil }

func isHostGatewayName(host string) bool {
	host, err := safehttp.CanonicalHostname(host)
	if err != nil {
		return true
	}
	switch host {
	case "host.docker.internal", "gateway.docker.internal", "host.containers.internal",
		"docker.for.mac.host.internal", "docker.for.win.localhost", "host-gateway":
		return true
	default:
		return false
	}
}

func isKnownHostGatewayAddress(address netip.Addr) bool {
	switch address {
	case netip.MustParseAddr("10.0.2.2"),
		netip.MustParseAddr("172.17.0.1"),
		netip.MustParseAddr("192.168.65.1"),
		netip.MustParseAddr("192.168.127.254"):
		return true
	default:
		return false
	}
}

func minEndpointDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func endpointEffectiveAuthority(value *url.URL) (string, error) {
	if value == nil {
		return "", safehttp.ErrURLRejected
	}
	host, err := safehttp.CanonicalHostname(value.Hostname())
	if err != nil {
		return "", err
	}
	port, err := safehttp.EffectivePort(value)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, port), nil
}

func endpointAuthorityAllowed(endpoint *url.URL, allowedHosts []string) bool {
	expected, err := endpointEffectiveAuthority(endpoint)
	if err != nil {
		return false
	}
	for _, allowed := range allowedHosts {
		if address, parseErr := netip.ParseAddr(allowed); parseErr == nil {
			if net.JoinHostPort(address.String(), safehttp.DefaultPort(endpoint.Scheme)) == expected {
				return true
			}
			continue
		}
		candidate := &url.URL{Scheme: endpoint.Scheme, Host: allowed}
		if authority, normalizeErr := endpointEffectiveAuthority(candidate); normalizeErr == nil && authority == expected {
			return true
		}
	}
	return false
}

var _ http.RoundTripper = (*endpointHTTPTransport)(nil)
var _ interface{ CloseIdleConnections() } = (*endpointHTTPTransport)(nil)
