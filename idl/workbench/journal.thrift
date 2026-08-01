namespace go workbench.journal_contract

include "../base.thrift"

const string JOURNAL_SCHEMA_VERSION = "1.1"
const string JOURNAL_PAYLOAD_VERSION = "1.0"
const string JOURNAL_PROTOCOL_VERSION = "1.1"
const double JOURNAL_SPLIT_RATIO_MIN = 0.40
const double JOURNAL_SPLIT_RATIO_MAX = 0.70

typedef string JournalExecutionStatus (ts.enum="true")
const JournalExecutionStatus JournalExecutionStatus_Pending = "pending"
const JournalExecutionStatus JournalExecutionStatus_Running = "running"
const JournalExecutionStatus JournalExecutionStatus_Completed = "completed"
const JournalExecutionStatus JournalExecutionStatus_Failed = "failed"
const JournalExecutionStatus JournalExecutionStatus_Cancelled = "cancelled"
const JournalExecutionStatus JournalExecutionStatus_TimedOut = "timed_out"

typedef string JournalContentStatus (ts.enum="true")
const JournalContentStatus JournalContentStatus_Empty = "empty"
const JournalContentStatus JournalContentStatus_Loading = "loading"
const JournalContentStatus JournalContentStatus_Streaming = "streaming"
const JournalContentStatus JournalContentStatus_Ready = "ready"
const JournalContentStatus JournalContentStatus_Error = "error"
const JournalContentStatus JournalContentStatus_NoPermission = "no_permission"

typedef string JournalProjectionState (ts.enum="true")
const JournalProjectionState JournalProjectionState_Healthy = "healthy"
const JournalProjectionState JournalProjectionState_Degraded = "degraded"
const JournalProjectionState JournalProjectionState_Disabled = "disabled"

typedef string JournalVisibility (ts.enum="true")
const JournalVisibility JournalVisibility_User = "user"

typedef string JournalMilestonePayloadType (ts.enum="true")
const JournalMilestonePayloadType JournalMilestonePayloadType_Milestone = "milestone"

typedef string JournalActionPayloadType (ts.enum="true")
const JournalActionPayloadType JournalActionPayloadType_Generic = "generic"
const JournalActionPayloadType JournalActionPayloadType_Document = "document"
const JournalActionPayloadType JournalActionPayloadType_Terminal = "terminal"
const JournalActionPayloadType JournalActionPayloadType_Code = "code"
const JournalActionPayloadType JournalActionPayloadType_Skill = "skill"
const JournalActionPayloadType JournalActionPayloadType_Browser = "browser"

typedef string JournalArtifactPayloadType (ts.enum="true")
const JournalArtifactPayloadType JournalArtifactPayloadType_Artifact = "artifact"

typedef string JournalVerificationPayloadType (ts.enum="true")
const JournalVerificationPayloadType JournalVerificationPayloadType_Verification = "verification"

typedef string JournalConfirmationPayloadType (ts.enum="true")
const JournalConfirmationPayloadType JournalConfirmationPayloadType_Confirmation = "confirmation"

typedef string JournalSnapshotContentType (ts.enum="true")
const JournalSnapshotContentType JournalSnapshotContentType_Document = "document"
const JournalSnapshotContentType JournalSnapshotContentType_Terminal = "terminal"
const JournalSnapshotContentType JournalSnapshotContentType_Code = "code"
const JournalSnapshotContentType JournalSnapshotContentType_Skill = "skill"
const JournalSnapshotContentType JournalSnapshotContentType_Browser = "browser"

typedef string JournalSnapshotFragmentKind (ts.enum="true")
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_DocumentBlock = "document_block"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_DocumentChapters = "document_chapters"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_TerminalStdout = "terminal_stdout"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_TerminalStderr = "terminal_stderr"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_CodeLines = "code_lines"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_CodeHighlights = "code_highlights"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_SkillItems = "skill_items"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_BrowserThumbnail = "browser_thumbnail"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_BrowserSnapshot = "browser_snapshot"
const JournalSnapshotFragmentKind JournalSnapshotFragmentKind_BrowserAnalysis = "browser_analysis"

