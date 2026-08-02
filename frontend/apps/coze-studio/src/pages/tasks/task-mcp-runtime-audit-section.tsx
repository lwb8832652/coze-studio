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

import { useCallback, useEffect, useMemo, useState } from 'react';

import { IconCozRefresh } from '@coze-arch/coze-design/icons';
import { Button, Spin, Tag } from '@coze-arch/coze-design';

import {
  listTaskThreadMCPRuntimeAuditEvents,
  type TaskThreadMCPRuntimeAuditEvent,
} from './service';
import { formatUpdatedTime } from './helpers';

const MCP_RUNTIME_AUDIT_PAGE_SIZE = 20;

const eventTypeLabel = (eventType: string) => {
  switch (eventType) {
    case 'tool.completed':
      return '完成';
    case 'tool.failed':
      return '失败';
    case 'tool.started':
      return '开始';
    default:
      return eventType || '未知';
  }
};

const eventTypeTagColor = (eventType: string) => {
  switch (eventType) {
    case 'tool.completed':
      return 'green';
    case 'tool.failed':
      return 'red';
    default:
      return 'grey';
  }
};

const formatOutputBytes = (bytes: number) => {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B';
  }

  if (bytes < 1024) {
    return `${bytes} B`;
  }

  const kilobytes = bytes / 1024;

  return `${Number.isInteger(kilobytes) ? kilobytes : kilobytes.toFixed(1)} KB`;
};

const buildMCPRuntimeAuditMeta = (event: TaskThreadMCPRuntimeAuditEvent) =>
  [
    event.server_id ? `server ${event.server_id}` : '',
    event.run_id ? `run ${event.run_id}` : '',
    `耗时 ${event.elapsed_millis}ms`,
    `输出 ${formatOutputBytes(event.output_bytes)}`,
    event.error_code ? `错误 ${event.error_code}` : '',
    `记录 ${formatUpdatedTime(event.created_at)}`,
  ]
    .filter(Boolean)
    .join(' · ');

const TaskMCPRuntimeAuditRow = ({
  event,
}: {
  event: TaskThreadMCPRuntimeAuditEvent;
}) => (
  <li
    className="coze-prototype-mcp-runtime-audit-row"
    data-event-type={event.event_type}
    data-testid="task-mcp-runtime-audit-row"
  >
    <span className="coze-prototype-mcp-runtime-audit-main">
      <span className="coze-prototype-mcp-runtime-audit-title-row">
        <Tag
          color={eventTypeTagColor(event.event_type)}
          size="small"
          type="light"
        >
          {eventTypeLabel(event.event_type)}
        </Tag>
        <span className="coze-prototype-mcp-runtime-audit-title">
          {event.runtime_tool_name || event.event_type}
        </span>
      </span>
      <span className="coze-prototype-mcp-runtime-audit-meta">
        {buildMCPRuntimeAuditMeta(event)}
      </span>
      <span className="coze-prototype-mcp-runtime-audit-event-type">
        {event.event_type}
      </span>
    </span>
  </li>
);

export const TaskMCPRuntimeAuditSection = ({
  spaceId,
  threadId,
}: {
  spaceId?: string;
  threadId?: string;
}) => {
  const [events, setEvents] = useState<TaskThreadMCPRuntimeAuditEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  const loadEvents = useCallback(async () => {
    if (!spaceId || !threadId) {
      setEvents([]);
      setTotal(0);
      return;
    }

    setLoading(true);
    setError('');
    try {
      const response = await listTaskThreadMCPRuntimeAuditEvents({
        thread_id: threadId,
        space_id: spaceId,
        page: 1,
        page_size: MCP_RUNTIME_AUDIT_PAGE_SIZE,
      });
      setEvents(response.data?.events ?? []);
      setTotal(response.data?.total ?? 0);
    } catch (err) {
      setEvents([]);
      setTotal(0);
      setError(err instanceof Error ? err.message : '加载工具调用失败');
    } finally {
      setLoading(false);
    }
  }, [spaceId, threadId]);

  useEffect(() => {
    void loadEvents();
  }, [loadEvents]);

  const sortedEvents = useMemo(
    () => [...events].sort((a, b) => b.created_at - a.created_at),
    [events],
  );

  if (!spaceId || !threadId) {
    return null;
  }

  return (
    <section
      className="coze-prototype-mcp-runtime-audit-panel"
      data-testid="task-mcp-runtime-audit-panel"
    >
      <div className="coze-prototype-mcp-runtime-audit-header">
        <h2>工具调用</h2>
        <span>{total} 条</span>
        <Button
          aria-label="刷新工具调用"
          icon={<IconCozRefresh />}
          loading={loading}
          size="small"
          theme="borderless"
          type="tertiary"
          onClick={() => void loadEvents()}
        >
          刷新
        </Button>
      </div>
      {error ? (
        <div className="coze-prototype-mcp-runtime-audit-error" role="alert">
          {error}
        </div>
      ) : null}
      <Spin spinning={loading}>
        {sortedEvents.length ? (
          <ol className="coze-prototype-mcp-runtime-audit-list">
            {sortedEvents.map(event => (
              <TaskMCPRuntimeAuditRow key={event.event_id} event={event} />
            ))}
          </ol>
        ) : (
          <div className="coze-prototype-mcp-runtime-audit-empty">
            {loading ? '加载工具调用中...' : '暂无工具调用'}
          </div>
        )}
      </Spin>
    </section>
  );
};
