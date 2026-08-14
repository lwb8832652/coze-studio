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

import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type MutableRefObject,
} from 'react';

import { AutoSizer, VariableSizeList } from '@coze-common/virtual-list';
import {
  IconCozArrowLeft,
  IconCozArrowRight,
  IconCozCode,
  IconCozDocument,
  IconCozEarth,
  IconCozImage,
  IconCozLightbulb,
  IconCozShell,
} from '@coze-arch/coze-design/icons';

import type {
  WorkbenchJournalAttempt,
  WorkbenchJournalEvent,
  WorkbenchJournalExecutionStatus,
} from '../../workbench/thread-client';
import type {
  JournalTransportStatus,
  JournalViewMode,
} from './journal-reducer';
import {
  buildJournalTimelineItems,
  type JournalActionKind,
  type JournalTimelineItem,
  type JournalTimelineSemanticKind,
} from './journal-event-model';

const virtualTimelineThreshold = 80;
const targetTimelinePointCount = 72;
const timelineNodeSlotWidth = 28;
const timelineWindowHeight = 18;
const minimumAggregateSize = 2;
const percentScale = 100;
const liveAnnouncementDelay = 400;

const attemptStatusLabel: Record<WorkbenchJournalExecutionStatus, string> = {
  pending: '等待中',
  running: '进行中',
  completed: '已完成',
  failed: '已失败',
  cancelled: '已取消',
  timed_out: '已超时',
  interrupted: '已中断',
};

interface JournalTimelineDisplayItem {
  id: string;
  title: string;
  kind: JournalActionKind;
  status: WorkbenchJournalExecutionStatus;
  event: WorkbenchJournalEvent;
  semanticKind: JournalTimelineSemanticKind;
  items: JournalTimelineItem[];
}

const timelineIcon = (item: JournalTimelineDisplayItem) => {
  switch (item.kind) {
    case 'document':
      return <IconCozDocument />;
    case 'code':
      return <IconCozCode />;
    case 'skill':
      return <IconCozLightbulb />;
    case 'browser':
      return <IconCozEarth />;
    case 'artifact':
      return <IconCozImage />;
    case 'terminal':
    case 'generic':
    case 'verification':
    case 'confirmation':
    default:
      return <IconCozShell />;
  }
};

const formatExactTime = (timestamp: number): string =>
  new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(timestamp));

const eventTime = (event: WorkbenchJournalEvent): number =>
  event.occurred_at ?? event.created_at;

const displayItem = (
  item: JournalTimelineItem,
): JournalTimelineDisplayItem => ({
  id: item.id,
  title: item.title,
  kind: item.kind,
  status: item.status,
  event: item.event,
  semanticKind: item.semanticKind,
  items: [item],
});

const aggregatedDisplayItem = (
  items: JournalTimelineItem[],
): JournalTimelineDisplayItem => {
  const first = items[0];
  const last = items[items.length - 1];
  return {
    id: `aggregate:${first.id}:${last.id}`,
    title: `${first.title} 至 ${last.title}（${items.length} 个操作）`,
    kind: last.kind,
    status: last.status,
    event: last.event,
    semanticKind: 'action',
    items,
  };
};

const aggregateTimelineItems = (
  items: JournalTimelineItem[],
): JournalTimelineDisplayItem[] => {
  if (items.length <= virtualTimelineThreshold) {
    return items.map(displayItem);
  }

  const chunkSize = Math.max(
    minimumAggregateSize,
    Math.ceil(items.length / targetTimelinePointCount),
  );
  const result: JournalTimelineDisplayItem[] = [];
  let actionRun: JournalTimelineItem[] = [];
  const flushActionRun = () => {
    for (let index = 0; index < actionRun.length; index += chunkSize) {
      const chunk = actionRun.slice(index, index + chunkSize);
      result.push(
        chunk.length === 1
          ? displayItem(chunk[0])
          : aggregatedDisplayItem(chunk),
      );
    }
    actionRun = [];
  };

  items.forEach(item => {
    if (item.aggregateEligible) {
      actionRun.push(item);
      return;
    }
    flushActionRun();
    result.push(displayItem(item));
  });
  flushActionRun();
  return result;
};

const selectedIndex = (
  items: JournalTimelineDisplayItem[],
  eventID?: string,
): number =>
  Math.max(
    0,
    items.findIndex(item =>
      item.items.some(candidate => candidate.event.event_id === eventID),
    ),
  );

