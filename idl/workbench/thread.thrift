namespace go workbench.thread_contract

include "../base.thrift"
include "./journal.thrift"
include "./thread_product.thrift"

struct CanonicalRouteRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalRunRouteRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CanonicalThread {
    1: required string thread_id
    2: required string created_at
    3: required string updated_at
    4: required string metadata (api.value_type="any")
    5: required string status
    6: required string values (api.value_type="any")
    7: required string interrupts (api.value_type="any")
    8: required string coze (api.value_type="any")
}

struct CanonicalRun {
    1: required string run_id
    2: required string thread_id
    3: required string assistant_id
    4: required string status
    5: required string created_at
    6: required string updated_at
    7: required string metadata (api.value_type="any")
    8: required string multitask_strategy
    9: required string coze (api.value_type="any")
}

struct CanonicalCheckpoint {
    1: required string thread_id
    2: required string checkpoint_ns
    3: required string checkpoint_id
    4: required string checkpoint_map (api.value_type="any")
}

struct CanonicalThreadState {
    1: required string values (api.value_type="any")
    2: required list<string> next
    3: required CanonicalCheckpoint checkpoint
    4: required string metadata (api.value_type="any")
    5: required string created_at
    6: optional CanonicalCheckpoint parent_checkpoint
    7: required string tasks (api.value_type="any")
    8: required string interrupts (api.value_type="any")
}

struct CanonicalThreadUpdateStateResult {
    1: required CanonicalCheckpoint checkpoint
    2: required CanonicalCheckpoint configurable
}

struct CanonicalMessage {
    1: required string message_id
    2: required string thread_id
    3: required string run_id
    4: required string role
    5: required string content
    6: required string metadata (api.value_type="any")
    7: required string created_at
    8: optional string seq
}

struct CanonicalMessagePage {
    1: required list<CanonicalMessage> data
    2: required bool has_more
    3: optional string next_before_seq
    4: optional string next_after_seq
}

struct CanonicalRunEventPage {
    1: required list<journal.JournalEvent> data
    2: required bool has_more
    3: optional string next_after_event_id
    4: optional string attempt_id
    5: optional i64 latest_sequence
    6: optional i64 next_after_sequence
}

struct CanonicalThreadListResponse {
    1: required list<CanonicalThread> body (api.body=".")
}

struct CanonicalThreadStateListResponse {
    1: required list<CanonicalThreadState> body (api.body=".")
}

struct CanonicalRunListResponse {
    1: required list<CanonicalRun> body (api.body=".")
}

struct CanonicalValuesResponse {
    1: required string body (api.body=".", api.value_type="any")
}

struct CanonicalStreamResponse {
    1: required string body (api.body=".")
}

struct CanonicalEmptyResponse {
}

struct CanonicalComposerSelectionV2 {
    1: optional i64 model_type (agw.js_conv="str", api.js_conv="true")
    2: optional string model_name
    3: optional list<string> explicit_enable_skills
    4: required list<string> allowed_skills
    5: required list<string> enable_mcp
    6: required list<string> enable_kbs
    7: required list<string> enable_databases
    8: required list<string> allowed_mcp_tools
}

struct CanonicalMemoryRetrievalV2 {
    1: required i32 limit
    2: required i32 candidate_limit
    3: required list<string> scopes
    4: required double min_confidence
}

struct CanonicalSkillsV2 {
    1: required bool enabled
    2: required string visibility
}

struct CanonicalMCPToolsV2 {
    1: required bool enabled
    2: required string visibility
}

struct CanonicalWebHTTPV2 {
    1: required bool enabled
    2: required list<string> allowed_hosts
    3: required i64 timeout_ms
    4: required i64 max_response_bytes
}

struct CanonicalWebSearchV2 {
    1: required bool enabled
    2: required i32 max_results
}

struct CanonicalWebToolsV2 {
    1: required bool enabled
    2: required string visibility
    3: required CanonicalWebHTTPV2 http
    4: required CanonicalWebSearchV2 search
}

struct CanonicalModelRetryV2 {
    1: required i32 max_retries
    2: required i64 backoff_ms
    3: required bool retry_empty_output
    4: required list<string> retry_finish_reasons
}

