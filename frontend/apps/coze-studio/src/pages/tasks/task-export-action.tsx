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
import {
  IconCozCode,
  IconCozDocument,
  IconCozDownload,
} from '@coze-arch/coze-design/icons';
import { Popover } from '@coze-arch/coze-design';

import { getTaskInputText, getTaskResultText } from './helpers';

type ChatTask = workbenchTask.ChatTask;
type TaskThreadMessage = workbenchTask.TaskThreadMessage;
type TaskExportFormat = 'markdown' | 'json';
type TaskExportMessageType = 'human' | 'ai';

interface ExportableTaskMessage {
  content: string;
  id?: string;
  roleTitle: string;
  type: TaskExportMessageType;
}

const INTERNAL_EXPORT_MARKERS = [
  /<think>[\s\S]*?<\/think>/gi,
  /<system-reminder>[\s\S]*?<\/system-reminder>/gi,
  /<uploaded_files>[\s\S]*?<\/uploaded_files>/gi,
];
const JSON_EXPORT_INDENT = 2;

const formatTaskExportTime = (value?: number) =>
  value && value > 0 ? new Date(value).toLocaleString() : 'Unknown';

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
      return '🧑 User';
    case 'assistant':
      return '🤖 Assistant';
    default:
      return '';
  }
};

const getRoleType = (role: string): TaskExportMessageType | undefined => {
  switch (role) {
    case 'user':
      return 'human';
    case 'assistant':
      return 'ai';
    default:
      return undefined;
  }
};

const getExportableMessages = (messages: TaskThreadMessage[]) =>
  messages
    .map(message => {
      const type = getRoleType(message.role);

      return {
        content: stripInternalExportMarkers(message.content),
        id: message.message_id || undefined,
        roleTitle: getRoleTitle(message.role),
        type,
      };
    })
    .filter((message): message is ExportableTaskMessage =>
      Boolean(message.type && message.roleTitle && message.content),
    );

const getFallbackMessages = (task: ChatTask) => {
  const userText = stripInternalExportMarkers(
    getTaskInputText(task.input) || task.title,
  );
  const assistantText = stripInternalExportMarkers(
    getTaskResultText(task.result),
  );

  return [
    userText
      ? {
          content: userText,
          roleTitle: getRoleTitle('user'),
          type: 'human' as const,
        }
      : undefined,
    assistantText
      ? {
          content: assistantText,
          roleTitle: getRoleTitle('assistant'),
          type: 'ai' as const,
        }
      : undefined,
  ].filter((message): message is ExportableTaskMessage => Boolean(message));
};

const appendMetadata = ({
  lines,
  task,
}: {
  lines: string[];
  task: ChatTask;
}) => {
  lines.push(
    `*Exported on ${new Date().toLocaleString()} · Created ${formatTaskExportTime(
      task.created_at,
    )}*`,
  );

  lines.push('', '---', '');
};

const getVisibleExportMessages = ({
  messages,
  task,
}: {
  messages?: TaskThreadMessage[];
  task: ChatTask;
}) =>
  messages?.length
    ? getExportableMessages(messages)
    : getFallbackMessages(task);

export const buildTaskMarkdownExport = ({
  messages,
  task,
}: {
  messages?: TaskThreadMessage[];
  task: ChatTask;
}) => {
  const lines = [`# ${task.title}`, ''];
  const visibleMessages = getVisibleExportMessages({ messages, task });

  appendMetadata({ lines, task });

  for (const message of visibleMessages) {
    lines.push(`## ${message.roleTitle}`, '', message.content, '', '---', '');
  }

  return `${lines.join('\n').trimEnd()}\n`;
};

const resolveThreadID = ({
  messages,
  task,
  threadId,
}: {
  messages?: TaskThreadMessage[];
  task: ChatTask;
  threadId?: string;
}) =>
  threadId ||
  messages?.find(message => message.thread_id)?.thread_id ||
  task.conversation_id ||
  task.id;

export const buildTaskJSONExport = ({
  messages,
  task,
  threadId,
}: {
  messages?: TaskThreadMessage[];
  task: ChatTask;
  threadId?: string;
}) => {
  const visibleMessages = getVisibleExportMessages({ messages, task });

  return `${JSON.stringify(
    {
      title: task.title,
      thread_id: resolveThreadID({ messages, task, threadId }),
      exported_at: new Date().toISOString(),
      messages: visibleMessages.map(message => ({
        type: message.type,
        id: message.id,
        content: message.content,
      })),
    },
    null,
    JSON_EXPORT_INDENT,
  )}\n`;
};

const downloadTaskFile = ({
  content,
  fileName,
  mimeType,
}: {
  content: string;
  fileName: string;
  mimeType: string;
}) => {
  const blob = new Blob([content], {
    type: mimeType,
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
  threadId,
}: {
  messages?: TaskThreadMessage[];
  task: ChatTask;
  threadId?: string;
}) => {
  const handleExport = (format: TaskExportFormat) => {
    if (format === 'markdown') {
      downloadTaskFile({
        content: buildTaskMarkdownExport({ messages, task }),
        fileName: `${sanitizeExportFilename(task.title)}.md`,
        mimeType: 'text/markdown;charset=utf-8',
      });
      return;
    }

    downloadTaskFile({
      content: buildTaskJSONExport({ messages, task, threadId }),
      fileName: `${sanitizeExportFilename(task.title)}.json`,
      mimeType: 'application/json;charset=utf-8',
    });
  };

  const menu = (
    <div
      className="coze-prototype-export-menu"
      role="menu"
      aria-label="任务导出格式"
    >
      <button
        type="button"
        role="menuitem"
        aria-label="导出任务为 Markdown"
        className="coze-prototype-export-menu-item"
        onClick={() => handleExport('markdown')}
      >
        <IconCozDocument className="text-[14px]" />
        <span>Markdown</span>
      </button>
      <button
        type="button"
        role="menuitem"
        aria-label="导出任务为 JSON"
        className="coze-prototype-export-menu-item"
        onClick={() => handleExport('json')}
      >
        <IconCozCode className="text-[14px]" />
        <span>JSON</span>
      </button>
    </div>
  );

  return (
    <Popover content={menu} position="bottomRight" showArrow trigger="click">
      <button
        type="button"
        aria-label="打开任务导出菜单"
        className="coze-prototype-task-action"
      >
        <IconCozDownload className="text-[14px]" />
        <span>导出</span>
      </button>
    </Popover>
  );
};
