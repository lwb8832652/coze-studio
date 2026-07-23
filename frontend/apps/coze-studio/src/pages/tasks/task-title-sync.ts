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

import { useCallback, useRef, type Dispatch, type SetStateAction } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';

import { emitWorkspaceTaskThreadUpsert } from './task-thread-events';

type ChatTask = workbenchTask.ChatTask;

const buildThreadTitleUpdatedTask = ({
  currentTask,
  threadId,
  title,
}: {
  currentTask?: ChatTask;
  threadId: string;
  title: string;
}) => {
  const normalizedTitle = title.trim();
  if (!currentTask || currentTask.id !== threadId || !normalizedTitle) {
    return undefined;
  }

  return {
    ...currentTask,
    title: normalizedTitle,
    updated_at: Date.now(),
  };
};

const emitThreadTitlePatch = ({
  spaceID,
  task,
}: {
  spaceID?: string;
  task: ChatTask;
}) => {
  emitWorkspaceTaskThreadUpsert({
    mode: 'patch',
    space_id: task.space_id || spaceID || '',
    thread: {
      thread_id: task.id,
      title: task.title,
      updated_at: task.updated_at,
    },
  });
};

export const useTaskThreadTitleSync = ({
  setTask,
  spaceID,
}: {
  setTask: Dispatch<SetStateAction<ChatTask | undefined>>;
  spaceID?: string;
}) => {
  const taskRef = useRef<ChatTask | undefined>();
  const setCurrentTask = useCallback(
    (nextTask?: ChatTask) => {
      const previousTask = taskRef.current;
      taskRef.current = nextTask;
      setTask(nextTask);

      const nextTitle = nextTask?.title?.trim();
      if (
        !nextTask ||
        !nextTitle ||
        previousTask?.id !== nextTask.id ||
        previousTask.title?.trim() === nextTitle
      ) {
        return;
      }

      emitThreadTitlePatch({
        spaceID,
        task: {
          ...nextTask,
          title: nextTitle,
        },
      });
    },
    [setTask, spaceID],
  );
  const handleThreadTitleUpdated = useCallback(
    ({ threadId, title }: { threadId: string; title: string }) => {
      const nextTask = buildThreadTitleUpdatedTask({
        currentTask: taskRef.current,
        threadId,
        title,
      });
      if (!nextTask) {
        return;
      }

      setCurrentTask(nextTask);
    },
    [setCurrentTask],
  );

  return {
    handleThreadTitleUpdated,
    setCurrentTask,
  };
};
