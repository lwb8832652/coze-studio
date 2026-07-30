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
  WorkbenchCursorPage,
  WorkbenchMessageCursorPage,
  WorkbenchPage,
  WorkbenchTokenUsageResult,
} from './workbench-thread-client';
import type {
  WorkbenchArtifact,
  WorkbenchArtifactContent,
  WorkbenchArtifactRestoreResult,
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
  WorkbenchThread,
  WorkbenchThreadCreation,
  WorkbenchUploadCreation,
} from './types';

export interface LegacyPageResponse<T> {
  data?: T;
  code: number;
  msg: string;
}

const success = <T>(data: T): LegacyPageResponse<T> => ({
  code: 0,
  msg: 'success',
  data,
});

export const presentTaskThreadListResponse = (
  page: WorkbenchPage<WorkbenchThread>,
) => success({ threads: page.items, total: page.total });

export const presentTaskThreadCreateResponse = (
  creation: WorkbenchThreadCreation,
) => success(creation);

export const presentTaskThreadGetResponse = (thread: WorkbenchThread) =>
  success(thread);

export const presentTaskThreadMessageListResponse = (
  page: WorkbenchMessageCursorPage,
) => success({ messages: page.items, total: page.total });

export const presentTaskThreadMessageResponse = (message: WorkbenchMessage) =>
  success(message);

export const presentTaskThreadSuggestionsResponse = (
  suggestions: string[],
) => ({ suggestions });

export const presentTaskThreadRunListResponse = (
  page: WorkbenchPage<WorkbenchRun>,
) => success({ runs: page.items, total: page.total });

export const presentTaskThreadRunCreateResponse = (
  creation: WorkbenchRunCreation,
) => ({
  code: 0,
  msg: 'success',
  data: creation.run,
  ...(creation.message === undefined ? {} : { message: creation.message }),
});

export const presentTaskThreadRunResponse = (run: WorkbenchRun) => success(run);

export const presentTaskThreadRunCancellationResponse = () => ({
  code: 0,
  msg: 'success',
});

export const presentTaskThreadRunEventListResponse = (
  page: WorkbenchCursorPage<WorkbenchRunEvent>,
) => success({ events: page.items, total: page.total });

export const presentEmptyTaskThreadRunEventListResponse = () =>
  success({ events: [] as WorkbenchRunEvent[], total: 0 });

export const presentTaskThreadUploadResponse = (
  creation: WorkbenchUploadCreation,
) =>
  success({
    files: creation.uploads,
    skipped_files: creation.skipped_files,
  });

export const presentTaskThreadArtifactListResponse = (
  page: WorkbenchPage<WorkbenchArtifact>,
) => success({ artifacts: page.items, total: page.total });

export const presentTaskThreadArtifactContentResponse = (
  content: WorkbenchArtifactContent,
) => ({
  blob: content.blob,
  contentDisposition: content.content_disposition,
  contentType: content.content_type,
});

export const presentTaskThreadArtifactSignedURLResponse = (
  signedURL: WorkbenchArtifactSignedURL,
) => success(signedURL);

export const presentTaskThreadArtifactRestoreResponse = (
  result: WorkbenchArtifactRestoreResult,
) =>
  success({
    artifact_id: result.artifact.artifact_id,
    restored: result.restored,
  });

export const presentTaskThreadArtifactScanReviewResponse = (
  result: WorkbenchArtifactScanReviewResult,
) => success(result);

export const presentTaskThreadArtifactScanJobListResponse = (
  page: WorkbenchPage<WorkbenchArtifactScanJob>,
) => success({ jobs: page.items, total: page.total });

export const presentTaskThreadArtifactScanRetryResponse = (
  result: WorkbenchArtifactScanRetryResult,
) => success(result);

export const presentTaskThreadTokenUsageResponse = (
  result: WorkbenchTokenUsageResult,
) =>
  success({
    usage: result.items,
    total: result.total,
    aggregate: result.aggregate,
    run_aggregates: result.run_aggregates,
  });

export const presentTaskThreadMemoryListResponse = (
  page: WorkbenchPage<WorkbenchMemory>,
) => success({ memories: page.items, total: page.total });

export const presentTaskThreadMemoryUpdateResponse = (
  result: WorkbenchMemoryUpdateResult,
) => success({ memory: result.memory, updated: result.updated });

export const presentTaskThreadMemoryDeleteResponse = () => ({
  code: 0,
  msg: 'success',
});

export const presentTaskThreadMemoryClearResponse = (
  result: WorkbenchMemoryClearResult,
) => success({ deleted: result.deleted });

export const presentTaskThreadMemoryRestoreResponse = (
  result: WorkbenchMemoryRestoreResult,
) => success(result);

export const presentTaskThreadMemoryImportResponse = (
  result: WorkbenchMemoryImportResult,
) => success(result);

export const presentTaskThreadMemoryExportResponse = (
  result: WorkbenchMemoryExport,
) => success(result);

export const presentTaskThreadMemoryAuditListResponse = (
  page: WorkbenchPage<WorkbenchMemoryAuditEvent>,
) => success({ events: page.items, total: page.total });

export const presentTaskThreadGuardrailAuditListResponse = (
  page: WorkbenchPage<WorkbenchGuardrailAuditEvent>,
) => success({ events: page.items, total: page.total });

export const presentTaskThreadGuardrailAuditExportResponse = (
  result: WorkbenchGuardrailAuditExport,
  pagination: { page?: number; page_size?: number },
) =>
  success({
    ...result,
    page: pagination.page ?? 1,
    page_size: pagination.page_size ?? result.events.length,
  });

export const presentTaskThreadMCPRuntimeAuditListResponse = (
  page: WorkbenchPage<WorkbenchMCPRuntimeAuditEvent>,
) => success({ events: page.items, total: page.total });
