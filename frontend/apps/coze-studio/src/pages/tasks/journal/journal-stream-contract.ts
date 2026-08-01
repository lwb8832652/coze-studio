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
  WorkbenchJournalMetadata,
  WorkbenchThreadClient,
} from '../../workbench/thread-client';
import type { JournalAction, JournalState } from './journal-reducer';
import type { JournalCursorStore, JournalStreamScope } from './journal-cursor';

export type JournalStreamClient = Pick<
  WorkbenchThreadClient,
  'getRunJournal' | 'listJournalEvents' | 'subscribeJournalEvents'
>;

export interface JournalStreamControllerOptions {
  client: JournalStreamClient;
  scope: JournalStreamScope;
  reduce: (action: JournalAction) => JournalState;
  cursorStore?: JournalCursorStore;
  onMetadata?: (metadata: WorkbenchJournalMetadata, receivedAt: number) => void;
  now?: () => number;
}

export interface JournalStreamController {
  start: () => Promise<void>;
  stop: () => void;
  resumeStream: () => void;
}
