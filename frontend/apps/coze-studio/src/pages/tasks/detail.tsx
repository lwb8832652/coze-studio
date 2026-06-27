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
import type { ReactNode } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import {
  IconCozAsynchronousTask,
  IconCozBell,
} from '@coze-arch/coze-design/icons';

import '../../components/workspace-prototype.less';
import '../workbench/index.less';
import { WorkbenchComposer } from '../workbench/components/workbench-composer';
import {
  type WorkbenchComposerSubmitPayload,
  type WorkbenchMode,
} from '../workbench/components/types';
import { TaskTokenUsageIndicator } from './task-token-usage-indicator';
import { TaskSubagentRunsSection } from './task-subagent-runs-section';
import { TaskMemorySection } from './task-memory-section';
import { TaskHumanInterruptCard } from './task-human-interrupt-card';
import { getPendingHumanInteraction } from './task-human-interaction';
import { TaskGuardrailAuditSection } from './task-guardrail-audit-section';
import { TaskRuntimeDoctorSection } from './task-runtime-doctor-section';
import { projectTaskExecutionEvents } from './task-event-projection';
import {
  type TaskDetailSource,
  type TaskDetailTokenUsage,
} from './task-detail-loader';
import { useTaskDetailActions, useTaskDetailData } from './task-detail-hooks';
import { TaskArtifactsPanel } from './task-artifacts-panel';
import {
  formatUpdatedTime,
  getLatestAnswerEventMessage,
  getTaskExecutionType,
  getTaskInputText,
  parseTaskResultPayload,
  type TaskResultPayload,
  getTaskStatusText,
  isTaskTerminalStatus,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;

const getTaskDetailSource = (threadId?: string): TaskDetailSource =>
  threadId ? 'thread' : 'task';

const AssistantMark = () => (
  <span className="coze-prototype-assistant-mark" aria-hidden="true">
    <svg viewBox="0 0 24 24" className="h-[14px] w-[14px]" fill="currentColor">
      <path d="M12 2 2 22h20L12 2zm0 6 6 12H6l6-12z" />
    </svg>
  </span>
);

const TaskTopBar = ({
  artifactAction,
  task,
  tokenUsage,
}: {
  artifactAction?: ReactNode;
  task: ChatTask;
  tokenUsage?: TaskDetailTokenUsage;
}) => (
  <header className="coze-prototype-task-topbar">
    <div className="coze-prototype-task-title-group">
      <IconCozAsynchronousTask className="text-[16px]" />
      <h1 className="coze-prototype-task-top-title">{task.title}</h1>
      <span className="coze-prototype-top-muted">›</span>
      <span className="coze-prototype-top-muted">
        {getTaskStatusText(task.status)}
      </span>
    </div>
    <TaskTokenUsageIndicator tokenUsage={tokenUsage} />
    <div className="flex-1" />
    {artifactAction}
    <button type="button" className="coze-prototype-task-action">
      ☆ 收藏
    </button>
    <button type="button" className="coze-prototype-task-action">
      分享
    </button>
    <button
      type="button"
      className="coze-prototype-icon-button"
      aria-label="通知"
    >
      <IconCozBell className="text-[14px]" />
    </button>
    <div className="coze-prototype-avatar">wb</div>
  </header>
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
  const hasRunningItem = visibleItems.some(
    item => item.display.status === 'running',
  );
  const shouldShowPendingStep =
    !isTaskTerminalStatus(task.status) && !hasRunningItem;
  const doneCount = visibleItems.filter(
    item => item.display.status === 'completed',
  ).length;
  const totalCount = Math.max(
    visibleItems.length + (shouldShowPendingStep ? 1 : 0),
    1,
  );

  return (
    <section className="coze-prototype-execution-panel">
      <div className="coze-prototype-execution-header">
        <span className="text-[14px] leading-[18px] text-[#747b8a]">⌄</span>
        <h2 className="m-0 text-[13px] leading-[20px] font-[500] text-[#232938]">
          执行流程
        </h2>
        <span className="coze-prototype-muted ml-auto">
          {doneCount}/{totalCount} 已完成 · {task.progress}%
        </span>
      </div>
      <ol className="coze-prototype-execution-list">
        {visibleItems.map(({ event, display }) => (
          <li
            key={event.id}
            className="coze-prototype-step"
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
          <li className="coze-prototype-step" data-status="running">
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

const TaskAnswer = ({
  task,
  result,
  streamingMessage,
}: {
  task: ChatTask;
  result: TaskResultPayload;
  streamingMessage?: string;
}) => (
  <article className="coze-prototype-answer" data-result-type="answer">
    <div className="coze-prototype-result-eyebrow">
      普通回答{result.executionType ? ` · ${result.executionType}` : ''}
    </div>
    <p>{result.message || streamingMessage || task.error || '结果生成中'}</p>
    {result.retrievalSources.length ? (
      <div className="coze-prototype-result-sources">
        {result.retrievalSources.map(source => (
          <span key={source}>{source}</span>
        ))}
      </div>
    ) : null}
  </article>
);

const TaskAgentResult = ({
  task,
  result,
}: {
  task: ChatTask;
  result: TaskResultPayload;
}) => (
  <article
    className="coze-prototype-agent-result"
    data-result-type="agent_trace"
  >
    <h2>Agent 最终结果</h2>
    <p>{result.message || task.error || '结果生成中'}</p>
  </article>
);

const TaskReport = ({
  task,
  result,
}: {
  task: ChatTask;
  result: TaskResultPayload;
}) => (
  <article className="coze-prototype-report">
    <h2>{task.title}报告</h2>
    <p>{result.message || task.error || '结果生成中'}</p>

    <h3>一、任务输入</h3>
    <p>{getTaskInputText(task.input) || task.title}</p>
  </article>
);

const TaskResultSection = ({
  task,
  events,
}: {
  task: ChatTask;
  events: TaskEvent[];
}) => {
  const result = parseTaskResultPayload(task.result);

  if (result.resultType === 'report') {
    return <TaskReport task={task} result={result} />;
  }

  if (result.resultType === 'agent_trace') {
    return <TaskAgentResult task={task} result={result} />;
  }

  return (
    <TaskAnswer
      task={task}
      result={result}
      streamingMessage={getLatestAnswerEventMessage(events)}
    />
  );
};

const FollowUpComposer = ({
  value,
  mode,
  loading,
  error,
  taskId,
  onValueChange,
  onModeChange,
  onSubmit,
}: {
  value: string;
  mode: WorkbenchMode;
  loading: boolean;
  error?: string;
  taskId?: string;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void | Promise<void>;
}) => (
  <section className="coze-prototype-followup">
    <WorkbenchComposer
      value={value}
      mode={mode}
      loading={loading}
      error={error}
      variant="detail"
      taskId={taskId}
      onValueChange={onValueChange}
      onModeChange={onModeChange}
      onSubmit={onSubmit}
    />
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
    loading,
    refreshArtifacts,
    subagentRuns,
    task,
    tokenUsage,
  } = useTaskDetailData({
    taskDetailId,
    taskDetailSource,
  });
  const pendingHumanInteraction = getPendingHumanInteraction(events);
  const memoryReadOnly = Boolean(
    taskDetailSource === 'thread' &&
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
    handleRetrySubagentRun,
    humanInteractionError,
    humanInteractionLoading,
    retryingSubagentRunId,
    setFollowUpMode,
    setFollowUpValue,
    subagentRetryError,
  } = useTaskDetailActions({
    applyTaskDetail,
    pendingHumanInteraction,
    spaceID: space_id,
    task,
    taskDetailId,
    taskDetailSource,
  });

  return (
    <main className="coze-prototype-page">
      {task ? (
        <TaskTopBar
          artifactAction={
            taskDetailSource === 'thread' ? (
              <TaskArtifactsPanel
                artifacts={artifacts}
                onArtifactsChanged={refreshArtifacts}
                threadId={taskDetailId}
              />
            ) : undefined
          }
          task={task}
          tokenUsage={tokenUsage}
        />
      ) : null}
      <section className="coze-prototype-detail-inner">
        {loading ? <div className="coze-prototype-empty">加载中...</div> : null}
        {error ? <div className="coze-prototype-error">{error}</div> : null}
        {!loading && !error && !task ? (
          <div className="coze-prototype-empty">未找到任务</div>
        ) : null}
        {task ? (
          <>
            <TaskConversation task={task} />
            {events.length ||
            parseTaskResultPayload(task.result).resultType === 'agent_trace' ? (
              <TaskEventsSection events={events} task={task} />
            ) : null}
            <TaskSubagentRunsSection
              subagentRuns={subagentRuns}
              retryError={subagentRetryError}
              retryingRunId={retryingSubagentRunId}
              onRetrySubagentRun={handleRetrySubagentRun}
            />
            {taskDetailSource === 'thread' ? (
              <TaskRuntimeDoctorSection spaceId={space_id} />
            ) : null}
            {taskDetailSource === 'thread' ? (
              <TaskGuardrailAuditSection threadId={taskDetailId} />
            ) : null}
            {taskDetailSource === 'thread' ? (
              <TaskMemorySection
                readOnly={memoryReadOnly}
                threadId={taskDetailId}
              />
            ) : null}
            {pendingHumanInteraction ? (
              <TaskHumanInterruptCard
                pending={pendingHumanInteraction}
                loading={humanInteractionLoading}
                error={humanInteractionError}
                onSubmit={handleHumanInteractionSubmit}
              />
            ) : null}
            <TaskResultSection task={task} events={events} />
            <FollowUpComposer
              value={followUpValue}
              mode={followUpMode}
              loading={followUpLoading}
              error={followUpError}
              taskId={task?.id ?? taskDetailId}
              onValueChange={setFollowUpValue}
              onModeChange={setFollowUpMode}
              onSubmit={handleFollowUpSubmit}
            />
          </>
        ) : null}
      </section>
    </main>
  );
};

export default TaskDetailPage;
