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

import { normalizeAppDevPreviewUrl } from '../utils/preview-url';
import type { AppDevDesignSelection } from './chat-panel';
import type { AppDevRuntimeInfo } from '../types';

interface PreviewPanelProps {
  runtime: AppDevRuntimeInfo;
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
  const previewUrl = normalizeAppDevPreviewUrl(runtime.previewUrl);
  const unsafePreviewUrl = Boolean(
    runtime.status === 'running' && runtime.previewUrl && !previewUrl,
  );
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

  useEffect(() => {
    if (!fullscreen) {
      return;
    }

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setFullscreen(false);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [fullscreen]);

  useEffect(() => {
    if (!fullscreenSignal || runtime.status !== 'running' || !previewUrl) {
      return;
    }

    setFullscreen(true);
  }, [fullscreenSignal, previewUrl, runtime.status]);

  if (runtime.status !== 'running' || !previewUrl) {
    const title = unsafePreviewUrl
      ? '预览地址不安全'
      : runtime.status === 'error'
        ? '预览异常'
        : '暂无预览';
    const description = unsafePreviewUrl
      ? '预览地址未通过安全校验，请检查 Preview Gateway 配置。'
      : runtime.status === 'starting' || runtime.status === 'restarting'
        ? '开发环境正在启动，首次启动会安装依赖，请稍等片刻。'
        : runtime.message || '启动开发环境后，这里会显示网页应用实时预览。';

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
                (runtime.status === 'restarting' && step.key === 'starting')
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
              onClick={() => setPickingElement(active => !active)}
            >
              {pickingElement ? '退出选择' : '选择元素'}
            </button>
          ) : null}
          <button type="button" onClick={refreshPreview}>
            刷新
          </button>
          <button type="button" onClick={() => setFullscreen(true)}>
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
        <div
          className="app-dev-preview-panel__fullscreen"
          role="dialog"
          aria-modal="true"
          aria-label="全屏预览"
          onClick={event => {
            if (event.target === event.currentTarget) {
              setFullscreen(false);
            }
          }}
        >
          <section>
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
                <button type="button" onClick={() => setFullscreen(false)}>
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
          </section>
        </div>
      ) : null}
    </div>
  );
};
