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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, max-lines -- Object storage admin form keeps provider rules close to the UI contract. */

import { useCallback, useEffect, useMemo, useState } from 'react';

import {
  IconCozCheckMark,
  IconCozEdit,
  IconCozPlus,
  IconCozRefresh,
  IconCozTrashCan,
} from '@coze-arch/coze-design/icons';

import {
  AdminAPIError,
  activateObjectStorageConfig,
  createObjectStorageConfig,
  deleteObjectStorageConfig,
  listObjectStorageConfigs,
  ObjectStorageHealthStatus,
  ObjectStorageProviderType,
  ObjectStorageRuntimeSource,
  testObjectStorageConfig,
  updateObjectStorageConfig,
  type ObjectStorageConfigView,
  type ObjectStoragePublicConfig,
} from './service';

import styles from './object-storage-section.module.less';

interface ProviderOption {
  value: ObjectStorageProviderType;
  label: string;
  hint: string;
}

interface ObjectStorageDraft {
  name: string;
  provider_type: ObjectStorageProviderType;
  bucket: string;
  region: string;
  endpoint: string;
  endpoint_override: string;
  force_path_style: boolean;
  use_ssl: boolean;
  download_domain: string;
  use_https: boolean;
  access_key_id: string;
  secret_access_key: string;
  replace_credential: boolean;
}

const PROVIDERS: ProviderOption[] = [
  {
    value: ObjectStorageProviderType.QINIU,
    label: '七牛云 Kodo',
    hint: '填写 Bucket、Region 和下载域名，AK/SK 提交后不会回显。',
  },
  {
    value: ObjectStorageProviderType.ALIYUN_OSS,
    label: '阿里云 OSS',
    hint: '推荐填写 Region；如使用专有 Endpoint 可额外填写 Endpoint。',
  },
  {
    value: ObjectStorageProviderType.TENCENT_COS,
    label: '腾讯云 COS',
    hint: '填写 Bucket 和 Region，Endpoint 可留空使用默认推导。',
  },
  {
    value: ObjectStorageProviderType.HUAWEI_OBS,
    label: '华为云 OBS',
    hint: '填写 Bucket、Region，必要时配置 Endpoint 覆盖。',
  },
  {
    value: ObjectStorageProviderType.AWS_S3,
    label: 'AWS S3',
    hint: '填写 Bucket、Region；兼容网关可开启 Path Style。',
  },
  {
    value: ObjectStorageProviderType.MINIO,
    label: 'MinIO',
    hint: '填写 Endpoint，私有部署通常开启 Path Style。',
  },
  {
    value: ObjectStorageProviderType.TOS,
    label: '火山引擎 TOS',
    hint: '填写 Bucket 和 Region，Endpoint 可用于私有域名覆盖。',
  },
];

const PROVIDER_LABELS = Object.fromEntries(
  PROVIDERS.map(provider => [provider.value, provider.label]),
) as Record<ObjectStorageProviderType, string>;

const HEALTH_LABELS: Record<ObjectStorageHealthStatus, string> = {
  [ObjectStorageHealthStatus.UNKNOWN]: '未检测',
  [ObjectStorageHealthStatus.HEALTHY]: '健康',
  [ObjectStorageHealthStatus.UNHEALTHY]: '异常',
};

const RUNTIME_SOURCE_LABELS: Record<ObjectStorageRuntimeSource, string> = {
  [ObjectStorageRuntimeSource.DATABASE]: '数据库配置',
  [ObjectStorageRuntimeSource.ENV_RESCUE]: '环境变量兜底',
};

const S3_LIKE_PROVIDERS = new Set<ObjectStorageProviderType>([
  ObjectStorageProviderType.AWS_S3,
  ObjectStorageProviderType.MINIO,
]);

const trimOrUndefined = (value: string) => {
  const trimmed = value.trim();
  return trimmed ? trimmed : undefined;
};

