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

import { Button } from '@coze-arch/coze-design';

import type { TaskDetailSubagentRun } from './task-detail-loader';
import { formatUpdatedTime } from './helpers';

const getSubagentRunMarker = (status: TaskDetailSubagentRun['status']) => {
  if (status === 'failed') {
    return '!';
  }
  if (status === 'completed') {
    return '✓';
  }
  if (status === 'canceled') {
    return '×';
  }

  return '•';
};

const formatSubagentDuration = (elapsedMs: number) => {
  if (elapsedMs <= 0) {
    return '';
  }
  if (elapsedMs < 1000) {
    return `${elapsedMs}ms`;
  }

  const seconds = elapsedMs / 1000;

  return `${Number.isInteger(seconds) ? seconds.toFixed(0) : seconds.toFixed(1)}s`;
};

const formatSubagentTokenCount = (tokens: number) =>
  new Intl.NumberFormat('en-US').format(tokens);

const formatSubagentCost = (run: TaskDetailSubagentRun) => {
  const { tokenUsage } = run;
  if (!tokenUsage?.currency || tokenUsage.costMicros <= 0) {
    return '';
  }

  return `Cost ${tokenUsage.currency} ${(tokenUsage.costMicros / 1_000_000).toFixed(6)}`;
};

const canRetrySubagentRun = (status: TaskDetailSubagentRun['status']) =>
  status === 'failed' || status === 'canceled';

const formatSubagentRetrySummary = (run: TaskDetailSubagentRun) => {
  const retryCount = run.retryAttempts?.length ?? 0;
  if (!retryCount) {
    return '';
  }

  const latestRetry = run.retryAttempts?.[0];
  const latestText = latestRetry?.statusText
    ? `最近重试：${latestRetry.statusText}`
    : '';

  return [`重试 ${retryCount} 次`, latestText].filter(Boolean).join(' · ');
};

type TaskDetailSubagentTimelineItem = NonNullable<
  TaskDetailSubagentRun['timeline']
>[number];

const getSubagentRunDetail = (run: TaskDetailSubagentRun) =>
  [
    run.errorMessage || run.assistantId,
    run.elapsedMs ? `耗时 ${formatSubagentDuration(run.elapsedMs)}` : '',
    run.terminalClassification,
    formatSubagentRetrySummary(run),
    run.modelAttribution,
    run.tokenUsage
      ? `Token ${formatSubagentTokenCount(run.tokenUsage.totalTokens)}`
      : '',
    formatSubagentCost(run),
    run.tokenUsage?.callCount ? `${run.tokenUsage.callCount} calls` : '',
  ]
    .filter(Boolean)
    .join(' · ');

const getSubagentTimelineDetail = (item: TaskDetailSubagentTimelineItem) =>
  [
    item.elapsedMs ? `耗时 ${formatSubagentDuration(item.elapsedMs)}` : '',
    item.terminalClassification,
    item.errorMessage,
  ]
    .filter(Boolean)
    .join(' · ');

const TaskSubagentTimeline = ({
  timeline,
}: {
  timeline?: TaskDetailSubagentRun['timeline'];
}) => {
  if (!timeline?.length) {
    return null;
  }

  return (
    <details className="coze-prototype-subagent-timeline">
      <summary>执行明细</summary>
      <ol>
        {timeline.map(item => {
          const detail = getSubagentTimelineDetail(item);

          return (
            <li key={item.id} data-status={item.status}>
              <span className="coze-prototype-subagent-timeline-marker">
                {getSubagentRunMarker(item.status)}
              </span>
              <span className="coze-prototype-subagent-timeline-main">
                <span className="coze-prototype-subagent-timeline-title">
                  {item.title}
                </span>
                {detail ? (
                  <span className="coze-prototype-subagent-timeline-detail">
                    {detail}
                  </span>
                ) : null}
              </span>
              <span className="coze-prototype-muted shrink-0">
                {formatUpdatedTime(item.createdAt)}
              </span>
            </li>
          );
        })}
      </ol>
    </details>
  );
};

export const TaskSubagentRunsSection = ({
  onRetrySubagentRun,
  retryError,
  retryingRunId,
  subagentRuns,
}: {
  onRetrySubagentRun?: (runId: string) => void | Promise<void>;
  retryError?: string;
  retryingRunId?: string;
  subagentRuns: TaskDetailSubagentRun[];
}) => {
  if (!subagentRuns.length) {
    return null;
  }

  return (
    <section className="coze-prototype-subagent-panel">
      <div className="coze-prototype-subagent-header">
        <h2>子智能体执行</h2>
        <span>{subagentRuns.length} 个子任务</span>
      </div>
      {retryError ? (
        <div className="coze-prototype-subagent-retry-error" role="alert">
          {retryError}
        </div>
      ) : null}
      <ol className="coze-prototype-subagent-list">
        {subagentRuns.map(run => {
          const detail = getSubagentRunDetail(run);
          const retrying = retryingRunId === run.runId;
          const showRetry =
            Boolean(onRetrySubagentRun) && canRetrySubagentRun(run.status);

          return (
            <li key={run.runId}>
              <span
                className="coze-prototype-subagent-row"
                data-status={run.status}
              >
                {run.status === 'running' ? (
                  <span className="coze-prototype-step-running" />
                ) : (
                  <span className="coze-prototype-step-check">
                    {getSubagentRunMarker(run.status)}
                  </span>
                )}
                <span className="coze-prototype-subagent-main">
                  <span className="coze-prototype-subagent-title-row">
                    <span className="coze-prototype-subagent-name">
                      {run.name}
                    </span>
                    <span className="coze-prototype-subagent-status">
                      {run.statusText}
                    </span>
                  </span>
                  {detail ? (
                    <span className="coze-prototype-subagent-detail">
                      {detail}
                    </span>
                  ) : null}
                </span>
                <span className="coze-prototype-subagent-actions">
                  {showRetry ? (
                    <Button
                      size="small"
                      theme="borderless"
                      type="primary"
                      disabled={retrying}
                      loading={retrying}
                      onClick={() => void onRetrySubagentRun?.(run.runId)}
                    >
                      重试
                    </Button>
                  ) : null}
                  <span className="coze-prototype-muted shrink-0">
                    {formatUpdatedTime(run.updatedAt || run.startedAt)}
                  </span>
                </span>
              </span>
              <TaskSubagentTimeline timeline={run.timeline} />
            </li>
          );
        })}
      </ol>
    </section>
  );
};
