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

import {
  IconCozArrowDown,
  IconCozBell,
  IconCozImage,
  IconCozLink,
  IconCozPlus,
  IconCozSendFill,
  IconCozUpload,
} from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';
import { workbench } from '@coze-studio/api-schema';

import './index.less';

import { ExtensionsPopover } from './extensions-popover';
import { createWorkbenchTask, sendWorkbenchChat } from './service';

const TEMPLATE_TABS = ['公开模板 6268', '我收藏的', '我创建的'] as const;

const MODES = ['Auto', 'Ask', 'Agent'] as const;

type WorkbenchMode = (typeof MODES)[number];

const MODE_PROMPTS: Record<WorkbenchMode, string> = {
  Auto: 'Hi,我会根据你的任务特性,自动匹配最佳的处理方式~',
  Ask: 'Hi,我会以最快的方式自动响应,为你提供高效且清晰的专业答案~',
  Agent: 'Hi,我会充分思考并灵活使用多种工具,帮你搞定复杂问题~',
};

const MODE_SYMBOLS: Record<WorkbenchMode, string> = {
  Auto: '✦',
  Ask: '?',
  Agent: 'A',
};

const TEMPLATE_CARDS = [
  {
    title: '年度工作总结报告(简洁版)',
    description: '从用户角度切入年度工作内容,生成结构清晰、详略得当的年终汇报材料。',
    tags: ['文档撰写', '通用', '公开'],
    prompt: '帮我生成一份年度工作总结报告，要求结构清晰、简洁专业。',
    stats: {
      stars: 371,
      uses: 5440,
    },
  },
  {
    title: '通过代码生成研发年度报告',
    description: '汇总 MR、OKR 与飞书任务,生成结构化的研发年度总结报告。',
    tags: ['文档撰写', '通用', '公开'],
    prompt: '根据代码提交、OKR 和任务记录，生成研发年度报告。',
    stats: {
      stars: 127,
      uses: 8578,
    },
  },
  {
    title: '通用自动化产品 Meego Bug 根因分析与修复',
    description:
      '该模版聚焦TikTok Main App的产品bug根因分析和修复，根据Meego Ticket,技术分析文档，风神监控等信息输出修复方案。',
    tags: ['代码开发', '服务端', '官方'],
    prompt: '帮我分析 Meego Bug 根因，并给出可执行修复方案。',
    stats: {
      stars: 33,
      uses: 322,
    },
  },
  {
    title: '后端架构整体方案设计',
    description: '设计完整的后端技术方案，涵盖架构、模块、接口、稳定性、监控及代码改动。',
    tags: ['文档撰写', '服务端', '公开'],
    prompt: '请帮我设计一个后端架构整体方案，覆盖模块、链路和扩展性。',
    stats: {
      stars: 859,
      uses: 2396,
    },
  },
  {
    title: 'Go 专家为你 CodeReview',
    description: '作为顶级 Go 专家，系统化执行代码审查，确保代码质量、性能和安全，符合 Go 最佳实践。',
    tags: ['质量检测', '服务端', '公开'],
    prompt: '请作为 Go 专家帮我做一次 CodeReview，并给出修改建议。',
    stats: {
      stars: 874,
      uses: 7977,
    },
  },
];

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
];

export const mapModeToChatMode = (mode: WorkbenchMode): workbench.ChatMode => {
  const modeMap: Record<WorkbenchMode, workbench.ChatMode> = {
    Auto: workbench.ChatMode.Auto,
    Ask: workbench.ChatMode.Ask,
    Agent: workbench.ChatMode.Agent,
  };

  return modeMap[mode];
};

const ColorDots = () => (
  <span className="chat-workbench-color-dots" aria-hidden="true">
    <span data-color="coral" />
    <span data-color="blue" />
    <span data-color="green" />
  </span>
);

const createTaskInput = (message: string) => JSON.stringify({ message });

interface ComposerProps {
  value: string;
  mode: WorkbenchMode;
  canSend: boolean;
  loading: boolean;
  onValueChange: (value: string) => void;
  onModeChange: (mode: WorkbenchMode) => void;
  onSend: () => void;
}

const WorkbenchTopbar = () => (
  <header className="chat-workbench-topbar" aria-label="工作台状态">
    <div className="chat-workbench-assistant-status">
      <span className="chat-workbench-status-dot" />
      <span>Aime 专属助理准备好,先聊聊吧~</span>
      <button type="button">去聊天专属助理</button>
    </div>
    <button
      type="button"
      className="chat-workbench-icon-button"
      aria-label="通知"
    >
      <IconCozBell />
    </button>
    <div className="chat-workbench-avatar" aria-label="当前用户">
      wb
    </div>
  </header>
);

const WorkbenchTitle = () => (
  <header className="chat-workbench-header">
    <h1 aria-label="欢迎来到 刘文波 的工作空间">
      欢迎来到 <span>刘文波 的工作空间</span>
    </h1>
  </header>
);

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

