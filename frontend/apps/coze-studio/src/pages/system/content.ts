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
    key: 'announcements',
    title: '公告通知',
    description: '创建、计划发布和审计管理员公告',
  },
  {
    key: 'models',
    title: '模型配置',
    description: '公共模型、供应商和接入密钥管理',
  },
  {
    key: 'billing-pricing',
    title: '模型定价',
    description: '按模型版本配置 Token 计价',
  },
  {
    key: 'billing-monitoring',
    title: '模型监控',
    description: '调用、Token 和积分消耗',
  },
  {
    key: 'sandbox',
    title: '沙箱管理',
    description: '运行 Provider、健康状态和默认范围',
  },
  {
    key: 'billing-config',
    title: '积分基础配置',
    description: '积分名称、结算和支付开关',
  },
  {
    key: 'billing-plans',
    title: '订阅套餐',
    description: '周期、价格和套餐权益',
  },
  {
    key: 'billing-packages',
    title: '积分包',
    description: '售价、额度和有效期',
  },
  {
    key: 'billing-accounts',
    title: '用户积分',
    description: '余额、预占和账户状态',
  },
  {
    key: 'billing-ledger',
    title: '积分记录',
    description: '不可变收入与消费流水',
  },
  { key: 'billing-orders', title: '订单管理', description: '支付和履约状态' },
  {
    key: 'settings',
    title: '系统配置',
    description: '知识库和基础服务配置',
  },
] as const;

export type SystemSectionKey = (typeof SYSTEM_SECTIONS)[number]['key'];

interface SystemNavGroup {
  key: string;
  label?: string;
  items: readonly SystemSectionKey[];
}

export const SYSTEM_NAV_GROUPS = [
  { key: 'general', items: ['overview', 'users', 'workspaces'] },
  {
    key: 'models',
    label: '模型管理',
    items: ['models', 'billing-pricing', 'billing-monitoring'],
  },
  { key: 'runtime', items: ['sandbox'] },
  {
    key: 'billing',
    label: '订阅与积分',
    items: [
      'billing-config',
      'billing-plans',
      'billing-packages',
      'billing-accounts',
      'billing-ledger',
      'billing-orders',
    ],
  },
  { key: 'system', items: ['announcements', 'settings'] },
] as const satisfies readonly SystemNavGroup[];

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
  announcements: {
    heading: '管理员公告通知',
    summary:
      '创建纯文本系统公告，按全体用户、工作空间或指定用户发布，并跟踪受众快照、通知投影和审计历史。',
    cards: [],
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
    summary: '管理公共模型、能力范围、接入端点、运行状态和使用授权。',
    cards: [
      {
        title: '模型目录',
        description: '按供应商、类型、状态和管控范围查询。',
        value: '可筛选',
      },
      {
        title: '接入与验证',
        description: '配置多 Endpoint、加密密钥并执行连通性测试。',
        value: '可维护',
      },
      {
        title: '授权管控',
        description: '按工作空间和用户限制模型使用范围。',
        value: '可授权',
      },
    ],
  },
  sandbox: {
    heading: '沙箱管理',
    summary:
      '统一管理 Agent、MCP stdio 和网页应用开发使用的运行 Provider、健康状态与安全边界。',
    cards: [
      {
        title: 'Provider 管理',
        description: '创建、编辑、启停和安全删除运行 Provider。',
        value: '已接 API',
      },
      {
        title: '运行健康',
        description: '执行受控健康检查并展示安全状态摘要。',
        value: '可检查',
      },
      {
        title: '默认范围',
        description: '为 Agent、MCP stdio 和 AppDev 设置默认 Provider。',
        value: '受版本保护',
      },
    ],
  },
  'billing-config': {
    heading: '积分基础配置',
    summary: '管理积分显示、运行时结算和支付能力。',
    cards: [],
  },
  'billing-plans': {
    heading: '订阅套餐',
    summary: '管理订阅周期、价格、额度和权益版本。',
    cards: [],
  },
  'billing-packages': {
    heading: '积分包',
    summary: '管理一次性积分包的售价、额度和有效期。',
    cards: [],
  },
  'billing-pricing': {
    heading: '模型定价',
    summary: '按供应商和模型维护可追溯的 Token 计价版本。',
    cards: [],
  },
  'billing-monitoring': {
    heading: '模型监控',
    summary: '查看模型调用量、Token 用量和积分消耗汇总。',
    cards: [],
  },
  'billing-accounts': {
    heading: '用户积分',
    summary: '查看用户账单账户的可用和预占积分。',
    cards: [],
  },
  'billing-ledger': {
    heading: '积分记录',
    summary: '查询可审计、不可变的积分收入与支出流水。',
    cards: [],
  },
  'billing-orders': {
    heading: '订单管理',
    summary: '查看订单支付、履约和异常状态。',
    cards: [],
  },
};
