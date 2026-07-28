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

export interface WorkbenchTodo {
  id: string;
  title: string;
  status: string;
}

export interface WorkbenchThreadValues {
  todos?: WorkbenchTodo[];
}

export interface WorkbenchThread {
  thread_id: string;
  space_id: string;
  title: string;
  status: string;
  source: string;
  progress: number;
  last_user_message: string;
  last_agent_message: string;
  can_edit: boolean;
  created_at: number;
  updated_at: number;
  values?: WorkbenchThreadValues;
}

export interface WorkbenchMessage {
  message_id: string;
  thread_id: string;
  run_id: string;
  role: string;
  content: string;
  metadata: string;
  created_at: number;
}

export interface WorkbenchSuggestionMessage {
  role: string;
  content: string;
}

export interface WorkbenchRun {
  run_id: string;
  thread_id: string;
  space_id: string;
  assistant_id: string;
  status: string;
  metadata: string;
  multitask_strategy: string;
  message_id?: string;
  attempt_kind: string;
  source_run_id?: string;
  parent_run_id?: string;
  run_kind: string;
  stream_modes: string[];
  on_disconnect: string;
  durability: string;
  terminal_reason?: string;
  started_at?: number;
  ended_at?: number;
  created_at: number;
  updated_at: number;
}

export interface WorkbenchRunEvent {
  event_id: string;
  thread_id: string;
  run_id: string;
  event_type: string;
  payload: string;
  created_at: number;
}

export interface WorkbenchUpload {
  file_id: string;
  file_name: string;
  virtual_path: string;
  content_type: string;
  size_bytes: number;
  created_at: number;
}

export interface WorkbenchArtifact {
  artifact_id: string;
  thread_id: string;
  run_id: string;
  file_id: string;
  title: string;
  artifact_type: string;
  virtual_path: string;
  content_type: string;
  size_bytes: number;
  preview_mode: string;
  metadata: string;
  created_at: number;
  updated_at: number;
  deleted_at?: number;
}

export interface WorkbenchArtifactScanJob {
  job_id: string;
  thread_id: string;
  run_id: string;
  space_id: string;
  artifact_id: string;
  file_id: string;
  scanner: string;
  status: string;
  worker_id: string;
  attempt_count: number;
  error_code: string;
  available_at?: number;
  started_at?: number;
  ended_at?: number;
  created_at: number;
  updated_at: number;
}

export interface WorkbenchTokenUsage {
  usage_id: string;
  thread_id: string;
  run_id: string;
  space_id: string;
  source: string;
  step_id: string;
  step_index: number;
  step_name: string;
  model_name: string;
  provider: string;
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cost_micros: number;
  currency: string;
  estimated: boolean;
  created_at: number;
}

export interface WorkbenchTokenUsageAggregate {
  input_tokens: number;
  output_tokens: number;
  total_tokens: number;
  cost_micros: number;
  call_count: number;
  lead_agent_tokens: number;
  subagent_tokens: number;
  middleware_tokens: number;
  tool_tokens: number;
}

export interface WorkbenchRunTokenUsageAggregate {
  run_id: string;
  aggregate: WorkbenchTokenUsageAggregate;
}

export interface WorkbenchMemory {
  memory_id: string;
  thread_id: string;
  run_id?: string;
  space_id: string;
  scope: string;
  content: string;
  metadata: string;
  score: number;
  confidence: number;
  source_type: string;
  source_id: string;
  correction_of_memory_id?: string;
  corrected_at?: number;
  expires_at?: number;
  created_at: number;
  updated_at: number;
  deleted_at?: number;
}

export interface WorkbenchMemoryAuditEvent {
  event_id: string;
  thread_id: string;
  run_id?: string;
  space_id: string;
  memory_id?: string;
  actor_id?: string;
  event_type: string;
  scope: string;
  source_type: string;
  source_id: string;
  affected_count: number;
  created_at: number;
}

export interface WorkbenchGuardrailAuditEvent {
  event_id: string;
  thread_id: string;
  run_id?: string;
  space_id: string;
  actor_id?: string;
  event_type: string;
  target_type: string;
  target_id: string;
  operation: string;
  source: string;
  action: string;
  fail_mode: string;
  provider: string;
  reason_code: string;
  rule_ids: string;
  created_at: number;
}

export interface WorkbenchMCPRuntimeAuditEvent {
  event_id: string;
  space_id: string;
  thread_id: string;
  run_id?: string;
  server_id?: string;
  runtime_tool_name: string;
  event_type: string;
  error_code: string;
  elapsed_millis: number;
  output_bytes: number;
  created_at: number;
}

export interface HumanInteractionResponse {
  schema: string;
  interaction_id: string;
  kind: string;
  decision: string;
  answer?: string;
  choice_id?: string;
  comment?: string;
  submitted_by?: string;
  submitted_at?: number;
  source?: string;
}

export interface WorkbenchThreadCreation {
  thread: WorkbenchThread;
  message?: WorkbenchMessage;
  run?: WorkbenchRun;
}

export interface WorkbenchRunCreation {
  run: WorkbenchRun;
  message?: WorkbenchMessage;
}

export interface WorkbenchUploadCreation {
  uploads: WorkbenchUpload[];
  skipped_files: string[];
}

export interface WorkbenchArtifactContent {
  blob: Blob;
  content_disposition: string;
  content_type: string;
}

export interface WorkbenchArtifactSignedURL {
  artifact_id: string;
  url: string;
  expires_in_seconds: number;
  content_type: string;
  preview_mode: string;
}

export interface WorkbenchArtifactRestoreResult {
  artifact: WorkbenchArtifact;
  restored: boolean;
}

export type WorkbenchArtifactScanDecision = 'release' | 'quarantine' | 'block';

export interface WorkbenchArtifactScanReviewResult {
  artifact_id: string;
  decision: WorkbenchArtifactScanDecision;
  scan_status: string;
  reviewed: boolean;
}

export interface WorkbenchArtifactScanRetryResult {
  job: WorkbenchArtifactScanJob;
  retried: boolean;
}

export interface WorkbenchMemoryUpdateResult {
  memory: WorkbenchMemory;
  updated: boolean;
}

export interface WorkbenchMemoryRestoreResult {
  memory: WorkbenchMemory;
  restored: boolean;
}

export interface WorkbenchMemoryClearResult {
  deleted: number;
}

export interface WorkbenchMemoryImportResult {
  imported: number;
  skipped: number;
  memories: WorkbenchMemory[];
}

export interface WorkbenchMemoryExport {
  schema: string;
  thread_id: string;
  exported_at: number;
  total: number;
  memories: WorkbenchMemory[];
}

export interface WorkbenchGuardrailAuditExport {
  schema: string;
  thread_id: string;
  exported_at: number;
  total: number;
  events: WorkbenchGuardrailAuditEvent[];
}
