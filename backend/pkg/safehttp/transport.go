// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package safehttp

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
	"strings"
	"sync"
	"time"
)

const (
	DefaultTimeout              = 30 * time.Second
	MaximumTimeout              = 2 * time.Minute
	DefaultResponseBodyMaxBytes = int64(8 * 1024 * 1024)
	MaximumResponseBodyMaxBytes = int64(64 * 1024 * 1024)
	MaximumRedirects            = 3
)

var (
	ErrUnsafeAddress        = errors.New("safe HTTP remote address is not allowed")
	ErrResponseBodyTooLarge = errors.New("safe HTTP response exceeds byte limit")
	ErrTransportUnavailable = errors.New("safe HTTP transport is unavailable")
)

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

type ClientOptions struct {
	Policy               *Policy
	Timeout              time.Duration
	MaxResponseBodyBytes int64
	UnavailableError     error
	AllowEventStream     bool
}

type transportTestOptions struct {
	resolver         Resolver
	dialer           Dialer
	baseRoundTripper http.RoundTripper
}

type dialValidationContextKey struct{}

type dialValidationContext struct {
	host              string
	debugLoopbackHTTP bool
}

type Transport struct {
	base                 *http.Transport
	delegate             http.RoundTripper
	customDelegate       bool
	policy               *Policy
	resolver             Resolver
	dialer               Dialer
	maxResponseBodyBytes int64
	unavailableError     error
	allowEventStream     bool
	timeout              time.Duration
}

type responseBodyLimitContextKey struct{}

func WithResponseBodyLimit(ctx context.Context, limit int64) context.Context {
	if ctx == nil || limit <= 0 {
		return ctx
	}
	return context.WithValue(ctx, responseBodyLimitContextKey{}, limit)
}

func NewTransport(options ClientOptions) (*Transport, error) {
	return newTransport(options, transportTestOptions{})
}

func newTransportForTest(options ClientOptions, testOptions transportTestOptions) (*Transport, error) {
	return newTransport(options, testOptions)
}

func newTransport(options ClientOptions, testOptions transportTestOptions) (*Transport, error) {
	if options.Policy == nil {
		return nil, ErrURLRejected
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if timeout > MaximumTimeout {
		return nil, options.Policy.reject()
	}
	maxResponseBodyBytes := options.MaxResponseBodyBytes
	if maxResponseBodyBytes <= 0 {
		maxResponseBodyBytes = DefaultResponseBodyMaxBytes
	}
	if maxResponseBodyBytes > MaximumResponseBodyMaxBytes {
		return nil, options.Policy.reject()
	}
	resolver := testOptions.resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := testOptions.dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	}
	unavailableError := options.UnavailableError
	if unavailableError == nil {
		unavailableError = ErrTransportUnavailable
	}
	transport := &Transport{
		policy:               options.Policy,
		resolver:             resolver,
		dialer:               dialer,
		maxResponseBodyBytes: maxResponseBodyBytes,
		unavailableError:     unavailableError,
		allowEventStream:     options.AllowEventStream,
		timeout:              timeout,
	}
	if testOptions.baseRoundTripper != nil {
		transport.delegate = testOptions.baseRoundTripper
		transport.customDelegate = true
		return transport, nil
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.DisableCompression = true
	base.DialContext = transport.dialContext
	base.DialTLSContext = nil
	base.TLSHandshakeTimeout = minDuration(timeout, 10*time.Second)
	base.ResponseHeaderTimeout = timeout
	base.ExpectContinueTimeout = time.Second
	base.IdleConnTimeout = 90 * time.Second
	base.MaxIdleConns = 128
	base.MaxIdleConnsPerHost = 16
	base.MaxConnsPerHost = 64
	transport.base = base
	transport.delegate = base
	return transport, nil
}

func NewClient(options ClientOptions) (*http.Client, error) {
	return newClient(options, transportTestOptions{})
}

func newClientForTest(options ClientOptions, testOptions transportTestOptions) (*http.Client, error) {
	return newClient(options, testOptions)
}

func newClient(options ClientOptions, testOptions transportTestOptions) (*http.Client, error) {
	transport, err := newTransport(options, testOptions)
	if err != nil {
		return nil, err
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	client := &http.Client{Transport: transport, Timeout: timeout}
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request == nil || request.URL == nil || len(via) == 0 || len(via) > MaximumRedirects {
			clearRedirectHeaders(request)
			return options.Policy.reject()
		}
		if err := options.Policy.ValidateRequest(request); err != nil {
			clearRedirectHeaders(request)
			return options.Policy.reject()
		}
		sameOrigin, err := SameOrigin(via[0].URL, request.URL)
		if err != nil || !sameOrigin {
			clearRedirectHeaders(request)
			return options.Policy.reject()
		}
		return nil
	}
	return client, nil
}

func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil || t.delegate == nil || t.policy == nil || request == nil || request.URL == nil {
		return nil, ErrURLRejected
	}
	if err := t.policy.ValidateRequest(request); err != nil {
		return nil, err
	}
	validatedHost, _ := CanonicalHostname(request.URL.Hostname())
	dialContext := context.WithValue(request.Context(), dialValidationContextKey{}, dialValidationContext{
		host:              validatedHost,
		debugLoopbackHTTP: t.policy.debugLocalHTTP && strings.EqualFold(request.URL.Scheme, "http"),
	})
	requestContext, cancel := context.WithTimeout(dialContext, t.timeout)
	timedRequest := request.Clone(requestContext)
	response, err := t.delegate.RoundTrip(timedRequest)
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
	if t.customDelegate {
		if _, ok := response.Body.(contextBoundResponseBody); !ok {
			cancel()
			setFixedBodyError(response, t.unavailableError)
			return response, nil
		}
	}
	response.Body = newDeadlineBody(response.Body, requestContext, cancel)
	if t.allowEventStream && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices && isEventStream(response) {
		response.ContentLength = -1
		return response, nil
	}
	t.bufferResponse(timedRequest, response)
	return response, nil
}

