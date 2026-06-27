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

type TaskThreadArtifact = workbenchTask.TaskThreadArtifact;
type ArtifactPreviewModeCarrier = Pick<
  TaskThreadArtifact,
  'content_type' | 'preview_mode'
>;

export type ArtifactInlinePreviewKind = 'text' | 'markdown' | 'json' | 'table';
export type ArtifactPreviewFamily = 'text' | 'image' | 'pdf';

export interface ArtifactInlinePreviewTableColumn {
  dataIndex: string;
  title: string;
}

export interface ArtifactInlinePreviewTableRow {
  key: string;
  [key: string]: string;
}

export interface ArtifactInlinePreview {
  columns?: ArtifactInlinePreviewTableColumn[];
  kind: ArtifactInlinePreviewKind;
  rows?: ArtifactInlinePreviewTableRow[];
  text: string;
  truncated: boolean;
}

const INLINE_PREVIEW_MAX_CHARS = 128 * 1024;
const INLINE_TABLE_MAX_COLUMNS = 12;
const INLINE_TABLE_MAX_ROWS = 50;

const TEXT_PREVIEW_CONTENT_TYPES = new Set([
  'text/plain',
  'text/markdown',
  'text/csv',
  'text/tab-separated-values',
  'application/json',
]);

const IMAGE_PREVIEW_CONTENT_TYPES = new Set([
  'image/png',
  'image/jpeg',
  'image/gif',
  'image/webp',
  'image/bmp',
  'image/tiff',
]);

const artifactContentType = (contentType: string) =>
  contentType.split(';')[0]?.trim().toLowerCase() ?? '';

export const artifactPreviewFamily = (
  artifact: ArtifactPreviewModeCarrier,
): ArtifactPreviewFamily | null => {
  const contentType = artifactContentType(artifact.content_type);
  switch (artifact.preview_mode) {
    case 'text':
      return TEXT_PREVIEW_CONTENT_TYPES.has(contentType) ? 'text' : null;
    case 'image':
      return IMAGE_PREVIEW_CONTENT_TYPES.has(contentType) ? 'image' : null;
    case 'pdf':
      return contentType === 'application/pdf' ? 'pdf' : null;
    default:
      return null;
  }
};

const clampInlinePreviewText = (content: string) => ({
  text: content.slice(0, INLINE_PREVIEW_MAX_CHARS),
  truncated: content.length > INLINE_PREVIEW_MAX_CHARS,
});

const parseDelimitedRows = (content: string, delimiter: ',' | '\t') => {
  const rows: string[][] = [];
  let currentField = '';
  let currentRow: string[] = [];
  let inQuotes = false;

  for (let index = 0; index < content.length; index += 1) {
    const char = content[index];
    const nextChar = content[index + 1];

    if (char === '"') {
      if (inQuotes && nextChar === '"') {
        currentField += '"';
        index += 1;
      } else {
        inQuotes = !inQuotes;
      }
      continue;
    }

    if (!inQuotes && char === delimiter) {
      currentRow.push(currentField);
      currentField = '';
      continue;
    }

    if (!inQuotes && (char === '\n' || char === '\r')) {
      currentRow.push(currentField);
      rows.push(currentRow);
      currentField = '';
      currentRow = [];
      if (char === '\r' && nextChar === '\n') {
        index += 1;
      }
      if (rows.length > INLINE_TABLE_MAX_ROWS) {
        break;
      }
      continue;
    }

    currentField += char;
  }

  if (currentField || currentRow.length) {
    currentRow.push(currentField);
    rows.push(currentRow);
  }

  return rows;
};

const buildTablePreview = (
  text: string,
  contentType: string,
  truncated: boolean,
): ArtifactInlinePreview => {
  const delimiter = contentType === 'text/tab-separated-values' ? '\t' : ',';
  const [header = [], ...dataRows] = parseDelimitedRows(text, delimiter);
  const columnCount = Math.min(
    Math.max(header.length, ...dataRows.map(row => row.length), 0),
    INLINE_TABLE_MAX_COLUMNS,
  );
  const columns = Array.from({ length: columnCount }, (_, index) => ({
    dataIndex: `col_${index}`,
    title: header[index]?.trim() || `Column ${index + 1}`,
  }));
  const rows = dataRows.slice(0, INLINE_TABLE_MAX_ROWS).map((row, rowIndex) =>
    columns.reduce<ArtifactInlinePreviewTableRow>(
      (record, column, columnIndex) => {
        record[column.dataIndex] = row[columnIndex] ?? '';
        return record;
      },
      { key: `row_${rowIndex}` },
    ),
  );

  return {
    columns,
    kind: 'table',
    rows,
    text,
    truncated:
      truncated ||
      dataRows.length > INLINE_TABLE_MAX_ROWS ||
      header.length > INLINE_TABLE_MAX_COLUMNS ||
      dataRows.some(row => row.length > INLINE_TABLE_MAX_COLUMNS),
  };
};

