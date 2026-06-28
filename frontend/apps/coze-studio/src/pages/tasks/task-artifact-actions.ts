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

import type { workbenchTask } from '@coze-studio/api-schema';

import {
  artifactInlinePreviewKind,
  artifactFileName,
  artifactPreviewFamily,
  buildArtifactInlinePreview,
} from './task-artifacts-helpers';
import type { RemovedArtifactNotice } from './task-artifact-undo-notice';
import type { ArtifactActionMode } from './task-artifact-list-item';
import type { ArtifactInlinePreviewState } from './task-artifact-inline-preview';
import {
  deleteTaskThreadArtifact,
  fetchTaskThreadArtifactContent,
  getTaskThreadArtifactSignedURL,
  reviewTaskThreadArtifactScan,
  restoreTaskThreadArtifact,
  type ArtifactScanReviewDecision,
} from './service';

type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;

export interface TaskArtifactActions {
  activeAction: string;
  clearInlinePreview: () => void;
  error: string;
  handleArtifactAction: (
    artifact: TaskThreadArtifact,
    mode: ArtifactActionMode,
  ) => Promise<void>;
  handleDeleteArtifact: (artifact: TaskThreadArtifact) => Promise<void>;
  handleRestoreArtifact: () => Promise<void>;
  handleReviewArtifact: (
    artifact: TaskThreadArtifact,
    decision: ArtifactScanReviewDecision,
  ) => Promise<void>;
  inlinePreview: ArtifactInlinePreviewState | null;
  removedArtifact: RemovedArtifactNotice | null;
}

const downloadSignedURL = (url: string, fileName: string) => {
  const link = document.createElement('a');
  link.href = url;
  link.rel = 'noopener noreferrer';
  link.target = '_blank';
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
};

const readTextArtifactPreview = async ({
  artifact,
  threadId,
}: {
  artifact: TaskThreadArtifact;
  threadId: string;
}): Promise<ArtifactInlinePreviewState> => {
  const response = await fetchTaskThreadArtifactContent({
    artifact_id: artifact.artifact_id,
    mode: 'preview',
    thread_id: threadId,
  });
  const contentType = response.contentType || artifact.content_type;
  const content = await response.blob.text();
  return {
    artifactId: artifact.artifact_id,
    contentType,
    name: artifactFileName(artifact),
    previewRenderer: 'content',
    preview: buildArtifactInlinePreview({ content, contentType }),
  };
};

const runTaskArtifactAction = async ({
  artifact,
  mode,
  setInlinePreview,
  threadId,
}: {
  artifact: TaskThreadArtifact;
  mode: ArtifactActionMode;
  setInlinePreview: (preview: ArtifactInlinePreviewState | null) => void;
  threadId: string;
}) => {
  const previewFamily =
    mode === 'preview' ? artifactPreviewFamily(artifact) : null;

  if (previewFamily === 'text' && artifactInlinePreviewKind(artifact)) {
    setInlinePreview(await readTextArtifactPreview({ artifact, threadId }));
    return;
  }

  const response = await getTaskThreadArtifactSignedURL({
    artifact_id: artifact.artifact_id,
    mode,
    thread_id: threadId,
    ttl_seconds: 300,
  });
  const signedURL = response.data?.url?.trim();
  if (!signedURL) {
    throw new Error('生成任务产物签名链接失败');
  }

  if (previewFamily === 'image') {
    setInlinePreview({
      artifactId: artifact.artifact_id,
      contentType: response.data?.content_type || artifact.content_type,
      name: artifactFileName(artifact),
      previewRenderer: 'image',
      url: signedURL,
    });
    return;
  }

  if (mode === 'preview') {
    window.open(signedURL, '_blank', 'noopener,noreferrer');
    return;
  }

  downloadSignedURL(signedURL, artifactFileName(artifact));
};

interface TaskArtifactActionContext {
  activeAction: string;
  onArtifactsChanged?: () => void | Promise<void>;
  removedArtifact: RemovedArtifactNotice | null;
  setActiveAction: (action: string) => void;
  setError: (error: string) => void;
  setInlinePreview: (
    updater:
      | ArtifactInlinePreviewState
      | null
      | ((
          previous: ArtifactInlinePreviewState | null,
        ) => ArtifactInlinePreviewState | null),
  ) => void;
  setRemovedArtifact: (notice: RemovedArtifactNotice | null) => void;
  threadId?: string;
}

