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

import { useEffect, useRef, useState, type ReactNode } from 'react';

import {
  IconCozDownload,
  IconCozImage,
  IconCozLoading,
  IconCozMinus,
  IconCozMusic,
  IconCozPlus,
  IconCozRefresh,
  IconCozVerifyFailed,
  IconCozVideo,
} from '@coze-arch/coze-design/icons';
import { Button } from '@coze-arch/coze-design';

import {
  type JournalMediaPreviewState,
  useJournalMediaPreview,
} from '../use-journal-media-preview';
import {
  type JournalMediaContext,
  type JournalMediaKind,
  journalMediaKind,
} from '../journal-media-model';
import { artifactSupportsCapability } from '../../task-artifacts-helpers';
import type { WorkbenchArtifact } from '../../../workbench/thread-client';

const minimumImageZoom = 0.25;
const maximumImageZoom = 4;
const imageZoomStep = 0.25;
const percentScale = 100;

const mediaIcon = (kind: JournalMediaKind): ReactNode => {
  switch (kind) {
    case 'audio':
      return <IconCozMusic />;
    case 'video':
      return <IconCozVideo />;
    case 'media_collection':
    case 'image':
    default:
      return <IconCozImage />;
  }
};

const MediaToolbar = ({
  actions,
  artifact,
  kind,
  onDownload,
}: {
  actions?: ReactNode;
  artifact: WorkbenchArtifact;
  kind: JournalMediaKind;
  onDownload?: (artifact: WorkbenchArtifact) => void | Promise<void>;
}) => (
  <header className="journal-media-toolbar">
    <span className="journal-media-kind-icon" aria-hidden="true">
      {mediaIcon(kind)}
    </span>
    <div className="journal-media-title">
      <strong>{artifact.title}</strong>
      <span>{artifact.content_type}</span>
    </div>
    <div className="journal-media-actions">
      {actions}
      {onDownload && artifactSupportsCapability(artifact, 'download') ? (
        <Button
          aria-label={`下载 ${artifact.title}`}
          icon={<IconCozDownload />}
          size="small"
          theme="borderless"
          type="tertiary"
          onClick={() => void onDownload(artifact)}
        />
      ) : null}
    </div>
  </header>
);

const MediaState = ({
  artifact,
  state,
  onDownload,
  onRefresh,
}: {
  artifact?: WorkbenchArtifact;
  state: JournalMediaPreviewState;
  onDownload?: (artifact: WorkbenchArtifact) => void | Promise<void>;
  onRefresh: () => void;
}) => {
  const content = {
    empty: {
      icon: <IconCozImage />,
      title: '暂无可预览内容',
      copy: '当前媒体集合还没有可展示的安全产物。',
    },
    loading: {
      icon: <IconCozLoading />,
      title: '正在加载预览',
      copy: '正在获取本次预览所需的短时访问地址。',
    },
    processing: {
      icon: <IconCozLoading />,
      title: '正在生成预览',
      copy: '产物事件已经建立，预览文件仍在生成。',
    },
    expired: {
      icon: <IconCozRefresh />,
      title: '预览链接已失效',
      copy: '短时访问地址已经过期，可以重新获取。',
    },
    no_permission: {
      icon: <IconCozVerifyFailed />,
      title: '无法查看此内容',
      copy: '内容不存在或无权访问。',
    },
    blocked: {
      icon: <IconCozVerifyFailed />,
      title: '内容未通过安全检查',
      copy: '该产物暂不能在线预览或下载。',
    },
    download_only: {
      icon: <IconCozDownload />,
      title: '当前格式仅支持下载',
      copy: '当前格式没有可用的安全预览器。',
    },
    error: {
      icon: <IconCozVerifyFailed />,
      title: '预览加载失败',
      copy: '暂时无法读取该产物，可以重新获取。',
    },
    ready: {
      icon: <IconCozImage />,
      title: '预览已就绪',
      copy: '',
    },
  }[state.status];
  const refreshable = state.status === 'expired' || state.status === 'error';
  const downloadable =
    state.status === 'download_only' && artifact && onDownload;

  return (
    <div className="journal-media-state" data-status={state.status}>
      <div className="journal-media-state-panel">
        <span aria-hidden="true">{content.icon}</span>
        <h3>{content.title}</h3>
        {content.copy ? <p>{content.copy}</p> : null}
        {refreshable ? (
          <Button
            icon={<IconCozRefresh />}
            size="small"
            theme="light"
            type="primary"
            onClick={onRefresh}
          >
            刷新链接
          </Button>
        ) : null}
        {downloadable ? (
          <Button
            icon={<IconCozDownload />}
            size="small"
            theme="light"
            type="primary"
            onClick={() => void onDownload(artifact)}
          >
            下载文件
          </Button>
        ) : null}
      </div>
    </div>
  );
};

