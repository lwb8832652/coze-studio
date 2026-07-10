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

import type { AppDevModel } from '../types';
import { listAppDevModels, normalizeAppDevError } from '../service';
import { useLatestRequest } from './use-latest-request';

export const useAppDevModelSelector = (spaceId?: string) => {
  const [models, setModels] = useState<AppDevModel[]>([]);
  const [selectedModelId, setSelectedModelId] = useState('');
  const [loading, setLoading] = useState(Boolean(spaceId));
  const [error, setError] = useState('');
  const { beginRequest, invalidateRequests } = useLatestRequest();

  const refresh = useCallback(async () => {
    const isLatest = beginRequest();
    if (!spaceId) {
      if (isLatest()) {
        setModels([]);
        setSelectedModelId('');
        setLoading(false);
      }
      return;
    }

    setLoading(true);
    setError('');

    try {
      const result = await listAppDevModels({ spaceId });
      if (isLatest()) {
        setModels(result.items);
        setSelectedModelId(current =>
          result.items.some(model => model.id === current)
            ? current
            : result.items[0]?.id || '',
        );
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
  }, [beginRequest, spaceId]);

  useEffect(() => {
    invalidateRequests();
    setModels([]);
    setSelectedModelId('');
    setLoading(Boolean(spaceId));
    setError('');
    void refresh();
  }, [invalidateRequests, refresh, spaceId]);

  return {
    models,
    selectedModelId,
    setSelectedModelId,
    loading,
    error,
    hasUsableModel: models.length > 0,
    refresh,
  };
};
