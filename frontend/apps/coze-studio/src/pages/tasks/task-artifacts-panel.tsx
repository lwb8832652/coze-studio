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
import { RemovedArtifactUndoNotice } from './task-artifact-undo-notice';
import { TaskArtifactScanJobsSection } from './task-artifact-scan-jobs-section';
import { TaskArtifactListItem } from './task-artifact-list-item';
import { TaskArtifactInlinePreview } from './task-artifact-inline-preview';
import { useTaskArtifactActions } from './task-artifact-actions';

type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;
type ArtifactPanelMode = 'active' | 'deleted';

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
