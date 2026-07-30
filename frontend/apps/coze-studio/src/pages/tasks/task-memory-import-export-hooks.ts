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

import { useState } from 'react';

import {
  MEMORY_EXPORT_LIMIT,
  MEMORY_EXPORT_SCHEMA,
  parseMemoryImportPayload,
  type MemoryScopeFilter,
} from './task-memory-section-utils';
import { NO_MEMORY_WRITE_PERMISSION } from './task-memory-list-actions';
import {
  exportTaskThreadMemories,
  importTaskThreadMemories,
  type ExportTaskThreadMemoriesData,
} from './service';

const buildExportRequest = ({
  includeDeleted,
  query,
  scope,
  spaceId,
  threadId,
}: {
  includeDeleted: boolean;
  query: string;
  scope: MemoryScopeFilter;
  spaceId: string;
  threadId: string;
}) => {
  const request: {
    include_deleted?: boolean;
    limit: number;
    q?: string;
    scope?: string;
    space_id: string;
    thread_id: string;
  } = {
    limit: MEMORY_EXPORT_LIMIT,
    space_id: spaceId,
    thread_id: threadId,
  };
  const trimmedQuery = query.trim();
  if (trimmedQuery) {
    request.q = trimmedQuery;
  }
  if (scope !== 'all') {
    request.scope = scope;
  }
  if (includeDeleted) {
    request.include_deleted = true;
  }

  return request;
};

const downloadMemoryExport = ({
  data,
  threadId,
}: {
  data: ExportTaskThreadMemoriesData;
  threadId: string;
}) => {
  const payload = {
    exported_at: data.exported_at,
    memories: data.memories ?? [],
    schema: data.schema || MEMORY_EXPORT_SCHEMA,
    thread_id: data.thread_id || threadId,
    total: data.total ?? data.memories?.length ?? 0,
  };
  const blob = new Blob([JSON.stringify(payload, null, 2)], {
    type: 'application/json',
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `task-thread-${threadId}-memories.json`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

export const useTaskMemoryImportExport = ({
  includeDeleted,
  loadMemories,
  query,
  readOnly,
  scope,
  setActiveAction,
  setError,
  setNotice,
  spaceId,
  threadId,
}: {
  includeDeleted: boolean;
  loadMemories: () => Promise<void>;
  query: string;
  readOnly: boolean;
  scope: MemoryScopeFilter;
  setActiveAction: (action: string) => void;
  setError: (message: string) => void;
  setNotice: (message: string) => void;
  spaceId?: string;
  threadId?: string;
}) => {
  const [importError, setImportError] = useState('');
  const [importText, setImportText] = useState('');
  const [importVisible, setImportVisible] = useState(false);

  const handleExportMemories = async () => {
    if (!spaceId || !threadId) {
      return;
    }
    setActiveAction('export');
    setError('');
    setNotice('');
    try {
      const response = await exportTaskThreadMemories(
        buildExportRequest({
          includeDeleted,
          query,
          scope,
          spaceId,
          threadId,
        }),
      );
      if (!response.data) {
        throw new Error('导出任务记忆响应为空');
      }
      downloadMemoryExport({ data: response.data, threadId });
      setNotice(`已导出 ${response.data.total ?? 0} 条任务记忆`);
    } catch (err) {
      setError(err instanceof Error ? err.message : '导出任务记忆失败');
    } finally {
      setActiveAction('');
    }
  };
  const handleOpenImport = () => {
    if (readOnly) {
      setError(NO_MEMORY_WRITE_PERMISSION);
      return;
    }
    setImportError('');
    setImportText('');
    setImportVisible(true);
  };
  const handleCloseImport = () => {
    setImportError('');
    setImportVisible(false);
  };
  const handleImportMemories = async () => {
    if (!spaceId || !threadId) {
      return;
    }
    if (readOnly) {
      setImportError(NO_MEMORY_WRITE_PERMISSION);
      return;
    }
    const parsed = parseMemoryImportPayload(importText);
    if (parsed.error) {
      setImportError(parsed.error);
      return;
    }

    setActiveAction('import');
    setError('');
    setImportError('');
    setNotice('');
    try {
      const response = await importTaskThreadMemories({
        memories: parsed.memories,
        space_id: spaceId,
        thread_id: threadId,
      });
      setImportText('');
      setImportVisible(false);
      await loadMemories();
      setNotice(
        `已导入 ${response.data?.imported ?? 0} 条，跳过 ${
          response.data?.skipped ?? 0
        } 条`,
      );
    } catch (err) {
      setImportError(err instanceof Error ? err.message : '导入任务记忆失败');
    } finally {
      setActiveAction('');
    }
  };

  return {
    handleCloseImport,
    handleExportMemories,
    handleImportMemories,
    handleOpenImport,
    importError,
    importText,
    importVisible,
    setImportText,
  };
};
