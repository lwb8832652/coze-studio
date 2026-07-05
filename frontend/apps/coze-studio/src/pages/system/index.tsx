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

import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import '../../components/workspace-prototype.less';
import { SYSTEM_SECTIONS } from './content';
import { ModelConfigSection } from './model-config-section';
import { OverviewSection } from './overview-section';
import { SystemSettingsSection } from './system-settings-section';
import { UserManagementSection } from './user-management-section';
import { WorkspaceManagementSection } from './workspace-management-section';
import {
  createAdminModel,
  deleteAdminModel,
  getSystemAdminStatus,
  getAdminBasicConfig,
  getAdminKnowledgeConfig,
  getAdminModelList,
  listAdminUserSpaces,
  listAdminWorkspaceMembers,
  listAdminUsers,
  listAdminWorkspaces,
  type AdminCreateModelPayload,
  type AdminBasicConfig,
  type AdminKnowledgeConfig,
  type AdminProviderModelListItem,
  type AdminUserSpace,
  type AdminUser,
  type AdminWorkspace,
  type AdminWorkspaceMember,
} from './service';
import {
  getActiveSystemSection,
  getSystemSectionContent,
} from './view-model';

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
  const [basicConfig, setBasicConfig] = useState<AdminBasicConfig>({});
  const [modelProviders, setModelProviders] = useState<
    AdminProviderModelListItem[]
  >([]);
  const [knowledgeConfig, setKnowledgeConfig] = useState<AdminKnowledgeConfig>(
    {},
  );
  const [loading, setLoading] = useState(false);
  const [accessDenied, setAccessDenied] = useState(false);
  const [errorMessage, setErrorMessage] = useState('');
  const [modelSaving, setModelSaving] = useState(false);
  const [modelRefreshing, setModelRefreshing] = useState(false);
  const [modelMessage, setModelMessage] = useState('');
  const activeSection = getActiveSystemSection(section);
  const content = getSystemSectionContent(section);

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
      setLoading(true);
      setErrorMessage('');
      try {
        const adminStatus = await getSystemAdminStatus();
        if (canceled) {
          return;
        }
        if (!adminStatus.is_admin) {
          setAccessDenied(true);
          return;
        }
        setAccessDenied(false);

        const [
          workspaceResp,
          userResp,
          configResp,
          modelResp,
          knowledgeResp,
        ] = await Promise.all([
          listAdminWorkspaces({
            page: 1,
            size: ADMIN_PAGE_SIZE,
          }),
          listAdminUsers({
            page: 1,
            size: ADMIN_PAGE_SIZE,
          }),
          getAdminBasicConfig(),
          getAdminModelList(),
          getAdminKnowledgeConfig(),
        ]);
        if (canceled) {
          return;
        }
        setWorkspaces(workspaceResp.workspaces);
        setWorkspaceTotal(workspaceResp.total);
        setUsers(userResp.users);
        setUserTotal(userResp.total);
        setBasicConfig(configResp.configuration ?? {});
        setModelProviders(modelResp.provider_model_list ?? []);
        setKnowledgeConfig(knowledgeResp.knowledge_config ?? {});
      } catch (error) {
        if (!canceled) {
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
  }, []);

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

  const renderOverview = () => (
    <OverviewSection
      userTotal={userTotal}
      workspaceTotal={workspaceTotal}
      basicConfig={basicConfig}
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
      onKeywordChange={setUserKeyword}
      onSearch={searchUsers}
      onShowUserSpaces={showUserSpaces}
      onTurnPage={turnUserPage}
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
      onKeywordChange={setWorkspaceKeyword}
      onSearch={searchWorkspaces}
      onShowWorkspaceMembers={showWorkspaceMembers}
      onTurnPage={turnWorkspacePage}
    />
  );

  const renderSettings = () => (
    <SystemSettingsSection
      basicConfig={basicConfig}
      knowledgeConfig={knowledgeConfig}
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
            {SYSTEM_SECTIONS.map(item => (
              <button
                key={item.key}
                type="button"
                className="coze-prototype-system-nav-item"
                data-active={activeSection === item.key}
                onClick={() => navigate(`/system/${item.key}`)}
              >
                <span>{item.title}</span>
                <span>{item.description}</span>
              </button>
            ))}
          </nav>
        </aside>

        <section className="coze-prototype-system-content">
          <header className="coze-prototype-system-hero">
            <h1>{content.heading}</h1>
            <p>{content.summary}</p>
          </header>

          {loading ? <p>加载中...</p> : null}
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
          {!loading && !accessDenied ? renderActiveSection() : null}
        </section>
      </div>
    </main>
  );
};

export default SystemManagementPage;
