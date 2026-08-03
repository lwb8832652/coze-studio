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
export enum CanonicalArtifactSource {
  AgentGenerated = "agent_generated",
  UserUpload = "user_upload",
  ToolOutput = "tool_output",
  ExternalReference = "external_reference",
}
export enum CanonicalArtifactGenerationStatus {
  Processing = "processing",
  Ready = "ready",
  Failed = "failed",
  Expired = "expired",
  Blocked = "blocked",
}
export enum CanonicalArtifactPreviewMode {
  Text = "text",
  Image = "image",
  PDF = "pdf",
  Audio = "audio",
  Video = "video",
  MediaCollection = "media_collection",
  Download = "download",
  Unsupported = "unsupported",
}
export enum CanonicalArtifactCapability {
  Open = "open",
  Preview = "preview",
  Download = "download",
  Copy = "copy",
}
export interface CanonicalProductEmptyResponse {}
export interface CanonicalProductThreadRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
}
export interface CanonicalProductPageRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  limit?: number,
  offset?: number,
}
export interface CanonicalUploadFile {
  file_id: string,
  file_name: string,
  virtual_path: string,
  content_type: string,
  size_bytes: number,
  created_at: string,
}
export interface CanonicalArtifact {
  artifact_id: string,
  thread_id: string,
  run_id: string,
  file_id: string,
  title: string,
  artifact_type: string,
  virtual_path: string,
  content_type: string,
  size_bytes: number,
  preview_mode: CanonicalArtifactPreviewMode,
  metadata: any,
  created_at: string,
  updated_at: string,
  deleted_at?: string,
  source?: CanonicalArtifactSource,
  generation_status?: CanonicalArtifactGenerationStatus,
  capabilities?: CanonicalArtifactCapability[],
  collection_id?: string,
  collection_order?: number,
  is_primary?: boolean,
}
export interface CanonicalArtifactCollection {
  collection_id: string,
  artifact_ids: string[],
  current_index?: number,
  total_count?: number,
}
export interface CanonicalArtifactScanJob {
  job_id: string,
  thread_id: string,
  run_id: string,
  artifact_id: string,
  file_id: string,
  scanner: string,
  status: string,
  worker_ref: string,
  attempt_count: number,
  error_code: string,
  available_at?: string,
  started_at?: string,
  ended_at?: string,
  created_at: string,
  updated_at: string,
}
export interface CanonicalTokenUsage {
  usage_id: string,
  thread_id: string,
  run_id: string,
  source: string,
  step_id: string,
  step_index: number,
  step_name: string,
  model_name: string,
  provider: string,
  input_tokens: number,
  output_tokens: number,
  total_tokens: number,
  cost_micros: number,
  currency: string,
  estimated: boolean,
  created_at: string,
}
export interface CanonicalTokenUsageAggregate {
  input_tokens: number,
  output_tokens: number,
  total_tokens: number,
  cost_micros: number,
  call_count: number,
  lead_agent_tokens: number,
  subagent_tokens: number,
  middleware_tokens: number,
  tool_tokens: number,
}
export interface CanonicalRunTokenUsageAggregate {
  run_id: string,
  aggregate: CanonicalTokenUsageAggregate,
}
export interface CanonicalMemory {
  memory_id: string,
  thread_id: string,
  run_id?: string,
  scope: string,
  content: string,
  metadata: any,
  score: number,
  confidence: number,
  source_type: string,
  source_id: string,
  correction_of_memory_id?: string,
  corrected_at?: string,
  expires_at?: string,
  created_at: string,
  updated_at: string,
  deleted_at?: string,
}
export interface CanonicalMemoryAuditEvent {
  event_id: string,
  thread_id: string,
  run_id?: string,
  memory_id?: string,
  actor_id?: string,
  event_type: string,
  scope: string,
  source_type: string,
  source_id: string,
  affected_count: number,
  created_at: string,
}
export interface CanonicalGuardrailAuditEvent {
  event_id: string,
  thread_id: string,
  run_id?: string,
  actor_id?: string,
  event_type: string,
  target_type: string,
  target_id: string,
  operation: string,
  source: string,
  action: string,
  fail_mode: string,
  provider: string,
  reason_code: string,
  rule_ids: string[],
  created_at: string,
}
export interface CanonicalMCPRuntimeAuditEvent {
  event_id: string,
  thread_id: string,
  run_id?: string,
  server_id?: string,
  runtime_tool_name: string,
  event_type: string,
  error_code: string,
  elapsed_millis: number,
  output_bytes: number,
  created_at: string,
}
export interface CanonicalUploadListResponse {
  uploads: CanonicalUploadFile[],
  total: number,
  has_more: boolean,
  next_cursor?: string,
}
export interface CanonicalArtifactListResponse {
  artifacts: CanonicalArtifact[],
  total: number,
  has_more: boolean,
  next_cursor?: string,
  collections?: CanonicalArtifactCollection[],
}
export interface CanonicalArtifactScanJobListResponse {
  jobs: CanonicalArtifactScanJob[],
  total: number,
  has_more: boolean,
  next_cursor?: string,
}
export interface CanonicalMemoryListResponse {
  memories: CanonicalMemory[],
  total: number,
  has_more: boolean,
  next_cursor?: string,
}
export interface CanonicalMemoryAuditEventListResponse {
  events: CanonicalMemoryAuditEvent[],
  total: number,
  has_more: boolean,
  next_cursor?: string,
}
export interface CanonicalGuardrailAuditEventListResponse {
  events: CanonicalGuardrailAuditEvent[],
  total: number,
  has_more: boolean,
  next_cursor?: string,
}
export interface CanonicalMCPRuntimeAuditEventListResponse {
  events: CanonicalMCPRuntimeAuditEvent[],
  total: number,
  has_more: boolean,
  next_cursor?: string,
}
export interface CanonicalTokenUsageResponse {
  usage: CanonicalTokenUsage[],
  total: number,
  has_more: boolean,
  next_cursor?: string,
  aggregate: CanonicalTokenUsageAggregate,
  run_aggregates: CanonicalRunTokenUsageAggregate[],
}
export interface CanonicalSuggestionResponse {
  suggestions: string[]
}
export interface CanonicalUploadResponse {
  uploads: CanonicalUploadFile[],
  skipped_files: string[],
}
export interface CanonicalArtifactContentResponse {
  body: Blob
}
export interface CanonicalArtifactSignedURLResponse {
  artifact_id: string,
  url: string,
  expires_in_seconds: number,
  content_type: string,
  preview_mode: CanonicalArtifactPreviewMode,
}
export interface CanonicalArtifactRestoreResponse {
  artifact: CanonicalArtifact,
  restored: boolean,
}
export interface CanonicalArtifactScanReviewResponse {
  artifact_id: string,
  decision: string,
  scan_status: string,
  reviewed: boolean,
}
export interface CanonicalArtifactScanJobRetryResponse {
  job: CanonicalArtifactScanJob,
  retried: boolean,
}
export interface CanonicalMemoryUpdateResponse {
  memory: CanonicalMemory,
  updated: boolean,
}
export interface CanonicalMemoryRestoreResponse {
  memory: CanonicalMemory,
  restored: boolean,
}
export interface CanonicalMemoryClearResponse {
  deleted: number
}
export interface CanonicalMemoryImportResponse {
  imported: number,
  skipped: number,
  memories: CanonicalMemory[],
}
export interface CanonicalMemoryExportResponse {
  schema: string,
  thread_id: string,
  exported_at: string,
  total: number,
  memories: CanonicalMemory[],
}
export interface CanonicalGuardrailAuditExportResponse {
  schema: string,
  thread_id: string,
  exported_at: string,
  total: number,
  events: CanonicalGuardrailAuditEvent[],
}
export interface AppendCanonicalThreadMessageRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  role: string,
  content: string,
  metadata?: any,
  append_mode: string,
}
export interface GenerateCanonicalThreadSuggestionsRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  n?: number,
  model_name?: string,
  model_type?: string,
}
export interface UploadCanonicalThreadFilesRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
}
export interface DeleteCanonicalThreadUploadRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  file_id: string,
}
export interface ListCanonicalThreadArtifactsRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  deleted_only?: boolean,
  limit?: number,
  offset?: number,
  collection_id?: string,
}
export interface CopyCanonicalThreadArtifactLinkRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  artifact_id: string,
}
export interface CopyCanonicalThreadArtifactLinkResponse {
  artifact_id: string,
  copy_url: string,
  expires_at: string,
}
export interface CanonicalArtifactRouteRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  artifact_id: string,
}
export interface GetCanonicalThreadArtifactContentRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  artifact_id: string,
  mode?: string,
}
export interface GetCanonicalThreadArtifactSignedURLRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  artifact_id: string,
  mode?: string,
  ttl_seconds?: number,
}
export interface ReviewCanonicalThreadArtifactScanRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  artifact_id: string,
  decision: string,
  reason?: string,
}
export interface ListCanonicalThreadArtifactScanJobsRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  artifact_id?: string,
  status?: string,
  scanner?: string,
  limit?: number,
  offset?: number,
}
export interface RetryCanonicalThreadArtifactScanJobRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  job_id: string,
}
export interface GetCanonicalThreadTokenUsageRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  include_child_runs?: boolean,
  source?: string,
  limit?: number,
  offset?: number,
}
export interface ListCanonicalThreadMemoriesRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  scope?: string,
  scopes?: string[],
  q?: string,
  include_expired?: boolean,
  include_deleted?: boolean,
  limit?: number,
  offset?: number,
}
export interface UpdateCanonicalThreadMemoryRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  memory_id: string,
  run_id?: string,
  scope: string,
  content: string,
  metadata?: any,
  score?: number,
  confidence?: number,
  source_type?: string,
  source_id?: string,
  correction_of_memory_id?: string,
  corrected_at?: string,
  expires_at?: string,
}
export interface CanonicalMemoryRouteRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  memory_id: string,
}
export interface ClearCanonicalThreadMemoriesRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  scopes?: string[],
}
export interface ExportCanonicalThreadMemoriesRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  scope?: string,
  scopes?: string[],
  q?: string,
  include_expired?: boolean,
  include_deleted?: boolean,
  limit?: number,
}
export interface CanonicalImportMemoryItem {
  run_id?: string,
  scope?: string,
  content: string,
  metadata?: any,
  score?: number,
  confidence?: number,
  source_type?: string,
  source_id?: string,
  correction_of_memory_id?: string,
  corrected_at?: string,
  expires_at?: string,
}
export interface ImportCanonicalThreadMemoriesRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  memories: CanonicalImportMemoryItem[],
}
export interface ListCanonicalThreadMemoryAuditEventsRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  memory_id?: string,
  limit?: number,
  offset?: number,
}
export interface ListCanonicalThreadGuardrailAuditEventsRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  limit?: number,
  offset?: number,
}
export interface ExportCanonicalThreadGuardrailAuditEventsRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  limit?: number,
  offset?: number,
}
export interface ListCanonicalThreadMCPRuntimeAuditEventsRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id?: string,
  limit?: number,
  offset?: number,
}
export interface RetryCanonicalSubagentRunRequest {
  thread_id: string,
  "X-Coze-Space-ID": string,
  run_id: string,
  "Idempotency-Key"?: string,
}