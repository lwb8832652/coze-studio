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

import { useEffect, useState } from 'react';
import { useParams } from 'react-router-dom';

import {
  IconCozAsynchronousTask,
  IconCozBell,
} from '@coze-arch/coze-design/icons';
import type { workbenchTask } from '@coze-studio/api-schema';

import '../../components/workspace-prototype.less';
import '../workbench/index.less';
import { WorkbenchComposer } from '../workbench/components/workbench-composer';
import {
  mapModeToChatMode,
  type WorkbenchComposerSubmitPayload,
  type WorkbenchMode,
} from '../workbench/components/types';
import { getTask, listTaskEvents, sendWorkbenchChat } from './service';
import {
  formatUpdatedTime,
  getLatestAnswerEventMessage,
  getTaskEventDisplay,
  getTaskExecutionType,
  getTaskInputText,
  parseTaskResultPayload,
  type TaskResultPayload,
  getTaskStatusText,
  isTaskTerminalStatus,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;

const fetchTaskDetail = async (taskId: string) => {
  const [taskResponse, eventsResponse] = await Promise.all([
    getTask({ task_id: taskId }),
    listTaskEvents({ task_id: taskId }),
  ]);

  return {
    task: taskResponse.data,
    events: eventsResponse.data?.events ?? [],
  };
};

const AssistantMark = () => (
  <span className="coze-prototype-assistant-mark" aria-hidden="true">
    <svg viewBox="0 0 24 24" className="h-[14px] w-[14px]" fill="currentColor">
      <path d="M12 2 2 22h20L12 2zm0 6 6 12H6l6-12z" />
    </svg>
  </span>
);

const TaskTopBar = ({ task }: { task: ChatTask }) => (
  <header className="coze-prototype-task-topbar">
    <div className="coze-prototype-task-title-group">
      <IconCozAsynchronousTask className="text-[16px]" />
      <h1 className="coze-prototype-task-top-title">{task.title}</h1>
      <span className="coze-prototype-top-muted">›</span>
      <span className="coze-prototype-top-muted">
        {getTaskStatusText(task.status)}
      </span>
    </div>
    <div className="flex-1" />
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
  const eventItems = events.map(event => ({
    event,
    display: getTaskEventDisplay(event.event_type, event.payload),
  }));
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
        <h2 className="m-0 text-[13px] leading-[20px] font-[500] text-[#232938]">执行流程</h2>
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
                <span className="coze-prototype-step-title">{display.title}</span>
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
              <span className="coze-prototype-step-title">等待任务执行结果</span>
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
  <article className="coze-prototype-agent-result" data-result-type="agent_trace">
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
  const { space_id, task_id } = useParams();
  const [task, setTask] = useState<ChatTask | undefined>();
  const [events, setEvents] = useState<TaskEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [followUpValue, setFollowUpValue] = useState('');
  const [followUpMode, setFollowUpMode] = useState<WorkbenchMode>('Auto');
  const [followUpLoading, setFollowUpLoading] = useState(false);
  const [followUpError, setFollowUpError] = useState('');

  useEffect(() => {
    if (!task_id) {
      return;
    }

    let canceled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const loadTaskDetail = async (showLoading = false) => {
      if (showLoading) {
        setLoading(true);
      }
      setError('');

      try {
        const detail = await fetchTaskDetail(task_id);

        if (!canceled) {
          setTask(detail.task);
          setEvents(detail.events);

          if (detail.task && !isTaskTerminalStatus(detail.task.status)) {
            timer = setTimeout(() => {
              void loadTaskDetail();
            }, 2000);
          }
        }
      } catch (err) {
        if (!canceled) {
          setError(err instanceof Error ? err.message : '加载任务详情失败');
        }
      } finally {
        if (!canceled) {
          setLoading(false);
        }
      }
    };

    void loadTaskDetail(true);

    return () => {
      canceled = true;
      if (timer) {
        clearTimeout(timer);
      }
    };
  }, [task_id]);

  const handleFollowUpSubmit = async (
    payload: WorkbenchComposerSubmitPayload,
  ) => {
    if (!payload.message || followUpLoading) {
      return;
    }

    if (!space_id || !task_id) {
      setFollowUpError('缺少任务上下文，无法继续追问');

      return;
    }

    setFollowUpLoading(true);
    setFollowUpError('');

    try {
      await sendWorkbenchChat({
        space_id,
        task_id,
        message: payload.message,
        mode: mapModeToChatMode(payload.mode),
        enable_skills: payload.enable_skills,
        enable_mcp: payload.enable_mcp,
        enable_kbs: payload.enable_kbs,
        enable_databases: payload.enable_databases,
      });

      setFollowUpValue('');

      const detail = await fetchTaskDetail(task_id);
      setTask(detail.task);
      setEvents(detail.events);
    } catch (err) {
      setFollowUpError(
        err instanceof Error ? err.message : '继续追问失败，请稍后重试',
      );
    } finally {
      setFollowUpLoading(false);
    }
  };

  return (
    <main className="coze-prototype-page">
      {task ? <TaskTopBar task={task} /> : null}
      <section className="coze-prototype-detail-inner">
        {loading ? (
          <div className="coze-prototype-empty">
            加载中...
          </div>
        ) : null}

        {error ? (
          <div className="coze-prototype-error">{error}</div>
        ) : null}

        {!loading && !error && !task ? (
          <div className="coze-prototype-empty">
            未找到任务
          </div>
        ) : null}

        {task ? (
          <>
            <TaskConversation task={task} />
            {parseTaskResultPayload(task.result).resultType ===
            'agent_trace' ? (
              <TaskEventsSection events={events} task={task} />
            ) : null}
            <TaskResultSection task={task} events={events} />
            <FollowUpComposer
              value={followUpValue}
              mode={followUpMode}
              loading={followUpLoading}
              error={followUpError}
              taskId={task_id}
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
