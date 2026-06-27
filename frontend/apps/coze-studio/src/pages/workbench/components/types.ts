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

import { workbench } from '@coze-studio/api-schema';

export const WORKBENCH_MODES = ['Auto', 'Ask', 'Agent'] as const;

export type WorkbenchMode = (typeof WORKBENCH_MODES)[number];

export type WorkbenchComposerVariant = 'home' | 'detail';

export interface WorkbenchResourceSelection {
  enable_skills: string[];
  enable_mcp: string[];
  enable_kbs: string[];
  enable_databases: string[];
}

export type WorkbenchMemoryScope = 'thread' | 'run' | 'long_term';

export interface WorkbenchRuntimeSettings {
  runtime: 'eino_adk';
  memory_retrieval: {
    limit: number;
    candidate_limit: number;
    scopes: WorkbenchMemoryScope[];
    min_confidence: number;
  };
  mcp_tools: {
    enabled: boolean;
    visibility: 'deferred';
    allowed_tools: string[];
  };
  web_tools: {
    enabled: boolean;
    visibility: 'deferred';
    http: {
      enabled: boolean;
      allowed_hosts: string[];
      timeout_ms: number;
      max_response_bytes: number;
    };
    search: {
      enabled: boolean;
      max_results: number;
    };
  };
  token_usage: {
    enabled: boolean;
  };
}

export interface WorkbenchLLMModel {
  name?: string;
  model_type?: number | string;
  model_class_name?: string;
  endpoint_name?: string;
}

export interface WorkbenchComposerSubmitPayload
  extends WorkbenchResourceSelection {
  message: string;
  mode: WorkbenchMode;
  taskId?: string;
  modelType?: number;
  modelName?: string;
  runtimeSettings: WorkbenchRuntimeSettings;
}

export const WORKBENCH_MODE_PROMPTS: Record<WorkbenchMode, string> = {
  Auto: 'Hi,我会根据你的任务特性,自动匹配最佳的处理方式~',
  Ask: 'Hi,我会以最快的方式自动响应,为你提供高效且清晰的专业答案~',
  Agent: 'Hi,我会充分思考并灵活使用多种工具,帮你搞定复杂问题~',
};

export const WORKBENCH_MODE_SYMBOLS: Record<WorkbenchMode, string> = {
  Auto: '✦',
  Ask: '?',
  Agent: 'A',
};

export const createDefaultWorkbenchResourceSelection =
  (): WorkbenchResourceSelection => ({
    enable_skills: [],
    enable_mcp: [],
    enable_kbs: [],
    enable_databases: [],
  });

export const createDefaultWorkbenchRuntimeSettings = (
  resourceSelection: WorkbenchResourceSelection = createDefaultWorkbenchResourceSelection(),
): WorkbenchRuntimeSettings => ({
  runtime: 'eino_adk',
  memory_retrieval: {
    limit: 5,
    candidate_limit: 20,
    scopes: ['thread', 'long_term'],
    min_confidence: 0.2,
  },
  mcp_tools: {
    enabled: resourceSelection.enable_mcp.length > 0,
    visibility: 'deferred',
    allowed_tools: [...resourceSelection.enable_mcp],
  },
  web_tools: {
    enabled: false,
    visibility: 'deferred',
    http: {
      enabled: false,
      allowed_hosts: [],
      timeout_ms: 10000,
      max_response_bytes: 262144,
    },
    search: {
      enabled: false,
      max_results: 5,
    },
  },
  token_usage: {
    enabled: true,
  },
});

export const cloneWorkbenchRuntimeSettings = (
  settings: WorkbenchRuntimeSettings,
): WorkbenchRuntimeSettings => ({
  runtime: settings.runtime,
  memory_retrieval: {
    ...settings.memory_retrieval,
    scopes: [...settings.memory_retrieval.scopes],
  },
  mcp_tools: {
    ...settings.mcp_tools,
    allowed_tools: [...settings.mcp_tools.allowed_tools],
  },
  web_tools: {
    ...settings.web_tools,
    http: {
      ...settings.web_tools.http,
      allowed_hosts: [...settings.web_tools.http.allowed_hosts],
    },
    search: {
      ...settings.web_tools.search,
    },
  },
  token_usage: {
    ...settings.token_usage,
  },
});

export const createWorkbenchRunConfig = (
  payload: WorkbenchComposerSubmitPayload,
) => ({
  runtime: payload.runtimeSettings.runtime,
  mode: payload.mode,
  model_type: payload.modelType,
  model_name: payload.modelName,
  enable_skills: payload.enable_skills,
  enable_mcp: payload.enable_mcp,
  enable_kbs: payload.enable_kbs,
  enable_databases: payload.enable_databases,
  memory_retrieval: payload.runtimeSettings.memory_retrieval,
  mcp_tools: payload.runtimeSettings.mcp_tools,
  web_tools: payload.runtimeSettings.web_tools,
  token_usage: payload.runtimeSettings.token_usage,
});

export const stringifyWorkbenchRunConfig = (
  payload: WorkbenchComposerSubmitPayload,
) => JSON.stringify(createWorkbenchRunConfig(payload));

export const mapModeToChatMode = (mode: WorkbenchMode): workbench.ChatMode => {
  const modeMap: Record<WorkbenchMode, workbench.ChatMode> = {
    Auto: workbench.ChatMode.Auto,
    Ask: workbench.ChatMode.Ask,
    Agent: workbench.ChatMode.Agent,
  };

  return modeMap[mode];
};
