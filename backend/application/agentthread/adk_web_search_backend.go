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

package agentthread

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	agentThreadWebSearchEnabledEnv          = "AGENT_THREAD_WEB_SEARCH_ENABLED"
	agentThreadWebSearchEndpointEnv         = "AGENT_THREAD_WEB_SEARCH_ENDPOINT"
	agentThreadWebSearchAPIKeyEnv           = "AGENT_THREAD_WEB_SEARCH_API_KEY"
	agentThreadWebSearchHeaderEnv           = "AGENT_THREAD_WEB_SEARCH_HEADER"
	agentThreadWebSearchTimeoutMsEnv        = "AGENT_THREAD_WEB_SEARCH_TIMEOUT_MS"
	agentThreadWebSearchMaxResponseBytesEnv = "AGENT_THREAD_WEB_SEARCH_MAX_RESPONSE_BYTES"
	agentThreadWebSearchAllowHTTPEnv        = "AGENT_THREAD_WEB_SEARCH_ALLOW_HTTP"
	agentThreadWebSearchAllowPrivateIPsEnv  = "AGENT_THREAD_WEB_SEARCH_ALLOW_PRIVATE_IPS"
)

const (
	defaultADKHTTPWebSearchTimeout       = 10 * time.Second
	maxADKHTTPWebSearchTimeout           = 60 * time.Second
	defaultADKHTTPWebSearchResponseBytes = 64 << 10
	minADKHTTPWebSearchResponseBytes     = 1 << 10
	maxADKHTTPWebSearchResponseBytes     = 1 << 20
	defaultADKHTTPWebSearchHeader        = "Authorization"
	defaultADKHTTPWebSearchSource        = "coze_http_web_search"
	defaultADKEnvHTTPWebSearchSource     = "coze_env_http"
)

var errADKWebSearchRedirectTargetNotAllowed = errors.New("web search redirect target is not allowed")

type ADKHTTPWebSearchBackendOptions struct {
	Endpoint         string
	APIKey           string
	HeaderName       string
	Timeout          time.Duration
	MaxResponseBytes int64
	AllowHTTP        bool
	AllowPrivateIPs  bool
	Source           string
	Client           *http.Client
}

type ADKHTTPWebSearchBackend struct {
	endpoint         string
	apiKey           string
	headerName       string
	client           *http.Client
	maxResponseBytes int64
	source           string
}

func ADKWebSearchBackendFromEnv() (ADKWebSearchBackend, bool, error) {
	enabled, err := adkWebSearchBoolEnv(agentThreadWebSearchEnabledEnv, false)
	if err != nil {
		return nil, false, err
	}
	if !enabled {
		return nil, false, nil
	}
	allowHTTP, err := adkWebSearchBoolEnv(agentThreadWebSearchAllowHTTPEnv, false)
	if err != nil {
		return nil, true, err
	}
	allowPrivateIPs, err := adkWebSearchBoolEnv(
		agentThreadWebSearchAllowPrivateIPsEnv,
		false,
	)
	if err != nil {
		return nil, true, err
	}
	timeoutMillis, err := adkWebSearchPositiveInt64Env(
		agentThreadWebSearchTimeoutMsEnv,
		int64(defaultADKHTTPWebSearchTimeout/time.Millisecond),
	)
	if err != nil {
		return nil, true, err
	}
	maxResponseBytes, err := adkWebSearchPositiveInt64Env(
		agentThreadWebSearchMaxResponseBytesEnv,
		defaultADKHTTPWebSearchResponseBytes,
	)
	if err != nil {
		return nil, true, err
	}

	backend, err := NewADKHTTPWebSearchBackend(
		ADKHTTPWebSearchBackendOptions{
			Endpoint:         os.Getenv(agentThreadWebSearchEndpointEnv),
			APIKey:           os.Getenv(agentThreadWebSearchAPIKeyEnv),
			HeaderName:       os.Getenv(agentThreadWebSearchHeaderEnv),
			Timeout:          time.Duration(timeoutMillis) * time.Millisecond,
			MaxResponseBytes: maxResponseBytes,
			AllowHTTP:        allowHTTP,
			AllowPrivateIPs:  allowPrivateIPs,
			Source:           defaultADKEnvHTTPWebSearchSource,
		},
	)
	if err != nil {
		return nil, true, err
	}

	return backend, true, nil
}

