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

export const SYSTEM_SECTIONS = [
  {
    key: 'overview',
    title: '概览',
    description: '系统运行、关键资源和待处理事项',
  },
  {
    key: 'users',
    title: '用户管理',
    description: '账号状态、登录信息和权限入口',
  },
  {
    key: 'workspaces',
    title: '工作空间管理',
    description: '团队空间、个人空间和成员规模',
  },
  {
    key: 'models',
    title: '模型配置',
    description: '公共模型、供应商和接入密钥管理',
  },
  {
    key: 'settings',
    title: '系统配置',
    description: '知识库和基础服务配置',
  },
] as const;

export type SystemSectionKey = (typeof SYSTEM_SECTIONS)[number]['key'];

export const SECTION_CONTENT: Record<
  SystemSectionKey,
  {
    heading: string;
    summary: string;
    cards: Array<{
      title: string;
      description: string;
      value: string;
    }>;
  }
> = {
  overview: {
    heading: '系统管理概览',
    summary:
      '后台管理员可以在这里统一查看用户、工作空间、模型配置和基础服务状态。',
    cards: [
      {
        title: '用户管理',
        description: '查看用户账号、状态和权限策略。',
        value: '已启用',
      },
      {
        title: '工作空间',
        description: '查看个人空间和团队空间的管理入口。',
        value: '已启用',
      },
      {
        title: '模型配置',
        description: '集中管理公共模型供应商和接入密钥。',
        value: '已启用',
      },
    ],
  },
  users: {
    heading: '用户管理',
    summary:
      '集中查看用户账号、所属工作空间和权限边界，高风险账号操作由权限与审计保护。',
    cards: [
      {
        title: '账号列表',
        description: '按用户、邮箱和最近登录时间查询。',
        value: '已接入',
      },
      {
        title: '权限角色',
        description: '展示后台管理员和普通用户的角色边界。',
        value: '只读',
      },
      {
        title: '安全审计',
        description: '记录关键配置和账号操作。',
        value: '受控',
      },
    ],
  },
  workspaces: {
    heading: '工作空间管理',
    summary:
      '集中查看个人空间、团队空间、空间所有者和成员规模，危险操作保持受控展示。',
    cards: [
      {
        title: '空间列表',
        description: '查看团队空间和个人空间。',
        value: '已接入',
      },
      {
        title: '成员规模',
        description: '展示成员数量、所有者和管理员信息。',
        value: '只读',
      },
      {
        title: '空间状态',
        description: '展示空间归属、成员规模和管理边界。',
        value: '受控',
      },
    ],
  },
  settings: {
    heading: '系统配置',
    summary:
      '这里承接系统基础配置。涉及密钥和高风险写操作的能力需要后端权限与审计先到位。',
    cards: [
      {
        title: '知识库配置',
        description: '查看知识库默认能力和限制。',
        value: '只读优先',
      },
      {
        title: '基础服务',
        description: '查看服务地址、沙盒和运行依赖。',
        value: '已接入',
      },
      {
        title: '注册策略',
        description: '查看账号注册开关和管理员白名单。',
        value: '可保存',
      },
    ],
  },
  models: {
    heading: '模型配置',
    summary: '集中管理公共模型供应商、模型接入信息、密钥状态和删除保护。',
    cards: [
      {
        title: '供应商分组',
        description: '按模型供应商展示已接入模型。',
        value: '已接 API',
      },
      {
        title: '新增模型',
        description: '配置模型标识、Base URL、API Key 和 Base64 URL。',
        value: '可创建',
      },
      {
        title: '删除模型',
        description: '删除前二次确认，避免误删公共模型。',
        value: '可删除',
      },
    ],
  },
};
