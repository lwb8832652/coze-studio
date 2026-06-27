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

import { IconCozCross } from '@coze-arch/coze-design/icons';
import { Button, Table } from '@coze-arch/coze-design';

import type { ArtifactInlinePreview } from './task-artifacts-helpers';

interface ArtifactPreviewBaseState {
  artifactId: string;
  contentType: string;
  name: string;
}

export interface ArtifactContentPreviewState extends ArtifactPreviewBaseState {
  previewRenderer: 'content';
  preview: ArtifactInlinePreview;
}

export interface ArtifactImagePreviewState extends ArtifactPreviewBaseState {
  previewRenderer: 'image';
  url: string;
}

export type ArtifactInlinePreviewState =
  | ArtifactContentPreviewState
  | ArtifactImagePreviewState;

export const TaskArtifactInlinePreview = ({
  inlinePreview,
  onClose,
}: {
  inlinePreview: ArtifactInlinePreviewState;
  onClose: () => void;
}) => {
  const { contentType, name } = inlinePreview;
  const contentPreview =
    inlinePreview.previewRenderer === 'content'
      ? inlinePreview.preview
      : undefined;
  const tableColumns = useMemo(
    () =>
      contentPreview?.columns?.map(column => ({
        ...column,
        ellipsis: { showTitle: false },
        render: (value?: string) => (
          <span className="coze-prototype-artifact-preview-cell">
            {value ?? ''}
          </span>
        ),
      })) ?? [],
    [contentPreview?.columns],
  );

  return (
    <section
      aria-label={`预览 ${name}`}
      className="coze-prototype-artifact-preview"
      data-testid="task-artifact-inline-preview"
    >
      <div className="coze-prototype-artifact-preview-header">
        <div className="coze-prototype-artifact-preview-heading">
          <span className="coze-prototype-artifact-preview-title">{name}</span>
          <div className="coze-prototype-artifact-preview-meta">
            <span>{contentType || 'text/plain'}</span>
            <span>
              {inlinePreview.previewRenderer === 'image'
                ? 'image'
                : contentPreview?.kind}
            </span>
            {contentPreview?.truncated ? <span>已截断</span> : null}
          </div>
        </div>
        <Button
          aria-label={`关闭预览 ${name}`}
          icon={<IconCozCross />}
          size="small"
          theme="borderless"
          onClick={onClose}
        />
      </div>
      {inlinePreview.previewRenderer === 'image' ? (
        <img
          alt={name}
          className="coze-prototype-artifact-preview-image"
          data-testid="task-artifact-inline-preview-image"
          loading="lazy"
          referrerPolicy="no-referrer"
          src={inlinePreview.url}
        />
      ) : contentPreview?.kind === 'table' && contentPreview.columns?.length ? (
        <Table
          className="coze-prototype-artifact-preview-table"
          columns={tableColumns}
          dataSource={contentPreview.rows ?? []}
          data-testid="task-artifact-inline-preview-table"
          pagination={false}
          rowKey="key"
          size="small"
          scroll={{ x: 'max-content', y: 260 }}
        />
      ) : (
        <pre className="coze-prototype-artifact-preview-text">
          <span data-testid="task-artifact-inline-preview-text">
            {contentPreview?.text}
          </span>
        </pre>
      )}
    </section>
  );
};
