/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/* eslint-disable @coze-arch/no-batch-import-or-export -- Generated workspace model values and types are consumed as one contract namespace. */

/* eslint-disable max-lines, max-lines-per-function, complexity -- Cohesive workspace model workflow. */
/* eslint-disable @coze-arch/max-line-per-function -- Form, filters and list belong to one settings surface. */

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import * as workbenchModel from '@coze-studio/api-schema/workbench-model';
import { useUserInfo } from '@coze-arch/foundation-sdk';
import {
  IconCozMagnifier,
  IconCozPlus,
  IconCozRefresh,
} from '@coze-arch/coze-design/icons';
import { Button, Input, Modal, Spin, Switch } from '@coze-arch/coze-design';

import styles from './workspace-model-settings-panel.module.less';

export const WORKSPACE_MODEL_SETTINGS_TAB_ID = 'workspace-models';

export interface WorkspaceModelSettingsLaunchIntent {
  key: number;
  type: 'create' | 'edit' | 'delete';
  modelId?: string;
}

interface WorkspaceModelSettingsPanelProps {
  launchIntent?: WorkspaceModelSettingsLaunchIntent;
  onModelsChanged?: () => void | Promise<void>;
  spaceId?: string;
}

type CreatorFilter = 'all' | 'me';

interface ModelEndpointFormState {
  id?: string;
  baseURL: string;
  apiKey: string;
  credentialConfigured: boolean;
  weight: string;
  enabled: boolean;
}

interface ModelFormState {
  providerKey: string;
  displayName: string;
  modelIdentifier: string;
  description: string;
  protocol: string;
  endpoints: ModelEndpointFormState[];
  capabilities: string[];
  usageScenarios: string[];
  maxContextTokens: string;
  maxOutputTokens: string;
  functionCallMode: string;
  enabled: boolean;
}

const CAPABILITY_OPTIONS = [
  { label: '文本生成', value: 'text' },
  { label: '图像理解', value: 'image' },
  { label: '语音理解', value: 'audio' },
  { label: '视频理解', value: 'video' },
  { label: '深度思考', value: 'reasoning' },
  { label: '文本向量', value: 'embedding' },
  { label: '多模态向量', value: 'multimodal_embedding' },
];

const USAGE_OPTIONS = [
  { label: '网页应用', value: 'appdev' },
  { label: '通用智能体', value: 'agent' },
  { label: '问答智能体', value: 'chat' },
  { label: '工作流', value: 'workflow' },
  { label: '外部 API 调用', value: 'api' },
];

const GENERATION_CAPABILITIES = new Set(['text', 'image', 'audio', 'video']);
const EMBEDDING_CAPABILITIES = new Set(['embedding', 'multimodal_embedding']);

const PROVIDER_DEFAULTS: Record<string, { baseURL: string; protocol: string }> =
  {
    claude: {
      baseURL: 'https://api.anthropic.com/v1',
      protocol: 'anthropic',
    },
    deepseek: {
      baseURL: 'https://api.deepseek.com/v1',
      protocol: 'openai-compatible',
    },
    gemini: {
      baseURL: 'https://generativelanguage.googleapis.com/v1beta/openai',
      protocol: 'gemini',
    },
    ollama: {
      baseURL: 'http://127.0.0.1:11434/v1',
      protocol: 'ollama',
    },
    openai: {
      baseURL: 'https://api.openai.com/v1',
      protocol: 'openai-compatible',
    },
    qwen: {
      baseURL: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      protocol: 'openai-compatible',
    },
  };

const createEndpoint = (baseURL = ''): ModelEndpointFormState => ({
  apiKey: '',
  baseURL,
  credentialConfigured: false,
  enabled: true,
  weight: '1',
});

const createEmptyForm = (providerKey = ''): ModelFormState => {
  const providerDefault = PROVIDER_DEFAULTS[providerKey];
  return {
    providerKey,
    displayName: '',
    modelIdentifier: '',
    description: '',
    protocol: providerDefault?.protocol ?? 'openai-compatible',
    endpoints: [createEndpoint(providerDefault?.baseURL)],
    capabilities: ['text'],
    usageScenarios: USAGE_OPTIONS.map(item => item.value),
    maxContextTokens: '128000',
    maxOutputTokens: '4096',
    functionCallMode: 'native',
    enabled: true,
  };
};

