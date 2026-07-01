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

/* eslint-disable max-lines -- P0 DeerFlow parity wiring; split after parity stabilizes. */

import { useParams } from 'react-router-dom';
import { useEffect, useState, type CSSProperties, type ReactNode } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import {
  IconCozArrowDown,
  IconCozCode,
  IconCozDocument,
  IconCozEdit,
  IconCozLightbulb,
  IconCozMagnifier,
  IconCozPlugin,
} from '@coze-arch/coze-design/icons';

import '../../components/workspace-prototype.less';
import '../workbench/index.less';
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
  getLatestAssistantRunID,
  loadTaskTokenUsageViewMode,
  saveTaskTokenUsageViewMode,
  TaskMessageTokenUsage,
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
import { projectTaskExecutionEvents } from './task-event-projection';
import {
  type LoadedTaskDetailSource,
  type TaskDetailSource,
  type TaskDetailTokenUsage,
  type TaskDetailSubagentRun,
  type TaskTokenUsageViewMode,
} from './task-detail-loader';
import { useTaskDetailActions, useTaskDetailData } from './task-detail-hooks';
import { TaskDetailHeader } from './task-detail-header';
import { canPreviewArtifact } from './task-artifacts-helpers';
import {
  TaskArtifactFeedback,
  TaskArtifactMessageList,
} from './task-artifact-message-list';
import {
  type TaskArtifactActions,
  useTaskArtifactActions,
} from './task-artifact-actions';
import {
  getTaskExecutionType,
  getTaskInputText,
  getLatestAnswerEventReasoning,
  getLatestAnswerEventMessage,
  parseTaskResultPayload,
  isTaskTerminalStatus,
  canCancelTask,
  type TaskEventDisplay,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;
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

const normalizeThreadRunID = (runID?: string) => {
  const value = String(runID ?? '').trim();

  return value && value !== '0' ? value : '';
};

const getTaskEventRunID = (event: TaskEvent) =>
  normalizeThreadRunID((event as TaskEvent & { run_id?: string }).run_id);

const getRunEvents = (events: TaskEvent[], runID: string) => {
  if (!runID) {
    return events;
  }

  return events.filter(event => getTaskEventRunID(event) === runID);
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

const getTaskDetailSource = (threadId?: string): TaskDetailSource =>
  threadId ? 'thread' : 'auto';

const isTaskMemoryReadOnly = ({
  source,
  task,
  userID,
}: {
  source: LoadedTaskDetailSource;
  task?: ChatTask;
  userID?: string;
}) =>
  Boolean(
    source === 'thread' &&
      task?.creator_id &&
      userID &&
      task.creator_id !== userID,
  );

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
  taskDetailSource,
}: {
  latestTaskRunID: string;
  task?: ChatTask;
  taskDetailSource: LoadedTaskDetailSource;
}) =>
  Boolean(
    taskDetailSource === 'thread' &&
      latestTaskRunID &&
      task &&
      canCancelTask(task.status),
  );

const AssistantMark = () => (
  <span className="coze-prototype-assistant-mark" aria-hidden="true">
    <svg viewBox="0 0 24 24" className="h-[14px] w-[14px]" fill="currentColor">
      <path d="M12 2 2 22h20L12 2zm0 6 6 12H6l6-12z" />
    </svg>
  </span>
);

const TaskConversation = ({ task }: { task: ChatTask }) => {
  const executionType = getTaskExecutionType(task.input);

  return (
    <>
      <div className="flex justify-end">
        <div className="coze-prototype-user-bubble">
          {getTaskInputText(task.input) || task.title}
        </div>
      </div>

      <div className="coze-prototype-assistant-line">
        <AssistantMark />
        <span>Aime · {executionType} 已为你启动工作流</span>
      </div>
    </>
  );
};

