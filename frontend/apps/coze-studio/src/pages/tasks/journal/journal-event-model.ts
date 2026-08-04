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

import type {
  WorkbenchJournalActionEventData,
  WorkbenchJournalContentType,
  WorkbenchJournalEvent,
  WorkbenchJournalExecutionStatus,
} from '../../workbench/thread-client';

export type JournalActionKind =
  | WorkbenchJournalContentType
  | 'generic'
  | 'artifact'
  | 'verification'
  | 'confirmation';

export interface JournalActionItem {
  id: string;
  milestone_id?: string;
  title: string;
  detail: string;
  kind: JournalActionKind;
  status: WorkbenchJournalExecutionStatus;
  event: WorkbenchJournalEvent;
}

export type JournalTimelineSemanticKind =
  | 'milestone'
  | 'action'
  | 'artifact'
  | 'verification'
  | 'confirmation';

export interface JournalTimelineItem extends JournalActionItem {
  semanticKind: JournalTimelineSemanticKind;
  aggregateEligible: boolean;
}

export interface JournalFailureDetails {
  summary: string;
  retryable: boolean;
  ledgerStatus?: string;
  traceID?: string;
}

export interface JournalMilestoneItem {
  id: string;
  title: string;
  status: WorkbenchJournalExecutionStatus;
  event: WorkbenchJournalEvent;
  actions: JournalActionItem[];
  atomic: boolean;
  order: number;
}

const contentTypes = new Set<WorkbenchJournalContentType>([
  'document',
  'terminal',
  'code',
  'skill',
  'browser',
]);

const asRecord = (value: unknown): Record<string, unknown> =>
  value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};

const asText = (value: unknown): string =>
  typeof value === 'string' ? value.trim() : '';

const asStatus = (
  status?: WorkbenchJournalExecutionStatus,
): WorkbenchJournalExecutionStatus => status ?? 'running';

export const journalEventData = (
  event: WorkbenchJournalEvent,
): Record<string, unknown> => {
  const payload = asRecord(event.payload);
  return asRecord(payload.data);
};

const journalPayloadType = (event: WorkbenchJournalEvent): string =>
  asText(asRecord(event.payload).type);

const eventOrder = (event: WorkbenchJournalEvent): number =>
  event.sequence ?? event.occurred_at ?? event.created_at;

const laterEvent = (
  current: WorkbenchJournalEvent,
  candidate: WorkbenchJournalEvent,
): WorkbenchJournalEvent =>
  eventOrder(candidate) >= eventOrder(current) ? candidate : current;

const contentTypeFromData = (
  event: WorkbenchJournalEvent,
  data: Record<string, unknown>,
): WorkbenchJournalContentType | undefined => {
  const explicit = asText(data.content_type);
  if (contentTypes.has(explicit as WorkbenchJournalContentType)) {
    return explicit as WorkbenchJournalContentType;
  }
  const payloadType = journalPayloadType(event);
  if (contentTypes.has(payloadType as WorkbenchJournalContentType)) {
    return payloadType as WorkbenchJournalContentType;
  }

  const operation = asText(data.operation).toLowerCase();
  if (operation === 'use_skill') {
    return 'skill';
  }
  if (/browser|browse|search_web|navigate|capture/.test(operation)) {
    return 'browser';
  }
  if (/terminal|shell|command|execute/.test(operation)) {
    return 'terminal';
  }
  if (/code|edit|patch/.test(operation)) {
    return 'code';
  }
  if (/document|read|write/.test(operation)) {
    return 'document';
  }
  return undefined;
};

const normalizeVerb = (verb: string, target: string): string => {
  if (!target || verb.includes(target)) {
    return verb || target;
  }
  const conciseVerb = verb.replace(/(?:代码|文档|文件|子任务|技能)$/u, '');
  return `${conciseVerb || verb} ${target}`.trim();
};

const journalActionVerb = (
  status: WorkbenchJournalExecutionStatus,
  data: Record<string, unknown>,
): string => {
  const runningVerb = asText(data.display_verb_running);
  const completedVerb = asText(data.display_verb_completed);
  const actionStem =
    completedVerb.replace(/^已/u, '') ||
    runningVerb.replace(/^正在/u, '') ||
    '执行';
  switch (status) {
    case 'completed':
      return completedVerb;
    case 'failed':
      return `${actionStem}失败`;
    case 'timed_out':
      return `${actionStem}超时`;
    case 'cancelled':
      return `已取消${actionStem}`;
    default:
      return runningVerb;
  }
};

export const journalActionLabel = (event: WorkbenchJournalEvent): string => {
  const data = journalEventData(event);
  const target = asText(data.target);
  const status = asStatus(event.status);
  const verb = journalActionVerb(status, data);
  return normalizeVerb(verb, target) || target || '执行操作';
};