typedef string JournalSnapshotAction (ts.enum="true")
const JournalSnapshotAction JournalSnapshotAction_CopyCommand = "copy_command"
const JournalSnapshotAction JournalSnapshotAction_CopyOutput = "copy_output"
const JournalSnapshotAction JournalSnapshotAction_CopyCode = "copy_code"
const JournalSnapshotAction JournalSnapshotAction_OpenOriginal = "open_original"
const JournalSnapshotAction JournalSnapshotAction_DownloadFragment = "download_fragment"

typedef string JournalErrorCode (ts.enum="true")
const JournalErrorCode JournalErrorCode_JournalCursorExpired = "JOURNAL_CURSOR_EXPIRED"
const JournalErrorCode JournalErrorCode_JournalEventGap = "JOURNAL_EVENT_GAP"
const JournalErrorCode JournalErrorCode_SnapshotUnavailable = "SNAPSHOT_UNAVAILABLE"
const JournalErrorCode JournalErrorCode_ResourceNotFound = "RESOURCE_NOT_FOUND"
const JournalErrorCode JournalErrorCode_RecoveryConflict = "RECOVERY_CONFLICT"
const JournalErrorCode JournalErrorCode_RecoveryConfirmRequired = "RECOVERY_CONFIRM_REQUIRED"
const JournalErrorCode JournalErrorCode_JournalRateLimited = "JOURNAL_RATE_LIMITED"
const JournalErrorCode JournalErrorCode_SchemaIncompatible = "SCHEMA_INCOMPATIBLE"
const JournalErrorCode JournalErrorCode_NoPermission = "NO_PERMISSION"

typedef string JournalControlFrameType (ts.enum="true")
const JournalControlFrameType JournalControlFrameType_JournalDisabled = "journal_disabled"
const JournalControlFrameType JournalControlFrameType_JournalDegraded = "journal_degraded"
const JournalControlFrameType JournalControlFrameType_CapabilityUnavailable = "capability_unavailable"
const JournalControlFrameType JournalControlFrameType_ProtocolIncompatible = "protocol_incompatible"

typedef string JournalStreamFrameKind (ts.enum="true")
const JournalStreamFrameKind JournalStreamFrameKind_Event = "event"
const JournalStreamFrameKind JournalStreamFrameKind_Heartbeat = "heartbeat"
const JournalStreamFrameKind JournalStreamFrameKind_Control = "control"

struct JournalEvent {
    1: required string event_id
    2: required string thread_id
    3: required string run_id
    4: required string event_type
    5: required string payload (api.value_type="any")
    6: required string created_at
    7: optional string schema_version
    8: optional string attempt_id
    9: optional i64 sequence
    10: optional string idempotency_key
    11: optional string parent_event_id
    12: optional JournalExecutionStatus status
    13: optional string occurred_at
    14: optional JournalVisibility visibility
    15: optional string payload_version
    16: optional string snapshot_id
    17: optional string trace_id
}

struct JournalMilestoneEventData {
    1: required string milestone_id
    2: required string title
}

struct JournalActionEventData {
    1: required string action_id
    2: optional string milestone_id
    3: required string operation
    4: required string target
    5: required string display_verb_running
    6: required string display_verb_completed
    7: optional JournalSnapshotContentType content_type
}

struct JournalArtifactEventData {
    1: required string artifact_id
    2: optional string collection_id
}

struct JournalVerificationEventData {
    1: required string verification_id
    2: required string title
    3: required string result_summary
}

struct JournalConfirmationEventData {
    1: required string confirmation_id
    2: required string confirmation_type
    3: required string prompt
    4: required list<string> allowed_action_keys
}

struct JournalMilestoneEventPayload {
    1: required JournalMilestonePayloadType type
    2: required JournalMilestoneEventData data
}

struct JournalActionEventPayload {
    1: required JournalActionPayloadType type
    2: required JournalActionEventData data
}

struct JournalArtifactEventPayload {
    1: required JournalArtifactPayloadType type
    2: required JournalArtifactEventData data
}

struct JournalVerificationEventPayload {
    1: required JournalVerificationPayloadType type
    2: required JournalVerificationEventData data
}

struct JournalConfirmationEventPayload {
    1: required JournalConfirmationPayloadType type
    2: required JournalConfirmationEventData data
}

struct JournalSkill {
    1: required string skill_id
    2: required string name
    3: optional string invocation_status
    4: optional string input_summary
    5: optional list<string> output_artifacts
    6: optional string purpose_summary
    7: optional string description
}

struct JournalSnapshotFragment {
    1: required string fragment_id
    2: required i32 fragment_index
    3: optional string content
    4: optional i64 byte_start
    5: optional i64 byte_end
    6: optional i64 size_bytes
    7: optional string content_hash
    8: optional JournalSnapshotFragmentKind kind
    9: optional string block_id
    10: optional string stream
    11: optional i32 start_line
    12: optional i32 end_line
    13: optional i32 item_start
    14: optional i32 item_end
    15: optional string binary_content_base64
    16: optional string mime_type
    17: optional list<JournalDocumentChapter> chapters
    18: optional list<JournalCodeHighlight> highlights
    19: optional list<JournalSkill> skills
    20: optional list<string> analysis
}

struct JournalDocumentChapter {
    1: required string chapter_id
    2: required string title
    3: required i32 level
}

struct JournalDocumentSnapshotContent {
    1: required string title
    2: optional string format
    3: optional string content
    4: optional string source_artifact_id
    5: optional string token
    6: optional list<JournalDocumentChapter> chapters
    7: optional string active_block
    8: optional string revision
    9: optional string sync_status
}

struct JournalTerminalSnapshotContent {
    1: required string command
    2: optional string output
    3: optional i32 exit_code
    4: optional string working_directory
    5: optional string session_id
    6: optional string started_at
    7: optional string finished_at
    8: optional string stdout
    9: optional string stderr
    10: optional i64 duration_ms
}

struct JournalCodeHighlight {
    1: required i32 start_line
    2: required i32 end_line
    3: optional string kind
}

struct JournalCodeSnapshotContent {
    1: required string file_path
    2: optional string language
    3: optional string content
    4: optional string diff
    5: optional i32 start_line
    6: optional i32 end_line
    7: required string repository
    8: required string revision
    9: optional list<JournalCodeHighlight> highlights
}

struct JournalSkillSnapshotContent {
    1: required list<JournalSkill> skills
}

struct JournalBrowserSnapshotContent {
    1: optional string url
    2: optional string title
    4: optional string screenshot_artifact_id
    5: required string capture_id
    6: optional string thumbnail_base64
    7: required string static_snapshot_base64
    8: required string mime_type
    9: optional list<string> analysis
    10: optional i32 index
    11: optional i32 total
    12: required bool redacted
    13: required string redaction_evidence_id
    14: required string redaction_policy_version
}

union JournalSnapshotContent {
    1: JournalDocumentSnapshotContent document
    2: JournalTerminalSnapshotContent terminal
    3: JournalCodeSnapshotContent code
    4: JournalSkillSnapshotContent skill
    5: JournalBrowserSnapshotContent browser
}

struct JournalSnapshotEnvelope {
    1: required JournalSnapshotContentType content_type
    2: required string snapshot_id
    3: required string event_id
    4: required string attempt_id
    5: required bool is_fragmented
    6: required JournalContentStatus status
    7: required string created_at
    8: required JournalVisibility visibility
    9: optional string error_code
    10: required list<JournalSnapshotFragment> fragments
    11: required bool has_more
    12: optional string next_cursor
    13: optional JournalSnapshotContent content
}

struct JournalRecoveryCapability {
    1: required bool allowed
    2: required bool requires_confirmation
    3: required list<string> allowed_actions
    4: optional string reason_code
}

struct JournalAttemptSummary {
    1: required string attempt_id
    2: required string run_id
    3: required JournalExecutionStatus status
    4: required JournalProjectionState projection_state
    5: required i64 latest_sequence
    6: required string created_at
    7: optional string started_at
    8: optional string ended_at
    9: required JournalRecoveryCapability recovery_capability
}

struct JournalEventPage {
    1: required list<JournalEvent> data
    2: required bool has_more
    3: optional string next_after_event_id
    4: optional string attempt_id
    5: optional i64 latest_sequence
    6: optional i64 next_after_sequence
}

struct JournalEnrollment {
    1: required bool enrolled
    2: required string schema_version
    3: required string payload_version
    4: required string journal_protocol_version
    5: required bool journal_enabled
    6: required bool snapshots_enabled
}

