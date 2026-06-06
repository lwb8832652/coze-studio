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

const EXTENSION_SKILLS = [
  'meego-guidelines',
  'aeolus-platform-analysis',
  'coral-hive-metric-explorer',
  'deepwiki',
  'code-review',
  'aime-toolkit',
  'lark-wiki',
  'lark-shared',
  'lark-drive',
];

export const ExtensionsPopover = () => {
  const navigate = useNavigate();
  const { space_id } = useParams();
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<'skills' | 'mcp'>('skills');
  const [selected, setSelected] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(EXTENSION_SKILLS.map(skill => [skill, true])),
  );
  const selectedCount = Object.values(selected).filter(Boolean).length;

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
              <input
                aria-label={tab === 'skills' ? '搜索技能' : '搜索 MCP'}
                placeholder={tab === 'skills' ? '搜索技能' : '搜索 MCP'}
              />
            </label>

            <div className="chat-workbench-extension-list">
              {EXTENSION_SKILLS.map(skill => (
                <button
                  key={skill}
                  type="button"
                  onClick={() =>
                    setSelected(current => ({
                      ...current,
                      [skill]: !current[skill],
                    }))
                  }
                >
                  <span className="chat-workbench-extension-icon">📕</span>
                  <span className="chat-workbench-extension-name">{skill}</span>
                  <span className="chat-workbench-extension-tag">
                    <span />
                    官方
                  </span>
                  <span
                    className="chat-workbench-extension-check"
                    data-selected={selected[skill]}
                    aria-hidden="true"
                  >
                    {selected[skill] ? '✓' : ''}
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
