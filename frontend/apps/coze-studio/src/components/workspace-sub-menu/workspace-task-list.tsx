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
import { IconCozAsynchronousTask } from '@coze-arch/coze-design/icons';
import type { workbenchTask } from '@coze-studio/api-schema';

import { listTasks } from '../../pages/tasks/service';
import { getWorkspaceTaskStatusMeta } from './workspace-task-status';

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
      <h2 className="m-0 px-[16px] text-[12px] leading-[18px] font-[400] text-[#747b8a]">
        我的任务
      </h2>

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

      <div className="mt-[8px] grid gap-[2px] px-[8px]">
        {tasks.map(task => {
          const statusMeta = getWorkspaceTaskStatusMeta(task.status);

          return (
            <button
              key={task.id}
              type="button"
              className="flex h-[34px] min-w-0 items-center gap-[8px] rounded-[6px] border-0 bg-transparent px-[8px] text-left cursor-pointer hover:coz-mg-secondary-hovered"
              onClick={() =>
                spaceId && navigate(`/space/${spaceId}/tasks/${task.id}`)
              }
            >
              <span className="relative flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded-[6px] border border-solid border-[rgba(77,101,148,0.15)] bg-white coz-fg-secondary">
                <IconCozAsynchronousTask className="text-[13px]" />
                <span className="absolute bottom-[-2px] right-[-2px] flex h-[12px] w-[12px] items-center justify-center rounded-full bg-[#f3f3f5]">
                  <span
                    className={`h-[6px] w-[6px] rounded-full ${statusMeta.dotClassName}`}
                    aria-label={statusMeta.ariaLabel}
                    data-status-tone={statusMeta.tone}
                  />
                </span>
              </span>
              <span className="min-w-0 flex-1 truncate text-[14px] leading-[20px] font-[600] text-[#232938]">
                {task.title}
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
};
