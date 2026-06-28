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

import { Popover } from '@coze-arch/coze-design';

import type { TaskDetailTokenUsage } from './task-detail-loader';

const TOKEN_COMPACT_THRESHOLD = 10_000;
const TOKEN_COMPACT_DIVISOR = 1000;

const formatTokenCount = (value: number) => {
  const safeValue = Math.max(0, value);

  return safeValue < TOKEN_COMPACT_THRESHOLD
    ? new Intl.NumberFormat('en-US').format(safeValue)
    : `${(safeValue / TOKEN_COMPACT_DIVISOR).toFixed(1)}K`;
};

export const TaskTokenUsageIndicator = ({
  tokenUsage,
}: {
  tokenUsage?: TaskDetailTokenUsage;
}) => {
  if (!tokenUsage || tokenUsage.totalTokens <= 0) {
    return null;
  }

  const detailText = `Input ${formatTokenCount(
    tokenUsage.inputTokens,
  )} · Output ${formatTokenCount(tokenUsage.outputTokens)} · Total ${formatTokenCount(
    tokenUsage.totalTokens,
  )}`;
  const usageRows = [
    {
      label: 'Input',
      value: formatTokenCount(tokenUsage.inputTokens),
    },
    {
      label: 'Output',
      value: formatTokenCount(tokenUsage.outputTokens),
    },
    {
      label: 'Total',
      value: formatTokenCount(tokenUsage.totalTokens),
    },
  ];
  const content = (
    <div
      className="coze-prototype-token-usage-popover"
      data-testid="task-token-usage-popover"
    >
      <div className="coze-prototype-token-usage-popover-title">Token 用量</div>
      <dl className="coze-prototype-token-usage-popover-list">
        {usageRows.map(row => (
          <div
            key={row.label}
            className="coze-prototype-token-usage-popover-row"
          >
            <dt>{row.label}</dt>
            <dd>{row.value}</dd>
          </div>
        ))}
      </dl>
    </div>
  );

  return (
    <Popover content={content} position="bottomRight" showArrow trigger="click">
      <button
        type="button"
        className="coze-prototype-token-usage"
        aria-label={`查看 Token 用量，总计 ${formatTokenCount(
          tokenUsage.totalTokens,
        )}`}
        title={detailText}
      >
        <span>Tokens</span>
        <span className="coze-prototype-token-usage-count">
          {formatTokenCount(tokenUsage.totalTokens)}
        </span>
      </button>
    </Popover>
  );
};
