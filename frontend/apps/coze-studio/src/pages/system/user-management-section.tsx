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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function -- Cohesive orchestrator. */
/* eslint-disable max-lines, complexity -- Cohesive orchestrator. */

import { useState } from 'react';

import {
  formatAdminTime,
  getRoleLabel,
  getWorkspaceDisplayName,
  getWorkspaceTypeLabel,
  isPersonalWorkspace,
} from './view-model';
import type {
  AdminCreateUserPayload,
  AdminResetUserPasswordPayload,
  AdminUpdateUserPayload,
  AdminUser,
  AdminUserSpace,
} from './service';

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
  isLoading?: boolean;
  isUserMutating?: boolean;
  userActionMessage?: string;
  onKeywordChange: (keyword: string) => void;
  onCreateUser?: (payload: AdminCreateUserPayload) => void | Promise<void>;
  onOpenWorkspace?: (spaceID: string) => void | Promise<void>;
  onRefresh?: () => void | Promise<void>;
  onResetUserPassword?: (
    payload: AdminResetUserPasswordPayload,
  ) => void | Promise<void>;
  onSearch: () => void | Promise<void>;
  onTurnPage: (page: number) => void | Promise<void>;
  onShowUserSpaces: (user: AdminUser) => void | Promise<void>;
  onUpdateUser?: (payload: AdminUpdateUserPayload) => void | Promise<void>;
}

