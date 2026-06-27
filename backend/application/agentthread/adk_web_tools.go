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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var errADKWebRedirectTargetNotAllowed = errors.New("redirect target is not allowed")

const (
	adkWebFetchToolName  = "web_fetch"
	adkWebSearchToolName = "web_search"
	adkWebFetchSchema    = "coze.web_fetch.v1"
	adkWebSearchSchema   = "coze.web_search.v1"

	defaultADKWebFetchTimeout       = 10 * time.Second
	maxADKWebFetchTimeout           = 60 * time.Second
	defaultADKWebFetchResponseBytes = 32 << 10
	minADKWebFetchResponseBytes     = 1 << 10
	maxADKWebFetchResponseBytes     = 1 << 20
	defaultADKWebSearchResults      = 5
	maxADKWebSearchResults          = 10
)

const adkWebFetchInputSchema = `{
  "type":"object",
  "properties":{
    "url":{
      "type":"string",
      "description":"HTTPS URL to fetch from an allowed host."
    }
  },
  "required":["url"]
}`

const adkWebSearchInputSchema = `{
  "type":"object",
  "properties":{
    "query":{
      "type":"string",
      "description":"Search query."
    },
    "max_results":{
      "type":"integer",
      "description":"Maximum number of search results to return."
    }
  },
  "required":["query"]
}`

type ADKWebToolCatalogOptions struct {
	HTTPClient    *http.Client
	SearchBackend ADKWebSearchBackend
}

type ADKWebToolCatalog struct {
	options ADKWebToolCatalogOptions
}

func NewADKWebToolCatalog(
	options ADKWebToolCatalogOptions,
) *ADKWebToolCatalog {
	return &ADKWebToolCatalog{options: options}
}

func (c *ADKWebToolCatalog) LoadADKRuntimeTools(
	_ context.Context,
	run *RunSummary,
) ([]ADKRuntimeToolDefinition, error) {
	if c == nil {
		return nil, fmt.Errorf("eino adk web tool catalog is required")
	}
	config, err := adkWebToolConfigFromRun(run)
	if err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, nil
	}

	definitions := make([]ADKRuntimeToolDefinition, 0, 2)
	if config.HTTP.Enabled {
		definitions = append(definitions, ADKRuntimeToolDefinition{
			Name:        adkWebFetchToolName,
			Description: "Fetch bounded text content from an explicitly allowed web host.",
			InputSchema: adkWebFetchInputSchema,
			Visibility:  config.Visibility,
			Invoker: &adkWebFetchInvoker{
				config: config.HTTP,
				client: c.options.HTTPClient,
			},
		})
	}
	if config.Search.Enabled {
		if c.options.SearchBackend == nil {
			return nil, fmt.Errorf("web search backend is required")
		}
		definitions = append(definitions, ADKRuntimeToolDefinition{
			Name:        adkWebSearchToolName,
			Description: "Search the web using a Coze-approved search backend.",
			InputSchema: adkWebSearchInputSchema,
			Visibility:  config.Visibility,
			Invoker: &adkWebSearchInvoker{
				config:  config.Search,
				backend: c.options.SearchBackend,
			},
		})
	}

	return definitions, nil
}

type adkWebToolConfig struct {
	Enabled    bool
	Visibility ADKRuntimeToolVisibility
	HTTP       adkWebHTTPConfig
	Search     adkWebSearchConfig
}

type adkWebHTTPConfig struct {
	Enabled          bool
	AllowedHosts     []string
	AllowHTTP        bool
	AllowPrivateIPs  bool
	Timeout          time.Duration
	MaxResponseBytes int64
}

type adkWebSearchConfig struct {
	Enabled    bool
	MaxResults int
}

