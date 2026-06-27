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

export interface TaskThreadMemory {
  memory_id: string;
  thread_id: string;
  run_id: string;
  space_id: string;
  scope: string;
  content: string;
  metadata: string;
  score: number;
  confidence: number;
  source_type: string;
  source_id: string;
  correction_of_memory_id: string;
  corrected_at: number;
  expires_at: number;
  created_at: number;
  updated_at: number;
  deleted_at: number;
}

export interface ListTaskThreadMemoriesResponse {
  data?: {
    memories: TaskThreadMemory[];
    total: number;
  };
  code: number;
  msg: string;
}

export interface TaskThreadMemoryAuditEvent {
  actor_id: string;
  affected_count: number;
  created_at: number;
  event_id: string;
  event_type: string;
  memory_id: string;
  run_id: string;
  scope: string;
  source_id: string;
  source_type: string;
  space_id: string;
  thread_id: string;
}

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
  data?: {
    deleted: boolean;
  };
  code: number;
  msg: string;
}

export interface ClearTaskThreadMemoriesResponse {
  data?: {
    cleared_count: number;
  };
  code: number;
  msg: string;
}

export const listTaskThreadMemories = async ({
  include_expired: includeExpired,
  page,
  page_size: pageSize,
  q,
  run_id: runID,
  scope,
  scopes,
  thread_id: threadID,
  include_deleted: includeDeleted,
}: {
  thread_id: string;
  run_id?: string;
  scope?: string;
  scopes?: string[];
  q?: string;
  include_expired?: boolean;
  include_deleted?: boolean;
  page?: number;
  page_size?: number;
}): Promise<ListTaskThreadMemoriesResponse> => {
  const params = new URLSearchParams();
  if (q?.trim()) {
    params.set('q', q.trim());
  }
  if (runID) {
    params.set('run_id', runID);
  }
  if (scope) {
    params.set('scope', scope);
  }
  for (const item of scopes ?? []) {
    if (item) {
      params.append('scopes', item);
    }
  }
  if (includeExpired) {
    params.set('include_expired', 'true');
  }
  if (includeDeleted) {
    params.set('include_deleted', 'true');
  }
  if (page) {
    params.set('page', String(page));
  }
  if (pageSize) {
    params.set('page_size', String(pageSize));
  }

  const query = params.toString();
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/memories${query ? `?${query}` : ''}`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'GET',
    },
  );

  if (!response.ok) {
    throw new Error('读取任务记忆失败');
  }

  const payload = (await response.json()) as ListTaskThreadMemoriesResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '读取任务记忆失败');
  }

  return payload;
};

export const listTaskThreadMemoryAuditEvents = async ({
  memory_id: memoryID,
  page,
  page_size: pageSize,
  thread_id: threadID,
}: {
  thread_id: string;
  memory_id?: string;
  page?: number;
  page_size?: number;
}): Promise<ListTaskThreadMemoryAuditEventsResponse> => {
  const params = new URLSearchParams();
  if (memoryID) {
    params.set('memory_id', memoryID);
  }
  if (page) {
    params.set('page', String(page));
  }
  if (pageSize) {
    params.set('page_size', String(pageSize));
  }

  const query = params.toString();
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/memories/audit_events${query ? `?${query}` : ''}`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'GET',
    },
  );

  if (!response.ok) {
    throw new Error('读取记忆审计失败');
  }

  const payload =
    (await response.json()) as ListTaskThreadMemoryAuditEventsResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '读取记忆审计失败');
  }

  return payload;
};

export const updateTaskThreadMemory = async ({
  confidence,
  content,
  corrected_at: correctedAt,
  correction_of_memory_id: correctionOfMemoryID,
  expires_at: expiresAt,
  memory_id: memoryID,
  metadata,
  run_id: runID,
  scope,
  score,
  source_id: sourceID,
  source_type: sourceType,
  thread_id: threadID,
}: {
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
}): Promise<UpdateTaskThreadMemoryResponse> => {
  const body: Record<string, string | number> = {};
  if (confidence !== undefined) {
    body.confidence = confidence;
  }
  body.content = content;
  if (correctionOfMemoryID) {
    body.correction_of_memory_id = correctionOfMemoryID;
  }
  if (correctedAt) {
    body.corrected_at = correctedAt;
  }
  if (expiresAt) {
    body.expires_at = expiresAt;
  }
  if (metadata !== undefined) {
    body.metadata = metadata;
  }
  if (runID) {
    body.run_id = runID;
  }
  body.scope = scope;
  if (score !== undefined) {
    body.score = score;
  }
  if (sourceID) {
    body.source_id = sourceID;
  }
  if (sourceType) {
    body.source_type = sourceType;
  }

  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/memories/${encodeURIComponent(memoryID)}`,
    {
      body: JSON.stringify(body),
      headers: {
        'content-type': 'application/json',
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'PUT',
    },
  );

  if (!response.ok) {
    throw new Error('更新任务记忆失败');
  }

  const payload = (await response.json()) as UpdateTaskThreadMemoryResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '更新任务记忆失败');
  }

  return payload;
};

export const restoreTaskThreadMemory = async ({
  memory_id: memoryID,
  thread_id: threadID,
}: {
  thread_id: string;
  memory_id: string;
}): Promise<RestoreTaskThreadMemoryResponse> => {
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/memories/${encodeURIComponent(memoryID)}/restore`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'POST',
    },
  );

  if (!response.ok) {
    throw new Error('恢复任务记忆失败');
  }

  const payload = (await response.json()) as RestoreTaskThreadMemoryResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '恢复任务记忆失败');
  }

  return payload;
};

export const deleteTaskThreadMemory = async ({
  memory_id: memoryID,
  thread_id: threadID,
}: {
  thread_id: string;
  memory_id: string;
}): Promise<DeleteTaskThreadMemoryResponse> => {
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/memories/${encodeURIComponent(memoryID)}`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'DELETE',
    },
  );

  if (!response.ok) {
    throw new Error('删除任务记忆失败');
  }

  const payload = (await response.json()) as DeleteTaskThreadMemoryResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '删除任务记忆失败');
  }

  return payload;
};

export const clearTaskThreadMemories = async ({
  run_id: runID,
  scopes,
  thread_id: threadID,
}: {
  thread_id: string;
  run_id?: string;
  scopes?: string[];
}): Promise<ClearTaskThreadMemoriesResponse> => {
  const body: Record<string, string | string[]> = {};
  if (runID) {
    body.run_id = runID;
  }
  if (scopes?.length) {
    body.scopes = scopes;
  }

  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(threadID)}/memories/clear`,
    {
      body: JSON.stringify(body),
      headers: {
        'content-type': 'application/json',
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'POST',
    },
  );

  if (!response.ok) {
    throw new Error('清空任务记忆失败');
  }

  const payload = (await response.json()) as ClearTaskThreadMemoriesResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '清空任务记忆失败');
  }

  return payload;
};
