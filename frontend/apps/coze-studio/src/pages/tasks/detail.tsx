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

import { useParams } from 'react-router-dom';
import { useEffect, useState } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';
import { useUserInfo } from '@coze-arch/foundation-sdk';

import '../../components/workspace-prototype.less';
import '../workbench/index.less';
import { TaskSubagentRunsSection } from './task-subagent-runs-section';
import {
  TaskRunActionBar,
  type TaskRunActionLoading,
} from './task-run-action-bar';
import { TaskResultSection } from './task-result-section';
import {
  getLatestAssistantRunID,
  loadTaskTokenUsageViewMode,
  saveTaskTokenUsageViewMode,
} from './task-message-token-usage';
import { TaskHumanInterruptCard } from './task-human-interrupt-card';
import {
  getPendingHumanInteraction,
  type PendingHumanInteraction,
} from './task-human-interaction';
import { TaskFollowUpComposer } from './task-follow-up-composer';
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
import { TaskArtifactMessageList } from './task-artifact-message-list';
import {
  formatUpdatedTime,
  getTaskExecutionType,
  getTaskInputText,
  parseTaskResultPayload,
  isTaskTerminalStatus,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;
type HumanInteractionResponse = workbenchTask.HumanInteractionResponse;
type TaskThreadMessage = workbenchTask.TaskThreadMessage;

const getTaskDetailSource = (threadId?: string): TaskDetailSource =>
  threadId ? 'thread' : 'auto';

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

const TaskEventsSection = ({
  events,
  task,
}: {
  events: TaskEvent[];
  task: ChatTask;
}) => {
  const eventItems = projectTaskExecutionEvents(events);
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
  const hasRunningItem = displayItems.some(
    item => item.display.status === 'running',
  );
  const shouldShowPendingStep =
    !isTaskTerminalStatus(task.status) && !hasRunningItem;
  const doneCount = displayItems.filter(
    item => item.display.status === 'completed',
  ).length;
  const totalCount = Math.max(
    displayItems.length + (shouldShowPendingStep ? 1 : 0),
    1,
  );

  return (
    <section className="coze-prototype-execution-feed coze-prototype-reasoning-panel">
      <div className="coze-prototype-execution-feed-header">
        <h2 className="coze-prototype-reasoning-title">执行流程</h2>
        <span className="coze-prototype-muted ml-auto">
          {doneCount}/{totalCount} 已完成 · {task.progress}%
        </span>
      </div>
      <ol className="coze-prototype-execution-feed-list">
        {displayItems.map(({ event, display }) => (
          <li
            key={event.id}
            className="coze-prototype-execution-feed-item coze-prototype-step"
            data-kind={display.kind}
            data-status={display.status}
          >
            {display.status === 'running' ? (
              <span className="coze-prototype-step-running" />
            ) : (
              <span className="coze-prototype-step-check">
                {display.status === 'failed' ? '!' : '✓'}
              </span>
            )}
            <span className="coze-prototype-step-content">
              <span className="coze-prototype-step-title-row">
                <span className="coze-prototype-step-title">
                  {display.title}
                </span>
                {display.runtime ? (
                  <span className="coze-prototype-step-runtime">
                    {display.runtime}
                  </span>
                ) : null}
              </span>
              {display.detail ? (
                <span className="coze-prototype-step-detail">
                  {display.detail}
                </span>
              ) : null}
              {display.thought ? (
                <span className="coze-prototype-step-thought">
                  {display.thought}
                </span>
              ) : null}
            </span>
            <span className="coze-prototype-muted shrink-0">
              {formatUpdatedTime(event.created_at)}
            </span>
          </li>
        ))}
        {shouldShowPendingStep ? (
          <li
            className="coze-prototype-execution-feed-item coze-prototype-step"
            data-status="running"
          >
            <span className="coze-prototype-step-running" />
            <span className="coze-prototype-step-content">
              <span className="coze-prototype-step-title">
                等待任务执行结果
              </span>
              <span className="coze-prototype-step-detail">
                后端执行器正在写入流程事件
              </span>
            </span>
            <span className="ml-[4px] text-[11px] text-[#2a9e06]">
              进行中...
            </span>
          </li>
        ) : null}
      </ol>
    </section>
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
  taskDetailSource,
  taskRunActionError,
  taskRunActionLoading,
  tokenUsageByRunID,
  tokenUsageViewMode,
  onCancelTaskRun,
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
  onCancelTaskRun: (runId: string) => void | Promise<void>;
  onHumanInteractionSubmit: (
    response: HumanInteractionResponse,
  ) => void | Promise<void>;
  onRetrySubagentRun: (runId: string) => void | Promise<void>;
  onRetryTaskRun: (runId: string) => void | Promise<void>;
}) => (
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
      onCancelTaskRun={onCancelTaskRun}
      onRetryTaskRun={onRetryTaskRun}
    />
    <TaskConversation task={task} />
    {events.length ||
    parseTaskResultPayload(task.result).resultType === 'agent_trace' ? (
      <TaskEventsSection events={events} task={task} />
    ) : null}
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
    <TaskResultSection
      task={task}
      events={events}
      tokenUsage={tokenUsageByRunID?.[getLatestAssistantRunID(messages)]}
      tokenUsageViewMode={tokenUsageViewMode}
    />
    {taskDetailSource === 'thread' ? (
      <TaskArtifactMessageList artifacts={artifacts} threadId={task.id} />
    ) : null}
  </section>
);

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
  const memoryReadOnly = Boolean(
    activeTaskDetailSource === 'thread' &&
      task?.creator_id &&
      userInfo?.user_id_str &&
      task.creator_id !== userInfo.user_id_str,
  );
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
    pendingHumanInteraction,
    spaceID: space_id,
    task,
    taskDetailId: activeTaskDetailId,
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
          {loading ? (
            <div className="coze-prototype-empty">加载中...</div>
          ) : null}
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
              onCancelTaskRun={handleCancelTaskRun}
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
            onValueChange={setFollowUpValue}
            onModeChange={setFollowUpMode}
            onSubmit={handleFollowUpSubmit}
          />
        ) : null}
      </section>
    </main>
  );
};

export default TaskDetailPage;
