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

import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
  type UIEvent,
} from 'react';

import {
  IconCozCode,
  IconCozCross,
  IconCozDocument,
  IconCozEarth,
  IconCozExpand,
  IconCozImage,
  IconCozLightbulb,
  IconCozMinimize,
  IconCozMusic,
  IconCozShell,
  IconCozSideExpand,
  IconCozVideo,
} from '@coze-arch/coze-design/icons';

import type {
  WorkbenchArtifact,
  WorkbenchJournalAttempt,
  WorkbenchJournalContentStatus,
  WorkbenchJournalContentType,
  WorkbenchJournalEvent,
  WorkbenchJournalSnapshot,
} from '../../workbench/thread-client';
import {
  JournalBrowserView,
  JournalCodeView,
  JournalDocumentView,
  JournalMediaView,
  JournalSkillView,
  JournalTerminalView,
  type JournalSnapshotActionHandler,
} from './views';
import { JournalTimeline } from './journal-timeline';
import type {
  JournalTransportStatus,
  JournalViewMode,
} from './journal-reducer';
import type { JournalScrollPosition } from './use-journal-experience';
import { journalMediaContextForEvent } from './journal-media-model';
import {
  findJournalEvent,
  journalContentTypeForEvent,
  journalEventLabel,
} from './journal-event-model';

import './journal.less';

const baseTabs: Array<{
  icon: typeof IconCozDocument;
  label: string;
  value: WorkbenchJournalContentType;
}> = [
  { icon: IconCozDocument, label: '文档', value: 'document' },
  { icon: IconCozShell, label: '终端', value: 'terminal' },
  { icon: IconCozCode, label: '代码', value: 'code' },
  { icon: IconCozLightbulb, label: '技能', value: 'skill' },
  { icon: IconCozEarth, label: '浏览器', value: 'browser' },
];

const mediaTabIcon = (previewMode: string) => {
  switch (previewMode) {
    case 'audio':
      return IconCozMusic;
    case 'video':
      return IconCozVideo;
    case 'image':
    case 'media_collection':
    default:
      return IconCozImage;
  }
};

const JournalSnapshotView = ({
  activeTab,
  contentStatus,
  onNextBrowserSnapshot,
  onPreviousBrowserSnapshot,
  snapshot,
  onSnapshotAction,
}: {
  activeTab: WorkbenchJournalContentType;
  contentStatus: WorkbenchJournalContentStatus;
  onNextBrowserSnapshot?: () => void;
  onPreviousBrowserSnapshot?: () => void;
  snapshot?: WorkbenchJournalSnapshot;
  onSnapshotAction?: JournalSnapshotActionHandler;
}) => {
  const props = {
    snapshot,
    status: contentStatus,
    onSnapshotAction,
  };
  switch (activeTab) {
    case 'terminal':
      return <JournalTerminalView {...props} />;
    case 'code':
      return <JournalCodeView {...props} />;
    case 'skill':
      return <JournalSkillView {...props} />;
    case 'browser':
      return (
        <JournalBrowserView
          {...props}
          onNextBrowserSnapshot={onNextBrowserSnapshot}
          onPreviousBrowserSnapshot={onPreviousBrowserSnapshot}
        />
      );
    case 'document':
    default:
      return <JournalDocumentView {...props} />;
  }
};

export const JournalRestoreButton = ({
  buttonRef,
  onRestore,
}: {
  buttonRef?: RefObject<HTMLButtonElement>;
  onRestore: () => void;
}) => (
  <button
    type="button"
    aria-label="重新打开执行详情"
    className="journal-restore-button"
    ref={buttonRef}
    title="重新打开执行详情"
    onClick={onRestore}
  >
    <IconCozSideExpand />
  </button>
);

