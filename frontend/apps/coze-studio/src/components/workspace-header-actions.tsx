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
import { Badge } from '@coze-arch/coze-design';

import { WorkspaceAccountDropdown } from './workspace-account-dropdown';
import { NotificationBell } from './notification-center/notification-bell';

interface WorkspaceHeaderActionsProps {
  className?: string;
  compact?: boolean;
  onNotificationClick?: () => void;
  unreadCount?: number;
}

export const WorkspaceHeaderActions = ({
  className,
  compact = false,
  onNotificationClick,
  unreadCount,
}: WorkspaceHeaderActionsProps) => {
  const normalizedUnreadCount = Number.isFinite(unreadCount ?? 0)
    ? Math.max(0, Math.floor(unreadCount ?? 0))
    : 0;
  const notificationButton = (
    <button
      type="button"
      className={[
        'notification-bell__trigger',
        compact ? 'notification-bell__trigger--compact' : '',
      ].join(' ')}
      aria-label={
        normalizedUnreadCount > 0
          ? `通知，${normalizedUnreadCount} 条未读`
          : '通知'
      }
      onClick={onNotificationClick}
    >
      <IconCozBell className={compact ? 'text-[14px]' : 'text-[16px]'} />
    </button>
  );

  return (
    <div
      className={[
        'flex shrink-0 items-center',
        compact ? 'gap-[6px]' : 'gap-[8px]',
        className,
      ]
        .filter(Boolean)
        .join(' ')}
    >
      {onNotificationClick ? (
        normalizedUnreadCount > 0 ? (
          <Badge
            count={<span aria-hidden="true">{normalizedUnreadCount}</span>}
          >
            {notificationButton}
          </Badge>
        ) : (
          notificationButton
        )
      ) : (
        <NotificationBell compact={compact} unreadCount={unreadCount} />
      )}
      <WorkspaceAccountDropdown />
    </div>
  );
};
