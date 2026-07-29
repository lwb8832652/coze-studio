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

import './newx-task-ui.less';

/* eslint-disable max-lines -- P0 DeerFlow parity wiring; split after parity stabilizes. */

import { useParams } from 'react-router-dom';
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent,
} from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';
import { useUserInfo } from '@coze-arch/foundation-sdk';

import '../../components/workspace-prototype.less';
import '../workbench/index.less';
import { TaskUsagePopover } from './task-usage-popover';
import {
  isTaskThreadDetailReadOnly,
  type TaskThreadDetailEvent,
  type TaskThreadDetailModel,
} from './task-thread-detail-model';
import { TaskSubagentRunsSection } from './task-subagent-runs-section';
import {
  TaskRunActionBar,
  type TaskRunActionLoading,
} from './task-run-action-bar';
import {
  TaskResultSection,
  TaskStreamingIndicator,
} from './task-result-section';
import {
  buildTaskUsageDetailItems,
  getCurrentAssistantRunID,
  loadTaskTokenUsageViewMode,
  saveTaskTokenUsageViewMode,
} from './task-message-token-usage';
import { TaskMarkdownContent } from './task-markdown-content';
import { TaskInlineReasoning } from './task-inline-reasoning';
import { TaskHumanInterruptCard } from './task-human-interrupt-card';
import {
  getPendingHumanInteraction,
  type PendingHumanInteraction,
} from './task-human-interaction';
import { TaskFollowUpComposer } from './task-follow-up-composer';
import { TaskExecutionTodoDock } from './task-execution-todo-dock';
import {
  type TaskDetailSubagentRun,
  type TaskTokenUsageViewMode,
} from './task-detail-loader';
import { useTaskDetailActions, useTaskDetailData } from './task-detail-hooks';
import { TaskDetailHeader } from './task-detail-header';
import { TaskAssistantMessageActions } from './task-assistant-message-actions';
import {
  artifactScanStatus,
  canPreviewArtifact,
} from './task-artifacts-helpers';
import {
  TaskArtifactFeedback,
  TaskArtifactMessageList,
} from './task-artifact-message-list';
import {
  type TaskArtifactActions,
  useTaskArtifactActions,
} from './task-artifact-actions';
import { generateTaskThreadSuggestions } from './service';
import {
  getTaskExecutionType,
  getTaskInputText,
  getLatestAnswerEventReasoning,
  getLatestAnswerEventMessage,
  parseTaskResultPayload,
  isTaskTerminalStatus,
  canCancelTask,
} from './helpers';
import { TaskExecutionSummary } from './execution-summary';
import {
  TaskAssistantTurnShell,
  TaskConversationColumn,
  TaskUserTurn,
} from './conversation-turn';

type HumanInteractionResponse = workbenchTask.HumanInteractionResponse;
type TaskThreadMessage = workbenchTask.TaskThreadMessage;
type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;

type ThreadTranscriptMessage = TaskThreadMessage & {
  role: 'user' | 'assistant';
  content: string;
};

interface AssistantArtifactTarget {
  createdAt: number;
  key: string;
  runID: string;
}

interface MessageArtifactGroups {
  before: TaskThreadArtifact[];
  after: TaskThreadArtifact[];
}

const TASK_DETAIL_SKELETON_STAGGER_MS = 60;
const TASK_DETAIL_RESPONSIVE_PAGE_CLASS = 'coze-task-detail-responsive-page';
const ARTIFACT_SPLIT_DEFAULT_WIDTH = 40;
const ARTIFACT_SPLIT_MIN_WIDTH = 30;
const ARTIFACT_SPLIT_MAX_WIDTH = 55;
const PERCENTAGE_SCALE = 100;
const TASK_DETAIL_SUGGESTION_COUNT = 3;
const TASK_DETAIL_SUGGESTION_HISTORY_LIMIT = 6;

const normalizeThreadRunID = (runID?: string) => {
  const value = String(runID ?? '').trim();

  return value && value !== '0' ? value : '';
};

const getTaskThreadEventRunID = (event: TaskThreadDetailEvent) =>
  normalizeThreadRunID(
    (event as TaskThreadDetailEvent & { run_id?: string }).run_id,
  );

const getRunEvents = (events: TaskThreadDetailEvent[], runID: string) => {
  if (!runID) {
    return events;
  }

  return events.filter(event => getTaskThreadEventRunID(event) === runID);
};

const getTaskArtifactRunID = (artifact: TaskThreadArtifact) =>
  normalizeThreadRunID(artifact.run_id);

const getTaskArtifactTime = (artifact: TaskThreadArtifact) =>
  Number(artifact.created_at || artifact.updated_at || 0);

const getTranscriptMessageTime = (message: ThreadTranscriptMessage) =>
  Number(message.created_at || 0);

