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
  type JOURNAL_PAYLOAD_VERSION,
  type JournalActionPayloadType,
  type JournalActionEventData,
  type JournalActionEventPayload,
  type JournalArtifactEventPayload,
  type JournalArtifactPayloadType,
  type JournalBrowserSnapshotContent,
  type JournalCodeSnapshotContent,
  type JournalConfirmationEventPayload,
  type JournalConfirmationPayloadType,
  type JournalContentStatus,
  type JournalControlStreamFrame as GeneratedJournalControlStreamFrame,
  type JournalDocumentSnapshotContent,
  type JournalEvent as GeneratedJournalEvent,
  type JournalEventStreamFrame as GeneratedJournalEventStreamFrame,
  type JournalHeartbeatStreamFrame as GeneratedJournalHeartbeatStreamFrame,
  type JournalMilestoneEventPayload,
  type JournalMilestonePayloadType,
  type JournalSkillSnapshotContent,
  type JournalSnapshotEnvelope as GeneratedJournalSnapshotEnvelope,
  type JournalSnapshotContentType,
  type JournalStreamFrameKind,
  type JournalTerminalSnapshotContent,
  type JournalVerificationEventPayload,
  type JournalVerificationPayloadType,
} from './idl/workbench/journal';

export * from './idl/workbench/journal';

export type JournalMilestonePayload = Omit<
  JournalMilestoneEventPayload,
  'type'
> & {
  type: JournalMilestonePayloadType.Milestone;
};

type JournalActionData = Omit<JournalActionEventData, 'content_type'>;

type JournalTypedActionPayload<
  TType extends JournalActionPayloadType,
  TContentType extends JournalSnapshotContentType,
> = Omit<JournalActionEventPayload, 'type' | 'data'> & {
  type: TType;
  data: JournalActionData & { content_type: TContentType };
};

export type JournalActionPayload =
  | (Omit<JournalActionEventPayload, 'type' | 'data'> & {
      type: JournalActionPayloadType.Generic;
      data: JournalActionData & { content_type?: never };
    })
  | JournalTypedActionPayload<
      JournalActionPayloadType.Document,
      JournalSnapshotContentType.Document
    >
  | JournalTypedActionPayload<
      JournalActionPayloadType.Terminal,
      JournalSnapshotContentType.Terminal
    >
  | JournalTypedActionPayload<
      JournalActionPayloadType.Code,
      JournalSnapshotContentType.Code
    >
  | JournalTypedActionPayload<
      JournalActionPayloadType.Skill,
      JournalSnapshotContentType.Skill
    >
  | JournalTypedActionPayload<
      JournalActionPayloadType.Browser,
      JournalSnapshotContentType.Browser
    >;

export type JournalArtifactPayload = Omit<
  JournalArtifactEventPayload,
  'type'
> & {
  type: JournalArtifactPayloadType.Artifact;
};

export type JournalVerificationPayload = Omit<
  JournalVerificationEventPayload,
  'type'
> & {
  type: JournalVerificationPayloadType.Verification;
};

export type JournalConfirmationPayload = Omit<
  JournalConfirmationEventPayload,
  'type'
> & {
  type: JournalConfirmationPayloadType.Confirmation;
};

export type JournalTypedEventPayload =
  | JournalMilestonePayload
  | JournalActionPayload
  | JournalArtifactPayload
  | JournalVerificationPayload
  | JournalConfirmationPayload;

type JournalEventBase = Omit<
  GeneratedJournalEvent,
  'payload' | 'payload_version'
>;

export type JournalLegacyEvent = JournalEventBase & {
  payload: unknown;
  payload_version?: never;
};

export type JournalVersionedEvent = JournalEventBase & {
  payload: JournalTypedEventPayload;
  payload_version: typeof JOURNAL_PAYLOAD_VERSION;
};

export type JournalEvent = JournalLegacyEvent | JournalVersionedEvent;

interface JournalSnapshotContentMap {
  [JournalSnapshotContentType.Document]: JournalDocumentSnapshotContent;
  [JournalSnapshotContentType.Terminal]: JournalTerminalSnapshotContent;
  [JournalSnapshotContentType.Code]: JournalCodeSnapshotContent;
  [JournalSnapshotContentType.Skill]: JournalSkillSnapshotContent;
  [JournalSnapshotContentType.Browser]: JournalBrowserSnapshotContent;
}

type JournalSnapshotContentBranch<
  TKey extends keyof JournalSnapshotContentMap,
> = { [TField in TKey]: JournalSnapshotContentMap[TField] } & {
  [TField in Exclude<keyof JournalSnapshotContentMap, TKey>]?: never;
};

export type JournalSnapshotContent = {
  [TKey in keyof JournalSnapshotContentMap]: JournalSnapshotContentBranch<TKey>;
}[keyof JournalSnapshotContentMap];

type JournalSnapshotEnvelopeBranch<
  TKey extends keyof JournalSnapshotContentMap,
> = Omit<
  GeneratedJournalSnapshotEnvelope,
  'content_type' | 'content' | 'status'
> & {
  content_type: TKey;
  content: JournalSnapshotContentBranch<TKey>;
  status:
    | JournalContentStatus.Loading
    | JournalContentStatus.Ready
    | JournalContentStatus.Streaming;
};

type JournalSnapshotEnvelopeWithContent = {
  [TKey in keyof JournalSnapshotContentMap]: JournalSnapshotEnvelopeBranch<TKey>;
}[keyof JournalSnapshotContentMap];

type JournalSnapshotEnvelopeWithoutContent = Omit<
  GeneratedJournalSnapshotEnvelope,
  'content' | 'status'
> & {
  content?: never;
  status:
    | JournalContentStatus.Empty
    | JournalContentStatus.Loading
    | JournalContentStatus.Error
    | JournalContentStatus.NoPermission;
};

export type JournalSnapshotEnvelope =
  | JournalSnapshotEnvelopeWithContent
  | JournalSnapshotEnvelopeWithoutContent;

export type JournalEventStreamFrame = Omit<
  GeneratedJournalEventStreamFrame,
  'kind' | 'event'
> & {
  kind: JournalStreamFrameKind.Event;
  event: JournalEvent;
};

export type JournalHeartbeatStreamFrame = Omit<
  GeneratedJournalHeartbeatStreamFrame,
  'kind'
> & {
  kind: JournalStreamFrameKind.Heartbeat;
};

export type JournalControlStreamFrame = Omit<
  GeneratedJournalControlStreamFrame,
  'kind'
> & {
  kind: JournalStreamFrameKind.Control;
};

export type JournalStreamFrame =
  | JournalEventStreamFrame
  | JournalHeartbeatStreamFrame
  | JournalControlStreamFrame;
