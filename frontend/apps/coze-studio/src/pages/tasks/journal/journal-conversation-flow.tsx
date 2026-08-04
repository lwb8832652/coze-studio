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

/* eslint-disable max-lines -- The flow keeps the accepted short and virtualized render paths together. */

import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from 'react';

import { AutoSizer, VariableSizeList } from '@coze-common/virtual-list';
import {
  IconCozArrowDown,
  IconCozArrowUp,
  IconCozCheckMark,
  IconCozCode,
  IconCozDocument,
  IconCozEarth,
  IconCozImage,
  IconCozLightbulb,
  IconCozLoading,
  IconCozShell,
  IconCozVerifyFailed,
} from '@coze-arch/coze-design/icons';

import type {
  WorkbenchJournalEvent,
  WorkbenchJournalRecoveryCapability,
} from '../../workbench/thread-client';
import type { JournalViewMode } from './journal-reducer';
import {
  JournalRecoveryDialog,
  journalRecoveryAction,
  recoveryRequiresDialog,
  type JournalRecoveryHandler,
} from './journal-recovery-dialog';
import { JournalFailureDetail } from './journal-failure-detail';
import {
  buildJournalMilestones,
  journalActionKind,
  journalExecutionIntro,
  journalFailureDetails,
  type JournalActionItem,
  type JournalMilestoneItem,
} from './journal-event-model';

const virtualRowThreshold = 80;
const milestoneHeaderHeight = 36;
const actionRowHeight = 64;
const failedActionRowHeight = 94;
const failedAtomicMilestoneHeight = 84;
const maximumVirtualListHeight = 560;

const statusIcon = (status: JournalMilestoneItem['status']): ReactNode => {
  if (status === 'completed') {
    return <IconCozCheckMark />;
  }
  if (status === 'failed' || status === 'timed_out') {
    return <IconCozVerifyFailed />;
  }
  if (status === 'running') {
    return <IconCozLoading />;
  }
  return null;
};

const actionIcon = (action: JournalActionItem): ReactNode => {
  switch (action.kind) {
    case 'document':
      return <IconCozDocument />;
    case 'terminal':
      return <IconCozShell />;
    case 'code':
      return <IconCozCode />;
    case 'skill':
      return <IconCozLightbulb />;
    case 'browser':
      return <IconCozEarth />;
    case 'artifact':
      return <IconCozImage />;
    case 'verification':
      return <IconCozCheckMark />;
    case 'confirmation':
    case 'generic':
    default:
      return <IconCozShell />;
  }
};

const selectedMilestoneID = (
  milestones: JournalMilestoneItem[],
  selectedEventID?: string,
): string | undefined =>
  milestones.find(
    milestone =>
      milestone.event.event_id === selectedEventID ||
      milestone.actions.some(
        action => action.event.event_id === selectedEventID,
      ),
  )?.id;

const defaultExpandedIDs = (milestones: JournalMilestoneItem[]): Set<string> =>
  new Set(
    milestones
      .filter(milestone => !milestone.atomic && milestone.status === 'running')
      .map(milestone => milestone.id),
  );

const atomicMilestoneAction = (
  milestone: JournalMilestoneItem,
): JournalActionItem => ({
  id: milestone.id,
  title: milestone.title,
  detail: milestone.title,
  kind: journalActionKind(milestone.event),
  status: milestone.status,
  event: milestone.event,
});

const JournalActionRow = ({
  action,
  recoveryAvailable,
  selected,
  onRequestRecovery,
  onSelectEvent,
}: {
  action: JournalActionItem;
  recoveryAvailable: boolean;
  selected: boolean;
  onRequestRecovery: (action: JournalActionItem) => void;
  onSelectEvent: (event: WorkbenchJournalEvent) => void;
}) => (
  <div
    className="journal-action-row"
    data-action-kind={action.kind}
    data-status={action.status}
    data-selected={selected}
  >
    <button
      type="button"
      aria-current={selected ? 'step' : undefined}
      aria-label={action.detail}
      className="journal-action-main"
      onClick={() => onSelectEvent(action.event)}
    >
      <span className="journal-action-title">{action.title}</span>
      <span className="journal-action-detail" data-status={action.status}>
        <span className="journal-action-icon" aria-hidden="true">
          {actionIcon(action)}
        </span>
        <span className="journal-action-detail-copy">{action.detail}</span>
      </span>
    </button>
    <JournalFailureDetail
      action={action}
      recoveryAvailable={recoveryAvailable}
      onRequestRecovery={onRequestRecovery}
    />
  </div>
);