const ensureSuccessfulResponse = (response: {
  code?: number;
  msg?: string;
}) => {
  if (response.code && response.code !== 0) {
    throw new Error(response.msg || '请求失败');
  }
};

const positiveNumber = (value: string, fallback = 1) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
};

const formatUpdatedAt = (timestamp?: number) => {
  if (!timestamp) {
    return '更新时间未知';
  }
  return `更新于 ${new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp))}`;
};

const safeEndpointHost = (baseURL?: string) => {
  if (!baseURL) {
    return '未配置服务地址';
  }
  try {
    return new URL(baseURL).host || baseURL;
  } catch {
    return baseURL;
  }
};

const formFromModel = (
  model: workbenchModel.WorkspaceModel,
): ModelFormState => ({
  providerKey: model.provider_key,
  displayName: model.display_name,
  modelIdentifier: model.model_identifier,
  description: model.description ?? '',
  protocol: model.protocol || 'openai-compatible',
  endpoints: model.endpoints?.map(endpoint => ({
    apiKey: '',
    baseURL: endpoint.base_url,
    credentialConfigured: endpoint.credential_configured,
    enabled: endpoint.enabled,
    id: endpoint.id,
    weight: String(endpoint.weight || 1),
  })) ?? [createEndpoint()],
  capabilities:
    model.capabilities && model.capabilities.length > 0
      ? model.capabilities
      : ['text'],
  usageScenarios:
    model.usage_scenarios && model.usage_scenarios.length > 0
      ? model.usage_scenarios
      : USAGE_OPTIONS.map(item => item.value),
  maxContextTokens: String(model.max_context_tokens || 128000),
  maxOutputTokens: String(model.max_output_tokens || 4096),
  functionCallMode: model.function_call_mode || 'native',
  enabled: model.enabled,
});

