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

/* eslint-disable max-lines -- Cohesive orchestrator. */

import type {
  AppDevApiResponse,
  AppDevBuildResult,
  AppDevChatAttachment,
  AppDevChatMessage,
  AppDevChatSession,
  AppDevChatStatus,
  AppDevCreateProjectParams,
  AppDevDataSource,
  AppDevFileContent,
  AppDevFileNode,
  AppDevModel,
  AppDevPageResult,
  AppDevProject,
  AppDevProjectIdentity,
  AppDevRuntimeInfo,
  AppDevRuntimeLog,
  AppDevSnapshot,
} from './types';

const jsonHeaders = {
  'content-type': 'application/json',
};

const buildProjectBasePath = ({ spaceId, projectId }: AppDevProjectIdentity) =>
  `/api/app-dev/spaces/${encodeURIComponent(
    spaceId,
  )}/projects/${encodeURIComponent(projectId)}`;

const buildQuery = (params: Record<string, string | undefined>) => {
  const searchParams = new URLSearchParams();

  Object.entries(params).forEach(([key, value]) => {
    if (value?.trim()) {
      searchParams.set(key, value.trim());
    }
  });

  const query = searchParams.toString();
  return query ? `?${query}` : '';
};

export const normalizeAppDevError = (error: unknown) => {
  if (error instanceof Error && error.message) {
    const { message } = error;
    const lowerMessage = message.toLowerCase();
    if (lowerMessage.includes('workspace does not allow development')) {
      return '当前工作空间未开启网页应用开发，请在“工作空间”设置中开启后重试';
    }
    if (
      lowerMessage.includes('missing session_key') ||
      lowerMessage.includes('missing user session') ||
      lowerMessage.includes('unauthorized') ||
      message.includes('请求失败：401')
    ) {
      return '登录状态已失效，请重新登录后重试';
    }
    if (
      lowerMessage.includes('workspace') &&
      (lowerMessage.includes('not found') ||
        lowerMessage.includes('permission'))
    ) {
      return '当前工作空间不可访问，请切换工作空间或刷新页面后重试';
    }
    if (message.includes('请求失败：404')) {
      return '当前工作空间或 AppDev 接口不可访问，请切换工作空间或刷新页面后重试';
    }

    return error.message;
  }

  if (typeof error === 'string' && error) {
    return error;
  }

  return '请求失败，请稍后重试';
};

const readJson = async <T>(
  response: Response,
): Promise<AppDevApiResponse<T>> => {
  if (response.headers.get('content-type')?.includes('application/json')) {
    const payload: AppDevApiResponse<T> = await response.json();
    return payload;
  }

  const payload: AppDevApiResponse<T> = {};
  return payload;
};

const requestAppDev = async <T>(
  path: string,
  init?: RequestInit,
): Promise<T> => {
  const response = await fetch(path, init);
  const payload = await readJson<T>(response);

  if (!response.ok || (payload.code !== undefined && payload.code !== 0)) {
    throw new Error(
      payload.message || payload.msg || `请求失败：${response.status}`,
    );
  }

  return payload.data as T;
};

export const listAppDevProjects = async ({
  spaceId,
  keyword,
}: {
  spaceId: string;
  keyword?: string;
}) =>
  requestAppDev<AppDevPageResult<AppDevProject>>(
    `/api/app-dev/spaces/${encodeURIComponent(
      spaceId,
    )}/projects${buildQuery({ keyword })}`,
  );

export const createAppDevProject = async ({
  spaceId,
  name,
  prompt,
}: AppDevCreateProjectParams) =>
  requestAppDev<AppDevProject>(
    `/api/app-dev/spaces/${encodeURIComponent(spaceId)}/projects`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ name, prompt }),
    },
  );

export const importAppDevProject = async ({
  spaceId,
  file,
  name,
}: {
  spaceId: string;
  file: File;
  name?: string;
}) => {
  const formData = new FormData();
  formData.set('file', file);
  if (name?.trim()) {
    formData.set('name', name.trim());
  }

  return requestAppDev<AppDevProject>(
    `/api/app-dev/spaces/${encodeURIComponent(spaceId)}/projects/import`,
    {
      method: 'POST',
      body: formData,
    },
  );
};

export const archiveAppDevProject = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<{ success: boolean }>(
    buildProjectBasePath({ spaceId, projectId }),
    {
      method: 'DELETE',
      headers: jsonHeaders,
    },
  );