const JournalMilestone = ({
  expanded,
  milestone,
  canRecover,
  renderActions = true,
  selectedEventId,
  onSelectEvent,
  onRequestRecovery,
  onToggle,
}: {
  expanded: boolean;
  milestone: JournalMilestoneItem;
  canRecover: (action: JournalActionItem) => boolean;
  renderActions?: boolean;
  selectedEventId?: string;
  onSelectEvent: (event: WorkbenchJournalEvent) => void;
  onRequestRecovery: (action: JournalActionItem) => void;
  onToggle: () => void;
}) => {
  const atomicAction = milestone.atomic
    ? atomicMilestoneAction(milestone)
    : undefined;
  const selected =
    milestone.event.event_id === selectedEventId ||
    milestone.actions.some(action => action.event.event_id === selectedEventId);
  const handleHeader = () => {
    if (milestone.atomic) {
      onSelectEvent(milestone.event);
      return;
    }
    onToggle();
  };

  return (
    <section
      className="journal-milestone"
      data-expandable={!milestone.atomic}
      data-expanded={expanded}
      data-milestone-id={milestone.id}
      data-selected={selected}
      data-status={milestone.status}
    >
      <button
        type="button"
        aria-expanded={milestone.atomic ? undefined : expanded}
        aria-label={milestone.title}
        className="journal-milestone-header"
        onClick={handleHeader}
      >
        <span className="journal-state-icon" aria-hidden="true">
          {statusIcon(milestone.status)}
        </span>
        <span className="journal-milestone-title">{milestone.title}</span>
        {!milestone.atomic ? (
          <span
            className="journal-milestone-chevron"
            data-testid="journal-chevron"
            aria-hidden="true"
          >
            {expanded ? <IconCozArrowUp /> : <IconCozArrowDown />}
          </span>
        ) : null}
      </button>
      {atomicAction && journalFailureDetails(milestone.event) ? (
        <div className="journal-atomic-failure">
          <JournalFailureDetail
            action={atomicAction}
            recoveryAvailable={canRecover(atomicAction)}
            onRequestRecovery={onRequestRecovery}
          />
        </div>
      ) : null}
      {renderActions && !milestone.atomic && expanded ? (
        <div className="journal-action-list">
          {milestone.actions.map(action => (
            <JournalActionRow
              action={action}
              key={action.id}
              recoveryAvailable={canRecover(action)}
              selected={action.event.event_id === selectedEventId}
              onRequestRecovery={onRequestRecovery}
              onSelectEvent={onSelectEvent}
            />
          ))}
        </div>
      ) : null}
    </section>
  );
};

type JournalVirtualRow =
  | {
      key: string;
      type: 'milestone';
      milestone: JournalMilestoneItem;
    }
  | {
      key: string;
      type: 'action';
      action: JournalActionItem;
    };

const buildVirtualRows = (
  milestones: JournalMilestoneItem[],
  expandedIDs: Set<string>,
): JournalVirtualRow[] =>
  milestones.flatMap(milestone => [
    {
      key: `milestone:${milestone.id}`,
      type: 'milestone' as const,
      milestone,
    },
    ...(!milestone.atomic && expandedIDs.has(milestone.id)
      ? milestone.actions.map(action => ({
          key: `action:${action.id}`,
          type: 'action' as const,
          action,
        }))
      : []),
  ]);

const virtualMilestoneHeight = (milestone: JournalMilestoneItem): number =>
  milestone.atomic && journalFailureDetails(milestone.event)
    ? failedAtomicMilestoneHeight
    : milestoneHeaderHeight;

