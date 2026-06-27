namespace go workbench.task

include "../base.thrift"

enum TaskStatus {
    Created = 1,
    Queued = 2,
    Running = 3,
    Succeeded = 4,
    Failed = 5,
    Canceling = 6,
    Canceled = 7,
}

struct TaskEvent {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 task_id (agw.js_conv="str", api.js_conv="true")
    3: required string event_type
    4: optional string payload
    5: required i64 created_at
    6: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
}

struct ChatTask {
    1: required i64 id (agw.js_conv="str", api.js_conv="true")
    2: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 creator_id (agw.js_conv="str", api.js_conv="true")
    4: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    5: optional i64 message_id (agw.js_conv="str", api.js_conv="true")
    6: optional i64 skill_id (agw.js_conv="str", api.js_conv="true")
    7: required string title
    8: required TaskStatus status
    9: required i32 progress
    10: optional string input
    11: optional string result
    12: optional string error
    13: required i64 created_at
    14: required i64 updated_at
}

struct TaskThread {
    1: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 legacy_task_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    4: required i64 creator_id (agw.js_conv="str", api.js_conv="true")
    5: required string title
    6: required string status
    7: required string source
    8: required i32 progress
    9: required string last_user_message
    10: required string last_agent_message
    11: required i64 created_at
    12: required i64 updated_at
}

struct TaskThreadMessage {
    1: required i64 message_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    4: required string role
    5: required string content
    6: required string metadata
    7: required i64 created_at
}

struct TaskThreadRun {
    1: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 parent_run_id (agw.js_conv="str", api.js_conv="true")
    4: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    5: required i64 creator_id (agw.js_conv="str", api.js_conv="true")
    6: required string assistant_id
    7: required string run_kind
    8: required string status
    9: required string command
    10: required string input
    11: required string config
    12: required string context
    13: required string metadata
    14: required string stream_mode
    15: required string multitask_strategy
    16: required string on_disconnect
    17: required string durability
    18: required string idempotency_key
    19: required string worker_id
    20: required string error_code
    21: required string error_message
    22: required i64 started_at
    23: required i64 ended_at
    24: required i64 created_at
    25: required i64 updated_at
}

struct TaskThreadRunEvent {
    1: required i64 event_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    4: required string event_type
    5: required string payload
    6: required i64 created_at
}

struct TaskThreadTokenUsage {
    1: required i64 usage_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    4: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    5: required string source
    6: required string step_id
    7: required i32 step_index
    8: required string step_name
    9: required string model_name
    10: required string provider
    11: required i64 input_tokens
    12: required i64 output_tokens
    13: required i64 total_tokens
    14: required i64 cost_micros
    15: required string currency
    16: required bool estimated
    17: required string raw_usage
    18: required string metadata
    19: required i64 created_at
}

struct TaskThreadTokenUsageAggregate {
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

struct TaskThreadTokenUsageRunAggregate {
    1: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    2: required TaskThreadTokenUsageAggregate aggregate
}

struct TaskThreadArtifact {
    1: required i64 artifact_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    4: required string file_id
    5: required string title
    6: required string artifact_type
    7: required string virtual_path
    8: required string content_type
    9: required i64 size_bytes
    10: required string preview_mode
    11: required string metadata
    12: required i64 created_at
    13: required i64 updated_at
    14: required i64 deleted_at
}

struct CreateTaskRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: required string title
    3: optional i64 conversation_id (agw.js_conv="str", api.js_conv="true")
    4: optional i64 message_id (agw.js_conv="str", api.js_conv="true")
    5: optional i64 skill_id (agw.js_conv="str", api.js_conv="true")
    6: optional string input
    255: optional base.Base Base (api.none="true")
}

