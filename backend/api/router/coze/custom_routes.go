// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package coze

import (
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/route"

	handler "github.com/coze-dev/coze-studio/backend/api/handler/coze"
)

// RegisterCustomRoutes keeps hand-written HTTP contracts alongside generated
// IDL routes. Existing generated routes are intentionally not duplicated.
func RegisterCustomRoutes(r *server.Hertz) {
	root := r.Group("/", rootMw()...)
	api := root.Group("/api", _apiMw()...)

	registerAdminCustomRoutes(api)
	registerWorkbenchCustomRoutes(api)
	registerWorkspaceCustomRoutes(api)
	registerLangGraphCustomRoutes(api)
}

func registerAdminCustomRoutes(api *route.RouterGroup) {
	admin := api.Group("/admin", _adminMw()...)
	admin.GET("/auth/status", handler.GetAdminAuthStatus)
	admin.POST("/workspaces/list", handler.ListAdminWorkspaces)
	admin.POST("/workspaces/members", handler.ListAdminWorkspaceMembers)
	admin.POST("/users/list", handler.ListAdminUsers)
	admin.POST("/users/spaces", handler.ListAdminUserSpaces)
	admin.POST("/users/create", handler.CreateAdminUser)
	admin.POST("/users/update", handler.UpdateAdminUser)
	admin.POST("/users/password/reset", handler.ResetAdminUserPassword)

	sandbox := handler.DefaultAdminSandboxRouteHandlers()
	admin.GET("/sandboxes", sandbox.List)
	admin.POST("/sandboxes", sandbox.Create)
	admin.GET("/sandboxes/defaults", sandbox.ListDefaults)
	admin.GET("/sandboxes/summary", sandbox.GetSummary)
	admin.GET("/sandboxes/capabilities", sandbox.Capabilities)
	admin.GET("/sandboxes/:id", sandbox.Get)
	admin.PUT("/sandboxes/:id", sandbox.Update)
	admin.DELETE("/sandboxes/:id", sandbox.Delete)
	admin.POST("/sandboxes/:id/enable", sandbox.Enable)
	admin.POST("/sandboxes/:id/disable", sandbox.Disable)
	admin.POST("/sandboxes/:id/credentials", sandbox.Credentials)
	admin.POST("/sandboxes/:id/defaults", sandbox.SetDefault)
	admin.POST("/sandboxes/:id/health", sandbox.Health)
	admin.GET("/sandboxes/:id/audit-events", sandbox.AuditEvents)
	registerAdminSandboxTrailingSlashRoutes(admin)
}

func registerWorkbenchCustomRoutes(api *route.RouterGroup) {
	workbench := api.Group("/workbench", _workbenchMw()...)

	workbench.GET("/im_channels", handler.ListIMChannels)
	workbench.POST("/im_channels", handler.CreateIMChannel)
	workbench.PUT("/im_channels/:channel_id", handler.UpdateIMChannel)
	workbench.DELETE("/im_channels/:channel_id", handler.DeleteIMChannel)
	imChannel := workbench.Group("/im_channels/:channel_id")
	imChannel.POST("/enable", handler.EnableIMChannel)
	imChannel.POST("/disable", handler.DisableIMChannel)
	imChannel.POST("/test", handler.TestIMChannelConnection)

	workbench.GET("/mcp_tools", handler.ListMCPToolServers)
	workbench.POST("/mcp_tools", handler.UpsertMCPToolServer)
	workbench.GET("/mcp_tools/registry_entries", handler.ListMCPToolRegistryEntries)
	mcpServer := workbench.Group("/mcp_tools/:server_id")
	mcpServer.GET("", handler.GetMCPToolServer)
	mcpServer.DELETE("", handler.DeleteMCPToolServer)
	mcpServer.POST("/test_call", handler.TestMCPToolCall)
	mcpServer.POST("/discover", handler.DiscoverMCPToolServer)
	mcpServer.GET("/export", handler.ExportMCPToolServer)
	mcpServer.GET("/audit_events", handler.ListMCPToolAuditEvents)

	thread := workbench.Group("/task_threads/:thread_id")
	thread.GET("/uploads", handler.ListTaskThreadUploadFiles)
	thread.POST("/uploads", handler.UploadTaskThreadFiles)
	thread.DELETE("/uploads/:filename", handler.DeleteTaskThreadUploadFile)
	thread.GET("/run_events/stream", handler.StreamTaskThreadRunEvents)
	thread.GET("/artifact_scan_jobs", handler.ListTaskThreadArtifactScanJobs)
	thread.POST("/artifact_scan_jobs/:job_id/retry", handler.RetryTaskThreadArtifactScanJob)
	thread.POST("/artifacts/:artifact_id/scan_review", handler.ReviewTaskThreadArtifactScan)
	thread.GET("/artifacts/:artifact_id/content", handler.GetTaskThreadArtifactContent)
	thread.DELETE("/artifacts/:artifact_id", handler.DeleteTaskThreadArtifact)
	thread.POST("/artifacts/:artifact_id/restore", handler.RestoreTaskThreadArtifact)
}

