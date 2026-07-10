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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function -- Cohesive orchestrator. */
/* eslint-disable max-params -- Cohesive orchestrator. */

import { useCallback, useEffect, useRef, useState } from 'react';

import { projectAppDevEvent } from '../utils/sse-utils';
import type {
  AppDevChatAttachment,
  AppDevChatMessage,
  AppDevDataSource,
} from '../types';
import {
  buildAppDevEventsUrl,
  cancelAppDevChat,
  getAppDevChatStatus,
  listAppDevChatHistory,
  normalizeAppDevError,
  sendAppDevChatMessage,
  uploadAppDevFiles,
} from '../service';

const maxAttachmentCount = 8;
const maxAttachmentFileBytes = 10 * 1024 * 1024;
const chatStatusReconcileIntervalMs = 3000;

const safeUploadFileName = (name: string) => {
  const normalized = name.trim().replace(/[^a-zA-Z0-9._-]/gu, '-');
  return normalized || 'attachment';
};

const formatUploadPath = (
  file: File,
  index: number,
  type?: AppDevChatAttachment['type'],
) =>
  `src/assets/uploads/${type === 'prototype_image' ? 'prototype-' : ''}${Date.now()}-${index}-${safeUploadFileName(file.name)}`;

const attachmentTypeFromFile = (
  file: File,
  type?: AppDevChatAttachment['type'],
): AppDevChatAttachment['type'] =>
  type === 'prototype_image'
    ? 'prototype_image'
    : file.type.startsWith('image/')
      ? 'image'
      : 'file';

const defaultAttachmentMessage = (attachments: AppDevChatAttachment[]) => {
  const hasPrototypeImage = attachments.some(
    attachment => attachment.type === 'prototype_image',
  );
  const hasNormalAttachment = attachments.some(
    attachment => attachment.type !== 'prototype_image',
  );

  if (hasPrototypeImage && hasNormalAttachment) {
    return '请参考附件和原型图继续开发页面';
  }
  if (hasPrototypeImage) {
    return '请参考原型图继续开发页面';
  }

  return attachments.length ? '请参考附件继续开发页面' : '';
};

