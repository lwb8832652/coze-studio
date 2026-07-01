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
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino-ext/components/tool/wikipedia"
	einotool "github.com/cloudwego/eino/components/tool"
)

const (
	defaultADKWikipediaSource      = "wikipedia"
	defaultADKWikipediaLanguage    = "en"
	defaultADKWikipediaTimeout     = 15 * time.Second
	defaultADKWikipediaDocMaxChars = 2000
)

type ADKWikipediaWebSearchBackendOptions struct {
	BaseURL     string
	Language    string
	UserAgent   string
	DocMaxChars int
	Timeout     time.Duration
	Source      string
	Searcher    adkWikipediaSearcher
}

type ADKWikipediaWebSearchBackend struct {
	searcher adkWikipediaSearcher
	language string
	source   string
}

type adkWikipediaSearchRequest struct {
	Query      string
	MaxResults int
	Language   string
}

type adkWikipediaSearchResult struct {
	Title   string
	URL     string
	Snippet string
	Extract string
}

type adkWikipediaSearcher interface {
	SearchADKWikipedia(
		ctx context.Context,
		request adkWikipediaSearchRequest,
	) ([]adkWikipediaSearchResult, error)
}

type adkWikipediaEinoSearcher struct {
	client einotool.InvokableTool
}

func NewADKWikipediaWebSearchBackend(
	options ADKWikipediaWebSearchBackendOptions,
) (*ADKWikipediaWebSearchBackend, error) {
	language := normalizeADKWikipediaLanguage(options.Language)
	source := trimADKWebText(options.Source, 128)
	if source == "" {
		source = defaultADKWikipediaSource
	}
	searcher := options.Searcher
	var err error
	if searcher == nil {
		searcher, err = newADKWikipediaEinoSearcher(options)
		if err != nil {
			return nil, err
		}
	}

	return &ADKWikipediaWebSearchBackend{
		searcher: searcher,
		language: language,
		source:   source,
	}, nil
}

func newADKWikipediaEinoSearcher(
	options ADKWikipediaWebSearchBackendOptions,
) (*adkWikipediaEinoSearcher, error) {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultADKWikipediaTimeout
	}
	docMaxChars := options.DocMaxChars
	if docMaxChars <= 0 {
		docMaxChars = defaultADKWikipediaDocMaxChars
	}
	client, err := wikipedia.NewTool(context.Background(), &wikipedia.Config{
		BaseURL:     strings.TrimSpace(options.BaseURL),
		UserAgent:   strings.TrimSpace(options.UserAgent),
		DocMaxChars: docMaxChars,
		Timeout:     timeout,
		TopK:        maxADKWebSearchResults,
		Language:    normalizeADKWikipediaLanguage(options.Language),
		ToolName:    "wikipedia_search",
	})
	if err != nil {
		return nil, fmt.Errorf("create wikipedia web search backend: %w", err)
	}

	return &adkWikipediaEinoSearcher{client: client}, nil
}

func (b *ADKWikipediaWebSearchBackend) SearchADKWeb(
	ctx context.Context,
	request ADKWebSearchRequest,
) (*ADKWebSearchResponse, error) {
	if b == nil || b.searcher == nil {
		return nil, fmt.Errorf("wikipedia web search backend is not configured")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return nil, fmt.Errorf("web search query is required")
	}
	maxResults := boundedADKHTTPWebSearchRequestResults(request.MaxResults)
	results, err := b.searcher.SearchADKWikipedia(
		ctx,
		adkWikipediaSearchRequest{
			Query:      query,
			MaxResults: maxResults,
			Language:   b.language,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("wikipedia web search request failed")
	}
	normalized := make([]ADKWebSearchResult, 0, len(results))
	for _, result := range results {
		normalized = append(normalized, ADKWebSearchResult{
			Title:   result.Title,
			URL:     result.URL,
			Snippet: mergeADKWikipediaSnippet(result.Snippet, result.Extract),
			Source:  b.source,
		})
	}

	return &ADKWebSearchResponse{
		Schema:  adkWebSearchSchema,
		Results: sanitizeADKWebSearchResults(normalized, maxResults),
	}, nil
}

func (s *adkWikipediaEinoSearcher) SearchADKWikipedia(
	ctx context.Context,
	request adkWikipediaSearchRequest,
) ([]adkWikipediaSearchResult, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("wikipedia searcher is not configured")
	}
	arguments, err := json.Marshal(wikipedia.SearchRequest{Query: request.Query})
	if err != nil {
		return nil, err
	}
	rawResponse, err := s.client.InvokableRun(ctx, string(arguments))
	if err != nil {
		return nil, err
	}
	var response wikipedia.SearchResponse
	if err := json.Unmarshal([]byte(rawResponse), &response); err != nil {
		return nil, err
	}
	results := make([]adkWikipediaSearchResult, 0, len(response.Results))
	for _, result := range response.Results {
		if result == nil {
			continue
		}
		results = append(results, adkWikipediaSearchResult{
			Title:   result.Title,
			URL:     result.URL,
			Snippet: result.Snippet,
			Extract: result.Extract,
		})
	}

	return results, nil
}

func normalizeADKWikipediaLanguage(language string) string {
	language = strings.TrimSpace(language)
	if language == "" {
		return defaultADKWikipediaLanguage
	}

	return language
}

func mergeADKWikipediaSnippet(snippet string, extract string) string {
	snippet = strings.TrimSpace(snippet)
	extract = strings.TrimSpace(extract)
	if snippet == "" {
		return extract
	}
	if extract == "" || extract == snippet {
		return snippet
	}

	return snippet + "\n\n" + extract
}
