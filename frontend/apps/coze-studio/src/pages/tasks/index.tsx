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

import './newx-task-ui.less';

import { useNavigate, useParams } from 'react-router-dom';
import { useEffect, useState } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';
import {
  IconCozAsynchronousTask,
  IconCozFilter,
} from '@coze-arch/coze-design/icons';

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import '../../components/workspace-prototype.less';
import { buildTaskThreadDetailPath } from '../chats/task-thread-routes';
import {
  WORKSPACE_TASK_THREAD_UPSERT_EVENT,
  type WorkspaceTaskThreadUpsertDetail,
} from './task-thread-events';
import { getTaskThreadDisplayTitle } from './task-display-title';
import { listTaskThreads } from './service';
import { formatUpdatedTime, type TaskStatusFilter } from './helpers';

type TaskThread = workbenchTask.TaskThread;

const getTaskThreadDescription = (task: TaskThread) =>
  task.last_user_message || task.last_agent_message || task.title;

const applyTaskThreadUpdate = (
  tasks: TaskThread[],
  thread: WorkspaceTaskThreadUpsertDetail['thread'],
  mode: WorkspaceTaskThreadUpsertDetail['mode'] = 'upsert',
) => {
  const existingTask = tasks.find(task => task.thread_id === thread.thread_id);

  if (mode === 'patch') {
    if (!existingTask) {
      return tasks;
    }

    return tasks.map(task =>
      task.thread_id === thread.thread_id ? { ...task, ...thread } : task,
    );
  }

  const nextTask = existingTask ? { ...existingTask, ...thread } : thread;

  return [
    nextTask as TaskThread,
    ...tasks.filter(task => task.thread_id !== thread.thread_id),
  ];
};

const getTaskThreadStatusText = (status: string) => {
  const statusMap: Record<string, string> = {
    created: '已创建',
    queued: '排队中',
    idle: '待处理',
    running: '运行中',
    succeeded: '已完成',
    completed: '已完成',
    failed: '失败',
    canceling: '取消中',
    canceled: '已取消',
  };

  return statusMap[status] ?? '未知';
};

const getTaskThreadStatusTone = (status: string) => {
  if (status === 'running' || status === 'queued' || status === 'canceling') {
    return 'running';
  }

  if (status === 'completed' || status === 'succeeded') {
    return 'success';
  }

  if (status === 'failed' || status === 'canceled') {
    return 'danger';
  }

  return 'neutral';
};

const filterTaskThreads = (
  tasks: TaskThread[],
  keyword: string,
  statusFilter: TaskStatusFilter,
) => {
  const normalizedKeyword = keyword.trim().toLowerCase();

  return tasks.filter(task => {
    const displayTitle = getTaskThreadDisplayTitle(task);
    const readableMessage = getTaskThreadDescription(task).toLowerCase();
    const matchesKeyword = normalizedKeyword
      ? displayTitle.toLowerCase().includes(normalizedKeyword) ||
        task.title.toLowerCase().includes(normalizedKeyword) ||
        readableMessage.includes(normalizedKeyword)
      : true;
    const matchesStatus =
      statusFilter === 'all' ||
      (statusFilter === 'running' &&
        ['created', 'queued', 'idle', 'running', 'canceling'].includes(
          task.status,
        )) ||
      (statusFilter === 'succeeded' &&
        ['succeeded', 'completed'].includes(task.status)) ||
      (statusFilter === 'failed' &&
        ['failed', 'canceled'].includes(task.status));

    return matchesKeyword && matchesStatus;
  });
};

const getStatusPillClassName = (status: string) => {
  const tone = getTaskThreadStatusTone(status);

  if (tone === 'danger') {
    return {
      color: '#f54a45',
      tone,
    };
  }

  if (tone === 'neutral') {
    return {
      color: '#a7adb8',
      tone,
    };
  }

  return {
    color: '#2a9e06',
    tone,
  };
};

interface TasksHeaderProps {
  loading: boolean;
  spaceId?: string;
  onRefresh: () => void;
}

const TasksHeader = ({ loading, spaceId, onRefresh }: TasksHeaderProps) => (
  <div className="newx-page-heading">
    <h1 className="coze-prototype-page-title">全部任务</h1>
    <p className="coze-prototype-page-subtitle">
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
  <section className="coze-prototype-toolbar">
    <label className="coze-prototype-search" data-width="wide">
      <span aria-hidden="true">⌕</span>
      <input
        aria-label="搜索任务"
        value={keyword}
        onChange={event => onKeywordChange(event.target.value)}
        placeholder="搜索会话"
      />
    </label>

    <button
      type="button"
      aria-label="任务状态"
      className="coze-prototype-filter-button"
      onClick={() =>
        onStatusFilterChange(statusFilter === 'all' ? 'running' : 'all')
      }
    >
      <IconCozFilter className="text-[16px]" />
    </button>

    <div className="coze-prototype-segment">
      {[
        { label: '全部', value: 'all' as const },
        { label: '已收藏', value: 'favorite' as const },
      ].map(item => (
        <button
          key={item.value}
          type="button"
          data-active={view === item.value}
          onClick={() => onViewChange(item.value)}
        >
          {item.label}
        </button>
      ))}
    </div>

    <div className="flex-1" />

    <button type="button" className="coze-prototype-secondary-button">
      批量操作
    </button>
  </section>
);