const getThreadTranscriptMessageKey = (
  message: ThreadTranscriptMessage,
  index: number,
) => message.message_id || `${message.role}-${index}`;

const getLatestPreviewableArtifactID = (artifacts: TaskThreadArtifact[]) =>
  [...artifacts]
    .filter(canPreviewArtifact)
    .sort(
      (left, right) =>
        right.created_at - left.created_at ||
        right.artifact_id.localeCompare(left.artifact_id),
    )[0]?.artifact_id ?? '';

const BLOCKING_ARTIFACT_SCAN_STATUSES = new Set([
  'blocked',
  'failed',
  'infected',
  'quarantined',
]);

const shouldClearArtifactPreviewForScanStatus = (
  artifact?: TaskThreadArtifact,
) =>
  artifact
    ? BLOCKING_ARTIFACT_SCAN_STATUSES.has(artifactScanStatus(artifact))
    : true;

const groupTaskArtifactsByRunID = (artifacts: TaskThreadArtifact[]) => {
  const grouped = new Map<string, TaskThreadArtifact[]>();

  artifacts.forEach(artifact => {
    const runID = getTaskArtifactRunID(artifact);
    if (!runID) {
      return;
    }
    grouped.set(runID, [...(grouped.get(runID) ?? []), artifact]);
  });

  return grouped;
};

const getFallbackArtifactTarget = (
  artifact: TaskThreadArtifact,
  targets: AssistantArtifactTarget[],
) => {
  if (!targets.length) {
    return undefined;
  }

  const artifactTime = getTaskArtifactTime(artifact);
  if (!artifactTime) {
    return targets[targets.length - 1];
  }

  const previousTarget = [...targets]
    .reverse()
    .find(target => !target.createdAt || target.createdAt <= artifactTime);

  return previousTarget ?? targets[0];
};

const groupFallbackTaskArtifactsByMessageKey = (
  artifacts: TaskThreadArtifact[],
  transcript: ThreadTranscriptMessage[],
) => {
  const assistantTargets = transcript
    .map((message, index): AssistantArtifactTarget | undefined => {
      if (message.role !== 'assistant') {
        return undefined;
      }

      return {
        createdAt: getTranscriptMessageTime(message),
        key: getThreadTranscriptMessageKey(message, index),
        runID: normalizeThreadRunID(message.run_id),
      };
    })
    .filter((target): target is AssistantArtifactTarget => Boolean(target));
  const assistantRunIDs = new Set(
    assistantTargets.map(target => target.runID).filter(Boolean),
  );
  const grouped = new Map<string, TaskThreadArtifact[]>();

  artifacts.forEach(artifact => {
    const runID = getTaskArtifactRunID(artifact);
    if (runID && assistantRunIDs.has(runID)) {
      return;
    }

    const target = getFallbackArtifactTarget(artifact, assistantTargets);
    if (!target) {
      return;
    }

    grouped.set(target.key, [...(grouped.get(target.key) ?? []), artifact]);
  });

  return grouped;
};

const splitTaskArtifactsForMessage = (
  artifacts: TaskThreadArtifact[],
  message: ThreadTranscriptMessage,
): MessageArtifactGroups => {
  const messageTime = getTranscriptMessageTime(message);

  return artifacts.reduce<MessageArtifactGroups>(
    (groups, artifact) => {
      const artifactTime = getTaskArtifactTime(artifact);

      if (artifactTime && messageTime && artifactTime < messageTime) {
        groups.before.push(artifact);
      } else {
        groups.after.push(artifact);
      }

      return groups;
    },
    { before: [], after: [] },
  );
};

const getMessageArtifactGroups = ({
  artifactsByRunID,
  fallbackArtifactsByMessageKey,
  index,
  message,
  renderedArtifactIDs,
}: {
  artifactsByRunID: Map<string, TaskThreadArtifact[]>;
  fallbackArtifactsByMessageKey: Map<string, TaskThreadArtifact[]>;
  index: number;
  message: ThreadTranscriptMessage;
  renderedArtifactIDs: Set<string>;
}) => {
  const runID = normalizeThreadRunID(message.run_id);
  const messageKey = getThreadTranscriptMessageKey(message, index);
  const seenArtifactIDs = new Set<string>();
  const messageArtifacts = [
    ...(runID ? (artifactsByRunID.get(runID) ?? []) : []),
    ...(fallbackArtifactsByMessageKey.get(messageKey) ?? []),
  ].filter(artifact => {
    if (renderedArtifactIDs.has(artifact.artifact_id)) {
      return false;
    }
    if (seenArtifactIDs.has(artifact.artifact_id)) {
      return false;
    }

    seenArtifactIDs.add(artifact.artifact_id);
    return true;
  });

  if (!messageArtifacts.length) {
    return { before: [], after: [] };
  }

  messageArtifacts.forEach(artifact =>
    renderedArtifactIDs.add(artifact.artifact_id),
  );

  return splitTaskArtifactsForMessage(messageArtifacts, message);
};

