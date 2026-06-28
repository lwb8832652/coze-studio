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

import { useMemo } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';
import {
  IconCozDocument,
  IconCozDownload,
  IconCozEye,
} from '@coze-arch/coze-design/icons';
import { Button } from '@coze-arch/coze-design';

import {
  artifactFileName,
  canPreviewArtifact,
  formatArtifactSize,
} from './task-artifacts-helpers';
import { TaskArtifactInlinePreview } from './task-artifact-inline-preview';
import { useTaskArtifactActions } from './task-artifact-actions';

type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;

const CONTENT_TYPE_LABELS: Record<string, string> = {
  'application/json': 'JSON',
  'application/pdf': 'PDF',
  'text/csv': 'CSV',
  'text/html': 'HTML',
  'text/markdown': 'Markdown',
};

const EXTENSION_LABELS: Record<string, string> = {
  csv: 'CSV',
  html: 'HTML',
  json: 'JSON',
  md: 'Markdown',
  pdf: 'PDF',
};

const fileExtension = (fileName: string) => {
  const normalized = fileName.toLowerCase();
  const extension = normalized.split('.').at(-1);
  return extension && extension !== normalized ? extension : '';
};

const contentTypeLabel = (artifact: TaskThreadArtifact) => {
  const contentType = artifact.content_type.split(';')[0]?.trim().toLowerCase();
  const extension = fileExtension(artifactFileName(artifact));
  const mappedContentType = contentType
    ? CONTENT_TYPE_LABELS[contentType]
    : undefined;
  const mappedExtension = extension ? EXTENSION_LABELS[extension] : undefined;

  if (mappedContentType || mappedExtension) {
    return mappedContentType || mappedExtension;
  }
  if (contentType?.startsWith('image/')) {
    return 'Image';
  }
  if (contentType?.startsWith('text/')) {
    return 'Text';
  }
  return artifact.artifact_type || 'File';
};

export const TaskArtifactMessageList = ({
  artifacts,
  threadId,
}: {
  artifacts: TaskThreadArtifact[];
  threadId?: string;
}) => {
  const {
    activeAction,
    clearInlinePreview,
    error,
    handleArtifactAction,
    inlinePreview,
  } = useTaskArtifactActions({ threadId });
  const visibleArtifacts = useMemo(
    () =>
      [...artifacts].sort(
        (left, right) =>
          left.created_at - right.created_at ||
          left.artifact_id.localeCompare(right.artifact_id),
      ),
    [artifacts],
  );

  if (!threadId || visibleArtifacts.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="生成文档"
      className="coze-prototype-artifact-message-list"
      data-testid="task-artifact-message-list"
    >
      <div className="coze-prototype-artifact-message-heading">
        已生成 {visibleArtifacts.length} 个文件
      </div>
      <div className="coze-prototype-artifact-message-cards">
        {visibleArtifacts.map(artifact => {
          const artifactName = artifactFileName(artifact);
          const previewable = canPreviewArtifact(artifact);
          const activePreview =
            activeAction === `preview:${artifact.artifact_id}`;
          const activeDownload =
            activeAction === `download:${artifact.artifact_id}`;
          const actionLocked = Boolean(activeAction);

          return (
            <article
              className="coze-prototype-artifact-message-card"
              data-artifact-id={artifact.artifact_id}
              data-testid="task-artifact-message-item"
              key={artifact.artifact_id}
            >
              <div className="coze-prototype-artifact-message-main">
                <span className="coze-prototype-artifact-message-icon">
                  <IconCozDocument className="text-[16px]" />
                </span>
                <div className="coze-prototype-artifact-message-copy">
                  <div className="coze-prototype-artifact-message-title">
                    {artifactName}
                  </div>
                  <div className="coze-prototype-artifact-message-meta">
                    <span>{contentTypeLabel(artifact)}</span>
                    <span>{formatArtifactSize(artifact.size_bytes)}</span>
                  </div>
                </div>
              </div>
              <div className="coze-prototype-artifact-message-actions">
                {previewable ? (
                  <Button
                    aria-label={`预览文档 ${artifactName}`}
                    disabled={
                      actionLocked &&
                      activeAction !== `preview:${artifact.artifact_id}`
                    }
                    icon={<IconCozEye />}
                    loading={activePreview}
                    size="small"
                    theme="borderless"
                    onClick={() =>
                      void handleArtifactAction(artifact, 'preview')
                    }
                  >
                    预览
                  </Button>
                ) : null}
                <Button
                  aria-label={`下载文档 ${artifactName}`}
                  disabled={
                    actionLocked &&
                    activeAction !== `download:${artifact.artifact_id}`
                  }
                  icon={<IconCozDownload />}
                  loading={activeDownload}
                  size="small"
                  theme="borderless"
                  onClick={() =>
                    void handleArtifactAction(artifact, 'download')
                  }
                >
                  下载
                </Button>
              </div>
            </article>
          );
        })}
      </div>
      {error ? <div className="coze-prototype-error">{error}</div> : null}
      {inlinePreview ? (
        <TaskArtifactInlinePreview
          inlinePreview={inlinePreview}
          onClose={clearInlinePreview}
        />
      ) : null}
    </section>
  );
};
