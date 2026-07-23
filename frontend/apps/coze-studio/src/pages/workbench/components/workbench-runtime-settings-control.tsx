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

import { type Dispatch, type SetStateAction, useEffect, useState } from 'react';

import { IconCozSetting } from '@coze-arch/coze-design/icons';
import { Input } from '@coze-arch/coze-design';

import type {
  WorkbenchReasoningEffort,
  WorkbenchRuntimeSettings,
} from './types';

// eslint-disable-next-line @typescript-eslint/no-magic-numbers -- User-visible memory recall presets.
const MEMORY_LIMIT_OPTIONS = [3, 5, 8] as const;
const CANDIDATE_LIMIT_MULTIPLIER = 4;
const REASONING_OPTIONS: Array<{
  ariaLabel: string;
  enabled: boolean;
  effort: WorkbenchReasoningEffort;
  label: string;
}> = [
  {
    ariaLabel: '模型推理 关闭',
    enabled: false,
    effort: 'medium',
    label: '关闭',
  },
  {
    ariaLabel: '模型推理 中',
    enabled: true,
    effort: 'medium',
    label: '中',
  },
  {
    ariaLabel: '模型推理 高',
    enabled: true,
    effort: 'high',
    label: '高',
  },
];
type WorkbenchRuntimeSettingsChange = Dispatch<
  SetStateAction<WorkbenchRuntimeSettings>
>;
type WorkbenchRuntimeSettingsUpdate = (
  resolveNext: (
    current: WorkbenchRuntimeSettings,
  ) => Partial<WorkbenchRuntimeSettings>,
) => void;

const parseAllowedHostsInput = (value: string) => {
  const seen = new Set<string>();

  return value
    .split(',')
    .map(host => host.trim())
    .filter(host => {
      if (!host || seen.has(host)) {
        return false;
      }
      seen.add(host);
      return true;
    });
};

const WorkbenchModelReliabilitySettingsRows = ({
  failoverCandidateCount,
  settings,
  update,
}: {
  failoverCandidateCount: number;
  settings: WorkbenchRuntimeSettings;
  update: WorkbenchRuntimeSettingsUpdate;
}) => (
  <>
    <div className="chat-workbench-runtime-row">
      <span>模型重试</span>
      <button
        type="button"
        aria-label="模型重试"
        aria-pressed={settings.model_retry.enabled}
        data-active={settings.model_retry.enabled}
        onClick={() =>
          update(current => ({
            model_retry: {
              ...current.model_retry,
              enabled: !current.model_retry.enabled,
            },
          }))
        }
      >
        {settings.model_retry.enabled ? '已开启' : '关闭'}
      </button>
    </div>
    <div className="chat-workbench-runtime-row">
      <span>模型切换</span>
      <button
        type="button"
        aria-label="模型切换"
        aria-pressed={settings.model_failover.enabled}
        data-active={settings.model_failover.enabled}
        disabled={failoverCandidateCount === 0}
        onClick={() =>
          update(current => ({
            model_failover: {
              ...current.model_failover,
              enabled: !current.model_failover.enabled,
            },
          }))
        }
      >
        {failoverCandidateCount > 0
          ? [
              settings.model_failover.enabled ? '已开启' : '关闭',
              failoverCandidateCount,
            ].join(' · ')
          : '无候选'}
      </button>
    </div>
  </>
);

const WorkbenchReasoningSettingsRow = ({
  settings,
  update,
}: {
  settings: WorkbenchRuntimeSettings;
  update: WorkbenchRuntimeSettingsUpdate;
}) => (
  <div className="chat-workbench-runtime-row">
    <span>模型推理</span>
    <div className="chat-workbench-runtime-options">
      {REASONING_OPTIONS.map(option => {
        const active =
          settings.reasoning.enabled === option.enabled &&
          (!option.enabled || settings.reasoning.effort === option.effort);

        return (
          <button
            key={option.ariaLabel}
            type="button"
            aria-label={option.ariaLabel}
            aria-pressed={active}
            data-active={active}
            onClick={() =>
              update(current => ({
                reasoning: {
                  ...current.reasoning,
                  enabled: option.enabled,
                  effort: option.effort,
                },
              }))
            }
          >
            {option.label}
          </button>
        );
      })}
    </div>
  </div>
);

