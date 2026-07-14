// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

const (
	defaultSafeHTTPTimeout          = 30 * time.Second
	defaultHTTPResponseBodyMaxBytes = int64(8 * 1024 * 1024)
	maximumHTTPResponseBodyMaxBytes = int64(64 * 1024 * 1024)
	maxSafeHTTPRedirects            = 3
)

var (
	ErrUnsafeRemoteAddress      = errors.New("mcp remote address is not allowed")
	ErrHTTPResponseBodyTooLarge = errors.New("mcp http response exceeds byte limit")
	ErrSSEEventLimitExceeded    = errors.New("mcp sse event exceeds limit")
)

type SafeHTTPPolicyOptions struct {
	AllowedHosts    []string
	AllowLocalDebug bool
}

type safeAllowedAuthority struct {
	host    string
	port    string
	hasPort bool
}

type SafeHTTPPolicy struct {
	allowed         []safeAllowedAuthority
	allowLocalDebug bool
}

type HTTPResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type HTTPDialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type SafeHTTPClientOptions struct {
	Policy               *SafeHTTPPolicy
	Resolver             HTTPResolver
	Dialer               HTTPDialer
	Timeout              time.Duration
	MaxResponseBodyBytes int64
}

type SafeHTTPTransport struct {
	base                 *http.Transport
	policy               *SafeHTTPPolicy
	resolver             HTTPResolver
	dialer               HTTPDialer
	maxResponseBodyBytes int64
}

func NewSafeHTTPPolicy(options SafeHTTPPolicyOptions) (*SafeHTTPPolicy, error) {
	if len(options.AllowedHosts) == 0 {
		return nil, ErrInvalidConnection
	}
	allowed := make([]safeAllowedAuthority, 0, len(options.AllowedHosts))
	seen := make(map[string]struct{}, len(options.AllowedHosts))
	for _, value := range options.AllowedHosts {
		authority, err := parseSafeAllowedAuthority(value)
		if err != nil {
			return nil, ErrInvalidConnection
		}
		key := authority.host + "|" + authority.port + "|" + strconv.FormatBool(authority.hasPort)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		allowed = append(allowed, authority)
	}
	if len(allowed) == 0 {
		return nil, ErrInvalidConnection
	}
	return &SafeHTTPPolicy{
		allowed:         allowed,
		allowLocalDebug: options.AllowLocalDebug,
	}, nil
}

func (p *SafeHTTPPolicy) ValidateURL(value *url.URL) error {
	if p == nil || value == nil || value.Opaque != "" || value.Host == "" ||
		value.User != nil || value.Fragment != "" {
		return ErrInvalidConnection
	}
	scheme := strings.ToLower(strings.TrimSpace(value.Scheme))
	if scheme != "https" && scheme != "http" {
		return ErrInvalidConnection
	}
	host, err := canonicalHTTPHostname(value.Hostname())
	if err != nil {
		return ErrInvalidConnection
	}
	port, err := effectiveHTTPPort(value, scheme)
	if err != nil {
		return ErrInvalidConnection
	}
	if host == "localhost" {
		if !p.allowLocalDebug {
			return ErrInvalidConnection
		}
	} else if scheme != "https" {
		return ErrInvalidConnection
	}
	for _, allowed := range p.allowed {
		if allowed.host != host {
			continue
		}
		if allowed.hasPort {
			if allowed.port == port {
				return nil
			}
			continue
		}
		if port == defaultHTTPPort(scheme) {
			return nil
		}
	}
	return ErrInvalidConnection
}

func NewSafeHTTPTransport(options SafeHTTPClientOptions) (*SafeHTTPTransport, error) {
	if options.Policy == nil {
		return nil, ErrInvalidConnection
	}
	resolver := options.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultSafeHTTPTimeout
	}
	dialer := options.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	}
	maxResponseBodyBytes := options.MaxResponseBodyBytes
	if maxResponseBodyBytes <= 0 {
		maxResponseBodyBytes = defaultHTTPResponseBodyMaxBytes
	}
	if maxResponseBodyBytes > maximumHTTPResponseBodyMaxBytes {
		return nil, ErrInvalidConnection
	}
	transport := &SafeHTTPTransport{
		policy:               options.Policy,
		resolver:             resolver,
		dialer:               dialer,
		maxResponseBodyBytes: maxResponseBodyBytes,
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.DialContext = transport.dialContext
	base.DialTLSContext = nil
	transport.base = base
	return transport, nil
}

