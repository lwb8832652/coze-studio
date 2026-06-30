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
  workbenchTask,
} from '@coze-studio/api-schema';

export const listTasks = workbenchTask.ListTasks;
export const getTask = workbenchTask.GetTask;
export const listTaskThreads = workbenchTask.ListTaskThreads;
export const createTaskThread = workbenchTask.CreateTaskThread;
export const getTaskThread = workbenchTask.GetTaskThread;
export const listTaskThreadMessages = workbenchTask.ListTaskThreadMessages;
export const appendTaskThreadMessage = workbenchTask.AppendTaskThreadMessage;
export const listTaskThreadRuns = workbenchTask.ListTaskThreadRuns;
export const createTaskThreadRun = workbenchTask.CreateTaskThreadRun;
export const resumeTaskThreadRun = workbenchTask.ResumeTaskThreadRun;
export const cancelTaskThreadRun = workbenchTask.CancelTaskThreadRun;
export const retryTaskThreadSubagentRun =
  workbenchTask.RetryTaskThreadSubagentRun;
export const listTaskThreadRunEvents = workbenchTask.ListTaskThreadRunEvents;
export const getTaskThreadTokenUsage = workbenchTask.GetTaskThreadTokenUsage;
export const listTaskThreadArtifacts = workbenchTask.ListTaskThreadArtifacts;
export const listTaskThreadMemories = workbenchTask.ListTaskThreadMemories;
export const updateTaskThreadMemory = workbenchTask.UpdateTaskThreadMemory;
export const deleteTaskThreadMemory = workbenchTask.DeleteTaskThreadMemory;
export const clearTaskThreadMemories = workbenchTask.ClearTaskThreadMemories;
export const restoreTaskThreadMemory = workbenchTask.RestoreTaskThreadMemory;
export const listTaskThreadMemoryAuditEvents =
  workbenchTask.ListTaskThreadMemoryAuditEvents;
export const listTaskThreadGuardrailAuditEvents =
  workbenchTask.ListTaskThreadGuardrailAuditEvents;
export const exportTaskThreadMemories = workbenchTask.ExportTaskThreadMemories;
export const exportTaskThreadGuardrailAuditEvents =
  workbenchTask.ExportTaskThreadGuardrailAuditEvents;
export const importTaskThreadMemories = workbenchTask.ImportTaskThreadMemories;
export const cancelTask = workbenchTask.CancelTask;
export const retryTask = workbenchTask.RetryTask;
export const listTaskEvents = workbenchTask.ListTaskEvents;
export const sendWorkbenchChat = workbench.WorkbenchChat;
export const getWorkbenchRuntimeDoctor = workbench.GetWorkbenchRuntimeDoctor;
export const installSkillFromArtifact = workbenchSkill.InstallSkillFromArtifact;
export type TaskThreadMemory = workbenchTask.TaskThreadMemory;
export type TaskThreadMemoryAuditEvent =
  workbenchTask.TaskThreadMemoryAuditEvent;
export type TaskThreadGuardrailAuditEvent =
  workbenchTask.TaskThreadGuardrailAuditEvent;
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

export const listTaskThreadArtifactScanJobs = async ({
  artifact_id: artifactID,
  page,
  page_size: pageSize,
  run_id: runID,
  scanner,
  space_id: spaceID,
  status,
  thread_id: threadID,
}: {
  thread_id: string;
  run_id?: string;
  artifact_id?: string;
  space_id?: string;
  status?: string;
  scanner?: string;
  page?: number;
  page_size?: number;
}): Promise<ListTaskThreadArtifactScanJobsResponse> => {
  const params = new URLSearchParams();
  if (runID) {
    params.set('run_id', runID);
  }
  if (artifactID) {
    params.set('artifact_id', artifactID);
  }
  if (status) {
    params.set('status', status);
  }
  if (scanner) {
    params.set('scanner', scanner);
  }
  if (spaceID) {
    params.set('space_id', spaceID);
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
    )}/artifact_scan_jobs${query ? `?${query}` : ''}`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'GET',
    },
  );

  if (!response.ok) {
    throw new Error('读取产物扫描队列失败');
  }

  const payload =
    (await response.json()) as ListTaskThreadArtifactScanJobsResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '读取产物扫描队列失败');
  }

  return payload;
};

export const retryTaskThreadArtifactScanJob = async ({
  job_id: jobID,
  space_id: spaceID,
  thread_id: threadID,
}: {
  thread_id: string;
  job_id: string;
  space_id?: string;
}): Promise<RetryTaskThreadArtifactScanJobResponse> => {
  const params = new URLSearchParams();
  if (spaceID) {
    params.set('space_id', spaceID);
  }
  const query = params.toString();
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/artifact_scan_jobs/${encodeURIComponent(jobID)}/retry${
      query ? `?${query}` : ''
    }`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'POST',
    },
  );

  if (!response.ok) {
    throw new Error('重试产物扫描任务失败');
  }

  const payload =
    (await response.json()) as RetryTaskThreadArtifactScanJobResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '重试产物扫描任务失败');
  }

  return payload;
};

