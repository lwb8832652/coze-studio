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

/* eslint-disable @coze-arch/max-line-per-function -- This hook owns one Journal panel lifecycle and its cancellation boundaries. */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { copyTextToClipboard } from '../task-clipboard';
import { canonicalThreadClient } from '../../workbench/thread-client/canonical-thread-client-singleton';
import { WorkbenchClientError } from '../../workbench/thread-client/canonical-fetch';
import type {
  WorkbenchJournalContentType,
  WorkbenchJournalEvent,
  WorkbenchJournalSettings,
  WorkbenchJournalSnapshot,
  WorkbenchJournalSnapshotAction,
  WorkbenchJournalSnapshotActionResult,
} from '../../workbench/thread-client';
import { createJournalStreamController } from './journal-stream';
import {
  createInitialJournalState,
  journalReducer,
  type JournalAction,
  type JournalState,
  type JournalViewMode,
} from './journal-reducer';
import type { JournalRecoveryHandler } from './journal-recovery-dialog';
import {
  buildJournalTimelineItems,
  journalContentTypeForEvent,
} from './journal-event-model';

const minimumSplitRatio = 0.4;
const maximumSplitRatio = 0.7;
const defaultSplitRatio = 0.4;
const splitSaveThreshold = 0.01;
const snapshotPageSize = 100;

const clampSplitRatio = (ratio: number): number =>
  Math.min(maximumSplitRatio, Math.max(minimumSplitRatio, ratio));

const idempotencyKey = (prefix: string): string => {
  const random = globalThis.crypto?.randomUUID?.();
  return `${prefix}:${random ?? `${Date.now()}:${Math.random()}`}`;
};

const isNoPermission = (error: unknown): boolean =>
  error instanceof WorkbenchClientError && error.code === 'NO_PERMISSION';

const isSettingsConflict = (error: unknown): boolean =>
  error instanceof WorkbenchClientError &&
  error.code === 'journal_settings_conflict';

const mergeSnapshotPage = (
  current: WorkbenchJournalSnapshot,
  page: WorkbenchJournalSnapshot,
): WorkbenchJournalSnapshot => ({
  ...current,
  ...page,
  content: current.content ?? page.content,
  fragments: [...current.fragments, ...page.fragments],
});

const downloadBase64 = ({
  content,
  mimeType,
  name,
}: {
  content: string;
  mimeType: string;
  name: string;
}) => {
  const binary = atob(content);
  const bytes = Uint8Array.from(binary, character => character.charCodeAt(0));
  const url = URL.createObjectURL(new Blob([bytes], { type: mimeType }));
  const link = document.createElement('a');
  link.download = name;
  link.href = url;
  link.referrerPolicy = 'no-referrer';
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
};

const performAuthorizedBrowserAction = async (
  result: WorkbenchJournalSnapshotActionResult,
): Promise<void> => {
  if (!result.allowed) {
    throw new Error('snapshot_action_not_allowed');
  }
  if (
    ['copy_command', 'copy_output', 'copy_code'].includes(result.action) &&
    result.copy_text
  ) {
    const copied = await copyTextToClipboard(result.copy_text);
    if (!copied) {
      throw new Error('snapshot_copy_failed');
    }
    return;
  }
  if (result.action === 'open_original' && result.download_url) {
    const opened = window.open(
      result.download_url,
      '_blank',
      'noopener,noreferrer',
    );
    if (opened) {
      opened.opener = null;
    }
    return;
  }
  if (result.action === 'download_fragment') {
    if (result.download_url) {
      const link = document.createElement('a');
      link.href = result.download_url;
      link.referrerPolicy = 'no-referrer';
      document.body.appendChild(link);
      link.click();
      link.remove();
      return;
    }
    if (result.download_content_base64) {
      downloadBase64({
        content: result.download_content_base64,
        mimeType: result.download_mime_type || 'application/octet-stream',
        name: `${result.snapshot_id}.bin`,
      });
      return;
    }
  }
  throw new Error('snapshot_action_payload_missing');
};

export interface JournalExperience {
  activeTab: WorkbenchJournalContentType;
  closePanel: () => void;
  commitSplitRatio: (ratio: number) => Promise<void>;
  getScrollPosition: (key: string) => JournalScrollPosition | undefined;
  layoutReady: boolean;
  maximized: boolean;
  openPanel: () => void;
  panelOpen: boolean;
  performSnapshotAction: (
    action: WorkbenchJournalSnapshotAction,
    fragmentID?: string,
  ) => Promise<void>;
  recover: JournalRecoveryHandler;
  rememberScrollPosition: (key: string, position: JournalScrollPosition) => void;
  selectAttempt: (attemptID: string) => void;
  selectEvent: (event: WorkbenchJournalEvent) => void;
  selectedEventId?: string;
  setActiveTab: (tab: WorkbenchJournalContentType) => void;
  setMaximized: (maximized: boolean) => void;
  setSplitRatio: (ratio: number) => void;
  setViewMode: (mode: JournalViewMode) => void;
  splitRatio: number;
  state: JournalState;
}