struct CanonicalModelFailoverV2 {
    1: required list<i64> candidate_model_ids (agw.js_conv="str", api.js_conv="true")
    2: required i32 max_retries
    3: required bool failover_empty_output
    4: required list<string> failover_finish_reasons
}

struct CanonicalTokenUsageV2 {
    1: required bool enabled
}

struct CanonicalRunConfigV2 {
    1: required string runtime
    2: required CanonicalMemoryRetrievalV2 memory_retrieval
    3: required CanonicalSkillsV2 skills
    4: required CanonicalMCPToolsV2 mcp_tools
    5: required CanonicalWebToolsV2 web_tools
    6: optional CanonicalModelRetryV2 model_retry
    7: optional CanonicalModelFailoverV2 model_failover
    8: required CanonicalTokenUsageV2 token_usage
}

struct CanonicalUploadedFileReferenceV2 {
    1: required i64 file_id (agw.js_conv="str", api.js_conv="true")
}

struct CanonicalRunInputV2 {
    1: required string message
    2: required list<CanonicalUploadedFileReferenceV2> uploaded_files
}

struct CanonicalRunLineageV2 {
    1: required i64 source_run_id (agw.js_conv="str", api.js_conv="true")
}

struct CanonicalRunMetadataV2 {
    1: required string source
}

struct CanonicalRunSubmissionV2 {
    1: required string schema_version
    2: required string kind
    3: required CanonicalRunInputV2 input
    4: required CanonicalComposerSelectionV2 composer
    5: required CanonicalRunConfigV2 config
    6: optional CanonicalRunLineageV2 lineage
    7: optional CanonicalRunMetadataV2 metadata
}

struct CanonicalInitialRunSubmissionV2 {
    1: required string schema_version
    2: required CanonicalRunInputV2 input
    3: required CanonicalComposerSelectionV2 composer
    4: required CanonicalRunConfigV2 config
    5: optional CanonicalRunMetadataV2 metadata
}

struct CanonicalHumanInteractionResponseV2 {
    1: required string schema
    2: required string interaction_id
    3: required string kind
    4: required string decision
    5: optional string answer
    6: optional string choice_id
    7: optional string comment
}

struct CreateCanonicalThreadRequest {
    1: optional string thread_id (api.body="thread_id")
    2: optional string metadata (api.body="metadata", api.value_type="any")
    3: optional string if_exists (api.body="if_exists")
    4: optional string ttl (api.body="ttl", api.value_type="any")
    5: optional string supersteps (api.body="supersteps", api.value_type="any")
    6: optional string coze (api.body="coze", api.value_type="any")
    7: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    8: optional CanonicalInitialRunSubmissionV2 initial_submission_v2 (api.body="initial_submission_v2")
    9: optional CanonicalInitialRunSubmissionV2 deferred_initial_submission_v2 (api.body="deferred_initial_submission_v2")
    255: optional base.Base Base (api.none="true")
}