export const WorkspaceModelSettingsPanel = ({
  launchIntent,
  onModelsChanged,
  spaceId,
}: WorkspaceModelSettingsPanelProps) => {
  const userInfo = useUserInfo();
  const requestSequence = useRef(0);
  const handledLaunchIntent = useRef<number>();
  const [models, setModels] = useState<workbenchModel.WorkspaceModel[]>([]);
  const [providers, setProviders] = useState<
    workbenchModel.WorkspaceModelProviderOption[]
  >([]);
  const [canManage, setCanManage] = useState(false);
  const [creatorFilter, setCreatorFilter] = useState<CreatorFilter>('all');
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [busyAction, setBusyAction] = useState('');
  const [formVisible, setFormVisible] = useState(false);
  const [editingModel, setEditingModel] =
    useState<workbenchModel.WorkspaceModel>();
  const [form, setForm] = useState<ModelFormState>(() => createEmptyForm());
  const [formError, setFormError] = useState('');
  const [testMessage, setTestMessage] = useState('');
  const [deleteCandidate, setDeleteCandidate] =
    useState<workbenchModel.WorkspaceModel>();

  const loadModels = useCallback(async () => {
    const requestID = ++requestSequence.current;
    if (!spaceId) {
      setModels([]);
      setProviders([]);
      setCanManage(false);
      setError('请先选择工作空间');
      return;
    }
    setLoading(true);
    setError('');
    try {
      const response = await workbenchModel.ListWorkspaceModels({
        space_id: spaceId,
        scope: workbenchModel.WorkspaceModelScope.Space,
      });
      ensureSuccessfulResponse(response);
      if (requestID !== requestSequence.current) {
        return;
      }
      setModels(response.data?.workspace_models ?? []);
      setProviders(response.data?.providers ?? []);
      setCanManage(response.data?.can_manage ?? false);
    } catch (loadError) {
      if (requestID === requestSequence.current) {
        setError(
          loadError instanceof Error ? loadError.message : '加载模型失败',
        );
      }
    } finally {
      if (requestID === requestSequence.current) {
        setLoading(false);
      }
    }
  }, [spaceId]);

  useEffect(() => {
    void loadModels();
    return () => {
      requestSequence.current += 1;
    };
  }, [loadModels]);

  const visibleModels = useMemo(() => {
    const normalizedKeyword = keyword.trim().toLowerCase();
    const currentUserID = userInfo?.user_id_str;
    return models.filter(model => {
      if (
        creatorFilter === 'me' &&
        String(model.creator_id ?? '') !== String(currentUserID ?? '')
      ) {
        return false;
      }
      return (
        !normalizedKeyword ||
        [
          model.display_name,
          model.model_identifier,
          model.provider_key,
          model.description,
          model.endpoints?.[0]?.base_url,
        ]
          .filter(Boolean)
          .join(' ')
          .toLowerCase()
          .includes(normalizedKeyword)
      );
    });
  }, [creatorFilter, keyword, models, userInfo?.user_id_str]);

  const selectedProvider = useMemo(
    () => providers.find(provider => provider.key === form.providerKey),
    [form.providerKey, providers],
  );
  const hasGenerationCapability = form.capabilities.some(capability =>
    GENERATION_CAPABILITIES.has(capability),
  );
  const hasEmbeddingCapability = form.capabilities.some(capability =>
    EMBEDDING_CAPABILITIES.has(capability),
  );

  const openCreateForm = () => {
    const preferredProvider =
      providers.find(provider => provider.key === 'deepseek') ?? providers[0];
    setEditingModel(undefined);
    setForm(createEmptyForm(preferredProvider?.key ?? ''));
    setFormError('');
    setTestMessage('');
    setFormVisible(true);
  };

  const openEditForm = (model: workbenchModel.WorkspaceModel) => {
    setEditingModel(model);
    setForm(formFromModel(model));
    setFormError('');
    setTestMessage('');
    setFormVisible(true);
  };

  const closeForm = () => {
    if (busyAction) {
      return;
    }
    setFormVisible(false);
    setEditingModel(undefined);
    setFormError('');
    setTestMessage('');
  };

  useEffect(() => {
    if (
      !launchIntent ||
      handledLaunchIntent.current === launchIntent.key ||
      loading ||
      !canManage
    ) {
      return;
    }

    if (launchIntent.type === 'create') {
      const preferredProvider =
        providers.find(provider => provider.key === 'deepseek') ?? providers[0];
      setEditingModel(undefined);
      setForm(createEmptyForm(preferredProvider?.key ?? ''));
      setFormError('');
      setTestMessage('');
      setFormVisible(true);
      handledLaunchIntent.current = launchIntent.key;
      return;
    }

    const targetModel = models.find(model => model.id === launchIntent.modelId);
    if (!targetModel || !targetModel.can_manage) {
      return;
    }

    if (launchIntent.type === 'edit') {
      setEditingModel(targetModel);
      setForm(formFromModel(targetModel));
      setFormError('');
      setTestMessage('');
      setFormVisible(true);
    } else {
      setDeleteCandidate(targetModel);
    }
    handledLaunchIntent.current = launchIntent.key;
  }, [canManage, launchIntent, loading, models, providers]);

  const updateForm = <K extends keyof ModelFormState>(
    field: K,
    value: ModelFormState[K],
  ) => setForm(current => ({ ...current, [field]: value }));

  const toggleSelection = (
    field: 'capabilities' | 'usageScenarios',
    value: string,
  ) =>
    setForm(current => ({
      ...current,
      [field]: current[field].includes(value)
        ? current[field].filter(item => item !== value)
        : [...current[field], value],
    }));

  const updateEndpoint = (
    index: number,
    patch: Partial<ModelEndpointFormState>,
  ) =>
    setForm(current => ({
      ...current,
      endpoints: current.endpoints.map((endpoint, endpointIndex) =>
        endpointIndex === index ? { ...endpoint, ...patch } : endpoint,
      ),
    }));

  const updateProvider = (providerKey: string) => {
    const previousDefault = PROVIDER_DEFAULTS[form.providerKey]?.baseURL;
    const providerDefault = PROVIDER_DEFAULTS[providerKey];
    setForm(current => ({
      ...current,
      providerKey,
      protocol: providerDefault?.protocol ?? current.protocol,
      endpoints: current.endpoints.map((endpoint, index) =>
        index === 0 &&
        (!endpoint.baseURL || endpoint.baseURL === previousDefault)
          ? { ...endpoint, baseURL: providerDefault?.baseURL ?? '' }
          : endpoint,
      ),
    }));
  };

  const buildDraft = (): workbenchModel.WorkspaceModelDraft => {
    const modelClass =
      selectedProvider?.model_class ?? editingModel?.model_class;
    if (!modelClass) {
      throw new Error('请选择模型供应商');
    }
    if (!form.displayName.trim() || !form.modelIdentifier.trim()) {
      throw new Error('请填写模型名称和模型标识');
    }
    if (!form.description.trim()) {
      throw new Error('请填写模型介绍');
    }
    if (form.capabilities.length === 0) {
      throw new Error('请至少选择一种模型类型');
    }
    if (form.endpoints.length === 0) {
      throw new Error('请至少配置一个接口地址');
    }
    for (const endpoint of form.endpoints) {
      if (!endpoint.baseURL.trim()) {
        throw new Error('请填写完整的接口地址');
      }
      if (
        !endpoint.apiKey.trim() &&
        !endpoint.credentialConfigured &&
        form.providerKey !== 'ollama'
      ) {
        throw new Error('请填写 API Key');
      }
    }
    const maxContextTokens = positiveNumber(form.maxContextTokens, 128000);
    const maxOutputTokens = positiveNumber(form.maxOutputTokens, 4096);
    if (hasGenerationCapability && maxOutputTokens > maxContextTokens) {
      throw new Error('最大输出 Tokens 不能超过最大上下文 Tokens');
    }
    return {
      provider_key: form.providerKey,
      model_class: modelClass,
      display_name: form.displayName.trim(),
      model_identifier: form.modelIdentifier.trim(),
      description: form.description.trim(),
      protocol: form.protocol.trim(),
      endpoints: form.endpoints.map(endpoint => ({
        id: endpoint.id,
        base_url: endpoint.baseURL.trim(),
        api_key: endpoint.apiKey.trim() || undefined,
        weight: positiveNumber(endpoint.weight),
        enabled: endpoint.enabled,
      })),
      capabilities: form.capabilities,
      usage_scenarios: hasEmbeddingCapability ? [] : form.usageScenarios,
      max_context_tokens: maxContextTokens,
      max_output_tokens: maxOutputTokens,
      function_call_mode: form.functionCallMode,
      enabled: form.enabled,
    };
  };

  const runMutation = async (
    actionKey: string,
    action: () => Promise<void>,
  ) => {
    if (busyAction) {
      return;
    }
    setBusyAction(actionKey);
    setError('');
    setNotice('');
    try {
      await action();
    } catch (mutationError) {
      const message =
        mutationError instanceof Error
          ? mutationError.message
          : '操作失败，请重试';
      setError(message);
      throw mutationError;
    } finally {
      setBusyAction('');
    }
  };

  const saveModel = async () => {
    if (!spaceId) {
      setFormError('缺少工作空间上下文');
      return;
    }
    let draft: workbenchModel.WorkspaceModelDraft;
    try {
      draft = buildDraft();
    } catch (validationError) {
      setFormError(
        validationError instanceof Error
          ? validationError.message
          : '请检查模型配置',
      );
      return;
    }
    setFormError('');
    try {
      await runMutation('save', async () => {
        const response = await workbenchModel.UpsertWorkspaceModel({
          space_id: spaceId,
          model_id: editingModel?.id,
          model: draft,
        });
        ensureSuccessfulResponse(response);
        setNotice(editingModel ? '模型已更新' : '模型已添加');
        setFormVisible(false);
        setEditingModel(undefined);
        await loadModels();
        await onModelsChanged?.();
      });
    } catch (saveError) {
      setFormError(
        saveError instanceof Error ? saveError.message : '保存模型失败',
      );
    }
  };

  const testDraft = async () => {
    if (!spaceId) {
      setFormError('缺少工作空间上下文');
      return;
    }
    let draft: workbenchModel.WorkspaceModelDraft;
    try {
      draft = buildDraft();
    } catch (validationError) {
      setFormError(
        validationError instanceof Error
          ? validationError.message
          : '请检查模型配置',
      );
      return;
    }
    setFormError('');
    setTestMessage('');
    try {
      await runMutation('test-form', async () => {
        const response = await workbenchModel.TestWorkspaceModel({
          space_id: spaceId,
          model_id: editingModel?.id,
          model: draft,
        });
        ensureSuccessfulResponse(response);
        setTestMessage(
          response.data?.message ||
            (response.data?.success
              ? `连接成功，耗时 ${response.data.duration_ms} ms`
              : '连接失败'),
        );
      });
    } catch (testError) {
      setFormError(
        testError instanceof Error ? testError.message : '连接测试失败',
      );
    }
  };

  const toggleModel = (
    model: workbenchModel.WorkspaceModel,
    enabled: boolean,
  ) =>
    runMutation(`toggle-${model.id}`, async () => {
      if (!spaceId) {
        throw new Error('缺少工作空间上下文');
      }
      const response = await workbenchModel.SetWorkspaceModelStatus({
        model_id: model.id,
        space_id: spaceId,
        enabled,
      });
      ensureSuccessfulResponse(response);
      setNotice(enabled ? '模型已启用' : '模型已停用');
      await loadModels();
      await onModelsChanged?.();
    }).catch(() => undefined);

  const deleteModel = async () => {
    if (!spaceId || !deleteCandidate) {
      return;
    }
    const candidate = deleteCandidate;
    try {
      await runMutation(`delete-${candidate.id}`, async () => {
        const response = await workbenchModel.DeleteWorkspaceModel({
          model_id: candidate.id,
          space_id: spaceId,
        });
        ensureSuccessfulResponse(response);
        setDeleteCandidate(undefined);
        setNotice('模型已删除');
        await loadModels();
        await onModelsChanged?.();
      });
    } catch (caughtError) {
      void caughtError;
      // The shared error banner already contains the safe failure message.
    }
  };

  const renderModel = (model: workbenchModel.WorkspaceModel) => {
    const endpoint = model.endpoints?.[0];
    const canEditModel = canManage && model.can_manage;
    const createdByMe =
      String(model.creator_id ?? '') === String(userInfo?.user_id_str ?? '');
    return (
      <article className={styles.modelCard} key={model.id}>
        <div className={styles.cardHeader}>
          <span className={styles.providerIcon} aria-hidden="true">
            {(model.provider_key || model.display_name)
              .slice(0, 1)
              .toUpperCase()}
          </span>
          <div>
            <h3>{model.display_name}</h3>
            <p>
              {createdByMe ? '由我创建' : `创建者 ${model.creator_id ?? '-'}`}
            </p>
          </div>
          <span
            className={styles.statusPill}
            data-status={model.enabled ? 'enabled' : 'disabled'}
          >
            {model.enabled ? '已启用' : '已停用'}
          </span>
        </div>
        <p className={styles.modelDescription}>
          {model.description || '暂无模型介绍'}
        </p>
        <div className={styles.modelMeta}>
          <span>{model.model_identifier}</span>
          <span>{safeEndpointHost(endpoint?.base_url)}</span>
        </div>
        <footer className={styles.cardFooter}>
          <span>{formatUpdatedAt(model.updated_at)}</span>
          {canEditModel ? (
            <div>
              <Switch
                size="small"
                checked={model.enabled}
                loading={busyAction === `toggle-${model.id}`}
                disabled={Boolean(busyAction)}
                aria-label={`${model.enabled ? '停用' : '启用'} ${
                  model.display_name
                }`}
                onChange={checked => void toggleModel(model, checked)}
              />
              <button type="button" onClick={() => openEditForm(model)}>
                编辑
              </button>
              <button type="button" onClick={() => setDeleteCandidate(model)}>
                删除
              </button>
            </div>
          ) : (
            <span>只读</span>
          )}
        </footer>
      </article>
    );
  };

  return (
    <section className={styles.panel}>
      <header className={styles.header}>
        <div className={styles.headerLeft}>
          <h2>模型管理</h2>
          <div
            className={styles.creatorTabs}
            role="tablist"
            aria-label="创建者"
          >
            <button
              type="button"
              role="tab"
              aria-selected={creatorFilter === 'all'}
              onClick={() => setCreatorFilter('all')}
            >
              所有人
            </button>
            <button
              type="button"
              role="tab"
              aria-selected={creatorFilter === 'me'}
              onClick={() => setCreatorFilter('me')}
            >
              由我创建
            </button>
          </div>
        </div>
        <div className={styles.headerRight}>
          <Input
            prefix={<IconCozMagnifier />}
            value={keyword}
            placeholder="搜索模型"
            aria-label="搜索模型"
            onChange={setKeyword}
          />
          <Button
            color="secondary"
            icon={<IconCozRefresh />}
            loading={loading}
            onClick={() => void loadModels()}
          >
            刷新
          </Button>
          {canManage ? (
            <Button
              type="primary"
              icon={<IconCozPlus />}
              onClick={openCreateForm}
            >
              模型
            </Button>
          ) : null}
        </div>
      </header>

      {!canManage && !loading ? (
        <div className={styles.readonlyBanner}>
          当前账号可查看空间模型，仅工作空间所有者或管理员可维护。
        </div>
      ) : null}
      {error ? (
        <div className={styles.feedback} data-tone="error" role="alert">
          <span>{error}</span>
          <button type="button" onClick={() => void loadModels()}>
            重试
          </button>
        </div>
      ) : null}
      {notice ? (
        <div className={styles.feedback} data-tone="success" role="status">
          {notice}
        </div>
      ) : null}

      <Spin spinning={loading}>
        {!loading && visibleModels.length === 0 ? (
          <div className={styles.emptyState}>
            <h3>未能找到相关结果</h3>
            <p>
              {keyword || creatorFilter === 'me'
                ? '调整创建者或搜索条件后再试。'
                : '当前空间还没有模型资源。'}
            </p>
            {!keyword && creatorFilter === 'all' && canManage ? (
              <Button type="primary" onClick={openCreateForm}>
                新增模型
              </Button>
            ) : null}
          </div>
        ) : (
          <div className={styles.modelGrid}>
            {visibleModels.map(renderModel)}
          </div>
        )}
      </Spin>

      <Modal
        title={editingModel ? '编辑模型' : '新增模型'}
        visible={formVisible}
        width={720}
        okText="确认"
        cancelText="取消"
        confirmLoading={busyAction === 'save'}
        maskClosable={false}
        onCancel={closeForm}
        onOk={() => void saveModel()}
      >
        <div className={styles.formBody}>
          <div className={styles.formGrid}>
            <label className={styles.fullWidth}>
              <span>供应商 *</span>
              <select
                value={form.providerKey}
                disabled={Boolean(editingModel)}
                onChange={event => updateProvider(event.target.value)}
              >
                <option value="" disabled>
                  请选择供应商
                </option>
                {providers.map(provider => (
                  <option value={provider.key} key={provider.key}>
                    {provider.name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              <span>模型名称 *</span>
              <Input
                value={form.displayName}
                maxLength={128}
                placeholder="输入模型名称"
                onChange={value => updateForm('displayName', value)}
              />
            </label>
            <label>
              <span>模型标识 *</span>
              <Input
                value={form.modelIdentifier}
                maxLength={256}
                placeholder="输入供应商下的模型标识"
                onChange={value => updateForm('modelIdentifier', value)}
              />
            </label>
            <label className={styles.fullWidth}>
              <span>模型介绍 *</span>
              <textarea
                value={form.description}
                maxLength={1000}
                placeholder="输入模型介绍"
                onChange={event =>
                  updateForm('description', event.target.value)
                }
              />
              <small>{form.description.length} / 1000</small>
            </label>
          </div>

          <section className={styles.formSection}>
            <h3>模型类型 *</h3>
            <div className={styles.optionGrid}>
              {CAPABILITY_OPTIONS.map(option => (
                <button
                  key={option.value}
                  type="button"
                  aria-pressed={form.capabilities.includes(option.value)}
                  onClick={() => toggleSelection('capabilities', option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </section>

          {hasGenerationCapability ? (
            <section className={styles.formSection}>
              <div className={styles.formGrid}>
                <label>
                  <span>最大输出 Tokens *</span>
                  <Input
                    value={form.maxOutputTokens}
                    placeholder="4096"
                    onChange={value => updateForm('maxOutputTokens', value)}
                  />
                </label>
                <label>
                  <span>最大上下文长度 *</span>
                  <Input
                    value={form.maxContextTokens}
                    placeholder="128000"
                    onChange={value => updateForm('maxContextTokens', value)}
                  />
                </label>
                <label className={styles.fullWidth}>
                  <span>函数调用支持 *</span>
                  <select
                    value={form.functionCallMode}
                    onChange={event =>
                      updateForm('functionCallMode', event.target.value)
                    }
                  >
                    <option value="none">不支持</option>
                    <option value="auto">自动检测</option>
                    <option value="native">支持流式函数调用</option>
                  </select>
                </label>
              </div>
            </section>
          ) : null}

          <section className={styles.formSection}>
            <div className={styles.switchRow}>
              <div>
                <strong>是否启用</strong>
                <span>停用后不会出现在当前空间的模型选择器中。</span>
              </div>
              <Switch
                checked={form.enabled}
                onChange={checked => updateForm('enabled', checked)}
              />
            </div>
          </section>

          {!hasEmbeddingCapability ? (
            <section className={styles.formSection}>
              <h3>可用范围</h3>
              <div className={styles.optionGrid}>
                {USAGE_OPTIONS.map(option => (
                  <button
                    key={option.value}
                    type="button"
                    aria-pressed={form.usageScenarios.includes(option.value)}
                    onClick={() =>
                      toggleSelection('usageScenarios', option.value)
                    }
                  >
                    {option.label}
                  </button>
                ))}
              </div>
            </section>
          ) : null}

          <section className={styles.formSection}>
            <div className={styles.formGrid}>
              <label className={styles.fullWidth}>
                <span>接口协议 *</span>
                <select
                  value={form.protocol}
                  onChange={event => updateForm('protocol', event.target.value)}
                >
                  <option value="openai-compatible">OpenAI Compatible</option>
                  <option value="anthropic">Anthropic</option>
                  <option value="gemini">Gemini</option>
                  <option value="ollama">Ollama</option>
                </select>
              </label>
            </div>
          </section>

          <section className={styles.formSection}>
            <div className={styles.sectionTitle}>
              <div>
                <h3>接口配置 *</h3>
                <p>API Key 加密保存，保存后不会再次回显。</p>
              </div>
              <Button
                color="secondary"
                size="small"
                onClick={() =>
                  updateForm('endpoints', [...form.endpoints, createEndpoint()])
                }
              >
                新增接口
              </Button>
            </div>
            <div className={styles.endpointList}>
              {form.endpoints.map((endpoint, index) => (
                <article key={`${endpoint.id ?? 'new'}-${index}`}>
                  <header>
                    <strong>Endpoint {index + 1}</strong>
                    {form.endpoints.length > 1 ? (
                      <button
                        type="button"
                        onClick={() =>
                          updateForm(
                            'endpoints',
                            form.endpoints.filter(
                              (_, endpointIndex) => endpointIndex !== index,
                            ),
                          )
                        }
                      >
                        删除
                      </button>
                    ) : null}
                  </header>
                  <div className={styles.formGrid}>
                    <label className={styles.fullWidth}>
                      <span>URL *</span>
                      <Input
                        value={endpoint.baseURL}
                        maxLength={2048}
                        placeholder="https://api.example.com/v1"
                        onChange={value =>
                          updateEndpoint(index, { baseURL: value })
                        }
                      />
                    </label>
                    <label>
                      <span>API Key *</span>
                      <Input
                        mode="password"
                        name={
                          index === 0
                            ? 'workspace-model-api-key'
                            : `workspace-model-api-key-${index}`
                        }
                        autoComplete="new-password"
                        value={endpoint.apiKey}
                        maxLength={8192}
                        placeholder={
                          endpoint.credentialConfigured
                            ? '已配置，留空则保持不变'
                            : '请输入 API Key'
                        }
                        onChange={value =>
                          updateEndpoint(index, { apiKey: value })
                        }
                      />
                    </label>
                    <label>
                      <span>权重 *</span>
                      <Input
                        value={endpoint.weight}
                        placeholder="1"
                        onChange={value =>
                          updateEndpoint(index, { weight: value })
                        }
                      />
                    </label>
                  </div>
                  <div className={styles.endpointSwitch}>
                    <span>启用该接口</span>
                    <Switch
                      size="small"
                      checked={endpoint.enabled}
                      onChange={checked =>
                        updateEndpoint(index, { enabled: checked })
                      }
                    />
                  </div>
                </article>
              ))}
            </div>
          </section>

          <div className={styles.formActions}>
            <Button
              color="secondary"
              loading={busyAction === 'test-form'}
              disabled={Boolean(busyAction) && busyAction !== 'test-form'}
              onClick={() => void testDraft()}
            >
              模型连通性测试
            </Button>
          </div>
          {formError ? (
            <div className={styles.formMessage} data-tone="error" role="alert">
              {formError}
            </div>
          ) : null}
          {testMessage ? (
            <div
              className={styles.formMessage}
              data-tone="success"
              role="status"
            >
              {testMessage}
            </div>
          ) : null}
        </div>
      </Modal>

      <Modal
        title="删除模型"
        visible={Boolean(deleteCandidate)}
        okText="确认删除"
        cancelText="取消"
        confirmLoading={busyAction === `delete-${deleteCandidate?.id ?? ''}`}
        onCancel={() => setDeleteCandidate(undefined)}
        onOk={() => void deleteModel()}
      >
        <p className={styles.deleteWarning}>
          删除「{deleteCandidate?.display_name}
          」后，它会立即从当前空间的模型选择器中移除。此操作不可恢复。
        </p>
      </Modal>
    </section>
  );
};
