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

import { useCallback, useEffect, useState } from 'react';

import type { AppDevProject } from '../types';
import {
  archiveAppDevProject,
  createAppDevProject,
  duplicateAppDevProject,
  importAppDevProject,
  listAppDevProjects,
  normalizeAppDevError,
  updateAppDevProject,
} from '../service';
import { useLatestRequest } from './use-latest-request';

export const useAppDevProjects = (spaceId?: string) => {
  const [projects, setProjects] = useState<AppDevProject[]>([]);
  const [total, setTotal] = useState(0);
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [creating, setCreating] = useState(false);
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState('');
  const {
    beginRequest: beginListRequest,
    invalidateRequests: invalidateListRequests,
  } = useLatestRequest();
  const {
    beginRequest: beginMutationRequest,
    invalidateRequests: invalidateMutationRequests,
  } = useLatestRequest();

  const fetchProjects = useCallback(
    async (nextKeyword = keyword) => {
      const isLatest = beginListRequest();
      if (!spaceId) {
        if (isLatest()) {
          setProjects([]);
          setTotal(0);
          setLoading(false);
        }
        return;
      }

      setLoading(true);
      setError('');

      try {
        const result = await listAppDevProjects({
          spaceId,
          keyword: nextKeyword,
        });
        if (isLatest()) {
          setProjects(result.items);
          setTotal(result.total);
        }
      } catch (requestError) {
        if (isLatest()) {
          setError(normalizeAppDevError(requestError));
        }
      } finally {
        if (isLatest()) {
          setLoading(false);
        }
      }
    },
    [beginListRequest, keyword, spaceId],
  );

  useEffect(() => {
    invalidateListRequests();
    invalidateMutationRequests();
    setProjects([]);
    setTotal(0);
    setLoading(false);
    setCreating(false);
    setImporting(false);
    setError('');
  }, [invalidateListRequests, invalidateMutationRequests, spaceId]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      void fetchProjects(keyword);
    }, 300);

    return () => window.clearTimeout(timer);
  }, [fetchProjects, keyword]);

  const createProject = useCallback(
    async ({ name, prompt }: { name: string; prompt: string }) => {
      if (!spaceId) {
        throw new Error('空间不存在，请刷新后重试');
      }

      const isLatest = beginMutationRequest();

      setCreating(true);
      try {
        const project = await createAppDevProject({
          spaceId,
          name,
          prompt,
        });
        if (isLatest()) {
          await fetchProjects(keyword);
        }
        return project;
      } catch (requestError) {
        throw new Error(normalizeAppDevError(requestError));
      } finally {
        if (isLatest()) {
          setCreating(false);
        }
      }
    },
    [beginMutationRequest, fetchProjects, keyword, spaceId],
  );

  const importProject = useCallback(
    async ({ file, name }: { file: File; name?: string }) => {
      if (!spaceId) {
        throw new Error('空间不存在，请刷新后重试');
      }

      const isLatest = beginMutationRequest();

      setImporting(true);
      try {
        const project = await importAppDevProject({
          spaceId,
          file,
          name,
        });
        if (isLatest()) {
          await fetchProjects(keyword);
        }
        return project;
      } catch (requestError) {
        throw new Error(normalizeAppDevError(requestError));
      } finally {
        if (isLatest()) {
          setImporting(false);
        }
      }
    },
    [beginMutationRequest, fetchProjects, keyword, spaceId],
  );

  const archiveProject = useCallback(
    async (projectId: string) => {
      if (!spaceId) {
        throw new Error('空间不存在，请刷新后重试');
      }

      const isLatest = beginMutationRequest();

      try {
        await archiveAppDevProject({ spaceId, projectId });
        if (isLatest()) {
          await fetchProjects(keyword);
        }
      } catch (requestError) {
        throw new Error(normalizeAppDevError(requestError));
      }
    },
    [beginMutationRequest, fetchProjects, keyword, spaceId],
  );

  const updateProject = useCallback(
    async ({
      projectId,
      name,
      description,
    }: {
      projectId: string;
      name: string;
      description?: string;
    }) => {
      if (!spaceId) {
        throw new Error('空间不存在，请刷新后重试');
      }

      const isLatest = beginMutationRequest();

      try {
        const project = await updateAppDevProject({
          spaceId,
          projectId,
          name,
          description,
        });
        if (isLatest()) {
          await fetchProjects(keyword);
        }
        return project;
      } catch (requestError) {
        throw new Error(normalizeAppDevError(requestError));
      }
    },
    [beginMutationRequest, fetchProjects, keyword, spaceId],
  );

  const duplicateProject = useCallback(
    async ({ projectId, name }: { projectId: string; name?: string }) => {
      if (!spaceId) {
        throw new Error('空间不存在，请刷新后重试');
      }

      const isLatest = beginMutationRequest();

      try {
        const project = await duplicateAppDevProject({
          spaceId,
          projectId,
          name,
        });
        if (isLatest()) {
          await fetchProjects(keyword);
        }
        return project;
      } catch (requestError) {
        throw new Error(normalizeAppDevError(requestError));
      }
    },
    [beginMutationRequest, fetchProjects, keyword, spaceId],
  );

  return {
    projects,
    total,
    keyword,
    setKeyword,
    loading,
    creating,
    importing,
    error,
    refresh: fetchProjects,
    createProject,
    importProject,
    archiveProject,
    updateProject,
    duplicateProject,
  };
};
