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

import type {
  ImportTaskThreadMemoryItem,
  TaskThreadMemory,
  TaskThreadMemoryAuditEvent,
} from './service';
import { formatUpdatedTime } from './helpers';

export type MemoryScopeFilter = 'all' | 'thread' | 'run' | 'long_term';

export interface MemoryEditorState {
  confidence: string;
  content: string;
  correctedAt: string;
  correctionOfMemoryID: string;
  expiresAt: string;
  metadata: string;
  runID: string;
  scope: string;
  score: string;
  sourceID: string;
  sourceType: string;
}

export const MEMORY_PAGE_SIZE = 20;
export const MEMORY_EXPORT_SCHEMA = 'coze.task_thread_memories.export.v1';
export const MEMORY_EXPORT_LIMIT = 100;

export const MEMORY_SCOPE_OPTIONS: Array<{
  label: string;
  value: MemoryScopeFilter;
}> = [
  { label: '全部', value: 'all' },
  { label: '线程', value: 'thread' },
  { label: '运行', value: 'run' },
  { label: '长期', value: 'long_term' },
];

const CLEAR_ALL_SCOPES = ['thread', 'run', 'long_term'];

export const EMPTY_MEMORY: TaskThreadMemory = {
  confidence: 0,
  content: '',
  corrected_at: 0,
  correction_of_memory_id: '',
  created_at: 0,
  deleted_at: 0,
  expires_at: 0,
  memory_id: '',
  metadata: '',
  run_id: '',
  scope: 'thread',
  score: 0,
  source_id: '',
  source_type: '',
  space_id: '',
  thread_id: '',
  updated_at: 0,
};

export const scopeLabel = (scope: string) => {
  switch (scope) {
    case 'thread':
      return 'thread';
    case 'run':
      return 'run';
    case 'long_term':
      return 'long_term';
    default:
      return scope || 'unknown';
  }
};

export const createEditorState = (
  memory: TaskThreadMemory,
): MemoryEditorState => ({
  confidence: String(memory.confidence ?? 0),
  content: memory.content,
  correctedAt: memory.corrected_at ? String(memory.corrected_at) : '',
  correctionOfMemoryID: memory.correction_of_memory_id ?? '',
  expiresAt: memory.expires_at ? String(memory.expires_at) : '',
  metadata: memory.metadata ?? '',
  runID: memory.run_id ?? '',
  scope: memory.scope || 'thread',
  score: String(memory.score ?? 0),
  sourceID: memory.source_id ?? '',
  sourceType: memory.source_type ?? '',
});

export const parseNumberField = (value: string, fallback = 0) => {
  const trimmed = value.trim();
  if (!trimmed) {
    return fallback;
  }

  const parsed = Number(trimmed);
  return Number.isFinite(parsed) ? parsed : Number.NaN;
};

export const buildSourceLabel = (memory: TaskThreadMemory) =>
  [memory.source_type, memory.source_id].filter(Boolean).join('/');

export const buildMemoryMeta = (memory: TaskThreadMemory) =>
  [
    scopeLabel(memory.scope),
    buildSourceLabel(memory),
    memory.confidence > 0 ? `置信度 ${memory.confidence.toFixed(2)}` : '',
    memory.expires_at ? `过期 ${formatUpdatedTime(memory.expires_at)}` : '',
    `更新 ${formatUpdatedTime(memory.updated_at || memory.created_at)}`,
  ]
    .filter(Boolean)
    .join(' · ');

export const buildAuditEventMeta = (event: TaskThreadMemoryAuditEvent) =>
  [
    scopeLabel(event.scope),
    [event.source_type, event.source_id].filter(Boolean).join('/'),
    event.affected_count > 0 ? `影响 ${event.affected_count}` : '',
    `记录 ${formatUpdatedTime(event.created_at)}`,
  ]
    .filter(Boolean)
    .join(' · ');