func adkWebToolConfigFromRun(run *RunSummary) (adkWebToolConfig, error) {
	config := adkWebToolConfig{
		Visibility: ADKRuntimeToolVisibilityStatic,
		HTTP: adkWebHTTPConfig{
			Timeout:          defaultADKWebFetchTimeout,
			MaxResponseBytes: defaultADKWebFetchResponseBytes,
		},
		Search: adkWebSearchConfig{
			MaxResults: defaultADKWebSearchResults,
		},
	}
	if run == nil || strings.TrimSpace(run.Config) == "" {
		return config, nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(run.Config), &payload); err != nil {
		return config, fmt.Errorf("parse web tools config: %w", err)
	}
	raw, ok := firstADKConfigObject(payload, "web_tools", "webTools")
	if !ok {
		return config, nil
	}
	config.Enabled = firstADKConfigBool(raw, "enabled")
	if !config.Enabled {
		return config, nil
	}
	if visibility := firstConfigString(raw, "visibility"); visibility != "" {
		switch ADKRuntimeToolVisibility(visibility) {
		case ADKRuntimeToolVisibilityStatic, ADKRuntimeToolVisibilityDeferred:
			config.Visibility = ADKRuntimeToolVisibility(visibility)
		default:
			return config, fmt.Errorf("unsupported web tool visibility: %s", visibility)
		}
	}
	if httpPayload, ok := firstADKConfigObject(raw, "http", "fetch"); ok {
		config.HTTP.Enabled = firstADKConfigBool(httpPayload, "enabled")
		config.HTTP.AllowedHosts = normalizeADKWebAllowedHosts(
			configStringSlice(httpPayload["allowed_hosts"]),
		)
		if len(config.HTTP.AllowedHosts) == 0 {
			config.HTTP.AllowedHosts = normalizeADKWebAllowedHosts(
				configStringSlice(httpPayload["allowedHosts"]),
			)
		}
		config.HTTP.AllowHTTP = firstADKConfigBool(httpPayload, "allow_http", "allowHttp")
		config.HTTP.AllowPrivateIPs = firstADKConfigBool(
			httpPayload,
			"allow_private_ips",
			"allowPrivateIPs",
		)
		config.HTTP.Timeout = boundedDurationFromMillis(
			firstConfigInt64(httpPayload, "timeout_ms", "timeoutMs"),
			defaultADKWebFetchTimeout,
			time.Second,
			maxADKWebFetchTimeout,
		)
		config.HTTP.MaxResponseBytes = boundedInt64(
			firstConfigInt64(
				httpPayload,
				"max_response_bytes",
				"maxResponseBytes",
			),
			defaultADKWebFetchResponseBytes,
			minADKWebFetchResponseBytes,
			maxADKWebFetchResponseBytes,
		)
		if config.HTTP.Enabled && len(config.HTTP.AllowedHosts) == 0 {
			return config, fmt.Errorf("web fetch allowed_hosts is required")
		}
	}
	if searchPayload, ok := firstADKConfigObject(raw, "search"); ok {
		config.Search.Enabled = firstADKConfigBool(searchPayload, "enabled")
		config.Search.MaxResults = int(boundedInt64(
			firstConfigInt64(searchPayload, "max_results", "maxResults"),
			defaultADKWebSearchResults,
			1,
			maxADKWebSearchResults,
		))
	}

	return config, nil
}

type ADKWebFetchRequest struct {
	URL string `json:"url"`
}

type ADKWebFetchResponse struct {
	Schema      string `json:"schema"`
	Host        string `json:"host"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type,omitempty"`
	Truncated   bool   `json:"truncated"`
	Body        string `json:"body"`
}

type adkWebFetchInvoker struct {
	config adkWebHTTPConfig
	client *http.Client
}

