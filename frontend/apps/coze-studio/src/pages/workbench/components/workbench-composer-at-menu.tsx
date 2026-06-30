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

import { useEffect, useMemo, useState } from 'react';

import { type workbenchSkill } from '@coze-studio/api-schema';

import { listSkills } from '../../skill/service';
import type { WorkbenchResourceSelection } from './types';

export type WorkbenchComposerOverlayPlacement = 'top' | 'bottom';

const AT_RESOURCE_NUMBER_WIDTH = 2;
const AT_RESOURCES = [
  '技能',
  '代码仓库',
  '仓库分支',
  '代码文件夹',
  '代码文件',
  '用户',
  '风神数据',
  'Meego 工作项',
  '空间文档库',
] as const;

type Skill = workbenchSkill.Skill;
type AtMenuView = 'resources' | 'skills';

const toggleSkillSelection = (
  selection: WorkbenchResourceSelection,
  skillId: string,
): WorkbenchResourceSelection => {
  const enableSkills = selection.enable_skills.includes(skillId)
    ? selection.enable_skills.filter(id => id !== skillId)
    : [...selection.enable_skills, skillId];

  return {
    ...selection,
    enable_skills: enableSkills,
    explicit_enable_skills: enableSkills,
  };
};

// eslint-disable-next-line @coze-arch/max-line-per-function -- P0 keeps @ resource and skill branches together.
export const AtMenu = ({
  placement,
  spaceId,
  value,
  onClose,
  onChange,
}: {
  placement: WorkbenchComposerOverlayPlacement;
  spaceId?: string;
  value: WorkbenchResourceSelection;
  onClose: () => void;
  onChange: (selection: WorkbenchResourceSelection) => void;
}) => {
  const [view, setView] = useState<AtMenuView>('resources');
  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillsLoading, setSkillsLoading] = useState(false);
  const [skillsError, setSkillsError] = useState('');
  const enabledSkills = useMemo(
    () => skills.filter(skill => skill.enabled),
    [skills],
  );

  useEffect(() => {
    if (view !== 'skills' || !spaceId) {
      return;
    }

    let canceled = false;
    setSkillsLoading(true);
    setSkillsError('');

    void listSkills({ space_id: spaceId, enabled: true })
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
  }, [spaceId, view]);

  const handleResourceClick = (resource: (typeof AT_RESOURCES)[number]) => {
    if (resource === '技能') {
      setView('skills');

      return;
    }

    onClose();
  };

  const handleSkillClick = (skill: Skill) => {
    onChange(toggleSkillSelection(value, skill.id));
    onClose();
  };

  return (
    <div
      className="chat-workbench-at-menu"
      data-placement={placement}
      role="menu"
      aria-label={view === 'skills' ? '@ 选择技能' : '@ 选择资源类型'}
    >
      <div className="chat-workbench-at-menu-title">
        {view === 'skills' ? (
          <button
            type="button"
            className="chat-workbench-at-menu-back"
            onClick={() => setView('resources')}
          >
            ‹
          </button>
        ) : (
          <span>@</span>
        )}
        {view === 'skills' ? '选择技能' : '选择资源类型'}
      </div>

      {view === 'resources' ? (
        <div className="chat-workbench-at-menu-list">
          {AT_RESOURCES.map((item, index) => (
            <button
              key={item}
              type="button"
              role="menuitem"
              onClick={() => handleResourceClick(item)}
            >
              <span className="chat-workbench-at-menu-icon">
                {String(index + 1).padStart(AT_RESOURCE_NUMBER_WIDTH, '0')}
              </span>
              <span>{item}</span>
              <span aria-hidden="true">›</span>
            </button>
          ))}
        </div>
      ) : (
        <div className="chat-workbench-at-menu-list">
          {skillsLoading ? (
            <div className="chat-workbench-at-menu-state">加载技能中...</div>
          ) : null}
          {skillsError ? (
            <div className="chat-workbench-at-menu-state">{skillsError}</div>
          ) : null}
          {!skillsLoading && !skillsError && enabledSkills.length === 0 ? (
            <div className="chat-workbench-at-menu-state">暂无可用技能</div>
          ) : null}
          {enabledSkills.map(skill => (
            <button
              key={skill.id}
              type="button"
              role="menuitemcheckbox"
              aria-checked={value.enable_skills.includes(skill.id)}
              onClick={() => handleSkillClick(skill)}
            >
              <span className="chat-workbench-at-menu-icon">✦</span>
              <span>
                <span className="chat-workbench-at-menu-name">
                  {skill.name}
                </span>
                {skill.description ? (
                  <span className="chat-workbench-at-menu-desc">
                    {skill.description}
                  </span>
                ) : null}
              </span>
              <span aria-hidden="true">
                {value.enable_skills.includes(skill.id) ? '✓' : ''}
              </span>
            </button>
          ))}
        </div>
      )}

      <div className="chat-workbench-at-menu-footer">
        <span>↑↓ 移动光标</span>
        <span>↵ 选择条目</span>
        <span>Esc 退出</span>
      </div>
    </div>
  );
};