const JournalImagePreview = ({
  artifact,
  url,
  onDownload,
}: {
  artifact: WorkbenchArtifact;
  url: string;
  onDownload?: (artifact: WorkbenchArtifact) => void | Promise<void>;
}) => {
  const [fit, setFit] = useState(true);
  const [zoom, setZoom] = useState(1);
  const imageRef = useRef<HTMLImageElement>(null);
  useEffect(
    () => () => {
      imageRef.current?.removeAttribute('src');
    },
    [url],
  );
  return (
    <div className="journal-media-preview">
      <MediaToolbar
        artifact={artifact}
        kind="image"
        onDownload={onDownload}
        actions={
          <>
            <button
              type="button"
              aria-label="缩小图片"
              disabled={zoom <= minimumImageZoom}
              title="缩小"
              onClick={() => {
                setFit(false);
                setZoom(value =>
                  Math.max(minimumImageZoom, value - imageZoomStep),
                );
              }}
            >
              <IconCozMinus />
            </button>
            <button
              type="button"
              aria-label="按原始大小显示"
              data-active={!fit && zoom === 1}
              onClick={() => {
                setFit(false);
                setZoom(1);
              }}
            >
              100%
            </button>
            <button
              type="button"
              aria-label="放大图片"
              disabled={zoom >= maximumImageZoom}
              title="放大"
              onClick={() => {
                setFit(false);
                setZoom(value =>
                  Math.min(maximumImageZoom, value + imageZoomStep),
                );
              }}
            >
              <IconCozPlus />
            </button>
            <button
              type="button"
              aria-label="适应窗口"
              data-active={fit}
              onClick={() => setFit(true)}
            >
              适应
            </button>
          </>
        }
      />
      <div
        className="journal-image-stage"
        data-fit={fit}
        data-journal-scroll-key="media-image-stage"
      >
        <img
          alt={artifact.title}
          ref={imageRef}
          referrerPolicy="no-referrer"
          src={url}
          style={fit ? undefined : { width: `${zoom * percentScale}%` }}
        />
      </div>
    </div>
  );
};

const JournalAudioPreview = ({
  artifact,
  url,
  onDownload,
}: {
  artifact: WorkbenchArtifact;
  url: string;
  onDownload?: (artifact: WorkbenchArtifact) => void | Promise<void>;
}) => {
  const playerRef = useRef<HTMLAudioElement>(null);
  useEffect(
    () => () => {
      playerRef.current?.pause();
      playerRef.current?.removeAttribute('src');
      playerRef.current?.load();
    },
    [url],
  );
  return (
    <div className="journal-media-preview">
      <MediaToolbar artifact={artifact} kind="audio" onDownload={onDownload} />
      <div
        className="journal-audio-stage"
        data-journal-scroll-key="media-audio-stage"
      >
        <span className="journal-audio-cover" aria-hidden="true">
          <IconCozMusic />
        </span>
        <div className="journal-audio-player">
          <h2>{artifact.title}</h2>
          <audio
            controls
            preload="metadata"
            ref={playerRef}
            referrerPolicy="no-referrer"
            src={url}
          />
        </div>
      </div>
    </div>
  );
};