const getThreadTranscriptMessages = (
  messages: TaskThreadMessage[],
): ThreadTranscriptMessage[] => {
  const transcript: ThreadTranscriptMessage[] = [];

  messages.forEach(message => {
    const role = String(message.role ?? '')
      .trim()
      .toLowerCase();
    if (role !== 'user' && role !== 'assistant') {
      return;
    }

    const content = String(message.content ?? '').trim();
    if (!content) {
      return;
    }

    const item: ThreadTranscriptMessage = {
      ...message,
      role,
      content,
    };
    const previous = transcript[transcript.length - 1];

    if (role === 'user' && previous?.role === 'user') {
      transcript[transcript.length - 1] = item;
      return;
    }

    transcript.push(item);
  });

  return transcript;
};

const getLatestAssistantTranscriptKey = (
  transcript: ThreadTranscriptMessage[],
) => {
  for (let index = transcript.length - 1; index >= 0; index -= 1) {
    const message = transcript[index];
    if (message.role !== 'assistant') {
      continue;
    }

    return (
      message.message_id ||
      [
        normalizeThreadRunID(message.run_id),
        message.created_at || 0,
        message.content.length,
      ].join(':')
    );
  }

  return '';
};

const getThreadSuggestionMessages = (
  transcript: ThreadTranscriptMessage[],
): workbenchTask.TaskThreadSuggestionMessage[] =>
  transcript
    .slice(-TASK_DETAIL_SUGGESTION_HISTORY_LIMIT)
    .map(message => ({
      role: message.role,
      content: message.content,
    }))
    .filter(message => message.content.trim());

const normalizeTaskFollowUpSuggestions = (suggestions?: string[]) =>
  [
    ...new Set((suggestions ?? []).map(item => item.trim()).filter(Boolean)),
  ].slice(0, TASK_DETAIL_SUGGESTION_COUNT);

const TaskDetailSkeletonBar = ({
  className,
  originRight,
  style,
}: {
  className?: string;
  originRight?: boolean;
  style?: CSSProperties;
}) => (
  <div
    className={`coze-prototype-detail-loading-bar ${className ?? ''}`}
    data-origin={originRight ? 'right' : 'left'}
    style={style}
  />
);

const TaskDetailLoadingSkeleton = () => {
  let index = 0;
  const nextDelay = () => ({
    animationDelay: `${index++ * TASK_DETAIL_SKELETON_STAGGER_MS}ms`,
  });

  return (
    <div
      aria-label="加载任务详情"
      className="coze-prototype-detail-loading-skeleton"
      data-testid="task-detail-loading-skeleton"
    >
      <div className="coze-prototype-detail-loading-human" role="human-message">
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-full"
          originRight
          style={nextDelay()}
        />
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-80"
          originRight
          style={nextDelay()}
        />
      </div>
      <div
        className="coze-prototype-detail-loading-assistant"
        role="assistant-message"
      >
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-full"
          style={nextDelay()}
        />
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-full"
          style={nextDelay()}
        />
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-70"
          style={nextDelay()}
        />
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-full"
          style={nextDelay()}
        />
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-full"
          style={nextDelay()}
        />
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-full"
          style={nextDelay()}
        />
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-60"
          style={nextDelay()}
        />
        <TaskDetailSkeletonBar
          className="coze-prototype-detail-loading-bar-40"
          style={nextDelay()}
        />
      </div>
    </div>
  );
};

const canStopTaskRunFromComposer = ({
  latestTaskRunID,
  task,
}: {
  latestTaskRunID: string;
  task?: TaskThreadDetailModel;
}) => Boolean(latestTaskRunID && task && canCancelTask(task.status));

const TaskConversation = ({ task }: { task: TaskThreadDetailModel }) => (
  <TaskUserTurn createdAt={task.created_at}>
    {getTaskInputText(task.input) || task.title}
  </TaskUserTurn>
);

const TaskThreadAssistantMessage = ({
  message,
  isRunning = false,
  reasoning,
  streamingMessage,
}: {
  message?: ThreadTranscriptMessage;
  isRunning?: boolean;
  reasoning?: string;
  streamingMessage?: string;
}) => {
  const messageResult = message
    ? parseTaskResultPayload(message.content)
    : undefined;
  const answerMessage = messageResult?.message || streamingMessage;
  const inlineReasoning = reasoning || messageResult?.reasoning;

  if (!answerMessage && !inlineReasoning && !isRunning) {
    return null;
  }

  return (
    <article className="coze-prototype-answer" data-result-type="answer">
      <TaskInlineReasoning content={inlineReasoning} />
      {answerMessage ? <TaskMarkdownContent value={answerMessage} /> : null}
      {isRunning ? <TaskStreamingIndicator /> : null}
      {!isRunning && answerMessage ? (
        <TaskAssistantMessageActions copyText={answerMessage} />
      ) : null}
    </article>
  );
};

