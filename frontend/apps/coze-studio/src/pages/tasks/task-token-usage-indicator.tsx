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

import { IconCozArrowDown } from '@coze-arch/coze-design/icons';
import { Popover } from '@coze-arch/coze-design';

import type {
  TaskDetailTokenUsage,
  TaskTokenUsageViewMode,
} from './task-detail-loader';

const TOKEN_COMPACT_THRESHOLD = 10_000;
const TOKEN_COMPACT_DIVISOR = 1000;

const formatTokenCount = (value: number) => {
  const safeValue = Math.max(0, value);

  return safeValue < TOKEN_COMPACT_THRESHOLD
    ? new Intl.NumberFormat('en-US').format(safeValue)
    : `${(safeValue / TOKEN_COMPACT_DIVISOR).toFixed(1)}K`;
};

const TOKEN_USAGE_VIEW_OPTIONS: Array<{
  description: string;
  label: string;
  value: TaskTokenUsageViewMode;
}> = [
  {
    value: 'off',
    label: '关闭',
    description: '隐藏顶部和会话内的 token 展示。',
  },
  {
    value: 'summary',
    label: '总览',
    description: '只在顶部显示当前对话累计 token。',
  },
  {
    value: 'per_turn',
    label: '每轮',
    description: '显示顶部累计，并为每轮 assistant 回复显示一条汇总 token。',
  },
  {
    value: 'debug',
    label: '调试',
    description: '显示顶部累计，并展示按步骤归类的 token 调试信息。',
  },
];

const tokenUsageModeLabel = (mode: TaskTokenUsageViewMode) =>
  TOKEN_USAGE_VIEW_OPTIONS.find(option => option.value === mode)?.label ??
  '每轮';

export const TaskTokenUsageIndicator = ({
  onViewModeChange,
  tokenUsage,
  viewMode,
}: {
  onViewModeChange?: (mode: TaskTokenUsageViewMode) => void;
  tokenUsage?: TaskDetailTokenUsage;
  viewMode: TaskTokenUsageViewMode;
}) => {
  if (!tokenUsage || tokenUsage.totalTokens <= 0) {
    return null;
  }

  const headerTotalVisible = viewMode !== 'off';
  const detailText = `输入 ${formatTokenCount(
    tokenUsage.inputTokens,
  )} · 输出 ${formatTokenCount(tokenUsage.outputTokens)} · 总计 ${formatTokenCount(
    tokenUsage.totalTokens,
  )}`;
  const usageRows = [
    {
      label: '输入',
      value: formatTokenCount(tokenUsage.inputTokens),
    },
    {
      label: '输出',
      value: formatTokenCount(tokenUsage.outputTokens),
    },
    {
      label: '总计',
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
      <div className="coze-prototype-token-usage-popover-divider" />
      <div className="coze-prototype-token-usage-popover-title">显示方式</div>
      <div
        className="coze-prototype-token-usage-mode-list"
        role="radiogroup"
        aria-label="Token 用量显示方式"
      >
        {TOKEN_USAGE_VIEW_OPTIONS.map(option => (
          <button
            key={option.value}
            type="button"
            role="radio"
            aria-checked={viewMode === option.value}
            className="coze-prototype-token-usage-mode-item"
            data-active={viewMode === option.value}
            onClick={() => onViewModeChange?.(option.value)}
          >
            <span className="coze-prototype-token-usage-mode-dot" />
            <span className="coze-prototype-token-usage-mode-copy">
              <span className="coze-prototype-token-usage-mode-label">
                {option.label}
              </span>
              <span className="coze-prototype-token-usage-mode-description">
                {option.description}
              </span>
            </span>
          </button>
        ))}
      </div>
      <div className="coze-prototype-token-usage-popover-divider" />
      <p className="coze-prototype-token-usage-note">
        顶部总量优先使用后端持久化的线程用量；当当前回复仍在流式返回时，还会叠加可见的进行中用量。每轮和调试用量只来自当前可见消息，可能与平台账单页不完全一致。
      </p>
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
        <span
          className="coze-prototype-token-usage-symbol"
          aria-hidden="true"
        />
        <span>Tokens</span>
        <span className="coze-prototype-token-usage-count">
          {headerTotalVisible
            ? formatTokenCount(tokenUsage.totalTokens)
            : tokenUsageModeLabel(viewMode)}
        </span>
        <IconCozArrowDown className="text-[12px]" />
      </button>
    </Popover>
  );
};

export { formatTokenCount as formatTaskTokenCount };
