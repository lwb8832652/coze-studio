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
import { useNavigate } from 'react-router-dom';

import { useSpaceStore } from '@coze-foundation/space-store';
import { Loading } from '@coze-arch/coze-design';
import type { workbenchTask } from '@coze-studio/api-schema';

import { formatUpdatedTime, getTaskStatusText } from '../../pages/tasks/helpers';
import { listTasks } from '../../pages/tasks/service';

type ChatTask = workbenchTask.ChatTask;

export const WorkspaceTaskList = () => {
  const navigate = useNavigate();
  const spaceId = useSpaceStore(state => state.space.id);
  const [tasks, setTasks] = useState<ChatTask[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!spaceId) {
      return;
    }

    let canceled = false;

    const loadTasks = async () => {
      setLoading(true);

      try {
        const response = await listTasks({ space_id: spaceId, page_size: 8 });

        if (!canceled) {
          setTasks(response.data?.tasks ?? []);
        }
      } catch {
        if (!canceled) {
          setTasks([]);
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

  return (
    <section className="w-full h-full flex flex-col" aria-label="我的任务">
      <div className="flex h-[24px] items-center justify-between pl-[8px] pr-[4px] mb-[4px]">
        <h2 className="m-0 text-[14px] leading-[20px] font-[600] coz-fg-secondary">
          我的任务
        </h2>
        <button
          type="button"
          className="border-0 bg-transparent text-[12px] leading-[18px] coz-fg-secondary cursor-pointer"
          onClick={() => spaceId && navigate(`/space/${spaceId}/tasks`)}
        >
          查看全部
        </button>
      </div>

      {loading ? (
        <div className="flex h-[120px] items-center justify-center">
          <Loading loading={true} size="mini" />
        </div>
      ) : null}

      {!loading && tasks.length === 0 ? (
        <div className="px-[8px] py-[12px] text-[13px] leading-[20px] coz-fg-dim">
          暂无任务
        </div>
      ) : null}

      <div className="grid gap-[4px]">
        {tasks.map(task => (
          <button
            key={task.id}
            type="button"
            className="min-w-0 rounded-[8px] border-0 bg-transparent px-[8px] py-[8px] text-left cursor-pointer hover:coz-mg-secondary-hovered"
            onClick={() =>
              spaceId && navigate(`/space/${spaceId}/tasks/${task.id}`)
            }
          >
            <div className="truncate text-[13px] leading-[20px] font-[500] coz-fg-primary">
              {task.title}
            </div>
            <div className="mt-[2px] flex items-center justify-between gap-[8px] text-[12px] leading-[18px] coz-fg-secondary">
              <span>{getTaskStatusText(task.status)}</span>
              <span className="truncate">
                {formatUpdatedTime(task.updated_at)}
              </span>
            </div>
          </button>
        ))}
      </div>
    </section>
  );
};