interface JournalMilestoneListProps {
  canRecover: (action: JournalActionItem) => boolean;
  expandedIDs: Set<string>;
  milestones: JournalMilestoneItem[];
  selectedEventId?: string;
  onSelectEvent: (event: WorkbenchJournalEvent) => void;
  onRequestRecovery: (action: JournalActionItem) => void;
  onToggle: (milestoneID: string) => void;
}

const JournalMilestoneList = ({
  canRecover,
  expandedIDs,
  milestones,
  selectedEventId,
  onSelectEvent,
  onRequestRecovery,
  onToggle,
}: JournalMilestoneListProps) => (
  <>
    {milestones.map(milestone => (
      <JournalMilestone
        expanded={expandedIDs.has(milestone.id)}
        canRecover={canRecover}
        key={milestone.id}
        milestone={milestone}
        selectedEventId={selectedEventId}
        onSelectEvent={onSelectEvent}
        onRequestRecovery={onRequestRecovery}
        onToggle={() => onToggle(milestone.id)}
      />
    ))}
  </>
);

const JournalVirtualizedMilestoneList = ({
  canRecover,
  expandedIDs,
  milestones,
  selectedEventId,
  onSelectEvent,
  onRequestRecovery,
  onToggle,
}: JournalMilestoneListProps) => {
  const listRef = useRef<VariableSizeList>(null);
  const rows = useMemo(
    () => buildVirtualRows(milestones, expandedIDs),
    [expandedIDs, milestones],
  );
  const height = Math.min(
    maximumVirtualListHeight,
    rows.reduce(
      (sum, row) =>
        sum +
        (row.type === 'milestone'
          ? virtualMilestoneHeight(row.milestone)
          : journalFailureDetails(row.action.event)
            ? failedActionRowHeight
            : actionRowHeight),
      0,
    ),
  );

  useEffect(() => {
    listRef.current?.resetAfterIndex(0, true);
  }, [rows]);

  return (
    <div
      className="journal-conversation-virtualized"
      data-testid="journal-flow-virtualized"
      style={{ height }}
    >
      <AutoSizer>
        {({ height: viewportHeight, width }) => (
          <VariableSizeList
            height={viewportHeight ?? 0}
            itemCount={rows.length}
            itemKey={index => rows[index].key}
            itemSize={index =>
              rows[index].type === 'milestone'
                ? virtualMilestoneHeight(rows[index].milestone)
                : journalFailureDetails(rows[index].action.event)
                  ? failedActionRowHeight
                  : actionRowHeight
            }
            overscanCount={4}
            ref={listRef}
            width={width ?? 0}
          >
            {({ index, style }: { index: number; style: CSSProperties }) => {
              const row = rows[index];
              if (row.type === 'action') {
                return (
                  <div className="journal-virtual-action-row" style={style}>
                    <JournalActionRow
                      action={row.action}
                      recoveryAvailable={canRecover(row.action)}
                      selected={row.action.event.event_id === selectedEventId}
                      onRequestRecovery={onRequestRecovery}
                      onSelectEvent={onSelectEvent}
                    />
                  </div>
                );
              }
              const { milestone } = row;
              return (
                <div className="journal-virtual-milestone" style={style}>
                  <JournalMilestone
                    expanded={expandedIDs.has(milestone.id)}
                    canRecover={canRecover}
                    milestone={milestone}
                    renderActions={false}
                    selectedEventId={selectedEventId}
                    onSelectEvent={onSelectEvent}
                    onRequestRecovery={onRequestRecovery}
                    onToggle={() => onToggle(milestone.id)}
                  />
                </div>
              );
            }}
          </VariableSizeList>
        )}
      </AutoSizer>
    </div>
  );
};

