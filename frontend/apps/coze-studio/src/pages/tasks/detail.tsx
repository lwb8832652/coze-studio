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
  IconCozLink,
  IconCozMicrophone,
  IconCozSendFill,
} from '@coze-arch/coze-design/icons';
import type { workbenchTask } from '@coze-studio/api-schema';

import '../../components/workspace-prototype.less';
import { getTask, listTaskEvents } from './service';
import {
  formatUpdatedTime,
  getTaskEventText,
  getTaskInputText,
  getTaskResultText,
  getTaskStatusText,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;

const getEventText = (event: TaskEvent) =>
  getTaskEventText(event.event_type, event.payload);

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

const TaskConversation = ({ task }: { task: ChatTask }) => (
  <>
    <div className="flex justify-end">
      <div className="coze-prototype-user-bubble">
        {getTaskInputText(task.input) || task.title}
      </div>
    </div>

    <div className="coze-prototype-assistant-line">
      <AssistantMark />
      <span>Aime · 已为你启动 Agent 工作流</span>
    </div>
  </>
);

const TaskEventsSection = ({
  events,
  task,
}: {
  events: TaskEvent[];
  task: ChatTask;
}) => {
  const doneCount = events.length;
  const totalCount = Math.max(doneCount + (task.progress < 100 ? 1 : 0), 1);

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
        {events.map(event => (
          <li key={event.id} className="coze-prototype-step">
            <span className="coze-prototype-step-check">
              ✓
            </span>
            <span className="min-w-0 flex-1 truncate text-[#444c5c]">
              {getEventText(event)}
            </span>
            <span className="coze-prototype-muted shrink-0">
              {formatUpdatedTime(event.created_at)}
            </span>
          </li>
        ))}
        {task.progress < 100 ? (
          <li className="coze-prototype-step">
            <span className="coze-prototype-step-running" />
            <span className="text-[#232938]">汇总分析结果,生成结构化报告</span>
            <span className="ml-[4px] text-[11px] text-[#2a9e06]">
              进行中...
            </span>
          </li>
        ) : null}
      </ol>
    </section>
  );
};

const TaskReport = ({ task }: { task: ChatTask }) => (
  <article className="coze-prototype-report">
    <h2>
      {task.title}报告
    </h2>
    <p>
      {getTaskResultText(task.result) || task.error || '结果生成中'}
    </p>

    <h3>一、任务输入</h3>
    <p>
      {getTaskInputText(task.input) || task.title}
    </p>
  </article>
);

const FollowUpComposer = () => (
  <section className="coze-prototype-followup">
    <div className="coze-prototype-followup-box">
        <input
          aria-label="继续追问"
          placeholder="继续追问..."
        />
        <button
          type="button"
          className="coze-prototype-followup-icon"
        >
          <IconCozLink className="text-[16px]" />
        </button>
        <button
          type="button"
          className="coze-prototype-followup-icon"
        >
          <IconCozMicrophone className="text-[16px]" />
        </button>
        <button
          type="button"
          className="coze-prototype-followup-send"
        >
          <IconCozSendFill className="text-[16px]" />
        </button>
    </div>
  </section>
);

const TaskDetailPage = () => {
  const { task_id } = useParams();
  const [task, setTask] = useState<ChatTask | undefined>();
  const [events, setEvents] = useState<TaskEvent[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!task_id) {
      return;
    }

    let canceled = false;

    const loadTaskDetail = async () => {
      setLoading(true);
      setError('');

      try {
        const [taskResponse, eventsResponse] = await Promise.all([
          getTask({ task_id }),
          listTaskEvents({ task_id }),
        ]);

        if (!canceled) {
          setTask(taskResponse.data);
          setEvents(eventsResponse.data?.events ?? []);
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

    void loadTaskDetail();

    return () => {
      canceled = true;
    };
  }, [task_id]);

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
            <TaskEventsSection events={events} task={task} />
            <TaskReport task={task} />
            <FollowUpComposer />
          </>
        ) : null}
      </section>
    </main>
  );
};

export default TaskDetailPage;
