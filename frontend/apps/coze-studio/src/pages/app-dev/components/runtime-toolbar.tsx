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

/* eslint-disable @coze-arch/max-line-per-function, complexity -- Lifecycle controls are rendered together. */

import type { AppDevBuildInfo, AppDevRuntimeInfo } from '../types';
import { canBuildAppDevRuntime } from '../utils/runtime-capabilities';
import type { TrustedAppDevPreviewURL } from '../utils/preview-url';

const STATUS_LABELS: Record<AppDevRuntimeInfo['status'], string> = {
  stopped: '未启动',
  starting: '启动中',
  running: '运行中',
  recovering: '恢复中',
  stopping: '停止中',
  cleanup_pending: '清理中',
  error: '异常',
};

const CONNECTION_LABELS: Record<AppDevRuntimeInfo['status'], string> = {
  stopped: '服务未启动',
  starting: '环境准备中',
  running: '服务已连接',
  recovering: '正在恢复服务状态',
  stopping: '正在停止服务',
  cleanup_pending: '正在清理运行资源',
  error: '服务异常',
};

const BUILD_LABELS: Record<AppDevBuildInfo['state'], string> = {
  idle: '未发布',
  building: '构建中',
  ready: '已发布',
  failed: '构建失败',
};

const formatRuntimeTime = (value?: string) => {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? value
    : date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
};

interface RuntimeToolbarProps {
  runtime: Omit<AppDevRuntimeInfo, 'previewUrl'>;
  trustedPreviewUrl?: TrustedAppDevPreviewURL;
  build: AppDevBuildInfo;
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
  trustedPreviewUrl,
  build,
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
  const lifecycleLocked = [
    'recovering',
    'stopping',
    'cleanup_pending',
  ].includes(runtime.status);
  const runtimeBusy = Boolean(
    loading || runtime.status === 'starting' || lifecycleLocked,
  );
  const releaseReady = build.state === 'ready' && build.releaseAvailable;
  const effectiveStale = Boolean(releaseStale || build.stale);
  const downloadReleaseDisabled = Boolean(
    runtimeBusy || downloadingRelease || !releaseReady || effectiveStale,
  );
  const lastKeepAliveLabel = formatRuntimeTime(runtime.lastKeepAliveAt);
  const previewReady =
    runtime.status === 'running' && Boolean(trustedPreviewUrl);
  const buildDisabled = !canBuildAppDevRuntime({
    runtime,
    runtimeLoading: loading,
    building,
  });

  return (
    <div className="app-dev-runtime-toolbar">
      <div role="status" aria-live="polite">
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
        <small>
          发布状态：{BUILD_LABELS[build.state]}
          {build.stale ? ' · 已过期' : ''}
        </small>
        {build.safeMessage ? <small>{build.safeMessage}</small> : null}
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
            <button type="button" onClick={onStop} disabled={runtimeBusy}>
              停止
            </button>
          </>
        ) : runtime.status === 'starting' || lifecycleLocked ? (
          <button type="button" disabled>
            {runtime.status === 'starting'
              ? '启动中...'
              : runtime.status === 'recovering'
                ? '恢复中...'
                : runtime.status === 'cleanup_pending'
                  ? '清理中...'
                  : '停止中...'}
          </button>
        ) : (
          <button
            type="button"
            className="app-dev-runtime-toolbar__primary-action"
            onClick={onStart}
            disabled={Boolean(loading || !runtime.canStart)}
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
            disabled={runtimeBusy}
          >
            日志
            {hasLogErrors ? <span aria-label="存在错误日志" /> : null}
          </button>
        ) : null}
        <button
          type="button"
          className="app-dev-runtime-toolbar__publish-action"
          onClick={() => onBuild('PAGE')}
          disabled={buildDisabled}
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
                disabled={runtimeBusy || importingProject}
              >
                {importingProject ? '导入中...' : '导入项目'}
              </button>
            ) : null}
            {onFullscreenPreview ? (
              <button
                type="button"
                onClick={onFullscreenPreview}
                disabled={runtimeBusy || !previewReady}
              >
                全屏预览
              </button>
            ) : null}
            <button
              type="button"
              onClick={() => onBuild('PAGE')}
              disabled={buildDisabled}
            >
              发布为页面
            </button>
            <button
              type="button"
              onClick={() => onBuild('AGENT')}
              disabled={buildDisabled}
            >
              发布为应用
            </button>
            <button
              type="button"
              onClick={onDownloadRelease}
              disabled={downloadReleaseDisabled}
            >
              {effectiveStale
                ? '需重新发布'
                : downloadingRelease
                  ? '下载中...'
                  : '下载产物'}
            </button>
            <button type="button" onClick={onExport} disabled={runtimeBusy}>
              导出源码
            </button>
          </div>
        </details>
      </div>
    </div>
  );
};