export const JournalConversationFlow = ({
  events,
  recoveryCapability,
  selectedEventId,
  viewMode = 'live_follow',
  onRecover,
  onSelectEvent,
}: {
  events: WorkbenchJournalEvent[];
  recoveryCapability?: WorkbenchJournalRecoveryCapability;
  selectedEventId?: string;
  viewMode?: JournalViewMode;
  onRecover?: JournalRecoveryHandler;
  onSelectEvent: (event: WorkbenchJournalEvent) => void;
}) => {
  const milestones = useMemo(() => buildJournalMilestones(events), [events]);
  const executionIntro = useMemo(() => journalExecutionIntro(events), [events]);
  const [expandedIDs, setExpandedIDs] = useState<Set<string>>(() =>
    defaultExpandedIDs(milestones),
  );
  const [recoveryTarget, setRecoveryTarget] = useState<JournalActionItem>();
  const [recoveryBusy, setRecoveryBusy] = useState(false);
  const [recoveryError, setRecoveryError] = useState('');
  const activeMilestoneID = selectedMilestoneID(milestones, selectedEventId);

  useEffect(() => {
    const runningIDs = defaultExpandedIDs(milestones);
    if (activeMilestoneID) {
      const active = milestones.find(item => item.id === activeMilestoneID);
      if (active && !active.atomic) {
        runningIDs.add(activeMilestoneID);
      }
    }
    setExpandedIDs(current => new Set([...current, ...runningIDs]));
  }, [activeMilestoneID, milestones]);

  if (!milestones.length && !executionIntro) {
    return null;
  }

  const toggleMilestone = (milestoneID: string) =>
    setExpandedIDs(current => {
      const next = new Set(current);
      if (next.has(milestoneID)) {
        next.delete(milestoneID);
      } else {
        next.add(milestoneID);
      }
      return next;
    });
  const shouldVirtualize =
    buildVirtualRows(milestones, expandedIDs).length > virtualRowThreshold;
  const canRecover = (action: JournalActionItem) =>
    Boolean(onRecover && journalRecoveryAction(action, recoveryCapability));
  const executeRecovery = async (
    action: JournalActionItem,
    recoveryAction: string,
    confirmed: boolean,
  ) => {
    if (!onRecover) {
      return;
    }
    setRecoveryBusy(true);
    setRecoveryError('');
    try {
      await onRecover(recoveryAction, confirmed);
      setRecoveryTarget(undefined);
    } catch (error) {
      void error;
      setRecoveryTarget(action);
      setRecoveryError('恢复请求未成功，请稍后重试。');
    } finally {
      setRecoveryBusy(false);
    }
  };
  const requestRecovery = (action: JournalActionItem) => {
    if (!recoveryCapability) {
      return;
    }
    const recoveryAction = journalRecoveryAction(action, recoveryCapability);
    if (!recoveryAction) {
      return;
    }
    if (recoveryRequiresDialog(action, recoveryCapability, viewMode)) {
      setRecoveryError('');
      setRecoveryTarget(action);
      return;
    }
    void executeRecovery(action, recoveryAction, false);
  };

  return (
    <div className="journal-conversation-flow" data-testid="journal-flow">
      {executionIntro ? (
        <p className="journal-execution-intro">{executionIntro}</p>
      ) : null}
      {milestones.length ? (
        shouldVirtualize ? (
          <JournalVirtualizedMilestoneList
            canRecover={canRecover}
            expandedIDs={expandedIDs}
            milestones={milestones}
            selectedEventId={selectedEventId}
            onSelectEvent={onSelectEvent}
            onRequestRecovery={requestRecovery}
            onToggle={toggleMilestone}
          />
        ) : (
          <JournalMilestoneList
            canRecover={canRecover}
            expandedIDs={expandedIDs}
            milestones={milestones}
            selectedEventId={selectedEventId}
            onSelectEvent={onSelectEvent}
            onRequestRecovery={requestRecovery}
            onToggle={toggleMilestone}
          />
        )
      ) : null}
      {recoveryTarget && recoveryCapability && onRecover ? (
        <JournalRecoveryDialog
          busy={recoveryBusy}
          capability={recoveryCapability}
          error={recoveryError}
          item={recoveryTarget}
          onCancel={() => setRecoveryTarget(undefined)}
          onRecover={(action, confirmed) =>
            executeRecovery(recoveryTarget, action, confirmed)
          }
        />
      ) : null}
    </div>
  );
};
