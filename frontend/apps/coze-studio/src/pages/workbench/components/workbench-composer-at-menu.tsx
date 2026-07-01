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

/* eslint-disable max-lines -- P0 @ resource menu keeps resource branches colocated until phase 2 split. */

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type CSSProperties,
} from 'react';

import { type workbenchSkill } from '@coze-studio/api-schema';

import {
  listWorkbenchDatabaseResources,
  listWorkbenchKnowledgeResources,
  listWorkbenchWorkflowResources,
  type WorkbenchReferenceResource,
} from '../service';
import { listSkills } from '../../skill/service';
import type { WorkbenchResourceSelection } from './types';

export type WorkbenchComposerOverlayPlacement = 'top' | 'bottom';

const AT_RESOURCES = [
  { name: '技能', aliases: ['skill', 'skills'], icon: '✦' },
  { name: '知识库', aliases: ['knowledge', 'kb'], icon: 'K' },
  { name: '数据库', aliases: ['database', 'db'], icon: 'D' },
  { name: '工作流', aliases: ['workflow', 'flow'], icon: 'W' },
  { name: '插件', aliases: ['plugin'], icon: 'P' },
  { name: '代码仓库', aliases: ['repo', 'repository'], icon: '</>' },
] as const;

type Skill = workbenchSkill.Skill;
export type WorkbenchAtResourceType = (typeof AT_RESOURCES)[number]['name'];

export type WorkbenchAtDraft =
  | {
      stage: 'resource-type';
      query: string;
    }
  | {
      stage: 'resource-search';
      query: string;
      resourceType: WorkbenchAtResourceType;
    };

export interface WorkbenchAtReference {
  id: string;
  name: string;
  resourceType: WorkbenchAtResourceType;
}

export type WorkbenchAtSegment =
  | {
      id: string;
      text: string;
      type: 'text';
    }
  | {
      id: string;
      reference: WorkbenchAtReference;
      type: 'reference';
    };

export const getWorkbenchAtResourceIcon = (
  resourceType: WorkbenchAtResourceType,
) => AT_RESOURCES.find(item => item.name === resourceType)?.icon ?? 'R';

export const addSkillSelection = (
  selection: WorkbenchResourceSelection,
  skillId: string,
): WorkbenchResourceSelection => {
  if (selection.enable_skills.includes(skillId)) {
    return selection;
  }

  const enableSkills = [...selection.enable_skills, skillId];

  return {
    ...selection,
    enable_skills: enableSkills,
    explicit_enable_skills: enableSkills,
  };
};

export const removeSkillSelection = (
  selection: WorkbenchResourceSelection,
  skillId: string,
): WorkbenchResourceSelection => {
  const enableSkills = selection.enable_skills.filter(id => id !== skillId);

  return {
    ...selection,
    enable_skills: enableSkills,
    explicit_enable_skills: enableSkills,
  };
};

export const addKnowledgeSelection = (
  selection: WorkbenchResourceSelection,
  knowledgeId: string,
): WorkbenchResourceSelection => {
  if (selection.enable_kbs.includes(knowledgeId)) {
    return selection;
  }

  return {
    ...selection,
    enable_kbs: [...selection.enable_kbs, knowledgeId],
  };
};

export const removeKnowledgeSelection = (
  selection: WorkbenchResourceSelection,
  knowledgeId: string,
): WorkbenchResourceSelection => ({
  ...selection,
  enable_kbs: selection.enable_kbs.filter(id => id !== knowledgeId),
});

export const addDatabaseSelection = (
  selection: WorkbenchResourceSelection,
  databaseId: string,
): WorkbenchResourceSelection => {
  if (selection.enable_databases.includes(databaseId)) {
    return selection;
  }

  return {
    ...selection,
    enable_databases: [...selection.enable_databases, databaseId],
  };
};

export const removeDatabaseSelection = (
  selection: WorkbenchResourceSelection,
  databaseId: string,
): WorkbenchResourceSelection => ({
  ...selection,
  enable_databases: selection.enable_databases.filter(id => id !== databaseId),
});

const matchesQuery = (
  values: Array<string | undefined>,
  query: string,
): boolean => {
  const normalizedQuery = query.trim().toLowerCase();

  if (!normalizedQuery) {
    return true;
  }

  return values.some(value => value?.toLowerCase().includes(normalizedQuery));
};