// eslint-disable-next-line complexity, @coze-arch/max-line-per-function -- Keep accepted panel state together.
export const JournalPanel = ({
  activeTab,
  artifacts = [],
  attempts = [],
  closeButtonRef,
  contentStatus,
  events,
  getScrollPosition,
  maximized = false,
  selectedEventId,
  selectedAttemptId,
  snapshot,
  spaceId = '',
  threadId = '',
  transportStatus,
  viewMode,
  onActiveTabChange,
  onArtifactDownload,
  onClose,
  onScrollPositionChange,
  onSelectEvent,
  onSelectAttempt,
  onSnapshotAction,
  onToggleMaximize,
  onViewModeChange,
}: {
  activeTab: WorkbenchJournalContentType;
  artifacts?: WorkbenchArtifact[];
  attempts?: WorkbenchJournalAttempt[];
  closeButtonRef?: RefObject<HTMLButtonElement>;
  contentStatus?: WorkbenchJournalContentStatus;
  events: WorkbenchJournalEvent[];
  getScrollPosition?: (key: string) => JournalScrollPosition | undefined;
  maximized?: boolean;
  selectedEventId?: string;
  selectedAttemptId?: string;
  snapshot?: WorkbenchJournalSnapshot;
  spaceId?: string;
  threadId?: string;
  transportStatus: JournalTransportStatus;
  viewMode: JournalViewMode;
  onActiveTabChange: (tab: WorkbenchJournalContentType) => void;
  onArtifactDownload?: (artifact: WorkbenchArtifact) => void | Promise<void>;
  onClose: () => void;
  onScrollPositionChange?: (
    key: string,
    position: JournalScrollPosition,
  ) => void;
  onSelectEvent: (event: WorkbenchJournalEvent) => void;
  onSelectAttempt?: (attemptID: string) => void;
  onSnapshotAction?: JournalSnapshotActionHandler;
  onToggleMaximize?: () => void;
  onViewModeChange: (mode: JournalViewMode) => void;
}) => {
  const tabRefs = useRef<Array<HTMLButtonElement | null>>([]);
  const contentRef = useRef<HTMLDivElement>(null);
  const [suppressedMediaEventID, setSuppressedMediaEventID] = useState('');
  const selectedEvent = useMemo(
    () => findJournalEvent(events, selectedEventId),
    [events, selectedEventId],
  );
  const selectedLabel = selectedEvent
    ? journalEventLabel(selectedEvent)
    : '执行详情';
  const resolvedContentStatus = contentStatus ?? snapshot?.status ?? 'empty';
  const mediaContext = useMemo(
    () => journalMediaContextForEvent(selectedEvent, artifacts),
    [artifacts, selectedEvent],
  );
  const mediaTabActive = Boolean(
    mediaContext && suppressedMediaEventID !== selectedEventId,
  );
  const browserEvents = useMemo(
    () =>
      events.filter(
        event =>
          Boolean(event.snapshot_id) &&
          journalContentTypeForEvent(event) === 'browser',
      ),
    [events],
  );
  const browserEventIndex = browserEvents.findIndex(
    event => event.event_id === selectedEventId,
  );
  const selectBrowserEvent = (index: number) => {
    const event = browserEvents[index];
    if (event) {
      onSelectEvent(event);
    }
  };
  const tabCount = baseTabs.length + (mediaContext ? 1 : 0);
  const scrollScope = [
    selectedAttemptId || 'legacy',
    selectedEventId || 'none',
    mediaTabActive
      ? `media:${mediaContext?.artifact.artifact_id ?? 'none'}`
      : activeTab,
  ].join(':');
  const scrollKey = (localKey: string) => `${scrollScope}:${localKey}`;
  useLayoutEffect(() => {
    const content = contentRef.current;
    if (!content || !getScrollPosition) {
      return;
    }
    const restore = () => {
      content
        .querySelectorAll<HTMLElement>('[data-journal-scroll-key]')
        .forEach(element => {
          const localKey = element.dataset.journalScrollKey;
          const position = localKey
            ? getScrollPosition(scrollKey(localKey))
            : undefined;
          if (position) {
            element.scrollLeft = position.left;
            element.scrollTop = position.top;
          }
        });
    };
    restore();
    const observer = new MutationObserver(restore);
    observer.observe(content, { childList: true, subtree: true });
    return () => observer.disconnect();
  }, [getScrollPosition, scrollScope]);
  const handleScrollCapture = (event: UIEvent<HTMLDivElement>) => {
    if (!onScrollPositionChange || !(event.target instanceof HTMLElement)) {
      return;
    }
    const localKey = event.target.dataset.journalScrollKey;
    if (!localKey) {
      return;
    }
    onScrollPositionChange(scrollKey(localKey), {
      left: event.target.scrollLeft,
      top: event.target.scrollTop,
    });
  };
  const handleTabKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) => {
    if (event.key !== 'ArrowRight' && event.key !== 'ArrowLeft') {
      return;
    }
    event.preventDefault();
    const direction = event.key === 'ArrowRight' ? 1 : -1;
    const nextIndex = (index + direction + tabCount) % tabCount;
    if (nextIndex === baseTabs.length && mediaContext) {
      setSuppressedMediaEventID('');
    } else {
      setSuppressedMediaEventID(selectedEventId ?? '');
      onActiveTabChange(baseTabs[nextIndex].value);
    }
    tabRefs.current[nextIndex]?.focus();
  };

  return (
    <aside
      aria-label="执行详情"
      className="journal-panel"
      data-maximized={maximized}
      data-testid="journal-panel"
    >
      <header className="journal-panel-tabs">
        <button
          type="button"
          aria-pressed={viewMode === 'live_follow'}
          className="journal-follow-button"
          data-active={viewMode === 'live_follow'}
          onClick={() => onViewModeChange('live_follow')}
        >
          <span aria-hidden="true" />
          Follow
        </button>
        <div className="journal-base-tabs" role="tablist" aria-label="执行内容">
          {baseTabs.map((tab, index) => {
            const TabIcon = tab.icon;

            return (
              <button
                type="button"
                aria-label={tab.label}
                aria-selected={!mediaTabActive && activeTab === tab.value}
                className="journal-view-tab"
                data-active={!mediaTabActive && activeTab === tab.value}
                key={tab.value}
                ref={element => {
                  tabRefs.current[index] = element;
                }}
                role="tab"
                tabIndex={!mediaTabActive && activeTab === tab.value ? 0 : -1}
                title={tab.label}
                onClick={() => {
                  setSuppressedMediaEventID(selectedEventId ?? '');
                  onActiveTabChange(tab.value);
                }}
                onKeyDown={event => handleTabKeyDown(event, index)}
              >
                <TabIcon />
                <span>{tab.label}</span>
              </button>
            );
          })}
          {mediaContext
            ? (() => {
                const MediaIcon = mediaTabIcon(mediaContext.kind);
                return (
                  <button
                    type="button"
                    aria-label={mediaContext.label}
                    aria-selected={mediaTabActive}
                    className="journal-view-tab journal-context-media-tab"
                    data-active={mediaTabActive}
                    ref={element => {
                      tabRefs.current[baseTabs.length] = element;
                    }}
                    role="tab"
                    tabIndex={mediaTabActive ? 0 : -1}
                    title={mediaContext.label}
                    onClick={() => setSuppressedMediaEventID('')}
                    onKeyDown={event =>
                      handleTabKeyDown(event, baseTabs.length)
                    }
                  >
                    <MediaIcon />
                    <span>{mediaContext.label}</span>
                  </button>
                );
              })()
            : null}
        </div>
        <div className="journal-panel-actions">
          {onToggleMaximize ? (
            <button
              type="button"
              aria-label={maximized ? '还原执行详情' : '放大执行详情'}
              title={maximized ? '还原执行详情' : '放大执行详情'}
              onClick={onToggleMaximize}
            >
              {maximized ? <IconCozMinimize /> : <IconCozExpand />}
            </button>
          ) : null}
          <button
            type="button"
            aria-label="关闭执行详情"
            ref={closeButtonRef}
            title="关闭执行详情"
            onClick={onClose}
          >
            <IconCozCross />
          </button>
        </div>
      </header>
      <div className="journal-panel-context">
        <span aria-hidden="true" />
        <strong>{selectedLabel}</strong>
        <span>{snapshot ? '执行快照' : ''}</span>
        <em data-status={transportStatus}>
          {viewMode === 'historical' ? '历史' : '实时'}
        </em>
      </div>
      <div
        className="journal-panel-content"
        ref={contentRef}
        onScrollCapture={handleScrollCapture}
      >
        {mediaTabActive && mediaContext ? (
          <JournalMediaView
            context={mediaContext}
            spaceId={spaceId}
            threadId={threadId}
            onDownload={onArtifactDownload}
          />
        ) : (
          <JournalSnapshotView
            activeTab={activeTab}
            contentStatus={resolvedContentStatus}
            onNextBrowserSnapshot={
              browserEventIndex >= 0 &&
              browserEventIndex < browserEvents.length - 1
                ? () => selectBrowserEvent(browserEventIndex + 1)
                : undefined
            }
            onPreviousBrowserSnapshot={
              browserEventIndex > 0
                ? () => selectBrowserEvent(browserEventIndex - 1)
                : undefined
            }
            snapshot={snapshot}
            onSnapshotAction={onSnapshotAction}
          />
        )}
      </div>
      <JournalTimeline
        attempts={attempts}
        events={events}
        selectedAttemptId={selectedAttemptId}
        selectedEventId={selectedEventId}
        transportStatus={transportStatus}
        viewMode={viewMode}
        onSelectEvent={onSelectEvent}
        onSelectAttempt={onSelectAttempt}
        onViewModeChange={onViewModeChange}
      />
    </aside>
  );
};
