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

import type {
  HumanInteractionResponse,
  WorkbenchArtifact,
  WorkbenchArtifactContent,
  WorkbenchArtifactRestoreResult,
  WorkbenchArtifactScanDecision,
  WorkbenchArtifactScanJob,
  WorkbenchArtifactScanRetryResult,
  WorkbenchArtifactScanReviewResult,
  WorkbenchArtifactSignedURL,
  WorkbenchGuardrailAuditEvent,
  WorkbenchGuardrailAuditExport,
  WorkbenchMCPRuntimeAuditEvent,
  WorkbenchMemory,
  WorkbenchMemoryAuditEvent,
  WorkbenchMemoryClearResult,
  WorkbenchMemoryExport,
  WorkbenchMemoryImportResult,
  WorkbenchMemoryRestoreResult,
  WorkbenchMemoryUpdateResult,
  WorkbenchMessage,
  WorkbenchRun,
  WorkbenchRunCreation,
  WorkbenchRunEvent,
  WorkbenchRunTokenUsageAggregate,
  WorkbenchSuggestionMessage,
  WorkbenchThread,
  WorkbenchThreadCreation,
  WorkbenchTokenUsage,
  WorkbenchTokenUsageAggregate,
  WorkbenchUpload,
  WorkbenchUploadCreation,
} from './types';

export interface WorkbenchScopedRequest {
  space_id: string;
}

export interface WorkbenchThreadRequest extends WorkbenchScopedRequest {
  thread_id: string;
}

export interface WorkbenchRunRequest extends WorkbenchThreadRequest {
  run_id: string;
}

export interface WorkbenchPageOptions {
  page?: number;
  page_size?: number;
}

export interface WorkbenchCursorOptions {
  cursor?: string;
}

export interface WorkbenchAbortOptions {
  signal?: AbortSignal;
}

export interface WorkbenchIdempotencyOptions {
  idempotency_key?: string;
}

export interface WorkbenchPage<T> {
  items: T[];
  total: number;
  has_more: boolean;
  next_cursor?: string;
}

export interface WorkbenchCursorPage<T> {
  items: T[];
  has_more: boolean;
  next_cursor?: string;
}

export interface WorkbenchMessageCursorPage {
  items: WorkbenchMessage[];
  has_more: boolean;
  next_before_seq?: string;
  next_after_seq?: string;
}

export interface SearchWorkbenchThreadsRequest
  extends WorkbenchScopedRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  status?: string;
}

export interface CreateWorkbenchThreadRequest
  extends WorkbenchScopedRequest,
    WorkbenchAbortOptions,
    WorkbenchIdempotencyOptions {
  message: string;
  title?: string;
  assistant_id?: string;
  command?: string;
  config?: string;
  context?: string;
  metadata?: string;
  stream_mode?: string;
  multitask_strategy?: string;
  on_disconnect?: string;
  durability?: string;
  defer_start?: boolean;
}

export interface GetWorkbenchThreadRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {}

export type ListWorkbenchMessagesRequest = WorkbenchThreadRequest &
  WorkbenchAbortOptions & {
    limit?: number;
  } & (
    | { before_seq?: string; after_seq?: never }
    | { before_seq?: never; after_seq?: string }
  );

export interface AppendWorkbenchMessageRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  run_id?: string;
  role: string;
  content: string;
  metadata?: string;
}

export interface GenerateWorkbenchSuggestionsRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  messages?: WorkbenchSuggestionMessage[];
  n?: number;
  model_name?: string;
  model_type?: string;
}

export interface ListWorkbenchRunsRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  parent_run_id?: string;
  status?: string;
}

export interface CreateWorkbenchRunRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions,
    WorkbenchIdempotencyOptions {
  assistant_id?: string;
  command?: string;
  input: string;
  config?: string;
  context?: string;
  metadata?: string;
  stream_mode?: string;
  multitask_strategy?: string;
  on_disconnect?: string;
  durability?: string;
  message_content?: string;
  message_metadata?: string;
}

export interface GetWorkbenchRunRequest
  extends WorkbenchRunRequest,
    WorkbenchAbortOptions {}

export interface CancelWorkbenchRunRequest
  extends WorkbenchRunRequest,
    WorkbenchAbortOptions {}

export interface ResumeWorkbenchRunRequest
  extends WorkbenchRunRequest,
    WorkbenchAbortOptions,
    WorkbenchIdempotencyOptions {
  interrupt_id: string;
  response: HumanInteractionResponse;
}

export interface RetryWorkbenchSubagentRunRequest
  extends WorkbenchRunRequest,
    WorkbenchAbortOptions,
    WorkbenchIdempotencyOptions {}

