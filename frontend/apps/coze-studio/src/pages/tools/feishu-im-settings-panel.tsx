/*
 * Copyright 2025 coze-dev Authors
 * SPDX-License-Identifier: Apache-2.0
 */

/* eslint-disable max-lines, @coze-arch/max-line-per-function, max-lines-per-function -- Cohesive Feishu configuration flow. */

import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  Button,
  Input,
  Modal,
  Select,
  Spin,
  Switch,
} from '@coze-arch/coze-design';

import {
  type AgentTarget,
  type FeishuGroupPolicy,
  type FeishuIMConfig,
  type FeishuIMMutation,
  type FeishuReplyMode,
  createFeishuIMConfig,
  deleteFeishuIMConfig,
  listFeishuIMConfigs,
  listPublishedAgentTargets,
  setFeishuIMConfigEnabled,
  testFeishuIMConfig,
  updateFeishuIMConfig,
} from './feishu-im-service';

import styles from './feishu-im-settings-panel.module.less';

export const FEISHU_IM_SETTINGS_TAB_ID = 'feishu-im';

interface FeishuIMSettingsPanelProps {
  spaceId?: string;
}

interface FormState {
  name: string;
  appId: string;
  appSecret: string;
  agentId: string;
  enabled: boolean;
  replyMode: FeishuReplyMode;
  groupPolicy: FeishuGroupPolicy;
}

const emptyForm = (): FormState => ({
  name: '',
  appId: '',
  appSecret: '',
  agentId: '',
  enabled: false,
  replyMode: 'stream',
  groupPolicy: 'mention_only',
});

const formFromConfig = (config: FeishuIMConfig): FormState => ({
  name: config.name,
  appId: config.app_id,
  appSecret: '',
  agentId: config.agent_id,
  enabled: config.enabled,
  replyMode: config.reply_mode,
  groupPolicy: config.group_policy,
});

const statusMeta: Record<
  FeishuIMConfig['runtime_status'],
  { label: string; className: string }
> = {
  disabled: { label: '已停用', className: '' },
  pending: { label: '等待连接', className: styles.statusPending },
  connecting: { label: '正在连接', className: styles.statusPending },
  connected: { label: '连接正常', className: styles.statusConnected },
  reconnecting: { label: '正在重连', className: styles.statusPending },
  error: { label: '连接异常', className: styles.statusError },
};

const formatTime = (timestamp?: number) => {
  if (!timestamp) {
    return '尚未检测';
  }
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp));
};

