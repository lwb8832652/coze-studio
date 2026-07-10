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

import { useCallback, useEffect, useMemo, useState } from 'react';

import type { AppDevDataSource } from '../types';
import { listAppDevDataSources, normalizeAppDevError } from '../service';

export const useAppDevDataSources = (spaceId?: string) => {
  const [dataSources, setDataSources] = useState<AppDevDataSource[]>([]);
  const [selectedIds, setSelectedIds] = useState<string[]>([]);
  const [loading, setLoading] = useState(Boolean(spaceId));
  const [error, setError] = useState('');

  const refresh = useCallback(async () => {
    if (!spaceId) {
      setDataSources([]);
      setSelectedIds([]);
      return;
    }

    setLoading(true);
    setError('');
    try {
      const result = await listAppDevDataSources({ spaceId });
      setDataSources(result.items);
      setSelectedIds(current =>
        current.filter(id => result.items.some(item => item.id === id)),
      );
    } catch (requestError) {
      setError(normalizeAppDevError(requestError));
    } finally {
      setLoading(false);
    }
  }, [spaceId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const selectedDataSources = useMemo(
    () => dataSources.filter(item => selectedIds.includes(item.id)),
    [dataSources, selectedIds],
  );

  const toggleDataSource = useCallback((id: string) => {
    setSelectedIds(current =>
      current.includes(id)
        ? current.filter(item => item !== id)
        : [...current, id].slice(0, 8),
    );
  }, []);

  return {
    dataSources,
    selectedIds,
    selectedDataSources,
    toggleDataSource,
    loading,
    error,
    refresh,
  };
};
