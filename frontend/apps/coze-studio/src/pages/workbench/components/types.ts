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
export type WorkbenchReasoningEffort = 'medium' | 'high';

export interface WorkbenchRuntimeSettings {
  runtime: 'eino_adk';
  reasoning: {
    enabled: boolean;
    effort: WorkbenchReasoningEffort;
  };
  memory_retrieval: {
    limit: number;
    candidate_limit: number;
    scopes: WorkbenchMemoryScope[];
    min_confidence: number;
  };
  skills: {
    enabled: boolean;
    visibility: 'deferred';
    allowed_skills: string[];
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
  model_retry: {
    enabled: boolean;
    max_retries: number;
    backoff_ms: number;
    retry_empty_output: boolean;
    retry_finish_reasons: string[];
  };
  model_failover: {
    enabled: boolean;
    candidate_model_ids: number[];
    max_retries: number;
    failover_empty_output: boolean;
    failover_finish_reasons: string[];
  };
  token_usage: {
    enabled: boolean;
  };
}

export interface WorkbenchLLMModel {
  name?: string;
  model_name?: string;
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

export interface CreateWorkbenchSubmitPayloadInput {
  message: string;
  mode: WorkbenchMode;
  taskId?: string;
  selectedModel?: WorkbenchLLMModel;
  models: WorkbenchLLMModel[];
  resourceSelection: WorkbenchResourceSelection;
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

export const workbenchModelTypeToNumber = (model: WorkbenchLLMModel) =>
  Number(model.model_type);

export const getWorkbenchFailoverCandidateModelIds = (
  models: WorkbenchLLMModel[],
  selectedModelType?: number,
) => {
  const seen = new Set<number>();

  return models.map(workbenchModelTypeToNumber).filter(modelType => {
    if (
      !Number.isFinite(modelType) ||
      modelType <= 0 ||
      modelType === selectedModelType ||
      seen.has(modelType)
    ) {
      return false;
    }
    seen.add(modelType);
    return true;
  });
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
  reasoning: {
    enabled: false,
    effort: 'medium',
  },
  memory_retrieval: {
    limit: 5,
    candidate_limit: 20,
    scopes: ['thread', 'long_term'],
    min_confidence: 0.2,
  },
  skills: {
    enabled: resourceSelection.enable_skills.length > 0,
    visibility: 'deferred',
    allowed_skills: [...resourceSelection.enable_skills],
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
  model_retry: {
    enabled: false,
    max_retries: 1,
    backoff_ms: 0,
    retry_empty_output: true,
    retry_finish_reasons: ['length'],
  },
  model_failover: {
    enabled: false,
    candidate_model_ids: [],
    max_retries: 1,
    failover_empty_output: true,
    failover_finish_reasons: ['length'],
  },
  token_usage: {
    enabled: true,
  },
});

export const cloneWorkbenchRuntimeSettings = (
  settings: WorkbenchRuntimeSettings,
): WorkbenchRuntimeSettings => ({
  runtime: settings.runtime,
  reasoning: {
    ...settings.reasoning,
  },
  memory_retrieval: {
    ...settings.memory_retrieval,
    scopes: [...settings.memory_retrieval.scopes],
  },
  skills: {
    ...settings.skills,
    allowed_skills: [...settings.skills.allowed_skills],
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
  model_retry: {
    ...settings.model_retry,
    retry_finish_reasons: [...settings.model_retry.retry_finish_reasons],
  },
  model_failover: {
    ...settings.model_failover,
    candidate_model_ids: [...settings.model_failover.candidate_model_ids],
    failover_finish_reasons: [
      ...settings.model_failover.failover_finish_reasons,
    ],
  },
  token_usage: {
    ...settings.token_usage,
  },
});

export const createWorkbenchSubmitPayload = ({
  message,
  mode,
  taskId,
  selectedModel,
  models,
  resourceSelection,
  runtimeSettings,
}: CreateWorkbenchSubmitPayloadInput): WorkbenchComposerSubmitPayload => {
  const modelType = selectedModel
    ? workbenchModelTypeToNumber(selectedModel)
    : undefined;
  const nextRuntimeSettings = cloneWorkbenchRuntimeSettings(runtimeSettings);
  nextRuntimeSettings.skills.allowed_skills = [
    ...resourceSelection.enable_skills,
  ];
  nextRuntimeSettings.mcp_tools.allowed_tools = [
    ...resourceSelection.enable_mcp,
  ];

  if (nextRuntimeSettings.model_failover.enabled) {
    const candidateModelIds = getWorkbenchFailoverCandidateModelIds(
      models,
      modelType,
    );
    nextRuntimeSettings.model_failover.candidate_model_ids = candidateModelIds;
    nextRuntimeSettings.model_failover.max_retries = Math.max(
      1,
      Math.min(
        nextRuntimeSettings.model_failover.max_retries,
        candidateModelIds.length,
      ),
    );
  }

  return {
    message,
    mode,
    taskId,
    modelType,
    modelName: selectedModel?.name,
    runtimeSettings: nextRuntimeSettings,
    enable_skills: nextRuntimeSettings.skills.enabled
      ? [...resourceSelection.enable_skills]
      : [],
    enable_mcp: nextRuntimeSettings.mcp_tools.enabled
      ? [...resourceSelection.enable_mcp]
      : [],
    enable_kbs: [...resourceSelection.enable_kbs],
    enable_databases: [...resourceSelection.enable_databases],
  };
};

export const createWorkbenchRunConfig = (
  payload: WorkbenchComposerSubmitPayload,
) => {
  const { runtimeSettings } = payload;
  const modelRetry = runtimeSettings.model_retry.enabled
    ? {
        max_retries: runtimeSettings.model_retry.max_retries,
        backoff_ms: runtimeSettings.model_retry.backoff_ms,
        retry_empty_output: runtimeSettings.model_retry.retry_empty_output,
        retry_finish_reasons: [
          ...runtimeSettings.model_retry.retry_finish_reasons,
        ],
      }
    : undefined;
  const failoverCandidateModelIds =
    runtimeSettings.model_failover.candidate_model_ids;
  const modelFailover =
    runtimeSettings.model_failover.enabled &&
    failoverCandidateModelIds.length > 0
      ? {
          candidate_model_ids: [...failoverCandidateModelIds],
          max_retries: Math.min(
            runtimeSettings.model_failover.max_retries,
            failoverCandidateModelIds.length,
          ),
          failover_empty_output:
            runtimeSettings.model_failover.failover_empty_output,
          failover_finish_reasons: [
            ...runtimeSettings.model_failover.failover_finish_reasons,
          ],
        }
      : undefined;

  return {
    runtime: runtimeSettings.runtime,
    mode: payload.mode,
    model_type: payload.modelType,
    model_name: payload.modelName,
    reasoning_effort: runtimeSettings.reasoning.enabled
      ? runtimeSettings.reasoning.effort
      : undefined,
    enable_skills: payload.enable_skills,
    enable_mcp: payload.enable_mcp,
    enable_kbs: payload.enable_kbs,
    enable_databases: payload.enable_databases,
    memory_retrieval: runtimeSettings.memory_retrieval,
    skills: runtimeSettings.skills,
    mcp_tools: runtimeSettings.mcp_tools,
    web_tools: runtimeSettings.web_tools,
    model_retry: modelRetry,
    model_failover: modelFailover,
    token_usage: runtimeSettings.token_usage,
  };
};

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
