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
import { IconCozDownload } from '@coze-arch/coze-design/icons';

import type { TaskDetailTokenUsage } from './task-detail-loader';
import {
  getTaskInputText,
  getTaskResultText,
  getTaskStatusText,
} from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskThreadMessage = workbenchTask.TaskThreadMessage;

const INTERNAL_EXPORT_MARKERS = [
  /<think>[\s\S]*?<\/think>/gi,
  /<system-reminder>[\s\S]*?<\/system-reminder>/gi,
  /<uploaded_files>[\s\S]*?<\/uploaded_files>/gi,
];

const formatTokenCount = (value: number) =>
  new Intl.NumberFormat('en-US').format(Math.max(0, value));

const formatTaskExportTime = (value?: number) =>
  value && value > 0 ? new Date(value).toLocaleString() : '未知';

const sanitizeExportFilename = (name: string) =>
  name.replace(/[^\p{L}\p{N}_\- ]/gu, '').trim() || 'task';

const stripInternalExportMarkers = (value: string) =>
  INTERNAL_EXPORT_MARKERS.reduce(
    (text, pattern) => text.replace(pattern, ''),
    value,
  ).trim();

const getRoleTitle = (role: string) => {
  switch (role) {
    case 'user':
      return '用户';
    case 'assistant':
      return '助手';
    default:
      return '';
  }
};

const getExportableMessages = (messages: TaskThreadMessage[]) =>
  messages
    .map(message => ({
      content: stripInternalExportMarkers(message.content),
      role: getRoleTitle(message.role),
    }))
    .filter(message => message.role && message.content);

const getFallbackMessages = (task: ChatTask) => {
  const userText = stripInternalExportMarkers(
    getTaskInputText(task.input) || task.title,
  );
  const assistantText = stripInternalExportMarkers(
    getTaskResultText(task.result),
  );

  return [
    userText ? { role: '用户', content: userText } : undefined,
    assistantText ? { role: '助手', content: assistantText } : undefined,
  ].filter((message): message is { role: string; content: string } =>
    Boolean(message),
  );
};

const appendMetadata = ({
  lines,
  task,
  tokenUsage,
}: {
  lines: string[];
  task: ChatTask;
  tokenUsage?: TaskDetailTokenUsage;
}) => {
  lines.push(
    `*导出于 ${new Date().toLocaleString()} · 创建 ${formatTaskExportTime(
      task.created_at,
    )}*`,
    '',
    `状态：${getTaskStatusText(task.status)}`,
  );

  if (tokenUsage?.totalTokens) {
    lines.push(
      `Tokens：${formatTokenCount(tokenUsage.totalTokens)} · Input ${formatTokenCount(
        tokenUsage.inputTokens,
      )} · Output ${formatTokenCount(tokenUsage.outputTokens)}`,
    );
  }

  lines.push('', '---', '');
};

export const buildTaskMarkdownExport = ({
  messages,
  task,
  tokenUsage,
}: {
  messages?: TaskThreadMessage[];
  task: ChatTask;
  tokenUsage?: TaskDetailTokenUsage;
}) => {
  const lines = [`# ${task.title}`, ''];
  const visibleMessages = messages?.length
    ? getExportableMessages(messages)
    : getFallbackMessages(task);

  appendMetadata({ lines, task, tokenUsage });

  for (const message of visibleMessages) {
    lines.push(`## ${message.role}`, '', message.content, '', '---', '');
  }

  return `${lines.join('\n').trimEnd()}\n`;
};

const downloadTaskMarkdown = ({
  content,
  fileName,
}: {
  content: string;
  fileName: string;
}) => {
  const blob = new Blob([content], {
    type: 'text/markdown;charset=utf-8',
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

export const TaskExportAction = ({
  messages,
  task,
  tokenUsage,
}: {
  messages?: TaskThreadMessage[];
  task: ChatTask;
  tokenUsage?: TaskDetailTokenUsage;
}) => {
  const handleExport = () => {
    downloadTaskMarkdown({
      content: buildTaskMarkdownExport({ messages, task, tokenUsage }),
      fileName: `${sanitizeExportFilename(task.title)}.md`,
    });
  };

  return (
    <button
      type="button"
      aria-label="导出任务为 Markdown"
      className="coze-prototype-task-action"
      onClick={handleExport}
    >
      <IconCozDownload className="text-[14px]" />
      导出
    </button>
  );
};
