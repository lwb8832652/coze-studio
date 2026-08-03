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
  WorkbenchJournalContentStatus,
  WorkbenchJournalSnapshot,
  WorkbenchJournalSnapshotAction,
} from '../../../workbench/thread-client';

export type JournalSnapshotActionHandler = (
  action: WorkbenchJournalSnapshotAction,
  fragmentID?: string,
) => Promise<void>;

export interface JournalSnapshotViewProps {
  onNextBrowserSnapshot?: () => void;
  onPreviousBrowserSnapshot?: () => void;
  snapshot?: WorkbenchJournalSnapshot;
  status: WorkbenchJournalContentStatus;
  onSnapshotAction?: JournalSnapshotActionHandler;
}
