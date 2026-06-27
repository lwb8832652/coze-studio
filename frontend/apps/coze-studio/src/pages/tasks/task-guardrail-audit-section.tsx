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

import { IconCozDownload, IconCozRefresh } from '@coze-arch/coze-design/icons';
import { Button, Spin, Tag } from '@coze-arch/coze-design';

import {
  exportTaskThreadGuardrailAuditEvents,
  listTaskThreadGuardrailAuditEvents,
  type ExportTaskThreadGuardrailAuditEventsData,
  type TaskThreadGuardrailAuditEvent,
} from './service';
import { formatUpdatedTime } from './helpers';

const GUARDRAIL_AUDIT_PAGE_SIZE = 20;
const GUARDRAIL_AUDIT_EXPORT_PAGE_SIZE = 1000;
const GUARDRAIL_AUDIT_EXPORT_SCHEMA =
  'coze.task_thread_guardrail_audit.export.v1';

const actionLabel = (action: string) => {
  switch (action) {
    case 'warn':
      return '已警告';
    case 'confirm':
      return '需要确认';
    case 'deny':
      return '已拒绝';
    case 'allow':
      return '已放行';
    default:
      return action || '未知';
  }
};

const actionTagColor = (action: string) => {
  switch (action) {
    case 'deny':
      return 'red';
    case 'confirm':
      return 'amber';
    case 'warn':
      return 'orange';
    case 'allow':
      return 'green';
    default:
      return 'grey';
  }
};

const parseRuleIDs = (value: string) => {
  const trimmed = value.trim();
  if (!trimmed) {
    return [];
  }

  try {
    const parsed: unknown = JSON.parse(trimmed);
    if (Array.isArray(parsed)) {
      return parsed.filter(
        (item): item is string =>
          typeof item === 'string' && Boolean(item.trim()),
      );
    }
  } catch (error) {
    void error;
    // Fall back to comma-separated rule IDs below.
  }

  return trimmed
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
};

const buildGuardrailAuditMeta = (event: TaskThreadGuardrailAuditEvent) =>
  [
    [event.target_type, event.target_id].filter(Boolean).join(' · '),
    event.operation,
    event.source,
    event.fail_mode,
    event.provider,
    event.run_id ? `run ${event.run_id}` : '',
    `记录 ${formatUpdatedTime(event.created_at)}`,
  ]
    .filter(Boolean)
    .join(' · ');

const toGuardrailAuditExportEvent = (event: TaskThreadGuardrailAuditEvent) => ({
  action: event.action,
  actor_id: event.actor_id,
  created_at: event.created_at,
  event_id: event.event_id,
  event_type: event.event_type,
  fail_mode: event.fail_mode,
  operation: event.operation,
  provider: event.provider,
  reason_code: event.reason_code,
  rule_ids: parseRuleIDs(event.rule_ids),
  run_id: event.run_id,
  source: event.source,
  space_id: event.space_id,
  target_id: event.target_id,
  target_type: event.target_type,
  thread_id: event.thread_id,
});

