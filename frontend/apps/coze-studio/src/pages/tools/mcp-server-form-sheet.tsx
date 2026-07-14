/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 */

import { useEffect, useState } from 'react';

import type { workbenchTool } from '@coze-studio/api-schema';
import { Button, Input, SideSheet, TextArea } from '@coze-arch/coze-design';

type MCPToolServer = workbenchTool.MCPToolServer;

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive create/edit drawer form. */

const JSON_INDENT = 2;

export interface MCPServerDraft {
  auth: string;
  config: string;
  description: string;
  name: string;
  serverType: string;
}

interface MCPServerFormSheetProps {
  server?: MCPToolServer;
  submitting: boolean;
  visible: boolean;
  onCancel: () => void;
  onSubmit: (draft: MCPServerDraft) => void | Promise<void>;
}

const CONFIG_TEMPLATES: Record<string, string> = {
  stdio: JSON.stringify(
    {
      command: 'npx',
      args: ['-y', '@modelcontextprotocol/server-example'],
      env: {},
    },
    null,
    JSON_INDENT,
  ),
  sse: JSON.stringify(
    { url: 'https://mcp.example.com/sse' },
    null,
    JSON_INDENT,
  ),
  streamable_http: JSON.stringify(
    { url: 'https://mcp.example.com/mcp' },
    null,
    JSON_INDENT,
  ),
};

const isJSONObject = (raw: string) => {
  try {
    const parsed = JSON.parse(raw);
    return (
      Boolean(parsed) && typeof parsed === 'object' && !Array.isArray(parsed)
    );
  } catch (parseError) {
    void parseError;
    return false;
  }
};

export const MCPServerFormSheet = ({
  server,
  submitting,
  visible,
  onCancel,
  onSubmit,
}: MCPServerFormSheetProps) => {
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [serverType, setServerType] = useState('stdio');
  const [config, setConfig] = useState(CONFIG_TEMPLATES.stdio);
  const [auth, setAuth] = useState('{}');
  const [error, setError] = useState('');

  useEffect(() => {
    if (!visible) {
      return;
    }
    setName(server?.name ?? '');
    setDescription(server?.description ?? '');
    setServerType(server?.server_type ?? 'stdio');
    setConfig(server?.config ?? CONFIG_TEMPLATES.stdio);
    setAuth(server ? '' : '{}');
    setError('');
  }, [server, visible]);

  const handleTypeChange = (nextType: string) => {
    setServerType(nextType);
    if (!server) {
      setConfig(CONFIG_TEMPLATES[nextType] ?? '{}');
      setAuth('{}');
    }
  };

  const handleSubmit = () => {
    if (!name.trim()) {
      setError('请输入服务名称。');
      return;
    }
    if (!description.trim()) {
      setError('请输入服务描述。');
      return;
    }
    if (!isJSONObject(config)) {
      setError('服务配置必须是有效的 JSON 对象。');
      return;
    }
    const normalizedAuth = auth.trim();
    if (normalizedAuth && !isJSONObject(normalizedAuth)) {
      setError('认证配置必须是有效的 JSON 对象。');
      return;
    }
    setError('');
    void onSubmit({
      auth: normalizedAuth || server?.auth || '{}',
      config: config.trim(),
      description: description.trim(),
      name: name.trim(),
      serverType,
    });
  };

  return (
    <SideSheet
      className="mcp-management-form-sheet"
      title={server ? '编辑 MCP 服务' : '新建 MCP 服务'}
      visible={visible}
      width={640}
      closeOnEsc
      onCancel={onCancel}
    >
      <div className="mcp-management-form">
        <div className="mcp-management-form-intro">
          <span className="mcp-management-form-mark" aria-hidden>
            M
          </span>
          <div>
            <strong>{server ? '更新服务连接' : '连接一个 MCP 服务'}</strong>
            <p>
              保存后由服务端发现工具、资源和提示词。认证信息只写入，不会在页面回显。
            </p>
          </div>
        </div>

        {error ? (
          <div
            className="mcp-management-alert mcp-management-alert-error"
            role="alert"
          >
            {error}
          </div>
        ) : null}

        <label className="mcp-management-field">
          <span>
            服务名称 <b>*</b>
          </span>
          <Input
            aria-label="服务名称"
            value={name}
            maxLength={128}
            placeholder="例如：知识库检索"
            onChange={setName}
          />
        </label>

        <label className="mcp-management-field">
          <span>
            服务描述 <b>*</b>
          </span>
          <TextArea
            aria-label="服务描述"
            value={description}
            maxLength={512}
            rows={3}
            autosize={false}
            placeholder="说明服务用途和可提供的能力"
            onChange={setDescription}
          />
        </label>

        <label className="mcp-management-field">
          <span>
            安装方式 <b>*</b>
          </span>
          <select
            aria-label="安装方式"
            value={serverType}
            disabled={Boolean(server)}
            onChange={event => handleTypeChange(event.target.value)}
          >
            <option value="stdio">stdio / 本地命令</option>
            <option value="sse">SSE</option>
            <option value="streamable_http">Streamable HTTP</option>
          </select>
          {server ? <small>安装方式创建后不可修改。</small> : null}
        </label>

        <label className="mcp-management-field">
          <span>
            服务配置 <b>*</b>
            <small> JSON</small>
          </span>
          <TextArea
            aria-label="服务配置 JSON"
            className="mcp-management-code-input"
            value={config}
            rows={10}
            autosize={false}
            onChange={setConfig}
          />
        </label>

        <label className="mcp-management-field">
          <span>
            认证配置{' '}
            <small>
              {server ? '留空保留现有认证，保存后不回显' : '选填，保存后不回显'}
            </small>
          </span>
          <TextArea
            aria-label="认证配置 JSON"
            className="mcp-management-code-input"
            value={auth}
            rows={6}
            autosize={false}
            placeholder={
              server
                ? '留空保留现有认证；输入 JSON 可替换'
                : '{"type":"bearer","token":"..."}'
            }
            onChange={setAuth}
          />
        </label>

        <div className="mcp-management-form-actions">
          <Button theme="borderless" type="tertiary" onClick={onCancel}>
            取消
          </Button>
          <Button
            theme="solid"
            type="primary"
            loading={submitting}
            disabled={submitting}
            onClick={handleSubmit}
          >
            保存并发现能力
          </Button>
        </div>
      </div>
    </SideSheet>
  );
};
