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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function -- Cohesive orchestrator. */
/* eslint-disable max-lines, complexity -- Cohesive orchestrator. */

import { useEffect, useMemo, useState } from 'react';

import type { AppDevFileNode } from '../types';
import { useAppDevModelSelector } from '../hooks/use-app-dev-model-selector';
import { useAppDevDataSources } from '../hooks/use-app-dev-data-sources';
import { useAppDevChat } from '../hooks/use-app-dev-chat';
import { ChatMessage } from './chat-message';

const QUICK_PROMPTS = [
  '创建一个响应式产品官网：首页、功能亮点、价格区和联系我们',
  '把当前页面改成更现代的 SaaS 落地页，突出转化按钮',
  '增加一个数据看板页面，包含指标卡片、趋势图和明细列表',
];

export const APP_DEV_DESIGN_TARGETS = [
  '全局视觉系统',
  '首页首屏',
  '导航与菜单',
  '内容卡片',
  '表单与输入区',
  '数据看板',
  '移动端适配',
];

const DESIGN_PRESETS = [
  '现代 SaaS',
  '数据产品',
  '品牌营销',
  '极简内容',
  '移动优先',
  '运营活动',
];

const DESIGN_COLORS = [
  '清爽蓝绿',
  '暖橙品牌',
  '黑白高级',
  '自然绿色',
  '科技蓝紫',
  '中性色专业',
];

const DESIGN_MOTION = ['轻量动效', '强调转化动效', '减少动效', '保留现有动效'];

const DESIGN_LAYOUT = [
  '保持当前布局',
  '增强留白层次',
  '紧凑信息密度',
  '卡片化分组',
  '左右分栏结构',
];

const DESIGN_RADIUS = [
  '保持当前圆角',
  '小圆角 8px',
  '中等圆角 16px',
  '大圆角 24px',
  '胶囊按钮',
];

const DESIGN_SHADOW = [
  '轻量阴影',
  '清晰层级阴影',
  '无阴影扁平',
  '玻璃拟态',
  '边框替代阴影',
];

const DESIGN_TYPOGRAPHY = [
  '保持当前字号',
  '强化标题层级',
  '提升正文可读性',
  '紧凑数据字体',
  '营销大标题',
];

const maxReferencedFileCount = 8;

const flattenProjectFiles = (nodes: AppDevFileNode[] = []): AppDevFileNode[] =>
  nodes.flatMap(node => [
    node,
    ...(node.children?.length ? flattenProjectFiles(node.children) : []),
  ]);

const formatReferenceNodeType = (node: AppDevFileNode) =>
  node.type === 'directory' ? '目录' : '文件';

export interface AppDevDesignModeState {
  active: boolean;
  summary: string;
  prompt: string;
}

export interface AppDevDesignSelection {
  target: string;
  device: 'desktop' | 'tablet' | 'mobile';
  xPercent: number;
  yPercent: number;
  xPx: number;
  yPx: number;
  viewportWidth: number;
  viewportHeight: number;
  selectedAt: string;
}

interface ChatPanelProps {
  spaceId?: string;
  projectId?: string;
  projectFiles?: AppDevFileNode[];
  hasPendingFileChanges?: boolean;
  onTaskSettled?: () => void;
  onFilesChanged?: () => void;
  openDesignSignal?: number;
  designTargetOverride?: string;
  designSelection?: AppDevDesignSelection | null;
  onBeforeDesignExecute?: (prompt: string) => Promise<void> | void;
  onDesignTargetChange?: (target: string) => void;
  onDesignModeChange?: (state: AppDevDesignModeState) => void;
}

