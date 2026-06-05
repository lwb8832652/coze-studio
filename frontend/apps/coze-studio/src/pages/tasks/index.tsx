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
import { useNavigate, useParams } from 'react-router-dom';

import type { workbenchTask } from '@coze-studio/api-schema';

import { cancelTask, listTasks, retryTask } from './service';
import {
  canCancelTask,
  canRetryTask,
  filterTasks,
  formatUpdatedTime,
  getTaskStatusTone,
  getTaskStatusText,
  type TaskStatusFilter,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;

const STATUS_FILTERS: Array<{ label: string; value: TaskStatusFilter }> = [
  { label: '全部状态', value: 'all' },
  { label: '运行中', value: 'running' },
  { label: '已完成', value: 'succeeded' },
  { label: '失败', value: 'failed' },
];

interface TasksHeaderProps {
  loading: boolean;
  spaceId?: string;
  onRefresh: () => void;
}

const TasksHeader = ({ loading, spaceId, onRefresh }: TasksHeaderProps) => (
  <div className="flex flex-wrap items-start justify-between gap-[16px]">
    <div className="min-w-0">
      <h1 className="m-0 text-[24px] leading-[32px] font-[600] coz-fg-primary">
        全部任务
      </h1>
      <p className="mt-[6px] mb-0 text-[14px] leading-[22px] coz-fg-secondary">
        跨任务追踪执行状态、产物和历史记录
      </p>
    </div>
    <button
      type="button"
      className="h-[34px] rounded-[6px] border border-solid coz-stroke-primary px-[12px] text-[14px] coz-fg-primary coz-bg-plus disabled:opacity-50"
      disabled={loading || !spaceId}
      onClick={onRefresh}
    >
      刷新
    </button>
  </div>
);

interface TasksToolbarProps {
  keyword: string;
  statusFilter: TaskStatusFilter;
  view: 'all' | 'favorite';
  onKeywordChange: (value: string) => void;
  onStatusFilterChange: (value: TaskStatusFilter) => void;
  onViewChange: (value: 'all' | 'favorite') => void;
}

const TasksToolbar = ({
  keyword,
  statusFilter,
  view,
  onKeywordChange,
  onStatusFilterChange,
  onViewChange,
}: TasksToolbarProps) => (
  <section className="mt-[20px] rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[16px] py-[14px]">
    <div className="flex flex-wrap items-center gap-[12px]">
      <label className="flex min-w-[240px] flex-1 items-center gap-[8px] rounded-[6px] border border-solid coz-stroke-primary px-[10px] py-[7px]">
        <span className="shrink-0 text-[13px] leading-[20px] coz-fg-secondary">
          搜索任务
        </span>
        <input
          aria-label="搜索任务"
          className="min-w-0 flex-1 border-0 bg-transparent text-[14px] leading-[20px] outline-none coz-fg-primary"
          value={keyword}
          onChange={event => onKeywordChange(event.target.value)}
          placeholder="输入任务名称或描述"
        />
      </label>

      <select
        aria-label="任务状态"
        className="h-[36px] rounded-[6px] border border-solid coz-stroke-primary px-[10px] text-[14px] coz-fg-primary coz-bg-plus"
        value={statusFilter}
        onChange={event =>
          onStatusFilterChange(event.target.value as TaskStatusFilter)
        }
      >
        {STATUS_FILTERS.map(item => (
          <option key={item.value} value={item.value}>
            {item.label}
          </option>
        ))}
      </select>

      <div className="grid h-[36px] grid-cols-2 rounded-[6px] border border-solid coz-stroke-primary bg-[#f7f7fa] p-[2px]">
        {[
          { label: '全部', value: 'all' as const },
          { label: '已收藏', value: 'favorite' as const },
        ].map(item => (
          <button
            key={item.value}
            type="button"
            className="rounded-[4px] border-0 bg-transparent px-[12px] text-[14px] coz-fg-secondary data-[active=true]:coz-bg-plus data-[active=true]:coz-fg-primary"
            data-active={view === item.value}
            onClick={() => onViewChange(item.value)}
          >
            {item.label}
          </button>
        ))}
      </div>
    </div>
  </section>
);

interface TaskRowProps {
  actionTaskId: string;
  favorite: boolean;
  spaceId?: string;
  task: ChatTask;
  onAction: (task: ChatTask, action: typeof cancelTask | typeof retryTask) => void;
  onNavigate: (task: ChatTask) => void;
  onToggleFavorite: (taskId: string) => void;
}

const TaskRow = ({
  actionTaskId,
  favorite,
  task,
  onAction,
  onNavigate,
  onToggleFavorite,
}: TaskRowProps) => (
  <article className="rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus px-[16px] py-[14px] transition-colors hover:coz-mg-secondary-hovered">
    <div className="flex flex-wrap items-center justify-between gap-[12px]">
      <div className="min-w-0">
        <button
          type="button"
          className="block min-w-0 border-0 bg-transparent p-0 text-left cursor-pointer"
          onClick={() => onNavigate(task)}
        >
          <h2 className="m-0 break-words text-[15px] leading-[22px] font-[600] coz-fg-primary">
            {task.title}
          </h2>
        </button>
        <div className="mt-[6px] flex flex-wrap items-center gap-[8px] text-[13px] leading-[20px] coz-fg-secondary">
          <span data-status-tone={getTaskStatusTone(task.status)}>
            {getTaskStatusText(task.status)}
          </span>
          <span>{task.progress}%</span>
          <span>更新于 {formatUpdatedTime(task.updated_at)}</span>
        </div>
      </div>
      <div className="flex flex-wrap gap-[8px]">
        <button
          type="button"
          className="min-h-[32px] rounded-[6px] border border-solid coz-stroke-primary px-[10px] text-[14px] coz-fg-primary coz-bg-plus"
          aria-pressed={favorite}
          onClick={() => onToggleFavorite(task.id)}
        >
          {favorite ? '已收藏' : '收藏'}
        </button>
        {canCancelTask(task.status) ? (
          <button
            type="button"
            className="min-h-[32px] rounded-[6px] border border-solid coz-stroke-primary px-[12px] text-[14px] coz-fg-primary coz-bg-plus disabled:opacity-50"
            disabled={Boolean(actionTaskId)}
            onClick={() => onAction(task, cancelTask)}
          >
            {actionTaskId === task.id ? '处理中' : '取消'}
          </button>
        ) : null}
        {canRetryTask(task.status) ? (
          <button
            type="button"
            className="min-h-[32px] rounded-[6px] border border-solid coz-stroke-primary px-[12px] text-[14px] coz-fg-primary coz-bg-plus disabled:opacity-50"
            disabled={Boolean(actionTaskId)}
            onClick={() => onAction(task, retryTask)}
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
);

interface TaskListProps {
  actionTaskId: string;
  favoriteTaskIds: string[];
  loading: boolean;
  spaceId?: string;
  tasks: ChatTask[];
  totalTasks: number;
  onAction: (task: ChatTask, action: typeof cancelTask | typeof retryTask) => void;
  onNavigate: (task: ChatTask) => void;
  onToggleFavorite: (taskId: string) => void;
}

const TaskList = ({
  actionTaskId,
  favoriteTaskIds,
  loading,
  spaceId,
  tasks,
  totalTasks,
  onAction,
  onNavigate,
  onToggleFavorite,
}: TaskListProps) => (
  <section className="mt-[16px]" aria-label="任务列表">
    {loading ? (
      <div className="rounded-[8px] border border-solid coz-stroke-primary coz-bg-plus py-[32px] text-center text-[14px] coz-fg-secondary">
        加载中...
      </div>
    ) : null}

    {!loading && tasks.length === 0 ? (
      <div className="rounded-[8px] border border-dashed coz-stroke-primary coz-bg-plus px-[16px] py-[32px] text-center text-[14px] coz-fg-secondary">
        {totalTasks === 0 ? '暂无任务' : '没有匹配的任务'}
      </div>
    ) : null}

    <div className="grid gap-[8px]">
      {tasks.map(task => (
        <TaskRow
          key={task.id}
          actionTaskId={actionTaskId}
          favorite={favoriteTaskIds.includes(task.id)}
          spaceId={spaceId}
          task={task}
          onAction={onAction}
          onNavigate={onNavigate}
          onToggleFavorite={onToggleFavorite}
        />
      ))}
    </div>
  </section>
);

const TasksPage = () => {
  const { space_id } = useParams();
  const navigate = useNavigate();
  const [tasks, setTasks] = useState<ChatTask[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [actionTaskId, setActionTaskId] = useState('');
  const [keyword, setKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState<TaskStatusFilter>('all');
  const [view, setView] = useState<'all' | 'favorite'>('all');
  const [favoriteTaskIds, setFavoriteTaskIds] = useState<string[]>([]);

  const filteredTasks = filterTasks(tasks, keyword, statusFilter);
  const visibleTasks =
    view === 'favorite'
      ? filteredTasks.filter(task => favoriteTaskIds.includes(task.id))
      : filteredTasks;

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

  const toggleFavorite = (taskId: string) => {
    setFavoriteTaskIds(current =>
      current.includes(taskId)
        ? current.filter(item => item !== taskId)
        : [...current, taskId],
    );
  };

  const handleNavigate = (task: ChatTask) => {
    if (space_id) {
      navigate(`/space/${space_id}/tasks/${task.id}`);
    }
  };

  return (
    <main className="h-full overflow-auto coz-bg-primary px-[24px] py-[28px]">
      <section className="mx-auto w-full max-w-[1080px]">
        <TasksHeader
          loading={loading}
          spaceId={space_id}
          onRefresh={loadTasks}
        />
        <TasksToolbar
          keyword={keyword}
          statusFilter={statusFilter}
          view={view}
          onKeywordChange={setKeyword}
          onStatusFilterChange={setStatusFilter}
          onViewChange={setView}
        />

        {error ? (
          <div className="mt-[16px] rounded-[8px] border border-solid border-[#ffd4cc] bg-[#fff1ee] px-[12px] py-[10px] text-[14px] leading-[20px] text-[#c02a1d] break-words">
            {error}
          </div>
        ) : null}

        <TaskList
          actionTaskId={actionTaskId}
          favoriteTaskIds={favoriteTaskIds}
          loading={loading}
          spaceId={space_id}
          tasks={visibleTasks}
          totalTasks={tasks.length}
          onAction={runTaskAction}
          onNavigate={handleNavigate}
          onToggleFavorite={toggleFavorite}
        />
      </section>
    </main>
  );
};

export default TasksPage;
