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

import { useEffect, useState } from 'react';

import type { TrustedAppDevPreviewURL } from '../utils/preview-url';
import type { AppDevDesignSelection } from './chat-panel';
import type { AppDevRuntimeInfo } from '../types';
import { AccessibleDialog } from './accessible-dialog';

interface PreviewPanelProps {
  runtime: Omit<AppDevRuntimeInfo, 'previewUrl'>;
  trustedPreviewUrl?: TrustedAppDevPreviewURL;
  onRefresh: () => void;
  onOpenLogs?: () => void;
  refreshSignal?: number;
  fullscreenSignal?: number;
  designModeActive?: boolean;
  designSummary?: string;
  designTarget?: string;
  designSelection?: AppDevDesignSelection | null;
  designTargetOptions?: string[];
  onOpenDesignMode?: () => void;
  onSelectDesignTarget?: (target: string) => void;
  onSelectPreviewPoint?: (selection: AppDevDesignSelection) => void;
}

const RUNTIME_STEPS: Array<{
  key: AppDevRuntimeInfo['status'];
  title: string;
  description: string;
}> = [
  {
    key: 'starting',
    title: '准备开发环境',
    description: '安装依赖并启动本地开发服务',
  },
  {
    key: 'running',
    title: '打开实时预览',
    description: '服务启动后会自动加载预览地址',
  },
  {
    key: 'error',
    title: '检查异常日志',
    description: '查看开发日志并按提示重新启动',
  },
];

const PREVIEW_SANDBOX =
  'allow-scripts allow-same-origin allow-forms allow-modals allow-popups allow-downloads';

