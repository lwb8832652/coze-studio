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

/* eslint-disable @coze-arch/max-line-per-function -- Usage virtualization, filters and accessibility state form one drawer workflow. */

import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react';

import { useCustomMediaQuery } from '@coze-arch/responsive-kit';
import { Button, SideSheet, Spin } from '@coze-arch/coze-design';

import { formatTaskTokenCount } from './task-token-usage-indicator';
import type { TaskUsageDetailItem } from './task-message-token-usage';
import type {
  TaskDetailTokenUsage,
  TaskTokenUsageViewMode,
} from './task-detail-loader';

const DETAIL_BATCH_SIZE = 100;
const DETAIL_MAX_ROWS = 1000;
const MODEL_ATTRIBUTION_DISPLAY_LIMIT = 4;

const VIEW_MODE_OPTIONS: Array<{
  description: string;
  label: string;
  value: TaskTokenUsageViewMode;
}> = [
  {
    value: 'off',
    label: '按需查看',
    description: '隐藏数值和明细，仍可通过设置入口重新开启。',
  },
  {
    value: 'summary',
    label: '会话总览',
    description: '仅显示本次对话累计用量。',
  },
  {
    value: 'per_turn',
    label: '逐回复',
    description: '保留原“每轮”语义，在明细中按 Agent 回复查看。',
  },
  {
    value: 'debug',
    label: '审计信息',
    description: '显示已审核的模型与来源统计，不展示内部载荷。',
  },
];

