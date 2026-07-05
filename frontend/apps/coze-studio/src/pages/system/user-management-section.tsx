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

import type { AdminUser, AdminUserSpace } from './service';
import { getRoleLabel } from './view-model';

interface UserManagementSectionProps {
  users: AdminUser[];
  userTotal: number;
  userKeyword: string;
  userPage: number;
  pageSize: number;
  selectedUser: AdminUser | null;
  userSpaces: AdminUserSpace[];
  userSpacesLoading: boolean;
  userSpacesError: string;
  onKeywordChange: (keyword: string) => void;
  onSearch: () => void | Promise<void>;
  onTurnPage: (page: number) => void | Promise<void>;
  onShowUserSpaces: (user: AdminUser) => void | Promise<void>;
}

const getUserDisplayName = (user: AdminUser) =>
  user.name || user.email || user.user_id;

const formatAdminTime = (timestamp?: number) => {
  if (!timestamp) {
    return '-';
  }
  return new Date(timestamp * 1000).toLocaleString('zh-CN', {
    hour12: false,
  });
};

export const UserManagementSection = ({
  users,
  userTotal,
  userKeyword,
  userPage,
  pageSize,
  selectedUser,
  userSpaces,
  userSpacesLoading,
  userSpacesError,
  onKeywordChange,
  onSearch,
  onTurnPage,
  onShowUserSpaces,
}: UserManagementSectionProps) => (
  <section className="coze-prototype-workspace-settings-list">
    <article className="coze-prototype-workspace-settings-row">
      <div>
        <h2>用户总览</h2>
        <p>后台账号、空间归属和权限边界的集中查看入口。</p>
        <p>
          共 {userTotal} 个用户 · 本页 {users.length} 个账号
        </p>
        <div className="mt-[12px] grid gap-[10px] md:grid-cols-3">
          <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
            <p className="m-0 text-[12px] text-[#687385]">用户总数</p>
            <strong className="text-[20px] text-[#1d2333]">
              {userTotal}
            </strong>
          </div>
          <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
            <p className="m-0 text-[12px] text-[#687385]">本页账号</p>
            <strong className="text-[20px] text-[#1d2333]">
              {users.length}
            </strong>
          </div>
          <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
            <p className="m-0 text-[12px] text-[#687385]">已展开空间</p>
            <strong className="text-[20px] text-[#1d2333]">
              {selectedUser ? userSpaces.length : 0}
            </strong>
          </div>
        </div>
        <p>
          {selectedUser
            ? `已选 ${getUserDisplayName(selectedUser)}`
            : '选择用户后查看所属工作空间'}
        </p>
      </div>
      <span>
        {selectedUser
          ? `已展开 ${userSpaces.length} 个空间`
          : '未选择用户'}
      </span>
    </article>

    <article className="coze-prototype-workspace-settings-row">
      <div>
        <h2>筛选用户</h2>
        <p>按昵称、邮箱或用户名搜索用户。</p>
      </div>
      <div className="flex items-center gap-[8px]">
        <input
          aria-label="搜索用户"
          className="h-[32px] w-[220px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
          placeholder="搜索用户"
          value={userKeyword}
          onChange={event => onKeywordChange(event.target.value)}
        />
        <button
          aria-label="执行用户搜索"
          className="h-[32px] rounded-[8px] bg-[#1d2333] px-[12px] text-[13px] text-white"
          type="button"
          onClick={() => void onSearch()}
        >
          搜索
        </button>
      </div>
    </article>

    <article className="coze-prototype-workspace-settings-row">
      <div className="w-full">
        <h2>用户列表</h2>
        <p>
          第 {userPage} 页，共 {userTotal} 个用户。
        </p>
        <div className="mt-[12px] overflow-x-auto rounded-[12px] border border-[#e7ebf3]">
          <table className="w-full min-w-[760px] border-collapse text-left text-[13px]">
            <thead className="bg-[#f8fafc] text-[#687385]">
              <tr>
                <th className="px-[14px] py-[10px] font-medium">用户</th>
                <th className="px-[14px] py-[10px] font-medium">邮箱</th>
                <th className="px-[14px] py-[10px] font-medium">用户名</th>
                <th className="px-[14px] py-[10px] font-medium">创建时间</th>
                <th className="px-[14px] py-[10px] font-medium">操作</th>
              </tr>
            </thead>
            <tbody>
              {users.length === 0 ? (
                <tr>
                  <td className="px-[14px] py-[18px] text-[#687385]" colSpan={5}>
                    暂无用户，当前筛选条件下没有账号。
                  </td>
                </tr>
              ) : null}
              {users.map(user => (
                <tr className="border-t border-[#edf0f5]" key={user.user_id}>
                  <td className="px-[14px] py-[12px] text-[#1d2333]">
                    {getUserDisplayName(user)}
                  </td>
                  <td className="px-[14px] py-[12px] text-[#4d566a]">
                    {user.email || '未绑定邮箱'}
                  </td>
                  <td className="px-[14px] py-[12px] text-[#4d566a]">
                    {user.user_unique_name || user.user_id}
                  </td>
                  <td className="px-[14px] py-[12px] text-[#4d566a]">
                    {formatAdminTime(user.created_at)}
                  </td>
                  <td className="px-[14px] py-[12px]">
                    <button
                      aria-label={`查看用户空间-${user.user_id}`}
                      className="h-[30px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[12px]"
                      type="button"
                      onClick={() => void onShowUserSpaces(user)}
                    >
                      查看空间
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
      <span>{users.length} 个账号</span>
    </article>

    {selectedUser ? (
      <article className="coze-prototype-workspace-settings-row">
        <div className="w-full">
          <h2>所属空间详情</h2>
          <p>{getUserDisplayName(selectedUser)}</p>
          {userSpacesLoading ? <p>正在加载工作空间...</p> : null}
          {userSpacesError ? <p>{userSpacesError}</p> : null}
          {!userSpacesLoading && userSpaces.length === 0 ? (
            <p>暂无所属工作空间</p>
          ) : null}
          {userSpaces.length > 0 ? (
            <div className="mt-[12px] overflow-x-auto rounded-[12px] border border-[#e7ebf3]">
              <table className="w-full min-w-[620px] border-collapse text-left text-[13px]">
                <thead className="bg-[#f8fafc] text-[#687385]">
                  <tr>
                    <th className="px-[14px] py-[10px] font-medium">
                      工作空间
                    </th>
                    <th className="px-[14px] py-[10px] font-medium">角色</th>
                    <th className="px-[14px] py-[10px] font-medium">成员数</th>
                    <th className="px-[14px] py-[10px] font-medium">所有者</th>
                  </tr>
                </thead>
                <tbody>
                  {userSpaces.map(space => (
                    <tr className="border-t border-[#edf0f5]" key={space.id}>
                      <td className="px-[14px] py-[12px] text-[#1d2333]">
                        {space.name}
                      </td>
                      <td className="px-[14px] py-[12px] text-[#4d566a]">
                        {getRoleLabel(space.role_type)}
                      </td>
                      <td className="px-[14px] py-[12px] text-[#4d566a]">
                        {space.total_member_num ?? 0} 人
                      </td>
                      <td className="px-[14px] py-[12px] text-[#4d566a]">
                        {space.owner_name || space.owner_user_id}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </div>
        <span>{userSpaces.length} 个空间</span>
      </article>
    ) : null}

    <article className="coze-prototype-workspace-settings-row">
      <div>
        <h2>分页</h2>
        <p>
          第 {userPage} 页，共 {userTotal} 个用户。
        </p>
      </div>
      <div className="flex items-center gap-[8px]">
        <button
          aria-label="上一页用户"
          className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px]"
          disabled={userPage <= 1}
          type="button"
          onClick={() => void onTurnPage(userPage - 1)}
        >
          上一页
        </button>
        <button
          aria-label="下一页用户"
          className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px]"
          disabled={userPage * pageSize >= userTotal}
          type="button"
          onClick={() => void onTurnPage(userPage + 1)}
        >
          下一页
        </button>
      </div>
    </article>
  </section>
);