const JournalVideoPreview = ({
  artifact,
  url,
  onDownload,
}: {
  artifact: WorkbenchArtifact;
  url: string;
  onDownload?: (artifact: WorkbenchArtifact) => void | Promise<void>;
}) => {
  const playerRef = useRef<HTMLVideoElement>(null);
  useEffect(
    () => () => {
      playerRef.current?.pause();
      playerRef.current?.removeAttribute('src');
      playerRef.current?.load();
    },
    [url],
  );
  return (
    <div className="journal-media-preview">
      <MediaToolbar artifact={artifact} kind="video" onDownload={onDownload} />
      <div
        className="journal-video-stage"
        data-journal-scroll-key="media-video-stage"
      >
        <video
          controls
          preload="metadata"
          ref={playerRef}
          referrerPolicy="no-referrer"
          src={url}
        />
      </div>
    </div>
  );
};

const MediaAssetPreview = ({
  artifact,
  state,
  onDownload,
  onRefresh,
}: {
  artifact?: WorkbenchArtifact;
  state: JournalMediaPreviewState;
  onDownload?: (artifact: WorkbenchArtifact) => void | Promise<void>;
  onRefresh: () => void;
}) => {
  const kind = artifact ? journalMediaKind(artifact) : undefined;
  if (!artifact || !kind || state.status !== 'ready' || !state.url) {
    return (
      <MediaState
        artifact={artifact}
        state={state}
        onDownload={onDownload}
        onRefresh={onRefresh}
      />
    );
  }
  switch (kind) {
    case 'audio':
      return (
        <JournalAudioPreview
          artifact={artifact}
          url={state.url}
          onDownload={onDownload}
        />
      );
    case 'video':
      return (
        <JournalVideoPreview
          artifact={artifact}
          url={state.url}
          onDownload={onDownload}
        />
      );
    case 'image':
    default:
      return (
        <JournalImagePreview
          artifact={artifact}
          url={state.url}
          onDownload={onDownload}
        />
      );
  }
};

export const JournalMediaView = ({
  context,
  spaceId,
  threadId,
  onDownload,
}: {
  context: JournalMediaContext;
  spaceId: string;
  threadId: string;
  onDownload?: (artifact: WorkbenchArtifact) => void | Promise<void>;
}) => {
  const [selection, setSelection] = useState({ artifactID: '', index: 0 });
  const selectedIndex =
    selection.artifactID === context.artifact.artifact_id
      ? selection.index
      : 0;
  const rootReady =
    !context.artifact.generation_status ||
    context.artifact.generation_status === 'ready';
  const selectedArtifact =
    context.kind === 'media_collection' && rootReady
      ? context.members[selectedIndex]
      : context.artifact;
  const { refresh, state } = useJournalMediaPreview({
    artifact: selectedArtifact,
    spaceId,
    threadId,
  });

  if (context.kind !== 'media_collection') {
    return (
      <MediaAssetPreview
        artifact={selectedArtifact}
        state={state}
        onDownload={onDownload}
        onRefresh={refresh}
      />
    );
  }

  return (
    <div className="journal-media-preview">
      <MediaToolbar
        artifact={context.artifact}
        kind="media_collection"
        onDownload={onDownload}
      />
      <div className="journal-media-collection">
        <aside
          className="journal-media-collection-list"
          data-journal-scroll-key="media-collection-list"
        >
          {context.members.map((artifact, index) => {
            const kind = journalMediaKind(artifact) ?? 'image';
            return (
              <button
                type="button"
                aria-current={index === selectedIndex}
                data-active={index === selectedIndex}
                key={artifact.artifact_id}
                onClick={() =>
                  setSelection({
                    artifactID: context.artifact.artifact_id,
                    index,
                  })
                }
              >
                <span aria-hidden="true">{mediaIcon(kind)}</span>
                <span>
                  <strong>{artifact.title}</strong>
                  <small>{artifact.content_type}</small>
                </span>
              </button>
            );
          })}
        </aside>
        <section className="journal-media-collection-preview">
          <MediaAssetPreview
            artifact={selectedArtifact}
            state={state}
            onDownload={onDownload}
            onRefresh={refresh}
          />
        </section>
      </div>
    </div>
  );
};
