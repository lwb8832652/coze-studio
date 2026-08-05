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

import { Toast } from '@coze-arch/coze-design';

import {
  useJournalExperience,
  type JournalExperience,
} from '../use-journal-experience';
import { JournalTimeline } from '../journal-timeline';
import { JournalPanel, JournalRestoreButton } from '../journal-panel';
import { JournalConversationFlow } from '../journal-conversation-flow';
import {
  buildJournalTimelineItems,
  journalActionLabel,
} from '../journal-event-model';
import { WorkbenchClientError } from '../../../workbench/thread-client/canonical-fetch';
import type {
  WorkbenchArtifact,
  WorkbenchJournalBootstrap,
  WorkbenchJournalEvent,
  WorkbenchJournalSnapshot,
} from '../../../workbench/thread-client';

const journalMocks = vi.hoisted(() => ({
  auditSnapshotAction: vi.fn(),
  copyText: vi.fn(),
  getSettings: vi.fn(),
  getSnapshot: vi.fn(),
  getArtifactSignedURL: vi.fn(),
  patchSettings: vi.fn(),
  recoverJournal: vi.fn(),
  reduce: undefined as undefined | ((action: unknown) => unknown),
  streamStart: vi.fn(),
  streamStop: vi.fn(),
}));

vi.mock(
  '../../../workbench/thread-client/canonical-thread-client-singleton',
  () => ({
    canonicalThreadClient: {
      auditJournalSnapshotAction: journalMocks.auditSnapshotAction,
      getJournalSettings: journalMocks.getSettings,
      getJournalSnapshot: journalMocks.getSnapshot,
      getArtifactSignedURL: journalMocks.getArtifactSignedURL,
      patchJournalSettings: journalMocks.patchSettings,
      recoverJournal: journalMocks.recoverJournal,
    },
  }),
);

vi.mock('../journal-stream', () => ({
  createJournalStreamController: (options: {
    reduce: (action: unknown) => unknown;
  }) => {
    journalMocks.reduce = options.reduce;
    return {
      resumeStream: vi.fn(),
      start: journalMocks.streamStart,
      stop: journalMocks.streamStop,
    };
  },
}));

vi.mock('../../task-clipboard', () => ({
  copyTextToClipboard: journalMocks.copyText,
}));

vi.mock('../../task-markdown-content', () => ({
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Match the mocked module export.
  TaskMarkdownContent: ({ value }: { value: string }) => (
    <div>
      <h2>验收章节</h2>
      {value}
    </div>
  ),
}));

vi.mock('@coze-arch/bot-monaco-editor', () => ({
  // eslint-disable-next-line @typescript-eslint/naming-convention -- Match the mocked module export.
  Editor: ({
    saveViewState,
    value,
  }: {
    saveViewState?: boolean;
    value: string;
  }) => (
    <pre data-save-view-state={saveViewState ? 'true' : 'false'}>{value}</pre>
  ),
}));

vi.mock('@coze-common/virtual-list', async () => {
  // eslint-disable-next-line @typescript-eslint/consistent-type-imports -- Required for hoisted mock.
  const react = await vi.importActual<typeof import('react')>('react');
  return {
    // eslint-disable-next-line @typescript-eslint/naming-convention -- Match the mocked module export.
    AutoSizer: ({
      children,
    }: {
      children: (size: { height: number; width: number }) => React.ReactNode;
    }) => children({ height: 320, width: 640 }),
    VariableSizeList: react.forwardRef(
      (
        {
          children,
          itemCount,
          itemSize,
        }: {
          children: (item: {
            index: number;
            style: React.CSSProperties;
          }) => React.ReactNode;
          itemCount: number;
          itemSize: (index: number) => number;
        },
        ref,
      ) => {
        react.useImperativeHandle(ref, () => ({
          resetAfterIndex: vi.fn(),
          scrollToItem: vi.fn(),
        }));
        return Array.from({ length: Math.min(itemCount, 12) }, (_, index) =>
          react.createElement(
            react.Fragment,
            { key: index },
            children({
              index,
              style: { height: itemSize(index) },
            }),
          ),
        );
      },
    ),
  };
});

const baseEvent = ({
  data,
  eventId,
  eventType,
  sequence,
  snapshotId,
  status,
  type,
}: {
  data: Record<string, unknown>;
  eventId: string;
  eventType: string;
  sequence: number;
  snapshotId?: string;
  status: WorkbenchJournalEvent['status'];
  type: string;
}): WorkbenchJournalEvent => ({
  event_id: eventId,
  thread_id: 'thread-1',
  run_id: 'run-1',
  attempt_id: 'attempt-1',
  event_type: eventType,
  payload: { type, data },
  created_at: 1_000 + sequence,
  occurred_at: 1_000 + sequence,
  sequence,
  status,
  ...(snapshotId ? { snapshot_id: snapshotId } : {}),
});

const milestone = ({
  eventId,
  milestoneId,
  sequence,
  status,
  title,
}: {
  eventId: string;
  milestoneId: string;
  sequence: number;
  status: WorkbenchJournalEvent['status'];
  title: string;
}) =>
  baseEvent({
    eventId,
    eventType:
      status === 'completed' ? 'milestone.terminal' : 'milestone.started',
    sequence,
    status,
    type: 'milestone',
    data: { milestone_id: milestoneId, title },
  });

const action = ({
  actionId,
  contentType,
  eventId,
  milestoneId,
  sequence,
  snapshotId,
  status,
  target,
}: {
  actionId: string;
  contentType: 'browser' | 'document' | 'skill';
  eventId: string;
  milestoneId?: string;
  sequence: number;
  snapshotId?: string;
  status: WorkbenchJournalEvent['status'];
  target: string;
}) =>
  baseEvent({
    eventId,
    eventType: status === 'completed' ? 'action.terminal' : 'action.started',
    sequence,
    snapshotId,
    status,
    type: contentType,
    data: {
      action_id: actionId,
      ...(milestoneId ? { milestone_id: milestoneId } : {}),
      operation: contentType === 'skill' ? 'use_skill' : 'read',
      target,
      display_verb_running: contentType === 'skill' ? '正在使用' : '正在读取',
      display_verb_completed: contentType === 'skill' ? '已使用' : '已读取',
      content_type: contentType,
    },
  });

const artifactEvent = ({
  artifactId,
  eventId,
  sequence,
  title,
}: {
  artifactId: string;
  eventId: string;
  sequence: number;
  title: string;
}) =>
  baseEvent({
    eventId,
    eventType: 'artifact.created',
    sequence,
    status: 'completed',
    type: 'artifact',
    data: { artifact_id: artifactId, title },
  });

const failedAction = ({
  eventId = 'event-failed',
  ledgerStatus,
}: {
  eventId?: string;
  ledgerStatus?: string;
} = {}) =>
  baseEvent({
    eventId,
    eventType: 'action.terminal',
    sequence: 2,
    status: 'failed',
    type: 'terminal',
    data: {
      action_id: 'action-failed',
      milestone_id: 'milestone-failed',
      operation: 'execute',
      target: '构建验证',
      display_verb_running: '正在执行',
      display_verb_completed: '已执行',
      content_type: 'terminal',
      error_summary: '构建命令执行失败',
      retryable: true,
      ...(ledgerStatus ? { ledger_status: ledgerStatus } : {}),
    },
  });

