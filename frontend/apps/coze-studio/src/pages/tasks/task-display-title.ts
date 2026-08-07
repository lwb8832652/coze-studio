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

const TITLE_MAX_LENGTH = 48;

const compactTitle = (value: string) =>
  value.replace(/\s+/g, ' ').trim().slice(0, TITLE_MAX_LENGTH);

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
