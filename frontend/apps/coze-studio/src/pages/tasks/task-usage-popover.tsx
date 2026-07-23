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

/* eslint-disable @coze-arch/max-line-per-function -- Compact usage summary and its resilient states form one popover workflow. */

import { useEffect, useMemo, useRef, useState, type RefObject } from 'react';

import {
  IconCozArrowDown,
  IconCozCross,
  IconCozSetting,
} from '@coze-arch/coze-design/icons';
import { Button, Popover, Spin } from '@coze-arch/coze-design';

import { TaskUsageDetailDrawer } from './task-usage-detail-drawer';
import { formatTaskTokenCount } from './task-token-usage-indicator';
import type { TaskUsageDetailItem } from './task-message-token-usage';
import type {
  TaskDetailTokenUsage,
  TaskTokenUsageViewMode,
} from './task-detail-loader';

const TaskUsageSummaryContent = ({
  currentReplyUsage,
  currentReplyIncomplete,
  dialogRef,
  error,
  isPartial,
  loadedCount,
  loading,
  onClose,
  onOpenDetail,
  onRetry,
  tokenUsage,
  totalCount,
}: {
  currentReplyUsage?: TaskDetailTokenUsage;
  currentReplyIncomplete: boolean;
  dialogRef: RefObject<HTMLElement>;
  error?: string;
  isPartial: boolean;
  loadedCount: number;
  loading: boolean;
  onClose: () => void;
  onOpenDetail: () => void;
  onRetry?: () => void | Promise<void>;
  tokenUsage?: TaskDetailTokenUsage;
  totalCount: number;
}) => (
  <section
    ref={dialogRef}
    aria-label="Token 用量"
    className="coze-prototype-token-usage-popover"
    data-testid="task-token-usage-popover"
    role="dialog"
    tabIndex={-1}
  >
    <div className="coze-prototype-usage-popover-heading">
      <strong>Token 用量</strong>
      <Button
        aria-label="关闭 Token 用量"
        className="coze-prototype-usage-popover-close"
        icon={<IconCozCross />}
        onClick={onClose}
      />
    </div>
    {tokenUsage ? (
      <>
        <div className="coze-prototype-usage-popover-total">
          <span>本次对话</span>
          <strong>{formatTaskTokenCount(tokenUsage.totalTokens)}</strong>
        </div>
        <dl className="coze-prototype-token-usage-popover-list">
          <div className="coze-prototype-token-usage-popover-row">
            <dt>输入</dt>
            <dd>{formatTaskTokenCount(tokenUsage.inputTokens)}</dd>
          </div>
          <div className="coze-prototype-token-usage-popover-row">
            <dt>输出</dt>
            <dd>{formatTaskTokenCount(tokenUsage.outputTokens)}</dd>
          </div>
          <div className="coze-prototype-token-usage-popover-row">
            <dt>会话总量</dt>
            <dd>{formatTaskTokenCount(tokenUsage.totalTokens)}</dd>
          </div>
        </dl>
      </>
    ) : null}
    {currentReplyUsage?.totalTokens || currentReplyIncomplete ? (
      <>
        <div className="coze-prototype-token-usage-popover-divider" />
        <div className="coze-prototype-token-usage-current">
          <span>
            当前回复
            {currentReplyUsage?.totalTokens && currentReplyIncomplete
              ? '（已加载）'
              : ''}
          </span>
          <strong>
            {currentReplyUsage?.totalTokens
              ? formatTaskTokenCount(currentReplyUsage.totalTokens)
              : '未完整加载'}
          </strong>
        </div>
      </>
    ) : null}
    {isPartial ? (
      <div className="coze-prototype-usage-partial" role="status">
        部分数据 · 已加载 {loadedCount} / {totalCount} 条明细
      </div>
    ) : null}
    {loading ? (
      <Spin spinning>
        <div className="coze-prototype-usage-state">正在加载 Token 用量...</div>
      </Spin>
    ) : error ? (
      <div className="coze-prototype-usage-state" role="alert">
        <span>{error}</span>
        {onRetry ? (
          <Button size="small" onClick={() => void onRetry()}>
            重试
          </Button>
        ) : null}
      </div>
    ) : !tokenUsage ? (
      <div className="coze-prototype-usage-state">
        <span>暂无 Token 用量</span>
        {onRetry ? (
          <Button size="small" onClick={() => void onRetry()}>
            刷新
          </Button>
        ) : null}
      </div>
    ) : null}
    <div className="coze-prototype-token-usage-popover-divider" />
    <Button
      className="coze-prototype-usage-detail-action"
      onClick={onOpenDetail}
    >
      <span>查看用量明细</span>
      <IconCozArrowDown aria-hidden="true" />
    </Button>
  </section>
);