export const reviewTaskThreadArtifactScan = async ({
  artifact_id: artifactID,
  decision,
  reason,
  space_id: spaceID,
  thread_id: threadID,
}: {
  thread_id: string;
  artifact_id: string;
  space_id?: string;
  decision: ArtifactScanReviewDecision;
  reason?: string;
}): Promise<ReviewTaskThreadArtifactScanResponse> => {
  const body =
    reason && reason.trim()
      ? { decision, reason: reason.trim() }
      : { decision };
  const params = new URLSearchParams();
  if (spaceID) {
    params.set('space_id', spaceID);
  }
  const query = params.toString();
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/artifacts/${encodeURIComponent(artifactID)}/scan_review${
      query ? `?${query}` : ''
    }`,
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
    throw new Error('审核产物扫描状态失败');
  }

  const payload =
    (await response.json()) as ReviewTaskThreadArtifactScanResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '审核产物扫描状态失败');
  }

  return payload;
};

export const fetchTaskThreadArtifactContent = async ({
  artifact_id: artifactID,
  mode,
  space_id: spaceID,
  thread_id: threadID,
}: {
  thread_id: string;
  artifact_id: string;
  space_id?: string;
  mode: 'preview' | 'download';
}): Promise<TaskThreadArtifactContentResponse> => {
  const params = new URLSearchParams();
  params.set('mode', mode);
  if (spaceID) {
    params.set('space_id', spaceID);
  }
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/artifacts/${encodeURIComponent(artifactID)}/content?${params.toString()}`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'GET',
    },
  );

  if (!response.ok) {
    throw new Error('读取任务产物失败');
  }

  return {
    blob: await response.blob(),
    contentDisposition: response.headers.get('content-disposition') ?? '',
    contentType: response.headers.get('content-type') ?? '',
  };
};

export const getTaskThreadArtifactSignedURL = async ({
  artifact_id: artifactID,
  mode,
  space_id: spaceID,
  thread_id: threadID,
  ttl_seconds: ttlSeconds,
}: {
  thread_id: string;
  artifact_id: string;
  space_id?: string;
  mode: 'preview' | 'download';
  ttl_seconds?: number;
}): Promise<TaskThreadArtifactSignedURLResponse> => {
  const params = new URLSearchParams();
  params.set('mode', mode);
  if (spaceID) {
    params.set('space_id', spaceID);
  }
  if (ttlSeconds) {
    params.set('ttl_seconds', String(ttlSeconds));
  }
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/artifacts/${encodeURIComponent(
      artifactID,
    )}/signed_url?${params.toString()}`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'GET',
    },
  );

  if (!response.ok) {
    throw new Error('生成任务产物签名链接失败');
  }

  const payload =
    (await response.json()) as TaskThreadArtifactSignedURLResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '生成任务产物签名链接失败');
  }

  return payload;
};

export const deleteTaskThreadArtifact = async ({
  artifact_id: artifactID,
  space_id: spaceID,
  thread_id: threadID,
}: {
  thread_id: string;
  artifact_id: string;
  space_id?: string;
}) => {
  const params = new URLSearchParams();
  if (spaceID) {
    params.set('space_id', spaceID);
  }
  const query = params.toString();
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/artifacts/${encodeURIComponent(artifactID)}${query ? `?${query}` : ''}`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'DELETE',
    },
  );

  if (!response.ok) {
    throw new Error('删除任务产物失败');
  }

  if (response.headers.get('content-type')?.includes('application/json')) {
    const payload = (await response.json()) as {
      code?: number;
      msg?: string;
    };
    if (typeof payload.code === 'number' && payload.code !== 0) {
      throw new Error(payload.msg || '删除任务产物失败');
    }
  }
};

export const restoreTaskThreadArtifact = async ({
  artifact_id: artifactID,
  space_id: spaceID,
  thread_id: threadID,
}: {
  thread_id: string;
  artifact_id: string;
  space_id?: string;
}): Promise<RestoreTaskThreadArtifactResponse> => {
  const params = new URLSearchParams();
  if (spaceID) {
    params.set('space_id', spaceID);
  }
  const query = params.toString();
  const response = await fetch(
    `/api/workbench/task_threads/${encodeURIComponent(
      threadID,
    )}/artifacts/${encodeURIComponent(artifactID)}/restore${
      query ? `?${query}` : ''
    }`,
    {
      headers: {
        'x-requested-with': 'XMLHttpRequest',
      },
      method: 'POST',
    },
  );

  if (!response.ok) {
    throw new Error('恢复任务产物失败');
  }

  const payload = (await response.json()) as RestoreTaskThreadArtifactResponse;
  if (typeof payload.code === 'number' && payload.code !== 0) {
    throw new Error(payload.msg || '恢复任务产物失败');
  }

  return payload;
};
