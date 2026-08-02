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

import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  createEditorState,
  EMPTY_MEMORY,
  MEMORY_PAGE_SIZE,
  parseNumberField,
  type MemoryEditorState,
  type MemoryScopeFilter,
} from './task-memory-section-utils';
import {
  NO_MEMORY_WRITE_PERMISSION,
  useTaskMemoryListActions,
} from './task-memory-list-actions';
import { useTaskMemoryImportExport } from './task-memory-import-export-hooks';
import {
  listTaskThreadMemoryAuditEvents,
  listTaskThreadMemories,
  type TaskThreadMemory,
  type TaskThreadMemoryAuditEvent,
  updateTaskThreadMemory,
} from './service';

const buildListRequest = ({
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
    page: number;
    page_size: number;
    q?: string;
    scope?: string;
    space_id: string;
    thread_id: string;
  } = {
    page: 1,
    page_size: MEMORY_PAGE_SIZE,
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

const buildUpdateParams = ({
  activeMemory,
  editorState,
  spaceId,
  threadId,
}: {
  activeMemory: TaskThreadMemory;
  editorState: MemoryEditorState;
  spaceId: string;
  threadId: string;
}) => {
  const content = editorState.content.trim();
  const scopeValue = editorState.scope.trim();
  if (!content) {
    return { error: '记忆内容不能为空' };
  }
  if (!scopeValue) {
    return { error: '记忆范围不能为空' };
  }

  const score = parseNumberField(editorState.score, activeMemory.score);
  const confidence = parseNumberField(
    editorState.confidence,
    activeMemory.confidence,
  );
  const correctedAt = parseNumberField(editorState.correctedAt);
  const expiresAt = parseNumberField(editorState.expiresAt);
  if (
    [score, confidence, correctedAt, expiresAt].some(value =>
      Number.isNaN(value),
    )
  ) {
    return { error: '评分、置信度和时间字段必须是数字' };
  }

  return {
    params: {
      confidence,
      content,
      corrected_at: correctedAt,
      correction_of_memory_id: editorState.correctionOfMemoryID.trim(),
      expires_at: expiresAt,
      memory_id: activeMemory.memory_id,
      metadata: editorState.metadata,
      run_id: editorState.runID.trim(),
      scope: scopeValue,
      score,
      source_id: editorState.sourceID.trim(),
      source_type: editorState.sourceType.trim(),
      space_id: spaceId,
      thread_id: threadId,
    },
  };
};

const useTaskMemoryAudit = ({
  spaceId,
  threadId,
}: {
  spaceId?: string;
  threadId?: string;
}) => {
  const [activeAuditMemory, setActiveAuditMemory] =
    useState<TaskThreadMemory>();
  const [auditError, setAuditError] = useState('');
  const [auditEvents, setAuditEvents] = useState<TaskThreadMemoryAuditEvent[]>(
    [],
  );
  const [auditLoading, setAuditLoading] = useState(false);
  const [auditTotal, setAuditTotal] = useState(0);

  const handleOpenAudit = async (memory: TaskThreadMemory) => {
    if (!spaceId || !threadId) {
      return;
    }
    setActiveAuditMemory(memory);
    setAuditError('');
    setAuditEvents([]);
    setAuditTotal(0);
    setAuditLoading(true);
    try {
      const response = await listTaskThreadMemoryAuditEvents({
        memory_id: memory.memory_id,
        page: 1,
        page_size: MEMORY_PAGE_SIZE,
        space_id: spaceId,
        thread_id: threadId,
      });
      setAuditEvents(response.data?.events ?? []);
      setAuditTotal(response.data?.total ?? 0);
    } catch (err) {
      setAuditError(err instanceof Error ? err.message : '读取记忆审计失败');
    } finally {
      setAuditLoading(false);
    }
  };

  return {
    activeAuditMemory,
    auditError,
    auditEvents,
    auditLoading,
    auditTotal,
    handleOpenAudit,
    setActiveAuditMemory,
  };
};

const useTaskMemoryEditor = ({
  loadMemories,
  readOnly,
  setActiveAction,
  setError,
  spaceId,
  threadId,
}: {
  loadMemories: () => Promise<void>;
  readOnly: boolean;
  setActiveAction: (action: string) => void;
  setError: (message: string) => void;
  spaceId?: string;
  threadId?: string;
}) => {
  const [activeMemory, setActiveMemory] = useState<TaskThreadMemory>();
  const [editorError, setEditorError] = useState('');
  const [editorState, setEditorState] = useState<MemoryEditorState>(
    createEditorState(EMPTY_MEMORY),
  );

  const handleOpenEditor = (memory: TaskThreadMemory) => {
    if (readOnly) {
      setError(NO_MEMORY_WRITE_PERMISSION);
      return;
    }
    setEditorError('');
    setActiveMemory(memory);
    setEditorState(createEditorState(memory));
  };
  const handleEditorChange = (patch: Partial<MemoryEditorState>) => {
    setEditorState(prev => ({ ...prev, ...patch }));
  };
  const handleSaveMemory = async () => {
    if (!activeMemory || !spaceId || !threadId) {
      return;
    }
    if (readOnly) {
      setEditorError(NO_MEMORY_WRITE_PERMISSION);
      return;
    }
    const result = buildUpdateParams({
      activeMemory,
      editorState,
      spaceId,
      threadId,
    });
    if (result.error || !result.params) {
      setEditorError(result.error ?? '更新任务记忆失败');
      return;
    }
    setActiveAction(`save:${activeMemory.memory_id}`);
    setEditorError('');
    try {
      await updateTaskThreadMemory(result.params);
      setActiveMemory(undefined);
      await loadMemories();
    } catch (err) {
      setEditorError(err instanceof Error ? err.message : '更新任务记忆失败');
    } finally {
      setActiveAction('');
    }
  };

  return {
    activeMemory,
    editorError,
    editorState,
    handleEditorChange,
    handleOpenEditor,
    handleSaveMemory,
    setActiveMemory,
  };
};

export const useTaskMemorySection = ({
  readOnly = false,
  spaceId,
  threadId,
}: {
  readOnly?: boolean;
  spaceId?: string;
  threadId?: string;
}) => {
  const [activeAction, setActiveAction] = useState('');
  const [includeDeleted, setIncludeDeleted] = useState(false);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [memories, setMemories] = useState<TaskThreadMemory[]>([]);
  const [notice, setNotice] = useState('');
  const [query, setQuery] = useState('');
  const [scope, setScope] = useState<MemoryScopeFilter>('all');
  const [total, setTotal] = useState(0);
  const audit = useTaskMemoryAudit({ spaceId, threadId });
  const listRequest = useMemo(
    () =>
      buildListRequest({
        includeDeleted,
        query,
        scope,
        spaceId: spaceId ?? '',
        threadId: threadId ?? '',
      }),
    [includeDeleted, query, scope, spaceId, threadId],
  );
  const loadMemories = useCallback(async () => {
    if (!spaceId || !threadId) {
      return;
    }
    setLoading(true);
    setError('');
    setNotice('');
    try {
      const response = await listTaskThreadMemories(listRequest);
      setMemories(response.data?.memories ?? []);
      setTotal(response.data?.total ?? 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : '读取任务记忆失败');
    } finally {
      setLoading(false);
    }
  }, [listRequest, spaceId, threadId]);

  useEffect(() => {
    void loadMemories();
  }, [loadMemories]);

  const editor = useTaskMemoryEditor({
    loadMemories,
    readOnly,
    setActiveAction,
    setError,
    spaceId,
    threadId,
  });
  const importExport = useTaskMemoryImportExport({
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
  });
  const listActions = useTaskMemoryListActions({
    loadMemories,
    readOnly,
    scope,
    setActiveAction,
    setError,
    spaceId,
    threadId,
  });
  return {
    activeAction,
    activeAuditMemory: audit.activeAuditMemory,
    activeMemory: editor.activeMemory,
    auditError: audit.auditError,
    auditEvents: audit.auditEvents,
    auditLoading: audit.auditLoading,
    auditTotal: audit.auditTotal,
    editorError: editor.editorError,
    editorState: editor.editorState,
    error,
    handleClearMemories: listActions.handleClearMemories,
    handleDeleteMemory: listActions.handleDeleteMemory,
    handleEditorChange: editor.handleEditorChange,
    handleCloseImport: importExport.handleCloseImport,
    handleExportMemories: importExport.handleExportMemories,
    handleImportMemories: importExport.handleImportMemories,
    handleOpenImport: importExport.handleOpenImport,
    handleOpenAudit: audit.handleOpenAudit,
    handleOpenEditor: editor.handleOpenEditor,
    handleRestoreMemory: listActions.handleRestoreMemory,
    handleSaveMemory: editor.handleSaveMemory,
    includeDeleted,
    importError: importExport.importError,
    importText: importExport.importText,
    importVisible: importExport.importVisible,
    loadMemories,
    loading,
    memories,
    notice,
    query,
    readOnly,
    scope,
    setActiveAuditMemory: audit.setActiveAuditMemory,
    setActiveMemory: editor.setActiveMemory,
    setIncludeDeleted,
    setImportText: importExport.setImportText,
    setQuery,
    setScope,
    total,
  };
};
