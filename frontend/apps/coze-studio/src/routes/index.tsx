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

import { createBrowserRouter, Navigate, useParams } from 'react-router-dom';

import { SpaceSubModuleEnum } from '@coze-foundation/space-ui-adapter';
import { GlobalError } from '@coze-foundation/layout';
import { BaseEnum } from '@coze-arch/web-context';

import { Layout } from '../layout';
import { SPACE_SUB_MODULE } from '../components/workspace-sub-menu/menu';
import {
  LoginPage,
  SpaceLayout,
  SpaceIdLayout,
  Develop,
  AgentIDELayout,
  AgentIDE,
  AgentPublishPage,
  Redirect,
  spaceSubMenu,
  exploreSubMenu,
  WorkflowPage,
  SearchPage,
  ProjectIDE,
  ProjectIDEPublish,
  Library,
  PluginLayout,
  PluginToolPage,
  PluginPage,
  KnowledgePreview,
  KnowledgeUpload,
  DatabaseDetail,
  ExplorePluginPage,
  ExploreTemplatePage,
  OAuthConsentConfirmPage,
  Workbench,
  AppDev,
  AppDevIDE,
  SkillPage,
  ToolsPage,
  WorkspacePage,
  PersonalCenterPage,
  SystemManagementPage,
  TaskDetailPage,
  TasksPage,
} from './async-components';

const TaskThreadDetailRedirect = () => {
  const { space_id, thread_id } = useParams();

  if (!space_id || !thread_id) {
    return <Navigate to="../chats" replace relative="path" />;
  }

  return <Navigate to={`/space/${space_id}/tasks/${thread_id}`} replace />;
};

