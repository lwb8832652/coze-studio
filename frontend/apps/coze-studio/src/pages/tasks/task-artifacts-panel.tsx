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

import { useMemo, useState } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';
import { Button, List, SideSheet } from '@coze-arch/coze-design';

import { TaskDeletedArtifactsSection } from './task-deleted-artifacts-section';
import {
  artifactInlinePreviewKind,
  artifactFileName,
  artifactPreviewFamily,
  buildArtifactInlinePreview,
} from './task-artifacts-helpers';
import {
  RemovedArtifactUndoNotice,
  type RemovedArtifactNotice,
} from './task-artifact-undo-notice';
import { TaskArtifactScanJobsSection } from './task-artifact-scan-jobs-section';
import {
  TaskArtifactListItem,
  type ArtifactActionMode,
} from './task-artifact-list-item';
import {
  TaskArtifactInlinePreview,
  type ArtifactInlinePreviewState,
} from './task-artifact-inline-preview';
import {
  deleteTaskThreadArtifact,
  fetchTaskThreadArtifactContent,
  getTaskThreadArtifactSignedURL,
  reviewTaskThreadArtifactScan,
  restoreTaskThreadArtifact,
  type ArtifactScanReviewDecision,
} from './service';

type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;
type ArtifactPanelMode = 'active' | 'deleted';

interface TaskArtifactActions {
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

const runArtifactAction = async ({
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

const useTaskArtifactActions = ({
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

  const handleArtifactAction = async (
    artifact: TaskThreadArtifact,
    mode: ArtifactActionMode,
  ) => {
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
      await runArtifactAction({ artifact, mode, setInlinePreview, threadId });
    } catch (err) {
      setError(err instanceof Error ? err.message : '读取任务产物失败');
    } finally {
      setActiveAction('');
    }
  };

  const handleDeleteArtifact = async (artifact: TaskThreadArtifact) => {
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

  const handleRestoreArtifact = async () => {
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

  const handleReviewArtifact = async (
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

  return {
    activeAction,
    clearInlinePreview: () => setInlinePreview(null),
    error,
    handleArtifactAction,
    handleDeleteArtifact,
    handleRestoreArtifact,
    handleReviewArtifact,
    inlinePreview,
    removedArtifact,
  };
};

export const TaskArtifactsPanel = ({
  artifacts,
  onArtifactsChanged,
  threadId,
}: {
  artifacts: TaskThreadArtifact[];
  onArtifactsChanged?: () => void | Promise<void>;
  threadId?: string;
}) => {
  const [visible, setVisible] = useState(false);
  const [panelMode, setPanelMode] = useState<ArtifactPanelMode>('active');
  const {
    activeAction,
    clearInlinePreview,
    error,
    handleArtifactAction,
    handleDeleteArtifact,
    handleRestoreArtifact,
    handleReviewArtifact,
    inlinePreview,
    removedArtifact,
  } = useTaskArtifactActions({ onArtifactsChanged, threadId });
  const sortedArtifacts = useMemo(
    () =>
      [...artifacts].sort(
        (left, right) =>
          right.created_at - left.created_at ||
          right.artifact_id.localeCompare(left.artifact_id),
      ),
    [artifacts],
  );

  if (!threadId) {
    return null;
  }

  return (
    <>
      <button
        type="button"
        className="coze-prototype-task-action"
        data-testid="task-artifacts-open"
        onClick={() => setVisible(true)}
      >
        产物 {artifacts.length}
      </button>
      <SideSheet
        title="任务产物"
        visible={visible}
        onCancel={() => setVisible(false)}
        width={520}
      >
        <div className="coze-prototype-artifacts-panel">
          {error ? <div className="coze-prototype-error">{error}</div> : null}
          {removedArtifact ? (
            <RemovedArtifactUndoNotice
              activeAction={activeAction}
              removedArtifact={removedArtifact}
              onRestore={handleRestoreArtifact}
            />
          ) : null}
          <div className="coze-prototype-artifact-mode-switch">
            <Button
              size="small"
              theme={panelMode === 'active' ? 'solid' : 'borderless'}
              type={panelMode === 'active' ? 'primary' : 'tertiary'}
              onClick={() => setPanelMode('active')}
            >
              当前
            </Button>
            <Button
              size="small"
              theme={panelMode === 'deleted' ? 'solid' : 'borderless'}
              type={panelMode === 'deleted' ? 'primary' : 'tertiary'}
              onClick={() => setPanelMode('deleted')}
            >
              已移除
            </Button>
          </div>
          {panelMode === 'active' ? (
            <>
              <TaskArtifactScanJobsSection
                threadId={threadId}
                visible={visible}
              />
              <List
                dataSource={sortedArtifacts}
                emptyContent={
                  <div className="coze-prototype-artifacts-empty">
                    暂无任务产物
                  </div>
                }
                renderItem={artifact => (
                  <TaskArtifactListItem
                    activeAction={activeAction}
                    artifact={artifact as TaskThreadArtifact}
                    onArtifactAction={handleArtifactAction}
                    onDeleteArtifact={handleDeleteArtifact}
                    onReviewArtifact={handleReviewArtifact}
                  />
                )}
              />
              {inlinePreview ? (
                <TaskArtifactInlinePreview
                  inlinePreview={inlinePreview}
                  onClose={clearInlinePreview}
                />
              ) : null}
            </>
          ) : (
            <TaskDeletedArtifactsSection
              threadId={threadId}
              visible={visible}
              onArtifactsChanged={onArtifactsChanged}
            />
          )}
        </div>
      </SideSheet>
    </>
  );
};
