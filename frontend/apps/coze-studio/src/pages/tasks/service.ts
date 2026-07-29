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

import {
  workbench,
  workbenchSkill,
} from '@coze-studio/api-schema';

import type {
  ExportWorkbenchGuardrailAuditEventsRequest,
  ImportWorkbenchMemoriesRequest,
  ListWorkbenchGuardrailAuditEventsRequest,
  ListWorkbenchMCPRuntimeAuditEventsRequest,
  WorkbenchGuardrailAuditEvent,
  WorkbenchGuardrailAuditExport,
  WorkbenchMCPRuntimeAuditEvent,
  WorkbenchMemory,
  WorkbenchMemoryAuditEvent,
  WorkbenchMemoryExport,
  WorkbenchMemoryImportItem,
  WorkbenchMemoryImportResult,
} from '../workbench/thread-client';
import {
  type LegacyPageResponse,
  presentEmptyTaskThreadRunEventListResponse,
  presentTaskThreadCreateResponse,
  presentTaskThreadGetResponse,
  presentTaskThreadGuardrailAuditExportResponse,
  presentTaskThreadGuardrailAuditListResponse,
  presentTaskThreadMCPRuntimeAuditListResponse,
  presentTaskThreadMemoryExportResponse,
  presentTaskThreadMemoryImportResponse,
  presentTaskThreadMessageListResponse,
  presentTaskThreadMessageResponse,
  presentTaskThreadRunCancellationResponse,
  presentTaskThreadRunCreateResponse,
  presentTaskThreadRunEventListResponse,
  presentTaskThreadRunListResponse,
  presentTaskThreadRunResponse,
  presentTaskThreadSuggestionsResponse,
  presentTaskThreadListResponse,
} from '../workbench/thread-client/legacy-page-response';
import {
  canonicalThreadClient,
  resolvePageServiceSpaceID,
} from '../workbench/thread-client/canonical-thread-client-singleton';
import type {
  AppendTaskThreadMessageRequest,
  AppendTaskThreadMessageResponse,
  CancelTaskThreadRunRequest,
  CancelTaskThreadRunResponse,
  CreateTaskThreadRequest,
  CreateTaskThreadResponse,
  CreateTaskThreadRunRequest,
  CreateTaskThreadRunResponse,
  GenerateTaskThreadSuggestionsRequest,
  GenerateTaskThreadSuggestionsResponse,
  GetTaskThreadRequest,
  GetTaskThreadResponse,
  ListTaskThreadMessagesRequest,
  ListTaskThreadMessagesResponse,
  ListTaskThreadRunEventsRequest,
  ListTaskThreadRunEventsResponse,
  ListTaskThreadRunsRequest,
  ListTaskThreadRunsResponse,
  ListTaskThreadsRequest,
  ListTaskThreadsResponse,
  PageSpace,
  PageScopedRequest,
  ResumeTaskThreadRunRequest,
  ResumeTaskThreadRunResponse,
  RetryTaskThreadSubagentRunRequest,
  RetryTaskThreadSubagentRunResponse,
} from './task-service-contract';

export {
  uploadTaskThreadFiles,
  type TaskThreadUploadedFile,
} from '../workbench/service';
export { isTaskThreadArtifactSafeError } from './task-artifact-safe-error';
export {
  deleteTaskThreadArtifact,
  fetchTaskThreadArtifactContent,
  getTaskThreadArtifactSignedURL,
  listTaskThreadArtifacts,
  listTaskThreadArtifactScanJobs,
  restoreTaskThreadArtifact,
  retryTaskThreadArtifactScanJob,
  reviewTaskThreadArtifactScan,
  type ArtifactScanReviewDecision,
  type ListTaskThreadArtifactScanJobsResponse,
  type RestoreTaskThreadArtifactResponse,
  type RetryTaskThreadArtifactScanJobResponse,
  type ReviewTaskThreadArtifactScanResponse,
  type TaskThreadArtifactContentResponse,
  type TaskThreadArtifactScanJob,
  type TaskThreadArtifactSignedURLResponse,
} from './task-artifact-service';
export {
  clearTaskThreadMemories,
  deleteTaskThreadMemory,
  listTaskThreadMemories,
  listTaskThreadMemoryAuditEvents,
  restoreTaskThreadMemory,
  updateTaskThreadMemory,
} from './task-memory-service';
export { getTaskThreadTokenUsage } from './task-usage-service';

