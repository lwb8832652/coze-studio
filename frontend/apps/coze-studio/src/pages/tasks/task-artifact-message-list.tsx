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

import { useEffect, useMemo, useRef } from 'react';

import {
  IconCozDocument,
  IconCozDownload,
  IconCozPlugin,
} from '@coze-arch/coze-design/icons';
import { Button } from '@coze-arch/coze-design';

import type { WorkbenchArtifact } from '../workbench/thread-client';
import {
  artifactScanStatus,
  artifactSupportsCapability,
  artifactFileExtension,
  artifactFileName,
  canPreviewArtifact,
  isSkillArtifact,
} from './task-artifacts-helpers';
import { TaskArtifactInlinePreview } from './task-artifact-inline-preview';
import {
  type TaskArtifactActions,
  useTaskArtifactActions,
} from './task-artifact-actions';
import type { ArtifactScanReviewDecision } from './service';

type TaskThreadArtifact = WorkbenchArtifact;

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
  skill: 'Skill',
};

const ARTIFACT_SOURCE_LABELS: Record<string, string> = {
  agent_generated: '智能体生成',
  external_reference: '外部引用',
  tool_output: '工具输出',
  user_upload: '用户上传',
};

const ARTIFACT_STATUS_LABELS: Record<string, string> = {
  blocked: '已阻止',
  expired: '已失效',
  failed: '生成失败',
  processing: '生成中',
  ready: '已生成',
};