struct CreateTaskResponse {
    1: optional ChatTask data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTasksRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional TaskStatus status
    3: optional i32 page
    4: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListTasksData {
    1: required list<ChatTask> tasks
    2: required i64 total
}

struct ListTasksResponse {
    1: optional ListTasksData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetTaskRequest {
    1: required i64 task_id (api.path="task_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetTaskResponse {
    1: optional ChatTask data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTaskThreadsRequest {
    1: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    2: optional string status
    3: optional i32 page
    4: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListTaskThreadsData {
    1: required list<TaskThread> threads
    2: required i64 total
}

struct ListTaskThreadsResponse {
    1: optional ListTaskThreadsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetTaskThreadRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetTaskThreadResponse {
    1: optional TaskThread data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTaskThreadMessagesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i32 page
    3: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct AppendTaskThreadMessageRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: required string role
    4: required string content
    5: optional string metadata
    255: optional base.Base Base (api.none="true")
}

struct ListTaskThreadRunsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 parent_run_id (agw.js_conv="str", api.js_conv="true")
    3: optional string status
    4: optional i32 page
    5: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListTaskThreadRunEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: optional i32 page
    4: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct GetTaskThreadTokenUsageRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: optional bool include_child_runs
    4: optional string source
    5: optional i32 page
    6: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListTaskThreadArtifactsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: optional bool deleted_only
    4: optional i32 page
    5: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct GetTaskThreadArtifactSignedURLRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 artifact_id (api.path="artifact_id", agw.js_conv="str", api.js_conv="true")
    3: optional string mode
    4: optional i32 ttl_seconds
    255: optional base.Base Base (api.none="true")
}

struct CreateTaskThreadRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional string assistant_id
    3: optional string command
    4: required string input
    5: optional string config
    6: optional string context
    7: optional string metadata
    8: optional string stream_mode
    9: optional string multitask_strategy
    10: optional string on_disconnect
    11: optional string durability
    12: optional string idempotency_key
    255: optional base.Base Base (api.none="true")
}

struct HumanInteractionResponse {
    1: required string schema
    2: required string interaction_id
    3: required string kind
    4: required string decision
    5: optional string answer
    6: optional string choice_id
    7: optional string comment
    8: optional string submitted_by
    9: optional i64 submitted_at
    10: optional string source
}

struct ResumeTaskThreadRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: required string interrupt_id
    4: required HumanInteractionResponse response
    5: optional string idempotency_key
    255: optional base.Base Base (api.none="true")
}

struct CancelTaskThreadRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct RetryTaskThreadSubagentRunRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: optional string idempotency_key
    255: optional base.Base Base (api.none="true")
}

struct ListTaskThreadMessagesData {
    1: required list<TaskThreadMessage> messages
    2: required i64 total
}

struct ListTaskThreadMessagesResponse {
    1: optional ListTaskThreadMessagesData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct AppendTaskThreadMessageResponse {
    1: optional TaskThreadMessage data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTaskThreadRunsData {
    1: required list<TaskThreadRun> runs
    2: required i64 total
}

struct ListTaskThreadRunsResponse {
    1: optional ListTaskThreadRunsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTaskThreadRunEventsData {
    1: required list<TaskThreadRunEvent> events
    2: required i64 total
}

struct ListTaskThreadRunEventsResponse {
    1: optional ListTaskThreadRunEventsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetTaskThreadTokenUsageData {
    1: required list<TaskThreadTokenUsage> usage
    2: required i64 total
    3: required TaskThreadTokenUsageAggregate aggregate
    4: optional list<TaskThreadTokenUsageRunAggregate> run_aggregates
}

struct GetTaskThreadTokenUsageResponse {
    1: optional GetTaskThreadTokenUsageData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTaskThreadArtifactsData {
    1: required list<TaskThreadArtifact> artifacts
    2: required i64 total
}

struct ListTaskThreadArtifactsResponse {
    1: optional ListTaskThreadArtifactsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct GetTaskThreadArtifactSignedURLData {
    1: required i64 artifact_id (agw.js_conv="str", api.js_conv="true")
    2: required string url
    3: required i64 expires_in_seconds
    4: required string content_type
    5: required string preview_mode
}

struct GetTaskThreadArtifactSignedURLResponse {
    1: optional GetTaskThreadArtifactSignedURLData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct CreateTaskThreadRunResponse {
    1: optional TaskThreadRun data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ResumeTaskThreadRunResponse {
    1: optional TaskThreadRun data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct CancelTaskThreadRunResponse {
    1: optional TaskThreadRun data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct RetryTaskThreadSubagentRunResponse {
    1: optional TaskThreadRun data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct TaskEventsData {
    1: required list<TaskEvent> events
}

struct TaskEventsResponse {
    1: optional TaskEventsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct TaskThreadMemory {
    1: required i64 memory_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    4: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    5: required string scope
    6: required string content
    7: required string metadata
    8: required double score
    9: required double confidence
    10: required string source_type
    11: required string source_id
    12: required i64 correction_of_memory_id (agw.js_conv="str", api.js_conv="true")
    13: required i64 corrected_at
    14: required i64 expires_at
    15: required i64 created_at
    16: required i64 updated_at
    17: required i64 deleted_at
}

struct TaskThreadMemoryAuditEvent {
    1: required i64 event_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    4: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    5: required i64 memory_id (agw.js_conv="str", api.js_conv="true")
    6: required i64 actor_id (agw.js_conv="str", api.js_conv="true")
    7: required string event_type
    8: required string scope
    9: required string source_type
    10: required string source_id
    11: required i64 affected_count
    12: required i64 created_at
}

struct TaskThreadGuardrailAuditEvent {
    1: required i64 event_id (agw.js_conv="str", api.js_conv="true")
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 run_id (agw.js_conv="str", api.js_conv="true")
    4: required i64 space_id (agw.js_conv="str", api.js_conv="true")
    5: required i64 actor_id (agw.js_conv="str", api.js_conv="true")
    6: required string event_type
    7: required string target_type
    8: required string target_id
    9: required string operation
    10: required string source
    11: required string action
    12: required string fail_mode
    13: required string provider
    14: required string reason_code
    15: required string rule_ids
    16: required i64 created_at
}

struct ListTaskThreadMemoriesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: optional string scope
    4: optional list<string> scopes
    5: optional string q
    6: optional bool include_expired
    7: optional bool include_deleted
    8: optional i32 page
    9: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListTaskThreadMemoriesData {
    1: required list<TaskThreadMemory> memories
    2: required i64 total
}

struct ListTaskThreadMemoriesResponse {
    1: optional ListTaskThreadMemoriesData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ExportTaskThreadMemoriesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: optional string scope
    4: optional list<string> scopes
    5: optional string q
    6: optional bool include_expired
    7: optional bool include_deleted
    8: optional i32 limit
    255: optional base.Base Base (api.none="true")
}

struct ExportTaskThreadMemoriesData {
    1: required string schema
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 exported_at
    4: required i64 total
    5: required list<TaskThreadMemory> memories
}

struct ExportTaskThreadMemoriesResponse {
    1: optional ExportTaskThreadMemoriesData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct UpdateTaskThreadMemoryRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 memory_id (api.path="memory_id", agw.js_conv="str", api.js_conv="true")
    3: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    4: required string scope
    5: required string content
    6: optional string metadata
    7: optional double score
    8: optional double confidence
    9: optional string source_type
    10: optional string source_id
    11: optional i64 correction_of_memory_id (agw.js_conv="str", api.js_conv="true")
    12: optional i64 corrected_at
    13: optional i64 expires_at
    255: optional base.Base Base (api.none="true")
}

struct UpdateTaskThreadMemoryData {
    1: optional TaskThreadMemory memory
    2: required bool updated
}

struct UpdateTaskThreadMemoryResponse {
    1: optional UpdateTaskThreadMemoryData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ImportTaskThreadMemoryItem {
    1: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    2: optional string scope
    3: required string content
    4: optional string metadata
    5: optional double score
    6: optional double confidence
    7: optional string source_type
    8: optional string source_id
    9: optional i64 correction_of_memory_id (agw.js_conv="str", api.js_conv="true")
    10: optional i64 corrected_at
    11: optional i64 expires_at
}

struct ImportTaskThreadMemoriesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required list<ImportTaskThreadMemoryItem> memories
    255: optional base.Base Base (api.none="true")
}

struct ImportTaskThreadMemoriesData {
    1: required i64 imported
    2: required i64 skipped
    3: required list<TaskThreadMemory> memories
}

struct ImportTaskThreadMemoriesResponse {
    1: optional ImportTaskThreadMemoriesData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct DeleteTaskThreadMemoryRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 memory_id (api.path="memory_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct DeleteTaskThreadMemoryResponse {
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ClearTaskThreadMemoriesRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: optional list<string> scopes
    255: optional base.Base Base (api.none="true")
}

struct ClearTaskThreadMemoriesData {
    1: required i64 deleted
}

struct ClearTaskThreadMemoriesResponse {
    1: optional ClearTaskThreadMemoriesData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct RestoreTaskThreadMemoryRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 memory_id (api.path="memory_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct RestoreTaskThreadMemoryData {
    1: optional TaskThreadMemory memory
    2: required bool restored
}

struct RestoreTaskThreadMemoryResponse {
    1: optional RestoreTaskThreadMemoryData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTaskThreadMemoryAuditEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 memory_id (agw.js_conv="str", api.js_conv="true")
    3: optional i32 page
    4: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListTaskThreadMemoryAuditEventsData {
    1: required list<TaskThreadMemoryAuditEvent> events
    2: required i64 total
}

struct ListTaskThreadMemoryAuditEventsResponse {
    1: optional ListTaskThreadMemoryAuditEventsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ListTaskThreadGuardrailAuditEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: optional i32 page
    4: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ListTaskThreadGuardrailAuditEventsData {
    1: required list<TaskThreadGuardrailAuditEvent> events
    2: required i64 total
}

struct ListTaskThreadGuardrailAuditEventsResponse {
    1: optional ListTaskThreadGuardrailAuditEventsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

struct ExportTaskThreadGuardrailAuditEventsRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: optional i64 run_id (agw.js_conv="str", api.js_conv="true")
    3: optional i32 page
    4: optional i32 page_size
    255: optional base.Base Base (api.none="true")
}

struct ExportTaskThreadGuardrailAuditEventsData {
    1: required string schema
    2: required i64 thread_id (agw.js_conv="str", api.js_conv="true")
    3: required i64 exported_at
    4: required i32 page
    5: required i32 page_size
    6: required i64 total
    7: required list<TaskThreadGuardrailAuditEvent> events
}

struct ExportTaskThreadGuardrailAuditEventsResponse {
    1: optional ExportTaskThreadGuardrailAuditEventsData data
    253: required i64 code
    254: required string msg
    255: optional base.BaseResp BaseResp (api.none="true")
}

service WorkbenchTaskService {
    CreateTaskResponse CreateTask(1: CreateTaskRequest request)(
        api.post="/api/workbench/tasks",
        api.category="workbench"
    )
    ListTasksResponse ListTasks(1: ListTasksRequest request)(
        api.get="/api/workbench/tasks",
        api.category="workbench"
    )
    GetTaskResponse GetTask(1: GetTaskRequest request)(
        api.get="/api/workbench/tasks/:task_id",
        api.category="workbench"
    )
    ListTaskThreadsResponse ListTaskThreads(1: ListTaskThreadsRequest request)(
        api.get="/api/workbench/task_threads",
        api.category="workbench"
    )
    GetTaskThreadResponse GetTaskThread(1: GetTaskThreadRequest request)(
        api.get="/api/workbench/task_threads/:thread_id",
        api.category="workbench"
    )
    ListTaskThreadMessagesResponse ListTaskThreadMessages(1: ListTaskThreadMessagesRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/messages",
        api.category="workbench"
    )
    AppendTaskThreadMessageResponse AppendTaskThreadMessage(1: AppendTaskThreadMessageRequest request)(
        api.post="/api/workbench/task_threads/:thread_id/messages",
        api.category="workbench"
    )
    ListTaskThreadRunsResponse ListTaskThreadRuns(1: ListTaskThreadRunsRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/runs",
        api.category="workbench"
    )
    ListTaskThreadRunEventsResponse ListTaskThreadRunEvents(1: ListTaskThreadRunEventsRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/run_events",
        api.category="workbench"
    )
    GetTaskThreadTokenUsageResponse GetTaskThreadTokenUsage(1: GetTaskThreadTokenUsageRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/token_usage",
        api.category="workbench"
    )
    ListTaskThreadMemoriesResponse ListTaskThreadMemories(1: ListTaskThreadMemoriesRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/memories",
        api.category="workbench"
    )
    ExportTaskThreadMemoriesResponse ExportTaskThreadMemories(1: ExportTaskThreadMemoriesRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/memories/export",
        api.category="workbench"
    )
    ImportTaskThreadMemoriesResponse ImportTaskThreadMemories(1: ImportTaskThreadMemoriesRequest request)(
        api.post="/api/workbench/task_threads/:thread_id/memories/import",
        api.category="workbench"
    )
    UpdateTaskThreadMemoryResponse UpdateTaskThreadMemory(1: UpdateTaskThreadMemoryRequest request)(
        api.put="/api/workbench/task_threads/:thread_id/memories/:memory_id",
        api.category="workbench"
    )
    DeleteTaskThreadMemoryResponse DeleteTaskThreadMemory(1: DeleteTaskThreadMemoryRequest request)(
        api.delete="/api/workbench/task_threads/:thread_id/memories/:memory_id",
        api.category="workbench"
    )
    ClearTaskThreadMemoriesResponse ClearTaskThreadMemories(1: ClearTaskThreadMemoriesRequest request)(
        api.post="/api/workbench/task_threads/:thread_id/memories/clear",
        api.category="workbench"
    )
    RestoreTaskThreadMemoryResponse RestoreTaskThreadMemory(1: RestoreTaskThreadMemoryRequest request)(
        api.post="/api/workbench/task_threads/:thread_id/memories/:memory_id/restore",
        api.category="workbench"
    )
    ListTaskThreadMemoryAuditEventsResponse ListTaskThreadMemoryAuditEvents(1: ListTaskThreadMemoryAuditEventsRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/memories/audit_events",
        api.category="workbench"
    )
    ListTaskThreadGuardrailAuditEventsResponse ListTaskThreadGuardrailAuditEvents(1: ListTaskThreadGuardrailAuditEventsRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/guardrail_audit_events",
        api.category="workbench"
    )
    ExportTaskThreadGuardrailAuditEventsResponse ExportTaskThreadGuardrailAuditEvents(1: ExportTaskThreadGuardrailAuditEventsRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/guardrail_audit_events/export",
        api.category="workbench"
    )
    ListTaskThreadArtifactsResponse ListTaskThreadArtifacts(1: ListTaskThreadArtifactsRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/artifacts",
        api.category="workbench"
    )
    GetTaskThreadArtifactSignedURLResponse GetTaskThreadArtifactSignedURL(1: GetTaskThreadArtifactSignedURLRequest request)(
        api.get="/api/workbench/task_threads/:thread_id/artifacts/:artifact_id/signed_url",
        api.category="workbench"
    )
    CreateTaskThreadRunResponse CreateTaskThreadRun(1: CreateTaskThreadRunRequest request)(
        api.post="/api/workbench/task_threads/:thread_id/runs",
        api.category="workbench"
    )
    ResumeTaskThreadRunResponse ResumeTaskThreadRun(1: ResumeTaskThreadRunRequest request)(
        api.post="/api/workbench/task_threads/:thread_id/runs/:run_id/resume",
        api.category="workbench"
    )
    CancelTaskThreadRunResponse CancelTaskThreadRun(1: CancelTaskThreadRunRequest request)(
        api.post="/api/workbench/task_threads/:thread_id/runs/:run_id/cancel",
        api.category="workbench"
    )
    RetryTaskThreadSubagentRunResponse RetryTaskThreadSubagentRun(1: RetryTaskThreadSubagentRunRequest request)(
        api.post="/api/workbench/task_threads/:thread_id/runs/:run_id/retry",
        api.category="workbench"
    )
    GetTaskResponse CancelTask(1: GetTaskRequest request)(
        api.post="/api/workbench/tasks/:task_id/cancel",
        api.category="workbench"
    )
    GetTaskResponse RetryTask(1: GetTaskRequest request)(
        api.post="/api/workbench/tasks/:task_id/retry",
        api.category="workbench"
    )
    TaskEventsResponse ListTaskEvents(1: GetTaskRequest request)(
        api.get="/api/workbench/tasks/:task_id/events",
        api.category="workbench"
    )
}