export const getClearScopes = (scope: MemoryScopeFilter) =>
  scope === 'all' ? CLEAR_ALL_SCOPES : [scope];

export const isDeletedMemory = (memory: TaskThreadMemory) =>
  memory.deleted_at > 0;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const readString = (record: Record<string, unknown>, key: string) => {
  const value = record[key];

  return typeof value === 'string' ? value : '';
};

const readTrimmedString = (record: Record<string, unknown>, key: string) =>
  readString(record, key).trim();

const readNumber = (record: Record<string, unknown>, key: string) => {
  const value = record[key];
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === 'string' && value.trim()) {
    const parsed = Number(value);

    return Number.isFinite(parsed) ? parsed : undefined;
  }

  return undefined;
};

export const parseMemoryImportPayload = (
  payload: string,
): { error?: string; memories: ImportTaskThreadMemoryItem[] } => {
  const trimmed = payload.trim();
  if (!trimmed) {
    return { error: '请粘贴任务记忆导出 JSON', memories: [] };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return { error: '导入 JSON 格式不正确', memories: [] };
  }

  if (!isRecord(parsed)) {
    return { error: '导入 JSON 必须是对象', memories: [] };
  }
  if (parsed.schema !== MEMORY_EXPORT_SCHEMA) {
    return { error: '导入 JSON schema 不匹配', memories: [] };
  }
  if (!Array.isArray(parsed.memories)) {
    return { error: '导入 JSON 缺少 memories 数组', memories: [] };
  }
  if (!parsed.memories.length) {
    return { error: '导入 JSON 没有可导入的任务记忆', memories: [] };
  }
  if (parsed.memories.length > MEMORY_EXPORT_LIMIT) {
    return {
      error: `单次最多导入 ${MEMORY_EXPORT_LIMIT} 条任务记忆`,
      memories: [],
    };
  }

  const memories: ImportTaskThreadMemoryItem[] = [];
  for (let index = 0; index < parsed.memories.length; index += 1) {
    const rawMemory = parsed.memories[index];
    if (!isRecord(rawMemory)) {
      return { error: `第 ${index + 1} 条任务记忆格式不正确`, memories: [] };
    }

    const content = readTrimmedString(rawMemory, 'content');
    if (!content) {
      return { error: `第 ${index + 1} 条任务记忆内容不能为空`, memories: [] };
    }

    const memory: ImportTaskThreadMemoryItem = { content };
    const metadata = readString(rawMemory, 'metadata');
    const runID = readTrimmedString(rawMemory, 'run_id');
    const scope = readTrimmedString(rawMemory, 'scope');
    const sourceType = readTrimmedString(rawMemory, 'source_type');
    const sourceID = readTrimmedString(rawMemory, 'source_id');
    const correctionOfMemoryID = readTrimmedString(
      rawMemory,
      'correction_of_memory_id',
    );
    const score = readNumber(rawMemory, 'score');
    const confidence = readNumber(rawMemory, 'confidence');
    const correctedAt = readNumber(rawMemory, 'corrected_at');
    const expiresAt = readNumber(rawMemory, 'expires_at');

    if (metadata) {
      memory.metadata = metadata;
    }
    if (runID) {
      memory.run_id = runID;
    }
    if (scope) {
      memory.scope = scope;
    }
    if (typeof score === 'number') {
      memory.score = score;
    }
    if (typeof confidence === 'number') {
      memory.confidence = confidence;
    }
    if (sourceType) {
      memory.source_type = sourceType;
    }
    if (sourceID) {
      memory.source_id = sourceID;
    }
    if (correctionOfMemoryID) {
      memory.correction_of_memory_id = correctionOfMemoryID;
    }
    if (typeof correctedAt === 'number') {
      memory.corrected_at = correctedAt;
    }
    if (typeof expiresAt === 'number') {
      memory.expires_at = expiresAt;
    }
    memories.push(memory);
  }

  return { memories };
};