const TaskThreadArtifactCards = ({
  artifactActions,
  artifacts,
  latestPreviewableArtifactID,
  task,
}: {
  artifactActions: TaskArtifactActions;
  artifacts: TaskThreadArtifact[];
  latestPreviewableArtifactID: string;
  task: TaskThreadDetailModel;
}) => {
  if (!artifacts.length) {
    return null;
  }

  return (
    <TaskArtifactMessageList
      artifactActions={artifactActions}
      artifacts={artifacts}
      autoPreview={artifacts.some(
        artifact => artifact.artifact_id === latestPreviewableArtifactID,
      )}
      renderFeedback={false}
      renderReviewActions={false}
      spaceId={task.space_id}
      threadId={task.id}
    />
  );
};

const TaskThreadConversation = ({
  artifactActions,
  artifacts,
  events,
  latestTaskRunID,
  messages,
  onAssistantMessageRef,
  task,
}: {
  artifactActions: TaskArtifactActions;
  artifacts: TaskThreadArtifact[];
  events: TaskThreadDetailEvent[];
  latestTaskRunID: string;
  messages: TaskThreadMessage[];
  onAssistantMessageRef?: (runID: string, element: HTMLElement | null) => void;
  task: TaskThreadDetailModel;
}) => {
  const transcript = getThreadTranscriptMessages(messages);
  const artifactsByRunID = groupTaskArtifactsByRunID(artifacts);
  const fallbackArtifactsByMessageKey = groupFallbackTaskArtifactsByMessageKey(
    artifacts,
    transcript,
  );
  const latestPreviewableArtifactID = getLatestPreviewableArtifactID(artifacts);
  const renderedArtifactIDs = new Set<string>();
  const latestRunID = normalizeThreadRunID(latestTaskRunID);
  const latestRunEvents = getRunEvents(events, latestRunID);
  const latestRunIsActive = !isTaskTerminalStatus(task.status);
  const latestUserIndex = transcript.reduce(
    (latestIndex, message, index) =>
      message.role === 'user' ? index : latestIndex,
    -1,
  );
  const latestAssistantIndexAfterLatestUser = transcript.reduce(
    (latestIndex, message, index) =>
      message.role === 'assistant' && index > latestUserIndex
        ? index
        : latestIndex,
    -1,
  );
  let latestEventsRendered = false;
  const hasLatestAssistantMessage = latestRunID
    ? transcript.some(
        message =>
          message.role === 'assistant' &&
          normalizeThreadRunID(message.run_id) === latestRunID,
      )
    : latestAssistantIndexAfterLatestUser >= 0;
  const isRunningAssistantMessage = (runID: string, index: number) =>
    latestRunIsActive &&
    (latestRunID
      ? runID === latestRunID
      : index === latestAssistantIndexAfterLatestUser);
  const renderedItems = transcript.map((message, index) => {
    if (message.role === 'user') {
      return (
        <TaskUserTurn
          key={message.message_id || `user-${index}`}
          createdAt={message.created_at}
        >
          {message.content}
        </TaskUserTurn>
      );
    }

    const runID = normalizeThreadRunID(message.run_id);
    const messageKey = getThreadTranscriptMessageKey(message, index);
    const messageRunEvents = runID ? getRunEvents(events, runID) : [];
    const messageArtifacts = getMessageArtifactGroups({
      artifactsByRunID,
      fallbackArtifactsByMessageKey,
      index,
      message,
      renderedArtifactIDs,
    });
    const shouldRenderMessageEvents = messageRunEvents.length > 0;
    latestEventsRendered =
      latestEventsRendered ||
      Boolean(
        shouldRenderMessageEvents && latestRunID && runID === latestRunID,
      );

    return (
      <TaskAssistantTurnShell
        key={messageKey}
        ref={element => {
          if (runID) {
            onAssistantMessageRef?.(runID, element);
          }
        }}
        createdAt={message.created_at}
        subtitle={getTaskExecutionType(task.input)}
      >
        {shouldRenderMessageEvents ? (
          <TaskExecutionSummary events={messageRunEvents} task={task} />
        ) : null}
        <TaskThreadArtifactCards
          artifactActions={artifactActions}
          artifacts={messageArtifacts.before}
          latestPreviewableArtifactID={latestPreviewableArtifactID}
          task={task}
        />
        <TaskThreadAssistantMessage
          isRunning={isRunningAssistantMessage(runID, index)}
          message={message}
          reasoning={getLatestAnswerEventReasoning(messageRunEvents)}
        />
        <TaskThreadArtifactCards
          artifactActions={artifactActions}
          artifacts={messageArtifacts.after}
          latestPreviewableArtifactID={latestPreviewableArtifactID}
          task={task}
        />
      </TaskAssistantTurnShell>
    );
  });
  const unmatchedArtifacts = artifacts.filter(
    artifact => !renderedArtifactIDs.has(artifact.artifact_id),
  );

  return (
    <>
      {renderedItems}
      {!latestEventsRendered &&
      (latestRunEvents.length || !hasLatestAssistantMessage) ? (
        <TaskAssistantTurnShell subtitle={getTaskExecutionType(task.input)}>
          {latestRunEvents.length ? (
            <TaskExecutionSummary events={latestRunEvents} task={task} />
          ) : null}
          {!hasLatestAssistantMessage ? (
            <TaskThreadAssistantMessage
              isRunning={latestRunIsActive}
              reasoning={getLatestAnswerEventReasoning(latestRunEvents)}
              streamingMessage={getLatestAnswerEventMessage(latestRunEvents)}
            />
          ) : null}
        </TaskAssistantTurnShell>
      ) : null}
      <TaskThreadArtifactCards
        artifactActions={artifactActions}
        artifacts={unmatchedArtifacts}
        latestPreviewableArtifactID={latestPreviewableArtifactID}
        task={task}
      />
    </>
  );
};

