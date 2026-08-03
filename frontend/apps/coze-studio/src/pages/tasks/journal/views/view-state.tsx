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
  IconCozLoading,
  IconCozWarningCircle,
} from '@coze-arch/coze-design/icons';

import type { WorkbenchJournalContentStatus } from '../../../workbench/thread-client';

export const JournalViewState = ({
  status,
}: {
  status: WorkbenchJournalContentStatus;
}) => {
  if (status === 'loading' || status === 'streaming') {
    return (
      <div className="journal-view-state" role="status">
        <IconCozLoading className="journal-view-state-loading" />
        <span>正在加载执行快照...</span>
      </div>
    );
  }
  if (status === 'no_permission') {
    return (
      <div className="journal-view-state" role="status">
        <IconCozWarningCircle />
        <span>当前没有查看此内容的权限</span>
      </div>
    );
  }
  if (status === 'error') {
    return (
      <div className="journal-view-state" role="alert">
        <IconCozWarningCircle />
        <span>执行快照暂时无法加载</span>
      </div>
    );
  }
  return <div className="journal-view-state">当前步骤没有此类内容</div>;
};
