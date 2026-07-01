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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	defaultADKBraveSearchBaseURL    = "https://search.brave.com/search"
	defaultADKBraveSearchSource     = "brave"
	defaultADKBraveSearchTimeout    = 10 * time.Second
	defaultADKBraveSearchUserAgent  = "Mozilla/5.0 (compatible; Coze-Agent-WebSearch/1.0)"
	maxADKBraveSearchResponseBytes  = 2 << 20
	defaultADKBraveSearchRegion     = defaultADKDuckDuckGoRegion
	defaultADKBraveSearchSafeSearch = defaultADKDuckDuckGoSafeSearch
)

type ADKBraveWebSearchBackendOptions struct {
	BaseURL         string
	Region          string
	SafeSearch      string
	Timeout         time.Duration
	AllowHTTP       bool
	AllowPrivateIPs bool
	Source          string
	Client          *http.Client
}

type ADKBraveWebSearchBackend struct {
	baseURL    string
	region     string
	safeSearch string
	source     string
	client     *http.Client
}

func NewADKBraveWebSearchBackend(
	options ADKBraveWebSearchBackendOptions,
) (*ADKBraveWebSearchBackend, error) {
	rawBaseURL := strings.TrimSpace(options.BaseURL)
	if rawBaseURL == "" {
		rawBaseURL = defaultADKBraveSearchBaseURL
	}
	parsed, err := validateADKWebSearchEndpoint(
		rawBaseURL,
		options.AllowHTTP,
		options.AllowPrivateIPs,
	)
	if err != nil {
		return nil, err
	}
	safeSearch, err := normalizeADKDuckDuckGoSafeSearch(options.SafeSearch)
	if err != nil {
		return nil, err
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultADKBraveSearchTimeout
	}
	source := trimADKWebText(options.Source, 128)
	if source == "" {
		source = defaultADKBraveSearchSource
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
		host:            normalizeADKWebHost(parsed.Hostname()),
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

	return &ADKBraveWebSearchBackend{
		baseURL:    parsed.String(),
		region:     normalizeADKDuckDuckGoRegion(options.Region),
		safeSearch: safeSearch,
		source:     source,
		client:     &cloned,
	}, nil
}

func (b *ADKBraveWebSearchBackend) SearchADKWeb(
	ctx context.Context,
	request ADKWebSearchRequest,
) (*ADKWebSearchResponse, error) {
	if b == nil || b.client == nil || strings.TrimSpace(b.baseURL) == "" {
		return nil, fmt.Errorf("brave web search backend is not configured")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, fmt.Errorf("web search query is required")
	}
	limit := boundedADKHTTPWebSearchRequestResults(request.MaxResults)
	searchURL, err := buildADKBraveSearchURL(b.baseURL, query)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("brave web search request is invalid")
	}
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml")
	httpReq.Header.Set("User-Agent", defaultADKBraveSearchUserAgent)
	addADKBraveSearchCookies(httpReq, b.region, b.safeSearch)

	resp, err := b.client.Do(httpReq)
	if err != nil {
		if strings.Contains(err.Error(), errADKWebSearchRedirectTargetNotAllowed.Error()) {
			return nil, fmt.Errorf("brave web search redirect target is not allowed")
		}
		return nil, fmt.Errorf("brave web search request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("brave web search returned non-2xx status")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxADKBraveSearchResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("brave web search response read failed")
	}
	if len(body) > maxADKBraveSearchResponseBytes {
		return nil, fmt.Errorf("brave web search response is too large")
	}
	results, err := parseADKBraveSearchResults(string(body), limit, b.source)
	if err != nil {
		return nil, err
	}

	return &ADKWebSearchResponse{
		Schema:  adkWebSearchSchema,
		Results: results,
	}, nil
}

func buildADKBraveSearchURL(baseURL string, query string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("brave web search base url is invalid")
	}
	values := parsed.Query()
	values.Set("q", query)
	values.Set("source", "web")
	parsed.RawQuery = values.Encode()

	return parsed.String(), nil
}

func addADKBraveSearchCookies(
	request *http.Request,
	region string,
	safeSearch string,
) {
	country := "us"
	if parts := strings.SplitN(strings.ToLower(strings.TrimSpace(region)), "-", 2); len(parts) > 0 && parts[0] != "" && parts[0] != "wt" {
		country = parts[0]
	}
	request.AddCookie(&http.Cookie{Name: country, Value: country})
	request.AddCookie(&http.Cookie{Name: "useLocation", Value: "0"})
	switch safeSearch {
	case "strict":
		request.AddCookie(&http.Cookie{Name: "safesearch", Value: "strict"})
	case "off":
		request.AddCookie(&http.Cookie{Name: "safesearch", Value: "off"})
	}
}

func parseADKBraveSearchResults(
	body string,
	limit int,
	source string,
) ([]ADKWebSearchResult, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("brave web search response is invalid")
	}
	results := make([]ADKWebSearchResult, 0, limit)
	seen := map[string]struct{}{}
	doc.Find("div[data-type='web']").EachWithBreak(
		func(_ int, item *goquery.Selection) bool {
			titleLink := item.Find("a").FilterFunction(
				func(_ int, candidate *goquery.Selection) bool {
					return candidate.Find("div.title").Length() > 0
				},
			).First()
			if titleLink.Length() == 0 {
				titleLink = item.Find("a[href^='http']").First()
			}
			href, _ := titleLink.Attr("href")
			href = strings.TrimSpace(href)
			if href == "" {
				return true
			}
			if _, ok := seen[href]; ok {
				return true
			}
			title := compactADKBraveText(titleLink.Find("div.title").Last().Text())
			if title == "" {
				title = compactADKBraveText(titleLink.Text())
			}
			if title == "" {
				return true
			}
			snippet := compactADKBraveText(
				item.Find("div.snippet div.content").First().Text(),
			)
			seen[href] = struct{}{}
			results = append(results, ADKWebSearchResult{
				Title:   title,
				URL:     href,
				Snippet: snippet,
				Source:  source,
			})

			return len(results) < limit
		},
	)

	return sanitizeADKWebSearchResults(results, limit), nil
}

func compactADKBraveText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
