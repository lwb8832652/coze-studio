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
/* eslint-disable max-lines -- Cohesive orchestrator. */

import { useNavigate, useParams } from 'react-router-dom';
import { useCallback, useEffect, useState } from 'react';

import { refreshSiteConfig } from '@coze-foundation/global-adapter';

import '../../components/workspace-prototype.less';
import './newx-system-ui.less';
import { WorkspaceManagementSection } from './workspace-management-section';
import { getActiveSystemSection, getSystemSectionContent } from './view-model';
import { UserManagementSection } from './user-management-section';
import { SystemSettingsSection } from './system-settings-section';
import {
  createAdminModel,
  createAdminUser,
  deleteAdminModel,
  getSystemAdminStatus,
  getAdminBasicConfig,
  getAdminKnowledgeConfig,
  getAdminModelList,
  isAdminBasicConfigConflict,
  listAdminUserSpaces,
  listAdminWorkspaceMembers,
  listAdminUsers,
  listAdminWorkspaces,
  resetAdminUserPassword,
  saveAdminBasicConfig,
  updateAdminUser,
  type AdminCreateModelPayload,
  type AdminCreateUserPayload,
  type AdminBasicConfig,
  type AdminBasicConfigPatch,
  type AdminKnowledgeConfig,
  type AdminProviderModelListItem,
  type AdminResetUserPasswordPayload,
  type AdminUpdateUserPayload,
  type AdminUserSpace,
  type AdminUser,
  type AdminWorkspace,
  type AdminWorkspaceMember,
} from './service';
import { SandboxManagementSection } from './sandbox-management-section';
import { OverviewSection } from './overview-section';
import { ModelConfigSection } from './model-config-section';
import { SYSTEM_NAV_GROUPS, SYSTEM_SECTIONS } from './content';
import {
  BillingManagementSection,
  type BillingSectionKey,
} from './billing-management-section';
import { AnnouncementsSection } from './announcements';

const ADMIN_PAGE_SIZE = 20;

