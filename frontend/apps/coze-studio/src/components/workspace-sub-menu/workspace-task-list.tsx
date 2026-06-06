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
    <section className="flex h-full w-full flex-col" aria-label="我的任务">
      <h2 className="coze-prototype-sidebar-task-title">我的任务</h2>

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

      <div className="coze-prototype-sidebar-task-list">
        {tasks.map(task => {
          const statusMeta = getWorkspaceTaskStatusMeta(task.status);

          return (
            <button
              key={task.id}
              type="button"
              className="coze-prototype-sidebar-task-row"
              onClick={() =>
                spaceId && navigate(`/space/${spaceId}/tasks/${task.id}`)
              }
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
              <span className="coze-prototype-sidebar-task-name">
                {task.title}
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
};
