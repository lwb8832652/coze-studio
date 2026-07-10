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

import { useCallback, useEffect, useState } from 'react';

import type { AppDevProject } from '../types';
import { getAppDevProject, normalizeAppDevError } from '../service';
import { useLatestRequest } from './use-latest-request';

export const useAppDevProjectInfo = (spaceId?: string, projectId?: string) => {
  const [project, setProject] = useState<AppDevProject | undefined>();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const { beginRequest } = useLatestRequest();

  const refresh = useCallback(async () => {
    const isLatest = beginRequest();
    if (!spaceId || !projectId) {
      if (isLatest()) {
        setProject(undefined);
        setLoading(false);
        setError('项目地址不完整，请返回项目列表重新进入');
      }
      return;
    }

    setProject(undefined);
    setLoading(true);
    setError('');

    try {
      const result = await getAppDevProject({ spaceId, projectId });
      if (isLatest()) {
        setProject(result);
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
  }, [beginRequest, projectId, spaceId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return {
    project,
    loading,
    error,
    refresh,
  };
};
