/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

export type FeishuReplyMode = 'final' | 'stream';
export type FeishuGroupPolicy = 'mention_only' | 'disabled';
export type FeishuRuntimeStatus =
  | 'disabled'
  | 'pending'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'error';

export interface FeishuIMConfig {
  id: string;
  space_id: string;
  creator_id: string;
  agent_id: string;
  agent_name: string;
  channel_type: 'feishu';
  name: string;
  app_id: string;
  secret_configured: boolean;
  enabled: boolean;
  reply_mode: FeishuReplyMode;
  group_policy: FeishuGroupPolicy;
  runtime_status: FeishuRuntimeStatus;
  runtime_error?: string;
  bot_open_id?: string;
  bot_name?: string;
  last_connected_at?: number;
  last_tested_at?: number;
  created_at: number;
  updated_at: number;
}

export interface FeishuIMListResult {
  configs: FeishuIMConfig[];
  can_manage: boolean;
  credential_ready: boolean;
}

export interface FeishuIMMutation {
  space_id: string;
  name: string;
  app_id: string;
  app_secret: string;
  agent_id: string;
  enabled: boolean;
  reply_mode: FeishuReplyMode;
  group_policy: FeishuGroupPolicy;
}

export interface AgentTarget {
  id: string;
  name: string;
  icon_uri?: string;
  published: boolean;
}

interface APIResponse<T> {
  code?: number | string;
  msg?: string;
  data?: T;
}

const request = async <T>(path: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(path, {
    ...init,
    credentials: 'include',
    headers: {
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  });
  const payload = (await response.json().catch(() => ({}))) as APIResponse<T>;
  if (!response.ok || (payload.code !== undefined && payload.code !== 0)) {
    throw new Error(payload.msg || `请求失败（${response.status}）`);
  }
  if (payload.data === undefined) {
    throw new Error('服务端返回为空');
  }
  return payload.data;
};

export const listFeishuIMConfigs = (spaceId: string) =>
  request<FeishuIMListResult>(
    `/api/workbench/im_channels?space_id=${encodeURIComponent(spaceId)}`,
  );

export const createFeishuIMConfig = (payload: FeishuIMMutation) =>
  request<FeishuIMConfig>('/api/workbench/im_channels', {
    method: 'POST',
    body: JSON.stringify(payload),
  });

export const updateFeishuIMConfig = (
  configId: string,
  payload: FeishuIMMutation,
) =>
  request<FeishuIMConfig>(
    `/api/workbench/im_channels/${encodeURIComponent(configId)}`,
    {
      method: 'PUT',
      body: JSON.stringify(payload),
    },
  );

export const setFeishuIMConfigEnabled = (
  configId: string,
  spaceId: string,
  enabled: boolean,
) =>
  request<FeishuIMConfig>(
    `/api/workbench/im_channels/${encodeURIComponent(configId)}/${
      enabled ? 'enable' : 'disable'
    }`,
    {
      method: 'POST',
      body: JSON.stringify({ space_id: spaceId }),
    },
  );

export const testFeishuIMConfig = (configId: string, spaceId: string) =>
  request<{ bot_open_id: string; bot_name: string }>(
    `/api/workbench/im_channels/${encodeURIComponent(configId)}/test`,
    {
      method: 'POST',
      body: JSON.stringify({ space_id: spaceId }),
    },
  );

export const deleteFeishuIMConfig = (configId: string, spaceId: string) =>
  request<{ deleted: boolean }>(
    `/api/workbench/im_channels/${encodeURIComponent(
      configId,
    )}?space_id=${encodeURIComponent(spaceId)}`,
    { method: 'DELETE' },
  );

export const listPublishedAgentTargets = async (
  spaceId: string,
): Promise<AgentTarget[]> => {
  const result = await request<{ targets: AgentTarget[]; total: number }>(
    `/api/workbench/scheduled_task_targets?space_id=${encodeURIComponent(
      spaceId,
    )}&target_type=1&page=1&page_size=100`,
  );
  return result.targets ?? [];
};
