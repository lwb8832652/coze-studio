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

import { useCallback, useEffect, useMemo, useState } from 'react';

import type { WorkbenchArtifact } from '../workbench/thread-client';
import { IconCozDocument } from '@coze-arch/coze-design/icons';
import { Button, List, Tag } from '@coze-arch/coze-design';

import { artifactFileName, formatArtifactSize } from './task-artifacts-helpers';
import { listTaskThreadArtifacts, restoreTaskThreadArtifact } from './service';

type TaskThreadArtifact = WorkbenchArtifact;

const DELETED_ARTIFACT_PAGE_SIZE = 50;

const formatDeletedAt = (deletedAt?: number) => {
  if (!deletedAt) {
    return '移除时间未知';
  }
  return new Date(deletedAt).toLocaleString();
};

const DeletedArtifactListItem = ({
  activeRestoreID,
  artifact,
  onRestore,
}: {
  activeRestoreID: string;
  artifact: TaskThreadArtifact;
  onRestore: (artifact: TaskThreadArtifact) => void | Promise<void>;
}) => {
  const artifactName = artifactFileName(artifact);
  const loading = activeRestoreID === artifact.artifact_id;
  const actionLocked = Boolean(activeRestoreID) && !loading;

  return (
    <div className="coze-prototype-artifact-item">
      <div className="coze-prototype-artifact-main">
        <span className="coze-prototype-artifact-icon">
          <IconCozDocument className="text-[16px]" />
        </span>
        <div className="coze-prototype-artifact-content">
          <div className="coze-prototype-artifact-title-row">
            <span className="coze-prototype-artifact-title">
              {artifact.title || artifactName}
            </span>
            <Tag>已移除</Tag>
            <Tag>{artifact.preview_mode}</Tag>
          </div>
          <div className="coze-prototype-artifact-meta">
            <span>{artifact.artifact_type || 'artifact'}</span>
            <span>{formatArtifactSize(artifact.size_bytes)}</span>
            <span>{artifact.content_type || 'application/octet-stream'}</span>
            <span>{formatDeletedAt(artifact.deleted_at)}</span>
          </div>
        </div>
      </div>
      <div className="coze-prototype-artifact-actions">
        <Button
          aria-label={`恢复 ${artifactName}`}
          disabled={actionLocked}
          loading={loading}
          size="small"
          theme="borderless"
          onClick={() => void onRestore(artifact)}
        >
          恢复
        </Button>
      </div>
    </div>
  );
};

export const TaskDeletedArtifactsSection = ({
  onArtifactRestored,
  onArtifactsChanged,
  spaceId,
  threadId,
  visible,
}: {
  onArtifactRestored?: (artifact: TaskThreadArtifact) => void | Promise<void>;
  onArtifactsChanged?: () => void | Promise<void>;
  spaceId?: string;
  threadId: string;
  visible: boolean;
}) => {
  const [deletedArtifacts, setDeletedArtifacts] = useState<
    TaskThreadArtifact[]
  >([]);
  const [deletedTotal, setDeletedTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [activeRestoreID, setActiveRestoreID] = useState('');
  const [error, setError] = useState('');
  const sortedDeletedArtifacts = useMemo(
    () =>
      [...deletedArtifacts].sort(
        (left, right) =>
          (right.deleted_at || 0) - (left.deleted_at || 0) ||
          right.artifact_id.localeCompare(left.artifact_id),
      ),
    [deletedArtifacts],
  );

  const loadDeletedArtifacts = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const response = await listTaskThreadArtifacts({
        thread_id: threadId,
        space_id: spaceId,
        deleted_only: true,
        page: 1,
        page_size: DELETED_ARTIFACT_PAGE_SIZE,
      });
      setDeletedArtifacts(response.data?.artifacts ?? []);
      setDeletedTotal(response.data?.total ?? 0);
    } catch (err) {
      setError(err instanceof Error ? err.message : '读取已移除产物失败');
    } finally {
      setLoading(false);
    }
  }, [spaceId, threadId]);

  useEffect(() => {
    if (visible) {
      void loadDeletedArtifacts();
    }
  }, [loadDeletedArtifacts, visible]);

  const handleRestore = async (artifact: TaskThreadArtifact) => {
    if (activeRestoreID) {
      return;
    }
    setActiveRestoreID(artifact.artifact_id);
    setError('');
    try {
      await restoreTaskThreadArtifact({
        artifact_id: artifact.artifact_id,
        space_id: spaceId,
        thread_id: threadId,
      });
      await onArtifactRestored?.(artifact);
      await onArtifactsChanged?.();
      await loadDeletedArtifacts();
    } catch (err) {
      setError(err instanceof Error ? err.message : '恢复任务产物失败');
    } finally {
      setActiveRestoreID('');
    }
  };

  return (
    <section className="coze-prototype-deleted-artifacts-section">
      <div className="coze-prototype-deleted-artifacts-heading">
        已移除 {deletedTotal}
      </div>
      {error ? (
        <div className="coze-prototype-error" role="alert">
          {error}
        </div>
      ) : null}
      <List
        dataSource={sortedDeletedArtifacts}
        emptyContent={
          <div className="coze-prototype-artifacts-empty">暂无已移除产物</div>
        }
        loading={loading}
        renderItem={artifact => (
          <DeletedArtifactListItem
            activeRestoreID={activeRestoreID}
            artifact={artifact as TaskThreadArtifact}
            onRestore={handleRestore}
          />
        )}
      />
    </section>
  );
};
