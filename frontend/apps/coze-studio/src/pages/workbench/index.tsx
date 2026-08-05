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

import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useState } from 'react';

import { useCommonConfigStore } from '@coze-foundation/global-store';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import {
  IconCozArrowDown,
  IconCozBot,
  IconCozCode,
  IconCozDocument,
  IconCozImage,
  IconCozPlus,
  IconCozUpload,
  IconCozWorkflow,
} from '@coze-arch/coze-design/icons';

import './index.less';

import { emitWorkspaceTaskThreadUpsert } from '../tasks/task-thread-events';
import {
  buildTaskThreadDetailPath,
  buildTaskThreadListPath,
} from '../chats/task-thread-routes';
import { WorkspaceHeaderActions } from '../../components/workspace-header-actions';
import {
  isSkillCreationNavigationState,
  SKILL_CREATION_HINT,
} from './skill-creation-intent';
import {
  createTaskThread,
  createTaskThreadRun,
  getWorkbenchLLMModels,
  type TaskThreadUploadedFile,
  uploadTaskThreadFiles,
} from './service';
import { WorkbenchComposer } from './components/workbench-composer';
import {
  stringifyWorkbenchRunConfig,
  WORKBENCH_REQUESTED_POLICY,
  type WorkbenchComposerSubmitPayload,
} from './components/types';

const TEMPLATE_TABS = ['公开模板 6268', '我收藏的', '我创建的'] as const;

const CREATE_SKILL_NAME = 'skill-creator';

const appendUnique = (values: string[], value: string) =>
  values.includes(value) ? values : [...values, value];

const activateSkillCreator = (
  payload: WorkbenchComposerSubmitPayload,
): WorkbenchComposerSubmitPayload => {
  const enableSkills = appendUnique(
    payload.enable_skills ?? [],
    CREATE_SKILL_NAME,
  );
  const allowedSkills = appendUnique(
    payload.runtimeSettings.skills.allowed_skills,
    CREATE_SKILL_NAME,
  );

  return {
    ...payload,
    enable_skills: enableSkills,
    runtimeSettings: {
      ...payload.runtimeSettings,
      skills: {
        ...payload.runtimeSettings.skills,
        enabled: true,
        allowed_skills: allowedSkills,
      },
    },
  };
};

const buildNewTaskRunInput = ({
  message,
  uploadedFiles,
}: {
  message: string;
  uploadedFiles?: TaskThreadUploadedFile[];
}) =>
  JSON.stringify({
    messages: [
      {
        role: 'user',
        content: message,
      },
    ],
    uploaded_files: uploadedFiles ?? [],
  });

const getNewTaskRunMetadata = () =>
  JSON.stringify({
    source: 'workbench_new_task',
    requested_policy: WORKBENCH_REQUESTED_POLICY,
  });

const createNewTaskRunIdempotencyKey = (threadId: string) => {
  const requestId =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;

  return `${threadId}:${requestId}:new-task`;
};

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
    icon: IconCozDocument,
    tone: 'green',
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
    icon: IconCozCode,
    tone: 'blue',
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
    icon: IconCozBot,
    tone: 'violet',
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
    icon: IconCozWorkflow,
    tone: 'orange',
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
    icon: IconCozCode,
    tone: 'amber',
  },
  {
    title: '生成 agent 学习路线',
    description: '基于你的背景与目标，生成结构化的 Agent 学习路线图。',
    tags: ['学习规划', '通用', '公开'],
    prompt: '请根据我的技术背景和学习目标，生成一份 Agent 学习路线图。',
    stats: {
      stars: 412,
      uses: 3186,
    },
    icon: IconCozBot,
    tone: 'cyan',
  },
];

const formatTemplateStat = (value: number) => value.toLocaleString('en-US');

const getUserDisplayName = (userInfo: ReturnType<typeof useUserInfo>): string =>
  userInfo?.name || userInfo?.screen_name || userInfo?.email || '用户';

export const WorkbenchTopbar = () => {
  const navigate = useNavigate();
  const { space_id } = useParams<{ space_id?: string }>();
  const siteName = useCommonConfigStore(state => state.siteConfig.siteName);
  const assistantChatPath = space_id
    ? buildTaskThreadListPath(space_id)
    : undefined;

  return (
    <header className="chat-workbench-topbar" aria-label="工作台状态">
      <div className="chat-workbench-assistant-status">
        <span className="chat-workbench-status-dot" />
        <span>{siteName} 专属助理已就绪，随时可以开始对话</span>
        <button
          type="button"
          disabled={!assistantChatPath}
          onClick={() => {
            if (assistantChatPath) {
              navigate(assistantChatPath);
            }
          }}
        >
          去聊天专属助理
        </button>
      </div>
      <WorkspaceHeaderActions />
    </header>
  );
};

