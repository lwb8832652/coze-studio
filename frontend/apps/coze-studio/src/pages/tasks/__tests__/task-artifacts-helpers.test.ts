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

import { describe, expect, it } from 'vitest';

import {
  artifactDisplayPath,
  artifactPreviewFamily,
  artifactInlinePreviewKind,
  buildArtifactInlinePreview,
  canPreviewArtifact,
} from '../task-artifacts-helpers';

const artifactWithPreview = ({
  contentType,
  previewMode,
}: {
  contentType: string;
  previewMode: string;
}) =>
  ({
    content_type: contentType,
    preview_mode: previewMode,
    title: 'artifact',
    virtual_path: '/mnt/user-data/outputs/artifact',
  }) as Parameters<typeof canPreviewArtifact>[0];

describe('task artifact preview helpers', () => {
  it('builds safe inline previews for JSON, CSV, and Markdown artifacts', () => {
    expect(
      artifactInlinePreviewKind({
        content_type: 'application/json; charset=utf-8',
        preview_mode: 'text',
      }),
    ).toBe('json');
    expect(
      buildArtifactInlinePreview({
        content: '{"b":2,"a":{"ok":true}}',
        contentType: 'application/json; charset=utf-8',
      }),
    ).toEqual({
      kind: 'json',
      text: '{\n  "b": 2,\n  "a": {\n    "ok": true\n  }\n}',
      truncated: false,
    });

    expect(
      buildArtifactInlinePreview({
        content: 'name,score\nAlice,10\nBob,12',
        contentType: 'text/csv',
      }),
    ).toEqual({
      columns: [
        { dataIndex: 'col_0', title: 'name' },
        { dataIndex: 'col_1', title: 'score' },
      ],
      kind: 'table',
      rows: [
        { col_0: 'Alice', col_1: '10', key: 'row_0' },
        { col_0: 'Bob', col_1: '12', key: 'row_1' },
      ],
      text: 'name,score\nAlice,10\nBob,12',
      truncated: false,
    });

    expect(
      buildArtifactInlinePreview({
        content: '# Title\n\n<script>alert(1)</script>',
        contentType: 'text/markdown',
      }),
    ).toEqual({
      kind: 'markdown',
      text: '# Title\n\n<script>alert(1)</script>',
      truncated: false,
    });
  });

  it('centralizes MIME preview families and rejects active-content mismatches', () => {
    expect(
      artifactPreviewFamily({
        content_type: 'Application/JSON; Charset=UTF-8',
        preview_mode: 'text',
      }),
    ).toBe('text');
    expect(
      artifactPreviewFamily({
        content_type: 'image/png; name=preview.png',
        preview_mode: 'image',
      }),
    ).toBe('image');
    expect(
      artifactPreviewFamily({
        content_type: 'application/pdf; name=report.pdf',
        preview_mode: 'pdf',
      }),
    ).toBe('pdf');

    for (const contentType of [
      'text/html; charset=utf-8',
      'application/xhtml+xml',
      'image/svg+xml',
      'application/octet-stream',
      '',
    ]) {
      expect(
        artifactPreviewFamily({
          content_type: contentType,
          preview_mode: 'text',
        }),
      ).toBeNull();
      expect(
        canPreviewArtifact(
          artifactWithPreview({ contentType, previewMode: 'text' }),
        ),
      ).toBe(false);
    }

    expect(
      artifactPreviewFamily({
        content_type: 'image/svg+xml',
        preview_mode: 'image',
      }),
    ).toBeNull();
    expect(
      canPreviewArtifact(
        artifactWithPreview({
          contentType: 'image/svg+xml',
          previewMode: 'image',
        }),
      ),
    ).toBe(false);
  });

  it('keeps internal object storage paths out of artifact display paths', () => {
    expect(
      artifactDisplayPath(
        artifactWithPreview({
          contentType: 'text/markdown',
          previewMode: 'text',
        }),
      ),
    ).toBe('/mnt/user-data/outputs/artifact');
    expect(
      artifactDisplayPath({
        ...artifactWithPreview({
          contentType: 'text/markdown',
          previewMode: 'text',
        }),
        virtual_path: 'agent-runtime://objects/private.md?token=secret',
      }),
    ).toBe('');
  });
});