const SystemManagementPage = () => {
  const { section } = useParams();
  const navigate = useNavigate();
  const [workspaces, setWorkspaces] = useState<AdminWorkspace[]>([]);
  const [workspaceTotal, setWorkspaceTotal] = useState(0);
  const [workspaceKeyword, setWorkspaceKeyword] = useState('');
  const [workspacePage, setWorkspacePage] = useState(1);
  const [selectedWorkspace, setSelectedWorkspace] =
    useState<AdminWorkspace | null>(null);
  const [workspaceMembers, setWorkspaceMembers] = useState<
    AdminWorkspaceMember[]
  >([]);
  const [workspaceMembersLoading, setWorkspaceMembersLoading] = useState(false);
  const [workspaceMembersError, setWorkspaceMembersError] = useState('');
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [userTotal, setUserTotal] = useState(0);
  const [userKeyword, setUserKeyword] = useState('');
  const [userPage, setUserPage] = useState(1);
  const [selectedUser, setSelectedUser] = useState<AdminUser | null>(null);
  const [userSpaces, setUserSpaces] = useState<AdminUserSpace[]>([]);
  const [userSpacesLoading, setUserSpacesLoading] = useState(false);
  const [userSpacesError, setUserSpacesError] = useState('');
  const [basicConfig, setBasicConfig] = useState<AdminBasicConfig | null>(null);
  const [basicConfigRevision, setBasicConfigRevision] = useState('');
  const [basicConfigLoading, setBasicConfigLoading] = useState(true);
  const [basicConfigLoadError, setBasicConfigLoadError] = useState('');
  const [basicConfigRefreshRequired, setBasicConfigRefreshRequired] =
    useState(false);
  const [modelProviders, setModelProviders] = useState<
    AdminProviderModelListItem[]
  >([]);
  const [knowledgeConfig, setKnowledgeConfig] = useState<AdminKnowledgeConfig>(
    {},
  );
  const [loading, setLoading] = useState(false);
  const [accessDenied, setAccessDenied] = useState(false);
  const [adminResolved, setAdminResolved] = useState(false);
  const [isSystemAdmin, setIsSystemAdmin] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [modelSaving, setModelSaving] = useState(false);
  const [modelRefreshing, setModelRefreshing] = useState(false);
  const [modelMessage, setModelMessage] = useState('');
  const [basicConfigSaving, setBasicConfigSaving] = useState(false);
  const [basicConfigMessage, setBasicConfigMessage] = useState('');
  const [userMutating, setUserMutating] = useState(false);
  const [userActionMessage, setUserActionMessage] = useState('');
  const activeSection = getActiveSystemSection(section);
  const content = getSystemSectionContent(section);

  const loadBasicConfiguration = useCallback(async () => {
    setBasicConfigLoading(true);
    setBasicConfigLoadError('');
    try {
      const response = await getAdminBasicConfig();
      setBasicConfig(response.configuration);
      setBasicConfigRevision(response.revision);
      setBasicConfigRefreshRequired(false);
    } catch (error) {
      setBasicConfig(null);
      setBasicConfigRevision('');
      setBasicConfigLoadError('加载系统基础配置失败，请重试');
    } finally {
      setBasicConfigLoading(false);
    }
  }, []);

  const loadWorkspaces = async (params?: {
    keyword?: string;
    page?: number;
  }) => {
    const nextKeyword = params?.keyword ?? workspaceKeyword;
    const nextPage = params?.page ?? workspacePage;
    const workspaceResp = await listAdminWorkspaces({
      keyword: nextKeyword || undefined,
      page: nextPage,
      size: ADMIN_PAGE_SIZE,
    });
    setWorkspaces(workspaceResp.workspaces);
    setWorkspaceTotal(workspaceResp.total);
    setWorkspaceKeyword(nextKeyword);
    setWorkspacePage(nextPage);
    setSelectedWorkspace(null);
    setWorkspaceMembers([]);
    setWorkspaceMembersError('');
  };

  const loadUsers = async (params?: { keyword?: string; page?: number }) => {
    const nextKeyword = params?.keyword ?? userKeyword;
    const nextPage = params?.page ?? userPage;
    const userResp = await listAdminUsers({
      keyword: nextKeyword || undefined,
      page: nextPage,
      size: ADMIN_PAGE_SIZE,
    });
    setUsers(userResp.users);
    setUserTotal(userResp.total);
    setUserKeyword(nextKeyword);
    setUserPage(nextPage);
    setSelectedUser(null);
    setUserSpaces([]);
    setUserSpacesError('');
  };

  useEffect(() => {
    let canceled = false;

    const loadSystemData = async () => {
      let adminVerified = false;
      setLoading(true);
      setErrorMessage('');
      try {
        const adminStatus = await getSystemAdminStatus();
        if (canceled) {
          return;
        }
        if (!adminStatus.is_admin) {
          setIsSystemAdmin(false);
          setAdminResolved(true);
          setAccessDenied(true);
          return;
        }
        adminVerified = true;
        setIsSystemAdmin(true);
        setAdminResolved(true);
        setAccessDenied(false);

        const [workspaceResp, userResp, modelResp, knowledgeResp] =
          await Promise.all([
            listAdminWorkspaces({
              page: 1,
              size: ADMIN_PAGE_SIZE,
            }),
            listAdminUsers({
              page: 1,
              size: ADMIN_PAGE_SIZE,
            }),
            getAdminModelList(),
            getAdminKnowledgeConfig(),
            loadBasicConfiguration(),
          ]);
        if (canceled) {
          return;
        }
        setWorkspaces(workspaceResp.workspaces);
        setWorkspaceTotal(workspaceResp.total);
        setUsers(userResp.users);
        setUserTotal(userResp.total);
        setModelProviders(modelResp.provider_model_list ?? []);
        setKnowledgeConfig(knowledgeResp.knowledge_config ?? {});
      } catch (error) {
        if (!canceled) {
          if (!adminVerified) {
            setIsSystemAdmin(false);
            setAdminResolved(true);
            setAccessDenied(true);
          }
          setErrorMessage('加载系统管理数据失败');
        }
      } finally {
        if (!canceled) {
          setLoading(false);
        }
      }
    };

    void loadSystemData();

    return () => {
      canceled = true;
    };
  }, [loadBasicConfiguration]);

  const searchUsers = async () => {
    setLoading(true);
    setErrorMessage('');
    try {
      await loadUsers({
        keyword: userKeyword,
        page: 1,
      });
    } catch (error) {
      setErrorMessage('加载用户列表失败');
    } finally {
      setLoading(false);
    }
  };

  const turnUserPage = async (page: number) => {
    setLoading(true);
    setErrorMessage('');
    try {
      await loadUsers({
        keyword: userKeyword,
        page,
      });
    } catch (error) {
      setErrorMessage('加载用户列表失败');
    } finally {
      setLoading(false);
    }
  };

  const refreshUsers = async () => {
    setLoading(true);
    setErrorMessage('');
    try {
      await loadUsers({
        keyword: userKeyword,
        page: userPage,
      });
    } catch (error) {
      setErrorMessage('刷新用户列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateAdminUser = async (payload: AdminCreateUserPayload) => {
    setUserMutating(true);
    setUserActionMessage('');
    try {
      await createAdminUser(payload);
      await loadUsers({
        keyword: userKeyword,
        page: userPage,
      });
      setUserActionMessage('用户已新增');
    } catch (error) {
      setUserActionMessage('新增用户失败，请检查邮箱、用户名和密码。');
      throw error;
    } finally {
      setUserMutating(false);
    }
  };

  const handleUpdateAdminUser = async (payload: AdminUpdateUserPayload) => {
    setUserMutating(true);
    setUserActionMessage('');
    try {
      await updateAdminUser(payload);
      await loadUsers({
        keyword: userKeyword,
        page: userPage,
      });
      setUserActionMessage('用户信息已更新');
    } catch (error) {
      setUserActionMessage('更新用户失败，请检查用户名是否重复。');
      throw error;
    } finally {
      setUserMutating(false);
    }
  };

  const handleResetAdminUserPassword = async (
    payload: AdminResetUserPasswordPayload,
  ) => {
    setUserMutating(true);
    setUserActionMessage('');
    try {
      await resetAdminUserPassword(payload);
      setUserActionMessage('用户密码已重置');
    } catch (error) {
      setUserActionMessage('重置密码失败，请确认用户邮箱有效。');
      throw error;
    } finally {
      setUserMutating(false);
    }
  };

  const showUserSpaces = async (user: AdminUser) => {
    setSelectedUser(user);
    setUserSpaces([]);
    setUserSpacesLoading(true);
    setUserSpacesError('');
    try {
      const resp = await listAdminUserSpaces({
        user_id: user.user_id,
      });
      setUserSpaces(resp.spaces);
    } catch (error) {
      setUserSpacesError('加载用户所属工作空间失败');
    } finally {
      setUserSpacesLoading(false);
    }
  };

  const searchWorkspaces = async () => {
    setLoading(true);
    setErrorMessage('');
    try {
      await loadWorkspaces({
        keyword: workspaceKeyword,
        page: 1,
      });
    } catch (error) {
      setErrorMessage('加载工作空间列表失败');
    } finally {
      setLoading(false);
    }
  };

  const turnWorkspacePage = async (page: number) => {
    setLoading(true);
    setErrorMessage('');
    try {
      await loadWorkspaces({
        keyword: workspaceKeyword,
        page,
      });
    } catch (error) {
      setErrorMessage('加载工作空间列表失败');
    } finally {
      setLoading(false);
    }
  };

  const refreshWorkspaces = async () => {
    setLoading(true);
    setErrorMessage('');
    try {
      await loadWorkspaces({
        keyword: workspaceKeyword,
        page: workspacePage,
      });
    } catch (error) {
      setErrorMessage('刷新工作空间列表失败');
    } finally {
      setLoading(false);
    }
  };

  const showWorkspaceMembers = async (workspace: AdminWorkspace) => {
    setSelectedWorkspace(workspace);
    setWorkspaceMembers([]);
    setWorkspaceMembersLoading(true);
    setWorkspaceMembersError('');
    try {
      const resp = await listAdminWorkspaceMembers({
        space_id: workspace.id,
      });
      setWorkspaceMembers(resp.members);
    } catch (error) {
      setWorkspaceMembersError('加载工作空间成员失败');
    } finally {
      setWorkspaceMembersLoading(false);
    }
  };

  const refreshModelProviders = async () => {
    const modelResp = await getAdminModelList();
    setModelProviders(modelResp.provider_model_list ?? []);
  };

  const handleCreateModel = async (payload: AdminCreateModelPayload) => {
    setModelSaving(true);
    setModelMessage('');
    try {
      await createAdminModel(payload);
      await refreshModelProviders();
      setModelMessage('模型配置已创建');
    } catch (error) {
      setModelMessage(
        '保存模型配置失败，接口会先测试模型连通性，请确认模型标识和密钥。',
      );
      throw error;
    } finally {
      setModelSaving(false);
    }
  };

  const handleDeleteModel = async (id: number | string) => {
    if (
      typeof window !== 'undefined' &&
      window.confirm &&
      !window.confirm('确认删除该模型配置吗？')
    ) {
      return;
    }
    setModelSaving(true);
    setModelMessage('');
    try {
      await deleteAdminModel({ id });
      await refreshModelProviders();
      setModelMessage('模型配置已删除');
    } catch (error) {
      setModelMessage('删除模型配置失败');
    } finally {
      setModelSaving(false);
    }
  };

  const handleRefreshModels = async () => {
    setModelRefreshing(true);
    setModelMessage('');
    try {
      await refreshModelProviders();
      setModelMessage('模型配置已刷新');
    } catch (error) {
      setModelMessage('刷新模型配置失败');
    } finally {
      setModelRefreshing(false);
    }
  };

  const handleSaveBasicConfig = async (updates: AdminBasicConfigPatch) => {
    if (!basicConfig || !basicConfigRevision) {
      setBasicConfigMessage('系统基础配置尚未加载，请刷新后重试');
      return;
    }
    setBasicConfigSaving(true);
    setBasicConfigMessage('');
    try {
      const response = await saveAdminBasicConfig(updates, basicConfigRevision);
      setBasicConfig({ ...basicConfig, ...updates });
      setBasicConfigRevision(response.revision);
      setBasicConfigRefreshRequired(false);
      setBasicConfigMessage('系统基础配置已保存');
      if (
        'site_name' in updates ||
        'site_description' in updates ||
        'site_logo_uri' in updates ||
        'favicon_uri' in updates
      ) {
        await refreshSiteConfig();
      }
    } catch (error) {
      if (isAdminBasicConfigConflict(error)) {
        setBasicConfigRefreshRequired(true);
        setBasicConfigMessage('配置已被其他管理员更新，请刷新后重试');
      } else {
        setBasicConfigMessage('保存系统基础配置失败，请检查服务地址和配置格式');
      }
      throw error;
    } finally {
      setBasicConfigSaving(false);
    }
  };

  const reloadBasicConfiguration = async () => {
    setBasicConfigMessage('');
    setBasicConfigRefreshRequired(false);
    await loadBasicConfiguration();
  };

  const renderOverview = () => (
    <OverviewSection
      userTotal={userTotal}
      workspaceTotal={workspaceTotal}
      basicConfig={basicConfig ?? {}}
      users={users}
      workspaces={workspaces}
    />
  );

  const renderUsers = () => (
    <UserManagementSection
      pageSize={ADMIN_PAGE_SIZE}
      selectedUser={selectedUser}
      userKeyword={userKeyword}
      userPage={userPage}
      userSpaces={userSpaces}
      userSpacesError={userSpacesError}
      userSpacesLoading={userSpacesLoading}
      userTotal={userTotal}
      users={users}
      isLoading={loading}
      isUserMutating={userMutating}
      userActionMessage={userActionMessage}
      onKeywordChange={setUserKeyword}
      onCreateUser={handleCreateAdminUser}
      onOpenWorkspace={spaceID => navigate(`/space/${spaceID}/workspace`)}
      onRefresh={refreshUsers}
      onResetUserPassword={handleResetAdminUserPassword}
      onSearch={searchUsers}
      onShowUserSpaces={showUserSpaces}
      onTurnPage={turnUserPage}
      onUpdateUser={handleUpdateAdminUser}
    />
  );

  const renderWorkspaces = () => (
    <WorkspaceManagementSection
      pageSize={ADMIN_PAGE_SIZE}
      selectedWorkspace={selectedWorkspace}
      workspaceKeyword={workspaceKeyword}
      workspaceMembers={workspaceMembers}
      workspaceMembersError={workspaceMembersError}
      workspaceMembersLoading={workspaceMembersLoading}
      workspacePage={workspacePage}
      workspaceTotal={workspaceTotal}
      workspaces={workspaces}
      isLoading={loading}
      onKeywordChange={setWorkspaceKeyword}
      onOpenWorkspace={spaceID => navigate(`/space/${spaceID}/workspace`)}
      onRefresh={refreshWorkspaces}
      onSearch={searchWorkspaces}
      onShowWorkspaceMembers={showWorkspaceMembers}
      onTurnPage={turnWorkspacePage}
    />
  );

  const renderSettings = () => (
    <SystemSettingsSection
      basicConfig={basicConfig}
      basicConfigLoadError={basicConfigLoadError}
      basicConfigLoading={basicConfigLoading}
      basicConfigMessage={basicConfigMessage}
      basicConfigRefreshRequired={basicConfigRefreshRequired}
      basicConfigSaving={basicConfigSaving}
      knowledgeConfig={knowledgeConfig}
      onReloadBasicConfig={reloadBasicConfiguration}
      onSaveBasicConfig={handleSaveBasicConfig}
    />
  );

  const renderModels = () => (
    <ModelConfigSection
      modelMessage={modelMessage}
      modelProviders={modelProviders}
      modelRefreshing={modelRefreshing}
      modelSaving={modelSaving}
      onCreateModel={handleCreateModel}
      onDeleteModel={handleDeleteModel}
      onRefresh={handleRefreshModels}
    />
  );

  const renderActiveSection = () => {
    if (activeSection === 'users') {
      return renderUsers();
    }
    if (activeSection === 'workspaces') {
      return renderWorkspaces();
    }
    if (activeSection === 'settings') {
      return renderSettings();
    }
    if (activeSection === 'models') {
      return renderModels();
    }
    if (activeSection === 'sandbox') {
      return <SandboxManagementSection />;
    }
    if (activeSection === 'announcements') {
      return <AnnouncementsSection />;
    }
    if (activeSection.startsWith('billing-')) {
      return (
        <BillingManagementSection
          section={activeSection as BillingSectionKey}
        />
      );
    }
    return renderOverview();
  };

  return (
    <main className="coze-prototype-system-page">
      <div className="coze-prototype-system-shell">
        <aside className="coze-prototype-system-nav">
          <div className="coze-prototype-system-brand">
            <h1>系统管理</h1>
            <p>后台管理员控制台</p>
          </div>
          <nav className="coze-prototype-system-nav-list">
            {SYSTEM_NAV_GROUPS.map(group => {
              const items = group.items.flatMap(key => {
                const item = SYSTEM_SECTIONS.find(entry => entry.key === key);
                return item && (item.key !== 'sandbox' || isSystemAdmin)
                  ? [item]
                  : [];
              });
              const buttons = items.map(item => (
                <button
                  key={item.key}
                  type="button"
                  className={`coze-prototype-system-nav-item${group.label ? ' is-child' : ''}`}
                  data-active={activeSection === item.key}
                  onClick={() => navigate(`/system/${item.key}`)}
                >
                  <span>{item.title}</span>
                </button>
              ));
              if (!buttons.length) {
                return null;
              }
              return group.label ? (
                <div
                  className="coze-prototype-system-nav-group"
                  key={group.key}
                >
                  <span>{group.label}</span>
                  {buttons}
                </div>
              ) : (
                buttons
              );
            })}
          </nav>
        </aside>

        <section
          className="coze-prototype-system-content"
          data-section={activeSection}
        >
          {activeSection !== 'models' || !adminResolved || accessDenied ? (
            <header className="coze-prototype-system-hero">
              <h1>
                {!adminResolved || accessDenied ? '系统管理' : content.heading}
              </h1>
              <p>
                {!adminResolved
                  ? '正在验证系统管理权限...'
                  : accessDenied
                    ? '后台能力仅对系统管理员开放。'
                    : content.summary}
              </p>
            </header>
          ) : null}

          {!adminResolved || loading ? <p role="status">加载中...</p> : null}
          {errorMessage ? <p>{errorMessage}</p> : null}
          {accessDenied ? (
            <section className="coze-prototype-workspace-settings-list">
              <article className="coze-prototype-workspace-settings-row">
                <div>
                  <h2>无权访问系统管理</h2>
                  <p>请联系系统管理员开通后台管理权限。</p>
                </div>
                <span>403</span>
              </article>
            </section>
          ) : null}
          {adminResolved && !loading && isSystemAdmin && !accessDenied
            ? renderActiveSection()
            : null}
        </section>
      </div>
    </main>
  );
};

export default SystemManagementPage;