export const exportAppDevProject = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) => {
  const response = await fetch(
    `${buildProjectBasePath({ spaceId, projectId })}/export`,
    {
      credentials: 'include',
    },
  );
  if (!response.ok) {
    const payload = await readJson<unknown>(response);
    throw new Error(
      payload.message || payload.msg || `导出失败：${response.status}`,
    );
  }

  return response.blob();
};

export const buildAppDevProject = async ({
  spaceId,
  projectId,
  publishType = 'PAGE',
}: AppDevProjectIdentity & {
  publishType?: string;
}) =>
  requestAppDev<AppDevBuildResult>(
    `${buildProjectBasePath({ spaceId, projectId })}/build`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ publishType }),
    },
  );

export const downloadAppDevRelease = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) => {
  const response = await fetch(
    `${buildProjectBasePath({ spaceId, projectId })}/release`,
    {
      credentials: 'include',
    },
  );
  if (!response.ok) {
    const payload = await readJson<unknown>(response);
    throw new Error(
      payload.message || payload.msg || `发布产物下载失败：${response.status}`,
    );
  }

  return response.blob();
};

export const getAppDevProject = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevProject>(buildProjectBasePath({ spaceId, projectId }));

export const updateAppDevProject = async ({
  spaceId,
  projectId,
  name,
  description,
}: AppDevProjectIdentity & {
  name: string;
  description?: string;
}) =>
  requestAppDev<AppDevProject>(buildProjectBasePath({ spaceId, projectId }), {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({ name, description }),
  });

export const duplicateAppDevProject = async ({
  spaceId,
  projectId,
  name,
}: AppDevProjectIdentity & {
  name?: string;
}) =>
  requestAppDev<AppDevProject>(
    `${buildProjectBasePath({ spaceId, projectId })}/duplicate`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ name }),
    },
  );

export const listAppDevFiles = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<{ items: AppDevFileNode[] }>(
    `${buildProjectBasePath({ spaceId, projectId })}/files`,
  );

export const getAppDevFileContent = async ({
  spaceId,
  projectId,
  path,
}: AppDevProjectIdentity & { path: string }) =>
  requestAppDev<AppDevFileContent>(
    `${buildProjectBasePath({ spaceId, projectId })}/files/content${buildQuery({
      path,
    })}`,
  );

export const saveAppDevFileContent = async ({
  spaceId,
  projectId,
  path,
  content,
  version,
}: AppDevProjectIdentity & {
  path: string;
  content: string;
  version?: string;
}) =>
  requestAppDev<AppDevFileContent>(
    `${buildProjectBasePath({ spaceId, projectId })}/files/content`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ path, content, version }),
    },
  );

export const uploadAppDevFiles = async ({
  spaceId,
  projectId,
  files,
  filePaths,
}: AppDevProjectIdentity & {
  files: File[];
  filePaths: string[];
}) => {
  const formData = new FormData();

  files.forEach(file => formData.append('files', file));
  filePaths.forEach(filePath => formData.append('filePaths', filePath));

  return requestAppDev<{ uploaded: number }>(
    `${buildProjectBasePath({ spaceId, projectId })}/files/upload`,
    {
      method: 'POST',
      body: formData,
    },
  );
};

export const deleteAppDevFile = async ({
  spaceId,
  projectId,
  path,
}: AppDevProjectIdentity & { path: string }) =>
  requestAppDev<{ success: boolean }>(
    `${buildProjectBasePath({ spaceId, projectId })}/files`,
    {
      method: 'DELETE',
      headers: jsonHeaders,
      body: JSON.stringify({ path }),
    },
  );

export const renameAppDevFile = async ({
  spaceId,
  projectId,
  sourcePath,
  targetPath,
}: AppDevProjectIdentity & {
  sourcePath: string;
  targetPath: string;
}) =>
  requestAppDev<{ success: boolean }>(
    `${buildProjectBasePath({ spaceId, projectId })}/files/rename`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ sourcePath, targetPath }),
    },
  );

export const listAppDevSnapshots = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<{ items: AppDevSnapshot[] }>(
    `${buildProjectBasePath({ spaceId, projectId })}/snapshots`,
  );

export const createAppDevSnapshot = async ({
  spaceId,
  projectId,
  label,
}: AppDevProjectIdentity & { label?: string }) =>
  requestAppDev<AppDevSnapshot>(
    `${buildProjectBasePath({ spaceId, projectId })}/snapshots`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ label }),
    },
  );

export const restoreAppDevSnapshot = async ({
  spaceId,
  projectId,
  snapshotId,
}: AppDevProjectIdentity & { snapshotId: string }) =>
  requestAppDev<{ success: boolean }>(
    `${buildProjectBasePath({
      spaceId,
      projectId,
    })}/snapshots/${encodeURIComponent(snapshotId)}/restore`,
    {
      method: 'POST',
      headers: jsonHeaders,
    },
  );