const WorkbenchTitle = () => {
  const userDisplayName = getUserDisplayName(useUserInfo());

  return (
    <header className="chat-workbench-header">
      <h1 aria-label={`欢迎回来，${userDisplayName}`}>
        欢迎回来，<span>{userDisplayName}</span>
      </h1>
    </header>
  );
};

const SkillCreationIntent = () => (
  <section className="chat-workbench-skill-intent" aria-label="AI 创建技能模式">
    <span className="chat-workbench-skill-intent-mark" aria-hidden="true">
      ✦
    </span>
    <strong>AI 创建技能</strong>
    <span>{SKILL_CREATION_HINT}</span>
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
        <div
          className="chat-workbench-template-create-actions"
          aria-label="模板操作"
        >
          <button
            type="button"
            onClick={() => onTemplateSelect('创建一个新的任务模板')}
          >
            <IconCozPlus />
            创建模板
          </button>
          <button type="button">
            <IconCozUpload />
            导入
          </button>
        </div>
      </div>
    </div>

    <div className="chat-workbench-templates">
      {TEMPLATE_CARDS.map(card => {
        const TemplateIcon = card.icon;

        return (
          <button
            key={card.title}
            type="button"
            className="chat-workbench-template-card"
            aria-label={`${card.title} 模板`}
            onClick={() => onTemplateSelect(card.prompt)}
          >
            <span
              className="chat-workbench-template-icon"
              data-tone={card.tone}
              aria-hidden="true"
            >
              <TemplateIcon />
            </span>
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
              <span className="chat-workbench-card-stats">
                <span>☆ {formatTemplateStat(card.stats.stars)}</span>
                <span>
                  <IconCozImage />
                  {formatTemplateStat(card.stats.uses)}
                </span>
              </span>
              <span className="chat-workbench-card-action" aria-hidden="true">
                →
              </span>
            </span>
          </button>
        );
      })}
    </div>
  </section>
);

const WorkbenchPage = () => {
  const { space_id } = useParams();
  const location = useLocation();
  const navigate = useNavigate();
  const skillCreationState = isSkillCreationNavigationState(location.state)
    ? location.state
    : undefined;
  const isSkillCreationMode = Boolean(skillCreationState);
  const [value, setValue] = useState(skillCreationState?.initialMessage ?? '');
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
      const submitPayload = isSkillCreationMode
        ? activateSkillCreator(payload)
        : payload;
      const files = submitPayload.files ?? [];

      if (files.length > 0) {
        const response = await createTaskThread({
          space_id,
          message: submitPayload.message,
          config: stringifyWorkbenchRunConfig(submitPayload),
          defer_start: true,
        });
        const thread = response?.data?.thread;
        if (!thread?.thread_id) {
          navigate(buildTaskThreadListPath(space_id));
          return;
        }

        const uploadResponse = await uploadTaskThreadFiles({
          thread_id: thread.thread_id,
          files,
          space_id,
        });
        await createTaskThreadRun({
          thread_id: thread.thread_id,
          space_id,
          input: buildNewTaskRunInput({
            message: submitPayload.message,
            uploadedFiles: uploadResponse.data?.files,
          }),
          config: stringifyWorkbenchRunConfig(submitPayload),
          metadata: getNewTaskRunMetadata(),
          message_content: submitPayload.message,
          message_metadata: stringifyWorkbenchRunConfig(submitPayload),
          idempotency_key: createNewTaskRunIdempotencyKey(thread.thread_id),
        });

        setValue('');
        emitWorkspaceTaskThreadUpsert({
          space_id,
          thread,
        });
        navigate(buildTaskThreadDetailPath(space_id, thread.thread_id));
        return;
      }

      const response = await createTaskThread({
        space_id,
        message: submitPayload.message,
        config: stringifyWorkbenchRunConfig(submitPayload),
      });

      setValue('');

      if (response?.data?.thread?.thread_id) {
        emitWorkspaceTaskThreadUpsert({
          space_id,
          thread: response.data.thread,
        });
        navigate(
          buildTaskThreadDetailPath(space_id, response.data.thread.thread_id),
        );

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

        {isSkillCreationMode ? <SkillCreationIntent /> : null}

        <WorkbenchComposer
          value={value}
          loading={loading}
          error={error}
          presentation="deerflow"
          spaceId={space_id}
          modelLoader={getWorkbenchLLMModels}
          onValueChange={setValue}
          onSubmit={handleSend}
        />

        <WorkbenchTemplateSection onTemplateSelect={setValue} />
      </section>
    </main>
  );
};

export default WorkbenchPage;