const TaskAssistantLine = ({ task }: { task: ChatTask }) => (
  <div className="coze-prototype-assistant-line">
    <AssistantMark />
    <span>Aime · {getTaskExecutionType(task.input)} 已为你启动工作流</span>
  </div>
);

const TaskThreadAssistantMessage = ({
  message,
  isRunning = false,
  reasoning,
  streamingMessage,
  tokenUsage,
  tokenUsageViewMode,
}: {
  message?: ThreadTranscriptMessage;
  isRunning?: boolean;
  reasoning?: string;
  streamingMessage?: string;
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageViewMode: TaskTokenUsageViewMode;
}) => {
  const messageResult = message
    ? parseTaskResultPayload(message.content)
    : undefined;
  const answerMessage = messageResult?.message || streamingMessage;
  const inlineReasoning = reasoning || messageResult?.reasoning;

  if (!answerMessage && !inlineReasoning && !tokenUsage && !isRunning) {
    return null;
  }

  return (
    <article className="coze-prototype-answer" data-result-type="answer">
      <TaskInlineReasoning content={inlineReasoning} />
      {answerMessage ? <TaskMarkdownContent value={answerMessage} /> : null}
      {isRunning ? <TaskStreamingIndicator /> : null}
      <TaskMessageTokenUsage
        tokenUsage={tokenUsage}
        viewMode={tokenUsageViewMode}
      />
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
  task: ChatTask;
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
  task,
  tokenUsageByRunID,
  tokenUsageViewMode,
}: {
  artifactActions: TaskArtifactActions;
  artifacts: TaskThreadArtifact[];
  events: TaskEvent[];
  latestTaskRunID: string;
  messages: TaskThreadMessage[];
  task: ChatTask;
  tokenUsageByRunID?: Record<string, TaskDetailTokenUsage>;
  tokenUsageViewMode: TaskTokenUsageViewMode;
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
        <div key={message.message_id || `user-${index}`}>
          <div className="flex justify-end">
            <div className="coze-prototype-user-bubble">{message.content}</div>
          </div>
          <TaskAssistantLine task={task} />
        </div>
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
      <div key={messageKey}>
        {shouldRenderMessageEvents ? (
          <TaskEventsSection events={messageRunEvents} task={task} />
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
          tokenUsage={tokenUsageByRunID?.[runID]}
          tokenUsageViewMode={tokenUsageViewMode}
        />
        <TaskThreadArtifactCards
          artifactActions={artifactActions}
          artifacts={messageArtifacts.after}
          latestPreviewableArtifactID={latestPreviewableArtifactID}
          task={task}
        />
      </div>
    );
  });
  const unmatchedArtifacts = artifacts.filter(
    artifact => !renderedArtifactIDs.has(artifact.artifact_id),
  );

  return (
    <>
      {renderedItems}
      {!latestEventsRendered && latestRunEvents.length ? (
        <TaskEventsSection events={latestRunEvents} task={task} />
      ) : null}
      {!hasLatestAssistantMessage ? (
        <TaskThreadAssistantMessage
          isRunning={latestRunIsActive}
          reasoning={getLatestAnswerEventReasoning(latestRunEvents)}
          streamingMessage={getLatestAnswerEventMessage(latestRunEvents)}
          tokenUsage={tokenUsageByRunID?.[latestRunID]}
          tokenUsageViewMode={tokenUsageViewMode}
        />
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

const TaskEventsSection = ({
  events,
  task,
}: {
  events: TaskEvent[];
  task: ChatTask;
}) => {
  const [showAllSteps, setShowAllSteps] = useState(false);
  const eventItems = projectTaskExecutionEvents(events);
  if (!eventItems.length) {
    return null;
  }

  const hasStructuredItems = eventItems.some(item => item.display.structured);
  const visibleItems = hasStructuredItems
    ? eventItems.filter(item => item.display.structured)
    : eventItems;
  const taskIsTerminal = isTaskTerminalStatus(task.status);
  const displayItems = visibleItems.map(item => {
    if (!taskIsTerminal || item.display.status !== 'running') {
      return item;
    }

    return {
      ...item,
      display: {
        ...item.display,
        status: 'completed' as const,
      },
    };
  });
  const lastActionableIndex = displayItems.reduce(
    (latestIndex, item, index) =>
      item.display.kind === 'thought' ? latestIndex : index,
    -1,
  );
  const collapsedStartIndex =
    lastActionableIndex >= 0
      ? lastActionableIndex
      : Math.max(displayItems.length - 1, 0);
  const hiddenStepCount = collapsedStartIndex;
  const visibleDisplayItems =
    hiddenStepCount > 0 && !showAllSteps
      ? displayItems.slice(collapsedStartIndex)
      : displayItems;
  const isPathDetail = (detail?: string) =>
    detail?.startsWith('/mnt/') || detail?.startsWith('write-file:');
  const renderStepIcon = (display: TaskEventDisplay) => {
    const { key, icon } = getTaskExecutionStepIcon(display);

    return (
      <span
        className="coze-prototype-step-icon"
        data-icon={key}
        data-status={display.status}
        aria-hidden="true"
      >
        {display.status === 'running' ? (
          <span className="coze-prototype-step-running" />
        ) : display.status === 'failed' ? (
          <span className="coze-prototype-step-status-mark">!</span>
        ) : (
          icon
        )}
        <span className="coze-prototype-step-rail" aria-hidden="true" />
      </span>
    );
  };

  return (
    <section
      aria-label="执行流程"
      className="coze-prototype-execution-feed coze-prototype-reasoning-panel"
    >
      <ol className="coze-prototype-execution-feed-list">
        {hiddenStepCount > 0 ? (
          <li>
            <button
              type="button"
              className="coze-prototype-step-more-button"
              aria-expanded={showAllSteps}
              onClick={() => setShowAllSteps(value => !value)}
            >
              <span
                className="coze-prototype-step-more-chevron"
                data-open={showAllSteps}
                aria-hidden="true"
              >
                <IconCozArrowDown />
              </span>
              <span>
                {showAllSteps
                  ? '隐藏步骤'
                  : `查看其他 ${hiddenStepCount} 个步骤`}
              </span>
            </button>
          </li>
        ) : null}
        {visibleDisplayItems.map(({ event, display }) => (
          <li
            key={event.id}
            className="coze-prototype-execution-feed-item coze-prototype-step"
            data-kind={display.kind}
            data-status={display.status}
          >
            {renderStepIcon(display)}
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
                      data-kind={isPathDetail(display.detail) ? 'path' : 'text'}
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
        ))}
      </ol>
    </section>
  );
};

const getTaskExecutionStepIcon = (
  display: TaskEventDisplay,
): { key: string; icon: ReactNode } => {
  if (display.kind === 'thought') {
    return {
      key: 'thought',
      icon: <IconCozLightbulb />,
    };
  }

  const title = display.title.toLowerCase();
  const detail = display.detail?.toLowerCase() ?? '';

  if (title.includes('搜索网页') || title.includes('web_search')) {
    return {
      key: 'search',
      icon: <IconCozMagnifier />,
    };
  }

  if (title.includes('to-do') || title.includes('todo')) {
    return {
      key: 'todo',
      icon: <IconCozDocument />,
    };
  }

  if (
    title.includes('文件') ||
    title.includes('文档') ||
    title.includes('file') ||
    detail.startsWith('/mnt/') ||
    detail.startsWith('write-file:')
  ) {
    return {
      key: 'edit',
      icon: <IconCozEdit />,
    };
  }

  if (title.includes('命令') || title.includes('command')) {
    return {
      key: 'terminal',
      icon: <IconCozCode />,
    };
  }

  return {
    key: 'tool',
    icon: <IconCozPlugin />,
  };
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
  taskDetailSource,
  taskRunActionError,
  taskRunActionLoading,
  tokenUsageByRunID,
  tokenUsageViewMode,
  onHumanInteractionSubmit,
  onRetrySubagentRun,
  onRetryTaskRun,
}: {
  artifacts: workbenchTask.TaskThreadArtifact[];
  events: TaskEvent[];
  humanInteractionError?: string;
  humanInteractionLoading: boolean;
  latestTaskRunID: string;
  messages: TaskThreadMessage[];
  pendingHumanInteraction?: PendingHumanInteraction;
  retryingSubagentRunId?: string;
  subagentRetryError?: string;
  subagentRuns: TaskDetailSubagentRun[];
  task: ChatTask;
  taskDetailSource: LoadedTaskDetailSource;
  taskRunActionError?: string;
  taskRunActionLoading: TaskRunActionLoading;
  tokenUsageByRunID?: Record<string, TaskDetailTokenUsage>;
  tokenUsageViewMode: TaskTokenUsageViewMode;
  onHumanInteractionSubmit: (
    response: HumanInteractionResponse,
  ) => void | Promise<void>;
  onRetrySubagentRun: (runId: string) => void | Promise<void>;
  onRetryTaskRun: (runId: string) => void | Promise<void>;
}) => {
  const artifactActions = useTaskArtifactActions({
    spaceId: task.space_id,
    threadId: taskDetailSource === 'thread' ? task.id : undefined,
  });

  return (
    <section
      className="coze-prototype-chat-transcript"
      data-testid="task-chat-transcript"
    >
      <TaskRunActionBar
        task={task}
        latestRunID={latestTaskRunID}
        loading={taskRunActionLoading}
        error={taskRunActionError}
        taskDetailSource={taskDetailSource}
        onRetryTaskRun={onRetryTaskRun}
      />
      {taskDetailSource === 'thread' && messages.length ? (
        <TaskThreadConversation
          artifactActions={artifactActions}
          artifacts={artifacts}
          events={events}
          latestTaskRunID={latestTaskRunID}
          messages={messages}
          task={task}
          tokenUsageByRunID={tokenUsageByRunID}
          tokenUsageViewMode={tokenUsageViewMode}
        />
      ) : (
        <>
          <TaskConversation task={task} />
          {events.length ||
          parseTaskResultPayload(task.result).resultType === 'agent_trace' ? (
            <TaskEventsSection events={events} task={task} />
          ) : null}
          <TaskResultSection
            task={task}
            events={events}
            tokenUsage={tokenUsageByRunID?.[getLatestAssistantRunID(messages)]}
            tokenUsageViewMode={tokenUsageViewMode}
          />
          {taskDetailSource === 'thread' ? (
            <TaskArtifactMessageList
              artifactActions={artifactActions}
              artifacts={artifacts}
              autoPreview={true}
              renderFeedback={false}
              renderReviewActions={false}
              spaceId={task.space_id}
              threadId={task.id}
            />
          ) : null}
        </>
      )}
      <TaskSubagentRunsSection
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
      {taskDetailSource === 'thread' ? (
        <TaskArtifactFeedback
          clearInlinePreview={artifactActions.clearInlinePreview}
          error={artifactActions.error}
          inlinePreview={artifactActions.inlinePreview}
        />
      ) : null}
    </section>
  );
};

// eslint-disable-next-line @coze-arch/max-line-per-function -- P0 keeps task detail orchestration together.
const TaskDetailPage = () => {
  const { space_id, task_id, thread_id } = useParams();
  const userInfo = useUserInfo();
  const taskDetailId = thread_id ?? task_id;
  const taskDetailSource = getTaskDetailSource(thread_id);
  const {
    applyTaskDetail,
    artifacts,
    error,
    events,
    latestTaskRunID,
    loadedTaskDetailSource,
    loadedThreadId,
    loading,
    messages,
    refreshArtifacts,
    subagentRuns,
    task,
    tokenUsage,
    tokenUsageByRunID,
  } = useTaskDetailData({
    spaceID: space_id,
    taskDetailId,
    taskDetailSource,
  });
  const [tokenUsageViewMode, setTokenUsageViewMode] =
    useState<TaskTokenUsageViewMode>(loadTaskTokenUsageViewMode);

  useEffect(() => {
    saveTaskTokenUsageViewMode(tokenUsageViewMode);
  }, [tokenUsageViewMode]);

  const activeTaskDetailSource = loadedTaskDetailSource;
  const activeTaskDetailId =
    activeTaskDetailSource === 'thread'
      ? loadedThreadId || taskDetailId
      : taskDetailId;
  const pendingHumanInteraction = getPendingHumanInteraction(events);
  const memoryReadOnly = isTaskMemoryReadOnly({
    source: activeTaskDetailSource,
    task,
    userID: userInfo?.user_id_str,
  });
  const {
    followUpError,
    followUpLoading,
    followUpMode,
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
  } = useTaskDetailActions({
    applyTaskDetail,
    artifacts,
    events,
    messages,
    pendingHumanInteraction,
    spaceID: space_id,
    subagentRuns,
    task,
    taskDetailId: activeTaskDetailId,
    taskDetailSource: activeTaskDetailSource,
    tokenUsage,
    tokenUsageByRunID,
  });
  const canStopLatestTaskRun = canStopTaskRunFromComposer({
    latestTaskRunID,
    task,
    taskDetailSource: activeTaskDetailSource,
  });

  return (
    <main className="coze-prototype-page coze-prototype-task-detail-page">
      {task ? (
        <TaskDetailHeader
          artifacts={artifacts}
          memoryReadOnly={memoryReadOnly}
          messages={messages}
          onArtifactsChanged={refreshArtifacts}
          onTokenUsageViewModeChange={setTokenUsageViewMode}
          spaceId={space_id}
          task={task}
          threadId={
            activeTaskDetailSource === 'thread' ? activeTaskDetailId : undefined
          }
          tokenUsage={tokenUsage}
          tokenUsageViewMode={tokenUsageViewMode}
        />
      ) : null}
      <section className="coze-prototype-detail-inner">
        <section
          className="coze-prototype-detail-scroll"
          data-testid="task-detail-scroll"
        >
          {loading ? <TaskDetailLoadingSkeleton /> : null}
          {error ? <div className="coze-prototype-error">{error}</div> : null}
          {!loading && !error && !task ? (
            <div className="coze-prototype-empty">未找到任务</div>
          ) : null}
          {task ? (
            <TaskTranscript
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
              taskDetailSource={activeTaskDetailSource}
              taskRunActionError={taskRunActionError}
              taskRunActionLoading={taskRunActionLoading}
              tokenUsageByRunID={tokenUsageByRunID}
              tokenUsageViewMode={tokenUsageViewMode}
              onHumanInteractionSubmit={handleHumanInteractionSubmit}
              onRetrySubagentRun={handleRetrySubagentRun}
              onRetryTaskRun={handleRetryTaskRun}
            />
          ) : null}
        </section>
        {task ? (
          <TaskFollowUpComposer
            value={followUpValue}
            mode={followUpMode}
            loading={followUpLoading}
            error={followUpError}
            spaceId={space_id}
            taskId={task?.id ?? activeTaskDetailId}
            todoDock={<TaskExecutionTodoDock events={events} task={task} />}
            stopLoading={taskRunActionLoading === 'cancel'}
            stopMode={canStopLatestTaskRun}
            onValueChange={setFollowUpValue}
            onModeChange={setFollowUpMode}
            onStop={() => handleCancelTaskRun(latestTaskRunID)}
            onSubmit={handleFollowUpSubmit}
          />
        ) : null}
      </section>
    </main>
  );
};

export default TaskDetailPage;
