// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package mcpruntime

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/coze-dev/coze-studio/backend/pkg/safehttp"
)

const (
	defaultSafeHTTPTimeout          = safehttp.DefaultTimeout
	defaultHTTPResponseBodyMaxBytes = safehttp.DefaultResponseBodyMaxBytes
	maximumHTTPResponseBodyMaxBytes = safehttp.MaximumResponseBodyMaxBytes
	maxSafeHTTPRedirects            = safehttp.MaximumRedirects
)

var (
	ErrUnsafeRemoteAddress      = safehttp.ErrUnsafeAddress
	ErrHTTPResponseBodyTooLarge = safehttp.ErrResponseBodyTooLarge
	ErrSSEEventLimitExceeded    = errors.New("mcp sse event exceeds limit")
)

type SafeHTTPPolicyOptions struct {
	AllowedHosts        []string
	AllowedPrivateCIDRs []string
	AllowLocalDebug     bool
}

type SafeHTTPPolicy struct {
	inner *safehttp.Policy
}

type HTTPResolver = safehttp.Resolver
type HTTPDialer = safehttp.Dialer

type SafeHTTPClientOptions struct {
	Policy               *SafeHTTPPolicy
	Resolver             HTTPResolver
	Dialer               HTTPDialer
	Timeout              time.Duration
	MaxResponseBodyBytes int64
}

type SafeHTTPTransport struct {
	inner *safehttp.Transport
}

const mcpRuntimeAllowLocalHTTPEnv = "MCP_RUNTIME_ALLOW_LOCAL_HTTP"

func NewSafeHTTPPolicy(options SafeHTTPPolicyOptions) (*SafeHTTPPolicy, error) {
	policyOptions := safehttp.PolicyOptions{
		AllowedHosts:        append([]string(nil), options.AllowedHosts...),
		AllowedPrivateCIDRs: append([]string(nil), options.AllowedPrivateCIDRs...),
		RejectedError:       ErrInvalidConnection,
	}
	var (
		policy *safehttp.Policy
		err    error
	)
	if mcpLocalHTTPDebugAuthorized(options) {
		policy, err = safehttp.NewDebugAuthorizedPolicy(safehttp.DebugPolicyOptions{
			AllowedHosts:        policyOptions.AllowedHosts,
			AllowedPrivateCIDRs: policyOptions.AllowedPrivateCIDRs,
			RejectedError:       policyOptions.RejectedError,
		})
	} else {
		policy, err = safehttp.NewPolicy(policyOptions)
	}
	if err != nil {
		return nil, ErrInvalidConnection
	}
	return &SafeHTTPPolicy{inner: policy}, nil
}

func (p *SafeHTTPPolicy) ValidateURL(value *url.URL) error {
	if p == nil || p.inner == nil || p.inner.ValidateURL(value) != nil {
		return ErrInvalidConnection
	}
	return nil
}

func NewSafeHTTPTransport(options SafeHTTPClientOptions) (*SafeHTTPTransport, error) {
	if options.Policy == nil || options.Policy.inner == nil || options.Resolver != nil || options.Dialer != nil {
		return nil, ErrInvalidConnection
	}
	transport, err := safehttp.NewTransport(sharedHTTPClientOptions(options))
	if err != nil {
		return nil, ErrInvalidConnection
	}
	return &SafeHTTPTransport{inner: transport}, nil
}

func NewSafeHTTPClient(options SafeHTTPClientOptions) (*http.Client, error) {
	if options.Policy == nil || options.Policy.inner == nil || options.Resolver != nil || options.Dialer != nil {
		return nil, ErrInvalidConnection
	}
	client, err := safehttp.NewClient(sharedHTTPClientOptions(options))
	if err != nil {
		return nil, ErrInvalidConnection
	}
	transport, ok := client.Transport.(*safehttp.Transport)
	if !ok {
		return nil, ErrInvalidConnection
	}
	client.Transport = &SafeHTTPTransport{inner: transport}
	return client, nil
}

func sharedHTTPClientOptions(options SafeHTTPClientOptions) safehttp.ClientOptions {
	return safehttp.ClientOptions{
		Policy:               options.Policy.inner,
		Timeout:              options.Timeout,
		MaxResponseBodyBytes: options.MaxResponseBodyBytes,
		UnavailableError:     ErrSessionUnavailable,
		AllowEventStream:     true,
	}
}

func mcpLocalHTTPDebugAuthorized(options SafeHTTPPolicyOptions) bool {
	return options.AllowLocalDebug && os.Getenv("APP_ENV") == "debug" &&
		os.Getenv(mcpRuntimeAllowLocalHTTPEnv) == "true"
}

func (t *SafeHTTPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil || t.inner == nil {
		return nil, ErrInvalidConnection
	}
	response, err := t.inner.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices && isHTTPEventStream(response) {
		response.Body = newBoundedSSEReadCloser(response.Body)
		response.ContentLength = -1
	}
	return response, nil
}

func (t *SafeHTTPTransport) CloseIdleConnections() {
	if t != nil && t.inner != nil {
		t.inner.CloseIdleConnections()
	}
}

func (t *SafeHTTPTransport) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if t == nil || t.inner == nil {
		return nil, ErrUnsafeRemoteAddress
	}
	return t.inner.DialContext(ctx, network, address)
}

func isHTTPEventStream(response *http.Response) bool {
	return safehttp.IsEventStream(response)
}

func canonicalHTTPHostname(value string) (string, error) {
	host, err := safehttp.CanonicalHostname(value)
	if err != nil {
		return "", ErrInvalidConnection
	}
	return host, nil
}

func canonicalHTTPPort(value string) (string, error) {
	port, err := safehttp.CanonicalPort(value)
	if err != nil {
		return "", ErrInvalidConnection
	}
	return port, nil
}

func effectiveHTTPPort(value *url.URL, _ string) (string, error) {
	port, err := safehttp.EffectivePort(value)
	if err != nil {
		return "", ErrInvalidConnection
	}
	return port, nil
}

func defaultHTTPPort(scheme string) string {
	return safehttp.DefaultPort(scheme)
}

func sameHTTPOrigin(left, right *url.URL) (bool, error) {
	same, err := safehttp.SameOrigin(left, right)
	if err != nil {
		return false, ErrInvalidConnection
	}
	return same, nil
}

func clearHTTPRedirectHeaders(request *http.Request) {
	safehttp.ClearRedirectHeaders(request)
}
