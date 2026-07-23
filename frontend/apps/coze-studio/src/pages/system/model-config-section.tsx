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

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, max-lines -- Production model management page. */

import { useCallback, useEffect, useMemo, useState } from 'react';

import { IconCozHandle, IconCozPlus } from '@coze-arch/coze-design/icons';

import { getI18nText } from './view-model';
import {
  createAdminManagedModel,
  deleteAdminModel,
  getAdminManagedModelDetail,
  getAdminManagedModelGrants,
  listAdminManagedModels,
  listAdminModelProviders,
  listAdminUsers,
  listAdminWorkspaces,
  saveAdminManagedModelGrants,
  sortAdminManagedModels,
  testAdminManagedModelEndpoint,
  updateAdminManagedModel,
  type AdminCreateModelPayload,
  type AdminManagedModel,
  type AdminModelDetail,
  type AdminModelGrantSubject,
  type AdminModelListFilters,
  type AdminModelManagementInput,
  type AdminModelProviderOption,
  type AdminProviderModelListItem,
  type AdminUser,
  type AdminWorkspace,
} from './service';
import { ModelGrantsDialog } from './model-grants-dialog';
import { ModelConfigDialog } from './model-config-dialog';

interface ModelConfigSectionProps {
  modelProviders?: AdminProviderModelListItem[];
  modelSaving?: boolean;
  modelRefreshing?: boolean;
  modelMessage?: string;
  onCreateModel?: (payload: AdminCreateModelPayload) => void | Promise<void>;
  onDeleteModel?: (id: number | string) => void | Promise<void>;
  onRefresh?: () => void | Promise<void>;
}

const CAPABILITY_LABELS: Record<string, string> = {
  audio: '语音理解',
  image: '图像理解',
  reasoning: '深度思考',
  text: '文本生成',
  video: '视频理解',
};

const ACCESS_ALL = 1;
const ACCESS_RESTRICTED = 2;