const LEGACY_MESSAGE_PAGE_SIZE = 50;
const LEGACY_RUN_EVENT_PAGE_SIZE = 100;

const pageResponse = <Response>(value: unknown): Response => value as Response;
const legacyPageNumber = (value?: number): number =>
  Number.isSafeInteger(value) && (value ?? 0) > 0 ? (value as number) : 1;
const legacyPageSize = (value: number | undefined, fallback: number): number =>
  Number.isSafeInteger(value) && (value ?? 0) > 0
    ? (value as number)
    : fallback;

export const listTaskThreads = async (
  request: ListTaskThreadsRequest,
): Promise<ListTaskThreadsResponse> =>
  pageResponse(
    presentTaskThreadListResponse(
      await canonicalThreadClient.searchThreads({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const createTaskThread = async (
  request: CreateTaskThreadRequest,
): Promise<CreateTaskThreadResponse> =>
  pageResponse(
    presentTaskThreadCreateResponse(
      await canonicalThreadClient.createThread({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const getTaskThread = async (
  request: GetTaskThreadRequest,
): Promise<GetTaskThreadResponse> =>
  pageResponse(
    presentTaskThreadGetResponse(
      await canonicalThreadClient.getThread({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadMessages = async (
  request: ListTaskThreadMessagesRequest,
): Promise<ListTaskThreadMessagesResponse> => {
  const {
    page: requestedPage,
    page_size: requestedPageSize,
    space_id: spaceID,
    ...messageRequest
  } = request;
  const page = legacyPageNumber(requestedPage);
  const pageSize = legacyPageSize(requestedPageSize, LEGACY_MESSAGE_PAGE_SIZE);
  const offset = (page - 1) * pageSize;
  const resolvedSpaceID = resolvePageServiceSpaceID(spaceID);
  const messagePage = await canonicalThreadClient.listMessages({
    ...messageRequest,
    ...(offset > 0 ? { after_seq: String(offset) } : {}),
    limit: pageSize,
    space_id: resolvedSpaceID,
  });
  return pageResponse(presentTaskThreadMessageListResponse(messagePage));
};

export const generateTaskThreadSuggestions = async (
  request: GenerateTaskThreadSuggestionsRequest,
): Promise<GenerateTaskThreadSuggestionsResponse> =>
  presentTaskThreadSuggestionsResponse(
    await canonicalThreadClient.generateSuggestions({
      ...request,
      space_id: resolvePageServiceSpaceID(request.space_id),
    }),
  );

export const appendTaskThreadMessage = async (
  request: AppendTaskThreadMessageRequest,
): Promise<AppendTaskThreadMessageResponse> =>
  pageResponse(
    presentTaskThreadMessageResponse(
      await canonicalThreadClient.appendMessage({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadRuns = async (
  request: ListTaskThreadRunsRequest,
): Promise<ListTaskThreadRunsResponse> => {
  const {
    parent_run_id: parentRunID,
    space_id: spaceID,
    ...runRequest
  } = request;

  return pageResponse(
    presentTaskThreadRunListResponse(
      await canonicalThreadClient.listRuns({
        ...runRequest,
        ...(parentRunID && parentRunID !== '0'
          ? { parent_run_id: parentRunID }
          : {}),
        space_id: resolvePageServiceSpaceID(spaceID),
      }),
    ),
  );
};

export const createTaskThreadRun = async (
  request: CreateTaskThreadRunRequest,
): Promise<CreateTaskThreadRunResponse> =>
  pageResponse(
    presentTaskThreadRunCreateResponse(
      await canonicalThreadClient.createRun({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const resumeTaskThreadRun = async (
  request: ResumeTaskThreadRunRequest,
): Promise<ResumeTaskThreadRunResponse> =>
  pageResponse(
    presentTaskThreadRunResponse(
      await canonicalThreadClient.resumeRun({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const cancelTaskThreadRun = async (
  request: CancelTaskThreadRunRequest,
): Promise<CancelTaskThreadRunResponse> => {
  await canonicalThreadClient.cancelRun({
    ...request,
    space_id: resolvePageServiceSpaceID(request.space_id),
  });

  // Canonical cancel is a 204 command. The page refreshes authoritative detail
  // after success, so synthesizing or refetching a stale Run would add risk.
  return pageResponse(presentTaskThreadRunCancellationResponse());
};

export const retryTaskThreadSubagentRun = async (
  request: RetryTaskThreadSubagentRunRequest,
): Promise<RetryTaskThreadSubagentRunResponse> =>
  pageResponse(
    presentTaskThreadRunResponse(
      await canonicalThreadClient.retrySubagentRun({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadRunEvents = async (
  request: ListTaskThreadRunEventsRequest,
): Promise<ListTaskThreadRunEventsResponse> => {
  const {
    event_types: eventTypes,
    page: requestedPage,
    page_size: requestedPageSize,
    run_id: requestedRunID,
    signal,
    space_id: requestedSpaceID,
    thread_id: threadID,
  } = request;
  const page = legacyPageNumber(requestedPage);
  const pageSize = legacyPageSize(
    requestedPageSize,
    LEGACY_RUN_EVENT_PAGE_SIZE,
  );
  const spaceID = resolvePageServiceSpaceID(requestedSpaceID);
  let runID = requestedRunID;

  if (!runID) {
    // The legacy detail loader omitted run_id. Canonical events are Run-scoped,
    // so bridge that call to the latest top-level Run during UI migration.
    const runs = await canonicalThreadClient.listRuns({
      space_id: spaceID,
      thread_id: threadID,
      page: 1,
      page_size: 1,
      signal,
    });
    runID = runs.items[0]?.run_id;
  }
  if (!runID) {
    return pageResponse(presentEmptyTaskThreadRunEventListResponse());
  }

  // Canonical public events already contain approved message/tool projections;
  // never reconstruct the legacy journal side channel with internal payloads.
  const result = await listCanonicalRunEventPage({
    eventTypes,
    page,
    pageSize,
    runID,
    signal,
    spaceID,
    threadID,
  });
  return pageResponse(presentTaskThreadRunEventListResponse(result));
};

const listCanonicalRunEventPage = async ({
  eventTypes,
  page,
  pageSize,
  runID,
  signal,
  spaceID,
  threadID,
}: {
  eventTypes?: string[];
  page: number;
  pageSize: number;
  runID: string;
  signal?: AbortSignal;
  spaceID: string;
  threadID: string;
}) => {
  let currentPage = 1;
  let cursor: string | undefined;
  const seenCursors = new Set<string>();

  while (true) {
    const result = await canonicalThreadClient.listRunEvents({
      space_id: spaceID,
      thread_id: threadID,
      run_id: runID,
      ...(eventTypes === undefined ? {} : { event_types: eventTypes }),
      ...(cursor === undefined ? {} : { cursor }),
      limit: pageSize,
      signal,
    });
    if (currentPage >= page) {
      return result;
    }
    if (!result.next_cursor) {
      if (result.has_more) {
        throw new Error('Canonical Run Event cursor is missing');
      }
      return { items: [], total: result.total, has_more: false };
    }
    if (seenCursors.has(result.next_cursor)) {
      throw new Error('Canonical Run Event cursor repeated');
    }
    seenCursors.add(result.next_cursor);
    cursor = result.next_cursor;
    currentPage += 1;
  }
};

export const getWorkbenchRuntimeDoctor = workbench.GetWorkbenchRuntimeDoctor;
export const installSkillFromArtifact = workbenchSkill.InstallSkillFromArtifact;
export type TaskThreadMemory = WorkbenchMemory;
export type TaskThreadMemoryAuditEvent = WorkbenchMemoryAuditEvent;
export type TaskThreadGuardrailAuditEvent = WorkbenchGuardrailAuditEvent;
export type TaskThreadMCPRuntimeAuditEvent = WorkbenchMCPRuntimeAuditEvent;
export type ListTaskThreadMemoriesResponse = LegacyPageResponse<{
  memories: WorkbenchMemory[];
  total: number;
}>;
export type UpdateTaskThreadMemoryResponse = LegacyPageResponse<{
  memory: WorkbenchMemory;
  updated: boolean;
}>;
export type DeleteTaskThreadMemoryResponse = Omit<
  LegacyPageResponse<never>,
  'data'
>;
export type ClearTaskThreadMemoriesResponse = LegacyPageResponse<{
  deleted: number;
}>;
export type RestoreTaskThreadMemoryResponse = LegacyPageResponse<{
  memory: WorkbenchMemory;
  restored: boolean;
}>;
export type ListTaskThreadMemoryAuditEventsResponse = LegacyPageResponse<{
  events: WorkbenchMemoryAuditEvent[];
  total: number;
}>;
export type ListTaskThreadGuardrailAuditEventsResponse = LegacyPageResponse<{
  events: WorkbenchGuardrailAuditEvent[];
  total: number;
}>;
export type ListTaskThreadMCPRuntimeAuditEventsResponse = LegacyPageResponse<{
  events: WorkbenchMCPRuntimeAuditEvent[];
  total: number;
}>;
export type ExportTaskThreadMemoriesData = WorkbenchMemoryExport;
export type ExportTaskThreadMemoriesResponse = LegacyPageResponse<
  ExportTaskThreadMemoriesData
>;
export type ExportTaskThreadGuardrailAuditEventsData =
  WorkbenchGuardrailAuditExport & {
    page: number;
    page_size: number;
  };
export type ExportTaskThreadGuardrailAuditEventsResponse = LegacyPageResponse<
  ExportTaskThreadGuardrailAuditEventsData
>;
export type ImportTaskThreadMemoryItem = WorkbenchMemoryImportItem;
export type ImportTaskThreadMemoriesResponse = LegacyPageResponse<
  WorkbenchMemoryImportResult
>;
export type WorkbenchRuntimeDoctorData = workbench.WorkbenchRuntimeDoctorData;
export type RuntimeDoctorCheck = workbench.RuntimeDoctorCheck;

interface ExportTaskThreadMemoriesRequest extends PageScopedRequest {
  thread_id: string;
  run_id?: string;
  scope?: string;
  scopes?: string[];
  q?: string;
  include_expired?: boolean;
  include_deleted?: boolean;
  limit?: number;
  signal?: AbortSignal;
}
type ImportTaskThreadMemoriesRequest = PageSpace<
  ImportWorkbenchMemoriesRequest
>;
type ListTaskThreadGuardrailAuditEventsRequest = PageSpace<
  ListWorkbenchGuardrailAuditEventsRequest
>;
type ExportTaskThreadGuardrailAuditEventsRequest = PageSpace<
  ExportWorkbenchGuardrailAuditEventsRequest
>;
type ListTaskThreadMCPRuntimeAuditEventsRequest = PageSpace<
  ListWorkbenchMCPRuntimeAuditEventsRequest
>;

export const exportTaskThreadMemories = async (
  request: ExportTaskThreadMemoriesRequest,
): Promise<ExportTaskThreadMemoriesResponse> => {
  const { limit, space_id: spaceID, ...memoryRequest } = request;

  return pageResponse(
    presentTaskThreadMemoryExportResponse(
      await canonicalThreadClient.exportMemories({
        ...memoryRequest,
        ...(limit === undefined ? {} : { page_size: limit }),
        space_id: resolvePageServiceSpaceID(spaceID),
      }),
    ),
  );
};

export const importTaskThreadMemories = async (
  request: ImportTaskThreadMemoriesRequest,
): Promise<ImportTaskThreadMemoriesResponse> =>
  pageResponse(
    presentTaskThreadMemoryImportResponse(
      await canonicalThreadClient.importMemories({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadGuardrailAuditEvents = async (
  request: ListTaskThreadGuardrailAuditEventsRequest,
): Promise<ListTaskThreadGuardrailAuditEventsResponse> =>
  pageResponse(
    presentTaskThreadGuardrailAuditListResponse(
      await canonicalThreadClient.listGuardrailAuditEvents({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const exportTaskThreadGuardrailAuditEvents = async (
  request: ExportTaskThreadGuardrailAuditEventsRequest,
): Promise<ExportTaskThreadGuardrailAuditEventsResponse> =>
  pageResponse(
    presentTaskThreadGuardrailAuditExportResponse(
      await canonicalThreadClient.exportGuardrailAuditEvents({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
      request,
    ),
  );

export const listTaskThreadMCPRuntimeAuditEvents = async (
  request: ListTaskThreadMCPRuntimeAuditEventsRequest,
): Promise<ListTaskThreadMCPRuntimeAuditEventsResponse> =>
  pageResponse(
    presentTaskThreadMCPRuntimeAuditListResponse(
      await canonicalThreadClient.listMCPRuntimeAuditEvents({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );
