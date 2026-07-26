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
  getSafeTaskToolDetail,
  getSafeTaskToolName,
} from './task-tool-event-safety';
import type { TaskThreadEventDisplay, TaskExecutionType } from './helpers';

const getString = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return typeof value === 'string' && value.trim() ? value.trim() : undefined;
};

const getBoolean = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return typeof value === 'boolean' ? value : undefined;
};

const getObject = (
  payload: Record<string, unknown> | undefined,
  key: string,
) => {
  const value = payload?.[key];

  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
};

const getFirstString = (
  payload: Record<string, unknown> | undefined,
  keys: string[],
) => {
  for (const key of keys) {
    const value = getString(payload, key);

    if (value) {
      return value;
    }
  }

  return undefined;
};

const fileNameFromPath = (path?: string) => {
  const normalized = path?.trim();

  if (!normalized) {
    return undefined;
  }

  return normalized.split('/').filter(Boolean).at(-1) ?? normalized;
};

const fileBaseName = (fileName?: string) => {
  if (!fileName) {
    return undefined;
  }

  const dotIndex = fileName.lastIndexOf('.');

  return dotIndex > 0 ? fileName.slice(0, dotIndex) : fileName;
};

const fileExtension = (fileName?: string) => {
  const dotIndex = fileName?.lastIndexOf('.') ?? -1;

  return dotIndex >= 0 ? fileName?.slice(dotIndex + 1).toLowerCase() : '';
};

const getDocumentTypeLabel = (fileName?: string) => {
  const extension = fileExtension(fileName);

  switch (extension) {
    case 'md':
    case 'markdown':
      return 'Markdown';
    case 'pdf':
      return 'PDF';
    case 'html':
    case 'htm':
      return 'HTML';
    case 'csv':
      return 'CSV';
    case 'json':
      return 'JSON';
    default:
      return extension ? extension.toUpperCase() : '文件';
  }
};

const getToolPath = (parsed: Record<string, unknown> | undefined) => {
  const args = getObject(parsed, 'args') ?? getObject(parsed, 'arguments');
  const result = getObject(parsed, 'result');

  return (
    getFirstString(parsed, [
      'path',
      'file_path',
      'filepath',
      'virtual_path',
      'output_path',
      'detail',
    ]) ??
    getFirstString(args, [
      'path',
      'file_path',
      'filepath',
      'virtual_path',
      'output_path',
    ]) ??
    getFirstString(result, [
      'path',
      'file_path',
      'filepath',
      'virtual_path',
      'output_path',
    ])
  );
};

const compactSafeToolDetail = (value?: string) => {
  const safeValue = getSafeTaskToolDetail(value, '');

  if (safeValue.length <= 80) {
    return safeValue;
  }

  return `${safeValue.slice(0, 80)}...`;
};

const getWebSearchQuery = (parsed: Record<string, unknown> | undefined) => {
  const args = getObject(parsed, 'args') ?? getObject(parsed, 'arguments');
  const query =
    getFirstString(parsed, ['query', 'search_query', 'q']) ??
    getFirstString(args, ['query', 'search_query', 'q']);

  return compactSafeToolDetail(query);
};

const getWebSearchTitle = (
  parsed: Record<string, unknown> | undefined,
  fallback = '搜索网页',
) => {
  const query = getWebSearchQuery(parsed);

  return query ? `搜索网页：“${query}”` : fallback;
};

const getSkillToolName = (parsed: Record<string, unknown> | undefined) =>
  getSafeTaskToolDetail(getFirstString(parsed, ['skill_name', 'skill']), '');

const getToolCompletedTitle = ({
  parsed,
  title,
  toolName,
}: {
  parsed: Record<string, unknown> | undefined;
  title?: string;
  toolName: string;
}) => {
  if (title) {
    return title;
  }

  if (toolName === 'write_todos') {
    return '更新 To-do 列表';
  }

  if (toolName === 'present_files') {
    return '展示文件';
  }

  if (toolName === 'web_search') {
    return getWebSearchTitle(parsed);
  }

  if (toolName === 'skill') {
    const skillName = getSkillToolName(parsed);

    return skillName ? `使用 “${skillName}” 技能` : '使用技能';
  }

  if (toolName === 'write_file' || toolName === 'str_replace') {
    const fileName = fileNameFromPath(getToolPath(parsed));
    const baseName = fileBaseName(fileName);
    const documentType = getDocumentTypeLabel(fileName);

    if (baseName) {
      return `创建${baseName} ${documentType} 文档`;
    }

    return toolName === 'str_replace' ? '编辑文件' : '创建文件';
  }

  return `使用 “${toolName}” 工具`;
};

export const getTaskToolEventDisplay = ({
  detail,
  eventType,
  parsed,
  runtime,
  title,
}: {
  detail?: string;
  eventType?: string;
  parsed?: Record<string, unknown>;
  runtime?: TaskExecutionType;
  title?: string;
}): TaskThreadEventDisplay | undefined => {
  if (!eventType?.startsWith('tool.')) {
    return undefined;
  }

  const toolName = getSafeTaskToolName(
    getString(parsed, 'tool_name') ||
      getString(parsed, 'step_name') ||
      getString(parsed, 'step_id'),
  );
  const errorMessage = getString(parsed, 'error_message');
  const argumentsPresent = getBoolean(parsed, 'arguments_present');
  const toolPath = getToolPath(parsed);
  const baseDisplay = {
    runtime: runtime ?? 'Agent',
    structured: true,
    kind: 'step' as const,
  };

  if (eventType === 'tool.started') {
    if (toolName === 'skill') {
      const skillName = getSkillToolName(parsed);

      return {
        ...baseDisplay,
        title: skillName ? `加载 “${skillName}” 技能` : '加载技能',
        detail: getSafeTaskToolDetail(detail, ''),
        status: 'running',
      };
    }

    if (toolName === 'web_search') {
      return {
        ...baseDisplay,
        title: title ?? getWebSearchTitle(parsed, '搜索网页'),
        detail: getSafeTaskToolDetail(
          detail,
          argumentsPresent ? '参数已准备' : '',
        ),
        status: 'running',
      };
    }

    return {
      ...baseDisplay,
      title: title ?? `调用工具 ${toolName}`,
      detail: getSafeTaskToolDetail(
        detail,
        argumentsPresent ? '参数已准备' : '',
      ),
      status: 'running',
    };
  }

  if (eventType === 'tool.completed') {
    return {
      ...baseDisplay,
      title: getToolCompletedTitle({ parsed, title, toolName }),
      detail: getSafeTaskToolDetail(detail ?? toolPath, ''),
      status: 'completed',
    };
  }

  if (eventType === 'tool.failed') {
    if (toolName === 'skill') {
      const skillName = getSkillToolName(parsed);

      return {
        ...baseDisplay,
        title: skillName ? `“${skillName}” 技能加载失败` : '技能加载失败',
        detail: getSafeTaskToolDetail(detail, '工具调用失败，详情已隐藏'),
        status: 'failed',
      };
    }

    return {
      ...baseDisplay,
      title: `工具 ${toolName} 调用失败`,
      detail: getSafeTaskToolDetail(
        detail ?? errorMessage,
        '工具调用失败，详情已隐藏',
      ),
      status: 'failed',
    };
  }

  return undefined;
};
