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

import { getTask, listTaskEvents } from './service';
import { formatUpdatedTime, getTaskStatusText } from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;

const getEventText = (event: TaskEvent) =>
  event.payload || event.event_type || '任务事件';

const AssistantMark = () => (
  <span
    className="flex h-[24px] w-[24px] shrink-0 items-center justify-center rounded-[6px] bg-gradient-to-br from-lime-300 to-green-500 text-white"
    aria-hidden="true"
  >
    △
  </span>
);

const TaskTopBar = ({ task }: { task: ChatTask }) => (
  <header className="flex h-[52px] items-center gap-[12px] border-0 border-b border-solid border-[rgba(77,101,148,0.1)] px-[24px]">
    <span className="text-[16px] leading-[20px] text-[#444c5c]" aria-hidden="true">
      <IconCozAsynchronousTask className="text-[16px]" />
    </span>
    <h1 className="m-0 min-w-0 truncate text-[14px] leading-[20px] font-[500] text-[#232938]">
      {task.title}
    </h1>
    <span className="text-[13px] leading-[18px] text-[#747b8a]">›</span>
    <span className="text-[13px] leading-[18px] text-[#747b8a]">
      {getTaskStatusText(task.status)}
    </span>
    <span className="text-[13px] leading-[18px] text-[#747b8a]">
      {task.progress}%
    </span>
    <div className="flex-1" />
    <button
      type="button"
      className="h-[28px] rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white px-[10px] text-[13px] text-[#444c5c]"
    >
      ☆ 收藏
    </button>
    <button
      type="button"
      className="h-[28px] rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white px-[10px] text-[13px] text-[#444c5c]"
    >
      分享
    </button>
    <button
      type="button"
      className="flex h-[28px] w-[28px] items-center justify-center rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white text-[#444c5c]"
      aria-label="通知"
    >
      <IconCozBell className="text-[14px]" />
    </button>
    <div className="flex h-[28px] w-[28px] items-center justify-center rounded-full bg-gradient-to-br from-orange-300 to-pink-400 text-[12px] leading-[16px] text-white">
      wb
    </div>
  </header>
);

const TaskConversation = ({ task }: { task: ChatTask }) => (
  <>
    <div className="flex justify-end">
      <div className="max-w-[80%] rounded-[16px] rounded-tr-[4px] bg-[rgba(91,100,117,0.06)] px-[16px] py-[12px] text-[14px] leading-[22px] text-[#232938]">
        {task.input || task.title}
      </div>
    </div>

    <div className="flex items-center gap-[8px] text-[13px] leading-[20px] text-[#444c5c]">
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
    <section className="overflow-hidden rounded-[12px] border border-solid border-[rgba(77,101,148,0.15)] bg-[#fafbfc]">
      <div className="flex items-center gap-[8px] border-0 border-b border-solid border-[rgba(77,101,148,0.1)] px-[16px] py-[12px]">
        <span className="text-[14px] leading-[18px] text-[#747b8a]">⌄</span>
        <h2 className="m-0 text-[13px] leading-[20px] font-[500] text-[#232938]">
          执行流程
        </h2>
        <span className="ml-auto text-[12px] leading-[18px] text-[#747b8a]">
          {doneCount}/{totalCount} 已完成 · {task.progress}%
        </span>
      </div>
      <ol className="m-0 grid list-none gap-[10px] px-[16px] py-[14px]">
        {events.map(event => (
          <li key={event.id} className="flex items-center gap-[8px] text-[13px]">
            <span className="flex h-[16px] w-[16px] shrink-0 items-center justify-center rounded-full bg-[#2a9e06] text-[11px] leading-[16px] text-white">
              ✓
            </span>
            <span className="min-w-0 flex-1 truncate text-[#444c5c]">
              {getEventText(event)}
            </span>
            <span className="shrink-0 text-[12px] text-[#747b8a]">
              {formatUpdatedTime(event.created_at)}
            </span>
          </li>
        ))}
        {task.progress < 100 ? (
          <li className="flex items-center gap-[8px] text-[13px]">
            <span className="h-[16px] w-[16px] shrink-0 rounded-full border-2 border-solid border-[#2a9e06]" />
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
  <article className="text-[14px] leading-[28px] text-[#232938]">
    <h2 className="m-0 text-[20px] leading-[28px] font-[600] text-[#1d2129]">
      {task.title}报告
    </h2>
    <p className="mt-[10px] mb-0 whitespace-pre-wrap text-[#444c5c]">
      {task.result || task.error || '结果生成中'}
    </p>

    <h3 className="mt-[28px] mb-[8px] text-[16px] leading-[24px] font-[600] text-[#1d2129]">
      一、任务输入
    </h3>
    <p className="m-0 whitespace-pre-wrap text-[#232938]">
      {task.input || task.title}
    </p>
  </article>
);

const FollowUpComposer = () => (
  <section className="sticky bottom-0 bg-gradient-to-t from-white via-white to-transparent pt-[24px] pb-[16px]">
    <div className="rounded-[16px] border border-solid border-[rgba(77,101,148,0.2)] bg-white p-[12px] shadow-[0_4px_24px_rgba(15,23,42,0.1)]">
      <div className="flex items-center gap-[8px]">
        <input
          aria-label="继续追问"
          className="h-[32px] min-w-0 flex-1 border-0 bg-transparent px-[8px] text-[14px] text-[#232938] outline-none"
          placeholder="继续追问..."
        />
        <button
          type="button"
          className="h-[28px] w-[28px] rounded-[6px] border-0 bg-transparent text-[#444c5c]"
        >
          🔗
        </button>
        <button
          type="button"
          className="h-[28px] w-[28px] rounded-[6px] border-0 bg-transparent text-[#444c5c]"
        >
          🎙
        </button>
        <button
          type="button"
          className="h-[32px] w-[32px] rounded-[8px] border-0 bg-gradient-to-br from-[#7dff84] to-[#3dbd3d] text-white"
        >
          ↑
        </button>
      </div>
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
    <main className="h-full overflow-auto bg-white">
      {task ? <TaskTopBar task={task} /> : null}
      <section className="mx-auto flex w-full max-w-[860px] flex-col gap-[24px] px-[24px] pt-[32px] pb-[120px]">
        {loading ? (
          <div className="rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[20px] py-[28px] text-center text-[14px] coz-fg-secondary">
            加载中...
          </div>
        ) : null}

        {error ? (
          <div className="rounded-[8px] border border-solid border-[#ffd4cc] bg-[#fff1ee] px-[12px] py-[10px] text-[14px] leading-[20px] text-[#c02a1d] break-words">
            {error}
          </div>
        ) : null}

        {!loading && !error && !task ? (
          <div className="rounded-[8px] border border-dashed coz-stroke-primary px-[16px] py-[32px] text-center text-[14px] coz-fg-secondary">
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
