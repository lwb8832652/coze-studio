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
import { useState } from 'react';

import {
  IconCozArrowDown,
  IconCozBell,
  IconCozImage,
  IconCozPlus,
  IconCozUpload,
} from '@coze-arch/coze-design/icons';

import './index.less';

import {
  buildTaskThreadDetailPath,
  buildTaskThreadListPath,
} from '../chats/task-thread-routes';
import { getWorkbenchLLMModels, sendWorkbenchChat } from './service';
import { WorkbenchComposer } from './components/workbench-composer';
import {
  mapModeToChatMode,
  stringifyWorkbenchRunConfig,
  type WorkbenchComposerSubmitPayload,
  type WorkbenchMode,
} from './components/types';

export { mapModeToChatMode } from './components/types';

const TEMPLATE_TABS = ['公开模板 6268', '我收藏的', '我创建的'] as const;

const TEMPLATE_CARDS = [
  {
    title: '年度工作总结报告(简洁版)',
    description:
      '从用户角度切入年度工作内容,生成结构清晰、详略得当的年终汇报材料。',
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
    description:
      '设计完整的后端技术方案，涵盖架构、模块、接口、稳定性、监控及代码改动。',
    tags: ['文档撰写', '服务端', '公开'],
    prompt: '请帮我设计一个后端架构整体方案，覆盖模块、链路和扩展性。',
    stats: {
      stars: 859,
      uses: 2396,
    },
  },
  {
    title: 'Go 专家为你 CodeReview',
    description:
      '作为顶级 Go 专家，系统化执行代码审查，确保代码质量、性能和安全，符合 Go 最佳实践。',
    tags: ['质量检测', '服务端', '公开'],
    prompt: '请作为 Go 专家帮我做一次 CodeReview，并给出修改建议。',
    stats: {
      stars: 874,
      uses: 7977,
    },
  },
];

const ColorDots = () => (
  <span className="chat-workbench-color-dots" aria-hidden="true">
    <span data-color="coral" />
    <span data-color="blue" />
    <span data-color="green" />
  </span>
);

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

  const handleSend = async (payload: WorkbenchComposerSubmitPayload) => {
    if (!payload.message || loading) {
      return;
    }

    if (!space_id) {
      setError('缺少空间信息，无法发送任务');

      return;
    }

    setLoading(true);
    setError('');

    try {
      const response = await sendWorkbenchChat({
        space_id,
        message: payload.message,
        mode: mapModeToChatMode(payload.mode),
        runtime_settings: stringifyWorkbenchRunConfig(payload),
        ...(payload.modelType
          ? {
              model_type: String(payload.modelType),
              model_name: payload.modelName,
            }
          : {}),
        enable_skills: payload.enable_skills,
        enable_mcp: payload.enable_mcp,
        enable_kbs: payload.enable_kbs,
        enable_databases: payload.enable_databases,
      });

      setValue('');

      if (response?.data?.task?.id) {
        navigate(buildTaskThreadDetailPath(space_id, response.data.task.id));

        return;
      }

      navigate(buildTaskThreadListPath(space_id));
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
          loading={loading}
          error={error}
          spaceId={space_id}
          modelLoader={getWorkbenchLLMModels}
          onValueChange={setValue}
          onModeChange={setMode}
          onSubmit={handleSend}
        />

        <WorkbenchTemplateSection onTemplateSelect={setValue} />
      </section>
    </main>
  );
};

export default WorkbenchPage;
