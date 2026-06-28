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

import {
  IconCozDownload,
  IconCozImport,
  IconCozMagnifier,
  IconCozRefresh,
  IconCozTrashCan,
} from '@coze-arch/coze-design/icons';
import {
  Button,
  Input,
  Popconfirm,
  SideSheet,
  Tag,
  TextArea,
} from '@coze-arch/coze-design';

import {
  buildAuditEventMeta,
  MEMORY_SCOPE_OPTIONS,
  type MemoryEditorState,
  type MemoryScopeFilter,
} from './task-memory-section-utils';
import { useTaskMemorySection } from './task-memory-section-hooks';
import { TaskMemoryRow } from './task-memory-row';
import { MemoryImportSheet } from './task-memory-import-sheet';
import type { TaskThreadMemory, TaskThreadMemoryAuditEvent } from './service';

const MemoryScopeSwitch = ({
  disabled,
  value,
  onChange,
}: {
  disabled?: boolean;
  value: MemoryScopeFilter;
  onChange: (scope: MemoryScopeFilter) => void;
}) => (
  <div className="coze-prototype-memory-scope-switch" aria-label="记忆范围">
    {MEMORY_SCOPE_OPTIONS.map(option => (
      <Button
        key={option.value}
        size="small"
        theme={value === option.value ? 'solid' : 'borderless'}
        type={value === option.value ? 'primary' : 'tertiary'}
        disabled={disabled}
        onClick={() => onChange(option.value)}
      >
        {option.label}
      </Button>
    ))}
  </div>
);

const MemoryEditSheet = ({
  activeMemory,
  editorError,
  editorState,
  saving,
  onCancel,
  onChange,
  onSave,
}: {
  activeMemory?: TaskThreadMemory;
  editorError?: string;
  editorState: MemoryEditorState;
  saving: boolean;
  onCancel: () => void;
  onChange: (patch: Partial<MemoryEditorState>) => void;
  onSave: () => void | Promise<void>;
}) => (
  <SideSheet
    title="编辑记忆"
    visible={Boolean(activeMemory)}
    onCancel={onCancel}
    width={520}
  >
    <div className="coze-prototype-memory-editor">
      {editorError ? (
        <div className="coze-prototype-error" role="alert">
          {editorError}
        </div>
      ) : null}
      <TextArea
        aria-label="记忆内容"
        value={editorState.content}
        rows={5}
        autosize={false}
        placeholder="记忆内容"
        onChange={content => onChange({ content })}
      />
      <MemoryEditFields editorState={editorState} onChange={onChange} />
      <TextArea
        aria-label="记忆元数据"
        value={editorState.metadata}
        rows={4}
        autosize={false}
        placeholder="metadata JSON"
        onChange={metadata => onChange({ metadata })}
      />
      <div className="coze-prototype-memory-editor-actions">
        <Button theme="borderless" type="tertiary" onClick={onCancel}>
          取消
        </Button>
        <Button theme="solid" type="primary" loading={saving} onClick={onSave}>
          保存
        </Button>
      </div>
    </div>
  </SideSheet>
);

