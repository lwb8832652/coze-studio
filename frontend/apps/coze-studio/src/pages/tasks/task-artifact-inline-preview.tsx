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

import {
  IconCozCode,
  IconCozCopy,
  IconCozCross,
  IconCozEye,
} from '@coze-arch/coze-design/icons';
import { Button, Table } from '@coze-arch/coze-design';

import { TaskMarkdownContent } from './task-markdown-content';
import { copyTextToClipboard } from './task-clipboard';
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

export interface ArtifactPDFPreviewState extends ArtifactPreviewBaseState {
  previewRenderer: 'pdf';
  url: string;
}

export type ArtifactInlinePreviewState =
  | ArtifactContentPreviewState
  | ArtifactImagePreviewState
  | ArtifactPDFPreviewState;

// eslint-disable-next-line @coze-arch/max-line-per-function -- P0 keeps preview branches together.
export const TaskArtifactInlinePreview = ({
  inlinePreview,
  onClose,
}: {
  inlinePreview: ArtifactInlinePreviewState;
  onClose: () => void;
}) => {
  const { contentType, name } = inlinePreview;
  const [copied, setCopied] = useState(false);
  const [viewMode, setViewMode] = useState<'code' | 'preview'>('preview');
  const contentPreview =
    inlinePreview.previewRenderer === 'content'
      ? inlinePreview.preview
      : undefined;
  const isMarkdownPreview =
    contentPreview?.kind === 'markdown' ||
    (contentPreview?.kind === 'text' &&
      (contentType.toLowerCase().includes('markdown') ||
        name.toLowerCase().endsWith('.md') ||
        name.toLowerCase().endsWith('.markdown')));
  const canSwitchView = Boolean(
    (contentPreview?.kind === 'text' || contentPreview?.kind === 'markdown') &&
      contentPreview.text,
  );
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
  const handleCopy = async () => {
    const text = contentPreview?.text;

    if (!text) {
      return;
    }

    const didCopy = await copyTextToClipboard(text);
    if (!didCopy) {
      return;
    }
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  return (
    <section
      aria-label={`预览 ${name}`}
      className="coze-prototype-artifact-preview"
      data-testid="task-artifact-inline-preview"
    >
      <div className="coze-prototype-artifact-preview-header">
        <div className="coze-prototype-artifact-preview-heading">
          <span className="coze-prototype-artifact-preview-title">{name}</span>
        </div>
        {canSwitchView ? (
          <div
            className="coze-prototype-artifact-preview-switch"
            role="tablist"
            aria-label="产物查看方式"
          >
            <button
              type="button"
              role="tab"
              aria-label="查看源码"
              aria-selected={viewMode === 'code'}
              data-active={viewMode === 'code'}
              onClick={() => setViewMode('code')}
            >
              <IconCozCode />
            </button>
            <button
              type="button"
              role="tab"
              aria-label="预览产物"
              aria-selected={viewMode === 'preview'}
              data-active={viewMode === 'preview'}
              onClick={() => setViewMode('preview')}
            >
              <IconCozEye />
            </button>
          </div>
        ) : (
          <div aria-hidden="true" />
        )}
        <div className="coze-prototype-artifact-preview-actions">
          {contentPreview?.text ? (
            <Button
              aria-label={copied ? `已复制文档 ${name}` : `复制文档 ${name}`}
              icon={<IconCozCopy />}
              size="small"
              theme="borderless"
              onClick={() => void handleCopy()}
            />
          ) : null}
          <Button
            aria-label={`关闭预览 ${name}`}
            icon={<IconCozCross />}
            size="small"
            theme="borderless"
            onClick={onClose}
          />
        </div>
      </div>
      {contentPreview?.truncated ? (
        <div
          className="coze-prototype-artifact-preview-truncated"
          data-testid="task-artifact-inline-preview-truncated"
        >
          内容较长，当前仅展示部分预览，下载文件可查看完整内容。
        </div>
      ) : null}
      {inlinePreview.previewRenderer === 'image' ? (
        <img
          alt={name}
          className="coze-prototype-artifact-preview-image"
          data-testid="task-artifact-inline-preview-image"
          loading="lazy"
          referrerPolicy="no-referrer"
          src={inlinePreview.url}
        />
      ) : inlinePreview.previewRenderer === 'pdf' ? (
        <iframe
          className="coze-prototype-artifact-preview-pdf"
          data-testid="task-artifact-inline-preview-pdf"
          referrerPolicy="no-referrer"
          sandbox=""
          src={inlinePreview.url}
          title={`预览 ${name}`}
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
      ) : isMarkdownPreview &&
        contentPreview?.text &&
        viewMode === 'preview' ? (
        <div
          className="coze-prototype-artifact-preview-markdown"
          data-testid="task-artifact-inline-preview-markdown"
        >
          <TaskMarkdownContent value={contentPreview.text} />
        </div>
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