func NewSafeHTTPClient(options SafeHTTPClientOptions) (*http.Client, error) {
	transport, err := NewSafeHTTPTransport(options)
	if err != nil {
		return nil, err
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultSafeHTTPTimeout
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request == nil || request.URL == nil || len(via) == 0 || len(via) > maxSafeHTTPRedirects {
			clearHTTPRedirectHeaders(request)
			return ErrInvalidConnection
		}
		if err := options.Policy.ValidateURL(request.URL); err != nil {
			clearHTTPRedirectHeaders(request)
			return ErrInvalidConnection
		}
		sameOrigin, err := sameHTTPOrigin(via[0].URL, request.URL)
		if err != nil || !sameOrigin {
			clearHTTPRedirectHeaders(request)
			return ErrInvalidConnection
		}
		return nil
	}
	return client, nil
}

func (t *SafeHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil || t.base == nil || request == nil || request.URL == nil {
		return nil, ErrInvalidConnection
	}
	if err := t.policy.ValidateURL(request.URL); err != nil {
		return nil, ErrInvalidConnection
	}
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices && isHTTPEventStream(response) {
		response.Body = newBoundedSSEReadCloser(response.Body)
		response.ContentLength = -1
		return response, nil
	}
	t.bufferBoundedResponse(response)
	return response, nil
}

func (t *SafeHTTPTransport) CloseIdleConnections() {
	if t != nil && t.base != nil {
		t.base.CloseIdleConnections()
	}
}

func (t *SafeHTTPTransport) dialContext(
	ctx context.Context,
	network string,
	address string,
) (net.Conn, error) {
	if t == nil || t.policy == nil || t.resolver == nil || t.dialer == nil {
		return nil, ErrUnsafeRemoteAddress
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrUnsafeRemoteAddress
	}
	host, err = canonicalHTTPHostname(host)
	if err != nil {
		return nil, ErrUnsafeRemoteAddress
	}
	if _, err := canonicalHTTPPort(port); err != nil {
		return nil, ErrUnsafeRemoteAddress
	}

	addresses := make([]netip.Addr, 0, 2)
	if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = append(addresses, literal)
	} else {
		resolved, resolveErr := t.resolver.LookupNetIP(ctx, "ip", host)
		if resolveErr != nil || len(resolved) == 0 {
			return nil, ErrUnsafeRemoteAddress
		}
		addresses = append(addresses, resolved...)
	}

	allowLoopback := t.policy.allowLocalDebug && host == "localhost"
	for _, address := range addresses {
		if !address.IsValid() || address.Zone() != "" || address.Is4In6() ||
			unsafeHTTPAddress(address, allowLoopback) {
			return nil, ErrUnsafeRemoteAddress
		}
	}
	for _, ip := range addresses {
		if network == "tcp4" && !ip.Is4() {
			continue
		}
		if network == "tcp6" && !ip.Is6() {
			continue
		}
		connection, dialErr := t.dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, ErrSessionUnavailable
}

