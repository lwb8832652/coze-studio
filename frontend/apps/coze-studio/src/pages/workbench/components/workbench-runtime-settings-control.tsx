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

import { useState } from 'react';

import { IconCozSetting } from '@coze-arch/coze-design/icons';

import type { WorkbenchRuntimeSettings } from './types';

// eslint-disable-next-line @typescript-eslint/no-magic-numbers -- User-visible memory recall presets.
const MEMORY_LIMIT_OPTIONS = [3, 5, 8] as const;
const CANDIDATE_LIMIT_MULTIPLIER = 4;

const WorkbenchRuntimeSettingsPanel = ({
  open,
  settings,
  onChange,
}: {
  open: boolean;
  settings: WorkbenchRuntimeSettings;
  onChange: (settings: WorkbenchRuntimeSettings) => void;
}) => {
  if (!open) {
    return null;
  }

  const update = (next: Partial<WorkbenchRuntimeSettings>) =>
    onChange({
      ...settings,
      ...next,
    });
  const updateWebTools = (
    webTools: Partial<WorkbenchRuntimeSettings['web_tools']>,
  ) =>
    update({
      web_tools: {
        ...settings.web_tools,
        ...webTools,
      },
    });

  return (
    <div className="chat-workbench-runtime-panel" aria-label="运行设置">
      <div className="chat-workbench-runtime-row">
        <span>运行内核</span>
        <strong>Eino ADK</strong>
      </div>
      <div className="chat-workbench-runtime-row">
        <span>记忆检索</span>
        <div className="chat-workbench-runtime-options">
          {MEMORY_LIMIT_OPTIONS.map(limit => (
            <button
              key={limit}
              type="button"
              aria-pressed={settings.memory_retrieval.limit === limit}
              data-active={settings.memory_retrieval.limit === limit}
              onClick={() =>
                update({
                  memory_retrieval: {
                    ...settings.memory_retrieval,
                    limit,
                    candidate_limit: Math.max(
                      limit * CANDIDATE_LIMIT_MULTIPLIER,
                      limit,
                    ),
                  },
                })
              }
            >
              {limit}
            </button>
          ))}
        </div>
      </div>
      <div className="chat-workbench-runtime-row">
        <span>MCP 工具</span>
        <strong>{settings.mcp_tools.allowed_tools.length}</strong>
      </div>
      <div className="chat-workbench-runtime-row">
        <span>联网搜索</span>
        <button
          type="button"
          aria-pressed={settings.web_tools.search.enabled}
          data-active={settings.web_tools.search.enabled}
          onClick={() => {
            const enabled = !settings.web_tools.search.enabled;

            updateWebTools({
              enabled,
              search: {
                ...settings.web_tools.search,
                enabled,
              },
            });
          }}
        >
          {settings.web_tools.search.enabled ? '已开启' : '关闭'}
        </button>
      </div>
      <div className="chat-workbench-runtime-row">
        <span>Token 用量</span>
        <button
          type="button"
          aria-pressed={settings.token_usage.enabled}
          data-active={settings.token_usage.enabled}
          onClick={() =>
            update({
              token_usage: {
                enabled: !settings.token_usage.enabled,
              },
            })
          }
        >
          {settings.token_usage.enabled ? '显示' : '隐藏'}
        </button>
      </div>
    </div>
  );
};

export const WorkbenchRuntimeSettingsControl = ({
  settings,
  onChange,
}: {
  settings: WorkbenchRuntimeSettings;
  onChange: (settings: WorkbenchRuntimeSettings) => void;
}) => {
  const [open, setOpen] = useState(false);

  return (
    <>
      <button
        type="button"
        className="chat-workbench-runtime-trigger"
        aria-label="运行设置"
        aria-expanded={open}
        onClick={() => setOpen(currentOpen => !currentOpen)}
      >
        <IconCozSetting />
        <span>运行设置</span>
      </button>
      <WorkbenchRuntimeSettingsPanel
        open={open}
        settings={settings}
        onChange={onChange}
      />
    </>
  );
};
