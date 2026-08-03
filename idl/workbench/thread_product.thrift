namespace go workbench.thread_product_contract

include "../base.thrift"

typedef string CanonicalArtifactSource (ts.enum="true")
const CanonicalArtifactSource CanonicalArtifactSource_AgentGenerated = "agent_generated"
const CanonicalArtifactSource CanonicalArtifactSource_UserUpload = "user_upload"
const CanonicalArtifactSource CanonicalArtifactSource_ToolOutput = "tool_output"
const CanonicalArtifactSource CanonicalArtifactSource_ExternalReference = "external_reference"

typedef string CanonicalArtifactGenerationStatus (ts.enum="true")
const CanonicalArtifactGenerationStatus CanonicalArtifactGenerationStatus_Processing = "processing"
const CanonicalArtifactGenerationStatus CanonicalArtifactGenerationStatus_Ready = "ready"
const CanonicalArtifactGenerationStatus CanonicalArtifactGenerationStatus_Failed = "failed"
const CanonicalArtifactGenerationStatus CanonicalArtifactGenerationStatus_Expired = "expired"
const CanonicalArtifactGenerationStatus CanonicalArtifactGenerationStatus_Blocked = "blocked"

typedef string CanonicalArtifactPreviewMode (ts.enum="true")
const CanonicalArtifactPreviewMode CanonicalArtifactPreviewMode_Text = "text"
const CanonicalArtifactPreviewMode CanonicalArtifactPreviewMode_Image = "image"
const CanonicalArtifactPreviewMode CanonicalArtifactPreviewMode_PDF = "pdf"
const CanonicalArtifactPreviewMode CanonicalArtifactPreviewMode_Audio = "audio"
const CanonicalArtifactPreviewMode CanonicalArtifactPreviewMode_Video = "video"
const CanonicalArtifactPreviewMode CanonicalArtifactPreviewMode_MediaCollection = "media_collection"
const CanonicalArtifactPreviewMode CanonicalArtifactPreviewMode_Download = "download"
const CanonicalArtifactPreviewMode CanonicalArtifactPreviewMode_Unsupported = "unsupported"

typedef string CanonicalArtifactCapability (ts.enum="true")
const CanonicalArtifactCapability CanonicalArtifactCapability_Open = "open"
const CanonicalArtifactCapability CanonicalArtifactCapability_Preview = "preview"
const CanonicalArtifactCapability CanonicalArtifactCapability_Download = "download"
const CanonicalArtifactCapability CanonicalArtifactCapability_Copy = "copy"

struct CanonicalProductEmptyResponse {
}

struct CanonicalProductThreadRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalProductPageRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i32 limit (api.query="limit")
    4: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalUploadFile {
    1: required string file_id
    2: required string file_name
    3: required string virtual_path
    4: required string content_type
    5: required i64 size_bytes
    6: required string created_at
}

struct CanonicalArtifact {
    1: required string artifact_id
    2: required string thread_id
    3: required string run_id
    4: required string file_id
    5: required string title
    6: required string artifact_type
    7: required string virtual_path
    8: required string content_type
    9: required i64 size_bytes
    10: required CanonicalArtifactPreviewMode preview_mode
    11: required string metadata (api.value_type="any")
    12: required string created_at
    13: required string updated_at
    14: optional string deleted_at
    15: optional CanonicalArtifactSource source
    16: optional CanonicalArtifactGenerationStatus generation_status
    17: optional list<CanonicalArtifactCapability> capabilities
    18: optional string collection_id
    19: optional i32 collection_order
    20: optional bool is_primary
}

struct CanonicalArtifactCollection {
    1: required string collection_id
    2: required list<string> artifact_ids
    3: optional i32 current_index
    4: optional i32 total_count
}

struct CanonicalArtifactScanJob {
    1: required string job_id
    2: required string thread_id
    3: required string run_id
    4: required string artifact_id
    5: required string file_id
    6: required string scanner
    7: required string status
    8: required string worker_ref
    9: required i32 attempt_count
    10: required string error_code
    11: optional string available_at
    12: optional string started_at
    13: optional string ended_at
    14: required string created_at
    15: required string updated_at
}

struct CanonicalTokenUsage {
    1: required string usage_id
    2: required string thread_id
    3: required string run_id
    4: required string source
    5: required string step_id
    6: required i32 step_index
    7: required string step_name
    8: required string model_name
    9: required string provider
    10: required i64 input_tokens
    11: required i64 output_tokens
    12: required i64 total_tokens
    13: required i64 cost_micros
    14: required string currency
    15: required bool estimated
    16: required string created_at
}

struct CanonicalTokenUsageAggregate {
    1: required i64 input_tokens
    2: required i64 output_tokens
    3: required i64 total_tokens
    4: required i64 cost_micros
    5: required i64 call_count
    6: required i64 lead_agent_tokens
    7: required i64 subagent_tokens
    8: required i64 middleware_tokens
    9: required i64 tool_tokens
}

struct CanonicalRunTokenUsageAggregate {
    1: required string run_id
    2: required CanonicalTokenUsageAggregate aggregate
}

