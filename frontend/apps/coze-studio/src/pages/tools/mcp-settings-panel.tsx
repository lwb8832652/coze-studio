/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/* eslint-disable max-lines, complexity, max-lines-per-function -- Cohesive management orchestrator. */
/* eslint-disable @coze-arch/max-line-per-function -- Cohesive MCP management page orchestrator. */

import { useCallback, useEffect, useMemo, useState } from 'react';

import type { workbenchTool } from '@coze-studio/api-schema';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import { IconCozDatabase, IconCozPlugin } from '@coze-arch/coze-design/icons';
import { Button, Input, Modal, Spin, Switch } from '@coze-arch/coze-design';

import {
  deleteMCPToolServer,
  discoverMCPToolServer,
  exportMCPToolServer,
  installMCPOfficialCatalog,
  listMCPToolAuditEvents,
  listMCPOfficialCatalog,
  listMCPToolServers,
  testMCPToolCall,
  upsertMCPToolServer,
} from './service';
import { MCPTestModal } from './mcp-test-modal';
import {
  MCPServerFormSheet,
  type MCPServerDraft,
} from './mcp-server-form-sheet';
import { MCPCapabilitySheet } from './mcp-capability-sheet';

export const MCP_TOOL_SETTINGS_TAB_ID = 'mcp-tools';

type MCPToolServer = workbenchTool.MCPToolServer;
type MCPToolDefinition = workbenchTool.MCPToolDefinition;
type MCPToolAuditEvent = workbenchTool.MCPToolAuditEvent;
type MCPExport = workbenchTool.ExportMCPToolServerData;
type MCPOfficialCatalogEntry = workbenchTool.MCPOfficialCatalogEntry;

const JSON_INDENT = 2;
const HTTP_STATUS_UNAUTHORIZED = 401;
const HTTP_STATUS_SERVICE_UNAVAILABLE = 503;

interface MCPToolSettingsPanelProps {
  mode?: 'compact' | 'full';
  spaceId?: string;
}

type SourceFilter = 'custom' | 'official';
type CreatorFilter = 'all' | 'me';
type StatusFilter = 'all' | 'enabled' | 'disabled' | 'healthy' | 'unhealthy';
type MCPSettingsLoadStatus = 'loading' | 'success' | 'error';

const mcpSettingsLoadErrorMessage = (cause: unknown) => {
  const response = (
    cause as {
      response?: { status?: number; data?: { msg?: unknown } };
    }
  )?.response;
  if (response?.status === HTTP_STATUS_UNAUTHORIZED) {
    return '登录状态已失效，请重新登录';
  }
  if (response?.status === HTTP_STATUS_SERVICE_UNAVAILABLE) {
    return '当前环境未启用或暂时无法提供 MCP 服务';
  }
  if (typeof response?.data?.msg === 'string' && response.data.msg.trim()) {
    return response.data.msg;
  }
  return '加载 MCP 服务失败，请重试';
};

const ensureSuccessfulWorkbenchResponse = (response: {
  code?: number;
  msg?: string;
}) => {
  if (response.code && response.code !== 0) {
    throw new Error(response.msg || '请求失败');
  }
};

const formatTime = (timestamp?: number) => {
  if (!timestamp) {
    return '-';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp));
};

const healthLabel = (server: MCPToolServer) => {
  if (!server.enabled) {
    return '已停用';
  }
  if (server.health_status === 'healthy') {
    return '运行正常';
  }
  if (server.health_status === 'unhealthy') {
    return '连接异常';
  }
  return '待检测';
};

const matchesStatus = (server: MCPToolServer, status: StatusFilter) => {
  switch (status) {
    case 'enabled':
      return server.enabled;
    case 'disabled':
      return !server.enabled;
    case 'healthy':
      return server.health_status === 'healthy';
    case 'unhealthy':
      return server.health_status === 'unhealthy';
    default:
      return true;
  }
};

const serverUpsertPayload = (
  server: MCPToolServer,
  enabled = server.enabled,
) => ({
  server_id: server.server_id,
  space_id: server.space_id,
  name: server.name,
  description: server.description,
  server_type: server.server_type,
  enabled,
  config: server.config,
  auth: server.auth,
  tools: server.tools,
});

