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

import { useRef, useState } from 'react';

import { IconCozArrowForward } from '@coze-arch/coze-design/icons';
import { Button } from '@coze-arch/coze-design';

import { TaskMarkdownContent } from '../../task-markdown-content';
import { JournalViewState } from './view-state';
import type { JournalSnapshotViewProps } from './types';
import { runJournalSnapshotAction } from './snapshot-action';

const documentText = ({ snapshot }: JournalSnapshotViewProps): string => {
  const direct = snapshot?.content?.document?.content;
  if (direct) {
    return direct;
  }
  return (
    snapshot?.fragments
      .filter(fragment => fragment.kind === 'document_block')
      .map(fragment => fragment.content ?? '')
      .join('') ?? ''
  );
};

export const JournalDocumentView = (props: JournalSnapshotViewProps) => {
  const { snapshot, status, onSnapshotAction } = props;
  const [opening, setOpening] = useState(false);
  const [activeChapterID, setActiveChapterID] = useState('');
  const articleRef = useRef<HTMLElement>(null);
  const content = documentText(props);
  const document = snapshot?.content?.document;
  const chapters =
    document?.chapters ??
    snapshot?.fragments.flatMap(fragment => fragment.chapters ?? []) ??
    [];

  if (status !== 'ready' || !snapshot || !document || !content) {
    return <JournalViewState status={status} />;
  }

  const selectChapter = (chapterID: string, title: string) => {
    setActiveChapterID(chapterID);
    const heading = Array.from(
      articleRef.current?.querySelectorAll('h1, h2, h3, h4, h5, h6') ?? [],
    ).find(element => element.textContent?.trim() === title);
    heading?.scrollIntoView?.({ behavior: 'smooth', block: 'start' });
  };

  return (
    <div className="journal-document-view">
      <aside
        className="journal-document-toc"
        data-journal-scroll-key="document-toc"
        aria-label="文档目录"
      >
        <strong>文档目录</strong>
        {chapters.length ? (
          chapters.map(chapter => (
            <button
              type="button"
              aria-current={
                activeChapterID === chapter.chapter_id ? 'location' : undefined
              }
              key={chapter.chapter_id}
              onClick={() =>
                selectChapter(chapter.chapter_id, chapter.title)
              }
            >
              {chapter.title}
            </button>
          ))
        ) : (
          <span>{document.title}</span>
        )}
      </aside>
      <article
        className="journal-document-page"
        data-journal-scroll-key="document-page"
        ref={articleRef}
      >
        <header>
          <div>
            <h2>{document.title}</h2>
            <span>
              {document.revision ? `版本 ${document.revision}` : '执行快照'}
            </span>
          </div>
          {document.source_artifact_id && onSnapshotAction ? (
            <Button
              aria-label={`打开原文 ${document.title}`}
              icon={<IconCozArrowForward />}
              loading={opening}
              size="small"
              theme="borderless"
              onClick={() => {
                setOpening(true);
                void runJournalSnapshotAction(
                  () => onSnapshotAction('open_original'),
                  () => setOpening(false),
                );
              }}
            />
          ) : null}
        </header>
        {document.active_block ? (
          <div className="journal-document-active-block">
            {document.active_block}
          </div>
        ) : null}
        <TaskMarkdownContent value={content} />
      </article>
    </div>
  );
};