export const getAppDevRuntimeStatus = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/status`,
  );

export const startAppDevRuntime = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/start`,
    {
      method: 'POST',
      headers: jsonHeaders,
    },
  );

export const keepAliveAppDevRuntime = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/keep-alive`,
    {
      method: 'POST',
      headers: jsonHeaders,
    },
  );

export const restartAppDevRuntime = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/restart`,
    {
      method: 'POST',
      headers: jsonHeaders,
    },
  );

export const stopAppDevRuntime = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/stop`,
    {
      method: 'POST',
      headers: jsonHeaders,
    },
  );

export const listAppDevRuntimeLogs = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<{ items: AppDevRuntimeLog[] }>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/logs`,
  );

export const listAppDevModels = async ({ spaceId }: { spaceId: string }) =>
  requestAppDev<{ items: Array<AppDevModel & Record<string, unknown>> }>(
    `/api/app-dev/spaces/${encodeURIComponent(
      spaceId,
    )}/models${buildQuery({ scenario: 'PageApp' })}`,
  ).then(result => ({
    ...result,
    items: (result.items || [])
      .map(model => ({
        id: String(model.id || ''),
        name: String(model.name || '未命名模型'),
        provider:
          typeof model.provider === 'string' ? model.provider : undefined,
        protocol:
          typeof model.protocol === 'string' ? model.protocol : undefined,
        supportsMultiModal: Boolean(
          model.supportsMultiModal ?? model.supports_multi_modal,
        ),
        supportsImageUnderstanding: Boolean(
          model.supportsImageUnderstanding ??
            model.supports_image_understanding,
        ),
        enableBase64URL: Boolean(
          model.enableBase64URL ?? model.enable_base64_url,
        ),
      }))
      .filter(model => model.id),
  }));

export const listAppDevDataSources = async ({
  spaceId,
}: {
  spaceId: string;
}) => {
  const response = await fetch('/api/knowledge/list', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      space_id: spaceId,
      page: 1,
      size: 50,
    }),
  });
  const payload = (await readJson<unknown>(response)) as {
    code?: number;
    message?: string;
    msg?: string;
    data?: {
      dataset_list?: Array<Record<string, unknown>>;
      items?: Array<Record<string, unknown>>;
      list?: Array<Record<string, unknown>>;
    };
    dataset_list?: Array<Record<string, unknown>>;
    items?: Array<Record<string, unknown>>;
    list?: Array<Record<string, unknown>>;
  };
  if (!response.ok || (payload.code !== undefined && payload.code !== 0)) {
    throw new Error(
      payload.message || payload.msg || `数据源加载失败：${response.status}`,
    );
  }

  const datasets =
    payload.data?.dataset_list ||
    payload.data?.items ||
    payload.data?.list ||
    payload.dataset_list ||
    payload.items ||
    payload.list ||
    [];
  const items = datasets
    .map(dataset => ({
      id: String(dataset.dataset_id || dataset.id || ''),
      name: String(dataset.name || dataset.dataset_name || '未命名数据源'),
      type: String(dataset.format_type || dataset.type || 'knowledge'),
      description: String(dataset.description || ''),
    }))
    .filter(dataset => dataset.id) as AppDevDataSource[];

  return {
    items,
    total: items.length,
  };
};

export const buildAppDevEventsUrl = ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  `${buildProjectBasePath({ spaceId, projectId })}/chat/events`;

export const sendAppDevChatMessage = async ({
  spaceId,
  projectId,
  message,
  modelId,
  dataSources,
  attachments,
}: AppDevProjectIdentity & {
  message: string;
  modelId?: string;
  dataSources?: AppDevDataSource[];
  attachments?: AppDevChatAttachment[];
}) =>
  requestAppDev<AppDevChatSession>(
    `${buildProjectBasePath({ spaceId, projectId })}/chat`,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ message, modelId, dataSources, attachments }),
    },
  );

export const cancelAppDevChat = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevChatStatus>(
    `${buildProjectBasePath({ spaceId, projectId })}/chat/cancel`,
    {
      method: 'POST',
      headers: jsonHeaders,
    },
  );

export const listAppDevChatHistory = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<{ items: AppDevChatMessage[] }>(
    `${buildProjectBasePath({ spaceId, projectId })}/chat/history`,
  );

export const getAppDevChatStatus = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevChatStatus>(
    `${buildProjectBasePath({ spaceId, projectId })}/chat/status`,
  );