const WorkbenchResourceRuntimeSettingsRows = ({
  settings,
  update,
}: {
  settings: WorkbenchRuntimeSettings;
  update: WorkbenchRuntimeSettingsUpdate;
}) => {
  const skillCount = settings.skills.allowed_skills.length;
  const mcpCount = settings.mcp_tools.allowed_tools.length;

  return (
    <>
      <div className="chat-workbench-runtime-row">
        <span>Skill</span>
        <button
          type="button"
          aria-label="Skill 调用"
          aria-pressed={settings.skills.enabled}
          data-active={settings.skills.enabled}
          disabled={skillCount === 0}
          onClick={() =>
            update(current => ({
              skills: {
                ...current.skills,
                enabled:
                  !current.skills.enabled &&
                  current.skills.allowed_skills.length > 0,
              },
            }))
          }
        >
          {skillCount > 0
            ? [settings.skills.enabled ? '已开启' : '关闭', skillCount].join(
                ' · ',
              )
            : '无可用'}
        </button>
      </div>
      <div className="chat-workbench-runtime-row">
        <span>MCP 工具</span>
        <button
          type="button"
          aria-label="MCP 工具调用"
          aria-pressed={settings.mcp_tools.enabled}
          data-active={settings.mcp_tools.enabled}
          onClick={() =>
            update(current => ({
              mcp_tools: {
                ...current.mcp_tools,
                enabled: !current.mcp_tools.enabled,
              },
            }))
          }
        >
          {mcpCount > 0
            ? [settings.mcp_tools.enabled ? '已开启' : '关闭', mcpCount].join(
                ' · ',
              )
            : settings.mcp_tools.enabled
              ? '自动发现'
              : '关闭'}
        </button>
      </div>
    </>
  );
};

const WorkbenchWebToolSettingsRows = ({
  settings,
  update,
}: {
  settings: WorkbenchRuntimeSettings;
  update: WorkbenchRuntimeSettingsUpdate;
}) => {
  const [allowedHostsInput, setAllowedHostsInput] = useState(() =>
    settings.web_tools.http.allowed_hosts.join(', '),
  );
  const updateWebTools = (
    resolveWebTools: (
      current: WorkbenchRuntimeSettings['web_tools'],
    ) => Partial<WorkbenchRuntimeSettings['web_tools']>,
  ) =>
    update(current => ({
      web_tools: {
        ...current.web_tools,
        ...resolveWebTools(current.web_tools),
      },
    }));
  const webFetchHostCount = settings.web_tools.http.allowed_hosts.length;

  return (
    <>
      <div className="chat-workbench-runtime-row">
        <span>联网搜索</span>
        <button
          type="button"
          aria-pressed={settings.web_tools.search.enabled}
          data-active={settings.web_tools.search.enabled}
          onClick={() => {
            updateWebTools(current => {
              const searchEnabled = !current.search.enabled;

              return {
                enabled: searchEnabled || current.http.enabled,
                search: {
                  ...current.search,
                  enabled: searchEnabled,
                },
              };
            });
          }}
        >
          {settings.web_tools.search.enabled ? '已开启' : '关闭'}
        </button>
      </div>
      <div className="chat-workbench-runtime-row">
        <span>网页读取</span>
        <button
          type="button"
          aria-label="网页读取"
          aria-pressed={settings.web_tools.http.enabled}
          data-active={settings.web_tools.http.enabled}
          disabled={webFetchHostCount === 0}
          onClick={() => {
            updateWebTools(current => {
              const httpEnabled =
                !current.http.enabled && current.http.allowed_hosts.length > 0;

              return {
                enabled: current.search.enabled || httpEnabled,
                http: {
                  ...current.http,
                  enabled: httpEnabled,
                },
              };
            });
          }}
        >
          {webFetchHostCount > 0
            ? [
                settings.web_tools.http.enabled ? '已开启' : '关闭',
                webFetchHostCount,
              ].join(' · ')
            : '需域名'}
        </button>
      </div>
      <div className="chat-workbench-runtime-row chat-workbench-runtime-input-row">
        <span>允许域名</span>
        <Input
          aria-label="网页读取允许域名"
          className="chat-workbench-runtime-host-input"
          placeholder="example.com, docs.example.com"
          showClear
          size="small"
          value={allowedHostsInput}
          onChange={value => {
            setAllowedHostsInput(value);
            const allowedHosts = parseAllowedHostsInput(value);

            updateWebTools(current => {
              const httpEnabled =
                current.http.enabled && allowedHosts.length > 0;

              return {
                enabled: current.search.enabled || httpEnabled,
                http: {
                  ...current.http,
                  enabled: httpEnabled,
                  allowed_hosts: allowedHosts,
                },
              };
            });
          }}
        />
      </div>
    </>
  );
};