const timelineTooltip = (item: JournalTimelineDisplayItem): string => {
  const first = item.items[0];
  const last = item.items[item.items.length - 1];
  const firstTime = formatExactTime(eventTime(first.event));
  const lastTime = formatExactTime(eventTime(last.event));
  return first === last
    ? `${item.title} · ${firstTime}`
    : `${item.title} · ${firstTime}–${lastTime}`;
};

interface TimelineNodeProps {
  index: number;
  item: JournalTimelineDisplayItem;
  selectedEventId?: string;
  buttonRefs: MutableRefObject<Array<HTMLButtonElement | null>>;
  onSelect: (index: number) => void;
  onKeyDown: (event: KeyboardEvent<HTMLButtonElement>, index: number) => void;
  onTooltipChange: (tooltip: string) => void;
}

const TimelineNode = ({
  index,
  item,
  selectedEventId,
  buttonRefs,
  onSelect,
  onKeyDown,
  onTooltipChange,
}: TimelineNodeProps) => {
  const selected = item.items.some(
    candidate => candidate.event.event_id === selectedEventId,
  );
  const tooltip = timelineTooltip(item);
  return (
    <button
      type="button"
      aria-label={item.title}
      aria-current={selected ? 'step' : undefined}
      className="journal-timeline-node"
      data-aggregated={item.items.length > 1}
      data-selected={selected}
      data-semantic-kind={item.semanticKind}
      data-status={item.status}
      ref={element => {
        buttonRefs.current[index] = element;
      }}
      tabIndex={selected || (!selectedEventId && index === 0) ? 0 : -1}
      title={tooltip}
      onBlur={() => onTooltipChange('')}
      onClick={() => onSelect(index)}
      onFocus={() => onTooltipChange(tooltip)}
      onKeyDown={event => onKeyDown(event, index)}
      onMouseEnter={() => onTooltipChange(tooltip)}
      onMouseLeave={event => {
        if (document.activeElement !== event.currentTarget) {
          onTooltipChange('');
        }
      }}
    >
      <span aria-hidden="true">{timelineIcon(item)}</span>
    </button>
  );
};

const TimelineHeader = ({
  activeIndex,
  attempts,
  eventCount,
  itemCount,
  live,
  selectedAttemptId,
  onSelectAttempt,
  onSelect,
  onViewModeChange,
}: {
  activeIndex: number;
  attempts: WorkbenchJournalAttempt[];
  eventCount: number;
  itemCount: number;
  live: boolean;
  selectedAttemptId?: string;
  onSelectAttempt?: (attemptID: string) => void;
  onSelect: (index: number) => void;
  onViewModeChange: (mode: JournalViewMode) => void;
}) => (
  <div className="journal-timeline-header">
    <div className="journal-timeline-controls">
      <button
        type="button"
        aria-label="上一个执行事件"
        disabled={activeIndex <= 0}
        onClick={() => onSelect(activeIndex - 1)}
      >
        <IconCozArrowLeft />
      </button>
      <button
        type="button"
        aria-label="下一个执行事件"
        disabled={activeIndex >= itemCount - 1}
        onClick={() => onSelect(activeIndex + 1)}
      >
        <IconCozArrowRight />
      </button>
      <button
        type="button"
        aria-pressed={live}
        className="journal-live-button"
        data-live={live}
        onClick={() => {
          onSelect(itemCount - 1);
          onViewModeChange('live_follow');
        }}
      >
        <span aria-hidden="true" />
        Live
      </button>
      {attempts.length > 1 && onSelectAttempt ? (
        <select
          aria-label="选择执行尝试"
          className="journal-attempt-selector"
          title="选择执行尝试"
          value={selectedAttemptId}
          onChange={event => onSelectAttempt(event.target.value)}
        >
          {[...attempts]
            .sort((left, right) => left.created_at - right.created_at)
            .map((attempt, index) => (
              <option key={attempt.attempt_id} value={attempt.attempt_id}>
                {`第 ${index + 1} 次 · ${attemptStatusLabel[attempt.status]}`}
              </option>
            ))}
        </select>
      ) : null}
    </div>
    <div className="journal-timeline-summary">
      执行时间线
      <span>{eventCount} 个事件</span>
    </div>
  </div>
);