func NewADKHTTPWebSearchBackend(
	options ADKHTTPWebSearchBackendOptions,
) (*ADKHTTPWebSearchBackend, error) {
	endpoint, err := validateADKWebSearchEndpoint(
		options.Endpoint,
		options.AllowHTTP,
		options.AllowPrivateIPs,
	)
	if err != nil {
		return nil, err
	}
	headerName := strings.TrimSpace(options.HeaderName)
	if headerName == "" {
		headerName = defaultADKHTTPWebSearchHeader
	}
	if !validADKWebSearchHeaderName(headerName) {
		return nil, fmt.Errorf("web search auth header is invalid")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultADKHTTPWebSearchTimeout
	}
	if timeout > maxADKHTTPWebSearchTimeout {
		timeout = maxADKHTTPWebSearchTimeout
	}
	maxResponseBytes := boundedInt64(
		options.MaxResponseBytes,
		defaultADKHTTPWebSearchResponseBytes,
		minADKHTTPWebSearchResponseBytes,
		maxADKHTTPWebSearchResponseBytes,
	)
	source := trimADKWebText(options.Source, 128)
	if source == "" {
		source = defaultADKHTTPWebSearchSource
	}
	client := options.Client
	if client == nil {
		client = &http.Client{}
	}
	cloned := *client
	if cloned.Timeout <= 0 || cloned.Timeout > timeout {
		cloned.Timeout = timeout
	}
	policy := adkWebSearchEndpointPolicy{
		host:            normalizeADKWebHost(endpoint.Hostname()),
		allowHTTP:       options.AllowHTTP,
		allowPrivateIPs: options.AllowPrivateIPs,
	}
	cloned.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req == nil || req.URL == nil {
			return errADKWebSearchRedirectTargetNotAllowed
		}
		if err := policy.validateRedirect(req.URL); err != nil {
			return fmt.Errorf("%w: %v", errADKWebSearchRedirectTargetNotAllowed, err)
		}
		return nil
	}

	return &ADKHTTPWebSearchBackend{
		endpoint:         endpoint.String(),
		apiKey:           strings.TrimSpace(options.APIKey),
		headerName:       http.CanonicalHeaderKey(headerName),
		client:           &cloned,
		maxResponseBytes: maxResponseBytes,
		source:           source,
	}, nil
}

func (b *ADKHTTPWebSearchBackend) SearchADKWeb(
	ctx context.Context,
	request ADKWebSearchRequest,
) (*ADKWebSearchResponse, error) {
	if b == nil || b.client == nil || strings.TrimSpace(b.endpoint) == "" {
		return nil, fmt.Errorf("web search backend is not configured")
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return nil, fmt.Errorf("web search query is required")
	}
	request.MaxResults = boundedADKHTTPWebSearchRequestResults(request.MaxResults)
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("web search backend request is invalid")
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		b.endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		return nil, fmt.Errorf("web search backend request is invalid")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "Coze-Agent-WebSearch/1.0")
	if b.apiKey != "" {
		httpReq.Header.Set(
			b.headerName,
			adkWebSearchAuthHeaderValue(b.headerName, b.apiKey),
		)
	}

	resp, err := b.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, errADKWebSearchRedirectTargetNotAllowed) {
			return nil, fmt.Errorf("web search backend redirect target is not allowed")
		}
		return nil, fmt.Errorf("web search backend request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("web search backend returned non-2xx status")
	}
	body, err := readBoundedADKWebSearchResponse(resp.Body, b.maxResponseBytes)
	if err != nil {
		return nil, err
	}

	var decoded ADKWebSearchResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("web search backend response is invalid")
	}
	decoded.Results = normalizeADKHTTPWebSearchResults(
		decoded.Results,
		request.MaxResults,
		b.source,
	)

	return &decoded, nil
}

