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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import {
  useJournalExperience,
  type JournalExperience,
} from '../use-journal-experience';
import { JournalPanel } from '../journal-panel';
import { JournalConversationFlow } from '../journal-conversation-flow';
import type {
  JournalEventSubscription,
  SubscribeWorkbenchJournalEventsRequest,
  WorkbenchJournalAttempt,
  WorkbenchJournalBootstrap,
  WorkbenchJournalEvent,
  WorkbenchJournalSnapshot,
} from '../../../workbench/thread-client';

const journalContractMocks = vi.hoisted(() => ({
  auditSnapshotAction: vi.fn(),
  getArtifactSignedURL: vi.fn(),
  getRunJournal: vi.fn(),
  getSettings: vi.fn(),
  getSnapshot: vi.fn(),
  listJournalEvents: vi.fn(),
  patchSettings: vi.fn(),
  recoverJournal: vi.fn(),
  subscriptions: [] as SubscribeWorkbenchJournalEventsRequest[],
}));

vi.mock(
  '../../../workbench/thread-client/canonical-thread-client-singleton',
  () => ({
    canonicalThreadClient: {
      auditJournalSnapshotAction: journalContractMocks.auditSnapshotAction,
      getArtifactSignedURL: journalContractMocks.getArtifactSignedURL,
      getJournalSettings: journalContractMocks.getSettings,
      getJournalSnapshot: journalContractMocks.getSnapshot,
      getRunJournal: journalContractMocks.getRunJournal,
      listJournalEvents: journalContractMocks.listJournalEvents,
      patchJournalSettings: journalContractMocks.patchSettings,
      recoverJournal: journalContractMocks.recoverJournal,
      subscribeJournalEvents: (
        request: SubscribeWorkbenchJournalEventsRequest,
      ): JournalEventSubscription => {
        journalContractMocks.subscriptions.push(request);
        return { close: vi.fn(), closed: new Promise(() => undefined) };
      },
    },
  }),
);

vi.mock('../../task-markdown-content', () => ({
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Match the mocked module export.
  TaskMarkdownContent: ({ value }: { value: string }) => (
    <article>{value}</article>
  ),
}));

const scope = {
  run_id: 'run-1',
  space_id: 'space-1',
  thread_id: 'thread-1',
};

const recoveryCapability = {
  allowed: false,
  allowed_actions: [],
  requires_confirmation: false,
};

const liveAttempt: WorkbenchJournalAttempt = {
  attempt_id: 'attempt-live',
  run_id: scope.run_id,
  status: 'running',
  projection_state: 'healthy',
  latest_sequence: 1,
  created_at: 2_000,
  recovery_capability: recoveryCapability,
};

const historicalAttempt: WorkbenchJournalAttempt = {
  attempt_id: 'attempt-history',
  run_id: scope.run_id,
  status: 'failed',
  projection_state: 'healthy',
  latest_sequence: 1,
  created_at: 1_000,
  ended_at: 1_500,
  recovery_capability: recoveryCapability,
};

const milestoneEvent: WorkbenchJournalEvent = {
  event_id: 'event-milestone',
  thread_id: scope.thread_id,
  run_id: scope.run_id,
  attempt_id: liveAttempt.attempt_id,
  event_type: 'milestone.started',
  payload: {
    type: 'milestone',
    data: { milestone_id: 'milestone-1', title: '核验交付链路' },
  },
  created_at: 2_001,
  occurred_at: 2_001,
  sequence: 1,
  status: 'running',
  visibility: 'user',
  payload_version: '1.0',
};