const TaskTranscript = ({
  artifacts,
  events,
  humanInteractionError,
  humanInteractionLoading,
  latestTaskRunID,
  messages,
  pendingHumanInteraction,
  retryingSubagentRunId,
  subagentRetryError,
  subagentRuns,
  task,
  taskRunActionError,
  taskRunActionLoading,
  taskRunActionsDisabled,
  artifactActions,
  onHumanInteractionSubmit,
  onAssistantMessageRef,
  onRetrySubagentRun,
  onRetryTaskRun,
}: {
  artifacts: workbenchTask.TaskThreadArtifact[];
  artifactActions: TaskArtifactActions;
  events: TaskThreadDetailEvent[];
  humanInteractionError?: string;
  humanInteractionLoading: boolean;
  latestTaskRunID: string;
  messages: TaskThreadMessage[];
  pendingHumanInteraction?: PendingHumanInteraction;
  retryingSubagentRunId?: string;
  subagentRetryError?: string;
  subagentRuns: TaskDetailSubagentRun[];
  task: TaskThreadDetailModel;
  taskRunActionError?: string;
  taskRunActionLoading: TaskRunActionLoading;
  taskRunActionsDisabled?: boolean;
  onHumanInteractionSubmit: (
    response: HumanInteractionResponse,
  ) => void | Promise<void>;
  onAssistantMessageRef?: (runID: string, element: HTMLElement | null) => void;
  onRetrySubagentRun: (runId: string) => void | Promise<void>;
  onRetryTaskRun: (runId: string) => void | Promise<void>;
}) => (
  <section
    className="coze-prototype-chat-transcript"
    data-testid="task-chat-transcript"
  >
    <TaskConversationColumn>
      <TaskRunActionBar
        disabled={taskRunActionsDisabled}
        task={task}
        latestRunID={latestTaskRunID}
        loading={taskRunActionLoading}
        error={taskRunActionError}
        onRetryTaskRun={onRetryTaskRun}
      />
      {messages.length ? (
        <TaskThreadConversation
          artifactActions={artifactActions}
          artifacts={artifacts}
          events={events}
          latestTaskRunID={latestTaskRunID}
          messages={messages}
          onAssistantMessageRef={onAssistantMessageRef}
          task={task}
        />
      ) : (
        <>
          <TaskConversation task={task} />
          <TaskAssistantTurnShell subtitle={getTaskExecutionType(task.input)}>
            {events.length ||
            parseTaskResultPayload(task.result).resultType === 'agent_trace' ? (
              <TaskExecutionSummary events={events} task={task} />
            ) : null}
            <TaskResultSection
              task={task}
              events={events}
              tokenUsage={undefined}
              tokenUsageViewMode="off"
            />
            <TaskArtifactMessageList
              artifactActions={artifactActions}
              artifacts={artifacts}
              autoPreview={true}
              renderFeedback={false}
              renderReviewActions={false}
              spaceId={task.space_id}
              threadId={task.id}
            />
          </TaskAssistantTurnShell>
        </>
      )}
      <TaskSubagentRunsSection
        actionsDisabled={taskRunActionsDisabled}
        subagentRuns={subagentRuns}
        retryError={subagentRetryError}
        retryingRunId={retryingSubagentRunId}
        onRetrySubagentRun={onRetrySubagentRun}
      />
      {pendingHumanInteraction ? (
        <TaskHumanInterruptCard
          pending={pendingHumanInteraction}
          loading={humanInteractionLoading}
          error={humanInteractionError}
          onSubmit={onHumanInteractionSubmit}
        />
      ) : null}
    </TaskConversationColumn>
  </section>
);