func (i *adkWebFetchInvoker) InvokeADKRuntimeTool(
	ctx context.Context,
	call ADKRuntimeToolCall,
) (string, error) {
	if i == nil {
		return "", fmt.Errorf("web fetch invoker is required")
	}
	request := ADKWebFetchRequest{}
	if err := json.Unmarshal([]byte(call.Arguments), &request); err != nil {
		return "", fmt.Errorf("parse web fetch arguments: %w", err)
	}
	targetURL, err := i.config.validateURL(request.URL)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		targetURL.String(),
		nil,
	)
	if err != nil {
		return "", fmt.Errorf("create web fetch request: %w", err)
	}
	httpReq.Header.Set("User-Agent", "Coze-Agent-WebFetch/1.0")

	client := i.httpClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		if errors.Is(err, errADKWebRedirectTargetNotAllowed) {
			return "", fmt.Errorf(
				"execute web fetch request: redirect target is not allowed",
			)
		}
		return "", fmt.Errorf("execute web fetch request: %w", err)
	}
	defer resp.Body.Close()

	body, truncated, err := readBoundedADKWebBody(
		resp.Body,
		i.config.MaxResponseBytes,
	)
	if err != nil {
		return "", fmt.Errorf("read web fetch response: %w", err)
	}
	payload := ADKWebFetchResponse{
		Schema:      adkWebFetchSchema,
		Host:        targetURL.Hostname(),
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Truncated:   truncated,
		Body:        body,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal web fetch response: %w", err)
	}

	return string(encoded), nil
}

func (i *adkWebFetchInvoker) httpClient() *http.Client {
	base := i.client
	if base == nil {
		base = &http.Client{}
	}
	client := *base
	client.Timeout = i.config.Timeout
	if client.Transport == nil {
		client.Transport = http.DefaultTransport
	}
	client.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req == nil || req.URL == nil {
			return errADKWebRedirectTargetNotAllowed
		}
		if _, err := i.config.validateParsedURL(req.URL); err != nil {
			return fmt.Errorf("%w: %v", errADKWebRedirectTargetNotAllowed, err)
		}
		return nil
	}
	return &client
}

func (c adkWebHTTPConfig) validateURL(rawURL string) (*url.URL, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("web fetch url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("web fetch url is invalid")
	}
	return c.validateParsedURL(parsed)
}