export const formatArtifactSize = (sizeBytes: number) => {
  if (!Number.isFinite(sizeBytes) || sizeBytes <= 0) {
    return '0 B';
  }
  if (sizeBytes < 1024) {
    return `${sizeBytes} B`;
  }
  if (sizeBytes < 1024 * 1024) {
    return `${(sizeBytes / 1024).toFixed(1)} KB`;
  }
  return `${(sizeBytes / 1024 / 1024).toFixed(1)} MB`;
};

export const artifactFileName = (artifact: TaskThreadArtifact) => {
  const title = artifact.title.trim();
  if (title) {
    return title;
  }
  const segments = artifact.virtual_path.split('/').filter(Boolean);
  return segments.at(-1) ?? 'artifact';
};

export const fileNameFromContentDisposition = (contentDisposition: string) => {
  const encodedMatch = /filename\*=UTF-8''([^;]+)/i.exec(contentDisposition);
  if (encodedMatch?.[1]) {
    try {
      return decodeURIComponent(encodedMatch[1]);
    } catch {
      return encodedMatch[1];
    }
  }

  const plainMatch = /filename="?([^";]+)"?/i.exec(contentDisposition);
  return plainMatch?.[1] ?? '';
};

export const artifactInlinePreviewKind = (
  artifact: ArtifactPreviewModeCarrier,
): Exclude<ArtifactInlinePreviewKind, 'table'> | 'csv' | 'tsv' | null => {
  if (artifactPreviewFamily(artifact) !== 'text') {
    return null;
  }
  const contentType = artifactContentType(artifact.content_type);
  switch (contentType) {
    case 'application/json':
      return 'json';
    case 'text/csv':
      return 'csv';
    case 'text/tab-separated-values':
      return 'tsv';
    case 'text/markdown':
      return 'markdown';
    case 'text/plain':
      return 'text';
    default:
      return null;
  }
};

export const buildArtifactInlinePreview = ({
  content,
  contentType,
}: {
  content: string;
  contentType: string;
}): ArtifactInlinePreview => {
  const normalizedContentType = artifactContentType(contentType);
  const { text, truncated } = clampInlinePreviewText(content);

  if (normalizedContentType === 'application/json') {
    try {
      return {
        kind: 'json',
        text: JSON.stringify(JSON.parse(text), null, 2),
        truncated,
      };
    } catch {
      return { kind: 'json', text, truncated };
    }
  }

  if (
    normalizedContentType === 'text/csv' ||
    normalizedContentType === 'text/tab-separated-values'
  ) {
    return buildTablePreview(text, normalizedContentType, truncated);
  }

  if (normalizedContentType === 'text/markdown') {
    return { kind: 'markdown', text, truncated };
  }

  return { kind: 'text', text, truncated };
};

export const canPreviewArtifact = (artifact: TaskThreadArtifact) =>
  Boolean(artifactPreviewFamily(artifact));

export const artifactScanStatus = (artifact: TaskThreadArtifact) => {
  try {
    const metadata = JSON.parse(artifact.metadata || '{}') as {
      scan_status?: unknown;
      scanStatus?: unknown;
    };
    const status =
      typeof metadata.scan_status === 'string'
        ? metadata.scan_status
        : metadata.scanStatus;
    return typeof status === 'string' ? status.trim().toLowerCase() : '';
  } catch {
    return '';
  }
};

export const scanStatusColor = (status: string) => {
  switch (status) {
    case 'clean':
    case 'succeeded':
      return 'green';
    case 'processing':
      return 'light-blue';
    case 'pending':
      return 'grey';
    case 'blocked':
    case 'infected':
    case 'quarantined':
    case 'failed':
      return 'red';
    default:
      return 'amber';
  }
};