export const router: ReturnType<typeof createBrowserRouter> =
  createBrowserRouter([
    // Document routing
    {
      path: '/open/docs/*',
      Component: Redirect,
      loader: () => ({
        hasSider: false,
        requireAuth: false,
      }),
    },
    {
      path: '/docs/*',
      Component: Redirect,
      loader: () => ({
        hasSider: false,
        requireAuth: false,
      }),
    },
    {
      path: '/information/auth/success',
      Component: Redirect,
      loader: () => ({
        hasSider: false,
        requireAuth: false,
      }),
    },
    // main application route
    {
      path: '/',
      Component: Layout,
      errorElement: <GlobalError />,
      children: [
        {
          index: true,
          element: <Navigate to="/space" replace />,
        },
        // login page routing
        {
          path: 'sign',
          Component: LoginPage,
          errorElement: <GlobalError />,
          loader: () => ({
            hasSider: false,
            requireAuth: false,
          }),
        },

        // OAuth consent confirm page
        {
          path: 'oauth/confirm',
          Component: OAuthConsentConfirmPage,
          errorElement: <GlobalError />,
          loader: () => ({
            hasSider: false,
            requireAuth: false,
          }),
        },

        // Workspace Routing
        {
          path: 'space',
          Component: SpaceLayout,
          loader: () => ({
            hasSider: true,
            hidePrimarySider: true,
            subMenuDefaultWidth: 300,
            requireAuth: true,
            subMenu: spaceSubMenu,
            menuKey: BaseEnum.Space,
          }),
          children: [
            {
              path: ':space_id',
              Component: SpaceIdLayout,
              children: [
                {
                  index: true,
                  element: <Navigate to="chats/new" replace />,
                },

                // Chat Workbench
                {
                  path: 'workbench',
                  element: (
                    <Navigate to="../chats/new" replace relative="path" />
                  ),
                },
                {
                  path: 'chats/new',
                  Component: Workbench,
                  loader: () => ({
                    subMenuKey: SPACE_SUB_MODULE.WORKBENCH,
                  }),
                },

                // Project Development
                {
                  path: 'develop',
                  Component: Develop,
                  loader: () => ({
                    subMenuKey: SpaceSubModuleEnum.DEVELOP,
                  }),
                },

                // Agent IDE
                {
                  path: 'bot/:bot_id',
                  Component: AgentIDELayout,
                  children: [
                    {
                      index: true,
                      Component: AgentIDE,
                    },
                    {
                      path: 'publish',
                      children: [
                        {
                          index: true,
                          Component: AgentPublishPage,
                          loader: () => ({
                            hasSider: false,
                            requireBotEditorInit: false,
                            pageName: 'publish',
                          }),
                        },
                      ],
                    },
                  ],
                  loader: () => ({
                    hasSider: false,
                    showMobileTips: true,
                    requireBotEditorInit: true,
                    pageName: 'bot',
                  }),
                },

                // Project IDE
                {
                  path: 'project-ide/:project_id/publish',
                  loader: () => ({
                    hasSider: false,
                  }),
                  Component: ProjectIDEPublish,
                },
                {
                  path: 'project-ide/:project_id/*',
                  Component: ProjectIDE,
                  loader: () => ({
                    hasSider: false,
                  }),
                },

                // resource library
                {
                  path: 'library',
                  Component: Library,
                  loader: () => ({
                    subMenuKey: SpaceSubModuleEnum.LIBRARY,
                  }),
                },

                // Web App Development
                {
                  path: 'app-dev',
                  Component: AppDev,
                  loader: () => ({
                    subMenuKey: SPACE_SUB_MODULE.APP_DEV,
                  }),
                },
                {
                  path: 'app-dev/:project_id',
                  Component: AppDevIDE,
                  loader: () => ({
                    subMenuKey: SPACE_SUB_MODULE.APP_DEV,
                  }),
                },

                // Skill Configuration
                {
                  path: 'skill',
                  Component: SkillPage,
                  loader: () => ({
                    subMenuKey: SpaceSubModuleEnum.SKILL,
                  }),
                },

                // Tool Configuration
                {
                  path: 'tools',
                  Component: ToolsPage,
                  loader: () => ({
                    subMenuKey: SPACE_SUB_MODULE.TOOLS,
                  }),
                },

                // Workspace Settings
                {
                  path: 'workspace',
                  Component: WorkspacePage,
                  loader: () => ({
                    subMenuKey: SPACE_SUB_MODULE.WORKSPACE,
                  }),
                },

                // Tasks
                {
                  path: 'tasks/:task_id',
                  Component: TaskDetailPage,
                  loader: () => ({
                    subMenuKey: SPACE_SUB_MODULE.TASKS,
                  }),
                },
                {
                  path: 'tasks',
                  element: <Navigate to="../chats" replace relative="path" />,
                },
                {
                  path: 'chats/:thread_id',
                  Component: TaskThreadDetailRedirect,
                  loader: () => ({
                    subMenuKey: SPACE_SUB_MODULE.TASKS,
                  }),
                },
                {
                  path: 'chats',
                  Component: TasksPage,
                  loader: () => ({
                    subMenuKey: SPACE_SUB_MODULE.TASKS,
                  }),
                },

                // Knowledge Base Resources
                {
                  path: 'knowledge',
                  children: [
                    {
                      path: ':dataset_id',
                      element: <KnowledgePreview />,
                    },
                    {
                      path: ':dataset_id/upload',
                      element: <KnowledgeUpload />,
                    },
                  ],
                  loader: () => ({
                    pageModeByQuery: true,
                  }),
                },

                // database resources
                {
                  path: 'database',
                  children: [
                    {
                      path: ':table_id',
                      element: <DatabaseDetail />,
                    },
                  ],
                  loader: () => ({
                    showMobileTips: true,
                    pageModeByQuery: true,
                  }),
                },

                // plugin resources
                {
                  path: 'plugin/:plugin_id',
                  Component: PluginLayout,
                  children: [
                    {
                      index: true,
                      Component: PluginPage,
                    },
                    {
                      path: 'tool/:tool_id',
                      children: [
                        {
                          index: true,
                          Component: PluginToolPage,
                        },
                      ],
                    },
                  ],
                },
              ],
            },
          ],
        },

        // System Management
        {
          path: 'profile',
          Component: PersonalCenterPage,
          loader: () => ({
            hasSider: false,
            requireAuth: true,
          }),
        },
        {
          path: 'system',
          element: <Navigate to="/system/overview" replace />,
        },
        {
          path: 'system/:section',
          Component: SystemManagementPage,
          loader: () => ({
            hasSider: false,
            requireAuth: true,
          }),
        },

        // workflow routing
        {
          path: 'work_flow',
          Component: WorkflowPage,
          loader: () => ({
            hasSider: false,
            requireAuth: true,
          }),
        },

        // search
        {
          path: 'search/:word',
          Component: SearchPage,
          loader: () => ({
            hasSider: true,
            requireAuth: true,
          }),
        },

        // explore
        {
          path: 'explore',
          Component: null,
          loader: () => ({
            hasSider: true,
            requireAuth: true,
            subMenu: exploreSubMenu,
            menuKey: BaseEnum.Explore,
          }),
          children: [
            {
              index: true,
              element: <Navigate to="plugin" replace />,
            },
            // plugin store
            {
              path: 'plugin',
              element: <ExplorePluginPage />,
              loader: () => ({
                type: 'plugin',
              }),
            },
            // template
            {
              path: 'template',
              element: <ExploreTemplatePage />,
              loader: () => ({
                type: 'template',
              }),
            },
          ],
        },
      ],
    },
  ]);