export const TaskUsagePopover = ({
  currentReplyUsage,
  currentReplyIncomplete = false,
  detailItems,
  detailItemsFactory,
  error,
  hasAssistantReply = true,
  isPartial = false,
  loadedCount = 0,
  loading = false,
  onRetry,
  onLocateReply,
  onViewModeChange,
  scopeKey,
  tokenUsage,
  totalCount = 0,
  viewMode,
}: {
  currentReplyUsage?: TaskDetailTokenUsage;
  currentReplyIncomplete?: boolean;
  detailItems: TaskUsageDetailItem[];
  detailItemsFactory?: () => TaskUsageDetailItem[];
  error?: string;
  hasAssistantReply?: boolean;
  isPartial?: boolean;
  loadedCount?: number;
  loading?: boolean;
  onLocateReply?: (runID: string) => void;
  onRetry?: () => void | Promise<void>;
  onViewModeChange?: (mode: TaskTokenUsageViewMode) => void;
  scopeKey: string;
  tokenUsage?: TaskDetailTokenUsage;
  totalCount?: number;
  viewMode: TaskTokenUsageViewMode;
}) => {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLElement>(null);
  const pendingLocateRunIDRef = useRef('');
  const popoverWasVisibleRef = useRef(false);
  const drawerWasVisibleRef = useRef(false);
  const [popoverVisible, setPopoverVisible] = useState(false);
  const [drawerVisible, setDrawerVisible] = useState(false);
  const resolvedDetailItems = useMemo(
    () => (drawerVisible ? (detailItemsFactory?.() ?? detailItems) : []),
    [detailItems, detailItemsFactory, drawerVisible],
  );

  useEffect(() => {
    setPopoverVisible(false);
    setDrawerVisible(false);
  }, [scopeKey]);

  useEffect(() => {
    if (viewMode === 'off') {
      setPopoverVisible(false);
      setDrawerVisible(false);
    }
  }, [viewMode]);

  useEffect(() => {
    if (popoverVisible) {
      let cancelled = false;
      queueMicrotask(() => {
        if (cancelled) {
          return;
        }
        const dialog = dialogRef.current;
        const firstAction = dialog?.querySelector<HTMLElement>(
          'button:not([disabled]), [href], [tabindex]:not([tabindex="-1"])',
        );
        (firstAction ?? dialog)?.focus();
      });
      popoverWasVisibleRef.current = true;
      return () => {
        cancelled = true;
      };
    } else if (popoverWasVisibleRef.current && !drawerVisible) {
      triggerRef.current?.focus();
    }
    popoverWasVisibleRef.current = popoverVisible;
  }, [drawerVisible, popoverVisible]);

  useEffect(() => {
    if (!drawerVisible && drawerWasVisibleRef.current) {
      triggerRef.current?.focus();
      const pendingRunID = pendingLocateRunIDRef.current;
      if (pendingRunID) {
        pendingLocateRunIDRef.current = '';
        queueMicrotask(() => onLocateReply?.(pendingRunID));
      }
    }
    drawerWasVisibleRef.current = drawerVisible;
  }, [drawerVisible, onLocateReply]);

  useEffect(() => {
    if (!popoverVisible) {
      return;
    }
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') {
        return;
      }
      setPopoverVisible(false);
    };
    const handleOutsidePointer = (event: MouseEvent) => {
      const { target } = event;
      if (
        !(target instanceof Node) ||
        triggerRef.current?.contains(target) ||
        dialogRef.current?.contains(target)
      ) {
        return;
      }
      setPopoverVisible(false);
    };
    document.addEventListener('keydown', handleEscape);
    document.addEventListener('mousedown', handleOutsidePointer);
    return () => {
      document.removeEventListener('keydown', handleEscape);
      document.removeEventListener('mousedown', handleOutsidePointer);
    };
  }, [popoverVisible]);

  const closePopover = () => {
    setPopoverVisible(false);
  };
  const openDrawer = () => {
    setDrawerVisible(true);
    setPopoverVisible(false);
  };
  const closeDrawer = () => {
    setDrawerVisible(false);
  };
  const locateReply = (runID: string) => {
    pendingLocateRunIDRef.current = runID;
    setDrawerVisible(false);
  };
  const totalVisible = viewMode !== 'off' && Boolean(tokenUsage?.totalTokens);
  const showCurrentReply = viewMode === 'per_turn' || viewMode === 'debug';
  const detailText = tokenUsage
    ? `输入 ${formatTaskTokenCount(
        tokenUsage.inputTokens,
      )} · 输出 ${formatTaskTokenCount(
        tokenUsage.outputTokens,
      )} · 总计 ${formatTaskTokenCount(tokenUsage.totalTokens)}`
    : '查看 Token 用量';
  const content = (
    <TaskUsageSummaryContent
      currentReplyUsage={showCurrentReply ? currentReplyUsage : undefined}
      currentReplyIncomplete={showCurrentReply && currentReplyIncomplete}
      dialogRef={dialogRef}
      error={error}
      isPartial={isPartial}
      loadedCount={loadedCount}
      loading={loading}
      tokenUsage={tokenUsage}
      totalCount={totalCount}
      onClose={closePopover}
      onOpenDetail={openDrawer}
      onRetry={onRetry}
    />
  );

  if (viewMode === 'off') {
    return (
      <>
        <button
          ref={triggerRef}
          type="button"
          aria-expanded={drawerVisible}
          aria-haspopup="dialog"
          aria-label="用量显示设置"
          className="coze-prototype-token-usage-settings"
          data-open={drawerVisible}
          title="用量显示设置"
          onClick={() => setDrawerVisible(true)}
        >
          <IconCozSetting aria-hidden="true" />
        </button>
        <TaskUsageDetailDrawer
          detailItems={[]}
          isPartial={false}
          loadedCount={0}
          loading={false}
          totalCount={0}
          viewMode={viewMode}
          visible={drawerVisible}
          onCancel={closeDrawer}
          onViewModeChange={onViewModeChange}
        />
      </>
    );
  }

  if (!hasAssistantReply) {
    return null;
  }

  return (
    <>
      <Popover
        content={popoverVisible ? content : null}
        position="topRight"
        showArrow
        trigger="custom"
        visible={popoverVisible}
      >
        <button
          ref={triggerRef}
          type="button"
          aria-expanded={popoverVisible}
          aria-haspopup="dialog"
          aria-label={
            totalVisible
              ? `查看 Token 用量，总计 ${formatTaskTokenCount(
                  tokenUsage?.totalTokens ?? 0,
                )}`
              : '查看 Token 用量'
          }
          className="coze-prototype-token-usage"
          data-open={popoverVisible}
          data-testid="task-usage-trigger"
          title={viewMode === 'off' ? '查看 Token 用量' : detailText}
          onClick={() => {
            queueMicrotask(() => setPopoverVisible(!popoverVisible));
          }}
        >
          <span>用量</span>
          {totalVisible ? (
            <span className="coze-prototype-token-usage-count">
              {formatTaskTokenCount(tokenUsage?.totalTokens ?? 0)}
            </span>
          ) : null}
          <IconCozArrowDown aria-hidden="true" />
        </button>
      </Popover>
      <TaskUsageDetailDrawer
        detailItems={resolvedDetailItems}
        error={error}
        isPartial={isPartial}
        loadedCount={loadedCount}
        loading={loading}
        tokenUsage={tokenUsage}
        totalCount={totalCount}
        viewMode={viewMode}
        visible={drawerVisible}
        onCancel={closeDrawer}
        onLocateReply={locateReply}
        onRetry={onRetry}
        onViewModeChange={onViewModeChange}
      />
    </>
  );
};