const downloadGuardrailAuditExport = ({
  data,
  threadId,
}: {
  data: ExportTaskThreadGuardrailAuditEventsData;
  threadId: string;
}) => {
  const exportedAt = data.exported_at || Date.now();
  const payload = {
    schema: data.schema || GUARDRAIL_AUDIT_EXPORT_SCHEMA,
    thread_id: data.thread_id || threadId,
    exported_at: exportedAt,
    page: data.page,
    page_size: data.page_size,
    total: data.total,
    events: data.events.map(toGuardrailAuditExportEvent),
  };
  const blob = new Blob([JSON.stringify(payload, null, 2)], {
    type: 'application/json;charset=utf-8',
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `guardrail-audit-${threadId}-${exportedAt}.json`;
  link.rel = 'noopener';
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

const fetchGuardrailAuditExportData = async ({
  threadId,
}: {
  threadId: string;
}): Promise<ExportTaskThreadGuardrailAuditEventsData> => {
  let page = 1;
  let firstPage: ExportTaskThreadGuardrailAuditEventsData | undefined;
  let total = 0;
  const events: TaskThreadGuardrailAuditEvent[] = [];

  while (true) {
    const response = await exportTaskThreadGuardrailAuditEvents({
      thread_id: threadId,
      page,
      page_size: GUARDRAIL_AUDIT_EXPORT_PAGE_SIZE,
    });
    if (!response.data) {
      throw new Error('导出安全审计失败');
    }

    if (!firstPage) {
      firstPage = response.data;
    }
    const pageEvents = response.data.events ?? [];
    total = response.data.total;
    events.push(...pageEvents);
    if (!pageEvents.length || events.length >= total) {
      break;
    }
    page += 1;
  }

  if (!firstPage) {
    throw new Error('导出安全审计失败');
  }

  return {
    ...firstPage,
    events,
    page: 1,
    page_size: GUARDRAIL_AUDIT_EXPORT_PAGE_SIZE,
    total,
  };
};

const TaskGuardrailAuditRow = ({
  event,
}: {
  event: TaskThreadGuardrailAuditEvent;
}) => {
  const ruleIDs = parseRuleIDs(event.rule_ids);
  const visibleRuleIDs = ruleIDs.slice(0, 4);
  const hiddenRuleCount = ruleIDs.length - visibleRuleIDs.length;

  return (
    <li
      className="coze-prototype-guardrail-audit-row"
      data-action={event.action}
      data-testid="task-guardrail-audit-row"
    >
      <span className="coze-prototype-guardrail-audit-main">
        <span className="coze-prototype-guardrail-audit-title-row">
          <Tag color={actionTagColor(event.action)} size="small" type="light">
            {actionLabel(event.action)}
          </Tag>
          <span className="coze-prototype-guardrail-audit-title">
            {event.reason_code || event.event_type}
          </span>
        </span>
        <span className="coze-prototype-guardrail-audit-meta">
          {buildGuardrailAuditMeta(event)}
        </span>
        {visibleRuleIDs.length ? (
          <span className="coze-prototype-guardrail-rule-list">
            {visibleRuleIDs.map(ruleID => (
              <Tag key={ruleID} color="grey" size="small" type="ghost">
                {ruleID}
              </Tag>
            ))}
            {hiddenRuleCount > 0 ? (
              <Tag color="grey" size="small" type="ghost">
                +{hiddenRuleCount}
              </Tag>
            ) : null}
          </span>
        ) : null}
      </span>
    </li>
  );
};

export const TaskGuardrailAuditSection = ({
  threadId,
}: {
  threadId?: string;
}) => {
  const [events, setEvents] = useState<TaskThreadGuardrailAuditEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [exportLoading, setExportLoading] = useState(false);
  const [error, setError] = useState('');

  const loadEvents = useCallback(async () => {
    if (!threadId) {
      setEvents([]);
      setTotal(0);
      return;
    }
    setLoading(true);
    setError('');
    try {
      const response = await listTaskThreadGuardrailAuditEvents({
        thread_id: threadId,
        page: 1,
        page_size: GUARDRAIL_AUDIT_PAGE_SIZE,
      });
      setEvents(response.data?.events ?? []);
      setTotal(response.data?.total ?? 0);
    } catch (err) {
      setEvents([]);
      setTotal(0);
      setError(err instanceof Error ? err.message : '加载安全审计失败');
    } finally {
      setLoading(false);
    }
  }, [threadId]);

  useEffect(() => {
    void loadEvents();
  }, [loadEvents]);

  const sortedEvents = useMemo(
    () => [...events].sort((a, b) => b.created_at - a.created_at),
    [events],
  );

  const handleExport = useCallback(async () => {
    if (!threadId || exportLoading) {
      return;
    }
    setExportLoading(true);
    setError('');
    try {
      const data = await fetchGuardrailAuditExportData({ threadId });
      downloadGuardrailAuditExport({
        data,
        threadId,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : '导出安全审计失败');
    } finally {
      setExportLoading(false);
    }
  }, [exportLoading, threadId]);

  if (!threadId) {
    return null;
  }

  return (
    <section
      className="coze-prototype-guardrail-audit-panel"
      data-testid="task-guardrail-audit-panel"
    >
      <div className="coze-prototype-guardrail-audit-header">
        <h2>安全审计</h2>
        <span>{total} 条</span>
        <Button
          aria-label="刷新安全审计"
          icon={<IconCozRefresh />}
          loading={loading}
          size="small"
          theme="borderless"
          type="tertiary"
          onClick={() => void loadEvents()}
        >
          刷新
        </Button>
        <Button
          aria-label="导出安全审计"
          disabled={!total}
          icon={<IconCozDownload />}
          loading={exportLoading}
          size="small"
          theme="borderless"
          type="tertiary"
          onClick={() => void handleExport()}
        >
          导出审计
        </Button>
      </div>
      {error ? (
        <div className="coze-prototype-guardrail-audit-error" role="alert">
          {error}
        </div>
      ) : null}
      <Spin spinning={loading}>
        {sortedEvents.length ? (
          <ol className="coze-prototype-guardrail-audit-list">
            {sortedEvents.map(event => (
              <TaskGuardrailAuditRow key={event.event_id} event={event} />
            ))}
          </ol>
        ) : (
          <div className="coze-prototype-guardrail-audit-empty">
            {loading ? '加载安全审计中...' : '暂无安全审计'}
          </div>
        )}
      </Spin>
    </section>
  );
};
