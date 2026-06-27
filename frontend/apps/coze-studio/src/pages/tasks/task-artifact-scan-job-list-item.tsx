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

import { Button, Tag } from '@coze-arch/coze-design';

import { scanStatusColor } from './task-artifacts-helpers';
import { type TaskThreadArtifactScanJob } from './service';

export const TaskArtifactScanJobListItem = ({
  activeRetryJob,
  job,
  onRetry,
}: {
  activeRetryJob: string;
  job: TaskThreadArtifactScanJob;
  onRetry: (job: TaskThreadArtifactScanJob) => void | Promise<void>;
}) => {
  const retryable = job.status === 'failed';
  const retrying = activeRetryJob === job.job_id;

  return (
    <div className="coze-prototype-artifact-scan-item">
      <div className="coze-prototype-artifact-scan-main">
        <div className="coze-prototype-artifact-scan-title-row">
          <span className="coze-prototype-artifact-scan-title">
            {job.job_id}
          </span>
          <Tag color={scanStatusColor(job.status)}>{job.status}</Tag>
        </div>
        <div className="coze-prototype-artifact-scan-meta">
          <span>{job.scanner || 'default scanner'}</span>
          <span>尝试 {job.attempt_count}</span>
          {job.worker_id ? <span>{job.worker_id}</span> : null}
          {job.run_id ? <span>Run {job.run_id}</span> : null}
        </div>
        {job.last_error ? (
          <div className="coze-prototype-artifact-scan-error">
            {job.last_error}
          </div>
        ) : null}
      </div>
      {retryable ? (
        <Button
          aria-label={`重试扫描任务 ${job.job_id}`}
          disabled={Boolean(activeRetryJob) && !retrying}
          loading={retrying}
          size="small"
          theme="borderless"
          onClick={() => void onRetry(job)}
        >
          重试
        </Button>
      ) : null}
    </div>
  );
};