struct JournalBootstrap {
    1: required list<JournalAttemptSummary> attempts
    2: required string default_attempt_id
    3: optional JournalAttemptSummary default_attempt
    4: required JournalProjectionState projection_state
    5: required i64 latest_sequence
    6: required JournalEventPage events
    7: required list<JournalSnapshotContentType> content_types
    8: required JournalEnrollment enrollment
    9: required string submit_at
    10: required string server_time
    11: required JournalRecoveryCapability recovery_capability
}

struct JournalStreamMetadata {
    1: required string thread_id
    2: required string run_id
    3: required string attempt_id
    4: required i64 latest_sequence
    5: required string submit_at
    6: required string server_time
    7: optional bool journal_enabled
    8: optional bool snapshots_enabled
    9: required string journal_protocol_version
}

struct JournalControlFrame {
    1: required JournalControlFrameType type
    2: required string schema_version
    3: required string journal_protocol_version
    4: required string server_time
    5: optional string attempt_id
    6: optional i64 latest_sequence
    7: optional JournalErrorCode error_code
    8: optional bool retryable
}

struct JournalHeartbeatFrame {
    1: required string server_time
    2: required string attempt_id
    3: required i64 latest_sequence
}

struct JournalEventStreamFrame {
    1: required JournalStreamFrameKind kind
    2: required JournalEvent event
}

struct JournalHeartbeatStreamFrame {
    1: required JournalStreamFrameKind kind
    2: required JournalHeartbeatFrame heartbeat
}

struct JournalControlStreamFrame {
    1: required JournalStreamFrameKind kind
    2: required JournalControlFrame control
}

struct GetCanonicalRunJournalRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    4: optional string attempt_id (api.query="attempt_id")
    5: optional i64 after_sequence (api.query="after_sequence")
    6: optional i32 limit (api.query="limit")
    7: optional string journal_protocol_version (api.query="journal_protocol_version")
    8: optional i64 after_event_id (api.query="after_event_id", agw.js_conv="str", api.js_conv="true")
    255: optional base.Base Base (api.none="true")
}

struct GetCanonicalRunSnapshotRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: required string snapshot_id (api.path="snapshot_id")
    4: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    5: optional string cursor (api.query="cursor")
    6: optional i32 limit (api.query="limit")
    255: optional base.Base Base (api.none="true")
}

struct AuditCanonicalRunSnapshotActionRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: required string snapshot_id (api.path="snapshot_id")
    4: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    5: required JournalSnapshotAction action (api.body="action")
    6: required string idempotency_key (api.header="Idempotency-Key")
    7: optional string fragment_id (api.body="fragment_id")
    255: optional base.Base Base (api.none="true")
}

struct JournalSnapshotActionAuditResponse {
    1: required string snapshot_id
    2: required JournalSnapshotAction action
    3: required bool allowed
    4: required string audited_at
    5: optional string copy_text
    6: optional string download_url
    7: optional string download_content_base64
    8: optional string download_mime_type
}

struct RecoverCanonicalRunJournalRequest {
    1: required i64 thread_id (api.path="thread_id", agw.js_conv="str", api.js_conv="true")
    2: required i64 run_id (api.path="run_id", agw.js_conv="str", api.js_conv="true")
    3: required i64 space_id (api.header="X-Coze-Space-ID", agw.js_conv="str", api.js_conv="true")
    4: optional string source_attempt_id (api.body="source_attempt_id")
    5: required string action (api.body="action")
    6: optional bool confirmed (api.body="confirmed")
    7: required string idempotency_key (api.header="Idempotency-Key")
    255: optional base.Base Base (api.none="true")
}

struct RecoverCanonicalRunJournalResponse {
    1: required JournalAttemptSummary attempt
    2: required bool accepted
}

struct JournalUserSettings {
    1: required double split_ratio
    2: required string revision
    3: optional string updated_at
}

struct GetCanonicalJournalSettingsRequest {
    255: optional base.Base Base (api.none="true")
}

struct PatchCanonicalJournalSettingsRequest {
    1: required double split_ratio (api.body="split_ratio")
    2: required string revision (api.body="revision")
    255: optional base.Base Base (api.none="true")
}

struct JournalErrorResponse {
    1: required string detail
    2: required string error_code
    3: required string code
    4: required string trace_id
    5: required bool retryable
}