export interface JournalScrollPosition {
  left: number;
  top: number;
}

export const useJournalExperience = ({
  enabled,
  runId,
  spaceId,
  threadId,
}: {
  enabled: boolean;
  runId?: string;
  spaceId?: string;
  threadId?: string;
}): JournalExperience => {
  const initialState = useMemo(() => createInitialJournalState(), []);
  const stateRef = useRef(initialState);
  const [state, setState] = useState(initialState);
  const [panelOpen, setPanelOpen] = useState(true);
  const [maximized, setMaximized] = useState(false);
  const [selectedEventId, setSelectedEventId] = useState<string>();
  const [activeTab, setActiveTab] =
    useState<WorkbenchJournalContentType>('document');
  const [splitRatio, setSplitRatioState] = useState(defaultSplitRatio);
  const [layoutReady, setLayoutReady] = useState(false);
  const [streamRevision, setStreamRevision] = useState(0);
  const [attemptOverrideID, setAttemptOverrideID] = useState<string>();
  const settingsRef = useRef<WorkbenchJournalSettings>({
    split_ratio: defaultSplitRatio,
    revision: 'missing',
  });
  const snapshotRequestRef = useRef<{
    controller?: AbortController;
    token: number;
  }>({ token: 0 });
  const scrollPositionsRef = useRef(new Map<string, JournalScrollPosition>());
  const scopeReady = Boolean(enabled && spaceId && threadId && runId);
  const scope = useMemo(
    () =>
      scopeReady
        ? {
            space_id: spaceId as string,
            thread_id: threadId as string,
            run_id: runId as string,
          }
        : undefined,
    [runId, scopeReady, spaceId, threadId],
  );
  const reduce = useCallback((action: JournalAction): JournalState => {
    const next = journalReducer(stateRef.current, action);
    stateRef.current = next;
    setState(next);
    return next;
  }, []);

  useEffect(() => {
    if (!scope) {
      return;
    }
    stateRef.current = createInitialJournalState();
    setState(stateRef.current);
    setPanelOpen(true);
    setMaximized(false);
    setSelectedEventId(undefined);
    setActiveTab('document');
    setAttemptOverrideID(undefined);
    scrollPositionsRef.current.clear();
  }, [scope]);

  useEffect(() => {
    if (!scope) {
      return;
    }
    setSelectedEventId(undefined);
    const controller = createJournalStreamController({
      client: canonicalThreadClient,
      scope,
      attemptId: attemptOverrideID,
      reduce,
    });
    void controller.start();
    return () => controller.stop();
  }, [attemptOverrideID, reduce, scope, streamRevision]);

  useEffect(() => {
    if (!scope) {
      setLayoutReady(current => (current ? false : current));
      return;
    }
    const controller = new AbortController();
    setLayoutReady(false);
    void canonicalThreadClient
      .getJournalSettings({ signal: controller.signal })
      .then(settings => {
        settingsRef.current = settings;
        setSplitRatioState(clampSplitRatio(settings.split_ratio));
      })
      .catch(() => {
        settingsRef.current = {
          split_ratio: defaultSplitRatio,
          revision: 'missing',
        };
        setSplitRatioState(defaultSplitRatio);
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLayoutReady(true);
        }
      });
    return () => controller.abort();
  }, [scope]);

  const timelineItems = useMemo(
    () => buildJournalTimelineItems(state.execution.events),
    [state.execution.events],
  );

  useEffect(() => {
    if (!timelineItems.length) {
      setSelectedEventId(undefined);
      return;
    }
    const selectionExists = timelineItems.some(
      item => item.event.event_id === selectedEventId,
    );
    if (!selectionExists || state.view_mode === 'live_follow') {
      const latest = timelineItems[timelineItems.length - 1].event;
      setSelectedEventId(latest.event_id);
      const contentType = journalContentTypeForEvent(latest);
      if (contentType) {
        setActiveTab(contentType);
      }
    }
  }, [selectedEventId, state.view_mode, timelineItems]);

  const selectedEvent = useMemo(
    () =>
      timelineItems.find(item => item.event.event_id === selectedEventId)
        ?.event,
    [selectedEventId, timelineItems],
  );

  useEffect(() => {
    snapshotRequestRef.current.controller?.abort();
    const token = snapshotRequestRef.current.token + 1;
    const controller = new AbortController();
    snapshotRequestRef.current = { controller, token };
    if (!panelOpen || !scope || !selectedEvent?.snapshot_id) {
      reduce({ type: 'snapshot_cleared' });
      return () => controller.abort();
    }
    const snapshotID = selectedEvent.snapshot_id;
    reduce({ type: 'snapshot_loading', snapshot_id: snapshotID });
    void (async () => {
      try {
        let snapshot = await canonicalThreadClient.getJournalSnapshot({
          ...scope,
          snapshot_id: snapshotID,
          limit: snapshotPageSize,
          signal: controller.signal,
        });
        while (snapshot.has_more && snapshot.next_cursor) {
          const page = await canonicalThreadClient.getJournalSnapshot({
            ...scope,
            snapshot_id: snapshotID,
            cursor: snapshot.next_cursor,
            limit: snapshotPageSize,
            signal: controller.signal,
          });
          snapshot = mergeSnapshotPage(snapshot, page);
        }
        if (
          controller.signal.aborted ||
          snapshotRequestRef.current.token !== token
        ) {
          return;
        }
        reduce({ type: 'snapshot_loaded', snapshot });
      } catch (error) {
        if (
          controller.signal.aborted ||
          snapshotRequestRef.current.token !== token
        ) {
          return;
        }
        reduce({
          type: 'snapshot_failed',
          error_code:
            error instanceof WorkbenchClientError
              ? error.code
              : 'SNAPSHOT_UNAVAILABLE',
          no_permission: isNoPermission(error),
        });
      }
    })();
    return () => controller.abort();
  }, [panelOpen, reduce, scope, selectedEvent]);

  const selectEvent = useCallback(
    (event: WorkbenchJournalEvent) => {
      setSelectedEventId(event.event_id);
      const contentType = journalContentTypeForEvent(event);
      if (contentType) {
        setActiveTab(contentType);
      }
      const latest = timelineItems[timelineItems.length - 1]?.event.event_id;
      reduce({
        type: 'view_mode_changed',
        mode:
          event.event_id === latest
            ? 'live_follow'
            : state.execution.status === 'running'
              ? 'live_paused'
              : 'historical',
      });
    },
    [reduce, state.execution.status, timelineItems],
  );

  const commitSplitRatio = useCallback(async (ratio: number) => {
    const next = clampSplitRatio(ratio);
    setSplitRatioState(next);
    if (
      Math.abs(next - settingsRef.current.split_ratio) <= splitSaveThreshold
    ) {
      return;
    }
    const save = async (revision: string) =>
      canonicalThreadClient.patchJournalSettings({
        split_ratio: next,
        revision,
      });
    try {
      settingsRef.current = await save(settingsRef.current.revision);
    } catch (error) {
      if (!isSettingsConflict(error)) {
        return;
      }
      try {
        const current = await canonicalThreadClient.getJournalSettings({});
        settingsRef.current = await save(current.revision);
      } catch (retryError) {
        void retryError;
      }
    }
  }, []);

  const performSnapshotAction = useCallback(
    async (action: WorkbenchJournalSnapshotAction, fragmentID?: string) => {
      const { snapshot } = stateRef.current.content;
      if (!scope || !panelOpen || !snapshot) {
        throw new Error('snapshot_action_without_current_snapshot');
      }
      const result = await canonicalThreadClient.auditJournalSnapshotAction({
        ...scope,
        snapshot_id: snapshot.snapshot_id,
        action,
        ...(fragmentID ? { fragment_id: fragmentID } : {}),
        idempotency_key: idempotencyKey(`journal:${action}`),
      });
      await performAuthorizedBrowserAction(result);
    },
    [panelOpen, scope],
  );

  const recover = useCallback<JournalRecoveryHandler>(
    async (action, confirmed) => {
      if (!scope) {
        throw new Error('journal_recovery_without_scope');
      }
      const result = await canonicalThreadClient.recoverJournal({
        ...scope,
        action,
        confirmed,
        ...(stateRef.current.execution.selected_attempt_id
          ? {
              source_attempt_id: stateRef.current.execution.selected_attempt_id,
            }
          : {}),
        idempotency_key: idempotencyKey(`journal:recover:${action}`),
      });
      if (!result.accepted) {
        throw new Error('journal_recovery_not_accepted');
      }
      setAttemptOverrideID(undefined);
      setStreamRevision(revision => revision + 1);
    },
    [scope],
  );

  const selectAttempt = useCallback((attemptID: string) => {
    const attempts = stateRef.current.execution.attempts;
    const preferred =
      attempts.find(attempt =>
        ['pending', 'running'].includes(attempt.status),
      ) ??
      [...attempts].sort(
        (left, right) => right.created_at - left.created_at,
      )[0];
    setAttemptOverrideID(
      attemptID === preferred?.attempt_id ? undefined : attemptID,
    );
  }, []);

  return {
    activeTab,
    closePanel: () => {
      snapshotRequestRef.current.controller?.abort();
      snapshotRequestRef.current.token += 1;
      reduce({ type: 'snapshot_cleared' });
      setMaximized(false);
      setPanelOpen(false);
    },
    commitSplitRatio,
    getScrollPosition: key => scrollPositionsRef.current.get(key),
    layoutReady,
    maximized,
    openPanel: () => setPanelOpen(true),
    panelOpen,
    performSnapshotAction,
    recover,
    rememberScrollPosition: (key, position) =>
      scrollPositionsRef.current.set(key, position),
    selectAttempt,
    selectEvent,
    selectedEventId,
    setActiveTab,
    setMaximized,
    setSplitRatio: ratio => setSplitRatioState(clampSplitRatio(ratio)),
    setViewMode: mode => reduce({ type: 'view_mode_changed', mode }),
    splitRatio,
    state,
  };
};
