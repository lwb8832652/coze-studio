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
  IconCozArrowDown,
  IconCozLink,
  IconCozStar,
} from '@coze-arch/coze-design/icons';

import type {
  WorkbenchArtifact,
  WorkbenchMessage,
} from '../workbench/thread-client';
import { buildTaskThreadListPath } from '../chats/task-thread-routes';
import { WorkspaceHeaderActions } from '../../components/workspace-header-actions';
import type { TaskThreadDetailModel } from './task-thread-detail-model';
import { TaskExportAction } from './task-export-action';
import { getTaskThreadDisplayTitle } from './task-display-title';
import { TaskDetailInspector } from './task-detail-inspector';
import { TaskArtifactsPanel } from './task-artifacts-panel';

type TaskThreadArtifact = WorkbenchArtifact;
type TaskThreadMessage = WorkbenchMessage;

const MILLISECONDS_PER_SECOND = 1000;
const MILLISECOND_TIMESTAMP_THRESHOLD = 1_000_000_000_000;
const DATE_TIME_PART_LENGTH = 2;

const normalizeTimestamp = (value?: number | string) => {
  const timestamp = Number(value);

  if (!Number.isFinite(timestamp) || timestamp <= 0) {
    return undefined;
  }

  return timestamp < MILLISECOND_TIMESTAMP_THRESHOLD
    ? timestamp * MILLISECONDS_PER_SECOND
    : timestamp;
};

const getCreatedAtDisplay = (value?: number | string) => {
  const timestamp = normalizeTimestamp(value);

  if (!timestamp) {
    return undefined;
  }

  const date = new Date(timestamp);
  const pad = (part: number) =>
    String(part).padStart(DATE_TIME_PART_LENGTH, '0');

  return {
    dateTime: date.toISOString(),
    label: `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(
      date.getDate(),
    )} ${pad(date.getHours())}:${pad(date.getMinutes())}`,
  };
};

export const TaskDetailHeader = ({
  artifacts,
  memoryReadOnly,
  messages,
  onArtifactsChanged,
  spaceId,
  task,
  threadId,
}: {
  artifacts: TaskThreadArtifact[];
  memoryReadOnly: boolean;
  messages: TaskThreadMessage[];
  onArtifactsChanged?: () => void | Promise<void>;
  spaceId: string;
  task: TaskThreadDetailModel;
  threadId?: string;
}) => {
  const navigate = useNavigate();
  const createdAt = getCreatedAtDisplay(task.created_at);
  const handleBackToTasks = () => {
    navigate(buildTaskThreadListPath(spaceId));
  };

  return (
    <header className="coze-prototype-task-topbar">
      <div className="coze-prototype-task-title-group">
        <div className="coze-prototype-task-title-copy">
          <div className="coze-prototype-task-title-line">
            <h1 className="coze-prototype-task-top-title">
              {getTaskThreadDisplayTitle(task) || task.title}
            </h1>
            <button
              type="button"
              className="coze-prototype-task-title-navigation"
              aria-label="返回全部任务"
              onClick={handleBackToTasks}
            >
              <IconCozArrowDown />
            </button>
          </div>
          {createdAt ? (
            <time
              className="coze-prototype-task-created-at"
              dateTime={createdAt.dateTime}
            >
              创建于 {createdAt.label}
            </time>
          ) : null}
        </div>
      </div>
      <div className="coze-prototype-task-topbar-actions">
        <div className="coze-prototype-task-topbar-primary">
          <TaskExportAction
            messages={messages}
            task={task}
            threadId={threadId}
          />
          {threadId ? (
            <TaskDetailInspector
              artifactAction={
                <TaskArtifactsPanel
                  artifacts={artifacts}
                  onArtifactsChanged={onArtifactsChanged}
                  spaceId={spaceId}
                  threadId={threadId}
                />
              }
              memoryReadOnly={memoryReadOnly}
              spaceId={spaceId}
              threadId={threadId}
            />
          ) : null}
        </div>
        <div
          className="coze-prototype-task-topbar-secondary"
          aria-label="任务辅助操作"
        >
          <button
            type="button"
            className="coze-prototype-task-action"
            aria-label="收藏任务"
          >
            <IconCozStar />
            <span>收藏</span>
          </button>
          <button
            type="button"
            className="coze-prototype-task-action"
            aria-label="分享任务"
          >
            <IconCozLink />
            <span>分享</span>
          </button>
          <WorkspaceHeaderActions compact />
        </div>
      </div>
    </header>
  );
};
