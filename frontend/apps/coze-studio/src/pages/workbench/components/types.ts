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

export const WORKBENCH_PRESET_SKILL_IDS = [
  'meego-guidelines',
  'aeolus-platform-analysis',
  'coral-hive-metric-explorer',
  'deepwiki',
  'code-review',
  'aime-toolkit',
  'lark-wiki',
  'lark-shared',
  'lark-drive',
] as const;

export const createDefaultWorkbenchResourceSelection =
  (): WorkbenchResourceSelection => ({
    enable_skills: [...WORKBENCH_PRESET_SKILL_IDS],
    enable_mcp: [],
    enable_kbs: [],
    enable_databases: [],
  });

export const mapModeToChatMode = (mode: WorkbenchMode): workbench.ChatMode => {
  const modeMap: Record<WorkbenchMode, workbench.ChatMode> = {
    Auto: workbench.ChatMode.Auto,
    Ask: workbench.ChatMode.Ask,
    Agent: workbench.ChatMode.Agent,
  };

  return modeMap[mode];
};
