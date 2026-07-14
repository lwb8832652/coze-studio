/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

import type { workbenchTool } from '@coze-studio/api-schema';
import { Button, SideSheet, Spin, TabPane, Tabs } from '@coze-arch/coze-design';

type MCPToolServer = workbenchTool.MCPToolServer;
type MCPToolDefinition = workbenchTool.MCPToolDefinition;
type MCPToolAuditEvent = workbenchTool.MCPToolAuditEvent;

/* eslint-disable complexity, @coze-arch/max-line-per-function -- Capability drawer renders four bounded views. */

interface MCPCapabilitySheetProps {
  auditError: string;
  auditEvents: MCPToolAuditEvent[];
  auditLoading: boolean;
  auditNextCursor: string;
  canManage: boolean;
  discovering: boolean;
  server?: MCPToolServer;
  visible: boolean;
  onCancel: () => void;
  onAuditActivate: () => void;
  onDiscover: (server: MCPToolServer) => void;
  onLoadMoreAudit: () => void;
  onRetryAudit: () => void;
  onTestTool: (tool: MCPToolDefinition) => void;
}

const formatTime = (timestamp?: number) => {
  if (!timestamp) {
    return '-';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(timestamp));
};

const EmptyCapability = ({ children }: { children: string }) => (
  <div className="mcp-management-empty-compact">{children}</div>
);

export const MCPCapabilitySheet = ({
  auditError,
  auditEvents,
  auditLoading,
  auditNextCursor,
  canManage,
  discovering,
  server,
  visible,
  onCancel,
  onAuditActivate,
  onDiscover,
  onLoadMoreAudit,
  onRetryAudit,
  onTestTool,
}: MCPCapabilitySheetProps) => (
  <SideSheet
    className="mcp-management-capability-sheet"
    title={server ? `${server.name} · 能力与日志` : '能力与日志'}
    visible={visible}
    width={760}
    closeOnEsc
    bodyStyle={{ padding: 0 }}
    onCancel={onCancel}
  >
    {server ? (
      <>
        <header className="mcp-management-capability-hero">
          <div className="mcp-management-service-icon" aria-hidden>
            {server.name.slice(0, 1).toUpperCase()}
          </div>
          <div>
            <h2>{server.name}</h2>
            <p>{server.description || '暂无服务描述'}</p>
            <div className="mcp-management-capability-meta">
              <span>{server.server_type}</span>
              <span>
                {server.source_type === 'official' ? '官方服务' : '自定义服务'}
              </span>
              <span>{server.enabled ? '已启用' : '已停用'}</span>
            </div>
          </div>
          {server.source_type !== 'official' && canManage ? (
            <Button
              size="small"
              theme="solid"
              type="primary"
              loading={discovering}
              onClick={() => onDiscover(server)}
            >
              刷新能力
            </Button>
          ) : null}
        </header>

        <Tabs
          className="mcp-management-capability-tabs"
          defaultActiveKey="tools"
          onChange={itemKey => {
            if (itemKey === 'audit') {
              onAuditActivate();
            }
          }}
        >
          <TabPane tab={`工具 ${server.tools.length}`} itemKey="tools">
            <div className="mcp-management-capability-list">
              {server.tools.map(tool => (
                <article key={tool.name}>
                  <div>
                    <h3>{tool.name}</h3>
                    <p>{tool.description || '暂无工具描述'}</p>
                  </div>
                  <Button
                    size="small"
                    theme="outline"
                    disabled={!server.enabled}
                    onClick={() => onTestTool(tool)}
                  >
                    试运行
                  </Button>
                </article>
              ))}
              {server.tools.length === 0 ? (
                <EmptyCapability>尚未发现工具，请刷新能力。</EmptyCapability>
              ) : null}
            </div>
          </TabPane>

          <TabPane tab={`资源 ${server.resources.length}`} itemKey="resources">
            <div className="mcp-management-capability-list">
              {server.resources.map(resource => (
                <article key={resource.resource_id}>
                  <div>
                    <h3>{resource.name || '未命名资源'}</h3>
                    <p>{resource.description || '暂无资源描述'}</p>
                  </div>
                  <code>{resource.mime_type || 'resource'}</code>
                </article>
              ))}
              {server.resources.length === 0 ? (
                <EmptyCapability>该服务没有声明资源。</EmptyCapability>
              ) : null}
            </div>
          </TabPane>

          <TabPane tab={`提示词 ${server.prompts.length}`} itemKey="prompts">
            <div className="mcp-management-capability-list">
              {server.prompts.map(prompt => (
                <article key={prompt.name}>
                  <div>
                    <h3>{prompt.name}</h3>
                    <p>{prompt.description || '暂无提示词描述'}</p>
                  </div>
                  <code>{prompt.arguments.length} 个参数</code>
                </article>
              ))}
              {server.prompts.length === 0 ? (
                <EmptyCapability>该服务没有声明提示词。</EmptyCapability>
              ) : null}
            </div>
          </TabPane>

          <TabPane tab="调用日志" itemKey="audit">
            <div className="mcp-management-audit-list">
              {auditError ? (
                <div
                  className="mcp-management-alert mcp-management-alert-error"
                  role="alert"
                >
                  {auditError}
                  <button type="button" onClick={onRetryAudit}>
                    重试调用日志
                  </button>
                </div>
              ) : null}
              {!auditError && auditLoading && auditEvents.length === 0 ? (
                <Spin />
              ) : null}
              {auditEvents.map(event => (
                <article key={event.event_id}>
                  <span
                    className="mcp-management-audit-status"
                    data-status={event.status}
                  >
                    {event.status}
                  </span>
                  <div>
                    <h3>{event.tool_name}</h3>
                    <p>
                      操作人 {event.actor_id} · {formatTime(event.created_at)}
                    </p>
                    {event.error_summary ? (
                      <small>{event.error_summary}</small>
                    ) : null}
                  </div>
                  <time>{event.latency_ms} ms</time>
                </article>
              ))}
              {!auditError && !auditLoading && auditEvents.length === 0 ? (
                <EmptyCapability>暂无试运行记录。</EmptyCapability>
              ) : null}
              {auditNextCursor ? (
                <Button
                  block
                  theme="outline"
                  loading={auditLoading}
                  onClick={onLoadMoreAudit}
                >
                  加载更多
                </Button>
              ) : null}
            </div>
          </TabPane>
        </Tabs>
      </>
    ) : null}
  </SideSheet>
);
