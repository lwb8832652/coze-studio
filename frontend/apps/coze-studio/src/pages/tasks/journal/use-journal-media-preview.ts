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

import { useCallback, useEffect, useRef, useState } from 'react';

import { artifactSupportsCapability } from '../task-artifacts-helpers';
import { WorkbenchClientError } from '../../workbench/thread-client/canonical-fetch';
import { canonicalThreadClient } from '../../workbench/thread-client/canonical-thread-client-singleton';
import type { WorkbenchArtifact } from '../../workbench/thread-client';
import { journalMediaKind } from './journal-media-model';

export type JournalMediaPreviewStatus =
  | 'empty'
  | 'loading'
  | 'ready'
  | 'processing'
  | 'expired'
  | 'no_permission'
  | 'blocked'
  | 'download_only'
  | 'error';

export interface JournalMediaPreviewState {
  status: JournalMediaPreviewStatus;
  url?: string;
}

const initialState: JournalMediaPreviewState = { status: 'empty' };
const signedURLRefreshLeadMilliseconds = 5_000;
const minimumSignedURLRefreshMilliseconds = 1_000;
const millisecondsPerSecond = 1_000;

const signedURLFailureState = (
  error: unknown,
): JournalMediaPreviewStatus => {
  if (error instanceof WorkbenchClientError) {
    const code = error.code.toLowerCase();
    if (
      code === 'no_permission' ||
      code === 'resource_not_found' ||
      error.status === 401 ||
      error.status === 403 ||
      error.status === 404
    ) {
      return 'no_permission';
    }
  }
  return error instanceof Error && /expired/i.test(error.message)
    ? 'expired'
    : 'error';
};

const artifactState = (
  artifact: WorkbenchArtifact,
): JournalMediaPreviewStatus | undefined => {
  switch (artifact.generation_status) {
    case 'processing':
      return 'processing';
    case 'failed':
      return 'error';
    case 'expired':
      return 'expired';
    case 'blocked':
      return 'blocked';
    default:
      break;
  }
  if (!artifactSupportsCapability(artifact, 'preview')) {
    return artifactSupportsCapability(artifact, 'download')
      ? 'download_only'
      : 'blocked';
  }
  if (journalMediaKind(artifact) === 'media_collection') {
    return 'empty';
  }
  return undefined;
};

export const useJournalMediaPreview = ({
  artifact,
  spaceId,
  threadId,
}: {
  artifact?: WorkbenchArtifact;
  spaceId: string;
  threadId: string;
}) => {
  const [state, setState] = useState<JournalMediaPreviewState>(initialState);
  const [refreshRevision, setRefreshRevision] = useState(0);
  const requestToken = useRef(0);
  const refresh = useCallback(
    () => setRefreshRevision(revision => revision + 1),
    [],
  );

  useEffect(() => {
    const token = ++requestToken.current;
    const abortController = new AbortController();
    let refreshTimer: ReturnType<typeof setTimeout> | undefined;
    if (!artifact || !spaceId || !threadId) {
      setState(initialState);
      return () => abortController.abort();
    }
    const immediateState = artifactState(artifact);
    if (immediateState) {
      setState({ status: immediateState });
      return () => abortController.abort();
    }

    setState({ status: 'loading' });
    void canonicalThreadClient
      .getArtifactSignedURL({
        artifact_id: artifact.artifact_id,
        mode: 'preview',
        signal: abortController.signal,
        space_id: spaceId,
        thread_id: threadId,
      })
      .then(result => {
        if (token !== requestToken.current || abortController.signal.aborted) {
          return;
        }
        setState({ status: 'ready', url: result.url });
        const refreshDelay = Math.max(
          minimumSignedURLRefreshMilliseconds,
          result.expires_in_seconds * millisecondsPerSecond -
            signedURLRefreshLeadMilliseconds,
        );
        refreshTimer = setTimeout(refresh, refreshDelay);
      })
      .catch(error => {
        if (token !== requestToken.current || abortController.signal.aborted) {
          return;
        }
        setState({
          status: signedURLFailureState(error),
        });
      });

    return () => {
      abortController.abort();
      if (refreshTimer) {
        clearTimeout(refreshTimer);
      }
    };
  }, [artifact, refresh, refreshRevision, spaceId, threadId]);

  return { refresh, state };
};