func (c adkWebHTTPConfig) validateParsedURL(parsed *url.URL) (*url.URL, error) {
	if parsed == nil {
		return nil, fmt.Errorf("web fetch url is required")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	switch scheme {
	case "https":
	case "http":
		if !c.AllowHTTP {
			return nil, fmt.Errorf("web fetch http scheme is not allowed")
		}
	default:
		return nil, fmt.Errorf("web fetch scheme is not allowed")
	}
	host := normalizeADKWebHost(parsed.Hostname())
	if host == "" {
		return nil, fmt.Errorf("web fetch host is required")
	}
	if !c.hostAllowed(host) {
		return nil, fmt.Errorf("web fetch host is not allowed")
	}
	if err := c.validateHostAddress(host); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (c adkWebHTTPConfig) hostAllowed(host string) bool {
	host = normalizeADKWebHost(host)
	for _, allowed := range c.AllowedHosts {
		allowed = normalizeADKWebHost(allowed)
		if allowed == "" {
			continue
		}
		if strings.HasPrefix(allowed, "*.") {
			suffix := strings.TrimPrefix(allowed, "*")
			if strings.HasSuffix(host, suffix) && host != strings.TrimPrefix(suffix, ".") {
				return true
			}
			continue
		}
		if host == allowed {
			return true
		}
	}
	return false
}

func (c adkWebHTTPConfig) validateHostAddress(host string) error {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	if c.AllowPrivateIPs {
		return nil
	}
	if addr.IsPrivate() ||
		addr.IsLoopback() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified() {
		return fmt.Errorf("web fetch private ip is not allowed")
	}
	return nil
}

type ADKWebSearchRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

type ADKWebSearchResult struct {
	Title   string `json:"title,omitempty"`
	URL     string `json:"url,omitempty"`
	Snippet string `json:"snippet,omitempty"`
	Source  string `json:"source,omitempty"`
}

type ADKWebSearchResponse struct {
	Schema  string               `json:"schema"`
	Results []ADKWebSearchResult `json:"results"`
}

type ADKWebSearchBackend interface {
	SearchADKWeb(
		ctx context.Context,
		request ADKWebSearchRequest,
	) (*ADKWebSearchResponse, error)
}

type ADKWebSearchBackendFunc func(
	ctx context.Context,
	request ADKWebSearchRequest,
) (*ADKWebSearchResponse, error)

func (f ADKWebSearchBackendFunc) SearchADKWeb(
	ctx context.Context,
	request ADKWebSearchRequest,
) (*ADKWebSearchResponse, error) {
	if f == nil {
		return nil, fmt.Errorf("web search backend is required")
	}
	return f(ctx, request)
}

type adkWebSearchInvoker struct {
	config  adkWebSearchConfig
	backend ADKWebSearchBackend
}

func (i *adkWebSearchInvoker) InvokeADKRuntimeTool(
	ctx context.Context,
	call ADKRuntimeToolCall,
) (string, error) {
	if i == nil || i.backend == nil {
		return "", fmt.Errorf("web search backend is required")
	}
	request := ADKWebSearchRequest{}
	if err := json.Unmarshal([]byte(call.Arguments), &request); err != nil {
		return "", fmt.Errorf("parse web search arguments: %w", err)
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" {
		return "", fmt.Errorf("web search query is required")
	}
	request.MaxResults = boundedADKWebSearchResults(
		request.MaxResults,
		i.config.MaxResults,
	)

	response, err := i.backend.SearchADKWeb(ctx, request)
	if err != nil {
		return "", err
	}
	if response == nil {
		return "", fmt.Errorf("web search backend returned empty response")
	}
	response.Schema = adkWebSearchSchema
	response.Results = sanitizeADKWebSearchResults(
		response.Results,
		request.MaxResults,
	)
	encoded, err := json.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("marshal web search response: %w", err)
	}

	return string(encoded), nil
}

func boundedADKWebSearchResults(requested, configured int) int {
	if configured <= 0 {
		configured = defaultADKWebSearchResults
	}
	if configured > maxADKWebSearchResults {
		configured = maxADKWebSearchResults
	}
	if requested <= 0 || requested > configured {
		return configured
	}
	return requested
}

func sanitizeADKWebSearchResults(
	results []ADKWebSearchResult,
	limit int,
) []ADKWebSearchResult {
	if limit <= 0 || limit > len(results) {
		limit = len(results)
	}
	sanitized := make([]ADKWebSearchResult, 0, limit)
	for index := 0; index < limit; index++ {
		item := results[index]
		sanitized = append(sanitized, ADKWebSearchResult{
			Title:   trimADKWebText(item.Title, 512),
			URL:     trimADKWebText(item.URL, 2048),
			Snippet: trimADKWebText(item.Snippet, 2048),
			Source:  trimADKWebText(item.Source, 128),
		})
	}
	return sanitized
}

func readBoundedADKWebBody(reader io.Reader, maxBytes int64) (string, bool, error) {
	if reader == nil {
		return "", false, nil
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return "", false, err
	}
	truncated := int64(len(body)) > maxBytes
	if truncated {
		body = body[:maxBytes]
	}
	return string(body), truncated, nil
}

func firstADKConfigObject(payload map[string]any, keys ...string) (map[string]any, bool) {
	for _, key := range keys {
		if parsed, ok := configObject(payload[key]); ok {
			return parsed, true
		}
	}
	return nil, false
}

func configStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				items = append(items, text)
			}
		}
		return items
	case string:
		if strings.TrimSpace(typed) != "" {
			return []string{typed}
		}
	}
	return nil
}

func normalizeADKWebAllowedHosts(hosts []string) []string {
	normalized := make([]string, 0, len(hosts))
	seen := map[string]struct{}{}
	for _, host := range hosts {
		host = normalizeADKWebHost(host)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		normalized = append(normalized, host)
	}
	return normalized
}

func normalizeADKWebHost(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	host = strings.TrimSuffix(host, ".")
	if strings.HasPrefix(host, "[") && strings.Contains(host, "]") {
		if parsed, _, err := net.SplitHostPort(host); err == nil {
			host = parsed
		}
	}
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	}
	return host
}

func boundedDurationFromMillis(
	value int64,
	defaultValue time.Duration,
	minValue time.Duration,
	maxValue time.Duration,
) time.Duration {
	if value <= 0 {
		return defaultValue
	}
	parsed := time.Duration(value) * time.Millisecond
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func boundedInt64(value, defaultValue, minValue, maxValue int64) int64 {
	if value <= 0 {
		return defaultValue
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func trimADKWebText(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit]
}
