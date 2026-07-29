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

import { useState } from 'react';

import type { HumanInteractionResponse } from '../workbench/thread-client';
import { Button, TextArea } from '@coze-arch/coze-design';

import type { PendingHumanInteraction } from './task-human-interaction';

const RESPONSE_SCHEMA = 'coze.human_interaction_response.v1';

const compact = (value: string | undefined) => value?.trim() ?? '';

const buildClarificationResponse = ({
  answer,
  choiceId,
  pending,
}: {
  answer: string;
  choiceId: string;
  pending: PendingHumanInteraction;
}): HumanInteractionResponse => {
  const response: HumanInteractionResponse = {
    schema: RESPONSE_SCHEMA,
    interaction_id: pending.interactionId,
    kind: 'clarification',
    decision: 'answered',
  };
  if (compact(answer)) {
    response.answer = compact(answer);
  }
  if (compact(choiceId)) {
    response.choice_id = compact(choiceId);
  }

  return response;
};

const buildConfirmationResponse = ({
  comment,
  decision,
  pending,
}: {
  comment: string;
  decision: 'approved' | 'rejected';
  pending: PendingHumanInteraction;
}): HumanInteractionResponse => {
  const response: HumanInteractionResponse = {
    schema: RESPONSE_SCHEMA,
    interaction_id: pending.interactionId,
    kind: 'confirmation',
    decision,
  };
  if (compact(comment)) {
    response.comment = compact(comment);
  }

  return response;
};

interface TaskHumanInterruptCardProps {
  error?: string;
  loading: boolean;
  pending: PendingHumanInteraction;
  onSubmit: (response: HumanInteractionResponse) => void | Promise<void>;
}

const ConfirmationInterruptCard = ({
  error,
  loading,
  onSubmit,
  pending,
}: TaskHumanInterruptCardProps) => {
  const [comment, setComment] = useState('');
  const { prompt } = pending;

  return (
    <section className="coze-prototype-human-interrupt">
      <div className="coze-prototype-human-interrupt-eyebrow">等待确认</div>
      <h2>{prompt.title || '请确认下一步操作'}</h2>
      {prompt.summary ? <p>{prompt.summary}</p> : null}
      <div className="coze-prototype-human-interrupt-meta">
        {prompt.action ? <span>操作：{prompt.action}</span> : null}
        {prompt.risk_level ? <span>风险：{prompt.risk_level}</span> : null}
      </div>
      {prompt.consequences?.length ? (
        <ul className="coze-prototype-human-interrupt-list">
          {prompt.consequences.map(item => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      ) : null}
      <TextArea
        aria-label="拒绝原因"
        className="coze-prototype-human-interrupt-input"
        placeholder="拒绝时可填写原因"
        value={comment}
        onChange={setComment}
      />
      {error ? (
        <div className="coze-prototype-error" role="alert">
          {error}
        </div>
      ) : null}
      <div className="coze-prototype-human-interrupt-actions">
        <Button
          type="danger"
          theme="light"
          loading={loading}
          onClick={() =>
            void onSubmit(
              buildConfirmationResponse({
                comment,
                decision: 'rejected',
                pending,
              }),
            )
          }
        >
          拒绝执行
        </Button>
        <Button
          type="primary"
          theme="solid"
          loading={loading}
          onClick={() =>
            void onSubmit(
              buildConfirmationResponse({
                comment,
                decision: 'approved',
                pending,
              }),
            )
          }
        >
          确认执行
        </Button>
      </div>
    </section>
  );
};

const ClarificationInterruptCard = ({
  error,
  loading,
  onSubmit,
  pending,
}: TaskHumanInterruptCardProps) => {
  const [answer, setAnswer] = useState('');
  const [choiceId, setChoiceId] = useState('');
  const { prompt } = pending;

  const canSubmit =
    !prompt.required || Boolean(compact(answer) || compact(choiceId));

  return (
    <section className="coze-prototype-human-interrupt">
      <div className="coze-prototype-human-interrupt-eyebrow">等待补充</div>
      <h2>{prompt.title || '需要补充信息'}</h2>
      <p>{prompt.question}</p>
      {prompt.description ? <p>{prompt.description}</p> : null}
      {prompt.choices?.length ? (
        <div className="coze-prototype-human-interrupt-choices">
          {prompt.choices.map(choice => {
            const value = choice.id || choice.value || choice.label || '';
            return (
              <Button
                key={value}
                type="tertiary"
                theme={choiceId === value ? 'solid' : 'outline'}
                onClick={() => setChoiceId(value)}
              >
                {choice.label || choice.value || choice.id}
              </Button>
            );
          })}
        </div>
      ) : null}
      {prompt.allow_free_text || !prompt.choices?.length ? (
        <TextArea
          aria-label="补充信息"
          className="coze-prototype-human-interrupt-input"
          placeholder="输入补充信息"
          value={answer}
          onChange={setAnswer}
        />
      ) : null}
      {error ? (
        <div className="coze-prototype-error" role="alert">
          {error}
        </div>
      ) : null}
      <div className="coze-prototype-human-interrupt-actions">
        <Button
          type="primary"
          theme="solid"
          disabled={!canSubmit}
          loading={loading}
          onClick={() =>
            void onSubmit(
              buildClarificationResponse({
                answer,
                choiceId,
                pending,
              }),
            )
          }
        >
          提交回答
        </Button>
      </div>
    </section>
  );
};

export const TaskHumanInterruptCard = (props: TaskHumanInterruptCardProps) =>
  props.pending.kind === 'confirmation' ? (
    <ConfirmationInterruptCard {...props} />
  ) : (
    <ClarificationInterruptCard {...props} />
  );
