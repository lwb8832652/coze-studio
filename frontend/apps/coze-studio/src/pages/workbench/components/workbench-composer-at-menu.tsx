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

export const AtMenu = ({
  placement,
  onClose,
}: {
  placement: WorkbenchComposerOverlayPlacement;
  onClose: () => void;
}) => (
  <div
    className="chat-workbench-at-menu"
    data-placement={placement}
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
            {String(index + 1).padStart(AT_RESOURCE_NUMBER_WIDTH, '0')}
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