const mediaArtifact = ({
  artifactId,
  contentType,
  previewMode,
}: {
  artifactId: string;
  contentType: string;
  previewMode: string;
}): WorkbenchArtifact => ({
  artifact_id: artifactId,
  thread_id: 'thread-1',
  run_id: 'run-1',
  file_id: `file-${artifactId}`,
  title: `asset-${artifactId}`,
  artifact_type: 'media',
  virtual_path: `/asset-${artifactId}`,
  content_type: contentType,
  size_bytes: 1024,
  preview_mode: previewMode,
  metadata: '{}',
  created_at: 1_000,
  updated_at: 1_000,
  source: 'agent_generated',
  generation_status: 'ready',
  capabilities: ['preview', 'download'],
  is_primary: true,
});

const events: WorkbenchJournalEvent[] = [
  milestone({
    eventId: 'event-1',
    milestoneId: 'milestone-1',
    sequence: 1,
    status: 'running',
    title: '规划分析',
  }),
  action({
    actionId: 'action-1',
    contentType: 'document',
    eventId: 'event-2',
    milestoneId: 'milestone-1',
    sequence: 2,
    snapshotId: 'snapshot-document',
    status: 'completed',
    target: '需求文档',
  }),
  milestone({
    eventId: 'event-3',
    milestoneId: 'milestone-1',
    sequence: 3,
    status: 'completed',
    title: '规划分析',
  }),
  action({
    actionId: 'action-2',
    contentType: 'skill',
    eventId: 'event-4',
    sequence: 4,
    snapshotId: 'snapshot-skill',
    status: 'running',
    target: 'research-planner',
  }),
];

const skillSnapshot: WorkbenchJournalSnapshot = {
  content_type: 'skill',
  snapshot_id: 'snapshot-skill',
  event_id: 'event-4',
  attempt_id: 'attempt-1',
  is_fragmented: false,
  status: 'ready',
  created_at: 1_004,
  visibility: 'user',
  fragments: [],
  has_more: false,
  content: {
    skill: {
      skills: [
        {
          skill_id: 'research-planner',
          name: 'research-planner',
          description: '规划公开资料检索与信息核验',
          invocation_status: 'available',
          input_summary: 'private arguments',
        },
        {
          skill_id: 'document-tools',
          name: 'document-tools',
          purpose_summary: 'internal purpose must stay hidden',
        },
      ],
    },
  },
};

const documentSnapshot = (
  snapshotId: string,
  eventId: string,
): WorkbenchJournalSnapshot => ({
  attempt_id: 'attempt-1',
  content_type: 'document',
  created_at: 2_000,
  event_id: eventId,
  fragments: [],
  has_more: false,
  is_fragmented: false,
  snapshot_id: snapshotId,
  status: 'ready',
  visibility: 'user',
  content: {
    document: {
      content: `# ${snapshotId}`,
      title: snapshotId,
    },
  },
});

const browserSnapshot: WorkbenchJournalSnapshot = {
  attempt_id: 'attempt-1',
  content_type: 'browser',
  created_at: 2_000,
  event_id: 'browser-event-1',
  fragments: [],
  has_more: false,
  is_fragmented: false,
  snapshot_id: 'browser-snapshot-1',
  status: 'ready',
  visibility: 'user',
  content: {
    browser: {
      capture_id: 'capture-1',
      title: '产品页面',
      static_snapshot_base64: 'aW1hZ2U=',
      mime_type: 'image/png',
      redacted: true,
      redaction_evidence_id: 'evidence-1',
      redaction_policy_version: '1',
    },
  },
};

const terminalSnapshot = (
  output: string,
  status: 'ready' | 'streaming' = 'ready',
): WorkbenchJournalSnapshot => ({
  attempt_id: 'attempt-1',
  content_type: 'terminal',
  created_at: 2_000,
  event_id: 'terminal-event-1',
  fragments: [],
  has_more: false,
  is_fragmented: false,
  snapshot_id: 'terminal-snapshot-1',
  status,
  visibility: 'user',
  content: {
    terminal: {
      command: 'rushx test',
      output,
      working_directory: 'frontend/apps/coze-studio',
    },
  },
});

const codeSnapshot: WorkbenchJournalSnapshot = {
  attempt_id: 'attempt-1',
  content_type: 'code',
  created_at: 2_000,
  event_id: 'code-event-1',
  fragments: [],
  has_more: false,
  is_fragmented: false,
  snapshot_id: 'code-snapshot-1',
  status: 'ready',
  visibility: 'user',
  content: {
    code: {
      content: 'export const ready = true;',
      file_path: 'src/journal.ts',
      language: 'typescript',
      repository: 'agent-output',
      revision: '0123456789abcdef',
    },
  },
};

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, reject, resolve };
};

const noPermissionError = () =>
  new WorkbenchClientError({
    code: 'NO_PERMISSION',
    message: 'forbidden',
    outcome: 'rejected',
    retryable: false,
    status: 403,
  });

const recoveryBootstrap = (): WorkbenchJournalBootstrap => {
  const attempt = {
    attempt_id: 'attempt-1',
    run_id: 'run-1',
    status: 'failed' as const,
    projection_state: 'healthy' as const,
    latest_sequence: 2,
    created_at: 1_000,
    recovery_capability: {
      allowed: true,
      requires_confirmation: false,
      allowed_actions: ['retry'],
    },
  };
  return {
    attempts: [attempt],
    default_attempt_id: attempt.attempt_id,
    default_attempt: attempt,
    projection_state: 'healthy',
    latest_sequence: 2,
    events: {
      items: [failedAction()],
      has_more: false,
      attempt_id: attempt.attempt_id,
      latest_sequence: 2,
    },
    content_types: ['terminal'],
    enrollment: {
      enrolled: true,
      schema_version: '1',
      payload_version: '1',
      journal_protocol_version: '1',
      journal_enabled: true,
      snapshots_enabled: true,
    },
    submit_at: 1_000,
    server_time: 1_100,
    recovery_capability: attempt.recovery_capability,
  };
};

let currentExperience: JournalExperience | undefined;

const JournalExperienceHarness = () => {
  currentExperience = useJournalExperience({
    enabled: true,
    runId: 'run-1',
    spaceId: 'space-1',
    threadId: 'thread-1',
  });
  return (
    <output data-layout-ready={currentExperience.layoutReady}>
      {currentExperience.state.content.snapshot?.snapshot_id ??
        currentExperience.state.content.status}
    </output>
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
  });
};

