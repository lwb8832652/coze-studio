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

import { IconCozSendFill } from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';

import './index.less';

const TEMPLATE_TAGS = [
  '工作总结',
  '数据分析',
  '代码分析',
  '异常排查',
  '飞书文档撰写',
];

const MODES = ['Auto', 'Ask', 'Agent'] as const;

type WorkbenchMode = (typeof MODES)[number];

const WorkbenchPage = () => {
  const [value, setValue] = useState('');
  const [mode, setMode] = useState<WorkbenchMode>('Auto');
  const canSend = Boolean(value.trim());

  return (
    <main className="chat-workbench-page">
      <section className="chat-workbench-shell" aria-label="Chat 工作台">
        <header className="chat-workbench-header">
          <h1>欢迎来到 刘文波 的工作空间</h1>
          <p>让我们一起高效完成工作吧</p>
        </header>

        <section className="chat-workbench-composer" aria-label="任务输入">
          <TextArea
            aria-label="任务描述"
            autosize={false}
            rows={5}
            value={value}
            onChange={setValue}
            placeholder="Hi，我会根据你的任务特性，自动匹配最佳处理方式。"
            className="chat-workbench-input"
          />

          <div className="chat-workbench-toolbar">
            <div className="chat-workbench-mode" aria-label="模式选择">
              {MODES.map(item => (
                <button
                  key={item}
                  type="button"
                  className="chat-workbench-mode-button"
                  data-active={mode === item}
                  aria-pressed={mode === item}
                  onClick={() => setMode(item)}
                >
                  {item}
                </button>
              ))}
            </div>

            <Button
              color="primary"
              disabled={!canSend}
              icon={<IconCozSendFill />}
              className="chat-workbench-send"
            >
              发送
            </Button>
          </div>
        </section>

        <div className="chat-workbench-templates" aria-label="任务模板">
          {TEMPLATE_TAGS.map(tag => (
            <button key={tag} type="button" className="chat-workbench-tag">
              {tag}
            </button>
          ))}
        </div>
      </section>
    </main>
  );
};

export default WorkbenchPage;
