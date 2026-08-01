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

export type WorkbenchJournalExecutionStatus =
  | 'pending'
  | 'running'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'timed_out';

export type WorkbenchJournalContentStatus =
  | 'empty'
  | 'loading'
  | 'streaming'
  | 'ready'
  | 'error'
  | 'no_permission';

export type WorkbenchJournalProjectionState =
  | 'healthy'
  | 'degraded'
  | 'disabled';

export type WorkbenchJournalContentType =
  | 'document'
  | 'terminal'
  | 'code'
  | 'skill'
  | 'browser';

export type WorkbenchJournalSnapshotAction =
  | 'copy_command'
  | 'copy_output'
  | 'copy_code'
  | 'open_original'
  | 'download_fragment';

export type WorkbenchJournalControlType =
  | 'journal_disabled'
  | 'journal_degraded'
  | 'capability_unavailable'
  | 'protocol_incompatible';

export type WorkbenchJournalErrorCode =
  | 'JOURNAL_CURSOR_EXPIRED'
  | 'JOURNAL_EVENT_GAP'
  | 'SNAPSHOT_UNAVAILABLE'
  | 'RESOURCE_NOT_FOUND'
  | 'RECOVERY_CONFLICT'
  | 'RECOVERY_CONFIRM_REQUIRED'
  | 'JOURNAL_RATE_LIMITED'
  | 'SCHEMA_INCOMPATIBLE'
  | 'NO_PERMISSION';

export interface WorkbenchJournalMilestoneEventData {
  milestone_id: string;
  title: string;
}

export interface WorkbenchJournalActionEventData {
  action_id: string;
  milestone_id?: string;
  operation: string;
  target: string;
  display_verb_running: string;
  display_verb_completed: string;
  content_type?: WorkbenchJournalContentType;
}

export interface WorkbenchJournalArtifactEventData {
  artifact_id: string;
  collection_id?: string;
}

export interface WorkbenchJournalVerificationEventData {
  verification_id: string;
  title: string;
  result_summary: string;
}

export interface WorkbenchJournalConfirmationEventData {
  confirmation_id: string;
  confirmation_type: string;
  prompt: string;
  allowed_action_keys: string[];
}

export type WorkbenchJournalEventPayload =
  | { type: 'milestone'; data: WorkbenchJournalMilestoneEventData }
  | {
      type: 'generic' | WorkbenchJournalContentType;
      data: WorkbenchJournalActionEventData;
    }
  | { type: 'artifact'; data: WorkbenchJournalArtifactEventData }
  | { type: 'verification'; data: WorkbenchJournalVerificationEventData }
  | { type: 'confirmation'; data: WorkbenchJournalConfirmationEventData }
  | Record<string, unknown>;

export interface WorkbenchJournalEvent {
  event_id: string;
  thread_id: string;
  run_id: string;
  event_type: string;
  payload: WorkbenchJournalEventPayload;
  created_at: number;
  schema_version?: string;
  attempt_id?: string;
  sequence?: number;
  idempotency_key?: string;
  parent_event_id?: string;
  status?: WorkbenchJournalExecutionStatus;
  occurred_at?: number;
  visibility?: 'user';
  payload_version?: string;
  snapshot_id?: string;
  trace_id?: string;
}

export interface WorkbenchJournalRecoveryCapability {
  allowed: boolean;
  requires_confirmation: boolean;
  allowed_actions: string[];
  reason_code?: string;
}

export interface WorkbenchJournalAttempt {
  attempt_id: string;
  run_id: string;
  status: WorkbenchJournalExecutionStatus;
  projection_state: WorkbenchJournalProjectionState;
  latest_sequence: number;
  created_at: number;
  started_at?: number;
  ended_at?: number;
  recovery_capability: WorkbenchJournalRecoveryCapability;
}

export interface WorkbenchJournalEventPage {
  items: WorkbenchJournalEvent[];
  has_more: boolean;
  next_after_event_id?: string;
  attempt_id?: string;
  latest_sequence?: number;
  next_after_sequence?: number;
}

export interface WorkbenchJournalEnrollment {
  enrolled: boolean;
  schema_version: string;
  payload_version: string;
  journal_protocol_version: string;
  journal_enabled: boolean;
  snapshots_enabled: boolean;
}

export interface WorkbenchJournalBootstrap {
  attempts: WorkbenchJournalAttempt[];
  default_attempt_id: string;
  default_attempt?: WorkbenchJournalAttempt;
  projection_state: WorkbenchJournalProjectionState;
  latest_sequence: number;
  events: WorkbenchJournalEventPage;
  content_types: WorkbenchJournalContentType[];
  enrollment: WorkbenchJournalEnrollment;
  submit_at: number;
  server_time: number;
  recovery_capability: WorkbenchJournalRecoveryCapability;
}

export interface WorkbenchJournalMetadata {
  thread_id: string;
  run_id: string;
  attempt_id: string;
  latest_sequence: number;
  submit_at: number;
  server_time: number;
  journal_enabled?: boolean;
  snapshots_enabled?: boolean;
  journal_protocol_version: string;
}

