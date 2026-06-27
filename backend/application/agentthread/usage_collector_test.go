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
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/coze-dev/coze-studio/backend/domain/agentthread/entity"
	"github.com/coze-dev/coze-studio/backend/domain/agentthread/repository"
	domainservice "github.com/coze-dev/coze-studio/backend/domain/agentthread/service"
)

func TestThreadUsageCollectorRecordsApplicationUsage(t *testing.T) {
	domainSVC := &recordingThreadService{
		recordedTokenUsage: &entity.TokenUsage{
			ID:          400,
			ThreadID:    10,
			RunID:       20,
			Source:      entity.TokenUsageSourceLeadAgent,
			TotalTokens: 20,
		},
	}
	collector := NewThreadUsageCollector(&ApplicationService{ThreadSVC: domainSVC})

	err := collector.Record(context.Background(), &RunSummary{
		RunID:    20,
		ThreadID: 10,
	}, AgentTokenUsage{
		Source:       TokenUsageSourceLeadAgent,
		StepID:       "model-1",
		StepIndex:    0,
		StepName:     "generate_answer",
		ModelName:    "gpt-test",
		Provider:     "openai-compatible",
		InputTokens:  12,
		OutputTokens: 8,
		RawUsage:     `{"prompt_tokens":12,"completion_tokens":8}`,
		Metadata:     `{"source":"harness"}`,
	})

	require.NoError(t, err)
	require.NotNil(t, domainSVC.recordTokenUsageReq)
	require.Equal(t, int64(20), domainSVC.recordTokenUsageReq.RunID)
	require.Equal(t, entity.TokenUsageSourceLeadAgent, domainSVC.recordTokenUsageReq.Source)
	require.Equal(t, "model-1", domainSVC.recordTokenUsageReq.StepID)
	require.Equal(t, int64(12), domainSVC.recordTokenUsageReq.InputTokens)
	require.Equal(t, int64(8), domainSVC.recordTokenUsageReq.OutputTokens)
	require.Equal(t, `{"prompt_tokens":12,"completion_tokens":8}`, domainSVC.recordTokenUsageReq.RawUsage)
}

func TestADKUsageFailoverAndResumePersistEachProviderCallOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, migrateAgentThreadTableForTest(db))
	require.NoError(t, db.Exec(`
		CREATE TABLE agent_token_usage (
			id integer PRIMARY KEY,
			thread_id integer,
			run_id integer,
			space_id integer,
			source text,
			step_id text,
			step_index integer,
			step_name text,
			model_name text,
			provider text,
			input_tokens integer,
			output_tokens integer,
			total_tokens integer,
			cost_micros integer,
			currency text,
			estimated boolean,
			raw_usage json,
			metadata json,
			created_at integer
		);
		CREATE UNIQUE INDEX uk_agent_token_usage_idempotency
		ON agent_token_usage(run_id, json_extract(metadata, '$.idempotency_key'))
		WHERE json_extract(metadata, '$.idempotency_key') IS NOT NULL;
		INSERT INTO agent_runs (id, thread_id, space_id, status, created_at, updated_at)
		VALUES (20, 10, 1, 'running', 1, 1);
	`).Error)

	repo := repository.NewThreadRepository(db)
	domainSVC := domainservice.NewService(&domainservice.Components{
		Repo:  repo,
		IDGen: &usageSequenceIDGen{next: 100},
	})
	collector := NewThreadUsageCollector(&ApplicationService{ThreadSVC: domainSVC})
	run := &RunSummary{RunID: 20, ThreadID: 10, Config: `{"agent_name":"lead"}`}

	firstProcess := NewADKUsageBridge(run, collector)
	recordADKModelUsage(t, firstProcess, "lead", "logical-call-1", 0, "chat", "primary-provider", 10, 4, 0, 0)
	recordADKModelUsage(t, firstProcess, "lead", "logical-call-1", 0, "chat", "backup-provider", 12, 5, 0, 0)
	require.NoError(t, firstProcess.Err())

	resumedProcess := NewADKUsageBridge(run, collector)
	recordADKModelUsage(t, resumedProcess, "lead", "logical-call-1", 0, "chat", "backup-provider", 12, 5, 0, 0)
	require.NoError(t, resumedProcess.Err())

	var total int64
	require.NoError(t, db.Table("agent_token_usage").Count(&total).Error)
	require.Equal(t, int64(2), total)
}

type usageSequenceIDGen struct {
	mu   sync.Mutex
	next int64
}

func (g *usageSequenceIDGen) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return g.next, nil
}

func (g *usageSequenceIDGen) GenMultiIDs(ctx context.Context, counts int) ([]int64, error) {
	ids := make([]int64, counts)
	for i := range ids {
		id, err := g.GenID(ctx)
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}
