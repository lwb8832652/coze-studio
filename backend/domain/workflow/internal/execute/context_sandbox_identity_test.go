/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

package execute

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	workflowModel "github.com/coze-dev/coze-studio/backend/crossdomain/workflow/model"
	"github.com/coze-dev/coze-studio/backend/domain/workflow/entity"
	"github.com/coze-dev/coze-studio/backend/pkg/sandboxidentity"
)

func TestSandboxIdentityContextUsesOnlyRootWorkflowExecutionFacts(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey{}, &Context{RootCtx: RootCtx{
		RootWorkflowBasic: &entity.WorkflowBasic{SpaceID: 101},
		RootExecuteID:     303,
		ExeCfg:            workflowModel.ExecuteConfig{Operator: 202},
	}})
	identity, ok := sandboxidentity.RequestFromContext(SandboxIdentityContext(ctx))
	require.True(t, ok)
	require.Equal(t, sandboxidentity.Request{
		Scope: sandboxidentity.ScopeAgent, SpaceID: 101, UserID: 202, ExecutionID: "workflow-303",
	}, identity)
}

func TestSandboxIdentityContextPreservesLegacyContextWhenRootFactsAreIncomplete(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey{}, &Context{RootCtx: RootCtx{
		RootWorkflowBasic: &entity.WorkflowBasic{SpaceID: 101},
		RootExecuteID:     303,
	}})
	_, ok := sandboxidentity.RequestFromContext(SandboxIdentityContext(ctx))
	require.False(t, ok)
}
