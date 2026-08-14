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
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	adminconfig "github.com/coze-dev/coze-studio/backend/api/model/admin/config"
)

func TestJournalIntegrationEnrollsProjectsAndServesAuthorizedDefaultRun(t *testing.T) {
	app := newJournalIntegrationApplication(t)
	ctx := context.Background()

	thread, err := app.CreateThread(ctx, &CreateThreadRequest{
		SpaceID: 9, UserID: 2, Title: "Journal integration",
	})
	require.NoError(t, err)
	run, err := app.CreateRun(ctx, &CreateRunRequest{
		ThreadID: thread.Thread.ThreadID,
		Status:   RunStatusQueued,
		Input:    `{"messages":[{"role":"user","content":"核验 Journal"}]}`,
		Config:   `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)

	_, err = app.AppendRunEvent(ctx, &AppendRunEventRequest{
		ThreadID:  thread.Thread.ThreadID,
		RunID:     run.Run.RunID,
		EventType: "plan.task.updated",
		Payload: `{"plan_task_id":"plan-1","subject":"核验事件链路",` +
			`"status":"in_progress","active_form":"正在核验事件链路"}`,
	})
	require.NoError(t, err)
	_, err = app.AppendRunEvent(ctx, &AppendRunEventRequest{
		ThreadID:  thread.Thread.ThreadID,
		RunID:     run.Run.RunID,
		EventType: "tool.completed",
		Payload: `{"tool_name":"read_file","tool_call_id":"call-1",` +
			`"result":"/private/customer.md token=secret-value"}`,
	})
	require.NoError(t, err)

	bootstrap, err := app.GetJournalBootstrap(ctx, GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 9, ThreadID: thread.Thread.ThreadID,
		RunID: run.Run.RunID, Limit: 20,
	})
	require.NoError(t, err)
	require.True(t, bootstrap.JournalEnabled)
	require.False(t, bootstrap.SnapshotsEnabled)
	require.Equal(t, []JournalContentType{
		JournalContentTypeDocument,
		JournalContentTypeTerminal,
		JournalContentTypeCode,
		JournalContentTypeSkill,
		JournalContentTypeBrowser,
	}, bootstrap.ContentTypes)
	require.Len(t, bootstrap.Events, 2)
	require.Equal(t, uint64(1), bootstrap.Events[0].Sequence)
	require.Equal(t, "milestone.started", bootstrap.Events[0].EventType)
	require.Equal(t, uint64(2), bootstrap.Events[1].Sequence)
	require.Equal(t, "action.terminal", bootstrap.Events[1].EventType)
	require.Equal(t, "read", bootstrap.Events[1].Operation)
	require.NotContains(t, bootstrap.Events[1].Payload, "/private/customer.md")
	require.NotContains(t, bootstrap.Events[1].Payload, "secret-value")
}

func TestJournalIntegrationKeepsAutomaticRunEmptyBeforePublicEvents(t *testing.T) {
	app := newJournalIntegrationApplication(t)
	ctx := context.Background()

	thread, err := app.CreateThread(ctx, &CreateThreadRequest{
		SpaceID: 9, UserID: 2, Title: "Automatic integration",
	})
	require.NoError(t, err)
	run, err := app.CreateRun(ctx, &CreateRunRequest{
		ThreadID: thread.Thread.ThreadID,
		Status:   RunStatusQueued,
		Input:    `{"messages":[{"role":"user","content":"直接回答"}]}`,
		Config:   `{"runtime":"eino_adk"}`,
	})
	require.NoError(t, err)

	bootstrap, err := app.GetJournalBootstrap(ctx, GetJournalBootstrapRequest{
		ViewerID: 2, SpaceID: 9, ThreadID: thread.Thread.ThreadID,
		RunID: run.Run.RunID, Limit: 20,
	})
	require.NoError(t, err)
	require.True(t, bootstrap.JournalEnabled)
	require.Empty(t, bootstrap.Events)
}

func newJournalIntegrationApplication(t *testing.T) *ApplicationService {
	t.Helper()
	dsn := fmt.Sprintf("file:journal-integration-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, migrateAgentThreadTableForTest(db))

	app := InitService(&ServiceComponents{
		DB: db, IDGen: &journalIntegrationIDGenerator{next: 100},
	})
	configuration := enabledJournalConfiguration()
	configuration.JournalRuntimeConfiguration.JournalSnapshots = false
	configuration.JournalRuntimeConfiguration.JournalSnapshotsRolloutBasisPoints = 0
	configuration.JournalRuntimeConfiguration.CheckpointRecovery = false
	configuration.JournalRuntimeConfiguration.CheckpointRecoveryRolloutBasisPoints = 0
	app.JournalFeatureGate = NewJournalFeatureGate(
		&journalIntegrationConfigProvider{configuration: configuration},
		JournalFeatureGateOptions{},
	)
	app.WorkspaceAuthorizer = &recordingWorkspaceAuthorizer{}
	return app
}

type journalIntegrationConfigProvider struct {
	configuration *adminconfig.BasicConfiguration
}

func (p *journalIntegrationConfigProvider) GetBaseConfig(
	context.Context,
) (*adminconfig.BasicConfiguration, error) {
	return p.configuration, nil
}

type journalIntegrationIDGenerator struct {
	mu   sync.Mutex
	next int64
}

func (g *journalIntegrationIDGenerator) GenID(context.Context) (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := g.next
	g.next++
	return id, nil
}

func (g *journalIntegrationIDGenerator) GenMultiIDs(
	_ context.Context,
	count int,
) ([]int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	ids := make([]int64, count)
	for i := range ids {
		ids[i] = g.next
		g.next++
	}
	return ids, nil
}
