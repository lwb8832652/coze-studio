// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coze-dev/coze-studio/backend/infra/coderunner"
)

func TestBindCodeRunnerUsesExplicitComponent(t *testing.T) {
	previous := coderunner.GetCodeRunner()
	global := &initCodeRunnerStub{id: 1}
	injected := &initCodeRunnerStub{id: 2}
	coderunner.SetCodeRunner(global)
	t.Cleanup(func() { coderunner.SetCodeRunner(previous) })

	service := &PluginApplicationService{}
	bindCodeRunner(service, &ServiceComponents{CodeRunner: injected})

	require.Same(t, injected, service.codeRunner)
	require.NotSame(t, global, service.codeRunner)
}

type initCodeRunnerStub struct {
	id int
}

func (*initCodeRunnerStub) Run(
	context.Context,
	*coderunner.RunRequest,
) (*coderunner.RunResponse, error) {
	return &coderunner.RunResponse{}, nil
}
