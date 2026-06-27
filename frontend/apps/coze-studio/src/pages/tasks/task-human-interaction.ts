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

import type { workbenchTask } from '@coze-studio/api-schema';

type TaskEvent = workbenchTask.TaskEvent;

export type HumanInteractionKind = 'clarification' | 'confirmation';

export interface HumanInteractionChoice {
  id?: string;
  label?: string;
  value?: string;
}

export interface HumanInteractionPrompt {
  schema: string;
  interaction_id: string;
  kind: HumanInteractionKind;
  title?: string;
  question?: string;
  description?: string;
  required?: boolean;
  allow_free_text?: boolean;
  choices?: HumanInteractionChoice[];
  risk_level?: string;
  tool_name?: string;
  tool_call_id?: string;
  policy_ref?: string;
  action?: string;
  summary?: string;
  consequences?: string[];
  affected_resources?: string[];
  default_decision?: string;
  rejection_guidance?: string;
  created_at?: number;
}

export interface PendingHumanInteraction {
  interruptId: string;
  interactionId: string;
  sourceRunId?: string;
  kind: HumanInteractionKind;
  prompt: HumanInteractionPrompt;
  event: TaskEvent;
}

interface InterruptItem {
  id?: string;
  is_root_cause?: boolean;
  info?: unknown;
}

const HUMAN_INTERACTION_SCHEMA = 'coze.human_interaction.v1';

const parseEventPayload = (payload?: string): Record<string, unknown> => {
  if (!payload) {
    return {};
  }
  try {
    const parsed = JSON.parse(payload);
    return isRecord(parsed) ? parsed : {};
  } catch (error) {
    void error;
    return {};
  }
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
  Boolean(value) && typeof value === 'object' && !Array.isArray(value);

const isHumanInteractionPrompt = (
  value: unknown,
): value is HumanInteractionPrompt => {
  if (!isRecord(value)) {
    return false;
  }
  return (
    value.schema === HUMAN_INTERACTION_SCHEMA &&
    typeof value.interaction_id === 'string' &&
    (value.kind === 'clarification' || value.kind === 'confirmation')
  );
};

const interruptItemsFromPayload = (
  payload: Record<string, unknown>,
): InterruptItem[] => {
  const { interrupts } = payload;
  if (!isRecord(interrupts) || !Array.isArray(interrupts.items)) {
    return [];
  }
  return interrupts.items.filter(isRecord) as InterruptItem[];
};

const promptsFromPayload = (
  payload: Record<string, unknown>,
): HumanInteractionPrompt[] => {
  const prompts: HumanInteractionPrompt[] = [];
  if (isHumanInteractionPrompt(payload.human_interaction)) {
    prompts.push(payload.human_interaction);
  }
  if (Array.isArray(payload.human_interactions)) {
    for (const item of payload.human_interactions) {
      if (isHumanInteractionPrompt(item)) {
        prompts.push(item);
      }
    }
  }

  return prompts;
};

const promptFromInterrupt = (
  item: InterruptItem,
): HumanInteractionPrompt | undefined => {
  if (isHumanInteractionPrompt(item.info)) {
    return item.info;
  }

  return undefined;
};

const collectInterruptedPrompts = (
  event: TaskEvent,
): PendingHumanInteraction[] => {
  const payload = parseEventPayload(event.payload);
  const interruptItems = interruptItemsFromPayload(payload);
  const prompts = promptsFromPayload(payload);
  const interruptByInteractionID = new Map<string, string>();
  const result: PendingHumanInteraction[] = [];

  for (const item of interruptItems) {
    const prompt = promptFromInterrupt(item);
    const interruptId = String(item.id ?? '').trim();
    if (!prompt || !interruptId) {
      continue;
    }
    interruptByInteractionID.set(prompt.interaction_id, interruptId);
    result.push({
      interruptId,
      interactionId: prompt.interaction_id,
      sourceRunId: event.run_id,
      kind: prompt.kind,
      prompt,
      event,
    });
  }

  for (const prompt of prompts) {
    if (
      result.some(
        item =>
          item.interactionId === prompt.interaction_id &&
          item.kind === prompt.kind,
      )
    ) {
      continue;
    }
    const interruptId = interruptByInteractionID.get(prompt.interaction_id);
    if (!interruptId) {
      continue;
    }
    result.push({
      interruptId,
      interactionId: prompt.interaction_id,
      sourceRunId: event.run_id,
      kind: prompt.kind,
      prompt,
      event,
    });
  }

  return result;
};

const interactionKey = (interruptId: string, interactionId: string) =>
  `${interruptId}:${interactionId}`;

export const getPendingHumanInteraction = (
  events: TaskEvent[],
): PendingHumanInteraction | undefined => {
  const pending = new Map<string, PendingHumanInteraction>();

  for (const event of events) {
    if (event.event_type === 'run.interrupted') {
      for (const item of collectInterruptedPrompts(event)) {
        pending.set(interactionKey(item.interruptId, item.interactionId), item);
      }
      continue;
    }

    if (event.event_type !== 'human.interaction.resolved') {
      continue;
    }
    const payload = parseEventPayload(event.payload);
    const interruptId = String(payload.interrupt_id ?? '').trim();
    const interactionId = String(payload.interaction_id ?? '').trim();
    if (interruptId && interactionId) {
      pending.delete(interactionKey(interruptId, interactionId));
      continue;
    }
    if (interactionId) {
      for (const key of pending.keys()) {
        if (key.endsWith(`:${interactionId}`)) {
          pending.delete(key);
        }
      }
    }
  }

  const pendingItems = Array.from(pending.values());

  return pendingItems[pendingItems.length - 1];
};
