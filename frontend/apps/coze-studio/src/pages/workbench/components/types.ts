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

export const WORKBENCH_MODES = ['flash', 'thinking', 'pro', 'ultra'] as const;

export type WorkbenchMode = (typeof WORKBENCH_MODES)[number];
export type WorkbenchLegacyMode = 'Auto' | 'Ask' | 'Agent';

export type WorkbenchComposerVariant = 'home' | 'detail';

export interface WorkbenchResourceSelection {
  enable_skills: string[];
  explicit_enable_skills: string[];
  enable_mcp: string[];
  enable_kbs: string[];
  enable_databases: string[];
}

export type WorkbenchMemoryScope = 'thread' | 'run' | 'long_term';
export type WorkbenchReasoningEffort = 'minimal' | 'low' | 'medium' | 'high';

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
  display_name?: string;
  model?: string;
  name?: string;
  model_name?: string;
  model_type?: number | string;
  model_class_name?: string;
  endpoint_name?: string;
}

export interface WorkbenchComposerSubmitPayload
  extends Omit<
    WorkbenchResourceSelection,
    'enable_skills' | 'explicit_enable_skills'
  > {
  enable_skills?: string[];
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

export const DEFAULT_WORKBENCH_MODE: WorkbenchMode = 'pro';

export const WORKBENCH_MODE_PROMPTS: Record<WorkbenchMode, string> = {
  flash: 'Hi,我会快速完成任务,尽量给你直接结果~',
  thinking: 'Hi,我会先思考再行动,在速度和准确性之间取得平衡~',
  pro: 'Hi,我会先计划再执行,帮你获得更精准的结果~',
  ultra: 'Hi,我会用更强的多步骤处理方式,帮你完成复杂任务~',
};

export const WORKBENCH_MODE_SYMBOLS: Record<WorkbenchMode, string> = {
  flash: '↯',
  thinking: '?',
  pro: 'P',
  ultra: 'U',
};

export const WORKBENCH_MODE_LABELS: Record<WorkbenchMode, string> = {
  flash: '闪速',
  thinking: '思考',
  pro: 'Pro',
  ultra: 'Ultra',
};

export const WORKBENCH_MODE_DESCRIPTIONS: Record<WorkbenchMode, string> = {
  flash: '快速且高效的完成任务，但可能不够精准',
  thinking: '思考后再行动，在时间与准确性之间取得平衡',
  pro: '思考、计划再执行，获得更精准的结果，可能需要更多时间',
  ultra: '继承自 Pro 模式，可调用子代理分工协作，适合复杂多步骤任务',
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
    explicit_enable_skills: [],
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
    enabled: true,
    visibility: 'deferred',
    allowed_skills: [...resourceSelection.enable_skills],
  },
  mcp_tools: {
    enabled: true,
    visibility: 'deferred',
    allowed_tools: [...resourceSelection.enable_mcp],
  },
  web_tools: {
    enabled: true,
    visibility: 'deferred',
    http: {
      enabled: false,
      allowed_hosts: [],
      timeout_ms: 10000,
      max_response_bytes: 262144,
    },
    search: {
      enabled: true,
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

  const explicitSkills = [...resourceSelection.explicit_enable_skills];
  const enableSkills = nextRuntimeSettings.skills.enabled
    ? explicitSkills.length > 0
      ? explicitSkills
      : undefined
    : [];

  return {
    message,
    mode,
    taskId,
    modelType,
    modelName: selectedModel?.model_name || selectedModel?.name,
    runtimeSettings: nextRuntimeSettings,
    enable_skills: enableSkills,
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
  const modeRuntimeContext = getWorkbenchModeRuntimeContext(payload.mode);
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
  const mcpTools = {
    ...runtimeSettings.mcp_tools,
    allowed_tools:
      runtimeSettings.mcp_tools.enabled &&
      runtimeSettings.mcp_tools.allowed_tools.length === 0
        ? undefined
        : runtimeSettings.mcp_tools.allowed_tools,
  };

  return {
    runtime: runtimeSettings.runtime,
    mode: payload.mode,
    model_type: payload.modelType,
    model_name: payload.modelName,
    thinking_enabled: modeRuntimeContext.thinking_enabled,
    is_plan_mode: modeRuntimeContext.is_plan_mode,
    subagent_enabled: modeRuntimeContext.subagent_enabled,
    ...(runtimeSettings.reasoning.enabled
      ? {
          reasoning_effort: runtimeSettings.reasoning.effort,
        }
      : {}),
    enable_skills: payload.enable_skills,
    enable_mcp: payload.enable_mcp,
    enable_kbs: payload.enable_kbs,
    enable_databases: payload.enable_databases,
    memory_retrieval: runtimeSettings.memory_retrieval,
    skills: runtimeSettings.skills,
    mcp_tools: mcpTools,
    web_tools: runtimeSettings.web_tools,
    model_retry: modelRetry,
    model_failover: modelFailover,
    token_usage: runtimeSettings.token_usage,
  };
};

export const stringifyWorkbenchRunConfig = (
  payload: WorkbenchComposerSubmitPayload,
) => JSON.stringify(createWorkbenchRunConfig(payload));

export const getWorkbenchModeRuntimeContext = (mode: WorkbenchMode) => ({
  thinking_enabled: mode !== 'flash',
  is_plan_mode: mode === 'pro' || mode === 'ultra',
  subagent_enabled: mode === 'ultra',
});

export const mapModeToChatMode = (
  mode: WorkbenchMode | WorkbenchLegacyMode,
): workbench.ChatMode => {
  const modeMap: Record<
    WorkbenchMode | WorkbenchLegacyMode,
    workbench.ChatMode
  > = {
    flash: workbench.ChatMode.Auto,
    thinking: workbench.ChatMode.Ask,
    pro: workbench.ChatMode.Agent,
    ultra: workbench.ChatMode.Agent,
    Auto: workbench.ChatMode.Auto,
    Ask: workbench.ChatMode.Ask,
    Agent: workbench.ChatMode.Agent,
  };

  return modeMap[mode];
};