export interface ListWorkbenchRunEventsRequest
  extends WorkbenchRunRequest,
    WorkbenchCursorOptions,
    WorkbenchAbortOptions {
  event_types?: string[];
}

export interface SubscribeWorkbenchRunEventsRequest
  extends WorkbenchRunRequest,
    WorkbenchCursorOptions {
  signal: AbortSignal;
  onEvent: (event: WorkbenchRunEvent) => void;
  onEnd: () => void;
  onError: (error: Error) => void;
}

export interface RunEventSubscription {
  // eslint-disable-next-line @typescript-eslint/method-signature-style -- Public contract is intentionally exact.
  close(): void;
  closed: Promise<void>;
}

export interface ListWorkbenchUploadsRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {}

export interface UploadWorkbenchFilesRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  files: File[];
}

export interface DeleteWorkbenchUploadRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  file_id: string;
}

export interface ListWorkbenchArtifactsRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  run_id?: string;
  deleted_only?: boolean;
}

export interface WorkbenchArtifactRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  artifact_id: string;
}

export interface GetWorkbenchArtifactContentRequest
  extends WorkbenchArtifactRequest {
  mode: 'preview' | 'download';
}

export interface GetWorkbenchArtifactSignedURLRequest
  extends WorkbenchArtifactRequest {
  mode: 'preview' | 'download';
  ttl_seconds?: number;
}

export interface ReviewWorkbenchArtifactScanRequest
  extends WorkbenchArtifactRequest {
  decision: WorkbenchArtifactScanDecision;
  reason?: string;
}

export interface ListWorkbenchArtifactScanJobsRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  run_id?: string;
  artifact_id?: string;
  status?: string;
  scanner?: string;
}

export interface RetryWorkbenchArtifactScanJobRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  job_id: string;
}

export interface GetWorkbenchTokenUsageRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  run_id?: string;
  include_child_runs?: boolean;
  source?: string;
}

export interface WorkbenchTokenUsageResult
  extends WorkbenchPage<WorkbenchTokenUsage> {
  aggregate: WorkbenchTokenUsageAggregate;
  run_aggregates: WorkbenchRunTokenUsageAggregate[];
}

export interface ListWorkbenchMemoriesRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  run_id?: string;
  scope?: string;
  scopes?: string[];
  q?: string;
  include_expired?: boolean;
  include_deleted?: boolean;
}

export interface WorkbenchMemoryRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  memory_id: string;
}

interface WorkbenchMemoryMutationFields {
  run_id?: string;
  content: string;
  metadata?: string;
  score?: number;
  confidence?: number;
  source_type?: string;
  source_id?: string;
  correction_of_memory_id?: string;
  corrected_at?: number;
  expires_at?: number;
}

export interface WorkbenchMemoryUpdate extends WorkbenchMemoryMutationFields {
  scope: string;
}

export interface WorkbenchMemoryImportItem
  extends WorkbenchMemoryMutationFields {
  scope?: string;
}

export interface UpdateWorkbenchMemoryRequest
  extends WorkbenchMemoryRequest,
    WorkbenchMemoryUpdate {}

export interface ClearWorkbenchMemoriesRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  run_id?: string;
  scopes?: string[];
}

export interface ImportWorkbenchMemoriesRequest
  extends WorkbenchThreadRequest,
    WorkbenchAbortOptions {
  memories: WorkbenchMemoryImportItem[];
}

export type ExportWorkbenchMemoriesRequest = ListWorkbenchMemoriesRequest;

export interface ListWorkbenchMemoryAuditEventsRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  memory_id?: string;
}

export interface ListWorkbenchGuardrailAuditEventsRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  run_id?: string;
}

export type ExportWorkbenchGuardrailAuditEventsRequest =
  ListWorkbenchGuardrailAuditEventsRequest;

export interface ListWorkbenchMCPRuntimeAuditEventsRequest
  extends WorkbenchThreadRequest,
    WorkbenchPageOptions,
    WorkbenchAbortOptions {
  run_id?: string;
}

export interface WorkbenchThreadClient {
  readonly contract: 'canonical_v1';

  searchThreads: (
    request: SearchWorkbenchThreadsRequest,
  ) => Promise<WorkbenchPage<WorkbenchThread>>;
  createThread: (
    request: CreateWorkbenchThreadRequest,
  ) => Promise<WorkbenchThreadCreation>;
  getThread: (request: GetWorkbenchThreadRequest) => Promise<WorkbenchThread>;