func (t *SafeHTTPTransport) bufferBoundedResponse(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	original := response.Body
	if response.ContentLength > t.maxResponseBodyBytes {
		_ = original.Close()
		setFixedHTTPBodyError(response, ErrHTTPResponseBodyTooLarge)
		return
	}
	data, err := io.ReadAll(io.LimitReader(original, t.maxResponseBodyBytes+1))
	closeErr := original.Close()
	if int64(len(data)) > t.maxResponseBodyBytes {
		setFixedHTTPBodyError(response, ErrHTTPResponseBodyTooLarge)
		return
	}
	if err != nil || closeErr != nil {
		setFixedHTTPBodyError(response, ErrSessionUnavailable)
		return
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
}

type fixedHTTPErrorBody struct {
	err error
}

func (b *fixedHTTPErrorBody) Read([]byte) (int, error) {
	return 0, b.err
}

func (b *fixedHTTPErrorBody) Close() error {
	return nil
}

func setFixedHTTPBodyError(response *http.Response, err error) {
	response.Body = &fixedHTTPErrorBody{err: err}
	response.ContentLength = -1
}

func isHTTPEventStream(response *http.Response) bool {
	if response == nil {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return err == nil && mediaType == "text/event-stream"
}

func unsafeHTTPAddress(address netip.Addr, allowLoopback bool) bool {
	if !address.IsValid() || address.Is4In6() {
		return true
	}
	if address.IsLoopback() {
		return !allowLoopback
	}
	if embedded, ok := embeddedIPv4Address(address); ok && unsafeEmbeddedIPv4Address(embedded) {
		return true
	}
	if isSpecialPurposeAddress(address) {
		return true
	}
	if !address.IsGlobalUnicast() || address.IsUnspecified() || address.IsPrivate() ||
		address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsInterfaceLocalMulticast() || address.IsMulticast() {
		return true
	}
	if netip.MustParsePrefix("100.64.0.0/10").Contains(address) {
		return true
	}
	for _, metadata := range []netip.Addr{
		netip.MustParseAddr("169.254.169.254"),
		netip.MustParseAddr("169.254.170.2"),
		netip.MustParseAddr("100.100.100.200"),
		netip.MustParseAddr("192.0.0.192"),
		netip.MustParseAddr("fd00:ec2::254"),
	} {
		if address == metadata {
			return true
		}
	}
	return false
}

func parseSafeAllowedAuthority(value string) (safeAllowedAuthority, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "://") || strings.ContainsAny(value, "/?#@") {
		return safeAllowedAuthority{}, ErrInvalidConnection
	}
	host := value
	port := ""
	hasPort := false
	if strings.HasPrefix(value, "[") {
		closing := strings.IndexByte(value, ']')
		if closing <= 1 {
			return safeAllowedAuthority{}, ErrInvalidConnection
		}
		host = value[1:closing]
		remainder := value[closing+1:]
		if remainder != "" {
			if !strings.HasPrefix(remainder, ":") {
				return safeAllowedAuthority{}, ErrInvalidConnection
			}
			port = strings.TrimPrefix(remainder, ":")
			hasPort = true
		}
	} else if strings.Count(value, ":") == 1 {
		separator := strings.LastIndexByte(value, ':')
		host, port, hasPort = value[:separator], value[separator+1:], true
	} else if strings.Count(value, ":") > 1 {
		if _, err := netip.ParseAddr(value); err != nil {
			return safeAllowedAuthority{}, ErrInvalidConnection
		}
	}
	canonicalHost, err := canonicalHTTPHostname(host)
	if err != nil {
		return safeAllowedAuthority{}, ErrInvalidConnection
	}
	if hasPort {
		port, err = canonicalHTTPPort(port)
		if err != nil {
			return safeAllowedAuthority{}, ErrInvalidConnection
		}
	}
	return safeAllowedAuthority{host: canonicalHost, port: port, hasPort: hasPort}, nil
}

func canonicalHTTPHostname(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.Contains(value, "%") || strings.HasSuffix(value, "..") {
		return "", ErrInvalidConnection
	}
	value = strings.TrimSuffix(value, ".")
	if address, err := netip.ParseAddr(value); err == nil {
		return address.String(), nil
	}
	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil || ascii == "" {
		return "", ErrInvalidConnection
	}
	ascii = strings.ToLower(ascii)
	if len(ascii) > 253 {
		return "", ErrInvalidConnection
	}
	return ascii, nil
}

func canonicalHTTPPort(value string) (string, error) {
	port, err := strconv.Atoi(value)
	if err != nil || port <= 0 || port > 65535 {
		return "", ErrInvalidConnection
	}
	return strconv.Itoa(port), nil
}

func effectiveHTTPPort(value *url.URL, scheme string) (string, error) {
	port := value.Port()
	if port == "" {
		return defaultHTTPPort(scheme), nil
	}
	return canonicalHTTPPort(port)
}

func defaultHTTPPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}
	return "80"
}

func sameHTTPOrigin(left, right *url.URL) (bool, error) {
	if left == nil || right == nil {
		return false, ErrInvalidConnection
	}
	leftScheme := strings.ToLower(left.Scheme)
	rightScheme := strings.ToLower(right.Scheme)
	leftHost, err := canonicalHTTPHostname(left.Hostname())
	if err != nil {
		return false, err
	}
	rightHost, err := canonicalHTTPHostname(right.Hostname())
	if err != nil {
		return false, err
	}
	leftPort, err := effectiveHTTPPort(left, leftScheme)
	if err != nil {
		return false, err
	}
	rightPort, err := effectiveHTTPPort(right, rightScheme)
	if err != nil {
		return false, err
	}
	return leftScheme == rightScheme && leftHost == rightHost && leftPort == rightPort, nil
}

func clearHTTPRedirectHeaders(request *http.Request) {
	if request != nil {
		request.Header = make(http.Header)
	}
}