const documentEvent = ({
  attemptId,
  eventId,
  snapshotId,
  target,
}: {
  attemptId: string;
  eventId: string;
  snapshotId: string;
  target: string;
}): WorkbenchJournalEvent => ({
  event_id: eventId,
  thread_id: scope.thread_id,
  run_id: scope.run_id,
  attempt_id: attemptId,
  event_type: 'action.terminal',
  payload: {
    type: 'document',
    data: {
      action_id: `action-${eventId}`,
      ...(attemptId === liveAttempt.attempt_id
        ? { milestone_id: 'milestone-1' }
        : {}),
      operation: 'read',
      target,
      display_verb_running: '正在读取',
      display_verb_completed: '已读取',
      content_type: 'document',
    },
  },
  created_at: 2_002,
  occurred_at: 2_002,
  sequence: attemptId === liveAttempt.attempt_id ? 2 : 1,
  snapshot_id: snapshotId,
  status: 'completed',
  visibility: 'user',
  payload_version: '1.0',
});

const liveDocumentEvent = documentEvent({
  attemptId: liveAttempt.attempt_id,
  eventId: 'event-live-document',
  snapshotId: 'snapshot-live',
  target: '当前验收文档',
});

const historicalDocumentEvent = documentEvent({
  attemptId: historicalAttempt.attempt_id,
  eventId: 'event-history-document',
  snapshotId: 'snapshot-history',
  target: '历史验收文档',
});

const bootstrap = ({
  attempt,
  events,
}: {
  attempt: WorkbenchJournalAttempt;
  events: WorkbenchJournalEvent[];
}): WorkbenchJournalBootstrap => ({
  attempts: [historicalAttempt, liveAttempt],
  default_attempt_id: attempt.attempt_id,
  default_attempt: attempt,
  projection_state: 'healthy',
  latest_sequence: attempt.latest_sequence,
  events: {
    items: events,
    has_more: false,
    attempt_id: attempt.attempt_id,
    latest_sequence: attempt.latest_sequence,
    next_after_sequence: attempt.latest_sequence,
  },
  content_types: ['document', 'terminal', 'code', 'skill', 'browser'],
  enrollment: {
    enrolled: true,
    schema_version: '1.1',
    payload_version: '1.0',
    journal_protocol_version: '1.1',
    journal_enabled: true,
    snapshots_enabled: true,
  },
  submit_at: 1_900,
  server_time: 2_000,
  recovery_capability: attempt.recovery_capability,
});

const snapshot = ({
  attemptId,
  eventId,
  snapshotId,
}: {
  attemptId: string;
  eventId: string;
  snapshotId: string;
}): WorkbenchJournalSnapshot => ({
  attempt_id: attemptId,
  content_type: 'document',
  created_at: 2_100,
  event_id: eventId,
  fragments: [],
  has_more: false,
  is_fragmented: false,
  snapshot_id: snapshotId,
  status: 'ready',
  visibility: 'user',
  content: {
    document: {
      title: snapshotId,
      content: `# ${snapshotId}\n生产交付内容`,
    },
  },
});

let currentExperience: JournalExperience | undefined;

const JournalContractHarness = () => {
  currentExperience = useJournalExperience({
    enabled: true,
    runId: scope.run_id,
    spaceId: scope.space_id,
    threadId: scope.thread_id,
  });
  if (!currentExperience.state.has_displayable_journal_content) {
    return <output data-testid="journal-state">waiting</output>;
  }
  return (
    <main>
      <JournalConversationFlow
        events={currentExperience.state.execution.events}
        selectedEventId={currentExperience.selectedEventId}
        onSelectEvent={currentExperience.selectEvent}
      />
      <JournalPanel
        activeTab={currentExperience.activeTab}
        attempts={currentExperience.state.execution.attempts}
        contentStatus={currentExperience.state.content.status}
        events={currentExperience.state.execution.events}
        selectedAttemptId={
          currentExperience.state.execution.selected_attempt_id
        }
        selectedEventId={currentExperience.selectedEventId}
        snapshot={currentExperience.state.content.snapshot}
        transportStatus={currentExperience.state.transport.status}
        viewMode={currentExperience.state.view_mode}
        onActiveTabChange={currentExperience.setActiveTab}
        onClose={currentExperience.closePanel}
        onSelectAttempt={currentExperience.selectAttempt}
        onSelectEvent={currentExperience.selectEvent}
        onSnapshotAction={currentExperience.performSnapshotAction}
        onViewModeChange={currentExperience.setViewMode}
      />
    </main>
  );
};

