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

import { useNavigate } from 'react-router-dom';

import { useSpaceStore } from '@coze-foundation/space-store';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import { IconCozBell } from '@coze-arch/coze-design/icons';
import { Avatar } from '@coze-arch/coze-design';

import './workspace-prototype.less';

export const WorkspacePageTopBar = () => {
  const navigate = useNavigate();
  const spaceId = useSpaceStore(state => state.space?.id ?? '');
  const userInfo = useUserInfo();
  const userDisplayName =
    userInfo?.name || userInfo?.screen_name || userInfo?.email || '用户';

  return (
    <header className="newx-workspace-topbar flex h-[52px] shrink-0 items-center justify-end gap-[12px] px-[24px]">
      <div
        className="flex items-center gap-[6px] text-[12px] leading-[18px]"
        style={{ color: 'var(--newx-color-text-secondary)' }}
      >
        <span
          className="h-[6px] w-[6px] rounded-full"
          style={{ background: 'var(--newx-color-success)' }}
        />
        <span>NewX AI 专属助理已就绪，随时可以开始对话</span>
      </div>
      <button
        type="button"
        className="cursor-pointer border-0 bg-transparent p-0 text-[12px] leading-[18px]"
        style={{ color: 'var(--newx-color-link)' }}
        disabled={!spaceId}
        onClick={() => {
          if (spaceId) {
            navigate(`/space/${spaceId}/chats/new`);
          }
        }}
      >
        去聊天专属助理
      </button>
      <button
        type="button"
        className="coze-prototype-icon-button"
        aria-label="通知"
      >
        <IconCozBell className="text-[16px]" />
      </button>
      <span aria-label={`当前用户：${userDisplayName}`}>
        <Avatar
          src={userInfo?.avatar_url}
          className="coze-prototype-avatar"
          style={{
            color: '#fff',
            backgroundColor: 'var(--newx-color-accent)',
          }}
        >
          {userDisplayName}
        </Avatar>
      </span>
    </header>
  );
};