const formatUsageTime = (createdAt?: number) => {
  if (!createdAt || !Number.isFinite(createdAt)) {
    return undefined;
  }

  const date = new Date(createdAt);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }

  return {
    dateTime: date.toISOString(),
    label: new Intl.DateTimeFormat('zh-CN', {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(date),
  };
};

const getUsageSourceRows = (usage: TaskDetailTokenUsage) =>
  [
    { label: 'Lead Agent', value: usage.leadAgentTokens },
    { label: 'Subagent', value: usage.subagentTokens },
    { label: 'Middleware', value: usage.middlewareTokens },
    { label: 'Tool', value: usage.toolTokens },
  ].filter(row => row.value > 0);

const TaskUsageSummary = ({
  tokenUsage,
}: {
  tokenUsage: TaskDetailTokenUsage;
}) => (
  <section
    aria-label="会话总量"
    className="coze-prototype-usage-drawer-summary"
  >
    <div>
      <span>会话总量</span>
      <strong>{formatTaskTokenCount(tokenUsage.totalTokens)}</strong>
    </div>
    <dl>
      <div>
        <dt>输入</dt>
        <dd>{formatTaskTokenCount(tokenUsage.inputTokens)}</dd>
      </div>
      <div>
        <dt>输出</dt>
        <dd>{formatTaskTokenCount(tokenUsage.outputTokens)}</dd>
      </div>
    </dl>
  </section>
);

const TaskUsageRunItem = ({
  isPartial,
  item,
  onLocateReply,
  viewMode,
}: {
  isPartial: boolean;
  item: TaskUsageDetailItem;
  onLocateReply?: (runID: string) => void;
  viewMode: TaskTokenUsageViewMode;
}) => {
  const usageTime = formatUsageTime(item.createdAt);
  const sourceRows = getUsageSourceRows(item.usage);
  const modelAttributions = item.usage.modelAttributions.slice(
    0,
    MODEL_ATTRIBUTION_DISPLAY_LIMIT,
  );
  const showAuditMetadata = viewMode === 'debug';

  return (
    <li className="coze-prototype-usage-run-item">
      <div className="coze-prototype-usage-run-heading">
        <div>
          <strong>Agent 回复</strong>
        </div>
        <div className="coze-prototype-usage-run-actions">
          {usageTime ? (
            <time dateTime={usageTime.dateTime}>{usageTime.label}</time>
          ) : (
            <span>时间暂不可用</span>
          )}
          {item.canLocate && !isPartial && onLocateReply ? (
            <Button size="small" onClick={() => onLocateReply(item.runID)}>
              定位回复
            </Button>
          ) : !item.canLocate ? (
            <span className="coze-prototype-usage-locate-unavailable">
              未匹配到回复
            </span>
          ) : null}
        </div>
      </div>
      <dl className="coze-prototype-usage-run-totals">
        <div>
          <dt>输入</dt>
          <dd>{formatTaskTokenCount(item.usage.inputTokens)}</dd>
        </div>
        <div>
          <dt>输出</dt>
          <dd>{formatTaskTokenCount(item.usage.outputTokens)}</dd>
        </div>
        <div>
          <dt>{isPartial ? '已加载' : '合计'}</dt>
          <dd>{formatTaskTokenCount(item.usage.totalTokens)}</dd>
        </div>
      </dl>
      {showAuditMetadata &&
      (item.usage.callCount > 0 ||
        sourceRows.length > 0 ||
        modelAttributions.length > 0) ? (
        <dl
          aria-label="可审计元数据"
          className="coze-prototype-usage-run-audit"
        >
          {item.usage.callCount > 0 ? (
            <div>
              <dt>调用次数</dt>
              <dd>{formatTaskTokenCount(item.usage.callCount)}</dd>
            </div>
          ) : null}
          {sourceRows.map(row => (
            <div key={row.label}>
              <dt>{row.label}</dt>
              <dd>{formatTaskTokenCount(row.value)}</dd>
            </div>
          ))}
          {modelAttributions.length > 0 ? (
            <div>
              <dt>模型</dt>
              <dd>{modelAttributions.join(', ')}</dd>
            </div>
          ) : null}
        </dl>
      ) : null}
    </li>
  );
};

export const TaskUsageDetailDrawer = ({
  detailItems,
  error,
  isPartial = false,
  loadedCount = 0,
  loading = false,
  onCancel,
  onLocateReply,
  onRetry,
  onViewModeChange,
  tokenUsage,
  totalCount = 0,
  viewMode,
  visible,
}: {
  detailItems: TaskUsageDetailItem[];
  error?: string;
  isPartial?: boolean;
  loadedCount?: number;
  loading?: boolean;
  onCancel: () => void;
  onLocateReply?: (runID: string) => void;
  onRetry?: () => void | Promise<void>;
  onViewModeChange?: (mode: TaskTokenUsageViewMode) => void;
  tokenUsage?: TaskDetailTokenUsage;
  totalCount?: number;
  viewMode: TaskTokenUsageViewMode;
  visible: boolean;
}) => {
  const isMobile = useCustomMediaQuery({ rangeMaxPx: '767px' });
  const modeRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const [visibleRows, setVisibleRows] = useState(DETAIL_BATCH_SIZE);
  const previousVisibilityRef = useRef(visible);
  const previousViewModeRef = useRef(viewMode);
  const isOpening = visible && !previousVisibilityRef.current;
  const viewModeChanged = previousViewModeRef.current !== viewMode;
  previousVisibilityRef.current = visible;
  previousViewModeRef.current = viewMode;
  const activeModeIndex = Math.max(
    0,
    VIEW_MODE_OPTIONS.findIndex(option => option.value === viewMode),
  );
  const showRunDetails = viewMode === 'per_turn' || viewMode === 'debug';
  const safeDetailItems = useMemo(() => {
    if (!visible) {
      return [];
    }

    const safeNumber = (value: number) =>
      Number.isFinite(value) ? Math.max(0, value) : 0;

    return detailItems
      .slice(0, DETAIL_MAX_ROWS)
      .map(item => ({
        canLocate: item.canLocate === true,
        createdAt:
          item.createdAt &&
          Number.isFinite(item.createdAt) &&
          item.createdAt > 0
            ? item.createdAt
            : undefined,
        runID: String(item.runID ?? '').trim(),
        usage: {
          inputTokens: safeNumber(item.usage.inputTokens),
          outputTokens: safeNumber(item.usage.outputTokens),
          totalTokens: safeNumber(item.usage.totalTokens),
          costMicros: safeNumber(item.usage.costMicros),
          currency: String(item.usage.currency ?? '')
            .trim()
            .slice(0, 16),
          callCount: safeNumber(item.usage.callCount),
          leadAgentTokens: safeNumber(item.usage.leadAgentTokens),
          subagentTokens: safeNumber(item.usage.subagentTokens),
          middlewareTokens: safeNumber(item.usage.middlewareTokens),
          toolTokens: safeNumber(item.usage.toolTokens),
          modelAttributions: item.usage.modelAttributions
            .filter(attribution => typeof attribution === 'string')
            .map(attribution => attribution.trim().slice(0, 80))
            .filter(Boolean)
            .slice(0, MODEL_ATTRIBUTION_DISPLAY_LIMIT),
        },
      }))
      .filter(item => item.runID && item.usage.totalTokens > 0)
      .sort((left, right) => (right.createdAt ?? 0) - (left.createdAt ?? 0));
  }, [detailItems, visible]);
  const renderedDetailItems = safeDetailItems.slice(
    0,
    isOpening || viewModeChanged ? DETAIL_BATCH_SIZE : visibleRows,
  );

  useLayoutEffect(() => {
    setVisibleRows(DETAIL_BATCH_SIZE);
  }, [viewMode, visible]);
  const selectMode = (index: number) => {
    const normalizedIndex =
      (index + VIEW_MODE_OPTIONS.length) % VIEW_MODE_OPTIONS.length;
    onViewModeChange?.(VIEW_MODE_OPTIONS[normalizedIndex].value);
    queueMicrotask(() => modeRefs.current[normalizedIndex]?.focus());
  };
  const handleModeKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) => {
    let nextIndex: number | undefined;
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        nextIndex = index + 1;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        nextIndex = index - 1;
        break;
      case 'Home':
        nextIndex = 0;
        break;
      case 'End':
        nextIndex = VIEW_MODE_OPTIONS.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    selectMode(nextIndex);
  };

  if (!visible) {
    return null;
  }

  return (
    <SideSheet
      bodyStyle={
        isMobile
          ? {
              maxHeight: 'calc(min(78dvh, 720px) - 56px)',
              paddingBottom: 'max(16px, env(safe-area-inset-bottom))',
            }
          : undefined
      }
      className="coze-prototype-usage-drawer"
      closeOnEsc
      height={isMobile ? 'min(78dvh, 720px)' : undefined}
      maskClosable
      placement={isMobile ? 'bottom' : 'right'}
      title={viewMode === 'off' ? '用量显示设置' : '用量明细'}
      visible={visible}
      width={isMobile ? '100%' : 520}
      onCancel={onCancel}
    >
      <div
        className="coze-prototype-usage-drawer-content"
        data-placement={isMobile ? 'bottom' : 'right'}
        data-testid="task-usage-detail-drawer"
      >
        {viewMode !== 'off' && tokenUsage ? (
          <TaskUsageSummary tokenUsage={tokenUsage} />
        ) : null}
        <section
          aria-label="用量显示偏好"
          className="coze-prototype-usage-preference"
        >
          <div className="coze-prototype-usage-section-title">显示偏好</div>
          <div role="radiogroup" aria-label="Token 用量显示偏好">
            {VIEW_MODE_OPTIONS.map((option, index) => (
              <button
                key={option.value}
                ref={element => {
                  modeRefs.current[index] = element;
                }}
                type="button"
                role="radio"
                aria-checked={viewMode === option.value}
                data-active={viewMode === option.value}
                tabIndex={index === activeModeIndex ? 0 : -1}
                onClick={() => selectMode(index)}
                onKeyDown={event => handleModeKeyDown(event, index)}
              >
                <span>{option.label}</span>
                <small>{option.description}</small>
              </button>
            ))}
          </div>
        </section>
        {showRunDetails ? (
          <section
            aria-label="每次 Agent 回复"
            className="coze-prototype-usage-run-section"
          >
            <div className="coze-prototype-usage-section-title">
              <span>每次 Agent 回复</span>
              {isPartial ? (
                <>
                  <span className="coze-prototype-usage-partial" role="status">
                    部分数据 · 已加载 {loadedCount} / {totalCount} 条明细
                  </span>
                  <span className="coze-prototype-usage-locate-unavailable">
                    明细不完整，暂不能定位
                  </span>
                </>
              ) : null}
            </div>
            {loading ? (
              <Spin spinning>
                <div className="coze-prototype-usage-state">
                  正在加载用量明细...
                </div>
              </Spin>
            ) : null}
            {error ? (
              <div className="coze-prototype-usage-state" role="alert">
                <span>{error}</span>
                {onRetry ? (
                  <Button size="small" onClick={() => void onRetry()}>
                    重试
                  </Button>
                ) : null}
              </div>
            ) : null}
            {safeDetailItems.length > 0 ? (
              <>
                <ol className="coze-prototype-usage-run-list">
                  {renderedDetailItems.map(item => (
                    <TaskUsageRunItem
                      key={item.runID}
                      isPartial={isPartial}
                      item={item}
                      viewMode={viewMode}
                      onLocateReply={onLocateReply}
                    />
                  ))}
                </ol>
                {renderedDetailItems.length < safeDetailItems.length ? (
                  <Button
                    className="coze-prototype-usage-show-more"
                    onClick={() =>
                      setVisibleRows(current =>
                        Math.min(
                          current + DETAIL_BATCH_SIZE,
                          safeDetailItems.length,
                        ),
                      )
                    }
                  >
                    显示更多
                  </Button>
                ) : null}
              </>
            ) : !loading && !error ? (
              <div className="coze-prototype-usage-state">
                <span>暂无逐次用量明细</span>
                {onRetry ? (
                  <Button size="small" onClick={() => void onRetry()}>
                    刷新
                  </Button>
                ) : null}
              </div>
            ) : null}
          </section>
        ) : null}
        {viewMode !== 'off' ? (
          <p className="coze-prototype-usage-privacy-note">
            会话总量沿用后端线程聚合；逐次用量按 run 与 Agent
            回复关联。这里只展示已审核的统计与审计字段。
          </p>
        ) : null}
      </div>
    </SideSheet>
  );
};