const journalActionTitles = new Map<string, string>([
  ['read', '读取相关内容'],
  ['inspect', '查看相关内容'],
  ['use_skill', '使用任务技能'],
  ['search', '检索相关资料'],
  ['browse', '访问相关页面'],
  ['execute', '运行相关操作'],
  ['verify', '校验执行结果'],
  ['create', '创建相关内容'],
  ['write', '写入相关内容'],
  ['edit', '更新相关内容'],
  ['generate', '生成相关内容'],
  ['upload', '上传相关文件'],
  ['download', '下载相关文件'],
  ['process', '处理相关内容'],
]);

const journalActionTitle = (event: WorkbenchJournalEvent): string => {
  const data = journalEventData(event);
  const explicit = asText(data.title);
  if (explicit) {
    return explicit;
  }
  const controlled = journalActionTitles.get(asText(data.operation));
  if (controlled) {
    return controlled;
  }
  const target = asText(data.target);
  return target || '执行操作';
};

export const journalFailureDetails = (
  event: WorkbenchJournalEvent,
): JournalFailureDetails | undefined => {
  const status = asStatus(event.status);
  if (status !== 'failed' && status !== 'timed_out') {
    return undefined;
  }
  const data = journalEventData(event);
  const ledgerStatus = asText(data.ledger_status).toLowerCase();
  const traceID = asText(event.trace_id);
  return {
    summary:
      asText(data.error_summary) ||
      asText(data.safe_error_summary) ||
      asText(data.failure_summary) ||
      (status === 'timed_out' ? '执行超时' : '执行失败'),
    retryable: data.retryable === true,
    ...(ledgerStatus ? { ledgerStatus } : {}),
    ...(traceID ? { traceID } : {}),
  };
};

export const journalEventLabel = (event: WorkbenchJournalEvent): string => {
  const data = journalEventData(event);
  if (event.event_type.startsWith('milestone.')) {
    return asText(data.title) || '执行步骤';
  }
  if (event.event_type.startsWith('action.')) {
    return journalActionLabel(event);
  }
  if (event.event_type.startsWith('verification.')) {
    return asText(data.title) || '验证结果';
  }
  if (event.event_type.startsWith('confirmation.')) {
    return asText(data.prompt) || '等待确认';
  }
  if (event.event_type.startsWith('artifact.')) {
    return asText(data.title) || '生成产物';
  }
  return '';
};

export const journalActionKind = (
  event: WorkbenchJournalEvent,
): JournalActionKind => {
  const data = journalEventData(event);
  if (event.event_type.startsWith('artifact.')) {
    return 'artifact';
  }
  if (event.event_type.startsWith('verification.')) {
    return 'verification';
  }
  if (event.event_type.startsWith('confirmation.')) {
    return 'confirmation';
  }
  return contentTypeFromData(event, data) ?? 'generic';
};

const actionIdentity = (event: WorkbenchJournalEvent): string => {
  const data = journalEventData(event);
  return (
    asText(data.action_id) ||
    asText(data.verification_id) ||
    asText(data.confirmation_id) ||
    asText(data.artifact_id) ||
    event.event_id
  );
};

const milestoneIdentity = (event: WorkbenchJournalEvent): string =>
  asText(journalEventData(event).milestone_id);

const isActionLike = (event: WorkbenchJournalEvent): boolean =>
  ['action.', 'artifact.', 'verification.', 'confirmation.'].some(prefix =>
    event.event_type.startsWith(prefix),
  );

const actionItem = (event: WorkbenchJournalEvent): JournalActionItem => {
  const data = journalEventData(event);
  const runtimeAction = event.event_type.startsWith('action.');
  const semanticLabel = journalEventLabel(event);
  return {
    id: actionIdentity(event),
    ...(asText(data.milestone_id)
      ? { milestone_id: asText(data.milestone_id) }
      : {}),
    title: runtimeAction
      ? journalActionTitle(event)
      : semanticLabel || journalActionTitle(event),
    detail: runtimeAction
      ? journalActionLabel(event)
      : semanticLabel || journalActionLabel(event),
    kind: journalActionKind(event),
    status: asStatus(event.status),
    event,
  };
};

const genericActionTargets = new Set([
  '内容',
  '文件',
  '文档',
  '命令',
  '网页',
  '相关资料',
  '执行结果',
  '技能',
]);

const mergeActionItem = (
  current: JournalActionItem,
  candidate: JournalActionItem,
): JournalActionItem => {
  const latest =
    laterEvent(current.event, candidate.event) === candidate.event
      ? candidate
      : current;
  const earlier = latest === candidate ? current : candidate;
  const latestData = journalEventData(latest.event);
  const latestTarget = asText(latestData.target);
  const earlierTarget = asText(journalEventData(earlier.event).target);
  const merged = {
    ...latest,
    ...(!latest.milestone_id && earlier.milestone_id
      ? { milestone_id: earlier.milestone_id }
      : {}),
  };
  if (
    !genericActionTargets.has(latestTarget) ||
    !earlierTarget ||
    genericActionTargets.has(earlierTarget)
  ) {
    return merged;
  }
  const verb = journalActionVerb(latest.status, latestData);
  return {
    ...merged,
    title: earlier.title,
    detail: normalizeVerb(verb, earlierTarget) || earlier.detail,
  };
};