export const ChatPanel = ({
  spaceId,
  projectId,
  projectFiles,
  hasPendingFileChanges,
  onTaskSettled,
  onFilesChanged,
  openDesignSignal,
  designTargetOverride,
  designSelection,
  onBeforeDesignExecute,
  onDesignTargetChange,
  onDesignModeChange,
}: ChatPanelProps) => {
  const [contextOpen, setContextOpen] = useState(false);
  const [designOpen, setDesignOpen] = useState(false);
  const [mentionOpen, setMentionOpen] = useState(false);
  const [composerFocused, setComposerFocused] = useState(false);
  const [dataSourceKeyword, setDataSourceKeyword] = useState('');
  const [referenceKeyword, setReferenceKeyword] = useState('');
  const [referencedFilePaths, setReferencedFilePaths] = useState<string[]>([]);
  const [designTarget, setDesignTarget] = useState(APP_DEV_DESIGN_TARGETS[0]);
  const [designPreset, setDesignPreset] = useState(DESIGN_PRESETS[0]);
  const [designColor, setDesignColor] = useState(DESIGN_COLORS[0]);
  const [designMotion, setDesignMotion] = useState(DESIGN_MOTION[0]);
  const [designLayout, setDesignLayout] = useState(DESIGN_LAYOUT[0]);
  const [designRadius, setDesignRadius] = useState(DESIGN_RADIUS[0]);
  const [designShadow, setDesignShadow] = useState(DESIGN_SHADOW[0]);
  const [designTypography, setDesignTypography] = useState(
    DESIGN_TYPOGRAPHY[0],
  );
  const [designNote, setDesignNote] = useState('');
  const [designExecuteError, setDesignExecuteError] = useState('');
  const [pendingChangesError, setPendingChangesError] = useState('');
  const [preparingDesignExecute, setPreparingDesignExecute] = useState(false);
  const [runningStartedAt, setRunningStartedAt] = useState<number | null>(null);
  const [runningClock, setRunningClock] = useState(() => Date.now());
  const modelSelector = useAppDevModelSelector(spaceId);
  const dataSourceSelector = useAppDevDataSources(spaceId);
  const chat = useAppDevChat(
    spaceId,
    projectId,
    modelSelector.selectedModelId,
    dataSourceSelector.selectedDataSources,
    onTaskSettled,
    onFilesChanged,
  );
  const selectedModel = useMemo(
    () =>
      modelSelector.models.find(
        model => model.id === modelSelector.selectedModelId,
      ),
    [modelSelector.models, modelSelector.selectedModelId],
  );
  const selectedModelSupportsPrototypeImages = Boolean(
    selectedModel?.supportsMultiModal &&
      selectedModel.supportsImageUnderstanding &&
      selectedModel.enableBase64URL,
  );
  const hasPrototypeImageAttachment = chat.attachments.some(
    attachment => attachment.type === 'prototype_image',
  );
  const prototypeImageCapabilityHint = selectedModelSupportsPrototypeImages
    ? '当前模型支持读取原型图，会按截图/原型的布局、层级和视觉结构生成页面。'
    : selectedModel
      ? '当前模型未同时开启多模态、图片理解和 Base64 URL，原型图会作为附件路径与素材说明发送，模型可能无法直接读取图片内容。'
      : '选择支持多模态、图片理解和 Base64 URL 的模型后，原型图会参与视觉理解。';
  const selectedDataSources = useMemo(
    () =>
      dataSourceSelector.dataSources.filter(dataSource =>
        dataSourceSelector.selectedIds.includes(dataSource.id),
      ),
    [dataSourceSelector.dataSources, dataSourceSelector.selectedIds],
  );
  const filteredDataSources = useMemo(() => {
    const keyword = dataSourceKeyword.trim().toLowerCase();
    if (!keyword) {
      return dataSourceSelector.dataSources;
    }

    return dataSourceSelector.dataSources.filter(dataSource =>
      [dataSource.name, dataSource.type, dataSource.description]
        .filter(Boolean)
        .some(value => value?.toLowerCase().includes(keyword)),
    );
  }, [dataSourceKeyword, dataSourceSelector.dataSources]);
  const allReferenceFiles = useMemo(
    () =>
      flattenProjectFiles(projectFiles).filter(
        file => file.path && file.path !== '/',
      ),
    [projectFiles],
  );
  const referencedFiles = useMemo(
    () =>
      referencedFilePaths
        .map(path => allReferenceFiles.find(file => file.path === path))
        .filter((file): file is AppDevFileNode => Boolean(file)),
    [allReferenceFiles, referencedFilePaths],
  );
  const filteredReferenceFiles = useMemo(() => {
    const keyword = referenceKeyword.trim().toLowerCase();
    const matchedFiles = keyword
      ? allReferenceFiles.filter(file =>
          [file.name, file.path, file.type]
            .filter(Boolean)
            .some(value => value.toLowerCase().includes(keyword)),
        )
      : allReferenceFiles;

    return matchedFiles.slice(0, 40);
  }, [allReferenceFiles, referenceKeyword]);
  const filteredReferenceDataSources = useMemo(() => {
    const keyword = referenceKeyword.trim().toLowerCase();
    if (!keyword) {
      return dataSourceSelector.dataSources;
    }

    return dataSourceSelector.dataSources.filter(dataSource =>
      [dataSource.name, dataSource.type, dataSource.description]
        .filter(Boolean)
        .some(value => value?.toLowerCase().includes(keyword)),
    );
  }, [dataSourceSelector.dataSources, referenceKeyword]);
  const selectedReferenceCount =
    referencedFiles.length + dataSourceSelector.selectedIds.length;
  const canSend =
    !chat.running &&
    Boolean(
      chat.input.trim() || chat.attachments.length || referencedFiles.length,
    ) &&
    !chat.uploadingAttachments &&
    !modelSelector.loading &&
    Boolean(modelSelector.selectedModelId) &&
    modelSelector.hasUsableModel &&
    !preparingDesignExecute;
  const canExecuteDesign =
    !chat.running &&
    !chat.uploadingAttachments &&
    !modelSelector.loading &&
    Boolean(modelSelector.selectedModelId) &&
    modelSelector.hasUsableModel &&
    !preparingDesignExecute;
  const pendingChangesMessage =
    '当前代码有未保存修改，请先保存当前文件，再让 AI 继续开发。';
  const ensureNoPendingChanges = () => {
    if (hasPendingFileChanges) {
      setPendingChangesError(pendingChangesMessage);
      return false;
    }

    setPendingChangesError('');
    return true;
  };
  const buildPromptWithReferences = (messageOverride?: string) => {
    if (!referencedFiles.length) {
      return messageOverride;
    }

    const messageSource =
      typeof messageOverride === 'string' ? messageOverride : chat.input;
    const message =
      messageSource.trim() || '请参考已选择的项目文件继续开发页面';
    const referenceLines = referencedFiles.map(
      file => `- ${file.path}（${formatReferenceNodeType(file)}）`,
    );

    return [
      message,
      '',
      '本次需求请重点参考以下项目文件/目录：',
      ...referenceLines,
    ].join('\n');
  };
  const sendPrompt = async (messageOverride?: string) => {
    if (!ensureNoPendingChanges()) {
      return;
    }

    const sent = await chat.sendMessage(
      buildPromptWithReferences(messageOverride),
    );
    if (sent) {
      setReferencedFilePaths([]);
      setMentionOpen(false);
      setReferenceKeyword('');
    }
  };
  const toggleReferencedFile = (file: AppDevFileNode) => {
    setReferencedFilePaths(current => {
      if (current.includes(file.path)) {
        return current.filter(path => path !== file.path);
      }

      if (current.length >= maxReferencedFileCount) {
        return current;
      }

      return [...current, file.path];
    });
  };
  useEffect(() => {
    if (!chat.running) {
      setRunningStartedAt(null);
      return;
    }

    setRunningStartedAt(current => current ?? Date.now());
    setRunningClock(Date.now());
    const timer = window.setInterval(() => {
      setRunningClock(Date.now());
    }, 30 * 1000);
    return () => window.clearInterval(timer);
  }, [chat.running]);
  useEffect(() => {
    if (!hasPendingFileChanges) {
      setPendingChangesError('');
    }
  }, [hasPendingFileChanges]);
  const runningDurationMs = runningStartedAt
    ? runningClock - runningStartedAt
    : 0;
  const runningDurationMinutes = Math.max(
    1,
    Math.floor(runningDurationMs / 60_000),
  );
  const isLongRunningTask = chat.running && runningDurationMs >= 3 * 60_000;
  const designPrompt = useMemo(() => {
    const note = designNote.trim();
    const selectionLine = designSelection
      ? `预览选区：${designSelection.target}，${designSelection.device} 视图，点击位置 ${designSelection.xPercent}% / ${designSelection.yPercent}%（视口 ${designSelection.viewportWidth}x${designSelection.viewportHeight}）。`
      : '';
    return [
      '请按网页应用设计模式修改当前项目。',
      `目标区域：${designTarget}`,
      `视觉风格：${designPreset}`,
      `色彩方向：${designColor}`,
      `动效要求：${designMotion}`,
      `设计属性：布局=${designLayout}；圆角=${designRadius}；阴影=${designShadow}；字体=${designTypography}。`,
      selectionLine,
      '请直接修改相关源码和样式，保持现有功能可用，完成后说明修改了哪些文件和可见变化。',
      note ? `补充说明：${note}` : '',
    ]
      .filter(Boolean)
      .join('\n');
  }, [
    designColor,
    designLayout,
    designMotion,
    designNote,
    designPreset,
    designRadius,
    designSelection,
    designShadow,
    designTarget,
    designTypography,
  ]);
  const designSummary = useMemo(
    () => `${designPreset} · ${designTarget}`,
    [designPreset, designTarget],
  );
  useEffect(() => {
    onDesignModeChange?.({
      active: designOpen,
      summary: designSummary,
      prompt: designPrompt,
    });
  }, [designOpen, designPrompt, designSummary, onDesignModeChange]);
  useEffect(() => {
    if (!openDesignSignal) {
      return;
    }

    setDesignOpen(true);
    setContextOpen(false);
  }, [openDesignSignal]);
  useEffect(() => {
    if (
      !designTargetOverride ||
      designTargetOverride === designTarget ||
      !APP_DEV_DESIGN_TARGETS.includes(designTargetOverride)
    ) {
      return;
    }

    setDesignTarget(designTargetOverride);
  }, [designTarget, designTargetOverride]);
  const updateDesignTarget = (target: string) => {
    setDesignTarget(target);
    onDesignTargetChange?.(target);
  };
  const applyDesignPrompt = () => {
    chat.setInput(designPrompt);
    setDesignOpen(false);
    setContextOpen(false);
  };
  const executeDesignPrompt = async () => {
    if (!ensureNoPendingChanges()) {
      return;
    }

    setPreparingDesignExecute(true);
    setDesignExecuteError('');
    try {
      await onBeforeDesignExecute?.(designPrompt);
      setDesignOpen(false);
      setContextOpen(false);
      const sent = await chat.sendMessage(
        buildPromptWithReferences(designPrompt),
      );
      if (sent) {
        setReferencedFilePaths([]);
        setMentionOpen(false);
        setReferenceKeyword('');
      }
    } catch (error) {
      setDesignExecuteError(
        error instanceof Error ? error.message : '设计任务执行前准备失败',
      );
    } finally {
      setPreparingDesignExecute(false);
    }
  };

  return (
    <div className="app-dev-chat-panel" data-running={chat.running}>
      <header className="app-dev-chat-panel__header">
        <div>
          <span
            className="app-dev-chat-panel__assistant-avatar"
            aria-hidden="true"
          >
            AI
          </span>
          <div>
            <strong>AI 应用开发助手</strong>
            <span>
              {chat.running
                ? '正在生成页面代码与运行步骤'
                : '描述需求，助手会持续修改项目文件'}
            </span>
          </div>
        </div>
        <div className="app-dev-chat-panel__header-meta">
          <span data-active={chat.running}>
            {chat.running ? '执行中' : '待命'}
          </span>
          <span>{selectedModel?.name || '未选择模型'}</span>
        </div>
      </header>

      <div className="app-dev-chat-panel__messages">
        {chat.loadingHistory ? (
          <div className="app-dev-chat-panel__empty">加载历史会话...</div>
        ) : null}
        {!chat.loadingHistory && !chat.messages.length ? (
          <div className="app-dev-chat-panel__welcome">
            <span>Start building</span>
            <strong>告诉我你想做的网页应用</strong>
            <p>
              可以描述业务目标、页面结构、交互流程，也可以粘贴截图或上传素材。我会把需求拆成任务，并同步更新右侧预览和代码。
            </p>
            <div>
              <em>页面生成</em>
              <em>代码修改</em>
              <em>实时预览</em>
            </div>
            <div className="app-dev-chat-panel__quick-prompts">
              {QUICK_PROMPTS.map(prompt => (
                <button
                  key={prompt}
                  type="button"
                  onClick={() => chat.setInput(prompt)}
                  disabled={chat.running}
                >
                  {prompt}
                </button>
              ))}
            </div>
          </div>
        ) : null}
        {chat.messages.map(message => (
          <ChatMessage key={message.id} message={message} />
        ))}
      </div>

      {chat.error ? (
        <div className="app-dev-chat-panel__error">{chat.error}</div>
      ) : null}
      {modelSelector.error ? (
        <div className="app-dev-chat-panel__error">{modelSelector.error}</div>
      ) : null}
      {dataSourceSelector.error ? (
        <div className="app-dev-chat-panel__error">
          {dataSourceSelector.error}
        </div>
      ) : null}
      {hasPendingFileChanges || pendingChangesError ? (
        <div className="app-dev-chat-panel__pending-warning">
          {pendingChangesError || pendingChangesMessage}
        </div>
      ) : null}
      {!modelSelector.loading && !modelSelector.hasUsableModel ? (
        <div className="app-dev-chat-panel__model-warning">
          当前没有可用于网页应用开发的模型，请先到
          <a href="/system/models">系统管理的模型配置</a>
          中完成配置。
        </div>
      ) : null}
      {isLongRunningTask ? (
        <div className="app-dev-chat-panel__long-running">
          <div>
            <strong>本次任务执行时间较长</strong>
            <span>
              当前页面已等待约 {runningDurationMinutes}{' '}
              分钟。可以刷新状态；如果仍未结束，建议取消后重试。
            </span>
          </div>
          <button type="button" onClick={() => void chat.refreshHistory()}>
            刷新状态
          </button>
        </div>
      ) : null}

      <footer
        className="app-dev-chat-panel__composer"
        data-focused={composerFocused}
      >
        <div className="app-dev-chat-panel__composer-toolbar">
          <button
            type="button"
            onClick={() => {
              setContextOpen(open => !open);
              setDesignOpen(false);
              setMentionOpen(false);
            }}
            aria-expanded={contextOpen}
          >
            上下文
            <span>{selectedDataSources.length} 个数据源</span>
          </button>
          <button
            type="button"
            onClick={() => {
              setMentionOpen(open => !open);
              setContextOpen(false);
              setDesignOpen(false);
            }}
            aria-expanded={mentionOpen}
          >
            @ 引用
            <span>{selectedReferenceCount} 个上下文</span>
          </button>
          <button
            type="button"
            onClick={() => {
              setDesignOpen(open => !open);
              setContextOpen(false);
              setMentionOpen(false);
            }}
            aria-expanded={designOpen}
          >
            设计模式
            <span>{designPreset}</span>
          </button>
          <span>⌘ Enter 发送</span>
        </div>
        {mentionOpen ? (
          <section className="app-dev-chat-panel__mention-panel">
            <header>
              <div>
                <span>@ 引用上下文</span>
                <strong>选择项目文件、目录或数据源</strong>
              </div>
              <p>
                文件/目录会写入本次需求说明；数据源会复用上下文发送，只传名称和元信息。
              </p>
            </header>
            <label className="app-dev-chat-panel__mention-search">
              <span>搜索引用项</span>
              <input
                value={referenceKeyword}
                placeholder="输入文件名、路径或数据源名称"
                onChange={event => setReferenceKeyword(event.target.value)}
              />
              {referenceKeyword.trim() ? (
                <button type="button" onClick={() => setReferenceKeyword('')}>
                  清空
                </button>
              ) : null}
            </label>
            <div className="app-dev-chat-panel__mention-columns">
              <section>
                <strong>项目文件与目录</strong>
                {filteredReferenceFiles.length ? (
                  <div className="app-dev-chat-panel__mention-list">
                    {filteredReferenceFiles.map(file => {
                      const selected = referencedFiles.some(
                        item => item.path === file.path,
                      );

                      return (
                        <button
                          type="button"
                          key={file.path}
                          data-selected={selected}
                          disabled={
                            !selected &&
                            referencedFiles.length >= maxReferencedFileCount
                          }
                          onClick={() => toggleReferencedFile(file)}
                        >
                          <span>
                            <strong>{file.name}</strong>
                            <em>{file.path}</em>
                          </span>
                          <small>{formatReferenceNodeType(file)}</small>
                        </button>
                      );
                    })}
                  </div>
                ) : (
                  <p>暂无匹配文件或目录</p>
                )}
              </section>
              <section>
                <strong>数据源</strong>
                {dataSourceSelector.loading ? (
                  <p>正在加载数据源...</p>
                ) : filteredReferenceDataSources.length ? (
                  <div className="app-dev-chat-panel__mention-list">
                    {filteredReferenceDataSources.map(dataSource => {
                      const selected = dataSourceSelector.selectedIds.includes(
                        dataSource.id,
                      );

                      return (
                        <button
                          type="button"
                          key={dataSource.id}
                          data-selected={selected}
                          disabled={
                            !selected &&
                            dataSourceSelector.selectedIds.length >= 8
                          }
                          onClick={() =>
                            dataSourceSelector.toggleDataSource(dataSource.id)
                          }
                        >
                          <span>
                            <strong>{dataSource.name}</strong>
                            <em>{dataSource.description || dataSource.type}</em>
                          </span>
                          <small>{dataSource.type}</small>
                        </button>
                      );
                    })}
                  </div>
                ) : (
                  <p>暂无匹配数据源</p>
                )}
              </section>
            </div>
            {referencedFiles.length >= maxReferencedFileCount ? (
              <p className="app-dev-chat-panel__mention-limit">
                单次最多引用 {maxReferencedFileCount} 个文件或目录。
              </p>
            ) : null}
          </section>
        ) : null}
        {contextOpen ? (
          <section className="app-dev-chat-panel__context">
            <div className="app-dev-chat-panel__context-overview">
              <div>
                <span>当前模型</span>
                <strong>{selectedModel?.name || '未选择模型'}</strong>
                <em>{selectedModel?.provider || '未配置服务商'}</em>
              </div>
              <div>
                <span>数据源上下文</span>
                <strong>{selectedDataSources.length}/8</strong>
                <em>
                  {selectedDataSources.length
                    ? '会随本次需求发送名称和元信息'
                    : '未绑定数据源'}
                </em>
              </div>
            </div>
            <div className="app-dev-chat-panel__model-row">
              <span>编码模型</span>
              <select
                value={modelSelector.selectedModelId}
                disabled={
                  modelSelector.loading || !modelSelector.hasUsableModel
                }
                onChange={event =>
                  modelSelector.setSelectedModelId(event.target.value)
                }
              >
                {modelSelector.loading ? (
                  <option value="">加载模型中...</option>
                ) : null}
                {!modelSelector.loading && !modelSelector.models.length ? (
                  <option value="">暂无可用模型</option>
                ) : null}
                {modelSelector.models.map(model => (
                  <option value={model.id} key={model.id}>
                    {model.name}
                    {model.provider ? ` · ${model.provider}` : ''}
                  </option>
                ))}
              </select>
              <button
                type="button"
                onClick={() => void modelSelector.refresh()}
                disabled={modelSelector.loading}
              >
                刷新
              </button>
            </div>
            <div
              className={`app-dev-chat-panel__model-capability ${
                selectedModelSupportsPrototypeImages
                  ? 'is-supported'
                  : 'is-limited'
              }`}
            >
              <span>原型图理解</span>
              <strong>
                {selectedModelSupportsPrototypeImages ? '已启用' : '受限'}
              </strong>
              <em>{prototypeImageCapabilityHint}</em>
            </div>
            <div className="app-dev-chat-panel__datasource-row">
              <div>
                <span>
                  绑定数据源（已选 {dataSourceSelector.selectedIds.length}/8）
                </span>
                <div>
                  {selectedDataSources.length ? (
                    <button
                      type="button"
                      onClick={() => {
                        selectedDataSources.forEach(dataSource =>
                          dataSourceSelector.toggleDataSource(dataSource.id),
                        );
                      }}
                      disabled={dataSourceSelector.loading}
                    >
                      清空
                    </button>
                  ) : null}
                  <button
                    type="button"
                    onClick={() => void dataSourceSelector.refresh()}
                    disabled={dataSourceSelector.loading}
                  >
                    刷新
                  </button>
                </div>
              </div>
              <label className="app-dev-chat-panel__datasource-search">
                <span>搜索数据源</span>
                <input
                  value={dataSourceKeyword}
                  placeholder="按名称、类型或描述搜索"
                  onChange={event => setDataSourceKeyword(event.target.value)}
                />
                {dataSourceKeyword.trim() ? (
                  <button
                    type="button"
                    onClick={() => setDataSourceKeyword('')}
                  >
                    清空
                  </button>
                ) : null}
              </label>
              {dataSourceSelector.loading ? (
                <p>正在加载数据源...</p>
              ) : dataSourceSelector.dataSources.length ? (
                <div className="app-dev-chat-panel__datasource-list">
                  {filteredDataSources.map(dataSource => (
                    <label key={dataSource.id}>
                      <input
                        type="checkbox"
                        checked={dataSourceSelector.selectedIds.includes(
                          dataSource.id,
                        )}
                        disabled={
                          !dataSourceSelector.selectedIds.includes(
                            dataSource.id,
                          ) && dataSourceSelector.selectedIds.length >= 8
                        }
                        onChange={() =>
                          dataSourceSelector.toggleDataSource(dataSource.id)
                        }
                      />
                      <span>
                        <strong>{dataSource.name}</strong>
                        <em>{dataSource.type}</em>
                      </span>
                    </label>
                  ))}
                  {!filteredDataSources.length ? (
                    <p>未找到匹配的数据源</p>
                  ) : null}
                </div>
              ) : (
                <p>暂无可绑定数据源</p>
              )}
              {selectedDataSources.length ? (
                <div className="app-dev-chat-panel__context-tags">
                  {selectedDataSources.map(dataSource => (
                    <em key={dataSource.id}>{dataSource.name}</em>
                  ))}
                </div>
              ) : null}
            </div>
            <p className="app-dev-chat-panel__privacy-note">
              生成时会把当前页面代码和选中的数据源名称发送给所选模型，不会发送数据源正文、密钥或环境变量。
            </p>
          </section>
        ) : null}
        {designOpen ? (
          <section className="app-dev-chat-panel__design-panel">
            <header>
              <div>
                <span>Design mode</span>
                <strong>把设计意图转成可执行开发指令</strong>
              </div>
              <p>
                选择目标区域和风格后，会生成结构化需求到输入框，由 AI
                助手继续修改文件并刷新预览。
              </p>
            </header>
            <div className="app-dev-chat-panel__design-grid">
              <label>
                <span>目标区域</span>
                <select
                  value={designTarget}
                  onChange={event => updateDesignTarget(event.target.value)}
                >
                  {APP_DEV_DESIGN_TARGETS.map(option => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span>视觉风格</span>
                <select
                  value={designPreset}
                  onChange={event => setDesignPreset(event.target.value)}
                >
                  {DESIGN_PRESETS.map(option => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span>色彩方向</span>
                <select
                  value={designColor}
                  onChange={event => setDesignColor(event.target.value)}
                >
                  {DESIGN_COLORS.map(option => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                <span>动效要求</span>
                <select
                  value={designMotion}
                  onChange={event => setDesignMotion(event.target.value)}
                >
                  {DESIGN_MOTION.map(option => (
                    <option key={option} value={option}>
                      {option}
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <section className="app-dev-chat-panel__design-properties">
              <header>
                <span>属性面板</span>
                <strong>细化本次设计修改的可视化参数</strong>
              </header>
              <div className="app-dev-chat-panel__design-grid">
                <label>
                  <span>布局密度</span>
                  <select
                    value={designLayout}
                    onChange={event => setDesignLayout(event.target.value)}
                  >
                    {DESIGN_LAYOUT.map(option => (
                      <option key={option} value={option}>
                        {option}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  <span>圆角</span>
                  <select
                    value={designRadius}
                    onChange={event => setDesignRadius(event.target.value)}
                  >
                    {DESIGN_RADIUS.map(option => (
                      <option key={option} value={option}>
                        {option}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  <span>阴影层级</span>
                  <select
                    value={designShadow}
                    onChange={event => setDesignShadow(event.target.value)}
                  >
                    {DESIGN_SHADOW.map(option => (
                      <option key={option} value={option}>
                        {option}
                      </option>
                    ))}
                  </select>
                </label>
                <label>
                  <span>字体层级</span>
                  <select
                    value={designTypography}
                    onChange={event => setDesignTypography(event.target.value)}
                  >
                    {DESIGN_TYPOGRAPHY.map(option => (
                      <option key={option} value={option}>
                        {option}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
            </section>
            <label className="app-dev-chat-panel__design-note">
              <span>补充说明</span>
              <textarea
                value={designNote}
                maxLength={1000}
                placeholder="例如：按钮更醒目，减少大面积留白，移动端首屏要先看到核心 CTA..."
                onChange={event => setDesignNote(event.target.value)}
              />
            </label>
            {designSelection ? (
              <div className="app-dev-chat-panel__design-selection">
                <span>预览选区</span>
                <strong>
                  {designSelection.target} · {designSelection.device} ·{' '}
                  {designSelection.xPercent}% / {designSelection.yPercent}%
                </strong>
                <p>
                  视口 {designSelection.viewportWidth}x
                  {designSelection.viewportHeight}，将随本次设计任务一起发送。
                </p>
              </div>
            ) : null}
            <div className="app-dev-chat-panel__design-actions">
              <button
                type="button"
                onClick={applyDesignPrompt}
                disabled={chat.running}
              >
                填入输入框
              </button>
              <button
                type="button"
                onClick={() => void executeDesignPrompt()}
                disabled={!canExecuteDesign}
              >
                {preparingDesignExecute ? '准备中...' : '直接执行'}
              </button>
              <span>
                可先填入输入框继续编辑，也可以直接交给 AI 应用开发助手执行。
              </span>
            </div>
            {designExecuteError ? (
              <p className="app-dev-chat-panel__design-execute-error">
                {designExecuteError}
              </p>
            ) : null}
          </section>
        ) : null}
        <textarea
          value={chat.input}
          maxLength={12000}
          placeholder="描述要新增或修改的页面，也可以粘贴截图..."
          onFocus={() => setComposerFocused(true)}
          onBlur={() => setComposerFocused(false)}
          onChange={event => chat.setInput(event.target.value)}
          onPaste={event => {
            const imageFiles = Array.from(event.clipboardData.items)
              .filter(item => item.type.startsWith('image/'))
              .map(item => item.getAsFile())
              .filter((file): file is File => Boolean(file));
            if (imageFiles.length) {
              event.preventDefault();
              void chat.addAttachmentFiles(imageFiles, {
                type: 'prototype_image',
              });
            }
          }}
          onKeyDown={event => {
            if (
              event.key === '@' &&
              !event.metaKey &&
              !event.ctrlKey &&
              !event.altKey
            ) {
              setMentionOpen(true);
              setContextOpen(false);
              setDesignOpen(false);
            }
            if (
              (event.metaKey || event.ctrlKey) &&
              event.key === 'Enter' &&
              canSend
            ) {
              void sendPrompt();
            }
          }}
        />
        {chat.attachments.length ? (
          <div className="app-dev-chat-panel__attachments">
            {chat.attachments.map(attachment => (
              <span key={attachment.id}>
                {attachment.type === 'prototype_image'
                  ? '原型图'
                  : attachment.type === 'image'
                    ? '图片'
                    : '文件'}{' '}
                · {attachment.name}
                <button
                  type="button"
                  onClick={() => chat.removeAttachment(attachment.id)}
                  disabled={chat.running}
                >
                  移除
                </button>
              </span>
            ))}
          </div>
        ) : null}
        <p className="app-dev-chat-panel__input-count">
          {chat.input.length}/12000
        </p>
        {referencedFiles.length || selectedDataSources.length ? (
          <div className="app-dev-chat-panel__mention-tags">
            {referencedFiles.map(file => (
              <span key={file.path} data-type={file.type}>
                {formatReferenceNodeType(file)} · {file.path}
                <button
                  type="button"
                  onClick={() => toggleReferencedFile(file)}
                  disabled={chat.running}
                >
                  移除
                </button>
              </span>
            ))}
            {selectedDataSources.map(dataSource => (
              <span key={dataSource.id} data-type="datasource">
                数据源 · {dataSource.name}
                <button
                  type="button"
                  onClick={() =>
                    dataSourceSelector.toggleDataSource(dataSource.id)
                  }
                  disabled={chat.running}
                >
                  移除
                </button>
              </span>
            ))}
          </div>
        ) : null}
        <div className="app-dev-chat-panel__actions">
          <span className="app-dev-chat-panel__composer-hint">
            {chat.running
              ? 'AI 正在修改项目文件，可以随时取消本次任务。'
              : hasPrototypeImageAttachment &&
                  !selectedModelSupportsPrototypeImages
                ? '已添加原型图；当前模型可能无法直接读取图片，请在需求中补充关键布局和文案。'
                : chat.attachments.length
                  ? `已添加 ${chat.attachments.length} 个附件/原型图，将随需求一起发送。`
                  : '支持粘贴截图、上传原型图、素材或设计文档。'}
          </span>
          <label className="app-dev-chat-panel__attach-button">
            {chat.uploadingAttachments ? '上传中...' : '添加附件'}
            <input
              type="file"
              multiple
              disabled={chat.running || chat.uploadingAttachments}
              onChange={event => {
                const files = Array.from(event.target.files || []);
                if (files.length) {
                  void chat.addAttachmentFiles(files);
                }
                event.target.value = '';
              }}
            />
          </label>
          <label className="app-dev-chat-panel__attach-button app-dev-chat-panel__prototype-button">
            原型图
            <span>
              {selectedModelSupportsPrototypeImages ? '可读' : '受限'}
            </span>
            <input
              type="file"
              multiple
              accept="image/*"
              disabled={chat.running || chat.uploadingAttachments}
              title={prototypeImageCapabilityHint}
              onChange={event => {
                const files = Array.from(event.target.files || []);
                if (files.length) {
                  void chat.addAttachmentFiles(files, {
                    type: 'prototype_image',
                  });
                }
                event.target.value = '';
              }}
            />
          </label>
          {chat.running ? (
            <button type="button" onClick={() => void chat.cancel()}>
              取消
            </button>
          ) : null}
          <button
            type="button"
            onClick={() => void sendPrompt()}
            disabled={!canSend}
          >
            {chat.running ? '执行中' : '发送'}
          </button>
        </div>
      </footer>
    </div>
  );
};
