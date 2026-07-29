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

import { type workbenchTask } from '@coze-studio/api-schema';

import {
  presentTaskThreadArtifactContentResponse,
  presentTaskThreadArtifactListResponse,
  presentTaskThreadArtifactRestoreResponse,
  presentTaskThreadArtifactScanJobListResponse,
  presentTaskThreadArtifactScanRetryResponse,
  presentTaskThreadArtifactScanReviewResponse,
  presentTaskThreadArtifactSignedURLResponse,
} from '../workbench/thread-client/legacy-page-response';
import {
  canonicalThreadClient,
  resolvePageServiceSpaceID,
} from '../workbench/thread-client/canonical-thread-client-singleton';
import { taskThreadArtifactPayloadError } from './task-artifact-safe-error';

export interface TaskThreadArtifactContentResponse {
  blob: Blob;
  contentDisposition: string;
  contentType: string;
}

export interface TaskThreadArtifactSignedURLResponse {
  data?: {
    artifact_id: string;
    url: string;
    expires_in_seconds: number;
    content_type: string;
    preview_mode: string;
  };
  code: number;
  msg: string;
  reason?: string;
}

export interface TaskThreadArtifactScanJob {
  job_id: string;
  thread_id: string;
  run_id: string;
  space_id: string;
  user_id: string;
  artifact_id: string;
  file_id: string;
  scanner: string;
  status: string;
  worker_id: string;
  attempt_count: number;
  last_error: string;
  available_at: number;
  lease_expires_at: number;
  started_at: number;
  ended_at: number;
  created_at: number;
  updated_at: number;
}

export interface ListTaskThreadArtifactScanJobsResponse {
  data?: {
    jobs: TaskThreadArtifactScanJob[];
    total: number;
  };
  code: number;
  msg: string;
}

export interface RetryTaskThreadArtifactScanJobResponse {
  data?: {
    job?: TaskThreadArtifactScanJob;
    retried: boolean;
  };
  code: number;
  msg: string;
}

export type ArtifactScanReviewDecision = 'release' | 'quarantine' | 'block';

export interface ReviewTaskThreadArtifactScanResponse {
  data?: {
    artifact_id: string;
    decision: ArtifactScanReviewDecision;
    reviewed: boolean;
    scan_status: string;
  };
  code: number;
  msg: string;
}

export interface RestoreTaskThreadArtifactResponse {
  data?: {
    artifact_id: string;
    restored: boolean;
  };
  code: number;
  msg: string;
}

interface ArtifactScanJobListRequest {
  thread_id: string;
  run_id?: string;
  artifact_id?: string;
  space_id?: string;
  status?: string;
  scanner?: string;
  page?: number;
  page_size?: number;
}

interface ArtifactScanJobRetryRequest {
  thread_id: string;
  job_id: string;
  space_id?: string;
}

interface ArtifactScanReviewRequest {
  thread_id: string;
  artifact_id: string;
  space_id?: string;
  decision: ArtifactScanReviewDecision;
  reason?: string;
}

interface ArtifactContentRequest {
  thread_id: string;
  artifact_id: string;
  space_id?: string;
  mode: 'preview' | 'download';
}

interface ArtifactSignedURLRequest extends ArtifactContentRequest {
  ttl_seconds?: number;
}

interface ArtifactMutationRequest {
  thread_id: string;
  artifact_id: string;
  space_id?: string;
}

const pageResponse = <Response>(value: unknown): Response => value as Response;

const artifactRequestError = (message: string, cause: unknown): never => {
  const error = new Error(message);
  (error as Error & { cause?: unknown }).cause = cause;
  throw error;
};

const artifactErrorCode = (error: unknown): string | undefined => {
  if (!error || typeof error !== 'object' || !('code' in error)) {
    return undefined;
  }
  const { code } = error as { code?: unknown };
  return typeof code === 'string' ? code : undefined;
};