const MemoryEditFields = ({
  editorState,
  onChange,
}: {
  editorState: MemoryEditorState;
  onChange: (patch: Partial<MemoryEditorState>) => void;
}) => (
  <div className="coze-prototype-memory-editor-grid">
    <Input
      aria-label="记忆范围值"
      value={editorState.scope}
      placeholder="thread / run / long_term"
      onChange={scope => onChange({ scope })}
    />
    <Input
      aria-label="记忆运行 ID"
      value={editorState.runID}
      placeholder="run_id"
      onChange={runID => onChange({ runID })}
    />
    <Input
      aria-label="记忆来源类型"
      value={editorState.sourceType}
      placeholder="source_type"
      onChange={sourceType => onChange({ sourceType })}
    />
    <Input
      aria-label="记忆来源 ID"
      value={editorState.sourceID}
      placeholder="source_id"
      onChange={sourceID => onChange({ sourceID })}
    />
    <Input
      aria-label="记忆评分"
      value={editorState.score}
      placeholder="score"
      onChange={score => onChange({ score })}
    />
    <Input
      aria-label="记忆置信度"
      value={editorState.confidence}
      placeholder="confidence"
      onChange={confidence => onChange({ confidence })}
    />
    <Input
      aria-label="记忆修正来源"
      value={editorState.correctionOfMemoryID}
      placeholder="correction_of_memory_id"
      onChange={correctionOfMemoryID => onChange({ correctionOfMemoryID })}
    />
    <Input
      aria-label="记忆修正时间"
      value={editorState.correctedAt}
      placeholder="corrected_at"
      onChange={correctedAt => onChange({ correctedAt })}
    />
    <Input
      aria-label="记忆过期时间"
      value={editorState.expiresAt}
      placeholder="expires_at"
      onChange={expiresAt => onChange({ expiresAt })}
    />
  </div>
);

const MemoryAuditSheet = ({
  activeMemory,
  error,
  events,
  loading,
  total,
  onCancel,
}: {
  activeMemory?: TaskThreadMemory;
  error?: string;
  events: TaskThreadMemoryAuditEvent[];
  loading: boolean;
  total: number;
  onCancel: () => void;
}) => (
  <SideSheet
    title="记忆审计"
    visible={Boolean(activeMemory)}
    onCancel={onCancel}
    width={520}
  >
    <div
      className="coze-prototype-memory-audit"
      data-testid="task-memory-audit-sheet"
    >
      <div className="coze-prototype-memory-audit-summary">
        {activeMemory?.memory_id ? `记忆 ${activeMemory.memory_id}` : '记忆'}
        <span>{total} 条</span>
      </div>
      {error ? (
        <div className="coze-prototype-memory-error" role="alert">
          {error}
        </div>
      ) : null}
      {events.length ? (
        <ol className="coze-prototype-memory-audit-list">
          {events.map(event => (
            <li
              key={event.event_id}
              className="coze-prototype-memory-audit-row"
              data-testid="task-memory-audit-row"
            >
              <span className="coze-prototype-memory-audit-title">
                {event.event_type}
              </span>
              <span className="coze-prototype-memory-meta">
                {buildAuditEventMeta(event)}
              </span>
            </li>
          ))}
        </ol>
      ) : (
        <div className="coze-prototype-memory-empty">
          {loading ? '加载记忆审计中...' : '暂无记忆审计'}
        </div>
      )}
    </div>
  </SideSheet>
);

type TaskMemorySectionState = ReturnType<typeof useTaskMemorySection>;

