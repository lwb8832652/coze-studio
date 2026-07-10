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

import { getWorkspaceDisplayName } from './view-model';
import type { AdminBasicConfig, AdminUser, AdminWorkspace } from './service';

interface OverviewSectionProps {
  userTotal: number;
  workspaceTotal: number;
  basicConfig: AdminBasicConfig;
  users: AdminUser[];
  workspaces: AdminWorkspace[];
}

export const OverviewSection = ({
  userTotal,
  workspaceTotal,
  basicConfig,
  users,
  workspaces,
}: OverviewSectionProps) => (
  <>
    <section className="coze-prototype-system-grid">
      <article className="coze-prototype-system-card">
        <h2>用户总数</h2>
        <p>当前系统用户账号数量。</p>
        <strong>{userTotal}</strong>
      </article>
      <article className="coze-prototype-system-card">
        <h2>工作空间总数</h2>
        <p>当前系统工作空间数量。</p>
        <strong>{workspaceTotal}</strong>
      </article>
      <article className="coze-prototype-system-card">
        <h2>管理员邮箱</h2>
        <p>后台管理入口的服务端权限白名单。</p>
        <strong>{basicConfig.admin_emails || '-'}</strong>
      </article>
    </section>

    <section className="coze-prototype-workspace-settings-list">
      <article className="coze-prototype-workspace-settings-card">
        <div>
          <h2>最近用户</h2>
          <p>展示本次加载到的用户快照，便于管理员快速定位账号。</p>
        </div>
        <div className="coze-prototype-system-list">
          {users.length ? (
            users.slice(0, 3).map(user => (
              <p key={user.user_id}>
                {user.name || user.email || user.user_id} ·{' '}
                {user.email || '未绑定邮箱'}
              </p>
            ))
          ) : (
            <p>暂无用户快照</p>
          )}
        </div>
        <span>{userTotal} 人</span>
      </article>
      <article className="coze-prototype-workspace-settings-card">
        <div>
          <h2>最近工作空间</h2>
          <p>展示本次加载到的工作空间快照，便于管理员快速巡检。</p>
        </div>
        <div className="coze-prototype-system-list">
          {workspaces.length ? (
            workspaces.slice(0, 3).map(workspace => (
              <p key={workspace.id}>
                {getWorkspaceDisplayName(workspace)} ·{' '}
                {workspace.owner_name || workspace.owner_user_id || '-'}
              </p>
            ))
          ) : (
            <p>暂无工作空间快照</p>
          )}
        </div>
        <span>{workspaceTotal} 个空间</span>
      </article>
    </section>
  </>
);