export const listTaskThreadArtifacts = async (
  request: workbenchTask.ListTaskThreadArtifactsRequest,
): Promise<workbenchTask.ListTaskThreadArtifactsResponse> =>
  pageResponse(
    presentTaskThreadArtifactListResponse(
      await canonicalThreadClient.listArtifacts({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    ),
  );

export const listTaskThreadArtifactScanJobs = async (
  request: ArtifactScanJobListRequest,
): Promise<ListTaskThreadArtifactScanJobsResponse> => {
  const {
    artifact_id: artifactID,
    page,
    page_size: pageSize,
    run_id: runID,
    scanner,
    space_id: spaceID,
    status,
    thread_id: threadID,
  } = request;

  try {
    return pageResponse(
      presentTaskThreadArtifactScanJobListResponse(
        await canonicalThreadClient.listArtifactScanJobs({
          artifact_id: artifactID,
          page,
          page_size: pageSize,
          run_id: runID,
          scanner,
          space_id: resolvePageServiceSpaceID(spaceID),
          status,
          thread_id: threadID,
        }),
      ),
    );
  } catch (error) {
    return artifactRequestError('读取产物扫描队列失败', error);
  }
};

export const retryTaskThreadArtifactScanJob = async (
  request: ArtifactScanJobRetryRequest,
): Promise<RetryTaskThreadArtifactScanJobResponse> => {
  const { job_id: jobID, space_id: spaceID, thread_id: threadID } = request;

  try {
    return pageResponse(
      presentTaskThreadArtifactScanRetryResponse(
        await canonicalThreadClient.retryArtifactScanJob({
          job_id: jobID,
          space_id: resolvePageServiceSpaceID(spaceID),
          thread_id: threadID,
        }),
      ),
    );
  } catch (error) {
    return artifactRequestError('重试产物扫描任务失败', error);
  }
};

export const reviewTaskThreadArtifactScan = async (
  request: ArtifactScanReviewRequest,
): Promise<ReviewTaskThreadArtifactScanResponse> => {
  const {
    artifact_id: artifactID,
    decision,
    reason,
    space_id: spaceID,
    thread_id: threadID,
  } = request;

  try {
    return pageResponse(
      presentTaskThreadArtifactScanReviewResponse(
        await canonicalThreadClient.reviewArtifactScan({
          artifact_id: artifactID,
          decision,
          reason,
          space_id: resolvePageServiceSpaceID(spaceID),
          thread_id: threadID,
        }),
      ),
    );
  } catch (error) {
    return artifactRequestError('审核产物扫描状态失败', error);
  }
};

export const fetchTaskThreadArtifactContent = async (
  request: ArtifactContentRequest,
): Promise<TaskThreadArtifactContentResponse> => {
  const {
    artifact_id: artifactID,
    mode,
    space_id: spaceID,
    thread_id: threadID,
  } = request;

  try {
    return presentTaskThreadArtifactContentResponse(
      await canonicalThreadClient.getArtifactContent({
        artifact_id: artifactID,
        mode,
        space_id: resolvePageServiceSpaceID(spaceID),
        thread_id: threadID,
      }),
    );
  } catch (error) {
    return artifactRequestError('读取任务产物失败', error);
  }
};

export const getTaskThreadArtifactSignedURL = async (
  request: ArtifactSignedURLRequest,
): Promise<TaskThreadArtifactSignedURLResponse> => {
  const {
    artifact_id: artifactID,
    mode,
    space_id: spaceID,
    thread_id: threadID,
    ttl_seconds: ttlSeconds,
  } = request;

  try {
    return presentTaskThreadArtifactSignedURLResponse(
      await canonicalThreadClient.getArtifactSignedURL({
        artifact_id: artifactID,
        mode,
        space_id: resolvePageServiceSpaceID(spaceID),
        thread_id: threadID,
        ttl_seconds: ttlSeconds,
      }),
    );
  } catch (error) {
    throw taskThreadArtifactPayloadError(
      artifactErrorCode(error),
      '生成任务产物签名链接失败',
      mode,
    );
  }
};

export const deleteTaskThreadArtifact = async (
  request: ArtifactMutationRequest,
): Promise<void> => {
  const {
    artifact_id: artifactID,
    space_id: spaceID,
    thread_id: threadID,
  } = request;

  try {
    await canonicalThreadClient.deleteArtifact({
      artifact_id: artifactID,
      space_id: resolvePageServiceSpaceID(spaceID),
      thread_id: threadID,
    });
  } catch (error) {
    return artifactRequestError('删除任务产物失败', error);
  }
};

export const restoreTaskThreadArtifact = async (
  request: ArtifactMutationRequest,
): Promise<RestoreTaskThreadArtifactResponse> => {
  const {
    artifact_id: artifactID,
    space_id: spaceID,
    thread_id: threadID,
  } = request;

  try {
    return presentTaskThreadArtifactRestoreResponse(
      await canonicalThreadClient.restoreArtifact({
        artifact_id: artifactID,
        space_id: resolvePageServiceSpaceID(spaceID),
        thread_id: threadID,
      }),
    );
  } catch (error) {
    return artifactRequestError('恢复任务产物失败', error);
  }
};
