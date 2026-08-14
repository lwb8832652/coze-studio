/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import * as base from './../base';
export { base };
export const JOURNAL_SCHEMA_VERSION = "1.1";
export const JOURNAL_PAYLOAD_VERSION = "1.0";
export const JOURNAL_PROTOCOL_VERSION = "1.1";
export const JOURNAL_SPLIT_RATIO_MIN = 0.4;
export const JOURNAL_SPLIT_RATIO_MAX = 0.7;
export enum JournalExecutionStatus {
  Pending = "pending",
  Running = "running",
  Completed = "completed",
  Failed = "failed",
  Cancelled = "cancelled",
  TimedOut = "timed_out",
  Interrupted = "interrupted",
}
export enum JournalContentStatus {
  Empty = "empty",
  Loading = "loading",
  Streaming = "streaming",
  Ready = "ready",
  Error = "error",
  NoPermission = "no_permission",
}
export enum JournalProjectionState {
  Healthy = "healthy",
  Degraded = "degraded",
  Disabled = "disabled",
}
export enum JournalVisibility {
  User = "user",
}
export enum JournalMilestonePayloadType {
  Milestone = "milestone",
}
export enum JournalActionPayloadType {
  Generic = "generic",
  Document = "document",
  Terminal = "terminal",
  Code = "code",
  Skill = "skill",
  Browser = "browser",
}
export enum JournalArtifactPayloadType {
  Artifact = "artifact",
}
export enum JournalVerificationPayloadType {
  Verification = "verification",
}
export enum JournalConfirmationPayloadType {
  Confirmation = "confirmation",
}
export enum JournalSnapshotContentType {
  Document = "document",
  Terminal = "terminal",
  Code = "code",
  Skill = "skill",
  Browser = "browser",
}
export enum JournalSnapshotFragmentKind {
  DocumentBlock = "document_block",
  DocumentChapters = "document_chapters",
  TerminalStdout = "terminal_stdout",
  TerminalStderr = "terminal_stderr",
  CodeLines = "code_lines",
  CodeHighlights = "code_highlights",
  SkillItems = "skill_items",
  BrowserThumbnail = "browser_thumbnail",
  BrowserSnapshot = "browser_snapshot",
  BrowserAnalysis = "browser_analysis",
}
export enum JournalSnapshotAction {
  CopyCommand = "copy_command",
  CopyOutput = "copy_output",
  CopyCode = "copy_code",
  OpenOriginal = "open_original",
  DownloadFragment = "download_fragment",
}
export enum JournalErrorCode {
  JournalCursorExpired = "JOURNAL_CURSOR_EXPIRED",
  JournalEventGap = "JOURNAL_EVENT_GAP",
  SnapshotUnavailable = "SNAPSHOT_UNAVAILABLE",
  ResourceNotFound = "RESOURCE_NOT_FOUND",
  RecoveryConflict = "RECOVERY_CONFLICT",
  RecoveryConfirmRequired = "RECOVERY_CONFIRM_REQUIRED",
  JournalRateLimited = "JOURNAL_RATE_LIMITED",
  SchemaIncompatible = "SCHEMA_INCOMPATIBLE",
  NoPermission = "NO_PERMISSION",
}
export enum JournalControlFrameType {
  JournalDisabled = "journal_disabled",
  JournalDegraded = "journal_degraded",
  CapabilityUnavailable = "capability_unavailable",
  ProtocolIncompatible = "protocol_incompatible",
}
export enum JournalStreamFrameKind {
  Event = "event",
  Heartbeat = "heartbeat",
  Control = "control",
}
export interface JournalEvent {
  event_id: string,
  thread_id: string,
  run_id: string,
  event_type: string,
  payload: any,
  created_at: string,
  schema_version?: string,
  attempt_id?: string,
  sequence?: number,
  idempotency_key?: string,
  parent_event_id?: string,
  status?: JournalExecutionStatus,
  occurred_at?: string,
  visibility?: JournalVisibility,
  payload_version?: string,
  snapshot_id?: string,
  trace_id?: string,
}
export interface JournalMilestoneEventData {
  milestone_id: string,
  title: string,
}
export interface JournalActionEventData {
  action_id: string,
  milestone_id?: string,
  operation: string,
  target: string,
  display_verb_running: string,
  display_verb_completed: string,
  content_type?: JournalSnapshotContentType,
}
export interface JournalArtifactEventData {
  artifact_id: string,
  collection_id?: string,
}
export interface JournalVerificationEventData {
  verification_id: string,
  title: string,
  result_summary: string,
}
export interface JournalConfirmationEventData {
  confirmation_id: string,
  confirmation_type: string,
  prompt: string,
  allowed_action_keys: string[],
}
export interface JournalMilestoneEventPayload {
  type: JournalMilestonePayloadType,
  data: JournalMilestoneEventData,
}
export interface JournalActionEventPayload {
  type: JournalActionPayloadType,
  data: JournalActionEventData,
}
export interface JournalArtifactEventPayload {
  type: JournalArtifactPayloadType,
  data: JournalArtifactEventData,
}
export interface JournalVerificationEventPayload {
  type: JournalVerificationPayloadType,
  data: JournalVerificationEventData,
}
export interface JournalConfirmationEventPayload {
  type: JournalConfirmationPayloadType,
  data: JournalConfirmationEventData,
}
export interface JournalSkill {
  skill_id: string,
  name: string,
  invocation_status?: string,
  input_summary?: string,
  output_artifacts?: string[],
  purpose_summary?: string,
  description?: string,
}
export interface JournalSnapshotFragment {
  fragment_id: string,
  fragment_index: number,
  content?: string,
  byte_start?: number,
  byte_end?: number,
  size_bytes?: number,
  content_hash?: string,
  kind?: JournalSnapshotFragmentKind,
  block_id?: string,
  stream?: string,
  start_line?: number,
  end_line?: number,
  item_start?: number,
  item_end?: number,
  binary_content_base64?: string,
  mime_type?: string,
  chapters?: JournalDocumentChapter[],
  highlights?: JournalCodeHighlight[],
  skills?: JournalSkill[],
  analysis?: string[],
}
export interface JournalDocumentChapter {
  chapter_id: string,
  title: string,
  level: number,
}
export interface JournalDocumentSnapshotContent {
  title: string,
  format?: string,
  content?: string,
  source_artifact_id?: string,
  token?: string,
  chapters?: JournalDocumentChapter[],
  active_block?: string,
  revision?: string,
  sync_status?: string,
}
export interface JournalTerminalSnapshotContent {
  command: string,
  output?: string,
  exit_code?: number,
  working_directory?: string,
  session_id?: string,
  started_at?: string,
  finished_at?: string,
  stdout?: string,
  stderr?: string,
  duration_ms?: number,
}
export interface JournalCodeHighlight {
  start_line: number,
  end_line: number,
  kind?: string,
}
export interface JournalCodeSnapshotContent {
  file_path: string,
  language?: string,
  content?: string,
  diff?: string,
  start_line?: number,
  end_line?: number,
  repository: string,
  revision: string,
  highlights?: JournalCodeHighlight[],
}
export interface JournalSkillSnapshotContent {
  skills: JournalSkill[]
}
export interface JournalBrowserSnapshotContent {
  url?: string,
  title?: string,
  screenshot_artifact_id?: string,
  capture_id: string,
  thumbnail_base64?: string,
  static_snapshot_base64: string,
  mime_type: string,
  analysis?: string[],
  index?: number,
  total?: number,
  redacted: boolean,
  redaction_evidence_id: string,
  redaction_policy_version: string,
}
export interface JournalSnapshotContent {
  document?: JournalDocumentSnapshotContent,
  terminal?: JournalTerminalSnapshotContent,
  code?: JournalCodeSnapshotContent,
  skill?: JournalSkillSnapshotContent,
  browser?: JournalBrowserSnapshotContent,
}
export interface JournalSnapshotEnvelope {
  content_type: JournalSnapshotContentType,
  snapshot_id: string,
  event_id: string,
  attempt_id: string,
  is_fragmented: boolean,
  status: JournalContentStatus,
  created_at: string,
  visibility: JournalVisibility,
  error_code?: string,
  fragments: JournalSnapshotFragment[],
  has_more: boolean,
  next_cursor?: string,
  content?: JournalSnapshotContent,
}
export interface JournalRecoveryCapability {
  allowed: boolean,
  requires_confirmation: boolean,
  allowed_actions: string[],
  reason_code?: string,
}
export interface JournalAttemptSummary {
  attempt_id: string,
  run_id: string,
  status: JournalExecutionStatus,
  projection_state: JournalProjectionState,
  latest_sequence: number,
  created_at: string,
  started_at?: string,
  ended_at?: string,
  recovery_capability: JournalRecoveryCapability,
}
export interface JournalEventPage {
  data: JournalEvent[],
  has_more: boolean,
  next_after_event_id?: string,
  attempt_id?: string,
  latest_sequence?: number,
  next_after_sequence?: number,
}
export interface JournalEnrollment {
  enrolled: boolean,
  schema_version: string,
  payload_version: string,
  journal_protocol_version: string,
  journal_enabled: boolean,
  snapshots_enabled: boolean,
}
export interface JournalBootstrap {
  attempts: JournalAttemptSummary[],
  default_attempt_id: string,
  default_attempt?: JournalAttemptSummary,
  projection_state: JournalProjectionState,
  latest_sequence: number,
  events: JournalEventPage,
  content_types: JournalSnapshotContentType[],
  enrollment: JournalEnrollment,
  submit_at: string,
  server_time: string,
  recovery_capability: JournalRecoveryCapability,
}
export interface JournalStreamMetadata {
  thread_id: string,
  run_id: string,
  attempt_id: string,
  latest_sequence: number,
  submit_at: string,
  server_time: string,
  journal_enabled?: boolean,
  snapshots_enabled?: boolean,
  journal_protocol_version: string,
}
export interface JournalControlFrame {
  type: JournalControlFrameType,
  schema_version: string,
  journal_protocol_version: string,
  server_time: string,
  attempt_id?: string,
  latest_sequence?: number,
  error_code?: JournalErrorCode,
  retryable?: boolean,
}
export interface JournalHeartbeatFrame {
  server_time: string,
  attempt_id: string,
  latest_sequence: number,
}
export interface JournalEventStreamFrame {
  kind: JournalStreamFrameKind,
  event: JournalEvent,
}
export interface JournalHeartbeatStreamFrame {
  kind: JournalStreamFrameKind,
  heartbeat: JournalHeartbeatFrame,
}
export interface JournalControlStreamFrame {
  kind: JournalStreamFrameKind,
  control: JournalControlFrame,
}
export interface GetCanonicalRunJournalRequest {
  thread_id: string,
  run_id: string,
  "X-Coze-Space-ID": string,
  attempt_id?: string,
  after_sequence?: number,
  limit?: number,
  journal_protocol_version?: string,
  after_event_id?: string,
}
export interface GetCanonicalRunSnapshotRequest {
  thread_id: string,
  run_id: string,
  snapshot_id: string,
  "X-Coze-Space-ID": string,
  cursor?: string,
  limit?: number,
}
export interface AuditCanonicalRunSnapshotActionRequest {
  thread_id: string,
  run_id: string,
  snapshot_id: string,
  "X-Coze-Space-ID": string,
  action: JournalSnapshotAction,
  "Idempotency-Key": string,
  fragment_id?: string,
}
export interface JournalSnapshotActionAuditResponse {
  snapshot_id: string,
  action: JournalSnapshotAction,
  allowed: boolean,
  audited_at: string,
  copy_text?: string,
  download_url?: string,
  download_content_base64?: string,
  download_mime_type?: string,
}
export interface RecoverCanonicalRunJournalRequest {
  thread_id: string,
  run_id: string,
  "X-Coze-Space-ID": string,
  source_attempt_id?: string,
  action: string,
  confirmed?: boolean,
  "Idempotency-Key": string,
}
export interface RecoverCanonicalRunJournalResponse {
  attempt: JournalAttemptSummary,
  accepted: boolean,
}
export interface JournalUserSettings {
  split_ratio: number,
  revision: string,
  updated_at?: string,
}
export interface GetCanonicalJournalSettingsRequest {}
export interface PatchCanonicalJournalSettingsRequest {
  split_ratio: number,
  revision: string,
}
export interface JournalErrorResponse {
  detail: string,
  error_code: string,
  code: string,
  trace_id: string,
  retryable: boolean,
}
