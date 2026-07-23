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

/* eslint-disable max-lines -- P0 extension overlay, split in phase 2. */

import { useNavigate, useParams } from 'react-router-dom';
import {
  type Dispatch,
  type SetStateAction,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import { workbenchSkill, type workbenchTool } from '@coze-studio/api-schema';
import {
  IconCozArrowDown,
  IconCozCheckMark,
  IconCozCode,
  IconCozMagnifier,
  IconCozPlugin,
  IconCozSetting,
  IconCozSkill,
  IconCozWorkflow,
} from '@coze-arch/coze-design/icons';
import { Button, Input, Spin, Tabs } from '@coze-arch/coze-design';

import { listMCPToolRegistryEntries } from '../tools/service';
import { listSkills } from '../skill/service';
import type { WorkbenchComposerOverlayPlacement } from './components/workbench-composer-at-menu';
import {
  createDefaultWorkbenchResourceSelection,
  type WorkbenchResourceSelection,
  type WorkbenchRuntimeSettings,
} from './components/types';

type Skill = workbenchSkill.Skill;
type MCPToolRegistryEntry = workbenchTool.MCPToolRegistryEntry;
type ExtensionTab = 'skills' | 'mcp';

interface MCPServerExtensionEntry {
  id: string;
  name: string;
  toolNames: string[];
}

interface ExtensionsPopoverProps {
  disabled?: boolean;
  open?: boolean;
  placement?: WorkbenchComposerOverlayPlacement;
  renderMask?: boolean;
  showSelectedCount?: boolean;
  value?: WorkbenchResourceSelection;
  settings?: WorkbenchRuntimeSettings;
  onChange?: (value: WorkbenchResourceSelection) => void;
  onSettingsChange?: Dispatch<SetStateAction<WorkbenchRuntimeSettings>>;
  onOpenChange?: (open: boolean) => void;
}

const defaultSelection = createDefaultWorkbenchResourceSelection();

const getNextResourceSelection = ({
  availableIds,
  resourceId,
  selection,
  tab,
  tabEnabled,
}: {
  availableIds: string[];
  resourceId: string;
  selection: WorkbenchResourceSelection;
  tab: ExtensionTab;
  tabEnabled: boolean;
}): { enabled: boolean; selection: WorkbenchResourceSelection } => {
  const key = tab === 'skills' ? 'enable_skills' : 'enable_mcp';
  const currentExplicitIds = selection[key].filter(id =>
    availableIds.includes(id),
  );
  const currentIds =
    tabEnabled && currentExplicitIds.length === 0
      ? availableIds
      : currentExplicitIds;
  const currentSet = new Set(currentIds);

  if (currentSet.has(resourceId)) {
    currentSet.delete(resourceId);
  } else {
    currentSet.add(resourceId);
  }

  const nextIds = availableIds.filter(id => currentSet.has(id));
  const nextEnabled = nextIds.length > 0;
  const nextExplicitIds =
    nextEnabled && nextIds.length === availableIds.length ? [] : nextIds;

  return {
    enabled: nextEnabled,
    selection: {
      ...selection,
      [key]: nextExplicitIds,
    },
  };
};

const getMCPServerId = (tool: MCPToolRegistryEntry) =>
  String(tool.server_id || tool.server_name || tool.name);

const getMCPServerEntries = (
  tools: MCPToolRegistryEntry[],
): MCPServerExtensionEntry[] => {
  const serverMap = new Map<string, MCPServerExtensionEntry>();

  for (const tool of tools) {
    if (!tool.enabled) {
      continue;
    }

    const serverId = getMCPServerId(tool);
    const current = serverMap.get(serverId);
    if (current) {
      current.toolNames.push(tool.name);
      continue;
    }

    serverMap.set(serverId, {
      id: serverId,
      name: tool.server_name || tool.name,
      toolNames: [tool.name],
    });
  }

  return Array.from(serverMap.values());
};

const getEnabledMCPServerIds = ({
  enabled,
  selectedToolNames,
  servers,
}: {
  enabled: boolean;
  selectedToolNames: string[];
  servers: MCPServerExtensionEntry[];
}) => {
  if (!enabled) {
    return [];
  }

  if (selectedToolNames.length === 0) {
    return servers.map(server => server.id);
  }

  const selectedToolNameSet = new Set(selectedToolNames);

  return servers
    .filter(server =>
      server.toolNames.every(toolName => selectedToolNameSet.has(toolName)),
    )
    .map(server => server.id);
};

const getNextMCPServerSelection = ({
  enabled,
  selection,
  serverId,
  servers,
}: {
  enabled: boolean;
  selection: WorkbenchResourceSelection;
  serverId: string;
  servers: MCPServerExtensionEntry[];
}): { enabled: boolean; selection: WorkbenchResourceSelection } => {
  const currentServerIds = getEnabledMCPServerIds({
    enabled,
    selectedToolNames: selection.enable_mcp,
    servers,
  });
  const currentServerIdSet = new Set(currentServerIds);

  if (currentServerIdSet.has(serverId)) {
    currentServerIdSet.delete(serverId);
  } else {
    currentServerIdSet.add(serverId);
  }

  const nextServers = servers.filter(server =>
    currentServerIdSet.has(server.id),
  );
  const nextEnabled = nextServers.length > 0;
  const nextToolNames =
    nextServers.length === servers.length
      ? []
      : nextServers.flatMap(server => server.toolNames);

  return {
    enabled: nextEnabled,
    selection: {
      ...selection,
      enable_mcp: nextToolNames,
    },
  };
};

const getSkillSourceText = (type: workbenchSkill.SkillType) => {
  switch (type) {
    case workbenchSkill.SkillType.DeerSkill:
      return 'Deer';
    case workbenchSkill.SkillType.PublicSkill:
      return '公共';
    case workbenchSkill.SkillType.CustomSkill:
      return '自定义';
    case workbenchSkill.SkillType.Script:
      return '脚本';
    case workbenchSkill.SkillType.Workflow:
      return '工作流';
    default:
      return '技能';
  }
};

interface ExtensionListProps {
  tab: ExtensionTab;
  mcpError: string;
  mcpLoading: boolean;
  visibleMCPServers: MCPServerExtensionEntry[];
  skillsLoading: boolean;
  skillsError: string;
  visibleSkills: Skill[];
  selectedIds: string[];
  onResourceToggle: (resourceId: string) => void;
}

const SkillExtensionItem = ({
  skill,
  selected,
  onToggle,
}: {
  skill: Skill;
  selected: boolean;
  onToggle: (resourceId: string) => void;
}) => (
  <button
    key={skill.id}
    aria-checked={selected}
    role="checkbox"
    type="button"
    className="chat-workbench-extension-item"
    data-selected={selected}
    onClick={() => onToggle(skill.id)}
  >
    <span className="chat-workbench-extension-icon">
      {skill.type === workbenchSkill.SkillType.Script ||
      skill.type === workbenchSkill.SkillType.Workflow ? (
        skill.type === workbenchSkill.SkillType.Script ? (
          <IconCozCode />
        ) : (
          <IconCozWorkflow />
        )
      ) : (
        <IconCozSkill />
      )}
    </span>
    <span className="chat-workbench-extension-body">
      <span className="chat-workbench-extension-title-row">
        <span className="chat-workbench-extension-name" title={skill.name}>
          {skill.name}
        </span>
        <span className="chat-workbench-extension-tag">
          <span />
          {getSkillSourceText(skill.type)}
        </span>
      </span>
    </span>
    <span
      className="chat-workbench-extension-check"
      data-selected={selected}
      aria-hidden="true"
    >
      {selected ? <IconCozCheckMark /> : null}
    </span>
  </button>
);

const MCPToolExtensionItem = ({
  selected,
  server,
  onToggle,
}: {
  selected: boolean;
  server: MCPServerExtensionEntry;
  onToggle: (resourceId: string) => void;
}) => (
  <button
    key={server.id}
    aria-checked={selected}
    role="checkbox"
    type="button"
    className="chat-workbench-extension-item"
    data-selected={selected}
    onClick={() => onToggle(server.id)}
  >
    <span className="chat-workbench-extension-icon">
      <IconCozPlugin />
    </span>
    <span className="chat-workbench-extension-body">
      <span className="chat-workbench-extension-title-row">
        <span className="chat-workbench-extension-name" title={server.name}>
          {server.name}
        </span>
        <span className="chat-workbench-extension-tag">
          <span />
          MCP
        </span>
      </span>
    </span>
    <span
      className="chat-workbench-extension-check"
      data-selected={selected}
      aria-hidden="true"
    >
      {selected ? <IconCozCheckMark /> : null}
    </span>
  </button>
);

const ExtensionList = ({
  tab,
  mcpError,
  mcpLoading,
  visibleMCPServers,
  skillsLoading,
  skillsError,
  visibleSkills,
  selectedIds,
  onResourceToggle,
}: ExtensionListProps) => (
  <div className="chat-workbench-extension-list">
    {tab === 'skills' && skillsLoading ? (
      <div className="chat-workbench-extension-empty">
        <Spin size="small" />
      </div>
    ) : null}
    {tab === 'skills' && skillsError ? (
      <div className="chat-workbench-extension-error">{skillsError}</div>
    ) : null}
    {tab === 'skills' &&
    !skillsLoading &&
    !skillsError &&
    visibleSkills.length === 0 ? (
      <div className="chat-workbench-extension-empty">暂无可用技能</div>
    ) : null}
    {tab === 'mcp' && mcpLoading ? (
      <div className="chat-workbench-extension-empty">
        <Spin size="small" />
      </div>
    ) : null}
    {tab === 'mcp' && mcpError ? (
      <div className="chat-workbench-extension-error">{mcpError}</div>
    ) : null}
    {tab === 'mcp' &&
    !mcpLoading &&
    !mcpError &&
    visibleMCPServers.length === 0 ? (
      <div className="chat-workbench-extension-empty">暂无可用 MCP 工具</div>
    ) : null}
    {tab === 'skills'
      ? visibleSkills.map(skill => (
          <SkillExtensionItem
            key={skill.id}
            skill={skill}
            selected={selectedIds.includes(skill.id)}
            onToggle={onResourceToggle}
          />
        ))
      : null}
    {tab === 'mcp'
      ? visibleMCPServers.map(server => (
          <MCPToolExtensionItem
            key={server.id}
            server={server}
            selected={selectedIds.includes(server.id)}
            onToggle={onResourceToggle}
          />
        ))
      : null}
  </div>
);

interface ExtensionPanelProps extends ExtensionListProps {
  keyword: string;
  placement: WorkbenchComposerOverlayPlacement;
  searchLabel: string;
  selectedCount: number;
  totalCount: number;
  mcpCount: number;
  skillsCount: number;
  onKeywordChange: (keyword: string) => void;
  onConfigClick: () => void;
  onTabChange: (tab: ExtensionTab) => void;
}

const ExtensionPanel = ({
  keyword,
  onKeywordChange,
  onConfigClick,
  onTabChange,
  placement,
  searchLabel,
  selectedCount,
  totalCount,
  mcpCount,
  skillsCount,
  ...listProps
}: ExtensionPanelProps) => (
  <div className="chat-workbench-extension-panel" data-placement={placement}>
    <Tabs
      className="chat-workbench-extension-tabs"
      type="line"
      size="small"
      activeKey={listProps.tab}
      tabList={[
        { itemKey: 'skills', tab: `技能 ${skillsCount}` },
        { itemKey: 'mcp', tab: `MCP ${mcpCount}` },
      ]}
      tabBarExtraContent={
        <span>
          已启用 {selectedCount}/{totalCount}
        </span>
      }
      onChange={key => onTabChange(key as ExtensionTab)}
    />

    <Input
      size="small"
      className="chat-workbench-extension-search"
      aria-label={searchLabel}
      value={keyword}
      prefix={<IconCozMagnifier />}
      onChange={onKeywordChange}
      placeholder={searchLabel}
    />

    <ExtensionList {...listProps} />

    <button
      type="button"
      className="chat-workbench-extension-footer"
      onClick={onConfigClick}
    >
      <IconCozSetting />
      {listProps.tab === 'skills' ? '技能配置' : '工具配置'}
    </button>
  </div>
);

// eslint-disable-next-line @coze-arch/max-line-per-function -- P0 keeps shared extension overlay state together.
export const ExtensionsPopover = ({
  disabled = false,
  open,
  placement = 'bottom',
  renderMask = true,
  showSelectedCount = true,
  value = defaultSelection,
  settings,
  onChange,
  onSettingsChange,
  onOpenChange,
}: ExtensionsPopoverProps) => {
  const navigate = useNavigate();
  const { space_id } = useParams();
  const [internalOpen, setInternalOpen] = useState(false);
  const [tab, setTab] = useState<ExtensionTab>('skills');
  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillsLoading, setSkillsLoading] = useState(false);
  const [skillsError, setSkillsError] = useState('');
  const [mcpTools, setMCPTools] = useState<MCPToolRegistryEntry[]>([]);
  const [mcpLoading, setMCPLoading] = useState(false);
  const [mcpError, setMCPError] = useState('');
  const [keyword, setKeyword] = useState('');
  const [skillsResultScope, setSkillsResultScope] = useState('');
  const [mcpResultScope, setMCPResultScope] = useState('');
  const rootRef = useRef<HTMLDivElement>(null);
  const skillGenerationRef = useRef(0);
  const mcpGenerationRef = useRef(0);
  const skillScopeRef = useRef('');
  const mcpScopeRef = useRef('');
  const requestedPanelOpen = open ?? internalOpen;
  const panelOpen = !disabled && requestedPanelOpen;
  const resourceScope = panelOpen && space_id ? space_id : '';

  if (skillScopeRef.current !== resourceScope) {
    skillScopeRef.current = resourceScope;
    skillGenerationRef.current += 1;
  }
  if (mcpScopeRef.current !== resourceScope) {
    mcpScopeRef.current = resourceScope;
    mcpGenerationRef.current += 1;
  }

  const scopedSkills = skillsResultScope === resourceScope ? skills : [];
  const scopedMCPTools = mcpResultScope === resourceScope ? mcpTools : [];
  const visibleSkills = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();

    if (!normalizedKeyword) {
      return scopedSkills;
    }

    return scopedSkills.filter(skill =>
      [skill.name, skill.description].some(text =>
        text.toLowerCase().includes(normalizedKeyword),
      ),
    );
  }, [keyword, scopedSkills]);
  const mcpServers = useMemo(
    () => getMCPServerEntries(scopedMCPTools),
    [scopedMCPTools],
  );
  const visibleMCPServers = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();

    if (!normalizedKeyword) {
      return mcpServers;
    }

    return mcpServers.filter(server =>
      [server.id, server.name, ...server.toolNames].some(text =>
        text.toLowerCase().includes(normalizedKeyword),
      ),
    );
  }, [keyword, mcpServers]);
  const availableSkillIds = useMemo(
    () => scopedSkills.filter(skill => skill.enabled).map(skill => skill.id),
    [scopedSkills],
  );
  const availableMCPServerIds = useMemo(
    () => mcpServers.map(server => server.id),
    [mcpServers],
  );
  const skillsEnabled = settings?.skills.enabled ?? true;
  const mcpToolsEnabled = settings?.mcp_tools.enabled ?? true;
  const getEffectiveEnabledIds = (
    explicitIds: string[],
    availableIds: string[],
    enabled: boolean,
  ) => {
    if (!enabled) {
      return [];
    }

    if (explicitIds.length === 0) {
      return availableIds;
    }

    return availableIds.filter(id => explicitIds.includes(id));
  };
  const selectedSkillIds = getEffectiveEnabledIds(
    value.enable_skills,
    availableSkillIds,
    skillsEnabled,
  );
  const selectedMCPServerIds = getEnabledMCPServerIds({
    enabled: mcpToolsEnabled,
    selectedToolNames: value.enable_mcp,
    servers: mcpServers,
  });
  const selectedIds =
    tab === 'skills' ? selectedSkillIds : selectedMCPServerIds;
  const selectedCount = selectedSkillIds.length + selectedMCPServerIds.length;
  const totalCount = availableSkillIds.length + availableMCPServerIds.length;
  const searchLabel = tab === 'skills' ? '搜索技能' : '搜索 MCP';
  const setPanelOpen = (nextOpen: boolean) => {
    if (disabled) {
      return;
    }
    (onOpenChange ?? setInternalOpen)(nextOpen);
  };

  useEffect(() => {
    if (disabled) {
      setInternalOpen(false);
      setKeyword('');
    }
  }, [disabled]);

  useEffect(() => {
    if (!panelOpen || !space_id) {
      return;
    }

    let canceled = false;
    const requestGeneration = ++skillGenerationRef.current;
    const requestScope = resourceScope;
    setSkillsResultScope(requestScope);
    setSkills([]);
    setSkillsLoading(true);
    setSkillsError('');

    void listSkills({ space_id, enabled: true })
      .then(response => {
        if (
          !canceled &&
          skillGenerationRef.current === requestGeneration &&
          skillScopeRef.current === requestScope
        ) {
          setSkills(response.data?.skills ?? []);
        }
      })
      .catch(err => {
        if (
          !canceled &&
          skillGenerationRef.current === requestGeneration &&
          skillScopeRef.current === requestScope
        ) {
          setSkills([]);
          setSkillsError(
            err instanceof Error ? err.message : '加载技能列表失败',
          );
        }
      })
      .finally(() => {
        if (
          !canceled &&
          skillGenerationRef.current === requestGeneration &&
          skillScopeRef.current === requestScope
        ) {
          setSkillsLoading(false);
        }
      });

    return () => {
      canceled = true;
    };
  }, [panelOpen, resourceScope, space_id]);

  useEffect(() => {
    if (!panelOpen || !space_id) {
      return;
    }

    let canceled = false;
    const requestGeneration = ++mcpGenerationRef.current;
    const requestScope = resourceScope;
    setMCPResultScope(requestScope);
    setMCPTools([]);
    setMCPLoading(true);
    setMCPError('');

    void listMCPToolRegistryEntries({ space_id })
      .then(response => {
        if (
          !canceled &&
          mcpGenerationRef.current === requestGeneration &&
          mcpScopeRef.current === requestScope
        ) {
          setMCPTools(response.data?.tools ?? []);
        }
      })
      .catch(err => {
        if (
          !canceled &&
          mcpGenerationRef.current === requestGeneration &&
          mcpScopeRef.current === requestScope
        ) {
          setMCPTools([]);
          setMCPError(
            err instanceof Error ? err.message : '加载 MCP 工具列表失败',
          );
        }
      })
      .finally(() => {
        if (
          !canceled &&
          mcpGenerationRef.current === requestGeneration &&
          mcpScopeRef.current === requestScope
        ) {
          setMCPLoading(false);
        }
      });

    return () => {
      canceled = true;
    };
  }, [panelOpen, resourceScope, space_id]);

  useEffect(() => {
    if (!panelOpen) {
      return;
    }

    const closeAndRestoreFocus = () => {
      setPanelOpen(false);
      queueMicrotask(() => {
        rootRef.current
          ?.querySelector<HTMLButtonElement>('button[aria-label="拓展"]')
          ?.focus();
      });
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        closeAndRestoreFocus();
      }
    };
    const handleMouseDown = (event: MouseEvent) => {
      if (
        event.target instanceof Node &&
        !rootRef.current?.contains(event.target)
      ) {
        closeAndRestoreFocus();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    document.addEventListener('mousedown', handleMouseDown);

    return () => {
      document.removeEventListener('keydown', handleKeyDown);
      document.removeEventListener('mousedown', handleMouseDown);
    };
  }, [panelOpen]);

  const handleResourceToggle = (resourceId: string) => {
    if (disabled) {
      return;
    }

    const { enabled, selection } =
      tab === 'skills'
        ? getNextResourceSelection({
            availableIds: availableSkillIds,
            resourceId,
            selection: value,
            tab,
            tabEnabled: skillsEnabled,
          })
        : getNextMCPServerSelection({
            enabled: mcpToolsEnabled,
            selection: value,
            serverId: resourceId,
            servers: mcpServers,
          });

    onChange?.(selection);
    onSettingsChange?.(current =>
      tab === 'skills'
        ? {
            ...current,
            skills: {
              ...current.skills,
              enabled,
            },
          }
        : {
            ...current,
            mcp_tools: {
              ...current.mcp_tools,
              enabled,
            },
          },
    );
  };

  const handleConfigClick = () => {
    if (!disabled && space_id) {
      navigate(`/space/${space_id}/${tab === 'skills' ? 'skill' : 'tools'}`);
    }
  };

  return (
    <div
      ref={rootRef}
      className="chat-workbench-extensions"
      data-placement={placement}
    >
      <Button
        size="small"
        theme={showSelectedCount ? 'outline' : 'borderless'}
        type={showSelectedCount ? 'primary' : 'tertiary'}
        className={`chat-workbench-extension${
          showSelectedCount ? '' : ' chat-workbench-extension-deerflow'
        }`}
        aria-label="拓展"
        aria-expanded={panelOpen}
        disabled={disabled}
        icon={<IconCozArrowDown />}
        iconPosition="right"
        onClick={() => setPanelOpen(!panelOpen)}
      >
        <span>拓展</span>
        {showSelectedCount ? <span>{selectedCount}</span> : null}
      </Button>

      {panelOpen ? (
        <>
          {renderMask ? (
            <button
              type="button"
              className="chat-workbench-popover-mask"
              aria-label="关闭拓展面板"
              onClick={() => setPanelOpen(false)}
            />
          ) : null}
          <ExtensionPanel
            placement={placement}
            tab={tab}
            keyword={keyword}
            onKeywordChange={nextKeyword => {
              if (!disabled) {
                setKeyword(nextKeyword);
              }
            }}
            onResourceToggle={handleResourceToggle}
            onConfigClick={handleConfigClick}
            onTabChange={nextTab => {
              if (!disabled) {
                setTab(nextTab);
              }
            }}
            searchLabel={searchLabel}
            selectedCount={selectedCount}
            totalCount={totalCount}
            mcpCount={mcpServers.length}
            mcpError={mcpError}
            mcpLoading={mcpLoading}
            visibleMCPServers={visibleMCPServers}
            selectedIds={selectedIds}
            skillsCount={skills.length}
            skillsError={skillsError}
            skillsLoading={skillsLoading}
            visibleSkills={visibleSkills}
          />
        </>
      ) : null}
    </div>
  );
};
