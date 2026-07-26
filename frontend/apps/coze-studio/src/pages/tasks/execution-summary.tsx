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

import { useMemo, useState, type ReactNode } from 'react';

import {
  IconCozArrowDown,
  IconCozCheckMarkCircleFill,
  IconCozCode,
  IconCozDocument,
  IconCozEdit,
  IconCozLightbulb,
  IconCozLoading,
  IconCozMagnifier,
  IconCozPlugin,
  IconCozWarningCircleFill,
} from '@coze-arch/coze-design/icons';

import type {
  TaskThreadDetailEvent,
  TaskThreadDetailModel,
} from './task-thread-detail-model';
import { projectTaskExecutionEvents } from './task-event-projection';
import { isTaskTerminalStatus, type TaskThreadEventDisplay } from './helpers';

const isPathDetail = (detail?: string) =>
  detail?.startsWith('/mnt/') || detail?.startsWith('write-file:');

const getStepIcon = (
  display: TaskThreadEventDisplay,
): { key: string; icon: ReactNode } => {
  if (display.kind === 'thought') {
    return { key: 'thought', icon: <IconCozLightbulb /> };
  }

  const title = display.title.toLowerCase();
  const detail = display.detail?.toLowerCase() ?? '';

  if (title.includes('搜索网页') || title.includes('web_search')) {
    return { key: 'search', icon: <IconCozMagnifier /> };
  }
  if (title.includes('to-do') || title.includes('todo')) {
    return { key: 'todo', icon: <IconCozDocument /> };
  }
  if (
    title.includes('文件') ||
    title.includes('文档') ||
    title.includes('file') ||
    detail.startsWith('/mnt/') ||
    detail.startsWith('write-file:')
  ) {
    return { key: 'edit', icon: <IconCozEdit /> };
  }
  if (title.includes('命令') || title.includes('command')) {
    return { key: 'terminal', icon: <IconCozCode /> };
  }

  return { key: 'tool', icon: <IconCozPlugin /> };
};

const normalizeTimestamp = (value?: number) => {
  if (!value) {
    return undefined;
  }

  return value < 1_000_000_000_000 ? value * 1000 : value;
};

const formatDuration = (events: TaskThreadDetailEvent[]) => {
  const timestamps = events
    .map(event => normalizeTimestamp(event.created_at))
    .filter((value): value is number => Boolean(value));

  if (timestamps.length < 2) {
    return undefined;
  }

  const duration = Math.max(...timestamps) - Math.min(...timestamps);
  if (duration < 1000) {
    return '< 1 秒';
  }
  if (duration < 60_000) {
    return `${Math.round(duration / 1000)} 秒`;
  }

  return `${Math.round(duration / 60_000)} 分钟`;
};

const getSummaryStatus = (
  displayItems: ReturnType<typeof projectTaskExecutionEvents>,
  task: TaskThreadDetailModel,
) => {
  if (displayItems.some(item => item.display.status === 'failed')) {
    return 'failed' as const;
  }
  if (!isTaskTerminalStatus(task.status)) {
    return 'running' as const;
  }

  return 'completed' as const;
};

const SummaryStatusIcon = ({
  status,
}: {
  status: 'completed' | 'failed' | 'running';
}) => (
  <span
    className="coze-prototype-execution-summary-status"
    data-status={status}
  >
    {status === 'failed' ? (
      <IconCozWarningCircleFill />
    ) : status === 'running' ? (
      <IconCozLoading />
    ) : (
      <IconCozCheckMarkCircleFill />
    )}
  </span>
);

export const TaskExecutionSummary = ({
  events,
  task,
}: {
  events: TaskThreadDetailEvent[];
  task: TaskThreadDetailModel;
}) => {
  const [expanded, setExpanded] = useState(false);
  const displayItems = useMemo(() => {
    const projected = projectTaskExecutionEvents(events);
    const hasStructuredItems = projected.some(item => item.display.structured);
    const visibleItems = hasStructuredItems
      ? projected.filter(item => item.display.structured)
      : projected;

    return visibleItems.map(item => {
      if (
        !isTaskTerminalStatus(task.status) ||
        item.display.status !== 'running'
      ) {
        return item;
      }

      return {
        ...item,
        display: { ...item.display, status: 'completed' as const },
      };
    });
  }, [events, task.status]);

  if (!displayItems.length) {
    return null;
  }

  const status = getSummaryStatus(displayItems, task);
  const duration = formatDuration(events);
  const statusLabel =
    status === 'failed'
      ? `执行失败 · ${displayItems.length} 个步骤`
      : status === 'running'
        ? `正在执行 · ${displayItems.length} 个步骤`
        : `已完成 ${displayItems.length} 个步骤`;

  return (
    <section
      aria-label="执行流程"
      className="coze-prototype-execution-feed coze-prototype-reasoning-panel coze-prototype-chain-of-thought"
      data-status={status}
    >
      <button
        type="button"
        className="coze-prototype-execution-summary-trigger"
        aria-expanded={expanded}
        onClick={() => setExpanded(value => !value)}
      >
        <SummaryStatusIcon status={status} />
        <span className="coze-prototype-execution-summary-copy">
          <strong>{statusLabel}</strong>
          <span aria-hidden="true">·</span>
          <span>可用技能目录</span>
        </span>
        {expanded && duration ? (
          <span className="coze-prototype-execution-summary-duration">
            {duration}
          </span>
        ) : null}
        <span
          className="coze-prototype-step-more-chevron"
          data-open={expanded}
          aria-hidden="true"
        >
          <IconCozArrowDown />
        </span>
      </button>
      <ol
        className="coze-prototype-execution-feed-list coze-prototype-chain-content"
        hidden={!expanded}
      >
        {expanded
          ? displayItems.map(({ event, display }) => {
              const { key, icon } = getStepIcon(display);

              return (
                <li
                  key={event.id}
                  className="coze-prototype-execution-feed-item coze-prototype-step"
                  data-kind={display.kind}
                  data-status={display.status}
                >
                  <span
                    className="coze-prototype-step-icon"
                    data-icon={key}
                    data-status={display.status}
                    aria-hidden="true"
                  >
                    {display.status === 'failed' ? (
                      <IconCozWarningCircleFill />
                    ) : display.status === 'running' ? (
                      <IconCozLoading />
                    ) : (
                      icon
                    )}
                  </span>
                  <span className="coze-prototype-step-content">
                    {display.kind === 'thought' && display.thought ? (
                      <span className="coze-prototype-step-thought">
                        {display.thought}
                      </span>
                    ) : (
                      <>
                        <span className="coze-prototype-step-title-row">
                          <span className="coze-prototype-step-title">
                            {display.title}
                          </span>
                          {display.runtime && display.runtime !== 'Agent' ? (
                            <span className="coze-prototype-step-runtime">
                              {display.runtime}
                            </span>
                          ) : null}
                        </span>
                        {display.detail ? (
                          <span
                            className="coze-prototype-step-detail"
                            data-kind={
                              isPathDetail(display.detail) ? 'path' : 'text'
                            }
                          >
                            {display.detail}
                          </span>
                        ) : null}
                        {display.thought ? (
                          <span className="coze-prototype-step-thought">
                            {display.thought}
                          </span>
                        ) : null}
                      </>
                    )}
                  </span>
                </li>
              );
            })
          : null}
      </ol>
    </section>
  );
};
