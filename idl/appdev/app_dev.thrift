namespace go appdev

// Generic contracts remain only for legacy AppDev routes that have not yet
// moved to the provider control plane.
struct AppDevRouteRequest {}
struct AppDevRouteResponse {}

struct AppDevProviderScopeRequest {
    1: required string SpaceID (api.path = "space_id", go.tag = "json:\"-\"")
    2: required string ProjectID (api.path = "project_id", go.tag = "json:\"-\"")
}

struct AppDevProviderOperationRequest {
    1: required string SpaceID (api.path = "space_id", go.tag = "json:\"-\"")
    2: required string ProjectID (api.path = "project_id", go.tag = "json:\"-\"")
    3: required string OperationID (api.header = "Idempotency-Key", go.tag = "json:\"-\"")
}

struct AppDevSnapshotRestoreRequest {
    1: required string SpaceID (api.path = "space_id", go.tag = "json:\"-\"")
    2: required string ProjectID (api.path = "project_id", go.tag = "json:\"-\"")
    3: required string SnapshotID (api.path = "snapshot_id", go.tag = "json:\"-\"")
    4: required string OperationID (api.header = "Idempotency-Key", go.tag = "json:\"-\"")
}

struct AppDevProviderRuntimeResponse {
    1: required i64 Generation (go.tag = "json:\"generation\"")
    2: required string State (go.tag = "json:\"state\"")
    3: required bool CanStart (go.tag = "json:\"can_start\"")
    4: required bool Recovering (go.tag = "json:\"recovering\"")
    5: required bool Stopping (go.tag = "json:\"stopping\"")
    6: optional string PreviewURL (go.tag = "json:\"preview_url,omitempty\"")
    7: optional string SafeMessage (go.tag = "json:\"safe_message,omitempty\"")
}

struct AppDevProviderBuildResponse {
    1: required i64 Generation (go.tag = "json:\"generation\"")
    2: required string State (go.tag = "json:\"state\"")
    3: required bool ReleaseAvailable (go.tag = "json:\"release_available\"")
    4: required i64 Size (go.tag = "json:\"size\"")
    5: optional string UpdatedAt (go.tag = "json:\"updated_at,omitempty\"")
	6: required bool Stale (go.tag = "json:\"stale\"")
	7: optional string SafeMessage (go.tag = "json:\"safe_message,omitempty\"")
	8: optional string SafeErrorCode (go.tag = "json:\"safe_error_code,omitempty\"")
}

struct AppDevProviderLogsResponse {
    1: required string State (go.tag = "json:\"state\"")
    2: required list<string> Logs (go.tag = "json:\"logs\"")
    3: optional string SafeMessage (go.tag = "json:\"safe_message,omitempty\"")
}

// The handler writes a verified application/zip stream and its HTTP headers.
struct AppDevReleaseStreamResponse {}

service AppDevService {
    AppDevRouteResponse ListAppDevModels(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/models",
        api.category="app_dev"
    )
    AppDevRouteResponse ListAppDevProjects(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects",
        api.category="app_dev"
    )
    AppDevRouteResponse CreateAppDevProject(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects",
        api.category="app_dev"
    )
    AppDevRouteResponse ImportAppDevProject(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/import",
        api.category="app_dev"
    )
    AppDevRouteResponse GetAppDevProject(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id",
        api.category="app_dev"
    )
    AppDevRouteResponse UpdateAppDevProject(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id",
        api.category="app_dev"
    )
    AppDevRouteResponse DuplicateAppDevProject(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/duplicate",
        api.category="app_dev"
    )
    AppDevRouteResponse ArchiveAppDevProject(1: AppDevRouteRequest request)(
        api.delete="/api/app-dev/spaces/:space_id/projects/:project_id",
        api.category="app_dev"
    )
    AppDevRouteResponse ExportAppDevProject(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/export",
        api.category="app_dev"
    )
    AppDevProviderBuildResponse BuildAppDevProject(1: AppDevProviderOperationRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/build",
        api.category="app_dev"
    )
    AppDevProviderBuildResponse GetAppDevBuildStatus(1: AppDevProviderOperationRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/build",
        api.category="app_dev"
    )
    AppDevReleaseStreamResponse DownloadAppDevRelease(1: AppDevProviderScopeRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/release",
        api.category="app_dev"
    )
    AppDevRouteResponse ListAppDevFiles(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/files",
        api.category="app_dev"
    )
    AppDevRouteResponse DeleteAppDevFile(1: AppDevRouteRequest request)(
        api.delete="/api/app-dev/spaces/:space_id/projects/:project_id/files",
        api.category="app_dev"
    )
    AppDevRouteResponse GetAppDevFileContent(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/files/content",
        api.category="app_dev"
    )
    AppDevRouteResponse SaveAppDevFileContent(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/files/content",
        api.category="app_dev"
    )
    AppDevRouteResponse UploadAppDevFiles(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/files/upload",
        api.category="app_dev"
    )
    AppDevRouteResponse RenameAppDevFile(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/files/rename",
        api.category="app_dev"
    )
    AppDevRouteResponse ListAppDevSnapshots(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/snapshots",
        api.category="app_dev"
    )
    AppDevRouteResponse CreateAppDevSnapshot(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/snapshots",
        api.category="app_dev"
    )
    AppDevProviderRuntimeResponse RestoreAppDevSnapshot(1: AppDevSnapshotRestoreRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/snapshots/:snapshot_id/restore",
        api.category="app_dev"
    )
    AppDevProviderRuntimeResponse StartAppDevRuntime(1: AppDevProviderOperationRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/start",
        api.category="app_dev"
    )
    AppDevProviderRuntimeResponse GetAppDevRuntimeStatus(1: AppDevProviderScopeRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/status",
        api.category="app_dev"
    )
    AppDevProviderRuntimeResponse KeepAliveAppDevRuntime(1: AppDevProviderScopeRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/keep-alive",
        api.category="app_dev"
    )
    AppDevProviderRuntimeResponse RestartAppDevRuntime(1: AppDevProviderOperationRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/restart",
        api.category="app_dev"
    )
    AppDevProviderRuntimeResponse StopAppDevRuntime(1: AppDevProviderOperationRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/stop",
        api.category="app_dev"
    )
    AppDevProviderLogsResponse ListAppDevRuntimeLogs(1: AppDevProviderScopeRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/runtime/logs",
        api.category="app_dev"
    )
    AppDevRouteResponse SendAppDevChatMessage(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/chat",
        api.category="app_dev"
    )
    AppDevRouteResponse SubscribeAppDevChatEvents(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/chat/events",
        api.category="app_dev"
    )
    AppDevRouteResponse CancelAppDevChat(1: AppDevRouteRequest request)(
        api.post="/api/app-dev/spaces/:space_id/projects/:project_id/chat/cancel",
        api.category="app_dev"
    )
    AppDevRouteResponse ListAppDevChatHistory(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/chat/history",
        api.category="app_dev"
    )
    AppDevRouteResponse GetAppDevChatStatus(1: AppDevRouteRequest request)(
        api.get="/api/app-dev/spaces/:space_id/projects/:project_id/chat/status",
        api.category="app_dev"
    )
}
