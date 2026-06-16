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

import type { TaskDetailTokenUsage } from './task-detail-loader';

const formatTokenCount = (value: number) =>
  new Intl.NumberFormat('en-US').format(Math.max(0, value));

export const TaskTokenUsageIndicator = ({
  tokenUsage,
}: {
  tokenUsage?: TaskDetailTokenUsage;
}) => {
  if (!tokenUsage || tokenUsage.totalTokens <= 0) {
    return null;
  }

  const sourceItems = [
    tokenUsage.leadAgentTokens > 0
      ? `Agent ${formatTokenCount(tokenUsage.leadAgentTokens)}`
      : '',
    tokenUsage.subagentTokens > 0
      ? `Subagent ${formatTokenCount(tokenUsage.subagentTokens)}`
      : '',
    tokenUsage.middlewareTokens > 0
      ? `Middleware ${formatTokenCount(tokenUsage.middlewareTokens)}`
      : '',
    tokenUsage.toolTokens > 0
      ? `Tool ${formatTokenCount(tokenUsage.toolTokens)}`
      : '',
  ].filter(Boolean);

  return (
    <span
      className="coze-prototype-token-usage"
      aria-label={`Token ${formatTokenCount(tokenUsage.totalTokens)}`}
    >
      <span>Token {formatTokenCount(tokenUsage.totalTokens)}</span>
      <span>
        In {formatTokenCount(tokenUsage.inputTokens)} / Out{' '}
        {formatTokenCount(tokenUsage.outputTokens)}
      </span>
      {sourceItems.map(item => (
        <span key={item}>{item}</span>
      ))}
    </span>
  );
};