const formatTime = (value?: string) => {
  if (!value) {
    return '-';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '-';
  }
  return date.toLocaleString('zh-CN', {
    hour12: false,
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
};

const providerHint = (providerType: ObjectStorageProviderType) =>
  PROVIDERS.find(provider => provider.value === providerType)?.hint || '';

const emptyDraft = (): ObjectStorageDraft => ({
  name: '',
  provider_type: ObjectStorageProviderType.QINIU,
  bucket: '',
  region: '',
  endpoint: '',
  endpoint_override: '',
  force_path_style: false,
  use_ssl: true,
  download_domain: '',
  use_https: true,
  access_key_id: '',
  secret_access_key: '',
  replace_credential: true,
});

const draftFromConfig = (
  config: ObjectStorageConfigView,
): ObjectStorageDraft => ({
  name: config.name,
  provider_type: config.provider_type,
  bucket: config.config.bucket || '',
  region: config.config.region || '',
  endpoint: config.config.endpoint || '',
  endpoint_override: config.config.endpoint_override || '',
  force_path_style: Boolean(config.config.force_path_style),
  use_ssl: config.config.use_ssl ?? true,
  download_domain: config.config.download_domain || '',
  use_https: config.config.use_https ?? true,
  access_key_id: '',
  secret_access_key: '',
  replace_credential: !config.credential_configured,
});

const buildPublicConfig = (
  draft: ObjectStorageDraft,
): ObjectStoragePublicConfig => ({
  bucket: trimOrUndefined(draft.bucket),
  region: trimOrUndefined(draft.region),
  endpoint: trimOrUndefined(draft.endpoint),
  endpoint_override: trimOrUndefined(draft.endpoint_override),
  force_path_style: draft.force_path_style,
  use_ssl: draft.use_ssl,
  download_domain: trimOrUndefined(draft.download_domain),
  use_https: draft.use_https,
});

const isMigrationConfirmationRequired = (error: unknown) =>
  error instanceof AdminAPIError &&
  error.errorCode === 'OBJECT_STORAGE_MIGRATION_CONFIRMATION_REQUIRED';

const getErrorMessage = (error: unknown, fallback: string) =>
  error instanceof AdminAPIError && error.message ? error.message : fallback;

interface ObjectStorageConfigDialogProps {
  config: ObjectStorageConfigView | null;
  mode: 'create' | 'edit';
  open: boolean;
  saving: boolean;
  onCancel: () => void;
  onSave: (
    draft: ObjectStorageDraft,
    config: ObjectStorageConfigView | null,
  ) => Promise<void>;
  onTest: (
    draft: ObjectStorageDraft,
    config: ObjectStorageConfigView | null,
  ) => Promise<string>;
}

const ObjectStorageConfigDialog = ({
  config,
  mode,
  open,
  saving,
  onCancel,
  onSave,
  onTest,
}: ObjectStorageConfigDialogProps) => {
  const [draft, setDraft] = useState<ObjectStorageDraft>(emptyDraft);
  const [validationMessage, setValidationMessage] = useState('');
  const [testMessage, setTestMessage] = useState('');
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    if (!open) {
      setDraft(emptyDraft());
      setValidationMessage('');
      setTestMessage('');
      return;
    }
    setDraft(config ? draftFromConfig(config) : emptyDraft());
    setValidationMessage('');
    setTestMessage('');
  }, [config, open]);

  if (!open) {
    return null;
  }

  const updateDraft = <K extends keyof ObjectStorageDraft>(
    key: K,
    value: ObjectStorageDraft[K],
  ) => {
    setDraft(current => ({
      ...current,
      [key]: value,
    }));
  };

  const needsEndpoint = draft.provider_type === ObjectStorageProviderType.MINIO;
  const shouldSendCredential =
    mode === 'create' || draft.replace_credential || !config;
  const showS3Options = S3_LIKE_PROVIDERS.has(draft.provider_type);

  const validate = () => {
    if (!draft.name.trim()) {
      return '请填写配置名称';
    }
    if (!draft.bucket.trim()) {
      return '请填写 Bucket';
    }
    if (needsEndpoint && !draft.endpoint.trim()) {
      return 'MinIO 配置需要填写 Endpoint';
    }
    if (shouldSendCredential) {
      if (!draft.access_key_id.trim()) {
        return '请填写 Access Key ID';
      }
      if (!draft.secret_access_key.trim()) {
        return '请填写 Secret Access Key';
      }
    }
    return '';
  };

  const submit = async () => {
    const nextMessage = validate();
    if (nextMessage) {
      setValidationMessage(nextMessage);
      return;
    }
    setValidationMessage('');
    await onSave(draft, config);
  };

  const test = async () => {
    const nextMessage = validate();
    if (nextMessage) {
      setValidationMessage(nextMessage);
      return;
    }
    setValidationMessage('');
    setTesting(true);
    setTestMessage('');
    try {
      setTestMessage(await onTest(draft, config));
    } catch (error) {
      setTestMessage(getErrorMessage(error, '连接测试失败，请检查配置和密钥'));
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className={styles.dialogBackdrop}>
      <section
        aria-labelledby="object-storage-dialog-title"
        aria-modal="true"
        className={styles.dialog}
        role="dialog"
      >
        <header className={styles.dialogHeader}>
          <div>
            <h2 id="object-storage-dialog-title">
              {mode === 'create' ? '新增对象存储配置' : '编辑对象存储配置'}
            </h2>
            <p>
              支持七牛、阿里 OSS、腾讯 COS、华为 OBS、AWS S3、MinIO 和 TOS。
            </p>
          </div>
          <button
            aria-label="关闭对象存储配置表单"
            disabled={saving || testing}
            type="button"
            onClick={onCancel}
          >
            关闭
          </button>
        </header>

        <div className={styles.dialogBody}>
          <label>
            <span>配置名称</span>
            <input
              aria-label="对象存储配置名称"
              autoFocus
              value={draft.name}
              onChange={event => updateDraft('name', event.target.value)}
            />
          </label>

          <label>
            <span>云厂商</span>
            <select
              aria-label="对象存储云厂商"
              disabled={mode === 'edit'}
              value={draft.provider_type}
              onChange={event =>
                updateDraft(
                  'provider_type',
                  Number(event.target.value) as ObjectStorageProviderType,
                )
              }
            >
              {PROVIDERS.map(provider => (
                <option key={provider.value} value={provider.value}>
                  {provider.label}
                </option>
              ))}
            </select>
            <small>{providerHint(draft.provider_type)}</small>
          </label>

          <label>
            <span>Bucket</span>
            <input
              aria-label="对象存储 Bucket"
              value={draft.bucket}
              onChange={event => updateDraft('bucket', event.target.value)}
            />
          </label>

          <label>
            <span>Region</span>
            <input
              aria-label="对象存储 Region"
              placeholder={needsEndpoint ? '可选' : '例如 z0 / cn-hangzhou'}
              value={draft.region}
              onChange={event => updateDraft('region', event.target.value)}
            />
          </label>

          <label>
            <span>Endpoint</span>
            <input
              aria-label="对象存储 Endpoint"
              placeholder={needsEndpoint ? 'http://minio:9000' : '可选'}
              value={draft.endpoint}
              onChange={event => updateDraft('endpoint', event.target.value)}
            />
          </label>

          <label>
            <span>Endpoint Override</span>
            <input
              aria-label="对象存储 Endpoint Override"
              placeholder="兼容 S3 或专有域名时填写"
              value={draft.endpoint_override}
              onChange={event =>
                updateDraft('endpoint_override', event.target.value)
              }
            />
          </label>

          <label>
            <span>下载域名</span>
            <input
              aria-label="对象存储下载域名"
              placeholder="assets.example.com"
              value={draft.download_domain}
              onChange={event =>
                updateDraft('download_domain', event.target.value)
              }
            />
          </label>

          <div className={styles.inlineChecks}>
            {showS3Options ? (
              <>
                <label>
                  <input
                    aria-label="对象存储 Path Style"
                    checked={draft.force_path_style}
                    type="checkbox"
                    onChange={event =>
                      updateDraft('force_path_style', event.target.checked)
                    }
                  />
                  <span>Path Style</span>
                </label>
                <label>
                  <input
                    aria-label="对象存储使用 SSL"
                    checked={draft.use_ssl}
                    type="checkbox"
                    onChange={event =>
                      updateDraft('use_ssl', event.target.checked)
                    }
                  />
                  <span>使用 SSL</span>
                </label>
              </>
            ) : null}
            <label>
              <input
                aria-label="对象存储下载使用 HTTPS"
                checked={draft.use_https}
                type="checkbox"
                onChange={event =>
                  updateDraft('use_https', event.target.checked)
                }
              />
              <span>下载 HTTPS</span>
            </label>
          </div>

          {mode === 'edit' ? (
            <div className={styles.credentialNotice}>
              <label>
                <input
                  aria-label="替换对象存储 AK/SK"
                  checked={draft.replace_credential}
                  type="checkbox"
                  onChange={event =>
                    updateDraft('replace_credential', event.target.checked)
                  }
                />
                <span>替换 AK/SK</span>
              </label>
              <p>
                {config?.credential_configured
                  ? '已保存密钥不会回显；不勾选时会继续使用当前加密密钥。'
                  : '当前未配置密钥，请勾选后补充 AK/SK。'}
              </p>
            </div>
          ) : null}

          <label>
            <span>Access Key ID</span>
            <input
              aria-label="对象存储 Access Key ID"
              disabled={!shouldSendCredential}
              value={draft.access_key_id}
              onChange={event =>
                updateDraft('access_key_id', event.target.value)
              }
            />
          </label>

          <label>
            <span>Secret Access Key</span>
            <input
              aria-label="对象存储 Secret Access Key"
              disabled={!shouldSendCredential}
              type="password"
              value={draft.secret_access_key}
              onChange={event =>
                updateDraft('secret_access_key', event.target.value)
              }
            />
          </label>
        </div>

        {validationMessage ? (
          <p className={styles.message} role="alert">
            {validationMessage}
          </p>
        ) : null}
        {testMessage ? (
          <p className={styles.message} role="status">
            {testMessage}
          </p>
        ) : null}

        <footer className={styles.dialogFooter}>
          <button
            disabled={saving || testing}
            type="button"
            onClick={() => void test()}
          >
            <IconCozCheckMark />
            <span>{testing ? '测试中...' : '测试连接'}</span>
          </button>
          <div>
            <button
              disabled={saving || testing}
              type="button"
              onClick={onCancel}
            >
              取消
            </button>
            <button
              aria-label="保存对象存储配置"
              className={styles.primaryButton}
              disabled={saving || testing}
              type="button"
              onClick={() => void submit()}
            >
              {saving ? '保存中...' : '保存'}
            </button>
          </div>
        </footer>
      </section>
    </div>
  );
};

export const ObjectStorageSection = () => {
  const [configs, setConfigs] = useState<ObjectStorageConfigView[]>([]);
  const [runtimeSource, setRuntimeSource] =
    useState<ObjectStorageRuntimeSource>(ObjectStorageRuntimeSource.DATABASE);
  const [restartRequired, setRestartRequired] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [dialogMode, setDialogMode] = useState<'create' | 'edit'>('create');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogConfig, setDialogConfig] =
    useState<ObjectStorageConfigView | null>(null);
  const [deleteCandidate, setDeleteCandidate] =
    useState<ObjectStorageConfigView | null>(null);
  const [activationCandidate, setActivationCandidate] =
    useState<ObjectStorageConfigView | null>(null);

  const activeConfig = useMemo(
    () =>
      configs.find(config => config.runtime_active || config.desired_active),
    [configs],
  );

  const loadConfigs = useCallback(async (successMessage = '') => {
    setLoading(true);
    setMessage('');
    try {
      const response = await listObjectStorageConfigs();
      setConfigs(response.configs ?? []);
      setRuntimeSource(
        response.runtime_source ?? ObjectStorageRuntimeSource.DATABASE,
      );
      setRestartRequired(Boolean(response.restart_required));
      if (successMessage) {
        setMessage(successMessage);
      }
    } catch (error) {
      setMessage('加载对象存储配置失败，请稍后重试');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadConfigs();
  }, [loadConfigs]);

  const openCreate = () => {
    setDialogMode('create');
    setDialogConfig(null);
    setDialogOpen(true);
  };

  const openEdit = (config: ObjectStorageConfigView) => {
    setDialogMode('edit');
    setDialogConfig(config);
    setDialogOpen(true);
  };

  const closeDialog = () => {
    setDialogOpen(false);
    setDialogConfig(null);
    setDialogMode('create');
  };

  const saveConfig = async (
    draft: ObjectStorageDraft,
    config: ObjectStorageConfigView | null,
  ) => {
    setBusy(true);
    setMessage('');
    try {
      const publicConfig = buildPublicConfig(draft);
      const credential =
        draft.replace_credential || dialogMode === 'create'
          ? {
              access_key_id: draft.access_key_id.trim(),
              secret_access_key: draft.secret_access_key.trim(),
            }
          : undefined;
      if (dialogMode === 'create') {
        if (!credential) {
          throw new AdminAPIError(
            400,
            '请填写 Access Key ID 和 Secret Access Key',
          );
        }
        await createObjectStorageConfig({
          name: draft.name.trim(),
          provider_type: draft.provider_type,
          config: publicConfig,
          credential,
        });
      } else if (config) {
        await updateObjectStorageConfig({
          id: config.id,
          expected_version: config.version,
          name: draft.name.trim(),
          config: publicConfig,
          credential,
        });
      }
      closeDialog();
      await loadConfigs(dialogMode === 'create' ? '配置已新增' : '配置已更新');
    } catch (error) {
      setMessage(getErrorMessage(error, '保存对象存储配置失败'));
      throw error;
    } finally {
      setBusy(false);
    }
  };

  const testConfig = async (
    draft: ObjectStorageDraft,
    config: ObjectStorageConfigView | null,
  ) => {
    const response = await testObjectStorageConfig({
      id: config?.id,
      expected_version: config?.version,
      provider_type: draft.provider_type,
      config: buildPublicConfig(draft),
      credential:
        draft.replace_credential || dialogMode === 'create'
          ? {
              access_key_id: draft.access_key_id.trim(),
              secret_access_key: draft.secret_access_key.trim(),
            }
          : undefined,
    });
    const label = HEALTH_LABELS[response.health.status] || '未知状态';
    if (!response.success) {
      return response.health.message || `连接测试未通过：${label}`;
    }
    return response.health.latency_ms
      ? `连接测试通过，耗时 ${response.health.latency_ms} ms`
      : '连接测试通过';
  };

  const activateConfig = async (
    config: ObjectStorageConfigView,
    migrationConfirmed: boolean,
  ) => {
    setBusy(true);
    setMessage('');
    try {
      await activateObjectStorageConfig({
        id: config.id,
        expected_version: config.version,
        migration_confirmed: migrationConfirmed,
      });
      setActivationCandidate(null);
      await loadConfigs('主配置已切换');
    } catch (error) {
      if (isMigrationConfirmationRequired(error)) {
        setActivationCandidate(config);
      } else {
        setMessage(getErrorMessage(error, '激活对象存储配置失败'));
      }
    } finally {
      setBusy(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteCandidate) {
      return;
    }
    setBusy(true);
    setMessage('');
    try {
      await deleteObjectStorageConfig({
        id: deleteCandidate.id,
        expected_version: deleteCandidate.version,
      });
      setDeleteCandidate(null);
      await loadConfigs('配置已删除');
    } catch (error) {
      setMessage(getErrorMessage(error, '删除对象存储配置失败'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className={styles.section}>
      <header className={styles.toolbar}>
        <div>
          <h2>对象存储</h2>
          <p>管理多套对象存储配置，并选择当前运行使用的主配置。</p>
        </div>
        <div>
          <button
            aria-label="刷新对象存储配置"
            disabled={busy || loading}
            type="button"
            onClick={() => void loadConfigs('对象存储配置已刷新')}
          >
            <IconCozRefresh />
            <span>刷新</span>
          </button>
          <button
            aria-label="新增对象存储配置"
            className={styles.primaryButton}
            disabled={busy}
            type="button"
            onClick={openCreate}
          >
            <IconCozPlus />
            <span>新增配置</span>
          </button>
        </div>
      </header>

      <div className={styles.summaryGrid}>
        <article>
          <span>当前主配置</span>
          <strong>{activeConfig?.name || '未配置'}</strong>
          <small>
            {activeConfig
              ? PROVIDER_LABELS[activeConfig.provider_type]
              : '请新增并激活配置'}
          </small>
        </article>
        <article>
          <span>运行来源</span>
          <strong>{RUNTIME_SOURCE_LABELS[runtimeSource]}</strong>
          <small>
            {restartRequired ? '切换后需重启生效' : '运行中配置已同步'}
          </small>
        </article>
        <article>
          <span>配置数量</span>
          <strong>{configs.length}</strong>
          <small>密钥已加密保存，页面不回显 AK/SK</small>
        </article>
      </div>

      {message ? (
        <p className={styles.message} role="status">
          {message}
        </p>
      ) : null}

      <section className={styles.panel}>
        <div className={styles.tableWrap}>
          <table>
            <thead>
              <tr>
                <th>配置</th>
                <th>云厂商</th>
                <th>Bucket / Region</th>
                <th>Endpoint</th>
                <th>健康</th>
                <th>运行状态</th>
                <th>更新时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {configs.map(config => (
                <tr key={config.id}>
                  <td>
                    <strong>{config.name}</strong>
                    <small>ID {config.id}</small>
                  </td>
                  <td>{PROVIDER_LABELS[config.provider_type]}</td>
                  <td>
                    <strong>{config.config.bucket || '-'}</strong>
                    <small>{config.config.region || '-'}</small>
                  </td>
                  <td className={styles.endpointCell}>
                    {config.config.endpoint ||
                      config.config.endpoint_override ||
                      config.config.download_domain ||
                      '-'}
                  </td>
                  <td>
                    <span
                      className={styles.healthBadge}
                      data-status={config.health.status}
                    >
                      {HEALTH_LABELS[config.health.status] || '未知'}
                    </span>
                    {config.health.message ? (
                      <small>{config.health.message}</small>
                    ) : null}
                  </td>
                  <td>
                    <div className={styles.stateTags}>
                      {config.desired_active ? <span>主配置</span> : null}
                      {config.runtime_active ? <span>运行中</span> : null}
                      {config.restart_required ? <span>需重启</span> : null}
                      {config.credential_configured ? (
                        <span>密钥已配置</span>
                      ) : null}
                    </div>
                  </td>
                  <td>{formatTime(config.updated_at)}</td>
                  <td>
                    <div className={styles.rowActions}>
                      <button
                        aria-label={`编辑对象存储配置-${config.id}`}
                        disabled={busy}
                        title="编辑配置"
                        type="button"
                        onClick={() => openEdit(config)}
                      >
                        <IconCozEdit />
                        <span>编辑</span>
                      </button>
                      <button
                        aria-label={`测试对象存储配置-${config.id}`}
                        disabled={busy}
                        title="测试连接"
                        type="button"
                        onClick={() =>
                          void testConfig(draftFromConfig(config), config)
                            .then(result => setMessage(result))
                            .catch(error =>
                              setMessage(
                                getErrorMessage(error, '对象存储连接测试失败'),
                              ),
                            )
                        }
                      >
                        <IconCozCheckMark />
                        <span>测试</span>
                      </button>
                      <button
                        aria-label={`激活对象存储配置-${config.id}`}
                        disabled={busy || config.desired_active}
                        title="激活为主配置"
                        type="button"
                        onClick={() => void activateConfig(config, false)}
                      >
                        <IconCozCheckMark />
                        <span>激活</span>
                      </button>
                      <button
                        aria-label={`删除对象存储配置-${config.id}`}
                        className={styles.dangerButton}
                        disabled={busy || config.desired_active}
                        title={
                          config.desired_active
                            ? '主配置不能直接删除'
                            : '删除配置'
                        }
                        type="button"
                        onClick={() => setDeleteCandidate(config)}
                      >
                        <IconCozTrashCan />
                        <span>删除</span>
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {!loading && configs.length === 0 ? (
                <tr>
                  <td className={styles.emptyCell} colSpan={8}>
                    暂无对象存储配置
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
          {loading ? <p className={styles.emptyCell}>加载中...</p> : null}
        </div>
      </section>

      <ObjectStorageConfigDialog
        config={dialogConfig}
        mode={dialogMode}
        open={dialogOpen}
        saving={busy}
        onCancel={closeDialog}
        onSave={saveConfig}
        onTest={testConfig}
      />

      {deleteCandidate ? (
        <div className={styles.dialogBackdrop}>
          <section
            aria-labelledby="object-storage-delete-title"
            aria-modal="true"
            className={styles.confirmDialog}
            role="alertdialog"
          >
            <h2 id="object-storage-delete-title">删除对象存储配置</h2>
            <p>确认删除“{deleteCandidate.name}”吗？删除后无法恢复。</p>
            <div>
              <button
                disabled={busy}
                type="button"
                onClick={() => setDeleteCandidate(null)}
              >
                取消
              </button>
              <button
                className={styles.dangerButton}
                disabled={busy}
                type="button"
                onClick={() => void confirmDelete()}
              >
                {busy ? '删除中...' : '确认删除'}
              </button>
            </div>
          </section>
        </div>
      ) : null}

      {activationCandidate ? (
        <div className={styles.dialogBackdrop}>
          <section
            aria-labelledby="object-storage-activate-title"
            aria-modal="true"
            className={styles.confirmDialog}
            role="alertdialog"
          >
            <h2 id="object-storage-activate-title">确认切换主配置</h2>
            <p>
              当前配置正在被运行时使用。确认后会切换主配置，已有对象不会自动迁移。
            </p>
            <div>
              <button
                disabled={busy}
                type="button"
                onClick={() => setActivationCandidate(null)}
              >
                取消
              </button>
              <button
                className={styles.primaryButton}
                disabled={busy}
                type="button"
                onClick={() => void activateConfig(activationCandidate, true)}
              >
                确认切换
              </button>
            </div>
          </section>
        </div>
      ) : null}
    </section>
  );
};