export const useAppDevChat = (
  spaceId?: string,
  projectId?: string,
  modelId?: string,
  dataSources?: AppDevDataSource[],
  onTaskSettled?: () => void,
  onFilesChanged?: () => void,
) => {
  const [messages, setMessages] = useState<AppDevChatMessage[]>([]);
  const [input, setInput] = useState('');
  const [attachments, setAttachments] = useState<AppDevChatAttachment[]>([]);
  const [running, setRunning] = useState(false);
  const [loadingHistory, setLoadingHistory] = useState(false);
  const [uploadingAttachments, setUploadingAttachments] = useState(false);
  const [error, setError] = useState('');
  const eventSourceRef = useRef<EventSource | undefined>();
  const settledRef = useRef(true);
  const onTaskSettledRef = useRef(onTaskSettled);
  const onFilesChangedRef = useRef(onFilesChanged);
  onTaskSettledRef.current = onTaskSettled;
  onFilesChangedRef.current = onFilesChanged;

  const handleTaskSettled = useCallback((refreshFiles = false) => {
    if (settledRef.current) {
      return;
    }
    settledRef.current = true;
    setRunning(false);
    onTaskSettledRef.current?.();
    if (refreshFiles) {
      onFilesChangedRef.current?.();
    }
  }, []);

  const closeEvents = useCallback(() => {
    eventSourceRef.current?.close();
    eventSourceRef.current = undefined;
  }, []);

  const connectEvents = useCallback(() => {
    if (!spaceId || !projectId || eventSourceRef.current) {
      return;
    }

    const source = new EventSource(
      buildAppDevEventsUrl({ spaceId, projectId }),
    );
    eventSourceRef.current = source;

    [
      'prompt_start',
      'agent_thought_chunk',
      'agent_message_chunk',
      'tool_call',
      'tool_call_update',
      'prompt_end',
      'error',
    ].forEach(eventName => {
      source.addEventListener(eventName, event => {
        let data: Record<string, unknown> = {};
        try {
          data = JSON.parse((event as MessageEvent).data || '{}') as Record<
            string,
            unknown
          >;
        } catch {
          data = {};
        }
        setError('');
        setMessages(current =>
          projectAppDevEvent(current, {
            event: eventName,
            data,
          }),
        );
        if (eventName === 'prompt_end' || eventName === 'error') {
          handleTaskSettled();
        }
      });
    });
    source.addEventListener('heartbeat', () => {
      setError('');
    });

    source.onerror = () => {
      setError('AI 事件流连接异常，正在等待恢复');
    };
  }, [handleTaskSettled, projectId, spaceId]);

  const refreshHistory = useCallback(async () => {
    if (!spaceId || !projectId) {
      setMessages([]);
      return;
    }

    setLoadingHistory(true);
    setError('');

    try {
      const result = await listAppDevChatHistory({ spaceId, projectId });
      setMessages(result.items);
      const status = await getAppDevChatStatus({ spaceId, projectId });
      settledRef.current = !status.running;
      setRunning(status.running);
    } catch (requestError) {
      setError(normalizeAppDevError(requestError));
    } finally {
      setLoadingHistory(false);
    }
  }, [projectId, spaceId]);

  const reconcileRunningStatus = useCallback(async () => {
    if (!spaceId || !projectId) {
      return;
    }

    try {
      const status = await getAppDevChatStatus({ spaceId, projectId });
      if (status.running) {
        return;
      }
      const result = await listAppDevChatHistory({ spaceId, projectId });
      setMessages(result.items);
      setError('');
      handleTaskSettled(true);
    } catch (requestError) {
      setError(normalizeAppDevError(requestError));
    }
  }, [handleTaskSettled, projectId, spaceId]);

  const addAttachmentFiles = useCallback(
    async (
      files: File[],
      options?: { type?: AppDevChatAttachment['type'] },
    ) => {
      if (!spaceId || !projectId) {
        setError('项目地址不完整，请返回项目列表重新进入');
        return;
      }

      const uploadFiles = files.filter(file => file.size > 0);
      if (!uploadFiles.length) {
        return;
      }
      if (
        options?.type === 'prototype_image' &&
        uploadFiles.some(file => !file.type.startsWith('image/'))
      ) {
        setError('原型图仅支持图片文件');
        return;
      }
      if (attachments.length + uploadFiles.length > maxAttachmentCount) {
        setError(`最多只能添加 ${maxAttachmentCount} 个附件`);
        return;
      }
      const oversized = uploadFiles.find(
        file => file.size > maxAttachmentFileBytes,
      );
      if (oversized) {
        setError(`「${oversized.name}」不能超过 10MB`);
        return;
      }

      const filePaths = uploadFiles.map((file, index) =>
        formatUploadPath(file, index, options?.type),
      );
      setUploadingAttachments(true);
      setError('');
      try {
        await uploadAppDevFiles({
          spaceId,
          projectId,
          files: uploadFiles,
          filePaths,
        });
        setAttachments(current => [
          ...current,
          ...uploadFiles.map((file, index) => ({
            id: `attachment_${Date.now()}_${index}`,
            name: file.name || filePaths[index].split('/').pop() || '附件',
            path: filePaths[index],
            mimeType: file.type,
            size: file.size,
            type: attachmentTypeFromFile(file, options?.type),
          })),
        ]);
        onFilesChanged?.();
      } catch (requestError) {
        setError(normalizeAppDevError(requestError));
      } finally {
        setUploadingAttachments(false);
      }
    },
    [attachments.length, onFilesChanged, projectId, spaceId],
  );

  const removeAttachment = useCallback((id: string) => {
    setAttachments(current =>
      current.filter(attachment => attachment.id !== id),
    );
  }, []);

  const sendMessage = useCallback(
    async (messageOverride?: string) => {
      const messageSource =
        typeof messageOverride === 'string' ? messageOverride : input;
      const message =
        messageSource.trim() || defaultAttachmentMessage(attachments);
      if (!spaceId || !projectId || !message) {
        return false;
      }
      if (!modelId) {
        setError('请选择编码模型后再发送');
        return false;
      }

      setInput('');
      const sendingAttachments = attachments;
      setAttachments([]);
      setError('');
      settledRef.current = false;
      setRunning(true);
      const localMessageId = `local_user_${Date.now()}`;
      setMessages(current => [
        ...current,
        {
          id: localMessageId,
          type: 'user',
          role: 'user',
          content: message,
          attachments: sendingAttachments,
          createdAt: new Date().toISOString(),
        },
      ]);
      connectEvents();

      try {
        await sendAppDevChatMessage({
          spaceId,
          projectId,
          message,
          modelId,
          dataSources,
          attachments: sendingAttachments,
        });
        return true;
      } catch (requestError) {
        setRunning(false);
        setInput(message);
        setAttachments(sendingAttachments);
        setMessages(current =>
          current.filter(messageItem => messageItem.id !== localMessageId),
        );
        setError(normalizeAppDevError(requestError));
        return false;
      }
    },
    [
      attachments,
      connectEvents,
      dataSources,
      input,
      modelId,
      projectId,
      spaceId,
    ],
  );

  const cancel = useCallback(async () => {
    if (!spaceId || !projectId) {
      return;
    }

    try {
      await cancelAppDevChat({ spaceId, projectId });
      setRunning(false);
    } catch (requestError) {
      setError(normalizeAppDevError(requestError));
    }
  }, [projectId, spaceId]);

  useEffect(() => {
    void refreshHistory();
    connectEvents();
    return closeEvents;
  }, [closeEvents, connectEvents, refreshHistory]);

  useEffect(() => {
    if (!running || !spaceId || !projectId) {
      return;
    }

    const timer = window.setInterval(() => {
      void reconcileRunningStatus();
    }, chatStatusReconcileIntervalMs);
    return () => window.clearInterval(timer);
  }, [projectId, reconcileRunningStatus, running, spaceId]);

  return {
    messages,
    input,
    setInput,
    attachments,
    uploadingAttachments,
    running,
    loadingHistory,
    error,
    addAttachmentFiles,
    removeAttachment,
    sendMessage,
    cancel,
    refreshHistory,
  };
};
