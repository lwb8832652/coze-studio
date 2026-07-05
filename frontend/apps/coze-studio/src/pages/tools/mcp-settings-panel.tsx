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

import { useCallback, useEffect, useState } from 'react';

import { type workbenchTool } from '@coze-studio/api-schema';
import { Switch } from '@coze-arch/coze-design';

import { listMCPToolServers, upsertMCPToolServer } from './service';

export const MCP_TOOL_SETTINGS_TAB_ID = 'mcp-tools';

type MCPToolServer = workbenchTool.MCPToolServer;

interface MCPToolSettingsPanelProps {
  spaceId?: string;
}

interface ToolServerCardProps {
  disabled: boolean;
  updating: boolean;
  server: MCPToolServer;
  onToggleEnabled: (server: MCPToolServer, enabled: boolean) => void;
}

const ensureSuccessfulWorkbenchResponse = (response: {
  code?: number;
  msg?: string;
}) => {
  if (response.code && response.code !== 0) {
    throw new Error(response.msg || '请求失败');
  }
};

const ToolServerCard = ({
  disabled,
  updating,
  server,
  onToggleEnabled,
}: ToolServerCardProps) => (
  <article className="coze-prototype-settings-item">
    <div className="coze-prototype-row-main">
      <h2 className="coze-prototype-settings-item-title">{server.name}</h2>
      <p className="coze-prototype-settings-item-description">
        {server.description || '暂无 MCP 工具描述'}
      </p>
    </div>
    <Switch
      size="small"
      checked={server.enabled}
      loading={updating}
      disabled={disabled}
      aria-label={`${server.enabled ? '关闭' : '开启'} ${server.name}`}
      onChange={checked => onToggleEnabled(server, checked)}
    />
  </article>
);

export const MCPToolSettingsPanel = ({
  spaceId,
}: MCPToolSettingsPanelProps) => {
  const [servers, setServers] = useState<MCPToolServer[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [updatingServerId, setUpdatingServerId] = useState('');

  const loadServers = useCallback(async () => {
    if (!spaceId) {
      setServers([]);
      return;
    }

    setLoading(true);
    setError('');
    try {
      const response = await listMCPToolServers({ space_id: spaceId });
      ensureSuccessfulWorkbenchResponse(response);
      setServers(response.data?.servers ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载工具配置失败');
    } finally {
      setLoading(false);
    }
  }, [spaceId]);

  useEffect(() => {
    void loadServers();
  }, [loadServers]);

  const handleToggleEnabled = async (
    server: MCPToolServer,
    enabled: boolean,
  ) => {
    if (!spaceId || updatingServerId) {
      return;
    }
    setUpdatingServerId(server.server_id);
    setError('');
    try {
      const response = await upsertMCPToolServer({
        server_id: server.server_id,
        space_id: server.space_id || spaceId,
        name: server.name,
        description: server.description,
        server_type: server.server_type,
        enabled,
        config: server.config,
        auth: server.auth,
        tools: server.tools,
      });
      ensureSuccessfulWorkbenchResponse(response);
      await loadServers();
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新 MCP 工具失败');
    } finally {
      setUpdatingServerId('');
    }
  };

  return (
    <section className="coze-prototype-settings-list" aria-label="MCP 配置">
      {!spaceId ? (
        <div className="coze-prototype-settings-muted">
          暂无空间上下文，无法加载 MCP 工具。
        </div>
      ) : null}
      {loading ? (
        <div className="coze-prototype-settings-muted">加载中...</div>
      ) : null}
      {error ? <div className="coze-prototype-error">{error}</div> : null}

      {!loading && !error && spaceId && servers.length === 0 ? (
        <div className="coze-prototype-settings-muted">暂无 MCP 工具。</div>
      ) : null}

      {!error && servers.length > 0 ? (
        <div className="coze-prototype-settings-item-list">
          {servers.map(server => (
            <ToolServerCard
              key={server.server_id}
              disabled={Boolean(updatingServerId)}
              server={server}
              updating={updatingServerId === server.server_id}
              onToggleEnabled={handleToggleEnabled}
            />
          ))}
        </div>
      ) : null}
    </section>
  );
};
