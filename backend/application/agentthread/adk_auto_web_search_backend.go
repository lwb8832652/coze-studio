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
)

type ADKAutoWebSearchBackend struct {
	backends []ADKWebSearchBackend
}

func NewADKAutoWebSearchBackend(
	backends ...ADKWebSearchBackend,
) *ADKAutoWebSearchBackend {
	normalized := make([]ADKWebSearchBackend, 0, len(backends))
	for _, backend := range backends {
		if backend != nil {
			normalized = append(normalized, backend)
		}
	}

	return &ADKAutoWebSearchBackend{backends: normalized}
}

func (b *ADKAutoWebSearchBackend) SearchADKWeb(
	ctx context.Context,
	request ADKWebSearchRequest,
) (*ADKWebSearchResponse, error) {
	if b == nil || len(b.backends) == 0 {
		return nil, fmt.Errorf("web search auto backend is not configured")
	}
	var lastErr error
	for _, backend := range b.backends {
		response, err := backend.SearchADKWeb(ctx, request)
		if err != nil {
			lastErr = err
			continue
		}
		if response != nil && len(response.Results) > 0 {
			return response, nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("web search auto backends failed")
	}

	return &ADKWebSearchResponse{Schema: adkWebSearchSchema}, nil
}
