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
import { listTasks } from './service';
import {
  filterTasks,
  formatUpdatedTime,
  getTaskInputText,
  getTaskResultText,
  getTaskStatusTone,
  getTaskStatusText,
  type TaskStatusFilter,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;

const getStatusPillClassName = (status: workbenchTask.TaskStatus) => {
  const tone = getTaskStatusTone(status);

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
  <div className="text-center">
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
  const description =
    getTaskInputText(task.input) ||
    getTaskResultText(task.result) ||
    task.error ||
    task.title;

  return (
    <article className="coze-prototype-row">
      <button
        type="button"
        className="coze-prototype-task-icon mt-[2px]"
        aria-label={`打开任务 ${task.title}`}
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
        <span className="coze-prototype-row-title">{task.title}</span>
        <span className="coze-prototype-row-desc">{description}</span>
      </button>
      <div className="coze-prototype-row-actions">
        <span
          className="coze-prototype-status-pill"
          data-tone={statusPill.tone}
          data-status-tone={getTaskStatusTone(task.status)}
        >
          <span
            className="coze-prototype-status-dot"
            style={{ backgroundColor: statusPill.color }}
          />
          {getTaskStatusText(task.status)}
        </span>
        <span className="coze-prototype-muted w-[60px] text-right">
          {formatUpdatedTime(task.updated_at)}
        </span>
        <button
          type="button"
          className="coze-prototype-more-button"
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
    {loading ? <div className="coze-prototype-empty">加载中...</div> : null}

    {!loading && tasks.length === 0 ? (
      <div className="coze-prototype-empty">
        {totalTasks === 0 ? '暂无任务' : '没有匹配的任务'}
      </div>
    ) : null}

    <div className="coze-prototype-list">
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
      navigate(buildTaskThreadDetailPath(space_id, task.id));
    }
  };

  return (
    <main className="coze-prototype-page">
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

        {error ? <div className="coze-prototype-error">{error}</div> : null}

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