struct SearchCanonicalThreadsRequest {
    1: optional string metadata (api.body="metadata", api.value_type="any")
    2: optional string status (api.body="status")
    3: optional list<string> ids (api.body="ids")
    4: optional i32 limit (api.body="limit")
    5: optional i32 offset (api.body="offset")
    6: optional string sort_by (api.body="sort_by")
    7: optional string sort_order (api.body="sort_order")
    8: optional bool values (api.body="values")
    9: optional list<string> select (api.body="select")
    10: optional bool extract (api.body="extract")
    11: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetCanonicalThreadRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional list<string> include (api.query="include")
    3: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct PatchCanonicalThreadRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional string prefer (api.header="Prefer")
    3: optional string metadata (api.body="metadata", api.value_type="any")
    4: optional string ttl (api.body="ttl", api.value_type="any")
    5: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetCanonicalThreadStateRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional string checkpoint (api.query="checkpoint")
    3: optional string checkpoint_id (api.query="checkpoint_id")
    4: optional bool subgraphs (api.query="subgraphs")
    5: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct UpdateCanonicalThreadStateRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional string values (api.body="values", api.value_type="any")
    3: optional string as_node (api.body="as_node")
    4: optional string checkpoint (api.body="checkpoint", api.value_type="any")
    5: optional string checkpoint_id (api.body="checkpoint_id")
    6: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetCanonicalThreadHistoryRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i32 limit (api.query="limit")
    3: optional string before (api.query="before")
    4: optional string checkpoint (api.query="checkpoint")
    5: optional string checkpoint_id (api.query="checkpoint_id")
    6: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct PostCanonicalThreadHistoryRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i32 limit (api.body="limit")
    3: optional string before (api.body="before")
    4: optional string checkpoint (api.body="checkpoint", api.value_type="any")
    5: optional string checkpoint_id (api.body="checkpoint_id")
    6: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalThreadMessagesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional string before_seq (api.query="before_seq")
    3: optional string after_seq (api.query="after_seq")
    4: optional i32 limit (api.query="limit")
    5: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalRunsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional string status (api.query="status")
    3: optional i32 limit (api.query="limit")
    4: optional i32 offset (api.query="offset")
    5: optional string parent_run_id (api.query="parent_run_id")
    6: optional list<string> select (api.query="select")
    7: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CreateCanonicalRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required string assistant_id (api.body="assistant_id")
    3: optional string input (api.body="input", api.value_type="any")
    4: optional string command (api.body="command", api.value_type="any")
    5: optional string metadata (api.body="metadata", api.value_type="any")
    6: optional string config (api.body="config", api.value_type="any")
    7: optional string context (api.body="context", api.value_type="any")
    8: optional string stream_mode (api.body="stream_mode", api.value_type="any")
    9: optional string multitask_strategy (api.body="multitask_strategy")
    10: optional string on_disconnect (api.body="on_disconnect")
    11: optional string durability (api.body="durability")
    12: optional bool stream_resumable (api.body="stream_resumable")
    13: optional bool stream_subgraphs (api.body="stream_subgraphs")
    14: optional string if_not_exists (api.body="if_not_exists")
    15: optional string webhook (api.body="webhook", api.value_type="any")
    16: optional string on_completion (api.body="on_completion", api.value_type="any")
    17: optional string after_seconds (api.body="after_seconds", api.value_type="any")
    18: optional string feedback_keys (api.body="feedback_keys", api.value_type="any")
    19: optional string interrupt_before (api.body="interrupt_before", api.value_type="any")
    20: optional string interrupt_after (api.body="interrupt_after", api.value_type="any")
    21: optional string checkpoint (api.body="checkpoint", api.value_type="any")
    22: optional string checkpoint_id (api.body="checkpoint_id", api.value_type="any")
    23: optional string langsmith_tracer (api.body="langsmith_tracer", api.value_type="any")
    24: optional string idempotency_key (api.header="Idempotency-Key")
    25: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    26: optional string coze (api.body="coze", api.value_type="any")
    27: optional CanonicalRunSubmissionV2 submission_v2 (api.body="submission_v2")
    255: optional base.Base Base (api.none="true")
}

struct WaitCanonicalRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required string assistant_id (api.body="assistant_id")
    3: optional string input (api.body="input", api.value_type="any")
    4: optional string command (api.body="command", api.value_type="any")
    5: optional string metadata (api.body="metadata", api.value_type="any")
    6: optional string config (api.body="config", api.value_type="any")
    7: optional string context (api.body="context", api.value_type="any")
    8: optional string stream_mode (api.body="stream_mode", api.value_type="any")
    9: optional string multitask_strategy (api.body="multitask_strategy")
    10: optional string on_disconnect (api.body="on_disconnect")
    11: optional string durability (api.body="durability")
    12: optional bool stream_resumable (api.body="stream_resumable")
    13: optional bool stream_subgraphs (api.body="stream_subgraphs")
    14: optional string if_not_exists (api.body="if_not_exists")
    15: optional string webhook (api.body="webhook", api.value_type="any")
    16: optional string on_completion (api.body="on_completion", api.value_type="any")
    17: optional string after_seconds (api.body="after_seconds", api.value_type="any")
    18: optional string feedback_keys (api.body="feedback_keys", api.value_type="any")
    19: optional string interrupt_before (api.body="interrupt_before", api.value_type="any")
    20: optional string interrupt_after (api.body="interrupt_after", api.value_type="any")
    21: optional string checkpoint (api.body="checkpoint", api.value_type="any")
    22: optional string checkpoint_id (api.body="checkpoint_id", api.value_type="any")
    23: optional string langsmith_tracer (api.body="langsmith_tracer", api.value_type="any")
    24: optional bool raise_error (api.body="raise_error")
    25: optional string idempotency_key (api.header="Idempotency-Key")
    26: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    27: optional string coze (api.body="coze", api.value_type="any")
    28: optional CanonicalRunSubmissionV2 submission_v2 (api.body="submission_v2")
    255: optional base.Base Base (api.none="true")
}

struct ReconnectCanonicalRunStreamRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: optional string after_event_id (api.query="after_event_id")
    4: optional string cancel_on_disconnect (api.query="cancel_on_disconnect")
    5: optional string last_event_id (api.header="Last-Event-ID")
    6: optional list<string> stream_mode (api.query="stream_mode")
    7: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    8: optional string journal_protocol_version (api.query="journal_protocol_version")
    255: optional base.Base Base (api.none="true")
}

struct JoinCanonicalRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: optional string cancel_on_disconnect (api.query="cancel_on_disconnect")
    4: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct CancelCanonicalRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: optional string action (api.query="action")
    4: optional string wait (api.query="wait")
    5: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct ResumeCanonicalRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: optional string interrupt_id (api.body="interrupt_id")
    4: optional string response (api.body="response", api.value_type="any")
    5: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    6: optional string idempotency_key (api.header="Idempotency-Key")
    7: optional CanonicalHumanInteractionResponseV2 response_v2 (api.body="response_v2")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalRunEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: optional string after_event_id (api.query="after_event_id")
    4: optional list<string> event_types (api.query="event_types")
    5: optional i32 limit (api.query="limit")
    6: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    7: optional string attempt_id (api.query="attempt_id")
    8: optional i64 after_sequence (api.query="after_sequence")
    255: optional base.Base Base (api.none="true")
}

struct ListCanonicalRunMessagesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: optional string before_seq (api.query="before_seq")
    4: optional string after_seq (api.query="after_seq")
    5: optional i32 limit (api.query="limit")
    6: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