// eslint-disable-next-line @coze-arch/max-line-per-function -- Timeline selection, focus, virtualization, and live announcements share one interaction state.
export const JournalTimeline = ({
  attempts = [],
  events,
  selectedAttemptId,
  selectedEventId,
  transportStatus,
  viewMode,
  onSelectEvent,
  onSelectAttempt,
  onViewModeChange,
}: {
  attempts?: WorkbenchJournalAttempt[];
  events: WorkbenchJournalEvent[];
  selectedAttemptId?: string;
  selectedEventId?: string;
  transportStatus: JournalTransportStatus;
  viewMode: JournalViewMode;
  onSelectEvent: (event: WorkbenchJournalEvent) => void;
  onSelectAttempt?: (attemptID: string) => void;
  onViewModeChange: (mode: JournalViewMode) => void;
}) => {
  const items = useMemo(() => buildJournalTimelineItems(events), [events]);
  const displayItems = useMemo(() => aggregateTimelineItems(items), [items]);
  const buttonRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const listRef = useRef<VariableSizeList>(null);
  const [tooltip, setTooltip] = useState('');
  const [announcement, setAnnouncement] = useState('');
  const activeIndex = selectedIndex(displayItems, selectedEventId);
  const shouldVirtualize = items.length > virtualTimelineThreshold;
  const latestItem = items[items.length - 1];

  useEffect(() => {
    if (shouldVirtualize) {
      listRef.current?.scrollToItem(activeIndex, 'smart');
    }
  }, [activeIndex, shouldVirtualize]);

  useEffect(() => {
    if (viewMode !== 'live_follow' || !latestItem) {
      setAnnouncement('');
      return;
    }
    const timer = setTimeout(
      () =>
        setAnnouncement(
          `${latestItem.title}，${attemptStatusLabel[latestItem.status]}`,
        ),
      liveAnnouncementDelay,
    );
    return () => clearTimeout(timer);
  }, [latestItem, viewMode]);

  const focusAt = (index: number) => {
    if (shouldVirtualize) {
      listRef.current?.scrollToItem(index, 'smart');
      requestAnimationFrame(() => buttonRefs.current[index]?.focus());
      return;
    }
    buttonRefs.current[index]?.focus();
  };
  const selectAt = (index: number) => {
    const item = displayItems[index];
    if (!item) {
      return;
    }
    const target = item.items[item.items.length - 1];
    onSelectEvent(target.event);
    onViewModeChange(
      target === items[items.length - 1] ? 'live_follow' : 'historical',
    );
    focusAt(index);
  };
  const handleKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) => {
    const direction = event.key === 'ArrowRight' ? 1 : -1;
    if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') {
      return;
    }
    event.preventDefault();
    selectAt((index + direction + displayItems.length) % displayItems.length);
  };
  const live = viewMode === 'live_follow';

  if (!items.length) {
    return null;
  }

  const progress = `${
    ((activeIndex + 1) / displayItems.length) * percentScale
  }%`;
  const renderNode = (item: JournalTimelineDisplayItem, index: number) => (
    <TimelineNode
      buttonRefs={buttonRefs}
      index={index}
      item={item}
      key={item.id}
      selectedEventId={selectedEventId}
      onKeyDown={handleKeyDown}
      onSelect={selectAt}
      onTooltipChange={setTooltip}
    />
  );

  return (
    <footer
      className="journal-timeline"
      data-transport-status={transportStatus}
      aria-label="执行时间线"
    >
      <TimelineHeader
        activeIndex={activeIndex}
        attempts={attempts}
        eventCount={items.length}
        itemCount={displayItems.length}
        live={live}
        selectedAttemptId={selectedAttemptId}
        onSelectAttempt={onSelectAttempt}
        onSelect={selectAt}
        onViewModeChange={onViewModeChange}
      />
      <div
        className="journal-timeline-track"
        data-testid="journal-timeline-track"
        data-virtualized={shouldVirtualize}
      >
        <div className="journal-timeline-line" aria-hidden="true">
          <span style={{ width: progress }} />
        </div>
        {shouldVirtualize ? (
          <div
            className="journal-timeline-window"
            data-testid="journal-timeline-virtualized"
          >
            <AutoSizer>
              {({ width }) => (
                <VariableSizeList
                  height={timelineWindowHeight}
                  itemCount={displayItems.length}
                  itemKey={index => displayItems[index].id}
                  itemSize={() => timelineNodeSlotWidth}
                  layout="horizontal"
                  overscanCount={6}
                  ref={listRef}
                  width={width ?? 0}
                >
                  {({
                    index,
                    style,
                  }: {
                    index: number;
                    style: CSSProperties;
                  }) => (
                    <div className="journal-timeline-slot" style={style}>
                      {renderNode(displayItems[index], index)}
                    </div>
                  )}
                </VariableSizeList>
              )}
            </AutoSizer>
          </div>
        ) : (
          displayItems.map(renderNode)
        )}
      </div>
      {tooltip ? (
        <div className="journal-timeline-tooltip" role="tooltip">
          {tooltip}
        </div>
      ) : null}
      <span className="journal-transport-sr" aria-live="polite">
        {announcement}
      </span>
    </footer>
  );
};
