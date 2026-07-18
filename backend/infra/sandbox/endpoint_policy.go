// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package sandbox

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	domainsandbox "github.com/coze-dev/coze-studio/backend/domain/sandbox"
	"github.com/coze-dev/coze-studio/backend/pkg/safehttp"
)

type RemoteProviderConfig struct {
	Endpoint            string
	Credential          string
	AllowedHosts        []string
	AllowedPrivateCIDRs []string
	Timeout             time.Duration
}

type endpointPolicy struct {
	endpoint   *url.URL
	httpPolicy *safehttp.Policy
}

func newEndpointPolicy(config RemoteProviderConfig) (*endpointPolicy, error) {
	if config.Endpoint == "" || strings.TrimSpace(config.Endpoint) != config.Endpoint {
		return nil, domainsandbox.ErrInvalidInput
	}
	parsed, err := url.Parse(config.Endpoint)
	if err != nil || parsed == nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" ||
		parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawFragment != "" || parsed.RawPath != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return nil, domainsandbox.ErrInvalidInput
	}
	policy, err := safehttp.NewPolicy(safehttp.PolicyOptions{
		AllowedHosts:        append([]string(nil), config.AllowedHosts...),
		AllowedPrivateCIDRs: append([]string(nil), config.AllowedPrivateCIDRs...),
	})
	if err != nil || policy.ValidateURL(parsed) != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	authority, err := safehttp.NormalizeAuthority(parsed)
	if err != nil {
		return nil, domainsandbox.ErrInvalidInput
	}
	return &endpointPolicy{
		endpoint:   &url.URL{Scheme: "https", Host: authority, Path: "/"},
		httpPolicy: policy,
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
		p.httpPolicy.ValidateRequest(request) != nil {
		return domainsandbox.ErrInvalidInput
	}
	sameOrigin, err := safehttp.SameOrigin(p.endpoint, request.URL)
	if err != nil || !sameOrigin {
		return domainsandbox.ErrInvalidInput
	}
	return nil
}
