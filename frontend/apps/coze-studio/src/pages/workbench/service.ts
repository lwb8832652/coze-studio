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

import { workbench, workbenchTask } from '@coze-studio/api-schema';
import {
  DeveloperApi,
  KnowledgeApi,
  MemoryApi,
  workflowApi,
} from '@coze-arch/bot-api';

import { type WorkbenchLLMModel } from './components/types';

export interface WorkbenchReferenceResource {
  id: string;
  name: string;
  description?: string;
}

export const createTaskThread = workbenchTask.CreateTaskThread;
export const sendWorkbenchChat = workbench.WorkbenchChat;
export const getWorkbenchRuntimeDoctor = workbench.GetWorkbenchRuntimeDoctor;

export const getWorkbenchLLMModels = async (
  spaceId: string,
): Promise<WorkbenchLLMModel[]> => {
  if (!spaceId) {
    return [];
  }

  const response = await DeveloperApi.GetTypeList({
    space_id: spaceId,
    model: true,
    cur_model_ids: [],
  });

  return response?.data?.model_list ?? [];
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