const contentTypeLabel = (artifact: TaskThreadArtifact) => {
  const contentType = artifact.content_type.split(';')[0]?.trim().toLowerCase();
  const extension = artifactFileExtension(artifact);
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

const ARTIFACT_REVIEW_ACTIONS: Array<{
  decision: ArtifactScanReviewDecision;
  label: string;
}> = [
  { decision: 'release', label: '放行' },
  { decision: 'quarantine', label: '隔离' },
  { decision: 'block', label: '阻断' },
];

const TaskArtifactMessageCard = ({
  activeAction,
  artifact,
  handleArtifactAction,
  handleReviewArtifact,
  renderReviewActions,
}: {
  activeAction: string;
  artifact: TaskThreadArtifact;
  handleArtifactAction: (
    artifact: TaskThreadArtifact,
    action: 'download' | 'install_skill' | 'preview',
  ) => void | Promise<void>;
  handleReviewArtifact: (
    artifact: TaskThreadArtifact,
    decision: ArtifactScanReviewDecision,
  ) => void | Promise<void>;
  renderReviewActions: boolean;
}) => {
  const artifactName = artifactFileName(artifact);
  const previewable = canPreviewArtifact(artifact);
  const downloadable = artifactSupportsCapability(artifact, 'download');
  const activeDownload = activeAction === `download:${artifact.artifact_id}`;
  const activeInstall =
    activeAction === `install_skill:${artifact.artifact_id}`;
  const actionLocked = Boolean(activeAction);
  const skillArtifact = isSkillArtifact(artifact);
  const scanStatus = artifactScanStatus(artifact);
  const reviewable = Boolean(
    renderReviewActions && scanStatus && scanStatus !== 'clean',
  );
  const openArtifact = () => {
    if (!previewable || actionLocked) {
      return;
    }
    void handleArtifactAction(artifact, 'preview');
  };

  return (
    <article
      className="coze-prototype-artifact-message-card"
      data-artifact-id={artifact.artifact_id}
      data-previewable={previewable}
      data-testid="task-artifact-message-item"
      key={artifact.artifact_id}
      role={previewable ? 'button' : undefined}
      tabIndex={previewable ? 0 : undefined}
      onClick={openArtifact}
      onKeyDown={event => {
        if (previewable && (event.key === 'Enter' || event.key === ' ')) {
          event.preventDefault();
          openArtifact();
        }
      }}
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
            <span>{contentTypeLabel(artifact)} file</span>
            {artifact.source ? (
              <span>
                {ARTIFACT_SOURCE_LABELS[artifact.source] ?? artifact.source}
              </span>
            ) : null}
            {artifact.generation_status ? (
              <span>
                {ARTIFACT_STATUS_LABELS[artifact.generation_status] ??
                  artifact.generation_status}
              </span>
            ) : null}
            {artifact.is_primary ? <span>主要产物</span> : null}
          </div>
        </div>
      </div>
      <div className="coze-prototype-artifact-message-actions">
        {reviewable
          ? ARTIFACT_REVIEW_ACTIONS.map(({ decision, label }) => {
              const reviewAction = `review:${decision}:${artifact.artifact_id}`;

              return (
                <Button
                  aria-label={`${label}产物 ${artifactName}`}
                  disabled={actionLocked && activeAction !== reviewAction}
                  key={decision}
                  loading={activeAction === reviewAction}
                  size="small"
                  theme="borderless"
                  type={decision === 'block' ? 'danger' : 'tertiary'}
                  onClick={event => {
                    event.stopPropagation();
                    void handleReviewArtifact(artifact, decision);
                  }}
                >
                  {label}
                </Button>
              );
            })
          : null}
        {skillArtifact ? (
          <Button
            aria-label={`安装技能 ${artifactName}`}
            disabled={actionLocked && !activeInstall}
            icon={<IconCozPlugin />}
            loading={activeInstall}
            size="small"
            theme="borderless"
            type="tertiary"
            onClick={event => {
              event.stopPropagation();
              void handleArtifactAction(artifact, 'install_skill');
            }}
          >
            安装
          </Button>
        ) : null}
        {downloadable ? (
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
            type="tertiary"
            onClick={event => {
              event.stopPropagation();
              void handleArtifactAction(artifact, 'download');
            }}
          >
            下载
          </Button>
        ) : null}
      </div>
    </article>
  );
};

export const TaskArtifactMessageList = ({
  artifactActions,
  artifacts,
  autoPreview = true,
  renderFeedback = true,
  renderReviewActions = true,
  spaceId,
  threadId,
}: {
  artifactActions?: TaskArtifactActions;
  artifacts: TaskThreadArtifact[];
  autoPreview?: boolean;
  renderFeedback?: boolean;
  renderReviewActions?: boolean;
  spaceId?: string;
  threadId?: string;
}) => {
  const localArtifactActions = useTaskArtifactActions({ spaceId, threadId });
  const {
    activeAction,
    clearInlinePreview,
    error,
    handleArtifactAction,
    handleReviewArtifact,
    inlinePreview,
  } = artifactActions ?? localArtifactActions;
  const visibleArtifacts = useMemo(
    () =>
      [...artifacts].sort(
        (left, right) =>
          Number(Boolean(right.is_primary)) -
            Number(Boolean(left.is_primary)) ||
          left.created_at - right.created_at ||
          left.artifact_id.localeCompare(right.artifact_id),
      ),
    [artifacts],
  );
  const autoPreviewedArtifactId = useRef('');
  const firstPreviewableArtifact = visibleArtifacts.find(canPreviewArtifact);

  useEffect(() => {
    if (
      !threadId ||
      !autoPreview ||
      !firstPreviewableArtifact ||
      inlinePreview ||
      activeAction ||
      autoPreviewedArtifactId.current === firstPreviewableArtifact.artifact_id
    ) {
      return;
    }

    autoPreviewedArtifactId.current = firstPreviewableArtifact.artifact_id;
    void handleArtifactAction(firstPreviewableArtifact, 'preview');
  }, [
    activeAction,
    autoPreview,
    firstPreviewableArtifact,
    handleArtifactAction,
    inlinePreview,
    threadId,
  ]);

  if (!threadId || visibleArtifacts.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="生成文档"
      className="coze-prototype-artifact-message-list"
      data-testid="task-artifact-message-list"
    >
      <div className="coze-prototype-artifact-message-cards">
        {visibleArtifacts.map(artifact => (
          <TaskArtifactMessageCard
            activeAction={activeAction}
            artifact={artifact}
            handleArtifactAction={handleArtifactAction}
            handleReviewArtifact={handleReviewArtifact}
            key={artifact.artifact_id}
            renderReviewActions={renderReviewActions}
          />
        ))}
      </div>
      {renderFeedback ? (
        <TaskArtifactFeedback
          clearInlinePreview={clearInlinePreview}
          error={error}
          inlinePreview={inlinePreview}
        />
      ) : null}
    </section>
  );
};

export const TaskArtifactFeedback = ({
  clearInlinePreview,
  error,
  inlinePreview,
}: {
  clearInlinePreview: () => void;
  error: string;
  inlinePreview: ReturnType<typeof useTaskArtifactActions>['inlinePreview'];
}) => (
  <>
    {error ? (
      <div className="coze-prototype-error" role="alert">
        {error}
      </div>
    ) : null}
    {inlinePreview ? (
      <aside
        aria-label="任务产物预览"
        className="coze-prototype-artifact-side-preview"
        data-layout="deerflow-split"
        data-testid="task-artifact-side-preview"
        data-width-mode="deerflow-60-40"
      >
        <TaskArtifactInlinePreview
          inlinePreview={inlinePreview}
          onClose={clearInlinePreview}
        />
      </aside>
    ) : null}
  </>
);
