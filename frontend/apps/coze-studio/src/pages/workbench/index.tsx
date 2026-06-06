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
import { useParams } from 'react-router-dom';

import {
  IconCozArrowDown,
  IconCozImage,
  IconCozLink,
  IconCozPlus,
  IconCozSendFill,
  IconCozUpload,
} from '@coze-arch/coze-design/icons';
import { Button, TextArea } from '@coze-arch/coze-design';
import { workbench, workbenchTask } from '@coze-studio/api-schema';

import './index.less';

import { sendWorkbenchChat } from './service';

const TEMPLATE_TABS = ['公开模板 6268', '我收藏的', '我创建的'] as const;

const TEMPLATE_CARDS = [
  {
    title: '年度工作总结报告(简洁版)',
    description: '从用户角度切入年度工作内容,生成结构清晰、详略得当的年终汇报材料。',
    tags: ['文档投递', '教育', '公开'],
    prompt: '帮我生成一份年度工作总结报告，要求结构清晰、简洁专业。',
    stats: {
      stars: 371,
      uses: 5440,
    },
  },
  {
    title: '通过代码生成研发年度报告',
    description: '汇总 MR、OKR 与飞书任务,生成结构化的研发年度总结报告。',
    tags: ['飞书文档', '研发', '公开'],
    prompt: '根据代码提交、OKR 和任务记录，生成研发年度报告。',
    stats: {
      stars: 127,
      uses: 8578,
    },
  },
  {
    title: '通用自动化产品 Meego Bug 根因分析与修复',
    description: '结合联系 Main App 打作 Meego Ticket,自动定位与修复模型。',
    tags: ['自动化', 'Bug', '公开'],
    prompt: '帮我分析 Meego Bug 根因，并给出可执行修复方案。',
    stats: {
      stars: 33,
      uses: 322,
    },
  },
  {
    title: '后端架构整体方案设计',
    description: '设计高可用、可扩展的后端架构方案,涵盖通信、模块、链路全要素。',
    tags: ['架构', '后端', '公开'],
    prompt: '请帮我设计一个后端架构整体方案，覆盖模块、链路和扩展性。',
    stats: {
      stars: 859,
      uses: 2396,
    },
  },
  {
    title: 'Go 专家为你 CodeReview',
    description: '扮演资深 Go 专家,系统化对代码进行结构、可读性、性能等维度的审查。',
    tags: ['Go', '代码', '公开'],
    prompt: '请作为 Go 专家帮我做一次 CodeReview，并给出修改建议。',
    stats: {
      stars: 874,
      uses: 7977,
    },
  },
];

const MODES = ['Auto', 'Ask', 'Agent'] as const;

type WorkbenchMode = (typeof MODES)[number];

type WorkbenchChatData = workbench.WorkbenchChatData;
type ChatTask = workbenchTask.ChatTask;

export const mapModeToChatMode = (mode: WorkbenchMode): workbench.ChatMode => {
  const modeMap: Record<WorkbenchMode, workbench.ChatMode> = {
    Auto: workbench.ChatMode.Auto,
    Ask: workbench.ChatMode.Ask,
    Agent: workbench.ChatMode.Agent,
  };

  return modeMap[mode];
};

export const getTaskStatusText = (status: workbenchTask.TaskStatus) => {
  const statusMap: Record<workbenchTask.TaskStatus, string> = {
    [workbenchTask.TaskStatus.Created]: '已创建',
    [workbenchTask.TaskStatus.Queued]: '排队中',
    [workbenchTask.TaskStatus.Running]: '运行中',
    [workbenchTask.TaskStatus.Succeeded]: '已完成',
    [workbenchTask.TaskStatus.Failed]: '失败',
    [workbenchTask.TaskStatus.Canceling]: '取消中',
    [workbenchTask.TaskStatus.Canceled]: '已取消',
  };

  return statusMap[status] ?? '未知';
};

const TaskCard = ({ task }: { task: ChatTask }) => (
  <article className="chat-workbench-task-card" aria-label="任务结果">
    <div className="chat-workbench-task-main">
      <h2>{task.title}</h2>
      <span>{getTaskStatusText(task.status)}</span>
    </div>
    <div className="chat-workbench-progress-row">
      <progress value={task.progress} max={100} />
      <span>{task.progress}%</span>
    </div>
  </article>
);

const ColorDots = () => (
  <span className="chat-workbench-color-dots" aria-hidden="true">
    <span data-color="coral" />
    <span data-color="blue" />
    <span data-color="green" />
  </span>
);

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
      ♡
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

const WorkbenchComposer = ({
  value,
  mode,
  canSend,
  loading,
  onValueChange,
  onModeChange,
  onSend,
}: ComposerProps) => (
  <section className="chat-workbench-composer" aria-label="任务输入">
    <div className="chat-workbench-composer-prompt">
      <span aria-hidden="true">✣</span>
      <span>Hi,我会根据你的任务特性,自动匹配最佳的处理方式~</span>
    </div>
    <TextArea
      aria-label="任务描述"
      autosize={false}
      rows={4}
      value={value}
      onChange={onValueChange}
      placeholder=""
      className="chat-workbench-input"
    />

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

        <button type="button" className="chat-workbench-extension">
          <span>拓展 47</span>
          <IconCozArrowDown />
        </button>
      </div>

      <div className="chat-workbench-toolbar-actions">
        <button type="button" aria-label="添加上下文">
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
          <span />
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
          </span>
        </button>
      ))}
    </div>
  </section>
);

const WorkbenchPage = () => {
  const { space_id } = useParams();
  const [value, setValue] = useState('');
  const [mode, setMode] = useState<WorkbenchMode>('Auto');
  const [chatData, setChatData] = useState<WorkbenchChatData | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const canSend = Boolean(value.trim()) && Boolean(space_id) && !loading;

  const handleSend = async () => {
    const message = value.trim();

    if (!message || !space_id || loading) {
      return;
    }

    setLoading(true);
    setError('');

    try {
      const response = await sendWorkbenchChat({
        space_id,
        message,
        mode: mapModeToChatMode(mode),
      });

      setChatData(response.data);
      setValue('');
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

        {chatData?.answer || chatData?.task ? (
          <section className="chat-workbench-result" aria-label="执行结果">
            {chatData.answer ? (
              <article className="chat-workbench-answer">
                {chatData.answer}
              </article>
            ) : null}
            {chatData.task ? <TaskCard task={chatData.task} /> : null}
          </section>
        ) : null}

        <WorkbenchTemplateSection onTemplateSelect={setValue} />
      </section>
    </main>
  );
};

export default WorkbenchPage;
