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

import { IconCozLink, IconCozSendFill } from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';

import { ExtensionsPopover } from '../extensions-popover';

import {
  createDefaultWorkbenchResourceSelection,
  WORKBENCH_MODE_PROMPTS,
  WORKBENCH_MODE_SYMBOLS,
  WORKBENCH_MODES,
  type WorkbenchComposerSubmitPayload,
  type WorkbenchComposerVariant,
  type WorkbenchMode,
} from './types';

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

export interface WorkbenchComposerProps {
  value: string;
  mode: WorkbenchMode;
  loading: boolean;
  error?: string;
  variant?: WorkbenchComposerVariant;
  taskId?: string;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onSubmit: (payload: WorkbenchComposerSubmitPayload) => void | Promise<void>;
}

const AtMenu = ({ onClose }: { onClose: () => void }) => (
  <div
    className="chat-workbench-at-menu"
    role="menu"
    aria-label="@ 选择资源类型"
  >
    <div className="chat-workbench-at-menu-title">
      <span>@</span>
      选择资源类型
    </div>
    <div className="chat-workbench-at-menu-list">
      {AT_RESOURCES.map((item, index) => (
        <button key={item} type="button" role="menuitem" onClick={onClose}>
          <span className="chat-workbench-at-menu-icon">
            {String(index + 1).padStart(2, '0')}
          </span>
          <span>{item}</span>
          <span aria-hidden="true">›</span>
        </button>
      ))}
    </div>
    <div className="chat-workbench-at-menu-footer">
      <span>↑↓ 移动光标</span>
      <span>↵ 选择条目</span>
      <span>Esc 退出</span>
    </div>
  </div>
);

export const WorkbenchComposer = ({
  value,
  mode,
  loading,
  error,
  variant = 'home',
  taskId,
  onValueChange,
  onModeChange,
  onSubmit,
}: WorkbenchComposerProps) => {
  const [atMenuOpen, setAtMenuOpen] = useState(false);
  const [resourceSelection, setResourceSelection] = useState(
    createDefaultWorkbenchResourceSelection,
  );
  const canSend = Boolean(value.trim()) && !loading;

  const handleValueChange = (nextValue: string) => {
    onValueChange(nextValue);
    setAtMenuOpen(nextValue.endsWith('@'));
  };

  const handleSubmit = () => {
    const message = value.trim();

    if (!message || loading) {
      return;
    }

    onSubmit({
      message,
      mode,
      taskId,
      enable_skills: [...resourceSelection.enable_skills],
      enable_mcp: [...resourceSelection.enable_mcp],
      enable_kbs: [...resourceSelection.enable_kbs],
      enable_databases: [...resourceSelection.enable_databases],
    });
  };

  return (
    <>
      <section
        className="chat-workbench-composer"
        data-variant={variant}
        aria-label="任务输入"
      >
        {atMenuOpen ? <AtMenu onClose={() => setAtMenuOpen(false)} /> : null}
        <div className="chat-workbench-composer-body">
          <TextArea
            aria-label="任务描述"
            autosize={false}
            rows={3}
            value={value}
            onChange={handleValueChange}
            placeholder=""
            className="chat-workbench-input"
          />
          {!value ? (
            <div
              className="chat-workbench-composer-prompt"
              aria-label="当前模式提示"
            >
              <span aria-hidden="true">{WORKBENCH_MODE_SYMBOLS[mode]}</span>
              <span>{WORKBENCH_MODE_PROMPTS[mode]}</span>
            </div>
          ) : null}
        </div>

        <div className="chat-workbench-toolbar">
          <div className="chat-workbench-toolbar-left">
            <div className="chat-workbench-mode" aria-label="模式选择">
              {WORKBENCH_MODES.map(item => (
                <button
                  key={item}
                  type="button"
                  className="chat-workbench-mode-button"
                  data-active={mode === item}
                  aria-pressed={mode === item}
                  onClick={() => onModeChange(item)}
                >
                  {item}
                </button>
              ))}
            </div>

            <ExtensionsPopover
              value={resourceSelection}
              onChange={setResourceSelection}
            />
          </div>

          <div className="chat-workbench-toolbar-actions">
            <button
              type="button"
              aria-label="添加上下文"
              aria-expanded={atMenuOpen}
              onClick={() => setAtMenuOpen(open => !open)}
            >
              @
            </button>
            <button type="button" aria-label="添加附件">
              <IconCozLink />
            </button>
          </div>
          <Button
            aria-label="发送任务"
            color="primary"
            disabled={!canSend}
            icon={<IconCozSendFill />}
            loading={loading}
            onClick={handleSubmit}
            className="chat-workbench-send"
          >
            <span className="chat-workbench-send-label">
              {loading ? '发送中' : '发送'}
            </span>
          </Button>
        </div>
      </section>

      {error ? (
        <div className="chat-workbench-error" role="alert">
          {error}
        </div>
      ) : null}
    </>
  );
};
