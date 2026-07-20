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

const getSnapshotInitial = (value: string) =>
  Array.from(value.trim())[0]?.toLocaleUpperCase() || '?';

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

    <section className="coze-prototype-system-snapshot-grid">
      <article className="coze-prototype-system-snapshot-card">
        <header className="coze-prototype-system-snapshot-header">
          <h2>最近用户</h2>
          <p>展示本次加载到的用户快照，便于管理员快速定位账号。</p>
        </header>
        <div className="coze-prototype-system-snapshot-list" role="list">
          {users.length ? (
            users.slice(0, 3).map(user => {
              const displayName = user.name || user.email || user.user_id;
              return (
                <div
                  className="coze-prototype-system-snapshot-row"
                  key={user.user_id}
                  role="listitem"
                >
                  <span
                    className="coze-prototype-system-snapshot-avatar"
                    aria-hidden="true"
                  >
                    {getSnapshotInitial(displayName)}
                  </span>
                  <span className="coze-prototype-system-snapshot-copy">
                    <strong>{displayName}</strong>
                    <span>{user.email || '未绑定邮箱'}</span>
                  </span>
                </div>
              );
            })
          ) : (
            <p className="coze-prototype-system-snapshot-empty">
              暂无用户快照
            </p>
          )}
        </div>
        <footer className="coze-prototype-system-snapshot-footer">
          <span>已展示 {Math.min(users.length, 3)} 条</span>
          <strong>{userTotal} 人</strong>
        </footer>
      </article>
      <article className="coze-prototype-system-snapshot-card">
        <header className="coze-prototype-system-snapshot-header">
          <h2>最近工作空间</h2>
          <p>展示本次加载到的工作空间快照，便于管理员快速巡检。</p>
        </header>
        <div className="coze-prototype-system-snapshot-list" role="list">
          {workspaces.length ? (
            workspaces.slice(0, 3).map(workspace => {
              const displayName = getWorkspaceDisplayName(workspace);
              const owner =
                workspace.owner_name || workspace.owner_user_id || '未知';
              return (
                <div
                  className="coze-prototype-system-snapshot-row"
                  key={workspace.id}
                  role="listitem"
                >
                  <span
                    className="coze-prototype-system-snapshot-avatar coze-prototype-system-snapshot-avatar-workspace"
                    aria-hidden="true"
                  >
                    {getSnapshotInitial(displayName)}
                  </span>
                  <span className="coze-prototype-system-snapshot-copy">
                    <strong>{displayName}</strong>
                    <span>
                      所有者：{owner} · {workspace.total_member_num || 0} 人
                    </span>
                  </span>
                </div>
              );
            })
          ) : (
            <p className="coze-prototype-system-snapshot-empty">
              暂无工作空间快照
            </p>
          )}
        </div>
        <footer className="coze-prototype-system-snapshot-footer">
          <span>已展示 {Math.min(workspaces.length, 3)} 条</span>
          <strong>{workspaceTotal} 个空间</strong>
        </footer>
      </article>
    </section>
  </>
);