export const FeishuIMSettingsPanel = ({
  spaceId,
}: FeishuIMSettingsPanelProps) => {
  const [configs, setConfigs] = useState<FeishuIMConfig[]>([]);
  const [agents, setAgents] = useState<AgentTarget[]>([]);
  const [canManage, setCanManage] = useState(false);
  const [credentialReady, setCredentialReady] = useState(true);
  const [loading, setLoading] = useState(false);
  const [busyAction, setBusyAction] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState<FeishuIMConfig>();
  const [form, setForm] = useState<FormState>(emptyForm);

  const loadData = useCallback(async () => {
    if (!spaceId) {
      setConfigs([]);
      setAgents([]);
      setCanManage(false);
      return;
    }
    setLoading(true);
    setError('');
    try {
      const [listResult, agentTargets] = await Promise.all([
        listFeishuIMConfigs(spaceId),
        listPublishedAgentTargets(spaceId),
      ]);
      setConfigs(listResult.configs ?? []);
      setCanManage(listResult.can_manage);
      setCredentialReady(listResult.credential_ready);
      setAgents(agentTargets.filter(agent => agent.published));
    } catch (loadError) {
      setError(
        loadError instanceof Error ? loadError.message : '加载飞书机器人失败',
      );
    } finally {
      setLoading(false);
    }
  }, [spaceId]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  const agentOptions = useMemo(
    () =>
      agents.map(agent => ({
        label: agent.name || agent.id,
        value: agent.id,
      })),
    [agents],
  );

  const runAction = async (key: string, action: () => Promise<void>) => {
    if (busyAction) {
      return;
    }
    setBusyAction(key);
    setError('');
    setNotice('');
    try {
      await action();
    } catch (actionError) {
      setError(
        actionError instanceof Error ? actionError.message : '操作失败，请重试',
      );
    } finally {
      setBusyAction('');
    }
  };

  const openCreate = () => {
    setEditing(undefined);
    setForm(emptyForm());
    setError('');
    setModalVisible(true);
  };

  const openEdit = (config: FeishuIMConfig) => {
    setEditing(config);
    setForm(formFromConfig(config));
    setError('');
    setModalVisible(true);
  };

  const saveConfig = async () => {
    if (!spaceId) {
      return;
    }
    if (
      !form.name.trim() ||
      !form.appId.trim() ||
      !form.agentId ||
      (!editing && !form.appSecret.trim())
    ) {
      setError('请完整填写机器人名称、App ID、App Secret 和目标 Agent');
      return;
    }
    const payload: FeishuIMMutation = {
      space_id: spaceId,
      name: form.name.trim(),
      app_id: form.appId.trim(),
      app_secret: form.appSecret.trim(),
      agent_id: form.agentId,
      enabled: form.enabled,
      reply_mode: form.replyMode,
      group_policy: form.groupPolicy,
    };
    await runAction('save', async () => {
      if (editing) {
        await updateFeishuIMConfig(editing.id, payload);
      } else {
        await createFeishuIMConfig(payload);
      }
      setModalVisible(false);
      setNotice(editing ? '飞书机器人配置已更新' : '飞书机器人已创建');
      await loadData();
    });
  };

  const toggleConfig = (config: FeishuIMConfig, enabled: boolean) => {
    if (!spaceId) {
      return;
    }
    void runAction(`toggle:${config.id}`, async () => {
      await setFeishuIMConfigEnabled(config.id, spaceId, enabled);
      setNotice(enabled ? '机器人正在建立飞书长连接' : '机器人已停用');
      await loadData();
    });
  };

  const testConfig = (config: FeishuIMConfig) => {
    if (!spaceId) {
      return;
    }
    void runAction(`test:${config.id}`, async () => {
      const identity = await testFeishuIMConfig(config.id, spaceId);
      setNotice(
        identity.bot_name
          ? `连接成功：${identity.bot_name}`
          : '飞书应用连接成功',
      );
      await loadData();
    });
  };

  const confirmDelete = (config: FeishuIMConfig) => {
    if (!spaceId) {
      return;
    }
    Modal.confirm({
      title: '删除飞书机器人？',
      content: `删除后会立即断开“${config.name}”的长连接，已有 Coze 任务记录不会被删除。`,
      okText: '删除',
      cancelText: '取消',
      okButtonProps: { type: 'danger' },
      onOk: async () => {
        await runAction(`delete:${config.id}`, async () => {
          await deleteFeishuIMConfig(config.id, spaceId);
          setNotice('飞书机器人已删除');
          await loadData();
        });
      },
    });
  };

  const renderCard = (config: FeishuIMConfig) => {
    const runtime = statusMeta[config.runtime_status] ?? statusMeta.pending;
    return (
      <article className={styles.card} key={config.id}>
        <div className={styles.cardHeader}>
          <div className={styles.cardIdentity}>
            <span className={styles.logo} aria-hidden="true">
              F
            </span>
            <div className="min-w-0">
              <div className={styles.name}>{config.name}</div>
              <div className={styles.appId}>{config.app_id}</div>
            </div>
          </div>
          <Switch
            checked={config.enabled}
            disabled={!canManage || Boolean(busyAction)}
            loading={busyAction === `toggle:${config.id}`}
            onChange={checked => toggleConfig(config, checked)}
            aria-label={`${config.enabled ? '停用' : '启用'}${config.name}`}
          />
        </div>

        <div className={`${styles.status} ${runtime.className}`}>
          <span className={styles.statusDot} />
          {runtime.label}
          {config.bot_name ? ` · ${config.bot_name}` : ''}
        </div>

        <div className={styles.meta}>
          <div className={styles.metaItem}>
            <div className={styles.metaLabel}>目标 Agent</div>
            <div className={styles.metaValue}>
              {config.agent_name || config.agent_id}
            </div>
          </div>
          <div className={styles.metaItem}>
            <div className={styles.metaLabel}>回复策略</div>
            <div className={styles.metaValue}>
              {config.reply_mode === 'stream' ? '流式回复' : '完整回复'}
              {' · '}
              {config.group_policy === 'mention_only'
                ? '群聊需 @'
                : '不响应群聊'}
            </div>
          </div>
          <div className={styles.metaItem}>
            <div className={styles.metaLabel}>凭据</div>
            <div className={styles.metaValue}>
              {config.secret_configured ? '已安全保存' : '未配置'}
            </div>
          </div>
          <div className={styles.metaItem}>
            <div className={styles.metaLabel}>最近检测</div>
            <div className={styles.metaValue}>
              {formatTime(config.last_tested_at)}
            </div>
          </div>
        </div>

        {config.runtime_error ? (
          <div className={styles.runtimeError}>{config.runtime_error}</div>
        ) : null}

        <div className={styles.actions}>
          <Button
            size="small"
            disabled={!canManage || Boolean(busyAction)}
            loading={busyAction === `test:${config.id}`}
            onClick={() => testConfig(config)}
          >
            检测连接
          </Button>
          <Button
            size="small"
            disabled={!canManage || Boolean(busyAction)}
            onClick={() => openEdit(config)}
          >
            编辑
          </Button>
          <Button
            size="small"
            disabled={!canManage || Boolean(busyAction)}
            onClick={() => confirmDelete(config)}
          >
            删除
          </Button>
        </div>
      </article>
    );
  };

  return (
    <div className={styles.panel}>
      <div className={styles.hero}>
        <div>
          <div className={styles.eyebrow}>FEISHU CHANNEL</div>
          <h2 className={styles.title}>IM 机器人</h2>
          <p className={styles.description}>
            将当前工作空间内已发布的 Agent
            接入飞书。私聊直接响应，群聊默认仅在明确
            @机器人时响应，消息与任务记录会持续保存在 Coze。
          </p>
        </div>
        <Button
          color="primary"
          disabled={!canManage || !credentialReady}
          onClick={openCreate}
        >
          + 添加飞书机器人
        </Button>
      </div>

      {!credentialReady ? (
        <div className={styles.error}>
          服务端尚未配置 IM 凭据加密密钥。配置
          {' IM_CHANNEL_CREDENTIAL_KEYS_JSON '}与
          {' IM_CHANNEL_CREDENTIAL_ACTIVE_KEY_ID '}
          后才能保存 App Secret。
        </div>
      ) : null}
      {notice ? <div className={styles.notice}>{notice}</div> : null}
      {error ? <div className={styles.error}>{error}</div> : null}

      <div className={styles.content}>
        <Spin spinning={loading}>
          {configs.length ? (
            <div className={styles.grid}>{configs.map(renderCard)}</div>
          ) : (
            <div className={styles.empty}>
              <div>
                <div className={styles.emptyIcon}>F</div>
                <div className={styles.emptyTitle}>还没有飞书机器人</div>
                <div className={styles.emptyDescription}>
                  创建后，系统会使用飞书官方 Go SDK 建立长连接。无需暴露公网
                  Webhook， 也不会将 App Secret 返回到浏览器。
                </div>
                {canManage && credentialReady ? (
                  <Button color="primary" onClick={openCreate}>
                    创建第一个机器人
                  </Button>
                ) : null}
              </div>
            </div>
          )}
        </Spin>
      </div>

      <Modal
        visible={modalVisible}
        title={editing ? '编辑飞书机器人' : '添加飞书机器人'}
        width={620}
        okText={editing ? '保存修改' : '创建'}
        cancelText="取消"
        confirmLoading={busyAction === 'save'}
        onCancel={() => setModalVisible(false)}
        onOk={() => void saveConfig()}
      >
        <div className={styles.form}>
          <div className={styles.formGrid}>
            <label className={styles.field}>
              <span className={styles.fieldLabel}>机器人名称</span>
              <Input
                value={form.name}
                maxLength={80}
                placeholder="例如：飞书专属助理"
                onChange={value =>
                  setForm(current => ({ ...current, name: value }))
                }
              />
            </label>
            <label className={styles.field}>
              <span className={styles.fieldLabel}>目标 Agent</span>
              <Select
                value={form.agentId || undefined}
                optionList={agentOptions}
                placeholder="选择已发布的 Agent"
                style={{ width: '100%' }}
                onChange={value =>
                  setForm(current => ({
                    ...current,
                    agentId: String(value ?? ''),
                  }))
                }
              />
              {!agents.length ? (
                <span className={styles.fieldHint}>
                  当前工作空间暂无已发布 Agent，请先发布后再绑定。
                </span>
              ) : null}
            </label>
          </div>

          <div className={styles.formGrid}>
            <label className={styles.field}>
              <span className={styles.fieldLabel}>App ID</span>
              <Input
                name="feishu-app-id"
                autoComplete="off"
                value={form.appId}
                maxLength={128}
                placeholder="cli_xxxxxxxxxxxxxxxx"
                onChange={value =>
                  setForm(current => ({ ...current, appId: value }))
                }
              />
            </label>
            <label className={styles.field}>
              <span className={styles.fieldLabel}>App Secret</span>
              <Input
                mode="password"
                name="feishu-app-secret"
                autoComplete="new-password"
                value={form.appSecret}
                maxLength={512}
                placeholder={
                  editing?.secret_configured
                    ? '留空则保持现有密钥'
                    : '请输入 App Secret'
                }
                onChange={value =>
                  setForm(current => ({ ...current, appSecret: value }))
                }
              />
              <span className={styles.fieldHint}>
                仅写入，使用 AES-GCM 加密保存，页面不会回显。
              </span>
            </label>
          </div>

          <div className={styles.formGrid}>
            <label className={styles.field}>
              <span className={styles.fieldLabel}>回复方式</span>
              <Select
                value={form.replyMode}
                optionList={[
                  { label: '流式回复', value: 'stream' },
                  { label: '生成完成后回复', value: 'final' },
                ]}
                style={{ width: '100%' }}
                onChange={value =>
                  setForm(current => ({
                    ...current,
                    replyMode: value as FeishuReplyMode,
                  }))
                }
              />
            </label>
            <label className={styles.field}>
              <span className={styles.fieldLabel}>群聊策略</span>
              <Select
                value={form.groupPolicy}
                optionList={[
                  { label: '仅明确 @机器人时响应', value: 'mention_only' },
                  { label: '不响应群聊', value: 'disabled' },
                ]}
                style={{ width: '100%' }}
                onChange={value =>
                  setForm(current => ({
                    ...current,
                    groupPolicy: value as FeishuGroupPolicy,
                  }))
                }
              />
            </label>
          </div>

          <div className={styles.switchRow}>
            <div>
              <div className={styles.fieldLabel}>创建后立即启用</div>
              <div className={styles.fieldHint}>
                启用后服务端会建立飞书官方 WebSocket 长连接。
              </div>
            </div>
            <Switch
              checked={form.enabled}
              onChange={checked =>
                setForm(current => ({ ...current, enabled: checked }))
              }
            />
          </div>
        </div>
      </Modal>
    </div>
  );
};
