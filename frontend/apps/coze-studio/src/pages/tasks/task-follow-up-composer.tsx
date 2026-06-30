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

import type { ReactNode } from 'react';

import { getWorkbenchLLMModels } from '../workbench/service';
import { WorkbenchComposer } from '../workbench/components/workbench-composer';
import {
  type WorkbenchComposerSubmitPayload,
  type WorkbenchMode,
} from '../workbench/components/types';

export const TaskFollowUpComposer = ({
  value,
  mode,
  loading,
  error,
  spaceId,
  taskId,
  todoDock,
  stopLoading,
  stopMode,
  onValueChange,
  onModeChange,
  onStop,
  onSubmit,
}: {
  value: string;
  mode: WorkbenchMode;
  loading: boolean;
  error?: string;
  spaceId?: string;
  taskId?: string;
  todoDock?: ReactNode;
  stopLoading?: boolean;
  stopMode?: boolean;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onStop?: () => void | Promise<void>;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void | Promise<void>;
}) => (
  <section className="coze-prototype-followup">
    <div className="coze-prototype-followup-stack">
      {todoDock}
      <WorkbenchComposer
        value={value}
        mode={mode}
        loading={loading}
        error={error}
        variant="detail"
        presentation="deerflow"
        spaceId={spaceId}
        taskId={taskId}
        stopLoading={stopLoading}
        stopMode={stopMode}
        modelLoader={getWorkbenchLLMModels}
        onValueChange={onValueChange}
        onModeChange={onModeChange}
        onStop={onStop}
        onSubmit={onSubmit}
      />
    </div>
  </section>
);
