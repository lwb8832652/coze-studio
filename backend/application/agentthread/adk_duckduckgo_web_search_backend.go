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
	"strings"
	"time"

	duckduckgo "github.com/cloudwego/eino-ext/components/tool/duckduckgo/v2"
)

const (
	defaultADKDuckDuckGoSource     = "duckduckgo"
	defaultADKDuckDuckGoRegion     = "wt-wt"
	defaultADKDuckDuckGoSafeSearch = "moderate"
	defaultADKDuckDuckGoTimeout    = 10 * time.Second
)

type ADKDuckDuckGoWebSearchBackendOptions struct {
	Region     string
	SafeSearch string
	Timeout    time.Duration
	Source     string
	Searcher   adkDuckDuckGoSearcher
}

type ADKDuckDuckGoWebSearchBackend struct {
	searcher   adkDuckDuckGoSearcher
	region     string
	safeSearch string
	source     string
}

type adkDuckDuckGoSearchRequest struct {
	Query      string
	MaxResults int
	Region     string
	SafeSearch string
}

type adkDuckDuckGoSearchResult struct {
	Title       string
	URL         string
	Description string
}

type adkDuckDuckGoSearcher interface {
	SearchADKDuckDuckGo(
		ctx context.Context,
		request adkDuckDuckGoSearchRequest,
	) ([]adkDuckDuckGoSearchResult, error)
}

type adkDuckDuckGoEinoSearcher struct {
	client duckduckgo.Search
}

func NewADKDuckDuckGoWebSearchBackend(
	options ADKDuckDuckGoWebSearchBackendOptions,
) (*ADKDuckDuckGoWebSearchBackend, error) {
	region := normalizeADKDuckDuckGoRegion(options.Region)
	safeSearch, err := normalizeADKDuckDuckGoSafeSearch(options.SafeSearch)
	if err != nil {
		return nil, err
	}
	source := trimADKWebText(options.Source, 128)
	if source == "" {
		source = defaultADKDuckDuckGoSource
	}
	searcher := options.Searcher
	if searcher == nil {
		searcher, err = newADKDuckDuckGoEinoSearcher(options)
		if err != nil {
			return nil, err
		}
	}

	return &ADKDuckDuckGoWebSearchBackend{
		searcher:   searcher,
		region:     region,
		safeSearch: safeSearch,
		source:     source,
	}, nil
}

func newADKDuckDuckGoEinoSearcher(
	options ADKDuckDuckGoWebSearchBackendOptions,
) (*adkDuckDuckGoEinoSearcher, error) {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultADKDuckDuckGoTimeout
	}
	client, err := duckduckgo.NewSearch(context.Background(), &duckduckgo.Config{
		Timeout:    timeout,
		MaxResults: maxADKWebSearchResults,
		Region:     duckduckgo.Region(normalizeADKDuckDuckGoRegion(options.Region)),
	})
	if err != nil {
		return nil, fmt.Errorf("create duckduckgo web search backend: %w", err)
	}

	return &adkDuckDuckGoEinoSearcher{client: client}, nil
}

func (b *ADKDuckDuckGoWebSearchBackend) SearchADKWeb(
	ctx context.Context,
	request ADKWebSearchRequest,
) (*ADKWebSearchResponse, error) {
	if b == nil || b.searcher == nil {
		return nil, fmt.Errorf("duckduckgo web search backend is not configured")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, fmt.Errorf("web search query is required")
	}
	maxResults := boundedADKHTTPWebSearchRequestResults(request.MaxResults)
	results, err := b.searcher.SearchADKDuckDuckGo(
		ctx,
		adkDuckDuckGoSearchRequest{
			Query:      query,
			MaxResults: maxResults,
			Region:     b.region,
			SafeSearch: b.safeSearch,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo web search request failed")
	}
	normalized := make([]ADKWebSearchResult, 0, len(results))
	for _, result := range results {
		normalized = append(normalized, ADKWebSearchResult{
			Title:   result.Title,
			URL:     result.URL,
			Snippet: result.Description,
			Source:  b.source,
		})
	}

	return &ADKWebSearchResponse{
		Schema:  adkWebSearchSchema,
		Results: sanitizeADKWebSearchResults(normalized, maxResults),
	}, nil
}

func (s *adkDuckDuckGoEinoSearcher) SearchADKDuckDuckGo(
	ctx context.Context,
	request adkDuckDuckGoSearchRequest,
) ([]adkDuckDuckGoSearchResult, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("duckduckgo searcher is not configured")
	}
	response, err := s.client.TextSearch(ctx, &duckduckgo.TextSearchRequest{
		Query:     request.Query,
		TimeRange: duckduckgo.TimeRangeAny,
	})
	if err != nil {
		return nil, err
	}
	results := make([]adkDuckDuckGoSearchResult, 0, len(response.Results))
	for _, result := range response.Results {
		if result == nil {
			continue
		}
		results = append(results, adkDuckDuckGoSearchResult{
			Title:       result.Title,
			URL:         result.URL,
			Description: result.Summary,
		})
	}

	return results, nil
}

func normalizeADKDuckDuckGoRegion(region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return defaultADKDuckDuckGoRegion
	}

	return region
}

func normalizeADKDuckDuckGoSafeSearch(safeSearch string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(safeSearch)) {
	case "", "moderate":
		return defaultADKDuckDuckGoSafeSearch, nil
	case "on", "strict":
		return "strict", nil
	case "off":
		return "off", nil
	default:
		return "", fmt.Errorf("duckduckgo safesearch is invalid")
	}
}