func (t *Transport) CloseIdleConnections() {
	if t != nil && t.base != nil {
		t.base.CloseIdleConnections()
		return
	}
	if t != nil {
		if closer, ok := t.delegate.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
	}
}

func (t *Transport) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return t.dialContext(ctx, network, address)
}

func (t *Transport) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if t == nil || t.policy == nil || t.resolver == nil || t.dialer == nil ||
		(network != "tcp" && network != "tcp4" && network != "tcp6") {
		return nil, ErrUnsafeAddress
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrUnsafeAddress
	}
	host, err = CanonicalHostname(host)
	if err != nil {
		return nil, ErrUnsafeAddress
	}
	port, err = CanonicalPort(port)
	if err != nil || !t.policy.dialAuthorityAllowed(host, port) {
		return nil, ErrUnsafeAddress
	}
	validation, hasValidation := ctx.Value(dialValidationContextKey{}).(dialValidationContext)
	debugLoopbackHTTP := hasValidation && validation.host == host && validation.debugLoopbackHTTP

	addresses := make([]netip.Addr, 0, 2)
	if literal, parseErr := netip.ParseAddr(host); parseErr == nil {
		addresses = append(addresses, literal)
	} else {
		resolved, resolveErr := t.resolver.LookupNetIP(ctx, "ip", host)
		if resolveErr != nil || len(resolved) == 0 {
			return nil, ErrUnsafeAddress
		}
		addresses = append(addresses, resolved...)
	}
	for _, resolved := range addresses {
		if !t.policy.addressAllowed(resolved, debugLoopbackHTTP) {
			return nil, ErrUnsafeAddress
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

func (t *Transport) bufferResponse(request *http.Request, response *http.Response) {
	limit := t.maxResponseBodyBytes
	if request != nil && request.Context() != nil {
		if requested, ok := request.Context().Value(responseBodyLimitContextKey{}).(int64); ok && requested > 0 && requested < limit {
			limit = requested
		}
	}
	original := response.Body
	if response.ContentLength > limit {
		_ = original.Close()
		setFixedBodyError(response, ErrResponseBodyTooLarge)
		return
	}
	data, readErr := io.ReadAll(io.LimitReader(original, limit+1))
	closeErr := original.Close()
	if int64(len(data)) > limit {
		setFixedBodyError(response, ErrResponseBodyTooLarge)
		return
	}
	if readErr != nil || closeErr != nil {
		setFixedBodyError(response, t.unavailableError)
		return
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
}

type deadlineBody struct {
	body      io.ReadCloser
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
	closeErr  error
}

// contextBoundResponseBody is intentionally package-private. Production
// transports use net/http response bodies, whose Reads are interrupted by the
// request context. Injected RoundTrippers are test-only and must explicitly
// prove the same property; otherwise their bodies are rejected without Read or
// Close so an uncooperative implementation cannot leak a goroutine.
type contextBoundResponseBody interface {
	io.ReadCloser
	safeHTTPContextBoundBody()
}

func newDeadlineBody(body io.ReadCloser, ctx context.Context, cancel context.CancelFunc) io.ReadCloser {
	return &deadlineBody{body: body, ctx: ctx, cancel: cancel}
}

func (b *deadlineBody) Read(buffer []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		b.cancel()
		return 0, err
	}
	count, err := b.body.Read(buffer)
	if contextErr := b.ctx.Err(); contextErr != nil {
		b.cancel()
		return 0, contextErr
	}
	if err != nil {
		b.cancel()
	}
	return count, err
}

func (b *deadlineBody) Close() error {
	b.closeOnce.Do(func() {
		b.cancel()
		b.closeErr = b.body.Close()
	})
	return b.closeErr
}

type fixedErrorBody struct {
	err error
}

func (b *fixedErrorBody) Read([]byte) (int, error) { return 0, b.err }
func (b *fixedErrorBody) Close() error             { return nil }

func setFixedBodyError(response *http.Response, err error) {
	response.Body = &fixedErrorBody{err: err}
	response.ContentLength = -1
}

func isEventStream(response *http.Response) bool {
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	return err == nil && mediaType == "text/event-stream"
}

func clearRedirectHeaders(request *http.Request) {
	if request != nil {
		request.Header = make(http.Header)
	}
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}

func IsEventStream(response *http.Response) bool {
	return isEventStream(response)
}

func ClearRedirectHeaders(request *http.Request) {
	clearRedirectHeaders(request)
}

func NormalizeAuthority(value *url.URL) (string, error) {
	if value == nil {
		return "", ErrURLRejected
	}
	host, err := CanonicalHostname(value.Hostname())
	if err != nil {
		return "", err
	}
	port, err := EffectivePort(value)
	if err != nil {
		return "", err
	}
	return formatAuthority(host, port, value.Port() != ""), nil
}
