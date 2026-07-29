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

/* eslint-disable @coze-arch/max-line-per-function -- Artifact actions share cancellation, progress and safe-download orchestration. */

import { useCallback, useEffect, useRef, useState } from 'react';

import type { WorkbenchArtifact } from '../workbench/thread-client';

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
  installSkillFromArtifact,
  isTaskThreadArtifactSafeError,
  reviewTaskThreadArtifactScan,
  restoreTaskThreadArtifact,
  type ArtifactScanReviewDecision,
} from './service';

type TaskThreadArtifact = WorkbenchArtifact;

export interface TaskArtifactActions {
  activeAction: string;
  clearInlinePreview: () => void;
  clearRemovedArtifact: (artifactId?: string) => void;
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

const artifactActionErrorMessage = (mode: ArtifactActionMode, err?: unknown) =>
  isTaskThreadArtifactSafeError(err)
    ? err.message
    : mode === 'install_skill' && err instanceof Error && err.message.trim()
      ? err.message
      : mode === 'download'
        ? '下载任务产物失败，请稍后重试'
        : mode === 'install_skill'
          ? '安装技能失败，请稍后重试'
          : '读取任务产物失败，请稍后重试';

const readTextArtifactPreview = async ({
  artifact,
  spaceId,
  threadId,
}: {
  artifact: TaskThreadArtifact;
  spaceId?: string;
  threadId: string;
}): Promise<ArtifactInlinePreviewState> => {
  const response = await fetchTaskThreadArtifactContent({
    artifact_id: artifact.artifact_id,
    mode: 'preview',
    space_id: spaceId,
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
  isCurrent,
  mode,
  spaceId,
  setInlinePreview,
  threadId,
}: {
  artifact: TaskThreadArtifact;
  isCurrent: () => boolean;
  mode: ArtifactActionMode;
  spaceId?: string;
  setInlinePreview: (preview: ArtifactInlinePreviewState | null) => void;
  threadId: string;
}) => {
  if (mode === 'install_skill') {
    if (!spaceId) {
      throw new Error('缺少空间信息，无法安装技能');
    }
    await installSkillFromArtifact({
      artifact_id: artifact.artifact_id,
      space_id: spaceId,
      thread_id: threadId,
    });
    return;
  }

  const previewFamily =
    mode === 'preview' ? artifactPreviewFamily(artifact) : null;

  if (previewFamily === 'text' && artifactInlinePreviewKind(artifact)) {
    const preview = await readTextArtifactPreview({
      artifact,
      spaceId,
      threadId,
    });
    if (isCurrent()) {
      setInlinePreview(preview);
    }
    return;
  }

  const response = await getTaskThreadArtifactSignedURL({
    artifact_id: artifact.artifact_id,
    mode,
    space_id: spaceId,
    thread_id: threadId,
    ttl_seconds: 300,
  });
  const signedURL = response.data?.url?.trim();
  if (!isCurrent()) {
    return;
  }
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

  if (previewFamily === 'pdf') {
    setInlinePreview({
      artifactId: artifact.artifact_id,
      contentType: response.data?.content_type || artifact.content_type,
      name: artifactFileName(artifact),
      previewRenderer: 'pdf',
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

interface TaskArtifactActionState {
  activeAction: string;
  error: string;
  inlinePreview: ArtifactInlinePreviewState | null;
  removedArtifact: RemovedArtifactNotice | null;
  scope: string;
}

const createEmptyArtifactActionState = (
  scope: string,
): TaskArtifactActionState => ({
  activeAction: '',
  error: '',
  inlinePreview: null,
  removedArtifact: null,
  scope,
});

export const useTaskArtifactActions = ({
  onArtifactsChanged,
  spaceId,
  threadId,
}: {
  onArtifactsChanged?: () => void | Promise<void>;
  spaceId?: string;
  threadId?: string;
}): TaskArtifactActions => {
  const scope = `${spaceId ?? ''}:${threadId ?? ''}`;
  const scopeRef = useRef(scope);
  const generationRef = useRef(0);
  const mountedRef = useRef(true);
  const activeOperationRef = useRef('');
  const [state, setState] = useState<TaskArtifactActionState>(() =>
    createEmptyArtifactActionState(scope),
  );
  if (scopeRef.current !== scope) {
    scopeRef.current = scope;
    generationRef.current += 1;
    activeOperationRef.current = '';
  }
  const visibleState =
    state.scope === scope ? state : createEmptyArtifactActionState(scope);
  const isCurrentScope = useCallback(
    (submittedScope: string, generation: number) =>
      mountedRef.current &&
      scopeRef.current === submittedScope &&
      generationRef.current === generation,
    [],
  );
  const updateCurrentState = useCallback(
    (
      submittedScope: string,
      generation: number,
      updater: (current: TaskArtifactActionState) => TaskArtifactActionState,
    ) => {
      if (!isCurrentScope(submittedScope, generation)) {
        return;
      }
      setState(current =>
        updater(
          current.scope === submittedScope
            ? current
            : createEmptyArtifactActionState(submittedScope),
        ),
      );
    },
    [isCurrentScope],
  );
  const beginOperation = useCallback(
    (actionKey: string, clearPreview = false) => {
      if (!threadId || activeOperationRef.current) {
        return;
      }
      const submittedScope = scope;
      const generation = generationRef.current;
      activeOperationRef.current = actionKey;
      updateCurrentState(submittedScope, generation, current => ({
        ...current,
        activeAction: actionKey,
        error: '',
        inlinePreview: clearPreview ? null : current.inlinePreview,
      }));

      return { generation, submittedScope, submittedThreadId: threadId };
    },
    [scope, threadId, updateCurrentState],
  );
  const finishOperation = useCallback(
    (submittedScope: string, generation: number) => {
      if (!isCurrentScope(submittedScope, generation)) {
        return;
      }
      activeOperationRef.current = '';
      updateCurrentState(submittedScope, generation, current => ({
        ...current,
        activeAction: '',
      }));
    },
    [isCurrentScope, updateCurrentState],
  );

  useEffect(() => {
    const generation = generationRef.current;
    if (scopeRef.current === scope) {
      setState(createEmptyArtifactActionState(scope));
      activeOperationRef.current = '';
    }

    return () => {
      if (generationRef.current === generation) {
        generationRef.current += 1;
      }
    };
  }, [scope]);

  useEffect(
    () => () => {
      mountedRef.current = false;
      generationRef.current += 1;
      activeOperationRef.current = '';
    },
    [],
  );

  const handleArtifactAction = useCallback(
    async (artifact: TaskThreadArtifact, mode: ArtifactActionMode) => {
      const operation = beginOperation(
        `${mode}:${artifact.artifact_id}`,
        mode === 'preview',
      );
      if (!operation) {
        return;
      }
      const { generation, submittedScope, submittedThreadId } = operation;
      const isCurrent = () => isCurrentScope(submittedScope, generation);

      try {
        await runTaskArtifactAction({
          artifact,
          isCurrent,
          mode,
          spaceId,
          setInlinePreview: preview =>
            updateCurrentState(submittedScope, generation, current => ({
              ...current,
              inlinePreview: preview,
            })),
          threadId: submittedThreadId,
        });
      } catch (err) {
        updateCurrentState(submittedScope, generation, current => ({
          ...current,
          error: artifactActionErrorMessage(mode, err),
        }));
      } finally {
        finishOperation(submittedScope, generation);
      }
    },
    [
      beginOperation,
      finishOperation,
      isCurrentScope,
      spaceId,
      updateCurrentState,
    ],
  );

  const handleDeleteArtifact = useCallback(
    async (artifact: TaskThreadArtifact) => {
      const operation = beginOperation(`delete:${artifact.artifact_id}`);
      if (!operation) {
        return;
      }
      const { generation, submittedScope, submittedThreadId } = operation;
      const name = artifactFileName(artifact);

      try {
        await deleteTaskThreadArtifact({
          artifact_id: artifact.artifact_id,
          space_id: spaceId,
          thread_id: submittedThreadId,
        });
        if (!isCurrentScope(submittedScope, generation)) {
          return;
        }
        updateCurrentState(submittedScope, generation, current => ({
          ...current,
          inlinePreview:
            current.inlinePreview?.artifactId === artifact.artifact_id
              ? null
              : current.inlinePreview,
          removedArtifact: { artifactId: artifact.artifact_id, name },
        }));
        await onArtifactsChanged?.();
      } catch (err) {
        updateCurrentState(submittedScope, generation, current => ({
          ...current,
          error: err instanceof Error ? err.message : '删除任务产物失败',
        }));
      } finally {
        finishOperation(submittedScope, generation);
      }
    },
    [
      beginOperation,
      finishOperation,
      isCurrentScope,
      onArtifactsChanged,
      spaceId,
      updateCurrentState,
    ],
  );

  const handleRestoreArtifact = useCallback(async () => {
    const { removedArtifact } = visibleState;
    if (!removedArtifact) {
      return;
    }
    const operation = beginOperation(`restore:${removedArtifact.artifactId}`);
    if (!operation) {
      return;
    }
    const { generation, submittedScope, submittedThreadId } = operation;

    try {
      await restoreTaskThreadArtifact({
        artifact_id: removedArtifact.artifactId,
        space_id: spaceId,
        thread_id: submittedThreadId,
      });
      if (!isCurrentScope(submittedScope, generation)) {
        return;
      }
      updateCurrentState(submittedScope, generation, current => ({
        ...current,
        removedArtifact: null,
      }));
      await onArtifactsChanged?.();
    } catch (err) {
      updateCurrentState(submittedScope, generation, current => ({
        ...current,
        error: err instanceof Error ? err.message : '恢复任务产物失败',
      }));
    } finally {
      finishOperation(submittedScope, generation);
    }
  }, [
    beginOperation,
    finishOperation,
    isCurrentScope,
    onArtifactsChanged,
    spaceId,
    updateCurrentState,
    visibleState.removedArtifact,
  ]);

  const handleReviewArtifact = useCallback(
    async (
      artifact: TaskThreadArtifact,
      decision: ArtifactScanReviewDecision,
    ) => {
      const operation = beginOperation(
        `review:${decision}:${artifact.artifact_id}`,
      );
      if (!operation) {
        return;
      }
      const { generation, submittedScope, submittedThreadId } = operation;

      try {
        await reviewTaskThreadArtifactScan({
          artifact_id: artifact.artifact_id,
          decision,
          space_id: spaceId,
          thread_id: submittedThreadId,
        });
        if (!isCurrentScope(submittedScope, generation)) {
          return;
        }
        updateCurrentState(submittedScope, generation, current => ({
          ...current,
          inlinePreview:
            current.inlinePreview?.artifactId === artifact.artifact_id
              ? null
              : current.inlinePreview,
        }));
        await onArtifactsChanged?.();
      } catch (err) {
        updateCurrentState(submittedScope, generation, current => ({
          ...current,
          error: err instanceof Error ? err.message : '审核产物扫描状态失败',
        }));
      } finally {
        finishOperation(submittedScope, generation);
      }
    },
    [
      beginOperation,
      finishOperation,
      isCurrentScope,
      onArtifactsChanged,
      spaceId,
      updateCurrentState,
    ],
  );

  return {
    activeAction: visibleState.activeAction,
    clearInlinePreview: () =>
      updateCurrentState(scope, generationRef.current, current => ({
        ...current,
        inlinePreview: null,
      })),
    clearRemovedArtifact: artifactId =>
      updateCurrentState(scope, generationRef.current, current => ({
        ...current,
        removedArtifact:
          !artifactId || current.removedArtifact?.artifactId === artifactId
            ? null
            : current.removedArtifact,
      })),
    error: visibleState.error,
    handleArtifactAction,
    handleDeleteArtifact,
    handleRestoreArtifact,
    handleReviewArtifact,
    inlinePreview: visibleState.inlinePreview,
    removedArtifact: visibleState.removedArtifact,
  };
};
