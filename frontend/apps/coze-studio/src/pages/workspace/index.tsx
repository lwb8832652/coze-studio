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

import { useParams } from 'react-router-dom';
import { useMemo, useEffect, useState } from 'react';

import { useSpaceStore } from '@coze-foundation/space-store';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import { Input, Loading, Modal, Toast } from '@coze-arch/coze-design';
import { SpaceType } from '@coze-arch/bot-api/developer_api';

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import '../../components/workspace-prototype.less';
import {
  addWorkspaceMembers,
  deleteWorkspace,
  getWorkspaceDetail,
  listWorkspaceMembers,
  removeWorkspaceMember,
  searchWorkspaceUsers,
  transferWorkspace,
  updateWorkspace,
  updateWorkspaceMemberRole,
  type WorkspaceDetail,
  type WorkspaceMember,
  type WorkspaceUserCandidate,
} from './service';

const ROLE_OWNER = 1;
const ROLE_ADMIN = 2;
const ROLE_MEMBER = 3;
const MEMBER_PAGE_SIZE = 10;

const getWorkspaceTypeText = (spaceType?: number | SpaceType) =>
  spaceType === SpaceType.Personal ? '个人空间' : '团队空间';

const getWorkspaceRoleText = (roleType?: number) => {
  switch (roleType) {
    case ROLE_OWNER:
      return '所有者';
    case ROLE_ADMIN:
      return '管理员';
    case ROLE_MEMBER:
      return '成员';
    default:
      return '未知';
  }
};

const getWorkspaceRoleClassName = (roleType?: number) => {
  switch (roleType) {
    case ROLE_OWNER:
      return 'coze-prototype-team-role-owner';
    case ROLE_ADMIN:
      return 'coze-prototype-team-role-admin';
    default:
      return 'coze-prototype-team-role-member';
  }
};

const formatJoinedAt = (joinedAt?: number) => {
  if (!joinedAt) {
    return '-';
  }
  const timestamp = joinedAt < 1000000000000 ? joinedAt * 1000 : joinedAt;
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(timestamp));
};

const matchesMemberKeyword = (member: WorkspaceMember, keyword: string) => {
  const normalized = keyword.trim().toLowerCase();
  if (!normalized) {
    return true;
  }

  return [member.name, member.user_unique_name, member.email, member.user_id]
    .filter(Boolean)
    .some(value => value?.toLowerCase().includes(normalized));
};

const candidateDisplayName = (candidate: WorkspaceUserCandidate) =>
  candidate.name ||
  candidate.email ||
  candidate.user_unique_name ||
  candidate.user_id;

const memberDisplayName = (member: WorkspaceMember) =>
  member.name || member.email || member.user_unique_name || member.user_id;

