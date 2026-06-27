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

import { useNavigate, useParams } from 'react-router-dom';
import { useEffect, useMemo, useState } from 'react';

import { workbenchSkill } from '@coze-studio/api-schema';
import {
  IconCozArrowDown,
  IconCozPlugin,
  IconCozSetting,
} from '@coze-arch/coze-design/icons';
import { Button, Input, Spin, Tabs } from '@coze-arch/coze-design';

import { listSkills } from '../skill/service';
import {
  createDefaultWorkbenchResourceSelection,
  type WorkbenchResourceSelection,
} from './components/types';

type Skill = workbenchSkill.Skill;
type ExtensionTab = 'skills' | 'mcp';

interface ExtensionsPopoverProps {
  value?: WorkbenchResourceSelection;
  onChange?: (value: WorkbenchResourceSelection) => void;
}

const defaultSelection = createDefaultWorkbenchResourceSelection();

const getResourceIds = (
  selection: WorkbenchResourceSelection,
  tab: ExtensionTab,
) => (tab === 'skills' ? selection.enable_skills : selection.enable_mcp);

const getNextResourceSelection = (
  selection: WorkbenchResourceSelection,
  tab: ExtensionTab,
  resourceId: string,
): WorkbenchResourceSelection => {
  const key = tab === 'skills' ? 'enable_skills' : 'enable_mcp';
  const currentIds = selection[key];
  const nextIds = currentIds.includes(resourceId)
    ? currentIds.filter(id => id !== resourceId)
    : [...currentIds, resourceId];

  return {
    ...selection,
    [key]: nextIds,
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
  <Button
    key={skill.id}
    block
    theme="borderless"
    type="tertiary"
    className="chat-workbench-extension-item"
    onClick={() => onToggle(skill.id)}
  >
    <span className="chat-workbench-extension-icon">
      {skill.type === workbenchSkill.SkillType.Script ||
      skill.type === workbenchSkill.SkillType.Workflow ? (
        <IconCozPlugin />
      ) : (
        <IconCozSetting />
      )}
    </span>
    <span className="chat-workbench-extension-name">{skill.name}</span>
    <span className="chat-workbench-extension-tag">
      <span />
      {getSkillSourceText(skill.type)}
    </span>
    <span
      className="chat-workbench-extension-check"
      data-selected={selected}
      aria-hidden="true"
    >
      {selected ? '✓' : ''}
    </span>
  </Button>
);

const ExtensionList = ({
  tab,
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
    {tab === 'mcp' ? (
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
  </div>
);

interface ExtensionPanelProps extends ExtensionListProps {
  keyword: string;
  searchLabel: string;
  selectedCount: number;
  skillsCount: number;
  onKeywordChange: (keyword: string) => void;
  onSkillConfigClick: () => void;
  onTabChange: (tab: ExtensionTab) => void;
}

const ExtensionPanel = ({
  keyword,
  onKeywordChange,
  onSkillConfigClick,
  onTabChange,
  searchLabel,
  selectedCount,
  skillsCount,
  ...listProps
}: ExtensionPanelProps) => (
  <div className="chat-workbench-extension-panel">
    <Tabs
      className="chat-workbench-extension-tabs"
      type="line"
      size="small"
      activeKey={listProps.tab}
      tabList={[
        { itemKey: 'skills', tab: `技能 ${skillsCount}` },
        { itemKey: 'mcp', tab: 'MCP 0' },
      ]}
      tabBarExtraContent={<span>已选择: {selectedCount}/100</span>}
      onChange={key => onTabChange(key as ExtensionTab)}
    />

    <Input
      size="small"
      className="chat-workbench-extension-search"
      aria-label={searchLabel}
      value={keyword}
      prefix={<span aria-hidden="true">⌕</span>}
      onChange={onKeywordChange}
      placeholder={searchLabel}
    />

    <ExtensionList {...listProps} />

    <Button
      block
      theme="borderless"
      type="tertiary"
      className="chat-workbench-extension-footer"
      icon={<IconCozSetting />}
      onClick={onSkillConfigClick}
    >
      技能配置
    </Button>
  </div>
);

export const ExtensionsPopover = ({
  value = defaultSelection,
  onChange,
}: ExtensionsPopoverProps) => {
  const navigate = useNavigate();
  const { space_id } = useParams();
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<ExtensionTab>('skills');
  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillsLoading, setSkillsLoading] = useState(false);
  const [skillsError, setSkillsError] = useState('');
  const [keyword, setKeyword] = useState('');
  const visibleSkills = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();

    if (!normalizedKeyword) {
      return skills;
    }

    return skills.filter(skill =>
      [skill.name, skill.description].some(text =>
        text.toLowerCase().includes(normalizedKeyword),
      ),
    );
  }, [keyword, skills]);
  const selectedIds = getResourceIds(value, tab);
  const selectedCount = [
    ...value.enable_skills,
    ...value.enable_mcp,
    ...value.enable_kbs,
    ...value.enable_databases,
  ].length;
  const searchLabel = tab === 'skills' ? '搜索技能' : '搜索 MCP';

  useEffect(() => {
    if (!open || tab !== 'skills' || !space_id) {
      return;
    }

    let canceled = false;
    setSkillsLoading(true);
    setSkillsError('');

    void listSkills({ space_id, enabled: true })
      .then(response => {
        if (!canceled) {
          setSkills(response.data?.skills ?? []);
        }
      })
      .catch(err => {
        if (!canceled) {
          setSkills([]);
          setSkillsError(
            err instanceof Error ? err.message : '加载技能列表失败',
          );
        }
      })
      .finally(() => {
        if (!canceled) {
          setSkillsLoading(false);
        }
      });

    return () => {
      canceled = true;
    };
  }, [open, space_id, tab]);

  const handleResourceToggle = (resourceId: string) =>
    onChange?.(getNextResourceSelection(value, tab, resourceId));

  const handleSkillConfigClick = () => {
    if (space_id) {
      navigate(`/space/${space_id}/skill`);
    }
  };

  return (
    <div className="chat-workbench-extensions">
      <Button
        size="small"
        theme="outline"
        className="chat-workbench-extension"
        aria-label="拓展"
        aria-expanded={open}
        icon={<IconCozArrowDown />}
        iconPosition="right"
        onClick={() => setOpen(current => !current)}
      >
        <span>拓展</span>
        <span>{selectedCount}</span>
      </Button>

      {open ? (
        <>
          <button
            type="button"
            className="chat-workbench-popover-mask"
            aria-label="关闭拓展面板"
            onClick={() => setOpen(false)}
          />
          <ExtensionPanel
            tab={tab}
            keyword={keyword}
            onKeywordChange={setKeyword}
            onResourceToggle={handleResourceToggle}
            onSkillConfigClick={handleSkillConfigClick}
            onTabChange={setTab}
            searchLabel={searchLabel}
            selectedCount={selectedCount}
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