  listMessages: (
    request: ListWorkbenchMessagesRequest,
  ) => Promise<WorkbenchMessageCursorPage>;
  appendMessage: (
    request: AppendWorkbenchMessageRequest,
  ) => Promise<WorkbenchMessage>;
  generateSuggestions: (
    request: GenerateWorkbenchSuggestionsRequest,
  ) => Promise<string[]>;

  listRuns: (
    request: ListWorkbenchRunsRequest,
  ) => Promise<WorkbenchPage<WorkbenchRun>>;
  createRun: (
    request: CreateWorkbenchRunRequest,
  ) => Promise<WorkbenchRunCreation>;
  getRun: (request: GetWorkbenchRunRequest) => Promise<WorkbenchRun>;
  cancelRun: (request: CancelWorkbenchRunRequest) => Promise<void>;
  resumeRun: (request: ResumeWorkbenchRunRequest) => Promise<WorkbenchRun>;
  retrySubagentRun: (
    request: RetryWorkbenchSubagentRunRequest,
  ) => Promise<WorkbenchRun>;
  listRunEvents: (
    request: ListWorkbenchRunEventsRequest,
  ) => Promise<WorkbenchCursorPage<WorkbenchRunEvent>>;
  subscribeRunEvents: (
    request: SubscribeWorkbenchRunEventsRequest,
  ) => RunEventSubscription;

  listUploads: (
    request: ListWorkbenchUploadsRequest,
  ) => Promise<WorkbenchPage<WorkbenchUpload>>;
  uploadFiles: (
    request: UploadWorkbenchFilesRequest,
  ) => Promise<WorkbenchUploadCreation>;
  deleteUpload: (request: DeleteWorkbenchUploadRequest) => Promise<void>;

  listArtifacts: (
    request: ListWorkbenchArtifactsRequest,
  ) => Promise<WorkbenchPage<WorkbenchArtifact>>;
  getArtifactContent: (
    request: GetWorkbenchArtifactContentRequest,
  ) => Promise<WorkbenchArtifactContent>;
  getArtifactSignedURL: (
    request: GetWorkbenchArtifactSignedURLRequest,
  ) => Promise<WorkbenchArtifactSignedURL>;
  deleteArtifact: (request: WorkbenchArtifactRequest) => Promise<void>;
  restoreArtifact: (
    request: WorkbenchArtifactRequest,
  ) => Promise<WorkbenchArtifactRestoreResult>;
  reviewArtifactScan: (
    request: ReviewWorkbenchArtifactScanRequest,
  ) => Promise<WorkbenchArtifactScanReviewResult>;
  listArtifactScanJobs: (
    request: ListWorkbenchArtifactScanJobsRequest,
  ) => Promise<WorkbenchPage<WorkbenchArtifactScanJob>>;
  retryArtifactScanJob: (
    request: RetryWorkbenchArtifactScanJobRequest,
  ) => Promise<WorkbenchArtifactScanRetryResult>;

  getTokenUsage: (
    request: GetWorkbenchTokenUsageRequest,
  ) => Promise<WorkbenchTokenUsageResult>;

  listMemories: (
    request: ListWorkbenchMemoriesRequest,
  ) => Promise<WorkbenchPage<WorkbenchMemory>>;
  updateMemory: (
    request: UpdateWorkbenchMemoryRequest,
  ) => Promise<WorkbenchMemoryUpdateResult>;
  deleteMemory: (request: WorkbenchMemoryRequest) => Promise<void>;
  clearMemories: (
    request: ClearWorkbenchMemoriesRequest,
  ) => Promise<WorkbenchMemoryClearResult>;
  restoreMemory: (
    request: WorkbenchMemoryRequest,
  ) => Promise<WorkbenchMemoryRestoreResult>;
  importMemories: (
    request: ImportWorkbenchMemoriesRequest,
  ) => Promise<WorkbenchMemoryImportResult>;
  exportMemories: (
    request: ExportWorkbenchMemoriesRequest,
  ) => Promise<WorkbenchMemoryExport>;

  listMemoryAuditEvents: (
    request: ListWorkbenchMemoryAuditEventsRequest,
  ) => Promise<WorkbenchPage<WorkbenchMemoryAuditEvent>>;
  listGuardrailAuditEvents: (
    request: ListWorkbenchGuardrailAuditEventsRequest,
  ) => Promise<WorkbenchPage<WorkbenchGuardrailAuditEvent>>;
  exportGuardrailAuditEvents: (
    request: ExportWorkbenchGuardrailAuditEventsRequest,
  ) => Promise<WorkbenchGuardrailAuditExport>;
  listMCPRuntimeAuditEvents: (
    request: ListWorkbenchMCPRuntimeAuditEventsRequest,
  ) => Promise<WorkbenchPage<WorkbenchMCPRuntimeAuditEvent>>;
}