const WorkbenchComposer = ({
  value,
  mode,
  canSend,
  loading,
  onValueChange,
  onModeChange,
  onSend,
}: ComposerProps) => {
  const [atMenuOpen, setAtMenuOpen] = useState(false);

  const handleValueChange = (nextValue: string) => {
    onValueChange(nextValue);
    setAtMenuOpen(nextValue.endsWith('@'));
  };

  return (
    <section className="chat-workbench-composer" aria-label="任务输入">
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
            <span aria-hidden="true">{MODE_SYMBOLS[mode]}</span>
            <span>{MODE_PROMPTS[mode]}</span>
          </div>
        ) : null}
      </div>

      <div className="chat-workbench-toolbar">
        <div className="chat-workbench-toolbar-left">
          <div className="chat-workbench-mode" aria-label="模式选择">
            {MODES.map(item => (
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

          <ExtensionsPopover />
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
          onClick={onSend}
          className="chat-workbench-send"
        >
          <span className="chat-workbench-send-label">
            {loading ? '发送中' : '发送'}
          </span>
        </Button>
      </div>
    </section>
  );
};

const WorkbenchTemplateSection = ({
  onTemplateSelect,
}: {
  onTemplateSelect: (prompt: string) => void;
}) => (
  <section className="chat-workbench-template-section" aria-label="任务模板">
    <div className="chat-workbench-template-controls">
      <div className="chat-workbench-template-tabs" role="tablist">
        {TEMPLATE_TABS.map((tab, index) => (
          <button
            key={tab}
            type="button"
            role="tab"
            aria-selected={index === 0}
            data-active={index === 0}
          >
            {tab}
          </button>
        ))}
      </div>

      <div className="chat-workbench-template-tools">
        <label className="chat-workbench-search">
          <span aria-hidden="true">⌕</span>
          <input aria-label="搜索模板" placeholder="搜索模板" />
        </label>
        <button type="button" className="chat-workbench-sort">
          推荐排序
          <IconCozArrowDown />
        </button>
      </div>
    </div>

    <div className="chat-workbench-templates">
      <article className="chat-workbench-create-card">
        <h2>创建模板</h2>
        <p>沉淀可复用的指令与配置</p>
        <div className="chat-workbench-create-preview">
          <span>Template</span>
        </div>
        <div className="chat-workbench-create-actions">
          <button
            type="button"
            onClick={() => onTemplateSelect('创建一个新的任务模板')}
          >
            <IconCozPlus />
            创建
          </button>
          <button type="button">
            <IconCozUpload />
            导入
          </button>
        </div>
      </article>

      {TEMPLATE_CARDS.map(card => (
        <button
          key={card.title}
          type="button"
          className="chat-workbench-template-card"
          aria-label={`${card.title} 模板`}
          onClick={() => onTemplateSelect(card.prompt)}
        >
          <span className="chat-workbench-template-title">{card.title}</span>
          <span className="chat-workbench-tags">
            {card.tags.map(tag => (
              <span key={tag}>{tag}</span>
            ))}
          </span>
          <span className="chat-workbench-template-desc">
            {card.description}
          </span>
          <span className="chat-workbench-card-footer">
            <ColorDots />
            <span className="chat-workbench-card-stats">
              <span>☆ {card.stats.stars}</span>
              <span>
                <IconCozImage />
                {card.stats.uses}
              </span>
            </span>
            <span className="chat-workbench-card-action" aria-hidden="true">
              →
            </span>
          </span>
        </button>
      ))}
    </div>
  </section>
);

const WorkbenchPage = () => {
  const { space_id } = useParams();
  const navigate = useNavigate();
  const [value, setValue] = useState('');
  const [mode, setMode] = useState<WorkbenchMode>('Auto');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const canSend = Boolean(value.trim()) && Boolean(space_id) && !loading;

  const navigateToCreatedTask = async (spaceId: string, message: string) => {
    const taskResponse = await createWorkbenchTask({
      space_id: spaceId,
      title: message,
      input: createTaskInput(message),
    });

    if (taskResponse.data?.id) {
      navigate(`/space/${spaceId}/tasks/${taskResponse.data.id}`);

      return;
    }

    navigate(`/space/${spaceId}/tasks`);
  };

  const handleSend = async () => {
    const message = value.trim();

    if (!message || !space_id || loading) {
      return;
    }

    setLoading(true);
    setError('');

    try {
      let response: Awaited<ReturnType<typeof sendWorkbenchChat>> | undefined;

      try {
        response = await sendWorkbenchChat({
          space_id,
          message,
          mode: mapModeToChatMode(mode),
        });
      } catch {
        response = undefined;
      }

      setValue('');

      if (response?.data?.task?.id) {
        navigate(`/space/${space_id}/tasks/${response.data.task.id}`);

        return;
      }

      await navigateToCreatedTask(space_id, message);
    } catch (err) {
      setError(err instanceof Error ? err.message : '发送失败，请稍后重试');
    } finally {
      setLoading(false);
    }
  };

  return (
    <main className="chat-workbench-page">
      <WorkbenchTopbar />

      <section className="chat-workbench-shell" aria-label="Chat 工作台">
        <WorkbenchTitle />

        <WorkbenchComposer
          value={value}
          mode={mode}
          canSend={canSend}
          loading={loading}
          onValueChange={setValue}
          onModeChange={setMode}
          onSend={handleSend}
        />

        {error ? (
          <div className="chat-workbench-error" role="alert">
            {error}
          </div>
        ) : null}

        <WorkbenchTemplateSection onTemplateSelect={setValue} />
      </section>
    </main>
  );
};

export default WorkbenchPage;
