// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterIncludesScheduledTaskCenterRoutes(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), "api.go", nil, 0)
	require.NoError(t, err)

	type route struct {
		receiver string
		method   string
		path     string
		handler  string
	}

	registered := make(map[route]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		switch selector.Sel.Name {
		case "GET", "POST", "PUT", "DELETE":
		default:
			return true
		}
		pathLiteral, ok := call.Args[0].(*ast.BasicLit)
		if !ok || pathLiteral.Kind != token.STRING {
			return true
		}
		path, err := strconv.Unquote(pathLiteral.Value)
		if err != nil {
			return true
		}

		handler := ""
		for _, argument := range call.Args[1:] {
			ast.Inspect(argument, func(candidate ast.Node) bool {
				handlerSelector, ok := candidate.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := handlerSelector.X.(*ast.Ident)
				if ok && pkg.Name == "coze" {
					handler = handlerSelector.Sel.Name
				}
				return true
			})
		}
		if handler != "" {
			registered[route{
				receiver: receiver.Name,
				method:   selector.Sel.Name,
				path:     path,
				handler:  handler,
			}] = struct{}{}
		}
		return true
	})

	for _, expected := range []route{
		{receiver: "_workbench", method: "GET", path: "/scheduled_tasks", handler: "ListScheduledTasks"},
		{receiver: "_workbench", method: "POST", path: "/scheduled_tasks", handler: "CreateScheduledTask"},
		{receiver: "_scheduled_tasks", method: "GET", path: "/:task_id", handler: "GetScheduledTask"},
		{receiver: "_scheduled_tasks", method: "PUT", path: "/:task_id", handler: "UpdateScheduledTask"},
		{receiver: "_scheduled_tasks", method: "DELETE", path: "/:task_id", handler: "DeleteScheduledTask"},
		{receiver: "_task_id", method: "POST", path: "/enable", handler: "EnableScheduledTask"},
		{receiver: "_task_id", method: "POST", path: "/disable", handler: "DisableScheduledTask"},
		{receiver: "_task_id", method: "POST", path: "/execute", handler: "ExecuteScheduledTask"},
		{receiver: "_task_id", method: "GET", path: "/executions", handler: "ListScheduledTaskExecutions"},
		{receiver: "_workbench", method: "GET", path: "/scheduled_task_targets", handler: "ListScheduledTaskTargets"},
		{receiver: "_workbench", method: "GET", path: "/scheduled_task_cron_presets", handler: "ListScheduledTaskCronPresets"},
	} {
		_, ok := registered[expected]
		require.Truef(t, ok, "generated router is missing route %+v", expected)
	}
}