service WorkbenchCanonicalThreadService {
    CanonicalThread CreateCanonicalThread(1: CreateCanonicalThreadRequest req) (api.post="/api/workbench/threads")
    CanonicalThreadListResponse SearchCanonicalThreads(1: SearchCanonicalThreadsRequest req) (api.post="/api/workbench/threads/search")
    CanonicalThread GetCanonicalThread(1: GetCanonicalThreadRequest req) (api.get="/api/workbench/threads/:thread_id")
    CanonicalThread PatchCanonicalThread(1: PatchCanonicalThreadRequest req) (api.patch="/api/workbench/threads/:thread_id")
    CanonicalEmptyResponse DeleteCanonicalThread(1: CanonicalRouteRequest req) (api.delete="/api/workbench/threads/:thread_id")
    CanonicalThreadState GetCanonicalThreadState(1: GetCanonicalThreadStateRequest req) (api.get="/api/workbench/threads/:thread_id/state")
    CanonicalThreadUpdateStateResult UpdateCanonicalThreadState(1: UpdateCanonicalThreadStateRequest req) (api.post="/api/workbench/threads/:thread_id/state")
    CanonicalThreadStateListResponse GetCanonicalThreadHistory(1: GetCanonicalThreadHistoryRequest req) (api.get="/api/workbench/threads/:thread_id/history")
    CanonicalThreadStateListResponse PostCanonicalThreadHistory(1: PostCanonicalThreadHistoryRequest req) (api.post="/api/workbench/threads/:thread_id/history")
    CanonicalMessagePage ListCanonicalThreadMessages(1: ListCanonicalThreadMessagesRequest req) (api.get="/api/workbench/threads/:thread_id/messages")
    CanonicalRunListResponse ListCanonicalRuns(1: ListCanonicalRunsRequest req) (api.get="/api/workbench/threads/:thread_id/runs")
    CanonicalRun CreateCanonicalRun(1: CreateCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs")
    CanonicalStreamResponse StreamCanonicalRun(1: CreateCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs/stream")
    CanonicalValuesResponse WaitCanonicalRun(1: WaitCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs/wait")
    CanonicalRun GetCanonicalRun(1: CanonicalRunRouteRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id")
    CanonicalStreamResponse ReconnectCanonicalRunStream(1: ReconnectCanonicalRunStreamRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/stream")
    CanonicalValuesResponse JoinCanonicalRun(1: JoinCanonicalRunRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/join")
    CanonicalEmptyResponse CancelCanonicalRun(1: CancelCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs/:run_id/cancel")
    CanonicalRun ResumeCanonicalRun(1: ResumeCanonicalRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs/:run_id/resume")
    CanonicalRunEventPage ListCanonicalRunEvents(1: ListCanonicalRunEventsRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/events")
    CanonicalMessagePage ListCanonicalRunMessages(1: ListCanonicalRunMessagesRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/messages")
    CanonicalMessage AppendCanonicalThreadMessage(1: thread_product.AppendCanonicalThreadMessageRequest req) (api.post="/api/workbench/threads/:thread_id/messages")
    thread_product.CanonicalSuggestionResponse GenerateCanonicalThreadSuggestions(1: thread_product.GenerateCanonicalThreadSuggestionsRequest req) (api.post="/api/workbench/threads/:thread_id/suggestions")
    thread_product.CanonicalUploadListResponse ListCanonicalThreadUploads(1: thread_product.CanonicalProductThreadRequest req) (api.get="/api/workbench/threads/:thread_id/uploads")
    thread_product.CanonicalUploadResponse UploadCanonicalThreadFiles(1: thread_product.UploadCanonicalThreadFilesRequest req) (api.post="/api/workbench/threads/:thread_id/uploads")
    thread_product.CanonicalProductEmptyResponse DeleteCanonicalThreadUpload(1: thread_product.DeleteCanonicalThreadUploadRequest req) (api.delete="/api/workbench/threads/:thread_id/uploads/:file_id")
    thread_product.CanonicalArtifactListResponse ListCanonicalThreadArtifacts(1: thread_product.ListCanonicalThreadArtifactsRequest req) (api.get="/api/workbench/threads/:thread_id/artifacts")
    thread_product.CanonicalArtifactContentResponse GetCanonicalThreadArtifactContent(1: thread_product.GetCanonicalThreadArtifactContentRequest req) (api.get="/api/workbench/threads/:thread_id/artifacts/:artifact_id/content")
    thread_product.CanonicalArtifactSignedURLResponse GetCanonicalThreadArtifactSignedURL(1: thread_product.GetCanonicalThreadArtifactSignedURLRequest req) (api.get="/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url")
    thread_product.CanonicalProductEmptyResponse DeleteCanonicalThreadArtifact(1: thread_product.CanonicalArtifactRouteRequest req) (api.delete="/api/workbench/threads/:thread_id/artifacts/:artifact_id")
    thread_product.CanonicalArtifactRestoreResponse RestoreCanonicalThreadArtifact(1: thread_product.CanonicalArtifactRouteRequest req) (api.post="/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore")
    thread_product.CanonicalArtifactScanReviewResponse ReviewCanonicalThreadArtifactScan(1: thread_product.ReviewCanonicalThreadArtifactScanRequest req) (api.post="/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review")
    thread_product.CanonicalArtifactScanJobListResponse ListCanonicalThreadArtifactScanJobs(1: thread_product.ListCanonicalThreadArtifactScanJobsRequest req) (api.get="/api/workbench/threads/:thread_id/artifact_scan_jobs")
    thread_product.CanonicalArtifactScanJobRetryResponse RetryCanonicalThreadArtifactScanJob(1: thread_product.RetryCanonicalThreadArtifactScanJobRequest req) (api.post="/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry")
    thread_product.CanonicalTokenUsageResponse GetCanonicalThreadTokenUsage(1: thread_product.GetCanonicalThreadTokenUsageRequest req) (api.get="/api/workbench/threads/:thread_id/token_usage")
    thread_product.CanonicalMemoryListResponse ListCanonicalThreadMemories(1: thread_product.ListCanonicalThreadMemoriesRequest req) (api.get="/api/workbench/threads/:thread_id/memories")
    thread_product.CanonicalMemoryUpdateResponse UpdateCanonicalThreadMemory(1: thread_product.UpdateCanonicalThreadMemoryRequest req) (api.put="/api/workbench/threads/:thread_id/memories/:memory_id")
    thread_product.CanonicalProductEmptyResponse DeleteCanonicalThreadMemory(1: thread_product.CanonicalMemoryRouteRequest req) (api.delete="/api/workbench/threads/:thread_id/memories/:memory_id")
    thread_product.CanonicalMemoryRestoreResponse RestoreCanonicalThreadMemory(1: thread_product.CanonicalMemoryRouteRequest req) (api.post="/api/workbench/threads/:thread_id/memories/:memory_id/restore")
    thread_product.CanonicalMemoryClearResponse ClearCanonicalThreadMemories(1: thread_product.ClearCanonicalThreadMemoriesRequest req) (api.post="/api/workbench/threads/:thread_id/memories/clear")
    thread_product.CanonicalMemoryExportResponse ExportCanonicalThreadMemories(1: thread_product.ExportCanonicalThreadMemoriesRequest req) (api.get="/api/workbench/threads/:thread_id/memories/export")
    thread_product.CanonicalMemoryImportResponse ImportCanonicalThreadMemories(1: thread_product.ImportCanonicalThreadMemoriesRequest req) (api.post="/api/workbench/threads/:thread_id/memories/import")
    thread_product.CanonicalMemoryAuditEventListResponse ListCanonicalThreadMemoryAuditEvents(1: thread_product.ListCanonicalThreadMemoryAuditEventsRequest req) (api.get="/api/workbench/threads/:thread_id/memories/audit_events")
    thread_product.CanonicalGuardrailAuditEventListResponse ListCanonicalThreadGuardrailAuditEvents(1: thread_product.ListCanonicalThreadGuardrailAuditEventsRequest req) (api.get="/api/workbench/threads/:thread_id/guardrail_audit_events")
    thread_product.CanonicalGuardrailAuditExportResponse ExportCanonicalThreadGuardrailAuditEvents(1: thread_product.ExportCanonicalThreadGuardrailAuditEventsRequest req) (api.get="/api/workbench/threads/:thread_id/guardrail_audit_events/export")
    thread_product.CanonicalMCPRuntimeAuditEventListResponse ListCanonicalThreadMCPRuntimeAuditEvents(1: thread_product.ListCanonicalThreadMCPRuntimeAuditEventsRequest req) (api.get="/api/workbench/threads/:thread_id/mcp_runtime_audit_events")
    CanonicalRun RetryCanonicalSubagentRun(1: thread_product.RetryCanonicalSubagentRunRequest req) (api.post="/api/workbench/threads/:thread_id/runs/:run_id/retry")
    journal.JournalBootstrap GetCanonicalRunJournal(1: journal.GetCanonicalRunJournalRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/journal")
    journal.JournalSnapshotEnvelope GetCanonicalRunSnapshot(1: journal.GetCanonicalRunSnapshotRequest req) (api.get="/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id")
    journal.JournalSnapshotActionAuditResponse AuditCanonicalRunSnapshotAction(1: journal.AuditCanonicalRunSnapshotActionRequest req) (api.post="/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id/actions")
    journal.RecoverCanonicalRunJournalResponse RecoverCanonicalRunJournal(1: journal.RecoverCanonicalRunJournalRequest req) (api.post="/api/workbench/threads/:thread_id/runs/:run_id/recover")
    journal.JournalUserSettings GetCanonicalJournalSettings(1: journal.GetCanonicalJournalSettingsRequest req) (api.get="/api/workbench/journal/settings")
    journal.JournalUserSettings PatchCanonicalJournalSettings(1: journal.PatchCanonicalJournalSettingsRequest req) (api.patch="/api/workbench/journal/settings")
    thread_product.CopyCanonicalThreadArtifactLinkResponse CopyCanonicalThreadArtifactLink(1: thread_product.CopyCanonicalThreadArtifactLinkRequest req) (api.post="/api/workbench/threads/:thread_id/artifacts/:artifact_id/copy_link")
}
