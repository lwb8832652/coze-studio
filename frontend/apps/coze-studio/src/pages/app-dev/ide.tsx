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

import { useNavigate, useParams } from 'react-router-dom';
import { useCallback, useEffect, useRef, useState } from 'react';

import { useUserInfo } from '@coze-arch/foundation-sdk';

import { isAppDevReleaseStale } from './utils/release-status';
import {
  clearAppDevPendingOperation,
  createAppDevOperationID,
  readAppDevPendingOperationResult,
  writeAppDevPendingOperation,
} from './utils/provider-operation';
import { downloadAppDevBlob } from './utils/download-blob';
import { canBuildAppDevRuntime } from './utils/runtime-capabilities';
import { normalizeAppDevPreviewUrl } from './utils/preview-url';
import type { AppDevSnapshot } from './types';
import {
  createAppDevSnapshot,
  downloadAppDevRelease,
  exportAppDevProject,
  importAppDevProject,
  isAppDevDeterministicError,
  isAppDevReleaseStaleError,
  listAppDevSnapshots,
  normalizeAppDevError,
  restoreAppDevSnapshot,
  updateAppDevProject,
} from './service';
import { useAppDevBuild } from './hooks/use-app-dev-build';
import { useAppDevRuntime } from './hooks/use-app-dev-runtime';
import { useAppDevProjectInfo } from './hooks/use-app-dev-project-info';
import { useAppDevFiles } from './hooks/use-app-dev-files';
import { useConfirmDialog } from './components/text-input-dialog';
import { AccessibleDialog } from './components/accessible-dialog';
import { RuntimeToolbar } from './components/runtime-toolbar';
import { PreviewPanel } from './components/preview-panel';
import { ImportProjectModal } from './components/import-project-modal';
import { FileTree } from './components/file-tree';
import { DevLogsPanel } from './components/dev-logs-panel';
import { CodeEditor } from './components/code-editor';
import { APP_DEV_DESIGN_TARGETS, ChatPanel } from './components/chat-panel';
import type {
  AppDevDesignModeState,
  AppDevDesignSelection,
} from './components/chat-panel';

import './index.less';
import './newx-refresh.less';