func registerWorkspaceCustomRoutes(api *route.RouterGroup) {
	workspace := api.Group("/workspace/space")
	workspace.GET("/detail", handler.GetWorkspaceDetail)
	workspace.POST("/members", handler.ListWorkspaceMembers)
	workspace.POST("/users/search", handler.SearchWorkspaceUsers)
	workspace.POST("/update", handler.UpdateWorkspace)
	workspace.POST("/members/add", handler.AddWorkspaceMembers)
	workspace.POST("/member/role", handler.UpdateWorkspaceMemberRole)
	workspace.POST("/member/remove", handler.RemoveWorkspaceMember)
	workspace.POST("/transfer", handler.TransferWorkspace)
	workspace.POST("/delete", handler.DeleteWorkspace)
}

func registerLangGraphCustomRoutes(api *route.RouterGroup) {
	api.POST("/threads", handler.CreateLangGraphThread)
	api.POST("/threads/search", handler.SearchLangGraphThreads)
	thread := api.Group("/threads/:thread_id")
	thread.GET("", handler.GetLangGraphThread)
	thread.PATCH("", handler.PatchLangGraphThread)
	thread.DELETE("", handler.DeleteLangGraphThread)
	thread.GET("/state", handler.GetLangGraphThreadState)
	thread.POST("/state", handler.PostLangGraphThreadState)
	thread.GET("/history", handler.GetLangGraphThreadHistory)
	thread.POST("/history", handler.PostLangGraphThreadHistory)
	thread.GET("/checkpoints/:checkpoint_id/resume", handler.GetLangGraphCheckpointResumeReadiness)
	thread.GET("/messages", handler.ListLangGraphThreadMessages)
	thread.GET("/runs", handler.ListLangGraphRuns)
	thread.POST("/runs", handler.CreateLangGraphRun)
	thread.POST("/runs/stream", handler.CreateLangGraphRunStream)
	thread.POST("/runs/wait", handler.WaitLangGraphRun)

	run := thread.Group("/runs/:run_id")
	run.GET("", handler.GetLangGraphRun)
	run.GET("/messages", handler.ListLangGraphRunMessages)
	run.GET("/events", handler.ListLangGraphRunEvents)
	run.POST("/cancel", handler.CancelLangGraphRun)
	run.GET("/stream", handler.StreamLangGraphRun)
	run.POST("/stream", handler.StreamLangGraphRun)
	run.POST("/join", handler.JoinLangGraphRun)
	run.GET("/join", handler.JoinLangGraphRunStream)

	api.POST("/runs", handler.CreateLangGraphStatelessRun)
	api.POST("/runs/stream", handler.CreateLangGraphStatelessRunStream)
	api.POST("/runs/wait", handler.WaitLangGraphStatelessRun)
	statelessRun := api.Group("/runs/:run_id")
	statelessRun.GET("", handler.GetLangGraphStatelessRun)
	statelessRun.GET("/messages", handler.ListLangGraphStatelessRunMessages)
	statelessRun.GET("/feedback", handler.ListLangGraphStatelessRunFeedback)
	statelessRun.POST("/cancel", handler.CancelLangGraphStatelessRun)
	statelessRun.GET("/stream", handler.StreamLangGraphStatelessRun)
	statelessRun.POST("/join", handler.JoinLangGraphStatelessRun)
	statelessRun.GET("/join", handler.JoinLangGraphStatelessRunStream)
}
