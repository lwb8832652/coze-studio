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

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive orchestrator. */
/* eslint-disable complexity -- Cohesive orchestrator. */

import {
  formatAdminTime,
  getRoleLabel,
  getWorkspaceDisplayDescription,
  getWorkspaceDisplayName,
  getWorkspaceTypeLabel,
  isPersonalWorkspace,
} from './view-model';
import type { AdminWorkspace, AdminWorkspaceMember } from './service';

interface WorkspaceManagementSectionProps {
  workspaces: AdminWorkspace[];
  workspaceTotal: number;
  workspaceKeyword: string;
  workspacePage: number;
  pageSize: number;
  selectedWorkspace: AdminWorkspace | null;
  workspaceMembers: AdminWorkspaceMember[];
  workspaceMembersLoading: boolean;
  workspaceMembersError: string;
  isLoading?: boolean;
  onKeywordChange: (keyword: string) => void;
  onOpenWorkspace?: (spaceID: string) => void | Promise<void>;
  onRefresh?: () => void | Promise<void>;
  onSearch: () => void | Promise<void>;
  onTurnPage: (page: number) => void | Promise<void>;
  onShowWorkspaceMembers: (workspace: AdminWorkspace) => void | Promise<void>;
}

const getMemberDisplayName = (member: AdminWorkspaceMember) =>
  member.name || member.email || member.user_id;

