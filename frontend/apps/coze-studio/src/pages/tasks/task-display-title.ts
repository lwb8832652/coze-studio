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

import type { WorkbenchArtifact } from '../workbench/thread-client';
import type { TaskThreadDetailModel } from './task-thread-detail-model';
import { artifactFileName } from './task-artifacts-helpers';

type TaskThreadArtifact = WorkbenchArtifact;

const TITLE_MAX_LENGTH = 48;
const KNOWN_FILE_EXTENSION_RE =
  /\.(?:csv|html?|json|md|markdown|pdf|txt|xlsx?)$/i;

const compactTitle = (value: string) =>
  value.replace(/\s+/g, ' ').trim().slice(0, TITLE_MAX_LENGTH);

const stripKnownFileExtension = (value: string) =>
  compactTitle(value.replace(KNOWN_FILE_EXTENSION_RE, ''));

const extractPromptSubject = (value: string) => {
  const bookTitle = /《([^》]{2,64})》/.exec(value);
  if (bookTitle?.[1]) {
    return compactTitle(bookTitle[1]);
  }

  return '';
};

interface TaskTitleSource {
  title?: string;
  last_user_message?: string;
  last_agent_message?: string;
}

export const getTaskThreadDisplayTitle = (task: TaskTitleSource) => {
  const candidates = [
    task.title,
    task.last_agent_message,
    task.last_user_message,
  ].filter((value): value is string => Boolean(value?.trim()));

  for (const candidate of candidates) {
    const promptSubject = extractPromptSubject(candidate);
    if (promptSubject) {
      return promptSubject;
    }
  }

  const compactCandidate = candidates.map(compactTitle).find(Boolean);

  return compactCandidate || '未命名任务';
};

export const getTaskDisplayTitle = ({
  artifacts,
  task,
}: {
  artifacts?: TaskThreadArtifact[];
  task: TaskThreadDetailModel;
}) => {
  const sortedArtifacts = [...(artifacts ?? [])].sort(
    (left, right) =>
      left.created_at - right.created_at ||
      left.artifact_id.localeCompare(right.artifact_id),
  );
  const artifactTitle = sortedArtifacts
    .map(artifact => stripKnownFileExtension(artifactFileName(artifact)))
    .find(Boolean);

  if (artifactTitle) {
    return artifactTitle;
  }

  return getTaskThreadDisplayTitle(task);
};
