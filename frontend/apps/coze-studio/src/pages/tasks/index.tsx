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

import { IconCozAsynchronousTask } from '@coze-arch/coze-design/icons';
import type { workbenchTask } from '@coze-studio/api-schema';

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import { listTasks } from './service';
import {
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

const getStatusPillClassName = (status: workbenchTask.TaskStatus) => {
  const tone = getTaskStatusTone(status);

  if (tone === 'danger') {
    return {
      dot: 'bg-[#f54a45]',
      pill: 'bg-[rgba(245,74,69,0.1)] text-[#f54a45]',
    };
  }

  if (tone === 'neutral') {
    return {
      dot: 'bg-[#a7adb8]',
      pill: 'bg-[rgba(91,100,117,0.1)] text-[#747b8a]',
    };
  }

  return {
    dot: 'bg-[#2a9e06]',
    pill: 'bg-[rgba(42,158,6,0.1)] text-[#2a9e06]',
  };
};

interface TasksHeaderProps {
  loading: boolean;
  spaceId?: string;
  onRefresh: () => void;
}

const TasksHeader = ({ loading, spaceId, onRefresh }: TasksHeaderProps) => (
  <div className="text-center">
    <h1 className="m-0 text-[26px] leading-[36px] font-[600] text-[#1d2129]">
      全部任务
    </h1>
    <p className="mt-[4px] mb-0 text-[13px] leading-[20px] text-[#747b8a]">
      这里收纳您当前工作空间内的全部任务
    </p>
    <button
      type="button"
      className="sr-only"
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
  <section className="mt-[24px] flex flex-wrap items-center gap-[12px]">
    <label className="flex h-[36px] w-full max-w-[280px] items-center gap-[8px] rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white px-[12px] text-[#747b8a]">
      <span className="shrink-0 text-[14px]" aria-hidden="true">
        ⌕
      </span>
      <input
        aria-label="搜索任务"
        className="min-w-0 flex-1 border-0 bg-transparent text-[13px] leading-[20px] text-[#232938] outline-none"
        value={keyword}
        onChange={event => onKeywordChange(event.target.value)}
        placeholder="搜索会话"
      />
    </label>

    <select
      aria-label="任务状态"
      className="h-[36px] w-[36px] rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white px-[8px] text-[0] text-[#444c5c]"
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

    <div className="grid h-[36px] grid-cols-2 rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white p-[2px]">
      {[
        { label: '全部', value: 'all' as const },
        { label: '已收藏', value: 'favorite' as const },
      ].map(item => (
        <button
          key={item.value}
          type="button"
          className="rounded-[4px] border-0 bg-transparent px-[12px] text-[13px] leading-[20px] text-[#444c5c] data-[active=true]:bg-[rgba(91,100,117,0.1)] data-[active=true]:text-[#1d2129]"
          data-active={view === item.value}
          onClick={() => onViewChange(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>

    <div className="flex-1" />

    <button
      type="button"
      className="h-[36px] rounded-[6px] border border-solid border-[rgba(77,101,148,0.2)] bg-white px-[12px] text-[13px] leading-[20px] text-[#444c5c]"
    >
      批量操作
    </button>
  </section>
);

interface TaskRowProps {
  favorite: boolean;
  spaceId?: string;
  task: ChatTask;
  onNavigate: (task: ChatTask) => void;
  onToggleFavorite: (taskId: string) => void;
}

const TaskRow = ({
  favorite,
  task,
  onNavigate,
  onToggleFavorite,
}: TaskRowProps) => {
  const statusPill = getStatusPillClassName(task.status);
  const description = task.input || task.result || task.error || task.title;

  return (
    <article className="flex items-start gap-[12px] border-0 border-b border-solid border-[rgba(77,101,148,0.08)] px-[16px] py-[12px] transition-colors last:border-b-0 hover:bg-[rgba(91,100,117,0.04)]">
      <button
        type="button"
        className="relative mt-[2px] flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded-[6px] border border-solid border-[rgba(77,101,148,0.15)] bg-white text-[12px] text-[#444c5c]"
        aria-label={`打开任务 ${task.title}`}
        onClick={() => onNavigate(task)}
      >
        <IconCozAsynchronousTask className="text-[13px]" />
        <span className="absolute bottom-[-2px] right-[-2px] flex h-[12px] w-[12px] items-center justify-center rounded-full bg-white">
          <span className="h-[6px] w-[6px] rounded-full bg-[#2a9e06]" />
        </span>
      </button>
      <button
        type="button"
        className="min-w-0 flex-1 border-0 bg-transparent p-0 text-left cursor-pointer"
        onClick={() => onNavigate(task)}
      >
        <span className="block truncate text-[14px] leading-[20px] font-[500] text-[#232938]">
          {task.title}
        </span>
        <span className="mt-[2px] block truncate text-[12px] leading-[18px] text-[#747b8a]">
          {description}
        </span>
      </button>
      <div className="mt-[2px] flex shrink-0 items-center gap-[12px]">
        <span
          className={`inline-flex h-[20px] items-center gap-[4px] rounded-full px-[8px] text-[11px] leading-[16px] ${statusPill.pill}`}
          data-status-tone={getTaskStatusTone(task.status)}
        >
          <span className={`h-[6px] w-[6px] rounded-full ${statusPill.dot}`} />
          {getTaskStatusText(task.status)}
        </span>
        <span className="w-[96px] text-right text-[12px] leading-[18px] text-[#747b8a]">
          {formatUpdatedTime(task.updated_at)}
        </span>
        <button
          type="button"
          className="border-0 bg-transparent text-[16px] leading-[20px] text-[#747b8a] cursor-pointer"
          aria-pressed={favorite}
          aria-label={favorite ? '取消收藏' : '收藏任务'}
          onClick={() => onToggleFavorite(task.id)}
        >
          ...
        </button>
      </div>
    </article>
  );
};

interface TaskListProps {
  favoriteTaskIds: string[];
  loading: boolean;
  spaceId?: string;
  tasks: ChatTask[];
  totalTasks: number;
  onNavigate: (task: ChatTask) => void;
  onToggleFavorite: (taskId: string) => void;
}

const TaskList = ({
  favoriteTaskIds,
  loading,
  spaceId,
  tasks,
  totalTasks,
  onNavigate,
  onToggleFavorite,
}: TaskListProps) => (
  <section className="mt-[16px]" aria-label="任务列表">
    {loading ? (
      <div className="rounded-[12px] border border-solid border-[rgba(77,101,148,0.15)] bg-white py-[32px] text-center text-[14px] text-[#747b8a]">
        加载中...
      </div>
    ) : null}

    {!loading && tasks.length === 0 ? (
      <div className="rounded-[12px] border border-dashed border-[rgba(77,101,148,0.2)] bg-white px-[16px] py-[32px] text-center text-[14px] text-[#747b8a]">
        {totalTasks === 0 ? '暂无任务' : '没有匹配的任务'}
      </div>
    ) : null}

    <div className="overflow-hidden rounded-[12px] border border-solid border-[rgba(77,101,148,0.15)] bg-white">
      {tasks.map(task => (
        <TaskRow
          key={task.id}
          favorite={favoriteTaskIds.includes(task.id)}
          spaceId={spaceId}
          task={task}
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
    <main className="flex h-full flex-col overflow-auto bg-white">
      <WorkspacePageTopBar />
      <section className="mx-auto w-[calc(100%_-_64px)] max-w-[1016px] pt-[24px] pb-[28px]">
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
          favoriteTaskIds={favoriteTaskIds}
          loading={loading}
          spaceId={space_id}
          tasks={visibleTasks}
          totalTasks={tasks.length}
          onNavigate={handleNavigate}
          onToggleFavorite={toggleFavorite}
        />
      </section>
    </main>
  );
};

export default TasksPage;
