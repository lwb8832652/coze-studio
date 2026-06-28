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

import type { workbenchTask } from '@coze-studio/api-schema';

import { TaskTopBar } from './task-top-bar';
import { TaskExportAction } from './task-export-action';
import type { TaskDetailTokenUsage } from './task-detail-loader';
import { TaskDetailInspector } from './task-detail-inspector';
import { TaskArtifactsPanel } from './task-artifacts-panel';

type ChatTask = workbenchTask.ChatTask;
type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;
type TaskThreadMessage = workbenchTask.TaskThreadMessage;

export const TaskDetailHeader = ({
  artifacts,
  memoryReadOnly,
  messages,
  onArtifactsChanged,
  spaceId,
  task,
  threadId,
  tokenUsage,
}: {
  artifacts: TaskThreadArtifact[];
  memoryReadOnly: boolean;
  messages: TaskThreadMessage[];
  onArtifactsChanged?: () => void | Promise<void>;
  spaceId?: string;
  task: ChatTask;
  threadId?: string;
  tokenUsage?: TaskDetailTokenUsage;
}) => (
  <TaskTopBar
    exportAction={
      <TaskExportAction
        messages={messages}
        task={task}
        tokenUsage={tokenUsage}
      />
    }
    inspectorAction={
      threadId ? (
        <TaskDetailInspector
          artifactAction={
            <TaskArtifactsPanel
              artifacts={artifacts}
              onArtifactsChanged={onArtifactsChanged}
              threadId={threadId}
            />
          }
          memoryReadOnly={memoryReadOnly}
          spaceId={spaceId}
          threadId={threadId}
        />
      ) : undefined
    }
    task={task}
    tokenUsage={tokenUsage}
  />
);