// eslint-disable-next-line @coze-arch/max-line-per-function -- P0 keeps @ resource and skill branches together.
export const AtMenu = ({
  anchorPosition,
  draft,
  placement,
  spaceId,
  value,
  onClose,
  onBackToResourceTypes,
  onReferenceSelect,
  onResourceTypeSelect,
}: {
  anchorPosition?: CSSProperties;
  draft: WorkbenchAtDraft;
  placement: WorkbenchComposerOverlayPlacement;
  spaceId?: string;
  value: WorkbenchResourceSelection;
  onClose: () => void;
  onBackToResourceTypes: () => void;
  onReferenceSelect: (reference: WorkbenchAtReference) => void;
  onResourceTypeSelect: (resourceType: WorkbenchAtResourceType) => void;
}) => {
  const [skills, setSkills] = useState<Skill[]>([]);
  const [skillsLoading, setSkillsLoading] = useState(false);
  const [skillsError, setSkillsError] = useState('');
  const [resources, setResources] = useState<WorkbenchReferenceResource[]>([]);
  const [resourcesLoading, setResourcesLoading] = useState(false);
  const [resourcesError, setResourcesError] = useState('');
  const [activeIndex, setActiveIndex] = useState(0);
  const view = draft.stage === 'resource-type' ? 'resources' : 'resources-data';
  const enabledSkills = useMemo(
    () => skills.filter(skill => skill.enabled),
    [skills],
  );
  const filteredResources = useMemo(
    () =>
      AT_RESOURCES.filter(resource =>
        matchesQuery([resource.name, ...resource.aliases], draft.query),
      ),
    [draft.query],
  );
  const filteredSkills = useMemo(
    () =>
      enabledSkills.filter(skill =>
        matchesQuery([skill.name, skill.description], draft.query),
      ),
    [draft.query, enabledSkills],
  );
  const filteredReferenceResources = useMemo(
    () =>
      resources.filter(resource =>
        matchesQuery([resource.name, resource.description], draft.query),
      ),
    [draft.query, resources],
  );
  const isSkillSearch =
    draft.stage === 'resource-search' && draft.resourceType === '技能';
  const isReservedSearch =
    draft.stage === 'resource-search' &&
    (draft.resourceType === '插件' || draft.resourceType === '代码仓库');
  const draftResourceType =
    draft.stage === 'resource-search' ? draft.resourceType : '';
  const menuStyle: CSSProperties | undefined =
    anchorPosition && Object.keys(anchorPosition).length > 0
      ? anchorPosition
      : undefined;

  const searchableResourceType =
    draft.stage === 'resource-search' &&
    draft.resourceType !== '技能' &&
    !isReservedSearch
      ? draft.resourceType
      : undefined;
  const selectableItemCount =
    view === 'resources'
      ? filteredResources.length
      : isSkillSearch && !skillsLoading && !skillsError
        ? filteredSkills.length
        : draft.stage === 'resource-search' &&
            !isReservedSearch &&
            !resourcesLoading &&
            !resourcesError
          ? filteredReferenceResources.length
          : 0;

  useEffect(() => {
    if (!isSkillSearch || !spaceId) {
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
  }, [isSkillSearch, spaceId]);

  useEffect(() => {
    if (!searchableResourceType || !spaceId) {
      setResources([]);

      return;
    }

    const resourceLoaders: Partial<
      Record<
        WorkbenchAtResourceType,
        (spaceId?: string) => Promise<WorkbenchReferenceResource[]>
      >
    > = {
      工作流: listWorkbenchWorkflowResources,
      数据库: listWorkbenchDatabaseResources,
      知识库: listWorkbenchKnowledgeResources,
    };
    const loadResources = resourceLoaders[searchableResourceType];

    if (!loadResources) {
      setResources([]);

      return;
    }

    let canceled = false;
    setResourcesLoading(true);
    setResourcesError('');

    void loadResources(spaceId)
      .then(nextResources => {
        if (!canceled) {
          setResources(nextResources);
        }
      })
      .catch(err => {
        if (!canceled) {
          setResources([]);
          setResourcesError(
            err instanceof Error
              ? err.message
              : `加载${searchableResourceType}失败`,
          );
        }
      })
      .finally(() => {
        if (!canceled) {
          setResourcesLoading(false);
        }
      });

    return () => {
      canceled = true;
    };
  }, [searchableResourceType, spaceId]);

  useEffect(() => {
    setActiveIndex(0);
  }, [draft.query, draft.stage, draftResourceType, selectableItemCount]);

  const handleResourceClick = useCallback(
    (resource: WorkbenchAtResourceType) => {
      onResourceTypeSelect(resource);
    },
    [onResourceTypeSelect],
  );

  const handleSkillClick = useCallback(
    (skill: Skill) => {
      onReferenceSelect({
        id: skill.id,
        name: skill.name,
        resourceType: '技能',
      });
      onClose();
    },
    [onClose, onReferenceSelect],
  );

  const handleReferenceClick = useCallback(
    (resource: WorkbenchReferenceResource) => {
      if (draft.stage !== 'resource-search') {
        return;
      }

      onReferenceSelect({
        id: resource.id,
        name: resource.name,
        resourceType: draft.resourceType,
      });
      onClose();
    },
    [draft, onClose, onReferenceSelect],
  );

  const handleActiveItemSelect = useCallback(() => {
    if (view === 'resources') {
      const resource = filteredResources[activeIndex];

      if (resource) {
        handleResourceClick(resource.name);
      }

      return;
    }

    if (isSkillSearch) {
      const skill = filteredSkills[activeIndex];

      if (skill) {
        handleSkillClick(skill);
      }

      return;
    }

    if (draft.stage === 'resource-search' && !isReservedSearch) {
      const resource = filteredReferenceResources[activeIndex];

      if (resource) {
        handleReferenceClick(resource);
      }
    }
  }, [
    activeIndex,
    draft.stage,
    filteredReferenceResources,
    filteredResources,
    filteredSkills,
    handleReferenceClick,
    handleResourceClick,
    handleSkillClick,
    isReservedSearch,
    isSkillSearch,
    view,
  ]);

  useEffect(() => {
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented) {
        return;
      }

      if (event.key === 'Escape' || event.key === 'Esc') {
        event.preventDefault();
        onClose();

        return;
      }

      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        if (selectableItemCount < 1) {
          return;
        }

        event.preventDefault();
        setActiveIndex(index => {
          const step = event.key === 'ArrowDown' ? 1 : -1;

          return (index + step + selectableItemCount) % selectableItemCount;
        });

        return;
      }

      if (event.key === 'Enter') {
        if (selectableItemCount < 1) {
          return;
        }

        event.preventDefault();
        handleActiveItemSelect();
      }
    };

    document.addEventListener('keydown', handleKeyDown);

    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [handleActiveItemSelect, onClose, selectableItemCount]);

  return (
    <div
      className="chat-workbench-at-menu"
      data-placement={placement}
      role="menu"
      aria-label={isSkillSearch ? '@ 选择技能' : '@ 选择资源类型'}
      style={menuStyle}
    >
      <div className="chat-workbench-at-menu-title">
        {draft.stage === 'resource-search' ? (
          <button
            type="button"
            className="chat-workbench-at-menu-back"
            onClick={onBackToResourceTypes}
          >
            ‹
          </button>
        ) : (
          <span>@</span>
        )}
        {isSkillSearch
          ? '选择技能'
          : draft.stage === 'resource-search'
            ? `选择${draft.resourceType}`
            : '选择资源类型'}
      </div>

      {view === 'resources' ? (
        <div className="chat-workbench-at-menu-list">
          {filteredResources.length ? null : (
            <div className="chat-workbench-at-menu-state">暂无匹配资源类型</div>
          )}
          {filteredResources.map((item, index) => (
            <button
              key={item.name}
              type="button"
              role="menuitem"
              data-active={index === activeIndex}
              onClick={() => handleResourceClick(item.name)}
              onMouseEnter={() => setActiveIndex(index)}
            >
              <span className="chat-workbench-at-menu-icon">{item.icon}</span>
              <span>{item.name}</span>
              <span aria-hidden="true">›</span>
            </button>
          ))}
        </div>
      ) : isSkillSearch ? (
        <div className="chat-workbench-at-menu-list">
          {skillsLoading ? (
            <div className="chat-workbench-at-menu-state">加载技能中...</div>
          ) : null}
          {skillsError ? (
            <div className="chat-workbench-at-menu-state">{skillsError}</div>
          ) : null}
          {!skillsLoading && !skillsError && filteredSkills.length === 0 ? (
            <div className="chat-workbench-at-menu-state">暂无匹配技能</div>
          ) : null}
          {filteredSkills.map((skill, index) => (
            <button
              key={skill.id}
              type="button"
              role="menuitemcheckbox"
              aria-checked={value.enable_skills.includes(skill.id)}
              data-active={index === activeIndex}
              onClick={() => handleSkillClick(skill)}
              onMouseEnter={() => setActiveIndex(index)}
            >
              <span className="chat-workbench-at-menu-icon">✦</span>
              <span className="chat-workbench-at-menu-name">{skill.name}</span>
              <span aria-hidden="true">
                {value.enable_skills.includes(skill.id) ? '✓' : ''}
              </span>
            </button>
          ))}
        </div>
      ) : isReservedSearch ? (
        <div className="chat-workbench-at-menu-list">
          <div className="chat-workbench-at-menu-state">
            暂无可用{draft.resourceType}
          </div>
        </div>
      ) : (
        <div className="chat-workbench-at-menu-list">
          {resourcesLoading ? (
            <div className="chat-workbench-at-menu-state">
              加载{draft.resourceType}中...
            </div>
          ) : null}
          {resourcesError ? (
            <div className="chat-workbench-at-menu-state">{resourcesError}</div>
          ) : null}
          {!resourcesLoading &&
          !resourcesError &&
          filteredReferenceResources.length === 0 ? (
            <div className="chat-workbench-at-menu-state">
              暂无匹配{draft.resourceType}
            </div>
          ) : null}
          {filteredReferenceResources.map((resource, index) => (
            <button
              key={resource.id}
              type="button"
              role="menuitem"
              data-active={index === activeIndex}
              onClick={() => handleReferenceClick(resource)}
              onMouseEnter={() => setActiveIndex(index)}
            >
              <span className="chat-workbench-at-menu-icon">
                {getWorkbenchAtResourceIcon(draft.resourceType)}
              </span>
              <span className="chat-workbench-at-menu-name">
                {resource.name}
              </span>
              <span aria-hidden="true" />
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
