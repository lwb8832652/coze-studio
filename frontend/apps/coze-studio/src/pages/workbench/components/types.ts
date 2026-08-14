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

/* eslint-disable max-lines -- Shared Workbench composer contracts remain one reviewed boundary. */

import type {
  CanonicalHumanInteractionResponseV2,
  CanonicalInitialRunSubmissionV2,
  CanonicalRunConfigV2,
  CanonicalRunSubmissionV2,
} from '@coze-studio/api-schema/workbench-thread';

import type { HumanInteractionResponse } from '../thread-client/types';

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
  description?: string;
  model?: string;
  name?: string;
  model_name?: string;
  model_type?: number | string;
  model_class_name?: string;
  endpoint_name?: string;
  model_brief_desc?: string;
  workspace_can_manage?: boolean;
  workspace_model_id?: string;
  workspace_model_can_manage?: boolean;
  workspace_model_description?: string;
}

export interface WorkbenchUploadedFile {
  file_id?: string;
  file_name: string;
  virtual_path: string;
  content_type?: string;
  size_bytes?: number;
  created_at?: number;
}

export interface WorkbenchComposerSubmitPayload
  extends Omit<
    WorkbenchResourceSelection,
    'enable_skills' | 'explicit_enable_skills'
  > {
  enable_skills?: string[];
  message: string;
  taskId?: string;
  modelType?: number;
  modelName?: string;
  runtimeSettings: WorkbenchRuntimeSettings;
  files?: File[];
}

export interface CreateWorkbenchSubmitPayloadInput {
  message: string;
  taskId?: string;
  selectedModel?: WorkbenchLLMModel;
  models: WorkbenchLLMModel[];
  resourceSelection: WorkbenchResourceSelection;
  runtimeSettings: WorkbenchRuntimeSettings;
  files?: File[];
}

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
  taskId,
  selectedModel,
  models,
  resourceSelection,
  runtimeSettings,
  files,
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
    taskId,
    modelType,
    modelName: selectedModel?.model_name || selectedModel?.name,
    runtimeSettings: nextRuntimeSettings,
    files,
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
    model_type: payload.modelType,
    model_name: payload.modelName,
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

const canonicalModelID = (value: number | undefined): string | undefined => {
  if (value === undefined) {
    return undefined;
  }
  if (!Number.isSafeInteger(value) || value <= 0) {
    throw new Error('模型 ID 无效');
  }
  return String(value);
};

const createCanonicalRunConfigV2 = (
  payload: WorkbenchComposerSubmitPayload,
): CanonicalRunConfigV2 => {
  const { runtimeSettings } = payload;
  const failoverCandidateIDs =
    runtimeSettings.model_failover.candidate_model_ids;

  return {
    runtime: runtimeSettings.runtime,
    memory_retrieval: {
      ...runtimeSettings.memory_retrieval,
      scopes: [...runtimeSettings.memory_retrieval.scopes],
    },
    skills: {
      enabled: runtimeSettings.skills.enabled,
      visibility: runtimeSettings.skills.visibility,
    },
    mcp_tools: {
      enabled: runtimeSettings.mcp_tools.enabled,
      visibility: runtimeSettings.mcp_tools.visibility,
    },
    web_tools: {
      enabled: runtimeSettings.web_tools.enabled,
      visibility: runtimeSettings.web_tools.visibility,
      http: {
        ...runtimeSettings.web_tools.http,
        allowed_hosts: [...runtimeSettings.web_tools.http.allowed_hosts],
      },
      search: { ...runtimeSettings.web_tools.search },
    },
    ...(runtimeSettings.model_retry.enabled
      ? {
          model_retry: {
            max_retries: runtimeSettings.model_retry.max_retries,
            backoff_ms: runtimeSettings.model_retry.backoff_ms,
            retry_empty_output: runtimeSettings.model_retry.retry_empty_output,
            retry_finish_reasons: [
              ...runtimeSettings.model_retry.retry_finish_reasons,
            ],
          },
        }
      : {}),
    ...(runtimeSettings.model_failover.enabled &&
    failoverCandidateIDs.length > 0
      ? {
          model_failover: {
            candidate_model_ids: failoverCandidateIDs.map(id => {
              const candidateID = canonicalModelID(id);
              if (candidateID === undefined) {
                throw new Error('模型 ID 无效');
              }
              return candidateID;
            }),
            max_retries: Math.min(
              runtimeSettings.model_failover.max_retries,
              failoverCandidateIDs.length,
            ),
            failover_empty_output:
              runtimeSettings.model_failover.failover_empty_output,
            failover_finish_reasons: [
              ...runtimeSettings.model_failover.failover_finish_reasons,
            ],
          },
        }
      : {}),
    token_usage: { ...runtimeSettings.token_usage },
  };
};