export const MCPToolSettingsPanel = ({
  mode = 'full',
  spaceId,
}: MCPToolSettingsPanelProps) => {
  const userInfo = useUserInfo();
  const [servers, setServers] = useState<MCPToolServer[]>([]);
  const [officialCatalog, setOfficialCatalog] = useState<
    MCPOfficialCatalogEntry[]
  >([]);
  const [canManage, setCanManage] = useState<boolean>();
  const [loadStatus, setLoadStatus] =
    useState<MCPSettingsLoadStatus>('loading');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busyAction, setBusyAction] = useState('');
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>('custom');
  const [creatorFilter, setCreatorFilter] = useState<CreatorFilter>('all');
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all');
  const [keyword, setKeyword] = useState('');
  const [formVisible, setFormVisible] = useState(false);
  const [officialCandidate, setOfficialCandidate] =
    useState<MCPOfficialCatalogEntry>();
  const [officialCredentials, setOfficialCredentials] = useState<
    Record<string, string>
  >({});
  const [officialFormError, setOfficialFormError] = useState('');
  const [editingServer, setEditingServer] = useState<MCPToolServer>();
  const [capabilityServer, setCapabilityServer] = useState<MCPToolServer>();
  const [testTool, setTestTool] = useState<MCPToolDefinition>();
  const [deleteCandidate, setDeleteCandidate] = useState<MCPToolServer>();
  const [exportedServer, setExportedServer] = useState<MCPExport>();
  const [exportedServerID, setExportedServerID] = useState('');
  const [auditEvents, setAuditEvents] = useState<MCPToolAuditEvent[]>([]);
  const [auditCursor, setAuditCursor] = useState('');
  const [auditError, setAuditError] = useState('');
  const [auditLoaded, setAuditLoaded] = useState(false);
  const [auditLoading, setAuditLoading] = useState(false);
  const loading = loadStatus === 'loading';

  const loadServers = useCallback(async () => {
    if (!spaceId) {
      setServers([]);
      setCanManage(undefined);
      setLoadStatus('error');
      return;
    }
    setLoadStatus('loading');
    setCanManage(undefined);
    setError('');
    try {
      const [serversResponse, catalogResponse] = await Promise.all([
        listMCPToolServers({ space_id: spaceId }),
        listMCPOfficialCatalog({ space_id: spaceId }),
      ]);
      ensureSuccessfulWorkbenchResponse(serversResponse);
      ensureSuccessfulWorkbenchResponse(catalogResponse);
      setServers(serversResponse.data?.servers ?? []);
      setOfficialCatalog(catalogResponse.data?.entries ?? []);
      setCanManage(
        Boolean(
          serversResponse.data?.can_manage && catalogResponse.data?.can_manage,
        ),
      );
      setLoadStatus('success');
    } catch (loadError) {
      setCanManage(undefined);
      setError(mcpSettingsLoadErrorMessage(loadError));
      setLoadStatus('error');
    }
  }, [spaceId]);

  useEffect(() => {
    void loadServers();
  }, [loadServers]);

  const visibleServers = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();
    const currentUserID = userInfo?.user_id_str;
    return servers.filter(server => {
      if ((server.source_type || 'custom') !== sourceFilter) {
        return false;
      }
      if (
        creatorFilter === 'me' &&
        currentUserID &&
        server.creator_id !== currentUserID
      ) {
        return false;
      }
      if (!matchesStatus(server, statusFilter)) {
        return false;
      }
      return (
        !normalizedKeyword ||
        server.name.toLowerCase().includes(normalizedKeyword) ||
        server.description.toLowerCase().includes(normalizedKeyword)
      );
    });
  }, [
    creatorFilter,
    keyword,
    servers,
    sourceFilter,
    statusFilter,
    userInfo?.user_id_str,
  ]);

  const visibleOfficialCatalog = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();
    return officialCatalog.filter(
      entry =>
        !normalizedKeyword ||
        entry.name.toLowerCase().includes(normalizedKeyword) ||
        entry.description.toLowerCase().includes(normalizedKeyword) ||
        (entry.publisher || '').toLowerCase().includes(normalizedKeyword),
    );
  }, [keyword, officialCatalog]);

  const runMutation = async (
    actionKey: string,
    action: () => Promise<void>,
  ) => {
    if (busyAction) {
      return;
    }
    setBusyAction(actionKey);
    setError('');
    setNotice('');
    try {
      await action();
    } catch (mutationError) {
      setError(
        mutationError instanceof Error
          ? mutationError.message
          : '操作失败，请重试',
      );
    } finally {
      setBusyAction('');
    }
  };

  const handleToggleEnabled = (server: MCPToolServer, enabled: boolean) =>
    runMutation(`toggle-${server.server_id}`, async () => {
      const response = await upsertMCPToolServer(
        serverUpsertPayload(server, enabled),
      );
      ensureSuccessfulWorkbenchResponse(response);
      setNotice(enabled ? '服务已启用。' : '服务已停用。');
      await loadServers();
    });

  const handleSaveServer = (draft: MCPServerDraft) =>
    runMutation('save', async () => {
      if (!spaceId) {
        throw new Error('缺少工作空间上下文');
      }
      const response = await upsertMCPToolServer({
        server_id: editingServer?.server_id,
        space_id: editingServer?.space_id || spaceId,
        name: draft.name,
        description: draft.description,
        server_type: draft.serverType,
        enabled: editingServer?.enabled ?? true,
        config: draft.config,
        auth: draft.auth,
        tools: [],
      });
      ensureSuccessfulWorkbenchResponse(response);
      setFormVisible(false);
      setEditingServer(undefined);
      setNotice(editingServer ? '服务配置已更新。' : 'MCP 服务已创建。');
      await loadServers();
    });

  const openOfficialInstaller = (entry: MCPOfficialCatalogEntry) => {
    if (entry.availability === 'adapter_required') {
      return;
    }
    setOfficialCandidate(entry);
    setOfficialCredentials({});
    setOfficialFormError('');
  };

  const handleInstallOfficial = () => {
    if (!officialCandidate || !spaceId) {
      return;
    }
    const missingField = officialCandidate.credential_fields.find(
      field => field.required && !officialCredentials[field.key]?.trim(),
    );
    if (missingField) {
      setOfficialFormError(`请输入${missingField.label}`);
      return;
    }
    void runMutation(`install-${officialCandidate.catalog_id}`, async () => {
      const response = await installMCPOfficialCatalog({
        catalog_id: officialCandidate.catalog_id,
        space_id: spaceId,
        credentials: officialCredentials,
      });
      ensureSuccessfulWorkbenchResponse(response);
      setOfficialCandidate(undefined);
      setOfficialCredentials({});
      setOfficialFormError('');
      setNotice('官方服务已保存，请确认配置后启用。');
      await loadServers();
    });
  };

  const handleDiscover = (server: MCPToolServer) =>
    runMutation(`discover-${server.server_id}`, async () => {
      const response = await discoverMCPToolServer({
        server_id: server.server_id,
      });
      ensureSuccessfulWorkbenchResponse(response);
      setNotice('能力列表已刷新。');
      await loadServers();
      setCapabilityServer(current =>
        current?.server_id === server.server_id
          ? {
              ...current,
              tools: response.data?.tools ?? [],
              resources: response.data?.resources ?? [],
              prompts: response.data?.prompts ?? [],
            }
          : current,
      );
    });

  const loadAuditEvents = async (
    serverID: string,
    cursor = '',
    append = false,
  ) => {
    if (auditLoading) {
      return;
    }
    setAuditLoading(true);
    setAuditError('');
    try {
      const response = await listMCPToolAuditEvents({
        server_id: serverID,
        limit: 20,
        cursor: cursor || undefined,
      });
      ensureSuccessfulWorkbenchResponse(response);
      setAuditEvents(current =>
        append
          ? [...current, ...(response.data?.events ?? [])]
          : (response.data?.events ?? []),
      );
      setAuditCursor(response.data?.next_cursor ?? '');
      setAuditLoaded(true);
    } catch (auditLoadError) {
      void auditLoadError;
      setAuditError('调用日志暂时不可用，请稍后重试。');
      setAuditLoaded(true);
    } finally {
      setAuditLoading(false);
    }
  };

  const handleOpenCapabilities = (server: MCPToolServer) => {
    setCapabilityServer(server);
    setAuditEvents([]);
    setAuditCursor('');
    setAuditError('');
    setAuditLoaded(false);
  };

  const handleRunTool = async (argumentsJSON: string) => {
    if (!capabilityServer || !testTool) {
      return undefined;
    }
    const response = await testMCPToolCall({
      server_id: capabilityServer.server_id,
      tool_name: testTool.name,
      arguments: argumentsJSON,
    });
    ensureSuccessfulWorkbenchResponse(response);
    setAuditLoaded(false);
    void loadAuditEvents(capabilityServer.server_id);
    void loadServers();
    return response.data;
  };

  const handleExport = (server: MCPToolServer) =>
    runMutation(`export-${server.server_id}`, async () => {
      const response = await exportMCPToolServer({
        server_id: server.server_id,
      });
      ensureSuccessfulWorkbenchResponse(response);
      setExportedServer(response.data);
      setExportedServerID(server.server_id);
    });

  const handleDelete = () => {
    if (!deleteCandidate) {
      return;
    }
    void runMutation(`delete-${deleteCandidate.server_id}`, async () => {
      const response = await deleteMCPToolServer({
        server_id: deleteCandidate.server_id,
      });
      ensureSuccessfulWorkbenchResponse(response);
      setDeleteCandidate(undefined);
      setNotice('MCP 服务已删除。');
      await loadServers();
    });
  };

  const exportText = exportedServer
    ? JSON.stringify(exportedServer, null, JSON_INDENT)
    : '';

  const handleDownloadExport = () => {
    if (!exportText || typeof URL.createObjectURL !== 'function') {
      return;
    }
    const url = URL.createObjectURL(
      new Blob([exportText], { type: 'application/json' }),
    );
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = `mcp-server-${exportedServerID}.json`;
    anchor.click();
    URL.revokeObjectURL(url);
  };

  const renderCard = (server: MCPToolServer) => {
    const updating = busyAction === `toggle-${server.server_id}`;
    const isOfficial = server.source_type === 'official';
    return (
      <article className="mcp-management-card" key={server.server_id}>
        <div className="mcp-management-card-top">
          <div className="mcp-management-service-icon" aria-hidden>
            {server.name.slice(0, 1).toUpperCase()}
          </div>
          <div className="mcp-management-card-title">
            <div>
              <h2>{server.name}</h2>
              <span data-source={server.source_type || 'custom'}>
                {isOfficial ? '官方' : '自定义'}
              </span>
            </div>
            <p>{server.description || '暂无 MCP 服务描述'}</p>
          </div>
          <Switch
            size="small"
            checked={server.enabled}
            loading={updating}
            disabled={canManage !== true || Boolean(busyAction)}
            aria-label={(server.enabled ? '关闭 ' : '开启 ') + server.name}
            onChange={checked => void handleToggleEnabled(server, checked)}
          />
        </div>

        <div className="mcp-management-card-meta">
          <span>{server.server_type}</span>
          <span>{server.tools.length} 个工具</span>
          <span>{server.resources.length} 个资源</span>
        </div>

        <div className="mcp-management-card-status">
          <span
            data-health={server.enabled ? server.health_status : 'disabled'}
          >
            <i />
            {healthLabel(server)}
          </span>
          <time>
            {server.health_latency_ms > 0
              ? `${server.health_latency_ms} ms`
              : `更新于 ${formatTime(server.updated_at)}`}
          </time>
        </div>

        {mode === 'full' ? (
          <footer>
            <Button
              size="small"
              theme="outline"
              onClick={() => handleOpenCapabilities(server)}
            >
              查看能力
            </Button>
            {!isOfficial && canManage === true ? (
              <details className="mcp-management-more">
                <summary>更多</summary>
                <div>
                  <button
                    type="button"
                    onClick={() => void handleDiscover(server)}
                  >
                    刷新能力
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setEditingServer(server);
                      setFormVisible(true);
                    }}
                  >
                    编辑配置
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleExport(server)}
                  >
                    服务导出
                  </button>
                  <button
                    type="button"
                    className="danger"
                    onClick={() => setDeleteCandidate(server)}
                  >
                    删除
                  </button>
                </div>
              </details>
            ) : isOfficial || canManage === false ? (
              <span className="mcp-management-readonly">
                {isOfficial ? '官方只读' : '只读'}
              </span>
            ) : null}
          </footer>
        ) : null}
      </article>
    );
  };

  const renderOfficialCard = (entry: MCPOfficialCatalogEntry) => {
    const { installation } = entry;
    const adapterRequired = entry.availability === 'adapter_required';
    const installedServer = installation
      ? servers.find(server => server.server_id === installation.server_id)
      : undefined;
    const installed = entry.install_status === 'installed';
    const needsMigration = entry.install_status === 'needs_migration';
    const updating = installedServer
      ? busyAction === `toggle-${installedServer.server_id}`
      : false;
    return (
      <article
        className="mcp-management-card mcp-management-official-card"
        key={entry.catalog_id}
      >
        <div className="mcp-management-card-top">
          <div
            className="mcp-management-service-icon mcp-management-official-icon"
            aria-hidden
          >
            <span className="mcp-management-official-icon-fallback">
              {entry.catalog_id === 'postgres' ? (
                <IconCozDatabase />
              ) : (
                <IconCozPlugin />
              )}
            </span>
            {entry.icon_url ? (
              <img
                src={entry.icon_url}
                alt=""
                referrerPolicy="no-referrer"
                onError={event => {
                  event.currentTarget.hidden = true;
                }}
              />
            ) : null}
          </div>
          <div className="mcp-management-card-title">
            <div>
              <h2>{entry.name}</h2>
              <span data-source="official">
                {entry.publisher || '平台官方'}
              </span>
              {adapterRequired ? (
                <span data-source="adapter">需要平台适配</span>
              ) : null}
            </div>
            <p>{entry.description}</p>
          </div>
          {installedServer ? (
            <Switch
              size="small"
              checked={installedServer.enabled}
              loading={updating}
              disabled={canManage !== true || Boolean(busyAction)}
              aria-label={
                (installedServer.enabled ? '关闭 ' : '开启 ') + entry.name
              }
              onChange={checked =>
                void handleToggleEnabled(installedServer, checked)
              }
            />
          ) : null}
        </div>
        <div className="mcp-management-card-meta">
          <span>{entry.server_type}</span>
          <span>
            {entry.tools.length > 0
              ? `${entry.tools.length} 个工具`
              : adapterRequired
                ? '平台适配中'
                : '安装后发现能力'}
          </span>
          <span>
            {adapterRequired
              ? '暂不可安装'
              : installed
                ? '已安装'
                : needsMigration
                  ? '待迁移'
                  : '可安装'}
          </span>
        </div>
        <div className="mcp-management-card-status">
          <span
            data-health={
              adapterRequired
                ? 'adapter'
                : installation?.enabled
                  ? installation.health_status || 'unknown'
                  : 'disabled'
            }
          >
            <i />
            {adapterRequired
              ? '等待平台接入'
              : installedServer
                ? healthLabel(installedServer)
                : '尚未启用'}
          </span>
          <time>
            {adapterRequired
              ? '目录信息已同步'
              : installation?.updated_at
                ? `更新于 ${formatTime(installation.updated_at)}`
                : '工作空间级安装'}
          </time>
        </div>
        {adapterRequired && entry.availability_reason ? (
          <p
            className="mcp-management-adapter-reason"
            title={entry.availability_reason}
          >
            {entry.availability_reason}
          </p>
        ) : null}
        <footer>
          {installedServer ? (
            <Button
              size="small"
              theme="outline"
              onClick={() => handleOpenCapabilities(installedServer)}
            >
              查看能力
            </Button>
          ) : (
            <span className="mcp-management-readonly">
              {adapterRequired ? entry.publisher || '平台官方' : '凭据加密保存'}
            </span>
          )}
          <Button
            size="small"
            theme={installed ? 'borderless' : 'solid'}
            type={installed ? 'tertiary' : 'primary'}
            disabled={
              adapterRequired || canManage !== true || Boolean(busyAction)
            }
            title={adapterRequired ? entry.availability_reason : undefined}
            onClick={() => openOfficialInstaller(entry)}
          >
            {adapterRequired
              ? '需要平台适配'
              : installed
                ? '重新配置'
                : needsMigration
                  ? '迁移并配置'
                  : '安装'}
          </Button>
        </footer>
      </article>
    );
  };

  if (!spaceId) {
    return (
      <section className="mcp-management-panel mcp-management-panel-compact">
        <div className="mcp-management-empty">
          暂无空间上下文，无法加载 MCP 服务。
        </div>
      </section>
    );
  }

  return (
    <section
      className={`mcp-management-panel ${
        mode === 'compact'
          ? 'mcp-management-panel-compact'
          : 'mcp-management-panel-full'
      }`}
      aria-label="MCP 配置"
    >
      {mode === 'full' ? (
        <>
          <div className="mcp-management-toolbar">
            <div className="mcp-management-source-tabs" role="tablist">
              <button
                type="button"
                role="tab"
                aria-selected={sourceFilter === 'custom'}
                onClick={() => setSourceFilter('custom')}
              >
                自定义服务
              </button>
              <button
                type="button"
                role="tab"
                aria-selected={sourceFilter === 'official'}
                onClick={() => setSourceFilter('official')}
              >
                官方服务
              </button>
            </div>
            <Input
              aria-label="搜索 MCP 服务"
              autoComplete="off"
              name="mcp-service-search"
              value={keyword}
              showClear
              placeholder="搜索 MCP 服务"
              onChange={setKeyword}
            />
            {sourceFilter === 'custom' ? (
              <Button
                theme="solid"
                type="primary"
                disabled={canManage !== true}
                onClick={() => {
                  setEditingServer(undefined);
                  setFormVisible(true);
                }}
              >
                + 新建 MCP 服务
              </Button>
            ) : null}
          </div>
          {sourceFilter === 'custom' ? (
            <div className="mcp-management-filters">
              <label>
                <span>创建者</span>
                <select
                  aria-label="创建者筛选"
                  value={creatorFilter}
                  onChange={event =>
                    setCreatorFilter(event.target.value as CreatorFilter)
                  }
                >
                  <option value="all">全部创建者</option>
                  <option value="me">我创建的</option>
                </select>
              </label>
              <label>
                <span>部署状态</span>
                <select
                  aria-label="部署状态筛选"
                  value={statusFilter}
                  onChange={event =>
                    setStatusFilter(event.target.value as StatusFilter)
                  }
                >
                  <option value="all">全部状态</option>
                  <option value="enabled">已启用</option>
                  <option value="disabled">已停用</option>
                  <option value="healthy">运行正常</option>
                  <option value="unhealthy">连接异常</option>
                </select>
              </label>
              <Button
                size="small"
                theme="borderless"
                type="tertiary"
                loading={loading}
                onClick={() => void loadServers()}
              >
                刷新
              </Button>
              {loadStatus === 'success' && canManage === false ? (
                <span className="mcp-management-permission-tip">
                  当前空间为只读权限
                </span>
              ) : null}
            </div>
          ) : (
            <div className="mcp-management-official-tip">
              <span>
                官方目录由平台维护，共收录 {officialCatalog.length} 个服务。
                可安装条目的凭据仅在当前工作空间加密保存。
              </span>
              <Button
                size="small"
                theme="borderless"
                type="tertiary"
                loading={loading}
                onClick={() => void loadServers()}
              >
                刷新
              </Button>
            </div>
          )}
        </>
      ) : null}

      {error ? (
        <div
          className="mcp-management-alert mcp-management-alert-error"
          role="alert"
        >
          {error}
          <button type="button" onClick={() => void loadServers()}>
            重试
          </button>
        </div>
      ) : null}
      {notice ? (
        <div className="mcp-management-alert mcp-management-alert-success">
          {notice}
        </div>
      ) : null}

      {loading && servers.length === 0 ? (
        <div className="mcp-management-loading">
          <Spin />
          <span>正在加载 MCP 服务...</span>
        </div>
      ) : null}

      {!loading &&
      !error &&
      (sourceFilter === 'official'
        ? visibleOfficialCatalog.length === 0
        : visibleServers.length === 0) ? (
        <div className="mcp-management-empty">
          <span aria-hidden>M</span>
          <h2>{keyword ? '没有找到匹配的服务' : '暂无 MCP 服务'}</h2>
          <p>
            {sourceFilter === 'official'
              ? '当前没有可用的官方服务。'
              : '新建服务后，系统会自动发现工具、资源和提示词。'}
          </p>
          {mode === 'full' &&
          canManage === true &&
          sourceFilter === 'custom' ? (
            <Button
              theme="solid"
              type="primary"
              onClick={() => setFormVisible(true)}
            >
              新建 MCP 服务
            </Button>
          ) : null}
        </div>
      ) : null}

      {!error && sourceFilter === 'custom' && visibleServers.length > 0 ? (
        <div className="mcp-management-grid">
          {visibleServers.map(renderCard)}
        </div>
      ) : null}

      {!error &&
      sourceFilter === 'official' &&
      visibleOfficialCatalog.length > 0 ? (
        <div className="mcp-management-grid">
          {visibleOfficialCatalog.map(renderOfficialCard)}
        </div>
      ) : null}

      <MCPServerFormSheet
        server={editingServer}
        submitting={busyAction === 'save'}
        visible={formVisible}
        onCancel={() => {
          setFormVisible(false);
          setEditingServer(undefined);
        }}
        onSubmit={handleSaveServer}
      />

      <Modal
        title={
          officialCandidate?.install_status === 'installed'
            ? `重新配置 ${officialCandidate?.name ?? ''}`
            : `安装 ${officialCandidate?.name ?? ''}`
        }
        visible={Boolean(officialCandidate)}
        okText="保存配置"
        cancelText="取消"
        confirmLoading={
          busyAction === `install-${officialCandidate?.catalog_id ?? ''}`
        }
        onCancel={() => {
          setOfficialCandidate(undefined);
          setOfficialCredentials({});
          setOfficialFormError('');
        }}
        onOk={handleInstallOfficial}
      >
        <div className="mcp-management-official-form">
          <div className="mcp-management-form-intro">
            <strong>接入信息仅对当前工作空间生效</strong>
            <p>敏感凭据将加密保存，提交后不会再次回显。</p>
          </div>
          {officialFormError ? (
            <div
              className="mcp-management-alert mcp-management-alert-error"
              role="alert"
            >
              {officialFormError}
            </div>
          ) : null}
          {officialCandidate?.credential_fields.map(field => (
            <label className="mcp-management-field" key={field.key}>
              <span>
                {field.label} {field.required ? <b>*</b> : null}
              </span>
              <Input
                aria-label={field.label}
                autoComplete={field.secret ? 'new-password' : 'off'}
                mode={field.secret ? 'password' : undefined}
                name={`official-mcp-${officialCandidate.catalog_id}-${field.key}`}
                value={officialCredentials[field.key] ?? ''}
                placeholder={field.placeholder}
                onChange={value => {
                  setOfficialFormError('');
                  setOfficialCredentials(current => ({
                    ...current,
                    [field.key]: value,
                  }));
                }}
              />
              <small>{field.description}</small>
            </label>
          ))}
        </div>
      </Modal>

      <MCPCapabilitySheet
        auditError={auditError}
        auditEvents={auditEvents}
        auditLoading={auditLoading}
        auditNextCursor={auditCursor}
        canManage={canManage === true}
        discovering={
          busyAction === `discover-${capabilityServer?.server_id ?? ''}`
        }
        server={capabilityServer}
        visible={Boolean(capabilityServer)}
        onCancel={() => setCapabilityServer(undefined)}
        onAuditActivate={() => {
          if (capabilityServer && !auditLoaded && !auditLoading) {
            void loadAuditEvents(capabilityServer.server_id);
          }
        }}
        onDiscover={server => void handleDiscover(server)}
        onLoadMoreAudit={() => {
          if (capabilityServer) {
            void loadAuditEvents(capabilityServer.server_id, auditCursor, true);
          }
        }}
        onRetryAudit={() => {
          if (capabilityServer) {
            setAuditLoaded(false);
            void loadAuditEvents(capabilityServer.server_id);
          }
        }}
        onTestTool={setTestTool}
      />

      <MCPTestModal
        tool={testTool}
        visible={Boolean(testTool)}
        onCancel={() => setTestTool(undefined)}
        onRun={handleRunTool}
      />

      <Modal
        title="服务导出"
        visible={Boolean(exportedServer)}
        okText="下载 JSON"
        cancelText="关闭"
        onCancel={() => setExportedServer(undefined)}
        onOk={handleDownloadExport}
      >
        <div className="mcp-management-export">
          <p>导出内容不包含认证信息或其他敏感凭据。</p>
          <pre>{exportText}</pre>
        </div>
      </Modal>

      <Modal
        title="删除 MCP 服务"
        visible={Boolean(deleteCandidate)}
        okText="确认删除"
        cancelText="取消"
        confirmLoading={
          busyAction === `delete-${deleteCandidate?.server_id ?? ''}`
        }
        onCancel={() => setDeleteCandidate(undefined)}
        onOk={handleDelete}
      >
        <p className="mcp-management-delete-warning">
          删除「{deleteCandidate?.name}
          」后，相关工具将立即从工作空间中移除，且无法恢复。
        </p>
      </Modal>
    </section>
  );
};
import './index.less';
