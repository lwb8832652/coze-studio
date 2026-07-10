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
import { useCallback, useState } from 'react';

import { isAppDevReleaseStale } from './utils/release-status';
import type { AppDevBuildResult, AppDevSnapshot } from './types';
import {
  buildAppDevProject,
  createAppDevSnapshot,
  downloadAppDevRelease,
  exportAppDevProject,
  importAppDevProject,
  listAppDevSnapshots,
  normalizeAppDevError,
  restoreAppDevSnapshot,
  updateAppDevProject,
} from './service';
import { useAppDevRuntime } from './hooks/use-app-dev-runtime';
import { useAppDevProjectInfo } from './hooks/use-app-dev-project-info';
import { useAppDevFiles } from './hooks/use-app-dev-files';
import { useConfirmDialog } from './components/text-input-dialog';
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

export default function AppDevIDEPage() {
  const navigate = useNavigate();
  const { space_id: spaceId = '', project_id: projectId = '' } = useParams();
  const projectState = useAppDevProjectInfo(spaceId, projectId);
  const refreshProject = projectState.refresh;
  const fileState = useAppDevFiles(spaceId, projectId);
  const runtimeState = useAppDevRuntime(spaceId, projectId);
  const [exporting, setExporting] = useState(false);
  const [building, setBuilding] = useState(false);
  const [downloadingRelease, setDownloadingRelease] = useState(false);
  const [importModalVisible, setImportModalVisible] = useState(false);
  const [importingProject, setImportingProject] = useState(false);
  const [lastBuildResult, setLastBuildResult] =
    useState<AppDevBuildResult | null>(null);
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
  const { openConfirmDialog, confirmDialog } = useConfirmDialog();
  const refreshSnapshots = useCallback(async () => {
    if (!spaceId || !projectId) {
      setSnapshots([]);
      return;
    }
    setSnapshotLoading(true);
    setNotice('');
    try {
      const result = await listAppDevSnapshots({ spaceId, projectId });
      setSnapshots(result.items);
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '快照列表加载失败');
    } finally {
      setSnapshotLoading(false);
    }
  }, [projectId, spaceId]);
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
      const message = error instanceof Error ? error.message : '保存快照失败';
      setNotice(message);
      setSnapshotNameError(message);
    } finally {
      setSnapshotActionLoading(false);
    }
  }, [projectId, refreshSnapshots, snapshotName, spaceId]);
  const restoreSnapshot = useCallback(
    async (snapshot: AppDevSnapshot) => {
      const confirmed = await openConfirmDialog({
        title: '恢复快照',
        description: `确认恢复到「${snapshot.label}」吗？当前文件会被覆盖。`,
        confirmText: '恢复',
      });
      if (!confirmed) {
        return;
      }
      setSnapshotActionLoading(true);
      setNotice('');
      try {
        await restoreAppDevSnapshot({
          spaceId,
          projectId,
          snapshotId: snapshot.id,
        });
        await fileState.refreshTree();
        if (fileState.selectedPath) {
          await fileState.openFile(fileState.selectedPath);
        }
        void runtimeState.refreshStatus();
        void runtimeState.refreshLogs();
        setPreviewRefreshSignal(signal => signal + 1);
        setReleaseStale(true);
        setActiveWorkspaceTab('preview');
        setNotice(`已恢复到快照「${snapshot.label}」，请重新发布产物`);
        setSnapshotModalVisible(false);
      } catch (error) {
        setNotice(error instanceof Error ? error.message : '恢复快照失败');
      } finally {
        setSnapshotActionLoading(false);
      }
    },
    [fileState, openConfirmDialog, projectId, spaceId],
  );
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
      setNotice(error instanceof Error ? error.message : '项目信息更新失败');
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
  const buildStatus = building
    ? 'building'
    : projectState.project?.lastBuildStatus ||
      lastBuildResult?.status ||
      'idle';
  const releaseReady =
    lastBuildResult?.status === 'success' ||
    projectState.project?.lastBuildStatus === 'success';
  const sourceChangedAfterBuild = isAppDevReleaseStale({
    lastBuildStatus: releaseReady ? 'success' : buildStatus,
    sourceUpdatedAt: projectState.project?.sourceUpdatedAt,
    lastBuildAt:
      lastBuildResult?.finishedAt || projectState.project?.lastBuildAt,
  });
  const effectiveReleaseStale = releaseStale || sourceChangedAfterBuild;
  const buildStatusLabel =
    effectiveReleaseStale && releaseReady
      ? '源码已更新'
      : buildStatus === 'building'
        ? '发布中'
        : buildStatus === 'success'
          ? '已发布'
          : buildStatus === 'error'
            ? '发布失败'
            : '未发布';
  const buildArtifactPath =
    lastBuildResult?.artifactPath ||
    projectState.project?.lastBuildArtifact ||
    '';
  const buildFinishedAt =
    lastBuildResult?.finishedAt || projectState.project?.lastBuildAt || '';
  const buildDurationText = lastBuildResult?.durationMs
    ? `${Math.max(1, Math.round(lastBuildResult.durationMs / 1000))} 秒`
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
  const formatPublishType = (publishType?: string) => {
    switch (publishType) {
      case 'AGENT':
        return '应用';
      case 'PAGE':
      case 'page':
        return '页面';
      default:
        return publishType || '页面';
    }
  };
  const handleBuild = useCallback(
    async (publishType = 'PAGE') => {
      const publishTypeLabel = formatPublishType(publishType);
      setBuilding(true);
      setNotice(`正在发布为${publishTypeLabel}，请稍候...`);
      try {
        const result = await buildAppDevProject({
          spaceId,
          projectId,
          publishType,
        });
        setLastBuildResult(result);
        await fileState.refreshTree();
        await refreshProject();
        setReleaseStale(false);
        setNotice(
          `发布为${formatPublishType(result.publishType)}成功，产物目录：${result.artifactPath}，耗时 ${Math.max(
            1,
            Math.round(result.durationMs / 1000),
          )} 秒`,
        );
      } catch (error) {
        await refreshProject();
        setNotice(error instanceof Error ? error.message : '发布构建失败');
      } finally {
        setBuilding(false);
      }
    },
    [fileState.refreshTree, projectId, refreshProject, spaceId],
  );
  const handleDownloadRelease = useCallback(async () => {
    setDownloadingRelease(true);
    setNotice('正在准备发布产物，请稍候...');
    try {
      const blob = await downloadAppDevRelease({
        spaceId,
        projectId,
      });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${projectState.project?.name || 'appdev-project'}-release.zip`;
      link.rel = 'noreferrer';
      link.style.display = 'none';
      document.body.appendChild(link);
      link.click();
      window.setTimeout(() => {
        URL.revokeObjectURL(url);
        link.remove();
      }, 0);
      setNotice('发布产物已开始下载');
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '发布产物下载失败');
    } finally {
      setDownloadingRelease(false);
    }
  }, [projectId, projectState.project?.name, spaceId]);
  const handleExportSource = useCallback(async () => {
    setExporting(true);
    setNotice('正在准备源码包，请稍候...');
    try {
      const blob = await exportAppDevProject({
        spaceId,
        projectId,
      });
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = `${projectState.project?.name || 'appdev-project'}.zip`;
      link.rel = 'noreferrer';
      link.style.display = 'none';
      document.body.appendChild(link);
      link.click();
      window.setTimeout(() => {
        URL.revokeObjectURL(url);
        link.remove();
      }, 0);
      setNotice('项目导出已开始下载');
    } catch (error) {
      setNotice(error instanceof Error ? error.message : '导出失败');
    } finally {
      setExporting(false);
    }
  }, [projectId, projectState.project?.name, spaceId]);

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
        {notice ? <div className="app-dev-ide__notice">{notice}</div> : null}
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
                    project={projectState.project}
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
                    }}
                    onOpenLogs={() => {
                      void runtimeState.refreshLogs();
                      setLogsVisible(true);
                    }}
                    onImportProject={() => setImportModalVisible(true)}
                    onFullscreenPreview={() => {
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
                      : buildArtifactPath
                        ? `产物目录：${buildArtifactPath}`
                        : '发布后可下载静态产物，也可以随时导出当前源码包。'}
                  </p>
                </div>
                <div className="app-dev-release-panel__meta">
                  <span>
                    类型：
                    {formatPublishType(
                      lastBuildResult?.publishType ||
                        projectState.project?.lastBuildType,
                    )}
                  </span>
                  {buildFinishedAtText ? (
                    <span>完成时间：{buildFinishedAtText}</span>
                  ) : null}
                  {buildDurationText ? (
                    <span>耗时：{buildDurationText}</span>
                  ) : null}
                </div>
                <div className="app-dev-release-panel__actions">
                  <button
                    type="button"
                    onClick={() => void handleBuild()}
                    disabled={building || runtimeState.loading}
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
        <div className="app-dev-modal-mask">
          <section className="app-dev-modal app-dev-modal--compact app-dev-project-settings-modal">
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
          </section>
        </div>
      ) : null}

      {snapshotModalVisible ? (
        <div className="app-dev-modal-mask">
          <section className="app-dev-modal app-dev-modal--compact app-dev-snapshot-modal">
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
          </section>
        </div>
      ) : null}
      {snapshotNameModalVisible ? (
        <div className="app-dev-modal-mask app-dev-modal-mask--nested">
          <section className="app-dev-modal app-dev-modal--compact">
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
          </section>
        </div>
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