const getUserDisplayName = (user: AdminUser) =>
  user.name || user.email || user.user_id;

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
  isLoading = false,
  isUserMutating = false,
  userActionMessage = '',
  onKeywordChange,
  onCreateUser,
  onOpenWorkspace,
  onRefresh,
  onResetUserPassword,
  onSearch,
  onTurnPage,
  onShowUserSpaces,
  onUpdateUser,
}: UserManagementSectionProps) => {
  const personalUserSpaceCount = userSpaces.filter(isPersonalWorkspace).length;
  const teamUserSpaceCount = userSpaces.length - personalUserSpaceCount;
  const [actionType, setActionType] = useState<
    'create' | 'edit' | 'reset' | null
  >(null);
  const [actionUser, setActionUser] = useState<AdminUser | null>(null);
  const [formEmail, setFormEmail] = useState('');
  const [formPassword, setFormPassword] = useState('');
  const [formName, setFormName] = useState('');
  const [formUniqueName, setFormUniqueName] = useState('');
  const [formLocale, setFormLocale] = useState('zh-CN');
  const [formMessage, setFormMessage] = useState('');

  const closeUserAction = () => {
    setActionType(null);
    setActionUser(null);
    setFormEmail('');
    setFormPassword('');
    setFormName('');
    setFormUniqueName('');
    setFormLocale('zh-CN');
    setFormMessage('');
  };

  const openCreateUser = () => {
    closeUserAction();
    setActionType('create');
  };

  const openEditUser = (user: AdminUser) => {
    closeUserAction();
    setActionType('edit');
    setActionUser(user);
    setFormName(user.name || '');
    setFormUniqueName(user.user_unique_name || '');
  };

  const openResetPassword = (user: AdminUser) => {
    closeUserAction();
    setActionType('reset');
    setActionUser(user);
  };

  const submitCreateUser = async () => {
    const email = formEmail.trim();
    const password = formPassword.trim();
    if (!email) {
      setFormMessage('邮箱不能为空');
      return;
    }
    if (password.length < 6) {
      setFormMessage('密码至少 6 位');
      return;
    }

    setFormMessage('');
    await onCreateUser?.({
      email,
      locale: formLocale.trim() || 'zh-CN',
      name: formName.trim(),
      password,
      user_unique_name: formUniqueName.trim(),
    });
    closeUserAction();
  };

  const submitUpdateUser = async () => {
    if (!actionUser?.user_id) {
      setFormMessage('请选择用户');
      return;
    }

    setFormMessage('');
    await onUpdateUser?.({
      locale: formLocale.trim() || 'zh-CN',
      name: formName.trim(),
      user_id: actionUser.user_id,
      user_unique_name: formUniqueName.trim(),
    });
    closeUserAction();
  };

  const submitResetPassword = async () => {
    if (!actionUser?.user_id) {
      setFormMessage('请选择用户');
      return;
    }
    const password = formPassword.trim();
    if (password.length < 6) {
      setFormMessage('密码至少 6 位');
      return;
    }

    setFormMessage('');
    await onResetUserPassword?.({
      password,
      user_id: actionUser.user_id,
    });
    closeUserAction();
  };

  return (
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
              ? `已选 ${getUserDisplayName(selectedUser)} · 团队 ${teamUserSpaceCount} 个 / 个人 ${personalUserSpaceCount} 个`
              : '选择用户后查看所属工作空间'}
          </p>
        </div>
        <span>
          {selectedUser ? `已展开 ${userSpaces.length} 个空间` : '未选择用户'}
        </span>
      </article>

      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>筛选用户</h2>
          <p>按昵称、邮箱或用户名搜索用户。</p>
        </div>
        <div className="flex items-center gap-[8px]">
          {onCreateUser ? (
            <button
              aria-label="新增用户"
              className="h-[32px] rounded-[8px] bg-[#1d2333] px-[12px] text-[13px] text-white disabled:cursor-not-allowed disabled:opacity-60"
              disabled={isLoading || isUserMutating}
              type="button"
              onClick={openCreateUser}
            >
              新增用户
            </button>
          ) : null}
          <input
            aria-label="搜索用户"
            className="h-[32px] w-[220px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
            placeholder="搜索用户"
            value={userKeyword}
            onChange={event => onKeywordChange(event.target.value)}
          />
          <button
            aria-label="执行用户搜索"
            className="h-[32px] rounded-[8px] bg-[#1d2333] px-[12px] text-[13px] text-white disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isLoading}
            type="button"
            onClick={() => void onSearch()}
          >
            搜索
          </button>
          <button
            aria-label="刷新用户列表"
            className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px] disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isLoading || !onRefresh}
            type="button"
            onClick={() => void onRefresh?.()}
          >
            刷新
          </button>
        </div>
      </article>

      {actionType ? (
        <article className="coze-prototype-workspace-settings-row">
          <div className="w-full">
            <h2>
              {actionType === 'create'
                ? '新增用户'
                : actionType === 'edit'
                  ? '编辑用户'
                  : '密码重置'}
            </h2>
            <p>
              {actionType === 'create'
                ? '创建后台用户并自动生成默认个人空间。'
                : actionType === 'edit'
                  ? '修改用户昵称、用户名和语言。'
                  : actionUser
                    ? `为 ${getUserDisplayName(actionUser)} 重置登录密码。`
                    : '重置当前用户的登录密码。'}
            </p>
            {userActionMessage ? <p>{userActionMessage}</p> : null}
            {formMessage ? <p>{formMessage}</p> : null}
            <div className="mt-[12px] grid gap-[10px] md:grid-cols-2">
              {actionType === 'create' ? (
                <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
                  邮箱
                  <input
                    aria-label="新增用户邮箱"
                    className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                    placeholder="user@example.com"
                    value={formEmail}
                    onChange={event => setFormEmail(event.target.value)}
                  />
                </label>
              ) : null}
              {actionType !== 'reset' ? (
                <>
                  <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
                    昵称
                    <input
                      aria-label={
                        actionType === 'create'
                          ? '新增用户昵称'
                          : '编辑用户昵称'
                      }
                      className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                      placeholder="用户昵称"
                      value={formName}
                      onChange={event => setFormName(event.target.value)}
                    />
                  </label>
                  <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
                    用户名
                    <input
                      aria-label={
                        actionType === 'create' ? '新增用户名' : '编辑用户名'
                      }
                      className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                      placeholder="唯一用户名"
                      value={formUniqueName}
                      onChange={event => setFormUniqueName(event.target.value)}
                    />
                  </label>
                  <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
                    语言
                    <input
                      aria-label={
                        actionType === 'create'
                          ? '新增用户语言'
                          : '编辑用户语言'
                      }
                      className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                      placeholder="zh-CN"
                      value={formLocale}
                      onChange={event => setFormLocale(event.target.value)}
                    />
                  </label>
                </>
              ) : null}
              {actionType === 'create' || actionType === 'reset' ? (
                <label className="flex flex-col gap-[6px] text-[13px] text-[#4d566a]">
                  登录密码
                  <input
                    aria-label={
                      actionType === 'create' ? '新增用户密码' : '重置用户密码'
                    }
                    className="h-[34px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
                    placeholder="至少 6 位"
                    type="password"
                    value={formPassword}
                    onChange={event => setFormPassword(event.target.value)}
                  />
                </label>
              ) : null}
            </div>
            <div className="mt-[12px] flex flex-wrap gap-[8px]">
              <button
                aria-label={
                  actionType === 'create'
                    ? '提交新增用户'
                    : actionType === 'edit'
                      ? '提交编辑用户'
                      : '提交重置密码'
                }
                className="h-[32px] rounded-[8px] bg-[#1d2333] px-[12px] text-[13px] text-white disabled:cursor-not-allowed disabled:opacity-60"
                disabled={isUserMutating}
                type="button"
                onClick={() => {
                  if (actionType === 'create') {
                    void submitCreateUser();
                    return;
                  }
                  if (actionType === 'edit') {
                    void submitUpdateUser();
                    return;
                  }
                  void submitResetPassword();
                }}
              >
                {isUserMutating ? '提交中...' : '提交'}
              </button>
              <button
                aria-label="取消用户操作"
                className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px]"
                type="button"
                onClick={closeUserAction}
              >
                取消
              </button>
            </div>
          </div>
          <span>真实接口</span>
        </article>
      ) : null}

      <article className="coze-prototype-workspace-settings-row">
        <div className="w-full">
          <h2>用户列表</h2>
          <p>
            第 {userPage} 页，共 {userTotal} 个用户。
          </p>
          {isLoading ? <p>正在加载用户...</p> : null}
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
                    <td
                      className="px-[14px] py-[18px] text-[#687385]"
                      colSpan={5}
                    >
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
                      <div className="flex flex-wrap gap-[8px]">
                        <button
                          aria-label={`查看用户空间-${user.user_id}`}
                          className="h-[30px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[12px]"
                          type="button"
                          onClick={() => void onShowUserSpaces(user)}
                        >
                          查看空间
                        </button>
                        {onUpdateUser ? (
                          <button
                            aria-label={`编辑用户-${user.user_id}`}
                            className="h-[30px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[12px]"
                            type="button"
                            onClick={() => openEditUser(user)}
                          >
                            编辑
                          </button>
                        ) : null}
                        {onResetUserPassword ? (
                          <button
                            aria-label={`重置用户密码-${user.user_id}`}
                            className="h-[30px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[12px]"
                            type="button"
                            onClick={() => openResetPassword(user)}
                          >
                            密码重置
                          </button>
                        ) : null}
                      </div>
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
                <table className="w-full min-w-[760px] border-collapse text-left text-[13px]">
                  <thead className="bg-[#f8fafc] text-[#687385]">
                    <tr>
                      <th className="px-[14px] py-[10px] font-medium">
                        工作空间
                      </th>
                      <th className="px-[14px] py-[10px] font-medium">类型</th>
                      <th className="px-[14px] py-[10px] font-medium">角色</th>
                      <th className="px-[14px] py-[10px] font-medium">
                        成员数
                      </th>
                      <th className="px-[14px] py-[10px] font-medium">
                        所有者
                      </th>
                      <th className="px-[14px] py-[10px] font-medium">操作</th>
                    </tr>
                  </thead>
                  <tbody>
                    {userSpaces.map(space => (
                      <tr className="border-t border-[#edf0f5]" key={space.id}>
                        <td className="px-[14px] py-[12px] text-[#1d2333]">
                          {getWorkspaceDisplayName(space)}
                        </td>
                        <td className="px-[14px] py-[12px] text-[#4d566a]">
                          <span className="rounded-full bg-[#f1f4f9] px-[8px] py-[4px] text-[12px] text-[#4d566a]">
                            {getWorkspaceTypeLabel(space)}
                          </span>
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
                        <td className="px-[14px] py-[12px]">
                          {onOpenWorkspace ? (
                            <button
                              aria-label={`进入用户所属空间-${space.id}`}
                              className="h-[30px] rounded-[8px] bg-[#1d2333] px-[10px] text-[12px] text-white"
                              type="button"
                              onClick={() => void onOpenWorkspace(space.id)}
                            >
                              进入空间
                            </button>
                          ) : (
                            <span className="text-[#a0a7b5]">-</span>
                          )}
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
            disabled={isLoading || userPage <= 1}
            type="button"
            onClick={() => void onTurnPage(userPage - 1)}
          >
            上一页
          </button>
          <button
            aria-label="下一页用户"
            className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px]"
            disabled={isLoading || userPage * pageSize >= userTotal}
            type="button"
            onClick={() => void onTurnPage(userPage + 1)}
          >
            下一页
          </button>
        </div>
      </article>
    </section>
  );
};