const timelineSemanticKind = (
  event: WorkbenchJournalEvent,
): JournalTimelineSemanticKind => {
  if (event.event_type.startsWith('artifact.')) {
    return 'artifact';
  }
  if (event.event_type.startsWith('verification.')) {
    return 'verification';
  }
  if (event.event_type.startsWith('confirmation.')) {
    return 'confirmation';
  }
  return 'action';
};

const timelineActionItem = (item: JournalActionItem): JournalTimelineItem => ({
  ...item,
  title: item.detail,
  semanticKind: timelineSemanticKind(item.event),
  aggregateEligible:
    item.event.event_type.startsWith('action.') && item.status === 'completed',
});

const timelineMilestoneItem = (
  milestone: JournalMilestoneItem,
): JournalTimelineItem => ({
  id: `milestone:${milestone.id}`,
  title: milestone.title,
  detail: milestone.title,
  kind: 'generic',
  status: milestone.status,
  event: milestone.event,
  semanticKind: 'milestone',
  aggregateEligible: false,
});

export const buildJournalMilestones = (
  events: WorkbenchJournalEvent[],
): JournalMilestoneItem[] => {
  const ordered = [...events].sort((left, right) =>
    eventOrder(left) === eventOrder(right)
      ? left.event_id.localeCompare(right.event_id)
      : eventOrder(left) - eventOrder(right),
  );
  const milestones = new Map<string, JournalMilestoneItem>();
  const actions = new Map<string, JournalActionItem>();

  ordered.forEach(event => {
    if (event.event_type.startsWith('milestone.')) {
      const id = milestoneIdentity(event);
      const title = journalEventLabel(event);
      if (!id || !title) {
        return;
      }
      const current = milestones.get(id);
      milestones.set(id, {
        id,
        title,
        status: asStatus(event.status),
        event: current ? laterEvent(current.event, event) : event,
        actions: current?.actions ?? [],
        atomic: false,
        order: current?.order ?? eventOrder(event),
      });
      return;
    }
    if (!isActionLike(event)) {
      return;
    }
    const next = actionItem(event);
    const current = actions.get(next.id);
    actions.set(next.id, current ? mergeActionItem(current, next) : next);
  });

  actions.forEach(item => {
    if (item.milestone_id && milestones.has(item.milestone_id)) {
      milestones.get(item.milestone_id)?.actions.push(item);
      return;
    }
    milestones.set(item.id, {
      id: item.id,
      title: item.detail,
      status: item.status,
      event: item.event,
      actions: [],
      atomic: true,
      order: eventOrder(item.event),
    });
  });

  return [...milestones.values()]
    .map(item => ({
      ...item,
      atomic: item.actions.length === 0,
      actions: [...item.actions].sort(
        (left, right) => eventOrder(left.event) - eventOrder(right.event),
      ),
    }))
    .sort((left, right) => left.order - right.order);
};

export const journalExecutionIntro = (
  events: WorkbenchJournalEvent[],
): string => {
  const ordered = [...events].sort((left, right) =>
    eventOrder(left) === eventOrder(right)
      ? left.event_id.localeCompare(right.event_id)
      : eventOrder(left) - eventOrder(right),
  );
  const explicit = ordered.find(
    event =>
      event.event_type === 'journal.intro' &&
      asText(journalEventData(event).text),
  );
  if (explicit) {
    return asText(journalEventData(explicit).text);
  }
  for (const event of ordered) {
    if (!event.event_type.startsWith('milestone.')) {
      continue;
    }
    const intro = asText(journalEventData(event).execution_intro);
    if (intro) {
      return intro;
    }
  }
  return '';
};

export const buildJournalTimelineItems = (
  events: WorkbenchJournalEvent[],
): JournalTimelineItem[] =>
  buildJournalMilestones(events).flatMap(milestone =>
    milestone.atomic
      ? [
          milestone.event.event_type.startsWith('milestone.')
            ? timelineMilestoneItem(milestone)
            : timelineActionItem(actionItem(milestone.event)),
        ]
      : [
          timelineMilestoneItem(milestone),
          ...milestone.actions.map(timelineActionItem),
        ],
  );

export const journalContentTypeForEvent = (
  event?: WorkbenchJournalEvent,
): WorkbenchJournalContentType | undefined =>
  event ? contentTypeFromData(event, journalEventData(event)) : undefined;

export const findJournalEvent = (
  events: WorkbenchJournalEvent[],
  eventID?: string,
): WorkbenchJournalEvent | undefined => {
  if (!eventID) {
    return undefined;
  }
  const timelineItem = buildJournalTimelineItems(events).find(
    item => item.event.event_id === eventID || item.id === eventID,
  );
  return (
    timelineItem?.event ?? events.find(event => event.event_id === eventID)
  );
};

export const journalActionData = (
  event: WorkbenchJournalEvent,
): WorkbenchJournalActionEventData =>
  journalEventData(event) as unknown as WorkbenchJournalActionEventData;
