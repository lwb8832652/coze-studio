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
type TaskThreadTokenUsage = workbenchTask.TaskThreadTokenUsage;

export type TaskTokenUsageViewMode = 'off' | 'summary' | 'per_turn' | 'debug';

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

const emptyTaskDetailTokenUsage = (): TaskDetailTokenUsage => ({
  inputTokens: 0,
  outputTokens: 0,
  totalTokens: 0,
  costMicros: 0,
  currency: '',
  callCount: 0,
  leadAgentTokens: 0,
  subagentTokens: 0,
  middlewareTokens: 0,
  toolTokens: 0,
});

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

export const mapTaskThreadTokenUsageRowsByRunID = (
  rows?: TaskThreadTokenUsage[],
): Record<string, TaskDetailTokenUsage> => {
  const usageByRunID: Record<string, TaskDetailTokenUsage> = {};

  for (const row of rows ?? []) {
    const runID = row.run_id?.trim();

    if (!runID || row.total_tokens <= 0) {
      continue;
    }

    const aggregate = usageByRunID[runID] ?? emptyTaskDetailTokenUsage();
    aggregate.inputTokens += row.input_tokens;
    aggregate.outputTokens += row.output_tokens;
    aggregate.totalTokens += row.total_tokens;
    aggregate.costMicros += row.cost_micros;
    aggregate.callCount += 1;

    switch (row.source) {
      case 'lead_agent':
        aggregate.leadAgentTokens += row.total_tokens;
        break;
      case 'subagent':
        aggregate.subagentTokens += row.total_tokens;
        break;
      case 'middleware':
        aggregate.middlewareTokens += row.total_tokens;
        break;
      case 'tool':
        aggregate.toolTokens += row.total_tokens;
        break;
      default:
        break;
    }

    usageByRunID[runID] = aggregate;
  }

  return usageByRunID;
};
