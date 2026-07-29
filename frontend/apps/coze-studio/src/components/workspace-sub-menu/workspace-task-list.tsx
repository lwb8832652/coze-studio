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

import { useNavigate } from 'react-router-dom';
import {
  type RefObject,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';

import { useSpaceStore } from '@coze-foundation/space-store';
import { IconCozAsynchronousTask } from '@coze-arch/coze-design/icons';
import { Loading } from '@coze-arch/coze-design';

import {
  WORKSPACE_TASK_THREAD_UPSERT_EVENT,
  type WorkspaceTaskThreadUpsertDetail,
} from '../../pages/tasks/task-thread-events';
import { getTaskThreadDisplayTitle } from '../../pages/tasks/task-display-title';
import { listTaskThreads } from '../../pages/tasks/service';
import type { WorkbenchThread } from '../../pages/workbench/thread-client';
import { buildTaskThreadDetailPath } from '../../pages/chats/task-thread-routes';
import { getWorkspaceTaskStatusMeta } from './workspace-task-status';

type TaskThread = WorkbenchThread;

const RECENT_TASK_PAGE_SIZE = 20;

const upsertTaskThread = (
  tasks: TaskThread[],
  task: WorkspaceTaskThreadUpsertDetail['thread'],
  mode: WorkspaceTaskThreadUpsertDetail['mode'] = 'upsert',
) => {
  const existingTask = tasks.find(item => item.thread_id === task.thread_id);

  if (mode === 'patch') {
    if (!existingTask) {
      return tasks;
    }

    return tasks.map(item =>
      item.thread_id === task.thread_id ? { ...item, ...task } : item,
    );
  }

  const nextTask = existingTask ? { ...existingTask, ...task } : task;

  return [
    nextTask as TaskThread,
    ...tasks.filter(item => item.thread_id !== task.thread_id),
  ];
};

const appendUniqueTaskThreads = (
  current: TaskThread[],
  incoming: TaskThread[],
) => {
  const existingIDs = new Set(current.map(task => task.thread_id));
  const next = [...current];

  incoming.forEach(task => {
    if (!existingIDs.has(task.thread_id)) {
      existingIDs.add(task.thread_id);
      next.push(task);
    }
  });

  return next;
};

const applyPendingTaskThreadPatches = (
  tasks: TaskThread[],
  pendingPatches: Map<string, WorkspaceTaskThreadUpsertDetail['thread']>,
) =>
  tasks.map(task => {
    const patch = pendingPatches.get(task.thread_id);
    if (!patch) {
      return task;
    }

    pendingPatches.delete(task.thread_id);
    return { ...task, ...patch };
  });

const useTaskThreadInfiniteScroll = ({
  hasMore,
  listRef,
  loadMoreTasks,
  sentinelRef,
}: {
  hasMore: boolean;
  listRef: RefObject<HTMLDivElement>;
  loadMoreTasks: () => void;
  sentinelRef: RefObject<HTMLDivElement>;
}) => {
  useEffect(() => {
    const root = listRef.current;
    const sentinel = sentinelRef.current;

    if (
      !root ||
      !sentinel ||
      !hasMore ||
      typeof IntersectionObserver === 'undefined'
    ) {
      return;
    }

    const observer = new IntersectionObserver(
      entries => {
        if (entries.some(entry => entry.isIntersecting)) {
          loadMoreTasks();
        }
      },
      {
        root,
        rootMargin: '120px 0px 120px 0px',
      },
    );

    observer.observe(sentinel);

    return () => {
      observer.disconnect();
    };
  }, [hasMore, listRef, loadMoreTasks, sentinelRef]);
};

// Pagination and race-safe live title patches share one state boundary.
// eslint-disable-next-line @coze-arch/max-line-per-function
const useWorkspaceTaskThreads = (spaceId?: string) => {
  const [tasks, setTasks] = useState<TaskThread[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [total, setTotal] = useState(0);
  const pageRef = useRef(1);
  const loadingMoreRef = useRef(false);
  const pendingPatchesRef = useRef<
    Map<string, WorkspaceTaskThreadUpsertDetail['thread']>
  >(new Map());
  const tasksRef = useRef<TaskThread[]>([]);
  const listRef = useRef<HTMLDivElement | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const hasMore = tasks.length < total;
  tasksRef.current = tasks;

  const loadMoreTasks = useCallback(async () => {
    if (
      !spaceId ||
      loading ||
      loadingMoreRef.current ||
      tasks.length >= total
    ) {
      return;
    }

    const nextPage = pageRef.current + 1;
    loadingMoreRef.current = true;
    setLoadingMore(true);

    try {
      const response = await listTaskThreads({
        space_id: spaceId,
        page: nextPage,
        page_size: RECENT_TASK_PAGE_SIZE,
      });
      const nextThreads = applyPendingTaskThreadPatches(
        response.data?.threads ?? [],
        pendingPatchesRef.current,
      );
      setTasks(current => appendUniqueTaskThreads(current, nextThreads));
      setTotal(response.data?.total ?? tasks.length + nextThreads.length);
      pageRef.current = nextPage;
    } catch {
      setTotal(current => Math.max(current, tasks.length));
    } finally {
      loadingMoreRef.current = false;
      setLoadingMore(false);
    }
  }, [loading, spaceId, tasks.length, total]);

  useEffect(() => {
    pendingPatchesRef.current.clear();

    if (!spaceId) {
      setTasks([]);
      setTotal(0);
      return;
    }

    let canceled = false;

    const loadTasks = async () => {
      setLoading(true);

      try {
        const response = await listTaskThreads({
          space_id: spaceId,
          page: 1,
          page_size: RECENT_TASK_PAGE_SIZE,
        });

        if (!canceled) {
          const nextThreads = applyPendingTaskThreadPatches(
            response.data?.threads ?? [],
            pendingPatchesRef.current,
          );
          setTasks(nextThreads);
          setTotal(response.data?.total ?? response.data?.threads?.length ?? 0);
          pageRef.current = 1;
        }
      } catch {
        if (!canceled) {
          setTasks([]);
          setTotal(0);
        }
      } finally {
        if (!canceled) {
          setLoading(false);
        }
      }
    };

    void loadTasks();

    return () => {
      canceled = true;
    };
  }, [spaceId]);

  useEffect(() => {
    if (!spaceId) {
      return;
    }

    const handleTaskThreadUpsert = (event: Event) => {
      const { detail } = event as CustomEvent<WorkspaceTaskThreadUpsertDetail>;

      if (!detail?.thread || detail.space_id !== spaceId) {
        return;
      }

      const taskExists = tasksRef.current.some(
        task => task.thread_id === detail.thread.thread_id,
      );
      if (detail.mode === 'patch' && !taskExists) {
        const pendingPatch = pendingPatchesRef.current.get(
          detail.thread.thread_id,
        );
        pendingPatchesRef.current.set(detail.thread.thread_id, {
          ...pendingPatch,
          ...detail.thread,
        });
      }

      const pendingPatch = pendingPatchesRef.current.get(
        detail.thread.thread_id,
      );
      const nextThread =
        detail.mode !== 'patch' && pendingPatch
          ? { ...detail.thread, ...pendingPatch }
          : detail.thread;
      if (detail.mode !== 'patch' && pendingPatch) {
        pendingPatchesRef.current.delete(detail.thread.thread_id);
      }

      setTasks(current => upsertTaskThread(current, nextThread, detail.mode));
      setTotal(current =>
        taskExists || detail.mode === 'patch'
          ? current
          : Math.max(current + 1, tasksRef.current.length + 1),
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
  }, [spaceId]);

  useTaskThreadInfiniteScroll({
    hasMore,
    listRef,
    loadMoreTasks,
    sentinelRef,
  });

  return {
    hasMore,
    listRef,
    loading,
    loadingMore,
    sentinelRef,
    tasks,
  };
};

const WorkspaceTaskRow = ({
  onNavigate,
  task,
}: {
  task: TaskThread;
  onNavigate: (task: TaskThread) => void;
}) => {
  const statusMeta = getWorkspaceTaskStatusMeta(task.status);
  const displayTitle = getTaskThreadDisplayTitle(task);

  return (
    <button
      key={task.thread_id}
      type="button"
      className="coze-prototype-sidebar-task-row"
      onClick={() => onNavigate(task)}
    >
      <span className="coze-prototype-task-icon">
        <IconCozAsynchronousTask className="text-[13px]" />
        <span className="coze-prototype-status-dot-wrap">
          <span
            className="coze-prototype-status-dot"
            style={{ backgroundColor: statusMeta.color }}
            aria-label={statusMeta.ariaLabel}
            data-status-tone={statusMeta.tone}
          />
        </span>
      </span>
      <span className="coze-prototype-sidebar-task-name">{displayTitle}</span>
    </button>
  );
};

const WorkspaceTaskLoadMore = ({
  loadingMore,
  sentinelRef,
}: {
  loadingMore: boolean;
  sentinelRef: RefObject<HTMLDivElement>;
}) => (
  <div
    ref={sentinelRef}
    className="coze-prototype-sidebar-load-more"
    role={loadingMore ? 'status' : undefined}
    aria-live="polite"
  >
    {loadingMore ? (
      <>
        <Loading loading={true} size="mini" />
        <span>加载更多任务...</span>
      </>
    ) : null}
  </div>
);

export const WorkspaceTaskList = () => {
  const navigate = useNavigate();
  const spaceId = useSpaceStore(state => state.space.id);
  const { hasMore, listRef, loading, loadingMore, sentinelRef, tasks } =
    useWorkspaceTaskThreads(spaceId);

  const handleNavigate = useCallback(
    (task: TaskThread) => {
      if (spaceId) {
        navigate(buildTaskThreadDetailPath(spaceId, task.thread_id));
      }
    },
    [navigate, spaceId],
  );

  return (
    <section className="flex h-full w-full flex-col" aria-label="我的任务">
      <h2 className="coze-prototype-sidebar-task-title">我的任务</h2>

      {loading && tasks.length === 0 ? (
        <div className="flex h-[120px] items-center justify-center">
          <Loading loading={true} size="mini" />
        </div>
      ) : null}

      {!loading && tasks.length === 0 ? (
        <div className="px-[8px] py-[12px] text-[13px] leading-[20px] coz-fg-dim">
          暂无任务
        </div>
      ) : null}

      <div className="coze-prototype-sidebar-task-list" ref={listRef}>
        {tasks.map(task => (
          <WorkspaceTaskRow
            key={task.thread_id}
            task={task}
            onNavigate={handleNavigate}
          />
        ))}
        {hasMore ? (
          <WorkspaceTaskLoadMore
            loadingMore={loadingMore}
            sentinelRef={sentinelRef}
          />
        ) : null}
      </div>
    </section>
  );
};