const WorkspacePage = () => {
  const { space_id } = useParams();
  const userInfo = useUserInfo();
  const currentSpace = useSpaceStore(state => state.space);
  const spaceList = useSpaceStore(state => state.spaceList);
  const [detail, setDetail] = useState<WorkspaceDetail>();
  const [members, setMembers] = useState<WorkspaceMember[]>([]);
  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [memberKeyword, setMemberKeyword] = useState('');
  const [roleFilter, setRoleFilter] = useState(0);
  const [memberPage, setMemberPage] = useState(1);
  const [activeTab, setActiveTab] = useState<'members' | 'settings'>('members');
  const [addMemberOpen, setAddMemberOpen] = useState(false);
  const [editWorkspaceOpen, setEditWorkspaceOpen] = useState(false);
  const [removeCandidate, setRemoveCandidate] = useState<WorkspaceMember>();
  const [transferOpen, setTransferOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [transferTargetUserID, setTransferTargetUserID] = useState('');
  const [deleteConfirmName, setDeleteConfirmName] = useState('');
  const [userKeyword, setUserKeyword] = useState('');
  const [userSearchLoading, setUserSearchLoading] = useState(false);
  const [userCandidates, setUserCandidates] = useState<
    WorkspaceUserCandidate[]
  >([]);
  const [selectedUsers, setSelectedUsers] = useState<WorkspaceUserCandidate[]>(
    [],
  );
  const [addMemberError, setAddMemberError] = useState('');
  const [addingMembers, setAddingMembers] = useState(false);
  const [savingWorkspace, setSavingWorkspace] = useState(false);
  const [updatingMemberRoleUserID, setUpdatingMemberRoleUserID] = useState('');
  const [removingMemberUserID, setRemovingMemberUserID] = useState('');
  const [transferringWorkspace, setTransferringWorkspace] = useState(false);
  const [deletingWorkspace, setDeletingWorkspace] = useState(false);
  const [savingSettingKey, setSavingSettingKey] = useState<
    'allow_develop' | 'receive_publish' | ''
  >('');
  const [editName, setEditName] = useState('');
  const [editDescription, setEditDescription] = useState('');
  const resolvedSpace =
    spaceList.find(space => space.id === space_id) || currentSpace;
  const workspaceSpaceType = detail?.space_type ?? resolvedSpace?.space_type;
  const rawWorkspaceName =
    detail?.name ||
    resolvedSpace?.name ||
    userInfo?.name ||
    userInfo?.screen_name ||
    '工作空间';
  const isPersonal = workspaceSpaceType === SpaceType.Personal;
  const workspaceName = isPersonal ? '个人空间' : rawWorkspaceName;
  const workspaceType = getWorkspaceTypeText(workspaceSpaceType);
  const currentRole = detail?.current_user_role;
  const canManageMembers =
    !isPersonal && (currentRole === ROLE_OWNER || currentRole === ROLE_ADMIN);
  const canEditWorkspace =
    !isPersonal && (currentRole === ROLE_OWNER || currentRole === ROLE_ADMIN);
  const canViewSpaceSetting = !isPersonal && currentRole === ROLE_OWNER;

  const refreshWorkspace = async () => {
    if (!space_id) {
      return;
    }
    setLoading(true);
    setErrorMessage('');
    try {
      const [detailResp, membersResp] = await Promise.all([
        getWorkspaceDetail(space_id),
        listWorkspaceMembers({
          space_id,
        }),
      ]);
      setDetail(detailResp.data);
      setMembers(membersResp.members);
    } catch (error) {
      setErrorMessage('加载工作空间失败');
      Toast.error({ content: '加载工作空间失败' });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void refreshWorkspace();
  }, [space_id]);

  useEffect(() => {
    if (!detail) {
      return;
    }
    setEditName(detail.name || '');
    setEditDescription(detail.description || '');
  }, [detail]);

  const selectedUserIDSet = useMemo(
    () => new Set(selectedUsers.map(user => user.user_id)),
    [selectedUsers],
  );

  const currentMemberIDSet = useMemo(
    () => new Set(members.map(member => member.user_id)),
    [members],
  );

  const visibleCandidates = userCandidates.filter(
    candidate =>
      !currentMemberIDSet.has(candidate.user_id) &&
      !selectedUserIDSet.has(candidate.user_id),
  );

  const visibleMembers = members.filter(
    member =>
      matchesMemberKeyword(member, memberKeyword) &&
      (roleFilter === 0 || member.role_type === roleFilter),
  );
  const memberPageCount = Math.max(
    1,
    Math.ceil(visibleMembers.length / MEMBER_PAGE_SIZE),
  );
  const paginatedVisibleMembers = visibleMembers.slice(
    (memberPage - 1) * MEMBER_PAGE_SIZE,
    memberPage * MEMBER_PAGE_SIZE,
  );
  const transferCandidates = members.filter(
    member => member.role_type !== ROLE_OWNER,
  );

  useEffect(() => {
    setMemberPage(1);
  }, [memberKeyword, roleFilter]);

  useEffect(() => {
    if (memberPage > memberPageCount) {
      setMemberPage(memberPageCount);
    }
  }, [memberPage, memberPageCount]);

  const openEditWorkspace = () => {
    setEditName(workspaceName);
    setEditDescription(detail?.description || '');
    setEditWorkspaceOpen(true);
  };

  const resetAddMemberModal = () => {
    setUserKeyword('');
    setUserCandidates([]);
    setSelectedUsers([]);
    setAddMemberError('');
    setAddingMembers(false);
  };

  const handleSearchUsers = async () => {
    if (!space_id) {
      return;
    }
    const normalized = userKeyword.trim();
    if (!normalized) {
      setAddMemberError('请输入用户昵称、邮箱或用户名后搜索');
      return;
    }
    setUserSearchLoading(true);
    setAddMemberError('');
    try {
      const resp = await searchWorkspaceUsers({
        space_id,
        keyword: normalized,
        limit: 30,
      });
      setUserCandidates(resp.users || []);
      if ((resp.users || []).length === 0) {
        setAddMemberError('未找到匹配用户');
      }
    } catch (error) {
      setAddMemberError('搜索用户失败');
      Toast.error({ content: '搜索用户失败' });
    } finally {
      setUserSearchLoading(false);
    }
  };

  const selectUser = (candidate: WorkspaceUserCandidate) => {
    setSelectedUsers(prev => [
      ...prev,
      {
        ...candidate,
        role_type: candidate.role_type || ROLE_MEMBER,
      },
    ]);
    setAddMemberError('');
  };

  const selectAllVisibleCandidates = () => {
    setSelectedUsers(prev => [
      ...prev,
      ...visibleCandidates.map(candidate => ({
        ...candidate,
        role_type: candidate.role_type || ROLE_MEMBER,
      })),
    ]);
    setAddMemberError('');
  };

  const removeSelectedUser = (userID: string) => {
    setSelectedUsers(prev => prev.filter(user => user.user_id !== userID));
  };

  const changeSelectedUserRole = (userID: string, roleType: number) => {
    setSelectedUsers(prev =>
      prev.map(user =>
        user.user_id === userID
          ? {
              ...user,
              role_type: roleType,
            }
          : user,
      ),
    );
  };

  const submitAddMembers = async () => {
    if (addingMembers) {
      return;
    }

    if (!space_id || selectedUsers.length === 0) {
      setAddMemberError('请先选择要加入工作空间的成员');
      return;
    }

    setAddingMembers(true);
    setAddMemberError('');
    try {
      await addWorkspaceMembers({
        space_id,
        members: selectedUsers.map(user => ({
          user_id: user.user_id,
          role_type: user.role_type || ROLE_MEMBER,
        })),
      });
      Toast.success({ content: '成员添加成功' });
      setAddMemberOpen(false);
      resetAddMemberModal();
      await refreshWorkspace();
    } catch (error) {
      setAddMemberError('添加成员失败');
      Toast.error({ content: '添加成员失败' });
    } finally {
      setAddingMembers(false);
    }
  };

  const changeMemberRole = async (
    member: WorkspaceMember,
    roleType: number,
  ) => {
    if (
      !space_id ||
      roleType === member.role_type ||
      updatingMemberRoleUserID === member.user_id
    ) {
      return;
    }

    const previousMembers = members;
    setUpdatingMemberRoleUserID(member.user_id);
    setMembers(prev =>
      prev.map(item =>
        item.user_id === member.user_id
          ? {
              ...item,
              role_type: roleType,
            }
          : item,
      ),
    );
    try {
      await updateWorkspaceMemberRole({
        space_id,
        user_id: member.user_id,
        role_type: roleType,
      });
      Toast.success({ content: '成员角色已更新' });
      await refreshWorkspace();
    } catch (error) {
      setMembers(previousMembers);
      Toast.error({ content: '成员角色更新失败' });
      await refreshWorkspace();
    } finally {
      setUpdatingMemberRoleUserID('');
    }
  };

  const submitRemoveMember = async () => {
    if (!space_id || !removeCandidate || removingMemberUserID) {
      return;
    }
    setRemovingMemberUserID(removeCandidate.user_id);
    try {
      await removeWorkspaceMember({
        space_id,
        user_id: removeCandidate.user_id,
      });
      Toast.success({ content: '成员已移除' });
      setRemoveCandidate(undefined);
      await refreshWorkspace();
    } catch (error) {
      Toast.error({ content: '移除成员失败' });
    } finally {
      setRemovingMemberUserID('');
    }
  };

  const submitUpdateWorkspace = async () => {
    if (savingWorkspace) {
      return;
    }

    if (!space_id) {
      return;
    }
    const normalizedName = editName.trim();
    if (!normalizedName) {
      Toast.error({ content: '工作空间名称不能为空' });
      return;
    }

    setSavingWorkspace(true);
    try {
      await updateWorkspace({
        space_id,
        name: normalizedName,
        description: editDescription.trim(),
      });
      Toast.success({ content: '工作空间资料已更新' });
      setEditWorkspaceOpen(false);
      await refreshWorkspace();
    } catch (error) {
      Toast.error({ content: '更新工作空间失败' });
    } finally {
      setSavingWorkspace(false);
    }
  };

  const submitWorkspaceSetting = async (
    key: 'allow_develop' | 'receive_publish',
    checked: boolean,
  ) => {
    if (!space_id || savingSettingKey) {
      return;
    }

    const previousDetail = detail;
    setSavingSettingKey(key);
    setDetail(prev =>
      prev
        ? {
            ...prev,
            [key]: checked,
          }
        : prev,
    );
    try {
      await updateWorkspace({
        space_id,
        [key]: checked,
      });
      Toast.success({ content: '空间设置已更新' });
      await refreshWorkspace();
    } catch (error) {
      setDetail(previousDetail);
      Toast.error({ content: '更新空间设置失败' });
    } finally {
      setSavingSettingKey('');
    }
  };

  const submitTransferWorkspace = async () => {
    if (transferringWorkspace) {
      return;
    }

    if (!space_id || !transferTargetUserID) {
      Toast.error({ content: '请选择新的空间所有者' });
      return;
    }
    setTransferringWorkspace(true);
    try {
      await transferWorkspace({
        space_id,
        target_user_id: transferTargetUserID,
      });
      Toast.success({ content: '工作空间已转让' });
      setTransferOpen(false);
      setTransferTargetUserID('');
      await refreshWorkspace();
    } catch (error) {
      Toast.error({ content: '转让工作空间失败' });
    } finally {
      setTransferringWorkspace(false);
    }
  };

  const submitDeleteWorkspace = async () => {
    if (deletingWorkspace) {
      return;
    }

    if (!space_id || deleteConfirmName !== workspaceName) {
      Toast.error({ content: '请输入完整工作空间名称确认删除' });
      return;
    }
    setDeletingWorkspace(true);
    try {
      await deleteWorkspace({
        space_id,
      });
      Toast.success({ content: '工作空间已删除' });
      setDeleteOpen(false);
      window.location.href = '/';
    } catch (error) {
      Toast.error({ content: '删除工作空间失败' });
      setDeletingWorkspace(false);
    }
  };

  return (
    <main className="coze-prototype-page coze-prototype-team-setting-page">
      <WorkspacePageTopBar />
      <section className="coze-prototype-page-inner">
        <Loading loading={loading} />
        <header className="coze-prototype-team-summary">
          <div className="coze-prototype-team-avatar" aria-hidden>
            {detail?.icon_url ? (
              <img src={detail.icon_url} alt="" />
            ) : (
              <span>{workspaceName.slice(0, 1)}</span>
            )}
          </div>
          <div className="coze-prototype-team-summary-main">
            <div className="coze-prototype-team-title-row">
              <h1>{workspaceName}</h1>
              {canEditWorkspace ? (
                <button
                  className="coze-prototype-team-edit-button"
                  type="button"
                  onClick={openEditWorkspace}
                >
                  编辑资料
                </button>
              ) : null}
            </div>
            <p>
              我的身份：
              <span className={getWorkspaceRoleClassName(currentRole)}>
                {getWorkspaceRoleText(currentRole)}
              </span>
              <span className="coze-prototype-team-summary-divider">/</span>
              {workspaceType}
              {!isPersonal && detail?.description
                ? ` / ${detail.description}`
                : ''}
            </p>
          </div>
        </header>
        {errorMessage ? (
          <section className="mb-[16px] rounded-[10px] border border-[#ffd2d2] bg-[#fff5f5] px-[14px] py-[10px] text-[13px] text-[#c33131]">
            {errorMessage}
          </section>
        ) : null}

        <section className="coze-prototype-team-tabs" aria-label="工作空间设置">
          <button
            className="coze-prototype-team-tab"
            data-active={activeTab === 'members'}
            type="button"
            onClick={() => setActiveTab('members')}
          >
            成员管理
          </button>
          {canViewSpaceSetting ? (
            <button
              className="coze-prototype-team-tab"
              data-active={activeTab === 'settings'}
              type="button"
              onClick={() => setActiveTab('settings')}
            >
              空间设置
            </button>
          ) : null}
        </section>

        {activeTab === 'members' ? (
          <section className="coze-prototype-member-manage-panel">
            <div className="coze-prototype-member-panel-head">
              <div>
                <h2>成员管理</h2>
                <p>
                  {isPersonal
                    ? '个人空间用于承载你的个人任务和资源，成员信息仅供查看。'
                    : '管理当前团队空间的协作者、角色和访问权限，成员变更会即时生效。'}
                </p>
              </div>
              <span>
                共 {members.length} 位成员
                {canManageMembers ? '，可添加新成员' : '，仅可查看'}
              </span>
            </div>
            {!canManageMembers ? (
              <div className="coze-prototype-member-readonly-banner">
                {isPersonal
                  ? '个人空间暂不支持成员邀请、角色调整和移除操作。如需协作，请创建或切换到团队空间。'
                  : '当前身份暂无成员管理权限，可以查看成员列表；如需调整角色或成员，请联系空间所有者或管理员。'}
              </div>
            ) : null}
            <div className="coze-prototype-member-toolbar">
              <div className="coze-prototype-member-search-group">
                <input
                  aria-label="搜索工作空间成员"
                  className="coze-prototype-member-search"
                  placeholder="搜索成员昵称、邮箱或用户名"
                  value={memberKeyword}
                  onChange={event => setMemberKeyword(event.target.value)}
                />
                <select
                  aria-label="筛选成员角色"
                  className="coze-prototype-member-role-filter"
                  value={roleFilter}
                  onChange={event => setRoleFilter(Number(event.target.value))}
                >
                  <option value={0}>全部角色</option>
                  <option value={ROLE_OWNER}>所有者</option>
                  <option value={ROLE_ADMIN}>管理员</option>
                  <option value={ROLE_MEMBER}>成员</option>
                </select>
              </div>
              {canManageMembers ? (
                <button
                  className="coze-prototype-member-add-button"
                  type="button"
                  onClick={() => {
                    resetAddMemberModal();
                    setAddMemberOpen(true);
                  }}
                >
                  + 添加成员
                </button>
              ) : null}
            </div>
            <div className="coze-prototype-member-table">
              <div className="coze-prototype-member-table-head">
                <span>昵称</span>
                <span>用户名</span>
                <span>角色</span>
                <span>加入时间</span>
                <span>操作</span>
              </div>
              {members.length === 0 ? (
                <div className="coze-prototype-member-empty">暂无成员</div>
              ) : null}
              {members.length > 0 && visibleMembers.length === 0 ? (
                <div className="coze-prototype-member-empty">
                  没有匹配的成员
                </div>
              ) : null}
              {paginatedVisibleMembers.map(member => {
                const isOwner = member.role_type === ROLE_OWNER;
                const canOperateMember =
                  canManageMembers &&
                  !isOwner &&
                  member.user_id !== detail?.owner_user_id;

                return (
                  <div
                    key={member.user_id}
                    className="coze-prototype-member-row"
                  >
                    <div className="coze-prototype-member-profile">
                      <div className="coze-prototype-member-avatar">
                        {member.avatar_url ? (
                          <img src={member.avatar_url} alt="" />
                        ) : (
                          <span>{memberDisplayName(member).slice(0, 1)}</span>
                        )}
                      </div>
                      <div>
                        <strong>{memberDisplayName(member)}</strong>
                        <small>{member.email || '未绑定邮箱'}</small>
                      </div>
                    </div>
                    <span>{member.user_unique_name || member.user_id}</span>
                    <span>
                      {canOperateMember ? (
                        <select
                          aria-label={`调整 ${memberDisplayName(member)} 的角色`}
                          className="coze-prototype-inline-role-select"
                          disabled={
                            updatingMemberRoleUserID === member.user_id ||
                            Boolean(removingMemberUserID)
                          }
                          value={member.role_type}
                          onChange={event =>
                            void changeMemberRole(
                              member,
                              Number(event.target.value),
                            )
                          }
                        >
                          <option value={ROLE_ADMIN}>管理员</option>
                          <option value={ROLE_MEMBER}>成员</option>
                        </select>
                      ) : (
                        <span
                          className={`coze-prototype-team-role-pill ${getWorkspaceRoleClassName(
                            member.role_type,
                          )}`}
                        >
                          {getWorkspaceRoleText(member.role_type)}
                        </span>
                      )}
                    </span>
                    <span>{formatJoinedAt(member.joined_at)}</span>
                    <span>
                      {canOperateMember ? (
                        <button
                          className="coze-prototype-member-danger-action"
                          disabled={
                            removingMemberUserID === member.user_id ||
                            updatingMemberRoleUserID === member.user_id
                          }
                          type="button"
                          onClick={() => setRemoveCandidate(member)}
                        >
                          {removingMemberUserID === member.user_id
                            ? '移除中'
                            : '移除'}
                        </button>
                      ) : (
                        <span className="coze-prototype-member-muted-action">
                          -
                        </span>
                      )}
                    </span>
                  </div>
                );
              })}
            </div>
            <div className="coze-prototype-member-pagination">
              <span>
                共 {visibleMembers.length} 位成员 · 第 {memberPage} /{' '}
                {memberPageCount} 页
              </span>
              <div>
                <button
                  aria-label="上一页成员"
                  disabled={memberPage <= 1}
                  type="button"
                  onClick={() => setMemberPage(page => Math.max(1, page - 1))}
                >
                  上一页
                </button>
                <button
                  aria-label="下一页成员"
                  disabled={memberPage >= memberPageCount}
                  type="button"
                  onClick={() =>
                    setMemberPage(page => Math.min(memberPageCount, page + 1))
                  }
                >
                  下一页
                </button>
              </div>
            </div>
          </section>
        ) : null}

        {activeTab === 'settings' ? (
          <section className="coze-prototype-space-setting-panel">
            <article className="coze-prototype-space-setting-section">
              <h2>转让空间</h2>
              <p>将空间所有者转让给其他成员，转让后你会变为管理员。</p>
              <button
                className="coze-prototype-space-setting-primary"
                type="button"
                onClick={() => setTransferOpen(true)}
              >
                转让空间
              </button>
            </article>
            <article className="coze-prototype-space-setting-section">
              <h2>删除空间</h2>
              <p>删除后空间将不可见，请确认所有成员和资源归属已经处理。</p>
              <button
                className="coze-prototype-space-setting-danger"
                type="button"
                onClick={() => {
                  setDeleteConfirmName('');
                  setDeleteOpen(true);
                }}
              >
                删除空间
              </button>
            </article>
            <article className="coze-prototype-space-setting-section">
              <h2>开发者功能</h2>
              <p>
                关闭后，所有成员将无法访问&quot;资源配置&quot;、&quot;网页应用开发&quot;、&quot;技能配置&quot;和&quot;开发配置&quot;。
              </p>
              <label className="coze-prototype-space-setting-switch">
                <input
                  aria-label="开发者功能"
                  checked={detail?.allow_develop ?? true}
                  disabled={Boolean(savingSettingKey)}
                  type="checkbox"
                  onChange={event =>
                    void submitWorkspaceSetting(
                      'allow_develop',
                      event.target.checked,
                    )
                  }
                />
                <span />
              </label>
            </article>
            <article className="coze-prototype-space-setting-section">
              <h2>接受来自外部空间的发布</h2>
              <p>
                打开后，拥有权限的用户可以将其他空间完成开发的智能体、插件、工作流发布到当前团队空间的协作资源中。
              </p>
              <label className="coze-prototype-space-setting-switch">
                <input
                  aria-label="接受来自外部空间的发布"
                  checked={detail?.receive_publish ?? false}
                  disabled={Boolean(savingSettingKey)}
                  type="checkbox"
                  onChange={event =>
                    void submitWorkspaceSetting(
                      'receive_publish',
                      event.target.checked,
                    )
                  }
                />
                <span />
              </label>
            </article>
          </section>
        ) : null}
      </section>

      <Modal
        title="添加成员"
        visible={addMemberOpen}
        okText={addingMembers ? '添加中...' : '确认添加'}
        cancelText="取消"
        maskClosable={false}
        onOk={() => void submitAddMembers()}
        onCancel={() => {
          setAddMemberOpen(false);
          resetAddMemberModal();
        }}
      >
        <div className="coze-prototype-add-member-modal">
          <div className="coze-prototype-add-member-left">
            <div className="coze-prototype-add-member-search-row">
              <Input
                value={userKeyword}
                placeholder="搜索用户昵称、邮箱或用户名"
                onChange={(value: string) => {
                  setUserKeyword(value);
                  setAddMemberError('');
                }}
              />
              <button
                className="coze-prototype-add-member-search-button"
                disabled={userSearchLoading}
                type="button"
                onClick={() => void handleSearchUsers()}
              >
                {userSearchLoading ? '搜索中' : '搜索'}
              </button>
            </div>
            <button
              className="coze-prototype-add-member-check-all"
              type="button"
              disabled={visibleCandidates.length === 0 || addingMembers}
              onClick={selectAllVisibleCandidates}
            >
              全选
            </button>
            <div className="coze-prototype-add-member-list">
              {visibleCandidates.length === 0 ? (
                <div className="coze-prototype-add-member-empty">
                  {userSearchLoading
                    ? '正在搜索用户...'
                    : userKeyword.trim()
                      ? '没有可添加的用户'
                      : '搜索并选择要加入的成员'}
                </div>
              ) : null}
              {visibleCandidates.map(candidate => (
                <button
                  key={candidate.user_id}
                  className="coze-prototype-add-member-option"
                  disabled={addingMembers}
                  type="button"
                  onClick={() => selectUser(candidate)}
                >
                  <span className="coze-prototype-add-member-checkbox" />
                  <span className="coze-prototype-member-avatar">
                    {candidate.avatar_url ? (
                      <img src={candidate.avatar_url} alt="" />
                    ) : (
                      <span>{candidateDisplayName(candidate).slice(0, 1)}</span>
                    )}
                  </span>
                  <span className="coze-prototype-add-member-name">
                    {candidateDisplayName(candidate)}
                    <small>
                      {candidate.email || candidate.user_unique_name}
                    </small>
                  </span>
                </button>
              ))}
            </div>
          </div>
          <div className="coze-prototype-add-member-right">
            <h3>已选成员（{selectedUsers.length}）</h3>
            {selectedUsers.length === 0 ? (
              <div className="coze-prototype-add-member-empty">
                右侧会显示已选成员和角色
              </div>
            ) : null}
            {selectedUsers.map(user => (
              <div
                key={user.user_id}
                className="coze-prototype-add-member-selected-row"
              >
                <span className="coze-prototype-member-avatar">
                  {user.avatar_url ? (
                    <img src={user.avatar_url} alt="" />
                  ) : (
                    <span>{candidateDisplayName(user).slice(0, 1)}</span>
                  )}
                </span>
                <span className="coze-prototype-add-member-selected-name">
                  {candidateDisplayName(user)}
                </span>
                <select
                  aria-label={`设置 ${candidateDisplayName(user)} 的角色`}
                  disabled={addingMembers}
                  value={user.role_type || ROLE_MEMBER}
                  onChange={event =>
                    changeSelectedUserRole(
                      user.user_id,
                      Number(event.target.value),
                    )
                  }
                >
                  <option value={ROLE_ADMIN}>管理员</option>
                  <option value={ROLE_MEMBER}>成员</option>
                </select>
                <button
                  className="coze-prototype-add-member-remove"
                  disabled={addingMembers}
                  type="button"
                  onClick={() => removeSelectedUser(user.user_id)}
                >
                  ×
                </button>
              </div>
            ))}
          </div>
          {addMemberError ? (
            <div className="coze-prototype-add-member-error">
              {addMemberError}
            </div>
          ) : null}
        </div>
      </Modal>

      <Modal
        title="编辑工作空间资料"
        visible={editWorkspaceOpen}
        okText={savingWorkspace ? '保存中...' : '保存'}
        cancelText="取消"
        maskClosable={false}
        onOk={() => void submitUpdateWorkspace()}
        onCancel={() => setEditWorkspaceOpen(false)}
      >
        <div className="coze-prototype-edit-workspace-form">
          <div className="coze-prototype-edit-workspace-intro">
            <span className="coze-prototype-edit-workspace-avatar" aria-hidden>
              {workspaceName.slice(0, 1)}
            </span>
            <p>
              通过创建团队空间，将支持智能体、插件、工作流、大模型和知识库在团队内进行协作和分享。
            </p>
          </div>
          <label>
            <span>工作空间名称</span>
            <Input
              value={editName}
              maxLength={50}
              placeholder="请输入工作空间名称"
              onChange={(value: string) => setEditName(value)}
            />
          </label>
          <label>
            <span>工作空间描述</span>
            <textarea
              value={editDescription}
              maxLength={2000}
              placeholder="描述这个工作空间的协作目标"
              onChange={event => setEditDescription(event.target.value)}
            />
          </label>
        </div>
      </Modal>

      <Modal
        title="删除成员"
        visible={Boolean(removeCandidate)}
        okText={removingMemberUserID ? '移除中...' : '移除'}
        cancelText="取消"
        maskClosable={false}
        onOk={() => void submitRemoveMember()}
        onCancel={() => setRemoveCandidate(undefined)}
      >
        <p className="coze-prototype-remove-member-confirm">
          确认将「{removeCandidate ? memberDisplayName(removeCandidate) : ''}」
          从当前工作空间移除吗？
        </p>
      </Modal>

      <Modal
        title="转让空间"
        visible={transferOpen}
        okText={transferringWorkspace ? '转让中...' : '确认转让'}
        cancelText="取消"
        maskClosable={false}
        onOk={() => void submitTransferWorkspace()}
        onCancel={() => {
          setTransferOpen(false);
          setTransferTargetUserID('');
        }}
      >
        <div className="coze-prototype-transfer-space-form">
          <p>选择一位成员作为新的空间所有者。</p>
          <select
            aria-label="选择新的空间所有者"
            disabled={transferringWorkspace}
            value={transferTargetUserID}
            onChange={event => setTransferTargetUserID(event.target.value)}
          >
            <option value="">请选择成员</option>
            {transferCandidates.map(member => (
              <option key={member.user_id} value={member.user_id}>
                {memberDisplayName(member)} /{' '}
                {getWorkspaceRoleText(member.role_type)}
              </option>
            ))}
          </select>
          {transferCandidates.length === 0 ? (
            <div className="coze-prototype-create-space-hint">
              当前没有可转让的成员，请先添加成员。
            </div>
          ) : null}
        </div>
      </Modal>

      <Modal
        title="删除空间"
        visible={deleteOpen}
        okText={deletingWorkspace ? '删除中...' : '确认删除'}
        cancelText="取消"
        maskClosable={false}
        onOk={() => void submitDeleteWorkspace()}
        onCancel={() => setDeleteOpen(false)}
      >
        <div className="coze-prototype-delete-space-form">
          <p className="coze-prototype-delete-space-warning">
            删除空间后，空间内成员关系和协作入口将不可恢复，请谨慎操作。
          </p>
          <p>请输入完整工作空间名称「{workspaceName}」确认删除。</p>
          <Input
            value={deleteConfirmName}
            placeholder={workspaceName}
            onChange={(value: string) => setDeleteConfirmName(value)}
          />
        </div>
      </Modal>
    </main>
  );
};

export default WorkspacePage;
