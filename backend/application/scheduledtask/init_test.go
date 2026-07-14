// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package scheduledtask

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	workflowentity "github.com/coze-dev/coze-studio/backend/domain/workflow/entity"
	workflowvo "github.com/coze-dev/coze-studio/backend/domain/workflow/entity/vo"
)

func TestInitServiceBuildsProductionDependencies(t *testing.T) {
	t.Parallel()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	workflowDomain := &workflowDomainStub{}

	service, worker, err := InitService(&ServiceComponents{
		DB:                db,
		IDGen:             &applicationIDGen{},
		UserSpaceReader:   membershipAuthorizer(true),
		AgentThreadClient: &agentThreadClientStub{},
		WorkflowDomain:    workflowDomain,
		RootContext:       context.Background(),
	})

	require.NoError(t, err)
	require.NotNil(t, service.Repository)
	require.NotNil(t, service.Targets)
	require.NotNil(t, service.Dispatcher)
	require.Same(t, service.Repository, worker.Repository)
	require.Same(t, service.Dispatcher, worker.Dispatcher)
}

func (s *workflowDomainStub) MGet(context.Context, *workflowvo.MGetPolicy) ([]*workflowentity.Workflow, int64, error) {
	if s.workflow == nil { return nil, 0, nil }
	return []*workflowentity.Workflow{s.workflow}, 1, nil
}

type applicationIDGen struct{ next int64 }

func (g *applicationIDGen) GenID(context.Context) (int64, error) { g.next++; return g.next, nil }
func (g *applicationIDGen) GenMultiIDs(_ context.Context, count int) ([]int64, error) {
	ids := make([]int64, count)
	for i := range ids { g.next++; ids[i] = g.next }
	return ids, nil
}
