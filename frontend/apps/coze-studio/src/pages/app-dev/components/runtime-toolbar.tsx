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

/* eslint-disable @coze-arch/max-line-per-function -- Cohesive orchestrator. */
/* eslint-disable complexity -- Cohesive orchestrator. */

import type { AppDevProject, AppDevRuntimeInfo } from '../types';

const STATUS_LABELS: Record<AppDevRuntimeInfo['status'], string> = {
  stopped: '未启动',
  starting: '启动中',
  running: '运行中',
  restarting: '重启中',
  error: '异常',
};

const CONNECTION_LABELS: Record<AppDevRuntimeInfo['status'], string> = {
  stopped: '服务未启动',
  starting: '环境准备中',
  running: '服务已连接',
  restarting: '环境重启中',
  error: '服务异常',
};

const formatRuntimeTime = (value?: string) => {
  if (!value) {
    return '';
  }

  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
  });
};

interface RuntimeToolbarProps {
  runtime: AppDevRuntimeInfo;
  project?: AppDevProject;
  loading?: boolean;
  building?: boolean;
  downloadingRelease?: boolean;
  releaseStale?: boolean;
  importingProject?: boolean;
  hasLogErrors?: boolean;
  onStart: () => void;
  onRestart: () => void;
  onStop: () => void;
  onRefresh: () => void;
  onOpenLogs?: () => void;
  onImportProject?: () => void;
  onFullscreenPreview?: () => void;
  onBuild: (publishType?: string) => void;
  onDownloadRelease: () => void;
  onExport: () => void;
}

export const RuntimeToolbar = ({
  runtime,
  project,
  loading,
  building,
  downloadingRelease,
  releaseStale,
  importingProject,
  hasLogErrors,
  onStart,
  onRestart,
  onStop,
  onRefresh,
  onOpenLogs,
  onImportProject,
  onFullscreenPreview,
  onBuild,
  onDownloadRelease,
  onExport,
}: RuntimeToolbarProps) => {
  const runtimeBusy =
    loading || runtime.status === 'starting' || runtime.status === 'restarting';
  const releaseReady = project?.lastBuildStatus === 'success';
  const downloadReleaseDisabled =
    loading || downloadingRelease || !releaseReady || releaseStale;
  const lastKeepAliveLabel = formatRuntimeTime(runtime.lastKeepAliveAt);
  const previewReady =
    runtime.status === 'running' && Boolean(runtime.previewUrl);

  return (
    <div className="app-dev-runtime-toolbar">
      <div>
        <span
          className="app-dev-runtime-toolbar__status"
          data-status={runtime.status}
        >
          {STATUS_LABELS[runtime.status]}
        </span>
        <div className="app-dev-runtime-toolbar__connection-row">
          <span
            className="app-dev-runtime-toolbar__connection"
            data-status={runtime.status}
          >
            {CONNECTION_LABELS[runtime.status]}
          </span>
          {lastKeepAliveLabel ? (
            <small>最近心跳 {lastKeepAliveLabel}</small>
          ) : null}
        </div>
        {runtime.message ? <small>{runtime.message}</small> : null}
        {project?.lastBuildStatus ? (
          <small>
            发布状态：
            {project.lastBuildStatus === 'building'
              ? '构建中'
              : project.lastBuildStatus === 'success'
                ? '已构建'
                : '构建失败'}
            {project.lastBuildAt ? ` · ${project.lastBuildAt}` : ''}
          </small>
        ) : null}
        {project?.lastBuildMessage ? (
          <small>{project.lastBuildMessage}</small>
        ) : null}
      </div>
      <div className="app-dev-runtime-toolbar__actions">
        {runtime.status === 'running' ? (
          <>
            <button
              type="button"
              className="app-dev-runtime-toolbar__primary-action"
              onClick={onRestart}
              disabled={runtimeBusy}
            >
              重启
            </button>
            <button type="button" onClick={onStop} disabled={loading}>
              停止
            </button>
          </>
        ) : runtime.status === 'starting' || runtime.status === 'restarting' ? (
          <>
            <button type="button" disabled>
              {runtime.status === 'restarting' ? '重启中...' : '启动中...'}
            </button>
            <button type="button" onClick={onStop} disabled={loading}>
              停止
            </button>
          </>
        ) : (
          <button
            type="button"
            className="app-dev-runtime-toolbar__primary-action"
            onClick={onStart}
            disabled={loading}
          >
            {loading ? '启动中...' : '启动环境'}
          </button>
        )}
        <button type="button" onClick={onRefresh} disabled={loading}>
          刷新
        </button>
        {onOpenLogs ? (
          <button
            type="button"
            className="app-dev-runtime-toolbar__logs-action"
            data-has-error={hasLogErrors ? 'true' : 'false'}
            onClick={onOpenLogs}
            disabled={loading}
          >
            日志
            {hasLogErrors ? <span aria-label="存在错误日志" /> : null}
          </button>
        ) : null}
        <button
          type="button"
          className="app-dev-runtime-toolbar__publish-action"
          onClick={() => onBuild('PAGE')}
          disabled={loading || building}
        >
          {building ? '发布中...' : '发布'}
        </button>
        <details className="app-dev-runtime-toolbar__more">
          <summary aria-label="更多操作">更多</summary>
          <div>
            {onImportProject ? (
              <button
                type="button"
                onClick={onImportProject}
                disabled={loading || importingProject}
              >
                {importingProject ? '导入中...' : '导入项目'}
              </button>
            ) : null}
            {onFullscreenPreview ? (
              <button
                type="button"
                onClick={onFullscreenPreview}
                disabled={loading || !previewReady}
              >
                全屏预览
              </button>
            ) : null}
            <button
              type="button"
              onClick={() => onBuild('PAGE')}
              disabled={loading || building}
            >
              发布为页面
            </button>
            <button
              type="button"
              onClick={() => onBuild('AGENT')}
              disabled={loading || building}
            >
              发布为应用
            </button>
            <button
              type="button"
              onClick={onDownloadRelease}
              disabled={downloadReleaseDisabled}
            >
              {releaseStale
                ? '需重新发布'
                : downloadingRelease
                  ? '下载中...'
                  : '下载产物'}
            </button>
            <button type="button" onClick={onExport} disabled={loading}>
              导出源码
            </button>
          </div>
        </details>
      </div>
    </div>
  );
};