interface TaskRowProps {
  favorite: boolean;
  spaceId?: string;
  task: TaskThread;
  onNavigate: (task: TaskThread) => void;
  onToggleFavorite: (taskId: string) => void;
}

const TaskRow = ({
  favorite,
  task,
  onNavigate,
  onToggleFavorite,
}: TaskRowProps) => {
  const statusPill = getStatusPillClassName(task.status);
  const description = getTaskThreadDescription(task);
  const displayTitle = getTaskThreadDisplayTitle(task);

  return (
    <article className="coze-prototype-row">
      <button
        type="button"
        className="coze-prototype-task-icon mt-[2px]"
        aria-label={`打开任务 ${displayTitle}`}
        onClick={() => onNavigate(task)}
      >
        <IconCozAsynchronousTask className="text-[13px]" />
        <span
          className="coze-prototype-status-dot-wrap"
          style={{ backgroundColor: '#fff' }}
        >
          <span
            className="coze-prototype-status-dot"
            style={{ backgroundColor: statusPill.color }}
          />
        </span>
      </button>
      <button
        type="button"
        className="coze-prototype-row-main border-0 bg-transparent p-0 text-left cursor-pointer"
        onClick={() => onNavigate(task)}
      >
        <span className="coze-prototype-row-title">{displayTitle}</span>
        <span className="coze-prototype-row-desc">{description}</span>
      </button>
      <div className="coze-prototype-row-actions">
        <span
          className="coze-prototype-status-pill"
          data-tone={statusPill.tone}
          data-status-tone={getTaskThreadStatusTone(task.status)}
        >
          <span
            className="coze-prototype-status-dot"
            style={{ backgroundColor: statusPill.color }}
          />
          {getTaskThreadStatusText(task.status)}
        </span>
        <span className="coze-prototype-muted w-[60px] text-right">
          {formatUpdatedTime(task.updated_at)}
        </span>
        <button
          type="button"
          className="coze-prototype-more-button"
          aria-pressed={favorite}
          aria-label={favorite ? '取消收藏' : '收藏任务'}
          onClick={() => onToggleFavorite(task.thread_id)}
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
  tasks: TaskThread[];
  totalTasks: number;
  onNavigate: (task: TaskThread) => void;
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
    {loading ? <div className="coze-prototype-empty">加载中...</div> : null}

    {!loading && tasks.length === 0 ? (
      <div className="coze-prototype-empty">
        {totalTasks === 0 ? '暂无任务' : '没有匹配的任务'}
      </div>
    ) : null}

    <div className="coze-prototype-list">
      {tasks.map(task => (
        <TaskRow
          key={task.thread_id}
          favorite={favoriteTaskIds.includes(task.thread_id)}
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
  const [tasks, setTasks] = useState<TaskThread[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [keyword, setKeyword] = useState('');
  const [statusFilter, setStatusFilter] = useState<TaskStatusFilter>('all');
  const [view, setView] = useState<'all' | 'favorite'>('all');
  const [favoriteTaskIds, setFavoriteTaskIds] = useState<string[]>([]);

  const filteredTasks = filterTaskThreads(tasks, keyword, statusFilter);
  const visibleTasks =
    view === 'favorite'
      ? filteredTasks.filter(task => favoriteTaskIds.includes(task.thread_id))
      : filteredTasks;

  const loadTasks = async () => {
    if (!space_id) {
      return;
    }

    setLoading(true);
    setError('');

    try {
      const response = await listTaskThreads({ space_id });
      setTasks(response.data?.threads ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载任务失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadTasks();
  }, [space_id]);

  useEffect(() => {
    if (!space_id) {
      return;
    }

    const handleTaskThreadUpsert = (event: Event) => {
      const { detail } = event as CustomEvent<WorkspaceTaskThreadUpsertDetail>;

      if (!detail?.thread || detail.space_id !== space_id) {
        return;
      }

      setTasks(current =>
        applyTaskThreadUpdate(current, detail.thread, detail.mode),
      );
    };

    window.addEventListener(
      WORKSPACE_TASK_THREAD_UPSERT_EVENT,
      handleTaskThreadUpsert,
    );

    return () => {
      window.removeEventListener(
        WORKSPACE_TASK_THREAD_UPSERT_EVENT,
        handleTaskThreadUpsert,
      );
    };
  }, [space_id]);

  const toggleFavorite = (taskId: string) => {
    setFavoriteTaskIds(current =>
      current.includes(taskId)
        ? current.filter(item => item !== taskId)
        : [...current, taskId],
    );
  };

  const handleNavigate = (task: TaskThread) => {
    if (space_id) {
      navigate(buildTaskThreadDetailPath(space_id, task.thread_id));
    }
  };

  return (
    <main className="coze-prototype-page newx-menu-page newx-tasks-page-shell">
      <WorkspacePageTopBar />
      <section className="coze-prototype-page-inner">
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
          <div className="coze-prototype-error" role="alert">
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
