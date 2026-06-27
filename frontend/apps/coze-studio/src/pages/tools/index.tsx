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

import { useParams } from 'react-router-dom';
import { useEffect, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';

import { type workbenchTool } from '@coze-studio/api-schema';
import {
  IconCozMore,
  IconCozPlugin,
  IconCozPlus,
} from '@coze-arch/coze-design/icons';

import { WorkspacePageTopBar } from '../../components/workspace-page-top-bar';
import '../../components/workspace-prototype.less';
import {
  deleteMCPToolServer,
  listMCPToolServers,
  testMCPToolCall,
  upsertMCPToolServer,
} from './service';

type MCPToolServer = workbenchTool.MCPToolServer;

const DEFAULT_SERVER_CONFIG = JSON.stringify({
  command: 'npx',
  args: ['-y', '@modelcontextprotocol/server-everything'],
});
const DEFAULT_AUTH_CONFIG = JSON.stringify({ type: 'none' });
const DEFAULT_TOOL_SCHEMA = JSON.stringify({
  type: 'object',
  properties: {
    query: {
      type: 'string',
    },
  },
});
const TEST_ARGUMENTS = '{"query":"coze studio"}';

const getUpdatedText = (timestamp: number) => {
  if (!timestamp) {
    return '-';
  }

  return new Date(timestamp).toLocaleDateString();
};

const getHealthStatusText = (server: MCPToolServer) => {
  if (server.health_status === 'healthy') {
    return server.health_latency_ms
      ? `健康 ${server.health_latency_ms}ms`
      : '健康';
  }
  if (server.health_status === 'unhealthy') {
    return '异常';
  }

  return '未检查';
};

interface ToolsPageHeaderProps {
  loading: boolean;
  spaceId?: string;
  onRefresh: () => void;
}

const ToolsPageHeader = ({
  loading,
  spaceId,
  onRefresh,
}: ToolsPageHeaderProps) => (
  <div className="text-center">
    <h1 className="coze-prototype-page-title">工具</h1>
    <p className="coze-prototype-page-subtitle">
      MCP 工具配置统一管理工作空间内可被智能体调用的工具服务。
      <button type="button" className="coze-prototype-link-button">
        查看文档
      </button>
    </p>
    <button
      type="button"
      className="sr-only"
      disabled={loading || !spaceId}
      onClick={onRefresh}
    >
      刷新
    </button>
  </div>
);

interface ToolsToolbarProps {
  creating: boolean;
  disabled: boolean;
  keyword: string;
  onCreate: () => void;
  onKeywordChange: (value: string) => void;
}

const ToolsToolbar = ({
  creating,
  disabled,
  keyword,
  onCreate,
  onKeywordChange,
}: ToolsToolbarProps) => (
  <section className="coze-prototype-toolbar">
    <div className="coze-prototype-segment" data-size="small">
      <button type="button" data-active>
        全部工具
      </button>
      <button type="button">已启用</button>
      <button type="button">已停用</button>
    </div>

    <div className="flex-1" />

    <label className="coze-prototype-search" data-width="compact">
      <span aria-hidden="true">⌕</span>
      <input
        aria-label="搜索工具"
        value={keyword}
        onChange={event => onKeywordChange(event.target.value)}
        placeholder="搜索工具"
      />
    </label>

    <button
      type="button"
      className="coze-prototype-primary-button disabled:opacity-50"
      disabled={disabled}
      onClick={onCreate}
    >
      <IconCozPlus className="text-[14px]" />
      {creating ? '创建中' : '创建工具配置'}
    </button>
  </section>
);

interface ToolServerCardProps {
  deleting: boolean;
  disabled: boolean;
  result?: string;
  running: boolean;
  server: MCPToolServer;
  onDelete: (server: MCPToolServer) => void;
  onTestCall: (server: MCPToolServer) => void;
}

const ToolServerCard = ({
  deleting,
  disabled,
  result,
  running,
  server,
  onDelete,
  onTestCall,
}: ToolServerCardProps) => {
  const firstTool = server.tools[0];

  return (
    <article className="coze-prototype-row">
      <div className="coze-prototype-skill-icon">
        <IconCozPlugin className="text-[14px]" />
      </div>
      <div className="coze-prototype-row-main">
        <div className="flex min-w-0 items-center gap-[8px]">
          <h2 className="m-0 truncate text-[14px] leading-[20px] font-[400] text-[#232938]">
            {server.name}
          </h2>
          <span
            className="h-[6px] w-[6px] shrink-0 rounded-full"
            style={{ backgroundColor: server.enabled ? '#2a9e06' : '#747b8a' }}
          />
          <span className="coze-prototype-tag">{server.server_type}</span>
          {firstTool ? (
            <span className="coze-prototype-tag">{firstTool.name}</span>
          ) : null}
        </div>
        <div className="coze-prototype-row-desc mt-[4px]">
          {server.description || '暂无工具服务描述'}
        </div>
      </div>
      <div className="coze-prototype-row-actions mt-[4px]">
        <span
          className="coze-prototype-status-pill"
          data-tone={server.enabled ? 'success' : 'neutral'}
        >
          <span
            className="coze-prototype-status-dot"
            style={{ backgroundColor: server.enabled ? '#2a9e06' : '#747b8a' }}
          />
          {server.enabled ? '已启用' : '已停用'}
        </span>
        <span className="coze-prototype-muted">
          更新于 {getUpdatedText(server.updated_at)}
        </span>
        <span
          className="coze-prototype-status-pill"
          data-tone={server.health_status === 'healthy' ? 'success' : 'neutral'}
        >
          {getHealthStatusText(server)}
        </span>
        {firstTool ? (
          <button
            type="button"
            className="coze-prototype-secondary-button h-[28px] px-[10px] text-[12px] disabled:opacity-50"
            disabled={disabled || deleting}
            onClick={() => onTestCall(server)}
          >
            {running ? '测试中' : `测试 ${firstTool.name}`}
          </button>
        ) : null}
        <button
          type="button"
          className="coze-prototype-secondary-button h-[28px] px-[10px] text-[12px] text-[#d0292f] disabled:opacity-50"
          disabled={disabled || deleting}
          onClick={() => onDelete(server)}
        >
          {deleting ? '删除中' : '删除'}
        </button>
        <IconCozMore className="text-[16px] text-[#747b8a]" />
      </div>
      {result ? (
        <pre className="mt-[10px] max-w-full overflow-auto rounded-[6px] bg-[#f7f7fa] px-[10px] py-[8px] text-[13px] leading-[20px] coz-fg-primary">
          {result}
        </pre>
      ) : null}
    </article>
  );
};

interface ToolServerListProps {
  deletingServerId: string;
  loading: boolean;
  servers: MCPToolServer[];
  testResults: Record<string, string>;
  testingServerId: string;
  onDelete: (server: MCPToolServer) => void;
  onTestCall: (server: MCPToolServer) => void;
}

const ToolServerList = ({
  deletingServerId,
  loading,
  servers,
  testResults,
  testingServerId,
  onDelete,
  onTestCall,
}: ToolServerListProps) => (
  <section className="mt-[20px]" aria-label="工具列表">
    {loading ? <div className="coze-prototype-empty">加载中...</div> : null}

    {!loading && servers.length === 0 ? (
      <div className="coze-prototype-empty">暂无工具配置</div>
    ) : null}

    <div className="coze-prototype-list">
      {servers.map(server => (
        <ToolServerCard
          key={server.server_id}
          deleting={deletingServerId === server.server_id}
          disabled={Boolean(testingServerId || deletingServerId)}
          result={testResults[server.server_id]}
          running={testingServerId === server.server_id}
          server={server}
          onDelete={onDelete}
          onTestCall={onTestCall}
        />
      ))}
    </div>
  </section>
);

const getVisibleServers = (servers: MCPToolServer[], keyword: string) => {
  const normalizedKeyword = keyword.trim().toLowerCase();

  if (!normalizedKeyword) {
    return servers;
  }

  return servers.filter(server =>
    [server.name, server.description ?? '', server.server_type].some(value =>
      value.toLowerCase().includes(normalizedKeyword),
    ),
  );
};

interface DeleteMCPServerParams {
  deletingServerId: string;
  server: MCPToolServer;
  testingServerId: string;
  loadServers: () => Promise<void>;
  setDeletingServerId: (value: string) => void;
  setError: (value: string) => void;
  setTestResults: Dispatch<SetStateAction<Record<string, string>>>;
}

const deleteMCPServer = async ({
  deletingServerId,
  server,
  testingServerId,
  loadServers,
  setDeletingServerId,
  setError,
  setTestResults,
}: DeleteMCPServerParams) => {
  if (deletingServerId || testingServerId) {
    return;
  }
  if (!window.confirm(`确认删除工具配置「${server.name}」吗？`)) {
    return;
  }

  setDeletingServerId(server.server_id);
  setError('');
  try {
    await deleteMCPToolServer({ server_id: server.server_id });
    setTestResults(current => {
      const { [server.server_id]: _staleResult, ...rest } = current;

      return rest;
    });
    await loadServers();
  } catch (err) {
    setError(err instanceof Error ? err.message : '删除工具配置失败');
  } finally {
    setDeletingServerId('');
  }
};

const ToolsPage = () => {
  const { space_id } = useParams();
  const [servers, setServers] = useState<MCPToolServer[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [creating, setCreating] = useState(false);
  const [deletingServerId, setDeletingServerId] = useState('');
  const [testingServerId, setTestingServerId] = useState('');
  const [testResults, setTestResults] = useState<Record<string, string>>({});
  const [keyword, setKeyword] = useState('');
  const visibleServers = getVisibleServers(servers, keyword);
  const loadServers = async () => {
    if (!space_id) {
      return;
    }
    setLoading(true);
    setError('');
    try {
      const response = await listMCPToolServers({ space_id });
      setServers(response.data?.servers ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载工具配置失败');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadServers();
  }, [space_id]);
  const handleCreate = async () => {
    if (!space_id || creating) {
      return;
    }
    setCreating(true);
    setError('');
    try {
      await upsertMCPToolServer({
        space_id,
        name: 'browser-tools',
        description: 'Browser automation tools',
        server_type: 'stdio',
        enabled: true,
        config: DEFAULT_SERVER_CONFIG,
        auth: DEFAULT_AUTH_CONFIG,
        tools: [
          {
            name: 'search',
            description: 'Search the web',
            input_schema: DEFAULT_TOOL_SCHEMA,
          },
        ],
      });
      await loadServers();
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建工具配置失败');
    } finally {
      setCreating(false);
    }
  };

  const handleTestCall = async (server: MCPToolServer) => {
    const firstTool = server.tools[0];
    if (!firstTool || testingServerId) {
      return;
    }
    setTestingServerId(server.server_id);
    setError('');
    setTestResults(current => {
      const { [server.server_id]: _staleResult, ...rest } = current;
      return rest;
    });
    try {
      const response = await testMCPToolCall({
        server_id: server.server_id,
        tool_name: firstTool.name,
        arguments: TEST_ARGUMENTS,
      });

      setTestResults(current => ({
        ...current,
        [server.server_id]: JSON.stringify(
          {
            output: response.data?.output ?? '',
            latency_ms: response.data?.latency_ms ?? 0,
            status: response.data?.status ?? 'success',
          },
          null,
          2,
        ),
      }));
    } catch (err) {
      setError(err instanceof Error ? err.message : '测试工具调用失败');
    } finally {
      setTestingServerId('');
    }
  };

  const handleDelete = (server: MCPToolServer) =>
    deleteMCPServer({
      deletingServerId,
      server,
      testingServerId,
      loadServers,
      setDeletingServerId,
      setError,
      setTestResults,
    });

  return (
    <main className="coze-prototype-page">
      <WorkspacePageTopBar />
      <section className="coze-prototype-page-inner">
        <ToolsPageHeader
          loading={loading}
          spaceId={space_id}
          onRefresh={loadServers}
        />
        <ToolsToolbar
          creating={creating}
          disabled={!space_id || creating}
          keyword={keyword}
          onCreate={handleCreate}
          onKeywordChange={setKeyword}
        />

        {error ? <div className="coze-prototype-error">{error}</div> : null}

        <ToolServerList
          deletingServerId={deletingServerId}
          loading={loading}
          servers={visibleServers}
          testResults={testResults}
          testingServerId={testingServerId}
          onDelete={handleDelete}
          onTestCall={handleTestCall}
        />
      </section>
    </main>
  );
};

export default ToolsPage;
