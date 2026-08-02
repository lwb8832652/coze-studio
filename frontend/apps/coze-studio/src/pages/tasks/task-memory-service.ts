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
  presentTaskThreadMemoryAuditListResponse,
  presentTaskThreadMemoryClearResponse,
  presentTaskThreadMemoryDeleteResponse,
  presentTaskThreadMemoryListResponse,
  presentTaskThreadMemoryRestoreResponse,
  presentTaskThreadMemoryUpdateResponse,
} from '../workbench/thread-client/legacy-page-response';
import {
  canonicalThreadClient,
  resolvePageServiceSpaceID,
} from '../workbench/thread-client/canonical-thread-client-singleton';
import type {
  WorkbenchMemory,
  WorkbenchMemoryAuditEvent,
} from '../workbench/thread-client';

export type TaskThreadMemory = WorkbenchMemory;

export interface ListTaskThreadMemoriesResponse {
  data?: {
    memories: TaskThreadMemory[];
    total: number;
  };
  code: number;
  msg: string;
}

export type TaskThreadMemoryAuditEvent = WorkbenchMemoryAuditEvent;

export interface ListTaskThreadMemoryAuditEventsResponse {
  data?: {
    events: TaskThreadMemoryAuditEvent[];
    total: number;
  };
  code: number;
  msg: string;
}

export interface UpdateTaskThreadMemoryResponse {
  data?: {
    memory?: TaskThreadMemory;
    updated: boolean;
  };
  code: number;
  msg: string;
}

export interface RestoreTaskThreadMemoryResponse {
  data?: {
    memory?: TaskThreadMemory;
    restored: boolean;
  };
  code: number;
  msg: string;
}

export interface DeleteTaskThreadMemoryResponse {
  code: number;
  msg: string;
}

export interface ClearTaskThreadMemoriesResponse {
  data?: {
    deleted: number;
  };
  code: number;
  msg: string;
}

interface PageScopedRequest {
  space_id?: string;
}

const pageResponse = <Response>(value: unknown): Response => value as Response;

const memoryRequestError = (message: string, cause: unknown): never => {
  if (
    cause &&
    typeof cause === 'object' &&
    'name' in cause &&
    cause.name === 'AbortError'
  ) {
    throw cause;
  }
  const error = new Error(message);
  (error as Error & { cause?: unknown }).cause = cause;
  throw error;
};

export const listTaskThreadMemories = async (
  request: {
    thread_id: string;
    run_id?: string;
    scope?: string;
    scopes?: string[];
    q?: string;
    include_expired?: boolean;
    include_deleted?: boolean;
    page?: number;
    page_size?: number;
  } & PageScopedRequest,
): Promise<ListTaskThreadMemoriesResponse> => {
  try {
    return pageResponse(
      presentTaskThreadMemoryListResponse(
        await canonicalThreadClient.listMemories({
          ...request,
          space_id: resolvePageServiceSpaceID(request.space_id),
        }),
      ),
    );
  } catch (error) {
    return memoryRequestError('读取任务记忆失败', error);
  }
};

export const listTaskThreadMemoryAuditEvents = async (
  request: {
    thread_id: string;
    memory_id?: string;
    page?: number;
    page_size?: number;
  } & PageScopedRequest,
): Promise<ListTaskThreadMemoryAuditEventsResponse> => {
  try {
    return pageResponse(
      presentTaskThreadMemoryAuditListResponse(
        await canonicalThreadClient.listMemoryAuditEvents({
          ...request,
          space_id: resolvePageServiceSpaceID(request.space_id),
        }),
      ),
    );
  } catch (error) {
    return memoryRequestError('读取记忆审计失败', error);
  }
};

export const updateTaskThreadMemory = async (
  request: {
    thread_id: string;
    memory_id: string;
    run_id?: string;
    scope: string;
    content: string;
    metadata?: string;
    score?: number;
    confidence?: number;
    source_type?: string;
    source_id?: string;
    correction_of_memory_id?: string;
    corrected_at?: number;
    expires_at?: number;
  } & PageScopedRequest,
): Promise<UpdateTaskThreadMemoryResponse> => {
  try {
    return pageResponse(
      presentTaskThreadMemoryUpdateResponse(
        await canonicalThreadClient.updateMemory({
          ...request,
          space_id: resolvePageServiceSpaceID(request.space_id),
        }),
      ),
    );
  } catch (error) {
    return memoryRequestError('更新任务记忆失败', error);
  }
};

export const restoreTaskThreadMemory = async (
  request: {
    thread_id: string;
    memory_id: string;
  } & PageScopedRequest,
): Promise<RestoreTaskThreadMemoryResponse> => {
  try {
    return pageResponse(
      presentTaskThreadMemoryRestoreResponse(
        await canonicalThreadClient.restoreMemory({
          ...request,
          space_id: resolvePageServiceSpaceID(request.space_id),
        }),
      ),
    );
  } catch (error) {
    return memoryRequestError('恢复任务记忆失败', error);
  }
};

export const deleteTaskThreadMemory = async (
  request: {
    thread_id: string;
    memory_id: string;
  } & PageScopedRequest,
): Promise<DeleteTaskThreadMemoryResponse> => {
  try {
    await canonicalThreadClient.deleteMemory({
      ...request,
      space_id: resolvePageServiceSpaceID(request.space_id),
    });

    return presentTaskThreadMemoryDeleteResponse();
  } catch (error) {
    return memoryRequestError('删除任务记忆失败', error);
  }
};

export const clearTaskThreadMemories = async (
  request: {
    thread_id: string;
    run_id?: string;
    scopes?: string[];
  } & PageScopedRequest,
): Promise<ClearTaskThreadMemoriesResponse> => {
  try {
    return presentTaskThreadMemoryClearResponse(
      await canonicalThreadClient.clearMemories({
        ...request,
        space_id: resolvePageServiceSpaceID(request.space_id),
      }),
    );
  } catch (error) {
    return memoryRequestError('清空任务记忆失败', error);
  }
};