const createArtifactActionHandler =
  ({
    activeAction,
    setActiveAction,
    setError,
    setInlinePreview,
    threadId,
  }: TaskArtifactActionContext) =>
  async (artifact: TaskThreadArtifact, mode: ArtifactActionMode) => {
    if (activeAction || !threadId) {
      return;
    }
    const actionKey = `${mode}:${artifact.artifact_id}`;
    setActiveAction(actionKey);
    setError('');
    if (mode === 'preview') {
      setInlinePreview(null);
    }
    try {
      await runTaskArtifactAction({
        artifact,
        mode,
        setInlinePreview,
        threadId,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : '读取任务产物失败');
    } finally {
      setActiveAction('');
    }
  };

const createDeleteArtifactHandler =
  ({
    activeAction,
    onArtifactsChanged,
    setActiveAction,
    setError,
    setInlinePreview,
    setRemovedArtifact,
    threadId,
  }: TaskArtifactActionContext) =>
  async (artifact: TaskThreadArtifact) => {
    if (activeAction || !threadId) {
      return;
    }
    const name = artifactFileName(artifact);
    setActiveAction(`delete:${artifact.artifact_id}`);
    setError('');
    try {
      await deleteTaskThreadArtifact({
        artifact_id: artifact.artifact_id,
        thread_id: threadId,
      });
      setInlinePreview(previous =>
        previous?.artifactId === artifact.artifact_id ? null : previous,
      );
      setRemovedArtifact({ artifactId: artifact.artifact_id, name });
      await onArtifactsChanged?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : '删除任务产物失败');
    } finally {
      setActiveAction('');
    }
  };

const createRestoreArtifactHandler =
  ({
    activeAction,
    onArtifactsChanged,
    removedArtifact,
    setActiveAction,
    setError,
    setRemovedArtifact,
    threadId,
  }: TaskArtifactActionContext) =>
  async () => {
    if (activeAction || !threadId || !removedArtifact) {
      return;
    }
    const actionKey = `restore:${removedArtifact.artifactId}`;
    setActiveAction(actionKey);
    setError('');
    try {
      await restoreTaskThreadArtifact({
        artifact_id: removedArtifact.artifactId,
        thread_id: threadId,
      });
      setRemovedArtifact(null);
      await onArtifactsChanged?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : '恢复任务产物失败');
    } finally {
      setActiveAction('');
    }
  };

const createReviewArtifactHandler =
  ({
    activeAction,
    onArtifactsChanged,
    setActiveAction,
    setError,
    threadId,
  }: TaskArtifactActionContext) =>
  async (
    artifact: TaskThreadArtifact,
    decision: ArtifactScanReviewDecision,
  ) => {
    if (activeAction || !threadId) {
      return;
    }
    const actionKey = `review:${decision}:${artifact.artifact_id}`;
    setActiveAction(actionKey);
    setError('');
    try {
      await reviewTaskThreadArtifactScan({
        artifact_id: artifact.artifact_id,
        decision,
        thread_id: threadId,
      });
      await onArtifactsChanged?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : '审核产物扫描状态失败');
    } finally {
      setActiveAction('');
    }
  };

export const useTaskArtifactActions = ({
  onArtifactsChanged,
  threadId,
}: {
  onArtifactsChanged?: () => void | Promise<void>;
  threadId?: string;
}): TaskArtifactActions => {
  const [activeAction, setActiveAction] = useState('');
  const [error, setError] = useState('');
  const [inlinePreview, setInlinePreview] =
    useState<ArtifactInlinePreviewState | null>(null);
  const [removedArtifact, setRemovedArtifact] =
    useState<RemovedArtifactNotice | null>(null);
  const context = {
    activeAction,
    onArtifactsChanged,
    removedArtifact,
    setActiveAction,
    setError,
    setInlinePreview,
    setRemovedArtifact,
    threadId,
  };

  return {
    activeAction,
    clearInlinePreview: () => setInlinePreview(null),
    error,
    handleArtifactAction: createArtifactActionHandler(context),
    handleDeleteArtifact: createDeleteArtifactHandler(context),
    handleRestoreArtifact: createRestoreArtifactHandler(context),
    handleReviewArtifact: createReviewArtifactHandler(context),
    inlinePreview,
    removedArtifact,
  };
};