export const PreviewPanel = ({
  runtime,
  trustedPreviewUrl,
  onRefresh,
  onOpenLogs,
  refreshSignal,
  fullscreenSignal,
  designModeActive = false,
  designSummary = '现代 SaaS · 全局视觉系统',
  designTarget = '全局视觉系统',
  designSelection,
  designTargetOptions = [],
  onOpenDesignMode,
  onSelectDesignTarget,
  onSelectPreviewPoint,
}: PreviewPanelProps) => {
  const [refreshKey, setRefreshKey] = useState(0);
  const [fullscreen, setFullscreen] = useState(false);
  const [device, setDevice] = useState<'desktop' | 'tablet' | 'mobile'>(
    'desktop',
  );
  const [pickingElement, setPickingElement] = useState(false);
  const previewUrl = trustedPreviewUrl;
  const refreshPreview = () => {
    setRefreshKey(key => key + 1);
    onRefresh();
  };

  useEffect(() => {
    if (!refreshSignal) {
      return;
    }

    setRefreshKey(key => key + 1);
  }, [refreshSignal]);

  const openFullscreen = (_trigger?: HTMLElement) => setFullscreen(true);

  const closeFullscreen = () => setFullscreen(false);

  useEffect(() => {
    if (!fullscreenSignal || runtime.status !== 'running' || !previewUrl) {
      return;
    }

    openFullscreen();
  }, [fullscreenSignal, previewUrl, runtime.status]);

  if (runtime.status !== 'running' || !previewUrl) {
    const stateCopy: Record<
      Exclude<AppDevRuntimeInfo['status'], 'running'>,
      { title: string; description: string }
    > = {
      stopped: {
        title: '开发环境未启动',
        description:
          runtime.message || '启动开发环境后，这里会显示网页应用实时预览。',
      },
      starting: {
        title: '正在启动开发环境',
        description: '首次启动可能需要准备依赖，请稍候。',
      },
      recovering: {
        title: '正在恢复开发环境',
        description: '控制面正在恢复已有执行，不会创建新的运行实例。',
      },
      stopping: {
        title: '正在停止开发环境',
        description: '运行实例正在终止，完成前不会启动新的实例。',
      },
      cleanup_pending: {
        title: '正在清理运行资源',
        description: '服务已进入安全清理阶段，请稍候。',
      },
      error: {
        title: '预览异常',
        description: runtime.message || '请查看安全日志并按提示重试。',
      },
    };
    const runningWithoutPreview = runtime.status === 'running';
    const copy = runningWithoutPreview
      ? undefined
      : stateCopy[
          runtime.status as Exclude<AppDevRuntimeInfo['status'], 'running'>
        ];
    const title = runningWithoutPreview
      ? '预览地址尚未就绪'
      : copy?.title || '预览暂不可用';
    const description = runningWithoutPreview
      ? '运行环境已启动，正在等待可信 Preview Gateway 地址。'
      : copy?.description || '请刷新运行状态后重试。';

    return (
      <div className="app-dev-preview-panel app-dev-preview-panel--empty">
        <span
          className="app-dev-preview-panel__status"
          data-status={runtime.status}
        >
          {runtime.status}
        </span>
        <h3>{title}</h3>
        <p>{description}</p>
        <ol className="app-dev-preview-panel__readiness">
          {RUNTIME_STEPS.map(step => (
            <li
              key={step.key}
              data-active={
                step.key === runtime.status ||
                (runtime.status === 'recovering' && step.key === 'starting')
              }
              data-done={
                runtime.status === 'running' && step.key === 'starting'
              }
            >
              <strong>{step.title}</strong>
              <span>{step.description}</span>
            </li>
          ))}
        </ol>
        <div className="app-dev-preview-panel__empty-actions">
          <button type="button" onClick={refreshPreview}>
            刷新状态
          </button>
          {onOpenDesignMode ? (
            <button type="button" onClick={onOpenDesignMode}>
              {designModeActive ? '继续设计' : '打开设计模式'}
            </button>
          ) : null}
          {onOpenLogs ? (
            <button type="button" onClick={onOpenLogs}>
              查看日志
            </button>
          ) : null}
        </div>
      </div>
    );
  }

  return (
    <div className="app-dev-preview-panel">
      <div className="app-dev-preview-panel__bar">
        <div className="app-dev-preview-panel__address">
          <span>Live Preview</span>
          <code>{previewUrl}</code>
        </div>
        <div className="app-dev-preview-panel__devices" role="group">
          {(['desktop', 'tablet', 'mobile'] as const).map(option => (
            <button
              key={option}
              type="button"
              data-active={device === option}
              aria-pressed={device === option}
              onClick={() => setDevice(option)}
            >
              {option === 'desktop'
                ? '桌面'
                : option === 'tablet'
                  ? '平板'
                  : '手机'}
            </button>
          ))}
        </div>
        {onOpenDesignMode ? (
          <button
            type="button"
            className="app-dev-preview-panel__design-status"
            data-active={designModeActive}
            aria-pressed={designModeActive}
            onClick={onOpenDesignMode}
          >
            <span>{designModeActive ? '设计中' : '设计模式'}</span>
            <strong>{designSummary}</strong>
          </button>
        ) : null}
        <div className="app-dev-preview-panel__actions">
          {onSelectPreviewPoint ? (
            <button
              type="button"
              data-active={pickingElement}
              aria-pressed={pickingElement}
              onClick={() => setPickingElement(active => !active)}
            >
              {pickingElement ? '退出选择' : '选择元素'}
            </button>
          ) : null}
          <button type="button" onClick={refreshPreview}>
            刷新
          </button>
          <button
            type="button"
            onClick={event => openFullscreen(event.currentTarget)}
          >
            全屏
          </button>
          <a href={previewUrl} target="_blank" rel="noreferrer noopener">
            新窗口
          </a>
        </div>
      </div>
      {onSelectDesignTarget && designTargetOptions.length ? (
        <div
          className="app-dev-preview-panel__design-targets"
          aria-label="选择设计编辑区域"
        >
          <span>编辑区域</span>
          {designTargetOptions.map(option => (
            <button
              key={option}
              type="button"
              data-active={designTarget === option}
              aria-pressed={designTarget === option}
              onClick={() => onSelectDesignTarget(option)}
            >
              {option}
            </button>
          ))}
        </div>
      ) : null}
      <div className="app-dev-preview-panel__viewport" data-device={device}>
        <iframe
          key={`${previewUrl}-${refreshKey}`}
          title="网页应用预览"
          src={previewUrl}
          sandbox={PREVIEW_SANDBOX}
          referrerPolicy="no-referrer"
        />
        {designSelection && designSelection.device === device ? (
          <span
            className="app-dev-preview-panel__selection-marker"
            style={{
              left: `${designSelection.xPercent}%`,
              top: `${designSelection.yPercent}%`,
            }}
            aria-label="已选择的预览位置"
          />
        ) : null}
        {pickingElement && onSelectPreviewPoint ? (
          <button
            type="button"
            className="app-dev-preview-panel__pick-layer"
            onClick={event => {
              const rect = event.currentTarget.getBoundingClientRect();
              const xPx = Math.max(0, Math.round(event.clientX - rect.left));
              const yPx = Math.max(0, Math.round(event.clientY - rect.top));
              const xPercent = Number(
                Math.min(100, Math.max(0, (xPx / rect.width) * 100)).toFixed(1),
              );
              const yPercent = Number(
                Math.min(100, Math.max(0, (yPx / rect.height) * 100)).toFixed(
                  1,
                ),
              );

              onSelectPreviewPoint({
                target: designTarget,
                device,
                xPercent,
                yPercent,
                xPx,
                yPx,
                viewportWidth: Math.round(rect.width),
                viewportHeight: Math.round(rect.height),
                selectedAt: new Date().toISOString(),
              });
              setPickingElement(false);
            }}
          >
            <span>点击预览中的元素或区域</span>
            <em>会记录位置并带入 AI 设计任务</em>
          </button>
        ) : null}
      </div>
      {fullscreen ? (
        <AccessibleDialog
          title="全屏预览"
          maskClassName="app-dev-preview-panel__fullscreen"
          className="app-dev-preview-panel__fullscreen-content"
          onClose={closeFullscreen}
          closeOnBackdrop
        >
          <header>
            <span>{previewUrl}</span>
            <div>
              <button type="button" onClick={refreshPreview}>
                刷新
              </button>
              {onOpenDesignMode ? (
                <button
                  type="button"
                  onClick={() => {
                    setFullscreen(false);
                    onOpenDesignMode();
                  }}
                >
                  设计模式
                </button>
              ) : null}
              <a href={previewUrl} target="_blank" rel="noreferrer noopener">
                新窗口打开
              </a>
              <button
                data-dialog-autofocus
                type="button"
                onClick={closeFullscreen}
              >
                关闭
              </button>
            </div>
          </header>
          <iframe
            key={`fullscreen-${previewUrl}-${refreshKey}`}
            title="网页应用全屏预览"
            src={previewUrl}
            sandbox={PREVIEW_SANDBOX}
            referrerPolicy="no-referrer"
          />
        </AccessibleDialog>
      ) : null}
    </div>
  );
};
