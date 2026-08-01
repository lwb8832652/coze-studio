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

import { IconCozCross, IconCozRefresh } from '@coze-arch/coze-design/icons';
import { Button } from '@coze-arch/coze-design';

import type { WorkbenchJournalRecoveryCapability } from '../../workbench/thread-client';
import type { JournalViewMode } from './journal-reducer';
import {
  journalFailureDetails,
  type JournalActionItem,
} from './journal-event-model';

export type JournalRecoveryHandler = (
  action: string,
  confirmed: boolean,
) => void | Promise<void>;

export const journalRecoveryAction = (
  item: JournalActionItem,
  capability?: WorkbenchJournalRecoveryCapability,
): string | undefined => {
  const failure = journalFailureDetails(item.event);
  if (!failure?.retryable || !capability?.allowed) {
    return undefined;
  }
  if (failure.ledgerStatus === 'unknown') {
    return ['retry', 'resume', 'skip', 'mark_succeeded'].find(action =>
      capability.allowed_actions.includes(action),
    );
  }
  return ['retry', 'resume'].find(action =>
    capability.allowed_actions.includes(action),
  );
};

const controlledUnknownActions = (
  capability: WorkbenchJournalRecoveryCapability,
): Array<{ action: string; label: string }> => {
  const actions: Array<{ action: string; label: string }> = [];
  if (capability.allowed_actions.includes('mark_succeeded')) {
    actions.push({ action: 'mark_succeeded', label: '标记成功并跳过' });
  }
  if (capability.allowed_actions.includes('skip')) {
    actions.push({ action: 'skip', label: '跳过该动作' });
  }
  const retryAction = capability.allowed_actions.includes('retry')
    ? 'retry'
    : capability.allowed_actions.includes('resume')
      ? 'resume'
      : '';
  if (retryAction) {
    actions.push({ action: retryAction, label: '重新发起' });
  }
  return actions;
};

export const recoveryRequiresDialog = (
  item: JournalActionItem,
  capability: WorkbenchJournalRecoveryCapability,
  viewMode: JournalViewMode,
): boolean =>
  journalFailureDetails(item.event)?.ledgerStatus === 'unknown' ||
  capability.requires_confirmation ||
  viewMode === 'historical';

export const JournalRecoveryDialog = ({
  busy,
  capability,
  error,
  item,
  onCancel,
  onRecover,
}: {
  busy: boolean;
  capability: WorkbenchJournalRecoveryCapability;
  error: string;
  item: JournalActionItem;
  onCancel: () => void;
  onRecover: JournalRecoveryHandler;
}) => {
  const failure = journalFailureDetails(item.event);
  const unknown = failure?.ledgerStatus === 'unknown';
  const normalAction = journalRecoveryAction(item, capability);
  const choices = unknown ? controlledUnknownActions(capability) : [];

  return (
    <div className="journal-recovery-backdrop">
      <section
        aria-labelledby="journal-recovery-title"
        aria-modal="true"
        className="journal-recovery-dialog"
        role="dialog"
      >
        <header>
          <h2 id="journal-recovery-title">
            {unknown ? '确认失败动作状态' : '确认重新执行'}
          </h2>
          <button
            type="button"
            aria-label="关闭恢复确认"
            disabled={busy}
            onClick={onCancel}
          >
            <IconCozCross />
          </button>
        </header>
        <div className="journal-recovery-dialog-body">
          <strong>{item.title}</strong>
          <p>{failure?.summary}</p>
          {error ? <p className="journal-recovery-error">{error}</p> : null}
        </div>
        <footer>
          {unknown
            ? choices.map(choice => (
                <Button
                  disabled={busy}
                  key={choice.action}
                  loading={busy}
                  size="small"
                  theme="light"
                  type="primary"
                  onClick={() => void onRecover(choice.action, true)}
                >
                  {choice.label}
                </Button>
              ))
            : null}
          {!unknown && normalAction ? (
            <Button
              disabled={busy}
              icon={<IconCozRefresh />}
              loading={busy}
              size="small"
              theme="solid"
              type="primary"
              onClick={() => void onRecover(normalAction, true)}
            >
              确认重试
            </Button>
          ) : null}
        </footer>
      </section>
    </div>
  );
};