export default function AppDevIDEPage() {
  const navigate = useNavigate();
  const { space_id: spaceId = '', project_id: projectId = '' } = useParams();
  const userInfo = useUserInfo();
  const principalId = userInfo?.user_id_str?.trim() || undefined;
  const projectState = useAppDevProjectInfo(spaceId, projectId);
  const refreshProject = projectState.refresh;
  const fileState = useAppDevFiles(spaceId, projectId);
  const runtimeState = useAppDevRuntime(spaceId, projectId, principalId);
  const buildState = useAppDevBuild(spaceId, projectId, principalId);
  const [exporting, setExporting] = useState(false);
  const [downloadingRelease, setDownloadingRelease] = useState(false);
  const [importModalVisible, setImportModalVisible] = useState(false);
  const [importingProject, setImportingProject] = useState(false);
  const [snapshotModalVisible, setSnapshotModalVisible] = useState(false);
  const [snapshotLoading, setSnapshotLoading] = useState(false);
  const [snapshotActionLoading, setSnapshotActionLoading] = useState(false);
  const [snapshots, setSnapshots] = useState<AppDevSnapshot[]>([]);
  const [snapshotNameModalVisible, setSnapshotNameModalVisible] =
    useState(false);
  const [snapshotName, setSnapshotName] = useState('手动快照');
  const [snapshotNameError, setSnapshotNameError] = useState('');
  const [savingProjectMeta, setSavingProjectMeta] = useState(false);
  const [projectMetaEditing, setProjectMetaEditing] = useState(false);
  const [projectMetaForm, setProjectMetaForm] = useState({
    name: '',
    description: '',
  });
  const [activeWorkspaceTab, setActiveWorkspaceTab] = useState<
    'preview' | 'code'
  >('preview');
  const [previewFullscreenSignal, setPreviewFullscreenSignal] = useState(0);
  const [previewRefreshSignal, setPreviewRefreshSignal] = useState(0);
  const [releaseStale, setReleaseStale] = useState(false);
  const [openDesignSignal, setOpenDesignSignal] = useState(0);
  const [selectedDesignTarget, setSelectedDesignTarget] = useState(
    APP_DEV_DESIGN_TARGETS[0],
  );
  const [designSelection, setDesignSelection] =
    useState<AppDevDesignSelection | null>(null);
  const [designModeState, setDesignModeState] = useState<AppDevDesignModeState>(
    {
      active: false,
      summary: '现代 SaaS · 全局视觉系统',
      prompt: '',
    },
  );
  const [logsVisible, setLogsVisible] = useState(false);
  const [fileTreeCollapsed, setFileTreeCollapsed] = useState(false);
  const [notice, setNotice] = useState('');
  const providerActionEpochRef = useRef(0);
  const snapshotActionRef = useRef<{
    operationId: string;
    promise: Promise<void>;
    controller: AbortController;
  }>();
  const snapshotRequestRef = useRef<{
    sequence: number;
    controller: AbortController;
  }>();
  const snapshotRequestSequenceRef = useRef(0);
  const releaseActionRef = useRef<{
    promise: Promise<void>;
    controller: AbortController;
  }>();
  const exportActionRef = useRef<{
    promise: Promise<void>;
    controller: AbortController;
  }>();
  const identityKey = `${spaceId}\u0000${projectId}\u0000${principalId || ''}`;
  const { openConfirmDialog, confirmDialog } = useConfirmDialog();

  useEffect(() => {
    providerActionEpochRef.current += 1;
    snapshotActionRef.current?.controller.abort();
    snapshotRequestRef.current?.controller.abort();
    snapshotRequestSequenceRef.current += 1;
    releaseActionRef.current?.controller.abort();
    exportActionRef.current?.controller.abort();
    snapshotActionRef.current = undefined;
    snapshotRequestRef.current = undefined;
    releaseActionRef.current = undefined;
    exportActionRef.current = undefined;
    setSnapshotActionLoading(false);
    setSnapshotLoading(false);
    setSnapshots([]);
    setDownloadingRelease(false);
    setExporting(false);
    setReleaseStale(false);
    setNotice('');
    return () => {
      providerActionEpochRef.current += 1;
      snapshotActionRef.current?.controller.abort();
      snapshotRequestRef.current?.controller.abort();
      snapshotRequestSequenceRef.current += 1;
      releaseActionRef.current?.controller.abort();
      exportActionRef.current?.controller.abort();
    };
  }, [identityKey]);
  const refreshSnapshots = useCallback(async () => {
    if (!spaceId || !projectId) {
      setSnapshots([]);
      setSnapshotLoading(false);
      return;
    }
    snapshotRequestRef.current?.controller.abort();
    const controller = new AbortController();
    const sequence = ++snapshotRequestSequenceRef.current;
    const epoch = providerActionEpochRef.current;
    const key = identityKey;
    snapshotRequestRef.current = { sequence, controller };
    setSnapshotLoading(true);
    setNotice('');
    try {
      const result = await listAppDevSnapshots({
        spaceId,
        projectId,
        signal: controller.signal,
      });
      if (
        controller.signal.aborted ||
        providerActionEpochRef.current !== epoch ||
        identityKey !== key ||
        snapshotRequestSequenceRef.current !== sequence
      ) {
        return;
      }
      setSnapshots(result.items);
    } catch (error) {
      if (
        !controller.signal.aborted &&
        providerActionEpochRef.current === epoch &&
        identityKey === key &&
        snapshotRequestSequenceRef.current === sequence
      ) {
        setNotice(normalizeAppDevError(error));
      }
    } finally {
      if (
        providerActionEpochRef.current === epoch &&
        identityKey === key &&
        snapshotRequestSequenceRef.current === sequence
      ) {
        setSnapshotLoading(false);
        if (snapshotRequestRef.current?.sequence === sequence) {
          snapshotRequestRef.current = undefined;
        }
      }
    }
  }, [identityKey, projectId, spaceId]);
  const openSnapshotModal = useCallback(() => {
    setSnapshotModalVisible(true);
    void refreshSnapshots();
  }, [refreshSnapshots]);
  const handleImportProject = useCallback(
    async (payload: { file: File; name?: string }) => {
      if (!spaceId) {
        throw new Error('空间不存在，请刷新后重试');
      }

      setImportingProject(true);
      setNotice('');

      try {
        const project = await importAppDevProject({
          spaceId,
          file: payload.file,
          name: payload.name,
        });
        setImportModalVisible(false);
        setNotice(`已导入「${project.name}」，正在进入开发工作台`);
        navigate(`/space/${spaceId}/app-dev/${project.id}`);
      } catch (error) {
        const message = normalizeAppDevError(error);
        setNotice(message);
        throw new Error(message);
      } finally {
        setImportingProject(false);
      }
    },
    [navigate, spaceId],
  );
  const openSnapshotNameModal = useCallback(() => {
    setSnapshotName('手动快照');
    setSnapshotNameError('');
    setSnapshotNameModalVisible(true);
  }, []);
  const createSnapshot = useCallback(async () => {
    const label = snapshotName.trim();
    if (!label) {
      setSnapshotNameError('快照名称不能为空');
      return;
    }
    setSnapshotActionLoading(true);
    setNotice('');
    setSnapshotNameError('');
    try {
      await createAppDevSnapshot({ spaceId, projectId, label });
      await refreshSnapshots();
      setSnapshotNameModalVisible(false);
      setNotice('快照已保存');
    } catch (error) {
      const message = normalizeAppDevError(error);
      setNotice(message);
      setSnapshotNameError(message);
    } finally {
      setSnapshotActionLoading(false);
    }
  }, [projectId, refreshSnapshots, snapshotName, spaceId]);
  const executeSnapshotRestore = useCallback(
    (
      operation: {
        operationId: string;
        snapshotId: string;
        phase?: 'pending' | 'requesting' | 'observed';
        createdAt?: number;
      },
      label?: string,
    ): Promise<void> => {
      const identity = { spaceId, projectId, principalId };
      const active = snapshotActionRef.current;
      if (active?.operationId === operation.operationId) {
        return active.promise;
      }
      active?.controller.abort();
      const controller = new AbortController();
      const epoch = providerActionEpochRef.current;
      const key = identityKey;
      setSnapshotActionLoading(true);
      setNotice('');
      const stored = writeAppDevPendingOperation(identity, 'snapshot-restore', {
        operationId: operation.operationId,
        snapshotId: operation.snapshotId,
        phase: 'requesting',
        createdAt: operation.createdAt,
      });
      if (stored.warning) {
        setNotice(stored.warning);
      }
      if (!stored.value) {
        return;
      }
      const promise = restoreAppDevSnapshot({
        ...identity,
        snapshotId: operation.snapshotId,
        operationId: operation.operationId,
        signal: controller.signal,
      })
        .then(async () => {
          if (
            controller.signal.aborted ||
            providerActionEpochRef.current !== epoch ||
            identityKey !== key
          ) {
            return;
          }
          const cleared = clearAppDevPendingOperation(
            identity,
            'snapshot-restore',
            operation.operationId,
          );
          if (cleared.warning) {
            setNotice(cleared.warning);
          }
          await fileState.refreshTree();
          if (
            controller.signal.aborted ||
            providerActionEpochRef.current !== epoch
          ) {
            return;
          }
          if (fileState.selectedPath) {
            await fileState.openFile(fileState.selectedPath);
          }
          if (
            controller.signal.aborted ||
            providerActionEpochRef.current !== epoch
          ) {
            return;
          }
          void runtimeState.refreshStatus();
          void runtimeState.refreshLogs();
          setPreviewRefreshSignal(signal => signal + 1);
          setReleaseStale(true);
          buildState.markStale();
          setActiveWorkspaceTab('preview');
          setNotice(
            label
              ? `已恢复到快照「${label}」，请重新发布产物`
              : '快照恢复已完成，请重新发布产物',
          );
          setSnapshotModalVisible(false);
        })
        .catch(error => {
          if (isAppDevDeterministicError(error)) {
            const cleared = clearAppDevPendingOperation(
              identity,
              'snapshot-restore',
              operation.operationId,
            );
            if (cleared.warning) {
              setNotice(cleared.warning);
            }
          }
          if (
            !controller.signal.aborted &&
            providerActionEpochRef.current === epoch &&
            identityKey === key
          ) {
            setNotice(normalizeAppDevError(error));
          }
        })
        .finally(() => {
          if (snapshotActionRef.current?.promise === promise) {
            snapshotActionRef.current = undefined;
            if (
              providerActionEpochRef.current === epoch &&
              identityKey === key
            ) {
              setSnapshotActionLoading(false);
            }
          }
        });
      snapshotActionRef.current = {
        operationId: operation.operationId,
        promise,
        controller,
      };
      return promise;
    },
    [
      buildState.markStale,
      fileState.openFile,
      fileState.refreshTree,
      fileState.selectedPath,
      identityKey,
      projectId,
      runtimeState.refreshLogs,
      runtimeState.refreshStatus,
      spaceId,
      principalId,
    ],
  );

  const restoreSnapshot = useCallback(
    async (snapshot: AppDevSnapshot) => {
      const confirmEpoch = providerActionEpochRef.current;
      const confirmIdentityKey = identityKey;
      const confirmed = await openConfirmDialog({
        title: '恢复快照',
        description: `确认恢复到「${snapshot.label}」吗？当前文件会被覆盖。`,
        confirmText: '恢复',
      });
      if (
        !confirmed ||
        providerActionEpochRef.current !== confirmEpoch ||
        identityKey !== confirmIdentityKey
      ) {
        return;
      }
      const identity = { spaceId, projectId, principalId };
      const pendingResult = readAppDevPendingOperationResult(
        identity,
        'snapshot-restore',
      );
      if (pendingResult.warning) {
        setNotice(pendingResult.warning);
      }
      const pending = pendingResult.value;
      const operationId =
        pending?.snapshotId === snapshot.id
          ? pending.operationId
          : createAppDevOperationID();
      if (pending && pending.snapshotId !== snapshot.id) {
        const cleared = clearAppDevPendingOperation(
          identity,
          'snapshot-restore',
          pending.operationId,
        );
        if (cleared.warning) {
          setNotice(cleared.warning);
        }
      }
      await executeSnapshotRestore(
        {
          operationId,
          snapshotId: snapshot.id,
          createdAt:
            pending?.snapshotId === snapshot.id ? pending.createdAt : undefined,
        },
        snapshot.label,
      );
    },
    [
      executeSnapshotRestore,
      identityKey,
      openConfirmDialog,
      principalId,
      projectId,
      spaceId,
    ],
  );

  useEffect(() => {
    if (!spaceId || !projectId) {
      return;
    }
    const pendingResult = readAppDevPendingOperationResult(
      { spaceId, projectId, principalId },
      'snapshot-restore',
    );
    if (pendingResult.warning) {
      setNotice(pendingResult.warning);
    }
    const pending = pendingResult.value;
    if (pending?.snapshotId) {
      void executeSnapshotRestore({
        operationId: pending.operationId,
        snapshotId: pending.snapshotId,
        phase: pending.phase,
        createdAt: pending.createdAt,
      });
    }
  }, [executeSnapshotRestore, identityKey, principalId, projectId, spaceId]);
  const openProjectMetaEditor = useCallback(() => {
    setProjectMetaForm({
      name: projectState.project?.name || '',
      description: projectState.project?.description || '',
    });
    setNotice('');
    setProjectMetaEditing(true);
  }, [projectState.project?.description, projectState.project?.name]);
  const saveProjectMeta = useCallback(async () => {
    const normalizedName = projectMetaForm.name.trim();
    if (!normalizedName) {
      setNotice('项目名称不能为空');
      return;
    }

    setSavingProjectMeta(true);
    setNotice('');
    try {
      await updateAppDevProject({
        spaceId,
        projectId,
        name: normalizedName,
        description: projectMetaForm.description.trim(),
      });
      await refreshProject();
      setProjectMetaEditing(false);
      setNotice('项目信息已更新');
    } catch (error) {
      setNotice(normalizeAppDevError(error));
    } finally {
      setSavingProjectMeta(false);
    }
  }, [
    projectId,
    projectMetaForm.description,
    projectMetaForm.name,
    refreshProject,
    spaceId,
  ]);
  const handleAppDevTaskSettled = useCallback(() => {
    void fileState.refreshTree();
    if (fileState.selectedPath && !fileState.dirty) {
      void fileState.openFile(fileState.selectedPath);
    }
    setPreviewRefreshSignal(signal => signal + 1);
    setReleaseStale(true);
    setNotice('AI 任务已完成，文件树、编辑器和预览已刷新，请重新发布产物');
  }, [
    fileState.dirty,
    fileState.openFile,
    fileState.refreshTree,
    fileState.selectedPath,
  ]);
  const handleSaveCurrentFile = useCallback(async () => {
    const saved = await fileState.saveCurrentFile();
    if (!saved) {
      return;
    }

    setPreviewRefreshSignal(signal => signal + 1);
    setReleaseStale(true);
    setActiveWorkspaceTab('preview');
    setNotice('文件已保存，预览已刷新；发布产物需重新发布后才是最新版本');
  }, [fileState]);
  const handleBeforeDesignExecute = useCallback(async () => {
    if (!spaceId || !projectId) {
      throw new Error('项目地址不完整，无法保存设计任务前快照');
    }

    setNotice('正在保存设计任务前快照...');
    const snapshotLabel = `设计任务前快照 ${new Date().toLocaleString('zh-CN', {
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    })}`;
    await createAppDevSnapshot({
      spaceId,
      projectId,
      label: snapshotLabel,
    });
    setNotice('已保存设计任务前快照，正在提交设计任务...');
  }, [projectId, spaceId]);
  const handleDesignModeChange = useCallback((state: AppDevDesignModeState) => {
    setDesignModeState(current =>
      current.active === state.active &&
      current.summary === state.summary &&
      current.prompt === state.prompt
        ? current
        : state,
    );
  }, []);
  const building = buildState.building;
  const canBuild = canBuildAppDevRuntime({
    runtime: runtimeState.runtime,
    runtimeLoading: runtimeState.loading,
    building,
  });
  const trustedPreviewUrl = normalizeAppDevPreviewUrl(
    runtimeState.runtime.previewUrl,
  );
  const buildStatus = buildState.projection.state;
  const releaseReady =
    buildStatus === 'ready' && buildState.projection.releaseAvailable;
  const sourceChangedAfterBuild = isAppDevReleaseStale({
    lastBuildStatus: releaseReady ? 'success' : buildStatus,
    sourceUpdatedAt: projectState.project?.sourceUpdatedAt,
    lastBuildAt: buildState.projection.updatedAt,
  });
  const effectiveReleaseStale =
    releaseStale || buildState.projection.stale || sourceChangedAfterBuild;
  const buildStatusLabel =
    effectiveReleaseStale && releaseReady
      ? '源码已更新'
      : buildStatus === 'building'
        ? '发布中'
        : buildStatus === 'ready'
          ? '已发布'
          : buildStatus === 'failed'
            ? '发布失败'
            : '未发布';
  const buildFinishedAt = buildState.projection.updatedAt || '';
  const buildSizeText = buildState.projection.size
    ? `${Math.max(1, Math.ceil(buildState.projection.size / 1024))} KB`
    : '';
  const buildFinishedAtText = (() => {
    if (!buildFinishedAt) {
      return '';
    }

    const date = new Date(buildFinishedAt);

    if (Number.isNaN(date.getTime())) {
      return buildFinishedAt;
    }

    return date.toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
    });
  })();
  const handleBuild = useCallback(
    async (_publishType = 'PAGE') => {
      setNotice('已提交发布任务，正在构建...');
      const result = await buildState.beginBuild();
      if (result?.state === 'building') {
        setNotice('发布任务正在后台构建，可继续编辑或查看预览');
      }
    },
    [buildState.beginBuild],
  );

  useEffect(() => {
    if (buildState.error) {
      setNotice(buildState.error);
      return;
    }
    if (buildState.projection.state === 'ready') {
      setReleaseStale(buildState.projection.stale);
      setNotice(
        buildState.projection.stale
          ? '发布已完成，但源码已更新，请重新发布后下载'
          : '发布构建完成，可以下载产物',
      );
      void refreshProject();
    } else if (buildState.projection.state === 'failed') {
      setNotice(buildState.projection.safeMessage || '发布构建失败，请重试');
    }
  }, [
    buildState.error,
    buildState.projection.safeMessage,
    buildState.projection.stale,
    buildState.projection.state,
    refreshProject,
  ]);
  const handleDownloadRelease = useCallback(async () => {
    if (!releaseReady || effectiveReleaseStale) {
      setNotice('当前发布产物不可下载，请先完成最新构建');
      return;
    }
    if (releaseActionRef.current) {
      return releaseActionRef.current.promise;
    }
    const controller = new AbortController();
    const epoch = providerActionEpochRef.current;
    const key = identityKey;
    setDownloadingRelease(true);
    setNotice('正在准备发布产物，请稍候...');
    const promise = downloadAppDevRelease({
      spaceId,
      projectId,
      signal: controller.signal,
    })
      .then(blob => {
        if (
          controller.signal.aborted ||
          providerActionEpochRef.current !== epoch ||
          identityKey !== key
        ) {
          return;
        }
        downloadAppDevBlob(
          blob,
          `${projectState.project?.name || 'appdev-project'}-release.zip`,
        );
        setNotice('发布产物已开始下载');
      })
      .catch(error => {
        if (
          controller.signal.aborted ||
          providerActionEpochRef.current !== epoch ||
          identityKey !== key
        ) {
          return;
        }
        if (isAppDevReleaseStaleError(error)) {
          setReleaseStale(true);
          buildState.markStale();
          void buildState.reconcile();
        }
        setNotice(normalizeAppDevError(error));
      })
      .finally(() => {
        if (releaseActionRef.current?.promise === promise) {
          releaseActionRef.current = undefined;
          if (providerActionEpochRef.current === epoch && identityKey === key) {
            setDownloadingRelease(false);
          }
        }
      });
    releaseActionRef.current = { promise, controller };
    return promise;
  }, [
    buildState.markStale,
    buildState.reconcile,
    effectiveReleaseStale,
    identityKey,
    projectId,
    projectState.project?.name,
    releaseReady,
    spaceId,
  ]);
  const handleExportSource = useCallback(async () => {
    if (exportActionRef.current) {
      return exportActionRef.current.promise;
    }
    const controller = new AbortController();
    const epoch = providerActionEpochRef.current;
    const key = identityKey;
    setExporting(true);
    setNotice('正在准备源码包，请稍候...');
    const promise = exportAppDevProject({
      spaceId,
      projectId,
      signal: controller.signal,
    })
      .then(blob => {
        if (
          controller.signal.aborted ||
          providerActionEpochRef.current !== epoch ||
          identityKey !== key
        ) {
          return;
        }
        downloadAppDevBlob(
          blob,
          `${projectState.project?.name || 'appdev-project'}.zip`,
        );
        setNotice('项目导出已开始下载');
      })
      .catch(error => {
        if (
          !controller.signal.aborted &&
          providerActionEpochRef.current === epoch &&
          identityKey === key
        ) {
          setNotice(normalizeAppDevError(error));
        }
      })
      .finally(() => {
        if (exportActionRef.current?.promise === promise) {
          exportActionRef.current = undefined;
          if (providerActionEpochRef.current === epoch && identityKey === key) {
            setExporting(false);
          }
        }
      });
    exportActionRef.current = { promise, controller };
    return promise;
  }, [identityKey, projectId, projectState.project?.name, spaceId]);

  return (
    <main className="app-dev-ide">
      <div className="app-dev-ide__shell">
        <header className="app-dev-ide__topbar">
          <button
            type="button"
            className="app-dev-ide__back-button"
            onClick={() => navigate(`/space/${spaceId}/app-dev`)}
            aria-label="返回网页应用列表"
          >
            ‹
          </button>
          <div className="app-dev-ide__project-icon" aria-hidden="true">
            &lt;/&gt;
          </div>
          <div className="app-dev-ide__project-info">
            <div>
              <h1>{projectState.project?.name || '网页应用项目'}</h1>
              <button
                type="button"
                disabled={!projectState.project || savingProjectMeta}
                onClick={openProjectMetaEditor}
                aria-label="编辑项目设置"
              >
                ✎
              </button>
            </div>
            <p>
              {projectState.project?.description ||
                '使用 AI 助手生成、编辑、预览和发布网页应用'}
            </p>
            <span>项目 ID：{projectId}</span>
            {projectState.loading ? <span>加载中...</span> : null}
            {projectState.error ? <span>{projectState.error}</span> : null}
          </div>
          <div className="app-dev-ide__top-actions">
            <button
              type="button"
              disabled={!projectState.project || snapshotActionLoading}
              onClick={openSnapshotNameModal}
            >
              保存快照
            </button>
            <button
              type="button"
              disabled={!projectState.project}
              onClick={openSnapshotModal}
            >
              版本历史
            </button>
          </div>
        </header>
        {notice ? (
          <div className="app-dev-ide__notice" role="status" aria-live="polite">
            {notice}
          </div>
        ) : null}
        <section className="app-dev-ide__section">
          <div className="app-dev-ide__main-row">
            <aside className="app-dev-ide__left-panel">
              <ChatPanel
                spaceId={spaceId}
                projectId={projectId}
                projectFiles={fileState.tree}
                hasPendingFileChanges={fileState.dirty}
                onTaskSettled={handleAppDevTaskSettled}
                onFilesChanged={() => {
                  void fileState.refreshTree();
                  setReleaseStale(true);
                }}
                openDesignSignal={openDesignSignal}
                designTargetOverride={selectedDesignTarget}
                designSelection={designSelection}
                onBeforeDesignExecute={handleBeforeDesignExecute}
                onDesignTargetChange={target => {
                  setSelectedDesignTarget(target);
                  setDesignSelection(null);
                }}
                onDesignModeChange={handleDesignModeChange}
              />
            </aside>
            <section className="app-dev-ide__right-panel">
              <header className="app-dev-ide__editor-header">
                <div
                  className="app-dev-ide__segmented-tabs"
                  role="tablist"
                  aria-label="网页应用工作区"
                >
                  <button
                    type="button"
                    role="tab"
                    aria-selected={activeWorkspaceTab === 'preview'}
                    data-active={activeWorkspaceTab === 'preview'}
                    onClick={() => setActiveWorkspaceTab('preview')}
                  >
                    <span aria-hidden="true">▣</span>
                    预览
                  </button>
                  <button
                    type="button"
                    role="tab"
                    aria-selected={activeWorkspaceTab === 'code'}
                    data-active={activeWorkspaceTab === 'code'}
                    onClick={() => setActiveWorkspaceTab('code')}
                  >
                    <span aria-hidden="true">{'</>'}</span>
                    代码
                  </button>
                </div>
                <div className="app-dev-ide__editor-actions">
                  <RuntimeToolbar
                    runtime={runtimeState.runtime}
                    trustedPreviewUrl={trustedPreviewUrl}
                    build={buildState.projection}
                    loading={runtimeState.loading || exporting}
                    building={building}
                    downloadingRelease={downloadingRelease}
                    releaseStale={effectiveReleaseStale}
                    importingProject={importingProject}
                    hasLogErrors={runtimeState.logs.some(
                      log => log.level === 'error',
                    )}
                    onStart={() => void runtimeState.start()}
                    onRestart={() => void runtimeState.restart()}
                    onStop={() => void runtimeState.stop()}
                    onRefresh={() => {
                      void runtimeState.refreshStatus();
                      void runtimeState.refreshLogs();
                      void buildState.reconcile();
                    }}
                    onOpenLogs={() => {
                      void runtimeState.refreshLogs();
                      setLogsVisible(true);
                    }}
                    onImportProject={() => setImportModalVisible(true)}
                    onFullscreenPreview={() => {
                      if (
                        runtimeState.runtime.status !== 'running' ||
                        !trustedPreviewUrl
                      ) {
                        return;
                      }
                      setActiveWorkspaceTab('preview');
                      setPreviewFullscreenSignal(signal => signal + 1);
                    }}
                    onBuild={publishType => void handleBuild(publishType)}
                    onDownloadRelease={() => void handleDownloadRelease()}
                    onExport={() => void handleExportSource()}
                  />
                  <button
                    type="button"
                    className="app-dev-ide__console-button"
                    data-active={logsVisible}
                    onClick={() => {
                      if (!logsVisible) {
                        void runtimeState.refreshLogs();
                      }
                      setLogsVisible(visible => !visible);
                    }}
                  >
                    开发日志
                  </button>
                </div>
              </header>
              <section
                className="app-dev-release-panel"
                data-status={buildStatus}
                data-stale={
                  effectiveReleaseStale && releaseReady ? 'true' : 'false'
                }
              >
                <div className="app-dev-release-panel__summary">
                  <span>发布概览</span>
                  <strong>{buildStatusLabel}</strong>
                  <p>
                    {effectiveReleaseStale && releaseReady
                      ? '源码已更新，预览已刷新。请重新发布后再下载最新产物。'
                      : buildState.projection.safeMessage ||
                        '发布后可下载静态产物，也可以随时导出当前源码包。'}
                  </p>
                </div>
                <div className="app-dev-release-panel__meta">
                  <span>类型：静态发布包</span>
                  {buildFinishedAtText ? (
                    <span>完成时间：{buildFinishedAtText}</span>
                  ) : null}
                  {buildSizeText ? <span>大小：{buildSizeText}</span> : null}
                </div>
                <div className="app-dev-release-panel__actions">
                  <button
                    type="button"
                    onClick={() => void handleBuild()}
                    disabled={!canBuild}
                  >
                    {building ? '发布中...' : '重新发布'}
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleDownloadRelease()}
                    disabled={
                      effectiveReleaseStale ||
                      !releaseReady ||
                      downloadingRelease ||
                      runtimeState.loading
                    }
                  >
                    {downloadingRelease ? '下载中...' : '下载产物'}
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleExportSource()}
                    disabled={exporting || runtimeState.loading}
                  >
                    {exporting ? '导出中...' : '导出源码'}
                  </button>
                </div>
              </section>
              <div className="app-dev-ide__right-main">
                <div className="app-dev-ide__content-area">
                  <div
                    className="app-dev-ide__content-row"
                    data-mode={activeWorkspaceTab}
                  >
                    {activeWorkspaceTab === 'code' ? (
                      <div
                        className="app-dev-ide__file-sidebar"
                        data-collapsed={fileTreeCollapsed}
                      >
                        <button
                          type="button"
                          className="app-dev-ide__file-sidebar-toggle"
                          onClick={() =>
                            setFileTreeCollapsed(collapsed => !collapsed)
                          }
                          aria-label={
                            fileTreeCollapsed ? '展开文件树' : '收起文件树'
                          }
                        >
                          {fileTreeCollapsed ? '›' : '‹'}
                        </button>
                        {!fileTreeCollapsed ? (
                          <FileTree
                            tree={fileState.tree}
                            selectedPath={fileState.selectedPath}
                            loading={fileState.loadingTree || fileState.saving}
                            onRefresh={() => void fileState.refreshTree()}
                            onOpenFile={path => void fileState.openFile(path)}
                            onCreateFile={path =>
                              void fileState.createFile(path)
                            }
                            onCreateDirectory={path =>
                              void fileState.createDirectory(path)
                            }
                            onUploadFiles={(files, options) =>
                              void fileState.uploadFiles(files, options)
                            }
                            onRenamePath={(sourcePath, targetPath) =>
                              void fileState.renamePath(sourcePath, targetPath)
                            }
                            onDeletePath={path =>
                              void fileState.deletePath(path)
                            }
                          />
                        ) : null}
                      </div>
                    ) : null}
                    <div className="app-dev-ide__editor-col">
                      {activeWorkspaceTab === 'preview' ? (
                        <PreviewPanel
                          runtime={runtimeState.runtime}
                          trustedPreviewUrl={trustedPreviewUrl}
                          onRefresh={() => void runtimeState.refreshStatus()}
                          onOpenLogs={() => {
                            void runtimeState.refreshLogs();
                            setLogsVisible(true);
                          }}
                          refreshSignal={previewRefreshSignal}
                          fullscreenSignal={previewFullscreenSignal}
                          designModeActive={designModeState.active}
                          designSummary={designModeState.summary}
                          designTarget={selectedDesignTarget}
                          designSelection={designSelection}
                          designTargetOptions={APP_DEV_DESIGN_TARGETS}
                          onOpenDesignMode={() => {
                            setActiveWorkspaceTab('preview');
                            setOpenDesignSignal(signal => signal + 1);
                          }}
                          onSelectDesignTarget={target => {
                            setSelectedDesignTarget(target);
                            setDesignSelection(null);
                            setActiveWorkspaceTab('preview');
                            setOpenDesignSignal(signal => signal + 1);
                          }}
                          onSelectPreviewPoint={selection => {
                            setSelectedDesignTarget(selection.target);
                            setDesignSelection(selection);
                            setActiveWorkspaceTab('preview');
                            setOpenDesignSignal(signal => signal + 1);
                          }}
                        />
                      ) : (
                        <CodeEditor
                          path={fileState.selectedPath}
                          value={fileState.draft}
                          dirty={fileState.dirty}
                          loading={fileState.loadingContent}
                          saving={fileState.saving}
                          onChange={fileState.setDraft}
                          onSave={() => void handleSaveCurrentFile()}
                        />
                      )}
                    </div>
                  </div>
                  {fileState.error ? (
                    <div className="app-dev-ide__inline-error">
                      {fileState.error}
                    </div>
                  ) : null}
                  {runtimeState.error ? (
                    <div className="app-dev-ide__runtime-error">
                      {runtimeState.error}
                    </div>
                  ) : null}
                </div>
                {logsVisible ? (
                  <div className="app-dev-ide__bottom-console">
                    <DevLogsPanel
                      logs={runtimeState.logs}
                      loading={runtimeState.logsLoading}
                      onRefresh={() => void runtimeState.refreshLogs()}
                    />
                  </div>
                ) : null}
              </div>
            </section>
          </div>
        </section>
      </div>

      {projectMetaEditing ? (
        <AccessibleDialog
          title="项目设置"
          className="app-dev-modal app-dev-modal--compact app-dev-project-settings-modal"
          onClose={() => {
            if (!savingProjectMeta) {
              setProjectMetaEditing(false);
            }
          }}
        >
          <header className="app-dev-modal__header">
            <div>
              <h2>项目设置</h2>
              <p>修改当前网页应用的名称和说明，保存后会同步到项目列表。</p>
            </div>
            <button
              type="button"
              disabled={savingProjectMeta}
              onClick={() => setProjectMetaEditing(false)}
            >
              关闭
            </button>
          </header>
          <label className="app-dev-form-field">
            <span>项目名称</span>
            <input
              data-dialog-autofocus
              maxLength={50}
              value={projectMetaForm.name}
              disabled={savingProjectMeta}
              onChange={event =>
                setProjectMetaForm(current => ({
                  ...current,
                  name: event.target.value,
                }))
              }
              placeholder="请输入项目名称"
            />
          </label>
          <label className="app-dev-form-field">
            <span>项目描述</span>
            <textarea
              maxLength={200}
              value={projectMetaForm.description}
              disabled={savingProjectMeta}
              onChange={event =>
                setProjectMetaForm(current => ({
                  ...current,
                  description: event.target.value,
                }))
              }
              placeholder="描述这个网页应用的用途、场景或交付目标"
            />
          </label>
          <footer className="app-dev-modal__footer">
            <button
              type="button"
              disabled={savingProjectMeta}
              onClick={() => setProjectMetaEditing(false)}
            >
              取消
            </button>
            <button
              type="button"
              disabled={savingProjectMeta}
              onClick={() => void saveProjectMeta()}
            >
              {savingProjectMeta ? '保存中...' : '保存'}
            </button>
          </footer>
        </AccessibleDialog>
      ) : null}

      {snapshotModalVisible ? (
        <AccessibleDialog
          title="版本历史"
          className="app-dev-modal app-dev-modal--compact app-dev-snapshot-modal"
          onClose={() => {
            if (!snapshotActionLoading) {
              setSnapshotModalVisible(false);
            }
          }}
        >
          <header className="app-dev-modal__header">
            <div>
              <h2>版本历史</h2>
              <p>保存和恢复当前网页应用的文件快照，方便在迭代中回退。</p>
            </div>
            <button
              type="button"
              disabled={snapshotActionLoading}
              onClick={() => setSnapshotModalVisible(false)}
            >
              关闭
            </button>
          </header>
          <div className="app-dev-snapshot-modal__toolbar">
            <button
              type="button"
              disabled={snapshotLoading || snapshotActionLoading}
              onClick={() => void refreshSnapshots()}
            >
              刷新
            </button>
            <button
              type="button"
              disabled={snapshotActionLoading}
              onClick={openSnapshotNameModal}
            >
              保存新快照
            </button>
          </div>
          {snapshotLoading ? (
            <div className="app-dev-snapshot-modal__state">加载快照中...</div>
          ) : null}
          {!snapshotLoading && !snapshots.length ? (
            <div className="app-dev-snapshot-modal__state">
              暂无快照，可以先保存一个当前版本。
            </div>
          ) : null}
          {!snapshotLoading && snapshots.length ? (
            <ol className="app-dev-snapshot-modal__list">
              {snapshots.map(snapshot => (
                <li key={snapshot.id}>
                  <div>
                    <strong>{snapshot.label}</strong>
                    <span>{snapshot.createdAt || snapshot.id}</span>
                  </div>
                  <button
                    type="button"
                    disabled={snapshotActionLoading}
                    onClick={() => void restoreSnapshot(snapshot)}
                  >
                    恢复
                  </button>
                </li>
              ))}
            </ol>
          ) : null}
        </AccessibleDialog>
      ) : null}
      {snapshotNameModalVisible ? (
        <AccessibleDialog
          title="保存快照"
          maskClassName="app-dev-modal-mask app-dev-modal-mask--nested"
          onClose={() => {
            if (!snapshotActionLoading) {
              setSnapshotNameModalVisible(false);
            }
          }}
        >
          <header className="app-dev-modal__header">
            <div>
              <h2>保存快照</h2>
              <p>为当前网页应用文件创建一个可恢复版本。</p>
            </div>
            <button
              type="button"
              disabled={snapshotActionLoading}
              onClick={() => setSnapshotNameModalVisible(false)}
            >
              关闭
            </button>
          </header>
          <label className="app-dev-form-field">
            <span>快照名称</span>
            <input
              data-dialog-autofocus
              value={snapshotName}
              disabled={snapshotActionLoading}
              placeholder="例如：完成首页首版"
              onChange={event => {
                setSnapshotName(event.target.value);
                setSnapshotNameError('');
              }}
            />
          </label>
          {snapshotNameError ? (
            <div className="app-dev-form-error">{snapshotNameError}</div>
          ) : null}
          <footer className="app-dev-modal__footer">
            <button
              type="button"
              disabled={snapshotActionLoading}
              onClick={() => setSnapshotNameModalVisible(false)}
            >
              取消
            </button>
            <button
              type="button"
              disabled={snapshotActionLoading}
              onClick={() => void createSnapshot()}
            >
              {snapshotActionLoading ? '保存中...' : '保存'}
            </button>
          </footer>
        </AccessibleDialog>
      ) : null}
      <ImportProjectModal
        visible={importModalVisible}
        loading={importingProject}
        onCancel={() => setImportModalVisible(false)}
        onSubmit={handleImportProject}
      />
      {confirmDialog}
    </main>
  );
}
