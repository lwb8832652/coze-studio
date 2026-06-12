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
import { useNavigate, useParams } from 'react-router-dom';

import { IconCozArrowDown } from '@coze-arch/coze-design/icons';

import {
  WORKBENCH_PRESET_SKILL_IDS,
  createDefaultWorkbenchResourceSelection,
  type WorkbenchResourceSelection,
} from './components/types';

interface ExtensionsPopoverProps {
  value?: WorkbenchResourceSelection;
  onChange?: (value: WorkbenchResourceSelection) => void;
}

const defaultSelection = createDefaultWorkbenchResourceSelection();

const getResourceIds = (
  selection: WorkbenchResourceSelection,
  tab: 'skills' | 'mcp',
) => (tab === 'skills' ? selection.enable_skills : selection.enable_mcp);

const getNextResourceSelection = (
  selection: WorkbenchResourceSelection,
  tab: 'skills' | 'mcp',
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

export const ExtensionsPopover = ({
  value = defaultSelection,
  onChange,
}: ExtensionsPopoverProps) => {
  const navigate = useNavigate();
  const { space_id } = useParams();
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<'skills' | 'mcp'>('skills');
  const resources = tab === 'skills' ? WORKBENCH_PRESET_SKILL_IDS : [];
  const selectedIds = getResourceIds(value, tab);
  const selectedCount = [
    ...value.enable_skills,
    ...value.enable_mcp,
    ...value.enable_kbs,
    ...value.enable_databases,
  ].length;
  const searchLabel = tab === 'skills' ? '搜索技能' : '搜索 MCP';

  const handleResourceToggle = (resourceId: string) =>
    onChange?.(getNextResourceSelection(value, tab, resourceId));

  return (
    <div className="chat-workbench-extensions">
      <button
        type="button"
        className="chat-workbench-extension"
        aria-label="拓展 47"
        aria-expanded={open}
        onClick={() => setOpen(current => !current)}
      >
        <span>拓展</span>
        <span>47</span>
        <IconCozArrowDown />
      </button>

      {open ? (
        <>
          <button
            type="button"
            className="chat-workbench-popover-mask"
            aria-label="关闭拓展面板"
            onClick={() => setOpen(false)}
          />
          <div className="chat-workbench-extension-panel">
            <div className="chat-workbench-extension-tabs">
              <button
                type="button"
                data-active={tab === 'skills'}
                onClick={() => setTab('skills')}
              >
                技能 <span>27</span>
              </button>
              <button
                type="button"
                data-active={tab === 'mcp'}
                onClick={() => setTab('mcp')}
              >
                MCP <span>20</span>
              </button>
              <span>已选择: {selectedCount}/100</span>
            </div>

            <label className="chat-workbench-extension-search">
              <span aria-hidden="true">⌕</span>
              <input aria-label={searchLabel} placeholder={searchLabel} />
            </label>

            <div className="chat-workbench-extension-list">
              {resources.map(resourceId => (
                <button
                  key={resourceId}
                  type="button"
                  onClick={() => handleResourceToggle(resourceId)}
                >
                  <span className="chat-workbench-extension-icon">📕</span>
                  <span className="chat-workbench-extension-name">
                    {resourceId}
                  </span>
                  <span className="chat-workbench-extension-tag">
                    <span />
                    官方
                  </span>
                  <span
                    className="chat-workbench-extension-check"
                    data-selected={selectedIds.includes(resourceId)}
                    aria-hidden="true"
                  >
                    {selectedIds.includes(resourceId) ? '✓' : ''}
                  </span>
                </button>
              ))}
            </div>

            <button
              type="button"
              className="chat-workbench-extension-footer"
              onClick={() => {
                if (space_id) {
                  navigate(`/space/${space_id}/skill`);
                }
              }}
            >
              ⚙ 技能配置
            </button>
          </div>
        </>
      ) : null}
    </div>
  );
};
