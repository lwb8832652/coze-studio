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
  IconCozDocument,
  IconCozDownload,
  IconCozEye,
  IconCozPlugin,
  IconCozTrashCan,
} from '@coze-arch/coze-design/icons';
import { Button, Popconfirm, Tag } from '@coze-arch/coze-design';

import {
  artifactDisplayPath,
  artifactFileName,
  artifactScanStatus,
  canPreviewArtifact,
  formatArtifactSize,
  isSkillArtifact,
  scanStatusColor,
} from './task-artifacts-helpers';
import type { ArtifactScanReviewDecision } from './service';

type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;
export type ArtifactActionMode = 'preview' | 'download' | 'install_skill';

interface ArtifactReviewActionButtonProps {
  activeAction: string;
  artifact: TaskThreadArtifact;
  artifactName: string;
  decision: ArtifactScanReviewDecision;
  label: string;
  type?: 'danger';
  onReviewArtifact: (
    artifact: TaskThreadArtifact,
    decision: ArtifactScanReviewDecision,
  ) => void | Promise<void>;
}

const ArtifactReviewActionButton = ({
  activeAction,
  artifact,
  artifactName,
  decision,
  label,
  onReviewArtifact,
  type,
}: ArtifactReviewActionButtonProps) => {
  const actionKey = `review:${decision}:${artifact.artifact_id}`;
  const actionLocked = Boolean(activeAction);

  return (
    <Button
      aria-label={`${label}产物 ${artifactName}`}
      disabled={actionLocked && activeAction !== actionKey}
      loading={activeAction === actionKey}
      size="small"
      theme="borderless"
      type={type}
      onClick={() => void onReviewArtifact(artifact, decision)}
    >
      {label}
    </Button>
  );
};

const ArtifactReviewActions = ({
  activeAction,
  artifact,
  artifactName,
  onReviewArtifact,
  scanStatus,
}: {
  activeAction: string;
  artifact: TaskThreadArtifact;
  artifactName: string;
  onReviewArtifact: ArtifactReviewActionButtonProps['onReviewArtifact'];
  scanStatus: string;
}) => {
  if (!scanStatus || scanStatus === 'clean') {
    return null;
  }

  return (
    <>
      <ArtifactReviewActionButton
        activeAction={activeAction}
        artifact={artifact}
        artifactName={artifactName}
        decision="release"
        label="放行"
        onReviewArtifact={onReviewArtifact}
      />
      {scanStatus !== 'quarantined' ? (
        <ArtifactReviewActionButton
          activeAction={activeAction}
          artifact={artifact}
          artifactName={artifactName}
          decision="quarantine"
          label="隔离"
          onReviewArtifact={onReviewArtifact}
        />
      ) : null}
      {scanStatus !== 'blocked' ? (
        <ArtifactReviewActionButton
          activeAction={activeAction}
          artifact={artifact}
          artifactName={artifactName}
          decision="block"
          label="阻断"
          type="danger"
          onReviewArtifact={onReviewArtifact}
        />
      ) : null}
    </>
  );
};

export const TaskArtifactListItem = ({
  activeAction,
  artifact,
  onArtifactAction,
  onDeleteArtifact,
  onReviewArtifact,
}: {
  activeAction: string;
  artifact: TaskThreadArtifact;
  onArtifactAction: (
    artifact: TaskThreadArtifact,
    mode: ArtifactActionMode,
  ) => void | Promise<void>;
  onDeleteArtifact: (artifact: TaskThreadArtifact) => void | Promise<void>;
  onReviewArtifact: ArtifactReviewActionButtonProps['onReviewArtifact'];
}) => {
  const artifactName = artifactFileName(artifact);
  const actionLocked = Boolean(activeAction);
  const previewable = canPreviewArtifact(artifact);
  const scanStatus = artifactScanStatus(artifact);
  const displayPath = artifactDisplayPath(artifact);
  const skillArtifact = isSkillArtifact(artifact);

  return (
    <div
      className="coze-prototype-artifact-item"
      data-artifact-id={artifact.artifact_id}
      data-testid="task-artifact-item"
    >
      <div className="coze-prototype-artifact-main">
        <span className="coze-prototype-artifact-icon">
          <IconCozDocument className="text-[16px]" />
        </span>
        <div className="coze-prototype-artifact-content">
          <div className="coze-prototype-artifact-title-row">
            <span className="coze-prototype-artifact-title">
              {artifact.title || artifactName}
            </span>
            <Tag>{artifact.preview_mode}</Tag>
            {scanStatus ? (
              <Tag color={scanStatusColor(scanStatus)}>{scanStatus}</Tag>
            ) : null}
          </div>
          <div className="coze-prototype-artifact-meta">
            <span>{artifact.artifact_type || 'artifact'}</span>
            <span>{formatArtifactSize(artifact.size_bytes)}</span>
            <span>{artifact.content_type || 'application/octet-stream'}</span>
          </div>
          {displayPath ? (
            <div className="coze-prototype-artifact-path">{displayPath}</div>
          ) : null}
        </div>
      </div>
      <div className="coze-prototype-artifact-actions">
        {previewable ? (
          <Button
            aria-label={`预览 ${artifactName}`}
            disabled={
              actionLocked && activeAction !== `preview:${artifact.artifact_id}`
            }
            icon={<IconCozEye />}
            loading={activeAction === `preview:${artifact.artifact_id}`}
            size="small"
            theme="borderless"
            onClick={() => void onArtifactAction(artifact, 'preview')}
          >
            预览
          </Button>
        ) : null}
        <ArtifactReviewActions
          activeAction={activeAction}
          artifact={artifact}
          artifactName={artifactName}
          scanStatus={scanStatus}
          onReviewArtifact={onReviewArtifact}
        />
        {skillArtifact ? (
          <Button
            aria-label={`安装技能 ${artifactName}`}
            disabled={
              actionLocked &&
              activeAction !== `install_skill:${artifact.artifact_id}`
            }
            icon={<IconCozPlugin />}
            loading={activeAction === `install_skill:${artifact.artifact_id}`}
            size="small"
            theme="borderless"
            onClick={() => void onArtifactAction(artifact, 'install_skill')}
          >
            安装
          </Button>
        ) : null}
        <Button
          aria-label={`下载 ${artifactName}`}
          disabled={
            actionLocked && activeAction !== `download:${artifact.artifact_id}`
          }
          icon={<IconCozDownload />}
          loading={activeAction === `download:${artifact.artifact_id}`}
          size="small"
          theme="borderless"
          onClick={() => void onArtifactAction(artifact, 'download')}
        >
          下载
        </Button>
        <Popconfirm
          title="移除任务产物？"
          content="只会从任务详情隐藏该产物，不会删除底层文件。"
          okText="移除"
          cancelText="取消"
          okType="danger"
          cancelButtonProps={{ autoFocus: true }}
          onConfirm={() => onDeleteArtifact(artifact)}
        >
          <Button
            aria-label={`删除 ${artifactName}`}
            disabled={
              actionLocked && activeAction !== `delete:${artifact.artifact_id}`
            }
            icon={<IconCozTrashCan />}
            loading={activeAction === `delete:${artifact.artifact_id}`}
            size="small"
            theme="borderless"
            type="danger"
          >
            删除
          </Button>
        </Popconfirm>
      </div>
    </div>
  );
};
