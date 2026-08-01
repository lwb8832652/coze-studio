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

import { useState } from 'react';

import {
  IconCozArrowLeft,
  IconCozArrowRight,
  IconCozOriginalSize,
  IconCozScaling,
} from '@coze-arch/coze-design/icons';

import { JournalViewState } from './view-state';
import type { JournalSnapshotViewProps } from './types';

const browserImage = ({ snapshot }: JournalSnapshotViewProps): string => {
  const browser = snapshot?.content?.browser;
  if (browser?.static_snapshot_base64) {
    return `data:${browser.mime_type};base64,${browser.static_snapshot_base64}`;
  }
  const fragment = snapshot?.fragments.find(
    item => item.kind === 'browser_snapshot' && item.binary_content_base64,
  );
  return fragment?.binary_content_base64
    ? `data:${fragment.mime_type || 'image/png'};base64,${fragment.binary_content_base64}`
    : '';
};

export const JournalBrowserView = (props: JournalSnapshotViewProps) => {
  const {
    onNextBrowserSnapshot,
    onPreviousBrowserSnapshot,
    snapshot,
    status,
  } = props;
  const [fit, setFit] = useState(true);
  const browser = snapshot?.content?.browser;
  const image = browserImage(props);
  const analysis =
    browser?.analysis ??
    snapshot?.fragments.flatMap(fragment => fragment.analysis ?? []) ??
    [];

  if (status !== 'ready' || !snapshot || !browser || !image) {
    return <JournalViewState status={status} />;
  }

  return (
    <div className="journal-browser-view">
      <aside
        className="journal-browser-captures"
        data-journal-scroll-key="browser-captures"
        aria-label="浏览记录"
      >
        <button type="button" aria-current="page">
          <img alt="" referrerPolicy="no-referrer" src={image} />
          <span>{browser.title || `页面 ${browser.index ?? 1}`}</span>
        </button>
      </aside>
      <section className="journal-browser-panel">
        <header>
          <div>
            <strong>{browser.title || '浏览器快照'}</strong>
            {browser.url ? <span>{browser.url}</span> : null}
          </div>
          <div>
            <button
              type="button"
              aria-label="上一个浏览快照"
              disabled={!onPreviousBrowserSnapshot}
              onClick={onPreviousBrowserSnapshot}
            >
              <IconCozArrowLeft />
            </button>
            <button
              type="button"
              aria-label="下一个浏览快照"
              disabled={!onNextBrowserSnapshot}
              onClick={onNextBrowserSnapshot}
            >
              <IconCozArrowRight />
            </button>
            <button
              type="button"
              aria-label={fit ? '查看实际大小' : '适应窗口'}
              aria-pressed={fit}
              onClick={() => setFit(current => !current)}
            >
              {fit ? <IconCozOriginalSize /> : <IconCozScaling />}
            </button>
          </div>
        </header>
        <div
          className="journal-browser-stage"
          data-fit={fit}
          data-journal-scroll-key="browser-stage"
        >
          <img
            alt={browser.title || '浏览器静态快照'}
            referrerPolicy="no-referrer"
            src={image}
          />
        </div>
        {analysis.length ? (
          <ul
            className="journal-browser-analysis"
            data-journal-scroll-key="browser-analysis"
          >
            {analysis.map((item, index) => (
              <li key={`${index}-${item}`}>{item}</li>
            ))}
          </ul>
        ) : null}
      </section>
    </div>
  );
};
