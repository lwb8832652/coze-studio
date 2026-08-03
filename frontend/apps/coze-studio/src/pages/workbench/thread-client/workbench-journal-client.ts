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
  AuditWorkbenchJournalSnapshotActionRequest,
  GetWorkbenchJournalSettingsRequest,
  GetWorkbenchJournalSnapshotRequest,
  GetWorkbenchRunJournalRequest,
  JournalEventSubscription,
  ListWorkbenchJournalEventsRequest,
  PatchWorkbenchJournalSettingsRequest,
  RecoverWorkbenchJournalRequest,
  SubscribeWorkbenchJournalEventsRequest,
} from './workbench-thread-client';
import type {
  WorkbenchJournalBootstrap,
  WorkbenchJournalEventPage,
  WorkbenchJournalRecoveryResult,
  WorkbenchJournalSettings,
  WorkbenchJournalSnapshot,
  WorkbenchJournalSnapshotActionResult,
} from './types';

export interface WorkbenchJournalClient {
  getRunJournal: (
    request: GetWorkbenchRunJournalRequest,
  ) => Promise<WorkbenchJournalBootstrap>;
  listJournalEvents: (
    request: ListWorkbenchJournalEventsRequest,
  ) => Promise<WorkbenchJournalEventPage>;
  subscribeJournalEvents: (
    request: SubscribeWorkbenchJournalEventsRequest,
  ) => JournalEventSubscription;
  getJournalSnapshot: (
    request: GetWorkbenchJournalSnapshotRequest,
  ) => Promise<WorkbenchJournalSnapshot>;
  auditJournalSnapshotAction: (
    request: AuditWorkbenchJournalSnapshotActionRequest,
  ) => Promise<WorkbenchJournalSnapshotActionResult>;
  recoverJournal: (
    request: RecoverWorkbenchJournalRequest,
  ) => Promise<WorkbenchJournalRecoveryResult>;
  getJournalSettings: (
    request: GetWorkbenchJournalSettingsRequest,
  ) => Promise<WorkbenchJournalSettings>;
  patchJournalSettings: (
    request: PatchWorkbenchJournalSettingsRequest,
  ) => Promise<WorkbenchJournalSettings>;
}