struct CanonicalMemory {
    1: required string memory_id
    2: required string thread_id
    3: optional string run_id
    4: required string scope
    5: required string content
    6: required string metadata (api.value_type="any")
    7: required double score
    8: required double confidence
    9: required string source_type
    10: required string source_id
    11: optional string correction_of_memory_id
    12: optional string corrected_at
    13: optional string expires_at
    14: required string created_at
    15: required string updated_at
    16: optional string deleted_at
}

struct CanonicalMemoryAuditEvent {
    1: required string event_id
    2: required string thread_id
    3: optional string run_id
    4: optional string memory_id
    5: optional string actor_id
    6: required string event_type
    7: required string scope
    8: required string source_type
    9: required string source_id
    10: required i64 affected_count
    11: required string created_at
}

struct CanonicalGuardrailAuditEvent {
    1: required string event_id
    2: required string thread_id
    3: optional string run_id
    4: optional string actor_id
    5: required string event_type
    6: required string target_type
    7: required string target_id
    8: required string operation
    9: required string source
    10: required string action
    11: required string fail_mode
    12: required string provider
    13: required string reason_code
    14: required list<string> rule_ids
    15: required string created_at
}

struct CanonicalMCPRuntimeAuditEvent {
    1: required string event_id
    2: required string thread_id
    3: optional string run_id
    4: optional string server_id
    5: required string runtime_tool_name
    6: required string event_type
    7: required string error_code
    8: required i64 elapsed_millis
    9: required i64 output_bytes
    10: required string created_at
}

struct CanonicalUploadListResponse {
    1: required list<CanonicalUploadFile> uploads
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
}

struct CanonicalArtifactListResponse {
    1: required list<CanonicalArtifact> artifacts
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
    5: optional list<CanonicalArtifactCollection> collections
}

struct CanonicalArtifactScanJobListResponse {
    1: required list<CanonicalArtifactScanJob> jobs
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
}

struct CanonicalMemoryListResponse {
    1: required list<CanonicalMemory> memories
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
}

struct CanonicalMemoryAuditEventListResponse {
    1: required list<CanonicalMemoryAuditEvent> events
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
}

struct CanonicalGuardrailAuditEventListResponse {
    1: required list<CanonicalGuardrailAuditEvent> events
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
}

struct CanonicalMCPRuntimeAuditEventListResponse {
    1: required list<CanonicalMCPRuntimeAuditEvent> events
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
}

struct CanonicalTokenUsageResponse {
    1: required list<CanonicalTokenUsage> usage
    2: required i64 total
    3: required bool has_more
    4: optional string next_cursor
    5: required CanonicalTokenUsageAggregate aggregate
    6: required list<CanonicalRunTokenUsageAggregate> run_aggregates
}

struct CanonicalSuggestionResponse {
    1: required list<string> suggestions
}

struct CanonicalUploadResponse {
    1: required list<CanonicalUploadFile> uploads
    2: required list<string> skipped_files
}

struct CanonicalArtifactContentResponse {
    1: required binary body (api.body=".")
}

struct CanonicalArtifactSignedURLResponse {
    1: required string artifact_id
    2: required string url
    3: required i64 expires_in_seconds
    4: required string content_type
    5: required CanonicalArtifactPreviewMode preview_mode
}

struct CanonicalArtifactRestoreResponse {
    1: required CanonicalArtifact artifact
    2: required bool restored
}

struct CanonicalArtifactScanReviewResponse {
    1: required string artifact_id
    2: required string decision
    3: required string scan_status
    4: required bool reviewed
}

struct CanonicalArtifactScanJobRetryResponse {
    1: required CanonicalArtifactScanJob job
    2: required bool retried
}

struct CanonicalMemoryUpdateResponse {
    1: required CanonicalMemory memory
    2: required bool updated
}

struct CanonicalMemoryRestoreResponse {
    1: required CanonicalMemory memory
    2: required bool restored
}

struct CanonicalMemoryClearResponse {
    1: required i64 deleted
}

struct CanonicalMemoryImportResponse {
    1: required i64 imported
    2: required i64 skipped
    3: required list<CanonicalMemory> memories
}

struct CanonicalMemoryExportResponse {
    1: required string schema
    2: required string thread_id
    3: required string exported_at
    4: required i64 total
    5: required list<CanonicalMemory> memories
}

struct CanonicalGuardrailAuditExportResponse {
    1: required string schema
    2: required string thread_id
    3: required string exported_at
    4: required i64 total
    5: required list<CanonicalGuardrailAuditEvent> events
}

struct AppendCanonicalThreadMessageRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.body="run_id", agw.js_conv="str", api.js_conv="true")
    4: required string role (api.body="role")
    5: required string content (api.body="content")
    6: optional string metadata (api.body="metadata", api.value_type="any")
    7: required string append_mode (api.body="append_mode")
    255: optional base.Base Base (api.none="true")
}

struct GenerateCanonicalThreadSuggestionsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i32 n (api.body="n")
    4: optional string model_name (api.body="model_name")
    5: optional i64 model_type (api.body="model_type", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct UploadCanonicalThreadFilesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct DeleteCanonicalThreadUploadRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 file_id (api.path="file_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalThreadArtifactsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.query="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional bool deleted_only (api.query="deleted_only")
    5: optional i32 limit (api.query="limit")
    6: optional i32 offset (api.query="offset")
    7: optional string collection_id (api.query="collection_id")
    255: optional base.Base Base (api.none="true")
}

struct CopyCanonicalThreadArtifactLinkRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 artifact_id (api.path="artifact_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CopyCanonicalThreadArtifactLinkResponse {
    1: required string artifact_id
    2: required string copy_url
    3: required string expires_at
}

struct CanonicalArtifactRouteRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 artifact_id (api.path="artifact_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetCanonicalThreadArtifactContentRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 artifact_id (api.path="artifact_id", agw.js_conv="str", api.js_conv="true")
    4: optional string mode (api.query="mode")
    255: optional base.Base Base (api.none="true")
}

struct GetCanonicalThreadArtifactSignedURLRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 artifact_id (api.path="artifact_id", agw.js_conv="str", api.js_conv="true")
    4: optional string mode (api.query="mode")
    5: optional i64 ttl_seconds (api.query="ttl_seconds")
    255: optional base.Base Base (api.none="true")
}

struct ReviewCanonicalThreadArtifactScanRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 artifact_id (api.path="artifact_id", agw.js_conv="str", api.js_conv="true")
    4: required string decision (api.body="decision")
    5: optional string reason (api.body="reason")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalThreadArtifactScanJobsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.query="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional i64 artifact_id (api.query="artifact_id", agw.js_conv="str", api.js_conv="true")
    5: optional string status (api.query="status")
    6: optional string scanner (api.query="scanner")
    7: optional i32 limit (api.query="limit")
    8: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}

struct RetryCanonicalThreadArtifactScanJobRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 job_id (api.path="job_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetCanonicalThreadTokenUsageRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.query="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional bool include_child_runs (api.query="include_child_runs")
    5: optional string source (api.query="source")
    6: optional i32 limit (api.query="limit")
    7: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalThreadMemoriesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.query="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional string scope (api.query="scope")
    5: optional list<string> scopes (api.query="scopes")
    6: optional string q (api.query="q")
    7: optional bool include_expired (api.query="include_expired")
    8: optional bool include_deleted (api.query="include_deleted")
    9: optional i32 limit (api.query="limit")
    10: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}

struct UpdateCanonicalThreadMemoryRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 memory_id (api.path="memory_id", agw.js_conv="str", api.js_conv="true")
    4: optional i64 run_id (api.body="run_id", agw.js_conv="str", api.js_conv="true")
    5: required string scope (api.body="scope")
    6: required string content (api.body="content")
    7: optional string metadata (api.body="metadata", api.value_type="any")
    8: optional double score (api.body="score")
    9: optional double confidence (api.body="confidence")
    10: optional string source_type (api.body="source_type")
    11: optional string source_id (api.body="source_id")
    12: optional i64 correction_of_memory_id (api.body="correction_of_memory_id", agw.js_conv="str", api.js_conv="true")
    13: optional string corrected_at (api.body="corrected_at")
    14: optional string expires_at (api.body="expires_at")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalMemoryRouteRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 memory_id (api.path="memory_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ClearCanonicalThreadMemoriesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.body="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional list<string> scopes (api.body="scopes")
    255: optional base.Base Base (api.none="true")
}

struct ExportCanonicalThreadMemoriesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.query="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional string scope (api.query="scope")
    5: optional list<string> scopes (api.query="scopes")
    6: optional string q (api.query="q")
    7: optional bool include_expired (api.query="include_expired")
    8: optional bool include_deleted (api.query="include_deleted")
    9: optional i32 limit (api.query="limit")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalImportMemoryItem {
    1: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    2: optional string scope
    3: required string content
    4: optional string metadata (api.value_type="any")
    5: optional double score
    6: optional double confidence
    7: optional string source_type
    8: optional string source_id
    9: optional i64 correction_of_memory_id (agw.js_conv="str", api.js_conv="true")
    10: optional string corrected_at
    11: optional string expires_at
}

struct ImportCanonicalThreadMemoriesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required list<CanonicalImportMemoryItem> memories (api.body="memories")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalThreadMemoryAuditEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 memory_id (api.query="memory_id", agw.js_conv="str", api.js_conv="true")
    4: optional i32 limit (api.query="limit")
    5: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalThreadGuardrailAuditEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.query="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional i32 limit (api.query="limit")
    5: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}

struct ExportCanonicalThreadGuardrailAuditEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.query="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional i32 limit (api.query="limit")
    5: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalThreadMCPRuntimeAuditEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (api.query="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional i32 limit (api.query="limit")
    5: optional i32 offset (api.query="offset")
    255: optional base.Base Base (api.none="true")
}

struct RetryCanonicalSubagentRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    3: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    4: optional string idempotency_key (api.header="Idempotency-Key")
    255: optional base.Base Base (api.none="true")
}