describe('accepted Journal production UI contract', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    journalMocks.getArtifactSignedURL.mockReset();
    vi.spyOn(HTMLMediaElement.prototype, 'load').mockImplementation(
      () => undefined,
    );
    vi.spyOn(HTMLMediaElement.prototype, 'pause').mockImplementation(
      () => undefined,
    );
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('shows only executed hierarchy, keeps completed groups collapsed, and has no chevron for atomic actions', () => {
    act(() =>
      root.render(
        <JournalConversationFlow
          events={events}
          selectedEventId="event-4"
          onSelectEvent={vi.fn()}
        />,
      ),
    );

    expect(container.textContent).toContain('规划分析');
    expect(container.textContent).not.toContain('需求文档');
    expect(container.textContent).toContain('正在使用 research-planner');
    expect(
      container
        .querySelector('[data-milestone-id="action-2"]')
        ?.getAttribute('data-expandable'),
    ).toBe('false');
    expect(
      container.querySelector(
        '[data-milestone-id="action-2"] [data-testid="journal-chevron"]',
      ),
    ).toBeNull();
    expect(container.textContent).not.toContain('EXECUTION JOURNAL');
    expect(container.textContent).not.toContain('Journal');
    expect(container.querySelector('.journal-execution-intro')).toBeNull();

    const completedHeader = container.querySelector<HTMLButtonElement>(
      '[data-milestone-id="milestone-1"] .journal-milestone-header',
    );
    act(() => completedHeader?.click());
    expect(container.textContent).toContain('已读取 需求文档');
    const completedAction = container.querySelector(
      '[data-milestone-id="milestone-1"] .journal-action-row',
    );
    expect(
      completedAction?.querySelector('.journal-action-title')?.textContent,
    ).toBe('读取相关内容');
    expect(
      completedAction?.querySelector('.journal-action-detail-copy')
        ?.textContent,
    ).toBe('已读取 需求文档');
    expect(
      completedAction?.querySelector(
        '.journal-action-title .journal-action-icon',
      ),
    ).toBeNull();
    expect(
      completedAction?.querySelector(
        '.journal-action-detail .journal-action-icon',
      ),
    ).not.toBeNull();
  });

  it('renders the persisted execution intro exactly once before the first milestone', () => {
    const introEvent = baseEvent({
      eventId: 'event-intro',
      eventType: 'journal.intro',
      sequence: 1,
      status: 'running',
      type: 'intro',
      data: {
        text: '收到。我会先核验执行链路，再完成实现与验证。',
      },
    });
    const firstMilestone = milestone({
      eventId: 'event-first-milestone',
      milestoneId: 'milestone-first',
      sequence: 2,
      status: 'running',
      title: '核验执行链路',
    });

    act(() =>
      root.render(
        <JournalConversationFlow
          events={[introEvent, firstMilestone]}
          onSelectEvent={vi.fn()}
        />,
      ),
    );

    const intro = container.querySelector('.journal-execution-intro');
    const milestoneNode = container.querySelector('.journal-milestone');
    expect(intro?.textContent).toBe(
      '收到。我会先核验执行链路，再完成实现与验证。',
    );
    expect(container.querySelectorAll('.journal-execution-intro')).toHaveLength(
      1,
    );
    expect(
      Boolean(
        intro &&
          milestoneNode &&
          intro.compareDocumentPosition(milestoneNode) &
            Node.DOCUMENT_POSITION_FOLLOWING,
      ),
    ).toBe(true);
  });

  it('uses the first visible milestone intro as a fallback', () => {
    const firstMilestone = milestone({
      eventId: 'event-fallback-milestone',
      milestoneId: 'milestone-fallback',
      sequence: 1,
      status: 'running',
      title: '核验执行链路',
    });
    firstMilestone.payload = {
      ...firstMilestone.payload,
      data: {
        ...(firstMilestone.payload.data as Record<string, unknown>),
        execution_intro: '收到。我会按完整计划推进，并在验证后交付结果。',
      },
    };

    act(() =>
      root.render(
        <JournalConversationFlow
          events={[firstMilestone]}
          onSelectEvent={vi.fn()}
        />,
      ),
    );

    expect(
      container.querySelector('.journal-execution-intro')?.textContent,
    ).toBe('收到。我会按完整计划推进，并在验证后交付结果。');
  });

  it('keeps a safe concrete target when the terminal event is generic', () => {
    const lifecycleEvents = [
      milestone({
        eventId: 'target-milestone',
        milestoneId: 'target-milestone',
        sequence: 1,
        status: 'running',
        title: '读取项目事实',
      }),
      action({
        actionId: 'target-action',
        contentType: 'document',
        eventId: 'target-started',
        milestoneId: 'target-milestone',
        sequence: 2,
        status: 'running',
        target: 'requirements.md',
      }),
      action({
        actionId: 'target-action',
        contentType: 'document',
        eventId: 'target-completed',
        milestoneId: 'target-milestone',
        sequence: 3,
        status: 'completed',
        target: '文件',
      }),
    ];

    act(() =>
      root.render(
        <JournalConversationFlow
          events={lifecycleEvents}
          selectedEventId="target-completed"
          onSelectEvent={vi.fn()}
        />,
      ),
    );

    expect(
      container.querySelector('.journal-action-detail-copy')?.textContent,
    ).toBe('已读取 requirements.md');
  });

  it('keeps a safe concrete target without masking a failed terminal state', () => {
    const lifecycleEvents = [
      milestone({
        eventId: 'failed-target-milestone',
        milestoneId: 'failed-target-milestone',
        sequence: 1,
        status: 'running',
        title: '读取项目事实',
      }),
      action({
        actionId: 'failed-target-action',
        contentType: 'document',
        eventId: 'failed-target-started',
        milestoneId: 'failed-target-milestone',
        sequence: 2,
        status: 'running',
        target: 'requirements.md',
      }),
      action({
        actionId: 'failed-target-action',
        contentType: 'document',
        eventId: 'failed-target-terminal',
        milestoneId: 'failed-target-milestone',
        sequence: 3,
        status: 'failed',
        target: '文件',
      }),
    ];

    act(() =>
      root.render(
        <JournalConversationFlow
          events={lifecycleEvents}
          selectedEventId="failed-target-terminal"
          onSelectEvent={vi.fn()}
        />,
      ),
    );

    expect(
      container.querySelector('.journal-action-detail-copy')?.textContent,
    ).toBe('读取失败 requirements.md');
  });

  it('preserves the started milestone when a terminal lifecycle event omits it', () => {
    const lifecycleEvents = [
      milestone({
        eventId: 'lifecycle-milestone',
        milestoneId: 'lifecycle-milestone',
        sequence: 1,
        status: 'running',
        title: '读取项目事实',
      }),
      action({
        actionId: 'lifecycle-action',
        contentType: 'document',
        eventId: 'lifecycle-started',
        milestoneId: 'lifecycle-milestone',
        sequence: 2,
        status: 'running',
        target: '文件',
      }),
      action({
        actionId: 'lifecycle-action',
        contentType: 'document',
        eventId: 'lifecycle-completed',
        sequence: 3,
        status: 'completed',
        target: '文件',
      }),
    ];

    act(() =>
      root.render(
        <JournalConversationFlow
          events={lifecycleEvents}
          selectedEventId="lifecycle-completed"
          onSelectEvent={vi.fn()}
        />,
      ),
    );

    expect(
      container.querySelector('[data-milestone-id="lifecycle-milestone"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-milestone-id="lifecycle-action"]'),
    ).toBeNull();
  });

  it('uses failure-aware action copy instead of completed verbs', () => {
    expect(journalActionLabel(failedAction())).toBe('执行失败 构建验证');
  });

  it('keeps semantic titles for artifact, verification, and confirmation timeline nodes', () => {
    const semanticEvents = [
      artifactEvent({
        artifactId: 'artifact-report',
        eventId: 'artifact-report-event',
        sequence: 1,
        title: 'Journal 验收报告',
      }),
      baseEvent({
        eventId: 'verification-event',
        eventType: 'verification.terminal',
        sequence: 2,
        status: 'completed',
        type: 'verification',
        data: {
          verification_id: 'verification-1',
          title: '浏览器验收',
          result_summary: '全部通过',
        },
      }),
      baseEvent({
        eventId: 'confirmation-event',
        eventType: 'confirmation.requested',
        sequence: 3,
        status: 'pending',
        type: 'confirmation',
        data: {
          confirmation_id: 'confirmation-1',
          prompt: '确认继续发布吗',
        },
      }),
    ];

    expect(
      buildJournalTimelineItems(semanticEvents).map(item => item.title),
    ).toEqual(['Journal 验收报告', '浏览器验收', '确认继续发布吗']);
  });

  it('bounds the rendered DOM for a long expanded milestone', () => {
    const longEvents = [
      milestone({
        eventId: 'long-milestone',
        milestoneId: 'long-milestone',
        sequence: 1,
        status: 'running',
        title: '执行长任务',
      }),
      ...Array.from({ length: 120 }, (_, index) =>
        action({
          actionId: `long-action-${index}`,
          contentType: 'document',
          eventId: `long-event-${index}`,
          milestoneId: 'long-milestone',
          sequence: index + 2,
          status: 'completed',
          target: `文件 ${index}`,
        }),
      ),
    ];

    act(() =>
      root.render(
        <JournalConversationFlow
          events={longEvents}
          selectedEventId="long-event-119"
          onSelectEvent={vi.fn()}
        />,
      ),
    );

    expect(
      container.querySelector('[data-testid="journal-flow-virtualized"]'),
    ).not.toBeNull();
    expect(
      container.querySelectorAll('.journal-action-row').length,
    ).toBeLessThan(20);
  });

  it('keeps five base tabs, renders a one-line skill list, and omits internal availability details', () => {
    act(() =>
      root.render(
        <JournalPanel
          activeTab="skill"
          events={events}
          selectedEventId="event-4"
          snapshot={skillSnapshot}
          transportStatus="connected"
          viewMode="live_follow"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      ),
    );

    expect(container.querySelectorAll('[role="tab"]')).toHaveLength(5);
    expect(
      container.querySelectorAll('[data-testid="journal-skill-row"]'),
    ).toHaveLength(2);
    expect(
      container.querySelector('[data-testid="journal-skill-row"] svg'),
    ).not.toBeNull();
    expect(container.textContent).toContain('规划公开资料检索与信息核验');
    expect(container.textContent).not.toContain('可用');
    expect(container.textContent).not.toContain('当前使用');
    expect(container.textContent).not.toContain('private arguments');
    expect(container.textContent).not.toContain('internal purpose');
    expect(container.textContent).not.toContain('Journal');
  });

  it('navigates between adjacent browser snapshot events', () => {
    const onSelectEvent = vi.fn();
    const browserEvents = [
      action({
        actionId: 'browser-action-1',
        contentType: 'browser',
        eventId: 'browser-event-1',
        sequence: 1,
        snapshotId: 'browser-snapshot-1',
        status: 'completed',
        target: '产品页面',
      }),
      action({
        actionId: 'browser-action-2',
        contentType: 'browser',
        eventId: 'browser-event-2',
        sequence: 2,
        snapshotId: 'browser-snapshot-2',
        status: 'completed',
        target: '文档页面',
      }),
    ];
    act(() =>
      root.render(
        <JournalPanel
          activeTab="browser"
          contentStatus="ready"
          events={browserEvents}
          selectedEventId="browser-event-1"
          snapshot={browserSnapshot}
          transportStatus="ended"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={onSelectEvent}
          onViewModeChange={vi.fn()}
        />,
      ),
    );

    const previous = container.querySelector<HTMLButtonElement>(
      '[aria-label="上一个浏览快照"]',
    );
    const next = container.querySelector<HTMLButtonElement>(
      '[aria-label="下一个浏览快照"]',
    );
    expect(previous?.disabled).toBe(true);
    expect(next?.disabled).toBe(false);
    act(() => next?.click());
    expect(onSelectEvent).toHaveBeenCalledWith(browserEvents[1]);
  });

  it('uses document chapters to navigate the rendered safe Markdown', () => {
    const snapshot = documentSnapshot('document-with-toc', 'event-document');
    if (snapshot.content?.document) {
      snapshot.content.document.chapters = [
        { chapter_id: 'chapter-1', title: '验收章节', level: 2 },
      ];
    }
    act(() =>
      root.render(
        <JournalPanel
          activeTab="document"
          contentStatus="ready"
          events={events}
          selectedEventId="event-2"
          snapshot={snapshot}
          transportStatus="ended"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      ),
    );
    const heading = Array.from(container.querySelectorAll('h2')).find(
      item => item.textContent === '验收章节',
    );
    const scrollIntoView = vi.fn();
    if (heading) {
      heading.scrollIntoView = scrollIntoView;
    }
    const chapter = container.querySelector<HTMLButtonElement>(
      '.journal-document-toc button',
    );
    act(() => chapter?.click());
    expect(scrollIntoView).toHaveBeenCalledWith({
      behavior: 'smooth',
      block: 'start',
    });
  });

  it('renders streaming terminal output and follows only while the user stays near the bottom', () => {
    const renderPanel = (output: string) =>
      root.render(
        <JournalPanel
          activeTab="terminal"
          contentStatus="streaming"
          events={events}
          selectedEventId="terminal-event-1"
          snapshot={terminalSnapshot(output, 'streaming')}
          transportStatus="connected"
          viewMode="live_follow"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );

    act(() => renderPanel('line 1'));
    const surface = container.querySelector<HTMLElement>(
      '.journal-terminal-surface',
    );
    expect(surface).not.toBeNull();
    expect(container.textContent).toContain('line 1');
    Object.defineProperty(surface, 'clientHeight', {
      configurable: true,
      value: 120,
    });
    Object.defineProperty(surface, 'scrollHeight', {
      configurable: true,
      value: 320,
    });

    act(() => renderPanel('line 1\nline 2'));
    expect(surface?.scrollTop).toBe(320);

    if (surface) {
      surface.scrollTop = 20;
      act(() => surface.dispatchEvent(new Event('scroll', { bubbles: false })));
      Object.defineProperty(surface, 'scrollHeight', {
        configurable: true,
        value: 420,
      });
    }
    act(() => renderPanel('line 1\nline 2\nline 3'));
    expect(surface?.scrollTop).toBe(20);
  });

  it('preserves the read-only code editor view state per fixed revision', () => {
    act(() =>
      root.render(
        <JournalPanel
          activeTab="code"
          contentStatus="ready"
          events={events}
          selectedEventId="code-event-1"
          snapshot={codeSnapshot}
          transportStatus="ended"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      ),
    );

    expect(
      container
        .querySelector('.journal-code-editor pre')
        ?.getAttribute('data-save-view-state'),
    ).toBe('true');
  });

  it('restores authorized view scroll after the panel is closed and reopened', () => {
    const positions = new Map<string, { left: number; top: number }>();
    const getScrollPosition = (key: string) => positions.get(key);
    const onScrollPositionChange = (
      key: string,
      position: { left: number; top: number },
    ) => positions.set(key, position);
    const renderPanel = () => (
      <JournalPanel
        activeTab="document"
        contentStatus="ready"
        events={events}
        getScrollPosition={getScrollPosition}
        selectedAttemptId="attempt-1"
        selectedEventId="event-2"
        snapshot={documentSnapshot('document-scroll', 'event-2')}
        transportStatus="ended"
        viewMode="historical"
        onActiveTabChange={vi.fn()}
        onClose={vi.fn()}
        onScrollPositionChange={onScrollPositionChange}
        onSelectEvent={vi.fn()}
        onViewModeChange={vi.fn()}
      />
    );
    act(() => root.render(renderPanel()));
    const page = container.querySelector<HTMLElement>('.journal-document-page');
    if (page) {
      page.scrollTop = 140;
      page.scrollLeft = 6;
      act(() => page.dispatchEvent(new Event('scroll', { bubbles: false })));
    }

    act(() => root.render(<div />));
    act(() => root.render(renderPanel()));

    const restored = container.querySelector<HTMLElement>(
      '.journal-document-page',
    );
    expect(restored?.scrollTop).toBe(140);
    expect(restored?.scrollLeft).toBe(6);
  });

  it('keeps Timeline labels in accessible tooltips and provides a keyboard-reachable restore control', () => {
    const onSelectEvent = vi.fn();
    act(() =>
      root.render(
        <>
          <JournalTimeline
            events={events}
            selectedEventId="event-4"
            transportStatus="connected"
            viewMode="live_follow"
            onSelectEvent={onSelectEvent}
            onViewModeChange={vi.fn()}
          />
          <JournalRestoreButton onRestore={vi.fn()} />
        </>,
      ),
    );

    const track = container.querySelector(
      '[data-testid="journal-timeline-track"]',
    );
    expect(track?.textContent?.trim()).toBe('');
    expect(
      track?.querySelector('[aria-label="正在使用 research-planner"]'),
    ).not.toBeNull();
    const latestNode = track?.querySelector<HTMLButtonElement>(
      '[aria-label="正在使用 research-planner"]',
    );
    expect(track?.getAttribute('role')).toBeNull();
    expect(latestNode?.getAttribute('role')).toBeNull();
    act(() => latestNode?.focus());
    expect(container.querySelector('[role="tooltip"]')?.textContent).toContain(
      '正在使用 research-planner',
    );
    act(() => latestNode?.blur());
    expect(container.querySelector('[role="tooltip"]')).toBeNull();
    expect(
      container.querySelector<HTMLButtonElement>(
        '[aria-label="重新打开执行详情"]',
      ),
    ).not.toBeNull();
  });

  it('handles rejected snapshot actions and restores the control state', async () => {
    const snapshotAction = vi
      .fn()
      .mockRejectedValue(new Error('audit unavailable'));
    const toast = vi.spyOn(Toast, 'error').mockImplementation(() => undefined);
    act(() =>
      root.render(
        <JournalPanel
          activeTab="terminal"
          events={events}
          selectedEventId="event-2"
          snapshot={terminalSnapshot('terminal-action')}
          transportStatus="ended"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onSnapshotAction={snapshotAction}
          onViewModeChange={vi.fn()}
        />,
      ),
    );

    const copy = container.querySelector<HTMLButtonElement>(
      '[aria-label="复制命令"]',
    );
    await act(async () => {
      copy?.click();
      await Promise.resolve();
    });

    expect(snapshotAction).toHaveBeenCalledWith('copy_command');
    expect(toast).toHaveBeenCalledWith({
      content: '操作失败，请稍后重试',
      showClose: false,
    });
    expect(copy?.disabled).toBe(false);
  });

  it('uses roving keyboard focus for the five fixed content tabs', () => {
    const onActiveTabChange = vi.fn();
    act(() =>
      root.render(
        <JournalPanel
          activeTab="skill"
          events={events}
          selectedEventId="event-4"
          snapshot={skillSnapshot}
          transportStatus="connected"
          viewMode="live_follow"
          onActiveTabChange={onActiveTabChange}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      ),
    );
    const skillTab = container.querySelector<HTMLButtonElement>(
      '[role="tab"][aria-label="技能"]',
    );
    act(() =>
      skillTab?.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'ArrowRight', bubbles: true }),
      ),
    );

    expect(onActiveTabChange).toHaveBeenCalledWith('browser');
    expect(document.activeElement?.getAttribute('aria-label')).toBe('浏览器');
  });

  it('throttles live action announcements and suppresses historical playback', async () => {
    vi.useFakeTimers();
    const renderTimeline = (viewMode: 'historical' | 'live_follow') => (
      <JournalTimeline
        events={events}
        selectedEventId="event-4"
        transportStatus="connected"
        viewMode={viewMode}
        onSelectEvent={vi.fn()}
        onViewModeChange={vi.fn()}
      />
    );
    act(() => root.render(renderTimeline('live_follow')));
    await act(async () => vi.advanceTimersByTimeAsync(500));
    expect(
      container.querySelector('[aria-live="polite"]')?.textContent,
    ).toContain('正在使用 research-planner');

    act(() => root.render(renderTimeline('historical')));
    await act(async () => vi.advanceTimersByTimeAsync(500));
    expect(container.querySelector('[aria-live="polite"]')?.textContent).toBe(
      '',
    );
  });

  it('shows an Attempt selector only when recovery history exists', () => {
    const onSelectAttempt = vi.fn();
    const attempts = [
      {
        attempt_id: 'attempt-1',
        run_id: 'run-1',
        status: 'failed' as const,
        projection_state: 'healthy' as const,
        latest_sequence: 2,
        created_at: 1_000,
        recovery_capability: {
          allowed: true,
          requires_confirmation: false,
          allowed_actions: ['retry'],
        },
      },
      {
        attempt_id: 'attempt-2',
        run_id: 'run-1',
        status: 'running' as const,
        projection_state: 'healthy' as const,
        latest_sequence: 1,
        created_at: 2_000,
        recovery_capability: {
          allowed: false,
          requires_confirmation: false,
          allowed_actions: [],
        },
      },
    ];
    act(() =>
      root.render(
        <JournalPanel
          activeTab="document"
          attempts={attempts}
          events={events}
          selectedAttemptId="attempt-2"
          selectedEventId="event-4"
          transportStatus="connected"
          viewMode="live_follow"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectAttempt={onSelectAttempt}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      ),
    );

    const selector = container.querySelector<HTMLSelectElement>(
      '[aria-label="选择执行尝试"]',
    );
    expect(selector).not.toBeNull();
    expect(selector?.value).toBe('attempt-2');
    act(() => {
      if (selector) {
        selector.value = 'attempt-1';
        selector.dispatchEvent(new Event('change', { bubbles: true }));
      }
    });
    expect(onSelectAttempt).toHaveBeenCalledWith('attempt-1');
  });

  it('windows a dense Timeline and never folds milestone or failure nodes into aggregates', () => {
    const longEvents = [
      milestone({
        eventId: 'timeline-milestone',
        milestoneId: 'timeline-milestone',
        sequence: 1,
        status: 'running',
        title: '批量处理',
      }),
      ...Array.from({ length: 120 }, (_, index) =>
        action({
          actionId: `timeline-action-${index}`,
          contentType: 'document',
          eventId: `timeline-event-${index}`,
          milestoneId: 'timeline-milestone',
          sequence: index + 2,
          status: index === 40 ? 'failed' : 'completed',
          target: `文件 ${index}`,
        }),
      ),
    ];

    act(() =>
      root.render(
        <JournalTimeline
          events={longEvents}
          selectedEventId="timeline-event-119"
          transportStatus="connected"
          viewMode="live_follow"
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      ),
    );

    const window = container.querySelector(
      '[data-testid="journal-timeline-virtualized"]',
    );
    expect(window).not.toBeNull();
    expect(
      window?.querySelectorAll('.journal-timeline-node').length,
    ).toBeLessThan(20);
    expect(
      window?.querySelector('[data-semantic-kind="milestone"]'),
    ).not.toBeNull();
    expect(window?.querySelector('[data-aggregated="true"]')).not.toBeNull();
  });

  it('adds a temporary media tab only for the selected media event', async () => {
    const imageEvent = artifactEvent({
      artifactId: 'artifact-image',
      eventId: 'event-image',
      sequence: 5,
      title: '方案评审图',
    });
    const imageArtifact = mediaArtifact({
      artifactId: 'artifact-image',
      contentType: 'image/png',
      previewMode: 'image',
    });
    journalMocks.getArtifactSignedURL.mockResolvedValue({
      artifact_id: 'artifact-image',
      url: 'https://media.example/image.png',
      expires_in_seconds: 60,
      content_type: 'image/png',
      preview_mode: 'image',
    });

    act(() =>
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[imageArtifact]}
          events={[...events, imageEvent]}
          selectedEventId="event-2"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      ),
    );
    expect(container.querySelectorAll('[role="tab"]')).toHaveLength(5);

    await act(async () => {
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[imageArtifact]}
          events={[...events, imageEvent]}
          selectedEventId="event-image"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="live_follow"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );
      await Promise.resolve();
    });

    expect(container.querySelectorAll('[role="tab"]')).toHaveLength(6);
    expect(container.querySelector('[aria-label="图片"]')).not.toBeNull();
    expect(
      container.querySelector('img[referrerpolicy="no-referrer"]'),
    ).not.toBeNull();
  });

  it('uses safe non-autoplay players for audio and video media', async () => {
    const audioEvent = artifactEvent({
      artifactId: 'artifact-audio',
      eventId: 'event-audio',
      sequence: 1,
      title: '语音摘要',
    });
    const videoEvent = artifactEvent({
      artifactId: 'artifact-video',
      eventId: 'event-video',
      sequence: 2,
      title: '讲解视频',
    });
    const audioArtifact = mediaArtifact({
      artifactId: 'artifact-audio',
      contentType: 'audio/mpeg',
      previewMode: 'audio',
    });
    const videoArtifact = mediaArtifact({
      artifactId: 'artifact-video',
      contentType: 'video/mp4',
      previewMode: 'video',
    });
    journalMocks.getArtifactSignedURL.mockImplementation(
      ({ artifact_id: artifactId }: { artifact_id: string }) =>
        Promise.resolve({
          artifact_id: artifactId,
          url: `https://media.example/${artifactId}`,
          expires_in_seconds: 60,
          content_type:
            artifactId === 'artifact-audio' ? 'audio/mpeg' : 'video/mp4',
          preview_mode: artifactId === 'artifact-audio' ? 'audio' : 'video',
        }),
    );

    await act(async () => {
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[audioArtifact, videoArtifact]}
          events={[audioEvent, videoEvent]}
          selectedEventId="event-audio"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );
      await Promise.resolve();
    });
    const audio = container.querySelector('audio');
    expect(audio?.getAttribute('preload')).toBe('metadata');
    expect(audio?.hasAttribute('autoplay')).toBe(false);
    expect(audio?.getAttribute('referrerpolicy')).toBe('no-referrer');

    await act(async () => {
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[audioArtifact, videoArtifact]}
          events={[audioEvent, videoEvent]}
          selectedEventId="event-video"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );
      await Promise.resolve();
    });
    const video = container.querySelector('video');
    expect(video?.getAttribute('preload')).toBe('metadata');
    expect(video?.hasAttribute('autoplay')).toBe(false);
    expect(video?.getAttribute('referrerpolicy')).toBe('no-referrer');
  });

  it('renders media collections from server collection fields', async () => {
    const collectionEvent = artifactEvent({
      artifactId: 'artifact-collection',
      eventId: 'event-collection',
      sequence: 1,
      title: '多媒体交付包',
    });
    const collection = {
      ...mediaArtifact({
        artifactId: 'artifact-collection',
        contentType: 'application/octet-stream',
        previewMode: 'media_collection',
      }),
      collection_id: 'collection-1',
      title: '多媒体交付包',
    };
    const image = {
      ...mediaArtifact({
        artifactId: 'artifact-collection-image',
        contentType: 'image/png',
        previewMode: 'image',
      }),
      collection_id: 'collection-1',
      collection_order: 1,
    };
    const audio = {
      ...mediaArtifact({
        artifactId: 'artifact-collection-audio',
        contentType: 'audio/mpeg',
        previewMode: 'audio',
      }),
      collection_id: 'collection-1',
      collection_order: 2,
    };
    journalMocks.getArtifactSignedURL.mockResolvedValue({
      artifact_id: image.artifact_id,
      url: 'https://media.example/collection-image.png',
      expires_in_seconds: 60,
      content_type: 'image/png',
      preview_mode: 'image',
    });

    await act(async () => {
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[collection, audio, image]}
          events={[collectionEvent]}
          selectedEventId="event-collection"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );
      await Promise.resolve();
    });

    expect(container.querySelector('[aria-label="媒体集"]')).not.toBeNull();
    expect(
      container.querySelectorAll('.journal-media-collection-list > button'),
    ).toHaveLength(2);
    expect(container.textContent).toContain(image.title);
    expect(container.textContent).toContain(audio.title);
  });

  it('resets the selected media item when switching collections', async () => {
    const firstEvent = artifactEvent({
      artifactId: 'collection-a',
      eventId: 'event-collection-a',
      sequence: 1,
      title: '媒体集 A',
    });
    const secondEvent = artifactEvent({
      artifactId: 'collection-b',
      eventId: 'event-collection-b',
      sequence: 2,
      title: '媒体集 B',
    });
    const collectionA = {
      ...mediaArtifact({
        artifactId: 'collection-a',
        contentType: 'application/octet-stream',
        previewMode: 'media_collection',
      }),
      collection_id: 'collection-a',
    };
    const collectionB = {
      ...mediaArtifact({
        artifactId: 'collection-b',
        contentType: 'application/octet-stream',
        previewMode: 'media_collection',
      }),
      collection_id: 'collection-b',
    };
    const firstImage = {
      ...mediaArtifact({
        artifactId: 'collection-a-image',
        contentType: 'image/png',
        previewMode: 'image',
      }),
      collection_id: 'collection-a',
      collection_order: 1,
    };
    const firstAudio = {
      ...mediaArtifact({
        artifactId: 'collection-a-audio',
        contentType: 'audio/mpeg',
        previewMode: 'audio',
      }),
      collection_id: 'collection-a',
      collection_order: 2,
    };
    const secondImage = {
      ...mediaArtifact({
        artifactId: 'collection-b-image',
        contentType: 'image/png',
        previewMode: 'image',
      }),
      collection_id: 'collection-b',
      collection_order: 1,
    };
    journalMocks.getArtifactSignedURL.mockImplementation(
      ({ artifact_id: artifactId }: { artifact_id: string }) =>
        Promise.resolve({
          artifact_id: artifactId,
          url: `https://media.example/${artifactId}`,
          expires_in_seconds: 60,
          content_type: artifactId.endsWith('audio')
            ? 'audio/mpeg'
            : 'image/png',
          preview_mode: artifactId.endsWith('audio') ? 'audio' : 'image',
        }),
    );

    await act(async () => {
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[collectionA, firstImage, firstAudio]}
          events={[firstEvent, secondEvent]}
          selectedEventId="event-collection-a"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );
      await Promise.resolve();
    });
    const firstCollectionButtons =
      container.querySelectorAll<HTMLButtonElement>(
        '.journal-media-collection-list > button',
      );
    await act(async () => {
      firstCollectionButtons[1].click();
      await Promise.resolve();
    });
    expect(journalMocks.getArtifactSignedURL).toHaveBeenLastCalledWith(
      expect.objectContaining({ artifact_id: firstAudio.artifact_id }),
    );

    await act(async () => {
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[collectionB, secondImage]}
          events={[firstEvent, secondEvent]}
          selectedEventId="event-collection-b"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(journalMocks.getArtifactSignedURL).toHaveBeenLastCalledWith(
      expect.objectContaining({ artifact_id: secondImage.artifact_id }),
    );
  });

  it('re-authorizes media after the signed URL reaches its refresh window', async () => {
    vi.useFakeTimers();
    const imageEvent = artifactEvent({
      artifactId: 'artifact-expiring',
      eventId: 'event-expiring',
      sequence: 1,
      title: '短时图片',
    });
    const imageArtifact = mediaArtifact({
      artifactId: 'artifact-expiring',
      contentType: 'image/png',
      previewMode: 'image',
    });
    journalMocks.getArtifactSignedURL.mockResolvedValue({
      artifact_id: imageArtifact.artifact_id,
      url: 'https://media.example/expiring.png',
      expires_in_seconds: 6,
      content_type: 'image/png',
      preview_mode: 'image',
    });

    await act(async () => {
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[imageArtifact]}
          events={[imageEvent]}
          selectedEventId="event-expiring"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );
      await Promise.resolve();
    });
    expect(journalMocks.getArtifactSignedURL).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });
    expect(journalMocks.getArtifactSignedURL).toHaveBeenCalledTimes(2);
  });

  it('removes a stale media URL when re-authorization can no longer see the artifact', async () => {
    vi.useFakeTimers();
    const imageEvent = artifactEvent({
      artifactId: 'artifact-revoked',
      eventId: 'event-revoked',
      sequence: 1,
      title: '权限变化图片',
    });
    const imageArtifact = mediaArtifact({
      artifactId: 'artifact-revoked',
      contentType: 'image/png',
      previewMode: 'image',
    });
    journalMocks.getArtifactSignedURL
      .mockResolvedValueOnce({
        artifact_id: imageArtifact.artifact_id,
        url: 'https://media.example/revoked.png',
        expires_in_seconds: 6,
        content_type: 'image/png',
        preview_mode: 'image',
      })
      .mockRejectedValueOnce(
        new WorkbenchClientError({
          code: 'resource_not_found',
          message: 'Resource not found',
          outcome: 'rejected',
          retryable: false,
          status: 404,
        }),
      );

    await act(async () => {
      root.render(
        <JournalPanel
          activeTab="document"
          artifacts={[imageArtifact]}
          events={[imageEvent]}
          selectedEventId="event-revoked"
          spaceId="space-1"
          threadId="thread-1"
          transportStatus="connected"
          viewMode="historical"
          onActiveTabChange={vi.fn()}
          onClose={vi.fn()}
          onSelectEvent={vi.fn()}
          onViewModeChange={vi.fn()}
        />,
      );
      await Promise.resolve();
    });
    expect(container.querySelector('img')?.getAttribute('src')).toBe(
      'https://media.example/revoked.png',
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1_000);
    });

    expect(container.querySelector('img')).toBeNull();
    expect(container.textContent).toContain('无法查看此内容');
    expect(container.textContent).toContain('内容不存在或无权访问');
    expect(container.textContent).not.toContain('Resource not found');
    expect(container.textContent).not.toContain('刷新链接');
  });

  it('gates recovery by server capability and limits Unknown to controlled actions', () => {
    const failure = {
      ...failedAction({ ledgerStatus: 'unknown' }),
      trace_id: 'trace-safe-1',
    };
    const failureEvents = [
      milestone({
        eventId: 'milestone-failed-start',
        milestoneId: 'milestone-failed',
        sequence: 1,
        status: 'running',
        title: '运行验证',
      }),
      failure,
      milestone({
        eventId: 'milestone-failed-end',
        milestoneId: 'milestone-failed',
        sequence: 3,
        status: 'failed',
        title: '运行验证',
      }),
    ];
    const onRecover = vi.fn().mockResolvedValue(undefined);

    act(() =>
      root.render(
        <JournalConversationFlow
          events={failureEvents}
          recoveryCapability={{
            allowed: false,
            requires_confirmation: false,
            allowed_actions: [],
          }}
          selectedEventId={failure.event_id}
          viewMode="historical"
          onRecover={onRecover}
          onSelectEvent={vi.fn()}
        />,
      ),
    );
    expect(container.textContent).toContain('构建命令执行失败');
    expect(container.textContent).toContain('trace-safe-1');
    expect(container.querySelector('[aria-label="恢复失败操作"]')).toBeNull();

    act(() =>
      root.render(
        <JournalConversationFlow
          events={failureEvents}
          recoveryCapability={{
            allowed: true,
            requires_confirmation: true,
            allowed_actions: ['mark_succeeded', 'skip', 'retry'],
          }}
          selectedEventId={failure.event_id}
          viewMode="historical"
          onRecover={onRecover}
          onSelectEvent={vi.fn()}
        />,
      ),
    );
    const recoveryButton = container.querySelector<HTMLButtonElement>(
      '[aria-label="恢复失败操作"]',
    );
    act(() => recoveryButton?.click());

    const dialog = container.querySelector('[role="dialog"]');
    expect(dialog?.textContent).toContain('标记成功并跳过');
    expect(dialog?.textContent).toContain('跳过该动作');
    expect(dialog?.textContent).toContain('重新发起');
    expect(dialog?.textContent).not.toContain('继续执行');
  });

  it('shows the same failure and recovery affordance for an atomic step', () => {
    const groupedFailure = failedAction();
    const atomicFailure = {
      ...groupedFailure,
      payload: {
        ...groupedFailure.payload,
        data: {
          ...(groupedFailure.payload.data as Record<string, unknown>),
          milestone_id: undefined,
        },
      },
    };

    act(() =>
      root.render(
        <JournalConversationFlow
          events={[atomicFailure]}
          recoveryCapability={{
            allowed: true,
            requires_confirmation: false,
            allowed_actions: ['retry'],
          }}
          selectedEventId={atomicFailure.event_id}
          onRecover={vi.fn()}
          onSelectEvent={vi.fn()}
        />,
      ),
    );

    expect(container.textContent).toContain('构建命令执行失败');
    expect(
      container.querySelector('[aria-label="恢复失败操作"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="journal-chevron"]'),
    ).toBeNull();
  });
});

