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

import { WorkbenchClientError } from '../../workbench/thread-client/canonical-fetch';
import type { WorkbenchJournalEvent } from '../../workbench/thread-client';

export const eventAtSequence = (
  events: WorkbenchJournalEvent[],
  sequence: number,
): WorkbenchJournalEvent | undefined =>
  events.find(event => event.sequence === sequence);

export const journalRetryDelay = (error: unknown, fallback: number): number =>
  error instanceof WorkbenchClientError &&
  error.code === 'JOURNAL_RATE_LIMITED' &&
  error.retryAfterMs !== undefined
    ? Math.max(fallback, error.retryAfterMs)
    : fallback;
