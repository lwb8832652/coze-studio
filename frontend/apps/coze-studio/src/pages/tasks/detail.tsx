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

import type { workbenchTask } from '@coze-studio/api-schema';

import { getTask, listTaskEvents } from './service';
import {
  formatUpdatedTime,
  getTaskStatusText,
  getTaskStatusTone,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskEvent = workbenchTask.TaskEvent;

const getEventText = (event: TaskEvent) =>
  event.payload || event.event_type || '任务事件';

const TaskStatusCard = ({ task }: { task: ChatTask }) => (
  <section className="rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[20px] py-[16px]">
    <div className="flex flex-wrap items-start justify-between gap-[12px]">
      <div className="min-w-0">
        <h1 className="m-0 break-words text-[20px] leading-[28px] font-[600] coz-fg-primary">
          {task.title}
        </h1>
        <div className="mt-[8px] flex flex-wrap gap-[10px] text-[13px] leading-[20px] coz-fg-secondary">
          <span data-status-tone={getTaskStatusTone(task.status)}>
            {getTaskStatusText(task.status)}
          </span>
          <span>更新于 {formatUpdatedTime(task.updated_at)}</span>
        </div>
      </div>
      <div className="text-[20px] leading-[28px] font-[600] text-[#0a8f5a]">
        {task.progress}%
      </div>
    </div>
    <div className="mt-[14px] grid grid-cols-[minmax(0,1fr)_auto] items-center gap-[10px] text-[13px] leading-[20px] coz-fg-secondary">
      <progress className="h-[8px] w-full" value={task.progress} max={100} />
      <span>{task.progress}%</span>
    </div>
  </section>
);

const TaskTextSection = ({
  children,
  title,
}: {
  children: string;
  title: string;
}) => (
  <section className="rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[20px] py-[16px]">
    <h2 className="m-0 text-[15px] leading-[22px] font-[600] coz-fg-primary">
      {title}
    </h2>
    <p className="mt-[10px] mb-0 whitespace-pre-wrap break-words text-[14px] leading-[22px] coz-fg-primary">
      {children}
    </p>
  </section>
);

const TaskEventsSection = ({ events }: { events: TaskEvent[] }) => (
  <section className="rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[20px] py-[16px]">
    <h2 className="m-0 text-[15px] leading-[22px] font-[600] coz-fg-primary">
      执行流程
    </h2>
    <div className="mt-[12px] grid gap-[10px]">
      {events.length > 0 ? (
        events.map(event => (
          <article
            key={event.id}
            className="rounded-[8px] bg-[#f7f7fa] px-[12px] py-[10px]"
          >
            <div className="text-[14px] leading-[22px] coz-fg-primary break-words">
              {getEventText(event)}
            </div>
            <div className="mt-[4px] text-[12px] leading-[18px] coz-fg-secondary">
              {formatUpdatedTime(event.created_at)}
            </div>
          </article>
        ))
      ) : (
        <div className="text-[14px] leading-[22px] coz-fg-secondary">
          暂无执行事件
        </div>
      )}
    </div>
  </section>
);

const FollowUpComposer = () => (
  <section className="sticky bottom-[16px] rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[12px] py-[10px] shadow-[0_8px_24px_rgb(29_28_35_/_8%)]">
    <input
      aria-label="继续追问"
      className="h-[36px] w-full rounded-[6px] border border-solid coz-stroke-primary px-[10px] text-[14px] coz-fg-primary"
      placeholder="继续追问这个任务"
    />
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
    <main className="h-full overflow-auto coz-bg-primary px-[24px] py-[24px]">
      <section className="mx-auto flex w-full max-w-[860px] flex-col gap-[16px]">
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
            <TaskStatusCard task={task} />
            <TaskTextSection title="任务描述">
              {task.input || task.title}
            </TaskTextSection>
            <TaskEventsSection events={events} />
            <TaskTextSection title="执行结果">
              {task.result || task.error || '结果生成中'}
            </TaskTextSection>
            <FollowUpComposer />
          </>
        ) : null}
      </section>
    </main>
  );
};

export default TaskDetailPage;