describe('Journal production state boundaries', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(async () => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    currentExperience = undefined;
    journalMocks.reduce = undefined;
    journalMocks.auditSnapshotAction.mockReset();
    journalMocks.copyText.mockReset();
    journalMocks.getSettings.mockReset();
    journalMocks.getSettings.mockResolvedValue({
      revision: 'settings-1',
      split_ratio: 0.4,
    });
    journalMocks.getSnapshot.mockReset();
    journalMocks.patchSettings.mockReset();
    journalMocks.recoverJournal.mockReset();
    journalMocks.streamStart.mockReset();
    journalMocks.streamStart.mockResolvedValue(undefined);
    journalMocks.streamStop.mockReset();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    await act(async () => {
      root.render(<JournalExperienceHarness />);
      await Promise.resolve();
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  const receiveEvents = async (nextEvents: WorkbenchJournalEvent[]) => {
    await act(async () => {
      journalMocks.reduce?.({ type: 'events_received', events: nextEvents });
      await Promise.resolve();
    });
    await flushReact();
  };

  it('keeps Follow closed until the user explicitly opens it', () => {
    expect(requireExperience().panelOpen).toBe(false);
  });

  it('ignores a stale snapshot response after the user advances to a newer event', async () => {
    const first = deferred<WorkbenchJournalSnapshot>();
    const second = deferred<WorkbenchJournalSnapshot>();
    journalMocks.getSnapshot.mockImplementation(
      ({ snapshot_id: snapshotId }: { snapshot_id: string }) =>
        snapshotId === 'snapshot-a' ? first.promise : second.promise,
    );
    const eventA = action({
      actionId: 'action-a',
      contentType: 'document',
      eventId: 'event-a',
      sequence: 1,
      snapshotId: 'snapshot-a',
      status: 'completed',
      target: '需求文档',
    });
    const eventB = action({
      actionId: 'action-b',
      contentType: 'document',
      eventId: 'event-b',
      sequence: 2,
      snapshotId: 'snapshot-b',
      status: 'completed',
      target: '验收文档',
    });

    act(() => requireExperience().openPanel());
    await receiveEvents([eventA]);
    await receiveEvents([eventB]);
    second.resolve(documentSnapshot('snapshot-b', 'event-b'));
    await flushReact();
    first.resolve(documentSnapshot('snapshot-a', 'event-a'));
    await flushReact();

    expect(requireExperience().state.content.snapshot?.snapshot_id).toBe(
      'snapshot-b',
    );
  });

  it('clears previously authorized content when the next snapshot is forbidden', async () => {
    journalMocks.getSnapshot
      .mockResolvedValueOnce(documentSnapshot('snapshot-a', 'event-a'))
      .mockRejectedValueOnce(noPermissionError());
    const eventA = action({
      actionId: 'action-a',
      contentType: 'document',
      eventId: 'event-a',
      sequence: 1,
      snapshotId: 'snapshot-a',
      status: 'completed',
      target: '需求文档',
    });
    const eventB = action({
      actionId: 'action-b',
      contentType: 'document',
      eventId: 'event-b',
      sequence: 2,
      snapshotId: 'snapshot-b',
      status: 'completed',
      target: '受限文档',
    });

    act(() => requireExperience().openPanel());
    await receiveEvents([eventA]);
    expect(requireExperience().state.content.snapshot?.snapshot_id).toBe(
      'snapshot-a',
    );
    await receiveEvents([eventB]);

    expect(requireExperience().state.content.status).toBe('no_permission');
    expect(requireExperience().state.content.snapshot).toBeUndefined();
  });

  it('fails closed before copy, open, or download when action audit fails', async () => {
    journalMocks.getSnapshot.mockResolvedValue(
      documentSnapshot('snapshot-a', 'event-a'),
    );
    journalMocks.auditSnapshotAction.mockRejectedValue(
      new Error('audit unavailable'),
    );
    const anchorClick = vi
      .spyOn(HTMLAnchorElement.prototype, 'click')
      .mockImplementation(() => undefined);
    const open = vi.spyOn(window, 'open').mockImplementation(() => null);
    act(() => requireExperience().openPanel());
    await receiveEvents([
      action({
        actionId: 'action-a',
        contentType: 'document',
        eventId: 'event-a',
        sequence: 1,
        snapshotId: 'snapshot-a',
        status: 'completed',
        target: '需求文档',
      }),
    ]);

    const experience = requireExperience();
    const results = await Promise.allSettled([
      experience.performSnapshotAction('copy_code'),
      experience.performSnapshotAction('open_original'),
      experience.performSnapshotAction('download_fragment', 'fragment-1'),
    ]);

    expect(results.every(result => result.status === 'rejected')).toBe(true);
    expect(journalMocks.copyText).not.toHaveBeenCalled();
    expect(open).not.toHaveBeenCalled();
    expect(anchorClick).not.toHaveBeenCalled();
    open.mockRestore();
    anchorClick.mockRestore();
  });

  it('clears snapshot content on close and refetches it after restore', async () => {
    journalMocks.getSnapshot.mockResolvedValue(
      documentSnapshot('snapshot-a', 'event-a'),
    );
    act(() => requireExperience().openPanel());
    await receiveEvents([
      action({
        actionId: 'action-a',
        contentType: 'document',
        eventId: 'event-a',
        sequence: 1,
        snapshotId: 'snapshot-a',
        status: 'completed',
        target: '需求文档',
      }),
    ]);
    expect(journalMocks.getSnapshot).toHaveBeenCalledTimes(1);

    act(() => requireExperience().closePanel());
    expect(requireExperience().state.content.status).toBe('empty');
    act(() => requireExperience().openPanel());
    await flushReact();

    expect(journalMocks.getSnapshot).toHaveBeenCalledTimes(2);
    expect(requireExperience().state.content.snapshot?.snapshot_id).toBe(
      'snapshot-a',
    );
  });

  it('waits for settings before layout and retries one CAS conflict', async () => {
    expect(requireExperience().layoutReady).toBe(true);
    journalMocks.patchSettings
      .mockRejectedValueOnce(
        new WorkbenchClientError({
          code: 'journal_settings_conflict',
          message: 'stale revision',
          outcome: 'rejected',
          retryable: true,
          status: 409,
        }),
      )
      .mockResolvedValueOnce({ revision: 'settings-3', split_ratio: 0.55 });
    journalMocks.getSettings.mockResolvedValueOnce({
      revision: 'settings-2',
      split_ratio: 0.4,
    });

    await act(async () => {
      await requireExperience().commitSplitRatio(0.55);
    });

    expect(journalMocks.patchSettings).toHaveBeenNthCalledWith(1, {
      revision: 'settings-1',
      split_ratio: 0.55,
    });
    expect(journalMocks.patchSettings).toHaveBeenNthCalledWith(2, {
      revision: 'settings-2',
      split_ratio: 0.55,
    });
    expect(journalMocks.getSettings).toHaveBeenCalledTimes(2);
  });

  it('recovers from the selected attempt with a fresh idempotency key and restarts the stream', async () => {
    await act(async () => {
      journalMocks.reduce?.({
        type: 'bootstrap_succeeded',
        bootstrap: recoveryBootstrap(),
      });
      await Promise.resolve();
    });
    journalMocks.recoverJournal.mockResolvedValue({
      accepted: true,
      attempt: {
        ...recoveryBootstrap().default_attempt,
        attempt_id: 'attempt-2',
        run_id: 'run-2',
        status: 'pending',
      },
    });
    const initialStreamStarts = journalMocks.streamStart.mock.calls.length;

    await act(async () => {
      await requireExperience().recover('retry', true);
    });

    expect(journalMocks.recoverJournal).toHaveBeenCalledWith(
      expect.objectContaining({
        action: 'retry',
        confirmed: true,
        source_attempt_id: 'attempt-1',
        space_id: 'space-1',
        thread_id: 'thread-1',
        run_id: 'run-1',
        idempotency_key: expect.stringMatching(/^journal:recover:retry:/),
      }),
    );
    expect(journalMocks.streamStart.mock.calls.length).toBeGreaterThan(
      initialStreamStarts,
    );
  });
});
