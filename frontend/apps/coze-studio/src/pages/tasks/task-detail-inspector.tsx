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

import { useState, type ReactNode } from 'react';

import { SideSheet } from '@coze-arch/coze-design';

import { TaskRuntimeDoctorSection } from './task-runtime-doctor-section';
import { TaskMemorySection } from './task-memory-section';
import { TaskMCPRuntimeAuditSection } from './task-mcp-runtime-audit-section';
import { TaskGuardrailAuditSection } from './task-guardrail-audit-section';

export const TaskDetailInspector = ({
  artifactAction,
  memoryReadOnly,
  spaceId,
  threadId,
}: {
  artifactAction?: ReactNode;
  memoryReadOnly: boolean;
  spaceId?: string;
  threadId: string;
}) => {
  const [visible, setVisible] = useState(false);

  return (
    <>
      <button
        type="button"
        className="coze-prototype-task-action"
        onClick={() => setVisible(true)}
      >
        详情
      </button>
      <SideSheet
        size="large"
        title="任务详情"
        visible={visible}
        onCancel={() => setVisible(false)}
      >
        <div className="coze-prototype-detail-inspector">
          {artifactAction}
          <TaskRuntimeDoctorSection spaceId={spaceId} />
          <TaskGuardrailAuditSection threadId={threadId} />
          <TaskMCPRuntimeAuditSection threadId={threadId} />
          <TaskMemorySection readOnly={memoryReadOnly} threadId={threadId} />
        </div>
      </SideSheet>
    </>
  );
};