const createCanonicalSubmissionBaseV2 = (
  payload: WorkbenchComposerSubmitPayload,
) => {
  const modelType = canonicalModelID(payload.modelType);

  return {
    input: { message: payload.message, uploaded_files: [] },
    composer: {
      ...(modelType === undefined ? {} : { model_type: modelType }),
      ...(payload.modelName ? { model_name: payload.modelName } : {}),
      ...(payload.enable_skills === undefined
        ? {}
        : { explicit_enable_skills: [...payload.enable_skills] }),
      allowed_skills: [...payload.runtimeSettings.skills.allowed_skills],
      enable_mcp: [...payload.enable_mcp],
      enable_kbs: [...payload.enable_kbs],
      enable_databases: [...payload.enable_databases],
      allowed_mcp_tools: [
        ...(payload.runtimeSettings.mcp_tools.allowed_tools ?? []),
      ],
    },
    config: createCanonicalRunConfigV2(payload),
  };
};

export const createInitialSubmissionV2 = (
  payload: WorkbenchComposerSubmitPayload,
): CanonicalInitialRunSubmissionV2 => ({
  schema_version: 'coze.workbench.initial_run_submission.v2',
  ...createCanonicalSubmissionBaseV2(payload),
});

export const createTurnSubmissionV2 = (
  payload: WorkbenchComposerSubmitPayload,
  uploadedFileIDs: string[],
  source: 'workbench_new_task' | 'workbench_detail_followup',
): CanonicalRunSubmissionV2 => ({
  schema_version: 'coze.workbench.run_submission.v2',
  kind: 'turn',
  ...createCanonicalSubmissionBaseV2(payload),
  input: {
    message: payload.message,
    uploaded_files: uploadedFileIDs.map(fileID => ({ file_id: fileID })),
  },
  metadata: { source },
});

export const requireUploadedFileIDs = (
  files: Array<{ file_id?: string }>,
): string[] =>
  files.map(file => {
    const fileID = file.file_id?.trim();
    if (!fileID) {
      throw new Error('上传文件缺少 ID');
    }
    return fileID;
  });

export const createRetrySubmissionV2 = (
  payload: WorkbenchComposerSubmitPayload,
  sourceRunID: string,
): CanonicalRunSubmissionV2 => ({
  schema_version: 'coze.workbench.run_submission.v2',
  kind: 'retry',
  ...createCanonicalSubmissionBaseV2(payload),
  lineage: { source_run_id: sourceRunID },
  metadata: { source: 'task_retry' },
});

export const normalizeHumanResponseV2 = (
  response: HumanInteractionResponse,
): CanonicalHumanInteractionResponseV2 =>
  response.kind === 'clarification'
    ? {
        schema: response.schema,
        interaction_id: response.interaction_id,
        kind: response.kind,
        decision: response.decision,
        ...(response.answer ? { answer: response.answer } : {}),
        ...(response.choice_id ? { choice_id: response.choice_id } : {}),
      }
    : {
        schema: response.schema,
        interaction_id: response.interaction_id,
        kind: response.kind,
        decision: response.decision,
        ...(response.comment ? { comment: response.comment } : {}),
      };
