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

import { IconCozCopy, IconCozRefresh } from '@coze-arch/coze-design/icons';

import { copyTextToClipboard } from '../task-clipboard';
import {
  journalFailureDetails,
  type JournalActionItem,
} from './journal-event-model';

export const JournalFailureDetail = ({
  action,
  recoveryAvailable,
  onRequestRecovery,
}: {
  action: JournalActionItem;
  recoveryAvailable: boolean;
  onRequestRecovery: (action: JournalActionItem) => void;
}) => {
  const failure = journalFailureDetails(action.event);
  if (!failure) {
    return null;
  }

  return (
    <div className="journal-failure-detail">
      <small className="journal-failure-summary">{failure.summary}</small>
      <div className="journal-action-failure-actions">
        {failure.traceID ? (
          <button
            type="button"
            aria-label={`复制 Trace ID ${failure.traceID}`}
            title={`Trace ID: ${failure.traceID}`}
            onClick={() => void copyTextToClipboard(failure.traceID ?? '')}
          >
            <IconCozCopy />
            <span>Trace ID: {failure.traceID}</span>
          </button>
        ) : null}
        {recoveryAvailable ? (
          <button
            type="button"
            aria-label="恢复失败操作"
            title="恢复失败操作"
            onClick={() => onRequestRecovery(action)}
          >
            <IconCozRefresh />
          </button>
        ) : null}
      </div>
    </div>
  );
};