const MemoryToolbar = ({ state }: { state: TaskMemorySectionState }) => (
  <div className="coze-prototype-memory-toolbar">
    <Input
      aria-label="搜索记忆"
      data-testid="task-memory-search"
      value={state.query}
      placeholder="搜索内容、来源"
      showClear
      onChange={state.setQuery}
      onEnterPress={() => void state.loadMemories()}
    />
    <Button
      icon={<IconCozMagnifier />}
      data-testid="task-memory-search-submit"
      loading={state.loading}
      size="small"
      theme="solid"
      type="primary"
      onClick={() => void state.loadMemories()}
    >
      搜索
    </Button>
    <Button
      icon={<IconCozRefresh />}
      data-testid="task-memory-refresh"
      loading={state.loading}
      size="small"
      theme="borderless"
      type="tertiary"
      onClick={() => void state.loadMemories()}
    >
      刷新
    </Button>
    <Button
      icon={<IconCozDownload />}
      data-testid="task-memory-export"
      loading={state.activeAction === 'export'}
      size="small"
      theme="borderless"
      type="tertiary"
      onClick={() => void state.handleExportMemories()}
    >
      导出记忆
    </Button>
    <Button
      disabled={state.readOnly}
      icon={<IconCozImport />}
      data-testid="task-memory-import"
      loading={state.activeAction === 'import'}
      size="small"
      theme="borderless"
      type="tertiary"
      onClick={state.handleOpenImport}
    >
      导入记忆
    </Button>
    <Button
      data-testid="task-memory-toggle-deleted"
      loading={state.loading}
      size="small"
      theme={state.includeDeleted ? 'solid' : 'borderless'}
      type={state.includeDeleted ? 'primary' : 'tertiary'}
      onClick={() => state.setIncludeDeleted(!state.includeDeleted)}
    >
      已删除
    </Button>
    {state.readOnly ? (
      <Button
        disabled
        icon={<IconCozTrashCan />}
        data-testid="task-memory-clear"
        size="small"
        theme="borderless"
        type="danger"
      >
        清空
      </Button>
    ) : (
      <Popconfirm
        title="清空任务记忆"
        content="会按当前范围清空任务记忆记录，并写入审计元数据。"
        okText="清空"
        cancelText="取消"
        okType="danger"
        cancelButtonProps={{ autoFocus: true }}
        onConfirm={state.handleClearMemories}
      >
        <Button
          icon={<IconCozTrashCan />}
          data-testid="task-memory-clear"
          loading={state.activeAction === 'clear'}
          size="small"
          theme="borderless"
          type="danger"
        >
          清空
        </Button>
      </Popconfirm>
    )}
  </div>
);

export const TaskMemorySection = ({
  readOnly,
  threadId,
}: {
  readOnly?: boolean;
  threadId?: string;
}) => {
  const state = useTaskMemorySection({ readOnly, threadId });

  if (!threadId) {
    return null;
  }

  return (
    <section
      className="coze-prototype-memory-panel"
      data-testid="task-memory-panel"
    >
      <div className="coze-prototype-memory-header">
        <h2>任务记忆</h2>
        {state.readOnly ? (
          <Tag data-testid="task-memory-readonly">只读</Tag>
        ) : null}
        <span>{state.total} 条</span>
      </div>
      <MemoryToolbar state={state} />
      <MemoryScopeSwitch
        disabled={state.loading}
        value={state.scope}
        onChange={state.setScope}
      />
      {state.error ? (
        <div className="coze-prototype-memory-error" role="alert">
          {state.error}
        </div>
      ) : null}
      {state.notice ? (
        <div className="coze-prototype-memory-notice" role="status">
          {state.notice}
        </div>
      ) : null}
      {state.memories.length ? (
        <ol className="coze-prototype-memory-list">
          {state.memories.map(memory => (
            <TaskMemoryRow
              key={memory.memory_id}
              activeAction={state.activeAction}
              memory={memory}
              readOnly={state.readOnly}
              onAudit={state.handleOpenAudit}
              onDelete={state.handleDeleteMemory}
              onEdit={state.handleOpenEditor}
              onRestore={state.handleRestoreMemory}
            />
          ))}
        </ol>
      ) : (
        <div className="coze-prototype-memory-empty">
          {state.loading ? '加载任务记忆中...' : '暂无任务记忆'}
        </div>
      )}
      <MemoryEditSheet
        activeMemory={state.activeMemory}
        editorError={state.editorError}
        editorState={state.editorState}
        saving={state.activeAction.startsWith('save:')}
        onCancel={() => state.setActiveMemory(undefined)}
        onChange={state.handleEditorChange}
        onSave={state.handleSaveMemory}
      />
      <MemoryAuditSheet
        activeMemory={state.activeAuditMemory}
        error={state.auditError}
        events={state.auditEvents}
        loading={state.auditLoading}
        total={state.auditTotal}
        onCancel={() => state.setActiveAuditMemory(undefined)}
      />
      <MemoryImportSheet
        error={state.importError}
        importing={state.activeAction === 'import'}
        value={state.importText}
        visible={state.importVisible}
        onCancel={state.handleCloseImport}
        onChange={state.setImportText}
        onImport={state.handleImportMemories}
      />
    </section>
  );
};
