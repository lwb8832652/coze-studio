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

import { useMemo, useState } from 'react';

import type { AppDevRuntimeLog } from '../types';

interface DevLogsPanelProps {
  logs: AppDevRuntimeLog[];
  loading?: boolean;
  onRefresh: () => void;
}

export const DevLogsPanel = ({
  logs,
  loading,
  onRefresh,
}: DevLogsPanelProps) => {
  const [levelFilter, setLevelFilter] = useState<
    'all' | AppDevRuntimeLog['level']
  >('all');
  const filteredLogs = useMemo(
    () =>
      levelFilter === 'all'
        ? logs
        : logs.filter(log => log.level === levelFilter),
    [levelFilter, logs],
  );
  const levelCount = useMemo(
    () =>
      logs.reduce(
        (result, log) => ({
          ...result,
          [log.level]: result[log.level] + 1,
        }),
        { debug: 0, info: 0, warn: 0, error: 0 },
      ),
    [logs],
  );

  return (
    <section className="app-dev-logs-panel">
      <header>
        <div>
          <span>开发日志</span>
          <small>{logs.length} 条运行事件</small>
        </div>
        <button type="button" onClick={onRefresh} disabled={loading}>
          {loading ? '刷新中...' : '刷新'}
        </button>
      </header>
      <div className="app-dev-logs-panel__filters" role="group">
        {(['all', 'error', 'warn', 'info', 'debug'] as const).map(level => (
          <button
            key={level}
            type="button"
            data-active={levelFilter === level}
            onClick={() => setLevelFilter(level)}
          >
            {level === 'all' ? '全部' : level}
            <span>{level === 'all' ? logs.length : levelCount[level]}</span>
          </button>
        ))}
      </div>
      {!filteredLogs.length ? (
        <div className="app-dev-logs-panel__empty">
          {logs.length ? '当前筛选下暂无日志' : '暂无开发日志'}
        </div>
      ) : (
        <ol>
          {filteredLogs.map(log => (
            <li key={log.id} data-level={log.level}>
              <time>{log.timestamp}</time>
              <span>{log.level}</span>
              <p>{log.message}</p>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
};
