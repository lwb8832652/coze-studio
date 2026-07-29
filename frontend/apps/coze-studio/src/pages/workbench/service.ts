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
  ListWorkspaceModels,
  WorkspaceModelScope,
} from '@coze-studio/api-schema/workbench-model';
import { workbench } from '@coze-studio/api-schema';
import {
  DeveloperApi,
  KnowledgeApi,
  MemoryApi,
  workflowApi,
} from '@coze-arch/bot-api';

import {
  type AppendWorkbenchMessageRequest,
  type CreateWorkbenchRunRequest,
  type CreateWorkbenchThreadRequest,
  type WorkbenchMessage,
  type WorkbenchRun,
  type WorkbenchThreadCreation,
} from './thread-client';
import {
  type LegacyPageResponse,
  presentTaskThreadCreateResponse,
  presentTaskThreadMessageResponse,
  presentTaskThreadRunCreateResponse,
  presentTaskThreadUploadResponse,
} from './thread-client/legacy-page-response';
import {
  canonicalThreadClient,
  resolvePageServiceSpaceID,
} from './thread-client/canonical-thread-client-singleton';
import { type WorkbenchLLMModel } from './components/types';

interface PageScopedRequest {
  space_id?: string;
}

type OptionalPageSpace<Request extends { space_id: string }> = Omit<
  Request,
  'space_id'
> &
  PageScopedRequest;

type CreateTaskThreadRequest = OptionalPageSpace<CreateWorkbenchThreadRequest>;
type AppendTaskThreadMessageRequest = OptionalPageSpace<
  AppendWorkbenchMessageRequest
>;
type CreateTaskThreadRunRequest = OptionalPageSpace<CreateWorkbenchRunRequest>;
type CreateTaskThreadResponse = LegacyPageResponse<WorkbenchThreadCreation>;
type AppendTaskThreadMessageResponse = LegacyPageResponse<WorkbenchMessage>;
interface CreateTaskThreadRunResponse
  extends LegacyPageResponse<WorkbenchRun> {
  message?: WorkbenchMessage;
}

export interface WorkbenchReferenceResource {
  id: string;
  name: string;
  description?: string;
}

export const createTaskThread = async (
  request: CreateTaskThreadRequest,
): Promise<CreateTaskThreadResponse> =>
  presentTaskThreadCreateResponse(
    await canonicalThreadClient.createThread({
      ...request,
      space_id: resolvePageServiceSpaceID(request.space_id),
    }),
  );

export const appendTaskThreadMessage = async (
  request: AppendTaskThreadMessageRequest,
): Promise<AppendTaskThreadMessageResponse> =>
  presentTaskThreadMessageResponse(
    await canonicalThreadClient.appendMessage({
      ...request,
      space_id: resolvePageServiceSpaceID(request.space_id),
    }),
  );

export const createTaskThreadRun = async (
  request: CreateTaskThreadRunRequest,
): Promise<CreateTaskThreadRunResponse> =>
  presentTaskThreadRunCreateResponse(
    await canonicalThreadClient.createRun({
      ...request,
      space_id: resolvePageServiceSpaceID(request.space_id),
    }),
  );
export const getWorkbenchRuntimeDoctor = workbench.GetWorkbenchRuntimeDoctor;

export interface TaskThreadUploadedFile {
  file_id?: string;
  file_name: string;
  virtual_path: string;
  content_type?: string;
  size_bytes?: number;
  created_at?: number;
}

export interface UploadTaskThreadFilesResponse {
  data?: {
    files?: TaskThreadUploadedFile[];
    skipped_files?: string[];
  };
  code: number;
  msg: string;
}

export const uploadTaskThreadFiles = async ({
  files,
  space_id: spaceID,
  thread_id: threadId,
}: {
  thread_id: string;
  space_id?: string;
  files: File[];
}): Promise<UploadTaskThreadFilesResponse> => {
  if (!threadId || files.length === 0) {
    return presentTaskThreadUploadResponse({
      uploads: [],
      skipped_files: [],
    });
  }

  try {
    return presentTaskThreadUploadResponse(
      await canonicalThreadClient.uploadFiles({
        files,
        space_id: resolvePageServiceSpaceID(spaceID),
        thread_id: threadId,
      }),
    );
  } catch (cause) {
    const error = new Error('上传附件失败');
    (error as Error & { cause?: unknown }).cause = cause;
    throw error;
  }
};

export const getWorkbenchLLMModels = async (
  spaceId: string,
): Promise<WorkbenchLLMModel[]> => {
  if (!spaceId) {
    return [];
  }

  const [response, workspaceResponse] = await Promise.all([
    DeveloperApi.GetTypeList({
      space_id: spaceId,
      model: true,
      cur_model_ids: [],
    }),
    ListWorkspaceModels({
      space_id: spaceId,
      scope: WorkspaceModelScope.Space,
    }).catch(() => undefined),
  ]);
  const workspaceData =
    workspaceResponse?.code === 0 ? workspaceResponse.data : undefined;
  const workspaceModelsByIdentifier = new Map(
    (workspaceData?.workspace_models ?? []).map(model => [
      model.model_identifier.trim().toLowerCase(),
      model,
    ]),
  );

  return (response?.data?.model_list ?? []).map(model => {
    const identifier = (model.model_name || model.name || '')
      .trim()
      .toLowerCase();
    const workspaceModel = workspaceModelsByIdentifier.get(identifier);

    return {
      ...model,
      description:
        workspaceModel?.description || model.model_brief_desc || undefined,
      workspace_can_manage: workspaceData?.can_manage ?? false,
      workspace_model_id: workspaceModel?.id,
      workspace_model_can_manage: Boolean(
        workspaceData?.can_manage && workspaceModel?.can_manage,
      ),
      workspace_model_description: workspaceModel?.description,
    };
  });
};

export const listWorkbenchKnowledgeResources = async (
  spaceId?: string,
): Promise<WorkbenchReferenceResource[]> => {
  if (!spaceId) {
    return [];
  }

  const response = await KnowledgeApi.ListDataset({
    page: 1,
    size: 50,
    space_id: spaceId,
  });

  return (response.dataset_list ?? [])
    .filter(item => item.dataset_id && item.name)
    .map(item => ({
      id: String(item.dataset_id),
      name: item.name ?? '',
      description: item.description,
    }));
};

export const listWorkbenchDatabaseResources = async (
  spaceId?: string,
): Promise<WorkbenchReferenceResource[]> => {
  if (!spaceId) {
    return [];
  }

  const response = await MemoryApi.ListDatabase({
    limit: 50,
    offset: 0,
    space_id: spaceId,
    table_type: 2,
  });

  return (response.database_info_list ?? [])
    .filter(item => item.id && item.table_name)
    .map(item => ({
      id: String(item.id),
      name: item.table_name ?? '',
      description: item.table_desc,
    }));
};

export const listWorkbenchWorkflowResources = async (
  spaceId?: string,
): Promise<WorkbenchReferenceResource[]> => {
  if (!spaceId) {
    return [];
  }

  const response = await workflowApi.WorkflowListV2({
    page: 1,
    size: 50,
    space_id: spaceId,
    flow_mode: 100,
  });

  return (response.data?.workflow_list ?? [])
    .filter(item => item.workflow_id && item.name)
    .map(item => ({
      id: String(item.workflow_id),
      name: item.name ?? '',
      description: item.desc,
    }));
};
