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

import { useEffect, useId, useMemo, useRef, useState } from 'react';

import type mermaid from 'mermaid';
import { IconCozCopy, IconCozDownload } from '@coze-arch/coze-design/icons';
import { MdBoxLazy } from '@coze-arch/bot-md-box-adapter/lazy';

import { copyTextToClipboard } from './task-clipboard';

type MermaidAPI = typeof mermaid;
interface MarkdownSegment {
  key: string;
  type: 'markdown' | 'mermaid';
  value: string;
}
type MermaidRenderStatus = 'loading' | 'ready' | 'error';

let mermaidRuntime: Promise<MermaidAPI> | undefined;

const getMermaidRuntime = () => {
  mermaidRuntime ??= import('mermaid').then(({ default: mermaidAPI }) => {
    mermaidAPI.initialize({
      startOnLoad: false,
      securityLevel: 'strict',
      htmlLabels: false,
      theme: 'default',
    });
    return mermaidAPI;
  });

  return mermaidRuntime;
};

const splitMarkdownByMermaid = (value: string): MarkdownSegment[] => {
  const pattern = /```mermaid[^\n\r]*\r?\n([\s\S]*?)```/gi;
  const segments: MarkdownSegment[] = [];
  let cursor = 0;
  let index = 0;

  for (const match of value.matchAll(pattern)) {
    const matchIndex = match.index ?? 0;
    const markdown = value.slice(cursor, matchIndex);
    if (markdown) {
      segments.push({
        key: `markdown-${index}`,
        type: 'markdown',
        value: markdown,
      });
      index += 1;
    }

    const diagram = String(match[1] ?? '').trim();
    if (diagram) {
      segments.push({
        key: `mermaid-${index}`,
        type: 'mermaid',
        value: diagram,
      });
      index += 1;
    }

    cursor = matchIndex + match[0].length;
  }

  const trailingMarkdown = value.slice(cursor);
  if (trailingMarkdown) {
    segments.push({
      key: `markdown-${index}`,
      type: 'markdown',
      value: trailingMarkdown,
    });
  }

  return segments.length
    ? segments
    : [{ key: 'markdown-0', type: 'markdown', value }];
};

const getSafeMermaidElement = (svg: string) => {
  const template = document.createElement('template');
  template.innerHTML = svg.trim();
  const element = template.content.querySelector('svg');

  if (!element || element.nodeName.toLowerCase() !== 'svg') {
    throw new Error('invalid mermaid svg');
  }

  return element.cloneNode(true);
};

const downloadMermaidSVG = ({
  fileName,
  svg,
}: {
  fileName: string;
  svg: string;
}) => {
  const blob = new Blob([svg], {
    type: 'image/svg+xml;charset=utf-8',
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `${fileName}.svg`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

const TaskMermaidBlock = ({ source }: { source: string }) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const rawID = useId();
  const renderID = useMemo(
    () => `task-mermaid-${rawID.replace(/[^a-zA-Z0-9_-]/g, '')}`,
    [rawID],
  );
  const [status, setStatus] = useState<MermaidRenderStatus>('loading');
  const [svg, setSVG] = useState('');

  useEffect(() => {
    let canceled = false;

    setStatus('loading');
    setSVG('');
    containerRef.current?.replaceChildren();

    void getMermaidRuntime()
      .then(mermaidAPI => mermaidAPI.render(renderID, source))
      .then(result => {
        if (canceled || !containerRef.current) {
          return;
        }

        const svgElement = getSafeMermaidElement(result.svg);
        containerRef.current.replaceChildren(svgElement);
        result.bindFunctions?.(containerRef.current);
        setSVG(result.svg);
        setStatus('ready');
      })
      .catch(() => {
        if (canceled) {
          return;
        }

        containerRef.current?.replaceChildren();
        setStatus('error');
      });

    return () => {
      canceled = true;
    };
  }, [renderID, source]);

  const handleCopySource = () => {
    void copyTextToClipboard(source);
  };

  const handleDownloadSVG = () => {
    if (!svg) {
      return;
    }

    downloadMermaidSVG({
      fileName: renderID,
      svg,
    });
  };

  return (
    <figure
      className="coze-prototype-mermaid"
      data-status={status}
      data-testid="task-mermaid-diagram"
    >
      {status === 'ready' || status === 'error' ? (
        <div
          className="coze-prototype-mermaid-actions"
          aria-label="Mermaid 图表操作"
        >
          {status === 'ready' ? (
            <button
              type="button"
              className="coze-prototype-mermaid-action"
              aria-label="下载 Mermaid SVG"
              onClick={handleDownloadSVG}
            >
              <IconCozDownload className="text-[14px]" />
            </button>
          ) : null}
          <button
            type="button"
            className="coze-prototype-mermaid-action"
            aria-label="复制 Mermaid 源码"
            onClick={handleCopySource}
          >
            <IconCozCopy className="text-[14px]" />
          </button>
        </div>
      ) : null}
      <div ref={containerRef} className="coze-prototype-mermaid-svg" />
      {status === 'loading' ? <figcaption>图表渲染中...</figcaption> : null}
      {status === 'error' ? (
        <>
          <figcaption>Mermaid 图表渲染失败，已保留源码</figcaption>
          <pre>
            <code>{source}</code>
          </pre>
        </>
      ) : null}
    </figure>
  );
};

export const TaskMarkdownContent = ({ value }: { value: string }) => {
  const segments = useMemo(() => splitMarkdownByMermaid(value), [value]);

  return (
    <div
      className="coze-prototype-markdown-content"
      data-testid="task-markdown-content"
    >
      {segments.map(segment =>
        segment.type === 'mermaid' ? (
          <TaskMermaidBlock key={segment.key} source={segment.value} />
        ) : (
          <MdBoxLazy key={segment.key} markDown={segment.value} />
        ),
      )}
    </div>
  );
};