export interface WorkbenchJournalControl {
  type: WorkbenchJournalControlType;
  schema_version: string;
  journal_protocol_version: string;
  server_time: number;
  attempt_id?: string;
  latest_sequence?: number;
  error_code?: WorkbenchJournalErrorCode;
  retryable?: boolean;
}

export interface WorkbenchJournalHeartbeat {
  server_time: number;
  attempt_id: string;
  latest_sequence: number;
}

export type WorkbenchJournalStreamMessage =
  | { kind: 'metadata'; metadata: WorkbenchJournalMetadata }
  | { kind: 'event'; event: WorkbenchJournalEvent }
  | { kind: 'heartbeat'; heartbeat: WorkbenchJournalHeartbeat }
  | { kind: 'control'; control: WorkbenchJournalControl }
  | {
      kind: 'end';
      attempt_id: string;
      status: WorkbenchJournalExecutionStatus;
      latest_sequence: number;
    };

export interface WorkbenchJournalSkill {
  skill_id: string;
  name: string;
  invocation_status?: string;
  input_summary?: string;
  output_artifacts?: string[];
  purpose_summary?: string;
  description?: string;
}

export interface WorkbenchJournalDocumentChapter {
  chapter_id: string;
  title: string;
  level: number;
}

export interface WorkbenchJournalCodeHighlight {
  start_line: number;
  end_line: number;
  kind?: string;
}

export interface WorkbenchJournalSnapshotFragment {
  fragment_id: string;
  fragment_index: number;
  content?: string;
  byte_start?: number;
  byte_end?: number;
  size_bytes?: number;
  content_hash?: string;
  kind?:
    | 'document_block'
    | 'document_chapters'
    | 'terminal_stdout'
    | 'terminal_stderr'
    | 'code_lines'
    | 'code_highlights'
    | 'skill_items'
    | 'browser_thumbnail'
    | 'browser_snapshot'
    | 'browser_analysis';
  block_id?: string;
  stream?: string;
  start_line?: number;
  end_line?: number;
  item_start?: number;
  item_end?: number;
  binary_content_base64?: string;
  mime_type?: string;
  chapters?: WorkbenchJournalDocumentChapter[];
  highlights?: WorkbenchJournalCodeHighlight[];
  skills?: WorkbenchJournalSkill[];
  analysis?: string[];
}

export interface WorkbenchJournalDocumentContent {
  title: string;
  format?: string;
  content?: string;
  source_artifact_id?: string;
  token?: string;
  chapters?: WorkbenchJournalDocumentChapter[];
  active_block?: string;
  revision?: string;
  sync_status?: string;
}

export interface WorkbenchJournalTerminalContent {
  command: string;
  output?: string;
  exit_code?: number;
  working_directory?: string;
  session_id?: string;
  started_at?: number;
  finished_at?: number;
  stdout?: string;
  stderr?: string;
  duration_ms?: number;
}

export interface WorkbenchJournalCodeContent {
  file_path: string;
  language?: string;
  content?: string;
  diff?: string;
  start_line?: number;
  end_line?: number;
  repository: string;
  revision: string;
  highlights?: WorkbenchJournalCodeHighlight[];
}

export interface WorkbenchJournalSkillContent {
  skills: WorkbenchJournalSkill[];
}

export interface WorkbenchJournalBrowserContent {
  url?: string;
  title?: string;
  screenshot_artifact_id?: string;
  capture_id: string;
  thumbnail_base64?: string;
  static_snapshot_base64: string;
  mime_type: string;
  analysis?: string[];
  index?: number;
  total?: number;
  redacted: boolean;
  redaction_evidence_id: string;
  redaction_policy_version: string;
}

export interface WorkbenchJournalSnapshotContent {
  document?: WorkbenchJournalDocumentContent;
  terminal?: WorkbenchJournalTerminalContent;
  code?: WorkbenchJournalCodeContent;
  skill?: WorkbenchJournalSkillContent;
  browser?: WorkbenchJournalBrowserContent;
}

export interface WorkbenchJournalSnapshot {
  content_type: WorkbenchJournalContentType;
  snapshot_id: string;
  event_id: string;
  attempt_id: string;
  is_fragmented: boolean;
  status: WorkbenchJournalContentStatus;
  created_at: number;
  visibility: 'user';
  error_code?: string;
  fragments: WorkbenchJournalSnapshotFragment[];
  has_more: boolean;
  next_cursor?: string;
  content?: WorkbenchJournalSnapshotContent;
}

export interface WorkbenchJournalSnapshotActionResult {
  snapshot_id: string;
  action: WorkbenchJournalSnapshotAction;
  allowed: boolean;
  audited_at: number;
  copy_text?: string;
  download_url?: string;
  download_content_base64?: string;
  download_mime_type?: string;
}

export interface WorkbenchJournalRecoveryResult {
  attempt: WorkbenchJournalAttempt;
  accepted: boolean;
}

export interface WorkbenchJournalSettings {
  split_ratio: number;
  revision: string;
  updated_at?: number;
}