// eslint-disable-next-line max-lines-per-function, @coze-arch/max-line-per-function -- Existing boundary.
const TaskDetailPage = () => {
  const { space_id, thread_id } = useParams();
  const userInfo = useUserInfo();
  const taskDetailId = thread_id;

  useEffect(() => {
    document.documentElement.classList.add(TASK_DETAIL_RESPONSIVE_PAGE_CLASS);
    document.body.classList.add(TASK_DETAIL_RESPONSIVE_PAGE_CLASS);

    return () => {
      document.documentElement.classList.remove(
        TASK_DETAIL_RESPONSIVE_PAGE_CLASS,
      );
      document.body.classList.remove(TASK_DETAIL_RESPONSIVE_PAGE_CLASS);
    };
  }, []);
  const {
    applyTaskDetail,
    applyOptimisticFollowUp,
    artifacts,
    captureTaskDetailRequestToken,
    error,
    events,
    latestTaskRunID,
    loadedTaskDetailCurrent,
    loading,
    messages,
    refreshArtifacts,
    suggestionModelName,
    suggestionModelType,
    subagentRuns,
    task,
    todos,
    tokenUsage,
    tokenUsageByRunID,
    tokenUsageError,
    tokenUsageIsPartial,
    tokenUsageLoadedCount,
    tokenUsageLoading,
    tokenUsageTotalCount,
    retryTokenUsage,
  } = useTaskDetailData({
    spaceID: space_id,
    taskDetailId,
  });
  const [tokenUsageViewMode, setTokenUsageViewMode] =
    useState<TaskTokenUsageViewMode>(loadTaskTokenUsageViewMode);
  const [followUpSuggestions, setFollowUpSuggestions] = useState<string[]>([]);
  const [followUpSuggestionsLoading, setFollowUpSuggestionsLoading] =
    useState(false);
  const [followUpSuggestionsHidden, setFollowUpSuggestionsHidden] =
    useState(false);
  const generatedSuggestionMessageKeyRef = useRef('');
  const assistantMessageRefs = useRef(new Map<string, HTMLElement>());

  useEffect(() => {
    saveTaskTokenUsageViewMode(tokenUsageViewMode);
  }, [tokenUsageViewMode]);

  const activeTaskDetailId = taskDetailId;
  const registerAssistantMessageRef = useCallback(
    (runID: string, element: HTMLElement | null) => {
      const normalizedRunID = String(runID ?? '').trim();
      if (!normalizedRunID) {
        return;
      }
      if (element) {
        assistantMessageRefs.current.set(normalizedRunID, element);
      } else {
        assistantMessageRefs.current.delete(normalizedRunID);
      }
    },
    [],
  );
  const locateAssistantReply = useCallback((runID: string) => {
    const target = assistantMessageRefs.current.get(String(runID ?? '').trim());
    if (!target) {
      return;
    }

    target.scrollIntoView?.({
      behavior: 'smooth',
      block: 'center',
    });
    target.focus({ preventScroll: true });
  }, []);
  useEffect(() => {
    assistantMessageRefs.current.clear();
  }, [activeTaskDetailId]);
  const currentAssistantRunID = getCurrentAssistantRunID(
    messages,
    latestTaskRunID,
  );
  const currentReplyTokenUsage = currentAssistantRunID
    ? tokenUsageByRunID[currentAssistantRunID]
    : undefined;
  const getTokenUsageDetailItems = useCallback(
    () => buildTaskUsageDetailItems(messages, tokenUsageByRunID),
    [messages, tokenUsageByRunID],
  );
  const hasAssistantReply = useMemo(
    () =>
      messages.some(
        message =>
          String(message.role ?? '')
            .trim()
            .toLowerCase() === 'assistant' &&
          Boolean(String(message.content ?? '').trim()),
      ),
    [messages],
  );
  const pendingHumanInteraction = getPendingHumanInteraction(events);
  const memoryReadOnly = isTaskThreadDetailReadOnly({
    task,
    userID: userInfo?.user_id_str,
  });
  const {
    followUpError,
    followUpLoading,
    followUpMode,
    followUpResetKey,
    followUpValue,
    handleFollowUpSubmit,
    handleHumanInteractionSubmit,
    handleCancelTaskRun,
    handleRetryTaskRun,
    handleRetrySubagentRun,
    humanInteractionError,
    humanInteractionLoading,
    retryingSubagentRunId,
    setFollowUpMode,
    setFollowUpValue,
    subagentRetryError,
    taskRunActionError,
    taskRunActionLoading,
    taskRunActionsDisabled,
  } = useTaskDetailActions({
    applyTaskDetail,
    applyOptimisticFollowUp,
    captureTaskDetailRequestToken,
    artifacts,
    events,
    messages,
    pendingHumanInteraction,
    spaceID: space_id,
    subagentRuns,
    task,
    taskDetailId: activeTaskDetailId,
    todos,
    tokenUsage,
    tokenUsageByRunID,
  });
  const canStopLatestTaskRun = canStopTaskRunFromComposer({
    latestTaskRunID,
    task,
  });
  const splitRef = useRef<HTMLElement | null>(null);
  const [artifactPanelWidth, setArtifactPanelWidth] = useState(
    ARTIFACT_SPLIT_DEFAULT_WIDTH,
  );
  const artifactActions = useTaskArtifactActions({
    onArtifactsChanged: refreshArtifacts,
    spaceId: space_id,
    threadId: loadedTaskDetailCurrent ? activeTaskDetailId : undefined,
  });
  const artifactPanelOpen = Boolean(artifactActions.inlinePreview);
  const artifactSplitStyle: CSSProperties = {
    '--coze-prototype-artifact-side-preview-width': `${artifactPanelWidth}%`,
  };

  useEffect(() => {
    if (
      !activeTaskDetailId ||
      !task ||
      loading ||
      !isTaskTerminalStatus(task.status)
    ) {
      setFollowUpSuggestions([]);
      setFollowUpSuggestionsLoading(false);
      return;
    }

    const transcript = getThreadTranscriptMessages(messages);
    const latestAssistantKey = [
      activeTaskDetailId,
      getLatestAssistantTranscriptKey(transcript),
      suggestionModelType,
      suggestionModelName,
    ]
      .filter(Boolean)
      .join(':');
    if (
      !latestAssistantKey ||
      latestAssistantKey === generatedSuggestionMessageKeyRef.current
    ) {
      return;
    }

    const suggestionMessages = getThreadSuggestionMessages(transcript);
    if (!suggestionMessages.length) {
      return;
    }

    let canceled = false;
    generatedSuggestionMessageKeyRef.current = latestAssistantKey;
    setFollowUpSuggestions([]);
    setFollowUpSuggestionsHidden(false);
    setFollowUpSuggestionsLoading(true);

    void generateTaskThreadSuggestions({
      thread_id: activeTaskDetailId,
      messages: suggestionMessages,
      n: TASK_DETAIL_SUGGESTION_COUNT,
      ...(suggestionModelName ? { model_name: suggestionModelName } : {}),
      ...(suggestionModelType ? { model_type: suggestionModelType } : {}),
    })
      .then(response => {
        if (!canceled) {
          setFollowUpSuggestions(
            normalizeTaskFollowUpSuggestions(response.suggestions),
          );
        }
      })
      .catch(() => {
        if (!canceled) {
          setFollowUpSuggestions([]);
        }
      })
      .finally(() => {
        if (!canceled) {
          setFollowUpSuggestionsLoading(false);
        }
      });

    return () => {
      canceled = true;
    };
  }, [
    activeTaskDetailId,
    loading,
    messages,
    suggestionModelName,
    suggestionModelType,
    task,
  ]);

  useEffect(() => {
    const inlineArtifactID = artifactActions.inlinePreview?.artifactId;
    if (!inlineArtifactID) {
      return;
    }

    const artifact = artifacts.find(
      item => item.artifact_id === inlineArtifactID,
    );
    if (shouldClearArtifactPreviewForScanStatus(artifact)) {
      artifactActions.clearInlinePreview();
    }
  }, [artifactActions, artifacts]);

  const handleArtifactResizePointerDown = useCallback(
    (event: PointerEvent<HTMLButtonElement>) => {
      const container = splitRef.current;
      if (!container) {
        return;
      }

      event.preventDefault();
      const handlePointerMove = (moveEvent: globalThis.PointerEvent) => {
        const rect = container.getBoundingClientRect();
        if (rect.width <= 0) {
          return;
        }
        const nextWidth =
          ((rect.right - moveEvent.clientX) / rect.width) * PERCENTAGE_SCALE;
        setArtifactPanelWidth(
          Math.min(
            ARTIFACT_SPLIT_MAX_WIDTH,
            Math.max(ARTIFACT_SPLIT_MIN_WIDTH, nextWidth),
          ),
        );
      };
      const handlePointerUp = () => {
        document.removeEventListener('pointermove', handlePointerMove);
        document.removeEventListener('pointerup', handlePointerUp);
      };

      document.addEventListener('pointermove', handlePointerMove);
      document.addEventListener('pointerup', handlePointerUp);
    },
    [],
  );

  return (
    <main className="coze-prototype-page coze-prototype-task-detail-page">
      {task ? (
        <TaskDetailHeader
          artifacts={artifacts}
          memoryReadOnly={memoryReadOnly}
          messages={messages}
          onArtifactsChanged={refreshArtifacts}
          spaceId={space_id ?? task.space_id}
          task={task}
          threadId={activeTaskDetailId}
        />
      ) : null}
      <section
        ref={splitRef}
        className="coze-prototype-detail-split"
        data-artifact-open={artifactPanelOpen}
        style={artifactSplitStyle}
      >
        <section className="coze-prototype-detail-inner">
          <section
            className="coze-prototype-detail-scroll"
            data-testid="task-detail-scroll"
          >
            {loading ? <TaskDetailLoadingSkeleton /> : null}
            {error ? (
              <div className="coze-prototype-error" role="alert">
                {error}
              </div>
            ) : null}
            {!loading && !error && !task ? (
              <div className="coze-prototype-empty">未找到任务</div>
            ) : null}
            {task ? (
              <TaskTranscript
                artifactActions={artifactActions}
                artifacts={artifacts}
                events={events}
                humanInteractionError={humanInteractionError}
                humanInteractionLoading={humanInteractionLoading}
                latestTaskRunID={latestTaskRunID}
                messages={messages}
                pendingHumanInteraction={pendingHumanInteraction}
                retryingSubagentRunId={retryingSubagentRunId}
                subagentRetryError={subagentRetryError}
                subagentRuns={subagentRuns}
                task={task}
                taskRunActionError={taskRunActionError}
                taskRunActionLoading={taskRunActionLoading}
                taskRunActionsDisabled={taskRunActionsDisabled}
                onHumanInteractionSubmit={handleHumanInteractionSubmit}
                onAssistantMessageRef={registerAssistantMessageRef}
                onRetrySubagentRun={handleRetrySubagentRun}
                onRetryTaskRun={handleRetryTaskRun}
              />
            ) : null}
          </section>
          {task ? (
            <TaskFollowUpComposer
              key={activeTaskDetailId}
              value={followUpValue}
              mode={followUpMode}
              loading={followUpLoading}
              error={followUpError}
              spaceId={space_id}
              taskId={activeTaskDetailId}
              todoDock={
                <TaskExecutionTodoDock
                  events={events}
                  task={task}
                  todos={todos}
                />
              }
              footerEnd={
                <TaskUsagePopover
                  currentReplyUsage={currentReplyTokenUsage}
                  currentReplyIncomplete={Boolean(
                    currentAssistantRunID && tokenUsageIsPartial,
                  )}
                  detailItems={[]}
                  detailItemsFactory={getTokenUsageDetailItems}
                  error={tokenUsageError}
                  hasAssistantReply={hasAssistantReply}
                  isPartial={tokenUsageIsPartial}
                  loadedCount={tokenUsageLoadedCount}
                  loading={tokenUsageLoading}
                  scopeKey={activeTaskDetailId ?? ''}
                  tokenUsage={tokenUsage}
                  totalCount={tokenUsageTotalCount}
                  viewMode={tokenUsageViewMode}
                  onRetry={retryTokenUsage}
                  onLocateReply={locateAssistantReply}
                  onViewModeChange={setTokenUsageViewMode}
                />
              }
              suggestions={followUpSuggestions}
              resetKey={followUpResetKey}
              capabilities={{
                attachments: true,
              }}
              suggestionsHidden={followUpSuggestionsHidden}
              suggestionsLoading={followUpSuggestionsLoading}
              stopLoading={taskRunActionsDisabled}
              stopMode={canStopLatestTaskRun}
              onDismissSuggestions={() => setFollowUpSuggestionsHidden(true)}
              onSuggestionClick={suggestion => {
                setFollowUpValue(currentValue =>
                  currentValue.trim()
                    ? `${currentValue.trimEnd()}\n${suggestion}`
                    : suggestion,
                );
                setFollowUpSuggestionsHidden(true);
              }}
              onValueChange={setFollowUpValue}
              onModeChange={setFollowUpMode}
              onStop={() => handleCancelTaskRun(latestTaskRunID)}
              onSubmit={handleFollowUpSubmit}
            />
          ) : null}
        </section>
        {artifactPanelOpen ? (
          <button
            type="button"
            aria-label="调整产物面板宽度"
            className="coze-prototype-artifact-resize-handle"
            onPointerDown={handleArtifactResizePointerDown}
          />
        ) : null}
        <TaskArtifactFeedback
          clearInlinePreview={artifactActions.clearInlinePreview}
          error={artifactActions.error}
          inlinePreview={artifactActions.inlinePreview}
        />
      </section>
    </main>
  );
};

export default TaskDetailPage;
