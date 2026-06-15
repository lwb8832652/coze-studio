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
	"strconv"
)

const defaultThreadMemoryProviderLimit int32 = 8

type ThreadMemoryProvider struct {
	app   *ApplicationService
	limit int32
}

func NewThreadMemoryProvider(app *ApplicationService, limit int32) *ThreadMemoryProvider {
	if limit <= 0 {
		limit = defaultThreadMemoryProviderLimit
	}

	return &ThreadMemoryProvider{
		app:   app,
		limit: limit,
	}
}

func (p *ThreadMemoryProvider) Recall(ctx context.Context, run *RunSummary) ([]AgentMemory, error) {
	if p == nil || p.app == nil {
		return nil, fmt.Errorf("agent thread memory provider application service is required")
	}
	if run == nil {
		return nil, fmt.Errorf("run is required")
	}
	if run.ThreadID <= 0 {
		return nil, fmt.Errorf("run thread id is required")
	}

	resp, err := p.app.RecallMemories(ctx, &RecallMemoriesRequest{
		ThreadID: run.ThreadID,
		RunID:    run.RunID,
		Limit:    p.limit,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Memories) == 0 {
		return nil, nil
	}

	memories := make([]AgentMemory, 0, len(resp.Memories))
	for _, memory := range resp.Memories {
		if memory == nil {
			continue
		}
		memories = append(memories, AgentMemory{
			ID:       strconv.FormatInt(memory.MemoryID, 10),
			Scope:    string(memory.Scope),
			Content:  memory.Content,
			Metadata: memory.Metadata,
			Score:    memory.Score,
		})
	}

	return memories, nil
}