const WorkbenchRuntimeSettingsPanel = ({
  failoverCandidateCount,
  open,
  settings,
  onChange,
}: {
  failoverCandidateCount: number;
  open: boolean;
  settings: WorkbenchRuntimeSettings;
  onChange: WorkbenchRuntimeSettingsChange;
}) => {
  if (!open) {
    return null;
  }

  const update: WorkbenchRuntimeSettingsUpdate = resolveNext =>
    onChange(current => ({
      ...current,
      ...resolveNext(current),
    }));

  return (
    <div className="chat-workbench-runtime-panel" aria-label="运行设置">
      <div className="chat-workbench-runtime-row">
        <span>运行内核</span>
        <strong>Eino ADK</strong>
      </div>
      <WorkbenchReasoningSettingsRow settings={settings} update={update} />
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
                update(current => ({
                  memory_retrieval: {
                    ...current.memory_retrieval,
                    limit,
                    candidate_limit: Math.max(
                      limit * CANDIDATE_LIMIT_MULTIPLIER,
                      limit,
                    ),
                  },
                }))
              }
            >
              {limit}
            </button>
          ))}
        </div>
      </div>
      <WorkbenchResourceRuntimeSettingsRows
        settings={settings}
        update={update}
      />
      <WorkbenchModelReliabilitySettingsRows
        failoverCandidateCount={failoverCandidateCount}
        settings={settings}
        update={update}
      />
      <WorkbenchWebToolSettingsRows settings={settings} update={update} />
      <div className="chat-workbench-runtime-row">
        <span>Token 用量</span>
        <button
          type="button"
          aria-pressed={settings.token_usage.enabled}
          data-active={settings.token_usage.enabled}
          onClick={() =>
            update(current => ({
              token_usage: {
                enabled: !current.token_usage.enabled,
              },
            }))
          }
        >
          {settings.token_usage.enabled ? '显示' : '隐藏'}
        </button>
      </div>
    </div>
  );
};

export const WorkbenchRuntimeSettingsControl = ({
  disabled = false,
  failoverCandidateCount = 0,
  settings,
  onChange,
}: {
  disabled?: boolean;
  failoverCandidateCount?: number;
  settings: WorkbenchRuntimeSettings;
  onChange: WorkbenchRuntimeSettingsChange;
}) => {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (disabled) {
      setOpen(false);
    }
  }, [disabled]);

  return (
    <>
      <button
        type="button"
        className="chat-workbench-runtime-trigger"
        aria-label="运行设置"
        aria-expanded={open}
        disabled={disabled}
        onClick={() => {
          if (!disabled) {
            setOpen(currentOpen => !currentOpen);
          }
        }}
      >
        <IconCozSetting />
        <span>运行设置</span>
      </button>
      <WorkbenchRuntimeSettingsPanel
        failoverCandidateCount={failoverCandidateCount}
        open={open && !disabled}
        settings={settings}
        onChange={onChange}
      />
    </>
  );
};