type adkWebSearchEndpointPolicy struct {
	host            string
	allowHTTP       bool
	allowPrivateIPs bool
}

func (p adkWebSearchEndpointPolicy) validateRedirect(target *url.URL) error {
	if target == nil {
		return fmt.Errorf("web search redirect target is required")
	}
	if _, err := validateADKWebSearchEndpoint(
		target.String(),
		p.allowHTTP,
		p.allowPrivateIPs,
	); err != nil {
		return err
	}
	if normalizeADKWebHost(target.Hostname()) != p.host {
		return fmt.Errorf("web search redirect host is not allowed")
	}

	return nil
}

func validateADKWebSearchEndpoint(
	rawEndpoint string,
	allowHTTP bool,
	allowPrivateIPs bool,
) (*url.URL, error) {
	rawEndpoint = strings.TrimSpace(rawEndpoint)
	if rawEndpoint == "" {
		return nil, fmt.Errorf("web search endpoint is required")
	}
	parsed, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, fmt.Errorf("web search endpoint is invalid")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	switch scheme {
	case "https":
	case "http":
		if !allowHTTP {
			return nil, fmt.Errorf("web search http scheme is not allowed")
		}
	default:
		return nil, fmt.Errorf("web search endpoint scheme is not allowed")
	}
	if parsed.User != nil {
		return nil, fmt.Errorf("web search endpoint must not include userinfo")
	}
	if strings.TrimSpace(parsed.RawQuery) != "" || strings.TrimSpace(parsed.Fragment) != "" {
		return nil, fmt.Errorf("web search endpoint must not include query or fragment")
	}
	host := normalizeADKWebHost(parsed.Hostname())
	if host == "" {
		return nil, fmt.Errorf("web search endpoint host is required")
	}
	if err := validateADKWebSearchHostAddress(host, allowPrivateIPs); err != nil {
		return nil, err
	}

	return parsed, nil
}

func validateADKWebSearchHostAddress(host string, allowPrivateIPs bool) error {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	if allowPrivateIPs {
		return nil
	}
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return fmt.Errorf("web search private ip is not allowed")
	}

	return nil
}

func readBoundedADKWebSearchResponse(reader io.Reader, maxBytes int64) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("web search backend response is invalid")
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("web search backend response read failed")
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("web search backend response is too large")
	}

	return body, nil
}

func normalizeADKHTTPWebSearchResults(
	results []ADKWebSearchResult,
	limit int,
	source string,
) []ADKWebSearchResult {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	normalized := make([]ADKWebSearchResult, 0, limit)
	for index := 0; index < limit; index++ {
		result := results[index]
		result.Source = trimADKWebText(result.Source, 128)
		if result.Source == "" {
			result.Source = source
		}
		normalized = append(normalized, result)
	}

	return normalized
}

func boundedADKHTTPWebSearchRequestResults(requested int) int {
	if requested <= 0 {
		return defaultADKWebSearchResults
	}
	if requested > maxADKWebSearchResults {
		return maxADKWebSearchResults
	}

	return requested
}

func adkWebSearchBoolEnv(key string, defaultValue bool) (bool, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", key, err)
	}

	return parsed, nil
}

func adkWebSearchPositiveInt64Env(key string, defaultValue int64) (int64, error) {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}

	return parsed, nil
}

func validADKWebSearchHeaderName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", char) {
			continue
		}
		return false
	}

	return true
}

func adkWebSearchAuthHeaderValue(headerName string, apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if strings.EqualFold(headerName, "Authorization") &&
		!strings.HasPrefix(strings.ToLower(apiKey), "bearer ") {
		return "Bearer " + apiKey
	}

	return apiKey
}
