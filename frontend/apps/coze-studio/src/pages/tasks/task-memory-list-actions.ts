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

import { useCallback } from 'react';

import {
  getClearScopes,
  type MemoryScopeFilter,
} from './task-memory-section-utils';
import {
  clearTaskThreadMemories,
  deleteTaskThreadMemory,
  restoreTaskThreadMemory,
  type TaskThreadMemory,
} from './service';

export const NO_MEMORY_WRITE_PERMISSION = '当前用户没有任务记忆写入权限';

const runMemoryListAction = async ({
  actionKey,
  fallbackError,
  loadMemories,
  operation,
  setActiveAction,
  setError,
}: {
  actionKey: string;
  fallbackError: string;
  loadMemories: () => Promise<void>;
  operation: () => Promise<unknown>;
  setActiveAction: (action: string) => void;
  setError: (message: string) => void;
}) => {
  setActiveAction(actionKey);
  setError('');
  try {
    await operation();
    await loadMemories();
  } catch (err) {
    setError(err instanceof Error ? err.message : fallbackError);
  } finally {
    setActiveAction('');
  }
};

export const useTaskMemoryListActions = ({
  loadMemories,
  readOnly,
  scope,
  setActiveAction,
  setError,
  threadId,
}: {
  loadMemories: () => Promise<void>;
  readOnly: boolean;
  scope: MemoryScopeFilter;
  setActiveAction: (action: string) => void;
  setError: (message: string) => void;
  threadId?: string;
}) => {
  const handleDeleteMemory = useCallback(
    async (memory: TaskThreadMemory) => {
      if (!threadId) {
        return;
      }
      if (readOnly) {
        setError(NO_MEMORY_WRITE_PERMISSION);
        return;
      }
      await runMemoryListAction({
        actionKey: `delete:${memory.memory_id}`,
        fallbackError: '删除任务记忆失败',
        loadMemories,
        operation: () =>
          deleteTaskThreadMemory({
            memory_id: memory.memory_id,
            thread_id: threadId,
          }),
        setActiveAction,
        setError,
      });
    },
    [loadMemories, readOnly, setActiveAction, setError, threadId],
  );

  const handleRestoreMemory = useCallback(
    async (memory: TaskThreadMemory) => {
      if (!threadId) {
        return;
      }
      if (readOnly) {
        setError(NO_MEMORY_WRITE_PERMISSION);
        return;
      }
      await runMemoryListAction({
        actionKey: `restore:${memory.memory_id}`,
        fallbackError: '恢复任务记忆失败',
        loadMemories,
        operation: () =>
          restoreTaskThreadMemory({
            memory_id: memory.memory_id,
            thread_id: threadId,
          }),
        setActiveAction,
        setError,
      });
    },
    [loadMemories, readOnly, setActiveAction, setError, threadId],
  );

  const handleClearMemories = useCallback(async () => {
    if (!threadId) {
      return;
    }
    if (readOnly) {
      setError(NO_MEMORY_WRITE_PERMISSION);
      return;
    }
    await runMemoryListAction({
      actionKey: 'clear',
      fallbackError: '清空任务记忆失败',
      loadMemories,
      operation: () =>
        clearTaskThreadMemories({
          scopes: getClearScopes(scope),
          thread_id: threadId,
        }),
      setActiveAction,
      setError,
    });
  }, [loadMemories, readOnly, scope, setActiveAction, setError, threadId]);

  return {
    handleClearMemories,
    handleDeleteMemory,
    handleRestoreMemory,
  };
};
