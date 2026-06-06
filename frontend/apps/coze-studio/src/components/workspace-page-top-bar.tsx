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

import { IconCozBell } from '@coze-arch/coze-design/icons';

import './workspace-prototype.less';

export const WorkspacePageTopBar = () => (
  <header className="flex h-[52px] shrink-0 items-center justify-end gap-[12px] px-[24px]">
    <div className="flex items-center gap-[6px] text-[12px] leading-[18px] text-[#444c5c]">
      <span className="h-[6px] w-[6px] rounded-full bg-[#2a9e06]" />
      <span>Aime 专属助理准备好,先聊聊吧~</span>
    </div>
    <button
      type="button"
      className="border-0 bg-transparent p-0 text-[12px] leading-[18px] text-[#2a6df4] cursor-pointer"
    >
      去聊天专属助理
    </button>
    <button
      type="button"
      className="coze-prototype-icon-button"
      aria-label="通知"
    >
      <IconCozBell className="text-[14px]" />
    </button>
    <div className="coze-prototype-avatar">wb</div>
  </header>
);
