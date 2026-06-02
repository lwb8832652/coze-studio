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

import { workbenchTask } from '@coze-studio/api-schema';

import { cancelTask, listTasks, retryTask } from './service';

type ChatTask = workbenchTask.ChatTask;

const getTaskStatusText = (status: workbenchTask.TaskStatus) => {
  const statusMap: Record<workbenchTask.TaskStatus, string> = {
    [workbenchTask.TaskStatus.Created]: '已创建',
    [workbenchTask.TaskStatus.Queued]: '排队中',
    [workbenchTask.TaskStatus.Running]: '运行中',
    [workbenchTask.TaskStatus.Succeeded]: '已完成',
    [workbenchTask.TaskStatus.Failed]: '失败',
    [workbenchTask.TaskStatus.Canceling]: '取消中',
    [workbenchTask.TaskStatus.Canceled]: '已取消',
  };

  return statusMap[status] ?? '未知';
};

export const canCancelTask = (status: workbenchTask.TaskStatus) =>
  status === workbenchTask.TaskStatus.Created ||
  status === workbenchTask.TaskStatus.Queued ||
  status === workbenchTask.TaskStatus.Running;

const canRetryTask = (status: workbenchTask.TaskStatus) =>
  status === workbenchTask.TaskStatus.Failed;

export const formatUpdatedTime = (timestamp: number) => {
  if (!timestamp) {
    return '-';
  }

  return new Date(timestamp).toLocaleString();
};

const TasksPage = () => {
  const { space_id } = useParams();
  const [tasks, setTasks] = useState<ChatTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [actionTaskId, setActionTaskId] = useState('');

  const loadTasks = async () => {
    if (!space_id) {
      return;
    }

    setLoading(true);
    setError('');

    try {
      const response = await listTasks({ space_id });
      setTasks(response.data?.tasks ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载任务失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadTasks();
  }, [space_id]);

  const runTaskAction = async (
    task: ChatTask,
    action: typeof cancelTask | typeof retryTask,
  ) => {
    if (actionTaskId) {
      return;
    }

    setActionTaskId(task.id);
    setError('');

    try {
      await action({ task_id: task.id });
      await loadTasks();
    } catch (err) {
      setError(err instanceof Error ? err.message : '任务操作失败');
    } finally {
      setActionTaskId('');
    }
  };

  return (
    <main className="h-full px-[24px] py-[24px] coz-bg-primary overflow-auto">
      <section className="rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[24px] py-[20px]">
        <div className="flex flex-wrap items-center justify-between gap-[12px]">
          <h1 className="m-0 text-[20px] leading-[28px] font-[600] coz-fg-primary">
            全部任务
          </h1>
          <button
            type="button"
            className="h-[32px] rounded-[6px] border border-solid coz-stroke-primary px-[12px] text-[14px] coz-fg-primary coz-bg-plus"
            disabled={loading || !space_id}
            onClick={loadTasks}
          >
            刷新
          </button>
        </div>

        {error ? (
          <div className="mt-[16px] rounded-[8px] border border-solid border-[#ffd4cc] bg-[#fff1ee] px-[12px] py-[10px] text-[14px] leading-[20px] text-[#c02a1d] break-words">
            {error}
          </div>
        ) : null}

        <section className="mt-[20px]" aria-label="任务列表">
          {loading ? (
            <div className="py-[32px] text-center text-[14px] coz-fg-secondary">
              加载中...
            </div>
          ) : null}

          {!loading && tasks.length === 0 ? (
            <div className="rounded-[8px] border border-dashed coz-stroke-primary px-[16px] py-[32px] text-center text-[14px] coz-fg-secondary">
              暂无任务
            </div>
          ) : null}

          <div className="grid gap-[12px]">
            {tasks.map(task => (
              <article
                key={task.id}
                className="rounded-[8px] border border-solid coz-stroke-primary px-[16px] py-[14px]"
              >
                <div className="flex flex-wrap items-start justify-between gap-[12px]">
                  <div className="min-w-0">
                    <h2 className="m-0 break-words text-[16px] leading-[24px] font-[600] coz-fg-primary">
                      {task.title}
                    </h2>
                    <div className="mt-[6px] flex flex-wrap gap-[8px] text-[13px] leading-[20px] coz-fg-secondary">
                      <span>{getTaskStatusText(task.status)}</span>
                      <span>{task.progress}%</span>
                      <span>更新于 {formatUpdatedTime(task.updated_at)}</span>
                    </div>
                  </div>
                  <div className="flex flex-wrap gap-[8px]">
                    {canCancelTask(task.status) ? (
                      <button
                        type="button"
                        className="min-h-[32px] rounded-[6px] border border-solid coz-stroke-primary px-[12px] text-[14px] coz-fg-primary coz-bg-plus disabled:opacity-50"
                        disabled={Boolean(actionTaskId)}
                        onClick={() => runTaskAction(task, cancelTask)}
                      >
                        {actionTaskId === task.id ? '处理中' : '取消'}
                      </button>
                    ) : null}
                    {canRetryTask(task.status) ? (
                      <button
                        type="button"
                        className="min-h-[32px] rounded-[6px] border border-solid coz-stroke-primary px-[12px] text-[14px] coz-fg-primary coz-bg-plus disabled:opacity-50"
                        disabled={Boolean(actionTaskId)}
                        onClick={() => runTaskAction(task, retryTask)}
                      >
                        {actionTaskId === task.id ? '处理中' : '重试'}
                      </button>
                    ) : null}
                  </div>
                </div>
                <div className="mt-[12px] grid grid-cols-[minmax(0,1fr)_auto] items-center gap-[10px] text-[13px] leading-[20px] coz-fg-secondary">
                  <progress className="h-[8px] w-full" value={task.progress} max={100} />
                  <span>{task.progress}%</span>
                </div>
              </article>
            ))}
          </div>
        </section>
      </section>
    </main>
  );
};

export default TasksPage;