const formatUpdatedTime = (value: number) => {
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

export const ModelConfigSection = (_props: ModelConfigSectionProps) => {
  const [models, setModels] = useState<AdminManagedModel[]>([]);
  const [total, setTotal] = useState(0);
  const [providers, setProviders] = useState<AdminModelProviderOption[]>([]);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [workspaces, setWorkspaces] = useState<AdminWorkspace[]>([]);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [capabilityType, setCapabilityType] = useState('');
  const [status, setStatus] = useState('');
  const [accessMode, setAccessMode] = useState('');
  const [activeFilters, setActiveFilters] = useState<AdminModelListFilters>({
    page: 1,
    page_size: 100,
  });
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogDetail, setDialogDetail] = useState<AdminModelDetail | null>(
    null,
  );
  const [grantsOpen, setGrantsOpen] = useState(false);
  const [grantsModel, setGrantsModel] = useState<AdminManagedModel | null>(
    null,
  );
  const [grants, setGrants] = useState<AdminModelGrantSubject[]>([]);
  const [deleteModel, setDeleteModel] = useState<AdminManagedModel | null>(
    null,
  );
  const [draggedID, setDraggedID] = useState<string | null>(null);

  const loadModels = useCallback(async (filters: AdminModelListFilters) => {
    const response = await listAdminManagedModels(filters);
    setModels(response.models ?? []);
    setTotal(response.total ?? response.models?.length ?? 0);
  }, []);

  const loadInitialData = useCallback(async () => {
    setLoading(true);
    setMessage('');
    try {
      const [modelResponse, providerResponse, userResponse, workspaceResponse] =
        await Promise.all([
          listAdminManagedModels({ page: 1, page_size: 100 }),
          listAdminModelProviders(),
          listAdminUsers({ page: 1, size: 100 }),
          listAdminWorkspaces({ page: 1, size: 100 }),
        ]);
      setModels(modelResponse.models ?? []);
      setTotal(modelResponse.total ?? modelResponse.models?.length ?? 0);
      setProviders(providerResponse.providers ?? []);
      setUsers(userResponse.users ?? []);
      setWorkspaces(workspaceResponse.workspaces ?? []);
    } catch (error) {
      setMessage('加载模型配置失败，请稍后重试');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadInitialData();
  }, [loadInitialData]);

  const providerNameMap = useMemo(
    () =>
      Object.fromEntries(
        providers.map(provider => [
          provider.provider_key,
          getI18nText(provider.name),
        ]),
      ),
    [providers],
  );

  const query = async () => {
    const filters: AdminModelListFilters = {
      access_mode: accessMode || undefined,
      capability_type: capabilityType || undefined,
      enabled:
        status === 'enabled' ? true : status === 'disabled' ? false : undefined,
      page: 1,
      page_size: 100,
    };
    setBusy(true);
    setMessage('');
    try {
      await loadModels(filters);
      setActiveFilters(filters);
    } catch (error) {
      setMessage('查询模型配置失败');
    } finally {
      setBusy(false);
    }
  };

  const reset = async () => {
    setCapabilityType('');
    setStatus('');
    setAccessMode('');
    const filters = { page: 1, page_size: 100 };
    setBusy(true);
    try {
      await loadModels(filters);
      setActiveFilters(filters);
    } finally {
      setBusy(false);
    }
  };

  const refresh = async (successMessage = '') => {
    await loadModels(activeFilters);
    if (successMessage) {
      setMessage(successMessage);
    }
  };

  const openCreate = () => {
    setDialogDetail(null);
    setDialogOpen(true);
  };

  const openEdit = async (model: AdminManagedModel) => {
    setBusy(true);
    setMessage('');
    try {
      const response = await getAdminManagedModelDetail(model.id);
      setDialogDetail(response.model);
      setDialogOpen(true);
    } catch (error) {
      setMessage('读取模型详情失败');
    } finally {
      setBusy(false);
    }
  };

  const saveModel = async (
    id: AdminManagedModel['id'] | undefined,
    modelClass: number,
    input: AdminModelManagementInput,
  ) => {
    setBusy(true);
    setMessage('');
    try {
      if (id) {
        await updateAdminManagedModel({ id, management: input });
      } else {
        await createAdminManagedModel({
          management: input,
          model_class: modelClass,
        });
      }
      setDialogOpen(false);
      await refresh(id ? '模型配置已更新' : '模型配置已创建');
    } catch (error) {
      setMessage('保存模型配置失败，请检查模型信息和 Endpoint 凭证');
      throw error;
    } finally {
      setBusy(false);
    }
  };

  const openGrants = async (model: AdminManagedModel) => {
    setBusy(true);
    try {
      const response = await getAdminManagedModelGrants(model.id);
      setGrants(response.grants ?? []);
      setGrantsModel(model);
      setGrantsOpen(true);
    } catch (error) {
      setMessage('读取模型授权范围失败');
    } finally {
      setBusy(false);
    }
  };

  const saveGrants = async (nextGrants: AdminModelGrantSubject[]) => {
    if (!grantsModel) {
      return;
    }
    setBusy(true);
    try {
      await saveAdminManagedModelGrants({
        access_mode: ACCESS_RESTRICTED,
        grants: nextGrants,
        model_id: grantsModel.id,
      });
      setGrantsOpen(false);
      await refresh('模型授权范围已更新');
    } catch (error) {
      setMessage('保存模型授权范围失败');
      throw error;
    } finally {
      setBusy(false);
    }
  };

  const toggleAccess = async (
    model: AdminManagedModel,
    restricted: boolean,
  ) => {
    if (restricted) {
      await openGrants(model);
      return;
    }
    setBusy(true);
    try {
      await saveAdminManagedModelGrants({
        access_mode: ACCESS_ALL,
        grants: [],
        model_id: model.id,
      });
      await refresh('模型已调整为全部用户可用');
    } catch (error) {
      setMessage('更新模型管控状态失败');
    } finally {
      setBusy(false);
    }
  };

  const reorder = async (targetID: string) => {
    if (!draggedID || draggedID === targetID) {
      setDraggedID(null);
      return;
    }
    const sourceIndex = models.findIndex(
      model => String(model.id) === draggedID,
    );
    const targetIndex = models.findIndex(
      model => String(model.id) === targetID,
    );
    if (sourceIndex < 0 || targetIndex < 0) {
      setDraggedID(null);
      return;
    }
    const reordered = [...models];
    const [source] = reordered.splice(sourceIndex, 1);
    reordered.splice(targetIndex, 0, source);
    setModels(reordered);
    setDraggedID(null);
    setBusy(true);
    try {
      await sortAdminManagedModels(
        reordered.map((model, index) => ({
          id: model.id,
          sort_order: index + 1,
        })),
      );
      setMessage('模型排序已保存');
    } catch (error) {
      await refresh();
      setMessage('保存模型排序失败');
    } finally {
      setBusy(false);
    }
  };

  const confirmDelete = async () => {
    if (!deleteModel) {
      return;
    }
    setBusy(true);
    try {
      await deleteAdminModel({ id: deleteModel.id, preview: false });
      setDeleteModel(null);
      await refresh('模型配置已删除');
    } catch (error) {
      setMessage('删除模型配置失败，模型可能仍被业务引用');
    } finally {
      setBusy(false);
    }
  };

  const prepareDelete = async (model: AdminManagedModel) => {
    setBusy(true);
    try {
      await deleteAdminModel({ id: model.id, preview: true });
      setDeleteModel(model);
    } catch (error) {
      setMessage('无法检查模型依赖，请稍后重试');
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="coze-prototype-system-model-page">
      <header className="coze-prototype-system-model-toolbar">
        <div>
          <h2>模型配置</h2>
          <p>集中管理公共模型、能力范围、接入端点和使用授权。</p>
        </div>
        <button
          aria-label="添加模型"
          className="is-primary coze-prototype-system-model-add"
          disabled={busy}
          type="button"
          onClick={openCreate}
        >
          <IconCozPlus />
          <span>添加模型</span>
        </button>
      </header>

      <section className="coze-prototype-system-model-panel">
        <div className="coze-prototype-system-model-filters">
          <select
            aria-label="模型类型筛选"
            value={capabilityType}
            onChange={event => setCapabilityType(event.target.value)}
          >
            <option value="">类型</option>
            {Object.entries(CAPABILITY_LABELS).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </select>
          <select
            aria-label="模型状态筛选"
            value={status}
            onChange={event => setStatus(event.target.value)}
          >
            <option value="">状态</option>
            <option value="enabled">启用</option>
            <option value="disabled">停用</option>
          </select>
          <select
            aria-label="模型管控筛选"
            value={accessMode}
            onChange={event => setAccessMode(event.target.value)}
          >
            <option value="">管控</option>
            <option value="all">关闭</option>
            <option value="restricted">开启</option>
          </select>
          <div className="coze-prototype-system-model-filter-actions">
            <button disabled={busy} type="button" onClick={() => void reset()}>
              重置
            </button>
            <button
              className="is-primary"
              disabled={busy}
              type="button"
              onClick={() => void query()}
            >
              查询
            </button>
          </div>
        </div>

        <span className="coze-prototype-system-model-count">
          共 {total} 个模型
        </span>
        {message ? (
          <p className="coze-prototype-system-model-message" role="status">
            {message}
          </p>
        ) : null}
        <div className="coze-prototype-system-model-table-wrap">
          <table>
            <thead>
              <tr>
                <th>排序</th>
                <th>模型名称</th>
                <th>类型</th>
                <th>模型标识</th>
                <th>模型介绍</th>
                <th>状态</th>
                <th>创建者</th>
                <th>更新时间</th>
                <th>管控</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {models.map(model => (
                <tr
                  draggable
                  key={String(model.id)}
                  onDragOver={event => event.preventDefault()}
                  onDragStart={() => setDraggedID(String(model.id))}
                  onDrop={() => void reorder(String(model.id))}
                >
                  <td>
                    <button
                      aria-label={`调整${model.name}排序`}
                      className="coze-prototype-system-model-drag"
                      title="拖动排序"
                      type="button"
                    >
                      <IconCozHandle />
                    </button>
                  </td>
                  <td>
                    <strong>{model.name}</strong>
                    <small>
                      {providerNameMap[model.provider_key] ||
                        model.provider_key}
                    </small>
                  </td>
                  <td>
                    <div className="coze-prototype-system-model-tags">
                      {model.capability_types.length > 0
                        ? model.capability_types.map(type => (
                            <span key={type}>
                              {CAPABILITY_LABELS[type] || type}
                            </span>
                          ))
                        : '-'}
                    </div>
                  </td>
                  <td>{model.model_identifier}</td>
                  <td className="coze-prototype-system-model-description">
                    {model.description || '-'}
                  </td>
                  <td>
                    <span
                      className="coze-prototype-system-model-status"
                      data-enabled={model.enabled}
                    >
                      {model.enabled ? '启用' : '停用'}
                    </span>
                  </td>
                  <td>
                    {model.creator_name ||
                      (model.creator_id ? `用户 ${model.creator_id}` : '系统')}
                  </td>
                  <td>{formatUpdatedTime(model.updated_at_ms)}</td>
                  <td>
                    <label className="coze-prototype-system-model-switch">
                      <input
                        aria-label={`${model.name} 管控`}
                        checked={model.access_mode === ACCESS_RESTRICTED}
                        disabled={busy}
                        type="checkbox"
                        onChange={event =>
                          void toggleAccess(model, event.target.checked)
                        }
                      />
                      <span />
                    </label>
                  </td>
                  <td>
                    <div className="coze-prototype-system-model-actions">
                      <button
                        disabled={busy}
                        type="button"
                        onClick={() => void openEdit(model)}
                      >
                        编辑
                      </button>
                      {model.access_mode === ACCESS_RESTRICTED ? (
                        <button
                          disabled={busy}
                          type="button"
                          onClick={() => void openGrants(model)}
                        >
                          授权
                        </button>
                      ) : null}
                      <button
                        className="is-danger"
                        disabled={busy}
                        type="button"
                        onClick={() => void prepareDelete(model)}
                      >
                        删除
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {!loading && models.length === 0 ? (
                <tr>
                  <td
                    className="coze-prototype-system-model-empty"
                    colSpan={10}
                  >
                    暂无符合条件的模型配置
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
          {loading ? (
            <p className="coze-prototype-system-model-empty">加载中...</p>
          ) : null}
        </div>
      </section>

      <ModelConfigDialog
        detail={dialogDetail}
        open={dialogOpen}
        providers={providers}
        saving={busy}
        onCancel={() => setDialogOpen(false)}
        onSave={saveModel}
        onTest={testAdminManagedModelEndpoint}
      />
      <ModelGrantsDialog
        grants={grants}
        loading={busy}
        model={grantsModel}
        open={grantsOpen}
        users={users}
        workspaces={workspaces}
        onCancel={() => setGrantsOpen(false)}
        onSave={saveGrants}
      />
      {deleteModel ? (
        <div className="coze-prototype-model-dialog-backdrop">
          <section
            aria-labelledby="system-model-delete-title"
            aria-modal="true"
            className="coze-prototype-model-confirm-dialog"
            role="alertdialog"
          >
            <h2 id="system-model-delete-title">删除模型配置</h2>
            <p>
              确认删除“{deleteModel.name}”吗？删除后该模型将无法继续用于新任务。
            </p>
            <div>
              <button
                disabled={busy}
                type="button"
                onClick={() => setDeleteModel(null)}
              >
                取消
              </button>
              <button
                className="is-danger"
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
    </section>
  );
};
