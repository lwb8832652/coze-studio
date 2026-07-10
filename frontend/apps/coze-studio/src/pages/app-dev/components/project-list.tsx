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

import { useMemo, useState } from 'react';

import { isAppDevReleaseStale } from '../utils/release-status';
import type { AppDevProject } from '../types';

const STATUS_LABELS: Record<AppDevProject['status'], string> = {
  creating: '创建中',
  ready: '可开发',
  archived: '已归档',
  error: '异常',
};

const formatPublishType = (publishType?: string) => {
  switch (publishType) {
    case 'AGENT':
      return '应用';
    case 'PAGE':
    case 'page':
      return '页面';
    default:
      return publishType ? publishType : '页面';
  }
};

interface ProjectListProps {
  projects: AppDevProject[];
  total: number;
  loading?: boolean;
  error?: string;
  keyword: string;
  onKeywordChange: (keyword: string) => void;
  onRefresh: () => void;
  onCreate: () => void;
  onImport: () => void;
  onOpen: (project: AppDevProject) => void;
  onArchive: (project: AppDevProject) => void;
  onRename: (project: AppDevProject) => void;
  onDuplicate: (project: AppDevProject) => void;
  onExport: (project: AppDevProject) => void;
}

export const ProjectList = ({
  projects,
  total,
  loading,
  error,
  keyword,
  onKeywordChange,
  onRefresh,
  onCreate,
  onImport,
  onOpen,
  onArchive,
  onRename,
  onDuplicate,
  onExport,
}: ProjectListProps) => {
  const [publishFilter, setPublishFilter] = useState<
    'all' | 'published' | 'unpublished'
  >('all');
  const visibleProjects = useMemo(
    () =>
      projects.filter(project => {
        const published = project.lastBuildStatus === 'success';
        if (publishFilter === 'published') {
          return published;
        }
        if (publishFilter === 'unpublished') {
          return !published;
        }
        return true;
      }),
    [projects, publishFilter],
  );
  const countText =
    publishFilter === 'all'
      ? `${total} 个项目`
      : `${visibleProjects.length} 个项目`;

  return (
    <section className="app-dev-project-list">
      <div className="app-dev-project-list__header">
        <div className="app-dev-project-list__header-left">
          <h1>网页应用开发</h1>
          <button
            type="button"
            className="app-dev-project-list__filter-pill"
            data-active={publishFilter === 'all'}
            onClick={() => setPublishFilter('all')}
          >
            全部
          </button>
          <button
            type="button"
            className="app-dev-project-list__filter-pill"
            data-active={publishFilter === 'published'}
            onClick={() => setPublishFilter('published')}
          >
            已发布
          </button>
          <button
            type="button"
            className="app-dev-project-list__filter-pill"
            data-active={publishFilter === 'unpublished'}
            onClick={() => setPublishFilter('unpublished')}
          >
            未发布
          </button>
          <span className="app-dev-project-list__filter-pill app-dev-project-list__filter-pill--static">
            网页应用
          </span>
          <span className="app-dev-project-list__count">{countText}</span>
        </div>
        <div className="app-dev-project-list__toolbar">
          <label className="app-dev-project-list__search">
            <span>搜索</span>
            <input
              value={keyword}
              placeholder="搜索页面名称"
              onChange={event => onKeywordChange(event.target.value)}
            />
            {keyword.trim() ? (
              <button type="button" onClick={() => onKeywordChange('')}>
                清空
              </button>
            ) : null}
          </label>
          <button type="button" onClick={onRefresh} disabled={loading}>
            刷新
          </button>
          <button type="button" onClick={onImport}>
            导入项目
          </button>
          <button type="button" onClick={onCreate}>
            + 网页应用
          </button>
        </div>
      </div>

      {loading ? (
        <div className="app-dev-project-list__state">正在加载网页应用...</div>
      ) : null}

      {!loading && error ? (
        <div className="app-dev-project-list__state app-dev-project-list__state--error">
          <span>{error}</span>
          <button type="button" onClick={onRefresh}>
            重试
          </button>
        </div>
      ) : null}

      {!loading && !error && !visibleProjects.length ? (
        <div className="app-dev-project-list__empty">
          <div className="app-dev-page__empty-icon" aria-hidden="true">
            &lt;/&gt;
          </div>
          <h2>
            {keyword.trim()
              ? '未找到匹配的网页应用'
              : projects.length
                ? '未找到符合条件的网页应用'
                : '还没有网页应用'}
          </h2>
          <p>
            {keyword.trim()
              ? '可以清空搜索词，或换一个页面名称继续查找。'
              : projects.length
                ? '可以调整发布状态筛选，或搜索其它页面名称。'
                : '创建一个网页应用，开始使用 AI 生成、编辑和预览页面。'}
          </p>
          {keyword.trim() || projects.length ? (
            <button
              type="button"
              onClick={() => {
                setPublishFilter('all');
                if (keyword.trim()) {
                  onKeywordChange('');
                }
              }}
            >
              {keyword.trim() ? '清空搜索' : '查看全部'}
            </button>
          ) : (
            <button type="button" onClick={onCreate}>
              创建网页应用
            </button>
          )}
        </div>
      ) : null}

      {!loading && !error && visibleProjects.length ? (
        <>
          <div className="app-dev-project-grid">
            {visibleProjects.map(project => {
              const published = project.lastBuildStatus === 'success';
              const releaseStale = isAppDevReleaseStale(project);
              const publishTypeLabel = formatPublishType(project.lastBuildType);
              return (
                <article className="app-dev-project-card" key={project.id}>
                  <div className="app-dev-project-card__cover">
                    <span aria-hidden="true">&lt;/&gt;</span>
                    <strong>{project.name.slice(0, 1).toUpperCase()}</strong>
                  </div>
                  <div>
                    <span
                      className="app-dev-project-card__publish-status"
                      data-status={
                        releaseStale
                          ? 'stale'
                          : published
                            ? 'published'
                            : 'unpublished'
                      }
                    >
                      {releaseStale
                        ? '需重新发布'
                        : published
                          ? '已发布'
                          : '未发布'}
                    </span>
                    <span
                      className="app-dev-project-card__status"
                      data-status={project.status}
                    >
                      {STATUS_LABELS[project.status]}
                    </span>
                    {published ? (
                      <span className="app-dev-project-card__publish-type">
                        {publishTypeLabel}
                      </span>
                    ) : null}
                    <h3>{project.name}</h3>
                    <p>{project.description || project.prompt || '暂无描述'}</p>
                  </div>
                  <footer>
                    <span>
                      {project.creatorName || '未知创建者'}
                      {project.updatedAt ? ` · ${project.updatedAt}` : ''}
                    </span>
                    <div className="app-dev-project-card__actions">
                      <button
                        type="button"
                        className="app-dev-project-card__primary-action"
                        onClick={() => onOpen(project)}
                      >
                        进入开发
                      </button>
                      <details className="app-dev-project-card__more">
                        <summary>更多</summary>
                        <div>
                          <button
                            type="button"
                            onClick={() => onRename(project)}
                          >
                            重命名
                          </button>
                          <button
                            type="button"
                            onClick={() => onDuplicate(project)}
                          >
                            复制项目
                          </button>
                          <button
                            type="button"
                            onClick={() => onExport(project)}
                          >
                            导出源码
                          </button>
                          <button
                            type="button"
                            onClick={() => onArchive(project)}
                          >
                            归档
                          </button>
                        </div>
                      </details>
                    </div>
                  </footer>
                </article>
              );
            })}
          </div>
        </>
      ) : null}
    </section>
  );
};