export const WorkspaceManagementSection = ({
  workspaces,
  workspaceTotal,
  workspaceKeyword,
  workspacePage,
  pageSize,
  selectedWorkspace,
  workspaceMembers,
  workspaceMembersLoading,
  workspaceMembersError,
  isLoading = false,
  onKeywordChange,
  onOpenWorkspace,
  onRefresh,
  onSearch,
  onTurnPage,
  onShowWorkspaceMembers,
}: WorkspaceManagementSectionProps) => {
  const personalWorkspaceCount = workspaces.filter(isPersonalWorkspace).length;
  const teamWorkspaceCount = workspaces.length - personalWorkspaceCount;

  return (
    <section className="coze-prototype-workspace-settings-list">
      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>工作空间总览</h2>
          <p>集中查看个人空间、团队空间、所有者和成员规模。</p>
          <p>
            共 {workspaceTotal} 个空间 · 本页 {workspaces.length} 个空间
          </p>
          <div className="mt-[12px] grid gap-[10px] md:grid-cols-4">
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">空间总数</p>
              <strong className="text-[20px] text-[#1d2333]">
                {workspaceTotal}
              </strong>
            </div>
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">本页空间</p>
              <strong className="text-[20px] text-[#1d2333]">
                {workspaces.length}
              </strong>
            </div>
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">空间类型</p>
              <strong className="text-[20px] text-[#1d2333]">
                {teamWorkspaceCount} / {personalWorkspaceCount}
              </strong>
              <p className="m-0 mt-[4px] text-[12px] text-[#687385]">
                团队 / 个人
              </p>
            </div>
            <div className="rounded-[12px] border border-[#e7ebf3] bg-[#f8fafc] px-[14px] py-[12px]">
              <p className="m-0 text-[12px] text-[#687385]">已展开成员</p>
              <strong className="text-[20px] text-[#1d2333]">
                {selectedWorkspace ? workspaceMembers.length : 0}
              </strong>
            </div>
          </div>
          <p>
            {selectedWorkspace
              ? `已选 ${getWorkspaceDisplayName(selectedWorkspace)}`
              : '选择工作空间后查看成员'}
          </p>
        </div>
        <span>
          {selectedWorkspace
            ? `已展开 ${workspaceMembers.length} 位成员`
            : '未选择空间'}
        </span>
      </article>

      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>筛选工作空间</h2>
          <p>按空间名称或描述搜索工作空间。</p>
        </div>
        <div className="flex items-center gap-[8px]">
          <input
            aria-label="搜索工作空间"
            className="h-[32px] w-[220px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[13px] outline-none"
            placeholder="搜索工作空间"
            value={workspaceKeyword}
            onChange={event => onKeywordChange(event.target.value)}
          />
          <button
            aria-label="执行工作空间搜索"
            className="h-[32px] rounded-[8px] bg-[#1d2333] px-[12px] text-[13px] text-white disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isLoading}
            type="button"
            onClick={() => void onSearch()}
          >
            搜索
          </button>
          <button
            aria-label="刷新工作空间列表"
            className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px] disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isLoading || !onRefresh}
            type="button"
            onClick={() => void onRefresh?.()}
          >
            刷新
          </button>
        </div>
      </article>

      <article className="coze-prototype-workspace-settings-row">
        <div className="w-full">
          <h2>工作空间列表</h2>
          <p>
            第 {workspacePage} 页，共 {workspaceTotal} 个工作空间。
          </p>
          {isLoading ? <p>正在加载工作空间...</p> : null}
          <div className="mt-[12px] overflow-x-auto rounded-[12px] border border-[#e7ebf3]">
            <table className="w-full min-w-[940px] border-collapse text-left text-[13px]">
              <thead className="bg-[#f8fafc] text-[#687385]">
                <tr>
                  <th className="px-[14px] py-[10px] font-medium">工作空间</th>
                  <th className="px-[14px] py-[10px] font-medium">类型</th>
                  <th className="px-[14px] py-[10px] font-medium">所有者</th>
                  <th className="px-[14px] py-[10px] font-medium">成员数</th>
                  <th className="px-[14px] py-[10px] font-medium">创建时间</th>
                  <th className="px-[14px] py-[10px] font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {workspaces.length === 0 ? (
                  <tr>
                    <td
                      className="px-[14px] py-[18px] text-[#687385]"
                      colSpan={6}
                    >
                      暂无工作空间，当前筛选条件下没有空间。
                    </td>
                  </tr>
                ) : null}
                {workspaces.map(workspace => (
                  <tr className="border-t border-[#edf0f5]" key={workspace.id}>
                    <td className="px-[14px] py-[12px]">
                      <strong className="block text-[#1d2333]">
                        {getWorkspaceDisplayName(workspace)}
                      </strong>
                      <span className="text-[12px] text-[#687385]">
                        {getWorkspaceDisplayDescription(workspace)}
                      </span>
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      <span className="rounded-full bg-[#f1f4f9] px-[8px] py-[4px] text-[12px] text-[#4d566a]">
                        {getWorkspaceTypeLabel(workspace)}
                      </span>
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      {workspace.owner_name || workspace.owner_user_id}
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      {workspace.total_member_num ?? 0} 人
                    </td>
                    <td className="px-[14px] py-[12px] text-[#4d566a]">
                      {formatAdminTime(workspace.created_at)}
                    </td>
                    <td className="px-[14px] py-[12px]">
                      <div className="flex flex-wrap gap-[8px]">
                        <button
                          aria-label={`查看工作空间成员-${workspace.id}`}
                          className="h-[30px] rounded-[8px] border border-[#d8dde8] px-[10px] text-[12px]"
                          type="button"
                          onClick={() => void onShowWorkspaceMembers(workspace)}
                        >
                          查看成员
                        </button>
                        {onOpenWorkspace ? (
                          <button
                            aria-label={`进入工作空间-${workspace.id}`}
                            className="h-[30px] rounded-[8px] bg-[#1d2333] px-[10px] text-[12px] text-white"
                            type="button"
                            onClick={() => void onOpenWorkspace(workspace.id)}
                          >
                            进入空间
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
        <span>{workspaces.length} 个空间</span>
      </article>

      {selectedWorkspace ? (
        <article className="coze-prototype-workspace-settings-row">
          <div className="w-full">
            <h2>成员详情</h2>
            <p>{getWorkspaceDisplayName(selectedWorkspace)}</p>
            <div className="mt-[8px] flex flex-wrap gap-[8px] text-[12px] text-[#4d566a]">
              <span className="rounded-full bg-[#f1f4f9] px-[8px] py-[4px]">
                {getWorkspaceTypeLabel(selectedWorkspace)}
              </span>
              <span className="rounded-full bg-[#f1f4f9] px-[8px] py-[4px]">
                所有者：
                {selectedWorkspace.owner_name ||
                  selectedWorkspace.owner_user_id}
              </span>
              <span className="rounded-full bg-[#f1f4f9] px-[8px] py-[4px]">
                成员数：{selectedWorkspace.total_member_num ?? 0} 人
              </span>
            </div>
            {workspaceMembersLoading ? <p>正在加载成员...</p> : null}
            {workspaceMembersError ? <p>{workspaceMembersError}</p> : null}
            {!workspaceMembersLoading && workspaceMembers.length === 0 ? (
              <p>暂无成员</p>
            ) : null}
            {workspaceMembers.length > 0 ? (
              <div className="mt-[12px] overflow-x-auto rounded-[12px] border border-[#e7ebf3]">
                <table className="w-full min-w-[680px] border-collapse text-left text-[13px]">
                  <thead className="bg-[#f8fafc] text-[#687385]">
                    <tr>
                      <th className="px-[14px] py-[10px] font-medium">成员</th>
                      <th className="px-[14px] py-[10px] font-medium">邮箱</th>
                      <th className="px-[14px] py-[10px] font-medium">角色</th>
                      <th className="px-[14px] py-[10px] font-medium">
                        加入时间
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {workspaceMembers.map(member => (
                      <tr
                        className="border-t border-[#edf0f5]"
                        key={member.user_id}
                      >
                        <td className="px-[14px] py-[12px] text-[#1d2333]">
                          {getMemberDisplayName(member)}
                        </td>
                        <td className="px-[14px] py-[12px] text-[#4d566a]">
                          {member.email || '未绑定邮箱'}
                        </td>
                        <td className="px-[14px] py-[12px] text-[#4d566a]">
                          {getRoleLabel(member.role_type)}
                        </td>
                        <td className="px-[14px] py-[12px] text-[#4d566a]">
                          {formatAdminTime(member.joined_at)}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : null}
          </div>
          <span>{workspaceMembers.length} 人</span>
        </article>
      ) : null}

      <article className="coze-prototype-workspace-settings-row">
        <div>
          <h2>分页</h2>
          <p>
            第 {workspacePage} 页，共 {workspaceTotal} 个工作空间。
          </p>
        </div>
        <div className="flex items-center gap-[8px]">
          <button
            aria-label="上一页工作空间"
            className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px]"
            disabled={isLoading || workspacePage <= 1}
            type="button"
            onClick={() => void onTurnPage(workspacePage - 1)}
          >
            上一页
          </button>
          <button
            aria-label="下一页工作空间"
            className="h-[32px] rounded-[8px] border border-[#d8dde8] px-[12px] text-[13px]"
            disabled={isLoading || workspacePage * pageSize >= workspaceTotal}
            type="button"
            onClick={() => void onTurnPage(workspacePage + 1)}
          >
            下一页
          </button>
        </div>
      </article>
    </section>
  );
};
