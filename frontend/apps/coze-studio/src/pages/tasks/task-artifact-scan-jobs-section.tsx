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

import { Button, List } from '@coze-arch/coze-design';

import { TaskArtifactScanJobListItem } from './task-artifact-scan-job-list-item';
import {
  listTaskThreadArtifactScanJobs,
  retryTaskThreadArtifactScanJob,
  type TaskThreadArtifactScanJob,
} from './service';

const SCAN_JOB_PAGE_SIZE = 20;

export const TaskArtifactScanJobsSection = ({
  spaceId,
  threadId,
  visible,
}: {
  spaceId?: string;
  threadId: string;
  visible: boolean;
}) => {
  const [scanJobs, setScanJobs] = useState<TaskThreadArtifactScanJob[]>([]);
  const [scanJobTotal, setScanJobTotal] = useState(0);
  const [scanJobsLoading, setScanJobsLoading] = useState(false);
  const [scanJobsError, setScanJobsError] = useState('');
  const [activeRetryJob, setActiveRetryJob] = useState('');
  const sortedScanJobs = useMemo(
    () =>
      [...scanJobs].sort(
        (left, right) =>
          right.updated_at - left.updated_at ||
          right.job_id.localeCompare(left.job_id),
      ),
    [scanJobs],
  );

  const loadScanJobs = useCallback(async () => {
    setScanJobsLoading(true);
    setScanJobsError('');
    try {
      const response = await listTaskThreadArtifactScanJobs({
        thread_id: threadId,
        space_id: spaceId,
        page: 1,
        page_size: SCAN_JOB_PAGE_SIZE,
      });
      setScanJobs(response.data?.jobs ?? []);
      setScanJobTotal(response.data?.total ?? 0);
    } catch (err) {
      setScanJobsError(
        err instanceof Error ? err.message : '读取产物扫描队列失败',
      );
    } finally {
      setScanJobsLoading(false);
    }
  }, [spaceId, threadId]);

  useEffect(() => {
    if (visible) {
      void loadScanJobs();
    }
  }, [loadScanJobs, visible]);

  const handleRetryScanJob = async (job: TaskThreadArtifactScanJob) => {
    if (activeRetryJob) {
      return;
    }
    setActiveRetryJob(job.job_id);
    setScanJobsError('');
    try {
      await retryTaskThreadArtifactScanJob({
        thread_id: threadId,
        job_id: job.job_id,
        space_id: spaceId,
      });
      await loadScanJobs();
    } catch (err) {
      setScanJobsError(
        err instanceof Error ? err.message : '重试产物扫描任务失败',
      );
    } finally {
      setActiveRetryJob('');
    }
  };

  return (
    <section className="coze-prototype-artifact-scan-section">
      <div className="coze-prototype-artifact-scan-header">
        <div>
          <h3 className="coze-prototype-artifact-scan-heading">
            扫描队列 {scanJobTotal}
          </h3>
          <div className="coze-prototype-artifact-scan-subtitle">
            最近 20 条扫描任务，仅展示安全元数据
          </div>
        </div>
        <Button
          aria-label="刷新扫描队列"
          disabled={scanJobsLoading}
          loading={scanJobsLoading}
          size="small"
          theme="borderless"
          onClick={() => void loadScanJobs()}
        >
          刷新
        </Button>
      </div>
      {scanJobsError ? (
        <div className="coze-prototype-error">{scanJobsError}</div>
      ) : null}
      <List
        dataSource={sortedScanJobs}
        emptyContent={
          <div className="coze-prototype-artifacts-empty">暂无扫描任务</div>
        }
        loading={scanJobsLoading}
        renderItem={job => (
          <TaskArtifactScanJobListItem
            activeRetryJob={activeRetryJob}
            job={job as TaskThreadArtifactScanJob}
            onRetry={handleRetryScanJob}
          />
        )}
      />
    </section>
  );
};
