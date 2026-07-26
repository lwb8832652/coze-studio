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

import {
  IconCozAsynchronousTask,
  IconCozBell,
} from '@coze-arch/coze-design/icons';

import { TaskTokenUsageIndicator } from './task-token-usage-indicator';
import type { TaskThreadDetailModel } from './task-thread-detail-model';
import type {
  TaskDetailTokenUsage,
  TaskTokenUsageViewMode,
} from './task-detail-loader';

export const TaskTopBar = ({
  displayTitle,
  exportAction,
  inspectorAction,
  onTokenUsageViewModeChange,
  task,
  tokenUsage,
  tokenUsageViewMode,
}: {
  displayTitle?: string;
  exportAction?: ReactNode;
  inspectorAction?: ReactNode;
  onTokenUsageViewModeChange?: (mode: TaskTokenUsageViewMode) => void;
  task: TaskThreadDetailModel;
  tokenUsage?: TaskDetailTokenUsage;
  tokenUsageViewMode: TaskTokenUsageViewMode;
}) => (
  <header className="coze-prototype-task-topbar">
    <div className="coze-prototype-task-title-group">
      <IconCozAsynchronousTask className="text-[16px]" />
      <div className="coze-prototype-task-title-copy">
        <h1 className="coze-prototype-task-top-title">
          {displayTitle || task.title}
        </h1>
      </div>
    </div>
    <div className="coze-prototype-task-topbar-actions">
      <div className="coze-prototype-task-topbar-primary">
        <TaskTokenUsageIndicator
          tokenUsage={tokenUsage}
          viewMode={tokenUsageViewMode}
          onViewModeChange={onTokenUsageViewModeChange}
        />
        {exportAction}
        {inspectorAction}
      </div>
      <div
        className="coze-prototype-task-topbar-secondary"
        aria-label="任务辅助操作"
      >
        <button type="button" className="coze-prototype-task-action">
          ☆ 收藏
        </button>
        <button type="button" className="coze-prototype-task-action">
          分享
        </button>
        <button
          type="button"
          className="coze-prototype-icon-button"
          aria-label="通知"
        >
          <IconCozBell className="text-[14px]" />
        </button>
        <div className="coze-prototype-avatar">wb</div>
      </div>
    </div>
  </header>
);
