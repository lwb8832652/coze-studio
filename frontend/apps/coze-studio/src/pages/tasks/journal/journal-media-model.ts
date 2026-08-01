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

import type {
  WorkbenchArtifact,
  WorkbenchJournalEvent,
} from '../../workbench/thread-client';
import { journalEventData } from './journal-event-model';

export type JournalMediaKind = 'image' | 'audio' | 'video' | 'media_collection';

export interface JournalMediaContext {
  artifact: WorkbenchArtifact;
  kind: JournalMediaKind;
  label: string;
  members: WorkbenchArtifact[];
}

const mediaLabel: Record<JournalMediaKind, string> = {
  image: '图片',
  audio: '音频',
  video: '视频',
  media_collection: '媒体集',
};

const normalizedContentType = (artifact: WorkbenchArtifact): string =>
  artifact.content_type.split(';')[0]?.trim().toLowerCase() ?? '';

export const journalMediaKind = (
  artifact: WorkbenchArtifact,
): JournalMediaKind | undefined => {
  switch (artifact.preview_mode) {
    case 'image':
      return normalizedContentType(artifact) === 'image/svg+xml'
        ? undefined
        : 'image';
    case 'audio':
      return 'audio';
    case 'video':
      return 'video';
    case 'media_collection':
      return 'media_collection';
    default: {
      const contentType = normalizedContentType(artifact);
      if (contentType.startsWith('image/') && contentType !== 'image/svg+xml') {
        return 'image';
      }
      if (contentType.startsWith('audio/')) {
        return 'audio';
      }
      if (contentType.startsWith('video/')) {
        return 'video';
      }
      return undefined;
    }
  }
};

const collectionMembers = (
  root: WorkbenchArtifact,
  artifacts: WorkbenchArtifact[],
): WorkbenchArtifact[] => {
  if (!root.collection_id) {
    return [];
  }
  return artifacts
    .filter(
      artifact =>
        artifact.artifact_id !== root.artifact_id &&
        artifact.collection_id === root.collection_id &&
        journalMediaKind(artifact) !== undefined,
    )
    .sort(
      (left, right) =>
        (left.collection_order ?? Number.MAX_SAFE_INTEGER) -
          (right.collection_order ?? Number.MAX_SAFE_INTEGER) ||
        left.created_at - right.created_at ||
        left.artifact_id.localeCompare(right.artifact_id),
    );
};

export const journalMediaContextForEvent = (
  event: WorkbenchJournalEvent | undefined,
  artifacts: WorkbenchArtifact[],
): JournalMediaContext | undefined => {
  if (!event?.event_type.startsWith('artifact.')) {
    return undefined;
  }
  const artifactID = String(journalEventData(event).artifact_id ?? '').trim();
  const artifact = artifacts.find(item => item.artifact_id === artifactID);
  if (!artifact) {
    return undefined;
  }
  const kind = journalMediaKind(artifact);
  if (!kind) {
    return undefined;
  }
  return {
    artifact,
    kind,
    label: mediaLabel[kind],
    members:
      kind === 'media_collection' ? collectionMembers(artifact, artifacts) : [],
  };
};