const requireExperience = (): JournalExperience => {
  if (!currentExperience) {
    throw new Error('Journal experience is not mounted');
  }
  return currentExperience;
};

const flushReact = async () => {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
};

describe('Journal end-to-end frontend contract', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    currentExperience = undefined;
    localStorage.clear();
    journalContractMocks.subscriptions.length = 0;
    journalContractMocks.getRunJournal.mockReset();
    journalContractMocks.getRunJournal.mockImplementation(
      ({ attempt_id: attemptId }: { attempt_id?: string }) =>
        Promise.resolve(
          attemptId === historicalAttempt.attempt_id
            ? bootstrap({
                attempt: historicalAttempt,
                events: [historicalDocumentEvent],
              })
            : bootstrap({ attempt: liveAttempt, events: [milestoneEvent] }),
        ),
    );
    journalContractMocks.getSettings.mockReset();
    journalContractMocks.getSettings.mockResolvedValue({
      revision: 'settings-1',
      split_ratio: 0.4,
    });
    journalContractMocks.getSnapshot.mockReset();
    journalContractMocks.getSnapshot.mockImplementation(
      ({ snapshot_id: snapshotId }: { snapshot_id: string }) =>
        Promise.resolve(
          snapshotId === 'snapshot-history'
            ? snapshot({
                attemptId: historicalAttempt.attempt_id,
                eventId: historicalDocumentEvent.event_id,
                snapshotId,
              })
            : snapshot({
                attemptId: liveAttempt.attempt_id,
                eventId: liveDocumentEvent.event_id,
                snapshotId,
              }),
        ),
    );
    journalContractMocks.listJournalEvents.mockReset();
    journalContractMocks.patchSettings.mockReset();
    journalContractMocks.recoverJournal.mockReset();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it('streams a typed action into the accepted UI and reopens an immutable historical Attempt', async () => {
    await act(async () => {
      root.render(<JournalContractHarness />);
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(journalContractMocks.subscriptions).toHaveLength(1);
    act(() => requireExperience().openPanel());
    await flushReact();
    const liveSubscription = journalContractMocks.subscriptions[0];
    await act(async () => {
      liveSubscription.onMessage({
        kind: 'metadata',
        metadata: {
          thread_id: scope.thread_id,
          run_id: scope.run_id,
          attempt_id: liveAttempt.attempt_id,
          latest_sequence: 1,
          submit_at: 1_900,
          server_time: 2_000,
          journal_enabled: true,
          snapshots_enabled: true,
          journal_protocol_version: '1.1',
        },
      });
      liveSubscription.onMessage({ kind: 'event', event: liveDocumentEvent });
      await Promise.resolve();
    });
    await flushReact();

    expect(container.textContent).toContain('核验交付链路');
    expect(container.textContent).toContain('已读取 当前验收文档');
    expect(container.textContent).toContain('snapshot-live');
    expect(container.querySelectorAll('[role="tab"]')).toHaveLength(5);
    expect(requireExperience().state.transport.status).toBe('connected');
    expect(requireExperience().state.content.snapshot?.snapshot_id).toBe(
      'snapshot-live',
    );

    act(() => requireExperience().selectAttempt(historicalAttempt.attempt_id));
    await flushReact();
    await flushReact();

    expect(requireExperience().state.execution.selected_attempt_id).toBe(
      historicalAttempt.attempt_id,
    );
    expect(requireExperience().state.view_mode).toBe('historical');
    expect(requireExperience().state.content.snapshot?.snapshot_id).toBe(
      'snapshot-history',
    );
    expect(container.textContent).toContain('已读取 历史验收文档');
    expect(container.textContent).toContain('snapshot-history');
    expect(journalContractMocks.subscriptions).toHaveLength(1);
  });
});
