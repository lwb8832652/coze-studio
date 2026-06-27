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

import type { workbenchTask } from '@coze-studio/api-schema';

type TaskThreadTokenUsageAggregate =
  workbenchTask.TaskThreadTokenUsageAggregate;

export interface TaskDetailTokenUsage {
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  costMicros: number;
  currency: string;
  callCount: number;
  leadAgentTokens: number;
  subagentTokens: number;
  middlewareTokens: number;
  toolTokens: number;
}

export const mapTaskThreadTokenUsageAggregate = (
  aggregate?: TaskThreadTokenUsageAggregate,
): TaskDetailTokenUsage | undefined => {
  if (!aggregate || aggregate.total_tokens <= 0) {
    return undefined;
  }

  return {
    inputTokens: aggregate.input_tokens,
    outputTokens: aggregate.output_tokens,
    totalTokens: aggregate.total_tokens,
    costMicros: aggregate.cost_micros,
    currency: '',
    callCount: aggregate.call_count,
    leadAgentTokens: aggregate.lead_agent_tokens,
    subagentTokens: aggregate.subagent_tokens,
    middlewareTokens: aggregate.middleware_tokens,
    toolTokens: aggregate.tool_tokens,
  };
};
