// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"errors"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/route"

	handler "github.com/coze-dev/coze-studio/backend/api/handler/coze"
)

var errAdminAppDevProviderExecutionRoute = errors.New("admin appdev provider execution route is invalid")

func RegisterAdminAppDevProviderExecutionRoutes(r *server.Hertz) error {
	if r == nil {
		return errAdminAppDevProviderExecutionRoute
	}
	admin := r.Group("/api/admin", _adminMw()...)
	path := "/app-dev/provider-executions/:space_id/:project_id/:generation/quarantine-disposition"
	admin.POST(path, handler.DisposeAdminAppDevProviderExecutionQuarantine)
	admin.POST(path+"/", handler.RejectAdminAppDevProviderExecutionTrailingSlash)
	return nil
}

func registerAdminSandboxTrailingSlashRoutes(admin *route.RouterGroup) {
	admin.GET("/sandboxes/", handler.RejectAdminSandboxTrailingSlash)
	admin.POST("/sandboxes/", handler.RejectAdminSandboxTrailingSlash)
	admin.GET("/sandboxes/defaults/", handler.RejectAdminSandboxTrailingSlash)
	admin.GET("/sandboxes/summary/", handler.RejectAdminSandboxTrailingSlash)
	admin.GET("/sandboxes/capabilities/", handler.RejectAdminSandboxTrailingSlash)
	admin.GET("/sandboxes/scheduler-settings/", handler.RejectAdminSandboxTrailingSlash)
	admin.PUT("/sandboxes/scheduler-settings/", handler.RejectAdminSandboxTrailingSlash)
	admin.GET("/sandboxes/runtime-status/", handler.RejectAdminSandboxTrailingSlash)
	admin.GET("/sandboxes/:id/", handler.RejectAdminSandboxTrailingSlash)
	admin.PUT("/sandboxes/:id/", handler.RejectAdminSandboxTrailingSlash)
	admin.DELETE("/sandboxes/:id/", handler.RejectAdminSandboxTrailingSlash)
	admin.POST("/sandboxes/:id/enable/", handler.RejectAdminSandboxTrailingSlash)
	admin.POST("/sandboxes/:id/disable/", handler.RejectAdminSandboxTrailingSlash)
	admin.POST("/sandboxes/:id/credentials/", handler.RejectAdminSandboxTrailingSlash)
	admin.POST("/sandboxes/:id/defaults/", handler.RejectAdminSandboxTrailingSlash)
	admin.POST("/sandboxes/:id/health/", handler.RejectAdminSandboxTrailingSlash)
	admin.GET("/sandboxes/:id/audit-events/", handler.RejectAdminSandboxTrailingSlash)
}
