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
  type workbenchTask,
} from '@coze-studio/api-schema';

import {
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

interface PageScopedRequest {
  space_id?: string;
}

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
  request: workbenchTask.ListTaskThreadsRequest,
): Promise<workbenchTask.ListTaskThreadsResponse> =>
  pageResponse(
    presentTaskThreadListResponse(
      await canonicalThreadClient.searchThreads({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const createTaskThread = async (
  request: workbenchTask.CreateTaskThreadRequest,
): Promise<workbenchTask.CreateTaskThreadResponse> =>
  pageResponse(
    presentTaskThreadCreateResponse(
      await canonicalThreadClient.createThread({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const getTaskThread = async (
  request: workbenchTask.GetTaskThreadRequest & PageScopedRequest,
): Promise<workbenchTask.GetTaskThreadResponse> =>
  pageResponse(
    presentTaskThreadGetResponse(
      await canonicalThreadClient.getThread({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadMessages = async (
  request: workbenchTask.ListTaskThreadMessagesRequest & PageScopedRequest,
): Promise<workbenchTask.ListTaskThreadMessagesResponse> => {
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
  request: workbenchTask.GenerateTaskThreadSuggestionsRequest &
    PageScopedRequest,
): Promise<workbenchTask.GenerateTaskThreadSuggestionsResponse> =>
  presentTaskThreadSuggestionsResponse(
    await canonicalThreadClient.generateSuggestions({
      ...request,
      space_id: resolvePageServiceSpaceID(request.space_id),
    }),
  );

export const appendTaskThreadMessage = async (
  request: workbenchTask.AppendTaskThreadMessageRequest & PageScopedRequest,
): Promise<workbenchTask.AppendTaskThreadMessageResponse> =>
  pageResponse(
    presentTaskThreadMessageResponse(
      await canonicalThreadClient.appendMessage({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadRuns = async (
  request: workbenchTask.ListTaskThreadRunsRequest & PageScopedRequest,
): Promise<workbenchTask.ListTaskThreadRunsResponse> => {
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
  request: workbenchTask.CreateTaskThreadRunRequest & PageScopedRequest,
): Promise<workbenchTask.CreateTaskThreadRunResponse> =>
  pageResponse(
    presentTaskThreadRunCreateResponse(
      await canonicalThreadClient.createRun({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const resumeTaskThreadRun = async (
  request: workbenchTask.ResumeTaskThreadRunRequest & PageScopedRequest,
): Promise<workbenchTask.ResumeTaskThreadRunResponse> =>
  pageResponse(
    presentTaskThreadRunResponse(
      await canonicalThreadClient.resumeRun({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const cancelTaskThreadRun = async (
  request: workbenchTask.CancelTaskThreadRunRequest & PageScopedRequest,
): Promise<workbenchTask.CancelTaskThreadRunResponse> => {
  await canonicalThreadClient.cancelRun({
    ...request,
    space_id: resolvePageServiceSpaceID(request.space_id),
  });

  // Canonical cancel is a 204 command. The page refreshes authoritative detail
  // after success, so synthesizing or refetching a stale Run would add risk.
  return pageResponse(presentTaskThreadRunCancellationResponse());
};

export const retryTaskThreadSubagentRun = async (
  request: workbenchTask.RetryTaskThreadSubagentRunRequest & PageScopedRequest,
): Promise<workbenchTask.RetryTaskThreadSubagentRunResponse> =>
  pageResponse(
    presentTaskThreadRunResponse(
      await canonicalThreadClient.retrySubagentRun({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadRunEvents = async (
  request: workbenchTask.ListTaskThreadRunEventsRequest & PageScopedRequest,
): Promise<workbenchTask.ListTaskThreadRunEventsResponse> => {
  const {
    page: requestedPage,
    page_size: requestedPageSize,
    run_id: requestedRunID,
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
    });
    runID = runs.items[0]?.run_id;
  }
  if (!runID) {
    return pageResponse(presentEmptyTaskThreadRunEventListResponse());
  }

  // Canonical public events already contain approved message/tool projections;
  // never reconstruct the legacy journal side channel with internal payloads.
  const result = await listCanonicalRunEventPage({
    page,
    pageSize,
    runID,
    spaceID,
    threadID,
  });
  return pageResponse(presentTaskThreadRunEventListResponse(result));
};

const listCanonicalRunEventPage = async ({
  page,
  pageSize,
  runID,
  spaceID,
  threadID,
}: {
  page: number;
  pageSize: number;
  runID: string;
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
      ...(cursor === undefined ? {} : { cursor }),
      limit: pageSize,
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
export type TaskThreadMemory = workbenchTask.TaskThreadMemory;
export type TaskThreadMemoryAuditEvent =
  workbenchTask.TaskThreadMemoryAuditEvent;
export type TaskThreadGuardrailAuditEvent =
  workbenchTask.TaskThreadGuardrailAuditEvent;
export type TaskThreadMCPRuntimeAuditEvent =
  workbenchTask.TaskThreadMCPRuntimeAuditEvent;
export type ListTaskThreadMemoriesResponse =
  workbenchTask.ListTaskThreadMemoriesResponse;
export type UpdateTaskThreadMemoryResponse =
  workbenchTask.UpdateTaskThreadMemoryResponse;
export type DeleteTaskThreadMemoryResponse =
  workbenchTask.DeleteTaskThreadMemoryResponse;
export type ClearTaskThreadMemoriesResponse =
  workbenchTask.ClearTaskThreadMemoriesResponse;
export type RestoreTaskThreadMemoryResponse =
  workbenchTask.RestoreTaskThreadMemoryResponse;
export type ListTaskThreadMemoryAuditEventsResponse =
  workbenchTask.ListTaskThreadMemoryAuditEventsResponse;
export type ListTaskThreadGuardrailAuditEventsResponse =
  workbenchTask.ListTaskThreadGuardrailAuditEventsResponse;
export type ListTaskThreadMCPRuntimeAuditEventsResponse =
  workbenchTask.ListTaskThreadMCPRuntimeAuditEventsResponse;
export type ExportTaskThreadMemoriesResponse =
  workbenchTask.ExportTaskThreadMemoriesResponse;
export type ExportTaskThreadMemoriesData =
  workbenchTask.ExportTaskThreadMemoriesData;
export type ExportTaskThreadGuardrailAuditEventsResponse =
  workbenchTask.ExportTaskThreadGuardrailAuditEventsResponse;
export type ExportTaskThreadGuardrailAuditEventsData =
  workbenchTask.ExportTaskThreadGuardrailAuditEventsData;
export type ImportTaskThreadMemoryItem =
  workbenchTask.ImportTaskThreadMemoryItem;
export type ImportTaskThreadMemoriesResponse =
  workbenchTask.ImportTaskThreadMemoriesResponse;
export type WorkbenchRuntimeDoctorData = workbench.WorkbenchRuntimeDoctorData;
export type RuntimeDoctorCheck = workbench.RuntimeDoctorCheck;

export const exportTaskThreadMemories = async (
  request: workbenchTask.ExportTaskThreadMemoriesRequest & PageScopedRequest,
): Promise<workbenchTask.ExportTaskThreadMemoriesResponse> => {
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
  request: workbenchTask.ImportTaskThreadMemoriesRequest & PageScopedRequest,
): Promise<workbenchTask.ImportTaskThreadMemoriesResponse> =>
  pageResponse(
    presentTaskThreadMemoryImportResponse(
      await canonicalThreadClient.importMemories({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadGuardrailAuditEvents = async (
  request: workbenchTask.ListTaskThreadGuardrailAuditEventsRequest &
    PageScopedRequest,
): Promise<workbenchTask.ListTaskThreadGuardrailAuditEventsResponse> =>
  pageResponse(
    presentTaskThreadGuardrailAuditListResponse(
      await canonicalThreadClient.listGuardrailAuditEvents({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const exportTaskThreadGuardrailAuditEvents = async (
  request: workbenchTask.ExportTaskThreadGuardrailAuditEventsRequest &
    PageScopedRequest,
): Promise<workbenchTask.ExportTaskThreadGuardrailAuditEventsResponse> =>
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
  request: workbenchTask.ListTaskThreadMCPRuntimeAuditEventsRequest &
    PageScopedRequest,
): Promise<workbenchTask.ListTaskThreadMCPRuntimeAuditEventsResponse> =>
  pageResponse(
    presentTaskThreadMCPRuntimeAuditListResponse(
      await canonicalThreadClient.listMCPRuntimeAuditEvents({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const getTaskThreadRunEventsStreamURL = ({
  afterEventId,
  runId,
  threadId,
}: {
  threadId: string;
  runId?: string;
  afterEventId?: string;
}) => {
  const params = new URLSearchParams();
  if (runId) {
    params.set('run_id', runId);
  }
  if (afterEventId) {
    params.set('after_event_id', afterEventId);
  }

  const query = params.toString();

  return `/api/workbench/task_threads/${encodeURIComponent(
    threadId,
  )}/run_events/stream${query ? `?${query}` : ''}`;
};
